---
id: cloud-identity-and-access
title_en: Cloud Identity and Access
title_zh: 云身份与访问控制
summary_en: A request is allowed only if several separate documents each allow it, none of them denies it, and the ones that are only capable of limiting get their say. Measured by evaluating the documented rules — a resource ARN whose shape silently decides what is covered, a trust policy shorthand that means an entire account rather than a user, and a cross-account check that needs both sides.
summary_zh: 一个请求要被允许，必须有好几份彼此独立的文档各自允许它、没有一份拒绝它，而且那些"只能限制"的文档也都要点头。这一篇把文档化的求值规则跑出来 —— 一个"形状决定覆盖范围"的资源 ARN、一个其实指整个账号的信任策略简写，以及一次需要两侧都同意的跨账号判断。
tags: [beginner, cloud, iam, policy, least-privilege]
tools: [aws, gcloud, az, python3]
attck: [T1078, T1098]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Several documents, and the answer is their intersection

A cloud identity system looks like roles and policies. Underneath it is three rules, and every behaviour in this entry comes from them.

> **Nothing is allowed by default. An explicit denial beats every permission. And the layers that can only limit have to agree.**

The third rule is the one that makes this different from every access control system measured so far in this series: there is no inheritance to reason about, there is an **intersection**, and the layers are not all capable of the same thing — some can grant, and some can only take away.

Measured below by evaluating the documented rules against requests that differ in one field at a time.

### Part 1: the shape of it

| Element | What it is |
|---|---|
| **principal** | a user, a role session, or a service |
| **identity policy** | attached to a principal; says what it may do |
| **resource policy** | attached to a resource; says who may touch it |
| **trust policy** | attached to a role; says **who may become it** |
| **permission boundary** | attached to a principal; **only limits** |
| **service control policy** | attached to an account or organisation; **only limits** |
| **session policy** | passed when credentials are obtained; **only limits** |

**And a role is the element that makes the model readable**: it is an identity that nobody logs into, which a principal becomes temporarily. **That is why a role has two documents rather than one**, and why the confusion in Part 4 exists at all.

**The credentials that come out of assuming a role are temporary**, which is the property the entries on keys pointed at: they expire, so a leak has a bounded life — and the boundary is only useful if nothing long-lived was left behind as well.

### Part 2: the three rules, measured

**Nothing is allowed by default.** Every measurement below that ends in a denial ends that way because no document said yes — which is a safe default and the reason a misconfigured policy fails closed.

**An explicit denial beats a permission.** Measured, a policy granting everything, with one action denied on top:

| Action with `"Action": "*"`, `"Resource": "*"` | As granted | With one `Deny` added |
|---|---|---|
| `s3:GetObject` | Allow | **Allow** |
| `s3:DeleteObject` | Allow | **Deny** |
| `iam:CreateUser` | Allow | **Allow** |

**The broad grant stayed broad and the one denied action stopped working**, which is what makes a denial the right tool for stopping something immediately: it does not require finding and narrowing the policy that granted it.

**And the layers that can only limit have to agree.** Measured, with a wide identity policy and a boundary that covers storage only:

| Request | Identity policy | Boundary | Outcome |
|---|---|---|---|
| `s3:GetObject` | allow | allow | **Allow** |
| `s3:DeleteObject` | allow | allow | **Allow** |
| **`ec2:RunInstances`** | allow | **no** | **Deny — the boundary did not allow it** |

**And in the other direction**, with a narrow identity policy and a boundary covering something else entirely:

| Request | Identity policy | Boundary | Outcome |
|---|---|---|---|
| `s3:GetObject` | **allow** | **no** | **Deny — the boundary did not allow it** |
| `s3:DeleteObject` | no | no | Deny — nothing allowed it |

**Both directions deny**, and the two rows say different things: **a boundary cannot grant what the identity policy withholds, and it can withhold what the identity policy grants.**

**The same intersection was measured for the other two limiting layers.** A service control policy denying everything outside one region:

| Requested region | Outcome |
|---|---|
| the allowed region | **Allow** |
| any other region | **Deny — explicit denial from the service control policy** |

**And a session policy narrower than the identity policy**, where the identity policy allows all of storage and the session allows read on one prefix:

| Request | Outcome |
|---|---|
| read inside the prefix | Allow |
| read outside it | **Deny — the session policy did not allow it** |
| write inside the prefix | **Deny — the session policy did not allow it** |

**Which produces the sentence worth remembering about this model:**

> **There is no inheritance here, only an intersection. So "we gave someone a permission" can be answered with "and nothing changed" — because a layer above them did not allow it.**

**And the practical consequence for a review is about where to look.** The answer to "can this principal do this" is not in one document; it is the result of intersecting several, **and the one that produced the denial is the only useful thing to report.**

### Part 3: the shape of a resource ARN decides what it covers

The measurement most likely to explain a policy that "looks right and does nothing". A storage bucket and its objects are two different resources with two different ARN shapes:

```
bucket:  arn:aws:s3:::data
objects: arn:aws:s3:::data/*
```

Measured, three policies each granting one action on one shape:

| Policy | `s3:GetObject` on `data/report.pdf` | `s3:ListBucket` on `data` | `s3:GetObject` on `other/report.pdf` |
|---|---|---|---|
| allow `GetObject` on **`data/*`** | **Allow** | **Deny** | Deny |
| allow `GetObject` on **`data`** | **Deny** | Deny | Deny |
| allow `ListBucket` on **`data`** | Deny | **Allow** | Deny |

**Read the middle row**: an action that reads an object, granted on the bucket ARN, allows nothing at all. **And the first row**: the object ARN does not cover the action that lists the bucket.

> **A policy can name the right action and the wrong shape of resource, and the result is a silent nothing rather than an error.**

**Which is why "the policy is attached and the action is spelled correctly" is not the end of a review.** The pairing between an action and the resource it acts on is part of the action's definition, and it is the part a reader skips.

**And the negative forms are the same trap from the other side.** A statement with `NotAction` or `NotResource` grants everything except what is named — measured nowhere here, because the shape of the mistake is the same and the blast radius is not: **a `Deny` with `NotAction` names the few things that keep working, and everything else stops.**

### Part 4: a role has two documents, and one shorthand is misread

A role's trust policy answers "who may become this", and its identity policy answers "what may be done once they have". Measured, both, with the principal changing:

| Trust policy says | A user in account 2222 | Account 2222's own root | A user in account 3333 |
|---|---|---|---|
| `Principal.AWS = arn:aws:iam::222222222222:root` | **allowed** | **allowed** | denied |
| `Principal.AWS = arn:aws:iam::222222222222:user/bob` | denied | denied | denied |
| `Principal: "*"` | **allowed** | **allowed** | **allowed** |

**The first row is the one to keep.** The string looks like it names a user called `root`, and it actually names the account:

> **`:root` in a trust policy is shorthand for "this account", so it matches every principal in it. Written intending to allow one account, it allows everyone in that account.**

**Which is usually what the author meant and occasionally is not** — the difference matters when the account is somebody else's, when it has many principals, or when a compromised principal in it can now assume a role nobody thought was shared.

**And the two documents do not interact.** Measured, a role whose trust policy allows only one specific user: that user can assume it, and the identity policy's contents are unaffected by which trust policy is in place — **so tightening the trust policy changes nothing about what an already-assumed session may do, and widening the identity policy changes nothing about who can assume it.**

**And the last row is the shape to search for**: a trust policy with a bare `Principal: "*"` and no condition. Measured, it allowed a principal from an unrelated account, which is what it says it does.

### Part 5: cross-account needs both sides

For a principal in one account to reach a resource in another, the measurement is a conjunction:

| Configuration | Outcome |
|---|---|
| the resource's policy allows the caller **and** the caller's identity policy allows the action | **Allow** |
| only the resource's policy allows | **Deny — the caller's identity policy did not allow it** |
| only the caller's identity policy allows | **Deny — the resource's policy did not allow it** |
| same account, resource policy only | **Allow** |
| same account, identity policy only | **Allow** |

**The last two rows are why this is worth measuring**: inside one account a resource policy is sufficient on its own, so **a configuration that works in testing fails in production when the caller moves to another account** — and the denial names the side that is missing, which is the only useful diagnostic.

