---
id: db-neo4j
title_en: Neo4j
title_zh: Neo4j
summary_en: The first store here where a relationship is stored rather than computed, which is why a traversal does not get more expensive with depth and why there is no schema to declare uniqueness with. Measured on a live instance over its HTTP interface — plan shapes, a uniqueness constraint that validates existing data, a conditional rollback, and two ways a query built from input goes wrong.
summary_zh: 这是这份指南里第一个"关系是存下来的、不是算出来的"存储 —— 这就是为什么遍历的代价不随深度爆炸，也是为什么没有 schema 来声明唯一性。在一个真在跑的实例上通过它的 HTTP 接口实测 —— 计划形状、一个会校验已有数据的唯一约束、一次可以回滚的事务，以及两处"由输入拼出来的查询"出问题的地方。
tags: [beginner, database, neo4j, graph, cypher, cypher-injection]
tools: [neo4j, cypher, python3]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Relationships are stored

Every store so far answers a question about rows, documents or values. A graph database stores the connections as things in their own right.

> **A relationship is a stored record with its own address, not a match computed at query time.**

That one decision changes two things that matter here. **A traversal does not get more expensive with depth the way a join chain does**, because walking an edge is following a pointer rather than searching a table for a matching value. And **there is no table, so there is no place to declare that a column is unique** — uniqueness, if you want it, is a separate object you create.

Measured on a live **Neo4j 4.4.26** community instance, through its HTTP interface, then stopped and its data cleared.

### Part 1: the shape of it

| Relational | Here |
|---|---|
| table | **label** (a tag on a node, and a node may have several) |
| row | **node** |
| column | **property** (a key-value pair on a node or relationship) |
| foreign key / join table | **relationship**, which has a type, a direction and its own properties |
| primary key | `id(node)`, and how nodes are found is your problem |
| schema | **none** — labels and properties are conventions |

Measured, a small graph built with a few statements:

```
CREATE (a:Person {name:'alice', email:'alice@example.com', age:30})
CREATE (b:Person {name:'bob'})
CREATE (c:Company {name:'acme'})
MATCH (a:Person {name:'alice'}), (c:Company {name:'acme'})
  CREATE (a)-[:WORKS_AT {since:2020}]->(c)
MATCH (a:Person {name:'alice'}), (b:Person {name:'bob'})
  CREATE (a)-[:KNOWS {since:2019}]->(b)
```

Which produces:

| Query | Result |
|---|---|
| `MATCH (n) RETURN labels(n), count(*)` | `[['Person'], 2]`, `[['Company'], 1]` |
| relationships | `Person -[KNOWS {since:2019}]-> Person`, `Person -[WORKS_AT {since:2020}]-> Company` |

**A relationship has a type, a direction, and properties of its own** — the `since` on `WORKS_AT` is data about the connection, which in a relational schema is either a column on a join table or nowhere.

**And a property that was never set reads as null**, exactly as an unwritten Cassandra column does and unlike a MongoDB field, which simply is not there. Measured, the two `Person` nodes:

```
name=alice  email=alice@example.com  age=30
name=bob    email=null               age=null
```

### Part 2: why a traversal does not explode

This is the property that justifies the whole model, and it is visible in the query plans. Measured, a four-node chain `a -> b -> c -> d`, queried at one, two and three hops:

| Hops | Operators |
|---|---|
| 1 | `ProduceResults -> Projection -> VarLengthExpand(All) -> Filter -> **NodeByLabelScan**` |
| 2 | `ProduceResults -> Projection -> Filter -> VarLengthExpand(All) -> **AllNodesScan**` |
| 3 | `ProduceResults -> Projection -> Filter -> VarLengthExpand(All) -> **AllNodesScan**` |

**Read the last operator of each row.** The traversal itself is a single `VarLengthExpand` at every depth — **going three hops is one operator, not three joins** — while the cost of *finding the starting point* is what changes: with a label and no index it is a scan of all nodes with that label, and from two hops on the plan shows a scan of all nodes, because the pattern's first element is no longer constrained by a label in the way the planner can use.

