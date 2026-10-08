---
id: classical-ciphers-substitution
title_en: Classical Ciphers, Part 1 — Substitution
title_zh: 古典密码（一）：替换类
summary_en: Substitution ciphers are worthless today, and studying them is still worth a week — because they are where frequency analysis, key space and known-plaintext reasoning are learned on problems small enough to do by hand. This entry covers the Caesar family, affine and Atbash with the modular arithmetic behind them, the general substitution cipher, and the frequency analysis that breaks it.
summary_zh: 替换密码今天一文不值，但研究它们仍然值得花上一周 —— 因为频率分析、密钥空间、已知明文推理这些思维方式，正是在这类小到能手工算的题目上学会的。这一篇讲凯撒族、仿射与埃特巴什（连同背后的模运算）、广义单表替换，以及真正能破掉它的频率分析。
tags: [beginner, ctf, classical-cipher, substitution, frequency-analysis, caesar]
tools: [Python, quipqiup, CyberChef, dCode]
attck: [T1140]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why study ciphers nobody uses

A substitution cipher can be broken by a child with a pencil. So why does this deserve more than a table with one row per cipher, which is what the beginner entry gives it?

Because the techniques transfer. **Frequency analysis** is how you attack a substitution cipher, and it is the same reasoning that underlies traffic analysis, side-channel work and the statistics used to spot covert channels. **Key space** is the vocabulary for "how much work is this worth", and it is the first question to ask of any scheme. **Known-plaintext and chosen-plaintext** reasoning starts here, where you can see the whole picture, rather than in a modern cipher where it is buried under mathematics.

There is a second reason. If you ever meet one of these in the wild — an old application with a "custom encryption" routine, a puzzle, an insider who thinks weak obfuscation is protection — you need to recognise it in seconds. That recognition skill is built by having implemented them once.

### The property that makes them all breakable

Every cipher in this entry **substitutes one letter for another and never changes the order**. The direct consequence: **the frequency distribution of the plaintext survives into the ciphertext.** If `e` is the most common letter in English, whatever letter replaces `e` becomes the most common letter in the ciphertext.

That single sentence is the entire reason these ciphers are dead, and the entire basis of the attack. If you remember one thing from this entry, make it this.

### The Caesar family

**The algorithm** is shifting the alphabet by a fixed amount. With `k` as the shift and letters numbered 0 to 25:

```
encrypt:  C = (P + k) mod 26
decrypt:  P = (C - k) mod 26
```

The modulo is what wraps `z` back around to `a`. In Python, the whole cipher is two lines:

```python
def caesar(text, k):
    out = []
    for ch in text:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            out.append(chr((ord(ch) - base + k) % 26 + base))
        else:
            out.append(ch)
    return ''.join(out)

print(caesar('Attack at dawn', 3))   # Dwwdfn dw gdzq
print(caesar('Dwwdfn dw gdzq', -3))  # Attack at dawn
```

**The key space is 25** — not 26, because a shift of zero is the plaintext. That number is the whole security argument: 25 candidates is a hand computation, and any cipher whose key space is small enough to enumerate has no security at all. This is the first place to practise the habit of asking "how many keys are there?" before looking at anything else.

**Manual method.** Write the ciphertext, then write the alphabet shifted by one beneath it, and see whether anything reads. Repeat. In practice you do not try all 25 in order; you try the shifts that make common words appear — if the ciphertext contains a repeated three-letter group, try to make it `the`.

**ROT13** is Caesar with `k = 13`, and it has one property that keeps it in use: **13 is exactly half of 26, so it is its own inverse.** Applying it twice returns the original. That is why it is used to hide spoilers and puzzle answers on forums — it is a "do not read this by accident" marker, not a security measure. Anyone who treats it as encryption has misunderstood the difference this guide's encoding entry describes.

**Variants and relatives:**

| Name | Shift | Note |
|---|---|---|
| ROT13 | 13 | Self-inverse; letters only |
| ROT5 | 5 | Applied to digits, often alongside ROT13 |
| ROT18 | 13 + 5 | Letters and digits together |
| ROT47 | 47 | Over the printable ASCII range, so it covers punctuation too |

