---
id: api-authentication
title_en: API Authentication
title_zh: API 认证
summary_en: The credentials an API accepts, and the two separate questions they have to answer — who is calling, and whether this particular request is one they sent. Measured by building the schemes and attacking them — a verifier that trusts the token's own algorithm, the confusion that enables, a replay that succeeds, and where a token ends up depending on how it is sent.
summary_zh: API 接受的凭据，以及它们必须回答的两个不同问题 —— 谁在调用，以及这一个请求是不是他发的。这一篇把各种方案真的搭起来再攻一遍：一个信任令牌自带算法的校验器、由此产生的算法混淆、一次成功的重放，以及令牌放在哪里决定了它最终会出现在什么地方。
tags: [beginner, api, authentication, jwt, oauth]
tools: [python3, curl, openssl]
attck: [T1078, T1550]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Two questions, and only one of them is about identity

An API has to answer something a browser session does not. A session cookie is issued to a browser that keeps it and sends it back; an API may be called by a script, another service, a mobile application or a partner, over a connection nobody is watching.

**And the credentials have to answer two separate questions:**

**Who is calling** — authentication, the subject of most of the discussion.
**And whether this particular request is one they sent** — freshness, integrity, and non-replayability, which is where the failures actually are.

Measured below by building each scheme and attacking it: verifiers written both the wrong way and the right way, a forged token accepted by one of them, a signature replayed successfully, and the same token sent two ways leaving different traces.

### Part 1: the shape of it

| | Options |
|---|---|
| **who calls** | a person, a service, a device |
| **credential** | password, API key, bearer token, signed request, client certificate |
| **transport** | `Authorization` header, query string, cookie, request signature |
| **lifetime** | per request, minutes, days, until revoked, forever |
| **revocable** | centrally, by expiry, not at all |

**The choices are not independent**, and the combinations that go wrong are recognisable: a long-lived credential that cannot be revoked, a credential that travels in a place that gets logged, a credential that is valid for every endpoint because the API has no notion of scope.

**And authentication is not authorisation.** A credential says who; what they may do is a separate decision made per request. **Every entry in this series about access control assumes the first part is done**, and the failures here are usually about the credential itself rather than about the permissions attached to it.

### Part 2: HTTP Basic, and the lesson it teaches

Basic authentication puts a username and password in a header, encoded. Measured:

```
Authorization: Basic YXBwOk15UEBzc3cwcmQ=
decoded:             app:MyP@ssw0rd
```

**The encoding is reversible and the name says so** — it is Base64, and the only thing it provides is that the credentials survive transport if they contain characters a header cannot carry. Over TLS that is not nothing; without TLS it is a plaintext password with extra steps.

**And it has no concept of expiry.** The credentials are sent on **every request**, so:

> **The exposure of a Basic credential is the number of requests multiplied by the number of places a request is recorded.**

Every proxy, every access log, every debugging capture and every error report that includes headers is a copy. **Which is why the scheme is usable and why it should carry a credential that is not the user's password** — a scoped, short-lived token in the same header, which is what every modern API does instead.

### Part 3: bearer tokens, and what a JWT does not protect

A bearer token is a credential whose only requirement is possession: whoever holds it is treated as the subject. That makes its handling the entire security property.

**And the self-contained variant, the JWT, is read by anyone.** Measured, a token's two leading parts decode without any key:

```
header : {"alg":"HS256","typ":"JWT"}
payload: {"sub":"user1","role":"user","iat":1791514246,"exp":1791517846}
```

**The signature covers integrity, not confidentiality.**

> **A JWT is signed, so nobody can change it. It is not encrypted, so everybody can read it.**

**Which makes the payload the wrong place for anything that should not be visible** — an internal identifier the caller should not learn, an email address, a role the caller should not be able to enumerate, a claim that reveals another tenant's name. **The measured payload was readable with a base64 decode and no key**, and that is the whole test.

**What a JWT does buy is that the server does not have to look anything up.** The claims travel with the request, which is what makes it cheap at scale and what makes revocation the hard part: **a token that is valid until it expires is a decision already made**, and undoing it needs a list of revoked tokens — which is the lookup the format was designed to avoid.

