---
id: api-gateway-and-rate-limiting
title_en: API Gateways and Rate Limiting
title_zh: API 网关与限速
summary_en: The gateway is the one place every request passes through, which is its value and the source of every way it fails — because anything that changes what "passes through" means is a bypass. Measured with a real gateway in front of a real backend — a window that lets twice its limit through, a key the caller chooses, five spellings of one path that walk past a rule, and an identity header the caller can set.
summary_zh: 网关是所有请求都要经过的唯一一处，这既是它的价值，也是它每一种失败方式的来源 —— 因为任何改变"经过"这个词含义的东西，都是一次绕过。这一篇用一个真网关挡在一个真后端前面实测：一个会放进两倍上限的窗口、一个由调用者选择的限速键、一条路径的五种写法走过规则，以及一个调用者能自己设置的身份头。
tags: [beginner, api, gateway, rate-limiting, waf]
tools: [nginx, envoy, python3, curl]
attck: [T1499, T1190]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The one place everything passes through

Every other component in an API has a specific job. A gateway has a position: it sits in front, so every request goes through it.

**That position is the entire value and the entire problem.**

> **Anything that changes what "through it" means is a bypass** — and there are more ways to change that than there are features in the gateway.

Measured below with a gateway written for the purpose in front of a backend also written for the purpose, so that both sides of each mismatch are visible.

### Part 1: what a gateway does, and what it cannot

| Job | Typical |
|---|---|
| terminate TLS | yes, and often the only place that does |
| route by host and path | the reason it exists |
| authenticate | verify a token before the request reaches anything |
| rate limit | per key, per route, per consumer |
| log and redact | one place to record what arrived |
| block known-bad patterns | a WAF, sometimes in the same product |

**And it does not know the business.** A gateway can decide that a request is allowed to reach `/api/v2/orders`; it cannot decide whether this particular caller may read this particular order. **That is the authorisation the service still has to do**, and the reason a gateway is a layer rather than a boundary.

**The mistake this entry is about is treating that layer as the boundary** — because everything measured below is a case where the layer's idea of the request and the service's idea of it are not the same request.

### Part 2: rate limiting, and the shape every approximation allows

A rate limit is a claim about a rate, enforced by counting something over some interval. **Every implementation of that idea allows some burst**, and the burst is what an attacker uses.

Measured, a fixed window of two seconds with a limit of five, counted per key:

| What was done | Requests | Allowed |
|---|---|---|
| five requests at the end of one window, five at the start of the next | **10** | **10** |

**Ten through a limit of five**, by sending them close together across a boundary the counter does not see. The window is not a rate; it is a count per interval.

> **A fixed window guarantees "no more than five per window". It does not guarantee "no more than five in any two seconds".** At the boundary those are different claims, and the difference is a factor of two.

**The alternatives have their own shapes.** A sliding window smooths the boundary and costs more state; a token bucket allows a deliberate burst up to the bucket size, which is often what you want for an interactive client and is a mistake for a login endpoint. **What none of them do is make the limit mean "per second"** — they make it mean "per bucket of some kind", and the shape of the bucket is the thing to know.

**And the limit has to be chosen against a purpose**, because the same number does different work:

| Purpose | The key that matters |
|---|---|
| stop password guessing | per **account**, not per IP |
| stop scraping | per **session or token** |
| protect capacity | per **route**, across everyone |
| stop one abusive client | per **client**, as identified by you |

**A single limit keyed on one of those does not cover the others**, and the measured mistake below is what happens when the key is chosen by the wrong party.

### Part 3: the key, which is where it usually goes wrong

The measured gateway keyed its limit on the `X-Forwarded-For` header:

| Sent | Requests | Allowed |
|---|---|---|
| a **different** value each time | 20 | **19** |
| **the same** value every time | 20 | **5** |

**Changing a header defeated the limit entirely.** And the mechanism is one sentence:

> **A rate limit limits the identity you chose to count. If that identity comes from the caller, you are limiting the identity the caller is willing to claim.**

**`X-Forwarded-For` is the specific trap**, because it is a real header that proxies do add and that clients can also add. The rule is about **which end of the list you trust**: a request arriving through one proxy you control has your client's address appended by that proxy, so the trustworthy entry is the one that proxy wrote — and a header with more entries than your topology allows is a header somebody else composed.

