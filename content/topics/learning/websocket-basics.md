---
id: websocket-basics
title_en: WebSocket Basics
title_zh: WebSocket 入门
summary_en: HTTP is a question and an answer, and a chat message, a price tick or a collaborator's keystroke all need the other direction. Measured at the byte level — the handshake as plain HTTP, the frame layout, masking, fragmentation, and what a header costs compared with a request.
summary_zh: HTTP 是问与答，而一条聊天消息、一次行情跳动、别人的一次按键，需要的是另一个方向。这一篇上升到字节级别 —— 握手就是普通 HTTP、帧的布局、mask、分片，以及一次帧头相对一次请求头到底省了多少。
tags: [beginner, web, websocket, http, protocol, browser]
tools: [curl, python3, browser devtools, wireshark]
attck: [T1071.001]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A second protocol, on the same port

An HTTP exchange has a fixed shape: the client asks, the server answers, the exchange is over. Almost everything on the web fits that shape, and the parts that do not are the ones this entry is about.

> **HTTP is a question and an answer. A chat message, a price tick and a collaborator's keystroke all need the other direction**: the server knows something first, and the client has to know it immediately.

WebSocket is the standard answer. It is worth understanding at the byte level for three reasons: it starts as HTTP and then stops being HTTP, its framing is simple enough to read without a library, and most of what confuses people about it — and most of what breaks in production — comes from the mechanics rather than from the API.

### Part 1: the shape of HTTP, and what it cannot do

Four properties of HTTP define the problem:

**It is client-initiated.** A server cannot decide to send something to a browser. There is no address for "this browser, right now" — the server holds a socket open, but the protocol has no message that means "here is something you did not ask for".

**It is a request and a response.** Even with `keep-alive`, where one TCP connection carries many exchanges, each exchange is still a question followed by an answer.

**It is stateless.** Nothing about one request is remembered by the protocol; whatever continuity exists — a session, a cart — is carried in cookies or tokens by the application.

**Headers are repeated every time.** A request carries the same host, user agent, accept list and cookies it carried a moment ago.

And a large class of applications needs the opposite:

| What the user expects | Who knows it first |
|---|---|
| A chat message appears | the other user's server |
| A price moves | the exchange |
| Someone else edits the document | the other editor |
| A job finishes, a build fails, an order ships | the backend |
| A game opponent moves | the other client |

The historical workarounds, and what each costs:

| Approach | Direction | The cost |
|---|---|---|
| **Short polling** — ask every N seconds | client asks | latency is the interval, and almost every request is empty |
| **Long polling** — ask and let the server hold the request open | client asks, server holds | one connection and one worker held per client; every answer needs a new request |
| **Server-Sent Events** | **server to client only** | one direction only, and it is still HTTP: text-only, over a connection that can be closed by anything in the middle |
| **WebSocket** | **both directions** | a protocol change, which some intermediaries do not support |

**WebSocket is not HTTP/2.** They are usually listed together as "modern", and they solve different problems. HTTP/2 multiplexes many request/response exchanges over one connection, so fetching a hundred assets stops needing a hundred connections. It does not give the server a way to start a conversation. A page that needs both uses both.

### Part 2: the handshake is an HTTP request

A WebSocket connection begins as an ordinary HTTP request with one unusual header. Measured, the request a client sends is:

```
GET /ws HTTP/1.1
Host: 127.0.0.1:9101
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: 8oXB0PvCyJ2ScYG1es++qQ==
Sec-WebSocket-Version: 13
```

**155 bytes**, and every line of it is ordinary HTTP. The server answers:

```
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: akEhchw8/+pJOtPLxAGhIFOnpC8=
```

**129 bytes**, and `101 Switching Protocols` is the whole point: **the same TCP connection, on the same port, stops speaking HTTP from this response onward.**

The headers, one at a time:

| Header | What it does |
|---|---|
| `Upgrade: websocket` | asks for the protocol change |
| `Connection: Upgrade` | the hop-by-hop signal that a change is being negotiated |
| `Sec-WebSocket-Key` | **16 random bytes, base64-encoded** — a nonce, not a credential |
| `Sec-WebSocket-Version: 13` | the only version in use; anything else is refused |
| `Sec-WebSocket-Accept` | the server's response to the nonce |

**And the nonce is not a security mechanism.** It is computed as:

```
Sec-WebSocket-Accept = base64(sha1(Sec-WebSocket-Key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
```

Measured, that computation reproduces the header value exactly:

```
key         : 8oXB0PvCyJ2ScYG1es++qQ==
computed    : akEhchw8/+pJOtPLxAGhIFOnpC8=
in response : akEhchw8/+pJOtPLxAGhIFOnpC8=   (identical)
```

**Nothing secret is involved** — the GUID is a published constant and the key is sent in the clear. The purpose is to make the response **impossible to produce by accident**: a proxy that does not understand `Upgrade` sees a request with unknown headers and a `101` status, and if it were to cache or replay that response, clients would reject it because the accept value would not match. This is the same motivation as client-side masking in Part 3 — **the protocol defends against intermediaries that misunderstand it**.

