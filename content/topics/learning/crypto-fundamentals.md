---
id: crypto-fundamentals
title_en: "Cryptographic Misuse — The Maths Is Rarely the Broken Part"
title_zh: "密码学误用：数学很少是被攻破的那一环"
summary_en: Three measured failures of correct primitives — a mode that leaves structure visible, a nonce used twice so one known plaintext recovers the other, and a padding oracle that yields a whole block from a boolean. None of them breaks the cipher.
summary_zh: 三个"原语本身正确、用法失败"的实测 —— 一个把结构留在密文里的模式、一个被用两次的 nonce（于是知道一条明文就能恢复另一条）、以及一个只凭布尔值就交出一整个块的 padding oracle。三者都没有攻破那个密码算法。
tags: [web, cryptography, cwe-327, cwe-329, ecb, padding-oracle]
tools: [python3, cryptography, openssl]
attck: [T1552, T1600]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The part that is almost never broken

Every entry in this guide has a sentence that relocates the problem, and this one is short:

> **The failure is almost never the algorithm. It is how the algorithm was used.**

The maths behind AES has held for decades. What fails is the **mode**, the **initialisation value**, the **error handling**, the **comparison** and the **randomness** — all of which are ordinary code, written by people under time pressure, using APIs whose defaults are not security decisions.

That is why this class is worth a separate entry rather than a paragraph inside another one: the review question is completely different. It is not "is this algorithm strong" but **"what does this API do by default, and does it cover integrity as well as confidentiality"**.

### Four layers

1. **The trust boundary.** The application trusts that "encrypted" means "protected". Encryption protects **confidentiality** — one property out of several — and the mistakes below achieve their result without breaking it.
2. **Data and instruction share a plane.** The ciphertext hides the **content** and leaves the **structure** visible, and structure is information. Two identical blocks of plaintext producing two identical blocks of ciphertext tells an attacker which records match, without any key.
3. **Why the usual fix fails.** A longer key, a newer algorithm, a library upgrade — none of them change the mode, the nonce discipline or whether decryption failures are distinguishable. Reaching for a stronger primitive is the natural response and it addresses none of this.
4. **The variants.** ECB as a mode; a repeated IV or nonce; a padding oracle in any of its observable forms; a non-cryptographic random source; a comparison that leaks by timing; and anything home-made.

### Measured: a mode that keeps structure

Same key, same plaintext containing two identical 16-byte blocks, two modes:

| | Block 0 | Block 2 | Equal? |
|---|---|---|---|
| **ECB** | `9500025f1668a0f3…` | `9500025f1668a0f3…` | **yes** |
| CBC | `83bb277e38b9fa48…` | `714e8498779e2011…` | no |

**ECB encrypts each block independently, which leaves "these two blocks are the same" in the ciphertext.** Nothing was decrypted and no key was needed; the equality survived the encryption. The classic demonstration is an image — the well-known ECB penguin — where the outline remains visible because runs of identical pixels produce runs of identical ciphertext. The same effect applies to structured records: a database column of encrypted values reveals which users share a value.

**CBC differs because each block is xored with the previous ciphertext before being encrypted**, so identical plaintext blocks become different inputs. That is a demonstration of the general principle: **a mode is secure only if identical plaintext does not produce identical ciphertext**, and that requires some per-message or per-block variation.

### Measured: a nonce used twice

Against a stream cipher, encrypting two different messages with the **same nonce** — so the same keystream:

```
ciphertext1 XOR ciphertext2 == plaintext1 XOR plaintext2 : True
recovering message 2 from a known message 1            : 'TRANSFER: to=bob   amount=9999 '
matches the real message 2                             : True
```

**Why it works is one line of algebra**: `c = m XOR keystream`, so `c1 XOR c2 = m1 XOR m2` — the keystream cancels. The attacker never learns the key; they learn the **relationship between two messages**, and as soon as one of them is guessable — a fixed format, a known field, a value they supplied themselves — the other falls out.

