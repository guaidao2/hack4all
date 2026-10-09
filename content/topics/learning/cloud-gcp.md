---
id: cloud-gcp
title_en: Google Cloud Service Map and Identity Model
title_zh: Google Cloud 服务地图与身份模型
summary_en: A provider whose resource hierarchy is the backbone of its permission model, so the answer to "where does this permission come from" is usually in an ancestor, and where a request has to pass two authorisation gates — the scopes carried by its token and the roles granted to its identity. Measured by evaluating inheritance and intersecting the two gates.
summary_zh: 这家的资源层级就是它权限模型的骨架，所以"这个权限从哪来"的答案通常在上面某一层；而它另一个特点是**一个请求要过两道授权门** —— 它的令牌带着的范围、以及授予它身份的角色。这一篇把继承求值出来、把两道门取交集来实测。
tags: [beginner, cloud, gcp, iam, service-accounts]
tools: [gcloud, gsutil, python3]
attck: [T1078.004, T1098.001]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The hierarchy is the permission model

Every provider has a resource hierarchy. This one uses it as the skeleton of the permission system, which changes what a review has to read.

> **A grant is placed at some level and inherits downward, so the answer to "where does this permission come from" is usually an ancestor of the thing you are looking at. And a request has to pass two independent gates, which is a property that is easy to miss because most platforms have one.**

Both are measured below.

### Part 1: the map

| Area | The services that define it |
|---|---|
| identity | IAM policies and service accounts; organisation policy as a ceiling |
| compute | virtual machines, containers, functions, managed runtimes |
| storage | object storage, block volumes, shared filesystems |
| network | virtual networks, subnets, firewall rules, load balancing, DNS |
| data | managed relational, key-value, warehouse, streaming |
| operations | logging, monitoring, tracing, deployment |
| security | key management, secret storage, certificate authority |

**And the project is the smallest unit that holds resources**, which makes it the equivalent of another provider's account — **with the difference that projects sit inside folders inside an organisation, and permissions flow down that chain.**

**Which is the single most useful structural fact about this platform for a security review**: an environment with a hundred projects does not have a hundred independent permission configurations, it has one tree with grants placed at various heights in it.

### Part 2: a grant inherits downward

Measured, the same role bound at two different levels:

| Where the grant is | In one project | In a sibling project |
|---|---|---|
| at the **organisation** | **allowed** | **allowed** |
| at **one project** | **allowed** | **refused** |

**And the inheritance is one-directional**: grants flow from an ancestor to its descendants, and a grant in one project says nothing about its sibling or its parent.

**And where two levels both grant, the result is the union.** Measured, a subject with a viewer role at the organisation and an administrator role in one project:

```
in that project   read, list, create, delete
in its sibling    read, list          (the organisation grant only)
```

> **Reading a binding has to start with the level it is attached to. The same role name at an organisation and at a project are two different things.**

**And the reason the hierarchy exists is administrative rather than security-related**: granting a department access to a fleet of projects should be one operation rather than a hundred. **The consequence is that one binding can cover everything below it**, which is why the levels above a project are where the widest grants live and where a review that only reads project-level policies will find nothing.

**And there is no denial rule here either**, so the same property measured in the other capability entries applies: **narrowing means removing or changing a binding, and adding a more specific one subtracts nothing.**

### Part 3: two gates, and the request passes both

This is the property that has no equivalent in most other platforms, and it is worth stating as a principle before the measurement.

**A request arrives with a token, and the token carries a set of scopes.** The identity behind the token has roles. **Both are checked**, and the effective permission is the intersection.

Measured, one identity holding an object-administrator role, with three different tokens:

| Scope carried by the token | Permissions actually usable |
|---|---|
| unrestricted | read, list, create, delete |
| read and list only | **read, list** |
| read only | **read** |

**One role, three outcomes**, and the identity's configuration was identical in all three cases.

> **A request passes two gates: the scopes its token carries and the roles its identity holds. The effective permission is the intersection, so setting roles to least privilege is only half of it — the token that is handed out carries the other half.**

**Which inverts the intuition from most platforms.** Elsewhere a wide role is the finding; here a wide role on a narrowly scoped token is harmless, **and a narrowly scoped role on a widely scoped token is also harmless** — the exposure comes from both being wide at once.

