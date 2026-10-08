---
id: deserialization-gadgets
title_en: "Deserialisation, Part 2 — Chains, Gadgets and Why Some Languages Need Them"
title_zh: "反序列化（二）：链、gadget，以及为什么有些语言需要它们"
summary_en: Whether an attacker needs a chain depends on one property of the format — can it name a callable directly, or only a class. This entry answers that per language, builds a working PHP POP chain and a minimal Java gadget, and explains why a deny-list of class names cannot catch a combination.
summary_zh: 攻击者是否需要一条"链"，取决于格式的一个性质 —— 它能不能直接命名一个可调用对象，还是只能命名一个类。这一篇逐语言回答这个问题，搭一条能跑的 PHP POP 链和一个最小的 Java gadget，并说明为什么"类名黑名单"抓不住一个组合。
tags: [web, deserialization, gadget-chain, pop-chain, cwe-502]
tools: [php, java, ysoserial, phpggc, python3]
attck: [T1190, T1059]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The property that decides whether a chain is needed

The previous entry established that a serialisation stream has to say what to build. The follow-up question is more specific and it explains almost everything about how these attacks differ between languages:

> **Can the format name a callable directly, or only a class?**

If it can name a callable, the attacker writes the call into the payload and is done. If it can only name a **class**, the attacker can choose *which code runs during reconstruction* but not *what that code does* — so they must find a class that, when constructed, does something useful. That search is what the word "gadget" refers to, and the assembly of several such classes is a **chain**.

| Format | Can it name a callable? | Consequence |
|---|---|---|
| Python `pickle` | **Yes** — `GLOBAL`/`STACK_GLOBAL` then `REDUCE` | No chain needed; one class with `__reduce__` is the whole exploit |
| PHP `serialize` | No — a class name and its properties | Needs a POP chain |
| Java serialisation | No — class descriptors | Needs a gadget chain |
| Ruby `Marshal` | No — class names | Needs a class whose `_load`/`marshal_load` does something |
| .NET `BinaryFormatter` | No — assembly-qualified type names | Needs a gadget chain |
| JSON | No, and cannot name a type either | Not applicable |

That table is worth keeping, because it converts "which languages have worse deserialisation bugs" into a question about **format design** rather than about language quality.

### The four parts of a chain

The earlier Java entry named three parts; in practice four are worth checking, and each maps to a question to ask about a target.

| Part | What it is | The question |
|---|---|---|
| **Source** | Where untrusted bytes reach the deserialiser | Cookies, view state, queue messages, cache entries, model files, RPC |
| **Trigger** | A hook the runtime calls during reconstruction | Which methods does this language invoke on restore? |
| **Gadget** | A class whose hook does something, which leads to something else | What does that hook call, and with what argument? |
| **Sink** | The operation that matters | Command execution, file write, request, lookup |

A chain needs all four. An endpoint with a source and a trigger but no reachable gadget is not exploitable **yet** — "yet" being the operative word, since the gadget only has to appear on the class path, and class paths grow with every dependency added.

### Building a PHP POP chain

PHP's serialised form carries a class name and the values of its properties. It cannot express "call this function", so the attacker composes classes instead. The technique is called **POP — property-oriented programming**, and the name is accurate: **you do not write code, you arrange properties, and the framework's own code walks the chain for you.**

Three classes are enough to show the shape. The sink is a class whose cleanup method uses a property; the middle is a class whose string conversion calls a method on a property; the entry is a class whose `__wakeup` performs a string operation:

