---
id: http-and-web-fundamentals
title_en: HTTP and Web Fundamentals
title_zh: HTTP 与 Web 基础安全
summary_en: Some of the highest-value bugs are not in the application at all. They are in how a request is framed, cached, routed and normalised by the two or three systems sitting between the browser and the code.
summary_zh: 价值最高的一批漏洞根本不在应用里，而在"请求是怎么被分帧、缓存、路由和规范化的" —— 也就是浏览器与代码之间那两三层系统。
tags: [web, http, request-smuggling, host-header, cache-poisoning, crlf]
tools: [Burp Suite, smuggler, h2csmuggler, ffuf]
attck: [T1190]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The idea running through all of this

A modern request passes through a CDN, a load balancer, a reverse proxy, a WAF, an application server and a framework before it reaches code. Each of those parses the request (or the response) in its own way. Where two of them disagree, an attacker gets to choose which one the security controls see.

That single sentence covers request smuggling, host header attacks, cache poisoning and most header injection. The payloads differ; the reasoning does not.

### Request smuggling

Front-end and back-end disagree about where one request ends and the next begins, because they handle `Content-Length` and `Transfer-Encoding` differently:

```http
POST / HTTP/1.1
Host: target.example
Content-Length: 13
Transfer-Encoding: chunked

0

SMUGGLED
```

If the front-end uses `Content-Length` and the back-end uses `Transfer-Encoding`, the back-end sees a second request (`SMUGGLED`) that the front-end never inspected — and that request may be processed as if it came from the next user on the connection.

The variants worth knowing:

| Variant | The disagreement |
|---|---|
| CL.TE | Front-end trusts Content-Length, back-end trusts Transfer-Encoding |
| TE.CL | The reverse |
| TE.TE | Both support chunked, but one can be tricked into ignoring it with obfuscation |
| H2.CL / H2.TE | HTTP/2 in the front, HTTP/1.1 behind, with headers that should not survive the downgrade |

Detection is timing-based and fiddly. Use the tools rather than hand-crafting:

```bash
# Burp extension: HTTP Request Smuggler, and its CLI equivalent
python3 smuggler.py -u https://target.example
```

Smuggling is powerful and dangerous in equal measure: a successful probe can affect other users' requests. Test only with explicit authorisation, and never experiment on shared production infrastructure without agreeing the method first.

### Host header attacks

The application trusts the `Host` header, and the `Host` header is attacker-controlled unless the edge validates it. Where that trust lands:

- **Password reset poisoning.** The reset link is built from `Host`, so the victim's token is delivered to the attacker.
- **Cache poisoning.** A response cached under the attacker's host serves to everyone.
- **Routing-based SSRF.** An internal proxy or load balancer routes on `Host`, so it can be pointed at an internal service.
- **Virtual host confusion.** The default vhost serves a different application than the one the attacker asked for.

Variants that often bypass naive validation: `X-Forwarded-Host`, `X-Forwarded-Server`, `X-Host`, `X-Original-URL`, absolute URLs in the request line, a host with a trailing dot, and duplicate `Host` headers.

```http
GET / HTTP/1.1
Host: attacker.example
X-Forwarded-Host: attacker.example
```

### Cache poisoning and cache deception

Caches key on some parts of the request and not others. Anything the cache ignores but the application uses is an **unkeyed input**, and it is the raw material for poisoning:

```http
GET /home HTTP/1.1
Host: target.example
X-Forwarded-Host: evil.example      <-- used by the app, not part of the cache key
```

If the response reflects that header and gets cached, every subsequent visitor receives the attacker's version. The same idea with different inputs: `X-Forwarded-Scheme`, a fat `GET` with a body, or a parameter the origin reads but the cache ignores.

**Cache deception** is the mirror image: trick the cache into storing a page with private data by requesting it with a suffix the cache treats as a static file (`/account/nonexistent.css`). The next visitor gets the victim's page.

### CRLF injection and header manipulation

A newline in a header value ends the header and starts a new one:

```
%0d%0aSet-Cookie: session=attacker
```

Where it appears: redirect targets built from user input, header values echoed back, log entries that get rendered into a page. It can split a response, inject a cookie, or inject content into a proxied response.

### HTTP/2 specific

