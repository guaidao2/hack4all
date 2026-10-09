---
id: db-elasticsearch
title_en: Elasticsearch
title_zh: Elasticsearch
summary_en: A store where the schema is inferred from the first document that arrives, the data is taken apart into an inverted index rather than kept as written, and a write is not searchable until something refreshes it. Measured on a live node — the mapping it invented, a term query that cannot find the string it was given, and a bulk write that half succeeded.
summary_zh: 一个 schema 由第一个到来的文档推断、数据被拆成倒排索引而不是原样保留、而且写入在有人刷新之前搜不到的存储。在一个真在跑的节点上实测 —— 它自己造出来的映射、一个找不到给定字符串的 term 查询，以及一次只成功一半的批量写入。
tags: [beginner, database, elasticsearch, search, mapping, inverted-index]
tools: [elasticsearch, curl, python3]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The index is the data

Every store in this series keeps what you gave it, in some arrangement. A search engine does something more drastic: it takes the document apart and keeps a structure built from its contents.

> **What is stored is not the document. It is an inverted index — a map from terms to the documents containing them — plus the original source for retrieval.**

Three properties follow, and all three are measured below:

**The schema is inferred**, because a document cannot be indexed until a decision has been made about its fields and nobody is there to make it.

**Text is taken apart into terms**, so what can be found is a term rather than a string.

**And a write becomes searchable later**, because building the index structure is separate from accepting the document.

Measured on a live **Elasticsearch 8.19.23** single node, started for the purpose and stopped afterwards.

### Part 1: the shape of it

| | Here |
|---|---|
| table | **index** |
| row | **document** (JSON) |
| column | **field**, with a **mapping** that may be declared or inferred |
| schema | **inferred from the first document unless you declare one** |
| storage | **an inverted index**; the source document is kept alongside it |
| distribution | an index is split into **shards**, each with **replicas** |
| consistency | **near-real-time** — a write is searchable after a refresh |

Measured, a fresh node reports itself as one machine with everything green:

```
GET /                -> version 8.19.23, cluster_name "elasticsearch"
GET /_cluster/health -> status "green", number_of_nodes 1
GET /_cat/shards?v   -> one STARTED primary shard for the index being used
```

**And the coordination API answers without credentials**, which is the first thing to notice about a default installation and the subject of Part 7.

### Part 2: the mapping is invented from the first document

There is no `CREATE TABLE`. Measured, indexing a single document:

```json
{"title": "Hello World", "body": "The quick brown fox jumps",
 "age": 30, "when": "2026-10-08", "tag": "alpha"}
```

**And the server decides the types**, which it then reports:

| Field | Type |
|---|---|
| `age` | `long` |
| `title`, `body`, `tag` | **`text`, each with a `keyword` sub-field** |
| **`when`** | **`date`** |

**Two things in that table are worth pausing on.**

**The date was inferred from a string.** The value `"2026-10-08"` is a string in JSON, and the server recognised its shape and decided the field holds dates. That decision is now permanent for the index, and measured:

| Value written to `when` | Result |
|---|---|
| `"2026-11-01"` | accepted |
| `"2026-11-01T10:00:00Z"` | accepted |
| **`"08/10/2026"`** | **refused** — `failed to parse field [when] of type [date]` |

**A date written in a different but perfectly ordinary order is rejected**, because the field was pinned to the shapes that could be parsed when it was created. The measured string that started it was unambiguous; the one that fails is the format most of the world writes.

**And every string field gets two fields.** The mapping shows `text` with a `keyword` child:

```json
"title": { "type": "text",
           "fields": { "keyword": { "type": "keyword", "ignore_above": 256 } } }
```

**One field for searching word by word, one for matching exactly** — and the second has a limit of 256 characters, which is a detail that will matter.

**Which leads to the property that governs everything else here:**

> **A field's type is the history of the data that arrived first, not a decision anyone made.** And when the inference is wrong, the fix is a reindex rather than an `ALTER`.

**The coercion that follows from an inferred type is measured too**, on a `long` field:

| Value | Result |
|---|---|
| `"31"` — a numeric string | **accepted** |
| `31.7` — a fractional number | **accepted** |
| `"not a number"` | **refused** — `failed to parse field [age] of type [long]` |

