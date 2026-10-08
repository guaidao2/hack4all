---
id: classical-ciphers-polyalphabetic
title_en: Classical Ciphers, Part 2 — Polyalphabetic and the Statistics That Broke Them
title_zh: 古典密码（二）：多表替换与破它的统计学
summary_en: Vigenère was called the unbreakable cipher for three centuries, and it fell to statistics rather than to cleverness. This entry covers the cipher and its relatives, then the whole attack — the key length from Kasiski and the index of coincidence, each key letter from chi-squared — and ends where the idea is still alive today, in stream ciphers and nonce reuse.
summary_zh: 维吉尼亚被叫作"不可破的密码"叫了三百年，而它最终是被统计学打掉的，不是被什么聪明办法。这一篇讲这个密码与它的亲族，然后讲完整的攻击 —— 用卡西斯基和重合指数求密钥长度、用卡方检验逐个求出密钥字母 —— 最后落到这个思想今天还活着的地方：流密码与 nonce 重用。
tags: [ctf, classical-cipher, vigenere, cryptanalysis, index-of-coincidence, kasiski]
tools: [Python, dCode, CyberChef]
attck: [T1140]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The step that mattered

Part 1 in this series ended with the fatal flaw of substitution: it preserves letter frequency, so `e` stays the most common letter and statistics do the rest.

Polyalphabetic ciphers fix exactly that, and the fix is worth understanding as an idea rather than as a cipher: **the same plaintext letter is encrypted differently depending on where it sits.** Shift the alphabet by a different amount at each position, and the letter frequencies flatten out. Now `e` is not `e` any more — it is whatever the key happens to produce at that position.

That is the birth of the **key stream**, and it is the direct ancestor of every stream cipher in use today. Vigenère is not a historical curiosity; it is the first working version of an idea that is currently encrypting traffic on your machine.

### The Vigenère cipher

**The algorithm.** Choose a keyword. Repeat it across the plaintext, and shift each plaintext letter by the amount of the corresponding keyword letter:

```
C_i = (P_i + K_(i mod m)) mod 26
```

where `m` is the key length. Decryption subtracts instead of adding.

**Worked example**, the standard one. Plaintext `ATTACKATDAWN`, key `LEMON` repeated as `LEMONLEMONLE`:

```
plaintext   A T T A C K A T D A W N
key         L E M O N L E M O N L E
            0 19 19 0 2 10 0 19 3 0 22 13     (numbers)
key numbers 11 4 12 14 13 11 4 12 14 13 11 4
ciphertext  L X F O P V E F R N H R
```

Check the first two by hand: `A` (0) + `L` (11) = 11 = `L`. `T` (19) + `E` (4) = 23 = `X`. The rest follow the same way.

```python
def vigenere(text, key, decrypt=False):
    out, ki = [], 0
    for ch in text:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            k = ord(key[ki % len(key)].upper()) - ord('A')
            shift = -k if decrypt else k
            out.append(chr((ord(ch) - base + shift) % 26 + base))
            ki += 1
        else:
            out.append(ch)
    return ''.join(out)

print(vigenere('ATTACKATDAWN', 'LEMON'))            # LXFOPVEFRNHR
print(vigenere('LXFOPVEFRNHR', 'LEMON', True))      # ATTACKATDAWN
```

Note that the key advances **only on letters**. Get that wrong — advancing on spaces and punctuation — and your decryption of a real message will be subtly, maddeningly off.

**Why it survived three centuries.** The key space is `26^m`, which for a key of any length is astronomically large; and far more importantly, the letter frequencies are gone. Every attack in Part 1 depends on a stable frequency distribution, and Vigenère destroyed it. It was called *le chiffre indéchiffrable*, and for the state of the art at the time, that was accurate.

**The relatives**, which you should recognise because they change the attack:

| Variant | Change | Effect on the attack |
|---|---|---|
| **Autokey** | The key is the keyword followed by the plaintext itself | Flattens statistics almost completely; needs a crib |
| **Gronsfeld** | The key is digits rather than letters | Same method, much smaller key space |
| **Beaufort** | `C = K − P` instead of `C = P + K` | Self-inverse; same analysis |
| **Variant Beaufort** | `C = P − K` | Same analysis |
| **Running key** | The key is a passage of text | Hardest of the family; needs crib dragging |
| **One-time pad** | Key as long as the message, truly random, never reused | **Unbreakable** — and unusable, for reasons below |

### Breaking it, step 1: how long is the key?

Everything depends on this. Once you know the length `m`, the ciphertext splits into `m` independent Caesar ciphers, and each one falls to Part 1's techniques. Without it, you have a flat frequency distribution and nothing to grip.

#### Kasiski examination (1863)

