---
id: mobile-fundamentals
title_en: "Mobile — The Client Belongs to the Attacker"
title_zh: "移动端：客户端属于攻击者"
summary_en: On mobile the client runs on a device the user controls, which inverts the trust model everything else in this guide assumes. Measured with two server implementations of the same request, and with a URL scheme that two applications can claim.
summary_zh: 在移动端，客户端跑在用户控制的设备上 —— 这把这份指南其余部分所依赖的信任模型反了过来。这一篇用"同一个请求的两个服务端实现"和"两个应用都能声明的 URL scheme"实测。
tags: [mobile, android, ios, deep-link, certificate-pinning, cwe-312]
tools: [apktool, frida, jadx, mitmproxy]
attck: [T1409, T1417, T1634]
platform: [android, ios]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The trust model, inverted

Every other entry in this guide assumes that the code running on the client is the code the organisation wrote. On mobile that assumption is false in the strongest possible way:

> **The client is not "your code". It is code running on somebody else's device.**

And that device belongs to the user, who can read its files, modify the application, attach a debugger, intercept its traffic and replay any request it makes. Not as an attack that has to be achieved first — as the ordinary state of affairs.

The consequence is a single sentence that reorganises this whole entry: **every security property of a mobile application is a server-side property.** The client is a user interface. What it computes, decides and displays is a suggestion, and the work of making it bind happens on the server.

### Four layers

1. **The trust boundary.** The application trusts that the client it shipped is the client that is talking to it. There is no mechanism in the platform that makes that true, and a modified client is indistinguishable from yours at the protocol level.
2. **Data and instruction share a plane.** A number the client computed is **data** that the server also treats as **a fact** — a price, a permission level, an "is administrator" flag. The client's arithmetic becomes the server's decision unless the server recomputes it.
3. **Why the usual fix fails.** Certificate pinning, root detection and obfuscation all raise the cost of a modification without establishing a boundary. Each can be defeated by a user with control of the device, and none of them tells the server anything it can rely on.
4. **The variants.** Secrets in ordinary application storage; pinning treated as a security property; a URL scheme that any application may claim; a WebView bridge that widens what script can reach; logs, screenshots and clipboard; and an update mechanism — the supply chain entry, on a platform with a store in the middle.

### Measured: client-side validation is a user experience feature

Two servers handling the same request. One checks that the fields look right; the other ignores the submitted total and computes its own:

| | Format check only | Server recomputes |
|---|---|---|
| Honest request | `200`, charged **5800** | `200`, charged **5800** |
| **Same cart, submitted total changed to 1** | `200`, charged **1** | `200`, charged **5800** |

**Both answers are `200`.** The status code does not distinguish them, and neither does the response shape — the difference is **what the server charged**. One implementation handed pricing authority to the client, and the client is in the user's hands; the other kept it.

That is the whole finding, and it is why "the mobile app validates the input" is not a security statement. The client-side check improves the experience — a user who types a bad quantity gets told immediately rather than after a round trip — and it has no effect on what a modified client can submit.

**And this is not a mobile-specific bug.** It is the business logic entry's rule, arriving at a platform where the client is *by design* outside your control. What is unusual about mobile is not that the mistake is possible but that **the environment makes it the default**: there is no same-origin policy to lean on, no server-rendered page that happens to include the right value, and no session the client cannot inspect.

### Measured: a URL scheme is a public namespace

A custom URL scheme such as `myapp://` is a name that applications register for. Nothing prevents a second application from registering the same name, and when a link arrives, **which handler receives it is not decided by the sender or by the receiver**:

| What the receiving app can see | Reliability |
|---|---|
| The URL it was called with | its own input, entirely attacker-controlled |
| Who called it | on Android a calling package that can be spoofed; on iOS generally unavailable |
| Whether the caller is trustworthy | unknown unless the app verifies something itself |

