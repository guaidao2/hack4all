---
id: cloud-compute-and-metadata
title_en: Cloud Compute and Instance Metadata
title_zh: 云主机与实例元数据
summary_en: Every instance carries an address that only the instance can reach, and that address hands out the instance's credentials to whoever asks. Measured against a simulated metadata service with request primitives of different abilities — one GET being enough for the older mode, a PUT plus a custom header being needed for the newer one, and a hop limit that defeats the proxied case either way.
summary_zh: 每一台实例都带着一个只有它自己够得到的地址，而那个地址会把实例的凭据交给任何开口要的人。这一篇对着一个模拟的元数据服务、用能力不同的请求原语实测 —— 旧模式一个 GET 就够、新模式需要一个 PUT 加一个自定义头，以及一条对两种模式都有效的跳数限制。
tags: [beginner, cloud, compute, imds, ssrf]
tools: [curl, aws, python3]
attck: [T1552.005, T1190]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### An address that only the machine can reach

A cloud instance is three things at once: a machine, a network position, and **an identity**. The identity is the part that makes this entry different from anything about servers, and it is delivered through an address that answers only from inside.

> **The metadata service has no authentication, because its design assumption is that reaching the address means you are the instance.**

Everything in this entry follows from that assumption and from where it stops holding: a request crafted by an application, a container sharing a network namespace, or a proxy that forwards one.

Measured against a metadata service built for the purpose, with request primitives of deliberately different abilities.

### Part 1: the shape of it

| Element | What it is |
|---|---|
| instance | a machine, with a role attached |
| **instance profile** | the container that lets an instance carry a role |
| the credential | temporary, expiring, **delivered by the metadata service** |
| the address | a link-local address, reachable only from the instance |
| bootstrap data | the **startup script**, readable through the same service |
| the disk | volumes and snapshots, which are a separate access question |

**And the role is what makes the credential worth stealing.** The permissions attached to the instance profile are the permissions of whatever code can reach that address — **so the blast radius of a request forgery on an instance is the policy of the role, not the application's own reach.**

**Which is the sentence to carry into every entry on server-side request forgery**: on a web server it is a way to read internal pages; on a cloud instance it is **a way to become the instance.**

### Part 2: why the service exists, and what it assumes

The problem it solves is real and worth stating, because it explains the design. Code running on the instance needs credentials to call the provider's APIs. **Putting a long-lived key on the disk means a key that is hard to rotate and easy to find in an image.** So the provider gives the instance an identity through its own hypervisor, and the code asks the hypervisor for a credential — which is temporary, rotated automatically, and never written by the operator.

**And the way the hypervisor is reached is a network address**, because that was the simplest available channel. Which produces the assumption:

> **Anything that can send a request to the address is treated as the instance, because the design has no other way to tell.**

**And that assumption holds in three configurations and fails in three others**, which is the whole of the rest of this entry:

| Holds | Fails |
|---|---|
| code running on the instance itself | an application tricked into fetching a supplied URL |
| the instance's own agent | a container sharing the host's network namespace |
| a process with the instance's normal privileges | anything that can proxy a request through it |

### Part 3: two modes, and what each of them actually requires

The newer mode exists because a single `GET` was too easy. Measured, the same credential-gathering sequence run against both modes by primitives with different abilities:

| What the primitive can do | Older mode | Newer mode enforced |
|---|---|---|
| **send a `GET` only, no custom headers** | **credential obtained** | **failed — the primitive cannot `PUT`** |
| send any method, no custom headers | **credential obtained** | **failed — a required header is missing** |
| send any method and set custom headers | **credential obtained** | **credential obtained** |
| the above, but through one extra hop | **failed — the response did not survive the hop** | **failed — the response did not survive the hop** |

**Read the first row as the difference the new mode makes.** In the older mode, a primitive that can only fetch a URL is enough. In the newer mode it is not, because the sequence requires **two things a simple fetch does not have**: a `PUT`, and a header whose value the client chooses.

