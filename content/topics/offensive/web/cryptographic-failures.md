---
id: cryptographic-failures
title_en: Cryptographic Failures
title_zh: 加密机制失效
summary_en: The maths is almost never broken. What fails is everything around it — secrets stored with a fast hash, tokens built from a timestamp, encryption done in the browser with the key shipped alongside it, and data that was never meant to leave the server kept in plaintext.
summary_zh: 数学几乎从没被攻破。失效的是它周围的一切 —— 用快哈希存的口令、用时间戳拼出来的 token、在浏览器里做加密却把密钥一起发下去、以及本该留在服务端却以明文保存的数据。
tags: [web, crypto, owasp-a02, password-storage, tls, tokens, bugbounty]
tools: [hashcat, hashid, jwt_tool, testssl.sh, sslscan]
attck: [T1552, T1600]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### What this category actually is

OWASP's "cryptographic failures" is not about breaking AES. It is about the six or seven ways implementations get cryptography wrong around the primitives:

1. Data in transit without protection, or with protection that validates nothing.
2. Passwords stored with an algorithm designed to be fast.
3. Tokens and identifiers generated with something predictable.
4. Sensitive data encrypted somewhere the key is also available.
5. Keys and secrets committed, baked into images, or shipped to clients.
6. Modes and constructions used where their properties are exactly wrong.

Each of those is testable from outside, which is what this entry is for.

### 1. Transport

Send credentials over HTTP and they are on the wire, in the proxy log, and in whatever the client's network does with them. Two things worth checking beyond the obvious:

- **Mixed content.** A login page served over HTTPS that posts to an HTTP endpoint loses the protection at the last step.
- **Internal HTTP.** Service-to-service traffic on a flat network is routinely plaintext, which is what makes lateral movement trivial after a single foothold.

```bash
testssl.sh https://target.example          # protocol versions, ciphers, HSTS, certificate chain
```

### 2. Password storage, and how fast you can tell

The question is not "is it hashed" but "which hash, and can I afford to attack it".

| Stored as | Verdict | Cost to an attacker |
|---|---|---|
| Plaintext, or reversible encryption | Broken | Read it |
| MD5 / SHA1 / SHA256, unsalted | Broken | Billions per second on a modern GPU |
| Same, salted | Weak | Not rainbow tables, but still fast to brute force |
| bcrypt / scrypt / argon2 | Correct | Deliberately slow; per-guess cost measured in milliseconds |

Identify what you are looking at before doing anything expensive:

```bash
hashid '5f4dcc3b5aa765d61d8327deb882cf99'
# then crack offline with the matching mode
hashcat -m 0     hash.txt wordlist.txt -r best64.rule   # MD5
hashcat -m 100   hash.txt wordlist.txt                  # SHA1
hashcat -m 1400  hash.txt wordlist.txt                  # SHA256
hashcat -m 3200  hash.txt wordlist.txt                  # bcrypt
```

Salting stops precomputation, not targeted guessing: if the account is `admin` and the hash leaked, a dictionary plus rules still recovers `Summer2026!`. The fix is the algorithm, not the salt.

### 3. Predictable randomness

This is the one that turns into account takeover most often. Test any identifier the application hands out: reset tokens, session ids, invite codes, coupon codes, order references, file names.

- **Sequential or timestamp-based**: collect a dozen samples and sort them. If they increase with time, the search space is minutes.
- **Framework PRNGs used for security**: PHP's `mt_rand` is predictable after observing a few outputs, Java's `Random` is a linear congruential generator, and `Math.random()` in JavaScript is not a security source in any runtime.
- **UUID v1**: it contains a timestamp and the MAC address, so it is guessable; v4 from a good source is not.
- **Truncated hashes and short tokens**: check the length. Anything under 128 bits of real entropy is worth attacking, and enumerable tokens are much shorter than that in practice.

```python
# the shape of the test: collect samples, look for a pattern
samples = [request_reset_token(user) for user in range(20)]
# sequential? time-correlated? same prefix? derived from the username?
```

### 4. Encryption performed on the client

If the browser encrypts something, the browser has the key, and the key is in the JavaScript. This appears as:

- A login form that "encrypts the password with RSA" before submitting.
- API parameters passed as base64 of an AES ciphertext, with the key in the bundle.
- Client-side "end-to-end" encryption for chat or notes that the server can nonetheless read.

The test is straightforward: read the JavaScript, find the key, and then check whether the server accepts plaintext anyway. Encrypting on the client changes the representation of the data on the wire; it does not protect it from anyone who can read the client.

### 5. ECB mode, and why repeated blocks give it away

ECB encrypts each 16-byte block independently, so identical plaintext blocks produce identical ciphertext blocks. If you can influence part of the plaintext and see the ciphertext, you can detect it immediately: send a value made of repeating blocks and look for repeating blocks in the output.

The classic demonstration is an image with large flat areas: ECB preserves its structure. In an application, the same property lets you infer plaintext, reorder blocks, and sometimes splice one user's data into another's.

