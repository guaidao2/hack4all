---
id: ctf-and-writeups
title_en: CTF and Writing It Up
title_zh: CTF 入门与 writeup 写法
summary_en: A CTF is the fastest feedback loop in this field — a bounded problem, an unambiguous answer, and a published solution you can compare yourself against afterwards. This entry covers the formats, the categories in the order a beginner should attack them, and the writeup habit that turns a solved puzzle into a skill.
summary_zh: CTF 是这个领域里反馈最快的一种训练 —— 题目边界清晰、答案没有歧义，而且赛后还能拿别人的解法来对照自己。这一篇讲赛制、讲新手该按什么顺序进攻题目分类，以及那个能把"做对一道题"变成"长出一项能力"的习惯：写 writeup。
tags: [beginner, ctf, methodology, writeup, learning]
tools: [CyberChef, Burp Suite, Ghidra, pwntools, CyberChef, binwalk]
attck: [T1595]
platform: [any]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### What a CTF actually is

A Capture The Flag competition hands you problems with hidden answers (flags), and scores you on finding them. There are three formats, and knowing which one you are in changes how you should play:

| Format | How it works | Beginner-friendly? |
|---|---|---|
| **Jeopardy** | A board of independent challenges by category and difficulty. Solve what you can, in any order | Yes — this is where beginners start |
| **Attack-Defense** | Each team has a service. You attack others and defend yours, continuously | No — needs a team and infrastructure |
| **King of the Hill** | Everyone fights over the same machines | No — needs speed and tooling |

Almost every online event is Jeopardy. It is also the format that teaches the most per hour, because the challenges are independent: being bad at binary exploitation does not stop you from solving a forensics task.

### Why it is worth the time

- **The feedback loop is short.** You either have the flag or you do not. No ambiguity, no waiting for a report triage.
- **The scope is bounded.** You know the answer exists, which removes the hardest part of real testing — deciding whether a thing is exploitable at all.
- **The breadth is forced.** A team event pulls you into categories you would never choose, which is how people discover what they are actually good at.
- **Writeups are public.** After the event you can read exactly how someone solved the problem you stared at for six hours, and that is the single most efficient learning resource in the field.

### The categories, in the order a beginner should attack them

| Category | What it tests | Entry difficulty | Where to read more |
|---|---|---|---|
| **Misc** | Encodings, steganography, file analysis, esoteric formats | Low | `learning/encoding-and-misc`, `learning/image-steganography`, `learning/audio-steganography` |
| **Forensics** | Disk images, memory dumps, network captures, logs | Low to medium | `offensive/forensics` |
| **Crypto** | Classical ciphers, then modern constructions and maths | Low at first, steep later | `learning/cryptography-basics` |
| **Web** | The whole web vulnerability space | Medium | the web entries in this guide |
| **Reverse** | Reading binaries and recovering logic | Medium to high | `offensive/reverse-engineering` |
| **Pwn** | Memory corruption and exploitation | High | `offensive/binary-exploitation` |

The order matters. **Start with Misc.** It teaches the habit that everything else depends on: look at what you have before deciding what it is. Half of Misc is the skill of running `file`, reading the output, trying the obvious decoding, and noticing what does not fit.

Then **Forensics**, which is Misc with bigger inputs. Then **Crypto**, which starts as puzzles (Caesar, Vigenère) and only later becomes mathematics. **Web** after you know HTTP. **Reverse** and **Pwn** last, because they are the deepest and there is no shame in arriving there in year two rather than month two.

### How to work a challenge

A procedure that works better than staring:

1. **Read the description twice.** Every word is a hint. A mention of "metadata", a strange capitalisation, a word that looks like a cipher name, a file size in the title — challenge authors are not subtle, and they do not waste words.
2. **Look at what you were given.** `file`, `xxd | head`, `strings`. Never trust a file extension; never trust that an attachment is what its name says.
3. **Write down what you know** before you start guessing. The flag format, the category, the attachment types, anything the description implied.
4. **Form one hypothesis at a time**, and test it. "It is Base64" is a hypothesis; decode it and see.
5. **Keep a running log of what you tried.** This is the habit beginners lack most. Without it, you will re-try the same thing an hour later, and you will have nothing to write up afterwards.
6. **When you are stuck, change the axis.** Not "the same tool with different flags", but something else entirely: look at the file structure rather than the content, at the end of the file rather than the beginning, at what is missing rather than what is present.
7. **Set a timer and move on.** A competition is won by solving several easy challenges, not by nearly solving one hard one. Come back later with fresh eyes.
8. **After the event, read a writeup and redo the challenge yourself.** Not read-and-nod — actually redo it. This is where the learning happens, and almost nobody does it.

### Platforms worth your time

| Platform | Best for |
|---|---|
| **picoCTF** | Absolute beginners — the gentlest on-ramp with a real progression |
| **OverTheWire (Bandit)** | Linux command line, as a game |
| **PortSwigger Web Security Academy** | Web, free, with labs and explanations |
| **CryptoHack** | Cryptography from first principles |
| **Hack The Box / TryHackMe** | Machine-based practice; TryHackMe is more guided |
| **VulnHub** | Downloadable vulnerable machines to run locally |
| **Root-Me** | A very wide range of small challenges |
| **pwn.college** | Binary exploitation, structured |
| **CTFtime** | The calendar of upcoming events, and writeups from past ones |

### Writing it up

A writeup is not homework. It is three things at once: a record for your future self, a contribution to the community that taught you, and — if you are job hunting — a public demonstration that you can explain your reasoning.

The structure that works:

```markdown
# <Challenge name> — <category> — <points>

## One-line summary
The insight that unlocked it, in one sentence.

## What we were given
Files, formats, and anything the description told us.

## Solution
1. Step, with the exact command and the output that mattered.
2. Next step, same.
   ...

## Why that worked
The principle behind each step. This is the part that transfers.

## Dead ends
What I tried that did not work, and why. (Most valuable to readers,
and the part that proves you actually solved it.)

## Takeaways
What I would recognise faster next time.
```

Three rules that separate a useful writeup from a useless one:

- **It must be reproducible.** Someone with the same files and the same commands should reach the flag. "Then I used a script" with no script is not a writeup.
- **It must explain, not just list.** A list of commands is a transcript. The reason each step was taken is what a reader needs.
- **Include the dead ends.** They are what make it honest and what make it useful — a reader who is currently stuck in the same dead end learns from yours.

What to leave out: a wall of screenshots with no text, unexplained tool output, and the assumption that the reader has your exact environment.

### The debrief, which matters more than the score

This is the part that gets skipped, and it is where skill actually accumulates.

**Do it the same day.** Memory decays fast; by the next week you will have forgotten why you took a particular turn.

**Answer three questions for every challenge:**

1. **What was it testing?** Not the answer, the concept. "PNG chunk structure" rather than "the flag was in the metadata".
2. **Where did I get stuck, and what would have unstuck me sooner?** Frequently the answer is "I did not look at the file structure first" or "I did not read the third word of the description".
3. **What is the generalisable move?** Something you would apply to a different challenge with a different file.

**Turn the answers into a checklist of your own.** Not "tips" but specific triggers: if a file is larger than its contents justify, check for appended data; if a PNG fails a CRC check, check the dimensions. That list becomes your actual skill, and it is what experienced players are running from memory.

**Then redo what you could not solve.** Read the writeup, close it, and solve the challenge from scratch. The gap between "I understand the solution" and "I can produce the solution" is large, and closing it is the whole training effect.

### Teams

- **Divide by category at the start.** The first ten minutes of reading everything saves the rest of the team from duplicating effort.
- **Keep one shared document of what has been tried.** The most common waste in a team event is two people testing the same hypothesis.
- **Announce a flag the moment you submit it.** Nothing is worse than two people solving the same challenge.
- **Do not hoard a hard challenge.** If you have been on it for an hour with no progress, hand it to someone else and take a fresh one.
- **Unblock people by asking questions, not by taking over.** "What have you tried" is faster than grabbing the keyboard.

### The things every beginner worries about

**"I do not understand the question."** Look up every term in the description; challenge authors name their techniques. If a word is unfamiliar, that unfamiliarity is the challenge.

