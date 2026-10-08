---
id: encoding-base-family
title_en: Encoding, Part 1 — The Base Family
title_zh: 编码（一）：Base 家族
summary_en: Base64 is not encryption, and it is everywhere — in tokens, in payloads, in exfiltration and in obfuscation. This entry derives it from the bit arithmetic instead of presenting it as a black box, covers the family from Base16 to Base85 with the recognition rule for each, and shows where it turns up in real attacks and real detection.
summary_zh: Base64 不是加密，而它无处不在 —— 在令牌里、在载荷里、在外泄里、在混淆里。这一篇从位运算把它推出来，而不是当成黑盒；把从 Base16 到 Base85 的整个家族连同各自的识别判据讲一遍；然后看它在真实攻击与真实检测里的位置。
tags: [beginner, encoding, base64, base32, base58, data-exfiltration]
tools: [Python, CyberChef, base64, xxd]
attck: [T1132, T1027, T1140]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Encoding is not encryption, and that is the point

Every entry in the cryptography series had a key. This one has none: encoding is a **reversible transformation with no secret**. Anyone who recognises the format can decode it, and the only thing it protects is the *transport* — making binary data survive a channel that only accepts text.

The beginner entry in this guide states the distinction in one line: Base64 is not encryption. This entry is about what it actually is, because in offensive and defensive work it matters more than it looks:

- **Tokens and payloads are encoded.** A JSON Web Token is three base64url segments. PowerShell's `-EncodedCommand` is base64 of UTF-16LE. `certutil -decode` writes a binary from a text file.
- **Encoding is used to hide things from simple inspection.** Malware strings, staged payloads and injected commands are frequently base64, because a scanner looking for a literal command name will not find it.
- **Data is encoded to leave a network.** Exfiltration over DNS, HTTP parameters or user agents frequently base64-encodes the payload so arbitrary bytes survive the channel.
- **And it is used legitimately constantly.** Every inline image in a stylesheet, every certificate in a PEM file, every cookie with a complex value.

So the skill is two-sided: recognise it and decode it fast when you are analysing, and know that it is not protection when you are designing.

### Base64 from the bit level up

Forget the base64 command for a moment and derive the whole thing.

**The problem.** You have arbitrary bytes — say a PNG — and a channel that only carries printable ASCII. Sending raw bytes breaks: control characters, characters the protocol treats specially, and 8-bit values in a 7-bit channel.

**The constraint.** Choose 64 characters that are safe in almost any context: `A`–`Z`, `a`–`z`, `0`–`9`, `+`, `/`. That is exactly 64 symbols, which is exactly **6 bits** of information each (`2^6 = 64`).

**The arithmetic.** A byte is 8 bits. You cannot chop 8 into 6-bit pieces evenly, so you take **three bytes at a time** — 24 bits — and split them into **four 6-bit groups**:

```
three bytes                24 bits
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  byte 1   |  byte 2   |  byte 3   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 \______/ \______/ \______/ \______/
   6 bits   6 bits   6 bits   6 bits
      |        |        |        |
    index    index    index    index
      |        |        |        |
      T        W        F        u
```

**Worked example, every bit shown.** Encode the ASCII text `Man`:

```
'M' = 0x4D = 01001101
'a' = 0x61 = 01100001
'n' = 0x6E = 01101110

concatenate:  01001101 01100001 01101110
regroup by 6: 010011 010110 000101 101110

010011 = 19  -> alphabet[19] = 'T'
010110 = 22  -> alphabet[22] = 'W'
000101 =  5  -> alphabet[5]  = 'F'
101110 = 46  -> alphabet[46] = 'u'

result: "TWFu"
```

**Now the padding, which is where implementations go wrong.** If the input length is not a multiple of three, the last group is short and padding makes it up:

| Input bytes in the last group | Bits available | Output | Padding |
|---|---|---|---|
| 3 | 24 | 4 characters | none |
| 2 | 16 | 3 characters | `=` |
| 1 | 8 | 2 characters | `==` |

