---
id: db-oracle
title_en: Oracle Database
title_zh: Oracle Database
summary_en: The engine with two properties no other store in this series has — a user is also a schema, and an empty string is null — and a privilege system whose revocation does not always reach the grants that were handed on. The portability boundary measured, including the two Oracle idioms another engine quietly accepts.
summary_zh: 这个引擎有两个这份指南里其他存储都没有的性质 —— 一个用户同时是一个 schema，以及空串就是 null —— 还有一套"回收权限时不总能追到已经转授出去的那些"的权限体系。可移植性的边界在这里实测出来，包括那两处被另一个引擎悄悄接受的 Oracle 写法。
tags: [beginner, database, oracle, plsql, dialect, privileges]
tools: [sqlplus, sqlcl, python3]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Two properties nothing else here has

Every engine in this series has something distinctive. This one has two facts that change how code and permissions behave, and neither is a detail:

> **A user is also a schema.** Creating an account creates a namespace with the same name, and that user owns everything in it.
>
> **An empty string is null.** `'' IS NULL` is true, the two are stored identically, and no query can tell them apart.

The first makes identity and naming the same object, which is why the privilege model here has a shape of its own. The second makes "no value" and "a value that is empty" the same thing, which is why a comparison that works elsewhere can silently match nothing here.

Measured below: the portability boundary, run against the three engines available in this lab, plus one contrast the other engines cannot show.

### Part 1: the shape of it

| | Here |
|---|---|
| top level | **instance** — usually one database per instance (the container database model is the exception) |
| naming | **`user` = `schema`** — a table is `SCHEMA.TABLE`, and the schema is an account |
| account | a database user with a password, created by `CREATE USER` |
| storage | **tablespaces**, with data files underneath |
| privileged accounts | `SYS` (the dictionary owner) and `SYSTEM` |
| catalogue | `USER_*`, `ALL_*`, `DBA_*` views, plus `V$` dynamic views |

**The `USER_*`, `ALL_*`, `DBA_*` families are worth knowing by shape**, because they answer three different questions with the same columns:

| Prefix | Answers |
|---|---|
| `USER_` | what **I** own |
| `ALL_` | what **I** can reach |
| `DBA_` | everything, for a privileged account |

**So "which tables can this account see" is `ALL_TABLES`, not `DBA_TABLES`** — and the difference between the two is the review question, since a `DBA_` query run by an application account tells you nothing about that account's own access.

**And the identity model is one layer, not two.** Measured in the previous entry, SQL Server has instance-level logins mapped to per-database users; here a user is created in the database and is the schema. **Which means the unit of authorisation is also the unit of naming** — granting a right and creating a namespace are the same kind of act.

### Part 2: an empty string is null

This is the property to know before anything else, because it is invisible in the code that trips over it. Measured on the three engines available here:

| Expression | SQLite | PostgreSQL | MariaDB |
|---|---|---|---|
| `SELECT '' IS NULL` | **`0`** (false) | **`f`** (false) | **`0`** (false) |

**All three say false. On this engine the answer is true**, because a zero-length string is not stored as a value at all — it is treated as null at every level, so nothing can distinguish them afterwards.

**The consequences are all of the same shape, and they matter in both directions.**

**A comparison against an empty string never matches.** `WHERE code = ''` cannot be true, because `''` is null and `null = null` is not true. On the engines measured above the same predicate matches rows whose value is the empty string. **So a query written for one engine that filters on an empty value returns nothing here, without an error.**

**And `IS NULL` matches both.** `WHERE code IS NULL` is true for the row that was inserted as `NULL` and for the row that was inserted as `''`. **Which is the dangerous direction**: a check meaning "this field was never set" also fires for a field that was deliberately set to empty — and a check meaning "the token is empty, so let it through" is now a check that fires for a token that is null.

**Inserting an empty string stores a null**, so a column with `NOT NULL` refuses `''` as well as `NULL`, and a read of that column returns null rather than an empty string. Round-tripping a value through the engine can change it from "empty" to "absent" without any statement saying so.

**And concatenation is the standard operator, not a function:**

| Expression | SQLite | PostgreSQL | MariaDB |
|---|---|---|---|
| `'a' \|\| 'b'` | `ab` | `ab` | **`0`** |
| `CONCAT('a','b')` | `ab` | `ab` | `ab` |

**`||` is the operator here and on two of the three measured engines**, and on the third it is logical OR returning a number — the trap already measured in the MySQL entry. The part specific to this engine is what happens when one side is null: **because null propagates, concatenating anything with a value that came in as an empty string concatenates with null and yields null**, which is how an "empty means absent" assumption turns into a missing row.

### Part 3: the portability boundary, measured

The same idioms, run against the three engines available in this lab:

| Oracle idiom | SQLite | PostgreSQL | MariaDB |
|---|---|---|---|
| `SELECT 1 FROM DUAL` | `no such table: DUAL` | `relation "dual" does not exist` | **`1`** |
| `NVL(NULL,'x')` | no such function | `function nvl(...) does not exist` | **`x`** |
| `SELECT ROWNUM ...` | `no such column: ROWNUM` | `column "rownum" does not exist` | `Unknown column` |
| `SYSDATE` | no such column | `column "sysdate" does not exist` | `Unknown column` |
| `TO_CHAR(1)` | no such function | `function to_char(integer) does not exist` | **`Invalid arguments`** |
| `SELECT 1 MINUS SELECT 2` | syntax error | syntax error | `ERROR 1064` |
| `SELECT 1 EXCEPT SELECT 2` | **`1`** | **`1`** | **`1`** |
| `LPAD('x',3,'0')` | no such function | **`00x`** | **`00x`** |
| `CREATE TABLE (id NUMBER, s VARCHAR2(10))` | syntax error | `type "number" does not exist` | `ERROR 1064` |
| `... WHERE a.id = b.id(+)` | syntax error | syntax error | `ERROR 1064` |

**Two rows are the interesting ones, and they point in opposite directions.**

**`DUAL` and `NVL` are accepted by MariaDB.** That engine ships both for Oracle compatibility — a one-row table to select from and an alias for its own null-handling function. **So two of the most recognisable Oracle idioms work on one of the three measured engines and fail on the other two**, which makes them exactly the wrong thing to rely on when porting: the failure mode depends on which engine the code lands on.

**And `TO_CHAR` is recognised by name** on MariaDB — the error is `Invalid arguments` rather than "no such function", meaning a function of that name exists with different parameters. **A name being known is not the same as a signature being portable.**

**`MINUS` fails everywhere while `EXCEPT` works everywhere**, including the engines that implement both spellings of other things. It is the cleanest example in this table of a difference that is pure vocabulary: the same operation, two words, and only one of them is understood outside this engine.

**`LPAD` shows the opposite pattern** — absent on the engine with the smallest function set, present on the other two. **A portability table read once is not a portability policy**; what it gives you is the list of names to look up.

### Part 4: types

The type system has four features worth knowing before reading a schema.

**`NUMBER` is one type for integers and decimals.** There is no separate integer and floating type in the common case, and the precision is part of the declaration. **So "the column is a number" does not say whether it holds a count or an amount**, and a ported schema that expects `INT` and `DECIMAL` to behave differently has to be rewritten rather than translated.

**`VARCHAR2` is the string type**, and its length is in bytes or characters depending on the declaration. The measured engines rejected it by name, which is useful: a schema script written here fails loudly rather than creating something approximate.

**And `DATE` includes a time.** That is the one that changes behaviour: in most engines a date column holds a date, and the time is either a separate type or lost. **Here a `DATE` column holds both**, so a value read out and written back keeps its time, and a comparison between a date and a date-with-time follows the time as well.

**And there is no boolean type at the SQL level.** PL/SQL has one, and it cannot be used in SQL — a column cannot be declared boolean, and a PL/SQL boolean cannot be returned by a query. **So a truth value is a `NUMBER(1)` or a `CHAR(1)` with a check constraint**, which is the same absence measured on SQL Server with a different implementation.

### Part 5: transactions, and a default that does not block

This engine is fully transactional, and its default isolation level is **`READ COMMITTED`** — the same name as the defaults measured on PostgreSQL and SQL Server. The three implementations are not the same thing:

| Engine | Default level | Implementation |
|---|---|---|
| PostgreSQL | `READ COMMITTED` | snapshot per statement |
| **SQL Server** | `READ COMMITTED` | **shared locks — readers block writers** |
| **Oracle** | `READ COMMITTED` | **undo data — readers do not block writers** |

**Same name, three behaviours**, and the difference is what an application has to be written against. Here, readers and writers do not block each other, because a reader reconstructs the version of a row it needs from **undo** rather than waiting for a lock.

**Which turns undo into an operational and a correctness topic.** A long-running query needs the undo from when it started to still exist; if the undo tablespace has been reused, the query fails with a snapshot-too-old error rather than returning a wrong answer. **So the guarantee is "the query sees a consistent point in time, provided the engine still has the data to reconstruct it"** — a caveat the other two implementations of the same level do not have.

**DDL commits, as it does on MySQL and does not on PostgreSQL.** One measured example: creating a table inside a transaction commits everything before it, so a migration that fails halfway cannot be rolled back. The entries on those two engines measured the difference directly, and the pattern here follows the MySQL one.

**And `SELECT ... FOR UPDATE` is the explicit lock**, which is how a read-then-write sequence is protected. It exists because the default level does not protect it: **two transactions reading the same row and then updating it will both succeed at the read and one will overwrite the other**, because neither blocked.

