---
id: classical-ciphers-transposition
title_en: Classical Ciphers, Part 3 — Transposition
title_zh: 古典密码（三）：换位类
summary_en: Substitution changes the letters and keeps the positions; transposition does the opposite, which makes it invisible to every statistical test that breaks a substitution cipher. This entry covers the scytale, the rail fence, columnar and route ciphers, the one measurement that tells them apart, and why modern ciphers need both operations.
summary_zh: 替换改变字母、保留位置；换位正好相反 —— 这让它对第一篇里攻破替换密码的每一种统计检验都免疫。这一篇讲斯巴达棒、栅栏、列置换与路由换位，讲那个能把两者区分开的测量，以及为什么现代密码两种操作都要用。
tags: [ctf, classical-cipher, transposition, rail-fence, columnar, cryptanalysis]
tools: [Python, dCode, CyberChef]
attck: [T1140]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The opposite operation, and why it matters

Part 1 broke substitution ciphers by counting letters. Part 2 broke Vigenère by counting letters in groups. Both attacks work because substitution **changes the letters** — which is exactly the property a transposition cipher does not have.

**Transposition keeps every letter and rearranges their positions.** The letter `e` stays `e`; it just ends up somewhere else.

The consequences are immediate and worth stating as a table, because this is the fact the whole entry hangs on:

| Property | Substitution | Transposition |
|---|---|---|
| Which letters appear | Changed | **Unchanged** |
| How often each appears | Preserved | **Preserved** |
| Where each appears | Unchanged | **Changed** |
| Single-letter frequency table | Same shape, different labels | **Identical, labels and all** |
| Index of coincidence | ≈ 0.067 | ≈ 0.067 |
| Narrowed by frequency analysis | Yes | **No** |

Read the last two rows carefully, because they describe the trap. **The index of coincidence does not distinguish a transposition cipher from plaintext.** Both sit at the English value, because the letters and their proportions are untouched. If you run the Part 2 diagnostic on a columnar transposition cipher, it will tell you "this is a single substitution cipher" — and you will waste an hour looking for a mapping that does not exist.

The distinguishing measurement is different, and it is simple: **in a transposition cipher, the most common letter in the ciphertext is `E`.** In a substitution cipher it is whatever `E` was mapped to, which is rarely `E` itself. So:

- Frequency table matches English **letter for letter** → transposition. The letters are right, the order is wrong.
- Frequency table has the right **shape** but the wrong **labels** → substitution.

That one check saves more time than any tool.

### The scytale

The oldest transposition cipher, and the simplest to understand physically: wrap a strip of leather around a rod of a fixed diameter, write the message along the rod, then unwrap. The letters are now in an order that only the right rod diameter can restore.

Mathematically it is a **fixed-width columnar write**. With width `w`, write the plaintext into rows of `w` characters and read it out column by column:

```python
def scytale_encrypt(text, w):
    return ''.join(text[i::w] for i in range(w))     # read down each column

def scytale_decrypt(text, w):
    n = len(text)
    rows = -(-n // w)                                # ceiling division
    out = [''] * n
    i = 0
    for col in range(w):
        for row in range(rows):
            idx = row * w + col
            if idx < n:
                out[idx] = text[i]
                i += 1
    return ''.join(out)

print(scytale_encrypt('WEAREDISCOVEREDFLEEATONCE', 3))
```

**The key space is the width**, which for a message of length `n` is at most `n`. That is small enough to try every value, and the check is trivial: does the result read? Even better, you do not need to guess — **try every width and score the output** with the language model approach described at the end of this entry.

### The rail fence

Two different ciphers share this name in different countries, and confusing them costs time. Learn both.

#### The plain rail fence (rows, read by columns)

Write the text in rows of `n` characters, then read down the columns. This is the scytale with a different name, and it is what "栅栏密码" usually means in Chinese material:

```
W E A R E
D I S C O
V E R E D
```
read down: `WDV EEI ARS RCE EOD` → `WDVEEIARSRCEEOD`

#### The zigzag rail fence

Write the text in a **zigzag** across `n` rails, then read the rails in order. This is the version known as *Rail Fence* in English material:

```
rail 0:  W . . . E . . . C . . . R . . . E . . . E
rail 1:  . E . D . S . O . E . E . F . E . T . N . E
rail 2:  . . A . . . I . . . V . . . D . . . A . . . C
```
read rail 0, then rail 1, then rail 2: `WECREEEE...` and so on.

```python
def zigzag_encrypt(text, rails):
    if rails < 2:
        return text
    fence = [[] for _ in range(rails)]
    rail, step = 0, 1
    for ch in text:
        fence[rail].append(ch)
        if rail == 0:
            step = 1
        elif rail == rails - 1:
            step = -1
        rail += step
    return ''.join(''.join(r) for r in fence)

def zigzag_decrypt(text, rails):
    if rails < 2:
        return text
    # work out the zigzag pattern of positions, then fill it rail by rail
    pattern = []
    rail, step = 0, 1
    for _ in range(len(text)):
        pattern.append(rail)
        if rail == 0:
            step = 1
        elif rail == rails - 1:
            step = -1
        rail += step

    counts = [pattern.count(r) for r in range(rails)]
    rails_text, i = [], 0
    for c in counts:
        rails_text.append(list(text[i:i+c]))
        i += c

    out, idx = [], [0] * rails
    for r in pattern:
        out.append(rails_text[r][idx[r]])
        idx[r] += 1
    return ''.join(out)

print(zigzag_encrypt('WEAREDISCOVEREDFLEEATONCE', 3))
print(zigzag_decrypt(zigzag_encrypt('WEAREDISCOVEREDFLEEATONCE', 3), 3))
```

**Breaking it** is a loop over rail counts from 2 to about 10, decrypting with each and scoring the result. The correct rail count produces readable text; wrong ones produce English letters in the wrong order, which is easy to score automatically and obvious to the eye.

### Columnar transposition

This is the serious one, and the version that was used in real military traffic for centuries.

**The algorithm:**

1. Choose a keyword, e.g. `ZEBRAS`.
2. Number the letters of the keyword alphabetically: `Z`=6, `E`=3, `B`=2, `R`=5, `A`=1, `S`=4.
3. Write the plaintext in rows under the keyword.
4. Read the columns out **in the order of those numbers**.

```
Key:      Z E B R A S
Order:    6 3 2 5 1 4
Plain:    A T T A C K
          A T D A W N
          ...
Cipher:   read column 5 (A...), then column 2 (B...), then column 3, 4, 6
```

```python
def columnar_encrypt(text, key):
    text = text.replace(' ', '').upper()   # this example works on letters only
    key = key.upper()
    ncols = len(key)
    order = sorted(range(ncols), key=lambda i: (key[i], i))
    rows = [text[i:i+ncols] for i in range(0, len(text), ncols)]
    rows[-1] = rows[-1].ljust(ncols)          # pad the last row
    return ''.join(''.join(row[c] for row in rows) for c in order).replace(' ', '')

def columnar_decrypt(text, key):
    text = text.replace(' ', '').upper()
    key = key.upper()
    ncols = len(key)
    order = sorted(range(ncols), key=lambda i: (key[i], i))
    nrows = -(-len(text) // ncols)
    col_len = [nrows] * ncols
    # the last row is short, so the first few columns lose one character
    for c in order[-(ncols * nrows - len(text)):]:
        col_len[c] -= 1

    cols, i = {}, 0
    for c in order:
        cols[c] = text[i:i+col_len[c]]
        i += col_len[c]

    out = []
    for r in range(nrows):
        for c in range(ncols):
            if r < len(cols[c]):
                out.append(cols[c][r])
    return ''.join(out)
```

That padding detail is where most implementations break: with a short last row, the columns are not all the same length, and which columns are short depends on the reading order.

**Variants worth recognising:**

| Variant | Difference |
|---|---|
| **Myszkowski** | The key letters give column *widths*, not just an order |
| **Double transposition** | Apply it twice, usually with two different keys; much harder |
| **Disrupted / incomplete** | Blanks inserted mid-message to break the periodicity |
| **Ciphertext-first** | The ciphertext is written into the grid and read out in rows — the inverse, and easy to get backwards |