The fix is an authenticated mode with a unique nonce (AES-GCM, ChaCha20-Poly1305), not "a different padding".

### 6. Keys and secrets

- **Committed to a repository**: `gitleaks`, `trufflehog`, or just `git log -p` on a small project. A secret that was ever committed is compromised, even if it was deleted later.
- **Baked into images and mobile apps**: an APK or a container layer contains the key; `strings` finds more than people expect.
- **Reused across environments**: the same key in staging and production means a staging leak is a production leak.
- **Hardcoded fallbacks**: an "if no key configured, use this default" branch is a backdoor with good intentions.

### 7. CBC bit flipping and padding oracles

Two properties of CBC mode are worth remembering even without implementing them by hand:

- **Bit flipping**: changing a bit in the previous ciphertext block flips the corresponding bit in the *next* plaintext block, predictably. Where the first block is the IV and the application does not authenticate it, you control part of the decrypted plaintext.
- **Padding oracles**: if the application reveals whether padding was valid, you can decrypt data without the key. The RSA entry covers the same idea in a different construction.

Both are defeated by authenticated encryption (GCM, Poly1305) and by never telling an attacker *why* decryption failed.

### Detection

- **Weak hashes in a database export** — recognisable by length and prefix; a monitoring rule on the schema is more reliable than an audit.
- **Certificate and TLS configuration drift**: expired certificates, TLS 1.0 still enabled, missing HSTS.
- **Secrets in source control**: alert on commits containing high-entropy strings and known key prefixes (`AKIA`, `ghp_`, `sk_live_`), and rotate on discovery.
- **Tokens that arrive too fast to be random** — measure the entropy of issued tokens rather than trusting the generator's documentation.
- **HTTP endpoints carrying credentials**: find them in proxy logs rather than by asking.

### Mitigation

- **Use vetted libraries and modern defaults.** AES-GCM or ChaCha20-Poly1305, TLS 1.2+ with a sane cipher list, bcrypt/scrypt/argon2 for passwords. Do not implement primitives.
- **Store passwords with a memory-hard algorithm** and tune the cost so a single verification takes tens of milliseconds.
- **Generate every security token with a CSPRNG**, at least 128 bits, and make reset tokens single-use with a short expiry.
- **Never do cryptography in the client and call it protection.** If the server must not see the data, the key has to be somewhere the server is not.
- **Keep keys out of code**, in a secrets manager with rotation, scoped per environment and per purpose.
- **Authenticate ciphertext**, and make all decryption failures indistinguishable.
- **Encrypt internal traffic too**, and terminate TLS where you can inspect it rather than skipping it.
- **Alert on secrets in commits** and treat detection as an incident, not a lint warning.

<!-- lang:zh -->
### 这一类到底是什么

OWASP 说的"加密机制失效"不是指 AES 被攻破，而是指实现在原语周围犯的那六七种错：

1. 传输中的数据没有保护，或者保护了却什么都不校验。
2. 口令用"设计上就快"的算法存储。
3. token 和标识符用可预测的东西生成。
4. 敏感数据被加密了，但密钥就在同一个地方。
5. 密钥与机密被提交进仓库、打进镜像，或者下发到客户端。
6. 使用了属性恰好相反的加密模式与构造。

上面每一条都能从外部测出来，这一篇就是讲怎么测。

### 一 —— 传输

凭据走 HTTP，就等于出现在线路上、代理日志里，以及客户端网络对它做的一切处理中。除了显而易见的部分，还有两点值得查：

- **混合内容。** 登录页走 HTTPS，却把表单提交到 HTTP 端点 —— 保护在最后一步丢掉了。
- **内网 HTTP。** 扁平网络里服务之间的流量常年是明文，这正是"一个落脚点之后横向移动轻而易举"的原因。

```bash
testssl.sh https://target.example          # 协议版本、套件、HSTS、证书链
```

### 二 —— 口令存储，以及一眼能看出多少

问题不是"有没有哈希"，而是"用的哪种哈希，我攻得起吗"。

| 存储形式 | 结论 | 攻击者成本 |
|---|---|---|
| 明文，或可逆加密 | 已破 | 直接读 |
| MD5 / SHA1 / SHA256 且无盐 | 已破 | 现代 GPU 每秒几十亿次 |
| 同上但加了盐 | 弱 | 挡不住彩虹表，但仍然能快速爆破 |
| bcrypt / scrypt / argon2 | 正确 | 刻意很慢，每次尝试以毫秒计 |

在花大代价之前，先认出你面对的是什么：

```bash
hashid '5f4dcc3b5aa765d61d8327deb882cf99'
# 然后按匹配的模式离线破解
hashcat -m 0     hash.txt wordlist.txt -r best64.rule   # MD5
hashcat -m 100   hash.txt wordlist.txt                  # SHA1
hashcat -m 1400  hash.txt wordlist.txt                  # SHA256
hashcat -m 3200  hash.txt wordlist.txt                  # bcrypt
```

