---
id: upload-bypass-and-detection
title_en: "File Upload, Part 3 — Bypasses, Interpretation and Detection"
title_zh: "文件上传（三）：绕过、解释权与检测"
summary_en: Every check answers the question it was asked, and the exploitable ones answer a question about appearance rather than about capability. This entry walks the bypasses by what they attack — content inspection, re-encoding, the server's own handler rules, and the time window of the check — then states the one control that has no bypass.
summary_zh: 每一道检查都回答了它被问到的问题，而可被利用的那些，回答的是关于"像不像"的问题，不是关于"能不能"的问题。这一篇按攻击对象走一遍绕过 —— 内容检测、二次渲染、服务器自己的处理器规则、以及检查的时间窗 —— 然后给出那个没有绕过的控制。
tags: [web, file-upload, polyglot, htaccess, toctou, detection]
tools: [python3, exiftool, ImageMagick, Burp Suite, curl]
attck: [T1190, T1105, T1546]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### What each defence is actually being asked

The first two entries described the chain: an application decides on a **string**, a web server decides on **configuration**, and a filesystem decides on **a name**. This entry is about the bypasses, and each one is best understood by naming the question the defence answers.

| Defence | The question it answers | How it is bypassed | Why it fails |
|---|---|---|---|
| `Content-Type` check | What did the sender claim? | Change the claim | An assertion, not a property |
| Extension blacklist | Does the name contain a known-bad string? | Case, unlisted extensions | Finite list, and removal can concatenate |
| Extension allowlist | Is the final extension permitted? | Multiple extensions, unquoted attributes, handler rules | The check and the handler may read different tokens |
| Magic-byte check | Do the first bytes look like an image? | A valid header followed by a payload | It reads the start, the interpreter reads the whole file |
| Re-encoding | Can the bytes be rebuilt as a clean image? | Put the payload where the encoder does not look | The encoder rebuilds image data and copies metadata |
| Upload directory is not executable | Can this file ever become code? | **Nothing** | It removes the capability instead of testing for it |

The pattern is that the first five answer **"does this look dangerous"** and the last one answers **"can this be dangerous"**. Only one of those has a bypass, and the difference is not the care taken — it is whether the control is a test or a transformation.

### Bypassing content inspection: the polyglot

A magic-byte check verifies that a file begins with something recognisable. The file is free to continue with something else.

```python
# a valid 1x1 GIF, followed by executable PHP
payload = b'GIF89a' + b'\x01\x00\x01\x00\x00\x00\x00;' + b'<?php echo "PWNED"; ?>'
open('shell.gif.php', 'wb').write(payload)
```

Two components read that one file and reach different conclusions:

```
$ file -b shell.gif.php
GIF image data, version 89a, 1 x 1

$ php shell.gif.php
GIF89a   ;PWNED
```

The identification tool reports a legitimate image; the interpreter executes the payload and emits the GIF header as text on the way. **Neither is broken** — they are reading different parts of the file, and the check was performed by the one that reads the beginning.

The same trick works with more care taken, in places a validator is even less likely to look:

| Placement | Why it survives |
|---|---|
| Appended after the image data | Many formats tolerate trailing bytes |
| JPEG `COM` (comment) segment | Part of the format, ignored by decoders |
| PNG `tEXt` chunk | Metadata, carried through by most tools |
| EXIF fields | Written by `exiftool`, preserved by many pipelines |
| Inside `IDAT` / compressed data | Requires care, but decoders ignore what they cannot use |

The general form: **anything the parser ignores is free space for the payload.** That is why content inspection raises cost rather than removing ability.

### Bypassing re-encoding, and why it is still the strongest content control

Re-encoding is different in kind from the checks above. The application does not examine the file — it **decodes it into pixels and writes a new image**. Payload bytes that are not pixel data have nowhere to survive, and this is the closest thing in upload defence to parameterisation: instead of asking whether the input is dangerous, it **reconstructs the value** so that only the intended meaning remains.

