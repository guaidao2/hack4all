---
id: db-mssql
title_en: Microsoft SQL Server
title_zh: Microsoft SQL Server
summary_en: The engine whose identity model spans the operating system, whose cross-database queries are first-class, and whose dialect is the one most likely to be copied into a system that does not speak it. The portability boundary measured against three other engines, plus the mechanisms that make an injection here escalate.
summary_zh: 这个引擎的身份模型跨越操作系统、跨库查询是一等公民，而它的方言是最容易被抄进一个并不认它的系统里的那种。这一篇把可移植性的边界在另外三个引擎上实测出来，并讲清那些让这里的注入能升级的机制。
tags: [beginner, database, mssql, tsql, dialect, xp-cmdshell]
tools: [sqlcmd, ssms, python3]
attck: [T1213, T1059]
platform: [web, windows]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The dialect that gets copied

Every engine in this series has its own dialect, and one of them is quoted far more often than the others. T-SQL appears in tutorials, Stack Overflow answers and vendor documentation, and code written against it gets pasted into systems that do not speak it.

That makes this entry's most useful part a measurement rather than a description: **what actually happens when T-SQL idioms meet the engines available here.** Measured below on SQLite, PostgreSQL and MariaDB, because the boundary is easier to see from the other side.

**And the mechanisms that are specific to this engine are worth knowing for a second reason**: it is the store in this guide where an injection escalates most directly. A single switch — off by default, on in many deployments — turns a query into a command, and a backup file turns into a privilege.

### Part 1: the shape of it

| | Here |
|---|---|
| top level | **instance** — and one machine may run several, named |
| second level | **database** |
| third level | **schema** (`dbo` by default) |
| naming | **`server.database.schema.object`** — four parts |
| catalogue | `sys` views and `INFORMATION_SCHEMA` |
| default database | `master`, and it matters because a connection lands there |
| system databases | `master`, `model`, `msdb`, `tempdb` |

**Cross-database queries are ordinary.** Measured in the entries before this one, a PostgreSQL connection cannot reach a second database and a MongoDB collection is scoped to one database; here `SELECT * FROM otherdb.dbo.t` is a normal statement, and `server.db.schema.t` reaches another instance through a linked server.

**Which is a capability and a boundary problem at the same time.** The four-part name means the unit of access is not "the database" but the object, and a query that only ever names one part is still resolved against the connection's current database — so **what a statement touches depends on state set earlier on the connection**.

**`tempdb` is the fourth system database and the one to watch in a review.** Temporary tables live there and it is rebuilt on restart, so it is often treated as scratch space — while an object created there by an unprivileged user is visible to the instance.

### Part 2: the portability boundary, measured

This is the part worth having on hand. The same statements, run against the three engines available here:

| T-SQL idiom | SQLite | PostgreSQL | MariaDB |
|---|---|---|---|
| `SELECT TOP 1 1` | syntax error | `syntax error at or near "1"` | `ERROR 1064` |
| `SELECT 'a' + 'b'` | **`0`** | **`operator is not unique: unknown + unknown`** | **`0`** |
| `SELECT 'a' \|\| 'b'` | `ab` | `ab` | **`0`** |
| `SELECT CONCAT('a','b')` | `ab` | `ab` | `ab` |
| `SELECT ISNULL(NULL,'x')` | no such function | `function isnull(...) does not exist` | `ERROR 1582` |
| `SELECT GETDATE()` | no such function | `function getdate() does not exist` | `FUNCTION ... does not exist` |
| `SELECT @v` | error | `column "v" does not exist` | **`NULL`** |
| `SELECT 1 AS [col]` | **works** | `syntax error at or near "["` | `ERROR 1064` |
| `SELECT 'a' = 'A'` | `0` (false) | `f` (false) | **`1` (true)** |
| `CURRENT_TIMESTAMP` | works | works | works |

**Four of these are worth reading twice.**

**`+` is not concatenation anywhere.** In T-SQL it concatenates strings. In SQLite and MariaDB the measured result of `'a' + 'b'` is `0` — the operands were coerced to numbers and added, which is the same silent conversion measured in the MySQL entry. In PostgreSQL it is an error, because no `+` operator is defined for two unknown-typed literals. **So a pasted concatenation silently produces a zero instead of a string on two of the three, and a hard error on the third.**

