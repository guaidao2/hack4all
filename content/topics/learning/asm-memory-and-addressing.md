---
id: asm-memory-and-addressing
title_en: Memory and Addressing
title_zh: 内存与寻址
summary_en: Memory is a set of numbered bytes whose permissions the processor enforces on every access, and addressing is a small algebra the hardware performs as part of a load. Measured in a real process — one file mapped with three different permissions, the same bytes faulting and then returning a value once the page becomes executable, and a base address that changes between runs while the low bits do not.
summary_zh: 内存是一组带编号的字节，而它的权限由处理器在每一次访问上执行；寻址则是硬件在载入时顺手完成的一小套代数。这一篇在一个真实进程里实测 —— 同一个文件被映射成三种权限、同一串字节先故障、在页面变成可执行之后返回一个值，以及每次运行都会改变而低位不变的基础地址。
tags: [beginner, asm, x86-64, memory, aslr]
tools: [nasm, gcc, objdump, gdb]
attck: [T1027]
platform: [linux]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Numbered bytes, and a small algebra over them

Two ideas carry this entry. **Memory is a set of numbered bytes and nothing else** — no types, no boundaries, no notion of what is stored where, only the question of whether an access is allowed. **And addressing is arithmetic the hardware performs as part of a load**, which is why one instruction can express "the eighth element of an array reached through a pointer".

> **The same bytes fault or return a value depending on the permissions of the page they are on. So whether data can be executed is a question about a page table, not about the data.**

Measured below in a real process, with the kernel's own report of the layout.

### Part 1: why there is an addressing syntax at all

**Arithmetic happens in registers**, so a value in memory must be loaded before it can be used — **which would make reaching the thousandth element of an array two instructions at best**: compute the address, then load it.

**And that pattern is frequent enough that the hardware performs it.** An address operand can name a base register, an index register, a scale and a constant, **and the processor computes the sum as part of the access** — so one instruction does the arithmetic and the load together.

**And the same arithmetic is available without the load**, which is how compiled code ends up full of instructions that compute addresses and never touch memory — the observation from the fundamentals entry, now with the encoding that makes it one byte cheaper.

### Part 2: what a process actually gets

Measured, addresses printed by a running program and the kernel's own description of them:

| What | Address |
|---|---|
| a function | `0x401196` |
| a string literal | `0x402008` |
| an initialised global | `0x404050` |
| an uninitialised global | `0x404058` |
| memory from the allocator | `0x23f8a010` |
| a local variable | `0x7ffe5f29ca20` |

**And the same program's own view of its mappings**:

```
00400000-00401000 r--p  ...  the program
00401000-00402000 r-xp  ...  the program
00402000-00403000 r--p  ...  the program
00403000-00404000 r--p  ...  the program
00404000-00405000 rw-p  ...  the program
23f8a000-23fab000 rw-p       [heap]
7f1888dc2000-...   r-xp      the library
7ffe5f27e000-...   rw-p       [stack]
```

**One file mapped five times, with three different sets of permissions.** The code is in the executable mapping, the constants are in a readable one, and the variables are in a writable one.

> **The permission column is not documentation. It is the set of checks the processor performs on every access, and the layout is a description of which checks apply where.**

**And two rows deserve attention because they are where the interesting things live.** The heap and the stack are **readable and writable but not executable** — measured as `rw-p` — **so the data an attacker can write is, by default, not data that can be executed.**

### Part 3: the permissions are enforced, not described

Measured, the same program writing to two different places:

| Write | Result |
|---|---|
| to an initialised global | **succeeds** |
| to a string literal | **SIGSEGV** |

**And the executable half of the same rule, measured with a page the program allocated itself**:

| Page permission | Calling the bytes as code |
|---|---|
| readable and writable | **SIGSEGV** |
| the same page made executable | **returns 42** |

**The bytes were identical in both cases.** Only the mapping changed.

> **Whether a sequence of bytes can be executed is a property of the page it is on, not of its contents — which is why "data turning into code" is a change of permission before it is anything else.**

**And that is the reason exploitation took the shape it did.** If a writable page cannot be executed, then injecting instructions into a buffer and jumping to them requires a step that makes it executable — **and that step is a system call, which is visible.** The alternative is to reuse code that is already executable, which is what the later entries on that subject are about.

### Part 4: the address changes between runs

Measured, the same program executed twice:

| Build | Address of a function, two runs |
|---|---|
| position independent (the default) | `0x55705f37a159` then `0x55999aed6159` |
| built at a fixed base | `0x401146` then `0x401146` |

