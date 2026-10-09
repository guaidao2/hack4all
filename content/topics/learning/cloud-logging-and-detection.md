---
id: cloud-logging-and-detection
title_en: Cloud Logging and Detection
title_zh: 云日志与检测
summary_en: Two planes record two different kinds of event, and neither can answer the other's question, so "who read this object" and "who changed this policy" come from different places. Measured by running one scenario and asking five questions of each log — a read that no log can answer, an attribution that names a session rather than a role, and a disabled log whose disabling was itself recorded elsewhere.
summary_zh: 两个平面记录两类不同的事件，而任何一方都答不了对方的问题，所以"谁读了这个对象"与"谁改了这个策略"来自不同的地方。这一篇跑一个场景、拿五个问题去问每一份日志 —— 一次没有任何日志能回答的读取、一次点到会话而不是角色的归因，以及一条被停掉的日志（而"停掉"这个动作本身被记在了别处）。
tags: [beginner, cloud, logging, detection, audit]
tools: [aws, gcloud, az, jq]
attck: [T1562.008, T1070]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Two planes, and neither answers the other's question

A log sounds like one thing. On a cloud platform it is at least two, and the split follows the platform's own structure.

> **A control-plane action changes the rules. A data-plane action uses them. "Who made this bucket public" and "who read this object" are therefore two kinds of event, recorded in two places, and each is invisible in the other.**

The measurement below is one scenario — an intrusion with a few steps — followed by five questions asked of each log. The interesting results are the questions no log can answer, and the one where the answer survived because a second log existed.

### Part 1: the shape of it

| | Control plane | Data plane |
|---|---|---|
| the events | creating users, changing policies, starting and stopping logs | reading, writing, deleting objects |
| the volume | small | **potentially enormous** |
| usually on by default | **yes** | **no, or by resource** |
| answers | "who changed the configuration" | "who touched the data" |
| the subject | the caller's identity | the caller's identity |

**And the log is itself a configuration.** Who may write to it, where it is delivered, how long it is kept, and whether it can be deleted are all settings under the same access control as everything else — **which makes the log a resource to be protected rather than a fact to be trusted.**

**And there is a second axis that is easy to miss**: the log records the API call, and the API call is not the request the application received. **The link between them lives in the application's own logs**, which is why an investigation usually needs both and why one of them is usually absent.

### Part 2: five questions, three logs

The scenario, in order: a deployment identity created an access key, a session read one object in a bucket with no data events, read one from a bucket that has them, granted itself administrator rights, stopped the log, read another object, deleted one, and created a user.

Measured against each log:

| Question | Main control log | A second, independent control log | Data event log |
|---|---|---|---|
| who read the object in the bucket without data events? | nothing | nothing | **nothing** |
| who read the object in the bucket with them? | no | no | **yes** |
| who granted themselves administrator rights? | **yes** | **yes** | no |
| who stopped the log? | **yes** | **yes** | no |
| who created a user after the log was stopped? | **no — the log was stopped** | **yes** | no |

**Three rows carry the entry.**

**The first row has no answer anywhere**, because data events were never enabled for that resource. Nothing failed; the question simply is not among the ones that log was configured to answer.

**The second row shows the split in its normal form**: a data-plane read appears in the data log and nowhere in either control log.

**And the fifth row shows why one log is not enough**: the control log that was stopped has nothing after the stop, and the independent one has everything.

> **"It is not in the log" has four different meanings: it did not happen, that resource had no data events enabled, the record is past its retention, or it has not been delivered yet. All four look identical in a query result.**

**Which is why the useful question about a log is not "is it on" but "can this specific question be answered from it"** — and the way to find out is to ask the question.

### Part 3: the subject is a session, not a role

Measured, the identities in a data-plane record:

```
GetObject   reports/q3.pdf   arn:aws:sts::111111111111:assumed-role/app-role/i-0abc123
```

**The principal is the session, including which instance assumed it** — not the role's name. **Which matters because a role is assumed by many workloads**, so a record naming only the role would not say which one acted.

> **The log records who made the API call, not who the call was for. The first is an identity and the second is application context, and only the first is in the platform's log.**

**So a request made by a user, on a service's behalf, through a role, appears under the identity that actually called the API** — and the "on whose behalf" part is a field the application either recorded or did not.

**And this is the same distinction the entry on keys drew about attribution**: what makes an action attributable is the session it was taken under, and the session is what an investigation follows.