**And read the third row as the limit.** A primitive with control over the method and the headers gets the credential anyway — **so the new mode is not a defence against request forgery; it is a defence against the simplest kind of it.**

> **The older mode's requirement is "can you send a GET". The newer mode's requirement is "can you send a PUT and set a header". The difference matters exactly when the primitive is less capable than a full HTTP client.**

**And the fourth row is a different mechanism entirely.** The hop limit makes the metadata service's responses unable to survive an extra network hop, so a request that arrives through a proxy — including a containerised workload behind NAT — **gets no answer at all**, in either mode. **That is what the setting is for, and it is independent of the mode.**

**Which gives the configuration that composes**: newer mode **and** a hop limit of one. The mode raises the cost of the simplest attack; the hop limit removes the whole class that goes through something else.

### Part 4: the address has more than one spelling

Measured, the forms that resolve to the same host:

| Written as | Resolves to |
|---|---|
| `127.0.0.1` | `127.0.0.1` |
| `127.1` | `127.0.0.1` |
| `2130706433` | `127.0.0.1` |
| `0x7f000001` | `127.0.0.1` |
| `0177.0.0.1` | `127.0.0.1` |
| `127.0.0.2` | a different address |

**Five spellings, one address**, and the same is true of the link-local address a cloud instance uses. **So a rule that refuses the string refuses one spelling of it**, which is the measurement from the webhook entry arriving again with a different address:

> **Blocking an address by its text blocks the wording. The address has several.**

**And a name can point at it too**, which is why the check belongs after resolution. **The sequence that works is: resolve, validate the resolved value, and connect to that same value** — because a check that resolves and a connection that resolves again are two lookups, and the answer can differ.

### Part 5: what else is at the address

Measured, two more paths on the same service:

```
GET /latest/meta-data/      -> ami-id, hostname, iam/, instance-id
GET /latest/user-data       -> #!/bin/sh
                               export DB_PASSWORD='prod-db-password'
```

**The first is descriptive and useful to an attacker** — it says which account, which instance, which identity, and confirms what the machine is.

**And the second is the one that turns a data leak into a credential leak without needing the credential path at all.** The startup script is where operators put what the machine needs at first boot, and what a machine needs at first boot is often a password:

> **Whatever is in the startup script is readable by anything that can send that request — and the startup script is the most convenient place to put a secret, which is what makes it the worst.**

**Which means the two paths have to be considered together.** An instance with the newer mode enforced protects the credential path and does nothing about the script, and a deployment that hardens one and not the other has hardened half.

**And the same reasoning applies to the disk.** A snapshot of the instance's volume contains whatever the running system had written, including anything a script wrote before the operator removed it — the shape measured in the entries on object storage versions and on repository history.

### Part 6: the other things attached to an instance

**The management agent is a second channel.** An agent that accepts commands from the provider is also an identity, and one whose permissions are often broader than the application's.

**And the volumes are separate.** An instance's role governs the API; a volume or snapshot has its own access control, and a snapshot shared or copied carries the data without the running instance.

**And the network position is part of the identity.** A security group that allows an unusual port inbound turns the instance into a place an attacker can start from, and the metadata service turns that start into the instance's own credentials.

**And the log is where attribution lives.** Every action taken with a temporary credential is logged against the role session, so **an incident beginning with a request forgery ends with a session whose actions are all attributable to that instance** — which is useful for the response and also tells the attacker to work quickly.

### Part 7: the same service from inside a container

**A container that shares the host's network namespace can reach the metadata address**, and the workload inside it has whatever network capabilities the container runtime gave it. **That is why the hop limit matters**: a containerised request that exits the namespace and returns is a request that traversed a hop.

**And managed container platforms provide their own endpoint** for the same purpose, with a different address and often a different credential model — **which means "we disabled the instance metadata service" is a statement about one endpoint rather than about credential exposure.**

