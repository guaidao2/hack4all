---
id: cloud-security-fundamentals
title_en: Cloud Attack Surface Fundamentals
title_zh: 云攻击面基础
summary_en: The cloud moves the perimeter into the control plane. Identity policy replaces the firewall, the API is the attack surface, and the audit log records almost everything — which cuts both ways for an attacker.
summary_zh: 云把边界搬进了控制面。身份策略取代了防火墙，API 就是攻击面，而审计日志几乎记录一切 —— 对攻击者来说，这是把双刃剑。
tags: [cloud, aws, azure, gcp, iam, metadata, enumeration]
tools: [ScoutSuite, Prowler, Pacu, cloud_enum, cloudsplaining]
attck: [T1078.004, T1530, T1552.005]
platform: [cloud]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### What is actually different about the cloud

Four properties change the shape of an attack, and everything else follows from them:

1. **The control plane is the attack surface.** Creating a user, changing a policy, reading a secret — all of it is an API call, and all of it is an attack action. There is no separate "administrative network" to hide behind.
2. **Identity is the perimeter.** A firewall rule is no longer the boundary; an IAM policy is. An attacker with a valid token is inside, regardless of geography.
3. **Metadata is a credential source.** The instance metadata service hands out temporary credentials to anything that can make an HTTP request from the instance, which is why SSRF in the cloud is so much worse than SSRF on-premise.
4. **The shared responsibility model means configuration is yours.** The provider secures the hypervisor; public buckets, permissive roles and open security groups are the customer's problem, and they are where almost every cloud incident comes from.

This entry is the map. The AWS IAM privilege escalation and Kubernetes attack path entries cover specific mechanics.

### Stage 1 — Getting in

| Route | What it looks like |
|---|---|
| Leaked credentials | Keys in a public repository, a CI log, an environment file in a container image, a screenshot |
| SSRF into metadata | An application that fetches URLs, pointed at `169.254.169.254` |
| Public storage | A bucket, blob container or snapshot that allows anonymous listing or reading |
| Compromised CI/CD | A build pipeline holding deployment credentials (see the integrity entry) |
| Phishing or token theft | Session tokens for the console or the CLI, which are just as good as keys |
| Exposed services | A database port open to the internet (see the middleware entry) |

```bash
# what is publicly reachable, without authenticating to anything
cloud_enum -k company-name
aws s3 ls s3://bucket-name --no-sign-request
```

### Stage 2 — Enumerating what the identity can do

The first question after obtaining any credential is not "what can I reach" but "what am I allowed to call". Every provider has an API for that:

```bash
# AWS: is this identity an admin, and what does its policy actually grant?
aws sts get-caller-identity
aws iam list-attached-user-policies --user-name compromised
aws iam list-user-policies --user-name compromised

# automated enumeration of the whole account
scoutsuite aws --report-dir ./scout
prowler aws -M json-ocsf
```

Tools like `cloudsplaining` read the policies and report the dangerous permissions, which is more useful than reading the JSON by hand: `iam:PassRole` combined with `lambda:CreateFunction`, `iam:CreatePolicyVersion`, `sts:AssumeRole` on a permissive trust policy, and the ability to modify a service that already has a powerful role.

Spend the enumeration on the questions that matter: which roles can I assume, which services can I write to, and which secrets can I read.

### Stage 3 — Privilege escalation and lateral movement

Escalation in the cloud is usually a policy problem rather than a software exploit. A permission that looks administrative in a spreadsheet is often enough:

- **Create or modify a policy** and attach it to yourself.
- **Pass a role** to a service you control, then make that service run your code.
- **Assume a role** whose trust policy is broader than intended.
- **Update the code** of a Lambda or a container that already runs with a powerful role.
- **Change a trust relationship**, so an identity you control can assume a privileged role.

Lateral movement between accounts follows the same idea at a larger scale: role chains, resource policies that trust an external account, and shared services like a central CI system.

### Stage 4 — Persistence, and how visible it is

The straightforward persistence methods are also the most detectable, which is a trade-off worth understanding before using them:

| Method | Why it works | How visible |
|---|---|---|
| A new access key for an existing user | Simple, durable | CloudTrail records `CreateAccessKey`; GuardDuty flags it |
| A new user with the permissions you need | Simple | Records and often alerts |
| A modified trust policy | Survives credential rotation | Requires an API call that most baselines do not expect |
| Backdoored Lambda or container image | Runs with a role, and the code is legitimate-looking | Requires a deployment event, which is often normal traffic |
| An EC2 instance with a role, in a forgotten region | Fire-and-forget | The instance creation is logged, the ongoing use is not |
| Credentials in a secret that a scheduled job reads | Long-lived and quiet | Only visible if secrets access is monitored |

The last two are the interesting ones, and they illustrate the general rule: **the persistence that survives longest is the one that looks like normal infrastructure.**

### Stage 5 — Data and impact

- **Storage**: buckets, blobs, and the snapshots behind them. A snapshot shared publicly is a full database copy.
- **Databases**: RDS and equivalents, often reachable from inside the VPC with credentials found in an application config.
- **Secrets managers**: a single read can be the whole engagement.
- **Key management**: access to the KMS key that protects the data is as good as the data.

```bash
aws s3 sync s3://target-bucket ./loot --no-sign-request
aws secretsmanager get-secret-value --secret-id prod/db
```

### The multi-cloud translation table

| Concept | AWS | Azure | GCP |
|---|---|---|---|
| Identity and policy | IAM | Entra ID plus RBAC | Cloud IAM |
| Instance metadata | `169.254.169.254` | `169.254.169.254` | `metadata.google.internal` |
| Audit log | CloudTrail | Activity Log | Cloud Audit Logs |
| Object storage | S3 | Blob Storage | Cloud Storage |
| Secrets | Secrets Manager, KMS | Key Vault | Secret Manager, KMS |
| Threat detection | GuardDuty | Defender for Cloud | Security Command Center |
| Organisation guardrails | SCP | Management Groups, Policy | Organisation Policy |

The mechanics differ, the questions do not: what can this identity do, what does it trust, what can it read, and what does it leave in the log.

### Detection

The cloud's advantage is that the control plane is logged by default — as long as it is enabled, in every region, and shipped somewhere the attacker cannot reach.

- **Turn on audit logging in all regions and all accounts**, and send it to a separate account with restricted access. An attacker with account admin will try to disable logging first.
- **Alert on the escalation primitives directly**: `CreateAccessKey`, `CreateUser`, `AttachUserPolicy`, `PutRolePolicy`, `CreatePolicyVersion`, `PassRole` followed by a service creation, and any change to a trust policy.
- **Alert on the identity anomalies**: a role used from a new IP, a user calling APIs they have never called, console logins without MFA, and API calls from a region you do not operate in.
- **Alert on storage exposure**: a bucket policy that grants public access, an account-level setting that unblocks public access, and a snapshot shared outside the organisation.
- **Use the provider's own detection** (GuardDuty, Defender, SCC), but treat it as one input: it catches known patterns, and your environment's specific escalation path is unlikely to be one of them.
- **Baseline normal API behaviour per identity**, which is what turns the huge volume of control plane logs into a signal.

### Mitigation

- **Eliminate long-lived credentials.** Roles, workload identity and OIDC federation instead of access keys, and short session durations.
- **Enforce IMDSv2** on every instance, and require a hop limit of 1, which makes the metadata service much harder to reach through an SSRF.
- **Apply least privilege with tools, not intentions.** Generate policies from observed usage (`iam:GenerateServiceLastAccessedDetails`, Access Analyzer), and use permissions boundaries so a mistaken grant cannot become full admin.
- **Block public access at the organisation level**, not per bucket, so a single mistake does not leak data.
- **Guard the escalation primitives** with SCPs: deny `iam:CreatePolicyVersion`, `iam:PassRole` to untrusted services, and trust policy changes outside a change window.
- **Monitor the log pipeline itself** — a gap in delivery, a disabled trail, or a new account that is not covered by the central trail is an incident.
- **Treat the root account and the CI system as Tier 0**, with hardware MFA and no daily use.

