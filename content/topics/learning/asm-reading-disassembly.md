---
id: asm-reading-disassembly
title_en: Reading Disassembly
title_zh: 读反汇编
summary_en: A disassembly is a set of decodings from a starting point rather than the program itself, so the method is to cross-check several partial views — a scan that reads data as code, an exception table that keeps function ranges after stripping, a string and the table of pointers to it. Measured on binaries built, stripped and disassembled for the purpose.
summary_zh: 一份反汇编是"从某个起点开始的一组解码"，而不是那个程序本身；所以方法是用几条彼此片面的线索互相核对 —— 一次把数据读成代码的扫描、一张剥符号后仍保留函数范围的异常表、以及一个字符串和指向它的指针表。这一篇用为此编译、剥符号并反汇编的二进制实测。
tags: [beginner, asm, reverse-engineering, disassembly]
tools: [objdump, ndisasm, nm, strings, readelf, gdb]
attck: [T1027]
platform: [linux]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A set of decodings, not the program

Two facts decide how this has to be done. **An instruction has no length until it is decoded**, so a starting point is part of the result. **And the things that make a disassembly readable — symbols, function boundaries, source lines — are all separate from the instructions**, added by tools, and removable.

> **So a disassembly is a set of decodings from a chosen starting point, and reading one means cross-checking several partial views rather than trusting a single listing.**

Measured below on binaries built for the purpose, stripped and left alone.

### Part 1: what changes the output

| Input to the reading | What it decides |
|---|---|
| the starting point | where decoding begins, and therefore what the bytes mean |
| the scanning strategy | whether data inside the code section is decoded as code |
| the symbol table | whether anything has a name |
| the debug information | whether an address maps back to a source line |
| the optimisation level | which function boundaries exist at all |

**Every row is a choice made by somebody else** — a build flag, a packaging step, or the disassembler's own default. **So two tools pointed at one binary can produce two different, internally consistent descriptions**, and neither is lying.

### Part 2: the starting point is part of the result

This was measured in the fundamentals entry with a single byte string read from three offsets, and it is the same property here at the scale of a whole program.

**The consequence is that a scan is only as good as its boundaries.** A disassembler that starts at the beginning of an executable section and keeps going **will decode whatever is there**, and code sections contain data more often than the name suggests.

### Part 3: linear sweep, and what it gets wrong

Measured, a short program assembled into one flat blob of bytes — real instructions, then a data table, then more instructions — and disassembled from offset zero:

```
b8 01 00 00 00   c3   48 89 e5   48 83 ec 20   c3   48 8b 45 f8   48 01 d8   c3 ...

mov    eax, 0x1
ret
mov    rbp, rsp      <- the table's first four bytes
sub    rsp, 0x20     <- the next four
ret
mov    rax, [rbp-0x8]
add    rax, rbx
ret
```

**Every line is a valid decoding, and half of them are a table.** The four bytes that the program never executes became a function prologue, because **the decoder had no way to know that nobody jumps there.**

> **A linear scan decodes from its starting point to the end of the section and reports what the bytes could mean, which is not the same as what runs.**

**And the alternative has its own failure.** A scan that follows control flow — starting from entry points and following each branch — **never decodes the data**, and in return **cannot see code that is only reachable through a computed jump**, which the control-flow entry measured as an ordinary thing in a compiler's output.

> **The two strategies are wrong in different places: one reads data as code, the other follows the control flow and therefore misses whatever the control flow does not lead to directly.**

**Which is why the useful habit is not to pick one, but to know which one produced the listing in front of you.**

### Part 4: what stripping removes, and what it does not

Measured, the same program with and without symbols and debug information:

| | With symbols | Stripped |
|---|---|---|
| lines from `nm` | **33** | **1** |
| dynamic symbols | present | **present** |
| exception table entries | 6 | **5 function ranges** |

**The names are gone and three things are not.** The dynamic symbols have to stay, because dynamic linking resolves them by name at run time. **And the exception table stays**, which is the interesting one:

```
pc=0000000000001060..0000000000001082
pc=0000000000001020..0000000000001050
pc=0000000000001050..0000000000001058
pc=0000000000001149..000000000000119f
pc=000000000000119f..00000000000011ec
```

**Five address ranges, each one a function.** The table exists so that a fault can unwind the stack, **and it describes each function by its start and its length.**

> **Stripping removes names, not structure. The exception table, the dynamic symbols and the import table all survive — and the exception table happens to carry every function's address range.**

**Which is a practical way to recover function boundaries from a stripped binary** without any heuristic: ask the unwinder's own table, then read each range as a unit.

**And the import table names the library calls.** Measured on a stripped binary:

```
call <strlen@plt>
call <printf@plt>
```

**So the calls into the library are labelled even though the code around them is not**, which turns a stripped binary into something with visible landmarks.

### Part 5: symbols and debug information are separate questions

Measured, resolving an address back to a source line:

| Build | `addr2line` |
|---|---|
| with debug information | **`prog.c:3`** |
| stripped | **`??:0`** |

**And with debug information the listing interleaves the source with the instructions**, which is what makes a compiled function easy to read at all.

> **Whether an address maps to a source line is a property of one section, removed independently of the symbol table. So "it has symbols" and "it has source information" are two separate findings.**

**And that distinction has a version of it in every language**: a build that produces a symbol file for crashes and ships a stripped binary is making two separate decisions, **and the file it kept has the source lines that the shipped binary does not.**

### Part 6: data is an entrance of its own

**A stripped binary still contains its literals**, and they are usually the fastest way in. Measured:

```
2004 connection refused
2017 permission denied
2029 no such user
2036 invalid token
```

**And the table that points at them is right there**, in a writable data section:

```
4040: 04 20 00 00 00 00 00 00   17 20 00 00 00 00 00 00
      29 20 00 00 00 00 00 00   36 20 00 00 00 00 00 00
```

**Four eight-byte values: `0x2004`, `0x2017`, `0x2029`, `0x2036`** — the four string addresses, in order. **A data structure with a purpose, visible without any symbol at all.**

> **Strings and the tables that point at them survive stripping and say a great deal about what a program does — and following the references outward from them is how the code that uses them is found.**

**And the references are not written as the addresses themselves.** Measured, the code reaches data through a table in the writable segment — an entry loaded with a relative access:

```
mov    rax, QWORD PTR [rip+0x2fc5]     # an entry in the writable segment
```

**Which is why searching a disassembly for a string's address finds nothing**: the address lives in the pointer table, not in the instruction.

### Part 7: optimisation decides what structure exists

Measured, a function that loops and calls a helper, compiled at two levels:

| Level | What the listing shows |
|---|---|
| unoptimised | **`<check>:` as a symbol**, with the loop visible |
| optimised | **no such symbol** — the call was inlined into its caller |

> **A function boundary is an arrangement the compiler made, not a property of the program. An inlined function does not exist in the disassembly while its instructions do.**

**Which is the disassembly-side version of the measurement from the fundamentals entry**, where a call became a constant: **the structure of the listing is the compiler's decision, and the behaviour is the instructions'** — so a reading that depends on function boundaries depends on a build flag.

### Part 8: what follows for security

**Any single view of a binary is partial, and the parts are complementary.** Measured, a linear scan produced a plausible function prologue out of a data table; the exception table gave five function ranges with no symbols at all; the dynamic symbols named the library calls; and the string table with its pointers described the program's vocabulary. **So the method is to collect several of those and check them against each other**, and a conclusion drawn from one listing is a conclusion about that listing.

**And stripping is not concealment.** Measured, the function ranges survived in the unwinder's table, the library calls survived as named imports, and the literals survived with the table that points at them. **So a binary that was stripped to make analysis harder is a binary whose structure is still described by three sections that were left in place** — because removing them would break the unwinding and the linking that the program needs.

**And the choice of scan decides which way a reading is wrong.** Measured, a linear scan decoded data as instructions, **and the alternative cannot see targets it does not reach by following control flow** — so knowing which produced a listing is more useful than having a preference between them.

