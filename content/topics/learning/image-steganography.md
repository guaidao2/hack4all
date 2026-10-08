---
id: image-steganography
title_en: Image Steganography from Scratch
title_zh: 图片隐写从零开始
summary_en: Before you can hide data in an image, or find what somebody else hid, you have to know how an image is stored. Pixels, colour channels, chunk structure and compression decide which hiding places exist, so this entry builds that foundation first and then walks every common technique with the commands to find it.
summary_zh: 想在图片里藏数据、或者把别人藏进去的数据找出来，先得知道图片是怎么存的。像素、颜色通道、数据块结构和压缩方式，决定了哪些位置能藏东西 —— 所以这一篇先把这层地基打好，再逐一把常见手法和对应的查找命令走一遍。
tags: [beginner, ctf, steganography, png, jpeg, bmp, lsb]
tools: [zsteg, stegsolve, exiftool, binwalk, foremost, dd, steghide, pngcheck, Python]
attck: [T1027.003, T1041]
platform: [any]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Why a beginner should learn the file format first

Almost every image steganography puzzle is solved by knowing one fact about how images are stored. The person who knows that a PNG is a series of named chunks, and that the image data lives in some of them, will find appended data in seconds. The person who only knows "run a steg tool" will try six tools in the wrong order and give up.

So this entry is in two halves. The first explains what an image actually is on disk. The second is the catalogue of hiding places, each with the command that finds it.

The short version for a terminal, if you only remember one thing:

```bash
file suspect.png          # what is it really
exiftool suspect.png      # metadata is where lazy hiding happens
strings -n 8 suspect.png  # sometimes it is not even hidden
binwalk suspect.png       # anything appended or embedded
```

### Part 1: what an image is on disk

**A raster image is a grid of pixels.** Each pixel has one or more numbers describing its colour. A typical colour image uses **three channels** — red, green and blue — and often a fourth, **alpha**, for transparency. So one pixel is three or four bytes.

That gives us the two quantities worth keeping in mind:

- **Width x height = number of pixels.** A 1920x1080 image has about two million pixels, so about six million bytes of raw colour data.
- **Bit depth = how many bits per channel.** 8 bits per channel (the usual case) means each channel is a value from 0 to 255. A 24-bit image means 8 bits for each of R, G and B.

**The formats differ in how they store that grid.** This is the part that matters:

| Format | How the pixels are stored | Consequences |
|---|---|---|
| **BMP** | Raw, pixel after pixel, sometimes padded per row | Largest files, trivially readable, ideal for learning |
| **PNG** | Lossless compression, in named chunks | Exact reconstruction, so LSB changes survive |
| **JPEG** | Lossy, transformed into frequency coefficients | Small files, but re-saving destroys small changes |
| **GIF** | An indexed palette (max 256 colours) plus LZW | The palette itself is a hiding place |

**PNG in more detail,** because it comes up constantly. A PNG file is a signature followed by a sequence of **chunks**. Each chunk has a length, a four-letter type, its data, and a CRC of the type and data:

```
89 50 4E 47 0D 0A 1A 0A     <- the 8-byte PNG signature
[length][type][data][crc]   <- a chunk
[length][type][data][crc]   <- another chunk
...
```

The chunk types you will meet:

| Chunk | Holds |
|---|---|
| `IHDR` | Width, height, bit depth, colour type — always first |
| `PLTE` | The palette, for indexed images |
| `IDAT` | The actual image data, possibly split across several chunks |
| `tEXt` / `zTXt` / `iTXt` | Text metadata — a documented, easy hiding place |
| `IEND` | Marks the end of the image — anything after it is not part of the PNG |

That last row is why appended data works: a viewer stops at `IEND`, so a ZIP glued on after it is invisible to the image but perfectly present in the file.

**JPEG** is different in a way that matters for steganography. It divides the image into 8x8 blocks, transforms each into frequency coefficients, and quantises them — throwing away detail the eye will not miss. That is why JPEG is small, and also why editing a JPEG's pixels and saving it again destroys anything hidden in the low bits. Steganography in JPEG therefore happens in the coefficient domain, not the pixel domain.

### Part 2: the hiding places, and how to find each one

#### 1. Data appended after the image

**The idea.** Write the image, then write a ZIP or another file after it. Every image viewer stops at the image's end marker. The file is now a valid PNG *and* a valid archive.

