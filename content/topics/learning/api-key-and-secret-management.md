---
id: api-key-and-secret-management
title_en: API Keys and Secret Management
title_zh: API 密钥与密钥管理
summary_en: Credentials come in two kinds with opposite security properties — low-entropy ones that a slow function has to protect and high-entropy ones that simply cannot be searched — and the failures are spread across generation, storage, distribution, rotation and revocation. Measured with real numbers — a short key cracked, a keyspace table, the cost difference between a fast hash and a slow one, a secret still present in git history, and a scanner that finds three secrets and five things that are not.
summary_zh: 凭据分两种，而它们的安全属性正好相反 —— 低熵的那些要靠慢函数保护，高熵的那些根本搜不动 —— 而失败散布在生成、存储、分发、轮换与吊销这几步里。这一篇给出的是真实数字：一个短密钥被破出、一张密钥空间表、快哈希与慢 KDF 的代价差、一个仍在 git 历史里的密钥，以及一个扫出三个密钥和五样不是密钥的东西的扫描器。
tags: [beginner, api, secrets, keys, rotation, git]
tools: [git, gitleaks, trufflehog, python3]
attck: [T1552, T1078]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Two kinds of credential, with opposite protections

Nearly every mistake in this area comes from treating one kind of credential like the other.

> **A password is guessed. A key is (or is not) searched. The first is protected by making each attempt expensive; the second by making the space too large to traverse.**

Everything else here follows from that split: how a credential is generated, how it is stored at rest, whether it needs rate limiting, and what "revocation" has to mean.

Measured below on one host, telling the numbers so that the reasoning is visible: a short key actually cracked, a table of what different key lengths are worth, the cost difference between hashing and a key-derivation function, a secret recovered from a repository, and a scanner's output examined for what it missed and what it invented.

### Part 1: the shape of it

| | A password | An API key or token |
|---|---|---|
| entropy | **low — a person chose it** | **high — generated** |
| attack | guessing, offline against a stolen hash | searching the space |
| protection | **a slow function, plus rate limiting** | **length and randomness** |
| storage | must be slow-hashed | hashed, or encrypted if it must be recovered |
| rotation | by the user, rarely | **by the operator, on a schedule** |
| revocation | for one account | for one key, without touching others |

**And a credential has a lifecycle with five stages, each with its own failure**: generated, distributed to whoever needs it, stored, used, and eventually rotated or revoked. **Most incidents are in distribution and in storage**, which is why those get their own parts below.

### Part 2: what entropy is worth

Measured first, the throughput of the search itself, on this host:

```
SHA-256 in a tight loop                3,822,677 hashes/second
the same, in the cracking loop          2,473,336 tries/second
```

Then an actual crack of a key drawn from a four-letter lowercase alphabet — unknown to the script that searched for it:

```
candidates tried:      74,557
time taken:            under a second
recovered:             the four-letter key
```

**Then the same measured rate applied to longer keys:**

| Key | Candidates | Time to exhaust, at the measured rate |
|---|---|---|
| 6 lowercase letters | 308,915,776 | **1.3 minutes** |
| 8 lowercase letters | 208,827,064,576 | **15.2 hours** |
| 8 alphanumeric | 218,340,105,584,896 | **2 years** |
| 16 lowercase letters | 43,608,742,899,428,874,059,776 | **361 million years** |
| 16 random bytes (32 hex digits) | 3.4e38 | 2.8e24 years |
| 32 random bytes | 1.2e77 | 9.6e62 years |

**Read the first three rows as one lesson and the rest as another.** Six characters is a minute; eight characters is a day; eight alphanumeric is two years. **Every added character multiplies the cost by the size of the alphabet**, which is why the difference between "too short" and "enough" is a small number of characters and a large factor.

**And read the last three as the actual recommendation.** Once a key is drawn from a real random source at 128 bits or more, the search is not the attack any more — **the attack is finding the key somewhere it was written down**, which is the subject of Parts 5 and 6.

**And the rate is the attacker's, not this host's.** The measured number is a Python loop on one core; dedicated hardware does orders of magnitude better and costs what a graphics card costs. **The reason to state a measured figure is to show the shape of the curve, and the reason to design with margin is that the curve only moves one way.**

