---
id: cors-fundamentals
title_en: "CORS, Part 1 — The Same-Origin Policy Restricts Reading"
title_zh: "CORS（一）：同源策略限制的是读"
summary_en: Cross-origin requests have always been sendable; what the same-origin policy restricts is reading the response, which makes this the other half of the CSRF entry. Measured with an application that reflects Origin — every origin including null reads the private response, while a wildcard cannot.
summary_zh: 跨源请求一直都是发得出去的；同源策略限制的是"能不能读到响应"，这让它成为 CSRF 那篇的另一半。这一篇在一个反射 Origin 的应用上实测 —— 每一个源、包括 null，都能读到私密响应，而通配符反而不能。
tags: [web, cors, cwe-942, same-origin-policy, browser]
tools: [curl, python3, browser devtools]
attck: [T1190, T1539]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The half of the boundary CSRF is not about

The CSRF entry covered **writing**: a page on another origin can cause a state change, because the browser attaches credentials to requests by destination and the response is never read. This entry covers **reading**, and the two are separated by one sentence:

> **Cross-origin requests have always been sendable. What the same-origin policy restricts is whether the response can be read.**

That distinction is what makes CORS easy to reason about once it is stated:

| | Writing (CSRF) | Reading (CORS) |
|---|---|---|
| Needs the attacker to see a response | no | **yes** |
| Affected by CORS configuration | **no** | yes |
| Stopped by a CSRF token | yes | no |

Two consequences follow, and both are frequently got wrong:

**A CORS misconfiguration does not create CSRF.** CSRF needs no CORS header, no preflight and no cooperation from the response — it is a request being sent with the victim's credentials. A permissive CORS policy cannot make that worse.

**What a misconfiguration does is worse in a different way.** It upgrades an attacker who could *cause* an action into one who can *read the result* — and reading is what turns a blind forgery into exfiltration. Measured, against an application whose private endpoint returns an account's data:

```
Origin: https://evil.example
response: Access-Control-Allow-Origin: https://evil.example
          Access-Control-Allow-Credentials: true
browser decides: readable
attacker reads:  {"user":"alice","email":"alice@corp.example",
                  "csrf_token":"T0KEN-abc123","balance":1234.56}
```

The attacker's page did not need to break anything. It asked, the application said yes, and the browser — doing exactly what it was told — handed the response to a script that should never have seen it.

### Four layers

1. **The trust boundary.** The browser trusts the response's `Access-Control-Allow-*` headers as the server's statement about who may read it. The server, in turn, has to decide that **from a value the client supplied** — which is the host header entry's shape in a new place.
2. **Data and instruction share a plane.** The `Origin` header is simultaneously **a statement of who is asking** and **the input from which the server decides what to allow**. A server that echoes it has converted a description into a permission.
3. **Why the usual fix fails.** "We added CORS headers" sounds like "we configured a policy". It is not, if the value of `Access-Control-Allow-Origin` is taken from the request — **echoing the asker's name is not a policy, it is an absence of one**.
4. **The variants.** `null` as an origin, matching by prefix, suffix, substring or an unescaped regular expression, comparing hosts while forgetting the scheme, and trusting every subdomain of a registrable domain.

### What CORS is, and what the preflight is for

**CORS does not enable cross-origin requests.** It enables cross-origin **reads**. The request was going to be sent either way; the headers decide whether the page that sent it gets to see the answer.

**The preflight is the browser asking on its own behalf.** For a request that is not "simple" — a custom header, a method other than `GET`/`POST`/`HEAD`, a JSON content type — the browser first sends `OPTIONS` and checks whether the response permits what it is about to do. Only then does it send the real request.

That mechanism looks like access control and is not one, for a reason that matters here:

> **The preflight only applies to non-simple requests, and everything CSRF needs is a simple request.**

A form, an image, a `GET`, a `POST` with `text/plain` — the previous entry measured that last one being used to carry a JSON body — are all simple, so no preflight is sent and no permission is asked. The endpoint receives the request regardless of its CORS configuration. **CORS is not a CSRF defence, and adding a restrictive policy does not make one.**

### The two headers, and why both are needed

The permission is a combination, not a single value:

| Header | What it permits |
|---|---|
| `Access-Control-Allow-Origin` | **Which origin** may read the response |
| `Access-Control-Allow-Credentials` | Whether the read may include **credentials** |

Measured, with a carefully written policy, against the same private endpoint:

| Request origin | `Access-Control-Allow-Origin` | Browser |
|---|---|---|
| `https://app.example` | `https://app.example` | **readable** |
| `https://evil.example` | *(absent)* | not readable |
| `null` | *(absent)* | not readable |

And with the origin reflected, all three were readable, `null` included. A policy that is correct produces a **fixed** value in that header; a policy that reflects produces a value that changes with the request — which is the observation that detects the misconfiguration.

**Neither header is sufficient alone.** An `Allow-Origin` naming the attacker without `Allow-Credentials` gives a read that carries no session, so it returns the anonymous view. `Allow-Credentials` without a matching `Allow-Origin` permits nothing. Both together, with the origin taken from the request, permit everything.

