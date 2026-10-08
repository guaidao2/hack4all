---
id: audio-steganography
title_en: Audio Steganography from Scratch
title_zh: 音频隐写从零开始
summary_en: Sound is a wave, and a computer stores it as a list of numbers — which means an audio file is data you can add to, hide inside, or simply look at in a different domain. This entry explains how audio is stored, then walks the hiding places, starting with the one that solves most audio challenges, the spectrogram.
summary_zh: 声音是波，而计算机把它存成一串数字 —— 也就是说，音频文件既可以往里加东西、也可以往里藏东西，还可以换一个域去看它。这一篇先讲音频是怎么存的，再逐个走藏匿点，而第一个就是能解决大多数音频题的东西：频谱图。
tags: [beginner, ctf, steganography, audio, wav, spectrogram, dtmf]
tools: [Audacity, Sonic Visualiser, sox, ffmpeg, exiftool, binwalk, multimon-ng, Python]
attck: [T1027.003, T1041]
platform: [any]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Why audio is different from images

An image is already a two-dimensional grid, so hiding in it means changing values that are already there. Audio is a **time series** — one number per instant — and that changes what you can do:

- You can hide data in the **values** (the same idea as LSB in an image).
- You can hide data in the **timing between samples**, which has no image equivalent.
- And, most importantly for a beginner, you can **look at it in a different domain**. A waveform shows amplitude over time. A spectrogram shows frequency over time — and human eyes are very good at spotting a word drawn in a spectrogram, which is why it is the first thing to check.

### Part 1: what audio is on disk

**Sound is a pressure wave.** A microphone converts that pressure into a varying voltage, and an analog-to-digital converter turns the voltage into numbers at regular intervals. Three parameters describe the result:

| Parameter | Meaning | Common values |
|---|---|---|
| **Sample rate** | How many numbers per second | 8000 (telephone), 44100 (CD), 48000 (video) |
| **Bit depth** | How precisely each number is stored | 8, 16 (usual), 24, 32 |
| **Channels** | How many streams | 1 (mono), 2 (stereo) |

So you can predict the size of a file. CD-quality audio, 16-bit stereo at 44100 Hz for ten seconds, is 44100 x 2 x 2 x 10 = 1.76 million bytes. **If a file is much larger than that, something extra is in it.**

**The formats differ in exactly the way image formats do:**

| Format | How the samples are stored | Consequences for steganography |
|---|---|---|
| **WAV** | Header, then raw sample values | Lossless, trivially editable. The right format for learning |
| **FLAC** | Lossless compression | Still exact, but you must decode before editing |
| **MP3 / AAC / OGG** | Lossy, using a model of what the ear ignores | Small, but re-encoding destroys small changes |

**WAV deserves detail** because it is the format most audio challenges use, and its structure is simple. A WAV file is a **RIFF** container made of chunks, the same idea as PNG:

```
"RIFF" [file size] "WAVE"
  "fmt " [size] [audio format][channels][sample rate][bit rate]...
  "data" [size] [sample values, one after another]
```

Two things follow, and they are what you will actually use:

- The `data` chunk declares its own length. **If the file is longer than the header says, there is something after the audio.**
- Extra chunks can be added, including text ones (`LIST`, `INFO`), which is a documented hiding place.

**MP3 hides in a different place**, because its audio is stored as compressed frequency data, not samples. It carries **ID3 tags** — artist, title, album art, and free-form comment fields. Those tags are where an MP3-based challenge usually hides something, and they are why `exiftool` is step two and not step nine.

**Why lossy formats break sample-level hiding.** MP3's encoder discards detail the ear will not notice, and the lowest bit of a sample is exactly that kind of detail. Write a message into those bits, encode to MP3, and the message is gone. The same rule as JPEG: LSB work needs a lossless container.

### Part 2: the hiding places, and how to find each one

#### 1. The spectrogram — check this first

**The idea.** Any signal can be described either by its waveform (amplitude over time) or by its frequency content over time. A spectrogram computes the second: it slides a small window along the audio, runs a Fourier transform on each slice, and draws frequency on one axis, time on the other, with brightness for energy.

