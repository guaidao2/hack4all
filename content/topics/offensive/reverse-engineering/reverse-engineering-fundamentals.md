---
id: reverse-engineering-fundamentals
title_en: Reverse Engineering Fundamentals
title_zh: 逆向分析基础
summary_en: Reverse engineering is not reading assembly until something makes sense. It is deciding which question you need answered, then picking the cheapest tool that answers it — a string, a breakpoint, a decompiler, or an emulator.
summary_zh: 逆向不是"一直读汇编直到看懂"。它是先想清楚自己要回答哪个问题，然后用最便宜的工具去回答 —— 可能是一个字符串、一个断点、一个反编译器，或者一个模拟器。
tags: [reverse-engineering, disassembly, ghidra, frida, malware-analysis, ctf]
tools: [Ghidra, IDA, Binary Ninja, x64dbg, Frida, radare2, dnSpy, jadx, angr]
attck: [T1140, T1027]
platform: [any]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### Start by identifying the target, not by opening a disassembler

The first five minutes decide how expensive the rest is. What kind of artifact is this?

| Target | Cheapest approach |
|---|---|
| .NET, Java, Android | Decompile directly (`dnSpy`, `jadx`, CFR). Often you get near-source, and obfuscation is the only obstacle |
| Python bytecode | `pycdc`, `decompyle3`; check the version first, since the bytecode changes per release |
| Go, Rust | Symbols are usually present, so function names survive. Go binaries are large but readable |
| Stripped C/C++ | Decompiler (Ghidra) plus dynamic analysis; this is the expensive case |
| Packed or obfuscated | Unpack first (`upx -d`, memory dump), then start over |
| Firmware | `binwalk` to find filesystems, then identify the architecture of each binary |

```bash
file ./target
strings -n 8 ./target | head -50
readelf -h ./target          # Linux: architecture, entry point, PIE
rabin2 -I ./target           # rizin's summary, if you have it
```

`strings` is not a toy. Config paths, error messages, format strings, URLs and embedded keys are frequently just sitting there, and they also give you landmarks to search for in the decompiler.

### Static analysis: work from landmarks, not from the entry point

Reading a binary from `main` outward is only practical for small programs. The productive loop:

1. **Find landmarks.** Interesting strings, imported functions (`read`, `strcmp`, `memcpy`, `socket`, `CryptEncrypt`), and exported symbols.
2. **Follow references backwards.** In Ghidra, an xref from a string to the function that uses it skips most of the program.
3. **Rename as you go.** A function named `sub_401230` in a five-hundred-function binary is a maze; naming it `parse_header` makes the rest of the analysis possible.
4. **Look for the comparison.** Most checks reduce to a branch on a value you control. Find the branch, understand the condition, and you have the answer.
5. **Recognise standard algorithms by their constants.** AES has the S-box, MD5 and SHA have their initial values, RC4 has its two hundred and fifty-six byte permutation, CRC32 has its polynomial. A table of constants identifies the algorithm faster than reading the code.

### Dynamic analysis: cheaper than reading

Running the program and watching is usually faster than decompiling it:

- **Breakpoints on APIs**, not on addresses. Breaking on `strcmp` and reading both arguments answers "what should the password be" immediately.
- **String-triggered breakpoints** in x64dbg are the workhorse for Windows binaries.
- **Conditional breakpoints** narrow down a loop that runs a million times.
- **Tracing** with `strace`/`ltrace` shows which syscalls the binary makes, which reveals file access, network use and `ptrace` anti-debugging.
- **Frida** hooks functions at runtime, on desktop and mobile, without a debugger. For a mobile app it is the most productive tool by a wide margin: hook the function that returns "is this device rooted" and return `false`.

```javascript
// Frida: replace a function's return value
Java.perform(function () {
  var Check = Java.use('com.example.app.RootCheck');
  Check.isRooted.implementation = function () { return false; };
});
```

### Common tasks, and how each one actually goes

**Algorithm recovery.** Identify the algorithm from constants, extract the key and IV from their memory locations, then reimplement or call the original library. Decryption is often easier than reversing the math: find where the key is loaded and dump it.

