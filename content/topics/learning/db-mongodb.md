---
id: db-mongodb
title_en: MongoDB
title_zh: MongoDB
summary_en: The first database here whose queries are themselves documents — and that single property explains its injection class, why there is no parameterised query to fall back on, and why the fix is to stop input from becoming an operator. Measured on a live instance, including a login bypassed three different ways and server-side JavaScript used as a timing channel.
summary_zh: 这是这份指南里第一个"查询本身就是一个文档"的数据库 —— 而这一条性质就解释了它的注入类别、为什么它没有"参数化查询"可以退，以及为什么修法是阻止输入变成操作符。在一个真在跑的实例上实测，包括用三种方式绕过登录，以及用服务端 JavaScript 做时间侧信道。
tags: [beginner, database, mongodb, nosql, injection, json]
tools: [mongod, mongosh, pymongo]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Queries are documents

Every database so far in this series takes a query as text and data as values. MongoDB takes both as the same kind of thing.

> **A query is a document.** `{user: "admin", pass: "..."}` is a BSON object, and so is the input the application received.

That is the whole reason this entry exists. A relational engine separates the statement from its parameters, which is why parameterisation is a complete fix there. Here the query *is* data — so when input reaches the position of a value, and the parser let it become an object, **the input has changed the structure of the query rather than only its content**.

Measured below on a live **MongoDB 9.0.2** (community, standalone), started on a temporary data directory bound to loopback and removed afterwards.

### Part 1: the shape of it

| Relational | Here |
|---|---|
| database → table → row → column | database → **collection** → **document** → **field** |
| schema declared up front | **no schema — each document decides its own fields** |
| every row has the same columns | a field may be present in one document and missing in the next |
| values are typed by the column | **values carry their own BSON type** |
| primary key column | **`_id`, unique and indexed by default** |

Measured, three documents inserted into one collection with no schema at all:

```
{"name": "alice", "age": 30,   "tags": ["a","b"]}
{"name": "bob",   "email": "bob@example.com"}
{"name": "carol", "age": "31"}
```

**All three were accepted**, and the collection holds three documents with three different shapes. There is no "column count" to disagree about.

**And `_id` is the one field with a guarantee.** Measured, inserting a second document with the same `_id`:

```
DuplicateKeyError: E11000 duplicate key error collection: lab.dupe index: _id_ dup key: { _id: 1 }
```

**So the model is: the collection guarantees nothing about shape, and guarantees uniqueness only for `_id`.** Everything else an application needs — that `age` is a number, that `email` exists — is a convention, and nothing enforces it.

**Which makes "the field is missing" a normal state rather than an error.** That sounds harmless and is the subject of the next part.

### Part 2: absent and null are the same thing

Measured, against those three documents:

| Query | Matches |
|---|---|
| `{age: {$gt: 30}}` | **0** |
| `{age: "31"}` | **1** — the string |
| `{age: 31}` | **0** — the number |

**The first row is the surprise.** One document has `age` as the string `"31"`, and a numeric comparison does not match it: **there is no implicit conversion here**, so `$gt: 30` compares a number against a number and skips the text. That is the opposite of the behaviour measured in the relational entries, where `'1' = 1` is true on two engines and `'abc' + 1` yields a number on two of them.

**And the second surprise is stronger.** Measured:

| Query | Matches |
|---|---|
| `{email: null}` | **2** |
| `{email: {$exists: true}}` | **1** |

**Two documents match "email is null", and only one of them has an `email` field at all.** In BSON a missing field and a null field are indistinguishable to a query, so:

> **A query for null is a query for "null or absent", and there is no way to ask for one of them with a bare `null`.**

**The consequence for authorisation is direct.** A check shaped like "this document has no `ownerId`, so it belongs to nobody, so allow" is a check that also fires for `ownerId: null` — and a null written by an update path that meant to clear the field is a document whose ownership just changed meaning.

### Part 3: operators, and the injection they enable

Values in a query are not only values. A field can carry an operator document, and that is what makes the query language expressive:

| Operator | Means |
|---|---|
| `$eq`, `$ne` | equal, not equal |
| `$gt`, `$gte`, `$lt`, `$lte` | comparison |
| `$in`, `$nin` | in a set |
| `$exists` | field present |
| `$regex` | pattern match |
| `$or`, `$and`, `$not` | boolean combination |
| `$where`, `$function` | **run JavaScript** (Part 4) |

