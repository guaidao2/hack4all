---
id: xxe-fundamentals
title_en: "XXE, Part 1 — XML Carries Instructions"
title_zh: "XXE（一）：XML 自带指令"
summary_en: DOCTYPE and ENTITY are a macro mechanism the XML specification put there on purpose, so an entity reference inside a text node can be an instruction to fetch a file or a URL. This entry covers why that is a feature rather than a parser bug, a measured table showing every parser defaults to safe and every one can be opened, and why refusing the DOCTYPE is stronger than declining to expand it.
summary_zh: DOCTYPE 与 ENTITY 是 XML 规范有意放进去的宏机制，所以一个文本节点里的实体引用可以是一条"去取某个文件或 URL"的指令。这一篇讲为什么这是规范特性而不是解析器 bug、用一张实测表说明每个解析器默认都安全而每个都能被打开，以及为什么"拒绝 DOCTYPE"比"不展开实体"更强。
tags: [web, xxe, cwe-611, xml, entity, ssrf-prep]
tools: [python3, php, java, ruby, defusedxml, Burp Suite]
attck: [T1190, T1059]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### XML is not a data format, it is a language with a macro facility

Most people meet XML as "a way to write structured data", and that is the useful part of it. But the specification also defines a **document type definition**, and the DTD defines **entities** — named references that stand for something else. Two declarations are all that is needed:

```xml
<?xml version="1.0"?>
<!DOCTYPE svg [
  <!ENTITY leak SYSTEM "file:///etc/hostname">
]>
<svg><title>&leak;</title></svg>
```

Read what each line is:

- `<!DOCTYPE ...>` declares that this document has a type definition.
- `<!ENTITY leak SYSTEM "...">` defines an entity named `leak` whose value is **the content of a resource identified by a system identifier** — the keyword `SYSTEM` literally means "fetch this".
- `&leak;` **references** it. The parser resolves the reference, fetches the resource, and substitutes its content.

That third line is the whole vulnerability: **an entity reference sits inside a text node, where a value would normally sit, and resolving it performs an I/O operation.** Data and instruction share one plane, exactly as in every other entry in this series — with the parser as the interpreter and the DTD as the instruction set.

It is worth being precise about the status of this, because it changes how the fix is chosen: **this is a documented feature of the XML specification**, added so a document could reuse content and share definitions. It is not a parser bug, not an edge case, and not going away. Any component that accepts XML and enables the feature is exposing whatever the parser can reach.

### What the parser can be made to reach

The `SYSTEM` identifier is a URI, and the parser will use whatever scheme it supports:

| Scheme | Effect |
|---|---|
| `file://` | **Read a local file** — the classic, and the one that produces data in the response |
| `http://`, `https://` | **Make a request from the server** — the bridge to SSRF, covered in its own entry |
| `ftp://`, `gopher://`, `expect://` | Depending on the runtime and installed handlers |
| `php://filter` | Read application source through a filter chain, as in the PHP entry |
| `jar:`, `netdoc:` | Runtime-specific handlers |

Two consequences follow, and both are about **position** rather than about XML:

**The parser acts with the application's privileges.** It can read whatever the process can read and connect wherever the process can connect. That is why an XXE in a service holding credentials, or one sitting inside a network segment, matters more than the same bug in a sandbox.

**The parser is a general-purpose I/O primitive that the application did not intend to offer.** The developer asked for "parse this document"; they got "read any file this process can read and fetch any URL it can reach". The gap between the intent and the capability is the vulnerability.

### Every parser defaults to safe, and every one can be opened

This is the empirical part of the entry, and it is the same question the language and archive entries asked: **what happens when you pass nothing?**

Measured with a document whose entity points at a file, on current runtimes:

| Parser | Default behaviour | After the switch is thrown |
|---|---|---|
| Python `xml.etree.ElementTree` | **Refuses** — `ParseError: undefined entity` | (no entity expansion path) |
| Python `xml.dom.minidom` | **Safe** — the reference is left unresolved (`<title/>`) | — |
| Python `xml.sax` | **Safe** — `external-general-entities` reports **0** | with the feature set to true, **the file is read** |
| PHP `DOMDocument` | **Safe** — no flags means no expansion | with `LIBXML_NOENT`, **the file is read** |
| PHP `SimpleXML` | **Safe** | — |
| Ruby `REXML` | **Safe** — the literal `&leak;` remains | — |
| Java `DocumentBuilderFactory` | **Safe** — nothing read | with `external-general-entities` enabled, **the file is read** |
| Java with `disallow-doctype-decl` | **Refuses the document outright** — `SAXParseException` | — |
| Python `lxml` | — | with `resolve_entities=True`, **the file is read** |

