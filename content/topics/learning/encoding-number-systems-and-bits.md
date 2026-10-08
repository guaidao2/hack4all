---
id: encoding-number-systems-and-bits
title_en: "Encoding, Part 2 — Number Systems, Bits and Bytes"
title_zh: "编码（二）：进制、位与字节"
summary_en: Everything underneath the text you read is bits, and most of this field's sharp edges are there — octal that a validator reads as decimal, a length field in the wrong byte order, an integer that wraps. This entry covers number systems from the arithmetic rather than from the shortcuts, bit operations with what they are actually for, endianness, and the integer bugs that become vulnerabilities.
summary_zh: 你读到的文本底下全是比特，而这个领域大部分锋利的边缘都在那里 —— 校验器按十进制读的八进制、字节序写反的长度字段、回绕的整数。这一篇讲进制的算术而不是快捷方式、讲位运算各自的真实用途、讲字节序，以及那些会变成漏洞的整数 bug。
tags: [beginner, encoding, hex, bitwise, endianness, integer-overflow]
tools: [Python, xxd, CyberChef, gdb]
attck: [T1027, T1190]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why bits are worth an entry

Three reasons this matters more in security work than in ordinary programming:

1. **Every encoding is a bit operation.** The base family is regrouping bits; hex is a shorthand for them; UTF-8 is a byte-level scheme. If you can read bits, all of it stops looking like magic.
2. **Definitions disagree, and that is exploitable.** If one component reads `010` as ten and another reads it as eight, the difference is a bypass. Most real-world "weird input" bugs are two layers disagreeing about how to interpret the same bytes.
3. **The sharp edges are integer edges.** Overflow, truncation, signedness and endianness cause an enormous share of memory-safety vulnerabilities, and they are all visible if you are thinking in bytes.

### Number systems, from the arithmetic

A number system is **positional**: each digit's value is the digit times the base raised to the position. That one sentence generates every conversion.

```
Decimal 247   = 2×10² + 4×10¹ + 7×10⁰
Binary  11110111 = 1×2⁷ + 1×2⁶ + 1×2⁵ + 1×2⁴ + 0×2³ + 1×2² + 1×2¹ + 1×2⁰ = 247
Hex     F7    = 15×16¹ + 7×16⁰ = 247
```

**Why 2, 8 and 16, and not 3 or 10.** Because 8 and 16 are powers of two, binary converts to them by **grouping digits**: three bits per octal digit, four bits per hexadecimal digit. Nothing else has that property, which is why hex is the notation programmers actually use — it is binary with four times fewer characters and no ambiguity about where the bits are.

```
binary   1111 0111
hex         F    7      <- one hex digit per 4 bits, no arithmetic needed
octal    011 110 111
octal      3   6   7    <- one octal digit per 3 bits
```

**Converting decimal to another base** is repeated division, and the reason is the positional definition: dividing by the base peels off the lowest digit each time.

```
247 ÷ 16 = 15 remainder 7    <- lowest hex digit is 7
 15 ÷ 16 =  0 remainder 15   <- next is F
reading remainders bottom-up: F7
```

```python
def to_base(n, base):
    digits = '0123456789abcdefghijklmnopqrstuvwxyz'
    if n == 0:
        return '0'
    sign, n = ('-', -n) if n < 0 else ('', n)
    out = ''
    while n:
        n, r = divmod(n, base)
        out = digits[r] + out
    return sign + out

def from_base(s, base):
    digits = '0123456789abcdefghijklmnopqrstuvwxyz'
    s = s.lower().lstrip()
    sign = -1 if s.startswith('-') else 1
    s = s.lstrip('+-')
    n = 0
    for ch in s:
        n = n * base + digits.index(ch)      # Horner's method: no powers needed
    return sign * n

print(to_base(247, 16))          # f7
print(from_base('f7', 16))       # 247
print(to_base(247, 2))           # 11110111
print(from_base('11110111', 2))  # 247
```

Python's own versions, which you should prefer in real code:

```python
int('f7', 16)          # 247
int('0xf7', 16)        # 247  (the prefix is accepted)
int('0b1111', 2)       # 15
bin(247)               # '0b11110111'
hex(247)               # '0xf7'
oct(247)               # '0o367'
format(247, '08b')     # '11110111' — and this is how you pad to a fixed width
format(247, '#010x')   # '0x000000f7'
```

Note that last one: **`format` with a width is how padding is done**, and it is what every encoder in the base family is doing when it splits a value into fixed-width pieces.

### The ambiguity that becomes a vulnerability

`010` is **ten** in most contexts and **eight** in any language or parser that treats a leading zero as octal. That gap has produced real bypasses:

| Layer | How it reads `010` |
|---|---|
| Python `int('010')` | 10 (raises on `int('010', 0)`? no — base 0 raises, base 10 gives 10) |
| C, C++, Java, JavaScript (legacy octal literals) | 8 in some forms |
| PHP `intval('010')` | 10, but `intval('010', 8)` is 8 — and PHP's `is_numeric` says it is fine |
| `chmod` | 8 (octal is the whole point) |
| IP address parsers | **127 for `0177`** — which is how `0177.0.0.1` reaches localhost |

That last row is the classic one. A filter that blocks `127.0.0.1` and `localhost` to stop server-side request forgery can be walked past with any other spelling of the same address:

| Spelling | What it is |
|---|---|
| `127.0.0.1` | Dotted decimal |
| `0177.0.0.1` | Octal first octet |
| `0x7f.0.0.1` | Hex first octet |
| `2130706433` | The whole address as a 32-bit integer |
| `0x7f000001` | The same, in hex |
| `127.1` | Short form: the trailing part is the remaining bytes |
| `127.0.0.1.` | Trailing dot — some resolvers accept it |

All of these are the same four bytes, and a blocklist that matches strings sees seven different things:

```python
import socket, struct

def spell_out(ip):
    """Every common spelling of one IPv4 address."""
    packed = socket.inet_aton(ip)
    n = struct.unpack('!I', packed)[0]
    a, b, c, d = packed
    return {
        'dotted': ip,
        'integer': str(n),
        'hex integer': hex(n),
        # only a segment whose octal form differs from decimal needs the 0 prefix
        'octal octets': '.'.join(('0%o' % x) if x >= 8 else str(x) for x in packed),
        'hex octets': '0x%x' % a + f'.{b}.{c}.{d}',
        'short': f'{a}.{b}.{c}.{d}' if d == 0 else f'{a}.{(b << 16) | (c << 8) | d}',
        'trailing dot': ip + '.',
    }

for k, v in spell_out('127.0.0.1').items():
    print(f'{k:14} {v}')
```

The defensive lesson is the important half: **validate the parsed value, not the string.** Resolve the input to an address first, then apply the policy to the address. Any check performed on the text is a check on one spelling of a thing that has many.

### Bit operations, and what each is for

| Operator | Name | What it is actually for |
|---|---|---|
| `&` | AND | **Masking** — keep certain bits, clear the rest |
| `\|` | OR | **Setting** — force certain bits to 1 |
| `^` | XOR | **Toggling**, and the basis of stream ciphers and checksums |
| `~` | NOT | Inversion, usually combined with a mask |
| `<<` | Left shift | Multiply by a power of two, and **assemble fields** |
| `>>` | Right shift | Divide by a power of two, and **extract fields** |

Truth tables, for reference:

```
AND  0&0=0  0&1=0  1&0=0  1&1=1
OR   0|0=0  0|1=1  1|0=1  1|1=1
XOR  0^0=0  0^1=1  1^0=1  1^1=0
```

```python
x = 0b11001010          # 202

x & 0b00001111          # 10   — low nibble: masking
x | 0b00000001          # 203  — force bit 0 on: setting a flag
x ^ 0b11111111          # 53   — invert the low byte: toggling
(x >> 4) & 0b1111       # 12   — high nibble: extract, then mask
(0b1010 << 4) | 0b0011  # 163  — assemble two nibbles into one byte: 0xA3
x & (1 << 3)            # 8    — test bit 3 (non-zero means set)
x | (1 << 2)            # 206  — set bit 2
x & ~(1 << 2)           # 202  — clear bit 2
x ^ (1 << 0)            # 203  — toggle bit 0
```

**Why `>>` and `&` always go together** when reading a field: shifting alone brings the bits down but leaves everything above them; the mask is what isolates them. Forgetting the mask is a common source of bugs in parsers, and a common source of *under*-reporting in analysis tools.

#### Flags, which are bits used as a set

A single integer can carry many yes/no values, one per bit. You will see this everywhere:

- **File permissions.** `755` is three octal digits, each three bits: `111 101 101` — read/write/execute for owner, group and other. `chmod` is bit manipulation with a friendly front end.
- **TCP flags.** SYN, ACK, FIN, RST, PSH, URG each occupy a bit in one byte — which is why a capture tool can show them as a letter list.
- **Windows access masks.** Permissions are a 32-bit field, and many are combinations of others (`GENERIC_ALL` includes a set of specific rights).
- **Cipher and capability negotiation.** A bitmap of supported features, which is why "which bit is this" questions come up in protocol work.

