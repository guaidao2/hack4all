---
id: lfi-source-disclosure
title_en: "LFI, Part 2 — Reading Source Code"
title_zh: "LFI（二）：读源码"
summary_en: Reading a password file proves the bug; reading the application's own source is the objective — and PHP makes that hard because include both reads and executes. This entry covers the php://filter toolkit in full, why the wrappers that switch off URL fetching do not stop it, and why no other language needs an equivalent.
summary_zh: 读一个口令文件只是证明 bug 存在；真正的目标是应用自己的源码 —— 而 PHP 让这件事变难，因为 include 同时做了"读"和"执行"。这一篇把 php://filter 整套讲全，说明为什么"关掉取 URL 的开关"挡不住它，以及为什么其他语言根本不需要等价物。
tags: [web, lfi, php-filter, source-disclosure, wrapper, cwe-98]
tools: [php, python3, curl, Burp Suite]
attck: [T1005, T1083, T1552.001]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The objective is the application's own source

Reading `/etc/passwd` demonstrates that the bug is real. It is rarely the point. What an attacker wants from file read is usually the application's **own code**:

- Credentials and API keys hardcoded in a configuration file.
- The SQL the application builds, which turns a file read into the next vulnerability.
- The authorisation logic, so a bypass can be constructed rather than guessed.
- Framework and library versions, from a composer or requirements file.

And on a PHP target there is a specific obstacle to reaching it, which is the reason `php://filter` exists as a technique at all.

### PHP reads and executes in the same operation

In PHP the function that reads a file for inclusion **executes** it. That is what `include` is for, and it means a file read aimed at source code does not return source code:

```php
ob_start();
include "/var/www/config.php";     // the file runs
$output = ob_get_clean();
// $output is whatever the file printed — for a config file, nothing at all
```

Measured on a file containing a password assignment, the captured output was **empty** (a byte-order mark and nothing else). The file had been read, and what came back was the *effect* of reading it.

The workaround is the whole idea behind the filter wrapper: **put a transformation between reading and emitting, so the bytes come back as data rather than as a program.** That is data-versus-code separation, achieved by making the file something the interpreter will not execute — and the same reading of the same file, through a filter, produced text containing `$DB_PASSWORD` exactly as written.

**This is the first thing to understand about `php://filter`: it is not an escape from the language, it is the mechanism that makes "read a PHP file" mean "read" rather than "run".**

### Why this is a PHP-specific problem

Stated as a comparison, the reason `php://filter` has no equivalent elsewhere becomes obvious, and it doubles as a table of what to expect per language.

| Language | Are "read" and "execute" the same operation? | Reading source needs a special tool? |
|---|---|---|
| **PHP** | **Yes** — `include`, `require` | **Yes** — `php://filter` and friends |
| **Python** | No — `open()` reads, `exec` executes | No — `open()` returns source directly |
| **Java** | No | No |
| **Ruby** | No | No |
| **Node.js** | No | No |

Measured on the Python side of the same test: `open(path).read()` returned the password assignment verbatim, with no wrapper and no trick. Nothing was needed because nothing was fighting it.

> **If a language merges reading and executing, it necessarily needs an encoding wrapper to read its own source. PHP does; the others do not.**

That single sentence explains why this section of the subject is dominated by one runtime, and why a Python target with the same file-read bug is exploited differently — the "read the source" step is simply the bug itself.

### The php://filter toolkit

The syntax is a chain of filters around a target:

```
php://filter/[read=<filter chain>/][write=<filter chain>/]resource=<target>
```

The explicit `read=` form and the shorthand produce identical output — measured, byte for byte — so the shorthand seen in most payloads is just the same thing with the default position.

#### The filters worth knowing

