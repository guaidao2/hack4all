---
id: linux-command-line
title_en: The Linux Command Line for Security Work
title_zh: Linux 命令行基础
summary_en: Almost every tool in this field is a command, and almost every server you will touch has no graphical interface. This entry builds the command line from the shell upward — files, permissions, pipes, processes, network commands — with the security question attached to each part.
summary_zh: 这个领域里几乎每一个工具都是一条命令，而你接触到的服务器几乎都没有图形界面。这一篇从 shell 开始把命令行搭起来 —— 文件、权限、管道、进程、网络命令 —— 每一部分都附上它对应的安全含义。
tags: [beginner, linux, shell, bash, permissions, command-line]
tools: [bash, grep, find, sed, awk, ss, systemctl, ssh, tcpdump]
attck: [T1059.004, T1548.001]
platform: [linux]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Why the command line first

Three reasons, and all three matter for security work specifically:

1. **Servers do not have a graphical interface.** A Linux server is usually you, a terminal, and SSH.
2. **The tools are commands.** `nmap`, `tcpdump`, `sqlmap`, `hashcat` — the entire ecosystem is command-line programs that take options and print text.
3. **Commands compose.** This is not a stylistic preference. A one-line pipeline can filter a gigabyte of logs down to the five lines that matter, and the same line can be rerun, scripted and put in a report. A GUI cannot.

### Part 1: the shell, and the idea underneath

#### Terminal, shell, command

The words get used interchangeably, but they are three things:

- **The terminal** is the program that draws text and reads your keystrokes.
- **The shell** is the program inside it that interprets what you type. `bash` is the most common; `zsh`, `fish`, `sh` and `dash` are others.
- **A command** is a program the shell runs. `ls` is a program (usually `/bin/ls`).

```bash
which ls          # where is this program?
type cd           # cd is a shell builtin, not a program
echo $SHELL       # which shell am I in?
```

That distinction explains a lot of small confusions: `cd` cannot be an external program, because changing the directory of a child process would not affect the shell.

#### Everything is a file

The design principle of Unix is that most things you interact with are exposed as files:

| Path | What it really is |
|---|---|
| `/etc/passwd` | A real file |
| `/dev/sda` | A block device (a disk) |
| `/dev/null` | A device that discards writes |
| `/proc/1234/cmdline` | A view into a running process |
| `/proc/net/tcp` | The kernel's TCP connection table |

The consequences show up constantly in security work: you can read process information with `cat`, write to a device with `>` , and enumerate connections by reading a file.

### Part 2: the filesystem

#### The layout, and why each part matters

| Directory | Holds | Why you will care |
|---|---|---|
| `/` | The root of everything | — |
| `/bin`, `/usr/bin` | Programs | Where the tools live |
| `/etc` | Configuration | Passwords, services, scheduled jobs, everything misconfigurable |
| `/home/<user>` | User files | Where user data and keys are |
| `/root` | The root user's home | Often the goal |
| `/tmp` | Temporary, world-writable | Where things land, and where they can be executed |
| `/var/log` | Logs | Evidence |
| `/var/www` | Web content | Where a web shell would be written |
| `/opt`, `/usr/local` | Third-party software | Non-package installs |
| `/proc` | Kernel and process info | Live system state |
| `/sys` | Device and kernel parameters | Hardening settings |
| `/dev` | Devices | Disks, terminals, `/dev/null` |
| `/mnt`, `/media` | Mount points | Attached filesystems |

#### Navigation

```bash
pwd                 # where am I
ls                  # what is here
ls -la              # long format, including hidden files
cd /etc             # absolute path
cd ../..            # relative: up two levels
cd ~                # my home directory
cd -                # the previous directory
```

`.` means the current directory, `..` the parent, `~` your home. Paths starting with `/` are absolute; everything else is relative to where you are. This matters when a script behaves differently depending on the working directory.

#### Reading `ls -l` properly

```
-rwxr-xr-x  1 root root  1234 Oct  8 09:20  backup.sh
│└┬┘└┬┘└┬┘  │  │    │     │        │         └ name
│ │  │  └── others: r-x       │        └ modification time
│ │  └───── group:  r-x       └ size in bytes
│ └──────── owner:  rwx
└────────── type: - file, d directory, l symlink
```

