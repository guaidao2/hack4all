---
id: cloud-fundamentals
title_en: "Cloud and Containers — The Boundary Is an Identity, Not a Machine"
title_zh: "云与容器：边界是身份，不是机器"
summary_en: An attacker in the cloud does not get a machine, they get an identity — and what that identity can do is a document that can be enumerated. Measured with a policy checker and a reachability walk where a narrow service role reached admin in two hops.
summary_zh: 在云里，进攻者拿到的不是一台机器，而是一个身份 —— 而那个身份能做什么，是一份可以被枚举出来的文档。这一篇用一个策略检查器与一次可达性遍历实测，其中一条很窄的服务角色顺着两跳到了管理员。
tags: [web, cloud, container, iam, kubernetes, cwe-269]
tools: [aws, kubectl, trivy, python3]
attck: [T1078.004, T1610, T1611]
platform: [cloud, container]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The boundary moved, and it is made of text

On a traditional host, the question after a compromise is "what can this process reach on this machine". In a cloud environment the machine has an **identity**: it can call an API, assume another role, read from a bucket it was granted. The boundary that matters is no longer the host, and the thing that defines it is a document.

> **An attacker in the cloud does not get a machine. They get an identity** — and what that identity may do is a policy, written in strings.

That change makes this class different from everything else in this guide in one specific and useful way: **permissions and resources are enumerable objects.** "How far can this identity reach" is not a guess, it is a computation — which the measurements below perform.

### Four layers

1. **The trust boundary.** The application trusts that code running on an instance has that instance's permissions and no more. In the cloud the instance **is** a principal, and its permissions are usually broader than the workload needs — because they were granted for the workload plus whatever came up during development.
2. **Data and instruction share a plane.** A policy document is **configuration data** and it is also **the executable decision about who may do what**. It is made of string patterns, so `s3:GetObject` and `s3:*` differ by four characters in text and by an entire service's worth of actions in effect.
3. **Why the usual fix fails.** Reading policies one at a time answers the wrong question: the measurement below shows a role that reaches administrator through two assumptions while its own policy is beyond reproach. And treating the container as a security boundary answers the wrong one too.
4. **The variants.** The instance metadata endpoint; a wildcard action or resource; a `NotAction` policy; a role that can assume another; a container running privileged or with the runtime socket mounted; a service account with more than it needs; a public bucket; and audit logging that was never turned on.

### Measured: what a policy says versus what it grants

Five roles, each with policies that look routine:

| Role | What the policy says | What it grants |
|---|---|---|
| `web-app-role` | `s3:GetObject` on one prefix, plus `s3:*` on `*` | **every action in the storage service** |
| `ci-deploy-role` | `ecr:*`, `iam:PassRole` | a whole registry, plus **the ability to hand any role to a service** |
| **`readonly-audit-role`** | **`NotAction: [iam:Delete*, organizations:*]`** | **everything except those two patterns** |
| `narrow-role` | `sqs:SendMessage` on one queue | genuinely that |
| `admin-role` | `*` on `*` | administrator |

Two rows carry the lesson.

**`NotAction` reads like a restriction and is a denylist.** It enumerates what is *not* permitted, which means everything else is — and the list only looks adequate until the environment grows. This is the same conclusion as every filter in this guide, at policy scale: **the set of things that should be denied is open-ended; the set of things that should be allowed is a short list somebody can write down.**

**`iam:PassRole` is rarely filed as a high-privilege permission and it is one.** It lets an identity give a role to a service that will then run with it, which is a standard step from a moderate identity to a more powerful one. A policy review that ranks permissions by how alarming their names sound will rank this below `iam:CreateUser` and be wrong about which one is exploitable.

### Measured: reachability is the question

Five roles, and a trust graph — which roles may be assumed by which:

```
web-app-role        -> ci-deploy-role
ci-deploy-role      -> narrow-role, admin-role
```

Walking the graph from every identity:

| Starting identity | Reaches | |
|---|---|---|
| `web-app-role` | **4 roles** — including `admin-role` | **two hops** |
| `ci-deploy-role` | 3 roles, including `admin-role` | |
| `readonly-audit-role` | only itself | |
| `narrow-role` | only itself | |
| `admin-role` | only itself | |

**A web application role reaches administrator in two assumptions**, and the path is mundane: the service was given permission to assume the deployment role "for automation", and the deployment role was given permission to assume the release role, which is administrator. Each of those grants was reasonable in isolation and reviewed as such.

**This is the finding the per-policy reading cannot produce.** `narrow-role`'s policy is flawless. `web-app-role`'s is defensible if you consider only what the application calls at runtime. **"Who can reach admin" is a property of the trust graph, not of any single document** — and since both the policies and the graph are machine-readable, it is a property that can be computed, diffed and alerted on rather than reviewed by eye.

### The endpoint that hands out the identity

`169.254.169.254` is the instance metadata service: an HTTP endpoint reachable from the instance that returns, among other things, **credentials for the instance's role**. It is the reason the SSRF entry ends where it does, because a server-side request forgery that can reach it turns "read an internal page" into "become the instance".

The providers differ in a way that matters:

| Provider | Endpoint | Barrier to a naive SSRF |
|---|---|---|
| **AWS (IMDSv1)** | `169.254.169.254` | **none** — a plain GET returns credentials |
| **AWS (IMDSv2)** | same | a token must be fetched first with a `PUT`, which most SSRF primitives cannot do |
| **GCP** | `metadata.google.internal` | requires a `Metadata-Flavor` header |
| **Azure** | `169.254.169.254` | requires a `Metadata: true` header |

**Three of the four require a header or a token, and that requirement is the whole defence** — because an SSRF that can only issue a plain GET cannot satisfy it. AWS's v1 needed no such thing, which is why v2 exists and why enforcing it is a one-line change with a large effect.

### Containers are a process boundary, not a sandbox

A container is a process with namespaces and a restricted capability set. What it can do depends on what the kernel was configured to give it, and several common configurations give it enough to reach the host:

| Configuration | What it means |
|---|---|
| Running as root | kernel capabilities are present; the starting point is "you already have some privilege" |
| `privileged: true` | effectively host root — devices, mounts, all capabilities |
| **Mounting the container runtime's socket** | **host root** — the socket can start a container that mounts the host filesystem |
| `hostPath` mounts (Kubernetes) | read and write host paths |
| `hostNetwork` / `hostPID` | the host's network and process table |
| `--cap-add=SYS_ADMIN` | enough capabilities combined are sufficient to escape |
| The contents of the base image | everything in the image is in the runtime filesystem, **including credentials left by the build** |

**The last row is the one developers forget**: a build that copies a token, clones a private repository or installs from an authenticated registry leaves artifacts in the layer, and the runtime filesystem contains them. Multi-stage builds exist partly for this — the final image can omit the stage that had the credentials.

### Kubernetes: what a compromised pod is worth

The same question as for the instance, and the answer is one object:

> **What can this pod's service account do?**

| Its service account can | Which means |
|---|---|
| Mount a token (the default) | the API server is reachable from inside the pod |
| List pods | the shape of the cluster, and other workloads' names |
| **Create pods** | **arbitrary images run on the cluster — usually straight to the host** |
| Read secrets | the credentials it is permitted to read, and RBAC mistakes here are common |
| The cluster's secrets at rest | by default secrets are not encrypted in etcd, so etcd access is secret access |

**`create pods` is the escalation** to look for first, because a pod spec can mount host paths, run a privileged container, or pin to a node — the permissions on this row are worth more than the others combined.

### Detection and mitigation

