---
id: windows-and-active-directory-basics
title_en: Windows and Active Directory Basics
title_zh: Windows 与域基础
summary_en: Most corporate environments are Windows, and most of them are joined to an Active Directory domain, which makes the identity model the first thing worth understanding. This entry covers SIDs and access tokens, NTLM and Kerberos, the registry, what a domain actually is, the commands you will use daily, and the event log you leave behind.
summary_zh: 多数企业环境是 Windows，而其中多数又加入了 Active Directory 域 —— 所以最该先弄懂的是它的身份模型。这一篇讲 SID 与访问令牌、NTLM 与 Kerberos、注册表、域到底是什么、每天会用到的命令，以及你会留下痕迹的事件日志。
tags: [beginner, windows, active-directory, kerberos, ntlm, powershell, event-log]
tools: [whoami, net, dsquery, nltest, PowerShell, Sysmon, Get-WinEvent]
attck: [T1078.002, T1550.002, T1550.003, T1558.003]
platform: [windows]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Why Windows before anything else on the red side

Two facts shape almost every corporate engagement:

1. **The users are on Windows.** Workstations, file shares, email, the applications people actually use.
2. **The identity lives in Active Directory.** One directory decides who can do what, across thousands of machines.

So the useful thing to understand first is not a tool, it is the model: how Windows decides that you are who you say you are, and what that decision lets you do.

### Part 1: identity — SIDs and tokens

#### A SID, not a name

Every account and group on a Windows system has a **Security Identifier**. Names are for humans; access control decisions are made with SIDs. Changing a username does not change its SID, which is why an account can be renamed and keep every permission it had.

```
S-1-5-21-1004336348-1177238915-682003330-1105
│ │ │  └──────────── domain identifier ────────┘ └┬┘
│ │ │                                            RID
│ │ └── authority (5 = NT)
│ └──── version
└────── S
```

The last part is the **RID** (relative identifier), and a few are fixed and worth memorising:

| SID | Who |
|---|---|
| `S-1-5-18` | Local SYSTEM — the machine's own account, higher privilege than an administrator |
| `S-1-5-19` / `S-1-5-20` | Local Service / Network Service |
| `S-1-5-32-544` | The local Administrators group |
| `S-1-5-21-<domain>-500` | The domain's built-in Administrator (RID 500) |
| `S-1-5-21-<domain>-502` | The built-in Guest |
| `S-1-5-21-<domain>-512` | Domain Admins |
| `S-1-5-21-<domain>-515` | Domain Computers |
| `S-1-5-21-<domain>-516` | Domain Controllers |
| `S-1-5-21-<domain>-1000+` | Ordinary accounts, created in order |

RID 500 is worth remembering for a practical reason: it exists on every domain, it cannot be locked out by account lockout policy in the default configuration, and it is a favourite target.

#### The access token

When you log in, the system builds an **access token** containing your SID, the SIDs of every group you are in, and your privileges. Every action you take is checked against that token.

```cmd
whoami /all
```

That command is the single most useful orientation command on a Windows host: it prints your user, your SIDs, your group memberships, and your privileges. The privileges are worth recognising:

| Privilege | What it allows |
|---|---|
| `SeDebugPrivilege` | Read and write memory of other processes — including LSASS |
| `SeImpersonatePrivilege` | Impersonate another user — service accounts usually have it, and it is the basis of several escalations |
| `SeBackupPrivilege` | Read any file regardless of ACLs — used to read registry hives |
| `SeRestorePrivilege` | Write any file |
| `SeTakeOwnershipPrivilege` | Take ownership of anything |
| `SeLoadDriverPrivilege` | Load a kernel driver |
| `SeTcbPrivilege` | Act as part of the operating system |

#### Integrity levels and UAC

Windows also tags processes with an **integrity level** — Medium for a normal process, High for an elevated one. User Account Control is the prompt you see when something wants to elevate.

**UAC is not a security boundary**, and this matters. It exists to prevent *accidental* system changes. An administrator who clicks "Yes" gets a High-integrity token; an administrator who is on the Administrators list already has the power and only lacks the token, which is why many escalations consist of obtaining a High-integrity token rather than becoming a new user.

One practical consequence: since UAC's remote restrictions, a local administrator connecting **over the network** gets a filtered token, so the same account can behave differently locally and remotely. That is why "I am local admin but cannot do X over SMB" is a common surprise.

