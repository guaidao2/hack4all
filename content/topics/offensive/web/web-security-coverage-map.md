---
id: web-security-coverage-map
title_en: Web Security Coverage Map
title_zh: Web 安全覆盖地图
summary_en: Every other entry is one technique; this one is the index for the web side of the guide — what is already covered, what is missing, and which gaps to close first. It doubles as a map of the defensive surface, because a class of bug you have not written about is usually a class you are not watching for.
summary_zh: 其他每一篇都是单个技术点，这一篇是 Web 这一侧的索引 —— 已经覆盖了什么、还缺什么、以及哪几个缺口该先补。它同时也是一张防守面地图，因为你没写过的漏洞类，通常也是你没在盯的那一类。
tags: [web, methodology, coverage, index, planning, checklist]
attck: [T1190, T1059, T1059.007, T1083, T1105, T1550.001, T1552.005, T1505.003]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why this page exists

The red team tactical map is the index for an engagement: the stages, in order, with the questions that decide whether a phase was covered. This is the same idea for **one phase** — the web application itself — and it exists for two reasons.

**For the reader choosing what to study**: the guide has grown past the point where "read all of it" is useful advice. A map that says *these six classes have a full treatment, these have a one-page overview, and these have nothing* is more useful than a list.

**For the reader defending**: a class of bug that has no entry here is usually a class that is not in anyone's detection rules either. The gaps below are a to-do list for the defender as much as for the writer.

### How to use it

- **Planning study or a test**: read the coverage table, then the gap table. The gaps are ordered, and the reason for the order is in the table rather than in a preamble.
- **Writing a report or a detection backlog**: read the gap table from the bottom of the second tier upward. The entries lower down are the ones most often missing from a test plan.
- **Deciding what to write next**: the third table maps this guide against a purpose-built vulnerable lab, which is a useful second opinion — if a lab bothered to build a challenge for a class, the class is worth a page.

### What is covered, and how deeply

Three layers exist today. The distinction matters: a **full treatment** means several entries that go from mechanism to exploitation to detection; an **overview** is one page that names the class, its shape and its defences; a **mention** is a paragraph inside another entry.

| Area | Depth | Entries |
|---|---|---|
| **SQL injection** | full | five entries: fundamentals, UNION and error, blind, out-of-band and second-order, dialects and filters |
| **NoSQL injection** | full | two: operators and MongoDB, then the other databases and the defence |
| **XSS** | full | three: the browser as interpreter, contexts and exploitation, filters and detection |
| **File upload** | full | three: who allows and who runs, names and archives, bypasses and interpretation |
| **Deserialisation** | full | three: restoration is construction, chains and gadgets, defence and detection |
| **XXE** | full | two: XML carries instructions, blind reads and encoding |
| **LFI / RFI** | full | three: names and paths, reading source code, reading to running |
| **SSRF** | full | two: addresses are not strings, targets and blind probes |
| **JWT** | full | two: the token declares how to verify it, key sources and claims |
| **WAF bypass** | full | two: the cross-class methodology, and an overview entry |
| **Language features and parsing differences** | full | five: the general entry plus PHP, ASP.NET and IIS, Java and JSP, and the four-runtime entry |
| **Encoding and classical ciphers** | full | nine, in the beginner-adjacent track |
| **Command injection** | overview | one page naming the shape and the shell's role |
| **SSTI** | overview | one page: a template engine evaluating what it should not |
| **CSRF, CORS, clickjacking, postMessage** | overview | inside the frontend security page |
| **IDOR / broken object authorisation** | overview | one page |
| **Authentication flaws** | overview | one page, JWT now split out into its own treatment |
| **Cryptographic failures** | overview | one page |
| **Business logic, insecure design** | overview | one page each |
| **Security misconfiguration, outdated components** | overview | one page each |
| **Middleware and database exposure** | overview | one page |
| **HTTP and web fundamentals** | overview | one page, including request parsing |
| **Logging and monitoring failures** | overview | one page |
| **Supply chain, integrity failures** | overview | one page each |

Read as a whole: **the classes with a distinct mechanism have a full treatment, and the classes that are "a mistake in the application's own logic" have an overview.** That split is deliberate — a mechanism can be explained once and reused, while a logic flaw is different in every application — but it leaves a specific set of gaps.

### The gaps, ordered

#### Tier one: a lab built a challenge for it, and there is no full treatment

The signal here is strong. A purpose-built vulnerable application includes a challenge when the class has a **distinct mechanism worth teaching** and is **common enough to matter**. Five of those have no entry.