### Why a wildcard can be safer than a reflection

This is the counter-intuitive part, and it follows from the specification rather than from caution:

| Configuration | Sent with credentials | Browser |
|---|---|---|
| `Allow-Origin: *` | yes | **refused** — `*` cannot be combined with credentials |
| `Allow-Origin: *` | no | readable — the public view only |
| `Allow-Origin: *` **and** `Allow-Credentials: true` | yes | **refused** — the browser enforces the rule regardless |

So a hard-coded `*` cannot be used to read another user's data, because the browser refuses to combine it with credentials at all. **What is dangerous is not permissiveness; it is responding dynamically to whoever asks** — the reflection satisfies "matches the origin" and "allows credentials" simultaneously, and that combination is the whole vulnerability.

### Detection and mitigation

- **Alert when `Access-Control-Allow-Origin` equals the request's `Origin`.** A correct policy emits a fixed value known at deploy time; a value that tracks the request is reflection, and it is visible in the response without any knowledge of the application.
- **Alert on `Access-Control-Allow-Origin: null`.** The `null` origin comes from sandboxed iframes, `data:` URLs and local files, so allowing it allows readers the deployment cannot enumerate — and a legitimate partner is never `null`.
- **Alert on the pair, not just the origin header.** `Allow-Credentials: true` alongside any cross-origin `Allow-Origin` is the combination that permits reading private data; each header alone is far less interesting, which is why the rule has to be written over the pair.
- **And inventory the endpoints that emit CORS headers at all.** A policy on a public, unauthenticated resource is routine; the same policy on an endpoint that returns account data is the finding, and the list of such endpoints is short enough to maintain.
- **For mitigation, use an exact allowlist.** A fixed set of origins compared with `==`, not a prefix, suffix, substring or pattern, and not a value derived from the request. This is the same rule as the host header, `redirect_uri` and CSRF origin entries — an origin is an identity, and similarity is not identity.
- **Do not reflect, and do not rely on a check that the request's origin appears somewhere in a trusted value.** Every ordering — the trusted name inside the attacker's string, the attacker's name inside the trusted string — is a bypass, and the measured variants are listed in the next entry.
- **Enable CORS only where cross-origin reading is required, and credentials only where it is required too.** A policy that has no reason to exist is a policy nobody reviews, and `Allow-Credentials` is the switch that turns a public read into a private one.
- **And remember that none of this protects the write side.** The request arrives and is processed whether or not the response may be read — which is why the CSRF controls have to be implemented independently of whatever CORS policy is in place.

<!-- lang:zh -->
### CSRF 不讲的那一半边界

CSRF 那篇讲的是**写**：另一个源上的页面能造成状态变更，因为浏览器按目的地附上凭据、而响应从不被读取。这一篇讲**读**，而两者被一句话分开：

> **跨源请求一直都是发得出去的。同源策略限制的是"能不能读到响应"。**

这个区分一旦说出来，CORS 就好推理了：

| | 写（CSRF） | 读（CORS） |
|---|---|---|
| 需要攻击者看到响应 | 不需要 | **需要** |
| 受 CORS 配置影响 | **不受** | 受 |
| 被 CSRF token 挡住 | 是 | 否 |

由此有两个后果，而两个都经常被搞错：

**CORS 配错不会制造 CSRF。** CSRF 不需要任何 CORS 头、不需要预检、也不需要响应配合 —— 它是"一个带着受害者凭据被发出的请求"。一个宽松的 CORS 策略没法让它更糟。

**而配错在另一个方向上更糟。** 它把一个"能造成某个动作"的攻击者，升级成一个"能读到结果"的攻击者 —— 而读，正是把一次盲目的伪造变成外带数据的东西。实测，对着一个私密接口返回账户数据的应用：

```
Origin: https://evil.example
响应:   Access-Control-Allow-Origin: https://evil.example
        Access-Control-Allow-Credentials: true
浏览器判断: 可读
攻击者读到: {"user":"alice","email":"alice@corp.example",
             "csrf_token":"T0KEN-abc123","balance":1234.56}
```

攻击者的页面不需要破坏任何东西。它问了，应用说了可以，而浏览器 —— 完全照它被要求的方式 —— 把响应交给了一段本不该看到它的脚本。

### 四层

1. **信任边界。** 浏览器信任响应里的 `Access-Control-Allow-*` 头，把它当作服务端关于"谁可以读"的陈述。而服务端反过来必须**从一个客户端提供的值**来决定它 —— 这就是 Host 头那篇的形状换了个地方。
2. **数据与指令共用同一平面。** `Origin` 头同时是**"谁在问"的陈述**和**服务端据以决定"允许什么"的输入**。一个把它回显出去的服务端，把一个描述变成了一个许可。
3. **为什么常见修法失败。** "我们加了 CORS 头"听起来像"我们配置了策略"。如果 `Access-Control-Allow-Origin` 的值取自请求，那就不是 —— **把问的人的名字回显回去不是一项策略，而是没有策略**。
4. **变体。** 作为来源的 `null`、按前缀/后缀/子串/未转义正则来匹配、比了主机却忘了协议、以及信任某个可注册域下的所有子域。

