---
id: xss-cross-site-scripting
title_en: Cross-Site Scripting (XSS)
title_zh: 跨站脚本（XSS）
summary_en: XSS is a context bug before it is a payload bug. One unencoded character in the right context is a full compromise; a perfect payload in the wrong context does nothing at all. Read where your input lands, then close the right thing.
summary_zh: XSS 首先是上下文问题，其次才是 payload 问题。在正确的上下文里，一个未编码的字符就足以完全接管；在错误的上下文里，再完美的 payload 也毫无作用。先看你输入落在哪里，再闭合对应的东西。
tags: [web, xss, injection, javascript, csp, bugbounty, 跨站脚本]
tools: [Burp Suite, dalfox, XSStrike, DOMPurify]
attck: [T1059.007]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Context decides everything

Most failed XSS attempts are not filtered, they are simply in the wrong place. A payload that works inside a JavaScript string does nothing inside an HTML attribute, and one that works in an attribute is inert in a JSON response rendered by a framework that escapes by default.

So the first step is never "try a payload", it is: where did my string land?

| Where your input lands | What you must break out of | Example |
|---|---|---|
| HTML text | Nothing — just open a tag | `<img src=x onerror=alert(1)>` |
| Attribute, double quoted | The quote, then the attribute | `" onmouseover="alert(1)` |
| Attribute, single quoted | Same with a single quote | `' onmouseover='alert(1)` |
| Attribute, unquoted | A space plus a new attribute | `x onmouseover=alert(1)` |
| Inside `<script>`, string literal | The quote and the statement | `';alert(1)//` |
| Inside `<script>`, template literal | The `${}` | `${alert(1)}` |
| Inside `<script>`, JSON blob | Close the tag and the script | `</script><script>alert(1)</script>` |
| `href` / `src` attribute | The scheme | `javascript:alert(1)` |
| CSS context | The declaration, or `</style>` | `</style><script>alert(1)</script>` |
| HTML comment | The comment terminator | `--><script>alert(1)</script>` |

A quick way to learn the context: submit a string that is distinctive and hard to mangle, such as `xss1234<>"'&`, then read the raw response (not the rendered page) and look at exactly which characters survived and which got encoded. That tells you what escaping exists, and therefore what you actually need to break.

### Three flavours, three different proofs

- **Reflected** — your payload comes back in that same response. Works when a victim clicks a crafted link. The URL is the delivery mechanism, so the link must be plausible enough to send.
- **Stored** — the payload is saved and served to everyone who views the page. Far more serious, and the usual high-payout variant. Comments, profile fields, filenames, support tickets, log viewers and admin panels are the classic hosts.
- **DOM-based** — the payload never reaches the server. Client-side JavaScript reads from a **source** and writes to a **sink**, and the server-side filtering never sees it.

DOM XSS is where scanners are weakest, so it is worth doing by hand. Sources to look for: `location`, `location.hash`, `document.referrer`, `window.name`, `postMessage` handlers, `localStorage`. Sinks that execute or inject: `innerHTML`, `outerHTML`, `document.write`, `eval`, `setTimeout` with a string, `Function()`, `insertAdjacentHTML`, jQuery's `$()` with HTML, and framework escape hatches such as `dangerouslySetInnerHTML`, `v-html`, or `|safe`.

```javascript
// a source-to-sink path you can often trace by searching the bundle
const q = new URLSearchParams(location.search).get('q');
document.getElementById('result').innerHTML = 'No results for ' + q;   // sink
```

### Finding it

```bash
# reflected and DOM scanning with a real browser engine
dalfox url 'https://target/search?q=test'

# crawl, then test every parameter, and report only what executes
dalfox file urls.txt --deep-domxss --mining-dom
```

In Burp, the useful habit is to keep the proxy history filtered on your marker string and read the *response* body, not the rendered page. A payload that is HTML-encoded in the response is dead in that context regardless of what the browser does with it later.

Also test the boring-looking places: search result pages that echo the query, error pages that echo the path, JSON APIs where the front-end later injects the string into the DOM (`innerHTML` on a fetched field is a very common real bug), and anything that stores a filename or a display name.

### When CSP gets in the way

A strict CSP means your inline `<script>` will not run. Before giving up, look at what the policy actually allows:

- Read the header: `Content-Security-Policy` (and the report-only variant, which tells you what the team is about to enforce).
- `script-src 'self'` plus a JSONP endpoint on the same origin is a bypass: call the endpoint with your payload as the callback.
- Angular, React or other frameworks on the page can serve as a **script gadget** — a trick that makes existing library code execute a string from the DOM.
- If `strict-dynamic` is present with a nonce, look for a way to get your script loaded by an already-trusted script.
- `base-uri` unset means you may be able to inject a `<base>` tag and change where relative scripts load from.
- Dangling markup injection lets you exfiltrate content without executing anything at all, when the goal is data rather than JavaScript.

CSP is a mitigation, not a wall. Report a working bypass as its own finding, with the policy included in the report.