ROT47 is worth knowing because it is not limited to letters — it rotates every printable ASCII character, which makes it slightly harder to spot by eye:

```python
def rot47(s):
    return ''.join(chr(33 + (ord(c) - 33 + 47) % 94) if 33 <= ord(c) <= 126 else c for c in s)
```

### Affine

**The algorithm** combines a multiplication and a shift:

```
encrypt:  C = (a * P + b) mod 26
decrypt:  P = a_inv * (C - b) mod 26
```

It looks like a small extension of Caesar, and it teaches something Caesar cannot: **not every key is valid.** For the decryption to exist, `a` must have a modular inverse modulo 26, which is true only when `gcd(a, 26) = 1`. Since 26 = 2 × 13, that rules out every even `a` and `a = 13`.

So the valid values of `a` are the twelve numbers coprime to 26 — 1, 3, 5, 7, 9, 11, 15, 17, 19, 21, 23, 25 — and `b` has 26 possibilities:

**Key space = 12 × 26 = 312.**

Two lessons worth taking from that arithmetic:

1. **Key space is not "the number of symbols" squared** — it is the number of keys that actually work, and a cipher with invalid keys has a smaller space than it looks.
2. If `a = 1` it degenerates into Caesar, and if `a = 25` (which is −1 mod 26) it becomes Atbash. The "different" ciphers in this entry are mostly the same cipher with different parameters, which is a useful thing to notice early.

```python
from math import gcd

def affine(text, a, b):
    assert gcd(a, 26) == 1, 'a must be coprime with 26'
    a_inv = pow(a, -1, 26)
    out = []
    for ch in text:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            x = ord(ch) - base
            y = (a * x + b) % 26 if a_inv else 0
            out.append(chr(y + base))
        else:
            out.append(ch)
    return ''.join(out)

def affine_decrypt(text, a, b):
    a_inv = pow(a, -1, 26)
    out = []
    for ch in text:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            y = ord(ch) - base
            out.append(chr((a_inv * (y - b)) % 26 + base))
        else:
            out.append(ch)
    return ''.join(out)
```

**Breaking it** is a 312-case brute force, which is still nothing, and there is a smarter route: an affine cipher with `a = 1` or `a = 25` is a Caesar or Atbash, so try those first, then enumerate the rest.

### Atbash

Atbash replaces each letter with its mirror in the alphabet — `a` with `z`, `b` with `y`, and so on:

```python
def atbash(text):
    out = []
    for ch in text:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            out.append(chr(base + 25 - (ord(ch) - base)))
        else:
            out.append(ch)
    return ''.join(out)
```

It is the affine cipher with `a = 25, b = 25`, and like ROT13 it is self-inverse. It appears constantly in puzzles because it is a Hebrew cipher historically, and because "mirror the alphabet" is a natural thing for a puzzle author to reach for.

**Recognising it** takes seconds: take the first word of the ciphertext and mirror it back. `GSRH` becomes `THIS`, which is a word, and you are done.

### The general substitution cipher

Now the real thing: **any permutation of the alphabet**. The key is a mapping of all 26 letters, not a shift.

**Key space = 26! ≈ 4.03 × 10^26.**

That number is worth comparing to the others: 25, then 312, then 10^26. At this size brute force is finished — even at a billion keys per second it is longer than the age of the universe. And yet these ciphers were broken routinely, by hand, for centuries.

**The reason is that the key space was never the right measure.** The cipher preserves letter frequency, so the *effective* uncertainty is far smaller than 26!. This is a lesson that recurs in modern cryptography: the size of the key space is not the same as the difficulty of the problem.

### Frequency analysis, properly

This is the skill the entry exists to teach, and it is a procedure rather than a trick.

#### The data you work from