**The insight** is elegant. If a group of letters repeats in the ciphertext, it is usually because the same plaintext repeated **and** happened to align with the same part of the key. Both conditions together mean the distance between the two occurrences is a **multiple of the key length**.

**The procedure:**

1. Find repeated sequences of three or more characters in the ciphertext.
2. Record the distance between each pair of occurrences.
3. Factor those distances and find the common divisors.
4. The key length is very likely the largest common divisor, or a multiple of it.

**A worked sketch.** Suppose `VHVS` appears at positions 20 and 78, and `QPF` at positions 12 and 96:

- Distance 1: 78 − 20 = 58 = 2 × 29
- Distance 2: 96 − 12 = 84 = 2² × 3 × 7

The shared factor is 2, so the key length is probably 2 — or 4, or 6, all multiples. That is the weakness of Kasiski: it gives you candidates, not an answer, and short texts may contain coincidental repeats that point at nothing.

**In code**, the first two steps are:

```python
import re
from collections import Counter, defaultdict   # Counter is used by the IC code below

def kasiski(ciphertext, minlen=3):
    text = re.sub(r'[^A-Za-z]', '', ciphertext).upper()
    positions = defaultdict(list)
    for n in range(minlen, 6):
        for i in range(len(text) - n + 1):
            positions[text[i:i+n]].append(i)
    for seq, pos in positions.items():
        if len(pos) > 1:
            gaps = [pos[i+1] - pos[i] for i in range(len(pos)-1)]
            print(f'{seq}: positions {pos}, gaps {gaps}')
```

#### The index of coincidence

This is the better tool, and it is worth understanding because it is a *measurement* rather than a search.

**Definition.** Take two letters at random from the text. What is the probability that they are the same? For a language with a very uneven letter distribution, that probability is high; for a uniform distribution it is low.

**The formula:**

```
IC = Σ f_i (f_i − 1) / ( N (N − 1) )
```

where `f_i` is the count of letter `i` and `N` is the total number of letters.

**The reference values to memorise:**

| Text | IC |
|---|---|
| English plaintext | ≈ 0.0667 |
| Random / uniform (1/26) | ≈ 0.0385 |
| A single Caesar or substitution cipher | ≈ 0.0667 (the distribution survives) |
| A polyalphabetic cipher with a long key | ≈ 0.0385 (flattened) |

So the IC of a ciphertext answers the first question directly: **is this one alphabet or many?** Around 0.066 means a substitution cipher and Part 1 applies. Around 0.038 means a polyalphabetic cipher and you continue here.

**A hand calculation**, and also a warning. Take the ciphertext from the worked example, `LXFOPVEFRNHR`:

Letters: `L X F O P V E F R N H R` — counts: `F` twice, `R` twice, everything else once. `N = 12`.

```
IC = ( 2×1 + 2×1 ) / ( 12 × 11 ) = 4 / 132 ≈ 0.030
```

That is *below* the random value, which is nonsense — and the reason is that twelve characters is far too short for this statistic to mean anything. **The IC is a measurement over hundreds of characters.** A four-letter "ciphertext" has no meaningful frequency profile, and no amount of arithmetic will give it one.

#### Finding the length with the IC

Here is the method that actually works, and it is the one to implement:

1. For each candidate length `m` from 1 to about 20, split the ciphertext into `m` groups: characters at positions 0, m, 2m… in group 0; 1, m+1, 2m+1… in group 1; and so on.
2. Compute the IC of each group.
3. Average the `m` ICs.
4. **When `m` equals the real key length, each group is a single Caesar cipher, so each group's IC jumps to about 0.066.** When `m` does not divide the real length, the groups mix different shifts and the IC stays near 0.038.

The signature to look for is a spike. Multiples of the true length also spike, because those groupings still separate the shifts consistently — so **take the smallest `m` that spikes**, and check its divisors too.

A typical result looks like this (illustrative, not from a specific text):

```
m=1  0.041
m=2  0.039
m=3  0.040
m=4  0.062   <- spike
m=5  0.038
m=6  0.039
m=7  0.041
m=8  0.063   <- spike (multiple of 4)
m=9  0.040
```

Key length 4. The spike at 8 is the same answer, doubled.

```python
def index_of_coincidence(text):
    text = re.sub(r'[^A-Za-z]', '', text).upper()
    n = len(text)
    if n < 2:
        return 0.0
    counts = Counter(text)
    return sum(f * (f - 1) for f in counts.values()) / (n * (n - 1))

def average_ic_for(ciphertext, m):
    text = re.sub(r'[^A-Za-z]', '', ciphertext).upper()
    ics = [index_of_coincidence(text[i::m]) for i in range(m)]
    return sum(ics) / len(ics)

for m in range(1, 21):
    print(m, round(average_ic_for(ciphertext, m), 4))
```

#### Friedman's test