### Part 6: PL/SQL, and the packages that reach outside

Stored code here is PL/SQL, and the language's own capabilities are the part worth knowing from a security position, because they are not database capabilities.

**Dynamic SQL is `EXECUTE IMMEDIATE`, and it takes bind variables.** `EXECUTE IMMEDIATE 'SELECT ... WHERE id = :1' USING v_id` keeps the value out of the statement text, so the language that looks most dangerous has the parameterisation built in — and using it is the same decision as everywhere else in this series.

**And the packages that reach outside the database:**

| Package | Reaches |
|---|---|
| `UTL_HTTP`, `UTL_TCP`, `UTL_SMTP` | **the network, from the database server** |
| `UTL_FILE` | **the filesystem of the server** |
| `DBMS_SCHEDULER` | **jobs, including operating-system commands** |
| `DBMS_ASSERT` | the opposite: validating identifiers and literals |

**So a database account with execute rights on those packages is not "an account that can touch data".** It is one that can make the server fetch a URL, read a file, or schedule work — the same class of capability as the command switch measured on SQL Server, spread across several packages instead of one setting.

**And network access is controlled separately, which is the control to look for.** Since the version that introduced it, outbound network access from PL/SQL is governed by **network access control lists** managed through `DBMS_NETWORK_ACL_ADMIN`, so the question is not "does the account have execute on `UTL_HTTP`" alone but "and is there an ACL granting it a host".

**`DBMS_SCHEDULER` is the one to compare with `xp_cmdshell`.** Both turn a database privilege into execution on the host; the difference is that one is a single configuration switch and the other is a package privilege combined with a job, which makes it quieter.

### Part 7: the privilege model, and three asymmetries

**The first is the reason this part exists: revoking a system privilege does not cascade.**

| Grant | On revoke |
|---|---|
| object privilege `WITH GRANT OPTION` | **cascades** — grants the grantee handed on are removed too |
| **system privilege `WITH ADMIN OPTION`** | **does not cascade** — grants the grantee handed on **remain** |

**So revoking a privilege that was passed on does not remove it from the accounts that received it.** The access that was just taken away is still in place, on an account nobody looked at, and the only way to find it is to enumerate who was granted it. **This is a documented asymmetry rather than a defect**, and it is the reason a revocation is not finished when the `REVOKE` succeeds.

**The second is `PUBLIC`.** Every user is a member of it, so a grant to `PUBLIC` is a grant to everyone including accounts created later. It is the standard mechanism for exposing things instance-wide, and the reason "is anything granted to `PUBLIC`" is a question with a short answer and a large consequence.

**And the third is the class of privileges whose name says what they do.** `SELECT ANY TABLE`, `INSERT ANY TABLE`, `EXECUTE ANY PROCEDURE` — the word `ANY` is the scope, and a grant of one of them is not a narrowing of the account's reach but a widening of it to every schema in the database. **Where the previous engines in this series had per-database roles, the equivalent shortcut here crosses schemas**, because schemas are users.

**The administrative accounts are `SYS` and `SYSTEM`, and `SYSDBA`/`SYSOPER` are privileges rather than users** — held through a privileged connection, and grantable to an operating-system group so that membership of that group is the authentication.

**And the well-known accounts are the historical ones.** Older installations shipped a set of demonstration and monitoring accounts with passwords that were published in documentation and are still attempted first by every scanner. Current versions lock and expire them by default; the measurable question on any installation is which of them are still unlocked.

**Doing it yourself:**

```bash
sqlplus app/password@//host:1521/service
lsnrctl status                    # the listener, on port 1521
```

```sql
SELECT table_name FROM user_tables;                    -- what I own
SELECT table_name FROM all_tables;                     -- what I can reach
SELECT * FROM user_tab_privs;                          -- grants I hold
SELECT * FROM user_role_privs;
SELECT grantee, privilege, admin_option FROM dba_sys_privs WHERE admin_option = 'YES';
SELECT grantee, table_name, privilege, grantable FROM dba_tab_privs WHERE grantable = 'YES';
```

**Those last two queries are the measured asymmetry in practice**: the accounts holding a grant option are the ones whose revocation will need following up.

### Part 8: what follows for security

**A user being a schema means permissions and names are the same object.** Granting an account the right to create a table creates a namespace; dropping the account drops everything in it; and an object is always addressed as `schema.object`. **Which makes "who owns this" answerable from the name**, and makes a shared schema a shared namespace — the common arrangement where an application's tables sit in one schema means every account with rights in it can create objects that other code will resolve.

**The empty-string-is-null behaviour is an authorisation fact in one direction and a filter bug in the other.** A check of the form "the field is empty, so treat it as unset" fires for null as well. A check of the form "the field does not equal the empty string" is always true. **Both are silent**, because neither produces an error, and both are the kind of thing that is written once against another engine and never revisited.