Learning to read that line at a glance is worth ten minutes: it tells you who can do what to a file, and "who can write what" is the question at the centre of a great deal of security work.

### Part 3: permissions

#### The model

Every file has an **owner** and a **group**, and three sets of permissions: for the owner, for the group, and for everyone else. Each set has three bits: **r**ead, **w**rite, **e**xecute.

As numbers, `r=4`, `w=2`, `x=1`, added together:

| Octal | Symbolic | Meaning |
|---|---|---|
| 644 | `rw-r--r--` | Owner reads and writes; everyone else reads |
| 600 | `rw-------` | Only the owner can touch it — the right mode for a private key |
| 755 | `rwxr-xr-x` | Owner full control; others read and execute — a normal program |
| 777 | `rwxrwxrwx` | Everyone can do anything — almost always a mistake |

```bash
chmod 600 id_rsa            # numeric
chmod u+x script.sh         # symbolic: add execute for the owner
chown alice:dev file.txt    # change owner and group
umask                       # what new files will not have
```

#### The special bits

Three bits sit outside the normal nine, and each has a security meaning:

| Bit | On a file | On a directory |
|---|---|---|
| **setuid** | Runs as the file's owner, not the caller | (ignored) |
| **setgid** | Runs with the file's group | New files inherit the group |
| **sticky** | (ignored) | Only the owner can delete their files — this is why `/tmp` works |

**setuid is the one to understand.** A program with setuid and owner root runs as root regardless of who executes it. That is how `passwd` can change your password in a root-owned file. It is also why a setuid binary with a flaw is a privilege escalation, and why finding unusual setuid files is a standard check:

```bash
find / -perm -4000 -type f 2>/dev/null      # setuid files
find / -perm -2000 -type f 2>/dev/null      # setgid files
```

#### Users

```bash
whoami                  # who am I
id                      # my uid, gid and groups
groups                  # my groups
sudo -l                 # what am I allowed to run as root
```

`/etc/passwd` holds the account list and is readable by everyone. `/etc/shadow` holds the password hashes and is readable only by root — that split is deliberate, and it is why a readable `/etc/shadow` is serious. `sudo` lets a permitted user run a command as another user, and `/etc/sudoers` decides who and what.

### Part 4: looking at and processing files

#### Reading

```bash
cat file.txt            # dump the whole thing
less file.txt           # page through it (q to quit, / to search)
head -20 file.txt       # the first 20 lines
tail -20 file.txt       # the last 20
tail -f /var/log/syslog # follow a log as it grows — indispensable
wc -l file.txt          # count lines
file mystery.bin        # what is this really
stat file.txt           # timestamps, size, inode, permissions
```

`tail -f` is worth internalising early: while you reproduce something, having the log scrolling beside you turns guesswork into observation.

#### Searching: grep

```bash
grep 'error' app.log                 # lines containing error
grep -i 'failed' app.log             # case-insensitive
grep -rn 'password' /var/www         # recursive, with line numbers
grep -v 'health' access.log          # invert: lines NOT containing it
grep -E 'user(id|name)' config.php   # extended regex
grep -c '404' access.log             # count matches
```

Regular expressions deserve an hour of your time. The subset that covers most day-to-day work is small: `.` any character, `*` zero or more, `+` one or more, `?` optional, `^` start of line, `$` end of line, `[abc]` a character class, `\d` a digit in extended mode, and `|` for alternatives.

#### Finding: find

`find` is how you answer "where is it" questions, including by attributes:

```bash
find / -name 'id_rsa' 2>/dev/null              # by name
find /var/www -type f -newermt '-1 day'        # modified in the last day
find / -perm -4000 -type f 2>/dev/null         # setuid
find /home -type f -size +100M                 # large files
find / -writable -type d 2>/dev/null           # directories I can write to
```

The `2>/dev/null` is not decoration: without it, `find` over `/` floods you with permission errors. Understanding *why* it works is Part 5.

#### Transforming: sed and awk

```bash
sed 's/old/new/g' file.txt          # replace all occurrences
sed -n '10,20p' file.txt            # print lines 10 to 20
awk '{print $1}' access.log         # the first column
awk -F: '{print $1}' /etc/passwd    # first field, colon-separated
awk '$9 == 404 {print $7}' access.log | sort | uniq -c | sort -rn | head
```