### Part 4: stopping one log does not stop logging

Measured, with the scenario's log stopped partway through:

| | After the stop |
|---|---|
| the stopped log's data events | **0** |
| the main control log's events | stopped |
| **the second, independent control log's events** | **still recording — including the stop itself and everything after it** |

**And the action of stopping was recorded**, in both the log being stopped and the one that was not:

```
1000020  StopLogging  trail/main  arn:aws:sts::111111111111:assumed-role/app-role/i-0abc123
```

> **You can see when the lights went out — and that is usually enough to know where to look.**

**And the independent log is the part that matters.** A second destination, in a different account or trust domain, is one that the identity being audited cannot reach — **which is the difference between a log and a log that the incident can edit.**

> **Keeping one log means handing the answers to some questions to whoever wants to delete them.**

**And the volume of what is left is often the detection.** A trail that stops is an event; a gap in a series that should be continuous is visible without knowing what filled it.

### Part 5: two boundaries, delivery and retention

Measured, both limits on what a query can return:

| Situation | Result |
|---|---|
| an event that just happened, queried immediately | **not present — not delivered yet** |
| the same, after the delivery interval | present |
| a record older than the retention period | **not present — expired** |

**Both are ordinary and neither is an error**, and together they mean a query result is a statement about a window rather than about history.

> **"It is not in the log" is not "it did not happen"**, and the two reasons are opposite in time: one record is too new and the other is too old.

**Which makes retention a detection decision rather than a storage one.** An incident is often discovered long after it began, **so a retention period shorter than the time it takes to notice is a period that cannot answer the question it exists for.**

**And integrity is a third boundary.** A log that can be rewritten is a record of what somebody chose to leave, so append-only storage and the ability to prove that nothing was removed are what make the earlier rows meaningful. **The check is not only "is it delivered" but "could it have been changed".**

### Part 6: the coverage of data events is a choice

Data events are the expensive half — potentially enormous volume in exchange for answering "who touched this" — so they are usually enabled per resource, and **the selection is where the coverage question lives.**

**And the measured scenario is the ordinary shape of a mistake**: the bucket holding the environment file had no data events, and the bucket holding a report did. **Neither choice was made about sensitivity**, because the report bucket was enabled for an unrelated reason.

> **The coverage of data-plane logging is a decision, and it is usually not made with the question "where is the sensitive data" in mind.**

**Which is what makes the first row of the measurement worth returning to**: the question "who read the environment file" cannot be answered by any log, because nobody chose to answer it.

### Part 7: detection is a chain, not a record

**A single record describes an action; an event is a sequence of them, usually across the planes.** The scenario's intrusion is legible only when the rows are joined:

```
a control-plane event    an access key was created
a data-plane event       a large number of objects were read
a control-plane event    a policy was attached to the identity
a control-plane event    the log was stopped
```

**Each row alone is unremarkable.** A key being created is normal; a policy being attached is normal; reading objects is normal. **What is not normal is the order and the speed**, and that is what a rule matches on.

**And the baseline is what makes the order visible.** Volume, source, time of day and the identity's normal behaviour are all needed before "a lot of reads" means anything.

**And the alerting has the same problem as the scanners measured earlier**: a rule that fires on every record produces a list nobody reads. **The measured conclusion there applies here** — output is triage, so detections should be written as a small number of chains that a person can act on, rather than as one rule per event type.

### Part 8: what follows for security

**Ask of a logging setup what questions it can answer, not whether it is enabled.** Measured, one scenario produced a question with no answer in any log, a question answered only by the data plane, and a question answered only because a second control log existed. **The configuration is a set of decisions about coverage, and reading it does not tell you which questions it covers — asking them does.**

**The two planes are not substitutes.** A control log noticed the policy change and the log being stopped; a data log noticed the read. **An investigation needs both, and the link between them is the identity and the timestamp**, which is why the session principal in Part 3 is the join key.

**And the independent destination is the control that survives a compromise.** Measured, stopping one log left the other recording the stop and everything after it. **A log in the same account as the identity being audited is a log that the incident can end** — and the measured fifth row is what the difference looks like.

**Delivery lag and retention are the two ways a query lies by omission**, in opposite directions. **Both are settings, and the second one is a decision about how long an incident can go unnoticed** — which is not a storage question.