Two examples, again bit by bit:

```
"Ma"  = 01001101 01100001
6-bit groups: 010011 010110 0001(00)     <- the last group is padded with zeros
indices:      19     22     4
result:       T      W      E  + "="     -> "TWE="

"M"   = 01001101
6-bit groups: 010011 01(0000)
indices:      19     16
result:       T      Q  + "=="           -> "TQ=="
```

So a base64 string's length is always a multiple of four, and it carries at most two `=` at the end. That is the recognition rule, and the arithmetic behind it.

**The overhead** is worth knowing: three bytes become four characters, so base64 is **33 percent larger** than the data it carries. That is the price of a text-safe channel, and it is why nobody uses base64 for bulk storage.

```python
ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'

def b64_encode(data: bytes) -> str:
    out = []
    for i in range(0, len(data), 3):
        chunk = data[i:i+3]
        # build a 24-bit integer, left-aligned, remembering how many bytes we had
        n = int.from_bytes(chunk.ljust(3, b'\x00'), 'big')
        chars = [
            ALPHABET[(n >> 18) & 63],
            ALPHABET[(n >> 12) & 63],
            ALPHABET[(n >> 6) & 63],
            ALPHABET[n & 63],
        ]
        out.extend(chars[:len(chunk) + 1])      # 3 bytes -> 4 chars, 2 -> 3, 1 -> 2
        out.extend('=' * (3 - len(chunk)))
    return ''.join(out)

def b64_decode(text: str) -> bytes:
    text = text.rstrip('=')                      # padding carries no data
    bits = ''.join(format(ALPHABET.index(c), '06b') for c in text)
    return bytes(int(bits[i:i+8], 2) for i in range(0, len(bits) - 7, 8))

print(b64_encode(b'Man'))            # TWFu
print(b64_encode(b'Ma'))             # TWE=
print(b64_encode(b'M'))              # TQ==
print(b64_decode('TWFu'))            # b'Man'
```

Those two functions are the whole format. Everything else in this entry is a variation on which characters are used and how many bits go in a group.

### The family

Once you see base64 as "regroup bits and map to an alphabet", the rest of the family is arithmetic:

| Encoding | Bits per group | Alphabet size | Characters from | Overhead | Padding |
|---|---|---|---|---|---|
| **Base16** (hex) | 4 | 16 | `0-9A-F` | +100% | none |
| **Base32** | 5 | 32 | `A-Z2-7` | +60% | `=` to a multiple of 8 |
| **Base58** | — (see below) | 58 | Base62 minus `0OIl` | ~+37% | none |
| **Base64** | 6 | 64 | `A-Za-z0-9+/` | +33% | `=` to a multiple of 4 |
| **Base85** | — (see below) | 85 | varies (`!`–`u`) | +25% | varies |
| **Base91** | — | 91 | most printable | +23% | none |

**Base32** groups 5 bits, so it takes five bytes (40 bits) to make eight characters, and its alphabet avoids the letters that are easy to confuse when read aloud or by eye: it uses `A`–`Z` and `2`–`7` only, skipping `0`, `1`, `8`, `9`. That is why it shows up in places where a human might retype a value — TOTP secrets, for instance.

**Base16** is just hexadecimal with a formal name. Four bits per character, no padding needed because a byte is exactly two characters. It doubles the size, which is why it is used for short values like hashes and never for payloads.

**Base58** is the interesting exception, and it breaks the bit-grouping model: **58 is not a power of two**, so it cannot be a fixed-width bit split. Instead it treats the entire byte string as one big integer and repeatedly divides by 58, using the remainders as digits. The alphabet is base62 with the visually ambiguous characters removed (`0`, `O`, `I`, `l`) plus `+` and `/` — which is exactly what you want for something a person might read off a screen. It is why Bitcoin addresses look the way they do.