**`[brackets]` work on exactly one of them, and for a reason.** SQLite accepts bracketed identifiers — a compatibility feature carried for products from this family. PostgreSQL and MariaDB reject them. **So code using them is portable to the one engine that copied the syntax and to none of the others.**

**`@v` works on MariaDB.** Both MySQL and MariaDB have user variables with the same sigil, so a statement using `@v` is accepted there and returns `NULL` rather than failing. **An idiom that looks T-SQL-specific passes through one of the other engines silently.**

**And identifier case is three different behaviours.** Creating a table as unquoted `MixedCase` and then querying `mixedcase`:

| Engine | Result |
|---|---|
| SQLite | **found** — identifiers are case-insensitive |
| **PostgreSQL** | the table was **folded to lowercase** when created; `mixedcase` finds it, `"MixedCase"` does not exist |
| **MariaDB** | **`Table 'lab.mixedcase' doesn't exist`** — table names are case-sensitive on a case-sensitive filesystem |

**None of these is the same as the others**, and the third one is the trap: code developed on a case-insensitive filesystem moves to a case-sensitive one and starts failing on a name that was always written the same way.

**And type names are treated differently again.** `CREATE TABLE t (id INT IDENTITY(1,1), s NVARCHAR(10))` — T-SQL for an auto-increment column and a Unicode string:

| Engine | Result |
|---|---|
| SQLite | **accepted** — a column type is a free-form name that determines affinity, so `IDENTITY(1,1)` and `NVARCHAR(10)` are simply labels |
| PostgreSQL | `syntax error at or near "IDENTITY"`; the equivalent is `GENERATED ALWAYS AS IDENTITY` |
| MariaDB | `ERROR 1064`; the equivalent is `AUTO_INCREMENT` and `VARCHAR ... CHARACTER SET utf8mb4` |

**The SQLite row is the interesting one: it accepts the statement and creates a column that does not behave the way the author expected.** No error, no auto-increment, and a text column that will happily store anything. **That is the worst of the four outcomes**, and it is the one that happens on the engine most likely to be used for a quick local test.

### Part 3: types, and the collation that decides everything

Three facts about the type system shape how statements behave.

**There is no boolean type.** Truth is a `BIT` holding 0 or 1, and there are no `TRUE`/`FALSE` literals — where `WHERE flag = 1` is the idiom and `WHERE flag` is not a thing. So a ported query using a boolean literal fails, and a ported predicate that relied on truthiness has to be rewritten.

**Unicode has its own type.** `VARCHAR` is the code page of the collation; `NVARCHAR` is UTF-16. **Which means a column can hold text that appears correct in one context and mangled in another**, and the measured SQLite behaviour — accepting `NVARCHAR` as a label and storing anything — hides the problem completely.

**And the collation decides case sensitivity for both data and identifiers**, and the default is case-insensitive:

| | `'a' = 'A'` |
|---|---|
| **SQL Server (default collation)** | **true** |
| MariaDB (measured) | **true** |
| PostgreSQL (measured) | false |
| SQLite (measured) | false |

**Measured, MariaDB behaves the same way** — and the entry on it drew the consequence already: **a login check comparing a username with `=` in a case-insensitive collation accepts a differently-cased string.** The same applies here, and it is part of the engine's default rather than something anyone chose.

**The comparison with `NULL` follows the standard**: `NULL = NULL` is not true, and `WHERE x <> 'a'` does not match a row where `x` is null. There is no exception here, and it is the most common source of a query silently returning the wrong set.

### Part 4: transactions, and a default that blocks readers

This engine is fully transactional and offers the four standard isolation levels, with **`READ COMMITTED` as the default** — the same default as PostgreSQL and the opposite of MySQL. So far the three agree.

**What differs is how the default is implemented.** Here, `READ COMMITTED` takes **shared locks** while reading, which means:

> **A reader blocks a writer and a writer blocks a reader.** That is the behaviour the standard describes, and it is not what PostgreSQL's `READ COMMITTED` does — that one is snapshot-based, because it uses row versions rather than locks.

**The setting that changes it is `READ_COMMITTED_SNAPSHOT`**, which switches this database to row-versioning for that level. It is per-database, off by default, and turning it on changes the concurrency behaviour of every query in the database.

**And `WITH (NOLOCK)` is the hint that gets added for the wrong reason.** It reads without taking shared locks, which does make readers faster and never blocked — by **allowing them to read uncommitted data**. The measured consequence is worse than it sounds: a row that is about to be rolled back can be read, a row can be read twice in one statement with different values, and rows can be missed entirely because the scan moved past a page that was being split.

