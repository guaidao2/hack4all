---
id: kubernetes-rbac-and-hardening
title_en: Kubernetes RBAC and Hardening
title_zh: Kubernetes RBAC 与加固
summary_en: A model with only unions and no denials, which turns privilege escalation into a set of ordinary API calls rather than a bypass. Measured by evaluating the rules — a narrow binding that subtracts nothing, a name restriction that cannot constrain a list, and a three-step chain where two permissions meant to allow administration end with reading secrets the subject was never granted.
summary_zh: 一个只有并集、没有拒绝的模型，于是提权变成了一串普通的 API 调用、而不是一次绕过。这一篇把规则跑出来实测 —— 一条减不掉任何东西的窄绑定、一条约束不了列举操作的对象名限制，以及一条三步的链：两个本意是允许管理员的权限，最后让主体读到了从未授予过它的秘密。
tags: [beginner, cloud, kubernetes, rbac, hardening]
tools: [kubectl, kubeaudit, python3]
attck: [T1078.004, T1610]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A model with only unions, and escalation as a designed feature

RBAC looks like a permission system because it is one. What makes it interesting is what it deliberately leaves out and what it deliberately adds.

> **There are no denial rules, so narrowing can only mean removing a binding. And two permissions — one to escalate and one to bind — exist precisely so that administration is possible, which makes them the shortest path to everything.**

Both are measured below by implementing the rule matching and running an authorisation engine against the questions that matter.

### Part 1: the shape of it

| Element | What it is |
|---|---|
| subject | a user, a group, or **a service account** |
| Role / ClusterRole | **a set of rules**, with no effect on its own |
| RoleBinding | attaches a role **within one namespace** |
| ClusterRoleBinding | attaches a ClusterRole **across the cluster** |
| a rule | api groups, resources, verbs, and optionally **names** |

**And the scope of a grant lives in the binding rather than in the role.** A ClusterRole is a reusable set of rules; where it applies is decided entirely by which kind of binding references it. **Which is why reading a role tells you what could be granted and nothing about to whom.**

**And there is no effect without a binding**, which is the safe half of the design: a role that grants everything grants nothing until something binds it to somebody.

### Part 2: it is a union, and there is nothing to subtract

Measured, a subject bound to a narrow role and then to a broad one:

| Request | Before the broad binding | After |
|---|---|---|
| read a secret | allowed | allowed |
| **delete a secret** | refused | **allowed** |
| **create a cluster role** | refused | **allowed** |

**And the second binding is what did it** — the record of why shows both bindings matching, and the broad one alone would have been enough.

> **In this model narrowing can only mean removing a binding. Adding a more specific one subtracts nothing.**

**Which is the same property the entry on cloud networks measured about an allow-only security group**, and it shows up wherever a permission model is built without a denial: **"we restricted it" is a claim about what was removed, and it can only be checked by looking for the widest thing that still matches.**

**And it has a practical consequence for review**: a subject's effective permissions are the **union of every binding that names it**, directly or through a group — so the question is not what each binding grants but what the union comes to, which is usually much larger than any single line suggests.

### Part 3: a name restriction cannot constrain a listing

Measured, a rule granting `get` and `list` on secrets, restricted to one name:

| Request | Outcome |
|---|---|
| get the named secret | **allowed** |
| get a different secret | refused |
| **list all secrets** | **refused** |

**The third row is the one worth reading twice.** The rule mentions `list`, and the listing was refused, because the restriction is applied to the request's target name — **and a listing does not have one**.

> **A name restriction only applies to requests that name an object. A listing targets a set, so the rule does not match at all.**

**Which makes this the one place where the mistake is in the safe direction**: an administrator intending "you may read only this secret" writes `list` alongside `get`, and the result is that the subject cannot list — **not that it can list everything**. **The failure is a missing permission rather than an extra one, and it presents as a broken tool rather than as a finding.**

### Part 4: the role says what, the binding says where

Measured, one ClusterRole granting secret reads, referenced two ways:

| Subject | Binding | In one namespace | In another |
|---|---|---|---|
| a RoleBinding in the first namespace | namespace-scoped | **allowed** | refused |
| a ClusterRoleBinding | cluster-wide | **allowed** | **allowed** |

**The role is identical in both cases**, and only the binding differs.

> **The contents of a role and the reach of a grant are two different things, written in two different objects.**