<!-- lang:zh -->
### 云究竟哪里不一样

有四个性质改变了攻击的形状，其他一切都由此推出：

1. **控制面就是攻击面。** 建用户、改策略、读密钥 —— 全都是 API 调用，全都是攻击动作。没有一层独立的"管理网"可以躲。
2. **身份就是边界。** 防火墙规则不再是边界，IAM 策略才是。持有有效 token 的攻击者就在里面，与地理位置无关。
3. **元数据是凭据来源。** 实例元数据服务会把临时凭据交给任何能从该实例发起 HTTP 请求的东西 —— 这就是云上的 SSRF 比本地 SSRF 严重得多的原因。
4. **共享责任模型意味着配置是你的问题。** 厂商负责虚拟机监控器；公开的存储桶、过宽的角色、开放的安全组是客户的事，而几乎每一朵云上的事故都来自这里。

这一篇是地图。AWS IAM 提权与 Kubernetes 攻击路径那两篇讲具体机制。

### 阶段一 —— 进来

| 路径 | 它长什么样 |
|---|---|
| 泄漏的凭据 | 公开仓库里的密钥、CI 日志、容器镜像里的环境文件、一张截图 |
| SSRF 打元数据 | 一个会取 URL 的应用，被指向 `169.254.169.254` |
| 公开存储 | 允许匿名列举或读取的桶、Blob 容器或快照 |
| CI/CD 被拿下 | 持有部署凭据的构建流水线（见完整性那篇） |
| 钓鱼或 token 窃取 | 控制台或 CLI 的会话 token，效用等同于密钥 |
| 暴露的服务 | 对公网开放的数据库端口（见中间件那篇） |

```bash
# 在完全不认证任何东西的情况下，看什么是对公网可达的
cloud_enum -k company-name
aws s3 ls s3://bucket-name --no-sign-request
```

### 阶段二 —— 枚举这个身份能做什么

拿到任何凭据后的第一个问题不是"我能到达什么"，而是"**我被允许调用什么**"。每家云都有对应的 API：

```bash
# AWS：这个身份是不是管理员，它的策略究竟授了什么权？
aws sts get-caller-identity
aws iam list-attached-user-policies --user-name compromised
aws iam list-user-policies --user-name compromised

# 对整个账号做自动化枚举
scoutsuite aws --report-dir ./scout
prowler aws -M json-ocsf
```

`cloudsplaining` 这类工具会读策略并报出危险权限，比手工读 JSON 有用得多：`iam:PassRole` 配上 `lambda:CreateFunction`、`iam:CreatePolicyVersion`、在宽松信任策略上的 `sts:AssumeRole`，以及"能改一个本来就持有强角色的服务"。

把枚举花在真正要紧的问题上：我能承担哪些角色、我能写哪些服务、我能读哪些密钥。

### 阶段三 —— 提权与横向移动

云上的提权通常是**策略问题**，而不是软件漏洞。一条在表格里看起来像"管理类"的权限，往往就够了：

- **创建或修改策略**，然后挂到自己身上。
- **传递角色（PassRole）**给你控制的服务，再让那个服务跑你的代码。
- **承担一个角色**，而它的信任策略比预期宽。
- **更新代码**，改动一个本来就以强角色运行的 Lambda 或容器。
- **改信任关系**，让你控制的身份能承担一个特权角色。

账号之间的横向移动是同一思路的放大版：角色链、信任外部账号的资源策略，以及像中央 CI 系统这样的共享服务。

### 阶段四 —— 持久化，以及它有多显眼

最直白的持久化方法往往也最容易被检测到，这个取舍值得在使用前想清楚：

| 方法 | 为什么有效 | 有多显眼 |
|---|---|---|
| 给已有用户新建 access key | 简单、耐久 | CloudTrail 记录 `CreateAccessKey`；GuardDuty 会告警 |
| 新建一个具备所需权限的用户 | 简单 | 有记录，而且常常有告警 |
| 修改信任策略 | 能扛过凭据轮换 | 需要一次 API 调用，而大多数基线并不预期它 |
| 给 Lambda 或容器镜像开后门 | 以角色身份运行，代码看起来合法 | 需要一次部署事件，而部署常被当成正常流量 |
| 在某个被遗忘的区域起一台带角色的 EC2 | 发射后不管 | 创建有记录，后续使用没有 |
| 把凭据放进定时任务会读的密钥里 | 长期且安静 | 只有监控密钥访问时才看得见 |

