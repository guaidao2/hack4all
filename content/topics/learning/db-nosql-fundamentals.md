---
id: db-nosql-fundamentals
title_en: NoSQL
title_zh: NoSQL 入门
summary_en: NoSQL names something the databases do not do rather than something they share, and the four families differ from each other more than they differ from a relational one. Measured — what the two data models cost, and what a real key-value store does with a lookup it was not designed for.
summary_zh: NoSQL 说的是这些数据库"不做什么"，而不是它们共有的是什么，而四类之间的差别比它们与关系库的差别还大。这一篇实测两种数据模型的代价，以及一个真的键值库面对"不是为它设计的查询"时会怎样。
tags: [beginner, database, nosql, mongodb, redis, keyvalue, document]
tools: [redis-cli, mongosh, python3, sqlite3]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A name that describes what it does not do

"NoSQL" is a negation, and negations make poor categories. What the databases gathered under it share is **the absence of the relational model** — and the four families inside that group have less in common with each other than any of them has with a relational database.

> **NoSQL says what these stores do not do.** Choosing one means choosing which guarantees to give up, and knowing what replaces them — usually the application.

| Family | Examples | Data model | Reaches for |
|---|---|---|---|
| **Document** | MongoDB, CouchDB, Firestore | JSON-like documents | one aggregate read at a time |
| **Key-value** | Redis, Memcached, etcd | key, and an opaque value | speed on a key, counters, caches |
| **Wide-column** | Cassandra, HBase, Bigtable | rows with sparse columns, a partition key | very high write volume |
| **Graph** | Neo4j, JanusGraph | nodes and edges as first-class | traversal of relationships |

The measurements below use SQLite to hold both data models side by side — normalised tables against a JSON column — because the trade-offs are properties of the models rather than of any one product. The key-value section uses a **real Redis 8.0.6**, started on a temporary port, bound to the loopback address, with persistence disabled, and shut down at the end.

### Part 1: what the relational model was giving you

The reason to look at the relational model first is that leaving it means leaving specific things behind, and each of them was doing work:

| It provided | What happens without it |
|---|---|
| a declared schema | the shape of the data lives in the writing code |
| referential integrity | the application maintains the links |
| joins computed at query time | the access path is decided when the data is written |
| transactions across rows and tables | atomicity shrinks to one document or one key |
| ad-hoc queries | queries are served from indexes designed in advance |

**None of those is a bug in a NoSQL store.** Each was traded for something: flexible shapes, horizontal scale, write throughput, or a data model that matches a particular problem. But the trade is only a good one when the thing given up is not needed — and the cheapest way to be wrong about that is to believe that "schema-less" means "no constraints to maintain".

### Part 2: the data model — aggregate instead of table

A relational schema splits data by kind and reassembles it with joins. A document store keeps together what is read together. Measured, reading 500 complete orders — five line items each — from one process:

| Layout | Rows | Time to read 500 orders | Queries issued |
|---|---|---|---|
| **normalised** — `orders` + `order_items` | 50,000 + 250,000 | **2355 ms** | **1000** (two per order) |
| **embedded** — one JSON document per order | 50,000 | **0.57 ms** | **500** (one per order) |

**Both the model and the number of round trips differ here**, and the second is the larger factor: the normalised version issues two queries per order, the document version one. **That is exactly what the two models are about.** The relational version has to touch two tables because the data was split by kind; the document version reads one record because the data was stored the way it is used.

**The cost appears on the other side.** In that document, the customer name is stored in every one of the 50,000 orders, while the normalised version stores it once and references it. Changing a customer's name is one `UPDATE` in the first layout and 50,000 documents in the second.

**The thinking unit is different.** A relational table is a set of rows of one kind; a document is an **aggregate** that is read and written as a unit. That decision pre-determines what can be atomic, which is the next part.

### Part 3: who guarantees the shape

"Schema-less" is a misnomer. The structure still exists — it simply lives in the code that writes, rather than in the database. Measured, on the same missing field:

| Statement | Result |
|---|---|
| `INSERT INTO strict_t (id, customer) VALUES (1, 'alice')` with `amount NOT NULL` | **refused** — `NOT NULL constraint failed: strict_t.amount` |
| the same data as a document without an `amount` field | **accepted** — stored as `{"customer": "alice"}` |
| then counting rows where the amount is above zero | **0 rows** — the record is invisible to that query |

**The record exists.** It is simply not findable by the query that was supposed to concern it, and nothing failed at the moment it was written. A `NOT NULL` in a relational schema is a **guarantee enforced by the database**; the absence of the field in a document is a **mistake enforced by nobody**.

**The correction exists and is optional.** MongoDB's `$jsonSchema` validator, or a schema on a collection, does exactly what `NOT NULL` does — and being optional, it is off unless someone turns it on. That is the shape of the whole shift: **validation moves from something the database must do to something the application should do**, and the move is invisible in a review that looks at the database.

### Part 4: querying moves into the model

In a relational database, a query can filter and join on anything, and the planner works out how. In the other families, the access path is something you declare:

| Engine | Plan for finding one customer's orders |
|---|---|
| a plain column, no index | `SCAN orders` — every row |
| the same column, indexed | `SEARCH orders USING INDEX idx_cust` |
| a field **inside a JSON document** | `SCAN orders_doc` — every document, parsed |

Measured time for the identical result set: **33.19 ms searching inside the documents against 0.05 ms on the indexed column.**

**The difference is not that documents are slow.** It is that **the field being searched is not a column**, so there is no sorted structure to consult — the value has to be extracted from every document before it can be compared. Document stores solve this the same way they solve everything else here: you **declare an index** for the fields you intend to query (`createIndex` in MongoDB), which means deciding in advance what will be asked.

**And that is what "modelling for queries" means.** In a relational database you model the data and ask anything later; here the questions are part of the design, and changing one later is a migration rather than a query.

### Part 5: a key-value store, measured

The model is one step further: **only the key has structure**, and the value is opaque to the store. Holding 100,000 rows in a key-value shape inside SQLite shows the asymmetry, and a real Redis shows the same thing without the relational layer:

| Lookup | SQLite as a key-value table | Redis 8.0.6 |
|---|---|---|
| by key | **0.10 ms** — `SEARCH ... USING INDEX` | **0.12 ms** |
| by a field **inside the value** | **30.33 ms** — `SCAN` | **2222 ms** — every key fetched and read |

Writing the 50,000 keys into Redis one command at a time took **2298 ms**, which is the round-trip cost rather than the store's: each `SET` is a request and a reply.

**The asymmetry is the model, not the implementation.** A key is what the store maintains an index over; a value is bytes it never interprets. So any question that is not "what is at this key" has to be answered by maintaining **another structure alongside** — a secondary index, a sorted set, a set of ids — and keeping it in step with the data. Every key-value design that answers a second question is doing that, whether or not it says so.

**The values can hold structure.** Redis hashes, lists, sorted sets, streams are structures inside a single key — and they are useful precisely to the holder of that key:

```
HSET user:1 name alice role admin    -> a hash under one key
LPUSH queue:jobs a b c               -> a list under one key
ZADD board 100 alice 250 bob         -> a sorted set under one key
```

**Structure within a value is a private matter between the application and that key.** Nothing in the store can join two keys, so relationships between them are the application's to maintain — which is the same statement as Part 1, in a different family.

**And one thing this family has that relational engines generally do not**, at least not as a native concept:

```
SET session:abc value EX 60   ->   TTL 60
SET nottl value               ->   TTL -1   (no expiry)
```

**Expiry is enforced by the storage layer**, not by a cleanup job in the application. For session data that is a meaningful difference: "this stops being valid at this time" becomes a property of where the data lives rather than a property of the code that reads it.

### Part 6: atomicity, and what a transaction is not

Every family has an atomic unit, and it is smaller than a relational transaction. Measured on the same Redis connection, a group of commands where the third is invalid against the type of the key:

```
MULTI                          -> OK
SET a 1                        -> QUEUED
LPUSH a x                      -> QUEUED      (a holds a string)
SET b 2                        -> QUEUED
EXEC                           -> ['OK', 'ERR WRONGTYPE ...', 'OK']
a = 1     b = 2
```

**The middle command failed and the other two took effect.** `MULTI` guarantees that **no other client's commands are interleaved** with the group; it does not guarantee that the group either all happens or none does. A command that is invalid against the key's type is discovered when it runs, and the rest of the queue proceeds.

**So "I wrapped it in MULTI" is not "I have a transaction"**, and the same caution applies to any grouping primitive: what it excludes, and what it does not undo, are two different questions.

**In document stores the atomic unit is the document.** A multi-document transaction exists in current versions and carries a cost, which is why the design guidance is to put things that must change together **into one document**. That is not a workaround; it is what the aggregate in Part 2 is for.

### Part 7: eventual consistency, and the window it opens

Replication across nodes is where the guarantee changes shape. Reading from a replica means a write acknowledged by the primary may not be visible for a while, so the same read gives different answers to different readers:

| Anomaly | What a reader sees |
|---|---|
| read-your-writes violated | your own change is not there yet |
| monotonic reads violated | a value goes backwards between two reads |
| causality violated | a reply appears before the message it replies to |

**The security reading of that window is concrete.** Where a permission lives in a replicated store — a role, a session, a token's revocation — **a change takes effect on each replica when it gets there**. For a revocation, that means the credential still works, on some nodes, for the length of the window.

That does not make replication wrong; it makes the window something to **state and budget** rather than assume away. A revocation that propagates in under a second is a different design from one that propagates in thirty.

### Part 8: what follows for security

**Validation moved, so the injection surface moved with it.** The relational family's characteristic failure is a string built by concatenation meeting a parser. The document family's characteristic failure is a **type** meeting a query: where a filter accepts either a value or an operator, sending the operator changes the question.

```
{ "user": "alice" }              a value: match this
{ "user": { "$ne": null } }      an operator: match anything that is not null
```

**If the code passes a request body into a query without deciding what it is, the client chooses whether it sent a name or a condition** — which is the NoSQL entry's finding, arriving through the same door as mass assignment.

**And the capability boundary is the attack boundary.** Relational injection reaches `UNION`, stacked statements, file reads and writes where the engine and account permit. The other families have their own primitives with the same property:

| Primitive | Where it exists |
|---|---|
| `$where` and server-side JavaScript | MongoDB, where it is enabled |
| `EVAL` / Lua scripts | Redis |
| aggregation pipelines with `$lookup` | MongoDB — joins, per query |
| `KEYS`, `SCAN`, `CONFIG` | Redis — enumeration, and configuration from a client |

**The rule is the same in every family: what the query language can express is what an injection can express.** Which makes "which operators and commands does this deployment allow" a security question rather than a matter of taste.

**Defaults are the other face of it.** A document store with no validator accepts anything; a key-value store with no authentication and a public bind accepts connections from anywhere. **Neither is a vulnerability in the product** — both are defaults that suit a development machine, and both have been the whole finding in real incidents.

**And the final consequence is about where a guarantee is enforced.** A relational database enforces a great deal itself: types, uniqueness, referential integrity, the atomicity of a transaction. The other families move much of that into the application — which means **an audit that reads database configuration and schemas will find less**, and the same question has to be answered by reading application code.

### Detection and mitigation