```php
<?php
class Cache {                                   // sink: cleanup uses a controllable value
    public $key = "";
    public $value = "";
    public function __destruct() {
        echo "    3) Cache::__destruct -> 用 key/value 走了一次写操作\n";
        echo "       写入内容: {$this->value}\n";
    }
}
class Report {                                  // middle: conversion calls a method on a property
    public $writer;
    public function __toString() {
        echo "    2) Report::__toString -> 调 writer->write()\n";
        return $this->writer->write();
    }
}
class Prefs {                                   // entry: the hook fires during unserialize
    public $title;
    public function __wakeup() {
        echo "    1) Prefs::__wakeup -> 对 title 做字符串操作\n";
        $x = (string)$this->title;              // triggers __toString
    }
}
class Writer {
    public $note = "";
    public function write() { echo "       Writer::write, note={$this->note}\n"; return "ok"; }
}

// the attacker's entire contribution: arranging properties
$pop = new Prefs();
$pop->title = new Report();
$pop->title->writer = new Writer();
$pop->title->writer->note = "从属性里来的内容";

$payload = serialize($pop);
echo "  载荷: $payload\n\n";
unserialize($payload);                          // the chain walks itself
```

The output payload makes the structure visible, and it is worth reading it as a nesting of properties rather than as data:

```
O:5:"Prefs":1:{s:5:"title";O:6:"Report":1:{s:6:"writer";O:6:"Writer":1:{s:4:"note";s:24:"...";}}}
```

Running it prints the chain walking itself:

```
1) Prefs::__wakeup -> 对 title 做字符串操作
2) Report::__toString -> 调 writer->write()
   Writer::write, note=从属性里来的内容
```

Four observations that generalise:

- **The payload contains no code.** It is class names and property values. Everything that executes was already in the application.
- **The nesting depth is the chain length.** Reading a payload's structure tells you how many hops the author needed.
- **Every hop is a legitimate method.** `__destruct` is cleanup, `__toString` is display, `__wakeup` is restoration. None was written as an attack surface.
- **Real chains are found, not written.** In a framework with hundreds of classes, the search is for a path from a trigger to a sink through existing code. Tooling exists for the common ones — `phpggc` publishes chains for well-known libraries — and its existence is the point: **these chains are discovered in ordinary code.**

### A minimal Java gadget

Java's payload cannot name a method either; it names a **class**, and the class's own `readObject` decides what happens. Here is the smallest faithful demonstration — a class that looks like a preference holder and reconstructs by reflectively invoking something:

```java
public static class Gadget implements Serializable {
    private String methodName;
    private String arg;

    private void readObject(ObjectInputStream in) throws Exception {
        in.defaultReadObject();
        System.out.println("    2) Gadget.readObject -> 反射调用 " + methodName);
        Method m = SomeService.class.getMethod(methodName, String.class);
        m.invoke(null, arg);                    // the "gadget": data names the method
    }
}
```

With a normal object the round trip is unremarkable — the stream starts `AC ED 00 05`, base64 `rO0AB`, and the object comes back:

```
=== 正常对象的往返 ===
  流头 AC ED 00 05  base64 前缀 rO0AB
  还原回来 Prefs(light, )
```

With the class name changed, the hook runs during the read:

```
=== 把类名换成 gadget 之后 ===
  1) 字节流里的类名: GadgetDemo$Gadget
  2) Gadget.readObject -> 反射调用 dangerous
  3) 落点被触达, 参数来自载荷: 来自载荷的参数
  4) 应用拿到对象: GadgetDemo$Gadget@433c675d（它根本没用过这个对象）
```

That fourth line is the one to remember. **The application never used the object it deserialised, and the damage was already done** — because the hook fires during reconstruction, not after it. Any validation performed on the returned object is validation after the fact.

Real Java chains do the same thing with library classes: a `readObject` that calls a transformer, a comparator that invokes a method from the data, a map that triggers a lazy load. That is what `ysoserial` packages — dozens of published paths through `CommonsCollections`, `Spring`, `Groovy` and others.

### Why a deny-list cannot keep up

The structural reason is in the definition: **a chain is a combination of classes, while a deny-list is a list of class names.**