**And in the position-independent case the low bits do not change**: the two values end in the same three hexadecimal digits. **The base moves; the offset within the mapping does not.**

**And the stack and the heap move in both builds.** Measured, two runs of the fixed-base program gave the heap at `0x16256010` and `0x37704010`, with the stack differing as well — **so "the address is fixed" is a claim that holds for the code segment in one build and for nothing else.**

> **What is randomised is the base, not the offset within a mapping. So "the low twelve bits are stable" is a usable fact and "the address is stable" is not.**

**Which is the practical version of the concept**: an address written down in a payload is a value that depends on the build and on the current boot, **and the part of it that can be relied on is only the part that does not move.**

### Part 5: the syntax and its encoding

Measured, the same load written with progressively more of the address form:

| Form | Encoding | Bytes |
|---|---|---|
| `mov rax, [rbx]` | `48 8B 03` | 3 |
| `mov rax, [rbx+8]` | `48 8B 43 08` | 4 |
| `mov rax, [rbx+rcx]` | `48 8B 04 0B` | 4 |
| `mov rax, [rbx+rcx*8]` | `48 8B 04 CB` | 4 |
| `mov rax, [rbx+rcx*8+16]` | `48 8B 44 CB 10` | 5 |
| **`lea rax, [rbx+rcx*8+16]`** | **`48 8D 44 CB 10`** | **5** |
| `mov eax, [0x404000]` | `8B 04 25 00 40 40 00` | 7 |
| `mov rax, [0x404000]` | `48 8B 04 25 00 40 40 00` | 8 |
| `lea rax, [rel msg]` | `48 8D 05 07 00 00 00` | 7 |
| `mov rax, [rel msg]` | `48 8B 05 00 00 00 00` | 7 |

**Reading down the table**: adding a displacement costs a byte, adding an index costs a byte, adding a scale costs nothing, **and turning the load into an address computation costs exactly one** — the difference between the two opcodes.

**And the last two rows are the same length**, which is why the relative form is not a size optimisation but a requirement in position-independent code: **the address is expressed as a distance from the instruction rather than as a number**, so the instruction is correct wherever the mapping lands.

### Part 6: relative addressing is what makes position independence possible

**If the code can be mapped anywhere, then a global variable's address is not known when the instruction is written.** Measured, the form the compiler emits for one:

```
lea    rax, [rip+0x7]        # the assembler even resolves the distance
```

**And the same instruction is emitted whether or not the build is position independent** — what changes is whether anything else can assume a fixed base.

> **The distance between an instruction and the data it uses does not change when the whole mapping moves, so an address written as a distance stays correct — and that is what position independence is built on, rather than being an optimisation for it.**

**Which explains the fourth part's measurement**: the low bits of an address are stable because **the offset inside a mapping is a compile-time number**, while the base is decided at run time.

### Part 7: the scale has four values

**The hardware provides scale factors of one, two, four and eight** — the sizes of the primitive types — and nothing else. Measured, a structure of **twelve bytes**, which is not among them:

| Element | Address | Offset |
|---|---|---|
| the first | `0x7ffe411d53a8` | +0 |
| the second | `0x7ffe411d53b4` | **+12** |
| the third | `0x7ffe411d53c0` | **+24** |
| the fourth | `0x7ffe411d53cc` | **+36** |

**The steps are twelve**, and measured, the compiler computed that multiply in two operations:

```
lea    rax, [rsi+rsi*2]          ; the index times three
mov    eax, [rdi+rax*4+0x8]      ; then times four, plus the field offset
```

**Twelve written as three times four**, because each of those is something the addressing hardware already does.

> **The scale has four values, so an element size outside them is built from operations the hardware does have — and the compiler's choice of decomposition is visible in the instructions.**

### Part 8: what follows for security

**Memory safety has two faces and the hardware separates them.** Measured, the same bytes faulted when written through a read-only mapping and faulted when called through a non-executable one, **while the same bytes on a writable mapping were fine and returned a value once the mapping was made executable.** So a boundary violation and a code-execution question are two different checks — **and a design that reasons about one while ignoring the other has reasoned about half.**

**And the executable check is why injected instructions are not enough.** Measured, a writable page could not be called, **and the operation that would have allowed it is a system call that changes the page's permissions** — which is both visible and a step in the chain that has to be explained. The alternative, reusing instructions that are already executable, is what the later entries cover.