**And the cross-account form is where the interesting mistakes are**, because it requires two documents that two different teams own. **A resource policy written to allow an account is a decision about that account's entire principal population**, which is the same shorthand measured in Part 4.

### Part 6: the confused deputy, in its current form

A role that a service assumes on your behalf is trusted with a service principal rather than an account. Measured:

| Trust policy | Where the event came from | Outcome |
|---|---|---|
| `Principal.Service = events...`, **no condition** | another account's resource | **Allow** |
| same, plus `aws:SourceAccount` = your account | another account's resource | **Deny** |
| same, plus `aws:SourceAccount` = your account | your account's resource | **Allow** |

**The first row is the finding.** A trust policy naming only a service means **any resource, in any account, that can make that service call you, can make it assume that role** — and the policy says nothing about whose resource it was.

> **A service principal is a statement about which service, not about which caller. The caller is a separate fact, and a condition is the only place to name it.**

**And the condition has two useful forms, which do different jobs.** `aws:SourceAccount` restricts to an account — measured to deny another account's resource and allow your own. `aws:SourceArn` restricts to a specific resource, which is the narrower and usually correct one, **because an account is the same shorthand measured in Part 4: everyone in it, not one thing in it.**

### Part 7: temporary credentials, and what makes them better

The credentials a role session uses are issued with an expiry, which is the one property that makes a leak survivable.

**Which is why the assumption to log is a security event.** "Who became this role, when, and from where" is answerable from the identity the session carries, and it is what makes a later action attributable to a principal rather than to a role that many principals use.

**And a session can be narrowed at the moment it is obtained.** Measured in Part 2, a session policy intersected with the identity policy produced denials for everything outside one prefix — **so the same role can hand out sessions of different reach to different callers**, which is the mechanism that makes one role usable by several workloads without giving each of them the union.

**And the lifetime is a trade rather than a setting.** A short one bounds a leak and increases the number of times credentials are obtained; a long one is convenient and is a long-lived credential wearing a temporary name. **The reason temporary is better is not the number of minutes — it is that the number exists.**

### Part 8: what follows for security

**The three rules decide what a review can and cannot conclude.** Because nothing is allowed by default, a misconfiguration fails closed and an audit finds permissions nobody granted. Because a denial wins, it is the tool for an incident. **And because the layers intersect, the answer to "can this principal do this" is computed from several documents rather than read from one** — which is the single most useful thing to know about this model, and the reason a review that reads one policy file has not answered the question.

**Measured, the two layers that can only limit denied in both directions**: a boundary withheld an action the identity policy granted, and withheld one the identity policy granted on a different resource. **So a boundary is the correct place to express "at most this", and the wrong place to express "at least this"** — it cannot grant, and a policy written with that expectation is either ineffective or confusing.

**The ARN measurement is the most common silent failure.** An action paired with the wrong resource shape grants nothing and reports nothing. **The way to catch it is to test with a request rather than to read the policy**, because the pairing lives in the action's definition rather than in the document.

**The `:root` shorthand and the bare service principal are the same mistake twice**: a policy element that reads as narrower than it behaves. **`:root` means an account, which means every principal in it. A service principal means a service, which means every resource that can invoke it.** Both are fixed by naming the account or the resource explicitly — in a condition, where the trust policy allows one.

**And cross-account access is a conjunction owned by two teams.** Measured, a configuration that works inside one account fails across two, and the useful error names which side is missing. **Which means a cross-account grant is not one change but two, and the one that is easy to forget is the caller's own permission.**

**And least privilege here is limited by readability rather than by intent.** Policies are JSON with wildcards, conditions and ARN shapes, and the difference between the measured rows is a `/*`. **So the practical approaches are the ones that do not depend on reading**: start from a narrow policy and widen in response to denials, use an access-analyser that reports what was actually used, and treat every `"Action": "*"` with `"Resource": "*"` as a finding until something justifies it.

### Detection and mitigation