- **New combinations of old classes are new chains.** A list that blocks the classes used in a published chain does not block a different arrangement of the same classes, and does not block a chain that uses a class nobody had thought to combine before.
- **The gadget only has to be on the class path.** It does not have to be reachable from the application's own code, and in a large project the class path is the transitive dependency tree — hundreds of libraries, most of them never inspected for this property.
- **Every dependency added is new chain material.** Which is why "we upgraded the library" fixes a specific published chain and not the class.

The workable defences are therefore about **reducing what can be built** rather than about listing what must not be:

| Defence | Why it works |
|---|---|
| **Allow-list of classes** in the runtime filter | The application names what it needs; everything else is refused, including combinations nobody has published |
| **Shrink the class path** | A library that is not present cannot be a gadget; this is direct, effective, and requires no knowledge of chains |
| **Do not deserialise untrusted data** | The only one that removes the class rather than bounding it |
| **Isolation and least privilege** | Does not fix it; decides what a successful chain can reach |

"Shrink the class path" deserves emphasis because it is the one that is both practical and under-used: **the attack surface here is the set of classes that the reconstructor can construct, and that set is a build-time decision.**

### The gadget landscape by language

| Language | Trigger hooks | Chain needed | Where gadgets come from | Tooling |
|---|---|---|---|---|
| **Python** | `__reduce__`, `__setstate__` | **No** | The payload names the callable | Hand-written |
| **PHP** | `__wakeup`, `__destruct`, `__toString`, `__call` | Yes (POP) | Frameworks and libraries — Laravel, Symfony, Monolog, Guzzle | `phpggc` |
| **Java** | `readObject`, `readResolve`, `validateObject` | Yes | CommonsCollections, Spring, Groovy, Xalan and many more | `ysoserial` |
| **Ruby** | `_load`, `marshal_load`, `init_with` | Yes | Rails and ActiveSupport internals | Hand-written |
| **.NET** | `ISerializable`, `OnDeserialized` | Yes | `ObjectDataProvider`, `TypeConfuseDelegate` | `ysoserial.net` |
| **Node.js** | No native format | Library-dependent | `node-serialize` and similar, which store a function body in the payload | Hand-written |

The Python row is the one that surprises people: **it needs no chain, which makes it strictly easier to exploit than Java** despite Java having the richer ecosystem. A single class with a `__reduce__` is a complete exploit.

### Detection and mitigation

- **Alert on gadget library names in payloads, not only on the format signature.** `org.apache.commons.collections`, `com.sun.org.apache.xalan`, `Monolog`, `GuzzleHttp`, and similar names appearing inside a serialised stream in a request are attempts to choose a class, and the class chosen tells you which chain the author had in mind.
- **Alert on class loading during deserialisation.** A business endpoint reconstructing a settings object has no reason to load a collections library or an XML transformer. Watching **which classes get loaded** on a deserialisation path is a much better detector than watching payload bytes, because it catches chains that have not been published.
- **Treat unusual stream length and structure as a signal.** Gadget chains are long — they carry an object graph rather than a settings object — and a deserialisation endpoint receiving kilobytes where it expects dozens of bytes is worth a look.
- **Watch the process, not only the request.** A chain's purpose is a child process, a file write, or an outbound connection. Process creation from a web worker, DNS lookups to unexpected names, and connections to internal addresses are the observable consequences, and they are the same signals the SSRF and command-injection entries use.
- **For mitigation, reduce the set of constructible classes first.** Allow-list in the runtime filter, and remove dependencies the application does not use. Neither requires knowing any chain, and both shrink the space where chains exist.
- **Treat library upgrades as necessary and not sufficient.** Upgrading closes a published path; it does not change the property that made the path possible.
- **And keep the four-part question in the review checklist**: where does untrusted data reach a deserialiser, which hooks does the runtime call, what can those hooks reach, and where does the reachable path end. Answering those for a stack finds chains that no tool has published.

<!-- lang:zh -->
### 决定"需不需要链"的那个性质

