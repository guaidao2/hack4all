---
id: lab-setup
title_en: Building a Practice Lab
title_zh: 练习环境搭建
summary_en: You cannot learn this by reading, and you must not learn it on somebody else's production network. This entry is the middle ground — a lab you can break, snapshot and rebuild — with the network layout explained rather than just the install steps.
summary_zh: 这件事光靠读是学不会的，而你又绝不能在别人的生产网络上练。这一篇讲的就是中间那块地方 —— 一个你可以随便搞坏、随时快照回滚、随时重建的练习环境。网络怎么布是关键，所以讲的是原理，而不只是安装步骤。
tags: [beginner, lab, virtualization, docker, isolation, practice]
tools: [VirtualBox, VMware, Hyper-V, Docker, Kali Linux]
attck: [T1583]
platform: [any]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Why a lab, and why it must be isolated

Three reasons to build one before touching anything else:

1. **Learning requires breaking things.** You will misconfigure, crash and infect your own practice machines repeatedly. That has to be free.
2. **Snapshots turn mistakes into nothing.** "I broke it" becomes "I rolled back and tried again", which is the single biggest accelerator in this field.
3. **Isolation is a technical requirement, not a courtesy.** A deliberately vulnerable machine on a shared network is a machine you have handed to anyone who scans it. That is true of your home network and more true of a company one.

So the goal is a lab that is **disposable** (snapshot and roll back), **isolated** (it cannot reach anything that matters) and **repeatable** (you can rebuild it from notes).

### Part 1: the isolation layers

| Layer | How it isolates | Trade-off |
|---|---|---|
| **Separate physical machine** | Completely | Costs money, needs space |
| **Virtual machine** | Hardware virtualisation, separate kernel | The standard choice; some overhead |
| **Container** | Namespaces and cgroups — **shares the host kernel** | Fast and light, but weaker isolation |
| **Cloud instance** | Someone else's hardware, your network rules | Good for offloading; costs money and data leaves your network |

The important line is between a VM and a container. A container escape is a real thing, so an untrusted binary belongs in a VM — and a container running an intentionally vulnerable web app is fine, because the risk is the app, not the container.

### Part 2: virtual machines

#### Which hypervisor

| Type | Examples | Notes |
|---|---|---|
| **Type 1** (bare metal) | ESXi, Proxmox, Hyper-V Server | Runs on the hardware; used in labs with several machines |
| **Type 2** (hosted) | VirtualBox, VMware Workstation/Fusion, Hyper-V on Windows | Runs inside your OS; easiest to start with |

For a first lab, a Type 2 hypervisor is right. VirtualBox is free and cross-platform; VMware Workstation is more polished; Hyper-V is already on Windows Pro but conflicts with some other hypervisors.

#### What the host needs

| Resource | Realistic minimum | Why |
|---|---|---|
| RAM | 16 GB | One Windows VM can take 4 GB; two or three VMs plus your desktop needs room |
| CPU | 4 cores with virtualisation enabled | Check it is enabled in BIOS/UEFI, not just "supported" |
| Disk | 100 GB free, SSD | VMs are large; SSD makes them usable |
| Network | Anything | Updates and downloads only |

Enable **nested virtualisation** if your CPU supports it, and disable Hyper-V when using VirtualBox or VMware on Windows if you hit conflicts.

#### Snapshots, and how to use them

A snapshot records the machine's state so you can return to it. The habit worth forming:

- **Before every test session:** take a snapshot named for what you are about to do.
- **After a test that got messy:** roll back rather than repair. Repairing a corrupted lab machine teaches you nothing about security and costs an hour.
- **Keep one clean "base" snapshot** you never overwrite, and clone from it rather than starting from an ISO each time.

### Part 3: network modes, which are where beginners get lost

This is the part that decides whether your lab works, and it is worth understanding rather than clicking through.

| Mode | The VM can reach | Can be reached from | Use it for |
|---|---|---|---|
| **NAT** | The internet, through the host | Not from outside | Updates, downloads |
| **Bridged** | Everything the host can reach, as a peer on the LAN | Yes, by anything on the LAN | **Almost never in a lab** — this is how a vulnerable VM ends up exposed |
| **Host-only** | The host, and other VMs on the same host-only network | Other VMs on that network | The isolated lab network |
| **Internal** | Only other VMs on that network, not even the host | Same | Full isolation |

**The layout that works for most beginners** is two adapters on the attack machine:

- **Adapter 1: NAT** — so you can install tools and update.
- **Adapter 2: Host-only** — the isolated network where the targets live.

The targets get **Host-only only**. That way your vulnerable machines can talk to your attack machine, cannot reach the internet, and cannot be reached from your home or office network.

```bash
# on the attack VM (Kali), once the host-only adapter is attached
ip a                                  # find the interface name, e.g. eth1
sudo ip addr add 192.168.56.10/24 dev eth1
sudo ip link set eth1 up
ping -c2 192.168.56.1                 # the host-only gateway answers
```

### Part 4: where targets come from

| Source | Examples | Notes |
|---|---|---|
| **Online platforms** | Hack The Box, TryHackMe, PortSwigger Academy | Zero setup, but you learn less about infrastructure |
| **Downloadable VMs** | VulnHub images, Metasploitable | Realistic, run locally |
| **Containerised apps** | DVWA, OWASP Juice Shop, VAmPI, WebGoat | Seconds to start, ideal for a first target |
| **Your own code** | A deliberately vulnerable app you write | The best teacher, because you know what is wrong |
| **AD labs** | GOAD, DetectionLab-style builds | Several Windows VMs and a domain controller; heavy on RAM |
| **CTF attachment** | Files from a past challenge | Good for the analysis skills |

**Start with a container.** It takes seconds and gives you a real target:

```bash
docker run -d -p 8080:80 vulnerables/web-dvwa        # DVWA
docker run -d -p 3000:3000 bkimminich/juice-shop     # OWASP Juice Shop
```

```bash
# a network just for the lab, so containers do not sit on your default bridge
docker network create labnet
docker run -d --name dvwa --network labnet -p 127.0.0.1:8080:80 vulnerables/web-dvwa
```

Two containers notes worth knowing: publishing a port to `127.0.0.1` only keeps it off your LAN, and a container shares the host kernel, so an app that is meant to be exploitably dangerous is fine but an untrusted binary is not.

### Part 5: layouts for the four situations you will meet

**1. First target — one attack machine, one target**

```
[ Kali VM ] --host-only--> [ target VM or container ]
      |
      +-- NAT --> internet (updates only)
```

**2. Several targets — an internal network**

Create a host-only network (`192.168.56.0/24`), attach every VM to it, and treat it as "the corporate network". This is where you practise discovering hosts you were not told about:

```bash
nmap -sn 192.168.56.0/24          # who is alive
```

**3. A domain lab**

A domain controller plus one or two member servers plus a workstation. This is the only way to practise Active Directory properly, and it is where a machine with 16 GB starts to feel small. Build it once, snapshot everything at "domain is up and clean", and clone from there.

**4. Malware analysis**

This needs more than isolation: no network at all, or a simulated one, plus no shared folders and no clipboard. The separate entry on malicious documents covers that workflow, and the rule there is stronger — the sample must not be able to reach anything, and nothing should be able to reach the sample.

### Part 6: isolation hygiene

- **Never bridge a vulnerable VM.** If it is on your LAN, you have added a compromised machine to your network on purpose.
- **Keep shared folders and clipboard off** unless you need them, and remember they are a two-way path between the guest and the host.
- **Throwaway credentials only.** Nothing in a lab should reuse a password from your real life, ever.
- **Snapshot before, roll back after**, especially after anything that writes persistence.
- **Cap resources.** Assign CPU and memory limits so one runaway VM does not freeze the host.
- **Document the lab.** A single note with each VM's name, IP, credentials and purpose saves an hour every time you come back to it after a month.
- **Do not install lab tooling on the host** where you can avoid it. Keeping the host clean is what makes "just roll back" possible.

### Part 7: a checklist to build it once

1. Install a hypervisor on the host.
2. Create a **host-only network** with a fixed subnet, e.g. `192.168.56.0/24`.
3. Install the attack machine (Kali or similar), attach **NAT + host-only**, and give it a static host-only address.
4. Verify: `ping` the host-only gateway, then `nmap -sn` the subnet.
5. Start one target on the host-only network (a container or a downloaded VM).
6. Verify reachability from the attack machine, and verify the target **cannot** reach the internet.
7. Take a **base snapshot** of every machine and name it clearly.
8. Write the lab note: names, IPs, credentials, ports, snapshots.
9. Update the attack machine, snapshot again, and never touch the base snapshot.

### Part 8: maintenance and recovery

