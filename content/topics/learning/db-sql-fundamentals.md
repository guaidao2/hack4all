---
id: db-sql-fundamentals
title_en: SQL from the Outside
title_zh: SQL 入门
summary_en: SQL is the one common language where you describe the result and the engine decides the procedure, and that division of labour explains both its power and most of the trouble afterwards. Measured — what an index buys, what a function on a column costs, and what NULL and floating point quietly do.
summary_zh: SQL 是唯一一门常见的语言：你描述要什么，引擎决定怎么拿。这个分工既说明了它为什么好用，也说明了之后大部分麻烦的来源。这一篇实测三件事 —— 索引买到了什么、对列做函数要付多少代价、以及 NULL 与浮点会安静地做什么。
tags: [beginner, database, sql, sqlite, indexing, transactions]
tools: [sqlite3, mysql, psql, python3]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The one declarative language most people use

Programmers spend their day writing procedures: do this, then that, and handle the case where the third thing fails. SQL is the opposite, and it is unusual enough to be worth stating plainly.

> **In SQL you describe the result you want, and the engine decides how to get it.** You do not say which index to walk, in what order to join, or whether to scan at all.

That division of labour is why SQL survived every database fashion: the same query keeps working when the data grows, when an index is added, or when the engine gets a better planner. And it is also why the language has properties that surprise people — an execution order different from the written order, behaviour that depends on what the engine chose, and a set of silent traps around `NULL` and types.

Two things are worth saying before the mechanics, because they shape everything after.

**The database is usually the last boundary an application has.** Application servers are many and replaceable; the database holds the data. Whatever an attacker can reach through the application eventually meets this layer, and what happens next depends on what this layer will let the application's account do.

**And it is the layer most often treated as a black box.** Applications that carefully validate input will happily build SQL by string concatenation, because the language reads like English and it is easy to forget that it is also a program.

### Part 1: from files to relations

Storing data in a file works until two things happen at once. Reading a file is easy; **writing it from two places is not** — one process overwrites the other's changes, and a crash halfway through leaves a half-written file. Then queries arrive: "which orders over 500 were placed last week" turns into loading everything and filtering in code.

A relational database answers both by being a program that owns the file and offers a language for it:

| It gives you | Which means |
|---|---|
| concurrent access | many readers and writers without corrupting anything |
| atomicity | a group of changes either all happen or none do |
| a query language | filtering, joining and aggregating without moving data to the application |
| constraints | rules the database enforces, not rules the application remembers |
| indexes | a way to find rows without reading all of them |

The model itself is smaller than it looks:

| Term | Meaning |
|---|---|
| **relation** (table) | a set of rows with the same columns |
| **tuple** (row) | one record |
| **attribute** (column) | a named field with a type |
| **primary key** | the column(s) that identify a row uniquely |
| **foreign key** | a column whose value must exist as a key in another table |

**The important part is not the tables, it is the relations.** Two rows are connected because a value in one matches a value in the other — not because one contains a pointer to the other. That is what makes a join a query-time decision: the engine can reach the same records from either direction, and it can pick whichever is cheaper for the query at hand.

### Part 2: three kinds of statement

SQL is usually divided by what a statement acts on, and the division matters because the risks are different in each:

| Kind | Statements | Acts on | The risk |
|---|---|---|---|
| **DDL** — definition | `CREATE`, `ALTER`, `DROP` | the structure | a user-supplied value must never decide the shape of a table |
| **DML** — manipulation | `SELECT`, `INSERT`, `UPDATE`, `DELETE` | the rows | this is where user input arrives, and where parameterisation matters |
| **DCL** — control | `GRANT`, `REVOKE` | who may do what | this decides how bad everything else can get |

**DCL is the one beginners skip and attackers care about most.** The account an application connects with is the ceiling on every other failure: an account that may only `SELECT` on one schema turns an injection into a read; an account that may `DROP`, or read files, or connect to other databases, turns the same injection into something much larger.

### Part 3: SELECT, and the order things actually happen

A query is written in one order and executed in another, and most "why does this not work" questions live in that gap.

| Written order | | Execution order |
|---|---|---|
| `SELECT` | | `FROM` |
| `FROM` | | `WHERE` |
| `WHERE` | | `GROUP BY` |
| `GROUP BY` | | `HAVING` |
| `HAVING` | | `SELECT` |
| `ORDER BY` | | `ORDER BY` |
| `LIMIT` | | `LIMIT` |

**`WHERE` runs before grouping and before `SELECT`**, which explains two things at once. Measured, an aggregate in `WHERE` is refused outright:

```
SELECT COUNT(*) FROM orders WHERE COUNT(*) > 400
  -> OperationalError: misuse of aggregate: COUNT()
```

There is no group to filter yet at that point, so filtering groups is `HAVING`, and filtering rows is `WHERE`. And because `SELECT` has not been evaluated when `WHERE` runs, an alias defined in `SELECT` should not be visible there either — though engines differ:

| Engine | `SELECT amount*2 AS d ... WHERE d > 100` |
|---|---|
| SQLite | **accepted** — an extension beyond the standard |
| Most others | refused |

**That difference is worth knowing for a practical reason**: the same query can work in development and fail in production if the two use different engines, and the error will look like a syntax problem rather than a portability one.

The rest of the vocabulary is smaller than it appears. `JOIN` combines rows from two tables on a condition; `INNER` keeps only matches, `LEFT` keeps every row from the left and fills the rest with `NULL`, `CROSS` pairs everything with everything. `GROUP BY` collapses rows into groups and the aggregates (`COUNT`, `SUM`, `AVG`, `MIN`, `MAX`) summarise each group. `UNION` stacks result sets; `WITH` names a subquery so it can be referenced twice; a subquery in `FROM` is a table for the duration of the query.

### Part 4: constraints, and what a transaction is

**Constraints are the database doing what the application cannot be trusted to do consistently.**

| Constraint | Enforces |
|---|---|
| `PRIMARY KEY` | uniqueness and non-null, per row |
| `UNIQUE` | no two rows share a value |
| `NOT NULL` | a value must be present |
| `CHECK` | an expression must hold |
| `FOREIGN KEY` | a referenced row must exist |
| `DEFAULT` | what happens when nothing is supplied |

**The reason to put a rule here rather than in application code is arity.** An application has many ways in — the web front end, the API, a batch job, an admin script, a second service, a migration, someone at a prompt. The database has one. A rule in application code is a rule that has to be remembered by every one of those paths; a constraint is enforced for all of them at once.

**A transaction is the same idea applied to a group of statements.** Measured, subtracting from one account and rolling back leaves the balance untouched, while two statements committed together move both or neither:

```
BEGIN; UPDATE acct SET bal = bal - 30 WHERE id = 1; ROLLBACK;
  account 1: 100    account 2: 50        (unchanged)

BEGIN; UPDATE ... -30 ...; UPDATE ... +30 ...; COMMIT;
  account 1: 70     account 2: 80        (total unchanged)
```

The four properties that are usually listed as ACID are worth restating in plain terms, because the interesting one is the third:

| Property | Means |
|---|---|
| Atomicity | all of it or none of it |
| Consistency | constraints hold before and after |
| **Isolation** | concurrent transactions do not see each other's half-finished work |
| Durability | once committed, it survives a crash |

**Isolation is where the surprises live**, and it is not absolute: the standard defines levels that trade correctness against concurrency, from "read uncommitted" up to "serializable". A read followed by a write based on it is not atomic unless both are inside the same transaction, and at lower isolation levels two transactions can interleave in ways that produce results neither would have produced alone. That is the mechanism behind the race condition class: **the database gives you atomicity for the statements you group, and nothing for the ones you do not.**

### Part 5: indexes, or why the same query can differ by two orders of magnitude

An index is a sorted structure the database maintains alongside the table, so it can find rows by a value without reading all of them. Measured, on 200,000 rows, counting how many orders belong to one customer:

```
no index on customer :  5.00 ms   SCAN orders
with idx_customer    :  0.03 ms   SEARCH orders USING COVERING INDEX idx_customer (customer=?)
primary key lookup   :            SEARCH orders USING INTEGER PRIMARY KEY (rowid=?)
```

**A hundred-and-sixty-fold difference, and the plan says why**: `SCAN` reads every row, `SEARCH ... USING INDEX` reads only the matching part of a sorted structure. (The word `COVERING` means the index itself contained everything the query needed, so the table was never touched at all.)

**And there is a condition that is easy to violate by accident.** Measured, three queries selecting exactly the same 16,667 rows:

| Condition | Time | Plan |
|---|---|---|
| `created >= '2026-03-01' AND created < '2026-04-01'` | **0.36 ms** | `SEARCH ... USING INDEX idx_created` |
| `substr(created, 1, 7) = '2026-03'` | 9.32 ms | `SCAN orders` |
| `CAST(substr(created, 6, 2) AS INTEGER) = 3` | 11.04 ms | `SCAN orders` |

**Same rows, same result, and the first one is roughly thirty times faster.** The reason is one sentence:

> **An index remembers the values in a column, not the results of a function applied to them.**