| Filter | What it does | When it is used |
|---|---|---|
| `convert.base64-encode` | Base64-encodes the stream | **The standard way to read source**, because the output is safe ASCII |
| `convert.base64-encode` twice | Double encoding | Against a defence that decodes once; the reader decodes twice |
| `convert.iconv.<from>.<to>` | Converts between character sets | **The output is not base64**, so it defeats a filter that only recognises base64 |
| `string.strip_tags` | Removes HTML tags | Notably **does not strip PHP tags**, so it is not a way to make source readable |
| `string.rot13`, `string.toupper`, `string.tolower` | Character transforms | Light obfuscation, and useful as link elements in a chain |
| `zlib.deflate`, `zlib.inflate` | Compression | Produces a binary stream, useful when paired |
| `convert.base64-encode\|convert.base64-decode` | Cancel each other out | **Resets the stream**, so earlier filters can be undone mid-chain |

The last row is the one that makes chains a design space rather than a list. Because a filter's output is the next filter's input, any sequence of transformations can be composed:

```
php://filter/convert.iconv.UTF8.UTF7/convert.base64-encode/resource=/var/www/config.php
```

Measured, that produced a different length and different content from a plain base64 read — which is the point: **a defence that blocks the string `convert.base64-encode` has not blocked the ability to read source, only one spelling of it.**

And because filters compose, the "cancel" pair lets a chain undo an earlier step before continuing:

```
php://filter/convert.base64-encode/convert.base64-decode/convert.iconv.UTF8.UTF16/resource=...
```

That is why an enumeration of forbidden filter names is the wrong shape of defence here, just as it was in the upload and XSS entries.

#### The two switches people expect to help, and do not

This is the most practically useful measurement in the entry, because it corrects a belief that is widespread.

`allow_url_include` and `allow_url_fopen` control **fetching remote URLs**. `php://filter` is a filter over a **local stream**, and it is not a URL in the sense those settings govern. Measured with both settings turned off:

| Setting | Value |
|---|---|
| `allow_url_fopen` | `Off` |
| `allow_url_include` | `Off` |
| `php://filter/convert.base64-encode/resource=/var/www/config.php` | **still returns the source**, 260 bytes, containing `$DB_PASSWORD` |

So a target hardened with "we disabled remote file inclusion" is **not** protected against source disclosure. The control is aimed at a real risk — remote code inclusion — and it does not cover this one. Anyone who has treated those two directives as a general mitigation for file-read bugs should re-test the assumption.

#### What else can be read, and how

The wrapper ecosystem extends the reach of a file read well beyond "a path on disk":

| Wrapper or path | What it adds |
|---|---|
| `php://input` | The raw request body, treated as a file |
| `php://fd/<n>` | An existing file descriptor of the process |
| `data://` | Inline data as a file; with `allow_url_include=On` this is code execution, not just reading |
| `zip://<archive>#<inner>` | **A file inside a ZIP archive** — measured, this returns the inner file, and it composes with `php://filter` |
| `phar://` | An archive, with the deserialisation side effect described in the PHP entry |
| `expect://` | Command execution, where the extension is present |
| `/proc/self/environ` | The process environment — where credentials and keys frequently live |
| `/proc/self/cmdline` | The command line the process was started with |
| `/proc/self/fd/<n>` | Open file descriptors, including logs and sockets |

Two of those are worth emphasising. **`zip://` means a readable archive is a readable filesystem**, so an upload of a ZIP — combined with an unrelated file-read bug — reaches files nobody put on disk directly; and it composes with the filter, so the inner file can be base64-encoded on the way out. **`/proc/self/environ` is where the secrets usually are** in a containerised deployment, because environment variables are the recommended place to keep them.

### Python: no equivalent, and no need for one

Because Python separates reading from executing, the source-disclosure technique is not a technique. `open()` returns text. What Python has instead is a different **next step**, which is worth knowing because it is specific to this stack and it is a complete path from reading a file to running code.

**The Flask/Werkzeug debugger console.** When debugging is left on, the Werkzeug debugger exposes an interactive Python console in the browser. It is protected by a **PIN**, and the PIN is derived from information about the host and the application:

- the user the process runs as,
- the module and application names,
- **the application's file path**,
- **a machine identifier**, commonly read from `/etc/machine-id` or a kernel-provided boot identifier,
- **the host's MAC address**, from `/sys/class/net/<interface>/address`.