### Part 2: authentication

Windows has two authentication protocols in play, and knowing which is which explains a great deal of attack and defence.

#### NTLM

NTLM is a **challenge-response** protocol:

1. The client says "I am alice."
2. The server sends a random challenge.
3. The client encrypts the challenge with a key derived from alice's password hash and sends the result.
4. The server (or the domain controller) checks it.

The crucial detail: **the password itself never crosses the network, but the hash is what proves identity.** The stored form is the **NT hash** — MD4 of the password encoded as UTF-16LE, unsalted.

That single design fact explains a whole family of attacks: if the hash is what authenticates you, then possessing the hash is as good as possessing the password. No cracking required.

NTLM is still everywhere for compatibility, but it is legacy: it does not support modern protections well, and it is why "disable or restrict NTLM" appears in every hardening guide.

#### Kerberos

In a domain, the default is Kerberos. It uses a ticket system with three parties: the client, the **Key Distribution Center** (which runs on domain controllers), and the service you want.

The exchange, simplified:

1. **AS-REQ / AS-REP** — the client authenticates to the KDC and receives a **TGT** (Ticket Granting Ticket). The TGT is encrypted with the hash of the `krbtgt` account, so only the KDC can read it.
2. **TGS-REQ / TGS-REP** — to reach a service, the client presents its TGT and asks for a **service ticket** for that service.
3. **AP-REQ** — the client presents the service ticket to the service, which can decrypt it with its own key and thereby trust the KDC's word.

A service is identified by an **SPN** (Service Principal Name), like `MSSQLSvc/db01.corp.local:1433`.

Three consequences worth internalising, because they are the seed of many techniques:

- **Tickets are credentials.** A TGT or service ticket in memory can be presented by anyone who holds it.
- **Service tickets are encrypted with the service account's password hash.** Anyone who can request a ticket for a service can take it away and try to crack it offline.
- **Password changes matter enormously.** `krbtgt`'s password is what protects every TGT; there is a reason "reset it twice" is standard advice after a domain compromise.

#### Where credentials live on a host

| Location | Holds |
|---|---|
| **LSASS** process memory | Cleartext passwords (sometimes), NT hashes, Kerberos tickets |
| **SAM** registry hive | Local account hashes |
| **NTDS.dit** (on a DC) | Every domain account hash |
| **DPAPI** | Secrets encrypted per user or per machine — browser passwords, saved credentials |
| **Credential Manager** | Saved network and RDP credentials |

This list is why LSASS is the most attacked process on Windows, and why protecting it (Credential Guard, LSA protection, limiting who can debug processes) is a priority for defenders.

### Part 3: the registry

The registry is a hierarchical database of configuration — for the OS, for applications, and for user settings.

```
HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run
└──────┬───────┘└──────────────┬──────────────────────────────┘
     hive                    key                        value
```

The hives you will meet:

| Hive | Contents |
|---|---|
| `HKLM` | Machine-wide settings — services, drivers, installed software |
| `HKCU` | The current user's settings |
| `HKU` | All loaded user profiles |
| `HKCR` | File associations and COM registration |

Keys that matter for security, in both directions:

| Key | Why |
|---|---|
| `...\CurrentVersion\Run` / `RunOnce` | Programs that start automatically — a classic persistence location |
| `HKLM\SYSTEM\CurrentControlSet\Services` | Service definitions |
| `HKLM\SAM`, `HKLM\SECURITY` | Local hashes and LSA secrets (not readable while the system runs) |
| `...\Image File Execution Options` | Debugger redirection — a quiet way to hijack execution |
| `...\Winlogon` | Shell, userinit — another persistence location |
| `...\Policies` | Settings pushed by group policy |

```cmd
reg query "HKLM\Software\Microsoft\Windows\CurrentVersion\Run"
reg query HKLM\SYSTEM\CurrentControlSet\Services /s | findstr /i imagepath
reg add "HKCU\Software\Example" /v Setting /t REG_SZ /d value /f
```

### Part 4: the file system and execution

**NTFS** differs from what Linux users expect in two ways worth knowing:

- **ACLs** are explicit lists of who may do what, with inheritance and deny entries. They are more expressive than `rwx`, and correspondingly easier to get wrong.
- **Alternate Data Streams (ADS)** let a file carry more than one data stream — `file.txt:hidden.txt` is a second stream inside the same file. Most copy operations drop it and most tools do not display it.