**So the lifetime is the revocation mechanism**, and a long one is a decision to accept that a stolen token works until it expires.

### Part 4: the verifier is where the bugs are

Everything so far is a format. The vulnerability is in the code that checks it — measured, with the same five tokens offered to two verifiers, one that reads the algorithm from the token and one that pins it:

| Token | Verifier trusting the header | Verifier pinning the algorithm |
|---|---|---|
| valid, signed correctly | **accepted** | accepted |
| **`alg: none`, no signature at all** | **accepted** | **refused** — `alg none is not HS256` |
| **expired one hour ago** | **accepted** | **refused** — `expired` |
| **no `exp` claim** | **accepted** | **refused** — `no expiry` |
| signed with the wrong key | refused | refused |

**Four rows, and three of them are the same mistake**: the verifier did what the token said.

> **The `alg` field is in the token, and the token comes from the caller. A verifier that reads the algorithm from it is letting the caller choose how the caller is verified.**

**And the expiry rows show the other half**: checking a signature is not checking a token. The signature was perfectly valid on the expired token and on the one with no expiry at all. **A claim that is present but not examined is a claim that is not enforced** — and `exp` is only one of them, alongside `nbf` (not before), `aud` (who the token was issued for) and `iss` (who issued it).

**And the last row is the point of the exercise: the one thing a naive verifier does get right is the signature itself.** So a test suite that only checks "a token signed with the wrong key is rejected" passes on a verifier that will accept `alg: none`. **That is why the failure is common — it survives the obvious test.**

### Part 5: algorithm confusion, measured

The `alg: none` case is the famous one and the easy one to fix. The more interesting version is what happens when the same field has to describe two different things. Measured, with the server verifying RS256 signatures using a public key:

| Token | Verifier trusting the header | Verifier pinning RS256 |
|---|---|---|
| valid RS256, signed by the private key | accepted | accepted |
| **forged HS256, "signed" with the public key as the shared secret** | **accepted** | **refused** — `alg HS256 is not RS256` |

**And the forged token's payload was this:**

```
{"sub":"admin","role":"admin"}
```

**The mechanism is one sentence: a verifier that has a public key and is told the algorithm is `HS256` will use that public key as an HMAC secret.** The public key is public. Anyone can compute the same HMAC. **The algorithm the token claims must not be the thing that decides whether the key is a shared secret or a public key**, because those are different kinds of object and the choice between them is the verifier's, not the caller's.

**And that is why the fix is not "reject `none`".** Rejecting one algorithm value leaves the confusion in place; the fix is to **pin the algorithm in the verifier and select it by configuration, never by the token**, and to branch on the key type rather than on the claimed algorithm.

**Confirmed by measurement on the other side:** the pinned verifier refused the forged token for the right reason — not "bad signature", but "that is not the algorithm I use".

### Part 6: where the credential goes decides where it turns up

The same token, sent two ways, against a server that records what it sees. Measured:

| How it was sent | Where it appeared |
|---|---|
| `GET /api/me?api_key=TOKEN` | **in the request line** |
| `Authorization: Bearer TOKEN` | in the header; **the request line is clean** |
| a page linked from a URL containing the token | **in the `Referer` header of the next request** |

**The request line is what access logs, proxies, monitoring and error trackers record by default.** A token there is a token in every one of those places, and the measured `Referer` row shows the second-order effect: **a URL that contains a token becomes the `Referer` for anything linked from it**, so the credential travels to whatever the next request touches.

**A header, by contrast, is seen only by the endpoints that look at it** — the terminating server and anything deliberately reading headers. That is a smaller set, and it is why the `Authorization` header is the default recommendation for a bearer token.

**The third option is a cookie**, which the browser sends automatically, and which comes with its own controls: `HttpOnly` keeps it away from scripts, `Secure` keeps it off plaintext connections, and `SameSite` decides whether another site's request carries it. **The trade with a header is real**: a header is not sent automatically, so it is not exposed to cross-site requests, and it also means the client has to be written deliberately rather than just working.

### Part 7: signatures, replay, and one comparison

