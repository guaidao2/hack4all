---
id: asm-control-flow-and-syscalls
title_en: Control Flow and System Calls
title_zh: 控制流与系统调用
summary_en: There is no conditional instruction, only comparisons that set flags and jumps that read them, which means the type of a comparison lives in the choice of jump rather than in the flags — and a system call uses a register convention of its own that consumes two registers the function convention does not. Measured with real programs and real syscalls.
summary_zh: 指令集里没有"如果"，只有设置标志的比较和读标志的跳转，这意味着"这次比较是什么类型"住在"选了哪条跳转"里、而不在标志里；而系统调用用一套它自己的寄存器约定，会吃掉函数约定不会碰的两个寄存器。这一篇用真实的程序与真实的系统调用实测。
tags: [beginner, asm, x86-64, control-flow, syscalls]
tools: [nasm, gcc, objdump, strace]
attck: [T1027]
platform: [linux]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### No conditional instruction, only flags and jumps

A processor has no "if". It has comparisons that leave their result in flags and jumps that read those flags — and that indirection is where the interesting properties come from.

> **Signed and unsigned comparisons set the same flags and read them differently, so the type of a comparison is encoded in which jump was chosen rather than in the flags themselves. And a system call takes its fourth argument from a different register than a function call does, because it consumes two registers for its own purposes.**

Both measured below on real programs and real system calls.

### Part 1: why control flow is built on flags

**An instruction set that had a conditional instruction for every comparison would need a large, redundant family of them**: equality, ordering, signed, unsigned, and combinations. **Setting the result aside in a small set of bits and then reading them is a decomposition that keeps the instruction set small.**

**And the cost is that the flags are shared state.** Two different branches can read the same flag, **so the correctness of a conditional depends on which instruction last touched the flag it reads** — the property measured in the fundamentals entry, where an increment left the carry flag alone and an addition of zero changed it.

**And a further consequence is that the meaning of a comparison is not in the comparison.** The same subtraction produces the same flags regardless of whether the author meant signed or unsigned arithmetic; **what differs is the jump chosen afterwards.**

### Part 2: the same comparison, read two ways

Measured, a function testing "greater than" written twice — once signed and once unsigned — with it called on a negative value:

| Source | Instructions | Result of comparing -1 with 1 |
|---|---|---|
| signed | `cmp` then **`setg`** | **0 — not greater** |
| unsigned | `cmp` (operands swapped) then **`setb`** | **1 — greater** |

**One bit pattern, two meanings, two opposite answers.** And the compiler's choice for the unsigned case is worth noticing on its own: it **swapped the operands and asked whether the first is below the second**, which is the same statement written with the other condition code.

> **Signed and unsigned branches read the same flags differently, so the type of a comparison is encoded in the choice of jump rather than in the flags. A reader who assumes the obvious jump can invert a check without touching the flags.**

**And that is not a theoretical distinction.** A length or a count is naturally unsigned, **and a check written as "less than zero" is a check that never fires** — the condition is not unlikely, it is impossible, and the code around it looks correct in every other respect.

**And the condition codes themselves are a short table**, measured with the encodings the assembler produced:

| Mnemonic | Meaning | Condition |
|---|---|---|
| `je` / `jz` | equal / zero | zero flag set |
| `jg` / `jge` / `jl` / `jle` | **signed** ordering | sign and overflow |
| `ja` / `jae` / `jb` / `jbe` | **unsigned** ordering | carry and zero |
| `js` / `jo` / `jc` | sign, overflow, carry | one flag each |

**And the family is smaller than it looks**: measured, `jz` and `je` assembled to the same two bytes, as did `jnz` and `jne`. **So a disassembler prints one of the names and the choice is a convention rather than a fact about the bytes.**

### Part 3: many branches become a table

Measured, a function selecting among eight cases and a default, compiled and linked:

```
cmp    edi, 0x7
ja     <default>
mov    edi, edi
lea    rdx, [rip+0xed0]          # the table
movsxd rax, DWORD PTR [rdx+rdi*4]
add    rax, rdx
jmp    rax
```

**Three things are happening in those six instructions.** A **range check** that sends everything outside the cases to the default, the **table lookup** with the index scaled by four, and an **indirect jump** whose target is computed from the value.

**And the range check uses an unsigned jump.** Measured, `ja` above the highest case — **which catches both a value above the range and a negative value**, since a negative index is an enormous unsigned number. **One comparison, two failure modes covered.**

**And the table's contents are not addresses.** Measured, the bytes in the read-only section:

```
39 f1 ff ff   69 f1 ff ff   3f f1 ff ff   ...
```

**Each entry is a signed 32-bit offset, and the instruction that reads it adds the table's own address** — so the entries are **relative to the table** rather than to the start of the program.

> **A jump table stores offsets rather than addresses, so it works wherever the code is mapped — the same reason relative addressing is used for data.**

**Which is the position-independence measurement from the memory entry appearing in a third place**: the code does not know where it will be loaded, **so every reference to another location inside it is written as a distance.**

### Part 4: indirect jumps, and where a target comes from

**A direct jump carries its target in the instruction.** **An indirect one computes it at run time** — measured above as `jmp rax`, where the value came from a table indexed by the input.

> **A direct jump's destination is part of the instruction; an indirect jump's destination is data. So "where does this code go next" is answered by reading the instruction in one case and by following a value in the other.**

**And the table measured here is a legitimate instance of data deciding control flow**, which is worth saying plainly because the same description is how a control-flow hijack is usually characterised. **What separates them is the range check and the fact that the table is in a read-only mapping** — measured as such in the memory entry.

**And the difference between the two forms is also an encoding difference** — a direct jump is a fixed few bytes, an indirect one is shorter but says nothing about its destination, **which is why a disassembler's output for an indirect jump is a register rather than a label.**

### Part 5: a system call is a different convention

**The function-calling convention is an agreement between compilers**, and it exists so that separately compiled code can call itself. **A system call is an agreement between user code and the kernel** — a different pair of parties, with a different history, **and there is no reason for the two to be the same.**

**And they are not.** Measured, the registers a system call touches for its own purposes:

| Register | Before | After |
|---|---|---|
| `rcx` | `0x1111111111111111` | **`0x000055bdacae521b` — an address** |
| `r11` | `0x2222222222222222` | **`0x0000000000000202` — the flags** |

**Both were overwritten**, and the values say why: **the instruction has to save where to return to and the state of the flags, and it uses those two registers to do it.**

> **`syscall` consumes `rcx` and `r11`, so the fourth argument arrives in `r10` rather than in `rcx` — and any code that wants those registers across a system call has to preserve them itself.**

**And the argument order is otherwise recognisable**: the first three in the same registers a function call would use, the fourth moved to the register the instruction did not need. **Measured, a wrapper for a system call takes its fourth argument as a function argument in `rcx` and moves it to `r10` before the instruction.**

**And the number of the system call goes in a register too**, which is what makes a system call a function of its number rather than of an instruction per operation — **and what makes the set of them observable from outside**, as the tools that trace them demonstrate.

### Part 6: errors are return values, and two layers represent them

Measured, opening a file that does not exist, once through the instruction directly and once through the library:

| Path | Returned |
|---|---|
| the `syscall` instruction, unmodified | **`-2`, unsigned `0xfffffffffffffffe`** |
| the library's function | **`-1`, with the error number set separately** |

**And the negative value is the error number negated** — measured, `-2` corresponds to "no such file or directory", which is the same number the library reported.

> **The kernel reports a failure by returning a negative error number. The library converts that into a failure indicator plus a separate variable — so "the call failed" has two representations depending on which layer is being read.**

