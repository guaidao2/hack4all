---
id: asm-fundamentals
title_en: Assembly Fundamentals
title_zh: 汇编基础
summary_en: The layer between a compiler and the machine, where an instruction is a sequence of bytes whose length is not fixed and a register write has a width that decides how much of it is cleared. Measured by assembling, running and disassembling real code — one mnemonic encoded four different ways, four sub-register writes producing four different values, and one byte string read three ways from three starting points.
summary_zh: 编译器与机器之间的那一层 —— 那里一条指令是一串字节、长度不固定，而一次寄存器写入的宽度决定了它清掉多少位。这一篇把真实的代码汇编、运行、反汇编来实测：同一个助记符的四种编码、四次子寄存器写入得到的四个不同的值，以及同一串字节从三个起点读出的三种读法。
tags: [beginner, asm, x86-64, reverse-engineering]
tools: [nasm, gcc, objdump, gdb]
attck: [T1027]
platform: [linux]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The layer where nothing is inferred

Every description of a program above this layer is an interpretation. A source file says what the author meant, a compiler says what it decided, and **the instruction stream says what the machine will do** — and where the three disagree, the third one wins.

> **An instruction is a sequence of bytes, its length is not fixed, and where the next one starts is decided by the current one. A register write has a width, and the width decides how much of the register stops being what it was.**

Both of those are measured below by assembling, running and disassembling real code.

### Part 1: why this layer is worth reading

**Because it is the only complete record.** A source file may be unavailable, an intermediate representation is a compiler's business, and documentation describes intent — **while the instructions are the thing that runs**, and every property that matters for behaviour is present in them.

**And because it is the layer where the interesting mistakes live.** A check that the compiler removed, a register write that cleared more or less than the author expected, a branch that reads a flag nobody set — none of those are visible in the source, and all of them are visible here.

**And because it is finite.** The instruction set is a table, the encoding rules are a table, and the small number of conventions around them are a table. **What follows is those tables plus what the measurements show about using them.**

### Part 2: the machine's model

| Element | What it is |
|---|---|
| registers | a small, named, fixed set — **the only place arithmetic happens** |
| memory | bytes, addressed by number, **not directly computable** |
| flags | a handful of bits set as a side effect of arithmetic |
| instruction pointer | the address of the next instruction |
| an instruction | **a byte sequence with an operation and its operands** |

**And the first row is the one that shapes everything**: arithmetic operates on registers, so a value in memory has to be loaded, changed and stored — **which is why reading assembly means tracking where a value currently lives.**

**And the third row is how control flow works.** There is no "if" in the instruction set: there are comparisons that set flags and jumps that read them, **so a conditional in source is a pair of instructions whose correctness depends on nothing having changed the flags in between.**

### Part 3: instructions have no fixed length

Measured, assembling one-line statements and reading their encodings:

| Statement | Encoding | Bytes |
|---|---|---|
| `nop` | `90` | 1 |
| `mov eax, 1` | `B8 01000000` | 5 |
| `mov rax, 1` | `B8 01000000` | **5 — the same as the 32-bit form** |
| `mov rax, 0x123456789abcdef0` | `48 B8 <8 bytes>` | **10** |
| `mov rax, -1` | `48 C7 C0 FFFFFFFF` | 7 |
| `mov al, 1` | `B0 01` | 2 |
| `mov ax, 1` | `66 B8 0100` | 4 |
| `mov eax, ebx` | `89 D8` | 2 |
| `mov rax, [rsp]` | `48 8B 0424` | **4** |
| `mov eax, [rsp]` | `8B 0424` | 3 |
| `inc rax` | `48 FF C0` | **3** |
| `inc eax` | `FF C0` | 2 |
| `xor rax, rax` | `48 31 C0` | 3 |
| `xor eax, eax` | `31 C0` | 2 |

**Two things to take from the table.** The prefix byte appears exactly when the width matters, **costing one byte** — and the same operation on the 32-bit register is one byte shorter every time.

**And the assembler chose the 32-bit form for `mov rax, 1`**, because for a non-negative value that fits in 32 bits the results are identical — **which is the property measured in the next part, being used by the tool.**

**And the second consequence of variable length is that a starting point is part of the decoding.** Measured, one byte string from three offsets:

```
bytes: 48 31 c0 48 83 c0 01 c3

offset 0:   xor rax, rax          add rax, 0x1        ret
offset 1:   xor eax, eax          add rax, 0x1        ret
offset 2:   ror BYTE PTR [rax-0x7d], 0xc0             add ebx, eax
```

