---
id: cmdi-fundamentals
title_en: "OS Command Injection, Part 1 — The Shell Is a Second Interpreter"
title_zh: "命令注入（一）：shell 是第二个解释器"
summary_en: A command is a program and the shell is its interpreter, so the same string is data in one call and code in another. Measured across shell and no-shell invocations, the metacharacter family, the whole class of whitespace substitutes, and where each language puts the boundary.
summary_zh: 命令是一个程序，而 shell 是它的解释器，所以同一个字符串在一次调用里是数据、在另一次里是代码。这一篇量了"经过 shell"与"不经过 shell"的差别、元字符族、整类空白字符替代写法，以及各语言把边界放在哪里。
tags: [web, command-injection, cwe-78, shell, rce]
tools: [python3, curl, bash, sh]
attck: [T1059, T1059.004, T1190]
platform: [web, linux, windows]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### A command is a program

Everything in this series has the same underlying shape, and command injection is its clearest form:

> **The application builds a command — and a command is a program, not a value.**

The shell reads that program. It has its own grammar: separators, substitution, redirection, globbing, variable expansion. So when a value from a request is concatenated into a command, the value enters a language with the ability to add statements — which is exactly the SQL injection shape, with a shell on the other side instead of a database.

Four layers, as with every injection:

1. **The trust boundary.** The application means to pass a *parameter* and instead passes *source code*. Both sides behave correctly: the shell does what a shell does with the string it was handed.
2. **Data and instruction share a plane.** The parameter and the separators are characters in one string, and nothing removes the structural meaning of the separators.
3. **Why the usual fix fails.** Escaping and denylists filter **characters**, while the shell's grammar lives at the **lexical** layer — and the lexical layer has more members than any hand-written list. The measured lab filter blocked spaces, semicolons, pipes, backticks, command substitution and redirection, and was defeated by a **newline** and a **variable that expands to whitespace**.
4. **The encoding and platform variants.** `$IFS`, newlines, wildcards, tab versus space, and — the one that costs the most time — the difference between `bash`, `dash` and `cmd.exe`.

The question that decides whether the vulnerability exists at all is the one worth asking first:

> **Where does this string end up being executed, and by what?**

### Whether a shell is involved

This is the largest single difference between stacks, and it changes the payload set completely. Measured, the same string through two Python calls:

```python
subprocess.run(f"echo {INJ}", shell=True)   # INJ = "example.com; id"
# -> example.com
#    uid=0(root) gid=0(root)                 the second command ran

subprocess.run(["echo", INJ])
# -> example.com; id                        the string is one argument
```

Same input, same program, and one of them is a vulnerability. The difference is not sanitisation — it is **whether the string is parsed by a shell**.

| Language | Goes through a shell | Does not |
|---|---|---|
| **Python** | `os.system`, `os.popen`, `subprocess(..., shell=True)` | `subprocess.run([...])` with a list |
| **PHP** | `system`, `exec`, `shell_exec`, backticks, `popen` | `proc_open` with an argument array |
| **Java** | an explicit `sh -c` | **`Runtime.exec(String)`** and `ProcessBuilder` |
| **Node.js** | `child_process.exec` | `execFile`, `spawn` with an argument array |
| **Go** | an explicit `sh -c` | **`exec.Command`** |
| **Ruby** | backticks, `system`, `%x`, `IO.popen` with a string | `IO.popen([...])` |

**Two rows deserve comment.** Java and Go both default to *not* using a shell, which removes the metacharacter family entirely — a payload of `; id` is simply an argument, as in the Python list example above. But they introduce a different problem, and it is a familiar one from this guide: **a String-based API that splits the command itself**. `Runtime.exec("ls -l /tmp")` does not call a shell, yet it still has to decide where the command ends and the arguments begin, and it does that by whitespace. So the program is parsed by **two** implementations with **two** sets of rules — the caller's and the runtime's — which is the same disagreement that makes WAF bypass and request smuggling work.

**And "no shell" does not mean "safe".** If the user controls an **argument** rather than the command, the danger is no longer metacharacters but the arguments themselves:

```
user-controlled value passed to curl:
  -o /etc/cron.d/payload       write the download anywhere
  --config /tmp/attacker.rc    load a config file
  @/etc/shadow                 read a file as a request body

user-controlled value passed to tar or git:
  --to-command=... , --checkout=... , -c core.sshCommand=...
```

This is **argument injection**, and it lives in exactly the code that was written carefully to avoid using a shell. The fix is different too: not escaping, but **ending the option list**.

### Metacharacters, and the whitespace class

Only meaningful when a shell is involved, and worth having complete because a filter that covers most of them is the normal case. Measured:

| Separator | Payload | Ran |
|---|---|---|
| Semicolon | `example.com;id` | yes |
| Pipe | `example.com\|id` | yes |
| Logical or | `example.com\|\|id` | yes |
| Logical and | `example.com&&id` | no — the first command failed |
| Backticks | `` example.com`id` `` | yes |
| Substitution | `example.com$(id)` | yes |
| **Newline** | `example.com\nid` | **yes** |

The newline row is the one filters miss, and the reason is a modelling error rather than an oversight: a newline is a **command separator of the same rank as a semicolon**, while a denylist is written as a list of *characters someone thought of as separators*. `&&` failing in that test is instructive in the other direction — it requires the preceding command to succeed, so it is not a reliable separator when the first command is deliberately broken.

**Whitespace is a category, not a character.** Measured, with the space removed from every payload:

| Substitute | Working payload |
|---|---|
| `${IFS}` | `cat${IFS}/etc/hostname` |
| `${IFS}${IFS}` or `$IFS$9` | `cat$IFS$9/etc/hostname` |
| Input redirection | `cat</etc/hostname` |
| Tab | `cat\t/etc/hostname` |

`IFS` is the shell's **Internal Field Separator**, and its default value is space, tab and newline — so a variable that expands to whitespace defeats any filter that only blocks the literal space character. The redirection form is a separate route: `<` needs no whitespace to separate a command from its argument.

**And what works depends on which shell is running.** The lab's filter blocked tab as well as space, but not the newline, so `%0A` was the first crack; and the syntactic sugar that a bash-minded payload list assumes does not exist in `dash`, which is `/bin/sh` on Debian-family systems:

| Syntax | bash | dash |
|---|---|---|
| `{cat,/etc/hostname}` | brace expansion | **not found** |
| `${IFS:0:1}` | substring expansion | **Bad substitution** |
| `$'\x20'` | ANSI-C quoting | **unsupported** |

Measured: the brace form fails on the lab's shell with `not found`. **So the payload set is a function of the target's shell, not of the technique** — determining which shell is running is part of the reconnaissance, and a payload that fails may be a payload for the wrong interpreter rather than a filter doing its job.

### Detection and mitigation

- **Alert on metacharacters in any input that reaches a command.** `;`, `|`, `&`, backtick, `$(`, `${`, newline, `>` and `<` in a hostname, filename, argument or search field are not accidental. The list should include the encoded forms (`%0A`, `%3B`) and the whitespace substitutes (`$IFS`), because a filter and a detector face the same enumeration problem and the detector has the advantage of seeing the payload.
- **Watch the process tree, which is the strongest signal available.** A web server or application process spawning `sh -c` is normal; that shell then spawning `id`, `whoami`, `curl`, `wget`, `nc`, `bash` or `cat` on a path outside the application directory is not. This detection survives every bypass, because it looks at what ran rather than at how the string was written.
- **Watch command output for content that should not be there.** `uid=`, `gid=`, the contents of `/etc/passwd`, a directory listing of the application root, or the contents of a file the application never reads. The lab's own pass condition is exactly this: the output has to contain `uid=`.
- **Watch for the time and network side channels, since command injection is often blind.** A request whose duration matches a `sleep` the input asked for; an outbound connection from the application host to an address in the input; a DNS lookup for a name the application had no reason to resolve. These are the observations that catch injection when the response body shows nothing.
- **For mitigation, do not use a shell.** Passing an argument array — the row in the table above that did not execute anything — removes the entire metacharacter class, because there is no interpreter left to give the characters meaning.
- **Where a shell is genuinely required, allowlist rather than denylist.** The measured lab filter is the argument: it blocked six classes of separator and was defeated by the seventh. An allowlist over the values that are acceptable (a hostname pattern, a filename from a known set, a numeric range) fails closed, which is the property a filter needs.
- **End the option list for anything that takes arguments.** A `--` before the user-controlled value stops it being read as an option, and rejecting values that begin with `-` closes the argument-injection route where no shell is involved.
- **Escape for the layer that will parse the string.** `escapeshellarg`-style quoting is correct **for a shell** and wrong where an argument array is used — quoting there becomes part of the argument. The escaping function and the invocation style have to be chosen together.
- **Run the process with the least privilege it needs.** Root was the measured context and it is a common one for operations panels, which is what turns a diagnostic tool into an administrative one. A dedicated low-privilege user bounds the consequence without preventing the bug.

<!-- lang:zh -->
### 一条命令就是一个程序

本系列的一切都有同一个底层形状，而命令注入是它最清楚的形式：

> **应用拼出来的是一条命令 —— 而命令是一个程序，不是一个值。**

