---
id: prototype-pollution-fundamentals
title_en: "Prototype Pollution — Writing to the Layer Every Object Shares"
title_zh: "原型污染：写进所有对象共享的那一层"
summary_en: A merge that walks into __proto__ does not set a value on one object, it changes the default behaviour of every object in the process. Measured in Node, with the contrast that Python and Go have no such layer to write to.
summary_zh: 一个走进 __proto__ 的合并操作，不是在某一个对象上设了一个值，而是改了这个进程里每一个对象的默认行为。这一篇在 Node 上实测，并给出对照：Python 与 Go 根本没有那一层可写。
tags: [web, prototype-pollution, javascript, nodejs, cwe-1321, mass-assignment]
tools: [node, python3]
attck: [T1190, T1059.007]
platform: [web, nodejs]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### One key that is not like the others

JavaScript resolves a property by walking a chain: the object's own properties, then its prototype, then that prototype's prototype. Objects created the same way share a prototype, so a property found there is a property **every one of them** appears to have.

`__proto__` is a key that reaches that shared layer. Feed it to a merge routine and the routine does what it was written to do — it assigns a nested value under a key — except the key is not a data key:

> **The merge does not set a value on the object it was given. It sets a default for every object of that type in the process.**

Measured in Node, merging a payload into an empty object and then looking at unrelated objects:

| After the merge | Result |
|---|---|
| A brand-new empty object `a`: `a.isAdmin` | **true** |
| An unrelated object `b = {name: "bob"}`: `b.isAdmin` | **true** |
| `a.theme` and `b.theme`, from a `theme` key in the same payload | **`pwned`** for both |
| `Object.keys(a).length` | **0** — the property is not among the object's own keys |

That last row is the one that makes this class hard to see. **The polluted property does not appear in the object's own keys**, so `Object.keys`, `JSON.stringify` and a debugger's property list do not show it. It is found only at the end of a property lookup — which is precisely where application code looks when it asks `if (user.isAdmin)`.

### Four layers

1. **The trust boundary.** The application trusts that the keys in a merged object will land on that object. The merge routine does what it was told; the routing of `__proto__` to a shared destination is a language behaviour, not a mistake in the routine.
2. **Data and instruction share a plane.** A key name is simultaneously **which field to write** and **which object to write it on**. `__proto__` is a key whose second meaning is "the shared layer" — and a merge cannot tell a data key from that one.
3. **Why the usual fix fails.** Filtering `__proto__` at the entry point handles the shallow case and misses the nested one. Measured: a top-level `__proto__` was blocked by a check on the incoming keys, while `{"a": {"__proto__": {"isAdmin": true}}}` was not — because the recursion reached the inner object without re-applying the check at that level.
4. **The variants.** `__proto__` directly and nested, `constructor.prototype`, array-index paths, and the same object arriving through a query parser rather than a JSON body — the query-string parser is a separate merge implementation with its own opinion about keys.

### `JSON.parse` is not the problem; the merge is

This is the most common reason a project believes it is not affected, and it is measurable in two lines:

```
after JSON.parse('{"__proto__":{"x":1}}')   ->  ({}).x is undefined
after merging that same object into {}      ->  ({}).x is 1
```

`JSON.parse` produces an ordinary object whose own keys include the string `__proto__`; nothing has been polluted. **It is the recursive merge — the routine that copies a source's own keys onto a target — that sends that key to the prototype.** So "we parse the body with `JSON.parse`" is not a statement about safety; it describes where the data came from, not where it goes.

The corollary matters for libraries: any function that takes a configuration object and deep-merges it into defaults is a candidate, and the attacker's contribution may arrive as a query parameter, a header or a stored preference rather than a JSON body.

### `constructor.prototype`, and why one path is worth trying over another

Measured with a straightforward recursive merge, the `__proto__` path polluted a new object reliably, while the payload `{"constructor": {"prototype": {"isAdmin": true}}}` did **not** — because the target's `constructor` is a function, the routine replaced it with a plain object rather than walking into it.

That is not a reason to skip the variant; it is a reason to state the difference honestly:

| Path | Depends on |
|---|---|
| `__proto__` | Almost nothing — it is a language-level accessor on every ordinary object |
| `constructor.prototype` | **How the merge treats a key whose current value is a function** |
| Nested `__proto__` | Whether the check is reapplied at every level of the recursion |

**The first is the one that nearly always works; the others are the ones that work when the implementation has a particular shape** — which is exactly why they are worth trying against a specific target and why writing them up as though they always succeed leads to a payload list that fails half the time for reasons the reader cannot see.

