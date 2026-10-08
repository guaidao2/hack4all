---
id: cryptography-basics
title_en: Cryptography Basics for Security Work
title_zh: 密码学入门
summary_en: Cryptography answers three different questions, and most real-world mistakes come from confusing them. This entry separates confidentiality, integrity and identity, then builds each one up — hashes, symmetric encryption and its modes, public key, signatures, certificates — and ends with the list of ways implementations get them wrong.
summary_zh: 密码学回答的是三个不同的问题，而现实中大多数错误都源于把它们混为一谈。这一篇先把机密性、完整性和身份分开，再逐个搭起来 —— 哈希、对称加密及其模式、公钥、签名、证书 —— 最后收在一份"实现是怎么用错的"清单上。
tags: [beginner, cryptography, hashing, aes, rsa, tls, certificates, pki]
tools: [openssl, hashcat, gpg, ssh-keygen]
attck: [T1552.004, T1553]
platform: [any]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Three questions, not one

Beginners say "it is encrypted" to mean "it is safe". Cryptography actually answers three separate questions, and the tools are different:

| Question | Answered by | Key |
|---|---|---|
| Can anyone else read this? | **Encryption** | A secret key |
| Has this been changed? | **Hashing / MAC** | None, or a shared key |
| Who really sent it? | **Digital signature / certificate** | A key pair |

A message can be encrypted and still forged. A hash can prove integrity and reveal nothing about who sent it. A signature proves authorship and hides nothing. Keeping the three apart is most of what "understanding cryptography" means in practice.

And one thing that is none of the three: **encoding**. Base64 is not encryption — there is no key, and anyone can reverse it. That mistake has its own entry in this guide, and it is the most common one.

### Part 1: hashing

#### What it is

A hash function takes any input and produces a fixed-size output (a **digest**). It has four properties that matter:

1. **Deterministic** — the same input always gives the same output.
2. **One-way** — you cannot compute the input from the output.
3. **Avalanche** — a one-bit change in the input changes about half the output bits.
4. **Collision-resistant** — it is infeasible to find two inputs with the same output.

```bash
echo -n 'hello' | sha256sum
# 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
echo -n 'hellp' | sha256sum
# a completely different value — that is the avalanche effect
```

#### Which ones, and which are broken

| Algorithm | Output | Status |
|---|---|---|
| MD5 | 128 bits | **Broken** — practical collisions since 2004 |
| SHA-1 | 160 bits | **Broken** — collision demonstrated in 2017 |
| SHA-256 / SHA-512 | 256 / 512 bits | Fine for integrity and signatures |
| SHA-3 | variable | The newer standard; fine |
| bcrypt / scrypt / argon2 | variable | **For passwords specifically** |

The distinction that trips people up: SHA-256 is a perfectly good hash and a terrible password hash. It is designed to be **fast**, which is exactly wrong for passwords — an attacker with a GPU tries billions of guesses per second.

#### Why passwords need something else

Password hashing needs two extra properties:

- **A salt** — a random value stored alongside each hash, so identical passwords produce different hashes and precomputed tables ("rainbow tables") stop working. A salt does **not** stop targeted guessing; it stops mass precomputation.
- **Slowness** — a work factor that makes each guess cost milliseconds. bcrypt, scrypt and argon2 are memory-hard and deliberately slow.

```
bcrypt:  $2b$12$...        work factor encoded in the string
argon2:  $argon2id$v=19$...  memory and iterations encoded
sha256:  2cf24dba5fb0...     instant, and therefore wrong for passwords
```

You will meet hashes in security work in three places: password stores (where the algorithm says how much trouble the defenders took), file integrity (where a hash proves a download is unmodified), and signatures (where a hash is what actually gets signed).

#### The length-extension trap

`hash(secret + message)` is **not** a MAC. Given the hash of one message and its length, an attacker can compute the hash of that message plus extra data, without knowing the secret. This is the **length extension attack**, and it is why the correct construction is an HMAC (or a hash function designed to resist it), never a home-made concatenation.

### Part 2: symmetric encryption

One key, used to both encrypt and decrypt. It is fast, and it is what actually protects your data.

#### AES

