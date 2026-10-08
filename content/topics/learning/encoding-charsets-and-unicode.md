---
id: encoding-charsets-and-unicode
title_en: "Encoding, Part 3 — Charsets and Unicode"
title_zh: "编码（三）：字符集与 Unicode"
summary_en: Most input-handling bugs are two layers disagreeing about what a sequence of bytes means, and those disagreements live in charsets. This entry separates character set from encoding, derives the UTF-8 byte structure, covers UTF-16 and the CJK double-byte encodings where the sharp edges are, then the three things that turn text into a security problem — normalization, confusables and invisible characters.
summary_zh: 大多数输入处理的 bug，都是两层对"一串字节是什么意思"的理解不一致，而这些不一致就住在字符集里。这一篇把字符集与编码分开、把 UTF-8 的字节结构推导出来、讲 UTF-16 与那些真正有锋利边缘的 CJK 双字节编码，最后讲三件把文本变成安全问题的事 —— 规范化、同形字、不可见字符。
tags: [beginner, encoding, unicode, utf8, gbk, normalization, homoglyph, trojan-source]
tools: [Python, unicodedata, iconv, CyberChef, xxd]
attck: [T1027, T1190]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Four words that get used as one

Almost every charset bug starts with vocabulary. These are four different things:

| Term | What it is | Example |
|---|---|---|
| **Character set** | A catalogue of characters | Unicode |
| **Code point** | A character's number in that catalogue | `U+4E2D` |
| **Encoding** | The rule mapping code points to bytes | UTF-8, GBK, Shift-JIS |
| **Glyph** | How a code point is drawn | The shape of 中 |

So **"Unicode" is not an encoding.** Unicode is the catalogue; UTF-8, UTF-16 and UTF-32 are three different ways to write its code points as bytes. Saying "the file is in Unicode" tells you nothing about how to read it, and that vagueness is how the bugs get in.

And a fifth thing that is none of the above: **a byte string is not text**. `b'\xe4\xb8\xad'` is three bytes. Which character it represents depends entirely on which encoding you decode it with — UTF-8 says 中, Latin-1 says `ä¸`, GBK says something else again. **The bytes are not ambiguous; the interpretation is.** Every one of these problems is a disagreement about interpretation.

### ASCII, and why there are so many encodings

ASCII defines 128 code points in 7 bits: control characters, digits, letters, punctuation. Every English-language protocol was built on it, and it is the reason UTF-8 works the way it does.

128 characters is not enough for anything else, so every language and region invented its own extension — and this is where the mess comes from:

| Encoding | Structure | Covers |
|---|---|---|
| **ASCII** | 1 byte, 7 bits used | English |
| **Latin-1** (ISO-8859-1) | 1 byte per character | Western European |
| **Windows-1252** | 1 byte, differs from Latin-1 in 0x80–0x9F | Western European, and the actual default in a lot of legacy software |
| **GB2312 / GBK / GB18030** | Mostly 2 bytes | Chinese |
| **Shift-JIS** | Mostly 2 bytes | Japanese |
| **Big5** | Mostly 2 bytes | Traditional Chinese |
| **EUC-KR** | Mostly 2 bytes | Korean |
| **UTF-8** | 1 to 4 bytes | Everything |

The important structural divide is **single-byte versus double-byte**. In Latin-1, every byte is one character, and byte length equals character count. In GBK or Shift-JIS, a lead byte says "the next byte is part of the same character" — and that is the property that produces the entire class of width-related vulnerabilities in the next entry.

### UTF-8, derived

UTF-8 is a **variable-length** encoding, one to four bytes per code point, and its design is worth understanding because every property it has is deliberate.

**The byte patterns:**

| Code point range | Bytes | Pattern |
|---|---|---|
| U+0000 – U+007F | 1 | `0xxxxxxx` |
| U+0080 – U+07FF | 2 | `110xxxxx 10xxxxxx` |
| U+0800 – U+FFFF | 3 | `1110xxxx 10xxxxxx 10xxxxxx` |
| U+10000 – U+10FFFF | 4 | `11110xxx 10xxxxxx 10xxxxxx 10xxxxxx` |

Read the patterns carefully and you see three design decisions:

1. **The number of leading 1s in the first byte tells you how long the sequence is.** `0` means one byte, `110` means two, `1110` means three, `11110` means four. A parser knows the length from the first byte alone.
2. **Continuation bytes always start with `10`.** So a byte starting with `10` can never be a first byte, and a byte starting with `0` or `11` can never be a continuation. This gives **self-synchronisation**: after a corrupted or truncated byte, a parser can find the next character boundary without guessing.
3. **ASCII is unchanged.** Any byte below 0x80 means the same thing in UTF-8 as in ASCII, so every ASCII file is a valid UTF-8 file, and every ASCII-era protocol keeps working.

