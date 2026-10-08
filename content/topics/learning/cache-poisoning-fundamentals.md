---
id: cache-poisoning-fundamentals
title_en: "Web Cache Poisoning, Part 1 — The Key Is Narrower Than the Response"
title_zh: "Web 缓存投毒（一）：键比响应窄"
summary_en: A cache assumes that an identical key means a reusable response, and poisoning is what happens when an input changes the response without changing the key. Measured on a purpose-built cache, with the unkeyed inputs worth looking for and the reason one poisoned entry reaches every later visitor.
summary_zh: 缓存的前提是"键相同就意味着响应可以复用"，而投毒就是"某个输入改变了响应、却没有改变键"时发生的事。这一篇在一个专门搭出来的缓存上实测，列出值得找的未键输入，以及为什么一个被投毒的条目会影响之后每一个访问者。
tags: [web, cache-poisoning, cwe-444, http, caching, xss]
tools: [curl, python3, Burp Suite]
attck: [T1190, T1499]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### One assumption, and how to break it

A cache exists to answer a request without asking the application. To do that it needs one guarantee:

> **If the key is the same, the response may be reused.**

Everything a cache does follows from that. The key is typically built from the request method, the path and the query string, plus whatever the deployment was configured to include. **Poisoning is what happens when some input changes the response without changing the key** — the cache then stores one visitor's response under a key that other visitors use.

That is the whole class, and it is worth noting how little the cache has to do wrong. It applies its configured key faithfully. The application applies its configured behaviour faithfully. The gap is that **nobody wrote down the full set of inputs that affect the response**, and the cache has no way to discover it.

The question that finds it:

> **Which inputs change this response, and are all of them in the key?**

### Four layers

1. **The trust boundary.** The cache trusts the application's implicit promise that the response depends only on the key. That promise is never stated anywhere — it is an assumption the deployment inherits, and the cache cannot verify it.
2. **Data and instruction share a plane.** A request header is simultaneously **an input the application may reflect or act on** and **a detail the cache considers irrelevant to identity**. The same header plays both roles because nothing assigns it one.
3. **Why the usual fix fails.** A cache includes nothing in the key by default beyond the request line, and adding coverage requires the **application** to declare it with `Vary` — per response, per header, remembered every time. The failure mode is therefore not a bug but a **default**: unlisted inputs are unkeyed inputs, whether or not anyone thought about them.
4. **The variants.** Unkeyed headers, unkeyed query parameters, method-override headers, and — the most controllable — a smuggled request, which lets an attacker place an arbitrary request where the cache will see it (the subject of the previous two entries).

### Measured: poisoning, and the fix

Against a cache whose key is method plus path, with an application that reflects `X-Forwarded-Host` into the page's support link:

| Visitor | Request | Result |
|---|---|---|
| Attacker | `GET /` with `X-Forwarded-Host: evil.example` | **MISS (stored)** — the page contains `https://evil.example/support` |
| Victim | `GET /` — ordinary request | **HIT** — the same poisoned page |

The victim sent nothing unusual and received the attacker's page. **One request, and every subsequent visitor to that URL is served it** — which is what separates this class from reflected XSS, where the attacker has to deliver a crafted link to each target individually.

Now the same application with `Vary: X-Forwarded-Host` on the response:

| Visitor | Request | Result |
|---|---|---|
| Attacker | `GET /` with the header | **MISS (not stored, Vary=X-Forwarded-Host)** |
| Victim | `GET /` | **MISS** — the victim's own version, `https://shop.example/support` |

The fix is small and it works, because it addresses the cause: the response is now declared to depend on that header, so a response produced for one value of it is not reused for another.

### The unkeyed inputs worth looking for

Anything the application reads from the request but the cache does not key on is a candidate. In practice a short list covers most of it:

| Input | Why it is usually unkeyed |
|---|---|
| `X-Forwarded-Host`, `X-Forwarded-Scheme`, `X-Host` | Treated as proxy metadata; the application may use it to build absolute URLs |
| `X-Forwarded-For` | The application may log it, and some frameworks reflect the client address |
| `X-Original-URL`, `X-Rewrite-URL` | Routed on by some frameworks, unkeyed by most caches |
| `X-HTTP-Method-Override` | Changes the effective method without changing the key's method |
| Query parameters the cache ignores | Some caches drop parameters they consider irrelevant, or normalise the query string |
| Cookie values | Reflected content or A/B state, where the cache does not key on cookies |
| A smuggled request | Gives precise control over the request the cache processes |

The general test is one question per input: **does this value appear anywhere in the response?** If it does, and it is not in the key, the deployment is poisonable with it.

### Why the consequence is larger than it looks

**One request affects every subsequent visitor.** The measured HIT is what the next thousand visitors get. There is no per-victim delivery step, which makes this the cheapest way to achieve stored XSS across a user population.

**Caches serve in front of authentication.** A cached response is returned before the application sees the request, so a poisoned entry reaches users the attacker could not otherwise influence, including ones who never interact with the attacker.