```python
B58 = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'

def b58_encode(data: bytes) -> str:
    n = int.from_bytes(data, 'big')
    out = ''
    while n:
        n, r = divmod(n, 58)
        out = B58[r] + out
    # leading zero bytes are encoded as leading '1's
    pad = len(data) - len(data.lstrip(b'\x00'))
    return '1' * pad + out

def b58_decode(text: str) -> bytes:
    n = 0
    for c in text:
        n = n * 58 + B58.index(c)
    body = n.to_bytes((n.bit_length() + 7) // 8, 'big')
    pad = len(text) - len(text.lstrip('1'))
    return b'\x00' * pad + body
```

**Base85** goes the other way: instead of shrinking the alphabet, it enlarges it so fewer characters are needed. Four bytes (32 bits) are treated as one integer and written in base 85, producing five characters — hence the 25 percent overhead, the best of the practical encodings. The cost is a character set full of punctuation, which needs escaping in HTML, XML and shell contexts. Adobe's Ascii85, Git's binary patch format and Z85 are the variants you will meet.

### Recognising which one you are looking at

This is the practical part. Given an unknown blob:

| What you see | What it probably is |
|---|---|
| Only `0-9A-Fa-f`, even length | Base16 |
| Only `A-Z` and `2-7`, often ending in several `=` | Base32 |
| Mixed case plus `+` and `/`, length a multiple of 4, ends in `=` or `==` | Base64 |
| Mixed case plus `-` and `_`, no padding | Base64url |
| Mixed case and digits, **no** `0`, `O`, `I`, `l`, `+`, `/`, no padding | Base58 |
| Mixed case, digits and lots of punctuation | Base85 (or Base91) |

Two refinements worth having:

- **Length gives the base away.** Base32 output lengths are multiples of 8, base64 of 4, base16 of 2. When a string's length is a multiple of 8 and its alphabet is uppercase-plus-2-to-7, you do not need to guess.
- **Entropy is not the signal.** Base64 *looks* random, but so do hashes, UUIDs, session identifiers and encrypted blobs. What distinguishes base64 is the **alphabet and length constraint**, not randomness.

```python
import re

PATTERNS = [
    ('base64',      re.compile(r'^[A-Za-z0-9+/]+={0,2}$')),
    ('base64url',   re.compile(r'^[A-Za-z0-9_-]+$')),
    ('base32',      re.compile(r'^[A-Z2-7]+=*$')),
    ('base16',      re.compile(r'^[0-9A-Fa-f]+$')),
    ('base58',      re.compile(r'^[1-9A-HJ-NP-Za-km-z]+$')),
]

def guess_encoding(s):
    hits = []
    for name, pat in PATTERNS:
        if pat.match(s):
            if name == 'base64' and len(s) % 4:
                continue                      # length must work out
            if name == 'base16' and len(s) % 2:
                continue
            hits.append(name)
    return hits

for s in ['TWFu', 'TWE=', 'MZXW6===', '4d616e', '2NEpo7TZRRrLZSi2U', 'SGVsbG8sIHdvcmxkIQ==']:
    print(s, '->', guess_encoding(s))
```

### Nested encodings, which is most of CTF

Layers of encoding are extremely common: base64 inside base64, hex inside base64, base64 of a gzipped blob. The tell is a decoding result that is *itself* recognisable.

```python
import base64, binascii, gzip, zlib

def peel(s, max_layers=10):
    """Decode repeatedly until nothing recognisable is left."""
    for layer in range(max_layers):
        original = s
        try:
            if re.fullmatch(r'[A-Za-z0-9+/]+={0,2}', s) and len(s) % 4 == 0:
                s = base64.b64decode(s).decode('utf-8', 'replace')
            elif re.fullmatch(r'[0-9A-Fa-f]+', s) and len(s) % 2 == 0:
                s = binascii.unhexlify(s).decode('utf-8', 'replace')
            elif re.fullmatch(r'[A-Z2-7]+=*', s):
                s = base64.b32decode(s).decode('utf-8', 'replace')
        except Exception:
            break
        if s == original:
            break
        print(f'layer {layer + 1}: {s[:120]}')
    return s

peel('VTBkV2MySkhPSE5KU0ZKdllWaE5aMkZZVFdkWlUwSjZaRWRHZVdSQlBUMD0=')
```

