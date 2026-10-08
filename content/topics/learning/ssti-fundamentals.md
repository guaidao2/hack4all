---
id: ssti-fundamentals
title_en: "SSTI, Part 1 — A Template Engine Is an Evaluator"
title_zh: "SSTI（一）：模板引擎是求值器"
summary_en: A template engine is a small language rather than a placeholder system, so what matters is where the input lands, not what it contains. This entry measures that difference in one line of Jinja2, shows how far evaluation reaches, and gives the probe set for telling engines apart.
summary_zh: 模板引擎是一门小语言，不是一套占位符替换，所以要紧的是输入落在了哪里，而不是它里面有什么。这一篇用一行 Jinja2 把那个区别量出来，展示求值能走多远，并给出用来分辨引擎的一套探针。
tags: [web, ssti, cwe-1336, template, jinja2, rce]
tools: [python3, jinja2, Burp Suite, tplmap]
attck: [T1190, T1059]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The word "template" is the problem

A template sounds like a form letter with blanks: the application fills in a name, a date and a total, and the engine substitutes them. That model is wrong for every engine that matters, and the gap between the model and the reality is the vulnerability.

A template engine is **a small language with an interpreter**. Its syntax has variables, expressions, function calls, filters, conditionals and loops, and it evaluates all of them. What it lacks is not capability but a sandbox — and a sandbox is a separate, optional, frequently-disabled feature.

So the question that decides everything is not "what is in the input" but:

> **Does this input land in the template's data, or in the template's source?**

Measured on Jinja2, with the same string both ways:

```python
import jinja2
env = jinja2.Environment()
user = "{{7*7}}"

env.from_string("Hello {{ name }}").render(name=user)   # 'Hello {{7*7}}'   — the data
env.from_string("Hello " + user).render()               # 'Hello 49'        — the source
```

The string is identical. **One call treats it as text to print, the other as a program to run**, and the only difference is which argument it was passed as. That is the same distinction as parameterised queries against string concatenation, in a language nobody thinks of as a language.

### Why this shape of feature keeps appearing

SSTI is not usually the result of a careless `render(user_input)` scattered somewhere. It comes from a **product decision**:

- Let customers write their own welcome message.
- Let them customise an email template, or a notification.
- Let them supply a report layout, a theme, a widget.
- Let an admin edit the error page from a web form.

Every one of those is a request to **let the user write template source**, and every one of them turns a data field into a program field. The feature is legitimate; what it needs is either a restricted template language or a sandbox — and the entries that follow are about what happens when it has neither.

The places to look are the ones where a form field stores something that will later be rendered: welcome messages, signatures, e-mail bodies, report titles, dashboard captions, custom error pages, and anything an admin can edit that appears to a user.

### How far evaluation reaches

Measured on Jinja2 without any sandbox, escalating one step at a time:

| Payload | Output | What it demonstrates |
|---|---|---|
| `{{7*7}}` | `49` | The input is being **evaluated** |
| `{{7*'7'}}` | `7777777` | The engine's Python semantics are reachable — string repetition, not an error |
| `{{ 'abc'.upper() }}` | `ABC` | **Methods can be called** on the objects in scope |
| `{{ self.__class__ }}` | `<class 'jinja2.runtime.TemplateReference'>` | The **runtime's own objects** are reachable |
| `{{ config }}` | `{'SECRET_KEY': 'demo-secret', ...}` | Whatever the application put in the **context** is readable |
| `{{ [].__class__.__base__.__subclasses__() \| length }}` | `345` | The engine can be walked out to **the language's own type hierarchy** |

Read as a ladder, the four rungs are:

1. **Evaluation.** An expression runs. This alone is a finding — it means an attacker controls a program.
2. **Context access.** Everything the application passed to the renderer is now readable. In a Flask application that includes `config`, and `config` holds `SECRET_KEY` — which is the key that signs sessions and, in the previous entries, the thing that turns a signature into a forgery.
3. **Runtime access.** The objects the engine itself uses are reachable, which is the way out of the template and into the language.
4. **Code execution.** From the runtime objects, the chain to a process-spawning call is short — the classic one is `cycler.__init__.__globals__.os.popen(...)`, and the mechanism behind it is the subject of the next entry.

**A single `{{7*7}}` returning `49` therefore means all four are available**, because nothing between them is a barrier.