**So a string that looks like a number is stored, and a fraction is accepted** — which means the value that ends up in the index is not necessarily the value that was sent. That is the same silent coercion measured in the relational engines, in a system where nobody declared the type in the first place.

### Part 3: text is taken apart, so a term is not a string

This is the part that most often turns into a bug. Measured, asking the server to show how it would break a value up:

| Value | Field | Terms produced |
|---|---|---|
| `"Hello World"` | `title` (`text`) | **`hello`, `world`** |
| `"alpha beta"` | `tag` (`text`) | **`alpha`, `beta`** |
| `"alpha beta"` | `tag.keyword` | **`alpha beta`** — one term, the whole string |

**And the consequence, measured on a document whose `title` is exactly `"Hello World"`:**

| Query | Result |
|---|---|
| `term` on `title` for `"Hello World"` | **0 hits** |
| `term` on `title` for `"hello"` | **1 hit** |
| `match` on `title` for `"Hello World"` | **1 hit** |

**Read those three rows together.** The string that is literally in the document cannot be found by an exact-term query, because the index does not contain that string — it contains the two terms `hello` and `world`. Searching for `hello` works. And `match` works because **it analyzes its input the same way the field was analyzed**, producing the same two terms to look up.

> **`term` asks "is this exact token in the index". `match` asks "what does this text reduce to, and are those tokens present".** Which one you need depends on whether the field was analyzed.

**And the `keyword` sub-field is the answer for exact matching** — it holds the whole value as a single term, which is why the mapping provides one automatically for every string.

**Two consequences that are easy to hit in production.** A filter that takes a value from a user and searches it with `term` will silently return nothing for anything containing a space or punctuation. And **`ignore_above: 256`** means that for a long string the `keyword` sub-field is not indexed at all — so an exact-match lookup on a value that is long enough fails without an error, on a field that works fine for short values.

### Part 4: a write is not searchable yet

The index structure is built by a refresh, which happens periodically rather than on every write. Measured:

| Step | Searchable |
|---|---|
| index a document with `refresh=false` | **0 hits immediately afterwards** |
| wait for the periodic refresh, or call `POST /_refresh` | **2 hits** |

**The document was accepted and acknowledged, and it was not findable.** That is the near-real-time property, and it is a deliberate trade: batching the index build is what makes writes cheap, and the price is a window in which the data exists but cannot be searched.

**And it is a bug generator with a specific shape**: a service writes a record, then immediately queries for it to confirm the write, sees nothing, and reports failure. The write succeeded. The measured `refresh=wait_for` parameter exists for exactly this case — it makes the request wait until the document is searchable.

### Part 5: no transactions, and concurrency is per document

A single document is written atomically. Across documents there is no transaction, and it shows in a bulk request. Measured, three operations in one `_bulk` call with the middle one malformed:

| Operation | Status |
|---|---|
| document 1 | `201` |
| **document 2** | **`400 document_parsing_exception`** |
| document 3 | `201` |

**And afterwards the index contains documents 1 and 3.** The failed operation did not undo the others, and there is no mechanism that would have.

**The concurrency control is optimistic and versioned.** Measured, updating with the sequence number read beforehand succeeds, and reusing that same number fails:

```
PUT /lab/_doc/1?if_seq_no=0&if_primary_term=1   -> 200
PUT /lab/_doc/1?if_seq_no=0&if_primary_term=1   -> 409 version_conflict_engine_exception
```

**So the model is: read the version, write with it, and retry on conflict.** That is the correct pattern for a search index and the wrong one for anything that needs a guarantee across documents — for which the answer is to keep the record of truth somewhere else.

### Part 6: scripting, and how far it reaches

Queries can carry a script, in a language called Painless, and the server reports a large surface of script contexts. Measured:

| Script | Result |
|---|---|
| `doc['age'].value * 2` | **`{'computed': [60]}`** — the value was computed per document |
| `Runtime.class.getName()` | **`script_exception / compile error`** |
| `GET /_script_context` | **41 contexts** |

