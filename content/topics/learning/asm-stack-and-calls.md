---
id: asm-stack-and-calls
title_en: The Stack and Function Calls
title_zh: 栈与函数调用
summary_en: A calling convention answers four questions, and the stack is where the answers live — measured by calling into hand-written assembly to see exactly eight bytes appear, by reading a frame to find the caller's frame pointer and the return address, and by watching an overlong write reach the point where control leaves the program's own code.
summary_zh: 一个调用约定回答四个问题，而栈就是那些答案住的地方 —— 这一篇用一段手写汇编量出"调用一次正好多出八个字节"，读一个栈帧找出调用方的帧指针与返回地址，并看着一次过长的写入越过那个让控制流离开程序自身代码的界限。
tags: [beginner, asm, x86-64, stack, calling-convention]
tools: [nasm, gcc, objdump, gdb]
attck: [T1027]
platform: [linux]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Four questions, and one area that answers all of them

A call has to hand over control, hand over arguments, receive a result, and come back. **A calling convention is the agreement about how, and the stack is where most of it happens** — including the one slot that decides where control goes afterwards.

> **A call leaves exactly one thing on the stack: the address to return to. Anything that can rewrite those eight bytes decides where the program goes next.**

Measured below by calling into hand-written assembly and by reading a real frame.

### Part 1: why there is a stack at all

**Registers are a fixed, small set**, and a function needs somewhere to keep its own values while it calls another function — which is the situation recursion forces on every design.

**So there is an area of memory that is allocated per call and released on return.** It grows downward on this architecture, a register holds the current boundary, and the arrangement is what makes a function's state private to that call.

**And there is exactly one piece of that state that is not the function's own business**: the address to come back to, which the call instruction puts there and the return instruction takes out. **Everything in this entry that matters for security is about that one slot and its neighbourhood.**

### Part 2: what a convention has to decide

| Question | This platform's answer |
|---|---|
| where do arguments go | **the first six in registers, the rest on the stack** |
| where does the result go | registers |
| which registers survive a call | **some, and the callee has to restore them** |
| who cleans the stack | divided, by whether the caller or callee owns the space |

**And the third row is the one that lets separately compiled code work together at all**: if a function could freely destroy any register, no caller could keep anything in one across a call. **The convention splits the registers into those a caller must assume are gone and those a callee must give back.**

**And the first row is the one that makes reading an unfamiliar function a counting exercise.** Several arguments arrive in named registers, the rest in memory above the return address — **so the position of an argument on the stack depends on which argument it is.**

### Part 3: what one call leaves behind

Measured, with a hand-written function that reports the stack pointer it sees on entry:

| Where the stack pointer is read | Value |
|---|---|
| in the caller, before the call | `0x00007ffffde89b30` |
| at the first instruction of the callee | `0x00007ffffde89b28` |
| **the difference** | **8 bytes** |

**And the value at the callee's stack pointer is the return address** — an address in the code segment, which is what the return instruction will load into the instruction pointer.

> **A call pushes exactly one thing: the address to come back to. That is the only trace a call leaves on the stack, and it is the slot the rest of this entry is about.**

**Which is why the phrase "the stack" is misleading in a security context.** The interesting part is not the area; it is **eight bytes at a known offset from the frame pointer**, which happen to be an address that a later instruction will jump to.

### Part 4: arguments, in registers and then not

Measured, the instructions a compiler emitted to call a seven-argument function:

```
push   0x7
mov    r9d, 0x6
mov    r8d, 0x5
mov    ecx, 0x4
mov    edx, 0x3
mov    esi, 0x2
mov    edi, 0x1
call   ...
add    rsp, 0x8
```

**Reading it in order: the seventh argument — the one that does not fit — is pushed first, then the first six are placed in their registers, then the call, then the stack is adjusted back.**

**And the function being called stores them all into its own frame on entry**, six from registers and the seventh from above the return address.

> **An argument's location depends on which argument it is: the first six are in registers and the rest are in memory above the return address. So reading an unfamiliar function's entry means counting its arguments first.**

**And the push before the call has a second effect worth noting**: it moves the stack pointer by eight, **which is half of the alignment the next part is about** — so an odd number of stacked arguments and a misaligned call are the same problem seen twice.

### Part 5: which registers survive

Measured, two functions doing the same arithmetic, one using registers the callee must preserve and one using registers it need not:

| Function uses | What the compiler emitted |
|---|---|
| two callee-saved registers | `push r12` … `push rbx` at the top, `pop rbx` … `pop r12` before returning |
| two caller-saved registers | **nothing** — used directly, restored by nobody |

