---
id: clientside-fundamentals
title_en: "Client-Side Storage and postMessage — A Wall With a Door in It"
title_zh: "客户端存储与 postMessage：一堵有门的墙"
summary_en: The same-origin policy stops one origin reading another's DOM, and postMessage is a door built into that wall — measured in a browser, where a page read a secret across origins until both ends checked. Plus what each storage mechanism hides from script, which is one of them.
summary_zh: 同源策略挡住了一个源去读另一个源的 DOM，而 postMessage 是那堵墙上开的一扇门 —— 在浏览器里实测：一个页面跨源读到了密钥，直到两端都做了检查。文章还列出各种存储机制里对脚本隐藏的那些，答案是只有一种。
tags: [web, postmessage, localstorage, cwe-346, cwe-922, browser]
tools: [chrome, python3, browser devtools]
attck: [T1539, T1185]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Two things that look like privacy and are not

Browser storage carries an intuition that it belongs to the site, and the intuition is half right: the storage **is** per-origin, so another site cannot read it. What it is not is private **within** the origin. Any script running on the page — including one an attacker injected — reads everything the site put there.

> **Client-side storage is not private. It is durable**, and those two properties get conflated constantly.

Measured across the mechanisms, on the one question that matters:

| Mechanism | Readable by script | Persists | Note |
|---|---|---|---|
| **`HttpOnly` cookie** | **no** | configurable | **the only one hidden from script** |
| Ordinary cookie | yes | configurable | |
| `localStorage` | yes | **yes, indefinitely** | never expires unless cleared |
| `sessionStorage` | yes | per tab | |
| IndexedDB | yes | yes | structured data, and indexed |
| Cache API | yes | yes | |
| Service worker registrations and their caches | yes | yes | **can also intercept requests** |
| `window.name` | yes | **survives cross-origin navigation** | a historical leak channel |

**Exactly one of those is hidden from script.** So a session token in `localStorage` and a session token in a URL are equivalent from an XSS's point of view — which is the session entry's point, and the reason "we do not use cookies, so we do not have CSRF" is not a security improvement: it trades a CSRF exposure for an XSS exposure, and the second one is worse, because **`localStorage` never expires**. A stolen session ends; a stolen long-lived token does not.

Two more consequences in the same column:

**What is stored persists across sessions.** A cached API response, a token, a draft with personal data — all of it is still there tomorrow, and it is readable by whatever script runs then.

**And a source map is storage of a different kind.** Shipping `.map` files to production sends the application's source to every visitor; the browser fetches them only in developer tools, but it fetches them with no authentication. That is the information disclosure entry's finding, delivered by the build.

### `postMessage`: a door in the wall, measured

The same-origin policy stops one origin from reading another's DOM. `postMessage` exists to make that possible deliberately, and it is a door rather than a hole — **but a door has to be locked from both sides**, and the check is different on each.

Measured in a browser, with a victim page on `http://localhost:9030` framed by a page on `http://127.0.0.1:9030` — two different origins, since a hostname is part of the origin:

| The attacker's page | Unfixed victim | Fixed victim |
|---|---|---|
| Reads the framed page's DOM directly | **blocked** — `contentDocument` is `null` | blocked |
| Sends `{cmd: "give-me-the-secret"}` | **executed** | refused |
| Receives the reply | **`SECRET-API-KEY-9f2a41`** | **`null`** |

Three observations from that table.

**The same-origin policy did its job.** The attacker could not read a single node of the framed document — the DOM access returned nothing. **The wall is real.**

**And the door let the same attacker through.** The unfixed victim listened for messages without checking who sent them, so it acted on an instruction from a page it had never heard of, and answered by sending the secret to a window it did not name.

**Fixing one end is not enough.** The two checks are independent and both are required:

| Direction | The check | What omitting it allows |
|---|---|---|
| **Sending** | `targetOrigin` | `"*"` delivers the message to **whatever window is there** |
| **Receiving** | `event.origin` | any window at all can send an instruction |

