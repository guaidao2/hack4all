---
id: lang-python-ruby-node-go
title_en: "Python, Ruby, Node and Go — Dynamic Dispatch, Prototypes and Safety by Design"
title_zh: "Python、Ruby、Node 与 Go：动态派发、原型与设计上的安全"
summary_en: Four runtimes with four different failure modes. Python and Ruby hand you deserialisation and dynamic dispatch, Node shares one writable prototype across every object, and Go's type system closes most of these doors outright. This entry covers each, then one table of the safe API beside the convenient one for every language in this series.
summary_zh: 四种运行时，四种不同的失效方式。Python 与 Ruby 把反序列化和动态派发直接交到你手上，Node 让所有对象共享一个可写的原型，而 Go 的类型系统把上面这些门大部分直接关上了。这一篇逐个讲，最后给出本系列每种语言的"安全 API 与便利 API"对照表。
tags: [web, python, ruby, nodejs, golang, deserialization-prep, prototype-pollution]
tools: [python3, ruby, node, go, ysoserial]
attck: [T1190, T1059]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Four runtimes, four failure modes

The previous three entries covered stacks where the language itself supplies the attack surface. These four are more varied, and the differences are instructive:

| Runtime | The property that decides its bug profile |
|---|---|
| **Python** | Everything is an object, and `pickle` is willing to rebuild any of them — no gadget hunt required |
| **Ruby** | Dynamic dispatch is idiomatic (`send`), and `Marshal`/`YAML` reconstruct objects |
| **Node.js** | One prototype object is shared by every object, **and it is writable from data** |
| **Go** | Static types, explicit errors, no implicit conversion — most of the above simply cannot be expressed |

Go's presence in this entry is deliberate: seeing what a runtime *without* these properties looks like is what tells you which properties actually matter.

### Python

#### `pickle` is a code-execution format

Unpickling is not parsing. The format has an opcode for "call this callable with these arguments", and a class defines it through `__reduce__`:

```python
import pickle, os, base64

class Exploit:
    def __reduce__(self):
        # the callable and its arguments are stored in the stream and invoked on load
        return (os.system, ('echo pickle-executed',))

payload = pickle.dumps(Exploit())
print(base64.b64encode(payload)[:24].decode())
print(payload[:2].hex())          # 8004 for protocol 4, the default in Python 3.8+

pickle.loads(payload)             # prints: pickle-executed
```

Note what is **not** here: there is no gadget chain to find, no classpath to search, no version to match. The attacker writes the callable into the payload, because the format has a field for it. **That is the difference between Python's deserialisation risk and Java's**: Java needs a gadget that already exists on the classpath; Python needs one line.

`copyreg`-registered types, `__setstate__`, `__reduce_ex__` and the persistent-id mechanism are all further hooks, but the point stands: **loading a pickle is running a program that the pickle chose.**

The alternative is a format with no such concept:

```python
import json
data = json.loads(untrusted)      # can produce dicts, lists, strings, numbers, booleans, null
```

JSON cannot name a callable, so it cannot be talked into calling one. That is why "use JSON" is the fix rather than "be careful with pickle".

The signature is also a detection gift: a pickle stream begins with `0x80` followed by a protocol byte (`0x04` for protocol 4, `0x05` for 5), and base64 of that is recognisable at the start of a value.

#### YAML, and the loader that matters

`yaml.load` historically instantiated arbitrary Python objects from tags in the document:

```yaml
!!python/object/apply:os.system ["echo yaml-executed"]
```

PyYAML changed the default so that a `Loader` argument is required, and `FullLoader` still permits some construction. The only safe choice for untrusted input is explicit:

```python
import yaml
yaml.safe_load(untrusted)         # only the basic types, no object construction
```

The general rule across languages: **the loader determines the capability, not the format.** A YAML parser configured for "safe" types is safe; the same library in full mode is not.

#### Dynamic dispatch and formatting