Directories worth knowing:

| Path | Contents |
|---|---|
| `C:\Windows\System32` | Core binaries and DLLs |
| `C:\Windows\Temp`, `C:\Temp` | Writable by many users |
| `C:\ProgramData` | Machine-wide application data, often world-writable |
| `C:\Users\<user>\AppData\Roaming` | Per-user application data, follows the user between machines |
| `C:\Users\<user>\AppData\Local` | Per-user data that stays on the machine |

**PowerShell** is central to administration and therefore to both sides of security. It is a full programming environment with access to .NET, which is why it is used for automation and also for attacks. It leaves logs if you enable them, which is the defender's counterweight.

**LOLBins** ("living off the land binaries") are signed Microsoft programs that can be abused to do something other than their purpose — `certutil` downloading a file, `rundll32` executing a DLL, `mshta` running script, `regsvr32` fetching and running. They matter because they are legitimate software: a block-list based on filenames will not catch them.

### Part 5: Active Directory

#### What a domain is

A **domain** is a centralised identity and administration boundary. Instead of every machine keeping its own accounts, one directory holds the accounts and every joined machine trusts it.

- **Domain Controller (DC)** — a server running AD services. It holds the database (`NTDS.dit`), answers authentication, and serves LDAP queries.
- **The directory** — a hierarchy of objects: users, computers, groups, organisational units (OUs), and group policy objects.
- **LDAP** is how you read and write it. Objects have a **distinguished name**:

```
CN=Alice Smith,OU=Staff,DC=corp,DC=local
│             │        │
│             │        └ domain components
│             └ organisational unit (a folder, and a place policy is applied)
└ common name
```

- **Group Policy (GPO)** — settings and scripts pushed from the domain to machines and users. A GPO can install software, map drives, set registry values, run a logon script, and change local administrators. It is the domain's configuration management, and therefore an attractive target.

#### Why joining a domain matters so much

Two structural facts:

1. **Domain administrators can administer joined machines.** By design, Domain Admins are local administrators on every domain-joined computer.
2. **Machines trust the domain's authentication.** A service on a joined machine accepts Kerberos tickets issued by the DC.

Together they mean the domain is the prize: compromising one workstation is a foothold, compromising the directory is everything.

#### Trusts

Domains can trust each other, so an identity from one is accepted by another. Trusts are usually **transitive** within a forest. They are also why "our domain is separate" is frequently untrue in practice, and why the direction and transitivity of trust is one of the first things to map in an engagement.

#### The pieces you will hear named

| Term | What it is |
|---|---|
| **OU** | A container for objects; the unit policy is applied to |
| **GPO** | A set of settings linked to a site, domain or OU |
| **SPN** | The identifier of a service instance, used by Kerberos |
| **ACL / ACE** | The permissions on a directory object, and each entry in them |
| **Delegation** | Letting a service act on behalf of a user |
| **SYSVOL** | A share on every DC holding policies and scripts, readable by all domain users |
| **Global Catalog** | A partial replica of the whole forest for fast lookups |
| **FSMO roles** | Five single-master roles, including the one that issues RIDs |

### Part 6: the commands you will actually type

```cmd
:: identity — the first command on any host
whoami /all
whoami /priv
net user %USERNAME% /domain
net group "Domain Admins" /domain
net localgroup Administrators

:: the machine and its network
systeminfo
hostname
ipconfig /all
route print
arp -a
nslookup corp.local
nltest /dclist:corp.local
net view /domain

:: processes, services, tasks
tasklist /v
tasklist /svc
sc query
sc qc <service>
schtasks /query /fo LIST /v

:: files and registry
dir /a
type file.txt
findstr /si password *.txt *.xml *.config
where python
reg query HKLM\Software\Microsoft\Windows\CurrentVersion\Run

:: domain queries (with RSAT installed)
dsquery user -limit 0
dsquery computer
dsquery group -name "Domain Admins"
```

PowerShell equivalents, because that is what modern environments use:

```powershell
Get-Process | Sort-Object CPU -Descending | Select-Object -First 10
Get-Service | Where-Object Status -eq 'Running'
Get-LocalUser
Get-LocalGroupMember -Group Administrators
Get-ADUser -Filter * -Properties * | Select-Object Name, LastLogonDate
Get-ADGroupMember -Identity "Domain Admins"
Get-ADComputer -Filter * | Select-Object Name, OperatingSystem
Get-CimInstance Win32_Service | Select-Object Name, StartMode, PathName
```