Reading that table top to bottom, the shape of the problem is visible:

**The defaults are safe.** Every mainstream parser starts in the state a security reviewer would have chosen. Python's `sax` reports the external-entity feature as `0`; PHP expands nothing without an explicit flag; Java reads nothing; REXML leaves the reference alone.

**The switches are one line each, and they are opened on purpose.** `resolve_entities=True` reads better than the safe default when a developer wants a document to be able to include other content. Enabling entities is what makes a "flexible" parser feel flexible. The bug is not usually introduced by carelessness — it is introduced by **choosing the more capable option**, and by copying a snippet that had already chosen it.

**Defaults are not a property of "XML", they are a property of the library.** `lxml` and `sax` are both Python XML libraries and they differ; a framework that wraps a parser may change the default again. The question "is our XML parsing safe" has to be asked of the actual code path, not of the language.

This is the mirror image of the archive entry, where `tarfile.extractall` was unsafe by default while `zipfile` was safe. **In both cases the lesson is the same and it is not about which is safer — it is that the default is a fact to be verified per library and per version, not an assumption.**

### Refusing the DOCTYPE beats declining to expand it

There are two levels of fix, and the difference matters.

**The weaker one: do not expand external entities.** Configure the parser so entity references are not resolved. The document is still parsed as XML-with-a-DTD, and the parser still processes the DTD — which means the attack surface is still present, just unexercised. A later change to a neighbouring flag, a framework upgrade that alters a default, or a different code path through the same library re-opens it.

**The stronger one: refuse documents that carry a DOCTYPE at all.** The measurement above shows the difference directly: with `disallow-doctype-decl` the parser **rejects the document** rather than parsing it inertly.

```java
DocumentBuilderFactory f = DocumentBuilderFactory.newInstance();
f.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);  // reject outright
```

The reason this is stronger is the same reason the upload entry preferred "store where nothing executes" to a better extension check: **it removes the capability rather than constraining its use.** A parser that never processes a DTD cannot be talked into resolving an entity, however the payload is written.

The per-platform settings reflect that ranking:

| Platform | The setting to reach for |
|---|---|
| **Java** | `disallow-doctype-decl` = true, plus external general and parameter entities off. `FEATURE_SECURE_PROCESSING` is a useful addition, not a substitute |
| **.NET** | `XmlReaderSettings.DtdProcessing = DtdProcessing.Prohibit` |
| **Python** | `defusedxml`, which is the maintained answer; or keep `sax`'s entity features at their defaults and stay off `lxml`'s `resolve_entities` |
| **PHP** | Do not pass `LIBXML_NOENT`; `libxml_set_external_entity_loader` to a null loader for defence in depth |
| **Ruby** | `REXML` leaves entities alone by default; with Nokogiri, `NONET` and no DTD loading |
| **Node.js** | Whatever the library calls it — check whether `noent`-style expansion is on |

And the strongest position, where the data genuinely does not need XML: **use a format that has no DTD concept.** JSON has no entity mechanism, no `SYSTEM` keyword and no way to express "fetch this" — the same argument as the deserialisation entry's.

### Where XML actually shows up

The reason this class is common is that XML arrives in places people do not think of as "XML endpoints":

| Where | Why it counts |
|---|---|
| **SVG uploads** | SVG **is** XML, and upload handlers often parse it to read metadata — the classic combination |
| **Office documents** | `.docx`, `.xlsx`, `.pptx` are ZIP archives of XML parts; preview and conversion features parse them |
| **SAML and SOAP** | XML by definition, and often reachable without authentication |
| **RSS and Atom feeds** | Parsed on the server side by aggregators and importers |
| **Configuration and data import** | "Upload your config" features |
| **Mobile and API payloads** | Older APIs default to XML, and many accept it if the content type is changed |
| **Printing and report pipelines** | XSLT and template engines that process XML |

