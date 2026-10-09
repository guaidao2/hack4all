---
id: db-cassandra
title_en: Cassandra
title_zh: Cassandra
summary_en: A store designed for writes and availability across many machines, where the data model has to be built from the queries rather than the other way round and where a delete is itself a write. Measured on a live node — the refusal to query outside the partition key, an immutable file appearing on delete, and authentication that is off while a superuser still exists.
summary_zh: 一个为"写入"和"多机可用"而设计的存储：数据模型必须从查询倒推出来，而删除本身是一次写入。在一个真在跑的节点上实测 —— 按键之外查询会被拒绝、删除会新增一个不可变的文件、认证默认关着而超级用户却已经存在。
tags: [beginner, database, cassandra, nosql, distributed, cql, tombstones]
tools: [cqlsh, nodetool, cassandra]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Designed for writes, everywhere at once

The databases before this one in the series are all, in one way or another, one machine's opinion about the data. Cassandra is built on the opposite premise: **many machines, no leader, and a write should succeed even if some of them are down.**

Three decisions follow from that premise, and each one is visible in daily use:

**Data is spread by a partition key**, so knowing where a row lives requires knowing that key — which is why the query language refuses queries it cannot route.

**Consistency is chosen per query**, because "is this value agreed" is a dial rather than a property.

**Storage files are immutable**, so a delete cannot edit anything — it writes a record saying the data is gone.

This entry measures all three, on a live **Cassandra 4.1.0** node started for the purpose and stopped afterwards.

### Part 1: the shape of it

| | Here |
|---|---|
| nodes | **peers; every node can serve every request** |
| a node's role | no primary, no leader, no failover step |
| namespace | keyspace → table → **partition** |
| row identity | **partition key + clustering columns** |
| distribution | rows are spread across nodes by a hash of the partition key |
| redundancy | each keyspace declares a **replication factor** |
| language | CQL, which looks like SQL and is not SQL |

**And the keyspace declares how many copies exist.** Measured, the keyspaces on a fresh node:

| Keyspace | Replication |
|---|---|
| `system`, `system_schema` | `LocalStrategy` (not replicated) |
| `system_auth` | `SimpleStrategy`, factor **1** |
| `system_distributed` | `SimpleStrategy`, factor **3** |
| `system_traces` | `SimpleStrategy`, factor **2** |

**Replication is a per-keyspace property, not a per-cluster one**, and the measured values show that the system's own keyspaces do not agree with each other: authentication data is stored once, tracing data twice, and the distributed table three times — on a cluster that, for this lab, has one node.

**Which is worth noticing before anything else here:** a replication factor is a promise about copies, and on a single-node cluster that promise is not kept. The default for `system_auth` being 1 means **losing that node means losing the accounts**.

### Part 2: the data model comes from the queries

This is the part that decides whether an application works well or badly, and it runs in the opposite direction from a relational one.

Measured, a table with a partition key and a clustering column:

```sql
CREATE TABLE events (
  tenant text, day text, ts timeuuid, kind text, detail text,
  PRIMARY KEY ((tenant, day), ts)
) WITH CLUSTERING ORDER BY (ts DESC);
```

Here `(tenant, day)` is the **partition key** — the pair that decides which nodes hold the row — and `ts` is a **clustering column**, which orders rows within a partition. Measured, querying one partition returned the two rows in reverse time order, as declared, without an `ORDER BY`.

**And a query that cannot be routed to a partition is refused.** Measured:

```sql
SELECT * FROM users WHERE email = 'alice@example.com';
```

```
InvalidRequest: Error from server: code=2200 [Invalid query] message="Cannot execute
this query as it might involve data filtering and thus may have unpredictable
performance. If you want to execute this query despite the performance
unpredictability, use ALLOW FILTERING"
```

**The error does not say "no such index". It says the coordinator would have to look everywhere.** By the same rule, supplying only part of a composite partition key is refused too — measured, `WHERE tenant = 'acme'` on that table gives the same error, because half a partition key does not identify a partition.

**And the two things a relational schema is built on are not in the language at all.** Measured:

| Statement | Result |
|---|---|
| `... FROM users u JOIN events e ON u.id = e.tenant` | **`SyntaxException: line 1:37 mismatched input 'u' expecting EOF`** |
| `ALTER TABLE users ADD CONSTRAINT uq UNIQUE (email)` | **`SyntaxException: line 1:40 mismatched input 'UNIQUE' expecting EOF`** |

