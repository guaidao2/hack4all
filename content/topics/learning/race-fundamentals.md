---
id: race-fundamentals
title_en: "Race Conditions, Part 1 — The Window Between Checking and Using"
title_zh: "竞态条件（一）：检查与使用之间的窗口"
summary_en: The code is not wrong; the logic is simply not atomic. This entry measures that window with twelve concurrent requests, connects it to the check-then-use failures in the upload and SSRF entries, and names the four situations where it appears.
summary_zh: 代码没有写错；是这段逻辑本身不原子。这一篇用十二个并发请求把那个窗口量出来，把它和上传、SSRF 那两篇里的"先检查后使用"接上，并指出它出现的四种场景。
tags: [web, race-condition, cwe-362, concurrency, toctou]
tools: [python3, curl, Burp Suite, Turbo Intruder]
attck: [T1190, T1565]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The code is correct and the logic is not

A race condition is the second class in this guide that has nothing to do with parsing. The previous entry covered the assumption that a client will use a flow in a certain order; this one covers the assumption that **nothing else happens while this code runs**.

> **Each statement is correct. What is missing is that the sequence has to be indivisible.**

The shape to recognise is always the same two steps:

```
1. read something            if already_claimed: reject
2. act on what was read      mark_claimed(); add_balance()
```

Between those two steps the world can change. A second request can perform step 1 before the first request has performed step 2, so **both read the same "not yet claimed"** and both proceed. Nothing in the code is wrong; the code simply assumed that step 1 and step 2 happen as one thing.

Measured, with twelve concurrent requests against an account holding 100 and an operation costing 100:

| Implementation | Succeeded | Final balance |
|---|---|---|
| Read, then update | **8 of 12** | **-700.00** — charged eight times |
| Condition inside the update | **1 of 12** | 0.00 |

And the same shape for a coupon that may be claimed once, where each successful claim adds 100:

| Implementation | Succeeded | Final balance |
|---|---|---|
| Check, then mark | **7 of 12** | **800.00** — one coupon honoured seven times |
| Condition inside the update | **1 of 12** | 200.00 |

Eight charges out of one balance, and one coupon honoured seven times. **The window in that test was twenty milliseconds**, deliberately widened so the result is stable; production windows are microseconds. That difference changes how hard it is to hit, not whether it exists.

### The same failure as check-then-use, in one more dimension

This is worth connecting explicitly, because the guide has met it twice already under different names:

| Entry | What is checked | What changes before the use |
|---|---|---|
| **File upload** | Whether a file is acceptable | The file is already reachable, and remains so until the verdict |
| **SSRF** | Where a hostname resolves | The name resolves somewhere else when the client connects |
| **Race condition** | Whether an action is allowed | The state that allowed it changes before the action is written |

In each case **the object that was checked and the object that is used are not guaranteed to be the same object at the same time.** The upload and SSRF entries describe it as a boundary problem; here it is easier to see as a **time** problem, which is the more useful framing for fixing it.

### Why a smaller window is not a fix

The natural reaction is that the window is too small to hit. Three things make that reasoning wrong.

**The window is not a fixed width.** It contains the whole of whatever happens between the read and the write: a database round trip, a call to another service, a hash computation, a log line, a garbage collection pause. In the measured example the window was a deliberate sleep; in a real handler it is whatever the handler does.

**Requests can be made to overlap.** Sending many requests at once maximises the chance that two are in the window together, and the attacker controls how many. Twelve threads found the window immediately; a few hundred, sent repeatedly, find a microsecond window too.

**And concurrency is not even required.** A request that is **not idempotent** can be replayed — by a double click, a retried connection, a browser resubmission, a queue delivering a message twice. If the action was supposed to happen once and nothing records that it happened, the second delivery does it again. That is the same defect as the previous entry's refund, and it needs **no timing at all**.

The distinction is worth stating plainly, because it decides where the fix goes:

| Trigger | What it needs | What fixes it |
|---|---|---|
| Concurrent requests | Timing | Atomicity — the check and the act as one operation |
| Replayed request | Nothing | Idempotency — recording that it happened |

Both are usually present, which is why the fixes are usually applied together.

### The four situations where it appears

#### Balance and quota

Anything that spends from a finite pool: an account balance, a credit limit, a gift card, a download quota, an API rate allowance. The check is "is there enough" and the act is "subtract"; between them, another request asks the same question about the same unchanged number.

The measured example is the purest form: 100 in the account, twelve requests each wanting 100, eight of them served.

#### One-time resources

Coupons, invitation codes, trial activations, limited editions, seat reservations, "first 100 customers". The check is "has this been used" and the act is "mark it used". The measured coupon was honoured seven times.

#### Non-idempotent actions

Refunds, transfers, submissions, "confirm" buttons, report generation that charges per run. There may be no concurrency in the attack at all — the same request twice is enough, as the previous entry showed. **The check that would have stopped it is a record, not a condition.**

#### State transitions

Two requests reading the same state and both deciding to move it. "Approve" and "cancel" arriving together; a shipping action and a refund action overlapping; a state machine walked twice in parallel. This is the state-machine entry's problem with the arrow of time added: not only can an invalid sequence be composed, two sequences can be composed **simultaneously**.

### The diagnostic question

One question finds most of this class, and it is worth asking of every handler that writes:

> **Is there a value that was read earlier in this request, and is being relied on now as if it were still true?**

If the answer is yes, the code has a window whose width is however long ago that read was. The follow-up questions order the fixes:

- **Does anything else write that value?** If nothing can, there is no race — and if the value is process-local and written by this request only, there is no race for a different reason.
- **Could this request arrive twice?** If yes, the problem is idempotency, and no amount of locking helps, because the two requests are not simultaneous.
- **What is the shortest single operation that expresses both the check and the act?** That operation — a conditional update, a compare-and-set, an insert with a unique constraint — is the fix, and it does not care how many requests arrive or how close together.

### Detection and mitigation

- **Alert on outcomes that arithmetic says are impossible.** A balance that went negative, a coupon used more times than it exists, a counter exceeding its limit, a total refunded larger than the total paid. These checks are cheap, they use data the application already has, and they catch the result whichever request produced it — which matters because the requests themselves are all ordinary.
- **Alert on duplicated operations against one resource in a short window.** The same coupon, the same order, the same account, the same endpoint, several times within a second. This is the reconnaissance phase as well as the attack: an attacker tuning a payload sends a burst, watches which attempts succeed, and tunes.
- **Record an operation ledger rather than a flag, and alert on its count.** "How many times has this happened" is the question that detects the class — and a boolean flag cannot answer it, for the defender or for the application. Building the ledger is the fix and the detection at once.
- **For mitigation, make the check and the act one operation.** A conditional update — `UPDATE ... WHERE id = ? AND balance >= ?`, checking the affected row count — expresses both, and the database serialises it. Measured against the read-then-write version on the same workload, it turned eight successes into one.
- **Where one statement cannot express the logic, take a lock.** A mutex around the critical section, a row lock, or a distributed lock if more than one instance can serve the request. Measured, a mutex turned eight successes into one as well — at the cost of serialising that section, which is the trade.
- **And add a unique constraint as the last line.** A database constraint on the thing that must happen once — one row per coupon per account, one refund per order — makes the duplicated write fail rather than succeed. Measured, a primary key turned a race into a single success. It is the only one of the four that holds when the application logic is wrong again later.
- **Remember what locking does not fix.** A lock does not help against the replayed request, because there is no overlap to serialise. Idempotency — computing what remains rather than what was requested, and recording the occurrence — is a different control for a different trigger, and the classes overlap often enough that both belong in the same review.

<!-- lang:zh -->
### 代码是对的，逻辑不是

竞态条件是这份指南里第二个与解析毫无关系的类别。上一篇讲的是"客户端会按某个顺序使用流程"这个假设；这一篇讲的是**"这段代码运行期间不会有别的事情发生"**这个假设。