**So the thing to read is the binding**, and the thing to check is the binding kind — because "we only gave it access to one namespace" is a statement about a RoleBinding, and the same role name bound the other way is a cluster-wide grant that looks familiar.

### Part 5: escalate and bind, where escalation is an ordinary call

The rule worth knowing is a safeguard: **a subject cannot create a role granting permissions it does not itself hold**, which stops anybody with role-creation rights from simply writing themselves an administrator. **And two permissions exist as the deliberate exception**, because otherwise administration would be impossible.

Measured, a subject with permission to create roles and bindings but no permission to read secrets:

| Step | Outcome |
|---|---|
| read a secret | **refused** |
| create a role granting secret reads | **refused — it does not hold those permissions and has no `escalate`** |
| the same, after `escalate` on roles | **allowed** |
| bind the new role to itself | allowed |
| **read a secret** | **allowed** |

**Three ordinary API calls, each of them logged, none of them a bypass.** The subject did not defeat the model; it used a permission the model provides.

> **`escalate` and `bind` grant no access to any resource. What they grant is the ability to change authorisation — which is why they are among the most dangerous permissions in the system and are usually granted as part of a role that sounds administrative.**

**And the same is true of `impersonate`**, which is the third of the family: with it, a subject does not need to construct anything, because it can simply act as somebody else.

### Part 6: permission to create a workload is permission to be any service account

Measured, a subject that can create pods and nothing else:

| Step | Outcome |
|---|---|
| read a secret directly | **refused** |
| create a pod with a service account that can read secrets | **allowed — and the pod runs with that service account's permissions** |

**The subject never gained a permission.** It created a workload whose specification named a different identity, and the workload is given that identity's credentials by the platform.

> **Being able to create a workload in a namespace means being able to be any service account in that namespace — because the workload's specification chooses the identity and the platform mounts its credentials.**

**And the permissions that count are wider than "create pods"**: creating a deployment, a cron job, or a job all produce pods, and **attaching to a running pod gives whatever that pod's identity has.**

**Which is why the review question about this class of permission is not "what can it run" but "whose identity can it run as"** — and the answer, without anything else in place, is every service account in the namespace, including ones created for components that were never meant to be reachable this way.

### Part 7: permission to read secrets is permission to be an identity

Measured, a subject that can read secrets in a namespace where another identity's credentials are stored:

| Fact | Consequence |
|---|---|
| the subject may read secrets | allowed |
| a secret holds a service account's credentials | the credentials are readable |
| **the subject may therefore act as that identity** | the distinction between reading a secret and being an identity is zero |

**And a namespace's secrets are not only other identities' credentials.** Measured in the other entries of this series: a database password in a connection string, a registry credential, a TLS private key, and an application's own signing key. **So the permission to read secrets in a namespace is the permission to reach most of what the workloads in it can reach.**

> **In this system, secrets are where credentials are kept — so "may read secrets" is not a resource permission. It is an identity permission.**

**And that is what makes the namespace the security boundary it is usually described as** — but only if the secrets inside it are ones that its workloads are mutually entitled to, which is a property of how it was populated rather than of the model.

### Part 8: what follows for security

**The review that works is about what a subject can end up with, not what it was granted.** Measured, one subject went from no secret access to reading secrets in three calls, and another got the same result in one call by creating a workload. **Both were authorised at every step**, so a review that asks "is this binding correct" finds nothing.

**The only-union structure means the check is for the widest matching grant.** Measured, a specific binding added nothing and a broad one gave everything. **So the list to produce is the union per subject**, and the questions that find the problems are about wildcards, about cluster-wide bindings, and about the bindings nobody remembers adding.

**`escalate`, `bind` and `impersonate` are the authorisation-changing permissions.** Measured, the first two made escalation an ordinary call. **They belong with the credentials in review, not with the configuration** — because what they grant is not access but the ability to define access.

**And the workload-creation permissions are identity permissions.** Measured, creating a pod with a chosen service account reached that account's permissions without the subject ever being granted them. **The control that actually works is an admission policy restricting which service accounts a workload may use**, because RBAC alone cannot express "you may create pods but only as yourself".

**And secrets are the identity store.** Measured, reading one was equivalent to being the identity it belonged to. **So the namespace's secret population is part of its permission boundary**, and a namespace is only as separate as the credentials that were put into it.

### Detection and mitigation