### Route ciphers

The plaintext is written into a grid along a **path** and read out along a different one: spiral in, rows out; boustrophedon (snake) in, columns out; diagonals in, rows out.

There is no key to speak of — the security is entirely in guessing the route. Breaking one is a matter of identifying the grid shape from the length, then trying the handful of plausible routes. They appear in puzzles and CTF far more than in history.

### Breaking transposition ciphers properly

The attack strategy is different from Parts 1 and 2, and it comes down to three facts:

1. **The key space is small.** Rail fence is a rail count; columnar is an anagram of the key length; scytale is a width. Nothing like `26^m`.
2. **Correctness is trivially checkable.** A wrong key produces English letters that read as nonsense. A right key produces a sentence. **That means you can brute-force with a scoring function**, which is exactly how modern automated solvers work.
3. **Known plaintext is devastating.** If you know any fragment of the plaintext, the alignment constrains the whole grid.

#### Step 1 — confirm it is transposition

```python
from collections import Counter
import re

text = re.sub(r'[^A-Za-z]', '', ciphertext).upper()
freq = Counter(text).most_common(5)
ic = sum(f * (f - 1) for f in Counter(text).values()) / (len(text) * (len(text) - 1))
print('IC:', round(ic, 4), 'top letters:', freq)
```

- `IC ≈ 0.067` and the top letters are **`E`, `T`, `A`** → transposition. Continue.
- `IC ≈ 0.067` and the top letters are something like `X`, `Q`, `M` → substitution. Back to Part 1.
- `IC ≈ 0.038` → polyalphabetic. Back to Part 2.

#### Step 2 — guess the geometry, score the result

The scorer is a simple measure of "does this look like English": the proportion of common digraphs, or the sum of the log-probabilities of each letter pair.

```python
COMMON_DIGRAPHS = ['th','he','in','er','an','re','on','at','en','nd','ti','es',
                   'or','te','of','ed','is','it','al','ar','st','to','nt','ng']

def score(text):
    text = re.sub(r'[^A-Za-z]', '', text).lower()
    return sum(text.count(d) for d in COMMON_DIGRAPHS)

def break_rail_fence(ciphertext, max_rails=12):
    best = max(range(2, max_rails + 1),
               key=lambda r: score(zigzag_decrypt(ciphertext, r)))
    return best, zigzag_decrypt(ciphertext, best)

def break_scytale(ciphertext, max_width=20):
    best = max(range(2, max_width + 1),
               key=lambda w: score(scytale_decrypt(ciphertext, w)))
    return best, scytale_decrypt(ciphertext, best)

print(break_rail_fence('WECREE...'))
```

#### Step 3 — columnar, when the key is unknown

Here the geometry is "how many columns", and each candidate has `ncols!` orderings — too many to try exhaustively past about eight columns. The practical approaches, in order of usefulness:

- **Known or guessed key length, small.** Enumerate the permutations and score each. Six columns is 720 candidates; trivial.
- **Use digraph scoring on column adjacency.** Rather than permuting blindly, find the column order that puts the most common digraphs back together: build a matrix of how often each column's last letter and another column's first letter form a common pair, then pick the best path through it. This is the classic manual attack and it scales much further than brute force.
- **Crib dragging.** If you suspect a word appears in the plaintext, place it and see which alignment makes the columns line up.
- **Simulated annealing.** Same idea as the Part 1 solver: shuffle the column order, keep improvements in the score, occasionally accept a worse move to escape a local maximum.