**"My tool returns nothing."** Check the tool's assumptions: wrong file, wrong version, wrong endianness, wrong wordlist, wrong mode. Read the tool's output rather than assuming it found nothing.

**"I have been stuck for hours."** Take the hint. Hints exist to be used, and a solved challenge with a hint teaches more than an unsolved one without.

**"Everyone else is faster."** They have seen more shapes. That is exactly what writeups and repetition fix, and it is why the debrief matters more than the score.

**"Do I need to memorise tools?"** No. You need to recognise the shape of a problem and know where to look. Tools change; the shapes do not. That is the editorial position of this guide, and it is why every entry explains the mechanism before the command.

### Detection and mitigation

This entry is about training rather than attacking, but there is a direct line from a CTF habit to defensive skill:

- **Ask of every step you take: what would this look like in a log?** A web challenge teaches you the request that exploits a flaw; the same request in production is a log line, a WAF signature and an alert rule. Practising that translation is how a CTF player becomes useful on the blue side.
- **Writeups are detection research.** A public writeup describes, step by step, what an attack looks like from outside. Reading them adversarially — "which of these steps would my telemetry catch?" — is one of the cheapest ways to build detection coverage.
- **Build labs instead of only using them.** Standing up a vulnerable target and watching what it logs teaches both halves at once, and it is where the machine-based platforms get their material.
- **Note the difference between a lab and a real target.** In a lab, breaking things is the point. In a real environment, the same action has a different consequence, and knowing which steps are destructive is a skill you should practise deliberately rather than discover accidentally.
- **Keep your own notes as a corpus.** Every debrief and writeup you write is a candidate checklist, a candidate detection rule, and a record of how you think. Six months later it is more valuable than any single challenge you solved.

<!-- lang:zh -->
### CTF 到底是什么

CTF（Capture The Flag）竞赛给你一批藏着答案（flag）的题目，按找到的答案计分。有三种赛制，而知道你正在打哪一种，会改变你的打法：

| 赛制 | 怎么打 | 对新手友好吗 |
|---|---|---|
| **Jeopardy（解题）** | 一块按分类和难度排列的题板，独立题目，能解什么解什么、顺序随意 | 友好 —— 新手从这里开始 |
| **Attack-Defense（攻防）** | 每队有一台服务，不断攻击别人、防守自己 | 不友好 —— 需要团队和基础设施 |
| **King of the Hill（占山为王）** | 所有人争夺同样的机器 | 不友好 —— 拼速度和工具链 |

线上赛事几乎都是 Jeopardy。它也是每小时学到最多东西的赛制，因为题目彼此独立：**你二进制利用不行，完全不耽误你解一道取证题。**

### 为什么值得花时间

- **反馈回路很短。** 要么拿到 flag，要么没有。没有含糊，也不用等报告审核。
- **范围是有边界的。** 你知道答案存在 —— 这就去掉了真实测试里最难的那部分：判断一个东西到底能不能被利用。
- **广度是被迫的。** 团队赛会把你拖进你永远不会主动选的分类，而人就是这样发现自己真正擅长什么的。
- **writeup 是公开的。** 赛后你能读到别人**到底**怎么解出你盯了六个小时的那道题，而这是本领域效率最高的学习资源。

### 题目分类，以及新手该按什么顺序进攻

| 分类 | 考什么 | 入门门槛 | 延伸阅读 |
|---|---|---|---|
| **Misc（杂项）** | 编码、隐写、文件分析、冷门格式 | 低 | `learning/encoding-and-misc`、`learning/image-steganography`、`learning/audio-steganography` |
| **Forensics（取证）** | 磁盘镜像、内存转储、流量包、日志 | 低到中 | `offensive/forensics` |
| **Crypto（密码学）** | 先古典密码，后现代构造与数学 | 一开始低，后面陡 | `learning/cryptography-basics` |
| **Web** | 整个 Web 漏洞空间 | 中 | 本指南的 Web 各篇 |
| **Reverse（逆向）** | 读二进制、还原逻辑 | 中到高 | `offensive/reverse-engineering` |
| **Pwn** | 内存破坏与利用 | 高 | `offensive/binary-exploitation` |

