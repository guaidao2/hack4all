---
id: api-webhooks-and-callbacks
title_en: Webhooks and Callbacks
title_zh: Webhook 与回调
summary_en: The one pattern where your service makes a request to an address somebody else chose, and separately receives requests that no user authenticated. Both directions are measured — the outbound call as a server-side request forgery with four spellings of one address and a redirect that reaches it anyway, and the inbound side as a question about which bytes a signature covers.
summary_zh: 这是唯一一个"你的服务去请求一个别人选的地址"的模式，而且它同时要接收一批没有任何用户认证过的请求。两个方向都实测了 —— 外呼作为服务端请求伪造，一个地址的四种写法加一次重定向照样到达；入站那一侧则是"签名到底盖住了哪些字节"。
tags: [beginner, api, webhook, ssrf, signature]
tools: [python3, curl, hmac]
attck: [T1190, T1557]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The pattern with two directions

Every other integration in this series has one direction. A webhook has two, and they fail in different ways.

> **Your service makes an HTTP request to an address somebody else chose. And it receives HTTP requests that no user authenticated.**

The first is a server-side request forgery primitive by construction — "give it a URL and it will fetch it" is the definition. The second is an endpoint that has to be reachable by anyone and still be able to tell who sent the request, which is why it is the one place where the application has to implement authentication itself.

Both measured below.

### Part 1: the shape of it

| | Outbound (you deliver) | Inbound (you receive) |
|---|---|---|
| who starts it | your service | the provider |
| the address | **supplied by a user or a tenant** | yours, published |
| the caller's identity | a signature you compute | **a signature you verify** |
| failure handling | retries, backoff, a dead letter | return a status, expect a retry |
| the trust direction | **you are the client** | **you are the server and the stranger** |

**And the reason the pattern exists at all is worth one sentence**, because it explains why the outbound address is user-supplied: a webhook replaces polling, so the subscriber has to tell the publisher where to call. **The address is configuration, and configuration is input.**

### Part 2: outbound — the address is chosen by somebody else

The measurement, with an internal service that an outside caller cannot reach:

| What was registered as the webhook | Result |
|---|---|
| **the internal address directly** | `200`, the internal service received `POST /admin/secret` |
| **an attacker's address that answers `302` to the internal one** | the sender **followed the redirect**, and the internal service received `GET /admin/secret` |
| the same, with redirects not followed | `302` returned, **the internal service was not reached** |
| five spellings of the internal address | **four of them reached it** |

**The fourth row is the one to keep.** With one host reachable at `127.0.0.1`, the same host was reached by all of these:

```
http://127.0.0.1:<port>/admin     reached
http://127.1:<port>/admin         reached
http://2130706433:<port>/admin    reached
http://0x7f000001:<port>/admin    reached
http://[::1]:<port>/admin         not reached, because that service listened on IPv4 only
```

**Three of those are the same four bytes of address written differently** — a shortened form, a decimal integer, and a hexadecimal one. A rule that refuses the string "127.0.0.1" refuses exactly one spelling, and the measured table is four.

> **Blocking an address by its text means blocking the people who wrote it that way. The address itself has more than one text.**

**And the redirect row is a second lesson with the same shape.** The address the application validated and the address it eventually contacted were different, and nothing in the flow compared them. **Measured, not following redirects stopped it completely** — which is why that is a control rather than a nicety.

**And there is a third gap in the same family, which follows from the first two.** If the check resolves a hostname and the connection resolves it again, those are two lookups, and the answer can differ between them. **The control is to validate the resolved address and then connect to that same address**, so the thing checked and the thing used are the same value — the same principle as the entry on gateways, where a filter and a router disagreed about a path.

**What a blocking rule should look like, in order of how much it depends on parsing:**

| Approach | What it stops |
|---|---|
| a text deny-list of hostnames | the measurements above, and little else |
| parsing and refusing private ranges **per resolved address** | the four spellings, and any name that resolves into one |
| the above, plus connecting to the validated address | the same, plus a second lookup that answers differently |
| the above, plus no redirects, standard ports only, and an egress host | the same, plus the redirect measured above |

**And the strongest control is not a filter at all**: deliver from a host that has no access to the internal network, so that reaching the internal network is not something the filter has to prevent.

### Part 3: outbound — what the request carries

**The event body is the data, and the signature is what lets the receiver believe it came from you.** Two things about that are worth stating.

