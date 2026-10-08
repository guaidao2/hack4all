---
id: api-authorization-bola-bfla
title_en: "API Authorization — BOLA and BFLA Are Not the Same Problem"
title_zh: "API 授权：BOLA 与 BFLA 不是同一个问题"
summary_en: The caller is already authenticated, which is what makes both classes invisible to an authentication review. Measured separately — one is about somebody else's data behind the same function, the other about somebody else's capability behind the same identity.
summary_zh: 调用者已经通过认证了，这正是这两类能躲过认证评审的原因。分别实测：一个是"同一个功能、别人的数据"，另一个是"同一个身份、别人的能力"。
tags: [web, api, idor, bola, bfla, authorization, owasp-api]
tools: [curl, python3, Burp Suite]
attck: [T1078, T1190]
platform: [web, api]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Both are invisible to an authentication review

The two most common serious API findings share a property that makes them easy to miss:

> **The caller is already authenticated.**

There is no broken login, no missing token, no expired session. A reviewer checking the authentication layer finds it in order. What is missing is a second question, asked at a different place, and it comes in two forms that are routinely conflated.

| | **BOLA** — object level | **BFLA** — function level |
|---|---|---|
| The question | **Is this object yours?** | **May you use this capability?** |
| Measured case | `alice` requesting `bob`'s order | `alice` calling the admin endpoint |
| Same function, somebody else's **data** | Same identity, somebody else's **capability** | |
| Where it is fixed | At **every operation that takes an id** | At the **route**, as a role check |
| Measured: check only login | `200` with `bob`'s order | `200` with the user list |
| Measured: check the second thing | `403` | `403` |

**BOLA** (broken object level authorization) is the IDOR entry with an API shape: the endpoint knows who you are and acts on whatever identifier it is given.

**BFLA** (broken function level authorization) is a different failure: the endpoint is the right one for you to reach, and it does something only some roles may do. Measured, an authenticated non-admin calling `/admin/users` receives the user list because the handler checked the session and stopped there.

**Why the distinction matters is the fix.** BOLA is fixed at each object operation, because each one is a separate opportunity to forget. BFLA is fixed at the route, because "who may call this at all" is one decision per endpoint. An API can be excellent at one and hopeless at the other, and a report that says "authorization is missing" without saying which has not said enough to act on.

### Four layers

1. **The trust boundary.** The application trusts that being authenticated implies being permitted — for this object, or for this capability. **"Who are you" and "may you do this" are two questions**, and the first one being answered well says nothing about the second.
2. **Data and instruction share a plane.** The identifier in the path is simultaneously **which object to fetch** and **the thing authorization should be about**. The handler uses it for the first and never considers it as the second, so a value chosen by the client decides whose data comes back.
3. **Why the usual fixes fail.** "We have authentication" is the most common reason this survives review, and it is true. Hiding the button or the route in the front end changes nothing, because an API is reachable directly. And unpredictable identifiers — UUIDs instead of sequence numbers — make discovery harder while granting nothing: **an unguessable id is obscurity, not authorization.**
4. **The variants.** The identifier appearing in more than one place, endpoints taking a list of identifiers, nested resources where only one segment is checked, and methods where the check exists on one verb and not another.

### Why APIs make it worse

**The shape of a REST API is `/resource/<id>`.** Acting on a client-supplied identifier is not an oversight in that design, it is the design — which means the number of places where the ownership question must be asked is the number of endpoints, and each one is a separate opportunity to leave it out.

**A user interface accidentally provides an authorization layer.** A page shows the orders belonging to the logged-in user because it queried them that way; the buttons for "cancel" and "refund" appear only where they apply. **The API behind it exposes the full surface**, and none of that accidental protection travels with the request.

**And the variants that matter are shapes, not payloads.** A path parameter and a body field naming the same object; a batch endpoint taking an array; a nested route like `/users/<me>/orders/<other>` where only the first segment was checked. Each is the same rule violated in a place the reviewer did not look.

### Detection and mitigation

