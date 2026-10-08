---
id: web-basics
title_en: Web Basics for Beginners
title_zh: Web 入门基础
summary_en: A web application is a conversation between a browser and a server, in a protocol you can read. This entry covers that protocol, the state it pretends not to have, the browser as a platform, and the three tools a beginner needs to watch the conversation happen.
summary_zh: 一个 Web 应用，就是浏览器和服务端在你读得懂的协议上的一段对话。这一篇讲这段协议、讲它声称自己没有却其实有的状态、讲作为一个平台的浏览器，以及新手看这段对话发生所需要的三个工具。
tags: [beginner, web, http, browser, devtools, cookies, api]
tools: [curl, browser devtools, Burp Suite, jq]
attck: [T1190]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### What actually happens when you open a page

Before any protocol detail, get the shape of the whole thing. Typing a URL and pressing enter causes, in order:

1. **DNS** resolves the hostname to an IP address.
2. **TCP** connects to that address on port 443 (or 80).
3. **TLS** negotiates encryption and validates the server's certificate.
4. **HTTP** sends a request for a path, with headers.
5. The server sends back a status code, headers, and a body.
6. The browser parses the body, finds more resources referenced in it (images, CSS, JavaScript), and repeats steps 2 to 5 for each — which is why one page view becomes dozens of requests.
7. JavaScript executes, the DOM is built, and the page becomes interactive.

You can watch the whole thing from a terminal with one command, and you should do it at least once:

```bash
curl -v https://example.com/ 2>&1 | head -40
```

You will see the connection, the TLS handshake summary, the request headers your client sent, and the response headers. **That is the entire protocol in one screen.** Everything else in this entry is detail about those lines.

### Part 1: HTTP, the protocol

HTTP is a **text-based request-response protocol**. One request, one response, then the connection is idle (or reused).

#### The request

```
GET /search?q=term HTTP/1.1        <- method, path, version
Host: example.com                  <- headers
User-Agent: curl/8.5.0
Accept: text/html
Cookie: session=abc123
                                   <- a blank line ends the headers
                                   <- the body goes here (empty for GET)
```

#### The response

```
HTTP/1.1 200 OK                    <- version, status code, reason
Content-Type: text/html; charset=utf-8
Content-Length: 1256
Set-Cookie: session=xyz; HttpOnly
                                   <- blank line
<!DOCTYPE html>...                 <- the body
```

#### Methods

| Method | Means | Body? | Safe? | Idempotent? |
|---|---|---|---|---|
| GET | Give me this resource | No | Yes | Yes |
| HEAD | Like GET, headers only | No | Yes | Yes |
| POST | Here is data, process it | Yes | No | No |
| PUT | Replace this resource | Yes | No | Yes |
| PATCH | Modify part of it | Yes | No | No |
| DELETE | Remove it | Sometimes | No | Yes |
| OPTIONS | What is allowed here? | No | Yes | Yes |

**Safe** means it should not change anything. **Idempotent** means doing it twice has the same effect as doing it once. These are not academic: a GET that changes state can be triggered by an image tag on another site, and a retried POST that is not idempotent can create two orders.

#### Status codes

The first digit tells you the category, and that is the part worth internalising:

| Range | Meaning |
|---|---|
| 1xx | Informational — rarely seen directly |
| 2xx | Success |
| 3xx | Redirection |
| 4xx | The client did something wrong |
| 5xx | The server did something wrong |

The ones you will meet constantly:

| Code | Name | What it really means |
|---|---|---|
| 200 | OK | Normal success |
| 201 | Created | A POST created something; the `Location` header says where |
| 204 | No Content | Success with nothing to return |
| 301 | Moved Permanently | Cache this redirect forever |
| 302 / 307 | Found / Temporary Redirect | Redirect for now |
| 304 | Not Modified | Use your cached copy |
| 400 | Bad Request | Malformed request |
| 401 | Unauthorized | Not authenticated — "who are you?" |
| 403 | Forbidden | Authenticated, but not allowed — "I know who you are, no" |
| 404 | Not Found | No such thing, or the server is hiding it |
| 405 | Method Not Allowed | Right path, wrong method |
| 415 | Unsupported Media Type | Wrong `Content-Type` |
| 429 | Too Many Requests | Rate limited |
| 500 | Internal Server Error | Unhandled exception |
| 502 | Bad Gateway | The proxy could not reach the app |
| 503 | Service Unavailable | Overloaded or down |

**401 versus 403 confuses everyone.** 401 means "I do not know who you are". 403 means "I know exactly who you are, and you may not do this".

#### Headers that matter

Request headers:

| Header | Meaning |
|---|---|
| `Host` | Which site you want — one IP can serve many |
| `User-Agent` | What client you are |
| `Accept` / `Accept-Language` | What you can handle |
| `Content-Type` | What the body is (`application/json`, `application/x-www-form-urlencoded`, `multipart/form-data`) |
| `Content-Length` | How big the body is |
| `Cookie` | State the server gave you earlier |
| `Authorization` | Credentials, e.g. `Bearer <token>` |
| `Referer` | The page you came from |
| `Origin` | The site that initiated this request |
| `X-Forwarded-For` | Added by proxies; **client-controllable unless the edge rewrites it** |

Response headers:

| Header | Meaning |
|---|---|
| `Content-Type` | What the body is — this is how the browser decides to render or download |
| `Content-Length` / `Transfer-Encoding` | How the body is delimited |
| `Set-Cookie` | Store this state |
| `Location` | Where to go next, with a 3xx |
| `Cache-Control`, `ETag`, `Last-Modified` | Caching rules |
| `Content-Security-Policy` | What this page is allowed to load and execute |
| `Strict-Transport-Security` | Always use HTTPS for this site |
| `X-Content-Type-Options: nosniff` | Do not guess the content type |
| `Access-Control-Allow-Origin` | Which origins may read this response |

#### The bodies you will meet

| `Content-Type` | Shape |
|---|---|
| `text/html` | A page |
| `application/json` | `{"key": "value"}` — the modern API default |
| `application/x-www-form-urlencoded` | `a=1&b=2` — what an HTML form sends by default |
| `multipart/form-data` | Sections with boundaries — required for file uploads |
| `text/plain`, `text/xml` | Exactly what they say |

#### Versions, briefly

HTTP/1.1 sends one request at a time per connection, which is why browsers open several connections. **HTTP/2** multiplexes many requests over one connection and compresses headers. **HTTP/3** runs over QUIC/UDP instead of TCP, which removes a round trip from connection setup. For a beginner the practical point is that a modern site is not one request after another — it is dozens, in parallel, over one or two connections.

### Part 2: state, which HTTP pretends not to have

HTTP is **stateless**: the server does not remember your previous request. That is a feature for scalability and a problem for anything that requires logging in.

#### Cookies

The solution is a cookie: the server sends `Set-Cookie`, the browser stores it and sends it back on every subsequent request to that site.

```
Set-Cookie: session=abc123; Path=/; Domain=example.com; Max-Age=3600; Secure; HttpOnly; SameSite=Lax
```

Every attribute has a purpose, and they are worth learning because they are also the defences:

| Attribute | Meaning | What it prevents |
|---|---|---|
| `Domain` | Which hosts receive it | Cookies leaking to sibling domains |
| `Path` | Which paths receive it | Over-broad exposure |
| `Expires` / `Max-Age` | When it dies | Sessions that never end |
| `Secure` | HTTPS only | The cookie travelling in cleartext |
| `HttpOnly` | JavaScript cannot read it | Script-based theft |
| `SameSite=Lax` | Not sent on cross-site subrequests | Cross-site requests carrying your cookie |
| `SameSite=Strict` | Only same-site requests | The same, more aggressively |
| `SameSite=None` | Always sent — requires `Secure` | (Needed for legitimate cross-site use) |

#### Sessions

A **session** is the server-side record. The cookie holds only an identifier, and the server looks up the rest:

```
Cookie: session=abc123   ->   server stores { user_id: 42, role: "user", cart: [...] }
```

Advantages: the client cannot tamper with what it cannot read, and the server can invalidate a session instantly. The cost: the server must store the sessions.

#### Tokens

A **token** (JWT is the common format) puts the state in the client instead:

```
Authorization: Bearer eyJhbGciOi...
```

A JWT is three base64url parts — header, payload, signature — separated by dots. The payload is **readable by anyone**; the signature is what makes it trustworthy. Advantage: no server-side session store. Cost: revocation is hard, because the server is not the one keeping the list.

A beginner should be able to say the difference out loud: **a session cookie is a claim ticket, a token is a signed note**. The claim ticket can be voided at the counter; the signed note cannot, until it expires.

### Part 3: URLs

```
https://user:pass@example.com:8443/a/b?x=1&y=2#frag
└─┬─┘ └───┬───┘ └────┬────┘└┬─┘└─┬─┘└────┬────┘└─┬─┘
scheme  userinfo    host   port path    query   fragment
```

- **scheme** — `http` or `https`.
- **path** — which resource.
- **query** — parameters, `key=value` pairs separated by `&`.
- **fragment** — client-side only; **never sent to the server**.

That last point trips people up constantly. If it is after the `#`, the server has never seen it.