**The arithmetic worked and the attempt to reach the JVM's own classes did not** — the sandbox allows expressions over documents and blocks the path to the runtime. That is the right shape of restriction, and it is a restriction on *what the language can do*, not on *whose data it touches*: a script in a query runs with the privileges of the request, over whatever that request can read.

**And the surface is large on purpose.** Forty-one script contexts is a search engine offering computation at many points in a query, which is a useful feature and a larger area than a filter expression.

### Part 7: doing it yourself

```bash
systemctl start elasticsearch
curl -s http://127.0.0.1:9200/
```

```bash
GET  /_cat/indices?v          # what exists, and how big
GET  /_cat/shards?v           # how it is distributed
GET  /_cat/nodes?v            # where the resources are going
GET  /idx/_mapping            # the schema it invented
GET  /idx/_analyze            # how a value would be broken into terms
POST /idx/_search             # query
POST /_bulk                   # many writes in one request
```

**`_analyze` is the tool for the confusion in Part 3.** When a query does not match, seeing the terms the field actually contains answers the question in one request, instead of guessing at the query.

**And `_mapping` is the tool for the confusion in Part 2.** The types are not in the application's code, because nothing in the application declared them; the index is where they live.

**One version-specific fact worth stating:** current versions have authentication and TLS enabled by default and generate credentials on first start, so an installation that answers without credentials had security switched off — deliberately, or by following an older guide.

### Part 8: what follows for security

**An unauthenticated node discloses its identity and its contents.** Measured, with no credentials:

| Request | Response |
|---|---|
| `GET /` | **version, cluster name, Lucene version** |
| `GET /_cat/indices?v` | **every index, with document counts and sizes** |
| `GET /_cat/shards?v` | shard layout |
| `GET /_cat/nodes?v` | **heap, RAM and CPU per node** |
| `GET /_cluster/settings` | cluster settings |

**The version tells a scanner what to try; the `_cat` APIs tell it what is worth taking.** And a search index is a particular kind of target, because it is often a *copy* of data that lives elsewhere — with the access controls of neither.

**The inference in Part 2 is writable, and that is the deeper issue.** A mapping created from the first document means **whoever writes the first document decides the types**. Two consequences:

**An attacker who can write can pin a field to a type that breaks legitimate writes.** A string field inferred as `date` refuses the application's own values afterwards; a numeric field refuses text. The measured rejections are correct behaviour on a schema the attacker chose.

**And fields can be created by writing to them.** With dynamic mapping on, a document containing new keys adds them to the mapping, so an injection that reaches a write can grow the index's shape and its storage.

**The `term` versus `match` difference is a security issue in one direction.** A filter built with `term` on an analyzed field returns nothing — which is a bug. A filter built with `match` on data that should have been compared exactly is worse: **`match` analyzes the input, so partial and reordered text can match**, and a check written expecting equality gets a similarity. **Where a value must match exactly, the `keyword` sub-field is the field to use**, and the measured `ignore_above: 256` means long values need the limit raised or they are not indexed at all — failing silently in the direction of "no match", which for an allow-list is the safe direction and for a deny-list is not.

**Scripting is code execution with the request's privileges.** Measured, the sandbox blocked reaching the JVM and allowed arithmetic over documents. **So the risk is not the sandbox escaping; it is a query whose script text came from input.** As with every language in this part of the guide, the value should be a parameter and the script should be a constant.

**And there is no transaction, which is a correctness problem that becomes a security problem when a decision depends on a write.** Measured, a bulk request left two of three documents in place. **An application that treats a bulk response as all-or-nothing has already lost**, because the measured response reports per-document statuses and nothing rolls back.

### Detection and mitigation

