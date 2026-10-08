---
id: hostheader-fundamentals
title_en: "Host Header Injection — Where the Site's Own Name Comes From"
title_zh: "Host 头注入：站点的名字不该来自请求"
summary_en: The Host header exists so a client can name the site it wants, and applications keep using it to answer a different question — who am I. Measured across the shapes a request can take, the substring whitelist that cannot work, and the chain that ends in account takeover.
summary_zh: Host 头的存在是为了让客户端说出它想访问哪个站点，而应用一直拿它回答另一个问题 —— 我是谁。这一篇量了请求能呈现的几种形状、为什么子串白名单必然失效，以及那条通向账号接管的链条。
tags: [web, host-header, cwe-644, password-reset, routing]
tools: [curl, python3, Burp Suite]
attck: [T1190, T1556]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### A header that answers a question about the client, used as an answer about the server

HTTP/1.1 requires every request to carry a `Host` header, and the reason is virtual hosting: several sites share an address, so the client has to say which one it means. The header is therefore **a statement from the client about what it wants** — in the same category as the path and the query string.

Applications then use it to answer a question about themselves: **what is my own name?** That substitution is the vulnerability, and it is worth stating as the rule it violates:

> **The address a site presents to the world is configuration. It is not request data.**

Everything that follows is a consequence. Password reset links, activation e-mails, absolute URLs in API responses, OAuth redirect URIs, cache keys and CSRF origin checks are all places where an application needs to know its own identity — and every one of them is a place where a value from the Host header, if used, lets the requester choose it.

It is a small bug with an outsized consequence, which is why it is worth understanding rather than pattern-matching: one forged header can produce **a genuine password reset e-mail, sent by the real application, containing a link to the attacker's domain** — with the victim's token in the URL.

### Four layers, as with every injection

1. **The trust boundary.** The application trusts the client's description of which site it is, in order to decide what the site calls itself. Both sides behave correctly: the client sent a syntactically valid `Host`, and the framework reported it faithfully.
2. **Data and instruction share a plane.** The same field is both a **routing instruction** ("serve the virtual host named here") and **data used to build a link**. Nothing distinguishes the two roles, so a value chosen for the first is used in the second.
3. **Why the usual fix fails.** The measured lab validates the header against a whitelist — and the whitelist uses **substring matching**. Domain names are read **right to left**, so a name that *contains* the site's domain is not the site's domain:

   ```
   Host: vuln4all.local.evil.example        allowed by a substring check
   -> the mail links to  http://127.0.0.1.evil.example/reset?token=...
   ```

   The generated link is measured, and it points off-site. The whitelist ran, and it passed the wrong value.
4. **The variants.** `X-Forwarded-Host`, an absolute URI in the request line, two `Host` headers, a trailing dot, mixed case, an appended port, and an embedded `@`.

### Who uses the header

Before looking for the payload, it is worth knowing which parts of a stack consult this value, because the list is longer than it looks and each entry is a separate way to be surprised:

| Consumer | What it does with the value |
|---|---|
| **Link generation** | Builds absolute URLs for e-mails, resets, callbacks, invitations |
| **Routing and virtual hosts** | Chooses which site to serve |
| **Cache keys** | Some caches include the host in the key — the web cache poisoning entry builds on this |
| **CSRF and CORS origin checks** | Decides whether a request came from "us" |
| **Redirects** | Builds `Location` headers and post-login destinations |
| **Logging and audit** | Records which site was accessed |

**One link generator is enough.** If any code path builds an absolute URL from the request's host, and that URL reaches a user by e-mail, then the injection exists — regardless of how the rest of the stack treats the header.

### What actually arrives, and what each stack reads

HTTP itself does not object to most of the interesting shapes. Measured on a raw socket listener, every one of these arrived intact:

| Request | What the server received |
|---|---|
| `Host: good.example` | as sent |
| Two `Host` headers | **both** headers, in order |
| `Host` plus `X-Forwarded-Host` | **both** headers |
| `GET http://evil.example/ HTTP/1.1` with `Host: good.example` | absolute URI in the request line **and** the `Host` header |
| `Host: good.example.` (trailing dot) | as sent |
| `Host: good.example:8080` | as sent |
| `Host: good.example@evil.example` | as sent |

So the shapes are not blocked; they are all well-formed HTTP. What differs is **which one a given stack decides is authoritative**, and that is where the measurements get interesting. Through a real HTTP stack:

- **`X-Forwarded-Host` and `Host` are both readable, separately.** An application that prefers the first — a common choice for deployments behind a proxy — is reading a header that any client can set, and the lab's application does exactly this.
- **An absolute URI in the request line arrives as the path.** The measured request `GET http://evil.example/ HTTP/1.1` with `Host: good.example` produced `path='http://evil.example/'` and `Host='good.example'` — two candidates in one request, and which one the application uses is a framework decision.
- **Two `Host` headers do not have one answer.** Some stacks take the first, some the last, and some reject the request or report an empty host — the lab measured an empty host from Werkzeug, and an empty value is still not a rejection.
- **A trailing dot resolves.** `example.com.` is the fully-qualified form of `example.com` and resolves — measured — while mixed case also resolves. A comparison that does not normalise will treat these as different names, in whichever direction happens to be unhelpful.

The general statement, and the reason this deserves a per-deployment check rather than a payload list: **when a proxy sits in front, which header or which part of the request line counts is decided by the proxy's implementation.** A payload that does nothing on one deployment is a working payload one hop earlier.

### Why substring matching cannot work

The lab's rule is written out on the page — the host is allowed if it **contains** `vuln4all.local`, or `127.0.0.1`, or `localhost`. Every part of that is the wrong shape.

**A domain is read from the right.** In `vuln4all.local.evil.example`, the registered domain is `evil.example`; `vuln4all.local` is a label inside it. A substring check looks for the presence of a string and finds it.

**A substring relationship is not an identity relationship.** The same failure has appeared twice already in this guide, with different syntax and the same cause:

- **CORS**: an origin check written as `origin.endswith("example.com")` accepts `evilexample.com`.
- **SSRF**: a host allowlist written as a substring or a prefix check accepts `example.com.attacker.net`.
- **Host header**: the same check, on the same kind of value.

**The correct comparison is exact, or segment-aware:**

```python
def host_allowed(host: str, allowed: set[str]) -> bool:
    host = host.split(":")[0].rstrip(".").lower()      # normalise first
    return host in allowed or any(host.endswith("." + a) for a in allowed)
```

Three details in that snippet are each a measured failure mode: strip the **port** before comparing, strip the **trailing dot**, and lowercase — then compare either exactly or on a **label boundary**, which is what the leading `.` in `"." + a` enforces.

### The chain

Worth stating in full, because the severity is not obvious from the bug:

1. The attacker submits a password reset for a victim's address, with a forged host.
2. The application generates the link using that host and sends it **from its own domain** — which is what makes the mail trustworthy.
3. The victim receives a legitimate e-mail and clicks; the URL carries the real reset token and points at the attacker's server.
4. The token arrives in the attacker's logs; the victim's password is changed.

The same chain works for invitation links, e-mail confirmation, OAuth redirect handling and any "click this link we sent you" flow. **The link is genuine in every respect except where it points**, which is why users are not expected to notice.

### Detection and mitigation

- **Alert when the Host header is not one of the configured names.** This is the easiest detection in the guide, because the expected values are a short fixed list known at deploy time — the same property that makes the fix easy. Log the header, compare, and treat a mismatch as a security event rather than a routing error.
- **Alert on `X-Forwarded-Host`, `Forwarded` and `X-Forwarded-Server` arriving from outside.** These headers are meaningful only when a trusted proxy sets them. Their presence on a request that came directly from the internet is either a misconfigured proxy or an injection attempt, and both need attention.
- **Alert on shapes that never come from a real client.** A trailing dot, an embedded `@`, two `Host` headers, a host with a port the deployment does not use, and a host that differs from its own lowercase form. Any single one is unusual; several in one window is reconnaissance.
- **And detect at the outcome, which is the strongest signal.** A generated link, e-mail or redirect pointing at a domain the application does not own is the finding itself, and it is visible in the application's own output rather than inferred from a header.
- **For mitigation, generate absolute URLs from configuration.** A base URL setting, read once at startup and never from a request, removes the class — the same shape of fix as everywhere else in this guide: the value is used where it was produced, and the site's own name is produced by the deployment.
- **If the header must be consulted, validate it exactly and on a label boundary.** Normalise port, trailing dot and case, then require either an exact match or a suffix preceded by a dot. A substring or prefix comparison is not a weaker version of this check — it is a different check that answers a different question.
- **Never trust forwarded headers unless a trusted proxy strips and rewrites them.** The client can send `X-Forwarded-Host` on any request; the only reason it means anything is that the proxy in front overwrites it. If that is the design, the header has to be **removed** at the edge for requests that arrive with one, or the ordering assumption fails silently.
- **Keep the reset and confirmation flows on relative paths or a configured base.** Where the link is built inside a template, passing the configured base into the template is one line, and it makes the flow independent of any header.
- **And apply the same normalisation to CSRF and CORS origin checks.** The measured lesson — a substring check is not an identity check — is the same rule as the CORS entry's, and the two are commonly wrong in the same codebase.

<!-- lang:zh -->
### 一个"关于客户端"的头，被当成"关于服务端"的答案

HTTP/1.1 要求每个请求都带一个 `Host` 头，原因就是虚拟主机：好几个站点共用一个地址，所以客户端得说清它要哪个。因此这个头**是客户端对自己想要什么的一句陈述** —— 和路径、查询串属于同一类。

