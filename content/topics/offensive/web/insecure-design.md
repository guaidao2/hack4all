---
id: insecure-design
title_en: Insecure Design
title_zh: 不安全设计
summary_en: Every other category on this list is a mistake in code. This one is a mistake in the plan — the control was never designed, so there is nothing for the code to get wrong. It is also the category scanners cannot see, because nothing is malformed.
summary_zh: 这份清单上其他类别都是代码写错了。这一类的错在计划里：那个控制从来没被设计出来，所以代码没什么可写错的。它也是扫描器看不见的一类 —— 因为请求没有任何畸形之处。
tags: [web, owasp-a04, insecure-design, business-logic, threat-modeling]
tools: [Burp Suite, ffuf, Turbo Intruder]
attck: [T1190]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The distinction that matters

An injection is a defect: the developer intended to be safe and used the wrong function. Insecure design is different — nobody ever asked "what happens if someone does this on purpose?", so the safe path was never built:

- The login flow was designed to be secure, the account recovery flow was designed to be convenient.
- The payment endpoint validates the price, the coupon endpoint was never given the same treatment because "the UI only offers valid coupons".
- Rate limiting exists on login because a checklist said so, and does not exist on the API because nobody thought of the API as a place a human would attack.

The practical consequence: there is no payload to send, no error to trigger. You have to reason about the system's rules and find where they are *absent* rather than where they are wrong.

### Where to look: model the flow, then abuse it

Take any multi-step feature and write down its states. Then ask the three abuse questions.

**1. What if the steps happen out of order?**

A checkout designed as cart, address, payment, confirm. What happens if you post the confirm request directly, with a total you chose? What if you post payment before address? State machines enforced in the browser are not enforced at all.

```http
POST /checkout/confirm
{"cart_id": 91, "total": 0.01, "address_id": 12}
```

If the server recomputes the total from the cart, this is nothing. If it trusts the field because the UI always sends the right value, that is the finding.

**2. What if the same step happens many times, at once?**

Any operation that should happen once is a candidate: applying a coupon, claiming a reward, transferring points, submitting an order, using a gift card, voting.

- **Sequential repetition**: the second request should fail and often does not.
- **Concurrent repetition**: two requests inside the same window both see "not yet used". Send them simultaneously with Turbo Intruder or `xargs -P`; a single-threaded test will never show it.

```bash
# forty parallel attempts to reuse a one-time code
seq 40 | xargs -P 40 -I{} curl -s -X POST https://target/redeem \
  -H 'Cookie: session=...' -d 'code=SAVE10'
```

**3. What if a legitimate feature is used as intended, but too much?**

Abuse is often not a bypass at all: the feature works exactly as designed and the design has no quota. Bulk export, invitation emails, SMS sending, password reset requests, search with wildcards, "add a friend" — these are the features that turn into spam relays, enumeration oracles and resource exhaustion.

### Design flaws that are not one endpoint

Some of this category is structural rather than local:

- **A permission model too coarse to express the requirement.** If the system has only `admin` and `user`, then any feature that needs "this user sees only their department" will be implemented as a filter in the client, or forgotten. The bug appears everywhere at once.
- **Trust between internal components.** Microservices that assume "only the gateway calls us" and validate nothing create one entry point that bypasses every control in the system. This is why an exposed internal service is so much worse than its apparent functionality suggests.
- **Recovery weaker than authentication.** A system with strong login and a reset flow that answers "is this email registered" has a login that is only as strong as the reset.
- **Default open.** New endpoints are reachable until someone adds an authorization check, rather than denied until someone adds an exception. Every forgotten endpoint becomes a bug; the reverse default fails closed.
- **No separation between read and write authorization.** The ability to view an object is granted, and the ability to modify it follows implicitly.

### How to test without a payload

The method is structured curiosity, not fuzzing:

1. **List the invariants.** What must always be true? A balance never goes negative, a coupon is used once, an order is paid before it ships, a user sees only their own data.
2. **List the flows that could break them.** Then trace each one through the API rather than the UI: every request the client makes is a request you can make differently.
3. **For each request, ask what the server verifies.** If the answer is "the client sent it correctly", the invariant is client-side.
4. **Change the order, the count, the concurrency and the values.** One at a time, from a state you can return to.
5. **Watch for the features with no limit.** Time how long it takes to trigger an email, an SMS, or a report generation, and whether anything stops you at ten, a hundred, or never.

### Detection

Design flaws are visible as behaviour, not as attack strings:

- **A legitimate endpoint called at a rate no human reaches** — the coupon endpoint is the classic; alert on volume per user per feature, not just per IP.
- **Sequences that skip steps**: a confirm request without the preceding payment, a download without the preceding generation.
- **The same one-time token used twice**, which shows up as a constraint violation or a duplicate in the database rather than as an attack signature.
- **Impossible states**: a balance below zero, an order shipped unpaid, a coupon redeemed more times than its limit. Detect them as invariants, because the request that created them looked fine.
- **Third-party complaints** (the first sign of an email or SMS relay).

### Mitigation

- **Threat-model new features before writing them**: who benefits from abusing this, and what would that look like? The abuse cases are the requirement; without them, no amount of careful coding produces the missing control.
- **Enforce state transitions on the server.** Each step validates that the system is in the state the step assumes, not that the request looks plausible.
- **Recompute, do not trust.** Prices, totals, permissions and identifiers come from the server's own data, never from the request.
- **Make one-time operations idempotent with a key**, so a repeated or concurrent request is rejected by the database rather than by hope. A unique constraint is a better control than a check-then-write.
- **Default deny**: an endpoint without an explicit authorization decision should be unreachable, not open.
- **Design the permission model for the requirement**, including tenancy, sharing and delegation, rather than retrofitting filters per endpoint.
- **Add quotas and cost to expensive or externally visible actions** (email, SMS, exports, searches), with per-account limits and alerting on the ceiling.
- **Do not let recovery be weaker than login**: the same rate limits, the same enumeration resistance, the same monitoring.

<!-- lang:zh -->
### 这个区分很要紧

注入是缺陷：开发者本意是安全的，只是用错了函数。不安全设计不一样 —— 从来没有人问过"如果有人**故意**这么做会怎样"，所以那条安全的路根本没被建出来：

- 登录流程是按"安全"设计的，账户找回流程是按"方便"设计的。
- 支付接口校验了价格，优惠券接口没有同样待遇，因为"界面只会给出有效券"。
- 登录有限流是因为清单上要求了，API 没有是因为没人把 API 当成"人会来打的地方"。

实际后果是：没有 payload 可发，没有报错可触发。你必须推理系统**本应**有什么规则，然后找出规则**缺失**的位置，而不是写错的位置。

### 从哪里入手：把流程画出来，然后滥用它

任取一个多步骤功能，把它的状态写下来。然后问这三个"滥用问题"。

**一、如果这些步骤乱序发生会怎样？**

一个"购物车 → 地址 → 支付 → 确认"的结账流程。如果你直接提交确认请求、并自己指定金额呢？如果先提交支付、后提交地址呢？在浏览器里强制执行的**状态机等于没有强制执行**。

```http
POST /checkout/confirm
{"cart_id": 91, "total": 0.01, "address_id": 12}
```

如果服务端从购物车重新计算金额，这不构成问题。如果它相信这个字段 —— 因为界面总是发对的值 —— 那就是发现。

**二、如果同一步骤重复很多次、而且同时发生呢？**

任何"本应只发生一次"的操作都是候选：用券、领奖励、转积分、下单、用礼品卡、投票。

- **顺序重复**：第二次请求本该失败，而它往往不会。
- **并发重复**：两个请求在同一个时间窗口里都读到"尚未使用"。用 Turbo Intruder 或 `xargs -P` 同时发出去；单线程测试永远看不到这个。

```bash
# 四十个并发，试图重复使用一次性兑换码
seq 40 | xargs -P 40 -I{} curl -s -X POST https://target/redeem \
  -H 'Cookie: session=...' -d 'code=SAVE10'
```