### Part 7: the event log

This is the defender's half, and an attacker should know it because it is what they leave behind.

| Log | Contains |
|---|---|
| Security | Logons, privilege use, account changes, object access |
| System | Services, drivers, hardware |
| Application | Application events |
| PowerShell | Script execution, if enabled |
| Sysmon (if deployed) | Process creation with hashes, network connections, file and registry events |

The event IDs worth knowing cold:

| ID | Meaning | Why it matters |
|---|---|---|
| 4624 | Successful logon | Logon type tells you how (2 interactive, 3 network, 10 RDP) |
| 4625 | Failed logon | The signal for password guessing |
| 4648 | Logon with explicit credentials | `runas`, and anything using a different account |
| 4672 | Special privileges assigned | Fires for administrators at logon — the signal for "this is a privileged session" |
| 4688 | Process created | The process tree, if command-line auditing is on |
| 4720 / 4726 | Account created / deleted | New accounts are worth a question |
| 4728 / 4732 / 4756 | Added to a group | Membership changes are how privilege is granted |
| 4768 / 4769 / 4771 | Kerberos TGT / service ticket / pre-auth failure | The audit trail of Kerberos activity |
| 7045 | Service installed | A classic persistence mechanism |
| 1102 | Audit log cleared | Someone tidied up. This is an incident |

```powershell
Get-WinEvent -LogName Security -MaxEvents 20
Get-WinEvent -FilterHashtable @{LogName='Security'; Id=4624} -MaxEvents 20
wevtutil qe Security /c:20 /rd:true /f:text
```

### Part 8: the security map, at concept level

This entry is foundations. Each item here has its own entry later; what matters now is that you can place them:

| Concept | The idea | Why it exists as an attack |
|---|---|---|
| Local admin | Control of one machine | Usually the first step up, and often reused across machines |
| Credential dumping | Reading hashes and tickets from memory | Because possession of the hash authenticates you |
| Pass-the-hash / pass-the-ticket | Using a stolen hash or ticket | NTLM and Kerberos cannot tell a stolen credential from the owner's |
| Kerberoasting | Requesting a service ticket and cracking it offline | Service tickets are encrypted with the service account's hash |
| ACL abuse | A permission that grants control over an object | The directory is an access-control system |
| Delegation abuse | A service acting for a user | Convenient by design, dangerous when misconfigured |
| ADCS | Certificate services in the domain | A certificate can be a credential |
| GPO abuse | Policy is pushed to machines | Policy can run code and change local admins |
| DCSync | Replicating directory data | The replication protocol is the fastest way to every hash |

### Detection and mitigation

- **Tier the administration model.** Tier 0 accounts only on Tier 0 systems, separate accounts for workstation administration, no browsing email from a domain admin session. This is the only structural fix that defeats most of the table above.
- **Protect the credentials.** Credential Guard and LSA protection where the platform supports them, `LAPS` for local administrator passwords so one compromise does not open every machine, and no passwords in scripts or GPO preferences.
- **Reduce the legacy protocol surface.** Restrict or disable NTLM where you can, require SMB signing, and remove unconstrained delegation.
- **Turn on the logs you will need.** Command-line auditing in 4688, PowerShell script block logging, and Sysmon if you can deploy it. Without these, most of the events above simply do not exist.
- **Alert on shapes, not on single events.** A service account logging in interactively, a burst of 4769 tickets for many SPNs from one account, replication requests from a host that is not a DC, a new Domain Admin — these are sequences, and each is worth a rule.
- **Treat the event log as evidence, and protect it.** Ship it off the host in real time; a log that only exists locally is a log an intruder can delete.
- **Assume the domain is the target** and monitor the directory itself, not just the endpoints. Most of what matters in a domain compromise is visible in the domain's own logs.

<!-- lang:zh -->
### 为什么红队方向要先学 Windows

有两个事实塑造了几乎每一个企业项目：

1. **用户都在 Windows 上。** 工作站、文件共享、邮件、人们真正在用的应用。
2. **身份活在 Active Directory 里。** 一个目录决定谁能做什么，横跨成千上万台机器。

所以最该先弄懂的不是某个工具，而是**模型**：Windows 怎么判定"你确实是你说的那个人"，以及这个判定让你能做什么。

### 第一部分：身份 —— SID 与令牌

