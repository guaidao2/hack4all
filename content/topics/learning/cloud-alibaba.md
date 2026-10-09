---
id: cloud-alibaba
title_en: Alibaba Cloud Service Map and Identity Model
title_zh: 阿里云服务地图与身份模型
summary_en: A platform where both the policy language and the network rules can refuse, which makes "narrow a wide grant by subtracting from it" a writable intent rather than a structural problem. Measured by evaluating a denial inside a broad grant, reading resource names of a different shape, and comparing two signing styles that cover different sets of parameters.
summary_zh: 一个策略语言与网络规则**都能拒绝**的平台，这就让"用一个宽授权覆盖大部分、再用一条拒绝把少数摘出去"成为一种写得出来的意图，而不是一个结构上的难题。这一篇把宽授权里的一条拒绝求值出来、把形状不同的资源名拆开看，并对比两种覆盖集合不同的签名风格。
tags: [beginner, cloud, alibaba, ram, oss]
tools: [aliyun, ossutil, python3]
attck: [T1078.004, T1098.001]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A model that can refuse

In the entries on the other large platforms, one property kept appearing: **a permission model built without denial can only be narrowed by removing things.** This platform has the other shape, in two places.

> **The policy language has a denial effect, and the network rules have a drop. So "allow broadly, except for this" is an intent that can be written down here rather than one that has to be expressed by enumerating everything that is permitted.**

That single difference changes what a review looks for, and it is measured below along with the resource-name shape and two signing styles.

### Part 1: the map

| Area | The services that define it |
|---|---|
| identity | the account, its users and roles, policies, and temporary credentials |
| compute | virtual machines, container services, functions |
| storage | object storage, block storage, file storage |
| network | virtual networks, security groups, load balancing, DNS |
| data | managed relational, key-value, warehouse, streaming |
| operations | action trail, resource logs, monitoring, deployment |
| security | key management, certificate management, security centre |

**And the account is the boundary**, with resource groups available as a way of organising what is inside it. **Which is a distinction worth holding on to**: a resource group is an organising and billing construct, **and the permissions still live in policies attached to identities and resources rather than to the group** — unlike a hierarchy where the level itself is a place a grant can be attached.

**So the review question here is not "what is granted at this level" but "which policies name this resource"**, which is a different search and finds different things.

### Part 2: the policy language can deny

Measured, a policy that allows everything and then subtracts one bucket:

```
Allow  *          on  *
Deny   oss:*      on  acs:oss:*:*:prod-secrets/*
```

| Request | Outcome |
|---|---|
| read an object in the named bucket | **refused — the explicit denial wins** |
| read an object in another bucket | allowed |
| an operation on a different service | allowed |
| read an object in a bucket whose name differs only in case | **allowed — the name did not match** |

**The first and last rows together say two things.** **An explicit denial beats a permission**, which is the evaluation order a model with both effects has to define. **And the resource name is matched as a string**, so a denial written with different capitalisation denies nothing while looking like it does.

> **"Cover most of it with a broad grant and subtract the few things that matter" is writable here. In an allow-only model the same intent has to be expressed by listing everything that is permitted.**

**And the cost is that evaluation order becomes part of the meaning.** Whether a policy achieves its purpose depends on whether some other denial matches more widely, **so the useful check on a denial is not that it exists but that it matches the exact resource the author had in mind** — which is where the measured case difference lives.

**And a denial can carry a condition.** Measured, allowing everything and denying by source address:

| Source | Outcome |
|---|---|
| inside the two named ranges | **refused** |
| outside them | allowed |

**Which is the shape of a boundary that an allow-only model cannot express at all** without enumerating the complement — **and in practice, enumerating a complement is where the mistakes in that model come from.**

### Part 3: resource names of a different shape

Measured, the fields of several resource names:

| Resource name | Service | Region | Account |
|---|---|---|---|
| `acs:oss:*:1234567890:mybucket` | oss | **`*` — any** | `1234567890` |
| `acs:oss:*:1234567890:mybucket/path/file.txt` | oss | `*` | `1234567890` |
| `acs:ecs:cn-hangzhou:1234567890:instance/i-abc123` | ecs | `cn-hangzhou` | `1234567890` |
| `acs:ram::1234567890:user/alice` | ram | empty — global | `1234567890` |

**The shape is service, region, account, resource**, and the prefix differs from the other platforms.

**And the storage case is the one that catches people out.** Here an object storage name carries an account but no region, **where the equivalent name on another platform carries neither** — so a policy written by analogy to that other platform either names an account that is not there or omits one that is.

> **The fields of a resource name are not portable between platforms. A name copied from one provider's habit does not match here.**