That last example is base64 of base64 of base64 of `Hello, this is a start`. Notice how the first decode gives `U0dWc2...`-looking text, which is *itself* valid base64 — that is the signal to keep going.

One practical warning: **do not blindly loop.** Base64 decoding can produce bytes that happen to be valid base64 by accident, and a loop will run away producing nonsense. Stop when the alphabet stops matching, or when the output is no longer printable, or after a fixed number of layers.

### Where this actually shows up in attacks

| Context | What is base64 | Why it matters |
|---|---|---|
| **JWT** | Three base64url segments: header, payload, signature | The first two are readable by anyone — never put secrets there |
| **PowerShell `-EncodedCommand`** | Base64 of the command in **UTF-16LE** | The UTF-16 part is why a decoder sometimes gives you spaced-out text; the command line itself is a detection signal |
| **`certutil -decode` / `-urlcache`** | Base64 or URL in a text file | A signed Windows binary used to write or fetch a payload |
| **Exfiltration over DNS/HTTP** | Payload base64-encoded into subdomains or parameters | Arbitrary bytes survive a text channel; long high-entropy labels are the detection signal |
| **Obfuscated scripts** | Strings and whole blocks base64-encoded | Defeats literal string matching |
| **Inline web assets** | `data:image/png;base64,...` | Legitimate, and a place a payload can hide |

**The PowerShell detail is worth remembering** because it is a common stumbling block: `-EncodedCommand` takes base64 of a **UTF-16LE** string, not UTF-8. So:

```python
import base64
cmd = 'Get-Process'
encoded = base64.b64encode(cmd.encode('utf-16-le')).decode()
print(encoded)      # RwBlAHQALQBQAHIAbwBjAGUAcwBzAA==   <- note the 'A's: that is UTF-16LE
print(base64.b64decode(encoded).decode('utf-16-le'))
```

If you decode a suspicious `-EncodedCommand` value as UTF-8 and get text with gaps between every character, that is UTF-16LE and you decoded it with the wrong codec.

### Detection and mitigation

- **The signal is not "base64 exists", it is "base64 where it does not belong".** Base64 is everywhere in normal traffic: image data, tokens, certificates, cookies. What deserves a rule is a long high-entropy base64 string in a place that has no legitimate reason to carry one — a DNS query label, a URL parameter that normally holds a short identifier, a `User-Agent` header, or a filename.
- **Watch the command line.** `-EncodedCommand`, `-enc`, `FromBase64String`, `certutil -decode`, `-urlcache`, `base64 -d` piped into a shell. These are specific strings that appear in real intrusions, and they are cheap to alert on when command-line auditing is on.
- **Decode during triage, always.** When a report contains a base64 blob, decode it before deciding anything about it. Half of "suspicious unknown string" investigations end at the first decode, and the other half get much more specific. This is also the fastest way to read a JWT payload.
- **Length and entropy together narrow it down.** Base32 lengths are multiples of 8, base64 of 4. An extremely long single-token string in a log or a URL is worth a second look regardless of the alphabet.
- **For defenders, the mitigation is application-side.** Never store or transmit anything sensitive in base64 and call it protected: it is readable by design, and it appears in logs, error messages and crash dumps where developers do not expect it. If the data needs confidentiality, encrypt it and manage the key; if it needs integrity, sign it.
- **And remember that base64 defeats matching, not analysis.** Any control that works by matching literal strings — a WAF rule, a blocklist, a signature — can be bypassed by encoding the payload. The countermeasure is to decode and normalise **before** matching, at every layer that inspects, and to know which layer decodes what. A WAF that inspects base64 blindly, without decoding it, is inspecting the wrong thing.

<!-- lang:zh -->
### 编码不是加密，而这正是要点