**顺序是有意义的。** 从 **Misc** 开始。它教的是其他一切都要依赖的那个习惯：**先看清楚手里是什么，再决定它是什么。** Misc 有一半就是这套技能 —— 跑 `file`、读输出、试最明显的解码、注意哪里不对劲。

然后 **Forensics**，它是输入更大的 Misc。然后 **Crypto**，它从谜题开始（凯撒、维吉尼亚），很久之后才变成数学。懂了 HTTP 再上 **Web**。**Reverse** 和 **Pwn** 放最后，因为它们最深 —— 第二年才走到那里而不是第二个月，一点都不丢人。

### 拿到一道题该怎么下手

一个比"盯着看"有效得多的流程：

1. **把题面读两遍。** 每个词都是线索。"metadata"这种提法、奇怪的字母大小写、看起来像密码学名字的词、标题里的文件大小 —— 出题人不含蓄，也从不浪费字。
2. **看清手里有什么。** `file`、`xxd | head`、`strings`。永远不要相信扩展名，也永远不要相信附件就是它名字说的东西。
3. **在开始猜之前，先写下你知道什么。** flag 格式、分类、附件类型、题面暗示过的一切。
4. **一次只立一个假设**，然后去验证它。"这是 Base64"就是一个假设；解一下不就知道了。
5. **持续记录你试过什么。** 这是新手最缺的习惯。没有它，你一小时后会把同样的事再试一遍，而且事后什么也写不出来。
6. **卡住时，换一根轴。** 不是"同一个工具换参数"，而是换一个完全不同的角度：看文件结构而不是内容、看文件的结尾而不是开头、看**缺了什么**而不是有什么。
7. **设个闹钟，然后走开。** 比赛是靠解出好几道简单题赢的，不是靠"差一点解出"一道难题。晚点带着清醒的脑子回来。
8. **赛后读一份 writeup，然后自己重做一遍。** 不是读完点头，是真的重做。学习就发生在这里，而几乎没人这么做。

### 值得投入时间的平台

| 平台 | 适合什么 |
|---|---|
| **picoCTF** | 绝对新手 —— 坡度最缓，且有真实进阶路径 |
| **OverTheWire（Bandit）** | Linux 命令行，做成游戏 |
| **PortSwigger Web Security Academy** | Web，免费，带实验环境和讲解 |
| **CryptoHack** | 从第一原理学密码学 |
| **Hack The Box / TryHackMe** | 整机练习；TryHackMe 引导更多 |
| **VulnHub** | 可下载的靶机，本地跑 |
| **Root-Me** | 大量小型题目，范围极广 |
| **pwn.college** | 二进制利用，成体系 |
| **CTFtime** | 赛事日历，以及往届的 writeup |

### writeup 怎么写

writeup 不是作业。它同时是三样东西：给未来的自己的记录、对教会你的那个社区的回馈、以及（如果你在找工作）一份公开的证明——你能讲清自己的推理过程。

好用的结构：

```markdown
# <题目名> — <分类> — <分值>

## 一句话结论
解开它的那个关键洞察，一句话说完。

## 手里有什么
文件、格式，以及题面告诉了我们什么。

## 解法
1. 第一步，附确切命令与关键输出。
2. 下一步，同样。
   ……

## 为什么这样行得通
每一步背后的原理。**这才是能迁移的部分。**

## 走过的弯路
我试过但没用的，以及为什么。**对读者最有价值，
也是"你确实自己解出来了"的证据。**

## 收获
下次我会更快认出什么。
```

三条规则，区分有用的 writeup 和没用的：

- **必须可复现。** 拿到同样文件的人，照着同样的命令应该能走到 flag。"然后我用了一个脚本"却没有脚本，那不叫 writeup。
- **要解释，不能只罗列。** 一串命令只是流水账。读者需要的是**每一步为什么这么做**。
- **把弯路写进去。** 它们让文章诚实，也让文章有用 —— 正卡在同一个弯路上的读者，能从你的弯路里学到东西。

