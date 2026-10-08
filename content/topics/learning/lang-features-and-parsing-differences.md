---
id: lang-features-and-parsing-differences
title_en: Language Features, Feature Bugs and Parsing Differences
title_zh: 语言特性、特性漏洞与解析差异
summary_en: Vulnerabilities do not live in the web, they live in a runtime — and every runtime has conveniences that behave surprisingly when they meet untrusted input. This entry is the foundation for file upload and deserialisation work — how a language feature becomes attack surface, which parsing differences recur across every stack, and how to work from a technology stack rather than from a payload list.
summary_zh: 漏洞不长在"Web"上，而长在某个具体的运行时上 —— 而每一种运行时都有一些便利特性，在遇到不可信输入时表现得出人意料。这一篇是文件上传与反序列化的地基：讲一个语言特性怎么变成攻击面、哪些解析差异在每种技术栈里反复出现，以及怎么从"先定技术栈"而不是从"payload 清单"开始工作。
tags: [web, language-features, parsing-differences, php, java, python, file-upload-prep]
tools: [curl, Burp Suite, whatweb, nmap, php, python3]
attck: [T1190, T1027]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The premise: there is no such thing as "the web"

A payload does not run against "a website". It runs against **a runtime** — PHP's interpreter, the JVM, Python's import machinery, the .NET CLR, Node's event loop — wrapped in a web server that has its own parsing rules, on an operating system with its own path semantics. Every one of those layers makes decisions about your input, and the interesting bugs live where their decisions differ.

That is why the same string is harmless in one stack and catastrophic in another. `?file=php://filter/convert.base64-encode/resource=index.php` means nothing to a Java application; `0e12345` compared with `0` is a password bypass in PHP 7 and a failed login in PHP 8; a file named `shell.php.` is a PHP script on Apache and a text file on Nginx depending on configuration.

This entry exists because two whole families of vulnerabilities — **file upload** and **deserialisation** — are almost entirely about language and parser behaviour, and neither can be understood by memorising payloads. Get the foundation right and both become tractable; skip it and every target looks like a different magic trick.

### How a feature becomes attack surface

The useful framing is not "this language is insecure". It is: **a convenience that assumes something about its input becomes a vulnerability when the assumption is false.** Sort the conveniences into five groups and the whole landscape becomes navigable.

| Group | The convenience | Where it becomes a vulnerability |
|---|---|---|
| **Type system** | Implicit conversion, loose comparison, dynamic typing | Type confusion: a value of one type treated as another |
| **String and encoding** | Byte strings, multi-byte charsets, lenient decoding | Truncation, overlong forms, normalisation mismatches |
| **Filesystem semantics** | Paths as strings, extensions as suffixes, OS-level convenience | Path traversal, extension confusion, alternate streams |
| **Code execution surface** | `eval`, reflection, templates, deserialisation | Anywhere input reaches a parser that can execute |
| **Convenience functions** | `include`, `open`, `exec`, `load`, one-line helpers | One function call that does something dangerous with its argument |

Two observations tie this together with the rest of this guide:

- **None of these is a bug by itself.** Implicit conversion is a language design choice; `include` is a language feature. What makes them exploitable is that **the boundary between data and instruction was crossed** — the same statement as in every injection entry in this series.
- **The recurring root cause is the same everywhere**: two components parse the same bytes and reach different conclusions. This entry is largely about enumerating *which* two components, for the language layer.

### The recurring bug: two parsers, one input

State it as a single rule and most of this entry follows from it:

> **When two components interpret the same input, and something checks it in one interpretation while something else uses it in the other, the difference is the vulnerability.**

You have met this in the encoding entries (byte-level escaping versus character-level decoding), in the SQL entries (a filter's lexer versus the database's parser), and in the NoSQL entries (a value versus an operator). At the language layer the pairs of disagreeing components are:

| Pair | Where the disagreement shows up |
|---|---|
| **Two functions in the same language** | `os.path.join("/safe", user)` versus string concatenation; `text/template` versus `html/template` |
| **The language and the operating system** | Windows strips trailing dots and spaces from filenames; Linux does not |
| **The language and the web server** | Nginx's `fastcgi_split_path_info` versus what PHP thinks the script path is |
| **The language and its own other version** | `%00` truncation was removed from PHP; loose comparison changed in PHP 8 |
| **The language and a downstream service** | The application's length check versus the database's column width |