### The consequence depends on who reads the property

Prototype pollution is not an outcome in itself, and that is why it is under-weighted in reviews:

> **It changes a default. What that is worth depends entirely on which code reads the default.**

Three shapes worth looking for, in increasing order of severity:

| Reader | What the polluted default does |
|---|---|
| **An authorisation check** | `if (user.isAdmin)` — a property that now exists on every object and is truthy |
| **A template engine or serializer** | Reads properties from the object it is rendering, including inherited ones, which turns the pollution into an XSS entry's payload |
| **A process-spawning call** | In Node, an options object read by the child-process API — a polluted `shell`, `env` or argument field turns a merge into command execution |

**So the review question is not "can this be polluted" but "who reads that property afterwards"** — and, for a library, which of its colleagues does. That is why a prototype pollution finding in a widely used package is severe: everything downstream that merges the same object also sees the polluted default.

### The languages that do not have this

| Language | Affected | Why |
|---|---|---|
| **JavaScript** | **yes** | The prototype chain is the language's inheritance mechanism, and it is mutable at runtime |
| **Python** | no | Instance attributes live in the instance's own `__dict__`; class attributes live on the class |
| **Go** | no | Struct fields are fixed at compile time; there is no dynamic property layer |
| **Java** | no | Class members are fixed at compile time |
| **Ruby, PHP** | partially | Open classes and dynamic property assignment exist, but they are that language's own feature rather than a shared inherited layer reached by a data key |

Measured in Python, merging `{"__proto__": {...}, "role": "admin"}` into one instance:

```
u1.role       -> admin          (that instance was updated)
u2.role       -> user           (the other instance was not)
u2.__dict__   -> {}             (no shared layer was written to)
```

**In Python, `__proto__` is a plain string key with no special meaning**, and there is no layer that a merge can reach which other instances read from. Affecting every instance requires writing to the class explicitly — `setattr(SomeClass, ...)` — which is application code doing something deliberate rather than a side effect of a merge.

**The rule to carry across stacks:** prototype pollution is specific to JavaScript because "an object inherits from another object" is that language's mechanism. The nearest equivalents elsewhere — assigning to a class in Python, reopening a class in Ruby — are each language's own problem with its own fix, and treating them as the same finding produces payloads that do not apply.

### Detection and mitigation

- **Alert when `__proto__`, `constructor` or `prototype` appears as a key anywhere in a request body or query string.** At any depth, in any casing, in any encoding. There is no legitimate reason for a client to send these as field names, and the nested case is the one that the obvious filters miss.
- **Assert the invariant rather than watching for the attack.** A startup check that `Object.prototype` has no unexpected own properties, and a runtime assertion after any merge in a test, catches this class from the inside — and it catches the library you did not know was vulnerable.
- **And watch the readers.** Authorisation decisions, template rendering and child-process invocation are the three places where a polluted default becomes an outcome; telemetry on those three is smaller than telemetry on every merge.
- **For mitigation, make the merged container have no prototype.** `Object.create(null)`, or a `Map`, means `__proto__` is an ordinary string key with nowhere special to go — which removes the class rather than filtering it. This is the control that is complete by construction.
- **Do not write the recursion yourself, and do not assume a well-known helper is safe.** `Object.assign` performs a `[[Set]]` on the target, which triggers the same accessor; a deep merge from a utility library is exactly the code whose handling of a function-valued key decides whether `constructor.prototype` works. Prefer a merge that takes an explicit field list, or a parser that produces null-prototype objects.
- **Freeze the prototype where the application permits it.** `Object.freeze(Object.prototype)` makes the shared layer read-only, so any attempt to pollute it fails loudly — and it is worth testing against the dependencies, because a library that assigns to the prototype legitimately will break.
- **Validate the shape, not just the keys.** A schema that enumerates the accepted fields rejects `__proto__` and everything like it as a side effect, and it is the same allowlist argument as the mass assignment entry — of which this is the JavaScript-specific variant, arriving through the same binding.
- **And when fixing a dependency, check what it was merged into.** A polluted process keeps the default until it restarts, and in a long-running server that can be hours of a state that no deployment removed.

<!-- lang:zh -->
### 一个与其他键不同的键

JavaScript 解析一个属性的方式是沿着一条链走：对象自己的属性，然后它的原型，然后原型的原型。用同样方式创建出来的对象共享一个原型，所以在那里找到的属性，是**它们每一个**看起来都拥有的属性。