**And keying on the address has its own cost in the other direction**: a large office behind one address is one key, so a per-address limit is a limit on that organisation, and the first user to be abusive takes the rest with them. **Which is the same trade the entries on authentication make about lockouts** — a control aimed at the attacker lands on everyone sharing the identifier.

### Part 4: where the count lives

Measured, the gateway kept its counters in a dictionary in its own process, and clearing that dictionary restored the full allowance:

```
WINDOWS = {"10.0.0.1": (window start, count in this window)}
cleared -> the same key passed 5 more times
```

**So the effective limit is the configured limit multiplied by the number of instances**, which is not a bug in the implementation — it is what a local counter means.

**The three options and their costs:**

| Counters | Accurate | Cost |
|---|---|---|
| in each instance | no — multiplied by instance count | free, no dependency |
| in a shared store | yes | a dependency in the request path, and its own latency |
| in the client's token | yes, if the token is not forged | no store, and the count dies with the token |

**And the accuracy only matters for the limits that are protecting something specific.** A limit whose job is to stop a script is fine slightly loose; a limit whose job is to make password guessing impractical needs the count to be real, because the attacker can also add instances of their own — by sending requests from many addresses, which is the same multiplication from the other side.

### Part 5: the gateway and the backend disagree about the path

This is the measurement worth keeping. A gateway rule blocking any path beginning with `/api/admin`, in front of a backend that routes on its own normalised view of the same path:

| Path sent | Gateway | Backend routed it as |
|---|---|---|
| `/api/admin/users` | **`403` blocked** | — |
| `/api/ADMIN/users` | let through | **admin** |
| `/api//admin/users` | let through | **admin** |
| `/api/./admin/users` | let through | **admin** |
| `/api/x/../admin/users` | let through | **admin** |
| `/api/admin%2Fusers` | blocked | — |
| `/api/users/../admin/users` | let through | **admin** |

**Five of seven spellings of the same path reached the admin route through a rule that was meant to block it.**

**And the mechanism is not exotic, because this series has met it twice already**: the escaping difference between engines, and a built-in scanner whose idea of a string differed from the parser's. **The pattern is always the same:**

> **A filter looks at one version of the input, and the thing being protected looks at another. Every transformation between them is a place where the two can disagree.**

**The transformations are a known list**, and every one of them has been a real bypass somewhere: case folding, repeated separators, dot segments, percent-encoding — once, and again, and sometimes a third time — path parameters after a semicolon, trailing dots or spaces, Unicode normalisation, and length truncation in a fixed-size buffer.

**So the rule for a gateway authorisation rule is not "write a good pattern".** It is that **the rule and the router must be looking at the same string**, which in practice means normalising once, in one place, before either sees it — or matching on the normalised value that the backend will actually route on.

**And the check is cheap to run**: send each transformation of a path you know is blocked, and see whether the answer changes. **The measured table is exactly that test, written out.**

### Part 6: the identity header the caller can set

Gateways commonly add a header that tells the backend who the caller is, so the service does not have to verify the token itself. Measured, with the gateway written as `uid = the client's X-User-Id, or "anonymous" if absent`:

| Client sent | Backend saw |
|---|---|
| nothing | `anonymous` |
| **`X-User-Id: admin`** | **`admin`** |
| `X-User-Id: someone-else` | `someone-else` |

**The header was set by the gateway only when the client had not set it** — which leaves the claim to whoever asks first.

> **A header that is added only when it is missing is a header the caller decides.**

**The correct version is measured beside it**: an unconditional overwrite, where the client's value is discarded whatever it was —

```
client sends nothing         -> backend sees "the-gateway-decides"
client claims to be admin    -> backend sees "the-gateway-decides"
```

**And this class has a family**, all sharing the property that they are routing or identity hints a backend may trust: `X-Forwarded-For`, `X-Real-IP`, `X-Forwarded-Host`, and the ones that historically let a request be routed differently than it was checked, by naming an original or rewritten URL. **Any of them is a header the edge has to define rather than pass through**, and the backend has to be written to require the one its own edge produces.

### Part 7: the path that does not go through the gateway

Measured, comparing the same request through the gateway and directly to the backend:

| | Through the gateway | Direct to the backend |
|---|---|---|
| `GET /api/admin/users` | **`403`** | **`200`, admin route** |
| ten requests in a burst | **4 allowed** | **10 allowed** |

