---
id: xss-filter-bypass-and-detection
title_en: "XSS, Part 3 — Filters, Bypasses and Detection"
title_zh: "XSS（三）：过滤器、绕过与检测"
summary_en: A filter is another parser, so bypassing one is the same exercise as bypassing anything else — find where its model and the browser's model diverge. This entry covers the three kinds of filter, the structural weakness of each, a reproducible experiment against a blacklist, and the detection that still works after a bypass.
summary_zh: 过滤器是另一个解析器，所以绕过它和绕过别的任何东西是同一件事 —— 找到它的模型与浏览器的模型在哪里分岔。这一篇讲三种过滤器、讲每一种的结构性弱点、讲一个针对黑名单的可复现实验，以及绕过成功之后依然管用的检测。
tags: [web, xss, filter-bypass, waf-bypass, csp, mxss]
tools: [browser devtools, Burp Suite, curl, DOMPurify]
attck: [T1059.007, T1027]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### A filter is another parser

The framing carries over from the language entries unchanged. A filter in front of a browser has the same problem the browser has: it must decide what a byte sequence means, and it may decide differently.

> **Bypassing a filter is not finding a string it forgot. It is finding an input where the filter's interpretation and the browser's interpretation diverge.**

That makes the exercise predictable instead of a payload lottery, and it also tells you something useful about defence: the *first* bypass found is rarely the interesting one. The interesting question is whether a given design **can** be bypassed at all, or merely has not been yet.

### Three kinds of filter, three failure modes

| Kind | How it works | Where it breaks |
|---|---|---|
| **Blacklist** | Matches and removes or rejects known-bad strings | The list is finite and the language is not; removal can also create new strings |
| **Sanitiser** | Parses the input, keeps an allowlist of elements and attributes, re-serialises | Parser differentials: the browser parses the *serialised* output and may build a different tree |
| **WAF at the HTTP layer** | Inspects requests before they reach the application | It is guessing at a request shape the application has not decided yet, and it sees fewer channels |

Every one of these has been defeated in practice, and each fails for a structural reason rather than a missing entry.

### Blacklists, and the two reasons they fail

**Reason one: the list is finite and the execution surface is not.** A blacklist that removes `<script>`, `<img>`, `onerror`, `onload`, `javascript:` and `alert(` has covered the payloads its author had seen. The browser still offers:

| Category | Surface the list usually misses |
|---|---|
| Elements that fire on load | `<iframe onload>`, `<video><source onerror>`, `<marquee onstart>`, `<object>`, `<embed>` |
| Elements that fire without interaction | `<details open ontoggle>`, `<input autofocus onfocus>`, `<body onpageshow>` |
| Events outside the common handful | `onanimationstart`, `onpointerover`, `onwheel`, `ontoggle`, `onfocus` |
| Functions outside the common handful | `eval`, `Function`, `fetch`, `print`, `confirm`, `prompt` |
| Alternative schemes | `data:` in an `iframe` or `object`, `blob:`, `filesystem:` |
| Attribute-driven execution | `srcdoc` on an iframe, `formaction`, `xlink:href` in SVG |

**Reason two: removal can concatenate.** This is the one that surprises people, and it is worth a concrete case. Take an input of

```
<scr<script>ipt>alert(1)</script>
```

A filter that deletes the substring `<script>` removes the inner one and produces

```
<script>alert(1)</script>
```

The filter **built the payload it was trying to remove.** Any defence that works by deleting a substring has this property, and the general form is that the filter's output is not a subset of its input's meanings.

#### A reproducible experiment

The value of doing this once by hand is that the two failure reasons become visible at the same time. The filter below is written to be typical rather than clever — a list, and deletion:

```python
# the shape of a home-made blacklist: delete the offending substring
BANNED = ['<script', '</script', '<img', '<svg', 'onerror', 'onload',
          'onfocus', 'javascript:', 'data:', 'alert']

def defend(q: str) -> str:
    for b in BANNED:
        q = q.replace(b, '')
    return q
```

Render the result into HTML text context (`<p>Hello {output}</p>`) and submit each of these, then look at what the page actually received:

| Input | After the filter | Verdict |
|---|---|---|
| `<script>alert(1)</script>` | `(1)` | blocked — both the tag and the function name are on the list |
| `<img src=x onerror=...>` | `(empty)` | blocked, because `<img` is on the list |
| `<scr<script>ipt>document.title='X'</script>` | `<script>document.title='X'` | **the filter created a working `<script>` opening tag** |
| `<input autofocus onfocus=document.title='X'>` | unchanged | **executes** — neither the element nor the event is on the list |
| `<iframe onload=document.title='X' src=about:blank>` | unchanged | **executes** |
| `<details open ontoggle="document.title='X'">` | unchanged | **executes** |

Two observations from the table that generalise:

- **The nested case is not a lucky guess.** It is what deletion does. Any list-based filter that removes a *prefix* such as `<script` can be made to assemble it from two halves.
- **The three working payloads are ordinary.** They are not obfuscated, not encoded, and use the simplest possible attributes. The filter is not being defeated by cleverness; it is being defeated because it enumerates a language with the wrong tool.

**And the correct fix is visible in the same experiment**: rendering the value with HTML entity encoding makes all six inputs inert, because none of them is a character sequence any more — they are text. That is the entire argument for output encoding over input filtering, demonstrated in one page.

### Sanitisers, and why they need parser-differential reasoning

A sanitiser is the right shape of defence: parse the input as HTML, drop everything not on an allowlist, serialise. The remaining risk is subtle and worth understanding.

**The sanitiser's output is a *string*, and the browser parses that string again.** If the two parses disagree, the second one decides what actually runs — and it is not the one that made the security decision.

This is **mutation XSS (mXSS)**, and the classical shapes involve contexts where the HTML parser's tree construction differs from a naive parse-then-serialise:

| Mechanism | Why the two parses differ |
|---|---|
| **Namespace confusion** | `<math>`, `<svg>` and `<table>` have different parsing rules, and serialising out of them can lose that context |
| **Raw-text elements** | `<noscript>`, `<style>`, `<textarea>`, `<title>` treat their content as text — until scripting is enabled or a state changes |
| **Re-parsing on innerHTML** | Content that is inert where it was parsed can become active when moved into a different element |
| **Double processing** | Sanitise, then do any further string manipulation, and the manipulation can re-create what the sanitiser removed |

The practical consequences for review and for testing:

- **A sanitiser is only as good as its version.** DOMPurify and its equivalents are maintained precisely because new differentials are found; "we use a sanitiser" is not a control unless it is current.
- **Never transform the output of a sanitiser with string operations.** Each such operation is a fresh chance to construct something the sanitiser had removed, and it moves the decision away from the parser that understood the tree.
- **Sanitise at the point of output, and only once.** Sanitising on input and rendering later is the wrong layer: the value may be used in several contexts, and only one of them was considered.
- **Where the platform offers a safer sink, use it.** Trusted Types plus a sanitising policy is the browser-enforced version of "always sanitise before assigning to `innerHTML`".

### HTTP-layer filters

A WAF sees the request, not the document, so its view is a guess about what the application will do with the bytes. That guess is where it fails, and the categories are the ones from the encoding entry applied to HTTP:

| Category | Shape |
|---|---|
| **Encoding** | Percent-encoding used twice, entity encoding the browser will decode, Unicode escapes |
| **Parameter interpretation** | Duplicate parameters, where the WAF and the framework disagree about which one wins |
| **Body handling** | Chunked transfer encoding, `Content-Type` changes, multipart boundaries |
| **Channels the filter cannot see** | The **URL fragment** (never sent), `postMessage`, WebSocket frames, and values fetched client-side before being written into the DOM |

That last row is why the DOM-based type is the one to test first against a defended target: a payload after the `#` never appears in any request the WAF inspects, so the WAF's verdict is not just weak, it is irrelevant.

### What still holds after a bypass

A bypass is only interesting in relation to what remains. Two controls are worth deploying precisely because they limit what a successful filter bypass achieves:

**Content Security Policy with a nonce.** A policy of `script-src 'self' 'nonce-<random>'` with `object-src 'none'` and `base-uri 'self'` makes every payload in the experiment above inert, including the ones that execute — because they are **inline**, and inline script without the nonce is not run. This is why the mitigation hierarchy matters: the bypass defeated the filter and did not defeat the policy.

| Defence | Bypasses it survives | Why |
|---|---|---|
| Context-aware output encoding | All of them | Nothing becomes code, so there is nothing to bypass |
| Sanitiser (current, used once) | Most, until a new differential | Parse-based rather than string-based |
| CSP with a nonce, no `unsafe-inline` | Inline payloads, which is most of them | The browser refuses to run them |
| Trusted Types | Sinks that would take a plain string | The sink requires a sanitised value |
| `HttpOnly` cookie | Nothing, but bounds the theft | The cookie is not readable from script |