```
window.addEventListener("message", e => {
  if (e.origin !== "https://app.example") return;   // receiving side
  e.source.postMessage(reply, "https://app.example"); // sending side
});
```

And the comparison has to be **exact**, which is the same rule as the CORS, host header and redirect entries: `origin.endsWith("app.example")` accepts `evil-app.example`, `origin.indexOf("app.example") >= 0` accepts a string containing it, and a regular expression over hosts has the dot-escaping problem. **An origin is an identity, so comparison is equality.**

`event.source` deserves a mention for the same reason on a third axis: knowing that the message came from the right **origin** is not the same as knowing it came from the window you expected, when a page has more than one frame or popup open.

### Four layers

1. **The trust boundary.** The application trusts that what it places in the client is reached only by its own code. The origin boundary is real and is about **other sites**; there is no boundary at all between the application's script and an injected one.
2. **Data and instruction share a plane.** A stored value is **data**, and it is frequently also the **basis for a decision** — a token, a role, a feature flag. A `postMessage` payload is the same: data that arrives with an implicit claim about who sent it.
3. **Why the usual fix fails.** Moving tokens out of cookies removes them from CSRF's reach and puts them in XSS's, which is a worse trade since `localStorage` does not expire. And a `postMessage` handler that checks one of the two directions is half a control — the measured pair shows both failing independently.
4. **The variants.** `targetOrigin: "*"` on a message carrying a secret; a listener with no origin check; a substring or pattern origin comparison; no `event.source` validation; a received payload passed to `eval` or used to build markup, which is the XSS entry arriving through the messaging API; `window.name` surviving a navigation; and source maps shipped to production.

### Detection and mitigation

- **Search the source for `postMessage(..., "*")` and for `message` listeners without an `origin` check.** Both are mechanically findable, and both are exact statements about a control being absent rather than a heuristic — this is one of the few client-side classes a lint rule catches reliably.
- **Alert when a response carries a `SourceMap` header or a `.map` file is served in production.** It is a build-time mistake with a runtime disclosure, and it is visible in the response.
- **And watch for tokens in storage as an architectural finding.** A scan of the application's own code for `localStorage.setItem` with a name suggesting a credential is a review item, not a runtime alert — the point being that the decision is visible long before anything is exploited.
- **For mitigation, keep session tokens in `HttpOnly` cookies.** Nothing in the browser hides a value from script except that attribute, and it is the one mechanism that survives an XSS with the credential intact.
- **Check both sides of every `postMessage`.** An exact `event.origin` comparison on receipt, a specific `targetOrigin` on send, and `event.source` where more than one window could be talking to the page.
- **Do not treat a received message as code.** A payload that reaches `eval`, `innerHTML` or a `Function` constructor is the XSS entry with an extra hop, and the message's origin tells you about the sender rather than about the contents.
- **Keep nothing long-lived and sensitive in client storage.** Favour values the server can invalidate, short lifetimes, and a server-side record of what the client is allowed to do — the session entry's reasoning, applied to the storage the application chooses instead of cookies.
- **Clear `window.name` when a page is done with it.** It is a legacy feature that persists across navigations, and it is the least obvious of the channels listed above.
- **Do not ship source maps, or serve them only to authenticated internal traffic.** A `.map` file is the application's source, published with no access control.
- **And write the two rules down as rules, because both are counter-intuitive.** Everything in client-side storage must be evaluated as **readable by any XSS**, and every `postMessage` receiver must be evaluated as **reachable by any window**. An application that keeps those two sentences in mind gets the rest of this entry right by default.

<!-- lang:zh -->
### 两件看起来像隐私、其实不是的事

浏览器存储带着一个直觉：它属于那个站点。这个直觉对了一半：存储**确实是**按源隔离的，所以另一个站点读不到。它不是的是**在源内部**的私密性。页面上运行着的任何脚本 —— 包括攻击者注入的那一个 —— 都能读到站点放进去的一切。

