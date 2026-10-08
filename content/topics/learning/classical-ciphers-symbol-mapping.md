---
id: classical-ciphers-symbol-mapping
title_en: Classical Ciphers, Part 4 — Symbol Mapping and Coordinates
title_zh: 古典密码（四）：符号映射与坐标
summary_en: Morse, Bacon, Polybius squares and ADFGX all replace letters with something smaller — dots, digits, or a handful of letters — and that smallness is what gives them away. This entry covers each family with runnable code, and the recognition routine that classifies an unknown symbol set in about a minute.
summary_zh: 摩斯、培根、波利比奥斯方阵、ADFGX 都是把字母换成更小的东西 —— 点、数字，或者一小撮字母 —— 而这份"小"正是它们暴露自己的地方。这一篇逐个讲清这些家族并附可运行代码，再给一套能在一分钟内把未知符号集归类出来的识别流程。
tags: [ctf, classical-cipher, morse, bacon, polybius, adfgx, braille]
tools: [Python, CyberChef, dCode]
attck: [T1140]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### What this family is

Every cipher so far worked on the 26-letter alphabet. This family works differently: it **replaces letters with a smaller symbol set** — dots and dashes, two letters, two digits, two flags, six raised dots.

Two consequences follow, and they shape the whole entry:

1. **These are substitution ciphers underneath.** Morse is a substitution over a 30-odd symbol alphabet; Bacon is a substitution over a 24-letter alphabet expressed in binary. Frequency analysis still applies, which is why short CTF messages in these systems are often solved by recognition rather than statistics.
2. **The small symbol set is a fingerprint.** If a ciphertext uses only two distinct characters, or only the digits 1 to 5, the candidate list is short and you can find the answer by trying the standard mappings. Nobody invents a new one.

That second point is the practical one: this family is the most *recognisable* in classical cryptography, and the skill is classification rather than cryptanalysis.

### Morse code

The one everybody has seen, and the one whose design is worth a moment. Morse is a **variable-length** code, and the lengths were assigned by how common each letter is in English: `E` is a single dot, `T` is a single dash, and rare letters like `Q` and `Z` are four symbols long. That is the same principle as Huffman coding, fifty years early.

**The alphabet:**

```
A .-      H ....    O ---     V ...-    0 -----
B -...    I ..      P .--.    W .--     1 .----
C -.-.    J .---    Q --.-    X -..-    2 ..---
D -..     K -.-     R .-.     Y -.--    3 ...--
E .       L .-..    S ...     Z --..    4 ....-
F ..-.    M --      T -       -- .-.-.-  5 .....
G --.     N -.      U ..-     , --..--  6 -....
                              ? ..--..   7 --...
                              / -..-.    8 ---..
                               .-.-.-    9 ----.
                              - -....-   : ---...
```

**Separators matter.** Letters are separated by spaces, words by `/` or a longer gap. Change the separator convention and the same dots and dashes decode differently — which is the first thing to check when someone's Morse "does not work".

```python
MORSE = {
    'A': '.-',   'B': '-...', 'C': '-.-.', 'D': '-..',  'E': '.',    'F': '..-.',
    'G': '--.',  'H': '....', 'I': '..',   'J': '.---', 'K': '-.-',  'L': '.-..',
    'M': '--',   'N': '-.',   'O': '---',  'P': '.--.', 'Q': '--.-', 'R': '.-.',
    'S': '...',  'T': '-',    'U': '..-',  'V': '...-', 'W': '.--',  'X': '-..-',
    'Y': '-.--', 'Z': '--..',
    '0': '-----', '1': '.----', '2': '..---', '3': '...--', '4': '....-',
    '5': '.....', '6': '-....', '7': '--...', '8': '---..', '9': '----.',
    '.': '.-.-.-', ',': '--..--', '?': '..--..', "'": '.----.', '!': '-.-.--',
    '/': '-..-.',  '(': '-.--.',  ')': '-.--.-', '&': '.-...', ':': '---...',
    ';': '-.-.-.', '=': '-...-',  '+': '.-.-.',  '-': '-....-', '_': '..--.-',
    '"': '.-..-.', '$': '...-..-', '@': '.--.-.',
}

REVERSE = {v: k for k, v in MORSE.items()}

def morse_decode(text, sep=' ', word_sep='/'):
    return ' '.join(
        ''.join(REVERSE.get(sym, '?') for sym in word.split(sep) if sym)
        for word in text.strip().split(word_sep)
    )

def morse_encode(text):
    return ' '.join(MORSE.get(c.upper(), '') for c in text if c.upper() in MORSE)

print(morse_decode('.... . .-.. .-.. --- / .-- --- .-. .-.. -..'))   # HELLO WORLD
print(morse_encode('HELLO WORLD'))
```

**The variants you will meet:**