**Which matters because the two are not interchangeable.** Code that reads a raw return has to test the sign; code that reads a library return has to test against the failure indicator and then consult the variable. **And the sign test is only valid inside the convention** — a successful call can return a large value, so "negative means error" is a statement about this interface rather than about numbers.

### Part 7: what the layer gives a reader

**Control flow here is a pair**: an instruction that sets flags and an instruction that reads them, **so reading a branch means finding the last thing that touched the flag it reads.** Measured in the fundamentals entry, an increment and an addition of zero differ in exactly that respect.

**And the signed-or-unsigned decision is a property of the jump rather than of the comparison**, **so a review of a check has to look at which jump was chosen** — which is where the measured `-1 > 1` disagreement comes from.

**And targets come in two kinds**: in the instruction, or computed. **The second kind is where a value turns into a destination**, and the measured jump table shows that this is ordinary — with a range check in front of it and a read-only table behind it.

**And the boundary to the kernel is a single instruction**, which makes it both the narrowest place to observe what a process does and the place where a convention differs from the one the rest of the code follows.

### Part 8: what follows for security

**A check's correctness depends on a choice that is not visible in the comparison.** Measured, the same subtraction followed by two different condition codes produced opposite answers for a negative value, **and a range check written with an unsigned jump covered both an oversized value and a negative one in a single comparison.** So a review of a bounds check is a review of which jump was emitted.

**And "less than zero" on an unsigned value is not a weak check but an impossible one.** Measured, the signed reading of the same flags said no where the unsigned reading said yes. **Which makes a signedness mismatch a class of bug that no amount of testing the happy path will find.**

**And jumps whose target is computed are ordinary and are where control flow becomes data.** Measured, a jump table indexed by the input, with a range check in front and a read-only table behind. **So the question about an indirect jump is not whether one exists but where its target comes from and what constrains the index.**

**And the kernel boundary has its own convention.** Measured, the instruction overwrote two registers, one with a return address and one with the flags, **which is why the fourth argument is elsewhere** — so hand-written code around a system call has to treat those two registers as scratch whether or not the function convention does.

**And errors have two representations and only one of them is a sign.** Measured, the raw return was the negated error number and the library's was a failure indicator with a separate variable. **So code that mixes the two layers, or tests the wrong one, reads a failure as success** — and the type of a register is not something the instruction says.

### Detection and mitigation

- **Check the condition code on every bounds check**, since the measured signed and unsigned readings of one comparison disagreed, and an unsigned reading covered two failure modes at once.
- **Look for comparisons against zero on values that cannot be negative**, because the measured check would be impossible rather than merely unlikely.
- **Follow the index of every indirect jump back to its constraint**, since the measured table relied on a range check in front of it and a read-only table behind it.
- **Keep jump tables in a read-only mapping**, because the measured entries were data that decided control flow and the mapping they were in did not permit writing.
- **Trace the flag a conditional reads back to the instruction that set it**, since measured instructions differ in which flags they touch.
- **Treat the two registers a system call consumes as scratch** around hand-written interface code, because the measured instruction overwrote both.
- **Test a raw return for its sign and a library return against its failure indicator**, since the measured values were a negated error number and a separate variable respectively.
- **Prefer the library's interface where it exists**, because the conversion from a negative number to a failure indicator is the part that is easy to get wrong by hand.
- **Observe the kernel boundary rather than inferring it**, since every operation a process performs on anything outside itself passes through one instruction.
- **And read control flow as pairs of instructions**, because the measured difference between two similar instructions was exactly which flag they left alone.

<!-- lang:zh -->
### 没有条件指令，只有标志与跳转

处理器没有"如果"。它有的是把结果留在标志里的比较、以及读那些标志的跳转 —— 而有趣的性质都来自这层间接。

> **有符号与无符号的比较设置同一批标志、却用不同的方式读它们，所以"这次比较是什么类型"被编码在"选了哪条跳转"里，而不是在标志里。而一次系统调用从与函数调用不同的寄存器取它的第四个参数，因为它把两个寄存器用在了自己的事情上。**

