---
id: cloud-storage-security
title_en: Object Storage Security
title_zh: 对象存储安全
summary_en: A filesystem replaced by an HTTP API, which makes access control a decision about one request and makes objects immutable so that a delete is a record rather than an erasure. Measured by implementing the signing and evaluating the documented rules — a presigned URL reused five times, three access switches that override each other, and a delete that leaves every version in place.
summary_zh: 一个被 HTTP API 取代的文件系统 —— 这让访问控制变成对"一个请求"的判断，也让对象不可变，于是删除是一条记录而不是一次抹除。这一篇把签名实现出来、把文档化的规则跑出来实测 —— 一条被重复使用五次的预签名 URL、三层互相覆盖的公开开关，以及一次把每个版本都留在原处的删除。
tags: [beginner, cloud, storage, s3, presigned-url]
tools: [aws, rclone, curl, python3]
attck: [T1530, T1078]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A filesystem with an HTTP API

The thing that makes object storage different from a file server is not that it is remote. It is that everything a file server does through a protocol, this does through **requests** — and a request carries its own authorisation, its own parameters, and its own signature.

> **Access control here is a decision about one HTTP request. And the data underneath is immutable, so a delete is a new record rather than a removal.**

Those two sentences produce everything in this entry: why a presigned URL is a bearer ticket, why the layers that decide public access disagree in ways that are not obvious, and why deleting a file from a versioned bucket is a statement about what the current view shows.

Measured below by implementing the signing scheme and evaluating the documented rules.

### Part 1: the shape of it

| | Here |
|---|---|
| container | a **bucket**, globally named |
| unit | an **object**, with a key that looks like a path |
| hierarchy | **none** — the slashes are part of the name |
| protocol | HTTP, with the authorisation in the request |
| mutation | objects are **replaced**, and optionally **versioned** |
| front doors | the API endpoint, a website endpoint, a distribution |

**The absence of hierarchy is the first thing to internalise**: a prefix is a naming convention, and everything that looks like a directory operation is a listing with a filter. **Which is why "the file is in that folder" is a statement about a name**, and why a policy written against a prefix is matching strings.

**And the front doors are the second.** The same objects are served through the API, through a website endpoint with its own rules, and through a distribution that may cache and rewrite. **The rules are usually written for one of them**, which is the same shape as the entry on gateways and the one on shadow endpoints.

### Part 2: a presigned URL is a bearer ticket

Access is normally decided by who is asking. A presigned URL moves that decision into the URL itself: the signer proves its own rights once, and the resulting string carries the result.

**Which makes the expiry the entire access control.** Measured, the same URL used five times in a row:

| Use | Result |
|---|---|
| first, second, third, fourth, fifth | **accepted every time** |

**There is no nonce, no binding to a client and no revocation.** Whatever holds the string during its window is treated as the signer.

> **A presigned URL is a bearer ticket with a clock. Handing it out hands out that window, and the only way to shorten it is to have made it short.**

**And the clock is the parameter, measured against the documented rule:**

| `X-Amz-Expires` | Time since signing | Result |
|---|---|---|
| 3600 | just signed | accepted |
| 3600 | 30 minutes | accepted |
| 3600 | **2 hours** | **refused — 7200 seconds exceeds the 3600-second validity** |
| 60 | 59 seconds | accepted |
| 60 | 60 seconds | accepted |

**Which means the value is chosen when the URL is generated and cannot be changed afterwards** — the expiry is inside the signature, so extending it invalidates it.

**And a presigned URL lives in a URL**, which is where the entry on authentication measured the consequences: the request line, the access log, and the `Referer` of whatever is loaded from the page.

**So the interesting question is not only how long, but where it goes.** A link that a user clicks, a link embedded in a page, and a link returned in a JSON response all end up in different places — and a presigned URL in a page's URL ends up wherever that page's requests go.

### Part 3: what the signature covers, and what it does not

A signature covers some of the request and not the rest, and the boundary is a choice made when the URL is generated. Measured, the same signed URL with things appended:

| Request | Verifier covering all parameters | Verifier covering only the signed ones |
|---|---|---|
| unmodified | accepted | accepted |
| plus `response-content-type` | **refused** | **accepted** |
| plus `response-content-disposition` | **refused** | **accepted** |
| plus an arbitrary `x-trace=1` | **refused** | **accepted** |
| path changed | refused | refused |

**The two columns disagree about every appended parameter, and the disagreement is the whole point**: the question "is this request still the one I signed" has different answers depending on which parameters were included when the signature was computed.