Once the condition is written as a function of the column, the engine has to compute that function for every row before it can compare anything, and the sorted order it maintained is useless. The fix is to keep the column bare on one side of the comparison — which is why ranges are preferred to functions, and why storing a derived value in its own column is sometimes worth the redundancy.

Two more facts about indexes that come up constantly:

**A composite index is usable from its left edge.** An index on `(customer, created)` helps `WHERE customer = ?`, helps `WHERE customer = ? AND created > ?`, and does not help `WHERE created > ?` alone.

**Every index costs something.** It has to be updated on every write and it occupies space, so the goal is not "index everything" but "index what the queries actually filter and sort on" — which is a question the query plan answers better than intuition.

### Part 6: seeing it happen yourself

SQLite is the fastest way to try all of this, because it is one file and one command with nothing to install:

```bash
sqlite3 /tmp/demo.db
```

```sql
CREATE TABLE orders (id INTEGER PRIMARY KEY, customer TEXT, amount INTEGER, created TEXT);
CREATE INDEX idx_customer ON orders(customer);
-- how does it plan to find this?
EXPLAIN QUERY PLAN SELECT * FROM orders WHERE customer = 'cust-42';
```

**`EXPLAIN QUERY PLAN` is the single most useful habit in this entry.** It is the database telling you what it intends to do, and the one word to look for is `SCAN`:

```
SCAN orders                                             <- every row will be read
SEARCH orders USING INDEX idx_customer (customer=?)     <- only the matching part
SEARCH orders USING INTEGER PRIMARY KEY (rowid=?)       <- direct lookup by key
```

When a query is slow, the plan is the first thing to read, because it distinguishes "the database chose a bad way to get this" from "this query genuinely has to look at everything".

Measured, on the same 200,000 rows, three things that surprise people:

**Writing row by row is dominated by commits, not by SQL.** Twenty thousand rows inserted with a commit after each took **37.6 ms**; the same rows in a single transaction took **9.8 ms** — 3.8 times faster, with identical statements. The cost was never the `INSERT`, it was making the transaction durable twenty thousand times.

**`NULL` is a state, not a value.** On a four-row table where two rows have a `NULL` in `b`:

```
COUNT(*)                    -> 4     counts rows
COUNT(b)                    -> 2     NULL is ignored by the aggregate
WHERE b = NULL              -> 0     never true
WHERE b IS NULL             -> 2
WHERE b != 'x'              -> 1     the NULL rows are not "not equal to x" either
WHERE b != 'x' OR b IS NULL -> 3
```

**A comparison with `NULL` is unknown rather than false**, so it excludes the row from the result — silently, without an error. A filter that forgets an `IS NULL` therefore loses rows quietly, which is the worst way to lose them.

**Floating point is floating point.** `0.1 + 0.2` is `0.30000000000000004`, in a database as much as in any other language, and adding twice through a `REAL` column keeps the error:

```
SELECT 0.1 + 0.2                   -> 0.30000000000000004
REAL column, 0.1 + 0.2             -> 0.30000000000000004
NUMERIC column, same arithmetic    -> 0.30000000000000004   (stored type: real)
integer cents, 10 + 20             -> 30
```

**And the type situation differs between engines, which is the important part.** SQLite has no fixed-point type: `NUMERIC` is an affinity, and the value above was stored as `real`. PostgreSQL's `NUMERIC` and MySQL's `DECIMAL` are true fixed-point and do not drift. So "use a decimal type for money" is only actionable once you know whether the engine has one — and if it does not, the answer is to store integer cents.

**Deep paging gets slower the further you go.** Selecting one page of twenty rows, ordered by customer:

| Page | `OFFSET` | Time |
|---|---|---|
| 1 | 0 | 0.03 ms |
| 1,000 | 19,980 | 0.45 ms |
| 5,000 | 99,980 | **1.78 ms** |
| 5,000 (keyset) | — | **0.05 ms** |

`OFFSET` means "produce that many rows and throw them away", so the work grows with the page number — and it does so **even when the index is used**, because the index scan still has to walk past everything skipped. Pagination by key — `WHERE (customer, id) > (last row of the previous page)` — starts after that key and costs about the same on page 1 and page 5,000.

### Part 7: the traps everyone walks into