**Both of the gateway's controls were absent on the direct path** — the authorisation rule and the rate limit — and nothing about the backend changed. **So "we have a gateway" is a statement about the traffic that reaches it**, and the review question is which paths do not.

**And the routes it does have rules for are the ones somebody registered**, which is the same gap measured in the entry on shadow APIs: a route the gateway does not know about is a route its rules have no entry for. **The two entries are the same finding seen from two sides** — one from the inventory, one from the enforcement point.

### Part 8: what follows for security

**A gateway is depth, not a boundary.** Every measured failure here was a case where the gateway's view and the service's view differed — a different string, a different identity, or a different path to the same code. **So the service has to authorise its own requests regardless**, not because the gateway is unreliable but because the two are looking at different things by construction.

**Normalise once, and let both sides see the same value.** Measured, five spellings of one path went past a prefix rule because the rule matched raw text and the backend matched a normalised path. **Either normalise before the rule is applied, or write the rule against the value the router will use** — and then test it with the transformation table, because the list of transformations is finite and known.

**Define identity headers at the edge, unconditionally.** Measured, a caller could claim to be anyone. **The rule is that the edge overwrites, and the service requires what the edge produces** — and where the edge cannot be trusted to have run, the service verifies the token itself.

**Choose the rate limit key from the server's own knowledge.** Measured, a key taken from a client-controlled header let 19 of 20 requests through. **The key should be an account identifier, a session, or an address your own edge derived** — never a value the caller supplied, unless it has been validated against something the server knows.

**And know what the limit is for.** A limit on the wrong key protects nothing specific: per-address limits miss password guessing spread across addresses, and per-account limits do nothing about scraping. **The purpose decides the key, and the key decides whether the limit is real.**

**Counts in local memory multiply with instances**, which is acceptable for loose limits and not for tight ones. **And the attacker's equivalent of scaling out is a list of source addresses**, so the counting has to be on the dimension the attacker cannot multiply.

**And always look for the path that bypasses the edge.** Measured, the direct route had neither the rule nor the limit. **A gateway's controls are attached to the position, so the review question is not "is the gateway configured well" but "what reaches the service without passing it".**

### Detection and mitigation

- **Alert on `429` with a key that does not match any real client.** A spike of limits against a spread of distinct `X-Forwarded-For` values is somebody distributing their traffic, or a proxy chain you have not accounted for.
- **Compare the gateway's log with the service's log for the same request.** Measured, a gateway matched raw text while the backend routed on a normalised path; the two logs showing different paths for one request is the signal that they disagree.
- **Alert on requests whose path contains transformation markers**: `%2f`, `%252f`, `//`, `/./`, `/../`, a trailing dot or space, a semicolon. None of them is illegitimate by itself, and a distribution of them is a scan of exactly the measured table.
- **Alert on requests carrying identity headers from outside the edge** — `X-User-Id` and its relatives arriving at a service that should only ever see them from the gateway.
- **Watch for direct access to service ports that bypass the gateway**, and treat any traffic on them as a finding rather than as noise.
- **For mitigation, normalise the path once and match on that value**, then test the rule with each known transformation rather than with the canonical spelling only.
- **Overwrite identity and forwarding headers at the edge, unconditionally**, and have the service reject requests that lack the header its edge sets.
- **Derive rate-limit keys from server-side identity**, and pick the key from the purpose — account for guessing, session for scraping, route for capacity.
- **Use a shared counter for limits that protect an account**, and accept local ones only where being loose is acceptable.
- **Authorise in the service as well as at the edge.** Measured, the direct path had the rule removed entirely; the second check is what makes the first one depth rather than a single point.
- **Keep services on a network boundary that does not accept requests that have not passed the edge**, since every other control depends on that being true.
- **And test the edge with the bypasses rather than with the happy path.** Every measured failure in this entry was found by sending one path several ways, one header twice, and one request around the gateway — three tests, none of which needs a scanner.

<!-- lang:zh -->
### 所有东西都要经过的那一处

API 里每一个其他组件都有一个具体的职责。而网关有一个位置：它坐在最前面，所以每个请求都经过它。

**那个位置就是它的全部价值，也是它的全部问题。**

> **任何改变"经过它"这个词含义的东西，都是一次绕过** —— 而改变这个含义的方式，比网关的特性还多。

下面用一个为此写出来的网关挡在一个同样为此写出来的后端前面实测，这样每一处不一致的两边都看得见。

### 第一部分：网关做什么，以及它做不到什么

