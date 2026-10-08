---
id: deserialization-fundamentals
title_en: "Deserialisation, Part 1 — Restoration Is Construction"
title_zh: "反序列化（一）：还原就是构造"
summary_en: A serialisation format has to record which type to build, so restoring an object means walking the code path that constructs it. This entry shows that instruction inside the byte stream with a pickle disassembly, lists how each language expresses it, and explains why signing does not fix it while changing the format does.
summary_zh: 序列化格式必须记录"要构造哪个类型"，于是还原一个对象就等于走一遍构造它的代码路径。这一篇用一段 pickle 反汇编把那条指令摊在字节流里，列出各语言如何表达它，并说明为什么"加签名"修不好而"换格式"能修好。
tags: [web, deserialization, cwe-502, pickle, gadget, integrity]
tools: [python3, php, java, ruby, ysoserial, phpggc]
attck: [T1190, T1059]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The property that makes this class exist

Every entry in this series has come down to the same thing: a string that one component reads as data and another reads as code. Deserialisation is the case where the format itself **cannot avoid carrying instructions**, and the reason is structural rather than a mistake in any implementation.

To reconstruct an object, the format must record **what type it was**. That is a requirement, not a convenience: without the type name, `{"x": 1}` cannot be restored as a `User`, a `Settings` or a `Session`, and the caller cannot get back what it stored. So a serialisation stream contains, at minimum:

- **Values** — the data people think of when they say "serialised".
- **Type information** — the names of the classes to construct.
- **Reconstruction instructions** — how to build them, in some formats explicitly.

Restoring the object therefore means **executing the code path that builds it**, on data chosen by whoever produced the stream. That is the entire vulnerability class, and it is why the right mental model is not "parsing untrusted data" but **"running the other party's constructor"**.

> **Deserialisation is not reading data. It is instantiating objects — and instantiation is code.**

### Seeing the instruction in the byte stream

This is easiest to accept after looking at the bytes rather than the prose. Take the smallest possible Python case:

```python
import base64, pickle, pickletools

class Hook:
    def __reduce__(self):
        return (print, ("the stream chose this call",))

data = pickle.dumps(Hook())
print(data[:2].hex())                                  # 8004  (protocol marker)
print(base64.b64encode(data).decode()[:16])            # gASV... — recognisable in a request
print(pickletools.dis(data))                           # the instruction listing
```

`pickletools.dis` prints the program that the stream contains. Reduced to the interesting lines:

```
SHORT_BINUNICODE 'builtins'
SHORT_BINUNICODE 'print'
STACK_GLOBAL                     <- import builtins.print
SHORT_BINUNICODE 'the stream chose this call'
TUPLE1
REDUCE                           <- call it with that argument
```

Read that as what it is: **the stream names a callable and asks for it to be called.** `pickle.loads` is not decoding a document; it is executing an instruction sequence that the document supplied. No implementation bug is involved — this is what the format is for.

### How each language expresses it

The names differ, the mechanism does not. A measured comparison, each run producing the observation in the last column:

| Language | How the type is named in the stream | Hook invoked on restore | Observed |
|---|---|---|---|
| **Python** | `GLOBAL` / `STACK_GLOBAL` opcodes carrying module and attribute names | `__reduce__`, `__setstate__` | `'builtins'`, `'print'`, `STACK_GLOBAL`, `REDUCE` in the disassembly; the callable ran |
| **PHP** | `O:4:"Hook":1:{...}` — the class name is **plaintext** | `__wakeup`, `__destruct`, `__toString` | `__wakeup` then `__toString` then `__destruct` all fired |
| **Java** | Class descriptors inside the stream | `readObject`, `readResolve` | stream header `AC ED 00 05`, `readObject` fired |
| **Ruby** | Class name in the marshalled data | `_load`, `marshal_load` | header `04 08`, `_load` received the stream's payload |
| **.NET** | Assembly-qualified type name | `ISerializable`, `OnDeserialized` callbacks | type and assembly named in the payload |
| **Perl** | Class name | `STORABLE_thaw` | hook invoked with the stream's payload |

Two observations from that table are worth more than the table:

**The type name is usually plaintext.** In PHP it literally is — `O:4:"Hook":...` — which means changing **one string** in the payload changes which class is constructed, and therefore which code runs. In Python it is a module and attribute name spelled out in opcodes. **The payload is a program with a symbol table**, and editing symbols is a normal text-editing operation.

**The hooks are ordinary methods that were written for other reasons.** `__destruct` exists for cleanup. `readObject` exists for custom reconstruction. `_load` exists for custom formats. None of them was written as an attack surface, and all of them run **before the application looks at the object**.

