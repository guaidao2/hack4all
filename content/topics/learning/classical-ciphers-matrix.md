---
id: classical-ciphers-matrix
title_en: Classical Ciphers, Part 5 — Matrix Ciphers and the Machines
title_zh: 古典密码（五）：矩阵类与机械密码
summary_en: Playfair and Hill encrypt letters in groups, which destroys single-letter frequency and is why they held up far longer than anything before them. This entry covers both with runnable code, the digraph statistics that break Playfair, the two known-plaintext pairs that break Hill, and the machines — where the lesson turned out to be the operator rather than the mathematics.
summary_zh: 普莱费尔与希尔是按组加密字母的，这摧毁了单字母频率，也是它们比之前所有密码都撑得更久的原因。这一篇讲这两种密码并附可运行代码、讲破普莱费尔的双字母组统计、讲破希尔只需要两组已知明文，最后讲机械密码 —— 而那里的教训最终落到了操作员身上，而不是数学。
tags: [ctf, classical-cipher, playfair, hill-cipher, enigma, cryptanalysis]
tools: [Python, numpy, dCode, CyberChef]
attck: [T1140]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The step this family makes

Everything before this handled one letter at a time. This family handles **groups of letters** — pairs for Playfair, vectors of `n` letters for Hill.

That single change is why they lasted: **single-letter frequency analysis stops working.** In Playfair, `e` is not encrypted to a fixed letter; it depends on its partner. The frequency table flattens noticeably, and the statistic that still works is the one over **letter pairs**, which is a much weaker signal.

Two consequences shape this entry:

1. **Playfair is a substitution over an alphabet of 625 pairs**, which is why brute force is out and digraph statistics are in.
2. **Hill is linear algebra**, which means that with enough known plaintext the key is not searched for at all — it is *solved*.

### Playfair

Invented in 1854 and used into the Second World War, which for a pencil-and-paper cipher is an extraordinary run.

#### The square

A 5×5 grid built from a keyword, with the rest of the alphabet following and `J` merged into `I`. With the keyword `MONARCHY`:

```
      0    1    2    3    4
0     M    O    N    A    R
1     C    H    Y    B    D
2     E    F    G    I    K
3     L    P    Q    S    T
4     U    V    W    X    Z
```

#### Preparing the plaintext

Three rules, and every implementation gets at least one of them wrong:

1. **Split into pairs.** `HIDETHE...` becomes `HI DE TH E...`
2. **Never allow a pair with two identical letters.** If a pair is `EE`, insert `X` between them, pushing the second `E` into the next pair: `EE` becomes `EX E...`
3. **If the last pair has one letter, pad with `X`.** So a message of odd length always gets one `X`.

#### The three encryption rules

Each pair is encrypted by looking at where its two letters sit in the square:

| Situation | Rule |
|---|---|
| **Same row** | Each letter moves one column to the **right**, wrapping around |
| **Same column** | Each letter moves one row **down**, wrapping around |
| **Rectangle** (neither) | Each letter is replaced by the one in **its own row, the other letter's column** |

#### A worked example

Using the square above, encrypt `HIDE`:

- `HI` — `H` is at row 1 column 1, `I` is at row 2 column 3. Different row and column, so it is a rectangle. `H` moves to its own row, `I`'s column → row 1, column 3 = `B`. `I` moves to row 2, column 1 = `F`. Result: `BF`.
- `DE` — `D` is at (1,4), `E` is at (2,0). Rectangle: `D` → (1,0) = `C`; `E` → (2,4) = `K`. Result: `CK`.

Ciphertext: `BFCK`. Decrypting, `BF` → `H`(1,1) and `I`(2,3), `CK` → `D`(1,4) and `E`(2,0). Correct.

The other two rules, quickly: `MO` is the same row, so each shifts right → `ON`. `MC` is the same column, so each shifts down → `CE`.

