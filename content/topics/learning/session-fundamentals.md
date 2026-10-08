---
id: session-fundamentals
title_en: "Session Management, Part 1 — A Session ID Is a Password"
title_zh: "会话管理（一）：会话 ID 就是密码"
summary_en: A session identifier is a bearer credential, so it needs everything a password needs plus revocability. Measured across the three places session state can live, the properties only a cookie can carry, and the two ways an identifier goes wrong before anyone steals it.
summary_zh: 会话标识符是一个持有者凭据，所以它需要密码需要的一切，另外还需要"可撤销"。这一篇量了会话状态能放的三个位置、只有 cookie 能带上的那些属性，以及标识符在被人偷走之前就已经出错的两种方式。
tags: [web, session, cwe-565, cwe-384, authentication, cookies]
tools: [python3, curl, browser devtools, flask-unsign]
attck: [T1539, T1550, T1078]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The whole security model in one sentence

HTTP has no memory. Every request arrives without context, so a site that wants to remember a logged-in user has to give the browser something to present next time — and then trust it.

> **The session identifier is a bearer credential: whoever presents it is treated as the person it belongs to.**

That is not a flaw in a particular implementation, it is the design. And it has a consequence that is easy to miss: **a session ID has to satisfy everything a password satisfies**, because it is used in place of one on every request — with one addition. A password can be changed when it is stolen; a session ID has to be **revocable**.

So there are three required properties, and the third is the one most implementations fail:

| Property | What it means | How it fails |
|---|---|---|
| **Unpredictable** | It cannot be guessed or derived | A non-cryptographic random source |
| **Unforgeable** | It cannot be constructed by a client | A signing key that is weak, leaked or absent |
| **Revocable** | The server can invalidate it | State kept entirely on the client |

### Four layers

1. **The trust boundary.** The server trusts a string it handed out earlier, presented by whoever has it. It has no way to tell the legitimate holder from anyone else, **and it is not supposed to** — that is what a bearer token is.
2. **Data and instruction share a plane.** The identifier is simultaneously **an index into server-side state** ("find this session") and **proof of identity** ("this is that user"). One value doing both jobs means anything that leaks it becomes a full compromise rather than a partial one.
3. **Why the usual fix fails.** Signing and encrypting session cookies makes the cookie tamper-proof, and invites the conclusion that the session is therefore safe. It is not: **a signature does not stop anyone reading the data, and it does not stop someone else using the cookie.** Neither does encryption. Both address the "unforgeable" property and leave the other two.
4. **The variants.** Cookie scope and the `__Host-` prefix, `SameSite`, an identifier passed in a URL instead of a cookie, and the same identifier stored where scripts can read it.

### Where the state can live

Three architectures, with different failure modes:

| Where the state lives | Revocable | Client can read it | Main risk |
|---|---|---|---|
| **On the server**, keyed by an ID | yes | no | Storage and sharing across instances |
| **In a signed cookie** | **no** | **yes — signing is not encryption** | A weak, leaked or hard-coded key gives forgery |
| **In an encrypted cookie** | no | no | Key management; still not revocable |

The middle row is worth dwelling on because it is the most common modern choice, and the one that most often gets the security reasoning wrong.

**Signing means the client cannot change the contents.** Measured on a signed cookie, with the payload replaced and the original signature kept:

```
replaced payload, reused signature -> signature does not match
```

**It does not mean the client cannot read them.** The measured cookie's first segment decodes to plain JSON:

```
{"user": "alice", "role": "user"}
```

**And it does not stop someone else using the cookie.** There is nothing in a signed cookie that ties it to a particular browser once it has been copied out.

The measured third point is what makes the design fragile: **the entire "unforgeable" property rests on one secret.**

```
signature verifies with the recovered key -> {"user": "alice", "role": "admin"}
```

And that secret can be attacked offline, exactly as in the JWT entries, because a signature is verifiable without contacting the server:

```
9 candidates -> found 'secret' in under a millisecond, with no request sent
```

**The comparison that matters is with the first row.** A server-side session keyed by an ID can be invalidated — logging out, changing a password, or an administrator revoking access all take effect immediately. With state on the client, **none of those can reach it**: the cookie remains valid until it expires, and "logging out" deletes a copy the server never knew about. That is the trade being made, and it is usually made without noticing.

### Three ways to carry the identifier