**The signing secret is a long-lived shared secret between two organisations**, which puts it in the same family as an API key: it has to be generated with enough entropy to resist guessing, stored where the receiving side can use it and no one else can read it, rotatable without a flag day, and separate per subscriber so that one leak does not let somebody forge events to every customer.

**And the request goes to an address somebody else controls**, which means **the response body is third-party content.** Measured, a receiver answered with this:

```
{"status": "ok", "internal_note": "db=prod-01 user=app token=SHOULD-NOT-LEAK"}
```

**A sender that logs the response body, or includes it in a retry record or an error message, has put that content in its own logs.** The receiver chose it; the sender's log is where it ends up.

### Part 4: inbound — what the signature actually covers

This is the measurement the receiving side needs, and it is about a distinction that looks like a detail.

A signed body, and three byte strings that all parse to the same object:

```
{"event": "payment", "amount": 1}      the original bytes
{"amount": 1, "event": "payment"}      keys reordered
{\n  "event": "payment",\n  "amount": 1\n}   pretty printed
```

Offered to two verification routines, one over the raw bytes and one over the re-serialised parsed object:

| Body | Verified over raw bytes | Verified over the parsed object |
|---|---|---|
| the original | **accepted** | **accepted** |
| keys reordered | **refused** | **accepted** |
| pretty printed | **refused** | **accepted** |

**The left column is strict and the right column is not**, and the reason is what the signature is over:

> **A signature covers bytes. Parsing turns bytes into an object, and the object has lost everything at the byte level** — the order, the spacing, the duplicates, the exact spelling of every escape.

**So the order of operations is the control:**

> **Verify the signature over the bytes you received, and parse afterwards. Verifying a parsed body means the thing you checked is not the thing you read.**

**And the practical reason implementers get this wrong is sympathetic**: a signature over raw bytes fails when the sender and receiver disagree about serialisation, and the quickest fix looks like re-serialising both sides. **That fix removes the strictness measured in the left column**, and it is worth knowing that it does before choosing it.

### Part 5: duplicate keys, where two parsers disagree

JSON permits the same key more than once, and what that means is left to the implementation. Measured, one body and two parsers:

```
{"amount": 1, "amount": 100}

taking the last occurrence (the common default):  amount = 100
taking the first occurrence (strict, and some others): amount = 1
```

**One string of bytes, two values, and both are correct per the specification.**

**And signed with the raw bytes, that body verifies.** Measured, a signature over the whole string:

```
{"amount": 1, "amount": 100}   signature over the bytes: accepted
                               what the application reads: 100
```

**So the signature did its job — the bytes are exactly the bytes that were signed — and the value the application acts on is whichever occurrence its parser preferred.** If the component that verified and the component that reads are not the same code, they can disagree about which one that is.

**And the same mechanism shows up as a strictness test.** Measured, taking a legitimate signed event and appending a second copy of a key:

```
{"event": "payment", "amount": 1, "amount": 99999}
  verified over raw bytes with the original signature: refused — the bytes changed
```

**Which is the reassuring result**: raw-byte verification does catch the appended duplicate, because appending is a byte change. **The window opens when verification happens over a parsed object**, or when the two sides of the check parse differently — and then the signature is over a value nobody compared.

**So the rule is the same as Part 4, with a second half**: verify the bytes, parse once, and **have one parser on one side of the boundary** — because the measured ambiguity is inside the format, not in either implementation.

### Part 6: replay, and why the sender makes it routine

**A signature says the content is authentic. It does not say this is the first time.** Measured, on a receiver that verified and did nothing else:

| Delivery | Verifying receiver | Receiver with a timestamp and a record of seen signatures |
|---|---|---|
| first | accepted | accepted |
| **the same signed request again** | **accepted** | **refused — this signature has been processed** |

**And the reason this matters more here than elsewhere is that retries are part of the design.** A delivery that times out is retried, so **the same event arriving twice is the normal case**, not an attack — which means the receiver has to be idempotent regardless, and the replay defence is a second, separate mechanism on top.

**Two mechanisms, and they do different jobs:**

**A timestamp window** bounds how long a captured event stays usable.
**An idempotency key from the event itself** makes processing it twice have the effect of processing it once — and the key has to come from the sender's event, not from the delivery attempt, or the retry gets a new one.

**And the signature is a good thing to deduplicate on**, as measured, provided the sender signs each event exactly once and does not re-sign on retry.

### Part 7: the inbound endpoint is the one you must implement yourself

**Every other endpoint in an application sits behind the application's own authentication.** A webhook endpoint cannot: the caller is another organisation's server, there is no session, and there is no user.