**And where a function is called from a table, the address is not in the instruction.** Measured, the string addresses lived in a pointer table and the code reached it through a relative load, **so searching a listing for a value finds it where the value is stored and not where it is used** — and the direction of that search is the difference between finding a structure and finding nothing.

**And source lines are a separate artefact.** Measured, an address resolved to a file and line only in the build that had the information, **and a stripped build resolved to nothing** — which is why a crash report without a matching symbol file is an address and not a location.

### Detection and mitigation

- **Record which starting point and which scan produced a listing**, since the measured linear scan decoded a data table as a function prologue.
- **Prefer the unwinder's table over heuristics when recovering function boundaries from a stripped binary**, because the measured ranges were exact and heuristic prologue matching is not.
- **Use the dynamic symbols and the import table as landmarks**, since the measured library calls stayed named while the code around them did not.
- **Search for a value's storage rather than for its use**, because the measured addresses lived in a pointer table and the code reached it through a relative load.
- **Read the literals and the tables that point at them early**, since they describe what a program is about before any code has been understood.
- **Keep the symbol file that matches a shipped build**, because the measured resolution to a source line depended on information the shipped binary does not have.
- **Check whether a binary was optimised before reasoning about its function boundaries**, since a measured function was inlined and had no symbol while its instructions were present.
- **Cross-check two disassemblers on a disputed region**, because both will decode the same bytes and agreement narrows the possibilities without confirming the intent.
- **Do not treat stripping as a control**, since the measured function ranges, imports and literals all survived it.
- **And write down which of the partial views each conclusion came from**, because the measured views disagree in opposite directions and a conclusion inherits the limits of its source.

<!-- lang:zh -->
### 一组解码，而不是那个程序

两个事实决定了这件事必须怎么做。**一条指令在被解码之前没有长度**，所以起点是结果的一部分。**而让一份反汇编读得懂的那些东西 —— 符号、函数边界、源码行 —— 都与指令本身分开**，由工具加上去，也可以被拿掉。

> **所以一份反汇编是"从某个选定的起点开始的一组解码"，而读它意味着用几条彼此片面的线索互相核对，而不是相信单独一份清单。**

下面用为此编译、剥符号与未剥符号的二进制实测。

### 第一部分：是什么在改变输出

| 读的时候的输入 | 它决定了什么 |
|---|---|
| 起点 | 解码从哪里开始，以及因此这些字节是什么意思 |
| 扫描策略 | 代码段里的数据会不会被当成代码解码 |
| 符号表 | 有没有什么东西有名字 |
| 调试信息 | 一个地址能不能回到源码行 |
| 优化等级 | 哪些函数边界存在 |

**每一行都是别人做的一个选择** —— 一个构建参数、一个打包步骤、或者反汇编器自己的默认值。**所以两个工具对着同一个二进制可以给出两份不同、却各自自洽的描述**，而两者都没有撒谎。

### 第二部分：起点是结果的一部分

这一点在基础篇已经用一串从三个偏移读的字节实测过，而在这里它是同一性质在"整个程序"这个尺度上的形态。

**后果是：一次扫描的好坏取决于它的边界。** 一个从某个可执行段开头一路解下去的反汇编器，**会把那里有的东西都解码**，而代码段里的数据比名字暗示的要多。

### 第三部分：线性扫描，以及它错在哪

实测，一个短程序汇编成一整块平坦字节 —— 真指令、然后一张数据表、然后再一些指令 —— 从偏移零开始反汇编：

```
b8 01 00 00 00   c3   48 89 e5   48 83 ec 20   c3   48 8b 45 f8   48 01 d8   c3 ...

mov    eax, 0x1
ret
mov    rbp, rsp      <- 表里的头四个字节
sub    rsp, 0x20     <- 接着的四个
ret
mov    rax, [rbp-0x8]
add    rax, rbx
ret
```

**每一行都是一次有效的解码，而其中一半是一张表。** 程序从不执行的那四个字节变成了一个函数前言，因为**解码器无从知道没有人往那里跳。**