- **Alert on the authorisation-changing verbs** — `escalate`, `bind`, `impersonate` — and on any role that grants them, since they change what is possible rather than what is allowed.
- **Alert on the sequence of creating a role, binding it, and then using it**, which is the measured three-step path and is legible in an audit log even though each call is legitimate.
- **Report bindings that grant wildcards in any of the four rule fields**, and bindings that reference high-privilege ClusterRoles cluster-wide.
- **Treat permission to create pods, deployments, jobs or cron jobs as permission to use every service account in the namespace**, and list the subjects that have them.
- **Report permission to read secrets**, since the measured distance between reading a secret and being an identity is zero.
- **Check that service account tokens are not mounted where they are not needed**, because a mounted token is a credential present in every process of the workload.
- **Review the union per subject rather than each binding**, since measured additions can grant everything and the widest grant is what matters.
- **For mitigation, do not grant wildcards**, and treat a rule granting `*` on `*` as a finding regardless of how it was justified.
- **Grant `escalate`, `bind` and `impersonate` to nothing that does not administer authorisation**, and review them as credentials rather than as configuration.
- **Restrict which service accounts a workload may reference with an admission policy**, since this is the only control that addresses the measured workload-creation path.
- **Give each service account the least it needs**, because a service account's permissions are reachable by everything that can create a workload in its namespace.
- **And keep a namespace's secrets to what its workloads are mutually entitled to**, since the namespace is a boundary only as far as its credentials allow.

<!-- lang:zh -->
### 一个只有并集的模型，而提权是它有意留出的功能

RBAC 看起来像一个权限系统，因为它就是。真正有意思的是它刻意去掉了什么、又刻意加上了什么。

> **这里没有拒绝规则，所以收窄只能是撤掉一条绑定。而有两个权限 —— 一个叫 escalate、一个叫 bind —— 存在的意义恰恰是让管理成为可能，这就让它们成为通往一切的最短路径。**

两条都在下面实测：把规则匹配实现出来，然后拿要紧的问题去问一个鉴权引擎。

### 第一部分：它的形状

| 元素 | 它是什么 |
|---|---|
| 主体 | 一个用户、一个组、或者**一个服务账号** |
| Role / ClusterRole | **一组规则**，本身没有任何效果 |
| RoleBinding | 把角色挂在**一个命名空间之内** |
| ClusterRoleBinding | 把一个 ClusterRole 挂在**整个集群** |
| 一条规则 | api 组、资源、动词，以及可选的**对象名** |

**而一次授予的范围住在绑定里、不在角色里。** 一个 ClusterRole 是一组可复用的规则；它在哪里生效，完全由引用它的是哪种绑定决定。**这就是为什么读一个角色只能知道"可能被授予什么"，而对"授予给谁"一无所知。**

**而没有绑定就没有任何效果**，这是这个设计安全的那一半：一个授予了一切的角色，在被绑给某人之前什么都不授予。

### 第二部分：它是并集，而没有东西可以做减法

实测，一个主体先被绑到一个窄角色、然后被绑到一个宽角色：

| 请求 | 加上宽绑定之前 | 之后 |
|---|---|---|
| 读一个 secret | 允许 | 允许 |
| **删一个 secret** | 拒绝 | **允许** |
| **创建一个 ClusterRole** | 拒绝 | **允许** |

**而做这件事的是那第二条绑定** —— 原因记录里能看到两条绑定都命中了，而光那条宽的就已经够了。

> **在这个模型里，收窄只能是撤掉一条绑定。加一条更具体的，减不掉任何东西。**

**这与讲云网络那一篇关于"只有允许的安全组"实测到的性质是同一条**，而它出现在每一个没有拒绝概念的权限模型里：**"我们限制了它"是一句关于"移除了什么"的话，而它只能靠寻找"仍然匹配的最宽的那条"来核实。**

**而它对评审有一个实际后果**：一个主体的有效权限是**每一条点名了它的绑定（直接或通过组）的并集** —— 所以要问的不是每条绑定授予了什么，而是那个并集合起来是什么，而它通常比任何单独一行暗示的都大得多。

### 第三部分：对象名限制约束不了列举

实测，一条授予 secrets 上 `get` 与 `list`、但限制到一个对象名的规则：

| 请求 | 结果 |
|---|---|
| 读那个被点名的 secret | **允许** |
| 读另一个 secret | 拒绝 |
| **列举全部 secret** | **拒绝** |