- **Alert on trust policies with a bare `Principal: "*"`, and on service principals without a source condition.** Measured, the first allows any account's principal and the second allows any resource that can invoke that service.
- **Search for `:root` in trust policies and check what it was meant to allow.** Measured, it matches every principal in the account rather than one user.
- **Report every policy combining `"Action": "*"` with `"Resource": "*"`**, and everything granting `iam:*` or the ability to create credentials, since those are the permissions that turn into any other permission.
- **Use an access analyser to find permissions that are granted and never used**, which is the only reliable way to narrow a policy whose current shape is too broad to reason about by reading.
- **Alert on the assumption of a role from an unexpected account or through an unexpected path**, since the session identity is the thing a later action is attributed to.
- **Watch for permission boundaries and service control policies being removed**, because both only ever restrict and their absence is invisible in the permissions that remain.
- **For mitigation, rely on the default**: a policy that grants nothing is the starting point, and every grant should be traceable to something that needed it.
- **Express "at most" in a boundary or a service control policy, and "at least" in an identity or resource policy**, and do not expect either to do the other's job.
- **Name the account and the resource in a trust policy condition** rather than relying on a principal element to be narrow.
- **Test a policy change with a request that should be denied**, since the measured failures here are silences rather than errors.
- **Check the resource ARN shape against the action being granted**, because the measurement showed a correctly spelled action on a correctly named resource granting nothing.
- **And treat a resource policy as a decision about every principal in the account it names**, which is the same shorthand as `:root` with a different spelling.

<!-- lang:zh -->
### 几份文档，而答案是它们的交集

云的身份系统看起来是角色与策略。它底下是三条规则，而这一篇里的每一个行为都来自它们。

> **默认什么也不允许。显式拒绝压过每一条许可。而那些"只能限制"的层必须一起点头。**

第三条正是它在这个系列里此前测过的每一个访问控制体系之外的原因：这里没有可以推理的继承，只有一个**交集**，而且各层能力并不相同 —— 有些能授予，有些只能拿走。

下面把文档化的规则跑在"每次只差一个字段"的请求上。

### 第一部分：它的形状

| 元素 | 它是什么 |
|---|---|
| **主体** | 一个用户、一个角色会话、或者一个服务 |
| **身份策略** | 挂在主体上；说明它能做什么 |
| **资源策略** | 挂在资源上；说明谁可以碰它 |
| **信任策略** | 挂在角色上；说明**谁可以成为它** |
| **权限边界** | 挂在主体上；**只能限制** |
| **服务控制策略** | 挂在账号或组织上；**只能限制** |
| **会话策略** | 获取凭据时传入；**只能限制** |

**而角色是让这个模型读得懂的那个元素**：它是一个没有人登录的身份，由某个主体临时变成。**这就是为什么一个角色有两份文档、而不是一份**，也是第四部分那处混淆存在的原因。

**从扮演角色得到的凭据是临时的**，这正是密钥那几篇指向的性质：它们会过期，所以一次泄露有一个有界的寿命 —— 而这个边界只有在同时没有留下任何长期东西的时候才有用。

### 第二部分：三条规则，实测

**默认什么也不允许。** 下面每一个以拒绝结束的实测都是那样结束的，因为没有文档说过"可以" —— 这是一个安全的默认值，也是一个配错的策略会朝失败关闭的原因。

**显式拒绝压过许可。** 实测，一个允许一切、再在其上拒绝一个动作的策略：

| 在 `"Action": "*"`、`"Resource": "*"` 之下 | 单纯授予时 | 加了一条 Deny 之后 |
|---|---|---|
| `s3:GetObject` | Allow | **Allow** |
| `s3:DeleteObject` | Allow | **Deny** |
| `iam:CreateUser` | Allow | **Allow** |

**那个宽泛的授予仍然宽泛，而被拒绝的那一个动作停止了工作**，这就是为什么拒绝是立刻止住某件事的正确工具：它不需要你先找到并收窄那条授予它的策略。

**而那些只能限制的层必须一起点头。** 实测，一个宽的身份策略加一个只覆盖存储的边界：

| 请求 | 身份策略 | 边界 | 结果 |
|---|---|---|---|
| `s3:GetObject` | 允许 | 允许 | **Allow** |
| `s3:DeleteObject` | 允许 | 允许 | **Allow** |
| **`ec2:RunInstances`** | 允许 | **没有** | **Deny —— 边界没有允许** |

