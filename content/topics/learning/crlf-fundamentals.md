---
id: crlf-fundamentals
title_en: "CRLF Injection — When a Value Ends a Line"
title_zh: "CRLF 注入：当一个值结束了一行"
summary_en: HTTP frames its messages with CRLF, so a newline inside a value becomes a new line of the protocol. Measured in a live server where header injection succeeded, in a log file where a fabricated entry appeared, and across two orders of validation and decoding.
summary_zh: HTTP 用 CRLF 给消息划界，所以一个值里的换行会变成协议里新的一行。这一篇在一个真的服务器上实测（头注入成功了）、在一个日志文件里实测（多出一条伪造的记录）、并对照了校验与解码的两种顺序。
tags: [web, crlf, header-injection, response-splitting, cwe-93, cwe-113]
tools: [curl, python3, nc]
attck: [T1190, T1070]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The same two bytes, in a place that means something

HTTP is a text protocol whose structure is carried by a two-byte sequence. A header line ends with `CRLF`; the headers end with an empty line; a log line ends the same way; so does a mail header. Nothing about those bytes says "this is data" — they say it by **where they appear**.

> **A value's boundary and a message's boundary are the same pair of bytes.**

So a value that arrives from a request and is written into any of those places can end a line that the application intended to continue, and whatever follows becomes a line of protocol the application never wrote. Four layers, and the second is unusually literal:

1. **The trust boundary.** The application trusts that a value it received will only ever occupy the position it was placed in. The protocol has no notion of "this byte came from a user and is inert".
2. **Data and instruction share a plane.** A value contains both **the data** and **the instruction that this line ends here**. Whether the second reading takes effect depends entirely on which protocol consumes the string.
3. **Why the usual fix fails.** Filtering CRLF works only if the filter runs after the value has reached its final decoded form — and the order of those two steps is a decision, not a default. Measured below: the same input is refused by one ordering and accepted by the other.
4. **The variants.** A bare `LF`, a bare `CR`, percent-encoded and double-encoded forms, a filter that strips only one of the two characters, and — a genuine cross-protocol difference — the Unicode line separators `U+2028` and `U+2029`, which are line terminators in JavaScript and JSON but not in HTTP.

### Measured: header injection still works

This is the part where the current state of the art is easy to assume rather than check. Against a live Python `http.server`, with an application setting a response header from a URL parameter:

| Value submitted | Header written | What the client received |
|---|---|---|
| `hello` | accepted | `X-Greeting` |
| `hello\nX-Injected: yes` | **accepted** | `X-Greeting`, **`X-Injected`** |
| `hello\r\nX-Injected: yes` | **accepted** | `X-Greeting`, **`X-Injected`** |
| `hello\r\n\r\n<html>injected</html>` | **accepted** | the extra content landed **after the headers** |
| `hello%0d%0aX-Injected: yes` | **accepted** | `X-Greeting`, **`X-Injected`** |

**The injected header reached the client.** The server did not reject the value, and the fourth row is no longer merely adding a header — content after the blank line is a response body, which is the beginning of constructing a second response.

Two conclusions, and the second is the useful one:

**The check is not universal.** Some servers and frameworks reject CR or LF in a header value and turn the attempt into an error instead of a vulnerability; this one did not. **Whether it does is a property of the component in the path**, and with a proxy, a framework and a templating layer in the chain there are several places the decision could be made — or not.

**Which is why this is tested rather than assumed.** "Modern frameworks reject that" is a reasonable expectation and an unsafe substitute for asking this deployment. The measured row is the answer for this component, at this version.

### The same bytes in other places

The reason this class has survived is that only some of its destinations are HTTP headers, and only HTTP headers pass through an HTTP server:

| Written into | Becomes |
|---|---|
| A response header | Header injection, response splitting, an injected `Set-Cookie` or `Location` |
| A response that is cached | A split-off response stored for whoever requests the same key next — the cache entry's failure mode |
| A request header | Request splitting, which is the smuggling entry's problem from the other side |
| A log line | Log injection — a fabricated record that looks like a real one |
| A mail header | Mail header injection — an added `Bcc`, an altered `From` or `Subject` |
| A `Location` header | An arbitrary redirect target, which is the open redirect entry's problem |

**So the question is not "is CRLF dangerous" but "which protocol is this value ultimately written into".** Measured for the log case, since it is the one with no server in the middle:

```
one request, username = alice
  INFO login succeeded for user=alice

username = "alice\n2026-10-08 03:00:00 ERROR login failed for user=admin (3 attempts)"
  INFO login succeeded for user=alice
  2026-10-08 03:00:00 ERROR login failed for user=admin (3 attempts)
```

**A second record now exists in the log, and nothing distinguishes it from a real one** — same format, plausible timestamp, plausible message. Audit searches, alert rules and any investigation built on "what does the log say happened" now have a line that the application never wrote. **A log's integrity rests on one record being one line**, and a newline in a value breaks exactly that.

**And the destinations that are not HTTP headers have no server-side rejection to fall back on.** A framework that refuses CRLF in a header will happily write it into a log line, a mail header or a CSV cell, because those are strings the application assembles.

### Validation and decoding, in the wrong order

The measured pair, where the only difference is which happens first:

```
validate, then decode:
  hello%0d%0aX-Injected:%20yes  ->  allowed, decodes to 'hello\r\nX-Injected: yes'

decode, then validate:
  hello%0d%0aX-Injected:%20yes  ->  refused
```

**The same input, two opposite outcomes.** A check placed before decoding examines a string that does not yet contain the characters it is looking for, and the decoded value that reaches the output is different from the one that was checked.

This is the fourth layer of the vulnerability-essence standard this guide is built on, at a new location: **the layer that validates and the layer that decodes are not the same layer.** The same shape appears in the OAuth entry's double-encoded path, the cache entry's normalisation disagreement, and the command injection entry's `$IFS` — one rule, four settings.

### Detection and mitigation

- **Alert on `\r`, `\n`, `%0d`, `%0a` and their double-encoded forms in any user-supplied value.** At the point of entry, before anything decodes it — and treat the encoded forms as the case that matters, since the plain ones are easy to see.
- **Alert on response headers the application does not set.** A header appearing that no code path in the deployment writes is the outcome, and it is checkable by comparing against the set the application intends to emit.
- **Alert when a log record contains a second timestamp.** This is the practical detector for log injection and it does not depend on knowing which field was abused: a line with two timestamps is a line that was assembled from more than one record.
- **And treat a mail with an unexpected recipient or an altered header as an incident**, since that class is delivered to people outside the organisation and cannot be recalled.
- **For mitigation, decode to the final form first, then reject.** A value containing CRLF has no legitimate restored form in a header, a log line or a mail field, so the correct response is refusal — **not stripping the two bytes**, because deleting them maps two different inputs onto the same output, which creates a new problem while hiding the old one.
- **Do not assemble these strings by hand.** A header-setting API, a structured logger and a mail library each handle the boundary correctly as part of their job; a formatted string does not, and the measured log case is exactly what hand-assembly looks like.
- **Use structured logging rather than delimited text.** JSON records with escaping remove the "one record is one line" assumption entirely, which is what makes the fabricated-entry case impossible rather than merely detected.
- **And remember which pieces the HTTP server is not protecting.** It can refuse to write a header containing CRLF, depending on the implementation; it has no opinion about a log line or a mail header, because those never pass through it. **The further a value is from being passed to a library as a parameter, the more explicitly it has to be handled** — the same conclusion as the SQL injection and command injection entries, at a place where the protocol is text.

<!-- lang:zh -->
### 同一对字节，出现在一个有意义的位置