**So there is no join and no unique constraint.** The first is because joining means gathering rows from machines that do not know about each other; the second is because enforcing uniqueness needs agreement, and agreement is what a leaderless cluster avoids by default.

**All of which is a design instruction rather than a set of limitations:**

> **In a relational database you model the entities and then add indexes until the queries are fast. Here you start from the queries and build one table per query.**

**Denormalisation is not an optimisation here; it is the schema.** The same data is written to several tables, each shaped for one access path, and keeping them in step is the application's job. `ALLOW FILTERING` exists as an escape hatch and is the thing to look for in a review, because it turns a routed read into a scan of the cluster.

### Part 3: consistency is a dial, per query

Because every node can answer, "did my write take effect" depends on how many replicas have to answer. Measured, the level is a session setting and can be changed for one query:

```
CONSISTENCY QUORUM;      -> Consistency level set to QUORUM.
CONSISTENCY;             -> Current consistency level is ONE.
```

**The relationship that matters is `read + write > replicas`**, which is what makes a quorum read see the last quorum write. And the measured consequence for a small deployment is the one to remember: **on one node with a replication factor of 1, `QUORUM` is 1** — the same as `ONE`. The setting is correct and the topology makes it meaningless, which is why the replication factor belongs next to the consistency level in any review.

**The trade is stated plainly in the model.** `ONE` is fast and may return stale data; `ALL` is consistent and fails if any replica is down; the middle is arithmetic. There is no setting that gives all three.

### Part 4: writes are appends, and the files never change

The reason writes here are cheap is that nothing is modified in place. A write goes to a **commit log** on disk for durability and a **memtable** in memory; when the memtable fills it is flushed as a new, immutable **SSTable**.

**Measured, and it is the clearest evidence of the design.** Inserting rows and flushing produced one file; deleting every row and flushing produced **a second file, while the first was unchanged**:

```
after 300 inserts  : nb-2-big-Data.db   4379 bytes
after 300 deletes  : nb-2-big-Data.db   4379 bytes   (unchanged)
                     nb-3-big-Data.db   2843 bytes   (new)
```

**The delete did not modify anything. It added a file describing what was removed.** Queries then returned zero rows, because the new file's records override the old file's data.

**And durability has a window that is easy to miss.** Measured from the configuration:

```
commitlog_sync: periodic
commitlog_sync_period: 10000ms
```

**In the default `periodic` mode the commit log is flushed every ten seconds**, so a write acknowledged to the client can be lost if the machine dies inside that window. The alternative, `batch`, waits for the flush before acknowledging. **This is a deliberate availability-for-durability trade in the default configuration**, and it is the kind of default that a deployment inherits without deciding.

### Part 5: deletion leaves a record, for days

The consequence of immutable files: a delete cannot remove anything, so it **writes a tombstone** — a record with a timestamp saying the data at that position is gone. Queries skip what a tombstone covers, and a background **compaction** merges files and eventually discards both the tombstone and the data it covered.

**How long it waits is a table property, `gc_grace_seconds`, whose built-in default is 864000 seconds — ten days.** In this node's `cassandra.yaml` the setting is not present at all, so that default is in force. Measured, the deletion above produced its own SSTable rather than changing the existing one.

**Three consequences, all of them practical:**

**Deleted data is still on disk for the grace period.** For anything with a retention promise — a token, personal data, a secret — "we deleted it" means "we wrote a tombstone", and the bytes are in an SSTable until compaction passes. **The measured pattern of immutable files plus a ten-day grace period is the mechanism**, and it applies to backups of those files as well.

**Tombstones cost reads.** A partition with many of them gets slower to read, because each one has to be considered and skipped. A workload that inserts and deletes heavily can degrade its own read latency, which is a genuine operational trap.

**And `gc_grace_seconds` exists for a reason worth knowing**: it is how long a tombstone is kept so that it can be propagated before the data it deletes is compacted away — otherwise a replica that was offline would resurrect deleted rows.

### Part 6: transactions, bounded by the partition

There is no general transaction. What exists is defined by what can be coordinated cheaply.

**A single partition** is stored together, so a batch within one partition is atomic — and that is the only place atomicity is free.