- **Log the command or operation, not just the connection, for the key-value family.** `KEYS`, `SCAN` over the whole keyspace, `CONFIG`, `EVAL`, `FLUSHALL` — these are not things an ordinary application issues in production, and each is cheap to alert on individually.
- **Watch for operator-shaped input where a value is expected.** A JSON body arriving as `{"$ne": null}` in a field that should hold a string is the signature of the class, and it can be detected at the application boundary before it reaches a query.
- **Alert on documents or keys whose size and count move sharply.** Since a document is the atomic unit, a document that keeps growing is both a performance problem and a sign that something is appending rather than updating.
- **And measure replication lag, because it is the length of a revocation window.** The metric is not only operational: it is the time during which a disabled account may still be enabled somewhere.
- **For mitigation, turn on the validation the store offers.** A schema validator on a collection, a type check before a filter is built, a whitelist of the fields a client may set — the store's own mechanism is cheaper than a review that hopes the application checked.
- **Disable the primitives you do not use.** Server-side JavaScript, client-submitted scripts, and administrative commands reachable from an application connection are capabilities granted to anyone who can reach that connection. If nothing depends on them, turning them off costs nothing.
- **Authentication and binding are the first two settings to look at**, and they are the same two for every family: who may connect, and from where.
- **Keep the application's account minimal, and note that the ceiling differs by family.** A read-only relational account cannot write files; a key-value account with `CONFIG` can change the server's behaviour at runtime.
- **Where an application needs a guarantee the store does not provide, put it back explicitly.** Cross-document invariants, secondary indexes for the second question, and an application-level transaction where the store's atomic unit is too small — each of those is a place where the relational engine used to do the work for free.
- **And write the consistency window down as a number.** If a permission change takes up to thirty seconds to reach every reader, then thirty seconds is part of the system's behaviour, and a design that assumes zero is wrong in a way that only shows up under load.

<!-- lang:zh -->
### 一个描述"它不做什么"的名字

"NoSQL" 是一个否定，而否定不适合做分类。被它收进去的这些数据库共有的东西是**关系模型的缺席** —— 而那个集合里的四类，彼此之间的共同点比它们各自与关系库的共同点还少。

> **NoSQL 说的是这些存储不做什么。** 选其中一个，意味着选择放弃哪些保证，并且知道替代它们的是什么 —— 通常是应用。

| 类别 | 例子 | 数据模型 | 适合 |
|---|---|---|---|
| **文档** | MongoDB、CouchDB、Firestore | 类 JSON 的文档 | 一次读一个聚合 |
| **键值** | Redis、Memcached、etcd | 键，和一个不透明的值 | 按键取的速度、计数、缓存 |
| **宽列** | Cassandra、HBase、Bigtable | 稀疏列的行 + 分区键 | 极高的写入量 |
| **图** | Neo4j、JanusGraph | 节点与边都是一等公民 | 关系上的遍历 |

下面的实测用 SQLite 把两种数据模型并排放着 —— 规范化的表 对着 一个 JSON 列 —— 因为这些取舍是模型的属性，不是某一个产品的属性。键值那一节用的是**真的 Redis 8.0.6**：起在一个临时端口上、只绑回环地址、关掉持久化、结束前关掉。

### 第一部分：关系模型原本在给你什么

先看关系模型的理由，是"离开它"意味着放下一些具体的东西，而每一件都在干活：

| 它提供 | 没有它之后 |
|---|---|
| 声明的模式 | 数据的形状住在写入的代码里 |
| 引用完整性 | 联系由应用维护 |
| 查询时计算的连接 | 访问路径在写下数据时就定了 |
| 跨行跨表的事务 | 原子性缩小到一个文档或一个键 |
| 即席查询 | 查询得由事先设计好的索引来服务 |

**上面这些没有一条是 NoSQL 存储的缺陷。** 每一条都被换成了别的东西：形状灵活、水平扩展、写入吞吐、或者一个贴合某类问题的数据模型。但只有当"放弃的那样东西"确实不需要时，这笔交易才划算 —— 而要在这件事上判断错，最省事的办法就是相信"无模式"等于"没有约束要维护"。

### 第二部分：数据模型 —— 聚合，而不是表

关系模式按种类把数据拆开，再用连接拼回来。文档存储把一起读的东西放在一起。实测，从一个进程里读 500 个完整订单 —— 每个五个行项：

