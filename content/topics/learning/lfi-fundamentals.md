---
id: lfi-fundamentals
title_en: "LFI, Part 1 — When a Name Becomes a Path"
title_zh: "LFI（一）：当名字变成路径"
summary_en: A file download reads a name and the filesystem reads a path, which is the same disagreement as the archive entry with a different name space. This entry covers the join trap, the three layers at which filtering fails, and the difference between a filter that blocks and one that deletes.
summary_zh: 一个文件下载读的是"名字"，而文件系统读的是"路径" —— 这和归档那篇是同一个分歧，只是名字空间换了。这一篇讲 join 那个陷阱、讲过滤在哪三个层次上失效，以及"拦截式过滤器"和"删除式过滤器"的区别。
tags: [web, lfi, path-traversal, cwe-22, cwe-98, file-read]
tools: [curl, Burp Suite, python3, php]
attck: [T1005, T1083]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The same disagreement as the archive entry

A download endpoint takes a **name** and turns it into a **path**. The archive entry described exactly this shape: an archive member is a name, and extraction writes it onto a filesystem where it means a path. Here it is one string instead of a list, and the consequences are the same.

> **The application reads the input as a name. The filesystem reads it as a path. Everything between those two readings is the vulnerability.**

The distinction matters because it decides what a defence can be. A name has no structure — it is a label. A path has grammar: separators, a parent reference, an absolute form, and a resolution algorithm involving symbolic links. As long as the input is treated as a name, it cannot express "go up two levels"; the moment it reaches a path-aware function, it can.

There is a second reading error stacked on top of the first, and it is the one that catches experienced developers: **the function that looks like it safely combines a base directory with a user-supplied name does not always do so.**

```python
import os.path, posixpath

print(posixpath.join('/srv/files', 'report.md'))     # /srv/files/report.md
print(posixpath.join('/srv/files', '/etc/passwd'))   # /etc/passwd      <- the base is gone
print(posixpath.join('/srv/files', '../private/x'))  # not normalised — join does not resolve ..
```

The second line is the trap. An **absolute** second argument discards everything before it, so a name beginning with `/` escapes the base directory completely, and the code that called `join` never looks wrong. The third line compounds it: `join` does not resolve `..`, so even the relative case is a path that has not been normalised when the next function receives it.

This is why "we use `os.path.join`, so we are safe from traversal" is not a control. It is a function that combines strings according to rules that do not match the caller's mental model.

### Three layers at which filtering fails

Almost every attempt to fix traversal by filtering does so at the wrong layer. Naming the layers makes the failures predictable.

#### Layer one: the encoding

A filter operates on a string; the application may decode that string again before using it. The gap between the two is the attack.

This was measured on a challenge whose filter inspects the **first** decoded value while the application decodes a **second** time:

| Input | After the filter's decode | After the application's second decode |
|---|---|---|
| `%2e%2e%2f` | `../` — **blocked** | — |
| `%252e%252e%252f` | `%2e%2e%2f` — passes the filter | `../` — **traversal** |

And the same trick applies to the file **name** when the filter also blocks sensitive names: encoding just the characters that match the pattern is enough.

| Input | Filter sees | Application resolves |
|---|---|---|
| `ops-token.txt` | matches the blocked name — **rejected** | — |
| `ops-%2574oken.txt` | `ops-%2574oken.txt` — no match | `ops-token.txt` |

Two generalisable points:

**The question to ask about any filter is how many times it decodes, and how many times the application does.** If the numbers differ, there is a bypass, and the number of encoding layers needed is exactly the difference.

**Only the matching characters need encoding.** A filter matching a literal string is defeated by encoding a single character of it — the pattern is on the decoded value the filter computed, not on the character sequence that will eventually be used.

#### Layer two: the normalisation

A filter that blocks the literal `..` is blocking a **string**, while the filesystem interprets a **path**. Between the two there is a normalisation step the filter is not performing:

| Form | Why a literal `..` check misses it |
|---|---|
| Multiple separators | `....//` — the filter's view depends on whether it **deletes** matches or **rejects** them |
| Encoded separators | `..%2f`, `%2e%2e/` handled by different layers |
| Redundant segments | `./../`, `a/../../` — the path resolves upward even when no single `..` looks dangerous in context |
| Windows semantics | Trailing dots and spaces, `\` as a separator, alternate data streams, short names |
| Absolute paths | **No `..` at all** — the base directory is simply discarded |

That last row is worth restating, because it is the cleanest example of a filter answering the wrong question: a request containing **no traversal sequence whatsoever** can still read outside the intended directory, because the input was absolute. **A defence aimed at `..` does not see it.**

#### Layer three: the semantics

`..` is not the only way to reach a file outside a directory:

| Mechanism | Note |
|---|---|
| **Absolute path** | Discards the base; needs no traversal syntax |
| **Symbolic link** | A link inside the allowed directory pointing out of it; the path looks innocent and resolution is what escapes |
| **Windows device and stream syntax** | `::`, reserved names, UNC paths |
| **Directory junction** | The Windows equivalent of a symlink at directory level |
| **Case and short names** | Two spellings, one file |

A filter that only understands `..` is filtering one of six mechanisms.

### Blocking versus deleting, and why it changes the payload

This distinction is worth its own section because it decides which payloads are even candidate solutions, and getting it wrong wastes time on inputs that were never going to work.

**A deleting filter removes matches from the input.** The consequence is the one already seen in the XSS entry: **removal can concatenate**.

```
input:   ....//../private/x
delete '../'  ->  ../../private/x        <- still traversal
```

Deleting `../` from `....//../private/x` leaves `../../private/x`, which still traverses. This is the same structural failure as a blacklist that removes `<script>` and thereby assembles a new one.

**A blocking filter rejects the whole request when anything matches.** Concatenation tricks do nothing here, because the match is evaluated on the input as received:

```
input:   ....//private/x
contains '..'  ->  rejected outright
```

So the first useful question about a filtered endpoint is **which kind it is**, and the second is **how many times each side decodes**. Only after those two are answered does the payload follow:

| Filter kind | What to try |
|---|---|
| Deleting | Inputs that reassemble the sequence after removal |
| Blocking | Encoding layers, normalisation differences, absolute paths, symlinks |
| Either | Mechanisms in the third layer, since none of them involves `..` |

And one piece of knowledge that belongs here because it saves a wrong turn: **overlong UTF-8 encodings of `.` and `/` (`%c0%ae`, `%c0%af`) do not work in current runtimes.** Python's `unquote` treats them as invalid UTF-8 and drops them. Knowing that an old technique has been fixed is as useful as knowing a new one.

### The fix that removes the problem

The ranking follows the same pattern as every other entry: remove the ability rather than constrain it.

**Strongest: do not let the input be part of a path at all.** Give the user an identifier and map it to a path server-side.

```python
# the input selects an entry; it does not contribute to a path
FILES = {"q1-report": "/srv/files/q1-report.txt", "onboarding": "/srv/files/onboarding.txt"}

name = request.args.get("file")
path = FILES.get(name)
if path is None:
    abort(404)
return send_file(path)
```

There is no traversal here because there is no path arithmetic — the input selects from a fixed set, which is the same move as a parameterised query and the same move as storing uploads where nothing executes. **The input never becomes a path, so path syntax in the input is inert.**

**Next: canonicalise, then verify containment.** Where a name genuinely has to be used, resolve it the way the filesystem will and compare with a separator appended — the shape from the opening entry of this series:

```python
import os

def safe_path(base: str, name: str) -> str:
    base = os.path.realpath(base)
    target = os.path.realpath(os.path.join(base, name))
    if not target.startswith(base + os.sep):
        raise PermissionError("outside the base directory")
    return target
```

Two details carry the weight: `realpath` resolves `..` **and** symbolic links, and the comparison appends a separator so that `/srv/files-evil` is not mistaken for a child of `/srv/files`.

**Then: allowlist names.** If the set of legitimate names is small, comparing against it is simpler and stronger than any parsing.