Query values are URL-encoded, which is why a space is `%20` and Chinese text becomes a long run of `%E4%B8%AD` groups. Some characters are safe, others must be encoded, and the encoding is what the earlier entry on encoding covers.

### Part 4: the browser as a platform

#### HTML, the minimum you need

An HTML document is nested elements. The parts that matter for how a site behaves:

```html
<form action="/login" method="post">
  <input type="text" name="username" value="">
  <input type="password" name="password" value="">
  <input type="hidden" name="csrf_token" value="abc123">
  <button type="submit">Sign in</button>
</form>
```

Three things to notice, because they explain a great deal later:

- The **`name`** is what the server receives as a parameter name. The `value` is what it receives as the value.
- **`method="post"`** means the data goes in the body; the default `get` would put it in the URL.
- **Hidden fields are not hidden.** They are in the page source, and a client can change any of them.

#### The DOM

The browser parses HTML into the **DOM** — a tree of objects that JavaScript can read and modify. This is why "the page" and "the HTML the server sent" are different things: by the time you look at it, JavaScript may have rewritten it.

In developer tools, Elements shows you the **current DOM**, not the original source. If you want the original, use View Source or `curl`.

#### JavaScript in the browser

JavaScript can do anything to the page it is running on: read and rewrite the DOM, make requests, store data, read cookies that are not `HttpOnly`.

JavaScript **cannot** freely read data from other origins. That restriction is the next section.

#### The same-origin policy

An **origin** is exactly three things: **scheme + host + port**.

| URL A | URL B | Same origin? |
|---|---|---|
| `https://example.com/a` | `https://example.com/b` | Yes |
| `https://example.com` | `http://example.com` | No — different scheme |
| `https://example.com` | `https://www.example.com` | No — different host |
| `https://example.com` | `https://example.com:8443` | No — different port |

The same-origin policy says a script from one origin cannot read the responses of another. It is the reason a random site cannot read your webmail: the cookie might be sent, but the response cannot be read.

**CORS** is the controlled exception. A server can say "I allow origin X to read me" with `Access-Control-Allow-Origin`, and the browser will permit it. For requests that are not "simple", the browser first sends a **preflight** `OPTIONS` request to ask permission.

Two facts worth keeping: CORS is enforced by the **browser**, not the server — a script with `curl` ignores it entirely. And CORS is about *reading responses*, not about *sending requests*: a cross-site form post still arrives.

### Part 5: the three tools

#### 1. The browser's developer tools

Open with F12. The panels a beginner needs:

| Panel | What it is for |
|---|---|
| **Network** | Every request, with headers, body, timing and status. The most useful panel for security work |
| **Elements** | The live DOM and CSS |
| **Console** | Run JavaScript in the page's context |
| **Application / Storage** | Cookies, localStorage, sessionStorage, service workers |
| **Sources** | Breakpoints in JavaScript, and the ability to modify it live |

The Network panel moves worth learning on day one: **click a request and read the headers** (this is how you see what cookies and tokens are actually sent), **right-click → Copy → Copy as cURL** (reproduce the request in a terminal), **right-click → Edit and Resend** (change something and see what happens), and the **Preserve log** toggle, without which navigation wipes the history.

#### 2. curl

The same requests, from a script, reproducible in a report:

```bash
curl -i https://example.com/                          # include response headers
curl -s -X POST https://example.com/login \
     -H 'Content-Type: application/json' \
     -d '{"username":"alice","password":"test"}'      # a JSON POST
curl -s -b 'session=abc123' https://example.com/me    # send a cookie
curl -sD - -o /dev/null https://example.com/          # headers only
curl -s https://api.example.com/items | jq .          # pretty-print JSON
```

#### 3. An intercepting proxy

Developer tools can see and edit one request. A proxy can **intercept everything**, pause a request before it leaves, modify it, replay it, fuzz it, and keep a history across sessions. Burp Suite and ZAP are the two you will meet.

How it works: the browser is configured to send traffic to the proxy instead of directly to the server. Because HTTPS is encrypted end to end, the proxy must be able to decrypt, which means it generates its own certificate and your browser must trust it. That is why installing the proxy's CA certificate is a required setup step, and why it only works on a machine you control.

| Component | Use it for |
|---|---|
| Proxy → Intercept | Pause and modify a request in flight |
| HTTP history | Everything that happened, searchable |
| Repeater | Edit one request and send it repeatedly |
| Intruder | Send many variations (parameter fuzzing, enumeration) |
| Decoder | Encode and decode without leaving the tool |
| Comparer | Diff two responses |

### Part 6: doing it end to end

A first exercise that ties everything together. Pick a site you own or a lab target.