AES is a **block cipher**: it encrypts fixed 128-bit blocks. Key sizes are 128, 192 or 256 bits. On its own, a block cipher only handles one block, so you need a **mode of operation** to handle real data — and the mode is where almost every real-world mistake happens.

#### Modes, and why ECB is a punchline

| Mode | What it does | Verdict |
|---|---|---|
| **ECB** | Encrypts each block independently | **Never use it.** Identical blocks produce identical ciphertext, so structure leaks — the famous "ECB penguin" |
| **CBC** | Chains blocks, needs an IV | Works, but padding gives rise to padding-oracle attacks, and it is not authenticated |
| **CTR** | Turns the block cipher into a stream using a counter | Good, but also not authenticated |
| **GCM** | CTR plus authentication | **The modern default.** Encrypt and authenticate in one operation |
| **ChaCha20-Poly1305** | Stream cipher plus authentication | The modern alternative, preferred where AES hardware is absent |

**The two rules a beginner should take from this table:**

1. **Use an authenticated mode (AEAD) — GCM or ChaCha20-Poly1305.** Without authentication, ciphertext can be modified and the receiver will not know.
2. **Never reuse an IV/nonce with the same key.** In CTR and GCM, reusing a nonce is catastrophic: it leaks the plaintext. The nonce does not need to be secret, but it must be unique per message under that key.

```bash
# a correct choice, from the command line
openssl enc -aes-256-gcm -pbkdf2 -in file -out file.enc
openssl enc -d -aes-256-gcm -pbkdf2 -in file.enc -out file
```

#### Where the key comes from

Symmetric encryption is fast and strong, but it has a distribution problem: both sides need the same key, and they need to agree on it without an eavesdropper learning it. That problem is what public key cryptography solves.

### Part 3: public key cryptography

Two keys, mathematically related: what one encrypts, only the other decrypts.

| Operation | Key used | Proves |
|---|---|---|
| Encrypt | Recipient's **public** key | Only the recipient can read it |
| Decrypt | Recipient's **private** key | — |
| Sign | Sender's **private** key | Only the key holder could have produced it |
| Verify | Sender's **public** key | Anyone can check |

The security rests on problems that are hard in one direction and easy in the other:

- **RSA** — multiplying two large primes is easy; factoring the product is hard.
- **Elliptic curve (ECDSA, Ed25519)** — the discrete logarithm problem on a curve, which gives the same strength with much shorter keys.

| Strength | RSA key | ECC key |
|---|---|---|
| ~112 bits | 2048 bits | 224 bits |
| ~128 bits | 3072 bits | 256 bits |
| ~256 bits | 15360 bits | 512 bits |

That is why modern systems use Ed25519 for SSH keys and ECDSA for certificates.

#### Diffie-Hellman, the surprising part

Public key cryptography also makes possible something that seems impossible: two parties agreeing on a secret key over a channel somebody is listening to.

The intuition, without the maths: each side mixes its own secret with the other's public value in a way that produces the same result on both sides, but which an observer cannot reproduce without one of the secrets. It is called **key exchange**, and the ephemeral variant (ECDHE) is what gives TLS **forward secrecy** — each session gets a fresh key, so stealing the long-term private key later does not decrypt recorded past sessions.

#### Hybrid encryption, which is what actually happens

Public key operations are slow. Real protocols use them only to agree on a symmetric key, then use that symmetric key for the data. That is TLS: the handshake uses ECDHE and certificates, and the actual traffic is AES-GCM. When someone says "the connection is encrypted with AES", the asymmetric part already finished.

### Part 4: MACs and signatures

Two ways to prove a message was not tampered with, with an important difference.

| | HMAC | Digital signature |
|---|---|---|
| Key | Shared secret (symmetric) | Private key (asymmetric) |
| Speed | Fast | Slower |
| Who can verify | Anyone holding the key | Anyone with the public key |
| Who can forge | Anyone holding the key | Only the private key holder |
| Provides non-repudiation | No — the verifier could have made it | Yes |

Use an HMAC when both sides are trusted and you want speed. Use a signature when the verifier must be able to prove to a third party who sent it.

### Part 5: certificates and PKI