**Which makes the method simple to state and hard to do well**: for any input, ask *which parsers will touch it*, and *what does each one do with it*. That question is the actual skill, and it outlives every payload list.

### Parsing differences worth knowing by heart

These recur across stacks, and several show up again in file upload and deserialisation work.

#### Paths

| Difference | Why it matters |
|---|---|
| Multiple slashes, `.`, `..` | Different normalisation between the check and the use |
| **Trailing dots and spaces (Windows)** | `shell.php.` and `shell.php ` resolve to `shell.php` on Windows; the check sees a different name |
| **Alternate data streams** | `file.php::$DATA` — NTFS treats the part after `::` as a stream, so the file content is `file.php` |
| **Reserved device names (Windows)** | `CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9` are special regardless of extension or directory |
| **Backslash versus forward slash** | `\` is a separator on Windows only; a check that normalises `/` and misses `\` is bypassed on Windows |
| **Case sensitivity** | Windows filesystems are case-insensitive: `Shell.PHP` is `shell.php` |
| **Absolute path overrides a prefix join** | `os.path.join("/safe/dir", "/etc/passwd")` returns `/etc/passwd` — the second argument wins |

That last one deserves a runnable demonstration, because it is a common misconception that joining is safe:

```python
import os, os.path

print(os.path.join("/var/www/uploads", "shell.php"))       # /var/www/uploads/shell.php
print(os.path.join("/var/www/uploads", "/etc/passwd"))     # /etc/passwd  <- the prefix is discarded
print(os.path.join("/var/www/uploads", "../../etc/passwd"))  # not normalised — join does not resolve ..
print(os.path.normpath("/var/www/uploads/../../etc/passwd")) # /var/etc/passwd

# the shape of a correct check: resolve, then verify the prefix
def safe_join(base: str, name: str) -> str:
    base = os.path.realpath(base)
    target = os.path.realpath(os.path.join(base, name))
    if not target.startswith(base + os.sep):
        raise ValueError("path escapes the base directory")
    return target
```

Note what the correct version does: it **resolves** both sides and then compares with a separator appended, because `/var/www/uploads-evil` starts with `/var/www/uploads` as a string while being a different directory.

#### Extensions

This is the area where file upload lives, and the disagreements are layered:

| Difference | Example |
|---|---|
| **Which extension decides execution** | Apache's `AddHandler` matches *any* extension in the name, so `shell.php.jpg` runs as PHP; `AddType` behaves differently |
| **Multiple extensions and how the check reads them** | A check using `endswith(".jpg")` accepts `shell.php.jpg`; the server executes by the `.php` |
| **Case** | `.PHP`, `.PhP` on case-insensitive handlers |
| **Alternative extensions** | `.phtml`, `.php3`, `.php4`, `.php5`, `.php7`, `.phar`, `.pht` are often mapped to the PHP handler |
| **Trailing characters** | `shell.php.` on Windows; `shell.php%00.jpg` in old PHP versions with NUL truncation |
| **Web server versus application** | Nginx `location ~ \.php$` and a misconfigured `fastcgi_split_path_info` can hand `/upload/avatar.jpg/x.php` or `/upload/x.php.jpg` to PHP-FPM |

The general lesson, and it is the one that makes upload testable rather than guesswork: **ask which component decides whether a file is executable, and which component decided whether it was allowed.** They are usually different components with different rules, and the gap between them is the vulnerability.

#### URLs and parameters

| Difference | Example |
|---|---|
| Duplicate parameter names | The framework may take the first, the last, or build an array |
| Separator characters | `&` versus `;` as a parameter separator in older servlet containers |
| Encoding of separators | `%26`, `%3D`, `%2F` — a filter decoding once, the application twice |
| `Content-Type` charset versus actual bytes | A body declared as one charset and parsed as another |

#### Numbers and comparison

| Difference | Example |
|---|---|
| Lenient numeric parsing | `"1abc"` becomes `1` in PHP and JavaScript, and an error in Python |
| Leading zeros and prefixes | `010` is 8 in some contexts and 10 in others; `0x10` is 16 |
| **Loose comparison** | `"0e123" == "0e456"` is **true** in PHP, because both parse as `0 × 10^n` |
| Loose comparison, version-dependent | `"abc" == 0` was **true** in PHP 7 and is **false** in PHP 8 — the same code, different answer |
| Large integers | Java's `int` overflow versus Python's arbitrary precision |

```php
<?php
// the classic: two different hashes, equal under loose comparison
var_dump("0e123456" == "0e654321");   // bool(true)  — both are 0 in scientific notation
var_dump(md5("240610708") == md5("QNKCDZO"));  // bool(true) — both digests start with 0e