两者都在下面用真实的程序与真实的系统调用实测。

### 第一部分：为什么控制流建在标志上

**一个为每一种比较都配一条条件指令的指令集，会需要一个庞大而冗余的家族**：相等、排序、有符号、无符号、以及它们的组合。**把结果放进一小撮位里、然后再去读，是一个让指令集保持小的分解。**

**而代价是标志成了共享状态。** 两个不同的分支可以读同一个标志，**所以一个条件的正确性取决于"最后碰到它所读那个标志的是哪条指令"** —— 那是基础篇实测过的性质：一次递增放着进位标志不管，而一次加零改了它。

**而进一步的后果是"一次比较的含义不在比较里"。** 同一次减法不管作者想的是有符号还是无符号都会产出同一批标志；**不同的是之后选了哪条跳转。**

### 第二部分：同一次比较，两种读法

实测，一个"大于"的判断写两遍 —— 一遍有符号、一遍无符号 —— 然后拿一个负值去调它：

| 源码 | 指令 | -1 与 1 比较的结果 |
|---|---|---|
| 有符号 | `cmp` 然后 **`setg`** | **0 —— 不大于** |
| 无符号 | `cmp`（操作数换了个位置）然后 **`setb`** | **1 —— 大于** |

**一个位模式、两种含义、两个相反的答案。** 而编译器为无符号那种选的做法本身就值得注意：它**把两个操作数换了个位置，然后问"第一个是不是低于第二个"** —— 那是同一句话换了另一个条件码来写。

> **有符号与无符号的分支用不同方式读同一批标志，所以一次比较的类型被编码在跳转的选择里、而不是在标志里。一个想当然地读"那条明显的跳转"的人，可以在完全不碰标志的情况下把一个检查反过来。**

**而这不是一个理论上的区别。** 一个长度或者计数天然是无符号的，**而一个写成"小于零"的检查是一个永远不会触发的检查** —— 那个条件不是不太可能，而是不可能，而它周围的代码在别的每一处看起来都对。

**而条件码本身就是一张很短的表**，下面是用汇编器产出的编码实测的：

| 助记符 | 含义 | 条件 |
|---|---|---|
| `je` / `jz` | 相等 / 为零 | 零标志置位 |
| `jg` / `jge` / `jl` / `jle` | **有符号**排序 | 符号与溢出 |
| `ja` / `jae` / `jb` / `jbe` | **无符号**排序 | 进位与零 |
| `js` / `jo` / `jc` | 符号、溢出、进位 | 各一个标志 |

**而这个家族比看起来要小**：实测，`jz` 与 `je` 汇编出来是同样的两个字节，`jnz` 与 `jne` 也一样。**所以反汇编器只会打印其中一个名字，而那个选择是一个约定、不是关于字节的事实。**

### 第三部分：很多个分支会变成一张表

实测，一个在八个分支加一个默认之间做选择的函数，编译并链接之后：

```
cmp    edi, 0x7
ja     <default>
mov    edi, edi
lea    rdx, [rip+0xed0]          # 表
movsxd rax, DWORD PTR [rdx+rdi*4]
add    rax, rdx
jmp    rax
```

**那六条指令里在发生三件事。** 一次**范围检查**把落在分支之外的一切送到默认分支、一次**表查询**（索引按四缩放）、以及一次目标由那个值算出来的**间接跳转**。

**而那次范围检查用的是一条无符号跳转。** 实测，`ja` 跳向"高于最大的那个分支" —— **而它同时抓住了一个超出范围的值与一个负值**，因为一个负的索引是一个巨大的无符号数。**一次比较，两个失败模式一起覆盖了。**

**而表的内容不是地址。** 实测，只读段里的字节：

```
39 f1 ff ff   69 f1 ff ff   3f f1 ff ff   ...
```