`__proto__` 是一个能到达那个共享层的键。把它喂给一个合并例程，那个例程会做它被写出来要做的事 —— 在一个键下面赋一个嵌套的值 —— 只不过那个键不是一个数据键：

> **这次合并不是给它拿到的那个对象设了一个值。它给这个进程里那个类型的每一个对象设了一个默认值。**

在 Node 上实测，把一个载荷合并进一个空对象，然后去看无关的对象：

| 合并之后 | 结果 |
|---|---|
| 一个全新的空对象 `a`：`a.isAdmin` | **true** |
| 一个无关对象 `b = {name: "bob"}`：`b.isAdmin` | **true** |
| 同一个载荷里 `theme` 键带来的 `a.theme` 与 `b.theme` | 两者都是 **`pwned`** |
| `Object.keys(a).length` | **0** —— 那个属性不在对象自己的键里 |

最后一行才是让这一类难看出来的东西。**被污染出来的属性不出现在对象自己的键里**，所以 `Object.keys`、`JSON.stringify` 和调试器的属性列表都不显示它。它只在一次属性查找的终点被找到 —— 而那恰好是应用代码问 `if (user.isAdmin)` 时会去看的地方。

### 四层

1. **信任边界。** 应用信任"被合并的对象里的键会落在那个对象上"。合并例程做了它被告知要做的事；把 `__proto__` 路由到一个共享目的地是语言行为，不是那个例程的错误。
2. **数据与指令共用同一平面。** 一个键名同时是**要写哪个字段**和**要写到哪个对象上**。`__proto__` 是一个第二个含义为"那个共享层"的键 —— 而一次合并分不清数据键和它。
3. **为什么常见修法失败。** 在入口处过滤 `__proto__` 能处理浅层的情况、漏掉嵌套的。实测：一个顶层的 `__proto__` 被"检查进来的键"挡住了，而 `{"a": {"__proto__": {"isAdmin": true}}}` 没有 —— 因为递归到达内层对象时没有再在那个层级施加检查。
4. **变体。** 直接的与嵌套的 `__proto__`、`constructor.prototype`、数组下标路径，以及同一个对象经由查询解析器而不是 JSON 请求体到达 —— 查询串解析器是另一套合并实现，对键有它自己的意见。

### `JSON.parse` 不是问题，合并才是

这是项目认为自己不受影响最常见的原因，而它在两行里就能测出来：

```
JSON.parse('{"__proto__":{"x":1}}') 之后   ->  ({}).x 是 undefined
把同一个对象合并进 {} 之后                  ->  ({}).x 是 1
```

`JSON.parse` 产生的是一个普通对象，它自己的键里含字符串 `__proto__`；还没有任何东西被污染。**是那个递归合并 —— 把来源自身的键拷到目标上的例程 —— 把那个键送上了原型。** 所以"我们用 `JSON.parse` 解析请求体"不是一句关于安全的声明；它描述的是数据从哪里来，而不是它到哪里去。

一个推论对库很重要：任何接受一个配置对象、把它深合并进默认值的函数都是候选，而攻击者的贡献可能以查询参数、请求头或者一条已存的偏好设置的形式到达，而不是 JSON 请求体。

### `constructor.prototype`，以及为什么一条路径比另一条更值得试

用一个直白的递归合并实测，`__proto__` 那条路稳定地污染了一个新对象，而载荷 `{"constructor": {"prototype": {"isAdmin": true}}}` **没有** —— 因为目标的 `constructor` 是一个函数，那个例程把它替换成了一个普通对象，而没有走进去。

那不是跳过这个变体的理由；它是把差别如实说出来的理由：

| 路径 | 取决于 |
|---|---|
| `__proto__` | 几乎不取决于什么 —— 它是每个普通对象上的一个语言级访问器 |
| `constructor.prototype` | **合并怎么处理"当前值是一个函数的键"** |
| 嵌套的 `__proto__` | 检查是否在递归的每一层被重新施加 |

**第一条几乎总会成功；其余的是"当实现具有某种形状时"才会成功** —— 而这正是它们值得对着一个具体目标去试的原因，也是把它们写成"总能成功"会导致一份有一半时候会失败、而读者看不出原因的 payload 清单的原因。

### 后果取决于谁去读那个属性

原型污染本身不是一种结果，这就是它在评审里被低估的原因：

> **它改的是一个默认值。那个默认值值多少，完全取决于哪段代码去读它。**

三种值得去找的读者，按严重性递增：