| Rank | Letter | Typical English frequency |
|---|---|---|
| 1 | E | 12.7% |
| 2 | T | 9.1% |
| 3 | A | 8.2% |
| 4 | O | 7.5% |
| 5 | I | 7.0% |
| 6 | N | 6.7% |
| 7 | S | 6.3% |
| 8 | H | 6.1% |
| 9 | R | 6.0% |
| 10 | D | 4.3% |

The classic mnemonic is `ETAOIN SHRDLU`. At the other end, `Z`, `Q` and `X` together are under 1%.

Beyond single letters, the patterns that do most of the work:

- **Double letters**: `LL`, `EE`, `SS`, `OO`, `TT`, `FF`, `RR`, `NN`, `PP`, `MM` are common; `AA`, `II`, `UU` are rare.
- **Two-letter words**: `of`, `to`, `in`, `it`, `is`, `be`, `as`, `at`, `so`, `we`, `he`, `by`, `or`, `on`, `do`, `if`, `me`, `my`, `up`, `an`, `no`, `us`, `am`.
- **Three-letter words**: `the`, `and`, `for`, `are`, `but`, `not`, `you`, `all`, `any`, `can`, `had`, `her`, `was`, `one`, `our`, `out`, `day`, `get`, `has`, `him`, `his`, `how`, `man`, `new`, `now`, `old`, `see`, `two`, `way`, `who`, `boy`, `did`, `its`, `let`, `put`, `say`, `she`, `too`, `use`.
- **Common digraphs**: `th`, `he`, `in`, `er`, `an`, `re`, `on`, `at`, `en`, `nd`, `ti`, `es`, `or`, `te`, `of`, `ed`, `is`, `it`, `al`, `ar`, `st`, `to`, `nt`, `ng`, `se`, `ha`, `as`, `ou`, `io`, `le`, `ve`, `co`, `me`, `de`, `hi`, `ri`, `ro`, `ic`, `ne`, `ea`, `ra`, `ce`, `li`, `ch`, `ll`, `be`, `ma`, `si`, `om`, `ur`.
- **Common trigraphs**: `the`, `and`, `ing`, `her`, `hat`, `his`, `tha`, `ere`, `for`, `ent`, `ion`, `ter`, `was`, `you`, `ith`, `ver`, `all`, `wit`, `thi`, `tio`.

#### The procedure

1. **Strip the text down.** Remove spaces and punctuation, or keep them — but decide, because a ciphertext with the original spacing preserved is far easier (word lengths are information).
2. **Count.** Frequency of single letters, then doubles, then short words.
3. **Guess the most frequent letter** and tentatively assign it to `E`. Then `T`, then `A`.
4. **Use a short word as an anchor.** A repeated three-letter word is very often `the`. That single guess can give you three letters at once, and those letters then constrain everything else.
5. **Fill in and iterate.** Each correct guess makes more of the text readable; each mistake shows up as unreadable fragments.
6. **Check the double letters.** If your tentative `E` is not part of any double, it is probably wrong — `EE` and `LL` and `SS` are common, and a text of any length will contain some.

#### Doing it by hand once

Take this ciphertext with the spacing preserved:

```
WKLV LV D VHFUHW PHVVDJH
```

Four words: 4, 2, 1, 7, 7 letters. The one-letter word is almost certainly `A` or `I`. Try the three-letter word first — no, there is none. Try the two-letter word: `LV` is very likely `IS`, which gives `L → I` and `V → S`.

Now the last two words end in the same two letters as each other — `VH` in both. With `V = S`, that is `S?`. Both words are seven letters ending in `SH`. `PHVVDJH` with the substitutions so far: `P H S S D J H`, so the pattern is `_ _ s s _ _ s`. "message" is `M E S S A G E` — fits. So `P → M`, `H → E`, `D → A`, `J → G`.

Now the first word `WKLV` with `V = S`: `_ _ _ s`. We have `H = E` from above... wait, `H` appears in the first word too: `W K L E S` where `L = I`. So `_ _ I E S` → `GOES`? No. Let me redo it: `WKLV` maps to `W K I S`, and the sentence should read `THIS IS A ...`. So `W → T`, `K → H`, and `H → E` is consistent, `V → S`, `L → I`. `THIS IS A MESSAGE` — and the last word `PHVVDJH` is `MESSAGE`.