**And the payload does not have to be script.** Poisoning a redirect sends every visitor to the attacker's site. Poisoning a page that builds a link — the measured case — leaks whatever is in the URL, including tokens when the link is a reset or confirmation link. Poisoning an error response is a denial of service against every visitor, achieved once.

**The Host header entry is the same bug one layer up.** There, the application generated a link from a header the client controls. Here, that response is also **stored** for other visitors, which turns a per-request confusion into a persistent one. The two are commonly chained, and the fix belongs in both places.

### Detection and mitigation

- **Alert when a cacheable response contains a value that came from a request header.** The measured case is exactly this: a page containing `https://evil.example/support`, where that host came from an unkeyed header. Comparing the stored response against the request that produced it is the direct test, and it needs no knowledge of which headers matter.
- **Log which headers and parameters each response actually depends on, and compare that against the cache key.** If a response reflects a header, that header is part of the key whether or not the cache thinks so. Producing that list is the review artefact this class needs, because the failure is always an omission.
- **Alert on unkeyed control headers arriving at the edge.** `X-Forwarded-Host`, `X-Forwarded-Scheme`, `X-Original-URL`, `X-HTTP-Method-Override` and their relatives arriving from a client are the reconnaissance step, and the edge can simply strip them before the cache ever sees the request.
- **Watch for responses that look identical across users.** A page whose content does not vary when it should — the same dashboard for every account, a personalised path returning generic content — indicates a cache serving one user's entry to another.
- **For mitigation, do not cache what should not be cached.** Personalised, authenticated and session-dependent responses belong behind `Cache-Control: private` or `no-store`, and this is the control that removes the class rather than narrowing it.
- **Where responses are cached, make them declare their inputs.** `Vary` is the mechanism, and the measured test shows it working — but it has to be applied to **every** input the response depends on, which makes it a maintenance obligation rather than a one-time setting.
- **Prefer an explicit allowlist of what may be cached over a denylist of what may not.** This is the same conclusion as the WAF and filter entries: a list of inputs someone thought of is defeated by the one they did not, while a cache that stores nothing until told otherwise fails closed.
- **Strip the control headers at the edge.** Whatever the application does, the value of `X-Forwarded-Host` should be set by the trusted proxy, not accepted from the client — the same rule as the Host header entry, applied before the cache rather than at the application.

<!-- lang:zh -->
### 一个前提，以及怎么打破它

缓存存在的意义是不问应用就回答一个请求。要做到这一点，它需要一个保证：

> **键相同，响应就可以复用。**

缓存所做的一切都由此而来。键通常由请求方法、路径、查询串，加上部署被配置成要包含的那些东西构成。**投毒就是"某个输入改变了响应、却没有改变键"** —— 于是缓存把一位访客的响应，存在了其他访客会用的键下面。

这就是整一类，而值得注意的是**缓存几乎没做错什么**。它忠实地套用它被配置的键，应用忠实地执行它被配置的行为。缝在于**没有人把"会影响这个响应的全部输入"写下来**，而缓存也没有办法自己发现。

找出它的问题：

> **有哪些输入会改变这个响应，它们全都进键了吗？**

### 四层

1. **信任边界。** 缓存信任应用那个隐含的承诺：响应只依赖键。那个承诺从没被写在任何地方 —— 它是这个部署继承来的一个假设，而缓存无法验证它。
2. **数据与指令共用同一平面。** 一个请求头同时是**应用可能回显或据以行动的输入**，又是**缓存认为与身份无关的细节**。同一个头扮演两个角色，因为没有东西给它分派其中一个。
3. **为什么常见修法失败。** 缓存的键除了请求行之外默认什么也不含，而扩大覆盖面要求**应用**用 `Vary` 去声明它 —— 逐响应、逐头、每次都要记得。所以失效模式不是一个 bug，而是一个**默认值**：没被列出的输入就是未键输入，无论有没有人想过它。
4. **变体。** 未键的头、未键的查询参数、方法覆盖头，以及最可控的那一个 —— 走私请求，它让攻击者把一个任意请求放在缓存会看到的位置（也就是前面两篇的主题）。

### 实测：投毒，以及修法

对着一个"键 = 方法 + 路径"的缓存，应用把 `X-Forwarded-Host` 回显进页面的支持链接：

| 访客 | 请求 | 结果 |
|---|---|---|
| 攻击者 | `GET /` 带 `X-Forwarded-Host: evil.example` | **MISS（已存储）** —— 页面里含 `https://evil.example/support` |
| 受害者 | `GET /` —— 普通请求 | **HIT** —— 同一个被投毒的页面 |

受害者什么都没做特殊的事，却收到了攻击者的页面。**一次请求，而这个 URL 之后每一位访客都会被这样服务** —— 这正是这一类与反射型 XSS 的区别：后者需要攻击者把构造好的链接逐个送达目标。

