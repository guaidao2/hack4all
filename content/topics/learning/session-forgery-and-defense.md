---
id: session-forgery-and-defense
title_en: "Session Management, Part 2 — When the Signing Key Is the Whole Security Model"
title_zh: "会话管理（二）：当签名密钥就是全部的安全模型"
summary_en: This part follows the unforgeable property to its single point of failure. Measured — a weak key and a hard-coded one both recovered offline from nine candidates, a strong one not, and a signed pickle payload that verifies and then executes.
summary_zh: 这一篇把"不可伪造"这条性质追到它唯一的失效点。实测：弱密钥与硬编码密钥都能从九条候选里离线还原，强密钥不能；而一个签过名的 pickle 载荷会通过校验、然后被执行。
tags: [web, session, cwe-565, cwe-502, cookies, serialisation]
tools: [python3, curl, flask-unsign, hashcat]
attck: [T1539, T1550, T1505.003]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### One property, one secret

The previous entry listed three properties a session identifier needs: unpredictable, unforgeable, revocable. Signed cookies address the second, and they do it in the way most cryptographic mechanisms do — **by moving the entire problem into a key**.

The signature itself is not the weak part. What varies is where the key lives and how it was chosen, and both are outside the algorithm:

| Where the key comes from | How it leaves |
|---|---|
| **Hard-coded in the source** | Repository, image layers, a decompiled package, a shared snippet |
| A config file or environment variable | Path traversal, SSRF, a backup file, a log line, a stack trace |
| A short or guessable string | Offline recovery — a signature verifies without contacting the server |
| The same value across environments | A test deployment's leak is production's leak |

Measured, with a nine-word candidate list and no requests sent:

| Signing key | Result |
|---|---|
| `secret` | **recovered** in under a millisecond |
| A hard-coded demo value | **recovered** in under a millisecond |
| 32 random bytes | not found |

**The second row is the one that matters in practice.** A hard-coded key is not weak in the sense of being short — it can be long and random-looking and still be public, because it is in the source, and the source is what the attacker is reading. "Recovered from a candidate list" and "recovered from a file" are the same outcome for the person doing it.

So the formulation worth keeping is this: **signing converts "can this be forged?" into "can the key be obtained?"** Everything the signature promises depends on the answer, and nothing in the signature improves it.

### What having the key buys, in two steps

**The first step is changing data.** Measured, with the key in hand:

```
original cookie: {"user": "alice", "role": "user"}
re-signed:       {"user": "alice", "role": "admin"}
```

The signature verifies, because it was produced by someone who holds the key. The payload is still just data, and the application now believes what it says.

**The second step applies when the payload is deserialised by something that can construct objects.** Measured with a signed payload that is a Python pickle:

```
signature valid: yes
deserialising it: the constructor ran  -> code execution
```

**Signing does not make deserialisation safe, and it does not filter anything.** It establishes that the payload came from a holder of the key. When the holder of the key is the attacker, and the deserialiser is unsafe, **the signature only establishes that this is the same unsafe input everyone else gets** — which is the deserialisation entry's problem, arriving through a session cookie.

That combination is why a signed cookie holding **structured objects** is worth flagging in review, whatever the signing key looks like: with a JSON payload, a key leak is an authorization bypass; with a pickle payload, it is remote code execution, and the difference is one line of configuration.

### The revocation dilemma

The previous entry noted that client-side state cannot be revoked. Here is what that means concretely when the key is also the thing that might have leaked.

**Do not rotate, and a leaked key stays useful forever.** Every cookie ever issued with that key remains valid — there is nothing to revoke, no denylist consulted, no server-side record to delete.

**Rotate, and every logged-in user is signed out.** Measured:

```
old cookie checked against the new key -> fails   (every session invalidated at once)
new cookie checked against the new key -> passes
```

There is no way to revoke one session, or one account's sessions, or the cookies issued in a window of time. The choice is between all and nothing, and a rotation performed under pressure — which is exactly when a key leak is discovered — is a mass logout.

A deployment can soften this with a transition period where two keys are accepted, and it should: rotating to a new key while continuing to accept the old one lets sessions migrate. But **the old key has to be removed on a deadline**, and until it is, the leaked key remains usable — so the transition period is also the length of the compromise.

**Server-side state does not have this problem**, which is the point of the comparison rather than an aside: revocation is the property that a client-side architecture gives up in exchange for not storing sessions.

### How this relates to the JWT entries

Same properties, different packaging:

| | JWT | Signed session cookie |
|---|---|---|
| Contents readable by the client | yes | yes |
| Forgeable given the key | yes | yes |
| Revocable | no | no |
| Format | **standardised** | **each framework's own** |
| Distinctive failure | **Algorithm confusion** — the token names its own algorithm | **Invented formats**, and hand-rolled signing |

**Standardisation cuts both ways.** A JWT library has an `alg` field to get wrong, which is why algorithm confusion exists as a named class; a session cookie has no such field, but it also has no specification that a thousand deployments have already argued over. A framework's session mechanism is usually well reviewed; a bespoke signed token invented in an afternoon, with a custom encoding and a signature over a concatenated string, is where the avoidable mistakes live.