The whole thing took one key guess (`LV = IS`) and one word-shape match. That is the entire method, and it is why the key space being 10^26 does not matter.

**In code**, the counting step is three lines:

```python
from collections import Counter
import re

text = re.sub(r'[^A-Za-z]', '', ciphertext).upper()
print(Counter(text).most_common(10))                        # letters
print(Counter(re.findall(r'(.)\1', text)).most_common(10))  # double letters
print(Counter(re.findall(r'\b\w{3}\b', ciphertext)).most_common(10))  # 3-letter words
```

#### When frequency analysis fails

- **Short text.** Under about a hundred characters the statistics are noise. A 20-character ciphertext has no reliable frequency profile, and the answer is usually a word-shape puzzle instead.
- **Unusual subject matter.** A text about zebras and quartz has a different letter distribution from ordinary prose.
- **Preserved or destroyed spacing.** Original spacing is a gift; a single block of letters is harder.
- **Languages other than English.** Every language has its own profile, but the method is identical — count, guess, iterate. Chinese pinyin and European languages are all soluble this way; a character-based language is not, which is a completely different problem.

For languages where hand analysis is tedious, **simulated annealing** solves substitution ciphers automatically: score a trial mapping with a language model, then randomly swap letters and keep the improvements. It is worth seeing once because it is the same idea as modern automated cryptanalysis.

### Recognising which cipher you are facing

Before attacking, classify. The order that saves the most time:

1. **Letters only, and shifting it by a few produces words?** Caesar.
2. **Mirror it and it reads?** Atbash.
3. **A single fixed substitution, translatable by hand?** General substitution — go to frequency analysis.
4. **Letter frequency is flat, and repeated groups appear at regular intervals?** Not a substitution cipher at all — that is a polyalphabetic cipher, which is the next entry in this series.
5. **Not letters at all — dots, digits, or a small symbol set?** A symbol-mapping cipher (Morse, Bacon), covered later in the series.

That step 4 test has a quantitative form: the **index of coincidence**, which measures how unevenly letters are distributed. English gives about 0.066; a polyalphabetic cipher drives it toward 0.038. It is the standard measurement for "is this one alphabet or many", and it is the bridge to the next entry.

### Tools, and when not to use them

| Tool | Use |
|---|---|
| **quipqiup** | Solves substitution ciphers automatically; the fastest route when you do not care how |
| **dCode** | A catalogue of classical ciphers with solvers for each |
| **CyberChef** | Chaining decoders; useful when the cipher is only one layer |
| **Python** | When you need to see the frequencies yourself, or when the puzzle is not quite standard |
| **`gpg`/`openssl`** | Not for these — mentioned only to say: do not confuse a puzzle cipher with real cryptography |

The honest advice: use a solver when you are in a competition and the goal is the flag, and do it by hand once when the goal is to learn. The second is where the skill comes from.

### Practise this way

1. Find any English paragraph of 300+ words.
2. Encrypt it with a Caesar shift, break it with all 25 shifts, and time yourself.
3. Encrypt it with a random substitution alphabet, then break it by frequency analysis **with a pencil**, writing your guesses down.
4. Do 3 again, but with a script that only prints the statistics, and solve it from the printout.
5. Encrypt a short text (under 100 characters) and confirm that frequency analysis fails — understanding *why* it fails is the point.

### Detection and mitigation

Classical ciphers are not a live threat, but the reasoning around them is, and two things carry over directly:

- **Recognising weak "encryption" in the wild is a real skill.** A production system with a homemade obfuscation routine, an application storing data with a fixed substitution, a vendor who "encrypts" with a hardcoded key — these appear in reviews and audits. Knowing the classical families by sight means recognising them in seconds instead of assuming the data is protected. The mitigation is the one from the cryptography entry: use a vetted library, and never let a substitution pass for encryption.
- **Frequency analysis is the basis of traffic analysis.** The same statistics that break a substitution cipher are what detect a covert channel: unusually flat entropy in a stream, repeated patterns at intervals, or a distribution that does not match the protocol claimed. When you are on the defensive side asking "does this traffic look like what it says it is", you are running the same reasoning as the pencil-and-paper attack above, one layer up.