**And the generator matters as much as the length.** A long string built from a timestamp, a counter and a random suffix is not a long random string; **the searchable part is the part that came from a guessable source**, whatever its length.

### Part 3: a prefix adds convenience and subtracts nothing

Keys are usually issued with a recognisable shape, and the measurements of the shapes are worth having:

| Random input | Resulting key |
|---|---|
| 128 bits | 22 URL-safe characters |
| 192 bits | 32 characters |
| 256 bits | 43 characters |

**A prefix like `sk_live_` adds no entropy at all** — it is the same for every key — and it does two useful things and one that is worth naming:

**It lets the operator tell a key from a random string**, in a log, a paste, or a support ticket.
**It lets the format carry a public identifier**, so a lookup can find the record without storing the secret — the `prefix + id + secret` shape, where only the last part must be secret.
**And it lets anybody else tell a key from a random string too**, which is what a scanner does.

> **A recognisable format is a deliberate trade: it makes the key findable by the people who operate it, and by anyone else who looks.**

**And the correctness requirement that follows is specific**: the public part must identify the record and the secret part must be compared against a stored hash, **and the comparison must be constant-time** — the measurement for which is in the authentication entry.

### Part 4: storing them, where the two kinds diverge

Measured, the cost of the two ways to store a credential on this host:

| Function | Per call | Rate |
|---|---|---|
| SHA-256 | **0.0003 ms** | 3,120,626 / second |
| PBKDF2-HMAC-SHA256, 1 iteration | 0.0054 ms | — |
| PBKDF2-HMAC-SHA256, 100,000 iterations | **10.7 ms** | 94 / second |
| PBKDF2-HMAC-SHA256, 600,000 iterations | **64.3 ms** | 16 / second |

**That is a factor of about thirty thousand between the two ends**, and the point is which credential each one is for:

> **A slow function exists to make a small search space expensive. It does not make a space larger — so it protects a password, and it is unnecessary for a 256-bit key that cannot be searched either way.**

**Which gives a rule that is easy to state and easy to get wrong in both directions**: hashing a random key with SHA-256 is fine, because the space is what protects it; storing a user's password with SHA-256 is not, because the space is small and the measured rate makes it smaller.

**And the choice between hashing and encrypting is a choice about recovery.** A hashed credential can be verified and never read; an encrypted one can be read by whoever holds the encryption key, which is what makes it usable downstream and what makes the encryption key the real secret. **A store that keeps keys encrypted is a store whose backup contains all of them.**

### Part 5: distribution, and what the exposure actually is

**The exposure of a credential is not where it is supposed to be. It is everywhere it has been.**

The places measured or demonstrated earlier in this series, and the ones around them:

| Where | Why it happens |
|---|---|
| a query string | measured, it reaches the request line, the access log and the next request's `Referer` |
| a log line | whatever is logged gets read by more people than intended |
| an error trace | the value is in the frame's locals or the connection string |
| a command-line argument | it is in the process list and the shell history |
| an environment variable | it is in crash dumps and in anything that dumps its environment |
| a container image layer | a `RUN` step that used it leaves it in the layer even if the file is removed |
| CI output | a command that echoes, or a debug flag left on |
| a pasted message | the fastest way to hand a credential to a colleague |
| a screenshot | the fourth character of the same problem |

**And the reason a list is useful is that the controls are per-place rather than general**: a secret manager solves storage and not the command-line argument; a log filter solves logs and not the shell history. **The measurable version of this is Part 6, because one of those places keeps its contents longer than anyone expects.**

### Part 6: a secret deleted from a repository is still there

Measured, in a fresh repository: a key written into a file and committed, then the file changed to read the value from the environment and committed again.

```
working tree, searching for the value:      not found
git history, searching for the value:       2 hits
walking every commit and grepping:          5e5d7ea:config.py:API_KEY = "h-FVHTQP0P9owFKRMfKrbw5s"
total objects in the repository:            6
```

**The value is not in the working tree and it is fully present in the repository.** Every commit that ever contained it is still an object, and `git rev-list --all` plus a grep over each revision is a two-line recovery that the measurement performed.

