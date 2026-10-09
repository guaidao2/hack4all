---
id: cloud-aws
title_en: AWS Service Map and Identity Model
title_zh: AWS 服务地图与身份模型
summary_en: The oldest of the large providers, which is why its vocabulary became the industry's, and whose identity model has three properties worth knowing — an account as the boundary, roles as identities nobody logs into, and a signature scoped to a date, a region and a service. Measured by implementing the signature scheme and reading real resource names apart.
summary_zh: 几家大厂里最早的一家，所以它的词成了行业通用词；而它的身份模型有三条值得知道的性质 —— 账号是边界、角色是没有人登录的身份、以及签名被限定在某一天、某个区域、某个服务上。这一篇把签名方案实现出来、把真实的资源名拆开来看。
tags: [beginner, cloud, aws, iam, sigv4]
tools: [aws, awscli, cloudsplaining, python3]
attck: [T1078.004, T1098.003]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The one whose words everyone uses

S3, EC2, IAM, Lambda — the names of this provider's services are used as generic terms by its competitors, and that fact says something about the model worth starting from.

**This is the earliest of the large providers, so its abstractions were the first ones people learned.** When another platform says "bucket" or "role" or "policy", it is describing its own thing in these words, and the words carry assumptions from here that do not always hold there.

Measured below: the signature scheme's scope, and what real resource names contain.

### Part 1: the map

| Area | The services that define it |
|---|---|
| identity | IAM users, roles and policies; STS for temporary credentials; Organizations for accounts |
| compute | virtual machines, containers, functions |
| storage | object storage, block volumes, shared filesystems |
| network | virtual networks, subnets, gateways, load balancers, DNS |
| data | managed relational, key-value, warehouse, streaming |
| operations | logging, metrics, configuration, deployment |
| security | key management, secret storage, certificate management |

**And every provider has this same map with different names.** That is why the entries in this series are grouped by capability rather than by vendor: **object storage is object storage, and the differences that matter are in the identity model and in the few places where the abstraction leaks.**

**And the leaks are where the security content is.** A managed service that grants a role to run code on your behalf, a storage service whose names are globally unique, a key service with an access policy of its own — **each is an abstraction that puts a second decision somewhere other than the policy you were reading.**

### Part 2: the account is the boundary

**An account is the unit of isolation, of quota and of billing.** Almost every access question is ultimately "from which account, to which account", and the answers have a shape that is worth internalising:

| Fact | Consequence |
|---|---|
| resources live in an account | a resource policy names an account or a principal |
| identities live in an account | a trust policy decides which accounts may assume a role |
| **the account identifier is not a secret** | it appears in resource names, in error messages and in bucket names |
| the region is a second dimension | a resource is in one, and a grant can be limited to some |

**And the account identifier being public is a fact worth being explicit about**, because it looks like an identifier that should be protected and is not. **What it establishes is not access but which doors exist** — which is why the useful control is the trust policy rather than the obscurity of the number.

**And the root credential of an account is a thing of its own.** It is not a user, it is not governed by the policies that govern users, and it cannot be limited by most of the mechanisms in this entry. **Which is why "can this account be taken over" is really a question about its root credential and about who can create an identity that can act as it.**

### Part 3: a role is an identity nobody logs into

**The role is the abstraction that makes this model readable**, and it has two properties that explain most of the rest of the entry.

**It has two documents.** A trust policy says **who may become it**, and a permission policy says **what may be done once they have** — measured in the earlier entry on cloud identity, including the shorthand that makes an account look narrower than it is.

**And it is assumed rather than logged into**, which makes the credentials that come out of it temporary and attributable. **A session carries the identity that assumed it**, which is the join key an investigation uses and the reason a role shared by many workloads is a poor thing to attribute an action to.

**And a role is how a service is given permission to act on your behalf.** A function, an instance or a pipeline is given a role, and the platform hands the workload the role's credentials — **which is why the entry on serverless measured that one role serving several event sources is one permission boundary covering all of them.**