| Trap | What happens |
|---|---|
| `WHERE col = NULL` | no rows, no error — `IS NULL` is the comparison you meant |
| An aggregate in `WHERE` | refused; aggregate in `HAVING` instead |
| `UPDATE` or `DELETE` with no `WHERE` | every row |
| Money in a float column | values that do not add up |
| `DELETE` vs `TRUNCATE` vs `DROP` | rows, all rows and reset, the table itself |
| `COUNT(*)` vs `COUNT(col)` | the second ignores `NULL` |
| String comparison and case | depends on the column's collation, which differs per engine and per column |
| Implicit type conversion | `'1' = 1` may or may not match, and the answer changes the plan |
| Timestamps without a zone | two servers, two answers |
| `LIMIT` with no `ORDER BY` | which rows you get is undefined |

The last one deserves a sentence: a query without an `ORDER BY` has no defined order, so "the first ten rows" is a statement the engine is free to satisfy differently each time — and adding an index can change which rows those are.

### Part 8: what follows for security

Every mechanism above has a security reading, and most of them are the same fact seen from the other side.

**A declarative language executed by string concatenation is an injection.** The engine must know where the query ends and the data begins, and if both arrive as one string built in the application, nothing in the engine can tell them apart. Parameterisation is not a filter over the string; it is the mechanism that keeps them separate — which is why it holds where escaping and denylists fail.

**Constraints and application validation are not alternatives.** They reject different things: a constraint refuses to store a row that breaks an invariant, and application validation refuses a request that should not have been made. The first survives a new code path; the second can explain the refusal to a user. Both are needed, and the constraint is the backstop.

**The application's database account is the blast radius.** Every finding in the injection family is measured, in practice, by what that account may do — which is why the account is a security control and not a deployment detail.

**Isolation level is a correctness setting with a security reading.** The race condition class exists because a read and the write based on it were not in one transaction, and no amount of application-level checking replaces grouping them.

**And the query plan is visibility in both directions.** A slow-query log and a plan are how you find the query that scans a table it should not; they are also where someone else's unusual query shows up, because a database is one of the few places where an attacker's behaviour produces a legible artifact.

### Detection and mitigation

- **Watch the slow-query log, and set the threshold low enough that a table scan is visible.** The measured gap between a scan and a search on the same query is two orders of magnitude, so the signal is hard to miss once the threshold is anywhere near it.
- **Alert on plans that changed shape.** A query that used an index yesterday and scans today means a function was wrapped around a column, an index was dropped, or the data distribution crossed a threshold the planner cares about. The plan is small enough to store and compare.
- **Alert on the statement types each account issues.** An account that only ever issues `SELECT` and one day issues `UNION`, `INTO OUTFILE`, or a DDL statement is the strongest single indicator available for injection, because legitimate code does not change statement classes by accident.
- **Alert on errors that suggest probing.** Syntax errors, type mismatches and "unknown column" in production traffic are usually somebody constructing queries by hand.
- **And watch query duration and rows examined per account.** The DoS reading of this layer is a query that expands a join or a sort into something enormous, and it is visible as resources before it is visible as an outage.
- **For mitigation, give the application's account only the DML verbs it uses, on only the schemas it needs.** No DDL, no cross-database access, no file privileges. This is the control that decides how much any other failure is worth.
- **Parameterise every query, and take it as a rule rather than a judgement call.** A prepared statement, an ORM's parameter binding, or a query builder — anything where the data travels separately from the statement.
- **Put the invariants in constraints, not only in application code.** Uniqueness, non-null, ranges and referential integrity, enforced where every code path must pass.
- **Group related reads and writes in one transaction, and pick an isolation level deliberately.** The default is a trade-off, not a guarantee, and a read-then-write pair outside a transaction has no atomicity at all.
- **Index what the queries filter and sort on, and check the plan rather than guessing.** A missing index is a slow query today and a denial-of-service vector when a query is under someone else's control.
- **Set statement timeouts and resource limits.** A query the application would never write is still a query the database will run, and a join without a bound is cheap to send and expensive to answer.
- **And practise restoring from a backup.** Every control above reduces the chance of needing it; none of them replaces having done it once.

<!-- lang:zh -->
### 大多数人在用的唯一一门声明式语言

程序员一天到晚在写过程：先做这个，再做那个，第三步失败时怎么办。SQL 是相反的，而它独特到值得明说：

> **在 SQL 里，你描述想要的结果，引擎决定怎么拿到它。** 你不说走哪个索引、以什么顺序连接、要不要全表扫描。

这个分工就是 SQL 活过每一次数据库潮流的原因：同一条查询在数据变大时、加了索引时、引擎换了更好的查询规划器时，都继续能用。它也是这门语言里那些让人意外之处的原因 —— 执行顺序与书写顺序不同、行为取决于引擎怎么选、以及围绕 `NULL` 与类型的一堆安静陷阱。

机制之前有两件事要先说，因为后面的一切都由它们决定。

