---
id: cloud-azure
title_en: Azure Service Map and Identity Model
title_zh: Azure 服务地图与身份模型
summary_en: A platform with two authorization planes — what a resource looks like and what is inside it — where the roles for one do not grant anything on the other, and where a subtraction inside a role definition is widely mistaken for a denial. Measured by evaluating the two pools separately and by restoring a subtracted action with a second assignment.
summary_zh: 一个有两套授权平面的平台 —— "资源长什么样"与"资源里有什么" —— 而一个平面的角色对另一个平面什么都不授予；此外，角色定义内部的减法常被误当成拒绝。这一篇把两个池子分开求值，并用第二条分配把被减掉的动作要回来。
tags: [beginner, cloud, azure, rbac, managed-identity]
tools: [az, azure-cli, python3]
attck: [T1078.004, T1098.001]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Two planes, and a subtraction that is not a denial

This platform splits authorisation in a way most others do not, and it has one field whose name suggests a protection it does not provide.

> **Managing a resource and reading what is inside it are two separate grants, so a role that looks like it permits everything on one plane grants nothing on the other. And a subtraction inside a role definition only applies to that definition — it is a statement about the role, not about the person.**

Both are measured below.

### Part 1: the map

| Area | The services that define it |
|---|---|
| identity | role assignments, role definitions, managed identities, policy as a ceiling |
| compute | virtual machines, container platforms, functions, app hosting |
| storage | object storage, disks, files, queues |
| network | virtual networks, subnets, security rules, load balancing, DNS |
| data | managed relational, key-value, warehouse, streaming |
| operations | activity log, resource logs, monitoring, deployment |
| security | key vault, certificate management, security posture |

**And the subscription is the boundary that matters**, with management groups above it and resource groups below it. **Which makes the hierarchy a three-level question**: what is granted at the management group, what at the subscription, and what at the resource group — **and the wide grants are usually at the first two.**

**And the resource provider namespace is part of every action name**, which is worth knowing because it is what makes an action name in this platform self-describing: `Microsoft.Storage/storageAccounts/blobServices/...` says which provider, which resource type and which operation.

### Part 2: two pools that do not mix

The measurement that explains a very common confusion. A role definition has two separate lists of actions, one for the management plane and one for the data plane, and **a request is matched against only one of them.**

Measured, the same three questions against three roles:

| Role, and where it is assigned | Read the resource's configuration | **Read an object inside it** | Create a virtual machine |
|---|---|---|---|
| read-only, at the subscription | **allowed** | **refused** | refused |
| object-data-reader, at the storage account | refused | **allowed** | refused |
| contributor, at the subscription | **allowed** | **refused** | **allowed** |

**The third row is the one to read twice.** A role whose management-plane list is a wildcard **cannot read an object**, because object reads are data-plane actions and the wildcard was in the other pool.

> **"What roles does this identity have" is two questions: can it change the configuration, and can it touch the data. The answers come from different sets of roles.**

**And this is the mechanism behind two opposite mistakes.** A team grants a broad management role and finds the application cannot read its storage. **And a team grants nothing on the data plane and hands out the storage account key instead** — which is the next measurement, and which removes the authorisation question entirely.

**So a review of roles has to be a review of two lists**, and the useful output is a table like the one above per identity, because **"has rights to the storage account" and "can read the blobs in it" are different statements with different role names.**

### Part 3: the subtraction is inside one definition

A custom role can be written as everything except a few actions, and that field is regularly read as a denial. Measured:

| Assignment | Deleting the resource |
|---|---|
| one custom role: everything, **minus** the delete action | **refused** |
| the same, plus a second role that grants the delete action | **allowed — the second assignment restored it** |

**And inside the single role definition, the subtraction does what it looks like**: the delete action refused, other actions allowed.

> **The subtraction speaks about what that one role contains. It says nothing about what the identity may do, because another assignment can grant the same action back.**

