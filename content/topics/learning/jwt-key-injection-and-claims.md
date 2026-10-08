---
id: jwt-key-injection-and-claims
title_en: "JWT, Part 2 — Key Sources and Claims the Token Should Not Choose"
title_zh: "JWT（二）：不该由 token 选择的密钥来源与声明"
summary_en: Part 1 covered the algorithm; this part covers everything else a token can say about its own verification — which key, where the key comes from, and what the claims allow. Measured end to end — /dev/null as a key, kid as a SQL query, a token carrying its own public key, and a token accepted by the wrong service.
summary_zh: 第一篇讲算法；这一篇讲一个 token 关于"怎么验它"还能说的其余一切 —— 用哪把密钥、密钥从哪里来、以及它带来的声明允许什么。全部实测：把 `/dev/null` 当密钥、把 `kid` 当 SQL 查询、token 自带公钥、以及一张被错误的服务接受的 token。
tags: [web, jwt, kid-injection, jku, cwe-347, auth]
tools: [python3, sqlite3, pyjwt, curl]
attck: [T1550.001, T1606.001]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The same pattern, three more fields

Part 1 established the rule with `alg`: **the verifier decides how verification happens, not the thing being verified.** A JWT has three more places where that rule can be broken, and they are all about **the key**.

| Field | What it decides | What trusting it gives the attacker |
|---|---|---|
| `kid` | Which key to use | A key whose contents they already know |
| `jku`, `x5u` | Where to fetch the key from | A key they generated |
| `jwk` | The key itself, inline | The same, without needing to host anything |

And below those, a second family: the **claims**. A valid signature proves who issued a token; it does not prove the token is meant for **this** service, at **this** time, for **this** purpose. Leaving those checks out turns a signed token into a universal one.

### kid: which key

Key rotation is a real requirement, so a token naming its key is a reasonable design — this is what `kid` is for, and real implementations use it. The vulnerability is what the name is used **for**.

#### As a file path

If the implementation resolves `kid` to a path and reads the file, the attacker chooses **which file is used as the signing key**. That is only useful if they can find a file whose **contents they know** — and file systems are full of those:

| `kid` value | Why it works |
|---|---|
| `/dev/null` | Contents are always zero bytes, so the key is the empty string |
| `../../dev/null` | The same file by a relative path, if the prefix is joined blindly |
| `/etc/hostname`, `/proc/self/cmdline` | Predictable or guessable contents |
| A file the attacker can upload | Contents chosen by the attacker |
| A relative path like `.` or a directory | Usually fails to read, and the error discloses the path |

Measured, against an implementation that joins `kid` onto a key directory and reads the file:

```
kid='/dev/null'         read 0 bytes as the key  -> forged token ACCEPTED
kid='../../dev/null'    read 0 bytes as the key  -> forged token ACCEPTED
kid='rotate-2026.key'   read 32 bytes of real key -> forged token rejected
```

And the same forged tokens against an implementation that treats `kid` as a lookup key:

```
kid='/dev/null'         ACCEPTED: False  (unknown kid, refused)
kid='rotate-2026.key'   ACCEPTED: False  (verified against the real key)
```

The difference is one line of design. **A key identifier is a key into a table, not a path into a file system** — and the table has a fixed set of entries, so a value outside it is refused rather than resolved.

`/dev/null` is the classic because it is the cleanest instance of "content known to the attacker": it is empty, so the HMAC key is empty, and an empty key is something anyone can sign with. The general statement is more useful than the example: **any file whose contents the attacker can predict is a candidate key.**

#### As a SQL query

The same `kid` often comes from a key table, and building the query by concatenation puts the identifier on the program side of the string:

```sql
SELECT secret FROM keys WHERE kid = 'rotate-2026.key'      -- intended
SELECT secret FROM keys WHERE kid = 'x' UNION SELECT 'attacker-known-secret' --  -- injected
```

Measured against a local SQLite table:

```
kid = "x' UNION SELECT 'attacker-known-secret' -- "
key the service used: b'attacker-known-secret'    attacker knows it: True
```

