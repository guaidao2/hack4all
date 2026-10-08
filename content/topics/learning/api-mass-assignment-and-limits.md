---
id: api-mass-assignment-and-limits
title_en: "APIs — Mass Assignment, Rate Limits and What Leaves in the Response"
title_zh: "API：批量赋值、限流，以及响应里带走了什么"
summary_en: A framework that maps a request body onto an object accepts every field by default, a quota keyed on something the client controls is not a quota, and a response containing fields the page never renders still sent them. Measured on all three.
summary_zh: 一个把请求体映射到对象的框架默认接受所有字段；一个用客户端能控制的东西做键的配额不算配额；而一个含有页面从不渲染的字段的响应，仍然把它们发出去了。三件事都做了实测。
tags: [web, api, mass-assignment, rate-limiting, data-exposure, owasp-api]
tools: [curl, python3, browser devtools]
attck: [T1078, T1499, T1213]
platform: [web, api]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Three questions about one request

The previous entry covered whether a caller may touch an object or a capability. This one covers three questions about a request that is already allowed to arrive:

1. **Which fields may it set?**
2. **How often may it ask?**
3. **What comes back in the answer?**

All three have the same shape as everything else in this guide — the application has to decide something and, by default, decides it in the caller's favour.

### Mass assignment: the convenience that accepts everything

Every web framework has a facility for taking a request body and applying it to a model object. It exists because writing the mapping by hand for every field is tedious, and it is convenient for exactly the reason it is dangerous: **its default is to accept everything.**

The measured comparison, on a model with `name`, `role`, `balance` and `verified`, against a payload setting all of them:

| Implementation | Fields actually changed |
|---|---|
| Merge the body into the model | `role`, `balance`, `verified` |
| Denylist `role` and `is_admin` | **`balance`, `verified`** — the list had two names, the model had more |
| Denylist, plus a nested payload | **`profile.role` set to admin** — see below |
| Allowlist `name`, `email` | **nothing** |

**The denylist row is the argument against denylists in this class.** It blocked the field somebody thought of and allowed two they did not, on a model whose writable surface is defined by its own schema rather than by anyone's memory. And the nested payload shows why the list cannot be completed by trying harder: the field that matters is not always at the top level.

```
top-level role       -> blocked by a top-level denylist
{"profile": {"role": "admin"}}   -> not blocked, because the check only looked at the first level
```

A denylist has to enumerate **both the field names and the level they live at**, while an allowlist does not care about either. That asymmetry is the whole reason the allowlist wins here, and it is the same conclusion as every filter in this guide: **the set of things that should be rejected is open-ended; the set of things that should be accepted is the application's own data model.**

**Four layers:**

1. **The trust boundary.** The application trusts that the fields a client submits are the fields it is permitted to change. Both sides behave correctly — the client sent a well-formed body, the framework bound it faithfully.
2. **Data and instruction share a plane.** The request body is simultaneously **the values to write** and **the declaration of which fields may be written**. The second of those is a decision that belongs to the application, and the framework hands it to the caller by default.
3. **Why the usual fix fails.** A denylist of sensitive-sounding names has to anticipate the schema, the naming conventions and the nesting of every model it protects. It fails at all three, and it fails silently.
4. **The variants.** Nested objects, associated records, arrays of objects, and — in JavaScript — `__proto__` and `constructor` keys, which is the prototype pollution entry arriving through the same binding.

**The fix is to declare the writable surface.** An explicit allowlist, or a data-transfer object listing exactly the fields a given caller may set, in code, per operation. A model-level "these attributes cannot be assigned" marker is useful as a second layer — and it is not a first one, because nesting and associations are exactly where it is incomplete.

### Rate limits: the dimension has to be one the attacker cannot change

A rate limit is a quota, and a quota is only meaningful if its key is stable. Measured, with a limiter keyed on the client address and a client that varies one header:

| Trusts `X-Forwarded-For` | Requests allowed out of 12 |
|---|---|
| no | 3 |
| **yes** | **12** |