#### 是 SID，不是名字

Windows 上每一个账户和组都有一个**安全标识符（SID）**。名字是给人看的；访问控制决策是用 SID 做的。改用户名不会改它的 SID —— 这就是一个账户被改名之后，仍然保留着它原来所有权限的原因。

```
S-1-5-21-1004336348-1177238915-682003330-1105
│ │ │  └──────────── 域标识符 ────────────────┘ └┬┘
│ │ │                                          RID
│ │ └── authority（5 = NT）
│ └──── 版本
└────── S
```

最后一段是 **RID**（相对标识符），有几个是固定的，值得记：

| SID | 是谁 |
|---|---|
| `S-1-5-18` | 本地 SYSTEM —— 机器自己的账户，权限比管理员还高 |
| `S-1-5-19` / `S-1-5-20` | Local Service / Network Service |
| `S-1-5-32-544` | 本地 Administrators 组 |
| `S-1-5-21-<域>-500` | 域内置 Administrator（RID 500） |
| `S-1-5-21-<域>-502` | 内置 Guest |
| `S-1-5-21-<域>-512` | Domain Admins |
| `S-1-5-21-<域>-515` | Domain Computers |
| `S-1-5-21-<域>-516` | Domain Controllers |
| `S-1-5-21-<域>-1000+` | 按创建顺序排列的普通账号 |

RID 500 值得记住，有个很实际的理由：每个域都有它，默认配置下账户锁定策略锁不住它，而它是攻击者的心头好。

#### 访问令牌

你登录时，系统会构建一个**访问令牌**，里面装着你的 SID、你所在每个组的 SID，以及你的特权。你做的每一个动作，都拿这个令牌去校验。

```cmd
whoami /all
```

这是在 Windows 主机上最有用的定位命令：它打印出你的用户、SID、组成员关系，以及特权。这些特权值得认得：

| 特权 | 它允许什么 |
|---|---|
| `SeDebugPrivilege` | 读写其他进程的内存 —— 包括 LSASS |
| `SeImpersonatePrivilege` | 冒充其他用户 —— 服务账号通常都有，它是好几种提权的基础 |
| `SeBackupPrivilege` | 无视 ACL 读取任何文件 —— 用来读注册表 hive |
| `SeRestorePrivilege` | 写入任何文件 |
| `SeTakeOwnershipPrivilege` | 取得任何东西的所有权 |
| `SeLoadDriverPrivilege` | 加载内核驱动 |
| `SeTcbPrivilege` | 作为操作系统的一部分行事 |

#### 完整性级别与 UAC

Windows 还会给进程打上**完整性级别** —— 普通进程是 Medium，提权后是 High。用户账户控制（UAC）就是某个东西想提权时你看到的那个提示框。

**UAC 不是安全边界**，这一点很重要。它的存在是为了防止**误操作**改坏系统。点了"是"的管理员会拿到 High 完整性令牌；而一个名字已经在 Administrators 列表里的管理员，本来就握有权力的来源，只差一个令牌 —— 这就是为什么很多提权其实是"拿到一个 High 完整性令牌"，而不是"变成一个全新的用户"。

一个实际后果：自从 UAC 引入远程限制之后，一个本地管理员**通过网络**连接时拿到的是过滤过的令牌，所以同一个账号在本地和远程的表现可能不同。这也是"我是本地管理员，但通过 SMB 就是做不了某件事"这种常见困惑的来源。

### 第二部分：认证

Windows 上有两套认证协议在同时使用，分清它们能解释大量攻防现象。

#### NTLM

NTLM 是一个**挑战-响应**协议：

1. 客户端说"我是 alice"。
2. 服务端发来一个随机挑战。
3. 客户端用从 alice 口令哈希派生出的密钥加密这个挑战，把结果发回去。
4. 服务端（或域控）校验它。

关键的细节是：**口令本身从不在网络上传输，但哈希才是证明身份的东西。** 存储形式是 **NT 哈希** —— 口令按 UTF-16LE 编码后取 MD4，不加盐。

这一个设计事实解释了整整一类攻击：如果哈希就是用来认证你的东西，那么拥有哈希就等于拥有口令，**根本不需要破解**。

NTLM 出于兼容性仍然到处都是，但它是遗留协议：对现代防护支持不好，这也是"禁用或限制 NTLM"出现在每一份加固指南里的原因。

#### Kerberos