It is not absolute, because encoders preserve some things deliberately. Measured on a 16x16 image with the payload placed in three different positions:

| Where the payload sits | Survives re-encoding? |
|---|---|
| Appended after the GIF image data | **No** — the trailer is discarded |
| JPEG `COM` comment segment | **Yes** — the comment is copied into the new file |
| PNG `tEXt` chunk | **Yes** — the text chunk is carried across |

The lesson is precise rather than discouraging: **the encoder rebuilds image data and copies metadata.** A payload in the pixels is destroyed; a payload in the metadata survives. That gives the defence a specific requirement — re-encode **and** strip metadata — and it gives the test a specific question: after processing, does the output still contain the string the input contained?

```python
# what to check after re-encoding, rather than trusting that it happened
after = open(reencoded_path, 'rb').read()
for marker in (b'<?php', b'<%', b'<script', b'${', b'#!/'):
    assert marker not in after, f'payload survived re-encoding: {marker}'
```

And a warning that belongs with it: **a re-encode that silently fails leaves the original in place.** If the code is `try: reencode() except: pass`, the strongest control in the list becomes a no-op exactly when the input is unusual enough to matter. Verified output is the requirement, not attempted transformation.

### Changing the rules instead of satisfying them

The most elegant bypass in this family does not attack the check at all. It attacks **the thing that decides what a file means.**

Apache reads `.htaccess`, PHP-FPM reads `.user.ini`, IIS reads `web.config`. All three are configuration files that a directory may contain, and all three can change how other files in that directory are handled:

```apache
# .htaccess — every .jpg in this directory is now a PHP script
AddType application/x-httpd-php .jpg
```

```ini
; .user.ini — prepend a file to every script executed in this directory
auto_prepend_file = shell.jpg
```

```xml
<!-- web.config — add a handler mapping for an extension that was allowed -->
<configuration><system.webServer><handlers>
  <add name="x" path="*.jpg" verb="*" modules="IsapiModule" scriptProcessor="...php-cgi.exe" />
</handlers></system.webServer></configuration>
```

Read what happened: **the uploaded file's extension is on the allowlist.** `shell.jpg` is exactly what the application permitted. It executes because a second file changed the server's interpretation rules — the allowlist was never wrong, it was answering a question about the name, and the answer stopped being relevant.

Three consequences for defence, all structural:

- **Configuration files must be refused by name, not by extension.** `.htaccess`, `.user.ini`, `web.config`, plus the framework-specific equivalents. The list is short, it is known, and there is no legitimate reason for a user upload to be one of them.
- **The upload directory must not allow them to take effect.** `AllowOverride None` in Apache, equivalent settings elsewhere. A defence that depends on the file not being uploadable is one configuration change away from failing; a defence that depends on the file not being *read* is not.
- **Never put the upload directory inside a path whose configuration the application does not control.** Share nothing with the code tree.

### The time dimension: check-then-use

Some designs check after storing: the file is written, a scanner examines it, and it is deleted if unacceptable. The window between the write and the delete is exploitable, and it does not require any parsing trick at all.

```
t0  file written to /uploads/xyz.jpg        <- requester can fetch it NOW
t1  scanner runs
t2  if bad: unlink
```

Request the file at `t0` and the application's own storage directory serves it before judgement. Common variants:

| Variant | The window |
|---|---|
| Async antivirus scan | Between write and verdict |
| Validation after a redirect | The response to the upload already exists |
| Cleanup job on a schedule | Until the job runs |
| A queue the file passes through | Full queue latency |

The underlying issue is the same trust-boundary problem in a different dimension: **the object that was checked and the object that is used are not guaranteed to be the same object at the same time.** The fix is to change the **order** rather than to shorten the window:

- **Validate before the file is reachable.** Write to a staging location that is not served, validate there, and only then move into the served location.
- **If validation must be asynchronous, do not serve from the staging area at all** — and make the move the last step, performed by the validator.
- **Do not delete-and-hope.** Removal leaves a window; a move out of the reachable area does not have the same race, because the file was never reachable.