shell 读这个程序。它有自己的一套文法：分隔符、替换、重定向、通配、变量展开。所以当一个来自请求的值被拼进一条命令时，这个值进入的是一门有"添加语句"能力的语言 —— 这和 SQL 注入是同一个形状，只是另一头是 shell 而不是数据库。

四层，和每一种注入一样：

1. **信任边界。** 应用想传的是一个*参数*，实际传的是*源码*。两边行为都正确：shell 对它拿到的那串字符做了 shell 该做的事。
2. **数据与指令共用同一平面。** 参数与分隔符是同一个字符串里的字符，而没有任何东西移除了分隔符的**结构含义**。
3. **为什么常见修法失败。** 转义与黑名单过滤的是**字符**，而 shell 的文法活在**词法**层 —— 而词法层的成员比任何手写清单都多。靶场里那份过滤器拦了空格、分号、管道、反引号、命令替换和重定向，却被一个**换行**和一个**展开成空白的变量**击败。
4. **编码与平台变体。** `$IFS`、换行、通配符、制表符与空格、以及最费时间的那个 —— `bash`、`dash` 与 `cmd.exe` 之间的差别。

决定这个漏洞到底存不存在的问题，值得第一个问：

> **这个字符串最终在哪里、被谁执行？**

### 中间有没有一个 shell

这是各技术栈之间最大的单个差异，而它完全改变 payload 集合。实测，同一个字符串走两次 Python 调用：

```python
subprocess.run(f"echo {INJ}", shell=True)   # INJ = "example.com; id"
# -> example.com
#    uid=0(root) gid=0(root)                 第二条命令运行了

subprocess.run(["echo", INJ])
# -> example.com; id                        这整个字符串是一个参数
```

同样的输入、同样的程序，而其中一个是漏洞。区别不在"有没有做过滤"—— 而在**这个字符串有没有被 shell 解析过**。

| 语言 | 经过 shell | 不经过 |
|---|---|---|
| **Python** | `os.system`、`os.popen`、`subprocess(..., shell=True)` | 传列表的 `subprocess.run([...])` |
| **PHP** | `system`、`exec`、`shell_exec`、反引号、`popen` | 传参数数组的 `proc_open` |
| **Java** | 显式写 `sh -c` | **`Runtime.exec(String)`** 与 `ProcessBuilder` |
| **Node.js** | `child_process.exec` | `execFile`、传参数数组的 `spawn` |
| **Go** | 显式写 `sh -c` | **`exec.Command`** |
| **Ruby** | 反引号、`system`、`%x`、传字符串的 `IO.popen` | `IO.popen([...])` |

**有两行值得说。** Java 与 Go 都默认**不**使用 shell，这就整个移除了元字符族 —— 一个 `; id` 的 payload 只是一个参数，和上面 Python 传列表的例子一样。但它们引入了另一个问题，而这个问题在本指南里很熟悉：**一个基于字符串、需要自己拆分命令的 API**。`Runtime.exec("ls -l /tmp")` 不调用 shell，可它仍然得判断命令在哪里结束、参数从哪里开始，而它是按空白来分的。于是这个程序被**两套实现、两套规则**解析 —— 调用方一套、运行时一套 —— 这与让 WAF 绕过和请求走私成立的是同一种分歧。

**而"没有 shell"不等于"安全"。** 如果用户控制的是一个**参数**而不是命令，危险就不再是元字符，而是参数本身：

```
把用户可控的值传给 curl：
  -o /etc/cron.d/payload       把下载内容写到任意路径
  --config /tmp/attacker.rc    加载一个配置文件
  @/etc/shadow                 把某个文件当作请求体读出去

把用户可控的值传给 tar 或 git：
  --to-command=... 、--checkout=... 、-c core.sshCommand=...
```

这就是**参数注入**，而它恰好活在那些"当初刻意避免使用 shell"的代码里。修法也不一样：不是转义，而是**结束选项列表**。

### 元字符，以及空白那一类

只在有 shell 时才有意义，但值得列全 —— 因为"覆盖了大部分"才是过滤器的常态。实测：

| 分隔符 | payload | 是否执行 |
|---|---|---|
| 分号 | `example.com;id` | 是 |
| 管道 | `example.com\|id` | 是 |
| 逻辑或 | `example.com\|\|id` | 是 |
| 逻辑与 | `example.com&&id` | 否 —— 前一条命令失败了 |
| 反引号 | `` example.com`id` `` | 是 |
| 命令替换 | `example.com$(id)` | 是 |
| **换行** | `example.com\nid` | **是** |