**Three readings of one string.** The first is what the assembler produced; the second is a similar computation with a different register width; **the third is something else entirely.**

> **An instruction has no length until it is decoded, so where the next one starts is decided by the current one — and the processor does not know what the author intended, it decodes from wherever it is told to start.**

**Which matters in both directions.** A disassembler pointed at the right start produces the program; **pointed at the wrong one — a jump into the middle of an instruction, an embedded data table — it produces a plausible sequence that is not what runs.**

### Part 4: a register write has a width

This is the measurement that decides more bugs than any other in this entry. Measured, four ways of writing the same value into the same register, after filling it with all ones:

| Write | Resulting 64-bit value |
|---|---|
| `mov eax, 1` | `0x0000000000000001` — **the upper 32 bits are cleared** |
| `mov ax, 1` | `0xffffffffffff0001` — the upper bits are kept |
| `mov al, 1` | `0xffffffffffffff01` — the upper bits are kept |
| `xor eax, eax` | `0x0000000000000000` |

**And with a negative value, the same three writes give three different numbers:**

| Write | Encoding | Resulting value |
|---|---|---|
| `mov eax, -1` | `B8 FFFFFFFF` | `0x00000000ffffffff` |
| `mov rax, -1` | `48 C7 C0 FFFFFFFF` | `0xffffffffffffffff` |
| `mov ax, -1` | `66 B8 FFFF` | `0x000000000000ffff` |
| `mov al, -1` | `B0 FF` | `0x00000000000000ff` |

> **Writing the 32-bit name of a register clears the upper 32 bits; writing the 16-bit or 8-bit name does not. The three are not interchangeable, and the assembler relies on the difference to pick the shorter encoding.**

**And the reason is compatibility rather than elegance**: making a 32-bit write produce a well-defined 64-bit result means code written for the 32-bit mode behaves the same way here, **without a prefix and without extra instructions.**

**Which produces two opposite consequences worth keeping apart.** A clear of a register written as `xor eax, eax` **does clear the whole thing** — the property is what makes the idiom reliable. **And a sequence that writes a 32-bit name expecting the upper bits to survive gets them cleared instead**, which is the shape of a bug that only appears when a value is large.

### Part 5: flags are a side effect, and only some instructions produce them

Measured, the flags after each of a series of operations:

| Instruction | CF | ZF | SF | OF | PF | AF |
|---|---|---|---|---|---|---|
| `mov rax, 5` | 0 | 0 | 0 | 0 | 1 | 0 |
| `cmp rax, rax` (equal) | 0 | **1** | 0 | 0 | 1 | 0 |
| `cmp` 5 with 9 | **1** | 0 | **1** | 0 | 1 | 1 |
| `cmp` 9 with 5 | 0 | 0 | 0 | 0 | 0 | 0 |
| `add` overflowing | 0 | 0 | **1** | **1** | 1 | 1 |
| `sub` borrowing | **1** | 0 | **1** | 0 | 1 | 1 |
| `test rax, rax` (zero) | 0 | **1** | 0 | 0 | 1 | 0 |
| **`inc rax` from -1** | **1 — unchanged from before** | 1 | 0 | 0 | 1 | 1 |
| **`add rax, 0` from -1** | **0 — modified** | 0 | 1 | 0 | 1 | 0 |

**Three lessons in one table.**

**`mov` sets nothing**, so a flag read after a `mov` is a flag from an earlier instruction — **which makes the flags a record of the last arithmetic rather than a property of the current state.**

**A comparison is a subtraction that keeps the flags**, so the ordering outcomes are visible: less-than sets carry and sign, equal sets zero, greater sets neither.

**And two instructions that look interchangeable are not.** The measured `inc` left the carry flag exactly as it was while the `add` of zero changed it — **so a loop whose exit condition reads the carry across an increment is depending on a flag that the increment does not touch**, which is correct and also easy to misread.

> **Only some instructions produce flags, and only some flags. Reading a branch means finding the last instruction that set the flag it reads.**

### Part 6: addressing, and the instruction that computes without touching memory

**A memory operand is computed by the processor from a base, an index, a scale and a displacement**, which means **address arithmetic is available as a side effect of every load and store.**

**And one instruction exposes that arithmetic without performing a load**: measured, the compiler's output for a three-argument addition:

```
add    rdi, rsi
lea    rax, [rdi+rdx*1]
ret
```

