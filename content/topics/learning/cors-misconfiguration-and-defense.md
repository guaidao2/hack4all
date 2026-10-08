---
id: cors-misconfiguration-and-defense
title_en: "CORS, Part 2 — Allowlists That Match More Than They Should"
title_zh: "CORS（二）：匹配了过多东西的允许清单"
summary_en: Six ways to write an origin allowlist, measured against ten origins, with each style admitting between two and five non-trusted sources and only exact comparison admitting none. Plus the caching interaction — a response that varies by Origin has to say so.
summary_zh: 六种写允许清单的方式，对着十个来源实测：每一种都放行了两到五个非信任来源，而只有精确比较一个都没放行。再加上缓存那层交互 —— 一个随 Origin 变化的响应必须把它说出来。
tags: [web, cors, cwe-942, allowlist, caching]
tools: [curl, python3, browser devtools]
attck: [T1190, T1539]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The allowlist is the whole decision

The previous entry established that a reflected `Origin` is not a policy. The remaining question is what a policy looks like when it is written with an intention to restrict — and the answer is a table of six reasonable attempts, each of which admits something it should not.

Measured, against ten origins, with `https://partner.example` as the trusted value:

| Origin submitted | Substring | Prefix | Suffix | Regex (dot unescaped) | Host only | Allows `null` |
|---|---|---|---|---|---|---|
| the trusted origin | allow | allow | allow | deny | allow | allow |
| `partner.example.evil.example` | **allow** | **allow** | deny | deny | deny | **allow** |
| `evil.example/?x=https://partner.example` | **allow** | deny | **allow** | **allow** | deny | **allow** |
| `evil-partner.example` | deny | deny | **allow** | **allow** | deny | deny |
| `partner.evil.example` | deny | deny | deny | deny | deny | deny |
| `evilXpartnerYexample` | deny | deny | deny | **allow** | deny | deny |
| `http://partner.example` | deny | deny | **allow** | deny | **allow** | deny |
| `partner.example:8443` | **allow** | **allow** | deny | deny | **allow** | **allow** |
| `null` | deny | deny | deny | deny | deny | **allow** |

And the count each style admits:

| Style | Non-trusted origins admitted |
|---|---|
| Allows `null` | **5** |
| Substring | **4** |
| Prefix | 3 |
| Suffix | 3 |
| Regex with an unescaped dot | 3 |
| Host only | 2 |
| **Exact comparison** | **0** |

**Only exact comparison refused every one of them**, which is the sixth time this guide has arrived at the same conclusion from a different starting point.

### Reading the table

**Substring and prefix both admit a longer domain beginning with the trusted one.** `partner.example.evil.example` contains the trusted value and starts with it, while its registrable domain is `evil.example`. That is the host header entry's substring whitelist, unchanged.

**Substring also admits the trusted value appearing elsewhere in the URL.** `https://evil.example/?x=https://partner.example` contains the trusted string as a parameter while the origin is `evil.example` — the same shape as the OAuth `redirect_uri` case and the CSRF `Referer` case.

**Suffix matching admits a hyphenated neighbour.** `origin.endswith("partner.example")` accepts `evil-partner.example`, because a suffix match without a **label boundary** does not care what character precedes the match. The correct form is the leading dot:

```python
origin.endswith("." + trusted_host)      # label boundary
```

This is the exact rule the host header entry gave for hostnames, in the place where it is a read permission instead of a routing decision.

**A regex with an unescaped dot admits an arbitrary character.** `re.match(r"https://.*.partner.example", origin)` — the dot between `.*` and `partner` is the **regex any-character metacharacter**, not a literal dot. Measured, `https://evilXpartnerYexample` matches it: an origin whose registrable domain is `example`, admitted by a pattern written in a hurry. Every literal dot in a pattern that matches hostnames has to be escaped.

**Host-only comparison forgets two things.** Checking `urlsplit(origin).hostname` against the trusted name accepts `http://partner.example`, and accepts `https://partner.example:8443`. A scheme downgrade means the response is readable and **modifiable** in transit, and the CORS permission travels with it; a port difference means a different service on the same host is being granted the right to read.

**Allowing `null` admits a set of readers nobody can enumerate.** The `null` origin is what a sandboxed iframe, a `data:` URL and a local file present. Measured, it is admitted by five of the six styles — and it is the one entry that cannot be justified by any business requirement, because a real partner is never `null`.

### The caching interaction

The cache entry and this one meet at a specific piece of configuration: **a response whose content depends on the request's `Origin` must declare that dependency, or a cache will serve one origin's response to another.**

Measured, with a cache keyed on the path and an application that reflects an allowed origin:

```
without Vary: Origin
  request from the trusted partner  -> Allow-Origin: https://partner.example   MISS
  request from the attacker         -> Allow-Origin: https://partner.example   HIT (cached)

with Vary: Origin
  request from the trusted partner  -> Allow-Origin: https://partner.example   MISS
  request from the attacker         -> Allow-Origin: (absent)                  MISS
```