**So the comparison with a relational engine is:**

> **In SQL, a relationship is discovered by matching values, so each additional hop is another pass over a table. Here the relationship is a record, so following it is following a pointer.**

**And the cost of that is paid up front, in the modelling.** You cannot traverse a relationship you did not store; a graph is not more flexible than a schema, it makes the relationships explicit and expects you to have decided them when you wrote the data.

**The measured `AllNodesScan` is the other half of the lesson.** A traversal is cheap once it starts; deciding *where to start* is an ordinary lookup problem, and with neither a label nor an index it is a scan of everything.

### Part 3: indexes and constraints are separate objects

There is no schema to attach these to, so they are created as their own things. Measured on 3000 nodes, with and without an index on the property being matched:

| | Plan |
|---|---|
| **no index** | `ProduceResults -> Filter -> **NodeByLabelScan**` |
| **index on the property** | `ProduceResults -> **NodeIndexSeek**` |

**Two differences, and the second is the interesting one.** The scan becomes a seek, and the `Filter` operator disappears — because the index lookup already selected exactly the matching node, so there is nothing left to filter. **An index here is not only faster; it removes a step.**

**And uniqueness is a constraint you declare**, with a consequence that surprised me and is worth knowing. Measured:

| Step | Result |
|---|---|
| create two nodes with the same `email`, no constraint | **both accepted** |
| `CREATE CONSTRAINT ... ASSERT p.email IS UNIQUE` while the duplicate exists | **refused** — `Unable to create Constraint ... Both Node(3) an...` |
| create the constraint on clean data | **succeeds** |
| insert a second node with the same `email` | **refused** — ``Node(3004) already exists with label `Person` and property `email` = 'dup@example.com'`` |

**Creating the constraint validates the data that is already there**, so on a database with duplicates it fails and reports which nodes conflict. Which means **adding uniqueness to an existing database is a migration with a cleanup step**, not a declaration — the same shape as adding a `NOT NULL` to a populated column, and worth scheduling rather than discovering.

### Part 4: transactions are real

Of the non-relational stores in this series, this is the one whose transaction behaviour is closest to the relational engines. Measured over the HTTP interface, which exposes explicit transactions:

| Step | State of the graph |
|---|---|
| begin a transaction, create a node inside it | **not visible outside the transaction** |
| roll the transaction back | still not there |
| begin another, create a node, commit | **the node is there** |

**A write inside a transaction is invisible until it commits, and it can genuinely be undone.** That is stated plainly because it is not universal in this part of the guide: the measured document store had no rollback at all, and the measured wide-column store offered a conditional write rather than a transaction.

**The HTTP interface is where this is most visible, and it is also the part to look at from the outside.** Each request is its own transaction unless you open one explicitly, which is why the query inside the transaction above returned nothing — the read was a different transaction, so it saw the last committed state.

### Part 5: queries built from input

Cypher is a text language, so the same rule applies as for SQL: input that reaches the statement text reaches the statement's meaning. Measured, with the application doing the obvious thing:

```
MATCH (u:User) WHERE u.name = '<input>' RETURN u.name, u.secret
```

| Input | Result |
|---|---|
| `admin` | `[['admin', 'S3cret-Admin']]` |
| `x' OR 1=1 //` | **error** — `Query cannot conclude with MATCH` |
| **`x' OR u.name <> '`** | **`[['admin', ...], ['guest', 'guest']]`** — both users returned |
| `x' RETURN u //` | `[]` |

**Two mechanics are visible here.** The second row is the classic payload and it **fails** for an instructive reason: `//` comments out the rest of the line, including the `RETURN` clause the query needed, so the statement is invalid rather than bypassed. The third row is the one that works, and its shape is the same as the classic one — **it closes the string and adds a condition that is true for every row**, and the trailing quote of the original query is commented out by the `//` at the end.

**And the fix is the same as elsewhere: parameters.** Cypher has them — `MATCH (u:User) WHERE u.name = $name` with the value supplied separately — and using them keeps the input out of the statement text entirely.