**Across partitions**, the tool is a lightweight transaction: Paxos, four round trips, and a cost to be paid deliberately. Measured:

| Statement | Result |
|---|---|
| `INSERT INTO users (id,name) VALUES ('u1','duplicate') IF NOT EXISTS` | **`[applied] False`** — and the existing row was returned |
| `INSERT INTO users (id,name) VALUES ('u9','new') IF NOT EXISTS` | **`[applied] True`** |
| reading `u1` afterwards | unchanged, `alice` |

**`[applied]` is the answer to a compare-and-set**, and it is the only place CQL reports whether a write happened. Which makes the boundary of this database's consistency easy to state: **a conditional write on one row is available; a transaction over several is not**, and the design intent is that you arrange your data so you do not need one.

### Part 7: doing it yourself

Two startup details that cost time if unknown:

```bash
export JAVA_HOME=/usr/lib/jvm/java-11-openjdk-amd64
MAX_HEAP_SIZE=1G HEAP_NEWSIZE=200M cassandra -R -f
```

**It refuses to run as root without `-R`**, and it wants Java 8 or 11 — not the newest JDK on the machine. On the version measured, `512M` of heap produced heavy garbage collection and `1G` was stable.

```sql
DESCRIBE KEYSPACES;  DESCRIBE TABLE events;
SELECT * FROM system_schema.keyspaces;      -- replication per keyspace
SELECT * FROM system_auth.roles;            -- who exists
TRACING ON;                                 -- per-query path through the cluster
```

```bash
nodetool status          # is the ring up
nodetool tablestats ks.t # sizes and tombstone counters
nodetool flush ks        # force memtables to SSTables
```

**And one measured warning to expect from ordinary-looking queries:**

```
SELECT count(*) FROM lab.tombstones;
  -> 0 rows, plus: Warnings: Aggregation query used without partition key
```

**The engine answered and told you it had to look everywhere to do it.** That warning is the cheapest available signal that a query is not routed, and it is easy to have in logs without anyone reading it.

### Part 8: what follows for security

**Authentication is off, and a superuser exists anyway.** Measured from the configuration:

```
authenticator: AllowAllAuthenticator
authorizer:    AllowAllAuthorizer
```

**And from the data:**

| role | can_login | is_superuser |
|---|---|---|
| `cassandra` | true | **true** |

**So a fresh installation has a superuser account whose well-known default password is the same as its name, and a configuration that does not ask anyone for it.** The account matters the moment authentication is switched on, because it is the one that exists; the `AllowAll` settings matter until then, because they mean the account is not consulted at all.

**Which gives the review two questions with a definite answer each:** what are `authenticator` and `authorizer` set to, and does the installation still have the default superuser with its default password. Both are measurable from a configuration file and one query, and both are inherited unchanged far more often than they are chosen.

**`ALLOW FILTERING` is a denial-of-service in a query.** Measured, the server refuses an unrouted query by default and performs it on request. On a real cluster that request is a scan coordinated across nodes, and an endpoint that passes a user's conditions through to CQL can be asked to perform one repeatedly. **Finding it in application code is a review item, not a runtime one**, because there is no server setting that forbids it.

**And `count(*)` without a partition key is the same shape.** Measured, the engine warns. Treating a warning as acceptable in a code review is how a cheap-looking endpoint becomes a cluster-wide scan.

**Tombstones are a security-relevant fact about deletion.** Measured, a delete writes a new file rather than changing an old one, and the record it writes is retained by default for ten days. For a personal-data deletion request or an incident where a token must be gone, **the honest answer is not "it is deleted" but "it is tombstoned and will be compacted away after the grace period"** — and the same statement applies to every backup of those SSTables taken in the meantime.

**The commit log window is a durability question with a security reading.** Measured, the default acknowledges writes before they are flushed, with a ten-second window. For an audit log, a session revocation, or anything where "the write succeeded" is being relied on for a security decision, the mode belongs in the review.

**And replication factors are a data-protection decision.** Measured, `system_auth` defaults to one copy. Loosening that for availability and tightening it for confidentiality are the same dial, and the per-keyspace values are visible in one query.

### Detection and mitigation

