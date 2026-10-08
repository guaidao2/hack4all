---
id: c2-fundamentals
title_en: Command and Control Fundamentals
title_zh: C2 命令控制基础
summary_en: A C2 framework is a mail service for an implant — a channel, a beacon schedule and an operator console. What decides whether it survives is not the framework, it is how the traffic looks next to the traffic that is supposed to be there.
summary_zh: C2 框架本质上是一套替植入体收发指令的邮局：一条信道、一个心跳节奏、一个操作台。决定它能不能活下来的不是框架本身，而是它的流量放在"本该存在"的流量旁边像不像。
tags: [c2, command-and-control, red-team, beacon, opsec, detection]
tools: [Sliver, Mythic, Havoc, Cobalt Strike, Merlin]
attck: [T1071.001, T1573, T1090.004]
platform: [windows, linux]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The parts

Every framework is the same five pieces with different polish:

| Part | Job |
|---|---|
| Team server | Coordinates everything: sessions, tasks, output, logs |
| Listener | Where the implant connects to; one per channel |
| Stager | The tiny first stage, when the full implant is too big for the initial delivery |
| Implant / beacon | Runs on the target, checks in, executes tasks |
| Operator client | Your console; usually a GUI, sometimes a CLI |

The design question that matters most is the **check-in model**: does the implant poll (beacon) or hold a connection (interactive)? Polling survives NAT, proxies and flaky networks; it also creates a rhythm, and rhythm is the single easiest thing for a defender to detect.

### Channel selection

| Channel | Good for | What it looks like |
|---|---|---|
| HTTPS to a real-looking domain | Almost everything; blends with browsing | TLS to a domain with a plausible age and certificate |
| DNS (TXT/A/CNAME) | Networks where nothing else egresses | Heavy query volume, long labels, one domain |
| SMB / named pipes | Lateral movement inside a network | No network traffic at all — named pipes between hosts |
| WebSocket or gRPC | Low latency, modern stacks | Long-lived upgrade connections; less common in most environments |
| mTLS with client certs | Frameworks designed around it | Certificate-based, hard to inspect |

For an engagement with real defenders, the honest ranking is usually: HTTPS to good infrastructure first, DNS or SMB only when the network forces it, everything else when you have a specific reason.

### The beacon is the fingerprint

Two settings decide how detectable a beacon is:

- **Sleep interval.** Too short saturates logs and looks like a scanner; too long makes the operator slow. Common ranges are 30–120 seconds for interactive work and 5–60 minutes for long-haul persistence.
- **Jitter.** Randomisation of that interval. **Zero jitter is the classic tell**: a process making a request every 60.000 seconds is not a browser. A useful jitter is 20–40%, which makes the interval distribution overlap with normal application traffic.

The second fingerprint is the **profile**: which URIs, headers, user agent and certificate the traffic uses. A framework's default profile is a signature — default URIs (`/submit.php`, `/updates`), a `User-Agent` that no browser sends, a self-signed certificate with an obvious CN. A malleable profile is not cheating; it is making your traffic look like the application traffic that already exists in that environment, which requires looking at that environment first.

Two things worth being honest about:

- **Domain fronting** (making the TLS SNI differ from the HTTP Host to abuse a CDN) has been largely closed off by the major CDNs. Treat it as a historical technique unless you have verified it works against the specific provider.
- **Redirectors are not optional** in a real engagement: a small VPS or cloud instance that forwards to the team server, so the team server's address never appears in target logs. Use one per channel, and expect to burn them.

### OPSEC checklist

The framework does not create operational security; your habits do:

- Infrastructure per engagement, never reused between clients. New domains, new certificates, new hosting accounts.
- Domain age and category matter more than the domain name: a two-day-old `.xyz` is blocked or flagged by reputation feeds long before anybody inspects the traffic.
- Match the environment's traffic: if the organisation uses a proxy with a specific header, your beacon should go through it, not around it.
- Keep long-running persistence quiet and rare; use short-lived interactive sessions when a human is actually working.
- Expect every action to be logged somewhere, and design the engagement so that the *detection story* you write at the end matches what the blue team can actually see.
- Know your legal boundary: this is for authorised engagements and detection engineering, with a scope document and a point of contact.

### Frameworks, without picking a side