**And addresses are stable in a way that is easy to overstate.** Measured, a position-independent build gave a different function address on every run while the last three hex digits stayed put, **and a fixed-base build gave the same function address twice while the heap and stack still moved.** So the reliable part of an address is its offset inside a mapping, **and anything written against a full address is written against one build on one machine.**

**And the relative form is a requirement rather than a size choice.** Measured, the two relative encodings were the same length as the absolute ones, **so the reason to use them is that the distance survives the mapping moving** — which is also why the offset inside a mapping is a compile-time constant and the base is not.

**And the addressing form is the compiler's toolkit for arithmetic.** Measured, a scale of twelve became a multiply by three and then by four. **So a load can be doing two or three arithmetic operations**, and reading it as "fetch this" loses most of what it says.

### Detection and mitigation

- **Read the permission column of a process's mappings when auditing**, since the measured layout had one file in three different permission sets and two writable regions with no execute permission.
- **Report any mapping that is both writable and executable**, because the measured execute check is a property of the mapping and nothing else.
- **Check whether position independence and address randomisation are enabled for a build**, since the measured fixed-base build kept its code address across runs while the heap and the stack moved.
- **Do not write an exploit or a patch around a full address**, because the measured stable part is the offset within a mapping and the base is decided at run time.
- **Treat a change of page permissions as a step to account for**, since the measured writable page could not be executed until a system call said so.
- **Use offsets rather than addresses when describing a location inside a module**, since they survive rebuilds and randomisation while addresses do not.
- **Read an address operand as arithmetic**, because the measured form with base, index, scale and displacement is several operations in one instruction.
- **Remember that the scale takes four values**, so the measured twelve-byte stride became two instructions and a stride that looks simple in the source may be several in the binary.
- **For mitigation, keep read-only data in a read-only mapping**, since the measured write to a literal faulted rather than silently succeeding.
- **And treat the heap and the stack as non-executable unless something has changed that**, since the measured mappings were writable without being executable — and the exception is a decision worth recording.

<!-- lang:zh -->
### 带编号的字节，以及它们上面的一小套代数

两个想法承载这一篇。**内存是一组带编号的字节、别的什么都不是** —— 没有类型、没有边界、没有"什么存在哪里"的概念，只有"这次访问允不允许"这个问题。**而寻址是硬件在载入时顺手完成的一次算术**，这就是为什么一条指令能表达"通过一个指针够到数组的第八个元素"。

> **同一串字节是故障还是返回一个值，取决于它所在页面的权限。所以"数据能不能被执行"是一个关于页表的问题，不是关于数据的问题。**

下面在一个真实进程里实测，用的是内核自己给出的布局报告。

### 第一部分：为什么需要一套寻址语法

**算术在寄存器里做**，所以内存里的一个值要先被载入才能用 —— **那会让"够到数组的第一千个元素"至少变成两条指令**：算出地址，再载入。

**而这个模式足够常见，以至于硬件自己去做它。** 一个地址操作数可以点名一个基址寄存器、一个索引寄存器、一个比例和一个常量，**而处理器在访问过程中把这个和算出来** —— 于是一条指令同时做了算术和载入。

**而同一套算术不载入也能用**，这就是为什么编译出来的代码里全是"算地址、根本不碰内存"的指令 —— 那是基础篇的观察，现在有了让它便宜一个字节的编码。

### 第二部分：一个进程真正拿到什么

实测，一个运行中的程序打印出的地址，以及内核自己对这些地址的描述：

| 什么 | 地址 |
|---|---|
| 一个函数 | `0x401196` |
| 一个字符串字面量 | `0x402008` |
| 一个已初始化的全局变量 | `0x404050` |
| 一个未初始化的全局变量 | `0x404058` |
| 分配器给的内存 | `0x23f8a010` |
| 一个局部变量 | `0x7ffe5f29ca20` |

**而同一个程序对自己映射的看法**：

```
00400000-00401000 r--p  ...  这个程序
00401000-00402000 r-xp  ...  这个程序
00402000-00403000 r--p  ...  这个程序
00403000-00404000 r--p  ...  这个程序
00404000-00405000 rw-p  ...  这个程序
23f8a000-23fab000 rw-p       [heap]
7f1888dc2000-...   r-xp      库
7ffe5f27e000-...   rw-p       [stack]
```

**一个文件被映射了五次，带着三套不同的权限。** 代码在可执行的映射里、常量在一个可读的里、变量在一个可写的里。