> **Committing a secret once makes it permanent. Editing the file, deleting it, and even removing the commit from the branch all make it invisible by default rather than gone.**

**And the consequence is a procedure rather than a tool.** Rewriting history removes the object from *this* clone, and every other clone, fork, CI cache and artifact store keeps the copy it took. **So the correct response to a committed key is to revoke and reissue it**, and treat the history rewrite as a tidiness exercise that happens afterwards if at all.

**Which moves the control to before the commit.** A pre-commit scan is cheap, runs on the machine that has the value in its working tree, and stops the whole class — where every later measure is cleanup.

### Part 7: scanning, and what it can and cannot tell you

Measured, seven files scanned with four shape-based rules: three containing a credential-shaped value, and four containing something that looks the same and is not.

```
prod/app.py           AWS access key id    a generated value
prod/deploy.sh        JWT                  a generated value
prod/deploy.sh        Bearer token         the same value, second rule
prod/settings.py      Stripe live key      a generated value
docs/aws-example.md   AWS access key id    the vendor's published example key
docs/api.md           JWT                  a placeholder in documentation
docs/api.md           Bearer token         the same placeholder, second rule
tests/fixtures.py     Stripe live key      a test fixture
tests/test_auth.py    AWS access key id    a deliberately fake value
```

**Nine hits over three real credentials and four files that are not credentials.** And in the other direction, three values that were not reported because their shape was wrong:

| Value | Reported |
|---|---|
| a 32-character hexadecimal hash | not reported |
| a UUID | not reported |
| `TOKEN = "abc123"` | not reported |

**Both directions are the finding.**

> **A scanner matches shapes. A shape is necessary and not sufficient — so its output is a list to read, not a conclusion.**

**Which means the value of scanning is triage rather than certainty**: it takes a repository of ten thousand files and produces nine lines. **And it means a rule that fires on a variable name rather than on a value would have reported `abc123`** — the third row above is the case a name-based heuristic gets wrong in the direction of noise, while the opposite heuristic misses a key that was renamed.

**And scanning has a second use beyond the repository**: a scheduled scan for credentials that are still valid is what turns "we generated it once" into an inventory, and **a key nobody can remember creating is a key nobody will rotate.**

**And rotation is a design property rather than an operational habit.** A key that is referenced in forty places cannot be changed, so rotation has to be planned as an overlap: **the new key works, the old key still works, the users move, the old one is disabled** — and none of that is possible unless the credential's users are known and the revocation is a single action.

### Part 8: what follows for security

**The two kinds of credential need two different sets of controls**, and using the wrong set for either is the most common mistake in this area. **Measured, a slow function costs thirty thousand times a fast one per attempt**, which is exactly what a small space needs and exactly what a large one does not. **Rate limiting belongs with the first kind** — a 256-bit key does not need to be slowed down, but the endpoint that accepts one should still stop a script from trying a million of them.

**Length and generation decide whether a key can be searched at all**, and the measured table shows the transition happening over a handful of characters. **The practical statement is that there is no useful middle: either the key is short enough that the measured curve makes it a matter of hours, or it is long enough that the search is irrelevant and the only attacks are theft and reuse.**

**A recognisable format is a deliberate trade, not a mistake.** It helps operations, it enables a public-identifier design, and it makes the key findable by anyone scanning. **The response is not to make it unrecognisable but to make it rotatable** — because the measured repository case shows that a leaked key is a key to replace, not a key to hide.

**Storage has three legitimate options and the choice is a choice about recovery**: a slow hash for something a person chose, a fast hash for something random, and encryption only where the plaintext is genuinely needed downstream. **Each of them is a decision about what happens when the store is read by somebody who should not read it.**

**And the repository measurement generalises to every store with history.** A log with a thirty-day retention, an artifact store with indefinite retention, a CI system with build logs, a container registry with layers, and a backup system with monthly fulls — **each is a place where a deleted secret is not deleted**, and each has a retention setting that nobody chose.

**And the operational conclusion is about being able to change a key.** The measured exposure and the measured history both point the same way: **a credential that cannot be rotated is a credential whose leak cannot be fixed**, so the review question is not only where each secret is but how many places would have to change for it to be replaced.

### Detection and mitigation

