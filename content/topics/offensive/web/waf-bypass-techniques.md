---
id: waf-bypass-techniques
title_en: WAF Bypass Techniques
title_zh: WAF 绕过
summary_en: A WAF is a second parser sitting in front of your target. Every bypass is a disagreement between what that parser sees and what the application sees — encoding it decodes differently, a body it does not inspect, a request it cannot agree on with the backend.
summary_zh: WAF 是坐在目标和外网之间的第二个解析器。所有绕过本质上都是"它看到的"和"应用看到的"不一致 —— 它解码方式不同的编码、它不检查的请求体、它与后端无法达成一致的请求。
tags: [web, waf, bypass, evasion, request-smuggling, bugbounty]
tools: [wafw00f, Burp Suite, ffuf, smuggler]
attck: [T1190]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The principle worth internalising

You are not looking for a flaw in the WAF. You are looking for a **place where two parsers disagree** about the same bytes:

- The WAF decodes and normalises the request, then matches rules against its version of it.
- The application (or framework, or reverse proxy, or database driver) decodes it again, in its own way.

Anything that makes those two versions differ is a bypass: an encoding only one of them expands, a body one of them does not read, a parameter one of them picks and the other ignores, a request boundary they disagree about.

That framing is also the fix, which is why defenders should read this section too: **if the WAF and the application parse identically, most of these techniques evaporate.**

### Step 1 — Know what you are up against

```bash
wafw00f https://target.example
```

Fingerprints worth learning: a `Server` header that names the vendor, vendor-specific cookies and headers (`cf-ray`, `x-sucuri-id`, `x-akamai-*`), a distinctive block page, and a 403 that appears only for certain payloads. Sending one obviously malicious request and one harmless one tells you whether rules are active and how strictly they fire.

Also note where the WAF sits: at the CDN, at the load balancer, as a module in the web server, or as an SDK inside the application. An SDK WAF sees the parameter after the framework has already parsed it, which changes what you must bypass.

### Step 2 — Encoding

The first layer, and the one that most often works, because vendors differ on how many times they decode:

| Technique | Example | Why it can slip through |
|---|---|---|
| URL encoding | `%27` for `'` | Baseline; most WAFs decode this |
| Double encoding | `%2527` | WAF decodes once to `%27`, the app decodes twice to `'` |
| Unicode / overlong UTF-8 | `%c0%a7`, `%u0027` | Decoders disagree on validity and normalisation |
| UTF-16 / UTF-7 body | `Content-Type: application/json; charset=utf-16` | Rules written against UTF-8 miss the markers entirely |
| Hex and base64 in SQL | `0x61646d696e`, `FROM_BASE64('...')` | The dangerous text never appears literally |
| HTML entities | `&lt;script&gt;` | Matters where the value is decoded before use |

The pattern to test: encode once, twice, three times, and see which layer normalises. The layer that stops normalising before the application does is your bypass.

### Step 3 — Syntax

Once encoding is exhausted, the question becomes "what does this payload *mean* to the backend, and does the rule recognise that form?"

- **Comments as separators.** `UNION/**/SELECT`, `/*!50000SELECT*/` (a MySQL version-conditional comment that executes), `--` line comments.
- **Whitespace substitution.** `%09`, `%0a`, `%0b`, `%0c`, `+`, and in some parsers nothing at all (`UNION(SELECT(...))`).
- **Case and keyword splitting.** `UnIoN SeLeCt`, `SEL/**/ECT`.
- **Equivalent operators.** `||` for `OR`, `&&` for `AND`, `LIKE` instead of `=`, `BETWEEN` instead of comparisons.
- **Function and literal alternatives.** `CHAR(0x27)`, `CONCAT()`, hex literals, `CURRENT_USER()` instead of `user()`.
- **Different payload, same effect.** If `<script>` is blocked but `<svg onload=` is not, the rule is incomplete rather than the vulnerability being unfixable.

This is the layer where knowing the target's stack matters most: `/*!50000*/` only helps on MySQL, `%00` only on stacks that still truncate, and JSON-specific escapes only where JSON is parsed.

### Step 4 — Protocol level

These bypasses are structural rather than textual, and they are the most reliable because they do not depend on the rule set at all:

- **Chunked transfer encoding.** A body split into chunks may be inspected chunk by chunk, or not at all.
- **Request smuggling** (CL.TE, TE.CL). When the WAF and the backend disagree about where a request ends, one of them sees the injected content as part of the next request — which often skips inspection entirely. Test with `smuggler` and confirm carefully: smuggling can poison other users' connections.
- **HTTP/2 downgrade.** A request that arrives as HTTP/2 and is forwarded as HTTP/1.1 can be reshaped in transit; some WAFs inspect the h2 form and the backend sees the h1 form.
- **Content type switching.** The same parameter sent as `application/x-www-form-urlencoded`, then `application/json`, then `multipart/form-data` — different rules cover different bodies.
- **Parameter pollution.** `?id=1&id=<payload>`: the validator reads one value, the application reads the other.
- **Path confusion.** `/admin/..;/`, `//admin`, `/./admin`, trailing dots and spaces, case variation, and encoded slashes. Rules match paths as strings; routers normalise them.