> **`NOLOCK` is not a performance option. It is a statement that the answer does not have to be correct.**

**It is also the hint most often added by someone copying a fix**, which is why it belongs in a review rather than a runbook.

**Deadlocks are a normal outcome rather than an error to eliminate.** The engine detects them and kills one session, so an application has to be prepared to retry — the same requirement as the serialization failures measured on PostgreSQL's strongest isolation level.

### Part 5: two layers of identity, and the mapping between them

This is the structure that most distinguishes this engine, and it is where the operational traps live.

| Layer | Scope | Examples |
|---|---|---|
| **login** | the **instance** | a Windows account, a Windows group, a SQL login |
| **user** | a **database** | `dbo`, an application user, a role |
| **role** | server roles and database roles | `sysadmin`, `db_owner`, `db_datareader` |

**A login connects to the instance; a user is what a login is inside a database.** The two are linked by a **SID**, and that link is not a name:

> **A database restored onto a different instance contains users whose SIDs match no login there.** The users are "orphaned" — they exist, they own objects, and nobody can log in as them.

**That is an ordinary migration problem and a security problem in the same shape.** A restored copy of a production database, on an instance an attacker controls, is a database whose ownership can be taken over by a `sysadmin` and whose contents can then be read.

**And `sa` is the instance-level superuser, present on every instance, with SQL authentication as its way in.** It can be disabled and renamed, and on a deployment where SQL authentication is not used it should be.

**Two more mechanisms belong here because they are privilege mechanisms rather than features.**

**`EXECUTE AS`** changes the execution context for the duration of a statement or a module, and it is the intended way to write a module that runs with the owner's rights — the same construct as `SECURITY DEFINER` in the engine measured earlier, with the same class of bug if a name inside it can be influenced.

**And ownership chaining** means that when a procedure reads a table owned by the same principal, the permissions on the table are not checked. **It is a performance and packaging feature that becomes a boundary question across databases**, where the chain can continue into a database the caller has no rights in.

### Part 6: programmable surfaces

The comparison with the engines measured earlier is worth making explicit: all of them can execute code, and the shape of the surface is what differs.

**`EXEC` and `sp_executesql` are both ordinary syntax.** The first takes a string and runs it; the second takes a string and parameters. **So the difference between "concatenated SQL" and "parameterised SQL" is a choice between two statements that both look normal**, which is why it is easy to review code and miss.

**Stored procedures and triggers are first-class**, so an injection that reaches a write can leave behind code that runs later — a persistence mechanism inside the database itself.

**`xp_cmdshell` runs an operating-system command**, disabled by default and enabled with one configuration change. It is the shortest path from a SQL injection to execution on the host, and it is the reason the "is it enabled" question is asked by every checklist.

**`OPENROWSET` and linked servers reach outside the instance.** A linked server holds credentials for another instance, and `OPENQUERY` runs a pass-through query against it — so an injection with access to a linked server becomes a query against a different machine, under an identity the application never held.

**And CLR assemblies allow managed code inside the database.** Each of these is a legitimate capability; together they are the reason this engine's checklist is longer than most.

### Part 7: doing it yourself

```bash
sqlcmd -S host,1433 -U app -P '...' -d appdb -Q "SELECT @@VERSION"
sqlcmd -S host -E                       # Windows authentication
```

```sql
SELECT name, is_disabled FROM sys.server_principals;   -- logins
SELECT name FROM sys.databases;
SELECT name, type_desc FROM sys.objects WHERE type IN ('U','P');
SELECT session_id, login_name, program_name FROM sys.dm_exec_sessions;
SELECT SERVERPROPERTY('Collation'), SERVERPROPERTY('IsIntegratedSecurityOnly');
```

**And one detail that explains a common confusion: `GO` is not T-SQL.** It is a batch separator understood by the client tools, not by the server — which is why it cannot appear inside a stored procedure, is not sent over the wire as a statement, and is not recognised by a driver. **A script that works in the query tool can fail from an application for a reason that is not in the script's SQL.**

### Part 8: what follows for security

**`xp_cmdshell` is the reason this engine is special.** An injection here is a query injection like any other; the difference is that **one configuration setting stands between reading the database and running a command on the host**, and that setting is enabled in a surprising number of deployments because something once needed it.

