---
id: deserialization-defense-and-detection
title_en: "Deserialisation, Part 3 — Defence, Bypasses and Detection"
title_zh: "反序列化（三）：防御、绕过与检测"
summary_en: Defences differ in kind — removing the channel beats limiting the set, and limiting the set beats inspecting the content. This entry ranks them, covers the bypasses that survive a class allow-list, warns about the JSON-shaped trap, and gives the signatures and the inventory checklist.
summary_zh: 防御是有种类差别的 —— 移除通道强于限定集合，限定集合强于检查内容。这一篇把它们排序，讲清能穿过类允许清单的几种绕过，提醒那个"长得像 JSON"的陷阱，并给出指纹表与盘点清单。
tags: [web, deserialization, cwe-502, allow-list, detection, jackson, fastjson]
tools: [php, java, python3, ysoserial, phpggc]
attck: [T1190, T1059]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Defences are not all the same kind of thing

The previous two entries described what the vulnerability is and what makes it exploitable. This one is about defence, and the useful way to organise it is by **what kind of thing each control does**:

| Kind | Example | What it can do |
|---|---|---|
| **Remove the channel** | Do not deserialise untrusted data; use a format that cannot name a type | Eliminates the class |
| **Limit the set** | Runtime class allow-list; shrink the class path | Shrinks the space where chains exist |
| **Inspect the content** | Signatures, deny-lists, WAF rules | Detects attempts that match what you already know |
| **Bound the consequence** | Isolation, least privilege, separate process | Does not prevent; decides the size of the incident |

The order is not a preference. **A control that removes the channel cannot be bypassed by a cleverer payload, because there is no longer a payload that means anything.** Everything below that line is a matter of degree, and every bypass in this entry applies to the lower kinds.

### The JSON-shaped trap

This is the most common false comfort in the whole subject, and it is worth stating precisely.

**Using JSON does not make deserialisation safe. Using a format that cannot express a type name does.** Some JSON libraries will happily write a class name into the document and reconstruct that class on the way back — and then the payload is JSON, it passes every format check, and it does exactly what a binary serialisation stream would have done.

| Library or feature | What it enables |
|---|---|
| Jackson with default typing enabled | A class name in the JSON; the named class is constructed |
| fastjson `@type` | The same, from the payload's own field |
| Any "polymorphic deserialisation" feature | Type information carried in the document by design |
| XStream, SnakeYAML in default mode | XML or YAML naming the class to build |
| `node-serialize` and similar | A function body carried in the payload as a string |

So the question to ask about a JSON API is not "is it JSON" but **"can the document name a type, and does the parser act on that name?"** If yes, the API has the same exposure as `ObjectInputStream` and deserves the same treatment: an allow-list at the parser, and ideally a schema that does not carry types at all.

### Bypasses that survive a class allow-list

An allow-list is a genuine improvement, and it is a control over **which classes may be constructed** — not over what those classes do. Several techniques follow from that distinction.

**Indirect arrival.** The allow-listed class does not have to be the sink. `Foo` may be permitted because the application needs it, and `Foo`'s `readObject` may call something reachable that the allow-list never had to mention. The list constrains the **entry** of the chain, not its length.

**Classes that are always allowed.** JDK and framework classes tend to be allow-listed wholesale — `java.*`, `java.util.*`, and the application's own base packages. But `PriorityQueue` invokes a comparator, `HashMap` can be made to invoke `equals` and `hashCode` on keys, and `<java.*>` is a large space with its own published chains. An allow-list entry like `com.example.*` or `java.*` is a wildcard that reopens what the list was meant to close.

**Double deserialisation.** If an allow-listed class contains a byte array or a nested stream that the application deserialises later, the first layer is legitimate and the second layer is the attack. Classes designed to wrap and carry opaque data — signed objects, caches, message envelopes — are the usual carriers. The defence has to apply to **every** deserialisation, including the ones inside a library.

**Parser-detail bypasses, and what happened to them.** There is a family of tricks that exploits details of a parser rather than classes, and one is worth knowing because it teaches something about failures. In older PHP, a serialised object whose declared property count was **higher** than the properties actually present caused `unserialize` to skip `__wakeup` — so a defence that relied on `__wakeup` running could be skipped, while `__destruct` still fired when the object was collected. Measured on PHP 8.4:

| Payload | `unserialize` result | Object created | Hooks observed |
|---|---|---|---|
| Correct property count | success | yes | `__wakeup`, then `__destruct` on collection |
| Declares 3, provides 2 | **fails** | no | no `__wakeup` |
| Declares 1, provides 2 | **fails** | no | **`__wakeup` already ran** |

Two lessons, and the second is the one to keep:

- **The announced bypass is closed on current versions** — the mismatched payloads are rejected rather than silently mishandled.
- **"The parse failed" is not the same as "nothing happened".** In the third row the hook had already executed before the failure was reported. A system that logs a deserialisation error and treats it as a non-event is treating a possible side effect as a non-event, and the same reasoning applies to any parser with hooks: **the code runs during parsing, so the parse has to succeed at the end for anything to be safe — fence the input before the parser sees it, not after it fails.**

### Signatures worth alerting on

These are the format fingerprints, with the byte values and the base64 prefixes measured on a current system. The base64 column is the practical one, because serialised data is usually transported encoded.

| Format | Leading bytes | Base64 prefix | Notes |
|---|---|---|---|
| Java serialisation | `AC ED 00 05` | **`rO0AB`** | Also appears inside other payloads, so search the whole value |
| Python pickle | `80` + protocol byte (`04`/`05`) | **`gASV`**, `gAUV` | The prefix varies with protocol and content; match on the first two bytes after decoding |
| Ruby `Marshal` | `04 08` | **`BAh`** | Short and common enough to need context |
| Perl `Storable` | `04 0B 08` | **`BAsI`** | Distinct from Ruby despite the shared `04` |
| .NET `BinaryFormatter` | `00 01 00 00 00 FF FF FF FF` | **`AAEAAAD/////`** | |
| ASP.NET ViewState | (varies) | **`/wEP`** | The `__VIEWSTATE` field itself is the signal |
| PHP `serialize` | plaintext `O:<n>:"Class"` or `a:<n>:{` | (no encoding) | Readable without decoding, which makes it the easiest to catch |

A detection rule worth writing: **a parameter, cookie or body field whose decoded value begins with any of these is carrying a serialised object**, and there is almost no legitimate reason for a client to do that. For Java, also search the whole value rather than only the prefix — a serialised stream can appear mid-payload in a larger structure.

### Detection and mitigation

- **Class loading on deserialisation paths.** This is the strongest signal in the whole entry, and it is the one that catches unpublished chains. A service that reconstructs a preferences object has no reason to load a collections library, an expression evaluator or an XML transformer. Watching **which classes are loaded while a request is being deserialised**, and alerting on classes outside the expected set, does not require knowing any chain.
- **Baseline the streams.** A given endpoint's serialised values have a typical length, a typical class set and a typical shape. An endpoint that normally receives 40 bytes and receives 4 KB carrying unfamiliar class names is anomalous, and the anomaly is measurable without understanding the payload.
- **Treat parse failures as events.** As the PHP measurement shows, a failure can be reported after a hook has run. A burst of deserialisation errors from one source is someone probing the parser, and each error is a data point about what the parser accepts.
- **Watch what the process does next.** A chain's purpose is a child process, a file write, an outbound connection or a lookup. Process creation from a web worker, DNS for unexpected names and connections to internal addresses are the same signals the SSRF and command-injection entries use — and they are the ones that fire when the payload itself was not recognised.
- **Alert on class names in error messages and logs.** Class-not-found and cast failures on a deserialisation endpoint mean the input named a type and the runtime tried to build it.

### The inventory everyone should run

The reason these bugs persist is that the deserialisation entry points are scattered across a stack. The check is a list, and it is short enough to actually do:

