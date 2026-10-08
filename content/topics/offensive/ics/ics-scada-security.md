---
id: ics-scada-security
title_en: ICS and SCADA Security
title_zh: 工控 ICS/SCADA 安全
summary_en: Industrial systems are the one domain where a successful test can stop production or hurt somebody. The techniques are ordinary IT techniques applied to protocols designed without security; the discipline is knowing what you must not touch.
summary_zh: 工控是唯一一个"测试成功就可能停产、甚至伤到人"的领域。技术手段其实就是普通 IT 手法作用在那些设计时没有安全概念的上位协议上；真正的专业素养在于知道哪些东西你不能碰。
tags: [ics, scada, plc, modbus, ot, industrial, safety]
tools: [Wireshark, Zeek, Grassmarlin, Shodan, nmap, plcscan]
attck: [T1190, T1595]
platform: [ics]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### Read this before anything else

In every other entry in this guide, a mistake costs a finding. Here it can cost a production line, a physical device, or a person:

- **A write to a PLC is a physical action.** Changing a setpoint, a coil or a register changes what a machine does. There is no undo, and the consequence may be equipment damage or injury.
- **Active scanning can crash legacy controllers.** Some PLCs and RTUs fall over on malformed or unexpected traffic, and a crashed controller may take a process with it.
- **Restarting a device may be impossible during production.** Many controllers cannot be safely rebooted on demand, and some hold state that is not recoverable.
- **The safety system is not the control system.** A Safety Instrumented System exists to prevent harm; touching it, even to read, may need a different authorisation and a different conversation.

So: **passive first, and active only in a test environment or with written authorisation that names the specific actions.** When in doubt, do not send the packet. This is the one area where "I will ask" is always the right answer.

### The Purdue model, as a risk map

| Level | What lives there | Risk of touching it |
|---|---|---|
| 0 | Sensors and actuators, the physical process | Immediate physical effect |
| 1 | PLCs, RTUs, safety controllers | Controls the process directly |
| 2 | HMIs, SCADA servers, engineering workstations | Can command Level 1 |
| 3 | Site operations: historians, patch servers, jump hosts | Where the project files and credentials are |
| 3.5 | DMZ between OT and IT | The designed boundary |
| 4/5 | Enterprise IT and cloud | Where the attack usually starts |

Two things follow. The attack path almost always runs top-down — a phishing email at Level 4 becomes a foothold at Level 3 and then a command at Level 1. And the value of a target goes up as the level number goes down, along with the risk of disturbing it.

The **engineering workstation** at Level 3 deserves special mention. It holds the PLC programming software and, usually, the project files that describe the whole process. Compromising it is how an attacker learns the process well enough to affect it deliberately.

### The protocols, and why they are the way they are

| Protocol | Port | Security model |
|---|---|---|
| Modbus TCP | 502 | None. No authentication, no encryption, no integrity |
| DNP3 | 20000 | Optional authentication in newer versions, rarely deployed |
| IEC 60870-5-104 | 2404 | None in practice |
| IEC 61850 | 102 | Security extensions exist, rarely used |
| S7comm | 102 | Proprietary, historically no authentication |
| EtherNet/IP, PROFINET | 44818, various | Designed for trusted networks |
| OPC UA | 4840 | Has a real security model, and is often deployed without it |

They were designed for isolated serial networks where the only devices present were the ones the vendor installed. A modern plant has those same protocols on Ethernet, sometimes reachable from the corporate network, sometimes from the internet.

The distinction that matters for testing: **read function codes versus write function codes.** Modbus function code 3 (read holding registers) is not the same action as function code 6 (write single register), and only one of them changes the physical world.

### What actually goes wrong

- **Exposure.** Internet-facing Modbus, S7 and HMI panels are routinely found by search engines. This is the most common finding and the least excusable.
- **No authentication.** Any device that can reach the controller can command it. There is no credential to steal because there isn't one.
- **Flat OT networks.** Once on the plant network, everything is reachable, including the safety systems.
- **Old Windows on the HMI and engineering workstation.** Unpatched, sometimes unsupported, and connected to both sides.
- **Remote access.** A cellular modem or a vendor VPN with weak credentials, installed for maintenance and forgotten.
- **Weak change control.** The engineering workstation is shared, its project files are on a network share, and nobody knows who changed the program last.
- **Dual-homed hosts** that bridge OT and IT because somebody needed to copy a file.

### Testing, in the safest order