**Find it.** Look at the end of the file and at what the file claims to contain:

```bash
binwalk suspect.png              # lists embedded or appended data
binwalk -e suspect.png           # extracts it
foremost -i suspect.png -o out/  # a second opinion
```

**Manually,** when `binwalk` is unsure. Find where the PNG ends and look at what follows:

```bash
xxd suspect.png | tail -5        # look for IEND (49 45 4E 44) and what comes after
# then cut from the offset just after it
dd if=suspect.png of=tail.bin bs=1 skip=<offset>
file tail.bin
```

**How to spot it without tools.** The file size is wrong. A PNG of that resolution should be some size; if it is three times larger, something else is in there. `ls -l` and a rough expectation of image size is a real skill.

#### 2. LSB — the least significant bit

**The idea.** Changing the last bit of a colour value changes it by 1 out of 255. Nobody can see that. So if you take the lowest bit of each channel, you get a stream of bits you can fill with a message, and the picture looks identical.

For a 1920x1080 image, the low bits of three channels give you 1920 x 1080 x 3 = 6.2 million bits, which is about 777 KB of hidden data. That is why this is the classic technique.

**Why PNG and BMP, not JPEG.** PNG is lossless, so the low bit you changed is still there when the file is read back. JPEG throws away exactly that kind of detail, so the message would not survive its own compression.

**Find it.** `zsteg` is the tool built for this, and it checks many variants at once:

```bash
zsteg suspect.png                # try everything
zsteg -a suspect.png             # try everything, including slow modes
zsteg -E 'b1,r,lsb,xy' suspect.png > out.bin   # extract one specific mode
```

Read the `zsteg` output like this: it prints each channel/bit/order combination it tried and any text it found. A hit looks like `b1,rgb,lsb,xy .. text: "flag{...}"`.

**Do it by hand,** because understanding this once makes every variant obvious. This extracts the low bit of the red channel, in reading order:

```python
from PIL import Image
img = Image.open('suspect.png').convert('RGB')
bits = ''
for r, g, b in img.getdata():
    bits += str(r & 1)                 # the lowest bit of the red channel
data = bytes(int(bits[i:i+8], 2) for i in range(0, len(bits) - 7, 8))
print(data[:80])
```

If the message comes out as garbage, the variant is different — try green or blue, try the second-lowest bit, or try reading in column order instead of row order. `zsteg` exists because there are dozens of these combinations.

**Variants worth knowing.** The bit can be taken from one channel or from all three; in row order or column order; from the lowest bit or the second; and the message can be terminated by a length prefix, a null byte, or just run to the end.

#### 3. Metadata

**The idea.** Image formats have documented fields for text: EXIF in JPEG, `tEXt`/`iTXt` chunks in PNG, comments in GIF. They are meant for camera model and creation date and are frequently used for anything else.

**Find it.** One command shows everything:

```bash
exiftool suspect.png
exiftool -b -ThumbnailImage suspect.jpg > thumb.jpg   # extract an embedded thumbnail
```

Look for fields that do not belong: an unusually long `Comment`, a `Software` field with something in it, base64 in a description, or a thumbnail that does not match the image.

**Do not skip this step because it seems boring.** In real challenges and in real incidents, metadata is where the payload is, precisely because everyone jumps straight to the exciting LSB tools.

#### 4. A modified width or height

**The idea.** A PNG's `IHDR` chunk declares the dimensions, and the CRC protects the whole chunk. Change the height to something smaller and the image renders as a cropped version, hiding the lower part of the picture. The CRC no longer matches, but many viewers do not check it.

**Find it.** Check the CRCs:

```bash
pngcheck -v suspect.png
# a line like: IHDR chunk, invalid CRC
```

**Fix it.** Read the real dimensions from the image data and repair the header, or brute-force the height until the CRC matches. Here is the idea in Python — try heights until the CRC is valid:

```python
import struct, zlib, binascii

data = open('suspect.png', 'rb').read()
# IHDR data starts at byte 16: width(4) height(4) depth(1) colour(1) ...
w, h = struct.unpack('>II', data[16:24])
for test_h in range(1, 3000):
    chunk = data[12:16] + struct.pack('>II', w, test_h) + data[24:29]
    crc = binascii.crc32(chunk) & 0xffffffff
    if crc == struct.unpack('>I', data[29:33])[0]:
        print('real height is', test_h)
        break
```