**Worked example, bit by bit.** Encode 中, which is `U+4E2D`:

```
U+4E2D = 0100 1110 0010 1101      (16 bits, so a 3-byte sequence)

template:  1110xxxx 10xxxxxx 10xxxxxx
fill bits: 1110 0100 10 111000 10 101101

bytes:     11100100 10111000 10101101
hex:       E4       B8       AD
```

So 中 is `E4 B8 AD` in UTF-8 — three bytes, and you can verify each one carries the right prefix.

```python
def utf8_encode(cp: int) -> bytes:
    if cp < 0x80:
        return bytes([cp])
    if cp < 0x800:
        return bytes([0xC0 | (cp >> 6), 0x80 | (cp & 0x3F)])
    if cp < 0x10000:
        return bytes([0xE0 | (cp >> 12), 0x80 | ((cp >> 6) & 0x3F), 0x80 | (cp & 0x3F)])
    if cp <= 0x10FFFF:
        return bytes([0xF0 | (cp >> 18), 0x80 | ((cp >> 12) & 0x3F),
                      0x80 | ((cp >> 6) & 0x3F), 0x80 | (cp & 0x3F)])
    raise ValueError('code point out of range')

def utf8_decode(data: bytes) -> int:
    b = data[0]
    if b < 0x80:
        return b
    for n, mask in ((2, 0x1F), (3, 0x0F), (4, 0x07)):
        if b & (0xFF << (8 - n - 1) & 0xFF) == (0xFF << (8 - n) & 0xFF):
            cp = b & mask
            for i in range(1, n):
                cp = (cp << 6) | (data[i] & 0x3F)
            return cp
    raise ValueError('invalid lead byte')

print(utf8_encode(0x4E2D).hex())          # e4b8ad
print(hex(utf8_decode(bytes.fromhex('e4b8ad'))))   # 0x4e2d
print('中'.encode('utf-8').hex())          # e4b8ad, from the standard library
```

**And now the rules that make it security-relevant.** A valid UTF-8 decoder must reject four things, and implementations that do not are the source of real bypasses:

| Must be rejected | Why it matters |
|---|---|
| **Overlong encodings** — `C0 80` for U+0000, `E0 80 AF` for `/` | The same character has a second, longer spelling. A filter matching the canonical bytes misses it |
| **Surrogate code points** — U+D800–U+DFFF encoded in UTF-8 | Not valid UTF-8. Some decoders accept them and produce unpredictable results |
| **Code points above U+10FFFF** | Out of range by definition |
| **Truncated or invalid continuation bytes** | Indicates corruption or a deliberately malformed input |

That first row is the one that gets people. **`%C0%AF` decodes to `/`** in a lenient decoder, which means a path check that looks for `/` and `..` can be walked past by encoding them non-canonically. This is a live bug class, not a historical one, and it is why the next entry exists.

### UTF-16 and UTF-32

**UTF-16** uses two bytes as its unit. Code points in the Basic Multilingual Plane (U+0000–U+FFFF) are written directly; anything above that is written as a **surrogate pair** — two 16-bit values from the range U+D800–U+DFFF, which exists precisely so that non-BMP characters can be represented.

```python
s = 'A中\U0001F600'
print(s.encode('utf-16-le').hex())    # 4100 2d4e 3dd8 00de
print(len(s), len(s.encode('utf-16-le')))   # 3 characters, 8 bytes
```

**Byte order is a real question here**, unlike in UTF-8 where the encoding itself fixes it. Two conventions exist, and a **BOM** (byte order mark) at the start of a file declares which:

| Encoding | BOM | Note |
|---|---|---|
| UTF-8 | `EF BB BF` | Optional, and its presence breaks tools that do not expect it |
| UTF-16 little-endian | `FF FE` | The default on Windows |
| UTF-16 big-endian | `FE FF` | |
| UTF-32 LE / BE | `FF FE 00 00` / `00 00 FE FF` | |

This is the answer to a question the base family entry raised: **PowerShell's `-EncodedCommand` is base64 of a UTF-16LE string**, which is why decoding it as UTF-8 gives you text with a gap after every character — those gaps are the zero high bytes. And it is why a UTF-8 BOM at the start of a config file can break a script that parses it as ASCII.

**UTF-32** writes every code point in four bytes. Simple, fixed-width, easy to index, and three to four times larger than UTF-8 for typical text. It exists mainly for internal use.

The same character in all four:

| Encoding | 中 |
|---|---|
| UTF-8 | `E4 B8 AD` |
| UTF-16LE | `2D 4E` |
| UTF-16BE | `4E 2D` |
| UTF-32LE | `2D 4E 00 00` |

### The CJK double-byte encodings, where the sharp edges are