> **权限那一列不是文档。它是处理器在每一次访问上执行的那组检查，而那份布局是一份"哪些检查适用于哪里"的描述。**

**而有两行值得注意，因为有意思的东西就住在那里。** 堆与栈是**可读可写但不可执行**的 —— 实测为 `rw-p` —— **所以攻击者能写进去的数据，默认不是能被执行的代码。**

### 第三部分：权限是被执行的，不是被描述的

实测，同一个程序往两个不同的地方写：

| 写入 | 结果 |
|---|---|
| 写一个已初始化的全局变量 | **成功** |
| 写一个字符串字面量 | **SIGSEGV** |

**以及同一条规则可执行的那一半**，用一个程序自己分配的页面测：

| 页面权限 | 把那些字节当代码调用 |
|---|---|
| 可读可写 | **SIGSEGV** |
| 同一个页面加上执行权限 | **返回 42** |

**两种情况下字节完全相同。** 变的只是映射。

> **一串字节能不能被执行，是它所在页面的性质、不是它内容的性质 —— 这就是为什么"数据变成代码"首先是一次权限变更。**

**而这就是利用技术长成今天这个形状的原因。** 如果可写页不能执行，那把指令注入一块缓冲区再跳过去就需要一步把它变成可执行 —— **而那一步是一次系统调用，它是看得见的。** 另一条路是复用已经可执行的代码，那正是后面讲这个主题的篇目要处理的。

### 第四部分：地址在两次运行之间会变

实测，同一个程序执行两次：

| 构建 | 两次运行里某个函数的地址 |
|---|---|
| 位置无关（默认） | `0x55705f37a159` 然后 `0x55999aed6159` |
| 固定在某个基址构建 | `0x401146` 然后 `0x401146` |

**而在位置无关那种情况下，低位不变**：两个值都以同样的三位十六进制数结尾。**动的是基址；映射内部的偏移没动。**

**而栈与堆在两种构建里都会动。** 实测，那个固定基址的程序两次运行的堆分别是 `0x16256010` 与 `0x37704010`，栈也不同 —— **所以"地址是固定的"这句话，在一种构建里对代码段成立、对其他任何东西都不成立。**

> **被随机化的是基址，不是映射内部的偏移。所以"低十二位是稳定的"是一个可用的事实，而"地址是稳定的"不是。**

**这就是那个概念在实践里的版本**：一个写在攻击载荷里的地址，是一个取决于构建、也取决于这一次启动的值，**而其中能依赖的部分，只有那部分它不动的部分。**

### 第五部分：语法与它的编码

实测，同一个载入逐步加上地址形式的各个部分：

| 形式 | 编码 | 字节 |
|---|---|---|
| `mov rax, [rbx]` | `48 8B 03` | 3 |
| `mov rax, [rbx+8]` | `48 8B 43 08` | 4 |
| `mov rax, [rbx+rcx]` | `48 8B 04 0B` | 4 |
| `mov rax, [rbx+rcx*8]` | `48 8B 04 CB` | 4 |
| `mov rax, [rbx+rcx*8+16]` | `48 8B 44 CB 10` | 5 |
| **`lea rax, [rbx+rcx*8+16]`** | **`48 8D 44 CB 10`** | **5** |
| `mov eax, [0x404000]` | `8B 04 25 00 40 40 00` | 7 |
| `mov rax, [0x404000]` | `48 8B 04 25 00 40 40 00` | 8 |
| `lea rax, [rel msg]` | `48 8D 05 07 00 00 00` | 7 |
| `mov rax, [rel msg]` | `48 8B 05 00 00 00 00` | 7 |

**顺着这张表往下读**：加一个位移花一个字节，加一个索引花一个字节，加一个比例不花，**而把"载入"变成"算地址"恰好花一个** —— 两个操作码之间的差别。

**而最后两行长度相同**，这就是为什么相对形式不是一个体积优化、而是位置无关代码里的一条要求：**地址被表达为相对于本条指令的一段距离、而不是一个数字**，所以这条指令落在哪里都是对的。

### 第六部分：相对寻址是位置无关之所以可能的原因

**如果代码可以被映射到任何地方，那么一个全局变量的地址在写下这条指令时是不知道的。** 实测，编译器为此发出的形式：

```
lea    rax, [rip+0x7]        # 汇编器甚至把距离直接算出来了
```

**而无论构建是否位置无关，发出的都是同一条指令** —— 变的是有没有别的东西可以假定一个固定基址。