That loop is the whole trick, and it is worth typing once so the idea sticks.

#### 5. The palette, in indexed images

**The idea.** GIF and some PNGs do not store colours per pixel. They store a **palette** of at most 256 colours, and each pixel is an index into it. Change the order of the palette, or add entries that are never used, and you have changed the file without changing the picture.

**Find it.** Compare the palette with a re-saved version, or look at it directly:

```bash
zsteg suspect.gif            # zsteg handles GIF palettes too
python3 -c "
from PIL import Image
img = Image.open('suspect.gif')
print(img.getpalette()[:30])   # the first colours, in order
"
```

#### 6. A second image inside the file

**The idea.** Two pictures in one file: the visible one, and the real one behind it. Common in challenge images, where you are supposed to notice that the file has more data than one image needs.

**Find it.** `binwalk` again, then extract and inspect what comes out. If the extracted file is another image, open it.

#### 7. The alpha channel, and other visual tricks

**The idea.** The fourth channel (transparency) is often ignored visually — a fully transparent pixel still has colour values. So data can be hidden in the RGB values of pixels that are invisible, or the picture can simply be adjusted so something becomes visible.

**Find it.** Look at each channel in isolation. `stegsolve` is the classic tool for this: it shows the image with only red, only green, only blue, only alpha, and with each bit plane isolated, and you page through them looking for a shape. Any image editor can show a single channel; `stegsolve` just makes the paging fast.

python
```python
from PIL import Image
img = Image.open('suspect.png').convert('RGBA')
img.getchannel('A').show()     # only the alpha channel
```

Adjusting **contrast and brightness** also works surprisingly often: hidden content is frequently a low-contrast shape that a level adjustment makes obvious.

#### 8. JPEG coefficient steganography

**The idea.** Since JPEG stores frequencies rather than pixels, hiding happens in the coefficients. Tools like `jsteg`, `outguess` and `F5` do this, and most need a passphrase.

**Find it.** These are hard to detect by hand. Check whether the file was produced by a known tool:

```bash
steghide info suspect.jpg        # will ask for a passphrase
outguess -r suspect.jpg out.txt  # if outguess was used
```

If no passphrase is available and the tool is unknown, this is where a challenge usually expects another clue. In practice, most CTF image challenges use PNG or BMP precisely because JPEG is hard.

#### 9. Files that are not images at all

The reverse trick: a file named `.png` whose contents are something else. Always check the magic bytes before anything else.

```bash
file suspect.png     # "PNG image data" or "Zip archive data"
```

### Your toolchain, and when to reach for each

| Tool | Use it for | Install |
|---|---|---|
| `file`, `xxd`, `strings` | The first three commands on anything | Already present |
| `exiftool` | Every metadata field, including thumbnails | Common package |
| `binwalk`, `foremost` | Appended and embedded data | Common package |
| `zsteg` | PNG/BMP LSB and palette analysis | Ruby gem |
| `stegsolve` | Visual inspection of channels and bit planes | A Java jar |
| `pngcheck` | Chunk structure and CRC validation | Common package |
| `steghide`, `outguess` | JPEG and WAV with a passphrase | Common package |
| Python + Pillow | When a puzzle needs ten lines of logic | `pip install pillow` |

### The order to actually work in

Do not start with the exciting tools. Work from cheapest to most expensive, and stop when something works:

1. `file` — is this even what it claims?
2. `strings -n 8` — is anything obviously there?
3. `exiftool` — metadata.
4. `ls -l` versus expectation — is the file too big?
5. `pngcheck -v` (PNG) — are there structural errors?
6. `binwalk` — is something appended?
7. `zsteg -a` (PNG/BMP) — LSB and palette.
8. `steghide info` (JPEG/WAV) — a passphrase-protected payload?
9. Visual: channels, bit planes, contrast.
10. Write a script — when none of the above fits, the puzzle wants your own logic.

### Practise by hiding something yourself

The fastest way to understand this is to build one. Make an image with a message in the low bits, then find it with the tools:

```python
from PIL import Image
img = Image.open('cover.png').convert('RGB')
pixels = list(img.getdata())
msg = 'flag{you_found_me}' + '\0'
bits = ''.join(f'{ord(c):08b}' for c in msg)

out = []
for i, (r, g, b) in enumerate(pixels):
    if i < len(bits):
        r = (r & 0xFE) | int(bits[i])   # replace the lowest bit
    out.append((r, g, b))
img.putdata(out)
img.save('hidden.png')
```

Now run `zsteg hidden.png` and watch it find your own message. You will never be confused by an LSB challenge again.

### Where this matters outside CTF

Hiding data in images is not only a puzzle:

- **Exfiltration through an allowed channel.** An insider who can upload images to a public host can move data out inside the pixels, and nothing looks like a file transfer.
- **Malware payloads.** An image that carries a script or an executable in its metadata or appended data, fetched by a loader. This is the same trick as a malicious document, with a different container.
- **C2 through images.** A beacon that fetches a picture from a legitimate-looking host and reads commands from its bits.
- **Watermarking and tracking.** The legitimate use: embedding an identifier to trace a leaked copy.

### Detection and mitigation

- **Do not trust the extension or the MIME type.** Inspect the bytes; a file whose header says ZIP is a ZIP regardless of its name.
- **Strip and re-encode on upload.** Parsing an image and writing it out again destroys appended data, metadata and almost all LSB payloads in one operation. This is the single most effective control, and it is also what you want for privacy.
- **Monitor the volume, not just the content.** A covert channel is designed to look normal, so baseline how much each user uploads and alert on the change.
- **Watch outbound destinations.** An image upload to a host your organisation does not use is worth a question regardless of what is inside it.
- **Check for structural anomalies at the gateway** where you can: CRC failures, chunks after `IEND`, and dimensions that do not match the data size are all cheap signals.
- **Log the decoded form.** If a payload was hidden in metadata or in an appended archive, extract it into the case file — it is evidence, and the next analyst will need it.

<!-- lang:zh -->
### 新手为什么该先学文件格式

几乎每一道图片隐写题，都是靠"知道图片是怎么存的"这一个事实解开的。知道 PNG 是一连串具名数据块、图像数据住在其中某几个块里的人，几秒钟就能找到附加数据；只知道"跑个隐写工具"的人，会用错误的顺序试六个工具，然后放弃。

所以这一篇分两半。前半讲清楚一张图片在磁盘上到底是什么；后半是藏匿点的目录，每一个都给出找到它的命令。

如果你只记得一条，就记这四行：

```bash
file suspect.png          # 它到底是什么
exiftool suspect.png      # 偷懒的隐藏都在元数据里
strings -n 8 suspect.png  # 有时候根本没藏，只是没人看
binwalk suspect.png       # 后面有没有附加或内嵌的东西
```

### 第一部分：图片在磁盘上是什么

**位图就是一张像素格子。** 每个像素用一到几个数字描述它的颜色。普通彩色图用**三个通道** —— 红、绿、蓝 —— 常常还有第四个 **alpha**（透明度）。所以一个像素是三到四个字节。

由此得到两个该记住的量：

- **宽 × 高 = 像素个数。** 1920×1080 约两百万像素，原始颜色数据约六百万字节。
- **位深 = 每个通道用几位。** 每通道 8 位（最常见）意味着每个通道取值 0 到 255。24 位图就是 R、G、B 各 8 位。

**各种格式的差别就在"怎么存这张格子"** —— 这才是关键：

| 格式 | 像素怎么存 | 后果 |
|---|---|---|
| **BMP** | 原样一个挨一个，有时按行补齐 | 文件最大、最好读，非常适合用来学原理 |
| **PNG** | 无损压缩，放在具名数据块里 | 能精确还原，所以改 LSB 不会丢 |
| **JPEG** | 有损，转成频率系数 | 文件小，但重新保存会毁掉微小改动 |
| **GIF** | 索引调色板（最多 256 色）+ LZW | 调色板本身就是藏东西的地方 |

**PNG 值得细讲**，因为它出现得最多。一个 PNG 就是一段签名，后面跟着一串**数据块（chunk）**。每块由长度、四字母类型、数据，以及"类型+数据"的 CRC 组成：

```
89 50 4E 47 0D 0A 1A 0A     <- 8 字节的 PNG 签名
[长度][类型][数据][CRC]      <- 一个块
[长度][类型][数据][CRC]      <- 又一个块
...
```

