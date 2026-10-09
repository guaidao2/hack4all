---
id: browser-same-origin-policy
title_en: The Same-Origin Policy
title_zh: 浏览器的同源策略
summary_en: The most basic rule in the browser, and it is a rule about reading rather than sending. Measured in a real browser across eleven cross-origin attempts against one server with two host names — what gets blocked, what merely becomes unreadable, and what the policy has nothing to do with.
summary_zh: 浏览器里最基础的一条规则，而它是一条关于"读取"、不是关于"发送"的规则。这一篇在真实浏览器里实测 11 次跨源尝试（同一个服务器、两个 host 名）—— 什么被挡住、什么只是变得读不到、以及哪些事跟这条规则毫无关系。
tags: [beginner, web, browser, same-origin, cors, csrf, cookies]
tools: [chrome, curl, python3, browser devtools]
attck: [T1185]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The rule, in one sentence

A browser is in an unusual position: it is the only program that reaches websites **carrying the user's credentials automatically**, and it runs code **from every site the user visits**. Both at once.

> The same-origin policy is the rule that decides **which page may read the response from which site**. Everything else in browser security — CORS, CSRF tokens, `postMessage` checks, the WebSocket `Origin` check — exists in relation to it.

**And the most useful thing to know about it up front is that it governs reading, not sending.** Almost every misunderstanding in this area comes from expecting it to prevent a request that it has no intention of preventing.

The measurements below are from a real browser, with one server listening on one port and answering to two host names — `localhost` and `127.0.0.1`, which the browser treats as **different origins** despite being the same machine and the same process. That is the whole setup a demonstration needs.

### Part 1: why it has to exist

Remove the rule and the situation is this: you are logged into your bank in one tab. You open an attacker's page in another. That page issues a request to the bank — and the browser, being helpful, attaches your session cookie. Without a same-origin policy the page can also **read the answer**, which is your balance, your statement, your account list.

**The rule is what stands between "the browser will fetch this for you" and "any page can read anything the browser can fetch".**

Two things are worth being precise about, because both get muddled:

| Confusion | The actual situation |
|---|---|
| "It protects the server" | It protects **the user's other content** from the page. The server is protected by its own checks. |
| "It stops cross-origin requests" | It stops cross-origin **reads**. Requests go out. |

### Part 2: how an origin is computed

An origin is a triple — **scheme, host, port** — and all three have to match.

| Pair | Same origin? | Why |
|---|---|---|
| `http://localhost:9050` and `http://127.0.0.1:9050` | **no** | different host — **the measured setup** |
| `http://example.com` and `http://example.com:80` | yes | 80 is the default for `http` |
| `http://EXAMPLE.com` and `http://example.com` | yes | host names are case-insensitive |
| `http://example.com` and `https://example.com` | **no** | different scheme |
| `http://example.com:80` and `http://example.com:8080` | **no** | different port |

**And the measured case is the instructive one.** `localhost` and `127.0.0.1` resolve to the same address, reach the same process, and are answered by the same code — and they are two origins. **Nothing about the server being the same matters**, because the policy is about names, not about where they lead.

A few details that come up in practice:

| Case | Behaviour |
|---|---|
| `file://` URLs | treated inconsistently between browsers; a local file may be able to read another |
| sandboxed iframes, `data:` URLs | get an **opaque origin** that matches nothing, not even itself |
| `document.domain = …` | **deprecated** — it used to be able to pull two subdomains into one origin, and that relaxation caused real bugs |
| redirects | an origin is where the response finally came from, not where the request started |

**"It looks like the same site" is not the test.** The test is whether the triple computes equal — which is why an application spread over `www`, `api` and `static` host names is three origins that have to be told about each other.

### Part 3: what it blocks, and what it merely makes unreadable

Eleven cross-origin attempts from a page on `http://localhost:9050` against `http://127.0.0.1:9050`, measured:

| Attempt | Result |
|---|---|
| `fetch()` a plain endpoint | **`TypeError: Failed to fetch`** — blocked by CORS policy |
| `fetch(..., {mode:'no-cors'})` | **`type=opaque status=0`** — the request went; the response cannot be read |
| `fetch()` an endpoint sending `Access-Control-Allow-Origin: *` | **read the body successfully** |
| an `<img>` pointing at the other origin | **loaded** — `1x1` |
| `canvas.getImageData()` after drawing that image | **`SecurityError: The canvas has been tainted by cross-origin data`** |
| a `<script src>` pointing at the other origin | **loaded and executed** |
| `fetch(..., {credentials:'include'})` where the server permits it | the **server received the request** — `Origin: http://localhost:9050`, `Cookie: (none)` |
| the same fetch where the server does not permit it | `TypeError: Failed to fetch` |
| a form POST to the other origin | **submitted** — the server logged it; this page cannot read the reply |
| `window.open()` then reading `.document` | **`SecurityError: Blocked a frame with origin … from accessing a cross-origin frame`** |
| an `<iframe>` and reading `.contentDocument` | **`null`** |

**The blocked row is the one people know about.** The rest is the part that matters:

**The request goes out anyway.** `mode: 'no-cors'` returns a response object with `type=opaque` and `status=0` — the browser made the request, the server answered it, and the page is handed a shape it cannot read anything out of. **For "fire and forget" traffic like a beacon that is fine; for anything else it is a request whose answer is unavailable.**

**A `<script>` is not blocked, and it runs.** It loaded and set a global variable from the other origin, which is how JSONP worked for years and why a `<script src>` to an attacker-controlled host is game over.

**An image loads and its pixels do not.** The `<img>` element is unaffected, and the canvas is **tainted** by the cross-origin draw, so reading the pixels back throws. That is the compromise: display is allowed, extraction is not.

**A form can post across origins and its answer is unreadable.** The server's log line shows the request arrived, with a real browser-set `Origin` and the form fields:

```
[server] POST /form from Origin=http://localhost:9050 body='note=sent+from+http%3A%2F%2Flocalhost%3A9050'
```

> **The policy governs reading, not sending.** A cross-origin request still goes out, still reaches the server, and still takes effect — the page that sent it simply cannot see what came back.

**And two conclusions follow directly from that asymmetry**: a state-changing request that needs no reply can be triggered from another origin (**CSRF**), and an origin that *should* be able to read a response needs the server to say so (**CORS**).

### Part 4: what the same-origin policy does not govern

A long list, and each row is the entrance to a different class of problem:

| Not governed by it | Governed by |
|---|---|
| whether a **cookie is attached** | the cookie's own rules — `Domain`, `Path`, `SameSite` |
| whether the **request is sent** | nothing, essentially |
| **navigation** (`window.location = …`) | allowed across origins |
| **form submission** | allowed across origins |
| **loading** images, scripts, stylesheets, fonts | allowed across origins |
| **WebSocket connections** | not restricted by it — the server checks `Origin` |
| **DNS rebinding** | nothing at this layer — see Part 7 |

**The cookie row deserves the measurement.** The request where the server explicitly permits credentials arrived, with the browser-set origin, and the server reported:

```
target saw Cookie: (none) | Origin: http://localhost:9050
```

**The request arrived and the cookie did not.** The reason has nothing to do with origins: the cookie in this test was set with `SameSite=Lax`, and a cross-site request does not carry a `Lax` cookie. **Two independent systems, and only one of them was involved.**

> **Whether a cookie rides along is decided by the cookie's attributes, not by the same-origin policy.** A cookie without `SameSite` protection, on a request that is cross-site, is what the CSRF entry is about.

### Part 5: the channels, and who has to consent

Every cross-origin read that works is one where **someone with authority said yes**:

| Channel | Can read? | Who has to agree |
|---|---|---|
| `fetch` / `XHR` | only with CORS headers | **the server** |
| `fetch(mode:'no-cors')` | never | — |
| `<img>` | cannot read pixels | — |
| `<script src>` | executes; cannot read the text | nothing — **which is why it is dangerous** |
| `<iframe>`, `window.open` | cannot read the DOM | via `postMessage`, **the receiver** |
| form POST | cannot read the reply | — |
| `postMessage` | yes | **the receiving page, by checking `event.origin`** |
| WebSocket | yes | **the server, by checking `Origin` at the handshake** |
| CORS-enabled `fetch` | yes | **the server, by naming the origin** |

**Read the last four rows together and the shape is clear.** When the browser will not enforce the boundary, the boundary has to be enforced by a human decision on the other side — a `postMessage` handler checking where a message came from, a WebSocket server checking the handshake, an HTTP server naming the origins it trusts.

**And every row that says "cannot read" is a CSRF candidate**, because a CSRF attack does not need to read anything.

### Part 6: CORS is the server opening a door

CORS is frequently described as the browser relaxing the same-origin policy. It is the opposite: **it is a mechanism for a server to state which origins may read its responses**, and the browser enforces that statement.

Measured, on the same two origins, the whole difference is one response header:

| The endpoint | What the page got |
|---|---|
| no CORS header | **`TypeError: Failed to fetch`**, and in the console: `has been blocked by CORS policy: No 'Access-Control-Allow-Origin' header is present on the requested resource` |
| `Access-Control-Allow-Origin: *` | the body, read normally |

**Two details decide whether a CORS setup is safe:**

**The wildcard cannot be combined with credentials.** `Access-Control-Allow-Origin: *` means "any origin may read this", and allowing that *with* cookies would mean any site could read the user's data from this server. So the browser refuses the combination: a credentialed request needs a **specific origin** in the header plus `Access-Control-Allow-Credentials: true`. The measured credentialed endpoint does exactly that, which is why the request went through and the cookie question became the cookie's own.

**`Origin` is set by the browser and cannot be set by a page.** That is what makes it usable as a check — and also why a CORS configuration that reflects whatever `Origin` it received is equivalent to `*`, just written in a way that looks deliberate.

**A request that is not "simple" is preflighted.** A method other than `GET`/`HEAD`/`POST`, or a custom header, causes the browser to send an `OPTIONS` request first and wait for permission. That is why a CORS problem sometimes looks like a missing `OPTIONS` handler on the server.

### Part 7: three things that follow

**CSRF exists because sending is allowed.** A cross-origin page can cause a request that changes state, and it cannot read the answer — and for a state change, not reading the answer is fine. **The same-origin policy is not a defence against CSRF**; it never was, because the attack does not need to read anything.

**XSS is severe because it happens inside the origin.** An injected script runs on the victim site's own origin, so the policy says it is legitimate code there: it can read the DOM, read the responses, and use the session. **Same-origin protection is what an XSS gets to stand behind**, which is why the boundary worth defending is the one that keeps script out.

**DNS rebinding gets around the policy by changing what a name means.** The origin is computed from the **host name**, and a host name is resolved by whoever answers DNS. An attacker who controls a name can serve the attacker's page from it, then re-point it at an internal address — and the browser, still seeing the same host name, considers the internal device to be the **same origin** and lets the page read it.

| What that means in practice |
|---|
| An internal service that is not reachable from the internet is still reachable from a browser on the internal network |
| "It is on a private address, so nobody can reach it" is not a security control |
| Host validation on the internal service is what actually stops it, since the request arrives with the attacker's host name |

### Part 8: what follows for security

**Everything here reduces to one sentence, and it is the one to carry:**

> **The same-origin policy is a rule about reading, enforced in the user's browser, and it exists to protect the user's other content. It was never a defence for your server.**

Four consequences, in the order an application meets them:

**A request arriving is not evidence of anything.** The browser will send a cross-origin request and the server will process it; the measured form POST arrived with a correct `Origin` header and no involvement from the user beyond loading a page. Whatever the endpoint does, it has to decide for itself whether the request should be honoured.