换行那一行是过滤器会漏掉的，而原因是一个建模错误而非疏忽：换行是**与分号同级的一种命令分隔符**，而黑名单是按"有人想到的那些分隔符字符"写出来的清单。那一测里 `&&` 不执行则是反方向的启发 —— 它要求前一条命令成功，所以当前一条被故意写坏时，它不是一个可靠的分隔符。

**空白是一个类别，不是一个字符。** 实测，把每个 payload 里的空格都去掉：

| 替代写法 | 可用的 payload |
|---|---|
| `${IFS}` | `cat${IFS}/etc/hostname` |
| `${IFS}${IFS}` 或 `$IFS$9` | `cat$IFS$9/etc/hostname` |
| 输入重定向 | `cat</etc/hostname` |
| 制表符 | `cat\t/etc/hostname` |

`IFS` 是 shell 的**内部字段分隔符**，默认值就是空格、制表符与换行 —— 于是一个"展开成空白"的变量，能击败任何只拦了字面空格字符的过滤器。重定向那种写法是另一条路：`<` 不需要任何空白就能把命令与它的参数分开。

**而哪种写法能用，取决于跑的是哪个 shell。** 靶场那份过滤器把制表符和空格都拦了，却没拦换行，所以 `%0A` 是第一道裂缝；而一份按 bash 思路写的 payload 清单所假设的那些语法糖，在 `dash` 里并不存在 —— Debian 系上 `/bin/sh` 就是它：

| 语法 | bash | dash |
|---|---|---|
| `{cat,/etc/hostname}` | 花括号展开 | **not found** |
| `${IFS:0:1}` | 子串展开 | **Bad substitution** |
| `$'\x20'` | ANSI-C 引号 | **不支持** |

实测：花括号那种写法在靶场的 shell 上以 `not found` 失败。**所以 payload 集合是"目标 shell"的函数，不是"技术"的函数** —— 判断跑的是哪个 shell 属于侦察的一部分，而一个失败的 payload 可能只是"给错了翻译器"，不是过滤器起了作用。

### 检测与缓解

- **对任何最终会到达命令的输入里出现元字符告警。** 主机名、文件名、参数或搜索框里的 `;`、`|`、`&`、反引号、`$(`、`${`、换行、`>`、`<` 都不是偶然。这份清单要包含编码形式（`%0A`、`%3B`）与空白替代写法（`$IFS`），因为过滤器和检测器面对同一个枚举难题，而检测器的优势在于它看得见 payload。
- **盯进程树，那是现有最强的信号。** Web 服务器或应用进程起一个 `sh -c` 是正常的；那个 shell 接着起了 `id`、`whoami`、`curl`、`wget`、`nc`、`bash`，或者对一个应用目录之外的路径执行 `cat`，就不正常。这种检测能扛过一切绕过，因为它看的是**跑起来了什么**，而不是那串字符是怎么写的。
- **盯命令输出里本不该有的内容。** `uid=`、`gid=`、`/etc/passwd` 的内容、应用根目录的列表、或者应用从不读取的某个文件的内容。靶场自己的通关条件就是这一条：输出里必须出现 `uid=`。
- **盯时间与网络这两条旁路，因为命令注入常常是盲的。** 一个请求的耗时与输入里要求的 `sleep` 相符；应用主机向输入里给出的地址发起出站连接；应用毫无理由去解析的一个域名被解析了。当响应体什么都不显示时，正是这些观察抓到注入。
- **缓解上，不要用 shell。** 传参数数组 —— 就是上面那张表里什么都没执行的那一行 —— 移除了整个元字符类，因为没有翻译器再给这些字符赋予含义。
- **确实需要 shell 的地方，用允许清单而不是黑名单。** 靶场那份过滤器就是论据：它拦了六类分隔符，被第七类击败。一份针对可接受值的允许清单（一个主机名模式、一个来自已知集合的文件名、一个数值范围）会**失败关闭**，而那正是过滤器需要的性质。
- **对任何接受参数的东西，结束选项列表。** 在用户可控值前面加一个 `--` 能阻止它被当成选项读，而拒绝以 `-` 开头的值则关上了"没有 shell 也存在的"参数注入那条路。
- **针对"将要解析这串字符的那一层"做转义。** `escapeshellarg` 那一类引用**对 shell** 是对的，而在用参数数组的地方是错的 —— 那里的引号会变成参数的一部分。转义函数与调用方式必须一起选。
- **用这个进程所需的**最小权限**去跑它。** 实测里那个上下文是 root，而这对运维面板来说很常见 —— 正是这一点把一个诊断工具变成了管理工具。一个专用的低权限用户能在不阻止漏洞的前提下限制后果。