Public key cryptography has a bootstrapping problem: if I receive a public key claiming to be `example.com`, how do I know it is?

**A certificate** solves it. It is a signed statement:

```
Subject:      CN=example.com
Subject Alt Names: example.com, www.example.com
Public Key:   <the public key>
Issuer:       CN=Some Intermediate CA
Validity:     not before / not after
Signature:    <the CA's signature over all of the above>
```

The chain works upward: the leaf certificate is signed by an intermediate CA, which is signed by a root CA, and the root is one your browser or OS already trusts (it ships with a few hundred of them).

**When your browser validates a certificate, it checks:**

| Check | Failure means |
|---|---|
| The signature chain leads to a trusted root | Unknown or self-signed issuer |
| The certificate is within its validity dates | Expired, or not yet valid |
| The requested name appears in Subject/SAN | Wrong site |
| The certificate is not revoked (CRL or OCSP) | Stolen or misissued |
| Key usage permits this purpose | Wrong kind of certificate |

**Self-signed certificates** are the common case you will meet: they are cryptographically valid but nobody vouches for them. That is why the browser warns — not because the encryption is weak, but because the identity is unverified. It is also why an intercepting proxy needs its own CA installed: it is doing exactly the "man in the middle" that the check exists to prevent, so you have to tell your browser to trust it.

**Certificate Transparency** is worth knowing about: certificates are logged publicly, so anyone can detect a certificate issued for their domain by someone who should not be able to. It is how misissuance is discovered in practice.

### Part 6: how it all fits together in TLS

A modern TLS 1.3 handshake does four things:

1. **Key exchange** — ECDHE, producing a shared secret neither side transmitted.
2. **Authentication** — the server proves its identity with a certificate chain.
3. **Key derivation** — both sides derive session keys from the shared secret.
4. **Encrypted traffic** — AES-GCM or ChaCha20-Poly1305, with fresh keys per session.

Version history in one line: TLS 1.0 and 1.1 are deprecated, 1.2 is acceptable with a good cipher configuration, and 1.3 removes the legacy options entirely. If you are configuring a server, 1.2 and 1.3 only.

```bash
openssl s_client -connect example.com:443 -servername example.com
# then, at the prompt: the chain, the cipher, and the negotiated parameters
echo | openssl s_client -connect example.com:443 2>/dev/null | openssl x509 -noout -dates -subject -issuer
```

### Part 7: passwords and keys

#### Password strength, honestly

Length beats complexity. Twenty lowercase characters have more entropy than eight characters with a symbol, and are easier to remember.

Attack types you should be able to name, because they decide what "strong" means:

| Attack | Method | Defeated by |
|---|---|---|
| Dictionary | Try common passwords | Not being a common password |
| Rule-based | Dictionary plus mutations (`P@ssw0rd`) | Longer passwords, less predictable patterns |
| Brute force | Try everything | Length |
| Mask | Try a known pattern (`?u?l?l?l?l?d?d`) | Unpredictability |
| Credential stuffing | Reuse leaked pairs | Not reusing passwords |

```bash
hashcat -m 0 hash.txt wordlist.txt -r best64.rule      # MD5
hashcat -m 1000 ntlm.txt wordlist.txt                  # Windows NT hash
hashcat -m 3200 bcrypt.txt wordlist.txt                # bcrypt (slow, on purpose)
```

#### Keys

- **Never hardcode a key** in source, in an app, or in a container image. Anything shipped to a client is public.
- **Never commit one** to a repository. A key that was ever committed is compromised, even if deleted later — it is in the history.
- **Rotate them**, and prefer short-lived credentials over long-lived ones.
- **Use a secrets manager**, scoped per environment, with audit logging.

### Part 8: the ways implementations get it wrong

Most real-world cryptographic failures are on this list, and none of them is a broken algorithm:

1. **Inventing your own algorithm or protocol.** Do not. Use a library, use a standard construction.
2. **Using MD5 or SHA-1** for anything security-relevant.
3. **Using a fast hash for passwords** — SHA-256 is not a password hash.
4. **ECB mode**, which leaks structure.
5. **No authentication** — CBC or CTR without a MAC, so ciphertext can be modified undetected.
6. **Reusing an IV/nonce** with the same key, which in CTR/GCM leaks plaintext.
7. **A predictable "random" value** — `rand()`, `Math.random()`, a timestamp, or a counter used where a CSPRNG is required.
8. **Teaching that encoding is encryption** — Base64 is not a secret.
9. **Comparing secrets with `==`** instead of a constant-time comparison, leaking information through timing.
10. **Trusting a certificate without validating it** — the classic "disable certificate verification" line in a client.
11. **Home-made MACs** (`hash(secret + message)`), which fall to length extension.

### Detection and mitigation

- **Encryption changes what detection can see, so monitor metadata.** TLS hides content, not behaviour: the destination, the certificate, the SNI name, the volume, the timing and the client fingerprint (JA3/JA4) are all still visible, and all still useful.
- **Watch certificates.** Self-signed or internal CAs appearing on outbound traffic, recently registered domains with valid certificates, and short-lived certificates are all signals worth a rule. Certificate Transparency monitoring tells you when somebody obtains a certificate for your domain.
- **Scan for keys in code and images.** A secret scanner in CI, and a scan of container layers, catch the most common real leak. Rotate on discovery, because detection is not remediation.
- **Audit how passwords are stored.** The hash format tells you: `$argon2id$` or `$2b$` means somebody thought about it; a bare 32-character hex string means MD5 and a problem.
- **Enforce modern defaults.** TLS 1.2+ with AEAD ciphers, HSTS, and no legacy protocol fallback. Most downgrade attacks need a legacy option to exist.
- **Protect the private keys.** File permissions (0600), hardware storage or a KMS where it matters, and rotation. A leaked private key defeats every other control you have.
- **Keep the library patched.** Heartbleed, the Debian OpenSSL RNG bug, and countless padding-oracle CVEs were implementation failures in correctly designed cryptography. The maths being sound does not make the code sound.

<!-- lang:zh -->
### 三个问题，不是一个

新手说"它加密了"来表达"它安全了"。但密码学其实回答的是三个彼此独立的问题，而用的工具各不相同：

| 问题 | 由什么回答 | 密钥 |
|---|---|---|
| 别人能读到内容吗？ | **加密** | 一把机密密钥 |
| 内容被改过吗？ | **哈希 / MAC** | 没有，或一把共享密钥 |
| 到底是谁发的？ | **数字签名 / 证书** | 一对密钥 |

一条消息可以被加密，同时仍能被伪造。一个哈希可以证明完整性，却对"谁发的"一言不发。一个签名能证明作者身份，却什么都不隐藏。**在实践里，"懂密码学"的大部分含义就是能把这三种分开。**

还有一样三者都不是的东西：**编码**。Base64 不是加密 —— 它没有密钥，谁都能还原。这个错误在本指南里有专篇，而它是最常见的那个。

### 第一部分：哈希

#### 它是什么

哈希函数接受任意输入，产生**固定长度**的输出（**摘要**）。有四个性质要紧：

1. **确定性** —— 同样的输入永远给同样的输出。
2. **单向** —— 从输出算不回输入。
3. **雪崩效应** —— 输入改一个比特，输出大约一半的比特会变。
4. **抗碰撞** —— 找到两个输入产生同一输出，在计算上不可行。

```bash
echo -n 'hello' | sha256sum
# 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
echo -n 'hellp' | sha256sum
# 完全不同的值 —— 这就是雪崩效应
```

#### 该用哪个，哪些已经被破

| 算法 | 输出 | 状态 |
|---|---|---|
| MD5 | 128 位 | **已破** —— 2004 年起就有实用碰撞 |
| SHA-1 | 160 位 | **已破** —— 2017 年演示了碰撞 |
| SHA-256 / SHA-512 | 256 / 512 位 | 用于完整性和签名是好的 |
| SHA-3 | 可变 | 更新的标准，同样可用 |
| bcrypt / scrypt / argon2 | 可变 | **专用于口令** |

最容易绊倒人的区分是：SHA-256 是一个完全合格的哈希，却是一个糟糕的口令哈希。它的设计目标是**快**，而这恰恰是口令最不想要的 —— 有 GPU 的攻击者每秒能试几十亿次。