| Platform | Search for |
|---|---|
| **Java** | `ObjectInputStream`, `XMLDecoder`, `XStream`, `SnakeYAML`, Jackson with default typing, fastjson `@type`, RMI, JMX, JMS consumers, HTTP session replication |
| **PHP** | `unserialize`, session handlers, `phar://` (a file operation on a phar triggers deserialisation), framework cache and queue drivers |
| **Python** | `pickle`, `cPickle`, `marshal`, `shelve`, `yaml.load` without a safe loader, `torch.load`, `joblib.load`, `pandas.read_pickle`, Django signed cookies |
| **Ruby** | `Marshal.load`, `YAML.load`, `JSON.load` with `create_additions` |
| **.NET** | `BinaryFormatter`, `LosFormatter`, `NetDataContractSerializer`, `ObjectStateFormatter`, ViewState with a known key |
| **Node.js** | `node-serialize`, `serialize-javascript`, libraries that `eval` a deserialised string |

`phar://` deserves the parenthetical it has: in PHP, **a file operation on a phar archive triggers deserialisation of its metadata**, so the entry point can be a function that looks like it only checks whether a file exists. That is why the inventory is a search for APIs rather than a review of input handling.

#### Applying the fixes in order

1. **Inventory.** Find every deserialiser, including inside libraries and including the indirect triggers above.
2. **Remove or replace.** Where the data crosses a trust boundary, move to a format that cannot name a type. This is the step that ends the class for that path.
3. **Allow-list where replacement is impossible.** Enforce it in the runtime, keep the list beside the dependency tree, and review the wildcards — `java.*` and `com.example.*` are the entries that quietly undo the control.
4. **Shrink the class path.** Remove dependencies the application does not use. A library that is not present cannot be a gadget, and this requires knowing nothing about chains.
5. **Isolate and limit.** Least privilege, a separate process, no access to the code tree, and outbound restrictions so that a successful chain has somewhere uninteresting to land.
6. **Monitor.** Signatures for the known, class loading and baseline deviation for the unknown, and process behaviour for the consequences.

Steps 2 and 4 are the ones that change the property rather than the odds.

<!-- lang:zh -->
### 防御不是一个种类的东西

前两篇讲了漏洞是什么、以及什么使它可利用。这一篇讲防御，而有用的组织方式是按**每一项控制做的是哪一种事**：

| 种类 | 例子 | 它能做到什么 |
|---|---|---|
| **移除通道** | 不反序列化不可信数据；用无法命名类型的格式 | 消除这一整类 |
| **限定集合** | 运行时类允许清单；给类路径瘦身 | 缩小"链存在"的空间 |
| **检查内容** | 签名、黑名单、WAF 规则 | 检出与你已知特征相符的尝试 |
| **限制后果** | 隔离、最小权限、独立进程 | 不阻止发生；决定事故的大小 |

这个顺序不是偏好。**一项移除通道的控制，不会被更聪明的载荷绕过 —— 因为已经不存在"有意义的载荷"了。** 那条线以下的一切都是程度问题，而这一篇里的每一个绕过都属于下面那几种。

### 长得像 JSON 的那个陷阱

这是整个主题里最常见的一种虚假安心，值得说准。

**用 JSON 并不会让反序列化变安全。让格式无法表达类型名才会。** 有些 JSON 库会痛快地把类名写进文档，并在回来的路上重建那个类 —— 于是载荷是 JSON、通过一切格式检查，而它做的事和一段二进制序列化流一模一样。

| 库或特性 | 它打开了什么 |
|---|---|
| 开启了默认类型的 Jackson | JSON 里的一个类名；那个被指名的类会被构造 |
| fastjson 的 `@type` | 同上，而且来自载荷自己的字段 |
| 任何"多态反序列化"特性 | 按设计把类型信息放进文档 |
| 默认模式下的 XStream、SnakeYAML | XML 或 YAML 指名要构造的类 |
| `node-serialize` 之类 | 把函数体当字符串放进载荷 |

所以对一个 JSON 接口要问的不是"它是不是 JSON"，而是**"这份文档能不能命名一个类型，以及解析器会不会照那个名字去做？"** 如果会，这个接口和 `ObjectInputStream` 有同样的暴露面，也该受同样的对待：在解析器那层放允许清单，理想情况下则用一个完全不携带类型的 schema。

### 能穿过类允许清单的几种绕过