上一篇立起了"序列化流必须说明构造什么"。接着的问题是更具体的，而它几乎解释了这些攻击在各语言之间差别的全部：

> **这个格式能不能直接命名一个可调用对象，还是只能命名一个类？**

如果能命名可调用对象，攻击者把那句调用写进载荷就完事了。如果只能命名一个**类**，攻击者能选择的是*重建过程中跑哪段代码*，而不是*那段代码做什么* —— 于是他必须找一个"被构造时顺手做了有用的事"的类。这个寻找过程就是"gadget"这个词的含义，而把若干个这样的类拼起来就是一条**链**。

| 格式 | 能命名可调用对象吗 | 后果 |
|---|---|---|
| Python `pickle` | **能** —— `GLOBAL`/`STACK_GLOBAL` 然后 `REDUCE` | 不需要链；一个有 `__reduce__` 的类就是完整的利用 |
| PHP `serialize` | 不能 —— 一个类名加它的属性 | 需要 POP 链 |
| Java 序列化 | 不能 —— 类描述符 | 需要 gadget 链 |
| Ruby `Marshal` | 不能 —— 类名 | 需要一个 `_load`/`marshal_load` 里做事的类 |
| .NET `BinaryFormatter` | 不能 —— 带程序集限定的类型名 | 需要 gadget 链 |
| JSON | 也不能命名类型 | 不适用 |

那张表值得留着，因为它把"哪些语言的反序列化 bug 更严重"变成了一个关于**格式设计**的问题，而不是关于语言好坏的问题。

### 一条链的四个部分

前面那篇 Java 里说了三部分；实践中有四个值得逐个检查，而每一个都对应一个要问目标的问题。

| 部分 | 它是什么 | 要问的问题 |
|---|---|---|
| **入口（source）** | 不可信字节到达反序列化器的地方 | Cookie、view state、队列消息、缓存条目、模型文件、RPC |
| **触发（trigger）** | 运行时在重建过程中会调用的钩子 | 这门语言在还原时会调哪些方法？ |
| **gadget** | 它的钩子做了某件事、并引向另一件事的类 | 那个钩子调了什么，用什么参数？ |
| **落点（sink）** | 真正要紧的那个操作 | 命令执行、写文件、发请求、查询 |

一条链四个都要有。一个有入口和触发、但没有可达 gadget 的端点**目前**不可利用 —— "目前"是要紧的词，因为 gadget 只需要出现在类路径上，而类路径会随每一个新增依赖而增长。

### 搭一条 PHP POP 链

PHP 的序列化形式携带一个类名和它属性的值。它无法表达"调用这个函数"，所以攻击者改为**组合类**。这个技术叫 **POP —— 属性导向编程**，而这个名字很准确：**你不写代码，你摆属性，然后框架自己的代码替你把链走完。**

三个类就足以展示形状。落点是一个清理方法用到了属性的类；中间环是一个字符串转换会调用属性上方法的类；入口是一个 `__wakeup` 里做了字符串操作的类：

```php
<?php
class Cache {                                   // 落点：清理方法用了可控的值
    public $key = "";
    public $value = "";
    public function __destruct() {
        echo "    3) Cache::__destruct -> 用 key/value 走了一次写操作\n";
        echo "       写入内容: {$this->value}\n";
    }
}
class Report {                                  // 中间环：转换时调用了属性上的方法
    public $writer;
    public function __toString() {
        echo "    2) Report::__toString -> 调 writer->write()\n";
        return $this->writer->write();
    }
}
class Prefs {                                   // 入口：unserialize 期间触发钩子
    public $title;
    public function __wakeup() {
        echo "    1) Prefs::__wakeup -> 对 title 做字符串操作\n";
        $x = (string)$this->title;              // 触发 __toString
    }
}
class Writer {
    public $note = "";
    public function write() { echo "       Writer::write, note={$this->note}\n"; return "ok"; }
}

// 攻击者的全部贡献：摆属性
$pop = new Prefs();
$pop->title = new Report();
$pop->title->writer = new Writer();
$pop->title->writer->note = "从属性里来的内容";

$payload = serialize($pop);
echo "  载荷: $payload\n\n";
unserialize($payload);                          // 链会自己走起来
```