**每一项都是一个有符号的 32 位偏移，而读它的那条指令会把表自己的地址加上去** —— 所以这些项是**相对于表的**，而不是相对于程序起点。

> **跳转表存的是偏移而不是地址，所以它不管代码被映射到哪里都能用 —— 与数据使用相对寻址是同一个理由。**

**这就是内存那一篇的位置无关实测出现在第三个地方**：代码不知道它会被载入到哪里，**所以它内部对另一个位置的每一次引用都被写成一个距离。**

### 第四部分：间接跳转，以及目标从哪里来

**直接跳转把目标带在指令里。** **间接跳转在运行时把它算出来** —— 上面实测为 `jmp rax`，那个值来自一张被输入索引的表。

> **直接跳转的去处在指令里；间接跳转的去处是数据。所以"这段代码接下来去哪"这个问题，在一种情况下靠读指令回答、在另一种情况下靠追一个值回答。**

**而这里实测的这张表是"数据决定控制流"的一个正当实例**，这值得直说，因为同一句描述也是控制流被劫持通常的刻画方式。**把两者分开的是一个范围检查、以及那张表在一张只读映射里** —— 内存那一篇实测过它是只读的。

**而两种形式的差别也是一个编码差别** —— 直接跳转是固定的几个字节，间接跳转更短却对它的去处在字面上什么都没说，**这就是为什么反汇编器对一条间接跳转打印出来的是一个寄存器、而不是一个标签。**

### 第五部分：系统调用是另一套约定

**函数调用约定是编译器之间的约定**，它存在的意义是让分开编译的代码能互相调用。**系统调用是用户代码与内核之间的约定** —— 另外一对当事人、另一段历史，**而两者没有理由相同。**

**而它们确实不同。** 实测，一次系统调用为了自己的事情用掉的那些寄存器：

| 寄存器 | 调用前 | 调用后 |
|---|---|---|
| `rcx` | `0x1111111111111111` | **`0x000055bdacae521b` —— 一个地址** |
| `r11` | `0x2222222222222222` | **`0x0000000000000202` —— 标志** |

**两个都被覆盖了**，而那些值说明了原因：**这条指令必须把"回哪里"和标志的状态存下来，而它就是用这两个寄存器做的。**

> **`syscall` 用掉了 `rcx` 与 `r11`，所以第四个参数从 `r10` 到达而不是从 `rcx` —— 而任何想跨过一次系统调用继续使用这两个寄存器的代码，都必须自己保存它们。**

**而参数顺序在别的方面是认得出的**：前三个在函数调用会用的同样那些寄存器里，第四个被移到了那条指令不需要的那个寄存器上。**实测，一个系统调用的包装函数把它的第四个参数按函数约定收在 `rcx`，然后在指令之前移到 `r10`。**

**而系统调用的编号也放在一个寄存器里**，这正是为什么一次系统调用是"它的编号的函数"、而不是每个操作一条指令 —— **也是为什么这一整组操作可以从外部观察**，那些追踪它们的工具就演示了这一点。

### 第六部分：错误是返回值，而两层各有自己的表示

实测，打开一个不存在的文件，一次经由那条指令、一次经由库：

| 路径 | 返回 |
|---|---|
| `syscall` 指令，未经修改 | **`-2`，无符号是 `0xfffffffffffffffe`** |
| 库里的函数 | **`-1`，错误号被设在另一个地方** |

**而那个负值就是错误号取负** —— 实测，`-2` 对应"没有这个文件"，与库报出的那个数字是同一个。

> **内核用一个负的错误号来报告失败。库把它转换成一个失败指示加上一个单独的变量 —— 所以"这次调用失败了"有两种表示，取决于在读哪一层。**

**这要紧，因为两者不能互换。** 读原始返回值的代码要测符号；读库返回值的代码要对着失败指示去测、然后去查那个变量。**而符号那个判据只在约定之内成立** —— 一次成功的调用可以返回一个很大的值，所以"负数是错误"是一句关于这个接口的话、不是关于数字的话。