// and the version-dependent trap
var_dump("abc" == 0);   // PHP 7: true    PHP 8: false
?>
```

#### Unicode and case

| Difference | Example |
|---|---|
| Locale-dependent case folding | In Java or C#, `"I".toLowerCase()` under a Turkish locale gives `ı`, not `i` — a check and a use can disagree |
| Normalisation forms | Two code point sequences that render identically but compare differently |
| Confusables | Cyrillic `а` versus Latin `a` |
| Overlong or invalid sequences | Accepted by lenient decoders, rejected by strict ones |

### The languages, and their personalities

Not a bug list — a short profile of what each runtime's convenience assumes, because that is what you are testing against.

| Runtime | The convenience that bites | Typical surface |
|---|---|---|
| **PHP** | Loose typing and implicit conversion; dynamic inclusion; everything is a string until it is not | Type confusion in authentication, local file inclusion, `phar://` deserialisation, type juggling |
| **ASP / ASP.NET** | A single `Request` object merging query, form, cookies and server variables; ViewState as a serialised object in the client | Parameter pollution, ViewState tampering, upload bypasses on IIS |
| **Java / JSP** | Reflection and serialisation as first-class features; a rich classpath | Deserialisation gadget chains, expression language injection, class loading |
| **Python** | `pickle` and `yaml.load` are convenient and unsafe; strings are implicit in many APIs | Deserialisation, template injection, path handling |
| **Ruby** | `Marshal` and `YAML.load`; `send` for dynamic dispatch; method-missing magic | Deserialisation, arbitrary method invocation |
| **Node.js** | Prototypes are writable and shared; `require` is dynamic | Prototype pollution, code execution through polluted properties |
| **Go** | Static typing and explicit errors; almost no implicit conversion | Very little injection surface — the notable ones are `text/template` (no escaping) versus `html/template`, and path joining |

**Go is worth a sentence** because its presence in a stack changes the assessment: type confusion and deserialisation attacks largely do not apply, and the review shifts toward template choice, path handling and the usual logic bugs. Knowing which *classes* of bug a runtime makes possible is itself a finding.

### Working from the stack, not from a list

This is the practical procedure the entry is building toward, and it is what makes the later upload and deserialisation entries usable rather than memorised.

**Step 1 — identify the stack.** Response headers (`X-Powered-By`, `Server`), cookie names (`PHPSESSID`, `JSESSIONID`, `ASP.NET_SessionId`, `connect.sid`), error page style, URL extensions (`.php`, `.aspx`, `.jsp`, `.do`, `.action`), default files, and the framework's distinctive 404 page.

**Step 2 — list the parsers your input will pass through.** A single query parameter might be parsed by the reverse proxy, the web server, the framework's router, the language's URL decoder, the framework's parameter parser, the ORM, and the database. Each has rules.

**Step 3 — find where two of them disagree.** That is where the bug is. For a file upload the interesting pair is usually *the code that decided the extension* and *the code that decided whether to execute it*. For a deserialisation issue it is *the code that decided the input was data* and *the code that decided to reconstruct an object from it*.

**Step 4 — test the difference directly.** Take a benign value, and find the smallest input that one component accepts and the other interprets differently. That is a far better use of a request budget than spraying a payload list.

### Detection and mitigation

