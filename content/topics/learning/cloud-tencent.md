---
id: cloud-tencent
title_en: Tencent Cloud Service Map and Identity Model
title_zh: 腾讯云服务地图与身份模型
summary_en: The same problems solved with different placements — a signature whose derivation chain takes the date and the service and leaves the region to a signed header, resource names whose fields line up with another platform's but whose contents do not, and a metadata endpoint that is a name rather than an address. Measured by implementing the derivation chain and comparing it with the one it is most often confused with.
summary_zh: 同样几个问题，东西放的位置不同 —— 一个派生链只取日期与服务、把区域留给一个被签的头的签名；一批字段位置与另一家对得上、内容却对不上的资源名；以及一个用域名而不是地址写的元数据端点。这一篇把派生链实现出来，并拿它最容易混淆的那一条对照。
tags: [beginner, cloud, tencent, cam, tc3]
tools: [tccli, coscmd, python3]
attck: [T1078.004, T1098.001]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The same problems, placed differently

Every large platform solves the same handful of problems: how to name a resource, how to sign a request, how to express a permission, and where a workload gets its credentials. **What differs is where each answer is put**, and this entry is organised around four placements that are easy to get wrong by analogy.

> **A field's position in a scheme decides what changing it does. A name's fields decide whether a pattern matches. An endpoint's form decides what kind of control can block it.**

All four are measured below.

### Part 1: the map

| Area | The services that define it |
|---|---|
| identity | the account, its sub-accounts, groups, roles and policies |
| compute | virtual machines, container services, functions |
| storage | object storage, block storage, file storage |
| network | virtual networks, security groups, load balancing, DNS |
| data | managed relational, key-value, warehouse, streaming |
| operations | action trail, logs, monitoring, deployment |
| security | key management, certificate management, security centre |

**And the account is the boundary**, identified by a number that appears in resource names and in policies. **As on the other platforms, that number is not a secret** — it establishes which doors exist rather than who may open them.

### Part 2: the identity model

**Sub-accounts, groups, roles and policies**, in the shape this family of platforms shares: a principal is granted policies, a policy is a list of statements, and a statement names actions and resources.

**And roles exist for workloads as well as for cross-account access**, which means a virtual machine can be given an identity — **with the whole of the analysis from the entry on instance metadata applying, including the part about what a request forgery on that machine obtains.**

**And policies come in two kinds**, one written by the platform and one written by the customer, **which makes the preset policies a dependency worth tracking**: a broad preset attached for convenience is the same finding as a broad custom policy, and it is harder to notice because nobody wrote it.

### Part 3: the policy language can refuse, and the refusal is narrow

Measured, a policy that allows an entire service and then subtracts one operation on one prefix:

| Request | Outcome |
|---|---|
| read an object under the denied prefix | **refused — the explicit denial wins** |
| read an object elsewhere in the bucket | allowed |
| **write an object under the denied prefix** | **allowed** |
| read an object under a prefix differing only in case | allowed |

**The third row is the one worth reading twice.** The denial named one operation, **so every other operation on the same prefix is unaffected** — including the one that overwrites what the denial was protecting.

> **A denial's granularity is the pair of an action and a resource. Narrowing one operation does not touch another, so "we locked that bucket down" has to say which operations it locked.**

**And the fourth row is the same class of mistake measured on another platform**: the effect value is matched case-insensitively while the resource name is matched as a string, **so the one field that is forgiving and the one that is exact are the two fields a reader treats the same way.**

### Part 4: resource names whose fields line up

Measured, several resource names split into their fields:

| Resource name | Service | Region |
|---|---|---|
| `qcs::cvm:ap-guangzhou:uin/1234567890:instance/i-abc123` | cvm | ap-guangzhou |
| `qcs::cos:ap-guangzhou:uid/1234567890:bucket-1250000000/obj/key.txt` | cos | ap-guangzhou |
| `qcs::cam::uin/1234567890:roleName/app-role` | cam | empty — global |

