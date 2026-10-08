---
id: lang-aspnet-iis
title_en: "ASP.NET and IIS — Merged Input, ViewState and Windows Paths"
title_zh: "ASP.NET 与 IIS：合并输入、ViewState 与 Windows 路径"
summary_en: ASP.NET hands the application a request object that merges four input sources, and IIS decides what a filename means on a filesystem whose conventions other platforms do not share. This entry covers source confusion, ViewState as a serialised object living in the client, the Windows path semantics behind classic upload bypasses, and how IIS resolves extensions.
summary_zh: ASP.NET 交给应用的是一个合并了四种输入来源的请求对象，而 IIS 在一个别的平台没有的文件系统约定之下，决定一个文件名是什么意思。这一篇讲来源混淆、讲住在客户端的序列化对象 ViewState、讲那些经典上传绕过背后的 Windows 路径语义，以及 IIS 怎么解析扩展名。
tags: [web, aspnet, iis, viewstate, source-confusion, file-upload-prep]
tools: [Burp Suite, curl, ysoserial.net, iis, PowerShell]
attck: [T1190, T1027]
platform: [web, windows]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why these two belong together

On IIS the application and the web server make separate decisions about the same request, and both of those decisions are shaped by conventions that do not exist on Linux stacks:

- **ASP and ASP.NET merge several input sources into one lookup.** A parameter's value depends on which source the *application* chose to read, which may not be the source a filter inspected.
- **IIS runs on Windows**, and Windows filenames have properties — trailing dots, alternate data streams, reserved device names, 8.3 short names — that a Unix-minded validation routine does not anticipate.

Put those together and you get the classic upload bypass: the application's check reads a filename and approves it, and IIS resolves that same filename to something executable. This entry is about those two mechanisms, not about the exploit techniques built on them.

### Source confusion: the request object that merges four things

In classic ASP, `Request("x")` without a collection name searches collections **in a fixed order** and returns the first match:

| Order | Collection |
|---|---|
| 1 | `QueryString` |
| 2 | `Form` |
| 3 | `Cookies` |
| 4 | `ClientCertificate` |
| 5 | `ServerVariables` |

ASP.NET's `Request["x"]` does the same across `QueryString`, `Form`, `Cookies` and `ServerVariables`.

Three consequences follow, and they are all security-relevant.

**1. A filter and the application can read different values.** A component that inspects only the query string, while the application reads `Request["x"]` and gets the form value, is inspecting something the application never uses:

```
POST /page.aspx?id=1                 <- a WAF inspects this
Body: id=1' OR 1=1--                 <- the application reads this
```

**2. Two components can read different sources deliberately.** If a proxy or middleware writes a security decision based on one source and the handler reads another, the gap is the vulnerability — the same shape as parameter pollution, with an extra layer because the sources are different collections rather than duplicate names in one.

**3. `ServerVariables` is not all server-generated.** IIS maps HTTP headers into `HTTP_*` variables, so:

```
Request.ServerVariables("HTTP_X_FORWARDED_FOR")   <- whatever the client sent
Request.ServerVariables("REMOTE_ADDR")            <- actually from the connection
```

Any application that trusts `HTTP_X_FORWARDED_FOR`, `HTTP_X_ORIGINAL_URL`, `HTTP_REFERER` or similar for an access decision is trusting a client-supplied header. The variable name says "server variable", and it is easy to read that as "the server decided this".

**The defence is explicit sourcing.** Every read should name its collection — `Request.QueryString["id"]` rather than `Request["id"]` — so that the answer does not depend on a search order. And any component that makes a decision should make it about the **same source** the handler will use; if that cannot be arranged, the decision has to be made at the point of use.

```csharp
// ambiguous: which source wins depends on the framework's order
string id = Request["id"];

// explicit: the answer cannot be influenced by putting the value somewhere else
string id = Request.QueryString["id"];

// if more than one source is legitimate, require that exactly one is present
var fromQuery = Request.QueryString["id"];
var fromForm  = Request.Form["id"];
if (fromQuery != null && fromForm != null) {
    throw new BadHttpRequestException("ambiguous parameter");
}
```

### ViewState: a serialised object that lives in the client

ASP.NET keeps control state in the page by **serialising it and sending it to the browser**, which posts it back with each request:

```html
<input type="hidden" name="__VIEWSTATE" value="/wEPDwUKMT..." />
<input type="hidden" name="__VIEWSTATEGENERATOR" value="CA0B0334" />
<input type="hidden" name="__EVENTVALIDATION" value="/wEdAA..." />
```

Two properties matter, and both are language-level facts about the framework rather than exploits:

**1. It is deserialised on the server.** The value comes back as bytes and is turned back into an object graph — historically with `LosFormatter`, and with `ObjectStateFormatter` in later versions. That makes ViewState **an entry point into .NET deserialisation**: if an attacker can supply the bytes and the framework accepts them, the object graph is constructed, and the class of bugs that follows is the subject of the deserialisation entry.

**2. Whether that is possible depends on the MAC and the key.** ViewState is normally signed so that tampering is detected. The relevant history:

- `EnableViewStateMac` could be **disabled** in older versions, which removed the integrity check entirely.
- Since .NET 4.5.2 it is **enforced** — the framework refuses to run with it turned off — which closed that particular door.
- What remains is the **key**: ViewState signing uses `machineKey`, and if that key is a hardcoded value shared across a deployment, or has been disclosed, an attacker can produce a **validly signed** ViewState. That combination has produced real remote code execution: not by breaking the MAC, but by **knowing the key**.

So the practical questions when you see `__VIEWSTATE` are: **is it signed, and does anyone outside the application know the key?** A default or documented `machineKey` in `web.config` is the finding, and it is a configuration finding rather than a parsing one.

```xml
<!-- the dangerous shape: a fixed key committed to source control -->
<machineKey validationKey="..." decryptionKey="..." />

<!-- the safe shape: generated per application, and never shared or committed -->
<machineKey validation="HMACSHA256" decryption="AES" />
```

`__VIEWSTATEGENERATOR` and `__EVENTVALIDATION` are also worth recognising: the first identifies the page's control tree, and the second is a validation token whose presence tells you something about the framework version in use.

### Windows path semantics

This is where upload bypasses live, and **each of these is a real Windows behaviour rather than a trick**.

| Behaviour | What it means | Why validation misses it |
|---|---|---|
| **Trailing dots and spaces are stripped** | `shell.aspx.` and `shell.aspx ` both resolve to `shell.aspx` | A check comparing the literal string sees a different name than the filesystem does |
| **Alternate data streams** | `file.aspx::$DATA` addresses the default data stream of `file.aspx` | The `::$DATA` suffix is not part of the filename to Windows, but it looks like one to a string check |
| **Reserved device names** | `CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9` can be interpreted as devices rather than files, depending on how the path reaches the OS | A filename allowlist that does not account for them can behave unexpectedly |
| **Case-insensitive comparison** | `Shell.ASPX` is `shell.aspx` | A case-sensitive extension check passes on a filesystem that will execute it |
| **8.3 short names** | Long names have a generated short alias like `SHELL~1.ASP` | A check that only considers the long name misses that both names refer to one file |
| **Both separators** | `\` and `/` are both separators on Windows | A normalisation that only handles `/` is incomplete here |

```powershell
# trailing dot: the name that lands on disk is not the name that was requested
Set-Content -Path '.tmp\wtest\shell.aspx.' -Value 'x'
Get-ChildItem '.tmp\wtest' | Select-Object -ExpandProperty Name      # shell.aspx

# alternate data stream: one file, two contents
Set-Content -Path '.tmp\wtest\report.txt' -Value 'visible'
Add-Content -Path '.tmp\wtest\report.txt' -Stream 'note' -Value 'hidden stream'
Get-Content  -Path '.tmp\wtest\report.txt'                    # visible
Get-Content  -Path '.tmp\wtest\report.txt' -Stream 'note'     # hidden stream
Get-Item     -Path '.tmp\wtest\report.txt' -Stream * | Select-Object Stream, Length