| 职责 | 典型情况 |
|---|---|
| 终止 TLS | 是，而且常常是唯一做这件事的地方 |
| 按主机与路径路由 | 它存在的理由 |
| 认证 | 在请求到达任何东西之前校验令牌 |
| 限速 | 按键、按路由、按消费者 |
| 记录与脱敏 | 一个记录"来的是什么"的地方 |
| 拦已知恶意模式 | 一个 WAF，有时是同一个产品里的 |

**而它不懂业务。** 网关能决定一个请求允许到达 `/api/v2/orders`；它无法决定这个特定的调用者能不能读这一个特定的订单。**那是服务仍然必须做的授权**，也是网关是一层、而不是一道边界的原因。

**这一篇要讲的错误，就是把那一层当成边界** —— 因为下面实测的每一件事，都是那一层对请求的理解与服务对它的理解不是同一个请求。

### 第二部分：限速，以及每一种近似都允许的形状

限速是一句关于速率的断言，通过在一段时间里数某个东西来执行。**那个想法的每一种实现都允许某种突发**，而突发正是攻击者要用的东西。

实测，两秒的固定窗口、上限五次、按键计数：

| 做了什么 | 请求数 | 通过 |
|---|---|---|
| 在一个窗口末尾发五次，在下一个窗口开头发五次 | **10** | **10** |

**十次通过了上限五次的限制**，靠的是把它们紧凑地发在一个计数器看不见的边界两侧。窗口不是一个速率，它是一个区间内的计数。

> **固定窗口保证的是"每个窗口内不超过五次"。它不保证"任意两秒内不超过五次"。** 在边界上，这是两个不同的断言，而差别是两倍。

**替代方案各有各的形状。** 滑动窗口把边界磨平、代价是更多状态；令牌桶允许一个刻意的、上限为桶大小的突发，那对一个交互式客户端常常正是你要的、而对一个登录端点是个错误。**它们没有一个让限制变成"每秒"** —— 它们让它变成"每个某种桶"，而桶的形状才是要知道的东西。

**而限制必须对照着一个目的来选**，因为同一个数字做的是不同的事：

| 目的 | 要紧的键 |
|---|---|
| 阻止口令猜测 | 按**账号**，不是按 IP |
| 阻止抓取 | 按**会话或令牌** |
| 保护容量 | 按**路由**，对所有人合计 |
| 阻止某一个滥用客户端 | 按**客户端**，由你来认定 |

**只按其中一种设一条限制，覆盖不了其他几种**，而下面实测到的错误，就是当那个键由错误的一方选择时会发生什么。

### 第三部分：键，通常就错在这里

实测那个网关把限制键取在 `X-Forwarded-For` 头上：

| 发出的 | 请求数 | 通过 |
|---|---|---|
| 每次一个**不同**的值 | 20 | **19** |
| **每次同一个**值 | 20 | **5** |

**换一个头就把限制完全打掉了。** 而机制是一句话：

> **限速限的是你选择去计数的那个身份。如果那个身份来自调用者，那你限的就是调用者愿意声称的那个身份。**

**`X-Forwarded-For` 是那个具体的陷阱**，因为它是一个代理确实会加、而客户端也能加的真实头部。规则在于**你信任那个列表的哪一端**：一个经过你自己控制的一个代理到达的请求，它的客户端地址是那个代理追加的，所以可信的那一项是那个代理写下的 —— 而一个条目数超过你的拓扑所允许的头，就是别人拼出来的头。

**而按地址计数在另一个方向上也有代价**：一个共享一个出口地址的大办公室就是一个键，所以按地址的限制是对那个组织的限制，第一个滥用的人会把其余的人一起带走。**这与认证那几篇里关于锁定的取舍是同一件事** —— 一个针对攻击者的控制，落在了所有共享那个标识的人身上。

### 第四部分：计数住在哪里

实测，那个网关把自己的计数器放在自己进程的一个字典里，而清空那个字典就恢复了全部额度：

```
WINDOWS = {"10.0.0.1": (窗口开始, 本窗口内的次数)}
清空之后 -> 同一个键又通过了 5 次
```

**所以实际生效的上限，是配置的上限乘以实例数**，而这不是实现里的 bug —— 这就是"本地计数器"的含义。

**三个选项与它们的代价：**

