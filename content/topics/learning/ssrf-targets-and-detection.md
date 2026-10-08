---
id: ssrf-targets-and-detection
title_en: "SSRF, Part 2 — Targets, Blind Probes and Detection"
title_zh: "SSRF（二）：目标、盲探与检测"
summary_en: What the borrowed position is worth depends on what is behind it, and the most valuable target is not at a fixed address. This entry covers cloud metadata across providers, gopher as a general byte-sending primitive with a measured example, blind probing by timing, and the detection that holds when the response tells you nothing.
summary_zh: 借来的位置值多少，取决于它后面有什么；而价值最高的那个目标并不在一个固定的地址上。这一篇讲各家云的元数据、讲 gopher 作为通用"发字节"原语（带实测）、讲靠时间做盲探，以及当响应什么都不告诉你时仍然成立的那些检测。
tags: [web, ssrf, cloud-metadata, gopher, cwe-918, blind-ssrf]
tools: [curl, python3, Burp Suite, dig]
attck: [T1190, T1552.005]
platform: [web, cloud]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### What the position is worth

Part 1 established that the request comes from the server's network position. This part is about what is behind that position and what can be done when the response tells you nothing.

| Behind the position | What it gives |
|---|---|
| **Cloud metadata** | Instance credentials, identity, user data — usually the highest value |
| **Loopback admin interfaces** | Unauthenticated control planes bound to `127.0.0.1` |
| **Internal HTTP services** | Service meshes, dashboards, CI, orchestration APIs |
| **Internal datastores** | Redis, Memcached, databases — reachable via their wire protocols |
| **The filesystem** | Where the client supports `file://` |
| **The network itself** | Port and host discovery through timing and error differences |

The first row deserves more detail than the others, because it is the reason SSRF is treated as a severe class rather than an annoyance.

### Cloud metadata: the target that is not at a fixed address

Every major provider exposes an instance metadata service on a **link-local address**, reachable from the instance without credentials, and the address and the required headers differ between them.

| Provider | Endpoint | Required header or step |
|---|---|---|
| **AWS** | `http://169.254.169.254/latest/meta-data/` | None for IMDSv1; **IMDSv2 requires a `PUT` to obtain a token first** |
| **Azure** | `http://169.254.169.254/metadata/instance?api-version=...` | `Metadata: true` |
| **GCP** | `http://metadata.google.internal/computeMetadata/v1/` | `Metadata-Flavor: Google` |
| **Alibaba Cloud** | `http://100.100.100.200/latest/meta-data/` | None |
| **Tencent Cloud** | `http://metadata.tencentyun.com/latest/meta-data/` | None |

Three consequences follow, and they are the practical content of this section.

**A blacklist containing `169.254.169.254` does not cover metadata.** Alibaba's endpoint is at `100.100.100.200`, several providers use a hostname, and the IPv6 metadata address (`fd00:ec2::254` on AWS) is a different literal again. The target is identified by **what it is**, not by one string — which is the same lesson as part 1's address spellings, applied to the destination instead of the source.

**The header requirement decides whether a given SSRF can reach it.** A client that lets the attacker set request headers can satisfy `Metadata: true` or `Metadata-Flavor: Google`; a client that only follows a URL cannot. That is why "can this feature set headers" is a question worth asking during triage: it separates a low-impact SSRF from one that returns credentials.

**IMDSv2 is a target-side control that works.** Requiring a token obtained through a `PUT` with a hop limit means a simple GET-based SSRF cannot read metadata, regardless of how the address was written. It is the kind of mitigation worth deploying precisely because the caller-side check has so many ways to go wrong.

What metadata yields, when it is reachable: **temporary credentials** for the instance's role (an access key, a secret and a session token), the **user data** supplied at launch — which is often a startup script containing secrets — and the instance's identity and network configuration. Temporary credentials from a metadata read are the usual bridge from a web vulnerability to a cloud account, and the damage they do is decided by the **role's permissions** rather than by the SSRF.

### gopher and the step from "forge a request" to "send bytes"