| Framework | Notes |
|---|---|
| Cobalt Strike | The commercial standard for red teams; malleable profiles and a mature ecosystem; also the most-detected, because it is the most-used by criminals |
| Sliver | Open source, Go, mTLS/HTTP/DNS/WireGuard channels, multiplayer |
| Mythic | Open source, modular, language-agnostic agents, good UI |
| Havoc | Open source, C/C++/C# agents, modern evasion focus |
| Merlin | Go, HTTP/2 and HTTP/3, designed around those protocols |

The choice matters far less than the infrastructure and the profile. A well-configured open-source framework beats a default-configured commercial one in every environment that has an EDR.

### Detection

- **Beacon periodicity.** Statistical analysis of connection intervals per host/domain finds fixed-interval polling even through TLS. This is the highest-value detection in this whole area: look for low-variance inter-arrival times, then for a jitter band that is *too* regular.
- **TLS fingerprints.** JA3/JA4 hashes of the client. Frameworks and their language runtimes have distinctive fingerprints; a Go implant does not look like Chrome.
- **Domain reputation.** Newly registered, uncategorised, or rare domains; DNS requests for a domain that no other host in the organisation has ever asked for.
- **Certificate anomalies.** Self-signed, very short validity, obvious CNs, or a certificate that does not match the SNI.
- **Default profiles.** Alerting on known default URIs and user agents catches the low-effort majority, which is most of what happens.
- **Named pipes.** SMB beacons create pipes with recognisable names; Sysmon event 17/18 with a review list is cheap and effective.
- **Volume and direction.** A workstation uploading more than it downloads, consistently, at regular intervals.

### Mitigation

- **Default-deny egress** with an allow-list of destinations per host class; most C2 dies at the firewall before any EDR sees it.
- **TLS inspection** where legally and practically possible, plus DNS logging at the resolver.
- **Block newly registered domains** and uncategorised destinations for a cooling-off period; this alone removes a large share of real-world C2.
- **Segment the network** so SMB beacons cannot traverse it, and monitor named pipe creation between workstations.
- **Baseline normal traffic per host** (volume, destinations, TLS fingerprints) so "different" is measurable rather than a feeling.
- **Practise the detection**: run a known framework in a lab, and check that your rules fire. A detection that has never been tested is a hypothesis.

<!-- lang:zh -->
### 组成部分

所有框架都是同样五块拼装，只是做工不同：

| 组件 | 职责 |
|---|---|
| Team server | 统筹一切：会话、任务、回显、日志 |
| Listener | 植入体连接的地方；每条信道一个 |
| Stager | 极小的第一阶段，用于完整植入体太大没法直接投递的场合 |
| Implant / beacon | 跑在目标上，回连、取任务、执行 |
| 操作端 | 你的控制台；通常是 GUI，也有 CLI |

最关键的设计问题是**回连模型**：植入体是轮询（beacon）还是保持长连接（交互式）？轮询能穿过 NAT、代理和不稳的网络；它同时制造了节奏 —— 而节奏是防守方最容易检测的东西。

### 信道选择

| 信道 | 适合 | 看起来像什么 |
|---|---|---|
| 到仿冒真实域名的 HTTPS | 几乎所有情况；混在浏览流量里 | 指向一个域名年龄与证书都合理的 TLS 连接 |
| DNS（TXT/A/CNAME） | 别的都出不去时的网络 | 查询量高、标签长、集中在一个域名 |
| SMB / 命名管道 | 内网横向 | 完全没有网络流量 —— 主机之间的命名管道 |
| WebSocket 或 gRPC | 低延迟、现代技术栈 | 长连接的 upgrade；在多数环境里并不常见 |
| 双向 TLS | 围绕它设计的框架 | 基于证书，难以检查 |

对有真实防守方的项目，诚实的排序通常是：优先用基础设施做得好的 HTTPS，只有在网络逼你时才用 DNS 或 SMB，其他信道要有明确理由才上。

### beacon 就是指纹

两个设置决定 beacon 有多容易被发现：

- **心跳间隔。** 太短会刷爆日志、看起来像扫描器；太长则操作很慢。交互作业常见 30–120 秒，长期驻留 5–60 分钟。
- **抖动（jitter）。** 对间隔做随机化。**零抖动是经典破绽**：每 60.000 秒精确发一次请求的进程不是浏览器。有用的抖动是 20%–40%，让间隔分布与正常应用的流量重叠。