```python
TCP_FIN, TCP_SYN, TCP_RST, TCP_PSH, TCP_ACK, TCP_URG = (1 << i for i in range(6))

flags = 0x12                      # 0b010010 = ACK + SYN
print('SYN' if flags & TCP_SYN else '-', 'ACK' if flags & TCP_ACK else '-')
```

#### XOR, and why it is the simplest stream cipher

XOR has four properties that together make it the foundation of stream encryption:

```
a ^ a = 0          the same value cancels
a ^ 0 = a          zero is neutral
a ^ b = b ^ a      commutative
(a ^ b) ^ c = a ^ (b ^ c)   associative
```

From which: **`(plaintext ^ key) ^ key = plaintext`.** The same operation encrypts and decrypts, which is why XOR is the first thing anyone writes.

```python
def xor_bytes(data: bytes, key: bytes) -> bytes:
    return bytes(b ^ key[i % len(key)] for i, b in enumerate(data))

cipher = xor_bytes(b'secret message', b'KEY')
print(xor_bytes(cipher, b'KEY'))          # b'secret message'
```

**And now the connection that matters.** That function *is* a Vigenère cipher over bits — a repeating key stream, exactly the structure from the classical ciphers entry. Which means it inherits the same fatal flaw:

- **Reusing the key across messages** gives `C1 ^ C2 = P1 ^ P2`, the two-time pad, and both messages fall to crib dragging.
- **A key as long as the message and never reused** is a one-time pad and is genuinely unbreakable — and genuinely impractical for the reasons that entry lists.

Every real stream cipher is this, with the repeating key replaced by a cryptographically generated stream and a nonce that must never repeat. That is why the nonce rule is a rule.

### Hex, bytes and reading a dump

One byte is exactly two hex digits, so a hex dump is a direct view of the bytes.

```
$ xxd logo.png | head -3
00000000: 8950 4e47 0d0a 1a0a 0000 000d 4948 4452  .PNG........IHDR
00000010: 0000 0200 0000 0100 0806 0000 0072 5b8b  .............r[.
00000020: 7d00 0000 0970 4859 7300 000f 6100 000f  }....pHYs...a...
```

Read it in three columns: **offset** (where in the file), **hex** (the bytes), **ASCII** (the same bytes, dots where they are not printable). The PNG magic `89 50 4E 47 0D 0A 1A 0A` is right there — and note that the ASCII column shows `.PNG....`, because `0x89` is not valid ASCII. That is the same reasoning the steganography entry uses when it says to look at the structure before the content.

```python
data = open('logo.png', 'rb').read(32)
print(data.hex())                      # hex string
print(' '.join(f'{b:02x}' for b in data))
print(bytes.fromhex('89504e47'))       # b'\x89PNG'
print(data[:8] == bytes.fromhex('89504e470d0a1a0a'))   # True for a PNG
```

### Endianness

A multi-byte value has to be written down in *some* order. **Big-endian** puts the most significant byte first; **little-endian** puts it last. Neither is wrong; they are conventions, and mismatches between them are a classic bug class.

The value `0x12345678`:

| Order | Byte sequence in memory |
|---|---|
| Big-endian | `12 34 56 78` |
| Little-endian | `78 56 34 12` |

**Where you meet each:**

| Context | Order |
|---|---|
| Network protocols (TCP/IP headers) | **Big-endian** — "network byte order" |
| x86 and x86-64 memory | Little-endian |
| ARM | Bi-endian, usually little in practice |
| PNG, JPEG | Big-endian |
| BMP, GIF | Little-endian |
| ELF headers | Depends on the target; the header itself declares it |

**Why it is a security issue** rather than a trivia question: if one component writes a length in one order and another reads it in the other, the length becomes a wildly different number. A 1000-byte field read as little-endian instead of big-endian becomes 3,895,936,384 — and if that number is used to allocate a buffer or bound a copy, the result is an overflow or an out-of-bounds read. Byte-order mismatches between parsing layers are a recurring source of vulnerabilities in protocol stacks and file format parsers.

```python
import struct

n = 0x12345678
print(n.to_bytes(4, 'big').hex())        # 12345678
print(n.to_bytes(4, 'little').hex())     # 78563412
print(int.from_bytes(bytes.fromhex('78563412'), 'little'))   # 0x12345678

# struct is the explicit way, and the angle brackets are endianness, not decoration
print(struct.pack('<I', n).hex())        # 78563412  (< little)
print(struct.pack('>I', n).hex())        # 12345678  (> big)
print(struct.unpack('>H', bytes.fromhex('0100'))[0])   # 256, not 1
```

