---
id: db-postgresql
title_en: PostgreSQL
title_zh: PostgreSQL
summary_en: The engine that hands many decisions to configuration and roles where others hand them to defaults, and whose transaction semantics differ from MySQL in ways that change how code has to be written. Measured on a live server — isolation levels, abort behaviour, row-level security, and a privilege escalation that turns on name resolution.
summary_zh: 这个引擎把很多决定交给了配置与角色，而别的引擎把它们交给了默认值；它的事务语义与 MySQL 的差别，会改变代码该怎么写。这一篇在一个真在跑的服务上实测 —— 隔离级别、出错之后的行为、行级安全，以及一次靠"名字解析"完成的提权。
tags: [beginner, database, postgresql, sql, mvcc, rls, transactions]
tools: [psql, pg_dump, python3]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The engine that asks you to decide

Two database engines can implement the same standard and still expect different things from you. MySQL's character comes from its **defaults** — an escape character, a collation, a storage engine — things chosen long ago that a deployment inherits. PostgreSQL's character comes from its **configuration surface**: isolation levels, roles, schemas, row-level security, extensions, and a name-resolution path that the caller controls.

> **The practical difference is where you look.** With one you read the documentation to learn what the defaults do; with the other you read the deployment's configuration and roles to learn what was decided.

That is why this entry is mostly about **mechanisms that change how code must be written**, and why several of them are security-relevant by construction rather than by misconfiguration.

Measured on a live **PostgreSQL 18.4**, using one `psql` session per group of related statements, with temporary schemas, roles and tables created and dropped. Every session-dependent result comes from statements that shared a session, because a result measured across separate connections is not a measurement of the thing.

### Part 1: the shape of it

Four structural facts, each of which differs from MySQL:

| | PostgreSQL | MySQL |
|---|---|---|
| **Namespace** | `database` → `schema` → `table` | `database` = `schema` → `table` (two levels) |
| **Name resolution** | governed by **`search_path`** | the current database, plus explicit prefixes |
| **Account** | a **role**, global to the cluster | **`user`@`host`**, per host |
| **Process model** | one backend process **per connection** | one server with a thread per connection |
| **Cross-database queries** | not possible in one connection | possible with a prefix |

**The namespace difference matters more than it sounds.** A connection is to one **database** and cannot reach another; inside that database, objects live in **schemas**, and a name that is not qualified is resolved by walking `search_path`. That is a feature — it lets one database hold `app`, `audit` and `staging` side by side — and it is also the mechanism behind the escalation in Part 8.

**And the role being global while the connection rules live in a file** is the second difference: authentication (who may connect, from where) is `pg_hba.conf`, and authorisation (what they may do) is `GRANT`. Two systems, two places to look. Measured, a fresh role:

| Role | Can log in | Superuser |
|---|---|---|
| `pgprobe_a` | **no** | no |
| `pgprobe_b` | **no** | no |

**A role is not a login by default.** `CREATE ROLE` makes something that can own objects and be granted privileges; `CREATE USER` or `LOGIN` is what makes it able to connect. In MySQL the account *is* the login, and it is scoped to a host.

### Part 2: isolation levels, which decide what a transaction can see

PostgreSQL uses **MVCC**: a reader does not block a writer and a writer does not block a reader, because each transaction sees a **snapshot**. What a snapshot contains depends on the isolation level.

Measured, the server's default:

```
default_transaction_isolation = read committed
```

And the difference between that and `repeatable read`, demonstrated inside one session with a second connection committing rows in between:

| Step | `REPEATABLE READ` | `READ COMMITTED` |
|---|---|---|
| first read in the transaction | `1` | `2` |
| **a different session commits a row** | | |
| second read in the same transaction | **`1`** | **`3`** |
| after `COMMIT` | `2` | — |

**Under `REPEATABLE READ` the transaction did not see the other session's committed row at all** — the snapshot was taken when the transaction began. Under `READ COMMITTED`, each statement takes a fresh snapshot, so the second read saw it.