| Carrier | `HttpOnly` | `Secure` | `SameSite` | Note |
|---|---|---|---|---|
| **Cookie** | yes | yes | yes | The only carrier where all three apply |
| **URL parameter** | n/a | n/a | n/a | Lands in logs, history and `Referer` headers |
| **`localStorage`** | n/a | n/a | n/a | Readable by any script in the origin, so any XSS takes it |

`HttpOnly` removes the identifier from JavaScript's reach, which turns most XSS from "session stolen" into "actions performed as the user during the visit" — a serious downgrade of the attacker's position. `Secure` keeps it off plaintext connections. `SameSite` limits cross-site requests carrying it. **None of that is available to the other two carriers**, which is why an identifier in a URL or in storage is a finding on its own, independent of anything else being wrong.

### Two ways it goes wrong before anyone steals it

**Predictable identifiers.** Measured, generating IDs with a seeded non-cryptographic random source:

```
seed=1234 -> e302123b7000bfe4, 2530f72e221829fb
seed=1234 -> e302123b7000bfe4, 2530f72e221829fb     the same sequence, every time
seed=4321 -> 81d25b3151f9d7cd, e5e034e786ad7f69
```

With the cryptography-grade source, two calls never repeat and there is no seed to recover. The rule is narrow and absolute: **the identifier must come from a source designed for secrets.** A general-purpose random library is reproducible by design, and a framework default that uses one is a finding even before anyone attacks it.

**Session fixation.** If the identifier does not change when the user logs in, then anyone who knew it before the login knows a valid authenticated session afterwards:

```
no rotation:  before sess-XYZ  ->  after sess-XYZ
rotation:     before sess-XYZ  ->  after sess-7f3a91c2
```

Becoming authenticated is a change of privilege, and **every privilege change needs a new identifier** — including the transition from one role to another within a session. The attack is usually delivered by planting the identifier before the victim logs in: through a link containing it, or by setting the cookie from a related subdomain.

### Detection and mitigation

- **Alert when a session identifier is used from a different address or user agent than the one it was issued to.** This is the most direct compromise signal, and it needs a baseline: users on mobile networks change address legitimately, so the useful rule is a change in **country or network** together with other anomalies rather than any change at all.
- **Check that identifiers do not rotate on login, and that they are not a fixed value.** Both are directly testable in a pre-deployment test: log in twice and confirm the identifier changed, and confirm that a sample of issued identifiers shows no pattern, no counter and no timestamp.
- **Alert on bulk signature-verification failures.** As with JWTs, a burst of failed signature checks against a session cookie indicates a key-recovery attempt. The signal only exists if the failure is logged distinctly from "no session".
- **Alert when identifiers appear in URLs.** A session token in a path or query parameter reaches the access log, the `Referer` header of every outbound link, and browser history. Its appearance there is worth an alert because it cannot be un-leaked.
- **For mitigation, prefer server-side state.** It is the only architecture in which revocation works, and revocation is what makes the difference between "a stolen session until it expires" and "a stolen session until someone notices".
- **Use the platform's secure random source, and let the framework generate the identifier.** Do not build them, do not shorten them, and do not use a value derived from anything about the user.
- **Rotate on login, on privilege change and on password change.** Each is a point where the old identifier's authority should end — and rotating also invalidates anything an attacker planted earlier.
- **Keep session cookies where only the browser can put them.** `HttpOnly`, `Secure`, `SameSite`, a `__Host-` prefix where the platform supports it, and a narrow path and domain. A session cookie scoped to a parent domain is shared with every subdomain, including any that is less well maintained.
- **Treat a signed session cookie as an HMAC with all the JWT rules attached.** The key is a key: random, long, per environment, out of the repository, rotated, and never a value in a source file. **And put nothing confidential in the payload**, because signing does not hide it.
- **Give sessions both an idle timeout and an absolute lifetime.** An idle timeout bounds an abandoned session; an absolute lifetime bounds one that is being kept alive deliberately.
- **And make logout a server-side act.** With client-side state, deleting the cookie removes the user's copy and leaves the credential valid for anyone who already has one — which is why a denylist or a server-side session is needed for logout to mean anything.

<!-- lang:zh -->
### 整个安全模型，一句话

HTTP 没有记忆。每一个请求到来时都不带上下文，所以一个想记住"已登录用户"的站点，必须给浏览器一个东西让它在下次出示 —— 然后信任它。