**Bypassing a check.** Patch the jump, hook the return value, or find the check and satisfy it. Patching is fastest locally; hooking is better when the binary is signed or the check is repeated.

**Protocol reverse engineering.** Capture traffic, then find the serialisation code by searching for the magic bytes or the field order you observed. Once you find the parser, the message layout usually falls out of the structure offsets.

**Malware analysis.** Mostly static triage first (strings, imports, packer detection), then dynamic in an isolated environment, then detailed reversing of the interesting parts: persistence, C2 protocol, and the actions taken. The separate entry on malicious document analysis covers the office-file case.

**Unpacking.** Start with the obvious (`upx -d`), then for custom packers set a breakpoint on the point where the original entry point is reached and dump from memory. The memory image usually has the imports resolved, which is why dumping beats static unpacking.

### Deobfuscation, in practice

- **String encryption**: break where the decryption function returns, then dump the string. Doing it a hundred times by hand is what scripting the debugger is for.
- **Control-flow flattening**: focus on the state variable and the dispatcher. Symbolic execution (angr) can sometimes solve it directly when your goal is a specific input rather than full understanding.
- **Junk code and opaque predicates**: ignore them. Reversing only what matters to your question is the skill, not a shortcut.
- **Anti-debugging**: `ptrace(PTRACE_TRACEME)` on Linux, `IsDebuggerPresent` on Windows. Patch the call, hide the debugger, or use a tool that does it for you.

### A realistic mindset

The goal is almost never "understand this binary completely". It is one of:

- What input makes this function return true?
- Where is the key, and what is it?
- What does this sample do to the system, and how does it talk to its operator?
- Which code path is reachable from the input I control?

Answer the question, document the steps that got you there, and stop. Complete comprehension is a hobby, not a deliverable.

### Detection and mitigation, from the defending side

- **Stripping symbols and enabling optimisations** raises the cost a little. It is a speed bump, not a control.
- **Obfuscation and packing** mainly buy time and defeat automated analysis. A determined human analyst will still get there; the question is whether there is something worth hiding.
- **Anti-debugging and anti-tampering** are detectable in themselves — a process that checks for `ptrace`, or a client that fails integrity checks, is a signal in your own telemetry.
- **The most useful defensive use of RE is triage**: knowing which crash is exploitable, which update is a repackaged binary, and what a suspicious sample actually does. That is worth more day to day than any single unpacking trick.

<!-- lang:zh -->
### 先识别目标，而不是先打开反汇编器

头五分钟决定后面有多贵。这到底是什么东西？

| 目标 | 最省事的做法 |
|---|---|
| .NET、Java、Android | 直接反编译（`dnSpy`、`jadx`、CFR）。往往能拿到接近源码的东西，唯一障碍是混淆 |
| Python 字节码 | `pycdc`、`decompyle3`；先确认版本，因为字节码随版本变化 |
| Go、Rust | 符号通常还在，函数名保留。Go 二进制大但可读 |
| 去符号的 C/C++ | 反编译器（Ghidra）加动态分析，这是最贵的一类 |
| 加壳或混淆 | 先脱壳（`upx -d`、内存 dump），再从头开始 |
| 固件 | 用 `binwalk` 找出文件系统，再逐个识别二进制的架构 |

```bash
file ./target
strings -n 8 ./target | head -50
readelf -h ./target          # Linux：架构、入口点、是否 PIE
rabin2 -I ./target           # 有 rizin 的话，它能给一份摘要
```

`strings` 不是玩具。配置路径、错误信息、格式串、URL、内嵌密钥经常会直接躺在那里，而且它们还是你在反编译器里搜索时的路标。

### 静态分析：从路标出发，而不是从入口点出发

从 `main` 往外读，只对小程序可行。有效的循环是：

1. **找路标。** 有意思的字符串、导入的函数（`read`、`strcmp`、`memcpy`、`socket`、`CryptEncrypt`）、导出的符号。
2. **沿引用往回走。** 在 Ghidra 里，从一个字符串交叉引用到使用它的函数，能跳过大部分程序。
3. **随手改名。** 在一个五百个函数的二进制里，`sub_401230` 就是迷宫；把它命名成 `parse_header`，后续分析才有可能。
4. **找那个比较。** 大多数校验最终都归结为"对你可控的值做一次分支"。找到分支、搞懂条件，答案就有了。
5. **靠常量认出标准算法。** AES 有 S 盒，MD5 和 SHA 有初始值，RC4 有那个 256 字节的置换表，CRC32 有多项式。一张常量表比逐行读代码更快告诉你这是什么算法。