**A signature proves knowledge and integrity. It does not prove freshness.** Measured, a server that verifies an HMAC over method, path, body hash, timestamp and nonce:

| Server | First request | The same request replayed |
|---|---|---|
| verifies the signature only | **accepted** | **accepted — the replay worked** |
| timestamp window plus a record of nonces | accepted | **refused — `nonce already used`** |

**And the timestamp is a separate mechanism from the nonce.** Measured against the windowed server, a signature made an hour earlier was refused with `timestamp outside the window`, and a fresh signature with a used nonce was refused with `nonce already used`. **Neither check alone is enough**: a window without nonces allows a replay inside the window, and nonces without a window grow the record forever.

**What the signature covers is part of the design.** The measured canonical string included the method, the path, a hash of the body, the timestamp and the nonce — which means a signature cannot be moved to a different endpoint or a different body. **A signature over the body alone can be replayed to a different URL**; a signature over the URL alone cannot protect the body.

**And the comparison that decides whether a token matches is itself a measurable thing.** Measured, comparing a guess against a target, 20000 times per prefix length:

| Comparison | Prefix matching 0 / 8 / 16 / 24 / 32 bytes |
|---|---|
| character-by-character, returning at the first difference | **4.5 → 6.4 → 8.6 → 10.1 ms** |
| constant-time comparison | **1.1 ms flat** |

**The time rises with how much of the guess was right**, because the loop stops at the first wrong byte. The absolute numbers are small — this is a local loop, not a network — but the property is what matters: **a comparison whose duration depends on the secret is a comparison that leaks the secret a byte at a time**, and the fix is a language-provided constant-time comparison rather than a hand-written loop.

### Part 8: what follows for security

**The first conclusion is that the format is not the vulnerability.** Every measured failure in Part 4 and Part 5 was in code that did the right thing with the wrong assumption — that the caller's description of itself could be trusted.

**Pinning the algorithm, and validating every claim that matters, is the whole fix for the JWT family.** Measured, the pinned verifier refused `none`, an expired token, a token with no expiry and the confused-algorithm forgery — and the naive one accepted three of those four. **A token's claims are only enforced if they are examined**, and the list is short: algorithm by configuration, signature, `exp`, `nbf`, `aud`, `iss`, and whatever the application added.

**Second: a self-contained token is readable, so nothing secret can live in it.** The measured payload decoded without a key. **And a self-contained token cannot be revoked without a lookup**, so its lifetime is its revocation story — which makes "short access token plus a refresh token that is checked against a store" the arrangement that gets both properties.

**Third: where a credential is sent decides how many copies exist.** Measured, a token in the query string reached the request line and the next request's `Referer`. **Any place a full URL is recorded is a place that token is now stored** — logs, analytics, error trackers, browser history, and the `Referer` sent to third parties.

**Fourth: possession is the whole property for a bearer token.** There is no proof of who is holding it, which is why a stolen one is indistinguishable from a legitimate one, and why the controls are lifetime, scope, binding to a client where possible, and a revocation list that is consulted for anything sensitive.

**Fifth: a signature is not a replay defence.** Measured, the same signed request was accepted twice by a server that only verified the signature. **The two mechanisms that fix it are independent and both needed** — a timestamp window and a record of what has been used.

**And sixth: the comparison is part of the authentication.** Measured, the naive comparison took longer the more of the secret the guess matched. **On a network the signal is noisy and the attack is slow, which is exactly why it is usually dismissed and occasionally works** — and why the fix, a constant-time comparison, costs nothing.

### Detection and mitigation

