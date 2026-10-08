---
id: jwt-fundamentals
title_en: "JWT, Part 1 — The Token Declares How to Verify It"
title_zh: "JWT（一）：token 自己声明怎么验它"
summary_en: A JSON Web Token carries its own verification parameters in a header the attacker also controls, which is the entire class in one sentence. This entry covers the three-part structure, alg none across its spellings, the RS256-to-HS256 confusion measured against a naive verifier and a correct one, and why an HMAC secret can be attacked offline.
summary_zh: 一个 JSON Web Token 把"怎么验证它"的参数放在一个攻击者同样能改的头部里 —— 这一整类漏洞就浓缩在这一句里。这一篇讲三段结构、alg 为 none 的各种拼法、RS256 与 HS256 的混淆（对着一个天真验证器与一个正确验证器实测），以及为什么 HMAC 密钥可以被离线爆破。
tags: [web, jwt, cwe-347, auth, algorithm-confusion, token]
tools: [python3, pyjwt, openssl, hashcat, curl]
attck: [T1550.001, T1606.001]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The whole class in one sentence

A JSON Web Token has three parts joined by dots:

```
base64url(header) . base64url(payload) . base64url(signature)
```

The header is JSON, and it contains the field `alg` — **the name of the algorithm the verifier should use**. That field is written by whoever created the token, and in a typical attack that is the attacker.

> **The thing being verified supplies the parameters for how to verify it.**

That is the same trust-boundary failure as every other entry in this series, in an unusually pure form. It is not a parsing bug, not a weak algorithm and not a missing check in the library: it is an application accepting a **security decision parameter from the object it is deciding about**.

A second token field, `kid`, names **which key** to use, and it is supplied the same way. The two together give the class its shape:

| Field | What it decides | What goes wrong when it is trusted |
|---|---|---|
| `alg` | How to verify the signature | The attacker chooses an algorithm that cannot fail |
| `kid` | Which key to verify with | The attacker chooses a key whose value they know |

One sentence covers both, and it is the design rule the attacks violate: **the verifier decides how verification happens, not the thing being verified.**

### Structure, and why a signature is not encryption

The payload is base64url, which is an **encoding, not a cipher**. Anyone holding the token can read it:

```python
import base64, json

def b64d(s: str) -> bytes:
    return base64.urlsafe_b64decode(s + "=" * (-len(s) % 4))

token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiYW5hbHlzdCIsInJvbGUiOiJ2aWV3ZXIifQ.XXXX"
h, p, sig = token.split(".")
print(json.loads(b64d(h)))    # {'alg': 'HS256', 'typ': 'JWT'}
print(json.loads(b64d(p)))    # {'user': 'analyst', 'role': 'viewer'}
```

Measured on a freshly minted token: three segments, the header and payload readable as plain JSON, and a 32-byte signature for HS256.

Two practical consequences, both frequently got wrong:

- **Do not put anything confidential in a JWT.** The signature proves the token was not modified and (with a shared secret) who issued it. It says nothing about who can read it.
- **The payload is attacker-controlled input in every sense.** Even when the signature is valid, the claims inside were chosen by the issuer — and when the signature check can be defeated, they are chosen by the attacker.

### alg: none

The weakest form of the attack, and the one worth checking first because it takes one request.

**The mechanism.** A verifier that reads `alg` from the token, and treats `none` as "no signature required", accepts a token with an empty third segment:

```python
def make_none(claims: dict) -> str:
    enc = lambda o: base64.urlsafe_b64encode(json.dumps(o, separators=(",", ":")).encode()).rstrip(b"=").decode()
    return f"{enc({'alg': 'none', 'typ': 'JWT'})}.{enc(claims)}."
```

The token ends with a dot and carries no signature at all, and any claim inside it is taken at face value.

**The spellings matter.** Because implementations differ in how they normalise the field, all of these are worth trying, and all four were constructible in the same test:

| Spelling | Note |
|---|---|
| `none` | The canonical form |
| `None` | Libraries that do a case-insensitive comparison against a list |
| `NONE` | Same, for the upper-case variant |
| `nOnE` | Defeats a check written as `alg == "None"` or `alg.lower() != "none"` inconsistently |

**Why it exists at all** is worth knowing, because it explains why the field is accepted rather than rejected: `none` is part of the specification, for tokens whose integrity is provided by other means. A library that supports the specification has to be told **not** to allow it, and a verifier that takes the algorithm from the token has effectively been told to allow whatever arrives.