- **Check whether security is enabled and whether the node is reachable beyond loopback.** Measured, an unauthenticated node answers `GET /` and the `_cat` APIs; current versions enable both authentication and TLS by default, so an open one was opened.
- **Alert on `_cat/*` and `_cluster/*` requests from outside the cluster's own tooling.** They are reconnaissance by nature and legitimate by design, which makes them easy to distinguish from normal traffic.
- **Watch for warning headers and rejected documents from date and type parsing.** Measured, a field pinned to `date` refuses a differently formatted date, which is either an application bug or a field type that somebody else's write decided.
- **Monitor the mapping for changes**, since dynamic mapping means a write can add fields and change types — a schema change with no migration and no review.
- **Audit queries carrying scripts, and any script text that is not a constant.** Measured, the sandbox restricts what the language can reach; it does not restrict what the request can read.
- **Track bulk responses for partial failures.** Measured, a `_bulk` with one bad document indexed the others and reported `errors: true` per document; treating it as atomic is the mistake to look for in code.
- **For mitigation, keep the default security configuration or configure it deliberately**, and put the node behind the same network boundary as the data it indexes.
- **Use roles and API keys rather than a shared credential**, and give each client only the indices it needs, since an index is usually a copy of data with its own classification.
- **Use `keyword` for exact matching and `text` for searching, and say which one each field is for.** The measured `term` on an analyzed field returns nothing, and a `match` where equality was intended returns too much.
- **Declare mappings explicitly for anything with a schema of consequence.** Inferred types are the first document's accident, and correcting them afterwards is a reindex.
- **Turn off dynamic mapping where field names come from data**, so that a write cannot extend the schema.
- **And design for the refresh window.** A write that must be immediately searchable needs `refresh=wait_for` or an explicit refresh; assuming it is visible is the measured cause of a class of false negatives.

<!-- lang:zh -->
### 索引就是数据

这个系列里每一个存储都把你给它的东西、以某种排布保留下来。搜索引擎做的事更彻底一些：**它把文档拆开，并保留一份由它的内容建起来的结构。**

> **被存下来的不是那个文档，而是一份倒排索引 —— 一张从"词"到"含它的文档"的映射 —— 外加原始 source 供取回。**

由此推出三条性质，而三条都会在下面实测：

**schema 是被推断出来的**，因为一个文档在被索引之前必须先有人决定它的字段是什么类型，而没有人在那儿做这个决定。

**文本会被拆成一个个词**，所以能被找到的是"词"，而不是"字符串"。

**而一次写入是稍后才变得可搜索的**，因为建索引结构与接收文档是两件事。

在一个为此启动、用完即停的 **Elasticsearch 8.19.23** 单节点上实测。

### 第一部分：它的形状

| | 这里 |
|---|---|
| 表 | **索引**（index） |
| 行 | **文档**（JSON） |
| 列 | **字段**，附一份可以声明、也可以被推断出来的**映射** |
| schema | **除非你声明，否则由第一个文档推断** |
| 存储 | **倒排索引**；原始文档另存一份 |
| 分布 | 一个索引被切成若干**分片**，每片可有**副本** |
| 一致性 | **近实时** —— 写入在若干刷新之后才可搜索 |

实测，一个全新节点报告自己是一台机器、而且一切都是绿的：

```
GET /                -> 版本 8.19.23，cluster_name "elasticsearch"
GET /_cluster/health -> status "green"，number_of_nodes 1
GET /_cat/shards?v   -> 正在用的那个索引有一个 STARTED 的主分片
```

**而这些协调用的 API 不需要凭据就会应答**，这是一个默认安装第一件该注意到的事，也是第七部分的主题。

### 第二部分：映射是从第一个文档造出来的

这里没有 `CREATE TABLE`。实测，索引一个文档：

```json
{"title": "Hello World", "body": "The quick brown fox jumps",
 "age": 30, "when": "2026-10-08", "tag": "alpha"}
```

**而服务端自己决定类型**，然后把它报出来：

| 字段 | 类型 |
|---|---|
| `age` | `long` |
| `title`、`body`、`tag` | **`text`，各自带一个 `keyword` 子字段** |
| **`when`** | **`date`** |

**那张表里有两处值得停一下。**

**那个日期是从一个字符串推断出来的。** 在 JSON 里 `"2026-10-08"` 是一个字符串，而服务端认出了它的形状、断定这个字段装的是日期。这个决定对这个索引从此就是永久的，实测：

| 往 `when` 里写的值 | 结果 |
|---|---|
| `"2026-11-01"` | 接受 |
| `"2026-11-01T10:00:00Z"` | 接受 |
| **`"08/10/2026"`** | **被拒** —— `failed to parse field [when] of type [date]` |