**And the default differs between engines**, which is the part that changes code:

| Engine | Default isolation |
|---|---|
| **PostgreSQL** | **`READ COMMITTED`** |
| MySQL / InnoDB | `REPEATABLE READ` |

**A read-then-write pair that relies on "nothing changed between my two statements" works by default on MySQL and does not on PostgreSQL.** That is not a defect in either; it is a default, and it is the reason the race condition class exists: **an application that assumes its two statements see the same world has assumed an isolation level it may not have.**

**On this engine there is also a real `SERIALIZABLE`**, implemented as serializable snapshot isolation, and it can abort a transaction that would not be serializable. So the highest level is available and costs retries — which is a design decision rather than a setting to turn on and forget.

### Part 3: after an error, the whole transaction is finished

This is the difference that most often surprises people coming from another engine. Measured:

```sql
BEGIN;
SELECT 1;          -- ok
SELECT 1/0;        -- ERROR: division by zero
SELECT 2;          -- ERROR: current transaction is aborted,
                   --        commands ignored until end of transaction block
ROLLBACK;
SELECT 42;         -- ok
```

**One failed statement poisons the transaction.** Every subsequent statement is refused until it is rolled back, and the message says so explicitly.

**In MySQL the same sequence continues**: the failed statement is skipped and the following ones run. So code that does "try each step, keep going if one fails, commit at the end" behaves completely differently on the two engines — on PostgreSQL it silently does nothing after the first failure, and the `COMMIT` becomes a `ROLLBACK`.

**The practical rule is that a transaction here is a unit of work or it is nothing.** Error handling inside one has to be done with savepoints, or by treating any error as "this transaction is over".

### Part 4: type strictness, and where the coercion happens

Measured:

| Statement | Result |
|---|---|
| `INSERT INTO t (n) VALUES ('123')` where `n` is `integer` | **accepted**, stored as `123` |
| `INSERT INTO t (n) VALUES ('abc')` | **refused** — `invalid input syntax for type integer: "abc"` |
| `SELECT '1' = 1` | **`t`** — true |
| `SELECT 'abc' + 1` | **refused** — `invalid input syntax for type integer` |

**A literal is coerced when the coercion is defined, and refused when it is not.** `'123'` becomes an integer; `'abc'` cannot, so the statement fails rather than guessing. This is the opposite of MySQL, where `'abc' + 1` returns `1` because non-numeric text becomes zero — and it is worth putting the two side by side:

| Expression | PostgreSQL | MySQL | SQLite |
|---|---|---|---|
| `'1' = 1` | true | true | **false** |
| `'abc' + 1` | **error** | `1` | `1` |
| `'abc'` into an integer column | **error** | `0` | **stored as text** |

**Three engines, three answers, none of them wrong.** The practical consequence is that a query containing an implicit conversion fails loudly here, silently elsewhere, or stores something that is not what the column says it is.

### Part 5: identifiers are folded, and names are resolved by a path

Measured:

```
CREATE TABLE pgprobe_Case (x int);       -- unquoted
SELECT count(*) FROM pgprobe_case;       -- -> 0 rows, no error
SELECT count(*) FROM "pgprobe_Case";     -- -> ERROR: relation "pgprobe_Case" does not exist
```

**An unquoted identifier is folded to lower case**, so the table is named `pgprobe_case`. A quoted identifier keeps its case exactly, which is why the second query looks for a different table — one that does not exist.

**And an unqualified name is resolved by `search_path`**, which is a session setting and can be changed by the session. Measured for a fresh low-privilege role:

```
search_path = "$user", public
```

Three things follow, and they are the setup for Part 8:

**A name in a statement is not necessarily a name in a particular schema.** It is resolved by walking a list, and `"$user"` means "a schema named after the current role, if it exists".

**The session can change that list.** `SET search_path` is available to any role, and it is not a privileged operation — which is correct, since it only affects the session's own resolution.