# case-insensitivity: one file, two spellings
Get-Item '.tmp\wtest\REPORT.TXT' | Select-Object -ExpandProperty Name
```

Each line above is a place where **"the name the application checked" and "the name the filesystem used" can differ**, which is the same root cause as every other entry in this series, expressed in file naming rather than in characters.

### How IIS resolves extensions

The application decides whether to store a file; **IIS decides whether to execute it**, and the rules are not the same rules.

| Pattern | What happens | Where it works |
|---|---|---|
| `shell.aspx` | Executed by the ASP.NET handler | Any IIS |
| `shell.jpg.aspx` | The last extension maps to the handler — executed | Any IIS with default config, if the upload was allowed |
| `shell.aspx.jpg` | Depends: an application check reading the last extension sees an image; the handler mapping may still match `.aspx` | Configuration dependent |
| `shell.asp;.jpg` | IIS 6 truncated at the semicolon and executed `shell.asp` | **IIS 6 only** — fixed in IIS 7 |
| `shell.aspx:.jpg` | The `:` starts an alternate data stream, so the file is `shell.aspx` | Windows, where ADS is permitted |
| `shell.aspx::$DATA` | Default data stream of `shell.aspx`; served as the file's content | Windows |
| `shell.aspx.` | Trailing dot stripped → `shell.aspx` | Windows |
| `shell.aspx/` or `shell.aspx/.jpg` | Path info handling depends on configuration | Configuration dependent |
| `SHELL~1.ASP` | The short name of a long filename | Windows with 8.3 names enabled |

Two structural points follow:

**The extension that the application checks is often not the extension the server acts on.** That is why "we only allow `.jpg`" is not a control unless the check reads the same token the handler will. Checking the last extension while the handler matches any extension is the classic mismatch.

**IIS 7 and later closed the semicolon truncation, and did not close the rest.** Trailing dots, alternate data streams, short names and case-insensitivity are properties of the filesystem and the platform, not bugs that were patched, so they are still in play on current servers.

IIS also has its own knobs that decide how much of this is reachable:

| Setting | Effect |
|---|---|
| `<requestFiltering>` → `fileExtensions` | Which extensions may be requested at all |
| `allowDoubleEscaping` | Whether `%2e%2e` and similar are decoded twice — **default false**, and setting it to true is a common way to reintroduce traversal |
| `<httpRuntime enableVersionHeader>` | Whether `X-AspNet-Version` is disclosed |
| `<customErrors mode>` | Whether stack traces reach the client |
| Application pool identity | What the process can write and execute — the backstop for everything above |

### Other ASP.NET features worth knowing

| Feature | What it does | Why it matters |
|---|---|---|
| `Server.Execute`, `Server.Transfer` | Run another page in the same request | Server-side inclusion; a user-controlled path is dangerous |
| `MapPath` | Turn a virtual path into a physical one | The containment check has to be applied **after** mapping |
| `HttpUtility.UrlDecode` | Decode a string | Calling it twice is the double-decoding bug from the encoding entry, in .NET form |
| `<%= %>` and data binding | Emit values into the response | Escaping depends on the control used, not on the language |
| `Response.Redirect` | Send a redirect | Header injection if a value reaches the `Location` header unvalidated |
| `machineKey` | Signing and encryption key for ViewState and forms auth | Shared, hardcoded or disclosed keys defeat both |

### Recognising the stack

| Signal | What it tells you |
|---|---|
| `Server: Microsoft-IIS/10.0` | IIS version, and therefore which of the above are historical |
| `X-Powered-By: ASP.NET` | ASP.NET, unless suppressed |
| `X-AspNet-Version: 4.0.30319` | Exact framework version, if `enableVersionHeader` is on |
| `ASP.NET_SessionId` cookie | ASP.NET session |
| `__VIEWSTATE`, `__EVENTVALIDATION`, `__RequestVerificationToken` in a form | A strong fingerprint, and the first two are the ViewState entry point |
| `.aspx`, `.ashx`, `.asmx`, `.svc`, `.axd` | ASP.NET, with `.ashx` and `.asmx` worth noting as endpoints that take parameters directly |
| Detailed IIS error pages | Misconfiguration, and information disclosure |

### Detection and mitigation

- **Read every parameter from a named source, and make the decision at the same place it is used.** `Request["x"]` is the ambiguity; `Request.QueryString["x"]` is a decision. Where several sources are legitimate, require exactly one to be present rather than letting a framework's search order decide.
- **Never make an access decision from a header-derived server variable.** `HTTP_X_FORWARDED_FOR`, `HTTP_REFERER` and friends come from the client; treat them as input, and if a proxy is genuinely in front, trust only the value the proxy rewrites.
- **Treat ViewState as untrusted input that happens to be signed.** Keep `EnableViewStateMac` enforced, enable ViewState encryption where the state is sensitive, and make sure `machineKey` is **generated per application and never shared, hardcoded or committed**. A leaked or default key turns a signed blob into an attacker-controlled deserialisation input, and that is a configuration finding worth reporting on its own.
- **Canonicalise filenames before deciding anything about them.** Strip trailing dots and spaces the way the filesystem will, resolve short names, handle `::$DATA` and ADS, and compare case-insensitively on Windows. The check must operate on the name the filesystem will actually create, not on the string that arrived.
- **Decide execution in one place, and make it the place that serves the file.** The application's extension check and IIS's handler mapping are two different policies; a single policy applied after canonicalisation, at the point of storage, is what closes the gap. Better still, **do not store uploads under a name derived from the client at all** — generate a name, keep the original only as metadata, and store outside the web root where nothing is executable.
- **Harden IIS explicitly.** `requestFiltering` with the extensions you actually serve listed, `allowDoubleEscaping=false`, version headers off, custom errors on, and an application pool identity that cannot execute what it writes. Every one of these removes a technique rather than a payload.
- **Alert on requests shaped like a Windows bypass attempt**: filenames containing `::`, a trailing dot or space, a semicolon before an extension, `~` followed by a digit, or a second extension after an allowed one. None is proof of an attack and all of them are cheap, and they are exactly the inputs that look harmless to a Linux-oriented filter.
- **And log the filename at each layer.** Client-supplied name, post-canonicalisation name, the name written to disk, and the path served. On Windows those four can all differ, and the difference between any two of them is the bug.

<!-- lang:zh -->
### 为什么这两个要放在一起

在 IIS 上，应用和 Web 服务器对同一个请求各自做决定，而这两个决定都受一些 Linux 技术栈上不存在的约定影响：

- **ASP 与 ASP.NET 把好几种输入来源合并成一次查找。** 一个参数的值取决于**应用**选择读哪个来源，而那个来源可能不是某个过滤器检查的那个。
- **IIS 跑在 Windows 上**，而 Windows 的文件名有一些 Unix 思维写出的校验不会预料到的性质 —— 末尾的点、备用数据流、保留设备名、8.3 短名。

把这两点合起来，就得到经典的上传绕过：**应用的检查读了一个文件名并放行，而 IIS 把同一个文件名解析成了可执行的东西。** 这一篇讲的就是这两个机制，不是建立在它们之上的利用手法。

### 来源混淆：那个合并了四样东西的请求对象

在经典 ASP 里，不带集合名的 `Request("x")` 会**按固定顺序**搜索各个集合，返回第一个匹配：

| 顺序 | 集合 |
|---|---|
| 1 | `QueryString` |
| 2 | `Form` |
| 3 | `Cookies` |
| 4 | `ClientCertificate` |
| 5 | `ServerVariables` |

ASP.NET 的 `Request["x"]` 在 `QueryString`、`Form`、`Cookies` 与 `ServerVariables` 之间做同样的事。

由此产生的三个后果，都与安全相关。

**一、过滤器与应用可以读到不同的值。** 一个只检查查询串的组件，而应用读 `Request["x"]` 拿到的是表单值，那这个组件检查的是应用从不使用的东西：

```
POST /page.aspx?id=1                 <- WAF 检查的是这里
Body: id=1' OR 1=1--                 <- 应用读的是这里
```

**二、两个组件可以有意地读不同来源。** 如果某个代理或中间件基于一个来源做安全决策，而处理器读另一个来源，那条缝就是漏洞 —— 和参数污染同一个形状，只是多了一层：这些来源是不同的集合，而不是同一个集合里的重复参数名。

**三、`ServerVariables` 并不全是服务器产生的。** IIS 会把 HTTP 头映射成 `HTTP_*` 变量，于是：

```
Request.ServerVariables("HTTP_X_FORWARDED_FOR")   <- 客户端发什么就是什么
Request.ServerVariables("REMOTE_ADDR")            <- 真的来自连接
```

任何因为访问决策而信任 `HTTP_X_FORWARDED_FOR`、`HTTP_X_ORIGINAL_URL`、`HTTP_REFERER` 之类东西的应用，都是在信任一个客户端提供的头。名字叫"服务器变量"，很容易被读成"这是服务器决定的"。

**防御是显式指定来源。** 每次读取都应该点明集合 —— `Request.QueryString["id"]` 而不是 `Request["id"]` —— 这样答案就不取决于某种搜索顺序。而任何一个做决定的组件，都应该针对**处理器将会使用的那个来源**做决定；如果这安排不了，那决定就得在使用点上做。

```csharp
// 含糊：哪个来源胜出取决于框架的顺序
string id = Request["id"];