- **Alert on tokens whose `alg` is not the expected one, and on tokens with no `exp`.** Both are measurable properties of a request, and the first is the signature of a confused verifier or an attack on one.
- **Watch for credentials in URLs.** A request line or a `Referer` containing `token=`, `api_key=` or `access_token=` is a copy of a credential in a log, and the count of those is a measurable exposure.
- **Log authentication failures with the reason, and alert on a rise in `expired` versus `signature`.** A spike in one means something different from a spike in the other, and an enumeration attempt looks different from a stolen-token attempt.
- **Count rejected replays separately.** A nonce or timestamp rejection is a request that was already seen, which is either a retry bug or an attack.
- **Keep the signing keys out of logs and error reports, and rotate them on a schedule with a named owner.**
- **For mitigation, pin the algorithm in the verifier and choose it by configuration**, branch on key type rather than on a claimed algorithm, and reject tokens that do not carry the claims the application needs.
- **Send credentials in the `Authorization` header**, or in a cookie with `HttpOnly`, `Secure` and an explicit `SameSite`. Do not accept them from a query string, and where a legacy client requires it, treat that as a finding with a deadline.
- **Keep access tokens short and pair them with a refresh mechanism that can be revoked**, so that the revocation story is not "wait for it to expire".
- **Put nothing in a token payload that the holder should not read.** Measured, it is a base64 decode away from readable, and the only thing the signature prevents is modification.
- **Sign the method, the path, the body and a timestamp**, not just one of them, and add a nonce with a store of what has been used, bounded by the same window.
- **Use a constant-time comparison for anything compared against a secret**, and do not write the loop by hand.
- **Separate authentication from authorisation in the code and in the review.** Who is calling and what they may do are two decisions, and a valid credential says nothing about the second.
- **Where services call each other, prefer a client certificate or a workload identity over a shared secret**, because a shared secret is a credential that has to be copied to every place that uses it.

<!-- lang:zh -->
### 两个问题，而只有一个关于身份

API 要回答一些浏览器会话不需要回答的东西。会话 cookie 发给一个会保存并回传它的浏览器；而 API 可能被脚本、另一个服务、一个手机应用或者一个合作方调用，连接那头没有人在看。

**而凭据必须回答两个不同的问题：**

**谁在调用** —— 认证，也是大部分讨论的主题。
**以及这一个请求是不是他发的** —— 新鲜度、完整性、不可重放，而失败其实在这里。

下面把每种方案真的搭起来再攻一遍：一个写错的和一个写对的校验器、一个被前者接受的伪造令牌、一次成功的签名重放，以及同一个令牌两种发法留下的不同痕迹。

### 第一部分：它的形状

| | 可选项 |
|---|---|
| **谁在调用** | 一个人、一个服务、一个设备 |
| **凭据** | 口令、API key、bearer 令牌、签名请求、客户端证书 |
| **传输** | `Authorization` 头、查询串、cookie、请求签名 |
| **有效期** | 每请求、几分钟、几天、直到撤销、永远 |
| **可撤销** | 集中撤销、靠过期、不能撤销 |

**这些选择不是彼此独立的**，而出问题的组合是可辨认的：一个长期有效且无法撤销的凭据、一个走在会被记录的地方的凭据、一个因为 API 没有作用域概念而对所有端点都有效的凭据。

**而认证不是授权。** 凭据说明"是谁"；他能做什么是一个按请求单独做的决定。**这个系列里每一篇关于访问控制的内容都假定第一件事已经做好了**，而这里的失败通常关于凭据本身，而不是附在它上面的权限。

### 第二部分：HTTP Basic，以及它示范的那一课

Basic 认证把一个用户名和口令放进一个头里，编码。实测：

```
Authorization: Basic YXBwOk15UEBzc3cwcmQ=
解码回来:          app:MyP@ssw0rd
```

**这个编码是可逆的，而名字就说明了这一点** —— 它是 Base64，它提供的唯一东西是：当口令里含有头装不下的字符时，凭据能活着到达。在 TLS 之上这不等于没有；没有 TLS 时它就是明码口令加了几步。

**而它没有过期的概念。** 凭据会**在每一个请求上**被发送，所以：

> **一个 Basic 凭据的暴露面，等于请求数乘以记录请求的地方的数量。**

每一个代理、每一份访问日志、每一次包含头的调试抓包、每一份包含头的错误报告，都是一份副本。**这就是为什么这个方案可用、也是为什么它应该携带一个不是用户口令的凭据** —— 一个限定作用域、短期有效的令牌放在同一个头里，也就是每个现代 API 实际在做的事。

### 第三部分：bearer 令牌，以及 JWT 不保护什么

bearer 令牌是一种唯一的持有条件就是"持有"的凭据：谁拿着它，就被当成那个主体。这让它的处理方式就是全部的安全属性。