That last point explains a behaviour that otherwise looks strange: **an application can be vulnerable without ever using the object it deserialised.** The hooks fire during the read, so validation performed afterwards is validation performed too late.

### Why the common fixes do not hold

#### "It is signed, so it cannot be tampered with"

Signing gives **integrity**, not safety, and the two are routinely conflated. Three separate problems:

- **A signature cannot distinguish a legitimate dangerous object from a forged one.** If the class path contains a class whose cleanup method does something useful to an attacker, the application can build that object **itself, honestly, and sign it**.
- **The key is part of the attack surface.** A leaked, shared or default key means the attacker signs their own payloads. The view-state entry in this series is the same failure with a different framework.
- **The data may legitimately come from somewhere untrusted.** A queue message, a cache entry, a session store, an uploaded model file, a cookie set by another component — signed by the application, and influenced by someone else.

#### "We filter the class names"

A deny-list of known-dangerous classes is a list of what has already been published. The interesting gadget is by definition one nobody has written down yet, and it only has to be **on the class path** — which in a large application means the dependency tree, not the application's own code. An allow-list naming the classes the application actually needs is the workable version, and it has to be maintained alongside the dependency tree.

#### "We use a safe loader"

This is the right instinct, and it is only as good as the option actually passed. The language entries already showed the shape: `yaml.load` needed a loader argument, `tarfile.extractall` needed a filter, and in both cases the default was the unsafe one. The same question has to be asked of every deserialisation API: **what does this do when I pass nothing?**

### Why changing the format does fix it

The fix that removes the class rather than filtering it is the one that removes the **ability to name a type**:

| Format | Can it name a callable or a class? |
|---|---|
| JSON | No — objects map to dictionaries, lists, strings, numbers, booleans, null |
| Protobuf, Avro, CBOR with a schema | No, unless a schema explicitly encodes types |
| Java serialisation, Python `pickle`, PHP `serialize`, Ruby `Marshal`, .NET `BinaryFormatter` | **Yes** — that is what they are for |

That is the whole argument, and it is the same shape as parameterised queries and context-aware encoding: **do not filter the dangerous input, remove the channel through which it becomes code.** A format that cannot express "construct this class" cannot be talked into constructing one, and no amount of creativity in the payload changes that.

Where a rich format is genuinely required — distributed caches, internal RPC, model files — the workable position is a **combination**: a format without type naming where possible, and where not, an allow-list of classes enforced by the runtime and a trust boundary that the data genuinely respects.

### Detection and mitigation

- **Alert on the format signatures, because each is distinctive and none belongs in client input.** Java serialisation begins `AC ED 00 05`, which in base64 is the prefix **`rO0AB`**; Python pickle begins with `0x80` and a protocol byte; Ruby `Marshal` begins `04 08`; PHP serialised objects look like `O:` or `a:` followed by a length; .NET binary formatter streams and view state have their own shapes. A parameter, cookie or body field carrying any of these is the platform's vocabulary arriving from outside.
- **Alert on type names appearing where they should not.** A class name in a request is the signature of an attempt to choose what gets constructed — and it is often visible in an error message or a stack trace even when the payload is encoded.
- **Treat any error containing a class-not-found or a cast failure on a deserialisation endpoint as a signal, not noise.** Those errors mean the input named a type, and the application tried to build it. Repeated versions of the same error from one source are someone enumerating the class path.
- **Replace the format where the data crosses a trust boundary.** JSON, or a schema-carrying format. Where that is not possible, enforce an allow-list in the runtime (`ObjectInputFilter` in Java, `safe_load` for YAML, equivalent controls elsewhere), and keep the list next to the dependency tree rather than in the application's own packages.
- **Protect signing keys as a first-class secret.** Per-application, generated rather than committed, rotated, and never shared with a component that handles untrusted input. A signature is only as strong as the key, and a shared key is a shared ability to mint valid payloads.
- **Do not deserialise in a privileged context.** If a component must accept serialised data, run it with the least privilege available, in its own process, so that what the constructor path can reach is bounded. It does not fix the bug and it decides the size of the incident.
- **And keep the framing**: this class exists because a format has to say **what to build**. Every mitigation above is either "do not let the input say what to build", "let it say only what you wrote down in advance", or "make building it uninteresting". The first is the fix; the second is a maintained compromise; the third is damage control.

<!-- lang:zh -->
### 让这一整类存在的那个性质