Without `Vary`, the attacker's request hits the cached entry and receives **a permission header generated for somebody else** — which is the cache poisoning entry's rule applied to a CORS header: *the response varies by an input, so that input belongs in the key*. The cost of `Vary: Origin` is a lower cache hit rate, and it is the cost of being correct.

There is a second caching layer inside the browser: **`Access-Control-Max-Age`** makes the browser store a preflight result for a period. A deployment that fixes its CORS configuration will find clients still acting on the previous answer until that expires, which is worth knowing when verifying a fix.

### Detection and mitigation

- **Alert when `Access-Control-Allow-Origin` is not one of the configured values.** This is the same rule as the previous entry stated positively: a correct policy emits a fixed value, so any other value — the request's origin, a `null`, a wildcard on an endpoint that carries sessions — is worth a look.
- **Alert when the same URL answers different origins with different `Allow-Origin` values and does not send `Vary: Origin`.** That is the measured combination, and it is detectable purely from response headers: a varying value and a missing declaration is a cache waiting to hand a permission to the wrong client.
- **Alert on `null`.** Nothing legitimate needs it, and it is the entry with the largest admitted set in the measured table.
- **And watch for the hyphenated and parameter-bearing shapes in `Origin`.** `evil-partner.example` and `https://evil.example/?x=https://partner.example` are not forms a browser produces for a real origin; their presence is a probe against a substring or suffix check.
- **For mitigation, keep the allowlist a list of exact origins.** Not a set of rules, not patterns, not hostnames with a scheme check added afterwards. An origin includes the scheme, the host **and** the port, and all three are part of the identity.
- **Escape every literal dot in any pattern that matches a hostname**, and prefer not to use patterns at all for this — the measured regex case shows how a single unescaped character widens the match to anything.
- **If suffix comparison is genuinely needed, require a label boundary** — the leading `.` — so that `evil-partner.example` is not a suffix of the trusted domain.
- **Never allow `null`**, and treat `Allow-Credentials` as the switch that decides whether a mistake is a public read or a private one.
- **Send `Vary: Origin` on any response whose CORS headers depend on the request's origin**, and accept the cache hit rate as the price. Where that is unacceptable, the endpoint's CORS behaviour has to stop depending on the request, which means an exact match against a fixed value.
- **And test the policy with the shapes from the table rather than with a browser.** Each row is a `curl` invocation, and the measured results are what a correct configuration should refuse — which makes this a checklist that can be run before deployment rather than discovered afterwards.

<!-- lang:zh -->
### 允许清单就是那个全部的决定

上一篇立起了"反射 `Origin` 不是策略"。剩下的问题是：一个**确实想限制**的策略长什么样 —— 而答案是一张六种合理尝试的表，每一种都放行了它本不该放行的东西。

实测，对着十个来源，受信值是 `https://partner.example`：

| 提交的来源 | 子串 | 前缀 | 后缀 | 正则（点未转义） | 只比 host | 允许 `null` |
|---|---|---|---|---|---|---|
| 受信来源本身 | 放行 | 放行 | 放行 | 拒绝 | 放行 | 放行 |
| `partner.example.evil.example` | **放行** | **放行** | 拒绝 | 拒绝 | 拒绝 | **放行** |
| `evil.example/?x=https://partner.example` | **放行** | 拒绝 | **放行** | **放行** | 拒绝 | **放行** |
| `evil-partner.example` | 拒绝 | 拒绝 | **放行** | **放行** | 拒绝 | 拒绝 |
| `partner.evil.example` | 拒绝 | 拒绝 | 拒绝 | 拒绝 | 拒绝 | 拒绝 |
| `evilXpartnerYexample` | 拒绝 | 拒绝 | 拒绝 | **放行** | 拒绝 | 拒绝 |
| `http://partner.example` | 拒绝 | 拒绝 | **放行** | 拒绝 | **放行** | 拒绝 |
| `partner.example:8443` | **放行** | **放行** | 拒绝 | 拒绝 | **放行** | **放行** |
| `null` | 拒绝 | 拒绝 | 拒绝 | 拒绝 | 拒绝 | **放行** |

以及每种写法放行的数量：

| 写法 | 放行的非信任来源 |
|---|---|
| 允许 `null` | **5** |
| 子串匹配 | **4** |
| 前缀匹配 | 3 |
| 后缀匹配 | 3 |
| 正则点未转义 | 3 |
| 只比 host | 2 |
| **精确比较** | **0** |

**只有精确比较拒绝了它们全部**，而这是这份指南第六次从不同的起点走到同一个结论。

### 读那张表

**子串与前缀都放行了一个"以受信域名开头、但更长"的域名。** `partner.example.evil.example` 含受信值、也以它开头，而它的注册域是 `evil.example`。这就是 Host 头那篇的子串白名单，原样搬过来。

**子串还会放行"受信值出现在 URL 里别处"的情形。** `https://evil.example/?x=https://partner.example` 把受信字符串当作参数含在里面，而来源是 `evil.example` —— 和 OAuth 的 `redirect_uri`、CSRF 的 `Referer` 是同一个形状。