**And the login bypass follows from one sentence.** Measured, an application whose login is written the obvious way:

```python
db.users.find_one({"user": username, "pass": password})
```

| Password value supplied | Result |
|---|---|
| `"S3cret-Admin-Pass"` | **login succeeds** |
| `"wrong"` | login fails |
| **`{"$ne": None}`** | **login succeeds — and returns the admin's password** |
| **`{"$gt": ""}`** | **login succeeds** |
| **`{"$regex": ".*"}`** | **login succeeds** |

**Read the second and third rows together: a wrong string fails and a wrong object succeeds.** So the vulnerability is not about the content of the password at all — it is about its **type**. The application intended to compare a field against a value, and the input became a *condition* instead.

The mechanism in one line:

> **`{pass: {"$ne": null}}` does not mean "the password equals that object". It means "the password is not null."**

**And how the input becomes an object is the interesting part, because it is not exotic.** Any layer that parses input into structures and preserves the keys it was given will do it:
- a JSON request body (`{"password": {"$ne": null}}`) parsed as-is
- a query-string parser that expands brackets or dots into nesting
- a form decoder that treats a `$`-prefixed field name as a field name

**The application never intended to accept an object for a password.** It accepted whatever the body contained, because the body was already a document and so was the query.

### Part 4: server-side JavaScript

Four operators run JavaScript **on the server, per document**, and one of them is a query operator. Measured:

| Query | Result |
|---|---|
| `find({$where: "true"})` | 2 documents |
| `find({$where: "this.user === 'admin'"})` | 1 document |
| `find({$where: "this.pass.length > 100"})` | 0 documents |

**The script sees each document as `this` and decides whether it matches.** So a query supplied by a user is not evaluated solely by the database's query engine; it is evaluated by a JavaScript interpreter that the user's input reaches.

**And it is measurable as a timing channel:**

| Query | Time |
|---|---|
| `find({$where: "sleep(600) || true"})` | **2714 ms** |
| `find({$where: "function(){ sleep(600); return true; }"})` | **2704 ms** |
| the equivalent ordinary query | **1.0 ms** |

**A delay written into the query, executed once per document.** That is a blind-injection primitive: the response time carries information, and no data has to be returned for it to be observable.

**Two details worth keeping.**

`count_documents` **refuses** `$where` — measured, `$where is not allowed in this context`. The reason is that counting is implemented as an aggregation, and `$where` is not permitted inside one. **So whether a JavaScript-carrying query works depends on which API method the application used**, which makes it easy to test for and easy to miss.

**And the aggregation pipeline has its own JavaScript operator**, `$function`, which measured works inside a `$match`. Disabling `$where` somewhere is therefore not the same as removing server-side script execution.

### Part 5: indexes, and why the stage name is not enough

Measured on a collection of 50 000 documents, one field queried before and after an index existed:

| | Plan | Time |
|---|---|---|
| no index | `COLLSCAN`, 30 000 documents examined | **5.2 ms** |
| index on the field | `FETCH -> IXSCAN` | **0.8 ms** |

**And the part that is easy to get wrong — a regex.** Both of these plans report `IXSCAN`, and they are not remotely the same:

| Query (with an index on `s`, 30 000 documents) | Plan | **Keys examined** | Docs examined | Time |
|---|---|---|---|---|
| `/^user000042/` — anchored | `FETCH -> IXSCAN` | **2** | 1 | 0 ms |
| `/user000042/` — not anchored | `FETCH -> IXSCAN` | **30 000** | 1 | 10 ms |
| `"user000042@example.com"` — equality | `FETCH -> IXSCAN` | **1** | 1 | 0 ms |

**Same stage name, 15 000 times the work.** An anchor at the start of the pattern lets the engine seek into the index; without it the whole index is walked and every key tested. So the rule is:

> **An index turns a scan into a scan of the index, which is not the same as a bounded lookup.** Read `totalKeysExamined`, not the stage name.

**Which makes a user-supplied pattern a cost centre.** A filter feature that accepts a regex, applied to a large collection, is a query whose price the user chooses — and the unanchored case is the one people write by accident.