- **Use templates.** Build a machine once, convert it to a template or clone it, and never install from an ISO again.
- **Prune snapshots.** They consume disk fast; keep the base plus whatever you are working on, and delete the rest.
- **Keep a working snapshot before updating.** Updates break labs; being one click from "before the update" is worth the disk.
- **Rebuild is a feature.** If a lab machine is beyond repair, rebuilding it from notes takes minutes and proves your documentation works.
- **Store the smaller artefacts in version control.** Compose files, notes, scripts and checklists belong in a repository; the multi-gigabyte VM images do not.

### Detection and mitigation

Even a lab is part of a real system, and that has two consequences:

- **Assume your host's security tooling can see into the guests, and plan for it.** EDR agents, telemetry and network monitoring on the host may or may not extend into a VM, and knowing which is which is part of building a lab that behaves predictably. It is also the first lesson in a bigger one: in a real engagement, everything you do on a monitored network leaves traces.
- **Build a defensive lab as well as an offensive one.** The same isolation techniques give you a place to generate logs and test detections: a host running an intentionally vulnerable app, a collector, and a set of rules you are trying to write. A practice environment where you can break something and then find it in the logs teaches both halves at once, which is why the platform projects that build complete lab environments (with a domain, an attacker, and a log pipeline) are so popular.
- **Segment a lab from production the way you would segment anything else.** VLAN, firewall, separate subnet. If the lab exists inside a corporate network, it should be unreachable from it in both directions — not only so your vulnerable machines cannot be attacked, but so that practising does not trigger the monitoring you are also responsible for.
- **Snapshot is your rollback.** The same reasoning applies to production, which is why "take a snapshot before you start remediation" appears in the incident response entry. A lab is where that instinct becomes a habit.
- **Keep the malware rule absolute.** For anything potentially malicious: no network, simulated services instead, snapshots before every run, and a copy of the evidence stored before the machine is rolled back.

<!-- lang:zh -->
### 为什么要搭环境，以及为什么必须隔离

在碰别的东西之前先搭一个，有三个理由：

1. **学习需要把东西搞坏。** 你会反复把自己练习用的机器配错、搞崩、搞感染。这件事必须是免费的。
2. **快照能把失误清零。** "我把它弄坏了"变成"我回滚了，再试一次" —— 而这是这个领域里最大的加速器。
3. **隔离是技术要求，不是礼貌。** 一台故意做成有漏洞的机器放在共享网络上，就是一台你亲手交给"任何会扫网的人"的机器。这在你的家庭网络上成立，在公司网络上更是如此。

所以目标是一个**可抛弃**（快照、回滚）、**隔离**（够不着任何要紧的东西）、**可重建**（照着笔记就能重来）的练习环境。

### 第一部分：隔离的层次

| 层次 | 怎么隔离 | 代价 |
|---|---|---|
| **独立物理机** | 彻底 | 花钱、占地方 |
| **虚拟机** | 硬件虚拟化、独立内核 | 标准选择；有一点性能开销 |
| **容器** | 命名空间与 cgroup —— **共享宿主内核** | 快而轻，但隔离更弱 |
| **云主机** | 别人的硬件、你的网络规则 | 适合卸载负载；花钱，且数据离开你的网络 |

**关键的分界线在虚拟机和容器之间。** 容器逃逸是真实存在的东西，所以不可信的二进制要放进虚拟机；而一个故意有漏洞的 Web 应用跑在容器里没问题，因为风险来自那个应用，不是容器。

### 第二部分：虚拟机

#### 用哪个 hypervisor

| 类型 | 例子 | 说明 |
|---|---|---|
| **Type 1**（裸金属） | ESXi、Proxmox、Hyper-V Server | 直接跑在硬件上；多机实验环境用它 |
| **Type 2**（宿主型） | VirtualBox、VMware Workstation/Fusion、Windows 上的 Hyper-V | 跑在你的操作系统里；最容易起步 |

第一个实验环境用 Type 2 就行。VirtualBox 免费且跨平台；VMware Workstation 更精致；Hyper-V 在 Windows 专业版里就有，但会和其他 hypervisor 冲突。

#### 宿主机需要什么

| 资源 | 现实的底线 | 为什么 |
|---|---|---|
| 内存 | 16 GB | 一台 Windows 虚拟机就能吃掉 4 GB；两三个虚拟机加你的桌面需要余量 |
| CPU | 4 核，且开启虚拟化 | 确认 BIOS/UEFI 里已经**开启**，而不只是"支持" |
| 磁盘 | 100 GB 空闲，最好是 SSD | 虚拟机很大；SSD 才用得下去 |
| 网络 | 随便 | 只用于更新和下载 |