**Where the browser will not enforce a boundary, the receiving side must.** `postMessage` without an origin check, a WebSocket handshake without an `Origin` check, and a CORS configuration that reflects the caller's origin are three versions of "trusting whoever arrives".

**Cookies need their own decisions.** `SameSite` is the setting that decides whether a cookie rides along on a cross-site request, and the measured result — request arrives, cookie absent — is that setting doing its job. It is a separate layer from the same-origin policy and it is the one that speaks about cookies.

**And the internal network is not exempt.** DNS rebinding means a browser can be pointed at internal services; the policy will not stop it, because as far as the browser is concerned the origin is the attacker's own name.

### Detection and mitigation

- **Audit the CORS configuration, and alert on the two forms that give everything away.** A literal `Access-Control-Allow-Origin: *` on an endpoint that also carries credentials, and a header that echoes the request's `Origin` value, are both readable from the response and both mean "any origin may read this".
- **Use the fetch metadata headers, which the browser sets and a page cannot.** `Sec-Fetch-Site: cross-site` on a request to an endpoint that is only ever called from your own pages, `Sec-Fetch-Mode`, and `Sec-Fetch-Dest` are values a server can compare rather than guess — and they catch the attempt even when the response is blocked.
- **Log the `Origin` on state-changing requests.** The measured server saw `Origin: http://localhost:9050` on a cross-origin form post; an application that records that field has the evidence, and one that does not has a request that looks ordinary.
- **Alert on `postMessage` with a wildcard target and on message handlers with no origin check.** Both are mechanically findable in the source, and both are statements about a missing control rather than a heuristic.
- **For mitigation, list the origins you trust rather than reflecting what arrives.** A specific `Access-Control-Allow-Origin` plus `Access-Control-Allow-Credentials: true` where credentials are needed; no wildcard anywhere credentials could be involved.
- **Set `SameSite` on session cookies and understand which value you chose.** `Lax` is a sensible default and it is what kept the cookie off the measured cross-origin request; `None` explicitly re-opens the cross-site case and requires `Secure`.
- **Protect state-changing endpoints with a token that a cross-origin page cannot produce.** The CSRF entry's control, and the one thing that holds regardless of which cross-origin channel is used.
- **Validate `Origin` where the browser will not do it for you — WebSocket handshakes and `postMessage` receivers.** These are the two places where the policy is absent by design, so the check is the entire control.
- **Send `X-Content-Type-Options: nosniff`, and do not serve user content from a host that also serves the application.** A response that a browser is willing to treat as a script is a cross-origin read primitive, which is why JSONP-shaped endpoints and permissive content types matter.
- **Do not treat a private network address as protection.** Validate the `Host` header on internal services, and require authentication on anything a browser on the internal network can reach.
- **And keep the framing in mind when reading anything else about browser security.** The browser gave the user a set of rules; every one of them is enforced in the browser, on the user's machine, in code the user does not control. **What a server can rely on is what the server checks itself.**

<!-- lang:zh -->
### 一句话说这条规则

浏览器处在一个不寻常的位置：它是唯一一个**自动带着用户凭据**去访问网站的程序，同时又**运行着用户访问过的每一个站点送来的代码**。两件事同时成立。

> 同源策略就是那条决定**哪个页面可以读哪个站点的回应**的规则。浏览器安全里其他的一切 —— CORS、CSRF token、`postMessage` 的来源检查、WebSocket 的 `Origin` 检查 —— 都是相对于它而存在的。

**而关于它，最先要知道的一件事是：它管的是读取，不是发送。** 这个领域里几乎所有的误解，都来自指望它去阻止一个它从一开始就没打算阻止的请求。

下面的实测来自一个真实浏览器：一个服务器、监听一个端口、对两个 host 名作出应答 —— `localhost` 与 `127.0.0.1`，而浏览器把它们当成**两个不同的源**，尽管它们是同一台机器、同一个进程。做演示需要的全部准备就是这些。