The trick works because **hearing and seeing are different**: a tone at 18 kHz is nearly inaudible to an adult, but on a spectrogram it is a crisp line. So you can draw text, a QR code, or a picture into the high frequencies, and nobody notices by listening.

**Do it.** With Audacity, open the file and switch the track view to **Spectrogram** (the dropdown at the left of the track). With a command line, `sox` does it directly:

```bash
sox audio.wav -n spectrogram -o spec.png
# a taller, higher-resolution picture
sox audio.wav -n spectrogram -Y 800 -x 2000 -o big.png
ffmpeg -i audio.mp3 -lavfi showspectrumpic=s=1920x1080 spec.png
```

**Read it.** In the resulting image, look for shapes that are not natural sound: straight horizontal lines at odd frequencies, blocky patterns, text, a QR code. Natural audio looks like smoke; a message looks like a picture.

**When you see nothing, adjust.** This is where beginners give up too early:

- **Zoom into the frequency range.** Most viewers show 0 to 22 kHz. The message may be in the top few kHz, or below 500 Hz.
- **Change the window size.** A short window gives good time resolution and blurry frequencies; a long window does the opposite. Text with sharp edges wants a longer window.
- **Change the dynamic range.** Spectrogram viewers usually apply a contrast curve; increasing the gain can reveal faint content.
- **Look at each channel separately.** In stereo files, a message may be drawn in only one channel.

#### 2. LSB in the sample values

**The idea.** Identical to image LSB, with more data available: each 16-bit sample has 16 bits, and changing the lowest one alters the value by 1 in 32768. Nobody hears that.

**Do it.** There is no single standard tool, so write the ten lines. This reads the low bit of each sample into bytes:

```python
import wave
w = wave.open('audio.wav', 'rb')
frames = w.readframes(w.getnframes())
bits = ''
for i in range(0, len(frames), 2):        # 16-bit samples: two bytes each
    sample = int.from_bytes(frames[i:i+2], 'little', signed=True)
    bits += str(sample & 1)
data = bytes(int(bits[i:i+8], 2) for i in range(0, len(bits) - 7, 8))
print(data[:200])
```

As with images, if it comes out as garbage the variant is different: try the second-lowest bit, try reading left and right channels separately, or try the opposite bit order.

**Why this works in WAV and not MP3.** Because WAV stores the sample values you changed, and MP3 does not store samples at all.

#### 3. Data appended after the audio

**The idea.** Exactly the same as appending to a PNG: write the audio, then write something else after it. Players stop at the end of the declared `data` chunk.

**Find it.** The signal is a **mismatch between the declared sizes and the real file size**:

```bash
binwalk audio.wav                # lists what it finds
binwalk -e audio.wav             # and extracts it
xxd audio.wav | head -3          # RIFF size field, in bytes 4-7, little-endian
ls -l audio.wav                  # compare with the size the header claims
```

Reading the first bytes by hand is worth learning once: bytes 4 to 7 hold the RIFF size, little-endian, and it should be the file size minus 8. If the file is larger, something follows.

#### 4. Metadata

**The idea.** ID3 tags in MP3, `LIST`/`INFO` chunks in WAV. Free text fields, often used literally.

**Find it.**

```bash
exiftool audio.mp3
ffprobe -hide_banner audio.mp3
id3v2 -l audio.mp3                # list ID3 frames
```

Look for a long comment, an unusually large embedded image, or fields that make no sense for the track. `ffprobe` also shows the codec and duration, which is how you notice a file whose audio is shorter than its length.

#### 5. DTMF, Morse and other signalling tones

**The idea.** Some sounds *are* data in a documented code. Telephone keypads use **DTMF** — two simultaneous tones per digit, one from a low group and one from a high group — and Morse code is short and long tones.

**Find it.** Listen first; DTMF is unmistakable. Then decode automatically:

```bash
multimon-ng -a DTMF -t wav audio.wav
multimon-ng -a MORSE -t wav audio.wav
```

If the tooling is unavailable, the spectrogram shows it clearly: DTMF appears as two parallel short lines per digit, and you can map the frequency pairs by hand.

#### 6. Reversal, speed and channel tricks

**The idea.** Sometimes the audio simply contains speech that was disguised rather than hidden.