如果 CPU 支持，打开**嵌套虚拟化**；在 Windows 上用 VirtualBox 或 VMware 时若碰到冲突，就关掉 Hyper-V。

#### 快照，以及怎么用

快照记录机器的状态，让你能回到那一刻。值得养成的习惯：

- **每次测试前**：打一个快照，名字就写你接下来要干什么。
- **测试搞乱之后**：回滚，而不是修。修一台被弄坏的实验机学不到任何安全知识，还要花一小时。
- **留一个永远不覆盖的干净"基础"快照**，从它克隆，而不是每次都从 ISO 装。

### 第三部分：网络模式 —— 新手最容易迷路的地方

这部分决定了你的实验环境能不能用，所以值得真正搞懂，而不是一路点"下一步"。

| 模式 | 虚拟机可以到达 | 谁可以到达它 | 用来做什么 |
|---|---|---|---|
| **NAT** | 通过宿主机上互联网 | 外部到不了 | 更新、下载 |
| **桥接** | 宿主机能到达的一切，作为局域网里的一台对等机器 | 能，局域网上任何东西都能 | **实验环境里几乎永远不要用** —— 有漏洞的虚拟机就是这么暴露出去的 |
| **仅主机（Host-only）** | 宿主机，以及同一 host-only 网络上的其他虚拟机 | 该网络上的其他虚拟机 | 隔离的实验网络 |
| **内部（Internal）** | 只有该网络上的其他虚拟机，连宿主机都不到 | 同上 | 完全隔离 |

**对多数新手最合适的布局**，是攻击机上挂两块网卡：

- **网卡 1：NAT** —— 让你能装工具、能更新。
- **网卡 2：仅主机** —— 靶机所在的隔离网络。

而靶机**只挂仅主机**。这样你的靶机只能和你的攻击机通信，既到不了互联网，也无法从你的家庭或办公网络被访问到。

```bash
# 在攻击机（Kali）上，挂好仅主机网卡之后
ip a                                  # 找到网卡名，比如 eth1
sudo ip addr add 192.168.56.10/24 dev eth1
sudo ip link set eth1 up
ping -c2 192.168.56.1                 # 仅主机网关应该有回应
```

### 第四部分：靶机从哪来

| 来源 | 例子 | 说明 |
|---|---|---|
| **在线平台** | Hack The Box、TryHackMe、PortSwigger Academy | 零搭建，但你对基础设施本身学得少 |
| **可下载的虚拟机** | VulnHub 镜像、Metasploitable | 更真实，本地运行 |
| **容器化的应用** | DVWA、OWASP Juice Shop、VAmPI、WebGoat | 几秒就能起，适合当第一个靶机 |
| **自己的代码** | 你自己写一个故意有漏洞的应用 | 最好的老师，因为你知道哪里是错的 |
| **AD 靶场** | GOAD、DetectionLab 那类构建 | 好几台 Windows 加一个域控；很吃内存 |
| **CTF 附件** | 往届题目的文件 | 适合练分析技能 |

**从容器开始。** 几秒钟就能得到一个真实目标：

```bash
docker run -d -p 8080:80 vulnerables/web-dvwa        # DVWA
docker run -d -p 3000:3000 bkimminich/juice-shop     # OWASP Juice Shop
```

```bash
# 给实验环境单独建一个网络，别让容器躺在默认网桥上
docker network create labnet
docker run -d --name dvwa --network labnet -p 127.0.0.1:8080:80 vulnerables/web-dvwa
```

两个关于容器的点值得知道：把端口发布到 `127.0.0.1` 能保证它不暴露到局域网；而容器共享宿主内核，所以"故意危险的应用"没问题，但"不可信的二进制"不行。

### 第五部分：你会遇到的四种布局

**1. 第一个靶机 —— 一台攻击机、一个目标**

```
[ Kali 虚拟机 ] --仅主机--> [ 靶机虚拟机 或 容器 ]
      |
      +-- NAT --> 互联网（只用来更新）
```

**2. 多个目标 —— 一张内部网络**

建一张仅主机网络（`192.168.56.0/24`），把所有虚拟机都挂上去，把它当成"公司内网"。你练习"发现没人告诉你的主机"就在这里：

```bash
nmap -sn 192.168.56.0/24          # 谁活着
```