// 明确：答案无法通过把值放到别处来影响
string id = Request.QueryString["id"];

// 如果多个来源都合法，就要求恰好只有一个存在
var fromQuery = Request.QueryString["id"];
var fromForm  = Request.Form["id"];
if (fromQuery != null && fromForm != null) {
    throw new BadHttpRequestException("ambiguous parameter");
}
```

### ViewState：住在客户端的序列化对象

ASP.NET 把控件状态留在页面里的方式是**把它序列化并送到浏览器**，随每次请求再发回来：

```html
<input type="hidden" name="__VIEWSTATE" value="/wEPDwUKMT..." />
<input type="hidden" name="__VIEWSTATEGENERATOR" value="CA0B0334" />
<input type="hidden" name="__EVENTVALIDATION" value="/wEdAA..." />
```

有两个性质要紧，而它们都是关于这个框架的语言层面事实，而不是利用手法：

**一、它会在服务端被反序列化。** 那个值作为字节回来，然后被变回一个对象图 —— 历史上用 `LosFormatter`，较新版本用 `ObjectStateFormatter`。这让 ViewState **成为 .NET 反序列化的一个入口**：如果攻击者能提供那些字节、而框架接受了它们，对象图就会被构造出来，而随之而来的那一类 bug 是反序列化那一篇的主题。

**二、能不能做到，取决于 MAC 和密钥。** ViewState 通常会被签名，以便检测篡改。相关的历史：

- 老版本里 `EnableViewStateMac` 可以**被关掉**，那等于完全没有完整性检查。
- 从 .NET 4.5.2 起它是**强制**的 —— 框架拒绝在关闭它的状态下运行 —— 那扇门被关上了。
- 剩下的是**密钥**：ViewState 的签名用 `machineKey`，如果那个密钥是硬编码的、在整个部署里共用的，或者已经被泄漏，攻击者就能造出**签名有效**的 ViewState。这个组合产生过真实的远程代码执行：不是破掉了 MAC，而是**知道那个密钥**。

所以看到 `__VIEWSTATE` 时，实际要问的是：**它被签名了吗，以及应用之外有谁知道那个密钥？** `web.config` 里一个默认或写在文档里的 `machineKey` 就是发现项，而且它是一条**配置**发现，不是解析发现。

```xml
<!-- 危险的形状：一个提交进了版本控制的固定密钥 -->
<machineKey validationKey="..." decryptionKey="..." />

