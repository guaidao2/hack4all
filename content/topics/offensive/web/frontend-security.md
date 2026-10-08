---
id: frontend-security
title_en: Frontend Security
title_zh: 前端安全
summary_en: The browser is its own trust boundary. CSRF, CORS, clickjacking, postMessage and client-side storage are variations on one question, which is whether another origin can make your user's browser do something on your behalf.
summary_zh: 浏览器本身就是一道信任边界。CSRF、CORS、点击劫持、postMessage 与客户端存储，本质上都在问同一个问题 —— 另一个源能不能让你的用户浏览器替他做点什么。
tags: [web, csrf, cors, clickjacking, postmessage, frontend, bugbounty]
tools: [Burp Suite, browser devtools]
attck: [T1185, T1059.007]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### One question, several shapes

A browser sends cookies with a request because of where the request is going, not because of who caused it. Every frontend vulnerability in this entry is an exploitation of that gap between *where* and *who*.

XSS is covered separately and is the case where the attacker's code runs on the origin. This entry is about the cases where the attacker's code stays on their own origin and still gets the victim's browser to do something.

### CSRF

The classic shape: a form on `evil.example` posting to `target.example`, with the victim's cookies attached because the browser does that automatically.

```html
<form action="https://target.example/account/email" method="POST">
  <input name="email" value="attacker@evil.example">
</form>
<script>document.forms[0].submit()</script>
```

What makes it exploitable, in the order you should check:

- **No token at all.**
- **A token that is not validated**, or only validated on some endpoints.
- **A token that is not bound to the session**, so any valid token works for any user.
- **A predictable token**, or one derived from the session id.
- **A token accepted from a GET parameter or a cookie**, which can be set by the attacker.
- **SameSite not set.** Modern browsers default to `Lax`, which still allows top-level GET navigation — so state-changing GETs are exploitable even with the default.
- **A JSON endpoint that only checks `Content-Type`.** Browsers will not send `application/json` cross-origin without a preflight, so an attacker sends `text/plain` — and a tolerant parser accepts it:

```html
<script>
fetch('https://target.example/api/email', {
  method: 'POST',
  mode: 'no-cors',
  headers: {'Content-Type': 'text/plain'},
  body: '{"email":"attacker@evil.example"}'
});
</script>
```

- **Multipart forms**, which can be created cross-origin and often bypass a JSON-specific check.
- **Login CSRF**, where the attacker logs the victim into the attacker's account, so the victim's later activity is recorded in a place the attacker can read.

### CORS

CORS is a relaxation of the same-origin policy, and it is easy to relax too much. What to look for:

| Configuration | Why it is exploitable |
|---|---|
| `Access-Control-Allow-Origin: *` with credentials | Browsers reject that combination, but it signals a misunderstanding that often has a workaround nearby |
| Origin reflected without validation | Any site can read authenticated responses |
| `null` origin allowed | `sandbox` iframes and `data:` URLs send `null` |
| Subdomain trust (`*.target.example`) | Any subdomain, including a takeover candidate, is trusted |
| Regex or `endsWith` matching | `target.example.evil.example` and `eviltarget.example` both pass a naive suffix check |
| Weak prefix matching | `attackertarget.example` passes a `startsWith` check |

Test by sending an `Origin` you control and reading the response headers. The browser enforces the policy, so exploitation needs the victim's browser rather than curl.

```bash
curl -s -I https://target.example/api/me -H 'Origin: https://evil.example' | grep -i access-control
```

### Clickjacking

Framing the target inside a page the attacker controls, then positioning it so the victim clicks something they cannot see.

- The control is `Content-Security-Policy: frame-ancestors 'none'` (or an allow-list). `X-Frame-Options: DENY` works but is the older mechanism.
- `X-Frame-Options: ALLOW-FROM` is not supported by modern browsers and is not a control.
- Variants worth trying when basic framing is blocked: double framing, frames with `sandbox`, drag-and-drop payloads, and SVG-based frames.
- The impact depends on what the framed action does. A click on "delete account" or "confirm payment" is a real finding; a click that opens a settings page is usually not.

### postMessage

`postMessage` is how frames and popups talk, and the two mistakes are symmetric:

- **Receiver does not check `event.origin`.** Any page that can hold a reference to the window can send a message and trigger whatever the handler does — including actions like changing an email address.
- **Sender uses `targetOrigin: '*'`.** The message, including any token in it, is delivered to whatever document is currently at that window. If the window navigated, the token goes elsewhere.

```javascript
// receiving side, the check that is often missing
window.addEventListener('message', (e) => {
  if (e.origin !== 'https://trusted.example') return;   // without this, anyone can drive it
  doSomething(e.data);
});
```

### Client-side storage

Where the front end keeps things decides what an XSS is worth:

- **`localStorage` tokens** are readable by any script on the origin, so an XSS is immediate account takeover. An `HttpOnly` cookie is not readable and is a better place for a session.
- **Service workers** persist after the page closes and can intercept requests, so a single registration survives until it is explicitly unregistered. That makes them a persistence mechanism as well as a caching one.
- **IndexedDB and the Cache API** often hold the same data the application was careful about server-side.

### DOM clobbering and client-side prototype pollution

- **DOM clobbering**: HTML with `id` or `name` attributes can shadow global variables and properties, which breaks scripts that assume `document.x` is what they defined.
- **Prototype pollution in the browser**: a URL parameter or a JSON payload that gets merged into an object can add properties to `Object.prototype`, and a gadget in the page's own code then executes. Client-side libraries are full of the `merge`/`extend` patterns that make this possible.

### Detection

- **State-changing requests without an origin check.** Log `Origin` and `Referer` for sensitive endpoints and alert on a mismatch with your own domains.
- **CORS misconfiguration** is discoverable by inspection: audit every response that sets `Access-Control-Allow-Origin` and confirm it is a fixed value from an allow-list, not a reflection.
- **Missing `frame-ancestors`** on pages that perform actions is a review item that a header scan can find.
- **`postMessage` handlers** that do not compare `e.origin` are visible in the source; a code search for `addEventListener('message'` is productive.
- **Tokens in `localStorage`** are visible in any client-side error report or browser extension review.

### Mitigation

- **CSRF tokens bound to the session**, validated on every state-changing request, plus `SameSite=Lax` at minimum and `Strict` where the flow allows it. Use the framework's implementation rather than a hand-rolled one.
- **Require `Content-Type: application/json` and reject others** for JSON APIs, and do not parse a body whose content type you did not expect.
- **CORS with an exact allow-list**, no reflection, no suffix matching, no `null`, and `Access-Control-Allow-Credentials` only where it is genuinely needed.
- **`frame-ancestors 'none'`** on everything that is not explicitly designed to be framed.
- **Validate `event.origin` in every message handler** and send with an explicit `targetOrigin`.
- **Keep session tokens in `HttpOnly`, `Secure`, `SameSite` cookies.** `localStorage` is for non-sensitive state.
- **Pin and verify third-party scripts** (see the integrity entry in this guide), and prefer self-hosting them.
- **Review client-side code for merge and extend patterns** that accept user-controlled input, and use objects without prototypes (`Object.create(null)`) where untrusted keys are handled.

<!-- lang:zh -->
### 一个问题，几种形状

浏览器带上 cookie，是因为**请求发往哪里**，而不是因为**谁发起的**。这一篇里的每一个前端漏洞，都是在利用"发往哪里"与"谁发起"之间的这道缝。

XSS 另有专篇，那是攻击者的代码跑在目标源上的情况。这一篇讲的是攻击者代码留在自己的源上、却仍能让受害者浏览器替他做事的那些情况。

### CSRF

经典形态：`evil.example` 上的一个表单提交到 `target.example`，受害者的 cookie 会被自动带上。

```html
<form action="https://target.example/account/email" method="POST">
  <input name="email" value="attacker@evil.example">
</form>
<script>document.forms[0].submit()</script>
```

按检查顺序，让它可利用的原因：

- **完全没有 token。**
- **有 token 但不校验**，或者只在部分接口校验。
- **token 没有绑定会话**，于是任意有效 token 对任意用户都成立。
- **token 可预测**，或由会话 id 派生。
- **接受 GET 参数或 cookie 里的 token** —— 那都是攻击者能设置的。
- **没有设 SameSite。** 现代浏览器默认 `Lax`，而 `Lax` 仍允许顶层 GET 导航 —— 所以会改状态的 GET 即便在默认配置下也可被利用。
- **JSON 接口只检查 `Content-Type`。** 浏览器跨源不会在不预检的情况下发 `application/json`，于是攻击者发 `text/plain` —— 而宽松的解析器会接受它：

```html
<script>
fetch('https://target.example/api/email', {
  method: 'POST',
  mode: 'no-cors',
  headers: {'Content-Type': 'text/plain'},
  body: '{"email":"attacker@evil.example"}'
});
</script>
```

- **multipart 表单** —— 可以跨源构造，且常常绕过针对 JSON 做的检查。
- **登录 CSRF** —— 攻击者把受害者登进**攻击者的账号**，于是受害者之后的活动都记录在攻击者能看到的地方。

### CORS

CORS 是对同源策略的放宽，而它很容易被放宽过头。要看的配置：

