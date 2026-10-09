---
id: asm-architecture-differences
title_en: Architecture Differences
title_zh: 架构差异
summary_en: The same source compiled for two widths differs in four places at once — how wide an address is, where arguments go, how the instruction pointer is obtained, and which instruction talks to the kernel — and each one changes how a binary has to be read. Measured by compiling the same program both ways and disassembling both.
summary_zh: 同一份源码为两种位宽各编译一次，会在四个地方同时不同 —— 地址有多宽、参数去哪里、指令指针怎么取得、以及哪条指令跟内核说话 —— 而每一处都改变一个二进制该怎么读。这一篇把同一个程序两种编译、然后把两边都反汇编出来。
tags: [beginner, asm, architecture, x86, reverse-engineering]
tools: [gcc, objdump, file, readelf]
attck: [T1027]
platform: [linux]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Four differences at once

An architecture decides more than which instructions exist. **The same program compiled for two widths differs in how wide an address is, where arguments go, how the instruction pointer is obtained, and which instruction reaches the kernel** — and understanding a binary means knowing all four.

> **None of those differences is a detail of the instruction set. Each one changes what a frame looks like, what a payload has to contain, and what a system call does.**

Measured below by compiling one program for both widths and reading both.

### Part 1: what an architecture decides

| Question | Where the answer shows up |
|---|---|
| how wide is an address | every pointer, every offset, every saved value |
| where do arguments go | the arrangement of a stack frame |
| how is the instruction pointer obtained | whether position-independent code needs a helper |
| which instruction calls the kernel | what a system call looks like in a listing |
| are instructions fixed or variable length | how disassembly and alignment work at all |

**And these are independent axes.** Two architectures can agree on width and disagree on everything else, or agree on the calling convention and differ in how they reach the kernel. **So "which architecture" is not one question.**

**And the first axis is the one that is visible from the file itself.** Measured on the two builds of one source:

```
64-bit build:  ELF 64-bit LSB pie executable, x86-64
32-bit build:  ELF 32-bit LSB pie executable, Intel i386
```

**Which matters because everything after it depends on it** — an offset in a listing is a different number, a saved value occupies a different amount, and a payload written for one is not a payload for the other.

### Part 2: width, measured on one source

Measured, the same program compiled twice:

| | 64-bit | 32-bit |
|---|---|---|
| `sizeof(void *)` | **8** | **4** |
| `sizeof(long)` | **8** | **4** |
| `sizeof(int)` | 4 | 4 |

**And a pointer printed as an integer**:

| Build | Value |
|---|---|
| 64-bit | `0x7ffc311fb2c4` |
| 32-bit | `0xffb87cd4` |

> **A pointer is a different amount of memory in each build, so every structure that holds one, every saved frame pointer and every return address occupies a different amount.**

**Which is why "how wide is this binary" is a precondition rather than a piece of metadata**, and why a technique described without it is a technique described for one of the two.

### Part 3: where arguments go

This is the measurement with the most consequence for anything involving a stack. **Measured, the same six-argument call compiled both ways:**

**64-bit — the first six arguments go into registers:**

```
mov    r9d, 0x6
mov    r8d, 0x5
mov    ecx, 0x4
mov    edx, 0x3
mov    esi, 0x2
mov    edi, 0x1
call   ...
```

**32-bit — all six are pushed:**

```
push   0x6
push   0x5
push   0x4
push   0x3
push   0x2
push   0x1
call   ...
add    esp, 0x18
```

**And the callee reads them from correspondingly different places.** Measured, the 64-bit entry moves registers into its frame, **while the 32-bit entry reads from above the return address**:

```
mov    edx, DWORD PTR [ebp+0x8]
mov    eax, DWORD PTR [ebp+0xc]
```

> **In one width a frame contains saved registers and locals; in the other it also contains the arguments, immediately above the return address — so what is "past the end of a buffer" is a different thing in each.**

**Which is the concrete difference behind a technique that exists for one width and not the other.** An overflow that reaches the return address lands next to **arguments** in the 32-bit layout and next to **nothing of the kind** in the 64-bit one, **so the same bug offers different material to work with.**

**And the clean-up is part of the convention**: measured, the 32-bit caller adjusts the stack pointer by the amount it pushed, **while the 64-bit caller has nothing to adjust** — which is also why the alignment arithmetic in the stack entry differs between the two.