密码学系列的每一篇都有一把钥匙。这一篇没有：编码是一种**没有秘密的可逆变换**。任何认得出这个格式的人都能解出来，而它唯一保护的东西是**传输过程** —— 让二进制数据能活着通过一条只接受文本的通道。

本指南的入门篇用一句话说过这个区别：Base64 不是加密。这一篇讲它到底是什么，因为它在攻防工作里比看上去要紧得多：

- **令牌与载荷是编码的。** 一个 JSON Web Token 是三段 base64url。PowerShell 的 `-EncodedCommand` 是 UTF-16LE 的 base64。`certutil -decode` 从一个文本文件写出一份二进制。
- **编码被用来躲过简单检查。** 恶意软件的字符串、分阶段的载荷、注入的命令，经常是 base64，因为一个在找字面命令名的扫描器找不到它。
- **数据被编码着离开网络。** 经由 DNS、HTTP 参数或 User-Agent 的外泄，经常把载荷 base64 一下，好让任意字节能活着通过那条通道。
- **而它也在被大量正当地使用。** 样式表里的每一张内联图片、PEM 文件里的每一张证书、每一个值比较复杂的 cookie。

所以这项技能是双向的：分析时能认出它并快速解出来；设计时知道它不构成保护。

### 从比特层面推出 Base64

先把 base64 命令忘掉，把这个东西整个推导出来。

**问题。** 你手上有任意字节 —— 比如一张 PNG —— 而通道只能传可打印 ASCII。直接发原始字节会坏掉：控制字符、协议会特殊对待的字符，以及 8 位值塞进 7 位通道。

**约束。** 选 64 个几乎在任何上下文里都安全的字符：`A`–`Z`、`a`–`z`、`0`–`9`、`+`、`/`。正好 64 个符号，而每个符号正好携带 **6 比特**信息（`2^6 = 64`）。

**算术。** 一个字节是 8 比特，8 没法整除成 6 比特的片，所以**每次取三个字节** —— 24 比特 —— 再切成**四组 6 比特**：

```
三个字节                    24 比特
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  字节 1   |  字节 2   |  字节 3   |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 \______/ \______/ \______/ \______/
   6 比特   6 比特   6 比特   6 比特
      |        |        |        |
     索引     索引     索引     索引
      |        |        |        |
      T        W        F        u
```

**逐位展示的例子。** 编码 ASCII 文本 `Man`：

```
'M' = 0x4D = 01001101
'a' = 0x61 = 01100001
'n' = 0x6E = 01101110

拼接：        01001101 01100001 01101110
按 6 位重分：  010011 010110 000101 101110

010011 = 19  -> 字母表[19] = 'T'
010110 = 22  -> 字母表[22] = 'W'
000101 =  5  -> 字母表[5]  = 'F'
101110 = 46  -> 字母表[46] = 'u'

结果："TWFu"
```

**然后是填充，而实现就是在这里出错的。** 如果输入长度不是三的倍数，最后一组就是短的，用填充补齐：

| 最后一组的输入字节 | 可用比特 | 输出 | 填充 |
|---|---|---|---|
| 3 | 24 | 4 个字符 | 无 |
| 2 | 16 | 3 个字符 | `=` |
| 1 | 8 | 2 个字符 | `==` |

再看两个逐位的例子：

```
"Ma"  = 01001101 01100001
6 位分组：010011 010110 0001(00)     <- 最后一组用零补齐
索引：     19     22     4
结果：     T      W      E  + "="     -> "TWE="

"M"   = 01001101
6 位分组：010011 01(0000)
索引：     19     16
结果：     T      Q  + "=="           -> "TQ=="
```

所以 base64 字符串的长度永远是 4 的倍数，末尾最多带两个 `=`。这就是识别判据，以及它背后的算术。

**开销**值得知道：三个字节变成四个字符，所以 base64 比它携带的数据**大 33%**。这是"文本安全通道"的代价，也是没人用它做批量存储的原因。

