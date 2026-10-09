---
id: cloud-network-and-exposure
title_en: Cloud Network and Exposure
title_zh: 云网络与暴露面
summary_en: Reachability is a conjunction of layers that are not equally capable — a route that decides whether there is a path, a stateless list that judges each direction separately, a stateful list that only allows, and a service that has to answer. Measured by evaluating the layers — a private subnet unreachable with an open security group, an inbound rule that cannot complete a connection, and a narrow rule that stops mattering the moment a broad one exists.
summary_zh: 可达性是几层判断的合取，而那几层能力并不相同 —— 路由决定有没有路、无状态的列表对每个方向各算各的、有状态的列表只允许、以及一个必须应答的服务。这一篇把这几层跑出来实测 —— 一个安全组全开而外面到不了的私有子网、一条完成不了连接的入站规则，以及一条在宽规则存在的瞬间就失去意义的窄规则。
tags: [beginner, cloud, network, exposure, security-groups]
tools: [aws, gcloud, az, nmap]
attck: [T1046, T1133]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Reachability is several judgements, not one

A firewall rule that says allow is not the same as a service that can be reached, and on a cloud network the difference is spread across four layers with different capabilities.

> **A route decides whether there is a path at all. A stateless list judges each direction separately. A stateful list only allows. And a service has to answer.**

The third layer is where the model differs from a host firewall, and the second is where most of the operational confusion comes from. Both are measured below by evaluating the layers.

### Part 1: the shape of it

| Layer | Attached to | Can it deny | Remembers connections |
|---|---|---|---|
| routing table | a subnet | no — it says where traffic goes | no |
| network access list | a subnet | **yes** | **no — stateless** |
| security group | an interface | no — **allow only** | **yes — stateful** |
| the host | the instance | yes | depends |
| the service | the process | — | — |

**Two of those rows are the whole entry.** A list that only allows cannot be used to narrow anything except by changing or removing what is there, and a list that does not remember connections has to be given a rule for the return traffic as well as the request.

**And the layer order matters for reading a configuration.** A rule that permits a port is the third of five questions, and the answer to the first one can make the rest irrelevant.

### Part 2: a route comes before any permission

Measured, two subnets with identical security groups, identical lists and a service listening on the same port:

| Subnet | Route table | Outcome |
|---|---|---|
| the one with a default route to a gateway | a route to the internet | **reachable** |
| **the one with only the local route** | no route outward | **not reachable — the subnet has no path to the internet, so nothing outside can initiate** |

**The security group allowed the port in both cases**, and in one of them that permission could not be exercised by anyone outside.

> **"The security group allows 443" is not "it is reachable from outside". Before permission there has to be a path.**

**Which means an exposure review starts at the routing tables rather than at the security groups**, and an instance in a private subnet with a wide-open group is a finding of a different kind from an instance in a public subnet with one — **the second is an exposure, the first is a latent one that becomes an exposure the moment a route appears.**

**And reachability from outside needs three things together**: a route to a gateway, an address that the outside can be pointed at, and a permission. **Any one missing makes the other two harmless**, which is why a change to any of the three deserves the same attention.

### Part 3: one remembers connections and one does not

This is the measurement that explains most "it should work" reports. Measured, an inbound port allowed on 443, with the outbound side configured the same way in one case and not in the other:

| The stateless list | Outcome |
|---|---|
| allows the request in and the response out | **reachable** |
| **allows the request in only** | **cannot connect — the outbound list has no rule for the return traffic** |

**The inbound rule was correct, the security group was correct, the service was listening, and the connection did not establish.** The reason is in the return path: the response from the server to the client has a destination port that is the client's source port, which is a high-numbered port, and the stateless list judges that packet as a separate decision.

> **The difference between the two layers is not how well the rules are written. It is who is responsible for the return traffic.**

**A stateful list accepts a connection and then permits everything belonging to it**, so one inbound rule is enough and no outbound rule is needed. **A stateless list sees each packet with no memory of the request it belongs to**, so both directions need a rule and the return rule has to cover the client's port range.

**Which produces a characteristic symptom**: a list whose return rule names a narrow port range **works for some clients and not others**, depending on which source port they happened to use — an intermittent failure that is very hard to read from a configuration, and easy to explain once the layer is understood.

### Part 4: only one of them can refuse

