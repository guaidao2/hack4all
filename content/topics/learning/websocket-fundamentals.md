---
id: websocket-fundamentals
title_en: "WebSocket Security — A Long-Lived Channel With One Authentication"
title_zh: "WebSocket 安全：认证一次的长命通道"
summary_en: The handshake is HTTP and everything after it is not, and a browser will not let a page add a custom header to it — so a cookie is attached automatically while a CSRF token has nowhere to go. Measured with a hand-written handshake and an origin check.
summary_zh: 握手是 HTTP，之后的一切都不是；而浏览器不允许页面给它加自定义头 —— 所以 cookie 会被自动带上，而 CSRF token 无处可放。这一篇用一次手写的握手和一组 Origin 检查实测。
tags: [web, websocket, cswsh, cwe-1385, origin]
tools: [python3, curl, browser devtools]
attck: [T1190, T1550]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The upgrade is HTTP; the rest is not

A WebSocket connection begins as an HTTP request carrying `Upgrade: websocket`. Once the server answers `101 Switching Protocols`, the socket stops being request/response and becomes a **persistent bidirectional channel** where either side may send at any moment.

That change of protocol is where the security model quietly diverges. Everything an HTTP framework provides — routing, middleware, per-request authorisation — was written for a request that arrives, is handled, and ends. A WebSocket connection arrives once and then **keeps arriving** for hours.

> **Many implementations authenticate at the upgrade and check nothing afterwards.**

### What a browser puts in the handshake

Measured with a hand-written handshake, capturing exactly what the server receives:

```
GET /ws HTTP/1.1
Origin: https://app.example      <- set by the browser
Cookie: session=alice            <- set by the browser
Upgrade: websocket
Sec-WebSocket-Key: ...
Sec-WebSocket-Version: 13
```

Two fields are set by the browser, and **one field is not available at all**: a page's WebSocket API does not let script add arbitrary request headers. That single limitation determines the whole defence surface, because it means:

| Mechanism | Works over WebSocket |
|---|---|
| Cookie | **yes** — attached automatically |
| Custom header (where a CSRF token usually goes) | **no** — the API will not let the page set one |
| `Origin` | **yes** — attached automatically, and readable by the server |
| A value inside the URL | yes, but it lands in logs and history |
| `Sec-WebSocket-Protocol` | yes — a page **can** name a subprotocol |

### The comparison that makes it concrete

Against the CSRF entry's situation:

| | HTTP request | WebSocket connection |
|---|---|---|
| Same-origin policy applies | yes, restricting reads | **not in the same way — cross-origin connections are allowed by default** |
| Credentials attached automatically | yes | **yes** |
| A token can be attached by the page | yes, in a header or body | **no custom header available** |
| Authorisation checked | per request | **once, at the upgrade** |

**Cookies are the same and tokens are not**, and that asymmetry is the whole class:

> **CSWSH is CSRF with a narrower defence surface** — the one mechanism that normally proves intent is unavailable, so the check has to live in the one field the browser sets and the server can read.

And there is a second difference that makes it worse: **a CSRF attack is blind** — the attacker's page causes a request and cannot read the response — while a cross-site WebSocket is **not blind**, because the attacker's script receives every frame the server sends back.

### Measured: what an origin check buys

Two servers, identical except that one inspects `Origin`:

| `Origin` sent | No check | With check |
|---|---|---|
| `https://app.example` | `101 Switching Protocols` | `101 Switching Protocols` |
| **`https://evil.example`** | **`101 Switching Protocols`** | **`403 Forbidden`** |
| *(absent)* | `101 Switching Protocols` | **`403 Forbidden`** |

**Without the check, a connection from `evil.example` is indistinguishable from a legitimate one except for the header the server is not reading.** That is the entire finding, and it is one comparison to fix.

**And absence must be refused.** A browser always sends `Origin` on a WebSocket handshake; a non-browser client can omit it. So a rule of "allow when absent" hands the check to a field any script can forge — and a client that omits it is not subject to the same-origin policy and has no user cookies to abuse, so it needs no exemption.

### What CSWSH actually achieves

The five steps, none of which involves a vulnerability in the WebSocket implementation:

1. The attacker's page runs `new WebSocket("wss://target/ws")`.
2. The browser attaches the target domain's cookies, because this is a request to that domain.
3. The browser sets `Origin: https://evil.example` — and this is the only signal the server has.
4. A server that does not read it cannot distinguish this from a legitimate client.
5. The attacker's script now **sends and receives** on that channel as the victim.

Step 5 is what separates this from CSRF: the channel is bidirectional and the attacker is on it. Whatever the protocol does — read messages, subscribe to a feed, trigger actions, alter state — happens under the victim's identity, with the results visible to the attacker.

### Four layers