**And the scopes are usually set somewhere less reviewed than the policies**: in a client library's configuration, in a pipeline's credentials step, in a workload's startup parameters. **Which makes them a place to look for the half of the decision that the policy review did not see.**

### Part 4: a service account is both an identity and a resource

**Both halves of that sentence matter, and their combination produces the permission that matters most here.**

Measured, a subject with no read access of its own and permission to act as a service account that does have it:

| Fact | Outcome |
|---|---|
| the subject reads an object directly | **refused** |
| the subject may act as the service account | allowed |
| the service account reads the object | allowed |
| **so the subject reaches the object through the service account** | **the subject obtained a permission it was never granted** |

> **The permission to act as a service account does not grant access to a resource. It grants another identity — which puts it in the same family as the workload-creation permissions measured elsewhere.**

**And the second half of the sentence is a second path.** A service account is a resource with a policy of its own, **so whoever can modify that policy can grant themselves or anybody else the ability to act as it** — and whoever can delete or disable it can break whatever depends on it.

**And the default service account of a project is the one to look at first**, because it exists in every project, it is used by anything that does not name one, and **its roles were often assigned once when the project was created and never revisited.**

### Part 5: public is a member name

Measured, the same role granted to three kinds of member:

| Member in the binding | A caller from the internet |
|---|---|
| **`allUsers`** | **allowed** |
| `allAuthenticatedUsers` (not bound) | refused |
| a specific user (not bound) | refused |

**Public access here is expressed as a member rather than as a switch**, which matters for two reasons: **the two special member names are the only things that mean "anyone"**, and **a binding containing one looks exactly like a binding for a group** — same fields, same role, a different value in one of them.

> **Public in this platform is a value in the member field. So the search is for that value, not for a boolean.**

**And the difference between the two special names is worth stating**: one means anybody at all, and the other means anybody who has authenticated with any account at all — **which for a resource on the internet is a much smaller set than it sounds and a much larger set than the operator usually intends.**

### Part 6: conditions narrow a binding

Measured, one grant placed at the organisation level, carrying a condition:

| Resource | Outcome |
|---|---|
| inside the condition | **allowed** |
| outside it | **refused** |

**So a binding at a high level does not always mean a wide grant**, and reading the role name alone overestimates it.

> **A grant has to be read with its condition, because a condition can turn what looks like an organisation-wide grant into a project-scoped one.**

**And the reverse is the review's concern**: a binding whose condition was removed, or was written narrowly and later edited, changes the reach of the grant without the role changing at all. **Which makes conditions a field to diff rather than a field to read once.**

### Part 7: keys and tokens have different lifetimes

**An access token is short-lived** — its validity is measured in an hour — **and a service account key is a downloadable file containing a private key with no expiry of its own.**

> **"Using a service account" and "using a service account key" are two different things: the first has credentials that expire, the second has a credential that works until it is revoked.**

**And the key being a file is what makes it a different kind of risk.** It can be copied, committed, put in an image layer, or emailed — **every one of which is a place measured in the entries on keys and on container images** — while a token obtained at run time exists only in the process that asked for it.

**And a key cannot be made safer by being rotated**, because rotation is an action somebody has to remember, where the alternative is an expiry that happens without anybody's involvement.

### Part 8: what follows for security

**Start a review at the top of the tree.** Measured, a grant at the organisation covered every project beneath it while a grant at a project covered one. **So the wide grants are in the ancestors, and the question "who can reach this project" is answered by walking upward** rather than by reading the project's own policies.

**And count the two gates rather than one.** Measured, one role produced three different sets of usable permissions depending on the token's scopes. **So a review of roles is a review of half the decision**, and the other half lives in wherever tokens are minted — a pipeline, a library's configuration, a workload's parameters.

**The service-account actions are identity permissions.** Measured, the ability to act as one reached permissions the subject was never granted, and the ability to edit one's policy is a second way to the same place. **So they belong with the credentials in review, not with the resource policies.**

**Public access is a value, not a flag.** Measured, one member name made a resource reachable by anyone and looked like an ordinary group binding. **So the check is a search for that value across the whole tree**, since a binding at any level covers everything below it.