HTTP 是一个文本协议，它的结构由一对字节承载。一行头以 `CRLF` 结束；头以一个空行结束；一行日志以同样的方式结束；一个邮件头也是。这些字节本身没有任何东西说"这是数据" —— 它们靠**出现的位置**来说这句话。

> **一个值的边界，与一条消息的边界，是同一对字节。**

所以一个从请求里到来、又被写进上述任何一个位置的值，可以结束掉应用本想继续的一行，而后面跟着的东西就成了应用从未写过的协议行。四层，而第二层难得地字面：

1. **信任边界。** 应用信任"它收到的值只会占据它被放进的那个位置"。协议里没有"这个字节来自用户、因而是惰性的"这种概念。
2. **数据与指令共用同一平面。** 一个值里既有**数据**，又有**"这一行到此结束"这条指令**。后一种读法是否生效，完全取决于哪一个协议去消费这个字符串。
3. **为什么常见修法失败。** 过滤 CRLF 只有在"值已经到达它的最终解码形态之后"才有效 —— 而这两步的先后是一个决定，不是一个默认。下面实测：同一份输入，一种顺序拒绝它，另一种接受它。
4. **变体。** 裸 `LF`、裸 `CR`、百分号编码与双重编码形式、只删掉两个字符之一的过滤器，以及一个真实的跨协议差异 —— Unicode 行分隔符 `U+2028` 与 `U+2029`，它们在 JavaScript 与 JSON 里是行终止符，而在 HTTP 里不是。

### 实测：头注入现在仍然成立

这一部分是最容易被"想当然"而不是去查的。对着一个活的 Python `http.server`，应用把一个 URL 参数设进响应头：

| 提交的值 | 头被写入 | 客户端收到 |
|---|---|---|
| `hello` | 接受 | `X-Greeting` |
| `hello\nX-Injected: yes` | **接受** | `X-Greeting`、**`X-Injected`** |
| `hello\r\nX-Injected: yes` | **接受** | `X-Greeting`、**`X-Injected`** |
| `hello\r\n\r\n<html>injected</html>` | **接受** | 多出来的内容落在了**头之后** |
| `hello%0d%0aX-Injected: yes` | **接受** | `X-Greeting`、**`X-Injected`** |

**注入出来的头到达了客户端。** 服务器没有拒绝那个值，而第四行已经不只是加一个头 —— 空行之后的内容是响应体，那已经是"构造第二个响应"的开始。

两个结论，而第二个更有用：

**那个检查不是普遍的。** 有些服务器与框架会拒绝头值里的 CR 或 LF，把这次尝试变成一个错误而不是一个漏洞；这一个没有。**它会不会，是路径上那个组件的性质** —— 而当链路里同时有代理、框架和模板层时，那个决定可能在好几个地方做出，也可能哪里都没做。

**这就是为什么这件事要测而不是要假设。** "现代框架会拒绝"是一个合理的预期，也是一个不安全的替代品。实测那一行，才是这个组件在这个版本上的答案。

### 同一串字节，出现在别处

这一类之所以活到今天，原因是它的落点里只有一部分是 HTTP 头，而只有 HTTP 头会经过一个 HTTP 服务器：

| 被写进 | 变成什么 |
|---|---|
| 一个响应头 | 头注入、响应拆分、注入 `Set-Cookie` 或 `Location` |
| 一个会被缓存的响应 | 拆出来的响应被存给下一个请求同一个键的人 —— 缓存那篇的失效模式 |
| 一个请求头 | 请求拆分，也就是走私那篇从另一侧看的问题 |
| 一行日志 | 日志注入 —— 一条看起来像真的伪造记录 |
| 一个邮件头 | 邮件头注入 —— 加上 `Bcc`、改掉 `From` 或 `Subject` |
| 一个 `Location` 头 | 任意的跳转目标，也就是开放重定向那篇的问题 |