<!-- lang:zh -->
### 为什么还要研究没人用的密码

替换密码一个小孩拿支铅笔就能破。那它凭什么值得比入门篇那种"每种一行"的表格更详细？

因为**方法会迁移**。**频率分析**是攻破替换密码的手段，而它背后那套推理，正是流量分析、侧信道，以及用来发现隐蔽通道的统计学的基础。**密钥空间**是"这值多少工作量"的词汇，也是面对任何方案时该问的第一个问题。**已知明文与选择明文**的推理从这里开始 —— 在这里你能看到全貌，而不是在现代密码里被数学埋起来。

还有第二个理由。如果你哪天在真实世界里遇到这种东西 —— 一个老系统带着自研的"加密"函数、一道谜题、一个以为弱混淆就是保护的内部人员 —— 你需要**几秒钟内认出它**。而这个认出它的能力，是靠自己实现过一遍长出来的。

### 一个性质，让这一整类都可被攻破

这一篇里的每一种密码，**都只是把一个字母换成另一个字母，从不改变顺序**。直接后果就是：**明文的频率分布会原样保留到密文里。** 如果英语里 `e` 最常见，那么密文里替代 `e` 的那个字母就会变成最常见的字母。

这一句话就是这类密码死掉的全部理由，也是攻击的全部基础。如果这一篇你只记一件事，就记这个。

### 凯撒族

**算法**是把字母表平移一个固定量。设位移为 `k`，字母编号 0 到 25：

```
加密：C = (P + k) mod 26
解密：P = (C - k) mod 26
```

那个取模就是让 `z` 绕回 `a` 的东西。在 Python 里整个密码只有两行：

```python
def caesar(text, k):
    out = []
    for ch in text:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            out.append(chr((ord(ch) - base + k) % 26 + base))
        else:
            out.append(ch)
    return ''.join(out)

print(caesar('Attack at dawn', 3))   # Dwwdfn dw gdzq
print(caesar('Dwwdfn dw gdzq', -3))  # Attack at dawn
```

**密钥空间是 25** —— 不是 26，因为位移为 0 时就是明文本身。这个数字就是全部的安全论证：25 个候选是手工能算完的量，而**任何密钥空间小到能枚举的密码，就完全没有安全性**。这也是练习"先问有多少把钥匙，再看别的"这个习惯的第一个地方。

**手工做法。** 写下密文，底下写平移一位后的字母表，看能不能读；不行就再移一位。实践中你不会老老实实试完 25 次，而是试那些能让常见词出现的位移 —— 如果密文里有一组重复的三字母，就试着把它变成 `the`。

**ROT13** 是 `k = 13` 的凯撒，它有一个让它至今仍在使用的性质：**13 恰好是 26 的一半，所以它自己是自己的逆。** 用它两次就回到原文。这就是论坛用它藏剧透和谜底的原因 —— 它是一个"别不小心读到"的标记，不是安全措施。谁把它当加密，谁就没搞懂本指南讲编码那一篇里的区别。

**变体与亲戚：**

| 名称 | 位移 | 说明 |
|---|---|---|
| ROT13 | 13 | 自逆；只处理字母 |
| ROT5 | 5 | 作用于数字，常和 ROT13 一起用 |
| ROT18 | 13 + 5 | 字母与数字一起 |
| ROT47 | 47 | 在可打印 ASCII 范围内旋转，所以连标点都覆盖 |

ROT47 值得知道，因为它不限于字母 —— 它旋转每一个可打印 ASCII 字符，所以肉眼更难认出来：

```python
def rot47(s):
    return ''.join(chr(33 + (ord(c) - 33 + 47) % 94) if 33 <= ord(c) <= 126 else c for c in s)
```