GBK, GB2312, Shift-JIS and Big5 are **variable-width**: ASCII bytes stand alone, and a **lead byte** in a defined range means the next byte belongs to the same character. The ranges are what matter:

| Encoding | Lead byte range | Trail byte range |
|---|---|---|
| **GBK** | `0x81`–`0xFE` | `0x40`–`0xFE` (excluding `0x7F`) |
| **GB2312** | `0xA1`–`0xF7` | `0xA1`–`0xFE` |
| **Shift-JIS** | `0x81`–`0x9F`, `0xE0`–`0xEF` | `0x40`–`0x7E`, `0x80`–`0xFC` |
| **Big5** | `0xA1`–`0xF9` | `0x40`–`0x7E`, `0xA1`–`0xFE` |

Look at GBK's trail range: **`0x5C` is inside it, and `0x5C` is the backslash.** And Shift-JIS includes `0x5C` in its trail range too. That single overlap is the entire basis of **wide-byte injection**, which the next entry covers in detail — here, just register the structural fact: in these encodings, a byte that a single-byte-aware layer treats as a special character can be the *second half of a different character*.

```python
# how a byte can be both a trail byte and a special character
gbk_char = bytes.fromhex('df5c')          # a valid GBK character whose trail byte is 0x5C
print(gbk_char.decode('gbk'))             # decodes to a single Chinese character
print('0x5C is backslash:', bytes.fromhex('5c') == b'\\')
```

**Telling GBK and UTF-8 apart**, which you will need constantly when handling Chinese text:

```python
def guess_charset(data: bytes):
    try:
        data.decode('utf-8')
        return 'utf-8 (or ascii)'
    except UnicodeDecodeError:
        pass
    try:
        data.decode('gbk')
        return 'gbk'
    except UnicodeDecodeError:
        return 'unknown'
```

That is a heuristic and not a truth: some byte sequences are valid in both, and the answer depends on context. It is also exactly the ambiguity attackers use — send bytes that the validator reads as GBK and the consumer reads as UTF-8, and the two disagree about what is special.

### Normalization, and why it is a security topic

Unicode allows **the same visual text to have more than one code point representation.** The letter é can be one code point (`U+00E9`) or two (`U+0065` + `U+0301`, e followed by a combining acute accent). A validator and a consumer that disagree about which form they are handling will disagree about whether two strings are equal.

**The four normalization forms:**

| Form | Name | Effect |
|---|---|---|
| **NFD** | Canonical decomposition | é → e + combining accent |
| **NFC** | Canonical composition | e + combining accent → é |
| **NFKD** | Compatibility decomposition | ﬁ → f + i, ＜ → <, and drops distinctions |
| **NFKC** | Compatibility composition | The composed version of the above |

```python
import unicodedata

s1 = '\u00e9'                  # é as one code point
s2 = 'e\u0301'                 # e + combining acute
print(s1 == s2)                                  # False — different code points
print(unicodedata.normalize('NFC', s1) == unicodedata.normalize('NFC', s2))   # True

print([hex(ord(c)) for c in s1])                 # ['0xe9']
print([hex(ord(c)) for c in s2])                 # ['0x65', '0x301']
print(s1.encode('utf-8').hex())                  # c3a9
print(s2.encode('utf-8').hex())                  # 65cc81  <- 3 bytes, different!

# the compatibility forms collapse things that look different but mean the same
for s in ['\ufb01', '\uff1c', '\u2168', '\uff21']:
    print(repr(s), '->', repr(unicodedata.normalize('NFKC', s)))
```

**Why this is exploitable.** If a check compares an input against a blocklist using one form, and the application later uses another form, the check can be bypassed. Two classic shapes:

- **Fullwidth characters.** `＜` (U+FF1C) is not `<` (U+003C), but **NFKC normalization turns it into `<`**. A filter that rejects `<` and a downstream component that normalizes with NFKC before use will disagree — the filter sees a harmless fullwidth character, the consumer sees an angle bracket.
- **Unicode case folding and length.** Some characters change length under normalization: `ﬁ` is one code point and normalizes to two (`fi`). A length check before normalization and a buffer after it can disagree.

**The rule that prevents all of it: normalize once, at the boundary, in a fixed form, before any validation — and then validate the normalized value.** Normalizing "somewhere in the middle" is what creates the gap. Which form to pick depends on the purpose (NFC is the common choice for text; NFKC when you deliberately want compatibility folding), but the important thing is that it happens exactly once and everything downstream agrees.

### Confusables, or why two different strings look identical

Different code points can render identically. The classic example:

```python
latin_a = 'a'          # U+0061
cyrillic_a = '\u0430'  # U+0430, Cyrillic small a
print(latin_a == cyrillic_a)      # False
print(latin_a, cyrillic_a)        # visually identical in most fonts
print([hex(ord(c)) for c in 'apple'], [hex(ord(c)) for c in '\u0430pple'])
```