### Part 6: transactions, and where they do not exist

Measured, on a standalone instance:

```
start_transaction() -> OperationFailure: Transaction numbers are only allowed
                       on a replica set member or mongos
```

**The statement is accepted by the driver and refused by the server.** Multi-document transactions exist in this engine, but they require a replica set — the mechanism that carries them is the same one that replicates, so a standalone server has nowhere to keep the transaction's state.

**Three consequences, in order of how often they bite:**

**A single-document operation is always atomic.** An update to one document either happens or does not, regardless of deployment.

**A multi-document transaction works in production and fails in a standalone development instance** — the opposite of the usual direction, where development is permissive and production is strict.

**And "we wrapped it in a transaction" is therefore a claim about the deployment**, not about the code. Which is why the deployment topology belongs in the review.

### Part 7: doing it yourself

```bash
mongod --dbpath /tmp/lab --bind_ip 127.0.0.1 --port 27017 --logpath /tmp/lab.log --fork
```

```javascript
show dbs                                  // databases
db.getCollectionNames()
db.users.find({user: "admin"}).pretty()
db.users.explain("executionStats").find({user: "admin"})
db.users.getIndexes()
db.adminCommand({getParameter: 1, javascriptEnabled: 1})   // is server-side JS on
db.runCommand({connectionStatus: 1})                       // who am I, per the server
```

**`explain("executionStats")` is the habit to form**, and Part 5 is why: the plan's shape, the documents examined and the keys examined together are what tell you whether an index is being used or merely present.

**And check what the server thinks of your session.** `connectionStatus` returns the authenticated users; on an instance with no authentication configured it returns an empty list and the connection is nonetheless fully privileged. That is the whole of Part 8's first point in one command.

### Part 8: what follows for security

**Injection here is the same idea as SQL injection with a different mechanism.** In a relational engine the input alters the *text* of a statement, and the fix removes the input from the text. Here the input alters the *structure* of a document, and there is no equivalent split to fall back on — **the query is already data.**

**Which means the fix cannot be "use parameters".** The measured three-way bypass is closed by making sure input can never occupy the position of an operator:

```javascript
// what the application intended, made explicit
db.users.findOne({"user": {"$eq": username}, "pass": {"$eq": password}})
```

**Wrapping the input in `$eq` forces the value to be treated as a value.** Combined with a parser that rejects `$`-prefixed keys in user input and refuses to expand bracket or dot notation from request bodies, the class is closed at both ends. **Leaving either half out is a bypass**, because a top-level filter is not the only place a document can carry an operator.

**Server-side JavaScript is code execution with the database's privileges.** Measured, a query carried a `sleep` and the response time reported it. The control is to turn scripting off — this engine has a setting for it — and to treat any use of `$where`, `$function` or `mapReduce` as code deployment rather than as a query.

**`null` meaning "null or absent" is a logic trap for authorisation.** A rule that says "no owner means public" also fires for `ownerId: null`, so a field cleared by an unrelated update path can change who may read a document. The fix is to make the states distinguishable — an explicit sentinel, or a separate boolean — so that "absent" is not carrying a decision.

**There is no schema, so injection can write a document of any shape.** In a relational engine an injection that writes has to satisfy the columns; here it can add fields the application will later read as though it had written them. **Documents written by an attacker are indistinguishable from legitimate ones unless the application checks the shape on the way out as well as on the way in.**

**And authentication is off until it is turned on.** Measured, a fresh instance reports zero accounts, and the connection used for all of the above needed no credentials. Combined with the default bind address — loopback, which is a sensible default — the failure mode is specific and historical: **an instance reachable from a network with no accounts on it.** In a lab instance created for one measurement that is fine; it is also the shape of the finding that put many deployments on the news, which is why the review questions are the same two every time: **what address is it listening on, and does it have any accounts.**

### Detection and mitigation

