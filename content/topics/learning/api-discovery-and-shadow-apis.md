---
id: api-discovery-and-shadow-apis
title_en: API Discovery and Shadow APIs
title_zh: API 发现与影子 API
summary_en: An API's surface is defined by what the server answers, not by what the documentation lists, and the difference between the two is where the findings are. Measured by building an API with a published spec and endpoints the spec omits, then finding them from the outside using nothing but status codes, an options request and a version guess.
summary_zh: 一个 API 的真实面由服务器实际回答的东西决定，而不是由文档列出的东西决定，而两者之间的差集就是要找的东西。这一篇搭一个"有规格、也有规格之外端点"的 API，然后只用状态码、一次 OPTIONS 请求和一个版本猜测，从外面把它们找出来。
tags: [beginner, api, discovery, openapi, shadow-api]
tools: [curl, ffuf, python3, jq]
attck: [T1595, T1087]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Two lists, and the interesting one is the difference

An API has a documented surface and an actual one. The first is what somebody wrote down; the second is the set of paths, methods and parameters the server responds to.

> **The documented list is an intention. The actual list is what a stranger can reach. The findings live in the difference.**

Nothing in this entry is about breaking anything. It is about **reading the difference**, and the measurements below are a small API built for the purpose, with a published specification and eight endpoints the specification does not mention — discovered from outside with status codes, an options request and a version guess.

### Part 1: what "the surface" is made of

| Dimension | What varies |
|---|---|
| **path** | `/api/v1/users`, `/admin`, `/internal/debug` |
| **method** | the same path may accept `GET` and `DELETE` |
| **version** | `/v1` and `/v2` may both be live, with different rules |
| **parameters** | documented ones, and ones the implementation reads anyway |
| **authentication** | required, optional, or required on one version only |
| **where it is served** | the gateway, the service, an old deployment, an internal host |

**And "where it is served" is the dimension that makes discovery hard rather than merely tedious.** The same application may be reachable through a gateway with authentication, directly on a service port without it, and through a legacy hostname that still resolves.

**Which is why a finding here is rarely "a path nobody knew about" and usually "a path with a different set of controls in front of it".**

### Part 2: finding the documentation

The first move is the cheapest: look for the specification where it is usually left. Measured against the API built for this entry:

| Path | Result |
|---|---|
| `/openapi.json` | **`200`** |
| `/swagger.json`, `/swagger-ui.html`, `/api-docs`, `/v2/api-docs`, `/docs`, `/redoc`, `/.well-known/openapi.json` | `404` |

**One of eight.** The remaining seven are the ones other frameworks and other conventions use, which is why the list is long and why a single guess proves nothing.

**And a specification, once found, is a map with a hole in it.** It lists what the author intended to expose — measured, three endpoints. **What it cannot list is what the implementation does beyond that**, because a specification is written by hand or generated at a point in time, and the running service is neither.

**So the specification is worth having for a different reason than it is usually fetched for**: not as an inventory, but as **one of the two lists whose difference is the finding**.

### Part 3: the difference, measured

Comparing the specification against what the service actually answered:

| | Count |
|---|---|
| endpoints in the specification | **3** |
| endpoints in the implementation | **11** |
| **in the implementation and not in the specification** | **8** |

The eight:

```
GET     /health
GET     /admin
GET     /actuator/env
GET     /api/v1/users
GET     /api/v1/admin/users
GET     /api/internal/metrics
GET     /api/internal/debug
DELETE  /api/v1/users/1
```

**Four sources produce these, and each one is a decision somebody made.**

**A feature was retired and the route was left in place**, because removing it might break a client that has not been heard from.

**A new version shipped and the old one was not switched off.** Measured, both `/api/v1/users` and `/api/v2/users` answer — and, as Part 5 shows, not with the same rules.

**An internal endpoint was written without going through the gateway**, because it was only going to be used by another service.

**And something was added for debugging and never removed**, which is the measured `/api/internal/debug` and `/actuator/env`.

**None of those is exotic, and all four produce the same result**: a path that exists, that the specification does not cover, and that the controls configured against the specification therefore do not cover either.

### Part 4: the status code is the signal