| Feature | The risk |
|---|---|
| `eval`, `exec` | Executes a string; treat as equivalent to running user code |
| `getattr(obj, name)` | A string chooses which function is called — the analogue of Ruby's `send` and of reflection in Java |
| `os.system`, `subprocess` with `shell=True` | A string is handed to a shell, so shell metacharacters in it are interpreted |
| `str.format` and f-strings | `"{0.__class__}".format(obj)` reaches attributes; a user-controlled format string is an information-disclosure primitive |
| `assert` | **Removed when Python runs with `-O`** — never use it for an authorisation check |

That last row is worth its own line: an application that relies on `assert user.is_admin` for a security decision has that decision **compiled out** in optimised mode. Python documents this; it is easy to miss because the code reads like a check.

#### Paths

Two functions that look interchangeable and are not:

```python
import os.path, posixpath
print(posixpath.commonprefix(['/var/www/uploads-a', '/var/www/uploads-b']))
# /var/www/uploads      <- a string prefix, not a directory
print(posixpath.commonpath(['/var/www/uploads-a', '/var/www/uploads-b']))
# /var/www              <- the actual shared directory
```

`commonprefix` is a **character-wise** comparison, so it happily returns a prefix that is not a path boundary. `commonpath` is the one that understands directories. The same distinction — string prefix versus path boundary — is why a containment check needs a separator appended, as shown in the opening entry of this series.

And the join behaviour to remember:

```python
print(posixpath.join('/var/www/uploads', 'shell.php'))    # /var/www/uploads/shell.php
print(posixpath.join('/var/www/uploads', '/etc/passwd'))  # /etc/passwd  <- absolute wins
```

### Ruby

#### `Marshal` and `YAML` reconstruct objects

```ruby
Marshal.load(untrusted)     # reconstructs Ruby objects; the header is \x04\x08
YAML.load(untrusted)        # historically instantiated arbitrary classes
```

`Marshal.load` has no safe mode with respect to object construction — the only safe use is on data you produced. `YAML.load` was the vector for a well-known Rails vulnerability, and the modern Ruby default changed: Psych 4 (Ruby 3.1+) makes `YAML.load` behave like `safe_load`. The lesson is again about versions and loaders rather than about the format.

#### Dynamic dispatch as a language feature

Ruby's `send` invokes a method named by its argument, including private methods:

```ruby
obj.send(params[:method])              # a request parameter chooses the method
send(:system, params[:cmd])            # and with the right arguments, that is execution
```

`public_send` avoids private methods and is the smaller surface. The idiom itself is not a bug — it is how Ruby metaprogramming works — but a parameter reaching `send` is the same event as a string reaching `getattr` or `Class.forName`.

Related features that execute or reach further than they look:

| Feature | Behaviour |
|---|---|
| `eval`, `instance_eval`, `class_eval` | Evaluate a string as Ruby code |
| Backticks and `%x{}` | Run a shell command |
| `Kernel#open` with a leading `\|` | Historically opens a pipe to a command rather than a file |
| `method_missing` | Handles calls to undefined methods; makes "what does this object respond to" non-obvious |
| String interpolation `#{}` | Evaluates its content, so a string built from input can contain code |

### Node.js

#### One prototype, shared and writable

This is the property no other runtime in this series has. Every object inherits from `Object.prototype`, and JavaScript lets data reach that chain:

```javascript
const obj = {};
obj.__proto__.polluted = "yes";        // writes onto Object.prototype
console.log({}.polluted);              // "yes"   <- every object now has it
```

The dangerous shape is not the direct write — it is the **recursive merge**, which almost every Node application has somewhere:

```javascript
// the vulnerable pattern: a deep merge that trusts its keys
function merge(target, source) {
  for (const key in source) {
    if (typeof source[key] === "object" && source[key] !== null) {
      target[key] = target[key] || {};
      merge(target[key], source[key]);
    } else {
      target[key] = source[key];
    }
  }
  return target;
}

merge({}, JSON.parse('{"__proto__": {"polluted": "yes"}}'));
console.log({}.polluted);
```

When `key` is `__proto__`, the recursion walks onto the prototype of the target and writes there. Now every object in the process carries the attacker's property — and the escalation depends on **which property** they chose:

| Polluted property | Consequence |
|---|---|
| A property the application reads for a decision | Logic bypass, privilege checks defeated |
| `shell`, `env`, `argv0` on the options of a `child_process` call | Command execution, if the library reads options from an object that inherits the polluted value |
| `NODE_OPTIONS` in an environment object | Code execution on the next spawned process |
| Template or serialisation engine options | Whatever that engine does with them |

**`JSON.parse` itself is safe** on this point, which is worth knowing precisely, because it changes where the bug is:

```javascript
const parsed = JSON.parse('{"__proto__": {"polluted": "yes"}}');
console.log({}.polluted);                 // undefined — JSON.parse sets an own property
console.log(Object.getOwnPropertyNames(parsed).includes("__proto__"));   // true
```

`JSON.parse` creates an **own** property named `__proto__` rather than writing to the prototype. The pollution happens when that object is then **merged, copied or assigned field by field** into something else — which is why the vulnerability is always in the library or the helper, never in `JSON.parse`.

Defences, in order of strength:

```javascript
Object.freeze(Object.prototype);                        // make the chain immutable
const safe = Object.create(null);                       // no prototype at all
// and when copying, refuse the dangerous keys
const BLOCKED = new Set(["__proto__", "constructor", "prototype"]);
for (const k of Object.keys(src)) if (!BLOCKED.has(k)) dst[k] = src[k];
```

#### Other Node surfaces

| Feature | The risk |
|---|---|
| `child_process.exec` | Runs through a shell; `execFile` and `spawn` without a shell do not |
| `eval`, `new Function`, `vm` | Code execution; the `vm` module is **not** a security boundary |
| `require(variable)`, dynamic `import()` | A string chooses which module loads |
| Deserialisation libraries with `serialize`/`unserialize` | Equivalent to `pickle` in reach |
| Prototype-dependent options | The escalation path above |

### Go

#### Static typing closes most of this

Go has no implicit conversion, no `==` surprise for boxed types, no method that reconstructs an arbitrary object from bytes, and no prototype chain. A review of a Go application is a review of **logic, templates and paths**, and the list is short:

| Area | The Go-specific concern |
|---|---|
| Templates | `text/template` performs **no escaping**; `html/template` performs context-aware escaping. Using the former for HTML is the bug |
| Command execution | `exec.Command(name, args...)` does **not** use a shell, so argument values cannot inject. Reintroducing `sh -c` reintroduces the problem |
| Paths | `filepath.Join` **cleans** the result, which is safer than Python's `join` |
| Deserialisation | `encoding/json` cannot name a type; `encoding/gob` reconstructs types but does not invoke arbitrary code the way `pickle` does |
| Integer arithmetic | Widths are explicit, but overflow still **wraps silently** for variables |
| `unsafe` | Explicitly named, and its presence is a review trigger |

The path difference is worth seeing, because it is the opposite of the Python behaviour above:

```go
package main

import (
	"fmt"
	"path/filepath"
)

func main() {
	fmt.Println(filepath.Join("/var/www/uploads", "shell.php"))        // /var/www/uploads/shell.php
	fmt.Println(filepath.Join("/var/www/uploads", "/etc/passwd"))      // /var/www/uploads/etc/passwd
	fmt.Println(filepath.Join("/var/www/uploads", "../../etc/passwd"))  // /var/etc/passwd
}
```

Note the second line: **the absolute second argument does not replace the base** — Go cleans and keeps it inside. That is a real difference from `os.path.join` in Python, and it is an example of a language choosing the safer default. The third line shows what is still left to handle: `..` **is** resolved, so a traversal can still escape the base, which is why the containment check (resolve, then compare with a separator) is still required.

`html/template` versus `text/template` deserves emphasis because it is the one Go mistake that produces a classic web vulnerability:

```go
import "text/template"     // no escaping: dropping user data into HTML here is XSS
import "html/template"     // escapes according to the HTML context it is writing into
```

The second package is context-aware — it escapes differently inside an attribute, a URL, a script block or text — which is stronger than what most templating systems offered historically, and it is used by accident far less often than it is used correctly. The bug appears when a developer imports the text package for a "plain" template and then renders HTML.