**Bit order is a separate question.** Byte order says how bytes are arranged; **bit order** says which end of a byte is "first". Most protocols send the most significant bit first, but not all, and some hardware is LSB-first. This matters directly in steganography: LSB embedding means writing into the **least significant bit** of each byte, and a tool that reads MSB-first will find nothing. When a bit-level scheme does not work, check the bit order before concluding it is not there.

### Integer bugs that become vulnerabilities

This is the section that earns the entry, because these four failure modes appear in real CVEs constantly.

#### Overflow and wraparound

A fixed-width integer has a maximum. Add one and it wraps:

```python
import ctypes
u8 = ctypes.c_ubyte(255)
u8.value = (u8.value + 1) % 256
print(u8.value)                     # 0 — wrapped

i8 = ctypes.c_int8(127)
print((i8.value + 1 + 128) % 256 - 128)   # -128 — signed overflow wraps to negative
```

Python itself has **arbitrary-precision integers**, so `2**100` is fine and nothing wraps. That is convenient, and it is a trap: exploit scripts written in Python must **simulate** the target's width explicitly, or the arithmetic will not match what the vulnerable binary does.

```python
def u32(x):
    return x & 0xFFFFFFFF           # what a 32-bit unsigned target would do
```

#### Truncation

Assigning a large value to a small type silently discards the high bits — no error, no warning in most languages.

```python
size = 0x100000004          # 4294967300
truncated = size & 0xFFFF   # what a 16-bit field keeps: 4
print(hex(truncated))       # 0x4
```

**The attack shape**: a check is performed on the large value, and the *truncated* value is used afterwards. "Length must be less than 1024" passes because the length is huge; then the low 16 bits are used for allocation, and the copy writes far more than was allocated. This is textbook, and it still ships.

#### Signedness

Mixing signed and unsigned is where the surprises live. A negative value compared against an unsigned bound becomes enormous:

```python
length = -1
# in C, if length is signed int and the bound is size_t (unsigned), the comparison
# promotes length to a huge unsigned value, so this check passes:
if (length <= MAX_LEN)          # -1 becomes 0xFFFFFFFFFFFFFFFF -> true
```

That is the classic shape of a length-validation bypass: the check passes, the allocation is tiny, and the copy uses the negative length as an unsigned size.

#### Why Python hides all of it, and why that matters

Python has arbitrary-precision integers and no unsigned types. So:

- Your local testing **cannot reproduce** an overflow bug by accident — you must add the masking yourself.
- The target almost certainly **does** wrap, so exploit parameters that look fine in Python may be truncated on the wire.
- When writing protocol traffic, use `struct.pack` with an explicit format so the width and endianness are stated, rather than relying on default behaviour.

### Detection and mitigation

- **The generalisable detection signal is "the same input interpreted two ways".** Alternate spellings of an address, a numeric field that is valid as one base and not another, a length that is plausible when read with one byte order and absurd with the other. In logs, that shows up as requests whose parameters are unusual in *form* while being ordinary in value — a numeric parameter padded with a leading zero, an oversized number where a small one belongs, a consistently rejected path attempted with different encodings. Grouping by decoded value rather than by raw string is what makes these visible.
- **Normalise before you validate, and validate the value not the string.** Parse an address, then apply policy to the address. Parse a number, decide its type and width, then check bounds. Every string-based check is a check on one of infinitely many spellings.
- **State the endianness explicitly, everywhere.** `struct` format strings, `to_bytes(..., 'big')`, network byte order at protocol boundaries. Most byte-order vulnerabilities exist because a conversion was implicit and someone assumed the other convention.
- **Use unsigned types for lengths and sizes, and check for overflow before you allocate.** Better: use a language or library where the conversion is checked (Rust's `TryFrom`, Go's explicit conversions, `SafeInt` in C++), so a truncated value is a compile or runtime error rather than a silent surprise. And validate the arithmetic result, not just the input: `if (a + b > MAX)` is itself an overflow; write `if (a > MAX - b)`.
- **Test the boundaries deliberately.** Zero, one, the maximum, the maximum plus one, negative values, and values whose low bits differ from their high bits. Every one of the integer bugs above is caught by a test at a boundary, and none of them is caught by ordinary functional testing.
- **When analysing, read the dump.** For a suspicious binary or a protocol problem, the hex view is the ground truth. Magic bytes, field widths, endianness and padding are all directly visible, and a surprising amount of "the tool is wrong" turns out to be "the field is not where I assumed".

<!-- lang:zh -->
### 为什么比特值得单独一篇