1. **Terminal:** `curl -v https://target/` and read the request and response headers. Identify `Content-Type`, `Set-Cookie` and any `Cache-Control`.
2. **Browser:** open developer tools, go to Network, enable **Preserve log**, then log in. Find the login request. What method? What `Content-Type`? Did the body carry the password in the clear? What came back, a `302` with `Set-Cookie`, or a `200` with a JSON token?
3. **Look at the storage:** in Application, find the cookie or token. Is it `HttpOnly`? `Secure`? `SameSite`? How long until it expires?
4. **Reproduce it:** right-click the request → Copy as cURL, paste into a terminal, and confirm you get the same response.
5. **Change one thing:** use Edit and Resend (or Repeater) and modify a value. Watch what the server does. **This is the moment the whole subject clicks** — the client is entirely under your control, and everything that matters is what the server decides to do about it.

### Part 7: APIs, the modern shape

Most modern web applications are not pages anymore; they are a JavaScript front-end talking to a JSON API. The same protocol, different content.

- **The URL identifies a resource:** `/api/users/42`.
- **The method is the verb:** GET to read, POST to create, PUT/PATCH to modify, DELETE to remove.
- **The status code is the outcome.**
- **The body is JSON:** `{"id": 42, "name": "alice", "role": "user"}`.

Authentication options you will encounter:

| Scheme | Shape | Notes |
|---|---|---|
| Cookie | `Cookie: session=...` | Browser-managed; the session entry covers the attributes |
| Bearer token | `Authorization: Bearer eyJ...` | Often a JWT; stateless |
| API key | `X-API-Key: ...` or a query parameter | Common for machine access; long-lived |
| Basic | `Authorization: Basic base64(user:pass)` | Encoded, not encrypted — only over TLS |

```bash
curl -s -H 'Authorization: Bearer eyJ...' https://api.example.com/me | jq .
```

### Part 8: the security ideas that follow from all this

Still concepts, not techniques — the specific vulnerability classes have their own entries. But four things become obvious once you understand the protocol:

1. **The client is not yours.** Anything in the browser — validation, hidden fields, JavaScript checks, disabled buttons — can be changed. If the server does not re-check it, there is no check.
2. **Hidden is not secret.** Page source, comments, JavaScript files, source maps and old API versions are all readable. Development builds leak endpoints, keys and comments routinely.
3. **HTTP without TLS is public.** Anyone on the path reads everything, including cookies and passwords.
4. **Everything you send is data the server must handle.** Parameter names, values, headers, content types and file uploads all arrive from a machine you do not control. That is the whole reason input handling matters, and it is where the later entries on individual vulnerability classes begin.

### Detection and mitigation

- **Set the security headers and mean them.** `Content-Security-Policy` restricts what a page can load and execute, `Strict-Transport-Security` forces HTTPS, `X-Content-Type-Options: nosniff` stops content-type guessing, and `Referrer-Policy` controls what leaks in referrers. They are one line each in the web server, and they remove whole classes of problem.
- **Cookie attributes are a control, not decoration.** `Secure`, `HttpOnly` and `SameSite` on every session cookie; short lifetimes; rotate the identifier on login.
- **Validate on the server, always.** Client-side validation is a usability feature. Write it as if the client does not exist, because from the server's point of view it does not.
- **Do not put secrets in the front end.** API keys in JavaScript, tokens in `localStorage`, and hidden fields holding prices are all readable. If the browser has it, the user has it.
- **Log requests with enough context.** Method, path, status, the authenticated user, the source address and a request id. Without the request id, an incident cannot be reconstructed from logs that do not join up.
- **Monitor the shape of traffic, not just the volume.** A spike in 4xx from one session, requests to paths that do not exist, and unusual `Content-Type` values are cheap signals that something is being probed.
- **Force HTTPS everywhere**, including inside the network, so that a position on the path is worth nothing.

<!-- lang:zh -->
### 打开一个页面时，实际发生了什么

在看任何协议细节之前，先建立整体形状。输入一个网址并回车，会**按顺序**发生这些事：

1. **DNS** 把域名解析成 IP 地址。
2. **TCP** 连到那个地址的 443 端口（或 80）。
3. **TLS** 协商加密并校验服务端证书。
4. **HTTP** 发出对某个路径的请求，带着请求头。
5. 服务端返回状态码、响应头和响应体。
6. 浏览器解析响应体，发现里面还引用了别的资源（图片、CSS、JavaScript），于是对每一个重复第 2 到 5 步 —— 这就是为什么"看一个页面"会变成几十个请求。
7. JavaScript 执行、DOM 构建完成，页面变得可交互。

你可以在终端里用一条命令把整个过程看完，而且**至少该做一次**：