### The safe API, next to the convenient one

This is the table to keep, and it is the practical summary of the whole series.

| Need | Python | Ruby | Node.js | Go |
|---|---|---|---|---|
| **Deserialise** | `json.loads` / `pickle` | `JSON.parse` / `Marshal.load` | `JSON.parse` | `encoding/json` / `encoding/gob` |
| **Run a command** | `subprocess.run([...])` / `shell=True` | `system(cmd, *args)` / backticks | `execFile` / `exec` | `exec.Command(name, args...)` / `sh -c` |
| **Template into HTML** | Jinja2 autoescape / `\|safe` | ERB (no escaping by default) | framework-dependent | `html/template` / `text/template` |
| **Join a path** | `realpath` + separator check | `File.realpath` + check | `path.resolve` + check | `filepath.Clean` + separator check |
| **Dynamic call** | `getattr` | `send` | `obj[name]()` | reflection (rarely, and verbosely) |
| **Compare secrets** | `hmac.compare_digest` | `Rack::Utils.secure_compare` | `crypto.timingSafeEqual` | `subtle.ConstantTimeCompare` |
| **Types** | dynamic, no implicit conversion | dynamic | dynamic, `===` | static, explicit conversions |

Three patterns run down the table: **use a format that cannot name code**, **pass arguments separately rather than as a string**, and **pick the API whose default is the safe behaviour**.

### Detection and mitigation

- **Do not deserialise untrusted data, in any of these four.** Use JSON or another format without a "call this" concept. Where a serialising format is genuinely required, treat the input as code.
- **Alert on the signatures, because each is distinctive.** Python pickle streams start with `0x80` and a protocol byte; Ruby `Marshal` streams start with `\x04\x08`; YAML carrying `!!python/` or `!!ruby/` tags; and any of them base64-encoded in a cookie or parameter. These are structural anomalies in the same family as `rO0AB` for Java and `php://` for PHP: the platform's vocabulary appearing in a client's request.
- **Alert on `__proto__`, `constructor` and `prototype` as JSON keys.** They have no legitimate reason to arrive from a client, and their presence is the signature of a prototype-pollution attempt. Pair it with a check on the **types** of the values, since the payload is often an object where a string was expected — the same structural check as the NoSQL entry.
- **Trace the merge, not the parse.** The Node bug is never in `JSON.parse`; it is in the helper that copies keys onto another object. Reviewing third-party merge and defaults utilities, and keeping their versions current, is where this class is actually fixed.
- **Check that a check is not compiled away.** `assert` in Python disappears under `-O`; a debug-only validation branch disappears in production mode in several frameworks. An authorisation decision must not live behind a flag that turns it off.
- **Use the loader, the template engine and the process API that default to safe.** `yaml.safe_load`, `YAML.safe_load`, `html/template`, `execFile`, `subprocess.run([...])`. Every one of them is the same size as the unsafe alternative, which is why the choice is a convention to be written down rather than a judgement call to be made each time.
- **Resolve paths and compare with a separator, everywhere.** Go cleans the join but still resolves `..`; Python's join does neither; Node's `path.resolve` normalises but the containment check is still yours. The check is the same in all four, and the concrete shape is in the opening entry of this series.
- **And remember what the language cannot express.** A Go service has no prototype, no implicit conversion and no pickle, so a review there can spend its time on logic, templates and authorisation instead of on a catalogue of runtime footguns. Knowing which classes of bug a stack makes *impossible* is as useful as knowing which it makes easy.

<!-- lang:zh -->
### 四种运行时，四种失效方式

前面三篇讲的都是"语言本身提供攻击面"的技术栈。这四种更参差，而差异本身很有教育意义：

| 运行时 | 决定其 bug 画像的那个性质 |
|---|---|
| **Python** | 一切都是对象，而 `pickle` 愿意把其中任何一个重建出来 —— 不需要找 gadget |
| **Ruby** | 动态派发是惯用法（`send`），而 `Marshal`/`YAML` 会重建对象 |
| **Node.js** | 有一个原型对象被所有对象共享，**而且它可以被数据写入** |
| **Go** | 静态类型、显式错误、没有隐式转换 —— 上面大部分东西根本无法表达 |

