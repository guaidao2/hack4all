---
id: ssti-sandbox-and-escape
title_en: "SSTI, Part 2 — Sandboxes and the Paths Around Them"
title_zh: "SSTI（二）：沙箱，以及绕过它的那些路"
summary_en: A sandbox restricts one thing — reaching from template objects into the host language — and cannot restrict what the application put in the context. Measured across eight attribute-access paths in two sandboxes, plus the escape shape for each engine family.
summary_zh: 沙箱只限制一件事 —— 从模板对象伸进宿主语言 —— 而它管不住应用放进上下文的东西。这一篇在两个沙箱上量了八条取属性的路径，并给出各引擎家族的逃逸形状。
tags: [web, ssti, cwe-1336, sandbox, jinja2, rce]
tools: [python3, jinja2, Burp Suite]
attck: [T1190, T1059]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### What a sandbox does, and the one thing it cannot do

Part 1 established that a template engine can be walked from a value to the language runtime. A sandbox exists to break that walk, and it is the right tool for the job — as long as its boundary is understood.

> **A sandbox restricts how the template reaches the host language. It has no opinion about what the application put in the context.**

That second half is where the findings are. Measured on Jinja2's `SandboxedEnvironment`, with a context containing an application config:

| Payload | Native sandbox |
|---|---|
| `{{lipsum.__globals__}}` | filtered to empty |
| `{{lipsum.__getattribute__('__globals__')}}` | **SecurityError** |
| `{{''.__class__.__mro__}}` | **SecurityError** |
| `{{config}}` | **rendered in full, including `SECRET_KEY`** |
| `{{request.environ}}` | **rendered in full, including environment variables** |

The sandbox stopped both escape attempts and did nothing at all about the two payloads that leaked the application's own secrets, **because those two never left the context.** No amount of sandbox configuration changes that, and it is the reason "we use the sandbox" is not an answer to "is this feature safe".

### Why the native sandbox holds

The sandbox's rule is not a list of forbidden names. It refuses **any attribute whose name begins with an underscore**, and that is a rule about a category rather than about the members of one.

```
is_safe_attribute(obj, '__class__')                    -> False
is_safe_attribute(obj, '__mro__')                      -> False
is_safe_attribute(obj, '_TemplateReference__context')  -> False
is_safe_attribute(obj, 'upper')                        -> True
is_safe_attribute(obj, 'user')                         -> True
```

The reason this works is worth stating precisely, because it is the same lesson as the rest of the guide: **underscore-prefixed attributes are the only doorway from an object into the language.** `__class__` leads to the type, `__mro__` to the hierarchy, `__subclasses__` to every loaded class, `__globals__` to a module's namespace, `__dict__` to raw state. Close that doorway and there is nowhere to walk — which is why the sandbox does not need to know what an attacker will try.

**Contrast that with what the weakened sandbox in the lab did.** Its author found the default too strict and replaced the category rule with a list of nine names:

```
__base__  __builtins__  __code__  __globals__  __import__
__loader__  __mro__  __reduce__  __subclasses__
```

That list is a blacklist, and a blacklist on a category with more members than the author counted is an open door. Two members were missing and both were enough: `__getattribute__` and `__dict__`.

### Empty is not the same as blocked

The probes above return three different outcomes, and reading them correctly is what makes the blacklist visible.

| Result | What it means |
|---|---|
| A value rendered | The path worked |
| **Empty output** | The attribute was resolved but produced an undefined value — the sandbox **filtered** it |
| **`SecurityError`** | The sandbox **refused** the attribute by name — it is on the list |
| **`UndefinedError`** | The name does not exist in the context at all — **this says nothing about the sandbox** |

The last row is the one that misleads. Probing `{{request.environ}}` in a render call that was not given a `request` object returns `UndefinedError`, exactly as an empty context would, and it is easy to read that as the sandbox doing its job. It is not: it is a missing variable. **A payload that fails because the name is absent tells you nothing about what is blocked**, and treating it as a pass is how a real opening gets missed.

The distinction between filtered and refused is what lets an attacker enumerate the list: one probe per candidate name, and the ones that raise `SecurityError` are the ones that were anticipated.

### Eight paths to the same attribute

An attribute can be obtained in more ways than the dot, and the sandbox's check is attached to the dot. Measured, with the same target attribute reached eight ways:

| Path | Native sandbox | Weakened sandbox |
|---|---|---|
| `lipsum.__globals__` | filtered | filtered |
| `lipsum \| attr('__globals__')` | filtered | filtered |
| `lipsum['__globals__']` | filtered | filtered |
| `getattr(lipsum, '__globals__')` | refused | refused |
| **`lipsum.__getattribute__('__globals__')`** | **SecurityError** | **returns the globals** |
| `lipsum.__dict__` | filtered | **returns `{}`** |
| `lipsum \| attr('__dict__')` | filtered | **returns `{}`** |
| `''.__class__.__base__.__getattribute__('__subclasses__')()` | SecurityError | error |

Two things are visible in that table.

**`__getattribute__` is the method that performs attribute lookup.** Writing `obj.attr` and `obj.__getattribute__('attr')` do the same thing; the difference is that the second is an ordinary **method call**, so the sandbox sees a call to a name rather than a lookup of a name. Under the native sandbox that name is underscored and therefore refused — the category rule covers it. Under the weakened sandbox it is not on the list, and the check that guards every other path is bypassed by calling the guard itself.

**The generalisation is the useful part.** A security check placed on **one of several equivalent routes** protects that route and nothing else. The same shape appears in every language that has more than one way to read a property:

| Language | The checked route | Equivalent routes around it |
|---|---|---|
| Python | `obj.attr` | `__getattribute__`, `getattr`, `obj.__dict__`, `vars` |
| Java | field access | `getClass()`, reflection, `Class.forName` |
| Ruby | method call | `send`, `public_send`, `instance_variable_get` |
| JavaScript | property access | `constructor`, `__proto__`, `Reflect.get` |

### The escape shape for each engine

The payload is engine-specific, and so is whether an escape is needed at all. This is the part worth checking per stack rather than copying between them.

| Stack | Escape shape | Note |
|---|---|---|
| **Jinja2** (Python) | `cycler.__init__.__globals__.os.popen(...)` | Reaches a module namespace through a function's `__globals__`; the sandbox blocks it, `__getattribute__` bypasses a weakened one |
| **Mako** (Python) | `<% import os %>` | `<% %>` **is** Python; there is no sandbox concept to escape |
| **Freemarker** (Java) | `<#assign ex="freemarker.template.utility.Execute"?new()>${ex("id")}` | `?new()` instantiates an arbitrary class; recent versions restrict it and many deployments enable it anyway |
| **Velocity** (Java) | `$class.inspect(...)`, `$class.forName(...)` | Reaches Java reflection from the template's own tools |
| **Thymeleaf** (Java) | `__${...}__` preprocessing, `T(java.lang.Runtime)` | The preprocessing syntax **builds an expression out of data**, which is the injection primitive |
| **ERB** (Ruby) | `<%= system("id") %>` | `<% %>` is Ruby; nothing to escape |
| **EJS** (Node) | `<%= process.mainModule.require('child_process').execSync('id') %>` | JavaScript with `process` in scope |
| **Handlebars** (Node) | no expression evaluation | Logic-less by design; reached through prototype pollution instead (a separate entry) |
| **Twig** (PHP) | `{{['id']\|filter('system')}}`, `_self` in older versions | Modern Twig removed most of these; the version matters |
| **Go** | **nothing to escape to** | `text/template` exposes fields, methods and registered functions only — no route into the runtime exists, so severity is decided by the context and the function map |

**The Go row is the one to hold on to.** There is no sandbox escape there because there is no walk to make: a Go template cannot climb into the runtime at all. That does not make the injection harmless — a context carrying a method that shells out, or a `FuncMap` entry that does, is code execution — but it means the review question is about the application's own data, not about the engine's defences.

### Ask whether an escape is needed

The measured table at the top answers this for most real cases: **the payloads that leaked the most secrets never touched the sandbox.** They asked the context for what it already had.

So the order of work is not "find an escape, then look around". It is:

1. **Read the context.** `{{config}}`, `{{request}}`, `{{settings}}`, `{{env}}`, the ORM object, the user object. Whatever the framework injects globally is available without any escape at all.
2. **Look at the registered helpers.** Custom filters and functions are written by the same team that wrote the feature, and a helper that reads a file, runs a command or evaluates an expression is reachable by name. This applies to every stack, and it is the whole story on Go.
3. **Only then look for a way into the language**, and if one is needed, look for the route the check does not cover.