**And the workload identity question is the same one**: the credential is issued to a workload, it is reachable by whatever can send a request, and the permissions are the ones attached at the platform level.

### Part 8: what follows for security

**A request forgery on a cloud instance is a credential theft, and it should be handled as one.** Measured, a single `GET` sufficed in the older mode. **So the response to finding one is not to fix the fetch and move on**: it is to enumerate what the instance's role could do, to look at what was done with it, and to treat the credentials as disclosed.

**And the two modes are the difference between one capability and two.** Measured, the newer mode failed for a primitive that could only `GET`, and for one that could send any method but not a header — and succeeded for one with full control. **So enforcing it is a real reduction and not a solution**, and the entries on request forgery remain the place where the input is stopped.

**The hop limit is the setting that removes a class rather than raising a cost**, because it makes the response unable to survive an extra hop. **It is also the one that breaks workloads that legitimately proxy**, which is why it is a decision with a consequence rather than a switch to turn on.

**The startup script is a credential store that is often forgotten.** Measured, it returned a database password. **The control is to keep secrets out of it entirely** — the instance's role can read a secret store, which means the script can name a secret instead of containing one, and the difference is which thing a request forgery obtains.

**And the permissions on the instance's role are the real bound.** Measured, the credential obtained is the instance's; **what it can do is whatever the policy allows**, so the smallest role that works is the difference between a leaked credential that reads one bucket and one that reads the account.

**And the address having several spellings means the defence is not a deny-list.** Measured, five forms resolved to one address. **The sequence that works is to resolve, validate the resolved value against the ranges that must not be reachable, and connect to that value** — with redirects disabled, as the entries on gateways and webhooks measured the same gap.

**And egress is the boundary that makes the rest optional.** An instance that can only reach the addresses its application needs **cannot fetch anything an attacker names**, which is why an allow-list of destinations is worth more than any block-list of them — and why "where can this machine connect" belongs in the review alongside "what can this role do".

### Detection and mitigation

- **Alert on requests to the metadata address from processes that should not make them**, and on any request to the startup script path from an application rather than from provisioning.
- **Alert on credentials being used from outside the instance's network position**, since a stolen instance credential is used from wherever the attacker runs.
- **Watch the audit log for the actions of a role session whose volume or variety looks unlike its normal use**, because the session is what the actions are attributed to.
- **Check every instance's mode and hop limit**, and treat a fleet with mixed settings as a fleet where the older mode is still reachable somewhere.
- **Check what is in every startup script**, and treat a secret there as a credential to rotate rather than a configuration to tidy.
- **Report instance roles that can read broadly**, since the measured blast radius of a request forgery is that policy.
- **For mitigation, enforce the newer mode and a hop limit of one together**, and accept the workload changes the hop limit requires rather than leaving it off.
- **Keep secrets out of startup data** by having the instance read them from a secret store using its own role.
- **Validate destinations after resolution and connect to the validated address**, with redirects disabled and egress restricted to what the workload uses.
- **Give the instance role the smallest policy that works**, and review it as the bound on what a forged request obtains.
- **Treat a found request forgery as a credential incident**: rotate, enumerate the role's permissions, and read the audit trail for the window.
- **And remember the workload platforms have their own endpoint.** Hardening the instance's metadata service is one decision, and the credential model of whatever runs on it is another.

<!-- lang:zh -->
### 一个只有这台机器够得到的地址

一台云实例同时是三样东西：一台机器、一个网络位置、以及**一个身份**。身份才是让这一篇区别于任何关于服务器的内容的那一样，而它通过一个只从内部应答的地址交付。

> **元数据服务没有认证，因为它的设计假设是：能到达那个地址，就意味着你是这台实例。**

这一篇里的一切都由那个假设推出，也由它在哪些地方不再成立推出：一个由应用构造的请求、一个共享网络命名空间的容器、或者一个替它转发的代理。

下面对着一个为此搭起来的元数据服务实测，用的请求原语在能力上被刻意做出差别。

