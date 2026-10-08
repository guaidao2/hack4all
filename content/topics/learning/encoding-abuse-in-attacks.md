---
id: encoding-abuse-in-attacks
title_en: "Encoding, Part 4 — Encoding Differences as an Attack Surface"
title_zh: "编码（四）：编码差异作为攻击面"
summary_en: Every technique in this entry has the same shape — a validator and a consumer take different views of the same bytes. Wide-byte injection, double decoding, overlong UTF-8, normalization and parameter pollution are variations on one root cause, and so is the defence, which is to decode once, at the right layer, and hand a canonical value to everything downstream.
summary_zh: 这一篇里的每一种手法都是同一个形状 —— 校验器和使用者对同一串字节取了不同的视图。宽字节注入、二次解码、过长 UTF-8、规范化与参数污染，都是同一个根因的变体；防御也是同一个：在正确的层次上解码一次，然后把规范形式交给下游的一切。
tags: [encoding, waf-bypass, sql-injection, unicode, canonicalization, homoglyph]
tools: [Python, Burp Suite, curl, sqlmap]
attck: [T1027, T1190, T1140]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### One root cause, five faces

Everything in this entry is the same bug wearing different clothes:

> **A validator and a consumer look at the same input and see different things.**

Neither is buggy in isolation. The validator faithfully checks the bytes it received. The consumer faithfully decodes them. They disagree because **they are operating at different layers of interpretation** — one is reasoning about characters, the other about bytes; one decodes once, the other decodes twice; one sees the canonical form, the other sees a compatibility variant.

That is the framing worth holding onto, because it explains both halves of the problem. It explains why **blocklists never converge**: you are not patching a missing character, you are trying to enumerate every spelling of every dangerous character in every encoding, and the space is unbounded. And it explains why the effective defences are structural rather than lexical: **decode fully, normalize once at the boundary, and validate the value that will actually be used** — never a different representation of it.

The four entries before this one exist so that this one can be concrete. The byte structures come from there; the attacks are here.

### Wide-byte injection

The classic, and the clearest example of the whole family. It needs three conditions, and all three are ordinary:

1. **The application uses a multi-byte charset where a trail byte can be `0x5C`** — GBK, Shift-JIS and Big5 all qualify.
2. **Escaping is done byte-wise**, as `addslashes()` and similar functions do: see a quote byte, insert a backslash byte before it.
3. **The database interprets the bytes as multi-byte**, which it does, because the connection charset says so.

**The mechanism, byte by byte.** The attacker sends `%df%27` as a parameter value:

```
after URL decoding:      DF 27
escape sees 27 ('):      DF 5C 27        <- inserted a backslash before the quote
GBK decodes the pair:    DF5C is one valid GBK character
what the database sees:  <one character>  followed by 27 = '
```

**The backslash was eaten.** `DF 5C` is a legal GBK character, so the database consumes both bytes as a single character and the quote — the byte that was supposed to be neutralised — is standing free. The escaped quote is no longer escaped.

Look at what each layer believed:

| Layer | What it thought it was doing |
|---|---|
| Escaping function | "I saw a quote byte and put a backslash in front of it. It is now safe." |
| Database | "Those two bytes are one character. Here is a quote after it." |

Both were right about their own model. **The bug is that they had different models**, and the escaping happened at a layer below the one where the meaning is decided. That is why adding `0x5C` to a blocklist does not help: the byte is not the problem, the layer is.

```python
def escape_like_php(data: bytes, quote=0x27) -> bytes:
    """What addslashes does: byte-wise, no idea what a character is."""
    out = bytearray()
    for b in data:
        if b in (quote, 0x22, 0x5C):      # ' " \
            out.append(0x5C)
        out.append(b)
    return bytes(out)

payload = bytes.fromhex('df27')            # %df%27 after URL decoding
escaped = escape_like_php(payload)
print('escaped  :', escaped.hex())          # df5c27  — looks escaped

# now decode the way a GBK-aware database would
decoded = escaped.decode('gbk', errors='replace')
print('as gbk   :', repr(decoded))
print('quote survived:', '27' in escaped.hex() and decoded.endswith("'"))
```