- **Check authentication and the bind address together.** Zero accounts on an instance that is not restricted to loopback is the finding; either half alone is a configuration choice.
- **Log and alert on `$where`, `$function` and `mapReduce` in application queries.** As a set they are the server-side script surface, and an application that never intends to use them should never produce them.
- **Watch for `$`-prefixed keys and dotted or bracketed field names arriving in request bodies.** They are the raw material of the bypass in Part 3, and they appear in the request rather than in the query.
- **Sample `explain` output in production and look at `totalKeysExamined`.** A query whose stage is an index scan but which examines the whole index is the shape in Part 5, and it is invisible in the response time until the collection grows.
- **Alert on queries with unbounded `$regex` against large collections**, and cap pattern length where a filter accepts one.
- **For mitigation, wrap user input in `$eq` at every position a value can go**, and reject `$`-prefixed keys in user input rather than escaping them.
- **Turn server-side scripting off** unless something genuinely needs it, and audit for it before and after, because the aggregation pipeline carries its own JavaScript operator.
- **Enable authentication, and create the application's account with the narrowest role that works.** Then review the roles: the built-in read and read-write roles are per-database, which is closer to least privilege than the administrative ones.
- **Run a replica set even for small deployments** if the code uses multi-document transactions, because the failure is silent in development and only appears on the deployment that lacks one.
- **Make "absent" and "null" distinguishable wherever the difference decides something.** A field whose absence grants access is a field that must not be nullable by accident.
- **Validate the shape of documents on read as well as on write.** With no schema, a document that appeared through another path — a migration, a restore, an injection — is read by the same code as one the application wrote.
- **And keep the database off the public network regardless of authentication.** The measured instance was one flag away from being reachable, and the default was the only thing preventing it.

<!-- lang:zh -->
### 查询就是文档

这个系列到目前每一个数据库，查询都是文本、数据都是值。MongoDB 把两者当成了同一种东西。

> **一个查询就是一个文档。** `{user: "admin", pass: "..."}` 是一个 BSON 对象，而应用收到的输入也是。

这就是这一篇存在的全部理由。关系引擎把语句与它的参数分开，所以在那里面参数化是一个完整的修法。而在这里**查询本身就是数据** —— 于是当输入到达值的位置、而解析层又让它变成了对象时，**输入改变的是查询的结构，而不只是它的内容**。

下面在一个真在跑的 **MongoDB 9.0.2**（社区版、单机）上实测：临时数据目录、只绑回环，跑完删掉。

### 第一部分：它的形状

| 关系型 | 这里 |
|---|---|
| 数据库 → 表 → 行 → 列 | 数据库 → **集合** → **文档** → **字段** |
| schema 事先声明 | **没有 schema —— 每个文档自己决定有哪些字段** |
| 每一行有同样的列 | 一个字段可能在某个文档里有、下一个文档里没有 |
| 值由列决定类型 | **值带着自己的 BSON 类型** |
| 主键列 | **`_id`，默认唯一且已建索引** |

实测，往一个完全没有 schema 的集合里插三个文档：

```
{"name": "alice", "age": 30,   "tags": ["a","b"]}
{"name": "bob",   "email": "bob@example.com"}
{"name": "carol", "age": "31"}
```

**三个都被接受**，集合里是三个形状各不相同的文档。这里没有"列数不一致"这回事可以争。

**而 `_id` 是唯一有保证的字段。** 实测，插入第二个相同 `_id` 的文档：

```
DuplicateKeyError: E11000 duplicate key error collection: lab.dupe index: _id_ dup key: { _id: 1 }
```

**所以这个模型是：集合对形状不做任何保证，只对 `_id` 保证唯一。** 应用需要的一切其他东西 —— `age` 是数字、`email` 存在 —— 都是约定，没有东西在强制它。

**而这就让"字段不存在"成了一个正常状态，而不是一个错误。** 这听起来无害，而它就是下一部分的主题。

### 第二部分：不存在与 null 是同一件事

实测，对着上面那三个文档：

| 查询 | 匹配 |
|---|---|
| `{age: {$gt: 30}}` | **0** |
| `{age: "31"}` | **1** —— 那个字符串 |
| `{age: 31}` | **0** —— 那个数字 |

**第一行是意外。** 有一个文档的 `age` 是字符串 `"31"`，而一次数值比较不会匹配它：**这里没有隐式转换**，所以 `$gt: 30` 是拿数字和数字比、跳过了文本。那与前面关系型那几篇实测到的行为相反 —— 那里 `'1' = 1` 在两个引擎上为真，`'abc' + 1` 在其中两个上还能算出个数。