**And `IS NULL` is the widest predicate here.** It matches the row where nothing was written and the row where an empty value was written, so **a query that means "no value was provided" is a query that also matches "a value was provided and it was empty"** — which for a token, a code or a filename is a difference that decides something.

**The revocation asymmetry means "we revoked it" is a claim that needs an enumeration behind it.** Since system privileges granted onward survive a `REVOKE`, the honest procedure is to find the accounts holding the privilege with the grant option, then check what each of them granted. **A review that reads the `REVOKE` statement and stops has confirmed an intention rather than a state.**

**And the `ANY` privileges are the shortcut that looks narrower than it is.** `SELECT ANY TABLE` reads as a table-level grant and behaves as a cross-schema one — because schemas are users, and there are as many of them as there are accounts.

**PL/SQL's reach is the largest capability surface in this guide.** The measured engines each had one path from a query to the host; here there are several, in different packages, controlled in different ways: a network ACL for outbound access, a package privilege for file access, a scheduler job for execution. **So the inventory question is per-package rather than per-switch**, and an application account holding execute on `UTL_*` or `DBMS_SCHEDULER` that never calls them is a grant to review rather than a capability in use.

**And PL/SQL injection is the same class as everything else in this series.** `EXECUTE IMMEDIATE` with a concatenated string is the vulnerable form; with `USING` it is the parameterised one. **The difference is one clause in a statement that otherwise looks identical**, which is why it belongs next to the SQL injection entries rather than in a language-specific discussion.

**The listener is a network service of its own**, separate from the database, with its own version and its own commands. It has a history of unauthenticated command handling, including the external procedure feature that turned a listener connection into execution. **The database's patch level and the listener's are two different things to check.**

### Detection and mitigation

- **Enumerate accounts holding system privileges with the admin option, then check what each of them granted.** The measured asymmetry means a revocation does not remove the grants that were handed on, so the state has to be read rather than assumed.
- **Check what is granted to `PUBLIC`.** Every account is a member, which makes it the widest possible grantee and the shortest one to review.
- **Review `ANY` privileges on application accounts.** `SELECT ANY TABLE` and `EXECUTE ANY PROCEDURE` are cross-schema by definition, because a schema is a user.
- **Inventory execute rights on `UTL_HTTP`, `UTL_TCP`, `UTL_FILE` and `DBMS_SCHEDULER`, and the network ACLs that go with them.** Together these are the path from a database account to the server's network and filesystem, and the ACL is the control that decides whether the path is open.
- **Check the default accounts, the listener version, and the listener's own configuration**, as three separate items, because they are patched and configured separately from the database.
- **Watch for `EXECUTE IMMEDIATE` in stored code and for any dynamic statement built by concatenation**, and check that binds are used rather than text.
- **Alert on DDL from application connections**, since it commits the surrounding transaction and creates namespaces in a schema that other code resolves against.
- **For mitigation, grant per object rather than `ANY`**, and keep the application's account in its own schema so that its namespace is not shared with code it does not own.
- **Revoke with the follow-up built in**: find the grant-option holders first, revoke, then verify the downstream grants are gone — and where they are not, revoke them explicitly.
- **Do not deploy code that relies on an empty string being distinct from null.** Where the difference decides something, use an explicit sentinel and a `NOT NULL` constraint, so that "absent" and "empty" are two states rather than one.
- **Keep outbound network access closed by default and add ACLs per host and per account**, rather than granting execute on a network package and treating that as the decision.
- **And treat the empty string, the null, and the unset field as one question in review**, because on this engine they are one value, and code written elsewhere has three.

<!-- lang:zh -->
### 两个别处都没有的性质

这个系列里每个引擎都有点自己的东西。这一个有两个会改变代码与权限行为的事实，而两个都不是细节：

> **一个用户同时是一个 schema。** 建一个账号就建出一个同名的名字空间，而里面的一切都归那个用户。
>
> **空串就是 null。** `'' IS NULL` 为真，两者被存成同一个东西，而没有任何查询能把它们区分开。

第一条让身份与命名成了同一个对象，这就是为什么这里的权限模型有它自己的形状。第二条让"没有值"与"值是空的"成了同一件事，这就是为什么一个在别处能用的比较会在这里静默地匹配不到任何东西。

下面实测的是：可移植性的边界，在实验室里现有的三个引擎上跑；以及一个别的引擎无法展示的对照。

### 第一部分：它的形状

| | 这里 |
|---|---|
| 最上层 | **实例** —— 通常一个实例一个数据库（容器数据库模型是例外） |
| 命名 | **`user` = `schema`** —— 一张表是 `SCHEMA.TABLE`，而 schema 就是一个账号 |
| 账号 | 一个带口令的数据库用户，由 `CREATE USER` 建出来 |
| 存储 | **表空间**，底下是数据文件 |
| 特权账号 | `SYS`（字典的属主）与 `SYSTEM` |
| 目录 | `USER_*`、`ALL_*`、`DBA_*` 视图，外加 `V$` 动态视图 |