**而自包含的那个变体，JWT，是任何人都能读的。** 实测，一个令牌的前两段不需要任何密钥就能解码：

```
头部  : {"alg":"HS256","typ":"JWT"}
载荷  : {"sub":"user1","role":"user","iat":1791514246,"exp":1791517846}
```

**签名覆盖的是完整性，不是机密性。**

> **JWT 是签名的，所以没人能改它。它没有加密，所以谁都能读它。**

**这就让载荷成为"任何不该被看到的东西"的错误去处** —— 一个调用者本不该知道的内部标识、一个邮箱地址、一个调用者本不该能枚举的角色、一个泄露了另一个租户名字的声明。**实测那个载荷是一次 base64 解码就能读的，不需要密钥**，而这就是全部的检验。

**JWT 换来的东西是服务端不必去查任何表。** 声明跟着请求走，这是它在大规模下便宜的原因，也是撤销成为难点的原因：**一个"到期前一直有效"的令牌，是一个已经做出的决定**，撤销它需要一份已撤销令牌的清单 —— 而那正是这种格式被设计出来要避免的那次查询。

**所以有效期就是撤销机制**，而一个很长的有效期，是一个"接受被偷的令牌在到期前一直能用"的决定。

### 第四部分：漏洞在校验器里

到目前为止的一切都是格式。漏洞在检查它的那段代码里 —— 实测，把同样的五个令牌交给两个校验器，一个从令牌里读算法、一个把算法钉住：

| 令牌 | 信任头部的校验器 | 钉住算法的校验器 |
|---|---|---|
| 有效、签名正确 | **通过** | 通过 |
| **`alg: none`，完全没有签名** | **通过** | **拒绝** —— `alg none is not HS256` |
| **一小时前就过期** | **通过** | **拒绝** —— `expired` |
| **没有 `exp` 声明** | **通过** | **拒绝** —— `no expiry` |
| 用错密钥签的 | 拒绝 | 拒绝 |

**四行，其中三行是同一个错误**：校验器照着令牌说的做了。

> **`alg` 字段在令牌里，而令牌来自调用者。一个从它里面读算法的校验器，是在让调用者选择调用者自己怎么被验证。**

**而过期那两行展示了另一半**：检查签名不等于检查令牌。那个过期令牌上的签名完全有效，那个根本没有有效期的令牌上的签名也一样。**一个存在但不被查看的声明，就是一个不被执行的声明** —— 而 `exp` 只是其中之一，与它并列的还有 `nbf`（不早于）、`aud`（这个令牌是签给谁的）和 `iss`（谁签的）。

**而最后一行是这次练习的意义所在：一个天真校验器唯一做对的事情，就是签名本身。** 所以一套只检查"用错密钥签的令牌会被拒绝"的测试，会让一个接受 `alg: none` 的校验器通过。**这就是为什么这个失败很常见 —— 它扛得过那个显而易见的测试。**

### 第五部分：算法混淆，实测

`alg: none` 那个是最有名的、也是最好修的。更有意思的版本，是当同一个字段不得不描述两件不同的事情时会发生什么。实测，服务端用公钥验证 RS256 签名：

| 令牌 | 信任头部的校验器 | 钉住 RS256 的校验器 |
|---|---|---|
| 合法的 RS256，由私钥签名 | 通过 | 通过 |
| **伪造的 HS256，"签名"用的是公钥本身作为共享密钥** | **通过** | **拒绝** —— `alg HS256 is not RS256` |

**而那个伪造令牌的载荷是这样的：**

```
{"sub":"admin","role":"admin"}
```

**机制一句话：一个手里有公钥、又被告诉算法是 `HS256` 的校验器，会拿那个公钥当 HMAC 的密钥。** 公钥是公开的，任何人都能算出同一个 HMAC。**令牌所声称的算法，绝不能成为决定"这把密钥是共享密钥还是公钥"的东西**，因为那是两类不同的对象，而在它们之间做选择是校验方的事、不是调用方的事。

**这就是为什么修法不是"拒绝 `none`"。** 拒绝一个算法取值，把混淆留在原处；修法是**在校验器里把算法钉住、由配置来选择，绝不由令牌来选择**，并按密钥类型分支、而不是按声称的算法分支。