**The field order is the same as another platform's**, and the prefix and the account field are not: **the account appears as `uin/` or `uid/` followed by the number**, in one case with an owner number and a bucket name carrying its own identifier.

> **Two platforms can put the same fields in the same places and still not accept each other's names, because the contents of a field are not the field's position.**

**Which is the practical warning**: a pattern copied by analogy — the right number of colons, the right field order — **matches nothing, and the failure is a silence.** The check that works is to take a real resource name out of an audit log and test the pattern against it.

### Part 5: what goes into a derived key

The signing scheme here derives a key from the secret before signing, and the chain is worth comparing with the one it is most often confused with.

| | The chain, in order |
|---|---|
| **this platform** | prefix + secret → **date** → **service** → final string |
| another platform | prefix + secret → **date** → **region** → **service** → final string |

**Measured, the difference shows up immediately**: changing the region produces **the same derived key** here, and a different one there.

**And the signature still changes when the region changes**, because the region reaches it by another route — as a **signed header**, which is part of the canonical request.

> **A parameter is constrained if it appears somewhere in what the signature covers. Whether that somewhere is the derived key or a signed header changes which implementation produces the same signature, not whether the parameter matters.**

**Which is the useful way to compare two signing schemes**: not "which algorithm", but **which inputs are in the chain and which are in the covered set** — because that is what decides whether editing a field invalidates a request.

**And the chain's length is the number of dimensions built into the key.** Measured, the two chains differ by one stage, **and that stage is exactly the region** — so an implementation written for one scheme produces a value that never verifies against the other, with no error explaining why.

### Part 6: the covered set is a choice

Measured, the same request signed with two different header sets:

| Headers covered | Signature |
|---|---|
| content type and host | one value |
| the same, plus a timestamp header | **a different value** |

**Both can describe the same request**, and the second says that the timestamp may not be altered without invalidating it while the first permits anything to be done with it.

> **The set of covered headers is a decision, and whatever is outside it can be changed by anybody.**

**Which is the same conclusion the entries on webhooks and on storage reached from different protocols**: the signature's boundary is the set it covers, and reading a signature scheme means finding that set rather than reading the algorithm's name.

### Part 7: a metadata endpoint that is a name

The address a workload asks for its credentials has a different form here: **it is a hostname rather than an address.**

Measured, the difference that form makes:

| Endpoint | With a resolver | Without one |
|---|---|---|
| a numeric address | reachable — no lookup needed | **reachable — still no lookup needed** |
| **a hostname** | reachable, after resolving to an address | **not reachable — the name cannot become an address** |

**So a control that blocks a name does nothing about an address, and a workload with no working resolver does not know where to ask.**

> **The form of an endpoint decides which kind of control can act on it. A name can be blocked by a resolver policy; an address has to be blocked in the network.**

**And this is the detail most likely to be carried over wrongly from another platform**, because the address is memorable and the name is not, **and a rule written for one of them is silently inapplicable on the other.**

### Part 8: what follows for security

**Put the same questions to every platform and expect different answers to arrive in different places.** Measured: the region of a request is inside the derived key on one platform and in a signed header on another; a resource name's fields line up while their contents do not; a denial's granularity is a pair; and a credential endpoint is a name.

**A denial is only as wide as its action list.** Measured, denying one read operation left the write operation on the same prefix allowed. **So a policy that subtracts something has to be read as the pair of an action and a resource**, and the useful check is whether the other operations on that resource are also meant to be excluded.

**And a denial fails by not matching.** Measured, a prefix differing only in case matched nothing, and the effect field is the forgiving one while the resource field is not. **So denials belong on the list of things to verify against real requests** rather than on the list of things to read.

**Do not carry a pattern or an endpoint across platforms.** The measured name has the right number of fields and the wrong contents, and the measured endpoint is a name where another platform's is an address. **Both failures are silences**, which is why the check is against real strings from an audit log rather than against an understanding of the shape.