- **Reverse it.** `sox audio.wav reversed.wav reverse`, or Audacity's Effect menu. Backwards speech is a classic.
- **Change the speed.** Slowing a recording down reveals speech that was sped up; speeding it up reveals a low, slow message.
- **Subtract the channels.** If a message is in the difference between left and right, inverting one channel and mixing them cancels the shared audio and leaves the difference: `sox stereo.wav diff.wav remix 1v1,2v-1`.
- **Look at the very beginning and end.** Silence is cheap to hide in; a burst of data in the first 200 ms is easy to miss.

#### 7. Echo and phase hiding

**The idea.** Two techniques that survive compression better and are used in real watermarking:

- **Echo hiding** adds a quiet echo a few milliseconds after each sample, and encodes bits in the delay or the amplitude of that echo. The ear cannot resolve an echo that close, so it just sounds slightly different.
- **Phase coding** modifies the phase of the signal rather than its amplitude.

**Find them.** These are not visible on a spectrogram and not audible. They need signal analysis — autocorrelation for echo, cepstrum for both:

```python
import numpy as np, wave
w = wave.open('audio.wav', 'rb')
x = np.frombuffer(w.readframes(w.getnframes()), dtype=np.int16).astype(float)
ac = np.correlate(x, x, 'full')[len(x)-1:]
# a peak at a small lag is an echo at that delay, and its position encodes bits
print(np.argmax(ac[50:400]) + 50)
```

Knowing this exists is the point for a beginner: when the spectrogram is clean and LSB is empty, this is the direction to look.

#### 8. Files that are not audio at all

Always first, always cheap:

```bash
file mystery.wav     # "RIFF (little-endian) data, WAVE audio" or something else entirely
```

### Your toolchain

| Tool | Use it for |
|---|---|
| **Audacity** | The beginner's workbench: spectrogram view, reverse, speed, channel split, amplification |
| **Sonic Visualiser** | More serious spectral analysis, with plugins for many transforms |
| `sox` | The command-line Swiss army knife: convert, reverse, mix, generate a spectrogram |
| `ffmpeg` / `ffprobe` | Convert anything; `ffprobe` reports stream details |
| `exiftool`, `id3v2` | Metadata, including ID3 frames |
| `binwalk` | Appended and embedded data |
| `multimon-ng` | Decoding DTMF, Morse and pager signals |
| Python (`wave`, `numpy`) | LSB extraction, autocorrelation, anything custom |

### The order to actually work in

1. `file` — is it what it claims?
2. `exiftool` — tags and comments.
3. **Spectrogram** — the single highest-yield step for audio, and cheap.
4. `ls -l` against the expected size — is the file bigger than its audio?
5. `binwalk` — appended data.
6. `strings` — obviously readable content.
7. **Listen to it.** Really: play the file. Reverse it, slow it down, listen to each channel.
8. LSB extraction with a short script.
9. `steghide info` — a passphrase-protected payload?
10. Autocorrelation and cepstrum — echo or phase hiding.

### Practise by hiding something yourself

Draw a message into a spectrogram, then find it. `sox` can generate tones, and doing this once makes the technique obvious forever:

```python
import numpy as np, wave

rate = 44100
duration = 2.0
t = np.linspace(0, duration, int(rate * duration), endpoint=False)
base = 0.02 * np.sin(2 * np.pi * 440 * t)          # a quiet audible tone

# draw a "flag" of short high-frequency bursts above the audible range
mark = np.zeros_like(t)

def burst(start, length, freq, amp=0.05):
    seg = (t >= start) & (t < start + length)
    mark[seg] += amp * np.sin(2 * np.pi * freq * t[seg])

for i in range(10):
    burst(0.1 + i * 0.15, 0.08, 17000 + i * 200)   # a rising staircase near 17-19 kHz

samples = np.clip((base + mark) * 32767, -32768, 32767).astype(np.int16)
w = wave.open('hidden.wav', 'wb')
w.setnchannels(1); w.setsampwidth(2); w.setframerate(rate)
w.writeframes(samples.tobytes()); w.close()
```

Now open `hidden.wav` in Audacity's spectrogram view: you will see your staircase. Most people cannot hear it at all.