在域里，默认是 Kerberos。它用票据体系，涉及三方：客户端、**密钥分发中心**（KDC，跑在域控上）、以及你想访问的服务。

简化后的交换过程：

1. **AS-REQ / AS-REP** —— 客户端向 KDC 认证，拿到一张 **TGT**（票据授予票据）。TGT 用 `krbtgt` 账户的哈希加密，所以只有 KDC 能读它。
2. **TGS-REQ / TGS-REP** —— 要访问某个服务时，客户端出示 TGT，申请该服务的**服务票据**。
3. **AP-REQ** —— 客户端把服务票据交给服务，服务用自己的密钥解密它，从而信任 KDC 的背书。

一个服务由 **SPN**（服务主体名称）标识，形如 `MSSQLSvc/db01.corp.local:1433`。

三个该吃透的后果，因为它们是很多手法的种子：

- **票据就是凭据。** 内存里的一张 TGT 或服务票据，谁拿到谁就能用。
- **服务票据是用服务账号的口令哈希加密的。** 任何能申请到某服务票据的人，都可以把它带走、离线破解。
- **口令变更极其重要。** 保护每一张 TGT 的是 `krbtgt` 的口令；域被拿下之后建议"重置两次"是有原因的。

#### 主机上的凭据住在哪

| 位置 | 装什么 |
|---|---|
| **LSASS** 进程内存 | 明文口令（有时）、NT 哈希、Kerberos 票据 |
| **SAM** 注册表 hive | 本地账户哈希 |
| **NTDS.dit**（在域控上） | 全部域账户哈希 |
| **DPAPI** | 按用户或按机器加密的机密 —— 浏览器口令、保存的凭据 |
| **凭据管理器** | 保存的网络与 RDP 凭据 |

这份清单就是 LSASS 成为 Windows 上被攻击最多的进程的原因，也是为什么保护它（Credential Guard、LSA 保护、限制谁能调试进程）是防守方的优先事项。

### 第三部分：注册表

注册表是一个层级式配置数据库 —— 给操作系统、给应用、也给用户设置。

```
HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run
└──────┬───────┘└──────────────┬──────────────────────────────┘
     配置单元(hive)              项(key)                  值(value)
```

你会遇到的 hive：

| Hive | 内容 |
|---|---|
| `HKLM` | 全机器范围的设置 —— 服务、驱动、已安装软件 |
| `HKCU` | 当前用户的设置 |
| `HKU` | 所有已加载的用户配置 |
| `HKCR` | 文件关联与 COM 注册 |

攻防双方都关心的项：

| 项 | 为什么 |
|---|---|
| `...\CurrentVersion\Run` / `RunOnce` | 开机自动启动的程序 —— 经典持久化位置 |
| `HKLM\SYSTEM\CurrentControlSet\Services` | 服务定义 |
| `HKLM\SAM`、`HKLM\SECURITY` | 本地哈希与 LSA 机密（系统运行时读不到） |
| `...\Image File Execution Options` | 调试器重定向 —— 一种安静的劫持执行的方式 |
| `...\Winlogon` | Shell、userinit —— 另一个持久化位置 |
| `...\Policies` | 由组策略下发的设置 |

```cmd
reg query "HKLM\Software\Microsoft\Windows\CurrentVersion\Run"
reg query HKLM\SYSTEM\CurrentControlSet\Services /s | findstr /i imagepath
reg add "HKCU\Software\Example" /v Setting /t REG_SZ /d value /f
```

### 第四部分：文件系统与执行

**NTFS** 和 Linux 用户的预期有两处不同，值得知道：

- **ACL** 是显式的"谁能做什么"列表，带继承和拒绝项。它比 `rwx` 表达力更强，也因此更容易配错。
- **备用数据流（ADS）** 让一个文件可以携带多于一个数据流 —— `file.txt:hidden.txt` 就是同一个文件里的第二个流。多数复制操作会把它丢掉，多数工具也不显示它。

值得知道的目录：

| 路径 | 内容 |
|---|---|
| `C:\Windows\System32` | 核心二进制与 DLL |
| `C:\Windows\Temp`、`C:\Temp` | 很多用户可写 |
| `C:\ProgramData` | 全机器范围的应用数据，常常所有人可写 |
| `C:\Users\<用户>\AppData\Roaming` | 按用户的应用数据，会跟着用户换机器 |
| `C:\Users\<用户>\AppData\Local` | 按用户、但留在本机的数据 |