The shape of the attack: register a domain, a username, or a filename that renders as something trusted but compares as something else. `аpple.com` with a Cyrillic `а` looks like `apple.com` and is a different name. The same trick bypasses a blocklist that compares strings, and impersonates a user in a chat or a code review.

**Detecting it** is a matter of looking at the code points rather than the shapes: is this string mixing scripts where it should not? Python's `unicodedata` gives you the tools:

```python
import unicodedata

def script_of(ch):
    """Very rough script bucket, enough to spot a mixed-script string."""
    name = unicodedata.name(ch, '')
    for s in ('LATIN', 'CYRILLIC', 'GREEK', 'ARABIC', 'HEBREW', 'CJK', 'HIRAGANA', 'KATAKANA', 'HANGUL'):
        if name.startswith(s):
            return s
    return 'OTHER'

def mixed_script(s):
    scripts = {script_of(c) for c in s if c.isalpha()}
    return scripts if len(scripts) > 1 else None

for s in ['apple', '\u0430pple', 'paypal', 'p\u0430ypal', '中文abc']:
    print(f'{s!r:20} -> {mixed_script(s)}')
```

A production-grade check needs a confusables table (Unicode publishes one), but the mixed-script heuristic catches the common case, and it is what you would implement first for a username or domain policy.

### Invisible characters, and the one that hides code

Some code points produce no visible output at all, and that makes them useful both for watermarking and for deception.

| Code point | Name | Common use |
|---|---|---|
| U+200B | Zero-width space | Invisible separator; watermarking; breaking up a keyword to defeat matching |
| U+200C | Zero-width non-joiner | Script shaping control; also used to break words |
| U+200D | Zero-width joiner | Emoji sequences; joining |
| U+FEFF | Zero-width no-break space / BOM | Byte-order mark, and an invisible character mid-text |
| U+00A0 | Non-breaking space | Looks like a space, is not one — breaks naive parsing |
| U+202E | Right-to-left override | **Reorders displayed text** |

**U+202E is the serious one**, and it is not theoretical: it is the core of the **Trojan Source** class of vulnerabilities (CVE-2021-42574). A right-to-left override character makes the *rendering* of the following text run backwards, so a line of code can be displayed with its logic reversed relative to what the compiler sees. A reviewer reads one thing; the compiler executes another.

```python
line = 'if (isAdmin) { /* \u202e } else { grantAccess(); /* }'
print(line)          # the display is not what you would parse
```

**The mitigation is procedural as much as technical**: reject bidi control characters in source code and in identifiers, add a lint rule that flags them, and treat their presence in a pull request as suspicious rather than as a formatting quirk. Some editors and code-review tools now render them visibly; knowing which ones do is part of the defence.

### Detection and mitigation

- **The generalisable detection signal is a mismatch between how a string is displayed and how it compares.** A username mixing scripts, a filename containing zero-width characters, a domain with a confusable character, source code containing a bidi override, input whose normalized form differs from its raw form. None of these is visible by looking at the rendered text, and all of them are visible in the code points — which is why logging and alerting should record the escaped form, not just the rendered one.
- **Standardise on UTF-8 end to end.** Declare it in the protocol, store it in the database, set the collation to a Unicode-aware one, and convert at exactly one boundary. Most charset vulnerabilities exist because two components in the same pipeline are using different encodings, and the ambiguity is the vulnerability.
- **Normalize once, at the boundary, in a documented form, before validating.** Then validate the normalized value and use it. Never validate the raw form and use the normalized one, and never normalize twice with different forms in the same path.
- **Validate strictly, and reject rather than repair.** A UTF-8 decoder should reject overlong sequences, surrogate code points and code points above U+10FFFF — not substitute a replacement character and continue. Rejecting bad input is a security control; silently repairing it is how a validator and a consumer end up disagreeing.
- **Count in the unit that matters.** A length limit in **bytes** and a length limit in **code points** are different limits, and one emoji is four bytes but one code point. Say which you mean, and check the same thing everywhere — a length check in one unit and a buffer sized in another is a classic overflow shape.
- **Restrict character sets where the domain allows it.** Usernames, hostnames, filenames and identifiers rarely need the whole of Unicode. An allowlist of letters, digits and a few separators removes confusables, invisible characters and normalization questions in one step — and it is far more reliable than trying to blocklist the bad ones.
- **Treat bidi and zero-width characters as content, not formatting.** Alert on them in source, in configuration and in user-controlled fields. Add them to code review checklists and to lint rules. The Trojan Source class was found by people asking why the bytes and the display disagreed, which is exactly the habit this entry is trying to build.

<!-- lang:zh -->
### 四个常被当成一个的词

几乎每一个字符集 bug 都从词汇开始。这是四件不同的事：