**The second function's caller cannot rely on those registers holding anything after the call**, and it does not: the convention says so, and the compiler has arranged its own code accordingly.

> **The convention divides the registers into those a caller must assume are gone and those a callee promises to give back — and that division is the whole basis on which separately compiled code can call itself.**

**And the security consequence points the other way from most of this series.** A corrupted stack is usually discussed as a way to control the return address, **but a corrupted callee-saved register is a way to go wrong far away from where the corruption happened** — in the caller, after the function returned, with nothing left on the stack to explain it.

### Part 6: the frame, and which direction the return address is

Measured, inside a function with a 32-byte buffer, walking outward from the buffer:

| Offset from the buffer | Value | What it is |
|---|---|---|
| +0 to +24 | `0xa7a6a5a4a3a2a1a0` … | the bytes the program wrote |
| **+96** | `0x00007ffcf1065fa0` | **`[rbp]` — the caller's frame pointer** |
| **+104** | `0x000055ef3f9a1327` | **`[rbp+8]` — the return address, in the code segment** |

**And the measurement confirms the slot by comparison**: the value at `[rbp]` equals the caller's frame pointer exactly, which is what "saved frame pointer" means — **it is the previous function's, not this one's.**

> **Local buffers live below the frame pointer, and the return address lives eight bytes above it. So an overlong write into a local buffer travels toward the return address.**

**And how far it has to travel is not a property of the source.** Measured across builds of the same program:

| Build | Distance from the buffer to the return address |
|---|---|
| one arrangement of locals | **56 bytes** |
| another | **72 bytes** |
| one keeping the frame pointer and an offset live across a loop | **104 bytes** |
| the same source with optimisation | **40 bytes** |

**Four numbers for the same source.** The distance is decided by what the compiler chose to keep in the frame between the buffer and the frame pointer.

> **The offset from a buffer to the return address is a property of one build, not of the program. It is discovered by reading or measuring, never assumed.**

**And the consequence is a curve rather than a boundary.** Measured, writing increasing amounts into a 16-byte buffer:

| Bytes written | Result |
|---|---|
| 16, 20, 24, 28, 32, 36, 40 | **the program exits normally** |
| **48, 64, 128** | **terminated by a signal** |

**Which is the two halves of the story in one table**: past the buffer there is other frame content that can be overwritten harmlessly, **then the return address, after which the return instruction jumps to something that is not a return address.**

### Part 7: two agreements that are not in the instructions

**The first is alignment.** Measured, calling a function with the stack pointer deliberately off by eight bytes:

| Call | Result |
|---|---|
| with the stack pointer off by 8 | **the program receives a segmentation fault** |
| with it as the convention requires | returns normally |

**And the instruction responsible is visible in the disassembly**: the function moves a 16-byte value through a register with an instruction that **requires a 16-byte aligned address**.

> **The stack must be aligned at the point of a call, and violating it does not mean "slower" — it means a crash that appears only in the functions that use an alignment-sensitive instruction.**

**Which is why hand-written assembly that constructs a call has to do the arithmetic**, and why the measured push of a stacked argument matters twice: **it changes the alignment of everything the call does.**

**And the second agreement is the space below the stack pointer**, which a leaf function may use freely. Measured, a function that writes to an address below the stack pointer **without adjusting it at all**:

```
mov    QWORD PTR [rsp-0x28], rdi
```

**That is the compiler using the reserved area**, and the same instruction appears in the hand-written version.

> **A function that calls nothing may use the bytes just below the stack pointer without claiming them — which is why a function's stack usage cannot be read off its own prologue alone.**

**And the exception is what makes it safe**: the area is reserved for the running function, and anything that could arrive asynchronously and use the stack would break the assumption — **which is exactly why the convention reserves it only for functions that do not call anything.**

### Part 8: what follows for security

**The return address is the only piece of the stack that decides where control goes**, and it sits at a fixed offset above the frame pointer. **Everything about stack corruption follows from that one sentence**, and the measurements give the two numbers that make it concrete: **a call moves the stack pointer by exactly eight bytes**, and the return address is at **frame pointer plus eight**.

**And the distance from a buffer to that slot is not a constant.** Four builds of one program gave 56, 72, 104 and 40 bytes, **and a write of forty bytes into a sixteen-byte buffer was survivable while forty-eight was not.** So the offset is a fact about a binary to be established, **and a payload written against an assumed constant is a payload that works on one build.**

