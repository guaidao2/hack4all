---
id: csrf-tokens-and-bypass
title_en: "CSRF, Part 2 — Tokens, and the One Assumption They Rest On"
title_zh: "CSRF（二）：token，以及它唯一依赖的那个前提"
summary_en: A synchroniser token works because another origin cannot read it, so every bypass is either that assumption being broken or the token being made generic. Measured — a token not bound to a session accepts the attacker's own, and four origin-checking styles against six request shapes.
summary_zh: 同步器 token 之所以有效，是因为另一个源读不到它，所以每一种绕过要么是这个前提被打破、要么是 token 被做成了通用的。实测：不与会话绑定的 token 会接受攻击者自己的那个，以及四种来源校验实现对着六种请求形状的表现。
tags: [web, csrf, cwe-352, tokens, origin, samesite]
tools: [curl, python3, Burp Suite]
attck: [T1185, T1550]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### One assumption, and everything that breaks it

The previous entry established that a cross-site request arrives indistinguishable from a legitimate one, because the browser attaches the cookie by destination. The defence is a value the attacker's page cannot produce:

> **A synchroniser token works because it is stored server-side against the session, and a page on another origin cannot read it.**

That "cannot read it" is not a property of the token; it is the **same-origin policy**. Everything below is either that assumption being broken, or the token being weakened until reading it is unnecessary.

### The bypasses, grouped by how the assumption fails

**The token is not bound to the session.** This is the most common and the most consequential, because it needs nothing from the victim's browser. Every logged-in user has a token — including the attacker. Measured:

| Request | Bound to session | Not bound |
|---|---|---|
| The victim, with their own token | accepted | accepted |
| The victim's session, with **the attacker's token** | refused | **accepted** |
| No token at all | refused | refused |

The third row is the whole attack: **the attacker registers an account, reads their own token from their own page, and uses it in a request sent with the victim's cookie.** The token was valid — it just was not *this session's* token. A `GET` endpoint that renders the token into a page is enough to obtain one, so the cost to the attacker is one registration.

**The token is read from a cookie rather than from server state.** The double-submit pattern sends the token twice — once as a cookie, once in a request field — and compares the two. It works only if the attacker cannot set a cookie for the target's domain, which fails when a subdomain is compromised, when the cookie is not host-scoped, or when any other cookie-injection path exists. **The comparison proving the two copies agree says nothing about who sent them.**

**An XSS on the same origin.** Covered below, because it is qualitative rather than incremental.

**The token reaches a place that leaks it.** A token in a URL lands in the `Referer` header of outbound links, in the access log, and in browser history. A token in a page's HTML is fine; a token in a query string is not.

**The check covers only some methods or paths.** Measured:

| Method | Token checked |
|---|---|
| `POST` | yes |
| `PUT` | **no** |
| `PATCH` | **no** |
| `DELETE` | **no** |
| `GET` | no |

If a route accepts both `POST` and `PUT` — which many frameworks do for the same handler — and the check lives in the `POST` branch, then `PUT` is a synonym for the same state change with none of the protection.

**And the token itself is predictable.** A token generated from a non-cryptographic source is guessable, and that is the session entry's rule applied to a second value: anything that stands in for a secret has to come from a source built for secrets.

### Origin checking, and the same comparison error again

`Origin` is set by the browser rather than by the page, it is present on cross-origin writes, and it is not affected by the privacy settings that strip `Referer`. It is a good control, and the way it is usually implemented is the same mistake this guide has now documented four times.

Measured, four implementations against six request shapes:

| Request | Exact | Prefix | Substring | Missing means allow |
|---|---|---|---|---|
| Normal same-origin | allow | allow | allow | allow |
| Cross-site | deny | deny | deny | deny |
| Malicious subdomain | deny | deny | deny | deny |
| **`app.example.evil.example`** | deny | **allow** | **allow** | deny |
| **`Referer` containing the origin elsewhere** | deny | deny | **allow** | deny |
| **Neither header present** | deny | deny | deny | **allow** |

**Only exact matching refused every cross-site shape.** The failures are worth reading individually because they are the same failures as before, in a new place:

- **`app.example.evil.example`** — a prefix or substring check finds the site's name at the start of the string and allows it, while the destination's registrable domain is `evil.example`. This is the host header entry's substring whitelist.
- **A `Referer` containing the origin as a parameter** — a substring check asks "does our name appear anywhere", and it does, in a URL whose actual origin is elsewhere. This is the OAuth entry's `redirect_uri` case.
- **Neither header present** — treating absence as permission means **omitting a header becomes a bypass**. Modern browsers send `Origin` on cross-origin writes, so the correct rule is the opposite: no header, no write.

**Comparing origins by similarity fails for the same reason comparing hostnames and redirect targets does** — these are identity questions, and similarity is not identity. The correct check is an exact match against a short known list, with absence refused.

### XSS defeats tokens, so the order of review matters

This is the part that changes conclusions rather than probabilities.

A token protects against **another origin's page**. An XSS payload runs **on your origin**, in a document that the same-origin policy allows to read anything the page can read — including the token, whether it is in the DOM, in a meta tag, or fetched from an endpoint.