**And anything that executes a name inside another role's privileges inherits that problem**, which is exactly what the next-but-one part measures.

### Part 6: privileges — roles, schemas, and per-row rules

Three layers, and this engine has all three separately.

**Roles are global and can be granted to each other.** A role can own objects, hold privileges, and be a member of other roles; connecting is a separate flag.

**Schema privileges decide who may create and use objects**, and the default for `public` changed. Measured, on this version:

```
public nspacl = {pg_database_owner=UC/pg_database_owner,=U/pg_database_owner}
```

The `=U` entry is **`PUBLIC` (the empty role name) with `U`** — usage, and no create. Measured, a fresh low-privilege role trying to create a table there:

```
ERROR: permission denied for schema public
```

**In older versions `PUBLIC` had `CREATE` on `public`**, so any role could create objects there, and an object created by one role could be resolved by another's unqualified name. That was tightened in PostgreSQL 15 precisely because of the name-resolution behaviour in Part 5.

**And row-level security is built into the engine.** Measured, a table with two tenants' rows, RLS enabled, and a policy comparing a column against `current_user`:

| Session | Rows visible |
|---|---|
| superuser | **2** |
| `SET ROLE pgprobe_a` | **1** — only its own row |

**The filter is applied by the database, not by the query.** A role that queries the table without a `WHERE` clause gets only its rows, which is a different kind of control from "the application always remembers to filter" — and it is the one place in this guide where a multi-tenant isolation rule can be enforced somewhere other than the application.

### Part 7: doing it yourself

```bash
sudo -u postgres psql          # peer authentication: the OS user maps to the role
psql "postgresql://app@host/db"
```

```sql
\l                             -- databases          \du   -- roles
\dn                            -- schemas            \dp   -- privileges
\d table_name                  -- columns and indexes
SHOW default_transaction_isolation;
SELECT current_user, session_user, current_setting('search_path');
EXPLAIN (ANALYZE, BUFFERS) SELECT ... ;
```

**`EXPLAIN` is the habit worth forming.** Measured, on the JSONB example below, the difference between two plans for the same query is one index:

| Plan | Query |
|---|---|
| `Seq Scan` with a filter | `WHERE doc @> '{"a": 7}'` before an index exists |
| `Bitmap Index Scan on pgprobe_js_doc_idx` | the same query after `CREATE INDEX ... USING gin (doc)` |

**And the index types are a feature in their own right.** This engine has indexes for arrays, for JSON containment, for text search and for geometric data — which means a query that requires "scan everything" on another engine can be indexed here, provided the operator matches the index's operator class.

### Part 8: what follows for security

**A `SECURITY DEFINER` function that does not pin its `search_path` borrows its privileges into the caller's name resolution.** That is the whole finding, and it was measured end to end. A function owned by a privileged role, granted to a low-privilege one, whose body reads an unqualified name:

| What the low-privilege role did | What the function returned |
|---|---|
| called it with the default path | **error** — `relation "config" does not exist` |
| **put its own schema first**, containing a table with the same name | **`ATTACKER-CONTROLLED-VALUE`** |
| — and then the function was pinned to its own schema | **`the-real-value`** |

**And the comparison that makes it an escalation**: the same role reading the real table directly got `permission denied for table config`. So the function is the only way it can reach that data — and the function resolved the name the attacker chose.

**The fix is one line per function** — `ALTER FUNCTION … SET search_path = app, pg_temp` — and it works because the function's own path then applies instead of the caller's. **Which is why this is a finding worth looking for by name**: it is invisible in the function body, it is invisible in the grant, and it is only visible in a property of the function object.

**`COPY … FROM PROGRAM` runs a shell command as the database's operating-system user.** Measured:

```
COPY pgprobe_copy FROM PROGRAM 'echo shell-ran-as-$(id -un) in $(pwd)';
  -> shell-ran-as-postgres in /var/lib/postgresql/18/main
```

