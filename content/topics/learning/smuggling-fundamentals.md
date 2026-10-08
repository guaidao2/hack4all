---
id: smuggling-fundamentals
title_en: "HTTP Request Smuggling, Part 1 — Two Parsers, One Boundary"
title_zh: "HTTP 请求走私（一）：两个解析器，一条边界"
summary_en: Two correct parsers disagree about where the first request ends, and the bytes after that point become the next request — one the front-end never inspected. Measured on a purpose-built front-end and back-end pair, with the four combinations and the queue desynchronisation that hands one user another user's response.
summary_zh: 两个各自都正确的解析器，对"第一个请求在哪里结束"意见不一致，而那个位置之后的字节就成了下一个请求 —— 一个前端从未检查过的请求。这一篇在一对专门搭出来的前端/后端上实测，给出四种组合，以及把别人的响应递给你的那种队列失步。
tags: [web, request-smuggling, cwe-444, http, parsing]
tools: [python3, curl, Burp Suite]
attck: [T1190, T1557]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The purest form of the pattern

Every entry in this series has the same underlying shape, and this is where it is most visible: **two components read the same bytes and disagree about what they mean.** WAF bypass is that shape applied to a filter's opinion of a payload; this is that shape applied to the question of **where a request ends**.

> **How many requests are in a byte stream depends on who is reading it.**

A modern deployment has at least two HTTP parsers in the path — a reverse proxy, load balancer or CDN in front, and the application server behind it. Each was written to a specification, each was implemented at a different time, and each has to decide where the first request stops and the second begins. When they decide differently, the bytes after the front-end's boundary are still in the stream, and the back-end reads them as **the next request on that connection**.

Three properties make this the most dangerous parsing disagreement in the web stack:

- **Both parsers are correct.** Neither is buggy; each is following its own rules for a well-formed request.
- **The smuggled request never reaches the front-end.** Whatever the proxy does — authentication, rate limiting, WAF rules, logging — applies to the first request. The second one was inside it.
- **The connection is shared.** The back-end is reading from a connection pool, so the "next request" it assembles is frequently **the next request from a different user**.

### Four layers

1. **The trust boundary.** The front-end trusts that the back-end will find the same request boundaries it did, and the back-end trusts the stream it is given. Neither verifies the other's parsing, because there is no mechanism that would.
2. **Data and instruction share a plane.** This is the purest instance in the guide: the bytes of a request **body** are simultaneously *data* and *the beginning of the next request*. Nothing marks the transition except a length, and the two sides compute that length differently.
3. **Why the usual fix fails.** A WAF, a rate limiter and an authentication layer all operate on **requests they can see**. A smuggled request is, by construction, one they cannot see — it arrives inside an earlier request's body. Adding inspection in front does not help, and can make the disagreement worse, because the front-end is now a third parser with its own opinion.
4. **The variants.** `CL.TE`, `TE.CL`, `TE.TE` and duplicate `Content-Length` headers, plus the obfuscations used to produce the third case: header case, a space before the colon, and values such as `xchunked` that one parser accepts and another ignores.

### Two ways to say how long a body is

Both are legitimate, and they coexist for historical reasons.

**`Content-Length`** is a number: the body is exactly that many bytes. It is simple, and it is the reason a request with no `Content-Length` and no `Transfer-Encoding` has an empty body.

**`Transfer-Encoding: chunked`** is a stream: the body arrives as chunks, each prefixed by its length in hexadecimal, and a chunk of size zero ends it. It exists so a sender can start transmitting before knowing the total size.

When both are present the specification resolves it — `Transfer-Encoding` takes precedence, and `Content-Length` should be ignored (or the request rejected). **That is what the specification says; what a given deployment does is a different question**, and the difference between those two sentences is the vulnerability. A proxy that predates the rule, or that was configured to normalise one way while the back-end normalises another, produces a disagreement without anyone making a mistake.

### The four combinations, measured

Measured on a purpose-built pair: a front-end that trusts `Content-Length`, forwarding bytes verbatim to a back-end that trusts `Transfer-Encoding`.

A request whose body is a chunked terminator followed by an entire second request:

```
POST / HTTP/1.1
Host: x
Content-Length: 53
Transfer-Encoding: chunked

0

GET /smuggled-admin-action HTTP/1.1
Host: x
```

| Where it was observed | What happened |
|---|---|
| **Front-end** | Read all 53 bytes as the body of one request. Saw one request. |
| **Back-end** | Followed `chunked`, consumed `0\r\n\r\n` and stopped — then read the remaining 48 bytes as **a new request** |
| **Requests the back-end processed** | `POST /`, and then `GET /smuggled-admin-action` |
| **Response the client received** | The response to `POST /` only — the second response stayed in the connection |