- **Check `authenticator` and `authorizer`, and then check whether the default superuser still exists.** Measured, a fresh node allows everything without credentials and contains a superuser role; both are configuration facts rather than runtime ones.
- **Audit for `ALLOW FILTERING` in application code and for `count(*)` without a partition key.** They are the measured shapes of a query that leaves the routed path, and neither has a server-side switch.
- **Watch tombstone counts per table**, because they slow reads and they indicate a delete-heavy workload. A rising count with falling performance is the pattern.
- **Decide `commitlog_sync` deliberately where a write is a security event.** The default acknowledges before flushing within a ten-second window.
- **Review replication factors against what they are protecting.** Authentication data at one copy is a measured default, and losing that node loses the accounts.
- **For mitigation, set `PasswordAuthenticator` and `CassandraAuthorizer`, then change the default superuser's password and create per-application roles.** Leaving the well-known account in place is the finding, not the setup.
- **Keep the node off untrusted networks until those two settings are changed.** The measured configuration needs no credentials at all.
- **Make the data model route every query**, and where an access pattern genuinely needs a different shape, add a table for it rather than an `ALLOW FILTERING` clause.
- **Use prepared statements for anything user-supplied**, as with any query language, and treat `ALLOW FILTERING`, `USING TTL` and batch clauses as part of the statement rather than as data.
- **Choose consistency levels with the replication factor in hand**, and write both into the deployment's documentation, since neither is meaningful alone.
- **And treat deletion as a promise with a delay.** For data that must actually stop existing, the controls are the table's grace period, a forced compaction, and the retention of every backup of those files.

<!-- lang:zh -->
### 为"到处都在写"而设计

这个系列里在这个之前的所有数据库，多少都是"一台机器对数据的看法"。Cassandra 建在一个相反的预设上：**很多台机器、没有主、而且就算其中一些挂了，写入也应当成功。**

由这个预设推出三个决定，而每一个在日常使用里都看得见：

**数据按分区键散开**，所以要知道某一行在哪里，就必须知道那个键 —— 这就是为什么这门查询语言会拒绝它无法路由的查询。

**一致性是按查询选的**，因为"这个值是否已达成一致"是一个旋钮，而不是一个属性。

**存储文件是不可变的**，所以删除无法修改任何东西 —— 它写下一条"数据没了"的记录。

这一篇在一个为此启动、用完即停的 **Cassandra 4.1.0** 节点上实测这三件事。

### 第一部分：它的形状

| | 这里 |
|---|---|
| 节点 | **对等的；每个节点都能服务任何请求** |
| 节点的角色 | 没有主、没有 leader、没有切换步骤 |
| 名字空间 | keyspace → table → **partition** |
| 行的身份 | **分区键 + 聚类列** |
| 分布 | 行按键的哈希散到各节点 |
| 冗余 | 每个 keyspace 自己声明**复制因子** |
| 语言 | CQL，长得像 SQL，而不是 SQL |

**而 keyspace 自己声明存在几份副本。** 实测，一个全新节点上的 keyspace：

| Keyspace | 复制 |
|---|---|
| `system`、`system_schema` | `LocalStrategy`（不复制） |
| `system_auth` | `SimpleStrategy`，因子 **1** |
| `system_distributed` | `SimpleStrategy`，因子 **3** |
| `system_traces` | `SimpleStrategy`，因子 **2** |

**复制是按 keyspace 的属性，不是按集群的属性**，而实测值显示系统自己的那些 keyspace 之间也不一致：认证数据只存一份，追踪数据两份，分布式表三份 —— 而在这个实验里，集群只有一个节点。

**这一点值得在别的一切之前先注意到**：复制因子是一句关于副本的承诺，而在单节点集群上那句承诺没有被兑现。`system_auth` 默认是 1，意味着**丢掉那个节点就丢掉了账号**。

### 第二部分：数据模型来自查询

这是决定一个应用好用还是难用的部分，而它的方向与关系型相反。

实测，一张带分区键与聚类列的表：

```sql
CREATE TABLE events (
  tenant text, day text, ts timeuuid, kind text, detail text,
  PRIMARY KEY ((tenant, day), ts)
) WITH CLUSTERING ORDER BY (ts DESC);
```

这里 `(tenant, day)` 是**分区键** —— 决定这一行由哪些节点持有的那一对 —— 而 `ts` 是一个**聚类列**，它给分区内的行排序。实测，查询一个分区返回了两行、按声明的时间倒序，而且没有写 `ORDER BY`。