**Measured, a list with a denial at a lower number and a permission at a higher one blocked the traffic, while the same two rules in the opposite order allowed it.** That is what makes the numbering matter: **the rules are read in order and the first match decides.**

**And the security group has no denial at all.** Measured, a group containing both a rule for one source range and a rule for everything:

| Rules in the group | A request from outside the narrow range |
|---|---|
| `0.0.0.0/0` on 443 **and** `203.0.113.0/24` on 443 | **allowed — the broad rule matched** |

**Adding a narrower rule changed nothing**, because the rules are a union and the wider one was already there.

> **In an allow-only model, narrowing can only mean removing or changing a rule. Adding a more specific one narrows nothing.**

**Which is the mechanism behind a very common experience**: a restriction is added, nothing changes, and the conclusion drawn is that the rule did not take effect. **It did take effect; it just cannot subtract.**

**And the two layers are therefore used differently.** The allow-only one is where the precise permissions live, and the one that can deny is where coarse boundaries live — **a source range that must never be reachable, or a protocol that is never used**, expressed once at the subnet rather than per interface.

### Part 5: reachable and answering are two questions

Measured, with every rule permitting the port and nothing listening on it:

| Rules | Listening | Outcome |
|---|---|---|
| all permitting | yes | reachable |
| all permitting | **no** | **not reachable — nothing is answering on that port** |

**A scanner sees a refused connection, and a configuration reader sees an open port.** Both are describing the same state.

> **There are three different questions about one port: do the rules allow it, is something listening, and can a handshake complete. Only the first can be read out of a configuration.**

**Which is the same conclusion the entries on cloud identity and object storage reached from the other direction**: a rule that permits and a result that happens are two facts, and the only reliable way to learn the second is to make a request. **For exposure specifically, that means probing from outside rather than listing the rules.**

### Part 6: exposure is a three-part fact

Measured, one rule and two sources:

| Rule | Source | Outcome |
|---|---|---|
| `0.0.0.0/0` on 443 | any | allowed |
| `203.0.113.0/24` on 443 | inside that range | allowed |
| `203.0.113.0/24` on 443 | **outside that range** | **refused** |

**The same port is exposed and not exposed depending on who is asking.**

> **Exposure is a protocol, a port and a source. A port number on its own says nothing.**

**And there is usually more than one way in.** A peering connection, a transit gateway, a load balancer in front, a distribution that reaches an origin, a tunnel from an office — **each is a path that the rule's source range may or may not have been written with in mind**, and each is another place where the measured distinction between "the rule allows" and "somebody can reach it" has to be checked.

### Part 7: outbound, and what it decides after a compromise

**Inbound rules decide who can get in. Outbound rules decide what a compromised workload can do**, and they are usually configured far more loosely — often with a default that permits everything.

**And the difference is measurable in the same three layers.** A subnet with a route to a gateway and a default outbound permission **can reach any address on the internet**, which means data does not need a tunnel to leave: an ordinary outbound request carries it.

**Which makes outbound restriction the control that survives the failure of others.** The entries on request forgery and on instance metadata both end at the same place: **the destination the attacker names is reached because the network allowed it**, and an allow-list of destinations is the one control that does not depend on the application getting its input handling right.

**And the internal destinations are part of it.** A workload that can reach the instance metadata address, the internal DNS, or a database in another subnet is a workload whose outbound reach is part of its attack surface — **and the metadata entry measured that the address in question has several spellings**, so the restriction belongs after resolution rather than in a list of strings.

### Part 8: what follows for security

**A rule that permits is one of several conditions, and it is not the first one.** Measured, a private subnet with an open group was unreachable and a public one with the same group was not. **So the exposure inventory starts at the routing table**, and a change that adds a route deserves the same review as a change that adds a rule.

**The stateless and stateful distinction is the one that produces unexplainable reports.** Measured, an inbound rule that was correct in every layer still could not complete a connection because the return traffic had no rule. **The way to read a configuration that "should work" is to follow one packet in each direction separately**, which is exactly what the layer without memory does.

**And the allow-only layer cannot narrow.** Measured, adding a narrow rule to a group that already permitted everything changed nothing. **So "we restricted it" is a claim that has to be checked by looking for the rule that still permits it** — and the check is a search for the widest rule rather than for the newest one.