### Proving impact

`alert(1)` proves execution to you and proves nothing to a triager. What counts:

1. **Read something the attacker should not have** — session data in `document.cookie` (if not `HttpOnly`), an API token in `localStorage`, CSRF tokens from the page, or data rendered elsewhere in the same origin.
2. **Change state** — perform an action as the victim: change their email, add an API key, send a message. Screenshot the before/after in the victim's own account.
3. **Reach internal endpoints** — fetch same-origin APIs that are not reachable from outside, and exfiltrate the result.
4. **Persist on an admin page** — stored XSS in a support ticket that an administrator reads is an account-takeover primitive for the whole application; say so.

When cookies are `HttpOnly`, XSS is still severe: read the CSRF token from the DOM and issue state-changing requests, capture keystrokes into the login form, or use the session through the browser itself (`fetch` with credentials). Do not downgrade the finding just because you cannot see the session cookie.

### Detection

- WAF and application logs containing HTML tag patterns, `javascript:` schemes or `onerror=` in parameters, headers, or cookies.
- Stored payloads appearing in database text columns — search for `<script`, `onerror`, `<img`, and `javascript:` in user-generated content.
- Unusual outbound requests from a browser session to a domain that is not referenced anywhere (exfiltration).
- The strongest signal is a CSP report: a violation report names the blocked source and the page, and a spike is often the first sign of an attempt.

### Mitigation

- **Encode on output, per context.** HTML-escape for HTML text, attribute-escape for attributes, JavaScript-escape inside script blocks, URL-escape in URLs. Input filtering is not a substitute.
- **Let the framework escape by default** and audit the escape hatches (`dangerouslySetInnerHTML`, `v-html`, `|safe`, `innerHTML`) as a list of review items.
- **Sanitise rich text with a maintained library** (DOMPurify) and an allow-list — never a regex.
- **Deploy a strict CSP** (`default-src 'self'`, nonce or hash for scripts, no `unsafe-inline`), set `base-uri 'none'`, and enable reporting.
- **Set `HttpOnly` and `SameSite` on session cookies** — this limits the damage, it does not fix the bug.
- **Consider Trusted Types** in modern browsers: it turns DOM-based sinks into type errors unless an explicit policy allows them.
- **Treat DOM sources and sinks as code review targets**; a linter that bans `innerHTML` with interpolated strings removes the whole class from new code.

<!-- lang:zh -->
### 上下文决定一切

大多数失败的 XSS 尝试并不是被过滤掉了，而是**位置不对**。在 JavaScript 字符串里能用的 payload，放进 HTML 属性里毫无作用；在属性里能用的 payload，放进一个由框架默认转义后渲染的 JSON 响应里同样是死的。

所以第一步永远不是"试个 payload"，而是：我这段字符串落在哪里了？

| 输入落点 | 你需要跳出什么 | 例子 |
|---|---|---|
| HTML 文本 | 什么都不用 —— 直接开标签 | `<img src=x onerror=alert(1)>` |
| 双引号属性值 | 引号，然后是新属性 | `" onmouseover="alert(1)` |
| 单引号属性值 | 同上，用单引号 | `' onmouseover='alert(1)` |
| 无引号属性值 | 一个空格加新属性 | `x onmouseover=alert(1)` |
| `<script>` 内的字符串 | 引号和当前语句 | `';alert(1)//` |
| `<script>` 内的模板字面量 | `${}` | `${alert(1)}` |
| `<script>` 内的 JSON 块 | 闭合标签与脚本 | `</script><script>alert(1)</script>` |
| `href` / `src` 属性 | 协议 | `javascript:alert(1)` |
| CSS 上下文 | 声明，或 `</style>` | `</style><script>alert(1)</script>` |
| HTML 注释 | 注释终止符 | `--><script>alert(1)</script>` |

快速摸清上下文的办法：提交一个特征鲜明、不容易被混淆的字符串，比如 `xss1234<>"'&`，然后看**原始响应**（不是渲染后的页面），逐字符对比哪些原样保留、哪些被编码。这直接告诉你存在什么样的转义，也就告诉了你需要闭合什么。

### 三种形态，三种证明方式

- **反射型** —— payload 在同一个响应里回来。需要受害者点击构造好的链接，所以链接要足够可信才送得出去。
- **存储型** —— payload 被存下来，所有访问该页的人都会中招。严重得多，也是赏金最高的那种。评论区、个人资料字段、文件名、工单、日志查看器、管理后台是经典宿主。
- **DOM 型** —— payload 根本不经过服务端。客户端 JS 从某个 **source** 读取，写进某个 **sink**，服务端的过滤完全看不到。

DOM XSS 是扫描器最弱的地方，值得手工做。要找的 source：`location`、`location.hash`、`document.referrer`、`window.name`、`postMessage` 处理器、`localStorage`。会执行或注入的 sink：`innerHTML`、`outerHTML`、`document.write`、`eval`、传字符串的 `setTimeout`、`Function()`、`insertAdjacentHTML`、jQuery 传 HTML 的 `$()`，以及框架的逃生舱如 `dangerouslySetInnerHTML`、`v-html`、`|safe`。

