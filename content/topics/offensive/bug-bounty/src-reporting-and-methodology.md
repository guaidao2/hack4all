---
id: src-reporting-and-methodology
title_en: Bug Bounty and SRC Reporting
title_zh: SRC 与众测报告方法
summary_en: Finding a bug is half the work, and a report a triager accepts is the other half. This entry is about scope discipline, proving impact, and the mistakes that turn a valid finding into a rejected one.
summary_zh: 找到一个漏洞是一半工作，写出一份审核会受理的报告是另一半。这一篇讲范围纪律、证明影响，以及那些把有效发现变成"已拒绝"的常见错误。
tags: [bug-bounty, src, reporting, methodology, scope]
tools: [Burp Suite, ffuf, shodan, nuclei]
attck: [T1595]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Three models, one discipline

| Model | Who runs it | What matters most |
|---|---|---|
| Corporate SRC | The company itself | The rules are internal; response times vary widely |
| Crowd-testing platform | A platform with a contract | Scope and rules are explicit and enforced |
| Public bug bounty | The company, with published terms | Payout tiers and duplicate handling |

All three have the same first requirement: **read the scope and the rules before sending a single request.** Everything in this entry assumes you have. Testing out of scope is not a lesser finding; it is the thing that ends your account, and in some jurisdictions it is a legal problem rather than a policy one.

### Reading the rules properly

The parts that decide what you may actually do:

- **In-scope assets**: domains, subdomains, mobile apps, APIs, IP ranges. A wildcard scope usually excludes specific hosts — read the exclusions.
- **Out-of-scope** is usually longer than in-scope: third-party services, marketing sites, employee accounts, anything that touches real user data.
- **Testing limits**: whether automated scanning is allowed, rate limits, prohibited techniques (denial of service, social engineering, physical), and whether production data may be accessed.
- **Prohibited actions**: no lateral movement, no persistence, no accessing other users' data beyond the minimum to demonstrate impact, and no disclosing anything about the target.
- **Reward rules**: what counts as a duplicate, how severity is judged, and whether "informational" findings are accepted at all.
- **Disclosure policy**: whether and when you may write about it.

A finding that violates the testing rules is worse than no finding: it can invalidate your other reports and, in the wrong company, escalate.

### The workflow, from recon to report

1. **Reconnaissance.** Subdomains, forgotten hosts, new features, exposed panels, source maps, API versions. The reconnaissance entry covers the mechanics. The goal here is to find what other testers have not.
2. **Choose where to spend time.** Newly deployed features, recently acquired companies, admin panels behind a login nobody audited, and API versions older than the current one are all under-tested compared to the main application.
3. **Test with intent.** One hypothesis at a time: what is this endpoint supposed to enforce, and what happens if I send something else?
4. **Verify before writing.** Reproduce three times from a clean state. If it only works once, understand why before claiming it.
5. **Minimise the proof.** The smallest request that demonstrates the issue, with test accounts you created, and the least data needed to show impact.
6. **Assess the real impact.** What can an attacker actually do, how much data, how many users, and can it be chained into something bigger?
7. **Write the report.** Assume the reader is a busy triager handling forty reports today.

### The report

A structure that gets accepted:

**Title.** One line, with the vulnerability and the impact: `Stored XSS in profile bio executes for any user viewing the page`, not `XSS bug found`.

**Severity and reasoning.** Use the platform's classification or CVSS, and state the reasoning. A CVSS vector without a sentence of justification reads as inflated; a clear sentence explaining the access required and the consequence reads as credible.

**Summary.** Two or three sentences: what the bug is, where it is, what it lets an attacker do.

**Steps to reproduce.** Numbered, exact, and copy-pasteable. Include the account state you started from. If it needs two accounts, say so and give both. If a header or a cookie matters, show it.

**Evidence.** The raw request and response, not only a screenshot of the browser. Timestamps. Where the payload landed. Screenshots help the reader; requests and responses are the proof.

**Impact.** Written for a non-technical reader as well: what an attacker gains, and what it costs the business. This is the section that decides the severity.

**Remediation.** A concrete suggestion — parameterise the query, encode the output, check the permission server-side. It signals that you understand the bug rather than just triggering it.

