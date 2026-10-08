---
id: lang-php
title_en: "PHP — Loose Types, Dynamic Execution and File Semantics"
title_zh: "PHP：松散类型、动态执行与文件语义"
summary_en: PHP powers most of the web, and its conveniences are why an entire family of classic bugs exists. This entry covers loose comparison and the 0e collision together with the PHP 8 change, the dynamic-execution features that turn a parameter into code, phar and the stream wrappers, file handling as upload preparation, and the php.ini settings that decide how much of it is reachable.
summary_zh: PHP 支撑着 Web 的大部分，而它那些便利特性正是一整族经典 bug 存在的原因。这一篇讲松散比较与 0e 碰撞（连同 PHP 8 的变化）、把参数变成代码的动态执行特性、phar 与流包装器、作为上传前置知识的文件处理，以及决定有多少东西够得着的 php.ini 设置。
tags: [web, php, type-juggling, loose-comparison, phar, file-upload-prep, lfi-prep]
tools: [php, php-fpm, Burp Suite, curl, phpggc]
attck: [T1190, T1027]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why PHP gets its own entry

PHP runs a large share of the web, and its design choices are unusually visible in the bug reports: **dynamic typing with aggressive implicit conversion**, **functions that execute strings**, and **file handling that is one parameter away from being dangerous**. None of those is a defect in isolation — they are what make the language pleasant to write — but together they produce a bug family that other runtimes simply do not have.

This entry is about the features, not about the vulnerabilities they later feed into. The path traversal, the file inclusion and the deserialisation entries each get their own treatment; what is here is the language behaviour they depend on, which is what you need in order to test them deliberately rather than by payload.

### Loose comparison, and the 0e collision

Two values compared with `==` are converted to a common type first, and the rules are **not** the ones most developers assume. The canonical consequence:

```php
<?php
var_dump("0e123456" == "0e654321");            // bool(true)   both parse as 0 x 10^n
var_dump("1e1" == "10");                       // bool(true)
var_dump("010" == "10");                       // bool(true)   both are numeric strings
var_dump(md5("240610708") == md5("QNKCDZO"));  // bool(true)   both digests start with 0e
var_dump(md5("240610708"));                    // 0e462097431906509019562988736854
var_dump(md5("QNKCDZO"));                      // 0e830400451993494058024219903391
```

Both digests begin with `0e` followed by digits, so as numeric strings they both evaluate to zero, and two zeros are equal. The classic exploitation is a password check written as:

```php
// vulnerable: comparing hashes with ==
if (md5($password) == $row['password_hash']) { /* logged in */ }
```

An attacker who can choose the **input** side rather than the stored side supplies `240610708`, whose MD5 has the `0e` shape, and the comparison passes against any stored hash that also has that shape. The bug is not in MD5 — MD5 is already unsuitable for passwords for other reasons — it is in **`==` being used on strings that are supposed to be compared as strings**.

The same collision exists for SHA-1 and for any hash whose output can begin with `0e` followed by digits only.

**The fix is not "avoid inputs starting with 0e".** It is:

```php
// correct: compare as strings, and in constant time
if (hash_equals($row['password_hash'], md5($password))) { ... }   // PHP 5.6+

// or, better: do not compare hashes at all, use the password API
if (password_verify($password, $row['password_hash'])) { ... }
```

`hash_equals` exists precisely because `==` on strings is not a string comparison, and because a comparison that returns early leaks timing. `password_verify` with `password_hash` is better still, because it uses a password-appropriate algorithm (bcrypt/argon2) instead of MD5.

#### The PHP 8 change, and why version matters

The comparison rules were tightened in PHP 8. The single most useful example:

```php
<?php
var_dump("abc" == 0);
// PHP 7: bool(true)   — "abc" is converted to 0
// PHP 8: bool(false)  — a non-numeric string is never equal to a number
```

That is a behaviour change in a security-relevant path: an old authentication bypass may simply stop working, and a new one may appear in code that relied on the old semantics. **The same source file has different behaviour on different runtimes**, which is the version-difference category from the previous entry in this series, and it is why "does this work" is a question about the target's version rather than about the payload.

The table worth internalising:

| Comparison | PHP 7 | PHP 8 |
|---|---|---|
| `"abc" == 0` | true | **false** |
| `"" == 0` | true | true |
| `"0e123" == "0e456"` | true | **true** (both numeric strings) |
| `0 == "0"` | true | true |
| `"1abc" == 1` | true | **false** |
| `"10" == "1e1"` | true | true |

Note which rows did *not* change: the `0e` collision is about **two numeric strings**, and that is still true in PHP 8. The change affects comparisons between a **number and a non-numeric string**.

**The general lesson for every language in this series**: security-relevant comparison semantics are not stable across versions, so a check written with `==` is a check whose meaning depends on the runtime. Use `===` when the types should match, use `hash_equals` for secrets, and use a password API for passwords.

### Features that execute what you give them

PHP has an unusually large set of ways for a string to become code. They are legitimate features; the risk is entirely about whether user input reaches them.

| Feature | What it does | Where input must not reach |
|---|---|---|
| `eval($code)` | Executes a string as PHP | Anywhere a string comes from a request |
| `include` / `require` | Loads and executes another file, by path or URL or stream wrapper | The path argument |
| `assert()` | In old versions executed a string argument | Historic; removed as a code path in PHP 8 |
| `preg_replace` with `/e` | Evaluated the replacement as PHP | Removed in PHP 7 |
| `create_function` | Created a function from a string | Removed in PHP 8 |
| `call_user_func` | Calls a function named by a string | The function name |
| `$$var` (variable variables) | Treats a string as a variable name | Anywhere attacker-chosen names reach it |
| `extract($array)` | Creates variables from an array's keys | Before code that trusts its variable names |
| `unserialize($data)` | Reconstructs objects, invoking magic methods | Any untrusted data |

**Variable variables and `extract` deserve a sentence each**, because they are the PHP-specific version of a general mistake — letting an input decide *which name* something has rather than *which value* it holds:

```php
<?php
// variable variables: the request chooses the variable name
$name = $_GET['name'];
$$name = "attacker controlled";
// ?name=isAdmin  sets $isAdmin

// extract: the request chooses every variable name at once
extract($_GET);
// ?isAdmin=1  creates $isAdmin with value 1
```

Both are the same shape as the operator injection in the NoSQL entry: the input is not choosing a value inside a structure, it is **choosing part of the structure**. The defence is the same in spirit — constrain what can be a name, rather than filtering what is inside a value.

`unserialize` and `phar` are the bridge to the deserialisation entry, so this entry only states the language-level facts:

- **`unserialize` reconstructs objects** and calls their magic methods (`__wakeup`, `__destruct`) as a side effect of the reconstruction itself. That is why the danger is not "the data is malformed" but "an object graph gets built".
- **`phar://` is a stream wrapper that triggers this as a side effect of ordinary file operations.** The subtlety worth remembering: a call like `file_exists("phar://uploads/avatar.jpg")` is enough to make PHP read the phar metadata, which historically meant **deserialisation could be triggered by a file operation that looks completely harmless** in the source. Newer PHP versions changed the specifics, which is why the version matters here too.

### Stream wrappers

PHP addresses streams by scheme, and a parameter that ends up in a file function can therefore select the scheme:

| Wrapper | Effect |
|---|---|
| `php://filter` | Applies filters to a stream; `convert.base64-encode` turns a PHP source file into something readable |
| `php://input` | The raw request body |
| `data://` | Inline data as if it were a file |
| `phar://` | An archive, with the metadata side effect above |
| `zip://` | Read inside a ZIP archive |
| `expect://` | Executes a command via the expect extension, if installed |

```
?file=php://filter/convert.base64-encode/resource=index.php
```

That single parameter is not a vulnerability by itself — the vulnerability is that a file function received a user-controlled path. What makes it worth knowing as a *language feature* is that the scheme is part of the string, so a defence that validates a file name must also decide what to do with a scheme. Validating a suffix while ignoring `php://` is validating half the input.

### File semantics, as upload preparation

File upload is its own entry, so this section is about the language behaviour it depends on.

**How an uploaded file arrives.** PHP puts the upload in a temporary file and exposes it in `$_FILES`:

```php
<?php
// $_FILES['avatar'] = [
//   'name'     => what the client sent      <- attacker controlled
//   'type'     => what the client claimed   <- attacker controlled
//   'tmp_name' => the real temporary path   <- server controlled
//   'error'    => an integer code
//   'size'     => an integer
// ]
move_uploaded_file($_FILES['avatar']['tmp_name'], "/var/www/uploads/" . $_FILES['avatar']['name']);
```

Two language-level facts follow from that structure, and both matter for upload testing:

- **`name` and `type` come from the request.** A defence that trusts `$_FILES['type']` is trusting the client, and a defence that uses `name` directly in a path is exposing the filename to path traversal.
- **`tmp_name` is the only component the server chose**, which is why `is_uploaded_file()` exists — it verifies that a path really is an uploaded temporary file, so that a function expecting an upload cannot be made to read an arbitrary path.

**Extension handling is where PHP's own functions disagree with each other**, which is the pattern from the previous entry showing up inside one runtime:

```php
<?php
$name = "shell.php.jpg";
echo pathinfo($name, PATHINFO_EXTENSION), "\n";   // jpg   <- the last extension
echo pathinfo($name, PATHINFO_FILENAME), "\n";    // shell.php
echo pathinfo("shell.php.", PATHINFO_EXTENSION), "\n";   // (empty)
// note: passing a string containing a NUL byte to pathinfo raises a ValueError in PHP 8
```

Read that carefully: `pathinfo` reports `jpg` for `shell.php.jpg`, while the web server's handler configuration may execute by `.php` anywhere in the name. **The application's check and the server's execution decision are reading the same filename differently** — the archetypal bug of this whole series, and the reason upload bypasses keep working.

Other language-level details worth knowing:

| Behaviour | Why it matters |
|---|---|
| `basename()` and locale | With a multi-byte locale, `basename()` can behave differently — a check and a use can disagree |
| NUL truncation | Removed in PHP 5.3.4; historically a filename check could be defeated by an embedded `\0` |
| `realpath()` and `open_basedir` | `realpath` resolves symlinks and `..`, which is what a correct containment check needs |
| Case sensitivity of the file system | On Windows, `SHELL.PHP` is `shell.php` |
| `upload_tmp_dir` | Where uploads land; a world-writable directory is a different problem than a bad extension check |

### Configuration is attack surface

`php.ini` decides how much of the above is reachable, and a stack review that ignores it is incomplete.

| Setting | Effect when wrong |
|---|---|
| `allow_url_include` | Lets `include` fetch a remote URL — remote file inclusion as a language feature |
| `allow_url_fopen` | Lets file functions open URLs |
| `open_basedir` | Restricts file operations to a set of directories; unset means anywhere the process can reach |
| `disable_functions` | Removes specific functions; a partial list is a puzzle rather than a control |
| `expose_php` | Adds `X-Powered-By: PHP/x.y.z`, which hands the attacker the version for free |
| `display_errors` | Prints errors to the response, which is an information channel |
| `session.upload_progress` | Enables progress tracking, which writes a session file whose **name and content are partly attacker-influenced** — a known way to get a file with controlled content onto the server |
| `register_globals`, `magic_quotes_gpc` | Historic; `magic_quotes` is worth remembering as a lesson (below) |

**`magic_quotes_gpc` is the best historical illustration of this guide's four-layer standard.** It escaped quotes at the input layer — automatically, for every request. It did not prevent SQL injection, because escaping happens at the wrong layer: it escapes characters, while the meaning is decided by the database's parser, in a different charset, after any further decoding. It also corrupted legitimate data containing quotes, which is why it was removed. The lesson survives the feature: **a defence that works on the representation instead of the boundary is both incomplete and harmful.**

### Detection and mitigation

