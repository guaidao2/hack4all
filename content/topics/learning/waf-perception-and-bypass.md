---
id: waf-perception-and-bypass
title_en: "WAF — Perception, Bypass Methodology and Evaluation"
title_zh: "WAF：视野、绕过方法论与评估"
summary_en: A WAF sits outside the application and therefore has to guess what the application will see, which makes every bypass in this series an instance of one of five gaps. This entry names them, measures a differential test against a purpose-built filter, and argues that the useful role for a WAF is sensing rather than blocking.
summary_zh: WAF 在应用之外，因此必须去猜应用会看到什么 —— 这让本系列里的每一个绕过都成为那五条缝之一的实例。这一篇把它们命名出来，对一个专门搭出来的过滤器做一次差分测试，并论证 WAF 有用的角色是"传感器"而不是"墙"。
tags: [web, waf, bypass, modsecurity, detection, differential-testing]
tools: [curl, python3, Burp Suite, ModSecurity]
attck: [T1190, T1027]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The one sentence that explains the whole class

Everything in this entry follows from where a WAF sits:

> **A WAF is outside the application, so it does not see what the application sees. It sees the request, and it has to guess what the application will make of it.**

That guess is the vulnerability surface. The application decodes, normalises, parses and routes; the WAF does its own version of those steps, usually with different code, sometimes with different settings, and always with less information about what happens next. Every bypass in the previous six entries is an instance of the two disagreeing.

The contrast that makes it concrete is **RASP** — protection inside the application runtime rather than in front of it:

| | WAF (in front) | RASP (inside) |
|---|---|---|
| Sees | The raw request | The value the application is about to use |
| Decoding | Guessed | Known — it is the application's own decoder |
| Normalisation | Its own rules | The same rules the application applies |
| Bypass surface | Large | Much smaller |
| Fails when | Its guess is wrong | The application's own logic is wrong |

The comparison also says what a WAF cannot be: **it is not a place where a decision can be made correctly for all time**, because the correctness depends on a system it does not control. What it can be is a filter, a cost, and a sensor.

### The five gaps, and every bypass in this series belongs to one

This is the unifying section. Each family of bypasses from the previous entries reduces to one of five questions, and the questions are cheap to ask about any defence.

#### Gap 1 — the decode count

> **How many times does the filter decode, and how many times does the application?**

If the numbers differ, there is a bypass, and the number of encoding layers needed is exactly the difference.

| Filter decodes | Application decodes | Input that works |
|---|---|---|
| 1 | 2 | Double encoding: `%252e%252e%252f` becomes `%2e%2e%2f` at the filter and `../` at the application |
| 1 | 1 | Nothing from this gap — look at the others |
| 0 | 1 | Single encoding the filter never looks at |

This is the gap measured in the path-traversal, LFI, XSS and XXE entries, and it is the first thing to determine about any defence, because it is a question with a numeric answer.

#### Gap 2 — the representation

> **How many ways are there to write the same thing, and does the filter enumerate them?**

| Domain | Representations of one meaning |
|---|---|
| IPv4 address | `127.0.0.1`, `127.1`, `0177.0.0.1`, `0x7f000001`, `2130706433`, `0` |
| SQL syntax | Keyword case, comments as whitespace, `+` versus space, equivalent operators |
| HTML execution | Many elements, events and schemes rather than one |
| File extension | Case, multiple extensions, alternative handlers |
| Character encoding | Entities, Unicode escapes, overlong forms, alternate charsets |

A blacklist here fails for a structural reason rather than an incomplete list: **it enumerates representations while the meaning is singular.** No amount of additional entries changes the ratio.

#### Gap 3 — structure versus string

> **Does the filter reason about a string, while the application reasons about a structure?**

| The filter sees | The application sees |
|---|---|
| A URL containing a hostname it recognises | A URL whose **host** is somewhere else (`user@host`) |
| An archive member with an innocent name | A **path** that resolves upward |
| A multipart part with a `filename` | An extended `filename*` it prefers |
| A request with two parameters | One of them, by a rule the filter does not know |

