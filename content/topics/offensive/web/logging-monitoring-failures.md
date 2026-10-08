---
id: logging-monitoring-failures
title_en: Logging and Monitoring Failures
title_zh: 日志与监控失效
summary_en: This is the defender-side failure, which is exactly why attackers care about it. When nothing is recorded, nothing is noticed and nothing is answered, every other technique in this guide becomes quieter and last longer.
summary_zh: 这是防守方的失效 —— 也正因如此，攻击者才在意它。当没有记录、没有发现、没有响应时，本指南里所有其他手法都会变得更安静、也活得更久。
tags: [web, owasp-a09, logging, monitoring, detection, purple-team]
tools: [Sysmon, auditd, Sigma, Elastic, Splunk]
attck: [T1562.008, T1070]
platform: [web, network]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why an attacker studies detection

Three questions decide how a real intrusion goes, and none of them is about exploiting anything:

1. **Is it recorded?** If the action leaves no log, it can be repeated indefinitely.
2. **Is it noticed?** A log nobody reads is a write-only archive.
3. **Is it answered?** A detection with no response process is a notification, not a control.

The gap between the three is where dwell time lives. Most breaches are discovered by a third party — a customer, a bank, a journalist — which is a statement about monitoring rather than about the initial vulnerability.

For a red team, evaluating these three is a deliverable in itself. "We obtained domain admin" is one finding; "we obtained domain admin and you had 14 detections you did not act on" is a much more useful one.

### Failure 1 — events that are not recorded

The standard list of things worth logging, and the ones most often missing:

- Authentication: successes, failures, logouts, MFA challenges, password resets.
- Authorization: access denied, privilege changes, role assignments.
- Input validation failures, which are the earliest sign of an attack.
- Administrative actions: user creation, configuration change, key rotation.
- Application errors, especially the ones that indicate tampering.
- Data access to sensitive records (who read the medical file).

An application that logs only errors and not authorization decisions is blind to the most common class of attack in this guide, because a successful IDOR produces a perfectly ordinary 200 response.

### Failure 2 — logs that leak

The reverse problem, and an attacker's shortcut:

- Passwords, tokens or session identifiers written into log lines.
- Full request bodies captured by a debugging middleware left enabled.
- Stack traces with connection strings shown to users or shipped to a third-party error service.
- Log files readable by more people than they should be, or served from the web root.

A log is a copy of sensitive data. It needs the same classification and retention rules as the database it describes.

### Failure 3 — logs that can be removed

- **Local files only**, on the host being attacked, with permissions that the compromised account can write to.
- **No central collection**, so deleting the file deletes the evidence.
- **No integrity protection**: a central store that accepts deletion requests from the same agents that write to it is not a record of anything.
- **Short retention**, so an intrusion discovered three weeks later has no data left.

### Failure 4 — monitoring that does not exist

Collecting is not monitoring. The signs that monitoring is nominal:

- Alerts on infrastructure health but nothing on security events.
- Rules that fire so often they are muted, or so rarely they have never been tested.
- No correlation: the brute force, the successful login from a new country, and the new API key are three separate events and one story.
- No baseline: without knowing what normal looks like, "unusual" cannot be computed.

### Failure 5 — logging as an attack surface

Two techniques worth knowing from the offensive side, because they are cheap and effective:

- **Log injection.** CRLF in a logged value can forge additional log entries, which is useful both for hiding an action and for framing another account. Encode line breaks in anything that reaches a log.
- **Log flooding.** Enough noise to push the interesting entry out of the retention window, or to exhaust storage. A deny-of-logging is a denial of detection.

### Testing the detection, with authorisation

This is purple-team work and it needs to be agreed in advance: the point is to measure the monitoring, not to break in.

1. Perform a small, obviously suspicious action in a controlled window — an impossible-travel login, or a known-bad user agent.
2. Ask whether it appears in the logs, in the SIEM, and in an alert.
3. Measure the time from action to alert, and whether anyone responds.
4. Repeat with a subtler action, and see whether it is caught at all.

The output is a detection matrix, which is a much more valuable deliverable than a list of vulnerabilities, because it tells the client what their monitoring actually sees.

### Detection, from the defender's side

- **Log every authentication and authorization decision**, including the denials, and include enough context to reconstruct the story (who, what, from where, with what result).
- **Send logs somewhere the application account cannot reach** — a central collector with append-only permissions, and ideally immutability for the retention period.
- **Alert on sequences, not events**: failed logins followed by a success, a privilege change followed by bulk data access, a new access key followed by calls from a new IP.
- **Test your own alerts.** A rule that has never fired in a drill is a hypothesis, and the drill is cheap.
- **Sanitise logs on write** (strip or encode newlines), classify what may be logged, and never log credentials or tokens.
- **Monitor the monitoring**: if an agent stops reporting, if event volume drops to zero for a host, or if retention is being truncated, that is itself an incident.
- **Keep retention long enough to be useful.** Ninety days of searchable data answers most questions about an intrusion; thirty days of hot logs plus cold storage is a reasonable shape.

### Mitigation

- **Log enough, and make the decision per event class rather than per line of code.** The OWASP logging guidance is a reasonable starting point; the important part is that authorization denials and administrative actions are always included.
- **Centralise, protect and retain**, with the collector as a Tier 0 system reached by the minimum number of identities.
- **Build detections from the technique list in this guide** — each entry's detection section is a rule candidate.
- **Practise response, not just detection**, and rehearse the path from alert to decision. The failure mode at that stage is usually organisational, not technical.
- **Treat detection coverage as a first-class asset metric**, alongside patch coverage and inventory completeness.