<!-- 安全的形状：每个应用各自生成，不共享、不提交 -->
<machineKey validation="HMACSHA256" decryption="AES" />
```

`__VIEWSTATEGENERATOR` 与 `__EVENTVALIDATION` 也值得认得：前者标识页面的控件树，后者是一个校验令牌，它的存在能透露所用的框架版本。

### Windows 路径语义

上传绕过就住在这里，而**下面每一条都是真实的 Windows 行为，不是什么花招**。

| 行为 | 它的含义 | 校验为什么会漏 |
|---|---|---|
| **末尾的点和空格会被剥掉** | `shell.aspx.` 与 `shell.aspx ` 都解析到 `shell.aspx` | 做字面字符串比较的检查看到的，和文件系统看到的是不同的名字 |
| **备用数据流** | `file.aspx::$DATA` 指向 `file.aspx` 的默认数据流 | 对 Windows 来说 `::$DATA` 不是文件名的一部分，但字符串检查看起来像 |
| **保留设备名** | `CON`、`PRN`、`AUX`、`NUL`、`COM1`–`COM9`、`LPT1`–`LPT9` 可能被解释成设备而不是文件，取决于路径怎么到达操作系统 | 一份没考虑它们的文件名白名单，行为可能出人意料 |
| **大小写不敏感的比较** | `Shell.ASPX` 就是 `shell.aspx` | 区分大小写的扩展名检查，在一个会执行它的文件系统上通过了 |
| **8.3 短名** | 长名字会有生成的短别名，如 `SHELL~1.ASP` | 只看长名字的检查，漏掉了两个名字指的是同一个文件 |
| **两种分隔符** | Windows 上 `\` 与 `/` 都是分隔符 | 只处理 `/` 的规范化在这里是不完整的 |

```powershell
# 末尾的点：落到磁盘上的名字并不是请求的那个名字
Set-Content -Path '.tmp\wtest\shell.aspx.' -Value 'x'
Get-ChildItem '.tmp\wtest' | Select-Object -ExpandProperty Name      # shell.aspx