The last line is the demonstration: the database sees a quote, and the escaping layer thinks it neutralised one.

**Variants and relatives:**

| Encoding | A lead/trail pair that swallows a backslash |
|---|---|
| GBK | `%df%5c`, and anything ending in `%5c` after a valid lead byte |
| Shift-JIS | lead `0x81`–`0x9F` with trail `0x5C` |
| Big5 | lead `0xA1`–`0xF9` with trail `0x5C` |

The defence is not "filter `0xDF`". It is to stop escaping and start **parameterising**, which is the point the entry returns to at the end: a prepared statement sends the query structure and the data through separate channels, so no byte sequence in the data can be reinterpreted as structure, regardless of charset. That is a stronger statement than "we escape correctly", and it is the same reasoning as the one in the guided-query example of the SQL entry.

### Double decoding

The second face: **the validator decodes once, the consumer decodes again.**

```
attacker sends:   %2527
first decode:     %27          <- the validator sees a percent sign and digits: harmless
second decode:    '            <- the consumer sees a quote
```

Any component that decodes URL data more than once — a proxy that unescapes, then an application that also unescapes — has this gap. The number of `%25` layers shows how many decode passes a payload is designed to survive:

| Payload | Decodes to, after n passes |
|---|---|
| `%27` | `'` after 1 |
| `%2527` | `'` after 2 |
| `%252527` | `'` after 3 |
| `%25%32%37` | `'` after 2 (the second layer is itself percent-encoded) |

And it generalises past URLs: **any two decoders in a chain that disagree about how many times to decode**. URL then HTML entity, then Unicode escape, then base64. Each combination is its own gap.

```python
from urllib.parse import unquote

payload = '%252527'
for i in range(1, 4):
    payload = unquote(payload)
    print(f'decode {i}: {payload!r}')
```

**The defensive rule that follows**: decode **exactly once**, at a defined point, and then **reject** any input that still contains encoding after that point. If a value should be a filename and it still contains `%27` after decoding, that is not a filename — it is malformed input, and rejecting it closes the gap rather than leaving it for the next component.

### Overlong UTF-8

The third face, and it depends on a decoder being lenient when it should be strict.

UTF-8 has one canonical encoding per code point, but the byte patterns permit other, longer spellings. `/` is `U+002F`, canonically `2F` in one byte. It can also be written:

| Bytes | Bytes used | Decodes to |
|---|---|---|
| `2F` | 1 | `/` |
| `C0 AF` | 2 | `/` |
| `E0 80 AF` | 3 | `/` |
| `F0 80 80 AF` | 4 | `/` |

All four decode to the same character in a decoder that does not enforce canonicity, and **only the first is legal UTF-8**. A filter checking for the byte `0x2F` sees none of the others; a lenient decoder downstream produces a slash from any of them.

The path traversal shape: `..%c0%af..%c0%afetc%c0%afpasswd` becomes `../../etc/passwd` in a lenient decoder, while a check for `../` sees nothing resembling it.

```python
for hexbytes in ['2f', 'c0af', 'e080af']:
    raw = bytes.fromhex(hexbytes)
    strict = 'rejected'
    try:
        strict = raw.decode('utf-8')
    except UnicodeDecodeError:
        pass
    lenient = raw.decode('utf-8', errors='replace')
    print(f'{hexbytes:8} strict={strict!r:12} lenient={lenient!r}')
```

**Modern status**: Python, Go, Rust and current C libraries reject overlong sequences in their standard decoders, and the classic IIS and Java exploits from the 2000s are patched. So why keep it in the list? Because **the check that matters is on the target you are testing**, and there are still parsers in embedded systems, old application servers, custom protocol implementations and hand-written decoders that accept them. The second reason is more durable: overlong encoding is the cleanest illustration of the canonicalization principle. When you understand why `C0 AF` is dangerous, you understand why "decode fully, then re-encode and compare" is a defence, and that reasoning applies to every other face in this entry.