### Part 4: registers, by name

**The width is in the name**: a 64-bit register, its 32-bit half, its 16-bit half and its 8-bit half are all spelled differently, and the naming is regular enough to read at a glance.

**And the 64-bit set is larger**, because sixteen registers were available when the width grew. Measured, an argument arrives in a register that has no 32-bit equivalent at all:

```
mov    r9d, 0x6        <- a fifth argument, in a register the 32-bit build does not have
```

> **A register's name carries its width, so a listing in one width reads as the same program with different names — and that impression is wrong wherever the width changes behaviour, which the fundamentals entry measured for writes to a 32-bit name.**

**Which is the practical caution about reading two listings side by side**: the shapes match, **the sizes do not**, and the instructions that depend on the size are exactly the ones that matter.

### Part 5: obtaining the instruction pointer

**Position-independent code has to reference data relative to where it is, and the two widths do that differently.** Measured, the 32-bit listing contains a call followed by an addition:

```
call   <next instruction>
add    eax, 0x1
```

**That is a call to a helper whose only job is to put the instruction pointer into a register** — the relocation that names the helper is resolved at link time, which is why the listing shows the call going to the following address.

**And the 64-bit build does not need one.** Measured in the memory entry, it references data with an addressing form that is already relative to the instruction pointer:

```
lea    rax, [rip+0x7]
```

> **One width has an addressing mode that computes an address relative to the instruction pointer; the other does not, so it has to obtain the pointer first and add to it. The difference is not an optimisation — it is whether the hardware can express the reference at all.**

**Which is the same measurement as the memory entry's, seen from the other side**: there, the relative form was what made position independence possible; here, **an architecture without it needs a helper function in every function that touches global data.**

### Part 6: the instruction that calls the kernel

**Measured, on two static builds of one program:**

| Build | Instruction found in the listing |
|---|---|
| 32-bit | **`int $0x80`** |
| 64-bit | **`syscall`** |

**And those instructions do not merely look different.** They use different registers for the number, **a different set of registers for the arguments, and in places a different numbering of the same operation** — so the same source line becomes a different sequence in each build.

> **The same "write one byte" is a different instruction, with a different argument layout, in each width — which is why a payload written against one of them does not do the same thing in the other, even before anything else is considered.**

**Which is the mechanism behind a practice that looks like arbitrary convention**: code intended to run without a library is written per architecture **and per width**, because the interface to the kernel is part of the architecture rather than part of the operating system.

**And the tooling reflects it.** Measured, this environment's binutils reports the x86 family only, **and none of the common cross-compilers is installed**:

```
aarch64-linux-gnu-gcc      not present
riscv64-linux-gnu-gcc      not present
arm-linux-gnueabi-gcc      not present
```

**So the same bytes handed to a disassembler configured for the wrong architecture produce nothing or nonsense**, and analysing a binary of another architecture begins with having the tool that understands it.

### Part 7: fixed and variable length

**In a fixed-length architecture every instruction occupies the same amount of memory**, and the boundary of the next is a multiplication rather than a decoding. **In a variable-length one, the boundary is the result of decoding the current instruction**, which the fundamentals entry measured by reading one byte string from three offsets.

**The difference changes two whole procedures.** **Alignment** is a matter of instruction placement in one and a packing decision in the other. **And disassembly from an arbitrary offset** is meaningless in one and merely wrong in the other — meaningless because the offset is not a boundary that can exist, wrong because the bytes will decode into something.

**And the two axes are independent.** There are fixed-length architectures with an optional shorter encoding, so "fixed" describes the default rather than a guarantee, **and a listing has to be read with whichever of the two applies to the build in front of you.**

### Part 8: what follows for security

**A vulnerability is not the same object on two architectures.** Measured, the same call took six registers in one width and six pushes in the other; **the frame that an overflow reaches into therefore contains different things**, and a technique that works by controlling what sits beside the return address works differently, or not at all.

**And the position of a parameter is the clearest example.** Measured, the 32-bit callee read its arguments from above the return address, **so the region past a buffer is populated by the caller's arguments in one width and not in the other.**

**And obtaining the position is part of the code rather than of the payload.** Measured, the 32-bit build carried a helper call for it and the 64-bit build used an addressing mode. **So a rop chain or a shellcode stub that has to locate itself does it with different instructions, or does not have to do it at all.**