- **Scan before the commit, not after.** Measured, a committed value stays in every clone; the pre-commit check is the only control that runs before that is true.
- **Run a scheduled scan that checks whether reported credentials are still valid.** A hit that no longer authenticates is noise, and the pipeline that tells the two apart is what makes the alerts readable.
- **Treat scanner output as triage.** Measured, nine hits covered three real credentials and four files that were not credentials, and three values were not reported at all.
- **Alert on credentials appearing in URLs, in `Referer` headers and in request lines.** That is a measurable property of traffic, and the earlier entry measured where those end up.
- **Track key age and last use.** A key older than the rotation policy, or unused for longer than a threshold, is the inventory question that makes rotation possible.
- **Watch for a single credential used from more than one place, or from more than one geography**, since that is what a shared key looks like when it is also somebody else's.
- **For mitigation, generate keys from a cryptographic random source with at least 128 bits of real entropy**, and prefer 256.
- **Use a public identifier in the key so the record can be found without storing the secret**, compare the secret in constant time, and keep only a hash of it.
- **Do not put a credential in a command-line argument, a query string or a log line.** Pass it in a header or an environment the process reads once, and configure the log to redact it.
- **Design for rotation from the first use**: an overlap window, a single action to disable the old key, and a record of every consumer — because a key that cannot be changed cannot be fixed.
- **Scope each key to the least it needs**, so that a leak is bounded by what that key could do rather than by what the account could do.
- **And when a key is found in history, revoke it first.** The rewrite is optional, and the earlier it is attempted the less it is worth.

<!-- lang:zh -->
### 两种凭据，两套相反的防护

这一片区域里几乎每一个错误，都来自把一种凭据当成另一种来对待。

> **口令是被猜的。密钥是被搜的（或者搜不动）。前者的防护是让每一次尝试变贵；后者的防护是让那个空间大到走不完。**

这里其他一切都是由这个划分推出来的：凭据怎么生成、静态怎么存、需不需要限速、以及"吊销"必须意味着什么。

下面在一台主机上实测，把数字给出来，好让推理看得见：一个短密钥真的被破出来、一张不同长度值多少的表、哈希与密钥派生函数的代价差、一个从仓库里恢复出来的密钥，以及一份扫描结果里它漏了什么、又凭空多出了什么。

### 第一部分：它的形状

| | 口令 | API 密钥或令牌 |
|---|---|---|
| 熵 | **低 —— 是人选的** | **高 —— 是生成的** |
| 攻击 | 猜，或者对着偷来的哈希离线跑 | 搜那个空间 |
| 防护 | **一个慢函数，加上限速** | **长度与随机性** |
| 静态存储 | 必须慢哈希 | 哈希；如果必须能还原，就加密 |
| 轮换 | 由用户做，很少做 | **由运维做，按计划** |
| 吊销 | 针对一个账号 | 针对一把密钥，不动其他 |

**而一个凭据有一个五阶段的寿命，每一阶段有它自己的失败方式**：生成、分发给需要它的人、存储、使用、以及最终轮换或吊销。**大多数事故在分发和存储这两步**，这就是为什么它们各自有下面的一部分。

### 第二部分：熵值多少

先测搜索本身的吞吐，在这台主机上：

```
紧凑循环里的 SHA-256                3,822,677 次/秒
同样的东西，放在破解循环里           2,473,336 次/秒
```

然后真的把一个从四位小写字母空间里抽出来的密钥破出来 —— 搜它的那个脚本并不知道它是谁：

```
试过的候选数:   74,557
用时:           不到一秒
恢复出来的:     那个四位密钥
```

**然后把实测速率套到更长的密钥上：**

| 密钥 | 候选数 | 按实测速率穷举需要 |
|---|---|---|
| 6 位小写字母 | 308,915,776 | **1.3 分钟** |
| 8 位小写字母 | 208,827,064,576 | **15.2 小时** |
| 8 位字母数字 | 218,340,105,584,896 | **2 年** |
| 16 位小写字母 | 43,608,742,899,428,874,059,776 | **3.61 亿年** |
| 16 字节随机（32 位 hex） | 3.4e38 | 2.8e24 年 |
| 32 字节随机 | 1.2e77 | 9.6e62 年 |