**The database executed a shell command and put the output into a table.** This is a superuser-only feature by default, which is the control — and it is why "the application's role is not a superuser" is not a formality here. The same reasoning applies to `pg_read_file`, `lo_import`, and every extension: **an extension is code installed by a superuser and executed inside the server**, so `CREATE EXTENSION` is a code-deployment action.

**One failed statement ends the transaction**, which is a correctness feature and also a small availability surface: an endpoint that wraps several operations in one transaction and does not use savepoints turns any single failure into "nothing happened", and code that does not notice will report success.

**Row-level security is the one place a tenancy rule can live outside the application**, and its limitation is worth stating: the table owner bypasses it unless `FORCE ROW LEVEL SECURITY` is set, and superusers always bypass it. So it protects against the application's role, not against the owner.

**And the defaults are the third layer.** `public` no longer grants `CREATE`; a role does not log in by default; a connection is to one database. Each of those is a recent tightening that an old deployment will not have — so **the version is part of the security posture here**, in a way it is not for a database whose behaviour is fixed by the standard.

### Detection and mitigation

- **Audit `SECURITY DEFINER` functions for a pinned `search_path`.** This is a catalogue query, not a review: every function with `prosecdef` set and `proconfig` unset is a candidate, and the list is short.
- **Audit `public` schema privileges against the version.** A `CREATE` bit for `PUBLIC` on a version that no longer grants it by default means it was either inherited from an upgrade or granted deliberately.
- **Alert on `COPY … FROM PROGRAM`, `pg_read_file`, `lo_import`, and `CREATE EXTENSION` from an application connection.** These are the primitives that turn a SQL injection into command execution or file access on the server host.
- **Watch for `SET search_path` followed by a call to a definer function.** As a sequence it is a signature; individually each statement is ordinary.
- **And log failed transactions, because on this engine the first error decides the rest.** A rising rate of aborted transactions is either an application bug or an endpoint being fed inputs it does not expect.
- **For mitigation, pin `search_path` on every `SECURITY DEFINER` function**, including `pg_temp` in the list so a temporary object cannot be placed ahead of the real one. This is the single control that closes the measured escalation.
- **Prefer `SECURITY INVOKER` unless the function genuinely needs the owner's rights**, and grant on the objects rather than on the function where that is possible.
- **Use row-level security where a rule is genuinely per-row**, with `FORCE ROW LEVEL SECURITY` if the application role owns the table, and remember that it does not constrain superusers or the owner by default.
- **Keep the application role away from superuser, and treat that as more than a formality.** The measured shell execution was one statement away from an ordinary connection.
- **Choose the isolation level deliberately, and note that the default differs from MySQL's.** If a read-then-write pair relies on seeing a stable snapshot, it needs `REPEATABLE READ` explicitly — otherwise it has whatever the other engine gave it by default, which is the opposite.
- **Wrap multi-step work with savepoints, or treat the first error as the end.** On this engine the second option is the one the database itself enforces.
- **And record the version in the deployment inventory.** Several of the defaults that matter here — `public` privileges, identity columns, the `search_path` guidance — changed between major versions, so "which PostgreSQL" is a security question rather than a compatibility one.

<!-- lang:zh -->
### 那个请你做决定的引擎

两个数据库引擎可以实现同一份标准，却对你提出不同的要求。MySQL 的性格来自它的**默认值** —— 一个转义字符、一个排序规则、一个存储引擎 —— 都是很久以前选定、由部署继承下来的东西。PostgreSQL 的性格来自它的**配置面**：隔离级别、角色、schema、行级安全、扩展，以及一条由调用者控制的**名字解析路径**。

> **实务上的差别在于你往哪里看。** 用前者，你读文档来弄清默认值做了什么；用后者，你读那个部署的配置与角色来弄清做了什么决定。

这就是为什么这一篇主要讲**会改变代码该怎么写的机制**，也是为什么其中有几项是**按构造**就与安全相关，而不是因为配置错误。

