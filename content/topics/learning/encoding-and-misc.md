---
id: encoding-and-misc
title_en: Encoding, Decoding and CTF Misc
title_zh: 编码解码与杂项（CTF Misc）
summary_en: The first thing a newcomer needs is not a technique, it is the habit of asking what a blob actually is. This entry is the beginner's starting point — how to tell encoding from encryption, how to recognise the common ones by sight, and an order of operations for attacking an unknown file.
summary_zh: 新手首先需要的不是某个手法，而是"先搞清楚眼前这坨东西到底是什么"的习惯。这一篇是新手的起点：怎么区分编码和加密、怎么一眼认出常见的几种、以及面对一个不明文件时的动手顺序。
tags: [beginner, ctf, misc, encoding, base64, hex, classical-cipher]
tools: [CyberChef, base64, xxd, file, strings, binwalk, zsteg]
attck: [T1027, T1140]
platform: [any]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Start here: encoding is not encryption

This is the mistake that costs beginners the most time, so it comes first.

**Encoding** changes how data is written so it can travel safely through a channel that only accepts certain characters. There is no key. Anyone can reverse it. Base64, URL encoding, hex and UTF-8 are encodings.

**Encryption** changes data so that only someone with a key can read it. AES, RSA and ChaCha20 are encryption.

When you see `SGVsbG8sIHdvcmxkIQ==` and say "it is encrypted", you have just made the puzzle harder than it is. It is Base64 for `Hello, world!`, and you can decode it in one command. Real CTF challenges rely on people not checking.

A useful second distinction is **hashing**: a one-way function with no reverse. `5d41402abc4b2a76b9719d911017c592` is the MD5 of `hello`, and you do not "decode" it — you look it up or crack it.

| What you see | Likely | Reversible? |
|---|---|---|
| Letters, digits, `+` `/` and `=` at the end | Base64 | Yes, one command |
| Letters and digits `2`-`7`, `=` at the end | Base32 | Yes |
| Only `0-9a-f`, even length, often in pairs | Hex | Yes |
| `%E4%B8%AD` style | URL encoding | Yes |
| `&#x4e2d;` or `&lt;` | HTML entities | Yes |
| `\u4e2d` | Unicode escape | Yes |
| Dots and dashes | Morse | Yes, with a table |
| Exactly 32 hex characters | MD5 — a hash, not an encoding | No, look it up |
| Exactly 40 hex characters | SHA-1 hash | No |

### Recognising the common encodings by sight

**Base64** is the one you will meet constantly. Its alphabet is `A-Z a-z 0-9 + /`, and its length is always a multiple of four, padded with `=`. A URL-safe variant replaces `+` and `/` with `-` and `_`.

```bash
echo 'SGVsbG8sIHdvcmxkIQ==' | base64 -d          # Linux / macOS
[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('SGVsbG8sIHdvcmxkIQ=='))   # PowerShell
python3 -c "import base64; print(base64.b64decode('SGVsbG8sIHdvcmxkIQ==').decode())"
```

**Hex** turns every byte into two characters. Files, memory addresses and hashes are all shown this way, so learn to read it:

```bash
xxd file.bin | head        # the standard hex dump
python3 -c "print(bytes.fromhex('48656c6c6f'))"
```

**URL encoding** appears wherever data travels in a URL: a space becomes `%20`, and Chinese text becomes a long run of `%E4%B8%AD` groups. **HTML entities** (`&lt;`, `&#x4e2d;`) are the same idea for HTML, and they matter in web challenges because a filter that misses them can be walked around.

**Character sets** are the reason Chinese text turns into mojibake. The same bytes mean different things in UTF-8 and GBK:

```bash
iconv -f GBK -t UTF-8 garbled.txt
file -i unknown.txt        # what encoding does this claim to be?
```

### The classical ciphers

These are the warm-up puzzles, and each has a signature:

| Cipher | How to spot it | How to break it |
|---|---|---|
| Caesar | Letters only, looks like shifted English | Try all 25 shifts |
| ROT13 | The Caesar shift of 13, common on forums | Decode with the same operation |
| Vigenère | Letters only, no shift works | Needs the key; try online solvers with a wordlist |
| Morse | Only `.` `-` space and `/` | A lookup table |
| Bacon | Groups of five A/B characters | Each group is a letter |
| Rail fence | Letters only, jumbled order | Try 2-10 rails |
| Atbash | Letters only, reversed alphabet | Mirror the alphabet |

**Keyboard shifts** are not ciphers but they show up constantly: text typed with the wrong keyboard layout, or one key to the right on QWERTY. If `hello` came out as `jr;;p`, someone's hand slipped one key — shift it back.

### Attacking an unknown blob: the order to work in

This procedure is the whole skill. Follow it in order rather than guessing.

1. **`file` it.** Never trust the extension. `file` reads the magic bytes and tells you what it really is.
2. **`xxd | head` it.** The first bytes identify the format even when `file` is unsure.
3. **`strings` it.** Readable text often gives away the next step: a URL, a hint, a filename, an error message.
4. **Look at the character set.** Only `[A-Za-z0-9+/=]`? Base64. Only `[0-9a-f]`? Hex. Only `.` and `-`? Morse.
5. **Write down each step as you take it.** Multi-layer puzzles are the norm, and you will need to backtrack. A file of intermediate results beats your memory.
6. **When a step fails, do not force it.** Check whether your assumption about the previous layer was wrong.
7. **Reconsider the container.** If decoding goes nowhere, the answer may be in the file's structure rather than its content: a modified header, appended data, a file inside a file.

```bash
file mystery.bin
xxd mystery.bin | head
strings -n 6 mystery.bin | head -20
binwalk mystery.bin        # anything appended or embedded?
```

### When the file is not what it claims

| Magic bytes | Format |
|---|---|
| `89 50 4E 47 0D 0A 1A 0A` | PNG |
| `FF D8 FF` | JPEG |
| `47 49 46 38` | GIF |
| `50 4B 03 04` | ZIP (and docx/xlsx/jar, which are ZIPs) |
| `1F 8B` | gzip |
| `42 5A 68` | bzip2 |
| `25 50 44 46` | PDF |
| `7F 45 4C 46` | ELF (Linux executable) |
| `4D 5A` | PE (Windows executable) |
| `52 61 72 21` | RAR |
| `37 7A BC AF 27 1C` | 7-Zip |

A file whose extension says `.jpg` but whose bytes say `50 4B 03 04` is a ZIP. Renaming is a two-second puzzle and it catches people every time.

### The CTF misc tricks worth knowing as a beginner

- **Multiple layers.** Hex inside Base64 inside a URL. Decode in order, keeping notes.
- **An edited header.** A PNG whose width and height fields were changed still opens, but shows only part of the image; repairing the values reveals the rest.
- **Appended data.** A ZIP glued to the end of a PNG after the `IEND` marker. `binwalk -e` or an offset-based `dd` gets it out.
- **Fake encryption in ZIP files.** The "encrypted" flag set with no password: fix the flag byte and the archive opens.
- **Archive passwords.** Try the obvious first, then a wordlist with `fcrackzip` or `john`.
- **QR codes.** `zbarimg code.png` reads them. If it will not scan, check the quiet zone and whether the image was flipped or partly overwritten.
- **Steganography in images and audio.** Hidden in the least significant bits, in metadata, or drawn into the frequency spectrum of a sound file. There is a separate entry in this guide for that.
- **Network captures.** A `.pcap` with something transmitted in the clear. Open it in Wireshark, follow the TCP stream, read the conversation.

### The toolchain a beginner should build first

| Tool | Why |
|---|---|
| **CyberChef** | A browser page where you drag operations into a recipe. Its "Magic" mode tries many decodings at once, which is the fastest way to identify an unknown blob |
| `file`, `xxd`, `strings` | The three commands you will run on every unknown file |
| `binwalk`, `foremost` | Find and extract data embedded inside other files |
| `exiftool` | Show every metadata field, including the ones nobody meant to ship |
| Python | When a puzzle needs a loop, write 5 lines instead of clicking 50 times |
| `zbarimg`, `steghide`, `zsteg` | QR codes, and the two most common image steganography tools |