### Detection and mitigation

- **Alert on bulk signature-verification failures.** As with JWTs, this indicates key recovery rather than a broken client, and it only works if "signature invalid" is logged distinctly from "no session presented".
- **Alert when session contents disagree with server state.** A cookie claiming a role or plan that the account does not have is the measured outcome of a successful forgery, and it is detectable by comparing the two — which requires the server to look up the account rather than trusting the cookie for that comparison.
- **Watch for session contents changing without a corresponding event.** A role or plan that changes with no administrative action, no purchase and no login is a payload being edited.
- **Watch the cookie size.** An injected extra field, or a payload that grew because it now carries a serialised object, is visible in the header length, and a session cookie has no reason to change size.
- **Scan the repository and the deployment for signing keys.** This is the mitigation and the detection at once: a key in a source file, in a committed config, in a container image layer or in a CI variable that is echoed into a log is a finding independent of whether anyone has used it.
- **For mitigation, use the framework's JSON-based session, and never a payload that constructs objects.** The measured pickle case is remote code execution from a key leak; the JSON case is an authorization bypass from the same leak. Choosing JSON removes a whole consequence, and it is a configuration choice rather than a redesign.
- **Keep the signing key out of the code, per environment, and long.** A secret manager, generated rather than chosen, distinct in every environment, and never in a repository. A key that appears in a source file is public regardless of its entropy.
- **Plan key rotation before it is urgent.** Two keys accepted during a bounded transition, the old one removed on a deadline, and the deadline written down. An unplanned rotation is a mass logout; an unbounded transition is a leak that never closes.
- **And the structural fix: let the cookie carry a reference, not a decision.** A cookie that says **"session 9f2a"** and lets the server look up the user, role and plan cannot be escalated by re-signing, because there is nothing in it to escalate — the worst a forged or edited cookie achieves is a lookup that fails. A cookie carrying `admin: true` is the authorization decision itself, held by the client, protected by one secret. **The reduction is: a cookie should say who you are, never what you may do** — and where the client does need to carry state, keep it to data the server is willing to re-check.
- **And treat anything a signed cookie authorises as authorised by the key, not by the user.** The consequence of a leak is set by what the payload was allowed to decide, which is the design question that determines everything above it.

<!-- lang:zh -->
### 一个性质，一个秘密

上一篇列出了会话标识符需要的三个性质：不可预测、不可伪造、可撤销。签名 cookie 处理的是第二个，而它用的方式和多数密码学机制一样 —— **把整个问题搬进一把密钥里**。

签名本身不是薄弱的那个部分。会变的是密钥放在哪、以及它是怎么被选出来的，而这两件事都在算法之外：

| 密钥从哪来 | 它怎么出去 |
|---|---|
| **硬编码在源码里** | 仓库、镜像层、反编译出来的包、被分享的代码片段 |
| 配置文件或环境变量 | 路径穿越、SSRF、备份文件、一行日志、一段调用栈 |
| 短或可猜的字符串 | 离线还原 —— 签名不需要联系服务端就能验证 |
| 多个环境复用同一个值 | 测试部署的泄露就是生产环境的泄露 |

实测，九条候选、一个请求都不发：

| 签名密钥 | 结果 |
|---|---|
| `secret` | **还原**，不到一毫秒 |
| 一个硬编码的演示值 | **还原**，不到一毫秒 |
| 32 字节随机 | 未命中 |

**第二行才是实践中要紧的那个。** 硬编码的密钥并不是"短"意义上的弱 —— 它可以又长又随机，而依然是公开的，因为它在源码里，而源码正是攻击者正在读的东西。"从候选列表里还原"和"从文件里还原"，对做这件事的人来说是同一个结果。

所以值得记住的表述是这一句：**签名把"这个能不能被伪造"换成了"这把密钥能不能被拿到"。** 签名所承诺的一切都取决于那个答案，而签名本身对这个答案没有任何改善。

### 拿到密钥之后，两步

**第一步是改数据。** 实测，密钥在手：

```
原始 cookie: {"user": "alice", "role": "user"}
重签之后:    {"user": "alice", "role": "admin"}
```

签名验证通过，因为它是由一个持有密钥的人产生的。载荷仍然只是数据，而应用现在相信它说的话。

**第二步在载荷被"能构造对象的东西"反序列化时适用。** 实测，一个签过名、内容是 Python pickle 的载荷：

```
签名有效: 是
反序列化它: 构造器被执行了  -> 代码执行
```

**签名不会让反序列化变安全，也不会过滤任何东西。** 它确立的是"这个载荷来自一个持有密钥的人"。当持有密钥的是攻击者、而反序列化不安全时，**签名只确立了"这是别人也会收到的同一个不安全输入"** —— 那是反序列化那篇的问题，经由一个会话 cookie 到达。

这个组合就是为什么"签名 cookie 里放着**结构化对象**"在评审时值得标出来，无论签名密钥看起来多好：JSON 载荷下，密钥泄露是授权绕过；pickle 载荷下，它是远程代码执行 —— 而两者的差别是一行配置。