- **Diff policies and trust relationships, and alert on wildcards appearing in either.** A new `*` in an action or a resource is a change in what an identity can reach, and both documents are machine-readable, so this is a comparison rather than a review.
- **Compute reachability rather than reading policies.** The measured walk is the technique: build the graph from the trust relationships, and answer "which identities can reach which" as a property of the graph. This is what turns a class that is normally audited annually into something checkable per change.
- **Alert on calls to the instance metadata service from a workload.** An application's normal traffic does not include its own credential endpoint; something asking for it is either a misconfiguration or the last step of an SSRF chain.
- **Alert on public buckets, and on audit logging being off.** The first is the information disclosure entry with a cloud cause, and the second is what makes every other detection on this page possible.
- **For mitigation, write policies with specific actions and specific resources.** `s3:GetObject` on a named prefix, not `s3:*` on `*` — the same rule as every allowlist in this guide, in a document where it is unusually enforceable because the platform evaluates it for you.
- **Do not use `NotAction`.** It is a denylist in the one place where the complete list is knowable, which makes using one a choice rather than a constraint.
- **Give each workload its own identity with only what it needs, and enforce the metadata service's token requirement.** Per-workload roles limit the blast radius of any single compromise, and the token requirement removes the SSRF path to the instance credentials.
- **Do not run containers as root, do not mount the runtime socket, and do not add capabilities that are not needed.** Each of those three is individually sufficient to reach the host, and none of them is required by an ordinary application.
- **Disable automatic service account token mounting, and grant the service account only the verbs it uses.** Creating pods and reading secrets are the two permissions to examine first.
- **Keep credentials out of image layers, and encrypt secrets at rest.** Multi-stage builds for the first, a key management service for the second.
- **And keep the reframing, because it is what makes this list coherent.** Every mitigation here is a statement about **which resources an identity can reach** — and in the cloud that is a property that can be enumerated, computed and diffed. That is a rare thing in this guide: a class whose central question is answerable by a program.

<!-- lang:zh -->
### 边界转移了，而它是用文本做成的

在一台传统主机上，被攻破之后的问题是"这个进程能碰到这台机器上的什么"。在云环境里，那台机器有一个**身份**：它能调 API、能假设另一个角色、能读一个它被授予的存储桶。要紧的边界不再是主机，而定义那个边界的东西是一份文档。

> **在云里，进攻者拿到的不是一台机器，而是一个身份** —— 而那个身份可以做什么，是一份用字符串写成的策略。

那个变化让这一类在这份指南里有一处具体而不同的地方：**权限与资源都是可枚举的对象。** "这个身份能走多远"不是猜，是一次计算 —— 下面的实测就在做这件事。

### 四层

1. **信任边界。** 应用信任"跑在一台实例上的代码，拥有那台实例的权限、不多不少"。在云里那台实例**就是**一个主体，而它的权限通常比那个工作负载需要的更宽 —— 因为它们是按"工作负载加上开发期间冒出来的那些东西"授予的。
2. **数据与指令共用同一平面。** 一份策略文档**是配置数据**，同时它也是**"谁可以做什么"那个可执行的判定**。它由字符串模式组成，于是 `s3:GetObject` 与 `s3:*` 在文本上差四个字符，在效果上差整个服务的全部动作。
3. **为什么常见修法失败。** 逐条读策略回答的是错的问题：下面的实测显示一个角色在自身策略无可指摘的情况下，顺着两次假设到达了管理员。而把容器当成安全边界，回答的也是错的那个。
4. **变体。** 实例元数据端点；通配的动作或资源；`NotAction` 策略；一个能假设另一个角色的角色；以特权运行或挂载了运行时套接字的容器；权限超出所需的 Service Account；公开的存储桶；以及从来没打开过的审计日志。

### 实测：一份策略说的与它授予的

五个角色，每一个的策略看起来都很常规：

| 角色 | 策略里写的 | 实际授予的 |
|---|---|---|
| `web-app-role` | 某个前缀上的 `s3:GetObject`，加上 `*` 上的 `s3:*` | **存储服务的全部动作** |
| `ci-deploy-role` | `ecr:*`、`iam:PassRole` | 整个镜像仓库，加上**把任意角色交给一个服务的能力** |
| **`readonly-audit-role`** | **`NotAction: [iam:Delete*, organizations:*]`** | **除了那两个模式之外的一切** |
| `narrow-role` | 某一个队列上的 `sqs:SendMessage` | 真的就是这一条 |
| `admin-role` | `*` 与 `*` | 管理员 |