```python
def playfair_square(key=''):
    seen, alphabet = set(), []
    for ch in (key.upper() + 'ABCDEFGHIKLMNOPQRSTUVWXYZ').replace('J', 'I'):
        if ch not in seen:
            seen.add(ch)
            alphabet.append(ch)
    return [alphabet[i*5:(i+1)*5] for i in range(5)]

def _pos(square, ch):
    ch = 'I' if ch == 'J' else ch
    for r, row in enumerate(square):
        if ch in row:
            return r, row.index(ch)
    raise ValueError(ch)

def _prep(text):
    text = ''.join(c for c in text.upper().replace('J', 'I') if c.isalpha())
    out, i = [], 0
    while i < len(text):
        a = text[i]
        b = text[i+1] if i + 1 < len(text) else 'X'
        if a == b:
            b = 'X'                       # insert the filler, do not consume the next letter
            i += 1
        else:
            i += 2
        out.append(a + b)
    return out

def playfair_encrypt(text, key=''):
    square = playfair_square(key)
    out = []
    for a, b in _prep(text):
        ra, ca = _pos(square, a)
        rb, cb = _pos(square, b)
        if ra == rb:
            out.append(square[ra][(ca+1) % 5] + square[rb][(cb+1) % 5])
        elif ca == cb:
            out.append(square[(ra+1) % 5][ca] + square[(rb+1) % 5][cb])
        else:
            out.append(square[ra][cb] + square[rb][ca])
    return ''.join(out)

def playfair_decrypt(text, key=''):
    square = playfair_square(key)
    pairs = [text[i:i+2] for i in range(0, len(text) - 1, 2)]
    out = []
    for a, b in pairs:
        ra, ca = _pos(square, a)
        rb, cb = _pos(square, b)
        if ra == rb:
            out.append(square[ra][(ca-1) % 5] + square[rb][(cb-1) % 5])
        elif ca == cb:
            out.append(square[(ra-1) % 5][ca] + square[(rb-1) % 5][cb])
        else:
            out.append(square[ra][cb] + square[rb][ca])
    return ''.join(out)

print(playfair_encrypt('HIDE', 'MONARCHY'))       # BFCK
print(playfair_decrypt('BFCK', 'MONARCHY'))       # HIDE
```

#### Why it held, and how it falls

**Why it held.** Single-letter frequencies are scrambled; the surviving statistics are over pairs. With 625 possible pairs and only a few hundred characters of text, most pairs appear once, so the signal is weak. That is a *real* improvement, and it is the same reason modern ciphers operate on blocks.

**How it falls.** The pair statistics are still not uniform, and they are enough:

- **Common digraphs** (`th`, `he`, `in`, `er`) appear with predictable frequency, and their encrypted forms repeat.
- **The structure leaks.** The three rules are invertible guesses: if you think a ciphertext pair came from a same-row pair, that constrains the square. Each guess narrows the possibilities, and Playfair has no mechanism to hide the fact that a pair was in the same row.
- **Known plaintext is very strong.** A single known plaintext-ciphertext pair gives you several square constraints at once.
- **Simulated annealing works.** Score a candidate square by how English-like the decryption looks, swap two letters, keep improvements — the same loop as Parts 1 and 3, with the "mutation" being a square swap.

```python
def playfair_score(square, ciphertext):
    """How English does this square's decryption look?"""
    plain = playfair_decrypt_with_square(ciphertext, square)
    return score(plain)      # score() from part 3: the digraph counter
```

A solver is a few dozen lines when you already have a digraph scorer, which is why "write the scorer once, reuse it in every classical cipher" is good advice.

### Hill

Invented by Lester Hill in 1929, it is the first cipher to use linear algebra, and the first where the mathematics is the whole point.

#### The idea

Take `n` plaintext letters as a vector of numbers (a=0 … z=25), multiply it by an `n × n` key matrix, and reduce modulo 26:

```
C = K · P  (mod 26)
P = K⁻¹ · C  (mod 26)
```

Each ciphertext letter now depends on **all** `n` plaintext letters, not just one. That is the property Shannon later called **diffusion**, achieved here with a single matrix multiplication.

#### The key must be invertible

Decryption needs `K⁻¹`, which exists over the integers modulo 26 only when

```
gcd(det(K), 26) = 1
```

— exactly the condition from the affine cipher in Part 1 (`a` coprime with 26), and for the same reason: the modulus is not prime, so not every number has an inverse.

Since 26 = 2 × 13, `det(K)` must be odd and not a multiple of 13. A matrix with an even determinant, or determinant 13, produces a cipher you cannot decrypt.