The measurement that makes discovery possible without guessing is that **"does not exist" and "exists but you may not" are different answers**, and they have to be, because a client needs to tell them apart.

Measured:

| Path | Status | Reading |
|---|---|---|
| `/api/v2/users` | **`403`** | **exists, refused** — it is in the spec |
| `/api/v1/users` | `200` | exists and readable — not in the spec |
| `/api/v1/admin/users` | `200` | exists and readable — not in the spec |
| `/api/internal/debug` | `200` | exists and readable — not in the spec |
| `/api/v9/nothing` | `404` | does not exist |

> **A `403` is a proof of existence. A `404` is the absence of one.** So an enumeration does not need to find a readable endpoint to learn that something is there — the refusal itself is the information.

**And that is why hiding an endpoint by returning `404` is a weaker control than it looks.** It removes the distinction for the caller, and the distinction is what the protocol needs; anything that later reveals the path — an error, a log, a cached response, a different verb — puts the caller back where they started. **The measurable difference between `403` and `404` is not a leak to be closed; it is the mechanism doing its job.**

**And the error body matters in the same way.** Measured, the `403` returned `{"error": "forbidden", "required": "X-API-Key"}` — **the refusal named what would have been accepted**, which turns a "you cannot" into a "here is what to bring".

### Part 5: methods, versions, and the rules that differ

**An options request returns a list nobody asked to publish.** Measured:

| `OPTIONS` on | `Allow` |
|---|---|
| `/api/v2/users` | `GET, POST, OPTIONS` |
| **`/api/v1/users/1`** | **`DELETE, OPTIONS`** |
| `/api/v2/orders` | `GET, OPTIONS` |
| `/api/v9/nothing` | `404`, no header |

**The second row is a path that is not in the specification at all, and the answer says a `DELETE` exists on it.** So one request to a path that was already known produced a method that was not.

**And versions are findable by guessing a number.** Measured:

| Path | Status |
|---|---|
| `/api/v1/users` | **`200`** |
| `/api/v2/users` | `403` — exists, needs the key |
| `/api/v3/users` | `404` |
| `/api/v4/users` | `404` |

**Two versions live, and the older one has no authentication on it.** Measured directly: without credentials, `/api/v1/users` answers `200` while `/api/v2/users` answers `403`.

> **An old version is not merely "still running". It usually runs the rules from when it was written** — and those rules are the ones that have since been tightened.

**Which is the general shape of version-related findings**: the current version is the one that gets reviewed, the one that gets the security fix, and the one the gateway rules are written against; **the older one keeps answering with the controls it shipped with**.

### Part 6: enumeration, and what makes it work

Measured, a small wordlist against four prefixes — 64 requests:

| | |
|---|---|
| requests sent | 64 |
| paths that answered as existing | **5** |
| hit rate | about **8%** |

**And the reason a small wordlist works at all is that API paths are designed to be guessable.** `users`, `orders`, `admin`, `health`, `metrics`, `internal`, `debug` are chosen because a developer reading the path should know what it does. **The naming convention that makes an API usable is the same one that makes it enumerable.**

**So the wordlist is less important than the prefixes.** The measured hits came from trying the right prefixes — `/api/`, `/api/v1/`, `/api/internal/`, `/` — against a handful of words. **Four prefixes times sixteen words found five paths; a thousand words against one wrong prefix would have found none.**

**Where the words come from matters too**: the specification's own vocabulary, the paths visible in a front-end bundle, the names in an error message, and the endpoints another deployment of the same product exposes. **Each of those turns a generic list into a targeted one.**

**And enumeration is visible.** The measured pattern is many requests that all return the same status and differ only in the path — which is a shape in a log that ordinary traffic does not have. **That is why the entries on rate limiting and on logging are the other half of this one**: the request pattern is detectable precisely because it is monotonous.

### Part 7: what a shadow endpoint is worth

The measured content of the three undocumented endpoints that answered `200`:

| Path | Body |
|---|---|
| `/api/internal/debug` | `{"db_url": "postgres://app@10.0.0.5/appdb", "debug": true}` |
| `/actuator/env` | `{"APP_SECRET": "should-not-be-here"}` |
| `/api/v1/admin/users` | `{"users": [...], "roles": {"alice": "admin"}}` |