The last row is the important one. The second response was produced for a request the front-end never made, and it is now queued on a back-end connection that the next user's request will also use. **That is the response queue desynchronisation that turns smuggling into cross-user data disclosure**: the next request onto that connection is answered with the response to the smuggled one.

The same byte stream against a front-end that trusts `Transfer-Encoding` and a back-end that trusts `Content-Length` — `TE.CL` — produced the same outcome in the other direction, which is the point: **the payload does not decide the attack, the pair of trust decisions does.**

| Case | Front-end obeys | Back-end obeys | How it is produced |
|---|---|---|---|
| **CL.TE** | `Content-Length` | `Transfer-Encoding` | The measured case above |
| **TE.CL** | `Transfer-Encoding` | `Content-Length` | The same shape, opposite roles |
| **TE.TE** | `Transfer-Encoding` | `Transfer-Encoding` | **Both obey it, but one fails to recognise the header** — obfuscated values such as `xchunked`, `chunked\t`, or a duplicated header |
| **CL.CL** | `Content-Length` | `Content-Length` | Two different `Content-Length` values, resolved differently |

**TE.TE is worth dwelling on**, because it is the case where neither side is doing anything unusual. Both implement chunked encoding; one of them simply does not parse the *header value* the way the other does, so it falls back to `Content-Length` — or to treating the body as empty. The obfuscations exploit exactly that gap between "recognises the token" and "recognises the header".

### Why the consequence is large

**Front-end controls are bypassed.** Authentication, rate limiting, WAF rules and IP allowlists are all applied at the edge. A smuggled request is processed by the back-end having passed through none of them, because the front-end never saw it as a separate request.

**Other users' responses are reachable.** The measured queue behaviour is the mechanism: if a smuggled request's response remains on a connection, the next request to reuse that connection receives it. Against a target where responses contain personal data, session tokens or one-time codes, that is disclosure without any access to the victim's credentials.

**Requests can be hijacked.** Instead of leaving a complete rogue request in the stream, the attacker can leave a partial one — headers without a terminating line — so that the next user's request line is appended to it. The resulting request is a mix of the attacker's headers and the victim's, and what it does depends entirely on the application.

**And caches can be poisoned**, because a smuggled request may be one a cache will store under a key other users request. That is the subject of the next entry in this series.

### Detection and mitigation

- **Alert whenever `Transfer-Encoding` and `Content-Length` appear in the same request.** This is the single most direct detection, and it is also the recommended edge policy: **reject the request**. A legitimate client has no reason to send both, so the rule has no false positives worth worrying about.
- **Alert on obfuscated transfer-encoding values.** `xchunked`, `chunked `, `Transfer-Encoding: chunked, chunked`, two `Transfer-Encoding` headers, a header name with a space before the colon, and unusual casing. Each is an attempt to be recognised by one parser and ignored by another, and no normal client produces any of them.
- **Compare the front-end's request log with the back-end's.** The front-end recorded one request; the back-end recorded two. Nothing else in the stack surfaces that difference, and it is the only direct evidence of a smuggled request. This is the detection that has to be built deliberately, because the two logs live in different systems.
- **And treat user reports of seeing another account's page as an incident, not a bug report.** That symptom — one user receiving another's response — is the outcome of queue desynchronisation, and it means smuggling has been happening, possibly for a long time.
- **For mitigation, make both ends use the same boundary rules.** The cleanest version is to use one HTTP implementation end to end, or to speak HTTP/2 all the way through: **framing is explicit in HTTP/2 and there is no `Content-Length`-versus-`Transfer-Encoding` ambiguity to exploit.**
- **Where that is not possible, normalise at the edge and fail closed.** Reject any request carrying both headers; strip `Content-Length` when `Transfer-Encoding` is present (or the reverse, consistently); allow exactly one `Transfer-Encoding` value and only `chunked`. The policy has to be applied by the first parser in the path, because that is the one whose opinion the back-end must agree with.
- **Upgrade the parsers rather than trusting a version number.** These disagreements are implementation-specific and are fixed over time, so a deployment is only as consistent as its oldest component — which is usually the one nobody thinks about.
- **Consider disabling back-end connection reuse where the risk justifies it.** Sending each request on a fresh connection, or buffering the whole request at the edge before forwarding, removes the shared-connection precondition. Both cost performance, which is why they are a fallback rather than a first choice.
- **And do not rely on the front-end's own inspection to catch it.** By construction, the smuggled request is the one the front-end did not inspect — the control has to be about **framing**, not about content.