```python
import numpy as np
from math import gcd

def mod_inverse(a, m):
    a %= m
    for x in range(1, m):
        if (a * x) % m == 1:
            return x
    raise ValueError(f'{a} has no inverse mod {m}')

def hill_encrypt(text, key):
    K = np.array(key, dtype=int)
    n = K.shape[0]
    nums = [ord(c.upper()) - 65 for c in text if c.isalpha()]
    while len(nums) % n:
        nums.append(23)                       # pad with X
    out = []
    for i in range(0, len(nums), n):
        vec = np.array(nums[i:i+n])
        out.extend((K.dot(vec) % 26).tolist())
    return ''.join(chr(v + 65) for v in out)

def hill_decrypt(text, key):
    K = np.array(key, dtype=int)
    n = K.shape[0]
    det = round(np.linalg.det(K)) % 26
    if gcd(det, 26) != 1:
        raise ValueError(f'determinant {det} is not invertible mod 26')
    # adjugate via the cofactor matrix, then scale by det^-1
    cof = np.zeros((n, n), dtype=int)
    for i in range(n):
        for j in range(n):
            minor = np.delete(np.delete(K, i, axis=0), j, axis=1)
            cof[i, j] = round(np.linalg.det(minor)) * (-1) ** (i + j)
    Kinv = (mod_inverse(det, 26) * cof.T) % 26
    nums = [ord(c.upper()) - 65 for c in text if c.isalpha()]
    out = []
    for i in range(0, len(nums) - n + 1, n):
        vec = np.array(nums[i:i+n])
        out.extend((Kinv.dot(vec) % 26).tolist())
    return ''.join(chr(int(v) + 65) for v in out)
```

#### A worked 2×2 example

Key:

```
K = [ 3  3 ]
    [ 2  5 ]
```

`det(K) = 3×5 − 3×2 = 9`, and `gcd(9, 26) = 1`, so it is invertible.

Encrypt `HI` (`H`=7, `I`=8):

```
C = [ 3 3 ] · [7]  = [ 3×7 + 3×8 ] = [45]  ≡ [19] = T
    [ 2 5 ]   [8]    [ 2×7 + 5×8 ]   [54]    [ 2] = C
```

Ciphertext `TC`.

Decrypt: `det⁻¹ = 9⁻¹ mod 26 = 3` (since 9 × 3 = 27 ≡ 1). The adjugate is `[[5, −3], [−2, 3]]`, so:

```
K⁻¹ = 3 · [ 5 23 ] = [15 69] ≡ [15 17]
          [24  3 ]   [72  9]    [20  9]
```

And indeed `K⁻¹ · [19, 2] = [15×19 + 17×2, 20×19 + 9×2] = [319, 398] ≡ [7, 8] = HI`.

#### Breaking it: known plaintext solves it outright

This is the important part, and it is where Hill differs from every cipher in this series. **You do not search for the key. You solve for it.**

Given `n` known plaintext-ciphertext pairs, arrange them as matrices:

```
C = K · P        →        K = C · P⁻¹  (mod 26)
```

For a 2×2 key you need **four letters of known plaintext** — two pairs. That is all. Crib a word, solve a linear system, and the key falls out.

```python
def hill_known_plaintext(plain, cipher, n):
    """Recover the key from n plaintext/cipher pairs. plain and cipher are strings."""
    P = np.array([[ord(c.upper()) - 65 for c in plain[i*n:(i+1)*n]]
                  for i in range(n)], dtype=int).T
    C = np.array([[ord(c.upper()) - 65 for c in cipher[i*n:(i+1)*n]]
                  for i in range(n)], dtype=int).T
    det = round(np.linalg.det(P)) % 26
    if gcd(det, 26) != 1:
        raise ValueError('plaintext matrix is not invertible; choose different pairs')
    cof = np.zeros((n, n), dtype=int)
    for i in range(n):
        for j in range(n):
            minor = np.delete(np.delete(P, i, axis=0), j, axis=1)
            cof[i, j] = round(np.linalg.det(minor)) * (-1) ** (i + j)
    Pinv = (mod_inverse(det, 26) * cof.T) % 26
    return (C.dot(Pinv) % 26).tolist()

# with the example key, four known letters recover it immediately
print(hill_known_plaintext('HIHI', hill_encrypt('HIHI', [[3, 3], [2, 5]]), 2))
```

**Why this is fatal.** Most ciphers degrade gracefully under known plaintext — you learn some structure, not the key. Hill's key is a linear system, and **linear systems are solvable exactly**. There is no search, no statistics, no luck: the amount of known plaintext needed is exactly the size of the key.

The wider lesson is the one that matters for modern cryptography: **linearity is a weakness.** Any cipher layer whose output is a linear function of its input can be inverted from enough input-output pairs. Every modern cipher is therefore **non-linear** in its core — AES's S-box is deliberately built from a non-linear operation over a finite field, precisely so that known plaintext does not give you the key.

#### Breaking it without known plaintext

Possible but much harder: the digraph and trigraph statistics survive enough to attack, and a 2×2 or 3×3 key can be hunted with hill-climbing over candidate matrices scored by language likelihood. It is an exercise rather than a practical method, and the known-plaintext attack is so easy that nobody bothers.

