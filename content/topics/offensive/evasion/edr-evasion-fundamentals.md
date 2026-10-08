---
id: edr-evasion-fundamentals
title_en: EDR Evasion Fundamentals
title_zh: EDR 规避基础
summary_en: Evasion is not a trick you apply once, it is a conversation with a set of sensors. Understanding what each sensor sees — userland hooks, ETW, kernel callbacks, AMSI, memory scanning — is what tells you which technique is worth trying and, more importantly, what will still be caught.
summary_zh: 免杀不是"套一个技巧"，而是与一整套传感器对话。搞清楚每个传感器在看什么 —— 用户态 hook、ETW、内核回调、AMSI、内存扫描 —— 才能判断哪种手法值得试，更重要的是，哪种一定还会被抓。
tags: [evasion, edr, windows, red-team, amsi, etw, detection]
tools: [Sysmon, Process Monitor, API Monitor, WinDbg]
attck: [T1562.001, T1055, T1027]
platform: [windows]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### How EDR actually decides

"EDR" is a bundle of sensors, not one. Each one has blind spots, and each evasion technique answers exactly one of them. Getting past antivirus and getting past an EDR are different problems, which is why signatures and hashes matter far less than they used to.

| Sensor | What it sees | Typical blind spot |
|---|---|---|
| Userland hooks in `ntdll.dll` | API calls from the process, with arguments, before they reach the kernel | Direct or indirect syscalls skip the hooked stub entirely |
| AMSI | Script and macro content, before the engine compiles it | Native code, or a process where AMSI is not loaded |
| ETW providers (`Microsoft-Windows-Threat-Intelligence`) | Process, thread, image load, memory allocation events | Userland patching of `EtwEventWrite` affects the provider, not the consumer |
| Kernel callbacks (`PsSetCreateProcessNotifyRoutine`, `ObRegisterCallbacks`) | Process creation, handle operations — from inside the kernel | Nothing userland can unhook; you have to avoid the behaviour |
| Memory scanning | Regions with executable + writable permissions, known tooling byte patterns, unbacked executable memory | Well-executed in-memory execution that never looks like known tooling |
| Behavioural correlation | Sequences across processes and time: Office spawning PowerShell spawning whoami | Anything that breaks the expected parent/child and timing pattern |

The last row is the one people underestimate. Modern detection rarely fires on a single API call — it fires on a *chain* that does not make sense for that host.

### The categories, and what each one answers

**1. Execution without touching disk.** A file that is never written cannot be hashed or quarantined at rest. This is why reflective loading, in-memory .NET assemblies and process injection exist. It answers the *file* sensor and creates new problems: unbacked executable memory is itself a signal on well-tuned hosts.

**2. AMSI.** The Antimalware Scan Interface is an in-process COM interface that scripting engines (PowerShell, JScript, VBA, .NET) call before executing content. Anything that neutralises it — patching the entry point, forcing an error return, disabling the provider — is a known technique with well-known telemetry. Assume that if you touch `amsi.dll` in a process, that fact is recorded somewhere, even if your patch succeeds.

**3. ETW.** Event Tracing for Windows is the OS's own telemetry bus. Patching the userland `EtwEventWrite` function stops a process from emitting its own events — but `.NET` runtime events, kernel-side ETW-TI, and the fact that a *process stopped reporting* are themselves detectable. Treat ETW tampering as a loud action, not a quiet one.

**4. Unhooking.** EDR userland hooks are usually written at the start of `ntdll` functions. Restoring the original bytes from a fresh copy on disk, or calling the kernel directly, both work — and both have signatures. The relevant distinction in practice is *direct* syscalls (your own `syscall` instruction) versus *indirect* syscalls (jumping to a `syscall` instruction inside `ntdll`), because the call stack of the former looks unnatural to any stack-inspecting sensor.

**5. Sleep.** Malware sleeps, and while it sleeps its memory is a sitting target for scanning. Encrypting your own memory before sleeping and decrypting after is a legitimate countermeasure with a clear tell: a thread with `PAGE_EXECUTE_READWRITE` memory that changes between sleeps, and calls to `VirtualProtect` on a timer. This is an arms race, not a solution.