### Normalization differences

The fourth face. Unicode permits several code point sequences to represent the same text, and NFKC deliberately folds more — fullwidth forms, ligatures, superscripts — into their common equivalents.

| Input | Code point | After NFKC |
|---|---|---|
| `＜` | U+FF1C | `<` |
| `＞` | U+FF1E | `>` |
| `（` | U+FF08 | `(` |
| `Ⅸ` | U+2168 | `IX` |
| `ﬁ` | U+FB01 | `fi` |

So a validator that rejects `<` and a consumer that normalizes with NFKC before use **disagree about the same string**:

```
attacker sends:   ＜script＞alert(1)＜/script＞
validator sees:   no 0x3C bytes, and the literal string "<" does not appear  -> allowed
consumer applies NFKC: <script>alert(1)</script>
```

```python
import unicodedata

payload = '\uff1cscript\uff1ealert(1)\uff1c/script\uff1e'
print('raw contains <:', '<' in payload)                     # False
print('nfkc contains <:', '<' in unicodedata.normalize('NFKC', payload))   # True
```

The relevant question is never "is this payload dangerous" but "**which form does the component that matters see**". If the answer differs from the one your check used, the check is decorative.

### Confusables and invisible characters

The fifth face, and the one that breaks string comparison rather than decoding.

A blocklist containing `admin` does not match `аdmin` with a Cyrillic `а`. Neither does it match `adm\u200bin` with a zero-width space. If the consumer treats both as acceptable values — because a database comparison is byte-wise, or because it strips zero-width characters — then the check and the consumer have different views again.

| Trick | Example | Breaks |
|---|---|---|
| Homoglyph | `аdmin` (Cyrillic а) | Blocklists and string equality |
| Zero-width insertion | `ad\u200bmin` | Keyword matching, if the consumer ignores them |
| Fullwidth | `ａdmin` | Matching, until NFKC runs |
| Case folding | `ADMIN` vs `admin` | Naive case-sensitive blocklists |
| Combining marks | `adm\u0301in` | Matching, and sometimes length checks |

The subtlety worth noting: **some of these normalise away and some do not.** Fullwidth `ａ` becomes `a` under NFKC, so a consumer that normalizes will treat `ａdmin` as `admin` — while a validator that did not normalize saw something else. The Cyrillic `а` never becomes Latin `a` under any normalization, so it stays different forever, which is why it works for phishing and impersonation rather than for keyword bypass.

### The general shape: validator and consumer disagree

Once you have the five faces, you can classify almost any filter bypass you meet by asking one question: **what is different between what the check saw and what the code used?**

| Bypass class | What differs | Example |
|---|---|---|
| **Encoding** | The encoding layer | `%df%27` wide-byte; `%2527` double decode; `C0 AF` overlong |
| **Case** | The comparison's case sensitivity | `SeLeCt`, `UnIoN` |
| **Whitespace** | What counts as a separator | `/**/`, `%09`, `%0a`, `+` |
| **Comments** | Whether comments are stripped before matching | `/*!50000SELECT*/`, `UN/**/ION` |
| **Equivalents** | Which functions the blocklist knows | `SUBSTR` vs `MID` vs `SUBSTRING` |
| **Truncation** | Where the value ends | `admin\0x`, over-long input cut at a fixed length |
| **Parameter pollution** | Which of several values is taken | `?id=1&id=2` |
| **Normalization** | Which Unicode form is used | `＜` vs `<` |
| **Confusables** | The code points, not the glyphs | Cyrillic `а` |

**The table is the point.** A blocklist is an attempt to write down every cell in it, and the table is unbounded. The defence has to change the question from "did we catch this spelling" to "is there any spelling that reaches the interpreter as structure".

#### Two mechanisms worth their own paragraph

**NUL truncation.** A C-style string ends at the first `0x00`, but the buffer holding it knows its length separately. So a value like `admin\0evil` can be `admin` to one component and `admin\0evil` to another — pass the length check with the full string, then have the C function see only the first part. This is the mechanism behind a large family of file-extension and path checks that were bypassed by appending a NUL. Modern languages use length-prefixed strings and reject NUL in most contexts, but the pattern persists at boundaries with C libraries, in file paths on some systems, and wherever an application hands input to a native function.