```python
import itertools, random

def break_columnar_bruteforce(ciphertext, keylen):
    """Only for small key lengths: keylen! grows fast (8! = 40320)."""
    ncols = keylen
    nrows = -(-len(ciphertext) // ncols)
    best, best_order = -1, None
    for perm in itertools.permutations(range(ncols)):
        order = list(perm)                         # the permutation *is* the column order
        # reconstruct using the same column lengths the decryptor needs
        col_len = [nrows] * ncols
        for c in order[-(ncols * nrows - len(ciphertext)):]:
            col_len[c] -= 1
        cols, i = {}, 0
        ok = True
        for c in order:
            cols[c] = ciphertext[i:i+col_len[c]]
            i += col_len[c]
        plain = ''.join(
            cols[c][r] for r in range(nrows) for c in range(ncols)
            if r < len(cols[c])
        )
        s = score(plain)
        if s > best:
            best, best_order = s, perm
    return best_order, best

def break_columnar_annealing(ciphertext, ncols, iterations=20000):
    nrows = -(-len(ciphertext) // ncols)
    def build(order):
        col_len = [nrows] * ncols
        for c in order[-(ncols * nrows - len(ciphertext)):]:
            col_len[c] -= 1
        cols, i = {}, 0
        for c in order:
            cols[c] = ciphertext[i:i+col_len[c]]
            i += col_len[c]
        return ''.join(cols[c][r] for r in range(nrows) for c in range(ncols) if r < len(cols[c]))

    order = list(range(ncols))
    random.shuffle(order)
    current = score(build(order))
    for _ in range(iterations):
        i, j = random.sample(range(ncols), 2)
        order[i], order[j] = order[j], order[i]
        candidate = score(build(order))
        if candidate >= current or random.random() < 0.01:
            current = candidate
        else:
            order[i], order[j] = order[j], order[i]
    return order, build(order)
```

`break_columnar_annealing` is the one that works when you know the column count but not the key, and it is worth typing out once: the "score the plaintext, mutate the key, keep the improvement" loop is the same shape as every automated classical-cipher solver you will meet.

### The modern connection: confusion and diffusion

Here is why this entry is not only about old ciphers. In 1949 Shannon described two properties a secure cipher needs:

- **Confusion** — the relationship between the key and the ciphertext should be complicated. **This is what substitution provides.**
- **Diffusion** — changing one plaintext character should change many ciphertext characters, spreading structure out. **This is what transposition provides.**

Neither alone is enough, and the failure of each is exactly what Parts 1 and 3 demonstrate:

- Substitution alone leaves letter frequencies intact, so statistics break it.
- Transposition alone leaves letter frequencies intact, so *different* statistics break it — but note that frequency analysis is useless here, which is why "just count the letters" is not a complete method.

**AES does both.** Its `SubBytes` step is confusion (a substitution box), and its `ShiftRows` and `MixColumns` steps are diffusion (rearranging and mixing bytes across the block). Every modern block cipher alternates the two, and that alternation is the direct descendant of the observation that each alone is broken.

There is one more inheritance: **interleaving**. Transposition is used deliberately in communications to spread a burst of errors across many codewords, so that an error-correcting code can fix them. Same operation, opposite intent.

### Detect it, and why you probably will not

- **In production, a pure transposition cipher is essentially extinct**, and this is why: it changes nothing about the statistics of the data, so even a simple frequency check reveals that the letters are unmodified. Anyone who runs the Part 1 diagnostic will recognise it in seconds.
- **What you will meet instead is a transposition as one component**, and the detection question changes with it. A permutation applied on its own is visible as a rearrangement; a permutation inside a modern cipher is invisible and is not meant to be seen.
- **The structural signal is periodicity in the position, not in the letters.** When data has been rearranged with a fixed width, samples taken every `n` characters often show a regularity that the data as a whole does not — which is exactly the same test as the grouped index of coincidence in Part 2, applied to a different question. It is how a fixed-width interleaving or an odd storage layout shows up in an otherwise opaque blob.
- **For defenders, the design lesson is the one that matters.** A scheme built only on substitution (encoding) or only on rearrangement (obfuscation, shuffling, reordering) is trivially reversible, and both are common in home-grown "protection": XOR with a key, then shuffle bytes; base64, then permute. Recognising that neither step adds real security is why the mitigation is always the same — use a vetted cipher that provides both confusion and diffusion, with a real key.
- **And on the analysis side, keep the three diagnostics separate.** IC near 0.067 with `E` on top means transposition; IC near 0.067 with a strange letter on top means substitution; IC near 0.038 means polyalphabetic. Getting those three straight is the difference between an hour of work and a wasted afternoon.

<!-- lang:zh -->
### 一个相反的操作，以及它为什么重要