That last pipeline is the shape of real analysis: filter, extract a field, count, sort, look at the top. It answers "which URLs are 404-ing most" in one line, and no GUI does it faster.

#### The other small tools

```bash
sort file | uniq -c | sort -rn      # count and rank
cut -d, -f1,3 data.csv              # pick columns
tr 'A-Z' 'a-z' < file               # translate characters
xargs -I{} command {}               # turn input into command arguments
```

### Part 5: pipes and redirection

This is where the command line becomes more than a way to run programs.

Every program has three streams: **stdin** (0), **stdout** (1) and **stderr** (2). Redirection and pipes connect them.

```bash
command > file          # stdout to a file, overwriting
command >> file         # append
command < file          # stdin from a file
command 2> errors.log   # stderr to a file
command > out.log 2>&1  # both to the same file
command &> all.log      # bash shorthand for the same
command | other         # stdout of one becomes stdin of the next
command | tee file      # write to a file and continue to the screen
```

`2>/dev/null` means "send the error stream to the void". `find / -name x 2>/dev/null` therefore shows only results, not the flood of permission errors — which is exactly why it appears in almost every recipe.

**Command substitution** puts a command's output into another command:

```bash
ip=$(hostname -I); echo "my address is $ip"
for h in $(cat hosts.txt); do ping -c1 "$h"; done
```

**Exit codes** are how one command tells another whether it worked: `0` is success, anything else is failure. `$?` holds the last one.

```bash
grep -q 'needle' haystack.txt && echo found || echo missing
```

### Part 6: processes

```bash
ps aux                    # every process, with user and CPU
ps -ef                    # alternative format with parent PIDs
top                       # live view (htop is friendlier)
pgrep -a nginx            # find by name
kill 1234                 # ask PID 1234 to stop (SIGTERM)
kill -9 1234              # force it (SIGKILL) — no cleanup, last resort
kill -HUP 1234            # many daemons reload their config on SIGHUP
```

**The difference between SIGTERM and SIGKILL matters**: SIGTERM says "please exit", and a well-written program cleans up. SIGKILL is the kernel removing it with no chance to react, which is why it is the last resort rather than the first.

**Background jobs:**

```bash
long-task &               # start in the background
jobs                      # what is running in this shell
fg %1                     # bring job 1 to the foreground
nohup long-task &         # survive the terminal closing
```

**Services** on a modern Linux are managed by systemd:

```bash
systemctl status nginx
systemctl start|stop|restart nginx
systemctl enable nginx       # start at boot
journalctl -u nginx -f       # that service's logs, live
```

`/proc` is where the kernel exposes live state. `/proc/<pid>/cmdline` is the command line of a process, `/proc/<pid>/environ` its environment, `/proc/net/tcp` the connection table. Reading these is often faster than running a tool.

### Part 7: the network commands you will use constantly

```bash
ip a                      # my interfaces and addresses
ip r                      # my routing table
ss -tunap                 # TCP/UDP sockets, with the owning process
ss -tlnp                  # listening TCP only
ping -c 3 1.1.1.1         # reachability
traceroute 1.1.1.1        # path (mtr for a live view)
dig example.com           # DNS
curl -v https://example.com     # HTTP with detail
wget https://example.com/f.bin  # download
nc -lvnp 4444             # netcat listening on 4444, verbose, no DNS
nc 10.0.0.5 80            # connect to a port
nc -lvnp 9000 > received.bin    # receive a file
nc 10.0.0.5 9000 < send.bin     # send one
ssh user@host             # remote shell
ssh-keygen -t ed25519     # generate a key pair
scp file user@host:/path  # copy over SSH
tcpdump -i eth0 -nn port 80 -A  # watch HTTP
```

`nc` and `ssh` are worth learning properly because they are load-bearing. Netcat is a TCP connection you can type into, which makes it the simplest way to test whether a port answers, move a file between machines, or stand up a listener. SSH is not only a shell — it can forward ports and tunnel traffic, which is the foundation of almost every pivoting technique.

### Part 8: packages

```bash
sudo apt update && sudo apt install nmap     # Debian/Ubuntu
sudo dnf install nmap                        # Fedora/RHEL
sudo pacman -S nmap                          # Arch
dpkg -l | grep nmap                          # is it installed (Debian)
rpm -qa | grep nmap                          # (RHEL)
```