**So nonce reuse is not a weakening. It ties two messages together**, which is a different and worse failure mode than "the key is a bit short". The sources of reuse are worth listing because they are all mundane:

| Source | Why it happens |
|---|---|
| A counter that resets on restart | The nonce space restarts with the process |
| Multiple instances sharing a counter | Each believes it has the next value |
| A random nonce that is too short | Collisions become likely at high volume |
| A nonce derived from something repeatable | A timestamp with low resolution, a user id |

**And the same failure applies to authenticated modes.** GCM with a repeated nonce does not merely lose confidentiality — the authentication key can be recovered, which turns a confidentiality failure into a forgery capability. That is why nonce discipline is not a detail of the cipher choice.

### Measured: a padding oracle, from a boolean

The most instructive of the three, because the oracle leaks nothing that looks like information. Measured, against a server that answers exactly one question — **was the padding valid** — and nothing else:

```
recovering the first block, one byte at a time:
  byte 15: 't'   byte 14: 'y'   byte 13: 'b'   byte 12: '6'
  byte 11: '1'   byte 10: '-'   byte  9: 'g'   byte  8: 's'
  byte  7: 'm'   byte  6: '-'   byte  5: 't'   byte  4: 'e'
  byte  3: 'r'   byte  2: 'c'   byte  1: 'e'   byte  0: 's'

recovered block : b'secret-msg-16byt'
real block      : b'secret-msg-16byt'
identical       : True
```

**The oracle disclosed no key and no plaintext.** It answered one bit, about a property of the decrypted result that the protocol itself requires the server to check. The attack is that the answer is **observable**, and that a modified ciphertext block lets the attacker probe the intermediate value one byte at a time: by changing the preceding block so that the last byte decrypts to `0x01`, the padding is "valid", and the byte that achieved it reveals the intermediate byte, from which the plaintext follows.

**The observable difference does not have to be a message.** All of these are oracles:

| Observable | Oracle |
|---|---|
| A different error message | The classic |
| A different status code | Same information, different channel |
| **A different response time** | Padding checked with a loop that exits early |
| A different response length | An empty body versus an error page |
| A connection closed versus kept | |

**Any difference that distinguishes "padding was valid" from "padding was invalid" is the oracle** — which is why the fix is to make decryption failures indistinguishable rather than to change the cipher.

### Measured: what an authenticated mode changes

The same tampering against AES-GCM, where the ciphertext carries a 16-byte authentication tag:

```
ciphertext length: 33 (16 bytes of plaintext + 16-byte tag)
after flipping one bit: decryption fails, exception InvalidTag
```

**One kind of failure, no distinction between "bad padding" and "bad tag"** — because there is no padding to be bad, and the integrity check happens before anything is interpreted. This is the practical reason to reach for an AEAD mode:

**It covers integrity, so there is no padding oracle to observe.** The class of attack depends on a decryption step that can succeed partially, and AEAD does not have one.

**It removes the assembly step.** The alternative — encrypt, then compute a MAC — has an ordering question, and **MAC-then-encrypt** (what older TLS did) is the arrangement that produced padding oracles in the wild. **Encrypt-then-MAC** is the correct order if you must assemble it yourself; not assembling it at all is better, because the API will not get the order wrong.

### The rest of the family

| Misuse | What it costs |
|---|---|
| A non-cryptographic random source | Predictable tokens — the session entry's failure |
| A fast hash for passwords, or no salt | Offline cracking at scale, and rainbow tables |
| Comparing secrets with `==` | A timing oracle: the comparison exits at the first differing byte |
| Keys in source, shared across environments | Everything above, plus the session entry's revocation problem |
| Anything home-made | All of the above with no one having reviewed it |

The constant-time row is worth a note because it is easy to fix and easy to miss: **comparing a MAC or a token with `==` leaks how many leading bytes matched**, through the time the comparison takes. Libraries provide a constant-time comparison for exactly this reason, and using it is a one-line change.

