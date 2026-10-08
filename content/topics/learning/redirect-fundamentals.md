---
id: redirect-fundamentals
title_en: "Open Redirect — The Destination Is Executed by the Client"
title_zh: "开放重定向：跳转目标由客户端执行"
summary_en: Two rules that look like site checks each leave a hole, and the reason is that the server only writes a header while the browser performs the redirect. Measured across four validation styles and twelve targets, including the cases where the two interpreters disagree.
summary_zh: 两条看起来像"站内检查"的规则各留了一个洞，原因在于服务端只是写了一个头，而真正执行跳转的是浏览器。这一篇量了四种校验实现对着十二个目标，包括两层解释不一致的那些情形。
tags: [web, open-redirect, cwe-601, url-parsing, phishing]
tools: [curl, python3, browser devtools]
attck: [T1190, T1566]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Where the decision actually happens

A redirect endpoint has a shape everyone recognises — `?next=`, `?return=`, `?redirect_url=` — and a defence that seems obvious: check whether the target is local before using it. Measured on a lab whose handler applies two such checks, both were bypassed, and the reason is worth stating before the payloads:

> **The server writes a `Location` header. The browser is what travels.**

That single sentence changes what a fix means. The question is not "is my validation strict enough" but **"which strings will reach me, and what will the client do with each of them"** — because a client normalises malformed URLs, and different clients normalise them differently.

### Two rules, two holes

The lab's handler applies exactly the two rules most people would write first.

**Rule one: a value beginning with `/` is a local path.** Measured against it:

```
//evil.example/          starts with /  -> treated as local
```

But `//host/path` is a **protocol-relative URL**: the browser supplies the scheme from the current page and navigates to that host. **What begins with `//` is not a path; it is a host with the scheme omitted** — and a check written as `startswith("/")` sees only the slash.

**Rule two: a value mentioning our domain is local.** Measured against it:

```
https://vuln4all.local.evil.example/     mentions the domain -> treated as local
```

Domains are read **right to left**, so the registrable domain here is `evil.example`; `vuln4all.local` is a label inside it. This is the seventh place in this guide where a substring comparison against a hostname has failed — host header, `redirect_uri`, CORS allowlists, and now a redirect target, all the same mistake.

### What the four styles admit

Measured, against twelve targets:

| Target | Prefix | Substring | Host only | Strict allowlist |
|---|---|---|---|---|
| `/dashboard` (legitimate) | allow | allow | allow | allow |
| `https://evil.example/` | deny | deny | deny | deny |
| **`//evil.example/`** | **allow** | **allow** | deny | deny |
| **`https:evil.example`** | deny | deny | **allow** | deny |
| **`/\evil.example`** | **allow** | **allow** | **allow** | deny |
| **`/%2f%2fevil.example`** | **allow** | **allow** | **allow** | **allow** |
| **`/%252f%252fevil.example`** | **allow** | **allow** | **allow** | **allow** |
| `https://app.example@evil.example/` | **allow** | **allow** | deny | deny |
| `https://app.example.evil.example/` | **allow** | **allow** | deny | deny |
| `https://evil.example/?x=https://app.example` | deny | **allow** | deny | deny |
| `/\t/evil.example` | allow | allow | deny | **allow** |
| `/dashboard/https://evil.example` | allow | allow | allow | deny |

Prefix admitted eight, substring nine, host-only six. The strict allowlist — beginning with `/`, not beginning with `//` or `/\`, containing no `://` and no `@` — admitted four, and **the four it admitted are the interesting part**: `/%2f%2fevil.example` passes every text check because `%2f` is not the character `/`, and it becomes `//evil.example` for whichever layer decodes it.

**That is the encoding-layer problem from the OAuth entry, in a redirect parameter.** The check runs on the encoded string; the navigation runs on the decoded one.

### The two interpreters

The same table, read as "what would a browser do":

| Target | Parsed scheme | Parsed host |
|---|---|---|
| `/dashboard` | *(none)* | *(none)* — local |
| `//evil.example/` | *(none)* | **evil.example** |
| `https:evil.example` | https | *(none)* — some parsers treat it as relative |
| `/\evil.example` | *(none)* | *(none)* — but the browser normalises `\` to `/`, giving `//evil.example` |
| `https://app.example@evil.example/` | https | **evil.example** — `app.example` is userinfo |
| `https://app.example.evil.example/` | https | **app.example.evil.example** |
| `/\t/evil.example` | *(none)* | **evil.example** — after whitespace stripping |

**The server's parser and the browser's parser are two implementations**, and the security decision is made by the first while the effect is produced by the second. This is the OAuth entry's layer problem, the smuggling entry's boundary problem and the cache entry's normalisation problem, arriving at a `Location` header.

### Why "low severity" is the wrong label

An open redirect does not read data and does not execute code, which is why it is so often accepted as a minor finding. Two things make that label misleading.

**It is a phishing amplifier.** A victim who receives `https://your-domain/login?next=//evil.example` sees **your domain** in the link, and arrives at the attacker's site after logging in. The domain in the address bar is the entire basis on which users decide whether a link is trustworthy, and this hands it over without touching your infrastructure.