One habit worth forming early: know where your software comes from. `curl | sh` from an unverified URL is running someone else's code as your user with no review and no record of what changed — which is exactly the trust problem the supply chain entry describes.

### Part 9: editing text

```bash
nano file.txt      # the beginner-friendly editor: Ctrl+O to save, Ctrl+X to exit
vim file.txt      # the one you should be able to survive in
```

The honest minimum for `vim`: `i` to insert, `Esc` to leave insert mode, `:w` to write, `:q` to quit, `:wq` to do both, `:q!` to discard changes, `dd` to delete a line, `/text` to search, `n` for the next match. That is enough to edit a config file on a server, which is the actual requirement.

### Part 10: your first script

A shell script is a text file with commands in it. The pieces you need:

```bash
#!/bin/bash
# ^ the shebang: which interpreter runs this

name="world"                       # variables (no spaces around =)
echo "hello, $name"

for host in 10.0.0.1 10.0.0.2; do  # a loop
  if ping -c1 -W1 "$host" > /dev/null 2>&1; then
    echo "$host is up"
  else
    echo "$host is down"
  fi
done

count=$1                           # the first argument
echo "first argument was: $count"
exit 0                             # 0 means success
```

```bash
chmod +x ping-hosts.sh
./ping-hosts.sh 5
```

Writing scripts is how you stop doing the same manual steps twice. It is also how you make your work reproducible: a script in a report is evidence; a description of what you clicked is a claim.

### Part 11: the security angle

Nothing here was a vulnerability, but the command line is where most security work either happens or is recorded:

- **Your commands are logged.** `~/.bash_history` records what you typed (usually when the shell exits), and `last`, `/var/log/auth.log` and `sudo` logs record who logged in and what they escalated to. Anything you run on a client's machine is evidence, including your typos.
- **Permissions decide what one compromise becomes.** A service running as root with a writable configuration file is a different situation from one running as an unprivileged user with a read-only directory.
- **setuid binaries are a standing risk.** They are deliberately privileged. Every one on a system is a small piece of root exposed to anyone who can run it.
- **`/tmp` is world-writable by design**, which is why it is both convenient and a place where things appear that should not.
- **The shell itself is an interpreter.** Gluing user input into a command line is how command injection happens — the subject of its own entry.

### Detection and mitigation

- **Audit the right things.** Command history is useful but incomplete and easily tampered with; `auditd` rules on sensitive files and `execve` calls are stronger. Use `last`, `lastlog` and `sudo` logs for authentication events, and expect a determined intruder to try to edit them.
- **Enforce least privilege by default.** Services run as dedicated non-root users, application directories owned so the service cannot write code, and `sudo` restricted to specific commands rather than all of them.
- **Harden SSH before anything else.** Keys instead of passwords, `PermitRootLogin no`, and access limited to the accounts that need it. Most exposed Linux servers are attacked this way within hours of being reachable.
- **Find and review the privileged bits.** `find / -perm -4000` and `find / -perm -2000` should be part of a baseline, and any change to that list should be worth a question.
- **Watch the logs that matter.** Authentication failures, `sudo` usage, new services, new cron entries, and writes into web roots or `/etc` are the events that usually bracket an intrusion.
- **Keep a baseline of the filesystem** where you can, or at least of the directories where code lives, so a new file is visible rather than discovered months later.
- **Do not run unverified code**, and do not pipe a download into a shell. It is the same trust decision as any supply chain compromise, just made faster.

<!-- lang:zh -->
### 为什么先学命令行

三个理由，而且三个都和安全工作直接相关：

1. **服务器没有图形界面。** 一台 Linux 服务器通常就是你、一个终端，和 SSH。
2. **工具就是命令。** `nmap`、`tcpdump`、`sqlmap`、`hashcat` —— 整个生态都是接受参数、输出文本的命令行程序。
3. **命令可以组合。** 这不是风格偏好。一条管道能把几个 GB 的日志过滤成关键的那五行，而同一条命令可以被重跑、被写成脚本、被放进报告。图形界面做不到这件事。

### 第一部分：shell，以及底下的那个思想

#### 终端、shell、命令

这三个词常被混用，但它们是三样东西：