The first two rows are the ones to remember when reviewing an upload feature: **accepting a picture and accepting an XML document can be the same act.**

### Detection and mitigation

- **Alert on DTD syntax in any input, especially uploads.** `<!DOCTYPE`, `<!ENTITY`, `SYSTEM`, `PUBLIC` and `%`-prefixed parameter entities have almost no place in ordinary business data, and their presence in an uploaded SVG or office file is a strong signal. Custom entity references of the form `&something;` that are not one of the five predefined XML entities are worth the same attention.
- **Watch what the response contains.** File contents leaking into a response have recognisable shapes — `root:x:0:0:` from a password file, `-----BEGIN` from a key, environment-variable blocks, internal hostnames. A field that echoes back something resembling a file is a finding regardless of how it got there.
- **Watch what the process connects to.** An `http://` external entity is an outbound request from the server, which puts this class in the same detection family as SSRF: connections to internal addresses, DNS for unexpected names, and requests to cloud metadata endpoints at the moment an XML document is processed.
- **Treat parse errors naming an entity as a signal.** A parser complaining about an undefined entity is a parser that was handed an entity reference, which means it was handed a document that was trying to use one.
- **Configure the parser to refuse DOCTYPEs, and verify it does.** For each parsing path, confirm the setting is in force rather than assuming the library's default, and add a test that feeds a document with an external entity and asserts a rejection. A security setting that is not covered by a test is a setting someone will change.
- **Prefer a format without the concept where possible.** For configuration, data exchange and internal APIs, JSON or another format without a DTD removes the class instead of configuring around it.
- **And keep the parser unprivileged.** The parser reads files and makes requests as the application; running that application with the least privilege and outbound restrictions bounds what a successful resolution can reach. It is not the fix, and it is what makes the difference between reading a hostname and reading a private key.

<!-- lang:zh -->
### XML 不是数据格式，它是一门带宏机制的语言

多数人认识 XML 是从"一种写结构化数据的方式"开始的，而那确实是它有用的部分。但这份规范还定义了**文档类型定义**，而 DTD 定义了**实体** —— 即"代表别的东西"的具名引用。两句声明就够了：

```xml
<?xml version="1.0"?>
<!DOCTYPE svg [
  <!ENTITY leak SYSTEM "file:///etc/hostname">
]>
<svg><title>&leak;</title></svg>
```

逐行读它是什么：

- `<!DOCTYPE ...>` 声明这份文档有一个类型定义。
- `<!ENTITY leak SYSTEM "...">` 定义了一个名叫 `leak` 的实体，它的值是**由系统标识符所指的那个资源的内容** —— 关键字 `SYSTEM` 的字面意思就是"去取这个"。
- `&leak;` **引用**它。解析器解析这个引用、取回资源、把内容替换进去。

第三行就是整个漏洞：**一个实体引用坐在文本节点里，也就是一个值本该坐的位置，而解析它执行了一次 I/O 操作。** 数据与指令共用同一个平面，和本系列其他每一篇一样 —— 只是这里的解释器是解析器，"指令集"是 DTD。

有一点值得说准，因为它决定了修法怎么选：**这是 XML 规范里白纸黑字的特性**，加进来的目的是让一份文档可以复用内容、共享定义。它不是解析器 bug，不是边角情况，也不会消失。任何接受 XML 并打开了这个特性的组件，都在暴露**解析器能够到的一切**。

### 解析器能被指使去够到什么

`SYSTEM` 标识符是一个 URI，解析器会用它支持的任何协议：

| 协议 | 效果 |
|---|---|
| `file://` | **读一个本地文件** —— 经典手法，也是能把内容带进响应的那种 |
| `http://`、`https://` | **从服务器发一个请求** —— 通向 SSRF 的桥，那一类有自己的篇目 |
| `ftp://`、`gopher://`、`expect://` | 取决于运行时与已安装的处理器 |
| `php://filter` | 通过过滤器链读应用源码，如 PHP 那篇所述 |
| `jar:`、`netdoc:` | 运行时特有的处理器 |

由此有两个后果，而它们都是关于**位置**的，不是关于 XML 的：