| 读者 | 被污染的默认值做了什么 |
|---|---|
| **一个授权检查** | `if (user.isAdmin)` —— 一个现在存在于每个对象上、而且为真的属性 |
| **一个模板引擎或序列化器** | 从它正在渲染的对象上读属性，包括继承来的，于是污染变成了 XSS 那篇的 payload |
| **一次起进程的调用** | 在 Node 里，子进程 API 读的一个选项对象 —— 一个被污染的 `shell`、`env` 或参数字段，把一次合并变成了命令执行 |

**所以评审的问题不是"这能不能被污染"，而是"之后谁去读那个属性"** —— 而对一个库来说，是它的哪些同伴会读。这就是为什么一个被广泛使用的包里的原型污染发现是严重的：下游每一个合并同一个对象的东西，也会看到那个被污染的默认值。

### 没有这一层的那些语言

| 语言 | 受影响 | 原因 |
|---|---|---|
| **JavaScript** | **是** | 原型链就是那门语言的继承机制，而它在运行时可变 |
| **Python** | 否 | 实例属性在实例自己的 `__dict__` 里；类属性在类上 |
| **Go** | 否 | 结构体字段在编译期固定；没有动态属性层 |
| **Java** | 否 | 类成员在编译期固定 |
| **Ruby、PHP** | 部分 | 开放类与动态属性赋值存在，但那是那些语言自己的特性，而不是一个由数据键到达的、共享的继承层 |

在 Python 里实测，把 `{"__proto__": {...}, "role": "admin"}` 合并进一个实例：

```
u1.role       -> admin          （那个实例被更新了）
u2.role       -> user           （另一个实例没有被影响）
u2.__dict__   -> {}             （没有任何共享层被写到）
```

**在 Python 里，`__proto__` 是一个没有特殊含义的普通字符串键**，而且没有任何一层是"一次合并能到达、而其他实例会去读"的。要影响每一个实例，需要显式地写到类上 —— `setattr(SomeClass, ...)` —— 那是应用代码在做一件刻意的事，而不是一次合并的副作用。

**跨技术栈该带走的那条规则：** 原型污染是 JavaScript 特有的，因为"一个对象从另一个对象继承"是那门语言的机制。其他地方最接近的东西 —— 在 Python 里给类赋值、在 Ruby 里重新打开一个类 —— 是各自语言自己的问题、各有各的修法，而把它们当作同一条发现会产生根本不适用的 payload。

### 检测与缓解

- **当 `__proto__`、`constructor` 或 `prototype` 作为键出现在请求体或查询串的任何位置时告警。** 任何深度、任何大小写、任何编码。客户端没有正当理由把这些当字段名发过来，而嵌套那种正是显而易见的那几个过滤器会漏掉的。
- **断言不变式，而不是盯着攻击。** 一个启动检查，确认 `Object.prototype` 上没有预期之外的自有属性；以及在测试里，任何一次合并之后加一条运行时断言 —— 这从内部抓住这一类，而且它抓住的是你并不知道有问题的那个库。
- **并且盯住读者。** 授权判断、模板渲染与子进程调用，是"被污染的默认值变成一个结果"的三个地方；对这三处的遥测比"对每一次合并"更小。
- **缓解上，让被合并的容器没有原型。** `Object.create(null)`，或者一个 `Map`，意味着 `__proto__` 只是一个普通的字符串键、没有特殊的地方可去 —— 这移除的是这一类，而不是过滤它。这是按构造就完整的那项控制。
- **不要自己写那个递归，也不要假设某个知名助手是安全的。** `Object.assign` 在目标上执行 `[[Set]]`，会触发同一个访问器；来自某个工具库的深合并，恰恰就是"它怎么处理一个函数值的键"决定了 `constructor.prototype` 成不成立的那段代码。优先用接受一份显式字段清单的合并，或者一个产生无原型对象的解析器。
- **在应用允许的地方冻结原型。** `Object.freeze(Object.prototype)` 让那个共享层变成只读，于是任何污染它的尝试都会响亮地失败 —— 而它值得对着依赖测一遍，因为一个正当地给原型赋值的库会因此坏掉。
- **校验形状，而不仅是键。** 一份枚举可接受字段的 schema 会把 `__proto__` 以及和它同类的东西当副作用一起拒掉，而这和批量赋值那篇是同一条允许清单的论证 —— 这一篇就是它在 JavaScript 里的变体，经由同一个绑定到达。
- **而在修一个依赖时，检查它被合并进了什么。** 一个被污染的进程会一直带着那个默认值直到重启，而在一个长期运行的服务里，那可能是"没有任何一次部署能移除"的几个小时。