**前三行当一课读，其余当另一课读。** 六个字符是一分钟；八个字符是一天；八个字母数字是两年。**每多一个字符，代价就乘上字母表的大小** —— 这就是为什么"太短"与"够了"之间的差别是几个字符与一个大系数。

**而最后三行要当成实际的建议读。** 一旦一把密钥是从真正的随机源抽出来的、长度在 128 位以上，搜索就不再是攻击了 —— **攻击变成了在某个它被写下来的地方找到它**，那是第五、第六部分的主题。

**而这个速率是攻击者的，不是这台主机的。** 实测那个数字是一个单核上的 Python 循环；专用硬件的量级好得多，而成本就是一张显卡的成本。**给出实测数字的理由是展示那条曲线的形状，而留出余量设计的理由是那条曲线只朝一个方向动。**

**而生成器和长度一样要紧。** 一长串由时间戳、计数器和一个随机尾巴拼出来的东西，不是一长串随机的东西；**可搜索的部分就是来自可猜源的那部分**，无论它多长。

### 第三部分：一个前缀带来便利，不减去什么

密钥通常带着一个可辨认的形状发出去，而那些形状的实测值值得留着：

| 随机输入 | 得到的密钥 |
|---|---|
| 128 位 | 22 个 URL 安全字符 |
| 192 位 | 32 个字符 |
| 256 位 | 43 个字符 |

**像 `sk_live_` 这样的前缀一点都不增加熵** —— 它对每把密钥都一样 —— 而它做两件有用的事、以及一件值得点出来的事：

**它让运维能从一堆随机字符串里认出一把密钥**，在日志里、在粘贴的内容里、在工单里。
**它让这个格式能携带一个公开标识**，于是一次查找就能找到那条记录而不必存着秘密 —— `前缀 + 标识 + 秘密` 那个形状，其中只有最后一段必须是秘密。
**而它也让别人能从一堆随机字符串里认出一把密钥**，那正是扫描器在做的事。

> **一个可辨认的格式是一笔有意的取舍：它让操作它的人找得到它，也让其他看的人找得到它。**

**而由此推出的正确性要求很具体**：公开的那一段必须能定位到记录，秘密的那一段必须与一份存下来的哈希比较，**而那个比较必须是常数时间** —— 关于它的实测在认证那一篇里。

### 第四部分：存起来，两种凭据在这里分道

实测，在这台主机上存一个凭据的两种方式各要多少代价：

| 函数 | 每次调用 | 速率 |
|---|---|---|
| SHA-256 | **0.0003 ms** | 3,120,626 次/秒 |
| PBKDF2-HMAC-SHA256，1 次迭代 | 0.0054 ms | —— |
| PBKDF2-HMAC-SHA256，100,000 次迭代 | **10.7 ms** | 94 次/秒 |
| PBKDF2-HMAC-SHA256，600,000 次迭代 | **64.3 ms** | 16 次/秒 |

**两端之间差了大约三万倍**，而要点在于各自是给哪种凭据用的：

> **慢函数的存在是为了让一个小空间变贵。它不会把空间变大 —— 所以它保护的是口令，而对一把无论如何都搜不动的 256 位密钥，它没有必要。**

**由此得到一条容易说、也容易在两个方向上弄错的规则**：用 SHA-256 去哈希一把随机密钥是可以的，因为保护它的是那个空间；用 SHA-256 去存一个用户口令则不行，因为那个空间小，而实测那个速率让它更小。

**而哈希与加密之间的选择，是一个关于"能不能还原"的选择。** 一个被哈希的凭据可以被验证、永远读不出来；一个被加密的凭据可以被持有加密密钥的人读出来 —— 这正是它能被下游使用的原因，也正是加密密钥成为真正秘密的原因。**一个把密钥加密存起来的库，是一个备份里含着全部密钥的库。**

### 第五部分：分发，以及暴露面到底是什么

**一个凭据的暴露面，不是它应该在的地方，而是它去过的每一个地方。**

这个系列里前面实测或演示过的位置，以及它们周围的那些：