### 第七部分：这一层给读的人什么

**这里的控制流是一对东西**：一条设置标志的指令与一条读它们的指令，**所以读一个分支意味着找出最后碰过它所读那个标志的东西。** 基础篇实测过，一次递增与一次加零恰好在这件事上不同。

**而"有符号还是无符号"是跳转的性质、不是比较的性质**，**所以检查一个边界判断必须看它选的是哪条跳转** —— 实测那个 `-1 > 1` 的分歧就从这里来。

**而目标分两种**：在指令里，或者被算出来。**第二种是"一个值变成去处的"地方**，而实测那张跳转表表明这件事很平常 —— 前面有一个范围检查，后面有一张只读的表。

**而通向内核的边界是一条指令**，这既让它成为观察一个进程做了什么的最窄的地方，也让它成为"约定与代码其余部分所遵循的那一套不同"的地方。

### 第八部分：从这些机制推出的安全观念

**一个检查的正确性取决于一个在比较里看不见的选择。** 实测，同一次减法后面接两个不同的条件码，对一个负值给出了相反的答案，**而一个用无符号跳转写出来的范围检查在一次比较里同时覆盖了超标的值与负值。** 所以对一次边界检查的评审，是对"发出了哪条跳转"的评审。

**而"小于零"用在一个无符号值上，不是一个弱的检查、而是一个不可能的检查。** 实测，同一批标志的有符号读法说了不、而无符号读法说了是。**这就让符号性不匹配成为一类"把正常路径测多少遍都找不出来"的 bug。**

**而目标是算出来的跳转很平常，而那里就是控制流变成数据的地方。** 实测，一张被输入索引的跳转表，前面有范围检查、后面是只读的表。**所以关于一条间接跳转的问题不是"它存不存在"，而是"它的目标从哪来、什么在约束那个索引"。**

**而内核边界有它自己的约定。** 实测，那条指令覆盖了两个寄存器，一个变成返回地址、一个变成标志，**这就是第四个参数去了别处的原因** —— 所以围绕着一次系统调用的手写代码必须把那两个寄存器当成会被破坏的，无论函数约定怎么说。

**而错误有两种表示，其中只有一种是一个符号。** 实测，原始返回是取负的错误号，而库的返回是一个失败指示加上一个单独的变量。**所以把两层混着用、或者测错了那一个的代码，会把一次失败读成一次成功** —— 而一个寄存器的"类型"不是指令会说的东西。

### 检测与缓解

- **核对每一个边界检查用的条件码**，因为实测同一次比较的有符号与无符号读法给出了不同答案，而无符号那种一次覆盖了两个失败模式。
- **去找"拿一个不可能为负的值与零比较"的写法**，因为实测那种检查是不可能的、而不只是不太可能。
- **把每一条间接跳转的索引追回到它的约束上**，因为实测那张表依赖前面的范围检查与后面的只读表。
- **把跳转表放进只读映射**，因为实测那些项是决定控制流的数据，而它们所在的映射不允许写。
- **把一个条件所读的标志追回到设置它的那条指令**，因为实测的指令在"碰哪些标志"上不同。
- **在手写接口代码里，把系统调用用掉的那两个寄存器当成会被破坏的**，因为实测那条指令把两个都覆盖了。
- **原始返回值测符号、库返回值对着失败指示测**，因为实测那两个值分别是取负的错误号与一个单独的变量。
- **在库的接口存在的地方优先用它**，因为"从一个负数转换成失败指示"正是手工做容易做错的那部分。
- **观察内核边界，而不是去推断它**，因为一个进程对它自身之外的任何东西做的每一个操作都经过同一条指令。
- **并且把控制流读成成对的指令**，因为实测两条相似的指令之间的差别，恰好就是它们放着哪个标志不管。