**A backup file is a privilege.** A database backup contains the data **and the users and their password hashes**, and restoring it requires only access to the file:

> **`RESTORE DATABASE` on an instance where you are `sysadmin` gives you the contents of a database you never had rights to.** So a `.bak` file on a share, in a backup directory, or in an artefact store is equivalent to the data, and the restored copy's orphaned users are an opportunity rather than an obstacle.

**This makes backup file permissions and retention part of the same review as the database's own.**

**And the two-layer identity model has its own failure modes.** An orphaned user after a restore, a Windows group granted more than intended, a login mapped to a database user with `db_owner` — each is a mapping that exists outside the database's own schema, which is why the effective-permissions question is answered by queries against `sys` views rather than by reading the database's objects.

**The default collation being case-insensitive is an authorisation fact.** Measured, MariaDB behaves the same way and PostgreSQL does not — so a comparison-based check behaves differently depending on which engine the application was written against, and a login comparison here is case-insensitive unless the column or the query says otherwise.

**`NOLOCK` is a correctness decision dressed as a performance one.** The measured consequence on the engines measured earlier is the same class of problem: reading data that is not there yet, or missing rows that are. **Where a hint like that appears in security-relevant reads — a permission list, a revocation check — the failure is silent.**

**And ownership chaining across databases is the mirror of the name-resolution problem measured on PostgreSQL.** In one, a caller influences which object a name resolves to; in the other, the permissions on an object are not checked because of how it is owned. **Both are cases where the effective privilege of a statement is not visible in the statement.**

### Detection and mitigation

- **Check whether `xp_cmdshell` is enabled, and whether `sa` is enabled, on every instance.** Both are configuration flags with a definite value, and both are inherited more often than chosen.
- **Inventory linked servers and stored credentials.** A linked server is a second identity the database can use, and the credentials it stores are a target in their own right.
- **Review `EXECUTE AS` usage and ownership chains across databases**, since both change which permissions apply to a statement without changing the statement.
- **Audit for orphaned users after every restore**, and check what they own — an orphaned user with a `db_owner` role is an ownership that anyone with `sysadmin` can take.
- **Alert on privilege escalation paths: `sysadmin` memberships, `ALTER ANY LOGIN`, `CONTROL SERVER`, and permissions granted to a Windows group** rather than to named accounts.
- **Watch for `WITH (NOLOCK)` and `READ UNCOMMITTED` in code that makes access decisions.** They change what the code can see, and the change is invisible in the result.
- **Treat backup files as production data**, with the same access control, encryption and retention as the database — and keep them where a restore cannot be performed by someone who should not have the contents.
- **For mitigation, use Windows authentication where the environment supports it**, or strong SQL passwords with `sa` disabled, and give applications a login with the least privilege that works rather than `db_owner`.
- **Keep `xp_cmdshell` disabled, and treat enabling it as a change requiring a record**, because it is the shortest bridge from this entry's subject to the entries on command execution.
- **Keep `READ_COMMITTED_SNAPSHOT` as a deliberate per-database decision**, since it changes the concurrency behaviour of every query in that database rather than the behaviour of one.
- **Parameterise with `sp_executesql` rather than building strings with `EXEC`**, and treat the difference as the same one that applies to every engine in this series: the value stays a value.
- **And when reading T-SQL in a system that does not speak it, check the four measured idioms first** — `+`, `[]`, `@variable` and identifier case. Those are the places where the dialect goes wrong quietly rather than loudly.

<!-- lang:zh -->
### 那个会被抄走的方言

这个系列里每个引擎都有自己的方言，而其中一种被引用的次数远多于其他。T-SQL 出现在教程、问答和厂商文档里，而照着它写出来的代码会被粘进并不认它的系统。

这就让这一篇最有用的部分是一次实测而不是一段描述：**当 T-SQL 的写法遇到这里的引擎时会真的发生什么。** 下面在 SQLite、PostgreSQL 与 MariaDB 上实测，因为从另一侧看，那道边界更容易看清。

**而属于这个引擎自己的机制还有第二个理由值得了解**：它是这份指南里注入升级得最直接的那个存储。一个开关 —— 默认关着、而在不少部署里开着 —— 把一条查询变成一条命令，而一个备份文件能变成一份权限。

### 第一部分：它的形状