### Bypassing the filter rather than the logic

A network filter inspecting multipart bodies is a third parser in the request path, and it can be made to disagree with the application about the same body.

| Technique | The disagreement |
|---|---|
| `filename=` versus `filename*=` (RFC 5987 extended form) | Some parsers read one, some the other; a filter matching the visible `filename` may miss what the application stores |
| Repeated `filename` parameters | Which one wins — the filter's choice may not be the framework's |
| Percent-encoding in the filename | Decoded once by the filter, once by the application, or not at all |
| Case and double extensions | The signature list is finite; the extension grammar is not |
| Chunked transfer encoding, oversized bodies | Parts of the body a filter skips or truncates are still read by the application |
| Content-Type placeholders | A part declared as text and handled as a file by the application |

That last row is the shape worth testing first on any defended target, because it needs no obfuscation: **find an input the filter rates as harmless and the application rates as important.** It is the same exercise as the XSS entry's filter section, applied to the request envelope.

### Detection and mitigation

- **Alert on the interpretation-changing filenames above all.** `.htaccess`, `.user.ini`, `web.config`, `php.ini`, and equivalents appearing as an upload are not a suspicious payload — they are an attempt to **change how the server reads every other file**, and there is no business case for them.
- **Detect the mismatch, not the extension.** A part declared `image/jpeg` whose body contains `<?php`, `<%`, `${`, `#!/`, or a script tag is a claim contradicted by its bytes, and both halves are available for comparison at no cost. So is an upload whose stored content contains an executable marker after the validation step claims to have processed it.
- **Watch the pair of requests, not just the upload.** An upload followed within seconds by a request for the uploaded path — from the same session or the same address — is the pattern of someone testing what they just placed. On its own it is not proof; combined with a non-image stored name it is close to conclusive.
- **Log the four names and the resolved path** (client name, canonical name, stored name, served path), and log the archive member list when archives are involved. The evidence for these attacks is in the differences between values that are logged separately, not in any single one.
- **Detect the outcome**: a new file in the web root, a new symlink in the upload directory, a request for a path the application never generated, a modification to a file the application does not write. Integrity monitoring catches the construction nobody predicted, which is exactly the case that bypasses a signature.
- **For mitigation, prefer transformation to validation.** Re-encode images and verify the output contains no executable marker; generate the stored name; keep uploads outside the code tree; refuse configuration filenames; serve with `Content-Disposition: attachment` and `X-Content-Type-Options: nosniff`; and put user content on a separate origin.
- **Order operations so that the unreachable state comes first.** Staging, validation, then publication. The check-then-use window is closed by sequence, not by speed.
- **And keep the enablement test in the deployment checklist.** Upload a benign file and request it. If the server returns it as text, the directory is not an execution context and most of this entry is moot; if it executes, every other control is load-bearing. That single request tells you which world you are in.

### The one control without a bypass

Across three entries, every defence that **tests** a file has been bypassed: the declared type, the blacklist, the allowlist, the magic bytes, and — with metadata — even re-encoding. What has not been bypassed is the control that removes the capability:

> **A file that cannot reach an interpreter cannot be executed, regardless of what it contains or what it is called.**

That is the same conclusion as every other entry in this series, stated for files. Parameterised queries work because the data never becomes a query; context-aware encoding works because the value never becomes markup; **storing user content where nothing executes works because the file never becomes code.** Everything else in this entry is cost, detection, and defence in depth — useful, and none of it load-bearing on its own.

<!-- lang:zh -->
### 每道防御其实被问的是什么

前两篇描述了那条链：应用基于**字符串**决定，Web 服务器基于**配置**决定，文件系统基于**名字**决定。这一篇讲绕过，而每一个绕过最好的理解方式，是说清那道防御回答的是**哪个问题**。