**而第二个意外更强。** 实测：

| 查询 | 匹配 |
|---|---|
| `{email: null}` | **2** |
| `{email: {$exists: true}}` | **1** |

**有两个文档匹配"email 是 null"，而其中只有一个真的有 `email` 字段。** 在 BSON 里，一个字段缺失和一个字段为 null，对查询来说不可区分，所以：

> **一次对 null 的查询，就是一次"null 或者不存在"的查询，而用裸 `null` 没法只要其中一个。**

**对授权的后果是直接的。** 一个写成"这个文档没有 `ownerId`，所以它不属于任何人，所以放行"的检查，也会对 `ownerId: null` 成立 —— 而一个由某条本意是"清空该字段"的更新路径写下的 null，就是一个归属含义刚被改掉的文档。

### 第三部分：操作符，以及它们带来的注入

查询里的值不只是值。一个字段可以带一个操作符文档，而这就是这门查询语言表达力的来源：

| 操作符 | 含义 |
|---|---|
| `$eq`、`$ne` | 等于、不等于 |
| `$gt`、`$gte`、`$lt`、`$lte` | 比较 |
| `$in`、`$nin` | 在集合中 |
| `$exists` | 字段存在 |
| `$regex` | 模式匹配 |
| `$or`、`$and`、`$not` | 布尔组合 |
| `$where`、`$function` | **执行 JavaScript**（第四部分） |

**而登录绕过就是从一句话推出来的。** 实测，一个按最显然的写法写的登录：

```python
db.users.find_one({"user": username, "pass": password})
```

| 传进来的口令值 | 结果 |
|---|---|
| `"S3cret-Admin-Pass"` | **登录成功** |
| `"wrong"` | 登录失败 |
| **`{"$ne": None}`** | **登录成功 —— 并且把管理员的密码也返回了** |
| **`{"$gt": ""}`** | **登录成功** |
| **`{"$regex": ".*"}`** | **登录成功** |

**把第二行和第三行放在一起读：一个错误的口令字符串失败，而一个错误的对象成功。** 所以这个漏洞与口令的**内容**毫无关系 —— 它关于口令的**类型**。应用本意是拿一个字段与一个值比较，而输入变成了一个**条件**。

机制一句话：

> **`{pass: {"$ne": null}}` 不是"口令等于那个对象"，而是"口令不等于 null"。**

**而输入怎么变成一个对象的，才是有意思的部分，因为它一点都不稀奇。** 任何一层把输入解析成结构、又原样保留它收到的键的东西都会这样：
- 一个 JSON 请求体（`{"password": {"$ne": null}}`）被原样解析
- 一个把方括号或点号展开成嵌套的查询串解析器
- 一个把 `$` 开头的字段名当成字段名的表单解码器

**应用从来没有打算接受一个对象作为口令。** 它接受了请求体里的任何东西，因为请求体本来就是一个文档，而查询也是。

### 第四部分：服务端 JavaScript

有四个操作符**在服务端、对每个文档**执行 JavaScript，而其中一个是查询操作符。实测：

| 查询 | 结果 |
|---|---|
| `find({$where: "true"})` | 2 个文档 |
| `find({$where: "this.user === 'admin'"})` | 1 个文档 |
| `find({$where: "this.pass.length > 100"})` | 0 个文档 |

**这段脚本把每个文档看作 `this`，并决定它是否匹配。** 所以一段由用户提供的查询，不是只由数据库的查询引擎来求值；它由一个 JavaScript 解释器来求值，而用户的输入能到达那个解释器。

**而它可以作为一个时间侧信道被测量出来：**

| 查询 | 用时 |
|---|---|
| `find({$where: "sleep(600) || true"})` | **2714 ms** |
| `find({$where: "function(){ sleep(600); return true; }"})` | **2704 ms** |
| 等价的普通查询 | **1.0 ms** |

**一个写进查询里的延迟，每个文档执行一次。** 那是一个盲注原语：响应时间里带着信息，而要让这件事可观测，不需要返回任何数据。

**两个值得记住的细节。**

`count_documents` **拒绝** `$where` —— 实测报 `$where is not allowed in this context`。原因是计数是用聚合实现的，而里面不允许 `$where`。**所以一个带 JavaScript 的查询能不能跑，取决于应用用了哪个 API 方法** —— 这让它既好测、也容易漏。