**And the kernel interface is per architecture and per width.** Measured, one build used a trap instruction and the other a dedicated one, **with different registers and a different call number** — which is the reason a payload is specified by architecture in the first place, and the reason a cross-architecture transplant is a rewrite.

**And the tooling is not interchangeable.** Measured, this environment's binutils handles one family and no cross-compilers were installed, **so the first step of any cross-architecture analysis is having a disassembler that knows the target** — and pointing the wrong one at the bytes produces a listing rather than an error.

### Detection and mitigation

- **Establish the architecture and the width from the file before reading anything else**, since the measured pointer and long sizes differ between the two builds of one source.
- **Reason about a stack layout separately per width**, because the measured 32-bit callee read arguments from above the return address where the 64-bit one read registers.
- **Check whether a binary is position independent and how it obtains the instruction pointer**, since the measured 32-bit build carried a helper call the 64-bit build did not need.
- **Record the width alongside any address, offset or payload**, because the same number is a different amount of memory in each.
- **Confirm the toolchain knows the target architecture before trusting an empty or odd listing**, since the measured binutils handled one family and a wrong architecture produces output rather than an error.
- **Distinguish fixed from variable length before reasoning about alignment or offsets**, because the boundary of an instruction is computed in one case and decoded in the other.
- **Specify shellcode by architecture and width both**, since the measured kernel interface differed in the instruction, the registers and the call number.
- **Do not transplant an exploitation technique between widths without redoing the layout**, because the measured difference is what occupies the space an overflow reaches.
- **Check whether an optional shorter encoding is in use** when a listing from a fixed-length architecture shows mixed instruction sizes, since the two axes are independent.
- **And read two listings of one source side by side only for shape**, because the measured shapes agree while the sizes are exactly where the behaviour differs.

<!-- lang:zh -->
### 四个地方同时不同

一个架构决定的不只是"有哪些指令"。**同一个程序为两种位宽各编译一次，会在地址有多宽、参数去哪里、指令指针怎么取得、以及哪条指令够到内核这四处不同** —— 而读懂一个二进制意味着这四处都知道。

> **那些差别没有一个是指令集的细节。每一处都改变一个栈帧长什么样、一个载荷必须包含什么、以及一次系统调用做什么。**

下面把同一个程序为两种位宽各编译一次来实测、两边都读。

### 第一部分：一个架构决定了什么

| 问题 | 答案在哪里显出来 |
|---|---|
| 一个地址有多宽 | 每一个指针、每一个偏移、每一个被保存的值 |
| 参数去哪里 | 一个栈帧的安排 |
| 指令指针怎么取得 | 位置无关代码需不需要一个帮手 |
| 哪条指令调用内核 | 一次系统调用在清单里长什么样 |
| 指令是定长还是变长 | 反汇编与对齐究竟怎么进行 |

**而这几条是彼此独立的轴。** 两个架构可以在位宽上一致而在别的每一处都不同，也可以在调用约定上一致而在"怎么够到内核"上不同。**所以"哪个架构"不是一个问题。**

**而第一条轴是从文件本身就能看出来的。** 对同一份源码的两份构建实测：

```
64 位构建:  ELF 64-bit LSB pie executable, x86-64
32 位构建:  ELF 32-bit LSB pie executable, Intel i386
```

**这要紧，因为之后的一切都依赖它** —— 一份清单里的偏移是另一个数字、一个被保存的值占另一份大小，而为其中一种写的载荷不是给另一种的。

### 第二部分：位宽，在同一份源码上实测

实测，同一个程序编译两次：

| | 64 位 | 32 位 |
|---|---|---|
| `sizeof(void *)` | **8** | **4** |
| `sizeof(long)` | **8** | **4** |
| `sizeof(int)` | 4 | 4 |

**以及把一个指针当整数打印出来**：

| 构建 | 值 |
|---|---|
| 64 位 | `0x7ffc311fb2c4` |
| 32 位 | `0xffb87cd4` |

> **一个指针在两种构建里是不同大小的内存，所以每一个装着它的结构、每一个被保存的帧指针、每一个返回地址，占的都是不同的大小。**

**这就是为什么"这个二进制是多少位"是一个前提、而不是一条元数据**，也是为什么一个没说明位宽的技术，是一个只对两者之一成立的技术。

