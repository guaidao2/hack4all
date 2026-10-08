---
id: csrf-fundamentals
title_en: "CSRF, Part 1 — A Credential Is Not an Intent"
title_zh: "CSRF（一）：凭据不是意图"
summary_en: A cross-site request arrives with valid credentials that the browser attached automatically, so the server sees authenticity where the user expressed no intent. Measured with a text/plain form that carries valid JSON, and against a SameSite boundary that is wider than origin.
summary_zh: 一个跨站请求带着浏览器自动附上的有效凭据到达，于是服务端在"用户并没有表达意图"的地方看到了真实性。这一篇用一张能装下合法 JSON 的 text/plain 表单实测，也量了一条比 origin 宽的 SameSite 边界。
tags: [web, csrf, cwe-352, cookies, samesite]
tools: [curl, python3, browser devtools]
attck: [T1185, T1550]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The one thing the server cannot tell apart

Every entry in this series is about a component that cannot distinguish two things. Here the two things are **"the user did this"** and **"the user's browser did this"**, and the mechanism that makes them indistinguishable is a browser feature:

> **Cookies are attached to a request by the browser, based on the destination, not on who caused the request.**

So a request that a user initiated from your site and a request that a page on another site caused the browser to send arrive looking the same: same cookies, same method, same path, valid session. Measured against a lab whose password-change handler checks only the session:

```
POST /change-password   with the victim's cookie and Origin: http://evil.example
-> 200, and the new password works
```

The server did nothing wrong. It required a valid session and it got one. What it also required — without asking — was that the session's owner intended this.

**The precise statement is that a credential was treated as an intent.** A cookie answers "who is this?"; the application used it to answer "did they mean to do this?", and those are different questions with different answers.

### What CSRF is not

Two misreadings cause most of the bad defences, and both are worth clearing first.

**It is not about reading the response.** The attacker does not need to see anything: the request does its work, and the effect is visible to the victim afterwards. That is why **"the attacker cannot read the response" is not a defence** — it is a property of cross-origin reads, and it leaves the write fully available. The CORS entry covers the read side; this one covers the write side, and confusing them produces defences that address the wrong half.

**It is not limited to forms.** Anything a browser will send to another origin with cookies qualifies: a form, an image, a `fetch` in a "simple request" configuration, a top-level navigation. The mechanisms differ in what they can express, not in whether they work.

### Why "the endpoint only accepts JSON" is not a defence

This is the most common modern misconception, and it rests on a true fact used to draw a false conclusion.

The true fact: an HTML form's `enctype` can only be one of three values, and `application/json` is not among them.

| `enctype` | Encoding |
|---|---|
| `application/x-www-form-urlencoded` | The default; key-value pairs |
| `multipart/form-data` | Used when files are involved |
| `text/plain` | Rarely considered, and **it escapes nothing** |

The false conclusion: therefore a JSON-only endpoint cannot be reached by a cross-site form.

The reason it fails is the third row. With `enctype="text/plain"` the body is produced by concatenating **`name` + `=` + `value`** with no escaping at all, so the attacker can split a JSON document across the two attributes:

```html
<form action="https://app.example/api/email" method="POST" enctype="text/plain">
  <input name='{"email":"attacker@evil.example","ignore":"' value='"}'>
</form>
<script>document.forms[0].submit()</script>
```

Measured, that produces:

```
{"email":"attacker@evil.example","ignore":"="}
```

which is **valid JSON** — the leftover `=` became the value of an extra field the attacker invented for exactly that purpose. And `text/plain` is a **simple request**, so the browser sends it without a preflight and without asking anything of the server first.

Whether it lands comes down to how the server decides what a request is:

| Implementation | Outcome |
|---|---|
| Requires `Content-Type: application/json` and parses only then | refused |
| Does not check the header, calls the JSON parser on the body | **accepted** |
| Strips a trailing `=` from a `text/plain` body before parsing | **accepted** |

**"A form cannot send this content type" and "the server will not parse it" are two different statements**, and only the first is true by construction. The second is an implementation detail that the measured table shows going both ways.

### SameSite, and the width of "same site"

`SameSite` is genuinely useful and it is not a complete defence, for a reason that is easy to get wrong: **the unit it works in is the *site*, and a site is broader than an origin.**

Measured, on the question of whether two URLs are the same site:

| A | B | Same site |
|---|---|---|
| `https://app.example` | `https://app.example` | yes |
| `https://app.example` | `https://evil.example` | no |
| `http://app.example` | `https://app.example` | **yes** — scheme does not participate |
| `https://app.example:443` | `https://app.example:8443` | **yes** — port does not participate |
| `https://a.example` | `https://b.example` | **yes** — subdomains of one registrable domain |

Two consequences that matter in practice:

**`SameSite` cannot separate two applications that share a registrable domain.** A lab whose "victim site" and "attacker site" live on the same host is not cross-site by this definition, which is why a `Lax` cookie is no obstacle there — and why a real deployment with a customer-facing app and an admin app on the same domain should not treat `SameSite` as the boundary between them.

**And `Lax` has an exception that keeps a whole class of bugs alive.** The default behaviour permits cookies on **top-level GET navigations** across sites — a link, a `window.location` assignment — so any endpoint that changes state in response to `GET` remains exploitable under `Lax`. That is the same rule as the HTTP fundamentals entry, arriving from the other side: **a state-changing GET is a bug whether or not anyone races it.**

### Four layers

1. **The trust boundary.** The server trusts that a valid credential means the credential's owner acted. The browser, doing its documented job, attached the credential without asking who caused the request.
2. **Data and instruction share a plane.** The cookie is **identity**; the request is **intent**; the server reads the first as the second. Nothing in the request distinguishes them, because the mechanism that carries identity is not the mechanism that would carry intent.
3. **Why the usual fixes fail.** A `Referer` check alone is stripped by privacy settings and policies; `SameSite` alone operates at a coarser unit than the origin and has the `Lax` navigation exception; "the endpoint takes JSON" alone relies on a header the form does not control being checked by code that may not check it. Each addresses a proxy for intent rather than intent itself.
4. **The variants.** A `text/plain` form carrying JSON, a top-level `GET` under `Lax`, two applications on one registrable domain, and a permissive CORS policy that removes the read restriction as well.

### Detection and mitigation

- **Alert on state-changing requests whose `Origin` or `Referer` is not your own site.** Modern browsers send `Origin` on cross-origin `POST`s; a write with no `Origin` at all is at least as interesting as one with a foreign value, and the expected values are a short fixed list.
- **Alert on sensitive operations that change the account's own authentication material.** A password or e-mail change is the highest-value CSRF target, because the attacker does not need to read anything — the change itself is the win. These are few enough to monitor individually.
- **And record the origin alongside the operation in the audit log.** Investigating a CSRF incident after the fact needs to answer "where did this request come from", and that field is only useful if it was captured at the time.
- **For mitigation, make intent a value the attacker cannot guess.** A synchroniser token stored in the session and required in the request is the standard mechanism, and it works because the attacker's page cannot read it — which is also why **an XSS on the same origin defeats it**, and why the two findings should be considered together rather than separately.
- **Bind the token to the session, and consider making it per-request.** A token that is not tied to the session is a token another session can use; a token that rotates on each use narrows the window in which a leaked one is useful.
- **Check `Origin` as a second, independent control.** It is set by the browser rather than by the page, it is present on the requests that matter, and a mismatch is a refusal. It costs one comparison and it does not depend on the application having remembered to issue a token.
- **Use `SameSite` as depth, not as the wall.** `Lax` or `Strict` on session cookies raises the cost of the generic attack, and the measured table shows why it cannot be relied on to separate two applications sharing a domain.
- **Never change state on `GET`.** This removes the `Lax` navigation exception entirely, and it is a rule with no downside — the HTTP fundamentals entry makes the same point from the protocol side.
- **Re-authenticate for the operations that matter most.** Requiring the current password for a password change, or a second factor for an e-mail change, defeats CSRF against those endpoints completely, because **the attacker's page cannot supply something only the user knows** — it is the one control that does not depend on the request's provenance.
- **And keep CORS policy tight, since a permissive one removes even the read barrier.** The two entries are two halves of one boundary: this one is about a request being **made**, and the CORS entry is about the response being **read**. A policy that allows both turns a one-way forgery into a full interactive session.

<!-- lang:zh -->
### 服务端唯一分不开的那两件事

本系列每一篇都关于"某个组件分不开两样东西"。在这里，那两样是**"用户做了这个"**与**"用户的浏览器做了这个"**，而让它们无法区分的机制是一个浏览器特性：