**And the reports matter as much as the enforcement.** `report-to` turns every blocked attempt into a signal, which is how a bypass that *almost* worked becomes visible instead of silent.

### Detection and mitigation

- **Do not build a blacklist, and do not treat one as a control.** The experiment above is the argument: two structural failure modes, neither of which more entries can fix.
- **Use a maintained sanitiser, once, at the output, and never transform its result.** Add Trusted Types where the platform supports it so the dangerous sinks cannot take a raw string at all.
- **Deploy CSP with a nonce rather than `'unsafe-inline'`, and turn reporting on.** Enforce it on DOM-based XSS too — it is the only server-side control that reaches there.
- **Treat CSP reports as a high-value feed.** A `script-src` violation means something tried to execute and the policy stopped it. Group by the blocked URI and the violated directive: a repeated blocked inline script is either a live injection or a template that is still shipping.
- **Alert on the shapes of construction rather than on keywords.** Tags nested inside other tags (`<scr<script>ipt`), mixed-case end tags, entity-encoded keywords, `on*` attributes outside the common list, `srcdoc`, and unusual schemes. None is proof of an attack, and all of them indicate someone reasoning about your filter rather than about your application.
- **Test the fragment channel against a defended target.** If the filter is at the network layer, a payload placed after `#` never reaches it, and the question becomes what the client-side code does with `location.hash`. That is a code review question, not a request-filtering one.
- **For a WAF, evaluate rather than assume.** What does it decode, how many times, and does it match before or after normalising? A filter that matches the raw bytes is defeated by the encoding table in the earlier entry; one that matches after decoding is defeated by far less.
- **And keep the reason for caring about bypasses in view**: if a payload can execute, the capabilities from part 2 apply — reading the page, using the session, acting as the victim. The bypass is a step, not the finding.

<!-- lang:zh -->
### 过滤器是另一个解析器

这个框架从语言那几篇原样搬过来。浏览器前面的过滤器，面临和浏览器一样的问题：它必须判断一串字节是什么意思，而它可能判得不一样。

> **绕过过滤器不是找到它忘了的某个字符串，而是找到一个输入，让过滤器的解释和浏览器的解释产生分歧。**

这让整件事变得可预测，而不是一场 payload 抽奖；它也顺带告诉你一件关于防御的有用的事：**第一个**被找到的绕过，很少是有意思的那个。有意思的问题是：某个设计**能不能**被绕过，还是只是暂时还没被绕过。

### 三种过滤器，三种失效模式

| 种类 | 它怎么工作 | 它在哪里破 |
|---|---|---|
| **黑名单** | 匹配并删除或拒绝已知的坏串 | 清单是有限的而语言不是；删除还可能造出新串 |
| **净化器** | 把输入按 HTML 解析、按白名单保留元素与属性、再序列化 | 解析差异：浏览器解析的是**序列化后的输出**，可能建出另一棵树 |
| **HTTP 层的 WAF** | 在请求到达应用之前检查 | 它在猜一个应用还没决定的请求形状，而且它看到的通道更少 |

这三种在实践里都被打过，而且每一种都是**结构性**原因失败，不是"漏了一条"。

### 黑名单，以及它失败的两个原因

**原因一：清单是有限的，而执行面不是。** 一个删掉 `<script>`、`<img>`、`onerror`、`onload`、`javascript:`、`alert(` 的黑名单，覆盖的是作者见过的 payload。浏览器仍然提供：

| 类别 | 清单通常漏掉的攻击面 |
|---|---|
| 加载即触发的元素 | `<iframe onload>`、`<video><source onerror>`、`<marquee onstart>`、`<object>`、`<embed>` |
| 无需交互就触发的元素 | `<details open ontoggle>`、`<input autofocus onfocus>`、`<body onpageshow>` |
| 那几种常见事件之外的事件 | `onanimationstart`、`onpointerover`、`onwheel`、`ontoggle`、`onfocus` |
| 那几种常见函数之外的函数 | `eval`、`Function`、`fetch`、`print`、`confirm`、`prompt` |
| 替代协议 | `iframe`/`object` 里的 `data:`、`blob:`、`filesystem:` |
| 由属性驱动的执行 | iframe 的 `srcdoc`、`formaction`、SVG 里的 `xlink:href` |