**And compare signing schemes by their chain and their covered set.** Measured, one chain had a stage the other did not, and one covered header set differed from another for the same request. **Neither difference produces an error message** — the signature simply does not verify — so knowing which inputs are covered is the thing worth writing down.

### Detection and mitigation

- **Alert on changes to denial statements**, since a denial is where a prohibition is expressed and its granularity is the pair of an action and a resource.
- **Check that a denial names every operation on the resource it protects**, because the measurement left a write operation available under a denied prefix.
- **Verify denial patterns against real resource names**, including the account field's form and any case differences, since the measured mismatch produced a silence.
- **Report presets that grant broadly**, since a preset attached for convenience is the same finding as a broad custom policy and is harder to notice.
- **Report long-lived key pairs and prefer roles**, especially for workloads that can be given an identity.
- **Check which inputs a request signature covers**, and treat anything outside that set as modifiable by whoever holds the request.
- **Find the credential endpoint's form on every platform in use**, and check the control that matches it — a resolver policy for a name, a network rule for an address.
- **For mitigation, express prohibitions as denials and read them as action-and-resource pairs**, rather than as a statement about a resource.
- **Adapt resource patterns from real names rather than from another platform's documentation**, because the measured difference is in the contents of a field rather than its position.
- **Do not reuse a signing implementation across platforms**, since a measured chain has a stage that another one does not and the failure is a signature that never verifies.
- **Track the difference between the platforms in use as a table of placements** — signature chain, resource fields, denial support, endpoint form — because every one of them is a place where knowledge from one is wrong on another.
- **And treat the credential endpoint as an exposure surface that changes with the platform**, since the measured difference between a name and an address is the difference between two kinds of control.

<!-- lang:zh -->
### 同样几个问题，东西放的位置不同

每一家大平台都在解同样那几个问题：怎么命名一个资源、怎么给一个请求签名、怎么表达一条权限、以及一个工作负载从哪里取得凭据。**不同的是每个答案被放在哪里**，而这一篇就围绕四处容易靠类比弄错的位置来组织。

> **一个字段在方案里的位置，决定了改动它会带来什么。一个名字的字段，决定了某个写法能不能匹配上。一个端点的形式，决定了哪一类控制能挡得住它。**

四处都在下面实测。

### 第一部分：地图

| 区域 | 定义它的那些服务 |
|---|---|
| 身份 | 账号、它下面的子账号、组、角色与策略 |
| 计算 | 虚拟机、容器服务、函数 |
| 存储 | 对象存储、块存储、文件存储 |
| 网络 | 虚拟网络、安全组、负载均衡、DNS |
| 数据 | 托管关系库、键值、数仓、流 |
| 运维 | 操作审计、日志、监控、部署 |
| 安全 | 密钥管理、证书管理、安全中心 |

**而账号是边界**，由一个出现在资源名与策略里的数字标识。**与其他平台一样，那个数字不是秘密** —— 它确立的是"有哪些门存在"，而不是"谁能开门"。

### 第二部分：身份模型

**子账号、组、角色与策略**，形状是这一族平台共有的：一个主体被授予若干策略，一个策略是一串语句，一条语句点名若干动作与资源。

**而角色既为工作负载存在、也为跨账号访问存在**，这意味着**一台虚拟机可以被赋予一个身份** —— 讲实例元数据那一篇的全部分析都适用，包括"那台机器上的一次请求伪造能拿到什么"那部分。

**而策略有两种**，一种由平台写、一种由客户写，**这就让预设策略成为一项值得追踪的依赖**：为图方便挂上去的一条宽预设，与一条宽的自定义策略是同一条发现，而它更难被注意到，因为不是谁写的。

### 第三部分：策略语言能拒绝，而那个拒绝很窄

实测，一条允许整个服务、然后减掉某一个前缀上某一个操作的策略：

| 请求 | 结果 |
|---|---|
| 读被拒绝的前缀下的一个对象 | **拒绝 —— 显式拒绝优先** |
| 读那个桶里别处的对象 | 允许 |
| **写被拒绝的前缀下的一个对象** | **允许** |
| 读一个只差大小写的前缀下的对象 | 允许 |