**And conditions are what make a high-level binding not automatically wide.** Measured, one condition turned an organisation-level grant into a project-scoped one. **So they are worth diffing, because removing one changes the reach without changing the role.**

**And the hierarchy is the reason a project count is not a boundary count.** A hundred projects under few grants is one configuration, and the review that finds it is the one that reads the ancestors.

### Detection and mitigation

- **Review bindings at the organisation and folder levels first**, since the measured inheritance covers everything below and a project-level review will not see them.
- **Search the whole tree for the two public member names**, because a binding anywhere covers its descendants and the syntax matches an ordinary group grant.
- **Alert on the permissions that allow acting as a service account**, which the measurement showed reaching permissions the subject never held.
- **Alert on edits to a service account's own policy**, since it is a resource as well as an identity.
- **Report service account keys by age and by existence**, and treat a key as a credential to remove rather than to rotate.
- **Check the default service account of every project**, since it exists everywhere, is used by default, and is rarely revisited.
- **Diff the conditions on bindings**, because removing one widens a grant without changing the role.
- **Audit the scopes on tokens alongside the roles**, since the measured effective permission was the intersection of both and the scopes are set somewhere less reviewed.
- **For mitigation, prefer a short-lived credential obtained at run time to a downloaded key**, so that expiry happens without anybody's involvement.
- **Place grants as low in the tree as the work allows**, and treat a grant above the project level as covering everything below it.
- **Grant the ability to act as a service account only to what needs that identity**, and review it as an identity grant rather than as a resource grant.
- **And keep the organisation policy as the ceiling**, since a rule that denies holds when the configuration underneath is wrong.

<!-- lang:zh -->
### 层级就是权限模型

每一家都有资源层级。这一家把它当成权限系统的骨架，而这改变了评审要读什么。

> **一次授予被放在某一层、并向下继承，所以"这个权限从哪来"的答案通常是你正在看的那个东西的某一层祖先。而一个请求要过两道彼此独立的门 —— 这条性质容易被漏掉，因为大多数平台只有一道。**

两者都在下面实测。

### 第一部分：地图

| 区域 | 定义它的那些服务 |
|---|---|
| 身份 | IAM 策略与服务账号；组织策略作为天花板 |
| 计算 | 虚拟机、容器、函数、托管运行时 |
| 存储 | 对象存储、块卷、共享文件系统 |
| 网络 | 虚拟网络、子网、防火墙规则、负载均衡、DNS |
| 数据 | 托管关系库、键值、数仓、流 |
| 运维 | 日志、监控、链路追踪、部署 |
| 安全 | 密钥管理、秘密存储、证书颁发 |

**而项目是持有资源的最小单位**，这让它相当于别家的账号 —— **区别在于项目嵌在文件夹里、文件夹嵌在组织里，而权限沿着这条链向下流。**

**对一个安全评审来说，这是关于这个平台最有用的一个结构性事实**：一个有一百个项目的环境并没有一百份彼此独立的权限配置，它有一棵树，而授予被放在这棵树的不同高度上。

### 第二部分：授予向下继承

实测，同一个角色绑在两个不同的层级上：

| 授予放在哪里 | 在一个项目里 | 在它的兄弟项目里 |
|---|---|---|
| 在**组织**上 | **允许** | **允许** |
| 在**某一个项目**上 | **允许** | **拒绝** |

**而继承是单向的**：授予从某一层祖先流向它的后代，而一个项目里的授予对它的兄弟或它的父层什么都不说明。

**而两层都授予时，结果是并集。** 实测，一个主体在组织上有 viewer、在某个项目上有 administrator：

```
在那个项目里    读、列举、创建、删除
在它的兄弟里     读、列举（只有组织那一条生效）
```

> **读一条绑定必须从它挂在哪一层开始。同一个角色名在组织上与在项目上是两件不同的事。**

**而层级存在的理由是管理上的、不是安全上的**：给一个部门一批项目的访问应该是一次操作、而不是一百次。**后果是一条绑定可以覆盖它下面的一切**，这就是为什么"项目之上"那几层才是最宽的授予所在，也是一份只读项目级策略的评审在那里什么也找不到的原因。