An SSRF that only speaks HTTP is limited to what HTTP can express. A client that supports `gopher://` is not: **the scheme payload is sent verbatim to the socket**, which means arbitrary bytes to an arbitrary port.

Measured, sending a Redis protocol command to a socket that records what arrives:

```
request:  gopher://127.0.0.1:6390/_<URL-encoded RESP>
sent:     *3\r\n$3\r\nSET\r\n$5\r\npwned\r\n$2\r\nok\r\n
received: *3\r\n$3\r\nSET\r\n$5\r\npwned\r\n$2\r\nok\r\n\r\n
```

The bytes arrive; the `SET` command is intact. One detail worth knowing rather than assuming: **the received data is one CRLF longer than what was sent**, because the client appends a line terminator. That is harmless for a line-based protocol and it is the kind of thing that matters when a payload has to be byte-exact.

With that primitive, the reachable targets are the ones that speak a command language over a plain socket:

| Protocol | What an attacker does with it |
|---|---|
| **Redis** | Write a key that becomes a scheduled job, an SSH authorised key, or a module load |
| **FastCGI** | Speak to PHP-FPM directly, which is a well-known route to code execution |
| **Memcached** | Read and write cache entries, sometimes including session data |
| **MySQL and others** | Protocol-level features that read files or issue statements |
| **SMTP** | Send mail from the server's position |

So the question "which schemes does our HTTP client support" is not a detail. It decides whether this class is *forge a request* or *send arbitrary bytes anywhere the server can reach* — and the second is a much larger primitive.

### Blind SSRF: when the response says nothing

A preview that returns no content, or a webhook that only reports success, still performs the request — and the request itself is observable through side channels.

**Timing distinguishes three states.** Measured against a local socket, three destinations:

| Destination | Time | Result |
|---|---|---|
| Open port (service listening) | **9.3 ms** | connection succeeded |
| Closed port (nothing listening) | **0.6 ms** | **connection refused** — an immediate RST |
| Unreachable (packets dropped) | **3004 ms** | **timeout** |

Three distinguishable states from timing alone, which is a port scanner:

- **Refused is fast** — often faster than a success, because there is no handshake to complete.
- **Open is slower** — the handshake completes and, depending on the protocol, a banner or response comes back.
- **Filtered is slowest** — the timeout is the signal.

That gives an attacker a map of what is listening inside the network, before any exploiting begins, using nothing but the application's own outbound requests.

**Errors distinguish more.** `DNS resolution failed`, `connection refused`, `connection timed out`, and `certificate error` are four different messages about four different situations, and a verbose error handler turns them into a report.

**And when nothing is observable at all, there is always the attacker's own server.** A destination the attacker controls records the request's arrival: source address, timing, headers. That confirms the SSRF exists and often reveals something about the client, and **DNS resolution is the most sensitive form of it** — a request that never connects can still produce a lookup, and the lookup is visible in logs the attacker can read if they run the authoritative server.

### Bypasses collected

| Layer | What to try |
|---|---|
| **Address spelling** | Octal, hex, decimal integer, omitted parts, bare `0`, trailing dot, IPv4-mapped IPv6 (part 1) |
| **URL structure** | `user@host`, `#` truncation, `%2f`, tab and backslash inside the host |
| **Wildcard DNS** | `10.0.0.1.nip.io` resolves to `10.0.0.1` while the **string contains no address the blocklist knows** — measured, the name resolves correctly |
| **Redirects** | A `302` to an internal address, since validation often covers only the first hop |
| **Protocol** | `gopher://`, `dict://`, `file://`, `ldap://` where the client permits them |
| **IPv6** | Metadata and loopback endpoints have IPv6 forms with different literals |
| **Encoding** | Double encoding, as in the path-traversal entry |

The wildcard-DNS row is worth its own line because it is the cleanest demonstration that a string check cannot work: the hostname is **legitimate, resolves correctly, and contains nothing the filter was told to block**. The filter is not wrong about its pattern; the pattern is the wrong instrument.

### Detection and mitigation