Go 出现在这一篇里是刻意的：**看到一个"没有这些性质"的运行时长什么样**，才能告诉你到底哪些性质是真正要紧的。

### Python

#### `pickle` 是一种代码执行格式

反序列化不是解析。这个格式里有一个操作码意思是"用这些参数调用这个可调用对象"，而类通过 `__reduce__` 定义它：

```python
import pickle, os, base64

class Exploit:
    def __reduce__(self):
        # 可调用对象和它的参数被存进流里，并在 load 时被调用
        return (os.system, ('echo pickle-executed',))

payload = pickle.dumps(Exploit())
print(base64.b64encode(payload)[:24].decode())
print(payload[:2].hex())          # 协议 4 是 8004，Python 3.8+ 的默认

pickle.loads(payload)             # 打印: pickle-executed
```

注意这里**没有**什么：没有要找的 gadget 链、没有要搜的类路径、没有要匹配的版本。攻击者把可调用对象写进了载荷，因为这个格式里有一个字段就是放它的。**这就是 Python 的反序列化风险与 Java 的区别**：Java 需要一个已经存在于类路径上的 gadget，Python 只需要一行。

`copyreg` 注册的类型、`__setstate__`、`__reduce_ex__`、以及 persistent-id 机制都是更多的钩子，但结论不变：**载入一个 pickle，就是在运行一个由那个 pickle 选择的程序。**

替代方案是一种没有这个概念可言的格式：

```python
import json
data = json.loads(untrusted)      # 只能产出 dict、list、字符串、数字、布尔、null
```

JSON 无法命名一个可调用对象，于是也就无法被说服去调用一个。这就是为什么修法是"**改用 JSON**"，而不是"小心使用 pickle"。

签名也是检测上的礼物：pickle 流以 `0x80` 加一个协议字节开头（协议 4 是 `0x04`、协议 5 是 `0x05`），而它的 base64 在一个值的开头就认得出来。

#### YAML，以及那个要紧的 loader

`yaml.load` 在历史上会从文档里的标签实例化任意 Python 对象：

```yaml
!!python/object/apply:os.system ["echo yaml-executed"]
```

PyYAML 改了默认行为，现在必须给 `Loader` 参数，而 `FullLoader` 仍允许一部分构造。对不可信输入，唯一安全的选择是显式的：

```python
import yaml
yaml.safe_load(untrusted)         # 只有基本类型，不构造对象
```

跨语言的通用规则是：**决定能力的是 loader，而不是格式。** 一个配置为"安全类型"的 YAML 解析器是安全的；同一个库在 full 模式下不是。

#### 动态派发与格式化

| 特性 | 风险 |
|---|---|
| `eval`、`exec` | 执行一个字符串；等价于运行用户代码 |
| `getattr(obj, name)` | 一个字符串决定调用哪个函数 —— 对应 Ruby 的 `send`、Java 的反射 |
| `os.system`、带 `shell=True` 的 `subprocess` | 字符串被交给 shell，于是其中的 shell 元字符会被解释 |
| `str.format` 与 f-string | `"{0.__class__}".format(obj)` 会触达属性；用户可控的格式串是一条信息泄漏原语 |
| `assert` | **在 Python 以 `-O` 运行时会被移除** —— 永远不要用它做授权检查 |

最后一行值得单独一句：一个依赖 `assert user.is_admin` 做安全决策的应用，那个决策在优化模式下**被编译掉了**。Python 文档写明了这一点；而它容易被忽略，因为那段代码读起来就像一个检查。

#### 路径

两个看起来可以互换、实际不能的函数：

```python
import posixpath
print(posixpath.commonprefix(['/var/www/uploads-a', '/var/www/uploads-b']))
# /var/www/uploads      <- 一个字符串前缀，不是一个目录
print(posixpath.commonpath(['/var/www/uploads-a', '/var/www/uploads-b']))
# /var/www              <- 真正共享的那个目录
```