**所以问题不是"CRLF 危不危险"，而是"这个值最终被写进哪个协议"。** 日志那种实测如下，因为它是唯一一个中间没有服务器的：

```
一次请求，用户名 = alice
  INFO login succeeded for user=alice

用户名 = "alice\n2026-10-08 03:00:00 ERROR login failed for user=admin (3 attempts)"
  INFO login succeeded for user=alice
  2026-10-08 03:00:00 ERROR login failed for user=admin (3 attempts)
```

**日志里现在存在第二条记录，而没有任何东西把它与真的区分开** —— 同样的格式、合理的时间戳、合理的消息。审计检索、告警规则、以及任何建立在"日志说了发生了什么"之上的调查，现在有了一行应用从未写过的内容。**日志的完整性建立在"一条记录就是一行"上**，而一个值里的换行打破的正是这一条。

**而那些不是 HTTP 头的落点，没有"服务器替我拒绝"可以依靠。** 一个会拒绝头里 CRLF 的框架，会照样把它写进一行日志、一个邮件头或者一个 CSV 单元格，因为那些是应用自己拼出来的字符串。

### 校验与解码，顺序错了

实测的这一对，唯一的差别是谁先发生：

```
先校验后解码:
  hello%0d%0aX-Injected:%20yes  ->  放行，解码后是 'hello\r\nX-Injected: yes'

先解码后校验:
  hello%0d%0aX-Injected:%20yes  ->  拒绝
```

**同一份输入，两个相反的结论。** 一道放在解码之前的检查，检查的是一个还不含它要找的那些字符的字符串；而到达输出的解码后的值，与被检查过的那个不是同一个。

这是这份指南赖以成立的那套漏洞本质四层标准里的第四层，换了一个位置：**做校验的层与做解码的层不是同一层。** 同一个形状出现在 OAuth 那篇的双重编码路径、缓存那篇的规范化分歧、以及命令注入那篇的 `$IFS` —— 一条规则，四个场景。

### 检测与缓解

- **对任何用户提供的值里出现 `\r`、`\n`、`%0d`、`%0a` 以及它们的双重编码形式告警。** 在入口处、在任何东西解码它之前 —— 并且把编码形式当作要紧的那一种，因为明文的那几个一眼就看得见。
- **对"应用不会设置的那些响应头"告警。** 出现一个部署里没有任何代码路径会写的头，就是结果本身，而它可以通过与"应用打算发出的那组头"比对来检查。
- **当一条日志记录里出现第二个时间戳时告警。** 这是日志注入的实用检测器，而且它不依赖"知道是哪个字段被滥用"：一行里有两个时间戳，就是一行由不止一条记录拼出来的日志。
- **并且把"收件人非预期、或者头被改动的邮件"当作事件处理**，因为这一类是投递给组织之外的人的，收不回来。
- **缓解上，先解码到最终形态，再拒绝。** 一个含 CRLF 的值在头、日志行或邮件字段里没有合法的还原形态，所以正确的回应是拒绝 —— **而不是把那两个字节删掉**，因为删掉会把两个不同的输入映射成同一个输出，在掩盖旧问题的同时制造一个新问题。
- **不要手工拼这些字符串。** 设置头的 API、结构化日志器与邮件库各自都把手头这个边界处理正确；一个格式化字符串不会，而上面实测那个日志就是手工拼装长什么样。
- **用结构化日志，而不是用分隔符分隔的文本。** 带转义的 JSON 记录把"一条记录就是一行"这个假设整个移除了，这使伪造记录那种情况变成不可能，而不只是可检测。
- **并且记住哪些部分不是 HTTP 服务器在保护。** 它可以拒绝写一个含 CRLF 的头（取决于实现）；它对一行日志或一个邮件头没有意见，因为那些从不经过它。**一个值离"作为参数交给库"越远，就越需要被显式处理** —— 和 SQL 注入、命令注入那两篇是同一个结论，只是这里那个协议是文本。