### 第一部分：它为什么必须存在

把这条规则拿掉，情况是这样的：你在一个标签页里登录着银行，在另一个标签页里打开了攻击者的页面。那个页面向银行发一个请求 —— 而浏览器很热心地附上了你的会话 cookie。没有同源策略的话，那个页面还能**读到回应**，而那是你的余额、你的对账单、你的账户列表。

**这条规则就是拦在"浏览器会替你取这个"和"任何页面都能读浏览器能取到的一切"之间的东西。**

有两件事值得说准，因为两件都常被搞混：

| 常见的混淆 | 实际情况 |
|---|---|
| "它保护服务器" | 它保护的是**用户的其他内容**不被那个页面读。服务器由它自己的检查保护。 |
| "它阻止跨源请求" | 它阻止的是跨源**读取**。请求照样发出去。 |

### 第二部分：一个源是怎么算出来的

一个源是一个三元组 —— **scheme、host、port** —— 三者必须完全一致。

| 这一对 | 同源吗 | 为什么 |
|---|---|---|
| `http://localhost:9050` 与 `http://127.0.0.1:9050` | **不同源** | host 不同 —— **这就是实测里的那对** |
| `http://example.com` 与 `http://example.com:80` | 同源 | 80 是 `http` 的默认端口 |
| `http://EXAMPLE.com` 与 `http://example.com` | 同源 | host 名大小写不敏感 |
| `http://example.com` 与 `https://example.com` | **不同源** | scheme 不同 |
| `http://example.com:80` 与 `http://example.com:8080` | **不同源** | port 不同 |

**而实测里那一对才是最有教益的。** `localhost` 和 `127.0.0.1` 解析到同一个地址、到达同一个进程、由同一段代码应答 —— 而它们是两个源。**服务器是同一个这件事完全不重要**，因为这条策略说的是名字，不是名字指向哪里。

几个实务里会碰到的细节：

| 情形 | 行为 |
|---|---|
| `file://` URL | 各浏览器处理不一致；一个本地文件可能读得到另一个 |
| 沙箱 iframe、`data:` URL | 拿到一个**不透明源**，它谁也不匹配，连自己都不匹配 |
| `document.domain = …` | **已废弃** —— 它曾经能把两个子域拉进同一个源，而那个放宽造成过真实的漏洞 |
| 重定向 | 源看的是回应最终来自哪里，不是请求从哪里开始 |

**"看起来像同一个站点"不是判据。** 判据是那个三元组算出来是否相等 —— 这就是为什么一个分布在 `www`、`api`、`static` 上的应用是三个源，而它们得互相告知。

### 第三部分：它挡住什么，以及它只是让什么变得读不到

从 `http://localhost:9050` 上的一个页面向 `http://127.0.0.1:9050` 做的十一次跨源尝试，实测：

| 尝试 | 结果 |
|---|---|
| `fetch()` 一个普通端点 | **`TypeError: Failed to fetch`** —— 被 CORS 策略挡住 |
| `fetch(..., {mode:'no-cors'})` | **`type=opaque status=0`** —— 请求发出去了，回应读不到 |
| `fetch()` 一个带 `Access-Control-Allow-Origin: *` 的端点 | **成功读到正文** |
| 一个指向另一个源的 `<img>` | **加载成功** —— `1x1` |
| 画完那张图之后 `canvas.getImageData()` | **`SecurityError: The canvas has been tainted by cross-origin data`** |
| 一个指向另一个源的 `<script src>` | **加载并执行了** |
| 服务端允许凭据时的 `fetch(..., {credentials:'include'})` | **服务端收到了请求** —— `Origin: http://localhost:9050`、`Cookie: (none)` |
| 服务端不允许时的同一个 fetch | `TypeError: Failed to fetch` |
| 向另一个源的表单 POST | **提交成功** —— 服务端记到了日志；这个页面读不到回应 |
| `window.open()` 之后读 `.document` | **`SecurityError: Blocked a frame with origin … from accessing a cross-origin frame`** |
| 一个 `<iframe>`，读它的 `.contentDocument` | **`null`** |