```bash
curl -v https://example.com/ 2>&1 | head -40
```

你会看到连接过程、TLS 握手摘要、你的客户端发出的请求头、以及服务端的响应头。**一屏之内就是整个协议。** 这一篇剩下的内容，都是关于那几行的细节。

### 第一部分：HTTP 协议本身

HTTP 是**基于文本的请求-响应协议**。一个请求、一个响应，然后连接空闲（或被复用）。

#### 请求

```
GET /search?q=term HTTP/1.1        <- 方法、路径、版本
Host: example.com                  <- 请求头
User-Agent: curl/8.5.0
Accept: text/html
Cookie: session=abc123
                                   <- 一个空行结束请求头
                                   <- 请求体从这里开始（GET 为空）
```

#### 响应

```
HTTP/1.1 200 OK                    <- 版本、状态码、原因短语
Content-Type: text/html; charset=utf-8
Content-Length: 1256
Set-Cookie: session=xyz; HttpOnly
                                   <- 空行
<!DOCTYPE html>...                 <- 响应体
```

#### 方法

| 方法 | 含义 | 有请求体？ | 安全？ | 幂等？ |
|---|---|---|---|---|
| GET | 把资源给我 | 否 | 是 | 是 |
| HEAD | 像 GET，但只给头 | 否 | 是 | 是 |
| POST | 我这里有一份数据，处理它 | 是 | 否 | 否 |
| PUT | 替换这个资源 | 是 | 否 | 是 |
| PATCH | 修改它的一部分 | 是 | 否 | 否 |
| DELETE | 删除它 | 有时 | 否 | 是 |
| OPTIONS | 这里允许什么？ | 否 | 是 | 是 |

**安全**指它不该改变任何东西；**幂等**指做两次和做一次效果相同。这不是学术概念：一个会改状态的 GET 可以被另一个站点的图片标签触发，而一个非幂等的 POST 被重试就会多下一单。

#### 状态码

第一位数字就说明类别，这是该记住的部分：

| 区间 | 含义 |
|---|---|
| 1xx | 信息类 —— 很少直接见到 |
| 2xx | 成功 |
| 3xx | 重定向 |
| 4xx | 客户端做错了 |
| 5xx | 服务端做错了 |

你会不断遇到的：

| 码 | 名称 | 实际含义 |
|---|---|---|
| 200 | OK | 正常成功 |
| 201 | Created | POST 创建了东西，`Location` 头说明在哪 |
| 204 | No Content | 成功，但没有内容要返回 |
| 301 | Moved Permanently | 永久重定向，可以长期缓存 |
| 302 / 307 | Found / Temporary Redirect | 暂时重定向 |
| 304 | Not Modified | 用你缓存的那份 |
| 400 | Bad Request | 请求格式不对 |
| 401 | Unauthorized | 未认证 —— "你是谁？" |
| 403 | Forbidden | 已认证，但不允许 —— "我知道你是谁，不行" |
| 404 | Not Found | 没有这个东西，或者服务端在藏着它 |
| 405 | Method Not Allowed | 路径对，方法错 |
| 415 | Unsupported Media Type | `Content-Type` 不对 |
| 429 | Too Many Requests | 被限流了 |
| 500 | Internal Server Error | 未处理的异常 |
| 502 | Bad Gateway | 代理到不了后面的应用 |
| 503 | Service Unavailable | 过载或已下线 |

**401 和 403 最容易混。** 401 的意思是"我不知道你是谁"；403 的意思是"我完全知道你是谁，但你不能做这件事"。

#### 该认识的请求头

| 请求头 | 含义 |
|---|---|
| `Host` | 你要哪个站点 —— 一个 IP 可以服务很多站点 |
| `User-Agent` | 你是什么客户端 |
| `Accept` / `Accept-Language` | 你能接受什么 |
| `Content-Type` | 请求体是什么（`application/json`、`application/x-www-form-urlencoded`、`multipart/form-data`） |
| `Content-Length` | 请求体多大 |
| `Cookie` | 服务端之前给你的状态 |
| `Authorization` | 凭据，比如 `Bearer <token>` |
| `Referer` | 你从哪个页面来的 |
| `Origin` | 发起这次请求的站点 |
| `X-Forwarded-For` | 代理加上的；**除非边缘做了重写，否则客户端可控** |

#### 该认识的响应头