### The rest of the family

| Cipher | Structure | Note |
|---|---|---|
| **Two-square / Four-square** | Two or four 5×5 squares, pairs encrypted by lookup | Stronger than Playfair, because the squares move |
| **Bifid / Trifid** | Polybius coordinates, then a transposition of the coordinate stream | A matrix cipher and a transposition in one |
| **Fractionated Morse** | Morse, then a triple-based mapping, then transposition | How a symbol cipher becomes a group cipher |
| **VIC cipher** | A whole hand system: keyed squares, transposition, substitution, one-time-ish keys | Used by a real Soviet spy; the most elaborate pencil cipher known |

### The machines, and the lesson they teach

Mechanical cipher machines are where this series meets the twentieth century. Their details matter less than the pattern of how they were broken.

#### Enigma

The German rotor machine, and the first thing to understand is that it is **a polyalphabetic cipher made automatic**:

- **Rotors** — each keystroke advances a rotor, so the substitution changes every character. This is exactly the Vigenère idea (Part 2), with a mechanical key stream.
- **Reflector** — sends the signal back through the rotors, and, critically, **makes the machine self-inverse** (the same setting decrypts and encrypts).
- **Plugboard** — swaps letter pairs before and after the rotors, multiplying the key space.

The key space was on the order of 10^23 (three rotors, ring settings, plugboard). By the standard of Parts 1 and 2 that is enormous.

**The fatal property.** Because of the reflector, **a letter can never be encrypted to itself.** That is a structural constraint that no key setting removes — and it is what allowed the codebreakers to test guesses.

**How it was actually broken**, and this is the part worth remembering:

1. **Cribs.** Operators sent predictable messages — daily weather reports, standard acknowledgements, "nothing to report". A guessed phrase gives you a known plaintext.
2. **The self-encryption constraint ruled out positions.** If the crib says `WETTER` and the ciphertext at some alignment contains a `W` where the crib has a `W`, that alignment is impossible. This cut the search space enormously.
3. **Automation.** The Polish Cipher Bureau built the first Bombe, and Turing and Welchman's British Bombe searched alignments mechanically at scale.
4. **Operator mistakes.** Repeated message keys, predictable settings, lazily chosen plugboards. Several breaks came from procedure, not from mathematics.

#### The other machines

| Machine | Country | Broken by |
|---|---|---|
| **Enigma** | Germany | Bombe, cribs, operator error |
| **Lorenz** | Germany | Colossus, after a operator repeated a message key |
| **Purple** | Japan | Team analysis, then a rebuilt machine |
| **SIGABA** | USA | Never broken in service |

#### What the machines teach, in three lines

1. **A large key space is not security.** Enigma's was astronomical and it fell anyway. This is the third time in this series that this lesson appears.
2. **Structure leaks.** The reflector's self-inverse property was a design convenience that became a permanent constraint an attacker could test against.
3. **The operator is part of the system.** Repeated keys, predictable plaintext, sloppy procedure — the biggest breaks came from people, not from the rotor wiring. That is the same lesson as reusing a nonce in Part 2, one layer up.

### Detection and mitigation

- **Hill's lesson is the one that shaped modern design.** A cipher core that is linear can be solved from known plaintext by linear algebra; there is no brute force involved and no key space large enough to help. This is why every modern block cipher is built around a **non-linear** component — AES's S-box, designed so that no linear approximation holds well — surrounded by linear layers that provide diffusion. When you read that AES "mixes" its state, that is the linear part; the security against known plaintext comes from the non-linear part.
- **Enigma's lesson is about process, and it transfers directly.** Keys reused across messages, plaintext that is predictable because of a daily routine, and administrative shortcuts are what broke the most sophisticated cipher of its era. The same three failures show up in modern incidents as nonce reuse, predictable IVs, hardcoded keys, and long-lived credentials. The mathematics of the cipher is rarely the problem; the way it is operated usually is.
- **Reflector-style structural constraints are still worth hunting for.** A cipher property that makes some outputs impossible is a property an attacker can test. This is the same reasoning as testing whether an encryption oracle can produce a chosen value at all, and it is why modern designs avoid convenient self-inverse structures in favour of explicit encryption and decryption routines.
- **For detection on real networks, none of this appears directly** — these ciphers are puzzles now. What appears is the *pattern*: a system that uses a linear or custom transformation and calls it encryption. Spotting that in a review, and being able to say precisely why it fails, is the practical use of everything in this entry.