| 防御 | 它回答的问题 | 怎么绕 | 为什么失效 |
|---|---|---|---|
| `Content-Type` 检查 | 发送方声称了什么？ | 改那个声称 | 是断言，不是属性 |
| 扩展名黑名单 | 名字里有没有已知的坏串？ | 大小写、清单外的扩展名 | 清单有限，而删除还可能拼接 |
| 扩展名白名单 | 最后那个扩展名允许吗？ | 多扩展名、不带引号的属性、处理器规则 | 检查与处理器可能读不同的记号 |
| 魔数检查 | 头几个字节像不像图片？ | 合法文件头后面接载荷 | 它读开头，解释器读整个文件 |
| 二次渲染 | 字节能不能被重建成一张干净的图？ | 把载荷放到编码器不看的地方 | 编码器重建图像数据、原样搬运元数据 |
| **上传目录不可执行** | **这个文件有没有机会变成代码？** | **没有** | **它移除的是能力，而不是去检测** |

模式是：前五个回答的是"**它看起来危险吗**"，最后一个回答的是"**它能不能危险**"。只有一个有绕过，而差别不在用心程度 —— 在于那项控制是一次**检测**还是一次**变换**。

### 绕过内容检测：多语义文件

魔数检查确认一个文件以某个可识别的东西开头。而这个文件完全可以接着放别的东西。

```python
# 一个合法的 1x1 GIF，后面跟着可执行的 PHP
payload = b'GIF89a' + b'\x01\x00\x01\x00\x00\x00\x00;' + b'<?php echo "PWNED"; ?>'
open('shell.gif.php', 'wb').write(payload)
```

两个组件读同一个文件，得出不同结论：

```
$ file -b shell.gif.php
GIF image data, version 89a, 1 x 1

$ php shell.gif.php
GIF89a   ;PWNED
```

识别工具报告这是一张合法图片；解释器执行了载荷，顺手把 GIF 头当文本输出了。**两个都没坏** —— 它们读的是这个文件的不同部分，而检查是由"读开头"的那一个做的。

同样的手法可以做得更讲究，放到校验更不会去看的地方：

| 位置 | 为什么能活下来 |
|---|---|
| 图像数据之后追加 | 很多格式容忍尾随字节 |
| JPEG 的 `COM`（注释）段 | 格式的一部分，解码器忽略 |
| PNG 的 `tEXt` 块 | 元数据，多数工具会带过去 |
| EXIF 字段 | 由 `exiftool` 写入，很多流水线会保留 |
| `IDAT`／压缩数据内部 | 需要讲究，但解码器会忽略它用不上的东西 |

通用形式是：**解析器忽略的任何地方，都是载荷的自由空间。** 这就是为什么内容检测提高的是成本，而不是移除能力。

### 绕过二次渲染，以及为什么它仍是最强的内容控制

二次渲染在**种类上**就与上面那些检查不同。应用不是在检查文件 —— 它把文件**解码成像素、再写出一张新图**。不是像素数据的载荷字节无处可存，而这是上传防御里最接近"参数化"的东西：它不去问输入危不危险，而是**重建那个值**，让只剩下原本该有的含义。

它不是绝对的，因为编码器会**有意保留**一些东西。在一张 16x16 的图上，把载荷放在三个不同位置实测：

| 载荷放在哪 | 二次渲染后还在吗 |
|---|---|
| GIF 图像数据之后追加 | **不在** —— 尾部数据被丢弃 |
| JPEG 的 `COM` 注释段 | **在** —— 注释被复制进新文件 |
| PNG 的 `tEXt` 块 | **在** —— 文本块被带过去 |

教训很精确，也不令人沮丧：**编码器重建图像数据、原样搬运元数据。** 放在像素里的载荷会被销毁；放在元数据里的会活下来。这就给防御提出了具体要求 —— 二次渲染**并且**剥离元数据 —— 也给测试提出了具体问题：处理之后，输出里还含不含输入里含的那个字符串？