For the classical ciphers, an online solver is the right tool: nobody hand-solves a Vigenère cipher when a website will do it in a second. What matters is that you can recognise which cipher it is.

### A worked example

A file called `flag.txt` contains this one line:

```
4a534f4e7b53306d3374316d33735f31745f31735f6a7573745f6833787d
```

Step 1: only `[0-9a-f]`, so it is hex. Decode it:

```bash
echo '4a534f4e7b...' | xxd -r -p
# JSON{s0m3t1m3s_1t_1s_just_h3x}
```

Step 2: that reads like a flag with a renamed prefix. Nothing more to decode — the puzzle was one step, and the lesson is that step 1 was the whole challenge. Notice that the answer was not encrypted, and guessing a key would have been wasted effort.

### Where this actually matters outside CTF

Encoding is not only a puzzle format. It is how a great deal of malicious code hides:

- PowerShell written as a Base64 `-EncodedCommand`, which is why `-enc` on a command line is a detection signal.
- JavaScript obfuscated with `eval(atob('...'))`, and macros that build strings with `Chr()` one character at a time.
- Payloads split into hex and reassembled at runtime so no readable string appears in the file.
- Data leaving a network inside DNS queries or URL parameters, encoded so it looks like noise.

### Detection and mitigation

- **Log the decoded form as well as the raw one.** A rule written against the Base64 of a known string only catches that one string; the decoded form catches the technique.
- **Decode before analysing**: when investigating an incident, write the decoded payload to a file — it is evidence, and it makes the next step possible.
- **Alert on encoded blobs in unusual places**: a long Base64 argument to `powershell.exe -enc`, a URL parameter that is mostly Base64, or DNS labels longer than 30 characters.
- **Teach the distinction, because it is a security control.** People who believe "it is Base64, so it is encrypted" put credentials in code, in URLs and in client-side scripts.
- **Never use encoding as protection.** Base64 in a mobile app or a web page is not a secret; it is a puzzle someone will solve in a minute.

<!-- lang:zh -->
### 从这里开始：编码不是加密

这是最耗新手时间的一个误解，所以放在最前面。

**编码**改变的是数据的**写法**，好让它能安全通过只接受特定字符的通道。它没有密钥，任何人都能还原。Base64、URL 编码、十六进制、UTF-8 都是编码。

**加密**改变的是数据本身，只有拿到密钥的人才能读懂。AES、RSA、ChaCha20 是加密。

当你看到 `SGVsbG8sIHdvcmxkIQ==` 就说"这是加密的"，你刚刚把题目变难了。它是 Base64，明文就是 `Hello, world!`，一条命令就能解。真实的 CTF 题目，很多就是靠人不先去核对这一点。

还有一个值得区分的概念是**哈希**：单向、不可逆。`5d41402abc4b2a76b9719d911017c592` 是 `hello` 的 MD5，它不是"解码"出来的，而是查表或爆破出来的。

| 你看到的 | 大概是 | 可逆吗 |
|---|---|---|
| 字母数字加 `+` `/`，结尾有 `=` | Base64 | 可逆，一条命令 |
| 字母数字，只用 `2`-`7`，结尾 `=` | Base32 | 可逆 |
| 全是 `0-9a-f`，长度为偶数 | 十六进制 | 可逆 |
| `%E4%B8%AD` 这种 | URL 编码 | 可逆 |
| `&#x4e2d;` 或 `&lt;` | HTML 实体 | 可逆 |
| `\u4e2d` | Unicode 转义 | 可逆 |
| 只有点和划 | 摩斯电码 | 可逆，查表 |
| 刚好 32 位十六进制 | MD5 —— 哈希，不是编码 | 不可逆，只能查 |
| 刚好 40 位十六进制 | SHA-1 哈希 | 不可逆 |

### 靠长相认出常见的几种

**Base64** 是你最常遇到的那个。字符集是 `A-Z a-z 0-9 + /`，长度永远是 4 的倍数，不足的用 `=` 补齐。URL 安全变体把 `+` `/` 换成 `-` `_`。