允许清单是实打实的改进，而它管的**是"哪些类可以被构造"**，不是"那些类会做什么"。几种手法就是从这条区分里长出来的。

**间接到达。** 被允许的那个类不必是落点。`Foo` 可能因为应用需要而被允许，而 `Foo` 的 `readObject` 可能调用了某个可达的东西，那是允许清单从来不需要提到的。清单限制的是链的**入口**，不是链的**长度**。

**永远被允许的那些类。** JDK 与框架类往往被整片放行 —— `java.*`、`java.util.*`、应用自己的基础包。但 `PriorityQueue` 会调用比较器，`HashMap` 可以被做成对键调用 `equals` 与 `hashCode`，而 `java.*` 是一片有自己公开链的广大空间。`com.example.*` 或 `java.*` 这样的清单条目是一个通配符，它把清单本来要关上的东西重新打开了。

**二次反序列化。** 如果一个被允许的类里含有一段字节数组或一条嵌套的流，而应用之后会去反序列化它，那么第一层是正当的，第二层才是攻击。那些设计用来包裹并携带不透明数据的类 —— 签名对象、缓存、消息信封 —— 是常见的载体。防御必须覆盖**每一次**反序列化，包括库内部那些。

**解析器细节层面的绕过，以及它们后来怎么了。** 有一族手法利用的是解析器的细节而不是类。其中一个值得知道，因为它教了一件关于"失败"的事。在老版本 PHP 上，一个序列化对象如果**声明的属性数多于实际给出的**，会让 `unserialize` 跳过 `__wakeup` —— 于是一项依赖 `__wakeup` 会运行的防御可以被跳过，而对象被回收时 `__destruct` 仍然触发。在 PHP 8.4 上实测：

| 载荷 | `unserialize` 结果 | 对象建出来了吗 | 观察到的钩子 |
|---|---|---|---|
| 属性数正确 | 成功 | 是 | `__wakeup`，随后回收时 `__destruct` |
| 声明 3 个、只给 2 个 | **失败** | 否 | 没有 `__wakeup` |
| 声明 1 个、却给 2 个 | **失败** | 否 | **`__wakeup` 已经跑过了** |

两个教训，第二个是要留住的：

- **那个被公开的绕过在当前版本上已经关上了** —— 不匹配的载荷被拒绝，而不是被悄悄错误处理。
- **"解析失败了"与"什么都没发生"不是同一句话。** 第三行里，钩子在失败被报出来之前就已经执行完了。一个把反序列化错误记下来、并当作无事发生的系统，是把一个可能的副作用当成了无事发生；而同样的推理适用于任何带钩子的解析器：**代码是在解析过程中运行的，所以"解析最终成功"才是安全的前提 —— 要在解析器看到输入之前就把它围起来，而不是等它失败之后。**

### 值得告警的指纹

下面是各格式的指纹，字节值与 base64 前缀都在当前系统上实测过。base64 那一列才是实用的，因为序列化数据通常以编码形式传输。

| 格式 | 开头字节 | base64 前缀 | 说明 |
|---|---|---|---|
| Java 序列化 | `AC ED 00 05` | **`rO0AB`** | 也会出现在别的载荷**内部**，所以要搜整个值 |
| Python pickle | `80` + 协议字节（`04`/`05`） | **`gASV`**、`gAUV` | 前缀随协议与内容变化；解码后按头两个字节匹配 |
| Ruby `Marshal` | `04 08` | **`BAh`** | 太短太常见，需要上下文 |
| Perl `Storable` | `04 0B 08` | **`BAsI`** | 尽管共享 `04`，与 Ruby 可区分 |
| .NET `BinaryFormatter` | `00 01 00 00 00 FF FF FF FF` | **`AAEAAAD/////`** | |
| ASP.NET ViewState | （不固定） | **`/wEP`** | `__VIEWSTATE` 字段本身就是信号 |
| PHP `serialize` | 明文 `O:<n>:"Class"` 或 `a:<n>:{` | （无编码） | 不解码就能读，最容易抓 |