在一个真在跑的 **PostgreSQL 18.4** 上实测：每一组相关语句共用一个 `psql` 会话，临时 schema、角色与表用完即删。所有与会话有关的结果都来自共享同一会话的语句 —— 因为跨连接测出来的结果，测的不是那件事。

### 第一部分：它的形状

四个结构上的事实，每一个都与 MySQL 不同：

| | PostgreSQL | MySQL |
|---|---|---|
| **名字空间** | `database` → `schema` → `table` | `database` 就是 `schema` → `table`（两层） |
| **名字解析** | 由 **`search_path`** 决定 | 当前数据库，加上显式前缀 |
| **账号** | 一个**角色**，对整个集群全局 | **`user`@`host`**，按来源主机分 |
| **进程模型** | **每个连接一个后端进程** | 一个服务端，每连接一个线程 |
| **跨库查询** | 一个连接里做不到 | 加前缀就能做到 |

**名字空间那一处差别比听起来要紧。** 一个连接只连到一个**数据库**、够不到另一个；在那个数据库里面，对象住在 **schema** 里，而不合格的名字靠走一遍 `search_path` 来解析。那是一个特性 —— 它让一个数据库里能并排放 `app`、`audit`、`staging` —— 而它也正是第八部分那次提权背后的机制。

**而"角色是全局的、连接规则却在一个文件里"是第二处差别**：认证（谁能连、从哪连）是 `pg_hba.conf`，授权（能做什么）是 `GRANT`。两套系统，两个地方要看。实测，一个新建的角色：

| 角色 | 能登录 | 超级用户 |
|---|---|---|
| `pgprobe_a` | **不能** | 否 |
| `pgprobe_b` | **不能** | 否 |

**一个角色默认不是一个登录。** `CREATE ROLE` 造出来的是一个能拥有对象、能被授权的**东西**；`CREATE USER` 或加 `LOGIN` 才让它能连上来。在 MySQL 里账号**就是**登录，而且它被限定在一个主机上。

### 第二部分：隔离级别，它决定一个事务能看到什么

PostgreSQL 用 **MVCC**：读不阻塞写、写不阻塞读，因为每个事务看到的是一个**快照**。快照里有什么，取决于隔离级别。

实测，服务端的默认值：

```
default_transaction_isolation = read committed
```

以及它与 `repeatable read` 的差别 —— 在一个会话里演示，中间由另一个连接提交一行：

| 步骤 | `REPEATABLE READ` | `READ COMMITTED` |
|---|---|---|
| 事务里第一次读 | `1` | `2` |
| **另一个会话提交了一行** | | |
| 同一事务里第二次读 | **`1`** | **`3`** |
| `COMMIT` 之后 | `2` | —— |

**在 `REPEATABLE READ` 下，那个事务完全没有看到别的会话已提交的那一行** —— 快照是在事务开始时取的。而在 `READ COMMITTED` 下每条语句取一个新快照，所以第二次读看到了。

**而默认值在各引擎之间不一样**，这才是会改变代码的那部分：

| 引擎 | 默认隔离级别 |
|---|---|
| **PostgreSQL** | **`READ COMMITTED`** |
| MySQL / InnoDB | `REPEATABLE READ` |

**一对"先读再写"、并且依赖"我两条语句之间什么都没变"的代码，在 MySQL 上按默认就成立，在 PostgreSQL 上不成立。** 那不是任何一方的缺陷；那是一个默认值，也正是竞态那一类存在的原因：**一个假定自己两条语句看到同一个世界的应用，假定了一个它可能并没有的隔离级别。**

**这个引擎上还有一个真的 `SERIALIZABLE`**，以可串行化快照隔离实现，它会中止一个无法串行化的事务。所以最高级别是可用的、代价是重试 —— 那是一个设计决定，不是一个打开就忘的设置。

### 第三部分：出错之后，整个事务就结束了