**And the documented behaviour is the right column**, which means:

> **Everything outside the signed set is input that nobody signed and that still reaches the service.** A parameter the service understands and the signer did not include is a parameter an attacker chooses.

**And the same fact has a benign direction that is worth knowing**, because it is where implementations break: an application that appends a cache-buster or a tracking id to its own presigned URL works against a verifier that covers only the signed set, and breaks against one that covers everything.

**And the boundary moves with the parameters that a service treats as signed.** Response-override parameters are signed by design because they change what the client receives; an arbitrary query parameter is not signed because it changes nothing the service acts on — **and the review question is which of the two a given parameter is.**

### Part 4: one more parser, one more string

Measured while appending a second parameter with a name the signature already contained: both verifiers accepted it, because each took the **first** occurrence of the name and the first one was the signed value.

**This is the fourth appearance of the same shape in this series**, and it is worth naming because the pattern is what transfers:

| Where | One input | Two readings |
|---|---|---|
| an object's JSON body | two keys with the same name | the parser's preference decides the value |
| a protobuf message | one field number twice | last wins or first wins |
| a signed webhook body | a duplicate key | whichever the receiver's parser picks |
| **a presigned URL's query string** | **a repeated parameter** | **whichever the verifier picks** |

> **Wherever a format permits a name more than once, the sender and the checker can disagree about which one is the value — and nothing in the request says which reading is intended.**

**And in a query string the mechanism is more visible than in a body**: the parameters are parsed by the framework, by the verifier, by the signature computation and by the service, and each of them can have an opinion. **The check that follows is cheap**: send a request with a repeated parameter and see whether the answer changes.

### Part 5: three switches decide whether it is public

Public access is decided by documents at two levels plus a pair of account-level settings, and the measured evaluation shows the precedence:

| Account block | Bucket block | Public ACL or policy | Outcome |
|---|---|---|---|
| off | off | none | **not readable** |
| off | off | bucket policy `Principal: "*"` | **publicly readable** |
| off | off | public ACL | **publicly readable** |
| off | **on** | bucket policy `Principal: "*"` | **not readable — the bucket switch overrides the policy** |
| **on** | off | bucket policy `Principal: "*"` | **not readable — the account switch is not overridable** |
| **on** | **on** | public ACL | **not readable** |

**Two rows carry the lesson.** A bucket policy containing `Principal: "*"` is a statement about intent, not about reachability — **the switch above it decides**. And the account-level switch is not a default that a bucket can opt out of; **it is a ceiling**.

> **"The policy says public" and "the world can read it" are two different facts, held in two different places, and neither one implies the other.**

**Which is why the useful verification is a request from outside rather than a reading of the configuration** — the same conclusion as the entry on cloud identity, where a policy could name the right action and the wrong resource shape and produce a silence.

**And the two mechanisms that grant are independent.** A bucket policy and an object ACL are different documents with different owners, and public access can come from either. **A review that checks one of them has checked half.**

### Part 6: a delete is a record, and a versioned bucket keeps everything

Measured, in a bucket with versioning on:

| Step | What a `GET` returns | What the version stack holds |
|---|---|---|
| start | the original report | `v1` |
| upload a new version | the corrected report | `v1`, `v2` |
| **delete** | **nothing — the object appears absent** | `v1`, `v2`, **`v3` a delete marker** |
| read a specific version | the original, then the corrected | unchanged |
| upload again | the new content | `v1`, `v2`, `v3`, `v4` |

**The delete did not remove anything.** It put a marker on top of the stack, and the object's "current" content is whatever the top says — which is why the same delete followed by an upload makes the object visible again.

> **"Deleted" means the current view shows nothing. The bytes are in the version history until something removes them, and the something is a lifecycle rule rather than a delete.**

**And the same is true of a plain overwrite**, which is the case people forget: uploading a corrected file leaves the previous one in place, so **an object that once contained a secret still does**.

**Which ties this entry to the one on keys and repositories**, where a commit removed a value from the working tree and left it in history. **The shape is identical and the lesson is the same**: a system that keeps versions keeps the leak, and the response is to remove the version and to treat the old credential as compromised.

**And the storage-specific half is that removal is a separate mechanism.** Deleting the current version, deleting the delete marker, and expiring non-current versions are three different operations, and a bucket with versioning on and no lifecycle rule accumulates all three states indefinitely.

### Part 7: encryption, and who it is protecting against

The three modes differ in one thing, and it is not the algorithm:

| Mode | Who holds the key |
|---|---|
| a key the platform manages | the provider |
| a key the customer manages in a key service | the customer, with an audit trail for each use |
| a key the customer supplies with each request | the caller, and it never lives at rest |

**All three protect the same thing**, which is somebody with the disk and not the API. **None of them protects against a request that is authorised**, and a presigned URL is exactly that: a way in that is authorised by construction.

> **Encryption answers "can somebody who holds the storage medium read this". It does not answer "can somebody who holds a credential read this" — and the second is how object storage leaks.**

**And the customer-managed mode is the one with an operational difference**, because a key policy is a second access control with its own audit trail: **a principal without permission to use the key cannot read the object even with permission to read it**, which is a boundary that survives a misconfigured bucket policy.

### Part 8: what follows for security

**Everything here is a consequence of access control living in the request.** The signature is in the URL, the expiry is in the signature, the parameter set that the signature covers is a choice, and the request reaches several parsers on its way in. **So the review questions are about the request rather than about the store**: what does this URL carry, who can see it, how long does it last, and which parameters did the signer include.

**And the bearer nature of a presigned URL is the property to plan around.** Measured, it is reusable for its whole window, and it cannot be revoked. **So the controls are its lifetime and its destinations** — issue it for one object, for minutes rather than hours, return it to the one client that asked, and keep it out of anything that records URLs.

**Three switches decide public access and they live in three places.** Measured, a bucket policy saying `Principal: "*"` was overridden by a bucket setting, and an account setting was not overridable at all. **So the only reliable check is a request from outside the account**, made against an object that should be private — because the configuration can say public while the effective answer is not, and the reverse.

**And a delete is a claim about a view rather than about the data.** Measured, the version stack kept every version and the delete marker only changed what the current view showed. **For anything with a retention promise — personal data, a credential, a document that must not exist — the operations that matter are the ones that remove versions, and they are configured separately from the ones that remove objects.**

**And there is more than one front door.** The API, the website endpoint and the distribution serve the same objects under different rules, and the rules are usually written against whichever one the author had in mind. **Which is why the useful audit is a list of the ways an object can be reached**, rather than a list of the ways it was intended to be.

### Detection and mitigation

- **Test from outside rather than reading the configuration.** Measured, a policy saying public and an effective answer of private are both real states, and only a request distinguishes them.
- **Alert on any change to the public access switches, at both levels**, since those settings govern every bucket under them and their effect is invisible in the bucket's own documents.
- **Alert on presigned URLs appearing in logs, in `Referer` headers or in request lines.** That is the measured destination of any credential carried in a URL.
- **Track the age distribution of presigned URLs being generated.** A maximum measured in hours rather than minutes is a decision about how long a leaked link stays useful.
- **Report buckets with versioning enabled and no lifecycle rule**, because that combination is where a delete stops meaning anything and where storage grows.
- **Report objects whose version history is large or old**, since the versions are the copies that a deletion did not remove.
- **Check both granting mechanisms**, the bucket policy and the object ACL, and check the prefixes and actions each covers — including the measured distinction between a bucket's ARN and the objects in it.
- **For mitigation, leave the account-level switch on and enable it per bucket only where there is a reason**, since the safe configuration is also the default.
- **Issue presigned URLs per object and per recipient, with the shortest workable lifetime**, and generate them on demand rather than storing them.
- **Never enable versioning without a lifecycle rule**, and treat a version history as data with the same retention and access questions as the current object.
- **Decide which mode of encryption based on who should be able to read the data without the API**, and use a customer-managed key where the ability to revoke that is worth having.
- **And enumerate the front doors** — the API endpoint, the website endpoint and every distribution — because the rules are attached to the door rather than to the data.

<!-- lang:zh -->
### 一个带 HTTP API 的文件系统

对象存储与文件服务器的区别不在于它远。区别在于文件服务器通过一套协议做的每一件事，这里都是通过**请求**做的 —— 而一个请求带着它自己的授权、自己的参数、自己的签名。

> **这里的访问控制是对"一个 HTTP 请求"的判断。而底下的数据是不可变的，所以删除是一条新记录，而不是一次移除。**

这两句话推出了这一篇里的全部东西：为什么一条预签名 URL 是一张不记名的票、为什么决定公开性的那几层会以并不显然的方式互相覆盖、以及为什么从一个开了多版本的桶里删掉一个文件，是一句关于"当前视图显示什么"的话。

下面把签名方案实现出来、把文档化的规则跑出来实测。

### 第一部分：它的形状