```python
ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'

def b64_encode(data: bytes) -> str:
    out = []
    for i in range(0, len(data), 3):
        chunk = data[i:i+3]
        # 拼成 24 位整数（左对齐），同时记住原本有几个字节
        n = int.from_bytes(chunk.ljust(3, b'\x00'), 'big')
        chars = [
            ALPHABET[(n >> 18) & 63],
            ALPHABET[(n >> 12) & 63],
            ALPHABET[(n >> 6) & 63],
            ALPHABET[n & 63],
        ]
        out.extend(chars[:len(chunk) + 1])      # 3 字节 -> 4 字符，2 -> 3，1 -> 2
        out.extend('=' * (3 - len(chunk)))
    return ''.join(out)

def b64_decode(text: str) -> bytes:
    text = text.rstrip('=')                      # 填充不携带数据
    bits = ''.join(format(ALPHABET.index(c), '06b') for c in text)
    return bytes(int(bits[i:i+8], 2) for i in range(0, len(bits) - 7, 8))

print(b64_encode(b'Man'))            # TWFu
print(b64_encode(b'Ma'))             # TWE=
print(b64_encode(b'M'))              # TQ==
print(b64_decode('TWFu'))            # b'Man'
```

这两个函数就是整个格式。这一篇剩下的内容，都是"用哪些字符"和"一组几比特"的变化。

### 整个家族

一旦你把 base64 看成"重新分组比特、再映射到一张字母表"，家族里其余的成员就都是算术了：

| 编码 | 每组比特 | 字母表大小 | 字符来自 | 开销 | 填充 |
|---|---|---|---|---|---|
| **Base16**（十六进制） | 4 | 16 | `0-9A-F` | +100% | 无 |
| **Base32** | 5 | 32 | `A-Z2-7` | +60% | 补到 8 的倍数 |
| **Base58** | ——（见下） | 58 | Base62 去掉 `0OIl` | 约 +37% | 无 |
| **Base64** | 6 | 64 | `A-Za-z0-9+/` | +33% | 补到 4 的倍数 |
| **Base85** | ——（见下） | 85 | 各不相同（`!`–`u`） | +25% | 各不相同 |
| **Base91** | —— | 91 | 大部分可打印字符 | +23% | 无 |

**Base32** 每组 5 比特，所以要用五个字节（40 比特）生成八个字符；它的字母表避开了那些念出来或看过去容易混的字母：只用 `A`–`Z` 和 `2`–`7`，跳过 `0`、`1`、`8`、`9`。这就是它出现在"人可能要照着敲一遍"的地方的原因 —— 比如 TOTP 的密钥。

**Base16** 就是有了正式名字的十六进制。每字符 4 比特，因为一个字节正好是两个字符所以不需要填充。它把体积翻倍，所以只用于哈希这种短值，从不用于载荷。

**Base58** 是有意思的例外，而它**打破了位分组模型**：**58 不是 2 的幂**，所以不可能是固定宽度的比特切分。它把整个字节串当成一个大整数，反复除以 58，把余数当作数字。字母表是 base62 去掉视觉上易混的字符（`0`、`O`、`I`、`l`）再加上去掉 `+` 和 `/` —— 而这正是一个人要从屏幕上念出来时想要的东西。这就是比特币地址长成那样的原因。

```python
B58 = '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'

def b58_encode(data: bytes) -> str:
    n = int.from_bytes(data, 'big')
    out = ''
    while n:
        n, r = divmod(n, 58)
        out = B58[r] + out
    # 前导零字节被编码成前导的 '1'
    pad = len(data) - len(data.lstrip(b'\x00'))
    return '1' * pad + out

def b58_decode(text: str) -> bytes:
    n = 0
    for c in text:
        n = n * 58 + B58.index(c)
    body = n.to_bytes((n.bit_length() + 7) // 8, 'big')
    pad = len(text) - len(text.lstrip('1'))
    return b'\x00' * pad + body
```