### Telling engines apart

The payload depends on which engine is running, and the probes are cheap enough to send all of them. The output tells you which one answered.

| Probe | Jinja2 / Twig | Freemarker | Velocity | Smarty | ERB / EJS / Mako |
|---|---|---|---|---|---|
| `{{7*7}}` | `49` | literal | literal | literal | literal |
| `${7*7}` | literal | `49` | literal | literal | literal (Mako: `49`) |
| `#{7*7}` | literal | literal | literal | literal | literal |
| `<%= 7*7 %>` | literal | literal | literal | literal | `49` |
| `{7*7}` | literal | literal | literal | `49` | literal |
| `7*7` bare | literal | literal | literal | literal | literal |
| `{{7*'7'}}` | `7777777` | literal | literal | literal | literal |

Two things to take from that table.

**The distinguishing pair is `{{7*7}}` together with `{{7*'7'}}`.** The first proves evaluation somewhere; the second identifies the Python-family engines specifically, because string repetition is a Python semantics rather than a template one. In other engines the same expression is an error or a literal, which is itself the information.

**A literal echo is not a negative result.** In most of those cells the engine is not refusing — that syntax simply is not its syntax. Testing one probe and concluding "no SSTI" is how this class gets missed.

And the syntax is not the only way in. Engines also evaluate in filter arguments, in block directives, and inside included files, so a filter that strips `{{` from one field says nothing about the next one.

### The same feature, a different shape in every language

The engine decides how far a payload can go, and the difference between them is not a matter of degree — some engines can walk out into the host language, and some cannot reach anything the application did not hand them. This is the part worth checking per stack rather than assuming.

| Language | Engines | Can the template reach the host language by itself? | What a payload typically gets |
|---|---|---|---|
| **Python** | Jinja2, Mako | **Yes** — through `__class__`, `__mro__`, `__subclasses__`, `__globals__` | Code execution |
| **Java** | Freemarker, Velocity, Thymeleaf | **Yes** — reflection and `getClass()` are reachable | Code execution |
| **Ruby** | ERB | Not applicable — `<% %>` **is** the language | Code execution |
| **Node.js** | EJS, Pug, Handlebars | EJS embeds JavaScript directly; Handlebars is logic-less | Code execution (EJS) |
| **PHP** | Twig, Smarty, Blade | Twig is restricted by design; Smarty historically was not | Varies by engine |
| **Go** | `text/template`, `html/template` | **No** | **Only what the context and the function map expose** |

**Go deserves its own paragraph**, because its shape is different enough that a payload from another stack simply does not parse. Measured on Go's `text/template`:

| Template | Result |
|---|---|
| `{{7*7}}` | **parse error** — Go templates have no arithmetic expressions |
| `{{.}}` | the entire context, printed |
| `{{.Secret}}` | the field, if it is exported |
| `{{.Danger "id -un"}}` | **the method runs**, if the context type has one |
| `{{printf "%s-%s" .User .Secret}}` | built-in functions are available |
| `{{index . "DB"}}` | map lookup by key |
| `{{len .Secret}}`, `{{if .Secret}}` | functions and control flow |

Two consequences follow, and they change what a Go finding means.

**A Go template cannot climb out on its own.** There is no equivalent of `__globals__`: the engine exposes fields and methods of the data it was given, and calls functions that were registered. So the severity of a Go template injection is decided entirely by **what the application put in the context and what it registered in the function map** — and that is a code-review question, not an engine question. A context holding a method that shells out, or a `FuncMap` containing a helper that does, turns the same bug into code execution; a context holding a display name does not.

**And the Go failure mode people actually hit is the other one.** Using `text/template` to render HTML performs **no escaping at all**, while `html/template` escapes according to the HTML context it is writing into:

```
text/template  -> user says: <script>alert(document.cookie)</script>
html/template  -> user says: &lt;script&gt;alert(document.cookie)&lt;/script&gt;
```

That is the XSS entry's problem arriving through a template choice, and it is the Go-specific mistake worth looking for: the two packages have similar names, similar APIs, and entirely different security properties.

**So the review question changes per language.** For a Python or Java stack it is "which engine, and is it sandboxed"; for Go it is "what is in the context, what is in the function map, and which of the two template packages is rendering the page".

### Detection and mitigation