| 位置 | 为什么会发生 |
|---|---|
| 查询串 | 实测过：它到达请求行、访问日志、以及下一个请求的 `Referer` |
| 日志行 | 被记下来的东西会被比预想更多的人读到 |
| 错误堆栈 | 那个值在栈帧的局部变量里、或者在连接串里 |
| 命令行参数 | 它在进程列表里、也在 shell 历史里 |
| 环境变量 | 它在崩溃转储里、在转储自己环境的任何东西里 |
| 容器镜像层 | 一个用过它的 `RUN` 步骤把它留在层里，哪怕文件被删了 |
| CI 输出 | 一条 echo 的命令，或者一个忘了关的调试开关 |
| 粘贴的消息 | 把凭据交给同事最快的方式 |
| 一张截图 | 同一个问题的第四种形态 |

**而清单有用的原因，是控制是按位置来的、而不是通用的**：密钥管理器解决存储、解决不了命令行参数；日志过滤器解决日志、解决不了 shell 历史。**这件事可测量的版本在第六部分，因为那些位置里有一个保存内容的时间比任何人预期的都长。**

### 第六部分：从仓库里删掉的密钥还在

实测，在一个全新的仓库里：把一把密钥写进文件并提交，然后把文件改成从环境变量读、再提交一次。

```
工作区里搜那个值:        没找到
git 历史里搜那个值:      命中 2 次
遍历每一个提交再 grep:   5e5d7ea:config.py:API_KEY = "h-FVHTQP0P9owFKRMfKrbw5s"
仓库里的对象总数:        6
```

**那个值不在工作区里，而它完整地存在于仓库里。** 曾经包含它的每一个提交都还是一个对象，而 `git rev-list --all` 加一次对每个版本的 grep，就是这个测量执行的两行恢复。

> **把一个密钥提交一次，就让它永久存在。改那个文件、删掉它、甚至把这个提交从分支上移走，都只是让它默认看不见，而不是让它消失。**

**而后果是一套流程，而不是一个工具。** 改写历史会把那个对象从**这个**克隆里移走，而其他每一个克隆、fork、CI 缓存与产物仓库都留着它当时取走的那一份。**所以对一把被提交的密钥，正确的处置是吊销并重新签发**，而把改写历史当成之后（如果要做的话）的一次整洁工作。

**这就把控制点移到了提交之前。** 一个提交前扫描很便宜、就跑在那个工作区里有这个值的机器上、并且挡住整类问题 —— 而后面的每一个措施都是清理。

### 第七部分：扫描，以及它能说和不能说什么

实测，用四条形状态规则扫七个文件：三个装着凭据形状的值，四个装着看起来一样、其实不是的东西。

```
prod/app.py           AWS access key id    生成的值
prod/deploy.sh        JWT                  生成的值
prod/deploy.sh        Bearer token         同一个值，命中第二条规则
prod/settings.py      Stripe live key      生成的值
docs/aws-example.md   AWS access key id    厂商发布的示例密钥
docs/api.md           JWT                  文档里的占位符
docs/api.md           Bearer token         同一个占位符，命中第二条规则
tests/fixtures.py     Stripe live key      测试夹具
tests/test_auth.py    AWS access key id    故意的假值
```

**九条命中，覆盖三个真凭据和四个不是凭据的文件。** 而在另一个方向上，有三个值因为形状不对而没有被报出来：

| 值 | 是否报出 |
|---|---|
| 一个 32 字符的十六进制哈希 | 未报出 |
| 一个 UUID | 未报出 |
| `TOKEN = "abc123"` | 未报出 |

**两个方向都是那条发现。**

> **扫描器匹配的是形状。形状是必要条件、不是充分条件 —— 所以它的产出是一份要去读的清单，不是一个结论。**

**这意味着扫描的价值在于分诊而不是确定**：它把一万个文件的仓库变成九行。**而它也意味着一条按变量名而不是按值触发的规则会把 `abc123` 报出来** —— 上面第三行就是基于名字的启发式在噪声那个方向上会弄错的情形，而反向的启发式则会漏掉一把被改了名字的密钥。

**而扫描在仓库之外还有第二个用途**：定期扫一遍"仍然有效的凭据"，是把"我们当年生成过一次"变成一份清单的东西，而**一把没人记得创建过的密钥，就是一把没人会去轮换的密钥。**