- **Downgrade issues**: a header that is legal in HTTP/2 (for example, one containing a newline encoded as a pseudo-header trick) is interpreted differently once past a proxy that converts to HTTP/1.1.
- **Pseudo-header confusion**: `:path`, `:method` and `:authority` take priority over their HTTP/1.1 equivalents, and some stacks do not reconcile the two.
- **HPACK**: header compression state can be abused for side channels in specific configurations.
- **H2C smuggling**: a request that upgrades a plaintext connection to HTTP/2 past a proxy that only inspects the initial HTTP/1.1 request.

### Smaller things that still pay

- **Method override headers** (`X-HTTP-Method-Override: DELETE`) reaching a framework that honours them.
- **Content type confusion**: the same parameters as form data, JSON and multipart, where different layers parse different bodies.
- **Parameter parsing differences**: duplicate parameters, array syntax, semicolons as separators.
- **Open redirects**, which are usually a low-severity finding until they carry a token or bypass an OAuth check.
- **Status code semantics**: a `403` for one path and `404` for another is an enumeration primitive.

### Detection

- **Requests with both `Content-Length` and `Transfer-Encoding`** should be rejected at the edge, and their appearance is worth alerting on.
- **`Host` values that do not match your domains** on any request, which is a cheap and high-signal rule.
- **Cache keys that do not include headers the application reads** is a configuration review item; in logs, look for responses cached under an unexpected host or scheme.
- **CRLF sequences in parameters** (`%0d%0a`, encoded variants) reaching redirects or headers.
- **Anomalous timing on the same endpoint** from a single client, which is what smuggling probes look like.

### Mitigation

- **Normalise at the edge and reject ambiguity.** A request with conflicting framing headers should be refused, not interpreted. This one change removes most smuggling.
- **Validate the `Host` header against an allow-list** of your own domains, and do not build URLs from the request when a configured base URL will do.
- **Include everything the application reads in the cache key**, or strip those headers before they reach the origin. Autodiscover and normalise rather than trusting the origin's behaviour.
- **Strip or encode control characters** in anything that reaches a header, a redirect or a log.
- **Keep the parsers consistent** across the stack. Most of this category exists because a CDN, a proxy and a framework each implement a slightly different HTTP.
- **Prefer HTTP/2 end to end** where you can, since the downgrade boundary is where several of these live.

<!-- lang:zh -->
### 贯穿这一切的一个想法

一个现代请求在到达代码之前，会经过 CDN、负载均衡、反向代理、WAF、应用服务器和框架。每一层都用自己的方式解析请求（或响应）。**当其中两层理解不一致时，攻击者就能选择让安全控制看到哪一版。**

这一句话覆盖了请求走私、Host 头攻击、缓存投毒和大多数请求头注入。payload 不同，推理方式相同。

### 请求走私

前端与后端对"一个请求在哪里结束、下一个从哪里开始"理解不同，因为它们处理 `Content-Length` 与 `Transfer-Encoding` 的方式不同：

```http
POST / HTTP/1.1
Host: target.example
Content-Length: 13
Transfer-Encoding: chunked

0

SMUGGLED
```

如果前端按 `Content-Length` 解析、后端按 `Transfer-Encoding` 解析，后端就会看到一个前端从未检查过的第二请求（`SMUGGLED`）—— 而这个请求可能被当成"连接上下一个用户发来的"处理。

值得知道的几种变体：

| 变体 | 分歧点 |
|---|---|
| CL.TE | 前端信 Content-Length，后端信 Transfer-Encoding |
| TE.CL | 反过来 |
| TE.TE | 双方都支持 chunked，但可以用混淆手法让其中一方忽略它 |
| H2.CL / H2.TE | 前面是 HTTP/2、后面是 HTTP/1.1，而某些头本不该在降级后存活 |

检测靠计时，而且很挑环境。用工具，别手搓：

```bash
# Burp 扩展 HTTP Request Smuggler，以及它的命令行版本
python3 smuggler.py -u https://target.example
```

走私的威力和危险性一样大：一次成功的探测可能影响其他用户的请求。只在明确授权下测试，未经约定不要在共享的生产基础设施上做实验。

### Host 头攻击

应用信任 `Host` 头，而除非边缘做了校验，`Host` 头就是攻击者可控的。这份信任会落在这些地方：