本系列每一篇最终都落到同一件事：一个字符串，被一个组件读作数据、被另一个读作代码。反序列化是那种"**格式本身无法避免携带指令**"的情况，而原因是结构性的，不是任何实现里的失误。

要重建一个对象，格式必须记录**它原本是什么类型**。那是需求，不是便利：没有类型名，`{"x": 1}` 就无法被还原成 `User`、`Settings` 还是 `Session`，调用方也拿不回它存进去的东西。所以一条序列化流至少包含：

- **值** —— 人们说"序列化数据"时想到的那些。
- **类型信息** —— 要构造的类名。
- **重建指令** —— 在某些格式里是显式的。

于是还原这个对象意味着**在由产出这条流的人选定的数据上，执行构造它的那条代码路径**。这就是整个漏洞类，也是为什么正确的心智模型不是"解析不可信数据"，而是**"运行对方的构造器"**。

> **反序列化不是在读数据，它是在实例化对象 —— 而实例化就是代码。**

### 在字节流里看见那条指令

用字节而不是用文字来解释，最容易让人接受。看最小的 Python 例子：

```python
import base64, pickle, pickletools

class Hook:
    def __reduce__(self):
        return (print, ("the stream chose this call",))

data = pickle.dumps(Hook())
print(data[:2].hex())                                  # 8004（协议标记）
print(base64.b64encode(data).decode()[:16])            # gASV... —— 在请求里认得出来
print(pickletools.dis(data))                           # 指令清单
```

`pickletools.dis` 打印的是这条流里包含的那个**程序**。只留下有意思的几行：

```
SHORT_BINUNICODE 'builtins'
SHORT_BINUNICODE 'print'
STACK_GLOBAL                     <- 导入 builtins.print
SHORT_BINUNICODE 'the stream chose this call'
TUPLE1
REDUCE                           <- 用它作参数调用
```

把它当成它本来的意思读：**这条流点了一个可调用对象的名字，并请求调用它。** `pickle.loads` 不是在解码一份文档，它是在执行一段**由那份文档提供的**指令序列。这里没有任何实现 bug —— 格式就是这个用途。

### 每种语言怎么表达它

名字不同，机制相同。下面是实测对照，每一行最后一列是实际观察到的现象：

| 语言 | 类型在流里怎么被命名 | 还原时触发的钩子 | 实测 |
|---|---|---|---|
| **Python** | 携带模块名与属性名的 `GLOBAL` / `STACK_GLOBAL` 操作码 | `__reduce__`、`__setstate__` | 反汇编里出现 `'builtins'`、`'print'`、`STACK_GLOBAL`、`REDUCE`；那个可调用对象真的跑了 |
| **PHP** | `O:4:"Hook":1:{...}` —— 类名是**明文** | `__wakeup`、`__destruct`、`__toString` | `__wakeup` 先、`__toString` 次、`__destruct` 最后，全都触发 |
| **Java** | 流里的类描述符 | `readObject`、`readResolve` | 流头 `AC ED 00 05`，`readObject` 触发 |
| **Ruby** | 编组数据里的类名 | `_load`、`marshal_load` | 头 `04 08`，`_load` 收到了流里的载荷 |
| **.NET** | 带程序集限定的类型名 | `ISerializable`、`OnDeserialized` 回调 | 类型与程序集在载荷里被指名 |
| **Perl** | 类名 | `STORABLE_thaw` | 钩子带着流里的载荷被调用 |

那张表里有两个观察比表本身更值钱：

**类型名通常是明文。** 在 PHP 里它字面上就是 —— `O:4:"Hook":...` —— 这意味着改载荷里的**一个字符串**就改变了构造哪个类，从而改变跑什么代码。在 Python 里它是一个拼在操作码里的模块名与属性名。**这个载荷是一个带符号表的程序**，而改符号是普通的文本编辑操作。

**那些钩子是当初为别的目的写的普通方法。** `__destruct` 是为了清理。`readObject` 是为了自定义重建。`_load` 是为了自定义格式。没有一个是作为攻击面写的，而它们全部在**应用看到那个对象之前**运行。

最后这点解释了一个否则看起来很怪的行为：**一个应用可以在从未使用它所反序列化的那个对象的情况下就中招。** 钩子在读取过程中就触发了，所以读取之后才做的校验，是做得太晚的校验。

### 为什么常见的修法站不住

#### "它有签名，所以改不了"

签名给的是**完整性**，不是安全，而这两者经常被混为一谈。有三个各自独立的问题：

