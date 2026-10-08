---
id: red-team-tactical-map
title_en: Red Team Tactical Map and Coverage Checklist
title_zh: 红队战术地图与覆盖查漏清单
summary_en: Every other entry in this guide is one technique. This one is the index — the engagement laid out as a sequence, with the questions that decide whether a phase was actually covered. Use it to plan scope, and again before writing the report to find what you never tried.
summary_zh: 本指南其他每一篇都是单个技术点，这一篇是索引 —— 把一次项目摊开成一条序列，配上"这个阶段到底做没做"的问题。规划范围时过一遍，写报告前再过一遍，找出你从没试过的地方。
tags: [red-team, methodology, checklist, coverage, attack-chain, planning]
attck: [T1595, T1190, T1078, T1059, T1548, T1003, T1021, T1547, T1070, T1071, T1041]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why this page exists

The expensive failures in a red team engagement are rarely "we could not get in". They are:

- "We never tested the cloud account."
- "Nobody looked at the build pipeline."
- "We got domain admin and never asked whether anyone had noticed."
- "The report arrived and the client asked why the mobile app was untouched."

None of those are technical failures. They are **coverage** failures, and they happen because an engagement is a sequence of decisions made under time pressure, where whatever you are good at absorbs the hours.

This page is the countermeasure. It is a map of the whole engagement, arranged so a gap is visible.

### How to use it

**Twice.** Once at the start, to turn the scope into a plan and notice which stages have no time allocated. Once before the report, to find out what was never tried — and to write that into the report honestly, because "not tested" is a finding the client needs.

**Two axes.** Down the page is the engagement as a sequence of stages. Across it is a set of surfaces that cut through every stage, and that is where the largest gaps usually are: a team that lives in Active Directory can go a whole engagement without touching cloud, CI or the mobile client, and never notice.

**Read the items as questions, not tasks.** The point is not to tick every box but to be able to say why a box does not apply.

### The vertical axis: the stages

#### 1. Scoping and rules of engagement

- [ ] The asset list is complete: subsidiaries, recent acquisitions, cloud accounts, third-party hosted services, IPv6 ranges, non-production environments that hold production data.
- [ ] Prohibited actions are written down, not assumed: no denial of service, no social engineering, no physical, no touching the safety systems.
- [ ] There is a named emergency contact on the client side, and a stop procedure everyone knows.
- [ ] Test windows and rate limits are agreed, including for anything that could page an on-call engineer.
- [ ] Data handling is agreed: what may be exfiltrated, where it is stored, when it is destroyed.

**Often missed:** cloud and CI are not in the asset list because nobody thought of them as "assets"; IPv6 is live but unlisted; a recently acquired company's domain is in scope on paper and unknown to the testers.

#### 2. Reconnaissance

- [ ] External DNS, subdomains, certificate transparency, historical records.
- [ ] Cloud storage, snapshots, container registries, exposed management panels.
- [ ] Leaked credentials: public repositories, paste sites, CI logs, configuration files in images.
- [ ] People: technology stack hints in job adverts, vendor lists, conference talks.
- [ ] Third parties with network or identity access: MSPs, contractors, SaaS integrations.

**Often missed:** the IPv6 range; an old domain that still resolves and still authenticates; GitHub organisations and their forks; a vendor with a permanent VPN.

Related: `-x id:attack-surface-recon`

#### 3. Initial access

- [ ] Internet-facing services, including the appliances nobody inventories (VPN, mail gateway, file transfer).
- [ ] Credential reuse and password spraying against anything that authenticates.
- [ ] Phishing and pretexting, if it is in scope.
- [ ] Valid accounts from a leak, including in the cloud control plane and in SaaS.
- [ ] Supply chain: a build pipeline, a package, a managed service.

**Often missed:** the VPN concentrator and the file-transfer appliance, which are both the most exposed and the least patched; an SSO-integrated SaaS app that has never been reviewed.

Related: `-x id:vulnerable-outdated-components`, `-x id:authentication-flaws`, `-x id:supply-chain-security`

#### 4. Foothold and execution

- [ ] The access is stable enough to survive a restart or a service bounce.
- [ ] You know exactly which account and which host you hold, and whether it is a service account.
- [ ] There is an egress path that does not depend on the exploit that got you in.
- [ ] The evidence of your entry is understood: what did this action write, log or trigger?