**而反方向**，一个窄的身份策略加一个覆盖完全不同东西的边界：

| 请求 | 身份策略 | 边界 | 结果 |
|---|---|---|---|
| `s3:GetObject` | **允许** | **没有** | **Deny —— 边界没有允许** |
| `s3:DeleteObject` | 没有 | 没有 | Deny —— 没有东西允许它 |

**两个方向都拒绝**，而两行说的是不同的事：**边界给不出身份策略所不给的东西，而它能收走身份策略所给的东西。**

**同样这处交集在另外两个限制层上也测了。** 一个拒绝某个区域之外一切的服务控制策略：

| 请求的区域 | 结果 |
|---|---|
| 被允许的区域 | **Allow** |
| 任何其他区域 | **Deny —— 显式拒绝来自服务控制策略** |

**以及一个比身份策略更窄的会话策略**：身份策略允许整个存储，会话只允许读某一个前缀：

| 请求 | 结果 |
|---|---|
| 在前缀内读 | Allow |
| 在前缀外读 | **Deny —— 会话策略没有允许** |
| 在前缀内写 | **Deny —— 会话策略没有允许** |

**由此得到关于这个模型最该记住的一句话：**

> **这里没有继承，只有一个交集。所以"我们给某人加了一个权限"的答案可能是"什么也没变" —— 因为上面某一层没有允许它。**

**而这件事对评审的实际后果在于"该去哪看"。** "这个主体能不能做这件事"的答案不在一份文档里；它是把若干份求交之后的结果，**而那份产生了拒绝的文档，才是唯一值得报出来的东西。**

### 第三部分：资源 ARN 的形状决定它覆盖什么

这一组最可能解释一条"看起来对、却什么都不做"的策略。一个存储桶和它里面的对象是两个不同资源、两种不同的 ARN 形状：

```
桶:    arn:aws:s3:::data
对象:  arn:aws:s3:::data/*
```

实测，三条策略各自在一个形状上授予一个动作：

| 策略 | `s3:GetObject` 打 `data/report.pdf` | `s3:ListBucket` 打 `data` | `s3:GetObject` 打 `other/report.pdf` |
|---|---|---|---|
| 在 **`data/*`** 上给 `GetObject` | **Allow** | **Deny** | Deny |
| 在 **`data`** 上给 `GetObject` | **Deny** | Deny | Deny |
| 在 **`data`** 上给 `ListBucket` | Deny | **Allow** | Deny |

**看中间那一行**：一个读对象的动作、授在桶的 ARN 上，什么也没允许。**而第一行**：对象的 ARN 不覆盖那个列举桶的动作。

> **一条策略可以写对动作、写错资源的形状，而结果是静默的"什么也没有"，而不是一个错误。**

**这就是为什么"策略挂上去了、动作拼对了"不是一个评审的终点。** 动作与它所作用的资源之间的配对，是那个动作定义的一部分，也正是读的人会跳过的那一部分。

**而否定形式是同一个陷阱从另一侧来。** 一条带 `NotAction` 或 `NotResource` 的语句，授予的是"除了被点名的那些之外的一切" —— 这里没有实测它，因为错误的形状是一样的、而杀伤半径不是：**一条带 `NotAction` 的 `Deny` 点出的是少数还能用的东西，而其他一切都会停。**

### 第四部分：一个角色有两份文档，而一个简写会被读错

一个角色的信任策略回答"谁可以成为它"，它的身份策略回答"成为之后能做什么"。实测，两者一起看，主体在变：

| 信任策略写 | 2222 账号里的一个用户 | 2222 账号的 root 本体 | 3333 账号里的一个用户 |
|---|---|---|---|
| `Principal.AWS = arn:aws:iam::222222222222:root` | **允许** | **允许** | 拒绝 |
| `Principal.AWS = arn:aws:iam::222222222222:user/bob` | 拒绝 | 拒绝 | 拒绝 |
| `Principal: "*"` | **允许** | **允许** | **允许** |

**第一行才是要留下的。** 那个字符串看起来像在点名一个叫 `root` 的用户，而它实际点的是那个账号：