### 第三部分：参数去哪里

这一组实测对任何涉及栈的事情后果最大。**实测，同一个六参数调用两种编译：**

**64 位 —— 前六个参数进寄存器：**

```
mov    r9d, 0x6
mov    r8d, 0x5
mov    ecx, 0x4
mov    edx, 0x3
mov    esi, 0x2
mov    edi, 0x1
call   ...
```

**32 位 —— 六个全部压栈：**

```
push   0x6
push   0x5
push   0x4
push   0x3
push   0x2
push   0x1
call   ...
add    esp, 0x18
```

**而被调用方从相应地不同的地方读它们。** 实测，64 位的入口把寄存器搬进自己的帧，**而 32 位的入口从返回地址之上读**：

```
mov    edx, DWORD PTR [ebp+0x8]
mov    eax, DWORD PTR [ebp+0xc]
```

> **在一种位宽里，一个帧装的是被保存的寄存器与局部变量；在另一种里它还有参数，就紧挨在返回地址之上 —— 所以"一块缓冲区末尾之后是什么"在两种里是不同的东西。**

**这就是"某个技术只为一种位宽存在"背后的具体差别。** 一次够到返回地址的溢出，在 32 位布局里落在**参数**旁边，而在 64 位布局里旁边**没有这类东西** —— **所以同一个 bug 提供的材料不一样。**

**而清理也是约定的一部分**：实测，32 位的调用方把栈指针调回它压进去的量，**而 64 位的调用方没有东西要调** —— 这也是为什么讲栈那一篇里那种对齐计算在两种位宽下不同。

### 第四部分：按名字认寄存器

**宽度就在名字里**：一个 64 位寄存器、它的 32 位一半、16 位一半与 8 位一半各有拼法，而命名规则一致到一眼就能读。

**而 64 位那一组更大**，因为位宽变大的时候多出了十六个寄存器可用。实测，一个参数到达的寄存器在 32 位里根本没有对应物：

```
mov    r9d, 0x6        <- 第五个参数，在一个 32 位构建没有的寄存器里
```

> **一个寄存器的名字带着它的宽度，所以一种位宽下的清单读起来像"同一个程序换了名字" —— 而在宽度改变行为的地方，那个印象是错的；基础篇为"写 32 位名字"实测过这一点。**

**这就是把两份清单并排读时的实际警告**：形状对得上，**大小对不上**，而依赖大小的那些指令恰好是要紧的那些。

### 第五部分：取得指令指针

**位置无关代码必须相对"它在哪"来引用数据，而两种位宽做这件事的方式不同。** 实测，32 位的清单里有一次调用后面跟着一次加法：

```
call   <下一条指令>
add    eax, 0x1
```

**那是一次对某个帮手的调用，而那个帮手唯一的工作就是把指令指针放进一个寄存器** —— 指明那个帮手的重定位在链接时才解析，这就是清单显示这次调用跳向紧随其后的地址的原因。

**而 64 位的构建不需要它。** 内存那一篇实测过，它用一种本来就相对于指令指针的寻址形式引用数据：

```
lea    rax, [rip+0x7]
```

> **一种位宽有一种"算出相对于指令指针的地址"的寻址方式；另一种没有，所以它必须先取得那个指针、再加上去。这个差别不是一次优化 —— 而是硬件到底能不能表达这种引用。**

**这与内存那一篇的实测是同一件事的另一面**：那里，相对形式是位置无关之所以可能的原因；这里，**一个没有它的架构，要在每一个碰全局数据的函数里都放进一个帮手函数。**

### 第六部分：调用内核的那条指令

**实测，同一个程序的两份静态构建：**

| 构建 | 清单里出现的指令 |
|---|---|
| 32 位 | **`int $0x80`** |
| 64 位 | **`syscall`** |

**而那两条指令不只是长得不同。** 它们用不同的寄存器放编号、**用不同的一组寄存器放参数、有些地方连同一个操作的编号都不同** —— 所以同一行源码在两种构建里变成不同的序列。

> **同一个"写一个字节"，在两种位宽里是不同的指令、不同的参数摆放 —— 这就是为什么一个照着其中一种写的载荷在另一种里不会做同一件事，而且在考虑别的东西之前就已经如此。**