> **cookie 是浏览器按目的地附到请求上的，而不是按"谁导致了这次请求"。**

所以一个用户从你的站点发起的请求，和一个别站页面让浏览器发出的请求，到达时看起来一样：同样的 cookie、同样的方法、同样的路径、有效的会话。对一个"改密码处理器只检查会话"的靶场实测：

```
POST /change-password   带着受害者的 cookie 与 Origin: http://evil.example
-> 200，而且新密码真的能用
```

服务端什么都没做错。它要求了有效会话，也拿到了一个。它同时要求了 —— 却没有问 —— 的是：那个会话的主人有意做这件事。

**准确的说法是：一个凭据被当成了意图。** cookie 回答"这是谁"；应用拿它去回答"这是不是他想做的"，而那是两个问题、两个答案。

### CSRF 不是什么

有两种误读造成了大多数糟糕的防御，两者都值得先排除。

**它和"读响应"无关。** 攻击者不需要看到任何东西：请求自己完成了它的作用，受害者在之后看到效果。这就是为什么**"攻击者读不到响应"不是一种防御** —— 那是跨源读取的性质，而它让写入完全可用。CORS 那篇讲的是读的那一侧；这一篇讲写的那一侧，把两者混起来就会做出修错了那一半的防御。

**它也不限于表单。** 任何浏览器会带 cookie 发往另一个源的东西都算：一个表单、一张图片、一个"简单请求"配置下的 `fetch`、一次顶层导航。这些机制的区别在于**能表达什么**，而不在于**能不能用**。

### 为什么"这个接口只收 JSON"不是一种防御

这是现代最常见的误解，它建立在一个真事实之上，却推出了一个假结论。

真事实：HTML 表单的 `enctype` 只能是三个值之一，而 `application/json` 不在其中。

| `enctype` | 编码 |
|---|---|
| `application/x-www-form-urlencoded` | 默认；键值对 |
| `multipart/form-data` | 有文件时用 |
| `text/plain` | 很少有人考虑到，而**它什么都不转义** |

假结论：所以一个只收 JSON 的接口无法被跨站表单够到。

它失败的原因就是第三行。用 `enctype="text/plain"` 时，请求体是 **`name` + `=` + `value`** 直接拼出来的、不做任何转义，于是攻击者可以把一份 JSON 文档切开、分到两个属性里：

```html
<form action="https://app.example/api/email" method="POST" enctype="text/plain">
  <input name='{"email":"attacker@evil.example","ignore":"' value='"}'>
</form>
<script>document.forms[0].submit()</script>
```

实测，它产生：

```
{"email":"attacker@evil.example","ignore":"="}
```

那是一段**合法 JSON** —— 多出来的那个 `=` 成了攻击者专门为它发明的另一个字段的值。而 `text/plain` 属于**简单请求**，所以浏览器不发预检、也不先问服务端任何东西，直接把它发出去。

它能不能落地，取决于服务端怎么判断"一个请求是什么"：

| 实现 | 结果 |
|---|---|
| 要求 `Content-Type: application/json`，是才解析 | 拒绝 |
| 不检查这个头，直接对请求体调 JSON 解析器 | **接受** |
| 对 `text/plain` 的请求体容错地剥掉尾部 `=` 再解析 | **接受** |

**"表单发不出这个 Content-Type"和"服务端不会解析它"是两句不同的话**，而只有第一句是构造上成立的。第二句是一个实现细节，上面那张实测表显示它可以朝两个方向走。

### SameSite，以及"同站"有多宽

`SameSite` 确实有用，而它不是完整防御，原因很容易弄错：**它作用的单位是 *site*，而 site 比 origin 宽。**

实测，判断两个 URL 是不是同一个 site：

| A | B | 同 site |
|---|---|---|
| `https://app.example` | `https://app.example` | 是 |
| `https://app.example` | `https://evil.example` | 否 |
| `http://app.example` | `https://app.example` | **是** —— scheme 不参与判定 |
| `https://app.example:443` | `https://app.example:8443` | **是** —— 端口不参与判定 |
| `https://a.example` | `https://b.example` | **是** —— 同一个可注册域下的子域 |

两个在实践中要紧的后果：