What to keep out: exaggerated claims, data belonging to real users, a dump of scanner output, and unrelated findings bundled into one report.

### Why reports get rejected

Most rejections are avoidable, and they fall into a small number of patterns:

- **Not reproducible.** The steps assume a state you did not describe, or the bug is intermittent and the report does not say so.
- **Impact not established.** "This could potentially lead to..." with no demonstrated consequence. If the effect is theoretical, say so and let the triager downgrade it, rather than implying more.
- **Below the programme's bar.** Self-XSS, clickjacking with no sensitive action, CORS without a demonstrated read, missing headers, version disclosure, and open redirects are commonly out of scope. Not because they are not real, but because they are not what the programme is paying for. Read the acceptance criteria before spending a day on one.
- **Duplicate.** Nothing to do about it except be faster and pick less-tested assets.
- **Out of scope**, including the case where a finding is reachable only from an out-of-scope host.
- **Real user data**, which is a rules violation regardless of the finding.
- **One report for ten issues.** Triagers rate each finding separately; a bundle gets split or closed. Send them separately, worst first.

### Communication

- **Report the bug with the minimum demonstration** and stop. Do not enumerate further than the impact requires, and do not exfiltrate data to prove a point.
- **One issue per report.**
- **Be patient and polite.** Response times range from hours to months; a triager is a person with a queue, and hostility costs you more than it gains.
- **Never threaten disclosure.** Follow the published policy, and if there is none, ask.
- **If you disagree with a severity**, respond once with evidence and reasoning, then accept the decision. Escalating repeatedly is how testers get banned.
- **Keep your own notes.** A record of what you tested and how protects you if a later question arises about a request you made.

### Detection and mitigation, from the programme's side

- **Write the scope precisely.** Ambiguity gets tested, and the cost of a clarifying sentence is far below the cost of an incident.
- **Publish a first-response time and meet it.** A programme that answers in 24 hours gets better reports than one that answers in a month, because testers triage their own effort accordingly.
- **Have a triage standard.** Whether self-XSS counts, what severity means for a given data class, and how duplicates are decided. Inconsistency drives good testers away.
- **Feed findings back into the backlog with regression tests.** A fixed finding without a test comes back.
- **Watch the pattern across reports.** Ten testers finding missing authorization in different endpoints is one systemic problem, not ten tickets.
- **Monitor for out-of-scope activity** so you can stop it early instead of discovering it in the logs afterwards.

<!-- lang:zh -->
### 三种模式，同一种纪律

| 模式 | 由谁运营 | 最要紧的是什么 |
|---|---|---|
| 企业 SRC | 企业自己 | 规则是内部的；响应时间差别极大 |
| 众测平台 | 平台、有合同 | 范围与规则明确，且会被执行 |
| 公开 bug bounty | 企业，条款公开 | 奖励档位与重复处理方式 |

三者的第一个要求完全相同：**在发出第一个请求之前，先读范围和规则。** 这一篇的一切都建立在"你已经读过了"之上。测范围之外的东西不是"次一等的发现"，而是会让你账号被封的事；在某些司法辖区，它还不是政策问题是法律问题。

### 把规则读对

真正决定"你能做什么"的部分：

- **范围内资产**：域名、子域、移动 App、API、IP 段。通配范围通常会排除特定主机 —— 排除项要读。
- **范围外**通常比范围内长得多：第三方服务、营销站点、员工账号，以及任何会碰到真实用户数据的东西。
- **测试限制**：是否允许自动化扫描、速率限制、禁止的手法（拒绝服务、社会工程、物理接触），以及是否可以访问生产数据。
- **禁止动作**：不横向移动、不持久化、不在证明影响之外访问他人数据、不对外披露目标的任何信息。
- **奖励规则**：什么算重复、严重级别怎么判、以及"信息级"发现是否受理。
- **披露政策**：什么时候、是否可以公开写出来。

违反测试规则的发现比没有发现更糟：它可能让你其他报告一起作废，遇到较真的公司还会升级成别的问题。

### 从侦察到报告的流程