- **Alert when one account requests a run of identifiers it does not own.** Enumeration is the discovery step for BOLA and it is loud: sequential ids, one account, a short window, a high proportion of `403`s or `404`s. The signal is the pattern, not any single request.
- **Compare every returned object against the caller's ownership.** A successful unauthorized read looks exactly like a legitimate one in the logs — a `200` with a body — so the detection has to compare the returned resource's owner against the session, not watch for an error.
- **Alert on non-administrative accounts reaching administrative routes at all**, whatever the status code. The attempt is the finding, and a `403` tells you the route is being probed rather than that nothing happened.
- **And record the authorization decision with the request.** "Allowed because role=admin" and "allowed because owner matches" are what makes an incident reviewable, and the absence of that field is why an incident involving authorization takes days to scope.
- **For mitigation, make ownership part of the query rather than a check after it.** `SELECT ... WHERE id = ? AND owner = ?` returning nothing for somebody else's object means the caller gets a `404` — which is both the correct answer and **a better one than `403`**, because a `403` confirms the object exists. This is the single most effective control for BOLA and it is also the one that is hardest to forget, since the filter lives where the data is fetched.
- **Check the role at the route, once per endpoint.** For BFLA the decision is "may this class of caller reach this handler at all", so it belongs where the route is declared — and a default-deny posture, where a new endpoint requires an explicit authorization decision, removes the class of endpoint somebody added without one.
- **Check every element of a batch, and every segment of a nested path.** A check performed on the first id of a list or the first segment of a path is the same "one of several routes" mistake this guide has described for WAF bypasses and CSRF tokens.
- **And do the checks on every method that reaches the handler.** Where a route accepts `GET`, `PUT` and `DELETE`, an ownership check written in the `GET` branch leaves two ways to reach the same object.
- **Do not rely on identifiers being unguessable.** UUIDs and hashes raise the cost of discovery and change nothing about authorization — and a service that switched to UUIDs to fix an IDOR finding has fixed the symptom that made it findable rather than the finding.
- **And test with two accounts, in both directions.** The test that catches this class is mechanical: log in as A, request B's objects and B's capabilities, and assert a refusal — for every endpoint, including the ones added last.

<!-- lang:zh -->
### 两者对认证评审都是隐形的

最常见的两个严重 API 发现，共有一个让它们容易被漏掉的性质：

> **调用者已经通过认证了。**

没有坏掉的登录、没有缺失的令牌、没有过期的会话。一个检查认证层的评审会发现它一切正常。缺的是第二个问题，问在另一个地方，而它有两种形式，且经常被混为一谈。

| | **BOLA** —— 对象级 | **BFLA** —— 功能级 |
|---|---|---|
| 问的是 | **这个对象是你的吗？** | **你有权用这个能力吗？** |
| 实测的情形 | `alice` 请求 `bob` 的订单 | `alice` 调用管理员端点 |
| 同一个功能、别人的**数据** | 同一个身份、别人的**能力** | |
| 修在哪 | **每一个接受 id 的操作** | **路由上**，作为角色检查 |
| 实测：只检查登录 | `200`，返回 `bob` 的订单 | `200`，返回用户列表 |
| 实测：检查第二件事 | `403` | `403` |

**BOLA**（对象级授权缺陷）就是 IDOR 那篇的 API 形状：端点知道你是谁，然后对你给它的任何标识符照做。

**BFLA**（功能级授权缺陷）是另一种失效：那个端点对你来说是可以到达的，而它做的事只有部分角色能做。实测，一个已认证的非管理员调用 `/admin/users` 拿到了用户列表，因为那个处理器检查了会话就停下了。

**区分它们之所以要紧，在于修法。** BOLA 修在每一个对象操作上，因为每一个都是一次单独的"可能忘掉"的机会。BFLA 修在路由上，因为"谁可以调用这个"是每个端点一个决定。一个 API 可以在其中一个上做得很好、在另一个上无可救药，而一份只说"缺少授权"、不说清是哪一种的报告，没有说到可以据以行动的程度。

### 四层