So:

- **On an application with an XSS, the CSRF token is decoration.** The attacker reads it and sends a request that satisfies every check.
- **A CSRF token is not a substitute for XSS defence**, and it is not evidence that XSS does not exist.
- **A CSP helps both**, because it removes the injection that XSS needs and the script execution that would read the token — which is why the XSS entries' mitigation sections are also CSRF mitigation.

The practical consequence for a review: **check for XSS before concluding anything about CSRF.** The two findings are not independent, and a CSRF token present in the code is not a reason to skip the injection test.

### Detection and mitigation

- **Alert on state-changing requests with a foreign or absent `Origin`.** The expected values are a short known list, so this is a comparison; and because modern browsers send the header on cross-origin writes, **absence is at least as interesting as a foreign value**.
- **Alert when one token value is seen across more than one session.** That is the fingerprint of a token pool rather than session-bound tokens, and it is detectable purely from server-side comparison — no client-side signal needed.
- **Alert on bursts of token-validation failures.** Like signature failures on a session cookie or a JWT, a burst indicates an attempt to find a token or to find a request shape that skips the check.
- **And alert on unusual methods for a known operation.** A password change normally arrives as a `POST`; the same change arriving as a `PUT` is either a client the application did not expect or someone looking for the unguarded branch.
- **For mitigation, bind the token to the session and store it server-side.** The measured table shows the alternative: an unbound token accepts the attacker's own, and obtaining one costs a registration.
- **Rotate the token per request where the flow allows it,** which narrows the window a leaked or read token stays useful.
- **Check `Origin` exactly, and refuse when it is absent.** Exact match against a configured list, no prefix and no substring, and no "missing means allowed" fallback.
- **Apply the check to every method that can change state,** on the route rather than in a branch, so that a second method cannot reach the same handler without it.
- **Keep the token out of URLs**, and prefer server-side storage over a cookie comparison, so that reading it requires executing script on the origin rather than setting a cookie.
- **Fix the XSS first.** Until injection is closed, every control in this entry is conditional — and the review order matters more here than the individual checks.
- **And treat `SameSite` as depth.** It raises the cost of the generic attack, cannot separate applications sharing a registrable domain, and leaves the `Lax` navigation exception — all of which the previous entry measured.

<!-- lang:zh -->
### 一个前提，以及打破它的一切

上一篇立起了"跨站请求到达时与合法请求无法区分"，因为浏览器是按目的地附上 cookie 的。防御是一个攻击者页面产生不出来的值：

> **同步器 token 之所以有效，是因为它与会话一起存在服务端，而另一个源上的页面读不到它。**

那个"读不到"不是 token 的性质，它是**同源策略**。下面的一切，要么是这个前提被打破，要么是 token 被削弱到"不需要读它"。

### 绕过，按"前提怎么失效"分组

**token 不与会话绑定。** 这是最常见也后果最大的一个，因为它不需要受害者浏览器做任何事。每个已登录用户都有一个 token —— 包括攻击者。实测：

| 请求 | 绑定会话 | 不绑定 |
|---|---|---|
| 受害者带自己的 token | 接受 | 接受 |
| **受害者的会话，带攻击者的 token** | 拒绝 | **接受** |
| 完全不带 token | 拒绝 | 拒绝 |

第三行就是整个攻击：**攻击者注册一个账号，从自己的页面读出自己的 token，然后把它用在带着受害者 cookie 发出的请求里。** 那个 token 是有效的 —— 只是它不是**这个会话的** token。一个把 token 渲染进页面的 `GET` 接口就够拿到一个，所以攻击者的成本是一次注册。

**token 从 cookie 里读，而不是从服务端状态里读。** 双重提交模式把 token 发两次 —— 一次作为 cookie、一次在请求字段里 —— 然后比较两者。它成立的前提是攻击者无法为目标域设置 cookie，而这个前提在子域被攻陷、cookie 没有 host 作用域、或者存在任何别的 cookie 注入路径时就不成立。**"两份一致"这个结论关于'谁发的'什么都没说。**

**同源上的一个 XSS。** 下面单说，因为它不是量变而是质变。

**token 到了会泄露它的地方。** URL 里的 token 会落进外链的 `Referer` 头、访问日志和浏览器历史。token 在页面 HTML 里没事；token 在查询串里不行。

**校验只覆盖部分方法或路径。** 实测：

| 方法 | 校验 token |
|---|---|
| `POST` | 是 |
| `PUT` | **否** |
| `PATCH` | **否** |
| `DELETE` | **否** |
| `GET` | 否 |

如果一条路由同时接受 `POST` 与 `PUT` —— 很多框架对同一个处理器就是这么写的 —— 而校验住在 `POST` 那个分支里，那么 `PUT` 就是同一个状态变更的同义词，却不带任何保护。

**而 token 本身可预测。** 用非密码学随机源生成的 token 可以猜 —— 那是会话那篇的规则用在第二个值上：任何代替秘密的东西，都必须来自一个为秘密而造的源。