**And a wildcard in the region field is normal for global services**, which is why the region position being `*` rather than empty is not an error — **it is how "not limited to a region" is written in this shape.**

### Part 4: two signing styles that cover different sets

This platform has two request-signing styles, and the difference between them decides whether a signed request can have anything appended to it.

**The first covers every parameter except the signature itself.** Measured:

| Change to the request | Signature |
|---|---|
| unchanged | valid |
| **an extra parameter added** | **invalid** |
| one of the covered parameters changed | invalid |

**The second covers a defined set of parameters**, which is the style the object storage service uses. Measured:

| Change to the request | Signature |
|---|---|
| unchanged | valid |
| **an extra parameter outside the set** | **still valid** |
| **an arbitrary parameter added** | **still valid** |
| one of the covered parameters changed | invalid |

> **Whether a signed request can have parameters added to it depends on which style signed it. One covers everything, so any addition breaks it; the other covers a named set, so anything outside that set was never signed and still reaches the service.**

**And both styles often exist in the same platform and even in the same client library**, which means the answer is not a property of the provider but of the call being made. **The question to ask of any signed request is the one the other entries on signatures reached**: which parameters are inside the signature, and what does the service do with the ones outside.

**And both styles sort the parameters before signing**, which is why the order they appear in the request does not matter — **and why a signature is not a hash of the request**, since two different byte strings produce the same valid signature.

### Part 5: the network rules can drop

Measured, security group rules evaluated by priority:

| Priority | Action | Source | Port |
|---|---|---|---|
| **1** | **drop** | one address range | 22 |
| 100 | accept | everything | 22 |
| 100 | accept | everything | 443 |

| Caller | Port | Outcome |
|---|---|---|
| inside the dropped range | 22 | **dropped — the lower-priority-numbered rule matched first** |
| outside it | 22 | accepted |
| outside it | 443 | accepted |

> **"Accept everybody except this range" is one rule here. In an allow-only model the same intent means deleting the broad rule and writing out everything that should remain.**

**And the priority number is the semantic**, exactly as the order of rules is in a stateless packet filter — **so the check on a drop rule is the same as the check on a denial in a policy**: does it match the thing the author had in mind, and is there nothing above it that matches more widely.

**And that the platform provides a refusal in both places is the useful generalisation.** A model that can refine a broad grant is one where the review looks for **the denials that fail to match** as much as for the grants that are too wide.

### Part 6: instance roles, and the account's own key pair

**A workload on a virtual machine can be given a role**, and the credentials are obtained from an address that only the instance can reach — **the address belonging to this platform rather than the one belonging to another**, which is a concrete detail worth knowing because a rule or a signature written against the wrong address will simply not match.

**And the permission it carries is the role's**, so the whole of the analysis from the entry on instance metadata applies: reaching that address from inside the instance means obtaining its identity.

**And the account's own access key pair is a different kind of credential.** It belongs to the account rather than to a user, **and it is not constrained by the policies that constrain users** — which puts it in the same category as a root credential, with the same handling: it should not be used, it should not exist in automation, and its existence should be a finding rather than a configuration.

### Part 7: surfaces worth knowing

**Object storage has two granting mechanisms** — a bucket-level access control and the policy language — **and both can make a bucket public**, which is the same two-mechanism shape measured on the other storage services.

**Temporary credentials are obtainable by assuming a role**, with a trust relationship that decides who may — **and the shorthand that means an entire account appears here too**, so the check from the identity entry applies.

**Two planes of audit exist**, an account-level action trail and per-resource logs, with different defaults — the measurement from the logging entry in this platform's form.

**And a resource group is not a permission boundary**, which is the point from Part 1 worth repeating because it is a natural mistake: moving a resource between groups changes how it is organised and billed, **and changes no policy**, because policies name resources rather than groups.

### Part 8: what follows for security

**Use the denial effect where a prohibition is meant.** Measured, a broad grant with one denial subtracted a bucket from it, and a denial with a source condition expressed a boundary that an allow-only model cannot express without listing the complement. **So "must not" belongs in a denial here, and a review should look for the absences** — a denial that stopped matching, or one whose resource name was written differently.

**And check a denial against the resource it was meant to cover.** Measured, a name that differed only in capitalisation matched nothing while looking like a control. **Which makes the denials a list to verify against real requests rather than to read.**

**Do not carry resource-name habits between platforms.** Measured, the storage name here carries an account and no region where the equivalent elsewhere carries neither. **A policy copied by analogy names the wrong fields**, and the failure is a silence rather than an error.