**Often missed:** treating a reverse shell from an unpatched service as a stable foothold, then losing it mid-engagement and spending a day getting back in.

Related: `-x id:tunneling-and-pivoting`

#### 5. Privilege escalation

- [ ] Local: service misconfiguration, writable paths, tokens and privileges, scheduled tasks, kernel.
- [ ] Domain: ACL abuse, certificate services, delegation, group membership paths.
- [ ] Cloud: policy versions, role passing, trust policies, service code updates.

**Often missed:** `SeImpersonatePrivilege` on service accounts; a certificate template that is a one-step route to domain admin; an IAM permission that looks administrative in a spreadsheet.

Related: `-x id:linux-privilege-escalation`, `-x id:ad-acl-abuse`, `-x id:adcs-certificate-abuse`, `-x id:aws-iam-privilege-escalation`

#### 6. Credential access

- [ ] Memory and disk: LSASS, SAM, LSA secrets, cached domain credentials.
- [ ] Configuration and scripts: application configs, deployment scripts, unattended install files, GPP.
- [ ] Browsers, password managers, Wi-Fi profiles, saved RDP credentials.
- [ ] CI/CD variables and build secrets — often the fastest route to cloud or production.
- [ ] Cloud credentials in metadata, environment variables, container layers.

**Often missed:** the build system, which holds credentials for everything and is rarely treated as a Tier 0 target; secrets baked into container images.

Related: `-x id:kerberoasting`, `-x id:ntlm-relay`, `-x id:software-data-integrity-failures`

#### 7. Discovery

- [ ] Domain and forest: trusts, sites, group membership, privileged accounts.
- [ ] Sessions: which privileged account is logged in where — the graph an attacker actually walks.
- [ ] Shares, backups, and where the project files live.
- [ ] The cloud control plane: what this identity can do, and what it can assume.
- [ ] What the client believes you can reach, versus what you actually can.

**Often missed:** a single service account's sessions across a dozen hosts; a trust to a domain that was supposedly decommissioned.

Related: `-x id:internal-network-methodology`

#### 8. Lateral movement

- [ ] Credential-based: SMB, WinRM, RDP, SSH, remote services.
- [ ] Ticket-based: pass-the-ticket, overpass-the-hash, delegation abuse.
- [ ] Certificate-based: PKINIT with a stolen certificate.
- [ ] Cloud-to-on-premise and back: a hybrid identity path is often the shortest route between two environments that are assumed separate.

**Often missed:** the hybrid path. A cloud identity that can authenticate on-premise, or an on-premise account that can assume a cloud role, collapses two environments into one.

Related: `-x id:windows-lateral-movement`

#### 9. Persistence

- [ ] Host: services, scheduled tasks, WMI subscriptions, accounts, SSH keys.
- [ ] Domain: ACL changes, certificate templates, SID history, trust modification.
- [ ] Cloud: additional access keys, modified trust policies, a function with a legitimate-looking update.
- [ ] Build pipeline: a modified artifact, a workflow file, a publishing token.

**Often missed:** cloud-side persistence entirely; persistence placed in the build pipeline, which survives every host rebuild.

#### 10. Defense evasion

- [ ] What is actually watching: EDR, Sysmon, application logs, cloud audit logs, the SIEM.
- [ ] Logging gaps you created, and logging gaps that already existed.
- [ ] Whether your tooling is signed, unusual, or already known to the defenders.

**Often missed:** assuming "no alert" means "no detection". Ask for the detection data afterwards, in a controlled way.

Related: `-x id:edr-evasion-fundamentals`, `-x id:logging-monitoring-failures`

#### 11. Command and control

- [ ] A channel that matches the network's normal traffic, with a fallback.
- [ ] Egress rules tested: which destinations are actually allowed.
- [ ] Domain reputation and categorisation considered, for anything that touches the internet.

Related: `-x id:c2-fundamentals`

#### 12. Collection and exfiltration

- [ ] The data that matters was identified, not just the data that was easy to reach.
- [ ] The exfiltration was demonstrated with the minimum volume that proves it.
- [ ] The route was tested, and the volume that would trigger a DLP or egress alert is understood.

**Often missed:** proving access to the data without actually taking it, and saying so in the report — which is usually what the client wanted.

#### 13. Objective and impact