> **会话标识符是一个持有者凭据：出示它的人，就被当成它所属于的那个人。**

这不是某个实现的缺陷，这就是设计本身。而它带来一个容易漏掉的后果：**会话 ID 必须满足一个密码满足的一切**，因为它在每一个请求上被用来代替密码 —— 另外还要多一条。密码被偷了可以改；会话 ID 必须**可撤销**。

所以有三个必需的性质，而第三个是大多数实现失败的那个：

| 性质 | 它的意思 | 怎么失效 |
|---|---|---|
| **不可预测** | 猜不出来、推不出来 | 用了非密码学设计的随机源 |
| **不可伪造** | 客户端构造不出来 | 签名密钥弱、泄露或根本没有 |
| **可撤销** | 服务端能让它失效 | 状态完全放在客户端 |

### 四层

1. **信任边界。** 服务端信任一个它早先发出去的字符串，而由一个持有它的人出示。它没有任何办法区分合法持有者和其他人 —— **而且它本来就不该能** —— 那就是持有者令牌的定义。
2. **数据与指令共用同一平面。** 这个标识符同时是**服务端状态的下标**（"找到这个会话"）和**身份的证明**（"这就是那个用户"）。一个值同时干两件事，意味着任何一次泄漏都变成完整沦陷而不是部分泄漏。
3. **为什么常见修法失败。** 给会话 cookie 签名、加密，让 cookie 防篡改，于是引出"所以会话就安全了"的结论。并不是：**签名不阻止任何人读到数据，也不阻止别人拿这个 cookie 去用。** 加密也一样。两者都只解决了"不可伪造"，另外两个性质原样留着。
4. **变体。** cookie 的作用域与 `__Host-` 前缀、`SameSite`、把标识符放在 URL 里而不是 cookie 里、以及同一个标识符被存在脚本能读到的地方。

### 状态可以放在哪

三种架构，失效模式各不相同：

| 状态在哪 | 可撤销 | 客户端能读 | 主要风险 |
|---|---|---|---|
| **服务端**，用一个 ID 去索引 | 能 | 不能 | 存储与多实例共享 |
| **签名的 cookie 里** | **不能** | **能 —— 签名不是加密** | 密钥弱、泄露或硬编码就送伪造 |
| **加密的 cookie 里** | 不能 | 不能 | 密钥管理；仍然不可撤销 |

中间那一行值得多想一想，因为它是现代最常见的选择，也是安全推理最常出错的那一个。

**签名意味着客户端改不了内容。** 在一个签名 cookie 上实测，把载荷替换掉、沿用原来的签名：

```
替换了载荷、沿用旧签名 -> 签名不匹配
```

**它不意味着客户端读不了。** 实测那个 cookie 的第一段解开来就是明文 JSON：

```
{"user": "alice", "role": "user"}
```

**它也不阻止别人拿这个 cookie 去用。** 一个签名 cookie 里没有任何东西把它绑定到某个特定的浏览器 —— 一旦它被复制出去。

实测的第三点才是让这个设计脆弱的地方：**整个"不可伪造"性质压在一个秘密上。**

```
用还原出来的密钥重新签名 -> 签名有效: {"user": "alice", "role": "admin"}
```

而那个秘密可以离线攻击，和 JWT 那两篇里完全一样，因为签名不需要联系服务端就能验证：

```
9 条候选 -> 找到 'secret'，耗时不到一毫秒，一个请求都没发
```

**真正要紧的对照是和第一行比。** 一个用 ID 索引的服务端会话可以被失效 —— 登出、改密码、管理员撤销访问，全都立刻生效。而状态在客户端时，**上面这些一个都够不到它**：cookie 会一直有效到过期，而"登出"删掉的是一份服务端从不知道的副本。这就是那笔交易，而它通常是在没人注意的情况下做掉的。

### 标识符可以被携带的三种方式

| 载体 | `HttpOnly` | `Secure` | `SameSite` | 说明 |
|---|---|---|---|---|
| **Cookie** | 能 | 能 | 能 | 唯一三种属性都适用的载体 |
| **URL 参数** | 不适用 | 不适用 | 不适用 | 会落进日志、历史与 `Referer` 头 |
| **`localStorage`** | 不适用 | 不适用 | 不适用 | 源内任何脚本都能读，所以任何 XSS 都能拿走它 |