| 组织方式 | 行数 | 读 500 个订单耗时 | 发出的查询数 |
|---|---|---|---|
| **规范化** —— `orders` + `order_items` | 50,000 + 250,000 | **2355 ms** | **1000**（每单两次） |
| **嵌入** —— 一个订单一个 JSON 文档 | 50,000 | **0.57 ms** | **500**（每单一次） |

**这里既有模型的差别，也有往返次数的差别**，而后者是更大的因素：规范化那份每个订单发两次查询，文档那份一次。**这恰恰就是两种模型在争的那件事。** 关系那份必须碰两张表，因为数据是按种类拆开的；文档那份读一条记录，因为数据是按它被使用的方式存的。

**代价出现在另一侧。** 在那份文档里，客户名存在 50,000 个订单的每一个里；而规范化那份只存一次、其余引用它。改一个客户名，在第一种组织下是一条 `UPDATE`，在第二种下是 50,000 个文档。

**思考单位不同。** 关系表是"一种东西的一堆行"；文档是一个**聚合**，它被当作一个单位读写。这个决定预先定下了"什么可以是原子的"，那就是下一部分。

### 第三部分：谁保证形状是对的

"无模式"是一个误称。结构仍然存在 —— 它只是住在写入的代码里，而不是在数据库里。实测，同样缺失一个字段：

| 语句 | 结果 |
|---|---|
| `INSERT INTO strict_t (id, customer) VALUES (1, 'alice')`，而 `amount NOT NULL` | **拒绝** —— `NOT NULL constraint failed: strict_t.amount` |
| 同样的数据，作为一个没有 `amount` 字段的文档 | **接受** —— 存成 `{"customer": "alice"}` |
| 然后数"金额大于零"的行 | **0 行** —— 那条记录对这条查询不可见 |

**那条记录是存在的。** 它只是无法被那条本该关心它的查询找到，而在写入的那一刻没有任何东西失败。关系模式里的一句 `NOT NULL` 是**由数据库执行的保证**；文档里缺的那个字段是**没有任何人执行的错误**。

**纠正的手段是有的，而且它是可选的。** MongoDB 的 `$jsonSchema` 校验器、或者集合上的 schema，做的正是 `NOT NULL` 做的事 —— 而因为它是可选的，除非有人打开它，否则它就是关着的。这就是整个转变的形状：**校验从"数据库必须做"变成"应用应该做"**，而这次移动在一次"只看数据库"的评审里是看不见的。

### 第四部分：查询被搬进了模型里

在关系库里，一条查询可以在任何东西上过滤和连接，规划器去想怎么做。在其他几类里，访问路径是你要声明的东西：

| 引擎 | 找某个客户的订单时的计划 |
|---|---|
| 一个普通列，没有索引 | `SCAN orders` —— 每一行 |
| 同一列，建了索引 | `SEARCH orders USING INDEX idx_cust` |
| **JSON 文档里的一个字段** | `SCAN orders_doc` —— 每个文档，都要解析 |

同样的结果集，实测耗时：**在文档里面找 33.19 ms，在带索引的列上找 0.05 ms。**

**差别不在于文档慢。** 而在于**被搜的那个字段不是一个列**，所以没有有序结构可以查 —— 必须先把值从每个文档里取出来才能比较。文档存储解决这件事的办法，和它解决这里其他事情的办法一样：**为你要查询的字段声明一个索引**（MongoDB 的 `createIndex`），这意味着事先决定会被问什么。

**而这就是"为查询建模"的意思。** 在关系库里你为数据建模、之后随便问什么；这里，问题本身是设计的一部分，而事后改一个问题是迁移，不是一条查询。

### 第五部分：一个键值库，实测

模型又往前一步：**只有 key 是有结构的**，值对存储不透明。在一张键值形状的表里放 10 万行，能看出这个不对称；而真的 Redis 在没有关系层的情况下显示同一件事：

| 查找方式 | SQLite 当成键值表 | Redis 8.0.6 |
|---|---|---|
| 按 key | **0.10 ms** —— `SEARCH ... USING INDEX` | **0.12 ms** |
| 按**值里面**的字段 | **30.33 ms** —— `SCAN` | **2222 ms** —— 取回每一个键再读它 |