### 第一部分：它的形状

| 元素 | 它是什么 |
|---|---|
| 实例 | 一台机器，挂着一个角色 |
| **实例配置文件** | 让一台实例能携带一个角色的那个容器 |
| 凭据 | 临时的、会过期的、**由元数据服务交付** |
| 地址 | 一个链路本地地址，只有实例自己够得到 |
| 启动数据 | **启动脚本**，通过同一个服务可读 |
| 磁盘 | 卷与快照，那是另一个访问问题 |

**而角色才是让那份凭据值得偷的东西。** 挂在实例配置文件上的权限，就是任何能到达那个地址的代码的权限 —— **所以一台实例上的一次请求伪造，波及范围是那个角色的策略，而不是应用自己的可达范围。**

**这就是该带进每一篇关于服务端请求伪造的内容里的那句话**：在一台 Web 服务器上，它是一条读内网页面的路；在一台云实例上，它是**一条变成这台实例的路**。

### 第二部分：这个服务为什么存在，以及它假设了什么

它解决的问题很实在、也值得说清，因为它解释了那个设计。跑在实例上的代码需要凭据去调用服务商的 API。**把一把长期密钥放在磁盘上，意味着那是一把难以轮换、又容易在镜像里被找到的密钥。** 于是服务商通过自己的虚拟机监控程序给实例一个身份，代码向那个监控程序要一份凭据 —— 那是临时的、自动轮换的、而且从不由运维写下来。

**而够到那个监控程序的方式是一个网络地址**，因为那是最简单的可用通道。于是就有了那个假设：

> **任何能向那个地址发出请求的东西，都会被当成这台实例，因为这个设计没有别的办法区分。**

**而那个假设在三种配置下成立、在另外三种下不成立**，这就是这一篇剩下部分的全部：

| 成立 | 不成立 |
|---|---|
| 跑在实例自身的代码 | 一个被诱骗去取一个被提供的 URL 的应用 |
| 实例自己的管理代理 | 一个共享宿主网络命名空间的容器 |
| 一个拥有实例正常权限的进程 | 任何能把一个请求透过它转发出去的东西 |

### 第三部分：两种模式，各自到底要求什么

新模式之所以存在，是因为一个 `GET` 太容易了。实测，同一段收集凭据的流程，交给能力不同的原语去打两种模式：

| 原语能做到什么 | 旧模式 | 新模式被强制 |
|---|---|---|
| **只能发一个 `GET`、不能设自定义头** | **拿到凭据** | **失败 —— 原语发不出 `PUT`** |
| 能发任意方法、不能设自定义头 | **拿到凭据** | **失败 —— 缺一个必需的请求头** |
| 能发任意方法、也能设自定义头 | **拿到凭据** | **拿到凭据** |
| 同上，但经过一跳中转 | **失败 —— 响应没能活过那一跳** | **失败 —— 响应没能活过那一跳** |

**把第一行当作新模式带来的差别来读。** 在旧模式下，一个只能取 URL 的原语就够了。在新模式下不够，因为那个流程要求**一个简单取件不具备的两样东西**：一个 `PUT`，以及一个由客户端决定其值的请求头。

**而第三行要当作那个上限来读。** 一个能控制方法与请求头的原语照样拿到凭据 —— **所以新模式不是对请求伪造的防御，它是对最简那种的防御。**

> **旧模式的要求是"你能不能发一个 GET"。新模式的要求是"你能不能发一个 PUT 并设一个头"。这个差别只在原语比一个完整 HTTP 客户端更弱的时候才起作用。**

**而第四行完全是另一个机制。** 跳数限制让元数据服务的响应无法活过一次额外的网络跳，所以一个经由代理到达的请求 —— 包括一个在 NAT 之后的容器化工作负载 —— **根本得不到回答**，两种模式都一样。**那就是这个设置的用途，而它与模式无关。**