<!-- lang:zh -->
### 这一族迈出的那一步

到此为止的一切都是一次处理一个字母。这一族处理**字母的组** —— 普莱费尔处理成对，希尔处理 `n` 个字母的向量。

就是这一个改动，让它们撑得更久：**单字母频率分析失效了。** 在普莱费尔里，`e` 不会被加密成某个固定字母，它取决于它的搭档。频率表明显被摊平，而仍然管用的统计量变成了**字母对**上的统计 —— 那是一个弱得多的信号。

由此有两个后果决定了这一篇的结构：

1. **普莱费尔是在一个 625 个"字母对"的字母表上做替换** —— 所以暴力出局、双字母组统计上场。
2. **希尔是线性代数** —— 也就是说，只要已知明文足够，密钥根本不需要"搜"，它会被**解出来**。

### 普莱费尔（Playfair）

1854 年发明，一直用到第二次世界大战 —— 对一支铅笔加一张纸的密码来说，这是惊人的寿命。

#### 方阵

用关键词构造的 5×5 格子，其余字母按顺序填入，`J` 合并到 `I`。关键词 `MONARCHY`：

```
      0    1    2    3    4
0     M    O    N    A    R
1     C    H    Y    B    D
2     E    F    G    I    K
3     L    P    Q    S    T
4     U    V    W    X    Z
```

#### 明文的预处理

三条规则，而**每一份实现都至少会做错其中一条**：

1. **切成对。** `HIDETHE...` 变成 `HI DE TH E...`
2. **不允许出现两个相同字母组成的一对。** 如果出现 `EE`，就在中间插入 `X`，把第二个 `E` 挤到下一对：`EE` 变成 `EX E...`
3. **最后一对只剩一个字母时，用 `X` 补齐。** 所以奇数长度的消息总会多一个 `X`。

#### 三条加密规则

每一对按两个字母在格子里的位置来加密：

| 情况 | 规则 |
|---|---|
| **同一行** | 两个字母各**向右**移一格，到边界回绕 |
| **同一列** | 两个字母各**向下**移一格，到边界回绕 |
| **矩形**（既不同行也不同列） | 每个字母换成**它自己那一行、另一个字母那一列**上的字母 |

#### 一个走通的例子

用上面的方阵加密 `HIDE`：

- `HI` —— `H` 在第 1 行第 1 列，`I` 在第 2 行第 3 列。不同行不同列，是矩形。`H` 走到自己那一行、`I` 那一列 → 第 1 行第 3 列 = `B`；`I` 走到第 2 行第 1 列 = `F`。结果 `BF`。
- `DE` —— `D` 在 (1,4)，`E` 在 (2,0)。矩形：`D` → (1,0) = `C`；`E` → (2,4) = `K`。结果 `CK`。

密文 `BFCK`。解密时 `BF` → `H`(1,1) 与 `I`(2,3)，`CK` → `D`(1,4) 与 `E`(2,0)。正确。

另外两条规则快速验算一下：`MO` 同行，各右移 → `ON`；`MC` 同列，各下移 → `CE`。

```python
def playfair_square(key=''):
    seen, alphabet = set(), []
    for ch in (key.upper() + 'ABCDEFGHIKLMNOPQRSTUVWXYZ').replace('J', 'I'):
        if ch not in seen:
            seen.add(ch)
            alphabet.append(ch)
    return [alphabet[i*5:(i+1)*5] for i in range(5)]

def _pos(square, ch):
    ch = 'I' if ch == 'J' else ch
    for r, row in enumerate(square):
        if ch in row:
            return r, row.index(ch)
    raise ValueError(ch)

def _prep(text):
    text = ''.join(c for c in text.upper().replace('J', 'I') if c.isalpha())
    out, i = [], 0
    while i < len(text):
        a = text[i]
        b = text[i+1] if i + 1 < len(text) else 'X'
        if a == b:
            b = 'X'                       # 插入填充字母，不消费下一个字母
            i += 1
        else:
            i += 2
        out.append(a + b)
    return out

def playfair_encrypt(text, key=''):
    square = playfair_square(key)
    out = []
    for a, b in _prep(text):
        ra, ca = _pos(square, a)
        rb, cb = _pos(square, b)
        if ra == rb:
            out.append(square[ra][(ca+1) % 5] + square[rb][(cb+1) % 5])
        elif ca == cb:
            out.append(square[(ra+1) % 5][ca] + square[(rb+1) % 5][cb])
        else:
            out.append(square[ra][cb] + square[rb][ca])
    return ''.join(out)

def playfair_decrypt(text, key=''):
    square = playfair_square(key)
    pairs = [text[i:i+2] for i in range(0, len(text) - 1, 2)]
    out = []
    for a, b in pairs:
        ra, ca = _pos(square, a)
        rb, cb = _pos(square, b)
        if ra == rb:
            out.append(square[ra][(ca-1) % 5] + square[rb][(cb-1) % 5])
        elif ca == cb:
            out.append(square[(ra-1) % 5][ca] + square[(rb-1) % 5][cb])
        else:
            out.append(square[ra][cb] + square[rb][ca])
    return ''.join(out)

print(playfair_encrypt('HIDE', 'MONARCHY'))       # BFCK
print(playfair_decrypt('BFCK', 'MONARCHY'))       # HIDE
```