# 备用数据流：一个文件，两份内容
Set-Content -Path '.tmp\wtest\report.txt'      -Value 'visible'
Set-Content -Path '.tmp\wtest\report.txt:note' -Value 'hidden stream'
Get-Content  -Path '.tmp\wtest\report.txt'                    # visible
Get-Content  -Path '.tmp\wtest\report.txt:note'               # hidden stream
Get-Item     -Path '.tmp\wtest\report.txt' -Stream * | Select-Object Stream, Length

# 大小写不敏感：一个文件，两种拼法
Get-Item '.tmp\wtest\REPORT.TXT' | Select-Object -ExpandProperty Name
```

上面每一行都是一个"**应用检查的名字**"与"**文件系统使用的名字**"可能不同的地方 —— 和这个系列里其他每一篇同一个根因，只是这里体现在文件命名上，而不是字符上。

### IIS 怎么解析扩展名

应用决定要不要存下一个文件；**IIS 决定要不要执行它** —— 而两套规则不是同一套。

| 模式 | 会发生什么 | 在哪里有效 |
|---|---|---|
| `shell.aspx` | 由 ASP.NET 处理器执行 | 任何 IIS |
| `shell.jpg.aspx` | 最后一个扩展名映射到处理器 —— 被执行 | 只要上传被放行，默认配置下都有效 |
| `shell.aspx.jpg` | 看情况：读最后一个扩展名的应用检查看到图片；而处理器映射仍可能匹配 `.aspx` | 取决于配置 |
| `shell.asp;.jpg` | IIS 6 在分号处截断并执行 `shell.asp` | **仅 IIS 6** —— IIS 7 已修复 |
| `shell.aspx:.jpg` | `:` 开启一个备用数据流，于是文件是 `shell.aspx` | Windows 上（允许 ADS 时） |
| `shell.aspx::$DATA` | `shell.aspx` 的默认数据流；按文件内容提供 | Windows |
| `shell.aspx.` | 末尾点被剥掉 → `shell.aspx` | Windows |
| `shell.aspx/` 或 `shell.aspx/.jpg` | 路径信息怎么处理取决于配置 | 取决于配置 |
| `SHELL~1.ASP` | 某个长文件名的短名 | 启用了 8.3 名字的 Windows |

由此有两个结构性要点：

**应用检查的那个扩展名，常常不是服务器据以行动的那个扩展名。** 这就是为什么"我们只允许 `.jpg`"不是一个控制，除非那个检查读的和处理器匹配的是同一个记号。**读最后一个扩展名、而处理器匹配任意扩展名**，就是那个经典的错配。

**IIS 7 及以后关闭了分号截断，但没有关闭其余的。** 末尾点、备用数据流、短名、大小写不敏感，都是文件系统与平台的性质，不是被修补掉的 bug，所以它们在当前的服务器上依然有效。

IIS 还有一些自己的开关，决定上面这些有多少够得着：

| 设置 | 效果 |
|---|---|
| `<requestFiltering>` → `fileExtensions` | 哪些扩展名可以被请求 |
| `allowDoubleEscaping` | `%2e%2e` 之类是否被解码两次 —— **默认 false**，把它设成 true 是重新引入路径穿越的常见方式 |
| `<httpRuntime enableVersionHeader>` | 是否暴露 `X-AspNet-Version` |
| `<customErrors mode>` | 调用栈是否到达客户端 |
| 应用程序池身份 | 进程能写什么、能执行什么 —— 上面一切的兜底 |

### 其他值得知道的 ASP.NET 特性

| 特性 | 它做什么 | 为什么重要 |
|---|---|---|
| `Server.Execute`、`Server.Transfer` | 在同一个请求里运行另一个页面 | 服务端包含；用户可控的路径很危险 |
| `MapPath` | 把虚拟路径变成物理路径 | 包含检查必须**在映射之后**做 |
| `HttpUtility.UrlDecode` | 解码字符串 | 调两次就是编码那篇里的二次解码 bug 的 .NET 版本 |
| `<%= %>` 与数据绑定 | 把值输出到响应 | 是否转义取决于所用的控件，而不是语言 |
| `Response.Redirect` | 发一个重定向 | 若某个值未经校验到达 `Location` 头，就是头注入 |
| `machineKey` | ViewState 与表单认证的签名/加密密钥 | 共享、硬编码或已泄漏的密钥会让两者都失效 |

### 识别这个技术栈

| 信号 | 它告诉你什么 |
|---|---|
| `Server: Microsoft-IIS/10.0` | IIS 版本，由此可知上面哪些属于历史问题 |
| `X-Powered-By: ASP.NET` | ASP.NET（除非被屏蔽） |
| `X-AspNet-Version: 4.0.30319` | 精确的框架版本（若 `enableVersionHeader` 开着） |
| `ASP.NET_SessionId` cookie | ASP.NET 会话 |
| 表单里的 `__VIEWSTATE`、`__EVENTVALIDATION`、`__RequestVerificationToken` | 很强的指纹，而前两个就是 ViewState 的入口 |
| `.aspx`、`.ashx`、`.asmx`、`.svc`、`.axd` | ASP.NET；其中 `.ashx`、`.asmx` 值得注意，因为它们直接接受参数 |
| 详细的 IIS 错误页 | 配置有问题，同时也是信息泄漏 |

### 检测与缓解

- **每个参数都从具名来源读，并且把决定做在它被使用的地方。** `Request["x"]` 是那个含糊；`Request.QueryString["x"]` 是一个决定。当多个来源都合法时，就要求恰好只有一个存在，而不是让框架的搜索顺序替你决定。
- **绝不要用一个来自请求头的服务器变量做访问决策。** `HTTP_X_FORWARDED_FOR`、`HTTP_REFERER` 之类来自客户端；把它们当输入，而且如果前面确实有代理，只信任代理重写过的那个值。
- **把 ViewState 当成"恰好被签了名的不可信输入"。** 保持 `EnableViewStateMac` 强制开启；状态敏感时开启 ViewState 加密；并确保 `machineKey` **每个应用各自生成、从不共享、不硬编码、不提交**。一个泄漏或默认的密钥，会把一段签名过的数据变成攻击者可控的反序列化输入 —— 而那本身就是一条值得单独上报的配置发现。
- **在对文件名做任何决定之前先规范化它。** 按文件系统的方式剥掉末尾的点和空格、解析短名、处理 `::$DATA` 与 ADS、在 Windows 上按不区分大小写比较。检查必须作用于**文件系统真正会创建的那个名字**，而不是到达的那个字符串。
- **执行与否的决定只能有一个地方做，而且必须是提供服务的那一处。** 应用的扩展名检查与 IIS 的处理器映射是两套不同策略；一套在规范化之后、在存储点上应用的策略，才是关上那条缝的东西。更好的做法是：**上传文件根本不要用来自客户端的名字来存** —— 生成一个名字，把原名只留作元数据，并存到 Web 根目录之外、任何东西都不可执行的地方。
- **显式加固 IIS。** 用 `requestFiltering` 只列出你真的会提供的扩展名；`allowDoubleEscaping=false`；版本头关掉；自定义错误打开；应用程序池身份不能执行自己写下的东西。每一条移除的是一种技术，而不是一个 payload。
- **对"像 Windows 绕过尝试"的请求告警**：文件名含 `::`、末尾有点或空格、扩展名前有分号、`~` 后面跟数字，或者在一个被允许的扩展名之后又跟了一个扩展名。没有一条能证明是攻击，而每一条都很便宜，而且它们恰恰是对一个面向 Linux 的过滤器看起来无害的输入。
- **并且在每一层记下文件名。** 客户端提供的名字、规范化之后的名字、写进磁盘的名字、实际提供服务的路径。在 Windows 上这四个**可以全都不一样**，而其中任意两个的差异就是 bug。