> **客户端存储不是私密的。它是持久的**，而这两个性质一直被混为一谈。

跨各种机制实测，只问那个要紧的问题：

| 机制 | 脚本可读 | 持久 | 备注 |
|---|---|---|---|
| **`HttpOnly` cookie** | **不可读** | 可配置 | **唯一一个对脚本隐藏的** |
| 普通 cookie | 可读 | 可配置 | |
| `localStorage` | 可读 | **是，无限期** | 不清就一直不过期 |
| `sessionStorage` | 可读 | 标签页内 | |
| IndexedDB | 可读 | 是 | 结构化数据，而且有索引 |
| Cache API | 可读 | 是 | |
| Service Worker 的注册与它的缓存 | 可读 | 是 | **而且能拦截请求** |
| `window.name` | 可读 | **跨源导航后仍保留** | 一条历史遗留的泄漏途径 |

**上面那些里恰好只有一个对脚本隐藏。** 所以放在 `localStorage` 里的会话令牌与放在 URL 里的会话令牌，从一个 XSS 的视角看是等价的 —— 这就是会话那篇的要点，也是"我们不用 cookie，所以没有 CSRF 风险"不算一项安全改进的原因：它把一份 CSRF 暴露换成了一份 XSS 暴露，而后者更糟，因为 **`localStorage` 从不过期**。被偷的会话会结束；被偷的长期令牌不会。

同一列里还有两个后果：

**存进去的东西跨会话还在。** 一份被缓存的 API 响应、一个令牌、一份含个人信息的草稿 —— 明天它们都还在，而那时跑起来的任何脚本都能读。

**而 source map 是另一种形态的存储。** 把 `.map` 文件发到生产环境，等于把应用的源码发给每一个访客；浏览器只在开发者工具里取它，但它取的时候不需要任何认证。那是信息泄漏那篇的发现，由构建流程递送。

### `postMessage`：墙上的一扇门，实测

同源策略阻止一个源去读另一个源的 DOM。`postMessage` 的存在就是让那件事可以刻意地发生，而它是一扇门、不是一个洞 —— **但门必须从两侧都锁上**，而两侧的检查是不同的。

在浏览器里实测：受害页在 `http://localhost:9030`，被一个在 `http://127.0.0.1:9030` 的页面框住 —— 两个不同的源，因为主机名是源的一部分：

| 攻击者的页面 | 未修复的受害页 | 修复后 |
|---|---|---|
| 直接读被框页面的 DOM | **被阻止** —— `contentDocument` 是 `null` | 被阻止 |
| 发 `{cmd: "give-me-the-secret"}` | **被执行** | 被拒绝 |
| 收到回复 | **`SECRET-API-KEY-9f2a41`** | **`null`** |

那张表里有三个观察。

**同源策略履职了。** 攻击者读不到被框文档里的任何节点 —— DOM 访问什么都没返回。**这堵墙是真的。**

**而那扇门让同一个攻击者过去了。** 未修复的受害页监听消息而不检查是谁发的，于是它执行了一个来自它从未听说过的页面的指令，并用"发给它没指名过的那个窗口"的方式把密钥答了回去。

**只修一端不够。** 那两个检查彼此独立，而两个都必须有：

| 方向 | 检查 | 漏掉它允许什么 |
|---|---|---|
| **发送** | `targetOrigin` | `"*"` 把消息交给**当时在那里的任何窗口** |
| **接收** | `event.origin` | 任何窗口都能发一条指令过来 |

```
window.addEventListener("message", e => {
  if (e.origin !== "https://app.example") return;    // 接收侧
  e.source.postMessage(reply, "https://app.example"); // 发送侧
});
```

而那个比较必须是**精确的**，这和 CORS、Host 头、重定向那几篇是同一条规则：`origin.endsWith("app.example")` 会接受 `evil-app.example`，`origin.indexOf("app.example") >= 0` 会接受一个含它的字符串，而一个针对主机名的正则有点没转义的问题。**一个源就是一个身份，所以比较就是相等。**