**数据库通常是应用最后一道边界。** 应用服务器可以有很多、也可以换掉；数据在数据库里。攻击者通过应用能碰到的东西，最终都会走到这一层，而接下来发生什么，取决于这一层允许应用那个账号做什么。

**而它也是最常被当成黑盒的一层。** 那些会仔细校验输入的应用，会毫不犹豫地用字符串拼 SQL，因为这门语言读起来像英语，人容易忘记它同时也是一段程序。

### 第一部分：从文件到关系

把数据存进一个文件，在发生两件事之前是可行的。读文件很容易；**从两个地方写它不容易** —— 一个进程会覆盖另一个的改动，而中途崩溃会留下一个写了一半的文件。然后查询来了："上周下单的、金额超过 500 的订单有哪些"，变成了把所有东西加载进来、在代码里过滤。

关系数据库对这两件事的回答是：它自己是一个拥有那个文件的程序，并提供一门语言来操作它：

| 它给你 | 意味着 |
|---|---|
| 并发访问 | 很多读和写，而不会互相破坏 |
| 原子性 | 一组改动要么全发生，要么全不发生 |
| 一门查询语言 | 过滤、连接、聚合，不用把数据搬到应用里 |
| 约束 | 由数据库执行的规则，而不是应用要记住的规则 |
| 索引 | 一种不必读所有行就能找到行的办法 |

模型本身比看上去小：

| 术语 | 含义 |
|---|---|
| **关系**（表） | 一组列相同的行 |
| **元组**（行） | 一条记录 |
| **属性**（列） | 一个有名字、有类型的字段 |
| **主键** | 唯一标识一行的列 |
| **外键** | 它的值必须是另一张表里某个键的值 |

**重要的不是表，是"关系"。** 两行之所以有联系，是因为一行的某个值与另一行的某个值相等 —— 而不是因为一行里存着指向另一行的指针。这正是"连接是查询时的决定"的原因：引擎可以从任何一个方向到达同一批记录，也可以为手上的这条查询挑更便宜的那条路。

### 第二部分：三类语句

SQL 常按"它作用在什么上"来分，而这个分法要紧，因为每一类的风险不同：

| 类别 | 语句 | 作用在 | 风险 |
|---|---|---|---|
| **DDL** 定义 | `CREATE`、`ALTER`、`DROP` | 结构 | 用户提供的值绝不能决定一张表的形状 |
| **DML** 操作 | `SELECT`、`INSERT`、`UPDATE`、`DELETE` | 行 | 用户输入在这里到达，参数化在这里要紧 |
| **DCL** 控制 | `GRANT`、`REVOKE` | 谁能做什么 | 它决定其他一切出事之后能有多严重 |

**DCL 是新手会跳过、而攻击者最在意的那一类。** 应用连库用的那个账号，是其他一切失效的上限：一个只被允许对某个 schema 做 `SELECT` 的账号，把一次注入变成一次读取；而一个能做 `DROP`、能读文件、能连其他库的账号，把同一次注入变成大得多的事情。

### 第三部分：SELECT，以及实际发生的顺序

一条查询的书写顺序和它的执行顺序不一样，而大部分"这句为什么不对"的问题就住在这个缝里。

| 书写顺序 | | 执行顺序 |
|---|---|---|
| `SELECT` | | `FROM` |
| `FROM` | | `WHERE` |
| `WHERE` | | `GROUP BY` |
| `GROUP BY` | | `HAVING` |
| `HAVING` | | `SELECT` |
| `ORDER BY` | | `ORDER BY` |
| `LIMIT` | | `LIMIT` |

**`WHERE` 在分组之前、在 `SELECT` 之前执行**，这一句同时解释了两件事。实测，在 `WHERE` 里用聚合会被直接拒绝：

```
SELECT COUNT(*) FROM orders WHERE COUNT(*) > 400
  -> OperationalError: misuse of aggregate: COUNT()
```

那一刻还没有"组"可以过滤，所以过滤组要用 `HAVING`，过滤行要用 `WHERE`。而同样因为 `WHERE` 执行时 `SELECT` 还没求值，在 `SELECT` 里定义的别名本不该在那里可见 —— 不过各引擎不一样：

| 引擎 | `SELECT amount*2 AS d ... WHERE d > 100` |
|---|---|
| SQLite | **接受** —— 超出标准的扩展 |
| 多数其他引擎 | 拒绝 |

**这个差别有一个实用后果**：开发和生产如果换了引擎，同一条查询会一个能跑一个报错，而那个错误看起来像语法问题，而不是可移植性问题。