**第三行值得读两遍。** 那条规则里明明写了 `list`，而列举被拒绝了，因为那条限制作用在请求的目标名上 —— **而一次列举没有目标名**。

> **对象名限制只对那些"点名了某个对象"的请求生效。一次列举的目标是一个集合，所以那条规则根本匹配不上。**

**这就让这里的错误落在安全的那个方向上**：一个本意是"你只能读这一个 secret"的管理员，会把 `list` 和 `get` 写在一起，结果这个主体列不出来 —— **而不是它能列出全部**。**这个失败是一次缺失的权限，而不是一次多余的权限，它的表现是一个工具坏了，而不是一条发现。**

### 第四部分：角色说"什么"，绑定说"哪里"

实测，同一个授予 secret 读取的 ClusterRole，用两种方式被引用：

| 主体 | 绑定 | 在第一个命名空间 | 在另一个命名空间 |
|---|---|---|---|
| 在第一个命名空间里的一个 RoleBinding | 命名空间范围 | **允许** | 拒绝 |
| 一个 ClusterRoleBinding | 集群范围 | **允许** | **允许** |

**两种情况下角色完全相同**，差别只在绑定。

> **一个角色的内容与一次授予的触及范围是两件不同的事，写在两个不同的对象里。**

**所以要读的是那条绑定**，要查的是绑定的种类 —— 因为"我们只给了它一个命名空间的访问"是一句关于 RoleBinding 的话，而同一个角色名用另一种方式绑上去，就是一次看起来眼熟的、全集群的授予。

### 第五部分：escalate 与 bind，提权是一次普通的调用

有一条规则值得知道，它是一道保险：**一个主体不能创建一个授予了它自己并不拥有的权限的角色**，这挡住了任何一个有建角色权限的人直接给自己写一个管理员。**而有两个权限是它有意留出的例外**，因为没有它们管理就无法进行。

实测，一个有权创建角色与绑定、但无权读 secret 的主体：

| 步骤 | 结果 |
|---|---|
| 读一个 secret | **拒绝** |
| 创建一个授予读取 secret 的角色 | **拒绝 —— 它不拥有那些权限，也没有 `escalate`** |
| 同上，在给它 `escalate` 之后 | **允许** |
| 把那个新角色绑给自己 | 允许 |
| **读一个 secret** | **允许** |

**三次普通的 API 调用，每一次都被记下来了，没有一次是绕过。** 这个主体没有打败这个模型；它用了一个这个模型提供的权限。

> **`escalate` 与 `bind` 不授予任何资源访问。它们授予的是"改变授权的能力" —— 这就是为什么它们属于这个系统里最危险的权限，而它们通常作为某个听起来像管理员角色的组成部分被授予。**

**`impersonate` 是这一家的第三个**：有了它，主体什么都不用构造，因为可以直接以别人的身份行事。

### 第六部分：创建负载的权限，就是成为任意服务账号的权限

实测，一个只能创建 Pod、别的什么都不能的主体：

| 步骤 | 结果 |
|---|---|
| 直接读一个 secret | **拒绝** |
| 创建一个使用"能读 secret 的服务账号"的 Pod | **允许 —— 而这个 Pod 就带着那个服务账号的权限运行** |

**这个主体从来没有得到过那个权限。** 它创建了一个在规格里点了另一个身份的负载，而那个负载由平台赋予那个身份的凭据。

> **在一个命名空间里能创建负载，就意味着能成为那个命名空间里的任何一个服务账号 —— 因为负载的规格选择了身份，而平台把那个身份的凭据挂了进去。**

**而要算的权限比"create pods"更宽**：创建一个 deployment、一个 cron job、一个 job 都会产出 Pod，而**附着到一个正在运行的 Pod 上就得到那个 Pod 的身份所拥有的一切**。

**这就是为什么这一类权限的评审问题不是"它能跑什么"，而是"它能让东西以谁的身份跑"** —— 而在没有任何别的措施时，答案是那个命名空间里的每一个服务账号，包括那些为某个组件创建、从来没打算被人以这种方式够到的。

### 第七部分：读 Secret 的权限，就是成为一个身份的权限

实测，一个能在某个命名空间里读 secret 的主体，而那个命名空间里存着另一个身份的凭据：

