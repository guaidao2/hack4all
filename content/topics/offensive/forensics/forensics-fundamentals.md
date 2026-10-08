---
id: forensics-fundamentals
title_en: Digital Forensics Fundamentals
title_zh: 数字取证基础
summary_en: Forensics answers what happened, in what order, and what can be proven. The techniques matter less than the order of operations — volatile evidence first, and nothing touched before it is captured.
summary_zh: 取证要回答的是"发生了什么、按什么顺序、以及什么能被证明"。技术本身不如操作顺序要紧 —— 先取易失证据，采集之前不碰任何东西。
tags: [forensics, incident-response, timeline, memory, disk, volatility]
tools: [Volatility, Autopsy, Plaso, Wireshark, KAPE, Velociraptor]
attck: [T1070]
platform: [windows, linux]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Two rules that come before any tool

**Order of volatility.** Capture the things that disappear first:

1. CPU registers, cache, running processes and their memory.
2. Network connections, routing tables, ARP cache.
3. Temporary files, swap, running configuration.
4. Disk.
5. Backups, logs shipped elsewhere, cloud snapshots.

A machine that is powered off to "preserve" it has destroyed the most valuable evidence it had. If a memory capture is possible, that comes first.

**Chain of custody.** Every step is recorded: who collected what, when, with which tool and version, and where the hash is. Without it, the finding is an opinion rather than evidence, and the same discipline is what stops an investigator from accidentally changing what they are examining.

```bash
# acquire, then verify, and never work on the original
dc3dd if=/dev/sda of=/evidence/disk.img hash=sha256 log=/evidence/acq.log
sha256sum /evidence/disk.img          # record this, twice, on separate media
```

### What each evidence source answers

| Source | How to capture | What it answers |
|---|---|---|
| Memory | WinPmem, LiME, AVML | Running processes, injected code, network connections, keys, decrypted content |
| Disk image | dc3dd, FTK Imager, `dd` | Files, deleted files, file system metadata, timeline |
| Windows event logs | `.evtx` files, or the SIEM copy | Logons, process creation, service installs, account changes |
| Linux logs | `auth.log`, `syslog`, journald | Authentication, service activity, cron |
| Network | Full packet capture, Zeek logs, NetFlow | C2, exfiltration, lateral movement |
| Cloud | CloudTrail, Activity Log, audit logs | API calls, who did what through the control plane |
| Application | Its own logs and database | Business-level actions, often with more context than the OS has |

### Windows artifacts worth knowing

Windows is generous with evidence once you know where it lives:

- **`$MFT`** — every file, including deleted ones, with timestamps and sizes.
- **`$UsnJrnl`** — a journal of file changes, which survives some deletion.
- **Prefetch** — execution evidence: which programs ran, when, and how many times.
- **Amcache and ShimCache** — program execution and compatibility data.
- **SRUM** — per-application network and resource usage, over time.
- **Registry** — `Run` keys, `UserAssist`, Shellbags (which folders were browsed), USB history, and installed services.
- **Event logs** — the ones that matter most: 4624/4625 (logon success/failure), 4648 (explicit credential use), 4672 (special privileges), 4688 (process creation, with command line if enabled), 4720 (account created), 7045 (service installed).
- **Sysmon**, if deployed, gives process, network, file and registry events with hashes — the single most useful source in a Windows investigation.

### Linux artifacts worth knowing

- `/var/log/auth.log`, `/var/log/secure`, `journalctl` — authentication and service activity.
- `last`, `lastlog`, `wtmp` — logon history, sometimes edited by an intruder.
- Shell history for every user, including root.
- `/etc/cron*`, `systemd` units and timers, `~/.ssh/authorized_keys`.
- Package manager logs, which reveal what was installed when.
- `/proc` for live processes, and file timestamps — remembering that `relatime` means access times are not reliable evidence.

### Timeline analysis is the product

The output of a good investigation is not a list of artifacts; it is a **timeline**. Merge every source with a timestamp into one sequence (Plaso or `log2timeline` does the mechanical part), and then read it:

```bash
log2timeline.py --storage-file case.plaso /evidence/disk.img
psort.py -o l2tcsv case.plaso "date > '2026-09-01' and date < '2026-10-01'" > timeline.csv
```

A timeline turns scattered facts into a narrative — initial access, the tool that ran five minutes later, the account created the next day, the outbound connection the day after. That narrative is what the report is about.

### Anti-forensics, and why it does not fully work

An intruder who knows forensics will try to defeat it: clearing event logs, timestomping files, running entirely in memory, encrypting the exfiltration channel.

All of it leaves traces, and knowing where is the value of studying the techniques:

- **Timestomping** changes `$STANDARD_INFORMATION` timestamps but not `$FILE_NAME` in the `$MFT`, and the mismatch is the evidence.
- **Cleared event logs** produce an event saying the log was cleared (1102 on Windows), plus a gap.
- **Prefetch and Amcache** record execution independently of the file's current timestamps.
- **Memory-only activity** is invisible to disk forensics but not to memory forensics, which is why the order of volatility matters.
- **USN journal** entries exist even when the file they refer to has been deleted.
- **Missing logs at the collector** are themselves a signal, which is the argument for shipping logs off the host.

### Detection

- **Ship logs off the host in real time.** The only reliable defence against log deletion is that the deletion happens on a copy you control.
- **Deploy memory acquisition tooling in advance.** Installing it during an incident takes time you may not have.
- **Keep enough retention.** Thirty days of hot logs covers most investigations; ninety is comfortable.
- **Preserve proactively.** Snapshot before you start remediation, because the first response action often destroys evidence.
- **Practise the response.** An incident is a bad time to discover that nobody knows how to acquire memory, or that the legal team needs a chain of custody form you have never filled in.

### Mitigation

- **Centralise and protect logs** (immutable storage, separate credentials) so anti-forensics has a smaller surface.
- **Turn on the telemetry you will need**: command-line logging in 4688, PowerShell script block logging, Sysmon, and audit policy for object access where it matters.
- **Keep an inventory and a baseline** so "unusual" is computable.
- **Write the playbook** for the evidence types you are most likely to need, and rehearse it before it counts.
- **Do not power off.** Document that instruction, because the instinct in the moment is the opposite.

<!-- lang:zh -->
### 在任何工具之前的两条规则

**易失性顺序。** 先采集会最先消失的东西：

1. CPU 寄存器、缓存、运行中的进程及其内存。
2. 网络连接、路由表、ARP 缓存。
3. 临时文件、交换分区、运行中的配置。
4. 磁盘。
5. 备份、已送到别处的日志、云快照。

一台为了"保全"而被关掉的机器，已经销毁了它拥有的最有价值的证据。**如果内存可以被采集，那就先采内存。**

**证据链。** 每一步都要记录：谁在何时、用哪个工具与版本采集了什么，哈希在哪里。没有它，发现只是意见而不是证据；同一条纪律也正是防止调查者无意间改变被查对象的东西。

```bash
# 采集，然后校验；永远不要在原始介质上操作
dc3dd if=/dev/sda of=/evidence/disk.img hash=sha256 log=/evidence/acq.log
sha256sum /evidence/disk.img          # 记录下来，存两份，放在不同介质上
```

### 每种证据来源回答什么

| 来源 | 怎么采 | 回答什么 |
|---|---|---|
| 内存 | WinPmem、LiME、AVML | 运行中的进程、注入的代码、网络连接、密钥、已解密内容 |
| 磁盘镜像 | dc3dd、FTK Imager、`dd` | 文件、已删除文件、文件系统元数据、时间线 |
| Windows 事件日志 | `.evtx` 文件，或 SIEM 里的副本 | 登录、进程创建、服务安装、账号变更 |
| Linux 日志 | `auth.log`、`syslog`、journald | 认证、服务活动、cron |
| 网络 | 完整抓包、Zeek 日志、NetFlow | C2、数据外泄、横向移动 |
| 云 | CloudTrail、Activity Log、审计日志 | API 调用、谁通过控制面做了什么 |
| 应用 | 自身日志与数据库 | 业务级动作，往往比操作系统有更多上下文 |

### 值得知道的 Windows 证据