在安全工作里，这件事比在普通编程里重要得多，有三个理由：

1. **每一种编码都是位操作。** Base 家族是重新分组比特；十六进制是比特的简写；UTF-8 是字节层面的方案。你要是能读比特，这些就都不再像魔法。
2. **各层的定义会不一致，而这就是可利用点。** 如果一个组件把 `010` 读成十、另一个读成八，这个差异就是一个绕过。现实中大多数"奇怪输入"的 bug，都是两层对同一串字节的解释不一致。
3. **锋利的边缘是整数的边缘。** 溢出、截断、符号、字节序，造成了内存安全漏洞里极大的一部分，而只要你用字节思考，它们全都看得见。

### 进制，从算术讲起

进制是**位置记数法**：每一位的值 = 该位数字 × 基数的该位次幂。就这一句话，能推出所有转换。

```
十进制 247   = 2×10² + 4×10¹ + 7×10⁰
二进制 11110111 = 1×2⁷ + 1×2⁶ + 1×2⁵ + 1×2⁴ + 0×2³ + 1×2² + 1×2¹ + 1×2⁰ = 247
十六进制 F7  = 15×16¹ + 7×16⁰ = 247
```

**为什么是 2、8、16，而不是 3 或 10。** 因为 8 和 16 都是 2 的幂，所以二进制转它们只需要**分组**：三位二进制对应一位八进制，四位对应一位十六进制。别的基数没有这个性质 —— 这就是十六进制成为程序员实际使用的记法的原因：**它就是二进制，只是字符少了四分之三，而且比特的位置毫无歧义。**

```
二进制   1111 0111
十六进制    F    7      <- 每 4 比特一位，不需要算术
八进制   011 110 111
八进制     3   6   7    <- 每 3 比特一位
```

**十进制转其他进制**是反复取余，而理由就在位置记数法的定义里：除以基数，每次剥掉最低位。

```
247 ÷ 16 = 15 余 7    <- 最低的十六进制位是 7
 15 ÷ 16 =  0 余 15   <- 下一位是 F
把余数从下往上读：F7
```

```python
def to_base(n, base):
    digits = '0123456789abcdefghijklmnopqrstuvwxyz'
    if n == 0:
        return '0'
    sign, n = ('-', -n) if n < 0 else ('', n)
    out = ''
    while n:
        n, r = divmod(n, base)
        out = digits[r] + out
    return sign + out

def from_base(s, base):
    digits = '0123456789abcdefghijklmnopqrstuvwxyz'
    s = s.lower().lstrip()
    sign = -1 if s.startswith('-') else 1
    s = s.lstrip('+-')
    n = 0
    for ch in s:
        n = n * base + digits.index(ch)      # 霍纳法则：不需要算幂
    return sign * n

print(to_base(247, 16))          # f7
print(from_base('f7', 16))       # 247
print(to_base(247, 2))           # 11110111
print(from_base('11110111', 2))  # 247
```

Python 自带的那几个，真实代码里该优先用：

```python
int('f7', 16)          # 247
int('0xf7', 16)        # 247  （前缀会被接受）
int('0b1111', 2)       # 15
bin(247)               # '0b11110111'
hex(247)               # '0xf7'
oct(247)               # '0o367'
format(247, '08b')     # '11110111' —— 这就是补齐固定宽度的做法
format(247, '#010x')   # '0x000000f7'
```

注意最后那个：**带宽度参数的 `format` 就是做补齐的方式**，而 Base 家族里每一个"把数值切成固定宽度片段"的编码器，做的都是这件事。

### 那个会变成漏洞的歧义

`010` 在多数语境里是**十**，而在任何把前导零当八进制的语言或解析器里是**八**。这个差距制造过真实的绕过：

| 层 | 它把 `010` 读成 |
|---|---|
| Python `int('010')` | 10 |
| C、C++、Java、JavaScript（遗留八进制字面量） | 某些形式下是 8 |
| PHP `intval('010')` | 10，但 `intval('010', 8)` 是 8 —— 而 `is_numeric` 也认它 |
| `chmod` | 8（八进制本来就是它的本意） |
| IP 地址解析器 | **`0177` 是 127** —— 而 `0177.0.0.1` 就是这样够到 localhost 的 |

最后一行是经典的那个。一个为了防服务端请求伪造而屏蔽 `127.0.0.1` 和 `localhost` 的过滤器，可以被同一个地址的任何其他写法绕过去：