- **The strongest detection signal is a contradiction between two logs.** If the access log shows a request for `avatar.jpg` and the error log shows a PHP fatal error on `avatar.jpg`, something treated that file as PHP. If the WAF logged a blocked request and the application logged a successful one, the filter and the application disagreed. **Correlating components that disagree is how this class of bug is found in production**, because neither log looks wrong on its own.
- **Log the values, escaped, at every layer that parses them.** The raw request, the decoded parameter, the server's notion of the script path, the filename the application decided to store. When these differ, you have found either a bug or a bypass — and without them, the disagreement is invisible.
- **Alert on inputs that look like they are probing for differences**: multiple extensions, trailing dots or spaces, `::`, encoded path separators, mixed case extensions, and duplicate parameters. None is proof of an attack, and all of them are cheap signals that somebody is testing how your layers disagree rather than what your filters block.
- **For mitigation, the principle is to remove the ambiguity rather than to check harder.** Canonicalise once, at one boundary, and pass the canonical value everywhere; do not re-derive meaning from a string at a second point.
- **Use the language's safe API rather than its convenient one**: resolve paths and compare prefixes instead of trusting a join; use escaping templates rather than raw ones; pass argument lists to subprocesses rather than shell strings; prefer typed parsers over lenient ones; disable the features you do not need (`allow_url_include`, unsafe deserialisation, dynamic script evaluation).
- **Allowlist what you accept, and keep the allowlist in one place.** Extension checks scattered across the request handler, the storage layer and the serving configuration are three different policies, and the vulnerability usually lives in the gap between two of them. One policy, applied after canonicalisation, checked by the component that actually serves the file.
- **And keep least privilege as the backstop.** Every parsing bug above is amplified or bounded by what the process can do: a web process that cannot write outside its upload directory, cannot execute what it writes, and runs as an unprivileged user turns a bypass into a much smaller incident. That is not a substitute for fixing the parser, and it is the control that saves you when someone else's parser is the one that is wrong.

<!-- lang:zh -->
### 前提：不存在"Web"这种东西

一个 payload 不是跑在"一个网站"上的，它跑在**某个运行时**上 —— PHP 的解释器、JVM、Python 的导入机制、.NET 的 CLR、Node 的事件循环 —— 外面包着一台有自己的解析规则的 Web 服务器，再外面是一个有自己的路径语义的操作系统。这些层每一层都在对你的输入做判断，而**有意思的 bug 就住在它们的判断不一致的地方**。

这就是同一个字符串在一种技术栈里无害、在另一种里是灾难的原因。`?file=php://filter/convert.base64-encode/resource=index.php` 对一个 Java 应用毫无意义；`0e12345` 和 `0` 比较在 PHP 7 里是一次口令绕过、在 PHP 8 里是一次登录失败；一个叫 `shell.php.` 的文件，在 Apache 上是 PHP 脚本，在 Nginx 上取决于配置，可能只是文本文件。

这一篇存在的原因，是两个完整的漏洞家族 —— **文件上传**与**反序列化** —— 几乎完全是关于语言与解析器行为的，而两者都没法靠背 payload 来理解。地基打对了，两者都变得可下手；跳过它，每个目标看起来都像另一套戏法。

### 一个特性怎么变成攻击面

有用的框架不是"这门语言不安全"，而是：**一个对输入做了某种假设的便利特性，在假设不成立时就成了漏洞。** 把这些便利分五组，整个地形就可导航了。

| 组 | 那个便利 | 它在哪里变成漏洞 |
|---|---|---|
| **类型系统** | 隐式转换、宽松比较、动态类型 | 类型混淆：一种类型的值被当成另一种用 |
| **字符串与编码** | 字节串、多字节字符集、宽松解码 | 截断、过长形式、规范化不一致 |
| **文件系统语义** | 路径是字符串、扩展名是后缀、操作系统级的便利 | 路径穿越、扩展名混淆、备用数据流 |
| **代码执行面** | `eval`、反射、模板、反序列化 | 任何输入能到达一个会执行东西的解析器的地方 |
| **便利函数** | `include`、`open`、`exec`、`load`、一行搞定的助手 | 一次函数调用，用它的参数做了危险的事 |

有两个观察把它和本指南其余部分连起来：

- **上面每一条本身都不是 bug。** 隐式转换是语言设计选择；`include` 是语言功能。让它们可利用的是**数据与指令的边界被跨过** —— 和这个系列里每一篇注入都是同一句话。
- **反复出现的根因处处相同**：两个组件解析同一串字节、得出不同结论。这一篇基本上就是在枚举，在语言这一层上，*哪两个*组件。