**And know which signing style signed a request.** Measured, one style was broken by any addition and the other was not. **So the question for anything signed is which parameters are inside**, and the answer differs per service rather than per platform.

**And the network rules can express the same subtraction as the policies.** Measured, a drop at a lower priority number beat an accept at a higher one. **So the two mechanisms are read the same way**: find the refusals, and check that they match what they were meant to.

**And keep the account's own key pair out of everything.** It is not a user's credential and not governed by the policies that govern users, **so its presence in a repository, an image or a pipeline is a finding of the same severity as a root credential anywhere else.**

### Detection and mitigation

- **Alert on changes to denial statements**, since a denial is where a prohibition is expressed here and editing one changes what the whole model permits.
- **Verify each denial against the resource it names**, including capitalisation and wildcards, because the measured mismatch produced a silence rather than an error.
- **Report broad grants whose only refinement is a denial**, and check that the denial still matches.
- **Report drop rules in security groups and check their priority against the accept rules below them**, since the measured evaluation is by priority number.
- **Find the account's own access key pair and treat it as a root credential**, since it is not constrained by the policies that constrain users.
- **Report long-lived access key pairs on users**, and prefer roles with temporary credentials.
- **Check which address a workload obtains credentials from**, since it differs per platform and a rule written against another platform's address does not match.
- **Review the two granting mechanisms for object storage together**, since both can make a bucket public.
- **For mitigation, express prohibitions as denials**, and rely on the evaluation order rather than on removing every broad grant.
- **Do not treat a resource group as a permission boundary**, since policies name resources and moving one changes nothing about who can reach it.
- **For anything signed, establish which parameters are inside the signature**, because the measured styles differ in whether an addition invalidates it.
- **And prefer a role the platform issues to a key pair**, so that the credential has a lifetime the platform enforces rather than one a person remembers to rotate.

<!-- lang:zh -->
### 一个能拒绝的模型

在讲其他几个大平台的篇目里，有一条性质反复出现：**一个没有拒绝概念的权限模型，只能靠移除来收窄。** 这家是另一种形状，而且是两处。

> **策略语言里有拒绝效果，网络规则里有丢弃。所以"宽泛地允许，除了这一部分"在这里是一个写得下来的意图，而不是一个必须靠把所有被允许的东西列出来才能表达的意图。**

这处差别改变了评审要找什么，下面连着资源名的形状与两种签名风格一起实测。

### 第一部分：地图

| 区域 | 定义它的那些服务 |
|---|---|
| 身份 | 账号、它下面的用户与角色、策略、以及临时凭据 |
| 计算 | 虚拟机、容器服务、函数 |
| 存储 | 对象存储、块存储、文件存储 |
| 网络 | 虚拟网络、安全组、负载均衡、DNS |
| 数据 | 托管关系库、键值、数仓、流 |
| 运维 | 操作审计、资源日志、监控、部署 |
| 安全 | 密钥管理、证书管理、安全中心 |

**而账号是边界**，资源组则是一种组织账号内部东西的方式。**这是一个值得记住的区分**：资源组是一个组织与计费上的构造，**而权限仍然住在挂在身份与资源上的策略里、而不是挂在组上** —— 不同于那种"层本身就是可以挂授予的地方"的层级结构。

**所以这里的问题不是"这一层授予了什么"，而是"哪些策略点名了这个资源"** —— 那是另一次搜索，也会找到另一些东西。

### 第二部分：策略语言可以拒绝

实测，一条允许一切、然后减掉一个桶的策略：

```
Allow  *       on  *
Deny   oss:*   on  acs:oss:*:*:prod-secrets/*
```

| 请求 | 结果 |
|---|---|
| 读被点名那个桶里的一个对象 | **拒绝 —— 显式拒绝优先** |
| 读另一个桶里的一个对象 | 允许 |
| 对另一个服务的操作 | 允许 |
| 读一个只差大小写的桶名里的对象 | **允许 —— 那个名字没有匹配上** |

**第一行与最后一行合起来说了两件事。** **显式拒绝压过许可**，这是同时有两种效果的模型必须定义的求值顺序。**而资源名是按字符串匹配的**，所以一条用不同大小写写出来的拒绝什么都不拒绝、却看起来像一道控制。

> **"用一个宽授予覆盖大部分、再把要紧的少数减掉"在这里写得出来。而在只有允许的模型里，同样的意图必须靠列出所有被允许的东西来表达。**

**而代价是求值顺序成了含义的一部分。** 一条策略能不能达到目的，取决于有没有另一条拒绝在更宽地匹配 —— **所以对一条拒绝有用的检查不是"它存在"，而是"它匹配上了作者心里的那个确切资源"**，而实测那个大小写的例子就在那里。