**Base85** 走的是另一条路：不缩小字母表，而是**扩大**它，这样需要的字符就更少。四个字节（32 比特）当成一个整数、按 85 进制写出来，产生五个字符 —— 于是开销只有 25%，是实用编码里最好的。代价是字符集里全是标点，在 HTML、XML 和 shell 上下文里需要转义。Adobe 的 Ascii85、Git 的二进制补丁格式和 Z85 是你会遇到的变体。

### 认出你面对的是哪一种

这是实用的部分。拿到一坨不知道是什么的东西：

| 你看到 | 它多半是 |
|---|---|
| 只有 `0-9A-Fa-f`，长度为偶数 | Base16 |
| 只有 `A-Z` 和 `2-7`，常以若干 `=` 结尾 | Base32 |
| 大小写混合加 `+` 和 `/`，长度是 4 的倍数，末尾是 `=` 或 `==` | Base64 |
| 大小写混合加 `-` 和 `_`，没有填充 | Base64url |
| 大小写和数字，**没有** `0`、`O`、`I`、`l`、`+`、`/`，没有填充 | Base58 |
| 大小写、数字，以及大量标点 | Base85（或 Base91） |

两个值得拥有的细化：

- **长度会泄露底数。** Base32 的输出长度是 8 的倍数、base64 是 4 的倍数、base16 是 2 的倍数。当一个字符串长度是 8 的倍数、字符集又是大写字母加 2-7 时，你不需要猜。
- **熵不是信号。** Base64 **看起来**随机，但哈希、UUID、会话标识和加密后的数据块看起来也一样随机。**真正区分 base64 的是字符集与长度约束，不是随机程度。**

```python
import re

PATTERNS = [
    ('base64',      re.compile(r'^[A-Za-z0-9+/]+={0,2}$')),
    ('base64url',   re.compile(r'^[A-Za-z0-9_-]+$')),
    ('base32',      re.compile(r'^[A-Z2-7]+=*$')),
    ('base16',      re.compile(r'^[0-9A-Fa-f]+$')),
    ('base58',      re.compile(r'^[1-9A-HJ-NP-Za-km-z]+$')),
]

def guess_encoding(s):
    hits = []
    for name, pat in PATTERNS:
        if pat.match(s):
            if name == 'base64' and len(s) % 4:
                continue                      # 长度必须对得上
            if name == 'base16' and len(s) % 2:
                continue
            hits.append(name)
    return hits

for s in ['TWFu', 'TWE=', 'MZXW6===', '4d616e', '2NEpo7TZRRrLZSi2U', 'SGVsbG8sIHdvcmxkIQ==']:
    print(s, '->', guess_encoding(s))
```

### 嵌套编码，也就是 CTF 里的大头

层层编码极其常见：base64 套 base64、base64 里面是十六进制、base64 里面是 gzip 后的数据。破绽在于**解出来的结果本身就可识别**。

```python
import base64, binascii, gzip, zlib

def peel(s, max_layers=10):
    """反复解码，直到再也认不出可解的东西。"""
    for layer in range(max_layers):
        original = s
        try:
            if re.fullmatch(r'[A-Za-z0-9+/]+={0,2}', s) and len(s) % 4 == 0:
                s = base64.b64decode(s).decode('utf-8', 'replace')
            elif re.fullmatch(r'[0-9A-Fa-f]+', s) and len(s) % 2 == 0:
                s = binascii.unhexlify(s).decode('utf-8', 'replace')
            elif re.fullmatch(r'[A-Z2-7]+=*', s):
                s = base64.b32decode(s).decode('utf-8', 'replace')
        except Exception:
            break
        if s == original:
            break
        print(f'第 {layer + 1} 层: {s[:120]}')
    return s

peel('VTBkV2MySkhPSE5KU0ZKdllWaE5aMkZZVFdkWlUwSjZaRWRHZVdSQlBUMD0=')
```

最后这个例子是 base64 套 base64 再套 base64 的 `Hello, this is a start`。注意第一次解出来是一段看起来像 `U0dWc2...` 的文本，而它**本身**又是合法的 base64 —— 这就是"继续往下解"的信号。

