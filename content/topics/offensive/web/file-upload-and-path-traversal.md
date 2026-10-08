---
id: file-upload-and-path-traversal
title_en: File Upload and Path Traversal
title_zh: 文件上传与路径穿越
summary_en: Two bugs that live next to each other. Upload asks whether you can get a file somewhere useful; traversal asks whether a path parameter will walk out of where it was meant to stay. Together they are the shortest route from a web form to a shell.
summary_zh: 这是两个常挨在一起的漏洞。上传问的是"你能把文件放到有用的地方吗"；穿越问的是"路径参数会不会走出它本该待的地方"。两者合起来，就是从 Web 表单到 shell 最短的一条路。
tags: [web, file-upload, path-traversal, lfi, rce, bugbounty]
tools: [Burp Suite, ffuf, weevely, php_filter_chain_generator]
attck: [T1190, T1505.003]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Upload: three questions, in order

An upload feature is only a vulnerability if the chain completes. Ask in this order, and stop when the answer is no:

1. **Can I get a file onto the server?** Almost always yes.
2. **Can I reach it afterwards?** A file in a directory the web server does not serve is a file you cannot use — unless you have traversal or an include.
3. **Will it execute?** This is where the money is, and where the defences are.

A feature that answers yes to 1 and 2 but stores everything in object storage with a fixed `Content-Type` is a finding for stored XSS at best. A feature that answers yes to all three is remote code execution.

### Bypassing the checks, in the order they usually appear

| Check | Bypass |
|---|---|
| Client-side JavaScript only | Intercept and change the request; the check never reaches the server |
| Content-Type allow-list | Send `image/png` with a PHP body |
| Extension blocklist | Case (`pHP`), double extension (`x.php.jpg`), alternative extensions (`.phtml`, `.php5`, `.phar`, `.pht`), trailing dot or space, `%00`, `;` on IIS |
| Magic-byte check | Prepend `GIF89a;` or a real PNG header, keep the payload after it |
| Image re-encoding | Put the payload in EXIF, in an SVG, or in a ZIP inside the image |
| Filename sanitisation | Path traversal in the filename, or a long name that breaks the sanitizer |
| Size or type restriction only | Split across multiple files, or use a second feature |

Two configuration quirks are worth memorising because they turn a "safe" extension into code:

- **Apache** with `AddHandler` multi-extension behaviour may execute `shell.php.jpg`.
- **`.htaccess` or `web.config`** uploaded into an executable directory reconfigures the server: you can add a new handler and then any extension you like becomes PHP.

### After the upload

Where the uploaded file lands decides everything:

```bash
# a web shell, if it lands in a served directory
echo '<?php system($_GET["c"]); ?>' > shell.php
curl 'https://target/uploads/shell.php?c=id'
```

If it does not land where it can be served, look for the second half of the chain: a local file inclusion that will include your file, an export or import job that reads it, or an image processor that runs a command on it.

Formats that are dangerous without ever executing:

- **SVG** — it is XML, so it can carry JavaScript (stored XSS with the site's origin) and external entities (XXE).
- **HTML** — served from the same origin, it is a same-origin XSS and can read non-`HttpOnly` cookies.
- **ZIP/TAR** — if the server extracts it, entries named `../../etc/cron.d/x` are a write primitive anywhere on the filesystem (zip slip).
- **Office documents** — the classic macro and XXE surface on the processing pipeline.

### Path traversal: the encoding is the whole game

The bug is trivial (`../../../../etc/passwd`); getting past the filters is not. Try, in roughly this order:

```text
../                     plain
..%2f  %2e%2e%2f        URL encoded
..%252f                 double encoded
..\  ..%5c              Windows separators
....//                  filter removes one ../ and leaves one
/etc/passwd             absolute path, when the prefix is concatenated
%00                     null byte, on old runtimes
```

Then look for where the application *adds* something you must defeat:

- A **prefix** (`/var/www/uploads/` + input): traversal still works, you just walk out of it.
- A **suffix** (input + `.jpg`): use a null byte on old stacks, a path parameter (`;`, `#`), or a directory that already ends in the extension.
- A **resolved base path**: check whether it is resolved *before* or *after* your input is appended — this is the classic difference between two identical-looking code paths.

Frameworks and servers add their own quirks worth knowing: Nginx `alias` with a missing trailing slash (off-by-slash) turns `/static../` into a traversal; Spring and Tomcat normalise `..;` differently; and some stacks decode twice. When a filter looks solid, the bug is usually in the difference between the validator and the consumer.

### From traversal to code execution

Reading `/etc/passwd` proves the bug; these turn it into impact:

- **Log poisoning.** Write PHP into a header (the User-Agent), then include the access log through the LFI. Two requests, one shell.
- **Session files.** Write the payload into a session value, then include `/tmp/sess_<id>`.
- **Upload + include.** Upload the payload as a "jpg" and include it — the extension no longer matters because the server is including, not serving.
- **PHP filter chains.** With a single LFI and no file of your own to include, `php://filter` chains can synthesise arbitrary content, turning the include into RCE without writing anything to disk.

```bash
# a filter chain that builds a payload in memory and includes it
python3 php_filter_chain_generator.py --chain '<?php system($_GET["c"]); ?>'
```

That last one is worth internalising: a "read-only" LFI in a PHP application is very often code execution.

### Detection

- **Uploads outside the expected directory**, or uploads whose stored name does not match the declared content type.
- Files that were uploaded and then **requested within seconds** — the typical webshell workflow.
- **Executable files in upload directories**: any `.php`, `.jsp`, `.aspx` under `/uploads/`, and any `.htaccess` or `web.config` anywhere it was not deployed by CI.
- **Traversal patterns in URL and parameter logs**: `../`, encoded variants, `/etc/passwd`, `php://filter`, and `win.ini`.
- **Processes spawned by the web server** (`www-data → sh`, `w3wp.exe → cmd.exe`) — the signature that a web shell ran.
- File-integrity monitoring on web roots: an unexpected new file is the alarm.

### Mitigation

- **Store uploads outside the web root** and serve them through an application route that sets the content type explicitly; then a PHP file in storage is just bytes.
- **Do not rely on extension blocklists.** Use an allow-list, verify the real content type by parsing the file, and rename the stored file to a generated name with a fixed extension.
- **Strip or encode everything else** — filenames, metadata, EXIF — and never trust the client's `Content-Type`.
- **Disable script execution in upload directories** at the server level, and treat it as defence in depth, not the primary control.
- **Resolve and validate paths after decoding**, compare against a base directory with a real path check (not a string prefix), and reject anything that escapes it.
- **Do not fetch or extract archives** without validating each entry's final path (zip slip), and run extraction in a sandbox.
- **Run the web server as an unprivileged user** with a read-only application directory, so an upload cannot overwrite code, and a traversal cannot read the whole filesystem.
- **Alert on the combination**, because the two halves are individually noisy but together they are unambiguous: an upload followed by a request to the uploaded path, or an include parameter pointing at a file that arrived moments earlier.

<!-- lang:zh -->
### 上传：按顺序问三个问题

只有整条链走通时，上传功能才算漏洞。按顺序问，一旦某个答案是"否"就停下：

1. **我能把文件弄到服务器上吗？** 几乎总是能。
2. **事后我能访问到它吗？** 放在 Web 服务器不提供的目录里，你就用不了它 —— 除非你还有穿越或文件包含。
3. **它会执行吗？** 钱在这里，防御也在这里。

只满足第 1、2 条、但把文件存进对象存储并固定 `Content-Type` 的功能，最多算个存储型 XSS。三条全中的功能，就是远程代码执行。

### 绕过校验，按它们通常出现的顺序

| 校验 | 绕过方式 |
|---|---|
| 只有前端 JavaScript | 拦截并修改请求；那个校验根本到不了服务端 |
| Content-Type 白名单 | 正文是 PHP，却发 `image/png` |
| 扩展名黑名单 | 大小写（`pHP`）、双扩展名（`x.php.jpg`）、替代扩展名（`.phtml`、`.php5`、`.phar`、`.pht`）、结尾的点或空格、`%00`、IIS 上的 `;` |
| magic bytes 校验 | 前面垫 `GIF89a;` 或真实 PNG 头，payload 跟在后面 |
| 图片重编码 | 把 payload 放进 EXIF、SVG，或者图片里嵌的 ZIP |
| 文件名净化 | 文件名里做路径穿越，或者用超长文件名把净化逻辑搞崩 |
| 只限制体积或类型 | 拆成多个文件，或者换另一个功能 |

有两处配置怪癖值得背下来，因为它们能让"安全"的扩展名变成代码：

- **Apache** 在 `AddHandler` 多扩展名行为下，可能会执行 `shell.php.jpg`。
- 把 **`.htaccess` 或 `web.config`** 传进可执行目录，等于重新配置服务器：你可以加上新的 handler，然后随便什么扩展名都变成 PHP。

### 上传之后

文件落在哪，决定一切：

```bash
# 如果它落进了会被提供的目录，就是一个 webshell
echo '<?php system($_GET["c"]); ?>' > shell.php
curl 'https://target/uploads/shell.php?c=id'
```

如果它落不到能被访问的位置，就去找链条的另一半：一个能包含你文件的本地文件包含、一个会读取它的导入导出任务，或者一个会对它执行命令的图片处理器。

不执行也算危险的格式：

- **SVG** —— 它就是 XML，所以能携带 JavaScript（以站点源为上下文的存储型 XSS）和外部实体（XXE）。
- **HTML** —— 从同源提供，就是同源 XSS，能读非 `HttpOnly` 的 cookie。
- **ZIP/TAR** —— 如果服务端会解压，名为 `../../etc/cron.d/x` 的条目就是一个能在文件系统任意位置写入的原语（zip slip）。
- **Office 文档** —— 处理链路上经典的宏与 XXE 面。

### 路径穿越：编码就是全部

漏洞本身很简单（`../../../../etc/passwd`），难的是过掉过滤器。大致按这个顺序试：

```text
../                     明文
..%2f  %2e%2e%2f        URL 编码
..%252f                 双重编码
..\  ..%5c              Windows 分隔符
....//                  过滤器删掉一个 ../ 又留下一个
/etc/passwd             绝对路径（当前缀是拼接上去的时候）
%00                     空字节，在老运行时上
```

然后找出应用**额外拼了什么**，你需要打败它：

- **前缀**（`/var/www/uploads/` + 输入）：穿越照样有效，你只是从它里面走出去。
- **后缀**（输入 + `.jpg`）：在老技术栈上用空字节、用路径参数（`;`、`#`），或者找一个本身就以该扩展名结尾的目录。
- **已解析的基础路径**：确认它是在你的输入拼上**之前**还是**之后**解析的 —— 这正是两条看起来一模一样的代码路径之间的经典差异。

框架与服务器还有些值得知道的怪癖：Nginx 的 `alias` 少写结尾斜杠（off-by-slash）会让 `/static../` 变成穿越；Spring 和 Tomcat 对 `..;` 的归一化方式不同；有些技术栈会解码两次。当一个过滤器看起来无懈可击时，洞通常在**校验者与使用者之间的差异**里。

### 从穿越到代码执行

读到 `/etc/passwd` 只是证明漏洞，下面这些才把它变成影响：

- **日志投毒。** 把 PHP 写进请求头（User-Agent），再通过 LFI 包含访问日志。两次请求，一个 shell。
- **会话文件。** 把 payload 写进某个会话值，再包含 `/tmp/sess_<id>`。
- **上传 + 包含。** 把 payload 当"jpg"传上去再包含它 —— 扩展名不再重要，因为服务器是在包含，而不是在提供。
- **PHP filter chain。** 只有一个 LFI、手上没有可包含的文件时，`php://filter` 链可以在内存里合成任意内容，把包含变成 RCE，且不需要往磁盘写任何东西。

```bash
# 用 filter chain 在内存里拼出 payload 并包含它
python3 php_filter_chain_generator.py --chain '<?php system($_GET["c"]); ?>'
```

最后这条值得记住：PHP 应用里一个"只读"的 LFI，往往就是代码执行。

### 检测

- **上传落到了预期目录之外**，或者存储名与声明的内容类型不符。
- 文件上传后**几秒内就被请求** —— webshell 的典型工作流。
- **上传目录里出现可执行文件**：`/uploads/` 下任何 `.php`、`.jsp`、`.aspx`，以及任何不是由 CI 部署的 `.htaccess` 或 `web.config`。
- **URL 与参数日志里的穿越特征**：`../`、各种编码变体、`/etc/passwd`、`php://filter`、`win.ini`。
- **Web 服务器拉起的子进程**（`www-data → sh`、`w3wp.exe → cmd.exe`）—— webshell 执行过的签名。
- Web 根目录的文件完整性监控：冒出一个不该有的新文件就是警报。

### 缓解

- **把上传文件存到 Web 根目录之外**，通过应用路由提供，并显式设置内容类型；这样存储里的 PHP 文件就只是一串字节。
- **不要依赖扩展名黑名单。** 用白名单，通过解析文件验证真实内容类型，并把存储文件名改成生成的随机名加固定扩展名。
- **其余一律剥离或编码** —— 文件名、元数据、EXIF —— 永远不要相信客户端的 `Content-Type`。
- **在服务器层面禁止上传目录执行脚本**，并把它当作纵深防御，而不是主要控制。
- **先解码再解析和校验路径**，用真正的路径检查（而不是字符串前缀）与基础目录比对，并拒绝任何越界的结果。
- **不校验归档包内每个条目的最终路径就不要解压**（zip slip），并在沙箱里执行解压。
- **Web 服务器用非特权用户运行**、应用目录只读，这样上传无法覆盖代码，穿越也读不到整个文件系统。
- **对组合行为告警**，因为这两半单独看都很吵、合起来却毫无歧义：一次上传紧跟着对上传路径的请求，或者一个包含参数指向刚刚才到达的文件。
