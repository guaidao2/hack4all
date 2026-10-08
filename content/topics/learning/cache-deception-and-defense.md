---
id: cache-deception-and-defense
title_en: "Web Cache Deception — Two Standards for One Resource"
title_zh: "Web 缓存欺骗：同一个资源，两套标准"
summary_en: Deception is poisoning seen from the other side. The cache decides by the shape of a URL whether something is a static asset while the application decides by its routes that it is the same private page. Measured across nine URL shapes that one application maps to a single resource, plus what remains when both sides normalise by different rules.
summary_zh: 缓存欺骗是投毒从另一侧看过去的样子。缓存按 URL 的**形状**判断它是不是静态资源，而应用按路由判断它是同一个私密页面。这一篇量了九个 URL 形状 —— 一个应用把它们全都映射到同一个资源 —— 以及两边各自规范化时仍然剩下的那条缝。
tags: [web, cache-deception, cwe-525, http, caching, normalisation]
tools: [curl, python3, Burp Suite]
attck: [T1190, T1539]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The same bug, seen from the other side

The previous entry described poisoning as an input that changes a response without changing the key, so one visitor's response is stored where another visitor will find it. **Deception is that relationship reversed:** the response is genuinely the user's own, and the problem is that it gets stored at all, under a key someone else can request.

| | Who is served the wrong thing | What the attacker controls |
|---|---|---|
| **Poisoning** | Every later visitor to that URL | A value that reaches the response |
| **Deception** | The victim, who receives their own page — and anyone who then requests the stored entry | Nothing; the victim's own response is what gets stored |

Both come from the same root: **the cache's idea of "the same resource" is not the application's idea of it.** Poisoning exploits the inputs the cache ignores; deception exploits the URL shapes the cache treats as a separate, cacheable thing.

### The two conditions deception needs

**The cache must think the URL is cacheable.** Caches are frequently configured by **extension**: `.css`, `.js`, `.jpg`, `.png` and their relatives are treated as static and stored, while an extensionless path is passed through. The rule is a reasonable default for a site whose static assets are real files.

**The application must return the private page anyway.** Web frameworks route by **path prefix or pattern**, and a suffix the router does not recognise is either ignored or pushed into a parameter. `/account/profile` and `/account/profile/x.css` frequently reach the same handler and produce the same page.

Measured against a cache keyed on the raw path with an application that routes by prefix:

| Step | Request | Result |
|---|---|---|
| Victim, logged in | `GET /account/profile` | **MISS** — the cache does not store extensionless paths, so the private page is not stored |
| Victim, lured | `GET /account/profile/x.css` | **MISS (stored)** — the same private page, now stored because of the extension |
| Attacker, no session | `GET /account/profile/x.css` | **HIT** — the account page, with the session token in the response body |

The victim's session is not needed by the attacker and not obtained: **the victim's own browser made the request that stored their page**, which is why this is delivered as a link to click rather than as an attack.

### Two standards for one resource

"How many resources are these?" is answered differently by each side, and the disagreement is broader than the extension case. Measured, with a cache keying on the raw path and an application that collapses duplicate slashes, removes single-dot segments, drops matrix parameters, strips a trailing slash and lowercases:

| URL | Cache key | Application route |
|---|---|---|
| `/account/profile` | `/account/profile` | `/account/profile` |
| `/account/profile/` | `/account/profile/` | **`/account/profile`** |
| `/account//profile` | `/account//profile` | **`/account/profile`** |
| `/account/./profile` | `/account/./profile` | **`/account/profile`** |
| `/ACCOUNT/PROFILE` | `/ACCOUNT/PROFILE` | **`/account/profile`** |
| `/account/profile;v=2` | `/account/profile;v=2` | **`/account/profile`** |
| `/account/profile/x.css` | `/account/profile/x.css` | **`/account/profile`** |
| `/account/profile?x=.css` | `/account/profile?x=.css` | **`/account/profile`** |
| `/account/profile/x.js` | `/account/profile/x.js` | **`/account/profile`** |
| `/account/profile%2Fx.css` | `/account/profile%2Fx.css` | **`/account/profile`** |

**Nine of those ten are one resource to the application and nine different keys to the cache.** Every one of them is a separately stored copy of the same private page, available to anyone who requests that exact shape.

And the disagreement survives partial fixes. Adding normalisation to the cache — collapsing slashes, dropping matrix parameters, stripping a trailing slash — still left two shapes mismatched, because the cache did not handle single-dot segments or case while the application did:

| URL | Cache key (normalised) | Application route |
|---|---|---|
| `/account/./profile` | `/account/./profile` | `/account/profile` |
| `/ACCOUNT/PROFILE` | `/ACCOUNT/PROFILE` | `/account/profile` |

**Normalising is not the fix; normalising identically is.** Two implementations of "the same URL" that differ by one rule still leave a shape that one side thinks is new and the other does not — and that shape is the gap.

### What the disagreement is worth, in both directions