**And data-plane coverage is where the sensitive resources are either covered or not.** Measured, the bucket with the environment file had no data events. **The control is to decide the coverage from what a loss would be worth**, rather than from what was convenient to enable when something else needed it.

**And detection is written as chains.** A record is an action; an event is a sequence across planes and a baseline; **and an alert that fires on every record is the same failure mode as a scanner that reports every match.**

### Detection and mitigation

- **Alert on changes to logging configuration itself**, including stopping a trail, changing a destination and shortening retention, since the measured stop was recorded and is often the first visible step of an incident.
- **Alert on the delivery of logs failing or stopping**, because a gap in a continuous series is visible without knowing what filled it.
- **Send logs to a destination in a different account or trust domain**, so that the identity being audited cannot reach the record of its own actions.
- **Make the storage append-only**, and be able to show that nothing was removed, since a rewritable log is a record of what somebody chose to leave.
- **Set retention from how long an incident can go unnoticed**, not from storage cost.
- **Enable data events for the resources whose contents would matter if they leaked**, rather than for the ones that happened to need them.
- **Write detections as chains across the planes** — a new key, then a policy change, then an unusual volume of reads — with a baseline for each identity.
- **Keep the number of detections small enough to read**, since the measured scanners produce a list and an unread list is not a detection.
- **For mitigation, record the application context that the platform does not**, because the measured log names the session and never the end user.
- **Test the setup by asking questions of it**, one per kind of event worth detecting, and treat an unanswerable question as the finding.
- **Protect the logs as data**, including who can read them, since they contain the same identities and resource names as everything else.
- **And review the two planes together.** The measured scenario needed a control log for what changed and a data log for what was touched, and either alone would have described half an incident.

<!-- lang:zh -->
### 两个平面，而任何一方都答不了对方的问题

日志听起来是一样东西。在云平台上它至少是两样，而这个划分跟随着平台自己的结构。

> **控制面的动作改变规则。数据面的动作使用规则。所以"谁把这个桶设成了公开"与"谁读了这个对象"是两类事件、记在两个地方，而各自在另一处是看不见的。**

下面的实测是一个场景 —— 一次有几步的入侵 —— 然后把五个问题分别去问每一份日志。有意思的结果，是那些没有任何日志能回答的问题，以及那个因为存在第二份日志而答案还在的问题。

### 第一部分：它的形状

| | 控制面 | 数据面 |
|---|---|---|
| 事件 | 建用户、改策略、开关日志 | 读、写、删对象 |
| 量 | 小 | **可能极大** |
| 通常默认开启 | **是** | **否，或者按资源开** |
| 回答 | "谁改了配置" | "谁动了数据" |
| 主体 | 调用者的身份 | 调用者的身份 |

**而日志本身就是一份配置。** 谁能写它、它被投递到哪里、它保留多久、以及它能不能被删掉，都是与别处同一套访问控制之下的设置 —— **这就让日志成为一个要保护的资源，而不是一个可以直接相信的事实。**

**还有一条容易漏掉的轴**：日志记的是那次 API 调用，而那次 API 调用不是应用收到的那个请求。**两者之间的连接住在应用自己的日志里**，这就是为什么一次调查通常需要两份，而其中一份通常是缺的。

### 第二部分：五个问题，三份日志

场景，按顺序：一个部署身份创建了一把访问密钥，一个会话从一个没开数据事件的桶里读了一个对象、从一个开了的桶里读了一个、给自己加了管理员权限、停掉了日志、又读了一个对象、删了一个、并建了一个用户。

对着每一份日志实测：

| 问题 | 主控制日志 | 第二条独立的控制日志 | 数据事件日志 |
|---|---|---|---|
| 谁读了那个没开数据事件的桶里的对象？ | 没有 | 没有 | **没有** |
| 谁读了那个开了的桶里的对象？ | 没有 | 没有 | **有** |
| 谁给自己加了管理员权限？ | **有** | **有** | 没有 |
| 谁停掉了日志？ | **有** | **有** | 没有 |
| 谁在日志停掉之后又建了一个用户？ | **没有 —— 那份日志被停了** | **有** | 没有 |

**三行承载了这一篇。**

**第一行在哪里都没有答案**，因为那个资源从来没有开过数据事件。没有任何东西出故障；那个问题只是不在这份日志被配置来回答的问题之列。

**第二行显示了这个划分的常态**：一次数据面的读取出现在数据日志里，而在两份控制日志里都没有。

