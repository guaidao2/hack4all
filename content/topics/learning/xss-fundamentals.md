---
id: xss-fundamentals
title_en: "XSS, Part 1 — The Browser as the Interpreter"
title_zh: "XSS（一）：把浏览器当解释器"
summary_en: Cross-site scripting is the same bug as injection anywhere else — data reaching a position where it is read as code — except that the interpreter is the browser and the language changes with the context. This entry covers the three types, why the context decides what is dangerous, why filtering cannot work, and what actually fixes it.
summary_zh: 跨站脚本与其他任何地方的注入是同一个 bug —— 数据到达了一个被当作代码来读的位置 —— 只不过解释器是浏览器，而"语言"随上下文而变。这一篇讲三种类型、讲为什么是上下文决定什么危险、讲为什么过滤必然失败，以及什么才是真正的修法。
tags: [web, xss, browser, output-encoding, content-security-policy]
tools: [browser devtools, Burp Suite, curl, XSStrike]
attck: [T1059.007, T1185]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The same bug as everything else, with a different interpreter

Injection is one idea wearing many costumes: **data reaches a position where something reads it as code.** In SQL injection the interpreter is a database and the language is SQL. In cross-site scripting the interpreter is a **browser** and the language is whatever the surrounding context says it is — HTML, JavaScript, CSS, or a URL.

Two consequences follow immediately, and they shape the whole entry:

1. **The browser cannot tell your tags from the user's tags.** Once the response is assembled, it is one document. A `<script>` that the developer wrote and a `<script>` that arrived in a comment are indistinguishable to the parser — the same statement as "the database cannot tell which part of the string it was handed came from a user".
2. **There is no single dangerous character.** `<` matters in HTML text; it is harmless inside a JavaScript string. A quote matters inside an attribute; in text it is just a quote. `javascript:` matters in an `href`; in a paragraph it is a word.

That second point is the reason this entry spends most of its length on context. It is also the reason so much well-intentioned defence fails.

### The three types, by where the data goes

| Type | Path | Persistence | Server sees the payload? |
|---|---|---|---|
| **Reflected** | Request value → written into the same response | No | **Yes**, in the request |
| **Stored** | Request value → stored → rendered to other users later | Yes | Yes, at write time |
| **DOM-based** | The **client** reads a value and writes it into the DOM | Depends on the source | **Often no** |

**Reflected** requires luring a victim to a crafted link or form submission. It is the easiest to test and the easiest to dismiss as "needs user interaction" — which is a mistake, because the interaction is a click.

**Stored** is the serious one operationally: no lure is needed, every visitor is affected, and if the affected page is a profile or a message board, the payload propagates itself.

**DOM-based** deserves its own paragraph, because it breaks two assumptions that server-side defences are built on.

```javascript
// the vulnerable pattern, entirely in the browser
const name = new URLSearchParams(location.search).get("name");
document.getElementById("greeting").innerHTML = "Hello, " + name;
```

The server may never see the payload at all — for instance when it arrives in the **URL fragment**, which browsers do not send:

```
https://app/#<img src=x onerror=alert(1)>
```

Two practical consequences: a filter in front of the server cannot see it, and a server-side access log cannot record it. The bug lives in the client code, and finding it means reading JavaScript rather than replaying requests.

### Context decides what is dangerous

This is the heart of it. The same value is safe in one position and dangerous in another, and the set of characters that matter changes with each.

| Context | Example | What breaks out | What encoding is required |
|---|---|---|---|
| **HTML text** | `<p>USER</p>` | `<`, and `&` to a lesser degree | HTML entity encoding |
| **HTML attribute, quoted** | `<a title="USER">` | `"` (or `'`), closing the attribute | HTML entity encoding, quotes included |
| **HTML attribute, unquoted** | `<a title=USER>` | space, `>`, `/`, `=` | Entity encoding everything outside a safe set |
| **URL in an attribute** | `<a href="USER">` | `javascript:` (and other schemes), plus attribute quoting | URL encoding **plus** scheme allowlisting |
| **`<script>` block** | `<script>var x = USER;</script>` | `</script>`, quotes, and any JS syntax | JavaScript string encoding, and `</script>` must not appear |
| **JavaScript string** | `<script>var x = "USER";</script>` | `"`, `\`, newline, `</script>` | JavaScript string escaping plus `</script>` neutralisation |
| **JavaScript template literal** | ``<script>var x = `USER`;</script>`` | `` ` ``, `${` | JS escaping for template context |
| **CSS** | `<style>body { color: USER }</style>` | `}`, `</style>`, and historically `expression()` | CSS escaping |
| **HTML comment** | `<!-- USER -->` | `-->` | Remove, or entity-encode |
| **Event handler attribute** | `<a onclick="fn(USER)">` | quotes, `)`, and anything JavaScript | JS string encoding inside an HTML attribute |