**HTTP parameter pollution.** If a request contains `?role=user&role=admin`, then:

- A framework that takes the **first** value sees `user`.
- A framework that takes the **last** sees `admin`.
- A framework that builds an **array** sees both.
- A WAF inspecting the raw query string may see the first, or neither, depending on its parser.

A validator written in one component and a consumer written in another can therefore disagree **about the same request**, with no encoding trickery involved at all. The defence is a convention — duplicate parameter names are either rejected or have a defined, documented resolution — applied consistently at every layer, because the vulnerability is not the duplication, it is the inconsistency.

### Detection and mitigation

- **The single most useful detection idea: record two views.** Log the raw input **and** its fully decoded, normalized form, and alert when they disagree in a way that matters — a value that contains percent-encoding after decoding, bytes that are not valid UTF-8, characters from mixed scripts, characters outside the expected alphabet for that field. Neither view alone shows the problem; the comparison does. This is also the practical answer to "why did the WAF miss it": the WAF saw one view and the application used another.
- **Decode to the innermost layer, then reject what is left.** One decode, at a defined point. If a supposedly plain value still contains `%`, `&#`, `\u`, or a NUL after that, it is malformed and should be rejected rather than passed on. This closes double decoding and truncation at the same time.
- **Use canonicalization checks where the language supports them.** Decode to characters, re-encode, and compare with the original bytes. If they differ, the input was non-canonical — an overlong sequence, a non-normalized form, a different encoding. Rejecting non-canonical input is one control that covers several rows of the table above, and it is the same test whether the problem came from UTF-8, GBK or normalization.
- **Normalize exactly once, before validation, in a documented form** — and then use that same value. The bug is never "normalization exists"; it is "normalization happened in one place and validation in another".
- **Stop escaping and start parameterising.** Every escape-based defence in this entry is a filter on a representation, and representations are exactly what an attacker controls. Parameterised queries, structured argument lists (`execve` with an array rather than a shell string), prepared statements and typed APIs do not filter — they **separate the channel**, so no byte sequence in the data can become structure. This is the structural fix, and it is the same argument as the modular-arithmetic reasoning in the classical cipher entries: solve the class, not the instance.
- **Allowlist, rather than denylist, wherever the domain permits.** A field that should be a username or a filename has a small legitimate alphabet. Listing what is allowed eliminates every row of the bypass table at once, and it does not need updating when a new Unicode character or a new encoding is invented.
- **Declare the encoding and enforce it.** Set the connection charset explicitly on database links, send `Content-Type` with a charset, use UTF-8 end to end, and use a strict decoder that rejects overlong and invalid sequences. Most wide-byte and overlong bugs exist because the encoding was left implicit or the decoder was lenient.
- **Agree on duplicate parameters, once.** Pick a resolution — reject, or take the first, or take the last — write it down, implement it in every layer that parses a request, and test it. The vulnerability in parameter pollution is the disagreement, so the mitigation is a decision that everyone follows.
- **And for defenders reading an alert: look at the encoding, not just the payload.** A request containing `%25`, an overlong sequence, a mixed-script value or a duplicated parameter is a different signal from a request containing an obvious attack string. The first kind is someone probing how your layers interpret input, which is a more meaningful signal than the payload they happened to send.

<!-- lang:zh -->
### 一个根因，五张面孔

这一篇里的所有东西都是同一个 bug 换了衣服：

> **校验器和使用者看着同一份输入，却看到了不同的东西。**

单独看，谁都没错。校验器忠实地检查它收到的字节；使用者忠实地解码它们。它们之所以不一致，是因为**它们工作在解释的不同层次上** —— 一个在字符层面推理，另一个在字节层面；一个解码一次，另一个解码两次；一个看到的是规范形式，另一个看到的是兼容变体。