**A quota keyed on something the caller supplies is not a quota.** The forwarding headers are set by proxies, so trusting them when no trusted proxy set them means the key is attacker-chosen — the same rule as the host header entry, in the place where the consequence is an unlimited number of attempts rather than a poisoned password reset.

And the key has to be **normalised**, because the same endpoint has more than one spelling:

| Request shapes | Distinct strings counted |
|---|---|
| `/login` repeated | 1 |
| `/login`, `/Login`, `/LOGIN` | 3 |
| `/login`, `/login/` | 2 |
| `/login`, `/login?x=1` | 3 |

If the counter is keyed on the raw path, every variant gets its own budget. Lowercasing, stripping a trailing slash and ignoring the query string are the minimum normalisations, and each one is a bypass if omitted.

**And the layer decides what can be counted.** A gateway sees requests: count, address, path, method. It cannot see **whether this one was a failed login**, because that is a decision made inside the application. The application can see it, but only if it is still able to process requests. So:

| Layer | Counts | Cannot count |
|---|---|---|
| Gateway / CDN | Volume, address, path, method | Semantic outcomes — was this attempt a failure? |
| Application | Per account, per operation, per failure | Anything, once it is saturated |

**The two layers are counting different things, which is why both are needed.** A login limit and an account-lockout rule belong in the application; a volumetric limit belongs at the edge; and a deployment with only one of them is open to whichever attack the other one covers.

### What leaves in the response

The third question, and the one that needs no attack at all. Measured, serialising a database row directly versus serialising a chosen set of fields:

| Implementation | Fields in the response |
|---|---|
| Serialise the row | `api_key`, `email`, `id`, `name`, `password_hash`, `role`, `ssn` |
| A DTO with two fields | `id`, `name` |

**"The front end does not display it" is not "it was not sent."** The page rendered `name`; the response body carried a password hash, an API key and a national identifier, and anyone who opens the developer tools' network panel — no tooling required — reads all three. This is the information disclosure entry with an API-shaped cause: the query was `SELECT *`, and the serialiser was the whole object.

**And a response is just as much an interface as a request.** The allowlist logic applies in both directions, which is the least intuitive part of this entry: developers who carefully validate input will often serialise output with no filter at all, because the fields "belong to the user anyway" — except for the ones that do not.

### Detection and mitigation

- **Alert when a request body carries a field that is not in the endpoint's documented schema.** For a bound request body this is exactly the set of fields that will be written, so comparing it against the schema is the detection and the finding at once.
- **Alert specifically on authorisation-adjacent field names in request bodies.** `role`, `is_admin`, `permissions`, `balance`, `verified`, `owner_id` — arriving from a client, these are never routine, and the list is short enough to keep.
- **Alert when a privilege or balance changes without a corresponding administrative action.** This is the outcome check, and it holds whichever endpoint and whichever field produced the change.
- **Alert when one account's failed attempts arrive from a spread of addresses.** That is the fingerprint of a quota keyed on something the client controls, and it is visible wherever the attempts are logged.
- **And alert on response payloads that are larger or wider than the endpoint's contract.** A response carrying a field the documentation never mentions is worth a look, and a response whose field count jumped after a release is worth more.
- **For mitigation, bind by allowlist.** An explicit list per operation — a DTO, a form class, a parameter schema — so that the writable surface is a line of code rather than a framework default. This is the only one of the controls here that is complete by construction.
- **Add a model-level guard as a second layer, and do not rely on it alone.** Marking attributes as non-assignable catches the obvious mistakes and misses the nested ones, which is why it follows the allowlist rather than replacing it.
- **Key rate limits on something the attacker cannot choose, and normalise it.** The account or session for semantic limits, the real peer address for volumetric ones, with the path normalised before it becomes part of a key — and with forwarding headers ignored unless a trusted proxy sets them.
- **Put the semantic limit in the application and the volumetric one at the edge.** They answer different questions, and each is blind to the other's.
- **And filter the response as carefully as the request.** A response allowlist per endpoint, or a serialiser bound to a schema, so that adding a column to the table does not add it to the API. The general rule this entry shares with all the others: **input, output and quota each need a set of things that are allowed, declared by the application rather than left open by default.**

<!-- lang:zh -->
### 关于同一个请求的三个问题