- [ ] The agreed objective is reached, and there is evidence that is not ambiguous.
- [ ] The impact is expressed in business terms: which system, which data, how many users.
- [ ] The path that matters is the one you report, not the most technically interesting one.

#### 14. Detection assessment

- [ ] For each significant action: was it logged, was it alerted, was it answered?
- [ ] The detection matrix is built from the client's own data, reviewed with them, not asserted.
- [ ] The noisy actions are listed so the client can tune, and the quiet ones are listed so they know where the blind spots are.

**Often missed:** this whole stage. It is the most valuable half of a red team deliverable and the first thing dropped when time runs short.

Related: `-x id:logging-monitoring-failures`

#### 15. Reporting and cleanup

- [ ] Every artefact you created is removed, and a list of what was created is in the report.
- [ ] Credentials and data collected are destroyed, with the destruction recorded.
- [ ] The report separates "exploited", "tested and found resistant", and "not tested" — the last one is a finding.
- [ ] The remediation advice is specific enough to be actionable by the team that owns each system.

### The horizontal axis: surfaces that cut across every stage

| Surface | Why it gets skipped | In this guide |
|---|---|---|
| Cloud control plane | Nobody in the team owns it, and it is not on the network diagram | `-x id:cloud-security-fundamentals` |
| Containers and Kubernetes | Treated as "the platform team's problem" | `-x id:docker-escape`, `-x id:kubernetes-attack-paths` |
| CI/CD and supply chain | Seen as tooling, not as an attack surface | `-x id:supply-chain-security`, `-x id:software-data-integrity-failures` |
| Mobile clients | Out of scope by default, and a separate skill set | `-x id:mobile-app-testing` |
| Wireless and IoT | Needs hardware and a site visit | `-x id:iot-embedded-security` |
| OT and industrial | Requires a completely different safety conversation | `-x id:ics-scada-security` |
| AI agents and MCP | New enough that nobody has added it to the plan | `-x id:ai-agent-and-mcp-security` |
| Web applications | Thought to be covered, but usually only at the surface | `-x id:sqli-sql-injection`, `-x tag:web` |
| Code review | Skipped when there is no source access, and skipped anyway when there is | `-x id:secure-code-review` |
| People | Social engineering is often out of scope, which is worth confirming | `-x id:src-reporting-and-methodology` |

### The ten things engagements miss most

1. **The cloud account**, including who can assume what, and the metadata service.
2. **The build pipeline**, which holds credentials for everything and is trusted by everything.
3. **IPv6**, live on most networks, present in few scopes.
4. **Forgotten assets**: old domains, acquisitions, shadow IT, staging environments with production data.
5. **Third-party access**: the MSP, the contractor, the vendor VPN that was installed for a migration in 2019.
6. **Tokens and sessions**, not hashes. Stealing a live session is faster and quieter than cracking anything.
7. **ADCS and delegation**, which are frequently a one-step route to domain admin and rarely audited.
8. **Implicit trust between internal services**: what happens after you compromise the thing that is allowed to talk to everything else.
9. **Backups and snapshots**, where the data actually is.
10. **The detection assessment**, because "we got in" is only half of what the client is buying.

### If you only have a day

Test the three things that most often decide the outcome: an external service that is internet-facing and unpatched, a credential reused between an ordinary user and something privileged, and the path of least resistance from a workstation to the identity that matters. Then ask whether anyone would have noticed.

### Detection, for the defender

This map reads in both directions. Every stage above is a list of actions an attacker takes, and every one of those actions is something that should have been logged, alerted on and answered. The useful exercise is to take the stages most likely in your environment and ask, for each one, whether you would actually have known.

The "ten things" list doubles as a list of the places defenders usually have no visibility at all, and how to measure the assessment itself is in the logging entry.

Related: `-x id:logging-monitoring-failures`

### Mitigation, for the defender

Read the horizontal axis as your own coverage requirement rather than the attacker's. Cloud, containers, CI, mobile, wireless, OT and the agent layer are the surfaces most organisations have neither tested nor instrumented, and they are where an assessment most often finds nothing simply because nobody looked.

Read the vertical axis as a control inventory. Every stage where you cannot name the control that applies is a stage where the only thing standing between an attacker and the objective is luck.

Related: `-x id:insecure-design`

<!-- lang:zh -->
### 这一篇为什么存在