**而聚合管道有它自己的 JavaScript 操作符**，`$function`，实测在 `$match` 里可用。所以在某处关掉 `$where`，并不等于移除了服务端的脚本执行。

### 第五部分：索引，以及为什么光看阶段名不够

实测，在一个五万文档的集合上，同一个字段在有没有索引的两种情况下：

| | 计划 | 用时 |
|---|---|---|
| 没有索引 | `COLLSCAN`，扫了 30 000 个文档 | **5.2 ms** |
| 在该字段上有索引 | `FETCH -> IXSCAN` | **0.8 ms** |

**而容易看错的那一处 —— 正则。** 下面这两个计划都报 `IXSCAN`，而它们完全不是一回事：

| 查询（`s` 上有索引，30 000 个文档） | 计划 | **扫描的键** | 扫描的文档 | 用时 |
|---|---|---|---|---|
| `/^user000042/` —— 锚定 | `FETCH -> IXSCAN` | **2** | 1 | 0 ms |
| `/user000042/` —— 不锚定 | `FETCH -> IXSCAN` | **30 000** | 1 | 10 ms |
| `"user000042@example.com"` —— 等值 | `FETCH -> IXSCAN` | **1** | 1 | 0 ms |

**同样的阶段名，一万五千倍的工作量。** 模式开头的一个锚点让引擎能跳进索引；没有它，整个索引被走一遍、每个键都被试一次。所以规则是：

> **索引把一次扫描变成一次"对索引的扫描"，而那不等于一次有边界的查找。** 要看的是 `totalKeysExamined`，不是阶段名。

**这就让一个用户提供的模式变成一个成本中心。** 一个接受正则的过滤功能，用在一个大集合上，就是一条价格由用户决定的查询 —— 而不锚定那种，恰恰是人们不小心写出来的那种。

### 第六部分：事务，以及它在哪里不存在

实测，在一个单机实例上：

```
start_transaction() -> OperationFailure: Transaction numbers are only allowed
                       on a replica set member or mongos
```

**这条语句被驱动接受了，被服务端拒绝了。** 这个引擎里有多文档事务，但它们需要副本集 —— 承载它们的机制和复制是同一个，所以一个单机服务端没有地方存放那个事务的状态。

**三个后果，按被咬到的频率排：**

**单文档操作永远是原子的。** 对一个文档的更新要么发生、要么不发生，与部署形态无关。

**多文档事务在生产上能用、在单机开发实例上失败** —— 与通常的方向相反，通常是开发环境宽松、生产严格。

**所以"我们包在一个事务里了"是一句关于部署的话**，不是关于代码的话。这就是为什么部署拓扑属于评审范围。

### 第七部分：自己动手做一次

```bash
mongod --dbpath /tmp/lab --bind_ip 127.0.0.1 --port 27017 --logpath /tmp/lab.log --fork
```

```javascript
show dbs                                  // 有哪些数据库
db.getCollectionNames()
db.users.find({user: "admin"}).pretty()
db.users.explain("executionStats").find({user: "admin"})
db.users.getIndexes()
db.adminCommand({getParameter: 1, javascriptEnabled: 1})   // 服务端 JS 开着吗
db.runCommand({connectionStatus: 1})                       // 服务端认为我是谁
```

**`explain("executionStats")` 是该养成的习惯**，第五部分就是理由：计划的形状、扫描的文档数与扫描的键数放在一起，才能告诉你一个索引是被用上了、还是只是建在那里。

**而且要看看服务端怎么看你的这个会话。** `connectionStatus` 会返回已认证的用户；在一个没配置认证的实例上它返回一个空列表，而那个连接照样是全权的。那就是第八部分第一点在一条命令里的样子。

### 第八部分：从这些机制推出的安全观念

**这里的注入与 SQL 注入是同一个想法，机制不同。** 在关系引擎里，输入改变的是语句的**文本**，而修法是把输入从文本里移出去。在这里，输入改变的是一个文档的**结构**，而没有一层等价的切分可以退回去 —— **查询本来就已经是数据。**