`HttpOnly` 把标识符从 JavaScript 的触及范围里移出去，这把大多数 XSS 从"会话被偷"降级成"访问期间以用户身份执行动作" —— 对攻击者位置的一次严重降级。`Secure` 让它不出现在明文连接上。`SameSite` 限制跨站请求携带它。**另外两种载体拿不到这些中的任何一个**，所以一个出现在 URL 里或存储里的标识符本身就是一条发现，与别的地方有没有错无关。

### 在被人偷走之前就会出错的两种方式

**可预测的标识符。** 实测，用一个有种子、非密码学设计的随机源生成 ID：

```
seed=1234 -> e302123b7000bfe4, 2530f72e221829fb
seed=1234 -> e302123b7000bfe4, 2530f72e221829fb     每一次都是同一个序列
seed=4321 -> 81d25b3151f9d7cd, e5e034e786ad7f69
```

换成密码学级别的源，两次调用永不重复，也没有种子可以还原。规则很窄而且绝对：**标识符必须来自一个为"秘密"而设计的源。** 通用随机库按设计就是可复现的，而一个用了它的框架默认值，在任何人攻击之前就已经是一条发现。

**会话固定。** 如果用户登录时标识符不变，那么任何在登录之前就知道它的人，在登录之后就持有一个有效的已认证会话：

```
不轮换:  之前 sess-XYZ  ->  之后 sess-XYZ
轮换:    之前 sess-XYZ  ->  之后 sess-7f3a91c2
```

"变成已认证"是一次权限变化，而**每一次权限变化都需要一个新的标识符** —— 包括会话之内从一个角色变成另一个角色。这种攻击通常是这样投递的：在受害者登录之前把标识符种进去 —— 通过一条含有它的链接，或者从一个相关的子域设置那个 cookie。

### 检测与缓解

- **当会话标识符从与签发时不同的地址或 user agent 被使用时告警。** 这是最直接的沦陷信号，而它需要一个基线：移动网络上的用户合法地换地址，所以有用的规则是**国家或网络**发生变化、并伴随其他异常，而不是任何变化都报。
- **检查标识符是否在登录时轮换、以及是否是固定值。** 两者都能在部署前的测试里直接测：登录两次并确认标识符变了，再确认一批已签发的标识符里没有模式、没有计数器、没有时间戳。
- **对成批的签名校验失败告警。** 和 JWT 一样，一个会话 cookie 上爆发式的签名校验失败，说明有人在做密钥还原。这个信号只有在"校验失败"被与"没有会话"区分开来记录时才存在。
- **当标识符出现在 URL 里时告警。** 路径或查询参数里的会话 token 会到达访问日志、每一个外链的 `Referer` 头、以及浏览器历史。它出现在那里值得一条告警，因为它无法被"取消泄露"。
- **缓解上，优先服务端状态。** 它是唯一一种撤销能起作用的架构，而撤销正是"一个被偷的会话直到它过期"与"一个被偷的会话直到有人发现"之间的区别。
- **用平台提供的安全随机源，并让框架去生成标识符。** 不要自己造、不要截短、也不要用任何由用户信息推导出来的值。
- **在登录时、权限变化时、改密码时轮换。** 每一个都是"旧标识符的权限应当结束"的点 —— 而轮换同时让攻击者之前种下的东西失效。
- **把会话 cookie 留在只有浏览器能放的地方。** `HttpOnly`、`Secure`、`SameSite`、平台支持时加 `__Host-` 前缀，并且把 path 与 domain 收窄。一个作用域在父域的会话 cookie 会与每一个子域共享，包括维护得更差的那几个。
- **把签名的会话 cookie 当作一次 HMAC，并带上 JWT 的全部规则。** 密钥就是密钥：随机、够长、按环境区分、不在仓库里、定期轮换、绝不是源码文件里的一个字面量。**而且不要在载荷里放任何机密**，因为签名不会把它藏起来。
- **给会话同时设置空闲超时与绝对寿命。** 空闲超时限制一个被遗弃的会话；绝对寿命限制一个被刻意续命的会话。
- **并且让登出成为一次服务端行为。** 状态在客户端时，删掉 cookie 只是移除了用户手里那一份，而凭据对任何已经拿到它的人依然有效 —— 这就是为什么登出要有意义，就需要黑名单或者服务端会话。