第二个指纹是 **profile**：流量使用哪些 URI、请求头、User-Agent 和证书。框架的默认 profile 就是特征 —— 默认 URI（`/submit.php`、`/updates`）、浏览器根本不会发的 `User-Agent`、CN 一眼可辨的自签证书。写 malleable profile 不是作弊，而是让你的流量看起来像这个环境里**本来就有**的业务流量 —— 而这需要你先去看那个环境。

两点需要说实在的：

- **域前置**（让 TLS SNI 与 HTTP Host 不一致以滥用 CDN）已经被主流 CDN 基本封堵。除非你在具体服务商上验证过可行，否则把它当作历史技术。
- **重定向器不是可选项**：一台小 VPS 或云实例转发到 team server，这样目标日志里永远不会出现 team server 的地址。每条信道用一个，并且预期它会被烧掉。

### OPSEC 清单

操作安全不是框架给的，是习惯给的：

- 每个项目独立基础设施，绝不在客户之间复用。新域名、新证书、新主机账号。
- 域名**年龄**和分类比域名本身重要：一个两天大的 `.xyz`，在任何人检查流量之前就早被信誉库拦了。
- 匹配环境流量：如果组织走特定代理并带特定请求头，你的 beacon 应该走代理，而不是绕开。
- 长期驻留要安静且少动；有真人在作业时用短命的交互会话。
- 假设每个动作都会被记录在某个地方，并把项目设计成"你最后写的检测故事与蓝队真正能看到的东西对得上"。
- 清楚法律边界：这用于授权项目与检测工程，有范围文件和对接人。

### 框架概览（不站队）

| 框架 | 说明 |
|---|---|
| Cobalt Strike | 红队商业标准；支持 malleable profile，生态成熟；也是被检测最多的，因为犯罪团伙用得最多 |
| Sliver | 开源，Go 编写，mTLS/HTTP/DNS/WireGuard 信道，多人在线 |
| Mythic | 开源、模块化、语言无关的 agent，界面好 |
| Havoc | 开源，C/C++/C# agent，偏现代免杀 |
| Merlin | Go，基于 HTTP/2 与 HTTP/3 设计 |

框架选择远不如基础设施和 profile 重要。在任何有 EDR 的环境里，配置得当的开源框架都胜过默认配置的商业框架。

### 检测

- **beacon 周期性。** 按主机/域名统计连接间隔，即便走 TLS 也能找出固定间隔的轮询。这是这一整块里价值最高的检测：先找低方差的到达间隔，再找"抖得太规律"的抖动带。
- **TLS 指纹。** 客户端的 JA3/JA4 哈希。框架及其语言运行时都有独特指纹；Go 写的植入体看起来不像 Chrome。
- **域名信誉。** 新注册、未分类或极少见的域名；组织内其他主机从未查询过的域名。
- **证书异常。** 自签、有效期极短、CN 一眼可辨，或者证书与 SNI 不匹配。
- **默认 profile。** 对已知的默认 URI 与 User-Agent 告警，能抓住那些低成本的大多数 —— 而大多数情况就是这些。
- **命名管道。** SMB beacon 会创建名字可辨的管道；用 Sysmon 的 17/18 号事件配一份复查清单，成本低、效果好。
- **流量方向与体量。** 一台工作站持续地上传大于下载，并且时间间隔规律。

### 缓解

- **出站默认拒绝**，按主机类别给目标允许清单；大多数 C2 在防火墙就被掐死，EDR 根本看不到。
- **法律与工程条件允许时做 TLS 检查**，并在解析器上记录 DNS 日志。
- **阻断新注册域名与未分类目标**，设一段观察期；光这一条就能消掉现实中很大一部分 C2。
- **网络分段**，让 SMB beacon 走不通，并监控工作站之间的命名管道创建。
- **给每台主机建正常流量基线**（体量、目标、TLS 指纹），让"不一样"可度量，而不是靠感觉。
- **演练检测**：在实验室里跑一个已知框架，验证你的规则会不会响。从未被测过的检测只是假设。