**而拒绝可以带条件。** 实测，允许一切、按来源地址拒绝：

| 来源 | 结果 |
|---|---|
| 落在被点名的两个网段内 | **拒绝** |
| 在它们之外 | 允许 |

**这就是一个只有允许的模型完全表达不出来的边界形状** —— 除非把补集列出来，**而在实践中，列补集正是那个模型里错误的来源。**

### 第三部分：形状不同的资源名

实测，几个资源名的字段：

| 资源名 | 服务 | 区域 | 账号 |
|---|---|---|---|
| `acs:oss:*:1234567890:mybucket` | oss | **`*` —— 不限** | `1234567890` |
| `acs:oss:*:1234567890:mybucket/path/file.txt` | oss | `*` | `1234567890` |
| `acs:ecs:cn-hangzhou:1234567890:instance/i-abc123` | ecs | `cn-hangzhou` | `1234567890` |
| `acs:ram::1234567890:user/alice` | ram | 空 —— 全局 | `1234567890` |

**形状是 服务、区域、账号、资源**，而前缀与别家不同。

**而存储那个例子正是会让人栽的地方。** 这里的对象存储资源名带账号、不带区域，**而另一家的对应写法两者都不带** —— 所以照着那家直觉写出来的策略，要么点了一个不存在的账号、要么漏了一个存在的账号。

> **资源名的字段在不同平台之间不可搬运。一个照着某家习惯抄过来的名字在这里匹配不上。**

**而区域字段上的通配符对全局服务是常态**，这就是为什么区域位置写 `*` 而不是空、并不是一个错误 —— **那是这个形状里表达"不限区域"的方式。**

### 第四部分：两种覆盖集合不同的签名风格

这个平台有两种请求签名风格，而它们的差别决定了"一条签过名的请求能不能被追加东西"。

**第一种覆盖除签名本身之外的每一个参数。** 实测：

| 对请求的改动 | 签名 |
|---|---|
| 原样 | 有效 |
| **追加一个参数** | **失效** |
| 改一个被覆盖的参数 | 失效 |

**第二种只覆盖一个定义的参数集合**，也就是对象存储服务用的那种。实测：

| 对请求的改动 | 签名 |
|---|---|
| 原样 | 有效 |
| **追加一个集合之外的参数** | **仍然有效** |
| **追加一个随便的参数** | **仍然有效** |
| 改一个在集合里的参数 | 失效 |

> **一条签过名的请求能不能被追加参数，取决于签它的是哪一种风格。一种覆盖全部，所以任何追加都破坏它；另一种只覆盖一个点名的集合，所以那个集合之外的东西从来没被签过、却仍然会到达服务。**

**而两种风格常常同时存在于同一个平台、甚至同一个客户端库里**，这意味着答案不是服务商的性质、而是那一次调用的性质。**对任何签过名的请求该问的问题，与讲签名的其他篇目得出的那个一样**：哪些参数在签名里面，以及服务拿签名外面那些怎么办。

**而两种风格都会在签名之前把参数排序**，这就是为什么它们在请求里出现的顺序不影响结果 —— **也是为什么一次签名不是对请求的哈希**，因为两串不同的字节会产出同一个有效的签名。

### 第五部分：网络规则可以丢弃

实测，按优先级求值的安全组规则：

| 优先级 | 动作 | 来源 | 端口 |
|---|---|---|---|
| **1** | **丢弃** | 某一个地址段 | 22 |
| 100 | 接受 | 一切 | 22 |
| 100 | 接受 | 一切 | 443 |

| 调用者 | 端口 | 结果 |
|---|---|---|
| 在被丢弃的地址段内 | 22 | **被丢弃 —— 编号更小的那条先匹配** |
| 在它之外 | 22 | 接受 |
| 在它之外 | 443 | 接受 |

> **"接受所有人，除了这一个地址段"在这里是一条规则。而在只有允许的模型里，同样的意图意味着删掉那条宽规则、再把该留下的全部写出来。**

**而优先级数字就是语义**，与无状态包过滤器里规则的先后完全一样 —— **所以对一条丢弃规则的检查，与对策略里一条拒绝的检查是同一种**：它有没有匹配上作者心里的那个东西，以及它上面有没有更宽的匹配。

**而"这个平台在两处都提供了拒绝"是那句有用的概括。** 一个能对宽授予做细化的模型，是一个评审要找的东西**既包括"没匹配上目标的那些拒绝"、也包括"太宽的那些授予"**的模型。

### 第六部分：实例角色，以及账号自己的密钥对