**被挡住的那一行是大家都知道的那个。** 其余的才是要紧的：

**请求照样发出去了。** `mode: 'no-cors'` 返回一个 `type=opaque`、`status=0` 的响应对象 —— 浏览器发了那个请求，服务端答了它，而页面拿到一个读不出任何东西的形状。**对"发出去就行"的流量（比如打点）这样可以；对别的东西，那是一个拿不到答案的请求。**

**`<script>` 不被挡，而且它会执行。** 它加载了、并从另一个源设置了一个全局变量 —— 这就是 JSONP 多年来的原理，也是为什么"往攻击者控制的 host 加一个 `<script src>`"等于全盘失守。

**图片能加载，像素读不到。** `<img>` 元素不受影响，而 canvas 被那次跨源绘制**污染**了，于是把像素读回来会抛错。这是那个折中：允许显示，不允许提取。

**表单向可以跨源提交，而它的回应读不到。** 服务端的日志行显示请求到达了，带着浏览器设置的真实 `Origin` 和表单字段：

```
[server] POST /form from Origin=http://localhost:9050 body='note=sent+from+http%3A%2F%2Flocalhost%3A9050'
```

> **这条策略管的是读取，不是发送。** 一次跨源请求照样发出去、照样到达服务端、照样生效 —— 只是发出它的那个页面看不到回来的东西。

**而两个结论直接从这个不对称里长出来**：一次不需要回应的状态变更可以被另一个源触发（**CSRF**）；而一个**本该**能读回应的源，需要服务端明确说可以（**CORS**）。

### 第四部分：同源策略不管什么

一份很长的清单，而每一行都是另一类问题的入口：

| 不归它管 | 归谁管 |
|---|---|
| **cookie 带不带** | cookie 自己的规则 —— `Domain`、`Path`、`SameSite` |
| **请求发不发得出去** | 基本上没有东西管 |
| **跳转**（`window.location = …`） | 允许跨源 |
| **表单提交** | 允许跨源 |
| **加载**图片、脚本、样式、字体 | 允许跨源 |
| **WebSocket 连接** | 不受它限制 —— 由服务端检查 `Origin` |
| **DNS 重绑定** | 这一层没有东西管 —— 见第七部分 |

**cookie 那一行值得看实测。** 服务端明确允许凭据的那次请求到达了，带着浏览器设置的来源，而服务端报告：

```
target saw Cookie: (none) | Origin: http://localhost:9050
```

**请求到了，cookie 没到。** 原因和源没有关系：这次测试里那个 cookie 是用 `SameSite=Lax` 设置的，而一次跨站请求不会带上 `Lax` 的 cookie。**两套彼此独立的系统，而只有一套参与了。**

> **一个 cookie 跟不跟着走，由那个 cookie 的属性决定，不由同源策略决定。** 一个没有 `SameSite` 保护的 cookie，配上一次跨站请求，就是 CSRF 那篇在讲的东西。

### 第五部分：那些通道，以及谁必须同意

每一次能成功的跨源读取，都是**某个有权限的一方说了可以**：

| 通道 | 能读吗 | 谁必须同意 |
|---|---|---|
| `fetch` / `XHR` | 只有带 CORS 头才能 | **服务端** |
| `fetch(mode:'no-cors')` | 永远不能 | —— |
| `<img>` | 读不到像素 | —— |
| `<script src>` | 会执行；读不到源码文本 | 没人 —— **这就是它危险的原因** |
| `<iframe>`、`window.open` | 读不到 DOM | 经由 `postMessage`，**接收方** |
| 表单 POST | 读不到回应 | —— |
| `postMessage` | 能 | **接收页面，通过检查 `event.origin`** |
| WebSocket | 能 | **服务端，在握手时检查 `Origin`** |
| 带 CORS 的 `fetch` | 能 | **服务端，通过点名那个源** |