**解析器以应用的权限行事。** 进程能读什么它就能读什么，进程能连到哪里它就能连到哪里。这就是为什么一个持有凭据的服务里的 XXE，或者一个坐在某段内网里的 XXE，比沙箱里的同一个 bug 要紧得多。

**解析器成了应用本来没打算提供的一个通用 I/O 原语。** 开发者要的是"解析这份文档"，拿到的是"读这个进程能读的任何文件、取它能到的任何 URL"。**意图与能力之间的那个差，就是漏洞。**

### 每个解析器默认都安全，而每个都能被打开

这是本篇的实证部分，而它问的是和语言篇、归档篇同一个问题：**什么都不传的时候，它做什么？**

用一个实体指向某文件的文档，在当前运行时上实测：

| 解析器 | 默认行为 | 把那个开关扳开之后 |
|---|---|---|
| Python `xml.etree.ElementTree` | **拒绝** —— `ParseError: undefined entity` | （没有实体展开的路径） |
| Python `xml.dom.minidom` | **安全** —— 引用保持未解析（`<title/>`） | — |
| Python `xml.sax` | **安全** —— `external-general-entities` 报 **0** | 把该 feature 置真后，**读到了文件** |
| PHP `DOMDocument` | **安全** —— 不传 flags 就不展开 | 传 `LIBXML_NOENT` 后，**读到了文件** |
| PHP `SimpleXML` | **安全** | — |
| Ruby `REXML` | **安全** —— 字面的 `&leak;` 留在原地 | — |
| Java `DocumentBuilderFactory` | **安全** —— 什么都没读 | 打开 `external-general-entities` 后，**读到了文件** |
| Java 配 `disallow-doctype-decl` | **直接拒绝整份文档** —— `SAXParseException` | — |
| Python `lxml` | — | 传 `resolve_entities=True` 后，**读到了文件** |

从上到下读这张表，问题的形状就出来了：

**默认值是安全的。** 每一个主流解析器都从安全评审者会选的那个状态开始。Python 的 `sax` 把外部实体 feature 报成 `0`；PHP 不传显式 flag 就什么都不展开；Java 什么都不读；REXML 把引用原样留着。

**开关都只有一行，而且都是被有意扳开的。** 当一个开发者希望一份文档能引入别的内容时，`resolve_entities=True` 读起来比安全默认更顺眼。打开实体，正是让一个"灵活"的解析器显得灵活的东西。这个 bug 通常不是粗心引进来的 —— 是**选择了能力更强的那个选项**引进来的，以及照抄了一段已经选了它的代码。

**默认值不是"XML"的性质，而是某个库的性质。** `lxml` 和 `sax` 都是 Python 的 XML 库，而它们不一样；一个包装了解析器的框架可能又改一次默认。所以"我们的 XML 解析安不安全"这个问题，必须问**实际那条代码路径**，而不是问语言。

这是归档那篇的镜像，那边 `tarfile.extractall` 默认不安全而 `zipfile` 安全。**两种情况下的教训是同一条，而且与谁更安全无关 —— 默认值是一个要按库、按版本去核实的事实，不是一个可以假设的东西。**

### 拒绝 DOCTYPE，强于拒绝展开

修法有两个层次，差别是要紧的。

**较弱的那层：不展开外部实体。** 把解析器配置成不解析实体引用。文档仍然被当作"带 DTD 的 XML"解析，解析器仍然处理 DTD —— 这意味着攻击面仍然在场，只是没被使用。日后邻近某个 flag 的改动、一次改变默认值的框架升级，或者同一个库里另一条代码路径，都会把它重新打开。

**较强的那层：直接拒绝携带 DOCTYPE 的文档。** 上面的实测把差别摆在眼前：配上 `disallow-doctype-decl` 之后，解析器是**拒绝这份文档**，而不是把它惰性解析掉。

```java
DocumentBuilderFactory f = DocumentBuilderFactory.newInstance();
f.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);  // 直接拒绝
```

它更强的理由，和上传那篇更偏好"存到不会执行的地方"而不是一个更好的扩展名检查，是同一个：**它移除的是能力，而不是限制能力的用法。** 一个从不处理 DTD 的解析器，无论载荷怎么写，都无法被说服去解析一个实体。