1. **The trust boundary.** The application trusts that a connection which authenticated is a connection that is authorised. The connection outlives the moment of authentication, and the user's permissions do not.
2. **Data and instruction share a plane.** Every frame is both **data** and **a command to act on**. The framework's routing and middleware ran during the upgrade and are no longer in the path, so per-message decisions are the application's own to make.
3. **Why the usual fix fails.** "We authenticate the connection" is true and answers the wrong question — authorisation has to be re-asked per message, because the answer can change while the socket stays open. And "we use a CSRF token" does not carry over at all, for a reason that is an API limitation rather than a configuration.
4. **The variants.** CSWSH; rights revoked while a connection is open; per-object authorisation missing on messages (the BOLA entry's rule on a channel that never "re-requests"); no rate limit because there is no request to count; and `permessage-deflate`, where compression can amplify a small message.

### Detection and mitigation

- **Alert when the `Origin` on a WebSocket handshake is not an origin the deployment serves.** The expected values are known at deploy time and the comparison is exact — the same rule as the CORS and host header entries, in the one place where it is the only available control.
- **Alert on handshakes with no `Origin` at all.** A browser always sends one, so its absence means a non-browser client — which is not an attack by itself, but is a deviation worth knowing about for an endpoint that expects browsers.
- **Alert when a connection is established with no corresponding page load in the session.** This is the same shape as the clickjacking entry's check: the legitimate path to opening a socket includes loading the page that opens it.
- **And watch message rates and concurrent connections per account.** A persistent channel has no "requests" to count, so volume has to be measured per message and per socket — and a single account holding many sockets is a shape that has no benign explanation.
- **For mitigation, check `Origin` and refuse a mismatch or an absence.** It is the only field that both the browser sets and the attacker cannot forge, and in this protocol it is not one control among several — it is the control.
- **Where a token is genuinely needed, carry it in the subprotocol name.** `Sec-WebSocket-Protocol` is the one field a page can set, so a scheme that puts a short-lived value there is the available workaround — with the caveat that it is visible in logs, so it should be short-lived and single-use.
- **Re-check authorisation per message, not per connection.** The BOLA entry's rule applies exactly: every message that names an object should ask whether the caller may act on **that** object. A channel that authenticated once and then trusts every frame is the natural place for that check to be missing.
- **Rate-limit by message, and cap connections per account.** Neither is available from the HTTP layer's usual counters, because from that layer the connection looked like a single request.
- **Use `wss://`, and treat the message body as input.** Encryption of the channel and validation of the contents are the same two requirements as for HTTP, and neither is affected by the protocol being persistent.
- **And treat a WebSocket endpoint as a session rather than a sequence of requests.** That reframing is what makes the rest of this entry's advice obvious: sessions have lifetimes, permissions change during them, actions within them need authorisation, and the volume of activity in them is something to bound.

<!-- lang:zh -->
### 升级是 HTTP，其余不是

一条 WebSocket 连接以一个带 `Upgrade: websocket` 的 HTTP 请求开始。一旦服务器回了 `101 Switching Protocols`，这个套接字就不再是请求/响应，而变成一条**持久的双向通道**，两边都可以在任何时刻发送。

协议的这个变化，正是安全模型悄悄分岔的地方。HTTP 框架提供的一切 —— 路由、中间件、逐请求的授权 —— 都是为一个"到来、被处理、结束"的请求写的。而一条 WebSocket 连接到来一次，然后**持续到来**几个小时。

> **很多实现在升级那一刻做一次认证，之后再什么都不检查。**

### 浏览器在握手时放了什么进去

用一次手写的握手实测，抓下服务端确切收到的东西：

```
GET /ws HTTP/1.1
Origin: https://app.example      <- 浏览器设置
Cookie: session=alice            <- 浏览器设置
Upgrade: websocket
Sec-WebSocket-Key: ...
Sec-WebSocket-Version: 13
```

有两个字段是浏览器设置的，而**有一个字段根本拿不到**：页面的 WebSocket API 不允许脚本添加任意请求头。仅仅这一条限制就决定了整个防御面，因为它意味着：

| 机制 | 在 WebSocket 上可用吗 |
|---|---|
| Cookie | **可用** —— 自动带上 |
| 自定义头（CSRF token 通常放的地方） | **不可用** —— API 不让页面设置 |
| `Origin` | **可用** —— 自动带上，且服务端可读 |
| URL 里的一个值 | 可用，但它会落进日志与历史 |
| `Sec-WebSocket-Protocol` | 可用 —— 页面**可以**指定一个子协议 |

### 那个把它说清楚的对照

对着 CSRF 那篇的处境：

| | HTTP 请求 | WebSocket 连接 |
|---|---|---|
| 同源策略适用 | 是，限制读取 | **不是同样的方式 —— 跨源连接默认被允许** |
| 凭据自动带上 | 是 | **是** |
| 页面可以附上一个 token | 是，放头或体里 | **没有自定义头可用** |
| 授权被检查 | 每个请求 | **一次，在升级时** |

**cookie 一样，token 不一样**，而那个不对称就是整一类：

> **CSWSH 是防御面更窄的 CSRF** —— 通常用来证明意图的那个机制不可用，所以检查只能落在"浏览器会设置、而服务端能读"的那一个字段上。

还有一个差别让它更糟：**CSRF 攻击是盲的** —— 攻击者的页面造成一个请求，却读不到响应 —— 而跨站 WebSocket **不盲**，因为攻击者的脚本收得到服务端回发的每一帧。

### 实测：Origin 检查买到了什么

两个服务器，除了其中一个会检查 `Origin` 之外完全一样：

| 发来的 `Origin` | 不检查 | 检查 |
|---|---|---|
| `https://app.example` | `101 Switching Protocols` | `101 Switching Protocols` |
| **`https://evil.example`** | **`101 Switching Protocols`** | **`403 Forbidden`** |
| *（缺失）* | `101 Switching Protocols` | **`403 Forbidden`** |

**没有那道检查时，一个来自 `evil.example` 的连接，除了那个服务端不读的头之外，与合法连接无法区分。** 那就是整条发现，而修法是两次比较。

**而"缺失"也必须拒绝。** 浏览器在 WebSocket 握手时一定会发 `Origin`；非浏览器客户端可以不发。所以"缺失就放行"的规则，等于把检查交给一个任何脚本都能伪造的字段 —— 而一个不发它的客户端不受同源策略约束、也没有用户的 cookie 可以滥用，所以它不需要任何豁免。

### CSWSH 实际达成了什么

五步，其中没有一步是 WebSocket 实现里的漏洞：

1. 攻击者的页面执行 `new WebSocket("wss://target/ws")`。
2. 浏览器附上目标域的 cookie，因为这是发给那个域的请求。
3. 浏览器设置 `Origin: https://evil.example` —— 而这是服务端唯一拥有的信号。
4. 一个不读它的服务端，无法把它与合法客户端区分开。
5. 攻击者的脚本现在以受害者的身份在那条通道上**收发消息**。

第 5 步才是把它与 CSRF 分开的地方：通道是双向的，而攻击者就在上面。协议做什么 —— 读消息、订阅数据流、触发动作、改动状态 —— 都以受害者的身份发生，而结果对攻击者可见。

### 四层

1. **信任边界。** 应用信任"一条通过认证的连接就是一条被授权的连接"。连接比认证的那一刻活得更久，而用户的权限不会。
2. **数据与指令共用同一平面。** 每一帧既是**数据**又是**一条要执行的动作**。框架的路由与中间件在升级期间跑过，之后就不再处于路径上，所以逐消息的决定是应用自己的事。
3. **为什么常见修法失败。** "我们对连接做了认证"是真的，而且回答的是错的问题 —— 授权必须逐消息重新问，因为答案可以在套接字开着的时候改变。而"我们用了 CSRF token"完全无法搬过来，原因是 API 限制而不是配置问题。
4. **变体。** CSWSH；连接开着时权限被收回；消息上缺少逐对象授权（BOLA 那篇的规则，落在一条从不"重新请求"的通道上）；因为没有请求可数所以没有速率限制；以及 `permessage-deflate`，那里的压缩可以放大一条很小的消息。

### 检测与缓解

- **当 WebSocket 握手上的 `Origin` 不是这个部署服务的源时告警。** 期望值在部署时就知道、比较是精确的 —— 和 CORS、Host 头那两篇同一条规则，落在那唯一一处它是唯一可用控制的地方。
- **对完全没有 `Origin` 的握手告警。** 浏览器一定会发，所以它缺失意味着一个非浏览器客户端 —— 那本身不是攻击，但对一个预期只有浏览器的端点来说，是一个值得知道的偏离。
- **当一条连接被建立、而会话里没有对应的页面加载时告警。** 这和点击劫持那篇的检查形状相同：打开一个套接字的正当路径包含加载打开它的那个页面。
- **并且盯消息速率与每账号的并发连接数。** 持久通道没有"请求"可数，所以量级必须按消息和按套接字去量 —— 而一个账号握着很多套接字，是一个没有良性解释的形状。
- **缓解上，检查 `Origin`，不匹配或缺失都拒绝。** 它是唯一一个"浏览器会设置、而攻击者伪造不了"的字段，而在这个协议里它不是若干控制中的一项 —— 它就是那项控制。
- **确实需要 token 的地方，把它放在子协议名字里。** `Sec-WebSocket-Protocol` 是页面唯一能设置的字段，所以一个把短期值放在那里的方案是现有的变通 —— 附带一句：它会出现在日志里，所以它应当是短命且一次性的。
- **按消息重新检查授权，而不是按连接。** BOLA 那篇的规则在这里完全适用：每一条指名了某个对象的消息，都该问一句"调用者能不能对**那个**对象行动"。一条认证过一次、然后信任每一帧的通道，正是那道检查最容易缺失的地方。
- **按消息限流，并给每账号的连接数设上限。** 两者都拿不到 HTTP 层常用的那些计数器，因为在那一层看，这条连接只是一个请求。
- **用 `wss://`，并把消息体当作输入。** 通道加密与内容校验，和 HTTP 是同两项要求，而它们都不受"协议是持久的"影响。
- **并且把 WebSocket 端点当成一个会话，而不是一串请求。** 正是这个重新表述，让这一篇其余的建议变得显然：会话有寿命、权限会在其间改变、会话里的动作需要授权、而其中的活动量是需要被约束的。