**`USER_*`、`ALL_*`、`DBA_*` 这三族视图值得按形状记住**，因为它们用同样的列回答三个不同的问题：

| 前缀 | 回答 |
|---|---|
| `USER_` | **我**拥有什么 |
| `ALL_` | **我**能够到什么 |
| `DBA_` | 一切，给特权账号用 |

**所以"这个账号能看到哪些表"是 `ALL_TABLES`，不是 `DBA_TABLES`** —— 而两者的差别就是评审的问题，因为一个应用账号跑一句 `DBA_` 查询，说明不了那个账号自己有什么访问权。

**而身份模型是一层，不是两层。** 上一篇实测过，SQL Server 有实例级的登录名映射到按库的用户；而这里用户就建在数据库里、并且就是 schema。**这意味着授权的单位同时也是命名的单位** —— 授权与建一个名字空间是同一类动作。

### 第二部分：空串就是 null

这是在任何别的事情之前就该知道的性质，因为它在绊倒它的代码里是看不见的。在现有三个引擎上实测：

| 表达式 | SQLite | PostgreSQL | MariaDB |
|---|---|---|---|
| `SELECT '' IS NULL` | **`0`**（假） | **`f`**（假） | **`0`**（假） |

**三个都说假。而在本引擎上答案是真**，因为一个长度为零的字符串根本不是作为一个值被存下来的 —— 它在每一层都被当成 null，所以之后没有任何东西能把它们区分开。

**后果都是同一个形状，而在两个方向上都要紧。**

**与空串的比较永远不匹配。** `WHERE code = ''` 不可能为真，因为 `''` 就是 null、而 `null = null` 不为真。在上面实测的那些引擎上，同一个谓词会匹配到值为空串的行。**所以一段为某个引擎写的、按空值过滤的查询，在这里什么都不返回，而且不报错。**

**而 `IS NULL` 两者都匹配。** 对那一行插入 `NULL` 的和那一行插入 `''` 的，`WHERE code IS NULL` 都为真。**这才是有危险的那个方向**："这个字段从来没被设置过"这个检查，对一个被有意设成空值的字段也会成立 —— 而一个意思是"令牌是空的，所以放行"的检查，现在成了一个对 null 令牌也成立的检查。

**插入一个空串存下的是一个 null**，所以一个带 `NOT NULL` 的列既拒绝 `''` 也拒绝 `NULL`，而读那一列回来得到的是 null 而不是空串。**一个值穿过这个引擎一趟，可以从"空的"变成"没有"，而没有任何语句说过这件事。**

**而拼接用的是标准运算符，不是函数：**

| 表达式 | SQLite | PostgreSQL | MariaDB |
|---|---|---|---|
| `'a' \|\| 'b'` | `ab` | `ab` | **`0`** |
| `CONCAT('a','b')` | `ab` | `ab` | `ab` |

**`||` 是这里的运算符，也是三个实测引擎里两个的**，而在第三个上它是返回数字的逻辑或 —— 那个陷阱已经在 MySQL 那一篇里实测过了。属于本引擎的那部分是当一边是 null 时会发生什么：**因为 null 会传播，拿一个来自空串的值去拼接，就是拿 null 去拼接、得到 null** —— 这也就是"空即缺失"这个假设如何变成一行数据消失。

### 第三部分：可移植性的边界，实测

同一批写法，在实验室里现有的三个引擎上跑：

| Oracle 写法 | SQLite | PostgreSQL | MariaDB |
|---|---|---|---|
| `SELECT 1 FROM DUAL` | `no such table: DUAL` | `relation "dual" does not exist` | **`1`** |
| `NVL(NULL,'x')` | 没有这个函数 | `function nvl(...) does not exist` | **`x`** |
| `SELECT ROWNUM ...` | `no such column: ROWNUM` | `column "rownum" does not exist` | `Unknown column` |
| `SYSDATE` | 没有这一列 | `column "sysdate" does not exist` | `Unknown column` |
| `TO_CHAR(1)` | 没有这个函数 | `function to_char(integer) does not exist` | **`Invalid arguments`** |
| `SELECT 1 MINUS SELECT 2` | 语法错 | 语法错 | `ERROR 1064` |
| `SELECT 1 EXCEPT SELECT 2` | **`1`** | **`1`** | **`1`** |
| `LPAD('x',3,'0')` | 没有这个函数 | **`00x`** | **`00x`** |
| `CREATE TABLE (id NUMBER, s VARCHAR2(10))` | 语法错 | `type "number" does not exist` | `ERROR 1064` |
| `... WHERE a.id = b.id(+)` | 语法错 | 语法错 | `ERROR 1064` |