### 那个反复出现的 bug：两个解析器，一份输入

把它写成一条规则，这一篇的大部分内容都能由它推出来：

> **当两个组件解释同一份输入，而某个东西按一种解释去检查它、另一个东西按另一种解释去使用它时，那个差异就是漏洞。**

你在编码那几篇里见过它（字节层的转义 vs 字符层的解码），在 SQL 那几篇里见过它（过滤器的词法器 vs 数据库的解析器），在 NoSQL 那几篇里见过它（一个值 vs 一个运算符）。在语言这一层，互相不一致的组件对是：

| 组件对 | 分歧出现在哪 |
|---|---|
| **同一门语言里的两个函数** | `os.path.join("/safe", user)` 对字符串拼接；`text/template` 对 `html/template` |
| **语言与操作系统** | Windows 会剥掉文件名末尾的点和空格；Linux 不会 |
| **语言与 Web 服务器** | Nginx 的 `fastcgi_split_path_info` 对 PHP 认为的脚本路径 |
| **语言与它自己的另一个版本** | `%00` 截断已从 PHP 移除；宽松比较在 PHP 8 变了 |
| **语言与下游服务** | 应用的长度检查对数据库的列宽 |

**这让方法一说就清楚、做好却很难**：对任何输入，问*会有哪些解析器碰它*，以及*每一个会把它怎么处理*。这个问题才是真正的技能，而且它比任何 payload 清单都活得久。

### 值得记牢的解析差异

这些在各类技术栈里反复出现，其中好几条还会在文件上传与反序列化的工作里再次登场。

#### 路径

| 差异 | 为什么重要 |
|---|---|
| 多重斜杠、`.`、`..` | 检查与使用之间的规范化不同 |
| **末尾的点和空格（Windows）** | `shell.php.` 和 `shell.php ` 在 Windows 上解析到 `shell.php`；而检查看到的是另一个名字 |
| **备用数据流** | `file.php::$DATA` —— NTFS 把 `::` 之后的部分当作数据流，于是文件内容就是 `file.php` |
| **保留设备名（Windows）** | `CON`、`PRN`、`AUX`、`NUL`、`COM1`–`COM9`、`LPT1`–`LPT9` 无论扩展名和目录如何都是特殊的 |
| **反斜杠与正斜杠** | `\` 只在 Windows 上是分隔符；一个只规范化 `/` 而漏掉 `\` 的检查，在 Windows 上会被绕过 |
| **大小写敏感性** | Windows 文件系统不区分大小写：`Shell.PHP` 就是 `shell.php` |
| **绝对路径会覆盖前缀拼接** | `os.path.join("/safe/dir", "/etc/passwd")` 返回 `/etc/passwd` —— 第二个参数赢了 |

最后一条值得跑一次演示，因为"拼接是安全的"是个常见误解：

```python
import os, os.path

print(os.path.join("/var/www/uploads", "shell.php"))       # /var/www/uploads/shell.php
print(os.path.join("/var/www/uploads", "/etc/passwd"))     # /etc/passwd  <- 前缀被丢掉了
print(os.path.join("/var/www/uploads", "../../etc/passwd"))  # 不规范化 —— join 不解析 ..
print(os.path.normpath("/var/www/uploads/../../etc/passwd")) # /var/etc/passwd

# 正确检查的形状：先解析，再验证前缀
def safe_join(base: str, name: str) -> str:
    base = os.path.realpath(base)
    target = os.path.realpath(os.path.join(base, name))
    if not target.startswith(base + os.sep):
        raise ValueError("path escapes the base directory")
    return target