这个框架值得抓住，因为它同时解释了这个问题的两半。它解释了**为什么黑名单永远收敛不了**：你不是在补一个漏掉的字符，你是在试图枚举每一种危险字符在每一种编码下的每一种拼法，而这个空间是无界的。它也解释了为什么有效的防御是结构性的、而不是词法性的：**完整解码、在边界处只规范化一次、校验那个真的会被用到的值** —— 绝不是它的另一个表示。

前面四篇存在的意义，就是让这一篇能具体起来。字节结构来自那里，攻击在这里。

### 宽字节注入

经典手法，也是整族里最清楚的例子。它需要三个条件，而三个都很平常：

1. **应用使用一种多字节字符集，其中尾字节可以是 `0x5C`** —— GBK、Shift-JIS、Big5 都满足。
2. **转义是按字节做的**，就像 `addslashes()` 这类函数：看到一个引号字节，就在它前面插一个反斜杠字节。
3. **数据库把这些字节按多字节解释**，而它确实会这么解释，因为连接字符集就是这么设的。

**逐字节看机制。** 攻击者把一个参数值发成 `%df%27`：

```
URL 解码之后：        DF 27
转义看到 27（'）：      DF 5C 27        <- 在引号前插入了反斜杠
GBK 解码这一对：        DF5C 是一个合法的 GBK 字符
数据库看到的：         <一个字符>  后面跟着 27 = '
```

**反斜杠被吃掉了。** `DF 5C` 是一个合法的 GBK 字符，所以数据库把两个字节当成一个字符消费掉，而那个引号 —— 本应被中和的那个字节 —— 独立地站在那里。被转义的引号不再被转义。

看看每一层各自相信自己在做什么：

| 层 | 它以为自己在做什么 |
|---|---|
| 转义函数 | "我看到一个引号字节，在前面放了反斜杠。它现在安全了。" |
| 数据库 | "那两个字节是一个字符。这后面跟着一个引号。" |

两边对各自的模型都没说错。**bug 在于它们持有不同的模型**，而转义发生的层次，低于"意义在哪里被决定"的那个层次。这就是为什么把 `0x5C` 加进黑名单没用：**问题不是那个字节，而是那个层次。**

```python
def escape_like_php(data: bytes, quote=0x27) -> bytes:
    """addslashes 做的事：按字节，完全不知道什么叫字符。"""
    out = bytearray()
    for b in data:
        if b in (quote, 0x22, 0x5C):      # ' " \
            out.append(0x5C)
        out.append(b)
    return bytes(out)

payload = bytes.fromhex('df27')            # URL 解码后的 %df%27
escaped = escape_like_php(payload)
print('转义后  :', escaped.hex())           # df5c27 —— 看起来被转义了

# 现在按一个懂 GBK 的数据库那样解码
decoded = escaped.decode('gbk', errors='replace')
print('按 GBK  :', repr(decoded))
print('引号是否存活:', decoded.endswith("'"))
```

最后一行就是演示：数据库看到了一个引号，而转义层以为它中和了一个。

**变体与亲族：**

| 编码 | 会吞掉反斜杠的首尾字节对 |
|---|---|
| GBK | `%df%5c`，以及任何在合法首字节之后以 `%5c` 结尾的 |
| Shift-JIS | 首字节 `0x81`–`0x9F` 配尾字节 `0x5C` |
| Big5 | 首字节 `0xA1`–`0xF9` 配尾字节 `0x5C` |

防御不是"过滤 `0xDF`"。而是**停止转义、改用参数化** —— 这正是这一篇最后要回到的点：预编译语句把查询结构和数据放在**两条独立通道**上传输，所以数据里的任何字节序列都不可能被重新解释成结构，不管字符集是什么。这是一个比"我们转义得对"强得多的陈述，而它和 SQL 那一篇里讲的原理是同一条。

### 二次解码

第二张面孔：**校验器解码一次，使用者又解一次。**

```
攻击者发送：  %2527
第一次解码：  %27          <- 校验器看到百分号加数字：无害
第二次解码：  '            <- 使用者看到一个引号
```