**而这里同样没有拒绝规则**，所以其他能力域篇目里实测过的同一条性质适用：**收窄意味着移除或改动一条绑定，而加一条更具体的什么都减不掉。**

### 第三部分：两道门，而请求两道都要过

这是大多数其他平台里没有对应物的一条性质，值得在实测之前先把它当成一条原则说出来。

**一个请求带着一个令牌到达，而令牌带着一组范围。** 令牌背后的身份有角色。**两者都会被检查**，而有效的权限是它们的交集。

实测，一个持有对象管理员角色的身份，配三种不同的令牌：

| 令牌带着的范围 | 实际可用的权限 |
|---|---|
| 不限制 | 读、列举、创建、删除 |
| 只给读与列举 | **读、列举** |
| 只给读 | **读** |

**一个角色，三种结果**，而那个身份的配置在三种情况下完全相同。

> **一个请求要过两道门：它的令牌带着的范围、以及它的身份持有的角色。有效权限是交集，所以把角色配成最小权限只是这件事的一半 —— 发出去的那个令牌带着另一半。**

**这与大多数平台上的直觉相反。** 在别处一个宽角色就是那条发现；而这里，宽角色配一个窄范围令牌是无害的，**窄角色配一个宽范围令牌也是无害的** —— 暴露来自两者同时都宽。

**而那些范围通常被设在比策略更少被审的地方**：客户端库的配置里、流水线的凭据步骤里、工作负载的启动参数里。**这就让它们成为"策略评审没看到的那一半决定"该去找的地方。**

### 第四部分：服务账号既是身份，也是资源

**那句话的两半都要紧，而它们的组合产生了这里最要紧的那个权限。**

实测，一个自己没有读权限、但有权扮演一个有此权限的服务账号的主体：

| 事实 | 结果 |
|---|---|
| 该主体直接读一个对象 | **拒绝** |
| 该主体可以扮演那个服务账号 | 允许 |
| 那个服务账号读那个对象 | 允许 |
| **于是该主体通过那个服务账号够到了那个对象** | **它获得了一个从未被授予过的权限** |

> **"扮演一个服务账号"的权限不授予对某个资源的访问。它授予的是另一个身份 —— 这把它放进了与别处实测过的"创建负载"那一类权限同一个家族。**

**而那句话的后半是一个第二条路。** 一个服务账号是一个带自己策略的资源，**所以能修改那个策略的人就能给自己或别人授予"扮演它"的能力** —— 而能删掉或停用它的人能弄坏依赖它的一切。

**而一个项目的默认服务账号是第一个要看的**，因为它存在于每个项目里、任何没有指定服务账号的东西都会用它，**而它的角色往往是项目创建时被顺手赋予、之后再也没有回看过。**

### 第五部分：公开是一个成员名

实测，同一个角色授予三种成员：

| 绑定里的成员 | 一个来自互联网的调用者 |
|---|---|
| **`allUsers`** | **允许** |
| `allAuthenticatedUsers`（未绑定） | 拒绝 |
| 一个具体的用户（未绑定） | 拒绝 |

**在这家，公开访问是被表达成一个成员、而不是一个开关**，这有两重意义：**那两个特殊成员名是唯一意味着"任何人"的东西**，而**一条含其中一个的绑定，与一条给某个组的绑定在写法上一模一样** —— 同样的字段、同样的角色，只是一个字段的值不同。

> **在这家，公开是成员字段里的一个值。所以要搜的是那个值，而不是某个布尔字段。**

**而那两个特殊名字之间的差别值得说清**：一个意味着任何人，另一个意味着任何"用任何账号认证过"的人 —— **对一个放在互联网上的资源来说，那是比听起来小得多、又比运维通常想要的大得多的一个集合。**

### 第六部分：条件会收窄一条绑定

实测，一条挂在组织层、带条件的授予：

| 资源 | 结果 |
|---|---|
| 在条件之内 | **允许** |
| 在条件之外 | **拒绝** |

**所以一条挂在高层的绑定并不总是意味着一次宽的授予**，而只看角色名会高估它。