一条值得写的检测规则：**一个参数、cookie 或请求体字段，其解码后的值以其中任何一个开头，就是在携带一个序列化对象** —— 而客户端几乎没有正当理由这么做。对 Java 还要**搜整个值**而不只是前缀 —— 序列化流可以出现在更大结构的中段。

### 检测与缓解

- **反序列化路径上的类加载。** 这是全篇最强的信号，也是能抓到**尚未公开**的链的那一个。一个只是重建偏好对象的服务，没有理由去加载一个集合库、一个表达式求值器或一个 XML 转换器。盯**请求被反序列化期间加载了哪些类**，并对预期集合之外的类告警，不需要知道任何链。
- **给流做基线。** 某个端点的序列化值有典型的长度、典型的类集合和典型的形状。一个平时收 40 字节的端点收到 4 KB、还带着陌生类名，就是异常，而这个异常**不需要看懂载荷**就能测出来。
- **把解析失败当事件。** 正如那个 PHP 实测显示的，失败可能是在钩子跑完之后才被报出来的。一个来源爆发式的反序列化错误，就是有人在探这个解析器，而每一条错误都是关于"解析器接受什么"的数据点。
- **盯进程接下来做什么。** 一条链的目的是子进程、写文件、出网连接或一次查询。Web 工作进程创建子进程、向意外域名发起 DNS、连到内网地址 —— 这些和 SSRF、命令注入那两篇用的是同一批信号，而且它们正是**载荷本身没被认出来**时还会响的那些。
- **对错误信息与日志里的类名告警。** 反序列化端点上的"类找不到"和类型转换失败，意味着输入指名了一个类型、而运行时试着构造了它。

### 每个人都该跑一遍的盘点

这些 bug 之所以顽固，是因为反序列化的入口散落在一整套技术栈里。这个检查就是一份清单，而且短到真的能做完：

| 平台 | 搜什么 |
|---|---|
| **Java** | `ObjectInputStream`、`XMLDecoder`、`XStream`、`SnakeYAML`、开启默认类型的 Jackson、fastjson 的 `@type`、RMI、JMX、JMS 消费者、HTTP 会话复制 |
| **PHP** | `unserialize`、会话处理器、`phar://`（对 phar 的一次文件操作会触发反序列化）、框架的缓存与队列驱动 |
| **Python** | `pickle`、`cPickle`、`marshal`、`shelve`、不带安全 loader 的 `yaml.load`、`torch.load`、`joblib.load`、`pandas.read_pickle`、Django 的签名 cookie |
| **Ruby** | `Marshal.load`、`YAML.load`、带 `create_additions` 的 `JSON.load` |
| **.NET** | `BinaryFormatter`、`LosFormatter`、`NetDataContractSerializer`、`ObjectStateFormatter`、密钥已知的 ViewState |
| **Node.js** | `node-serialize`、`serialize-javascript`、以及任何会 `eval` 反序列化字符串的库 |

`phar://` 值得它那个括号：在 PHP 里，**对 phar 归档的一次文件操作会触发它元数据的反序列化**，所以入口可以是一个看起来只是在检查文件是否存在的函数。这也是为什么这份盘点要**搜 API**，而不是去评审输入处理。

#### 按顺序落地这些修法

1. **盘点。** 找出每一个反序列化器，包括库里的，也包括上面那些间接触发点。
2. **移除或替换。** 在数据跨越信任边界的地方，换成无法命名类型的格式。这一步是为那条路径终结这一类的步骤。
3. **换不了的地方放允许清单。** 在运行时强制，清单放在依赖树旁边，并**审视通配符** —— `java.*` 和 `com.example.*` 就是那些悄悄把控制撤销掉的条目。
4. **给类路径瘦身。** 移除应用用不到的依赖。不在场的库不可能成为 gadget，而这件事不需要知道任何一条链。
5. **隔离与限制。** 最小权限、独立进程、不能碰代码树、限制出网 —— 让一条成功的链落在一个没意思的地方。
6. **监控。** 已知的靠指纹，未知的靠类加载与基线偏离，后果靠进程行为。

第 2 步和第 4 步是**改变性质**的那两步，而不是改变概率的。