第一篇靠数字母破掉了替换密码，第二篇靠分组数字母破掉了维吉尼亚。这两次攻击能成立，都是因为替换**改变了字母** —— 而这恰恰是换位密码没有的性质。

**换位保留每一个字母，只重排它们的位置。** `e` 还是 `e`，只是跑到别处去了。

后果很直接，值得列成一张表，因为这一篇全挂在这个事实上：

| 性质 | 替换 | 换位 |
|---|---|---|
| 出现哪些字母 | 被改变 | **不变** |
| 每个字母出现多频繁 | 保留 | **保留** |
| 每个字母出现在哪 | 不变 | **被改变** |
| 单字母频率表 | 形状相同、标签不同 | **完全相同，连标签都一样** |
| 重合指数 | ≈ 0.067 | ≈ 0.067 |
| 能否被频率分析收窄 | 能 | **不能** |

最后两行要仔细读，因为它们描述的正是那个陷阱。**重合指数区分不出换位密码和明文** —— 两者都停在英语的数值上，因为字母和它们的比例都没被动过。如果你拿第二篇的诊断办法去测一个列置换密文，它会告诉你"这是单表替换"，然后你会花一个小时去找一个根本不存在的映射。

真正的判别测量是另一个，而且很简单：**换位密码里，密文里最常见的字母就是 `E`。** 而在替换密码里，最常见的是 `E` 被映射成的那个字母，那很少是 `E` 本身。所以：

- 频率表**逐字母**对上英语 → 换位。字母是对的，顺序是错的。
- 频率表**形状**对、**标签**不对 → 替换。

这一项检查省下的时间比任何工具都多。

### 斯巴达棒（Scytale）

最古老的换位密码，而且物理上最好懂：把一条皮革缠在固定直径的棒子上，沿着棒子写，再解开。字母的顺序变成只有正确直径才能复原的样子。

数学上它是一个**固定宽度的按列写入**。宽度为 `w` 时，把明文写成每行 `w` 个字符，然后按列读出：

```python
def scytale_encrypt(text, w):
    return ''.join(text[i::w] for i in range(w))     # 逐列读出

def scytale_decrypt(text, w):
    n = len(text)
    rows = -(-n // w)                                # 向上取整
    out = [''] * n
    i = 0
    for col in range(w):
        for row in range(rows):
            idx = row * w + col
            if idx < n:
                out[idx] = text[i]
                i += 1
    return ''.join(out)

print(scytale_encrypt('WEAREDISCOVEREDFLEEATONCE', 3))
```

**密钥空间就是宽度**，对长度为 `n` 的消息最多 `n` 种。小到可以把每个值都试一遍，而验证方法 trivial —— 结果读不读得通？更好的是，你根本不需要猜：**把每个宽度都试一遍，给输出打分**（用本篇末尾讲的语言模型方法）。

### 栅栏密码

这个名字在中英文材料里指的是**两种不同的密码**，混淆它们会浪费时间。两个都学。

#### 普通栅栏（按行写、按列读）

把文本写成每行 `n` 个字符，然后按列读下来。这就是换了个名字的斯巴达棒，也是中文材料里"栅栏密码"通常指的东西：

```
W E A R E
D I S C O
V E R E D
```
按列读：`WDV EEI ARS RCE EOD` → `WDVEEIARSRCEEOD`

#### Z 形栅栏（真 zigzag）

把文本沿 **之字形**写到 `n` 条轨道上，然后按轨道顺序读出。这是英文材料里的 *Rail Fence*：

```
轨道 0:  W . . . E . . . C . . . R . . . E . . . E
轨道 1:  . E . D . S . O . E . E . F . E . T . N . E
轨道 2:  . . A . . . I . . . V . . . D . . . A . . . C
```
按轨道 0、1、2 依次读出。