#### 口令为什么需要另一些东西

口令哈希需要两个额外的性质：

- **盐（salt）** —— 一个随机值，和每个哈希一起存，于是相同的口令会产生不同的哈希，预先算好的表（"彩虹表"）失效。盐**不能**阻止针对性猜测，它阻止的是大规模预计算。
- **慢** —— 一个工作因子，让每次猜测耗时以毫秒计。bcrypt、scrypt、argon2 是内存硬且刻意慢的。

```
bcrypt:  $2b$12$...        工作因子写在字符串里
argon2:  $argon2id$v=19$...  内存开销与迭代次数写在里面
sha256:  2cf24dba5fb0...     瞬时完成，因此对口令来说是错的
```

你会在安全工作中三个地方遇到哈希：口令库（算法本身就说明了防守方花了多少心思）、文件完整性（哈希证明下载没被改过）、以及数字签名（真正被签的其实是哈希）。

#### 长度扩展的陷阱

`hash(secret + message)` **不是** MAC。已知某条消息的哈希和它的长度，攻击者就能算出"那条消息 + 附加数据"的哈希，完全不需要知道 secret。这就是**长度扩展攻击**，也是为什么正确构造是 HMAC（或专门设计来抵抗它的哈希），而绝不是自己拼一下。

### 第二部分：对称加密

一把密钥，既用来加密也用来解密。它快，而且真正保护你数据的就是它。

#### AES

AES 是**分组密码**：它加密固定的 128 位分组。密钥长度有 128、192、256 位。单靠分组密码只能处理一个分组，所以要处理真实数据你需要一个**工作模式** —— 而现实中几乎所有的错误都出在模式上。

#### 各种模式，以及 ECB 为什么是个笑话

| 模式 | 它做什么 | 结论 |
|---|---|---|
| **ECB** | 每个分组独立加密 | **绝不要用。** 相同的明文分组产生相同的密文，结构直接泄漏 —— 著名的"ECB 企鹅" |
| **CBC** | 分组串联，需要一个 IV | 能用，但填充带来了填充预言机攻击，而且它不提供认证 |
| **CTR** | 把分组密码变成用计数器的流密码 | 可以，但同样不认证 |
| **GCM** | CTR 加认证 | **现代默认。** 一次操作同时完成加密与认证 |
| **ChaCha20-Poly1305** | 流密码加认证 | 现代替代方案，在没有 AES 硬件加速时更优 |

**新手该从这张表里带走两条规则：**

1. **用带认证的模式（AEAD）—— GCM 或 ChaCha20-Poly1305。** 没有认证，密文可以被改，而接收方不会知道。
2. **同一把密钥下，绝不重复使用 IV/nonce。** 在 CTR 和 GCM 里，nonce 重用是灾难性的：它会泄漏明文。nonce 不需要保密，但在该密钥下必须每条消息都不同。

```bash
# 命令行里的正确选择
openssl enc -aes-256-gcm -pbkdf2 -in file -out file.enc
openssl enc -d -aes-256-gcm -pbkdf2 -in file.enc -out file
```

#### 密钥从哪来

对称加密又快又强，但它有个分发问题：双方需要同一把密钥，而且要在有窃听者的信道上商定它。这个问题，正是公钥密码学要解决的。

### 第三部分：公钥密码学

两把数学上相关的密钥：一把加密的，只有另一把能解。

| 操作 | 用哪把钥匙 | 证明了什么 |
|---|---|---|
| 加密 | 接收方的**公钥** | 只有接收方能读 |
| 解密 | 接收方的**私钥** | —— |
| 签名 | 发送方的**私钥** | 只有持有私钥的人能产生 |
| 验签 | 发送方的**公钥** | 任何人都能校验 |

它的安全性建立在"一个方向容易、另一个方向困难"的数学问题上：

- **RSA** —— 两个大素数相乘很容易，把乘积分解回去很难。
- **椭圆曲线（ECDSA、Ed25519）** —— 曲线上的离散对数问题，能在密钥短得多的情况下给出同等强度。

| 强度 | RSA 密钥 | ECC 密钥 |
|---|---|---|
| 约 112 位 | 2048 位 | 224 位 |
| 约 128 位 | 3072 位 | 256 位 |
| 约 256 位 | 15360 位 | 512 位 |