**The cache being wider than the application** — several application resources behind one key — means one visitor's content served to another. That is the deception outcome above, and the same shape can be arranged the other way round: whatever gets stored under a shared key is displayed to everyone who reaches that key.

**The cache being narrower than the application** — one application resource spread across many keys — means the poisoning surface is multiplied. Each of the nine shapes is a key an attacker can poison, with the certain knowledge that the application will render that content for anyone visiting the canonical URL.

### Detection and mitigation

- **Alert when a response carrying `Set-Cookie`, an `Authorization` echo, or account data is stored by the cache.** This is the most direct signal, and it is available at the cache itself: a stored entry whose response marks it as private, or whose body varies by session, should not exist. Storing it is the finding, whether or not anyone has requested it yet.
- **Alert when a stored entry is served to a request with a different session or no session at all.** A cache that knows which user a stored response was produced for can notice that it is being handed to another. Where that comparison cannot be made, a distinctive per-response marker makes it possible after the fact.
- **Watch for static-looking requests to dynamic paths.** `.css`, `.js`, `.jpg` and their relatives appended to a path segment that the application treats as a page is the reconnaissance step, and it is cheap to see: the extension does not exist on disk, and the response is HTML.
- **And treat a cache whose content does not vary when it should as a finding.** A personalised path returning identical bytes for two sessions is either a broken cache key or a cached private page, and both need the same investigation.
- **For mitigation, let the origin declare cacheability rather than the cache inferring it.** `Cache-Control: private` or `no-store` on anything session-dependent removes the class, and it is the response's own statement about itself rather than a guess made from the URL.
- **Do not decide cacheability by extension.** The extension is in the URL, and the URL is attacker-controlled; a cache that stores `.css` and passes everything else through has handed the decision to whoever writes the link. An explicit allowlist of paths, or of origins whose responses may be stored, is the control that holds.
- **And make one implementation responsible for normalisation, applied before the cache key is computed.** If the edge rewrites paths to a canonical form and computes the key from the result, both sides are looking at the same string. Adding normalisation to each side independently produces the measured outcome — agreement on some shapes, disagreement on the rest.
- **On the application side, reject suffixes that are not part of the route.** A handler for `/account/profile` that returns the account page for `/account/profile/x.css` is the second condition of the attack. Returning 404 for unrecognised suffixes costs nothing and removes it.
- **The rule that both cache entries share, and that is worth carrying away:** a response's cacheability is a property of the response, declared by the origin. It is not a property of the URL's shape, and not something the cache should be guessing at.

<!-- lang:zh -->
### 同一个 bug，从另一侧看

上一篇把投毒描述成"某个输入改变了响应却没有改变键"，于是这位访客的响应被存在另一位访客会找到的地方。**欺骗则是这个关系的反面：** 响应确实是用户自己的，问题在于它被存了下来，而且存在别人也能请求的键下面。

| | 谁被服务了错的东西 | 攻击者控制什么 |
|---|---|---|
| **投毒** | 那个 URL 之后每一位访客 | 一个能到达响应的值 |
| **欺骗** | 受害者（他收到的是自己的页面）以及之后请求那份存储条目的人 | 什么都不用控制；被存下来的就是受害者自己的响应 |

两者同一个根源：**缓存对"同一个资源"的理解，不是应用的理解。** 投毒利用的是缓存忽略的那些输入；欺骗利用的是缓存当成"另一个可缓存的东西"的那些 URL 形状。

### 欺骗需要的两个条件

**缓存必须认为这个 URL 可缓存。** 缓存经常按**扩展名**配置：`.css`、`.js`、`.jpg`、`.png` 之类被当成静态资源存下来，而没有扩展名的路径直接放过。对一个"静态资源就是真文件"的站点来说，这是个合理的默认。

**而应用必须照样返回那个私密页面。** Web 框架按**路径前缀或模式**路由，而路由器不认识的后缀要么被忽略、要么被塞进一个参数。`/account/profile` 和 `/account/profile/x.css` 经常到达同一个处理器、产生同一个页面。

对着一个按原始路径做键、而应用按前缀路由的缓存实测：

| 步骤 | 请求 | 结果 |
|---|---|---|
| 受害者，已登录 | `GET /account/profile` | **MISS** —— 缓存不存无扩展名的路径，所以私密页面没被存 |
| 受害者，被诱导 | `GET /account/profile/x.css` | **MISS（已存储）** —— 同一个私密页面，因为那个扩展名而被存下 |
| 攻击者，无会话 | `GET /account/profile/x.css` | **HIT** —— 账户页，响应体里带着会话 token |

攻击者不需要受害者的会话，也没有拿到它：**是受害者自己的浏览器发出了那个把页面存下来的请求**。这也是为什么这种事是作为一条"点这个链接"来投递的，而不是作为一次攻击。

### 同一个资源，两套标准

"这些算几个资源"这个问题，两边给出的答案不同，而分歧比扩展名那种情况更广。实测：缓存按原始路径做键，应用则合并重复斜杠、去掉单点段、丢掉矩阵参数、剥掉结尾斜杠、并转成小写：