Optional headers worth knowing:

| Header | Purpose |
|---|---|
| `Origin` | set by the browser, and the only field a server can use to tell which page opened the connection |
| `Sec-WebSocket-Protocol` | a subprotocol name, negotiated here — and **the one field a browser page can set** |
| `Sec-WebSocket-Extensions` | what the client supports, such as compression |
| `Cookie` | attached by the browser like any other request to that origin |

**The handshake being HTTP is why the rules of that layer still apply at this moment** — the origin is present, the cookies are attached, and a server can refuse the upgrade for any reason an HTTP endpoint can. What changes is that after the `101`, none of the HTTP machinery is in the path any more.

### Part 3: frames, the smallest unit

After the upgrade, both sides exchange **frames**. A client frame is at most fourteen bytes of header plus the payload:

```
client sends: b"hello websocket"     (15 bytes of payload, 21 bytes on the wire)

  81 8f                <- 2 header bytes: FIN+opcode / MASK+length
  d7 76 23 c7          <- 4-byte masking key, random
  bf 13 4f ab b8 ...   <- 15 bytes of payload, XORed with the key
```

The two header bytes, bit by bit:

| Bits | Field | Meaning here |
|---|---|---|
| 1 | `FIN` | `1` = this is the last frame of the message |
| 3 | `RSV1-3` | reserved for extensions; `RSV1` is used by compression |
| 4 | `opcode` | `0x1` = text |
| 1 | `MASK` | `1` = payload is masked |
| 7 | payload length | `0x0f` = 15, or a marker for a longer length |

Opcodes:

| Opcode | Name | Carries |
|---|---|---|
| `0x0` | continuation | the rest of a message started by a previous frame |
| `0x1` | text | UTF-8 text |
| `0x2` | binary | arbitrary bytes |
| `0x8` | close | a close handshake, with a status code |
| `0x9` | ping | a keepalive probe |
| `0xa` | pong | the reply to a ping |

**Payload length has three encodings**, which is a detail that matters when parsing: up to 125 is stored in the 7 length bits; `126` means the real length follows in the next 2 bytes; `127` means it follows in the next 8. So a 6-byte payload costs a 6-byte header on the wire, and a 60-byte payload costs the same 6 bytes — the header only grows past 125.

**And the masking rule is asymmetric:**

> **A client must mask every frame it sends. A server must not mask any frame it sends.**

Measured, against a server that enforces it: a client frame with the mask bit clear is answered with a close frame carrying status **1002** and the connection is dropped.

```
server reaction to an unmasked client frame: closed, status 1002
```

**Masking is not encryption.** The key travels with the payload in the same frame, so anyone reading the bytes can unmask them in one line. Its purpose is to make the **bytes on the wire unpredictable to the sender's future self**: if a client could choose exactly which bytes it puts on a connection through a proxy, it could construct, in advance, a sequence that a later request would be interpreted as — the cache poisoning shape. Masking removes that ability. **It exists for the intermediaries, not for confidentiality.**

**Control frames are separate from data frames.** A ping or pong carries at most 125 bytes, cannot be fragmented, and may be inserted **in the middle of a fragmented message**, so an implementation that assembles a message by concatenating everything it receives will corrupt one. A ping is answered with a pong carrying the same payload — measured, the reply comes back with the payload intact:

```
client sends ping with b"are you there"  ->  server replies pong with b"are you there"
```

**And a frame is not a message.** A message may be split across any number of frames: the first carries the opcode (`0x1`), the rest carry `0x0`, and `FIN` stays clear until the last one. Measured, four frames carrying `"the "`, `"message "`, `"arrives "`, `"in pieces"` reassemble into one message.

```
text/continuation  fin=False  b'the '
text/continuation  fin=False  b'message '
text/continuation  fin=False  b'arrives '
text/continuation  fin=True   b'in pieces'
```

**The protocol separates them on purpose.** A producer that can start sending before it knows the total size does not have to buffer the whole message, and a large message does not have to block a small one behind it on the same connection.

### Part 4: measured against HTTP

The two protocols differ on almost every axis, and the practical ones:

| | HTTP | WebSocket |
|---|---|---|
| Who may speak first | the client | **either side, at any time** |
| Unit | one request, one response | **one frame, and a message built from frames** |
| Lifetime | the exchange | **the whole connection, until either side closes** |
| Per-message overhead | the full headers, every time | **2 to 14 bytes** |
| Connection state | none in the protocol | **a connection exists and can be authenticated once** |
| Multiplexing | HTTP/2 streams | **none built in** — one connection is one channel |
| Compression | per response, via content encoding | per message, via an extension |
| Through intermediaries | assumed to work | **requires `Upgrade` support** |
| Same-origin policy | restricts reads from a page | **does not apply the same way** |