1. **侦察。** 子域、被遗忘的主机、新上线的功能、暴露的面板、source map、API 版本。机制见侦察那篇。这里的目标是找到别人没测过的地方。
2. **决定把时间花在哪。** 新部署的功能、刚收购的公司、没人审计过的后台、以及比当前版本旧的 API —— 它们的测试覆盖都远不如主应用。
3. **带着假设去测。** 一次一个假设：这个接口本应强制什么？如果换成别的东西会发生什么？
4. **写之前先验证。** 从干净状态复现三次。如果只成功一次，先搞清楚为什么，再下结论。
5. **把证明最小化。** 用能说明问题的最小请求、你自己注册的测试账号、以及证明影响所需的最少数据。
6. **评估真实影响。** 攻击者实际能做到什么、涉及多少数据、多少用户、能不能串成更大的问题？
7. **写报告。** 假设读者是一位今天要处理四十份报告的审核员。

### 报告怎么写

一个容易被受理的结构：

**标题。** 一行说清漏洞与影响：`个人简介的存储型 XSS 会在任何用户查看该页面时执行`，而不是"发现 XSS"。

**严重级别与理由。** 用平台的分类或 CVSS，并给出理由。只有 CVSS 向量、没有一句解释，读起来像在抬价；一句说明"需要什么访问、后果是什么"的话，读起来才可信。

**摘要。** 两三句：是什么漏洞、在哪、能让攻击者做什么。

**复现步骤。** 编号、精确、可复制粘贴。写清你从什么账号状态开始。需要两个账号就说明并都给出来。某个请求头或 cookie 关键就展示出来。

**证据。** 原始请求与响应，不能只有浏览器截图。带时间戳。说明 payload 落在了哪里。截图帮助阅读，请求响应才是证据。

**影响。** 也要写给非技术读者看：攻击者得到什么、业务付出什么。这一节决定严重级别。

**修复建议。** 具体一点 —— 参数化查询、输出编码、在服务端检查权限。它表明你理解这个漏洞，而不只是触发了它。

不要写进去的：夸大的说法、真实用户的数据、扫描器原始输出，以及把不相关的发现打包在一份报告里。

### 报告为什么被拒

大多数拒收是可以避免的，而且集中在几种模式里：

- **无法复现。** 步骤默认了一个你没描述的状态，或者漏洞是间歇性的而报告没说。
- **影响没讲清。** "这可能导致……"却没有展示后果。如果效果只是理论上的，就直说，让审核去降级，而不是暗示更多。
- **低于项目门槛。** 自 XSS、没有敏感动作的点击劫持、无法证明读取的 CORS、缺安全头、版本号泄露、开放重定向，通常都在范围外。不是因为它们不真实，而是因为那不是这个项目付钱买的东西。花一天之前先读受理标准。
- **重复。** 除了更快、挑更少人测的资产，别无他法。
- **范围外**，包括"只有从范围外主机才能触达"这种情况。
- **用了真实用户数据** —— 无论发现本身如何，这都是违规。
- **一份报告塞十个问题。** 审核是按单个发现评级的；打包的会被拆开或直接关掉。分开报，从最严重的开始。

### 沟通

- **用最小证明报告，然后停手。** 不要超出影响所需的范围继续枚举，也不要为了证明观点去导出数据。
- **一份报告一个漏洞。**
- **耐心且礼貌。** 响应时间从几小时到几个月都有；审核员是一个有队列的人，敌意让你损失多于所得。
- **绝不威胁披露。** 按公开政策走，没有政策就去问。
- **不同意定级时**，带证据和理由回应一次，然后接受决定。反复升级是测试者被封号的典型方式。
- **自己留记录。** 你测过什么、怎么测的记录，可以在日后有人质疑某个请求时保护你。

### 检测与缓解（项目方视角）

- **把范围写精确。** 模糊就会被测，而一句澄清话的成本远低于一次事故的成本。
- **公布首次响应时间并做到。** 24 小时给回复的项目，收到的报告质量高于一个月才回的 —— 因为测试者会据此分配自己的精力。
- **有明确的审核标准。** 自 XSS 算不算、某类数据的严重级别是什么、重复如何判定。标准不一致会把好的测试者赶走。
- **把发现连同回归测试一起进待办。** 修了但没测试的漏洞会回来。
- **看报告之间的模式。** 十个测试者在不同接口发现授权缺失，那是一个系统性问题，不是十张工单。
- **监控范围外的活动**，好在你事后看日志之前就能叫停。