```javascript
// 这种 source 到 sink 的路径，往往在打包产物里搜一遍就能找到
const q = new URLSearchParams(location.search).get('q');
document.getElementById('result').innerHTML = 'No results for ' + q;   // sink
```

### 怎么找

```bash
# 用真实浏览器引擎做反射型与 DOM 型扫描
dalfox url 'https://target/search?q=test'

# 先爬，再逐个参数测，只报告真正执行成功的
dalfox file urls.txt --deep-domxss --mining-dom
```

在 Burp 里，一个好习惯是按你的标记字符串过滤代理历史，然后读**响应体**而不是渲染后的页面。一个在响应里被 HTML 实体编码的 payload，在那个上下文里已经死了，无论浏览器之后拿它做什么。

也别忘了那些看起来无聊的地方：回显查询词的搜索结果页、回显路径的错误页、前端之后会把字段塞进 DOM 的 JSON 接口（对 fetch 来的字段用 `innerHTML` 是非常常见的真实漏洞），以及任何存储文件名或显示名的地方。

### CSP 挡路的时候

严格的 CSP 意味着你内联的 `<script>` 不会执行。先别放弃，看看策略实际允许什么：

- 读响应头：`Content-Security-Policy`（以及 report-only 版本，它告诉你团队正准备强制什么）。
- `script-src 'self'` 加上同源上有个 JSONP 端点就是绕过：把你的 payload 当回调名去调用那个端点。
- 页面上有 Angular、React 等框架时，它们可能充当 **script gadget** —— 让已有的库代码去执行 DOM 里的字符串。
- 如果存在带 nonce 的 `strict-dynamic`，就找办法让你的脚本被某个已被信任的脚本加载。
- 没有设置 `base-uri` 意味着你可能能注入 `<base>` 标签，改变相对路径脚本的加载来源。
- 当目标只是取数据而不是执行代码时，悬空标记注入（dangling markup injection）完全不执行任何东西就能把内容带出去。

CSP 是缓解，不是墙。绕过一个有效策略本身就该单独报一个发现，报告里附上那条策略。

### 证明影响

`alert(1)` 向你自己证明了执行，对审核的人什么也没证明。真正算数的是：

1. **读到攻击者本不该有的东西** —— `document.cookie`（如果没设 `HttpOnly`）、`localStorage` 里的 API token、页面上的 CSRF token，或者同源下别处渲染的数据。
2. **改变状态** —— 以受害者身份执行操作：改邮箱、加 API key、发消息。在受害者自己的账号里截图前后对比。
3. **访问内部接口** —— 用 fetch 打同源下外部访问不到的 API，并把结果外带出去。
4. **在管理页面持久化** —— 管理员会查看的工单里的存储型 XSS，就是整个应用的账号接管原语，报告里要这么说。

当 Cookie 是 `HttpOnly` 时，XSS 依然严重：从 DOM 里读 CSRF token 发状态变更请求、拦截登录表单的键盘输入，或者干脆通过浏览器本身使用这个会话（带凭据的 `fetch`）。不要因为看不到会话 Cookie 就降低定级。

### 检测

- WAF 与应用日志里出现 HTML 标签模式、`javascript:` 协议或参数/请求头/Cookie 里的 `onerror=`。
- 数据库文本列里出现存储的 payload —— 在用户生成内容里搜 `<script`、`onerror`、`<img`、`javascript:`。
- 浏览器会话向任何页面都没引用过的域名发起异常外连（数据外带）。
- 最强的信号是 CSP 报告：违规报告会指出被拦的来源和发生页面，报告量突增往往是攻击尝试的第一个迹象。

### 缓解

- **按上下文在输出处编码。** HTML 文本用 HTML 转义，属性用属性转义，脚本块内用 JavaScript 转义，URL 里用 URL 转义。输入过滤不是替代品。
- **让框架默认转义**，并把逃生舱（`dangerouslySetInnerHTML`、`v-html`、`|safe`、`innerHTML`）列成一份定期复查的清单。
- **富文本用有维护的库做白名单净化**（DOMPurify），绝不要用正则。
- **部署严格 CSP**（`default-src 'self'`，脚本用 nonce 或 hash，不要 `unsafe-inline`），设置 `base-uri 'none'`，并开启报告。
- **会话 Cookie 设 `HttpOnly` 与 `SameSite`** —— 这限制损害，但不修复漏洞。
- **在现代浏览器上考虑 Trusted Types**：除非有显式策略允许，它会把 DOM sink 变成类型错误。
- **把 DOM 的 source 与 sink 当作代码审查目标**；一条禁止对插值字符串使用 `innerHTML` 的 lint 规则，能把这一整类问题从新代码里去掉。