| Variant | What changed | How to spot it |
|---|---|---|
| **No separators** | `.... . .-..` becomes `......-...` | One long run of dots and dashes |
| **Inverted** | Dots and dashes swapped | Decoding gives nonsense; swap and retry |
| **As bits** | `.`=0, `-`=1 | Two symbols, and the groups are the right lengths |
| **As a tree** | The code is a binary tree; left dot, right dash | Puzzle descriptions about paths or branching |
| **Fractionated** | Morse lengths used as numbers (see ADFGX below) | The *counts* matter, not the symbols |

**Decoding without separators** is the interesting case, and it is a search problem rather than a lookup. A run of dots and dashes can be cut into codewords in many ways; the answer is the segmentation that produces sensible text:

```python
from functools import lru_cache

MAXLEN = max(len(v) for v in MORSE.values())

def decode_unseparated(code, max_words=None):
    """Every segmentation that uses only valid Morse codewords."""
    results = []

    def walk(i, acc):
        if max_words and len(results) >= max_words:
            return
        if i == len(code):
            results.append(''.join(acc))
            return
        for n in range(1, MAXLEN + 1):
            chunk = code[i:i+n]
            if chunk in REVERSE:
                acc.append(REVERSE[chunk])
                walk(i + n, acc)
                acc.pop()

    walk(0, [])
    return results

# rank the candidates by how much they look like English
COMMON = ['the', 'and', 'ing', 'ion', 'ent', 'for', 'her', 'tha']
def best_unseparated(code):
    cands = decode_unseparated(code, max_words=5000)
    return sorted(cands, key=lambda s: -sum(s.lower().count(w) for w in COMMON))[:10]

for c in best_unseparated('.....-...'):
    print(c)
```

That is the general shape of every "unseparated Morse" CTF challenge: enumerate segmentations, score them, look at the top few.

**Recognising Morse** takes two seconds: **only two distinct symbols, maximum run length four, and both symbols appear in short groups.**

### Bacon's cipher

Bacon's cipher encodes each letter as a group of **five** characters, each being either `A` or `B` — that is, five bits, with 32 possible combinations for a 24-letter alphabet (I/J and U/V were merged in the original).

| Letter | Code | Letter | Code | Letter | Code |
|---|---|---|---|---|---|
| A | aaaaa | I/J | abaaa | R | baaaa |
| B | aaaab | K | abaab | S | baaab |
| C | aaaba | L | ababa | T | baabb |
| D | aaabb | M | ababb | U/V | babaa |
| E | aabaa | N | abbaa | W | babab |
| F | aabab | O | abbab | X | babba |
| G | aabba | P | abbba | Y | babbb |
| H | aabbb | Q | abbbb | Z | bbbbb |

```python
BACON_24 = 'ABCDEFGHIKLMNOPQRSTUWXYZ'   # 24 letters: I/J and U/V merged
BACON_26 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'

def bacon_decode(text, modern=True):
    table = BACON_26 if modern else BACON_24
    bits = [c.lower() for c in text if c.lower() in 'ab']
    out = []
    for i in range(0, len(bits) - 4, 5):
        idx = int(''.join('1' if b == 'b' else '0' for b in bits[i:i+5]), 2)
        if idx < len(table):
            out.append(table[idx])
    return ''.join(out)

def bacon_encode(text, modern=True):
    table = BACON_26 if modern else BACON_24
    return ' '.join(format(table.index(c.upper()), '05b').replace('0', 'a').replace('1', 'b')
                    for c in text if c.upper() in table)

print(bacon_encode('HELLO'))          # aabbb aabaa ababa ababa abbab
print(bacon_decode('aabbbaabaaababaababaabbab'))
```

**The two versions matter** and they are a common source of wrong answers: the original 24-letter table merges I/J and U/V, the modern 26-letter one does not. If your decode produces a plausible word with one odd letter, try the other table.

**The real point of Bacon** is that `A` and `B` do not have to be the letters A and B. They can be **any binary distinction**: lowercase versus uppercase, one font versus another, bold versus normal, one spelling versus another. That makes it a **steganographic** carrier, and it is how Bacon shows up in CTF: a paragraph of ordinary-looking text where the capitalisation, not the content, carries the message.

If you suspect it: take the text, map "capital" to `B` and "lowercase" to `A`, and group by five.

### The Polybius square, and every coordinate cipher

A 5×5 grid holding the alphabet (again with I/J merged), where each letter is addressed by **row and column**:

```
     1  2  3  4  5
1    A  B  C  D  E
2    F  G  H  I  K
3    L  M  N  O  P
4    Q  R  S  T  U
5    V  W  X  Y  Z
```

So `B` is 12, `R` is 42, and a message becomes pairs of digits from 1 to 5. This is the ancestor of every coordinate encoding, and once you see it you will see it everywhere: two numbers per character, values limited to 1–5 (or 1–6 for the 6×6 version that includes digits).