The overhead row is worth a measurement, because it is the reason long-lived connections exist at all:

```
one HTTP request with ordinary headers : 252 bytes, payload 0
a WebSocket frame carrying  6 bytes    :  12 bytes on the wire
a WebSocket frame carrying 20 bytes    :  26 bytes on the wire
a WebSocket frame carrying 60 bytes    :  66 bytes on the wire
```

**The handshake's few hundred bytes are paid once; after that each message costs a few bytes.** An HTTP request pays the whole header set again every time — host, user agent, accept list, cookies and all. That is the trade: cheap messages forever, in exchange for a connection that stays open and occupies resources on every device between the two ends.

**And the statefulness changes the security model rather than the syntax.** An HTTP endpoint authenticates each request, because that is the only moment it exists. A WebSocket connection authenticates once, at the handshake, and then carries however many messages the application defines — which is why authorisation has to be re-asked per message rather than per connection.

### Part 5: the life of one connection

Four phases, and the second one is where deployments break:

**Establishment** — the handshake, with authentication in it — cookies attached by the browser, or a token in the subprotocol field, or the connection refused.

**Keeping it alive** — this is the phase that surprises people. A connection that is idle has no way to prove it is still wanted, and every device in the path has a timeout for idle connections:

| Device | What it does with an idle connection |
|---|---|
| A load balancer | closes it at its configured idle timeout — often 60 seconds |
| A reverse proxy | the same, sometimes sooner |
| A NAT gateway | drops the mapping when its table needs the slot |
| A corporate middlebox | closes it whenever it likes |

**So an application that works locally and disconnects in production, roughly every minute, is not a bug in the application.** It is the middle of the network reclaiming a connection nobody was using. The fix is application-level traffic — a ping or a small message — at an interval **shorter** than the shortest timeout in the path. And it has to be application-level traffic: the WebSocket protocol has ping and pong frames, but **the browser API does not let a script send a ping**, so browser clients send an application message and treat any reply as proof of life.

**Reconnection** — a connection that dies must be re-established, which means the failed connection has to be treated as normal rather than exceptional. Reconnecting immediately and forever produces a thundering herd against a service that is already struggling, so clients back off. And reconnecting restores the **socket**, not the **session**: whatever the application had subscribed to, cached or acknowledged has to be re-established, and messages sent during the gap are gone unless the protocol has sequence numbers and the server can replay them.

**Closing** — either side sends a close frame with a status code, and the codes are worth reading in logs:

| Code | Meaning |
|---|---|
| `1000` | normal closure |
| `1001` | going away — the page is navigating, the server is shutting down |
| `1002` | protocol error — the measured unmasked-frame case |
| `1003` | unsupported data |
| `1008` | policy violation, including "you are no longer authorised" |
| `1009` | message too big |
| `1011` | the server hit an unexpected condition |
| `1006` | **not sent by anyone** — it means the connection closed without a close frame, so it appears in the client as an abnormal closure |

### Part 6: seeing it happen

Three observation points, and they see different amounts:

| Where | What you see |
|---|---|
| `curl` | **the handshake and nothing after it** |
| Browser devtools, the WS panel | the handshake and **every frame, in both directions, decoded** |
| `tcpdump` / Wireshark | the bytes — headers, frames, and everything else |
| Application logs | whatever the application chose to say about messages |

**`curl` stops at the handshake because after `101` the bytes are no longer HTTP**, and `curl` is an HTTP client. What it can do is confirm the upgrade negotiation and its result:

```bash
curl -i -N \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: $(head -c 16 /dev/urandom | base64)" \
  http://host/ws
```

A `101` with a matching accept value means the server agreed. Anything else — a `200`, a `400`, a `403` — means it did not, and the body usually says why. Once you have the `101`, use the browser panel or a small script: the framing is simple enough that a parser is about thirty lines.

**And this is the line that matters for everything in the security half of this topic**: an intermediary in the path can see the handshake, the connection's duration and the byte counts in each direction — and cannot see the frames. Whatever visibility exists into the messages has to come from the application.

### Part 7: the confusions every beginner has

**"Is `wss://` just `ws://` with TLS?"** Yes — and the order matters: TLS is negotiated first, and the HTTP upgrade happens inside the encrypted connection. So `wss` is indistinguishable from HTTPS to anything watching the port.

**"Can a WebSocket connect across origins?"** Yes, and by default. A page on one origin may open a socket to another, and no policy stops it — unlike `fetch`, which is limited by the same-origin policy. That is a difference in kind, not a configuration.

**"Does the handshake carry cookies?"** Yes, automatically, the same as any request to that origin. And the browser will not let a page attach arbitrary headers to the handshake, so a cookie is easy to send and a custom header is impossible.

**"Is a frame the same as a message?"** No. One message is one or more frames, and a control frame may be interleaved.

**"Why does it work locally and drop in production?"** Almost always an idle timeout in the path. See Part 5.