- **Compare with `===`, or better, with the right API.** `==` on security-relevant values is the single most productive PHP bug pattern: use `hash_equals` for secrets, `password_verify` for passwords, and strict comparison for everything else. Adding `declare(strict_types=1)` at the top of a file makes the runtime refuse implicit conversions inside it, which turns a class of silent confusion into an error.
- **Watch for the collision shapes in requests.** A parameter whose value starts with `0e` followed by digits is the signature of a loose-comparison bypass attempt, and it is cheap to alert on. So is an MD5 that begins with `0e` being submitted as a password.
- **Alert on stream wrappers and dynamic-execution functions in input.** `php://`, `phar://`, `data://`, `zip://`, `expect://`, `eval`, `assert`, `call_user_func`, and `extract` appearing in a request body or parameter have no legitimate reason to be there: they are the server's vocabulary, not the client's — the same signal as `$`-prefixed keys in the NoSQL entry.
- **Log and diff the filename at every layer.** The name the client sent, the name after `pathinfo`, the name the storage layer wrote, and the path the server actually served. The upload bypass lives in the difference between two of those four, and none of them shows it alone.
- **Turn off what you do not use.** `allow_url_include=Off`, `allow_url_fopen=Off` where possible, `open_basedir` set to the directories the application needs, `expose_php=Off`, `display_errors=Off` with errors going to a log, and a deliberate `disable_functions` list. Each of these removes a technique rather than a payload.
- **Replace `unserialize` with a structured format.** `json_decode` cannot construct arbitrary objects, which removes the entire gadget-chain surface without needing to enumerate it. Where `unserialize` is unavoidable, restrict the allowed classes.
- **Constrain what can be a name.** Never let a request choose a variable name (`extract`, `$$`), a function name (`call_user_func`), an included path, or a file extension. An allowlist of names is a stronger and shorter control than any filter on the contents.
- **Keep the process unable to do damage.** A PHP-FPM pool running as a user that can write inside one upload directory, cannot execute what it writes, and has no write access to the code directories turns a bypass into a much smaller incident — and it is the control that keeps working when the application's check is the one that is wrong.

<!-- lang:zh -->
### 为什么 PHP 值得单独一篇

PHP 承载着 Web 的很大一部分，而它的设计选择在漏洞报告里格外显眼：**动态类型加上积极的隐式转换**、**会执行字符串的函数**、以及**离危险只有一个参数的文​​件处理**。这些单独看都不是缺陷 —— 它们正是这门语言写起来舒服的原因 —— 但合在一起就产生了一族其他运行时根本没有的 bug。

这一篇讲的是**特性**，不是它们日后喂进的那些漏洞。路径穿越、文件包含、反序列化各有自己的篇目；这里放的是它们所依赖的语言行为 —— 而这正是你为了**有意识地测**、而不是按 payload 去试所需要的东西。

### 松散比较，以及 0e 碰撞

用 `==` 比较两个值时，它们会先被转换成同一个类型，而规则**不是**多数开发者以为的那样。最典型的后果：

```php
<?php
var_dump("0e123456" == "0e654321");            // bool(true)   两者都按 0 x 10^n 解析
var_dump("1e1" == "10");                       // bool(true)
var_dump("0x1A" == "26");                      // bool(true)   在会把字符串里的十六进制解析出来的版本上
var_dump(md5("240610708") == md5("QNKCDZO"));  // bool(true)   两个摘要都以 0e 开头
var_dump(md5("240610708"));                    // 0e462097431906509019562988736854
var_dump(md5("QNKCDZO"));                      // 0e830400451993494058024219903391
```

两个摘要都以 `0e` 加一串数字开头，所以作为数值字符串它们都等于零，而两个零相等。经典的利用场景是一个这样写的口令检查：

```php
// 有漏洞：用 == 比较哈希
if (md5($password) == $row['password_hash']) { /* 登录成功 */ }
```

能选择**输入**那一侧（而不是存储那一侧）的攻击者提交 `240610708`，它的 MD5 具备 `0e` 形状，于是比较对任何同样是这种形状的存储哈希都会通过。bug 不在 MD5 —— MD5 不适合口令另有原因 —— 而在**把 `==` 用在两个本该按字符串比较的字符串上**。

同样的碰撞也存在于 SHA-1，以及任何输出可能以 `0e` 加纯数字开头的哈希。

**修法不是"禁止以 0e 开头的输入"**，而是：