产出的载荷让结构变得可见，而它值得当成"属性的嵌套"来读，而不是当成数据：

```
O:5:"Prefs":1:{s:5:"title";O:6:"Report":1:{s:6:"writer";O:6:"Writer":1:{s:4:"note";s:24:"...";}}}
```

跑起来会打印链自己行走的过程：

```
1) Prefs::__wakeup -> 对 title 做字符串操作
2) Report::__toString -> 调 writer->write()
   Writer::write, note=从属性里来的内容
```

四个能推广的观察：

- **载荷里没有代码。** 它是类名和属性值。真正执行的一切早就在应用里了。
- **嵌套深度就是链长。** 读一个载荷的结构，就能知道作者需要几跳。
- **每一跳都是一个正当方法。** `__destruct` 是清理，`__toString` 是显示，`__wakeup` 是还原。没有一个是作为攻击面写的。
- **真实的链是被"找到"的，不是被写出来的。** 在一个有几百个类的框架里，搜索的就是一条从触发到落点、穿过既有代码的路径。常见的那批有工具 —— `phpggc` 公开发布了知名库的链 —— 而它的存在本身就是重点：**这些链是在普通代码里被发现的。**

### 一个最小的 Java gadget

Java 的载荷同样不能命名方法；它命名一个**类**，而那个类自己的 `readObject` 决定发生什么。下面是最小的忠实演示 —— 一个看起来像偏好容器的类，靠反射调用某个东西来完成重建：

```java
public static class Gadget implements Serializable {
    private String methodName;
    private String arg;

    private void readObject(ObjectInputStream in) throws Exception {
        in.defaultReadObject();
        System.out.println("    2) Gadget.readObject -> 反射调用 " + methodName);
        Method m = SomeService.class.getMethod(methodName, String.class);
        m.invoke(null, arg);                    // "gadget"：由数据决定调哪个方法
    }
}
```

用正常对象时往返平平无奇 —— 流以 `AC ED 00 05` 开头、base64 是 `rO0AB`，对象原样回来：

```
=== 正常对象的往返 ===
  流头 AC ED 00 05  base64 前缀 rO0AB
  还原回来 Prefs(light, )
```

把类名换掉之后，钩子在读取过程中就跑了：

```
=== 把类名换成 gadget 之后 ===
  1) 字节流里的类名: GadgetDemo$Gadget
  2) Gadget.readObject -> 反射调用 dangerous
  3) 落点被触达, 参数来自载荷: 来自载荷的参数
  4) 应用拿到对象: GadgetDemo$Gadget@433c675d（它根本没用过这个对象）
```

第四行是要记住的。**应用从未使用它反序列化出来的那个对象，而损害已经发生** —— 因为钩子在**重建过程中**触发，不是之后。任何对返回对象做的校验，都是事后校验。

真实的 Java 链用库里的类做同样的事：一个 `readObject` 调用某个 transformer、一个比较器从数据里取方法名去调用、一个 map 触发一次惰性加载。`ysoserial` 打包的就是这些 —— 几十条穿过 `CommonsCollections`、`Spring`、`Groovy` 等库的公开路径。

### 为什么黑名单跟不上

结构性的原因就写在定义里：**链是类的组合，而黑名单是类名的清单。**

- **老类的新组合就是新链。** 一份"封掉某条已公开链所用类"的清单，既不封同一个类的另一种摆法，也不封一条用了从没人想过要组合的类的链。
- **gadget 只需要在类路径上。** 它不需要从应用自己的代码可达，而在一个大项目里，类路径就是传递依赖树 —— 几百个库，其中大多数从没被按这个性质检查过。
- **每加一个依赖都是新的链素材。** 这就是为什么"我们升级了那个库"修的是一条具体的公开链，而不是这个类。