**"Can a page send a ping to keep the connection alive?"** Not with the standard API. The ping frame exists in the protocol and the browser will answer a server's ping, but a script cannot send one, so client-side keepalives are ordinary messages.

**"Should I use Server-Sent Events instead?"** If the data only flows one way, SSE is simpler: it is plain HTTP, reconnection is defined by the protocol, and it goes through anything. WebSocket is what you need when the client also has to talk.

**"Can I send binary?"** Yes — `0x2` frames, `ArrayBuffer` or `Blob` on the browser side. Text frames must be valid UTF-8; sending invalid UTF-8 in a text frame is a protocol error.

**"What about HTTP/2 server push?"** It is not a substitute, and it has been removed from browsers. It pushed resources the client was about to request anyway; it was never a general server-to-client channel.

### Part 8: what follows for security

The mechanics above determine the security properties, and each of the following is a mechanism seen from the other side:

**The handshake is HTTP, so that layer's rules still apply at that instant.** `Origin` is present and is the only field that identifies the page that opened the connection; cookies are attached; an endpoint can refuse. **After the `101` none of that machinery is in the path**, so whatever the HTTP layer would have enforced has to be enforced by the application from then on.

**Cross-origin connections are permitted by default.** There is no browser policy that stops one origin opening a socket to another, so an application that trusts the connection because it exists has the same problem as an endpoint that trusts a request because it arrived.

**A cookie is attached and a custom header cannot be, which is a specific combination.** The credential that would be sent automatically is present, and the token that would prove intent has nowhere to go.

**Masking exists because intermediaries misread traffic**, which is a reminder that a connection is not a private line between two programs. Anything that parses the stream may act on what it thinks it sees.

**A connection outlives the moment of authentication.** Permissions can change while a socket is open, and nothing in the protocol notices.

**Fragmentation means a message's first frame is not the whole message.** Anything that inspects, validates or size-limits based on the first frame can be bypassed with a later one.

**And there is no built-in limit on message size.** The protocol will happily deliver a 500 MB message in frames; how much memory that occupies, and what happens when the limit is reached, is the application's decision.

The general statement: **the protocol change solves the direction of communication, not the trust model.** The handshake still happens inside HTTP, and everything HTTP was providing stops at the upgrade.

### Detection and mitigation

- **Log the handshake, because it is the only part of the exchange an HTTP-layer tool can see.** The origin, the cookies, the requested subprotocol and the response status are all in it, and a connection that was refused is as interesting as one that was accepted.
- **Measure connection duration and byte counts per direction.** For anything sitting on the path, those are the only observables: how long, how much each way. A connection that transfers far more than the client application could produce, or many short connections from one client, is visible without reading a single message.
- **Alert on unexpected close codes.** `1002` is a protocol error and often a client implementation problem; `1006` means the connection died without a close handshake, and a rising rate of it usually means something in the path is timing out; `1008` means the server refused on policy, which is worth counting separately from a disconnection.
- **And instrument the application, because nothing else can see the messages.** Message counts, sizes, latency and error rates per endpoint and per account — the frame layer is opaque to everything but the two ends.
- **For mitigation in the protocol's own terms: send application-level traffic at an interval shorter than the shortest idle timeout in the path.** Determine that timeout from the deployment's own components rather than guessing, and make the interval a configuration value.
- **Treat reconnection as a designed path rather than an error path.** Backoff with jitter, a cap, and a resumption protocol with sequence numbers or a replay window — otherwise a network blip becomes silent data loss, and a service restart becomes a simultaneous reconnect storm.
- **Set a message size limit and enforce it across the whole message, not per frame.** Fragmentation means the frames can each be small while the assembled message is not.
- **Decide about `permessage-deflate` deliberately.** Compression is negotiated in the handshake, and it is what makes a small highly compressible message able to expand into a large one — the same amplification as any compression before encryption, on a channel where the attacker controls the content.
- **Use `wss://` and check the origin and the credential at the handshake**, then keep asking the authorisation question per message, because the connection will outlive the answer.

<!-- lang:zh -->
### 同一端口上的第二套协议

一次 HTTP 交换有固定的形状：客户端问、服务端答、交换结束。Web 上几乎所有东西都适配这个形状，而不适配的那些，就是这一篇要讲的。

> **HTTP 是问与答。一条聊天消息、一次行情跳动、别人的一次按键，需要的是另一个方向** —— 服务端先知道，而客户端必须马上知道。

WebSocket 是这件事的标准答案。它值得上升到字节级别去理解，有三个理由：它从 HTTP 开始、然后不再是 HTTP；它的帧简单到不用库就能读；而大多数让人困惑的地方 —— 以及大多数上线后才出的问题 —— 都来自机制，而不是来自 API。

### 第一部分：HTTP 的形状，以及它做不到的事

HTTP 的四个性质定义了这个难题：

**它由客户端发起。** 服务端不能决定往浏览器推东西。协议里没有一个地址表示"这个浏览器、就是现在" —— 服务端握着一条 socket，但没有任何一条消息的意思是"这是你没要过的东西"。