**而一个无法路由到某个分区的查询会被拒绝。** 实测：

```sql
SELECT * FROM users WHERE email = 'alice@example.com';
```

```
InvalidRequest: Error from server: code=2200 [Invalid query] message="Cannot execute
this query as it might involve data filtering and thus may have unpredictable
performance. If you want to execute this query despite the performance
unpredictability, use ALLOW FILTERING"
```

**这个错误说的不是"没有那个索引"，而是"协调节点将不得不到处看"。** 按同一条规则，只给复合分区键的一部分也会被拒 —— 实测，对那张表用 `WHERE tenant = 'acme'` 得到同样的错误，因为半个分区键不标识任何一个分区。

**而关系型 schema 赖以建立的那两样东西，在这门语言里根本不存在。** 实测：

| 语句 | 结果 |
|---|---|
| `... FROM users u JOIN events e ON u.id = e.tenant` | **`SyntaxException: line 1:37 mismatched input 'u' expecting EOF`** |
| `ALTER TABLE users ADD CONSTRAINT uq UNIQUE (email)` | **`SyntaxException: line 1:40 mismatched input 'UNIQUE' expecting EOF`** |

**所以既没有 join，也没有唯一约束。** 前者是因为 join 意味着从彼此互不知情的机器上把行聚到一起；后者是因为强制唯一性需要达成一致，而"达成一致"恰恰是一个无主集群默认要避开的东西。

**这一切是一条设计指令，而不是一组限制：**

> **在关系库里，你先把实体建模，然后加索引直到查询变快。在这里，你从查询出发，为每个查询建一张表。**

**反范式在这里不是优化，它就是 schema。** 同一份数据被写进几张表，每一张为一个访问路径塑形，而让它们保持同步是应用的活儿。`ALLOW FILTERING` 作为一条逃生通道存在，而它正是评审时要找的东西，因为它把一次"路由到的读"变成一次横跨集群的扫描。

### 第三部分：一致性是一个旋钮，按查询拧

因为每个节点都能回答，"我这次写生效了吗"取决于有多少副本作了答。实测，级别是一个会话设置，可以为一条查询单独改：

```
CONSISTENCY QUORUM;      -> Consistency level set to QUORUM.
CONSISTENCY;             -> Current consistency level is ONE.
```

**要紧的关系是 `读 + 写 > 副本数`**，正是它让一次 quorum 读能看到最近一次 quorum 写。而对小部署来说，实测出的那个后果才是该记住的：**在一个节点、复制因子为 1 的集群上，`QUORUM` 就是 1** —— 和 `ONE` 一样。设置是对的，而拓扑让它失去意义，这就是为什么复制因子必须和一致性级别放在一起评审。

**这笔取舍在模型里说得很直白。** `ONE` 快、可能返回旧数据；`ALL` 一致、任何一个副本挂了就失败；中间那段是算术。没有任何一个设置能同时给出三样。

### 第四部分：写入是追加，而文件从不改变

写入在这里便宜的原因是：没有任何东西被就地修改。一次写落到磁盘上的 **commit log**（为了持久性）和内存里的 **memtable**；memtable 满了就被刷成一个新的、不可变的 **SSTable**。

**实测，而这是那个设计最清楚的证据。** 插入若干行并 flush 产生了一个文件；删掉每一行再 flush 产生了**第二个文件，而第一个没有任何变化**：

```
插入 300 行之后 : nb-2-big-Data.db   4379 字节
删除 300 行之后 : nb-2-big-Data.db   4379 字节   （没变）
                  nb-3-big-Data.db   2843 字节   （新增）
```

**那次删除没有修改任何东西。它加了一个描述"什么被删掉了"的文件。** 之后查询返回零行，因为新文件里的记录覆盖了旧文件里的数据。

**而持久性有一个容易忽略的窗口。** 从配置里实测：

```
commitlog_sync: periodic
commitlog_sync_period: 10000ms
```

**在默认的 `periodic` 模式下，commit log 每十秒刷一次**，所以一条已经被确认给客户端的写，如果机器在这个窗口内死了，就可能丢。另一个选项 `batch` 会在确认之前等刷盘完成。**这是默认配置里一次有意的"用持久性换可用性"**，也是那种被部署原样继承、而没有被决定的默认值。