### Step 5 — Put the payload where it is not inspected

Sometimes the simplest bypass is a different location:

- A JSON field nested deeper than the rule covers, or an array where a string is expected.
- Headers that end up in logs, redirects or internal requests: `X-Forwarded-For`, `X-Original-URL`, `User-Agent`, `Referer`.
- Cookies, which are often covered by fewer rules than bodies.
- Uploaded file contents and their metadata, including filenames.
- The path itself, when a rule only inspects parameters.

The general question to ask of every endpoint: **which parts of my request does the application actually use, and are all of those parts inspected?** The answer is usually not all.

### Vendor notes, briefly

Cloudflare, AWS WAF, Akamai and ModSecurity CRS each have preferences — ModSecurity CRS has a paranoia level that decides how aggressive its rules are, AWS WAF is rule-based and inherits the mistakes of whoever wrote the rules, and CDN-based WAFs often normalise aggressively, which is why protocol-level and encoding tricks behave differently against them.

Do not memorise vendor payload lists: they age within weeks. Memorise the four layers above and test which one the target is weak at.

### Detection

WAF bypass is a signal, not a failure of the WAF:

- **Encoded payloads that decode to something malicious** are visible in logs even when the rule does not fire; alert on the decoded form, not just the raw.
- **Repeated 403s followed by a 200 on the same endpoint** by one client is a bypass being found.
- **Chunked bodies, unusual `Content-Type`, and malformed `Content-Length`** are worth logging at the edge.
- **Request smuggling needs both parsers configured identically** — the real fix is normalising at the edge and rejecting ambiguous requests rather than trying to detect them.
- **The WAF is a speed bump.** Every technique in this file is a reminder that the control is at the application: parameterised queries, output encoding, object-level authorization and allow-lists do not care what encoding you used.

### Mitigation

- **Fix the root cause, not the request.** A blocked payload is a blocked symptom; the injection, the traversal or the XSS is still there for the next encoding.
- **Normalise once, at the edge, and reject ambiguity.** If the edge fully decodes and the backend receives the decoded form, most encoding tricks stop working.
- **Reject requests you do not understand** rather than passing them through: conflicting `Content-Length` and `Transfer-Encoding`, malformed chunking, unexpected `Content-Type`.
- **Do not rely on a block-list of payloads.** Allow-list input shapes at the application layer, where you know what the parameter is supposed to be.
- **Keep the WAF in front of everything**, including the paths and methods you think are internal — bypasses love the endpoint nobody configured.
- **Log the decoded request** so an attempt is visible even when it succeeds, and alert on the patterns above.

<!-- lang:zh -->
### 值得先吃透的原则

你要找的不是 WAF 的缺陷，而是**两个解析器对同一串字节理解不一致的地方**：

- WAF 对请求做解码与规范化，然后拿它自己那一版去匹配规则。
- 应用（或框架、反向代理、数据库驱动）会用自己的方式再解码一次。

任何让这两版产生差异的东西都是绕过：只有一方会展开的编码、只有一方会读的请求体、一方取用而另一方忽略的参数、双方对边界理解不同的请求。

这个框架同时也是修复思路 —— 所以防守方也该读这一节：**如果 WAF 和应用的解析方式完全一致，下面大部分手法就自然失效了。**

### 第一步 —— 先搞清楚面对的是什么

```bash
wafw00f https://target.example
```

值得记住的指纹：直接写明厂商的 `Server` 头、厂商特有的 cookie 与 header（`cf-ray`、`x-sucuri-id`、`x-akamai-*`）、特征鲜明的拦截页，以及只对特定 payload 出现的 403。送一个明显恶意的请求加一个无害请求，就能知道规则是否生效、触发得有多严。

还要注意 WAF 的位置：在 CDN、在负载均衡、作为 Web 服务器模块，还是作为应用内 SDK。SDK 型 WAF 看到的是框架已经解析过的参数，这直接改变你需要绕过的东西。

### 第二步 —— 编码

第一层，也是最常奏效的一层，因为各家对"解码几次"的处理不一致：

| 手法 | 例子 | 为什么可能溜过去 |
|---|---|---|
| URL 编码 | `'` 写成 `%27` | 基线；多数 WAF 会解码 |
| 双重编码 | `%2527` | WAF 解一次得到 `%27`，应用解两次得到 `'` |
| Unicode / 过长 UTF-8 | `%c0%a7`、`%u0027` | 各家对合法性与规范化的判断不同 |
| UTF-16 / UTF-7 请求体 | `Content-Type: application/json; charset=utf-16` | 针对 UTF-8 写的规则完全看不到标记 |
| SQL 里的十六进制与 base64 | `0x61646d696e`、`FROM_BASE64('...')` | 危险文本从不字面出现 |
| HTML 实体 | `&lt;script&gt;` | 值在被使用前先被解码的场景 |

要测的模式是：编一次、两次、三次，看哪一层先停止规范化。在应用之前就不再规范化的那一层，就是你的突破口。

### 第三步 —— 语法

编码用尽之后，问题变成："这个 payload 对后端**意味着**什么，而那条规则认不认这种形式？"