**And the register half of the convention is a second, quieter way to go wrong.** Measured, a callee restores the registers it is obliged to restore and leaves the others alone, **so a corrupted value in a callee-saved register surfaces in the caller after the function has returned** — with the evidence pointing at the wrong place.

**And the alignment rule is a constraint on anything that constructs a call.** Measured, a call with the stack pointer off by eight bytes crashed in a function that used an alignment-sensitive instruction, **which means a hand-built call frame is correct or it is a fault, with nothing in between.**

**And the reserved area below the stack pointer is a reminder that a function's stack usage is not local information.** Measured, a function used space below the stack pointer without claiming it, **so reading one function's prologue does not tell you how much stack it used.**

### Detection and mitigation

- **Establish the offset from a buffer to the return address by reading the frame**, since four builds of one source measured 56, 72, 104 and 40 bytes.
- **Count a function's arguments before reading its entry**, because the first six arrive in registers and the rest above the return address.
- **Check that a callee restores every register it is obliged to**, since a corrupted callee-saved register fails in the caller rather than in the function.
- **Keep the stack pointer aligned at every call**, because a measured call that was off by eight bytes received a signal rather than a slowdown.
- **Account for stacked arguments when computing alignment**, since the measured push of the seventh argument moves the stack pointer by eight.
- **Do not infer a function's stack usage from its prologue alone**, because a leaf function may use the space below the stack pointer without adjusting it.
- **Look for the one slot that holds a code address when auditing a frame**, since it is the only part of the frame that changes control flow.
- **For mitigation, keep the compiler's protections in place where the platform offers them**, since the measured curve shows how little separates an overwrite of frame content from an overwrite of the return address.
- **Test with the build that ships**, since the measured layout differences come from the compiler's decisions rather than from the source.
- **And read a frame outward from a buffer to the return address when analysing a write primitive**, because the measured traversal is what turns a length violation into control flow.

<!-- lang:zh -->
### 四个问题，以及一块回答它们全部的区域

一次调用要交出控制、交出参数、收下一个结果，再回来。**调用约定是关于"怎么做"的那个约定，而栈是其中大部分事情发生的地方** —— 包括那个决定之后控制流去哪里的槽位。

> **一次调用在栈上只留下一样东西：回来的地址。任何能改写那八个字节的东西，就决定了程序接下来去哪。**

下面通过调进一段手写汇编、以及读一个真实的栈帧来实测。

### 第一部分：为什么需要栈

**寄存器是一个固定的小集合**，而一个函数在调用另一个函数时，需要某个地方保存自己的值 —— 这正是递归强加给每一种设计的处境。

**于是有一块内存，按调用分配、按返回释放。** 在这个架构上它向下增长，一个寄存器保存着当前的边界，而这套安排就是"一个函数的状态对那一次调用是私有的"这件事的来源。

**而那块状态里恰好有一样不属于这个函数自己的事**：回来的那个地址，由调用指令放进去、由返回指令取出来。**这一篇里对安全有影响的一切，都是关于那一个槽位和它周围的地方。**

### 第二部分：一个约定必须决定什么

| 问题 | 这个平台的答案 |
|---|---|
| 参数放哪里 | **前六个在寄存器里，其余在栈上** |
| 结果放哪里 | 寄存器 |
| 哪些寄存器能活过一次调用 | **一部分，而且被调用方必须把它们恢复** |
| 谁来清理栈 | 分开算，看那块空间归调用方还是被调用方 |

**而第三行是"分开编译的代码能互相调用"这件事的全部依据**：如果被调用方可以随便破坏任何寄存器，那调用方就没法在调用期间把任何东西留在寄存器里。**这个约定把寄存器分成两半：一半调用方必须假设已经没了，一半被调用方承诺还回来。**

**而第一行把"读一个陌生函数"变成了一道数数的题。** 若干个参数从点名的寄存器进来，其余的从返回地址上面的内存进来 —— **所以一个参数在栈上的位置取决于它是第几个参数。**

### 第三部分：一次调用留下了什么

实测，用一个手写函数报出它刚进来时看到的栈指针：

| 在哪里读栈指针 | 值 |
|---|---|
| 调用方，调用之前 | `0x00007ffffde89b30` |
| 被调用方第一条指令处 | `0x00007ffffde89b28` |
| **差值** | **8 字节** |

**而被调用方栈指针处那个值就是返回地址** —— 一个代码段里的地址，也就是返回指令将要载入指令指针的东西。

> **一次调用只压进一样东西：回来的那个地址。那是调用在栈上留下的唯一痕迹，也是这一篇剩下部分围绕的那个槽位。**