> **一条授予必须连着它的条件一起读，因为条件能把一条看起来覆盖整个组织的授予变成只覆盖一个项目。**

**而评审真正担心的是反方向**：一条条件被移除、或者本来写得很窄之后被改过的绑定，会在角色完全没变的情况下改变那次授予的触及范围。**这就让条件成为一个要"比对差异"的字段，而不是一个读一次就够的字段。**

### 第七部分：密钥与令牌的寿命不同

**访问令牌是短命的** —— 它的有效期以小时计 —— **而服务账号密钥是一份可下载的、含私钥的文件，本身没有过期时间。**

> **"用一个服务账号"与"用服务账号的一把密钥"是两件事：前者的凭据会过期，后者的凭据在被撤销之前一直有效。**

**而密钥是一份文件，这才是它成为另一类风险的原因。** 它可以被复制、被提交进仓库、被打进镜像层、被邮件发出去 —— **而这些每一处都是讲密钥与讲容器镜像那两篇里实测过的地方** —— 而一个在运行时取得的令牌只存在于向它索取的那个进程里。

**而密钥没法靠轮换变得更安全**，因为轮换是一件得有人记得去做的事，而替代它的那个东西是一次不需要任何人参与的过期。

### 第八部分：从这些机制推出的安全观念

**评审从这棵树的顶端开始。** 实测，一条在组织上的授予覆盖了它下面的每一个项目，而一条在项目上的只覆盖一个。**所以宽的授予在祖先那一层，而"谁能到这个项目"这个问题是靠向上走回答的**，而不是靠读那个项目自己的策略。

**并且要数两道门、不是一道。** 实测，一个角色在三种令牌范围下产出了三组不同的可用权限。**所以对角色做评审只是这件事的一半**，而另一半住在令牌被签发的地方 —— 一条流水线、一个库的配置、一个工作负载的参数。

**与服务账号有关的那些动作是身份权限。** 实测，扮演它的能力够到了那个主体从未被授予过的权限，而修改它自己的策略是通向同一个地方的第二条路。**所以它们该和凭据一起评审，而不是和资源策略一起。**

**公开是一个值，不是一个标志。** 实测，一个成员名让一个资源对任何人可达，而它看起来像一条普通的组绑定。**所以要做的检查是在整棵树上搜那个值**，因为在任何一层上的绑定都覆盖它下面的一切。

**而条件才是让高层绑定不自动等于"宽"的东西。** 实测，一个条件把一条组织层的授予变成了只覆盖一个项目。**所以它们值得比对差异，因为移除一个会在角色不变的情况下改变触及范围。**

**而层级是一个项目的数量不等于边界的数量的原因。** 一百个项目、少数几条授予，就是一份配置；而能找到它的那次评审，是读了祖先的那一次。

### 检测与缓解

- **先审组织与文件夹层的绑定**，因为实测的继承覆盖下面的一切，而一次项目级的评审看不到它们。
- **在整棵树上搜那两个公开成员名**，因为在任何一层上的绑定都覆盖它的后代，而它的写法与一条普通的组授予一模一样。
- **对"允许扮演服务账号"的权限告警**，实测它够到了那个主体从未持有的权限。
- **对服务账号自身策略的编辑告警**，因为它既是一个身份、也是一个资源。
- **按年龄与是否存在来报出服务账号密钥**，并把一把密钥当成要去除的凭据、而不是要去轮换的凭据。
- **检查每个项目的默认服务账号**，因为它无处不在、默认被使用、而很少被回看。
- **比对绑定上的条件差异**，因为移除一个会在角色不变的情况下放宽一次授予。
- **把令牌上的范围与角色放在一起审**，因为实测的有效权限是两者的交集，而范围被设在更少被审的地方。
- **缓解上，优先用运行时取得的短命凭据，而不是下载下来的密钥**，这样过期不需要任何人参与。
- **把授予放在工作允许的最低层**，并把项目层之上的授予当成覆盖它下面的一切。
- **只把扮演服务账号的能力授予真正需要那个身份的东西**，并把它当成一次身份授予来评审、而不是一次资源授予。
- **并且把组织策略留作天花板**，因为一条能拒绝的规则在下面那层配置出错时仍然成立。