The verifier then signs nothing it can notice: it computes an HMAC with a key the attacker chose, over a token the attacker wrote. This is the SQL injection entry arriving through a header field, and the fix is the same one — a parameterised query, or better, not querying at all for something that should be a fixed lookup.

#### Where it ends up in practice

Beyond reading files and querying tables, `kid` has been seen fed into shell commands and into path joins that produce enumeration (`/keys/1`, `/keys/2`, …). The pattern behind all of them is that the value is **used to reach something**, rather than **matched against a set**.

### jku, x5u and inline jwk: choosing the key source

The three fields that name or carry a key are worth grouping, because the failure is the same shape as `kid` and easier to overlook.

| Field | Legitimate use | Attack |
|---|---|---|
| `jku` | URL of a JWKS document holding the public keys | Point it at a JWKS the attacker hosts, containing their own key |
| `x5u` | URL of an X.509 certificate chain | The same, with a certificate the attacker generated |
| `jwk` | The public key, embedded in the header | The key travels inside the token, so nothing needs to be hosted |

The inline `jwk` is the purest version: **the token brings its own public key**, so a verifier that "verifies the signature using the key from the header" is checking that the token was signed by whoever wrote it — which is always true.

Measured, generating a key and embedding it:

```
token length: 931   header contains jwk: True   jwk.kty: RSA
```

The key in that header was generated by the attacker moments earlier, and a verifier that trusts it accepts the token by construction.

**The distinction that matters** is between a key that arrives **with** the data and a key that is **configured ahead of time**. In a legitimate `jku` flow the URL list is fixed by the deployment — `https://auth.example.com/.well-known/jwks.json` — and the header field, if honoured at all, only chooses among known issuers. When the header can introduce a **new** URL, the deployment has handed key selection to the attacker.

### Claims: a valid signature is not enough

Everything above is about forging a token. This section is about using a **genuine** one in a place it was not meant for — which needs no forgery at all, and is therefore often overlooked.

| Claim | What it is for | What skipping it allows |
|---|---|---|
| `exp` / `nbf` | When the token is valid | A token that never expires |
| `aud` | Who the token is for | **A token issued for one service, used against another** |
| `iss` | Who issued it | A token from a different issuer accepted, if keys are shared |
| `typ` | What kind of token it is | An ID token used as an access token |
| Application claims | Roles, scopes, tenant | Whatever the application decides on |

Measured, with two services sharing a secret key — which is itself the precondition that makes this possible:

```
A token issued for orders-api, presented to billing-api:

  no claim checks        ACCEPTED: True   (let through)
  aud checked            ACCEPTED: False  (aud mismatch: issued for orders-api)
  aud + exp + iss        ACCEPTED: False  (aud mismatch)

An already-expired token:

  exp not checked        ACCEPTED: True   (let through)
  exp checked            ACCEPTED: False  (expired)
```

Two conclusions worth separating:

**Shared keys make tokens portable.** If several services verify with the same secret, a token from any of them is cryptographically valid at all of them, and the **only** thing standing between a low-privilege user at one service and an admin endpoint at another is the `aud` check. That is why "we all use the same secret" and "we do not check `aud`" are a pair, not two independent choices.

**Expiry is not enforced by the format.** `exp` is a field, and a field that is not read has no effect. A token library that validates the signature and hands back the claims has done exactly what it was asked; deciding whether the claims are acceptable for this request is the application's job, and it is frequently left undone.

### Detection and mitigation