**有两行才是有意思的，而它们指向相反的方向。**

**`DUAL` 与 `NVL` 被 MariaDB 接受了。** 那个引擎为了 Oracle 兼容同时提供这两样 —— 一张用来 select 的单行表，以及它自己那个处理 null 的函数的一个别名。**所以两个最好认的 Oracle 写法，在三个实测引擎里的一个上能用、在另外两个上失败**，这让它们恰恰是最不该在移植时依赖的东西：失败方式取决于代码落在哪个引擎上。

**而 `TO_CHAR` 被按名字认了出来** —— 在 MariaDB 上报的是 `Invalid arguments` 而不是"没有这个函数"，意思是这个名字的函数存在、只是参数不同。**一个名字被认得，不等于一个签名是可移植的。**

**`MINUS` 在所有地方都失败，而 `EXCEPT` 在所有地方都能用**，包括那些为别的东西实现了两种拼法的引擎。它是这张表里纯粹词汇差异最干净的例子：同一个操作、两个词、而只有一个在这个引擎之外被理解。

**`LPAD` 显示的是相反的模式** —— 在函数集最少的那个引擎上缺失，在另外两个上都有。**一张读一遍的可移植性表不是一套可移植性策略**；它给你的是一份要去查名字的清单。

### 第四部分：类型

在读一份 schema 之前，有四件关于类型系统的事值得知道。

**`NUMBER` 是整数与小数共用的一种类型。** 常见情况下没有单独的整数与浮点类型，而精度是声明的一部分。**所以"这一列是个数字"没有说明它装的是计数还是金额**，而一份期望 `INT` 与 `DECIMAL` 行为不同的移植 schema，只能改写、不能翻译。

**`VARCHAR2` 是字符串类型**，它的长度按声明是字节或者字符。实测那些引擎按名字拒绝了它，这一点有用：在这里写的一份 schema 脚本会响亮地失败，而不是建出个近似的东西。

**而 `DATE` 包含时间。** 这一条改变行为：在多数引擎里日期列装的就是一个日期，时间要么是另一种类型、要么被丢掉。**这里一个 `DATE` 列两者都装**，所以读出来再写回去的值会保留时间，而一个日期与一个带时间的日期之间的比较也会带上时间。

**而 SQL 层面没有布尔类型。** PL/SQL 里有一个，而它不能在 SQL 里用 —— 一列不能被声明成布尔，一个 PL/SQL 的布尔也不能被一条查询返回。**所以一个真值是一个 `NUMBER(1)` 或者一个带检查约束的 `CHAR(1)`**，这与 SQL Server 上实测到的同一处缺失是同一个东西、不同的实现。

### 第五部分：事务，以及一个不阻塞的默认值

这个引擎有完整的事务，默认隔离级别是 **`READ COMMITTED`** —— 与 PostgreSQL 和 SQL Server 上实测到的默认值同名。三种实现不是同一回事：

| 引擎 | 默认级别 | 实现 |
|---|---|---|
| PostgreSQL | `READ COMMITTED` | 每条语句一个快照 |
| **SQL Server** | `READ COMMITTED` | **共享锁 —— 读者挡住写者** |
| **Oracle** | `READ COMMITTED` | **undo 数据 —— 读者不挡写者** |

**同一个名字，三种行为**，而差别正是应用必须照着写的那件事。在这里，读者与写者互不阻塞，因为读者从 **undo** 重建它需要的那个版本的行，而不是等锁。

**这就把 undo 变成了一个运维话题、也是一个正确性话题。** 一条长时间运行的查询需要它开始那一刻的 undo 仍然存在；如果 undo 表空间已经被复用，那条查询会以一个"快照过旧"的错误失败，而不是返回一个错误的答案。**所以这份保证是"这条查询看到一个一致的时间点，前提是引擎还有数据去重建它"** —— 一个另外两种同级别的实现没有的前提。

**DDL 会提交，和 MySQL 一样，与 PostgreSQL 相反。** 一个实测过的例子：在事务里建表会把它之前的一切提交掉，所以一个中途失败的迁移回滚不了。那两篇已经把这个差别直接测过了，而这里的模式跟随 MySQL 那一个。

**而 `SELECT ... FOR UPDATE` 是那种显式的锁**，也是保护一段"先读再写"的方式。它存在，正是因为默认级别不保护它：**两个事务读到同一行、然后各自更新，两边都会读成功，而其中一个会覆盖另一个**，因为谁都没被挡住。

### 第六部分：PL/SQL，以及那些够到外面的包

这里的存储代码是 PL/SQL，而它自身的语言能力是从安全位置看最该知道的部分，因为它们不是数据库能力。

**动态 SQL 是 `EXECUTE IMMEDIATE`，而它接受绑定变量。** `EXECUTE IMMEDIATE 'SELECT ... WHERE id = :1' USING v_id` 把值挡在语句文本之外，所以这门看起来最危险的语言把参数化内置了 —— 用不用它，和这个系列里其他地方是同一个决定。