- **终端**是那个画文字、读你按键的程序。
- **shell** 是里面解释你输入的程序。`bash` 最常见；还有 `zsh`、`fish`、`sh`、`dash`。
- **命令**是 shell 去运行的程序。`ls` 就是一个程序（通常在 `/bin/ls`）。

```bash
which ls          # 这个程序在哪？
type cd           # cd 是 shell 内建，不是程序
echo $SHELL       # 我用的是哪个 shell
```

这个区分能解释一堆小困惑：`cd` 不可能是外部程序，因为改一个子进程的目录，根本影响不到 shell 自己。

#### 一切皆文件

Unix 的设计原则是：你打交道的大部分东西，都以文件的形式暴露出来：

| 路径 | 它其实是什么 |
|---|---|
| `/etc/passwd` | 一个真实文件 |
| `/dev/sda` | 块设备（一块磁盘） |
| `/dev/null` | 一个把写入丢弃掉的设备 |
| `/proc/1234/cmdline` | 对一个运行中进程的视图 |
| `/proc/net/tcp` | 内核的 TCP 连接表 |

这些后果在安全工作中不断出现：你可以用 `cat` 读进程信息，用 `>` 往设备里写，靠读一个文件来枚举连接。

### 第二部分：文件系统

#### 目录布局，以及每一部分为什么重要

| 目录 | 装什么 | 你为什么会在意 |
|---|---|---|
| `/` | 一切的根 | —— |
| `/bin`、`/usr/bin` | 程序 | 工具放在这里 |
| `/etc` | 配置 | 口令、服务、定时任务，一切可配错的东西 |
| `/home/<用户>` | 用户文件 | 用户数据与密钥所在 |
| `/root` | root 用户的家目录 | 往往就是目标 |
| `/tmp` | 临时、所有人可写 | 东西会落在这里，也能在这里被执行 |
| `/var/log` | 日志 | 证据 |
| `/var/www` | 网站内容 | web shell 会被写到这里 |
| `/opt`、`/usr/local` | 第三方软件 | 非包管理器安装的东西 |
| `/proc` | 内核与进程信息 | 系统的实时状态 |
| `/sys` | 设备与内核参数 | 加固设置 |
| `/dev` | 设备 | 磁盘、终端、`/dev/null` |
| `/mnt`、`/media` | 挂载点 | 挂上的文件系统 |

#### 导航

```bash
pwd                 # 我在哪
ls                  # 这里有什么
ls -la              # 长格式，包含隐藏文件
cd /etc             # 绝对路径
cd ../..            # 相对路径：往上两层
cd ~                # 我的家目录
cd -                # 上一个目录
```

`.` 表示当前目录，`..` 表示父目录，`~` 是你的家目录。以 `/` 开头的是绝对路径，其他都是相对于你当前位置的。当一个脚本的表现取决于工作目录时，这一点就显出来了。

#### 正确读懂 `ls -l`

```
-rwxr-xr-x  1 root root  1234 Oct  8 09:20  backup.sh
│└┬┘└┬┘└┬┘  │  │    │     │        │         └ 文件名
│ │  │  └── 其他用户: r-x        │        └ 修改时间
│ │  └───── 所属组:   r-x        └ 字节数
│ └──────── 属主:     rwx
└────────── 类型: - 文件, d 目录, l 符号链接
```

花十分钟练到一眼能读这一行是值得的：它告诉你**谁能对这个文件做什么** —— 而"谁能写什么"，正是大量安全工作围绕的核心问题。

### 第三部分：权限

#### 模型

每个文件有一个**属主**和一个**所属组**，以及三组权限：属主、组、其他人。每组三个位：**读**、**写**、**执行**。

换成数字，`r=4`、`w=2`、`x=1`，相加：

| 八进制 | 符号 | 含义 |
|---|---|---|
| 644 | `rw-r--r--` | 属主读写，其他人只读 |
| 600 | `rw-------` | 只有属主能碰 —— 私钥就该是这个权限 |
| 755 | `rwxr-xr-x` | 属主全权，其他人可读可执行 —— 普通程序 |
| 777 | `rwxrwxrwx` | 谁都能做任何事 —— 几乎总是错误 |

```bash
chmod 600 id_rsa            # 数字写法
chmod u+x script.sh         # 符号写法：给属主加执行
chown alice:dev file.txt    # 改属主和组
umask                       # 新建文件会被去掉哪些权限
```