**后缀匹配会放行一个连字符邻居。** `origin.endswith("partner.example")` 接受 `evil-partner.example`，因为不带**标签边界**的后缀匹配不在乎匹配之前是什么字符。正确的形式是带那个前导点：

```python
origin.endswith("." + trusted_host)      # 标签边界
```

这正是 Host 头那篇给出的主机名规则，用在一个"读许可"而不是"路由决定"的地方。

**一个点没转义的正则会放行任意字符。** `re.match(r"https://.*.partner.example", origin)` —— `.*` 与 `partner` 之间的那个点是**正则的"任意字符"元字符**，不是字面点。实测，`https://evilXpartnerYexample` 匹配它：一个注册域是 `example` 的来源，被一条赶着写出来的模式放行了。任何用于匹配主机名的模式里，每一个字面点都必须转义。

**只比 host 会忘掉两件事。** 拿 `urlsplit(origin).hostname` 与受信名字比较，会接受 `http://partner.example`，也会接受 `https://partner.example:8443`。scheme 降级意味着响应在路上**可读也可改**，而 CORS 的许可跟着一起走；端口不同意味着同一台主机上的另一个服务被授予了读的权限。

**允许 `null` 就是放行了一批没人能枚举的读者。** `null` 这个来源是沙箱 iframe、`data:` URL 与本地文件所呈现的东西。实测它被六种写法里的五种放行 —— 而它是唯一一个无法被任何业务需求辩护的条目，因为真实的合作方从来不是 `null`。

### 缓存那一层交互

缓存那篇和这一篇在一个具体的配置点上相遇：**一个内容取决于请求 `Origin` 的响应必须声明这个依赖，否则缓存会把一个源的响应服务给另一个源。**

实测，用一个按路径做键的缓存，和一个会反射允许来源的应用：

```
没有 Vary: Origin
  来自受信合作方的请求 -> Allow-Origin: https://partner.example   MISS
  来自攻击者的请求     -> Allow-Origin: https://partner.example   HIT（缓存）

有 Vary: Origin
  来自受信合作方的请求 -> Allow-Origin: https://partner.example   MISS
  来自攻击者的请求     -> Allow-Origin:（缺失）                    MISS
```

没有 `Vary` 时，攻击者的请求命中那份缓存条目，收到**一个为别人产生的许可头** —— 那就是缓存投毒那篇的规则用在一个 CORS 头上：*响应随某个输入变化，那个输入就属于键*。`Vary: Origin` 的代价是更低的缓存命中率，而那是"做对"的代价。

浏览器内部还有第二层缓存：**`Access-Control-Max-Age`** 让浏览器把一次预检结果存一段时间。一个修好自己 CORS 配置的部署会发现客户端在它过期之前仍然按上一个答案行事 —— 这在验证一次修复时值得知道。

### 检测与缓解

- **当 `Access-Control-Allow-Origin` 不是配置里的那些值之一时告警。** 这和上一篇那条规则是同一个，只是从正面说：正确的策略发出一个固定的值，所以任何别的值 —— 请求的来源、一个 `null`、一个出现在承载会话的接口上的通配符 —— 都值得看一眼。
- **当同一个 URL 对不同来源回出不同的 `Allow-Origin`、却没有发 `Vary: Origin` 时告警。** 那正是实测出来的组合，而它纯粹从响应头就能发现：一个变化的值加一个缺失的声明，就是一个等着把许可交给错的客户端的缓存。
- **对 `null` 告警。** 没有任何正当需求需要它，而它是实测表里放行集合最大的那个条目。
- **并且盯 `Origin` 里那些连字符和带参数的形状。** `evil-partner.example` 与 `https://evil.example/?x=https://partner.example` 都不是浏览器会为真实来源产生的形式；它们出现就是一次针对子串或后缀检查的探测。
- **缓解上，让允许清单保持为一份精确来源的清单。** 不是一组规则、不是模式、也不是"主机名再加一个 scheme 检查"。一个来源包含 scheme、host **和**端口，三者都是那个身份的一部分。
- **在任何用于匹配主机名的模式里转义每一个字面点**，并且最好不要为这件事用模式 —— 实测那个正则例子显示了单个未转义字符能把匹配放宽到什么程度。
- **如果确实需要后缀比较，就要求一个标签边界** —— 那个前导点 —— 这样 `evil-partner.example` 就不是受信域名的后缀。
- **绝不允许 `null`**，并把 `Allow-Credentials` 当作那个"决定一次失误是公开读还是私密读"的开关。
- **在任何 CORS 头取决于请求来源的响应上发 `Vary: Origin`**，并把缓存命中率当作价格接受下来。在这个价格无法接受的地方，那个接口的 CORS 行为就必须停止依赖请求 —— 也就是对固定值做精确匹配。
- **并且用表里那些形状来测这条策略，而不是用浏览器。** 每一行都是一条 `curl` 命令，而实测结果就是一条正确配置应当拒绝的清单 —— 这让它成为一份可以在部署前跑的检查表，而不是事后才发现的东西。