**The three questions about a port are worth keeping separate in any report.** Allowed, listening, reachable — **measured here as three states of the same port**, and only the first is a configuration fact. **The inventory that matters is the one produced by probing**, with the configuration used to explain the result rather than to predict it.

**And the outbound direction is where the remaining control lives.** An application that has been made to fetch a supplied address, a workload running with a stolen identity, a function with a wide role — **all of them end at a destination, and the network is the only layer that can refuse it regardless of how they got there.**

### Detection and mitigation

- **Review routing tables as exposure**, since a route is what makes a permission reachable and a new route changes the meaning of every group behind it.
- **Report inbound rules that permit everything on a port, together with the source range**, because the measured exposure depends on both and the port alone says nothing.
- **Report a narrow rule that coexists with a broad one**, since the narrow one cannot subtract and is often mistaken for a restriction.
- **Alert on flow records showing connections that no rule seems to permit**, and on permitted rules with no traffic, since the two together are the gap between the configuration and the reality.
- **Check the return-traffic rules on every stateless list**, and watch for the intermittent failures that a narrow port range there produces.
- **Enumerate every path in** — peering, transit, load balancers, distributions, tunnels — and check the source ranges each one was written for.
- **For mitigation, put precise permissions in the stateful layer and coarse refusals in the layer that can deny**, since only one of them can express "never".
- **Restrict outbound to the destinations the workload uses**, which is the control that holds after the application has been made to misbehave.
- **Validate destinations after resolution**, because the addresses that matter have more than one spelling.
- **Make the exposure inventory from outside.** Rules say what is permitted; a connection says what is reachable, and the measurement showed a port in all three states.
- **And treat a change to a route, an address or a permission as one class of change**, because reachability needs all three and a review that watches only the rules has watched one of them.

<!-- lang:zh -->
### 可达性是好几层判断，不是一层

一条写着"允许"的防火墙规则，与一个能被连到的服务不是同一件事；而在云网络上，这个差别摊在四层上，而它们的能力并不相同。

> **路由决定到底有没有一条路。无状态的列表对每个方向各算各的。有状态的列表只允许。而服务必须应答。**

第三层才是这个模型与主机防火墙不同的地方，第二层才是运维上大多数困惑的来源。两者都在下面跑出来实测。

### 第一部分：它的形状

| 层 | 挂在哪里 | 能不能拒绝 | 记不记得连接 |
|---|---|---|---|
| 路由表 | 一个子网 | 不能 —— 它只说流量去哪 | 不 |
| 网络访问控制列表 | 一个子网 | **能** | **不能 —— 无状态** |
| 安全组 | 一张网卡 | 不能 —— **只有允许** | **能 —— 有状态** |
| 主机 | 实例本身 | 能 | 视实现 |
| 服务 | 那个进程 | —— | —— |

**其中两行就是这一篇的全部。** 一个只有允许的列表，除了改掉或删掉已有的东西之外，无法收窄任何东西；而一个不记得连接的列表，除了请求之外还必须给回程流量一条规则。

**而层的顺序对"读懂一份配置"是要紧的。** 一条放行某端口的规则是五个问题里的第三个，而第一个问题的答案可以让剩下的都变得无关。

### 第二部分：路由排在一切许可之前

实测，两个子网的安全组一样、列表一样、同一个端口上有服务在听：

| 子网 | 路由表 | 结果 |
|---|---|---|
| 有默认路由指向网关的那个 | 有一条通往互联网的路由 | **可达** |
| **只有本地路由的那个** | 没有向外的路由 | **不可达 —— 这个子网没有通往互联网的路，外面无法主动发起** |

**两种情况下安全组都放行了那个端口**，而在其中一种情况下那份许可没有任何外面的人能行使。

> **"安全组允许 443"不等于"外面连得上"。在许可之前，得先有一条路。**

**这意味着暴露面评审从路由表开始、而不是从安全组开始**；一台在私有子网里、安全组全开的实例，与一台在公有子网里、安全组全开的实例，是两种不同性质的发现 —— **后者是一次暴露，前者是一次潜在的暴露，而它会在一条路由出现的那一刻变成暴露。**

**而从外面可达需要三件事同时成立**：一条通往网关的路由、一个外面能指向的地址、以及一份许可。**任何一件缺失都让另外两件无害**，这就是为什么这三者中任何一个的改动都值得同样的关注。