**Which is why "we denied that action with a custom role" is a claim that does not hold**, and why the measured second row is the shape of the counter-example: an assignment added later, by another team, for an unrelated reason.

**And the platform does have objects that can refuse**, which are a different kind of thing from a role assignment — **so expressing "this must not happen" belongs there rather than in a role definition**, because an object that denies holds regardless of what else is assigned.

### Part 4: a key skips the whole model

**The data plane can also be reached with a credential rather than a role**, and the permission to obtain that credential is a normal management-plane action. Measured:

| Identity | Reading an object |
|---|---|
| one with **no role assignment at all** | **refused** |
| one that **may list the account's keys** | obtains a credential that reads and writes the data regardless of roles |

> **The permission to list the account's keys is not a resource permission. It is the permission to step outside the authorisation model, which is why it belongs with the credentials in review.**

**And the same is true of a shared access signature**: a signed URL that carries its own authorisation, which is the shape measured in the entries on object storage and on webhooks — **a bearer credential that works until it expires, whose exposure is wherever it appears.**

**So the data-plane boundary has to be read in two places**: the roles that grant data actions, **and every path that can produce a key or a signature.** A platform where the second path is open has a boundary made of credentials rather than of policy.

### Part 5: assignments inherit downward

Measured, one assignment at two levels:

| Where the assignment is | In one subscription | In a resource group of another |
|---|---|---|
| at the **management group** | **allowed** | **allowed** |
| at **one resource group** | **allowed** | refused |

**And it is one-directional**, so a grant in one resource group says nothing about its siblings.

**Which means the number of subscriptions is not the number of configurations.** A hundred subscriptions under a handful of management-group assignments is one configuration, **and the review that finds it is the one that reads the top of the tree** — the same conclusion the entry on the other hierarchy-based platform reached from a different structure.

### Part 6: two kinds of service identity, one without a secret

**An application registration has a credential** — a client secret or a certificate — **created by a person, rotated by a person, and downloadable into whatever place it is needed.**

**A managed identity has none.** The platform issues credentials to the resource at run time, and they live and die with it.

> **Both are expressed the same way in authorisation — a principal with role assignments — and they differ in whether there is a copyable credential. That difference is where leaks happen.**

**And "no secret" is not "no risk".** A managed identity can be granted a broad role, **so the question of what it may do is unchanged** — and the platform's own surfaces for obtaining a token from a resource are the paths that matter, which is the instance-metadata analysis from this series applied to a managed identity.

### Part 7: the surfaces worth knowing

**The activity log and the resource logs are two planes**, with different defaults and different retention — the measurement in the entry on cloud logging, arriving here as a subscription-level log and a per-resource one.

**Policy is a separate object with a refusing effect**, which is what makes it the right place to express a prohibition that must hold across assignments.

**Temporary elevation exists as a first-class feature**, which is worth knowing because it changes what "this identity has this role" means over time — **a role that is active only during an approved window is not the same as one that is always active**, and the difference is exactly the property that makes a permanent assignment a finding.

**And resources can be moved between resource groups**, which changes which assignments apply to them without any assignment being edited.

### Part 8: what follows for security

**Review the two planes separately and report them together.** Measured, a wildcard management role could not read an object, and a data-reader role could not read the configuration. **So a per-identity table with a management column and a data column is the artifact**, and a review that produces one column has answered half the question.

**Treat the key-listing permission as a credential permission.** Measured, an identity with no role could reach the data through a key. **So the data-plane boundary is the union of the data roles and every path to a key or a signature** — and the second set is usually smaller, easier to enumerate, and rarely reviewed.

**Do not express a prohibition in a role definition.** Measured, a second assignment restored a subtracted action. **So "must not" belongs in an object that refuses**, and a custom role with a subtraction is a statement about that role's scope rather than a control.