| 术语 | 它是什么 | 例子 |
|---|---|---|
| **字符集（character set）** | 一份字符的目录 | Unicode |
| **码点（code point）** | 某个字符在这份目录里的编号 | `U+4E2D` |
| **编码（encoding）** | 把码点映射成字节的规则 | UTF-8、GBK、Shift-JIS |
| **字形（glyph）** | 一个码点被画出来的样子 | 中 的形状 |

所以 **"Unicode"不是一种编码。** Unicode 是那份目录；UTF-8、UTF-16、UTF-32 是把它的码点写成字节的三种不同方式。说"这个文件是 Unicode 的"什么也没告诉你 —— 而正是这种含糊，让 bug 钻了进来。

还有第五件不属于以上任何一类的事：**字节串不是文本。** `b'\xe4\xb8\xad'` 是三个字节。它代表哪个字符，完全取决于你用哪种编码去解它 —— UTF-8 说是 中，Latin-1 说是 `ä¸`，GBK 又是另一个。**字节本身没有歧义；有歧义的是解释。** 这一篇里的每一个问题，都是关于解释的分歧。

### ASCII，以及为什么会有这么多编码

ASCII 用 7 位定义了 128 个码点：控制字符、数字、字母、标点。所有英语世界的协议都建立在它之上，而 UTF-8 之所以长成那样也和它有关。

128 个字符对别的任何东西都不够用，于是每个语言和地区都发明了自己的扩展 —— 混乱就是从这儿来的：

| 编码 | 结构 | 覆盖 |
|---|---|---|
| **ASCII** | 1 字节，用 7 位 | 英语 |
| **Latin-1**（ISO-8859-1） | 每字符 1 字节 | 西欧 |
| **Windows-1252** | 1 字节，在 0x80–0x9F 段与 Latin-1 不同 | 西欧，而且是很多遗留软件的实际默认 |
| **GB2312 / GBK / GB18030** | 大多 2 字节 | 中文 |
| **Shift-JIS** | 大多 2 字节 | 日文 |
| **Big5** | 大多 2 字节 | 繁体中文 |
| **EUC-KR** | 大多 2 字节 | 韩文 |
| **UTF-8** | 1 到 4 字节 | 一切 |

结构上最重要的分界是**单字节与双字节**。在 Latin-1 里每个字节就是一个人物，字节长度等于字符数。而在 GBK 或 Shift-JIS 里，一个**首字节**表示"下一个字节属于同一个字符" —— 而正是这个性质，产生了下一篇文章里整整一类与宽度有关的漏洞。

### UTF-8，推导出来

UTF-8 是**变长**编码，每个码点一到四个字节，而它的设计值得理解，因为它每一个性质都是刻意的。

**字节模式：**

| 码点范围 | 字节数 | 模式 |
|---|---|---|
| U+0000 – U+007F | 1 | `0xxxxxxx` |
| U+0080 – U+07FF | 2 | `110xxxxx 10xxxxxx` |
| U+0800 – U+FFFF | 3 | `1110xxxx 10xxxxxx 10xxxxxx` |
| U+10000 – U+10FFFF | 4 | `11110xxx 10xxxxxx 10xxxxxx 10xxxxxx` |

仔细看这些模式，你会看到三个设计决定：

1. **首字节开头连续 1 的个数，告诉你这个序列有多长。** `0` 是一字节、`110` 是两字节、`1110` 是三字节、`11110` 是四字节。解析器只看首字节就知道长度。
2. **续字节永远以 `10` 开头。** 所以以 `10` 开头的字节永远不可能是首字节，以 `0` 或 `11` 开头的字节永远不可能是续字节。这带来了**自同步**：一个字节被损坏或截断之后，解析器不用猜就能找到下一个字符边界。
3. **ASCII 原样保留。** 任何小于 0x80 的字节在 UTF-8 里和在 ASCII 里意思相同，所以每个 ASCII 文件都是合法的 UTF-8 文件，每个 ASCII 时代的协议都继续能用。

**逐位展示的例子。** 编码 中，它是 `U+4E2D`：

```
U+4E2D = 0100 1110 0010 1101      （16 位，所以是三字节序列）

模板：     1110xxxx 10xxxxxx 10xxxxxx
填比特：   1110 0100 10 111000 10 101101

字节：     11100100 10111000 10101101
十六进制：  E4       B8       AD
```

所以 中 在 UTF-8 里是 `E4 B8 AD` —— 三个字节，而且你可以逐个验证它们带着正确的前缀。