有两行承载着那个教训。

**`NotAction` 读起来像一个限制，而它是一份黑名单。** 它枚举的是*不允许*什么，那意味着其余全部允许 —— 而那份清单只有在环境变大之前看起来还够用。这和这份指南里每一个过滤器得到的结论相同，只是发生在策略这个尺度上：**应该被拒绝的集合是开放的；应该被允许的集合是一份有人能写下来的短清单。**

**`iam:PassRole` 很少被归为高权限，而它就是。** 它让一个身份把一个角色交给一个将以那个角色运行的服务，那是从"中等身份"到"更强身份"的一条标准路径。一次按"名字听起来多吓人"给权限排名的策略评审，会把它排在 `iam:CreateUser` 下面，并在"哪一个可被利用"这件事上判错。

### 实测：该问的是可达性

五个角色，加一张信任图 —— 谁可以假设谁：

```
web-app-role        -> ci-deploy-role
ci-deploy-role      -> narrow-role, admin-role
```

从每一个身份出发遍历这张图：

| 起始身份 | 可达 | |
|---|---|---|
| `web-app-role` | **4 个角色** —— 包括 `admin-role` | **两跳** |
| `ci-deploy-role` | 3 个角色，包括 `admin-role` | |
| `readonly-audit-role` | 只有自己 | |
| `narrow-role` | 只有自己 | |
| `admin-role` | 只有自己 | |

**一个 Web 应用角色，经过两次假设到达管理员**，而那条路径平淡无奇：那个服务被给了"为了自动化"假设部署角色的权限，而部署角色被给了假设发布角色的权限，那个发布角色就是管理员。每一次授予单独看都合理，而且也是这样被评审的。

**这是逐条读策略产生不了的那条发现。** `narrow-role` 的策略无可挑剔。`web-app-role` 的也站得住脚 —— 如果你只考虑那个应用运行时调用了什么。**"谁能到达管理员"是信任图的一个性质，不是任何单独文档的性质** —— 而由于策略与图都是机器可读的，它是一个可以被计算、被 diff、被告警的性质，而不是一个靠眼睛审的性质。

### 那个把身份发出去的端点

`169.254.169.254` 是实例元数据服务：一个从实例内部可达的 HTTP 端点，它返回的东西里包括**那台实例所属角色的凭据**。这就是 SSRF 那篇结尾停在那里的原因，因为一次能到达它的服务端请求伪造，把"读一个内部页面"变成了"成为那台实例"。

各家的差别是要紧的：

| Provider | Endpoint | Barrier to a naive SSRF |
|---|---|---|
| **AWS（IMDSv1）** | `169.254.169.254` | **没有** —— 一个普通 GET 就返回凭据 |
| **AWS（IMDSv2）** | 同上 | 必须先用一个 `PUT` 取一个 token，而多数 SSRF 原语做不到 |
| **GCP** | `metadata.google.internal` | 要求一个 `Metadata-Flavor` 头 |
| **Azure** | `169.254.169.254` | 要求一个 `Metadata: true` 头 |

**四家里有三家要求一个头或一个 token，而那个要求就是全部的防御** —— 因为一次只能发出普通 GET 的 SSRF 满足不了它。AWS 的 v1 不需要这样的东西，这就是 v2 存在的理由，也是"强制它"是一行改动却效果很大的原因。

### 容器是进程边界，不是沙箱

容器是一个带有命名空间与受限能力集的进程。它能做什么，取决于内核被配置成给了它什么，而几种常见的配置给得足够到达宿主：