If you want a single number rather than a scan, Friedman's formula estimates the key length from the overall IC:

```
m ≈ 0.027 × N / ( (N − 1) × IC − 0.038 × N + 0.065 )
```

where 0.027, 0.038 and 0.065 are constants derived from English letter frequencies. It gives a rough estimate in one calculation, and it is much less reliable than the grouped IC scan on short texts. Use it as a cross-check, not as the answer.

### Breaking it, step 2: the key letters

Once the length is known, you have `m` groups, each of which is a Caesar cipher over a subsample. The key letter for group `i` is a shift from 0 to 25, and you recover it by asking which shift makes the group's letter frequencies look most like English.

**Method 1 — chi-squared.** This is the standard. For each candidate shift, decrypt the group, count the letters, and compare the distribution with the expected English distribution:

```
χ² = Σ (observed_i − expected_i)² / expected_i
```

The shift with the **lowest** χ² is the best fit, because chi-squared measures how far apart two distributions are.

**Method 2 — mutual IC.** Instead of comparing with English, compare each group with another group. The shift that aligns group `i`'s distribution with group `j`'s gives the *difference* between their key letters. With one group's key found by chi-squared, the rest follow by differences.

**Method 3 — by hand.** In each group, the most common letter is very likely `E`, the second `T` or `A`. Subtract to get the shift, then sanity-check against the whole text: if the run of letters is nonsense, that group is wrong.

```python
ENGLISH = {
    'A': 8.17, 'B': 1.49, 'C': 2.78, 'D': 4.25, 'E': 12.70, 'F': 2.23,
    'G': 2.02, 'H': 6.09, 'I': 6.97, 'J': 0.15, 'K': 0.77, 'L': 4.03,
    'M': 2.41, 'N': 6.75, 'O': 7.51, 'P': 1.93, 'Q': 0.10, 'R': 5.99,
    'S': 6.33, 'T': 9.06, 'U': 2.76, 'V': 0.98, 'W': 2.36, 'X': 0.15,
    'Y': 1.97, 'Z': 0.07,
}

def chi_squared(text):
    text = re.sub(r'[^A-Za-z]', '', text).upper()
    n = len(text)
    if n == 0:
        return float('inf')
    counts = Counter(text)
    return sum(
        (counts.get(c, 0) - n * ENGLISH[c] / 100) ** 2 / (n * ENGLISH[c] / 100)
        for c in ENGLISH
    )

def find_shift(group):
    """The shift that makes this group look most like English."""
    best, best_score = None, float('inf')
    for shift in range(26):
        shifted = caesar(group, -shift)
        score = chi_squared(shifted)
        if score < best_score:
            best, best_score = shift, score
    return best

def break_vigenere(ciphertext, key_length):
    text = re.sub(r'[^A-Za-z]', '', ciphertext).upper()
    shifts = [find_shift(text[i::key_length]) for i in range(key_length)]
    key = ''.join(chr(s + ord('A')) for s in shifts)
    shifts = [ord(c) - ord('A') for c in key]
    out, ki = [], 0
    for ch in ciphertext:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            out.append(chr((ord(ch) - base - shifts[ki % key_length]) % 26 + base))
            ki += 1
        else:
            out.append(ch)
    return key, ''.join(out)
```

That is a complete attack in about forty lines. Run it on a few hundred characters of English encrypted with a short key and it will recover both key and plaintext.

### The whole procedure, once

1. **Strip and uppercase** the ciphertext, and note whether the original spacing survived.
2. **Compute the overall IC.** Near 0.066? Go back to Part 1 — it is a substitution cipher. Near 0.038? Continue.
3. **Scan candidate key lengths** with the grouped IC and look for the smallest spike. Cross-check with the Kasiski gaps and Friedman.
4. **For each group, run chi-squared** to get its shift, and turn the shifts into a key.
5. **Decrypt and read.** If the result is mostly readable with patches of nonsense, the key length is slightly wrong (often a multiple of the true length) — adjust and repeat.
6. **Sanity-check the key.** A real key is usually a word. If your recovered key is `QZXM`, the length is probably wrong.

Step 6 is not a joke: human-chosen keys are words, and "does this look like a word" is one of the most effective checks in the whole procedure.

### Variants, and how the attack changes

**Autokey** uses the keyword followed by the plaintext as the running key. This flattens the statistics so well that the IC method fails entirely — the key stream is not periodic. What works instead is a **crib**: guess a likely word in the plaintext, use it to extend the key, and see whether the result reads. Any known plaintext gives you as much key as the plaintext is long.

**Gronsfeld** uses digits as the key. Identical analysis, but the key space is `10^m` instead of `26^m`, so it falls faster.

**Beaufort and Variant Beaufort** are sign changes. The same grouped-IC and chi-squared attack works; only the final arithmetic differs.