```python
def zigzag_encrypt(text, rails):
    if rails < 2:
        return text
    fence = [[] for _ in range(rails)]
    rail, step = 0, 1
    for ch in text:
        fence[rail].append(ch)
        if rail == 0:
            step = 1
        elif rail == rails - 1:
            step = -1
        rail += step
    return ''.join(''.join(r) for r in fence)

def zigzag_decrypt(text, rails):
    if rails < 2:
        return text
    # 先算出之字形的位置模式，再逐轨道填充
    pattern = []
    rail, step = 0, 1
    for _ in range(len(text)):
        pattern.append(rail)
        if rail == 0:
            step = 1
        elif rail == rails - 1:
            step = -1
        rail += step

    counts = [pattern.count(r) for r in range(rails)]
    rails_text, i = [], 0
    for c in counts:
        rails_text.append(list(text[i:i+c]))
        i += c

    out, idx = [], [0] * rails
    for r in pattern:
        out.append(rails_text[r][idx[r]])
        idx[r] += 1
    return ''.join(out)

print(zigzag_encrypt('WEAREDISCOVEREDFLEEATONCE', 3))
print(zigzag_decrypt(zigzag_encrypt('WEAREDISCOVEREDFLEEATONCE', 3), 3))
```

**破解它**就是从 2 到 10 左右遍历轨道数，每个都解密一次并打分。正确的轨道数产生可读文本；错误的产生"字母都对、顺序不对"的英文，机器打分很容易，肉眼也一眼看得出。

### 列置换（Columnar Transposition）

这是真正严肃的那个，也是几个世纪里真实军用通信使用的那一版。

**算法：**

1. 选一个关键词，比如 `ZEBRAS`。
2. 把关键词的字母按字母序编号：`Z`=6、`E`=3、`B`=2、`R`=5、`A`=1、`S`=4。
3. 把明文成行写在关键词下面。
4. **按那些编号的顺序**读出各列。

```
关键词:   Z E B R A S
顺序:     6 3 2 5 1 4
明文:     A T T A C K
          A T D A W N
          ...
密文:     先读第 5 列（A...），再读第 2 列（B...），然后第 3、4、6 列
```

```python
def columnar_encrypt(text, key):
    text = text.replace(' ', '').upper()   # this example works on letters only
    key = key.upper()
    ncols = len(key)
    order = sorted(range(ncols), key=lambda i: (key[i], i))
    rows = [text[i:i+ncols] for i in range(0, len(text), ncols)]
    rows[-1] = rows[-1].ljust(ncols)          # 补最后一行
    return ''.join(''.join(row[c] for row in rows) for c in order).replace(' ', '')

def columnar_decrypt(text, key):
    text = text.replace(' ', '').upper()
    key = key.upper()
    ncols = len(key)
    order = sorted(range(ncols), key=lambda i: (key[i], i))
    nrows = -(-len(text) // ncols)
    col_len = [nrows] * ncols
    # 最后一行不满，所以按读取顺序排在前面的若干列会少一个字符
    for c in order[-(ncols * nrows - len(text)):]:
        col_len[c] -= 1

    cols, i = {}, 0
    for c in order:
        cols[c] = text[i:i+col_len[c]]
        i += col_len[c]

    out = []
    for r in range(nrows):
        for c in range(ncols):
            if r < len(cols[c]):
                out.append(cols[c][r])
    return ''.join(out)
```

**补位那个细节是大多数实现翻车的地方**：最后一行不满时，各列长度并不相同，而**哪几列短**取决于读取顺序。

**值得认得的变体：**

| 变体 | 差别 |
|---|---|
| **Myszkowski** | 密钥字母给出各列的**宽度**，不只是顺序 |
| **双重换位（double transposition）** | 做两次、通常用两把密钥；难得多 |
| **打断式 / 不完全式** | 消息中间插入空位，破坏周期性 |
| **密文优先（ciphertext-first）** | 密文写进格子、按行读出 —— 正好是反过来，很容易搞反 |

### 路由换位

把明文按某条**路径**写进格子，再按另一条路径读出：螺旋进、按行出；蛇形进、按列出；对角线进、按行出。

它谈不上有什么密钥 —— 安全性全在"猜路径"上。破解就是先由长度推断格子形状，再试那几种可能的路径。它们在谜题和 CTF 里出现的频率远高于历史。

### 正经地攻破换位密码

攻击策略和第一、二篇不同，归结为三个事实：