你会遇到的块类型：

| 块 | 装什么 |
|---|---|
| `IHDR` | 宽、高、位深、颜色类型 —— 永远在第一个 |
| `PLTE` | 调色板，索引色图才有 |
| `IDAT` | 真正的图像数据，可能分成好几块 |
| `tEXt` / `zTXt` / `iTXt` | 文本元数据 —— 一个文档化、又特别好用的藏匿点 |
| `IEND` | 图像结束标记 —— 它之后的东西都不属于这张 PNG |

最后一行就是"附加数据"能成立的原因：看图程序读到 `IEND` 就停了，所以粘在它后面的 ZIP 对图片来说不存在，但在文件里好端端地待着。

**JPEG** 的存法不一样，而这个不一样对隐写很要紧。它把画面切成 8×8 的小块，每块变换成频率系数再做量化 —— 把眼睛不会注意的细节丢掉。所以 JPEG 文件小，也因此**改完像素再保存会毁掉藏在低位里的东西**。JPEG 的隐写因此发生在**系数域**，不在像素域。

### 第二部分：藏匿点，以及怎么一个个找出来

#### 一、附加在图片后面的数据

**原理。** 先写图片，再在后面接一个 ZIP 或别的文件。所有看图程序都会在图片结束标记处停下。于是这个文件**既是**合法 PNG，**也是**合法压缩包。

**怎么找。** 看文件末尾，也看它到底包含了什么：

```bash
binwalk suspect.png              # 列出内嵌或附加的数据
binwalk -e suspect.png           # 解出来
foremost -i suspect.png -o out/  # 第二种意见
```

**`binwalk` 拿不准时手工来。** 找到 PNG 结束的位置，看后面是什么：

```bash
xxd suspect.png | tail -5        # 找 IEND（49 45 4E 44）以及它后面跟了什么
# 然后从它之后那个偏移开始切
dd if=suspect.png of=tail.bin bs=1 skip=<偏移>
file tail.bin
```

**不用工具怎么察觉。** 文件大小不对。这个分辨率的 PNG 大概应该是多大；如果大了三倍，里面就有别的东西。`ls -l` 加上对图片体积的粗略预期，是一项真本事。

#### 二、LSB —— 最低有效位

**原理。** 把一个颜色值的最低位改掉，它只变了 255 分之 1。没人看得出来。所以如果把每个通道的最低位取出来，你得到一串比特，可以往里灌消息，而画面一模一样。

1920×1080 的图，三个通道的最低位给你 1920 × 1080 × 3 = 620 万比特，约 777 KB 的隐藏容量。这就是它成为经典手法的原因。

**为什么是 PNG 和 BMP，不是 JPEG。** PNG 无损，你改掉的最低位读回来还是那个值。JPEG 丢掉的恰恰就是这类细节，消息会被它自己的压缩毁掉。

**怎么找。** `zsteg` 就是为这个造的，它会一次试很多种变体：

```bash
zsteg suspect.png                # 把能试的都试一遍
zsteg -a suspect.png             # 连慢的模式也试
zsteg -E 'b1,r,lsb,xy' suspect.png > out.bin   # 只按某一种模式提取
```

读 `zsteg` 输出的方法：它会打印它试过的每个"通道/位/顺序"组合，以及在其中找到的文本。命中长这样 `b1,rgb,lsb,xy .. text: "flag{...}"`。

**手工做一遍**，因为理解这一次，之后所有变体都一眼就懂。下面这段取红色通道的最低位，按阅读顺序：

```python
from PIL import Image
img = Image.open('suspect.png').convert('RGB')
bits = ''
for r, g, b in img.getdata():
    bits += str(r & 1)                 # 红色通道的最低位
data = bytes(int(bits[i:i+8], 2) for i in range(0, len(bits) - 7, 8))
print(data[:80])
```

如果出来是乱码，说明变体不同 —— 换绿或蓝、换第二位、或者改成按列读取。`zsteg` 的存在，就是因为这类组合有几十种。

**值得知道的变体。** 位可以取自单个通道或三个通道；按行序或列序；取最低位或次低位；消息可以用长度前缀、空字节结尾，或者一直铺到末尾。

#### 三、元数据