### 动态分析：比读代码便宜

跑起来看，通常比反编译快：

- **断在 API 上，而不是地址上。** 断在 `strcmp` 上读两个参数，"口令应该是什么"这个问题立刻就有答案。
- **字符串触发的断点**在 x64dbg 里是处理 Windows 二进制的主力手法。
- **条件断点**能收窄一个跑一百万次的循环。
- **追踪**用 `strace`/`ltrace`，看二进制做了哪些系统调用，这能暴露文件访问、网络使用以及 `ptrace` 反调试。
- **Frida** 能在运行时 hook 桌面和移动端的函数，不需要调试器。对移动应用它是效率高出一大截的工具：hook 那个返回"设备是否已 root"的函数，让它返回 `false`。

```javascript
// Frida：替换一个函数的返回值
Java.perform(function () {
  var Check = Java.use('com.example.app.RootCheck');
  Check.isRooted.implementation = function () { return false; };
});
```

### 常见任务，以及它们实际怎么走

**算法还原。** 靠常量认出算法，从它们所在的内存位置取出密钥与 IV，然后重新实现或直接调用原库。**解密往往比逆向数学更容易**：找到密钥在哪里被加载，dump 出来。

**绕过校验。** 改跳转、hook 返回值，或者找到校验并满足它。本地改指令最快；二进制有签名、或校验反复出现时，hook 更好。

**协议逆向。** 抓流量，然后按你观察到的魔数字节或字段顺序去搜序列化代码。找到解析器之后，报文结构通常就从结构体偏移里掉出来了。

**恶意样本分析。** 基本是先静态分诊（字符串、导入表、查壳），再在隔离环境里动态跑，最后对有意思的部分细逆：持久化、C2 协议、执行的动作。Office 文件那一种另有专篇。

**脱壳。** 先用最明显的办法（`upx -d`），遇到自定义壳就在"到达原始入口点"的位置下断点，然后从内存 dump。内存镜像里导入表通常已经解析好，这也是 dump 胜过静态脱壳的原因。

### 反混淆，实际怎么做

- **字符串加密**：断在解密函数返回处，把字符串 dump 出来。手工重复一百次这种事，正是脚本化调试器存在的意义。
- **控制流平坦化**：抓住那个状态变量和分发器。当你的目标只是"求一个特定输入"而不是完整理解时，符号执行（angr）有时能直接解掉。
- **垃圾代码与不透明谓词**：无视它们。**只逆和你问题相关的部分，这是技能，不是偷懒。**
- **反调试**：Linux 上是 `ptrace(PTRACE_TRACEME)`，Windows 上是 `IsDebuggerPresent`。改掉那个调用、把调试器藏起来，或者用现成工具处理。

### 一个务实的心态

目标几乎从来不是"完整理解这个二进制"，而是下面之一：

- 什么输入能让这个函数返回真？
- 密钥在哪，是什么？
- 这个样本对系统做了什么，怎么跟操控者通信？
- 在我可控的输入之下，哪条代码路径是可达的？

回答这个问题，把走到的步骤记录清楚，然后停。完整理解是爱好，不是交付物。

### 检测与缓解（防守方视角）

- **去符号、开优化**能稍微抬高成本。它是减速带，不是控制。
- **混淆与加壳**主要买时间、挡住自动化分析。铁了心的分析员照样能到，问题只在于"有没有值得藏的东西"。
- **反调试与防篡改本身是可检测的** —— 一个进程在检查 `ptrace`，或者一个客户端完整性校验失败，在你自己的遥测里就是信号。
- **逆向最有用的防守用途其实是分诊**：判断哪个崩溃可利用、哪个更新是重新打包的二进制、一个可疑样本到底干了什么。这些日常价值超过任何一个脱壳技巧。