- **Alert on `kid` values containing path syntax.** `/`, `..`, `\`, a leading `.`, a NUL byte, or a drive letter in a key identifier are never legitimate and always indicate an attempt to make a lookup into a file read. The same rule catches `kid` values that look like SQL fragments: quotes, `--`, `UNION`, `OR 1=1`.
- **Alert on `jku`, `x5u` and inline `jwk` at all.** A token whose header carries a `jwk` is anomalous by definition for a service that configures its keys. A `jku` or `x5u` pointing at a domain the deployment does not own is an explicit attempt to introduce a key, and both are cheap to detect because the expected values are a short fixed list.
- **Log the header alongside the verification result.** `alg`, `kid`, presence of `jku`/`x5u`/`jwk`, and whether the signature verified. In an incident this is the difference between "tokens were being forged" and "here is the algorithm and key identifier that were being tried".
- **Treat claim-validation failures as security events, and their absence as a finding.** A token that verifies but whose `aud` does not match, or whose `exp` has passed, should be a logged rejection — and a service that never produces those rejections is a service that is not checking.
- **Resolve `kid` against a fixed table, and refuse anything not in it.** A key identifier selects from a set of keys the verifier already has; it must not become a file path, a query, a command or a URL. Where the table is generated at deployment, generate it from configuration rather than from a directory listing.
- **Pin the key source, and refuse keys that arrive with the token.** Configure a fixed JWKS URL if `jku` is needed, validate the certificate chain if `x5u` is, and reject an inline `jwk` outright unless the architecture genuinely requires it — in which case the key still has to be checked against something already trusted.
- **Validate every claim the token carries, and only use claims you validated.** `exp`, `nbf`, `aud`, `iss` and any application-specific claim that feeds a decision. For the application claims, an allowlist of accepted values is stronger than a check for the presence of a field.
- **Give each service its own key, and keep lifetimes short.** Distinct secrets per service mean a token from one is not valid at another even if `aud` is missed. Short lifetimes bound what a stolen or misused token is worth, and a denylist covers the window that remains.
- **Never let an error message name the key path or the expected identifier.** The measured file-read failure prints the path it tried, which turns a login screen into a way to test whether a file exists. Log it; do not return it.
- **And read part 1's rule as the general one, not as a rule about `alg`.** Every field in this entry — `kid`, `jku`, `x5u`, `jwk` — is the token deciding something about its own verification. The question to ask of any token-based design is: **which of these decisions does the verifier make for itself, and which does it accept from the input?**

<!-- lang:zh -->
### 同一个模式，另外三个字段

第一篇用 `alg` 立下了那条规则：**由验证方决定验证怎么发生，而不是由被验证的那个东西决定。** 一个 JWT 还有三处会违反这条规则，而它们全都是关于**密钥**的。

| 字段 | 它决定什么 | 信任它等于给了攻击者什么 |
|---|---|---|
| `kid` | 用哪把密钥 | 一把他已经知道内容的密钥 |
| `jku`、`x5u` | 去哪里取密钥 | 一把他自己生成的密钥 |
| `jwk` | 密钥本身，内嵌 | 同上，而且不需要托管任何东西 |

而在这些之下还有第二族：**声明**。一个有效的签名证明是谁签发了这个 token；它不证明这个 token 是为**这个**服务、在**这个**时间、为**这个**用途准备的。把这些检查省掉，一张签过名的 token 就变成了万能 token。

### `kid`：用哪把密钥

密钥轮换是真实需求，所以"token 指明自己用哪把密钥"是合理设计 —— `kid` 就是干这个的，真实实现也这么用。漏洞在于**那个名字被拿去做了什么**。

#### 当成文件路径

如果实现把 `kid` 解析成路径并读文件，那么攻击者就选择了**哪个文件被当签名密钥用**。这只有在他能找到"**内容自己知道**"的文件时才有用 —— 而文件系统里遍地都是：

| `kid` 的值 | 为什么可行 |
|---|---|
| `/dev/null` | 内容恒为零字节，所以密钥就是空串 |
| `../../dev/null` | 同一个文件走相对路径，如果前缀是盲目拼接的 |
| `/etc/hostname`、`/proc/self/cmdline` | 内容可预测或可猜 |
| 一个攻击者能上传的文件 | 内容由攻击者选 |
| 像 `.` 或某个目录这样的相对路径 | 通常读失败，而报错会泄漏路径 |

实测，对着一个"把 `kid` 拼到密钥目录后读文件"的实现：

```
kid='/dev/null'          读到 0 字节当密钥   -> 伪造 token 被接受
kid='../../dev/null'     读到 0 字节当密钥   -> 伪造 token 被接受
kid='rotate-2026.key'    读到 32 字节真密钥  -> 伪造 token 被拒绝
```

同一批伪造 token，对着一个"把 `kid` 当查表的键"的实现：

```
kid='/dev/null'          接受: False  （未知 kid，拒绝）
kid='rotate-2026.key'    接受: False  （按真实密钥验签）
```

差别只是一行设计。**密钥标识符是查表用的键，不是文件系统里的路径** —— 而表有一组固定的条目，所以表外的值会被**拒绝**，而不是被**解析**。

`/dev/null` 之所以经典，是因为它是"内容为攻击者所知"最干净的实例：它是空的，所以 HMAC 密钥是空的，而空密钥是谁都能拿来签的。但通用表述比这个例子更有用：**任何内容可被攻击者预测的文件，都是一个候选密钥。**

#### 当成 SQL 查询

同一个 `kid` 常常来自一张密钥表，而用拼接构造查询，就把这个标识符放到了字符串的**程序侧**：

```sql
SELECT secret FROM keys WHERE kid = 'rotate-2026.key'      -- 本意
SELECT secret FROM keys WHERE kid = 'x' UNION SELECT 'attacker-known-secret' --  -- 注入后
```

对一张本地 SQLite 表实测：

```
kid = "x' UNION SELECT 'attacker-known-secret' -- "
服务端用到的密钥: b'attacker-known-secret'    攻击者已知: True
```

于是验证方算出的 HMAC 用的是攻击者选的密钥、算的是攻击者写的 token，而它自己察觉不到。这就是 SQL 注入那篇从一个请求头字段进来，而修法也一样 —— 参数化查询，或者更好：对一个本该是固定查表的东西，根本不要查。

#### 实践中它还会去哪里

除了读文件和查表，`kid` 还被见过喂进 shell 命令、以及喂进会产生枚举的路径拼接（`/keys/1`、`/keys/2` …）。它们背后的模式是同一个：那个值被**用来够到某个东西**，而不是**与一个集合比对**。

### `jku`、`x5u` 与内嵌 `jwk`：选择密钥来源

这三个"指明或携带密钥"的字段值得放在一起，因为失效形状和 `kid` 一样，却更容易被忽略。

| 字段 | 正当用途 | 攻击 |
|---|---|---|
| `jku` | 放着公钥的 JWKS 文档的 URL | 指向攻击者自己托管的 JWKS，里面是他的密钥 |
| `x5u` | X.509 证书链的 URL | 同上，用他自己生成的证书 |
| `jwk` | 公钥，内嵌在头部 | 密钥随 token 一起走，不需要托管任何东西 |

内嵌的 `jwk` 是最纯粹的版本：**token 自带公钥**，于是"用头部里的密钥验证签名"的验证方，验证的其实是"这个 token 是由写它的人签的" —— 而那永远是成立的。

实测，生成一把密钥并嵌进去：

```
token 长度: 931   头部含 jwk: True   jwk.kty: RSA
```

那个头部里的密钥是攻击者片刻之前生成的，而一个信任它的验证方按构造就会接受。

**真正要紧的区分**，是密钥**随数据一起来**，还是**事先配置好**。在正当的 `jku` 流程里，URL 列表由部署固定 —— `https://auth.example.com/.well-known/jwks.json` —— 而头部那个字段即使被采信，也只是在已知的签发者之间做选择。当头部能**引入一个新的** URL 时，这个部署就把密钥选择权交给了攻击者。