**PowerShell** 既是管理的中心，也因此是攻防两边的中心。它是一个完整的编程环境，能直接调用 .NET —— 所以既被用来做自动化，也被用来做攻击。而只要你打开那几项日志，它就会留痕，这是防守方的对价。

**LOLBins**（"就地取材的二进制"）是被微软签名的程序，可以被滥用来做它不是本意的事 —— `certutil` 下载文件、`rundll32` 执行 DLL、`mshta` 跑脚本、`regsvr32` 拉取并执行。它们要紧的原因是：**它们都是合法软件**，基于文件名的黑名单抓不到它们。

### 第五部分：Active Directory

#### 域是什么

**域**是一个集中的身份与管理边界。不再是每台机器各自维护账号，而是由一个目录保存账号，所有加入的机器都信任它。

- **域控制器（DC）** —— 运行 AD 服务的服务器。它保存数据库（`NTDS.dit`）、应答认证、并提供 LDAP 查询。
- **目录** —— 一棵对象层级：用户、计算机、组、组织单元（OU）、组策略对象。
- **LDAP** 是你读写它的方式。对象有一个**可分辨名称**：

```
CN=Alice Smith,OU=Staff,DC=corp,DC=local
│             │        │
│             │        └ 域组件
│             └ 组织单元（一个容器，也是策略被施加的地方）
└ 通用名
```

- **组策略（GPO）** —— 从域下发到机器和用户的设置与脚本。一个 GPO 可以安装软件、映射驱动器、设置注册表值、运行登录脚本、修改本地管理员。它是域的配置管理，因此也是很有吸引力的目标。

#### 加入域为什么影响这么大

两个结构性的点：

1. **域管理员可以管理所有加入域的机器。** 按设计，Domain Admins 是每台加入域的计算机上的本地管理员。
2. **机器信任域的认证。** 加入域机器上的服务，会接受由域控签发的 Kerberos 票据。

合起来就意味着：域才是奖品 —— 拿下一台工作站只是一个落脚点，拿下目录就是一切。

#### 信任

域之间可以互相建立信任，于是一个域的身份会被另一个域接受。在同一个林内，信任通常是**可传递的**。这也正是"我们的域是独立的"这句话在实践中经常不成立的原因，也是为什么信任的方向和可传递性，是项目一开始就要摸清的东西之一。

#### 你会听到名字的那些组成部分

| 术语 | 是什么 |
|---|---|
| **OU** | 对象的容器；策略施加的单元 |
| **GPO** | 一组设置，链接到站点、域或 OU |
| **SPN** | 某个服务实例的标识符，Kerberos 用 |
| **ACL / ACE** | 目录对象上的权限，以及其中每一条 |
| **委派（Delegation）** | 让一个服务代表用户行事 |
| **SYSVOL** | 每台 DC 上的共享，装着策略与脚本，所有域用户可读 |
| **全局编录** | 整个林的部分副本，用于快速查找 |
| **FSMO 角色** | 五个单主角色，其中包含发放 RID 的那个 |

### 第六部分：你实际会敲的命令

```cmd
:: 身份 —— 在任何主机上的第一条命令
whoami /all
whoami /priv
net user %USERNAME% /domain
net group "Domain Admins" /domain
net localgroup Administrators

:: 机器与网络
systeminfo
hostname
ipconfig /all
route print
arp -a
nslookup corp.local
nltest /dclist:corp.local
net view /domain

:: 进程、服务、任务
tasklist /v
tasklist /svc
sc query
sc qc <服务名>
schtasks /query /fo LIST /v

:: 文件与注册表
dir /a
type file.txt
findstr /si password *.txt *.xml *.config
where python
reg query HKLM\Software\Microsoft\Windows\CurrentVersion\Run

:: 域查询（装了 RSAT 才有）
dsquery user -limit 0
dsquery computer
dsquery group -name "Domain Admins"
```

以及对应的 PowerShell 写法，因为现代环境用的是它：

```powershell
Get-Process | Sort-Object CPU -Descending | Select-Object -First 10
Get-Service | Where-Object Status -eq 'Running'
Get-LocalUser
Get-LocalGroupMember -Group Administrators
Get-ADUser -Filter * -Properties * | Select-Object Name, LastLogonDate
Get-ADGroupMember -Identity "Domain Admins"
Get-ADComputer -Filter * | Select-Object Name, OperatingSystem
Get-CimInstance Win32_Service | Select-Object Name, StartMode, PathName
```

