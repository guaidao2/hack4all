---
id: subdomain-takeover-fundamentals
title_en: "Subdomain Takeover — When a Record Outlives What It Points At"
title_zh: "子域接管：当一条记录比它指向的东西活得久"
summary_en: A DNS record is a claim that a name points somewhere, and the somewhere can stop recognising the name. Measured with a detector that asks each upstream whether it still claims it, sorted into four verdicts rather than two.
summary_zh: 一条 DNS 记录是一个声明：这个名字指向某处。而那个"某处"可以不再认这个名字。这一篇用一个检测器实测 —— 逐条去问上游是否还认领它，并分成四种判定而不是两种。
tags: [web, subdomain-takeover, dns, cname, cwe-284]
tools: [dig, curl, python3]
attck: [T1584.001, T1583.001]
platform: [web, dns]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### A claim that outlives its subject

A DNS record is a statement that a name points at something. It is created deliberately, it is edited rarely, and **nothing about it expires when the thing it points at goes away**.

> **A record is a claim about ownership, and the owner at the other end can change.**

That is the whole class. A subdomain still resolves to a cloud bucket, a hosting platform, an application host or a CDN distribution; the resource behind it was deleted in a decommission that nobody connected back to the DNS entry; and the name is now pointing at a place where **anyone can claim it**.

Two conditions have to hold, and both are worth stating because the second one is what makes an individual finding exploitable or not:

| Condition | Why it is usually true |
|---|---|
| **The record still exists** | Deleting a service and deleting its DNS entry are two different actions, done by different people at different times |
| **The upstream lets anyone claim that name** | Many hosting, storage and page-hosting services do — the name is the only credential |

Where the upstream does **not** allow arbitrary claiming, a dangling record is still a finding — an error page on your own subdomain, a name resolving to nothing useful — but it is not a takeover.

### Four layers

1. **The trust boundary.** Browsers, users, and the organisation's own policies trust that content under a domain belongs to the organisation that registered it. **Ownership of a name, as far as anything technical is concerned, is decided by where it points**, not by who paid for it.
2. **Data and instruction share a plane.** A DNS record is simultaneously **a mapping from a name to an address** and **a declaration of who owns the content at that name**. The second reading is the one every trust decision depends on, and it is the one that changes silently when the first one is left dangling.
3. **Why the usual fix fails.** "Nobody uses that subdomain any more" is true and irrelevant. The question is not whether anyone is using it but **whether anyone can**. And a record that resolves correctly, serves an error page, and is present in a zone file is not obviously broken to any monitoring that checks liveness.
4. **The variants.** A dangling `CNAME` to a deleted resource; an `A` record pointing at an address that has been released and reassigned; a delegated `NS` record for a subdomain nobody maintains; a wildcard record that catches names nobody intended to publish; and cloud resource names that are globally unique and first-come-first-served.

### Detection is mechanical

This is the part that makes the class tractable, and the lab's framing of it is the clearest statement of why:

> **Every record is a claim that a name points somewhere. Verifying the claim means going to that somewhere and asking whether it still recognises the name.**

Measured with a detector that does exactly that against a list of six records:

| Record | Points at | Verdict | Evidence |
|---|---|---|---|
| `blog.example.com` | a live host | **claimed** | `HTTP 200`, normal content |
| `assets.example.com` | released object storage | **dangling** | `HTTP 404`, fingerprint `NoSuchBucket` |
| `app.example.com` | a decommissioned app host | **suspicious** | `HTTP 404`, **no recognised fingerprint** |
| `docs.example.com` | a deleted pages site | **dangling** | `HTTP 404`, fingerprint `no GitHub Pages site here` |
| `shop.example.com` | a live host | **claimed** | `HTTP 200` |
| `old.example.com` | an address with nothing on it | **undetermined** | connection failed — **which is itself a signal** |

**Four verdicts rather than two**, and the last two are the ones a checklist is tempted to drop:

| Verdict | What it means | Action |
|---|---|---|
| **Claimed** | The upstream still serves this name | none |
| **Dangling** | A fingerprint matched — the upstream says it does not know the name | **remove the record or claim the name** |
| **Suspicious** | An error without a recognised fingerprint | **a human looks** |
| **Undetermined** | The target does not answer at all | **a human looks** |

**Treating the bottom two as "fine" is how the finding is missed**, because the fingerprint list is a moving target: platforms change their error pages, add languages, or are replaced. A detector without a human review step for the unrecognised cases will report "no takeovers" on a zone full of them.

### What a takeover is worth

The mistake is to file this as "a subdomain", which sounds minor. The measured consequences are all of the form **an attacker now controls content under a name you trust**:

| Reachable from the taken subdomain | How |
|---|---|
| **Session cookies** | A cookie scoped to `.example.com` covers every subdomain, so the taken host is inside its scope |
| **A CORS allowlist** | An allowlist written as `*.example.com` — the CORS entry's suffix mistake — now includes a host the attacker controls |
| **An OAuth redirect URI** | A callback allowlist containing a subdomain means an authorization code can be delivered there |
| **Framing exceptions** | One of the origins permitted to frame the main site by `frame-ancestors` |
| **Mail** | Sending under a domain the organisation's SPF and DMARC cover, which changes how a phishing mail reads to every recipient |
| **Certificates** | A domain-validated certificate for the name, so the HTTPS padlock is present and correct |

Each of those is a finding from an earlier entry in this guide, and a takeover is a way to reach several of them at once. **The generalisation is that the name is the credential**, which is why a subdomain takeover is worth more than the "subdomain" in its name suggests.

### Detection and mitigation

- **Enumerate the records and ask each upstream whether it still claims the name.** The measured method, run on a schedule. The fingerprints are per-platform and change over time, so the check needs a review step for the cases it does not recognise rather than a pass/fail.
- **Watch certificate transparency logs for your own names.** A certificate issued for a name nobody in the organisation requested means somebody else has control of it — and since the log is public and append-only, **it is the one detection that fires after the takeover rather than before it**.
- **Alert when a subdomain serves an error page from the upstream rather than from the application.** That is the state between "service removed" and "record removed", and it is short and visible.
- **And include the check in the decommissioning process rather than only in monitoring.** The record is created by one process and removed by another, so the control has to live where the removal happens.
- **For mitigation, remove the record as part of removing the service** — in that order, because removing the service first and the record later is exactly how a name becomes dangling. A decommission checklist with "delete the DNS entry" on it is the whole fix for the common case.
- **Do not keep records for names nothing uses.** A record kept "in case" is a claim nobody is maintaining, and it costs nothing to delete and recreate.
- **If a name is already dangling, claim it before anyone else does.** Registering the resource at the upstream is the immediate remediation, because until it is claimed, the exposure is open to whoever looks first.
- **Narrow everything that trusts a whole domain.** Cookies scoped to the registrable domain, CORS allowlists written as a suffix, callback lists containing subdomains and framing exceptions granted to a wildcard are each a place where one taken subdomain becomes a problem for the main site. **Listing exact hosts instead of a domain removes the amplification**, and it applies whether or not a dangling record exists today.
- **And treat this as a process finding, not a code finding.** Nothing in the application is wrong. The bug lives in the gap between two operations performed by two teams, which is why it survives code review, static analysis and every test that exercises the running application — none of them look at the zone file.

<!-- lang:zh -->
### 一个比它所指的东西活得更久的声明

一条 DNS 记录是一个"这个名字指向某处"的陈述。它是被刻意创建的、极少被修改，而**当它指向的东西消失时，它本身没有任何部分会过期**。

> **一条记录是一份关于归属的声明，而另一端的归属者可以换人。**

这就是整一类。一个子域仍然解析到一个云存储桶、一个托管平台、一台应用主机或者一个 CDN 分发；它背后的资源在某次下线里被删掉了，而没有人把那件事和这条 DNS 记录联系起来；于是这个名字现在指向一个**任何人都可以认领**的地方。

两个条件必须同时成立，而两个都值得写出来，因为第二个决定了一条具体的发现能不能被利用：

| 条件 | 为什么通常成立 |
|---|---|
| **记录还在** | 删除一个服务和删除它的 DNS 记录是两个不同的动作，由不同的人在不同的时间做 |
| **上游允许任何人认领那个名字** | 很多托管、存储与页面托管服务就是这样 —— 那个名字是唯一的凭据 |

在上游**不**允许任意认领的地方，一条悬空记录仍然是一条发现 —— 你自己子域上的一个错误页、一个解析到无用之处名字 —— 但它不是接管。

### 四层

1. **信任边界。** 浏览器、用户、以及组织自己的策略都信任"一个域下的内容属于注册那个域的组织"。而**就任何技术层面的东西而言，一个名字的归属由它指向哪里决定**，不是由谁付的钱决定。
2. **数据与指令共用同一平面。** 一条 DNS 记录同时是**一个从名字到地址的映射**和**一份"谁拥有那个名字下内容"的声明**。每一个信任决定都依赖后一种读法，而它在第一种读法被悬空时静默地改变了。
3. **为什么常见修法失败。** "那个子域已经没人用了"是真的，而且不相关。问题不是有没有人在用它，而是**有没有人能用它**。而一条解析正常、返回一个错误页、并且存在于 zone 文件里的记录，对任何检查存活性的监控来说都不明显是坏的。
4. **变体。** 指向已删除资源的悬空 `CNAME`；指向一个已被释放并重新分配的地址的 `A` 记录；一个没人维护的子域上被委派出去的 `NS` 记录；一条捕获了没人打算发布的那些名字的通配记录；以及全局唯一、先到先得的云资源名。

### 检测是机械的

这是让这一类变得可处理的部分，而靶场对它的表述最清楚：

> **每一条记录都是一个"名字指向某处"的声明。验证这个声明，就是去那个某处问一句：你还认这个名字吗？**

用一个正是这么做的检测器对着六条记录实测：