**把最后四行一起读，形状就清楚了。** 当浏览器不肯执行那条边界时，边界就必须由另一端的一个人的决定来执行 —— 一个检查消息从哪来的 `postMessage` 处理器、一个检查握手的 WebSocket 服务端、一个点名它信任哪些源的 HTTP 服务端。

**而每一行写着"读不到"的，都是一个 CSRF 候选**，因为 CSRF 攻击不需要读任何东西。

### 第六部分：CORS 是服务端开的一扇门

CORS 常被描述成"浏览器放宽了同源策略"。它是反过来的：**它是服务端用来声明"哪些源可以读我的回应"的机制**，而浏览器执行那个声明。

实测，在同样两个源之间，全部差别就是一个响应头：

| 那个端点 | 页面得到了什么 |
|---|---|
| 没有 CORS 头 | **`TypeError: Failed to fetch`**，控制台里是 `has been blocked by CORS policy: No 'Access-Control-Allow-Origin' header is present on the requested resource` |
| `Access-Control-Allow-Origin: *` | 正常读到了正文 |

**有两个细节决定一份 CORS 配置安不安全：**

**通配符不能和凭据一起用。** `Access-Control-Allow-Origin: *` 的意思是"任何源都可以读这个"，而允许它**同时**带 cookie，就意味着任何站点都能读到这个服务器上属于用户的数据。所以浏览器拒绝这个组合：带凭据的请求需要头里是**一个具体的源**，外加 `Access-Control-Allow-Credentials: true`。实测里那个带凭据的端点正是这么做的，这也是为什么请求通过了、而 cookie 的问题变成了 cookie 自己的问题。

**`Origin` 由浏览器设置，页面设置不了。** 这正是它可以用来做检查的原因 —— 也正是一份"把收到的 `Origin` 原样反射回去"的 CORS 配置等价于 `*` 的原因，只是它写得看起来像经过考虑。

**不是"简单请求"的请求会先被预检。** 方法不是 `GET`/`HEAD`/`POST`，或者带了自定义头，浏览器就会先发一个 `OPTIONS` 请求并等许可。这就是为什么一个 CORS 问题有时看起来像是服务端少了一个 `OPTIONS` 处理器。

### 第七部分：三个推论

**CSRF 之所以存在，是因为发送是被允许的。** 一个跨源页面可以造成一次改变状态的请求，而它读不到回应 —— 对一次状态变更来说，读不到回应完全没关系。**同源策略不是对 CSRF 的防御**；它从来不是，因为这个攻击不需要读任何东西。

**XSS 之所以严重，是因为它发生在那个源里面。** 一个被注入的脚本跑在受害站自己的源上，于是策略说它在那里是合法代码：它能读 DOM、能读回应、能用那个会话。**同源保护正是 XSS 得以栖身的东西**，这就是为什么真正值得守的边界是把脚本挡在外面的那条。

**DNS 重绑定绕过这条策略的办法，是改变一个名字的含义。** 源是按 **host 名**算出来的，而 host 名由应答 DNS 的那一方解析。一个控制了某个名字的攻击者可以先从它那里提供攻击者的页面，再把那个名字重新指向一个内网地址 —— 而浏览器仍然看到同一个 host 名，就认为那台内网设备与它**同源**，于是让页面去读它。

| 这在实务里意味着什么 |
|---|
| 一个从公网不可达的内部服务，对**内网里的浏览器**仍然是可达的 |
| "它在私有地址上，所以没人能访问到"不是一项安全控制 |
| 真正能挡住它的是内网服务上的 Host 校验，因为请求是带着攻击者的 host 名到达的 |

### 第八部分：从这条规则推出的安全观念

**这里的一切归结为一句话，而那句话是要带走的：**