**原理。** 图片格式都有文档化的文本字段：JPEG 里的 EXIF、PNG 里的 `tEXt`/`iTXt` 块、GIF 里的注释。它们本来是给相机型号和拍摄时间用的，而经常被拿去装别的东西。

**怎么找。** 一条命令全看：

```bash
exiftool suspect.png
exiftool -b -ThumbnailImage suspect.jpg > thumb.jpg   # 把内嵌的缩略图取出来
```

找那些不该出现的字段：异常长的 `Comment`、内容奇怪的 `Software`、描述里的 base64，或者和主图对不上的缩略图。

**别因为这一步看起来无聊就跳过。** 在真实题目和真实事件里，载荷往往就在元数据里 —— 恰恰因为所有人都直奔刺激的 LSB 工具去了。

#### 四、被改过的宽或高

**原理。** PNG 的 `IHDR` 块声明了尺寸，而 CRC 保护整个块。把高度改小，图片就渲染成被裁掉下半部分的版本，把画面下方藏起来。CRC 对不上了，但很多看图程序根本不校验。

**怎么找。** 校验 CRC：

```bash
pngcheck -v suspect.png
# 会出现类似：IHDR chunk, invalid CRC
```

**怎么修。** 从图像数据反推真实尺寸并修好文件头，或者直接爆破高度直到 CRC 对上。思路用 Python 写出来是这样：

```python
import struct, zlib, binascii

data = open('suspect.png', 'rb').read()
# IHDR 的数据从第 16 字节开始：宽(4) 高(4) 位深(1) 颜色类型(1) ...
w, h = struct.unpack('>II', data[16:24])
for test_h in range(1, 3000):
    chunk = data[12:16] + struct.pack('>II', w, test_h) + data[24:29]
    crc = binascii.crc32(chunk) & 0xffffffff
    if crc == struct.unpack('>I', data[29:33])[0]:
        print('真实高度是', test_h)
        break
```

这个循环就是全部诀窍，值得亲手敲一遍，让这个思路长在手上。

#### 五、索引色图里的调色板

**原理。** GIF 和一部分 PNG 不是每像素存颜色，而是存一张最多 256 色的**调色板**，每个像素只是它的下标。改动调色板的顺序、或者塞进几个根本用不到的颜色，你就改了文件而没改画面。

**怎么找。** 把调色板和重新保存的版本对比，或者直接看它：

```bash
zsteg suspect.gif            # zsteg 也处理 GIF 调色板
python3 -c "
from PIL import Image
img = Image.open('suspect.gif')
print(img.getpalette()[:30])   # 前几个颜色，按顺序
"
```

#### 六、文件里的第二张图

**原理。** 一个文件里两张图：看得见的这张，和藏在后面的真图。这类题很常见，就是让你注意到"这个文件比一张图所需的数据多"。

**怎么找。** 还是 `binwalk`，解出来再看它是什么。如果解出来是另一张图，打开它。

#### 七、alpha 通道与其他视觉手法

**原理。** 第四个通道（透明度）在视觉上经常被忽略 —— 完全透明的像素照样有颜色值。于是数据可以藏在"看不见的那些像素"的 RGB 里，也可以把画面调整一下让某样东西显形。

**怎么找。** 把每个通道单独看。`stegsolve` 是这类操作的经典工具：它能把图只显示红、只显示绿、只显示蓝、只显示 alpha，以及单独显示每个位平面，你逐页翻找形状。任何图像编辑器都能单独看通道，`stegsolve` 只是把翻页变快了。

```python
from PIL import Image
img = Image.open('suspect.png').convert('RGBA')
img.getchannel('A').show()     # 只看 alpha 通道
```

调整**对比度和亮度**也出奇地常奏效：藏起来的内容常常是一块低对比度的形状，拉一下色阶就一目了然。

#### 八、JPEG 的系数域隐写

**原理。** JPEG 存的是频率而不是像素，所以隐藏发生在系数里。`jsteg`、`outguess`、`F5` 这类工具做这件事，而且大多需要口令。

**怎么找。** 这类很难手工判断。先看文件是不是某个已知工具产出的：

```bash
steghide info suspect.jpg        # 会问你要口令
outguess -r suspect.jpg out.txt  # 如果用的是 outguess
```