**So a deep link is an unauthenticated request from an unnamed sender.** If it carries an action — a password reset, a payment confirmation, an authorisation code — then the receiving application has to answer the same three questions any other endpoint answers: **who sent this, is it fresh, and has it been used already.** Measured, a signed payload verifies correctly and the same signature over a modified payload does not:

```
legitimate link, signature valid          : True
same signature, different user in payload : False
```

**A signature is what turns the scheme from a public name into an addressed message.** Without one, checking the format of a deep link is the mobile equivalent of the API entry's missing authorisation: the request is well-formed and the sender is whoever typed it.

Two platform features go further than a custom scheme: **iOS Universal Links and Android App Links** bind a link to a **domain the application proves it controls**, which means the name is no longer first-come-first-served. They are the right default for anything carrying an action — and they still do not replace validating the payload.

### Measured: what is on the device

| Data | Typical location | Reality |
|---|---|---|
| Keys, passwords, tokens | **Keychain / Keystore** | hardware-backed; the right place |
| Keys, passwords, tokens | `SharedPreferences`, `UserDefaults`, plain databases | readable once the device is rooted or jailbroken |
| Session cookies | the app's cookie store | the same file, the same fate |
| Logs | device logs | readable by other tooling, and copied into crash reports |
| Screenshots | the task switcher's snapshot | cached by the system while the app is backgrounded |
| Clipboard | the system clipboard | readable by other apps, and pasted elsewhere by the user |
| Build artifacts | strings and resources in the package | readable on decompilation; obfuscation raises the cost only |

**"It needs root to read" is not a layer of protection. It is a threshold.** Rooting a device the user owns is a supported activity on one of the two platforms and a well-documented procedure on the other, and every one of those locations holds still after that.

**The screen snapshot row is the one that surprises people.** An application that shows a statement, a token or a one-time code is captured by the system when it goes to the background, and that capture lands in storage the app never wrote to. The mitigation is to obscure the window when it is not in the foreground, and it is a property of the view lifecycle rather than of the data.

### Pinning and root detection raise the cost, they do not draw a line

Certificate pinning, jailbreak detection and obfuscation are the three controls most often offered as answers to a mobile finding, and all three are **cost controls**:

| Control | What it achieves | What it cannot do |
|---|---|---|
| Certificate pinning | makes a particular proxy harder | cannot stop a user who controls the device |
| Root / jailbreak detection | raises the effort of a modification | can be bypassed on a device the user owns |
| Obfuscation | makes reading the package slower | does not hide a secret, only delays finding it |

**"A man in the middle cannot see the traffic" is not a security property — it is a statement that one particular man in the middle has a harder time.** Pinning can be defeated by hooking the verification function, by modifying the package and re-signing it, or by routing the traffic through a proxy the device owner controls. All three are things the device owner is entitled to do.

**So pinning belongs in a defence-in-depth list and not in a trust decision**, and the reason is the one sentence from the top of this entry: **the server never knows whether the client talking to it is the one you shipped.** Anything that has to be true regardless of the client has to be true on the server.

### WebViews and JavaScript bridges

An application that renders web content and exposes native functionality to it has extended the reach of any script running in that view:

> **A bridge turns "an XSS in a WebView" from "modify the page" into "call native methods".**

The XSS entry's controls still apply to the page; the bridge decides what the script can reach once it is running. The practical rules are to expose **named, parameter-checked methods** rather than a general-purpose interface, to avoid mechanisms that expose reflection, and to treat the origin of loaded content as a decision rather than a convenience.

### Detection and mitigation