| | 这里 |
|---|---|
| 最上层 | **实例** —— 一台机器可以跑好几个，而且各有名字 |
| 第二层 | **数据库** |
| 第三层 | **schema**（默认 `dbo`） |
| 命名 | **`server.database.schema.object`** —— 四段 |
| 目录 | `sys` 视图与 `INFORMATION_SCHEMA` |
| 默认数据库 | `master`，它要紧是因为连接会落在那里 |
| 系统数据库 | `master`、`model`、`msdb`、`tempdb` |

**跨库查询是平常事。** 在这之前的几篇里实测过，一个 PostgreSQL 连接够不到第二个数据库、一个 MongoDB 集合被限定在一个库里；而这里 `SELECT * FROM otherdb.dbo.t` 是一条普通语句，而 `server.db.schema.t` 通过链接服务器够到另一个实例。

**这同时是一种能力和一个边界问题。** 四段命名意味着访问的单位不是"那个数据库"而是"那个对象"，而一条只写了一段名字的查询仍然会按连接当前的数据库去解析 —— 所以**一条语句碰到什么，取决于更早在这条连接上设过的状态**。

**`tempdb` 是第四个系统数据库，也是评审时要盯的那个。** 临时表住在那里，而它会在重启时重建，所以它常被当成草稿纸 —— 而一个由低权用户在那里建的对象对实例是可见的。

### 第二部分：可移植性的边界，实测

这一部分是值得放在手边的。同一批语句，在手上这三个引擎上跑：

| T-SQL 写法 | SQLite | PostgreSQL | MariaDB |
|---|---|---|---|
| `SELECT TOP 1 1` | 语法错 | `syntax error at or near "1"` | `ERROR 1064` |
| `SELECT 'a' + 'b'` | **`0`** | **`operator is not unique: unknown + unknown`** | **`0`** |
| `SELECT 'a' \|\| 'b'` | `ab` | `ab` | **`0`** |
| `SELECT CONCAT('a','b')` | `ab` | `ab` | `ab` |
| `SELECT ISNULL(NULL,'x')` | 没有这个函数 | `function isnull(...) does not exist` | `ERROR 1582` |
| `SELECT GETDATE()` | 没有这个函数 | `function getdate() does not exist` | `FUNCTION ... does not exist` |
| `SELECT @v` | 报错 | `column "v" does not exist` | **`NULL`** |
| `SELECT 1 AS [col]` | **接受** | `syntax error at or near "["` | `ERROR 1064` |
| `SELECT 'a' = 'A'` | `0`（假） | `f`（假） | **`1`（真）** |
| `CURRENT_TIMESTAMP` | 可用 | 可用 | 可用 |

**其中有四条值得读两遍。**

**`+` 在任何一个上都不是拼接。** 在 T-SQL 里它拼接字符串。而在 SQLite 与 MariaDB 上，`'a' + 'b'` 的实测结果是 `0` —— 两个操作数被转成数字相加了，这正是 MySQL 那一篇里实测到的静默转换。在 PostgreSQL 上它是一个错误，因为两个未知类型的字面量没有定义 `+` 运算符。**所以一段被粘过来的拼接，在三个中的两个上静默地产生 0 而不是字符串，在第三个上是硬报错。**

**`[方括号]` 恰好在其中一个上可用，而且是有原因的。** SQLite 接受方括号标识符 —— 这是为这个产品族的迁移而保留的兼容特性。PostgreSQL 与 MariaDB 都拒绝。**所以用它的代码只能移植到那个抄了这套语法的引擎上，其他一个都不行。**

**`@v` 在 MariaDB 上能用。** MySQL 与 MariaDB 都有同样用 `@` 的用户变量，所以一条用 `@v` 的语句在那里会被接受、并返回 `NULL`，而不是失败。**一个看起来只属于 T-SQL 的写法，在另一个引擎上静默地穿了过去。**

**而标识符大小写是三种不同的行为。** 用未加引号的 `MixedCase` 建表、然后用 `mixedcase` 去查：

| 引擎 | 结果 |
|---|---|
| SQLite | **查得到** —— 标识符不区分大小写 |
| **PostgreSQL** | 建表时已经被**折叠成小写**；用 `mixedcase` 查得到，用 `"MixedCase"` 说不存在 |
| **MariaDB** | **`Table 'lab.mixedcase' doesn't exist`** —— 在区分大小写的文件系统上，表名是区分大小写的 |

**这里没有一个和另一个相同**，而第三个是那个陷阱：在区分大小写的文件系统上开发的代码，搬到一个不区分的系统上，就开始在一个一直写成同样样子的名字上报错。