### Detection and mitigation

- **Grep for `ECB`, for hard-coded or all-zero IVs and nonces, and for `random` where `secrets` is meant.** These are API usages, which means they are visible in a code review rather than only in an exploit — this class is unusually reviewable, and almost none of it needs a running system to find.
- **Alert on distinguishable decryption failures.** Compare the status code, the response body and the response time for "tampered ciphertext" against "valid ciphertext, bad content". A measurable difference in any of the three is the oracle.
- **And watch for a repeated nonce in the logs.** Where a nonce is transmitted or logged, a duplicate is the whole finding — and a counter that restarts after a deploy is the classic source.
- **For mitigation, choose an AEAD mode and never assemble it by hand.** AES-GCM or ChaCha20-Poly1305 gives confidentiality and integrity in one call, with a documented nonce requirement. This is the single decision that removes the padding oracle, the ordering question and the "is the MAC over the right bytes" question at once.
- **Give every message a unique nonce, and make the counter durable.** Persisting the counter across restarts, or using a random nonce long enough that collisions are negligible, addresses the two most common sources of reuse.
- **Make decryption failures indistinguishable.** One status code, one message, one code path, and — where the difference in work is measurable — a constant-time comparison rather than an early exit.
- **Use the library's constant-time comparison for every secret.** MACs, tokens, signatures.
- **Use a password hash designed for passwords.** Argon2 or bcrypt with a per-password salt, not a general-purpose hash and not a single round of anything.
- **Take randomness from the platform's cryptographic source.** If the code path can be seeded, it can be reproduced.
- **And ask the question this entry opened with, because it is the one that finds these.** Not "is this algorithm strong", but **"what does this API do by default, which properties does it cover, and what does a failure look like from the outside"** — the three questions the measured failures above each answer in the wrong way.

<!-- lang:zh -->
### 几乎从不会被攻破的那一部分

这份指南每一篇都有一句话把问题挪个位置，而这一篇的很短：

> **失败几乎从不是算法。是算法被怎么用。**

AES 背后的数学已经站了几十年。失败的是**模式**、**初始值**、**错误处理**、**比较方式**与**随机性** —— 全都是普通代码，由赶时间的人写出来，用着那些默认值并不是安全决定的 API。

这就是为什么这一类值得单独成篇而不是塞进别处的一段：评审的问题完全不同。它不是"这个算法强不强"，而是 **"这个 API 默认做什么，以及它有没有把完整性也一起管了"**。

### 四层

1. **信任边界。** 应用信任"加密了"等于"被保护了"。加密保护的是**机密性** —— 若干性质里的一个 —— 而下面那些失误在完全不破坏机密性的前提下达成它们的结果。
2. **数据与指令共用同一平面。** 密文藏起了**内容**，却把**结构**留在了外面，而结构本身就是信息。两块完全相同的明文产生两块完全相同的密文，等于告诉攻击者哪些记录相同，全程不需要密钥。
3. **为什么常见修法失败。** 更长的密钥、更新的算法、升级库 —— 它们都不改变模式、不改变 nonce 纪律、也不改变"解密失败是否可被区分"。伸手去拿一个更强原语是很自然的反应，而它对这些一条都不解决。
4. **变体。** 作为模式的 ECB；被重复使用的 IV 或 nonce；任何一种可观察形态下的 padding oracle；非密码学的随机源；会因耗时泄漏的比较；以及任何自己发明的东西。

### 实测：一个把结构留在外面的模式

同样的密钥、同样的明文（其中有两块完全相同的 16 字节），两种模式：

| | 块 0 | 块 2 | 相同吗 |
|---|---|---|---|
| **ECB** | `9500025f1668a0f3…` | `9500025f1668a0f3…` | **相同** |
| CBC | `83bb277e38b9fa48…` | `714e8498779e2011…` | 不同 |