**一个顺序不同但完全正常的日期写法被拒绝了**，因为这个字段被钉在了它被创建时能解析出来的那几种形状上。引发这一切的那个字符串是毫无歧义的；失败的那个，是世界上很大一部分人写日期的写法。

**而每一个字符串字段都变成了两个字段。** 映射里的 `text` 带着一个 `keyword` 子字段：

```json
"title": { "type": "text",
           "fields": { "keyword": { "type": "keyword", "ignore_above": 256 } } }
```

**一个用来逐词搜索，一个用来精确匹配** —— 而第二个有一个 256 字符的上限，这是个要紧的细节。

**这就引出了支配这里其他一切的那条性质：**

> **一个字段的类型是"最先到达的那份数据的历史"，而不是任何人做过的决定。** 而当这个推断错了，修法是重建索引，而不是一句 `ALTER`。

**由推断出来的类型带来的强制转换也实测了**，在一个 `long` 字段上：

| 值 | 结果 |
|---|---|
| `"31"` —— 一个数字字符串 | **接受** |
| `31.7` —— 一个小数 | **接受** |
| `"not a number"` | **被拒** —— `failed to parse field [age] of type [long]` |

**所以一个看起来像数字的字符串会被存下来，而一个小数也会被接受** —— 这意味着最终进到索引里的值，不一定是发出去的那个值。这与前面关系型引擎里实测到的静默转换是同一件事，只是发生在一个根本没人声明过类型的系统里。

### 第三部分：文本被拆开了，所以"词"不是"字符串"

这是最容易变成 bug 的一部分。实测，让服务端展示它会怎么拆一个值：

| 值 | 字段 | 产生的词 |
|---|---|---|
| `"Hello World"` | `title`（`text`） | **`hello`、`world`** |
| `"alpha beta"` | `tag`（`text`） | **`alpha`、`beta`** |
| `"alpha beta"` | `tag.keyword` | **`alpha beta`** —— 一个词，整个字符串 |

**而后果是，在一个 `title` 恰好就是 `"Hello World"` 的文档上：**

| 查询 | 结果 |
|---|---|
| 用 `term` 在 `title` 上查 `"Hello World"` | **0 条** |
| 用 `term` 在 `title` 上查 `"hello"` | **1 条** |
| 用 `match` 在 `title` 上查 `"Hello World"` | **1 条** |

**把这三行放在一起读。** 那个**字面上就在文档里**的字符串，用精确词查询找不到，因为索引里没有那个字符串 —— 它有的是 `hello` 和 `world` 这两个词。查 `hello` 能找到。而 `match` 能找到，是因为**它会用字段当初被分析时的同一种方式分析你的输入**，得出同样那两个词去查。

> **`term` 问的是"索引里有没有这个确切的词"。`match` 问的是"这段文本会归约成哪些词，那些词在不在"。** 你需要哪一个，取决于那个字段当初有没有被分析。

**而 `keyword` 子字段就是精确匹配的答案** —— 它把整个值当成单独一个词存着，这就是映射会自动给每个字符串配一个的原因。

**两个在生产上很容易撞到的后果。** 一个把用户给的值拿去做 `term` 的过滤器，对任何含空格或标点的值都会静默地什么也不返回。而 **`ignore_above: 256`** 意味着对长字符串而言，`keyword` 子字段根本没被索引 —— 所以一个精确匹配查询会在值足够长时失败、而且不报错，而这个字段在短值上一直好好的。

### 第四部分：写完还搜不到

索引结构是靠一次 refresh 建起来的，而它是周期性发生的、不是每次写入都做。实测：

| 步骤 | 可搜索条数 |
|---|---|
| 用 `refresh=false` 索引一个文档 | **紧接着搜是 0 条** |
| 等到周期刷新，或者调 `POST /_refresh` | **2 条** |

**那个文档被接受了、也被确认了，而它搜不到。** 这就是近实时这条性质，它是一笔有意的取舍：把建索引攒起来批量做，正是写入便宜的原因，而代价是一段"数据在、但搜不到"的窗口。

**而它是一个有特定形状的 bug 生成器**：一个服务写入一条记录，然后立刻把它查回来以确认写入成功，结果什么也没查到，于是报告失败。**写入是成功的。** 实测到的 `refresh=wait_for` 参数正是为这种情况存在的 —— 它让这个请求等到文档可搜索为止。

