---
id: graphql-fundamentals
title_en: "GraphQL — When the Client Decides the Shape"
title_zh: "GraphQL：当客户端决定了查询的形状"
summary_en: One endpoint where REST had many, and the client chooses the query — so the controls built for REST stop applying. Measured across query shapes against three rate-limiting strategies, and what field-level authorisation has to look like.
summary_zh: REST 有很多端点，GraphQL 只有一个，而查询由客户端决定 —— 于是为 REST 建的那套控制不再适用。这一篇把几种查询形状对着三种限流策略实测，并说明逐字段授权必须长成什么样。
tags: [web, graphql, authorization, rate-limiting, introspection, api]
tools: [curl, python3, graphql-cli]
attck: [T1190, T1499]
platform: [web, api]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### One endpoint, many operations

REST's controls assume a shape: a URL identifies one operation, one request does one thing, and the middleware in front can count requests and check a permission per endpoint. GraphQL replaces that with a single endpoint and a schema, where **the client writes the query it wants**.

The consequence is not that GraphQL is less secure by nature. It is that **the layer where several controls used to live stops being able to see what is happening**:

> **One endpoint means the middleware sees one request — and that request may contain a dozen different operations.**

Rate limiting, per-endpoint authorisation and per-endpoint input validation were all things a gateway or a route decorator could do. In GraphQL they have to be re-implemented **inside the resolver layer**, because that is the only place that knows what the query is asking for.

### Four layers

1. **The trust boundary.** The application trusts that one request means one operation, one authorisation decision and one unit of work. All three assumptions come from the REST shape and none of them hold.
2. **Data and instruction share a plane.** The query is data (a string from the client) and it is also **the program the server will execute** — a graph traversal the client authored, against a schema the server published.
3. **Why the usual fix fails.** A request-count limit at the gateway passes every shape measured below. Parent-object authorisation passes every field that hangs off it. Depth limits handle one shape and miss the other. Each control addresses a dimension, and the client gets to choose the dimensions.
4. **The variants.** Deep nesting; **aliases** that repeat the same field many times; fragments that multiply a shape; batched mutations; `node(id:)` interfaces that reach any object by identifier; and introspection.

### Measured: three rate-limiting strategies against five shapes

A small analyser, counting the three things that matter — nesting depth, field count and alias count:

| Query | Max depth | Fields | Aliases |
|---|---|---|---|
| Ordinary query | 2 | 4 | 0 |
| One level deeper | 3 | 6 | 0 |
| **Deeply nested** | **9** | 9 | 0 |
| **Aliased batch (one field, twelve aliases)** | **2** | **48** | **12** |
| Fragment multiplication | 2 | 11 | 0 |

And the same shapes against three policies:

| Query | By request count | By depth ≤ 4 | By cost and aliases |
|---|---|---|---|
| Ordinary | allow | allow | allow |
| One level deeper | allow | allow | allow |
| **Deeply nested** | **allow** | **deny** | **allow** |
| **Aliased batch** | **allow** | **allow** | **deny** |
| Fragment multiplication | allow | allow | allow |

**Read as a whole, that table is the entry's argument:**

**A request-count limit — the REST-shaped rule — allows everything**, while the work the server does differs by orders of magnitude. Twelve aliased lookups of the same field is a single request and twelve database reads.

**A depth limit catches deep nesting and misses aliases.** The aliased batch has a maximum depth of **2**, because all twelve copies sit at the top level side by side rather than inside one another. Depth is one dimension; a shape can be shallow and expensive.

**A cost limit catches aliases and misses depth**, in the measured policy, because the cost function only counted fields and aliases. That is not an argument against cost limits — it is the demonstration that **a cost function has to include every dimension the client can inflate**, and depth is one of them.

**And aliases are the specific mechanism worth knowing**: the same field, requested under different names, in one query. Per-field limits do not see it, and a resolver that caches by field name does not either.

### Measured: authorisation has to move down to the field

The second place the REST assumption fails. A permission check on the top-level object does not cover what hangs off it, and a schema is a graph the client can traverse:

| Path within one query | What it reaches |
|---|---|
| `me → orders → items → product` | From your own object to somebody else's product |
| `user(id: other) → email` | A top-level argument naming another user directly |
| `node(id: base64) { ... on PrivateThing }` | Any object, by identifier, through the node interface |
| `order(id) { customer { email } }` | A parent you may read, and an association you may not |

**The rule is the BOLA entry's, applied one level lower**: every resolver that returns an object should ask whether **this caller** may see **that object** — and in GraphQL that means every field, on every type, not only the entry points into the graph.

The awkward part is that the graph is symmetric. `me.orders` and `order(id).customer` may reach the same records from two directions, and an authorisation rule attached to the query root protects the first path and not the second. **Checking at the root is checking where the query started, not what it touches.**

### Introspection: not a vulnerability, a decision

Introspection lets a client ask the schema about itself — every type, field, argument and type signature. In one query:

```
query { __schema { types { name fields { name type { name } } } } }
```

**This is a feature, and the question is whether to leave it on in production.** It reduces "understanding this API" from hours of guessing to one request, which is genuine reconnaissance value.

Two things balance that:

**Disabling it is not a security boundary.** Error messages, field-name guessing, and the client application's own code all reveal the schema's shape. A deployment that turns introspection off and treats the API as hidden has removed a convenience for the attacker and no capability.

**And a schema is a specification, not a secret.** The reason to consider disabling it is the same reason to consider rate-limiting the endpoint: to make enumeration cost something. Where persisted queries are used, it becomes moot — see below.

### Detection and mitigation

- **Alert on query depth, field count and alias count as distributions, not as pass/fail.** The aliased batch shape is visible as a request whose field count is an order of magnitude above normal and whose alias count is not zero; deep nesting is visible in the depth distribution's tail.
- **Alert on high-cost queries and correlate them with response time.** The DoS entry's rule applies with a new input: here the attacker chooses the query's **shape**, so a few requests can produce the load that a flood would.
- **Alert on introspection from an unauthenticated client, and on `node(id:)` access patterns that walk identifiers.** Both are reconnaissance shapes rather than attacks, and both are cheap to see.
- **And alert when one request produces authorisation refusals on several different objects.** In REST that pattern is spread across endpoints; in GraphQL it arrives as one query that tried twelve paths.
- **For mitigation, compute a cost over the parsed query and cap it.** Assign a cost per field, multiply for list fields, add cost per alias, include depth, and cap the total. **Because the client chooses the shape, the limit has to be computed after the query is parsed** — a gateway counting requests cannot do it.
- **Set a depth limit as well, and treat it as one dimension among several.** The measured table shows a depth limit doing real work and missing the other shape; both belong.
- **Authorise per field, in the resolver.** The BOLA rule at every node of the graph, including associations and interface lookups. Where a rule is genuinely query-wide — "this tenant only" — implement it as a filter the data layer applies, so a resolver that forgets cannot bypass it.
- **Cap list fields and paginate by default.** A list field with no page size is the cheapest way to turn one query into an unbounded read.
- **Consider persisted queries.** Registering the queries the client may run, and having the client send an identifier rather than a document, restores the property REST had: **the set of possible operations is fixed at build time**, so a cost limit becomes easier, introspection becomes irrelevant, and an attacker's freedom to choose a shape disappears. This is the strongest single mitigation available here, and it is an architectural choice rather than a setting.
- **And keep the closing rule in mind, because it explains every section above.** GraphQL moves three decisions that used to be made in front of the application — **how much work one request may cause, who may read which object, and what shape of input is acceptable** — into the resolver layer. They have to be implemented there rather than inherited, and every measured failure in this entry is one of those three arriving at a place where nobody had re-implemented it.

<!-- lang:zh -->
### 一个端点，很多操作

REST 的控制建立在一个形状上：一个 URL 标识一个操作，一次请求做一件事，而前面的中间件可以数请求、并按端点检查权限。GraphQL 用一个端点和一份 schema 取代了它，其中**查询由客户端写**。