**The second instruction computes an address and never reads it** — it is an addition, written in the syntax of memory.

> **`lea` takes an address expression rather than a memory access, so it does arithmetic at load-and-store speed without touching memory — which is why compiled code is full of it and why reading it as a load is wrong.**

**And the encoding of an address is where the length difference shows again**: measured, `mov eax, [rsp]` is three bytes and `mov rax, [rsp]` is four — **the same one-byte prefix**, because the width of the destination is what it is there for.

**And the byte order is little-endian.** Measured, the 32-bit value `0x01020304` in memory from the lowest address:

```
04 03 02 01
```

**Which matters when a value is read as bytes rather than as a number** — and every measurement in this series that involved comparing a byte string with a parsed value is a place where the two readings had to be kept apart.

### Part 7: what optimisation does to the correspondence

**A function and the instructions it became.** Measured, a three-argument addition and a function that calls it, compiled with optimisation:

```
add3:
    add    rdi, rsi
    lea    rax, [rdi+rdx*1]
    ret

call_it:
    mov    eax, 0x6
    ret
```

**The first function is three instructions** because arguments arrive in registers and the result leaves in one. **The second does not call anything**, because the call was evaluated at compile time — **the function it was supposed to call is not in its instruction stream at all.**

**And the gap between them is filled with a multi-byte `nop`**, which is alignment rather than anything the source asked for.

> **Optimised assembly does not correspond line by line with the source. Reading it as a translation of the code recovers an intention; reading it as a sequence of operations recovers the behaviour — and only the second one is reliable.**

**And the practical consequence is a habit rather than a rule**: when a check is missing from a binary, the question is whether it was removed, folded into another condition, or never present — **and all three are answered by reading the operations rather than by comparing the two versions of the text.**

### Part 8: what follows for security

**Assembly is the authority on what a program does.** A source file describes an intention, a compiler describes a decision, **and the instructions are what runs** — so a claim about behaviour that has not been checked against them is a claim about one of the other two.

**And the width of a register write is a property that does not appear in any higher-level language.** Measured, four writes of the same value produced four different 64-bit results, and the difference between the 32-bit and 16-bit forms is a clear of the upper half or not. **So a value that survives a "clearing" operation, or fails to, is a question to answer here.**

**And a check is only as good as the flags it reads.** Measured, `inc` left the carry flag alone where `add` changed it, and `mov` changed nothing at all — **so the correctness of a conditional is a question about which instruction last touched the flag it reads**, which is visible only in this layer.

**And variable length means a starting point is part of the reading.** Measured, three offsets into one byte string produced three programs, one of them nothing like the others. **So a disassembly is a claim about where instructions begin**, and anything that decodes from a computed address is making that claim.

**And optimisation changes the correspondence rather than the behaviour.** Measured, a call became a constant and a function became three instructions. **So the presence of a check in source and its presence in a binary are two different facts**, and the second is the one an attacker and a defender both care about.

### Detection and mitigation

- **Answer behaviour questions from the instructions rather than from the source**, since the measured optimised output had no call to the function it was supposedly calling.
- **Check the width of every register write that is meant to clear a value**, because the 32-bit form clears the upper half and the 16-bit and 8-bit forms do not.
- **Trace the flag a branch reads back to the instruction that set it**, since measured instructions differ in which flags they touch and some touch none.
- **Do not treat a write to a 32-bit name as equivalent to a write to the 64-bit one**, in either direction, and do not rely on the assembler's choice of the shorter encoding.
- **Record the entry point of a disassembly as part of the result**, since the measured offsets produced three different readings of one byte string.
- **Prefer decoding from a known instruction boundary** — a symbol, a call target, a declared entry — over decoding from a computed address.
- **Remember that alignment padding is not code**, since the measured output contained a multi-byte `nop` that no source line produced.
- **Compare the behaviour of two builds by their operations rather than by their text**, since optimisation changes the correspondence while preserving the result.
- **When writing assembly, state the intended width explicitly** rather than relying on a name, because the measured difference between `eax`, `ax` and `al` is not a naming choice.
- **And keep the three records apart** — the intention, the compiler's decision, and the instruction stream — because only the third one is checked by running it.

<!-- lang:zh -->
### 那一层不做任何推断的东西

在这一层之上，关于一个程序的每一种描述都是一种解释。源文件说的是作者的意思，编译器说的是它做的决定，**而指令流说的是机器将会做什么** —— 三者不一致时，第三个说了算。