| | 这里 |
|---|---|
| 容器 | 一个**桶**，名字全局唯一 |
| 单位 | 一个**对象**，它的键看起来像一条路径 |
| 层级 | **没有** —— 那些斜杠是名字的一部分 |
| 协议 | HTTP，授权在请求里 |
| 变更 | 对象是被**替换**的，可选地**保留版本** |
| 前门 | API 端点、网站端点、分发网络 |

**没有层级是第一件要内化的事**：前缀是一套命名约定，而每一件看起来像目录操作的事情，都是一次带过滤的列举。**这就是为什么"那个文件在那个文件夹里"是一句关于名字的话**，也是为什么一条按前缀写的策略在匹配字符串。

**而前门是第二件。** 同一批对象通过 API、通过一套有自己的规则的网站端点、以及通过一个可能缓存与改写的分发网络被提供出去。**规则通常是照着其中一个写的** —— 这与网关那一篇、以及影子端点那一篇是同一个形状。

### 第二部分：一条预签名 URL 是一张不记名的票

访问通常由"谁在问"决定。一条预签名 URL 把这个决定搬进了 URL 本身：签名者证明一次自己的权利，而那个结果字符串带着这个结论。

**这就让有效期成为全部的访问控制。** 实测，同一个 URL 连续使用五次：

| 使用 | 结果 |
|---|---|
| 第一、二、三、四、五次 | **每一次都接受** |

**没有 nonce、没有绑定到客户端、也没有撤销。** 在那个窗口内持有这个字符串的任何东西，都被当成签名者本人。

> **一条预签名 URL 是一张带时钟的不记名票。把它给出去，就是把那个窗口给出去；而缩短它的唯一办法，是一开始就把它设短。**

**而那个时钟就是参数本身**，对着文档化的规则实测：

| `X-Amz-Expires` | 距签发 | 结果 |
|---|---|---|
| 3600 | 刚签发 | 接受 |
| 3600 | 30 分钟后 | 接受 |
| 3600 | **两小时后** | **拒绝 —— 7200 秒超过了 3600 秒的有效期** |
| 60 | 第 59 秒 | 接受 |
| 60 | 第 60 秒 | 接受 |

**这意味着那个值是在生成 URL 时选定的、之后改不了** —— 有效期在签名里面，所以延长它会让它失效。

**而一条预签名 URL 住在 URL 里**，而认证那一篇实测过这件事的后果：请求行、访问日志，以及从那个页面加载的任何东西的 `Referer`。

**所以有意思的问题不只是"多久"，还有"它去哪里"。** 一个用户点的链接、一个嵌在页面里的链接、以及一个在 JSON 响应里返回的链接，最终到达的地方各不相同 —— 而一条出现在页面 URL 里的预签名 URL，会到那个页面发出的每个请求所到的地方。

### 第三部分：签名覆盖了什么，没覆盖什么

一个签名盖住请求的一部分、不盖另一部分，而那条边界是在生成 URL 时做出的一个选择。实测，同一条签过名的 URL 上追加东西：

| 请求 | 把全部参数纳入校验的实现 | 只纳入被签名参数的实施 |
|---|---|---|
| 原样 | 接受 | 接受 |
| 追加 `response-content-type` | **拒绝** | **接受** |
| 追加 `response-content-disposition` | **拒绝** | **接受** |
| 追加一个随便的 `x-trace=1` | **拒绝** | **接受** |
| 改路径 | 拒绝 | 拒绝 |

**两列在每一个被追加的参数上都不一致，而不一致就是全部要点**："这个请求还是我签的那个吗"这个问题的答案，取决于计算签名时纳入了哪些参数。

**而文档化的行为是右边那一列**，这意味着：

> **被签名的那个集合之外的一切，都是没人签过、却仍然会到达服务的输入。** 一个服务能理解、而签名者没有纳入的参数，就是一个由攻击者选择的参数。

**而同一个事实有一个良性的方向、值得知道**，因为实现就是在这里坏的：一个往自己的预签名 URL 上追加缓存破坏符或者追踪 id 的应用，在"只覆盖被签名集合"的校验器上能用，在"覆盖一切"的校验器上会把自己弄坏。

**而那条边界会随"哪些参数被服务视为已签名"而移动。** 响应改写类的参数被设计成要签，因为它们改变客户端拿到的东西；一个随意的查询参数不被签，因为它不改变服务据以行动的任何东西 —— **而评审的问题是：某个具体参数属于这两类中的哪一类。**

### 第四部分：又一个解析器，又一串字符串