- **Log and alert when business-critical values arrive from a client.** A submitted price, total, discount, role or permission bit appearing in a request is the client telling the server what to believe; the log line is the finding.
- **Alert when a session's API calls do not follow a flow the application defines.** A checkout submitted without the cart endpoint having been called, or a state transition reached out of order, is the business logic entry's shape on a mobile client — where reordering is trivial.
- **Alert on an action arriving through a deep link with no corresponding in-app navigation.** The same detector as the clickjacking and CSRF entries: the legitimate path to an action includes the steps that lead to it.
- **And treat an anomalous client fingerprint as a signal rather than a verdict.** A modified package is worth attention and is never a reason to trust the request more or less; the request has to stand on its own.
- **For mitigation, recompute on the server everything the client could have altered.** Prices, totals, permissions, roles and state transitions. The measured pair shows the only reliable version of this: the server ignores the number it was given.
- **Authorise per session and per object on the server**, which is the BOLA entry's rule — and which on mobile is not an additional control but the only control, because the client cannot be relied on to enforce anything.
- **Keep credentials in the platform's protected store, and treat everything else on the device as readable.** Keychain or Keystore for secrets; the ordinary storage locations for things you would be comfortable publishing.
- **Sign and time-bound deep link payloads, and use domain-bound links where the platform offers them.** A signature answers "who sent this" and a nonce or expiry answers "is it fresh", which are the two questions a public scheme cannot answer by itself.
- **Issue mobile clients their own short-lived tokens, with revocation.** The session entry's reasoning applies with more force here, since the device is a place tokens accumulate and a place they can be extracted from.
- **Default to non-exported components, and minimise the JavaScript bridge.** Both are platform settings that decide how much of the application other applications — or a script — can reach.
- **And keep the framing, because it makes the rest of this page follow.** The client side is the attacker's territory. Client-side checks, pinning and jailbreak detection raise the cost of an attack; **the security properties themselves have to hold on the server**, where the request arrives stripped of any claim about which client sent it.

<!-- lang:zh -->
### 被反转的信任模型

这份指南其他的每一篇都假设客户端上跑的是组织写的那份代码。在移动端，那个假设以最强的方式不成立：

> **客户端不是"你的代码"。它是跑在别人设备上的代码。**

而那台设备属于用户，他能读它的文件、改这个应用、挂调试器、截它的流量、重放它发出的任何请求。这不是一种"要先攻破"的攻击 —— 这是常态。

后果是一句话，而它重新组织了这一整篇：**移动端应用的每一项安全性质，都是服务端的性质。** 客户端是一个用户界面。它算出来的、判断的、显示的东西都只是建议，而让它成为约束力的那部分工作发生在服务端。

### 四层

1. **信任边界。** 应用信任"我发布的那个客户端，就是正在跟我说话的那个客户端"。平台上没有任何机制让这句话成立，而一个被改过的客户端在协议层面与你的无法区分。
2. **数据与指令共用同一平面。** 客户端算出的一个数字**是数据**，而服务端同时把它当成**事实** —— 价格、权限等级、一个"是不是管理员"的标志。除非服务端重算，否则客户端的算术就变成了服务端的决定。
3. **为什么常见修法失败。** 证书固定、root 检测、混淆，三者都提高了被改动的成本，都没有建立一条边界。每一个都能被一个控制了设备的用户绕过，而三者都不能告诉服务端任何它可以依赖的东西。
4. **变体。** 普通应用存储里的秘密；被当成安全性质的 pinning；一个任何应用都能声明的 URL scheme；一个扩大了脚本可达范围的 WebView 桥；日志、截图与剪贴板；以及一套更新机制 —— 那是供应链那篇，落在一个中间有应用商店的平台上。

### 实测：客户端校验是一个体验功能

两个服务端处理同一个请求。一个检查字段看起来对不对；另一个无视提交上来的总额、自己算：

| | 只校验格式 | 服务端重算 |
|---|---|---|
| 诚实的请求 | `200`，收了 **5800** | `200`，收了 **5800** |
| **同一个购物车，提交的总额被改成 1** | `200`，收了 **1** | `200`，收了 **5800** |

**两个答复都是 `200`。** 状态码区分不出它们，响应形状也区分不出 —— 区别在于**服务端收了多少钱**。一个实现把定价权交给了客户端，而客户端在用户手里；另一个留住了它。