加盐挡的是预计算，不是定向猜测：如果账号叫 `admin` 而哈希泄漏了，字典加规则照样能还原出 `Summer2026!`。修复靠算法，不靠盐。

### 三 —— 可预测的随机数

这一条最常演变成账号接管。凡是应用发给你的标识符都值得测：重置 token、会话 id、邀请码、优惠券码、订单号、文件名。

- **自增或基于时间戳**：收集十几个样本排个序。如果随时间递增，搜索空间就是"分钟"。
- **把框架的 PRNG 用于安全用途**：PHP 的 `mt_rand` 在观察到几次输出后即可预测，Java 的 `Random` 是线性同余发生器，JavaScript 的 `Math.random()` 在任何运行时里都不是安全随机源。
- **UUID v1**：内含时间戳与 MAC 地址，因此可猜；来自良好随机源的 v4 则不可猜。
- **截断的哈希与短 token**：先看长度。真实熵不到 128 位的都值得攻，而实际中可枚举的 token 往往远短于此。

```python
# 测试的形状：采样，找规律
samples = [request_reset_token(user) for user in range(20)]
# 是否自增？是否与时间相关？前缀相同？是否由用户名派生？
```

### 四 —— 在客户端做的加密

如果浏览器负责加密，那浏览器就握有密钥，而密钥就在 JavaScript 里。常见形态：

- 登录表单在提交前"用 RSA 加密口令"。
- 接口参数是 AES 密文的 base64，密钥在前端 bundle 里。
- 聊天或笔记的"端到端加密"，但服务端依然能读。

测法很直接：读 JavaScript、找到密钥，然后看服务端是否照样接受明文。客户端加密改变的是数据在链路上的表现形式，它不能防住任何能读到客户端的人。

### 五 —— ECB 模式，以及重复块为什么会露馅

ECB 把每个 16 字节分组独立加密，于是相同的明文块产生相同的密文块。只要你能影响部分明文并看到密文，就能立刻判定：送一个由重复块组成的值，看输出里是否出现重复块。

最经典的演示是面积大片纯色的图片 —— ECB 会把它的结构原样保留。在应用里，同样的性质让你可以推断明文、重排分组，有时还能把某个用户的数据拼接到另一个用户的结果里。

修复要用带唯一 nonce 的认证加密模式（AES-GCM、ChaCha20-Poly1305），而不是"换一种填充"。

### 六 —— 密钥与机密

- **提交进了仓库**：`gitleaks`、`trufflehog`，或者小项目直接 `git log -p`。只要提交过一次，那个机密就已泄漏，删掉也没用。
- **打进镜像和移动应用**：APK 或容器层里就带着密钥，`strings` 能翻出的东西比人们预期多得多。
- **跨环境复用**：staging 和生产用同一把密钥，意味着 staging 的泄漏就是生产的泄漏。
- **硬编码兜底值**：`if 没配密钥就用这个默认值` 这种分支，是一个用意良好的后门。

### 七 —— CBC 比特翻转与填充预言机

即使不手写实现，也有两个 CBC 的性质值得记住：

- **比特翻转**：改动前一密文分组的某一位，会**可预测地**翻转下一个明文分组的对应位。当第一个分组是 IV 且应用没有对它做认证时，你就控制了部分解密结果。
- **填充预言机**：如果应用透露"填充是否合法"，你就能在没有密钥的情况下解密数据。RSA 那篇里讲的是同一思路在另一种构造上的体现。

这两者都靠认证加密（GCM、Poly1305）以及"绝不告诉攻击者解密为什么失败"来防。

### 检测

- **数据库导出里的弱哈希**：靠长度与前缀即可辨认；在 schema 上做监控规则比定期审计更可靠。
- **证书与 TLS 配置漂移**：过期证书、仍开着 TLS 1.0、缺 HSTS。
- **源码里的机密**：对包含高熵字符串和已知密钥前缀（`AKIA`、`ghp_`、`sk_live_`）的提交告警，发现即轮换。
- **生成得太快的 token**：测量已签发 token 的熵，而不是信任生成器的文档。
- **携带凭据的 HTTP 端点**：从代理日志里找，而不是靠问。

### 缓解

- **使用经过验证的库和现代默认值。** AES-GCM 或 ChaCha20-Poly1305、TLS 1.2+ 配合合理的套件列表、口令用 bcrypt/scrypt/argon2。不要自己实现原语。
- **用内存硬算法存口令**，并把成本调到单次校验耗时几十毫秒。
- **所有安全 token 都用 CSPRNG 生成**，至少 128 位；重置 token 一次性且短有效期。
- **永远不要把客户端加密当作保护。** 如果服务端不能看到数据，密钥就必须放在服务端到不了的地方。
- **把密钥赶出代码**，放进带轮换的密钥管理里，按环境和用途分别授权。
- **对密文做认证**，并让所有解密失败不可区分。
- **内网流量也要加密**，并在能做检查的位置终止 TLS，而不是干脆跳过它。
- **对提交里的机密告警**，把发现当成事件处理，而不是一条 lint 提示。