**而那些够到数据库之外的包：**

| 包 | 够到 |
|---|---|
| `UTL_HTTP`、`UTL_TCP`、`UTL_SMTP` | **网络，从数据库服务器发出** |
| `UTL_FILE` | **服务器的文件系统** |
| `DBMS_SCHEDULER` | **作业，包括操作系统命令** |
| `DBMS_ASSERT` | 相反的：校验标识符与字面量 |

**所以一个对这些包有执行权限的数据库账号，不是"一个能碰数据的账号"。** 它是一个能让服务器去取一个 URL、读一个文件、或者排一个作业的账号 —— 与 SQL Server 上实测到的那个命令开关同一类能力，只是分散在几个包里、而不是一个设置上。

**而网络访问是单独控制的，那才是要找的控制点。** 从引入它的那个版本起，PL/SQL 的出站网络访问由一个 **网络访问控制列表** 管理，通过 `DBMS_NETWORK_ACL_ADMIN` 维护 —— 所以问题不只是"这个账号对 `UTL_HTTP` 有没有执行权限"，而是"以及有没有一条 ACL 给了它某个主机"。

**`DBMS_SCHEDULER` 是那个该和 `xp_cmdshell` 对照的。** 两者都把一条数据库权限变成宿主机上的执行；差别在于一个是单个配置开关、另一个是一个包权限加一个作业，这让后者更安静。

### 第七部分：权限模型，以及三处不对称

**第一处就是这一部分存在的原因：回收一个系统权限不会级联。**

| 授权 | 回收时 |
|---|---|
| 对象权限 `WITH GRANT OPTION` | **级联** —— 被授权者转授出去的那些也会被去掉 |
| **系统权限 `WITH ADMIN OPTION`** | **不级联** —— 被授权者转授出去的那些**仍然留着** |

**所以回收一个被转授过的权限，并没有把它从收到它的那些账号上拿掉。** 刚刚被收走的访问权仍然在一个没人看过的账号上，而唯一的发现方式是枚举谁被授过。**这是一处文档化的不对称、而不是缺陷**，也是为什么一次回收不是在 `REVOKE` 成功时就结束了。

**第二处是 `PUBLIC`。** 每个用户都是它的成员，所以一次对 `PUBLIC` 的授权就是一次对所有人、包括之后才建出来的账号的授权。它是把东西暴露到整个实例的标准机制，也是为什么"有没有什么被授给了 `PUBLIC`"是一个答案很短、后果很大的问题。

**第三处是那一类"名字就说明了作用域"的权限。** `SELECT ANY TABLE`、`INSERT ANY TABLE`、`EXECUTE ANY PROCEDURE` —— `ANY` 这个词就是作用域，而授出其中一个不是收窄了那个账号的可达范围，而是把它拓宽到了数据库里的每一个 schema。**这个系列里前面的引擎有按库的角色，而这里对应的捷径是跨 schema 的**，因为 schema 就是用户。

**管理账号是 `SYS` 与 `SYSTEM`，而 `SYSDBA`/`SYSOPER` 是权限而不是用户** —— 通过一个特权连接持有，也可以授给一个操作系统组，于是那个组的成员资格就是认证。

**而那些众所周知的账号是历史上留下的。** 较老的安装附带一组演示与监控账号，口令写在文档里、至今仍是每个扫描器首先尝试的。当前版本默认把它们锁定并过期；在任何安装上可测的问题是其中还有哪些是解锁的。

**自己动手做一次：**

```bash
sqlplus app/password@//host:1521/service
lsnrctl status                    # 监听器，在 1521 端口
```

```sql
SELECT table_name FROM user_tables;                    -- 我拥有的
SELECT table_name FROM all_tables;                     -- 我能达到的
SELECT * FROM user_tab_privs;                          -- 我持有的授权
SELECT * FROM user_role_privs;
SELECT grantee, privilege, admin_option FROM dba_sys_privs WHERE admin_option = 'YES';
SELECT grantee, table_name, privilege, grantable FROM dba_tab_privs WHERE grantable = 'YES';
```

**最后那两条查询就是那处不对称在实践中的样子**：持有授权选项的那些账号，就是回收时需要跟进的那些。

### 第八部分：从这些机制推出的安全观念

**用户就是 schema，意味着权限与命名是同一个对象。** 授给一个账号建表的权利就是建出一个名字空间；删掉那个账号就删掉了里面的一切；而一个对象永远以 `schema.object` 被寻址。**这让"这归谁"可以从名字回答出来**，也让共享 schema 成为一个共享名字空间 —— 那个常见的安排（应用的表明明放在一个 schema 里）意味着在那个 schema 里有权限的每个账号都能建出别的代码会去解析的对象。