任何对 URL 数据解码超过一次的组件 —— 一个做反转义的代理，加上一个也做反转义的应用 —— 都有这个缺口。`%25` 的层数，说明这个载荷被设计成能扛过几次解码：

| 载荷 | 经过 n 次解码后 |
|---|---|
| `%27` | 1 次后是 `'` |
| `%2527` | 2 次后是 `'` |
| `%252527` | 3 次后是 `'` |
| `%25%32%37` | 2 次后是 `'`（第二层本身也是百分号编码的） |

而且它不止于 URL：**链条上任何两个对"解码几次"意见不一致的解码器**。URL 之后是 HTML 实体、再之后是 Unicode 转义、再之后是 base64 —— 每一种组合都是它自己的缺口。

```python
from urllib.parse import unquote

payload = '%252527'
for i in range(1, 4):
    payload = unquote(payload)
    print(f'第 {i} 次解码: {payload!r}')
```

**由此推出的防御规则**：在定义的某一点上**恰好解码一次**，然后**拒绝**任何在那之后仍然含有编码的输入。如果一个本该是文件名的值在解码后还含 `%27`，那它就不是文件名 —— 它是畸形输入，拒绝它才是把缺口关上，而不是留给下一个组件。

### 过长 UTF-8

第三张面孔，它依赖的是解码器在该严格的地方宽松了。

UTF-8 对每个码点只有一种规范编码，但字节模式允许其他更长的拼法。`/` 是 `U+002F`，规范形式是一字节的 `2F`。它也可以写成：

| 字节 | 用了几字节 | 解出来是 |
|---|---|---|
| `2F` | 1 | `/` |
| `C0 AF` | 2 | `/` |
| `E0 80 AF` | 3 | `/` |
| `F0 80 80 AF` | 4 | `/` |

在一个不强制规范性的解码器里，这四种都解出同一个字符，而**只有第一种是合法 UTF-8**。检查 `0x2F` 这个字节的过滤器一个都看不到；下游宽松的解码器从任何一个都能产出斜杠。

路径穿越的形状：`..%c0%af..%c0%afetc%c0%afpasswd` 在宽松解码器里变成 `../../etc/passwd`，而检查 `../` 的东西看不到任何相似的东西。

```python
for hexbytes in ['2f', 'c0af', 'e080af']:
    raw = bytes.fromhex(hexbytes)
    strict = '已拒绝'
    try:
        strict = raw.decode('utf-8')
    except UnicodeDecodeError:
        pass
    lenient = raw.decode('utf-8', errors='replace')
    print(f'{hexbytes:8} 严格={strict!r:12} 宽松={lenient!r}')
```

**现代状况**：Python、Go、Rust 以及当前的 C 库，标准解码器都会拒绝过长序列，2000 年代那些经典的 IIS 与 Java 漏洞也已经补上。那为什么还留着它？因为**真正要紧的检查是做在你正在测的目标上的**，而嵌入式系统、老应用服务器、自定义协议实现和手写解码器里，仍然有接受它们的解析器。第二个理由更持久：**过长编码是"规范化原则"最干净的例证。** 当你明白了 `C0 AF` 为什么危险，你就明白了"完整解码、重新编码、比较是否一致"为什么是一种防御 —— 而这个推理适用于这一篇里的每一张面孔。

### 规范化差异

第四张面孔。Unicode 允许几种码点序列表示同一段文本，而 NFKC 还刻意折叠得更多 —— 全角形式、连字、上标 —— 都折成它们的常见等价物。

| 输入 | 码点 | NFKC 之后 |
|---|---|---|
| `＜` | U+FF1C | `<` |
| `＞` | U+FF1E | `>` |
| `（` | U+FF08 | `(` |
| `Ⅸ` | U+2168 | `IX` |
| `ﬁ` | U+FB01 | `fi` |

所以一个拒绝 `<` 的校验器，和一个在使用前先做 NFKC 规范化的使用者，**对同一个字符串的理解不一致**：