### Part 4: a resource name is a global namespace

Measured, real resource names split into their fields:

| Resource name | Region field | Account field |
|---|---|---|
| `arn:aws:s3:::examplebucket` | **empty — global** | **empty — none** |
| `arn:aws:s3:::examplebucket/report.pdf` | **empty** | **empty** |
| `arn:aws:iam::111111111111:user/deploy` | **empty — global** | `111111111111` |
| `arn:aws:iam::111111111111:role/app-role` | empty | `111111111111` |
| `arn:aws:iam::111111111111:root` | empty | `111111111111` |
| `arn:aws:kms:us-east-1:111111111111:key/1234abcd...` | `us-east-1` | `111111111111` |
| `arn:aws:lambda:us-east-1:111111111111:function:resize` | `us-east-1` | `111111111111` |

**The first two rows are the ones to notice.** Object storage names have neither a region nor an account, **so a policy written against one cannot be scoped to an account by its resource field** — the account goes in the statement's principal instead.

> **Not every resource name contains an account. Where it does not, the account has to be named somewhere else.**

**And `:root` at the end of an identity name is not a user called root** — it means the account as a whole, so a trust policy naming it admits every principal in it. **Which is why a name that reads like one identity and behaves like a population is worth searching for in review.**

### Part 5: resource patterns and actions are paired

Measured, patterns against resources:

| Pattern | Resource | Covers |
|---|---|---|
| `arn:aws:s3:::examplebucket/*` | the object | **yes** |
| `arn:aws:s3:::examplebucket/*` | the bucket itself | no |
| `arn:aws:s3:::examplebucket` | the object | no |
| `arn:aws:iam::111111111111:role/*` | a role in that account | yes |
| **`arn:aws:iam::*:role/app-role`** | **a role in another account** | **yes** |

**The first three rows are the pairing between an object-level action and an object-level name**, which is the measurement from the cloud identity entry arriving in a concrete form: **an action that reads an object needs the object's name, and an action that lists a bucket needs the bucket's.**

**And the last row is the one that surprises.** A wildcard in the account field matches every account, **so a pattern that looks like a convenience is a grant to the world** — and the field's position in the middle of the string makes it easy to read past.

**Which gives a review technique that does not depend on reading the whole policy**: **check each field of each resource pattern separately** — the partition, the service, the region, the account and the resource path — because the risky ones are the short segments in the middle.

### Part 6: a signature is scoped to a date, a region and a service

The signature scheme derives a key rather than using the secret key directly, and the derivation is a chain of four inputs. Measured, the same secret producing four different keys:

| Date | Region | Service | Derived key |
|---|---|---|---|
| 20261008 | us-east-1 | s3 | `c4d3d32a9f3d8454...` |
| 20261008 | **eu-west-1** | s3 | `5fcd21a13d48d69f...` |
| 20261008 | us-east-1 | **dynamodb** | `e2cd7708604a399d...` |
| **20261009** | us-east-1 | s3 | `436eaae7b53526cd...` |

**And the consequence was measured by re-signing in the other scopes**: the same request signed for one region and service does not verify under another — the recomputed signature differed in 59 of 64 characters.

> **A signature is pinned to a date, a region and a service. The long-term secret is one value; what it produces is valid in exactly one scope.**

**Which has a practical reading in both directions.** A signed request captured in transit is bound to its scope, **so it cannot be replayed against a different region or a different service** — and it is bounded in time by the date. **And in the other direction, a leaked long-term secret still produces everything**, which is why temporary credentials are the design goal and the measured scope is a narrowing rather than a defence.

**And the scope is visible in the request**: the credential field in the query string or the header contains the date, the region and the service. **So a signature is also self-describing**, which is what makes a captured one diagnosable.

### Part 7: the surfaces that are specific to this provider