**Prefer an identity without a secret.** A managed identity removes the copyable credential, **which removes the leak path measured in the entries on keys, container images and startup scripts** — while leaving untouched the question of how wide its roles are.

**And grant at the lowest level that works**, because a measured assignment at the top of the tree covered every subscription beneath it — and the number of subscriptions in an environment is not the number of permission decisions in it.

### Detection and mitigation

- **Report management-plane roles that are broad, and separately report data-plane roles**, since the measurement showed one granting nothing on the other plane.
- **Alert on the permission to list account keys or generate shared access signatures**, because that permission steps outside the authorisation model and reaches the data.
- **Alert on the creation of a shared access signature with a long validity**, since a signed URL is a bearer credential whose lifetime is its only bound.
- **Search custom role definitions for subtractions and treat them as scope statements rather than controls**, because a second assignment can restore the action.
- **Report assignments at the management group level**, since the measured inheritance covers every subscription below.
- **Check which identities are application registrations with credentials**, and prefer managed identities where a workload can use one.
- **Review role assignments that are permanent where temporary elevation is available**, since a window is a different exposure from an always-on grant.
- **Watch for resources moved between resource groups**, because the assignments in effect change without any assignment being edited.
- **For mitigation, express prohibitions in an object that denies**, and use the highest scope available as the ceiling.
- **Prefer an identity the platform issues over one with a downloadable credential**, so that there is nothing to copy.
- **Grant at the lowest level that works**, and treat a top-level assignment as covering everything beneath it.
- **And audit the two planes with two lists.** The measured confusion is a role that looks like everything and cannot read an object, and the fix is asking both questions of every identity.

<!-- lang:zh -->
### 两个平面，以及一个不是拒绝的减法

这个平台把授权拆开的方式大多数别家没有，而它有一个字段，名字暗示了一种它并不提供的保护。

> **管理一个资源与读它里面的内容，是两次彼此独立的授予，所以一个看起来在一个平面上放行一切的角色，在另一个平面上什么都不授予。而角色定义内部的一次减法只作用于那一个定义 —— 它是一句关于这个角色的话，不是关于这个人的话。**

两者都在下面实测。

### 第一部分：地图

| 区域 | 定义它的那些服务 |
|---|---|
| 身份 | 角色分配、角色定义、托管标识、策略作为天花板 |
| 计算 | 虚拟机、容器平台、函数、应用托管 |
| 存储 | 对象存储、磁盘、文件、队列 |
| 网络 | 虚拟网络、子网、安全规则、负载均衡、DNS |
| 数据 | 托管关系库、键值、数仓、流 |
| 运维 | 活动日志、资源日志、监控、部署 |
| 安全 | 密钥保管库、证书管理、安全态势 |

**而订阅是那个要紧的边界**，它上面有管理组、下面有资源组。**这就让层级成为一个三层问题**：管理组上授予了什么、订阅上授予了什么、资源组上授予了什么 —— **而宽的授予通常在前两层。**

**而资源提供程序命名空间是每一个动作名的一部分**，这值得知道，因为它正是让这个平台里的动作名自描述的原因：`Microsoft.Storage/storageAccounts/blobServices/...` 说出了哪个提供程序、哪种资源类型、哪个操作。

### 第二部分：两个不混用的池子

这一组实测解释了一个非常常见的困惑。一个角色定义有两份彼此独立的动作清单，一份管管理面、一份管数据面，而**一个请求只会在其中一份里被匹配。**

实测，同样三个问题对着三个角色：

| 角色，以及它被分配在哪里 | 读资源的配置 | **读它里面的一个对象** | 建一台虚拟机 |
|---|---|---|---|
| 只读，在订阅上 | **允许** | **拒绝** | 拒绝 |
| 对象数据读取，在存储账号上 | 拒绝 | **允许** | 拒绝 |
| 参与者，在订阅上 | **允许** | **拒绝** | **允许** |