**ECB 对每个块独立加密，于是"这两块一样"这件事被留在了密文里。** 没有解密任何东西，也不需要密钥；那个相等关系活过了加密。经典的演示是一张图片 —— 那只著名的 ECB 企鹅 —— 它的轮廓仍然可见，因为成片相同的像素产生了成片相同的密文。同一个效应作用在结构化记录上：一个装着加密值的数据库列会暴露哪些用户共享同一个值。

**CBC 之所以不同，是因为每一块在被加密前先与前一块密文异或**，于是相同的明文块变成了不同的输入。那是对一条通则的演示：**一个模式只有在"相同明文不产生相同密文"时才是安全的**，而那要求某种逐消息或逐块的差异。

### 实测：一个被用了两次的 nonce

对着一个流密码，用**同一个 nonce** 加密两条不同的消息 —— 也就是同一段密钥流：

```
密文1 XOR 密文2 == 明文1 XOR 明文2 : True
用已知的明文1反推明文2            : 'TRANSFER: to=bob   amount=9999 '
与真实的明文2相同                 : True
```

**它之所以成立是一行代数**：`c = m XOR keystream`，于是 `c1 XOR c2 = m1 XOR m2` —— 密钥流抵消了。攻击者从未知道密钥；他们知道的是**两条消息之间的关系**，而一旦其中一条可猜 —— 固定的格式、一个已知字段、一个他自己提供过的值 —— 另一条就掉出来了。

**所以 nonce 重用不是"削弱"，它把两条消息绑在了一起**，那是一种不同、而且更糟的失效模式，比"密钥有点短"要糟。重用的来源值得列出来，因为它们都很平常：

| 来源 | 为什么发生 |
|---|---|
| 重启后归零的计数器 | nonce 空间跟着进程重新开始 |
| 多个实例共用一个计数器 | 每个都以为自己拿到的是下一个值 |
| 太短的随机 nonce | 高量下碰撞变得可能 |
| 由可重复的东西推导出的 nonce | 低精度时间戳、用户 id |

**而同一个失效也适用于认证模式。** 一个被重复使用的 nonce 配 GCM，失去的不只是机密性 —— 认证密钥可以被还原，那会把一次机密性失败变成伪造能力。这就是为什么 nonce 纪律不是一个"选哪个密码"的细节。

### 实测：从一个布尔值到一整个块

三者里最有教益的一个，因为那个 oracle 泄漏的东西看起来完全不像信息。实测，对着一个只回答一个问题 —— **填充是否有效** —— 的服务器：

```
逐字节恢复第一个块：
  第 15 字节: 't'   第 14: 'y'   第 13: 'b'   第 12: '6'
  第 11 字节: '1'   第 10: '-'   第  9: 'g'   第  8: 's'
  第  7 字节: 'm'   第  6: '-'   第  5: 't'   第  4: 'e'
  第  3 字节: 'r'   第  2: 'c'   第  1: 'e'   第  0: 's'

解出的块: b'secret-msg-16byt'
真实的块: b'secret-msg-16byt'
完全相同: True
```

**那个 oracle 没有泄漏密钥、也没有泄漏明文。** 它回答了一个比特，关于解密结果的一个"协议本身要求服务器去检查"的性质。攻击在于那个答案是**可观察的**，而一个被改动过的密文块让攻击者可以逐字节探测中间值：通过改动前一个块、让最后一个字节解密成 `0x01`，填充就"有效"了，而达成它的那个字节反过来揭示了中间字节，明文随之而来。

**那个可观察的差异不必是一条消息。** 下面这些全都是 oracle：

| 可观察的东西 | 是不是 oracle |
|---|---|
| 不同的错误消息 | 经典的那种 |
| 不同的状态码 | 同样的信息，不同的通道 |
| **不同的响应时间** | 填充检查用了一个提前退出的循环 |
| 不同的响应长度 | 空响应体 与 错误页 |
| 连接被关闭还是保持 | |

**任何能区分"填充有效"与"填充无效"的差异就是那个 oracle** —— 所以修法是让解密失败变得无法区分，而不是换一个密码。