把 5 万个键一条一条写进 Redis 花了 **2298 ms**，那是往返的代价而不是存储的：每个 `SET` 都是一次请求加一次回复。

**这个不对称是模型，不是实现。** key 是存储为它维护了索引的东西；value 是它从不解释的字节。所以任何"不是这个键上有什么"的问题，都得靠**在旁边维护另一份结构**来回答 —— 一个二级索引、一个有序集合、一组 id —— 并让它与数据保持同步。每一个能回答第二个问题的键值设计都在做这件事，无论它说不说。

**值里面可以有结构。** Redis 的哈希、列表、有序集合、流，都是一个键里面的结构 —— 而它们的用处，恰恰只对拿到那个键的人成立：

```
HSET user:1 name alice role admin    -> 一个键底下的哈希
LPUSH queue:jobs a b c               -> 一个键底下的列表
ZADD board 100 alice 250 bob         -> 一个键底下的有序集合
```

**值内部的结构是应用与那个键之间的私事。** 存储里没有任何东西能把两个键连起来，所以它们之间的关系是应用要去维护的 —— 这和第一部分说的是同一句话，只是换了一个类别。

**而这个类别有一样关系引擎一般没有的东西**（至少不是一个原生概念）：

```
SET session:abc value EX 60   ->   TTL 60
SET nottl value               ->   TTL -1   （不过期）
```

**过期由存储层执行**，而不是由应用里的清理任务执行。对会话数据来说这是一个有意义的差别："这个东西在某个时刻起不再有效"成了数据所在之处的一个性质，而不是读它的那段代码的性质。

### 第六部分：原子性，以及事务不是什么

每一类都有一个原子单位，而它比关系事务小。在同一个 Redis 连接上实测，一组命令里第三条对键的类型来说是非法的：

```
MULTI                          -> OK
SET a 1                        -> QUEUED
LPUSH a x                      -> QUEUED      （a 里是字符串）
SET b 2                        -> QUEUED
EXEC                           -> ['OK', 'ERR WRONGTYPE ...', 'OK']
a = 1     b = 2
```

**中间那条失败了，另外两条生效了。** `MULTI` 保证的是**没有别的客户端的命令插进这一组之间**；它不保证这一组要么全发生要么全不发生。一条对键类型非法的命令在它运行时才被发现，而队列里剩下的照常执行。

**所以"我包在 MULTI 里了"不等于"我有一个事务"**，而同样的谨慎适用于任何分组原语：它排除了什么、以及它不回滚什么，是两个不同的问题。

**在文档存储里，原子单位是文档。** 当前版本里存在跨文档事务，而它有自己的代价 —— 这就是为什么设计指引是"把必须一起变的东西**放进同一个文档**"。那不是变通，那正是第二部分里那个聚合存在的理由。

### 第七部分：最终一致，以及它打开的那个窗口

跨节点复制是保证改变形状的地方。从副本读意味着主节点已确认的写入可能有一段时间不可见，于是同一次读对不同读者给出不同答案：

| 异常 | 读者看到什么 |
|---|---|
| 违反读己之写 | 你自己的改动还不在这里 |
| 违反单调读 | 两次读之间值往回退了 |
| 违反因果 | 一条回复出现在它所回复的消息之前 |

**那个窗口的安全读法很具体。** 当权限存在于一个复制的存储里 —— 一个角色、一个会话、一个令牌的吊销 —— **改动会在它到达时在每个副本上生效**。对吊销来说，这意味着那个凭据在某些节点上、在窗口长度内，仍然有效。

那并不说明复制是错的；它说明那个窗口是一件要被**说明与预算**的事，而不是假定它不存在。一个一秒内传播完的吊销，和一个三十秒传播完的吊销，是两个不同的设计。

### 第八部分：从这些模型推出的安全观念

**校验搬走了，注入面跟着搬走了。** 关系那一族的典型失效，是一个拼出来的字符串遇上一个解析器。文档那一族的典型失效，是**一个类型**遇上一个查询：当一个过滤条件既接受值又接受运算符时，发来运算符就改变了那个问题。