<!-- lang:zh -->
### 这个模式最纯粹的形式

本系列的每一篇都有同一个底层形状，而在这里它最看得见：**两个组件读同一串字节，对"它们是什么意思"意见不一致。** WAF 绕过是那个形状作用在"过滤器怎么看一个 payload"上；这里是那个形状作用在**"一个请求在哪里结束"**上。

> **一串字节里有几个请求，取决于谁在读它。**

一个现代部署的路径上至少有两个 HTTP 解析器 —— 前面是反向代理、负载均衡或 CDN，后面是应用服务器。每一个都是按某份规范写的，每一个的实现时间不同，而每一个都必须判断第一个请求在哪里停下、第二个从哪里开始。当它们的判断不同时，前端边界之后的那些字节仍然留在流里，而后端把它们读成**这条连接上的下一个请求**。

有三个性质让这件事成为整个 Web 栈里最危险的解析分歧：

- **两个解析器都是对的。** 谁都没有 bug；各自都是在对一个良构请求执行自己的规则。
- **走私的那个请求从不到达前端。** 代理做的一切 —— 认证、限流、WAF 规则、日志 —— 都作用在第一个请求上。第二个就在它里面。
- **连接是共享的。** 后端从连接池里读，所以它拼出来的那个"下一个请求"，经常是**另一个用户的下一个请求**。

### 四层

1. **信任边界。** 前端信任后端会找到与它相同的请求边界，后端信任交给它的那串流。谁也不去验证对方的解析，因为没有任何机制能做这件事。
2. **数据与指令共用同一平面。** 这是本指南里最纯粹的实例：一个请求**体**的字节同时是*数据*和*下一个请求的开头*。除了一个长度之外，没有任何东西标记这个转换，而两边算这个长度的方式不同。
3. **为什么常见修法失败。** WAF、限流器和认证层全都作用于**它们看得见的请求**。一个走私请求按构造就是它们看不见的那个 —— 它藏在更早那个请求的请求体里。在前面加检查帮不上忙，还可能让分歧更糟，因为前端现在成了第三个有自己的意见的解析器。
4. **变体。** `CL.TE`、`TE.CL`、`TE.TE` 以及重复的 `Content-Length`，再加上用来制造第三种情况的那些混淆：头名大小写、冒号前的空格、以及 `xchunked` 这类一个解析器接受、另一个忽略的值。

### 两种说明请求体有多长的方式

两者都正当，而且由于历史原因共存。

**`Content-Length`** 是一个数字：请求体正好是那么多字节。它简单，而这也是"既没有 `Content-Length` 也没有 `Transfer-Encoding` 的请求其请求体为空"的原因。

**`Transfer-Encoding: chunked`** 是一段流：请求体分块到达，每块前面是它十六进制的长度，而长度为 0 的块结束它。它存在的意义是让发送方在不知道总长时就能开始传输。

两者同时出现时，规范给出了裁决 —— `Transfer-Encoding` 优先，`Content-Length` 应当被忽略（或者请求被拒绝）。**那是规范说的；某个具体部署怎么做是另一个问题**，而这两句话之间的差别就是漏洞。一个早于该规则的代理，或者一个被配置成按某种方式规范化、而后端按另一种方式规范化的代理，会在没有任何人犯错的前提下产生分歧。

### 四种组合，实测

在一对专门搭出来的组件上实测：前端信任 `Content-Length`，把字节原样转发给一个信任 `Transfer-Encoding` 的后端。

一个请求体是"chunked 结束标记"加上一整个第二个请求的请求：

```
POST / HTTP/1.1
Host: x
Content-Length: 53
Transfer-Encoding: chunked

0

GET /smuggled-admin-action HTTP/1.1
Host: x
```

| 在哪里观察到的 | 发生了什么 |
|---|---|
| **前端** | 把全部 53 字节读成一个请求的请求体。看到了一个请求。 |
| **后端** | 按 `chunked` 走，消耗掉 `0\r\n\r\n` 就停了 —— 然后把剩下的 48 字节读成**一个新请求** |
| **后端处理的请求** | `POST /`，然后是 `GET /smuggled-admin-action` |
| **客户端收到的响应** | 只有 `POST /` 的响应 —— 第二个响应留在了连接里 |

最后一行是要紧的那一行。第二个响应是为一个"前端从未发出过"的请求产生的，而它现在排在某条后端连接的队列里，下一个用户的请求也会用那条连接。**这就是把请求走私变成跨用户数据泄漏的那种响应队列失步**：下一个用这条连接的请求，会收到那个走私请求的响应。