### A worked example

A challenge file `morse.wav` is 3 seconds of beeps.

1. `file morse.wav` — a real WAV, mono, 44100 Hz.
2. `exiftool morse.wav` — nothing.
3. Spectrogram in Audacity — a clean single tone switching on and off in patterns. That is Morse.
4. Measure or decode: `multimon-ng -a MORSE -t wav morse.wav` prints the letters.
5. The decoded text is a flag.

Notice the shape of the solution: no cryptography, no clever tool. Look at the file in the right domain, and read what is there.

### Where this matters outside CTF

- **Exfiltration through audio.** An insider can carry data out inside a podcast episode or a voice recording, and the file looks like media.
- **Covert channels for C2.** Malware has used audio-related interfaces to move data, including in air-gapped environments through speakers and microphones — slow, but it crosses a gap that has no network.
- **Watermarking.** Streaming services and stock libraries embed inaudible identifiers to trace a leaked copy. This is the legitimate use of the same science.
- **Acoustic side channels.** Keystroke sounds, printer noise and even disk activity have been used to recover information. It is a reminder that a physically present recorder is an input device.

### Detection and mitigation

- **Re-encode what leaves.** Transcoding audio to a lossy format destroys sample-level hiding and most watermarking, at the cost of quality. If audio is a business artefact, this is the same control as stripping image metadata.
- **Inspect outbound media at the gateway where policy allows**, and treat an unexplained audio upload as a question worth asking regardless of what is inside.
- **Watch for signalling tones in voice channels.** DTMF appearing inside a customer call, or a recording containing data bursts, is a documented fraud pattern — the tone is how an attacker walks an IVR.
- **Baseline transfer volumes per user.** A covert channel is designed to look normal, so the anomaly is the volume, not the content.
- **Keep a decoded copy during an investigation.** Extracted audio payloads, metadata and wavelet content are evidence; the next analyst will need them.
- **For high-assurance environments**, remember that microphones and speakers are network interfaces to a determined attacker, and treat them as part of the attack surface.

<!-- lang:zh -->
### 音频和图片有什么不一样

图片本来就是一个二维格子，所以往里藏东西就是改那些已经存在的值。音频是一条**时间序列** —— 每一瞬间一个数 —— 所以能做的事不一样：

- 你可以把数据藏在**数值**里（和图片 LSB 同一个思路）。
- 你可以把数据藏在**采样点之间的时序**里，这在图片里没有对应物。
- 而对新手最重要的是，你可以**换一个域去看它**。波形图显示的是"幅度随时间"，频谱图显示的是"频率随时间" —— 而人眼非常擅长在频谱图里认出一个词，这就是为什么它是第一个该看的东西。

### 第一部分：音频在磁盘上是什么

**声音是压力波。** 麦克风把压力变成变化的电压，模数转换器再以固定间隔把电压变成数字。描述结果需要三个参数：

| 参数 | 含义 | 常见取值 |
|---|---|---|
| **采样率** | 每秒取多少个数 | 8000（电话）、44100（CD）、48000（视频） |
| **位深** | 每个数存得多精确 | 8、16（最常见）、24、32 |
| **声道数** | 有几路 | 1（单声道）、2（立体声） |

于是你可以预估文件大小。CD 音质、16 位立体声、44100 Hz、十秒钟，就是 44100 × 2 × 2 × 10 = 176 万字节。**如果一个文件比这大出很多，里面就有别的东西。**

**各种格式的差别，和图片格式的差别一模一样：**

| 格式 | 采样点怎么存 | 对隐写的后果 |
|---|---|---|
| **WAV** | 头部 + 原始采样值 | 无损、随便改，最适合学原理 |
| **FLAC** | 无损压缩 | 仍然精确，但要先解码才能改 |
| **MP3 / AAC / OGG** | 有损，基于"耳朵听不见什么"的模型 | 文件小，但重新编码会毁掉微小改动 |

**WAV 值得细讲**，因为多数音频题都用它，而它的结构很简单。一个 WAV 是 **RIFF** 容器，由数据块组成，和 PNG 是同一个思路：