剩下的词汇比看上去少。`JOIN` 按一个条件把两张表的行合起来；`INNER` 只留匹配的，`LEFT` 保留左边所有行、其余用 `NULL` 补，`CROSS` 是两两相配。`GROUP BY` 把行收成组，聚合函数（`COUNT`、`SUM`、`AVG`、`MIN`、`MAX`）对每组做汇总。`UNION` 把结果集叠起来；`WITH` 给一个子查询起名字，好让它被引用两次；`FROM` 里的子查询在这条查询期间就是一张表。

### 第四部分：约束，以及事务是什么

**约束是数据库在做"应用没法被信任去一致地做"的那件事。**

| 约束 | 它保证 |
|---|---|
| `PRIMARY KEY` | 每行唯一且非空 |
| `UNIQUE` | 没有两行共享同一个值 |
| `NOT NULL` | 值必须存在 |
| `CHECK` | 某个表达式必须成立 |
| `FOREIGN KEY` | 被引用的行必须存在 |
| `DEFAULT` | 什么都不给时怎么办 |

**把规则放在这里而不是放在应用代码里的理由，是"入口的数量"。** 一个应用有很多条进来的路 —— Web 前端、API、批处理、管理脚本、第二个服务、数据库迁移、以及某个人在命令行上。数据库只有一条。写在应用代码里的规则，是一条必须被上面每一条路都记住的规则；而一个约束会一次性地替它们全部执行。

**事务是同一个想法用在"一组语句"上。** 实测：从一个账户扣钱然后回滚，余额没有变化；而两条语句一起提交时，两条要么都生效要么都不生效：

```
BEGIN; UPDATE acct SET bal = bal - 30 WHERE id = 1; ROLLBACK;
  账户 1: 100    账户 2: 50        （没有变化）

BEGIN; UPDATE ... -30 ...; UPDATE ... +30 ...; COMMIT;
  账户 1: 70     账户 2: 80        （合计不变）
```

通常被列成 ACID 的那四个性质值得用大白话重说一遍，因为有意思的是第三个：

| 性质 | 含义 |
|---|---|
| 原子性 | 要么全做，要么全不做 |
| 一致性 | 约束在前后都成立 |
| **隔离性** | 并发事务看不到彼此做到一半的工作 |
| 持久性 | 一旦提交，崩溃也丢不掉 |

**意外都住在隔离性里**，而它不是绝对的：标准定义了几个级别，在正确性与并发之间做取舍，从"读未提交"到"可串行化"。一次读、以及基于它的写，只有在同一个事务里才是原子的；而在较低的隔离级别下，两个事务可以交错成"单独跑都不会产生"的结果。这正是竞态那一类的机制：**数据库只为你归组在一起的那些语句提供原子性，没归组的那些一点都没有。**

### 第五部分：索引，或者同一条查询为什么会差两个数量级

索引是数据库在表旁边维护的一个有序结构，于是它不必读所有行就能按值找到行。实测，在 20 万行上数某个客户有多少订单：

```
customer 上没有索引 :  5.00 ms   SCAN orders
有 idx_customer     :  0.03 ms   SEARCH orders USING COVERING INDEX idx_customer (customer=?)
主键查找            :            SEARCH orders USING INTEGER PRIMARY KEY (rowid=?)
```

**一百六十倍的差距，而执行计划说明了原因**：`SCAN` 读每一行，`SEARCH ... USING INDEX` 只读有序结构里匹配的那一段。（`COVERING` 这个词表示索引本身已经含有这条查询需要的一切，表根本没被碰。）

**而有一个条件很容易被无意破坏。** 实测，三条查询筛出完全相同的 16,667 行：

| 条件 | 耗时 | 计划 |
|---|---|---|
| `created >= '2026-03-01' AND created < '2026-04-01'` | **0.36 ms** | `SEARCH ... USING INDEX idx_created` |
| `substr(created, 1, 7) = '2026-03'` | 9.32 ms | `SCAN orders` |
| `CAST(substr(created, 6, 2) AS INTEGER) = 3` | 11.04 ms | `SCAN orders` |

**同样的行、同样的结果，第一条快大约三十倍。** 理由是一句话：

> **索引记住的是列里的值，不是对列做运算之后的结果。**

一旦条件写成"对列做函数"，引擎就必须先为每一行算出那个函数值才能比较，而它维护的那个有序顺序用不上了。修法是让比较的一边保持是裸的列 —— 这就是为什么范围比较优于函数，也是为什么把一个派生值单独存成一列有时值得那份冗余。

还有两个关于索引的事实会不断出现：

**复合索引从最左边开始可用。** 一个 `(customer, created)` 上的索引对 `WHERE customer = ?` 有用、对 `WHERE customer = ? AND created > ?` 有用、而对单独的 `WHERE created > ?` 没用。