后果不是 GraphQL 天生更不安全。后果是**原本承载着好几项控制的那一层，看不见发生了什么**：

> **单个端点意味着中间件只能看到一次请求 —— 而那次请求里可能有十几个不同的操作。**

限流、逐端点授权、逐端点输入校验，都是网关或路由装饰器能做的事。在 GraphQL 里它们必须在**解析器那一层**重新实现，因为那是唯一知道"这个查询在要什么"的地方。

### 四层

1. **信任边界。** 应用信任"一次请求 = 一个操作 = 一次授权判断 = 一份工作量"。这三个假设全都来自 REST 的形状，而它们一条都不成立。
2. **数据与指令共用同一平面。** 查询是数据（来自客户端的一个字符串），同时它也是**服务端将要执行的程序** —— 一段由客户端编写、针对服务端公布的那份 schema 的图遍历。
3. **为什么常见修法失败。** 网关上的按请求数限流放行了下面实测的每一种形状。父对象上的授权放行了挂在它下面的每一个字段。深度限制处理一种形状、漏掉另一种。每项控制针对一个维度，而维度由客户端挑。
4. **变体。** 深层嵌套；把同一个字段重复很多次的**别名**；放大形状的分片；批量 mutation；按标识符够到任意对象的 `node(id:)` 接口；以及内省。

### 实测：三种限流策略对着五种形状

一个小分析器，数那三件要紧的事 —— 嵌套深度、字段数、别名数：

| 查询 | 最大深度 | 字段数 | 别名数 |
|---|---|---|---|
| 普通查询 | 2 | 4 | 0 |
| 深一层 | 3 | 6 | 0 |
| **深层嵌套** | **9** | 9 | 0 |
| **别名批量（一个字段，十二个别名）** | **2** | **48** | **12** |
| 分片放大 | 2 | 11 | 0 |

同一批形状对着三种策略：

| 查询 | 按请求数 | 按深度 <= 4 | 按成本与别名数 |
|---|---|---|---|
| 普通查询 | 放行 | 放行 | 放行 |
| 深一层 | 放行 | 放行 | 放行 |
| **深层嵌套** | **放行** | **拒绝** | **放行** |
| **别名批量** | **放行** | **放行** | **拒绝** |
| 分片放大 | 放行 | 放行 | 放行 |

**整张表合起来读，就是这一篇的论证：**

**按请求数限流 —— 那个 REST 形状的规则 —— 放行了一切**，而服务端做的工作量差着几个数量级。十二个别名查询是**一次请求**，也是**十二次数据库读取**。

**深度限制抓住了深层嵌套、漏掉了别名。** 别名批量的最大深度是 **2**，因为那十二份是并排放在顶层的，不是互相嵌套的。深度是一个维度；一个形状可以很浅却很贵。

**成本限制抓住了别名、漏掉了深度**，在那条实测的策略里，因为那个成本函数只数了字段与别名。这不是反对成本限制 —— 它演示的是**一个成本函数必须包含客户端能吹大的每一个维度**，而深度就是其中之一。

**而别名是那个特别值得知道的机制**：同一个字段，在同一个查询里用不同的名字要很多次。按字段的限流看不见它，按字段名做缓存的解析器也看不见。

### 实测：授权必须下沉到字段

REST 假设失效的第二个地方。对顶层对象的一次权限检查，覆盖不到挂在它下面的东西，而一份 schema 是客户端能遍历的一张图：

| 一次查询里的路径 | 它够到了什么 |
|---|---|
| `me → orders → items → product` | 从你自己的对象走到别人的商品 |
| `user(id: 别人) → email` | 一个直接指名别人的顶层参数 |
| `node(id: base64) { ... on PrivateThing }` | 通过 node 接口按标识符取任意对象 |
| `order(id) { customer { email } }` | 一个你能读的父对象，和一个你不能读的关联对象 |