1. **密钥空间很小。** 栅栏是轨道数；列置换是列数的一个排列；斯巴达棒是宽度。跟 `26^m` 完全不是一回事。
2. **"对不对"的验证 trivial。** 错误的密钥产生"字母都是英文、但读不通"的结果；正确的产生一句话。**这意味着你可以用打分函数来做暴力搜索** —— 而这正是现代自动求解器的做法。
3. **已知明文是毁灭性的。** 只要你已知明文的任何片段，对齐关系就会约束整个格子。

#### 第一步 —— 确认它是换位

```python
from collections import Counter
import re

text = re.sub(r'[^A-Za-z]', '', ciphertext).upper()
freq = Counter(text).most_common(5)
ic = sum(f * (f - 1) for f in Counter(text).values()) / (len(text) * (len(text) - 1))
print('IC:', round(ic, 4), '最高频字母:', freq)
```

- `IC ≈ 0.067` 且最高频是 **`E`、`T`、`A`** → 换位。继续。
- `IC ≈ 0.067` 且最高频是 `X`、`Q`、`M` 之类 → 替换。回第一篇。
- `IC ≈ 0.038` → 多表替换。回第二篇。

#### 第二步 —— 猜几何形状，给结果打分

打分器就是"它像不像英语"的简单度量：常见双字母组的占比，或者每个字母对的对数概率之和。

```python
COMMON_DIGRAPHS = ['th','he','in','er','an','re','on','at','en','nd','ti','es',
                   'or','te','of','ed','is','it','al','ar','st','to','nt','ng']

def score(text):
    text = re.sub(r'[^A-Za-z]', '', text).lower()
    return sum(text.count(d) for d in COMMON_DIGRAPHS)

def break_rail_fence(ciphertext, max_rails=12):
    best = max(range(2, max_rails + 1),
               key=lambda r: score(zigzag_decrypt(ciphertext, r)))
    return best, zigzag_decrypt(ciphertext, best)

def break_scytale(ciphertext, max_width=20):
    best = max(range(2, max_width + 1),
               key=lambda w: score(scytale_decrypt(ciphertext, w)))
    return best, scytale_decrypt(ciphertext, best)
```

#### 第三步 —— 列置换，密钥未知时

这里的几何形状是"有多少列"，而每个候选有 `ncols!` 种排列 —— 超过八列就别想穷举了。按有效程度排序的实用方法：

- **密钥长度已知或猜到、且很短。** 枚举排列并逐个打分。六列是 720 种，小菜一碟。
- **用双字母组给列相邻关系打分。** 别盲目排列，而是找出能让常见双字母组重新拼到一起的列顺序：先建一个矩阵，记录"某一列的最后一个字母 + 另一列的第一个字母"构成常见组合的频率，再在其中挑最佳路径。这是经典的手工攻击，能应付的规模远大于暴力。
- **Crib dragging。** 如果你怀疑明文里有某个词，把它放进去，看哪种对齐能让各列对得上。
- **模拟退火。** 和第一篇的求解器是同一个思路：打乱列顺序，保留得分变好的，偶尔接受一个更差的走法以跳出局部最优。

```python
import itertools, random

def break_columnar_bruteforce(ciphertext, keylen):
    """只适合很小的密钥长度：keylen! 增长很快（8! = 40320）。"""
    ncols = keylen
    nrows = -(-len(ciphertext) // ncols)
    best, best_order = -1, None
    for perm in itertools.permutations(range(ncols)):
        key = ''.join(chr(65 + p) for p in perm)
        order = sorted(range(ncols), key=lambda i: (key[i], i))
        col_len = [nrows] * ncols
        for c in order[-(ncols * nrows - len(ciphertext)):]:
            col_len[c] -= 1
        cols, i = {}, 0
        for c in order:
            cols[c] = ciphertext[i:i+col_len[c]]
            i += col_len[c]
        plain = ''.join(
            cols[c][r] for r in range(nrows) for c in range(ncols)
            if r < len(cols[c])
        )
        s = score(plain)
        if s > best:
            best, best_order = s, perm
    return best_order, best

def break_columnar_annealing(ciphertext, ncols, iterations=20000):
    nrows = -(-len(ciphertext) // ncols)
    def build(order):
        col_len = [nrows] * ncols
        for c in order[-(ncols * nrows - len(ciphertext)):]:
            col_len[c] -= 1
        cols, i = {}, 0
        for c in order:
            cols[c] = ciphertext[i:i+col_len[c]]
            i += col_len[c]
        return ''.join(cols[c][r] for r in range(nrows) for c in range(ncols) if r < len(cols[c]))

    order = list(range(ncols))
    random.shuffle(order)
    current = score(build(order))
    for _ in range(iterations):
        i, j = random.sample(range(ncols), 2)
        order[i], order[j] = order[j], order[i]
        candidate = score(build(order))
        if candidate >= current or random.random() < 0.01:
            current = candidate
        else:
            order[i], order[j] = order[j], order[i]
    return order, build(order)
```