```php
// 正确：按字符串比较，并且是恒定时间比较
if (hash_equals($row['password_hash'], md5($password))) { ... }   // PHP 5.6+

// 或者更好：根本不要比较哈希，用口令 API
if (password_verify($password, $row['password_hash'])) { ... }
```

`hash_equals` 存在的理由，正是"字符串上的 `==` 不是字符串比较"，以及"提前返回的比较会泄漏时序"。而 `password_hash` 配 `password_verify` 更好，因为它用的是适合口令的算法（bcrypt/argon2），而不是 MD5。

#### PHP 8 的变化，以及为什么版本要紧

比较规则在 PHP 8 被收紧了。最有用的一个例子：

```php
<?php
var_dump("abc" == 0);
// PHP 7: bool(true)   —— "abc" 被转成 0
// PHP 8: bool(false)  —— 非数值字符串永远不等于数字
```

这是在一个与安全相关的路径上的行为变化：一个旧的口令绕过可能直接失效，而依赖旧语义的代码里又可能出现新的绕过。**同一个源文件在不同运行时上行为不同** —— 这正是上一节里"语言与它自己的另一个版本"那一类差异，也是为什么"这招行不行"问的是目标的版本，而不是 payload。

该记牢的表：

| 比较 | PHP 7 | PHP 8 |
|---|---|---|
| `"abc" == 0` | true | **false** |
| `"" == 0` | true | true |
| `"0e123" == "0e456"` | true | **true**（两者都是数值字符串） |
| `0 == "0"` | true | true |
| `"1abc" == 1` | true | **false** |
| `"10" == "1e1"` | true | true |

注意**哪些行没有变**：`0e` 碰撞是关于**两个数值字符串**的，这在 PHP 8 里依然成立。变的是**数字与非数值字符串**之间的比较。

**这个系列里每一门语言都适用的一条通用教训**：与安全相关的比较语义在不同版本之间并不稳定，所以一个用 `==` 写的检查，其含义取决于运行时。类型本就该相同时用 `===`；机密用 `hash_equals`；口令用口令 API。

### 会执行你给它的东西的那些特性

PHP 里"一个字符串变成代码"的途径异常多。它们都是正当功能；风险完全在于用户输入是否够得着它们。

| 特性 | 它做什么 | 输入绝不能到达哪里 |
|---|---|---|
| `eval($code)` | 把字符串当 PHP 执行 | 任何来自请求的字符串 |
| `include` / `require` | 按路径、URL 或流包装器加载并执行另一个文件 | 那个路径参数 |
| `assert()` | 老版本会执行字符串参数 | 历史问题；PHP 8 移除了这条代码路径 |
| 带 `/e` 的 `preg_replace` | 把替换内容当 PHP 求值 | PHP 7 已移除 |
| `create_function` | 从字符串创建一个函数 | PHP 8 已移除 |
| `call_user_func` | 调用由字符串命名的函数 | 函数名 |
| `$$var`（变量变量） | 把字符串当变量名 | 攻击者能选名字的任何地方 |
| `extract($array)` | 用数组的键创建变量 | 在信任自己变量名的代码之前 |
| `unserialize($data)` | 重建对象，并触发魔术方法 | 任何不可信数据 |

**变量变量和 `extract` 各值得一句**，因为它们是某个通用错误在 PHP 里的版本 —— 让输入决定某个东西**叫什么名字**，而不是决定它**持有什么值**：

```php
<?php
// 变量变量：由请求选择变量名
$name = $_GET['name'];
$$name = "attacker controlled";
// ?name=isAdmin  就设置了 $isAdmin

// extract：请求一次性选择所有变量名
extract($_GET);
// ?isAdmin=1  创建了值为 1 的 $isAdmin
```

两者与 NoSQL 那篇里的运算符注入是同一个形状：输入不是在结构内部选一个值，而是**选了结构的一部分**。防御在精神上也一样 —— 限制什么可以当**名字**，而不是过滤值里面有什么。

`unserialize` 与 `phar` 是通向反序列化那一篇的桥，所以这一篇只陈述语言层面的事实：