上一篇讲的是一个调用者可不可以碰某个对象或某个能力。这一篇讲一个"已经被允许到达"的请求身上的三个问题：

1. **它可以设置哪些字段？**
2. **它可以问多少次？**
3. **答案里会带回什么？**

三者与这份指南里的其他东西形状相同 —— 应用必须决定某件事，而默认情况下它决定的是"向着调用者"的那一边。

### 批量赋值：一个"全都接受"的便利功能

每个 Web 框架都有一种设施，用来把请求体套用到模型对象上。它存在是因为手写每个字段的映射很烦，而它危险的原因恰好也是它便利的原因：**它的默认是接受一切。**

在一张有 `name`、`role`、`balance`、`verified` 的模型上，对着一个把四个字段全设上的请求体实测：

| 实现 | 实际被改的字段 |
|---|---|
| 把请求体合并进模型 | `role`、`balance`、`verified` |
| 黑名单 `role` 与 `is_admin` | **`balance`、`verified`** —— 名单上两个名字，而模型上的更多 |
| 黑名单 + 一个嵌套载荷 | **`profile.role` 被设成 admin** —— 见下 |
| 白名单 `name`、`email` | **什么都没改** |

**黑名单那一行就是这一类里反对黑名单的论据。** 它挡住了有人想到的那个字段，放过了两个他没想到的，而这是一张"可写面由它自己的 schema 定义、而不是由谁的记忆定义"的模型。而那个嵌套载荷说明了为什么这份名单没法"再努力一点"补全：要紧的字段并不总在顶层。

```
顶层的 role            -> 被顶层黑名单挡住
{"profile": {"role": "admin"}}   -> 没被挡住，因为检查只看了第一层
```

一份黑名单必须**同时**枚举字段名**和**它们所在的层级，而白名单两者都不在乎。这个不对称就是白名单在这里胜出的全部原因，也是这份指南里每一个过滤器得到的同一个结论：**应该被拒绝的集合是开放的；应该被接受的集合就是应用自己的数据模型。**

**四层：**

1. **信任边界。** 应用信任"客户端提交的字段，就是它被允许修改的字段"。两边行为都正确 —— 客户端发了一个良构的请求体，框架忠实地绑定了它。
2. **数据与指令共用同一平面。** 请求体同时是**要写入的值**和**对"哪些字段可被写入"的声明**。后者本该是应用的决定，而框架默认把它交给了调用者。
3. **为什么常见修法失败。** 一份"听起来敏感的名字"的黑名单，必须预先想到它保护的每一张模型的 schema、命名习惯与嵌套。三件事它都做不到，而且它失败得无声无息。
4. **变体。** 嵌套对象、关联记录、对象数组，以及在 JavaScript 里的 `__proto__` 与 `constructor` 键 —— 那是原型污染那篇经由同一个绑定到达。

**修法是声明可写面。** 一份显式的允许清单，或者一个数据传递对象，在代码里、按操作，列出这个调用者恰好可以设置哪些字段。在模型层标一个"这些属性不可赋值"是有用的第二层 —— 而它不是第一层，因为**嵌套与关联恰好是它不完整的地方**。

### 限流：维度必须是攻击者改不了的那一个

限流是一份配额，而配额只有在它的键稳定时才有意义。实测，用一个以客户端地址为键的限流器，客户端只变一个头：

| 信任 `X-Forwarded-For` | 12 次里放行 |
|---|---|
| 不信任 | 3 |
| **信任** | **12** |

**一个用调用者提供的东西做键的配额不算配额。** 转发头是由代理设置的，所以在没有可信代理设置它们时信任它们，等于那个键由攻击者选 —— 和 Host 头那篇同一条规则，只是后果从"被投毒的口令重置"变成"不限次数的尝试"。

而那个键还必须**被规范化**，因为同一个端点有不止一种写法：

| 请求形状 | 计到的不同字符串 |
|---|---|
| 重复 `/login` | 1 |
| `/login`、`/Login`、`/LOGIN` | 3 |
| `/login`、`/login/` | 2 |
| `/login`、`/login?x=1` | 3 |