### 来源校验，以及同一个比较错误又出现一次

`Origin` 由浏览器设置而不是由页面设置，它出现在跨源写请求上，而且不受那些会剥离 `Referer` 的隐私设置影响。它是一项好控制，而它通常的实现方式，是这份指南已经记过四次的同一个错误。

实测，四种实现对着六种请求形状：

| 请求 | 精确 | 前缀 | 子串 | 缺失即放行 |
|---|---|---|---|---|
| 正常同源 | 放行 | 放行 | 放行 | 放行 |
| 跨站 | 拒绝 | 拒绝 | 拒绝 | 拒绝 |
| 恶意子域 | 拒绝 | 拒绝 | 拒绝 | 拒绝 |
| **`app.example.evil.example`** | 拒绝 | **放行** | **放行** | 拒绝 |
| **`Referer` 里在别处含本站 origin** | 拒绝 | 拒绝 | **放行** | 拒绝 |
| **两个头都没有** | 拒绝 | 拒绝 | 拒绝 | **放行** |

**只有精确匹配拒绝了每一个跨站形状。** 那几处失败值得逐行读，因为它们和之前是同样的失败，只是换了个地方：

- **`app.example.evil.example`** —— 前缀或子串检查在字符串开头找到了站点的名字就放行，而那个目的地的注册域是 `evil.example`。这是 Host 头那篇的子串白名单。
- **`Referer` 里在别处含本站 origin** —— 子串检查问的是"我们的名字有没有出现过"，而它出现了，在一个实际来源在别处的 URL 里。这是 OAuth 那篇的 `redirect_uri` 情形。
- **两个头都没有** —— 把缺失当作许可，意味着**省略一个头就成了绕过**。现代浏览器在跨源写请求上都会发 `Origin`，所以正确的规则是相反的：没有头，就没有写。

**用相似性比较来源会失败，理由和比较主机名、比较重定向目标一样** —— 这些是同一性问题，而相似不是同一。正确的检查是对一份很短的已知清单做精确匹配，并且把缺失也拒绝。

### XSS 击败 token，所以评审顺序是有意义的

这是改变**结论**而不只是改变概率的那一部分。

token 防的是**另一个源的页面**。一个 XSS payload 跑在**你自己的源上**，在一个同源策略允许它读取页面能读到的一切的文档里 —— 包括 token，无论在 DOM 里、在 meta 标签里、还是从一个接口取回来。

所以：

- **在一个存在 XSS 的应用上，CSRF token 是装饰。** 攻击者读走它，再发一个满足所有检查的请求。
- **CSRF token 不能替代 XSS 防护**，它也不是"没有 XSS"的证据。
- **一条 CSP 同时帮两者**，因为它移除了 XSS 需要的注入点、也移除了会去读 token 的脚本执行 —— 这就是为什么 XSS 那几篇的缓解部分同时也是 CSRF 的缓解。

对评审的实际后果是：**在得出任何关于 CSRF 的结论之前先查 XSS。** 这两条发现不是独立的，而代码里存在一个 CSRF token，不构成跳过注入测试的理由。

### 检测与缓解

- **对"来源外来或缺失"的改状态请求告警。** 期望值是一份很短的已知清单，所以这是一次比较；而由于现代浏览器在跨源写请求上都会发这个头，**"缺失"至少和一个外来的值一样值得看**。
- **当一个 token 值出现在不止一个会话里时告警。** 那是"token 池"而非"会话绑定 token"的指纹，而它纯粹靠服务端比对就能发现，不需要任何客户端信号。
- **对 token 校验失败的爆发告警。** 和会话 cookie、JWT 上的签名失败一样，一次爆发说明有人在找一个 token，或者在找一条跳过校验的请求形状。
- **对"已知操作使用了不寻常的方法"告警。** 改密码正常是 `POST` 到达的；同一个改动以 `PUT` 到达，要么是应用没预料到的客户端，要么是有人在找那条没有守卫的分支。
- **缓解上，把 token 与会话绑定并存服务端。** 实测那张表给出了另一条路的后果：不绑定的 token 会接受攻击者自己的那一个，而拿到一个的成本是一次注册。
- **在流程允许的地方按请求轮换 token**，这收窄了一个被泄露或被读走的 token 的可用窗口。
- **精确校验 `Origin`，并在它缺失时拒绝。** 对着配置好的清单做精确匹配，不用前缀、不用子串，也不要"缺失即允许"的兜底。
- **把校验用在每一个能改状态的方法上**，写在路由上而不是分支里，这样第二种方法就无法不带着它到达同一个处理器。
- **让 token 远离 URL**，并优先用服务端存储而不是 cookie 比对，这样要读到它就需要在源上执行脚本，而不是设置一个 cookie。
- **先修 XSS。** 在注入被关上之前，这一篇里的每一项控制都是有条件的 —— 而在这里，评审的顺序比单独的每一项检查更要紧。
- **并且把 `SameSite` 当纵深。** 它提高了通用攻击的成本、分不开共用可注册域的应用、还留着 `Lax` 那个导航例外 —— 这些上一篇都量过了。