**从另一侧也得到了实测印证：** 钉住的校验器拒绝了那个伪造令牌，而且理由是对的 —— 不是"签名不对"，而是"那不是我用的算法"。

### 第六部分：凭据放在哪里，决定它出现在哪里

同一个令牌，两种发法，对着一个记录自己所见的服务端。实测：

| 怎么发的 | 它出现在哪 |
|---|---|
| `GET /api/me?api_key=TOKEN` | **在请求行里** |
| `Authorization: Bearer TOKEN` | 在头里；**请求行是干净的** |
| 从一个含令牌的 URL 链接出去的页面 | **在下一个请求的 `Referer` 头里** |

**请求行正是访问日志、代理、监控与错误追踪默认记录的东西。** 令牌在那里，就是令牌在那些地方的每一处；而实测的 `Referer` 那一行展示了二阶效应：**一个含令牌的 URL 会成为从它链接出去的任何东西的 `Referer`**，于是那个凭据会随着下一个请求走到它所触及的地方。

**相比之下，头只被那些会去看它的环节看到** —— 终结请求的服务端，以及任何有意读头的程序。那是更小的一批，也是 `Authorization` 头成为 bearer 令牌默认建议的原因。

**第三个选项是 cookie**，浏览器会自动发送它，而它自带一些控制点：`HttpOnly` 让它远离脚本、`Secure` 让它不出现在明文连接上、`SameSite` 决定另一个站点的请求会不会带上它。**与头之间的取舍是真实的**：头不会被自动发送，所以它不会暴露给跨站请求，而这也意味着客户端必须被有意地写好、而不是"自然就能用"。

### 第七部分：签名、重放，以及一次比较

**签名证明的是"知道密钥"和"内容完整"。它不证明新鲜。** 实测，一个对方法、路径、body 哈希、时间戳与 nonce 做 HMAC 校验的服务端：

| 服务端 | 第一次请求 | 同一份请求重放 |
|---|---|---|
| 只验签名 | **接受** | **接受 —— 重放成功了** |
| 时间戳窗口加 nonce 记录 | 接受 | **拒绝 —— `nonce already used`** |

**而时间戳是与 nonce 分开的另一个机制。** 实测，对一个带窗口的服务端，一小时前做的签名被以 `timestamp outside the window` 拒绝，而一个新鲜签名配一个用过的 nonce 被以 `nonce already used` 拒绝。**两个检查单靠任何一个都不够**：有窗口没 nonce，窗口内的重放会成功；有 nonce 没窗口，那份记录会永远长大。

**签名覆盖了什么，是设计的一部分。** 实测那个规范化字符串包含方法、路径、body 的哈希、时间戳与 nonce —— 这意味着一个签名不能被挪到另一个端点或者另一个 body 上。**只覆盖 body 的签名可以被重放到另一个 URL**；只覆盖 URL 的签名保护不了 body。

**而"这个令牌是否匹配"所依赖的那次比较，本身也是可测量的。** 实测，把猜测与目标比较、每个前缀长度做两万次：

| 比较方式 | 匹配前缀 0 / 8 / 16 / 24 / 32 字节 |
|---|---|
| 逐字符、遇到不同就返回 | **4.5 → 6.4 → 8.6 → 10.1 ms** |
| 常数时间比较 | **恒为 1.1 ms** |

**耗时随猜测中正确的部分变长而上升**，因为循环在第一个不同的字节就停了。绝对数字很小 —— 这是一个本地循环、不是一次网络请求 —— 但要紧的是那个性质：**一个耗时取决于秘密的比较，就是一个一次泄露秘密一个字节的比较**，而修法是语言提供的常数时间比较，不是手写的循环。

### 第八部分：从这些机制推出的安全观念

**第一个结论是：格式不是漏洞。** 第四、第五部分里每一次实测到的失败，都发生在"做了正确的事、但基于一个错误假设"的代码里 —— 那个假设是"调用者对自己的描述可以被信任"。