**第三行值得读两遍。** 一个管理面清单是通配符的角色**读不了一个对象**，因为读对象是数据面动作，而那个通配符在另一个池子里。

> **"这个身份有什么角色"是两个问题：它能改配置吗、它能碰数据吗。而答案来自不同的角色集合。**

**而这就是两个相反方向的错误背后的机制。** 一个团队给了一个很宽的管理角色，然后发现应用读不了它的存储。**而另一个团队在数据面上什么都没给，转而把存储账号密钥发了出去** —— 那是下一组实测，而它把授权这个问题整个去掉了。

**所以对角色做评审必须是对两份清单做评审**，而有的产出是每个身份一张上面那样的表，因为**"对存储账号有权限"与"能读它里面的对象"是两句不同的话，对应不同的角色名。**

### 第三部分：那个减法在一个定义内部

一个自定义角色可以被写成"除了少数几个动作之外的一切"，而那个字段经常被读成拒绝。实测：

| 分配 | 删掉那个资源 |
|---|---|
| 一个自定义角色：一切，**减去**那个删除动作 | **拒绝** |
| 同上，再加一条授予那个删除动作的角色 | **允许 —— 第二条分配把它要回来了** |

**而在那单独一个角色定义内部，那个减法的行为与看起来一致**：删除动作被拒、其他动作允许。

> **那个减法说的是"这一个角色包含什么"。它对"这个身份可以做什么"什么都没说，因为另一条分配可以把同一个动作给回来。**

**这就是为什么"我们用自定义角色把那个动作拒掉了"是一句不成立的话**，也是为什么实测第二行就是反例的形状：一条之后由另一个团队、因为不相关的原因加上的分配。

**而这个平台确实有能拒绝的对象**，那是与角色分配不同的一类东西 —— **所以表达"这件事绝不能发生"属于那里，而不是属于一个角色定义**，因为一个能拒绝的对象不管别的分配说了什么都成立。

### 第四部分：一把密钥跳过整套模型

**数据面也可以靠一个凭据而非一个角色到达**，而取得那个凭据的权限是一个普通的管理面动作。实测：

| 身份 | 读一个对象 |
|---|---|
| **完全没有角色分配的那个** | **拒绝** |
| **可以列出账号密钥的那个** | 取得一份凭据，不管角色如何都能读写数据 |

> **"可以列出账号密钥"这个权限不是一个资源权限。它是"走出授权模型"的权限，这就是为什么它该和凭据一起评审。**

**共享访问签名也一样**：一个自带授权的签名 URL，也就是讲对象存储与讲 webhook 那两篇里实测过的形状 —— **一张在过期之前一直有效的凭据，而它的暴露面就是它出现过的每一个地方。**

**所以数据面的边界要在两个地方读**：授予数据动作的那些角色，**以及每一条能产出密钥或签名的路径。** 一个第二条路径敞开的环境，它的边界是凭据做的、而不是策略做的。

### 第五部分：分配向下继承

实测，一条分配放在两个层级上：

| 分配放在哪里 | 在一个订阅里 | 在另一个订阅的资源组里 |
|---|---|---|
| 在**管理组**上 | **允许** | **允许** |
| 在**某一个资源组**上 | **允许** | 拒绝 |

**而它是单向的**，所以一个资源组里的授予对它的兄弟什么都不说明。

**这意味着订阅的数量不是配置的数量。** 少数几条管理组分配之下的一百个订阅，是一份配置，**而能找到它的那次评审是读了树的顶端的那一次** —— 与另一个以层级为核心的平台那一篇从不同结构得出的同一个结论。

### 第六部分：两种服务身份，一种没有秘密

**一个应用注册有一份凭据** —— 客户端秘密或证书 —— **由人创建、由人轮换、可以被下载到任何需要它的地方。**

**一个托管标识没有。** 平台在运行时把凭据发给那个资源，而它们随它生死。