- **Alert on template syntax in any user-supplied field.** `{{`, `}}`, `${`, `#{`, `<%`, `%>`, `{%`, `{#` and `{if}` are not text anyone types into a nickname by accident. The same rule catches attempts at the encoded forms (`%7B%7B`) and at concatenated ones.
- **Alert on evaluation results appearing where the original text should be.** This is the reliable signal, and it is the same shape as the reflected-XSS detection: **compare what was submitted with what came back**. A field that stored `{{7*7}}` and rendered `49` is the finding, and it is visible without knowing the engine.
- **Look for context leakage rather than only for execution.** `SECRET_KEY`, `AWS_`, `DATABASE_URL`, `PATH`, internal hostnames and class names appearing in a rendered page are indicators that the renderer is exposing its context — and that is a finding even when no process was spawned.
- **Treat renderer errors as information and as a signal.** A template error naming the engine, a file path and a line number tells an attacker which engine to target and confirms the input reached it. Errors should not be returned to clients, and the fact that they were is worth recording.
- **For mitigation, the strongest control is not to let users supply template source at all.** If a field needs to be customisable, give them **data** — a set of fields, a choice from a set of formats — rather than markup. A template that the application owns and the user only supplies values for cannot be injected into.
- **Where customisation genuinely requires a template, use a language that cannot evaluate.** Logic-less formats such as Mustache only interpolate; there is no expression to evaluate and therefore no ladder to climb. This is a real reduction in capability, which is exactly why it works.
- **If a full engine is unavoidable, use its sandbox — and understand what it does not cover.** The next entry measures that boundary in detail; the short version is that a sandbox restricts reaching into the language, and does nothing about what the application put in the context.
- **Keep the context minimal, whatever the engine.** `{{config}}` and `{{request.environ}}` leak because those objects were in scope. Passing the renderer exactly the values the template needs — and nothing else — removes a whole family of findings without touching the engine.

<!-- lang:zh -->
### "模板"这个词就是问题所在

"模板"听起来像一张填空的格式信：应用把名字、日期和总额填进去，引擎做替换。这个模型对每一个值得一提的引擎都是错的，而模型与事实之间的那道缝就是漏洞。

模板引擎是**一门带解释器的小语言**。它的语法里有变量、表达式、函数调用、过滤器、条件和循环，而它把这一切都求值。它缺的不是能力，而是一个沙箱 —— 而沙箱是一个独立的、可选的、经常被关掉的功能。

所以决定一切的问题不是"输入里有什么"，而是：

> **这份输入落在模板的数据里，还是落在模板的源码里？**

在 Jinja2 上实测，同一个字符串两种走法：

```python
import jinja2
env = jinja2.Environment()
user = "{{7*7}}"

env.from_string("Hello {{ name }}").render(name=user)   # 'Hello {{7*7}}'   —— 数据
env.from_string("Hello " + user).render()               # 'Hello 49'        —— 源码
```

字符串完全相同。**一次调用把它当作要打印的文本，另一次把它当作要运行的程序**，唯一的区别是它被当作哪个参数传进去。这和参数化查询对字符串拼接是同一个区分，只是发生在一门没人会当成"语言"的语言里。

### 为什么这种功能反复出现

SSTI 通常不是某处随手写了 `render(用户输入)` 的结果，它来自一个**产品决定**：

- 让客户自己写欢迎语。
- 让他们自定义邮件模板、或者通知内容。
- 让他们提供报表版面、主题、小组件。
- 让管理员在网页表单里改错误页。

上面每一个都是"让用户写模板源码"的请求，而每一个都把数据字段变成了程序字段。这个功能是正当的；它需要的是一个受限的模板语言，或者一个沙箱 —— 而后面几篇讲的就是两者都没有时会怎样。

要找的地方，是那些"某个表单字段存下来的东西之后会被渲染"的地方：欢迎语、签名、邮件正文、报表标题、看板上的说明文字、自定义错误页，以及任何管理员能编辑、用户会看到的东西。

### 求值能走多远

在无沙箱的 Jinja2 上实测，一步一级往上走：