**而第五行说明了为什么一份日志不够**：被停掉的那份控制日志在停掉之后什么都没有，而独立的那一份什么都有。

> **"日志里没有"有四种不同的含义：它没有发生、那个资源没开数据事件、那条记录已经过了保留期、或者它还没被投递到。四种在一个查询结果里长得一模一样。**

**这就是为什么关于一份日志有用的问题不是"它开着吗"，而是"这个具体的问题能不能从它这里得到答案"** —— 而弄清楚的办法就是把那个问题问出来。

### 第三部分：主体是一个会话，不是一个角色

实测，一条数据面记录里的身份：

```
GetObject   reports/q3.pdf   arn:aws:sts::111111111111:assumed-role/app-role/i-0abc123
```

**主体是那个会话，包括是哪台实例扮演了它** —— 而不是角色的名字。**这要紧，因为一个角色会被很多工作负载扮演**，所以一条只写角色名的记录说不出是哪一个动的手。

> **日志记的是"谁发出了这个 API 调用"，不是"这个调用是为谁做的"。前者是一个身份，后者是应用的上下文，而只有前者在平台的日志里。**

**所以一个由用户发起、以某个服务的名义、通过一个角色做出的请求，会以真正发出那次 API 调用的身份出现** —— 而"以谁的名义"是应用记录了或者没有记录的一个字段。

**这与讲密钥那一篇关于归因的区分是同一个**：让一个动作可归因的，是它发生时所处的那个会话，而会话正是调查要追的东西。

### 第四部分：停掉一份日志不等于停掉记录

实测，场景里的日志在中途被停掉：

| | 停掉之后 |
|---|---|
| 被停掉那份日志的数据事件 | **0** |
| 主控制日志的事件 | 停了 |
| **第二条独立的控制日志的事件** | **还在记 —— 包括那次停掉本身、以及之后的一切** |

**而"停掉"这个动作被记下来了**，既进了被停的那份，也进了没被停的那份：

```
1000020  StopLogging  trail/main  arn:aws:sts::111111111111:assumed-role/app-role/i-0abc123
```

> **"灯是什么时候灭的"看得见 —— 而那通常就够知道该去看哪里。**

**而独立的那一份才是要紧的部分。** 一个在不同账号或不同信任域里的第二个目的地，是被审计的那个身份够不到的 —— **那就是"一份日志"与"一份事故能改掉的日志"之间的差别。**

> **只留一份日志，等于把某些问题的答案交给想删掉它们的人。**

**而"剩下什么"本身体积常常就是检测。** 一条日志停掉是一个事件；一条本该连续的时间序列上出现一个缺口，不需要知道里面原来是什么就能看见。

### 第五部分：两个边界，投递与保留

实测，两个限制一次查询能返回什么的东西：

| 情形 | 结果 |
|---|---|
| 刚刚发生的事件，立刻查询 | **没有 —— 还没投递到** |
| 同上，等过投递间隔之后 | 有 |
| 一条比保留期更老的记录 | **没有 —— 已过期** |

**两者都正常、都不是错误**，而合在一起意味着一次查询结果说的是一段窗口，而不是历史。

> **"日志里没有"不是"它没有发生"**，而那两个原因在时间上是相反的：一条记录太新，另一条太老。

**这就让保留期成为一个检测决定、而不是一个存储决定。** 一次事故常常在开始很久之后才被发现，**所以一个短于"被发现所需时间"的保留期，是一个回答不了它自己存在的那个问题的期限。**

**而完整性是第三个边界。** 一份能被改写的日志，是"某人选择留下什么"的记录；所以只追加的存储、以及能证明没有东西被移除，才让前面那几行有意义。**要检查的不只是"它投递了吗"，还有"它有可能被改过吗"。**

### 第六部分：数据事件的覆盖面是一个选择

数据事件是贵的那一半 —— 用可能极大的量换"谁碰过这个"的答案 —— 所以它通常是按资源开的，而**那个选择就是覆盖面问题所在的地方。**

**而实测那个场景是一个错误的普通形态**：装着环境变量文件的桶没开数据事件，装着报告的桶开了。**两个选择都不是照着敏感度做的**，报告那个桶是因为别的原因被开上的。

> **数据面日志的覆盖面是一个决定，而它通常不是在想着"敏感数据在哪里"的情况下做出的。**