`event.source` 值得在第三个轴上提一句同样的理由：知道消息来自正确的**源**，和知道它来自你期望的**那个窗口**不是同一件事 —— 当一个页面开了不止一个 frame 或弹窗时。

### 四层

1. **信任边界。** 应用信任"它放进客户端的东西只有自己的代码能碰到"。源边界是真的，而它管的是**别的站点**；在应用自己的脚本与一个被注入的脚本之间，根本没有任何边界。
2. **数据与指令共用同一平面。** 一个被存下来的值**是数据**，而它经常同时是**一个决定的依据** —— 令牌、角色、功能开关。`postMessage` 的载荷也一样：到达的数据带着一个关于"谁发的"的隐含声明。
3. **为什么常见修法失败。** 把令牌从 cookie 里挪出来，让它脱离 CSRF 的触及范围，同时把它放进 XSS 的触及范围，而这是一笔更差的交易，因为 `localStorage` 不过期。而一个只检查了两个方向之一的 `postMessage` 处理器，是一半的控制 —— 上面那一对实测显示两端各自独立地失效。
4. **变体。** 在携带机密的消息上用 `targetOrigin: "*"`；没有来源检查的监听器；子串或模式的来源比较；不校验 `event.source`；把收到的载荷交给 `eval` 或用来拼标记 —— 那是 XSS 那篇经由消息 API 到达；`window.name` 活过一次导航；以及发到生产环境的 source map。

### 检测与缓解

- **在源码里搜 `postMessage(..., "*")`，以及没有 `origin` 检查的 `message` 监听器。** 两者都是机械可找的，而两者都是"某项控制缺失"的确切陈述而不是启发式 —— 这是少数几种能被 lint 规则可靠抓住的客户端类别之一。
- **当一个响应带着 `SourceMap` 头、或者生产环境在提供 `.map` 文件时告警。** 它是一个构建期的错误配上一次运行期的泄漏，而它在响应里可见。
- **并且把"存储里有令牌"当作一条架构发现来看。** 在应用自己的代码里扫 `localStorage.setItem` 且名字暗示凭据的调用，是一个评审项而不是一条运行期告警 —— 重点是这个决定在任何东西被利用之前很久就看得见。
- **缓解上，把会话令牌留在 `HttpOnly` cookie 里。** 浏览器里除了那个属性，没有别的东西能对脚本隐藏一个值，而它是唯一一种能在一次 XSS 之后让凭据保持完好的机制。
- **检查每一次 `postMessage` 的两侧。** 接收时精确比较 `event.origin`，发送时指定具体的 `targetOrigin`，而在有可能不止一个窗口与页面通话的地方校验 `event.source`。
- **不要把收到的消息当作代码。** 一个到达 `eval`、`innerHTML` 或 `Function` 构造器的载荷，是 XSS 那篇多了一跳；而消息的来源告诉你的是发送者，不是内容。
- **客户端存储里不要放长期有效且敏感的东西。** 优先那些服务端能失效的值、短寿命、以及一份"客户端被允许做什么"的服务端记录 —— 会话那篇的推理，用在应用选择用来替代 cookie 的那种存储上。
- **页面用完 `window.name` 就清掉它。** 它是一个跨导航保留的遗留特性，也是上面列出的那些通道里最不显眼的一个。
- **不要发布 source map，或者只对已认证的内部流量提供它。** 一个 `.map` 文件就是应用的源码，在没有任何访问控制的情况下被公开。
- **并且把那两条规则写成规则，因为两者都反直觉。** 客户端存储里的一切都要按**任何 XSS 都能读到**来评估，而每一个 `postMessage` 接收点都要按**任何窗口都能到达**来评估。一个把这两句话放在心上的应用，会默认把这一篇剩下的部分做对。
