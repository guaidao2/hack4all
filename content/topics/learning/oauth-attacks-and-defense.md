---
id: oauth-attacks-and-defense
title_en: "OAuth 2.0 and OIDC, Part 2 — Layers, Outbound Requests and the Chain to Takeover"
title_zh: "OAuth 2.0 与 OIDC（二）：层、出站请求，以及通向接管的链条"
summary_en: An exact match still leaves the question of which layer does the comparing — measured with a validator that decodes once and a server that decodes again. This entry covers that, the URLs an authorization server fetches on a client's behalf, scope elevation, and how small findings chain into takeover.
summary_zh: 精确匹配之后还剩一个问题：到底是哪一层在做比较 —— 实测一个"解一次"的校验器与一个"再解一次"的服务器。这一篇讲这件事、授权服务器替客户端去取的那些 URL、scope 提升，以及几个小发现怎么串成账号接管。
tags: [web, oauth, oidc, cwe-601, cwe-918, authentication]
tools: [curl, python3, Burp Suite]
attck: [T1550, T1528, T1078]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### Exact matching is necessary, and it is not sufficient

The previous entry measured `redirect_uri` validation and concluded that only exact comparison refused every bypass. That conclusion is correct and it has a precondition worth stating: **exact comparison against what?**

A URL is not one value. It is decoded by whoever receives it, normalised by whoever routes it, and compared by whoever decides whether it is allowed — and if those are three different layers with three different rules, "exact" describes the comparison rather than the outcome.

### The validator and the router

Measured, with an allow-check that decodes a path once before comparing it to the registered value, and a server that decodes again before routing:

| Case | Decision | Path the validator sees | Path the server arrives at |
|---|---|---|---|
| The registered value | allow | `/callback` | `/callback` |
| Plain traversal | deny | `/evil` | `/evil` |
| Single-encoded dots | deny | `/evil` | `/evil` |
| **Double-encoded dots** | **allow** | **`/callback/%2e%2e/%2e%2e/evil`** | **`/evil`** |
| Double-encoded slashes | deny | `/callback%2f..%2f..%2fevil` | `/evil` |
| Triple-encoded dots | allow | `/callback/%252e%252e/evil` | `/callback/%2e%2e/evil` |

The fourth row is the finding. To the validator, `/callback/%2e%2e/%2e%2e/evil` is **inside the registered path** — the first decode turned `%252e` into `%2e`, which is not a dot yet, so normalisation leaves it alone and the string still begins with `/callback`. To the server, one more decode turns `%2e%2e` into `..`, and the path resolves to `/evil`. **The same string is safe to one layer and elsewhere to the other**, and both are doing their job correctly.

**Each layer that decodes adds one level.** Measured on a triple-encoded payload:

```
0 decodes -> /callback/%25252e%25252e/evil
1 decode  -> /callback/%252e%252e/evil
2 decodes -> /callback/%2e%2e/evil
3 decodes -> /callback/../evil
```

So the question is never "is this encoded twice" — it is **how many times does each layer decode, and where does that count differ.** That is the fourth layer of the vulnerability-essence standard this guide is built around: **the layer that validates and the layer that decodes are not the same layer**, in its most literal form.

### The same disagreement, under other names

This is not an OAuth problem, and seeing it once makes it recognisable everywhere:

| Layer A | Layer B |
|---|---|
| The `redirect_uri` validator | The browser or client that follows the URL |
| A WAF's normalisation | The framework's decoding at the origin |
| The cache's key computation | The application's routing |
| The reverse proxy's path normalisation | The back-end's path parsing |
| The allowlist check in the SSRF entry | The resolver and HTTP client that connect |

**Every pair that disagrees by one rule leaves a string that one side considers new and the other does not** — and that string is the exploit. The fix is not more validation; it is **one layer being the only standard**, with everything downstream reading what it produced.

### Where the authorization server makes outbound requests

OAuth has a second family of findings that is not about redirects at all: the authorization server, and the client registration process, **fetch URLs supplied by someone else**.

| Field | Supplied by | When it is fetched |
|---|---|---|
| `jwks_uri` | The party registering the client | When validating that client's tokens |
| `request_uri` | Whoever initiates the authorization request | When handling that request |
| `logo_uri`, `policy_uri`, `tos_uri` | The party registering the client | When displaying the consent screen |
| `software_statement` (dynamic registration) | Anyone who can register | At registration |