### 第三部分：一个记得连接，一个不记得

这一组实测解释了大多数"它明明应该能通"的报告。实测，入站 443 被放行，而出站那一侧一种情况配成一样、另一种情况没有：

| 无状态的列表 | 结果 |
|---|---|
| 放请求进来、也放响应出去 | **可达** |
| **只放请求进来** | **连不上 —— 出站列表没有放回程流量的规则** |

**入站规则是对的、安全组是对的、服务也在听，而连接没有建立起来。** 原因在回程：从服务端回到客户端的那个响应，它的目标端口是客户端的源端口，那是一个高位端口，而无状态的列表把那一个包当成一次独立的判断。

> **这两层之间的差别不在于规则写得好不好，而在于"谁来负责回程流量"。**

**有状态的列表接受一条连接，然后放行属于它的所有东西**，所以一条入站规则就够了、不需要出站规则。**无状态的列表看每一个包、对它所属于的那次请求毫无记忆**，所以两个方向各需要一条规则，而且回程那条必须覆盖客户端的端口范围。

**由此产生一个很有特征的症状**：回程规则写了很窄的端口范围时，**有些客户端能通、有些不能**，取决于它们碰巧用了哪个源端口 —— 一个很难从配置里读出来的偶发故障，而一旦理解了那一层就很好解释。

### 第四部分：只有其中一层能拒绝

**实测，一份在较小号上写着拒绝、较大号上写着允许的列表拦住了流量，而同样两条规则顺序颠倒时放行了。** 这就是编号要紧的原因：**规则按顺序读，先匹配的决定结果。**

**而安全组根本没有拒绝这回事。** 实测，一个同时含有一条针对某个源网段的规则、以及一条针对一切的规则的安全组：

| 安全组里的规则 | 来自那个窄网段之外的请求 |
|---|---|
| `0.0.0.0/0` 上的 443 **以及** `203.0.113.0/24` 上的 443 | **允许 —— 命中那条宽的** |

**加一条更窄的规则什么也没改变**，因为规则是一个并集，而宽的那条早就在那里了。

> **在一个只有允许的模型里，收窄只能是删掉或者改小一条规则。加一条更具体的，什么也收不窄。**

**这就是一种很常见的经历背后的机制**：加了一条限制、什么也没变，于是得出结论说那条规则没生效。**它生效了；它只是没法做减法。**

**因此这两层的用法不同。** 只有允许的那一层放精确的许可，能拒绝的那一层放粗粒度的边界 —— **一个必须永远够不到的源网段、或者一个从来不用的协议**，写一次在子网上，而不是每张网卡各写一遍。

### 第五部分：可达与应答是两个问题

实测，所有规则都放行那个端口、而端口上没有任何东西在听：

| 规则 | 有东西在听 | 结果 |
|---|---|---|
| 全部放行 | 是 | 可达 |
| 全部放行 | **否** | **不可达 —— 那个端口上没有任何东西在应答** |

**扫描器看到的是一次连接被拒，读配置的人看到的是一次端口开放。** 两者描述的是同一个状态。

> **关于一个端口有三个不同的问题：规则允许吗、有东西在听吗、握手能完成吗。只有第一个能从配置里读出来。**

**这就是云身份与对象存储那两篇从另一个方向得出的同一个结论**：一条放行的规则与一个发生了的结果是两个事实，而要知道第二个，唯一可靠的办法是发出一个请求。**对暴露面尤其如此，那意味着从外面探测、而不是罗列规则。**

### 第六部分：暴露面是三件事一起说

实测，一条规则、两个源：

| 规则 | 源 | 结果 |
|---|---|---|
| `0.0.0.0/0` 上的 443 | 任何 | 允许 |
| `203.0.113.0/24` 上的 443 | 那个范围内 | 允许 |
| `203.0.113.0/24` 上的 443 | **那个范围之外** | **拒绝** |

**同一个端口，取决于谁在问，既暴露又不暴露。**

> **暴露面是协议、端口与源三样。一个端口号本身什么都说明不了。**

**而入口通常不止一个。** 一条对等连接、一个中转网关、前面一个负载均衡、一个会回源的分布、一条从办公室来的隧道 —— **每一样都是一条路，而规则的源网段未必是照着它写的**；每一样都是"规则允许"与"有人能到达"之间那处实测差别需要重新检查的地方。