没有口令、也不知道是哪个工具时 —— 到这里题目通常还会给别的线索。实际上大多数 CTF 图片题用的都是 PNG 或 BMP，正是因为 JPEG 太难。

#### 九、根本不是图片的文件

反过来的一种把戏：名字叫 `.png`，内容却是别的东西。**永远先看魔术字节。**

```bash
file suspect.png     # 是 "PNG image data" 还是 "Zip archive data"
```

### 你的工具链，以及什么时候用哪个

| 工具 | 用来做 | 怎么装 |
|---|---|---|
| `file`、`xxd`、`strings` | 面对任何东西的前三条命令 | 系统自带 |
| `exiftool` | 看全部元数据字段，包括缩略图 | 常见软件源 |
| `binwalk`、`foremost` | 找附加与内嵌数据 | 常见软件源 |
| `zsteg` | PNG/BMP 的 LSB 与调色板分析 | Ruby gem |
| `stegsolve` | 目视检查通道与位平面 | 一个 Java jar |
| `pngcheck` | 检查块结构与 CRC | 常见软件源 |
| `steghide`、`outguess` | 带口令的 JPEG 与 WAV | 常见软件源 |
| Python + Pillow | 题目需要十几行自己的逻辑时 | `pip install pillow` |

### 实际动手的顺序

别一上来就用刺激的工具。从最便宜做到最贵，出结果就停：

1. `file` —— 它是不是它自称的东西？
2. `strings -n 8` —— 有没有明摆着的内容？
3. `exiftool` —— 元数据。
4. `ls -l` 与预期体积对照 —— 文件是不是太大了？
5. `pngcheck -v`（PNG）—— 结构有没有错？
6. `binwalk` —— 后面有附加吗？
7. `zsteg -a`（PNG/BMP）—— LSB 与调色板。
8. `steghide info`（JPEG/WAV）—— 有口令保护的载荷吗？
9. 目视：通道、位平面、对比度。
10. 写脚本 —— 以上都不合适时，题目要的就是你自己的逻辑。

### 自己藏一次，就懂了

理解这件事最快的办法是自己造一个。做一张低位里带消息的图，再用工具找出来：

```python
from PIL import Image
img = Image.open('cover.png').convert('RGB')
pixels = list(img.getdata())
msg = 'flag{you_found_me}' + '\0'
bits = ''.join(f'{ord(c):08b}' for c in msg)

out = []
for i, (r, g, b) in enumerate(pixels):
    if i < len(bits):
        r = (r & 0xFE) | int(bits[i])   # 替换最低位
    out.append((r, g, b))
img.putdata(out)
img.save('hidden.png')
```

然后跑 `zsteg hidden.png`，看它把你自己的消息找出来。之后再遇到 LSB 题，你就不会懵了。

### 这些东西在 CTF 之外有什么用

把数据藏进图片不只是谜题：

- **借合法通道外泄。** 一个能往公开图床传图片的内部人员，可以把数据放在像素里搬出去，而整个过程看起来不像文件传输。
- **恶意载荷。** 一张图在元数据或附加数据里带着脚本或可执行文件，由加载器取回。这跟恶意文档是同一招，只是容器不同。
- **借图片做 C2。** 一个 beacon 从一个看起来正常的站点取回图片，从它的比特里读指令。
- **水印与追踪。** 正当用途：嵌一个标识，用来追查被泄露的副本。

### 检测与缓解

- **不要相信扩展名和 MIME 类型。** 看字节；头部写着 ZIP 的文件就是 ZIP，跟它叫什么无关。
- **上传时解析并重新编码。** 把图片解析一遍再写出去，这**一个动作**就同时毁掉附加数据、元数据和几乎全部 LSB 载荷。它也是你最想要的隐私控制。
- **盯体量，不只盯内容。** 隐蔽通道就是设计成看起来正常的，所以给每个用户的上传量做基线，变化了告警。
- **盯出站目标。** 往组织不使用的图床传图片，无论里面是什么都值得问一句。
- **在能做的位置检查结构异常**：CRC 不匹配、`IEND` 之后还有数据块、尺寸与数据量对不上 —— 这些都是便宜的信号。
- **把解码后的内容记档。** 如果载荷藏在元数据或附加的压缩包里，把它提取出来放进案卷 —— 它是证据，下一个分析的人需要它。