**Running key** uses an entire passage as the key, so it never repeats and no periodicity exists. This is the hardest of the classical ciphers, and it is broken with crib dragging and statistical scoring rather than with a formula.

**One-time pad** is the theoretical endpoint: a key as long as the message, truly random, used once and destroyed. It is **information-theoretically unbreakable** — the ciphertext gives an attacker no information that distinguishes one plaintext from another. It is also almost never usable, because:

- The key must be as long as the message (distribution problem),
- It must be truly random (not `random.random()`),
- It must **never be reused**, and
- It must be kept secret forever.

**Breaking the last rule is the classic failure**, and it has a name: **two-time pad**. Encrypt two messages with the same key stream and the ciphertexts are:

```
C1 = P1 ⊕ K
C2 = P2 ⊕ K
C1 ⊕ C2 = P1 ⊕ P2       (the key cancels out entirely)
```

You never learn the key, and you do not need to. `P1 ⊕ P2` is the XOR of two English texts, and that is solvable by **crib dragging**: guess a word for one message, XOR it in, and see whether the other side produces readable text.

### This is not history: it is the bug you will meet in production

The one-time-pad rule is the same rule that governs every modern stream cipher, and violating it is one of the most damaging real-world cryptographic bugs there is.

Two concrete cases:

- **AES-GCM with a reused nonce.** GCM is a stream construction. Reuse the nonce with the same key and the key stream repeats, so `C1 ⊕ C2 = P1 ⊕ P2`, exactly as above — and GCM's authentication is broken at the same time. The cryptography entry in this guide lists this as a top-ten implementation mistake; this entry is where you see *why*.
- **WEP.** The 802.11 encryption standard used RC4 with a 24-bit IV, which forced reuse after a few million packets — often minutes of traffic. FMS and the later PTW attack recover the key from the resulting key stream reuse. WEP was broken by exactly the mathematics in this entry, applied at scale.

The lesson generalises: **if a key stream repeats, the cipher is gone, no matter how strong the underlying primitive.** Modern applications have moved from "use a random IV" to "use a counter you cannot repeat" and "use an AEAD that refuses a repeated nonce" precisely because this failure was so common.

### Tools, and how to practise

| Tool | Use |
|---|---|
| Your own script | Worth writing once — the IC and chi-squared code above is the whole thing |
| **dCode** | Has Vigenère and Gronsfeld solvers with automatic key-length detection |
| **CyberChef** | Fine when the cipher is one layer among several |
| `vigenere-solver` and similar | Convenience; use only after you have done it by hand once |

**Practice routine:**

1. Take 1000 words of English and encrypt with a random six-letter key.
2. Run the grouped IC scan and confirm the spike at 6.
3. Recover each key letter with chi-squared, and confirm you get the key back.
4. Repeat with a 12-letter key and note how much more text the statistics need.
5. Encrypt a **short** message (200 characters) with a six-letter key and watch the method fail — then work out which step broke and why.
6. Encrypt two messages with the same key and break them with crib dragging, doing the XOR by hand at least once.

Step 6 is the one that pays off in real work, because it is the same attack that breaks nonce reuse.

### Detection and mitigation

- **The generalisable detection signal is repetition.** A repeating key stream leaves statistical traces whether it is a Vigenère cipher or a badly implemented stream cipher: repeated patterns, an IC that sits between the plaintext value and the random value, and nonce or IV fields that cycle sooner than they should. For WEP it was a 24-bit IV; in a modern protocol it is any counter that wraps, any "random" IV from a weak generator, and any session key reused across messages.
- **Watch for IV and nonce management in code review.** The bug rarely looks like "we reused a key". It looks like a counter reset on reconnect, a random IV generated with `rand()`, a session identifier derived from a timestamp, or a key that is regenerated per connection but not per message. The last one is the most subtle and the most common.
- **On the defensive side, protocol analysis is the same arithmetic.** An IC-style measure tells you whether a stream is structured or random: too flat is suspicious for data that should have structure, and too structured is suspicious for data that should be encrypted. That is how encrypted tunnels and covert channels get noticed.
- **The mitigation is the rule, and it is not optional.** A unique nonce per message under a given key, an AEAD that fails closed when the nonce repeats, and key rotation designed so that a key is never used twice for the same purpose. Every modern library supports this; the failures come from using it wrongly rather than from it being unavailable.
- **And the historical lesson for design**: Vigenère was believed unbreakable because it looked complex and the key space was enormous. It was broken by a measurement, not by cleverness. "It looks complicated to me" has never been a security argument, and this cipher is the cleanest demonstration of that in the whole field.

<!-- lang:zh -->
### 那一步真正有意义的地方