如果计数器以原始路径为键，每一种变体都拿到自己的一份预算。转小写、去掉结尾斜杠、忽略查询串是最低限度的规范化，而漏掉任何一条都是一次绕过。

**而"在哪一层"决定了能数到什么。** 网关看到的是请求：数量、地址、路径、方法。它看不到**这一次是不是一次失败的登录**，因为那是一个在应用内部做出的判断。应用能看到，但前提是它还有能力处理请求。所以：

| 层 | 能数 | 数不到 |
|---|---|---|
| 网关 / CDN | 流量、地址、路径、方法 | 语义结果 —— 这一次尝试是不是失败 |
| 应用 | 按账号、按操作、按失败 | 一旦被打满，什么都数不到 |

**两层数的是不同的东西，这就是两层都要的原因。** 登录次数限制与账号锁定规则属于应用；流量型限制属于边缘；而只做了其中一层的部署，对另一层所覆盖的攻击是敞开的。

### 响应里带走了什么

第三个问题，也是唯一一个根本不需要攻击的问题。实测，直接序列化一行数据库记录，与只序列化选定的一组字段：

| 实现 | 响应里的字段 |
|---|---|
| 序列化整行 | `api_key`、`email`、`id`、`name`、`password_hash`、`role`、`ssn` |
| 只含两个字段的 DTO | `id`、`name` |

**"前端没有显示它"不等于"它没有被发送"。** 页面渲染的是 `name`；响应体里带了一个口令哈希、一个 API key 和一个身份证号，而任何打开开发者工具网络面板的人 —— 不需要任何工具 —— 都能读到那三个。这是信息泄漏那篇的一个 API 形状的成因：查询是 `SELECT *`，而序列化器是整个对象。

**而响应和请求一样是一个接口。** 允许清单的逻辑在两个方向上都适用，而这是这一篇里最反直觉的部分：那些会仔细校验输入的人，往往对输出不做任何过滤地序列化，理由是那些字段"反正属于用户" —— 除了那些不属于的。

### 检测与缓解

- **当请求体携带一个不在该端点文档化 schema 里的字段时告警。** 对一个被绑定的请求体来说，这恰好就是将被写入的那组字段，所以把它与 schema 比对，同时是检测与发现。
- **特别对请求体里与授权相邻的字段名告警。** `role`、`is_admin`、`permissions`、`balance`、`verified`、`owner_id` —— 从客户端到来时它们从来不是例行公事，而这份清单短到可以维护。
- **当一个权限或余额在没有对应管理动作的情况下变化时告警。** 这是结果层的检查，而无论哪一个端点、哪一个字段造成了它，它都成立。
- **当一个账号的失败尝试来自一片分散的地址时告警。** 那是"配额被键在客户端可控之物上"的指纹，而它在记录这些尝试的任何地方都看得见。
- **并且对"比端点契约更大或更宽的响应载荷"告警。** 一个携带文档从未提及的字段的响应值得看一眼；一个在发布之后字段数跳升的响应更值得。
- **缓解上，用允许清单来绑定。** 按操作一份显式清单 —— 一个 DTO、一个表单类、一个参数 schema —— 让可写面成为一行代码，而不是一个框架默认。这是这里唯一一项**按构造就完整**的控制。
- **在模型层加一道守卫作为第二层，但不要只靠它。** 把属性标成不可赋值能抓到明显的错误、漏掉嵌套的那些，这就是它排在白名单之后而不是替代它的原因。
- **把限流的键设在攻击者选不了的东西上，并规范化它。** 语义限制用账号或会话，流量限制用真实的对端地址，路径在成为键的一部分之前先规范化 —— 并且在可信代理没有设置它们时忽略转发头。
- **语义限制放在应用里，流量限制放在边缘。** 它们回答不同的问题，而各自对对方那一块是盲的。
- **并且像对待请求一样仔细地过滤响应。** 每个端点一份响应允许清单，或者一个绑定到 schema 的序列化器，这样给表加一列不会等于给 API 加一列。这一篇与其余各篇共享的那条通则：**输入、输出与配额，各自都需要一组"被允许的东西"，由应用声明，而不是默认敞开。**