`commonprefix` 是**逐字符**比较的，所以它会痛快地返回一个并不是路径边界的前缀。`commonpath` 才是懂目录的那个。同一个区分 —— 字符串前缀对路径边界 —— 也是为什么包含检查需要补一个分隔符，本系列开篇已经演示过。

以及那个要记住的 join 行为：

```python
print(posixpath.join('/var/www/uploads', 'shell.php'))    # /var/www/uploads/shell.php
print(posixpath.join('/var/www/uploads', '/etc/passwd'))  # /etc/passwd  <- 绝对路径胜出
```

### Ruby

#### `Marshal` 与 `YAML` 会重建对象

```ruby
Marshal.load(untrusted)     # 重建 Ruby 对象；头部是 \x04\x08
YAML.load(untrusted)        # 历史上会实例化任意类
```

`Marshal.load` 在对象构造这件事上**没有安全模式** —— 唯一安全的用法是用在你自己产出的数据上。`YAML.load` 是一个著名 Rails 漏洞的载体，而现代 Ruby 改了默认值：Psych 4（Ruby 3.1+）让 `YAML.load` 表现得像 `safe_load`。教训依旧是关于版本与 loader，而不是关于格式。

#### 动态派发是一种语言特性

Ruby 的 `send` 会调用由参数命名的方法，**包括私有方法**：

```ruby
obj.send(params[:method])              # 一个请求参数选择了方法
send(:system, params[:cmd])            # 配上合适的参数，那就是执行
```

`public_send` 避开私有方法，攻击面更小。这个惯用法本身不是 bug —— 它就是 Ruby 元编程的工作方式 —— 但一个参数到达 `send`，与一个字符串到达 `getattr` 或 `Class.forName` 是同一个事件。

相关的、会执行或触达得比看上去更远的特性：

| 特性 | 行为 |
|---|---|
| `eval`、`instance_eval`、`class_eval` | 把字符串当 Ruby 代码求值 |
| 反引号与 `%x{}` | 运行 shell 命令 |
| 以 `\|` 开头的 `Kernel#open` | 历史上会打开一个到命令的管道，而不是打开文件 |
| `method_missing` | 处理对未定义方法的调用；让"这个对象响应什么"变得不直观 |
| 字符串插值 `#{}` | 会求值其中的内容，所以由输入拼出的字符串可以包含代码 |

### Node.js

#### 一个原型，被共享、且可写

这是本系列里其他运行时都没有的性质。每个对象都继承自 `Object.prototype`，而 JavaScript 允许数据触达那条链：

```javascript
const obj = {};
obj.__proto__.polluted = "yes";        // 写到 Object.prototype 上
console.log({}.polluted);              // "yes"   <- 现在每个对象都有它了
```

危险的形状不是直接写入 —— 而是**递归合并**，几乎每个 Node 应用都有这么一个：

```javascript
// 有漏洞的模式：一个相信自己的键的深合并
function merge(target, source) {
  for (const key in source) {
    if (typeof source[key] === "object" && source[key] !== null) {
      target[key] = target[key] || {};
      merge(target[key], source[key]);
    } else {
      target[key] = source[key];
    }
  }
  return target;
}

merge({}, JSON.parse('{"__proto__": {"polluted": "yes"}}'));
console.log({}.polluted);
```

当 `key` 是 `__proto__` 时，递归就走到了目标的原型上、并写在那里。于是进程里的每个对象都带着攻击者的属性 —— 而升级到什么程度，取决于**他们选了哪个属性**：

| 被污染的属性 | 后果 |
|---|---|
| 应用读来做决策的某个属性 | 逻辑绕过，权限检查被击败 |
| `child_process` 调用 options 上的 `shell`、`env`、`argv0` | 命令执行 —— 如果那个库从一个继承了被污染值的对象上读 options |
| 环境对象里的 `NODE_OPTIONS` | 下一个被 spawn 的进程上发生代码执行 |
| 模板或序列化引擎的 options | 那个引擎拿它们做什么，就是什么 |