<!-- lang:zh -->
### 攻击者为什么研究检测能力

三个问题决定一次真实入侵的走向，而它们全都与"怎么打进去"无关：

1. **记录了吗？** 如果动作不留日志，它就能被无限重复。
2. **发现了吗？** 没人看的日志只是一个只能写、不能读的归档。
3. **响应了吗？** 没有处置流程的检测只是通知，不是控制。

这三者之间的落差，就是"驻留时间"所在的地方。大多数入侵是被第三方发现的 —— 客户、银行、记者 —— 这句话说的其实是监控，而不是最初那个漏洞。

对红队来说，评估这三件事本身就是交付物。"我们拿到了域管"是一个发现；"我们拿到了域管，而整个过程你们有 14 次检测但没有处置"是一个有用得多的发现。

### 失效一 —— 根本没记录的事件

值得记录的东西有标准清单，而最常缺的就是这些：

- 认证：成功、失败、登出、MFA 挑战、口令重置。
- 授权：拒绝访问、权限变更、角色分配。
- 输入校验失败 —— 这是攻击最早期的信号。
- 管理动作：创建用户、变更配置、轮换密钥。
- 应用报错，尤其是那些暗示被篡改的。
- 敏感记录的数据访问（谁读了那份病历）。

一个只记错误、不记授权决定的应用，对本指南里最常见的那类攻击是完全盲的 —— 因为一次成功的 IDOR 产生的是一个再普通不过的 200 响应。

### 失效二 —— 日志本身在泄漏

反过来的问题，也是攻击者的捷径：

- 明文口令、token 或会话标识被写进日志行。
- 遗留的调试中间件把完整请求体都记了下来。
- 带连接串的堆栈被展示给用户，或发给了三方错误收集服务。
- 日志文件的可见范围过大，甚至就放在 Web 根目录下被直接下载。

日志是敏感数据的一份副本。它需要和它所描述的那个数据库同等的数据分级与保留规则。

### 失效三 —— 可以被删掉的日志

- **只存本地文件**，就存在被攻击的主机上，权限还允许那个被拿下的账号写入。
- **没有集中收集**，删掉文件就等于删掉证据。
- **没有完整性保护**：一个允许写入方直接发起删除的集中存储，不构成任何记录。
- **保留期太短**，三周后才发现的入侵已经没有数据可查。

### 失效四 —— 名存实亡的监控

收集不等于监控。监控只是摆设的迹象：

- 有基础设施健康告警，没有任何安全事件告警。
- 规则要么响得太频繁被静音，要么从没触发过。
- 没有关联分析：爆破、来自新国家的成功登录、新建的 API key，是三个独立事件，也是同一个故事。
- 没有基线：不知道正常长什么样，"异常"就算不出来。

### 失效五 —— 日志本身成为攻击面

从攻击侧看有两个便宜又好用的手法：

- **日志注入。** 被记录的值里带 CRLF 就能伪造额外日志条目，既可用来藏动作，也可用来栽赃别的账号。任何进入日志的内容都要编码换行符。
- **日志洪水。** 制造足够多的噪音，把关键那条挤出保留窗口，或者直接耗尽存储。拒绝记录就是拒绝检测。

### 在授权下测试检测能力

这是紫队工作，必须事先约定：目的是**测量监控**，不是闯进去。

1. 在受控时间窗内做一个小的、明显可疑的动作 —— 不可能旅行式的登录，或一个已知恶意的 user agent。
2. 问：它出现在日志里了吗？出现在 SIEM 里了吗？触发告警了吗？
3. 测量从动作到告警的时间，以及有没有人响应。
4. 换一个更隐蔽的动作重复，看是否完全没被捕捉。

产出是一张检测矩阵 —— 它比一份漏洞清单有价值得多，因为它告诉客户"你们的监控实际能看到什么"。

### 检测（防守方视角）

- **记录每一次认证与授权决定**，包括拒绝的那些，并带上足以还原故事的上下文（谁、做了什么、从哪来、结果如何）。
- **把日志送到应用账号够不着的地方** —— 集中收集器、只追加权限，最好在保留期内不可变。
- **对序列告警，而不是对事件告警**：连续登录失败之后的一次成功、权限变更之后的批量数据访问、新建 access key 之后来自新 IP 的调用。
- **测试你自己的告警。** 从没在演练中触发过的规则只是假设，而演练很便宜。
- **写入时就净化日志**（去掉或编码换行），定义什么可以记，永远不要记录凭据或 token。
- **监控监控本身**：采集 agent 停止上报、某台主机的事件量掉到零、保留期被截断 —— 这些本身就是事件。
- **保留期要够用。** 九十天可检索的数据能回答关于入侵的绝大多数问题；三十天热日志加冷存储是一个合理的形状。

### 缓解

- **记够，但不是按代码行、而是按事件类别来决定。** OWASP 的日志指南是合理的起点；要紧的是授权拒绝和管理动作必须包含在内。
- **集中、保护、保留**，把收集器当作 Tier 0 系统，只允许尽可能少的身份访问。
- **按本指南的手法清单来建检测** —— 每一篇的检测小节都是一条规则候选。
- **不只演练检测，还要演练响应**，把从告警到决策的路径走一遍。这个阶段的失效通常是组织问题，不是技术问题。
- **把检测覆盖率当成一等资产指标**，与补丁覆盖率、资产台账完整度并列。