这就是现代系统用 Ed25519 做 SSH 密钥、用 ECDSA 做证书的原因。

#### Diffie-Hellman，最反直觉的那部分

公钥密码学还让一件看起来不可能的事成为可能：双方在**有人监听**的信道上，协商出一把共享密钥。

不谈数学的直觉是：双方各自把自己的一份秘密与对方的公开值混合，混合方式让两边得到相同的结果 —— 而旁观者缺了其中任何一份秘密，就算不出来。这叫做**密钥交换**，而它的临时版本（ECDHE）给了 TLS **前向保密**：每次会话都有一把全新的密钥，所以事后偷到长期私钥，也解不开当初录下来的会话。

#### 实际发生的是混合加密

公钥运算很慢。真实协议只用它来商定一把对称密钥，然后用那把对称密钥处理数据。TLS 就是这样：握手用 ECDHE 和证书，实际流量用 AES-GCM。当有人说"这条连接是用 AES 加密的"，非对称的那部分早就结束了。

### 第四部分：MAC 与签名

两种证明"消息没被改"的方式，但有一个重要区别。

| | HMAC | 数字签名 |
|---|---|---|
| 密钥 | 共享密钥（对称） | 私钥（非对称） |
| 速度 | 快 | 较慢 |
| 谁能验证 | 持有密钥的人 | 任何拿到公钥的人 |
| 谁能伪造 | 持有密钥的人 | 只有私钥持有者 |
| 能否不可否认 | 不能 —— 验证方自己也能造 | 能 |

双方互信、又想要速度时用 HMAC；当验证方需要能向第三方证明"这是谁发的"时，用签名。

### 第五部分：证书与 PKI

公钥密码学有一个自举问题：如果我收到一个自称是 `example.com` 的公钥，我怎么知道它是？

**证书**解决这个问题。它是一份被签名的声明：

```
Subject:      CN=example.com
Subject Alt Names: example.com, www.example.com
Public Key:   <公钥>
Issuer:       CN=Some Intermediate CA
Validity:     not before / not after
Signature:    <CA 对以上全部内容的签名>
```

信任链是往上走的：叶证书由中级 CA 签名，中级 CA 由根 CA 签名，而根 CA 是你的浏览器或操作系统**已经信任**的那些（它内置了几百个）。

**浏览器校验证书时，检查这些：**

| 检查 | 失败意味着 |
|---|---|
| 签名链能走到一个受信任的根 | 未知或自签名的颁发者 |
| 证书在有效期内 | 过期，或者还没生效 |
| 请求的名字出现在 Subject/SAN 里 | 认错站了 |
| 证书未被吊销（CRL 或 OCSP） | 被盗或被错误签发 |
| 密钥用途允许这个目的 | 证书类型不对 |

**自签名证书**是你最常见的那个情况：密码学上它有效，只是没有人为它背书。这就是浏览器报警的原因 —— **不是因为加密弱，而是因为身份没被验证**。这也是为什么拦截代理需要装一个自己的 CA：它做的是检查机制本意要防的那件"中间人"，所以你必须告诉浏览器信任它。

**证书透明（Certificate Transparency）** 值得知道：证书会被公开记录，于是任何人都能发现"有人为我的域名签发了证书，而他不该签得出来"。现实中的错误签发就是这么被发现的。

### 第六部分：TLS 把它们串在一起

一个现代 TLS 1.3 握手做四件事：

1. **密钥交换** —— ECDHE，产生一份双方都没有传输过的共享秘密。
2. **认证** —— 服务端用证书链证明自己的身份。
3. **密钥派生** —— 双方从共享秘密派生出会话密钥。
4. **加密流量** —— AES-GCM 或 ChaCha20-Poly1305，每次会话都是全新密钥。

版本历史一句话：TLS 1.0 和 1.1 已弃用，1.2 在密码套件配置良好的前提下可以接受，1.3 把遗留选项整个删掉了。如果你在配服务器，只开 1.2 和 1.3。