### Detection and mitigation

- **Treat a sandbox interception as an alert, not a caught error.** `SecurityError` from a template renderer means someone asked an object for `__class__` or `__globals__`. No legitimate template does that, so the event itself is the finding — and it should be logged with the payload rather than swallowed into a generic 500.
- **Record both the filtered and the refused cases.** A payload that renders empty because an attribute was filtered is a probe that half-succeeded, and a burst of them is an attacker enumerating what the list contains. Distinguishing these two in the logs is what turns a series of harmless-looking requests into visible reconnaissance.
- **Watch for language internals appearing in output.** Class names, module names, `security error`, file paths and stack traces in a rendered page are evidence that the template reached somewhere it should not, whether or not anything executed.
- **And keep watching the context, because that is where the sandbox helps least.** Environment variables, `SECRET_KEY`, connection strings and internal hostnames appearing in a page are a finding regardless of how good the sandbox is.
- **For mitigation, the ordering from part 1 still applies.** Not letting users supply template source removes the class; a logic-less format removes the evaluation; a sandbox only reduces what an escape would be worth.
- **If a sandbox is used, use the one the project ships and do not weaken it.** The lab's weakened sandbox exists because a developer found the default inconvenient and replaced a category rule with a short list. A component that exists to enforce a security boundary is not the place to trade safety for convenience — and the measured result of doing so was a full escape with two names missing.
- **Where the engine permits it, restrict the function map as tightly as the context.** A registered helper is indistinguishable from a built-in to the template author and is the entire capability set on Go. The same review that lists what goes into the context should list what goes into the function table.
- **Keep the context minimal, and remember it is the control the sandbox cannot provide.** This is the one measure that works identically on every stack, engine and sandbox version: what is not passed in cannot be rendered.

<!-- lang:zh -->
### 沙箱做什么，以及它唯一做不到的那件事

第一篇立起了"模板引擎能从某个值一路走到语言运行时"。沙箱的存在就是为了打断那条路，而它是对的工具 —— 前提是明白它的边界在哪。

> **沙箱限制的是"模板怎么够到宿主语言"。它对应用往上下文里放了什么，没有任何意见。**

后面那半句才是发现所在。在 Jinja2 的 `SandboxedEnvironment` 上实测，上下文里放了一份应用配置：

| payload | 原生沙箱 |
|---|---|
| `{{lipsum.__globals__}}` | 被过滤成空 |
| `{{lipsum.__getattribute__('__globals__')}}` | **SecurityError** |
| `{{''.__class__.__mro__}}` | **SecurityError** |
| `{{config}}` | **完整渲染，含 `SECRET_KEY`** |
| `{{request.environ}}` | **完整渲染，含环境变量** |

沙箱挡住了两次逃逸尝试，而对那两个泄漏了应用自身机密的 payload **什么都没做** —— 因为它们从来没有离开上下文。怎么配沙箱都改变不了这一点，而这就是为什么"我们用了沙箱"不能算作"这个功能安全吗"的答案。

### 原生沙箱为什么守得住

沙箱的规则不是一张禁止名单，它拒绝的是**所有名字以下划线开头的属性**，而这是一条关于**类别**的规则，不是关于该类别的成员的。

```
is_safe_attribute(obj, '__class__')                    -> False
is_safe_attribute(obj, '__mro__')                      -> False
is_safe_attribute(obj, '_TemplateReference__context')  -> False
is_safe_attribute(obj, 'upper')                        -> True
is_safe_attribute(obj, 'user')                         -> True
```

它之所以有效，原因值得说准 —— 因为它就是本指南一路讲下来的那条道理：**以下划线开头的属性，是从一个对象走进语言内部的唯一一道门。** `__class__` 通向类型，`__mro__` 通向继承层级，`__subclasses__` 通向所有已加载的类，`__globals__` 通向一个模块的命名空间，`__dict__` 通向裸状态。把这道门关上就无处可走 —— 所以沙箱不需要知道攻击者会试什么。

**拿它和靶场里那个被削弱的沙箱对比。** 它的作者觉得默认太严，于是把那条类别规则换成了一张九个名字的清单：

```
__base__  __builtins__  __code__  __globals__  __import__
__loader__  __mro__  __reduce__  __subclasses__
```

那是一张黑名单，而在一张"成员比作者数过的更多"的类别上，黑名单就是一扇开着的门。有两个成员没被列上，而两个都够用：`__getattribute__` 和 `__dict__`。