| 记录 | 指向 | 判定 | 依据 |
|---|---|---|---|
| `blog.example.com` | 一台活着的主机 | **已认领** | `HTTP 200`，正常内容 |
| `assets.example.com` | 已释放的对象存储 | **悬空** | `HTTP 404`，指纹 `NoSuchBucket` |
| `app.example.com` | 一台已下线的应用主机 | **可疑** | `HTTP 404`，**没有认出的指纹** |
| `docs.example.com` | 一个已删除的页面站点 | **悬空** | `HTTP 404`，指纹 `no GitHub Pages site here` |
| `shop.example.com` | 一台活着的主机 | **已认领** | `HTTP 200` |
| `old.example.com` | 一个上面什么都没有的地址 | **无法判断** | 连接失败 —— **而这本身就是一个信号** |

**四种判定而不是两种**，而最后两种正是清单会想省掉的：

| 判定 | 它意味着什么 | 动作 |
|---|---|---|
| **已认领** | 上游仍在为这个名字服务 | 无 |
| **悬空** | 命中指纹 —— 上游说它不认识这个名字 | **删记录，或者认领这个名字** |
| **可疑** | 有错误但没有认出的指纹 | **人工看** |
| **无法判断** | 目标完全不回应 | **人工看** |

**把后两种当成"没问题"，就是漏掉这条发现的方式**，因为指纹清单是一个移动的目标：平台会改它们的错误页、加语言、或者被替换掉。一个没有"未识别情形需人工复核"这一步的检测器，会在一个满是悬空记录的 zone 上报告"没有接管"。

### 一次接管值多少

错误在于把它归档成"一个子域"，那听起来很次要。实测到的后果全都是同一个形式：**攻击者现在控制着一个你信任的名字下的内容**。

| 从被接管的子域能碰到什么 | 怎么碰到 |
|---|---|
| **会话 cookie** | 作用域为 `.example.com` 的 cookie 覆盖每一个子域，所以被接管的那台在被覆盖范围内 |
| **CORS 允许清单** | 一份写成 `*.example.com` 的允许清单 —— CORS 那篇的后缀错误 —— 现在包含了一台攻击者控制的主机 |
| **OAuth 重定向 URI** | 一份含某个子域的回调白名单，意味着授权码可以被送到那里 |
| **框住例外** | `frame-ancestors` 允许框住主站的来源之一 |
| **邮件** | 在一个组织的 SPF 与 DMARC 覆盖的域下发信，这会改变一封钓鱼邮件在每一个收件人眼里的可信度 |
| **证书** | 为这个名字申请一张域名验证证书，于是那个 HTTPS 小锁是存在而且正确的 |

上面每一条都是这份指南里某一篇的发现，而一次接管是同时碰到其中好几条的一种方式。**推广开来就是：那个名字就是凭据** —— 这就是为什么一次子域接管的份量，比它名字里的"子域"要大。

### 检测与缓解

- **把记录枚举出来，逐条问上游它还认不认这个名字。** 实测的那个方法，按计划跑。指纹是按平台而定的、会随时间变化，所以这个检查需要一个"对没认出的情形人工复核"的步骤，而不是一个通过/不通过。
- **盯你自己名字的证书透明度日志。** 一张为"组织里没人申请过的名字"签发的证书，意味着别人控制了它 —— 而由于那个日志是公开且只追加的，**它是唯一一条"在接管之后而非之前"触发的检测**。
- **当一个子域返回的是上游的错误页、而不是应用的错误页时告警。** 那是"服务已移除"与"记录已移除"之间的状态，很短、而且可见。
- **并且把这个检查放进下线流程，而不只是放进监控。** 记录由一个流程创建、由另一个流程删除，所以这项控制必须住在删除发生的那个地方。
- **缓解上，把删记录作为删服务的一部分** —— 按这个顺序，因为先删服务、后删记录，正是一个名字变悬空的方式。一份写着"删除 DNS 记录"的下线检查表，就是常见情形下的全部修法。
- **不要为没有东西在用的名字保留记录。** 一条"以防万一"留着的记录，是一份没人在维护的声明，而删掉再重建不花任何代价。
- **如果一个名字已经悬空了，赶在别人之前认领它。** 在上游把那个资源注册下来是立即的补救，因为在它被认领之前，暴露是敞开的、先看到的人先得。
- **收窄一切"信任整个域"的东西。** 作用域为可注册域的 cookie、写成后缀的 CORS 允许清单、含子域的回调清单、以及授予通配符的框住例外，每一个都是"一个子域被接管就变成主站的问题"的地方。**列精确主机名而不是一个域，移除了这个放大效应** —— 而且无论今天有没有悬空记录，它都适用。
- **并且把这一条当成流程发现，而不是代码发现。** 应用里没有任何东西是错的。这个 bug 住在"由两个团队执行的两个操作之间的缝"里，这就是它能活过代码评审、静态分析、以及每一个跑在运行中的应用上的测试的原因 —— 它们没有一个去看 zone 文件。