Read a few rows together and the practical rule falls out:

> **The context determines the encoding, so an encoder must know which context it is writing into.**

This is why `encodeURIComponent` does not help in HTML text (it leaves `<` alone), why `htmlspecialchars` alone is not enough inside a JavaScript string (it does not escape `\` or newlines), and why "we escape everything" is not a statement a reviewer can accept without knowing *where*.

```python
import html, urllib.parse

payload = '"><img src=x onerror=alert(1)>'

# correct for HTML text, wrong for a URL context
print(html.escape(payload))
# &quot;&gt;&lt;img src=x onerror=alert(1)&gt;

# correct for a URL *value* position, wrong for HTML text
print(urllib.parse.quote(payload, safe=''))
# %22%3E%3Cimg%20src%3Dx%20onerror%3Dalert%281%29%3E

# and note that neither one is enough on its own for href, where the scheme matters
for candidate in ('javascript:alert(1)', 'java\nscript:alert(1)', 'JaVaScRiPt:alert(1)'):
    print(candidate, '->', urllib.parse.quote(candidate, safe=':'))
```

### Why filtering cannot be the answer

Every "sanitise the input" approach converges on the same two shapes, and both fail for structural reasons.

**A blocklist of tags and attributes.** `<script>`, `onerror`, `javascript:` — and then the browser turns out to offer many more ways to run code:

```html
<img src=x onerror=alert(1)>
<svg onload=alert(1)>
<body onpageshow=alert(1)>
<iframe srcdoc="<script>alert(1)</script>">
<details open ontoggle=alert(1)>
<a href="javascript:alert(1)">x</a>
<a href="data:text/html,<script>alert(1)</script>">x</a>
<math><mtext><table><mglyph><style><!--</style><img title="--><img src=1 onerror=alert(1)>">
```

The last one is the interesting category: **mutation XSS**. A sanitiser parses the input, decides it is safe, serialises it back — and the browser, parsing that serialisation, produces a **different tree** in which the payload is live. That is the two-parser disagreement from the language entries, with a sanitiser on one side and a browser on the other.

**Escaping at the wrong layer.** Escaping on input, before the value's destination is known, cannot be correct: the same value may end up in an attribute, a script and a URL, each needing different treatment. `magic_quotes_gpc` in PHP is the historical version of this mistake, and the language entry already draws the lesson — a defence that works on the representation rather than at the boundary is both incomplete and harmful.

**What actually fixes it** is output encoding performed **at the point of output, with knowledge of the context**:

| Approach | Why it works |
|---|---|
| **Context-aware auto-escaping templates** | The template engine knows where the value is being written and picks the encoder |
| **A JavaScript framework's default rendering** | Text interpolated into JSX or a component is escaped as text, and the escape hatches are named (`dangerouslySetInnerHTML`, `v-html`, `\|safe`) |
| **`textContent` instead of `innerHTML`** | Setting text cannot create elements, so there is no context to break out of |
| **Structured DOM APIs** | `document.createElement` with `setAttribute` for safe attribute names; `document.createTextNode` for text |

```javascript
// the DOM version of the same fix
const el = document.getElementById("greeting");
el.textContent = "Hello, " + name;          // no element can be created

// and when HTML is genuinely required, it must come from a sanitiser that is
// maintained for that purpose, on a value that is not used anywhere else
```

**Content Security Policy is a second line, not the first.** A policy that forbids inline scripts and restricts `script-src` to your own origin turns many successful injections into inert ones — and it does nothing about the ones that the policy allows. Two things make CSP especially valuable here:

- It is the **only server-side control that also covers DOM-based XSS**, because the browser enforces it regardless of how the payload got into the page.
- **`report-uri` / `report-to` turns it into a detection mechanism** — the browser reports every blocked attempt, which is how you find XSS in production without waiting for an incident.

### Detection and mitigation

- **Encode on output, with the context known.** One encoder per context — HTML text, HTML attribute, URL, JavaScript string, CSS — chosen by the template engine or by the code at the point of writing. This is the primary control, and everything else is a layer behind it.
- **Ban the dangerous rendering APIs by convention, and name the exceptions.** `innerHTML`, `outerHTML`, `document.write`, `eval`, `Function`, `dangerouslySetInnerHTML`, `v-html`, `|safe`, and Go's `text/template` for HTML. A short list of forbidden calls is enforceable in review and in lint rules, which is more than can be said for "be careful with encoding".
- **Deploy CSP with a nonce or hash, and turn reporting on.** `script-src 'self' 'nonce-...'` with `object-src 'none'` and `base-uri 'self'` blocks a large share of real payloads, and the report endpoint gives you the detection signal. Treat the reports as a security feed, not as noise: an injected `report-uri` line is a finding.
- **Set cookies `HttpOnly` where JavaScript does not need them.** It does not prevent XSS; it prevents the most common exfiltration path from working, which changes the incident from "session stolen" to "page manipulated".
- **Log the values that reach the response, not just the requests.** The useful signal for reflected XSS is a response that contains an unencoded fragment of the request. Comparing the request's parameters against the response body, and alerting when a parameter appears verbatim inside an HTML or script context, catches payloads that no signature knows.
- **Remember that DOM-based XSS leaves no server-side trace.** For those, the sources are client-side (URL fragment, `document.referrer`, `localStorage`, `postMessage`) and the sinks are client-side (`innerHTML`, `eval`, `document.write`, `location`). Detection therefore comes from CSP reports and client-side monitoring rather than from the access log, and code review is the primary discovery method.
- **Do not rely on input filtering, and do not rely on a WAF as the fix.** Both are bypassable by construction, and the browser's tolerance for malformed HTML means the set of bypasses is not enumerable. Keep them as cost and detection; the fix is at the point of output.
- **And keep the framing from the first line**: this is the same bug as SQL injection, with a different interpreter. The fix has the same shape — separate the data channel from the instruction channel — and the practical form of that separation for a browser is *encoding decided by context*, because a browser has several instruction channels rather than one.

<!-- lang:zh -->
### 和别处一模一样的 bug，只是换了个解释器

注入是同一个想法穿着很多套衣服：**数据到达了一个被某样东西当作代码来读的位置。** 在 SQL 注入里解释器是数据库、语言是 SQL。在跨站脚本里解释器是**浏览器**，而语言由周围上下文决定 —— HTML、JavaScript、CSS，或者一个 URL。

由此立刻产生两个后果，而它们塑造了整篇：

1. **浏览器分不清你的标签和用户的标签。** 响应一旦组装完成，它就是一份文档。开发者写的那个 `<script>` 和从评论里来的那个 `<script>`，对解析器来说无从区分 —— 和"数据库分不清交给它的那个字符串里哪部分来自用户"是同一句话。
2. **没有哪一个字符天生危险。** `<` 在 HTML 文本里要紧，在 JavaScript 字符串里无害。引号在属性里要紧，在文本里就只是个引号。`javascript:` 在 `href` 里要紧，在段落里只是一个词。

第二点就是这一篇把大部分篇幅花在上下文上的原因，也是大量善意防御失效的原因。

### 三种类型，按数据去了哪里分

| 类型 | 路径 | 持久性 | 服务端看得到 payload 吗 |
|---|---|---|---|
| **反射型** | 请求值 → 写进同一个响应 | 否 | **看得到**，在请求里 |
| **存储型** | 请求值 → 存起来 → 之后渲染给其他用户 | 是 | 看得到，在写入时 |
| **DOM 型** | **客户端**读取一个值并写进 DOM | 取决于来源 | **常常看不到** |

**反射型**需要把受害者骗到一个构造好的链接或表单提交上。它最好测，也最容易被轻描淡写为"需要用户交互" —— 这是个错误，因为那个交互就是一次点击。

**存储型**在运营意义上才是严重的那个：不需要诱饵、每个访问者都受影响，而如果受影响的是个人资料或留言板，payload 会自我传播。

**DOM 型**值得单独一段，因为它打破了服务端防御所依赖的两个假设。

```javascript
// 有漏洞的模式，完全在浏览器里
const name = new URLSearchParams(location.search).get("name");
document.getElementById("greeting").innerHTML = "Hello, " + name;
```

服务端可能**根本看不到**这个 payload —— 比如它到达的是 **URL 片段**，而浏览器不会发送片段：

```
https://app/#<img src=x onerror=alert(1)>
```

两个实际后果：服务器前面的过滤器看不见它，服务端的访问日志也记不到它。这个 bug 活在客户端代码里，而找它的方式是**读 JavaScript**，不是重放请求。

### 上下文决定什么危险

这是核心。同一个值在一个位置安全、在另一个位置危险，而有影响的字符集合随位置而变。

| 上下文 | 例子 | 怎么突破 | 需要什么编码 |
|---|---|---|---|
| **HTML 文本** | `<p>USER</p>` | `<`，其次 `&` | HTML 实体编码 |
| **HTML 属性（带引号）** | `<a title="USER">` | `"`（或 `'`），闭合属性 | HTML 实体编码，包含引号 |
| **HTML 属性（不带引号）** | `<a title=USER>` | 空格、`>`、`/`、`=` | 对安全集合之外的一切做实体编码 |
| **属性里的 URL** | `<a href="USER">` | `javascript:`（及其他协议），加上属性引号 | URL 编码**加上**协议白名单 |
| **`<script>` 块内** | `<script>var x = USER;</script>` | `</script>`、引号，以及任何 JS 语法 | JavaScript 字符串编码，且 `</script>` 不能出现 |
| **JavaScript 字符串内** | `<script>var x = "USER";</script>` | `"`、`\`、换行、`</script>` | JS 字符串转义加 `</script>` 中和 |
| **JS 模板字符串** | ``<script>var x = `USER`;</script>`` | `` ` ``、`${` | 模板上下文的 JS 转义 |
| **CSS** | `<style>body { color: USER }</style>` | `}`、`</style>`，历史上还有 `expression()` | CSS 转义 |
| **HTML 注释** | `<!-- USER -->` | `-->` | 移除，或实体编码 |
| **事件处理属性** | `<a onclick="fn(USER)">` | 引号、`)`，以及任何 JavaScript | HTML 属性内部的 JS 字符串编码 |

把几行放在一起读，实用规则就出来了：

> **上下文决定编码方式，所以编码器必须知道自己正在写进哪个上下文。**

这就是为什么 `encodeURIComponent` 在 HTML 文本里帮不上忙（它不动 `<`），为什么单独的 `htmlspecialchars` 在 JavaScript 字符串里不够（它不转义 `\` 和换行），以及为什么"我们什么都转义了"不是评审能接受的说法 —— 除非说清**转在哪儿**。

```python
import html, urllib.parse

payload = '"><img src=x onerror=alert(1)>'

# 对 HTML 文本正确，对 URL 上下文错误
print(html.escape(payload))
# &quot;&gt;&lt;img src=x onerror=alert(1)&gt;

# 对 URL 的"值"位置正确，对 HTML 文本错误
print(urllib.parse.quote(payload, safe=''))
# %22%3E%3Cimg%20src%3Dx%20onerror%3Dalert%281%29%3E

# 而且注意：对 href 来说两者单独都不够，因为协议本身才是关键
for candidate in ('javascript:alert(1)', 'java\nscript:alert(1)', 'JaVaScRiPt:alert(1)'):
    print(candidate, '->', urllib.parse.quote(candidate, safe=':'))
```

### 为什么过滤不可能是答案

每一种"净化输入"的做法最终都会收敛到两种形状，而两者都因为结构性原因失败。

**标签与属性的黑名单。** `<script>`、`onerror`、`javascript:` —— 然后浏览器还有很多别的执行代码的方式：

```html
<img src=x onerror=alert(1)>
<svg onload=alert(1)>
<body onpageshow=alert(1)>
<iframe srcdoc="<script>alert(1)</script>">
<details open ontoggle=alert(1)>
<a href="javascript:alert(1)">x</a>
<a href="data:text/html,<script>alert(1)</script>">x</a>
<math><mtext><table><mglyph><style><!--</style><img title="--><img src=1 onerror=alert(1)>">
```

最后一条是有意思的那一类：**突变 XSS（mXSS）**。净化器解析输入、判定它安全、再把它序列化回去 —— 而浏览器解析那份序列化结果时，产出了**另一棵树**，在那棵树里 payload 是活的。这就是语言那几篇里"两个解析器不一致"，只不过一边是净化器、另一边是浏览器。

**在错误的层次转义。** 在输入阶段、还不知道值的目的地时就转义，不可能正确：同一个值可能落到属性、脚本和 URL 里，各自需要不同处理。PHP 的 `magic_quotes_gpc` 是这个错误的历史版本，语言那篇已经得出了教训 —— **作用于"表示"而不是作用于"边界"的防御，既不完备，也有害。**

**真正的修法**是在**输出点、带着上下文知识**做输出编码：

| 做法 | 为什么有效 |
|---|---|
| **上下文感知的自动转义模板** | 模板引擎知道这个值被写在哪里，于是选对编码器 |
| **JavaScript 框架的默认渲染** | 插进 JSX 或组件里的文本按文本转义，而逃生口是有名字的（`dangerouslySetInnerHTML`、`v-html`、`\|safe`） |
| **用 `textContent` 而不是 `innerHTML`** | 设置文本无法创建元素，于是没有上下文可以突破 |
| **结构化的 DOM API** | `document.createElement` 配安全的属性名 `setAttribute`；文本用 `document.createTextNode` |

```javascript
// 同一个修法的 DOM 版本
const el = document.getElementById("greeting");
el.textContent = "Hello, " + name;          // 无法创建元素

// 当确实需要 HTML 时，它必须来自一个为此目的维护的净化器，
// 而且那个值不能在任何别处被使用
```

**CSP 是第二道防线，不是第一道。** 一条禁止内联脚本、把 `script-src` 限制在自有源的策略，会把许多成功的注入变成哑弹 —— 而对它允许的那些注入什么也做不了。有两件事让 CSP 在这里格外有价值：

- 它是**唯一一个也能覆盖 DOM 型 XSS 的服务端控制**，因为浏览器无论 payload 怎么进到页面里都会执行它。
- **`report-uri` / `report-to` 把它变成一个检测机制** —— 浏览器会报告每一次被拦下的尝试，这是你在生产里发现 XSS 的方式，而不必等一起事故。

### 检测与缓解

- **在输出点编码，并且知道上下文。** 每个上下文一个编码器 —— HTML 文本、HTML 属性、URL、JavaScript 字符串、CSS —— 由模板引擎选，或者由写出的那段代码选。这是主要控制，其余一切都在它后面。
- **用约定禁用危险的渲染 API，并把例外逐个点名。** `innerHTML`、`outerHTML`、`document.write`、`eval`、`Function`、`dangerouslySetInnerHTML`、`v-html`、`|safe`，以及 Go 里用于 HTML 的 `text/template`。一份简短的禁用清单**在评审和 lint 规则里是可执行的**，这比"注意编码"要强得多。
- **部署带 nonce 或 hash 的 CSP，并打开报告。** `script-src 'self' 'nonce-...'` 配 `object-src 'none'`、`base-uri 'self'` 能挡下真实 payload 的很大一部分，而报告端点给了你检测信号。把那些报告当安全信息流处理，而不是噪声：一行被注入的 `report-uri` 本身就是一条发现。
- **JavaScript 不需要的 cookie 一律加 `HttpOnly`。** 它不阻止 XSS；它阻止最常见的外泄路径生效，从而把事故从"会话被偷"变成"页面被操纵"。
- **记下到达响应的值，而不只是记请求。** 反射型 XSS 的有用信号是**响应里含有请求中未经编码的片段**。把请求参数与响应体做对比、并在某个参数原样出现在 HTML 或脚本上下文里时告警，能抓到没有签名认识的 payload。
- **记住 DOM 型 XSS 不在服务端留痕。** 对它来说，来源在客户端（URL 片段、`document.referrer`、`localStorage`、`postMessage`），汇点在客户端（`innerHTML`、`eval`、`document.write`、`location`）。所以检测来自 CSP 报告与客户端监控，而不是访问日志；而代码评审才是主要的发现手段。
- **不要依赖输入过滤，也不要把 WAF 当修法。** 两者在构造上就可被绕过，而浏览器对畸形 HTML 的宽容意味着绕过集合无法枚举。把它们保留为成本与检测；修法在输出点。
- **并且保持第一句里的那个框架**：这和 SQL 注入是同一个 bug，只是解释器不同。修法形状也一样 —— 把数据通道与指令通道分开 —— 而对浏览器来说，那种分离的实用形式就是**由上下文决定编码**，因为浏览器有不止一条指令通道。