```python
def utf8_encode(cp: int) -> bytes:
    if cp < 0x80:
        return bytes([cp])
    if cp < 0x800:
        return bytes([0xC0 | (cp >> 6), 0x80 | (cp & 0x3F)])
    if cp < 0x10000:
        return bytes([0xE0 | (cp >> 12), 0x80 | ((cp >> 6) & 0x3F), 0x80 | (cp & 0x3F)])
    if cp <= 0x10FFFF:
        return bytes([0xF0 | (cp >> 18), 0x80 | ((cp >> 12) & 0x3F),
                      0x80 | ((cp >> 6) & 0x3F), 0x80 | (cp & 0x3F)])
    raise ValueError('码点超出范围')

print(utf8_encode(0x4E2D).hex())          # e4b8ad
print('中'.encode('utf-8').hex())          # e4b8ad，标准库的结果
```

**现在说那些让它和安全相关的规则。** 一个合法的 UTF-8 解码器必须拒绝四样东西，而不拒绝它们的实现，就是真实绕过的来源：

| 必须拒绝 | 为什么重要 |
|---|---|
| **过长编码（overlong）** —— `C0 80` 表示 U+0000、`E0 80 AF` 表示 `/` | 同一个字符有了第二种、更长的写法。匹配规范字节的过滤器看不到它 |
| **代理区码点** —— U+D800–U+DFFF 被编进 UTF-8 | 这不是合法 UTF-8。有些解码器接受它，然后产出不可预期的结果 |
| **超过 U+10FFFF 的码点** | 按定义就超范围 |
| **截断或非法的续字节** | 说明数据损坏，或者是被刻意构造的畸形输入 |

**第一行**才是让人中招的那个。**`%C0%AF` 在一个宽松的解码器里会解成 `/`** —— 也就是说，一个寻找 `/` 和 `..` 的路径检查，可以被非规范编码绕过。这是一类**仍然活着**的 bug，不是历史，也正是下一篇存在的理由。

### UTF-16 与 UTF-32

**UTF-16** 以两个字节为单位。基本多文种平面（U+0000–U+FFFF）内的码点直接写出；超过这个范围的字符用**代理对**表示 —— 也就是从 U+D800–U+DFFF 这个区间取两个 16 位值，这个区间的存在本身就是为了表示非 BMP 字符。

```python
s = 'A中\U0001F600'
print(s.encode('utf-16-le').hex())    # 4100 2d4e 3dd8 00de
print(len(s), len(s.encode('utf-16-le')))   # 3 个字符，8 个字节
```

**字节序在这里是个真问题**，不像 UTF-8 里编码本身把顺序定死了。存在两种约定，而文件开头的 **BOM**（字节序标记）用来声明是哪种：

| 编码 | BOM | 说明 |
|---|---|---|
| UTF-8 | `EF BB BF` | 可选，而它的存在会弄坏那些没预期到它的工具 |
| UTF-16 小端 | `FF FE` | Windows 上的默认 |
| UTF-16 大端 | `FE FF` | |
| UTF-32 小端 / 大端 | `FF FE 00 00` / `00 00 FE FF` | |

这也回答了 Base 家族那篇留下的一个问题：**PowerShell 的 `-EncodedCommand` 是 UTF-16LE 字符串的 base64**，所以按 UTF-8 解它，会得到每个字符之间都有空隙的文本 —— 那些空隙就是零值的高字节。同理，一个配置文件开头的 UTF-8 BOM 会弄坏把它按 ASCII 解析的脚本。

**UTF-32** 把每个码点写成四个字节。简单、定宽、方便索引，而对常见文本比 UTF-8 大三四倍。它主要用于内部处理。

同一个字符在四种编码下：

| 编码 | 中 |
|---|---|
| UTF-8 | `E4 B8 AD` |
| UTF-16LE | `2D 4E` |
| UTF-16BE | `4E 2D` |
| UTF-32LE | `2D 4E 00 00` |

### CJK 双字节编码 —— 锋利边缘所在

GBK、GB2312、Shift-JIS、Big5 都是**变宽**的：ASCII 字节单独成立，而一个**首字节**落在某个区间里，就表示下一个字节属于同一个字符。**区间本身**才是要紧的：

| 编码 | 首字节范围 | 尾字节范围 |
|---|---|---|
| **GBK** | `0x81`–`0xFE` | `0x40`–`0xFE`（不含 `0x7F`） |
| **GB2312** | `0xA1`–`0xF7` | `0xA1`–`0xFE` |
| **Shift-JIS** | `0x81`–`0x9F`、`0xE0`–`0xEF` | `0x40`–`0x7E`、`0x80`–`0xFC` |
| **Big5** | `0xA1`–`0xF9` | `0x40`–`0x7E`、`0xA1`–`0xFE` |

看 GBK 的尾字节范围：**`0x5C` 就在里面，而 `0x5C` 是反斜杠。** Shift-JIS 的尾字节范围也包含 `0x5C`。就是这一个重叠，构成了**宽字节注入**的全部基础 —— 下一篇会详细讲，这里先把那个结构性事实记下来：**在这些编码里，一个被"按单字节处理"的层视为特殊字符的字节，可以是另一个字符的后半个。**