**第三行值得读两遍。** 那条拒绝点名的是一个操作，**所以同一个前缀上的其他操作一律不受影响** —— 包括那个会覆盖掉这条拒绝本来想保护的东西的操作。

> **一条拒绝的粒度是"动作与资源"这一对。收窄一个操作不会碰到另一个，所以"我们把这个桶锁上了"必须说明锁的是哪些操作。**

**而第四行是在另一个平台上实测过的同一类错误**：效果值是大小写不敏感匹配的，而资源名是按字符串匹配的 —— **于是"宽容的那个字段"与"精确的那个字段"，正是读的人会同样对待的那两个字段。**

### 第四部分：字段对得上的资源名

实测，几个资源名按字段拆开：

| 资源名 | 服务 | 区域 |
|---|---|---|
| `qcs::cvm:ap-guangzhou:uin/1234567890:instance/i-abc123` | cvm | ap-guangzhou |
| `qcs::cos:ap-guangzhou:uid/1234567890:bucket-1250000000/obj/key.txt` | cos | ap-guangzhou |
| `qcs::cam::uin/1234567890:roleName/app-role` | cam | 空 —— 全局 |

**字段顺序与另一家一样**，而前缀与账号字段不一样：**账号以 `uin/` 或 `uid/` 加上那个数字的形式出现**，有些情形下还带一个所有者编号、以及一个自带标识的桶名。

> **两个平台可以把同样的字段放在同样的位置上，却仍然不接受对方的名字，因为一个字段的内容不是这个字段的位置。**

**这就是那条实用的提醒**：一个照着类比抄来的写法 —— 冒号数量对、字段顺序对 —— **什么都匹配不上，而那个失败是一次静默。** 行得通的检查是从审计日志里取一个真实的资源名，拿那个写法去试。

### 第五部分：派生密钥里放什么

这家的签名方案在签名之前先从秘密派生一把密钥，而那条派生链值得与它最常被混淆的那一条对照。

| | 派生链，依次 |
|---|---|
| **这家** | 前缀 + 秘密 → **日期** → **服务** → 最后一段 |
| 另一家 | 前缀 + 秘密 → **日期** → **区域** → **服务** → 最后一段 |

**实测，这个差别立刻显出来**：换一个区域，在这家得到的是**同一把派生密钥**，在那家不是。

**而换区域时签名仍然会变**，因为区域从另一条路到达签名 —— 作为一个**被签的请求头**，它是规范请求的一部分。

> **一个参数只要出现在签名覆盖的东西里的某一处，它就是受约束的。那一处在派生密钥里还是在被签的头里，改变的是"哪种实现能算出同一个签名"，而不是这个参数重不重要。**

**这是比较两种签名方案有用的方式**：不是问"哪个算法"，而是问**哪些输入在链里、哪些在被覆盖的集合里** —— 因为那才决定"改一个字段会不会让请求失效"。

**而链的长度就是做进密钥里的维度数。** 实测，两条链差一个环节，**而那一个环节恰好是区域** —— 所以为一种方案写的实现算出来的值，在另一种下永远验不过，而且没有任何错误信息解释为什么。

### 第六部分：被覆盖的集合是一个选择

实测，同一个请求用两套不同的头集合签名：

| 被覆盖的头 | 签名 |
|---|---|
| 内容类型与主机 | 一个值 |
| 同上，再加一个时间戳头 | **另一个值** |

**两者可以描述同一个请求**，而第二个说明那个时间戳被改动会让签名失效，第一个则允许对它做任何事情。

> **被覆盖的头集合是一个决定，而那个集合之外的东西谁都能改。**

**这与讲 webhook 与讲对象存储那两篇从不同协议得出的结论是同一条**：签名的边界就是它覆盖的那个集合，而读一个签名方案意味着找出那个集合、而不是读算法的名字。

### 第七部分：一个用域名写的元数据端点