### 撤销的两难

上一篇提到客户端状态无法撤销。当"可能已经泄露的东西"也正是那把密钥时，这意味着什么，值得具体说。

**不轮换，泄露的密钥就永久有用。** 用那把密钥签发过的每一个 cookie 都仍然有效 —— 没有任何东西可以撤销、没有黑名单被查询、没有服务端记录可以删。

**轮换，每一个已登录用户都被登出。** 实测：

```
用新密钥校验旧 cookie -> 失败    （所有会话一次性失效）
用新密钥校验新 cookie -> 通过
```

没有任何办法只撤销一个会话、一个账号的会话、或者某个时间窗内签发的 cookie。选择只有"全部"或"没有"，而在压力下执行的轮换 —— 发现密钥泄露时恰好就是这种时候 —— 是一次全体登出。

部署可以用"一段过渡期里接受两把密钥"来缓和，而且应该这么做：换到新密钥的同时继续接受旧的，让会话有机会迁移。但**旧密钥必须在一个截止时间被移除**，而在它被移除之前，泄露的密钥仍然可用 —— 所以那段过渡期同时也是这次沦陷的持续时间。

**服务端状态没有这个问题**，而这正是那个对照的重点而不是附带一句：撤销，就是客户端架构为了"不存会话"而放弃掉的那个性质。

### 这和 JWT 那几篇的关系

同样的性质，不同的包装：

| | JWT | 签名会话 cookie |
|---|---|---|
| 客户端可读内容 | 是 | 是 |
| 有密钥即可伪造 | 是 | 是 |
| 可撤销 | 否 | 否 |
| 格式 | **标准化的** | **各家框架自己的** |
| 特有失效 | **算法混淆** —— token 自己声明算法 | **自己发明的格式**，以及手写的签名 |

**标准化是双刃的。** 一个 JWT 库有一个 `alg` 字段可以搞错，这就是算法混淆存在为一类命名的原因；而一个会话 cookie 没有那个字段，但它也没有一份被上千个部署争论过的规范。框架的会话机制通常被审查得很好；而一个下午发明出来的自定义签名令牌 —— 自定编码、对一串拼接字符串做签名 —— 才是那些本可避免的错误住的地方。

### 检测与缓解

- **对成批的签名校验失败告警。** 和 JWT 一样，这说明的是密钥还原而不是客户端坏了，而它只有在"签名无效"被与"没有出示会话"区分开来记录时才起作用。
- **当会话内容与服务端状态不一致时告警。** 一个声称拥有某个角色或套餐、而账号并没有的 cookie，就是一次成功伪造的实测后果，而它可以通过把两者对比来发现 —— 这要求服务端去查账号，而不是拿 cookie 来做那次比较。
- **盯"会话内容变了、却没有对应事件"。** 一个角色或套餐在没有管理动作、没有购买、没有登录的情况下变化，就是一个正在被编辑的载荷。
- **盯 cookie 的大小。** 一个被注进去的额外字段，或者一个因为开始携带序列化对象而长大的载荷，在头长度上是看得见的 —— 而一个会话 cookie 没有理由改变大小。
- **扫描仓库与部署里的签名密钥。** 这既是缓解也是检测：源码文件里、被提交的配置里、容器镜像层里、或者被回显进日志的 CI 变量里的密钥，是一条独立于"有没有人用过它"的发现。
- **缓解上，用框架基于 JSON 的会话，绝不要用会构造对象的载荷。** 实测里 pickle 那种情况是"一次密钥泄露换来远程代码执行"；JSON 那种是"同一次泄露换来授权绕过"。选 JSON 移除了一整类后果，而它是一个配置选择，不是一次重新设计。
- **把签名密钥放在代码之外、按环境区分、并且够长。** 用密钥管理服务，生成而不是挑选，每个环境各不相同，绝不进仓库。一个出现在源码文件里的密钥，无论熵有多高都是公开的。
- **在它还不要紧之前就规划好密钥轮换。** 一段有界的过渡期里接受两把密钥，旧密钥在截止时间被移除，而那个截止时间要写下来。没有计划的轮换是一次全体登出；没有边界的过渡是一道永远关不上的泄露。
- **而结构性的修法是：让 cookie 携带一个引用，而不是一个决定。** 一个写着**"会话 9f2a"**、让服务端去查用户、角色与套餐的 cookie，无法通过重签来提升权限，因为里面没有东西可以提升 —— 一个被伪造或编辑过的 cookie 所能达到的最坏结果，是一次查不到的查询。一个携带 `admin: true` 的 cookie，就是那个授权决定本身，由客户端持有、由一个秘密保护着。**归纳起来：cookie 应该说你是谁，绝不说你可以做什么** —— 而在客户端确实需要携带状态的地方，把它限制在"服务端愿意重新核对"的数据上。
- **并且把"签名 cookie 授权的一切"当成由密钥授权，而不是由用户授权。** 一次泄露的后果由"那个载荷被允许决定什么"设定，而这正是那个决定上面一切的设计问题。