Each of these is the SSRF entry's pattern in a place where the consequence is larger than usual. **The authorization server is typically an internal system with high privilege**: it holds signing keys, it can act as any client, it can read other clients' registrations, and it frequently sits where it can reach services no user can.

- **`request_uri`** is the strongest of these, because it is set in an **authorization request** rather than at registration — meaning it needs no client credentials, and the fetched content is interpreted as a request object.
- **`jwks_uri`** is slower but more valuable: a client that can point it at an internal endpoint may be able to make the server read something and, depending on the validation path, observe the result.
- **Dynamic client registration**, where it is open, lets anyone supply all of the above.

The rule is the same as the SSRF entry's: **a value that causes the server to make a request is a value that needs an allowlist, not a filter.**

### Scope elevation, and the implicit flow

**Scope elevation** does not break any check. The client requests more than the user expects, the consent screen lists it in language few users read, and the authorization server issues it — **the consent screen is the only control**, which is why it has to be specific rather than a list of permission names.

**The implicit flow** returns tokens in the URL **fragment**, which the browser does not send to a server but does expose to scripts on the page and to anything that ends up in the address bar — browser history, a screenshot, a referrer if the fragment is copied into a link. It has been superseded by the authorization-code flow with PKCE for public clients, and where it is still in use it is worth treating as a finding on its own.

**And the `nonce`** is the identity layer's equivalent of `state`: it binds an `id_token` to the request that asked for it, which is what makes a replayed or pre-generated `id_token` fail.

### How the small findings chain

None of the entries above is account takeover by itself, and the reason this class matters is how they combine:

```
a loose redirect_uri match   ->  the code is delivered to the attacker
+ no PKCE                    ->  the code is exchangeable by the attacker
+ open redirect on the client ->  a second hop that reaches an allowed destination
+ state not checked          ->  a code from elsewhere lands in the victim's session
```

Each line on its own is a finding of moderate severity and is frequently accepted as such. Together they produce **tokens for an arbitrary user, obtained without the password and without the attacker intercepting anything.**

The mirror-image chain is also worth holding: **an open redirect on any domain that appears in a client's registered callbacks is an OAuth vulnerability**, because it converts a matched destination into an arbitrary one.

### Detection and mitigation

- **Decode and normalise the `redirect_uri` exactly once, in one place, and route on the result.** The measured escape exists because two layers each did one decode. The mitigation is structural: a single normalisation step whose output is what both the comparison and the routing see.
- **Alert when the number of decodes matters.** A `redirect_uri` containing `%25` — an encoded percent sign — is either a double-encoded sequence or a probe for one, and no legitimate client registers a callback that needs decoding twice.
- **Alert on a second exchange of the same authorization code, and on an exchange from a different address or client.** Both are the direct traces of a code that reached someone else, and the first is the single strongest signal available in an OAuth deployment.
- **Alert on outbound requests made by the authorization server.** It should fetch from a short list of known issuers and, at most, registered client metadata from allowlisted hosts. **Any request it makes to an address supplied in a request or a registration is worth an alert on its own**, because the set of legitimate destinations is small and known.
- **Alert when a client's requested scope changes.** A registration that starts requesting more than it did is either a compromise or a scope-creep bug, and both are cheap to spot because the value is recorded.
- **For mitigation, apply the allowlist rule to every URL the server fetches.** `request_uri` should be a stored reference rather than a fetched URL where the specification permits it, `jwks_uri` should come from a registered allowlist, and dynamic registration should require authentication and review rather than being open.
- **Require PKCE for every client, and check `state` and `nonce` on every response.** These remove the interception and substitution steps of the chain regardless of how the redirect validation behaves.
- **Make callbacks a list of exact strings, and keep open redirects off every one of them.** A redirect on a callback domain is what turns a matched destination into an arbitrary one, and the two findings are much more dangerous together than either is alone.
- **And write the consent screen for the person reading it.** Scope elevation is only possible because the screen lists capabilities rather than consequences; describing what the client will be able to do to the user's data is the control that makes the delegation decision real.

<!-- lang:zh -->
### 精确匹配是必需的，而且它不够