| Gap | Why it is first |
|---|---|
| **Business logic flaws** | Four challenges in the lab, and no entry that treats them as a class. They are the one category where every instance is different, which is exactly why the *method* needs writing down: find the invariant the code assumes and break the assumption rather than the input |
| **Race conditions** | One challenge, and the mechanism is genuinely distinct: the bug exists in a window, not in a value. It also contradicts the mental model most readers have, that a request is atomic |
| **SSTI as a full treatment** | The overview names it; the mechanism deserves the same treatment as LFI, because it is code execution through a template engine and it has its own family of sandbox escapes |
| **Command injection as a full treatment** | Same reasoning. The shell is a second interpreter with its own grammar, and the bypasses are specific — spaces, separators, encoding, blind extraction |
| **Host header injection** | One challenge, and it is under-taught relative to its consequence: password reset links, cache keys, routing and virtual hosts all trust a header the client writes |

#### Tier two: common in real applications, no entry at all

| Gap | Why it matters |
|---|---|
| **HTTP request smuggling** | The cleanest instance of this guide's recurring theme: two parsers disagreeing about where a request ends. It belongs in the guide for the same reason WAF bypass does |
| **Web cache poisoning and deception** | A caching layer is a third parser, and it decides what other users receive. High consequence, low awareness |
| **OAuth and OIDC flaws** | The overview covers authentication generally; the authorisation-code flow, redirect URI handling, token audience and state/nonce have their own failure modes that deserve their own page |
| **Session management** | Session fixation, rotation, cookie scope, and the signed-cookie pattern the lab has a challenge for. Currently spread across two overviews |
| **CSRF and CORS as full treatments** | Both live inside the frontend overview. CORS misconfiguration in particular is a one-line mistake with a whole-page consequence |
| **Open redirect and information disclosure** | Two lab challenges, and both are usually treated as low severity when they are the first step of a chain — redirect for token theft, backups and leftover files for credentials |
| **Denial of service, including ReDoS** | One lab challenge. Worth a page because the trigger is a regex in the application's own code, which makes it a code-review finding rather than a scan finding |
| **API-specific classes** | Mass assignment, excessive data exposure, rate limiting, and the difference between BOLA and BFLA. The web overviews cover the ideas in passing |
| **Prototype pollution as its own page** | Currently a section in the language-features entry. It has its own exploitation paths and its own detection |
| **Clickjacking and client-side attack surface** | Named in the frontend overview, no mechanism, no defences |
| **Subdomain takeover** | A dangling DNS record, a build pipeline and a certificate. Simple to detect, frequently missed |

#### Tier three: real, but further from the centre

| Gap | Note |
|---|---|
| **CRLF injection and response splitting** | Mostly historical, still appears in redirects and logging |
| **WebSocket security** | Its own trust boundary: origin checks, authentication after upgrade, message framing |
| **Client-side storage and postMessage** | Cross-window messaging with a checkable origin, and storage that is readable by any script in the origin |
| **Cryptographic misuse in depth** | Padding oracle, ECB patterns, IV reuse. The overview names the category |
| **GraphQL** | Introspection, batching, depth limits, authorisation per field |
| **Supply chain and CI/CD** | Partly covered; the build pipeline as an attack surface is not |
| **Cloud and container** | Three cloud pages exist, mostly around metadata and IAM; the container escape and orchestration layers are separate subjects |
| **Mobile and API clients** | A different discipline more than a gap in this one |

### The lab as a second opinion

A purpose-built vulnerable application is a useful check on this list, because someone already decided which classes deserved a challenge. Mapping the two:

| Class | Lab challenges | Guide |
|---|---|---|
| SQL injection | 11 | full |
| XSS | 5 | full |
| Business logic | 4 | **overview only** |
| File upload | 3 | full |
| Command injection | 2 | **overview only** |
| CSRF | 2 | overview |
| IDOR | 2 | overview |
| JWT | 2 | full |
| Path traversal | 2 | full |
| SSRF | 2 | full |
| SSTI | 2 | **overview only** |
| CORS | 1 | overview |
| Deserialisation | 1 | full |
| DoS (ReDoS) | 1 | **none** |
| Session forgery | 1 | **none** |
| Host header | 1 | **none** |
| Information disclosure | 1 | **none** |
| Open redirect | 1 | **none** |
| Race condition | 1 | **none** |
| XXE | 1 | full |

The bold rows in the "guide" column are the ones where the lab knows something the guide has not written down yet.