本系列第一篇结束于替换密码的致命伤：它保留字母频率，所以 `e` 还是最常见的字母，剩下的交给统计学。

多表替换要修的正是这个，而它的修法值得当成一个**思想**来理解，而不是一个密码：**同一个明文字母，根据它所处的位置被加密成不同的密文。** 在每个位置上平移量都不同，字母频率就被摊平了。`e` 不再总是 `e` —— 它是那个位置上密钥恰好产生的那个字母。

这就是**密钥流**的诞生，而它是今天所有流密码的直接祖先。维吉尼亚不是历史猎奇 —— 它是"此刻正在加密你机器上流量"的那个思想的第一个可用版本。

### 维吉尼亚密码

**算法。** 选一个关键词，把它在明文上循环铺开，用对应的密钥字母去平移每个明文字母：

```
C_i = (P_i + K_(i mod m)) mod 26
```

其中 `m` 是密钥长度。解密是减而不是加。

**标准示例。** 明文 `ATTACKATDAWN`，密钥 `LEMON` 铺开成 `LEMONLEMONLE`：

```
明文        A T T A C K A T D A W N
密钥        L E M O N L E M O N L E
            0 19 19 0 2 10 0 19 3 0 22 13     （数字）
密钥数字     11 4 12 14 13 11 4 12 14 13 11 4
密文        L X F O P V E F R N H R
```

手算前两个验证一下：`A`(0) + `L`(11) = 11 = `L`；`T`(19) + `E`(4) = 23 = `X`。其余同理。

```python
def vigenere(text, key, decrypt=False):
    out, ki = [], 0
    for ch in text:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            k = ord(key[ki % len(key)].upper()) - ord('A')
            out.append(chr((ord(ch) - base + (-k if decrypt else k)) % 26 + base))
            ki += 1
        else:
            out.append(ch)
    return ''.join(out)

print(vigenere('ATTACKATDAWN', 'LEMON'))            # LXFOPVEFRNHR
print(vigenere('LXFOPVEFRNHR', 'LEMON', True))      # ATTACKATDAWN
```

注意密钥**只在字母上推进**。搞错这一点 —— 在空格和标点上也让密钥前进 —— 你对真实报文的解密就会微妙地、气人地一直不对。

**它为什么活了三百年。** 密钥空间是 `26^m`，任何长度的密钥都大到天文数字；而更重要的是，**字母频率没有了**。第一篇里的每一种攻击都依赖稳定的频率分布，而维吉尼亚把它摧毁了。它被称作 *le chiffre indéchiffrable*（不可破的密码），按当时的水平，这个说法是准确的。

**它的亲族**，你该认得，因为它们会改变攻击方式：

| 变体 | 改动 | 对攻击的影响 |
|---|---|---|
| **Autokey（自动密钥）** | 密钥 = 关键词 + 明文本身 | 几乎完全摊平统计；需要已知明文（crib） |
| **Gronsfeld** | 密钥是数字而不是字母 | 方法相同，密钥空间小得多 |
| **Beaufort** | `C = K − P` 而不是 `C = P + K` | 自逆；分析一样 |
| **Variant Beaufort** | `C = P − K` | 分析一样 |
| **Running key（滚动密钥）** | 密钥是一段文本 | 这一族里最难；需要 crib dragging |
| **一次性密码本（OTP）** | 密钥与消息等长、真随机、绝不重用 | **不可破** —— 也基本没法用，原因见下 |

### 破解第一步：密钥有多长

一切都取决于这个。一旦知道长度 `m`，密文就分成 `m` 组独立的凯撒密码，每一组都能用第一篇的技巧解决。不知道它，你面对的是一片平坦的频率分布，无从下手。

#### 卡西斯基测定（1863）

**这个洞察很优雅。** 如果密文里有一段字母重复出现，通常是因为**明文重复了**，而且**恰好对齐到了密钥的同一部分**。两个条件同时成立，就意味着两次出现之间的距离是**密钥长度的整数倍**。

**步骤：**

1. 在密文里找出长度三个字符以上的重复片段。
2. 记录每一对出现位置之间的距离。
3. 把这些距离做因数分解，找公约数。
4. 密钥长度极可能就是最大的公约数，或者它的某个倍数。

**一个推演。** 假设 `VHVS` 出现在位置 20 和 78，`QPF` 出现在位置 12 和 96：

- 距离一：78 − 20 = 58 = 2 × 29
- 距离二：96 − 12 = 84 = 2² × 3 × 7

公约数是 2，所以密钥长度大概是 2 —— 或者 4、6 这些倍数。这正是卡西斯基的弱点：它给你**候选**，而不给答案；短文本里还会出现纯属巧合的重复，把你指向不存在的东西。

**用代码**，前两步是：