```bash
openssl s_client -connect example.com:443 -servername example.com
# 进入交互后：看证书链、密码套件、协商出的参数
echo | openssl s_client -connect example.com:443 2>/dev/null | openssl x509 -noout -dates -subject -issuer
```

### 第七部分：口令与密钥

#### 口令强度，说句实话

**长度胜过复杂度。** 二十个小写字母的熵，比八个带符号的字符更高，而且更好记。

你该能叫出名字的攻击类型，因为它们决定了"强"是什么意思：

| 攻击 | 方法 | 被什么克制 |
|---|---|---|
| 字典 | 试常见口令 | 不要用常见的 |
| 规则 | 字典加变形（`P@ssw0rd`） | 更长、更少可预测的模式 |
| 暴力 | 全试 | 长度 |
| 掩码 | 按已知模式试（`?u?l?l?l?l?d?d`） | 不可预测性 |
| 撞库 | 复用泄漏的账号口令对 | 不复用口令 |

```bash
hashcat -m 0 hash.txt wordlist.txt -r best64.rule      # MD5
hashcat -m 1000 ntlm.txt wordlist.txt                  # Windows NT 哈希
hashcat -m 3200 bcrypt.txt wordlist.txt                # bcrypt（故意很慢）
```

#### 密钥

- **绝不要把密钥硬编码**在源码、应用或容器镜像里。发给客户端的一切都是公开的。
- **绝不要提交进仓库。** 只要提交过一次，那个密钥就已泄漏，删掉也没用 —— 它在历史里。
- **要轮换**，并且优先用短期凭据而不是长期凭据。
- **用密钥管理服务**，按环境分别授权，并记录审计日志。

### 第八部分：实现是怎么用错的

现实中的密码学失败几乎都在这份清单上，而且没有一条是"算法被破了"：

1. **自己发明算法或协议。** 不要。用库，用标准构造。
2. **在任何和安全相关的场合使用 MD5 或 SHA-1。**
3. **用快哈希存口令** —— SHA-256 不是口令哈希。
4. **用 ECB 模式**，它会泄漏结构。
5. **不做认证** —— CBC 或 CTR 不配 MAC，密文被改也发现不了。
6. **同一密钥下重复使用 IV/nonce**，在 CTR/GCM 里会泄漏明文。
7. **可预测的"随机"值** —— 该用 CSPRNG 的地方用了 `rand()`、`Math.random()`、时间戳或计数器。
8. **教别人"编码就是加密"** —— Base64 不是秘密。
9. **用 `==` 比较机密**，而不是恒定时间比较，从而通过时间差泄漏信息。
10. **信任证书却不校验它** —— 客户端里那句经典的"禁用证书校验"。
11. **自制 MAC**（`hash(secret + message)`），会被长度扩展打穿。

### 检测与缓解

- **加密改变了检测能看见的东西，所以去监控元数据。** TLS 隐藏的是内容，不是行为：目标地址、证书、SNI 名称、流量大小、时序，以及客户端指纹（JA3/JA4）都仍然可见，也仍然有用。
- **盯住证书。** 出站流量上出现自签名或内部 CA、新注册域名配上有效证书、超短有效期的证书 —— 都值得一条规则。证书透明监控能告诉你"有人为你的域名签了证书"。
- **扫描代码与镜像里的密钥。** CI 里的密钥扫描器，加上对容器层的扫描，能抓住最常见的真实泄漏。发现即轮换 —— 检测不是修复。
- **审计口令是怎么存的。** 哈希的格式会告诉你：`$argon2id$` 或 `$2b$` 说明有人想过这件事；一串光秃秃的 32 位十六进制说明是 MD5，也说明有问题。
- **强制现代默认值。** TLS 1.2+ 配 AEAD 密码套件、开启 HSTS、不留遗留协议回退。大多数降级攻击都需要一个"遗留选项"存在才能成立。
- **保护私钥。** 文件权限（0600）、要紧的地方用硬件或 KMS 保存、并定期轮换。私钥泄漏会击穿你其余所有的控制。
- **保持库的更新。** Heartbleed、Debian OpenSSL 随机数缺陷，以及无数填充预言机 CVE，都是"设计正确的密码学"在实现上出的错。数学没问题，不代表代码没问题。