**And: keep the process unable to reach what it should not.** Sensitive files outside the web root, a service account with read access to the document directory and nothing else, and no credentials in files the download endpoint can reach. That is the layer that holds when the code is wrong.

### Detection and mitigation

- **Alert on path syntax arriving in a name parameter.** `../`, `..%2f`, `%2e%2e`, an absolute path, a leading `~`, backslashes where the platform uses them, `::`, and UNC-style prefixes. None of these is a legitimate file name in a business application, which makes this a low-noise rule.
- **Alert specifically on double-encoding markers.** A `%25` inside a file or path parameter means a percent sign was itself encoded, which is either a double-encode attempt or a very unusual client. In a download endpoint it is almost always the former, and it is the signature of a filter that decodes once while the application decodes twice.
- **Watch the responses, not only the requests.** A download of a file the endpoint was never meant to serve shows up as an **unusual response size or content type** for that endpoint. So does a request that resolves outside the base directory: the server's own log of the resolved path is the evidence, which is why logging that resolved path is worth doing.
- **Treat 403 and 404 bursts as enumeration.** Traversal is often preceded by probing for which depths and which names exist; a run of failures from one source against a download parameter is the reconnaissance, not the attack.
- **For mitigation, prefer identifiers to names, and canonicalise when names are unavoidable.** Verify with a test that feeds `../`, an absolute path, a symlink pointing outside, and a double-encoded sequence, and asserts refusal — because a containment check that is not covered by a test is one that a refactor will quietly remove.
- **Do not filter the traversal sequence.** It answers a string question about a path-shaped problem, and the three layers above are the reason. If a filter is retained for cost reasons, treat it as detection and keep the real control at the path layer.
- **And check the encoding count.** Whatever sits in front of the application and whatever the application does must decode the same number of times; a mismatch is a bypass regardless of how good the filter's pattern list is.

<!-- lang:zh -->
### 和归档那篇是同一个分歧

一个下载端点接收一个**名字**，把它变成一个**路径**。归档那篇描述的正是这个形状：归档成员是一个名字，而解压把它写到文件系统上，在那里它意味着路径。这里只是一个字符串而不是一份清单，后果相同。

> **应用把输入读作名字，文件系统把它读作路径。这两次读法之间的全部东西，就是漏洞。**

这个区分要紧，因为它决定防御能是什么。名字没有结构 —— 它只是一个标签。路径有语法：分隔符、父目录引用、绝对形式，以及一套涉及符号链接的解析算法。只要输入被当作名字，它就无法表达"往上走两层"；而在它到达一个懂路径的函数的那一刻，它就可以。

在第一次误读之上还叠着第二次，而这一次专抓有经验的开发者：**那个看起来会"把基准目录和用户提供的名字安全地组合起来"的函数，并不总是真的如此。**

```python
import os.path, posixpath

print(posixpath.join('/srv/files', 'report.md'))     # /srv/files/report.md
print(posixpath.join('/srv/files', '/etc/passwd'))   # /etc/passwd      <- 基准没了
print(posixpath.join('/srv/files', '../private/x'))  # 未规范化 —— join 不解析 ..
```

第二行就是那个陷阱。一个**绝对**的第二个参数会把它前面的一切丢掉，所以一个以 `/` 开头的名字完全逃出了基准目录 —— 而调用 `join` 的那段代码看起来毫无问题。第三行让它更糟：`join` 不解析 `..`，所以即使是相对的那种情况，交给下一个函数的也是一条尚未规范化的路径。

这就是为什么"我们用了 `os.path.join`，所以不怕穿越"不是一项控制。它是一个按"与调用者心智模型不符的规则"拼接字符串的函数。

### 过滤失效的三个层次

几乎每一次"靠过滤来修穿越"的尝试，都修在了错误的层次上。把这些层次命名出来，失效就变得可预测了。

#### 第一层：编码

过滤器作用在一个字符串上；而应用在使用它之前，可能**再解码一次**。两者之间的缝就是攻击。

在一个"过滤器看**第一次**解码后的值、而应用解码**第二次**"的题目上实测：