### Part 6: doing it yourself

The instance needs Java 11 or 17 depending on the version; the one measured wanted a recent JDK and answered on two ports:

```
7474   HTTP: a REST interface and a browser
7687   Bolt: the binary protocol the drivers use
```

**The HTTP interface is enough to work without any driver or shell**, which is how everything here was measured:

```bash
# read the version and the advertised endpoints, with no credentials
curl -s http://127.0.0.1:7474/

# run one statement
curl -s -X POST http://127.0.0.1:7474/db/neo4j/tx/commit \
  -H 'Content-Type: application/json' \
  -d '{"statements":[{"statement":"MATCH (n) RETURN count(n)"}]}'
```

**And two things to know when reading plans**, both of which cost time here:

`EXPLAIN` and `PROFILE` results come back in the result object under `plan`, not as rows — so a script that reads `data` sees an empty result and concludes the query returned nothing.

**A configuration property whose name is wrong is ignored, and the server does not complain.** Measured, a line written as `server.default_listen_address` had no effect at all; the actual property is `dbms.default_listen_address`, whose default is `localhost`, and only reading the effective configuration showed which of the two was in force:

```
dbms.default_listen_address        = localhost
dbms.connector.bolt.listen_address = :7687
```

**That is the general shape of a class of mistakes**: a setting that looks right in the file, is silently dropped, and leaves the default in place. It is harmless when the default is the safer value, and it is a finding when it is not.

### Part 7: what follows for security

**The HTTP interface answers without credentials.** Measured, `GET /` returned the version, the edition and both endpoint URLs to an unauthenticated request. It is a small disclosure with a specific use: it tells a scanner **which version's vulnerabilities to try**, and `dbms.security.http_auth_allowlist` exists precisely to carve out which paths may be reached before authentication.

**And `LOAD CSV` is a file and network read from inside the server.** Measured, the behaviour is asymmetric in a way that matters:

| Statement | Result |
|---|---|
| `LOAD CSV FROM 'file:///etc/passwd'` | **refused** — and the error shows the path was rewritten to `/usr/share/neo4j/import/etc/passwd` |
| `LOAD CSV FROM 'http://127.0.0.1:7474/'` | **the content was read** |

**A `file:///` URL is resolved inside the import directory**, so the path is confined rather than the input being rejected — the leading slash does not escape it. **An `http://` URL is fetched from wherever it points**, which makes this clause a server-side request forgery primitive on any interface the database server can reach, including ones only it can reach. On a deployment where a user can influence the URL or the query that contains it, this is the clause to look for.

**Cypher injection is SQL injection with different syntax.** Measured, closing the string and adding a condition returned rows the query was never intended to return. **Parameters are available and are the fix**, and as with any query language, a clause like `LOAD CSV` or `CALL` inside a statement built from input is a much larger problem than a filter that reads too much.

**Authentication is a switch, and the default user exists either way.** Measured, the first start logs a user being created from defaults with the password change required, and the configuration measured had authentication turned off:

```
dbms.security.auth_enabled               = false
dbms.security.auth_max_failed_attempts   = 3
dbms.security.auth_lock_time             = 5s
dbms.security.http_auth_allowlist        = /,/browser.*
```

**So the questions are the same two as for every other store in this part**: is authentication on, and has the default user been dealt with. The lockout and allowlist settings around it are already configured, which makes the switch itself the thing that gets left alone.

**And a graph has a disclosure shape of its own.** Relationships are data, so a role that can read is a role that can map the whole graph: who knows whom, which service talks to which. **Where a graph holds relationships that are themselves sensitive — reporting lines, trust paths, service dependencies — the sensitivity is in the edges, and a permission model that only considers node properties will miss it.**

### Detection and mitigation