**这就是一种看起来像任意约定的做法背后的机制**：打算不依赖库运行的代码，是按架构**并且按位宽**写的，因为通向内核的那个接口是架构的一部分、不是操作系统的一部分。

**而工具链也反映了这一点。** 实测，这个环境的 binutils 只报出 x86 家族，**而常见的交叉编译器一个都没装**：

```
aarch64-linux-gnu-gcc      没有
riscv64-linux-gnu-gcc      没有
arm-linux-gnueabi-gcc      没有
```

**所以同一串字节交给一个为错误架构配置的反汇编器，产出的是空白或者胡话**，而分析另一个架构的二进制，第一步是拿到那个懂它的工具。

### 第七部分：定长与变长

**在一个定长架构里，每条指令占同样多的内存**，而下一条的边界是一次乘法、不是一次解码。**在变长架构里，那个边界是解码当前这条指令的结果** —— 基础篇用"一串字节从三个偏移读"实测过这一点。

**这个差别改变了整整两套流程。** **对齐**在一种里是指令摆放的问题、在另一种里是一个打包决定。**而从一个任意偏移开始反汇编**在一种里没有意义、在另一种里只是错的 —— 没有意义，因为那个偏移不是一个能存在的边界；错，是因为那些字节会被解码成某个东西。

**而这两条轴彼此独立。** 存在带可选短编码的定长架构，所以"定长"描述的是默认值、不是一个保证，**而一份清单必须按"手上这个构建适用哪一条"来读。**

### 第八部分：从这些机制推出的安全观念

**一个漏洞在两个架构上不是同一个东西。** 实测，同一个调用在一种位宽里是六个寄存器、在另一种里是六次压栈；**于是一次溢出够到的那个帧里装着不同的东西**，而一个靠控制"返回地址旁边坐着什么"来工作的技术，做法不同、或者根本不成立。

**而参数的位置是最清楚的例子。** 实测，32 位的被调用方从返回地址之上读它的参数，**所以一块缓冲区之后那片区域，在一种位宽里会被调用方的参数填上、在另一种里不会。**

**而"取得自己的位置"是代码的一部分、不是载荷的一部分。** 实测，32 位构建为此带着一次帮手调用、而 64 位构建用的是寻址方式。**所以一条需要定位自己的链或者一段桩，用不同的指令做这件事、或者根本不必做。**

**而内核接口是按架构、也按位宽分的。** 实测，一份构建用一条陷阱指令、另一份用一条专用指令，**寄存器不同、调用编号也不同** —— 这就是为什么载荷首先要标明架构，也是为什么跨架构的移植是一次重写。

**而工具不可互换。** 实测，这个环境的 binutils 只处理一个家族、而交叉编译器都没装，**所以任何跨架构分析的第一步是有一个认识目标的反射器** —— 而把错误的那一个指向那些字节，产出的是清单、不是报错。

### 检测与缓解

- **在读别的之前先从文件确认架构与位宽**，因为实测同一份源码的两份构建里指针与 long 的大小不同。
- **按位宽分别推理栈布局**，因为实测 32 位的被调用方从返回地址之上读参数，而 64 位的读寄存器。
- **检查一个二进制是否位置无关、以及它怎么取得指令指针**，因为实测 32 位构建带着一次 64 位构建不需要的帮手调用。
- **把位宽与任何地址、偏移或载荷记在一起**，因为同一个数字在两者里是不同大小的内存。
- **在相信一份空白或者奇怪的清单之前，先确认工具链认识目标架构**，因为实测那套 binutils 只处理一个家族，而错误架构产出的是输出、不是报错。
- **在推理对齐或偏移之前先分清定长与变长**，因为一条指令的边界在一种情况下是算出来的、在另一种情况下是解码出来的。
- **载荷要同时标明架构与位宽**，因为实测那个内核接口在指令、寄存器与调用编号上都不同。
- **不要不加改动地把一个利用技术在两种位宽之间移植**，因为实测那个差别正是"一次溢出够到的空间里坐着什么"。
- **当一个定长架构的清单里出现混合的指令长度时，检查是不是用了可选的短编码**，因为那两条轴彼此独立。
- **并把同一份源码的两份清单只为"形状"而并排读**，因为实测那些形状一致，而大小恰好是行为不同的地方。