- **Monitor outbound connections from application servers, and alert on destinations that should not exist.** This remains the strongest signal in the class, and for SSRF it has three specific shapes worth alerting on separately: **loopback and private ranges** (part 1), **metadata addresses and hostnames** including the non-`169.254` ones, and **wildcard-DNS domains** like `nip.io`, `sslip.io` and `xip.io`, which are rarely legitimate in application traffic and are otherwise indistinguishable from a normal hostname.
- **Alert on the URL shapes that indicate reasoning about a validator**: an `@` in the authority, a host that is a bare integer or has fewer than four parts, a trailing dot, an IPv4-mapped IPv6 literal, `gopher://`, `dict://`, `file://`, and any scheme the feature does not need.
- **Watch credential usage, not only requests.** The consequence of a successful metadata read is a cloud credential being used from somewhere unexpected. Detection at the cloud provider's audit log — a role's credentials used from a new address, or used for a new API — catches the outcome even when the request was invisible.
- **Treat timing and error differences as reconnaissance.** A burst of requests whose durations cluster around a timeout, or that produce several distinct connection errors, is someone mapping the network. So is a sequence of requests to the same host with incrementing ports.
- **For mitigation, reduce the destination set before improving the check.** A fixed allow-list of hosts, or a dedicated egress proxy, is stronger than any validator because the attacker's input cannot expand the set. Where the destination must be influenced: resolve with the client's resolver, judge the canonical address, and connect to that resolved address.
- **Disable the schemes the feature does not need, and do not follow redirects** unless every hop is validated. Both are small changes that remove a whole category of payload rather than filtering it.
- **Constrain egress at the network layer.** A web tier that can only reach the destinations it requires turns a bypassed validator into a failed connection. For this class in particular, the destination list is usually short and the benefit is unusually clear.
- **Protect metadata at the target, and scope the role.** Require IMDSv2 or the provider's equivalent header, and give the instance role only the permissions the instance needs. The second is the one that limits the damage when everything else failed: an SSRF that retrieves credentials for a role that can do nothing useful is a much smaller incident than one that retrieves an administrator's.

<!-- lang:zh -->
### 这个位置值多少

第一篇立起了"请求来自服务器的网络位置"。这一篇讲那个位置后面有什么，以及当响应什么都不告诉你时还能做什么。

| 位置后面 | 它给出什么 |
|---|---|
| **云元数据** | 实例凭据、身份、用户数据 —— 通常是价值最高的 |
| **回环管理界面** | 绑在 `127.0.0.1` 上的无认证控制面 |
| **内部 HTTP 服务** | 服务网格、看板、CI、编排 API |
| **内部数据存储** | Redis、Memcached、数据库 —— 通过它们的线协议可达 |
| **文件系统** | 在客户端支持 `file://` 的地方 |
| **网络本身** | 靠时间与错误差异做端口与主机发现 |

第一行值得比别的多讲一些，因为它是 SSRF 被当作严重类别、而不是一个恼人小问题的原因。

### 云元数据：那个不在固定地址上的目标

每一家主要云厂商都在一个**链路本地地址**上暴露实例元数据服务，从实例内部无需凭据即可访问，而地址与所需请求头各家不同。

| 厂商 | 端点 | 需要的头或步骤 |
|---|---|---|
| **AWS** | `http://169.254.169.254/latest/meta-data/` | IMDSv1 无要求；**IMDSv2 需要先 `PUT` 取得令牌** |
| **Azure** | `http://169.254.169.254/metadata/instance?api-version=...` | `Metadata: true` |
| **GCP** | `http://metadata.google.internal/computeMetadata/v1/` | `Metadata-Flavor: Google` |
| **阿里云** | `http://100.100.100.200/latest/meta-data/` | 无 |
| **腾讯云** | `http://metadata.tencentyun.com/latest/meta-data/` | 无 |

由此有三个后果，而它们就是这一节的实用内容。

**一份含 `169.254.169.254` 的黑名单并不覆盖元数据。** 阿里云的端点在 `100.100.100.200`，好几家用的是主机名，而 AWS 的 IPv6 元数据地址（`fd00:ec2::254`）又是另一个字面量。这个目标是由**它是什么**来识别的，不是由某一个字符串 —— 这和第一篇里地址拼法是同一课，只不过用在了目的地而不是来源上。