红队项目里代价最大的失败，很少是"我们进不去"，而是：

- "我们完全没测云账号。"
- "没人看构建流水线。"
- "我们拿到域管了，但从没问过有没有人发现。"
- "报告交了，客户问移动端为什么一个字都没写。"

这些都不是技术失败，是**覆盖面**的失败。它们之所以发生，是因为一次项目就是一连串在时间压力下做的决策 —— 而你擅长的那部分，会把所有工时都吸走。

这一篇就是对策。它把整次项目摊成一张地图，让缺口看得见。

### 怎么用它

**用两次。** 开始前一次，把范围变成计划，并发现哪些阶段根本没分配时间；写报告前再一次，找出从没试过的地方 —— 并且诚实地写进报告，因为"未测试"本身就是客户需要知道的发现。

**两个方向。** 纵向是项目按阶段的推进顺序；横向是一组**横穿所有阶段的面**，而最大的缺口通常就在那里：一个活在 Active Directory 里的团队，可以整个项目都不碰云、CI 和移动端，而且完全没察觉。

**把每一项当问题读，而不是当任务。** 要点不是把每个框都打上勾，而是能说清楚为什么某个框不适用。

### 纵向：各个阶段

#### 1. 范围与交战规则

- [ ] 资产清单是完整的：子公司、近期收购、云账号、第三方托管服务、IPv6 段、持有生产数据的非生产环境。
- [ ] 禁止项是写下来的，不是默认的：不做拒绝服务、不做社会工程、不做物理接触、不碰安全系统。
- [ ] 客户侧有指定的应急联系人，且所有人都知道中止流程。
- [ ] 测试窗口与速率限制已约定，包括任何可能把值班工程师叫起来的东西。
- [ ] 数据处理已约定：什么可以被带出、存在哪里、什么时候销毁。

**常被漏掉：** 云和 CI 不在资产清单里，因为没人把它们当成"资产"；IPv6 是活的，但没被列进去；刚被收购那家公司的域名纸面上在范围内，而测试人员根本不知道。

#### 2. 侦察

- [ ] 外部 DNS、子域、证书透明度、历史记录。
- [ ] 云存储、快照、容器镜像仓库、暴露的管理面板。
- [ ] 泄漏的凭据：公开仓库、paste 站点、CI 日志、镜像里的配置文件。
- [ ] 人：招聘广告里的技术栈线索、供应商清单、会议演讲。
- [ ] 拥有网络或身份权限的第三方：外包运维、承包商、SaaS 集成。

**常被漏掉：** IPv6 段；一个仍然能解析、仍然能认证的旧域名；GitHub 组织及其 fork；一个握着永久 VPN 的供应商。

相关：`-x id:attack-surface-recon`

#### 3. 初始访问

- [ ] 面向互联网的服务，包括没人做台账的那些设备（VPN、邮件网关、文件传输）。
- [ ] 凭据复用与口令喷洒，打任何会认证的东西。
- [ ] 钓鱼与话术，如果在范围内。
- [ ] 来自泄漏的有效账号，包括云控制面和 SaaS 里的。
- [ ] 供应链：构建流水线、某个包、某个托管服务。

**常被漏掉：** VPN 网关和文件传输设备 —— 它们既最暴露、又最缺补丁；一个接了 SSO、从来没人审过的 SaaS 应用。

相关：`-x id:vulnerable-outdated-components`、`-x id:authentication-flaws`、`-x id:supply-chain-security`

#### 4. 立足点与执行

- [ ] 这个访问稳定到能扛过一次重启或服务重启。
- [ ] 你清楚自己握着的是哪个账号、哪台主机，以及它是不是服务账号。
- [ ] 有一条不依赖"当初进来的那个漏洞"的出网通道。
- [ ] 你理解自己进入的痕迹：这个动作写了什么、记了什么日志、触发了什么。

**常被漏掉：** 把一个来自未打补丁服务的反弹 shell 当成稳定立足点，中途掉了，然后花一天重新进去。

相关：`-x id:tunneling-and-pivoting`

#### 5. 提权

- [ ] 本地：服务配置错误、可写路径、令牌与特权、计划任务、内核。
- [ ] 域：ACL 滥用、证书服务、委派、组成员关系路径。
- [ ] 云：策略版本、角色传递、信任策略、服务代码更新。