**And it is usually one step in a chain.** The places it has already appeared in this guide:

| Chain | What the redirect contributes |
|---|---|
| **OAuth** | A registered `redirect_uri` that the client also uses as a redirect target turns a matched destination into an arbitrary one — the code goes to the attacker (two entries ago) |
| **SSRF** | An allowlisted host that has an open redirect is a host you can reach **through** — the allowlist is satisfied and the request continues somewhere else |
| **Session tokens in URLs** | A `next=` value containing a token reaches the `Referer` of the destination page, which is the session entry's leak |

In each case the redirect is not the vulnerability; it is the mechanism that converts **a trusted name into an arbitrary destination**, and the other finding supplies the payload.

### Detection and mitigation

- **Alert when a `Location` header points outside the deployment's own hosts.** The expected values are a short list known at deploy time, so this is a comparison rather than a heuristic, and it catches the outcome regardless of which bypass produced it.
- **Alert on the shapes no legitimate client sends.** `//`, `/\`, `https:/`, an `@` before the host, an encoded slash or a leading whitespace character in a redirect parameter. Any one is a probe, and several in a window is someone working through a list.
- **And alert on the parameter on which it happens.** A redirect parameter that starts accepting external values after a release is a regression worth seeing on its own.
- **For mitigation, do not accept a URL at all.** Take a **key** and look the destination up in a table — `?next=settings` resolving to a path the application chose. This removes the class rather than filtering it.
- **Where a URL must be accepted, allow only a relative path and validate the shape rather than the content.** A pattern such as a leading `/` followed by unreserved path characters, with `//`, `/\`, `@` and any percent-encoding of a slash refused, is small enough to reason about — and the measured table shows why "contains no `://`" is not enough on its own.
- **Do not build a denylist of bypasses.** The client normalises malformed URLs and does it differently per client, so the list of shapes is open-ended while the list of shapes a legitimate relative path can take is not. **Enumerate what is allowed; the disallowed set is the complement and it is infinite.**
- **And ask the question in the direction the client asks it.** The useful review question is not "does this string point outside" but **"which of these strings will reach this code, and where will the browser end up"** — which means testing with the odd shapes rather than with a well-formed attack URL.

<!-- lang:zh -->
### 决定到底在哪里发生

一个跳转接口有一个人人都认得的样子 —— `?next=`、`?return=`、`?redirect_url=` —— 以及一个看起来显然的防御：用之前检查一下目标是不是站内。在一个应用了两条这种检查的靶场上实测，**两条都被绕过了**，而原因值得写在 payload 之前：

> **服务端写的是一个 `Location` 头。真正走过去的是浏览器。**

这一句话改变了"修法"的含义。问题不是"我的校验够不够严"，而是 **"哪些字符串会走到我这儿，而客户端会拿它们各自做什么"** —— 因为客户端会纠正畸形 URL，而不同的客户端纠正方式不同。

### 两条规则，两个洞

靶场那个处理器应用的正是一般人最先会写的两条规则。

**规则一：以 `/` 开头的值是站内路径。** 对着它实测：

```
//evil.example/          以 / 开头  -> 被当成站内
```

但 `//host/path` 是一个**协议相对 URL**：浏览器用当前页面的协议补上，然后导航到那台主机。**以 `//` 开头的不是一个路径，是一个省略了协议的主机** —— 而写成 `startswith("/")` 的检查只看到了那个斜杠。

**规则二：提到我们域名的值是站内。** 对着它实测：

```
https://vuln4all.local.evil.example/     提到了域名  -> 被当成站内
```

域名是**从右往左**读的，所以这里的注册域是 `evil.example`；`vuln4all.local` 只是它里面的一个标签。这是这份指南里**第七处**"用子串比较主机名"失败的地方 —— Host 头、`redirect_uri`、CORS 允许清单，现在是跳转目标，全都是同一个错误。

### 那四种实现各自放行了什么

实测，对着十二个目标：

| 目标 | 前缀 | 子串 | 只比 host | 严格白名单 |
|---|---|---|---|---|
| `/dashboard`（正当） | 放行 | 放行 | 放行 | 放行 |
| `https://evil.example/` | 拒绝 | 拒绝 | 拒绝 | 拒绝 |
| **`//evil.example/`** | **放行** | **放行** | 拒绝 | 拒绝 |
| **`https:evil.example`** | 拒绝 | 拒绝 | **放行** | 拒绝 |
| **`/\evil.example`** | **放行** | **放行** | **放行** | 拒绝 |
| **`/%2f%2fevil.example`** | **放行** | **放行** | **放行** | **放行** |
| **`/%252f%252fevil.example`** | **放行** | **放行** | **放行** | **放行** |
| `https://app.example@evil.example/` | **放行** | **放行** | 拒绝 | 拒绝 |
| `https://app.example.evil.example/` | **放行** | **放行** | 拒绝 | 拒绝 |
| `https://evil.example/?x=https://app.example` | 拒绝 | **放行** | 拒绝 | 拒绝 |
| `/\t/evil.example` | 放行 | 放行 | 拒绝 | **放行** |
| `/dashboard/https://evil.example` | 放行 | 放行 | 放行 | 拒绝 |