**原因二：删除会拼接。** 这一条最让人意外，值得给个具体例子。输入

```
<scr<script>ipt>alert(1)</script>
```

一个删除子串 `<script>` 的过滤器会删掉里面那个，于是产出

```
<script>alert(1)</script>
```

**过滤器亲手拼出了它要移除的 payload。** 任何靠删除子串工作的防御都有这个性质，而通用形式是：**过滤器的输出，并不是它输入含义的子集。**

#### 一个可复现的实验

手工做一次的价值在于，那两个失败原因会同时变得可见。下面这个过滤器写得"典型"而不是"聪明"—— 一份清单，加删除：

```python
# 自己写的黑名单的形状：把违规子串删掉
BANNED = ['<script', '</script', '<img', '<svg', 'onerror', 'onload',
          'onfocus', 'javascript:', 'data:', 'alert']

def defend(q: str) -> str:
    for b in BANNED:
        q = q.replace(b, '')
    return q
```

把结果渲染进 HTML 文本上下文（`<p>Hello {output}</p>`），逐个提交下面这些，然后看页面实际收到了什么：

| 输入 | 过滤之后 | 结论 |
|---|---|---|
| `<script>alert(1)</script>` | `(1)` | 挡住 —— 标签和函数名都在清单上 |
| `<img src=x onerror=...>` | `（空）` | 挡住，因为 `<img` 在清单上 |
| `<scr<script>ipt>document.title='X'</script>` | `<script>document.title='X'` | **过滤器造出了一个能用的 `<script>` 开标签** |
| `<input autofocus onfocus=document.title='X'>` | 不变 | **执行** —— 元素与事件都不在清单上 |
| `<iframe onload=document.title='X' src=about:blank>` | 不变 | **执行** |
| `<details open ontoggle="document.title='X'">` | 不变 | **执行** |

表里有两个能推广的观察：

- **嵌套那个不是碰运气。** 那就是删除做的事。任何删除 `<script` 这类**前缀**的清单式过滤器，都可以被从两半拼出来。
- **那三个能用的 payload 很普通。** 它们没有混淆、没有编码、用的是最简单不过的属性。过滤器不是被聪明打败的，而是因为它**用错了工具去枚举一门语言**。

**而正确的修法在同一个实验里就看得见**：用 HTML 实体编码渲染那个值，六个输入全部变成哑弹 —— 因为它们不再是任何字符序列，它们是文本。这就是"输出编码优于输入过滤"的全部论证，在一张页面里演示完了。

### 净化器，以及为什么它需要"解析差异"的推理

净化器是**形状正确**的防御：把输入按 HTML 解析、丢掉白名单之外的一切、再序列化。剩下的风险很微妙，值得理解。

**净化器的输出是一个*字符串*，而浏览器会再解析那个字符串一次。** 如果两次解析不一致，**后一次决定实际跑什么** —— 而做安全决策的不是它。

这就是**突变 XSS（mXSS）**，而经典形状都涉及"HTML 解析器的树构建与天真的先解析再序列化不同"的那些上下文：

| 机制 | 为什么两次解析不同 |
|---|---|
| **命名空间混淆** | `<math>`、`<svg>`、`<table>` 各有不同的解析规则，从它们里面序列化出去会丢掉那个上下文 |
| **原始文本元素** | `<noscript>`、`<style>`、`<textarea>`、`<title>` 把自己的内容当文本 —— 直到脚本被启用、或者某个状态变化 |
| **innerHTML 引起的再解析** | 在某个位置无害的内容，被移动到另一个元素里就可能变成活的 |
| **二次处理** | 净化之后再做任何字符串操作，那个操作就可能重建被净化器删掉的东西 |

对评审和测试的实用结论：

- **净化器只和它的版本一样好。** DOMPurify 及其同类之所以在持续维护，正是因为不断有新的差异被发现；"我们用了净化器"不是一项控制，除非它是最新的。
- **绝不要用字符串操作去变换净化器的输出。** 每一次这样的操作都是一次重新拼出已被移除之物的机会，而且它把决定权从"理解那棵树"的解析器那里挪走了。
- **在输出点净化，而且只净化一次。** 在输入时净化、之后才渲染，是错的层次：那个值可能在好几个上下文里被使用，而只有一个被考虑过。
- **平台提供更安全的汇点时就用它。** Trusted Types 配一条净化策略，就是"赋值给 `innerHTML` 前总要净化"的浏览器强制版本。