- **用注释当分隔符。** `UNION/**/SELECT`、`/*!50000SELECT*/`（MySQL 的版本条件注释，会执行）、`--` 行注释。
- **替换空白。** `%09`、`%0a`、`%0b`、`%0c`、`+`，某些解析器里甚至可以什么都没有（`UNION(SELECT(...))`）。
- **大小写与关键字拆分。** `UnIoN SeLeCt`、`SEL/**/ECT`。
- **等价运算符。** `OR` 写成 `||`、`AND` 写成 `&&`、`=` 写成 `LIKE`、比较写成 `BETWEEN`。
- **函数与字面量替代。** `CHAR(0x27)`、`CONCAT()`、十六进制字面量、`CURRENT_USER()` 代替 `user()`。
- **换一种 payload，效果相同。** `<script>` 被拦但 `<svg onload=` 没被拦，说明规则不完整，而不是漏洞无法修复。

这一层最吃对目标技术栈的了解：`/*!50000*/` 只在 MySQL 有用，`%00` 只在还会截断的技术栈上有用，JSON 专属转义只在真正解析 JSON 的地方有用。

### 第四步 —— 协议层

这些绕过是结构性的而非文本性的，因此最可靠 —— 它们根本不依赖规则集：

- **分块传输编码。** 被拆成块的请求体可能被逐块检查，也可能完全不被检查。
- **请求走私**（CL.TE、TE.CL）。当 WAF 与后端对"请求到哪结束"理解不一致时，其中一方会把注入内容当成下一个请求的一部分 —— 于是往往完全跳过检查。用 `smuggler` 测试并谨慎确认：走私可能污染其他用户的连接。
- **HTTP/2 降级。** 以 HTTP/2 到达、以 HTTP/1.1 转发的请求会在中途被重塑；有些 WAF 检查的是 h2 形态，后端看到的是 h1 形态。
- **切换内容类型。** 同一个参数分别用 `application/x-www-form-urlencoded`、`application/json`、`multipart/form-data` 发送 —— 不同规则覆盖不同请求体。
- **参数污染。** `?id=1&id=<payload>`：校验器读一个值，应用读另一个。
- **路径混淆。** `/admin/..;/`、`//admin`、`/./admin`、结尾的点与空格、大小写变化、编码斜杠。规则按字符串匹配路径，路由器却会做规范化。

### 第五步 —— 把 payload 放到不被检查的地方

有时最简单的绕过就是换个位置：

- 嵌套得比规则覆盖更深的 JSON 字段，或者在该是字符串的地方给数组。
- 最终会进入日志、跳转或内部请求的 header：`X-Forwarded-For`、`X-Original-URL`、`User-Agent`、`Referer`。
- Cookie —— 覆盖它的规则通常比请求体少。
- 上传文件的内容与元数据，包括文件名。
- 路径本身，当规则只检查参数的时候。

对每个接口都该问一句：**我的请求里哪些部分真的会被应用使用，而它们是否都被检查了？** 答案通常是"不是全部"。

### 厂商特点（简略）

Cloudflare、AWS WAF、Akamai 与 ModSecurity CRS 各有偏好 —— ModSecurity CRS 有 paranoia level 决定规则的激进程度，AWS WAF 是规则驱动、会继承写规则人的疏漏，而 CDN 型 WAF 往往做很激进的规范化，所以协议层与编码类手法在它们身上的表现不一样。

不要背厂商 payload 清单：它们几周就过时。记住上面四层，然后测出目标弱在哪一层。

### 检测

WAF 绕过是一个信号，而不是 WAF 的失败：

- **编码过、但解码后是恶意的 payload** 即便规则没触发，在日志里也看得见；要对解码后的形态告警，而不只是原始形态。
- **同一客户端在同一接口上连续 403 之后突然 200**，就是有人在找绕过。
- **分块请求体、异常 `Content-Type`、畸形的 `Content-Length`** 都值得在边缘记录。
- **请求走私需要两个解析器配置一致**才能根治 —— 真正的修复是在边缘做规范化并拒绝有歧义的请求，而不是试图检测它们。
- **WAF 是减速带。** 这一节的每种手法都在提醒：控制点在应用侧 —— 参数化查询、输出编码、对象级授权和白名单，根本不在乎你用了什么编码。

### 缓解

- **修根因，而不是修请求。** 一个被拦住的 payload 只是被拦住的症状；注入、穿越或 XSS 仍然在那里等着下一种编码。
- **在边缘规范化一次，并拒绝歧义。** 如果边缘完整解码、后端收到的就是解码后的形态，大多数编码技巧立刻失效。
- **看不懂的请求就拒绝**，不要放行：`Content-Length` 与 `Transfer-Encoding` 冲突、畸形分块、意外的 `Content-Type`。
- **不要依赖 payload 黑名单。** 在应用层对输入做形态白名单，因为那里你才知道这个参数本来该长什么样。
- **让 WAF 覆盖所有东西**，包括你以为"内部"的路径与方法 —— 绕过最喜欢那个没人配置过的接口。
- **记录解码后的请求**，这样即便攻击成功也留有痕迹，并对上面的模式告警。