**`JSON.parse` 本身在这一点上是安全的**，这值得精确地知道，因为它改变了 bug 的位置：

```javascript
const parsed = JSON.parse('{"__proto__": {"polluted": "yes"}}');
console.log({}.polluted);                 // undefined —— JSON.parse 设的是自有属性
console.log(Object.getOwnPropertyNames(parsed).includes("__proto__"));   // true
```

`JSON.parse` 创建的是一个**自有**属性 `__proto__`，并没有写到原型上。污染发生在这个对象随后被**合并、拷贝、或逐个字段赋值**到别的东西上时 —— 这就是为什么这个漏洞总是在库或助手函数里，从来不在 `JSON.parse` 里。

防御，按强度排列：

```javascript
Object.freeze(Object.prototype);                        // 让原型链不可变
const safe = Object.create(null);                       // 干脆没有原型
// 拷贝时拒绝危险的键
const BLOCKED = new Set(["__proto__", "constructor", "prototype"]);
for (const k of Object.keys(src)) if (!BLOCKED.has(k)) dst[k] = src[k];
```

#### Node 的其他面

| 特性 | 风险 |
|---|---|
| `child_process.exec` | 经过 shell 运行；`execFile` 和不带 shell 的 `spawn` 不会 |
| `eval`、`new Function`、`vm` | 代码执行；`vm` 模块**不是**安全边界 |
| `require(变量)`、动态 `import()` | 一个字符串决定加载哪个模块 |
| 带 `serialize`/`unserialize` 的反序列化库 | 触达能力等同 `pickle` |
| 依赖原型的 options | 上面那条升级路径 |

### Go

#### 静态类型关上了大部分门

Go 没有隐式转换、包装类型没有 `==` 的意外、没有从字节重建任意对象的方法、也没有原型链。对 Go 应用的评审就是**对逻辑、模板与路径的评审**，而清单很短：

| 领域 | Go 特有的关注点 |
|---|---|
| 模板 | `text/template` **不做任何转义**；`html/template` 做上下文感知的转义。把前者用于 HTML 就是那个 bug |
| 命令执行 | `exec.Command(name, args...)` **不使用 shell**，所以参数值无法注入。重新引入 `sh -c` 就是把问题重新引入 |
| 路径 | `filepath.Join` 会**清理**结果，比 Python 的 `join` 安全 |
| 反序列化 | `encoding/json` 无法命名类型；`encoding/gob` 会重建类型，但不会像 `pickle` 那样调用任意代码 |
| 整数运算 | 宽度是显式的，但变量运算仍然**静默回绕** |
| `unsafe` | 名字里就写着，它的出现是一个评审触发点 |

路径上的差别值得亲眼看一次，因为它和上面 Python 的行为正好相反：

```go
package main

import (
	"fmt"
	"path/filepath"
)

func main() {
	fmt.Println(filepath.Join("/var/www/uploads", "shell.php"))
	fmt.Println(filepath.Join("/var/www/uploads", "/etc/passwd"))
	fmt.Println(filepath.Join("/var/www/uploads", "../../etc/passwd"))
}
```

注意第二行：**绝对路径的第二个参数不会替换掉基准** —— Go 会清理并把它保留在里面。这与 Python 的 `os.path.join` 是实打实的差别，也是一个"语言选择了更安全的默认值"的例子。第三行则显示了还剩下什么要处理：`..` **确实**会被解析，所以路径穿越仍然能逃出基准 —— 这就是为什么那个包含检查（先解析、再带分隔符比较前缀）依然必需。

`html/template` 与 `text/template` 值得强调，因为它是 Go 里唯一会产生经典 Web 漏洞的错误：

```go
import "text/template"     // 不转义：把用户数据放进 HTML 就是 XSS
import "html/template"     // 按它正在写入的 HTML 上下文来转义
```

后一个包是上下文感知的 —— 在属性里、URL 里、脚本块里、文本里，转义方式各不相同 —— 这比历史上多数模板系统提供的都要强，而它被误用的概率远低于被正确使用的概率。bug 出现在开发者为了一个"纯文本"模板引入了 text 包、然后拿它渲染 HTML 的时候。

