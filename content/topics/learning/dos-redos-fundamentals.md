---
id: dos-redos-fundamentals
title_en: "DoS and ReDoS — When Time Is Not a Function of Size"
title_zh: "DoS 与 ReDoS：当耗时不是大小的函数"
summary_en: A 27-character input holds a thread for three seconds, because a backtracking regex makes the work exponential in the input length. Measured across patterns and across engines — and in Go, where RE2 means the class does not arise.
summary_zh: 一段 27 个字符的输入能占住一个线程三秒，因为一条回溯型正则让工作量在输入长度上呈指数。这一篇跨模式和跨引擎实测 —— 也包括 Go，在那里 RE2 意味着这一类不会出现。
tags: [web, dos, redos, cwe-1333, regex, availability]
tools: [python3, go, curl, redos-checker]
attck: [T1499, T1499.004]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The property that makes this class different

Every denial-of-service finding in this guide shares one shape, and the lab states it in a single line:

> **The input is 27 characters, and the validation takes three seconds. It has nothing to do with data volume.**

That is what separates this class from ordinary resource exhaustion. A request body limit, a payload size cap, a `max_length` on a field — all of them assume that **work is a function of size**, and all of them are irrelevant here, because a backtracking regular expression makes the work exponential in the input length while the input stays tiny.

Four layers, and the second one is unusually literal for this series:

1. **The trust boundary.** The application trusts that a bounded input means bounded work. Nothing in the request format guarantees that, and one line of validation can break it.
2. **Data and instruction share a plane.** The regular expression is **the application's own code** — a validation rule the developer wrote — and the input determines **which execution path through it is taken**. The input is not data being examined; it is steering control flow through a program.
3. **Why the usual fix fails.** Length limits, body size caps and per-request byte limits do nothing, which is why the lab's own fix includes a length cap **as a mitigation rather than as the solution**: with exponential growth, capping the length does bound the worst case, but a cap is a number chosen against an attacker who gets to pick the shape.
4. **The variants.** Quantifier inside a quantifier, overlapping alternation, and a nullable branch inside a repetition — three spellings of the same ambiguity.

### The mechanism, measured

The engine used by most languages is a **backtracking** matcher: it tries one way of matching, and if that fails it tries another, and the number of ways grows with the input. The measured pattern is the lab's own shape reduced to its essentials:

| Input length | `^(a+)+$` | `^(a\|a)+$` | `^a+$` | `^[a-z]+$` |
|---|---|---|---|---|
| 14 | 0.36 ms | 0.73 ms | 0.00 ms | 0.00 ms |
| 16 | 1.56 ms | 3.01 ms | 0.00 ms | 0.00 ms |
| 18 | 6.11 ms | 11.63 ms | 0.00 ms | 0.00 ms |
| 20 | 23.99 ms | 49.37 ms | 0.00 ms | 0.00 ms |
| 22 | 96.06 ms | 195.24 ms | 0.00 ms | 0.00 ms |

Measured one character at a time, the ratio is the signature to look for:

```
length 18 ->  6.14 ms     x2.08
length 19 -> 11.89 ms     x2.01
length 20 -> 24.35 ms     x1.99
length 21 -> 49.10 ms     x2.04
length 22 -> 96.38 ms     x1.97
```

**Every extra character doubles the work**, so the lab's two-character steps multiply by four. And the single-quantifier patterns in the same table stay at zero milliseconds at every length — **the difference is ambiguity, not pattern complexity**.

**And the trigger has to fail.** A regex that *succeeds* returns as soon as the first alternative works, so it is fast; the exponential cost appears only when the engine has to prove that no match exists. That is why the payload is a run of matching characters followed by **one character the pattern does not accept** — and it is also why this is not found by sending normal-looking input.

### The language difference is structural

This is the part worth checking per stack, because for one of the languages covered by this guide the class does not exist.

| Language | Engine | Backtracking | Result |
|---|---|---|---|
| **Go** | **RE2** | **no** | Linear time — measured below |
| Python | `re` | yes | Provided by the regex, and there is no timeout parameter |
| Java | `java.util.regex` | yes | Same shape |
| JavaScript | V8 `RegExp` | yes | Same shape |
| PHP | PCRE | yes | Has a backtrack limit — a mitigation, not a fix |
| .NET | backtracking | yes | Has a timeout parameter |
| Ruby | Onigmo | yes | Same shape |

Measured on the same pattern that costs a Python process 96 ms at length 22 — `^(a+)+$`, which is not supposed to be usable in any regex engine:

| Input length | Go `regexp` |
|---|---|
| 14 | 0.020 ms |
| 1,000 | 0.025 ms |
| 10,000 | 0.264 ms |
| 100,000 | **3.795 ms** |