```
"RIFF" [文件大小] "WAVE"
  "fmt " [长度] [格式][声道数][采样率][码率]...
  "data" [长度] [采样值，一个接一个]
```

由此得到两件你实际会用到的事：

- `data` 块**自己声明了长度**。**如果文件比头部说的更长，音频后面就有东西。**
- 可以插入额外的块，包括文本块（`LIST`、`INFO`），这是一个文档化的藏匿点。

**MP3 藏在别的地方**，因为它的音频存的是压缩后的频率数据，不是采样点。它带 **ID3 标签** —— 艺术家、标题、专辑封面，以及自由填写的注释字段。基于 MP3 的题目通常就把东西藏在那里，这也是为什么 `exiftool` 是第二步而不是第九步。

**有损格式为什么会毁掉采样级隐藏。** MP3 编码器丢掉的正是耳朵不会注意的细节，而采样点的最低位恰好就是这类细节。把消息写进那些位、再编码成 MP3，消息就没了。和 JPEG 同一条规矩：LSB 类手法必须配无损容器。

### 第二部分：藏匿点，以及怎么一个个找出来

#### 一、频谱图 —— 第一个就该查这个

**原理。** 任何信号都可以用两种方式描述：波形（幅度随时间）或它的频率成分随时间的变化。频谱图算的是后者：沿着音频滑动一个小窗口，对每一小段做傅里叶变换，然后把频率画在一个轴、时间画在另一个轴、用亮度表示能量。

这个手法能成立，是因为**听和看是两回事**：18 kHz 的音成年人几乎听不见，但在频谱图里它是一条清晰的线。于是你可以把文字、二维码或一张图画进高频里，而没有人靠听能发现。

**动手。** 用 Audacity 打开文件，把音轨视图切成 **Spectrogram**（音轨左侧的下拉框）。用命令行的话，`sox` 直接出图：

```bash
sox audio.wav -n spectrogram -o spec.png
# 更高、更精细的图
sox audio.wav -n spectrogram -Y 800 -x 2000 -o big.png
ffmpeg -i audio.mp3 -lavfi showspectrumpic=s=1920x1080 spec.png
```

**怎么看。** 在图里找不像自然声音的形状：奇怪频率上的水平直线、成块的花纹、文字、二维码。自然音频看起来像烟雾，而消息看起来像一张图。

**看不到东西时要调参数** —— 新手就是在这里过早放弃的：

- **放大频率范围。** 多数看图工具默认显示 0 到 22 kHz。消息可能在最上面几 kHz，也可能在 500 Hz 以下。
- **改窗口大小。** 窗口短，时间分辨率好、频率糊；窗口长则相反。边缘锐利的文字要用长一点的窗口。
- **改动态范围。** 频谱查看器通常带一条对比度曲线，把增益调高常能让很淡的内容显出来。
- **左右声道分开看。** 立体声文件里，消息可能只画在其中一个声道。

#### 二、采样值里的 LSB

**原理。** 和图片 LSB 完全相同，但可用数据更多：每个 16 位采样点有 16 位，改最低位只让数值变了 32768 分之 1。没人听得出来。

**动手。** 这件事没有唯一的标准工具，所以自己写那十行。下面这段把每个采样点的最低位读成字节：

```python
import wave
w = wave.open('audio.wav', 'rb')
frames = w.readframes(w.getnframes())
bits = ''
for i in range(0, len(frames), 2):        # 16 位采样：每个两字节
    sample = int.from_bytes(frames[i:i+2], 'little', signed=True)
    bits += str(sample & 1)
data = bytes(int(bits[i:i+8], 2) for i in range(0, len(bits) - 7, 8))
print(data[:200])
```

和图片一样，出来是乱码就说明变体不同：试第二低位、把左右声道分开读、或者反过来取位序。

**为什么 WAV 行得通而 MP3 不行。** 因为 WAV 存的就是你改过的采样值，而 MP3 根本不存采样值。

#### 三、附加在音频后面的数据

**原理。** 和往 PNG 后面追加完全一样：写完音频，再往后写别的东西。播放器会在 `data` 块声明的位置停下。

**怎么找。** 信号是"**声明的长度和真实文件大小对不上**"：