### 第五部分：没有事务，而并发是按文档的

单个文档是原子写入的。跨文档没有事务，而这一点在一次批量请求里看得见。实测，一次 `_bulk` 里三个操作、中间那个格式坏掉：

| 操作 | 状态 |
|---|---|
| 文档 1 | `201` |
| **文档 2** | **`400 document_parsing_exception`** |
| 文档 3 | `201` |

**而之后索引里有文档 1 和文档 3。** 失败的那个操作没有撤销其他的，也没有任何机制会去撤销。

**并发控制是乐观的、按版本号的。** 实测，用事先读到的序号去更新会成功，而重复使用同一个序号会失败：

```
PUT /lab/_doc/1?if_seq_no=0&if_primary_term=1   -> 200
PUT /lab/_doc/1?if_seq_no=0&if_primary_term=1   -> 409 version_conflict_engine_exception
```

**所以这个模型是：读出版本号、带着它写、冲突就重试。** 对一个搜索索引来说这是正确的模式，而对任何需要跨文档保证的东西来说是错误的模式 —— 那种情况下，答案是把"真相的记录"放在别处。

### 第六部分：脚本，以及它能伸多远

查询可以带一段脚本，语言叫 Painless，而服务端报告出一个很大的脚本上下文面。实测：

| 脚本 | 结果 |
|---|---|
| `doc['age'].value * 2` | **`{'computed': [60]}`** —— 这个值是按文档算出来的 |
| `Runtime.class.getName()` | **`script_exception / compile error`** |
| `GET /_script_context` | **41 个上下文** |

**算术能跑，而伸手去够 JVM 自己的类不能** —— 沙箱允许对文档做表达式，堵住了通向运行时的路。这是正确的限制形状，而它限制的是**这门语言能做什么**，不是**它碰谁的数据**：查询里的一段脚本以那个请求的权限运行，作用域就是这个请求能读到的一切。

**而这个面是故意做大的。** 41 个脚本上下文，是一个搜索引擎在一条查询的很多位置上提供计算能力 —— 这是个有用的特性，也是一个比"过滤表达式"更大的面。

### 第七部分：自己动手做一次

```bash
systemctl start elasticsearch
curl -s http://127.0.0.1:9200/
```

```bash
GET  /_cat/indices?v          # 有哪些，多大
GET  /_cat/shards?v           # 怎么分布的
GET  /_cat/nodes?v            # 资源花在哪
GET  /idx/_mapping            # 它自己造出来的 schema
GET  /idx/_analyze            # 一个值会被拆成哪些词
POST /idx/_search             # 查询
POST /_bulk                   # 一次请求里做很多写
```

**`_analyze` 是解第三部分那种困惑的工具。** 当一条查询不匹配时，看一眼那个字段实际含有的词，一个请求就能回答，而不是靠猜查询。

**而 `_mapping` 是解第二部分那种困惑的工具。** 类型不在应用的代码里，因为应用里没有任何东西声明过它们；索引才是它们住的地方。

**一个与版本有关的事实值得说明：** 当前版本默认开启认证与 TLS，并在首次启动时生成凭据，所以一个不需要凭据就应答的安装，是把安全关掉了 —— 有意的，或者照着旧的指南做的。

### 第八部分：从这些机制推出的安全观念

**一个未认证的节点会公布自己的身份和内容。** 实测，不带任何凭据：

| 请求 | 响应 |
|---|---|
| `GET /` | **版本、集群名、Lucene 版本** |
| `GET /_cat/indices?v` | **每一个索引，带文档数与大小** |
| `GET /_cat/shards?v` | 分片布局 |
| `GET /_cat/nodes?v` | **每个节点的堆、内存与 CPU** |
| `GET /_cluster/settings` | 集群设置 |

**版本告诉扫描者该试什么；那些 `_cat` API 告诉它什么值得拿。** 而搜索索引是一种特别的目标，因为它往往是"别处也有一份"的数据的**副本** —— 而它并不带着任何一方的访问控制。