然后应用拿它回答一个关于自己的问题：**我自己叫什么？** 这个替换就是漏洞，而值得把它作为所违反的那条规则写出来：

> **一个站点对外呈现的地址是配置。它不是请求数据。**

后面的一切都是推论。口令重置链接、激活邮件、API 响应里的绝对 URL、OAuth 的重定向 URI、缓存键、CSRF 的 origin 校验，都是应用需要知道自己身份的地方 —— 而每一处，一旦用了来自 Host 头的值，就等于让请求方来选。

这是一个小 bug、后果却很大，所以值得理解而不是照着 payload 套：**一个伪造的头，能产生一封由真实应用发出的、真正的口令重置邮件，里面的链接指向攻击者的域名** —— 而受害者的 token 就在那个 URL 里。

### 四层，和每一种注入一样

1. **信任边界。** 应用为了决定自己叫什么，信任了客户端"这是哪个站点"的描述。两边行为都正确：客户端发了一个语法合法的 `Host`，框架忠实回报了它。
2. **数据与指令共用同一平面。** 同一个字段既是**路由指令**（"服务这里命名的虚拟主机"），又是**用来拼链接的数据**。没有任何东西区分这两个角色，于是为第一个角色选的值被用在了第二个上。
3. **为什么常见修法失败。** 靶场用一份白名单校验这个头 —— 而白名单用的是**子串匹配**。域名是**从右往左**读的，所以"包含"站点域名的一个名字，并不是站点的域名：

   ```
   Host: vuln4all.local.evil.example        子串检查放行
   -> 邮件里的链接指向  http://127.0.0.1.evil.example/reset?token=...
   ```

   那条生成出来的链接是实测的，它指向站外。白名单跑了，而它放行了错的值。
4. **变体。** `X-Forwarded-Host`、请求行里的绝对 URI、两个 `Host` 头、尾点、大小写混用、附加端口、内嵌的 `@`。

### 谁在用这个头

在找 payload 之前，值得知道一个技术栈里有哪些部分会看这个值 —— 因为这张清单比看上去长，而每一项都是一处会让人意外的地方：

| 使用方 | 它拿这个值做什么 |
|---|---|
| **链接生成** | 为邮件、重置、回调、邀请拼绝对 URL |
| **路由与虚拟主机** | 决定服务哪个站点 |
| **缓存键** | 有些缓存把 host 算进键里 —— Web 缓存投毒那篇建立在这上面 |
| **CSRF 与 CORS 的 origin 校验** | 判断请求是不是"来自我们自己" |
| **重定向** | 拼 `Location` 头与登录后的跳转目标 |
| **日志与审计** | 记录访问的是哪个站点 |

**一个链接生成点就够了。** 只要有任何一条代码路径用请求里的 host 拼绝对 URL，而那个 URL 会通过邮件到达用户，注入就成立 —— 无论栈的其余部分怎么对待这个头。

### 实际到达了什么，各栈又读成了什么

HTTP 本身对多数有意思的形状并不反对。在一个裸 socket 监听器上实测，下面每一个都原样到达：

| 请求 | 服务端收到的 |
|---|---|
| `Host: good.example` | 原样 |
| 两个 `Host` 头 | **两个**都在，有序 |
| `Host` 加 `X-Forwarded-Host` | **两个**都在 |
| `GET http://evil.example/ HTTP/1.1` 配 `Host: good.example` | 请求行里的绝对 URI **和** `Host` 头都在 |
| `Host: good.example.`（尾点） | 原样 |
| `Host: good.example:8080` | 原样 |
| `Host: good.example@evil.example` | 原样 |

所以这些形状并没有被阻止；它们全都是良构的 HTTP。不同的地方在于**某个栈决定哪一个才算数**，而实测有意思的正是这里。换成一个真实的 HTTP 栈：

- **`X-Forwarded-Host` 与 `Host` 都能被分别读到。** 一个优先用前者的应用 —— 部署在代理后面时的常见选择 —— 读的是一个任何客户端都能设的头，而靶场那个应用正是这么做的。
- **请求行里的绝对 URI 到达时表现为路径。** 实测的 `GET http://evil.example/ HTTP/1.1` 配 `Host: good.example` 产生了 `path='http://evil.example/'` 与 `Host='good.example'` —— 一个请求里有两个候选，而应用用哪一个，是框架的决定。
- **两个 `Host` 头没有唯一答案。** 有的栈取第一个，有的取最后一个，有的直接拒绝请求或者报一个空 host —— 靶场实测 Werkzeug 给的是空 host，而**空值也不等于拒绝**。
- **尾点能解析。** `example.com.` 是 `example.com` 的完全限定写法，实测能解析；大小写混用同样能解析。不做规范化的比较会把这些当成不同的名字 —— 而方向恰好是对自己不利的那一个。