```python
# 一个字节既可以是尾字节、又可以是特殊字符
gbk_char = bytes.fromhex('df5c')          # 一个合法的 GBK 字符，尾字节是 0x5C
print(gbk_char.decode('gbk'))             # 解出一个汉字
print('0x5C 是反斜杠:', bytes.fromhex('5c') == b'\\')
```

**区分 GBK 与 UTF-8**，这是你处理中文文本时会不断需要的事：

```python
def guess_charset(data: bytes):
    try:
        data.decode('utf-8')
        return 'utf-8 (或 ascii)'
    except UnicodeDecodeError:
        pass
    try:
        data.decode('gbk')
        return 'gbk'
    except UnicodeDecodeError:
        return '未知'
```

这是一个启发式，不是真理：有些字节序列在两者下都合法，而答案取决于上下文。**而这恰恰就是攻击者利用的歧义** —— 发送一段字节，让校验器按 GBK 读、让消费方按 UTF-8 读，于是两者对"什么算特殊字符"产生分歧。

### 规范化，以及它为什么是安全话题

Unicode 允许**同一段视觉文本有多种码点表示**。字母 é 可以是一个码点（`U+00E9`），也可以是两个（`U+0065` + `U+0301`，即 e 后面跟一个组合尖音符）。一个校验器和消费方如果在"自己处理的是哪种形式"上不一致，它们就会对"两个字符串是否相等"产生分歧。

**四种规范化形式：**

| 形式 | 名称 | 效果 |
|---|---|---|
| **NFD** | 规范分解 | é → e + 组合尖音符 |
| **NFC** | 规范合成 | e + 组合尖音符 → é |
| **NFKD** | 兼容分解 | ﬁ → f + i、＜ → <，并丢掉一些区别 |
| **NFKC** | 兼容合成 | 上面那种的合成版本 |

```python
import unicodedata

s1 = '\u00e9'                  # é 作为一个码点
s2 = 'e\u0301'                 # e + 组合尖音符
print(s1 == s2)                                  # False —— 码点不同
print(unicodedata.normalize('NFC', s1) == unicodedata.normalize('NFC', s2))   # True

print([hex(ord(c)) for c in s1])                 # ['0xe9']
print([hex(ord(c)) for c in s2])                 # ['0x65', '0x301']
print(s1.encode('utf-8').hex())                  # c3a9
print(s2.encode('utf-8').hex())                  # 65cc81  <- 3 个字节，不一样！

# 兼容形式会把"看起来不同但含义相同"的东西折叠掉
for s in ['\ufb01', '\uff1c', '\u2168', '\uff21']:
    print(repr(s), '->', repr(unicodedata.normalize('NFKC', s)))
```

**为什么这可以被利用。** 如果一个检查用某种形式比较输入与黑名单，而应用之后用的是另一种形式，检查就能被绕过。两种经典形状：

- **全角字符。** `＜`（U+FF1C）不是 `<`（U+003C），但 **NFKC 规范化会把它变成 `<`**。一个拒绝 `<` 的过滤器，和一个在使用前先做 NFKC 规范化的下游组件，会产生分歧 —— 过滤器看到的是一个无害的全角字符，消费方看到的是一个尖括号。
- **大小写折叠与长度。** 有些字符在规范化下会改变长度：`ﬁ` 是一个码点，规范化后变成两个（`fi`）。规范化之前做的长度检查和规范化之后用的缓冲区，可能对不上。

**能防住这一切的规则是：在边界处、用固定的形式、只规范化一次，而且发生在任何校验之前 —— 然后校验那个规范化后的值。** 在"中间某处"做规范化，才是产生缺口的原因。选哪种形式取决于用途（文本常用 NFC；当你确实想要兼容折叠时用 NFKC），但要点是它只发生**一次**，而且下游一切对此有共识。

### 同形字 —— 为什么两个不同的字符串看起来一模一样

不同的码点可以渲染成同样的样子。经典的例子：

```python
latin_a = 'a'          # U+0061
cyrillic_a = '\u0430'  # U+0430，西里尔小写 a
print(latin_a == cyrillic_a)      # False
print(latin_a, cyrillic_a)        # 在多数字体里看起来一样
print([hex(ord(c)) for c in 'apple'], [hex(ord(c)) for c in '\u0430pple'])
```

攻击的形状：注册一个渲染成可信样子的域名、用户名或文件名，但它在比较时是另一个东西。用西里尔 `а` 的 `аpple.com` 看起来像 `apple.com`，却是另一个名字。同样的手法可以绕过做字符串比较的黑名单，也可以在聊天或代码评审里冒充某个用户。

**检测它**靠的是看码点而不是看形状：这个字符串是不是在它不该混用脚本的地方混用了脚本？Python 的 `unicodedata` 给了你工具：