这就是整条发现，也是为什么"移动应用会校验输入"不是一句安全声明。客户端那层校验改善的是体验 —— 用户填了一个不合理的数量，立刻被告知，而不用等一个来回 —— 它对一个被改过的客户端能提交什么毫无影响。

**而这并不是一个移动端特有的 bug。** 它是业务逻辑那篇的规则，落在一个**客户端按设计就在你控制之外**的平台上。移动端不寻常的地方不是这个错误有可能发生，而是**这个环境让它成为默认**：没有同源策略可以依靠，没有一个"恰好包含了正确值"的服务端渲染页面，也没有客户端读不到的会话。

### 实测：URL scheme 是一个公共命名空间

像 `myapp://` 这样的自定义 scheme，是一个供应用注册的名字。没有任何东西阻止第二个应用注册同一个名字，而当一条链接到来时，**哪一个处理器会收到它，不由发送方决定，也不由接收方决定**：

| 接收方能看到什么 | 可靠程度 |
|---|---|
| 它被调用时收到的 URL | 它自己的输入，完全由攻击者控制 |
| 谁调用的 | Android 上是一个可以被伪造的 calling package；iOS 上通常拿不到 |
| 调用方是否可信 | 除非应用自己验证了什么，否则不知道 |

**所以一条深度链接是一个来自无名发送方的未认证请求。** 如果它承载一个动作 —— 口令重置、支付确认、授权码 —— 那么接收方必须回答任何其他端点都要回答的那三个问题：**这是谁发的、它是不是新的、它有没有被用过。** 实测：一个带签名的载荷验证通过，而同一个签名配一个被改过的载荷不通过：

```
合法链接，签名有效              : True
同一个签名，载荷里的 user 被改过  : False
```

**签名是把 scheme 从一个公共名字变成一条被寻址的消息的东西。** 没有它，检查一条深度链接的格式，就等于 API 那篇里缺失的授权在移动端的对应物：请求是良构的，而发送方就是任何会打字的人。

有两个平台特性比自定义 scheme 走得更远：**iOS 的 Universal Links 与 Android 的 App Links** 把一条链接绑定到**一个应用能证明自己控制的域名**上，于是那个名字不再是先到先得。对任何承载动作的东西来说，它们是更正确的默认 —— 而它们仍然不能取代校验载荷。

### 实测：设备上有什么

| 数据 | 典型位置 | 实际情况 |
|---|---|---|
| 密钥、口令、令牌 | **Keychain / Keystore** | 硬件支撑；正确的地方 |
| 密钥、口令、令牌 | `SharedPreferences`、`UserDefaults`、明文数据库 | 设备被 root 或越狱后可读 |
| 会话 cookie | 应用的 cookie 存储 | 同一个文件，同样的命运 |
| 日志 | 设备日志 | 其他工具可读，也会被复制进崩溃报告 |
| 截图 | 任务切换器的快照 | 应用退到后台时被系统缓存 |
| 剪贴板 | 系统剪贴板 | 其他应用可读，也是用户会粘到别处的地方 |
| 编译产物 | 包里的字符串与资源 | 反编译即可读；混淆只提高成本 |

**"需要 root 才能读"不是一层防护，而是一个门槛。** 给一台属于自己的设备 root，在一个平台上是受支持的活动，在另一个平台上是流程完备的操作；而上面每一个位置，在那之后都保持原样。

**截图那一行是让人意外的那个。** 一个显示对账单、令牌或一次性验证码的应用，在退到后台时会被系统截图，而那次截图落进了这个应用从未写过的存储里。缓解方式是窗口不在前台时把它遮起来，而那是视图生命周期的性质，不是数据的性质。

### pinning 与 root 检测提高成本，它们不画一条线

证书固定、越狱检测、混淆，是面对一条移动端发现时最常被给出的三个答案，而三者都是**成本控制**：