同一个字节流，换成信任 `Transfer-Encoding` 的前端与信任 `Content-Length` 的后端 —— `TE.CL` —— 在另一个方向上产生同样的结果，而重点正在这里：**决定攻击的不是 payload，是那一对信任决定。**

| 组合 | 前端遵守 | 后端遵守 | 怎么造出来 |
|---|---|---|---|
| **CL.TE** | `Content-Length` | `Transfer-Encoding` | 上面实测的那种 |
| **TE.CL** | `Transfer-Encoding` | `Content-Length` | 同一个形状，角色对调 |
| **TE.TE** | `Transfer-Encoding` | `Transfer-Encoding` | **两者都遵守它，但其中一个认不出这个头** —— `xchunked`、`chunked\t`、或者重复的头 |
| **CL.CL** | `Content-Length` | `Content-Length` | 两个不同的 `Content-Length` 值，被不同地裁决 |

**TE.TE 值得多想一想**，因为它是"两边都没做任何异常事"的那种情况。两者都实现了 chunked 编码；只是其中一个对那个*头的值*的解析方式与另一个不同，于是它退回 `Content-Length` —— 或者干脆把请求体当成空。那些混淆手法利用的正是"认得这个 token"与"认得这个头"之间的缝。

### 为什么后果很大

**前端的一切控制都被绕过。** 认证、限流、WAF 规则、IP 白名单都在边缘生效。一个走私请求由后端处理，而它没有经过其中任何一道，因为前端从未把它看成一个独立的请求。

**其他用户的响应变得可达。** 实测里那种队列行为就是机制：如果一个走私请求的响应留在连接上，下一个复用那条连接的请求就会收到它。而面对一个响应里含个人数据、会话 token 或一次性验证码的目标，那就是一次无需接触受害者凭据的泄漏。

**请求可以被劫持。** 攻击者不必在流里留下一个完整的恶意请求，可以留下一个**不完整的** —— 有头、没有结束行 —— 于是下一个用户的请求行会被接到它后面。拼出来的请求是攻击者的头与受害者的请求行的混合体，而它做什么完全取决于那个应用。

**而且缓存可以被投毒**，因为一个走私请求可能正是缓存会以别人请求的键存下来的那种请求。那是本系列下一篇的主题。

### 检测与缓解

- **只要 `Transfer-Encoding` 与 `Content-Length` 出现在同一个请求里就告警。** 这是最直接的单一检测，同时也是推荐给边缘的策略：**拒绝这个请求**。正当客户端没有理由同时发这两个，所以这条规则没有值得在意的误报。
- **对混淆过的 transfer-encoding 值告警。** `xchunked`、`chunked `、`Transfer-Encoding: chunked, chunked`、两个 `Transfer-Encoding` 头、冒号前带空格的头名、以及不寻常的大小写。每一个都是"想被一个解析器认出、被另一个忽略"的尝试，而没有正常客户端会产出其中任何一个。
- **把前端的请求日志与后端的并排比对。** 前端记录了一个请求；后端记录了两个。栈里没有别的东西会把这个差别显现出来，而它是走私请求唯一的直接证据。这条检测必须刻意去建，因为两份日志在不同的系统里。
- **并且把"用户报告看到了别人的页面"当作事件处理，而不是当 bug 报告。** 那个症状 —— 一个用户收到另一个人的响应 —— 是队列失步的结果，而它意味着走私已经在发生，可能已经很久了。
- **缓解上，让两端使用同一套边界规则。** 最干净的做法是端到端使用同一个 HTTP 实现，或者全程说 HTTP/2：**HTTP/2 的分帧是显式的，不存在 `Content-Length` 与 `Transfer-Encoding` 之间的那种歧义可供利用。**
- **做不到的时候，在边缘规范化并失败关闭。** 拒绝任何同时携带两个头的请求；在有 `Transfer-Encoding` 时剥掉 `Content-Length`（或者反过来，但要一致）；只允许一个 `Transfer-Encoding` 值、且只能是 `chunked`。这条策略必须由路径上第一个解析器执行，因为后端的意见必须与它一致。
- **升级解析器，而不是相信版本号。** 这些分歧与具体实现相关、会随时间被修掉，所以一个部署的一致性只取决于它最老的那个组件 —— 而那通常是没人会想到的那一个。
- **在风险足以支撑时，考虑禁用后端连接复用。** 每个请求走一条新连接、或者在边缘把整个请求缓冲完再转发，都移除了"连接共享"这个前提。两者都要付性能代价，所以它们是退路而不是首选。
- **并且不要指望靠前端自己的检查来抓它。** 按构造，走私请求就是前端没有检查的那一个 —— 这项控制必须关于**分帧**，而不是关于内容。