The substring check is the archetype: **"the URL contains this string" and "the host is this" are different questions**, and only one of them is the question that matters.

#### Gap 4 — the timing

> **Is the thing that was checked still the thing that is used?**

| Attack | What changed in between |
|---|---|
| DNS rebinding | The name resolved to a different address |
| Upload check-then-use | The file was reachable before the verdict |
| Deserialisation hooks | The code ran during the read, not after |
| TOCTOU on any resource | The resource was replaced |

The fix is never a better check; it is **ordering** — resolve once and use the resolved value, validate before the destination is reachable, treat the read as the dangerous moment.

#### Gap 5 — the channel

> **Can the filter see this direction at all?**

| Channel | Why the WAF misses it |
|---|---|
| URL fragment | Browsers do not send it |
| WebSocket frames | Outside the HTTP request path it inspects |
| Client-side sources | `localStorage`, `postMessage`, `document.referrer` |
| **Outbound traffic** | **The payload is not in the request; it is in what the server sends out** |
| Encrypted or tunneled content | Not visible at the inspected layer |

The outbound row is the one worth emphasising, because it inverts the model: in SSRF and in blind XXE, **the request is benign and the damage leaves the server in the other direction.** An ingress filter has no view of that at all, which is why the SSRF entry put outbound monitoring first.

### Failure modes of the WAF itself

Beyond being bypassed by design, filters fail in ways that are simple implementation choices. The two that matter most:

**Fail-open.** When the filter cannot decide — a timeout, a body over its inspection limit, a parse error — many deployments **allow the request**. That is a defensible availability choice and a poor security one, and the point is that it is a **choice**, often one nobody remembers making.

**Partial coverage.** Requests are inspected in the body but not in headers, cookies or the path; or up to a size limit and not beyond; or only at one layer of a chain of proxies. Each gap is a place where the filter's view and the application's view differ.

Measured on a deliberately simple filter with a body limit and a timeout:

| Request | Filter behaviour | Application behaviour |
|---|---|---|
| Payload over the inspection limit | **allowed without inspection** | its own check applied |
| Payload under the limit | blocked by rule | its own check applied |

The first row is the failure mode in one line: **the filter stopped checking, and only the application's own defence was left.**

### A differential test, and why it is the right method

The method for evaluating any filter is not a payload list. It is a comparison:

> **Send the same input twice — once through the filter, once directly to the application — and compare the outcomes. The difference is what the filter is contributing; the sameness is what it is not.**

Measured against a purpose-built pair: an application that decodes twice and applies its own check, and a filter in front that decodes once and matches a blacklist.

| Input | Direct to application | Through the filter | Reading |
|---|---|---|---|
| `../etc/passwd` | application refused | **403 — filter blocked** | filter working |
| `%2e%2e%2fetc%2fpasswd` | application refused | **403 — filter blocked** | filter working |
| `%252e%252e%252fetc%252fpasswd` | application refused | **application refused** (filter passed it) | **filter bypassed, application held** |
| `%25252e%25252e%25252fetc` | application accepted (still encoded after two decodes) | same | neither layer involved |
| `<script>alert(1)</script>` | application refused | **403 — filter blocked** | filter working |
| `%253Cscript%253E` | **application accepted** | **filter passed it** | **both layers missed it** |

Three readings from one table, and they are the three things an evaluation should produce:

**The filter contributes something.** Two rows show it blocking what the application also blocked, which is cost rather than value — but it does stop the request earlier, which is where an alert comes from.

**The filter is bypassable by encoding.** The double-encoded row is the decode-count gap, and it is exactly what the source-reading and traversal entries measured against their own filters.

**The real finding is a row where both layers miss.** The double-encoded script tag passed the filter *and* satisfied the application, which is the only row that represents an exploitable gap — and note that it was found by comparing outcomes, not by trying a longer payload list.

That last point is the practical argument for the method: **a payload list tests what the filter knows; a differential test measures what it does.**

### Evaluating a filter, in order