**空串即 null 在一个方向上是一个授权事实，在另一个方向上是一个过滤 bug。** 一个"这个字段是空的，所以当作没设置"的检查，对 null 也成立。一个"这个字段不等于空串"的检查永远为真。**两者都是静默的**，因为两者都不产生错误，而两者都属于那种"照着另一个引擎写一次、然后永远不再回头看"的东西。

**而 `IS NULL` 是这里最宽的谓词。** 它既匹配什么都没写的那一行，也匹配写了一个空值的那一行，所以**一条意思是"没有提供值"的查询，也是一条匹配"提供了一个空值"的查询** —— 对一个令牌、一个验证码或者一个文件名来说，这个差别是要决定事情的。

**回收权限的不对称意味着"我们已经回收了"是一句需要枚举在后面支撑的话。** 因为转授出去的系统权限在一次 `REVOKE` 之后仍然存在，诚实的做法是先找出持有那项权限及其授权选项的账号，再查每一个账号授出了什么。**一次读到 `REVOKE` 语句就停下的评审，确认的是一个意图、而不是一个状态。**

**而 `ANY` 那一类权限，是那个看起来比实际更窄的捷径。** `SELECT ANY TABLE` 读起来像一次表级授权，行为上是一次跨 schema 的授权 —— 因为 schema 就是用户，而账号有多少个，就有多少个 schema。

**PL/SQL 的可达范围是这份指南里最大的能力面。** 那些实测过的引擎各自只有一条从查询到宿主机的路；这里有若干条、在不同的包里、由不同的方式控制：出站访问靠网络 ACL、文件访问靠一个包权限、执行靠一个调度作业。**所以清点的问题是逐个包、而不是逐个开关**，而一个持有 `UTL_*` 或 `DBMS_SCHEDULER` 执行权限、却从不调用它们的应用账号，是一次待评审的授权、而不是一项在用的能力。

**而 PL/SQL 注入与这个系列里其他一切是同一类。** 拼接字符串的 `EXECUTE IMMEDIATE` 是有问题的那种写法；用 `USING` 的是参数化的那种。**差别是一条语句里的一个子句，而那条语句在其余部分看起来一模一样**，这就是为什么它该和 SQL 注入那几篇放在一起，而不是放进一段语言专属的讨论。

**监听器是它自己的一个网络服务**，与数据库分开，有自己的版本和自己的命令。它有一段未认证命令处理的历史，包括那个把一次监听器连接变成执行的"外部过程"特性。**数据库的补丁级别与监听器的补丁级别是两件要分别检查的事。**

### 检测与缓解

- **枚举持有带 admin 选项的系统权限的账号，然后查它们各自授出了什么。** 那处实测到的不对称意味着一次回收不会去掉已经转授出去的授权，所以状态必须被读出来、而不是被假定。
- **查有什么被授给了 `PUBLIC`。** 每个账号都是它的成员，这让它成为可能最宽的受权者、也是评审起来最短的一个。
- **审应用账号上的 `ANY` 类权限。** `SELECT ANY TABLE` 与 `EXECUTE ANY PROCEDURE` 按定义就是跨 schema 的，因为一个 schema 就是一个用户。
- **清点 `UTL_HTTP`、`UTL_TCP`、`UTL_FILE`、`DBMS_SCHEDULER` 上的执行权限，以及与之配套的网络 ACL。** 合在一起，它们就是从一条数据库账号通往服务器网络与文件系统的路，而 ACL 才是决定那条路开不开的控制点。
- **把默认账号、监听器版本、以及监听器自身的配置当成三件分别的事来查**，因为它们与数据库是分开打补丁、分开配置的。
- **盯存储代码里的 `EXECUTE IMMEDIATE`、以及任何由拼接建起来的动态语句**，并确认用的是绑定变量而不是文本。
- **对来自应用连接的 DDL 告警**，因为它会提交包着它的事务，并会在别的代码会去解析的那个 schema 里建出名字空间。
- **缓解上，按对象授权而不是用 `ANY`**，并让应用账号待在自己的 schema 里，这样它的名字空间不会和它并不拥有的代码共用。
- **回收时把跟进做成流程的一部分**：先找出持有授权选项的账号、再回收、然后核实下游的授权没了 —— 而没了的那些要显式回收。
- **不要部署依赖"空串与 null 有区别"的代码。** 在那个差别会决定什么的地方，用一个显式的哨兵值加一条 `NOT NULL` 约束，让"缺失"与"空"是两个状态而不是一个。
- **默认关闭出站网络访问，并按主机、按账号加 ACL**，而不是授出一个网络包的执行权限、然后把这当成那个决定。
- **并且在评审里把空串、null、未设置字段当成一个问题**，因为在这个引擎上它们是同一个值，而在别处写的代码认为它们是三个。