1. **信任边界。** 应用信任"已认证就意味着被许可" —— 对这个对象、或者对这个能力。**"你是谁"和"你能不能做这个"是两个问题**，而第一个答得好，对第二个什么也没说。
2. **数据与指令共用同一平面。** 路径里的那个标识符同时是**"要取哪个对象"**和**"授权判断本该针对的东西"**。处理器用了它做第一件事，从不把它当成第二件事，于是一个由客户端选定的值决定了返回谁的数据。
3. **为什么常见修法失败。** "我们有认证"是这类问题活过评审最常见的原因，而它是真的。在前端隐藏那个按钮或路由什么也改变不了，因为 API 是可以直接到达的。而不可预测的标识符 —— UUID 而不是自增数字 —— 只是让发现变难，什么也没授予：**猜不到的 id 是隐蔽性，不是授权。**
4. **变体。** 标识符出现在不止一个位置、端点接受一组标识符、嵌套资源里只检查了其中一段、以及检查存在于一个方法而另一个方法上没有。

### 为什么 API 让它更糟

**REST API 的形状就是 `/resource/<id>`。** 对一个由客户端提供的标识符采取动作，在那个设计里不是疏忽，它就是那个设计 —— 这意味着"必须问归属问题"的地方的数量等于端点的数量，而每一个都是一次单独漏掉它的机会。

**用户界面在无意中提供了一层授权。** 一个页面显示属于当前登录用户的订单，是因为它就是这么查询的；"取消"和"退款"的按钮只出现在适用的地方。**它背后的 API 暴露的是完整的操作面**，而那层无意的保护一点也不会随请求一起过去。

**而真正要紧的变体是形状，不是 payload。** 一个路径参数和一个请求体字段指向同一个对象；一个接受数组的批量端点；一条像 `/users/<me>/orders/<other>` 的嵌套路由，那里只检查了第一段。每一个都是同一条规则在评审没看的地方被违反。

### 检测与缓解

- **当一个账号连续请求它并不拥有的一串标识符时告警。** 枚举是 BOLA 的发现步骤，而且它很响：连续 id、一个账号、很短的窗口、`403` 或 `404` 占比很高。信号是这个模式，而不是任何单独一笔请求。
- **把每一个返回的对象与调用者的归属做比对。** 一次成功的未授权读取在日志里看起来完全像一次合法读取 —— 一个带响应体的 `200` —— 所以检测必须把返回资源的属主与会话比对，而不是去等一个错误。
- **对非管理账号"到达过"管理路由告警**，无论状态码是什么。那次尝试就是发现，而一个 `403` 告诉你这条路由正在被探测，而不是"什么也没发生"。
- **并且把授权决定与请求一起记下来。** "因为 role=admin 而被允许"和"因为属主匹配而被允许"，是让一次事故可以被复盘的东西；而那个字段的缺失，正是一次涉及授权的事故要花好几天才能圈定范围的原因。
- **缓解上，把归属做成查询的一部分，而不是查询之后的一道判断。** `SELECT ... WHERE id = ? AND owner = ?` 对别人的对象返回空，于是调用者拿到 `404` —— 那既是正确答案，也**比 `403` 更好**，因为 `403` 确认了那个对象存在。这是对 BOLA 最有效的一项控制，也是**最难忘记**的一项，因为那个过滤条件就住在数据被取出的地方。
- **在路由上检查角色，每个端点一次。** 对 BFLA 来说，那个决定是"这一类调用者能不能到达这个处理器"，所以它属于声明路由的地方 —— 而一个"默认拒绝"的姿态，即新端点必须显式做出授权决定，移除了"有人加了一个端点却没加检查"这一类。
- **检查批量里的每一个元素、嵌套路径里的每一段。** 只对一组 id 里的第一个、或者一条路径的第一段做检查，就是这份指南在 WAF 绕过与 CSRF token 上描述过的同一个"若干路径中的一条"的错误。
- **并且在每一个能到达那个处理器的方法上都做检查。** 一条路由同时接受 `GET`、`PUT` 和 `DELETE` 时，写在 `GET` 分支里的归属检查留下了两条到达同一个对象的其他方式。
- **不要依赖标识符猜不到。** UUID 和哈希提高了发现的成本，而对授权什么都没改变 —— 而一个"为了修一个 IDOR 发现就换成 UUID"的服务，修掉的是那个让它可被发现的症状，而不是那条发现。
- **并且用两个账号、双向地测。** 抓住这一类的测试是机械的：用 A 登录，请求 B 的对象和 B 的能力，断言被拒绝 —— 对每一个端点，包括最后加的那些。