| 写法 | 它是什么 |
|---|---|
| `127.0.0.1` | 点分十进制 |
| `0177.0.0.1` | 第一段是八进制 |
| `0x7f.0.0.1` | 第一段是十六进制 |
| `2130706433` | 整个地址作为一个 32 位整数 |
| `0x7f000001` | 同上，十六进制写法 |
| `127.1` | 短形式：后面那段代表剩下的字节 |
| `127.0.0.1.` | 末尾多个点 —— 有些解析器接受 |

这些全都是同样四个字节，而一个做字符串匹配的黑名单会看到七种不同的东西：

```python
import socket, struct

def spell_out(ip):
    """同一个 IPv4 地址的各种常见写法。"""
    packed = socket.inet_aton(ip)
    n = struct.unpack('!I', packed)[0]
    a, b, c, d = packed
    return {
        'dotted': ip,
        'integer': str(n),
        'hex integer': hex(n),
        # only a segment whose octal form differs from decimal needs the 0 prefix
        'octal octets': '.'.join(('0%o' % x) if x >= 8 else str(x) for x in packed),
        'hex octets': '0x%x' % a + f'.{b}.{c}.{d}',
        'short': f'{a}.{b}.{c}.{d}' if d == 0 else f'{a}.{(b << 16) | (c << 8) | d}',
        'trailing dot': ip + '.',
    }

for k, v in spell_out('127.0.0.1').items():
    print(f'{k:14} {v}')
```

防守那一半才是要紧的：**校验解析后的值，而不是字符串。** 先把输入解析成一个地址，再对该地址施加策略。任何做在文本上的检查，检查的都是"一个有很多种写法的东西"的其中一种写法。

### 位运算，以及每一个是干什么的

| 运算符 | 名称 | 它实际的用途 |
|---|---|---|
| `&` | 与 | **掩码** —— 保留某些位、清掉其余 |
| `\|` | 或 | **置位** —— 把某些位强制为 1 |
| `^` | 异或 | **翻转**，以及流密码与校验和的基础 |
| `~` | 取反 | 按位求反，通常和掩码一起用 |
| `<<` | 左移 | 乘 2 的幂，以及**拼装字段** |
| `>>` | 右移 | 除 2 的幂，以及**提取字段** |

真值表，备查：

```
与   0&0=0  0&1=0  1&0=0  1&1=1
或   0|0=0  0|1=1  1|0=1  1|1=1
异或 0^0=0  0^1=1  1^0=1  1^1=0
```

```python
x = 0b11001010          # 202

x & 0b00001111          # 10   —— 低四位：掩码
x | 0b00000001          # 203  —— 把第 0 位强制为 1：置标志
x ^ 0b11111111          # 53   —— 低字节取反：翻转
(x >> 4) & 0b1111       # 12   —— 高四位：先提取，再掩码
(0b1010 << 4) | 0b0011  # 163  —— 把两个四位拼成一个字节：0xA3
x & (1 << 3)            # 8    —— 测第 3 位（非零即已置位）
x | (1 << 2)            # 206  —— 置第 2 位
x & ~(1 << 2)           # 202  —— 清第 2 位
x ^ (1 << 0)            # 203  —— 翻转第 0 位
```

**读字段时 `>>` 和 `&` 为什么总是一起出现**：单靠移位只是把目标位移下来了，它上面还有别的位；掩码才是隔离它们的那一步。忘掉掩码在解析器里是常见 bug，在分析工具里则是常见的**低报**。

#### 标志位：把比特当成一个集合用

一个整数可以携带许多是/否的值，每位一个。你会到处看到它：

- **文件权限。** `755` 是三个八进制位，每位三个比特：`111 101 101` —— 属主、组、其他人各自的读/写/执行。`chmod` 就是给位操作套了个友好的外壳。
- **TCP 标志。** SYN、ACK、FIN、RST、PSH、URG 各占一个字节里的一位 —— 所以抓包工具能把它们显示成一串字母。
- **Windows 访问掩码。** 权限是一个 32 位字段，其中很多是别的权限的组合（`GENERIC_ALL` 包含一组具体权限）。
- **密码套件与能力协商。** 一个受支持特性的位图，所以协议工作里会出现"这是第几位"的问题。

```python
TCP_FIN, TCP_SYN, TCP_RST, TCP_PSH, TCP_ACK, TCP_URG = (1 << i for i in range(6))

flags = 0x12                      # 0b010010 = ACK + SYN
print('SYN' if flags & TCP_SYN else '-', 'ACK' if flags & TCP_ACK else '-')
```

#### 异或，以及它为什么是最简单的流密码

异或有四个性质，合起来使它成为流式加密的基础：

```
a ^ a = 0          相同的值相互抵消
a ^ 0 = a          零是中性的
a ^ b = b ^ a      交换律
(a ^ b) ^ c = a ^ (b ^ c)   结合律
```