**A connection string with a username, an application secret, and a role listing** — from three paths that the specification does not mention and, in a deployment where the controls are configured from that specification, that nothing is guarding.

**And that is the mechanism worth stating plainly:**

> **A shadow endpoint does not bypass authentication. It bypasses the meeting where somebody decided what authentication it should have.**

**The controls are usually attached to the inventory.** A gateway's authorisation rules, its rate limits, its logging and its redaction are configured per route — and a route that was never registered is a route those rules have no entry for. **So the endpoints that are hardest to find are also the ones with the fewest controls, which is not a coincidence**: both follow from never having been declared.

**And the same reasoning applies to the second deployment.** A service reachable directly on its port has the service's own controls and not the gateway's, which is why the same path can be protected through one door and open through another.

### Part 8: what follows for security

**The inventory has to be built from more than one source, and the differences are the work.** Four lists can be compared: the gateway's routes, the service's own routing, the specification, and the paths the service actually answers. **Measured, three of those four disagreed with each other**, and the two that matter for a finding are the last two.

**Nothing in this entry is an exploit, and that is the point.** Every measurement was a `GET`, an `OPTIONS` or a wrong path. **Discovery is a reading exercise**, and the reason it is worth its own entry is that the reading produces the list that every other review depends on — you cannot review the authentication of an endpoint you have not enumerated.

**`403` and `404` are a control and a signal, and they are not interchangeable.** The measured enumeration relied on being able to tell "refused" from "absent". **Turning the first into the second hides the distinction from an honest client as well as a dishonest one**, and leaves the endpoint unprotected rather than hidden.

**Old versions are the most productive place to look.** Measured, two versions of the same path answered, and the older one required nothing. **The controls go where the attention goes**, and attention follows the current release.

**And the fix is retirement rather than concealment.** An endpoint that is not needed should stop existing — because a route that is hidden is still a route, and every list the organisation maintains will keep disagreeing about it.

### Detection and mitigation

- **Reconcile four lists and treat the differences as findings.** The gateway's routes, the service's routing, the published specification, and the paths that answer. Measured, they disagreed by eight endpoints out of eleven.
- **Alert on requests to paths that are not in the registered inventory.** Measured, that is a small set of specific paths, and a request to one is either a mistake or a scan.
- **Watch for enumeration by its shape: many requests differing only in the path with the same status.** The request pattern is more detectable than any individual request.
- **Alert on `OPTIONS` requests in volume**, since they are a cheap way to ask what a path accepts and are rarely used by ordinary clients.
- **Track access to retired version prefixes, and set a date for them to stop answering.** Measured, an old version answered without authentication while the current one did not.
- **Do not replace a refusal with a not-found.** The distinction is needed by clients and is the signal that makes discovery possible, so hiding it costs a legitimate capability and removes no information.
- **For mitigation, generate the specification from the code rather than maintaining it alongside**, so that the documented list and the actual one cannot drift apart.
- **Register every route with the gateway, including the ones written for another service**, so that authorisation, rate limiting and logging apply by default rather than by remembering.
- **Remove debug and administrative endpoints from production images**, rather than protecting them with a rule, because the measured value of those endpoints was a connection string and a secret.
- **Keep internal services on a network boundary that requires more than knowing the path**, so that the second deployment does not answer without the controls of the first.
- **Add a check that fails a build when a route is added without appearing in the inventory.** The measured drift is the accumulation of small decisions, and a build-time check is the only place it is cheap to prevent.
- **And record the versions in a deployment inventory**, since "which of these is still answering" is a question that has a definite answer and is rarely asked.

<!-- lang:zh -->
### 两份清单，而有趣的是它们的差集

一个 API 有一份被文档化的面，和一份实际的面。前者是有人写下来的；后者是服务器实际会应答的路径、方法和参数。

> **被文档化的那份清单是一个意图。实际的那份是一个陌生人能够到的东西。要找的东西在两者的差集里。**