在追加一个"签名里已经含有的参数名"的第二个值时实测：两个校验器都接受了它，因为各自取的是那个名字的**第一次**出现，而第一次出现的那个是被签过的值。

**这是同一个形状在这个系列里的第四次出现**，值得点名，因为可迁移的正是那个模式：

| 在哪里 | 一份输入 | 两种读法 |
|---|---|---|
| 一个对象的 JSON 体 | 两个同名的键 | 解析器的偏好决定了那个值 |
| 一条 protobuf 消息 | 同一个字段号出现两次 | 取最后或取最先 |
| 一个签过名的 webhook 请求体 | 一个重复键 | 接收方解析器挑中的那个 |
| **一条预签名 URL 的查询串** | **一个重复的参数** | **校验器挑中的那个** |

> **只要一个格式允许同一个名字出现多次，发送方与检查方就可能对"哪一个是那个值"意见不同 —— 而请求里没有任何东西说明想要哪一种读法。**

**而在查询串里，这个机制比在请求体里更显眼**：参数会被框架解析、被校验器解析、被签名计算解析、被服务解析，而每一个都可能有自己的意见。**随后该做的检查很便宜**：发一个带重复参数的请求，看答案会不会变。

### 第五部分：三层开关决定它是不是公开的

公开性由两个层面的文档加一对账号级设置决定，而实测的求值显示了优先级：

| 账号级封锁 | 桶级封锁 | 公开的 ACL 或策略 | 结果 |
|---|---|---|---|
| 关 | 关 | 没有 | **不可读** |
| 关 | 关 | 桶策略 `Principal: "*"` | **公开可读** |
| 关 | 关 | 公开的 ACL | **公开可读** |
| 关 | **开** | 桶策略 `Principal: "*"` | **不可读 —— 桶级开关压过策略** |
| **开** | 关 | 桶策略 `Principal: "*"` | **不可读 —— 账号级开关不可被覆盖** |
| **开** | **开** | 公开的 ACL | **不可读** |

**两行承载了那一课。** 一条含 `Principal: "*"` 的桶策略是一句关于意图的话，而不是关于可达性的话 —— **它上面那个开关说了算**。而账号级开关不是一个桶可以选择退出的默认值；**它是一道天花板**。

> **"策略里写着公开"与"外面读得到"是两个不同的事实，住在两个不同的地方，而任何一个都不蕴含另一个。**

**这就是为什么有用的验证是从外面发一个请求、而不是读配置** —— 与云身份那一篇同一个结论：一条策略可以写对动作、写错资源的形状，然后产出一次静默。

**而两种授予机制彼此独立。** 桶策略与对象 ACL 是两份文档、有两个所有者，而公开访问可以从任何一边来。**只查了其中一份的评审，只查了一半。**

### 第六部分：删除是一条记录，而开了多版本的桶什么都留着

实测，在一个开了多版本的桶里：

| 步骤 | `GET` 返回什么 | 版本栈里有什么 |
|---|---|---|
| 开始 | 最初的那份报告 | `v1` |
| 上传一个新版本 | 更正后的报告 | `v1`、`v2` |
| **删除** | **什么都没有 —— 对象表现为不存在** | `v1`、`v2`、**`v3` 一条删除标记** |
| 按指定版本读 | 先是最初的，然后是更正后的 | 没变 |
| 再上传一次 | 新内容 | `v1`、`v2`、`v3`、`v4` |

**那次删除没有移除任何东西。** 它在栈顶放了一条标记，而对象的"当前内容"由栈顶说了算 —— 这就是为什么同一次删除之后再上传一次，对象又可见了。

> **"已删除"的意思是当前视图什么都不显示。那些字节在版本历史里，直到有东西把它们移除 —— 而那个"东西"是一条生命周期规则，不是一次删除。**

**而一次普通的覆盖写也一样**，而这正是人们会忘掉的那种情形：上传一份更正后的文件会把上一份留在原处，所以**一个曾经装着秘密的对象，现在仍然装着它**。

**这就把这一篇与讲密钥和代码仓库的那一篇连了起来** —— 那里一次提交把一个值从工作区移走、却把它留在历史里。**形状完全相同、那一课也一样**：一个保留版本的系统会保留那次泄露，而应对是移除那个版本、并且把旧凭据当成已泄露。

**而对象存储特有的那一半是：移除是一个单独的机制。** 删除当前版本、删除删除标记、以及让非当前版本过期，是三个不同的操作；而一个开了多版本、却没有生命周期规则的桶，会把这三种状态无限累积下去。