这是从别的引擎过来的人最容易意外的一处。实测：

```sql
BEGIN;
SELECT 1;          -- 正常
SELECT 1/0;        -- ERROR: division by zero
SELECT 2;          -- ERROR: current transaction is aborted,
                   --        commands ignored until end of transaction block
ROLLBACK;
SELECT 42;         -- 正常
```

**一条失败的语句会毒掉整个事务。** 之后的每一条语句都被拒绝，直到回滚为止，而那条消息把这件事说得很明白。

**在 MySQL 里同样的序列会继续往下跑**：失败的语句被跳过，后面的照常执行。所以一段"每一步都试一下、失败就继续、最后提交"的代码，在两个引擎上行为完全不同 —— 在 PostgreSQL 上它在第一次失败之后就静默地什么都不做了，而那个 `COMMIT` 变成了 `ROLLBACK`。

**实用的规则是：这里的事务要么是一个完整的工作单元，要么什么都不是。** 在事务内部的错误处理必须用保存点（savepoint）来做，或者把任何错误都当成"这个事务到此为止"。

### 第四部分：类型严格性，以及转换发生在哪

实测：

| 语句 | 结果 |
|---|---|
| `INSERT INTO t (n) VALUES ('123')`，而 `n` 是 `integer` | **接受**，存成 `123` |
| `INSERT INTO t (n) VALUES ('abc')` | **拒绝** —— `invalid input syntax for type integer: "abc"` |
| `SELECT '1' = 1` | **`t`** —— 真 |
| `SELECT 'abc' + 1` | **拒绝** —— `invalid input syntax for type integer` |

**当转换有定义时字面量会被转换，没有定义时就拒绝。** `'123'` 变成整数；`'abc'` 变不成，于是语句失败而不是去猜。这与 MySQL 相反 —— 那里 `'abc' + 1` 返回 `1`，因为非数字文本变成了零。三个引擎并排看：

| 表达式 | PostgreSQL | MySQL | SQLite |
|---|---|---|---|
| `'1' = 1` | 真 | 真 | **假** |
| `'abc' + 1` | **报错** | `1` | `1` |
| 把 `'abc'` 插进整数列 | **报错** | `0` | **存成文本** |

**三个引擎，三个答案，没有一个是错的。** 实务上的后果是：一条含隐式转换的查询在这里响亮地失败、在别处静默地成功，或者存下一个与列声明不符的东西。

### 第五部分：标识符会被折叠，名字靠一条路径解析

实测：

```
CREATE TABLE pgprobe_Case (x int);       -- 不加引号
SELECT count(*) FROM pgprobe_case;       -- -> 0 行，无报错
SELECT count(*) FROM "pgprobe_Case";     -- -> ERROR: relation "pgprobe_Case" does not exist
```

**未加引号的标识符被折叠成小写**，所以那张表叫 `pgprobe_case`。加了引号的标识符原样保留大小写，这就是第二条查询在找另一张表 —— 一张不存在的表 —— 的原因。

**而一个不合格的名字由 `search_path` 解析**，它是一个会话设置，而且任何会话都能改。实测，一个新建的低权角色的：

```
search_path = "$user", public
```

由此有三件事成立，而它们是第八部分的铺垫：

**语句里的一个名字，不一定是某个特定 schema 里的名字。** 它靠走一遍列表来解析，而 `"$user"` 的意思是"一个与当前角色同名的 schema，如果它存在"。

**会话可以改那个列表。** `SET search_path` 任何角色都能用，而且它不是一个特权操作 —— 这是对的，因为它只影响这个会话自己的解析。

**而任何在别人的权限下执行一个名字的东西，都继承了这个问题** —— 那正是倒数第二部分所测的。

### 第六部分：权限 —— 角色、schema，以及按行的规则

三层，而这个引擎把三层分开。

**角色是全局的，而且可以互相授权。** 一个角色能拥有对象、持有权限、并且是另一些角色的成员；能不能连上来是一个单独的标志。