所以可行的防御都是关于**减少"能被构造的东西"**，而不是关于"列出不许构造的东西"：

| 防御 | 为什么有效 |
|---|---|
| 运行时过滤器里的**类允许清单** | 应用点名它需要的；其余一律拒绝，包括没人公开过的组合 |
| **给类路径瘦身** | 不在场的库不可能成为 gadget；这直接、有效，而且不需要知道任何链 |
| **不反序列化不可信数据** | 唯一一个移除这一类、而不是围住它的 |
| **隔离与最小权限** | 修不好它；决定一条成功的链能够到什么 |

"给类路径瘦身"值得强调，因为它是既实用又被低估的那一条：**这里的攻击面是"重建器能构造哪些类"这个集合，而那个集合是构建期的决定。**

### 各语言的 gadget 地形

| 语言 | 触发钩子 | 需要链吗 | gadget 从哪来 | 工具 |
|---|---|---|---|---|
| **Python** | `__reduce__`、`__setstate__` | **不需要** | 载荷自己命名可调用对象 | 手写 |
| **PHP** | `__wakeup`、`__destruct`、`__toString`、`__call` | 需要（POP） | 框架与库 —— Laravel、Symfony、Monolog、Guzzle | `phpggc` |
| **Java** | `readObject`、`readResolve`、`validateObject` | 需要 | CommonsCollections、Spring、Groovy、Xalan 等很多 | `ysoserial` |
| **Ruby** | `_load`、`marshal_load`、`init_with` | 需要 | Rails 与 ActiveSupport 内部 | 手写 |
| **.NET** | `ISerializable`、`OnDeserialized` | 需要 | `ObjectDataProvider`、`TypeConfuseDelegate` | `ysoserial.net` |
| **Node.js** | 无原生格式 | 取决于库 | `node-serialize` 之类，把函数体存进载荷 | 手写 |

Python 那一行是最让人意外的：**它不需要链，这让它比 Java 更容易利用**，尽管 Java 有更丰富的生态。一个带 `__reduce__` 的类就是一次完整的利用。

### 检测与缓解

- **对载荷里的 gadget 库名告警，而不只是对格式签名。** `org.apache.commons.collections`、`com.sun.org.apache.xalan`、`Monolog`、`GuzzleHttp` 之类的名字出现在请求里的序列化流中，就是有人在选择类，而**被选中的类会告诉你作者心里想的是哪条链**。
- **对反序列化过程中的类加载告警。** 一个重建设置对象的业务端点，没有理由去加载一个集合库或一个 XML 转换器。盯**反序列化路径上加载了哪些类**，是比盯载荷字节更好的检测器，因为它能抓到**尚未被公开**的链。
- **把异常的流长度与结构当信号。** gadget 链很长 —— 它携带的是一个对象图，而不是一个设置对象 —— 一个期待几十字节的反序列化端点收到了几 KB，值得看一眼。
- **盯进程，而不只是盯请求。** 一条链的目的是子进程、写文件或一次出网连接。Web 工作进程创建子进程、向意外域名发起 DNS 查询、连到内网地址 —— 这些都是可观察的后果，而且和 SSRF、命令注入那两篇用的是同一批信号。
- **缓解上，先减少"可被构造的类"这个集合。** 运行时过滤器里放允许清单，并移除应用用不到的依赖。两者都不需要知道任何链，而两者都会缩小"链存在"的空间。
- **把库升级当作必要而不充分。** 升级关掉的是一条已公开的路径；它不改变让那条路径得以存在的性质。
- **并且把那个四部分的问题留在评审清单里**：不可信数据在哪里到达反序列化器、运行时会调哪些钩子、那些钩子能够到什么、那条可达路径在哪里结束。对一套技术栈回答完这些，就能找到没有任何工具公开过的链。