**Object storage's two access mechanisms.** A bucket has a policy and, in older configurations, objects have access control lists, and both can grant public access — **with account-level and bucket-level switches above them**, which the storage entry measured overriding the policy rather than being overridden by it.

**Instance metadata.** The address measured in the compute entry is a credential source with two modes and a hop limit, and its credentials are the instance's role.

**Key management with its own policy.** A key is an object with an access policy of its own, **which makes it a second decision about the same data**: a principal with permission to read an encrypted object and no permission to use the key cannot read it, **and that is a control which survives a misconfigured storage policy.**

**Organisations as a ceiling.** A policy attached above the account can deny, **and because it denies rather than grants, it is the mechanism that survives an account-level mistake.**

**And audit as two planes.** Configuration changes and data operations are recorded in different places with different defaults — the measurement in the logging entry — with the account's own log being a thing an attacker with sufficient rights inside it can stop.

### Part 8: what follows for security

**The account is the boundary and the role is the unit of permission**, and most of the review follows from those two sentences. **A long-lived key on a workload is the thing to find and replace with a role**, because the role's credentials expire and carry the identity that assumed them.

**The resource name's fields are worth checking one at a time.** Measured, an object name and a bucket name do not cover each other, and a wildcard in the account field grants across accounts. **So the check is not "does the policy look right" but "in this pattern, which fields are wildcards"** — and the risky ones are the short middle segments that a reader's eye skips.

**And the account field not existing at all is a property of some names.** Measured, object storage names carry neither region nor account, **so the account has to be named in the statement instead** — which is why a storage policy and an identity policy are read differently.

**The signature's scope is a narrowing rather than a defence.** Measured, one request verified in exactly one of three scopes, and the same long-term secret produced all three. **So the useful conclusions are about blast radius and about time**, and the control that removes the long-lived secret is separate.

**And the provider-specific surfaces are mostly second decisions**: a key policy beside a storage policy, a switch beside a bucket policy, an organisation policy above an account policy. **Each is a place where two documents decide one question** — which is the same shape the identity entry measured between an identity policy and a resource policy, and the reason an audit that reads one document has read part of the answer.

### Detection and mitigation

- **Alert on the use of the account's root credential**, since it is not governed by the policies that govern everything else.
- **Report any policy granting `*` on `*`**, and any trust policy containing a bare principal of everything, since the identity entry measured what the second admits.
- **Search resource patterns field by field** — partition, service, region, account, path — because a measured wildcard in the account field grants across accounts.
- **Alert on new cross-account trust** and on changes to a role's trust policy, since that is the document that decides who may become it.
- **Find long-lived access keys and replace them with roles**, since a role's credentials expire and a key's do not.
- **Require multi-factor authentication for the identities that can create other identities**, since creating an identity is a way to become one.
- **Alert on the account-level public access switches being changed**, and on key policies being edited, because both are decisions that override the documents below them.
- **For mitigation, prefer roles to keys, and temporary credentials to both**, so that a disclosed credential has a lifetime.
- **Put a ceiling above the account where the platform provides one**, since a policy that can deny is the only one that holds when the account's own configuration is wrong.
- **Give a workload its own role rather than sharing one**, because the measured session identity is what an action is attributed to.
- **Check both granting mechanisms for object storage and the switches above them**, since both can grant public access and only the switches can prevent it.
- **And read resource patterns with the same care as an IP range**, since the measured difference between covering an object and covering the bucket is two characters.

<!-- lang:zh -->
### 那家词被所有人用的

S3、EC2、IAM、Lambda —— 这家服务商的服务名被它的竞争者当成通用词使用，而这件事本身就说明了值得从这里开始的那个模型。

**它是几家大厂里最早的一家，所以它的抽象是人们最早学会的那一套。** 当另一个平台说"bucket""role""policy"时，它是在用这家的词描述自己的东西，而那些词带着从这里来的假设，在那边并不总是成立。