- **口令重置投毒。** 重置链接用 `Host` 拼接，于是受害者的 token 被送到攻击者手里。
- **缓存投毒。** 以攻击者的主机名缓存的响应，之后发给所有人。
- **路由型 SSRF。** 内部代理或负载均衡按 `Host` 路由，于是可以被指向内部服务。
- **虚拟主机混淆。** 默认 vhost 提供的应用和攻击者请求的那个不是同一个。

常常绕过幼稚校验的变体：`X-Forwarded-Host`、`X-Forwarded-Server`、`X-Host`、`X-Original-URL`、请求行里的绝对 URL、结尾带点的主机名，以及重复的 `Host` 头。

```http
GET / HTTP/1.1
Host: attacker.example
X-Forwarded-Host: attacker.example
```

### 缓存投毒与缓存欺骗

缓存的键只覆盖请求的一部分。**凡是缓存忽略、而应用会用的输入，就是未键控输入（unkeyed input）**，也是投毒的原料：

```http
GET /home HTTP/1.1
Host: target.example
X-Forwarded-Host: evil.example      <-- 应用会用它，但不在缓存键里
```

如果响应反射了这个头并被缓存，之后每个访客拿到的都是攻击者的版本。同样的思路换输入：`X-Forwarded-Scheme`、带请求体的 fat GET、或者源站会读而缓存忽略的参数。

**缓存欺骗**是它的镜像：用一个缓存会当作静态文件的后缀去请求私有页面（`/account/nonexistent.css`），骗缓存把它存下来，下一个访客就拿到受害者的页面。

### CRLF 注入与请求头操纵

请求头值里的一个换行就能结束当前头、开始一个新头：

```
%0d%0aSet-Cookie: session=attacker
```

出现位置：由用户输入拼接的跳转目标、被回显的头值、会被渲染进页面的日志条目。它可以拆分响应、注入 cookie，或者往被代理的响应里塞内容。

### HTTP/2 相关

- **降级问题**：在 HTTP/2 里合法的头（例如借助伪头技巧编码了换行的那个），一旦经过把协议转成 HTTP/1.1 的代理，含义就变了。
- **伪头混淆**：`:path`、`:method`、`:authority` 的优先级高于 HTTP/1.1 的对应项，而有些技术栈不会把两者对齐。
- **HPACK**：在特定配置下，头压缩状态可被用于侧信道。
- **H2C 走私**：一个把明文连接升级到 HTTP/2 的请求，穿过只检查初始 HTTP/1.1 请求的代理。

### 小一些但依然有收益的点

- **方法覆盖头**（`X-HTTP-Method-Override: DELETE`）打到会认它的框架。
- **内容类型混淆**：同一组参数分别用表单、JSON、multipart 发送，不同层解析不同请求体。
- **参数解析差异**：重复参数、数组语法、分号当分隔符。
- **开放重定向** —— 通常是低危，直到它携带了 token 或绕过了 OAuth 校验。
- **状态码语义**：某条路径返回 403、另一条返回 404，就是一个枚举原语。

### 检测

- **同时带 `Content-Length` 和 `Transfer-Encoding` 的请求**应在边缘被拒绝，出现就该告警。
- **`Host` 值不属于你的域名**的请求 —— 一条便宜且高命中的规则。
- **缓存键未包含应用会读的请求头**属于配置复查项；在日志里则找"以意外主机名或协议缓存的响应"。
- **参数里的 CRLF 序列**（`%0d%0a` 及各种编码变体）抵达了跳转或请求头。
- **同一接口上来自单个客户端的异常耗时** —— 走私探测就长这样。

### 缓解

- **在边缘做规范化，并拒绝歧义。** 分帧头互相冲突的请求应当被拒绝，而不是被解释。这一条就能消掉大部分走私。
- **用白名单校验 `Host` 头**（只允许自己的域名），能用配置里的固定基址就不要用请求里的值拼 URL。
- **把应用会读的东西都纳入缓存键**，或者在到达源站之前把这些头剥掉。宁可自动发现与规范化，也不要指望源站行为。
- **任何进入请求头、跳转或日志的内容都剥离或编码控制字符。**
- **让整条链路上的解析器保持一致。** 这一大类之所以存在，就是因为 CDN、代理和框架各自实现了略有不同的 HTTP。
- **能端到端用 HTTP/2 就用**，因为上面好几个问题都住在降级边界上。