| 事实 | 后果 |
|---|---|
| 该主体可以读 secret | 允许 |
| 有一个 secret 装着一个服务账号的凭据 | 那些凭据是可读的 |
| **于是该主体可以以那个身份行事** | "读一个 secret"与"成为一个身份"之间的距离是零 |

**而一个命名空间里的 secret 不只是别的身份的凭据。** 这个系列其他篇目里实测过的有：连接串里的一个数据库口令、一份仓库凭据、一个 TLS 私钥、以及一个应用自己的签名密钥。**所以在某个命名空间里读 secret 的权限，就是够到其中工作负载所能够到的绝大部分东西的权限。**

> **在这个系统里，secret 是凭据被存放的地方 —— 所以"可以读 secret"不是一个资源权限。它是一个身份权限。**

**这就是为什么命名空间是它通常被描述的那个安全边界** —— 但前提是它里面的 secret 确实是它的工作负载彼此有权知道的，而那是一条关于"它被怎么填充"的性质，不是模型的性质。

### 第八部分：从这些机制推出的安全观念

**有效的评审是关于"一个主体最终能得到什么"，而不是"它被授予了什么"。** 实测，一个主体从没有任何秘密访问，到能读 secret，只用了三次调用；另一个用一次调用得到了同样的结果 —— 通过创建一个负载。**两次每一步都是被授权的**，所以一个问"这条绑定对不对"的评审什么也找不到。

**只有并集这个结构意味着要查的是"最宽的那条匹配的授予"。** 实测，一条具体的绑定什么也没加上，而一条宽的把一切都给了。**所以要产出的清单是每个主体的并集**，而能找到问题的那些问题是关于通配符的、关于全集群绑定的、以及关于没人记得加过的那条绑定的。

**`escalate`、`bind` 与 `impersonate` 是改变授权的权限。** 实测，前两个把提权变成了一次普通调用。**它们该和凭据一起评审，而不是和配置一起** —— 因为它们授予的不是访问，而是"定义访问"的能力。

**而创建负载这一类权限是身份权限。** 实测，创建一个指定了某个服务账号的 Pod，够到了那个账号的权限，而主体从来没有被授予过它们。**真正管用的控制是一条限制"负载可以引用哪些服务账号"的准入策略**，因为 RBAC 本身无法表达"你可以建 Pod，但只能以你自己的身份"。

**而 secret 就是身份存储。** 实测，读其中一个等价于成为它所属的那个身份。**所以一个命名空间的秘密构成是它权限边界的一部分**，而一个命名空间能有多独立，取决于被放进去的是哪些凭据。

### 检测与缓解

- **对改变授权的动词告警** —— `escalate`、`bind`、`impersonate` —— 以及对任何授予它们的角色告警，因为它们改变的是"什么成为可能"，而不是"什么被允许"。
- **对"创建角色 → 绑定它 → 然后使用它"这个序列告警**，那是实测的三步路径，而且尽管每一次调用都合法，它在审计日志里是读得出来的。
- **报出"在四个规则字段里任意一个上授予通配符"的绑定**，以及全集群引用高权限 ClusterRole 的绑定。
- **把创建 Pod、deployment、job、cron job 的权限当成"使用该命名空间里每一个服务账号"的权限**，并列出拥有它们的主体。
- **报出读 secret 的权限**，因为实测"读一个 secret"与"成为一个身份"之间的距离是零。
- **检查服务账号令牌没有被挂在不需要它的地方**，因为被挂载的令牌是一个存在于该工作负载每一个进程里的凭据。
- **按主体评审并集，而不是逐条评审绑定**，因为实测中新增的一条可以授予一切，而要紧的是最宽的那条。
- **缓解上，不要授予通配符**，并把一条 `*` on `*` 的规则当成一条发现，无论它被怎么解释。
- **不把 `escalate`、`bind`、`impersonate` 授予任何不管理授权的东西**，并把它们当成凭据来评审、而不是当成配置。
- **用准入策略限制负载可以引用哪些服务账号**，因为这是唯一针对实测那条"创建负载"路径的控制。
- **给每个服务账号它所需要的最小权限**，因为一个服务账号的权限，对每一个能在它的命名空间里创建负载的东西都是可达的。
- **并且让一个命名空间里的秘密只限于它的工作负载彼此有权知道的那些**，因为命名空间只有在它的凭据允许的范围内才是一个边界。