> **一条指令是一串字节，长度不固定，而下一条从哪里开始由当前这一条决定。一次寄存器写入有一个宽度，而那个宽度决定了这个寄存器有多少位不再是原来的东西。**

两者都在下面通过汇编、运行与反汇编真实代码来实测。

### 第一部分：为什么这一层值得读

**因为它是唯一完整的记录。** 源文件可能拿不到，中间表示是编译器的事，文档描述的是意图 —— **而指令才是运行的那个东西**，一切对行为有影响的性质都在里面。

**也因为有意思的错误就住在这一层。** 一个被编译器去掉的检查、一次比作者预期清得多或少了的寄存器写入、一个读着没人设过的标志的分支 —— 这些在源码里都看不见，在这里全都看得见。

**还因为它是有限的。** 指令集是一张表，编码规则是一张表，围绕它们的那几条约定也是一张表。**接下来就是那几张表，加上实测显示出的关于怎么用它们的东西。**

### 第二部分：机器的模型

| 元素 | 它是什么 |
|---|---|
| 寄存器 | 一小撮有名字、数量固定的东西 —— **算术只在这里发生** |
| 内存 | 字节，按编号寻址，**不能直接参与运算** |
| 标志 | 几个位，作为算术的副作用被设置 |
| 指令指针 | 下一条指令的地址 |
| 一条指令 | **一串字节，含一个操作与它的操作数** |

**而第一行塑造了其余一切**：算术在寄存器上做，所以内存里的一个值必须先被载入、改动、再存回 —— **这就是为什么读汇编意味着追踪一个值此刻住在哪里。**

**而第三行是控制流的工作方式。** 指令集里没有"如果"：有的是设置标志的比较、以及读那些标志的跳转，**所以源码里的一个条件是一对指令，而它们的正确性取决于中间没有任何东西动过那些标志。**

### 第三部分：指令没有固定长度

实测，把一行行的语句汇编出来、读它们的编码：

| 语句 | 编码 | 字节数 |
|---|---|---|
| `nop` | `90` | 1 |
| `mov eax, 1` | `B8 01000000` | 5 |
| `mov rax, 1` | `B8 01000000` | **5 —— 与 32 位形式相同** |
| `mov rax, 0x123456789abcdef0` | `48 B8 <8 字节>` | **10** |
| `mov rax, -1` | `48 C7 C0 FFFFFFFF` | 7 |
| `mov al, 1` | `B0 01` | 2 |
| `mov ax, 1` | `66 B8 0100` | 4 |
| `mov eax, ebx` | `89 D8` | 2 |
| `mov rax, [rsp]` | `48 8B 0424` | **4** |
| `mov eax, [rsp]` | `8B 0424` | 3 |
| `inc rax` | `48 FF C0` | **3** |
| `inc eax` | `FF C0` | 2 |
| `xor rax, rax` | `48 31 C0` | 3 |
| `xor eax, eax` | `31 C0` | 2 |

**从这张表里拿两件事。** 那个前缀字节恰好在宽度要紧的时候出现，**代价是一个字节** —— 而同一个操作作用于 32 位寄存器时，每一次都短一个字节。

**而汇编器给 `mov rax, 1` 选了 32 位形式**，因为对一个放得进 32 位的非负值来说两者结果相同 —— **那正是下一部分要实测的性质，被工具拿去用了。**

**而变长带来的第二个后果是"起点"成了解码的一部分。** 实测，一串字节从三个偏移读：

```
字节: 48 31 c0 48 83 c0 01 c3

偏移 0:   xor rax, rax          add rax, 0x1        ret
偏移 1:   xor eax, eax          add rax, 0x1        ret
偏移 2:   ror BYTE PTR [rax-0x7d], 0xc0             add ebx, eax
```

**一串字节的三种读法。** 第一种是汇编器产出的东西；第二种是寄存器宽度不同的相近运算；**第三种完全是别的东西。**

> **一条指令在被解码之前没有长度，所以下一条从哪里开始由当前这一条决定 —— 而处理器不知道作者想要什么，你让它从哪里开始它就从哪里解码。**

**这件事两个方向都要紧。** 反汇编器指对了起点就产出那个程序；**指错了 —— 一次跳进指令中间的跳转、一张嵌进去的数据表 —— 它产出的是一个看起来说得通、却不是实际运行的东西的序列。**

### 第四部分：一次寄存器写入有一个宽度

这是这一篇里决定 bug 比任何别的都多的一组实测。实测，把一个寄存器填成全 1 之后，用四种方式写同一个值进去：