```python
# 二次渲染之后该检查的东西，而不是相信它"已经发生了"
after = open(reencoded_path, 'rb').read()
for marker in (b'<?php', b'<%', b'<script', b'${', b'#!/'):
    assert marker not in after, f'载荷在二次渲染后仍存在: {marker}'
```

以及一条该跟着它的警告：**一次静默失败的二次渲染会把原文件留在原地。** 如果代码写成 `try: reencode() except: pass`，那么这项列表里最强的控制，恰恰在输入古怪到真正要紧的时候变成了空操作。要求的是**被验证过的输出**，不是"尝试过的变换"。

### 不满足规则，而是改规则

这一类里最优雅的绕过根本不攻击那道检查。它攻击的是**决定一个文件是什么意思的那个东西**。

Apache 读 `.htaccess`，PHP-FPM 读 `.user.ini`，IIS 读 `web.config`。三者都是"某个目录可以包含的配置文件"，而三者都能改变同目录下其他文件的处理方式：

```apache
# .htaccess —— 这个目录里的每个 .jpg 现在都是 PHP 脚本
AddType application/x-httpd-php .jpg
```

```ini
; .user.ini —— 给这个目录里执行的每个脚本前置一个文件
auto_prepend_file = shell.jpg
```

```xml
<!-- web.config —— 给一个已被允许的扩展名加一条处理器映射 -->
<configuration><system.webServer><handlers>
  <add name="x" path="*.jpg" verb="*" modules="IsapiModule" scriptProcessor="...php-cgi.exe" />
</handlers></system.webServer></configuration>
```

看清楚发生了什么：**被上传文件的扩展名在白名单上。** `shell.jpg` 正是应用允许的东西。它被执行，是因为**第二个文件改掉了服务器的解释规则** —— 白名单从来没错，它回答的是关于名字的问题，而那个答案已经不相干了。

对防御的三个结论，都是结构性的：

- **配置文件必须按名字拒绝，而不是按扩展名。** `.htaccess`、`.user.ini`、`web.config`，加上各框架的对应物。清单很短、是已知的，而且用户上传没有任何正当理由成为其中之一。
- **上传目录必须让它们无法生效。** Apache 里 `AllowOverride None`，其他地方用等价设置。一项依赖"这个文件传不上来"的防御，离失败只有一次配置改动；一项依赖"这个文件不会被读"的防御则不是。
- **永远不要把上传目录放进一个应用无法控制其配置的路径里。** 不要与代码树共享任何东西。

### 时间维度：先检查后使用

有些设计是**先存后查**：文件写下去、扫描器检查、不合格就删掉。写入与删除之间的那个窗口是可利用的，而且它完全不需要任何解析技巧。

```
t0  文件被写到 /uploads/xyz.jpg        <- 请求方现在就能取它
t1  扫描器运行
t2  如果不合格：unlink
```

在 `t0` 请求那个文件，应用自己的存储目录会在判决之前把它提供出去。常见变体：

| 变体 | 那个窗口 |
|---|---|
| 异步杀毒扫描 | 写入与结论之间 |
| 重定向之后才校验 | 对上传的响应已经存在了 |
| 定时清理任务 | 直到任务运行为止 |
| 文件经过一个队列 | 整个队列延迟 |

底层问题是同一个信任边界问题换了个维度：**被检查的那个对象，与正在被使用的那个对象，不保证是同一时刻的同一个对象。** 修法是改变**顺序**，而不是缩短窗口：

- **让文件在被校验之前不可达。** 写到一个不被提供服务的暂存位置，在那里校验，通过了才移进被服务的位置。
- **如果校验必须异步，那就完全不要从暂存区提供服务** —— 并且让"移动"成为最后一步，由校验器执行。
- **不要"删掉然后祈祷"。** 删除会留下窗口；把它移出可达区域没有同样的竞态，因为那个文件从来就不可达。

### 绕过过滤器，而不是绕过逻辑

检查 multipart 请求体的网络过滤器，是请求路径上的**第三个解析器**，而它和一个应用对同一个请求体产生分歧是能做到的。