上一篇量了 `redirect_uri` 的校验，结论是只有精确比较拒绝了每一个绕过。那个结论是对的，而它有一个前提值得说出来：**跟什么精确比较？**

一个 URL 不是一个值。它会被收到它的那一层解码、会被路由它的那一层规范化、会被决定"它是否被允许"的那一层比较 —— 而如果那是三套规则不同的三层，那么"精确"描述的是那次比较，而不是那个结果。

### 校验器与路由器

实测：一个在比较之前把路径解码一次的允许检查，与一个在路由之前再解码一次的服务器：

| 情形 | 判定 | 校验器看到的路径 | 服务器到达的路径 |
|---|---|---|---|
| 注册值本身 | 放行 | `/callback` | `/callback` |
| 普通穿越 | 拒绝 | `/evil` | `/evil` |
| 单次编码的点 | 拒绝 | `/evil` | `/evil` |
| **双重编码的点** | **放行** | **`/callback/%2e%2e/%2e%2e/evil`** | **`/evil`** |
| 双重编码的斜杠 | 拒绝 | `/callback%2f..%2f..%2fevil` | `/evil` |
| 三重编码的点 | 放行 | `/callback/%252e%252e/evil` | `/callback/%2e%2e/evil` |

第四行就是发现所在。在校验器看来，`/callback/%2e%2e/%2e%2e/evil` **在注册路径之内** —— 第一次解码把 `%252e` 变成 `%2e`，那还不是一个点，所以规范化不动它，而这个字符串仍然以 `/callback` 开头。在服务器看来，再解一次就把 `%2e%2e` 变成 `..`，路径解析成 `/evil`。**同一个字符串，对一层是安全的，对另一层在别处**，而两层都在正确地做自己的事。

**每一层解码，就多一层。** 在一个三重编码的 payload 上实测：

```
解 0 次 -> /callback/%25252e%25252e/evil
解 1 次 -> /callback/%252e%252e/evil
解 2 次 -> /callback/%2e%2e/evil
解 3 次 -> /callback/../evil
```

所以问题从来不是"它有没有被编码两次"—— 而是**每一层各解码几次，而那些次数在哪里不同**。这正是这份指南赖以成立的那套漏洞本质四层标准里的第四层：**做校验的层与做解码的层不是同一层**，以它最字面的形式出现。

### 同一个分歧，在别处叫什么

这不是一个 OAuth 专有的问题，而见过一次之后，它在哪里都认得出来：

| A 层 | B 层 |
|---|---|
| `redirect_uri` 的校验器 | 跟着那个 URL 走的浏览器或客户端 |
| WAF 的规范化 | 源站框架的解码 |
| 缓存键的计算 | 应用的路由 |
| 反向代理的路径规范化 | 后端的路径解析 |
| SSRF 那篇里的允许清单检查 | 真正发起连接的解析器与 HTTP 客户端 |

**每一对只要差一条规则，就留下一个字符串：一边认为是新的，另一边不认为** —— 而那个字符串就是利用。修法不是"加更多校验"，而是**让某一层成为唯一的标准**，下游全都读它产出的东西。

### 授权服务器会主动去取哪些 URL

OAuth 还有第二族发现，它们与重定向无关：授权服务器、以及客户端注册流程，会**去取由别人提供的 URL**。

| 字段 | 由谁提供 | 什么时候去取 |
|---|---|---|
| `jwks_uri` | 注册这个客户端的一方 | 校验那个客户端的 token 时 |
| `request_uri` | 发起这次授权请求的人 | 处理那个请求时 |
| `logo_uri`、`policy_uri`、`tos_uri` | 注册这个客户端的一方 | 显示同意页面时 |
| `software_statement`（动态注册） | 任何能注册的人 | 注册时 |

上面每一个都是 SSRF 那篇的模式，只不过后果比平常更大。**授权服务器通常是一个特权很高的内部系统**：它持有签名密钥、它能以任何客户端的身份行动、它能读到其他客户端的注册信息，而且它经常处在"够得到任何用户都够不到的服务"的位置上。

- **`request_uri`** 是其中最厉害的，因为它在**授权请求**里设置，而不是在注册时 —— 意味着它不需要客户端凭据，而被取回的内容会被当作一个请求对象来解释。
- **`jwks_uri`** 更慢但更值钱：一个能把它指向内网端点的客户端，可能让服务器去读某个东西，并（取决于校验路径）观察到结果。
- **动态客户端注册**在它开放的时候，让任何人都能提供上面的一切。