**它是一个请求加一个响应。** 即使在 `keep-alive` 下、一条 TCP 连接承载很多次交换，每一次交换仍然是一问一答。

**它是无状态的。** 协议不记得上一个请求的任何东西；存在的那些连续性 —— 会话、购物车 —— 都是应用用 cookie 或令牌带过去的。

**头每次都要重带。** 一个请求带着和刚才那个一样的 host、user agent、accept 列表和 cookie。

而有一大类应用需要的是相反的：

| 用户期待什么 | 谁先知道 |
|---|---|
| 一条聊天消息出现 | 对方所在的服务器 |
| 价格跳动 | 交易所 |
| 别人改了这份文档 | 另一位编辑者 |
| 任务跑完、构建失败、订单发货 | 后端 |
| 对手落子 | 对方的客户端 |

历史上的几种绕法，以及各自的代价：

| 办法 | 方向 | 代价 |
|---|---|---|
| **短轮询** —— 每 N 秒问一次 | 客户端问 | 延迟就是那个间隔，而几乎每个请求都是空的 |
| **长轮询** —— 问上去，让服务端把请求挂着 | 客户端问，服务端挂住 | 每个客户端占一条连接和一个工作单元；每次拿到答案都要重发一个请求 |
| **Server-Sent Events** | **只能服务端到客户端** | 只有一个方向，而且它仍然是 HTTP：只发文本、连接随时可能被中间的任何东西关掉 |
| **WebSocket** | **双向** | 换协议，而有些中间设备不支持 |

**WebSocket 不是 HTTP/2。** 它们常被并列称作"现代"，而它们解决的是不同的问题。HTTP/2 把很多次请求/响应复用到一条连接上，于是抓一百个资源不再需要一百条连接。它没有给服务端一个主动开口的办法。两样都需要的页面，两样都用。

### 第二部分：握手就是一次 HTTP 请求

一条 WebSocket 连接以一个普通 HTTP 请求开始，只多了一个不寻常的头。实测，客户端发出的是：

```
GET /ws HTTP/1.1
Host: 127.0.0.1:9101
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: 8oXB0PvCyJ2ScYG1es++qQ==
Sec-WebSocket-Version: 13
```

**155 字节**，而且每一行都是普通 HTTP。服务端回答：

```
HTTP/1.1 101 Switching Protocols
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: akEhchw8/+pJOtPLxAGhIFOnpC8=
```

**129 字节**，而 `101 Switching Protocols` 是全部重点：**同一条 TCP 连接、同一个端口，从这个响应之后就不再讲 HTTP 了。**

逐个头看：

| 头 | 它做什么 |
|---|---|
| `Upgrade: websocket` | 请求换协议 |
| `Connection: Upgrade` | 逐跳的信号，表示正在协商一次更换 |
| `Sec-WebSocket-Key` | **16 个随机字节，base64 编码** —— 一个 nonce，不是凭据 |
| `Sec-WebSocket-Version: 13` | 唯一在用的版本；别的都会被拒 |
| `Sec-WebSocket-Accept` | 服务端对那个 nonce 的回应 |

**而那个 nonce 不是安全机制。** 它的算法是：

```
Sec-WebSocket-Accept = base64(sha1(Sec-WebSocket-Key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
```

实测，这个计算能精确复现那个头的值：

```
key         : 8oXB0PvCyJ2ScYG1es++qQ==
算出来的     : akEhchw8/+pJOtPLxAGhIFOnpC8=
响应里的     : akEhchw8/+pJOtPLxAGhIFOnpC8=   （一致）
```

**没有任何秘密参与** —— 那个 GUID 是公开的常量，key 也是明文发出去的。它的目的是让那个响应**不可能被误打误撞地产生出来**：一个看不懂 `Upgrade` 的代理会看到一个带未知头、带 `101` 状态码的请求，而如果它把这个响应缓存下来或者转发出去，客户端会因为 accept 值对不上而拒绝。这和第三部分的客户端 mask 是同一个动机 —— **协议在防那些会误解它的中间设备**。

几个值得知道的可选头：

| 头 | 用途 |
|---|---|
| `Origin` | 由浏览器设置，也是服务端唯一能用来判断"是哪个页面开的连接"的字段 |
| `Sec-WebSocket-Protocol` | 子协议名，在这里协商 —— 而且**这是浏览器页面唯一能自己设置的字段** |
| `Sec-WebSocket-Extensions` | 客户端支持什么，比如压缩 |
| `Cookie` | 由浏览器附上，和任何发往那个源的请求一样 |

**握手是 HTTP，正是"这一层的规则在这一刻仍然适用"的原因** —— 来源在、cookie 在，服务端可以用任何 HTTP 端点能用的理由拒绝这次升级。而变化在于：`101` 之后，HTTP 那套机器就完全不在路径上了。

### 第三部分：帧，最小的单位