- **签名无法区分"合法的危险对象"与"伪造的对象"。** 如果类路径里有一个类的清理方法对攻击者有用，应用可以**自己、诚实地**构造出那个对象并给它签名。
- **密钥本身就是攻击面的一部分。** 一个泄漏、共享或默认的密钥，意味着攻击者可以给自己的载荷签名。本系列里 view-state 那篇是同一个失效，只是换了个框架。
- **数据可能确实来自不受信的地方。** 队列消息、缓存条目、会话存储、上传的模型文件、由另一个组件设置的 cookie —— 由应用签名，却被别人影响。

#### "我们过滤类名"

一份"已知危险类"的拒绝清单，列的是**已经被公开过**的东西。有意思的 gadget 按定义就是还没被人写下来的那个，而它只需要**在类路径上** —— 在一个大型应用里，那意味着依赖树，而不是应用自己的代码。一份点明应用真正需要哪些类的允许清单才是可行的版本，而它必须和依赖树一起维护。

#### "我们用了安全的 loader"

这个直觉是对的，而它只和**实际传进去的那个参数**一样好。语言那几篇已经展示过这个形状：`yaml.load` 需要一个 loader 参数，`tarfile.extractall` 需要一个 filter，而两次的默认值都是不安全那个。同样的问题必须对每一个反序列化 API 问一遍：**我什么都不传时，它做什么？**

### 为什么换格式确实能修好

能移除这一整类、而不是过滤它的修法，是移除**"命名一个类型"的能力**：

| 格式 | 它能命名一个可调用对象或类吗 |
|---|---|
| JSON | 不能 —— 对象映射为字典、列表、字符串、数字、布尔、null |
| Protobuf、Avro、带 schema 的 CBOR | 不能，除非 schema 显式编码了类型 |
| Java 序列化、Python `pickle`、PHP `serialize`、Ruby `Marshal`、.NET `BinaryFormatter` | **能** —— 这正是它们的用途 |

这就是全部论证，而且形状和参数化查询、上下文感知编码一样：**不要去过滤危险输入，要移除那条让它变成代码的通道。** 一个无法表达"构造这个类"的格式，无法被说服去构造一个，而无论载荷多有创意都改变不了这一点。

当确实需要一个"表达力强"的格式时 —— 分布式缓存、内部 RPC、模型文件 —— 可行的立场是一个**组合**：能不用带类型命名的格式就不用；用的时候，由运行时强制一份类允许清单，并让数据的来源真正配得上那份信任。

### 检测与缓解

- **对格式签名告警，因为每一个都很有辨识度，而且没有一个该出现在客户端输入里。** Java 序列化以 `AC ED 00 05` 开头，base64 之后就是前缀 **`rO0AB`**；Python pickle 以 `0x80` 加一个协议字节开头；Ruby 的 `Marshal` 以 `04 08` 开头；PHP 的序列化对象形如 `O:` 或 `a:` 后跟长度；.NET 的二进制格式化流与 view state 各有形状。一个参数、cookie 或请求体字段里携带其中任何一个，就是**平台的词汇从外面到达**。
- **对出现在不该出现位置的类型名告警。** 一个请求里的类名，就是"有人试图选择构造什么"的特征 —— 而它常常在错误信息或调用栈里就能看到，即使载荷是编码过的。
- **把反序列化端点上任何含"类找不到"或转换失败的报错当成信号，而不是噪声。** 那些错误意味着输入**指名了一个类型**，而应用试着构造它。同一个来源反复出现同类错误，就是有人在枚举类路径。
- **在数据跨越信任边界的地方替换格式。** 用 JSON，或带 schema 的格式。做不到时，在运行时强制允许清单（Java 的 `ObjectInputFilter`、YAML 的 `safe_load`、其他语言的等价控制），并把那份清单放在**依赖树旁边**，而不是应用自己的包里。
- **把签名密钥当一等机密来保护。** 每个应用各自生成、不入库提交、定期轮换、绝不与处理不可信输入的组件共享。签名只和密钥一样强，而共享的密钥就是共享的"铸造合法载荷"的能力。
- **不要在特权上下文里反序列化。** 如果某个组件必须接受序列化数据，就用尽可能低的权限、在它自己的进程里运行，让那条构造器路径能够到的东西有界。它修不好这个 bug，但它决定事故的大小。
- **并且保持那个框架**：这一类的存在，是因为格式必须说明**要构造什么**。上面每一条缓解要么是"不让输入说构造什么"，要么是"只让它说预先写下来的那些"，要么是"让构造它变得没意思"。第一条是修法；第二条是需要维护的妥协；第三条是损害控制。