```bash
binwalk audio.wav                # 列出它发现的东西
binwalk -e audio.wav             # 并解出来
xxd audio.wav | head -3          # RIFF 长度字段在第 4-7 字节，小端
ls -l audio.wav                  # 与头部声明的长度对比
```

手工读一次头几个字节值得学：第 4 到 7 字节是 RIFF 长度，小端表示，它应该等于文件大小减 8。如果文件更大，后面就跟着东西。

#### 四、元数据

**原理。** MP3 的 ID3 标签、WAV 的 `LIST`/`INFO` 块。自由文本字段，经常被直白地拿来用。

**怎么找。**

```bash
exiftool audio.mp3
ffprobe -hide_banner audio.mp3
id3v2 -l audio.mp3                # 列出 ID3 帧
```

找异常长的注释、大得离谱的内嵌图片，或者跟这首曲子完全不搭的字段。`ffprobe` 还会给出编码和时长 —— 这是你发现"音频比文件本身短"的方式。

#### 五、DTMF、摩斯以及其他信号音

**原理。** 有些声音**本身就是**编码好的数据。电话键盘用的是 **DTMF** —— 每个数字两个同时发出的音，一个取自低频组、一个取自高频组；摩斯电码则是长短音。

**怎么找。** 先听，DTMF 一听就认得。然后自动解码：

```bash
multimon-ng -a DTMF -t wav audio.wav
multimon-ng -a MORSE -t wav audio.wav
```

手边没有工具时，频谱图会把它暴露得很清楚：DTMF 表现为每个数字两条平行的短横线，你可以手工把频率对映射成数字。

#### 六、倒放、变速与声道把戏

**原理。** 有时候音频里装的不是"藏起来"的话，而是"伪装过"的话。

- **倒放。** `sox audio.wav reversed.wav reverse`，或用 Audacity 的效果菜单。倒着说的话是经典题型。
- **改变速度。** 把录音放慢能听出被加速的话，加速则能听出低沉缓慢的消息。
- **把两个声道相减。** 如果消息在左右声道的差里，把其中一个反相再混合，共有的音频会被抵消，只剩差异：`sox stereo.wav diff.wav remix 1v1,2v-1`。
- **看开头和结尾。** 静音段最便宜；头 200 毫秒里的一小段数据很容易被漏掉。

#### 七、回声与相位隐藏

**原理。** 这两种手法更能扛压缩，也被真实的数字水印使用：

- **回声隐藏**在每个采样点之后几毫秒处加一个很轻的回声，把比特编码在回声的延迟或幅度里。这么近的回声人耳分辨不出来，只会觉得音色略有不同。
- **相位编码**改的是信号的相位，而不是幅度。

**怎么找。** 这两种在频谱图上看不见、也听不出来，需要信号分析 —— 回声用自相关，两者都可以用倒谱：

```python
import numpy as np, wave
w = wave.open('audio.wav', 'rb')
x = np.frombuffer(w.readframes(w.getnframes()), dtype=np.int16).astype(float)
ac = np.correlate(x, x, 'full')[len(x)-1:]
# 很小延迟处出现峰，说明那里有一个回声，而它的位置编码了比特
print(np.argmax(ac[50:400]) + 50)
```

对新手来说，知道它存在就够了：当频谱图很干净、LSB 也是空的时候，方向就在这儿。

#### 八、根本不是音频的文件

永远第一步，永远便宜：

```bash
file mystery.wav     # "RIFF (little-endian) data, WAVE audio"，或者完全别的东西
```

### 你的工具链

| 工具 | 用来做 |
|---|---|
| **Audacity** | 新手的工作台：频谱视图、倒放、变速、声道拆分、增益 |
| **Sonic Visualiser** | 更专业的频谱分析，插件可以做好多种变换 |
| `sox` | 命令行瑞士军刀：转换、倒放、混音、生成频谱图 |
| `ffmpeg` / `ffprobe` | 任何格式互转；`ffprobe` 报告流的细节 |
| `exiftool`、`id3v2` | 元数据，包括 ID3 帧 |
| `binwalk` | 附加与内嵌的数据 |
| `multimon-ng` | 解码 DTMF、摩斯、寻呼机信号 |
| Python（`wave`、`numpy`） | LSB 提取、自相关，以及任何自定义逻辑 |