Two of those are files, and **a file-read bug is exactly the tool that reads them**. That is the chain: read the paths and identifiers the PIN is derived from, compute the PIN, open the console, run code. The details of the derivation vary between versions, which is why the practical statement is about the **shape** rather than a formula: *a debug console whose access token is derived from readable local files is a code-execution path for anyone with a file read.*

The mitigations follow directly: **do not run the debugger in production**, and do not leave the paths and identifiers it depends on readable to the application user. The second is hard; the first is a configuration line.

Other Python-side routes from reading to executing, for completeness: the template loader (a controllable path into Jinja2's `FileSystemLoader` reads templates, and template injection turns that into execution), and compiled `.pyc` files where the source itself is not present.

### Where the read-equals-execute property leads, per language

| Language | Read + execute merged? | Reading source | From reading to executing |
|---|---|---|---|
| **PHP** | **Yes** | `php://filter` chain | Log poisoning, session files, `/proc/self/environ`, `data://` with inclusion enabled |
| **Python** | No | `open()` directly | **Debugger PIN**, template injection |
| **Ruby** | No | `File.read` directly | ERB evaluation, `Marshal` |
| **Java** | No | `file:`, `jar:`, `jrt:` | Deserialisation, expression language |
| **Node.js** | No | `fs.readFile` directly | Prototype pollution, `child_process` |

The second column is the one to carry: **it tells you whether the interesting payload will be a wrapper chain or just a path.**

### Detection and mitigation

- **Alert on PHP wrappers in requests, above everything else.** `php://`, `data://`, `zip://`, `phar://`, `expect://`, `php://fd/` appearing in a parameter, a filename field or a body have no legitimate client-side reason to exist. This is the same structural-anomaly rule as `rO0AB` in the deserialisation entry and `${` in the XSS entry: the platform's vocabulary arriving from outside.
- **Alert on filter names specifically.** `convert.base64-encode`, `convert.iconv`, `string.strip_tags`, `zlib.deflate` in a request are not ambiguous at all — they are the components of a source-reading payload, and their presence is intent rather than suspicion.
- **Watch for the reconnaissance shapes.** Requests for `/proc/self/environ`, `/proc/self/cmdline`, `/etc/machine-id`, `/proc/sys/kernel/random/boot_id` and `/sys/class/net/*/address` are not normal application traffic, and they are precisely the files a debugger-PIN chain needs. A single such request from an application user is worth an alert.
- **Detect the output, not only the input.** A response containing a long base64 block, or a content type of `text/plain` where HTML is expected, is the fingerprint of an encoded file read. Comparing response size against the endpoint's baseline catches the reads whose payload was encoded beyond recognition.
- **Do not treat `allow_url_include=Off` as a mitigation for this class, and re-check the assumption if you have.** The measurement above shows `php://filter` working with both URL directives off; the settings address remote inclusion, not local source disclosure.
- **For mitigation, keep user input out of `include` and out of file paths.** That is the fix from part 1, and it removes this entry's techniques entirely, because every one of them needs a controllable path or a controllable wrapper.
- **Use `open_basedir` and a minimal set of enabled wrappers as defence in depth**, knowing that neither is complete — `open_basedir` has a history of bypasses and the filter wrapper does not depend on the settings people usually reach for.
- **Do not keep secrets in source files.** A file read is much less valuable when the configuration contains no credentials, because they come from the environment or a secrets service instead. This does not fix the bug; it changes what the bug is worth, which is the practical difference between an incident and a footnote.
- **And on any Python service: turn the debugger off in production, and verify it is off.** It is a configuration line, it is frequently left on, and with a file read available it is remote code execution rather than an information leak.

<!-- lang:zh -->
### 目标是应用自己的源码

读 `/etc/passwd` 证明了 bug 是真的。它很少是目的。攻击者想从"读文件"里拿到的东西，通常是应用**自己的代码**：

- 硬编码在配置文件里的凭据与 API 密钥。
- 应用拼出来的 SQL —— 它把一次文件读取变成下一个漏洞。
- 鉴权逻辑 —— 这样绕过就是被构造出来的，而不是被猜出来的。
- 框架与库的版本，来自 composer 或 requirements 文件。

而在一个 PHP 目标上，要够到它有一个特定的障碍，这也正是 `php://filter` 作为一门技术存在的原因。

### PHP 的"读"与"执行"是同一个操作

在 PHP 里，那个"为了包含而读取文件"的函数会**执行**它。`include` 的用途就是这个，所以一次冲着源码去的文件读取，拿回来的不是源码：

```php
ob_start();
include "/var/www/config.php";     // 这个文件会被执行
$output = ob_get_clean();
// $output 是那个文件打印出来的东西 —— 对一个配置文件来说，什么都没有
```

在一个含密钥赋值的文件上实测，捕获到的输出是**空的**（一个字节序标记，别无其他）。文件确实被读了，而回来的是读它的**效果**。

绕开它的办法就是过滤器包装器背后的整个想法：**在"读"和"输出"之间插入一次变换，让那些字节以数据的形式回来，而不是以程序的形式回来。** 这就是数据与代码的分离，靠的是把文件变成解释器不会执行的东西 —— 而同一个文件、同一次读取，经过一个过滤器之后，产出的文本里 `$DB_PASSWORD` 和写下的完全一样。

**这是关于 `php://filter` 首先要理解的一点：它不是对语言的一次逃逸，而是那个让"读一个 PHP 文件"意味着"读"而不是"运行"的机制。**

### 为什么这是 PHP 特有的问题

把它写成一个对照，`php://filter` 在别处没有等价物的原因就变得显然了，而它同时也是一张"每种语言该期待什么"的表。

| 语言 | "读"与"执行"是同一个操作吗 | 读源码需要特制工具吗 |
|---|---|---|
| **PHP** | **是** —— `include`、`require` | **需要** —— `php://filter` 之类 |
| **Python** | 否 —— `open()` 读、`exec` 执行 | 不需要 —— `open()` 直接返回源码 |
| **Java** | 否 | 不需要 |
| **Ruby** | 否 | 不需要 |
| **Node.js** | 否 | 不需要 |

同一个测试在 Python 那一侧实测：`open(path).read()` 原样返回了那句密钥赋值，没有包装器，也没有任何技巧。什么都不需要，因为没有任何东西在跟它作对。

> **如果一门语言把"读"和"执行"合并了，它就必然需要一种编码包装器才能读自己的源码。PHP 需要；其他语言不需要。**

这一句话解释了为什么这一块主题被某一个运行时主导，也解释了为什么一个有同样文件读取 bug 的 Python 目标被利用的方式不同 —— "读源码"这一步，本身就是那个 bug。

### `php://filter` 的工具箱

语法是一条围绕目标的过滤器链：

```
php://filter/[read=<过滤器链>/][write=<过滤器链>/]resource=<目标>
```

显式的 `read=` 写法与简写产出的输出**逐字节相同**（实测），所以多数 payload 里看到的简写，只是同一个东西用了默认位置。

#### 值得知道的过滤器

| 过滤器 | 它做什么 | 什么时候用 |
|---|---|---|
| `convert.base64-encode` | 把流做 base64 | **读源码的标准手法**，因为输出是安全的 ASCII |
| `convert.base64-encode` 两次 | 双重编码 | 对付"只解码一次"的防御；读取方解两次 |
| `convert.iconv.<from>.<to>` | 在字符集之间转换 | **输出不是 base64**，所以能打穿只认 base64 的过滤器 |
| `string.strip_tags` | 去掉 HTML 标签 | 注意它**不去 PHP 标签**，所以它不是让源码可读的办法 |
| `string.rot13`、`string.toupper`、`string.tolower` | 字符变换 | 轻度混淆，也适合当链里的一环 |
| `zlib.deflate`、`zlib.inflate` | 压缩 | 产出二进制流，配对使用时有用 |
| `convert.base64-encode\|convert.base64-decode` | 互相抵消 | **重置流**，这样链中段可以撤销前面的过滤器 |

最后一行是把链变成一个"设计空间"而不是一份清单的原因。因为一个过滤器的输出就是下一个过滤器的输入，任何变换序列都能被组合出来：

```
php://filter/convert.iconv.UTF8.UTF7/convert.base64-encode/resource=/var/www/config.php
```

实测中，它产出的长度与内容都与单纯的 base64 读取不同 —— 这正是要点：**一项封禁了 `convert.base64-encode` 这个字符串的防御，并没有封住读源码的能力，只封住了它的一种拼法。**

而因为过滤器可以组合，那个"抵消"对子让一条链能在继续之前撤销前面的一步：

```
php://filter/convert.base64-encode/convert.base64-decode/convert.iconv.UTF8.UTF16/resource=...
```

这就是为什么"列举被禁的过滤器名"在这里是错误的防御形状，和上传、XSS 那两篇一样。

#### 人们以为管用的那两个开关，其实不管用

这是整篇里最实用的一条实测，因为它纠正了一个流传很广的信念。

`allow_url_include` 与 `allow_url_fopen` 管的是**取远程 URL**。而 `php://filter` 是对**本地流**的过滤器，它并不是那两个设置所约束意义上的 URL。把两个设置都关掉之后实测：

| 设置 | 值 |
|---|---|
| `allow_url_fopen` | `Off` |
| `allow_url_include` | `Off` |
| `php://filter/convert.base64-encode/resource=/var/www/config.php` | **仍然返回源码**，260 字节，含 `$DB_PASSWORD` |

所以一个用"我们禁用了远程文件包含"加固过的目标，**并没有**受到源码泄漏方面的保护。那项控制针对的是一个真实风险 —— 远程代码包含 —— 而它不覆盖这一个。任何把那两条指令当作文件读取类 bug 的通用缓解的人，都该重新测一下这个假设。

#### 还能读到什么，以及怎么读

包装器生态把一次文件读取的触达范围远远扩展到"磁盘上的一个路径"之外：

| 包装器或路径 | 它多给了什么 |
|---|---|
| `php://input` | 原始请求体，被当成一个文件 |
| `php://fd/<n>` | 进程里一个已存在的文件描述符 |
| `data://` | 内联数据当文件；配上 `allow_url_include=On` 这就是代码执行，不只是读 |
| `zip://<归档>#<内部名>` | **ZIP 归档里的一个文件** —— 实测它会返回内部文件，而且能与 `php://filter` 组合 |
| `phar://` | 一个归档，带有 PHP 那篇描述过的反序列化副作用 |
| `expect://` | 命令执行（存在该扩展时） |
| `/proc/self/environ` | 进程环境 —— 凭据与密钥经常就住在那里 |
| `/proc/self/cmdline` | 进程启动时的命令行 |
| `/proc/self/fd/<n>` | 打开的文件描述符，包括日志与套接字 |

其中两个值得强调。**`zip://` 意味着一个可读的归档就是一个可读的文件系统** —— 所以一次 ZIP 上传，配上另一个无关的文件读取 bug，就能读到没人直接放到磁盘上的文件；而它能与过滤器组合，所以内部文件可以被 base64 编码后再出来。**`/proc/self/environ` 是密钥通常所在的地方**，在容器化部署里尤其是，因为环境变量正是存放它们的推荐位置。

### Python：没有等价物，也不需要

因为 Python 把"读"和"执行"分开了，"源码泄漏技术"根本不算一门技术。`open()` 返回的就是文本。Python 有的是一个**不一样的下一步**，值得知道，因为它是这个技术栈特有的，而且是一条从读文件到运行代码的完整路径。

**Flask/Werkzeug 的调试控制台。** 调试开着的时候，Werkzeug 调试器会在浏览器里暴露一个可交互的 Python 控制台。它由一个 **PIN** 保护，而那个 PIN 是从关于主机与应用的信息推导出来的：

- 进程以哪个用户运行，
- 模块名与应用名，
- **应用的文件路径**,
- **一个机器标识**，通常读自 `/etc/machine-id` 或内核提供的启动标识，
- **主机的 MAC 地址**，读自 `/sys/class/net/<接口>/address`。

其中两项是文件，而**一个文件读取 bug 正好就是读它们的那件工具**。链路就是这样：读出 PIN 所依据的那些路径与标识、算出 PIN、打开控制台、运行代码。推导细节在不同版本间会变，所以实用的说法是关于**形状**而不是一个公式：*一个访问令牌由本地可读文件推导出来的调试控制台，对任何拥有文件读取的人来说都是一条代码执行路径。*

缓解直接跟着来：**生产环境不要开着调试器**，并且不要让应用用户能读到它依赖的那些路径与标识。后者很难；前者是一行配置。

为了完整性，Python 侧还有其他从读到执行的路径：模板加载器（一个可控路径进入 Jinja2 的 `FileSystemLoader` 就能读到模板，而模板注入把它变成执行），以及源码本身不在时的 `.pyc` 文件。

### 读即执行这条性质，各语言通向哪里

| 语言 | 读与执行合并了吗 | 读源码 | 从读到执行 |
|---|---|---|---|
| **PHP** | **是** | `php://filter` 链 | 日志投毒、会话文件、`/proc/self/environ`、开了包含时的 `data://` |
| **Python** | 否 | 直接 `open()` | **调试器 PIN**、模板注入 |
| **Ruby** | 否 | 直接 `File.read` | ERB 求值、`Marshal` |
| **Java** | 否 | `file:`、`jar:`、`jrt:` | 反序列化、表达式语言 |
| **Node.js** | 否 | 直接 `fs.readFile` | 原型污染、`child_process` |

要带走的是第二列：**它告诉你那个有意思的 payload 会是一条包装器链，还是只是一个路径。**

### 检测与缓解

- **首要告警的是请求里出现 PHP 包装器。** `php://`、`data://`、`zip://`、`phar://`、`expect://`、`php://fd/` 出现在参数、文件名字段或请求体里，在客户端都没有正当理由。这和反序列化那篇里的 `rO0AB`、XSS 那篇里的 `${` 是同一条结构性异常规则：**平台的词汇从外面到达。**
- **特别对过滤器名告警。** 请求里的 `convert.base64-encode`、`convert.iconv`、`string.strip_tags`、`zlib.deflate` 一点也不含糊 —— 它们就是读源码 payload 的组件，而它们出现意味着**意图**，不是嫌疑。
- **盯那些侦察形状。** 对 `/proc/self/environ`、`/proc/self/cmdline`、`/etc/machine-id`、`/proc/sys/kernel/random/boot_id`、`/sys/class/net/*/address` 的请求不是正常应用流量，而它们恰恰是"调试器 PIN"那条链需要的文件。来自应用用户的一次这类请求就值得告警。
- **检测输出，而不只是检测输入。** 一个含长 base64 块的响应，或者在本该是 HTML 的地方出现 `text/plain`，就是一次编码后的文件读取的指纹。把响应大小与该端点的基线对比，能抓到那些 payload 被编码到认不出来的读取。
- **不要把 `allow_url_include=Off` 当成这一类的缓解；如果你一直这么以为，重新核一遍。** 上面的实测显示两个 URL 指令都关着时 `php://filter` 照样工作；那些设置管的是远程包含，不是本地源码泄漏。
- **缓解上，让用户输入既不进 `include`、也不进文件路径。** 那是第一篇里的修法，而它会**整个移除**这一篇的手法，因为它们每一个都需要一个可控路径或一个可控包装器。
- **把 `open_basedir` 与"只开需要的包装器"当纵深防御**，同时知道两者都不完备 —— `open_basedir` 历史上有过绕过，而那个过滤器包装器并不依赖人们通常去够的那些设置。
- **不要把密钥放在源码文件里。** 当配置里没有凭据、它们来自环境或密钥服务时，一次文件读取的价值要小得多。它修不好这个 bug；它改变的是这个 bug 值多少 —— 而那就是"一起事故"和"一个脚注"之间的实际差别。
- **并且在任何 Python 服务上：生产环境关掉调试器，并核实它确实关着。** 那是一行配置，它经常被留着，而在有文件读取可用时，它是远程代码执行，不是信息泄漏。