**`SameSite` 无法分隔共用同一个可注册域的两个应用。** 一个"受害者站点"与"攻击者站点"在同一台主机上的靶场，按这个定义就不是跨站 —— 所以 `Lax` 的 cookie 在那里毫无阻碍；也所以一个真实部署里，同一域名下面向客户的应用与管理应用，不该把 `SameSite` 当作它们之间的边界。

**而 `Lax` 有一个例外，让整整一类 bug 继续活着。** 它的默认行为允许 cookie 在**跨站的顶层 GET 导航**上被带上 —— 一个链接、一次 `window.location` 赋值 —— 所以任何"用 GET 响应来改状态"的接口在 `Lax` 下仍然可被利用。这和 HTTP 基础那篇是同一条规则，只是从另一侧到达：**一个会改状态的 GET 本身就是 bug，与有没有人去竞态它无关。**

### 四层

1. **信任边界。** 服务端信任"有效凭据意味着凭据的主人做了这件事"。而浏览器在做它被文档规定要做的事：附上凭据，不问是谁导致了这次请求。
2. **数据与指令共用同一平面。** cookie 是**身份**；请求是**意图**；服务端把第一个读成了第二个。请求里没有任何东西区分它们，因为承载身份的那个机制，不是承载意图的那个机制。
3. **为什么常见修法失败。** 只查 `Referer`，会被隐私设置与策略剥离；只靠 `SameSite`，它的作用单位比 origin 粗、还有 `Lax` 那个导航例外；只靠"接口收 JSON"，依赖的是一个表单控制不了的头、被一段可能不检查它的代码检查。每一个针对的都是意图的替代品，而不是意图本身。
4. **变体。** 一张携带 JSON 的 `text/plain` 表单、`Lax` 下的顶层 `GET`、同一个可注册域上的两个应用、以及一个松开到连"读不到响应"都没了的 CORS 策略。

### 检测与缓解

- **对"`Origin` 或 `Referer` 不是本站"的改状态请求告警。** 现代浏览器在跨源 `POST` 上会发 `Origin`；一个**完全没有** `Origin` 的写操作，至少和一个带着外来源的一样值得看，而期望值是一份很短的固定清单。
- **对"改动账号自身认证材料的敏感操作"告警。** 改密码或改邮箱是 CSRF 里价值最高的目标，因为攻击者不需要读任何东西 —— 改动本身就是胜利。这类操作少到可以逐个盯着。
- **并且在审计日志里把来源与操作记在一起。** 事后调查一次 CSRF 需要回答"这个请求从哪里来"，而那个字段只有在当时被记下来才有用。
- **缓解上，让"意图"成为一个攻击者猜不到的值。** 会话里存一个同步器 token、请求里必须带上，是标准机制；它有效是因为攻击者的页面读不到它 —— 而这也正是**同源上的一个 XSS 会让它失效**的原因，也是这两条发现应当被一起考虑而不是分开的原因。
- **把 token 绑定到会话，并考虑按请求轮换。** 一个不与会话绑定的 token 是另一个会话也能用的 token；一个每次使用都轮换的 token 收窄了"一旦泄露还有多大用处"的窗口。
- **把 `Origin` 检查作为一项独立的第二控制。** 它由浏览器设置而不是由页面设置，它在要紧的那些请求上都在，而一次不匹配就是一次拒绝。它只花一次比较，而且不依赖"应用记得发过 token"。
- **把 `SameSite` 当纵深，而不是当那堵墙。** 在会话 cookie 上用 `Lax` 或 `Strict` 提高了通用攻击的成本，而上面那张实测表说明了为什么不能靠它来分隔共用域名的两个应用。
- **绝不在 `GET` 上改状态。** 这完全移除了 `Lax` 那个导航例外，而且它是一条没有代价的规则 —— HTTP 基础那篇从协议那一侧说过同一件事。
- **对最要紧的那几个操作要求重新认证。** 改密码要求当前密码、改邮箱要求第二因素，能对那几个接口完全击败 CSRF，因为**攻击者的页面无法提供只有用户本人知道的东西** —— 它是唯一一项不依赖"请求从哪来"的控制。
- **并且把 CORS 策略收紧，因为一个宽松的策略连那道读的屏障也拿掉了。** 这两篇是同一个边界的两半：这一篇讲请求**被发出**，CORS 那篇讲响应**被读取**。一个把两者都放开的策略，把一次单向的伪造变成了完整的交互式会话。