> **信任策略里的 `:root` 是"这个账号"的简写，所以它匹配账号里的每一个主体。本想只允许一个账号而写下它，效果是允许那个账号里的所有人。**

**这通常正是作者的意思，偶尔不是** —— 当那个账号是别人的、当它里面有很多主体、或者当它里面一个被拿下的主体从此可以扮演一个没人以为共享出去的角色时，那个差别就要紧。

**而两份文档互不作用。** 实测，一个信任策略只允许某个具体用户的角色：那个用户可以扮演它，而身份策略的内容并不因为换了一份信任策略而改变 —— **所以收窄信任策略不改变一个已经拿到的会话能做什么，放宽身份策略也不改变谁能扮演它。**

**而最后一行是要去搜的形状**：一条写着裸 `Principal: "*"`、没有任何条件的信任策略。实测，它允许了一个来自无关账号的主体，而那正是它字面上说的。

### 第五部分：跨账号需要两侧都同意

一个账号里的主体要够到另一个账号里的资源，实测是一次合取：

| 配置 | 结果 |
|---|---|
| 资源方的策略允许调用方 **且** 调用方的身份策略允许该动作 | **Allow** |
| 只有资源方的策略允许 | **Deny —— 调用方的身份策略没有允许** |
| 只有调用方的身份策略允许 | **Deny —— 资源方的策略没有允许** |
| 同账号，只有资源策略 | **Allow** |
| 同账号，只有身份策略 | **Allow** |

**最后两行就是这件事值得测的原因**：在同一个账号内，一条资源策略自己就够了，所以**一套在测试里能跑的配置，在调用方搬到另一个账号之后会失败** —— 而那次拒绝会说出缺的是哪一侧，那是唯一有用的诊断。

**而跨账号这种形式才是有意思的错误所在**，因为它需要两份由两个不同团队拥有的文档。**一条写来允许某个账号的资源策略，是一个关于那个账号全部主体人群的决定** —— 也就是第四部分实测到的那个简写。

### 第六部分：被混淆的代理，在现代的样子

一个由某个服务代你扮演的角色，信任的是一个服务主体、而不是一个账号。实测：

| 信任策略 | 事件来自哪里 | 结果 |
|---|---|---|
| `Principal.Service = events...`，**没有条件** | 另一个账号的资源 | **Allow** |
| 同上，外加 `aws:SourceAccount` = 你的账号 | 另一个账号的资源 | **Deny** |
| 同上，外加 `aws:SourceAccount` = 你的账号 | 你账号的资源 | **Allow** |

**第一行就是那条发现。** 一个只写了服务的信任策略意味着**任何账号里、任何能让那个服务来调用你的资源，都能让它扮演那个角色** —— 而策略里没有任何东西说那是谁的资源。

> **一个服务主体是一句关于"哪个服务"的话，不是一句关于"哪个调用者"的话。调用者是另一个事实，而条件键是唯一能点名它的地方。**

**而条件有两种有用的形式，做的是不同的事。** `aws:SourceAccount` 限制到一个账号 —— 实测它拒绝了另一个账号的资源、允许了你自己账号的。`aws:SourceArn` 限制到一个具体资源，那才是更窄、通常也更正确的那一个，**因为账号就是第四部分实测到的那个简写：里面的每一个人，而不是里面的某一个东西。**

### 第七部分：临时凭据，以及它好在哪

角色会话所用的凭据带着一个过期时间签发，而那是让一次泄露可承受的唯一性质。

**这就是为什么"扮演"这件事本身值得记入日志。** "谁在什么时候、从哪里变成了这个角色"可以由会话携带的身份回答，也正是它让之后的一个动作能归因到一个主体，而不是归因到一个很多主体都在用的角色。

**而一个会话可以在它被获取的那一刻被收窄。** 第二部分实测过，一个会话策略与身份策略求交之后，对某一个前缀之外的一切都拒绝 —— **所以同一个角色可以按调用方的不同，发出可达范围不同的会话**，这正是让一个角色能被若干个工作负载使用、而不必把它们的并集给每一个的机制。

**而寿命是一笔取舍，不是一个设置。** 短的那个给泄露设了边界、并增加了获取凭据的次数；长的那个方便，而且是一把长期凭据披着临时的名字。**临时比长期好的理由不是那个分钟数 —— 而是那个数存在。**