```

注意正确版本做了什么：它把两边都**解析**掉，然后**带上分隔符**比较前缀 —— 因为 `/var/www/uploads-evil` 在字符串上确实以 `/var/www/uploads` 开头，却是另一个目录。

#### 扩展名

文件上传就住在这里，而分歧是分层的：

| 差异 | 例子 |
|---|---|
| **哪个扩展名决定执行** | Apache 的 `AddHandler` 匹配名字里的*任意*扩展名，所以 `shell.php.jpg` 会按 PHP 执行；`AddType` 的行为不一样 |
| **多扩展名与检查怎么读它** | 用 `endswith(".jpg")` 的检查接受 `shell.php.jpg`；而服务器按 `.php` 执行 |
| **大小写** | 在不区分大小写的处理器上，`.PHP`、`.PhP` |
| **备用扩展名** | `.phtml`、`.php3`、`.php4`、`.php5`、`.php7`、`.phar`、`.pht` 常常都被映射到 PHP 处理器 |
| **末尾字符** | Windows 上的 `shell.php.`；老版本 PHP 有 NUL 截断时的 `shell.php%00.jpg` |
| **Web 服务器与应用** | Nginx 的 `location ~ \.php$` 配上配错的 `fastcgi_split_path_info`，会把 `/upload/avatar.jpg/x.php` 或 `/upload/x.php.jpg` 交给 PHP-FPM |

通用的教训，也是让上传变得可测而不是靠猜的那一条：**问清楚"哪个组件决定一个文件可不可执行"，以及"哪个组件决定了它被不被允许"。** 它们通常是规则不同的两个组件，而两者之间的缝就是漏洞。

#### URL 与参数

| 差异 | 例子 |
|---|---|
| 重复的参数名 | 框架可能取第一个、取最后一个，或者构建成数组 |
| 分隔符字符 | 较老的 Servlet 容器里，`&` 与 `;` 都能当参数分隔符 |
| 分隔符的编码 | `%26`、`%3D`、`%2F` —— 过滤器解码一次、应用解码两次 |
| `Content-Type` 的 charset 与实际字节 | 请求体声明了一种字符集，却按另一种解析 |

#### 数值与比较

| 差异 | 例子 |
|---|---|
| 宽松的数值解析 | `"1abc"` 在 PHP 和 JavaScript 里变成 `1`，在 Python 里是错误 |
| 前导零与前缀 | `010` 在某些语境里是 8、在另一些里是 10；`0x10` 是 16 |
| **宽松比较** | `"0e123" == "0e456"` 在 PHP 里是**真**，因为两者都按 `0 × 10^n` 解析 |
| 依赖版本的宽松比较 | `"abc" == 0` 在 PHP 7 里是**真**、在 PHP 8 里是**假** —— 同一段代码，答案不同 |
| 大整数 | Java 的 `int` 溢出对 Python 的任意精度 |

```php
<?php
// 经典：两个不同的哈希，在宽松比较下相等
var_dump("0e123456" == "0e654321");   // bool(true) —— 两者在科学计数法下都是 0
var_dump(md5("240610708") == md5("QNKCDZO"));  // bool(true) —— 两个摘要都以 0e 开头

// 以及那个依赖版本的陷阱
var_dump("abc" == 0);   // PHP 7: true    PHP 8: false
?>
```

#### Unicode 与大小写

| 差异 | 例子 |
|---|---|
| 依赖区域设置的大小写折叠 | 在 Java 或 C# 里，土耳其语区域下 `"I".toLowerCase()` 得到 `ı` 而不是 `i` —— 检查与使用可能不一致 |
| 规范化形式 | 两种码点序列渲染得一模一样、比较起来却不同 |
| 同形字 | 西里尔 `а` 对拉丁 `a` |
| 过长或非法序列 | 宽松解码器接受、严格解码器拒绝 |

### 各门语言的"脾气"

这不是一份 bug 清单，而是每种运行时的便利**假设了什么**的简短画像 —— 因为你测的就是那个。

| 运行时 | 会咬人的那个便利 | 典型攻击面 |
|---|---|---|
| **PHP** | 宽松类型与隐式转换；动态包含；在它不是字符串之前一切都像字符串 | 认证里的类型混淆、本地文件包含、`phar://` 反序列化、类型杂耍 |
| **ASP / ASP.NET** | 单个 `Request` 对象合并查询串、表单、cookie 与服务器变量；ViewState 是客户端的一个序列化对象 | 参数污染、ViewState 篡改、IIS 上的上传绕过 |
| **Java / JSP** | 反射与序列化是一等公民；类路径丰富 | 反序列化 gadget 链、表达式语言注入、类加载 |
| **Python** | `pickle` 和 `yaml.load` 方便且不安全；很多 API 里字符串是隐式的 | 反序列化、模板注入、路径处理 |
| **Ruby** | `Marshal` 与 `YAML.load`；`send` 做动态派发；method_missing 魔法 | 反序列化、任意方法调用 |
| **Node.js** | 原型可写且共享；`require` 是动态的 | 原型污染、通过被污染的属性实现代码执行 |
| **Go** | 静态类型与显式错误；几乎没有隐式转换 | 注入面极小 —— 值得注意的是 `text/template`（不转义）对 `html/template`，以及路径拼接 |