**每个索引都有代价。** 它要在每次写入时更新、还要占空间，所以目标不是"给所有东西加索引"，而是"给查询真正用来过滤和排序的那几列加索引" —— 而这个问题，执行计划比直觉答得好。

### 第六部分：自己把它看见

SQLite 是试这一切最快的办法，因为它就是一个文件加一条命令，什么都不用装：

```bash
sqlite3 /tmp/demo.db
```

```sql
CREATE TABLE orders (id INTEGER PRIMARY KEY, customer TEXT, amount INTEGER, created TEXT);
CREATE INDEX idx_customer ON orders(customer);
-- 它打算怎么找？
EXPLAIN QUERY PLAN SELECT * FROM orders WHERE customer = 'cust-42';
```

**`EXPLAIN QUERY PLAN` 是这一篇里最有用的一个习惯。** 它是数据库在告诉你它打算怎么做，而要看的就是那个词 `SCAN`：

```
SCAN orders                                             <- 每一行都会被读
SEARCH orders USING INDEX idx_customer (customer=?)     <- 只读匹配的那一段
SEARCH orders USING INTEGER PRIMARY KEY (rowid=?)       <- 直接按键查
```

一条查询慢的时候，执行计划是第一个要看的东西，因为它把"数据库选了一条糟糕的路"和"这条查询本来就非看全部不可"区分开了。

实测，在同一批 20 万行上，三件让人意外的事：

**逐行写入的开销在提交上，不在 SQL 上。** 两万行、每行提交一次是 **37.6 ms**；同样的行放在一个事务里是 **9.8 ms** —— 快 3.8 倍，语句完全相同。成本从来不是 `INSERT`，而是让那个事务落盘两万次。

**`NULL` 是一个状态，不是一个值。** 在一张四行的表上，两行的 `b` 是 `NULL`：

```
COUNT(*)                    -> 4     数行
COUNT(b)                    -> 2     NULL 被聚合忽略
WHERE b = NULL              -> 0     永远不成立
WHERE b IS NULL             -> 2
WHERE b != 'x'              -> 1     NULL 那两行也"不等于 x"不成立
WHERE b != 'x' OR b IS NULL -> 3
```

**与 `NULL` 的比较结果是"未知"而不是"假"**，所以它会把那一行排除在结果之外 —— 安静地，不报错。一个忘了写 `IS NULL` 的过滤条件因此会静悄悄地少返回行，而这是丢行最糟的一种方式。

**浮点就是浮点。** `0.1 + 0.2` 是 `0.30000000000000004`，在数据库里和在任何其他语言里一样，而通过 `REAL` 列加两次会把误差留下：

```
SELECT 0.1 + 0.2                   -> 0.30000000000000004
REAL 列，0.1 + 0.2                 -> 0.30000000000000004
NUMERIC 列，同样运算                -> 0.30000000000000004   （存储类型是 real）
整数分，10 + 20                    -> 30
```

**而类型这件事各引擎不同，这才是要紧的部分。** SQLite 没有定点类型：`NUMERIC` 是一种亲和性，上面那个值是按 `real` 存的。PostgreSQL 的 `NUMERIC`、MySQL 的 `DECIMAL` 是真正的定点、不会漂。所以"金额用定点类型"这句话，只有在知道那个引擎有没有定点类型之后才能执行 —— 如果没有，答案是用整数存分。

**翻得越深越慢。** 取一页 20 行、按客户排序：

| 页码 | `OFFSET` | 耗时 |
|---|---|---|
| 1 | 0 | 0.03 ms |
| 1,000 | 19,980 | 0.45 ms |
| 5,000 | 99,980 | **1.78 ms** |
| 5,000（keyset） | — | **0.05 ms** |

`OFFSET` 的语义是"取出这么多行然后丢掉"，所以工作量随页码增长 —— 而且**即使用了索引也一样**，因为索引扫描仍然要走过被跳过的那些。按键分页 —— `WHERE (customer, id) > (上一页最后一行)` —— 从那个键之后开始，第 1 页和第 5,000 页的代价差不多。

### 第七部分：每个人都会踩的坑