一般性的说法，也是为什么这件事值得按部署逐个查、而不是背一份 payload 清单：**前面挂着代理时，哪个头、或者请求行的哪一段算数，是由那个代理的实现决定的。** 在一个部署上毫无作用的 payload，可能在前一跳就是有效的。

### 为什么子串匹配不可能成立

靶场那条规则就写在页面上 —— host **含有** `vuln4all.local`、或者 `127.0.0.1`、或者 `localhost` 就放行。它每一部分都是错的形状。

**域名是从右往左读的。** 在 `vuln4all.local.evil.example` 里，注册域是 `evil.example`；`vuln4all.local` 只是它里面的一个标签。子串检查找的是"某个字符串出现过没有"，而它找到了。

**子串关系不是同一关系。** 同一个失效在本指南里已经出现过两次，语法不同、成因一样：

- **CORS**：写成 `origin.endswith("example.com")` 的 origin 校验会接受 `evilexample.com`。
- **SSRF**：写成子串或前缀检查的 host 允许清单会接受 `example.com.attacker.net`。
- **Host 头**：同一类值上的同一种检查。

**正确的比较是精确的，或者按段感知的：**

```python
def host_allowed(host: str, allowed: set[str]) -> bool:
    host = host.split(":")[0].rstrip(".").lower()      # 先规范化
    return host in allowed or any(host.endswith("." + a) for a in allowed)
```

那段代码里有三个细节，每一个都对应一种实测过的失效模式：比较前先剥掉**端口**、剥掉**尾点**、转成小写 —— 然后要么精确比较，要么在**标签边界**上比较，而 `"." + a` 开头那个点就是在强制这件事。

### 那条链条

值得完整说一遍，因为这个 bug 的严重性并不显而易见：

1. 攻击者用伪造的 host，为受害者的地址提交一次口令重置。
2. 应用用那个 host 生成链接，并**从它自己的域名**发出 —— 而正是这一点让那封邮件可信。
3. 受害者收到一封正当的邮件并点击；URL 里带着真实的重置 token，而它指向攻击者的服务器。
4. token 到了攻击者的日志里；受害者的口令被改掉。

同一条链条对邀请链接、邮箱确认、OAuth 重定向处理、以及任何"点我们发给你的链接"的流程都成立。**那条链接在每一个方面都是真的，除了它指向哪里** —— 所以不能指望用户看出来。

### 检测与缓解

- **当 Host 头不是配置里的那些名字时告警。** 这是本指南里最容易的检测，因为期望值是一份部署时就知道的、很短的固定清单 —— 而正是这个性质让修法也容易。把这个头记下来、去比对，并把不匹配当作安全事件而不是路由错误。
- **对从外部到来的 `X-Forwarded-Host`、`Forwarded`、`X-Forwarded-Server` 告警。** 这些头只有在可信代理设置它们时才有意义。它们出现在一个直接从互联网来的请求上，要么是代理配错了，要么是一次注入尝试 —— 两者都需要处理。
- **对真实客户端绝不会发出的形状告警。** 尾点、内嵌 `@`、两个 `Host` 头、带着部署并不使用的端口、以及和它自己的小写形式不一致的 host。任何单独一条都不寻常；一个窗口里出现好几条就是侦察。
- **并在结果层检测，那是最强的信号。** 一条指向应用并不拥有的域名的生成链接、邮件或重定向，本身就是发现，而它就在应用自己的输出里，不必从请求头去推断。
- **缓解上，用配置生成绝对 URL。** 一个启动时读一次、永不来自请求的 base URL 设置，移除了这一类 —— 这和本指南其他每一处是同一种修法：值在它被产生的地方使用，而站点自己的名字由部署产生。
- **如果确实要读这个头，就精确校验、并在标签边界上校验。** 先规范化端口、尾点与大小写，然后要么精确匹配，要么要求一个前面带点的后缀。子串或前缀比较不是这条检查的弱化版 —— 它是另一条检查，回答的是另一个问题。
- **除非可信代理会剥离并重写转发头，否则绝不要信任它们。** 客户端可以在任何请求上发 `X-Forwarded-Host`，它之所以有意义的唯一原因是前面的代理覆盖了它。如果设计如此，就必须在边缘**移除**带着这个头进来的请求上的它，否则那个顺序假设会静默失效。
- **把重置与确认流程留在相对路径或配置的基址上。** 当链接是在模板里拼的时候，把配置的基址传进模板只是一行，而它让这个流程不依赖任何请求头。
- **并且把同样的规范化用到 CSRF 与 CORS 的 origin 校验上。** 实测得到的教训 —— 子串检查不是同一性检查 —— 与 CORS 那篇是同一条规则，而这两处在同一个代码库里一起写错是常态。