Windows 在证据方面相当慷慨，前提是你知道它在哪：

- **`$MFT`** —— 每一个文件，包括已删除的，带时间戳与大小。
- **`$UsnJrnl`** —— 文件变更日志，部分删除后仍会留下。
- **Prefetch** —— 执行证据：哪些程序跑过、什么时候、跑了几次。
- **Amcache 与 ShimCache** —— 程序执行与兼容性数据。
- **SRUM** —— 按应用统计的网络与资源使用，且是随时间记录的。
- **注册表** —— `Run` 键、`UserAssist`、Shellbags（浏览过哪些目录）、USB 历史、已安装服务。
- **事件日志** —— 最要紧的几个：4624/4625（登录成功/失败）、4648（显式使用凭据）、4672（特殊权限）、4688（进程创建，开启后含命令行）、4720（创建账号）、7045（安装服务）。
- **Sysmon**（如果部署了）提供带哈希的进程、网络、文件与注册表事件 —— 这是 Windows 调查里最有用的单一来源。

### 值得知道的 Linux 证据

- `/var/log/auth.log`、`/var/log/secure`、`journalctl` —— 认证与服务活动。
- `last`、`lastlog`、`wtmp` —— 登录历史，有时会被入侵者改过。
- 每个用户的 shell history，包括 root。
- `/etc/cron*`、`systemd` 的 unit 与 timer、`~/.ssh/authorized_keys`。
- 包管理器日志，它会告诉你什么时候装了什么。
- 活着的进程看 `/proc`，文件时间戳要注意 `relatime` —— 访问时间不是可靠证据。

### 时间线才是产物

一次好的调查，产出不是一份证据清单，而是**一条时间线**。把所有带时间戳的来源合并成一个序列（机械部分交给 Plaso 或 `log2timeline`），然后去读：

```bash
log2timeline.py --storage-file case.plaso /evidence/disk.img
psort.py -o l2tcsv case.plaso "date > '2026-09-01' and date < '2026-10-01'" > timeline.csv
```

时间线把零散的事实变成叙述 —— 初始访问、五分钟后运行的工具、第二天创建的账号、再下一天的外连。报告写的就是这条叙述。

### 反取证，以及它为什么不能完全奏效

懂取证的入侵者会试图对抗它：清事件日志、篡改文件时间戳、全程只在内存里活动、加密外泄通道。

这些都会留下痕迹，而知道留在哪，正是研究这些手法的价值：

- **篡改时间戳**改的是 `$MFT` 里的 `$STANDARD_INFORMATION`，改不了 `$FILE_NAME`，两者不一致就是证据。
- **清空事件日志**会自己产生一条"日志已被清空"的事件（Windows 上是 1102），外加一段空白。
- **Prefetch 与 Amcache**独立记录了执行，与文件当前的时间戳无关。
- **只在内存中的活动**对磁盘取证不可见，但对内存取证不是 —— 这就是易失性顺序为什么重要。
- **USN journal** 里即使指向的文件已被删除，条目依然存在。
- **收集端缺失的日志**本身就是信号 —— 这正是"把日志送出主机"的论据。

### 检测

- **实时把日志送出主机。** 对抗日志删除唯一可靠的办法，是删除发生在你控制的副本上。
- **提前部署内存采集工具。** 事件发生时才去装，时间可能不够。
- **保留期要够。** 三十天热日志能覆盖大多数调查，九十天就很宽裕。
- **主动保全。** 在开始处置之前先做快照，因为第一个响应动作往往就在销毁证据。
- **演练响应。** 事故现场不是发现"没人会采内存"或者"法务要一份你从没填过的证据链表格"的好时候。

### 缓解

- **集中并保护日志**（不可变存储、独立凭据），让反取证的攻击面更小。
- **打开你将来需要的遥测**：4688 的命令行记录、PowerShell 脚本块日志、Sysmon，以及关键位置的对象访问审计策略。
- **维护资产台账与基线**，让"异常"可被计算。
- **为最可能需要的那几类证据写好剧本**，并且在真正需要之前演练。
- **不要关机。** 把这句话写进流程 —— 因为事发当时的本能恰好相反。