**常被漏掉：** 服务账号上的 `SeImpersonatePrivilege`；一个一步到域管的证书模板；一条在表格里看起来像管理类的 IAM 权限。

相关：`-x id:linux-privilege-escalation`、`-x id:ad-acl-abuse`、`-x id:adcs-certificate-abuse`、`-x id:aws-iam-privilege-escalation`

#### 6. 凭据访问

- [ ] 内存与磁盘：LSASS、SAM、LSA secrets、缓存的域凭据。
- [ ] 配置与脚本：应用配置、部署脚本、无人值守安装文件、GPP。
- [ ] 浏览器、口令管理器、Wi-Fi 配置、保存的 RDP 凭据。
- [ ] CI/CD 变量与构建机密 —— 往往是通往云或生产最快的一条路。
- [ ] 元数据、环境变量、容器层里的云凭据。

**常被漏掉：** 构建系统 —— 它握着通往一切的凭据，却极少被当成 Tier 0 目标；烤进容器镜像里的机密。

相关：`-x id:kerberoasting`、`-x id:ntlm-relay`、`-x id:software-data-integrity-failures`

#### 7. 发现

- [ ] 域与林：信任、站点、组成员关系、特权账号。
- [ ] 会话：哪个特权账号登录在哪里 —— 这才是攻击者真正会走的那张图。
- [ ] 共享、备份，以及项目文件放在哪。
- [ ] 云控制面：这个身份能做什么、能承担什么。
- [ ] 客户以为你能到达什么，与你实际能到达什么。

**常被漏掉：** 一个服务账号在十几台主机上的会话；一条通向往日"已下线"域名的信任。

相关：`-x id:internal-network-methodology`

#### 8. 横向移动

- [ ] 基于凭据：SMB、WinRM、RDP、SSH、远程服务。
- [ ] 基于票据：pass-the-ticket、overpass-the-hash、委派滥用。
- [ ] 基于证书：用窃来的证书做 PKINIT。
- [ ] 云到本地、本地到云：混合身份路径，往往是两个"被认为彼此隔离"的环境之间最短的路。

**常被漏掉：** 混合路径本身。一个能在本地认证的云身份，或者一个能承担云角色的本地账号，会把两个环境压成一个。

相关：`-x id:windows-lateral-movement`

#### 9. 持久化

- [ ] 主机：服务、计划任务、WMI 订阅、账号、SSH key。
- [ ] 域：ACL 变更、证书模板、SID history、信任修改。
- [ ] 云：额外的 access key、被改过的信任策略、一次看起来合法的函数更新。
- [ ] 构建流水线：被改过的产物、工作流文件、发布 token。

**常被漏掉：** 云侧持久化被整个跳过；以及放在构建流水线里的持久化 —— 它能扛过每一次主机重建。

#### 10. 防御规避

- [ ] 到底有什么在看：EDR、Sysmon、应用日志、云审计日志、SIEM。
- [ ] 你自己制造的日志缺口，以及本来就存在的日志缺口。
- [ ] 你的工具是签名的、罕见的，还是防守方早就认识的。

**常被漏掉：** 把"没有告警"当成"没有被发现"。事后要用受控的方式把检测数据要过来看。

相关：`-x id:edr-evasion-fundamentals`、`-x id:logging-monitoring-failures`

#### 11. 命令与控制

- [ ] 一条与网络正常流量相称的通道，并且有备用。
- [ ] 出网规则实测过：到底哪些目的地是允许的。
- [ ] 任何触网的东西都考虑过域名信誉与分类。

相关：`-x id:c2-fundamentals`

#### 12. 收集与外泄

- [ ] 被识别出来的是**真正要紧的数据**，不只是容易拿到的数据。
- [ ] 外泄是用"能证明问题的最小体量"演示的。
- [ ] 路线实测过，并且清楚多大的体量会触发 DLP 或出网告警。

**常被漏掉：** 证明"能访问"而没有真的把数据拿走，并且在报告里说明这一点 —— 这通常才是客户想要的。

#### 13. 目标与影响

- [ ] 约定的目标达成了，并且有不含歧义的证据。
- [ ] 影响是用业务语言表达的：哪个系统、什么数据、多少用户。
- [ ] 报告里写的是**真正要紧的那条路径**，而不是技术上最漂亮的那条。

#### 14. 检测能力评估