| 坑 | 会发生什么 |
|---|---|
| `WHERE col = NULL` | 没有行，也不报错 —— 你想写的是 `IS NULL` |
| 在 `WHERE` 里用聚合 | 被拒绝；聚合放 `HAVING` |
| 没有 `WHERE` 的 `UPDATE` 或 `DELETE` | 每一行 |
| 金额放在浮点列里 | 加起来不对的数 |
| `DELETE` 与 `TRUNCATE` 与 `DROP` | 删行、全删并重置、删掉表本身 |
| `COUNT(*)` 与 `COUNT(col)` | 后者忽略 `NULL` |
| 字符串比较与大小写 | 取决于列的排序规则，而它按引擎、按列都不同 |
| 隐式类型转换 | `'1' = 1` 成立与否不定，而答案还会影响执行计划 |
| 不带时区的时间戳 | 两台服务器，两个答案 |
| 没有 `ORDER BY` 的 `LIMIT` | 你拿到哪几行没有定义 |

最后一条值得补一句：没有 `ORDER BY` 的查询没有定义顺序，所以"前 10 行"是一个引擎每次都可以用不同方式满足的陈述 —— 而加一个索引就可能改变那 10 行是哪些。

### 第八部分：从这些机制推出的安全观念

上面每一个机制都有一个安全读法，而它们大多是同一个事实从另一面看过去。

**一门声明式语言 + 字符串拼接 = 注入。** 引擎必须知道查询在哪里结束、数据从哪里开始，而如果两者以"应用拼好的一个字符串"的形式一起到达，引擎里没有任何东西能把它们分开。参数化不是对字符串的过滤，它是让两者保持分离的那个机制 —— 这也是它在转义与黑名单失效的地方仍然成立的原因。

**约束与应用校验不是二选一。** 它们拒绝的是不同的东西：约束拒绝把一行破坏不变量的数据存进去，应用校验拒绝一个本不该发出的请求。前者能活过一条新加的代码路径，后者能向用户解释为什么被拒。两者都要，而约束是那个兜底。

**应用连库的那个账号就是波及面。** 注入这一族里每一条发现，在实践中都是用那个账号能做什么来衡量的 —— 这就是为什么那个账号是一项安全控制，而不是一个部署细节。

**隔离级别是一个正确性设置，同时有一个安全读法。** 竞态那一类存在，是因为一次读和基于它的写不在同一个事务里，而再多的应用层检查也替代不了把它们归到一组。

**而执行计划是两个方向上的可见性。** 慢查询日志与执行计划，是你找到"那条本不该扫描全表的查询"的办法；它们也是别人异常查询出现的地方，因为数据库是少数几个"攻击者的行为会留下可读痕迹"的地方之一。

### 检测与缓解

- **盯慢查询日志，并把阈值设得低到能让一次全表扫描显形。** 实测里同一条查询在扫描与查找之间差两个数量级，所以只要阈值大致在那个范围附近，这个信号很难被漏掉。
- **对"执行计划变了形状"告警。** 一条昨天用索引、今天扫描的查询，意味着有人给列套了个函数、索引被删了、或者数据分布越过了规划器在意的某个阈值。执行计划小到可以存下来做比较。
- **对每个账号发出的语句类别告警。** 一个一直以来只发 `SELECT` 的账号某天发了 `UNION`、`INTO OUTFILE` 或一条 DDL，是现有最强的注入指标，因为正当的代码不会碰巧改变语句类别。
- **对"像是探测"的错误告警。** 生产流量里的语法错误、类型不匹配、"未知列"，通常说明有人在手工构造查询。
- **并且盯每个账号的查询时长与扫描行数。** 这一层的拒绝服务读法，是一条把连接或排序膨胀到极大的查询，它在变成一次故障之前，先表现为资源。
- **缓解上，只给应用的账号它用到的那些 DML 动词、只给它需要的那些 schema。** 不给 DDL、不给跨库访问、不给文件权限。这一项控制决定了其他任何失效值多少。
- **把每一条查询参数化，并把它当成规则而不是判断题。** 预编译语句、ORM 的参数绑定、或者一个查询构造器 —— 任何让数据与语句分开走的办法。
- **把不变量放进约束，而不是只放在应用代码里。** 唯一、非空、范围、引用完整性，放在每一条代码路径都必须经过的地方。
- **把相关的读写归到一个事务里，并有意识地挑隔离级别。** 默认值是一个取舍，不是一份保证；而一对"先读再写"放在事务之外，完全没有原子性。
- **给查询真正用来过滤和排序的列加索引，并看执行计划而不是猜。** 缺一个索引今天是慢查询，而当查询落在别人手里时，它是一个拒绝服务的入口。
- **设语句超时与资源限制。** 一条应用永远不会写的查询，仍然是一条数据库会去执行的查询；而一个没有边界的连接，发出去很便宜、答起来很贵。
- **并且练一次从备份恢复。** 上面每一项控制都在降低"需要它"的概率；它们没有一个能替代"已经做过一次"。