这一篇里没有任何东西是关于"破坏"的。它讲的是**读那个差集**，而下面的实测是一个为此搭起来的小 API：它有一份公开的规格，以及八个规格里没提到的端点 —— 只用状态码、一次 OPTIONS 请求和一个版本猜测，从外面把它们找出来。

### 第一部分："面"由什么构成

| 维度 | 会变的东西 |
|---|---|
| **路径** | `/api/v1/users`、`/admin`、`/internal/debug` |
| **方法** | 同一个路径可能同时接受 `GET` 和 `DELETE` |
| **版本** | `/v1` 与 `/v2` 可能都活着，而规则不同 |
| **参数** | 有文档的，以及实现照样会读的 |
| **认证** | 必需、可选、或者只在一个版本上必需 |
| **它由谁提供** | 网关、服务本身、一个旧部署、一台内网主机 |

**而"它由谁提供"才是让发现变难、而不只是变烦的那个维度。** 同一个应用可能通过一个带认证的网关可达、直接在一个服务端口上不可达、以及通过一个仍然解析的旧域名可达。

**这就为什么这里的发现很少是"一条没人知道的路径"，而通常是"一条前面挂着一套不同控制的路径"。**

### 第二部分：先把文档找出来

第一步是最便宜的：去规格通常被留在的地方找它。对着这一篇搭出来的那个 API 实测：

| 路径 | 结果 |
|---|---|
| `/openapi.json` | **`200`** |
| `/swagger.json`、`/swagger-ui.html`、`/api-docs`、`/v2/api-docs`、`/docs`、`/redoc`、`/.well-known/openapi.json` | `404` |

**八个里的一个。** 剩下七个是别的框架、别的约定用的位置，这就是那份清单为什么长、以及为什么猜中一个不能证明什么。

**而一份找到了的规格，是一张带洞的地图。** 它列出的是作者打算暴露的东西 —— 实测三个端点。**它列不出的是实现在这之外还做了什么**，因为规格是手写的、或者是在某个时间点生成的，而正在运行的服务两样都不是。

**所以规格值得拿的理由，和它通常被拿的理由不同**：不是当一份清单，而是当**那两份清单里的一份，而它们的差集才是要找的东西**。

### 第三部分：那个差集，实测

把规格与服务实际应答的东西对比：

| | 数量 |
|---|---|
| 规格里声明的端点 | **3** |
| 实现里的端点 | **11** |
| **在实现里、不在规格里** | **8** |

那八个：

```
GET     /health
GET     /admin
GET     /actuator/env
GET     /api/v1/users
GET     /api/v1/admin/users
GET     /api/internal/metrics
GET     /api/internal/debug
DELETE  /api/v1/users/1
```

**四个来源产生它们，而每一个都是某人做过的决定。**

**一个功能下线了，而路由被留着**，因为删掉可能弄坏一个音信全无的客户端。

**一个新版本上线了，而旧版本没有被关掉。** 实测，`/api/v1/users` 与 `/api/v2/users` 都应答 —— 而正如第五部分所示，它们的规则不一样。

**一个内部端点写的时候没走网关**，因为它只打算给另一个服务用。

**还有一样是为了调试加上去、然后一直没删**，那就是实测到的 `/api/internal/debug` 与 `/actuator/env`。

**这四个里没有一个稀奇，而四个都产生同一个结果**：一条存在的路径，规格没覆盖它，而那些照着规格配的控制因此也不覆盖它。

### 第四部分：状态码就是信号

让发现可以不靠猜的那次实测是：**"不存在"与"存在但你不能用"是两个不同的答案**，而它们必须是不同的，因为客户端需要区分它们。

实测：

| 路径 | 状态 | 读法 |
|---|---|---|
| `/api/v2/users` | **`403`** | **存在、被拒** —— 它在规格里 |
| `/api/v1/users` | `200` | 存在且可读 —— 不在规格里 |
| `/api/v1/admin/users` | `200` | 存在且可读 —— 不在规格里 |
| `/api/internal/debug` | `200` | 存在且可读 —— 不在规格里 |
| `/api/v9/nothing` | `404` | 不存在 |

> **一个 `403` 是存在性的证明。一个 `404` 是"没有证明"。** 所以一次枚举不需要找到一个可读的端点才能知道那里有东西 —— 那次拒绝本身就是信息。