| 计数器在哪 | 准确 | 代价 |
|---|---|---|
| 每个实例自己的内存 | 不 —— 乘以实例数 | 免费，没有依赖 |
| 一个共享存储 | 是 | 请求路径上的一个依赖，以及它自己的延迟 |
| 客户端持有的令牌里 | 是，如果令牌不可伪造 | 不需要存储，而计数随令牌一起消失 |

**而准确性只对那些在保护某个具体东西的限制才要紧。** 一个职责是挡住脚本的限制稍微松一点没关系；一个职责是让口令猜测变得不划算的限制，需要那个计数是真的，因为**攻击者也能自己加实例** —— 从很多地址发请求，那就是同一件事从另一边做乘法。

### 第五部分：网关与后端对路径的理解不一致

这是最值得留着的那组实测。一个拦下所有以 `/api/admin` 开头路径的网关规则，挡在一个按自己对同一条路径的规范化结果路由的后端前面：

| 发出的路径 | 网关 | 后端把它路由成了 |
|---|---|---|
| `/api/admin/users` | **`403` 挡住** | —— |
| `/api/ADMIN/users` | 放过去 | **admin** |
| `/api//admin/users` | 放过去 | **admin** |
| `/api/./admin/users` | 放过去 | **admin** |
| `/api/x/../admin/users` | 放过去 | **admin** |
| `/api/admin%2Fusers` | 挡住 | —— |
| `/api/users/../admin/users` | 放过去 | **admin** |

**同一条路径的七种写法里，有五种穿过了一条本意是挡住它的规则，到达了 admin 路由。**

**而机制一点都不稀奇，因为这个系列已经遇到过它两次**：引擎之间那个转义差异，以及一个内建扫描器对字符串的理解与解析器不一致。**这个模式总是一样的：**

> **一个过滤器看的是输入的一个版本，被保护的东西看的是另一个版本。两者之间的每一次转换，都是它们可以产生分歧的地方。**

**那些转换是一份已知的清单**，而其中每一个都在某处成为过真实的绕过：大小写折叠、重复的分隔符、点段、百分号编码 —— 一次、两次、有时三次 —— 分号之后的路径参数、尾随的点或空格、Unicode 归一化、以及定长缓冲区里的长度截断。

**所以一条网关授权规则的写法不是"写一个好模式"。** 而是**规则与路由器必须看同一个字符串**，实践中这意味着在两者看到它之前、在一个地方规范化一次 —— 或者直接匹配后端实际会用来路由的那个规范化后的值。

**而这个检查跑起来很便宜**：把一条你已知被挡住的路径的每一种变换都发一遍，看答案会不会变。**上面那张实测表就是这个测试，写了出来。**

### 第六部分：调用者能自己设置的那个身份头

网关常常加一个头告诉后端调用者是谁，这样服务不必自己校验令牌。实测，网关的写法是 `uid = 客户端给的 X-User-Id，没有就 anonymous`：

| 客户端发的 | 后端见到的 |
|---|---|
| 什么都不发 | `anonymous` |
| **`X-User-Id: admin`** | **`admin`** |
| `X-User-Id: someone-else` | `someone-else` |

**那个头只在客户端没有设置时才被网关设置** —— 这就把声明权留给了先开口的那个人。

> **一个只在缺失时才被加上的头，是一个由调用者决定的头。**

**正确的写法就在旁边实测**：无条件覆盖，客户端给的值无论是什么都被丢掉 ——

```
客户端什么都不发      -> 后端见到 "the-gateway-decides"
客户端自称是 admin    -> 后端见到 "the-gateway-decides"
```

**而这一类有一个家族**，它们共享同一个性质：都是后端可能信任的路由或身份提示：`X-Forwarded-For`、`X-Real-IP`、`X-Forwarded-Host`，以及那些历史上通过命名一个"原始"或"重写后"的 URL、让一个请求被路由到与它被检查时不同的地方的头。**其中任何一个都是边缘必须自己定义、而不是透传的头**，而后端必须被写成要求它自己的边缘产出的那一个。

### 第七部分：那条不经过网关的路

实测，把同一个请求分别经网关和直连后端：

| | 经网关 | 直连后端 |
|---|---|---|
| `GET /api/admin/users` | **`403`** | **`200`，admin 路由** |
| 突发十个请求 | **通过 4 个** | **通过 10 个** |

**网关的两个控制在那条直连路上都不存在** —— 授权规则与限速 —— 而后端什么都没变。**所以"我们有网关"是一句关于到达它的那些流量的话**，而评审的问题是哪些路径不到达它。