**6. Injection.** Every injection primitive has a detectable shape: `CreateRemoteThread` (thread created in a process that did not ask for one), APC injection (queued APC in a process with no reason to have one), process hollowing (image mismatch between the section and the file), thread hijacking (an existing thread's context suddenly rewritten). Choosing a primitive is choosing which signal you are willing to leave.

**7. Living off the land.** `certutil`, `bitsadmin`, `mshta`, `rundll32`, `wmic`, `regsvr32` — signed Microsoft binaries that fetch or execute. The binary is trusted; the *command line*, the *parent process* and the *network destination* are not. Defenders have spent years building rules for exactly these.

### What actually gets caught, and why

| Failure mode | Why it happens |
|---|---|
| The tool is known | Byte patterns in memory, not just on disk; a well-known loader is a well-known loader |
| The parent is wrong | `winword.exe → powershell.exe → whoami.exe` is a chain, not an event |
| The timing is wrong | A process that runs for 200 ms, or an implant that beacons with zero jitter |
| The memory is wrong | RWX regions, unbacked executable pages, thread start addresses outside any module |
| The telemetry is wrong | The host reports nothing for a process that normally reports a lot |
| The network is wrong | Newly registered domain, no reputation, TLS fingerprint of a known framework |

The practical conclusion: **evasion is a budget, not a state.** You spend it on the steps that matter (the first execution, the credential access) and accept that later steps are noisier. A red team engagement succeeds when the objective is reached and the detection story is documented, not when the implant is immortal.

And the boundary worth stating plainly: this knowledge is for authorised testing and detection engineering. Deploying it against systems you do not have written permission to test is a crime in most jurisdictions, and it is not what this guide is for.

### Detection — the part that actually decides the engagement

Every technique above has a telemetry counterpart, and the defenders who win are the ones who correlate rather than block:

- **Hooks are visible from inside.** Compare the first bytes of `ntdll` functions in memory against the on-disk copy; a mismatch in a process that has no reason to be modifying them is a finding.
- **ETW tampering.** Alert when a known ETW function is patched, but also when a process that normally emits events stops or when its event volume drops to zero.
- **AMSI.** A failed or bypassed AMSI scan is often logged as a provider error; monitor for `amsi.dll` being unloaded or its entry point modified.
- **Memory.** Alert on executable memory that is not backed by a file on disk in processes that should not have any, and on `PAGE_EXECUTE_READWRITE` outside the normal set of processes.
- **Chains, not events.** Sigma rules and EDR analytics for parent/child relationships (`winword.exe → cmd.exe`), unusual command lines (`-enc`, `-nop`, `-w hidden`), and LOLBin network activity.
- **Network reputation.** Domain age, first-seen, TLS fingerprint, and beacon regularity (a fixed interval with fixed jitter is a pattern, not noise).

### Mitigation

- **Attack Surface Reduction rules** block the common chains (Office spawning child processes, script executing downloaded content) rather than individual tools.
- **WDAC / AppLocker** with a default-deny policy; signed-only execution removes most of the execution category at once.
- **Enable and centralise the telemetry** you already pay for: ETW-TI, PowerShell script block logging, Sysmon with a reviewed config, and AMSI provider logging.
- **Credential protection**: Credential Guard, LSA protection (`RunAsPPL`), and no cached domain credentials on workstations that do not need them.
- **Restrict LOLBins** you do not use, and alert on the ones you keep.
- **Patch, but do not rely on patching**: most evasion is not a vulnerability, it is a capability. The control is the sensor coverage and the allow-list.
- **Assume breach**: the question is whether the chain is *detected*, not whether the first step is blocked.

<!-- lang:zh -->
### EDR 到底是怎么判定的

"EDR" 是一堆传感器的集合，不是单一技术。每个都有盲区，而每一种规避手法恰好只针对其中一个。绕过杀软和绕过 EDR 是两个不同的问题 —— 这也是为什么特征码和哈希如今远不如以前重要。

| 传感器 | 它看到什么 | 典型盲区 |
|---|---|---|
| `ntdll.dll` 里的用户态 hook | 进程发出的 API 调用及其参数，在到达内核之前 | 直接或间接系统调用完全跳过被 hook 的桩 |
| AMSI | 脚本与宏的内容，在引擎编译之前 | 原生代码，或者没加载 AMSI 的进程 |
| ETW 提供程序（`Microsoft-Windows-Threat-Intelligence`） | 进程、线程、镜像加载、内存分配事件 | 用户态 patch `EtwEventWrite` 影响的是提供方，不是消费方 |
| 内核回调（`PsSetCreateProcessNotifyRoutine`、`ObRegisterCallbacks`） | 进程创建、句柄操作 —— 从内核里看 | 用户态没法脱钩；你只能不去做那个行为 |
| 内存扫描 | 可执行+可写内存、已知工具的字节特征、无文件支撑的可执行内存 | 执行得体、且看起来不像已知工具的纯内存操作 |
| 行为关联 | 跨进程、跨时间的序列：Office 拉起 PowerShell 再拉起 whoami | 任何打破正常父子链和时间模式的行为 |

最后一行是大家最容易低估的。现代检测很少因为单次 API 调用报警 —— 它报警是因为出现了**这条主机上不该有的链条**。

### 各类手法，以及各自回答的是哪个传感器

**1. 不落地执行。** 从没写进磁盘的文件无法被静态哈希或隔离。这就是反射加载、内存中的 .NET 程序集、进程注入存在的原因。它回答了**文件**这个传感器，同时制造了新问题：没有文件支撑的可执行内存在调优良好的主机上本身就是信号。

**2. AMSI。** 反恶意软件扫描接口是一个进程内 COM 接口，脚本引擎（PowerShell、JScript、VBA、.NET）在执行内容前会调用它。任何让它失效的做法 —— patch 入口点、强制返回错误、禁用提供程序 —— 都是已知手法，对应已知遥测。要假设：只要你在某个进程里碰了 `amsi.dll`，即便 patch 成功，这件事也被记在某个地方。

**3. ETW。** Event Tracing for Windows 是操作系统自己的遥测总线。patch 用户态的 `EtwEventWrite` 能让进程不再发出自己的事件 —— 但 .NET 运行时事件、内核侧的 ETW-TI，以及"一个平时报很多事件的进程突然不报了"这件事本身，都是可检测的。把篡改 ETW 当作高调动作，而不是隐蔽动作。

**4. Unhooking。** EDR 的用户态 hook 通常写在 `ntdll` 函数开头。从磁盘上的干净副本恢复原始字节，或者直接调内核，都能绕过 —— 也都有特征。实战中真正要紧的区分是**直接**系统调用（自己执行 `syscall` 指令）与**间接**系统调用（跳到 `ntdll` 内部的 `syscall` 指令），因为前者的调用栈在任何检查栈的传感器看来都不自然。

**5. Sleep。** 恶意代码要睡觉，而睡觉时它的内存就是扫描的靶子。睡前加密自己的内存、醒后解密，是合理的对抗手段，但有一个明确的破绽：线程里存在 `PAGE_EXECUTE_READWRITE` 内存并在每次睡眠之间变化，以及按定时器调用 `VirtualProtect`。这是军备竞赛，不是解决方案。

**6. 注入。** 每一种注入原语都有可检测的形状：`CreateRemoteThread`（在没请求过的进程里创建线程）、APC 注入（往本不该有 APC 的进程里排队）、进程镂空（内存节区与文件不匹配）、线程劫持（已有线程的上下文突然被改写）。选原语，就是在选你愿意留下哪一种信号。

**7. 就地取材（LOLBins）。** `certutil`、`bitsadmin`、`mshta`、`rundll32`、`wmic`、`regsvr32` —— 这些是微软签名的二进制。二进制本身受信任；**命令行**、**父进程**和**网络目标**不受信任。防守方在这些东西上积累多年的规则了。

### 实际会被抓的地方，以及原因

| 失败模式 | 为什么会发生 |
|---|---|
| 工具是已知的 | 内存里的字节特征，不只是磁盘上的；知名加载器就是知名加载器 |
| 父进程不对 | `winword.exe → powershell.exe → whoami.exe` 是一条链，不是一个事件 |
| 时间不对 | 只运行 200 毫秒的进程，或者零 jitter 心跳的植入体 |
| 内存不对 | RWX 区域、无文件支撑的可执行页、线程起始地址不在任何模块内 |
| 遥测不对 | 一个平时报大量事件的进程，在这台主机上什么都没报 |
| 网络不对 | 新注册域名、无信誉、已知框架的 TLS 指纹 |

由此得出的实际结论：**免杀是预算，不是状态。** 你把预算花在真正重要的步骤上（第一次执行、拿凭据），并接受后面的步骤更吵。红队项目成功的标准是达成目标并把检测故事记录清楚，而不是植入体永生。

有一点边界值得直说：这些知识用于**授权测试和检测工程**。把它用在没有书面授权测试的系统上是犯罪，也不是这份指南的目的。

### 检测 —— 真正决定项目成败的部分

上面每一种手法都有对应的遥测，而赢的防守方做的是关联，不是拦截：

- **hook 在进程内部可见。** 把内存里 `ntdll` 函数的前几个字节和磁盘上的副本对比；在一个没有理由修改它们的进程里发现不一致，就是发现。
- **ETW 篡改。** 既要在已知 ETW 函数被 patch 时告警，也要在"平时会发事件的进程突然不发、事件量掉到零"时告警。
- **AMSI。** AMSI 扫描失败或被绕过常常以提供程序错误的形式被记录；监控 `amsi.dll` 被卸载或其入口点被修改。
- **内存。** 对"本不该有可执行内存的进程里出现无文件支撑的可执行页"以及正常范围之外的 `PAGE_EXECUTE_READWRITE` 告警。
- **看链，不看事件。** 用 Sigma 规则和 EDR 分析盯父子关系（`winword.exe → cmd.exe`）、可疑命令行（`-enc`、`-nop`、`-w hidden`）以及 LOLBin 的网络行为。
- **网络信誉。** 域名年龄、首次出现时间、TLS 指纹，以及心跳规律性（固定间隔配固定抖动是模式，不是噪声）。

### 缓解

- **ASR 规则**拦的是常见链条（Office 拉起子进程、脚本执行下载内容），而不是单个工具。
- **WDAC / AppLocker** 配默认拒绝策略；只允许签名执行，一次性消掉大部分"执行"类手法。
- **把你已经付过钱却没开的遥测打开并集中**：ETW-TI、PowerShell 脚本块日志、经过审阅配置的 Sysmon、AMSI 提供程序日志。
- **凭据保护**：Credential Guard、LSA 保护（`RunAsPPL`），以及在不必要的工作站上不缓存域凭据。
- **限制用不到的 LOLBins**，对保留的做告警。
- **打补丁，但不要指望补丁**：大多数免杀不是漏洞，而是能力。控制点在传感器覆盖和允许清单。
- **假设已被突破**：问题在于这条链是否**被检测到**，而不在第一步是否被拦住。