### 仿射密码

**算法**把乘法和位移结合起来：

```
加密：C = (a * P + b) mod 26
解密：P = a_inv * (C - b) mod 26
```

它看起来只是凯撒的小扩展，却教了凯撒教不了的东西：**不是每一把钥匙都合法。** 解密要存在，`a` 必须在模 26 下有逆元，而这只在 `gcd(a, 26) = 1` 时成立。由于 26 = 2 × 13，所以所有偶数 `a` 和 `a = 13` 都被排除。

于是合法的 `a` 只有与 26 互质的十二个数 —— 1、3、5、7、9、11、15、17、19、21、23、25 —— 而 `b` 有 26 种：

**密钥空间 = 12 × 26 = 312。**

这道算术里有两个值得带走的教训：

1. **密钥空间不是"符号数的平方"** —— 它是**真正能用**的钥匙数量，而一个有非法钥匙的密码，实际空间比看起来小。
2. `a = 1` 时它退化成凯撒，`a = 25`（即模 26 下的 −1）时它变成埃特巴什。这一篇里那些"不同"的密码，大多只是同一套东西换了参数 —— 早点注意到这一点很有用。

```python
from math import gcd

def affine(text, a, b):
    assert gcd(a, 26) == 1, 'a 必须与 26 互质'
    out = []
    for ch in text:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            x = ord(ch) - base
            out.append(chr((a * x + b) % 26 + base))
        else:
            out.append(ch)
    return ''.join(out)

def affine_decrypt(text, a, b):
    a_inv = pow(a, -1, 26)
    out = []
    for ch in text:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            y = ord(ch) - base
            out.append(chr((a_inv * (y - b)) % 26 + base))
        else:
            out.append(ch)
    return ''.join(out)
```

**破解它**是 312 种情况的穷举，照样不算什么；另外还有条更聪明的路：`a = 1` 或 `a = 25` 的仿射就是凯撒或埃特巴什，所以先试这两个，再枚举剩下的。

### 埃特巴什（Atbash）

埃特巴什把每个字母换成字母表里的镜像 —— `a` 对 `z`、`b` 对 `y`，如此类推：

```python
def atbash(text):
    out = []
    for ch in text:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            out.append(chr(base + 25 - (ord(ch) - base)))
        else:
            out.append(ch)
    return ''.join(out)
```

它就是 `a = 25, b = 25` 的仿射密码，和 ROT13 一样自逆。它在谜题里出现得极多，因为历史上它是一种希伯来密码，也因为"把字母表镜像一下"是出题人很自然会想到的事。

**认出它**只要几秒：把密文的第一个词镜像回来。`GSRH` 变成 `THIS`，是个词，就完事了。

### 广义单表替换

现在说真正的那个：**字母表的任意排列**。密钥是全部 26 个字母的映射，不是位移。

**密钥空间 = 26! ≈ 4.03 × 10^26。**

这个数值得和前面几个比一比：25、312、10^26。到了这个量级，穷举已经出局 —— 就算每秒试十亿把钥匙，也比宇宙的年龄还长。然而这类密码被手工破解了几个世纪。

**原因在于，密钥空间从来就不是正确的衡量标准。** 这个密码保留了字母频率，所以**有效**的不确定性远小于 26!。这个教训在现代密码学里反复出现：**密钥空间的大小，和问题的难度不是一回事。**

### 频率分析，正经讲一遍

这是这一篇存在的意义，而它是一套流程，不是一个技巧。

#### 你依据的数据

| 排名 | 字母 | 英语典型频率 |
|---|---|---|
| 1 | E | 12.7% |
| 2 | T | 9.1% |
| 3 | A | 8.2% |
| 4 | O | 7.5% |
| 5 | I | 7.0% |
| 6 | N | 6.7% |
| 7 | S | 6.3% |
| 8 | H | 6.1% |
| 9 | R | 6.0% |
| 10 | D | 4.3% |