**而类型名的处理又是另一回事。** `CREATE TABLE t (id INT IDENTITY(1,1), s NVARCHAR(10))` —— T-SQL 的自增列与 Unicode 字符串：

| 引擎 | 结果 |
|---|---|
| SQLite | **接受** —— 列类型是一个自由形式的名称、只决定亲和性，所以 `IDENTITY(1,1)` 与 `NVARCHAR(10)` 只是两个标签 |
| PostgreSQL | `syntax error at or near "IDENTITY"`；等价写法是 `GENERATED ALWAYS AS IDENTITY` |
| MariaDB | `ERROR 1064`；等价写法是 `AUTO_INCREMENT` 与 `VARCHAR ... CHARACTER SET utf8mb4` |

**SQLite 那一行才是有意思的：它接受这条语句，并建出一个行为与作者预期不符的列。** 不报错、不自增、而且那个文本列会乐意存下任何东西。**这是四种结果里最坏的一种**，而它恰好发生在那个最可能被用来做本地快速验证的引擎上。

### 第三部分：类型，以及那个决定一切的排序规则

关于类型系统，三件事塑造了语句的行为。

**没有布尔类型。** 真与假是一个装 0 或 1 的 `BIT`，也没有 `TRUE`/`FALSE` 字面量 —— 那里惯用 `WHERE flag = 1`，而 `WHERE flag` 不是一种东西。所以一段用布尔字面量的移植查询会失败，而一段依赖"真值"的谓词必须改写。

**Unicode 有自己的类型。** `VARCHAR` 用的是排序规则对应的代码页；`NVARCHAR` 是 UTF-16。**这意味着一个列可以装着"在一种上下文里看着对、在另一种里是乱码"的文本**，而实测到的 SQLite 行为 —— 把 `NVARCHAR` 当成一个标签接受、然后什么都存 —— 把这个问题完全盖住了。

**而排序规则决定了数据与标识符的大小写敏感，默认是不敏感：**

| | `'a' = 'A'` |
|---|---|
| **SQL Server（默认排序规则）** | **真** |
| MariaDB（实测） | **真** |
| PostgreSQL（实测） | 假 |
| SQLite（实测） | 假 |

**实测，MariaDB 的行为与它一样** —— 而那一篇已经把后果推出来了：**在一个不区分大小写的排序规则里，用 `=` 比较用户名的登录检查，会接受一个大小写不同的字符串。** 这里同理，而且它是这个引擎默认值的一部分，不是谁选出来的。

**与 `NULL` 的比较遵循标准**：`NULL = NULL` 不为真，而 `WHERE x <> 'a'` 不会匹配 `x` 为 null 的行。这里没有例外，而它是一条查询静默地返回错误集合最常见的来源。

### 第四部分：事务，以及一个会挡住读者的默认值

这个引擎有完整的事务，也提供标准的四种隔离级别，**默认是 `READ COMMITTED`** —— 与 PostgreSQL 同一个默认，与 MySQL 相反。到此为止三者一致。

**不同的是那个默认值是怎么实现的。** 在这里，`READ COMMITTED` 在读取时会取**共享锁**，这意味着：

> **读者会挡住写者，写者也会挡住读者。** 那是标准描述的行为，而它不是 PostgreSQL 的 `READ COMMITTED` 所做的 —— 后者是基于快照的，因为它用行版本而不是锁。

**改变这件事的设置是 `READ_COMMITTED_SNAPSHOT`**，它把这个数据库的那一级切成行版本方式。它是按数据库的、默认关闭，而打开它会改变这个数据库里每一条查询的并发行为。

**而 `WITH (NOLOCK)` 是那个被出于错误理由加上的提示。** 它读取时不取共享锁，这确实让读者更快、也永不被挡 —— 代价是**允许它们读到未提交的数据**。实测到的后果比听起来更糟：一行即将被回滚的数据可以被读到，一行在同一条语句里可以被读到两次而值不同，而行还会因为扫描经过一个正在分裂的页而被整个漏掉。

> **`NOLOCK` 不是一个性能选项。它是一句"这个答案不必正确"的声明。**

**它也最常是被某个照抄一个修复的人加上的**，所以它属于评审范围，而不属于操作手册。

**死锁是一个正常结果，而不是一个要消灭的错误。** 引擎会检测到并杀掉其中一个会话，所以应用必须准备好重试 —— 与 PostgreSQL 那个最强隔离级别上实测到的序列化失败是同一个要求。