| URL | 缓存键 | 应用路由 |
|---|---|---|
| `/account/profile` | `/account/profile` | `/account/profile` |
| `/account/profile/` | `/account/profile/` | **`/account/profile`** |
| `/account//profile` | `/account//profile` | **`/account/profile`** |
| `/account/./profile` | `/account/./profile` | **`/account/profile`** |
| `/ACCOUNT/PROFILE` | `/ACCOUNT/PROFILE` | **`/account/profile`** |
| `/account/profile;v=2` | `/account/profile;v=2` | **`/account/profile`** |
| `/account/profile/x.css` | `/account/profile/x.css` | **`/account/profile`** |
| `/account/profile?x=.css` | `/account/profile?x=.css` | **`/account/profile`** |
| `/account/profile/x.js` | `/account/profile/x.js` | **`/account/profile`** |
| `/account/profile%2Fx.css` | `/account/profile%2Fx.css` | **`/account/profile`** |

**那十个里有九个，对应用来说是同一个资源，对缓存来说是九个不同的键。** 其中每一个都是同一个私密页面的一份独立存储副本，任何请求那个确切形状的人都能拿到。

而且这个分歧在部分修复之后仍然存在。给缓存加上规范化 —— 合并斜杠、丢掉矩阵参数、剥掉结尾斜杠 —— 仍然留了两个形状对不上，因为缓存没有处理单点段和大小写，而应用处理了：

| URL | 缓存键（规范化后） | 应用路由 |
|---|---|---|
| `/account/./profile` | `/account/./profile` | `/account/profile` |
| `/ACCOUNT/PROFILE` | `/ACCOUNT/PROFILE` | `/account/profile` |

**做规范化不是修法；做成一模一样的规范化才是。** 两套"同一个 URL"的实现只要差一条规则，就仍然留下一个"一边认为是新的、另一边不认为"的形状 —— 而那个形状就是缝。

### 那个分歧值多少，两个方向都算

**缓存比应用宽** —— 好几个应用资源共用一个键 —— 意味着一份访客的内容被服务给另一个。那就是上面那个欺骗结果，而同一个形状也能反过来安排：任何被存进那个共享键的内容，都会显示给到达那个键的每个人。

**缓存比应用窄** —— 一个应用资源散成很多键 —— 意味着投毒面被乘大了。那九个形状里每一个都是攻击者可以投毒的键，而他确知应用会把那份内容渲染给任何访问规范 URL 的人。

### 检测与缓解

- **当一个带 `Set-Cookie`、回显 `Authorization`、或者含账户数据的响应被缓存存储时告警。** 这是最直接的信号，而且在缓存那里就能拿到：一份"响应本身标记为私密"或者"内容随会话变化"的存储条目，本就不该存在。**它的存在就是发现**，无论有没有人请求过它。
- **当一份存储条目被服务给一个会话不同、或者根本没有会话的请求时告警。** 一个知道"这份响应是为哪个用户产生的"的缓存，能注意到它正被交给另一个人。做不到这种比对的地方，一个逐响应的独特标记能让事后比对成为可能。
- **盯"对动态路径的静态样子请求"。** `.css`、`.js`、`.jpg` 之类被拼在一个应用当成页面的路径段后面，就是侦察那一步，而它很便宜就能看到：那个扩展名在磁盘上不存在，而响应是 HTML。
- **并且把"一个本该变化的缓存内容却不变化"当作发现。** 一个个性化路径对两个会话返回完全相同的字节，要么是坏掉的缓存键，要么是一个被缓存的私密页面 —— 两者都需要同样的排查。
- **缓解上，让源站声明可缓存性，而不是让缓存去推断。** 对任何依赖会话的东西加 `Cache-Control: private` 或 `no-store` 就移除了这一类，而那是**响应对自己的一句陈述**，不是从 URL 猜出来的判断。
- **不要按扩展名决定可缓存性。** 扩展名在 URL 里，而 URL 是攻击者可控的；一个"存 `.css`、其余放过"的缓存，等于把决定权交给了写那条链接的人。一份明确的路径允许清单、或者"哪些源的响应可以存"的清单，才是站得住的控制。
- **并且让"规范化"只有一个实现负责，在计算缓存键之前施加。** 如果边缘把路径重写成规范形式、再拿结果去算键，那么两边看的就是同一个字符串。在两边各自独立地加规范化，得到的正是实测那个结果 —— 一部分形状一致，其余不一致。
- **在应用侧，拒绝不属于路由的后缀。** 一个 `/account/profile` 的处理器对 `/account/profile/x.css` 也返回账户页，就是那个攻击的第二个条件。对不认识的后缀返回 404 不付代价，而且移除了它。
- **两篇共享的那条规则，值得带走：** 一个响应的可缓存性，是响应自己的属性，由源站声明。它不是 URL 形状的属性，也不是缓存应该去猜的东西。
