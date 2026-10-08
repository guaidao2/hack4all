---
id: steganography-fundamentals
title_en: Steganography and Covert Channels
title_zh: 隐写术与隐蔽通道
summary_en: Steganography hides data inside other data so that nothing looks hidden. In CTF it is a puzzle; in the real world it is an exfiltration channel that survives inspection because there is nothing obviously unusual to inspect.
summary_zh: 隐写是把数据藏进别的数据里，让一切看起来都不像藏着东西。在 CTF 里它是一道谜题；在现实中，它是一条能躲过检查的外泄通道 —— 因为表面上没有明显异常可供检查。
tags: [steganography, covert-channel, exfiltration, ctf, c2]
tools: [binwalk, exiftool, zsteg, steghide, Sonic Visualiser, foremost]
attck: [T1027.003, T1041]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The idea, and why it matters outside CTF

Encryption makes data unreadable, which attracts attention. Steganography makes data invisible, which does not. That difference is why it appears in two real places:

- **Exfiltration.** Data encoded into images uploaded to a public image host, or into DNS queries, leaves the network without looking like data leaving the network.
- **C2.** Commands hidden in the padding of a protocol, in HTTP headers, or in the timing between packets.

The malicious document entry in this guide covers the case of a payload embedded in an Office file, which is steganography in service of initial access.

### Images

The richest category, because image formats have a lot of slack:

| Technique | Where the data lives | How to detect it |
|---|---|---|
| LSB substitution | The lowest bit of each colour channel | `zsteg`, StegSolve bit planes, chi-square analysis |
| Palette manipulation | The order or values of an indexed palette | Compare against a re-saved version |
| Metadata | EXIF, XMP, ICC profiles, comments | `exiftool` shows everything, including fields nobody meant to ship |
| Appended data | After the end-of-file marker | `binwalk`, `foremost`, or `tail` after the IEND/EOI |
| Geometry | Dimensions in the header, sometimes modified | A CRC mismatch or an image that renders oddly |
| Multiple IDAT chunks | Splitting data across PNG chunks | Compare chunk structure with a re-encoded image |

The standard first pass, in order:

```bash
file suspect.png
exiftool suspect.png                 # metadata is where the lazy hiding happens
strings -n 8 suspect.png | head      # sometimes it is not even hidden
binwalk suspect.png                  # appended archives and filesystems
binwalk -e suspect.png               # extract them
zsteg -a suspect.png                 # LSB and bit-plane analysis
steghide extract -sf suspect.jpg     # needs a passphrase, often blank
```

Two CTF classics worth knowing because they appear in real tooling too:

- **A PNG whose height was edited** will fail its CRC check but still render. Fix the height and the hidden content becomes visible (there are scripts that brute-force the correct value).
- **A ZIP or another image appended after the end marker.** `binwalk -e` or an offset-based `dd` extracts it. Some tools then still open the original image normally, which is exactly the point.

### Audio

- **Spectrograms** are the first thing to look at: open the file in Audacity or Sonic Visualiser and switch to spectrogram view. Text, morse code and images are frequently drawn directly in the frequency domain.
- **LSB in samples**, the same idea as images but with different tools.
- **Echo hiding** and phase coding, which survive format conversion better and are used in watermarking.
- **DTMF tones**, which encode digits audibly.

### Text and documents

- **Zero-width characters** (`U+200B`, `U+200C`, `U+FEFF`) encode bits while being invisible. A copy-paste into a hex editor or a script that maps them to binary reveals the message.
- **Whitespace** at the end of lines, or double versus single spaces, encodes bits the same way.
- **Acrostics and first-letter patterns**, which are essentially free to implement and easy to miss.
- **PDF**: objects not referenced by any page, text in white on white, content in comments.
- **Office files** are ZIP archives, so an extra part (a custom XML, an image with steganography) rides along unnoticed.
- **File system metadata**: EXIF in a photo, document properties, and NTFS alternate data streams (`file.txt:hidden.txt`), which most copy operations drop and most tools do not display.