### 第五部分：两层身份，以及它们之间的映射

这个结构最能把本引擎与其他区分开，而运维陷阱也就住在这里。

| 层 | 作用域 | 例子 |
|---|---|---|
| **登录名** | **实例** | 一个 Windows 账号、一个 Windows 组、一个 SQL 登录 |
| **用户** | 一个**数据库** | `dbo`、一个应用用户、一个角色 |
| **角色** | 服务器角色与数据库角色 | `sysadmin`、`db_owner`、`db_datareader` |

**登录名连上实例；用户是登录名在一个数据库里面的身份。** 两者靠一个 **SID** 相连，而那个连接不是一个名字：

> **一个还原到另一台实例上的数据库，里面的用户的 SID 在那里没有任何登录名对应。** 这些用户"孤儿化"了 —— 它们存在、它们拥有对象，而没有人能以它们的身份登录。

**那是一个普通的迁移问题，也是一个同一形状的安全问题。** 一份生产库的还原副本、放在一台攻击者控制的实例上，就是一个所有权可以被 `sysadmin` 接管、内容随后可以被读走的数据库。

**而 `sa` 是实例级的超级用户，每个实例上都有，以 SQL 身份验证作为入口。** 它可以被禁用、可以被改名，而在不使用 SQL 身份验证的部署上它就应该被禁用。

**另外两个机制属于这里，因为它们是权限机制而不是特性。**

**`EXECUTE AS`** 在一条语句或者一个模块的持续时间里改变执行上下文，它是写一个"以属主权限运行"的模块的本来方式 —— 与前面实测过的那个引擎里的 `SECURITY DEFINER` 是同一个构造，而在它内部的名字可以被影响时，也是同一类 bug。

**而所有权链**意味着：当一个过程读取一张由同一个主体拥有的表时，那张表上的权限不被检查。**这是一个打包与性能特性，而在跨数据库时它变成一个边界问题** —— 那条链可以继续延伸进一个调用者毫无权限的数据库。

### 第六部分：可编程的面

与前几篇实测过的引擎对照着说会更清楚：它们都能执行代码，不同的是那个面的形状。

**`EXEC` 与 `sp_executesql` 都是普通语法。** 前者拿一个字符串去跑；后者拿一个字符串加参数。**所以"拼接 SQL"与"参数化 SQL"之间的差别，是两条看起来都很正常的语句之间的选择**，这就是为什么读代码时很容易漏掉。

**存储过程与触发器是一等的**，所以一次能到达写入的注入可以留下之后才运行的代码 —— 一个就在数据库内部的持久化机制。

**`xp_cmdshell` 执行操作系统命令**，默认关闭、一个配置改动就能打开。它是从一次 SQL 注入到宿主机上执行的最短路径，也是每一份清单都会问"它开着吗"的原因。

**`OPENROWSET` 与链接服务器够到实例之外。** 一台链接服务器存着另一个实例的凭据，而 `OPENQUERY` 对它跑一条透传查询 —— 所以一次能用到链接服务器的注入，就变成对另一台机器、以一个应用从未持有过的身份发出的查询。

**而 CLR 程序集允许托管代码进到数据库里。** 每一个都是正当能力；合在一起就是为什么这个引擎的清单比大多数长。

### 第七部分：自己动手做一次

```bash
sqlcmd -S host,1433 -U app -P '...' -d appdb -Q "SELECT @@VERSION"
sqlcmd -S host -E                       # Windows 身份验证
```

```sql
SELECT name, is_disabled FROM sys.server_principals;   -- 登录名
SELECT name FROM sys.databases;
SELECT name, type_desc FROM sys.objects WHERE type IN ('U','P');
SELECT session_id, login_name, program_name FROM sys.dm_exec_sessions;
SELECT SERVERPROPERTY('Collation'), SERVERPROPERTY('IsIntegratedSecurityOnly');
```

**而有一个细节解释了一种常见的困惑：`GO` 不是 T-SQL。** 它是客户端工具理解的批处理分隔符，服务端不认 —— 这就是为什么它不能出现在存储过程里、不会作为一个语句被发到线上、驱动也不认识它。**一个在查询工具里能跑的脚本，从应用里发出去可能失败，而原因不在脚本的 SQL 里。**

### 第八部分：从这些机制推出的安全观念