> **每一条语句都是对的。缺的是"这个序列必须是不可分割的"。**

要认出的形状永远是同样两步：

```
1. 读点东西               if already_claimed: reject
2. 根据读到的东西行动      mark_claimed(); add_balance()
```

在这两步之间，世界可以变。第二个请求可以在第一个请求执行完第二步之前执行完第一步，于是**两个请求读到的是同一个"还没被领过"**，两个都往下走。代码里没有任何东西是错的；代码只是假设了第一步和第二步是当成一件事发生的。

实测，十二个并发请求打一个余额 100、每笔操作要花 100 的账户：

| 实现 | 成功 | 最终余额 |
|---|---|---|
| 先读，再更新 | **12 个里 8 个** | **-700.00** —— 扣了八次 |
| 把条件写进更新语句 | **12 个里 1 个** | 0.00 |

而"一张券只能领一次、每成功一次加 100"是同一个形状：

| 实现 | 成功 | 最终余额 |
|---|---|---|
| 先查，再标记 | **12 个里 7 个** | **800.00** —— 一张券被兑现了七次 |
| 把条件写进更新语句 | **12 个里 1 个** | 200.00 |

一个余额被扣了八次，一张券被兑现了七次。**那次测试里的窗口是二十毫秒**，故意放大以便结果稳定；生产里的窗口是微秒级。这个差别改变的是"有多难命中"，不是"它是否存在"。

### 和"先检查后使用"是同一个失效，只是多了一个维度

这一点值得明确接上，因为本指南已经用别的名字遇到它两次了：

| 篇目 | 被检查的是什么 | 在使用之前变了什么 |
|---|---|---|
| **文件上传** | 一个文件是否可接受 | 文件已经可达了，而且会一直可达直到判决出来 |
| **SSRF** | 一个主机名解析到哪里 | 客户端去连的时候，这个名字解析到了别处 |
| **竞态条件** | 一个动作是否被允许 | 允许它的那个状态，在这个动作被写下去之前变了 |

每一种情况下，**被检查的那个对象与正在被使用的那个对象，都不保证是同一时刻的同一个对象。** 上传与 SSRF 那两篇把它描述成一个边界问题；在这里它更容易被看成是一个**时间**问题，而那个视角对修它更有用。

### 为什么"窗口更小"不是修法

自然的反应是"这个窗口小到打不中"。有三件事让这个推理站不住。

**窗口不是一个固定宽度。** 它包含读与写之间发生的一切：一次数据库往返、一次对别的服务的调用、一次哈希计算、一行日志、一次垃圾回收停顿。在实测里那个窗口是一次故意的 sleep；在真实的处理器里，它就是那个处理器所做的一切。

**请求可以被做成重叠的。** 一次发很多请求，最大化了"两个请求同时处在窗口里"的概率，而攻击者控制发多少。十二个线程立刻就打中了窗口；几百个、反复发，微秒级的窗口也打得到。

**而且并发根本不是必需的。** 一个**不幂等**的请求可以被重放 —— 双击、重试的连接、浏览器的重新提交、队列把一条消息投递两次。如果这个动作本该只发生一次，而没有任何东西记录它发生过，第二次投递就会再做一遍。这和上一篇里那个退款是同一个缺陷，而且它**完全不需要时序**。

这个区分值得说清楚，因为它决定修法往哪放：

| 触发方式 | 它需要什么 | 什么能修好它 |
|---|---|---|
| 并发请求 | 时序 | 原子性 —— 把检查与动作变成一次操作 |
| 重放的请求 | 什么都不需要 | 幂等 —— 记录它发生过 |

两者通常同时存在，所以修法通常也一起上。

### 它出现的四种场景

#### 余额与额度

任何从有限池子里支出的东西：账户余额、信用额度、礼品卡、下载配额、API 速率额度。检查是"够不够"，动作是"减掉"；在两者之间，另一个请求对同一个没变的数字问同一个问题。