**一台虚拟机上的工作负载可以被赋予一个角色**，而凭据是从一个只有实例够得到的地址取得的 —— **那个地址属于这个平台、而不是属于另一家的那个**；这是一个具体的细节，值得知道，因为照着别家地址写的规则或者签名根本不匹配。

**而它带着的权限就是那个角色的权限**，所以讲实例元数据那一篇的全部分析都适用：从实例内部到达那个地址，就是取得了它的身份。

**而账号自己的访问密钥对是另一类凭据。** 它属于账号、而不属于某个用户，**并且不受约束用户的那些策略约束** —— 这把它放进与根凭据同一类，处置方式也一样：它不该被使用、不该出现在自动化里，而它的存在本身就应当是一条发现、而不是一处配置。

### 第七部分：几个值得知道的面

**对象存储有两套授予机制** —— 一个桶级的访问控制与策略语言 —— **而两者都能让一个桶公开**，与在别家存储服务上实测到的"两套机制"是同一个形状。

**临时凭据可以通过扮演角色取得**，而信任关系决定谁可以 —— **而那个表示"整个账号"的简写在这里也有**，所以讲身份那一篇的检查适用。

**审计有两个平面**，一个账号级的操作审计与一份按资源的日志，默认值不同 —— 日志那一篇的实测在这个平台上的形态。

**而资源组不是一个权限边界**，这是第一部分那一点值得重复的原因，因为它是一个很自然的错误：在组之间移动一个资源会改变它怎么被组织和计费，**而不会改变任何策略**，因为策略点的是资源、不是组。

### 第八部分：从这些机制推出的安全观念

**在意图是禁令的地方用拒绝效果。** 实测，一个宽授予加一条拒绝从一个桶里把它减掉了，而一条带来源条件的拒绝表达了一个只有允许的模型在不列出补集的情况下无法表达的边界。**所以"绝不能"在这家属于一条拒绝**，而评审该找的是**那些缺失** —— 一条不再匹配的拒绝，或者一条资源名写法不同的拒绝。

**并且拿那条拒绝去核对它本该覆盖的资源。** 实测，一个只差大小写的名字什么都没匹配上，却看起来像一道控制。**这就让拒绝成为一份要拿真实请求去核对、而不是去读的清单。**

**不要把资源名的习惯在平台之间搬运。** 实测，这里的存储资源名带账号、不带区域，而别家的对应写法两者都不带。**一条照着类比抄来的策略点错了字段**，而那个失败是一次静默、不是一次报错。

**并且要知道是哪一种签名风格签的。** 实测，一种会被任何追加破坏，另一种不会。**所以对任何签过名的东西，问题都是"哪些参数在签名里面"**，而答案按服务分、而不是按平台分。

**而网络规则能表达与策略同样的减法。** 实测，编号更小的丢弃压过了编号更大的接受。**所以这两套机制用同一种方式读**：找出那些拒绝，并核对它们匹配的是不是它们本该匹配的东西。

**并且把账号自己的密钥对挡在所有东西之外。** 它不是某个用户的凭据、也不受约束用户的那些策略约束，**所以它出现在仓库、镜像或流水线里，是与任何地方的根凭据同等严重的一条发现。**

### 检测与缓解

- **对拒绝语句的改动告警**，因为拒绝是这里表达禁令的地方，改一条就改变了整个模型允许什么。
- **把每一条拒绝拿它点名的资源核对一遍**，包括大小写与通配符，因为实测那种不匹配产出的是静默而不是报错。
- **报出"唯一的细化就是一条拒绝"的宽授予**，并检查那条拒绝是否还匹配得上。
- **报出安全组里的丢弃规则，并对照它下面那些接受规则检查它的优先级**，因为实测的求值是按优先级数字来的。
- **找出账号自己的访问密钥对，并把它当作根凭据处置**，因为它不受约束用户的那些策略约束。
- **报出用户身上的长期访问密钥对**，并优先用带临时凭据的角色。
- **查工作负载是从哪个地址取得凭据的**，因为它按平台不同，而照着另一家地址写的规则不匹配。
- **把对象存储的两套授予机制放在一起审**，因为两者都能让一个桶公开。
- **缓解上，把禁令表达成拒绝**，并依靠求值顺序、而不是靠移除每一条宽授予。
- **不要把资源组当成权限边界**，因为策略点的是资源，移动一个资源不改变谁能到达它。
- **对任何签过名的东西，先弄清哪些参数在签名里面**，因为实测的两种风格在"追加会不会让它失效"上不同。
- **并且优先用平台签发的角色、而不是密钥对**，这样凭据的寿命由平台强制，而不是靠有人记得轮换。