下面实测：签名方案的适用范围，以及真实的资源名里到底有哪些字段。

### 第一部分：地图

| 区域 | 定义它的那些服务 |
|---|---|
| 身份 | IAM 用户、角色与策略；STS 发临时凭据；Organizations 管账号 |
| 计算 | 虚拟机、容器、函数 |
| 存储 | 对象存储、块卷、共享文件系统 |
| 网络 | 虚拟网络、子网、网关、负载均衡、DNS |
| 数据 | 托管关系库、键值、数仓、流 |
| 运维 | 日志、指标、配置、部署 |
| 安全 | 密钥管理、秘密存储、证书管理 |

**而每一家都有同一张地图，只是名字不同。** 这就是这个系列按能力域而不是按厂商分组的原因：**对象存储就是对象存储，而真正要紧的差别在身份模型里、以及在少数几处抽象漏出来的地方。**

**而漏出来的地方就是安全内容所在。** 一个被授予角色来替你跑代码的托管服务、一个名字全局唯一的存储服务、一个自带访问策略的密钥服务 —— **每一个都是一处抽象，把第二个决定放在了你在读的那份策略之外的地方。**

### 第二部分：账号是边界

**账号是隔离、配额与计费的单位。** 几乎每一个访问问题最终都是"从哪个账号、到哪个账号"，而答案有一个值得内化的形状：

| 事实 | 后果 |
|---|---|
| 资源住在某个账号里 | 资源策略点名一个账号或一个主体 |
| 身份住在某个账号里 | 信任策略决定哪些账号可以扮演一个角色 |
| **账号标识不是秘密** | 它出现在资源名、错误信息与桶名里 |
| 区域是第二个维度 | 一个资源在某一个区域，而一次授予可以被限定在若干个 |

**而"账号标识是公开的"这件事值得明确说一次**，因为它看起来像一个应该被保护的标识、而其实不是。**它确立的不是访问，而是"有哪些门存在"** —— 这就是为什么有用的控制是信任策略，而不是把这个数字藏起来。

**而一个账号的根凭据是它自己的一类东西。** 它不是一个用户、不受管用户的那些策略约束、也无法被这一篇里大多数机制限制。**所以"这个账号会不会被拿下"实际上是关于它的根凭据、以及关于谁能创建一个能充当它的身份的问题。**

### 第三部分：角色是一个没有人登录的身份

**角色是让这个模型读得懂的那个抽象**，而它有两条性质解释了这一篇剩下的大部分。

**它有两份文档。** 信任策略说**谁可以成为它**，权限策略说**成为之后能做什么** —— 前面讲云身份那一篇实测过，包括那个让一个账号看起来比它实际更窄的简写。

**而它是被扮演的、不是被登录的**，这就让从中产生的凭据是临时的、且可归因的。**一个会话带着扮演它的那个身份**，那正是调查要用的连接键，也是一个被很多工作负载共用的角色在归因上不好用的原因。

**而角色是把权限交给一个服务的方式。** 一个函数、一台实例或一条流水线被赋予一个角色，而平台把那个角色的凭据交给那个工作负载 —— **这就是为什么讲无服务器那一篇实测到"一个角色服务好几种事件源"是一条覆盖它们全部的权限边界。**

### 第四部分：资源名是一个全局命名空间

实测，真实的资源名按字段拆开：

| 资源名 | 区域字段 | 账号字段 |
|---|---|---|
| `arn:aws:s3:::examplebucket` | **空 —— 全局** | **空 —— 没有** |
| `arn:aws:s3:::examplebucket/report.pdf` | **空** | **空** |
| `arn:aws:iam::111111111111:user/deploy` | **空 —— 全局** | `111111111111` |
| `arn:aws:iam::111111111111:role/app-role` | 空 | `111111111111` |
| `arn:aws:iam::111111111111:root` | 空 | `111111111111` |
| `arn:aws:kms:us-east-1:111111111111:key/1234abcd...` | `us-east-1` | `111111111111` |
| `arn:aws:lambda:us-east-1:111111111111:function:resize` | `us-east-1` | `111111111111` |