**第二部分那个推断是可写的，而这是更深的问题。** 一个由第一个文档创建的映射意味着**谁写下第一个文档，谁就决定了类型**。两个后果：

**能写入的攻击者可以把一个字段钉在一个会破坏正常写入的类型上。** 一个被推断成 `date` 的字符串字段之后会拒绝应用自己的值；一个数值字段会拒绝文本。那些实测到的拒绝，在一个由攻击者选定的 schema 上是正确行为。

**而字段可以被"写出来"。** 动态映射开着时，一个含新键的文档会把这些键加进映射，所以一次能到达写入的注入可以把这个索引的形状和它的存储一起撑大。

**`term` 与 `match` 的差别在一个方向上是一个安全问题。** 在一个被分析过的字段上用 `term` 搭的过滤器什么也不返回 —— 那是一个 bug。而在一份本该被精确比较的数据上用 `match` 搭的过滤器更糟：**`match` 会分析它的输入，所以部分匹配和顺序不同的文本都能匹配上**，于是一个本意是"相等"的检查拿到的是"相似"。**在值必须精确匹配的地方，`keyword` 子字段才是该用的字段**；而实测的 `ignore_above: 256` 意味着长值需要把上限调高，否则它们根本不会被索引 —— 而且是朝"不匹配"的方向静默失败，对一个白名单来说这个方向是安全的，对一个黑名单来说不是。

**脚本是以请求的权限进行的代码执行。** 实测，沙箱堵住了够向 JVM 的路、放过了对文档的算术。**所以风险不是沙箱被逃出去；而是一段脚本文本来自输入的查询。** 和这部分指南里的每一门语言一样，值应当是参数、脚本应当是常量。

**而没有事务，是一个正确性问题；当某个决定依赖于一次写入时，它就变成一个安全问题。** 实测，一次批量请求把三个文档里的两个留在了原处。**一个把批量响应当作全有或全无的应用已经输了**，因为实测到的响应是按文档报告状态的，而且没有任何东西会回滚。

### 检测与缓解

- **查安全有没有开、以及这个节点能不能从回环之外到达。** 实测，一个未认证的节点会应答 `GET /` 和那些 `_cat` API；当前版本默认同时开启认证与 TLS，所以一个开放的节点是被打开的。
- **对来自集群自有工具之外的 `_cat/*` 与 `_cluster/*` 请求告警。** 它们本质上是侦察，而它们按设计又是合法的，这让它们很容易和正常流量区分开。
- **盯来自日期与类型解析的告警头和被拒文档。** 实测，一个被钉成 `date` 的字段会拒绝格式不同的日期 —— 那要么是一个应用 bug，要么是一个字段类型由别人的写入决定了。
- **监视映射的变更**，因为动态映射意味着一次写入可以加字段、改类型 —— 那是一次没有迁移、也没有评审的 schema 变更。
- **审带脚本的查询，以及任何不是常量的脚本文本。** 实测，沙箱限制的是这门语言能碰到什么；它不限制这个请求能读到什么。
- **盯批量响应里的部分失败。** 实测，一个含坏文档的 `_bulk` 索引了其他的、并按文档报告 `errors: true`；把它当作原子操作，就是要在代码里找的那个错误。
- **缓解上，保持默认的安全配置、或者有意识地去配置它**，并把节点放在与它所索引的数据同一道网络边界之内。
- **用角色和 API key，而不是一个共享凭据**，并且只给每个客户端它需要的索引 —— 因为一个索引往往是一份带自己密级的数据的副本。
- **精确匹配用 `keyword`、搜索用 `text`，并把每个字段是给哪种用的说清楚。** 实测，在被分析过的字段上用 `term` 什么也返回不了，而本该相等的地方用 `match` 会返回太多。
- **对任何有实际后果的 schema，显式声明映射。** 推断出来的类型是第一个文档的偶然，而事后纠正它是一次重建索引。
- **在字段名来自数据的地方关掉动态映射**，这样一次写入就无法扩展 schema。
- **并且为那个刷新窗口做设计。** 一次必须立刻可搜索的写入需要 `refresh=wait_for` 或者一次显式刷新；假定它可见，就是实测到的那一类假阴性的成因。