#### 它为什么撑得住，又怎么倒

**为什么撑得住。** 单字母频率被打乱了；活下来的统计量是字母对上的。625 种可能的字母对，而文本只有几百个字符时，大多数对只出现一次，所以信号很弱。这是**真正的**进步，也正因如此，现代密码都是按块处理的。

**怎么倒。** 字母对的统计仍然不均匀，而这就够了：

- **常见双字母组**（`th`、`he`、`in`、`er`）出现的频率是可预测的，它们加密后的形式也会重复。
- **结构会泄漏。** 三条规则都是可逆的猜测：如果你认为某个密文对来自一个同行对，那就约束了方阵。每一次猜测都缩小可能性，而普莱费尔**没有任何机制去隐藏"这一对是同行"这件事**。
- **已知明文极强。** 一组已知的明文-密文对就能一次给出好几个方阵约束。
- **模拟退火管用。** 给候选方阵打分（看解出来像不像英语），交换两个字母，保留改进 —— 和第一、三篇是同一个循环，只是"变异"变成了交换方阵里的字母。

```python
def playfair_score(square, ciphertext):
    """这个方阵解出来的东西有多像英语？"""
    plain = playfair_decrypt_with_square(ciphertext, square)
    return score(plain)      # score() 来自第三篇：双字母组计数
```

当你手上已经有一个双字母组打分器时，求解器就是几十行的事 —— 这也是"打分器写一次、每个古典密码都复用"是个好建议的原因。

### 希尔密码（Hill）

1929 年由莱斯特·希尔发明，它是第一个使用线性代数的密码，也是第一个"数学就是全部"的密码。

#### 思想

把 `n` 个明文字母当成一个数字向量（a=0 … z=25），乘以一个 `n × n` 的密钥矩阵，再对 26 取模：

```
C = K · P  (mod 26)
P = K⁻¹ · C  (mod 26)
```

现在每个密文字母都依赖于**全部** `n` 个明文字母，而不只是一个。这就是香农后来称为**扩散**的性质，在这里由一次矩阵乘法实现。

#### 密钥必须可逆

解密需要 `K⁻¹`，而它在整数模 26 下存在，当且仅当

```
gcd(det(K), 26) = 1
```

—— 和第一篇里仿射密码的条件一模一样（`a` 与 26 互质），理由也一样：模数不是素数，所以不是每个数都有逆元。

由于 26 = 2 × 13，`det(K)` 必须是奇数、且不是 13 的倍数。行列式为偶数、或者为 13 的矩阵，产出的密文你解不回来。

```python
import numpy as np
from math import gcd

def mod_inverse(a, m):
    a %= m
    for x in range(1, m):
        if (a * x) % m == 1:
            return x
    raise ValueError(f'{a} 在模 {m} 下没有逆元')

def hill_encrypt(text, key):
    K = np.array(key, dtype=int)
    n = K.shape[0]
    nums = [ord(c.upper()) - 65 for c in text if c.isalpha()]
    while len(nums) % n:
        nums.append(23)                       # 用 X 补齐
    out = []
    for i in range(0, len(nums), n):
        vec = np.array(nums[i:i+n])
        out.extend((K.dot(vec) % 26).tolist())
    return ''.join(chr(v + 65) for v in out)

def hill_decrypt(text, key):
    K = np.array(key, dtype=int)
    n = K.shape[0]
    det = round(np.linalg.det(K)) % 26
    if gcd(det, 26) != 1:
        raise ValueError(f'行列式 {det} 在模 26 下不可逆')
    cof = np.zeros((n, n), dtype=int)
    for i in range(n):
        for j in range(n):
            minor = np.delete(np.delete(K, i, axis=0), j, axis=1)
            cof[i, j] = round(np.linalg.det(minor)) * (-1) ** (i + j)
    Kinv = (mod_inverse(det, 26) * cof.T) % 26
    nums = [ord(c.upper()) - 65 for c in text if c.isalpha()]
    out = []
    for i in range(0, len(nums) - n + 1, n):
        vec = np.array(nums[i:i+n])
        out.extend((Kinv.dot(vec) % 26).tolist())
    return ''.join(chr(int(v) + 65) for v in out)
```