**Four orders of magnitude of growth, still measured in microseconds and single-digit milliseconds**, because Go's `regexp` package is built on RE2 and does not backtrack. There is no ambiguity for it to explore: it simulates the automaton, and that construction is linear in the input by definition.

**So a Go application does not have ReDoS, and a Python application with the same validation does.** That is a property of the runtime rather than of the code, and it is a real advantage worth knowing — with the accompanying cost, which the lab's own write-up notes: **RE2 does not support backreferences or lookaround**, so not every pattern can be moved to it. A pattern that needs those features has to be rewritten instead.

For the languages that do backtrack, the strongest available control short of rewriting is a **timeout around the match** — and in Python that has to be built outside the regex engine, because `re` has no timeout parameter. That is a fallback, and the entries below put it last on purpose.

### Rewriting, which is the fix

The measured rewrite, and the rule behind it:

| Dangerous | Safe |
|---|---|
| `^(\w+\s?)*$` | `^\w+(?:\s\w+)*$` |
| `^(a+)+$` | `^a+$` |
| `^(a\|a)+$` | `^a+$` |
| `^(\w+\s?)+$` | `^\w+(?:\s\w+)*$` |

**The rule is not to let "matching one thing" have several equivalent ways to happen.** In the lab's pattern the ambiguity comes from two parts: `\s?` **can match nothing**, and the group containing it is repeated — so a run of letters can be divided among the repetitions in exponentially many ways. Removing the nullable part and requiring an explicit separator between items collapses the count to one. Measured on the rewritten form at length 5,000, where the original would not finish:

| Pattern | Time at length 5,000 |
|---|---|
| `^(a)+$` | 8.578 ms |
| `^a+$` | 0.015 ms |

Both are finite, and both are slow-looking only because the measurement includes the engine's setup at an input size the original could never reach.

### Finding them

The shapes to look for are a short list, and they are all visible by reading the pattern:

- **A quantifier inside a quantifier** — `(a+)+`, `(a*)*`, `([a-z]+)*`
- **Overlapping alternation** — `(a|a)+`, `(a|ab)+`
- **A nullable branch inside a repetition** — `(\w+\s?)*`, `(\s*)*`

And the test for each is the same: **a run of matching characters followed by one character the pattern rejects**, with the input grown a little at a time while the elapsed time is watched. Linear looks flat; exponential doubles per character.

**That test is a code review activity rather than a scan**, and the lab makes the reason explicit: the trigger lives in the application's own source, in a validation rule somebody wrote and reviewed as an ordinary expression. Reviews ask whether input can be injected; fewer ask whether a pattern can explode. The most useful change in practice is to make that a step in the review — and where a project has many patterns, to run a static check over them in CI.

### Detection and mitigation

- **Alert on the tail of the request-duration distribution.** ReDoS does not look like a flood; it looks like a few ordinary requests, each of which took seconds. A per-endpoint p99 that moves while request volume does not is the signature.
- **Alert when CPU saturates without a corresponding rise in request rate.** That combination — high CPU, normal traffic — is what separates a computational attack from a volumetric one, and it is the observation that distinguishes this class from the rest of the DoS family.
- **And alert on duration outliers tied to a specific endpoint and input length.** If the slow requests to a validation endpoint cluster at 24 to 28 characters, the correlation is the diagnosis.
- **For mitigation, rewrite the pattern first.** Removing ambiguity is the only fix that addresses the cause, and the measured rewrites above are small.
- **Add a length cap, understanding what it buys.** Because the growth is exponential, a cap has an exponential effect: 28 characters bounds the worst case to seconds while 40 would be minutes. It is a genuine control and it is not a substitute for the rewrite.
- **Prefer a linear engine where the pattern allows it.** RE2 and the runtimes built on it have no backtracking to exploit; the cost is the loss of backreferences and lookaround, which is a real constraint and worth checking per pattern.
- **Where neither is possible, put a timeout around the match.** For Python this means running the match in a process or thread pool with a deadline, since the engine offers no timeout of its own. It is the last line rather than the first.
- **And run a static check over the project's patterns in CI.** The shapes are syntactically recognisable, which makes this one of the few vulnerability classes that a lint rule can meaningfully catch before deployment.
- **Widen the lens when reviewing for this.** ReDoS is one instance of a broader shape — **an attacker spending a little and costing the target a lot** — which also covers algorithmic complexity in parsing and sorting, amplification through a third party (the SSRF and cache entries both show a small request producing a large effect), and resource exhaustion through repeated allocation. The review question that covers all of them is not "is this input valid" but **"what is the most work one request can cause"**.