现在同一个应用，在响应上加了 `Vary: X-Forwarded-Host`：

| 访客 | 请求 | 结果 |
|---|---|---|
| 攻击者 | `GET /` 带那个头 | **MISS（未存储，Vary=X-Forwarded-Host）** |
| 受害者 | `GET /` | **MISS** —— 受害者自己的版本，`https://shop.example/support` |

修法很小，而且有效，因为它针对的是成因：响应现在被声明为依赖那个头，于是为它的某个值产生的响应，不会不被复用于另一个值。

### 值得找的那些未键输入

任何"应用会从请求里读、而缓存不键它"的东西都是候选。实践里一份短清单就覆盖了大部分：

| 输入 | 为什么通常未键 |
|---|---|
| `X-Forwarded-Host`、`X-Forwarded-Scheme`、`X-Host` | 被当作代理元数据；应用可能用它拼绝对 URL |
| `X-Forwarded-For` | 应用可能记它，而有些框架会回显客户端地址 |
| `X-Original-URL`、`X-Rewrite-URL` | 有些框架据它路由，而多数缓存不键它 |
| `X-HTTP-Method-Override` | 改变了生效的方法，却没有改变键里的方法 |
| 缓存忽略的查询参数 | 有些缓存丢掉它认为无关的参数，或者会规范化查询串 |
| Cookie 的值 | 被回显的内容或 A/B 状态，而缓存不键 Cookie |
| 一个走私请求 | 对"缓存所处理的请求"给出精确控制 |

通用的检验是每个输入问一个问题：**这个值在响应里任何地方出现吗？** 如果出现，而它不在键里，那么这个部署就能用它投毒。

### 为什么后果比看上去大

**一次请求影响之后每一位访客。** 实测里那个 HIT，就是接下来一千位访客会拿到的东西。它没有逐个送货的步骤，这让它成为跨用户群达成存储型 XSS 最便宜的方式。

**缓存服务在认证前面。** 被缓存的响应在应用看到请求之前就被返回了，所以一个被投毒的条目能到达攻击者本来影响不到的用户 —— 包括那些从不与攻击者交互的人。

**而且 payload 不必是脚本。** 投毒一个重定向，会把每位访客送到攻击者的站点。投毒一个会拼链接的页面 —— 也就是实测的那种 —— 会泄漏 URL 里的一切，而当那条链接是重置或确认链接时，里面就有 token。投毒一个错误响应，就是针对每位访客的拒绝服务，而且只需达成一次。

**Host 头那篇是同一个 bug 高一层的位置。** 那里，应用用客户端可控的请求头生成了链接。而在这里，那个响应还被**存储**下来给其他访客 —— 于是"一次请求的混淆"变成了"持续的混淆"。两者经常被串起来用，而修法两处都需要。

### 检测与缓解

- **当一个可缓存的响应里含来自请求头的值时告警。** 实测那种正是如此：一个页面里含 `https://evil.example/support`，而那个 host 来自一个未键的头。把存下来的响应与产生它的请求对比，就是直接的检验，而且它不需要知道哪些头要紧。
- **记录每个响应实际依赖哪些头与参数，再和缓存键比。** 如果一个响应回显了某个头，那个头就是键的一部分 —— 无论缓存怎么想。把这份清单产出来，正是这一类需要的评审产物，因为它的失效永远是一次遗漏。
- **对到达边缘的未键控制头告警。** `X-Forwarded-Host`、`X-Forwarded-Scheme`、`X-Original-URL`、`X-HTTP-Method-Override` 及其亲属从客户端到来，就是侦察那一步 —— 而边缘完全可以在缓存看到这个请求之前就把它们剥掉。
- **盯那些"在不同用户之间看起来一样"的响应。** 一个本该变化却不变化的内容 —— 每个账户拿到同一个看板、一个个性化路径返回了通用内容 —— 说明缓存把一个用户的条目服务给了另一个。
- **缓解上，不该缓存的东西就不要缓存。** 个性化、已认证、依赖会话的响应应当被 `Cache-Control: private` 或 `no-store` 挡住，而这正是"移除这一类"而不是"缩小它"的那项控制。
- **在响应会被缓存的地方，让它们声明自己的输入。** `Vary` 就是机制，而实测里它确实有效 —— 但它必须覆盖响应依赖的**每一个**输入，这使它成为一项维护义务，而不是一次性设置。
- **优先"明确允许缓存什么"的允许清单，而不是"不许缓存什么"的黑名单。** 这和 WAF、过滤器那两篇是同一个结论：一份有人想到的输入清单，会被他们没想到的那一个击败；而一个"没被明确告知就什么都不存"的缓存会失败关闭。
- **在边缘剥掉那些控制头。** 无论应用怎么做，`X-Forwarded-Host` 的值都应当由可信代理设置、而不是从客户端接受 —— 这与 Host 头那篇是同一条规则，只不过施加在缓存之前而不是应用那里。