> **同源策略是一条关于读取的规则，在用户的浏览器里执行，它的存在是为了保护用户的其他内容。它从来不是对你服务器的防御。**

四个推论，按一个应用遇到它们的顺序：

**一个请求到达了，不构成任何证据。** 浏览器会发出跨源请求，服务端会处理它；实测里那次表单 POST 到达时带着一个正确的 `Origin` 头，而用户在整件事里的参与只是打开了一个页面。那个端点无论做什么，都必须自己决定这个请求该不该被受理。

**在浏览器不肯执行边界的地方，接收方必须执行。** 不检查来源的 `postMessage`、不检查 `Origin` 的 WebSocket 握手、以及把调用方的源反射回去的 CORS 配置，是"信任任何到达者"的三种写法。

**cookie 需要它自己的决定。** `SameSite` 就是决定"一次跨站请求要不要带上这个 cookie"的那个设置，而实测里那个结果 —— 请求到了、cookie 没到 —— 正是它在履职。它和同源策略是两层，而它才是那个对 cookie 说话的层。

**而内网不是例外。** DNS 重绑定意味着一个浏览器可以被指向内网服务；这条策略不会阻止它，因为在浏览器看来那个源就是攻击者自己的名字。

### 检测与缓解

- **审计 CORS 配置，并对两种"全给出去"的写法告警。** 一个字面的 `Access-Control-Allow-Origin: *` 出现在一个还带凭据的端点上，以及一个把请求里的 `Origin` 原样回显的头，两者都能从响应里读出来，而两者都意味着"任何源都可以读这个"。
- **用 fetch metadata 头，它们由浏览器设置、页面改不了。** 一个只该被自己页面调用的端点上出现了 `Sec-Fetch-Site: cross-site`，加上 `Sec-Fetch-Mode` 与 `Sec-Fetch-Dest`，是服务端能比较而不是猜的值 —— 而且它连"回应被挡住"的那次尝试也抓得到。
- **在改变状态的请求上记下 `Origin`。** 实测里那个服务端在一次跨源表单提交上看到了 `Origin: http://localhost:9050`；记下这个字段的应用手里有证据，而不记的应用手里是一个看起来普通的请求。
- **对 `postMessage` 的通配目标、以及没有来源检查的消息处理器告警。** 两者在源码里都是机械可找的，而两者都是关于"某项控制缺失"的陈述，不是启发式。
- **缓解上，列出你信任的源，而不是反射到达的那个。** 需要凭据的地方用具体的 `Access-Control-Allow-Origin` 加 `Access-Control-Allow-Credentials: true`；任何可能涉及凭据的地方都不要用通配符。
- **给会话 cookie 设上 `SameSite`，并清楚自己选的是哪个值。** `Lax` 是一个合理的默认，也正是它让实测里那次跨源请求没带上 cookie；`None` 明确地重新打开了跨站的情形，并且要求 `Secure`。
- **用跨源页面产生不出来的 token 保护改变状态的端点。** CSRF 那篇的控制，也是无论用哪条跨源通道都成立的那一项。
- **在浏览器不肯替你做的地方自己校验 `Origin` —— WebSocket 握手与 `postMessage` 接收端。** 这是这条策略按设计缺席的两个地方，所以那道检查就是全部的控制。
- **发 `X-Content-Type-Options: nosniff`，并且不要从一个同时提供应用的 host 上提供用户内容。** 一个浏览器愿意当成脚本处理的响应，就是一个跨源读取原语 —— 这也是为什么 JSONP 形状的端点和宽松的内容类型值得在意。
- **不要把私有网络地址当成保护。** 给内网服务校验 `Host` 头，并对任何"内网里的浏览器够得到"的东西要求认证。
- **并且在读任何关于浏览器安全的材料时记住这个框。** 浏览器给了用户一套规则；其中每一条都在浏览器里执行、在用户的机器上、用用户控制不了的代码执行。**服务端能依赖的，是它自己检查的东西。**