**Go 值得一句话**，因为技术栈里出现它会改变评估方式：类型混淆和反序列化攻击基本不适用，评审重心转向模板选择、路径处理和常见的逻辑 bug。**知道一种运行时让哪几类 bug 成为可能，本身就是一个发现。**

### 从技术栈出发，而不是从清单出发

这是这一篇要搭出来的实用流程，也是让后面上传与反序列化那几篇可被使用、而不是被背诵的东西。

**第一步 —— 确定技术栈。** 响应头（`X-Powered-By`、`Server`）、Cookie 名（`PHPSESSID`、`JSESSIONID`、`ASP.NET_SessionId`、`connect.sid`）、错误页风格、URL 扩展名（`.php`、`.aspx`、`.jsp`、`.do`、`.action`）、默认文件，以及框架特有的 404 页面。

**第二步 —— 列出你的输入会经过的解析器。** 一个查询参数可能经过反向代理、Web 服务器、框架路由、语言的 URL 解码器、框架的参数解析器、ORM，以及数据库。每一个都有规则。

**第三步 —— 找出其中哪两个不一致。** bug 就在那里。对一次文件上传，有意思的那一对通常是*决定扩展名的代码*和*决定要不要执行的代码*。对反序列化问题，则是*决定输入是数据的代码*和*决定要从它重建对象的代码*。

**第四步 —— 直接测那个差异。** 拿一个无害的值，找出"一个组件接受、另一个组件解释成别的东西"的最小输入。这比撒一份 payload 清单划算得多。

### 检测与缓解

- **最强的检测信号是两份日志之间的矛盾。** 如果访问日志显示请求的是 `avatar.jpg`，而错误日志显示 `avatar.jpg` 触发了 PHP 致命错误，那就说明有东西把它当成 PHP 了。如果 WAF 记了一条已拦截、应用却记了一条成功，过滤器与应用就不一致。**把"互相不一致的组件"关联起来，就是这类 bug 在生产里被找到的方式**，因为单独看哪一份日志都没错。
- **在每一个会解析它们的层，把值转义后记下来。** 原始请求、解码后的参数、服务器认为的脚本路径、应用决定存下来的文件名。当这些不一样时，你要么找到了一个 bug，要么找到了一条绕过 —— 而没有它们，这个不一致是看不见的。
- **对"像在试探差异"的输入告警**：多重扩展名、末尾的点和空格、`::`、编码过的路径分隔符、大小写混杂的扩展名、重复参数。没有一条能证明是攻击，而每一条都是很便宜的信号，说明有人在测**你的各层怎么分歧**，而不是在测你的过滤器拦什么。
- **缓解的原则是消除歧义，而不是检查得更狠。** 在一个边界上、只规范化一次，然后把规范化后的值传给所有地方；不要在第二个点上从字符串重新推导含义。
- **用语言的安全 API，而不是便利 API**：解析路径并比较前缀，而不是相信一次 join；用会转义的模板而不是原始的；把参数列表传给子进程而不是 shell 字符串；优先用带类型的解析器而不是宽松的；关掉你不需要的功能（`allow_url_include`、不安全的反序列化、动态脚本求值）。
- **只白名单你接受的东西，并且把白名单放在一个地方。** 扩展名检查散落在请求处理器、存储层和服务配置里，就是三套不同的策略，而漏洞通常住在其中两个之间的缝里。**一套策略，在规范化之后应用，由真正提供文件的那个组件来检查。**
- **并且始终把最小权限当兜底。** 上面每一个解析 bug 都被进程能做什么放大或限制：一个不能往上传目录外写、不能执行自己写进去的东西、以非特权用户运行的 Web 进程，会把一次绕过变成一起小得多的事件。这不是修好解析器的替代品，而是在**别人的解析器出错时**救你的那个控制。