> **一次线性扫描从它的起点一路解到段末，报告的是"这些字节能是什么意思"，而这与"实际运行的是什么"不是一回事。**

**而另一种做法有自己的失败。** 一种跟着控制流走的扫描 —— 从入口开始、每条分支都跟过去 —— **永远不会把数据解码**，而作为交换，**它看不到只能通过一个算出来的跳转到达的代码**，而控制流那一篇实测过那是编译器输出里很平常的东西。

> **两种策略错在不同的地方：一种把数据读成代码，另一种跟着控制流走、于是看不到控制流没有直接引向的东西。**

**这就是为什么有用的习惯不是挑一种，而是知道自己手上这份清单是哪一种产出的。**

### 第四部分：剥符号拿掉了什么、没拿掉什么

实测，同一个程序在有符号与无调试信息、以及剥掉之后：

| | 有符号 | 已剥符号 |
|---|---|---|
| `nm` 的行数 | **33** | **1** |
| 动态符号 | 在 | **在** |
| 异常表条目 | 6 | **5 个函数范围** |

**名字没了，而三样东西没没。** 动态符号必须留着，因为动态链接在运行时按名字解析它们。**而异常表留着**，那才是有意思的那个：

```
pc=0000000000001060..0000000000001082
pc=0000000000001020..0000000000001050
pc=0000000000001050..0000000000001058
pc=0000000000001149..000000000000119f
pc=000000000000119f..00000000000011ec
```

**五个地址范围，每一个是一个函数。** 那张表存在的意义是让一次故障能回溯栈，**而它用一个函数的起点与长度来描述它。**

> **剥符号拿掉的是名字，不是结构。异常表、动态符号与导入表都活了下来 —— 而异常表恰好带着每一个函数的地址范围。**

**这是从一个剥了符号的二进制里恢复函数边界的一条实用办法**，不需要任何启发式：去问回溯器自己的那张表，然后把每个范围当成一个单位来读。

**而导入表说出了它调了哪些库函数。** 在一个剥了符号的二进制上实测：

```
call <strlen@plt>
call <printf@plt>
```

**所以那些通向库的调用是有标签的，哪怕它们周围的代码没有** —— 这就把一个剥了符号的二进制变成了一个有可见地标的东西。

### 第五部分：符号与调试信息是两个问题

实测，把一个地址解析回源码行：

| 构建 | `addr2line` |
|---|---|
| 带调试信息 | **`prog.c:3`** |
| 已剥符号 | **`??:0`** |

**而有调试信息时，清单会把源码与指令交错显示**，那才让一个编译出来的函数好歹读得下去。

> **一个地址能不能映射到源码行，是某一节的性质，而它是被独立拿掉的。所以"它有符号"与"它有源码信息"是两条不同的发现。**

**而每一个语言里都有这条区分的一个版本**：一个构建产出符号文件用于崩溃分析、同时发布一个剥了符号的二进制，是在做两个决定，**而它留下来的那个文件里有发布版没有的源码行。**

### 第六部分：数据是它自己的一条入口

**一个剥了符号的二进制里仍然有它的字面量**，而那通常是进去最快的一条路。实测：

```
2004 connection refused
2017 permission denied
2029 no such user
2036 invalid token
```

**而指向它们的那张表就在旁边**，在一个可写的数据段里：

```
4040: 04 20 00 00 00 00 00 00   17 20 00 00 00 00 00 00
      29 20 00 00 00 00 00 00   36 20 00 00 00 00 00 00
```

**四个八字节的值：`0x2004`、`0x2017`、`0x2029`、`0x2036`** —— 四个字符串的地址，按顺序。**一个带用途的数据结构，在没有任何符号的情况下也看得见。**

> **字符串与指向它们的那些表活过了剥符号，并且把程序在做什么说出了很多 —— 而从它们往外追引用，就是找到使用它们的代码的办法。**

**而那些引用并不是以地址本身写出来的。** 实测，代码通过可写段里的一张表去够数据 —— 用一次相对访问载入一个表项：