```python
import re
from collections import Counter, defaultdict   # Counter is used by the IC code below

def kasiski(ciphertext, minlen=3):
    text = re.sub(r'[^A-Za-z]', '', ciphertext).upper()
    positions = defaultdict(list)
    for n in range(minlen, 6):
        for i in range(len(text) - n + 1):
            positions[text[i:i+n]].append(i)
    for seq, pos in positions.items():
        if len(pos) > 1:
            gaps = [pos[i+1] - pos[i] for i in range(len(pos)-1)]
            print(f'{seq}: 位置 {pos}, 间隔 {gaps}')
```

#### 重合指数（Index of Coincidence）

这是更好的工具，而且值得理解，因为它是一个**测量**，不是一次搜索。

**定义。** 从文本里随机取两个字母，它们相同的概率是多少？对一个字母分布很不均匀的语言，这个概率高；对均匀分布，它低。

**公式：**

```
IC = Σ f_i (f_i − 1) / ( N (N − 1) )
```

其中 `f_i` 是字母 `i` 的计数，`N` 是字母总数。

**要背下来的参考值：**

| 文本 | IC |
|---|---|
| 英语明文 | ≈ 0.0667 |
| 随机 / 均匀（1/26） | ≈ 0.0385 |
| 单个凯撒或替换密码 | ≈ 0.0667（分布被保留下来） |
| 长密钥的多表替换 | ≈ 0.0385（被摊平） |

所以密文的 IC 直接回答第一个问题：**这是一套字母表还是很多套？** 0.066 附近意味着替换密码，回去用第一篇；0.038 附近意味着多表替换，继续往下。

**一次手工计算，同时也是一个警告。** 拿上面示例的密文 `LXFOPVEFRNHR`：

字母 `L X F O P V E F R N H R` —— 计数：`F` 两次、`R` 两次，其余各一次。`N = 12`。

```
IC = ( 2×1 + 2×1 ) / ( 12 × 11 ) = 4 / 132 ≈ 0.030
```

这个值**低于**随机值，而这是荒谬的 —— 原因就是十二个字符对这项统计来说实在太短。**IC 是对几百个字符做出来的测量。** 一段四个字母的"密文"没有任何有意义的频率画像，再怎么算也算不出来。

#### 用 IC 找长度

这才是真正管用的方法，也是该动手实现的那个：

1. 对每个候选长度 `m`（从 1 到 20 左右），把密文分成 `m` 组：位置 0、m、2m… 的字符进第 0 组；1、m+1、2m+1… 进第 1 组；依此类推。
2. 算每一组的 IC。
3. 把 `m` 个 IC 求平均。
4. **当 `m` 等于真实密钥长度时，每一组都是一个凯撒密码，于是每组的 IC 会跳到约 0.066。** 而 `m` 不能整除真实长度时，组内混着不同的位移，IC 就停在 0.038 附近。

要找的特征是一个**尖峰**。真实长度的**倍数**也会出现尖峰，因为那些分组方式同样把位移稳定地分开了 —— 所以**取出现尖峰的最小 `m`**，同时检查它的因数。

一个典型的结果长这样（示意，不是某个具体文本的数据）：

```
m=1  0.041
m=2  0.039
m=3  0.040
m=4  0.062   <- 尖峰
m=5  0.038
m=6  0.039
m=7  0.041
m=8  0.063   <- 尖峰（4 的倍数）
m=9  0.040
```

密钥长度是 4。8 那个尖峰是同一个答案翻了倍。

```python
def index_of_coincidence(text):
    text = re.sub(r'[^A-Za-z]', '', text).upper()
    n = len(text)
    if n < 2:
        return 0.0
    counts = Counter(text)
    return sum(f * (f - 1) for f in counts.values()) / (n * (n - 1))

def average_ic_for(ciphertext, m):
    text = re.sub(r'[^A-Za-z]', '', ciphertext).upper()
    ics = [index_of_coincidence(text[i::m]) for i in range(m)]
    return sum(ics) / len(ics)

for m in range(1, 21):
    print(m, round(average_ic_for(ciphertext, m), 4))
```

#### 弗里德曼测试

如果你想要一个数而不是一次扫描，弗里德曼公式用整体 IC 估算密钥长度：

```
m ≈ 0.027 × N / ( (N − 1) × IC − 0.038 × N + 0.065 )
```

其中 0.027、0.038、0.065 是由英语字母频率导出的常数。它一次计算给出粗略估计，但在短文本上远比分组 IC 扫描不可靠。**把它当交叉验证，不要当答案。**

### 破解第二步：求出密钥字母

长度已知之后，你就有 `m` 组，每一组都是作用于某个子样本的凯撒密码。第 `i` 组的密钥字母是一个 0 到 25 的位移，而你要问的是：**哪个位移能让这一组的字母频率最像英语。**