### 实际动手的顺序

1. `file` —— 它是不是它自称的东西？
2. `exiftool` —— 标签与注释。
3. **频谱图** —— 音频里收益最高的单步，而且很便宜。
4. `ls -l` 与预期大小对比 —— 文件是不是比它的音频更大？
5. `binwalk` —— 附加数据。
6. `strings` —— 明显可读的内容。
7. **听一遍。** 真的去听：播放它，倒放、放慢、两个声道分别听。
8. 用一小段脚本做 LSB 提取。
9. `steghide info` —— 有口令保护的载荷吗？
10. 自相关与倒谱 —— 回声或相位隐藏。

### 自己藏一次，就懂了

把一条消息画进频谱图，再把它找出来。`sox` 能生成音调，做过一次之后这个手法就永远清楚了：

```python
import numpy as np, wave

rate = 44100
duration = 2.0
t = np.linspace(0, duration, int(rate * duration), endpoint=False)
base = 0.02 * np.sin(2 * np.pi * 440 * t)          # 一个很轻的、能听见的音

mark = np.zeros_like(t)

def burst(start, length, freq, amp=0.05):
    seg = (t >= start) & (t < start + length)
    mark[seg] += amp * np.sin(2 * np.pi * freq * t[seg])

for i in range(10):
    burst(0.1 + i * 0.15, 0.08, 17000 + i * 200)   # 17-19 kHz 附近一段上升的阶梯

samples = np.clip((base + mark) * 32767, -32768, 32767).astype(np.int16)
w = wave.open('hidden.wav', 'wb')
w.setnchannels(1); w.setsampwidth(2); w.setframerate(rate)
w.writeframes(samples.tobytes()); w.close()
```

现在用 Audacity 的频谱视图打开 `hidden.wav`，你会看到自己画的阶梯。而大多数人根本听不见它。

### 一个完整的小例子

题目给了一个 `morse.wav`，三秒的滴滴声。

1. `file morse.wav` —— 真 WAV，单声道，44100 Hz。
2. `exiftool morse.wav` —— 什么都没有。
3. Audacity 里看频谱 —— 一条干净的单音在按规律通断。这是摩斯。
4. 解它：`multimon-ng -a MORSE -t wav morse.wav` 直接打印出字母。
5. 解出来的文本就是 flag。

注意这个解法长什么样：没有密码学、没有花哨的工具。**换到正确的域去看这个文件，然后读那里写着的东西。**

### 这些东西在 CTF 之外有什么用

- **借音频外泄。** 内部人员可以把数据装在一集播客或一段录音里带出去，而文件看起来就是媒体。
- **用作 C2 的隐蔽通道。** 恶意软件用过与音频相关的接口搬数据，包括在物理隔离环境里通过扬声器和麦克风 —— 很慢，但它跨过了一道没有网络的鸿沟。
- **数字水印。** 流媒体和素材库会嵌入听不见的标识，用来追查被泄露的副本。这是同一套科学的正当用途。
- **声学侧信道。** 键盘声、打印机噪音甚至磁盘活动都被用来还原信息。它提醒你：一个物理上存在的录音设备，就是一个输入设备。

### 检测与缓解

- **出网的音频重新编码。** 转成有损格式会毁掉采样级隐藏和大多数水印，代价是音质。如果音频是业务产物，这跟"剥掉图片元数据"是同一种控制。
- **在策略允许的位置检查出站媒体**，并且对"无法解释的音频上传"问一句，不管里面是什么。
- **注意语音通道里的信号音。** 客户通话里出现 DTMF，或者一段录音里有数据突发，是有记录的欺诈模式 —— 攻击者就是靠这些音走完 IVR 的。
- **给每个用户的传输量做基线。** 隐蔽通道就是设计成看起来正常的，所以异常在体量上，不在内容上。
- **调查期间保留解码后的副本。** 提取出来的音频载荷、元数据、频谱内容都是证据，下一个分析的人需要它们。
- **在高保障环境里要记住**：对铁了心的攻击者来说，麦克风和扬声器就是网络接口，属于攻击面的一部分。