**规则是 BOLA 那篇的，只是低了一层**：每一个返回对象的解析器都该问一句"**这个调用者**能不能看到**那个对象**" —— 而在 GraphQL 里，那意味着每一个类型上的每一个字段，而不只是进入这张图的那些入口。

别扭的地方在于这张图是对称的。`me.orders` 与 `order(id).customer` 可能从两个方向够到同一批记录，而挂查询根节点上的授权规则保护了第一条路、没保护第二条。**在根上检查，检查的是查询从哪里开始，而不是它碰到了什么。**

### 内省：不是一个漏洞，是一个决定

内省让客户端向 schema 问关于它自己的事 —— 每一个类型、字段、参数与类型签名。一个查询就够：

```
query { __schema { types { name fields { name type { name } } } } }
```

**这是一个特性，问题是要不要在产环境上留着它。** 它把"读懂这个 API"从几小时的猜变成一个请求，那是真实的侦察价值。

有两件事与之相抵：

**关掉它不构成一道安全边界。** 错误消息、字段名猜测、以及客户端应用自己的代码，全都会暴露 schema 的形状。一个关掉内省、并把"这个 API 是隐藏的"当成防线的部署，只是拿掉了攻击者的一个方便，没有拿掉任何能力。

**而 schema 是一份规范，不是一个秘密。** 考虑关掉它的理由，与考虑给这个端点限流的理由是同一个：让枚举付出代价。在使用持久化查询的地方，这件事就无关紧要了 —— 见下。

### 检测与缓解

- **把查询深度、字段数、别名数当分布看，而不是当通过/不通过看。** 别名批量那个形状，表现为一个字段数比正常高一个数量级、且别名数不为零的请求；深层嵌套在深度分布的长尾里。
- **对高成本查询告警，并把它们与响应时间关联起来。** DoS 那篇的规则在这里换了个输入：这里攻击者挑的是查询的**形状**，于是几个请求就能产出一次洪水才能产出的负载。
- **对未认证客户端的内省、以及 `node(id:)` 那种沿标识符走的访问模式告警。** 两者都是侦察形状而不是攻击，而两者都很便宜就能看到。
- **并且当一个请求在好几个不同对象上产生授权拒绝时告警。** 在 REST 里那个模式分散在各个端点上；在 GraphQL 里，它作为"一个试了十二条路"的查询到达。
- **缓解上，对解析后的查询算一个成本并设上限。** 给每个字段一个成本、列表字段乘一个倍数、别名按次加成本、把深度也算进去，然后给总成本设上限。**因为形状由客户端挑，这个上限必须在查询被解析之后计算** —— 一个数请求的网关做不到。
- **同时也设一个深度上限，并把它当成若干维度之一。** 实测那张表显示了深度限制在实打实地起作用，也显示了它漏掉另一种形状；两者都要有。
- **按字段授权，写在解析器里。** BOLA 的规则用在图的每一个节点上，包括关联与接口查询。当一条规则确实是查询级的 —— "只限这个租户" —— 把它实现成数据层施加的过滤条件，这样一个忘了写的解析器也无法绕过。
- **给列表字段设上限，并默认分页。** 一个没有页大小的列表字段，是把一次查询变成一次无界读取最便宜的方式。
- **考虑持久化查询。** 把客户端可以运行的查询注册下来、让客户端发一个标识符而不是一整份文档，就恢复了 REST 曾经有的那个性质：**可能操作的集合在构建期就固定了** —— 于是成本上限更容易算、内省变得无关紧要、而攻击者选择形状的自由消失了。这是这里最强的一项缓解，而它是一个架构选择，不是一个设置。
- **并且记住那句收束的话，因为它解释了上面每一节。** GraphQL 把三个原本在应用前面做的决定 —— **一个请求可以造成多少工作、谁能读哪个对象、什么形状的输入是可接受的** —— 搬进了解析器那一层。它们必须在那一层被实现，而不是被继承；而这一篇里每一个实测到的失效，都是这三者之一到达了一个"没有人重新实现它"的地方。