> **A webhook endpoint is the one place that must be reachable anonymously and must still know who called — which is why it is the one place where the application implements authentication itself.**

**And what it has to do is a short list, all of it measured above or in the entry on authentication**: verify a signature over the raw bytes with a constant-time comparison, check a timestamp, reject a signature it has already processed, and act on an event whose shape it validates rather than trusts.

**And the failure modes are the ones already measured in this series**, which is why this entry is short on mechanisms and long on references:

| Failure | Where it was measured |
|---|---|
| a signature over a parsed body rather than the bytes | this entry, Part 4 |
| a non-constant-time comparison | the authentication entry, Part 7 |
| no timestamp, so a capture stays valid | this entry, Part 6 |
| two parsers disagreeing about one string | this entry, Part 5 |

**And the secrets are shared, long-lived and per subscriber**, which ties this endpoint to the key-management entry: the signing secret is a credential, and it is one that two organisations both hold.

### Part 8: what follows for security

**The outbound direction is a request forgery, and it is not a subtle one.** Measured, the internal service was reached directly, reached again through a redirect the sender followed, and reached through three alternative spellings of the same address. **The controls are about the value actually used rather than the text supplied** — resolve, validate the resolved address, connect to that address, do not follow redirects, and prefer a delivery host that cannot reach the internal network at all.

**And the redirect measurement is the cleanest of the four**, because it shows an address being checked and a different one being contacted inside a single request. **Not following redirects was measured to close it completely**, which makes it the cheapest control in the entry.

**The inbound direction is an unauthenticated endpoint that has to authenticate.** Measured, the difference between verifying bytes and verifying a parsed object changed the answer for two of three bodies that a person would call identical — and the duplicate-key measurement showed one byte string yielding two values depending on the parser. **So "did the signature check pass" and "what did the application act on" are two questions, and this is where they come apart.**

**Verify the bytes, then parse. Parse once.** Both halves matter, because the measured ambiguity is in the format rather than in the implementations, and no amount of care in either parser removes it.

**Replay is normal traffic here.** Measured, a verifying receiver processed the same signed request twice, and a receiver with a timestamp and a record did not. **And because retries are designed in, idempotency is not optional** — the receiver has to be able to process the same event twice and produce one effect, whatever the replay defence does.

**And the response body from a subscriber is third-party content.** Measured, one contained a database name, a user and a token. **Whatever the receiving side chooses to answer with is what the sending side's logs will contain** if it records responses, which is a decision each side makes about the other's data.

### Detection and mitigation

- **Alert on webhook destinations that resolve into private address space**, and log the resolved address rather than the supplied one. Measured, the supplied text has at least four spellings of the same host.
- **Alert on a delivery that followed a redirect**, and treat it as a configuration error or an attempt. Measured, following one reached an internal service from an attacker-supplied address.
- **Record the number of redirects and the final URL for every delivery**, because the measured gap was between the address checked and the address contacted.
- **Watch signature verification failures by subscriber**, since a run of them is either a broken key rotation or somebody guessing.
- **Watch for the same event identifier arriving repeatedly**, and separate "the sender retried" from "somebody replayed" by whether the signature is identical.
- **Alert if delivery responses are being logged in full**, because the measured response body contained a database name and a token.
- **For mitigation, resolve the address, validate the resolved value against private ranges, and connect to that same value**; disable redirects; restrict delivery to standard ports; and deliver from a host with no route to the internal network.
- **Verify the signature over the raw bytes and parse afterwards**, with one parser on one side of the boundary, and use a constant-time comparison.
- **Require a timestamp inside a short window, and keep a record of processed signatures**, in addition to an idempotency key taken from the event.
- **Treat the signing secret as a credential**: high entropy, per subscriber, stored so only the receiver can read it, and rotatable with an overlap period rather than a switch.
- **Validate the event's shape and its types before acting on it**, since the measured duplicate-key case produced a value the sender never intended to authorise.
- **And keep the delivery host more restricted than the application**, because the strongest control on an outbound call to a user-supplied address is not being able to reach anything worth reaching.

<!-- lang:zh -->
### 一个有两个方向的模式

这个系列里其他每一种集成都有一个方向。webhook 有两个，而它们以不同的方式失败。

> **你的服务向一个别人选的地址发出一个 HTTP 请求。而它同时接收一批没有任何用户认证过的 HTTP 请求。**