### "空"和"被拦"不是一回事

上面那些探针会给出三种不同的结果，而把它们读对，正是让黑名单现形的方法。

| 结果 | 它意味着什么 |
|---|---|
| 渲染出了值 | 这条路径通了 |
| **输出为空** | 属性被解析出来了，但产生了未定义的值 —— 沙箱**过滤**了它 |
| **`SecurityError`** | 沙箱**按名字拒绝**了这个属性 —— 它在名单上 |
| **`UndefinedError`** | 这个名字在上下文里根本不存在 —— **这和沙箱无关** |

最后一行最容易误导。在一次没有传 `request` 对象的渲染里探 `{{request.environ}}`，会返回 `UndefinedError`，和一个空上下文的表现一模一样，而把它读成"沙箱起作用了"是很自然的。它不是：那是一个不存在的变量。**一个因为名字缺失而失败的 payload，关于"什么被拦"什么也没告诉你**，而把它当作"通过"正是漏掉一个真实开口的方式。

"被过滤"与"被拒绝"之间的区别，正是让攻击者能枚举那份名单的东西：每个候选名字一个探针，抛出 `SecurityError` 的那些，就是被预料到的那些。

### 同一个属性，八条路径

取一个属性远不止点号一种写法，而沙箱的那道检查挂在点号上。实测，同一个目标属性用八种方式取：

| 路径 | 原生沙箱 | 被削弱的沙箱 |
|---|---|---|
| `lipsum.__globals__` | 被过滤 | 被过滤 |
| `lipsum \| attr('__globals__')` | 被过滤 | 被过滤 |
| `lipsum['__globals__']` | 被过滤 | 被过滤 |
| `getattr(lipsum, '__globals__')` | 被拒绝 | 被拒绝 |
| **`lipsum.__getattribute__('__globals__')`** | **SecurityError** | **返回了全局命名空间** |
| `lipsum.__dict__` | 被过滤 | **返回 `{}`** |
| `lipsum \| attr('__dict__')` | 被过滤 | **返回 `{}`** |
| `''.__class__.__base__.__getattribute__('__subclasses__')()` | SecurityError | 报错 |

那张表里有两点看得见。

**`__getattribute__` 就是执行属性查找的那个方法。** 写 `obj.attr` 和写 `obj.__getattribute__('attr')` 做的是同一件事；区别在于第二种是一次**普通的方法调用**，于是沙箱看到的是"对某个名字的调用"，而不是"对某个名字的查找"。在原生沙箱下，那个名字带下划线，因而被那条类别规则拒绝。在被削弱的沙箱下，它不在名单上 —— 于是**通过调用那个守卫本身，绕过了守卫着其他所有路径的那道检查**。

**可推广的部分才是有用的。** 一道放在**若干等价路径中的一条**上的安全检查，只保护那一条。同一个形状出现在每一门"有不止一种方式读属性"的语言里：

| 语言 | 被检查的那条路 | 绕过它的等价路径 |
|---|---|---|
| Python | `obj.attr` | `__getattribute__`、`getattr`、`obj.__dict__`、`vars` |
| Java | 字段访问 | `getClass()`、反射、`Class.forName` |
| Ruby | 方法调用 | `send`、`public_send`、`instance_variable_get` |
| JavaScript | 属性访问 | `constructor`、`__proto__`、`Reflect.get` |

### 各引擎的逃逸形状

payload 因引擎而异，而"到底需不需要逃逸"同样因引擎而异。这一块值得按技术栈逐个查，而不是在它们之间照抄。