经典口诀是 `ETAOIN SHRDLU`。另一端，`Z`、`Q`、`X` 三个加起来不到 1%。

除了单个字母，最管用的是这些模式：

- **双字母**：`LL`、`EE`、`SS`、`OO`、`TT`、`FF`、`RR`、`NN`、`PP`、`MM` 常见；`AA`、`II`、`UU` 罕见。
- **两字母词**：`of`、`to`、`in`、`it`、`is`、`be`、`as`、`at`、`so`、`we`、`he`、`by`、`or`、`on`、`do`、`if`、`me`、`my`、`up`、`an`、`no`、`us`、`am`。
- **三字母词**：`the`、`and`、`for`、`are`、`but`、`not`、`you`、`all`、`any`、`can`、`had`、`her`、`was`、`one`、`our`、`out`、`day`、`get`、`has`、`him`、`his`、`how`、`man`、`new`、`now`、`old`、`see`、`two`、`way`、`who`、`boy`、`did`、`its`、`let`、`put`、`say`、`she`、`too`、`use`。
- **常见双字母组**：`th`、`he`、`in`、`er`、`an`、`re`、`on`、`at`、`en`、`nd`、`ti`、`es`、`or`、`te`、`of`、`ed`、`is`、`it`、`al`、`ar`、`st`、`to`、`nt`、`ng`、`se`、`ha`、`as`、`ou`、`io`、`le`、`ve`、`co`、`me`、`de`、`hi`、`ri`、`ro`、`ic`、`ne`、`ea`、`ra`、`ce`、`li`、`ch`、`ll`、`be`、`ma`、`si`、`om`、`ur`。
- **常见三字母组**：`the`、`and`、`ing`、`her`、`hat`、`his`、`tha`、`ere`、`for`、`ent`、`ion`、`ter`、`was`、`you`、`ith`、`ver`、`all`、`wit`、`thi`、`tio`。

#### 流程

1. **把文本整理一下。** 去掉空格和标点，或者保留 —— 但要**决定**，因为保留了原始空格的密文要好破得多（词长本身就是信息）。
2. **统计。** 先单字母频率，再双字母，再短词。
3. **猜最常见的字母**，暂时把它当成 `E`；然后 `T`，然后 `A`。
4. **用一个短词当锚点。** 一个反复出现的三字母词，多半就是 `the`。这一个猜测能一次给你三个字母，而这三个字母又会约束其他一切。
5. **填空并迭代。** 每猜对一个，可读的部分就更多；每猜错一个，就会表现为读不通的片段。
6. **检查双字母。** 如果你暂定的 `E` 不参与任何双字母，它多半是错的 —— `EE`、`LL`、`SS` 都很常见，任何有点长度的文本里总会碰上几个。

#### 手工走一遍

拿这段保留了空格的密文：

```
WKLV LV D VHFUHW PHVVDJH
```

四个词：4、2、1、7、7 个字母。那个单词几乎肯定是 `A` 或 `I`。先看三字母词 —— 没有。那就看两字母词：`LV` 极可能是 `IS`，于是 `L → I`、`V → S`。

现在最后两个词以同样的两个字母结尾（都是 `VH`）。既然 `V = S`，那就是 `S?`。两个词都是七个字母、都以 `SH` 结尾。把已知的替换代入 `PHVVDJH`：`P H S S D J H`，形状是 `_ _ s s _ _ s`。`message` 就是 `M E S S A G E` —— 对上了。于是 `P → M`、`H → E`、`D → A`、`J → G`。

再看第一个词 `WKLV`，已知 `V = S`、`L = I`、`H = E`：`W K I S`，而整句应该是 `THIS IS A ...`，所以 `W → T`、`K → H`。连起来就是 `THIS IS A MESSAGE`。

整个过程只用了一个关键猜测（`LV = IS`）和一次词形匹配。**这就是全部方法**，也正是为什么密钥空间有 10^26 也救不了它。

**用代码**，统计那一步是三行：