**所以修法不可能是"用参数"。** 那条三种方式的绕过，是靠确保输入永远不会占据操作符的位置来关掉的：

```javascript
// 把应用本来想做的事写明确
db.users.findOne({"user": {"$eq": username}, "pass": {"$eq": password}})
```

**把输入包进 `$eq`，就强制它被当作一个值。** 再叠上一个拒绝用户输入里 `$` 开头键、并拒绝从请求体展开方括号/点号记法的解析层，这一类就在两端都关上了。**少做任何一半都是一条绕过**，因为顶层过滤器不是文档里唯一能带操作符的位置。

**服务端 JavaScript 是以数据库权限进行的代码执行。** 实测，一条查询带上了一个 `sleep`，而响应时间把它报了出来。控制手段是关掉脚本 —— 这个引擎有对应的设置 —— 并把任何使用 `$where`、`$function` 或 `mapReduce` 的地方当成一次代码部署，而不是一条查询。

**`null` 意味着"null 或者不存在"，这是授权上的一个逻辑陷阱。** 一条说"没有属主就是公开"的规则也会对 `ownerId: null` 成立，所以一个被无关更新路径清空的字段，可以改变谁能读一个文档。修法是让这些状态可区分 —— 一个显式的哨兵值，或者一个单独的布尔 —— 这样"缺席"就不在承载一个决定。

**没有 schema，所以注入可以写下一个任意形状的文档。** 在关系引擎里，一次写数据的注入必须满足那些列；在这里它可以加上一些字段，而应用之后会像读自己写的东西一样去读它们。**除非应用在写进去和读出来两头都检查形状，否则由攻击者写下的文档与合法的文档不可区分。**

**而认证在你去打开它之前一直是关的。** 实测，一个全新的实例报告零个账号，而上面这一切所用的那个连接不需要任何凭据。再叠上默认的监听地址 —— 回环，那是一个合理的默认 —— 失效的形态就很具体、也很有历史：**一个能从网络够到、而上面没有任何账号的实例。** 在一个为单次测量而建的实验实例上这没问题；它同时也是把很多部署送上新闻的那条发现的形状，这就是为什么评审问题每次都是同样两个：**它在哪个地址上监听，以及它上面有没有账号。**

### 检测与缓解

- **把认证与监听地址放在一起查。** 一个没有被限制在回环上的实例上有零个账号，就是那条发现；只看任何一半，都只是一个配置选择。
- **对应用查询里出现的 `$where`、`$function`、`mapReduce` 记录并告警。** 它们作为一组就是服务端脚本面，而一个从不打算使用它们的应用永远不该产出它们。
- **盯请求体里到来的 `$` 开头键、以及带点号或方括号的字段名。** 它们是第三部分那条绕过的原料，而且它们出现在请求里，不在查询里。
- **在生产上抽样 `explain` 输出，看 `totalKeysExamined`。** 一个阶段是索引扫描、却扫了整个索引的查询，就是第五部分那个形状，而在响应时间上它直到集合长大之前都是看不见的。
- **对大集合上的无边界 `$regex` 告警**，并在接受模式的功能上给模式长度设上限。
- **缓解上，在每一个值可以出现的位置把用户输入包进 `$eq`**，并**拒绝**用户输入里 `$` 开头的键，而不是去转义它们。
- **除非确实有东西需要，否则关掉服务端脚本**，并且前后都要审计，因为聚合管道带着它自己的 JavaScript 操作符。
- **打开认证，并用能干活的最窄角色建应用账号。** 然后审查那些角色：内建的读与读写角色是按数据库的，那比管理类角色更接近最小权限。
- **代码用了多文档事务的话，小部署也跑副本集**，因为这个失败在开发环境上是静默的、只在缺副本集的那个部署上才出现。
- **在"缺席"与"null"的差别会决定什么的地方，让两者可区分。** 一个"缺席即授权"的字段，就是一个绝不能碰巧可空的字段。
- **读的时候也要校验文档形状，不只是写的时候。** 没有 schema，一个从别的路径 —— 一次迁移、一次恢复、一次注入 —— 出现的文档，会被同一段代码当作应用自己写的来读。
- **而且不管有没有认证，都别把数据库放在公网上。** 实测那个实例离"能被够到"只差一个参数，而挡住它的只是那个默认值。