| 响应头 | 含义 |
|---|---|
| `Content-Type` | 响应体是什么 —— 浏览器靠它决定是渲染还是下载 |
| `Content-Length` / `Transfer-Encoding` | 响应体怎么界定边界 |
| `Set-Cookie` | 把这个状态存起来 |
| `Location` | 下一步去哪，配合 3xx |
| `Cache-Control`、`ETag`、`Last-Modified` | 缓存规则 |
| `Content-Security-Policy` | 这个页面允许加载和执行什么 |
| `Strict-Transport-Security` | 本站一律用 HTTPS |
| `X-Content-Type-Options: nosniff` | 不要猜内容类型 |
| `Access-Control-Allow-Origin` | 哪些源可以读这个响应 |

#### 你会遇到的请求体形态

| `Content-Type` | 形状 |
|---|---|
| `text/html` | 一个页面 |
| `application/json` | `{"key": "value"}` —— 现代 API 的默认 |
| `application/x-www-form-urlencoded` | `a=1&b=2` —— HTML 表单默认发这个 |
| `multipart/form-data` | 用边界分隔的多个段 —— 文件上传必须用它 |
| `text/plain`、`text/xml` | 名字就是答案 |

#### 版本，简单说

HTTP/1.1 每条连接一次只能发一个请求，所以浏览器会开好几条连接。**HTTP/2** 在一条连接上多路复用很多请求，并压缩请求头。**HTTP/3** 跑在 QUIC/UDP 上而不是 TCP 上，省掉了连接建立的一个往返。对新手来说，实用的一点是：现代站点不是"一个接一个的请求"，而是**几十个并行请求，走在一条或两条连接上**。

### 第二部分：状态 —— HTTP 假装自己没有的东西

HTTP 是**无状态**的：服务端不记得你上一次请求。这对扩展性是优点，对任何需要登录的东西是问题。

#### Cookie

解决办法是 cookie：服务端发 `Set-Cookie`，浏览器存下来，并在之后**每一次**对该站点的请求里带回去。

```
Set-Cookie: session=abc123; Path=/; Domain=example.com; Max-Age=3600; Secure; HttpOnly; SameSite=Lax
```

每个属性都有用途，而且它们同时也是防御手段，值得记住：

| 属性 | 含义 | 它防的是什么 |
|---|---|---|
| `Domain` | 哪些主机能收到它 | cookie 泄漏给兄弟域 |
| `Path` | 哪些路径能收到它 | 暴露面过宽 |
| `Expires` / `Max-Age` | 什么时候失效 | 永不过期的会话 |
| `Secure` | 只在 HTTPS 下发送 | cookie 走明文 |
| `HttpOnly` | JavaScript 读不到它 | 脚本窃取 |
| `SameSite=Lax` | 跨站子请求不发送 | 跨站请求带上你的 cookie |
| `SameSite=Strict` | 只发同站请求 | 同上，更严格 |
| `SameSite=None` | 总是发送 —— 必须配 `Secure` | （正当的跨站用途才需要） |

#### Session

**Session** 是服务端的那份记录。cookie 里只有一个标识符，服务端拿它去查其余部分：

```
Cookie: session=abc123   ->   服务端存着 { user_id: 42, role: "user", cart: [...] }
```

好处是：客户端改不了它读不到的东西，服务端也能立刻让一个会话失效。代价是：服务端必须存这些会话。

#### Token

**Token**（JWT 是最常见的格式）反过来，把状态放在客户端：

```
Authorization: Bearer eyJhbGciOi...
```

一个 JWT 是三段 base64url —— 头部、载荷、签名 —— 用点号隔开。载荷**任何人都能读**；让它可信的是签名。好处是不需要服务端的会话存储；代价是撤销很难，因为"名单"不在服务端手上。

新手应该能张口说出这个区别：**session cookie 是一张取货凭证，token 是一张签过名的字条**。凭证能在柜台作废，字条不行 —— 只能等它过期。

### 第三部分：URL

```
https://user:pass@example.com:8443/a/b?x=1&y=2#frag
└─┬─┘ └───┬───┘ └────┬────┘└┬─┘└─┬─┘└────┬────┘└─┬─┘
协议   用户信息    主机   端口   路径     查询     片段
```

- **协议（scheme）** —— `http` 或 `https`。
- **路径（path）** —— 哪个资源。
- **查询（query）** —— 参数，`key=value` 用 `&` 分隔。
- **片段（fragment）** —— 只在客户端用，**永远不会发给服务端**。

最后这点反复绊倒人。**`#` 之后的东西，服务端从没见过。**

查询值会被 URL 编码，所以空格是 `%20`、中文会变成一长串 `%E4%B8%AD`。有些字符是安全的，有些必须编码 —— 具体规则在前面讲编码那一篇里。

### 第四部分：浏览器作为一个平台

#### HTML，最低限度的必要部分

HTML 文档是嵌套的元素。和安全相关的部分长这样：

