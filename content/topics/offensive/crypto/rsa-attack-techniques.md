---
id: rsa-attack-techniques
title_en: RSA Attack Techniques
title_zh: RSA 攻击手法
summary_en: RSA is never broken by brute force, it is broken by what the implementation gave away — a small exponent with no padding, two moduli sharing a prime, a private exponent that is too small, or a padding check that answers yes or no.
summary_zh: RSA 从来不是被暴力破解的，而是被实现泄露的东西击穿的 —— 无填充的小指数、共享同一个素数的两个模数、过小的私钥指数，或者一个会回答"是/否"的填充校验。
tags: [crypto, rsa, ctf, cryptography, padding-oracle]
tools: [RsaCtfTool, SageMath, yafu, factordb, openssl]
attck: [T1552.004]
platform: [any]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### Read the parameters before attacking anything

Almost every RSA challenge or finding is decided by a handful of properties. Check them in this order — it takes a minute and saves hours.

| What you have | Look at | Likely attack |
|---|---|---|
| `n, e, c` with a small `e` (3, 5, 17) | Is the message short? Is there padding? | Small-exponent root, Håstad broadcast |
| The same `n` used with two different `e` | `gcd(e1, e2) == 1` | Common modulus |
| Many moduli `n1, n2, n3…` | Do any share a factor? | Batch GCD, then factor |
| `n` alone | Size, and whether p and q are close | Factordb, Fermat |
| `n, e` with a very small `d` | Is `d < n^0.25`? | Wiener's attack |
| A decryption oracle that returns errors | Does it behave differently on bad padding? | Bleichenbacher / padding oracle |
| Part of `p` or `m` known | Bits, structure | Coppersmith |

```bash
# the fastest first step on any CTF RSA: hand it to a tool and read what it tries
python3 RsaCtfTool.py --publickey key.pub --uncipherfile flag.enc
python3 RsaCtfTool.py --publickey key.pub --private   # recover the key itself

# check whether n is already factored by someone else
curl -s "http://factordb.com/api?query=$N" | jq .
```

### The classics, with the condition that makes each one work

**1. Small `e` with no padding.** If `m^e < n`, the ciphertext is not reduced modulo anything, so an integer `e`-th root recovers the plaintext outright:

```python
from gmpy2 import iroot
m, exact = iroot(c, 3)      # e = 3
assert exact
```

Even when `m^e > n` slightly, a small brute-force over `k` in `c + k*n` often lands on a perfect cube.

**2. Håstad's broadcast.** The same message encrypted to `e` different recipients with exponent `e` and no padding can be recovered with CRT, because the message is smaller than the product of the moduli. Padding defeats this completely — which is exactly why padding exists.

**3. Common modulus.** Two ciphertexts of the same message under the same `n` with coprime exponents `e1, e2`: extended Euclid gives `a*e1 + b*e2 = 1`, so `c1^a * c2^b = m (mod n)`.

**4. Shared prime.** Two moduli that share a factor fall to a single `gcd`. Generate a few hundred keys and check all pairs — this happens in the wild with weak randomness on embedded devices:

```python
from math import gcd
p = gcd(n1, n2)
q1, q2 = n1 // p, n2 // p
```

**5. Fermat factorisation.** If `p` and `q` were generated close together, `n = a^2 - b^2` with a small `b`:

```python
a = isqrt(n) + 1
while True:
    b2 = a*a - n
    if is_square(b2):
        p, q = a - isqrt(b2), a + isqrt(b2)
        break
    a += 1
```

**6. Wiener's attack.** A small private exponent (`d < n^0.25 / 3`) makes `e/n` a good approximation of a continued fraction convergent of `k/d`. If a developer "optimised" key generation by picking a small `d`, this recovers it in milliseconds.

**7. Known factorisations.** Small moduli (under 512 bits) fall to `yafu` or `msieve`; anything with a published factorisation is in factordb; and a modulus generated with a seeded PRNG can be reproduced.

### Bleichenbacher, the one that keeps coming back

A padding oracle answers a single question: *was the padding valid?* With PKCS#1 v1.5, that answer is enough to decrypt arbitrary ciphertext in a few thousand queries, byte by byte:

1. Send a modified ciphertext.
2. Observe whether the response is a padding error or a decryption error.
3. Use each answer to narrow the range of the plaintext.

This is not a theoretical curiosity. It has been found in real TLS stacks, in VPN appliances, in smart-card protocols, and in any custom protocol where someone wrote `if padding_invalid: return "bad request"`. The countermeasures are equally well known: use OAEP, make all failures look identical (constant time, same error, same timing), and never reveal *why* a decryption failed.

### Coppersmith, and everything about partial knowledge

Coppersmith's method finds small roots of polynomials modulo `n`. In practice it covers a wide family:

- Stereotyped messages: `m = known_prefix || x` with `x` small.
- Partial key exposure: half the bits of `p` or `d` known.
- `e` too large and `d` small in a structured way.

You will not implement this by hand; `SageMath` has it, and CTF writeups are the best reference for shaping the problem into the right polynomial.

### Signatures and nonces

Two mistakes account for most signature breaks, and neither is about the modulus:

- **No padding with a small exponent.** With `e = 3` and a raw signature, forging a "signature" for a chosen message is often just a cube root of a carefully crafted value. Textbook RSA signatures are malleable; PKCS#1 v1.5 with proper verification is not.
- **Reused nonces.** In DSA and ECDSA the per-signature nonce `k` must be unique. Two signatures with the same `k` leak the private key with one subtraction and one modular inverse — this is how several real-world console and cryptocurrency keys were recovered.

```python
# ECDSA nonce reuse, conceptually
k = (z1 - z2) * inverse(s1 - s2, n) % n
d = (s1 * k - z1) * inverse(r, n) % n
```

### Why this rarely works against real TLS

Modern TLS uses 2048- or 4096-bit moduli, OAEP or PSS, random padding, and constant-time error handling — the attacks above are designed out. The places they still land:

- Custom protocols written by application developers ("we encrypted it with RSA").
- Firmware and IoT devices with 512-bit keys and a shared prime across a product line.
- Old VPN and smart-card stacks with padding oracles.
- CTF challenges, which are deliberately constructed from one of the conditions above.
- Anything where the key came from a weak PRNG, which also covers the classic "two devices booted at the same second generated the same key".

### Detection and mitigation

Detection is mostly about the oracle: log and alert when decryption failures are *distinguishable* — different response bodies, different error codes, or measurably different timing for padding failures versus other failures. A spike of decryption failures from one source is the signature of a Bleichenbacher attempt.

Mitigation, in order of how much it matters:

- **Use a vetted library and a modern scheme** — OAEP for encryption, PSS for signatures. Do not implement RSA padding yourself.
- **2048 bits minimum**, randomly generated, with `p` and `q` far apart and `d` large.
- **Never reuse a nonce.** Use a deterministic scheme (RFC 6979) or a hardware RNG.
- **Make failures indistinguishable**, and keep the operation constant time.
- **Do not invent a protocol when TLS exists.** Almost every real-world RSA break is a protocol design choice, not a maths failure.

<!-- lang:zh -->
### 先读参数，再谈攻击

几乎每一个 RSA 题目或发现，都由少数几个性质决定。按这个顺序检查一遍，一分钟能省下几个小时。

| 你手里有 | 先看什么 | 可能的攻击 |
|---|---|---|
| `n, e, c` 且 `e` 很小（3、5、17） | 明文是否很短？有没有填充？ | 小指数开根、Håstad 广播 |
| 同一个 `n` 配了两个不同 `e` | `gcd(e1, e2) == 1` | 公共模数攻击 |
| 大量模数 `n1, n2, n3…` | 有没有两两共享因子 | 批量 GCD，然后分解 |
| 只有 `n` | 位数，以及 p 与 q 是否接近 | factordb、Fermat 分解 |
| `n, e` 且 `d` 很小 | `d < n^0.25`？ | Wiener 攻击 |
| 一个会返回错误的解密预言机 | 坏填充时的行为是否不同 | Bleichenbacher / 填充预言机 |
| 已知 `p` 或 `m` 的一部分 | 位数、结构 | Coppersmith |

```bash
# CTF 里 RSA 最快的第一步：丢给工具，看它试了什么
python3 RsaCtfTool.py --publickey key.pub --uncipherfile flag.enc
python3 RsaCtfTool.py --publickey key.pub --private   # 直接恢复私钥

# 看 n 是否已经被别人分解过
curl -s "http://factordb.com/api?query=$N" | jq .
```

### 经典手法，以及每种成立的条件

**1. 无填充的小指数。** 如果 `m^e < n`，密文根本没被取模，直接开 `e` 次整数根就拿到明文：

```python
from gmpy2 import iroot
m, exact = iroot(c, 3)      # e = 3
assert exact
```

即便 `m^e` 略大于 `n`，对 `c + k*n` 中的 `k` 做小范围暴力也常常能撞到完全立方数。

**2. Håstad 广播攻击。** 同一条消息用指数 `e` 加密给 `e` 个不同接收者、且无填充时，可以用 CRT 恢复 —— 因为消息小于所有模数之积。填充能彻底防住它，这正是填充存在的意义。

**3. 公共模数。** 同一条消息在同一个 `n` 下、用互素的 `e1, e2` 加密两次：扩展欧几里得给出 `a*e1 + b*e2 = 1`，于是 `c1^a * c2^b = m (mod n)`。