### If you only have a week

Write these five, in this order, and the map stops having holes in the tier that matters:

1. **Business logic flaws** — the class with the most challenges and the least coverage, and the one whose method is most reusable.
2. **HTTP request smuggling** — the purest example of the disagreement theme, and a class that gets missed because a proxy hides it.
3. **Race conditions** — short, distinct, and it corrects a wrong mental model.
4. **SSTI and command injection as full treatments** — both are already named in an overview, so the marginal work is smaller than it looks.
5. **Session management** — it ties together the JWT work, the CSRF overview and the signed-cookie challenge.

### Detection and mitigation, for the defender

The map is useful from the other side too, because **coverage gaps in a guide tend to be coverage gaps in a detection programme**, and for the same reason: both follow what somebody thought to write down.

- **Use the gap list as a detection backlog review.** Take each tier-two entry and ask whether the telemetry exists to see it at all. Request smuggling needs the proxy and the origin log side by side; cache poisoning needs the cache's notion of a key; race conditions need a way to see two requests in the same window. If the data is not collected, the class is invisible regardless of alerting.
- **Start with the classes whose evidence is already in the logs.** Business logic and authorisation flaws leave a trail of ordinary requests that were each legitimate and only wrong in sequence — which means the detection is behavioural, not signature-based, and the data is already there.
- **Treat the classes with no entry as the ones with no rules.** A class nobody wrote about usually has no rule, no dashboard and no runbook; the gap table doubles as the list of those.
- **For mitigation, weight by what the class costs when it lands.** Request smuggling and cache poisoning affect **other users** and can persist beyond the request; SSRF and metadata reach credentials; deserialisation and SSTI are code execution. Those four are worth closing before the ones that need an application-specific mistake to matter.
- **And keep the split in mind when choosing a control.** A mechanism-level class can be mitigated centrally — a parser setting, a decoder, a network policy. A logic class cannot; it needs invariants written down and tested. That is why the second kind stays an overview here: there is no single control to name.

<!-- lang:zh -->
### 这一篇为什么存在

红队战术地图是一次项目的索引：把阶段按顺序摊开，配上"这个阶段到底做没做"的问题。这一篇是同一个想法，但只针对**一个阶段** —— Web 应用本身 —— 而它存在的理由有两个。

**给正在挑学什么的读者**："全都读一遍"早就不是有用建议了。一张说清"这六类有完整讲解、这几类只有一页概览、这些还什么都没有"的图，比一份清单有用。

**给防守方的读者**：一个在这里没有篇目的漏洞类，通常也不在任何人的检测规则里。下面那些缺口，对防守方和作者而言同样是一份待办。

### 怎么用它

- **规划学习或测试**：先看覆盖表，再看缺口表。缺口是按优先级排的，理由就在表里，不写在序言里。
- **写报告或排检测待办**：从第二档的底部往上读。越往下越容易在测试计划里漏掉。
- **决定下一篇写什么**：第三张表把这份指南与一个专门做出来的漏洞靶场对照了一遍，那是一份有用的第二意见 —— 如果一个靶场专门为某一类做了一道题，那一类就值得一页。

### 已经覆盖了什么，有多深

现在有三层。这个区分是要紧的：**完整讲解**指若干篇从机理到利用再到检测；**概览**指一页命名这一类、说明它的形状与防御；**提及**指别的篇目里的一段。

| 领域 | 深度 | 篇目 |
|---|---|---|
| **SQL 注入** | 完整 | 五篇：本质、UNION 与报错、盲注、带外与二次注入、方言与过滤器 |
| **NoSQL 注入** | 完整 | 两篇：运算符与 MongoDB，然后其他数据库与防御 |
| **XSS** | 完整 | 三篇：把浏览器当解释器、上下文与利用、过滤器与检测 |
| **文件上传** | 完整 | 三篇：谁允许谁执行、名字与归档、绕过与解释权 |
| **反序列化** | 完整 | 三篇：还原即构造、链与 gadget、防御与检测 |
| **XXE** | 完整 | 两篇：XML 自带指令、盲读与编码 |
| **LFI / RFI** | 完整 | 三篇：名字与路径、读源码、从读到执行 |
| **SSRF** | 完整 | 两篇：地址不是字符串、目标与盲探 |
| **JWT** | 完整 | 两篇：token 自己声明怎么验它、密钥来源与声明 |
| **WAF 绕过** | 完整 | 两篇：跨漏洞的方法论，加一篇速查 |
| **语言特性与解析差异** | 完整 | 五篇：总论加 PHP、ASP.NET 与 IIS、Java 与 JSP，以及四种运行时那篇 |
| **编码与古典密码** | 完整 | 九篇，在新手入门的相邻轨里 |
| **命令注入** | 概览 | 一页，说清形状与 shell 的角色 |
| **SSTI** | 概览 | 一页：模板引擎求值了它不该求值的东西 |
| **CSRF、CORS、点击劫持、postMessage** | 概览 | 在"前端安全"那一页里 |
| **IDOR / 对象级授权缺陷** | 概览 | 一页 |
| **认证缺陷** | 概览 | 一页；JWT 现在已拆出去独立成篇 |
| **密码学失效** | 概览 | 一页 |
| **业务逻辑、不安全设计** | 概览 | 各一页 |
| **配置错误、过时组件** | 概览 | 各一页 |
| **中间件与数据库暴露** | 概览 | 一页 |
| **HTTP 与 Web 基础** | 概览 | 一页，含请求解析 |
| **日志与监控失效** | 概览 | 一页 |
| **供应链、完整性失效** | 概览 | 各一页 |