#### 一个 2×2 的完整例子

密钥：

```
K = [ 3  3 ]
    [ 2  5 ]
```

`det(K) = 3×5 − 3×2 = 9`，而 `gcd(9, 26) = 1`，所以可逆。

加密 `HI`（`H`=7、`I`=8）：

```
C = [ 3 3 ] · [7]  = [ 3×7 + 3×8 ] = [45]  ≡ [19] = T
    [ 2 5 ]   [8]    [ 2×7 + 5×8 ]   [54]    [ 2] = C
```

密文 `TC`。

解密：`det⁻¹ = 9⁻¹ mod 26 = 3`（因为 9 × 3 = 27 ≡ 1）。伴随矩阵是 `[[5, −3], [−2, 3]]`，于是：

```
K⁻¹ = 3 · [ 5 23 ] = [15 69] ≡ [15 17]
          [24  3 ]   [72  9]    [20  9]
```

验算：`K⁻¹ · [19, 2] = [15×19 + 17×2, 20×19 + 9×2] = [319, 398] ≡ [7, 8] = HI`，明文解回来了。

#### 破解它：已知明文直接解出密钥

这是要紧的部分，也是希尔与本系列里其他所有密码不同的地方：**你不是去"搜"密钥，而是去"解"它。**

给定 `n` 组已知的明文-密文对，把它们排成矩阵：

```
C = K · P        →        K = C · P⁻¹  (mod 26)
```

对一个 2×2 的密钥，你只需要**四个已知明文字母** —— 两组。就这么多。猜一个词，解一个线性方程组，密钥就出来了。

```python
def hill_known_plaintext(plain, cipher, n):
    """从 n 组明文/密文对恢复密钥；plain 与 cipher 是字符串。"""
    P = np.array([[ord(c.upper()) - 65 for c in plain[i*n:(i+1)*n]]
                  for i in range(n)], dtype=int).T
    C = np.array([[ord(c.upper()) - 65 for c in cipher[i*n:(i+1)*n]]
                  for i in range(n)], dtype=int).T
    det = round(np.linalg.det(P)) % 26
    if gcd(det, 26) != 1:
        raise ValueError('明文矩阵不可逆；换不同的几组')
    cof = np.zeros((n, n), dtype=int)
    for i in range(n):
        for j in range(n):
            minor = np.delete(np.delete(P, i, axis=0), j, axis=1)
            cof[i, j] = round(np.linalg.det(minor)) * (-1) ** (i + j)
    Pinv = (mod_inverse(det, 26) * cof.T) % 26
    return (C.dot(Pinv) % 26).tolist()
```

**为什么这是致命的。** 大多数密码在已知明文下是"退化"的 —— 你学到一些结构，而不是密钥。而希尔的密钥是一个线性方程组，**线性方程组是可以精确解出来的**。没有搜索、没有统计、也没有运气：需要的已知明文量，正好等于密钥的规模。

更广的教训，也是对现代密码学要紧的那一条：**线性是弱点。** 任何一个输出是输入之线性函数的密码层，都能从足够多的输入-输出对被反推出来。所以每一个现代密码的核心都是**非线性**的 —— AES 的 S 盒刻意建立在一个有限域上的非线性运算之上，正是为了让已知明文不能直接把密钥交出去。

#### 没有已知明文时怎么破

可能，但难得多：双字母组和三字母组的统计还能存活到可被利用的程度，2×2 或 3×3 的密钥可以用爬山法在候选矩阵上搜索、用语言似然打分。这更像一道练习题而不是实用方法 —— 而**已知明文攻击太容易了，没人愿意费这个劲**。

### 这一族的其余成员

| 密码 | 结构 | 说明 |
|---|---|---|
| **Two-square / Four-square** | 两个或四个 5×5 方阵，按查表加密字母对 | 比普莱费尔强，因为方阵会移动 |
| **Bifid / Trifid** | 先取波利比奥斯坐标，再对坐标流做换位 | 矩阵密码与换位密码的合体 |
| **Fractionated Morse** | 摩斯，再按三位一组映射，再换位 | 一个符号密码如何变成组密码 |
| **VIC 密码** | 一整套手工体系：带密钥的方阵、换位、替换、接近一次性使用的密钥 | 被真实苏联间谍用过；已知最精巧的纸笔密码 |