当你**知道列数但不知道密钥**时，`break_columnar_annealing` 就是管用的那个，值得亲手敲一遍："给明文打分、变异密钥、保留改进"这个循环，和你将来遇到的每一个古典密码自动求解器，形状都是一样的。

### 现代连接：混淆与扩散

这就是为什么这一篇不只是关于老密码。1949 年香农描述了一个安全密码需要的两个性质：

- **混淆（confusion）** —— 密钥与密文之间的关系应当复杂。**这正是替换提供的。**
- **扩散（diffusion）** —— 明文改一个字符，应当改变密文的许多字符，把结构铺散开。**这正是换位提供的。**

两者单独都不够，而它们各自失效的方式，正是第一篇和这一篇演示的：

- 只有替换，字母频率原样保留，于是统计学破掉它。
- 只有换位，字母频率同样原样保留，于是**另一套**统计学破掉它 —— 但注意，频率分析在这里完全没用，这就是为什么"数数字母就完了"不是一个完整的方法。

**AES 两者都做。** 它的 `SubBytes` 是混淆（一个替代表），它的 `ShiftRows` 和 `MixColumns` 是扩散（把字节在整个分组里重排、混合）。每一个现代分组密码都交替使用这两者，而这种交替，正是"单独用哪一个都会被破"这个观察的直接后代。

还有一份遗产：**交织（interleaving）**。换位被刻意用在通信里，把一阵突发的错误分散到很多码字上，好让纠错码能修掉它们。**同一个操作，相反的意图。**

### 检测它，以及为什么你大概不会遇到

- **在生产环境里，纯粹的换位密码基本绝迹了**，原因就在这儿：它对数据的统计性质什么都不改变，所以哪怕一次简单的频率检查都会暴露"字母没被动过"。任何跑过第一篇诊断的人几秒内就能认出来。
- **你真正会遇到的是"换位作为其中一个组件"**，而检测问题也随之改变。单独施加的一个置换，表现为一次重排；现代密码内部的置换则不可见，本来也不该被看见。
- **结构性的信号在"位置"的周期性上，而不在字母上。** 当数据按固定宽度被重排过，每隔 `n` 个字符取一次样本，常常会显示出整体数据所没有的规律性 —— 这和第二篇里的分组重合指数是同一个检验，只是问的问题不同。一个固定宽度的交织、或者一个奇怪的存储布局，就是靠这种方式在一坨看不懂的数据里露出马脚。
- **对防守方来说，要紧的是设计层面的教训。** 一个只建立在替换（编码）或只建立在重排（混淆、打乱、换序）之上的方案，是平凡可逆的，而这正是自研"保护"里最常见的两种做法：先跟密钥异或、再打乱字节；先 base64、再置换。**认出这两步都没有增加真正的安全性**，就是为什么缓解办法永远是同一个 —— 用一个同时提供混淆与扩散、并有真正密钥的、经过检验的密码。
- **在分析侧，把三个诊断分清楚。** IC 接近 0.067 且 `E` 在最高位 → 换位；IC 接近 0.067 但最高位是个奇怪的字母 → 替换；IC 接近 0.038 → 多表替换。**把这三件事分清，就是一个小时的工作和一个下午白干的区别。**