```
{ "user": "alice" }              一个值：匹配它
{ "user": { "$ne": null } }      一个运算符：匹配任何不是 null 的
```

**如果代码把请求体塞进查询而不先决定它是什么，客户端就自己选择了"发来的是一个名字还是一个条件"** —— 那是 NoSQL 那篇的发现，从与批量赋值同一扇门进来。

**而能力的边界就是攻击的边界。** 关系注入能碰到 `UNION`、堆叠语句、以及在引擎与账号允许时的文件读写。其他几族各有自己同类的原语：

| 原语 | 存在于 |
|---|---|
| `$where` 与服务端 JavaScript | MongoDB，在它被开启时 |
| `EVAL` / Lua 脚本 | Redis |
| 带 `$lookup` 的聚合管道 | MongoDB —— 按查询做连接 |
| `KEYS`、`SCAN`、`CONFIG` | Redis —— 枚举，以及从客户端改配置 |

**规则在每一族里都一样：查询语言能表达什么，注入就能表达什么。** 这让"这个部署允许哪些运算符与命令"成为一个安全问题，而不是一个口味问题。

**默认值是它的另一面。** 一个没有校验器的文档存储接受任何东西；一个没有认证、又绑在公网上的键值存储接受来自任何地方的连接。**两者都不是产品的漏洞** —— 两者都是适合开发机的默认值，而两者在真实事故里都曾经是全部的问题所在。

**而最后那个后果，是关于"保证由谁执行"的。** 关系数据库自己执行很多事：类型、唯一性、引用完整性、一个事务的原子性。其他几族把其中大部分搬进了应用 —— 这意味着**一次只读数据库配置与模式的审计会找到更少的东西**，而同一个问题必须靠读应用代码来回答。

### 检测与缓解

- **对键值那一族，记操作而不只是记连接。** `KEYS`、对整个键空间的 `SCAN`、`CONFIG`、`EVAL`、`FLUSHALL` —— 这些都不是普通应用在生产里会发的东西，而每一条单独拿出来都很便宜就能告警。
- **盯"该是值的地方出现了运算符形状的输入"。** 一个本该装字符串的字段收到了 `{"$ne": null}` 这样的 JSON，就是这一类的签名；它可以在应用边界上、在到达查询之前就被发现。
- **对大小与数量突变的文档或键告警。** 因为文档是原子单位，一个不断变大的文档既是性能问题，也是"有什么在追加而不是更新"的迹象。
- **并且量复制延迟，因为那就是吊销窗口的长度。** 这个指标不只是运维的：它是"一个被停用的账号在某些地方仍然可用"的那段时间。
- **缓解上，把存储提供的校验打开。** 集合上的 schema 校验器、构造过滤条件之前的类型检查、一份"客户端可以设置哪些字段"的白名单 —— 用存储自己的机制，比一次指望应用查过了的评审便宜。
- **关掉你用不到的原语。** 服务端 JavaScript、客户端提交的脚本、以及从应用连接可达的管理命令，都是授予"任何能到达这个连接的人"的能力。如果没有东西依赖它们，关掉不花任何代价。
- **认证与绑定是头两个要看的设置**，而它们在每一族里都是同样两个：谁能连，从哪连。
- **把应用账号的权限压到最小，并注意这个上限按族而变。** 一个只读的关系账号不能写文件；一个带 `CONFIG` 的键值账号可以在运行期改服务器的行为。
- **当应用需要一个存储不提供的保证时，把它显式补回来。** 跨文档的不变量、为第二个问题准备的二级索引、以及"存储的原子单位太小时的应用层事务" —— 每一处都是关系引擎曾经免费替你做的那份工作。
- **并且把一致性窗口写成一个数字。** 如果一次权限变更最多要三十秒才到每个读者，那三十秒就是这个系统行为的一部分，而一个假定它是零的设计，错在一种只有压力下才显形的方式上。