**这就是为什么在安全语境里"栈"这个词有误导性。** 有意思的不是那块区域；**是相对于帧指针某个已知偏移上的八个字节**，而那八个字节恰好是一条之后的指令会跳过去的地址。

### 第四部分：参数，在寄存器里，然后就不是了

实测，编译器为了调用一个七参数函数而发出的指令：

```
push   0x7
mov    r9d, 0x6
mov    r8d, 0x5
mov    ecx, 0x4
mov    edx, 0x3
mov    esi, 0x2
mov    edi, 0x1
call   ...
add    rsp, 0x8
```

**按顺序读：那个装不下的第七个参数先被压栈，然后前六个被放进各自的寄存器，然后调用，然后把栈调回来。**

**而被调用函数在入口处把它们全部存进自己的帧**，六个来自寄存器、第七个来自返回地址之上。

> **一个参数的位置取决于它是第几个：前六个在寄存器里，其余在返回地址上方的内存里。所以读一个陌生函数的入口，先要数出它有几个参数。**

**而调用前那次压栈还有第二个后果值得注意**：它把栈指针移动了八个字节，**也就是下一部分要讲的对齐要求的一半** —— 所以"栈上参数个数是奇数"与"调用点没对齐"是同一个问题的两种说法。

### 第五部分：哪些寄存器活得过调用

实测，两个做同样算术的函数，一个用被调用方必须保全的寄存器、一个用它不必保全的：

| 函数用了 | 编译器发出的东西 |
|---|---|
| 两个被调用方需保全的寄存器 | 开头 `push r12` … `push rbx`，返回前 `pop rbx` … `pop r12` |
| 两个调用方需保全的寄存器 | **什么都没有** —— 直接用，没有谁去恢复 |

**第二个函数的调用方不能指望那些寄存器在调用之后还装着什么**，而它也不指望：约定这么说了，编译器也照这个安排了自己的代码。

> **这个约定把寄存器分成两半：一半调用方必须假设已经没了，一半被调用方承诺还回来 —— 而这条分界线就是"分开编译的代码能互相调用"的全部基础。**

**而它的安全后果与这个系列里大多数篇目的方向相反。** 被破坏的栈通常被当成"夺走返回地址"的手段来讲，**而被破坏的被保全寄存器是一条"在离出事地点很远的地方出错"的路** —— 在调用方里、在那个函数返回之后，而栈上什么线索都没留下。

### 第六部分：栈帧，以及返回地址在哪个方向

实测，在一个有 32 字节缓冲区的函数里，从缓冲区往外走：

| 距缓冲区的偏移 | 值 | 它是什么 |
|---|---|---|
| +0 到 +24 | `0xa7a6a5a4a3a2a1a0` … | 程序写进去的字节 |
| **+96** | `0x00007ffcf1065fa0` | **`[rbp]` —— 调用方的帧指针** |
| **+104** | `0x000055ef3f9a1327` | **`[rbp+8]` —— 返回地址，在代码段里** |

**而实测通过比较确认了那个槽位**：`[rbp]` 处的值恰好等于调用方的帧指针，这正是"保存的帧指针"的含义 —— **它是上一个函数的，不是这个函数的。**

> **局部缓冲区在帧指针下方，而返回地址在它上面八个字节处。所以一次对局部缓冲区的过长写入，是在朝返回地址的方向走。**

**而它要走多远不是源码的性质。** 实测，同一个程序的几份构建：

| 构建 | 从缓冲区到返回地址的距离 |
|---|---|
| 一种局部变量安排 | **56 字节** |
| 另一种 | **72 字节** |
| 把帧指针与一个偏移跨循环保活 | **104 字节** |
| 同一份源码带优化 | **40 字节** |

**同一份源码、四个数字。** 那个距离由编译器选择在缓冲区与帧指针之间放什么决定。

> **从一块缓冲区到返回地址的偏移，是"某一份构建"的性质、而不是"这个程序"的性质。它靠读或量得到，永远不能假定。**

**而后果是一条曲线、不是一个边界。** 实测，往一个 16 字节的缓冲区里写越来越多的字节：

| 写入字节数 | 结果 |
|---|---|
| 16、20、24、28、32、36、40 | **程序正常退出** |
| **48、64、128** | **被信号终止** |

**这一张表就是整个故事的两半**：缓冲区之后还有别的帧内容可以被无害地覆盖，**然后是返回地址，越过它之后返回指令会跳到一个不是返回地址的东西上。**