| Question | How to answer it |
|---|---|
| **Where does it sit?** | Reverse proxy, cloud edge, or inside the runtime — this decides everything else |
| **How many times does it decode?** | Send one, two and three times encoded forms of the same payload |
| **Which positions does it inspect?** | Move the same payload through path, query, body, headers and cookies |
| **What is its size limit?** | Grow the body until behaviour changes |
| **What does it do when unsure?** | Oversized or malformed bodies, and requests that make it time out |
| **Does it see outbound traffic?** | Its presence tells you nothing about SSRF or outbound exfiltration |
| **What does it log, and is that log read?** | An unread log is the same as no filter |

None of those questions is about a payload, and all of them are answerable in an afternoon.

### What a WAF is actually good for

Being bypassable does not make it useless; it makes its value specific.

| It does help with | It does not help with |
|---|---|
| Commodity scanners and mass exploitation attempts | A targeted attack by someone who has read this series |
| Known exploits, during the window before a patch | Novel or logic vulnerabilities |
| Raising the cost and the noise for an opportunistic attacker | Anything on an outbound channel |
| Generating a signal about who is probing | Being the only control in front of a vulnerable application |

The honest framing, and the one this entry argues for:

> **Use a WAF as a sensor rather than as a wall.** Its blocking can be bypassed by construction; its **logs** record that somebody tried, in what shape, and how often — and that is information nothing else in the stack produces.

Which changes what "configuring it well" means:

- **Cover every input position**, because a filter that inspects only the body is a filter that teaches an attacker where to put the payload.
- **Prefer fail-closed, or make fail-open visible.** If the choice is availability, instrument it: a request that bypassed inspection should be logged as such.
- **Turn on logging, and read it.** Alerts on rule distribution, on new rules firing, and on requests that were allowed after a filter failure are the return on the deployment.
- **Feed the application's logs alongside the WAF's.** The most informative event in the pair is a request the WAF blocked **and the application processed anyway** — that is a divergence between two components, and this series has been about divergences from the first entry.

### Detection and mitigation

- **Treat WAF alerts as a signal about intent, and application logs as the signal about outcome.** A rule firing tells you the shape of the attempt; the application's own log tells you whether anything reached it. Correlating the two is what distinguishes a blocked scan from a bypass that worked.
- **Alert on divergences.** A request logged as blocked at the edge and as successful at the application is the single most valuable event in the pair, because it means the two components disagree about the same bytes — which is the definition of every bypass in this series.
- **Alert on fail-open events explicitly.** Timeouts, oversized bodies and inspection errors should produce a log line that is distinguishable from a normal allow, so that the window is measurable rather than invisible.
- **Do not treat rule coverage as a measure of protection.** Count of rules, or of blocked requests, measures activity. The measurable question is the differential one: does the same input behave differently with the filter in place.
- **Put the real controls where the application can see the value**: parameterised queries, context-aware output encoding, storing uploads where nothing executes, refusing the DOCTYPE, identifiers instead of paths, resolving and pinning addresses. Every one of those works because it is applied at the layer that understands the data — the property a WAF, by position, cannot have.
- **Keep the outbound direction covered by something else.** Egress filtering and outbound logging catch the classes whose payload never enters the request, and no ingress filter will.
- **And measure the library, not the label.** "We have a WAF" and "we have a WAF configured to inspect every position, fail closed, and log divergences" are different statements, and only the second is a control. The differential test takes an afternoon and answers it.

<!-- lang:zh -->
### 一句解释这一整类的话

这一篇里的一切都源自 WAF 所处的位置：

> **WAF 在应用之外，所以它看不到应用看到的东西。它看到的是请求，而它必须去猜应用会把它理解成什么。**

那个"猜"就是漏洞面。应用会解码、规范化、解析、路由；WAF 用自己的一套做这些步骤，代码通常不同、设置有时不同，而对"接下来会发生什么"的信息总是更少。前面六篇里的每一个绕过，都是这两者不一致的一个实例。

让这件事变得具体的对照是 **RASP** —— 保护在应用运行时**之内**，而不是在它前面：