实测的那个例子是最纯粹的形式：账户里 100，十二个请求各要 100，八个被服务了。

#### 一次性资源

优惠券、邀请码、试用激活、限量版、座位预订、"前 100 名顾客"。检查是"这个被用过吗"，动作是"标记为已用"。实测里那张券被兑现了七次。

#### 不幂等的动作

退款、转账、提交、确认按钮、按次计费的报表生成。攻击里可能**完全没有并发** —— 同一个请求发两次就够了，如上一篇所示。**能挡住它的那道检查是一条记录，不是一个条件。**

#### 状态转换

两个请求读到同一个状态，都决定要推动它。"批准"和"取消"同时到达；发货和退款重叠；一个状态机被并行走了两遍。这是状态机那篇的问题加上了时间之箭：不只是可以拼出一条非法序列，两条序列可以被**同时**拼出来。

### 那个诊断性的问题

一个问题能找出这一类的大部分，值得对每一个会写数据的处理器都问一遍：

> **这个请求里，有没有一个更早读到的值，现在正被当作"它仍然成立"来依赖？**

如果答案是"有"，那么这段代码就有一个窗口，其宽度就是"那次读发生在多久以前"。后续的问题决定修法的顺序：

- **还有别的东西会写那个值吗？** 如果没有，那就没有竞态 —— 而如果这个值是进程内、且只被这个请求写，那是另一个原因导致的没有竞态。
- **这个请求可能到达两次吗？** 如果可能，问题就是幂等性，而加多少锁都没用，因为那两个请求并不同时。
- **能同时表达"检查"与"动作"的最短单次操作是什么？** 那个操作 —— 条件更新、比较并交换、带唯一约束的插入 —— 就是修法，而它不在乎有多少请求、也不在乎它们靠得多近。

### 检测与缓解

- **对"算术上不可能的结果"告警。** 变成负数的余额、被使用次数超过其存在数量的券、超出上限的计数器、退款总额大于实付总额。这些检查很便宜、用的是应用本来就有的数据，而且无论哪一笔请求造成了它都能抓到**结果** —— 这很重要，因为那些请求本身全都平平无奇。
- **对一个资源在短窗口内的重复操作告警。** 同一张券、同一个订单、同一个账号、同一个端点，一秒内好几次。这既是侦察阶段也是攻击本身：一个在调 payload 的攻击者会发一批、看哪些成功、然后调整。
- **记流水账而不是记标志位，并对它的条数告警。** "这件事发生过几次"正是检测这一类的问题 —— 而一个布尔标志位答不了它，对防守方答不了，对应用也答不了。把流水账建起来，同时就是修法和检测。
- **缓解上，让检查与动作成为一次操作。** 一条条件更新 —— `UPDATE ... WHERE id = ? AND balance >= ?`，再检查受影响行数 —— 同时表达了二者，而数据库会把它串行化。在同一个工作负载上，它把实测的八次成功变成了 1 次。
- **当一条语句表达不了那段逻辑时，上锁。** 临界区外加互斥锁、行锁；如果不止一个实例能服务这个请求，就用分布式锁。实测中互斥锁同样把八次成功变成了 1 次 —— 代价是那一段被串行化了，这就是那笔交易。
- **并以一条唯一约束作为最后一道防线。** 对"必须只发生一次"的东西加数据库约束 —— 每个账号一张券、每个订单一次退款 —— 会让那次重复写入**失败**而不是成功。实测里一个主键约束把竞态变成了单次成功。它是这四种里唯一一种在应用逻辑日后再次写错时仍然管用的。
- **记住锁修不好什么。** 锁对重放的请求毫无帮助，因为那里没有重叠可以串行化。幂等 —— 算"还剩多少"而不是"请求要多少"，并记录这次发生 —— 是针对另一种触发的另一种控制，而这两类重叠得足够频繁，所以两者都属于同一次评审。

