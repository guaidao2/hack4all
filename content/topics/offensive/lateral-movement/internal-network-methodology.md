---
id: internal-network-methodology
title_en: Internal Network and Domain Methodology
title_zh: 内网与域渗透方法论
summary_en: Domain attacks are a collection of techniques; this entry is the line that connects them. Where to look first, what to collect before moving, and why the shortest path to the objective usually beats the most impressive one.
summary_zh: 域内攻击是一堆具体技术的集合，这一篇是把它们串起来的那条线：先看哪里、动手前该收什么、以及为什么通往目标的最短路径通常胜过最漂亮的那条。
tags: [active-directory, internal-network, methodology, bloodhound, red-team]
tools: [BloodHound, PowerView, netexec, impacket, Rubeus]
attck: [T1018, T1087, T1550]
platform: [windows]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The line, not the collection

This guide has entries for the individual moves: Kerberoasting, relay, ADCS, ACL abuse, lateral movement, tunnelling. What is missing there is the order to do them in, and the judgement about which one to spend the next hour on.

The shape of an internal engagement is almost always the same:

**Foothold → stabilise → understand where you are → harvest what is here → escalate → map the domain → pick a path → reach the objective → stop.**

The two failures are skipping "understand" (and wandering), and never stopping (and turning a clean objective into a noisy mess).

### Stage 1 — Stabilise the foothold

Before anything else, answer three questions about the access you already have:

- **What am I?** `whoami /priv`, `whoami /groups`, `net user %USERNAME% /domain`, and whether the session is interactive, a service, or a network logon.
- **Where am I?** Domain-joined or not, which subnet, what can I reach from here, and do I have an egress path (see the tunnelling entry).
- **How stable is this?** A shell from an exploit that will be patched on reboot is worth less than a service account with a password.

An unstable foothold is the most common reason an engagement stalls. Fixing it — creating a scheduled task, a service, an SSH key, or simply noting the credentials that got you in — is worth more than a quick escalation.

### Stage 2 — Understand the environment

```powershell
ipconfig /all
route print
net view /domain
nltest /dclist:corp.local
net group "Domain Admins" /domain
```

```bash
# from Linux, once you have credentials
nxc smb 10.10.10.0/24 -u user -p pass --gen-relay-list unsigned.txt
nxc ldap dc01.corp.local -u user -p pass --users --groups
```

Write down what you find. An engagement that keeps a running note of subnets, credentials, hosts and access is dramatically more effective than one that rediscovers things every hour.

### Stage 3 — Harvest what is already here

Credentials on the host you control are the cheapest escalation available:

| Source | What it gives |
|---|---|
| LSASS memory | Cleartext passwords, NTLM hashes, Kerberos tickets (needs admin or a specific privilege) |
| SAM and LSA secrets | Local accounts, service account passwords, cached domain credentials |
| `cmdkey /list` | Saved RDP and network credentials |
| Unattended install files (`Unattend.xml`, `sysprep.inf`) | Base image passwords, which are often reused everywhere |
| Group Policy Preferences (`Groups.xml`) | `cpassword`, decryptable with a public key |
| Configuration files, scripts, CI files | Application credentials, sometimes domain ones |
| The registry, browser stores, Wi-Fi profiles | More of the same |
| `AD` user description and comment fields | Passwords, occasionally, left by administrators |

```bash
# the audit of what exists, in one pass
nxc smb 10.10.10.0/24 -u user -p pass --sam --lsa
```

### Stage 4 — Escalate on the host, then in the domain

Local privilege escalation is covered separately. The Windows-specific privileges worth knowing are `SeImpersonatePrivilege` (service accounts, and the path to most potato-family exploits), `SeDebugPrivilege` (LSASS access), `SeBackupPrivilege` (read anything, including the registry hives), and always-install-elevated misconfigurations.

Once you are local admin somewhere, the question becomes domain-wide, and the answer is a graph:

```bash
bloodhound-python -u user -p pass -d corp.local -ns 10.10.10.10 -c All
```

In BloodHound, the questions that matter are the ones that shorten the path:

- Shortest path from my owned principals to Domain Admins.
- Which users have a session on which hosts (the fastest way to find a machine worth compromising).
- Who can write to whom (the ACL entry covers the abuse).
- Kerberoastable accounts with a path to privilege.
- Certificate templates that allow escalation (the ADCS entry).
- Trusts to other domains, and whether they are filtered.

### Stage 5 — Choose the path deliberately

Every viable route has three costs, and picking well is most of the skill:

| Cost | What to consider |
|---|---|
| Access required | Do you already hold it, or does the route need another step first? |
| Detectability | Does it create service installations, new accounts, certificate requests, or a burst of Kerberos traffic? |
| Time | Is this a five-minute action or a multi-day operation? |

A practical ordering for most environments, cheapest first:

1. **Reuse a credential you already have** — a local admin password that works elsewhere because LAPS is not deployed.
2. **Read what is exposed** — shares, GPP, description fields, SYSVOL.
3. **Kerberoast and crack** — quiet if you avoid RC4-heavy requests, and often rewarded with a service account that is a local admin somewhere.
4. **ACL abuse** — a write right two hops from Domain Admins is a clean, low-noise path.
5. **ADCS** — frequently a one-step route to domain admin, and frequently unnoticed.
6. **Relay and coerce** — powerful, but loud and dependent on signing configuration.
7. **Exploit a missing patch** — the loudest and least predictable, and usually unnecessary.

The last one deserves emphasis: in most real environments, the unpatched vulnerability is not the cheapest route. It is the one people reach for because it is the one they remember.

### Stage 6 — Reach the objective, then stop

"Domain admin" is usually a means, not the objective. The objective is whatever the client wrote in the scope: a specific database, a finance system, a proof of access to a tier of the network. Reaching it and documenting how is the engagement. Continuing past it — dumping every hash, creating persistence nobody asked for, moving laterally for fun — is how a good result turns into an incident report about you.

Before finishing, agree what to leave behind (usually nothing) and what to clean up.

### Detection

- **BloodHound is a double-edged tool**: run it yourself, and treat every path it finds to Tier 0 as a ticket. What an attacker finds in an hour is what you could have found first.
- **Session hunting**: which privileged accounts are logged into which workstations. That is the graph an attacker follows, and it is usually a finding on its own.
- **Account usage anomalies**: a service account logging in interactively, a user authenticating from a subnet they never use, or a computer account authenticating where it should not.
- **Honeytokens**: a credential in a description field, an unused service account with an SPN, a share with a suggestive name. Cheap, and they turn quiet enumeration into an alert.
- **Layered administration** is the only structural fix. Everything above is detection; tiering is prevention.

### Mitigation

- **Tier the administration model** and enforce it: Tier 0 accounts only on Tier 0 systems, workstations administered separately, no shared local admin passwords (deploy LAPS).
- **Remove the easy harvest**: no credentials in GPP or description fields, no unattended install files, no service accounts with `SeImpersonate` where they are not needed.
- **Reduce the graph's edges**: every `GenericAll`, every unconstrained delegation, every SPN on a privileged account is a path. Audit them with the same tooling attackers use.
- **Monitor identity, not just endpoints**: anomalous logons, new group memberships, certificate requests, and replication traffic are all visible in the domain's own logs.
- **Assume a foothold and rehearse**: an exercise that starts from "an attacker has a workstation" is worth more than another vulnerability scan.

<!-- lang:zh -->
### 要的是那条线，不是那堆技术

本指南里 Kerberoasting、中继、ADCS、ACL 滥用、横向移动、隧道各有一篇。缺的是**按什么顺序做**，以及**下一个小时该花在哪条路上**的判断。

内网项目的形状几乎总是一样的：

**立足点 → 稳住 → 搞清楚自己在哪 → 收割这里已有的东西 → 提权 → 画出域的图 → 选一条路 → 到达目标 → 停手。**

两种失败最常见：跳过"搞清楚"（于是到处乱转），以及从不收手（把一次干净的项目变成一团噪音）。

### 第一阶段 —— 稳住立足点

在做别的之前，先回答关于你已有访问权的三个问题：

- **我是什么？** `whoami /priv`、`whoami /groups`、`net user %USERNAME% /domain`，以及这个会话是交互式、服务型还是网络登录。
- **我在哪？** 是否加域、哪个网段、从这里能到达什么、有没有出网通道（见隧道那篇）。
- **它稳吗？** 一个来自"重启打补丁就没了"的漏洞的 shell，价值低于一个带口令的服务账号。

立足点不稳是项目卡住最常见的原因。把它稳住 —— 建计划任务、建服务、放 SSH key，或者至少把进来的那组凭据记下来 —— 比急着提权更值。

### 第二阶段 —— 摸清环境

```powershell
ipconfig /all
route print
net view /domain
nltest /dclist:corp.local
net group "Domain Admins" /domain
```

```bash
# 有了凭据之后，从 Linux 侧
nxc smb 10.10.10.0/24 -u user -p pass --gen-relay-list unsigned.txt
nxc ldap dc01.corp.local -u user -p pass --users --groups
```

把发现写下来。一个持续记录网段、凭据、主机和访问权限的项目，效率远高于每小时重新发现一遍的项目。

### 第三阶段 —— 收割手边就有的东西

你控制的那台主机上已有的凭据，是最便宜的提权来源：

| 来源 | 能得到什么 |
|---|---|
| LSASS 内存 | 明文口令、NTLM 哈希、Kerberos 票据（需要管理员或特定权限） |
| SAM 与 LSA secrets | 本地账号、服务账号口令、缓存的域凭据 |
| `cmdkey /list` | 保存的 RDP 与网络凭据 |
| 无人值守安装文件（`Unattend.xml`、`sysprep.inf`） | 基础镜像口令，常常到处复用 |
| 组策略首选项（`Groups.xml`） | `cpassword`，用公开的密钥就能解 |
| 配置文件、脚本、CI 文件 | 应用凭据，有时还有域凭据 |
| 注册表、浏览器存储、Wi-Fi 配置 | 同类东西 |
| AD 用户的描述与备注字段 | 管理员留下的口令，偶尔真有 |