**方法一 —— 卡方检验。** 这是标准做法。对每个候选位移，解密该组、统计字母、与英语期望分布比较：

```
χ² = Σ (观测值_i − 期望值_i)² / 期望值_i
```

**χ² 最小的那个位移就是最佳拟合**，因为卡方衡量的是两个分布离得有多远。

**方法二 —— 互重合指数。** 不跟英语比，而是拿两组互相比。能让第 `i` 组的分布与第 `j` 组对齐的位移，给出的是它们**密钥字母之差**。先用卡方确定一组，其余都可以靠差值推出来。

**方法三 —— 手工。** 每组里最常见的字母极可能是 `E`，第二常见的是 `T` 或 `A`。相减得到位移，然后用整段文本验证：如果解出来的字母串是乱码，这一组就错了。

```python
ENGLISH = {
    'A': 8.17, 'B': 1.49, 'C': 2.78, 'D': 4.25, 'E': 12.70, 'F': 2.23,
    'G': 2.02, 'H': 6.09, 'I': 6.97, 'J': 0.15, 'K': 0.77, 'L': 4.03,
    'M': 2.41, 'N': 6.75, 'O': 7.51, 'P': 1.93, 'Q': 0.10, 'R': 5.99,
    'S': 6.33, 'T': 9.06, 'U': 2.76, 'V': 0.98, 'W': 2.36, 'X': 0.15,
    'Y': 1.97, 'Z': 0.07,
}

def chi_squared(text):
    text = re.sub(r'[^A-Za-z]', '', text).upper()
    n = len(text)
    if n == 0:
        return float('inf')
    counts = Counter(text)
    return sum(
        (counts.get(c, 0) - n * ENGLISH[c] / 100) ** 2 / (n * ENGLISH[c] / 100)
        for c in ENGLISH
    )

def find_shift(group):
    """找出让这一组最像英语的那个位移。"""
    best, best_score = None, float('inf')
    for shift in range(26):
        score = chi_squared(caesar(group, -shift))
        if score < best_score:
            best, best_score = shift, score
    return best

def break_vigenere(ciphertext, key_length):
    text = re.sub(r'[^A-Za-z]', '', ciphertext).upper()
    shifts = [find_shift(text[i::key_length]) for i in range(key_length)]
    key = ''.join(chr(s + ord('A')) for s in shifts)
    out, ki = [], 0
    for ch in ciphertext:
        if ch.isalpha():
            base = ord('A') if ch.isupper() else ord('a')
            out.append(chr((ord(ch) - base - shifts[ki % key_length]) % 26 + base))
            ki += 1
        else:
            out.append(ch)
    return key, ''.join(out)
```

**这就是一次完整攻击，四十行左右。** 拿几百个字符的英语、用短密钥加密后跑一遍，密钥和明文都会回来。

### 完整流程走一遍

1. **去掉非字母并转大写**，同时留意原始空格有没有保留。
2. **算整体 IC。** 接近 0.066？回第一篇 —— 那是替换密码。接近 0.038？继续。
3. **扫描候选密钥长度**（分组 IC），找最小的那个尖峰。用卡西斯基的间隔和弗里德曼做交叉验证。
4. **对每一组跑卡方**求出位移，把位移拼成密钥。
5. **解密并阅读。** 如果结果大体可读、只有些片段乱码，说明长度略错（常常是真实长度的倍数）—— 调整后重来。
6. **检验密钥。** 真实密钥通常是个词。如果你解出来的密钥是 `QZXM`，长度大概是错的。

第 6 步不是玩笑：人选出来的密钥是词，而"它看起来像个词吗"是整个过程里最有效的检查之一。

### 各种变体，以及攻击怎么变

**Autokey** 把关键词后面接上明文本身作为滚动密钥。它把统计摊平得如此彻底，以至于 IC 方法完全失效 —— 密钥流不再是周期的。此时有效的是 **crib（已知明文片段）**：猜明文里可能出现的一个词，用它去延伸密钥，看结果读不读得通。任何已知明文都能给你与明文等长的密钥。

**Gronsfeld** 用数字做密钥。分析完全相同，但密钥空间是 `10^m` 而不是 `26^m`，所以垮得更快。

**Beaufort 与 Variant Beaufort** 只是符号变化。同一套分组 IC 与卡方攻击照样有效，只有最后那步算术不同。

**Running key** 用一整段文本做密钥，于是它从不重复、没有周期性。这是古典密码里最难的一种，靠的是 crib dragging 加统计打分，而不是靠公式。

**一次性密码本**是理论的终点：与消息等长的密钥、真随机、只用一次并销毁。它**在信息论上不可破** —— 密文不提供给攻击者任何能区分"这是哪个明文"的信息。它也几乎永远没法用，因为：