规则和 SSRF 那篇一样：**一个会导致服务器发起请求的值，是一个需要允许清单而不是过滤器的值。**

### scope 提升，以及隐式流程

**scope 提升**不破坏任何检查。客户端请求的比用户以为的多，同意页面用很少人会读的措辞把它列出来，授权服务器就签发了 —— **同意页面是唯一的控制**，所以它必须写得具体，而不是一串权限名。

**隐式流程**把 token 放在 URL 的 **fragment** 里返回，浏览器不会把它发给服务器，但页面上的脚本能看到它，而地址栏里的一切也会留下痕迹 —— 浏览历史、截图、以及当 fragment 被复制进链接时的 referrer。对公共客户端，它已经被"授权码流程加 PKCE"取代，而仍在用的地方，值得把它本身当作一条发现。

**而 `nonce`** 是身份层里与 `state` 对等的东西：它把一个 `id_token` 绑定到请求它的那次请求上，而这正是让一个被重放的、或预先生成的 `id_token` 失败的东西。

### 那些小发现是怎么串起来的

上面每一条本身都不是账号接管，而这一类之所以要紧，就在于它们怎么组合：

```
redirect_uri 匹配宽松      ->  授权码被送到攻击者那里
+ 没有 PKCE                ->  那个码可以被攻击者兑换
+ 客户端上有个开放重定向    ->  一跳能到达被允许的目的地
+ state 不被校验           ->  来自别处的码落进了受害者的会话
```

每一行单独看都是一条中等严重性的发现，而且经常就这样被接受。它们连起来就产出**任意用户的 token，全程没有密码，攻击者也没有拦截任何东西。**

镜像的那条链也值得记住：**任何一个出现在客户端注册回调里的域名上存在开放重定向，就是一个 OAuth 漏洞**，因为那条重定向把一个"匹配上的目的地"变成了任意的目的地。

### 检测与缓解

- **把 `redirect_uri` 的解码与规范化只做一次、只在一处做，并按那个结果路由。** 实测里那个逃逸之所以存在，是因为两层各解了一次。缓解是结构性的：唯一一步规范化，而比较与路由看到的都是它的输出。
- **当"解了几次"变得要紧时告警。** 一个含 `%25`（被编码的百分号）的 `redirect_uri`，要么是一段双重编码的序列，要么是对它的一次探测 —— 而没有正当客户端会注册一个需要解码两次的回调。
- **对同一授权码的第二次兑换、以及从不同地址或客户端发起的兑换告警。** 两者都是"授权码到了别人手里"的直接痕迹，而前者是一次 OAuth 部署里现有的最强单项信号。
- **对授权服务器发起的出站请求告警。** 它应当只从一份已知签发者的短清单上取，最多再加上来自允许清单主机的已注册客户端元数据。**它向一个"出现在请求或注册里的地址"发起的任何请求，本身就值得一条告警**，因为正当目的地的集合很小且已知。
- **当某个客户端请求的 scope 发生变化时告警。** 一个注册开始请求比以前更多的权限，要么是被攻陷，要么是一个权限膨胀的 bug，而两者都很便宜就能发现，因为那个值是被记录下来的。
- **缓解上，把允许清单规则用在服务器会去取的每一个 URL 上。** 在规范允许的地方，`request_uri` 应当是一个存储下来的引用而不是一个被取回的 URL；`jwks_uri` 应当来自一份注册过的允许清单；动态注册应当要求认证与审核，而不是开放。
- **对每个客户端都要求 PKCE，并在每次响应上检查 `state` 与 `nonce`。** 无论重定向校验表现如何，这些都移除了那条链里的拦截与替换两步。
- **把回调做成一份精确字符串的清单，并让其中每一个都不存在开放重定向。** 回调域名上的一个重定向，正是把"匹配上的目的地"变成任意目的地的东西，而这两条发现合起来比各自单独危险得多。
- **并且把同意页面写成给读它的人看的。** scope 提升之所以可能，是因为那个页面列的是能力而不是后果；描述"这个客户端将能对你的数据做什么"，才是让这次委托决定变得真实的那项控制。