```python
def polybius_square(key=''):
    seen, alphabet = set(), []
    for ch in key.upper() + 'ABCDEFGHIKLMNOPQRSTUVWXYZ':
        if ch not in seen and ch != 'J':
            seen.add(ch)
            alphabet.append(ch)
    return [alphabet[i*5:(i+1)*5] for i in range(5)]

def polybius_encode(text, key=''):
    square = polybius_square(key)
    pos = {c: (r + 1, cidx + 1) for r, row in enumerate(square) for cidx, c in enumerate(row)}
    return ' '.join(f'{pos[c.upper()][0]}{pos[c.upper()][1]}' for c in text if c.upper() in pos)

def polybius_decode(text, key=''):
    square = polybius_square(key)
    digits = [int(d) for d in text if d.isdigit()]
    return ''.join(square[digits[i]-1][digits[i+1]-1] for i in range(0, len(digits) - 1, 2))

print(polybius_encode('HELLO'))          # 23 15 31 31 34
print(polybius_decode('23 15 31 31 34')) # HELLO
```

**Variants:**

| Variant | Difference |
|---|---|
| **Keyed square** | A keyword fills the grid first, scrambling the alphabet |
| **6×6** | Adds the digits 0–9, so nothing is merged |
| **Tap code** | The digits are transmitted as **knocks** — row count, pause, column count. Used by prisoners of war because it needs nothing but a wall |
| **Bifid / Trifid** | Polybius coordinates, then a transposition of the coordinate stream before converting back — a hybrid, and much stronger than plain Polybius |
| **Nihilist** | Polybius plus a numeric key, added to each coordinate |

Tap code is worth knowing because it is the canonical example of a cipher that needs **no tools at all**: five taps, pause, three taps means row 5 column 3, which is `X`. If a puzzle describes knocking, tapping, or counting, that is what it means.

### ADFGX and ADFGVX

This is the interesting one, because it combines both operations from the previous entries — and it was used for real, by the German army in 1918, until Georges Painvin broke it.

**Why those letters.** `A`, `D`, `F`, `G`, `X` (and later `V`) were chosen because **their Morse representations are maximally distinct**: `.-`, `-..`, `..-.`, `--.`, `-..-`. On a noisy telegraph line, confusing them was unlikely. It is a rare case of a cipher designed around its transmission channel rather than its mathematics.

**The ADFGX version:**