**这就是为什么"用返回 `404` 来隐藏一个端点"是一个比看起来更弱的控制。** 它对调用者去掉了那个区分，而那个区分正是协议需要的东西；任何之后泄露该路径的东西 —— 一个错误、一份日志、一个缓存的响应、另一个动词 —— 都把调用者放回原处。**`403` 与 `404` 之间那个可测量的差别不是一个要堵住的泄漏，而是那个机制在正常工作。**

**而错误体以同样的方式要紧。** 实测，那个 `403` 返回了 `{"error": "forbidden", "required": "X-API-Key"}` —— **那次拒绝说出了本来什么会被接受**，这就把一句"你不能"变成了一句"这是该带的东西"。

### 第五部分：方法、版本，以及不同的规则

**一次 options 请求会返回一份没人打算公开的清单。** 实测：

| `OPTIONS` 打在 | `Allow` |
|---|---|
| `/api/v2/users` | `GET, POST, OPTIONS` |
| **`/api/v1/users/1`** | **`DELETE, OPTIONS`** |
| `/api/v2/orders` | `GET, OPTIONS` |
| `/api/v9/nothing` | `404`，没有这个头 |

**第二行是一条根本不在规格里的路径，而那个回答说明它上面存在一个 `DELETE`。** 所以对一条本来就已知的路径发一个请求，得到了一个本来不知道的方法。

**而版本是可以通过猜一个数字找到的。** 实测：

| 路径 | 状态 |
|---|---|
| `/api/v1/users` | **`200`** |
| `/api/v2/users` | `403` —— 存在，需要那个 key |
| `/api/v3/users` | `404` |
| `/api/v4/users` | `404` |

**两个版本活着，而旧的那个上面没有认证。** 直接实测：不带凭据时 `/api/v1/users` 应答 `200`，而 `/api/v2/users` 应答 `403`。

> **一个旧版本不只是"还在跑"。它跑的是它被写下来那年的规则** —— 而那些规则正是后来被收紧的那些。

**这就是与版本相关的发现的一般形状**：当前版本是被评审的那个、被安全修复的那个、也是网关规则照它写的那个；**而旧的那个继续用它出厂时的那套控制应答**。

### 第六部分：枚举，以及它为什么有效

实测，一份小词表打在四个前缀上 —— 64 个请求：

| | |
|---|---|
| 发出的请求 | 64 |
| 应答为"存在"的路径 | **5** |
| 命中率 | 约 **8%** |

**而一份小词表之所以有效，是因为 API 的路径本来就是为了可猜而设计的。** `users`、`orders`、`admin`、`health`、`metrics`、`internal`、`debug` 之所以被这么命名，是因为读这个路径的开发者应该知道它是干什么的。**让一个 API 好用的那套命名约定，就是让它可枚举的那一套。**

**所以词表本身没有前缀重要。** 实测那些命中来自试对了前缀 —— `/api/`、`/api/v1/`、`/api/internal/`、`/` —— 再配上一小把词。**四个前缀乘十六个词找到了五条路径；一千个词打在一个错的前缀上会一条都找不到。**

**词从哪里来也要紧**：规格自己的词汇、前端打包文件里可见的路径、一条错误信息里的名字、以及同一产品的另一个部署暴露的端点。**每一样都把一份通用清单变成一份有针对性的清单。**

**而枚举是可见的。** 实测到的那个形状是很多个请求全都返回同一个状态、只在路径上不同 —— 这是一个普通流量没有的形状。**这就是为什么限速与日志那两篇是这一篇的另一半**：请求模式之所以可检测，恰恰因为它单调。

### 第七部分：一个影子端点值多少

那三个应答了 `200` 的未文档化端点，内容实测如下：

| 路径 | 响应体 |
|---|---|
| `/api/internal/debug` | `{"db_url": "postgres://app@10.0.0.5/appdb", "debug": true}` |
| `/actuator/env` | `{"APP_SECRET": "should-not-be-here"}` |
| `/api/v1/admin/users` | `{"users": [...], "roles": {"alice": "admin"}}` |