```bash
# 一次把"存在什么"审计完
nxc smb 10.10.10.0/24 -u user -p pass --sam --lsa
```

### 第四阶段 —— 先在主机上提权，再到域里

本地提权另有专篇。Windows 上值得知道的几个特权是 `SeImpersonatePrivilege`（服务账号，通往大多数 potato 系列）、`SeDebugPrivilege`（读写 LSASS）、`SeBackupPrivilege`（读任何东西，包括注册表 hive），以及"安装时总是提权"这类配置错误。

一旦在某处是本地管理员，问题就变成域级别的了，而答案是一张图：

```bash
bloodhound-python -u user -p pass -d corp.local -ns 10.10.10.10 -c All
```

在 BloodHound 里，要紧的是那些能**缩短路径**的问题：

- 从我已控主体到 Domain Admins 的最短路径。
- 哪些用户在哪些主机上有会话（找"值得拿的机器"最快的方式）。
- 谁能写谁（ACL 那篇讲具体滥用）。
- 可 Kerberoast 且通往特权的账号。
- 允许提权的证书模板（ADCS 那篇）。
- 到其他域的信任，以及是否做了 SID 过滤。

### 第五阶段 —— 有意识地选路

每条可行路线都有三种成本，选得好就是这门手艺的大部分：

| 成本 | 要考虑什么 |
|---|---|
| 需要的权限 | 你已经有了，还是这条路还要先走一步？ |
| 可检测性 | 它会不会产生服务安装、新账号、证书请求，或者一波 Kerberos 流量？ |
| 时间 | 这是五分钟的动作，还是好几天的操作？ |

多数环境里一个实用的排序，从便宜到贵：

1. **复用手上的凭据** —— 一个本地管理员口令在别处也管用，因为没部署 LAPS。
2. **读已经暴露的东西** —— 共享、GPP、描述字段、SYSVOL。
3. **Kerberoast 并破解** —— 避开大量 RC4 请求就很安静，回报常常是一个在别处是本地管理员的服务账号。
4. **ACL 滥用** —— 距离 Domain Admins 两跳的一条写权限，是干净、低噪音的路。
5. **ADCS** —— 经常是一步到域管，也经常没人注意。
6. **中继与强制认证** —— 威力大，但吵，且依赖签名配置。
7. **打未修补的漏洞** —— 最吵、最不可预测，而且通常没必要。

最后一条值得强调：在多数真实环境里，未修补的漏洞并不是最便宜的路线。人们伸手去够它，只是因为它最让人记得住。

### 第六阶段 —— 到达目标，然后停手

"域管"通常是手段，不是目标。目标是客户写在范围里的东西：某个数据库、某套财务系统、对某个网络层级的访问证明。**到达它并记录清楚怎么到达的，这就是项目。** 越过它继续做 —— 导出所有哈希、建立没人要求的持久化、为了好玩继续横向 —— 就是一个好结果变成"关于你的事故报告"的方式。

收尾前把"留下什么"（通常什么都不留）和"清理什么"跟客户确认清楚。

### 检测

- **BloodHound 是双刃剑**：自己跑一遍，把它找到的每一条通往 Tier 0 的路径当工单处理。攻击者一小时找到的东西，本来是你该先找到的。
- **会话搜索**：哪些特权账号登录在哪台工作站上。那是攻击者会跟着走的图，而它本身往往就是一个发现。
- **账号使用异常**：服务账号交互式登录、用户从没用过的网段认证、计算机账号在不该认证的地方认证。
- **蜜罐凭据**：描述字段里放一个凭据、留一个带 SPN 的闲置服务账号、一个名字很有诱惑力的共享。成本极低，却能把安静的枚举变成告警。
- **分层管理**是唯一的结构性修复。上面全是检测，分层才是预防。

### 缓解

- **建立并强制执行分层管理模型**：Tier 0 账号只登 Tier 0 系统、工作站单独管理、本地管理员口令各不相同（部署 LAPS）。
- **清掉容易被收割的东西**：GPP 和描述字段里不放凭据、不留无人值守安装文件、不必要的地方不给服务账号 `SeImpersonate`。
- **减少图的边**：每一个 `GenericAll`、每一个无约束委派、特权账号上的每一个 SPN，都是一条路径。用攻击者用的那套工具去审计它们。
- **监控身份，而不只是端点**：异常登录、新的组成员、证书请求、复制流量，在域自己的日志里全都看得见。
- **假设已经有落脚点并演练**：一次从"攻击者已经拿到一台工作站"开始的演练，比再做一次漏洞扫描有价值。