- **Check whether authentication is enabled, and whether the default user still has its default password.** Measured, the account is created on first start with a change required, and the configuration is what decides whether anyone is asked for it.
- **Alert on `LOAD CSV` reaching `http://` or `https://` URLs.** Measured, that path reads remote content from the server's network position; the `file:///` form is confined to the import directory and the remote form is not.
- **Watch for `file:///` URLs that appear to be absolute.** Measured, they are resolved relative to the import directory rather than refused, so a path that looks like it escapes does not — which is worth knowing before treating a failure as a block.
- **Treat `CALL` of procedures and any dynamically built Cypher as code paths worth reviewing**, because the procedure surface is large and measured at dozens of entries.
- **Verify the effective configuration rather than the file.** Measured, a misspelled property was ignored without complaint, leaving the default in force; the running server's own view of its settings is the authority.
- **For mitigation, set parameters rather than escaping strings.** Cypher has parameters and using them is the complete fix for the measured injection.
- **Keep the HTTP and Bolt ports off untrusted networks, or keep authentication on and restrict the allowlist** — the measured interface discloses the version to anyone who can reach it.
- **Declare uniqueness constraints deliberately, and expect them to validate existing data.** Measured, creating one over duplicates fails and names the conflicting nodes, so it is a migration rather than a declaration.
- **Add indexes for the properties used as traversal starting points.** Measured, the difference is between a labelled scan plus a filter and a single index seek, and the second one is what a traversal's cost is actually about.
- **Model the relationships you intend to query.** A relationship that was never stored cannot be traversed, so the review question for a graph is not "is the schema right" but "does the stored graph contain the questions the application asks".
- **And decide whether the edges are sensitive.** In a graph, the connections can be the confidential part, and a permission review that looks only at nodes will miss them.

<!-- lang:zh -->
### 关系是存下来的

到目前为止的每一个存储，回答的都是关于行、文档或者值的问题。图数据库把"连接"本身当成一等的东西存起来。

> **一条关系是一条有自己地址的记录，不是查询时算出来的一次匹配。**

这一个决定改变了两件在这里要紧的事。**遍历的代价不会像 join 链那样随深度增长**，因为走一条边是顺着一个指针走，而不是在一张表里搜一个匹配的值。而**没有表，就没有地方声明某列唯一** —— 想要唯一性，它是一个你另外创建的对象。

在一个真在跑的 **Neo4j 4.4.26** 社区版实例上实测，走的是它的 HTTP 接口；测完停掉、数据清掉。

### 第一部分：它的形状

| 关系型 | 这里 |
|---|---|
| 表 | **标签**（节点上的一个标记，一个节点可以有多个） |
| 行 | **节点** |
| 列 | **属性**（节点或关系上的键值对） |
| 外键 / 关联表 | **关系**，它有类型、有方向，也有自己的属性 |
| 主键 | `id(node)`，而"怎么找到节点"是你的事 |
| schema | **没有** —— 标签和属性都是约定 |

实测，用几条语句建出一个小图：

```
CREATE (a:Person {name:'alice', email:'alice@example.com', age:30})
CREATE (b:Person {name:'bob'})
CREATE (c:Company {name:'acme'})
MATCH (a:Person {name:'alice'}), (c:Company {name:'acme'})
  CREATE (a)-[:WORKS_AT {since:2020}]->(c)
MATCH (a:Person {name:'alice'}), (b:Person {name:'bob'})
  CREATE (a)-[:KNOWS {since:2019}]->(b)
```

它产生的结果是：

| 查询 | 结果 |
|---|---|
| `MATCH (n) RETURN labels(n), count(*)` | `[['Person'], 2]`、`[['Company'], 1]` |
| 关系 | `Person -[KNOWS {since:2019}]-> Person`、`Person -[WORKS_AT {since:2020}]-> Company` |

**一条关系有类型、有方向，也有属于它自己的属性** —— `WORKS_AT` 上的 `since` 是关于这条连接的数据；在关系型 schema 里，它要么是关联表上的一列，要么无处安放。

**而一个从没被设置过的属性读出来是 null**，与 Cassandra 里未写入的列一样，而与 MongoDB 的字段不同 —— 那里字段就是不存在。实测，两个 `Person` 节点：

```
name=alice  email=alice@example.com  age=30
name=bob    email=null               age=null
```