### CORS 是什么，预检又是干什么的

**CORS 不是"允许跨源请求"。** 它允许的是跨源的**读**。请求无论如何都会被发出去；那些头决定的是"发它的那个页面能不能看到答案"。

**预检是浏览器在替自己问。** 对一个不"简单"的请求 —— 带自定义头、方法不是 `GET`/`POST`/`HEAD`、或者内容类型是 JSON —— 浏览器先发一个 `OPTIONS`，检查响应是否允许它即将做的事。然后才发真正的请求。

那个机制看起来像访问控制，而它不是，原因在这里很要紧：

> **预检只适用于非简单请求，而 CSRF 需要的一切都是简单请求。**

一个表单、一张图片、一个 `GET`、一个 `Content-Type: text/plain` 的 `POST` —— 上一篇量过最后这种被用来携带 JSON 请求体 —— 全都是简单请求，所以不发预检、也不问许可。接口无论如何都会收到那个请求，与它的 CORS 配置无关。**CORS 不是 CSRF 的防御，而加一条严格的策略也造不出一个。**

### 那两个头，以及为什么两个都要

许可是一个组合，不是一个单独的值：

| 头 | 它允许什么 |
|---|---|
| `Access-Control-Allow-Origin` | **哪个源**可以读这个响应 |
| `Access-Control-Allow-Credentials` | 这次读是否可以带上**凭据** |

实测，用一条写得正确的策略，对着同一个私密接口：

| 请求的源 | `Access-Control-Allow-Origin` | 浏览器 |
|---|---|---|
| `https://app.example` | `https://app.example` | **可读** |
| `https://evil.example` | *（缺失）* | 不可读 |
| `null` | *（缺失）* | 不可读 |

而在把 Origin 反射出去的配置下，那三个全都可读，`null` 也算。**正确的策略在那个头里产生一个固定的值；反射则产生一个随请求变化的值** —— 而那个观察就是检测这种配错的方法。

**两个头单独都不够。** 一个点了攻击者名字的 `Allow-Origin` 若没有 `Allow-Credentials`，给出的是一次不带会话的读，所以它返回的是匿名视图。`Allow-Credentials` 若没有匹配的 `Allow-Origin`，什么也不允许。而两者都有、来源又取自请求时，它允许一切。

### 为什么通配符可能比反射更安全

这是反直觉的那部分，而它是从规范来的，不是从谨慎来的：

| 配置 | 带着凭据发送 | 浏览器 |
|---|---|---|
| `Allow-Origin: *` | 是 | **拒绝** —— `*` 不能与凭据组合 |
| `Allow-Origin: *` | 否 | 可读 —— 只有公开视图 |
| `Allow-Origin: *` **且** `Allow-Credentials: true` | 是 | **拒绝** —— 浏览器无论如何都会执行那条规则 |

所以一个写死的 `*` 无法被用来读别人的数据，因为浏览器根本拒绝把它与凭据组合。**危险的不是"放得宽"，而是"动态地回应任何来问的人"** —— 反射同时满足了"与来源匹配"和"允许凭据"，而那个组合就是整个漏洞。

### 检测与缓解

- **当 `Access-Control-Allow-Origin` 等于请求里的 `Origin` 时告警。** 一条正确的策略发出的是一个部署时就知道的固定值；一个跟着请求走的值就是反射，而它在响应里直接可见，不需要任何关于应用的知识。
- **对 `Access-Control-Allow-Origin: null` 告警。** `null` 这个来源来自沙箱 iframe、`data:` URL 和本地文件，所以允许它等于允许了一批部署无法枚举的读者 —— 而正当的合作方从来不是 `null`。
- **对那一对头告警，而不只是来源头。** `Allow-Credentials: true` 配上任何跨源的 `Allow-Origin`，就是允许读私密数据的那个组合；单独每个头都远没那么有意思，所以规则必须写在"这一对"上。
- **并且把会发出 CORS 头的接口盘出来。** 一个公共、未认证资源上的策略是例行公事；同一条策略出现在一个返回账户数据的接口上就是发现，而这类接口的清单短到可以维护。
- **缓解上，用精确的允许清单。** 一组固定的来源、用 `==` 比较，不用前缀、后缀、子串或模式，也不用任何从请求推导出来的值。这和 Host 头、`redirect_uri`、CSRF 来源那几篇是同一条规则 —— 来源是一个身份，而相似不是身份。
- **不要反射，也不要依赖"请求的来源出现在某个受信值里"这种检查。** 两种顺序 —— 受信的名字在攻击者的字符串里、攻击者的名字在受信的字符串里 —— 都是绕过，实测过的那些变体列在下一篇里。
- **只在确实需要跨源读取的地方开 CORS，也只在确实需要的地方开凭据。** 一个没有理由存在的策略，就是一个没人会去审查的策略；而 `Allow-Credentials` 就是那个把"公开读取"变成"私密读取"的开关。
- **并且记住这一切都不保护写的那一侧。** 无论响应是否可读，请求都会到达并被处理 —— 这就是为什么 CSRF 那些控制必须独立于任何 CORS 策略去实现。