**一条带用户名的连接串、一个应用秘密、一份角色清单** —— 来自三条规格没有提到的路径，而在一个"控制是照着那份规格配的"部署里，它们前面没有任何东西在守。

**而这就是那个值得说白的机制：**

> **一个影子端点绕过的不是认证。它绕过的是那次"某人决定了它该有什么认证"的会议。**

**控制通常是挂在清单上的。** 一个网关的授权规则、限速、日志与脱敏是按路由配的 —— 而一条从没被注册过的路由，就是那些规则里没有条目的路由。**所以最难被找到的那些端点，也是控制最少的那些，这不是巧合**：两者都来自"从来没有被声明过"。

**同样的推理适用于第二个部署。** 一个直接在其端口上可达的服务有它自己的控制、而没有网关的，这就是为什么同一条路径可以从一扇门被保护、从另一扇门敞开。

### 第八部分：从这些机制推出的安全观念

**清单必须从不止一个来源建立，而差集就是活儿。** 可以对比四份清单：网关的路由、服务自己的路由、那份规格、以及服务实际应答的路径。**实测，四份里有三份彼此不一致**，而对一条发现来说要紧的，是最后那两份。

**这一篇里没有一样是漏洞利用，而这就是重点。** 每一次实测都是一次 `GET`、一次 `OPTIONS`、或者一条错路径。**发现是一次阅读练习**，而它值得单独一篇，是因为这次阅读产出的清单是其他每一次评审所依赖的 —— 你无法评审一个你还没枚举出来的端点的认证。

**`403` 与 `404` 是一个控制和一个信号，而它们不可互换。** 实测那次枚举依赖的是能区分"被拒"与"缺席"。**把前者变成后者，会把那个区分同时对诚实的客户端和不诚实的客户端一起藏起来**，而留下的是一个没被保护的端点、而不是一个被藏起来的端点。

**旧版本是最有产出的地方。** 实测，同一条路径的两个版本都应答，而旧的那个什么都不要求。**控制会去注意力所在的地方**，而注意力跟着当前发布走。

**而修法是退役，不是隐藏。** 一个不需要的端点应该停止存在 —— 因为一条被藏起来的路由仍然是一条路由，而组织维护的每一份清单都会继续对它意见不一。

### 检测与缓解

- **把四份清单对起来，把差异当成发现。** 网关的路由、服务的路由、公开的规格、以及实际应答的路径。实测，它们在十一个端点里差了八个。
- **对打向"不在已注册清单里"的路径的请求告警。** 实测，那是一小批具体的路径，而打向其中一条的请求要么是一次失误、要么是一次扫描。
- **按形状盯枚举：很多请求只在路径上不同、而状态相同。** 请求模式比任何单个请求都更容易被发现。
- **对成批的 `OPTIONS` 请求告警**，因为它们是一种便宜地问"这条路径接受什么"的方式，而普通客户端很少用。
- **追踪对已退役版本前缀的访问，并给它们定一个停止应答的日期。** 实测，一个旧版本在不需要认证的情况下应答，而当前版本需要。
- **不要用"找不到"替换一次拒绝。** 那个区分是客户端需要的，也是让发现得以进行的信号，所以把它藏起来会损失一项正当能力、而不会移除任何信息。
- **缓解上，让规格从代码生成，而不是与代码并行维护**，这样被文档化的清单与实际的那份不会漂移开。
- **每一条路由都要向网关注册，包括那些为另一个服务写的**，这样授权、限速与日志就是默认生效、而不是靠记得。
- **把调试与管理端点从生产镜像里去掉**，而不是用一条规则去保护它们，因为实测那些端点的价值是一条连接串和一个秘密。
- **把内部服务放在一道"仅知道路径还不够"的网络边界之后**，这样第二个部署不会在没有第一个的控制的情况下应答。
- **加一条检查：一条路由被加进来而没出现在清单里时，让构建失败。** 实测那种漂移是许多小决定累积出来的，而构建期是唯一能便宜地拦住它的地方。
- **并且把版本记进部署清单**，因为"这些里还有哪个在应答"是一个有确定答案、却很少被问到的问题。