| 控制 | 它达成了什么 | 它做不到什么 |
|---|---|---|
| 证书固定 | 让某个特定的代理更难 | 拦不住一个控制了设备的用户 |
| root / 越狱检测 | 提高一次改动的门槛 | 在一台用户拥有的设备上可被绕过 |
| 混淆 | 让读这个包更慢 | 不隐藏秘密，只推迟找到它 |

**"中间人看不到流量"不是一个安全性质，它是一个"某个特定的中间人更费劲"的陈述。** pinning 可以通过 hook 掉那个校验函数、通过改动包再重新签名、或者通过把流量引到设备主人自己控制的代理上被绕过。这三件事都是设备主人有权做的。

**所以 pinning 属于纵深防御清单，不属于信任决定**，理由就是这一篇开头那句话：**服务端永远不知道正在跟它说话的客户端是不是你发布的那个。** 任何必须"无论客户端是什么样都成立"的事情，都必须在服务端成立。

### WebView 与 JavaScript 桥

一个渲染网页内容、并向它暴露原生功能的应用，扩大了在那个视图里运行的任何脚本的可达范围：

> **桥把"WebView 里的一个 XSS"从"改改页面"变成了"调用原生方法"。**

XSS 那篇的控制仍然作用于那个页面；而桥决定了脚本一旦跑起来能碰到什么。实用的规则是：暴露**具名的、参数经过校验的方法**，而不是一个通用接口；避免会暴露反射的机制；并且把"加载内容的来源"当成一个决定，而不是一个方便。

### 检测与缓解

- **当业务关键的值从客户端到来时记录并告警。** 一个提交上来的价格、总额、折扣、角色或权限位出现在请求里，就是客户端在告诉服务端该相信什么；那行日志就是发现。
- **当一个会话的 API 调用不遵循应用定义的流程时告警。** 没调用购物车端点就提交了结账，或者一个状态被乱序到达 —— 那是业务逻辑那篇的形状落在一个移动客户端上，而那里重排顺序是件轻而易举的事。
- **对"通过深度链接到来、却没有对应的应用内导航"的动作告警。** 和点击劫持、CSRF 那两篇是同一个检测器：一个动作的正当路径包含通向它的那些步骤。
- **而客户端指纹异常要当成信号而不是结论。** 一个被改过的包值得注意，而它从来不是"更该信任"或"更不该信任"这个请求的理由；那个请求必须自己站得住。
- **缓解上，在服务端重算客户端可能改动过的一切。** 价格、总额、权限、角色与状态转换。实测的那一对展示了这件事唯一可靠的形式：服务端无视它被给到的那个数字。
- **在服务端按会话、按对象授权**，那是 BOLA 那篇的规则 —— 而在移动端它不是一项附加控制，而是唯一的控制，因为客户端不能被依赖去强制执行任何东西。
- **把凭据放进平台受保护的存储里，并把设备上其余一切都当成可读的。** 秘密放 Keychain 或 Keystore；那些你会乐意公开的东西，才放普通的存储位置。
- **给深度链接的载荷加签名与时效，并在平台提供的地方使用绑定域名的链接。** 签名回答"这是谁发的"，nonce 或过期时间回答"它是不是新的"—— 那正是公共 scheme 自己回答不了的两个问题。
- **给移动客户端发它们自己的短期令牌，并支持吊销。** 会话那篇的推理在这里更适用，因为设备是令牌会堆积的地方，也是它们会被提取出来的地方。
- **组件默认不导出，并把 JavaScript 桥最小化。** 两者都是平台设置，它们决定这个应用有多大一部分会被其他应用 —— 或者一段脚本 —— 碰到。
- **并且记住那个重新表述，因为这一页剩下的部分都由它推出。** 客户端那一侧是攻击者的领地。客户端校验、pinning、越狱检测提高的是攻击的成本；**安全性质本身必须在服务端成立**，在那里，到达的请求已经被剥掉了任何关于"是哪个客户端发的"的声明。