| payload | 输出 | 它证明了什么 |
|---|---|---|
| `{{7*7}}` | `49` | 输入正在被**求值** |
| `{{7*'7'}}` | `7777777` | 引擎背后的 Python 语义够得着 —— 字符串重复，而不是报错 |
| `{{ 'abc'.upper() }}` | `ABC` | 作用域里的对象**可以调方法** |
| `{{ self.__class__ }}` | `<class 'jinja2.runtime.TemplateReference'>` | **运行时自己的对象**够得着 |
| `{{ config }}` | `{'SECRET_KEY': 'demo-secret', ...}` | 应用放进**上下文**的一切都可读 |
| `{{ [].__class__.__base__.__subclasses__() \| length }}` | `345` | 引擎能被走出去，够到**这门语言自己的类型层级** |

当成一架梯子读，四级是：

1. **求值。** 一个表达式运行了。仅这一条就是一条发现 —— 它意味着攻击者控制着一个程序。
2. **上下文访问。** 应用传给渲染器的一切现在都可读。在一个 Flask 应用里，那包括 `config`，而 `config` 里放着 `SECRET_KEY` —— 它正是签会话的那把钥匙，也是前面几篇里那个"把签名变成伪造"的东西。
3. **运行时访问。** 引擎自己用的那些对象够得着了，那是走出模板、进入语言的通道。
4. **代码执行。** 从运行时对象到一次能起进程的调用距离很短 —— 经典那条是 `cycler.__init__.__globals__.os.popen(...)`，而它背后的机制是下一篇的主题。

**所以单独一个 `{{7*7}}` 返回 `49`，意味着这四级全都可用**，因为它们之间没有任何一层是屏障。

### 分辨是哪个引擎

payload 取决于跑的是哪个引擎，而探针足够便宜，可以全发一遍。输出会告诉你谁应了。

| 探针 | Jinja2 / Twig | Freemarker | Velocity | Smarty | ERB / EJS / Mako |
|---|---|---|---|---|---|
| `{{7*7}}` | `49` | 字面 | 字面 | 字面 | 字面 |
| `${7*7}` | 字面 | `49` | 字面 | 字面 | 字面（Mako 是 `49`） |
| `#{7*7}` | 字面 | 字面 | 字面 | 字面 | 字面 |
| `<%= 7*7 %>` | 字面 | 字面 | 字面 | 字面 | `49` |
| `{7*7}` | 字面 | 字面 | 字面 | `49` | 字面 |
| `{{7*'7'}}` | `7777777` | 字面 | 字面 | 字面 | 字面 |

从这张表里拿走两点。

**起分辨作用的是 `{{7*7}}` 配 `{{7*'7'}}` 这一对。** 第一个证明某处在求值；第二个专门指认 Python 家族的引擎，因为字符串重复是 Python 的语义、不是模板的。在别的引擎里同一个表达式是报错或者是字面量 —— 而这本身就是信息。

**原样回显不等于否定结论。** 上面大多数格子里，引擎并不是在拒绝 —— 那种语法根本就不是它的语法。**只试一个探针然后断定"没有 SSTI"，正是这一类被漏掉的方式。**

而且语法不是唯一的入口。引擎也会在过滤器参数里、在块指令里、在被包含的文件里求值，所以一个把某个字段里的 `{{` 过滤掉的措施，对下一个字段什么都没说明。

### 同一件事，每种语言形状不同

走多远由引擎决定，而它们之间的差别不是程度问题 —— 有些引擎能自己走进宿主语言，有些则够不到任何不是应用亲手交给它的东西。这一块值得按技术栈逐个查，而不是想当然。

| 语言 | 引擎 | 模板能自己够到宿主语言吗 | 一个 payload 通常能拿到什么 |
|---|---|---|---|
| **Python** | Jinja2、Mako | **能** —— 经 `__class__`、`__mro__`、`__subclasses__`、`__globals__` | 代码执行 |
| **Java** | Freemarker、Velocity、Thymeleaf | **能** —— 反射与 `getClass()` 够得着 | 代码执行 |
| **Ruby** | ERB | 不存在这个问题 —— `<% %>` **就是**那门语言 | 代码执行 |
| **Node.js** | EJS、Pug、Handlebars | EJS 直接嵌 JavaScript；Handlebars 无逻辑 | 代码执行（EJS） |
| **PHP** | Twig、Smarty、Blade | Twig 按设计受限；Smarty 历史上不是 | 因引擎而异 |
| **Go** | `text/template`、`html/template` | **不能** | **只有上下文与函数表暴露出来的那些** |