> **两者在授权上的表达一样 —— 一个主体加若干角色分配 —— 差别在于有没有一份能被复制的凭据。而那个差别正是泄露发生的地方。**

**而"没有秘密"不等于"没有风险"。** 一个托管标识可以被授予很宽的角色，**所以"它能做什么"这个问题没有变** —— 而平台从资源上取得令牌的那些面才是要紧的路径，也就是这个系列里关于实例元数据的分析换成一个托管标识。

### 第七部分：几个值得知道的面

**活动日志与资源日志是两个平面**，默认值与保留期都不同 —— 云日志那一篇的实测在这里表现为一份订阅级的日志与一份按资源的日志。

**策略是一个带拒绝效果的独立对象**，这正是它成为"表达一条必须在所有分配之上成立的禁令"的正确位置的原因。

**临时提权是一个一等特性**，这值得知道，因为它改变了"这个身份有这个角色"随时间意味着什么 —— **一个只在被批准的窗口内生效的角色，与一个一直生效的角色不是同一件事**，而这个差别恰恰就是让一条长期分配成为发现的属性。

**而资源可以在资源组之间移动**，这会在没有任何分配被编辑的情况下改变哪些分配作用于它。

### 第八部分：从这些机制推出的安全观念

**把两个平面分开评审、放在一起报告。** 实测，一个通配符的管理角色读不了一个对象，而一个数据读取角色读不了配置。**所以产出物是每个身份一张表、带管理面一列与数据面一列**，而只产出一列的评审只回答了一半。

**把"能列出密钥"当成一个凭据权限。** 实测，一个没有任何角色的身份可以靠一把密钥到达数据。**所以数据面的边界是"数据角色"与"每一条通向密钥或签名的路"的并集** —— 而第二个集合通常更小、更容易清点、也更少被审。

**不要在一个角色定义里表达禁令。** 实测，第二条分配把一个被减掉的动作要了回来。**所以"绝不能"属于一个能拒绝的对象**，而一个带减法的自定义角色是一句关于那个角色范围的话、不是一道控制。

**优先用没有秘密的那种身份。** 托管标识去掉了那份可复制的凭据，**也就去掉了讲密钥、讲容器镜像与讲启动脚本那几篇里实测到的泄露路径** —— 同时完全不碰"它的角色有多宽"这个问题。

**并且在能干活的最低层授予**，因为实测一条在树顶端的分配覆盖了它下面的每一个订阅 —— 而一个环境里订阅的数量不是它里面权限决定的数量。

### 检测与缓解

- **报出宽的管理面角色，并单独报出数据面角色**，因为实测显示一个平面上的角色对另一个平面什么都不授予。
- **对"可以列出账号密钥或生成共享访问签名"的权限告警**，因为那个权限走出了授权模型、直接到达数据。
- **对生成有效期很长的共享访问签名告警**，因为一个签名 URL 是一张凭据，它的寿命是它唯一的边界。
- **搜自定义角色定义里的减法，并把它当成范围声明而不是控制**，因为另一条分配能把那个动作要回来。
- **报出管理组层的分配**，因为实测的继承覆盖它下面的每一个订阅。
- **查哪些身份是带凭据的应用注册**，并在工作负载能用托管标识的地方优先用它。
- **审"在可以采用临时提权的地方却用了长期分配"**，因为一个窗口与一条一直生效的授予是两种不同的暴露。
- **盯资源在资源组之间被移动**，因为生效的分配会变、而没有任何分配被编辑过。
- **缓解上，把禁令表达在一个会拒绝的对象里**，并用可用的最高作用域作为天花板。
- **优先用平台签发的身份而不是带可下载凭据的那一种**，这样根本没有东西可以被复制。
- **在能干活的最低层授予**，并把一条顶层分配当成覆盖它下面的一切。
- **并且用两份清单审两个平面。** 实测那个困惑是一个看起来什么都能、却读不了一个对象的角色，而修法是就每一个身份把两个问题都问一遍。