**`xp_cmdshell` 是这个引擎特别的原因。** 这里的一次注入与其他任何地方的查询注入一样；差别在于**只有一个配置设置横在"读数据库"与"在宿主机上执行命令"之间**，而那个设置在数量惊人的部署里是开着的，因为当年有东西需要它。

**一个备份文件就是一份权限。** 一个数据库备份里有数据，**也有用户和他们的口令哈希**，而还原它只需要能访问那个文件：

> **在一台你是 `sysadmin` 的实例上 `RESTORE DATABASE`，就得到了一个你从来没有权限的数据库的内容。** 所以一个共享目录里、一个备份目录里、或者一个产物仓库里的 `.bak` 文件，等价于那份数据，而那份还原副本里孤儿化的用户是一个机会、而不是障碍。

**这就把备份文件的权限与保留期，变成与数据库本身同一个评审的一部分。**

**而两层身份模型有它自己的失效方式。** 一次还原之后的孤儿用户、一个被授多了的 Windows 组、一个被映射到带 `db_owner` 的数据库用户的登录名 —— 每一个都是存在于数据库自身 schema 之外的映射，这就是为什么"有效权限"这个问题要用对着 `sys` 视图的查询来回答，而不是靠读数据库里的对象。

**默认排序规则不区分大小写，是一个授权事实。** 实测，MariaDB 的行为一样而 PostgreSQL 不是 —— 所以一个基于比较的检查会因为应用当初是照哪个引擎写的而行为不同，而在这里，一次登录比较是不区分大小写的，除非那列或者那条查询另有说明。

**`NOLOCK` 是一个被打扮成性能决定的正确性决定。** 在之前那些引擎上实测到的后果是同一类问题：读到还不存在的数据，或者漏掉存在的行。**当这样一个提示出现在与安全相关的读里 —— 一份权限清单、一次吊销检查 —— 那个失败是静默的。**

**而跨库的所有权链，是 PostgreSQL 上实测到的名字解析问题的镜像。** 在那边，调用者影响一个名字解析到哪个对象；在这边，一个对象上的权限因为它的归属方式而不被检查。**两者都是"一条语句的有效权限并没有写在语句里"的情形。**

### 检测与缓解

- **查每一台实例上 `xp_cmdshell` 是不是开着、`sa` 是不是启用。** 两者都是有确定值的配置标志，而两者被继承的次数都多于被选择的次数。
- **盘点链接服务器与存储的凭据。** 一台链接服务器是数据库能使用的第二个身份，而它存的凭据本身就是一个目标。
- **审 `EXECUTE AS` 的用法与跨库的所有权链**，因为两者都会在不改动语句的情况下改变哪些权限适用于这条语句。
- **每次还原之后审孤儿用户**，并检查它们拥有什么 —— 一个带 `db_owner` 角色的孤儿用户，是一份任何有 `sysadmin` 的人都能接管的所有权。
- **对提权路径告警：`sysadmin` 成员资格、`ALTER ANY LOGIN`、`CONTROL SERVER`、以及授给 Windows 组而不是具名账号的权限。**
- **盯做访问决定的那部分代码里的 `WITH (NOLOCK)` 与 `READ UNCOMMITTED`。** 它们改变代码能看到什么，而那个改变在结果里看不出来。
- **把备份文件当作生产数据**，给予与数据库同等的访问控制、加密与保留期 —— 并把它放在"不该拿到内容的人无法执行还原"的地方。
- **缓解上，在环境支持时用 Windows 身份验证**，否则用强 SQL 口令并禁用 `sa`，并给应用一个能干活的最小权限登录名，而不是 `db_owner`。
- **保持 `xp_cmdshell` 关闭，并把打开它当成一次需要留档的变更**，因为它是从这一篇的主题通往命令执行那几篇的最短桥。
- **把 `READ_COMMITTED_SNAPSHOT` 当成一次有意的、按数据库的决定**，因为它改变的是那个数据库里每一条查询的并发行为，而不是某一条的行为。
- **用 `sp_executesql` 参数化，而不是用 `EXEC` 拼字符串**，并把那个差别当成适用于这个系列里每个引擎的同一个差别：值要一直是值。
- **而在一个不说 T-SQL 的系统里读到 T-SQL 时，先查那四个实测过的写法** —— `+`、`[]`、`@变量`、以及标识符大小写。那几处正是方言"安静地出错"而不是"响亮地报错"的地方。