### HTTP 层的过滤器

WAF 看到的是请求，不是文档，所以它的视图是关于"应用会拿这些字节做什么"的一个猜测。那个猜测就是它失败的地方，而类别就是编码那篇里的那些，用在 HTTP 上：

| 类别 | 形状 |
|---|---|
| **编码** | 百分号编码用两次、浏览器会解码的实体编码、Unicode 转义 |
| **参数解释** | 重复参数 —— WAF 与框架对"哪一个胜出"意见不一致 |
| **请求体处理** | 分块传输编码、`Content-Type` 变化、multipart 边界 |
| **过滤器看不到的通道** | **URL 片段**（从不发送）、`postMessage`、WebSocket 帧，以及客户端先取出再写进 DOM 的值 |

最后一行就是为什么**对一个有防御的目标，该先测 DOM 型**：`#` 之后的 payload 从不出现在 WAF 检查的任何请求里，所以 WAF 的判定不只是弱，而是**不相干**。

### 绕过之后，还剩下什么

绕过只有相对于"还剩下什么"才有意义。有两项控制正因为能限制"成功绕过过滤器所达成的效果"而值得部署：

**带 nonce 的 CSP。** 一条 `script-src 'self' 'nonce-<随机>'` 配 `object-src 'none'`、`base-uri 'self'` 的策略，会让上面实验里每一个 payload 变成哑弹 —— **包括那些执行了的** —— 因为它们是**内联**的，而没有 nonce 的内联脚本不会运行。这就是缓解层级要紧的原因：绕过击败了过滤器，却没有击败策略。

| 防御 | 它能扛住哪些绕过 | 为什么 |
|---|---|---|
| 上下文感知的输出编码 | 全部 | 没有东西变成代码，也就没有东西可绕 |
| 净化器（最新、只用一次） | 大多数，直到出现新的差异 | 基于解析而不是基于字符串 |
| 带 nonce、不含 `unsafe-inline` 的 CSP | 内联 payload —— 也就是大多数 | 浏览器拒绝运行它们 |
| Trusted Types | 那些会接受普通字符串的汇点 | 汇点要求一个被净化过的值 |
| `HttpOnly` cookie | 一个都挡不住，但限制了窃取范围 | 脚本读不到那个 cookie |

**而报告和执行一样要紧。** `report-to` 把每一次被拦下的尝试变成信号，这正是"差一点成功的绕过"从静默变成可见的方式。

### 检测与缓解

- **不要写黑名单，也不要把黑名单当控制。** 上面那个实验就是论据：两个结构性失效模式，再加多少条都修不好。
- **用维护中的净化器，在输出点用一次，绝不变换它的结果。** 在平台支持的地方加上 Trusted Types，让那些危险的汇点根本无法接受裸字符串。
- **部署带 nonce 的 CSP 而不是 `'unsafe-inline'`，并打开报告。** 对 DOM 型 XSS 也一样强制 —— 它是唯一能到达那里的服务端控制。
- **把 CSP 报告当作高价值信息流。** 一条 `script-src` 违规意味着有东西试图执行、而策略拦住了它。按被拦的 URI 与违规指令分组：反复出现的被拦内联脚本，要么是一次活的注入，要么是一个仍在线上跑的模板。
- **对"构造的形状"告警，而不是对关键词。** 标签套标签（`<scr<script>ipt`）、大小写混杂的结束标签、实体编码的关键词、常见清单之外的 `on*` 属性、`srcdoc`、不寻常的协议。没有一条能证明是攻击，而每一条都说明有人在**推理你的过滤器**，而不是推理你的应用。
- **对已经设防的目标，专门测片段通道。** 如果过滤在网络层，放在 `#` 之后的 payload 根本到不了它，问题就变成"客户端代码拿 `location.hash` 做了什么"。那是代码评审问题，不是请求过滤问题。
- **对手上的 WAF，去评估而不是假设。** 它解码什么、解几次、是在规范化之前还是之后匹配？匹配原始字节的过滤器会被编码那篇里的表打穿；解码之后才匹配的，能打穿它的手段少得多。
- **并且记住关心绕过的原因**：如果 payload 能执行，第二篇里的那些能力就都适用 —— 读页面、用会话、以受害者身份行动。绕过是一步，不是那个发现本身。