#### 特殊位

在常规九个位之外还有三个位，每一个都有安全含义：

| 位 | 在文件上 | 在目录上 |
|---|---|---|
| **setuid** | 以**文件属主**的身份运行，而不是调用者 | （忽略） |
| **setgid** | 以文件的组身份运行 | 新文件继承该组 |
| **sticky** | （忽略） | 只有属主能删自己的文件 —— 这就是 `/tmp` 能正常工作的原因 |

**setuid 是要理解的那个。** 一个 setuid 且属主是 root 的程序，不管谁执行都以 root 身份运行。`passwd` 就是靠它来改那个 root 拥有的口令文件。也正是因此，一个有缺陷的 setuid 程序就是一条提权路径，而"找出异常的 setuid 文件"是一个标准检查项：

```bash
find / -perm -4000 -type f 2>/dev/null      # setuid 文件
find / -perm -2000 -type f 2>/dev/null      # setgid 文件
```

#### 用户

```bash
whoami                  # 我是谁
id                      # 我的 uid、gid 和组
groups                  # 我在哪些组
sudo -l                 # 我被允许以 root 执行什么
```

`/etc/passwd` 保存账号列表，所有人可读；`/etc/shadow` 保存口令哈希，只有 root 可读 —— 这个拆分是刻意的，也是"可读的 `/etc/shadow` 很严重"的原因。`sudo` 让被许可的用户以另一个用户身份运行命令，而 `/etc/sudoers` 决定谁、能做什么。

### 第四部分：查看与处理文件

#### 读

```bash
cat file.txt            # 整个倒出来
less file.txt           # 分页看（q 退出，/ 搜索）
head -20 file.txt       # 前 20 行
tail -20 file.txt       # 后 20 行
tail -f /var/log/syslog # 跟着日志跑 —— 不可或缺
wc -l file.txt          # 数行数
file mystery.bin        # 这到底是什么
stat file.txt           # 时间戳、大小、inode、权限
```

`tail -f` 值得早点养成习惯：你复现某个现象时，让日志在旁边滚动，能把"猜"变成"看"。

#### 搜索：grep

```bash
grep 'error' app.log                 # 含 error 的行
grep -i 'failed' app.log             # 忽略大小写
grep -rn 'password' /var/www         # 递归，带行号
grep -v 'health' access.log          # 反向：不含它的行
grep -E 'user(id|name)' config.php   # 扩展正则
grep -c '404' access.log             # 统计匹配数
```

正则值得你投入一小时。覆盖日常九成工作的子集很小：`.` 任意字符、`*` 零或多次、`+` 一次或多次、`?` 可选、`^` 行首、`$` 行尾、`[abc]` 字符类、扩展模式下的 `\d` 数字，以及 `|` 表示"或"。

#### 查找：find

`find` 是你回答"东西在哪"这类问题的方式，还能按属性找：

```bash
find / -name 'id_rsa' 2>/dev/null              # 按名字
find /var/www -type f -newermt '-1 day'        # 最近一天改过的
find / -perm -4000 -type f 2>/dev/null         # setuid
find /home -type f -size +100M                 # 大文件
find / -writable -type d 2>/dev/null           # 我能写的目录
```

那个 `2>/dev/null` 不是装饰：不加它，在 `/` 上跑 `find` 会被权限错误淹没。而理解它**为什么**有效，就是第五部分的内容。

#### 变换：sed 与 awk

```bash
sed 's/old/new/g' file.txt          # 替换所有出现
sed -n '10,20p' file.txt            # 打印第 10 到 20 行
awk '{print $1}' access.log         # 第一列
awk -F: '{print $1}' /etc/passwd    # 以冒号分隔的第一段
awk '$9 == 404 {print $7}' access.log | sort | uniq -c | sort -rn | head
```

最后那条管道就是真实分析的形状：过滤、取字段、计数、排序、看头部。它一行回答了"哪些 URL 404 最多"，而没有任何图形工具比它更快。

#### 其他小工具

```bash
sort file | uniq -c | sort -rn      # 计数并排名
cut -d, -f1,3 data.csv              # 挑列
tr 'A-Z' 'a-z' < file               # 字符转换
xargs -I{} command {}               # 把输入变成命令参数
```