```html
<form action="/login" method="post">
  <input type="text" name="username" value="">
  <input type="password" name="password" value="">
  <input type="hidden" name="csrf_token" value="abc123">
  <button type="submit">Sign in</button>
</form>
```

注意三件事，它们日后能解释很多：

- **`name`** 就是服务端收到的参数名，`value` 是它收到的值。
- **`method="post"`** 表示数据放在请求体里；默认的 `get` 会把数据放进 URL。
- **隐藏字段并不隐藏。** 它就在页面源码里，而客户端可以改任何一个字段。

#### DOM

浏览器把 HTML 解析成 **DOM** —— 一棵 JavaScript 可以读取和修改的对象树。这就是"页面"和"服务端发来的 HTML"不是一回事的原因：等你看它的时候，JavaScript 可能已经改写过它了。

在开发者工具里，Elements 面板显示的是**当前 DOM**，不是原始源码。想看原始源码就用"查看源代码"或者 `curl`。

#### 浏览器里的 JavaScript

JavaScript 对它所运行的这个页面可以做任何事：读改 DOM、发请求、存数据、读非 `HttpOnly` 的 cookie。

但 JavaScript **不能**随意读取其他源的数据。这条限制就是下一节。

#### 同源策略

一个**源（origin）** 恰好由三样东西组成：**协议 + 主机 + 端口**。

| 地址 A | 地址 B | 同源吗 |
|---|---|---|
| `https://example.com/a` | `https://example.com/b` | 是 |
| `https://example.com` | `http://example.com` | 否 —— 协议不同 |
| `https://example.com` | `https://www.example.com` | 否 —— 主机不同 |
| `https://example.com` | `https://example.com:8443` | 否 —— 端口不同 |

同源策略说的是：来自一个源的脚本，读不到另一个源的响应。这也是为什么随便一个网站读不了你的网页邮箱 —— cookie 可能被带上了，但响应读不出来。

**CORS** 是受控的例外。服务端可以用 `Access-Control-Allow-Origin` 说"我允许 X 这个源读我"，浏览器就会放行。对于不是"简单请求"的请求，浏览器会先发一个 **预检**（preflight）`OPTIONS` 请求去问许可。

两点值得记住：CORS 是**浏览器**在执行，不是服务端 —— 用 `curl` 的脚本完全无视它。而且 CORS 管的是"**能不能读响应**"，不是"能不能发请求"：一个跨站表单提交照样能到达。

### 第五部分：三个工具

#### 一、浏览器的开发者工具

按 F12 打开。新手需要这几个面板：

| 面板 | 用来做什么 |
|---|---|
| **Network** | 每一个请求，带头部、请求体、时序和状态。安全工作中最有用的面板 |
| **Elements** | 实时的 DOM 与 CSS |
| **Console** | 在页面上下文里执行 JavaScript |
| **Application / Storage** | Cookie、localStorage、sessionStorage、Service Worker |
| **Sources** | 给 JavaScript 下断点，也能实时改它 |

Network 面板第一天就该会的操作：**点开一个请求读它的头**（这是你看清实际发了哪些 cookie 和 token 的方式）、**右键 → Copy → Copy as cURL**（把请求搬到终端里复现）、**右键 → Edit and Resend**（改一个地方看会怎样）、以及 **Preserve log** 开关 —— 不打开它，一导航历史就清了。

#### 二、curl

同样的请求，用脚本发，可以在报告里复现：

```bash
curl -i https://example.com/                          # 连响应头一起看
curl -s -X POST https://example.com/login \
     -H 'Content-Type: application/json' \
     -d '{"username":"alice","password":"test"}'      # 发一个 JSON POST
curl -s -b 'session=abc123' https://example.com/me    # 带上 cookie
curl -sD - -o /dev/null https://example.com/          # 只要响应头
curl -s https://api.example.com/items | jq .          # 格式化 JSON
```

#### 三、拦截代理

开发者工具能看和改**一个**请求。代理则能**拦截全部**、在请求发出前把它暂停、修改它、重放它、对它做模糊测试，并且跨会话保留历史。你会遇到的两个是 Burp Suite 和 ZAP。

它的工作原理是：浏览器被配置成把流量发给代理，而不是直接发给服务端。因为 HTTPS 是端到端加密的，代理必须能解密，所以它会生成自己的证书，而你的浏览器必须信任它。这就是"安装代理的 CA 证书"是必经步骤的原因，也是它只能在你自己控制的机器上用的原因。

| 组件 | 用来做什么 |
|---|---|
| Proxy → Intercept | 暂停并修改飞行中的请求 |
| HTTP history | 发生过的一切，可搜索 |
| Repeater | 改一个请求并反复发送 |
| Intruder | 发大量变体（参数模糊测试、枚举） |
| Decoder | 不用离开工具就能编解码 |
| Comparer | 对比两个响应的差异 |