> **当整个映射移动时，一条指令与它所用数据之间的距离不变，所以写成一个距离的地址仍然正确 —— 位置无关建立在这件事上，而不是"为了它做的优化"。**

**这就解释了第四部分那组实测**：一个地址的低位之所以稳定，是因为**映射内部的偏移是一个编译期的数字**，而基址是运行时决定的。

### 第七部分：比例只有四个取值

**硬件提供 1、2、4、8 四种比例** —— 也就是那几种基本类型的大小 —— 别的没有。实测，一个**十二字节**的结构体，它不在其中：

| 元素 | 地址 | 偏移 |
|---|---|---|
| 第一个 | `0x7ffe411d53a8` | +0 |
| 第二个 | `0x7ffe411d53b4` | **+12** |
| 第三个 | `0x7ffe411d53c0` | **+24** |
| 第四个 | `0x7ffe411d53cc` | **+36** |

**步进是十二**，而实测，编译器用两次操作把它算了出来：

```
lea    rax, [rsi+rsi*2]          ; 索引乘以三
mov    eax, [rdi+rax*4+0x8]      ; 再乘以四，加上字段偏移
```

**十二被写成三乘四**，因为这两步都是寻址硬件本来就会做的事。

> **比例只有四个取值，所以一个不在其中的元素大小，是用硬件已有的操作拼出来的 —— 而编译器选择怎么拆，在指令里看得见。**

### 第八部分：从这些机制推出的安全观念

**内存安全有两张脸，而硬件把它们分开了。** 实测，同一串字节通过一个只读映射去写会故障、通过一个不可执行的映射去调用也会故障，**而同样的字节在一个可写映射上是好的、并且在那张映射被改成可执行之后返回了一个值。** 所以"越界"与"能不能执行"是两个不同的检查 —— **而一个只推理其中一个、无视另一个的设计，只推理了一半。**

**而那个可执行的检查，就是"注入指令"为什么不够。** 实测，一个可写页调用不了，**而那个本可以允许它的操作是一次改变页面权限的系统调用** —— 它既看得见，也是链条上必须被解释的一步。另一条路是复用已经可执行的指令，那是后面篇目要讲的。

**而地址的稳定性很容易被说过头。** 实测，一个位置无关的构建每次运行给出的函数地址都不同、而最后三位十六进制数没动，**而一个固定基址的构建两次给出同一个函数地址、堆与栈却照样移动。** 所以一个地址里可靠的部分是它在映射内部的偏移，**而任何按完整地址写下的东西，是照着某一台机器上的某一份构建写的。**

**而相对形式是一条要求、不是一个体积选择。** 实测，两种相对编码与绝对编码长度相同，**所以使用它们的理由是那段距离在映射移动之后仍然成立** —— 这也是为什么映射内部的偏移是编译期常量、而基址不是。

**而寻址形式是编译器做算术的工具箱。** 实测，一个为十二的比例变成了先乘三再乘四。**所以一条载入指令可能同时在做两三次算术运算**，而把它读成"取这个"会丢掉它说的大部分内容。

### 检测与缓解

- **审一个进程的映射时读权限那一列**，因为实测那份布局里一个文件处于三套不同的权限之下，而两个可写区域没有执行权限。
- **报出任何既可写又可执行的映射**，因为实测那个执行检查是映射的性质、而不是别的什么。
- **检查一份构建是否开了位置无关和地址随机化**，因为实测那个固定基址的构建跨运行保持了它的代码地址，而堆与栈都在动。
- **不要围绕一个完整地址去写利用或者补丁**，因为实测其中稳定的部分是映射内部的偏移，而基址是运行时决定的。
- **把页面权限的变更当成一步要交代的事情**，因为实测那个可写页在系统调用发话之前无法被执行。
- **描述一个模块内部的某个位置时用偏移而不是地址**，因为它们能活过重建与随机化，而地址不能。
- **把一个地址操作数读成算术**，因为实测那个带基址、索引、比例与位移的形式是一条指令里的好几种运算。
- **记住比例只有四个取值**，所以实测那个十二字节的步进变成了两条指令，而源码里看着简单的步进在二进制里可能是好几个。
- **缓解上，把只读数据放进只读映射**，因为实测对字面量的写入是故障、而不是悄悄成功。
- **并且把堆与栈当成不可执行，除非有东西改了这一点**，因为实测那些映射是可写而不可执行的 —— 而例外的情形是一个值得记录下来的决定。