**对请求头的要求，决定了一次具体的 SSRF 能不能够到它。** 一个允许攻击者设置请求头的客户端能满足 `Metadata: true` 或 `Metadata-Flavor: Google`；一个只能给一个 URL 的客户端不能。这就是为什么"这个功能能不能设置请求头"是分级时值得问的问题：它把一次低影响的 SSRF 和一次能拿到凭据的 SSRF 分开。

**IMDSv2 是一项管用的目标侧控制。** 要求通过一个带跳数限制的 `PUT` 取得令牌，意味着一次简单的基于 GET 的 SSRF 无论地址怎么写的都读不到元数据。这正是那种值得部署的缓解，因为调用侧的检查有太多出错的方式。

元数据可达时能给出什么：实例角色对应的**临时凭据**（一个 access key、一个 secret 和一个会话令牌）、启动时提供的**用户数据**（往往是一段含密钥的启动脚本）、以及实例的身份与网络配置。从元数据读到的临时凭据，通常就是把一个 Web 漏洞接到云账号上的那座桥，而它们造成的损害由**角色的权限**决定，而不是由 SSRF 决定。

### gopher，以及从"伪造一个请求"到"发送字节"的那一步

一个只会说 HTTP 的 SSRF 受限于 HTTP 能表达的东西。而一个支持 `gopher://` 的客户端不受此限：**协议的载荷被原样发到套接字上**，也就是任意字节发到任意端口。

实测，把一条 Redis 协议命令发到一个记录收到的字节的套接字：

```
请求:  gopher://127.0.0.1:6390/_<URL 编码的 RESP>
发出:  *3\r\n$3\r\nSET\r\n$5\r\npwned\r\n$2\r\nok\r\n
收到:  *3\r\n$3\r\nSET\r\n$5\r\npwned\r\n$2\r\nok\r\n\r\n
```

字节到了，`SET` 命令完好。有一个细节值得知道而不是想当然：**收到的数据比发出的多一个 CRLF**，因为客户端追加了一个行终止符。对一个行式协议来说这是无害的，而在一个必须逐字节精确的载荷上，这就是要紧的事。

有了这个原语，可到达的目标就是那些在一个普通套接字上说命令语言的：

| 协议 | 攻击者拿它做什么 |
|---|---|
| **Redis** | 写一个键，让它变成计划任务、SSH 授权密钥，或者模块加载 |
| **FastCGI** | 直接对 PHP-FPM 说话，这是一条众所周知的通往代码执行的路径 |
| **Memcached** | 读写缓存条目，有时包括会话数据 |
| **MySQL 等** | 那些读文件或直接执行语句的协议级特性 |
| **SMTP** | 从服务器的位置发信 |

所以"我们的 HTTP 客户端支持哪些协议"不是一个细节。它决定这一类的性质是**伪造一个请求**，还是**把任意字节发到服务器能到的任何地方** —— 而后者是一个大得多的原语。

### 盲 SSRF：当响应什么都不说

一个不返回内容的预览，或者一个只报告成功与否的 webhook，**仍然执行了那个请求** —— 而请求本身能通过侧信道被观察到。

**时间能区分三种状态。** 对一个本地套接字实测三个目的地：

| 目的地 | 耗时 | 结果 |
|---|---|---|
| 开着的端口（有服务在听） | **9.3 ms** | 连接成功 |
| 关闭的端口（没人听） | **0.6 ms** | **连接被拒绝** —— 立即 RST |
| 不可达（包被丢弃） | **3004 ms** | **超时** |

仅凭时间就是三种可区分的状态，而这已经是一个端口扫描器：

- **被拒绝很快** —— 常常比成功还快，因为没有握手要完成。
- **开着慢一些** —— 握手完成，而且取决于协议，会有 banner 或响应回来。
- **被过滤最慢** —— 那个超时就是信号。

这就给了攻击者一张"网络里什么在听"的地图，在任何利用开始之前，用的只是应用自己的出站请求。