### Algorithm confusion: RS256 and HS256

This is the more interesting one, and it is worth measuring because the failure is a **type confusion** rather than a missing check.

The setup: the service signs with **RS256** (RSA, asymmetric) and publishes its **public key**, as it must. The attacker sends a token whose header says `HS256` (HMAC, symmetric).

A verifier that picks its verification path from the token's `alg` now loads what it has configured — the RSA public key — and uses it as the **HMAC secret**:

```
header   {"alg": "HS256", ...}      <- chosen by the attacker
verify   HMAC-SHA256(public_key_bytes, header + "." + payload)
```

And the public key is public. Anyone can compute that HMAC, so **anyone can mint valid tokens** — with whichever claims they like.

Measured, with a freshly generated 2048-bit RSA key, against two verifiers:

| Token | Naive verifier (algorithm from the token) | Correct verifier (algorithm fixed in configuration) |
|---|---|---|
| Forged with the public key as the HMAC secret | **accepted** | **rejected** |
| `alg: none`, empty signature | **accepted** | **rejected** |

The forged token was 125 characters and was accepted by the naive verifier without complaint. **Neither verifier is broken as a piece of code** — one asks the token how to verify it, the other decides for itself.

**A detail that costs time in practice:** the HMAC key has to be the public key in the **same form** the library would have loaded it. With a PEM-encoded key that is the PEM bytes (451 bytes in the test), and with DER it is the DER bytes (294 bytes) — and some implementations use the raw modulus or a JWK field instead. Trying the wrong form produces a signature that simply does not match, which looks like "the attack failed" when it is the encoding that is wrong.

### Offline attack against the HMAC secret

When the algorithm really is HMAC, the token's signature can be **verified without contacting the server** — that is the point of a signature. Which means guessing the secret is also offline, unlimited by rate limiting, lockouts or logging.

Measured against a token signed with `secret`:

```
dictionary of 8 candidates -> found 'secret' in under a millisecond
```

The measurement is unimpressive because the dictionary is small; the property is what matters:

- **No requests are sent**, so nothing in the application or the WAF observes the attempt.
- **The search space is whatever the secret is.** A secret from a config file, an example, or a short phrase is exhaustible; `hashcat -m 16500` does this with a GPU.
- **The finding generalises**: the same token format appears in APIs, mobile clients, webhooks, session cookies and single-sign-on flows, so one recovered secret is often many tokens.

The rule that follows is simple and frequently ignored: **the secret must be a high-entropy random value**, not a memorable string, and it must not be shared between environments.

### Detection and mitigation

- **Alert on `alg` values that do not match the deployment.** A service that issues HS256 and receives an RS256 token, or that receives `none` in any spelling, is being probed at minimum. Log the algorithm alongside the verification result, and treat a mismatch as a security event rather than a validation error.
- **Alert on signature verification failures in bulk, and on malformed tokens.** A burst of failures against a token endpoint is either a brute-force attempt or a client that has been misconfigured — both worth seeing. A token with only two segments, or three where the third is empty, never comes from a legitimate issuer.
- **Watch claims that a token should not be able to change.** The same `sub` appearing with a different `role` over time, or a `role` that escalates while everything else stays constant, is the signature of someone editing claims rather than stealing sessions.
- **Fix the algorithm in configuration and pass it explicitly.** Every library has a way to name the accepted algorithms — in `pyjwt` it is `jwt.decode(token, key, algorithms=["HS256"])`, and the list form is required for exactly this reason. Configure it from the service's own settings, and refuse `none` outright.
- **Use asymmetric algorithms where tokens cross a trust boundary.** With RS256 the verifier holds only the public key, so a leaked verifier cannot mint tokens; and the algorithm-confusion attack requires the verifier to be confused, which a fixed algorithm prevents.
- **Treat the HMAC secret as a key, not a string.** Generated randomly at deployment, at least as long as the hash output, rotated, stored in a secrets manager, and different per environment. A secret in the repository or in an example configuration is a public secret.
- **Validate every claim, and treat the payload as untrusted input.** `exp` and `nbf` for time, `aud` and `iss` for who the token is for and from, and any application-specific claim used for a decision. A signature proves the token was issued by someone holding the key — it does not prove the contents are appropriate for **this** service.
- **Do not use a JWT as a replacement for server-side session state.** A signed token cannot be revoked without a denylist, so a stolen token stays valid until it expires; keep lifetimes short, and keep anything that must be revocable on the server.