### 第二部分：为什么遍历不会爆炸

这就是支撑整个模型的那条性质，而它在查询计划里看得见。实测，一条 `a -> b -> c -> d` 的四节点链，分别查一跳、两跳、三跳：

| 跳数 | 算子 |
|---|---|
| 1 | `ProduceResults -> Projection -> VarLengthExpand(All) -> Filter -> **NodeByLabelScan**` |
| 2 | `ProduceResults -> Projection -> Filter -> VarLengthExpand(All) -> **AllNodesScan**` |
| 3 | `ProduceResults -> Projection -> Filter -> VarLengthExpand(All) -> **AllNodesScan**` |

**看每一行的最后一个算子。** 遍历本身在每一层深度上都只是单个 `VarLengthExpand` —— **走三跳是一个算子，不是三次 join** —— 而变的是**找到起点**的代价：有标签没有索引时，那是把这个标签下的节点全扫一遍；从两跳起，计划显示的是把所有节点扫一遍，因为模式里的第一个元素不再以规划器能利用的方式被标签约束住。

**所以与关系型引擎的对比是：**

> **在 SQL 里，关系是靠匹配值被发现的，所以每多一跳就多一遍对表的扫描。在这里关系是一条记录，所以顺着它走就是顺着一个指针走。**

**而这笔代价是在建模的时候预付的。** 你没存下来的关系就没法遍历；图并不比 schema 更灵活，它只是把关系显式化了，并且指望你在写数据的时候就已经想好了它们。

**实测到的那个 `AllNodesScan` 是这件事的另一半。** 遍历一旦开始就便宜；决定**从哪里开始**是一个普通的查找问题，而既没有标签也没有索引时，那就是全扫一遍。

### 第三部分：索引与约束是独立的对象

没有 schema 可以挂靠，所以它们被建成自己的东西。实测，在 3000 个节点上，对被匹配的那个属性有索引与没有索引：

| | 计划 |
|---|---|
| **没有索引** | `ProduceResults -> Filter -> **NodeByLabelScan**` |
| **属性上有索引** | `ProduceResults -> **NodeIndexSeek**` |

**两处差别，而第二处才有意思。** 扫描变成了查找，而且那个 `Filter` 算子消失了 —— 因为索引查找已经精确选出了匹配的节点，没有剩下什么可过滤的。**这里的索引不只是更快，它还去掉了一个步骤。**

**而唯一性是一条你要声明的约束**，它有一个让我意外、也值得知道的后果。实测：

| 步骤 | 结果 |
|---|---|
| 在两个节点上写同一个 `email`，无约束 | **两条都被接受** |
| 在重复数据还在时 `CREATE CONSTRAINT ... ASSERT p.email IS UNIQUE` | **被拒** —— `Unable to create Constraint ... Both Node(3) an...` |
| 在干净数据上建约束 | **成功** |
| 再插一个相同 `email` 的节点 | **被拒** —— ``Node(3004) already exists with label `Person` and property `email` = 'dup@example.com'`` |

**建约束会校验已经在那里的数据**，所以在有重复的库上它会失败、并报出哪些节点冲突。这意味着**给一个已有数据库加上唯一性是一次带清理步骤的迁移**，而不是一次声明 —— 和给一个已有数据的列加 `NOT NULL` 是同一个形状，值得排进计划，而不是被撞见。

### 第四部分：事务是真的

在这个系列的非关系型存储里，这一个的事务行为最接近关系型引擎。通过 HTTP 接口实测，它暴露了显式事务：

| 步骤 | 图的状态 |
|---|---|
| 开一个事务，在里面建一个节点 | **事务外看不到** |
| 把这个事务回滚 | 仍然不在 |
| 再开一个，建节点，提交 | **节点在了** |

**事务里的写在提交前不可见，而且它真的可以被撤销。** 这句话要明说，是因为在这部分指南里它并不普遍：实测过的那个文档存储根本没有回滚，实测过的那个宽列存储给的是一次条件写、而不是事务。