**这就给出了那个组合起来的配置**：新模式**加上**跳数限制为 1。模式抬高的是最简攻击的成本；跳数限制移除的是"经过别的东西"那一整类。

### 第四部分：那个地址不止一种写法

实测，解析到同一个主机的几种形式：

| 写成 | 解析为 |
|---|---|
| `127.0.0.1` | `127.0.0.1` |
| `127.1` | `127.0.0.1` |
| `2130706433` | `127.0.0.1` |
| `0x7f000001` | `127.0.0.1` |
| `0177.0.0.1` | `127.0.0.1` |
| `127.0.0.2` | 另一个地址 |

**五种写法、一个地址**，而云实例所用的那个链路本地地址也一样。**所以一条拒绝某个字符串的规则，拒绝的只是它的一种写法** —— 这就是 webhook 那一篇的实测换了一个地址再次出现：

> **按文本拦一个地址，拦的是措辞。而那个地址有好几种。**

**而一个域名也可以指向它**，这就是为什么那个检查要放在解析之后。**可行的顺序是：解析、校验解析出来的值、然后连到同一个值** —— 因为"做了检查的解析"与"建立连接时的解析"是两次查找，而答案可以不同。

### 第五部分：那个地址上还有什么

实测，同一个服务上的另外两条路：

```
GET /latest/meta-data/      -> ami-id、hostname、iam/、instance-id
GET /latest/user-data       -> #!/bin/sh
                               export DB_PASSWORD='prod-db-password'
```

**第一条是描述性的、而且对攻击者有用** —— 它说出是哪个账号、哪台实例、哪个身份，也确认了这台机器是什么。

**而第二条才是把一次数据泄漏变成一次凭据泄漏的那条路，而且它根本不需要走凭据那条路。** 启动脚本是运维放"这台机器开机时需要的东西"的地方，而一台机器开机时需要的东西往往就是一个口令：

> **启动脚本里的每一行，对一个能发出那个请求的人都是可读的 —— 而启动脚本是把一个秘密放进去最方便的地方，这正是它成为最糟地方的原因。**

**这意味着两条路必须一起考虑。** 一台强制了新模式的实例保护了凭据那条路，而对那个脚本什么都没做；一个只加固了其中一条的部署，只加固了一半。

**而同样的推理适用于磁盘。** 一个实例卷的快照里含着那个运行中的系统写下过的一切，包括某个脚本写进去、之后被运维删掉的东西 —— 这个形状在对象存储版本与代码仓库历史那两篇里都实测过。

### 第六部分：挂在一台实例上的其他东西

**管理代理是第二个通道。** 一个接受服务商命令的代理也是一个身份，而它的权限往往比应用的更宽。

**而卷是分开的。** 一台实例的角色管着 API；一个卷或者快照有自己的访问控制，而一个被共享或复制的快照带着数据走，不需要那台运行中的实例。

**而网络位置是身份的一部分。** 一个允许不常见端口入站的安全组，把这台实例变成了攻击者可以出发的地方，而元数据服务把那个出发点变成这台实例自己的凭据。

**而日志才是归因所在。** 每一件用临时凭据做的事，都被记在那个角色会话名下，所以**一次从请求伪造开始的事件，结束于一个会话，而它的每一个动作都可以归因到那台实例** —— 这对处置有用，同时也告诉攻击者要快。

### 第七部分：从容器里看同一个服务

**一个共享宿主网络命名空间的容器够得到元数据地址**，而里面的工作负载拥有容器运行时给它的那些网络能力。**这就是跳数限制要紧的原因**：一个离开命名空间又回来的容器化请求，就是一个经历过一跳的请求。

**而托管容器平台为同一个目的提供了自己的端点**，地址不同、凭据模型往往也不同 —— **这意味着"我们把实例元数据服务关掉了"是一句关于某一个端点的话，而不是关于凭据暴露面的话。**