**前两行才是要注意的。** 对象存储的名字既没有区域也没有账号，**所以一条按它写的策略，无法用资源字段把访问圈到某个账号** —— 那个账号要写在语句的主体里。

> **不是每一个资源名里都有账号。没有的时候，账号就得写在别的地方。**

**而一个身份名末尾的 `:root` 不是一个叫 root 的用户** —— 它指的是那个账号整体，所以一条点名它的信任策略会放进那个账号里的每一个主体。**这就是为什么一个"读起来像一个身份、行为像一个群体"的名字值得在评审里搜一搜。**

### 第五部分：资源的写法与动作是配对的

实测，写法对着资源：

| 写法 | 资源 | 是否覆盖 |
|---|---|---|
| `arn:aws:s3:::examplebucket/*` | 那个对象 | **是** |
| `arn:aws:s3:::examplebucket/*` | 桶本身 | 否 |
| `arn:aws:s3:::examplebucket` | 那个对象 | 否 |
| `arn:aws:iam::111111111111:role/*` | 那个账号里的一个角色 | 是 |
| **`arn:aws:iam::*:role/app-role`** | **另一个账号里的一个角色** | **是** |

**前三行是"对象级动作"与"对象级名字"之间的配对**，也就是云身份那一篇的实测在这里的具体形态：**读对象的动作需要对象的名字，列举桶的动作需要桶的名字。**

**而最后一行才是让人意外的那个。** 账号字段上的通配符会匹配每一个账号，**所以一个看起来像图省事的写法是一次给全世界的授予** —— 而那个字段位于字符串中间，很容易被读过去。

**这给出一个不依赖通读整份策略的评审手法**：**把每一条资源写法的每个字段分别检查一遍** —— 分区、服务、区域、账号与资源路径 —— 因为危险的那些是中间那几段短的。

### 第六部分：一次签名被限定在某一天、某个区域、某个服务上

签名方案不是直接用那个秘密，而是先派生一把密钥，而派生是四个输入的链。实测，同一个秘密产出四把不同的密钥：

| 日期 | 区域 | 服务 | 派生出的密钥 |
|---|---|---|---|
| 20261008 | us-east-1 | s3 | `c4d3d32a9f3d8454...` |
| 20261008 | **eu-west-1** | s3 | `5fcd21a13d48d69f...` |
| 20261008 | us-east-1 | **dynamodb** | `e2cd7708604a399d...` |
| **20261009** | us-east-1 | s3 | `436eaae7b53526cd...` |

**而后果是用另一个范围重新签同一个请求测出来的**：在某个区域与服务上签出来的签名，在另一个范围下验不过 —— 重算出来的签名 64 个字符里有 59 个不同。

> **一次签名被钉在某一天、某个区域、某个服务上。长期秘密是一个值；它产出的东西恰好只在一个范围里有效。**

**这件事两个方向都有实际读法。** 一个在传输中被抓到的已签名请求被绑定在它的范围上，**所以它无法被重放到另一个区域或另一个服务** —— 而它被日期限定在一段时间内。**而另一个方向上，一把泄露的长期秘密仍然能产出一切**，这就是为什么临时凭据是设计目标、而实测这个范围是一次收窄而不是一道防御。

**而那个范围在请求里是看得见的**：查询串或头里的凭据字段含着日期、区域与服务。**所以一次签名也是自描述的**，这正是被抓到的那个可被诊断的原因。

### 第七部分：这家特有的那些面

**对象存储的两套访问机制。** 一个桶有策略，而在较老的配置里对象还有访问控制列表，两者都能授予公开访问 —— **而在它们之上还有账号级与桶级开关**，存储那一篇实测过那些开关是压过策略、而不是被策略压过的。