**把算法钉住、并把每一个要紧的声明都校验，就是 JWT 这一族的全部修法。** 实测，钉住的校验器拒绝了 `none`、一个过期令牌、一个没有有效期的令牌、以及那个算法混淆的伪造品 —— 而天真的那个接受了其中三个。**一个令牌的声明只有在被查看时才被执行**，而那份清单很短：由配置决定的算法、签名、`exp`、`nbf`、`aud`、`iss`，以及应用自己加上的东西。

**第二：自包含的令牌是可读的，所以里面不能住任何秘密。** 实测那个载荷不需要密钥就能解码。**而一个自包含的令牌不做一次查询就无法撤销**，所以它的有效期就是它的撤销故事 —— 这让"短期访问令牌 + 一个要去存储里核对的刷新令牌"成为同时拿到两种性质的那个安排。

**第三：凭据发往哪里，决定了存在多少份副本。** 实测，放在查询串里的令牌到达了请求行、也到达了下一个请求的 `Referer`。**任何记录完整 URL 的地方，都是那个令牌现在被存下来的地方** —— 日志、分析、错误追踪、浏览器历史，以及发给第三方的 `Referer`。

**第四：对 bearer 令牌来说，"持有"就是全部属性。** 没有任何东西证明是谁在拿着它，这就是为什么被偷的那个与合法的那个不可区分，也是为什么控制手段是有效期、作用域、在可能时绑定到客户端，以及一份对敏感操作必须去查的撤销清单。

**第五：签名不是重放防护。** 实测，同一份签过名的请求，在一个只验签名的服务端上被接受了两次。**修好它的两个机制彼此独立、而且都需要** —— 一个时间戳窗口，加一份用过什么的记录。

**第六：比较也是认证的一部分。** 实测，那个天真的比较，在猜测与秘密匹配得越多时耗时越长。**在网络上这个信号很嘈杂、攻击很慢，而这也正是它通常被忽视、偶尔却成功的原因** —— 也是为什么那个修法（常数时间比较）不花任何代价。

### 检测与缓解

- **对 `alg` 不是预期的令牌、以及没有 `exp` 的令牌告警。** 两者都是请求上可测的属性，而前者是一个被混淆的校验器、或者对它的攻击的签名。
- **盯 URL 里的凭据。** 一条请求行或者一个 `Referer` 里含 `token=`、`api_key=`、`access_token=`，就是一份在日志里的凭据副本，而那些的数量就是一个可测量的暴露面。
- **把认证失败连同原因一起记，并对 `expired` 与 `signature` 的上升分别告警。** 两者上升意味着不同的事，而一次枚举尝试与一次被偷令牌的尝试看起来也不一样。
- **把被拒绝的重放单独计数。** 一次 nonce 或时间戳的拒绝，是一个已经被见过的请求，它要么是一个重试 bug、要么是一次攻击。
- **让签名密钥远离日志与错误报告，并按计划轮换、且有明确的负责人。**
- **缓解上，在校验器里钉住算法、由配置来选择**，按密钥类型分支而不是按声称的算法分支，并拒绝那些不携带应用所需声明的令牌。
- **把凭据放在 `Authorization` 头里**，或者放在一个带 `HttpOnly`、`Secure` 和明确 `SameSite` 的 cookie 里。不要从查询串接受它们，而在一个遗留客户端要求如此时，把它当成一条有期限的发现。
- **让访问令牌短，并配一个可以撤销的刷新机制**，这样撤销的故事就不是"等它过期"。
- **令牌载荷里不要放持有者不该读到的东西。** 实测，它离可读只差一次 base64 解码，而签名唯一阻止的是修改。
- **把方法、路径、body 与时间戳一起签**，而不是只签其中之一，并加一个 nonce 加一份用过什么的存储，由同一个窗口来限定。
- **任何与秘密作比较的地方都用常数时间比较**，不要手写那个循环。
- **在代码里和评审里把认证与授权分开。** 谁在调用、以及他能做什么是两个决定，而一个有效的凭据对第二个什么都没说。
- **在服务之间互相调用时，优先用客户端证书或者工作负载身份，而不是共享密钥**，因为一个共享密钥是一个必须被复制到每一个使用它的地方的凭据。