### 第五部分：删除留下一条记录，留好几天

不可变文件的后果是：删除无法移除任何东西，所以它**写一条墓碑** —— 一条带时间戳、说明那个位置的数据已经没了的记录。查询会跳过墓碑覆盖的东西，而后台的 **compaction** 会合并文件、并最终把墓碑和它覆盖的数据一起丢掉。

**它等多久是一个表属性 `gc_grace_seconds`，内建默认是 864000 秒 —— 十天。** 在这个节点的 `cassandra.yaml` 里这个设置根本没出现，所以生效的就是那个默认值。实测，上面那次删除产生的是它自己的 SSTable，而不是改动已有的那个。

**三个后果，都很实际：**

**被删的数据在宽限期内仍在磁盘上。** 对任何带保留期承诺的东西 —— 一个令牌、个人数据、一个秘密 —— "我们删掉了"的意思是"我们写了一条墓碑"，而那些字节在一个 SSTable 里，直到 compaction 经过。**实测到的"文件不可变 + 十天宽限期"就是这个机制**，而它同样适用于那些 SSTable 的每一份备份。

**墓碑让读变贵。** 一个有很多墓碑的分区读起来更慢，因为每一条都得被考虑并跳过。一个大量插入又大量删除的工作负载会拖慢自己的读延迟，那是一个真实的运维陷阱。

**而 `gc_grace_seconds` 存在是有理由的、这个理由值得知道**：它是一条墓碑被保留多久，好让它能被传播出去，再去压掉它所删除的数据 —— 否则一个离线过的副本会把已删除的行复活。

### 第六部分：事务，以分区为界

这里没有通用的事务。存在的东西由"什么能被便宜地协调"来定义。

**单个分区**被存在一起，所以一个分区内的批量是原子的 —— 而那是原子性唯一免费的地方。

**跨分区**时，工具是轻量事务：Paxos、四次往返、以及一个要有意付出的代价。实测：

| 语句 | 结果 |
|---|---|
| `INSERT INTO users (id,name) VALUES ('u1','duplicate') IF NOT EXISTS` | **`[applied] False`** —— 并把已有的行返回了 |
| `INSERT INTO users (id,name) VALUES ('u9','new') IF NOT EXISTS` | **`[applied] True`** |
| 之后读 `u1` | 没变，`alice` |

**`[applied]` 就是一次比较并交换的答案**，而它是 CQL 里唯一会报告"这次写有没有发生"的地方。这就让这个数据库的一致性边界很好陈述：**对一行的条件写是可用的；跨若干行的事务不是**，而设计意图是让你把数据安排好，从而不需要它。

### 第七部分：自己动手做一次

两个不知道就会浪费时间的启动细节：

```bash
export JAVA_HOME=/usr/lib/jvm/java-11-openjdk-amd64
MAX_HEAP_SIZE=1G HEAP_NEWSIZE=200M cassandra -R -f
```

**不加 `-R` 它拒绝以 root 运行**，而它要 Java 8 或 11 —— 不是机器上最新的那个 JDK。在实测这个版本上，`512M` 堆产生了很重的垃圾回收，`1G` 才稳定。

```sql
DESCRIBE KEYSPACES;  DESCRIBE TABLE events;
SELECT * FROM system_schema.keyspaces;      -- 每个 keyspace 的复制配置
SELECT * FROM system_auth.roles;            -- 有哪些角色
TRACING ON;                                 -- 看一条查询穿过集群的路径
```

```bash
nodetool status          # 环起来了吗
nodetool tablestats ks.t # 大小与墓碑计数
nodetool flush ks        # 强制把 memtable 刷成 SSTable
```

**还有一条看起来普通的查询会给出的实测警告：**

```
SELECT count(*) FROM lab.tombstones;
  -> 0 rows，另加：Warnings: Aggregation query used without partition key
```

**引擎回答了，并告诉你它为了回答不得不到处看。** 那条警告是"一条查询没有被路由"最便宜可得的信号，而它很容易就存在于日志里、没人读。

### 第八部分：从这些机制推出的安全观念

**认证是关的，而超级用户照样存在。** 从配置里实测：

```
authenticator: AllowAllAuthenticator
authorizer:    AllowAllAuthorizer
```