```
mov    rax, QWORD PTR [rip+0x2fc5]     # 可写段里的一个表项
```

**这就是为什么在反汇编里搜一个字符串的地址什么都搜不到**：那个地址住在指针表里，不在指令里。

### 第七部分：优化决定了存在哪些结构

实测，一个循环并调用一个辅助函数的函数，在两个等级下编译：

| 等级 | 清单显示什么 |
|---|---|
| 不优化 | **符号 `<check>:` 在**，循环看得见 |
| 优化 | **没有那个符号** —— 那次调用被内联进了调用方 |

> **一个函数边界是编译器做的一种安排，不是这个程序的性质。一个被内联的函数在反汇编里不存在，而它的指令在。**

**这就是基础篇那处实测在反汇编这一侧的版本** —— 那里一次调用变成了一个常数：**清单的结构是编译器的决定，而行为是指令的** —— 所以一个依赖函数边界的读法，依赖的是一个构建参数。

### 第八部分：从这些机制推出的安全观念

**任何一个关于二进制的视角都是片面的，而那些片面彼此互补。** 实测，一次线性扫描从一张数据表里产出了一个看起来说得通的函数前言；异常表在毫无符号的情况下给出了五个函数范围；动态符号点出了库调用；而字符串表连着它的指针描述了程序的词汇。**所以方法就是收集其中几样、然后互相核对**，而只从一份清单得出的结论，是关于那份清单的结论。

**而剥符号不是隐藏。** 实测，函数范围活在了回溯器的那张表里、库调用活成了带名字的导入、字面量连着指向它们的表一起活了下来。**所以一个为了加大分析难度而剥了符号的二进制，是一个结构仍然被三节留在原地的内容描述着的二进制** —— 因为把那些拿掉会弄坏这个程序需要的回溯与链接。

**而扫描方式的选择决定了一个读法朝哪个方向错。** 实测，线性扫描把数据解码成了指令，**而另一种看不到它没有沿控制流到达的目标** —— 所以知道一份清单是哪一种产出的，比在两者之间有个偏好更有用。

**而在函数是从一张表里被调用的时候，地址不在指令里。** 实测，那些字符串的地址住在一张指针表里，而代码通过一次相对载入够到它，**所以在清单里搜一个值，会在"值被存放的地方"找到它、而不是在"值被使用的地方"** —— 而这个搜索的方向，就是"找到一个结构"与"什么都没找到"之间的差别。

**而源码行是另一件产物。** 实测，一个地址只在带有那些信息的构建里解析出文件与行号，**而剥了符号的构建什么也解析不出来** —— 这就是为什么一份没有配套符号文件的崩溃报告是一个地址、而不是一个位置。

### 检测与缓解

- **记下这份清单是从哪个起点、用哪种扫描产出的**，因为实测那次线性扫描把一张数据表解码成了一个函数前言。
- **从剥符号的二进制恢复函数边界时，优先用回溯器的那张表而不是启发式**，因为实测那些范围是精确的，而靠前言特征去猜不是。
- **把动态符号与导入表当地标用**，因为实测那些库调用保持着名字，而它们周围的代码没有。
- **去搜一个值被存放的地方，而不是它被使用的地方**，因为实测那些地址住在一张指针表里，而代码通过一次相对载入够到它。
- **先读字面量以及指向它们的那些表**，因为它们在还没读懂任何代码之前就描述了程序大致是关于什么的。
- **保留与发布版匹配的那份符号文件**，因为实测那次解析到源码行依赖的是发布版所没有的信息。
- **在推理函数边界之前先确认这个二进制是否被优化过**，因为实测一个函数被内联之后就没有符号了、而它的指令还在。
- **对有争议的一段用两个反汇编器对照**，因为两者会解码同样的字节，而一致只会缩小可能性、不会确认意图。
- **不要把剥符号当成一种控制**，因为实测函数范围、导入与字面量都活了下来。
- **并且写下每一条结论来自哪一个片面视角**，因为实测那些视角朝相反方向出错，而一条结论会继承它那个来源的局限。