- **`unserialize` 会重建对象**，并在重建这个动作本身的过程中调用它们的魔术方法（`__wakeup`、`__destruct`）。这就是为什么危险不在于"数据是畸形的"，而在于"一个对象图被构造出来了"。
- **`phar://` 是一个流包装器，它会把上面这件事作为普通文件操作的副作用触发。** 值得记住的微妙之处：像 `file_exists("phar://uploads/avatar.jpg")` 这样一次调用就足以让 PHP 去读 phar 元数据 —— 这在历史上意味着**一次看起来完全无害的文件操作就能触发反序列化**。较新的 PHP 版本改变了具体细节，所以这里同样要看版本。

### 流包装器

PHP 用 scheme 来定位流，于是一个最终进入文件函数的参数就可以选择 scheme：

| 包装器 | 效果 |
|---|---|
| `php://filter` | 对流施加过滤器；`convert.base64-encode` 能把一个 PHP 源文件变成可读的东西 |
| `php://input` | 原始请求体 |
| `data://` | 内联数据，当作文件用 |
| `phar://` | 一个归档，带有上面那个元数据副作用 |
| `zip://` | 读 ZIP 归档里的内容 |
| `expect://` | 经由 expect 扩展执行命令（若已安装） |

```
?file=php://filter/convert.base64-encode/resource=index.php
```

单独这一个参数不是漏洞 —— 漏洞是某个文件函数收到了用户可控的路径。它作为**语言特性**值得知道的地方在于：scheme 是那个字符串的一部分，所以一个校验文件名的防御，也必须决定拿 scheme 怎么办。**只校验后缀而忽略 `php://`，等于只校验了输入的一半。**

### 文件语义，作为上传的前置知识

文件上传有自己的篇目，所以这一节讲的是它所依赖的语言行为。

**上传的文件是怎么到达的。** PHP 把上传放进一个临时文件，并在 `$_FILES` 里暴露出来：

```php
<?php
// $_FILES['avatar'] = [
//   'name'     => 客户端发来的名字      <- 攻击者可控
//   'type'     => 客户端声称的类型      <- 攻击者可控
//   'tmp_name' => 真正的临时路径        <- 服务器可控
//   'error'    => 一个整数错误码
//   'size'     => 一个整数
// ]
move_uploaded_file($_FILES['avatar']['tmp_name'], "/var/www/uploads/" . $_FILES['avatar']['name']);
```

从这个结构能推出两个语言层面的事实，而它们对上传测试都要紧：

- **`name` 和 `type` 来自请求。** 相信 `$_FILES['type']` 的防御是在相信客户端；把 `name` 直接用在路径里的防御，则把文件名暴露给了路径穿越。
- **`tmp_name` 是唯一由服务器选定的部分**，这也是 `is_uploaded_file()` 存在的原因 —— 它验证一个路径确实是上传的临时文件，这样"期待一个上传"的函数就无法被诱导去读任意路径。

**扩展名处理，是 PHP 自己的函数之间互相不一致的地方** —— 也就是上一节那个模式在这一个运行时内部再次出现：

```php
<?php
$name = "shell.php.jpg";
echo pathinfo($name, PATHINFO_EXTENSION), "\n";   // jpg   <- 最后一个扩展名
echo pathinfo($name, PATHINFO_FILENAME), "\n";    // shell.php
echo pathinfo("shell.php.", PATHINFO_EXTENSION), "\n";   // （空）
```

仔细读：对 `shell.php.jpg`，`pathinfo` 报的是 `jpg`，而 Web 服务器的处理器配置可能按名字里任何位置的 `.php` 去执行。**应用的检查和服务器的执行决定，读的是同一个文件名的不同部分** —— 这整个系列里的原型 bug，也是上传绕过一直有效的原因。

其他值得知道的语言层面细节：

| 行为 | 为什么重要 |
|---|---|
| `basename()` 与区域设置 | 在多字节区域下 `basename()` 可能表现不同 —— 检查与使用可能不一致 |
| NUL 截断 | PHP 5.3.4 已移除；历史上一个文件名检查可以被内嵌的 `\0` 击败 |
| `realpath()` 与 `open_basedir` | `realpath` 会解析符号链接和 `..`，而正确的前缀包含检查正需要这个 |
| 文件系统的大小写敏感性 | Windows 上 `SHELL.PHP` 就是 `shell.php` |
| `upload_tmp_dir` | 上传落在哪；一个所有人可写的目录，与一个糟糕的扩展名检查是两类不同的问题 |