<!-- lang:zh -->
### 一整类漏洞，一句话

一个 JSON Web Token 由点号连起的三段组成：

```
base64url(header) . base64url(payload) . base64url(signature)
```

头部是 JSON，里面有一个 `alg` 字段 —— **验证方应该使用哪种算法**。那个字段由创建 token 的人写入，而在典型的攻击里，那个人就是攻击者。

> **被验证的东西，提供了"怎么验证它"的参数。**

这和其他每一篇里的是同一个信任边界失效，只是形式格外纯粹。它不是解析 bug、不是算法太弱、也不是库里少了一道检查：它是**应用从一个"正要被它判断的对象"那里接受了安全决策参数**。

还有第二个字段 `kid`，它说明**用哪把密钥**，而它的来源是一样的。两者合起来，就是这一类的形状：

| 字段 | 它决定什么 | 被信任时出什么问题 |
|---|---|---|
| `alg` | 怎么验签 | 攻击者挑一个不可能失败的算法 |
| `kid` | 用哪把密钥验 | 攻击者挑一把他知道内容的密钥 |

一句话把两者都覆盖了，而这正是那些攻击所违反的设计规则：**由验证方决定验证怎么发生，而不是由被验证的那个东西决定。**

### 结构，以及为什么签名不是加密

载荷是 base64url，那是一种**编码，不是密码**。拿到 token 的任何人都能读它：

```python
import base64, json

def b64d(s: str) -> bytes:
    return base64.urlsafe_b64decode(s + "=" * (-len(s) % 4))

token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyIjoiYW5hbHlzdCIsInJvbGUiOiJ2aWV3ZXIifQ.XXXX"
h, p, sig = token.split(".")
print(json.loads(b64d(h)))    # {'alg': 'HS256', 'typ': 'JWT'}
print(json.loads(b64d(p)))    # {'user': 'analyst', 'role': 'viewer'}
```

在一个新签发的 token 上实测：三段、头部与载荷是可读的普通 JSON、HS256 的签名是 32 字节。

由此有两个实际后果，而两个都经常被搞错：

- **不要把任何机密放进 JWT。** 签名证明 token 没被改过、并在共享密钥的前提下证明是谁签发的；它对"谁可以读"什么都没说。
- **载荷在任何意义上都是攻击者可控的输入。** 即使签名有效，里面的声明也是签发者选的 —— 而当验签本身可以被击败时，那就是攻击者选的。

### `alg: none`

这一类里最弱的形式，也是首先该试的，因为它只需要一个请求。

**机制。** 一个从 token 里读 `alg`、并把 `none` 当作"不需要签名"的验证方，会接受第三段为空的 token：

```python
def make_none(claims: dict) -> str:
    enc = lambda o: base64.urlsafe_b64encode(json.dumps(o, separators=(",", ":")).encode()).rstrip(b"=").decode()
    return f"{enc({'alg': 'none', 'typ': 'JWT'})}.{enc(claims)}."
```

这个 token 以一个点结尾、完全不携带签名，而它里面的任何声明都按字面被采信。

**拼法是要紧的。** 因为各实现对那个字段的规范化方式不同，下面这些都值得一试 —— 在同一次测试里四种都能构造出来：

| 拼法 | 说明 |
|---|---|
| `none` | 规范形式 |
| `None` | 那些对一张列表做不区分大小写比较的库 |
| `NONE` | 同上，大写变体 |
| `nOnE` | 打穿那些写成 `alg == "None"`、或者大小写处理不一致的检查 |

**它为什么会存在**，值得知道，因为这解释了为什么这个字段是被接受而不是被拒绝的：`none` 是规范的一部分，用于那些完整性由其他方式提供的 token。一个支持规范的库必须被告知**不许**允许它，而一个从 token 里取算法的验证方，等于被告知"来什么就允许什么"。

### 算法混淆：RS256 与 HS256

这是更有意思的那个，值得实测，因为它的失效是一种**类型混淆**，而不是少了一道检查。

设定：服务用 **RS256**（RSA，非对称）签名，并公布它的**公钥** —— 那是它必须做的。攻击者发来一个头部写着 `HS256`（HMAC，对称）的 token。

一个从 token 的 `alg` 决定验证路径的验证方，现在会把**它配置着的东西** —— RSA 公钥 —— 加载进来，并把它当作 **HMAC 密钥**用：