### 第七部分：两条不在指令里的约定

**第一条是对齐。** 实测，故意把栈指针错开八个字节去调用一个函数：

| 调用 | 结果 |
|---|---|
| 栈指针错开 8 字节 | **程序收到段错误** |
| 按约定对齐 | 正常返回 |

**而负责的那条指令在反汇编里看得见**：那个函数通过一个寄存器搬动一个 16 字节的值，用的是一条**要求地址 16 字节对齐**的指令。

> **调用点上栈必须对齐，而违反它的后果不是"慢一点"，是一条只在那些用了对齐敏感指令的函数里出现的崩溃。**

**这就是为什么手写汇编拼装一次调用时必须自己算那笔账**，也是为什么实测那次"压入栈上参数"要紧两次：**它改变了这次调用所做一切的地址对齐。**

**而第二条约定是栈指针下面那块空间**，叶函数可以随便用。实测，一个函数往栈指针下方的地址写东西、**却完全没有调整它**：

```
mov    QWORD PTR [rsp-0x28], rdi
```

**那是编译器在用那块保留空间**，而同样一条指令也出现在手写版本里。

> **一个不调用任何东西的函数，可以不认领就直接使用栈指针正下方的那几个字节 —— 所以一个函数的栈用量没法只从它自己的前言里读出来。**

**而那个例外正是它安全的原因**：那块区域是留给正在运行的那个函数的，任何可能异步到来、又要用栈的东西都会打破这个假设 —— **这正是约定只把它留给"不调用任何东西的函数"的原因。**

### 第八部分：从这些机制推出的安全观念

**返回地址是栈上唯一决定控制流去哪里的那一样东西**，而它坐在帧指针上方一个固定的偏移处。**关于栈被破坏的一切都从这一句话推出来**，而实测给出了让它具体起来的两个数字：**一次调用把栈指针恰好移动八个字节**，而返回地址在**帧指针加八**处。

**而从一块缓冲区到那个槽位的距离不是一个常量。** 同一个程序的四份构建给出 56、72、104 和 40 字节，**而往一个十六字节的缓冲区里写四十个字节可以活着、写四十八个就不行。** 所以那个偏移是关于某一个二进制的事实、要去确立，**而按一个假定的常量写出来的东西，是只在某一份构建上成立的东西。**

**而约定里关于寄存器的那一半是第二种、更安静的出错方式。** 实测，被调用方恢复了它有义务恢复的寄存器、对其余的不动，**所以一个被破坏的被保全寄存器，是在那个函数返回之后、在调用方里浮出水面的** —— 而证据指向了错误的地方。

**而对齐规则是对任何"拼装调用"的东西的约束。** 实测，栈指针错开八个字节的调用，在一个用了对齐敏感指令的函数里崩了，**这意味着手搭的调用帧要么是对的、要么就是一次故障，中间没有别的可能。**

**而栈指针下面那块保留空间提醒我们：一个函数的栈用量不是局部信息。** 实测，一个函数没有认领就用了栈指针下方的空间，**所以读一个函数的前言，并不能告诉你它用了多少栈。**

### 检测与缓解

- **靠读栈帧来确立"从缓冲区到返回地址"的偏移**，因为同一份源码的四份构建实测是 56、72、104 与 40 字节。
- **读一个函数的入口之前先数出它有几个参数**，因为前六个从寄存器进来、其余从返回地址上方进来。
- **核对被调用方恢复了每一个它有义务恢复的寄存器**，因为一个被破坏的被保全寄存器是在调用方里失败、而不是在那个函数里。
- **在每一次调用上保持栈指针对齐**，因为实测一次错开八个字节的调用得到的是一个信号、而不是一点延迟。
- **算对齐的时候把栈上的参数算进去**，因为实测那次压入第七个参数把栈指针移动了八个字节。
- **不要只从一个函数的前言推断它的栈用量**，因为叶函数可以不调整栈指针就使用它下方的空间。
- **审一个栈帧时，去找那个装着代码地址的槽位**，因为它是帧里唯一改变控制流的部分。
- **缓解上，在平台提供保护的地方让编译器的保护留在原位**，因为实测那条曲线显示"覆盖帧内容"与"覆盖返回地址"之间隔得很少。
- **用实际发布的那个构建去测**，因为实测那些布局差异来自编译器的决定、而不是源码。
- **分析一个写入原语时，从缓冲区往外一路读到返回地址**，因为实测那条路径正是"一个长度违规"变成"控制流被夺走"的过程。