**4. 共享素数。** 两个共享因子的模数，一次 `gcd` 就倒。生成几百个公钥两两检查 —— 嵌入式设备上熵不足时，这在现实里真的会发生：

```python
from math import gcd
p = gcd(n1, n2)
q1, q2 = n1 // p, n2 // p
```

**5. Fermat 分解。** 如果 `p` 和 `q` 生成时靠得很近，就有 `n = a^2 - b^2` 且 `b` 很小：

```python
a = isqrt(n) + 1
while True:
    b2 = a*a - n
    if is_square(b2):
        p, q = a - isqrt(b2), a + isqrt(b2)
        break
    a += 1
```

**6. Wiener 攻击。** 私钥指数过小（`d < n^0.25 / 3`）时，`e/n` 会成为 `k/d` 的连分数收敛子。如果开发者为了"优化"而挑了个小 `d`，几毫秒就能恢复出来。

**7. 已知的分解结果。** 小于 512 位的模数交给 `yafu` 或 `msieve`；任何被公开分解过的都在 factordb；而用带种子的 PRNG 生成的模数可以被复现出来。

### Bleichenbacher，一个反复回来的漏洞

填充预言机只回答一个问题：**填充是否合法？** 对 PKCS#1 v1.5 来说，这个答案足以在几千次查询内逐字节解出任意密文：

1. 发送一个改动过的密文。
2. 观察返回的是"填充错误"还是"解密错误"。
3. 用每个答案缩小明文所在区间。

这不是理论猎奇。它在真实的 TLS 实现、VPN 设备、智能卡协议，以及任何有人写了 `if padding_invalid: return "bad request"` 的自研协议里都被找到过。对抗手段同样明确：用 OAEP，让所有失败看起来一模一样（恒定时间、同样的错误、同样的耗时），绝不透露解密**为什么**失败。

### Coppersmith，以及一切"部分已知"的情形

Coppersmith 方法求的是模 `n` 多项式的小根。实战中它覆盖一大类情况：

- 有固定前缀的消息：`m = 已知前缀 || x`，且 `x` 很小。
- 部分密钥泄露：已知 `p` 或 `d` 的一半比特。
- `e` 过大而 `d` 有特定结构。

不必手写实现 —— `SageMath` 里有，而 CTF writeup 是把问题整理成正确多项式的最好参考。

### 签名与随机数

签名被破的绝大多数原因只有两个，而且都与模数无关：

- **小指数 + 无填充。** `e = 3` 且对原始值签名时，伪造一条选定消息的"签名"往往就是把一个精心构造的值开三次方。教科书式 RSA 签名是可延展的，而正确校验的 PKCS#1 v1.5 不是。
- **随机数重用。** DSA 和 ECDSA 里每次签名用的 nonce `k` 必须唯一。两条用了同一个 `k` 的签名，一次减法加一次模逆就能算出私钥 —— 现实中若干游戏主机与加密货币的密钥就是这样被恢复的。

```python
# ECDSA nonce 重用，示意
k = (z1 - z2) * inverse(s1 - s2, n) % n
d = (s1 * k - z1) * inverse(r, n) % n
```

### 为什么这些对真实 TLS 基本无效

现代 TLS 用 2048 或 4096 位模数、OAEP 或 PSS、随机填充和恒定时间的错误处理 —— 上面这些攻击在设计阶段就被排除了。它们仍然能落地的场景是：

- 应用开发者自己写的协议（"我们用 RSA 加密了"）。
- 固件与 IoT 设备：512 位密钥，而且同一产品线共用一个素数。
- 老旧的 VPN 与智能卡协议，存在填充预言机。
- CTF 题目，本来就是从上面某个条件构造出来的。
- 任何密钥来自弱 PRNG 的场景，也包括经典的"两台设备在同一秒启动，生成了同一把密钥"。

### 检测与缓解

检测主要围绕预言机：当解密失败**可区分**时就要记录并告警 —— 不同的响应体、不同的错误码，或者坏填充与其他失败之间可测量的耗时差异。来自单一来源的解密失败激增，就是 Bleichenbacher 尝试的特征。

缓解按重要性排序：

- **使用经过验证的库和现代方案** —— 加密用 OAEP，签名用 PSS。不要自己实现 RSA 填充。
- **至少 2048 位**，随机生成，`p` 与 `q` 相距足够远，`d` 足够大。
- **绝不重用 nonce。** 用确定性方案（RFC 6979）或硬件随机源。
- **让失败不可区分**，并保持操作恒定时间。
- **有 TLS 就别自己发明协议。** 现实中几乎每一次 RSA 被破，都是协议设计选择的问题，而不是数学被攻破。