### 声明：签名有效还不够

上面讲的都是伪造 token。这一节讲的是把一张**真实的** token 用在他不该被用的地方 —— 这完全不需要伪造，因此常被忽略。

| 声明 | 它的用途 | 不校它会允许什么 |
|---|---|---|
| `exp` / `nbf` | token 什么时候有效 | 一张永不过期的 token |
| `aud` | token 是给谁的 | **签发給一个服务的 token，被拿去打另一个服务** |
| `iss` | 谁签发的 | 在密钥共享的前提下，别家签发的 token 被接受 |
| `typ` | 这是什么类型的 token | 把 ID token 当 access token 用 |
| 应用自定义声明 | 角色、范围、租户 | 应用拿它决定什么，就是什么 |

实测，两个服务共享同一把密钥 —— 而那本身就是让这件事成立的前提：

```
一张签给 orders-api 的 token，拿到 billing-api 上用：

  完全不校声明      接受: True   （放行）
  校 aud            接受: False  （aud 不匹配：签发給了 orders-api）
  校 aud+exp+iss    接受: False  （aud 不匹配）

一张已经过期的 token：

  不校 exp          接受: True   （放行）
  校 exp            接受: False  （已过期）
```

两个结论值得分开说：

**共享密钥让 token 变得可搬移。** 如果好几个服务用同一个密钥验签，那么其中任何一个签发的 token 在其余所有服务上都是密码学有效的，而挡在"A 服务的低权限用户"与"B 服务的管理接口"之间的**唯一**东西就是 `aud` 检查。这就是为什么"我们都用同一个密钥"和"我们不校 `aud`"是一对选择，而不是两个独立的选择。