一个实用警告：**不要盲目循环。** base64 解码可能偶然产出恰好也是合法 base64 的字节，循环就会一路跑下去、产出垃圾。字符集不再匹配、输出不再是可打印文本、或者到了固定层数，就该停。

### 它在真实攻击里的位置

| 场景 | 什么是 base64 | 为什么重要 |
|---|---|---|
| **JWT** | 三段 base64url：头部、载荷、签名 | 前两段任何人都能读 —— 永远不要把机密放进去 |
| **PowerShell `-EncodedCommand`** | 命令在 **UTF-16LE** 下的 base64 | UTF-16 这一点，就是解码器有时给你"每个字之间带空格"的文本的原因；而命令行本身就是检测信号 |
| **`certutil -decode` / `-urlcache`** | 文本文件里的 base64 或 URL | 一个微软签名的二进制，被用来写文件或下载载荷 |
| **经 DNS/HTTP 外泄** | 载荷被 base64 后放进子域名或参数 | 任意字节得以通过文本通道；长而高熵的标签就是检测信号 |
| **混淆脚本** | 字符串乃至整块代码被 base64 | 让字面字符串匹配失效 |
| **网页内联资源** | `data:image/png;base64,...` | 合法的用法，也是载荷可以藏身的地方 |

**PowerShell 这个细节值得记住**，因为它是个常见的绊脚石：`-EncodedCommand` 取的是 **UTF-16LE** 字符串的 base64，不是 UTF-8。所以：

```python
import base64
cmd = 'Get-Process'
encoded = base64.b64encode(cmd.encode('utf-16-le')).decode()
print(encoded)      # RwBlAHQALQBQAHIAbwBjAGUAcwBzAA==   <- note the 'A's: that is UTF-16LE
print(base64.b64decode(encoded).decode('utf-16-le'))
```

如果你把一个可疑的 `-EncodedCommand` 值按 UTF-8 解，得到的是每个字符之间都有空隙的文本，那就是 UTF-16LE，而你用错了编解码器。

### 检测与缓解

- **信号不是"存在 base64"，而是"base64 出现在了不该出现的地方"。** base64 在正常流量里到处都是：图片数据、令牌、证书、cookie。值得写一条规则的是**长而高熵的 base64 出现在本来没有理由携带它的位置** —— DNS 查询标签、本该放短标识符的 URL 参数、`User-Agent` 头、或者文件名。
- **盯命令行。** `-EncodedCommand`、`-enc`、`FromBase64String`、`certutil -decode`、`-urlcache`、`base64 -d` 管道进 shell。这些是真实入侵里会出现的具体字符串，而在命令行审计打开的前提下，对它们告警很便宜。
- **分诊时永远先解码。** 当一份报告里带着 base64 数据块时，在对其做任何判断之前先解开它。一半的"可疑未知字符串"调查在第一次解码就结束了，另一半会变得具体得多。这也是读 JWT 载荷最快的方式。
- **长度和熵放在一起就能缩小范围。** Base32 的长度是 8 的倍数，base64 是 4 的倍数。日志或 URL 里极长的一整段 token，不管字符集是什么都值得再看一眼。
- **对防守方来说，缓解在应用侧。** 永远不要把敏感数据用 base64 存起来或传出去、然后管它叫保护：**它按设计就是可读的**，而且它会出现在日志、错误信息和崩溃转储里，那是开发者预期不到的地方。数据需要机密性就用加密并管理密钥；需要完整性就签名。
- **最后记住：base64 打败的是匹配，不是分析。** 任何靠匹配字面字符串工作的控制 —— WAF 规则、黑名单、特征 —— 都可以通过把载荷编码绕过。对策是在**每个做检查的层里、在匹配之前先解码并规范化**，并且清楚哪一层负责解什么。一个盲目检查 base64、却不先解码的 WAF，检查的是错误的东西。