### 第七部分：事件日志

这是防守方的那一半，而攻击者也该懂它 —— 因为那就是他留下的痕迹。

| 日志 | 装什么 |
|---|---|
| Security | 登录、特权使用、账号变更、对象访问 |
| System | 服务、驱动、硬件 |
| Application | 应用事件 |
| PowerShell | 脚本执行（如果开启了） |
| Sysmon（若部署） | 带哈希的进程创建、网络连接、文件与注册表事件 |

该背下来的事件 ID：

| ID | 含义 | 为什么重要 |
|---|---|---|
| 4624 | 登录成功 | 登录类型说明怎么登的（2 交互、3 网络、10 RDP） |
| 4625 | 登录失败 | 口令猜测的信号 |
| 4648 | 使用显式凭据登录 | `runas`，以及任何用别的账号的操作 |
| 4672 | 分配了特殊特权 | 管理员登录时就会触发 —— "这是一个特权会话"的信号 |
| 4688 | 进程创建 | 进程树（前提是开了命令行审计） |
| 4720 / 4726 | 创建 / 删除账号 | 新账号值得问一句 |
| 4728 / 4732 / 4756 | 加入组 | 成员关系变更是权限被授予的方式 |
| 4768 / 4769 / 4771 | Kerberos TGT / 服务票据 / 预认证失败 | Kerberos 活动的审计轨迹 |
| 7045 | 安装服务 | 经典的持久化机制 |
| 1102 | 审核日志被清空 | 有人打扫了现场。这是一起事件 |

```powershell
Get-WinEvent -LogName Security -MaxEvents 20
Get-WinEvent -FilterHashtable @{LogName='Security'; Id=4624} -MaxEvents 20
wevtutil qe Security /c:20 /rd:true /f:text
```

### 第八部分：安全地图（概念层）

这一篇是地基。这里每一条后面都有自己的专篇；现在要紧的是你能把它们**摆对位置**：

| 概念 | 想法 | 它为什么能成为攻击 |
|---|---|---|
| 本地管理员 | 控制一台机器 | 通常是往上走的第一步，而且常常在机器之间复用 |
| 凭据提取 | 从内存里读出哈希与票据 | 因为拥有哈希就能认证你 |
| Pass-the-hash / pass-the-ticket | 使用偷来的哈希或票据 | NTLM 与 Kerberos 分不清"偷来的凭据"和"主人的凭据" |
| Kerberoasting | 申请服务票据并离线破解 | 服务票据是用服务账号的哈希加密的 |
| ACL 滥用 | 一条能控制某个对象的权限 | 目录本身就是一个访问控制系统 |
| 委派滥用 | 服务代表用户行事 | 按设计很方便，配错时很危险 |
| ADCS | 域里的证书服务 | 证书可以当凭据用 |
| GPO 滥用 | 策略会被下发到机器 | 策略可以运行代码、可以改本地管理员 |
| DCSync | 复制目录数据 | 复制协议是拿到全部哈希最快的路 |

### 检测与缓解

- **给管理模型分层。** Tier 0 账号只登 Tier 0 系统、管理普通工作站用另一个账号、域管会话里不查邮件。这是唯一能从结构上挫败上面那张表里大部分手法的办法。
- **保护凭据。** 平台支持就上 Credential Guard 和 LSA 保护；用 `LAPS` 让本地管理员口令各不相同，这样一次失陷不会打开每一台机器；脚本和 GPO 首选项里不放口令。
- **削减遗留协议面。** 能限制就限制、能禁用就禁用 NTLM，要求 SMB 签名，清掉无约束委派。
- **把你将来需要的日志打开。** 4688 的命令行审计、PowerShell 脚本块日志，能部署就上 Sysmon。没有这些，上面那些事件大多数根本不存在。
- **对"形状"告警，而不是对孤立事件告警。** 服务账号在做交互式登录、一个账号对大量 SPN 突发 4769、一台不是域控的主机发出复制请求、多出一个 Domain Admin —— 这些都是序列，每一个都值得一条规则。
- **把事件日志当证据，并保护它。** 实时送到主机之外；只存在本地的日志，是入侵者能删掉的日志。
- **假设域就是目标**，监控目录本身，而不只是端点。域失陷里要紧的东西，大多在域自己的日志里看得见。