升级之后，双方交换的是**帧**。一个客户端帧的头部最多十四个字节，剩下是载荷：

```
客户端发: b"hello websocket"     （载荷 15 字节，线上 21 字节）

  81 8f                <- 头部 2 字节：FIN+opcode / MASK+长度
  d7 76 23 c7          <- 4 字节 masking key，随机
  bf 13 4f ab b8 ...   <- 15 字节载荷，已被 key 逐字节异或
```

那两个头部字节，逐位看：

| 位数 | 字段 | 这里的含义 |
|---|---|---|
| 1 | `FIN` | `1` = 这是这条消息的最后一帧 |
| 3 | `RSV1-3` | 留给扩展；`RSV1` 被压缩用 |
| 4 | `opcode` | `0x1` = 文本 |
| 1 | `MASK` | `1` = 载荷被 mask 过 |
| 7 | 载荷长度 | `0x0f` = 15，或者是"长度在后面"的标记 |

opcode：

| opcode | 名称 | 承载什么 |
|---|---|---|
| `0x0` | continuation | 前面某一帧开始的那条消息的剩余部分 |
| `0x1` | text | UTF-8 文本 |
| `0x2` | binary | 任意字节 |
| `0x8` | close | 一次关闭握手，带状态码 |
| `0x9` | ping | 保活探测 |
| `0xa` | pong | 对 ping 的回复 |

**载荷长度有三种编码**，这个细节在解析时会咬人：125 及以下直接放在那 7 个长度位里；`126` 表示真实长度跟在后 2 个字节里；`127` 表示跟在后 8 个字节里。所以一个 6 字节载荷在线上的头是 6 字节，一个 60 字节载荷的头也是同样 6 字节 —— **头只有超过 125 才会变大**。

**而 mask 的规则是不对称的：**

> **客户端发出的每一帧都必须 mask。服务端发出的每一帧都不许 mask。**

实测，对着一个会强制执行它的服务端：一个 mask 位为 0 的客户端帧，会收到一个带状态码 **1002** 的关闭帧，然后连接被断开。

```
服务端对未 mask 的客户端帧的反应: 关闭，状态码 1002
```

**mask 不是加密。** 那把 key 和载荷在同一帧里一起传，所以任何读到这些字节的人一行就能还原。它的目的是让**线上的字节对发送者未来的自己也不可预测**：如果一个客户端能精确选择它经某个代理发出的字节，它就能提前构造出一段"之后某个请求会被解释成的东西" —— 缓存投毒那个形状。mask 拿掉了这个能力。**它存在是为了中间设备，不是为了保密。**

**控制帧与数据帧是分开的。** 一个 ping 或 pong 最多带 125 字节、不能分片，而且**可以插在一条分片消息的中间** —— 所以一个"把收到的所有东西拼起来"的实现会拼坏。ping 会用携带同样载荷的 pong 来回答 —— 实测，回复带着完整载荷回来：

```
客户端发 ping，载荷 b"are you there"  ->  服务端回 pong，载荷 b"are you there"
```

**而一帧不是一条消息。** 一条消息可以拆到任意多帧里：第一帧带 opcode（`0x1`），其余带 `0x0`，`FIN` 一直清着直到最后一帧。实测，四个分别装着 `"the "`、`"message "`、`"arrives "`、`"in pieces"` 的帧拼成了一条消息。

```
text/continuation  fin=False  b'the '
text/continuation  fin=False  b'message '
text/continuation  fin=False  b'arrives '
text/continuation  fin=True   b'in pieces'
```

**协议有意把它们分开。** 一个在还不知道总大小之前就能开始发的生产者，不必把整条消息先缓起来；而一条大消息也不必把同一连接上的一条小消息堵在后面。

### 第四部分：与 HTTP 逐项对照

两个协议几乎每个轴上都不同，其中实用的是这些：

| | HTTP | WebSocket |
|---|---|---|
| 谁可以先开口 | 客户端 | **任何一方，任何时刻** |
| 单位 | 一个请求、一个响应 | **一帧，以及由帧拼成的消息** |
| 寿命 | 那次交换 | **整条连接，直到某一方关闭** |
| 每消息开销 | 每次都是整套头 | **2 到 14 字节** |
| 连接状态 | 协议里没有 | **存在一条连接，而且可以被认证一次** |
| 多路复用 | HTTP/2 的流 | **没有内建** —— 一条连接就是一个通道 |
| 压缩 | 按响应，走内容编码 | 按消息，走扩展 |
| 经过中间设备 | 假定可以 | **需要它支持 `Upgrade`** |
| 同源策略 | 限制页面读取 | **不以同样的方式适用** |

开销那一行值得量一下，因为长期连接存在的理由就是它：

```
一次带常见头的 HTTP 请求        : 252 字节，载荷 0
一个 WebSocket 帧，载荷  6 字节 :  12 字节
一个 WebSocket 帧，载荷 20 字节 :  26 字节
一个 WebSocket 帧，载荷 60 字节 :  66 字节
```