| 技术栈 | 逃逸形状 | 说明 |
|---|---|---|
| **Jinja2**（Python） | `cycler.__init__.__globals__.os.popen(...)` | 经一个函数的 `__globals__` 够到模块命名空间；沙箱挡得住，`__getattribute__` 绕得过被削弱的那种 |
| **Mako**（Python） | `<% import os %>` | `<% %>` **就是** Python；没有可逃的沙箱概念 |
| **Freemarker**（Java） | `<#assign ex="freemarker.template.utility.Execute"?new()>${ex("id")}` | `?new()` 能实例化任意类；新版本限制它，而很多部署仍然开着 |
| **Velocity**（Java） | `$class.inspect(...)`、`$class.forName(...)` | 从模板自带的工具够到 Java 反射 |
| **Thymeleaf**（Java） | `__${...}__` 预处理、`T(java.lang.Runtime)` | 预处理语法**用数据拼出一个表达式**，那就是注入原语 |
| **ERB**（Ruby） | `<%= system("id") %>` | `<% %>` 就是 Ruby；没有可逃的东西 |
| **EJS**（Node） | `<%= process.mainModule.require('child_process').execSync('id') %>` | JavaScript，而 `process` 在作用域里 |
| **Handlebars**（Node） | 没有表达式求值 | 按设计无逻辑；要经原型污染去打（另有一篇） |
| **Twig**（PHP） | `{{['id']\|filter('system')}}`、老版本的 `_self` | 现代 Twig 移除了大部分；版本很要紧 |
| **Go** | **没有可逃的地方** | `text/template` 只暴露字段、方法与注册过的函数 —— 走进运行时的路根本不存在，所以严重程度由上下文与函数表决定 |

**Go 那一行值得记住。** 那里没有沙箱逃逸，因为根本没有路可走：Go 模板压根爬不进运行时。这不代表这种注入无害 —— 上下文里有一个会起 shell 的方法、或 `FuncMap` 里有这么一个条目，那就是代码执行 —— 但它意味着评审的问题落在应用自己的数据上，而不是引擎的防御上。

### 先问：我需要逃逸吗

顶部那张实测的表，对多数真实场景已经回答了这个问题：**泄漏最多机密的那些 payload，根本没碰沙箱。** 它们只是向上下文要了它本来就有的东西。

所以干活的顺序不是"先找一个逃逸、再四处看看"，而是：

1. **读上下文。** `{{config}}`、`{{request}}`、`{{settings}}`、`{{env}}`、那个 ORM 对象、那个用户对象。框架全局注入的东西，不需要任何逃逸就能拿到。
2. **看注册进来的助手。** 自定义过滤器与函数是"写出这个功能的那同一个团队"写的，而一个读文件、跑命令或求值表达式的助手，是可以按名字调用的。这适用于每个技术栈，而在 Go 上它就是全部。
3. **然后才去找走进语言的路**，而如果需要，就去找那道检查没覆盖的路径。

### 检测与缓解

- **把沙箱拦截当作告警，而不是"已拦下的错误"。** 渲染器抛出的 `SecurityError` 意味着有人向一个对象要了 `__class__` 或 `__globals__`。没有正当模板会做这件事，所以这个事件本身就是发现 —— 而它应当带着 payload 被记下来，而不是被吞进一个笼统的 500。
- **"被过滤"和"被拒绝"两种都要记。** 一个因为属性被过滤而渲染为空的 payload，是一次半数成功的探针；而它们成批出现，就是有人在枚举那份名单里有什么。在日志里把这两者区分开，正是把一串"看起来无害的请求"变成可见侦察的东西。
- **盯输出里出现的语言内部事物。** 渲染出来的页面里出现类名、模块名、`security error`、文件路径或调用栈，是"模板够到了不该够的地方"的证据 —— 无论有没有东西被执行。
- **并且继续盯上下文，因为那是沙箱最帮不上忙的地方。** 环境变量、`SECRET_KEY`、连接串与内部主机名出现在页面上，就是一条发现 —— 无论沙箱有多好。
- **缓解上，第一篇那个顺序依然成立。** 不让用户提供模板源码就移除了这一类；无逻辑格式移除了求值；沙箱只是降低"一次逃逸值多少"。
- **如果要用沙箱，就用项目自带的那一个，不要去削弱它。** 靶场里那个被削弱的沙箱之所以存在，是因为一个开发者觉得默认不方便，于是把一条类别规则换成了一张短清单。一个存在的意义就是守住安全边界的东西，不是拿安全换方便的地方 —— 而那么做的实测结果，是少了两个名字就换来一次完整逃逸。
- **在引擎允许的地方，把函数表收紧到和上下文一样的程度。** 一个注册进来的助手，对模板作者来说和内置函数没有区别，而在 Go 上它就是全部能力。那份"列出上下文里放了什么"的评审，也该列出函数表里放了什么。
- **让上下文最小，并记住它是沙箱提供不了的那项控制。** 这是唯一一条在每个技术栈、每个引擎、每个沙箱版本上都同样有效的措施：**没有传进去的东西，渲染不出来。**