- 密钥必须与消息等长（分发问题），
- 必须是真随机（不能是 `random.random()`），
- **绝不能被重用**，而且
- 必须永远保密。

**破坏最后一条是经典失败**，它有个名字：**两次一密（two-time pad）**。用同一段密钥流加密两条消息：

```
C1 = P1 ⊕ K
C2 = P2 ⊕ K
C1 ⊕ C2 = P1 ⊕ P2       （密钥完全抵消了）
```

你永远拿不到密钥，而你也不需要。`P1 ⊕ P2` 是两段英文的异或，而它是可解的 —— 靠 **crib dragging**：为其中一条消息猜一个词，异或进去，看另一侧会不会产生可读文本。

### 这不是历史，这是你会在生产环境里遇到的同一类 bug

一次性密码本那条规矩，就是今天每一个现代流密码都要遵守的规矩，而违反它的后果，是现实中最具破坏力的密码学缺陷之一。

两个真实案例：

- **AES-GCM 重用 nonce。** GCM 是一种流式构造。用同一密钥重用 nonce，密钥流就重复，于是 `C1 ⊕ C2 = P1 ⊕ P2`，和上面一字不差 —— 与此同时 GCM 的认证也被破坏了。本指南密码学那一篇把这条列为十大实现错误之一；而这一篇就是让你看到**为什么**。
- **WEP。** 802.11 的加密标准用 RC4 配 24 位 IV，这逼得它在几百万个包之后必然重用 —— 往往只是几分钟流量。FMS 以及后来的 PTW 攻击，就是从由此产生的密钥流重用里恢复密钥的。**WEP 正是被这一篇里的数学打掉的**，只是应用在了规模上。

教训可以推广：**只要密钥流重复，密码就没了，无论底层原语多强。** 现代应用从"用个随机 IV"进化到"用一个不可能重复的计数器"和"用会在 nonce 重复时拒绝工作的 AEAD"，正是因为这类失败太常见了。

### 工具与怎么练

| 工具 | 用途 |
|---|---|
| 你自己的脚本 | 值得写一次 —— 上面的 IC 加卡方代码就是全部 |
| **dCode** | 有维吉尼亚与 Gronsfeld 求解器，能自动检测密钥长度 |
| **CyberChef** | 当这种密码只是若干层之一时很好用 |
| `vigenere-solver` 之类 | 图方便；但请先手工做过一次 |

**练习方法：**

1. 取 1000 词的英语，用随机的六字母密钥加密。
2. 跑分组 IC 扫描，确认在 6 处出现尖峰。
3. 用卡方逐组求出密钥字母，确认密钥被完整还原。
4. 换成 12 字母密钥再做一遍，体会统计学需要多少文本才够。
5. 用六字母密钥加密一段**很短**的消息（200 字符），看着方法失效 —— 然后弄清是哪一步断了、为什么。
6. 用同一个密钥加密两条消息，用 crib dragging 破掉它们，**至少手工做一次异或**。

第 6 步是能在真实工作里回本的那一步，因为它和破 nonce 重用是同一次攻击。

### 检测与缓解

- **可推广的检测信号是"重复"。** 重复的密钥流会留下统计痕迹，无论是维吉尼亚密码还是一个实现糟糕的流密码：重复的模式、落在"明文值"与"随机值"之间的 IC，以及比它本该有的周期更短就开始循环的 nonce 或 IV 字段。WEP 上是 24 位 IV；现代协议里则是任何会回绕的计数器、任何来自弱随机源的"随机"IV、以及任何跨多条消息复用的会话密钥。
- **在代码评审里盯住 IV 与 nonce 的管理。** 这个 bug 很少长得像"我们重用了密钥"。它长得像：重连时被重置的计数器、用 `rand()` 生成的随机 IV、由时间戳派生的会话标识，以及"每连接重新生成、但不每消息重新生成"的密钥。最后一种最隐蔽，也最常见。
- **防守侧的协议分析用的就是同一套算术。** 一个 IC 式的度量能告诉你一段流是"有结构"还是"随机"：本该有结构的数据太平坦就可疑，本该加密的数据太有结构也可疑。加密隧道和隐蔽通道就是这么被注意到的。
- **缓解就是那条规矩，而且没有商量余地。** 同一密钥下每条消息一个唯一 nonce；用会在 nonce 重复时安全失败的 AEAD；密钥轮换要设计成"一把密钥绝不为同一用途使用两次"。每个现代库都支持这些；出问题的是用法，而不是库不支持。
- **给设计留一条历史教训**：维吉尼亚被认为不可破，是因为它**看起来复杂**、密钥空间巨大。而它最终是被一次**测量**打掉的，不是什么聪明办法。"它在我看来够复杂"从来不是安全论证，而这个密码是整个领域里对这一点最干净的演示。