| | WAF（在前面） | RASP（在里面） |
|---|---|---|
| 看到 | 原始请求 | 应用即将使用的那个值 |
| 解码 | 猜的 | 已知 —— 就是应用自己的解码器 |
| 规范化 | 自己的一套规则 | 与应用相同的规则 |
| 绕过面 | 大 | 小得多 |
| 何时失败 | 当它猜错 | 当应用自己的逻辑错了 |

这个对照也说出了 WAF 不能是什么：**它不是一个可以永远做出正确决定的地方**，因为那个正确性取决于一个它并不控制的系统。它能是的是一个过滤器、一份成本、以及一个传感器。

### 那五条缝，以及本系列里每一个绕过都属于其中之一

这是把全篇统一起来的一节。前面几篇里每一族绕过，都能归到五个问题之一，而这五个问题对任何防御都很好问。

#### 缝一 —— 解码次数

> **过滤器解码几次，而应用解码几次？**

数字不同就有绕过，而所需的编码层数恰好是那个差。

| 过滤器解码 | 应用解码 | 生效的输入 |
|---|---|---|
| 1 | 2 | 双重编码：`%252e%252e%252f` 在过滤器那里是 `%2e%2e%2f`，在应用那里是 `../` |
| 1 | 1 | 这条缝上没有 —— 看别的 |
| 0 | 1 | 过滤器根本不看的一次编码 |

这就是路径穿越、LFI、XSS、XXE 那几篇里量过的那条缝，也是任何防御身上**第一个**该确定的事，因为它是一个有数字答案的问题。

#### 缝二 —— 表示

> **同一个东西有多少种写法，而过滤器有没有枚举它们？**

| 领域 | 同一个含义的多种表示 |
|---|---|
| IPv4 地址 | `127.0.0.1`、`127.1`、`0177.0.0.1`、`0x7f000001`、`2130706433`、`0` |
| SQL 语法 | 关键字大小写、注释当空白、`+` 与空格、等价运算符 |
| HTML 执行 | 是很多元素、事件与协议，而不是一个 |
| 文件扩展名 | 大小写、多扩展名、备用处理器 |
| 字符编码 | 实体、Unicode 转义、过长形式、其他字符集 |

这里的黑名单是**结构性**失败，而不是清单不全：**它在枚举"表示"，而含义是唯一的。** 再加多少条目都改变不了这个比例。

#### 缝三 —— 结构与字符串

> **过滤器推理的是一个字符串，而应用推理的是一个结构？**

| 过滤器看到 | 应用看到 |
|---|---|
| 一个含它认识的主机名的 URL | 一个**主机**在别处的 URL（`用户@主机`） |
| 一个名字无辜的归档成员 | 一条会向上解析的**路径** |
| 一个带 `filename` 的 multipart 分片 | 一个它更偏好的扩展 `filename*` |
| 一个带两个参数的请求 | 其中一个 —— 按过滤器不知道的某种规则 |

子串检查是这条缝的原型：**"URL 里含这个字符串"与"主机是它"是两个问题**，而只有一个是真正要紧的问题。

#### 缝四 —— 时机

> **被检查的那个东西，还是正在被使用的那个东西吗？**

| 攻击 | 中间变了什么 |
|---|---|
| DNS 重绑定 | 那个名字解析到了另一个地址 |
| 上传的先检查后使用 | 文件在判决之前就可达 |
| 反序列化钩子 | 代码在读取**过程中**执行，不是之后 |
| 任何资源上的 TOCTOU | 资源被换掉了 |

修法从来不是"检查得更好"，而是**顺序** —— 解析一次并使用那个解析值、在目的地可达之前完成校验、把"读取"当成危险的那一刻。

#### 缝五 —— 通道

> **这个方向，过滤器看得到吗？**