```
攻击者发送：   ＜script＞alert(1)＜/script＞
校验器看到：   没有 0x3C 字节，字面串 "<" 也没出现  -> 放行
使用者做 NFKC：<script>alert(1)</script>
```

```python
import unicodedata

payload = '\uff1cscript\uff1ealert(1)\uff1c/script\uff1e'
print('原始里含 <:', '<' in payload)                     # False
print('NFKC 后含 <:', '<' in unicodedata.normalize('NFKC', payload))   # True
```

相关的问题从来不是"这个载荷危不危险"，而是"**真正要紧的那个组件看到的是哪种形式**"。如果答案和你检查时用的那个不一致，你的检查就只是装饰。

### 同形字与不可见字符

第五张面孔，也是破坏字符串比较而不是破坏解码的那张。

一份含 `admin` 的黑名单匹配不上带西里尔 `а` 的 `аdmin`，也匹配不上带零宽空格的 `adm\u200bin`。如果使用者把两者都当作可接受的值 —— 因为数据库比较是按字节的，或者因为它会剥掉零宽字符 —— 那么检查和使用者又对同一份输入产生了不同的视图。

| 手法 | 例子 | 破坏了什么 |
|---|---|---|
| 同形字 | `аdmin`（西里尔 а） | 黑名单与字符串相等判断 |
| 插入零宽字符 | `ad\u200bmin` | 关键词匹配（如果使用者忽略它们） |
| 全角 | `ａdmin` | 匹配 —— 直到 NFKC 跑起来 |
| 大小写折叠 | `ADMIN` 对 `admin` | 天真的区分大小写黑名单 |
| 组合附加符 | `adm\u0301in` | 匹配，有时还有长度检查 |

有个微妙之处值得留心：**这里面有些会被规范化掉，有些永远不会。** 全角 `ａ` 在 NFKC 下变成 `a`，所以一个会做规范化的使用者会把 `ａdmin` 当成 `admin` —— 而一个没做规范化的校验器看到的是别的东西。西里尔 `а` 在任何规范化下都不会变成拉丁 `a`，所以它永远是不同的 —— 这也是为什么它用于钓鱼和冒充，而不是用于关键词绕过。

### 通用的形状：校验器与使用者不一致

有了这五张面孔，你几乎可以把遇到的任何过滤器绕过归到一类，只问一个问题：**检查看到的东西和代码用的东西，差在哪里？**

| 绕过类别 | 差在哪 | 例子 |
|---|---|---|
| **编码** | 编码层 | `%df%27` 宽字节；`%2527` 二次解码；`C0 AF` 过长 |
| **大小写** | 比较是否区分大小写 | `SeLeCt`、`UnIoN` |
| **空白** | 什么算分隔符 | `/**/`、`%09`、`%0a`、`+` |
| **注释** | 匹配前是否剥掉注释 | `/*!50000SELECT*/`、`UN/**/ION` |
| **等价物** | 黑名单认得哪些函数 | `SUBSTR` 对 `MID` 对 `SUBSTRING` |
| **截断** | 值在哪里结束 | `admin\0x`、超长输入被截到固定长度 |
| **参数污染** | 多个值里取哪一个 | `?id=1&id=2` |
| **规范化** | 用哪种 Unicode 形式 | `＜` 对 `<` |
| **同形字** | 码点而不是字形 | 西里尔 `а` |

**这张表就是要点。** 黑名单是想把表里每一格都写下来，而这张表是无界的。防御必须把问题从"我们抓到这种拼法了吗"换成"**有没有哪一种拼法，能让它到达解释器时变成结构**"。

#### 两个值得单独成段的机制

**NUL 截断。** C 风格的字符串在第一个 `0x00` 处结束，但保存它的缓冲区另外知道自己的长度。于是一个像 `admin\0evil` 的值，对一个组件来说是 `admin`，对另一个来说是 `admin\0evil` —— 用完整字符串通过长度检查，然后让那个 C 函数只看到前一部分。这是很大一类文件扩展名与路径检查被绕过的机制（在末尾追加一个 NUL）。现代语言用带长度前缀的字符串，并在多数场景拒绝 NUL，但这个模式在与 C 库的边界上、在某些系统的文件路径里，以及应用把输入交给原生函数的任何地方，都还在。