整体读下来：**机理独特的那些类有完整讲解，而"错在应用自己的逻辑里"的那些类只有概览。** 这个划分是有意为之 —— 机理讲一次可以复用，而逻辑缺陷每个应用都不一样 —— 但它留下了一组具体的缺口。

### 缺口，按优先级

#### 第一档：靶场为它做了题，而这里没有完整讲解

这个信号很强。一个专门做出来的漏洞靶场，会为某一类加一道题，条件是这一类**有独特的机理值得教**、并且**足够常见**。其中有五个还没有篇目。

| 缺口 | 为什么排第一 |
|---|---|
| **业务逻辑缺陷** | 靶场里有四道题，而这里没有一篇把它当作一类来讲。它是唯一"每一例都不同"的类别，而这正是为什么**方法**需要写下来：去找代码假设的那个不变式，去破坏假设而不是破坏输入 |
| **竞态条件** | 一道题，而机理确实独特：bug 存在于一个**时间窗**里，而不是一个值里。它还纠正了多数读者心里的模型 —— 一次请求是原子的 |
| **SSTI 的完整讲解** | 概览命名了它；它的机理值得和 LFI 同等的待遇，因为它是通过模板引擎达成的代码执行，而且有自己的一整套沙箱逃逸 |
| **命令注入的完整讲解** | 同样的道理。shell 是第二个解释器、有自己的文法，而绕过是特定的 —— 空格、分隔符、编码、盲注提取 |
| **Host 头注入** | 一道题，而它相对其后果被教得太少：口令重置链接、缓存键、路由与虚拟主机，都在信任一个客户端写的头 |

#### 第二档：真实应用里常见，这里完全没有

| 缺口 | 为什么重要 |
|---|---|
| **HTTP 请求走私** | 本指南那条主线最干净的实例：两个解析器对"一个请求在哪里结束"意见不一致。它该进这份指南，理由和 WAF 绕过那篇一样 |
| **Web 缓存投毒与缓存欺骗** | 缓存层是第三个解析器，而它决定**别人**收到什么。后果重、认知低 |
| **OAuth 与 OIDC 缺陷** | 概览讲的是广义认证；授权码流程、重定向 URI 处理、令牌受众、state/nonce 各有自己的失效模式，值得自己一页 |
| **会话管理** | 会话固定、轮换、cookie 作用域，以及靶场里那道签名 cookie 题对应的模式。现在散在两篇概览里 |
| **CSRF 与 CORS 的完整讲解** | 两者都住在前端概览里。CORS 配置错误尤其是一行代码的失误、一整页的后果 |
| **开放重定向与信息泄漏** | 靶场两道题，而两者常被当作低危，实际上它们是链条的第一步 —— 重定向用于偷令牌，备份与残留文件用于偷凭据 |
| **拒绝服务，含 ReDoS** | 靶场一道题。值得一页，因为触发点是应用**自己代码里的正则**，这让它成为代码评审发现而不是扫描发现 |
| **API 特有的类别** | 批量赋值、过度数据暴露、速率限制，以及 BOLA 与 BFLA 的区别。Web 概览只是顺带提到这些想法 |
| **原型污染独立成篇** | 现在只是语言特性那篇里的一节。它有自己的一套利用路径和检测 |
| **点击劫持与客户端攻击面** | 前端概览命名了，没有机理、没有防御 |
| **子域接管** | 一条悬空的 DNS 记录、一条构建流水线、一张证书。检测简单，却经常漏 |