### Network covert channels

This is the category that matters for detection, because it does not need a file at all:

- **DNS**: data in the subdomain of queries, in TXT records, or in the label lengths. Long, random-looking labels at a regular interval are the signature.
- **ICMP**: payload beyond the header, which nothing legitimate needs.
- **HTTP**: data in headers, cookies, unusual parameters, or the ordering of requests.
- **Timing**: intervals between packets encoding bits, which is slow but very hard to see in flow data.
- **Protocol padding**: unused or reserved fields that a tolerant parser ignores.

### Detection

- **Entropy and regularity in outbound traffic.** Encoded data looks random, and random-looking data leaving a corporate network on a schedule is the signal. DNS labels longer than thirty characters, or a fixed query interval, are both worth an alert.
- **Volume relative to the channel.** A DNS server receiving ten megabytes a day of query data from one host is doing something other than resolving names.
- **Statistical analysis of images** that leave the organisation, where the threat model includes data exfiltration through uploads. Chi-square and RS analysis detect LSB substitution; they are not perfect, but they beat nothing.
- **DLP with format awareness**, since a photo that contains an appended archive is not what the extension claims.
- **Baseline the volume** of uploads per user and alert on the change, which catches the case no file inspection will.

### Mitigation

- **Treat every untrusted file as a container.** Parse it, re-encode it, and store the re-encoded version — which strips metadata, appended data and most steganographic payloads in one step.
- **Strip metadata on upload** as a matter of policy, since it is also a privacy control.
- **Do not allow arbitrary outbound DNS.** Force queries through a resolver you control and log them; this removes the easiest covert channel.
- **Inspect outbound volume and behaviour**, not just content, because the content of a covert channel is designed to look fine.
- **For malware analysis, assume the payload is not in the obvious place.** Check appended data, metadata, alternate data streams, and the parts of the file the format does not require.
- **Remember the asymmetry**: hiding is cheap and detection is expensive, so the practical controls are re-encoding files and monitoring the channel rather than hunting for the payload.

<!-- lang:zh -->
### 这个想法，以及它在 CTF 之外为什么重要

加密让数据不可读，这会引来注意；隐写让数据不可见，而这不引人注意。这个差别，正是它出现在两个真实场景里的原因：

- **数据外泄。** 把数据编码进图片上传到公开图床，或者编码进 DNS 查询 —— 数据离开了网络，却不像"数据离开网络"。
- **C2。** 把命令藏在协议填充、HTTP 请求头，或者包与包之间的时序里。

本指南的"恶意文档分析"那篇讲的是把载荷嵌进 Office 文件的情形，那是为初始访问服务的隐写。

### 图片

最丰富的一类，因为图片格式有大量冗余空间：

| 手法 | 数据藏在哪 | 怎么发现 |
|---|---|---|
| LSB 替换 | 每个颜色通道的最低位 | `zsteg`、StegSolve 的位平面、卡方分析 |
| 调色板操纵 | 索引调色板的顺序或取值 | 与重新保存过的版本对比 |
| 元数据 | EXIF、XMP、ICC profile、注释 | `exiftool` 会显示一切，包括没人打算发布的字段 |
| 附加数据 | 文件结束标记之后 | `binwalk`、`foremost`，或在 IEND/EOI 之后 `tail` |
| 几何信息 | 头部里的宽高，有时被改过 | CRC 不匹配，或渲染结果怪异 |
| 多个 IDAT 块 | 把数据分散到 PNG 的多个块里 | 与重新编码的图片对比块结构 |

标准的第一轮排查，按顺序：

```bash
file suspect.png
exiftool suspect.png                 # 偷懒的隐藏都藏在元数据里
strings -n 8 suspect.png | head      # 有时候根本没藏，只是没人看
binwalk suspect.png                  # 附加的归档与文件系统
binwalk -e suspect.png               # 把它们解出来
zsteg -a suspect.png                 # LSB 与位平面分析
steghide extract -sf suspect.jpg     # 需要口令，通常是空的
```