```
头部   {"alg": "HS256", ...}      <- 攻击者选的
验证   HMAC-SHA256(公钥字节, 头部 + "." + 载荷)
```

而公钥是公开的。任何人都能算出那个 HMAC，于是**任何人都能铸造有效的 token** —— 声明随他们写。

实测，用一把新生成的 2048 位 RSA 密钥，对着两个验证器：

| token | 天真验证器（算法取自 token） | 正确验证器（算法在配置里固定） |
|---|---|---|
| 用公钥当 HMAC 密钥伪造的 | **接受** | **拒绝** |
| `alg: none`、空签名 | **接受** | **拒绝** |

那个伪造的 token 有 125 个字符，被天真验证器毫无异议地接受了。**两个验证器作为一段代码都没有坏** —— 一个问 token 该怎么验它，另一个自己决定。

**一个在实际中很费时间的细节：** HMAC 的密钥必须是公钥的**某一种具体形式**，也就是库本来会加载的那一种。PEM 编码的密钥就是 PEM 字节（测试里 451 字节），DER 就是 DER 字节（294 字节），而有些实现用的是裸模数或 JWK 里的某个字段。试错了形式，得到的签名就是单纯不匹配 —— 那看起来像"攻击失败了"，而实际上是编码形式错了。

### 针对 HMAC 密钥的离线攻击

当算法确实是 HMAC 时，token 的签名可以**在不联系服务器的情况下被验证** —— 那正是签名的意义。这意味着猜密钥也是离线的，不受限流、锁定或日志的限制。

对一个用 `secret` 签名的 token 实测：

```
8 条候选的字典 -> 找到 'secret'，耗时不到一毫秒
```

这个数字不起眼是因为字典很小；要紧的是那个性质：

- **一个请求都不发**，所以应用和 WAF 都观察不到这次尝试。
- **搜索空间就是密钥本身。** 一个来自配置文件、示例或短句子的密钥是可穷尽的；`hashcat -m 16500` 用 GPU 干这件事。
- **这个发现是可推广的**：同一种 token 格式出现在 API、移动客户端、webhook、会话 cookie 与单点登录流程里，所以一个被还原的密钥常常是很多个 token。

由此得出的规则很简单，也经常被忽略：**密钥必须是一个高熵的随机值**，不是一个好记的字符串，而且不能在不同环境之间共用。

### 检测与缓解

- **对与部署不符的 `alg` 值告警。** 一个签发 HS256 的服务收到 RS256 的 token，或者收到任何拼法的 `none`，至少是在被探测。把算法与验签结果一起记下来，并把算法不匹配当作安全事件而不是校验错误。
- **对成批的验签失败与畸形 token 告警。** 一个 token 端点上爆发式的失败，要么是一次爆破尝试，要么是一个配置错了的客户端 —— 两者都值得看见。只有两段的 token，或者第三段为空的三段 token，从来不是正当签发者产出的。
- **盯那些"token 本不该改变"的声明。** 同一个 `sub` 在不同时间带着不同的 `role`，或者 `role` 在其余一切不变的情况下逐步提升，就是有人在改声明而不是在盗会话的特征。
- **把算法固定在配置里，并显式传进去。** 每个库都有办法指明接受的算法 —— 在 `pyjwt` 里是 `jwt.decode(token, key, algorithms=["HS256"])`，而正是因为这个原因，列表形式是必需的。从服务自己的设置里配置它，并直接拒绝 `none`。
- **在 token 跨越信任边界的地方使用非对称算法。** 用 RS256 时，验证方只持有公钥，所以一个被攻陷的验证方无法铸造 token；而算法混淆攻击要求验证方被弄混，一个固定的算法就能阻止。
- **把 HMAC 密钥当作密钥，而不是当作字符串。** 部署时随机生成、长度至少与哈希输出相同、定期轮换、存在密钥管理服务里、每个环境各不相同。放在仓库里或示例配置里的密钥，就是一个公开的密钥。
- **校验每一个声明，并把载荷当作不可信输入。** 时间上校 `exp` 与 `nbf`，用途与来源上校 `aud` 与 `iss`，以及任何被用来做决策的应用自定义声明。签名证明 token 是由某个持有密钥的人签发的 —— 它不证明里面的内容是**为这个服务**准备的。
- **不要用 JWT 取代服务端会话状态。** 一个已签名的 token 没有黑名单就无法撤销，所以被偷走的 token 会一直有效到过期；把有效期做短，并把任何必须可撤销的东西留在服务端。