| 通道 | WAF 为什么看不到 |
|---|---|
| URL 片段 | 浏览器不发送它 |
| WebSocket 帧 | 不在它检查的 HTTP 请求路径里 |
| 客户端来源 | `localStorage`、`postMessage`、`document.referrer` |
| **出站流量** | **载荷不在请求里；它在服务器发出去的东西里** |
| 加密或隧道内容 | 在被检查的那一层不可见 |

出站那一行值得强调，因为它把模型倒过来了：在 SSRF 和盲 XXE 里，**请求是良性的，损害从服务器往另一个方向离开。** 一个入口过滤器对此毫无视野 —— 这就是为什么 SSRF 那篇把出站监控放在第一位。

### WAF 自身的失败模式

除了"按设计可被绕过"，过滤器还会以一些纯粹是实现选择的方式失败。最要紧的两个：

**fail-open（出错就放行）。** 当过滤器无法判断 —— 超时、请求体超过它的检查上限、解析错误 —— 很多部署会**放行这个请求**。那是一个在可用性上说得通、在安全上很差的选择，而要点在于它**是一个选择**，常常是一个没人记得自己做过的选择。

**覆盖不全。** 检查了请求体却没检查请求头、cookie 或路径；或者只检查到某个大小上限；或者只检查代理链里的一层。每一处缺口都是"过滤器的视图"与"应用的视图"不一致的地方。

在一个刻意做得很简单、带体积上限与超时的过滤器上实测：

| 请求 | 过滤器的行为 | 应用的行为 |
|---|---|---|
| 载荷超过检查上限 | **未经检查直接放行** | 应用自己的检查仍然生效 |
| 载荷在上限之内 | 被规则拦下 | 应用自己的检查仍然生效 |

第一行就是这个失败模式的一句话版本：**过滤器不检查了，只剩下应用自己的防御。**

### 差分测试，以及它为什么是正确的方法

评估任何过滤器的方法都不是一份 payload 清单，而是一次比较：

> **把同一个输入发两次 —— 一次经过过滤器、一次直接打应用 —— 然后比较结果。差异就是过滤器贡献的东西；相同就是它没贡献的东西。**

对一对专门搭出来的东西实测：一个会解码两次、并施加自己检查的应用，前面是一个只解码一次、匹配黑名单的过滤器。

| 输入 | 直接打应用 | 经过过滤器 | 怎么读 |
|---|---|---|---|
| `../etc/passwd` | 应用拒绝 | **403 —— 过滤器拦下** | 过滤器在工作 |
| `%2e%2e%2fetc%2fpasswd` | 应用拒绝 | **403 —— 过滤器拦下** | 过滤器在工作 |
| `%252e%252e%252fetc%252fpasswd` | 应用拒绝 | **应用拒绝**（过滤器放行了） | **过滤器被绕过，应用守住了** |
| `%25252e%25252e%25252fetc` | 应用接受（两次解码后仍是编码态） | 同上 | 两层都没牵涉 |
| `<script>alert(1)</script>` | 应用拒绝 | **403 —— 过滤器拦下** | 过滤器在工作 |
| `%253Cscript%253E` | **应用接受** | **过滤器放行** | **两层都漏了** |

从一张表里读出三件事，而它们正是一次评估该产出的三样东西：

**过滤器确实贡献了一些东西。** 有两行显示它拦下了应用同样会拦的东西 —— 那是成本而不是价值，不过它更早地停下了请求，而告警正是从那里来的。

**过滤器可以被编码绕过。** 双重编码那一行就是解码次数那条缝，也正是读源码、路径穿越那几篇对各自的过滤器量到的东西。

**真正的发现是两层都漏的那一行。** 双重编码的脚本标签同时通过了过滤器和应用 —— 那是唯一代表"可利用缺口"的一行。而注意：它是靠**比较结果**找到的，不是靠试一份更长的 payload 清单。

最后这点就是这个方法在实用上的论据：**一份 payload 清单测的是"过滤器认识什么"，而一次差分测试测的是"它做什么"。**

### 评估一个过滤器，按顺序