**schema 权限决定谁能创建和使用对象**，而 `public` 的默认值变过。实测，在这个版本上：

```
public nspacl = {pg_database_owner=UC/pg_database_owner,=U/pg_database_owner}
```

那个 `=U` 条目是 **`PUBLIC`（空角色名）拥有 `U`** —— USAGE，没有 CREATE。实测，一个新建的低权角色在那里建表：

```
ERROR: permission denied for schema public
```

**在更早的版本里，`PUBLIC` 对 `public` 有 `CREATE`**，于是任何角色都能在那里建对象，而一个角色建的对象可能被另一个角色的不合格名字解析到。PostgreSQL 15 收紧这一点，正是为了第五部分那个名字解析行为。

**而行级安全是引擎内建的。** 实测，一张表里有两个租户的行、开了 RLS、并有一条把某列与 `current_user` 比较的策略：

| 会话 | 看到的行数 |
|---|---|
| 超级用户 | **2** |
| `SET ROLE pgprobe_a` | **1** —— 只有它自己那一行 |

**过滤是数据库施加的，不是查询施加的。** 一个角色不带 `WHERE` 去查那张表，也只会拿到自己的行 —— 这是一种与"应用永远记得过滤"不同性质的控制，而且是这份指南里少数几个**多租户隔离规则能放在应用之外执行**的地方。

### 第七部分：自己动手做一次

```bash
sudo -u postgres psql          # peer 认证：操作系统用户映射到角色
psql "postgresql://app@host/db"
```

```sql
\l                             -- 数据库            \du   -- 角色
\dn                            -- schema            \dp   -- 权限
\d 表名                        -- 列与索引
SHOW default_transaction_isolation;
SELECT current_user, session_user, current_setting('search_path');
EXPLAIN (ANALYZE, BUFFERS) SELECT ... ;
```

**`EXPLAIN` 是值得养成的习惯。** 实测，在下面那个 JSONB 例子上，同一条查询的两个计划只差一个索引：

| 计划 | 查询 |
|---|---|
| `Seq Scan` 加一个过滤条件 | 索引还不存在时的 `WHERE doc @> '{"a": 7}'` |
| `Bitmap Index Scan on pgprobe_js_doc_idx` | `CREATE INDEX ... USING gin (doc)` 之后的同一条查询 |

**而索引类型本身就是一项特性。** 这个引擎有为数组、为 JSON 包含、为全文检索、为几何数据准备的索引 —— 也就是说，一条在别的引擎上只能"全扫"的查询，这里可以被索引，前提是那个运算符匹配索引的运算符类。

### 第八部分：从这些机制推出的安全观念

**一个不钉住 `search_path` 的 `SECURITY DEFINER` 函数，把它自己的权限借给了调用者的名字解析。** 这就是整条发现，而它被端到端地测了出来。一个由特权角色拥有、被授权给低权角色的函数，函数体里读的是一个不合格的名字：

| 低权角色做了什么 | 函数返回了什么 |
|---|---|
| 用默认路径调用 | **报错** —— `relation "config" does not exist` |
| **把它自己的 schema 放到最前**，里面放一张同名的表 | **`ATTACKER-CONTROLLED-VALUE`** |
| —— 然后函数被钉到自己的 schema 上 | **`the-real-value`** |

**而让它成为一次提权的对照是**：同一个角色直接读那张真表得到的是 `permission denied for table config`。所以那个函数是它够到那份数据的唯一途径 —— 而函数解析的名字是攻击者选的。

**修法是每个函数一行** —— `ALTER FUNCTION … SET search_path = app, pg_temp` —— 它之所以有效，是因为此后用的是函数自己的路径、而不是调用者的。**这就是为什么这是一条值得按名字去找的发现**：它在函数体里看不见、在授权里看不见，只在该函数对象的一个属性上看得见。

**`COPY … FROM PROGRAM` 会以数据库的操作系统用户执行一条 shell 命令。** 实测：