- [ ] 对每一个重要动作：记录了吗、告警了吗、响应了吗？
- [ ] 检测矩阵是用客户自己的数据做出来的、和他们一起复核的，而不是你自己断言的。
- [ ] 吵的那些动作列出来供客户调优，静的那些列出来让他们知道盲区在哪。

**常被漏掉：** 这整个阶段。它是红队交付物里最有价值的一半，也是时间一紧最先被砍掉的。

相关：`-x id:logging-monitoring-failures`

#### 15. 报告与清理

- [ ] 你制造的每一个产物都被清理，且"造过什么"的清单在报告里。
- [ ] 收集到的凭据与数据已销毁，且销毁有记录。
- [ ] 报告把"已利用""测过且防住了""未测试"分开写 —— 最后一项是发现。
- [ ] 修复建议具体到各个系统的负责团队能直接执行。

### 横向：横穿所有阶段的面

| 面 | 为什么会被跳过 | 本指南 |
|---|---|---|
| 云控制面 | 团队里没人负责它，它也不在网络图上 | `-x id:cloud-security-fundamentals` |
| 容器与 Kubernetes | 被当成"平台团队的事" | `-x id:docker-escape`、`-x id:kubernetes-attack-paths` |
| CI/CD 与供应链 | 被看成工具链，而不是攻击面 | `-x id:supply-chain-security`、`-x id:software-data-integrity-failures` |
| 移动客户端 | 默认就在范围外，而且技能栈不同 | `-x id:mobile-app-testing` |
| 无线与 IoT | 需要硬件和现场 | `-x id:iot-embedded-security` |
| 工控 | 需要另一场完全不同的安全沟通 | `-x id:ics-scada-security` |
| AI agent 与 MCP | 太新，没人把它加进计划 | `-x id:ai-agent-and-mcp-security` |
| Web 应用 | 以为覆盖了，其实通常只在表面 | `-x id:sqli-sql-injection`、`-x tag:web` |
| 代码审计 | 拿不到源码时跳过，拿到了也照样跳过 | `-x id:secure-code-review` |
| 人 | 社会工程常常在范围外，而这值得确认一句 | `-x id:src-reporting-and-methodology` |

### 最常被漏掉的十件事

1. **云账号**，包括谁能承担什么角色，以及元数据服务。
2. **构建流水线**，它握着通往一切的凭据，又被一切信任。
3. **IPv6**，在多数网络上是活的，在多数范围里是缺的。
4. **被遗忘的资产**：旧域名、被收购的公司、影子 IT、持有生产数据的 staging。
5. **第三方接入**：外包运维、承包商、2019 年为了迁移装的那个厂商 VPN。
6. **令牌与会话**，而不是哈希。偷一个活着的会话比破解任何东西都快、都安静。
7. **ADCS 与委派**，它们常常是一步到域管的路，却极少被审计。
8. **内部服务之间的隐含信任**：等你拿下了那个"被允许和所有东西说话"的组件之后，会发生什么。
9. **备份与快照**，数据真正所在的地方。
10. **检测能力评估**，因为"我们进去了"只是客户买的东西的一半。

### 如果只剩一天

测那三件最常决定结果的事：一个面向互联网且没打补丁的外部服务、一组在普通用户与特权之间被复用的凭据，以及从一台工作站通往那个关键身份的最省力的路径。然后问一句：会有人发现吗？

### 检测（防守方视角）

这张图可以反向读。上面每一个阶段，都是攻击者会采取的动作清单；而其中每一个动作，都应该有它被记录、被告警、被响应的地方。有用的做法是挑出你环境里最可能发生的几个阶段，逐个问：**我们真的会知道吗？**

"最常被漏掉的十件事"同样可以当作"防守方通常完全没有可见性的地方"清单；评估本身怎么做，日志与监控那篇里有。

相关：`-x id:logging-monitoring-failures`

### 缓解（防守方视角）

把横向那几个面读成**你自己的覆盖要求**，而不是攻击者的：云、容器、CI、移动端、无线、工控、agent 层，是大多数组织既没测过、也没埋过点的地方 —— 也正是一次评估"什么都没发现"最常见的原因：根本没人看。

把纵向那些阶段读成**控制清单**。任何一个你说不出"这里适用什么控制"的阶段，就是攻击者通往目标之间只隔着运气的阶段。

相关：`-x id:insecure-design`