前一件事按构造就是一个服务端请求伪造原语 —— "给它一个 URL，它就会去取"就是它的定义。后一件事是一个必须对任何人可达、却仍然要能说出是谁发来的端点 —— 这就是为什么它是应用必须自己实现认证的那一处。

两个方向都在下面实测。

### 第一部分：它的形状

| | 出站（你投递） | 入站（你接收） |
|---|---|---|
| 谁发起 | 你的服务 | 提供方 |
| 那个地址 | **由用户或租户提供** | 你的，公开出去 |
| 调用者的身份 | 你计算出的签名 | **你要验证的签名** |
| 失败处理 | 重试、退避、死信 | 返回状态码、预期对方重试 |
| 信任方向 | **你是客户端** | **你是服务端，也是那个陌生人** |

**而这个模式为什么存在，一句话就够**，因为它解释了出站地址为什么由用户提供：webhook 取代轮询，所以订阅方必须告诉发布方该往哪打。**那个地址是配置，而配置就是输入。**

### 第二部分：出站 —— 地址是别人选的

实测，用一个外面调用者够不到的内网服务：

| 注册的 webhook 地址 | 结果 |
|---|---|
| **直接填内网地址** | `200`，内网服务收到了 `POST /admin/secret` |
| **一个攻击者的地址，它回 `302` 指向内网** | 发送方**跟随了重定向**，内网服务收到了 `GET /admin/secret` |
| 同上，但不跟随重定向 | 返回 `302`，**内网没有被到达** |
| 同一个内网地址的五种写法 | **其中四种到达了它** |

**第四行才是要留下的。** 一个主机在 `127.0.0.1` 上可达，而下面这些全都到达了同一个主机：

```
http://127.0.0.1:<port>/admin     到达
http://127.1:<port>/admin         到达
http://2130706433:<port>/admin    到达
http://0x7f000001:<port>/admin    到达
http://[::1]:<port>/admin         没到达，因为那个服务只监听了 IPv4
```

**其中三种是同一个地址的四个字节换了几种写法** —— 一个缩写形式、一个十进制整数、一个十六进制。一条拒绝字符串 "127.0.0.1" 的规则，拒绝的恰好是其中一种写法，而实测表里有四种。

> **按文本拦一个地址，拦的是"这么写的人"。而那个地址本身不止一种文本。**

**而重定向那一行是同一个形状的第二课。** 应用校验过的地址与它最终联系的地址不一样，而整个流程里没有任何东西比较过它们。**实测，不跟随重定向把它完全挡住了** —— 这就是为什么那是一条控制，而不是一个讲究。

**而这个家族里还有第三道缝，它从前两条推出来。** 如果检查时解析了一次主机名、连接时又解析一次，那是两次解析，而两次之间答案可以不同。**控制手段是校验解析出来的地址、然后连到同一个地址上**，这样被检查的东西与被使用的东西是同一个值 —— 与网关那一篇里"过滤器与路由器对路径的理解不一致"是同一条原则。

**一条拦截规则该长什么样，按它对解析的依赖程度排序：**

| 做法 | 能挡住什么 |
|---|---|
| 一份主机名文本黑名单 | 上面那些实测，以及很少的其他东西 |
| 解析之后**按解析出的地址**判断并拒绝私有段 | 那四种写法，以及任何解析到私有段的域名 |
| 再加"连到被校验的那个地址" | 同上，再加一次答案不同的二次解析 |
| 再加"不跟随重定向、只允许标准端口、专用出网主机" | 同上，再加实测那次重定向 |

**而最强的控制根本不是过滤器**：从一个够不到内网的主机上投递，这样"够到内网"就不是过滤器需要阻止的事情。

### 第三部分：出站 —— 这个请求带着什么

**事件体是数据，而签名是让接收方相信它来自你的东西。** 关于它有两点值得说。

**签名密钥是两个组织之间的长期共享秘密**，这把它放进了与 API key 同一类：它要有足够熵以抵抗猜测、要存在只有接收方能读到的地方、要能在没有"切换日"的情况下轮换、并且要按订阅方分开，这样一次泄露不会让某人向所有客户伪造事件。

**而这个请求发往一个别人控制的地址**，这意味着**响应体是第三方内容。** 实测，一个接收方这样回答：

```
{"status": "ok", "internal_note": "db=prod-01 user=app token=SHOULD-NOT-LEAK"}
```

**一个把响应体写进日志、写进重试记录或者错误信息的发送方，已经把那段内容放进了自己的日志里。** 那是接收方选的，而发送方的日志是它最终落地的地方。