### 机械密码，以及它们教的东西

机械密码机是这一系列与二十世纪相遇的地方。它们的细节没有"它们是怎么被破的"这个模式重要。

#### Enigma

德国的转子机，而第一件要理解的事是：**它就是把多表替换自动化了**。

- **转子（rotors）** —— 每按一个键就转动一格，所以替换每敲一个字符就变一次。这正是维吉尼亚的思想（第二篇），只是密钥流由机械产生。
- **反射器（reflector）** —— 把信号再送回转子，而关键在于它让**整个机器自逆**（同一组设置既加密也解密）。
- **插线板（plugboard）** —— 在转子前后交换字母对，把密钥空间又乘上一个量级。

密钥空间约 10^23 量级（三个转子、环设置、插线板）。按第一、二篇的标准，这巨大得离谱。

**那个致命性质。** 因为有反射器，**一个字母永远不可能被加密成它自己。** 这是一个任何密钥设置都消不掉的结构性约束 —— 而它正是破译者得以检验猜测的入口。

**它实际上是怎么被破的**，这才是值得记住的部分：

1. **Crib（已知明文猜测）。** 操作员发送可预测的消息 —— 每日天气报告、标准回执、"无事可报"。猜出一句话，你就有了已知明文。
2. **自加密约束排除了大量位置。** 如果 crib 里是 `WETTER`，而某个对齐位置上密文里也出现了 `W`，那这个对齐就是不可能的。这把搜索空间砍掉了极多。
3. **自动化。** 波兰密码局造出了第一台 Bombe，而图灵与韦尔奇曼的英国 Bombe 把对齐搜索机械化、规模化。
4. **操作员的失误。** 重复的消息密钥、可预测的设置、随便挑的插线板。好几次突破口来自流程，而不是数学。

#### 其他机器

| 机器 | 国家 | 被什么破 |
|---|---|---|
| **Enigma** | 德国 | Bombe、crib、操作失误 |
| **Lorenz** | 德国 | Colossus，起因是有操作员重复了消息密钥 |
| **Purple** | 日本 | 团队分析，之后重造了一台机器 |
| **SIGABA** | 美国 | 在服役期间从未被破 |

#### 这些机器教的东西，三句话

1. **密钥空间大不是安全。** Enigma 那个是天文数字，照样倒了。这是本系列里第三次出现这个教训。
2. **结构会泄漏。** 反射器带来的自逆性质本来是个设计上的便利，结果变成了攻击者可以拿来检验的永久约束。
3. **操作员是系统的一部分。** 重复的密钥、可预测的明文、草率的流程 —— 最大的突破口来自人，而不是转子的接线。这和第二篇里重用 nonce 是同一个教训，只是上升了一层。

### 检测与缓解

- **希尔的教训塑造了现代设计。** 一个线性的密码核心，可以被已知明文用线性代数直接解出；这里没有暴力搜索，也没有哪个密钥空间大到能救它。这就是为什么每个现代分组密码都围绕一个**非线性**组件来构造 —— AES 的 S 盒，其设计目标就是让任何线性近似都不成立 —— 而周围是提供扩散的线性层。你读到 AES 在"混合"它的状态时，那是线性部分；**抵抗已知明文的安全性来自非线性部分。**
- **Enigma 的教训关于流程，而且可以直接迁移。** 跨消息重用密钥、因为日常例行而可预测的明文、以及管理上的图省事 —— 这就是那个时代最精密的密码被破的原因。同样三种失误出现在现代事件里，就是 nonce 重用、可预测的 IV、硬编码密钥和长期不过期的凭据。**密码本身的数学很少是问题所在，它的使用方式通常是。**
- **反射器那种结构性约束至今值得去找。** 一个让某些输出不可能的密码性质，就是攻击者可以检验的性质。这和"测试一个加密预言机到底能不能产出某个选定值"是同一个思路，也是为什么现代设计避免便利的自逆结构，转而使用明确的加密与解密例程。
- **在真实网络上做检测的话，这些都不会直接出现** —— 它们现在是谜题。会出现的是那个**模式**：某个系统用了线性或自定义的变换，然后管它叫加密。在评审里认出这一点、并能准确说出它为什么不行，就是这一篇全部内容的实际用途。