**实例元数据。** 计算那一篇实测过的那个地址是一个凭据来源，有两个模式与一条跳数限制，而它的凭据就是实例的角色。

**带自己策略的密钥管理。** 一把密钥是一个对象、带自己的访问策略，**这就让同一个数据上有了第二个决定**：一个有权限读加密对象、却没有权限使用那把密钥的主体读不出来，**而那是一道在存储策略配错时仍然成立的控制。**

**组织作为天花板。** 挂在账号之上的一条策略可以拒绝，**而因为它拒绝、而不是授予，它是那个在账号层面出错时仍然成立的机制。**

**以及审计的两个平面。** 配置变更与数据操作被记在不同的地方、有不同的默认值 —— 日志那一篇的实测 —— 而账号自己的那份日志，是一个在其中拥有足够权限的人可以停掉的东西。

### 第八部分：从这些机制推出的安全观念

**账号是边界，角色是权限的单位**，而大部分评审都从这两句话推出来。**一个工作负载上的长期密钥是要找出来、并用角色替换掉的东西**，因为角色的凭据会过期、并且带着扮演它的那个身份。

**资源名的每个字段都值得逐个检查。** 实测，一个对象名与一个桶名互相不覆盖，而账号字段上的通配符跨账号授予。**所以要检查的不是"这条策略看起来对不对"，而是"在这个写法里，哪些字段是通配符"** —— 而危险的那些是读的人眼睛会跳过去的、中间那几段短的。

**而"账号字段根本不存在"是某些名字的性质。** 实测，对象存储的名字既不带区域也不带账号，**所以账号要写在语句里** —— 这就是为什么存储策略与身份策略的读法不同。

**签名的范围是一次收窄、而不是一道防御。** 实测，同一个请求恰好在三个范围里的一个里通过，而同一个长期秘密产出了全部三个。**所以有用的结论是关于波及范围与时间的**，而去掉那个长期秘密的控制是另一件事。

**而这家特有的那些面大多是"第二个决定"**：存储策略旁边的一把密钥策略、桶策略旁边的一个开关、账号策略上面的一条组织策略。**每一个都是"两份文档决定同一个问题"的地方** —— 与身份那一篇实测到的"身份策略与资源策略之间"是同一个形状，也是一份只读了一份文档的审计只读了答案的一部分的原因。

### 检测与缓解

- **对账号根凭据的使用告警**，因为它不受管其他一切的策略约束。
- **报出任何授予 `*` on `*` 的策略**，以及任何含裸"一切"主体的信任策略，因为身份那一篇实测过后者放进的是什么。
- **逐字段搜资源写法** —— 分区、服务、区域、账号、路径 —— 因为实测账号字段上的通配符跨账号授予。
- **对新增的跨账号信任、以及对角色信任策略的改动告警**，因为那是决定"谁可以成为它"的那份文档。
- **找出长期访问密钥并用角色替换**，因为角色的凭据会过期，而密钥不会。
- **对能创建其他身份的身份强制多因素认证**，因为创建一个身份就是成为它的一个办法。
- **对账号级公开访问开关的改动、以及密钥策略被编辑告警**，因为两者都是压过下面那些文档的决定。
- **缓解上，用角色代替密钥、用临时凭据代替两者**，这样一份被泄露的凭据有一个寿命。
- **在平台提供的地方把天花板放在账号之上**，因为一条能拒绝的策略是账号自身配置出错时唯一还成立的那条。
- **给每个工作负载自己的角色、而不是共用一个**，因为实测那个会话身份才是动作被归因到的东西。
- **对象存储的两套授予机制、以及它们之上的开关都要查**，因为两者都能授予公开访问，而只有那些开关能阻止它。
- **并且像读一段 IP 范围那样读资源写法**，因为实测"覆盖对象"与"覆盖桶"之间的差别是两个字符。