| 写入 | 得到的 64 位值 |
|---|---|
| `mov eax, 1` | `0x0000000000000001` —— **高 32 位被清零** |
| `mov ax, 1` | `0xffffffffffff0001` —— 高位保留 |
| `mov al, 1` | `0xffffffffffffff01` —— 高位保留 |
| `xor eax, eax` | `0x0000000000000000` |

**而换成一个负值，同样三种写入给出三个不同的数：**

| 写入 | 编码 | 得到的值 |
|---|---|---|
| `mov eax, -1` | `B8 FFFFFFFF` | `0x00000000ffffffff` |
| `mov rax, -1` | `48 C7 C0 FFFFFFFF` | `0xffffffffffffffff` |
| `mov ax, -1` | `66 B8 FFFF` | `0x000000000000ffff` |
| `mov al, -1` | `B0 FF` | `0x00000000000000ff` |

> **写一个寄存器的 32 位名字会清零高 32 位；写 16 位或 8 位的名字不会。三者不能互换，而汇编器正是靠这个差别去挑更短的编码。**

**而这么做的理由是兼容、不是优雅**：让一次 32 位写入产生一个确定的 64 位结果，意味着为 32 位模式写的代码在这里行为一样，**不需要前缀、也不需要额外指令**。

**由此产生两个相反方向的后果，值得分开记。** 一次写成 `xor eax, eax` 的清零**确实清掉了整个寄存器** —— 这个性质正是那个习惯写法可靠的原因。**而一段写 32 位名字、指望高位留着的序列，会得到高位被清零的结果**，那就是一个只在值变大时才出现的 bug 的形状。

### 第五部分：标志是副作用，而只有一部分指令产生它

实测，一串操作之后各自的标志：

| 指令 | CF | ZF | SF | OF | PF | AF |
|---|---|---|---|---|---|---|
| `mov rax, 5` | 0 | 0 | 0 | 0 | 1 | 0 |
| `cmp rax, rax`（相等） | 0 | **1** | 0 | 0 | 1 | 0 |
| `cmp` 5 与 9 | **1** | 0 | **1** | 0 | 1 | 1 |
| `cmp` 9 与 5 | 0 | 0 | 0 | 0 | 0 | 0 |
| `add` 溢出 | 0 | 0 | **1** | **1** | 1 | 1 |
| `sub` 借位 | **1** | 0 | **1** | 0 | 1 | 1 |
| `test rax, rax`（为零） | 0 | **1** | 0 | 0 | 1 | 0 |
| **`inc rax` 从 -1** | **1 —— 与之前一样，没被碰** | 1 | 0 | 0 | 1 | 1 |
| **`add rax, 0` 从 -1** | **0 —— 被改了** | 0 | 1 | 0 | 1 | 0 |

**一张表里有三课。**

**`mov` 什么都不设**，所以一条 `mov` 之后读到的标志是更早某条指令留下的 —— **这就让标志成为"上一条算术的结论"，而不是当前状态的性质。**

**一次比较就是一次保留标志的减法**，所以大小关系是看得出来的：小于会置进位与符号，相等会置零，大于一个都不置。

**而两条看起来可以互换的指令并不是。** 实测那个 `inc` 把进位标志原封不动地留着，而加零的那个 `add` 改了它 —— **所以一个退出条件跨过一次递增去读进位的循环，依赖的是一个递增根本不碰的标志**，这是对的，也很容易读错。

> **只有一部分指令产生标志，而且只产生一部分标志。读一个分支，就是找出最后一条设置它所读那个标志的指令。**

### 第六部分：寻址，以及那条不碰内存却在算的指令

**一个内存操作数由处理器用基址、索引、比例与位移算出来**，这意味着**地址运算在每一次载入与存储里都顺手可用。**

**而有一条指令把这个运算暴露出来、却不真的去载入**：实测，编译器为一个三参数加法给出的输出：

```
add    rdi, rsi
lea    rax, [rdi+rdx*1]
ret
```

**第二条指令算出一个地址、然后根本没去读它** —— 它是一次加法，只是写成了内存的语法。

> **`lea` 取的是一个地址表达式、不是一次访存，所以它能以载入存储的速度做算术而不碰内存 —— 这就是为什么编译出来的代码里到处都是它，也是为什么把它读成一次载入是错的。**