不要写进去的：没有文字的一堆截图、不加解释的工具输出、以及"默认读者拥有和你一样的环境"这个假设。

### 复盘 —— 它比分数重要

**这部分最常被跳过，而技能恰恰是在这里积累起来的。**

**当天就做。** 记忆衰减很快，拖到下周你就想不起来当初为什么拐那个弯了。

**每道题回答三个问题：**

1. **它考的是什么？** 不是答案，是概念。是"PNG 数据块结构"，而不是"flag 藏在元数据里"。
2. **我卡在哪，什么能让我更早脱困？** 答案常常是"我没先看文件结构"或者"我没读题面的第三个词"。
3. **可迁移的那一手是什么？** 某件你在另一道题、另一个文件上也能用的事。

**把答案变成你自己的清单。** 不是"小技巧"，而是具体的触发条件：如果文件比它内容该有的大小大，就查有没有附加数据；如果 PNG 的 CRC 校验不过，就查宽高。那张清单才是你真正的技能 —— 也是老手脑子里在跑的东西。

**然后重做你没能解出来的题。** 读 writeup、关掉它、从头自己做一遍。"我理解了那个解法"和"我能做出那个解法"之间隔着一大段距离，而把这段距离填平，就是训练效果的全部。

### 团队

- **一开始就按分类分工。** 开头十分钟把所有题都读一遍，能省下全队后面重复的劳动。
- **维护一份"已试过什么"的共享文档。** 团队赛里最常见的浪费，就是两个人在验证同一个假设。
- **提交成功的那一刻就喊一声。** 没有比两个人同时解出同一道题更糟的事了。
- **别霸着一道难题。** 如果你在上面待了一小时毫无进展，交给别人，自己换一道新的。
- **用提问来帮人，而不是接管键盘。** 一句"你试过什么"，比直接抢过键盘快得多。

### 每个新手都会担心的几件事

**"我看不懂题。"** 把题面里每个术语都查一遍 —— 出题人会给自己的技术起名字。哪个词你不认识，那个不认识就是这道题。

**"我的工具什么都没输出。"** 检查工具的假设：文件对了吗、版本对吗、字节序对吗、字典对吗、模式对吗。**去读工具的输出**，而不是假定它什么都没找到。

**"我卡了好几个小时。"** 看提示吧。提示就是拿来用的，一道看了提示解出来的题，比一道没看提示也没解出来的题教得多。

**"别人都比我快。"** 因为他们见过的形状更多。而这恰恰是 writeup 和重复能修的东西 —— 也是复盘比分数重要的原因。

**"我需要背工具吗？"** 不用。你需要的是**认出问题的形状**，以及知道该往哪看。工具会变，形状不会。这是本指南的编辑立场，也是每一篇都先讲机制、再讲命令的原因。

### 检测与缓解

这一篇讲的是训练而不是攻击，但从 CTF 习惯到防守能力有一条直路：

- **对你做的每一步都问一句：这一步在日志里长什么样？** 一道 Web 题教会你利用某个缺陷的那个请求；同一个请求在生产环境里就是一行日志、一条 WAF 特征、一条告警规则。练习这个"翻译"，是 CTF 选手变成有用蓝队成员的路径。
- **writeup 就是检测研究。** 一份公开 writeup 会一步步描述"这次攻击从外面看是什么样"。带着对抗的视角去读 —— "这些步骤里，哪几步我的遥测能抓到？"—— 是构建检测覆盖最便宜的方式之一。
- **不只是用靶场，自己搭靶场。** 起一个可被攻击的目标并观察它记了什么日志，能同时学到攻防两边 —— 整机平台的内容也是这么做出来的。
- **记住靶场和真实目标的区别。** 在靶场里，把东西打坏就是目的。在真实环境里，同一个动作后果不同 —— 而"哪几步是有破坏性的"，是你应该刻意练、而不是不小心撞出来的技能。
- **把自己的笔记攒成一个语料库。** 你写的每一份复盘和 writeup，都是一份候选清单、一条候选检测规则，以及一份"你是怎么想的"记录。半年之后，它比你解出的任何一道题都值钱。