**过期不是格式强制的。** `exp` 是一个字段，而一个没人读的字段没有作用。一个验证了签名、把声明交回来的库，做的正是它被要求做的事；判断这些声明对**这次请求**是否可接受，是应用的活，而它经常被漏掉。

### 检测与缓解

- **对含路径语法的 `kid` 值告警。** 密钥标识符里的 `/`、`..`、`\`、开头的 `.`、NUL 字节、或者盘符，从来不是正当的，而且总是意味着有人想把查表变成读文件。同一条规则也能抓到长得像 SQL 片段的 `kid`：引号、`--`、`UNION`、`OR 1=1`。
- **对 `jku`、`x5u` 与内嵌 `jwk` 本身告警。** 一个头部携带 `jwk` 的 token，对一个自己配置密钥的服务来说定义上就是异常。指向部署并不拥有的域的 `jku` 或 `x5u` 是明确的"试图引入一把密钥"，而两者都很容易检测，因为期望值是一份很短的固定清单。
- **把头部与验签结果一起记下来。** `alg`、`kid`、有没有 `jku`/`x5u`/`jwk`、以及签名是否通过。在一场事故里，这决定了你是只能说"有人在伪造 token"，还是能说"这是他们试的算法与密钥标识符"。
- **把声明校验失败当作安全事件，把它的缺席当作一条发现。** 一个签名通过、但 `aud` 不匹配或 `exp` 已过的 token，应当是一条被记录的拒绝 —— 而一个从来不产生这类拒绝的服务，就是一个没有在校验的服务。
- **`kid` 要对一张固定的表解析，表外的一律拒绝。** 密钥标识符是从"验证方已经拥有的密钥集合"里选一个；它不能变成文件路径、查询、命令或 URL。当这张表是部署时生成的，就从配置生成它，而不是从目录列表。
- **钉住密钥来源，并拒绝随 token 一起来的密钥。** 如果确实需要 `jku`，就配置一个固定的 JWKS URL；如果需要 `x5u`，就校验证书链；而内嵌的 `jwk` 应当直接拒绝 —— 除非架构确实需要它，而在那种情况下，那把密钥仍然必须与某个已经受信的东西比对过。
- **校验 token 携带的每一个声明，并且只用你校验过的声明。** `exp`、`nbf`、`aud`、`iss`，以及任何会喂进决策的应用自定义声明。对应用声明来说，一份"可接受值"的允许清单比"检查字段是否存在"要强。
- **给每个服务各自的密钥，并把有效期做短。** 每个服务独立的密钥意味着，即使 `aud` 被漏掉，一个服务的 token 在另一个服务上也不有效。短有效期限制了一张被盗用或被误用的 token 值多少，而黑名单覆盖剩下的那段窗口。
- **绝不要让报错说出密钥路径或期望的标识符。** 实测里那次读文件失败会打印它尝试过的路径，这就把一个登录页变成了"某个文件是否存在"的探测器。记进日志；不要返回给客户端。
- **并且把第一篇那条规则当作通则来读，而不是一条关于 `alg` 的规则。** 这一篇里的每一个字段 —— `kid`、`jku`、`x5u`、`jwk` —— 都是 token 在决定关于自己验证方式的事情。对任何基于 token 的设计都该问：**这些决定里，哪些是验证方自己做的，哪些是它从输入里接受的？**