| 手法 | 那个分歧 |
|---|---|
| `filename=` 对 `filename*=`（RFC 5987 扩展形式） | 有的解析器读一个、有的读另一个；匹配可见 `filename` 的过滤器可能漏掉应用真正存下的东西 |
| 重复的 `filename` 参数 | 哪一个胜出 —— 过滤器的选择未必是框架的选择 |
| 文件名里的百分号编码 | 过滤器解一次、应用解一次，或者都不解 |
| 大小写与双扩展名 | 签名清单有限；扩展名语法不是 |
| 分块传输编码、超大请求体 | 过滤器跳过或被截断的那部分，应用仍然会读 |
| Content-Type 占位 | 一个被声明为文本、却被应用当文件处理的分片 |

最后一行是任何有防御的目标上都该先测的形状，因为它不需要任何混淆：**找一个过滤器判为无害、应用判为要紧的输入。** 这和 XSS 那篇的过滤器一节是同一个练习，只是作用在请求信封上。

### 检测与缓解

- **首要告警的是那些"改变解释权"的文件名。** `.htaccess`、`.user.ini`、`web.config`、`php.ini` 以及对应物作为一个上传出现，不是可疑载荷 —— 那是企图**改变服务器如何读取其他每一个文件**，而它们没有任何业务理由。
- **检测不一致，而不是检测扩展名。** 一个声明为 `image/jpeg`、正文里却含 `<?php`、`<%`、`${`、`#!/` 或脚本标签的分片，是"被自己的字节反驳的声称"，而两半都可以零成本拿到并比对。同样便宜的还有：一个上传的存储内容在校验步骤声称已处理之后，仍含可执行标记。
- **盯"那一对请求"，而不只是上传。** 一次上传在数秒内被同一个会话或同一个地址请求它自己的路径 —— 那是有人在测试自己刚放上去的东西。单独看不算证据；配上"存下来的名字不是图片"，就接近确凿。
- **记下四个名字和解析后的路径**（客户端名、规范名、存储名、服务路径），涉及归档时还记下成员清单。这些攻击的证据在**分别记录的值之间的差**里，不在任何单独一个里。
- **检测结果**：Web 根里出现新文件、上传目录里出现新符号链接、有人请求应用从未生成过的路径、应用自己不写的文件被改动。完整性监控抓的是**谁也没预料到的构造方式** —— 而那恰恰是绕过签名的那种情况。
- **缓解上，宁愿变换而不是校验。** 二次渲染图片并验证输出里不含可执行标记；生成存储名；把上传放在代码树之外；按名字拒绝配置文件；用 `Content-Disposition: attachment` 与 `X-Content-Type-Options: nosniff` 提供；并把用户内容放到独立的源上。
- **把操作顺序排成"先不可达"。** 暂存、校验、再发布。先检查后使用的那个窗口是由**顺序**关掉的，不是由速度。
- **并且把那个可执行性测试留在部署清单里。** 上传一个无害文件再请求它。如果服务器把它当文本返回，这个目录就不是执行上下文，这一篇大半内容都不适用；如果它执行了，那么其他每一项控制都是承重的。那一个请求会告诉你自己身处哪个世界。

### 那个没有绕过的控制

三篇下来，每一道**检测**文件的防御都被绕过过：声明的类型、黑名单、白名单、魔数，甚至（靠元数据）二次渲染。没有被绕过的是那个**移除能力**的控制：

> **一个到不了解释器的文件无法被执行，无论它里面是什么、或者它叫什么名字。**

这和本系列其他每一篇是同一个结论，只是针对文件说一遍。参数化查询之所以有效，是因为数据从来没有变成查询；上下文感知编码之所以有效，是因为值从来没有变成标记；**把用户内容存到不会执行的地方之所以有效，是因为文件从来没有变成代码。** 这一篇里其余的一切都是成本、检测和纵深防御 —— 有用，但没有一项单独承重。