| 配置 | 它意味着什么 |
|---|---|
| 以 root 运行 | 内核能力还在；起点是"你已经有一点特权了" |
| `privileged: true` | 实际上等于宿主 root —— 设备、挂载、全部能力 |
| **挂载容器运行时的套接字** | **宿主 root** —— 那个套接字能起一个挂载宿主文件系统的容器 |
| `hostPath` 挂载（Kubernetes） | 读写宿主路径 |
| `hostNetwork` / `hostPID` | 宿主的网络与进程表 |
| `--cap-add=SYS_ADMIN` | 组合起来够多的能力足以逃逸 |
| 基础镜像的内容 | 镜像里的一切都在运行时的文件系统里，**包括构建留下的凭据** |

**最后一行是开发者会忘的那个**：一次把 token 复制进去、克隆了一个私有仓库、或者从需要认证的仓库安装的构建，会在层里留下痕迹，而运行时的文件系统里就有它们。多阶段构建部分正是为此 —— 最终镜像可以不含那个有凭据的阶段。

### Kubernetes：一个被攻破的 Pod 值多少

与实例同一个问题，而答案是同一个对象：

> **这个 Pod 的 Service Account 能做什么？**

| 它的 Service Account 能 | 那意味着 |
|---|---|
| 挂载 token（默认行为） | 从 Pod 内部可以到达 API server |
| 列 Pod | 集群的形状，以及其他工作负载的名字 |
| **建 Pod** | **在集群上跑任意镜像 —— 通常直接到宿主** |
| 读 Secret | 它被允许读的那些凭据，而这里的 RBAC 错误很常见 |
| 集群的静态 Secret | 默认存储在 etcd 里不加密，所以能读 etcd 就等于能读 Secret |

**"建 Pod"是首先要找的那条提权**，因为一个 Pod 规格可以挂载宿主路径、运行特权容器、或者固定到某个节点 —— 这一行的权限比其余几行加起来更值钱。

### 检测与缓解

- **对策略与信任关系做 diff，并对其中出现的通配符告警。** 动作或资源里新增的一个 `*`，就是"某个身份能到达什么"发生了变化；而两份文档都是机器可读的，所以这是一次比较，不是一次评审。
- **算可达性，而不是读策略。** 上面那次遍历就是手法：用信任关系建图，把"哪些身份能到达哪些"当成图的一个性质来回答。这正是把"一年审一次"的那一类变成"每次变更都可查"的东西。
- **对一个工作负载调用实例元数据服务告警。** 一个应用的正常流量里不包含它自己的凭据端点；去要它的东西，要么是一个错误配置，要么是一条 SSRF 链的最后一步。
- **对公开的存储桶告警，也对审计日志处于关闭状态告警。** 前者是信息泄漏那篇的一个云成因，后者是让这一页其余每一项检测成为可能的那个前提。
- **缓解上，用具体的动作与具体的资源写策略。** 在一个具名前缀上的 `s3:GetObject`，而不是 `*` 上的 `s3:*` —— 和这份指南里每一条允许清单同一条规则，只是在一份**由平台替你强制执行**的文档里。
- **不要用 `NotAction`。** 它是一份黑名单，而且是在唯一一个"完整清单可知"的地方 —— 这让使用它成为一个选择，而不是一种约束。
- **给每个工作负载自己的身份、只给需要的权限，并强制元数据服务的 token 要求。** 逐工作负载的角色限制了任何单点被攻破的波及面，而那个 token 要求移除了通往实例凭据的 SSRF 路径。
- **不要以 root 运行容器、不要挂载运行时套接字、不要添加不需要的能力。** 这三者各自都足以到达宿主，而一个普通应用三者都不需要。
- **关掉 Service Account token 的自动挂载，并只授予它用到的那些动词。** 建 Pod 与读 Secret 是最先要查的两个权限。
- **让凭据不进入镜像层，并加密静态的 Secret。** 前者用多阶段构建，后者用密钥管理服务。
- **并且记住这个重新表述，因为正是它让上面这张清单变得自洽。** 这里每一项缓解都是一个关于**"某个身份能到达哪些资源"**的陈述 —— 而在云里，那是一个可以被枚举、被计算、被 diff 的性质。在这份指南里这是罕见的一件事：一个中心问题可以由程序来回答的类别。