**而一个地址的编码正是长度差再次显出来的地方**：实测，`mov eax, [rsp]` 是三个字节，而 `mov rax, [rsp]` 是四个 —— **同一个字节的前缀**，因为目的操作数的宽度正是它在那里的理由。

**而字节序是小端。** 实测，32 位值 `0x01020304` 在内存里从最低地址开始：

```
04 03 02 01
```

**这在一个值被当成字节读、而不是当成数字读的时候就要紧** —— 而这个系列里每一处"把一串字节与一个解析后的值作比较"的实测，都是一个这两种读法必须分开的地方。

### 第七部分：优化对"对应关系"做了什么

**一个函数，以及它变成的指令。** 实测，一个三参数加法、以及一个调用它的函数，带优化编译：

```
add3:
    add    rdi, rsi
    lea    rax, [rdi+rdx*1]
    ret

call_it:
    mov    eax, 0x6
    ret
```

**第一个函数是三条指令**，因为参数从寄存器进来、结果从寄存器出去。**第二个什么都没调用**，因为那次调用在编译期就被算掉了 —— **它本该调用的那个函数根本不在它的指令流里。**

**而两者之间那段空隙被一个多字节的 `nop` 填满**，那是对齐，不是源码要求过的东西。

> **优化后的汇编与源码不是逐行对应的。把它当成代码的翻译来读，恢复的是一个意图；把它当成一串操作来读，恢复的是行为 —— 而只有后者是可靠的。**

**而实际后果是一个习惯、不是一条规则**：当二进制里少了一个检查，问题是它被去掉了、被折进了另一个条件、还是从来就没有 —— **而这三个都由读操作来回答，而不是由对比两份文本。**

### 第八部分：从这些机制推出的安全观念

**汇编是关于一个程序做什么的权威。** 源文件描述的是一种意图，编译器描述的是一次决定，**而指令才是运行的那个东西** —— 所以一个没有对着指令核对过的行为论断，是关于另外两者之一的论断。

**而一次寄存器写入的宽度是一个在任何更高层语言里都不出现的性质。** 实测，同一个值的四次写入产出四个不同的 64 位结果，而 32 位形式与 16 位形式之间的差别就是"高一半被不被清掉"。**所以一个值在"清零"操作之后是否幸存、或者是否本该幸存，是要在这一层回答的问题。**

**而一个检查只和它所读的那些标志一样可靠。** 实测，`inc` 放着进位标志不管、而 `add` 改了它，而 `mov` 什么都改 —— **所以一个条件的正确性，是一个"最后碰到它所读那个标志的是哪条指令"的问题**，而那只在这一层看得见。

**而变长意味着"起点"是读法的一部分。** 实测，一串字节的三个偏移产出三个程序，其中有一个与其余的毫无相似之处。**所以一次反汇编是一个关于"指令从哪里开始"的论断**，而任何从一个算出来的地址开始解码的东西，都在做那个论断。

**而优化改变的是对应关系、不是行为。** 实测，一次调用变成了一个常数、一个函数变成了三条指令。**所以"源码里有这个检查"与"二进制里有这个检查"是两个不同的事实**，而第二个才是攻击者与防守方都在意的那个。

### 检测与缓解

- **从指令回答问题，而不是从源码**，因为实测的优化输出里根本没有对它本该调用的那个函数的调用。
- **核对每一次"本意是清零"的寄存器写入的宽度**，因为 32 位形式会清掉高一半，而 16 位与 8 位形式不会。
- **把一个分支所读的标志，追回到设置它的那条指令**，因为实测的指令在"碰哪些标志"上不同，而有些一个都不碰。
- **不要把对 32 位名字的写入当成对 64 位名字写入的等价物**，两个方向都不要，也不要依赖汇编器选择更短编码这件事。
- **把反汇编的入口点作为结果的一部分记下来**，因为实测那些偏移对一串字节给出了三种不同的读法。
- **优先从一个已知的指令边界开始解码** —— 一个符号、一个调用目标、一个声明的入口 —— 而不是从一个算出来的地址。
- **记住对齐填充不是代码**，因为实测输出里含着一个没有任何源码行产生过的多字节 `nop`。
- **用操作而不是用文本对比两次构建的行为**，因为优化改变对应关系、却保持结果。
- **写汇编时把想要的宽度明确写出来**，不要靠名字去暗示，因为实测 `eax`、`ax` 与 `al` 之间的差别不是一个命名选择。
- **并且把三份记录分开** —— 意图、编译器的决定、以及指令流 —— 因为只有第三份是跑一遍就会被检验的。