1. **Paperwork and architecture.** Get the network diagram, the asset list and the change-control process. Much of the assessment can be done by reading.
2. **Passive discovery.** Mirror a switch port and analyse with Wireshark or Zeek with ICS protocol dissectors. You learn the devices, the protocols, the conversations and the polling patterns without sending a single packet. `Grassmarlin` was built for exactly this.
3. **External exposure review.** What is reachable from the internet or from the corporate network — done from outside the OT network, not inside it.
4. **The IT side of OT.** The HMI, the engineering workstation and the historian are ordinary Windows or Linux systems with ordinary weaknesses: missing patches, weak credentials, accessible shares, no MFA.
5. **Anything active, only in the agreed environment.** In a lab or a scheduled window, with the client's engineers present, starting with read-only function codes and stopping at the first sign of an effect.

A useful rule of thumb: if you cannot state exactly what the packet will cause, do not send it.

### Detection, from the defender's side

- **Passive monitoring is the primary control** in an OT network, because you cannot install agents on a PLC. ICS-aware tools (Nozomi, Claroty, Dragos) and Zeek with protocol parsers learn the normal pattern of polling and commands, and alert on deviations: a new device talking Modbus, a workstation sending write function codes it never sends, a scan that touches every unit.
- **Watch the boundary crossings.** DMZ traffic, dual-homed hosts, and remote-access sessions are where the IT-to-OT path lives.
- **Monitor the engineering workstations** as the sensitive Tier 0 systems they are: who logs in, when the programming software runs, and whether project files changed.
- **Alert on configuration changes in the controllers** where the platform supports reading the program checksum, because that is the closest thing OT has to file integrity monitoring.
- **Physical and network access to cabinets** is also an attack path, and often the easiest one.

### Mitigation

- **Segment, and enforce it.** OT separated from IT by a DMZ with a firewall and, where the risk justifies it, a unidirectional gateway. The Purdue model is a design, not a diagram.
- **Never expose OT protocols to the internet.** Not for remote support, not temporarily during a maintenance window.
- **Require authentication for remote access**, through a jump host in the DMZ with MFA, logged and time-limited.
- **Keep the network flat-free**: no dual-homed hosts, no unmanaged switches, no wireless access points bridging into the plant network.
- **Harden the IT side of OT** — patch the HMI and the engineering workstation, restrict the shares, remove the local admin sharing, and treat them as the crown jewels they are.
- **Control engineering changes.** Version the PLC programs, restrict who can deploy them, and review changes out of band.
- **Plan for the fact that you cannot patch a running process.** Compensating controls (segmentation, monitoring, physical controls) are the realistic answer for legacy equipment, and the business needs to fund the eventual replacement.

<!-- lang:zh -->
### 在做任何事之前先读这一段

在这份指南的其他每一篇里，失误的代价是一个发现。**在这里，代价可能是一条产线、一台设备，或者一个人：**

- **对 PLC 的写入就是物理动作。** 改一个设定值、一个线圈或一个寄存器，就是在改机器要做的事。没有撤销，后果可能是设备损坏或人身伤害。
- **主动扫描可能搞崩老旧控制器。** 有些 PLC 和 RTU 在收到畸形或意外流量时会挂掉，而一台挂掉的控制器可能带着整个工艺一起停。
- **生产期间可能根本无法重启设备。** 很多控制器不能按需安全重启，有些还持有不可恢复的状态。
- **安全系统不是控制系统。** 安全仪表系统（SIS）的存在是为了防止伤害；碰它 —— 哪怕是读 —— 也可能需要另一种授权和另一场沟通。

所以：**先被动，主动只在测试环境里做，或者在有书面授权、且授权书明确列出具体动作的情况下做。** 拿不准的时候，**别发包**。这是唯一一个"我去问一下"永远是正确答案的领域。

### Purdue 模型，当作风险地图来读

| 层级 | 这里有什么 | 碰它的风险 |
|---|---|---|
| 0 | 传感器与执行器，物理过程本身 | 立刻产生物理影响 |
| 1 | PLC、RTU、安全控制器 | 直接控制工艺 |
| 2 | HMI、SCADA 服务器、工程师站 | 能指挥 Level 1 |
| 3 | 站点运营：历史库、补丁服务器、跳板机 | 项目文件与凭据在这里 |
| 3.5 | OT 与 IT 之间的 DMZ | 被设计出来的边界 |
| 4/5 | 企业 IT 与云 | 攻击通常从这里开始 |

由此可以推出两件事。攻击路径几乎总是自上而下 —— Level 4 的一封钓鱼邮件变成 Level 3 的落脚点，再变成 Level 1 的一条指令。而目标的价值随层级数字变小而上升，惊动它的风险也一起上升。

Level 3 的**工程师站**值得单独说一句。它装着 PLC 编程软件，通常还装着描述整个工艺的项目文件。拿下它，攻击者才能把工艺理解到足以**刻意**影响它的程度。

### 那些协议，以及它们为什么长成这样