### 第五部分：管道与重定向

命令行在这里才变得不只是"一种运行程序的方式"。

每个程序都有三条流：**stdin**（0）、**stdout**（1）、**stderr**（2）。重定向和管道就是连接它们。

```bash
command > file          # stdout 写入文件，覆盖
command >> file         # 追加
command < file          # 从文件读 stdin
command 2> errors.log   # stderr 写入文件
command > out.log 2>&1  # 两者写到同一文件
command &> all.log      # bash 里同样的简写
command | other         # 前一个的 stdout 变成后一个的 stdin
command | tee file      # 同时写文件并继续输出到屏幕
```

`2>/dev/null` 的意思是"把错误流丢进虚空"。所以 `find / -name x 2>/dev/null` 只显示结果，不显示那一大片权限错误 —— 这也正是它出现在几乎每份配方里的原因。

**命令替换**把一个命令的输出放进另一个命令：

```bash
ip=$(hostname -I); echo "我的地址是 $ip"
for h in $(cat hosts.txt); do ping -c1 "$h"; done
```

**退出码**是一个命令告诉另一个命令"成没成"的方式：`0` 是成功，其他都是失败。`$?` 保存着上一个。

```bash
grep -q 'needle' haystack.txt && echo found || echo missing
```

### 第六部分：进程

```bash
ps aux                    # 所有进程，带用户和 CPU
ps -ef                    # 另一种格式，带父进程 PID
top                       # 实时视图（htop 更友好）
pgrep -a nginx            # 按名字找
kill 1234                 # 请 PID 1234 结束（SIGTERM）
kill -9 1234              # 强杀（SIGKILL）—— 不做清理，是最后手段
kill -HUP 1234            # 很多守护进程收到 SIGHUP 会重载配置
```

**SIGTERM 和 SIGKILL 的区别很重要**：SIGTERM 是"请退出"，写得好的程序会清理现场。SIGKILL 是内核直接把它拿掉，程序没有任何反应机会 —— 所以它是最后手段，不是第一手段。

**后台任务：**

```bash
long-task &               # 放到后台
jobs                      # 这个 shell 里有什么在跑
fg %1                     # 把 1 号任务调回前台
nohup long-task &         # 终端关掉也不受影响
```

**服务**在现代 Linux 上由 systemd 管理：

```bash
systemctl status nginx
systemctl start|stop|restart nginx
systemctl enable nginx       # 开机自启
journalctl -u nginx -f       # 看该服务的日志，实时
```

`/proc` 是内核暴露实时状态的地方。`/proc/<pid>/cmdline` 是某个进程的命令行，`/proc/<pid>/environ` 是它的环境变量，`/proc/net/tcp` 是连接表。读这些文件往往比跑一个工具更快。

### 第七部分：你会不断用到的网络命令

```bash
ip a                      # 我的网卡与地址
ip r                      # 我的路由表
ss -tunap                 # TCP/UDP 套接字，带所属进程
ss -tlnp                  # 只看监听的 TCP
ping -c 3 1.1.1.1         # 通不通
traceroute 1.1.1.1        # 路径（mtr 能看实时）
dig example.com           # DNS
curl -v https://example.com     # 带细节看 HTTP
wget https://example.com/f.bin  # 下载
nc -lvnp 4444             # netcat 监听 4444，带详细输出、不做解析
nc 10.0.0.5 80            # 连一个端口
nc -lvnp 9000 > received.bin    # 接收文件
nc 10.0.0.5 9000 < send.bin     # 发送文件
ssh user@host             # 远程 shell
ssh-keygen -t ed25519     # 生成密钥对
scp file user@host:/path  # 通过 SSH 传文件
tcpdump -i eth0 -nn port 80 -A  # 看 HTTP
```

`nc` 和 `ssh` 值得认真学，因为它们是承重的。Netcat 就是一条你可以直接往里打字的 TCP 连接，所以它测试端口通不通、在两台机器之间搬文件、起一个监听器，都是最简的方式。SSH 不只是 shell —— 它能转发端口、隧道流量，而这是几乎所有内网穿透手法的基础。

### 第八部分：软件包