**握手那几百字节只付一次；之后每条消息只付几个字节。** 而 HTTP 每次都要把整套头再付一遍 —— host、user agent、accept 列表、cookie，全套。这就是那笔交易：消息永远便宜，换来的是一条一直开着、并在两端之间每一个设备上占资源的连接。

**而"有状态"改变的是安全模型，不是语法。** 一个 HTTP 端点在每个请求上做认证，因为那是它唯一存在的时刻。一条 WebSocket 连接在握手时认证一次，然后承载应用定义的任意多条消息 —— 所以授权必须按消息重新问，而不是按连接问一次。

### 第五部分：一条连接的一生

四个阶段，而第二个是部署出问题的地方：

**建立** —— 握手，认证发生在里面 —— cookie 由浏览器附上、或者把令牌放在子协议字段里、或者直接拒绝这次连接。

**让它活着** —— 这是让人意外的阶段。一条空闲的连接没有任何办法证明自己还被需要，而路径上每个设备都有针对空闲连接的超时：

| 设备 | 它对一条空闲连接做什么 |
|---|---|
| 负载均衡器 | 到它配置的空闲超时就关掉 —— 常常是 60 秒 |
| 反向代理 | 一样，有时更早 |
| NAT 网关 | 表项需要位置时把映射丢掉 |
| 企业里的中间盒 | 它想关就关 |

**所以一个"本地正常、上线后大约每分钟断一次"的应用，不是应用里的 bug。** 那是网络中间的部分在回收一条没人用的连接。修法是在应用层发流量 —— 一个 ping 或一条小消息 —— 间隔要**短于**路径上最短的那个超时。而且必须是应用层的流量：WebSocket 协议有 ping 和 pong 帧，但**浏览器 API 不允许脚本发 ping**，所以浏览器端发一条应用消息，并把任何回复当作活着的证明。

**重连** —— 一条死掉的连接必须重建，这意味着那条失败的连接要被当成正常情况而不是异常。立刻且永不停歇地重连，会对一个本来就在挣扎的服务制造惊群，所以客户端要退避。而重连恢复的是**套接字**，不是**会话**：应用订阅了什么、缓存了什么、确认过什么，都得重新建立；而空档期间发出的消息就是丢了，除非协议有序号、服务端能重放。

**关闭** —— 任何一方发一个带状态码的关闭帧，而那些状态码值得在日志里读：

| 状态码 | 含义 |
|---|---|
| `1000` | 正常关闭 |
| `1001` | 要走了 —— 页面在跳转，服务端在停机 |
| `1002` | 协议错误 —— 上面实测的未 mask 那种 |
| `1003` | 不支持的数据类型 |
| `1008` | 违反策略，包括"你已经不再被授权了" |
| `1009` | 消息太大 |
| `1011` | 服务端遇到意外状况 |
| `1006` | **它不是谁发出来的** —— 它表示连接没有经过关闭握手就断了，所以在客户端表现为异常关闭 |

### 第六部分：真的把它看见

三个观察点，各自看到的东西不一样：

| 在哪看 | 能看到什么 |
|---|---|
| `curl` | **握手，以及之后的什么都没有** |
| 浏览器开发者工具的 WS 面板 | 握手，以及**双向的每一帧、已解码** |
| `tcpdump` / Wireshark | 字节 —— 头、帧，以及其余一切 |
| 应用日志 | 应用选择说出来的那些关于消息的东西 |

**`curl` 到握手为止，是因为 `101` 之后的字节不再是 HTTP**，而 `curl` 是一个 HTTP 客户端。它能做的是确认这次升级协商及其结果：

```bash
curl -i -N \
  -H "Connection: Upgrade" \
  -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" \
  -H "Sec-WebSocket-Key: $(head -c 16 /dev/urandom | base64)" \
  http://host/ws
```

一个带正确 accept 值的 `101` 表示服务端同意了。别的任何东西 —— `200`、`400`、`403` —— 表示它没同意，而响应体一般会说为什么。一旦拿到 `101`，就用浏览器面板或者一个小脚本：帧的格式简单到解析器大约三十行。

**而这条分界线，对这类话题的安全那一半是决定性的**：路径上的中间设备能看到握手、连接的时长、以及每个方向的字节数 —— **看不到帧**。关于消息的一切可见性，都只能来自应用。

### 第七部分：每个新手都会有的困惑

**"`wss://` 就是加了 TLS 的 `ws://` 吗？"** 是 —— 而顺序要紧：先协商 TLS，HTTP 升级发生在加密连接里面。所以对盯着端口的任何东西来说，`wss` 与 HTTPS 无法区分。

**"WebSocket 能跨源连吗？"** 能，而且默认就允许。一个源上的页面可以打开到另一个源的套接字，没有任何策略阻止它 —— 这与 `fetch` 不同，后者受同源策略限制。这是性质上的差别，不是一个配置项。