| 协议 | 端口 | 安全模型 |
|---|---|---|
| Modbus TCP | 502 | 没有。无认证、无加密、无完整性 |
| DNP3 | 20000 | 较新版本有可选认证，极少部署 |
| IEC 60870-5-104 | 2404 | 实践中没有 |
| IEC 61850 | 102 | 有安全扩展，极少启用 |
| S7comm | 102 | 私有协议，历史上无认证 |
| EtherNet/IP、PROFINET | 44818 等 | 为可信网络设计 |
| OPC UA | 4840 | 有真正的安全模型，但常常不带它部署 |

它们是为隔离的串行网络设计的，那时网络上只有厂商装的那几台设备。而现代工厂把这些同样的协议跑在以太网上，有时从企业网可达，有时从互联网可达。

测试时最要紧的区分是：**读功能码与写功能码。** Modbus 功能码 3（读保持寄存器）和功能码 6（写单个寄存器）不是同一件事，而其中只有一个会改变物理世界。

### 实际会出什么问题

- **暴露。** 面向互联网的 Modbus、S7 和 HMI 面板，搜索引擎一搜就有。这是最常见的发现，也是最不可原谅的。
- **没有认证。** 任何能到达控制器的设备都能指挥它。这里没有凭据可偷，因为根本没有凭据。
- **扁平的 OT 网络。** 一旦上了工厂网，什么都可达，包括安全系统。
- **HMI 与工程师站上的老旧 Windows。** 没打补丁、有时已不被支持，而且两边都连着。
- **远程访问。** 为了维护装的蜂窝模块或厂商 VPN，凭据很弱，然后被遗忘。
- **变更控制薄弱。** 工程师站是共用的，项目文件在共享盘上，没人知道上一次改程序的是谁。
- **双网卡主机**把 OT 和 IT 连了起来，因为当时有人要拷个文件。

### 测试，按最安全的顺序

1. **文件与架构。** 拿到网络图、资产清单和变更流程。很多评估工作靠阅读就能完成。
2. **被动发现。** 镜像一个交换机端口，用带 ICS 协议解析器的 Wireshark 或 Zeek 分析。你会在**一个包都不发**的情况下了解到设备、协议、会话关系和轮询模式。`Grassmarlin` 就是为此而生的。
3. **外部暴露面复查。** 哪些从互联网或企业网可达 —— 这一步从 OT 网络之外做，而不是在里面做。
4. **OT 的 IT 那一侧。** HMI、工程师站、历史库就是普通的 Windows 或 Linux 系统，带着普通的弱点：缺补丁、弱口令、可访问的共享、没有 MFA。
5. **任何主动动作，只在约定好的环境里做。** 在实验室或排定的窗口内、客户工程师在场、从只读功能码开始，一旦出现任何效果迹象就停。

一条实用的判断标准：**如果你说不清这个包会造成什么，就不要发它。**

### 检测（防守方视角）

- **被动监控是 OT 网络的首要控制**，因为你没法在 PLC 上装 agent。带 ICS 感知的工具（Nozomi、Claroty、Dragos）以及带协议解析器的 Zeek，会学习正常的轮询与指令模式，并对偏离告警：新设备在说 Modbus、某台工作站在发它从没发过的写功能码、一次覆盖每个单元的扫描。
- **盯住边界穿越。** DMZ 流量、双网卡主机、远程访问会话 —— IT 通往 OT 的路就在这些地方。
- **把工程师站当作敏感的 Tier 0 系统来监控**：谁登录、什么时候跑编程软件、项目文件有没有变。
- **在平台支持读取程序校验和的地方，对控制器配置变更告警** —— 这是 OT 里最接近文件完整性监控的东西。
- **对机柜的物理与网络访问**同样是一条攻击路径，而且往往是最容易的那条。

### 缓解

- **分段，并且真正强制它。** OT 与 IT 之间用带防火墙的 DMZ 隔开，风险足够高时上单向网关。Purdue 模型是一份设计，不是一张图。
- **绝不把 OT 协议暴露到互联网。** 不为远程支持开，也不在维护窗口"临时"开。
- **远程访问必须认证**，通过 DMZ 里的跳板机 + MFA，留日志、有时限。
- **网络不要有平面**：不要双网卡主机、不要非管理交换机、不要桥接进工厂网的无线 AP。
- **加固 OT 的 IT 侧** —— 给 HMI 和工程师站打补丁、限制共享、去掉本地管理员共享，把它们当成真正的心脏来对待。
- **管住工程变更。** 给 PLC 程序做版本管理、限制谁能下发、并且带外复核变更。
- **接受"运行中的工艺打不了补丁"这个事实。** 对老旧设备，补偿性控制（分段、监控、物理控制）才是现实答案，而业务方需要为将来的更换出钱。