### 第八部分：从这些机制推出的安全观念

**那三条规则决定了评审能得出与不能得出什么。** 因为默认什么都不允许，一个配错会朝失败关闭、而一次审计能找出没人授予过的权限。因为拒绝优先，它是事件处置的工具。**而因为各层求交，"这个主体能不能做这件事"的答案是从若干份文档算出来的、不是从一份读出来的** —— 那是关于这个模型最有用的一件事，也是一次只读了一个策略文件的评审没有回答问题的原因。

**实测，那两个只能限制的层在两个方向上都拒绝了**：一个边界收走了一个身份策略所授予的动作，也收走了一个身份策略在另一个资源上所授予的。**所以边界是表达"最多这些"的正确位置，也是表达"至少这些"的错误位置** —— 它给不出东西，而一条带着那个期望写的策略要么无效、要么让人困惑。

**ARN 那组实测是最常见的静默失败。** 一个动作配上错误的资源形状，什么也不授予、也不报告任何东西。**抓住它的方式是用一个请求去试、而不是去读策略**，因为那处配对住在动作的定义里、不在文档里。

**`:root` 那个简写与裸的服务主体是同一个错误出现了两次**：一个读起来比它实际行为更窄的策略元素。**`:root` 意味着一个账号、也就是里面的每一个主体。一个服务主体意味着一个服务、也就是每一个能调用它的资源。** 两者都靠显式点名账号或资源来修 —— 放在条件里，因为信任策略只允许一个。

**而跨账号访问是一次由两个团队共同拥有的合取。** 实测，一套在单账号里能跑的配置在两个账号之间会失败，而有用的那个错误会说缺的是哪一侧。**这意味着一次跨账号授予不是一个改动、而是两个**，而容易忘掉的那一个是调用方自己的权限。

**而这里的最小权限受限于可读性、而不是受限于意图。** 策略是带通配符、条件与 ARN 形状的 JSON，而实测那些行之间的差别是一个 `/*`。**所以可行的做法是那些不依赖阅读的做法**：从一条窄策略开始、在遇到拒绝时再放宽，用一个访问分析工具报告实际被用过的东西，并且把每一个 `"Action": "*"` 配 `"Resource": "*"` 都当成一条发现，直到有东西能说明它合理。

### 检测与缓解

- **对写着裸 `Principal: "*"` 的信任策略、以及没有来源条件的服务主体告警。** 实测，前者允许任何账号的主体，后者允许每一个能调用那个服务的资源。
- **搜信任策略里的 `:root`，并核对它本意是要允许什么。** 实测，它匹配账号里的每一个主体，而不是一个用户。
- **报出每一条把 `"Action": "*"` 与 `"Resource": "*"` 组合在一起的策略**，以及一切授予 `iam:*` 或创建凭据能力的策略，因为那些是会变成任何其他权限的权限。
- **用一个访问分析工具找出"授予了却从未被使用"的权限**，那是收窄一条当前形状宽到读不动的策略的唯一可靠方式。
- **对"从一个意外的账号、或者通过一条意外的路径扮演角色"告警**，因为会话身份才是之后一个动作被归因到的东西。
- **盯权限边界与服务控制策略被移除**，因为两者都只会限制，而它们的缺席在剩下的权限里是看不见的。
- **缓解上，依靠那个默认值**：什么都不授予的策略才是起点，而每一条授予都应当能追溯到某个需要它的东西。
- **把"最多这些"表达在边界或服务控制策略里，"至少这些"表达在身份或资源策略里**，不要指望任何一方做另一方的活。
- **在信任策略的条件里点名账号与资源**，而不是指望某个主体元素本身足够窄。
- **用一个"本应被拒绝"的请求去测每一次策略改动**，因为这里实测到的失败是静默、而不是报错。
- **把资源的 ARN 形状与被授予的动作对起来看**，因为实测显示一个拼写正确的动作、配一个命名正确的资源，可以什么都不授予。
- **并且把一条资源策略当成一个关于它所点名的账号里每一个主体的决定**，那与 `:root` 是同一个简写、只是写法不同。