### 实测：一个认证模式改变了什么

同样的篡改，对着 AES-GCM —— 密文带着 16 字节的认证标签：

```
密文长度: 33（16 字节明文 + 16 字节标签）
翻转一个比特之后: 解密直接失败，异常是 InvalidTag
```

**只有一种失败，没有"填充坏"与"标签坏"的区分** —— 因为根本没有填充可坏，而完整性检查发生在任何东西被解释之前。这就是伸手去拿 AEAD 的实用理由：

**它把完整性一起管了，所以没有 padding oracle 可看。** 那一类攻击依赖一个"能部分成功"的解密步骤，而 AEAD 没有这样一步。

**它移除了那个拼装步骤。** 另一条路 —— 先加密、再算 MAC —— 有一个顺序问题，而 **MAC-then-encrypt**（旧 TLS 的做法）正是现实中产生 padding oracle 的那个安排。如果非自己拼不可，正确的顺序是 **encrypt-then-MAC**；而根本不拼更好，因为 API 不会把顺序搞错。

### 这一族的其余部分

| 误用 | 代价 |
|---|---|
| 非密码学的随机源 | 可预测的令牌 —— 会话那篇的失效 |
| 用快哈希存口令，或者不加盐 | 大规模离线破解，以及彩虹表 |
| 用 `==` 比较秘密 | 一个计时 oracle：比较在第一个不同的字节处退出 |
| 密钥进源码、跨环境共用 | 上面的一切，再加会话那篇的撤销问题 |
| 任何自己发明的东西 | 上面全部，而且没有第二个人看过 |

常数时间那一行值得注一句，因为它修起来容易、漏起来也容易：**用 `==` 比较一个 MAC 或一个令牌，会通过比较耗时泄漏出"前多少个字节相同"**。库之所以提供常数时间比较，正是因为这个；而用它是一行的改动。

### 检测与缓解

- **grep `ECB`、硬编码或全零的 IV 与 nonce、以及本该用 `secrets` 却用了 `random` 的地方。** 这些都是 API 用法，意味着它们在代码评审里可见、而不只是在利用里可见 —— 这一类罕见地"可评审"，而它几乎不需要一个运行中的系统来发现。
- **对可区分的解密失败告警。** 把"密文被篡改"与"密文有效但内容不对"的状态码、响应体、响应时间放在一起比。三者中任何一项有可测量的差异，就是那个 oracle。
- **并且盯日志里重复出现的 nonce。** 在 nonce 被传输或被记录的地方，一次重复就是整条发现 —— 而一个部署后归零的计数器是经典的来源。
- **缓解上，选一个 AEAD 模式，并且绝不要自己拼。** AES-GCM 或 ChaCha20-Poly1305 用一次调用给出机密性与完整性，并附带明确的 nonce 要求。这是那一个能一次性移除 padding oracle、顺序问题、以及"MAC 盖的是不是正确的字节"问题的决定。
- **给每条消息一个唯一的 nonce，并让计数器持久。** 把计数器跨重启持久化，或者用一个足够长的随机 nonce 让碰撞可以忽略，就解决了两种最常见的重用来源。
- **让解密失败无法区分。** 一个状态码、一条消息、一条代码路径，而在工作量差异可测量的地方，用常数时间比较代替提前退出。
- **对每一个秘密都用库提供的常数时间比较。** MAC、令牌、签名。
- **用为口令设计的哈希。** Argon2 或 bcrypt 配每个口令自己的盐，不要用通用哈希、也不要只做一轮任何东西。
- **随机性取自平台的密码学源。** 如果那条代码路径可以被播种，它就可以被复现。
- **并且问这一篇开头那个问题，因为它是找到这些的那一个。** 不是"这个算法强不强"，而是 **"这个 API 默认做什么、它覆盖了哪些性质、以及一次失败从外面看起来是什么样"** —— 上面那些实测的失效，各自都把这三个问题答错了。