### 第四部分：入站 —— 签名到底盖住了什么

这是接收方需要的那组实测，而它关于一个看起来像细节的区别。

一个签过名的事件体，以及三个都解析成同一个对象的字节串：

```
{"event": "payment", "amount": 1}      原始字节
{"amount": 1, "event": "payment"}      键的顺序换了
{\n  "event": "payment",\n  "amount": 1\n}   美化过
```

交给两个校验函数，一个按原始字节、一个按重新序列化后的解析对象：

| 请求体 | 按原始字节验签 | 按解析后的对象验签 |
|---|---|---|
| 原始 | **通过** | **通过** |
| 键顺序不同 | **拒绝** | **通过** |
| 美化过 | **拒绝** | **通过** |

**左列是严格的，右列不是**，而原因就在于签名是对什么做的：

> **签名盖住的是字节。解析把字节变成一个对象，而那个对象丢了字节层面的全部东西** —— 顺序、空白、重复键、每一个转义的确切写法。

**所以操作的顺序就是那条控制：**

> **对你收到的字节验签，之后再解析。对一个解析后的体验签，意味着你检查的东西不是你读的东西。**

**而实现者为什么会弄错，原因是值得同情的**：按原始字节的签名会在发送方与接收方对序列化意见不一致时失败，而最快的修法看起来就是"两边都重新序列化一遍"。**那个修法移除了左列那个严格性**，而值得在选择它之前就知道这一点。

### 第五部分：重复键，两个解析器会分歧的地方

JSON 允许同一个键出现多次，而那意味着什么留给实现决定。实测，一个请求体、两个解析器：

```
{"amount": 1, "amount": 100}

取最后出现的那个（常见的默认）：  amount = 100
取最先出现的那个（严格实现，以及另一些）： amount = 1
```

**一串字节，两个值，而按规范两个都正确。**

**而按原始字节验签，这个请求体能通过。** 实测：

```
{"amount": 1, "amount": 100}   对整串字节验签：通过
                               而应用读到的是：100
```

**所以签名做了它该做的事 —— 那些字节就是被签的那些字节 —— 而应用据以行事的值，是它的解析器偏好的那一个。** 如果负责验签的组件与负责读取的组件不是同一段代码，它们可以对于"是哪一个"意见不同。

**而同一个机制会以一个严格性测试的形式出现。** 实测，拿一个合法的签过名的事件、给它追加一份同一个键：

```
{"event": "payment", "amount": 1, "amount": 99999}
  用原来那个签名对原始字节验签：拒绝 —— 字节变了
```

**这是让人放心的那个结果**：按原始字节验签确实抓得住追加的重复键，因为追加是一次字节改变。**窗口在"验签针对的是解析后的对象"时打开**，或者在检查的两侧解析方式不同时打开 —— 那时签名覆盖的是一个没人比对过的值。

**所以规则与第四部分一样，只是多了后半句**：对字节验签、只解析一次、并且**边界的一侧只有一个解析器** —— 因为实测那个歧义在格式里，而不在任何一方实现里。

### 第六部分：重放，以及为什么发送方让重放变成常态

**签名说的是内容是真的。它不说这是第一次。** 实测，在一个只验签、别的什么都不做的接收方上：

| 投递 | 只验签的接收方 | 带时间戳与签名记录的接收方 |
|---|---|---|
| 第一次 | 接受 | 接受 |
| **同一份签过名的请求再来一次** | **接受** | **拒绝 —— 这个签名已经处理过** |

**而这件事在这里比在别处更要紧，因为重试是设计的一部分。** 一次超时的投递会被重试，所以**同一个事件到达两次是正常情况**、而不是攻击 —— 这意味着接收方无论如何都必须是幂等的，而重放防护是在它之上的第二个、独立的机制。

**两个机制，做的是不同的事：**

**时间戳窗口**限制了被抓到的一个事件在多长时间内还能用。
**取自事件本身的幂等键**让处理两次的效果等于处理一次 —— 而那个键必须来自发送方的事件体、不能来自这次投递的尝试，否则重试会拿到一个新的。

**而签名是一个很适合用来去重的东西**，如实测所示，前提是发送方对每个事件只签一次、并且在重试时不重新签。

### 第七部分：入站端点是那个你必须自己实现的

**应用里其他每一个端点都坐在应用自己的认证后面。** webhook 端点不能：调用者是另一个组织的服务器，没有会话，也没有用户。

> **webhook 端点是唯一一个"必须匿名可达、却仍然必须知道是谁调用"的地方 —— 这就是为什么它是唯一一个应用自己实现认证的地方。**