**而它确实有规则的那些路由，是有人注册过的那些**，这与影子 API 那一篇里实测到的缺口是同一个：一条网关不知道的路由，就是它的规则里没有条目的路由。**这两篇是同一条发现的两个侧面** —— 一个从清单看，一个从执行点看。

### 第八部分：从这些机制推出的安全观念

**网关是纵深，不是边界。** 这里每一次实测到的失败，都是网关的视角与服务的视角不一致的情形 —— 不同的字符串、不同的身份、或者通往同一段代码的不同路径。**所以服务无论如何都要对自己的请求做授权**，不是因为网关不可靠，而是因为这两者按构造就在看不同的东西。

**规范化一次，让两边看到同一个值。** 实测，一条路径的五种写法走过了一条前缀规则，因为规则匹配的是原始文本、而后端匹配的是规范化后的路径。**要么在规则生效之前规范化，要么把规则写成对着路由器会用的那个值** —— 然后用那张变换表去测它，因为那份变换清单是有限且已知的。

**身份头在边缘无条件定义。** 实测，一个调用者可以声称自己是任何人。**规则是边缘覆盖，而服务要求边缘产出的那一个** —— 而在边缘可能没有运行的地方，服务得自己校验令牌。

**限速键要从服务端自己的知识里选。** 实测，一个取自客户端可控头的键让二十次里的十九次通过了。**那个键应该是一个账号标识、一个会话、或者你自己的边缘推导出来的地址** —— 永远不要是调用者提供的一个值，除非它已经对照着服务端知道的东西被校验过。

**而且要清楚这条限制是为了什么。** 一条键选错的限制什么具体的东西都保护不了：按地址的限制漏掉分散在多个地址上的口令猜测，而按账号的限制对抓取毫无作用。**目的决定键，而键决定这条限制是不是真的。**

**本地内存里的计数会随实例数翻倍**，这对宽松的限制可以接受、对紧的限制不行。**而攻击者那边等价于"扩容"的东西，是一串来源地址**，所以计数必须落在攻击者无法翻倍的那个维度上。

**并且永远要去找那条绕过边缘的路。** 实测，那条直连路上既没有规则也没有限速。**网关的控制是挂在那个位置上的，所以评审的问题不是"网关配得好不好"，而是"什么能在不经过它的情况下到达服务"。**

### 检测与缓解

- **对"键不匹配任何真实客户端"的 `429` 告警。** 针对一批不同 `X-Forwarded-For` 值的限制猛增，是有人在分散流量，或者是一条你没算进去的代理链。
- **把网关的日志与服务的日志对同一个请求比一比。** 实测，网关匹配的是原始文本、而后端按规范化后的路径路由；两个日志对同一个请求记下不同路径，就是它们不一致的信号。
- **对路径里带变换标记的请求告警**：`%2f`、`%252f`、`//`、`/./`、`/../`、尾随的点或空格、分号。它们本身没有一个是非法的，而它们的分布正是对上面那张实测表的扫描。
- **对来自边缘之外、携带身份头的请求告警** —— `X-User-Id` 及其同类出现在一个只该从网关看到它们的服务上。
- **盯绕过网关直连服务端口的访问**，并把那些端口上的任何流量当成一条发现，而不是噪声。
- **缓解上，把路径规范化一次并匹配那个值**，然后用每一种已知变换去测那条规则，而不是只测规范写法。
- **在边缘无条件覆盖身份与转发头**，并让服务拒绝缺少它自己边缘所设的那个头的请求。
- **限速键从服务端身份推导**，并按目的来选键 —— 猜测按账号、抓取按会话、容量按路由。
- **保护账号的那些限制要用共享计数器**，只有在"松一点可以接受"的地方才用本地的。
- **在服务里也做授权，不只是边缘。** 实测，那条直连路上规则完全不存在；第二道检查才是让第一道成为纵深、而不是一个单点的东西。
- **把服务放在一道"不接受未经边缘的请求"的网络边界里**，因为其他每一个控制都依赖这件事成立。
- **而且用绕过去测边缘，而不是用顺利路径去测。** 这一篇里每一次实测到的失败，都是靠把一条路径用几种写法发、把一个头发两次、以及把一个请求绕过网关发出来的 —— 三个测试，没有一个需要扫描器。