**3. 域实验环境**

一台域控 + 一到两台成员服务器 + 一台工作站。这是真正练 Active Directory 的唯一办法，也是 16 GB 内存开始显得不够用的地方。**建一次，在"域已起来且干净"的状态下把所有机器快照**，之后都从它克隆。

**4. 恶意样本分析**

这需要的不只是隔离：完全没有网络，或者一张模拟网络，外加不用共享文件夹、不用剪贴板。恶意文档那篇讲了这套流程，那里的规矩更严 —— 样本不能到达任何东西，任何东西也不能到达样本。

### 第六部分：隔离卫生

- **永远不要把有漏洞的虚拟机桥接出去。** 它一旦上了你的局域网，你就等于故意往自己网络里加了一台已被拿下的机器。
- **不需要时就关掉共享文件夹和剪贴板**，并记住它们是客户机与宿主机之间的双向通道。
- **只用一次性凭据。** 实验环境里任何东西都不该复用你现实生活中的口令 —— 永远不要。
- **测前快照、测后回滚**，尤其是在任何写入了持久化的操作之后。
- **限制资源。** 给 CPU 和内存设上限，免得一台失控的虚拟机把宿主机冻住。
- **把实验环境文档化。** 一条笔记记下每台虚拟机的名字、IP、凭据和用途，能让你一个月后回来时省下一小时。
- **能不在宿主机上装实验工具就别装。** 保持宿主机干净，正是"直接回滚"能成立的前提。

### 第七部分：一次搭好的清单

1. 在宿主机上装一个 hypervisor。
2. 建一张**仅主机网络**，固定网段，比如 `192.168.56.0/24`。
3. 装攻击机（Kali 之类），挂 **NAT + 仅主机**，给它一个静态的仅主机地址。
4. 验证：`ping` 仅主机网关，然后 `nmap -sn` 扫一遍该网段。
5. 在仅主机网络上起一个靶机（容器或下载的虚拟机）。
6. 从攻击机验证可达，并验证靶机**到不了**互联网。
7. 给每台机器打一个**基础快照**，命名清楚。
8. 写实验环境笔记：名字、IP、凭据、端口、快照。
9. 更新攻击机、再快照一次，然后永远不要动那个基础快照。

### 第八部分：维护与恢复

- **用模板。** 一台机器装一次，转成模板或克隆，之后就再也不用从 ISO 装了。
- **清理快照。** 它们很吃磁盘；留下基础和当前在用的，其余删掉。
- **更新前留一个可用的快照。** 更新会把实验环境搞坏；离"更新之前"只差一次点击，值那点磁盘。
- **重建是一种能力。** 如果一台实验机已经修不回来，照着笔记重建只要几分钟，而这正好验证了你的文档有没有用。
- **小的东西进版本控制。** compose 文件、笔记、脚本、清单都该进仓库；几个 GB 的虚拟机镜像不该。

### 检测与缓解

即使是一个实验环境，它也是真实系统的一部分，由此有两个后果：

- **假设宿主机的安全工具能看见客户机，并据此规划。** 宿主机上的 EDR、遥测和网络监控可能延伸进虚拟机、也可能不 —— 弄清哪些能、哪些不能，是搭出一个"行为可预期"的实验环境的一部分。它也是更大一课的开头：在真实项目里，你在被监控网络上做的每一件事都会留下痕迹。
- **除了攻击环境，也搭一个防守环境。** 同一套隔离技术可以给你一个生成日志、测试检测规则的地方：一台跑着故意有漏洞应用的主机、一个日志收集端，加上你正在尝试写的规则。一个能让你"搞坏东西、再在日志里找到它"的练习环境，能同时教你攻防两边 —— 这也是那些构建完整实验环境（带域、攻击机和日志管道）的开源项目为什么这么受欢迎。
- **像隔离任何东西那样，把实验环境与生产隔开。** VLAN、防火墙、独立网段。如果实验环境存在于公司网络内部，它应该在两个方向上都不可达 —— 不只为了让你的靶机不被打，也为了让你练习的动作不会触发你自己也负责的那套监控。
- **快照就是你的回滚。** 同样的道理适用于生产，这也是为什么事件响应那篇里写着"开始处置之前先做快照"。实验环境就是让这个本能变成习惯的地方。
- **恶意样本的规矩不留余地。** 对任何可能恶意的东西：断网、用模拟服务替代、每次运行前快照、并且在回滚之前把证据留一份副本。