**错误能区分更多。** "DNS 解析失败"、"连接被拒绝"、"连接超时"和"证书错误"是四条关于四种不同情况的不同消息，而一个啰嗦的错误处理器把它们变成一份报告。

**而当完全观察不到任何东西时，总还有攻击者自己的服务器。** 一个由攻击者控制的目的地会记录请求的到达：源地址、时间、请求头。这确认了 SSRF 存在，而且常常透露一些关于客户端的信息；而 **DNS 解析是它最敏感的形式** —— 一次从未连上的请求仍然能产生一次查询，而如果攻击者自己运营权威服务器，那次查询在他能读到的日志里。

### 绕过汇总

| 层 | 该试什么 |
|---|---|
| **地址拼法** | 八进制、十六进制、十进制整数、省略段、裸 `0`、末尾点、IPv4 映射的 IPv6（第一篇） |
| **URL 结构** | `用户@主机`、`#` 截断、`%2f`、主机里的 tab 与反斜杠 |
| **通配 DNS** | `10.0.0.1.nip.io` 解析到 `10.0.0.1`，而**字符串里不含任何黑名单认识的地** —— 实测该名字解析正确 |
| **重定向** | 一个指向内网地址的 `302`，因为校验常常只覆盖第一跳 |
| **协议** | 客户端允许时的 `gopher://`、`dict://`、`file://`、`ldap://` |
| **IPv6** | 元数据与回环端点都有字面量不同的 IPv6 形式 |
| **编码** | 双重编码，如路径穿越那篇所述 |

通配 DNS 那一行值得单独一句，因为它是"字符串检查不可能管用"最干净的演示：那个主机名**是合法的、解析是正确的、而且不含任何过滤器被告知要封的东西**。过滤器对它的模式没有错；那个模式本身就是错的工具。

### 检测与缓解

- **监控从应用服务器发出的出站连接，并对"不该存在的目的地"告警。** 这依然是这一类里最强的信号，而对 SSRF 来说它有三种值得分别告警的形状：**回环与私有网段**（第一篇）、**元数据地址与主机名**（包括那些不是 `169.254` 的）、以及 `nip.io`、`sslip.io`、`xip.io` 这类**通配 DNS 域名** —— 它们在应用流量里很少是正当的，而除此之外与一个普通主机名无法区分。
- **对"有人在推理一个校验器"的 URL 形状告警**：authority 部分里的 `@`、一个裸整数或段数少于四的主机、末尾的点、IPv4 映射的 IPv6 字面量、`gopher://`、`dict://`、`file://`，以及那个功能不需要的任何协议。
- **盯凭据的使用，而不只是盯请求。** 一次成功的元数据读取，其后果是一个云凭据从意想不到的地方被使用。在云厂商的审计日志上做检测 —— 某个角色的凭据从一个新地址、或用于一个新 API —— 能在请求本身不可见时抓到结果。
- **把时间与错误的差异当作侦察。** 一批耗时聚集在超时附近的请求，或者产生好几种不同连接错误的请求，就是有人在测绘网络。对同一主机用递增端口发一串请求也是。
- **缓解上，先缩小目的地集合，再改进检查。** 一份固定的主机允许清单，或者一个专门的出网代理，比任何校验器都强，因为攻击者的输入无法扩大那个集合。在目的地必须被影响的地方：用客户端的解析器解析、判断规范地址、并连到那个解析出来的地址。
- **关掉那个功能不需要的协议，并且不要跟随重定向** —— 除非每一跳都被校验。两者都是小改动，而它们移除的是整整一类载荷，而不是过滤它。
- **在网络层约束出网。** 一个只能到达它需要的那些目的地的 Web 层，会把一次被绕过的校验变成一次失败的连接。对这一类尤其如此：目的地清单通常很短，而收益格外清楚。
- **在目标侧保护元数据，并给角色划定范围。** 要求 IMDSv2 或厂商的等价请求头，并只给实例角色它需要的权限。第二条是在其他一切都失败时限制损害的那条：一次 SSRF 取到的是一个"什么都做不了"的角色的凭据，比起取到管理员凭据，是一起小得多的事故。