| 问题 | 怎么回答 |
|---|---|
| **它在哪一层？** | 反向代理、云边缘，还是运行时之内 —— 这决定其余一切 |
| **它解码几次？** | 把同一个 payload 用一、二、三次编码各发一遍 |
| **它检查哪些位置？** | 把同一个 payload 分别放进路径、查询、请求体、请求头和 cookie |
| **它的体积上限是多少？** | 逐步增大请求体，直到行为变化 |
| **它拿不准时做什么？** | 超大或畸形的请求体，以及能让它超时的请求 |
| **它看得到出站流量吗？** | 它存在与否，对 SSRF 与出站外泄什么也没说明 |
| **它记什么日志，而那日志有人看吗？** | 一份没人看的日志，和没有过滤器是一样的 |

这些问题里没有一个是在问 payload，而每一个都能在一个下午里答完。

### WAF 真正的用处

可被绕过并不让它没用；它让它的价值变得具体。

| 它确实帮得上 | 它帮不上 |
|---|---|
| 通用扫描器与批量利用尝试 | 一个读过本系列的人发起的针对性攻击 |
| 已知漏洞，在补丁之前的那段窗口 | 新型漏洞或逻辑漏洞 |
| 为机会主义攻击者抬高成本与噪声 | 任何走**出站**通道的东西 |
| 产生"谁在探"的信号 | 作为易受攻击的应用前面**唯一**的控制 |

诚实、也是这一篇所主张的说法是：

> **把 WAF 当传感器，而不是当墙。** 它的拦截按构造可以被绕过；它的**日志**记录了有人试过、用的什么形状、多频繁 —— 而那是技术栈里别的东西产生不出来的信息。

这改变了"把它配好"的含义：

- **覆盖每一个输入位置**，因为一个只检查请求体的过滤器，就是一个在教攻击者该把载荷放哪里的过滤器。
- **优先 fail-closed，或者让 fail-open 可见。** 如果为了可用性选了后者，就给它加上观测：一个绕过了检查的请求，应当被如此记录。
- **打开日志，并且读它。** 对规则分布、对新规则开始命中、以及对"过滤器失效后被放行的请求"告警，就是这次部署的回报。
- **把应用的日志与 WAF 的一起喂进来。** 这一对里信息量最大的事件，是一个**WAF 拦下、而应用照样处理了**的请求 —— 那是两个组件之间的分歧，而本系列从第一篇起讲的就是分歧。

### 检测与缓解

- **把 WAF 告警当作关于"意图"的信号，把应用日志当作关于"结果"的信号。** 一条规则命中告诉你这次尝试的形状；应用自己的日志告诉你有没有东西到达它。把两者关联起来，才是区分"一次被拦下的扫描"与"一次成功的绕过"的关键。
- **对分歧告警。** 一个在边缘被记为已拦截、在应用侧被记为成功的请求，是这一对里价值最高的单一事件，因为它意味着两个组件对同一串字节意见不一致 —— 那就是本系列里每一个绕过的定义。
- **对 fail-open 事件单独告警。** 超时、超大请求体和检查出错，都应当产生一行可与正常放行区分开的日志，这样那段窗口才是可度量的，而不是看不见的。
- **不要把规则覆盖度当保护力度。** 规则数量、或被拦请求数，度量的是活动量。可度量的那个问题是差分式的：同一个输入在过滤器在场时，行为是否不同。
- **把真正的控制放在应用能看到"值"的地方**：参数化查询、上下文感知的输出编码、把上传存到不会执行的地方、拒绝 DOCTYPE、用标识符代替路径、解析并钉住地址。上面每一个之所以有效，是因为它作用在**懂那份数据的那一层** —— 而那是 WAF 由位置决定、不可能具备的性质。
- **出站方向交给别的机制覆盖。** 出网过滤与出站日志抓的是那些"载荷从不进入请求"的类别，而任何入口过滤器都做不到。
- **并且度量那个库，而不是那个标签。** "我们有 WAF"与"我们的 WAF 配置成检查每一个位置、fail-closed、并对分歧记日志"，是两句不同的话，而只有第二句是一项控制。一次差分测试花一个下午，就能回答它。