两个 CTF 经典值得知道，因为它们在真实工具链里同样出现：

- **被改过高度的 PNG** 会校验失败却仍能渲染。把高度修对，藏起来的内容就露出来了（有脚本可以爆破出正确值）。
- **在结束标记之后附加的 ZIP 或另一个图片。** 用 `binwalk -e` 或按偏移 `dd` 就能解出。有些工具之后还能正常打开原图 —— 这正是它的用意。

### 音频

- **频谱图**是第一个该看的东西：用 Audacity 或 Sonic Visualiser 打开，切到频谱视图。文字、摩斯码和图像经常就画在频域里。
- **采样值的 LSB**，思路和图片一样，工具不同。
- **回声隐藏与相位编码**，它们更能扛格式转换，常用于水印。
- **DTMF 音**，用可听的方式编码数字。

### 文本与文档

- **零宽字符**（`U+200B`、`U+200C`、`U+FEFF`）编码比特而不可见。复制进十六进制编辑器，或用脚本把它们映射成二进制，消息就出来了。
- **空白字符**：行尾的空白，或者单空格与双空格之别，同样能编码比特。
- **藏头与首字母规律**，实现成本几乎为零，也容易漏看。
- **PDF**：没有任何页面引用的对象、白底白字、注释里的内容。
- **Office 文件**就是 ZIP，所以多出来的一个部件（自定义 XML、一张做过隐写的图片）会顺路一起走而无人注意。
- **文件系统元数据**：照片里的 EXIF、文档属性，以及 NTFS 的备用数据流（`file.txt:hidden.txt`）—— 大多数复制操作会把它丢掉，大多数工具也不会显示。

### 网络隐蔽通道

这一类对检测最要紧，因为它根本不需要文件：

- **DNS**：数据放在查询的子域、TXT 记录，或标签长度里。又长又随机、间隔规律的标签就是特征。
- **ICMP**：头部之后还带载荷，而任何正常用途都不需要。
- **HTTP**：数据放在请求头、cookie、不寻常的参数，或者请求的先后顺序里。
- **时序**：用包的间隔编码比特，很慢，但在流数据里非常难看出来。
- **协议填充**：宽容的解析器会忽略的未使用或保留字段。

### 检测

- **出站流量的熵与规律性。** 编码后的数据看起来是随机的，而"看起来随机、又按固定节奏离开企业网络"就是信号。DNS 标签超过三十个字符，或者查询间隔固定，都值得一条告警。
- **相对于通道本身的体量。** 一台主机每天让 DNS 服务器承载十兆字节的查询数据，它干的事肯定不是解析域名。
- **出网图片的统计分析**（当威胁模型包含"通过上传外泄数据"时）。卡方与 RS 分析能识别 LSB 替换；它们不完美，但比什么都不做强。
- **带格式感知的 DLP** —— 一张图片里附着一个归档，与它的扩展名所声明的内容并不相符。
- **给每用户的上传量做基线**并对变化告警，这能抓住任何文件检查都抓不住的情况。

### 缓解

- **把每个不可信文件当作容器。** 解析它、重新编码、存重新编码后的版本 —— 一步就剥掉了元数据、附加数据和大多数隐写载荷。
- **上传时按策略剥离元数据**，它同时也是一种隐私控制。
- **不要允许任意出站 DNS。** 强制查询走你控制的解析器并记录日志，这去掉了最简单的那条隐蔽通道。
- **检查出站体量与行为，而不只是内容** —— 因为隐蔽通道的内容就是被设计成看起来没问题的。
- **做样本分析时，假设载荷不在显眼处。** 检查附加数据、元数据、备用数据流，以及格式本身并不要求的那些部分。
- **记住这个不对称**：隐藏很便宜，检测很贵。所以务实的控制是"重新编码文件 + 监控通道"，而不是"寻找载荷"。