<!-- lang:zh -->
### 让这一类与众不同的那个性质

这份指南里每一类拒绝服务都有一个共同的形状，而靶场用一句话说了出来：

> **输入是 27 个字符，而这次校验要花三秒。它跟"数据量大"毫无关系。**

这就是把这一类与普通的资源耗尽分开的地方。请求体上限、载荷大小上限、字段的 `max_length` —— 它们全都假设**工作量是大小的函数**，而它们在这里全都不相关，因为一条回溯型正则会**让工作量在输入长度上呈指数**，而输入始终很小。

四层，而第二层在本系列里难得地字面：

1. **信任边界。** 应用信任"有界的输入意味着有界的工作量"。请求格式里没有任何东西保证这一点，而一行校验就能打破它。
2. **数据与指令共用同一平面。** 那条正则是**应用自己的代码** —— 一条开发者写的校验规则 —— 而输入决定了**它内部走哪条执行路径**。输入不是被检查的数据；它在**操纵一段程序的控制流**。
3. **为什么常见修法失败。** 长度上限、体积上限、每请求字节上限都无济于事 —— 这也是为什么靶场自己的修法把长度上限列为**缓解而不是解法**：在指数增长下，限制长度确实能约束最坏情况，但那个上限是一个**针对"由攻击者挑选形状"而选出的数字**。
4. **变体。** 量词套量词、重叠的交替、以及重复里可空的分支 —— 同一个歧义的三种写法。

### 机制，实测

多数语言用的引擎是一个**回溯型**匹配器：它用一种方式去匹配，失败就换一种，而方式的数目随输入增长。实测的模式是靶场那条形状削到最简：

| 输入长度 | `^(a+)+$` | `^(a\|a)+$` | `^a+$` | `^[a-z]+$` |
|---|---|---|---|---|
| 14 | 0.36 ms | 0.73 ms | 0.00 ms | 0.00 ms |
| 16 | 1.56 ms | 3.01 ms | 0.00 ms | 0.00 ms |
| 18 | 6.11 ms | 11.63 ms | 0.00 ms | 0.00 ms |
| 20 | 23.99 ms | 49.37 ms | 0.00 ms | 0.00 ms |
| 22 | 96.06 ms | 195.24 ms | 0.00 ms | 0.00 ms |

一个字符一个字符地量，那个比值就是该去找的特征：

```
长度 18 ->  6.14 ms     x2.08
长度 19 -> 11.89 ms     x2.01
长度 20 -> 24.35 ms     x1.99
长度 21 -> 49.10 ms     x2.04
长度 22 -> 96.38 ms     x1.97
```

**每多一个字符，工作量翻倍**，所以靶场里两字符一步的做法是乘四。而同一张表里单量词的那些模式在任何长度上都停在零毫秒 —— **差别在歧义，不在模式的复杂度**。

**而且触发它必须失败。** 一条*成功*的正则在第一种切法走通时就返回了，所以它很快；指数代价只在引擎**必须证明"不存在匹配"**时出现。这就是为什么 payload 是一串能匹配的字符后面跟**一个模式不接受的字符** —— 也正因如此，它不是靠发正常输入能找到的。

### 语言差异是结构性的

这一块值得按技术栈逐个查，因为这份指南覆盖的语言里，有一种根本不存在这一类。

| 语言 | 引擎 | 回溯 | 结果 |
|---|---|---|---|
| **Go** | **RE2** | **不会** | 线性时间 —— 下面实测 |
| Python | `re` | 会 | 由那条正则决定，而且没有超时参数 |
| Java | `java.util.regex` | 会 | 同一个形状 |
| JavaScript | V8 `RegExp` | 会 | 同一个形状 |
| PHP | PCRE | 会 | 有回溯上限 —— 是缓解，不是修复 |
| .NET | 回溯 | 会 | 有超时参数 |
| Ruby | Onigmo | 会 | 同一个形状 |

用同一条"在任何正则引擎里都不该可用"的模式 `^(a+)+$`（它让一个 Python 进程在长度 22 时花了 96 毫秒）在 Go 上实测：

| 输入长度 | Go `regexp` |
|---|---|
| 14 | 0.020 ms |
| 1,000 | 0.025 ms |
| 10,000 | 0.264 ms |
| 100,000 | **3.795 ms** |

**增长了四个数量级，耗时仍然在微秒到个位毫秒之间**，因为 Go 的 `regexp` 建立在 RE2 上、不回溯。它没有歧义可以去探索：它模拟自动机，而那个构造按定义是输入长度的线性函数。