最后两条才是有意思的，它们说明了一条通用规律：**活最久的持久化，是那些看起来像正常基础设施的持久化。**

### 阶段五 —— 数据与影响

- **存储**：桶、Blob，以及它们背后的快照。一个公开共享的快照就是一份完整的数据库拷贝。
- **数据库**：RDS 及同类，常常能从 VPC 内部、用应用配置里的凭据访问到。
- **密钥管理**：一次读取就可能是整个项目的全部成果。
- **KMS**：能拿到保护数据的密钥，就等于拿到了数据。

```bash
aws s3 sync s3://target-bucket ./loot --no-sign-request
aws secretsmanager get-secret-value --secret-id prod/db
```

### 多云对照表

| 概念 | AWS | Azure | GCP |
|---|---|---|---|
| 身份与策略 | IAM | Entra ID + RBAC | Cloud IAM |
| 实例元数据 | `169.254.169.254` | `169.254.169.254` | `metadata.google.internal` |
| 审计日志 | CloudTrail | Activity Log | Cloud Audit Logs |
| 对象存储 | S3 | Blob Storage | Cloud Storage |
| 密钥 | Secrets Manager、KMS | Key Vault | Secret Manager、KMS |
| 威胁检测 | GuardDuty | Defender for Cloud | Security Command Center |
| 组织级护栏 | SCP | Management Groups、Policy | Organisation Policy |

机制不同，问题相同：这个身份能做什么、它信任谁、它能读什么、以及它在日志里留下什么。

### 检测

云的优势是控制面默认就有日志 —— 前提是它被开启了、覆盖所有区域、并且被送到了攻击者够不着的地方。

- **在所有区域、所有账号开启审计日志**，并送到一个访问受限的独立账号。拿到账号管理员的攻击者，第一件事就是试图关掉日志。
- **直接对提权原语告警**：`CreateAccessKey`、`CreateUser`、`AttachUserPolicy`、`PutRolePolicy`、`CreatePolicyVersion`、`PassRole` 之后紧跟一次服务创建，以及任何对信任策略的修改。
- **对身份异常告警**：某个角色从新 IP 被使用、某个用户在调用他从没调用过的 API、没有 MFA 的控制台登录、以及来自你并不运营的区域的 API 调用。
- **对存储暴露告警**：授予公开访问的桶策略、解除了公开访问封锁的账号级设置、以及共享到组织之外的快照。
- **用厂商自带的检测**（GuardDuty、Defender、SCC），但把它当成一路输入：它抓的是已知模式，而你这个环境特有的提权路径不太可能是其中之一。
- **给每个身份的正常 API 行为建基线**，这是把海量控制面日志变成信号的唯一办法。

### 缓解

- **消除长期凭据。** 用角色、工作负载身份和 OIDC 联邦代替 access key，并把会话时长压短。
- **在每个实例上强制 IMDSv2**，并要求 hop limit 为 1 —— 这让元数据服务难得多地被 SSRF 摸到。
- **用工具而不是用意愿来落实最小权限。** 按观测到的使用生成策略（`iam:GenerateServiceLastAccessedDetails`、Access Analyzer），并用权限边界，让一次误授不可能变成完全管理员。
- **在组织级别封锁公开访问**，而不是逐个桶设置，这样一次失误不会泄漏数据。
- **用 SCP 看住提权原语**：拒绝 `iam:CreatePolicyVersion`、拒绝对不可信服务 `iam:PassRole`、拒绝在变更窗口之外改信任策略。
- **监控日志管道本身** —— 投递出现缺口、trail 被关闭、或者有新账号不在中央 trail 覆盖范围内，这些都是事件。
- **把 root 账号和 CI 系统当作 Tier 0**：硬件 MFA，且不日常使用。