**而它要做的事是一份很短的清单，全部都在上面、或者在认证那一篇里实测过**：用常数时间比较、对原始字节验签，检查时间戳，拒绝一个已经处理过的签名，并且对事件的形状作校验而不是信任。

**而失败方式都是这个系列里已经实测过的那些**，这就是为什么这一篇机制讲得少、引用得多：

| 失败 | 在哪里实测过 |
|---|---|
| 对解析后的体验签而不是对字节 | 本篇第四部分 |
| 非常数时间的比较 | 认证那一篇第七部分 |
| 没有时间戳，抓到的请求长期有效 | 本篇第六部分 |
| 两个解析器对同一串字节意见不同 | 本篇第五部分 |

**而秘密是共享的、长期有效的、按订阅方分开的**，这把本端点与密钥管理那一篇绑在一起：签名密钥就是一个凭据，而且是两个组织都持有的那一种。

### 第八部分：从这些机制推出的安全观念

**出站方向是一个请求伪造，而且一点都不隐晦。** 实测，内网服务被直接到达、被发送方跟随的一次重定向再次到达、还被同一个地址的三种替代写法到达。**控制手段是关于"实际被使用的那个值"、而不是"被提供的那个文本"** —— 解析、校验解析出的地址、连到那个地址、不跟随重定向，并且优先用一个根本够不到内网的投递主机。

**而重定向那次实测是四次里最干净的一次**，因为它展示了在同一个请求里"被校验的地址"与"被联系的地址"不同。**实测"不跟随重定向"把它完全关上了**，这让它成为本篇最便宜的一条控制。

**入站方向是一个必须做认证的未认证端点。** 实测，对字节验签与对解析后的对象验签，在三个"人会说完全相同"的请求体里有两个给出了不同答案 —— 而重复键那组显示一串字节能给出两个值，取决于解析器。**所以"签名检查过了没有"与"应用据以行事的是什么"是两个问题，而这里正是它们分开的地方。**

**对字节验签，然后解析。只解析一次。** 两半都要紧，因为实测那个歧义在格式里、不在实现里，而任何一方解析器里的任何小心都去不掉它。

**重放在这里是正常流量。** 实测，一个只验签的接收方处理了同一份签过名的请求两次，而带时间戳与记录的接收方没有。**而且因为重试是设计进来的，幂等不是可选项** —— 接收方必须能处理同一个事件两次而只产生一个效果，无论那个重放防护做了什么。

**而来自订阅方的响应体是第三方内容。** 实测，其中一个里面有数据库名、用户名和一个令牌。**接收方选择用什么应答，就会是发送方日志里的内容**（如果它记录响应的话）—— 而那是每一方对另一方的数据所做的一个决定。

### 检测与缓解

- **对"解析后落在私有地址段"的 webhook 目标告警**，并且记录解析出来的地址而不是被提供的那个。实测，被提供的文本至少有同一个主机的四种写法。
- **对"跟随了重定向"的投递告警**，并把它当成一次配置错误或一次尝试。实测，跟随一次就从攻击者提供的地址到达了内网服务。
- **为每一次投递记录重定向次数与最终 URL**，因为实测那道缝就在"被校验的地址"与"被联系的地址"之间。
- **按订阅方盯验签失败**，因为一连串失败要么是一次坏掉的密钥轮换、要么是有人在猜。
- **盯同一个事件标识反复到达**，并按签名是否完全相同来区分"发送方重试了"与"有人重放了"。
- **如果投递响应被完整记录，就告警**，因为实测那个响应体里有数据库名和一个令牌。
- **缓解上，解析地址、按解析出的值拒绝私有段、并连到同一个值**；关掉重定向；限制只投递到标准端口；并且从一个没有通往内网路由的主机上投递。
- **对原始字节验签、之后再解析**，边界的一侧只有一个解析器，并用常数时间比较。
- **要求一个落在短窗口内的时间戳，并保留一份已处理签名的记录**，此外再加一个取自事件的幂等键。
- **把签名密钥当成凭据**：高熵、按订阅方分开、存到只有接收方能读、并且能在一段重叠期里轮换、而不是一刀切换。
- **在据以行事之前校验事件的形状与类型**，因为实测那个重复键的情形产生了一个发送方从未打算授权的值。
- **并且让投递主机比应用更受限**，因为对一个用户提供的地址发起的外呼，最强的控制是它够不到任何值得够的东西。