一个工作负载索取凭据的那个地址在这里是另一种形式：**它是一个域名，而不是一个地址。**

实测，这个形式带来的差别：

| 端点 | 有解析器时 | 没有解析器时 |
|---|---|---|
| 一个数字地址 | 可达 —— 不需要任何查找 | **可达 —— 仍然不需要查找** |
| **一个域名** | 可达，但先要解析成地址 | **不可达 —— 那个名字变不成地址** |

**所以一条挡住域名的控制对一个地址毫无作用，而一台没有可用解析器的工作负载根本不知道去哪儿问。**

> **一个端点的形式决定了哪一类控制能作用于它。域名可以被解析器策略挡住；地址必须由网络来挡。**

**而这是最容易被从别的平台错误搬运过来的那个细节**，因为地址好记而域名不好记，**而为其中一种写下的规则在另一种上会静默地不适用。**

### 第八部分：从这些机制推出的安全观念

**把同样几个问题问向每一个平台，并预期答案落在不同的地方。** 实测：一个请求的区域在一个平台里在派生密钥里、在另一个平台里在被签的头里；一批资源名的字段对得上而内容对不上；一条拒绝的粒度是一对；而一个凭据端点是域名。

**一条拒绝的宽度就是它的动作清单。** 实测，拒绝一个读操作之后，同一个前缀上的写操作仍然允许。**所以一条做减法的策略必须被读成"动作与资源"这一对**，而有用的检查是：那个资源上的其他操作是不是也本该被排除。

**而一条拒绝是以"没匹配上"的方式失败的。** 实测，一个只差大小写的前缀什么都没匹配上，而效果字段是宽容的那个、资源字段不是。**所以拒绝属于"要拿真实请求去核对"的那份清单**，而不是"读一遍"的那份。

**不要把写法或者端点跨平台搬运。** 实测那个名字字段数量对、内容错，而实测那个端点在别的平台是地址、在这里是域名。**两种失败都是静默**，这就是为什么检查要对着审计日志里的真实字符串做、而不是对着"我对这个形状的理解"做。

**而比较签名方案要比它的链与它的覆盖集合。** 实测，一条链有另一条没有的环节，而同一个请求在两套覆盖集合下签名不同。**两种差别都不会产出错误信息** —— 签名就是验不过 —— 所以值得写下来的是"哪些输入被覆盖了"。

### 检测与缓解

- **对拒绝语句的改动告警**，因为拒绝是表达禁令的地方，而它的粒度是"动作与资源"这一对。
- **核对一条拒绝是否点名了它保护的资源上的每一个操作**，因为实测里被拒绝的前缀下写操作仍然可用。
- **拿真实资源名核对拒绝的写法**，包括账号字段的形式与大小写差异，因为实测那种不匹配产出的是静默。
- **报出"宽泛授予"的预设策略**，因为为图方便挂上去的预设与一条宽的自定义策略是同一条发现，而且更难被注意到。
- **报出长期密钥对并优先用角色**，尤其对那些可以被赋予身份的工作负载。
- **核对一个请求签名覆盖了哪些输入**，并把那个集合之外的任何东西当成"持有这个请求的人可以改的"。
- **查清在用平台里那个凭据端点的形式**，并检查与之相配的控制 —— 域名配解析器策略、地址配网络规则。
- **缓解上，把禁令表达成拒绝、并把它读成动作与资源的配对**，而不是一句关于某个资源的话。
- **从真实的名字去改资源写法，而不是从另一家的文档**，因为实测的差别在一个字段的内容里、不在它的位置上。
- **不要跨平台复用签名实现**，因为实测一条链有另一条没有的环节，而那个失败是一个永远验不过的签名。
- **把在用的平台之间的差异记成一张"东西放在哪"的表** —— 签名链、资源字段、是否支持拒绝、端点形式 —— 因为每一样都是"从一个平台得来的知识在另一个上会错"的地方。
- **并且把凭据端点当成一个随平台变化的暴露面**，因为实测"域名与地址"之间的差别就是两类控制之间的差别。