#### 第三档：真实，但离中心更远

| 缺口 | 说明 |
|---|---|
| **CRLF 注入与响应拆分** | 大多是历史问题，仍会出现在重定向与日志里 |
| **WebSocket 安全** | 它有自己的信任边界：来源检查、升级后的认证、消息分帧 |
| **客户端存储与 postMessage** | 跨窗口消息（来源可校验），以及源内任何脚本都能读的存储 |
| **密码学误用深入** | padding oracle、ECB 模式、IV 重用。概览命名了这一类 |
| **GraphQL** | 自省、批量查询、深度限制、逐字段授权 |
| **供应链与 CI/CD** | 部分覆盖；构建流水线作为攻击面还没有 |
| **云与容器** | 已有三篇云相关，主要围绕元数据与 IAM；容器逃逸与编排层是另外的题目 |
| **移动端与 API 客户端** | 更像是另一门学科，而不是这里的缺口 |

### 拿靶场当第二意见

一个专门做出来的漏洞靶场，是对这份清单有用的校验，因为已经有人替你判断过哪几类值得出一道题。把两者对上：

| 类别 | 靶场题数 | 这份指南 |
|---|---|---|
| SQL 注入 | 11 | 完整 |
| XSS | 5 | 完整 |
| 业务逻辑 | 4 | **只有概览** |
| 文件上传 | 3 | 完整 |
| 命令注入 | 2 | **只有概览** |
| CSRF | 2 | 概览 |
| IDOR | 2 | 概览 |
| JWT | 2 | 完整 |
| 路径穿越 | 2 | 完整 |
| SSRF | 2 | 完整 |
| SSTI | 2 | **只有概览** |
| CORS | 1 | 概览 |
| 反序列化 | 1 | 完整 |
| 拒绝服务（ReDoS） | 1 | **没有** |
| 会话伪造 | 1 | **没有** |
| Host 头 | 1 | **没有** |
| 信息泄漏 | 1 | **没有** |
| 开放重定向 | 1 | **没有** |
| 竞态条件 | 1 | **没有** |
| XXE | 1 | 完整 |

"这份指南"那一列加粗的行，就是靶场已经知道、而这份指南还没写下来的地方。

### 如果你只有一周

按这个顺序写这五块，这张图在要紧的那一档上就没有洞了：

1. **业务逻辑缺陷** —— 题最多、覆盖最少的那一类，而且它的方法最可复用。
2. **HTTP 请求走私** —— "不一致"那条主线最纯粹的实例，也是因为被代理挡住而最常被漏掉的一类。
3. **竞态条件** —— 短、独特，而且它纠正一个错误的心智模型。
4. **SSTI 与命令注入的完整讲解** —— 两者都已在概览里命名，所以边际工作量比看起来小。
5. **会话管理** —— 它把 JWT 那两篇、CSRF 概览和那道签名 cookie 题串起来。

### 检测与缓解，给防守方

这张图从另一侧看也有用，因为**一份指南里的覆盖缺口，往往就是一套检测体系里的覆盖缺口**，理由相同：两者都跟着"有人想到要写下来"走。

- **把缺口表当作检测待办来复盘。** 拿第二档的每一条问一遍：要看得到它，遥测存在吗。请求走私需要代理日志与源站日志并排看；缓存投毒需要缓存对"键"的理解；竞态条件需要能看到同一时间窗里的两个请求。数据没采，这一类就是不可见的，跟告警规则无关。
- **从"证据已经在日志里"的那些类开始。** 业务逻辑与授权缺陷留下的是一串**每一笔都合法、只有连起来才错**的普通请求 —— 这意味着检测是行为式的而不是签名式的，而数据本来就在那里。
- **把"没有篇目"的那些类，当作"没有规则"的那些类。** 没人写过的漏洞类，通常也没有规则、没有看板、没有处置手册；缺口表同时就是这份名单。
- **缓解上，按"落地之后值多少"加权。** 请求走私与缓存投毒影响的是**其他用户**，而且可能比这次请求活得更久；SSRF 与元数据通向凭据；反序列化与 SSTI 是代码执行。这四类值得排在那些"需要应用自己犯错才要紧"的前面。
- **选控制措施时记住那个分层。** 机理层面的类可以集中缓解 —— 一个解析器设置、一个解码器、一条网络策略。逻辑类不行；它需要把不变式写下来并测起来。这就是为什么第二类在这里始终是概览：**没有单一控制可以点名。**