1. Build a 5×5 Polybius square with a keyword and the alphabet.
2. Replace each plaintext letter with its two coordinates, expressed in the letters ADFGX instead of digits — so `B` at row 1 column 2 becomes `AF`.
3. Apply a **columnar transposition** with a second keyword (Part 3's cipher) to the resulting stream.

So the ciphertext is only ever the five letters `A D F G X`, in groups whose length is twice the message, then permuted.

```python
ADFGVX_ALPHABET = 'ADFGVX'

def adfgx_square(key=''):
    return polybius_square(key)          # same 5x5, coordinates then mapped to letters

def adfgx_encode(text, square_key='', trans_key=''):
    square = polybius_square(square_key)
    pos = {c: (r, cidx) for r, row in enumerate(square) for cidx, c in enumerate(row)}
    coords = ''.join(ADFGVX_ALPHABET[pos[c.upper()][0]] + ADFGVX_ALPHABET[pos[c.upper()][1]]
                     for c in text.upper() if c.upper() in pos)
    return columnar_encrypt(coords, trans_key)      # from part 3

def adfgx_decode(text, square_key='', trans_key=''):
    square = polybius_square(square_key)
    coords = columnar_decrypt(text, trans_key)
    out = []
    for i in range(0, len(coords) - 1, 2):
        r = ADFGVX_ALPHABET.index(coords[i])
        c = ADFGVX_ALPHABET.index(coords[i+1])
        out.append(square[r][c])
    return ''.join(out)
```

**The ADFGVX version** uses a 6×6 square with the digits included, and six letters `A D F G V X`, giving 36 symbols.

**How it is attacked**, and why this matters: ADFGVX is two ciphers stacked, so it is broken in two stages, outside in.

1. **Break the transposition first.** The ciphertext uses only six letters, so the *letter* frequencies tell you nothing about the plaintext — but the **coordinate-pair frequencies** do: whatever pair appears most often corresponds to whatever plaintext letter appears most often. More practically, the transposition is a columnar cipher, so the Part 3 techniques (guessing the period, digraph scoring, crib dragging) apply directly. Painvin broke it by crib dragging, using the repetition of German military phrases to align the columns.
2. **Then read the square.** With the transposition removed, you have pairs of ADFGVX letters. Count the pair frequencies and map the most common pair to `E`, then use the fact that **the square is a grid**: if `AF` is `E`, then `A?` and `?F` predict other letters by position. The plaintext structure recovers the square.

That two-stage attack — strip the outer layer, then solve the inner one with frequency analysis — is how every composed cipher is broken, and it is the reason stacking ciphers is not as strong as it looks.

### The rest of the family, by recognition signature

You will meet these in puzzles and CTF, usually in a single line of a description. Learn the signature and look up the details when you need them.

| Cipher | Signature | Idea |
|---|---|---|
| **Semaphore** | Descriptions of flags, arms, or angles; numbers 1–8 | Two flags at one of eight positions each, so 64 combinations |
| **Pigpen (Masonic)** | Grid or X-shaped glyphs | The alphabet is placed in nine tic-tac-toe and X cells; each letter is the cell plus a dot or not |
| **Braille** | Groups of six dots; 6-bit patterns | Six raised/not-raised positions give 64 symbols; a binary code like Bacon's |
| **NATO alphabet** | Whole words: Alpha, Bravo, Charlie… | Spoken-alphabet substitution; useful for reading out hex codes |
| **Book cipher** | Triples like `3-14-2` | Page, line, word — the key is the book |
| **Morse tree** | Descriptions of left/right, dots/dashes as paths | The Morse table *is* a binary tree; a path spells a letter |
| **Bit-level** | Five-bit groups | Bacon; also Baudot/ITA2, the old teleprinter code |

### The recognition routine

This is the part worth remembering, because it works on things not listed here too. Given an unknown ciphertext, in about a minute:

**1. Count the distinct symbols.**

- **Two symbols** (dots/dashes, A/B, 0/1, upper/lower) → Morse, Bacon, or plain binary. Check group lengths.
- **Five or six letters** → ADFGX/ADFGVX (or a Polybius square spelled out). Check whether the letters are among `A D F G V X`.
- **Digits 1–5 only** → Polybius or tap code. Check whether the length is even.
- **Digits 1–6, or 0–5** → the 6×6 variant.
- **Graphical glyphs** → pigpen, semaphore, or braille. Check the number of strokes or dots.
- **Letters, full alphabet, normal frequencies** → substitution family (Parts 1 to 3).

**2. Look at the lengths.**

- Every group is 5 → Bacon (or Baudot).
- The stream is all pairs → Polybius, tap code, ADFGX before transposition.
- Variable-length groups with separators → Morse.
- Even length overall, no grouping → coordinates without separators.

**3. Try the standard mapping before inventing anything.**

Almost every one of these appears in its standard form. Decode with the textbook table first; if the result is *nearly* readable (some letters right, some nonsense), you are probably one small thing off — the 24- vs 26-letter Bacon table, dots and dashes inverted, a different separator convention, or a keyed square where the puzzle told you the key and you skipped it.

**4. Treat the structure as the message.** If the content is ordinary text and the *shape* is not — capitalisation, spacing, font, a repeated pattern of dots — the carrier is the structure, and the cipher is Bacon or a Morse-in-audio variant.

### Tools, and how to practise

| Tool | Use |
|---|---|
| **CyberChef** | Has Morse, Bacon, ADFGVX and a Polybius-style "From Base" set; good for chaining |
| **dCode** | The widest catalogue of these, with variants separated properly |
| Your own table | Worth writing once: encoding a message and decoding it back catches every wrong assumption about separators |

**Practice routine:**

1. Encode a sentence in Morse, then remove all separators, then write a script that finds the top ten segmentations. Repeat until the search feels routine.
2. Take a paragraph of English, hide a Bacon message in its capitalisation, and hand it to someone else to find.
3. Do ADFGX end to end: build the square, encode, transpose, then break your own ciphertext using only the pair frequencies.
4. Take an unknown CTF challenge from the "misc" category and run the four-step recognition routine out loud before touching a tool.

### Detection and mitigation

- **These appear in two places in real work, and both are about carriers rather than secrets.** The first is steganography: Bacon hiding a message in capitalisation, Morse hidden in audio or in spacing. The detection question there is not "what does it decode to" but "why is this text's formatting irregular" — inconsistent capitalisation in a document, a font that changes mid-paragraph, white space that carries a pattern. Formatting is what these carriers cannot hide.
- **The second is signalling.** Morse is not obsolete: it is still used in aviation navigation beacons and amateur radio, and the tap code is the classic example of communicating with no equipment. On the defensive side, that means the signal is **a pattern in timing or tone**, which is why the audio entry's detection advice — look for signalling tones in channels where nobody should be signalling — applies directly here.
- **For detection engineering, the generalisable lesson is that payload can live in structure.** A message encoded in the shape of a document needs no encryption and leaves no content to inspect. If you are building monitoring for data leaving an environment, the checks that catch this are structural: unexpected formatting changes, unusual whitespace or encoding, and files whose *shape* differs from what their type should produce. Content inspection alone will not see any of it.
- **And do not mistake any of this for protection.** Every cipher in this entry is recognised in seconds and decoded in minutes. They are puzzles, teaching tools, covert signallers and CTF material — never a way to protect anything. If you find one of these guarding real data, the finding is that the data was never protected at all.

<!-- lang:zh -->
### 这一类是什么

到上一篇为止，所有密码都是围着 26 个字母做文章。这一类不一样：它**把字母换成更小的符号集** —— 点和划、两个字母、两个数字、两面旗、六个凸点。

由此带来两个后果，而它们决定了这一篇的全部结构：

1. **它们底下仍然是替换密码。** 摩斯是在一个三十来个符号的字母表上做替换；培根是在一个 24 字母表上做替换，只是用二进制表达。频率分析照样适用 —— 这也是为什么这些体系里短消息在 CTF 里往往靠"认出来"而不是靠统计来解决。
2. **符号集小就是指纹。** 如果密文只用了两种不同的字符，或者只用了 1 到 5 这几个数字，候选名单就很短，你只要把标准映射挨个试一遍就能得到答案。**没有人会自己发明一套新的。**

第二点是实用的那一点：这一类是古典密码里**最好认**的，而这里的技能是**分类**，不是密码分析。

### 摩斯电码

所有人都见过的那一个，而它的设计值得停一下。摩斯是**变长**编码，而每个字母的长度是按它在英语里的常见程度分配的：`E` 是一个点、`T` 是一个划，而 `Q`、`Z` 这种罕见的字母要四个符号。这和赫夫曼编码是同一个原理，只是早了五十年。

**码表：**

```
A .-      H ....    O ---     V ...-    0 -----
B -...    I ..      P .--.    W .--     1 .----
C -.-.    J .---    Q --.-    X -..-    2 ..---
D -..     K -.-     R .-.     Y -.--    3 ...--
E .       L .-..    S ...     Z --..    4 ....-
F ..-.    M --      T -       -- .-.-.-  5 .....
G --.     N -.      U ..-     , --..--  6 -....
                              ? ..--..   7 --...
                              / -..-.    8 ---..
                               .-.-.-    9 ----.
                              - -....-   : ---...
```

**分隔符很要紧。** 字母之间用空格分隔，单词之间用 `/` 或更长的间隔。分隔约定一改，同一串点和划就解出不同的东西 —— 而这就是"别人的摩斯解不出来"时该检查的第一件事。

```python
MORSE = {
    'A': '.-',   'B': '-...', 'C': '-.-.', 'D': '-..',  'E': '.',    'F': '..-.',
    'G': '--.',  'H': '....', 'I': '..',   'J': '.---', 'K': '-.-',  'L': '.-..',
    'M': '--',   'N': '-.',   'O': '---',  'P': '.--.', 'Q': '--.-', 'R': '.-.',
    'S': '...',  'T': '-',    'U': '..-',  'V': '...-', 'W': '.--',  'X': '-..-',
    'Y': '-.--', 'Z': '--..',
    '0': '-----', '1': '.----', '2': '..---', '3': '...--', '4': '....-',
    '5': '.....', '6': '-....', '7': '--...', '8': '---..', '9': '----.',
    '.': '.-.-.-', ',': '--..--', '?': '..--..', "'": '.----.', '!': '-.-.--',
    '/': '-..-.',  '(': '-.--.',  ')': '-.--.-', '&': '.-...', ':': '---...',
    ';': '-.-.-.', '=': '-...-',  '+': '.-.-.',  '-': '-....-', '_': '..--.-',
    '"': '.-..-.', '$': '...-..-', '@': '.--.-.',
}

REVERSE = {v: k for k, v in MORSE.items()}

def morse_decode(text, sep=' ', word_sep='/'):
    return ' '.join(
        ''.join(REVERSE.get(sym, '?') for sym in word.split(sep) if sym)
        for word in text.strip().split(word_sep)
    )

def morse_encode(text):
    return ' '.join(MORSE.get(c.upper(), '') for c in text if c.upper() in MORSE)

print(morse_decode('.... . .-.. .-.. --- / .-- --- .-. .-.. -..'))   # HELLO WORLD
print(morse_encode('HELLO WORLD'))
```

**你会遇到的变体：**

| 变体 | 改了什么 | 怎么看出来 |
|---|---|---|
| **无分隔符** | `.... . .-..` 变成 `......-...` | 一整串连续的点和划 |
| **点划互换** | 点划对调 | 解出来是乱码；换回来再试 |
| **当作比特** | `.`=0、`-`=1 | 两种符号，且分组长度对得上 |
| **当作树** | 摩斯表本身就是一棵二叉树，左点右划 | 题面提到"路径""分支" |
| **分馏式** | 用摩斯的**长度**当数字（见下面的 ADFGX） | 重要的是**数量**，不是符号 |

**没有分隔符时的解码**才是有意思的情况，而它是一个**搜索问题**，不是查表问题。一串点和划可以有很多种切法；答案就是那个能切出通顺文本的切法：

```python
from functools import lru_cache

MAXLEN = max(len(v) for v in MORSE.values())

def decode_unseparated(code, max_words=None):
    """枚举所有只使用合法摩斯码字的切分。"""
    results = []

    def walk(i, acc):
        if max_words and len(results) >= max_words:
            return
        if i == len(code):
            results.append(''.join(acc))
            return
        for n in range(1, MAXLEN + 1):
            chunk = code[i:i+n]
            if chunk in REVERSE:
                acc.append(REVERSE[chunk])
                walk(i + n, acc)
                acc.pop()

    walk(0, [])
    return results

# 按"有多像英语"给候选排序
COMMON = ['the', 'and', 'ing', 'ion', 'ent', 'for', 'her', 'tha']
def best_unseparated(code):
    cands = decode_unseparated(code, max_words=5000)
    return sorted(cands, key=lambda s: -sum(s.lower().count(w) for w in COMMON))[:10]

for c in best_unseparated('.....-...'):
    print(c)
```

这就是每一道"无分隔摩斯"CTF 题的通用形状：**枚举切分、打分、看前几名。**

**认出摩斯**只要两秒：**只有两种符号、连续长度最大是四、且两种符号都以小组形式出现。**

### 培根密码

培根密码把每个字母编码成**五个**字符的一组，每个字符非 `A` 即 `B` —— 也就是五个比特，32 种组合，对应一个 24 字母的字母表（原始版本把 I/J、U/V 合并了）。

| 字母 | 编码 | 字母 | 编码 | 字母 | 编码 |
|---|---|---|---|---|---|
| A | aaaaa | I/J | abaaa | R | baaaa |
| B | aaaab | K | abaab | S | baaab |
| C | aaaba | L | ababa | T | baabb |
| D | aaabb | M | ababb | U/V | babaa |
| E | aabaa | N | abbaa | W | babab |
| F | aabab | O | abbab | X | babba |
| G | aabba | P | abbba | Y | babbb |
| H | aabbb | Q | abbbb | Z | bbbbb |

```python
BACON_24 = 'ABCDEFGHIKLMNOPQRSTUWXYZ'   # 24 个字母：I/J 与 U/V 合并
BACON_26 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ'

def bacon_decode(text, modern=True):
    table = BACON_26 if modern else BACON_24
    bits = [c.lower() for c in text if c.lower() in 'ab']
    out = []
    for i in range(0, len(bits) - 4, 5):
        idx = int(''.join('1' if b == 'b' else '0' for b in bits[i:i+5]), 2)
        if idx < len(table):
            out.append(table[idx])
    return ''.join(out)

def bacon_encode(text, modern=True):
    table = BACON_26 if modern else BACON_24
    return ' '.join(format(table.index(c.upper()), '05b').replace('0', 'a').replace('1', 'b')
                    for c in text if c.upper() in table)

print(bacon_encode('HELLO'))          # aabbb aabaa ababa ababa abbab
print(bacon_decode('aabbbaabaaababaababaabbab'))
```

**两个版本的区别很重要**，也是答错的常见原因：原始 24 字母表合并 I/J 和 U/V，现代 26 字母表不合并。如果你的解码结果里有一个字母怪怪的、其余像是个词，就换另一张表试试。

**培根真正的意义**在于：`A` 和 `B` **不必是字母 A 和 B**。它们可以是**任何二元区别**：小写与大写、一种字体与另一种、粗体与常规、一种拼法与前一种。这让它成为一种**隐写载体**，也是它在 CTF 里的出现方式：一段看起来很普通的文本，承载信息的是**大小写**，不是内容。

怀疑它的时候：把文本里"大写"映射成 `B`、"小写"映射成 `A`，然后每五个一组。

### 波利比奥斯方阵，以及所有坐标类密码

一个 5×5 的格子装着字母表（同样合并 I/J），每个字母用**行和列**来定位：

```
     1  2  3  4  5
1    A  B  C  D  E
2    F  G  H  I  K
3    L  M  N  O  P
4    Q  R  S  T  U
5    V  W  X  Y  Z
```

于是 `B` 是 12、`R` 是 42，一条消息变成一对对 1 到 5 的数字。这是所有坐标编码的祖先，而一旦你认出它，就会到处看到它：**每个字符两个数字，取值范围限于 1–5**（含数字的 6×6 版本则是 1–6）。

```python
def polybius_square(key=''):
    seen, alphabet = set(), []
    for ch in key.upper() + 'ABCDEFGHIKLMNOPQRSTUVWXYZ':
        if ch not in seen and ch != 'J':
            seen.add(ch)
            alphabet.append(ch)
    return [alphabet[i*5:(i+1)*5] for i in range(5)]

def polybius_encode(text, key=''):
    square = polybius_square(key)
    pos = {c: (r + 1, cidx + 1) for r, row in enumerate(square) for cidx, c in enumerate(row)}
    return ' '.join(f'{pos[c.upper()][0]}{pos[c.upper()][1]}' for c in text if c.upper() in pos)

def polybius_decode(text, key=''):
    square = polybius_square(key)
    digits = [int(d) for d in text if d.isdigit()]
    return ''.join(square[digits[i]-1][digits[i+1]-1] for i in range(0, len(digits) - 1, 2))

print(polybius_encode('HELLO'))          # 23 15 31 31 34
print(polybius_decode('23 15 31 31 34')) # HELLO
```

**变体：**

| 变体 | 差别 |
|---|---|
| **带密钥方阵** | 先用关键词填格子，把字母表打乱 |
| **6×6** | 加入数字 0–9，于是什么都不用合并 |
| **敲击码（Tap code）** | 数字用**敲击**传达 —— 敲几下表示行、停顿、再敲几下表示列。战俘用它，因为除了墙什么都不需要 |
| **Bifid / Trifid** | 先取坐标，再把坐标流做一次换位，然后转回字母 —— 一种混合体，比裸的波利比奥斯强得多 |
| **Nihilist** | 波利比奥斯加一个数字密钥，逐坐标相加 |

敲击码值得知道，因为它是"**完全不需要任何工具**"的密码的典型：敲五下、停顿、敲三下，意思是第 5 行第 3 列，也就是 `X`。如果题面在描述敲击、拍打或者计数，指的就是它。

### ADFGX 与 ADFGVX

这是有意思的那一个，因为它把前两篇里的两种操作结合了起来 —— 而且它是真的被用过，1918 年德军在用，直到乔治·潘万把它破掉。

**为什么偏偏是这几个字母。** `A`、`D`、`F`、`G`、`X`（后来又加 `V`）被选中，是因为**它们的摩斯表示差别最大**：`.-`、`-..`、`..-.`、`--.`、`-..-`。在嘈杂的电报线上，把它们互相听错的可能性最小。这是密码设计围绕**传输信道**、而不是围绕数学的罕见例子。

**ADFGX 版本：**

1. 用一个关键词加字母表，构造 5×5 的波利比奥斯方阵。
2. 把每个明文字母换成它的两个坐标，但坐标用 `ADFGX` 这五个字母表示，而不是数字 —— 比如第 1 行第 2 列的 `B` 变成 `AF`。
3. 对得到的字符流施加**列置换**（第三篇的密码），用第二个关键词。

所以密文永远只有 `A D F G X` 五个字母，长度是消息的两倍，而且被打乱过。

```python
ADFGVX_ALPHABET = 'ADFGVX'

def adfgx_square(key=''):
    return polybius_square(key)          # 同一个 5x5，只是坐标再映射成字母

def adfgx_encode(text, square_key='', trans_key=''):
    square = polybius_square(square_key)
    pos = {c: (r, cidx) for r, row in enumerate(square) for cidx, c in enumerate(row)}
    coords = ''.join(ADFGVX_ALPHABET[pos[c.upper()][0]] + ADFGVX_ALPHABET[pos[c.upper()][1]]
                     for c in text.upper() if c.upper() in pos)
    return columnar_encrypt(coords, trans_key)      # 来自第三篇

def adfgx_decode(text, square_key='', trans_key=''):
    square = polybius_square(square_key)
    coords = columnar_decrypt(text, trans_key)
    out = []
    for i in range(0, len(coords) - 1, 2):
        r = ADFGVX_ALPHABET.index(coords[i])
        c = ADFGVX_ALPHABET.index(coords[i+1])
        out.append(square[r][c])
    return ''.join(out)
```

**ADFGVX 版本**用的是 6×6 方阵（含数字）和六个字母 `A D F G V X`，共 36 个符号。

**它是怎么被破的**，而这一点很重要：ADFGX 是两层密码叠起来的，所以要从外往里分两步破。

1. **先破换位。** 密文只用了六个字母，所以**字母**频率对明文一无所知 —— 但**坐标对的**频率有信息：出现最多的那一对，对应的就是出现最多的那个明文字母。更实用的是，那层换位就是列置换，所以第三篇的方法（猜周期、双字母组打分、crib dragging）直接可用。潘万就是靠 crib dragging 破的：利用德军军事用语里的重复片段来对齐各列。
2. **然后读方阵。** 去掉换位之后，你手上是一对对 ADFGVX 字母。统计坐标对的频率，把最常见的那一对判给 `E`，然后利用**方阵是网格**这一点：如果 `AF` 是 `E`，那么 `A?` 和 `?F` 就按位置预测出别的字母。明文结构会把方阵还原出来。

**"剥掉外层、再用频率分析解决内层"这个两步攻击**，是所有复合密码被攻破的方式，也正是"把密码叠起来并没有看起来那么强"的原因。

### 这一族的其余成员，按识别特征列

你会在谜题和 CTF 里遇到它们，通常只在题面里出现一行。记住特征，需要细节时再查。

| 密码 | 特征 | 想法 |
|---|---|---|
| **旗语（Semaphore）** | 描述旗帜、手臂或角度；数字 1–8 | 两面旗各处在八个位置之一，共 64 种组合 |
| **猪圈密码（Pigpen）** | 井字格或叉形图案 | 字母放进九个井字格与叉格里；每个字母是"哪个格 + 有没有点" |
| **盲文（Braille）** | 六个点的组合；6 位模式 | 六个位置的凸/不凸给出 64 个符号；和培根一样是二进制编码 |
| **北约字母表** | 完整的词：Alpha、Bravo、Charlie… | 口语化替换；读十六进制串时有用 |
| **书本密码（Book cipher）** | 三组数字，如 `3-14-2` | 页、行、词 —— 密钥是那本书 |
| **摩斯树** | 描述左/右、点/划作为路径 | 摩斯表本身就是二叉树；一条路径拼出一个字母 |
| **比特级** | 五位一组 | 培根；还有 Baudot/ITA2，老式电传打字机编码 |

### 识别流程

这是最值得记住的部分，因为它对没列在这里的东西也管用。拿到一段未知密文，一分钟内做完：

**1. 数不同的符号有几个。**

- **两种符号**（点划、A/B、0/1、大写小写）→ 摩斯、培根，或纯二进制。看分组长度。
- **五或六个字母** → ADFGX/ADFGVX（或者是用字母写出来的波利比奥斯坐标）。看这些字母是不是在 `A D F G V X` 里面。
- **只有数字 1–5** → 波利比奥斯或敲击码。看长度是不是偶数。
- **数字 1–6 或 0–5** → 6×6 版本。
- **图形符号** → 猪圈、旗语或盲文。数笔划或点的个数。
- **整张字母表、频率正常** → 替换家族（第一到第三篇）。

**2. 看长度。**

- 每组都是 5 个 → 培根（或 Baudot）。
- 整串都是成对的 → 波利比奥斯、敲击码、换位之前的 ADFGX。
- 变长分组且有分隔符 → 摩斯。
- 总长度是偶数、没有分组 → 没带分隔符的坐标。

**3. 先试标准映射，别急着发明东西。**

这些东西几乎总是以标准形式出现。先用教科书上的表解一遍；如果结果**差不多**能读（有些字母对、有些是乱码），你多半只差一个小地方 —— 培根是 24 字母表还是 26 字母表、点划是不是反的、分隔约定不同，或者是一个需要密钥的方阵而题面给了密钥你却漏了。

**4. 把结构本身当成消息。** 如果内容很普通、而**形状**不普通 —— 大小写、空格、字体、重复的点状图案 —— 那么载体就是结构，而密码是培根或者音频里的摩斯。

### 工具与怎么练

| 工具 | 用途 |
|---|---|
| **CyberChef** | 有摩斯、培根、ADFGVX 以及一套"From Base"式的波利比奥斯操作；串起来用很方便 |
| **dCode** | 这一类里目录最全，而且把变体分得很清楚 |
| 你自己的表 | 值得写一次：把一个消息编码再解回来，能抓出你对分隔符的每一个错误假设 |

**练习方法：**

1. 用摩斯编码一句话，然后删掉所有分隔符，再写脚本找出前十种切分。重复到你对手工搜索感到平常为止。
2. 取一段英文，把培根消息藏进它的大小写里，然后交给别人去找。
3. 把 ADFGX 完整做一遍：建方阵、编码、换位，然后**只用坐标对频率**破掉自己造的密文。
4. 从 CTF 的 misc 分类里挑一道没做过的题，动手用工具之前，先把那四步识别流程念一遍。

### 检测与缓解

- **它们在真实工作里出现在两个地方，而两处都是关于"载体"而不是"秘密"。** 第一处是隐写：培根藏在大小写里、摩斯藏在音频或空格里。那里的检测问题不是"它解出来是什么"，而是"**这段文本的格式为什么不规整**" —— 文档里忽大忽小的字母、段落中间变掉的字体、带着图案的空格。**格式是这些载体藏不住的东西。**
- **第二处是信号。** 摩斯并没有过时：它仍在航空导航信标和业余无线电里使用，而敲击码是"没有设备也能通信"的经典例子。对防守方来说，这意味着信号表现为**时序或音调上的图案** —— 这也是音频那一篇的检测建议在这里直接适用的原因：**在没有人在发信号的频道里，去找信号音。**
- **对检测工程来说，可推广的教训是"载荷可以活在结构里"。** 一个编码在文档形状里的消息不需要加密，也不留下任何可供检查的内容。如果你在为离开环境的数据做监控，能抓到它的检查都是结构性的：意料之外的格式变化、异常的空白或编码，以及那些**形状**与它应有的类型不符的文件。只做内容检查，这些一样都看不见。
- **最后，别把这里任何东西当成保护。** 这一篇里的每一种密码都是几秒内被认出、几分钟内被解出。它们是谜题、教学工具、隐蔽通信手段和 CTF 素材 —— 永远不是保护东西的办法。如果你发现这些东西在守着真实数据，那么结论是：**那些数据从来没有被保护过。**