```bash
echo 'SGVsbG8sIHdvcmxkIQ==' | base64 -d          # Linux / macOS
[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('SGVsbG8sIHdvcmxkIQ=='))   # PowerShell
python3 -c "import base64; print(base64.b64decode('SGVsbG8sIHdvcmxkIQ==').decode())"
```

**十六进制**把每个字节变成两个字符。文件内容、内存地址、哈希全都这么显示，所以这个要练到能直接读：

```bash
xxd file.bin | head        # 标准 hex dump
python3 -c "print(bytes.fromhex('48656c6c6f'))"
```

**URL 编码**出现在任何走 URL 的数据里：空格变成 `%20`，中文变成一长串 `%E4%B8%AD`。**HTML 实体**（`&lt;`、`&#x4e2d;`）是同一思路用在 HTML 上 —— 在 Web 题里很重要，因为漏掉它们的过滤器可以绕过去。

**字符集**是中文变成乱码的原因。同一串字节在 UTF-8 和 GBK 里含义不同：

```bash
iconv -f GBK -t UTF-8 garbled.txt
file -i unknown.txt        # 它自称是什么编码？
```

### 古典密码

这些是热身题，每一种都有自己的特征：

| 密码 | 怎么认 | 怎么破 |
|---|---|---|
| 凯撒 | 只有字母，像被平移过的英文 | 25 个位移全试一遍 |
| ROT13 | 位移 13 的凯撒，论坛上很常见 | 用同一个操作再解一次 |
| 维吉尼亚 | 只有字母，任何单一位移都试不出来 | 需要密钥；拿字典去在线求解器跑 |
| 摩斯 | 只有 `.` `-` 空格和 `/` | 查表 |
| 培根 | 五个一组，只有 A/B 两种字符 | 每组对应一个字母 |
| 栅栏 | 只有字母，但顺序被打乱 | 试 2 到 10 栏 |
| 埃特巴什 | 只有字母，字母表反着来 | 把字母表镜像 |

**键盘错位**不算密码，但它出现的频率极高：用错输入法打出来的文字，或者手往右偏了一格。`hello` 打成 `jr;;p` 就是手滑了一格 —— 移回去就行。

### 面对一个不明文件，按这个顺序动手

这个流程本身就是新手最该练的技能。按顺序做，别一上来就猜。

1. **先 `file`。** 永远不要相信扩展名，`file` 会读魔术字节告诉你它到底是什么。
2. **`xxd | head`。** 就算 `file` 拿不准，头几个字节也能看出格式。
3. **`strings`。** 可读文字常常直接给出下一步：一个网址、一句提示、一个文件名、一段报错。
4. **看字符集。** 只有 `[A-Za-z0-9+/=]`？Base64。只有 `[0-9a-f]`？十六进制。只有 `.` `-`？摩斯。
5. **每做一步就记下来。** 多层编码是常态，你需要能往回退。写个中间结果文件，比靠脑子强。
6. **某一步解不出来时，不要硬试。** 先回头检查上一步的假设是不是错了。
7. **重新考虑这个容器本身。** 如果解码走进死胡同，答案可能在文件的结构里而不是内容里：被改过的文件头、附加在末尾的数据、文件里的文件。

```bash
file mystery.bin
xxd mystery.bin | head
strings -n 6 mystery.bin | head -20
binwalk mystery.bin        # 有没有附加或内嵌的东西？
```

### 当文件不是它自称的东西

| 魔术字节 | 格式 |
|---|---|
| `89 50 4E 47 0D 0A 1A 0A` | PNG |
| `FF D8 FF` | JPEG |
| `47 49 46 38` | GIF |
| `50 4B 03 04` | ZIP（docx/xlsx/jar 也都是 ZIP） |
| `1F 8B` | gzip |
| `42 5A 68` | bzip2 |
| `25 50 44 46` | PDF |
| `7F 45 4C 46` | ELF（Linux 可执行文件） |
| `4D 5A` | PE（Windows 可执行文件） |
| `52 61 72 21` | RAR |
| `37 7A BC AF 27 1C` | 7-Zip |