前缀放行八个、子串九个、只比 host 六个。那份严格白名单 —— 以 `/` 开头、不以 `//` 或 `/\` 开头、不含 `://` 也不含 `@` —— 放行了四个，而**它放行的那四个才是有意思的部分**：`/%2f%2fevil.example` 通过了每一项文本检查，因为 `%2f` 不是字符 `/`，而对任何解码它的那一层来说它都会变成 `//evil.example`。

**那就是 OAuth 那篇的编码层问题，出现在一个跳转参数里。** 检查跑在编码后的字符串上，导航跑在解码后的字符串上。

### 两个解释器

同一张表，换成"浏览器会怎么做"来读：

| 目标 | 解析出的 scheme | 解析出的主机 |
|---|---|---|
| `/dashboard` | *（无）* | *（无）* —— 站内 |
| `//evil.example/` | *（无）* | **evil.example** |
| `https:evil.example` | https | *（无）* —— 有些解析器当成相对路径 |
| `/\evil.example` | *（无）* | *（无）* —— 但浏览器在 URL 里把 `\` 规范化成 `/`，于是得到 `//evil.example` |
| `https://app.example@evil.example/` | https | **evil.example** —— `app.example` 是 userinfo |
| `https://app.example.evil.example/` | https | **app.example.evil.example** |
| `/\t/evil.example` | *（无）* | **evil.example** —— 空白被剥掉之后 |

**服务端的解析器与浏览器的解析器是两个实现**，而安全决定由前者做出、效果由后者产生。这就是 OAuth 那篇的层问题、走私那篇的边界问题、缓存那篇的规范化问题，一起到达了一个 `Location` 头。

### 为什么"低危"这个标签是错的

开放重定向不读数据、也不执行代码，这就是它经常被当成一条轻微发现接受下来的原因。有两件事让那个标签具有误导性。

**它是钓鱼的放大器。** 一个收到 `https://你的域名/login?next=//evil.example` 的受害者，在链接里看到的是**你的域名**，而登录之后落到攻击者的站点上。地址栏里的域名，是用户判断一条链接是否可信的全部依据，而这个洞在不碰你任何基础设施的前提下把它交了出去。

**而且它通常是链条里的一步。** 它已经在这份指南里出现过的地方：

| 链条 | 跳转提供了什么 |
|---|---|
| **OAuth** | 一个被客户端同时用作跳转目标的已注册 `redirect_uri`，会把"匹配上的目的地"变成任意目的地 —— 授权码到了攻击者手里（前面两篇） |
| **SSRF** | 一个在允许清单里、又有开放重定向的主机，是一个你能**穿过**它的主机 —— 允许清单被满足了，而请求继续去了别处 |
| **URL 里的会话令牌** | 一个含 token 的 `next=` 值会到达目标页面的 `Referer`，那就是会话那篇里的泄漏 |

每一种情况下，重定向都不是那个漏洞；它是**把"一个受信的名字"变成"任意目的地"**的机制，而另一条发现提供了 payload。

### 检测与缓解

- **当 `Location` 头指向部署自有主机之外时告警。** 期望值是一份部署时就知道的短清单，所以这是一次比较而不是启发式，而无论哪一种绕过造成了它，它都能抓到**结果**。
- **对"没有正当客户端会发的形状"告警。** 跳转参数里的 `//`、`/\`、`https:/`、主机前面的 `@`、被编码的斜杠、或者一个前导空白字符。任何单独一个都是探测，而一个窗口里出现几个，就是有人在逐条试一份清单。
- **并且对"发生这件事的那个参数"告警。** 一个跳转参数在某次发布之后开始接受外部值，那本身就是一条值得看见的回归。
- **缓解上，根本不要接受 URL。** 收一个**键**，去一张表里查目的地 —— `?next=settings` 解析到应用自己选定的一个路径。这移除的是这一类，而不是过滤它。
- **确实必须接受 URL 的地方，只允许相对路径，并且校验形状而不是内容。** 一个"以 `/` 开头、后接不带保留含义的路径字符、并拒绝 `//`、`/\`、`@` 以及斜杠的任何百分号编码"的模式，小到可以推理清楚 —— 而上面那张实测表说明了为什么"不含 `://`"这一条单独不够。
- **不要建一张绕过的黑名单。** 客户端会纠正畸形 URL，而且每个客户端做得不一样，所以形状的清单是开放的，而一个正当相对路径能取的形状清单不是。**把允许的东西枚举出来；不允许的集合是它的补集，而那是无限的。**
- **并且按客户端提问的方向去问那个问题。** 有用的评审问题不是"这个字符串是不是指向外面"，而是 **"这些字符串里哪些会走到这段代码，浏览器最后会落到哪里"** —— 这意味着要用那些奇怪的形状去测，而不是用一个规规矩矩的攻击 URL 去测。