**所以一个 Go 应用没有 ReDoS，而一个用同样校验的 Python 应用有。** 那是运行时的性质而不是代码的性质，是一个值得知道的真实优势 —— 附带代价，而靶场自己的说明也提到了：**RE2 不支持反向引用与环视**，所以不是每一条模式都能搬过去。需要那些特性的模式只能改写。

对确实会回溯的语言来说，在不改写的前提下最强的控制是**给这次匹配套一个超时** —— 而在 Python 里这必须做在正则引擎之外，因为 `re` 没有超时参数。那是兜底，所以下面把它排在最后。

### 改写，那才是修法

实测的改写，以及它背后的规律：

| 危险 | 安全 |
|---|---|
| `^(\w+\s?)*$` | `^\w+(?:\s\w+)*$` |
| `^(a+)+$` | `^a+$` |
| `^(a\|a)+$` | `^a+$` |
| `^(\w+\s?)+$` | `^\w+(?:\s\w+)*$` |

**规律是不让"匹配一个东西"有多种等价的实现方式。** 在靶场那条模式里，歧义来自两部分：`\s?` **可以匹配空**，而包含它的那个组被重复 —— 于是一串字母可以按指数多种方式分给那些重复。把可空的部分去掉、改成条目之间必须有显式分隔符，切法的数量就塌缩成一。在改写后的形式上于长度 5,000 处实测，而原式在那个长度上跑不完：

| 模式 | 长度 5,000 的耗时 |
|---|---|
| `^(a)+$` | 8.578 ms |
| `^a+$` | 0.015 ms |

两者都是有限的，而两者看着不算快，只是因为测量里包含了引擎在"原式永远到不了的输入规模"上的启动开销。

### 怎么找它们

要找的形状是一份短清单，而它们读一遍模式就能看出来：

- **量词套量词** —— `(a+)+`、`(a*)*`、`([a-z]+)*`
- **重叠的交替** —— `(a|a)+`、`(a|ab)+`
- **重复里可空的分支** —— `(\w+\s?)*`、`(\s*)*`

而对每一条的测试都一样：**一串能匹配的字符，后面跟一个模式拒绝的字符**，一边把输入一点点加长、一边盯着耗时。线性的看着是平的；指数的每个字符翻一倍。

**那个测试是一项代码评审活动，而不是一次扫描**，而靶场把原因说得很明确：触发点在应用自己的源码里，在一条有人写过、并按一条普通表达式评审过的校验规则里。评审会问输入能不能被注入；更少有人问一条模式会不会爆炸。实践中最有用的改变，是把这件事变成评审里的一个步骤 —— 而当一个项目有很多条模式时，在 CI 里对它们跑一次静态检查。

### 检测与缓解

- **对请求耗时分布的长尾告警。** ReDoS 不像洪水；它像少数几个普通请求，而每一个都花了好几秒。某个端点的 p99 在移动、而请求量没有上升，那就是特征。
- **当 CPU 打满、而请求速率没有相应上升时告警。** 那个组合 —— 高 CPU、正常流量 —— 正是把"计算型攻击"与"流量型攻击"分开的东西，也是把这一类与 DoS 家族其余部分区分开的观察。
- **并且对"与某个端点和某个输入长度绑定的耗时离群值"告警。** 如果一个校验端点上的慢请求都聚集在 24 到 28 个字符，那个相关性就是诊断。
- **缓解上，先改写那条模式。** 消除歧义是唯一针对成因的修法，而上面那些实测的改写都很小。
- **加一个长度上限，并明白它买到了什么。** 因为增长是指数的，上限的效果也是指数的：28 个字符把最坏情况约束到几秒，而 40 个字符就是几分钟。它是一项真实的控制，而它不是改写的替代品。
- **在模式允许的地方优先用线性引擎。** RE2 以及建立在它之上的运行时没有被利用的回溯；代价是失去反向引用与环视，那是一个真实的约束，值得逐条模式去查。
- **两者都做不到的地方，给这次匹配套一个超时。** 对 Python 来说，这意味着把这次匹配放到一个带期限的进程或线程池里跑，因为引擎本身不提供超时。它是最后一道，而不是第一道。
- **并且在 CI 里对项目里的模式跑一次静态检查。** 那些形状在语法上可辨认，这让它成为少数几种能在部署前被一条 lint 规则有效拦住的漏洞类别。
- **评审这一类时把镜头放宽一点。** ReDoS 是更大形状的一个实例 —— **攻击者花一点点、让你付很多** —— 那个形状也覆盖解析与排序里的算法复杂度、经第三方放大（SSRF 与缓存那两篇都展示了小请求产生大效果）、以及通过重复分配达成的资源耗尽。覆盖它们全部的那个评审问题不是"这个输入合法吗"，而是 **"一个请求最多能造成多少工作"**。