| 配置 | 为什么可利用 |
|---|---|
| `Access-Control-Allow-Origin: *` 配凭据 | 浏览器会拒绝这种组合，但它说明实现存在误解，附近往往有变通 |
| 不校验就反射 Origin | 任意站点都能读已认证的响应 |
| 允许 `null` 来源 | `sandbox` 的 iframe 与 `data:` URL 会发 `null` |
| 信任子域（`*.target.example`） | 任意子域都被信任，包括可能被接管的那个 |
| 用正则或 `endsWith` 匹配 | `target.example.evil.example` 和 `eviltarget.example` 都能过幼稚的后缀检查 |
| 弱前缀匹配 | `attackertarget.example` 能过 `startsWith` 检查 |

测试方式：发一个你控制的 `Origin` 并读响应头。策略由浏览器执行，所以利用需要受害者的浏览器，而不是 curl。

```bash
curl -s -I https://target.example/api/me -H 'Origin: https://evil.example' | grep -i access-control
```

### 点击劫持

把目标页面嵌进攻击者控制的页面里，再摆放位置，让受害者点到他看不见的东西。

- 控制手段是 `Content-Security-Policy: frame-ancestors 'none'`（或白名单）。`X-Frame-Options: DENY` 也行，但那是更老的机制。
- `X-Frame-Options: ALLOW-FROM` 现代浏览器不支持，它不构成控制。
- 基础嵌套被封时可以试的变体：双重嵌套、带 `sandbox` 的框架、拖拽类 payload、基于 SVG 的框架。
- 影响取决于被嵌套的动作是什么。点到"删除账号"或"确认支付"是真实发现；点到"打开设置页"通常不是。

### postMessage

`postMessage` 是框架与弹窗之间通信的方式，而两个错误是对称的：

- **接收方不校验 `event.origin`。** 任何能持有该窗口引用的页面都能发消息，触发处理器里的任意动作 —— 包括改邮箱这类操作。
- **发送方用 `targetOrigin: '*'`。** 消息（包括其中的 token）会被投递给那个窗口当前的任何文档。如果那个窗口发生了跳转，token 就去了别处。

```javascript
// 接收侧，最常缺失的那个校验
window.addEventListener('message', (e) => {
  if (e.origin !== 'https://trusted.example') return;   // 没有这一行，谁都能驱动它
  doSomething(e.data);
});
```

### 客户端存储

前端把东西放在哪，决定了一次 XSS 值多少：

- **`localStorage` 里的 token** 对同源任何脚本可读，所以一次 XSS 就是即刻的账号接管。`HttpOnly` cookie 不可读，是更适合放会话的地方。
- **Service Worker** 在页面关闭后依然存在，能拦截请求，所以一次注册会一直有效直到被显式注销。它既是缓存机制，也是一种持久化机制。
- **IndexedDB 与 Cache API** 里往往存着服务端曾经小心翼翼处理过的同一批数据。

### DOM Clobbering 与客户端原型污染

- **DOM Clobbering**：带 `id` 或 `name` 属性的 HTML 可以遮蔽全局变量与属性，从而破坏那些假设 `document.x` 就是自己定义的东西的脚本。
- **浏览器里的原型污染**：一个 URL 参数或 JSON 载荷被合并进对象时，可能给 `Object.prototype` 加上属性，然后页面自身代码里的某个 gadget 就执行了。前端库里满是让这件事成立的 `merge`/`extend` 模式。

### 检测

- **会改状态、又不检查来源的请求。** 对敏感接口记录 `Origin` 与 `Referer`，与自己的域名不匹配就告警。
- **CORS 配置错误**可以通过检查发现：审计每一个设置 `Access-Control-Allow-Origin` 的响应，确认它是来自白名单的固定值，而不是反射值。
- **缺 `frame-ancestors`** 的执行动作页面，是一次响应头扫描就能找出的复查项。
- **不比较 `e.origin` 的 `postMessage` 处理器**在源码里可见；搜一下 `addEventListener('message'` 很有收获。
- **`localStorage` 里的 token** 在任何前端错误上报或浏览器扩展审查里都看得见。

### 缓解

- **会话绑定的 CSRF token**，在每个会改状态的请求上校验；再加至少 `SameSite=Lax`，流程允许的地方用 `Strict`。用框架自带的实现，别手搓。
- **JSON 接口要求 `Content-Type: application/json` 并拒绝其他类型**，不要解析你没预期的内容类型。
- **CORS 用精确白名单**：不反射、不做后缀匹配、不允许 `null`，`Access-Control-Allow-Credentials` 只在真正需要处开启。
- **所有非刻意允许被嵌套的页面都设 `frame-ancestors 'none'`。**
- **每个消息处理器都校验 `event.origin`**，发送时用明确的 `targetOrigin`。
- **会话 token 放 `HttpOnly`、`Secure`、`SameSite` 的 cookie 里。** `localStorage` 只用来放非敏感状态。
- **固定并校验第三方脚本**（见本指南完整性那篇），能自托管就自托管。
- **审查客户端代码里接受用户输入的 merge/extend 模式**，处理不可信键时使用无原型对象（`Object.create(null)`）。