### 配置就是攻击面

`php.ini` 决定了上面这些东西有多少够得着，而忽略它的技术栈评审是不完整的。

| 设置 | 配错时的效果 |
|---|---|
| `allow_url_include` | 让 `include` 去取远程 URL —— 把远程文件包含变成一个语言功能 |
| `allow_url_fopen` | 让文件函数能打开 URL |
| `open_basedir` | 把文件操作限制在一组目录内；不设就意味着进程能到的地方都可以 |
| `disable_functions` | 移除特定函数；一份残缺的清单是谜题，不是控制 |
| `expose_php` | 加上 `X-Powered-By: PHP/x.y.z`，等于把版本白送给攻击者 |
| `display_errors` | 把错误打印到响应里，那是一条信息通道 |
| `session.upload_progress` | 开启进度追踪，它会写一个会话文件，而那个文件的**名字和内容有一部分受攻击者影响** —— 这是已知的"在服务器上落一个内容可控的文件"的途径 |
| `register_globals`、`magic_quotes_gpc` | 历史设置；`magic_quotes` 值得作为一堂课记住（见下） |

**`magic_quotes_gpc` 是本指南四层标准最好的历史例证。** 它在输入层做引号转义 —— 自动地、对每一个请求。它并没有阻止 SQL 注入，因为**转义发生在错误的层次上**：它转义的是字符，而含义是由数据库的解析器决定的，用的还是另一种字符集，并且在任何进一步解码之后。它还会破坏含引号的合法数据，这也是它被移除的原因。教训比那个功能活得久：**一个作用于"表示"而不是作用于"边界"的防御，既不完备，也有害。**

### 检测与缓解

- **用 `===` 比较，或者更好：用对的 API。** 在与安全相关的值上用 `==`，是 PHP 里产出 bug 最多的单一模式：机密用 `hash_equals`，口令用 `password_verify`，其余一律用严格比较。在文件顶部加 `declare(strict_types=1)`，能让运行时拒绝该文件内的隐式转换，把一整类静默困惑变成报错。
- **在请求里盯那些碰撞形状。** 一个值以 `0e` 加一串数字开头，就是松散比较绕过的特征，对它告警很便宜。作为一个口令提交、且以 `0e` 开头的 MD5 也一样。
- **对输入里的流包装器和动态执行函数告警。** `php://`、`phar://`、`data://`、`zip://`、`expect://`、`eval`、`assert`、`call_user_func`、`extract` 出现在请求体或参数里都没有正当理由：它们是服务器的词汇，不是客户端的 —— 和 NoSQL 那篇里 `$` 开头的键是同一个信号。
- **在每一层记下文件名并做差分。** 客户端发来的名字、`pathinfo` 之后的名字、存储层写下的名字、服务器实际提供的路径。上传绕过就住在其中两个的差异里，而单独看哪一个都看不出来。
- **关掉你用不到的东西。** `allow_url_include=Off`；能关就 `allow_url_fopen=Off`；`open_basedir` 设成应用真正需要的目录；`expose_php=Off`；`display_errors=Off` 并把错误写进日志；以及一份刻意列出的 `disable_functions`。每一条移除的是一种技术，而不是一个 payload。
- **用结构化格式替换 `unserialize`。** `json_decode` 无法构造任意对象，于是整个 gadget 链攻击面被移除，而不需要把它枚举出来。实在必须用 `unserialize` 的地方，就限制允许的类。
- **限制什么可以当名字。** 永远不要让请求选择变量名（`extract`、`$$`）、函数名（`call_user_func`）、被包含的路径，或者文件扩展名。一份名字的白名单，比任何对内容的过滤都更强、也更短。
- **让进程没有能力造成破坏。** 一个以"只能在一个上传目录里写、不能执行自己写下的东西、对代码目录没有写权限"的用户运行的 PHP-FPM 池，会把一次绕过变成一起小得多的事件 —— 而且当出错的是**应用自己的检查**时，它仍然管用。