```
COPY pgprobe_copy FROM PROGRAM 'echo shell-ran-as-$(id -un) in $(pwd)';
  -> shell-ran-as-postgres in /var/lib/postgresql/18/main
```

**数据库执行了一条 shell 命令，并把输出放进了表里。** 这是一个默认仅限超级用户的功能，而那正是控制所在 —— 也是为什么"应用的角色不是超级用户"在这里不是一句客套话。同样的推理适用于 `pg_read_file`、`lo_import`，以及每一个扩展：**扩展是由超级用户安装、在服务端内部执行的代码**，所以 `CREATE EXTENSION` 是一次代码部署动作。

**一条失败语句会结束整个事务**，这既是一个正确性特性、也是一个小小的可用性面：一个把若干操作包在一个事务里、又不用保存点的端点，会把任何单点失败变成"什么都没发生"，而没注意到的代码会报告成功。

**行级安全是"租户规则能放在应用之外"的唯一一处**，而它的限制值得说出来：表属主会绕过它（除非设了 `FORCE ROW LEVEL SECURITY`），超级用户永远绕过它。所以它防的是应用那个角色，不防属主。

**而默认值是第三层。** `public` 不再授予 `CREATE`；角色默认不能登录；一个连接只连一个数据库。每一条都是近期的收紧，而一个老部署不会有 —— 所以**版本在这里是安全姿态的一部分**，对一个行为由标准固定的数据库来说则不然。

### 检测与缓解

- **审计 `SECURITY DEFINER` 函数有没有钉住 `search_path`。** 这是一条查目录的语句，不是一次阅读评审：每个设了 `prosecdef` 而 `proconfig` 为空的函数都是候选，而那份清单很短。
- **把 `public` schema 的权限与版本对起来查。** 在一个默认已不再授予的版本上，`PUBLIC` 对 `public` 有 `CREATE` 位，意味着它是从升级继承来的、或者是被有意授出去的。
- **对来自应用连接的 `COPY … FROM PROGRAM`、`pg_read_file`、`lo_import`、`CREATE EXTENSION` 告警。** 这些是把一次 SQL 注入变成服务端主机上命令执行或文件访问的原语。
- **盯"`SET search_path` 之后紧跟一次 definer 函数调用"。** 作为一个序列它是一个签名；单独看，每一条语句都很平常。
- **并且记录失败的事务，因为在这个引擎上第一次错误决定了后面的全部。** 被中止事务的比率上升，要么是应用 bug，要么是一个端点正在被喂它不预期的输入。
- **缓解上，给每一个 `SECURITY DEFINER` 函数钉住 `search_path`**，列表里要包含 `pg_temp`，这样一个临时对象就不能被放到真对象前面。这是关掉上面那条实测提权的唯一一项控制。
- **除非函数确实需要属主的权限，否则优先用 `SECURITY INVOKER`**，并且在可能的时候授在对象上而不是函数上。
- **在规则确实是按行的时候用行级安全**，如果应用那个角色是表属主就加上 `FORCE ROW LEVEL SECURITY`，并记住它默认不约束超级用户与属主。
- **让应用角色远离超级用户，并把这句话当成不只一句客套。** 实测那次 shell 执行，离一个普通连接只有一条语句。
- **有意识地选隔离级别，并注意默认值与 MySQL 的不同。** 如果一对"先读再写"依赖看到一个稳定的快照，它就得显式写成 `REPEATABLE READ` —— 否则它拿到的是另一个引擎的默认值，而那正好相反。
- **多步工作要么用保存点包起来，要么把第一次错误当成结束。** 在这个引擎上，后一个选项是数据库自己强制的那个。
- **并且把版本记进部署清单。** 这里几处要紧的默认值 —— `public` 的权限、标识列、`search_path` 的指引 —— 都在大版本之间变过，所以"哪一个 PostgreSQL"是一个安全问题，而不是一个兼容性问题。