```bash
sudo apt update && sudo apt install nmap     # Debian/Ubuntu
sudo dnf install nmap                        # Fedora/RHEL
sudo pacman -S nmap                          # Arch
dpkg -l | grep nmap                          # 装了吗（Debian）
rpm -qa | grep nmap                          # （RHEL）
```

有个习惯值得早点养成：**知道你的软件从哪来**。从一个没验证过的网址 `curl | sh`，等于以你的用户身份运行别人的代码，既没有审查，也没有留下了什么的记录 —— 那正是供应链那一篇讲的信任问题。

### 第九部分：编辑文本

```bash
nano file.txt      # 对新手友好的编辑器：Ctrl+O 保存，Ctrl+X 退出
vim file.txt      # 你至少该能在里面活下来的那个
```

`vim` 诚实的最低要求：`i` 进入插入模式、`Esc` 退出插入模式、`:w` 保存、`:q` 退出、`:wq` 保存并退出、`:q!` 放弃修改、`dd` 删一行、`/文字` 搜索、`n` 下一个匹配。这些足够你在服务器上改一个配置文件 —— 而这就是真正的需求。

### 第十部分：你的第一个脚本

shell 脚本就是装着命令的文本文件。你需要的那几块：

```bash
#!/bin/bash
# ^ shebang：由哪个解释器来运行这个文件

name="world"                       # 变量（等号两边不能有空格）
echo "hello, $name"

for host in 10.0.0.1 10.0.0.2; do  # 循环
  if ping -c1 -W1 "$host" > /dev/null 2>&1; then
    echo "$host is up"
  else
    echo "$host is down"
  fi
done

count=$1                           # 第一个参数
echo "first argument was: $count"
exit 0                             # 0 表示成功
```

```bash
chmod +x ping-hosts.sh
./ping-hosts.sh 5
```

写脚本，是你不再手工重复同一步骤的方式。它也是让你的工作可复现的方式：**报告里的脚本是证据，"我点了哪些按钮"的描述只是说法。**

### 第十一部分：安全视角

上面没有一处是漏洞，但命令行正是大多数安全工作发生、或被记录下来**的地方**：

- **你的命令会被记录。** `~/.bash_history` 记下你打过的命令（通常是在 shell 退出时），而 `last`、`/var/log/auth.log` 和 `sudo` 日志记下谁登录了、提权做了什么。你在客户机器上跑过的一切都是证据，包括你的手误。
- **权限决定了"一次失陷"会变成什么。** 一个以 root 运行、配置文件还可写的服务，和一个以非特权用户运行、目录只读的服务，处境完全不同。
- **setuid 程序是长期存在的风险。** 它们是被刻意赋予特权的。系统上每一个 setuid 程序，都是一小片暴露给"任何能运行它的人"的 root。
- **`/tmp` 按设计就是所有人可写**，所以它既方便，也是不该出现的东西会出现的地方。
- **shell 本身是个解释器。** 把用户输入粘进命令行，就是命令注入的成因 —— 那另有专篇。

### 检测与缓解

- **审计该审计的东西。** 命令历史有用但不完整、且易于篡改；对敏感文件和 `execve` 调用加 `auditd` 规则要强得多。认证事件用 `last`、`lastlog` 和 `sudo` 日志，同时要预期一个铁了心的入侵者会试图改它们。
- **默认强制最小权限。** 服务用专用的非 root 用户运行，应用目录的属主设置成"服务写不了代码"，`sudo` 限制到具体命令而不是全部放开。
- **先加固 SSH，再谈别的。** 用密钥而不是口令、`PermitRootLogin no`、只让需要的账号能登。暴露在公网的 Linux 服务器，通常在可达后几小时内就会被这样攻击。
- **找出并复查那些特权位。** `find / -perm -4000` 和 `find / -perm -2000` 应该成为基线的一部分，而这个清单上的任何变化都值得问一句。
- **盯住要紧的日志。** 认证失败、`sudo` 使用、新服务、新的 cron 条目，以及往 web 根目录或 `/etc` 的写入 —— 这些通常是入侵前后的路标。
- **能做的话，给文件系统建基线**，至少给放代码的目录建，这样新文件是"看得见"，而不是几个月后才发现。
- **不要运行未经验证的代码**，也不要把下载内容直接管进 shell。这跟任何供应链失陷是同一个信任决策，只是做得更快。