| 输入 | 过滤器解码后 | 应用第二次解码后 |
|---|---|---|
| `%2e%2e%2f` | `../` —— **被拦** | — |
| `%252e%252e%252f` | `%2e%2e%2f` —— 通过过滤器 | `../` —— **穿越成功** |

同样的手法也适用于**文件名**那一关：当过滤器同时封禁敏感名字时，**只编码匹配到的那几个字符**就够了。

| 输入 | 过滤器看到 | 应用解析为 |
|---|---|---|
| `ops-token.txt` | 命中被封的名字 —— **拒绝** | — |
| `ops-%2574oken.txt` | `ops-%2574oken.txt` —— 不命中 | `ops-token.txt` |

两个能推广的要点：

**对任何过滤器要问的问题是：它解码几次，而应用解码几次。** 数字不同就有绕过，而所需的编码层数恰好是那个差。

**只需要编码命中的那些字符。** 一个匹配字面串的过滤器，会被"编码其中单个字符"打穿 —— 因为那个模式是作用在过滤器自己算出来的解码值上的，不是作用在最终会被使用的字符序列上的。

#### 第二层：规范化

一个封禁字面 `..` 的过滤器，封的是一个**字符串**，而文件系统解释的是一个**路径**。两者之间有一个过滤器没有做的规范化步骤：

| 形式 | 为什么字面 `..` 检查漏掉它 |
|---|---|
| 多重分隔符 | `....//` —— 过滤器的视图取决于它是**删除**匹配还是**拒绝**匹配 |
| 编码的分隔符 | `..%2f`、`%2e%2e/`，由不同层次处理 |
| 冗余段 | `./../`、`a/../../` —— 即使单看某个 `..` 在上下文里不危险，路径仍然往上解析 |
| Windows 语义 | 末尾的点与空格、`\` 作为分隔符、备用数据流、短名 |
| 绝对路径 | **一个 `..` 都没有** —— 基准目录被直接丢弃 |

最后一行值得再说一遍，因为它是"过滤器回答了错的问题"最干净的例子：一个**完全不含穿越序列**的请求，仍然能读到预期目录之外，因为那输入是绝对的。**一项针对 `..` 的防御看不见它。**

#### 第三层：语义

`..` 不是到达目录外文件的唯一方式：

| 机制 | 说明 |
|---|---|
| **绝对路径** | 丢弃基准；不需要任何穿越语法 |
| **符号链接** | 允许目录里的一个链接指向外面；路径看起来无辜，逃逸发生在解析时 |
| **Windows 设备与数据流语法** | `::`、保留设备名、UNC 路径 |
| **目录联接（junction）** | Windows 上目录级别的等价物 |
| **大小写与短名** | 两种拼法，一个文件 |

一个只懂 `..` 的过滤器，在过滤六种机制里的一种。

### 拦截与删除，以及它为什么改变 payload

这个区分值得单独一节，因为它决定了哪些 payload 连候选都算不上；搞错了就会在"本来就不可能成功"的输入上浪费时间。

**删除式过滤器把匹配从输入里移除。** 后果就是 XSS 那篇已经见过的那个：**删除会拼接。**

```
输入:     ....//../private/x
删除 '../'  ->  ../../private/x        <- 仍然是穿越
```

从 `....//../private/x` 里删掉 `../` 得到 `../../private/x`，它照样穿越。这和"删掉 `<script>` 于是拼出一个新的"是同一个结构性失效。

**拦截式过滤器在有任何匹配时拒绝整个请求。** 拼接把戏在这里毫无用处，因为匹配是在**收到的输入**上判定的：

```
输入:     ....//private/x
含 '..'  ->  直接拒绝
```

所以对一个有过滤的端点，**第一个有用的问题是它是哪一种**，第二个是**两边各解码几次**。只有这两个答完，payload 才谈得上：

| 过滤器种类 | 该试什么 |
|---|---|
| 删除式 | 在删除之后能重新拼回那个序列的输入 |
| 拦截式 | 编码层、规范化差异、绝对路径、符号链接 |
| 两种都适用 | 第三层里的那些机制，因为它们都不涉及 `..` |