**而轮换是一个设计属性，不是运维习惯。** 一把被四十个地方引用的密钥换不掉，所以轮换必须按一次重叠来计划：**新密钥能用、旧密钥也还能用、使用者迁过去、然后停掉旧的** —— 而只要那把凭据的使用者不清楚、吊销不是一次动作，这一切都做不到。

### 第八部分：从这些机制推出的安全观念

**两种凭据需要两套不同的控制**，而对任何一种用错另一套，是这片区域里最常见的错误。**实测，一个慢函数每次尝试比快函数贵三万倍**，那恰好是一个小空间需要的、也恰好是一个大空间不需要的。**限速属于第一种** —— 一把 256 位的密钥不需要被放慢，但接受它的那个端点仍然应该挡住一个脚本尝试一百万次。

**长度与生成方式决定了一把密钥到底能不能被搜**，而实测那张表显示这个转折发生在几个字符之内。**实用的说法是没有有用的中间地带：要么那把密钥短到实测那条曲线让搜索变成几小时的事，要么它长到搜索无关紧要、而唯一的攻击是窃取与复用。**

**一个可辨认的格式是一笔有意的取舍，不是一个错误。** 它帮助运维、它让"公开标识"这个设计成立、它也让它能被任何在扫描的人找到。**应对不是把它弄得无法辨认，而是让它可轮换** —— 因为实测那个仓库的情形表明：泄露的密钥是要被替换的，不是要被藏起来的。

**静态存储有三个正当选项，而选择是一个关于"能不能还原"的选择**：给人们自己选的东西用慢哈希、给随机的东西用快哈希、只有在明文确实被下游需要时才加密。**它们每一个都是一个决定：当那个库被一个不该读它的人读到时会发生什么。**

**而仓库那组实测可以推广到每一个有历史的存储。** 一份保留三十天的日志、一个无限期保留的产物仓库、一个有构建日志的 CI 系统、一个有镜像层的容器仓库、以及一个按月全备的备份系统 —— **每一个都是"被删掉的密钥并没有被删掉"的地方**，而每一个都有一个没人选过的保留期设置。

**而运维上的结论是关于"能不能换掉一把密钥"。** 实测到的暴露面与实测到的历史指向同一个方向：**一把换不掉的凭据，是一把泄露了也修不好的凭据**，所以评审的问题不只是每个秘密在哪里，而是"要替换它得改多少个地方"。

### 检测与缓解

- **在提交之前扫，而不是之后。** 实测，一个被提交的值留在每一个克隆里；提交前检查是唯一在那个事实成立之前运行的控制。
- **定期扫一遍，并检查被报出来的凭据是否还有效。** 一条已经不能认证的命中是噪声，而把两者区分开的流程才是让告警可读的东西。
- **把扫描输出当成分诊。** 实测，九条命中覆盖三个真凭据和四个不是凭据的文件，而另有三个值完全没被报出。
- **对凭据出现在 URL、`Referer` 头与请求行里告警。** 那是流量的一个可测属性，而前面那一篇实测过它们最终会到哪里。
- **追踪密钥的年龄与最近使用。** 一把比轮换策略更老的、或者长时间没被用过的密钥，就是那个让轮换变得可能的清单问题。
- **盯同一把凭据从不止一个地方、或者不止一个地理位置被使用**，因为那正是一把共享密钥在它同时也属于别人时的样子。
- **缓解上，从密码学随机源生成、至少 128 位真实熵，优先 256 位。**
- **在密钥里用一个公开标识，这样不存秘密也能定位记录**，用常数时间比较秘密，并且只存它的哈希。
- **不要把凭据放进命令行参数、查询串或日志行。** 把它放进一个头、或者一个进程只读一次的环境变量，并把日志配成会脱敏它。
- **从第一次使用就为轮换做设计**：一个重叠窗口、一次动作就能停掉旧密钥、以及一份每个使用者的记录 —— 因为一把换不掉的密钥修不好。
- **把每把密钥限定在它最少需要的作用域上**，这样一次泄露的边界是那把密钥能做的事，而不是那个账号能做的事。
- **而当一把密钥在历史里被发现时，先吊销它。** 改写历史是可选的，而越早尝试越不值。