**三、如果一个正常功能被按设计使用，但用得太多呢？**

滥用往往根本不需要绕过：功能完全按设计工作，而设计里没有配额。批量导出、邀请邮件、发短信、请求重置、带通配符的搜索、"加好友" —— 这些功能会变成垃圾邮件中转站、枚举预言机和资源耗尽点。

### 不是单接口层面的设计缺陷

这一类里有些是结构性的，不是局部的：

- **权限模型太粗，表达不了需求。** 如果系统只有 `admin` 和 `user`，那任何需要"这个用户只能看自己部门"的功能，就会被实现成客户端过滤，或者干脆被忘掉。于是 bug 会同时出现在所有地方。
- **内部组件之间的信任。** 假设"只有网关会调用我们"、于是什么都不校验的微服务，制造出一个绕过系统内所有控制的入口。这也是"暴露的内部服务"为什么远比它表面功能危险。
- **找回流程弱于登录本身。** 一个登录很强、但重置流程会回答"这个邮箱注册过吗"的系统，其登录强度只等于重置流程的强度。
- **默认开放。** 新接口在有人加上授权检查之前一直是可访问的，而不是在有人加白名单之前不可访问。每个被遗忘的接口都变成 bug；反过来默认拒绝则会安全地失败。
- **读写授权没有分开。** 授予了"能看某个对象"，"能改它"就顺带成立了。

### 没有 payload 时怎么测

方法是结构化的好奇心，不是 fuzzing：

1. **列出不变量。** 什么必须永远为真？余额不为负、券只能用一次、订单先付款后发货、用户只能看自己的数据。
2. **列出可能破坏它们的所有流程。** 然后沿 API 而不是界面走一遍：客户端发的每个请求，你都能换个方式发。
3. **对每个请求问：服务端到底校验了什么？** 如果答案是"客户端发对了"，那这个不变量就只在客户端。
4. **改变顺序、次数、并发和取值。** 一次只改一个，并且从你能回退到的状态出发。
5. **盯住那些没有限制的功能。** 计时触发一封邮件、一条短信、一次报表生成需要多久，然后在十次、一百次、还是永远不停的时候被拦住。

### 检测

设计缺陷表现在行为上，而不是攻击字符串上：

- **一个正常接口被以人类达不到的频率调用** —— 优惠券接口是典型；按"每用户每功能的调用量"告警，而不只是按 IP。
- **跳过步骤的序列**：没有前置支付就来的确认请求，没有前置生成就来的下载请求。
- **同一个一次性 token 被用了两次**，它表现为约束冲突或数据库里的重复记录，而不是攻击特征。
- **不可能的状态**：负余额、未付款已发货的订单、超额兑换的券。把它们当不变量去检测，因为造成它们的那个请求看起来完全正常。
- **第三方投诉**（邮件或短信中转站的第一个信号）。

### 缓解

- **写新功能之前先做威胁建模**：谁会从滥用它获利，那会是什么样子？这些滥用用例就是需求；没有它们，再怎么小心写代码也写不出那个缺失的控制。
- **在服务端强制状态流转。** 每一步都要校验系统确实处于该步骤所假设的状态，而不是校验"请求看起来合理"。
- **重新计算，不要相信。** 价格、金额、权限、标识都取自服务端自己的数据，绝不取自请求。
- **让一次性操作靠幂等键生效**，让重复或并发请求由数据库拒绝，而不是靠"希望如此"。唯一约束比"先查后写"是更好的控制。
- **默认拒绝**：没有明确授权决定的接口应当不可达，而不是开放。
- **按需求设计权限模型**，把租户、共享、委派都考虑进去，而不是在每个接口上事后补过滤器。
- **给昂贵或对外可见的操作加上配额与成本核算**（邮件、短信、导出、搜索），按账号限制，并在触顶时告警。
- **不要让找回流程弱于登录**：同样的限流、同样的抗枚举、同样的监控。