**而工作负载身份的问题是同一个**：凭据发给一个工作负载，任何能发出请求的东西都够得到它，而权限是平台层面挂上去的那些。

### 第八部分：从这些机制推出的安全观念

**一台云实例上的请求伪造是一次凭据窃取，而它就该按凭据事件来处置。** 实测，旧模式下**一个 GET 就够**。**所以发现它之后的应对不是把那个取件修好就完了**：要清点那台实例的角色能做什么、看那些权限被用来做了什么、并把凭据当成已泄露。

**而两种模式之间的差别是一个能力与两个能力。** 实测，新模式对一个只能 `GET` 的原语失败、对一个能发任意方法但设不了头的原语也失败，而对一个拥有完全控制权的原语成功。**所以强制它是一种真实的收窄、而不是一个解决方案**，而"输入在哪里被挡住"仍然属于讲请求伪造的那几篇。

**跳数限制是那个"移除一类、而不是抬高成本"的设置**，因为它让响应无法活过一次额外的跳。**它也是那个会弄坏合法的代理型工作负载的设置**，这就是为什么它是一个有后果的决定，而不是一个打开就完事的开关。

**启动脚本是一个常被忘掉的凭据存储。** 实测，它返回了一个数据库口令。**控制手段是彻底不让秘密出现在里面** —— 实例的角色能读一个密钥服务，这意味着那个脚本可以点出一个秘密的名字、而不是含着它，而两者的差别在于一次请求伪造能拿到什么。

**而实例角色上的权限才是真正的边界。** 实测，拿到的那份凭据就是这台实例的；**它能做什么完全取决于那条策略**，所以"能干活的最小角色"就是"一份泄露的凭据只能读一个桶"与"能读整个账号"之间的差别。

**而那个地址有多种写法，意味着防御不是一份黑名单。** 实测，五种形式解析到同一个地址。**可行的顺序是解析、按那些必须够不到的网段校验解析出来的值、然后连到那个值** —— 并且关掉重定向，因为网关与 webhook 那两篇实测过同一道缝。

**而出网是那个让其余措施变成可选的边界。** 一台只能到达它应用所需地址的实例，**取不到任何由攻击者命名的东西**，这就是为什么一份目标白名单比任何黑名单都更值钱 —— 也是为什么"这台机器能连到哪里"该和"这个角色能做什么"一起进评审。

### 检测与缓解

- **对"来自不该发出请求的进程、打向元数据地址"的请求告警**，以及对"来自应用、而不是来自置备流程、打向启动脚本路径"的请求告警。
- **对"凭据从这台实例的网络位置之外被使用"告警**，因为被偷的实例凭据是从攻击者运行的地方被使用的。
- **盯审计日志里那些"用量或种类与该角色平常不一样的会话所做的事"**，因为那些动作被归因到的就是那个会话。
- **查每一台实例的模式与跳数限制**，并把一个设置混杂的机群当成"旧模式在其中某处仍然可达"的机群。
- **查每一份启动脚本里有什么**，并把那里的一个秘密当成一份要轮换的凭据，而不是一处要收拾的配置。
- **报出能读得很宽的实例角色**，因为一次请求伪造实测到的波及范围就是那条策略。
- **缓解上，把新模式与跳数限制为 1 一起强制**，并接受跳数限制所要求的工作负载改动、而不是把它关掉。
- **不让秘密出现在启动数据里**，做法是让实例用它自己的角色去一个密钥服务读。
- **在解析之后校验目标、并连到那个被校验过的地址**，同时关掉重定向、把出网限制在工作负载真正用的那些上。
- **给实例角色能干活的最小策略**，并把那条策略当成"一次被伪造的请求能拿到什么"的边界来评审。
- **把发现的一次请求伪造当作一次凭据事件来处置**：轮换、清点那个角色的权限、读那个时间窗的审计记录。
- **并且记住工作负载平台有自己的端点。** 加固实例的元数据服务是一个决定，而跑在它上面的东西的凭据模型是另一个。