### 第七部分：出站，以及它在被拿下之后决定什么

**入站规则决定谁能进来。出站规则决定一个被拿下的工作负载能做什么**，而出站通常配得宽松得多 —— 往往是默认放行一切。

**而那个差别在同样的三层里是可测的。** 一个有一条通往网关的路由、又有一条默认出站许可的子网，**能到达互联网上的任何地址**，这意味着数据不需要任何隧道就能离开：一次普通的出站请求就把它带走了。

**这就让出站限制成为"其他控制都失效之后仍然成立"的那一条。** 讲请求伪造与实例元数据的那两篇都结束在同一个地方：**攻击者指名的那个目的地被到达，是因为网络允许了它**，而一份目的地白名单是唯一不依赖"应用把输入处理对了"的控制。

**而内部目的地也是这件事的一部分。** 一个能到达实例元数据地址、内部 DNS、或者另一个子网里的数据库的工作负载，它的出站可达范围就是它攻击面的一部分 —— **而元数据那一篇实测过：那个地址有好几种写法**，所以这条限制属于解析之后，而不是一张字符串清单。

### 第八部分：从这些机制推出的安全观念

**一条放行的规则是若干条件里的一个，而且不是第一个。** 实测，一个安全组全开的私有子网不可达，而公有子网里同样配置的就可达。**所以暴露面清单从路由表开始**，而一个新增路由的改动值得与一个新增规则的改动同样的评审。

**无状态与有状态这个区别是那个产出"无法解释的报告"的地方。** 实测，一条在每一层都正确的入站规则，仍然因为回程没有规则而完成不了连接。**读懂一份"明明应该能通"的配置的办法，是把一个包在每个方向上分别跟一遍** —— 那正是那一层没有记忆的列表所做的事。

**而只有允许的那一层无法收窄。** 实测，往一个已经放行一切的安全组里加一条窄规则什么也没变。**所以"我们限制了它"是一个必须靠寻找"仍然放行它的那条规则"来检验的说法** —— 而那次检查是搜最宽的那条，不是搜最新的那条。

**关于一个端口的三个问题在任何报告里都值得分开写。** 允许、在听、可达 —— 这里实测为一个端口的三种状态，而只有第一种是配置事实。**真正有用的那份清单来自探测**，配置用来解释结果，而不是用来预测结果。

**而出站方向是剩下那道控制所在的地方。** 一个被弄去取一个被提供地址的应用、一个拿着被偷身份运行的工作负载、一个角色很宽的函数 —— **它们全都结束在一个目的地上，而网络是唯一那个不管它们是怎么到那里的都能拒绝的层。**

### 检测与缓解

- **把路由表当成暴露面来评审**，因为一条路由才是让一份许可变得可达的东西，而一条新路由会改变它后面每一个安全组的含义。
- **报出"在某端口上放行一切"的入站规则，并连着源网段一起报**，因为实测暴露面取决于两者，而只看端口什么都说明不了。
- **报出"与一条宽规则共存"的窄规则**，因为窄的收不窄任何东西，而它常被误当成一条限制。
- **对流日志里"没有规则看起来允许、却实际发生过"的连接告警**，也**对"规则允许、却没有任何流量"告警**，因为两者合起来就是配置与事实之间的那道差。
- **检查每一份无状态列表上的回程规则**，并盯那里写了窄端口范围所产生的偶发故障。
- **把每一条入口路径列出来** —— 对等、中转、负载均衡、分布、隧道 —— 并检查每一条的源网段是照着什么写的。
- **缓解上，把精确许可放在有状态那一层、把粗粒度拒绝放在能拒绝的那一层**，因为只有一层能表达"永远不"。
- **把出站限制在工作负载真正用的那些目的地**，那是在应用被弄坏之后仍然成立的控制。
- **在解析之后校验目的地**，因为要紧的那些地址不止一种写法。
- **从外面做暴露面清单。** 规则说的是什么被允许，一次连接说的才是什么可达 —— 实测里一个端口三种状态都出现过。
- **并且把"路由、地址、许可"的改动当成同一类改动**，因为可达需要三者同时成立，而一个只盯规则的评审只盯了其中一样。