**HTTP 接口是这件事最可见的地方，而它也正是从外面要看的那个部分。** 除非你显式开一个事务，否则每个请求就是它自己的事务 —— 这就是为什么上面事务里的那次查询什么都没返回：那次读是另一个事务，看到的是最后提交的状态。

### 第五部分：由输入拼出来的查询

Cypher 是一门文本语言，所以和 SQL 一样的规则成立：到达语句文本的输入，就到达了语句的含义。实测，应用按最显然的写法写：

```
MATCH (u:User) WHERE u.name = '<input>' RETURN u.name, u.secret
```

| 输入 | 结果 |
|---|---|
| `admin` | `[['admin', 'S3cret-Admin']]` |
| `x' OR 1=1 //` | **报错** —— `Query cannot conclude with MATCH` |
| **`x' OR u.name <> '`** | **`[['admin', ...], ['guest', 'guest']]`** —— 两个用户都返回了 |
| `x' RETURN u //` | `[]` |

**这里能看到两种机制。** 第二行是那个经典载荷，而它**失败**的原因很有教益：`//` 把这一行剩下的部分注释掉了，包括这次查询需要的 `RETURN` 子句，于是这条语句非法、而不是被绕过。第三行是生效的那个，而它的形状和经典那个一样 —— **它闭合了字符串、加上一个对每一行都为真的条件**，而原来那条查询末尾的引号被最后的 `//` 注释掉了。

**而修法和别处一样：参数。** Cypher 有参数 —— `MATCH (u:User) WHERE u.name = $name`，值另外提供 —— 用它们就能把输入完全挡在语句文本之外。

### 第六部分：自己动手做一次

实例要 Java 11 或 17，视版本而定；实测这个要一个新一点的 JDK，并且在两个端口上应答：

```
7474   HTTP：一个 REST 接口，以及一个浏览器界面
7687   Bolt：驱动用的二进制协议
```

**光靠 HTTP 接口就够干活，不需要驱动也不需要 shell**，这里的一切就是这么测的：

```bash
# 不带任何凭据，读出它的版本与它公布的端点
curl -s http://127.0.0.1:7474/

# 跑一条语句
curl -s -X POST http://127.0.0.1:7474/db/neo4j/tx/commit \
  -H 'Content-Type: application/json' \
  -d '{"statements":[{"statement":"MATCH (n) RETURN count(n)"}]}'
```

**而读计划时要知道两件事**，两件都在这里花了时间：

`EXPLAIN` 与 `PROFILE` 的结果是放在结果对象的 `plan` 字段里回来的，不是行 —— 所以一个读 `data` 的脚本会看到空结果，然后以为这条查询什么都没返回。

**一个名字写错的配置项会被忽略，而服务端不会有任何抱怨。** 实测，一行写成 `server.default_listen_address` 的设置完全没生效；真正的属性是 `dbms.default_listen_address`，它的默认值是 `localhost`，而只有读生效后的配置才能看出这两者里哪一个说了算：

```
dbms.default_listen_address        = localhost
dbms.connector.bolt.listen_address = :7687
```

**这就是一类错误的通用形状**：一个在文件里看着对的设置被悄悄丢掉、把默认值留在原地。当默认值恰好是更安全的那个时它无害；当不是的时候，它就是一条发现。

### 第七部分：从这些机制推出的安全观念

**HTTP 接口不需要凭据就会应答。** 实测，`GET /` 对一次未认证的请求返回了版本、版本类型和两个端点地址。这是一次小小的信息泄漏，而它有具体用途：它告诉扫描者**该去试哪个版本的漏洞**，而 `dbms.security.http_auth_allowlist` 的存在正是为了划定哪些路径可以在认证之前被访问。

**而 `LOAD CSV` 是一次从服务端内部发起的文件与网络读取。** 实测，它的行为在两个方向上是**不对称的**，而这件事要紧：