由此推出：**`(明文 ^ 密钥) ^ 密钥 = 明文`**。同一个操作既加密也解密，所以它是所有人写的第一个东西。

```python
def xor_bytes(data: bytes, key: bytes) -> bytes:
    return bytes(b ^ key[i % len(key)] for i, b in enumerate(data))

cipher = xor_bytes(b'secret message', b'KEY')
print(xor_bytes(cipher, b'KEY'))          # b'secret message'
```

**现在说那个要紧的连接。** 这个函数**就是**比特层面的维吉尼亚密码 —— 一段循环使用的密钥流，和古典密码那篇里的结构一模一样。于是它继承了同一个致命缺陷：

- **跨消息重用密钥** 就得到 `C1 ^ C2 = P1 ^ P2`，即两次一密，两条消息都会倒在 crib dragging 之下。
- **与消息等长、且永不重用的密钥** 就是一次性密码本，它确实不可破 —— 也确实是不可用的，理由那一篇列过了。

每一个真实的流密码都是这个东西，只是把"循环的密钥"换成了密码学生成的密钥流，以及一个绝不能重复的 nonce。**这就是 nonce 规则之所以是规则的原因。**

### 十六进制、字节，以及怎么读一份 dump

一个字节正好是两位十六进制，所以十六进制 dump 就是字节的直接视图。

```
$ xxd logo.png | head -3
00000000: 8950 4e47 0d0a 1a0a 0000 000d 4948 4452  .PNG........IHDR
00000010: 0000 0200 0000 0100 0806 0000 0072 5b8b  .............r[.
00000020: 7d00 0000 0970 4859 7300 000f 6100 000f  }....pHYs...a...
```

三列读它：**偏移**（在文件里的位置）、**十六进制**（字节）、**ASCII**（同样的字节，不可打印的地方显示成点）。PNG 的魔数 `89 50 4E 47 0D 0A 1A 0A` 就在那儿 —— 注意 ASCII 列显示的是 `.PNG....`，因为 `0x89` 不是合法的 ASCII。这和隐写那篇说"先看结构、再看内容"是同一个道理。

```python
data = open('logo.png', 'rb').read(32)
print(data.hex())                      # 十六进制字符串
print(' '.join(f'{b:02x}' for b in data))
print(bytes.fromhex('89504e47'))       # b'\x89PNG'
print(data[:8] == bytes.fromhex('89504e470d0a1a0a'))   # PNG 为 True
```

### 字节序

多字节的值总得按**某种**顺序写下来。**大端**把最高有效字节放在最前；**小端**放在最后。两者都没有错，它们只是约定，而两者之间的错配是一类经典的 bug。

值 `0x12345678`：

| 顺序 | 内存里的字节序列 |
|---|---|
| 大端 | `12 34 56 78` |
| 小端 | `78 56 34 12` |

**你会在哪些地方遇到它们：**

| 场景 | 顺序 |
|---|---|
| 网络协议（TCP/IP 头） | **大端** —— 即"网络字节序" |
| x86 与 x86-64 的内存 | 小端 |
| ARM | 双端，实际中通常是小端 |
| PNG、JPEG | 大端 |
| BMP、GIF | 小端 |
| ELF 头 | 取决于目标；头本身会声明 |

**为什么它是安全问题而不是冷知识**：如果一个组件按一种顺序写长度、另一个按另一种顺序读，那个长度就会变成一个天差地别的数。一个 1000 字节的字段被当成小端而不是大端读，就变成 3,895,936,384 —— 而这个数如果被用来分配缓冲区或限制拷贝，结果就是溢出或越界读。**解析层之间的字节序错配，是协议栈与文件格式解析器里反复出现的漏洞来源。**

```python
import struct

n = 0x12345678
print(n.to_bytes(4, 'big').hex())        # 12345678
print(n.to_bytes(4, 'little').hex())     # 78563412
print(int.from_bytes(bytes.fromhex('78563412'), 'little'))   # 0x12345678

# struct 是显式的做法，尖括号是字节序，不是装饰
print(struct.pack('<I', n).hex())        # 78563412  （< 小端）
print(struct.pack('>I', n).hex())        # 12345678  （> 大端）
print(struct.unpack('>H', bytes.fromhex('0100'))[0])   # 256，不是 1
```

**位序是另一个问题。** 字节序说的是字节怎么排；**位序**说的是一个字节里哪一端算"开头"。多数协议先发最高有效位，但不是全部，而且有些硬件是 LSB 优先。这和隐写直接相关：LSB 嵌入的意思是往每个字节的**最低有效位**里写，而一个 MSB 优先读的工具什么都找不到。**当一个位层面的方案不奏效时，先检查位序，再下结论说东西不在那里。**