**HTTP 参数污染。** 如果一个请求包含 `?role=user&role=admin`，那么：

- 取**第一个**值的框架看到 `user`。
- 取**最后一个**值的框架看到 `admin`。
- 构建**数组**的框架两个都看到。
- 检查原始查询串的 WAF，可能看到第一个，也可能两个都看不到，取决于它的解析器。

于是一个写在一个组件里的校验器，和一个写在另一个组件里的使用者，可以**对同一个请求**产生分歧，这里完全没有用到任何编码花招。防御是一个约定 —— 重复参数名要么拒绝、要么有一个定义明确且写下来的处理方式 —— 并在每一层一致执行，因为漏洞不是"重复"，而是"不一致"。

### 检测与缓解

- **最有用的一个检测思路：记录两种视图。** 把原始输入**和**它完整解码、规范化之后的形式都记下来，并在两者出现要紧的分歧时告警 —— 解码之后仍含百分号编码的值、不是合法 UTF-8 的字节、混合脚本的字符、超出该字段预期字母表的字符。**单独看哪一种视图都看不出问题，两者对比才能。** 这也是"WAF 为什么没拦住"的实用答案：WAF 看了一种视图，应用用了另一种。
- **解码到最内层，然后拒绝剩下的东西。** 在定义好的某一点解码一次。如果一个本该是纯文本的值在那之后仍然含 `%`、`&#`、`\u` 或者 NUL，那它就是畸形，应当拒绝而不是往下传。这一条同时关上二次解码和截断。
- **语言支持的话，用规范化检查（canonicalization check）。** 解码到字符、重新编码、和原始字节比较。如果两者不同，输入就是非规范的 —— 过长序列、未规范化的形式、或者另一种编码。**拒绝非规范输入是一条能覆盖上表好几行的控制**，而且不管问题来自 UTF-8、GBK 还是规范化，它都是同一个检验。
- **在校验之前、按文档化的形式、恰好规范化一次** —— 然后使用同一个值。bug 从来不是"规范化存在"，而是"规范化发生在一处、校验发生在另一处"。
- **停止转义，改用参数化。** 这一篇里每一种基于转义的防御，都是对**某种表示**做的过滤，而表示恰恰是攻击者控制的东西。参数化查询、结构化参数列表（用数组调 `execve` 而不是拼 shell 字符串）、预编译语句和带类型的 API 都不做过滤 —— 它们**分离通道**，于是数据里没有任何字节序列能变成结构。这是结构性的修法，它和古典密码那几篇里的模运算推理是同一个论证：**解决这一类，而不是这一个实例。**
- **领域允许的地方，用白名单而不是黑名单。** 一个本该是用户名或文件名的字段，合法字母表很小。**列出允许的东西，能一次消掉那张绕过表里的每一行**，而且当新的 Unicode 字符或新的编码被发明出来时，它不需要更新。
- **声明编码并强制执行它。** 在数据库连接上显式设置字符集、在 `Content-Type` 里带 charset、端到端用 UTF-8，并使用会拒绝过长与非法序列的严格解码器。大多数宽字节与过长 bug 之所以存在，是因为编码被留成了隐式的，或者解码器太宽松。
- **把重复参数的处理方式定下来，只定一次。** 选一种做法 —— 拒绝、取第一个、或取最后一个 —— 写下来，在每一层解析请求的地方都实现它，并测它。**参数污染的漏洞在于分歧**，所以缓解办法就是一个所有人都遵守的决定。
- **给读告警的防守方一句话：看编码，不要只看载荷。** 一个含 `%25`、含过长序列、含混合脚本值、或者带重复参数的请求，和含一个明显攻击串的请求，是两种不同的信号。**前一种是在试探你的各层怎么解释输入**，这比"他碰巧发了什么载荷"更有意义。