**而从数据里：**

| 角色 | 可登录 | 是超级用户 |
|---|---|---|
| `cassandra` | 是 | **是** |

**所以一个全新安装有一个超级用户账号，它那个众所周知的默认口令与它的名字相同，而配置又根本不问任何人要凭据。** 这个账号在认证被打开的那一刻就要紧，因为它是唯一已经存在的那个；而那两个 `AllowAll` 设置在那一刻之前要紧，因为它们意味着这个账号压根不会被查。

**这就让评审有了两个各自有确定答案的问题：** `authenticator` 与 `authorizer` 被设成了什么，以及这个安装是否还留着那个默认超级用户和它的默认口令。两个都能从一个配置文件和一条查询里测到，而两者被原样继承、而不是被选择的次数，远多于被选择。

**`ALLOW FILTERING` 是一条查询里的拒绝服务。** 实测，服务端默认拒绝未路由的查询、并在被要求时执行它。在一个真集群上，那个请求是一次跨节点协调的扫描，而一个把用户的过滤条件直接透传成 CQL 的端点，可以被反复要求执行一次。**在应用代码里找到它是一件评审事项，不是运行时事项**，因为没有任何服务端设置能禁止它。

**而没有分区键的 `count(*)` 是同一个形状。** 实测，引擎会警告。在代码评审里把警告当作可接受，就是一个看起来便宜的端点变成一次全集群扫描的方式。

**墓碑是一个与安全有关的、关于删除的事实。** 实测，一次删除写一个新文件而不是改一个旧文件，而它写下的那条记录默认被保留十天。对于一次个人数据删除请求、或者一次必须让某个令牌消失的事件，**诚实的答案不是"已经删除了"，而是"它已经被墓碑标记，会在宽限期之后被压掉"** —— 而同一句话也适用于这期间那些 SSTable 的每一份备份。

**commit log 那个窗口是一个持久性问题，带一个安全读法。** 实测，默认在刷盘之前就确认写入，窗口是十秒。对一份审计日志、一次会话吊销、或者任何"写入成功"被当作安全决定依据的地方，那个模式属于评审范围。

**而复制因子是一个数据保护决定。** 实测，`system_auth` 默认只有一份。为了可用性放松它、为了机密性收紧它，拧的是同一个旋钮，而每个 keyspace 的值一条查询就能看到。

### 检测与缓解

- **查 `authenticator` 与 `authorizer`，然后查那个默认超级用户还在不在。** 实测，一个全新节点不凭任何凭据就允许一切，并且里面有一个超级用户角色；两者都是配置事实而不是运行时事实。
- **在应用代码里审 `ALLOW FILTERING`、以及没有分区键的 `count(*)`。** 它们是实测到的"离开路由路径"的形状，而两者都没有服务端开关。
- **盯每张表的墓碑计数**，因为它们让读变慢、也表明有一个重删除的工作负载。计数上升而性能下降，就是这个形状。
- **在"一次写入就是一个安全事件"的地方，有意识地决定 `commitlog_sync`。** 默认在十秒窗口内、刷盘之前就确认。
- **拿复制因子与它在保护的东西对起来看。** 认证数据只有一份是一个实测到的默认值，而丢掉那个节点就丢掉了账号。
- **缓解上，设 `PasswordAuthenticator` 与 `CassandraAuthorizer`，然后改掉默认超级用户的口令、并按应用创建角色。** 把那个众所周知的账号留着才是那条发现，而不是安装本身。
- **在这两个设置改掉之前，别把节点放在不可信网络上。** 实测那套配置完全不需要凭据。
- **让数据模型路由每一条查询**，而某个访问模式确实需要另一种形状时，为它加一张表，而不是加一个 `ALLOW FILTERING` 子句。
- **对任何来自用户的输入用预处理语句**，和在任何查询语言里一样，并把 `ALLOW FILTERING`、`USING TTL`、批量子句当成语句的一部分、而不是数据。
- **手里拿着复制因子去选一致性级别**，并把两者都写进部署文档，因为单独看哪一个都没有意义。
- **并且把删除当成一个有延迟的承诺。** 对必须真正不再存在的数据，控制手段是那张表的宽限期、一次强制 compaction、以及那些文件每一份备份的保留策略。