### 会变成漏洞的整数 bug

这一节才是这篇的价值所在，因为这四种失效模式在真实 CVE 里不断出现。

#### 溢出与回绕

固定宽度的整数有最大值，加一就绕回去：

```python
import ctypes
u8 = ctypes.c_ubyte(255)
u8.value = (u8.value + 1) % 256
print(u8.value)                     # 0 —— 绕回去了

i8 = ctypes.c_int8(127)
print((i8.value + 1 + 128) % 256 - 128)   # -128 —— 有符号溢出绕成负数
```

Python 本身是**任意精度整数**，所以 `2**100` 没问题、什么都不会绕。这很方便，也是陷阱：用 Python 写的利用脚本必须**显式模拟**目标的宽度，否则算术结果和那个有漏洞的二进制不一致。

```python
def u32(x):
    return x & 0xFFFFFFFF           # 32 位无符号目标会做的事
```

#### 截断

把一个大值赋给小类型，高位会被静默丢掉 —— 没有错误，在多数语言里也没有警告。

```python
size = 0x100000004          # 4294967300
truncated = size & 0xFFFF   # 16 位字段留下的是：4
print(hex(truncated))       # 0x4
```

**攻击的形状**：检查做在那个大值上，而之后用的是**截断后**的值。"长度必须小于 1024"通过了，因为这个长度巨大；接着低 16 位被用来分配，而拷贝写入的远多于分配量。这是教科书级别的手法，而它至今仍在出货。

#### 符号

有符号和无符号混用是惊喜的来源。一个负值跟一个无符号上界比较时会变成巨大的数：

```python
length = -1
# 在 C 里，如果 length 是有符号 int 而上界是 size_t（无符号），比较会把 length
# 提升为一个巨大的无符号值，于是这个检查通过：
if (length <= MAX_LEN)          # -1 变成 0xFFFFFFFFFFFFFFFF -> 真
```

这就是长度校验被绕过的经典形状：检查通过、分配很小、而拷贝把那个负长度当成无符号大小来用。

#### 为什么 Python 把这一切都藏起来了，以及这为什么重要

Python 有任意精度整数、没有无符号类型。所以：

- **本地测试不会**意外复现出溢出 bug —— 你得自己加掩码。
- 目标几乎一定会**回绕**，所以那些在 Python 里看起来没问题的利用参数，到了线上可能已经被截断。
- 构造协议流量时用 `struct.pack` 配显式格式串，把宽度和字节序写出来，不要依赖默认行为。

### 检测与缓解

- **可推广的检测信号是"同一个输入被解释成了两种意思"。** 一个地址的多种写法、一个在某种进制下合法而在另一种下不合法的数值字段、一个按某种字节序读很合理而按另一种读荒谬至极的长度。在日志里，它们表现为**形式上不寻常、值上却普通**的请求 —— 数值参数前面多了个零、本该很小的地方出现了超大数字、同一条被拒绝的路径换着编码反复尝试。**按解码后的值分组，而不是按原始字符串分组**，才能让这些显出来。
- **先规范化再校验，而且校验值、不校验字符串。** 把地址解析出来，再对地址施加策略。把数字解析出来、确定它的类型和宽度，再查边界。每一个基于字符串的检查，检查的都是无穷多种写法中的一种。
- **处处显式声明字节序。** `struct` 格式串、`to_bytes(..., 'big')`、协议边界上的网络字节序。多数字节序漏洞之所以存在，是因为转换是隐式的，而某人默认了另一种约定。
- **长度和大小一律用无符号类型，并在分配之前检查溢出。** 更好的做法是使用转换被检查的语言或库（Rust 的 `TryFrom`、Go 的显式转换、C++ 的 `SafeInt`），这样被截断的值是编译期或运行期的错误，而不是静默的意外。而且要校验**运算结果**，不只是输入：`if (a + b > MAX)` 本身就是一次溢出，应写成 `if (a > MAX - b)`。
- **刻意测边界值。** 零、一、最大值、最大值加一、负数，以及那些"低位与高位不一样"的值。上面每一个整数 bug 都能被一个边界测试抓到，而它们**一个都不会**被常规功能测试抓到。
- **分析的时候读 dump。** 面对可疑的二进制或协议问题，十六进制视图才是地面真相。魔数、字段宽度、字节序和填充全都直接可见，而"工具错了"里有相当一部分最后被证明是"字段并不在我以为的位置"。