### 第七部分：加密，以及它在防谁

三种模式只在一件事上不同，而那不是算法：

| 模式 | 谁持有密钥 |
|---|---|
| 平台托管的密钥 | 服务商 |
| 客户在密钥服务里托管的密钥 | 客户，且每次使用都有审计记录 |
| 客户在每次请求里提供的密钥 | 调用方，而它从不落盘 |

**三种保护的是同一件事**，也就是"拿到磁盘、但拿不到 API"的人。**它们没有一个防得住一次被授权的请求**，而一条预签名 URL 正是这个：一条按构造就被授权的进入方式。

> **加密回答的是"拿着存储介质的人能不能读这个"。它不回答"拿着凭据的人能不能读这个" —— 而后者才是对象存储泄露的方式。**

**而客户托管那种模式有一个运维上的差别**，因为密钥策略是第二道访问控制、带自己的审计记录：**一个没有权限使用那把密钥的主体，即使有读对象的权限也读不到** —— 那是一道在桶策略配错时仍然成立的边界。

### 第八部分：从这些机制推出的安全观念

**这里的一切都是"访问控制住在请求里"的后果。** 签名在 URL 里，有效期在签名里，签名覆盖哪些参数是一个选择，而那个请求在进来的路上会遇到好几个解析器。**所以评审的问题是关于请求的、不是关于那个库的**：这条 URL 带着什么、谁能看到它、它能活多久、以及签名者纳入了哪些参数。

**而预签名 URL 的不记名性质是那个要做计划围绕的性质。** 实测，它在整个窗口内可重复使用、且无法撤销。**所以控制手段是它的寿命与它的去处** —— 为一个对象签发、以分钟而不是小时计、只返回给发起请求的那一个客户端，并且别让它进入任何记录 URL 的东西。

**三层开关决定公开性，而它们住在三个地方。** 实测，一条写着 `Principal: "*"` 的桶策略被一个桶级设置压过，而一个账号级设置根本无法被覆盖。**所以唯一可靠的检查，是从账号外面、对一个本应私有的对象发一个请求** —— 因为配置可以说公开而实际答案是不，反过来也一样。

**而删除是一句关于视图的话，不是关于数据的话。** 实测，版本栈留住了每一个版本，而删除标记只改变了当前视图显示什么。**对任何带保留承诺的东西 —— 个人数据、一个凭据、一份必须不存在的文档 —— 要紧的是那些移除版本的操作，而它们与移除对象的操作是分开配置的。**

**而且前门不止一个。** API、网站端点与分发网络用不同的规则提供同一批对象，而规则通常是照着作者心里的那一个写的。**这就是为什么有用的审计是一份"一个对象能被哪些方式够到"的清单，而不是一份"它被打算用哪些方式够到"的清单。**

### 检测与缓解

- **从外面测，而不是读配置。** 实测，"策略说公开"与"实际答案是私有"都是真实状态，而只有一个请求能把它们区分开。
- **对两个层面的公开访问开关的任何改动告警**，因为那些设置管着它们之下的每一个桶，而它们的效果在桶自己的文档里看不见。
- **对预签名 URL 出现在日志、`Referer` 头或请求行里告警。** 那是任何被 URL 携带的凭据实测会到的地方。
- **追踪正在生成的预签名 URL 的寿命分布。** 一个以小时而不是以分钟计的上限，是一个关于"一条泄露的链接还能用多久"的决定。
- **报出"开了多版本、却没有生命周期规则"的桶**，因为那个组合正是"删除不再有任何意义"、而且存储不断增长的地方。
- **报出版本历史很大或很老的对象**，因为那些版本就是一次删除没有移除的副本。
- **两种授予机制都查** —— 桶策略与对象 ACL —— 并且查各自覆盖的前缀与动作，包括实测到的"桶的 ARN"与"桶里的对象"那个区别。
- **缓解上，保留账号级开关开着，只在有理由的地方按桶打开**，因为安全的配置同时也是默认配置。
- **按对象、按接收方签发预签名 URL，用能干活的最短寿命**，并且按需生成、不要存起来。
- **绝不在没有生命周期规则的情况下打开多版本**，并把版本历史当成一份与当前对象有同样保留与访问问题的数据。
- **按"谁应当能在不经过 API 的情况下读到这份数据"来决定加密模式**，而在"能撤销这种能力"值得拥有的地方使用客户托管的密钥。
- **并且把前门列出来** —— API 端点、网站端点、以及每一个分发 —— 因为规则挂在门上，而不是挂在数据上。