### 安全 API，与便利 API 并排

这是那张该留着的表，也是整个系列的实用总结。

| 需求 | Python | Ruby | Node.js | Go |
|---|---|---|---|---|
| **反序列化** | `json.loads` / `pickle` | `JSON.parse` / `Marshal.load` | `JSON.parse` | `encoding/json` / `encoding/gob` |
| **执行命令** | `subprocess.run([...])` / `shell=True` | `system(cmd, *args)` / 反引号 | `execFile` / `exec` | `exec.Command(name, args...)` / `sh -c` |
| **渲染 HTML 模板** | Jinja2 自动转义 / `\|safe` | ERB（默认不转义） | 取决于框架 | `html/template` / `text/template` |
| **拼接路径** | `realpath` + 分隔符检查 | `File.realpath` + 检查 | `path.resolve` + 检查 | `filepath.Clean` + 分隔符检查 |
| **动态调用** | `getattr` | `send` | `obj[name]()` | 反射（很少用，而且很啰嗦） |
| **比较机密** | `hmac.compare_digest` | `Rack::Utils.secure_compare` | `crypto.timingSafeEqual` | `subtle.ConstantTimeCompare` |
| **类型** | 动态，无隐式转换 | 动态 | 动态，`===` | 静态，显式转换 |

有三条模式贯穿整张表：**使用无法命名代码的格式**、**把参数分开传而不是拼成字符串**、以及**挑那个默认行为就安全的 API**。

### 检测与缓解

- **在这四种语言里，都不要反序列化不可信数据。** 用 JSON 或其他没有"调用这个"概念的格式。在确实需要序列化格式的地方，把输入当成代码。
- **对签名告警，因为每一种都很有辨识度。** Python pickle 流以 `0x80` 加一个协议字节开头；Ruby 的 `Marshal` 流以 `\x04\x08` 开头；YAML 里带 `!!python/` 或 `!!ruby/` 标签；以及它们任何一个被 base64 编进 cookie 或参数。这些和 Java 的 `rO0AB`、PHP 的 `php://` 是同一族结构性异常：**平台的词汇出现在了客户端的请求里**。
- **对作为 JSON 键出现的 `__proto__`、`constructor`、`prototype` 告警。** 它们没有任何正当理由从客户端到来，它们出现就是原型污染尝试的特征。再配上一个对**值类型**的检查，因为那个载荷常常是本该是字符串的位置上来了对象 —— 和 NoSQL 那篇同一个结构性检查。
- **追那个 merge，不要追那个 parse。** Node 的这个 bug 从不在 `JSON.parse` 里，而在把键拷贝到另一个对象上的那个助手函数里。评审第三方的 merge 与 defaults 工具、并保持它们的版本更新，才是这一类真正被修好的地方。
- **确认一个检查没有被编译掉。** Python 的 `assert` 在 `-O` 下消失；几个框架里"仅调试"的校验分支在生产模式下消失。授权决策不能住在某个能把它关掉的开关后面。
- **用那个默认就安全的 loader、模板引擎与进程 API。** `yaml.safe_load`、`YAML.safe_load`、`html/template`、`execFile`、`subprocess.run([...])`。它们的体积与不安全的那几个完全一样 —— 这就是为什么这个选择应该是一条**写下来的约定**，而不是每次临场判断。
- **处处解析路径、并带分隔符比较。** Go 会清理 join 但仍然解析 `..`；Python 的 join 两者都不做；Node 的 `path.resolve` 会规范化，但包含检查仍然是你的活。四种语言里的检查都是同一个，而具体形状在本系列开篇那篇里。
- **并且记住这门语言"无法表达"什么。** 一个 Go 服务没有原型、没有隐式转换、没有 pickle，所以在那里评审可以把时间花在逻辑、模板与授权上，而不是花在一份运行时坑的清单上。**知道一个技术栈让哪几类 bug 变得不可能，和知道它让哪几类变得容易一样有用。**