扩展名写着 `.jpg`、字节却是 `50 4B 03 04`，那它是个 ZIP。改扩展名只要两秒，但每次都有人栽在这上面。

### 新手该知道的 CTF misc 套路

- **多层编码。** URL 里套 Base64、Base64 里套十六进制。按顺序剥，边剥边记。
- **被改过的文件头。** PNG 的宽高字段被改过之后仍能打开，但只显示一部分；把值改回去，剩下的内容就出来了。
- **附加数据。** 一个 ZIP 粘在 PNG 的 `IEND` 标记之后。用 `binwalk -e` 或者按偏移 `dd` 取出来。
- **ZIP 伪加密。** 只把"已加密"标志位置上了，其实没有密码：改掉那个标志字节就能打开。
- **压缩包密码。** 先试最明显的，再用 `fcrackzip` 或 `john` 跑字典。
- **二维码。** `zbarimg code.png` 直接读。扫不出来时检查静默区，以及图是不是被翻转或覆盖过一部分。
- **图片与音频隐写。** 藏在最低有效位、元数据里，或者声音文件的频谱图里。本指南另有专篇。
- **流量包。** 一个 `.pcap`，里面有明文传输的东西。用 Wireshark 打开，追踪 TCP 流，读对话。

### 新手该先备好的工具

| 工具 | 为什么 |
|---|---|
| **CyberChef** | 一个网页，把各种操作拖成一个"配方"依次执行；它的 Magic 模式会一次尝试多种解码，是识别不明数据最快的方式 |
| `file`、`xxd`、`strings` | 面对任何不明文件你都会先跑这三条 |
| `binwalk`、`foremost` | 找出并提取藏在别的文件里的数据 |
| `exiftool` | 显示全部元数据字段，包括没人打算发布出去的那些 |
| Python | 题目需要循环时，写 5 行代码比点 50 次鼠标快 |
| `zbarimg`、`steghide`、`zsteg` | 二维码，以及最常用的两个图片隐写工具 |

古典密码就直接用在线求解器：维吉尼亚没有人手算，网站一秒就出结果。**你要练的是认出它是哪一种**。

### 一个完整的小例子

一个叫 `flag.txt` 的文件里只有一行：

```
4a534f4e7b53306d3374316d33735f31745f31735f6a7573745f6833787d
```

第一步：只有 `[0-9a-f]`，是十六进制。解开：

```bash
echo '4a534f4e7b...' | xxd -r -p
# JSON{s0m3t1m3s_1t_1s_just_h3x}
```

第二步：读起来像一个换了前缀的 flag，没有别的要解了 —— 题目就一层，而这正是重点：第一步就是全部。注意它**没有被加密**，如果去猜密钥，那是白费力气。

### 这些东西在 CTF 之外有什么用

编码不只是谜题格式，它也是大量恶意代码藏身的方式：

- PowerShell 用 Base64 写成的 `-EncodedCommand` —— 这也是为什么命令行里出现 `-enc` 会是一个检测信号。
- JavaScript 用 `eval(atob('...'))` 混淆，宏则用 `Chr()` 一个字符一个字符地拼字符串。
- 载荷拆成十六进制、运行时再拼回去，于是文件里不出现任何可读字符串。
- 数据借 DNS 查询或 URL 参数离开网络，编码之后看起来就像噪音。

### 检测与缓解

- **日志里同时留原始形态和解码后的形态。** 针对某个已知字符串的 Base64 写规则只能抓那一个字符串；针对解码后的形态写规则，抓的是这个手法。
- **分析之前先解码**：排查事件时，把解出来的载荷落成文件 —— 它既是证据，也让下一步成为可能。
- **对出现在异常位置的编码数据告警**：`powershell.exe -enc` 后面那一长串 Base64、几乎整段是 Base64 的 URL 参数、超过 30 个字符的 DNS 标签。
- **把这个区分讲给团队听，它本身就是一种安全控制。** 相信"Base64 就是加密"的人，会把凭据写进代码、URL 和前端脚本里。
- **永远不要拿编码当保护。** 移动应用或网页里的 Base64 不是秘密，那是一个有人一分钟就能解开的谜题。