### 第六部分：端到端做一次

一个把前面所有东西串起来的练习。选你自己拥有的站点或一个靶场。

1. **终端**：`curl -v https://target/`，读请求和响应头。认出 `Content-Type`、`Set-Cookie` 和任何 `Cache-Control`。
2. **浏览器**：打开开发者工具，进 Network，打开 **Preserve log**，然后登录。找到那个登录请求。用的什么方法？`Content-Type` 是什么？请求体里口令是明文吗？回来的是 `302` 加 `Set-Cookie`，还是 `200` 加一个 JSON token？
3. **看存储**：在 Application 里找到 cookie 或 token。它有 `HttpOnly` 吗？`Secure` 吗？`SameSite` 吗？多久过期？
4. **复现它**：右键该请求 → Copy as cURL，粘到终端里，确认你拿到同样的响应。
5. **改一个地方**：用 Edit and Resend（或 Repeater）改一个值，看服务端怎么做。**这就是整个学科开窍的那一刻** —— 客户端完全在你的掌控之下，真正要紧的只有一件事：服务端决定怎么应对。

### 第七部分：API —— 现代的形态

大多数现代 Web 应用已经不是"页面"了，而是一个 JavaScript 前端在跟一个 JSON API 说话。协议相同，内容不同。

- **URL 标识资源**：`/api/users/42`。
- **方法是动词**：GET 读、POST 建、PUT/PATCH 改、DELETE 删。
- **状态码是结果。**
- **响应体是 JSON**：`{"id": 42, "name": "alice", "role": "user"}`。

你会遇到的认证方式：

| 方式 | 形状 | 备注 |
|---|---|---|
| Cookie | `Cookie: session=...` | 浏览器管理；属性见会话那一节 |
| Bearer token | `Authorization: Bearer eyJ...` | 常常是 JWT；无状态 |
| API key | `X-API-Key: ...` 或放在查询参数里 | 机器访问常用；长期有效 |
| Basic | `Authorization: Basic base64(user:pass)` | 只是编码，不是加密 —— 只能配 TLS 用 |

```bash
curl -s -H 'Authorization: Bearer eyJ...' https://api.example.com/me | jq .
```

### 第八部分：从这些知识直接推出的安全观念

仍然只是观念，不是手法 —— 具体的漏洞类别另有专篇。但懂了协议之后，有四件事会变得显而易见：

1. **客户端不是你的。** 浏览器里的一切 —— 校验、隐藏字段、JavaScript 检查、被禁用的按钮 —— 都可以改。如果服务端不再校验一遍，那就等于没有校验。
2. **隐藏不等于保密。** 页面源码、注释、JavaScript 文件、source map、旧版 API，全都可读。开发构建泄漏接口、密钥和注释是常态。
3. **没有 TLS 的 HTTP 就是公开的。** 路径上的任何人都能读到全部内容，包括 cookie 和口令。
4. **你发出去的一切，都是服务端必须处理的数据。** 参数名、参数值、请求头、内容类型、文件上传，全都来自一台你控制不了的机器。这就是"输入处理"为什么重要，也是后面那些漏洞篇的起点。

### 检测与缓解

- **把安全响应头配上，并且当真。** `Content-Security-Policy` 限制页面能加载和执行什么，`Strict-Transport-Security` 强制 HTTPS，`X-Content-Type-Options: nosniff` 阻止内容类型嗅探，`Referrer-Policy` 控制 Referer 泄漏什么。它们在 Web 服务器里各自只有一行，却能消掉一整类问题。
- **Cookie 属性是控制，不是装饰。** 每个会话 cookie 都上 `Secure`、`HttpOnly`、`SameSite`；生命周期要短；登录时轮换标识符。
- **永远在服务端校验。** 前端校验是易用性功能。写代码时就当客户端不存在 —— 因为从服务端的角度，它确实不存在。
- **不要把机密放进前端。** JavaScript 里的 API key、`localStorage` 里的 token、藏着价格的隐藏字段，全都是可读的。浏览器拿到的东西，用户就拿到了。
- **记日志时要带够上下文。** 方法、路径、状态码、认证到的用户、来源地址，以及一个请求 id。没有请求 id，你就无法把对不上的日志拼回一次事件。
- **监控流量的形状，而不只是数量。** 某个会话 4xx 突增、请求不存在的路径、异常的 `Content-Type`，这些都是"有人在探测"的便宜信号。
- **处处强制 HTTPS**，包括网络内部，让"待在路径上"这件事变得没有价值。