**Go 值得单独一段**，因为它的形状差异大到"别的技术栈的 payload 在它这儿根本解析不过去"。在 Go 的 `text/template` 上实测：

| 模板 | 结果 |
|---|---|
| `{{7*7}}` | **解析失败** —— Go 模板没有算术表达式 |
| `{{.}}` | 整个上下文，被打印出来 |
| `{{.Secret}}` | 那个字段，若它是导出的 |
| `{{.Danger "id -un"}}` | **方法会被执行**，如果上下文那个类型上有一个 |
| `{{printf "%s-%s" .User .Secret}}` | 内置函数可用 |
| `{{index . "DB"}}` | 按键取 map 的值 |
| `{{len .Secret}}`、`{{if .Secret}}` | 函数与控制流 |

由此有两个后果，而它们改变了"一个 Go 上的发现意味着什么"。

**Go 模板无法自己爬出去。** 它没有 `__globals__` 的对应物：引擎只暴露"它拿到的数据"上的字段与方法，并调用"被注册过的函数"。所以一次 Go 模板注入的严重程度，完全由**应用往上下文里放了什么、往函数表里注册了什么**决定 —— 而那是代码评审问题，不是引擎问题。上下文里有一个会起 shell 的方法、或者 `FuncMap` 里有一个会这么做的助手，就把同一个 bug 变成代码执行；上下文里只有一个显示名，就不会。

**而 Go 上人们真正会踩的，是另一个。** 用 `text/template` 渲染 HTML **完全不做转义**，而 `html/template` 会按它正在写入的 HTML 上下文做转义：

```
text/template  -> user says: <script>alert(document.cookie)</script>
html/template  -> user says: &lt;script&gt;alert(document.cookie)&lt;/script&gt;
```

这是 XSS 那篇的问题经由一次模板选择到达，也是 Go 特有的那个该找的错误：两个包名字相似、API 相似，而安全性质完全不同。

**所以评审的问题随语言而变。** 对 Python 或 Java 栈，它是"哪个引擎、有没有沙箱"；对 Go，它是"上下文里有什么、函数表里有什么、以及渲染页面用的是哪一个模板包"。

### 检测与缓解

- **对任何用户提供的字段里出现模板语法告警。** `{{`、`}}`、`${`、`#{`、`<%`、`%>`、`{%`、`{#`、`{if}` 不是谁会在昵称里顺手打出来的文本。同一条规则也能抓到编码形式（`%7B%7B`）与拼接形式。
- **对"本该显示原文的地方出现了求值结果"告警。** 这是可靠的信号，而且它和反射型 XSS 的检测是同一个形状：**把提交的内容和回来的内容对比**。一个存了 `{{7*7}}` 却渲染出 `49` 的字段就是发现，而它在不知道引擎是什么的情况下也看得见。
- **找上下文泄漏，而不只是找执行。** `SECRET_KEY`、`AWS_`、`DATABASE_URL`、`PATH`、内部主机名与类名出现在渲染出来的页面上，是"渲染器在暴露它的上下文"的指标 —— 而即使没有起任何进程，那也是一条发现。
- **把渲染报错当作信息和信号。** 一条点名了引擎、文件路径与行号的模板错误，等于告诉攻击者该打哪个引擎，并确认输入到达了它。错误不该返回给客户端，而"它被返回了"这件事本身值得记录。
- **缓解上，最强的控制是根本不让用户提供模板源码。** 如果一个字段需要可定制，给他们**数据** —— 一组字段、从若干格式里选一个 —— 而不是标记。一个由应用拥有、用户只提供值的模板，无法被注入。
- **在定制确实需要模板的地方，用一门无法求值的语言。** Mustache 这类无逻辑格式只做插值；没有表达式可求值，因此也没有梯子可爬。这是一次真实的能力削减 —— 而它之所以有效，恰恰因为这个。
- **如果一个完整引擎不可避免，就用它的沙箱 —— 并且理解它不覆盖什么。** 下一篇会详细量那条边界；简版是：沙箱限制的是"往语言里够"，而对"应用放进了上下文的东西"什么也不做。
- **无论用什么引擎，让上下文最小。** `{{config}}` 和 `{{request.environ}}` 会泄漏，是因为那些对象在作用域里。只把模板需要的值传给渲染器 —— 别的都不给 —— 就能不碰引擎地移除一整族发现。