| 语句 | 结果 |
|---|---|
| `LOAD CSV FROM 'file:///etc/passwd'` | **被拒** —— 而错误显示路径被重写成了 `/usr/share/neo4j/import/etc/passwd` |
| `LOAD CSV FROM 'http://127.0.0.1:7474/'` | **内容被读到了** |

**`file:///` 的 URL 是在导入目录里解析的**，所以被限制住的是路径、而不是输入被拒绝 —— 开头那个斜杠并不能逃出去。**而 `http://` 的 URL 会去它指向的任何地方取**，这就让这个子句成了一个服务端请求伪造原语，只要数据库服务端能到达就行，包括只有它能到达的地方。在一个用户能影响那个 URL、或者影响包含它的那条查询的部署上，这就是要找的子句。

**Cypher 注入是换了语法的 SQL 注入。** 实测，闭合字符串再加一个条件，返回了那条查询本来绝不打算返回的行。**参数是可用的，而它就是修法**；并且和任何查询语言一样，一条由输入拼出来的语句里出现 `LOAD CSV` 或 `CALL` 这样的子句，比"一个过滤器读得太多"要严重得多。

**认证是一个开关，而默认用户两种情况下都存在。** 实测，第一次启动时日志里记录了按默认值创建一个用户、并要求改密；而实测这套配置把认证关掉了：

```
dbms.security.auth_enabled               = false
dbms.security.auth_max_failed_attempts   = 3
dbms.security.auth_lock_time             = 5s
dbms.security.http_auth_allowlist        = /,/browser.*
```

**所以问题和这部分里每一个存储一样是两个**：认证开没开，以及那个默认用户处理掉没有。它周围的锁定与白名单设置都已经配好了，这让那个开关本身成了唯一会被放着不管的东西。

**而图有一种它自己的泄漏形状。** 关系是数据，所以一个能读的角色就是一个能把整张图摸清的角色：谁认识谁、哪个服务跟哪个服务说话。**当一张图里的关系本身就是敏感的 —— 汇报线、信任路径、服务依赖 —— 敏感的部分在边上，而一个只看节点属性的权限模型会漏掉它。**

### 检测与缓解

- **查认证有没有开，以及默认用户是否还在用默认口令。** 实测，那个账号在首次启动时被创建、并要求改密，而配置决定了到底有没有人会问你要它。
- **对 `LOAD CSV` 访问 `http://` 或 `https://` URL 告警。** 实测那条路是以服务端的网络位置去读远程内容；`file:///` 那种形式被限制在导入目录里，远程那种没有。
- **盯"看起来是绝对路径的 `file:///` URL"。** 实测它们是被相对于导入目录解析的、而不是被拒绝，所以一条看起来能逃出去的路径其实不能 —— 在把一次失败当成"被挡住了"之前，这件事值得知道。
- **把 `CALL` 过程、以及任何动态拼出来的 Cypher 当成需要审的代码路径**，因为过程面很大，实测有几十条。
- **去查生效后的配置，而不是那个文件。** 实测，一个拼错的属性被无视而没有任何抱怨，把默认值留在生效状态；正在运行的服务端对自己设置的看法才是权威。
- **缓解上，用参数而不是转义字符串。** Cypher 有参数，用它们就是实测那次注入的完整修法。
- **把 HTTP 与 Bolt 端口放在不可信网络之外，或者保持认证开着并收紧白名单** —— 实测那个接口对任何能到达它的人都公布版本。
- **有意识地声明唯一性约束，并预期它会校验已有数据。** 实测，在有重复时创建它会失败、并点名冲突的节点，所以它是一次迁移而不是一次声明。
- **为"被用作遍历起点"的那些属性加索引。** 实测，差别一边是"带标签的扫描加一个过滤"、另一边是单次索引查找，而后者才是一次遍历的代价真正所在。
- **把打算查询的关系建出来。** 从没存过的关系无法遍历，所以对一张图来说，评审的问题不是"schema 对不对"，而是"存下来的这张图里含不含应用要问的那些问题"。
- **并且判断边本身是否敏感。** 在一张图里，连接可能就是机密的那部分，而一次只看节点的权限评审会漏掉它们。