```python
from collections import Counter
import re

text = re.sub(r'[^A-Za-z]', '', ciphertext).upper()
print(Counter(text).most_common(10))                        # 字母频率
print(Counter(re.findall(r'(.)\1', text)).most_common(10))  # 双字母
print(Counter(re.findall(r'\b\w{3}\b', ciphertext)).most_common(10))  # 三字母词
```

#### 频率分析什么时候会失效

- **文本太短。** 一百个字符以下，统计就是噪声。20 个字符的密文没有可靠的频率画像，那种题的答案通常是词形谜题。
- **内容主题特殊。** 一篇讲斑马和石英的文章，字母分布和普通文章不同。
- **空格被保留或被破坏。** 原始空格是礼物；一整块连续的字母更难。
- **非英语语言。** 每种语言都有自己的画像，但方法完全一样 —— 统计、猜、迭代。拼音和欧洲语言都能这样解；而表意文字语言不行，那是完全另一类问题。

对手工分析太繁琐的语言，**模拟退火**能自动解替换密码：用一个语言模型给候选映射打分，然后随机交换字母、保留变好的那些。值得看一次，因为它和现代自动化密码分析是同一个思路。

### 认出你面对的是哪一种

动手之前先分类。最省时间的顺序：

1. **只有字母，平移几位就成词？** 凯撒。
2. **镜像回来就能读？** 埃特巴什。
3. **是固定的单表替换、能手工翻译？** 广义单表替换 —— 去用频率分析。
4. **字母频率很平，重复词组按规律间隔出现？** 那根本不是替换密码 —— 那是多表替换，本系列的下一篇。
5. **根本不是字母 —— 是点、数字，或很小的符号集？** 那是符号映射类（摩斯、培根），本系列后面讲。

第 4 条有一个量化的形式：**重合指数（index of coincidence）**，它衡量字母分布有多不均匀。英语大约 0.066；多表替换会把它压向 0.038。这是"这是一套字母表还是很多套"的标准测量，也是通往下一篇的桥。

### 工具，以及什么时候不该用它们

| 工具 | 用途 |
|---|---|
| **quipqiup** | 自动解替换密码；不在乎过程时的最快路径 |
| **dCode** | 古典密码目录，每种都有求解器 |
| **CyberChef** | 串联解码器；当这种密码只是其中一层时好用 |
| **Python** | 当你要亲眼看频率，或题目不太标准时 |
| **`gpg`/`openssl`** | 不是给这些用的 —— 提它们只是想说：别把谜题密码和真实密码学搞混 |

诚实的建议：比赛里目标就是 flag 时，用求解器；目标是学习时，手工做一次。**技能是从后一种里长出来的。**

### 这样练

1. 找任意一段 300 词以上的英文。
2. 用凯撒位移加密，再用 25 种位移全部破一遍，给自己计时。
3. 用随机的替换字母表加密，然后**用铅笔**做频率分析，把你的每次猜测都写下来。
4. 再做一遍第 3 步，但只让脚本打印统计结果，你从打印结果里解出来。
5. 加密一段短文本（100 字符以下），确认频率分析失效 —— 理解它**为什么**失效才是重点。

### 检测与缓解

古典密码不是活着的威胁，但它周边的推理是，而且有两点直接迁移过来：

- **在真实世界里认出"弱加密"是一项真本事。** 一个生产系统带着自研的混淆函数、一个用固定替换存数据的应用、一个自称"加密"却用硬编码密钥的厂商 —— 这些东西会出现在评审和审计里。按长相认得古典密码家族，意味着你几秒钟就能认出来，而不是默认数据受到了保护。缓解办法就是密码学那篇里写的：用经过检验的库，绝不要让一个替换冒充加密。
- **频率分析是流量分析的基础。** 破替换密码用的那套统计学，正是检测隐蔽通道用的：数据流里异常平坦的熵、按固定间隔出现的重复模式，或者某个分布和它声称的协议对不上。当你在防守侧问"这段流量看起来像它自称的东西吗"，你跑的就是上面那套铅笔加纸的推理，只是上升了一层。