各平台的设置也反映这个排序：

| 平台 | 该去够的那个设置 |
|---|---|
| **Java** | `disallow-doctype-decl` 置真，同时关掉外部通用实体与参数实体。`FEATURE_SECURE_PROCESSING` 是有用的补充，不是替代品 |
| **.NET** | `XmlReaderSettings.DtdProcessing = DtdProcessing.Prohibit` |
| **Python** | `defusedxml`，这是被维护的那个答案；或者让 `sax` 的实体 feature 保持默认、并且不要打开 `lxml` 的 `resolve_entities` |
| **PHP** | 不要传 `LIBXML_NOENT`；纵深防御上把 `libxml_set_external_entity_loader` 设成空加载器 |
| **Ruby** | `REXML` 默认就不碰实体；用 Nokogiri 时加 `NONET` 且不加载 DTD |
| **Node.js** | 看具体库怎么叫它 —— 核实那个 `noent` 风格的展开是不是开着的 |

而在数据确实不需要 XML 的地方，最强的立场是：**用一个没有 DTD 概念的格式。** JSON 没有实体机制、没有 `SYSTEM` 关键字、也没有任何方式表达"去取这个" —— 和反序列化那篇同一个论证。

### XML 实际出现在哪里

这一类之所以常见，是因为 XML 会出现在人们不当作"XML 端点"的地方：

| 位置 | 为什么算数 |
|---|---|
| **SVG 上传** | SVG **就是** XML，而上传处理器常常解析它来读元数据 —— 经典组合 |
| **Office 文档** | `.docx`、`.xlsx`、`.pptx` 是 XML 分片的 ZIP 归档；预览与转换功能会解析它们 |
| **SAML 与 SOAP** | 按定义就是 XML，而且常常无需认证即可到达 |
| **RSS 与 Atom 订阅源** | 由聚合器和导入器在服务端解析 |
| **配置与数据导入** | "上传你的配置文件"这类功能 |
| **移动端与 API 载荷** | 较老的 API 默认 XML，而且很多在改一下 Content-Type 后也接受它 |
| **打印与报表流水线** | 处理 XML 的 XSLT 与模板引擎 |

评审一个上传功能时，前两行是要记住的：**接受一张图片，和接受一份 XML 文档，可以是同一件事。**

### 检测与缓解

- **对任何输入里的 DTD 语法告警，尤其是上传。** `<!DOCTYPE`、`<!ENTITY`、`SYSTEM`、`PUBLIC` 以及 `%` 开头的参数实体，在普通业务数据里几乎没有位置，而它们出现在上传的 SVG 或 Office 文件里就是一个强信号。形如 `&something;`、且不属于那五个预定义 XML 实体的自定义实体引用，同样值得注意。
- **看响应里有什么。** 泄漏到响应里的文件内容有可辨识的形状 —— 口令文件里的 `root:x:0:0:`、密钥的 `-----BEGIN`、环境变量块、内部主机名。一个回显了类似文件内容的字段就是一条发现，无论它是怎么到那儿的。
- **看进程连到了哪里。** 一个 `http://` 外部实体就是一次从服务器发出的请求，这把这一类放进和 SSRF 同一族检测里：内网地址的连接、意外域名的 DNS 查询、以及在某份 XML 被处理的那一刻访问云元数据端点。
- **把"报出实体名的解析错误"当信号。** 一个抱怨"未定义实体"的解析器，就是被塞了一个实体引用；也就是说，它被塞了一份试图使用它的文档。
- **把解析器配置成拒绝 DOCTYPE，并核实它真的拒绝了。** 对每一条解析路径，确认那个设置是生效的而不是假设库的默认，并加一条测试：喂一份带外部实体的文档、断言它被拒绝。**一项没有被测试覆盖的安全设置，就是一项迟早会被人改掉的设置。**
- **能不用这个概念的格式就不用。** 对配置、数据交换和内部 API，JSON 或其他没有 DTD 的格式是移除这一类，而不是为它做配置。
- **并且让解析器没有权限。** 解析器以应用的身份读文件、发请求；用最小权限与出网限制运行那个应用，能限制一次成功的解析能够到什么。它不是修法，而它决定"读到一个主机名"与"读到一把私钥"之间的差别。