```python
import unicodedata

def script_of(ch):
    """很粗糙的脚本归类，够用来发现混合脚本的字符串。"""
    name = unicodedata.name(ch, '')
    for s in ('LATIN', 'CYRILLIC', 'GREEK', 'ARABIC', 'HEBREW', 'CJK', 'HIRAGANA', 'KATAKANA', 'HANGUL'):
        if name.startswith(s):
            return s
    return 'OTHER'

def mixed_script(s):
    scripts = {script_of(c) for c in s if c.isalpha()}
    return scripts if len(scripts) > 1 else None

for s in ['apple', '\u0430pple', 'paypal', 'p\u0430ypal', '中文abc']:
    print(f'{s!r:20} -> {mixed_script(s)}')
```

生产级的检查需要一张同形字表（Unicode 官方发布了一张），但混合脚本这个启发式能抓到常见情况，而且是你为用户名或域名策略实现的第一个检查。

### 不可见字符，以及那个能把代码藏起来的

有些码点完全不产生可见输出，这让它们既能用于水印，也能用于欺骗。

| 码点 | 名称 | 常见用途 |
|---|---|---|
| U+200B | 零宽空格 | 不可见分隔符；水印；拆开关键词以躲过匹配 |
| U+200C | 零宽不连字 | 文字塑形控制；也被用来拆词 |
| U+200D | 零宽连字 | emoji 序列；连接 |
| U+FEFF | 零宽不换行空格 / BOM | 字节序标记，也可以出现在文本中间 |
| U+00A0 | 不换行空格 | 看起来像空格，但不是 —— 能弄坏天真的解析 |
| U+202E | 从右到左覆盖 | **重排显示出来的文本** |

**U+202E 是严重的那个**，而且它不是理论：它是 **Trojan Source** 这一类漏洞（CVE-2021-42574）的核心。一个从右到左覆盖字符会让它**后面**的文本在**显示**时反向排列，于是一行代码显示出来的逻辑可以和编译器看到的相反。评审者读到一件事，编译器执行另一件事。

```python
line = 'if (isAdmin) { /* \u202e } else { grantAccess(); /* }'
print(line)          # 显示出来的不是你解析出来的
```

**缓解一半是流程上的，一半是技术上的**：在源代码和标识符里拒绝 bidi 控制字符，加一条 lint 规则把它们标出来，并且把 pull request 里出现它们当作可疑而不是格式怪癖。有些编辑器和代码评审工具现在会把它们显式渲染出来；知道哪些会，本身就是防御的一部分。

### 检测与缓解

- **可推广的检测信号是"显示方式与比较方式不一致"。** 一个混用脚本的用户名、一个含零宽字符的文件名、一个带同形字的域名、包含 bidi 覆盖的源代码、规范化形式与原始形式不同的输入。这些**没有一样**能靠看渲染后的文本发现，而**全都能**在码点里看到 —— 这就是为什么日志与告警应当记录转义后的形式，而不只是渲染后的形式。
- **端到端统一用 UTF-8。** 在协议里声明它、在数据库里存它、把排序规则设成 Unicode 感知的那种、并且只在**一个**边界上做转换。多数字符集漏洞之所以存在，是因为同一条流水线里的两个组件用了不同的编码 —— **那个歧义就是漏洞**。
- **在边界处、按文档化的形式、在校验之前只规范化一次。** 然后校验规范化后的值并使用它。**绝不**校验原始形式却使用规范化后的形式，也**绝不**在同一条路径上先后用两种形式规范化两次。
- **严格校验，宁可拒绝也不要修复。** UTF-8 解码器应当拒绝过长序列、代理区码点和超过 U+10FFFF 的码点 —— 而不是替换成一个替代字符继续跑。**拒绝坏输入是一项安全控制**；静默修复它，正是校验器与消费方产生分歧的方式。
- **在真正要紧的那个单位上计数。** 以**字节**为单位的长度限制和以**码点**为单位的长度限制是不同的限制，而一个 emoji 是四个字节、一个码点。说清你指哪一个，并在所有地方检查同一个东西 —— 一个用某种单位检查长度、另一个用另一种单位分配缓冲区，是经典的溢出形状。
- **在领域允许的地方限制字符集。** 用户名、主机名、文件名和标识符很少需要整个 Unicode。一份"字母、数字加几个分隔符"的白名单，能一步消掉同形字、不可见字符和规范化问题 —— 而且它比试图把坏字符列黑名单可靠得多。
- **把 bidi 和零宽字符当内容，而不是格式。** 在源码、配置和用户可控字段里对它们告警。把它们加进代码评审清单和 lint 规则。**Trojan Source 这一类就是被人问"为什么字节和显示不一致"时发现的** —— 而这正是这一篇想养成的习惯。