**"握手会带 cookie 吗？"** 会，自动带，和发往那个源的任何请求一样。而浏览器不允许页面给握手加任意请求头，所以 cookie 容易发，自定义头不可能。

**"帧和消息是一回事吗？"** 不是。一条消息是一帧或多帧，而控制帧可以插在中间。

**"为什么本地正常、上线就断？"** 几乎总是路径上某个空闲超时。见第五部分。

**"页面能发 ping 来保活吗？"** 标准 API 不能。ping 帧在协议里存在，浏览器也会回答服务端的 ping，但脚本发不了，所以客户端侧的保活就是普通消息。

**"该用 Server-Sent Events 代替吗？"** 如果数据只往一个方向流，SSE 更简单：它是纯 HTTP、重连由协议定义、而且能穿过任何东西。客户端也要说话时，才需要 WebSocket。

**"能发二进制吗？"** 能 —— `0x2` 帧，浏览器侧用 `ArrayBuffer` 或 `Blob`。文本帧必须是合法 UTF-8；在文本帧里发非法 UTF-8 是协议错误。

**"HTTP/2 的 server push 能替代吗？"** 不能，而且它已经从浏览器里被移除了。它推的是客户端本来就准备请求的资源，从来不是一个通用的服务端到客户端通道。

### 第八部分：从这些机制推出的安全观念

上面的机制决定了安全性质，下面每一条都是同一个机制从另一面看过去：

**握手是 HTTP，所以那一层的规则在那一瞬间仍然适用。** `Origin` 在、而且是唯一能标识"是哪个页面开的连接"的字段；cookie 被附上；端点可以拒绝。**而 `101` 之后那套机器就不在路径上了**，所以 HTTP 层本来会执行的东西，从此之后必须由应用来执行。

**跨源连接默认被允许。** 没有任何浏览器策略阻止一个源去打开到另一个源的套接字，所以一个"因为连接建立了就信任它"的应用，和一个"因为请求到达了就信任它"的端点，是同一个问题。

**cookie 会被附上，而自定义头带不了 —— 这是一个很具体的组合。** 本该自动发送的那个凭据在场，而本该用来证明意图的那个令牌无处可放。

**mask 存在是因为中间设备会误读流量**，这提醒我们：一条连接不是两个程序之间的专线。任何解析这条流的实体，都可能对它以为看到的东西采取行动。

**一条连接会比认证的那一刻活得久。** 权限可以在套接字开着的时候改变，而协议里没有任何东西会注意到。

**分片意味着一条消息的第一帧不是整条消息。** 任何"只看第一帧"的检查、校验或大小限制，都能被后面某一帧绕过。

**而且协议没有内建的消息大小上限。** 它会乐意地用很多帧送出 500 MB 的一条消息；那占多少内存、以及到上限时发生什么，是应用的决定。

总的说法：**换协议解决的是通信方向的问题，不是信任模型的问题。** 握手仍然发生在 HTTP 里面，而 HTTP 提供的一切在升级那一刻就停了。

### 检测与缓解

- **把握手记下来，因为那是 HTTP 层工具能看到的唯一部分。** 来源、cookie、请求的子协议、以及响应状态都在里面，而被拒绝的一次连接和被接受的一次同样有意思。
- **量连接的时长与每个方向的字节数。** 对任何位于路径上的东西来说，那是唯一的可观测量：多久、各方向多少。一条传输量远超客户端应用能产出的连接，或者一个客户端发出很多短连接，都不用读任何一条消息就能看见。
- **对意外的关闭状态码告警。** `1002` 是协议错误，常常是客户端实现的问题；`1006` 意味着连接没有经过关闭握手就死了，它上升通常说明路径上有什么在超时；`1008` 是服务端按策略拒绝，值得和普通断开分开计数。
- **并且给应用加埋点，因为别的东西看不到消息。** 按端点和按账号的消息条数、大小、延迟与错误率 —— 帧那一层除了两端以外对一切都不透明。
- **按协议自己的术语缓解：以短于路径上最短空闲超时的间隔发应用层流量。** 那个超时要从部署自己的组件上确认，而不是猜，并且把间隔做成一个配置项。
- **把重连当成一条设计好的路径，而不是错误路径。** 带抖动的退避、一个上限、以及带序号或重放窗口的恢复协议 —— 否则一次网络抖动会变成静默的数据丢失，而一次服务重启会变成同时重连的风暴。
- **设一个消息大小上限，并且在整条消息上执行，而不是按帧。** 分片意味着每一帧都可以很小，而拼起来的那条消息不是。
- **对 `permessage-deflate` 要有意识地决定。** 压缩在握手时协商，而正是它让一条很小、又高度可压缩的消息能膨胀成一条很大的消息 —— 和"加密前压缩"是同一个放大效应，只不过发生在一条攻击者控制内容的通道上。
- **用 `wss://`，在握手处检查来源与凭据**，然后按消息继续问授权问题，因为这条连接会比那个答案活得久。