而有一条知识属于这里，因为它能省下一次走错路：**`.` 和 `/` 的超长 UTF-8 编码（`%c0%ae`、`%c0%af`）在当前运行时里不成立。** Python 的 `unquote` 会把它们当非法 UTF-8 处理掉。**知道一个老技巧已经失效，和知道一个新技巧一样有用。**

### 移除问题的那种修法

排序和本系列其他每一篇是同一个模式：移除能力，而不是限制用法。

**最强：根本不让输入成为路径的一部分。** 给用户一个标识符，在服务端把它映射到一条路径。

```python
# 输入只是选择了一个条目；它没有参与构造路径
FILES = {"q1-report": "/srv/files/q1-report.txt", "onboarding": "/srv/files/onboarding.txt"}

name = request.args.get("file")
path = FILES.get(name)
if path is None:
    abort(404)
return send_file(path)
```

这里没有穿越，因为没有路径运算 —— 输入从一个固定集合里选择。这和参数化查询是同一个动作，也和"把上传存到不会执行的地方"是同一个动作。**输入从来没有变成路径，所以输入里的路径语法是惰性的。**

**其次：先规范化，再验证包含关系。** 当确实必须使用一个名字时，按文件系统会用的方式解析它、并带上分隔符比较前缀 —— 本系列开篇给过的形状：

```python
import os

def safe_path(base: str, name: str) -> str:
    base = os.path.realpath(base)
    target = os.path.realpath(os.path.join(base, name))
    if not target.startswith(base + os.sep):
        raise PermissionError("outside the base directory")
    return target
```

两个细节承重：`realpath` 会解析 `..` **和**符号链接；而比较时补上了分隔符，这样 `/srv/files-evil` 不会被误认为 `/srv/files` 的子目录。

**然后：白名单文件名。** 如果合法名字的集合很小，拿它去比对，比任何解析都更简单也更强。

**以及：让进程够不着它不该够着的东西。** 敏感文件放在 Web 根之外、服务账号只对文档目录有读权限、并且下载端点够得着的文件里没有凭据。那是代码写错时仍然管用的那一层。

### 检测与缓解

- **对"名字参数里出现路径语法"告警。** `../`、`..%2f`、`%2e%2e`、绝对路径、开头的 `~`、在平台用反斜杠时出现的反斜杠、`::`、UNC 风格前缀。在业务应用里这些都不是合法的文件名，所以这是一条低噪声规则。
- **特别对双重编码的标记告警。** 文件或路径参数里出现 `%25`，意味着一个百分号本身被编码了 —— 这要么是一次双重编码尝试，要么是一个极不寻常的客户端。在下载端点里它几乎总是前者，而它就是"过滤器解一次、应用解两次"的特征。
- **看响应，而不只是看请求。** 一个端点从不该提供的文件被下载，会表现为该端点**异常的响应大小或内容类型**。同样，一个解析到了基准目录之外的请求也会留下痕迹：服务器自己关于解析后路径的日志就是证据 —— 这也是记录那个解析后路径值得做的原因。
- **把 403 与 404 的爆发当成枚举。** 穿越之前常常是先探哪些深度、哪些名字存在；同一个来源对某个下载参数的一连串失败，那是侦察，不是攻击。
- **缓解上，优先用标识符而不是名字；名字不可避免时要规范化。** 用一条测试来核实：喂 `../`、一个绝对路径、一个指向外面的符号链接、以及一个双重编码序列，并断言拒绝 —— 因为一项没有被测试覆盖的包含检查，就是一次重构会悄悄拿掉的那种检查。
- **不要去过滤那个穿越序列。** 它是在用一个字符串问题回答一个路径形状的问题，而上面那三个层次就是理由。如果为了成本仍然保留一个过滤器，把它当检测用，把真正的控制留在路径那一层。
- **并且核对解码次数。** 挡在应用前面的那个东西和应用自己做的事，必须解码**同样多的次数**；次数不一致就是一条绕过，无论那个过滤器的模式清单写得多好。