**这就是为什么实测的第一行值得再回去看一眼**："谁读了那个环境变量文件"这个问题，没有任何日志能回答，因为没有人选择去回答它。

### 第七部分：检测是一条链，不是一条记录

**一条记录描述一个动作；一个事件是一串动作，通常跨平面。** 场景里那次入侵，只有把那些行连起来才读得出来：

```
一条控制面事件    一把访问密钥被创建
一条数据面事件    大量对象被读取
一条控制面事件    一个策略被挂到那个身份上
一条控制面事件    日志被停掉
```

**单独看每一行都平淡无奇。** 创建密钥是正常的；挂策略是正常的；读对象是正常的。**不平常的是顺序和速度**，而规则匹配的正是那个。

**而基线才是让那个顺序可见的东西。** 量级、来源、时间、以及这个身份平时的行为，都要先有，"读了很多"才有意义。

**而告警的问题与前面实测过的扫描器一样**：一条对着每条记录都触发的规则产出的是没人看的清单。**那里得出的结论在这里适用** —— 产出是分诊，所以检测应该写成少量人能据以行动的链，而不是每个事件类型一条规则。

### 第八部分：从这些机制推出的安全观念

**对一套日志配置，要问的是"它能回答哪些问题"，而不是"它开着吗"。** 实测，一个场景产出了一个在哪份日志里都没有答案的问题、一个只有数据面能回答的问题、以及一个只因为存在第二份控制日志才有答案的问题。**那份配置是一组关于覆盖面的决定，而读它并不会告诉你它覆盖了哪些问题 —— 把问题问出来才会。**

**两个平面不能互相替代。** 一份控制日志注意到了策略变更与日志被停；一份数据日志注意到了那次读取。**一次调查两者都要，而它们之间的连接是身份与时间戳**，这就是为什么第三部分那个会话主体是那个连接键。

**而独立的目的地才是能在一次入侵之后仍然成立的控制。** 实测，停掉一份日志之后，另一份还在记那次停掉与之后的一切。**一份放在被审计身份同一个账号里的日志，是一次事故能了结掉的日志** —— 而实测的第五行就是那个差别长什么样。

**投递延迟与保留期是查询用"遗漏"撒谎的两种方式**，方向相反。**两者都是设置，而后者是一个关于"一次事故能被忽视多久"的决定** —— 那不是存储问题。

**而数据面的覆盖面，就是敏感资源被覆盖或没被覆盖的地方。** 实测，装着环境变量文件的桶没有数据事件。**控制手段是按"丢掉它值多少"来决定覆盖面**，而不是按"别的东西需要时顺手开了什么"。

**而检测要写成链。** 一条记录是一个动作；一个事件是跨平面、跨基线的一串动作；**而对着每条记录都触发的告警，与一个报出每一处匹配的扫描器是同一种失败。**

### 检测与缓解

- **对日志配置本身的改动告警**，包括停掉一条、改目的地、缩短保留期 —— 因为实测那次停掉被记了下来，而它常常是一次事故第一个看得见的动作。
- **对日志投递失败或者停止告警**，因为一条本该连续的序列上出现缺口，不需要知道原来是什么就能看见。
- **把日志送到另一个账号或另一个信任域里的目的地**，这样被审计的身份够不到关于自己行为的记录。
- **让存储只追加**，并且能证明没有东西被移除，因为一份可改写的日志是"某人选择留下什么"的记录。
- **按"一次事故能被忽视多久"来定保留期**，而不是按存储成本。
- **为"内容泄露了会在乎"的那些资源开数据事件**，而不是为碰巧需要它的那些。
- **把检测写成跨平面的链** —— 一把新密钥、然后一次策略变更、然后一个不寻常的读取量 —— 并为每个身份准备基线。
- **把检测条数控制到读得完**，因为实测的扫描器产出的是一份清单，而没人读的清单不是检测。
- **缓解上，记录平台不会记的那部分应用上下文**，因为实测那份日志写的是会话、从不写最终用户。
- **用"问它问题"来验证这套配置**，每类值得检测的事件问一个，并把回答不了的问题当成那条发现。
- **把日志当成数据保护**，包括谁能读它，因为它里面装着与其他一切同样的身份与资源名。
- **并且把两个平面放在一起评审。** 实测那个场景需要一份控制日志回答"什么被改了"、一份数据日志回答"什么被碰了"，任何单独一份都只能描述半次事故。
