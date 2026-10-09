---
id: db-dialects-and-features
title_en: SQL Dialects
title_zh: 各数据库的方言与差异
summary_en: Standard SQL is a reference rather than a runnable dialect, and every engine implements it its own way. Measured against two live engines — what a column type actually enforces, how quotes and backslashes are read, and which of the names for one operation exists where.
summary_zh: 标准 SQL 是一份参考，不是一种能直接跑的方言，而每一家都有自己的实现方式。这一篇对着两个活的引擎实测 —— 列类型到底约束了什么、引号与反斜杠怎么被理解、以及同一件事的几个名字各自在哪一家存在。
tags: [beginner, database, sql, mysql, postgresql, mssql, oracle, sqlite]
tools: [sqlite3, psql, mysql, sqlcmd, python3]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A standard everyone implements differently

There is an ISO standard for SQL, and there is no database that implements only it. Every engine is a **dialect**: the standard plus its own types, its own function names, its own defaults, and its own history.

> **Standard SQL is a reference, not a runnable dialect.** Writing "portable SQL" means knowing which parts are shared and where the engines diverge.

The divergence is not random. It comes from three places: features added before the standard caught up, features added for performance, and defaults chosen decades ago that cannot be changed now without breaking every existing application. The last category is where the surprises live, because a default is invisible in the code that depends on it.

Measured below, on two engines actually running: **SQLite 3.53** in-process, and **PostgreSQL 18.4** over the local socket. When a third engine differs in a way neither of those shows, it is named — because the point of the entry is the shape of the differences, not a table of one pair.

### Part 1: why dialects exist, and what it costs to ignore them

| Source of difference | Example |
|---|---|
| Added before standardisation | SQLite's type affinity; MySQL's backslash escapes |
| Added for a use case | PostgreSQL's arrays and JSON operators; Oracle's hierarchical queries |
| A default chosen long ago | case sensitivity of identifiers; whether `||` concatenates |
| Storage or engine design | where `SEQUENCE` lives; whether DDL is transactional |

**And the cost of ignoring them is not a crash.** Most dialect differences produce a **different answer**, not an error:

| What diverges | What happens instead of an error |
|---|---|
| Type coercion | a value is stored as text in a numeric column |
| Comparison rules | a filter matches a different set of rows |
| Collation | `'a' = 'A'` is true in one place and false in another |
| Identifier folding | a query names a table that exists under another name |
| Transaction handling | a rollback succeeds, or the change was already committed |

**That is the property worth carrying into the rest of this entry**: a dialect difference is usually silent. A statement that fails loudly is easy to find and fix; a statement that succeeds and means something else is not.

### Part 2: a column type is a constraint in one engine and a hint in another

The clearest measured difference, and the one with the widest consequences. Inserting into an `INTEGER` column:

| Statement | SQLite 3.53 | PostgreSQL 18.4 |
|---|---|---|
| `INSERT ... VALUES ('123')` | **accepted** — stored as `integer` | accepted, converted to a number |
| `INSERT ... VALUES ('abc')` | **accepted** — stored as `text` | **refused** — `invalid input syntax for type integer: "abc"` |
| `SELECT typeof(n)` after both | `integer\|123` then `text\|abc` | no such function; the type is the column's |

**SQLite's column types are an affinity, not a constraint.** The declared type tells the engine how to *prefer* to store a value; a string that looks like a number is stored as a number, and a string that does not is stored as text, **in the same column**. So a column declared `INTEGER` can end up holding text, and every later comparison and calculation has to deal with that.

**PostgreSQL is strongly typed.** The value is converted on the way in if the conversion is defined, and refused if it is not. The column type is a statement about what the column contains.

The other engines sit between these two positions, and the setting that decides it is worth knowing by name:

| Engine | Behaviour |
|---|---|
| SQLite | affinity — anything can go anywhere |
| PostgreSQL | strong typing, conversion only where defined |
| MySQL / MariaDB | **strict mode** refuses, non-strict mode coerces with a warning — and strict is the default in current versions |
| SQL Server | strong typing, with implicit conversion rules in expressions |
| Oracle | strong typing; `''` is `NULL`, which surprises everyone |

**"Which mode is this server in" is a question that changes what the application stores**, and it is a deployment setting rather than something visible in the query.

### Part 3: comparing different types

Measured, the same four expressions:

| Expression | SQLite | PostgreSQL |
|---|---|---|
| `'1' = 1` | **`0`** — false | **`t`** — true |
| `'1' + 1` | `2` | `2` |
| `'abc' + 1` | **`1`** — the text counts as 0 | **error** — `invalid input syntax for type integer` |
| `1 = 1.0` | `1` — true | `t` — true |

**The two engines disagree about equality and agree about arithmetic**, and the reason is a design choice rather than a bug:

**SQLite compares values of different types without converting them.** In its ordering, numbers sort before text, so the string `'1'` and the number `1` are simply different values and never equal. But arithmetic has to produce a number, so `'1' + 1` converts and gives 2 — and `'abc' + 1` converts the text to 0 and quietly gives 1.

**PostgreSQL converts the literal to a type that makes the comparison meaningful**, so `'1' = 1` is true. Arithmetic likewise, but `'abc'` cannot be an integer, so it refuses instead of guessing.

**Two readings follow, and both matter.** A condition like `WHERE token = '1'` can match a row in one engine and not in another. And an arithmetic expression over text either produces a number or stops the query, depending on where it runs — which is the difference between a silent wrong answer and a visible failure.

### Part 4: quoting, escaping, and the character that means three things

**Double quotes.** In the standard they delimit an **identifier**; single quotes delimit a string. Measured:

| Statement | SQLite | PostgreSQL |
|---|---|---|
| `SELECT "abc"` | **`abc`** — falls back to treating it as a string | **error** — `column "abc" does not exist` |
| `SELECT "n" FROM d1` | `123` — `n` is a real column | `123` |

**SQLite accepts double quotes as a string when no such column exists**, which is a compatibility measure from before it followed the standard. **PostgreSQL is strict**: double quotes always mean an identifier. **MySQL treats double quotes as a string by default** unless `ANSI_QUOTES` is enabled. So one character has three meanings across three engines, and a query that works on one may be a string literal, a column reference, or an error on the next.

**Backslashes.** This is the difference with the longest history in injection, and both measured engines are on the same side:

| Statement | SQLite | PostgreSQL |
|---|---|---|
| `SELECT length('\n')` | **2** | **2** |
| `SELECT '\n' = '\' \|\| 'n'` | true | true |

**Neither treats a backslash as an escape character**: `'\n'` is a backslash followed by the letter n, two characters, not a newline. **MySQL does treat it as an escape by default**, so the same literal is one character there — and `\'` is a single quote rather than the end of the string.

**That is why the same concatenation code produces different strings on different engines**, and it is why escaping routines are engine-specific. A function that escapes for MySQL is not correct for PostgreSQL, and vice versa; the standard's own answer is to represent a quote by doubling it, which is why `standard_conforming_strings` being on by default in PostgreSQL matters.

**And there is a mode for everything.** MySQL can be told not to treat backslash as an escape (`NO_BACKSLASH_ESCAPES`), which changes the meaning of every string the application builds. Between "which engine" and "which mode", the assumption a piece of code makes about escaping is a property of the deployment.

### Part 5: one operation, several names

Most of what people call "dialect differences" is naming rather than capability. Every engine can concatenate rows into a string; they disagree about what to call it:

| Operation | SQLite | PostgreSQL | MySQL | SQL Server | Oracle |
|---|---|---|---|---|---|
| concatenate two strings | `\|\|` | `\|\|` | `CONCAT()` (or `\|\|` with a mode) | `+` | `\|\|` |
| concatenate rows in a group | `group_concat` | **`string_agg`** | `group_concat` | `string_agg` | `listagg` |
| format a date | `strftime` | **`to_char`** | `date_format` | `format` / `convert` | `to_char` |
| limit rows | `LIMIT` | `LIMIT` / `FETCH FIRST` | `LIMIT` | `TOP` / `OFFSET…FETCH` | `FETCH FIRST` / `ROWNUM` |
| auto-increment | `INTEGER PRIMARY KEY` | `GENERATED … AS IDENTITY` / `serial` | `AUTO_INCREMENT` | `IDENTITY` | `SEQUENCE` + trigger |
| current time | `datetime('now')` | `now()` | `now()` | `getdate()` | `sysdate` |
| substring | `substr` | `substr` / `substring` | `substring` | `substring` | `substr` |

Measured across the two live engines, the pattern is that each name exists in one place and not the other:

| Statement | SQLite | PostgreSQL |
|---|---|---|
| `SELECT 'a' \|\| 'b'` | `ab` | `ab` |
| `... group_concat(s) ...` | `x,y` | **error** — `function group_concat(text) does not exist` |
| `... string_agg(s, ',') ...` | **error** | `x,y` |
| `SELECT strftime('%Y-%m', '2026-10-08')` | `2026-10` | **error** — no such function |
| `SELECT to_char(date '2026-10-08', 'YYYY-MM')` | **error** — the `date '…'` literal is not accepted | `2026-10` |
| `SELECT 1 LIMIT 1 OFFSET 0` | `1` | `1` |
| `SELECT 1 FETCH FIRST 1 ROWS ONLY` | **error** — syntax | `1` |

**The practical reading is that porting SQL is mostly translation, not redesign.** The capability exists everywhere; what changes is the name, the argument order, and the format string. Which is why the expensive part of a migration is rarely "the database cannot do this" and usually "somebody has to translate three hundred queries correctly, including the ones generated by an ORM".

**And the format strings themselves are a dialect.** `%Y-%m`, `YYYY-MM` and `%Y-%m` mean the same output in three different engines, so a date written into a report can be correct in one deployment and wrong in another without any error.

### Part 6: identifiers, case, and the quotes that freeze them

Measured:

| Engine | Behaviour |
|---|---|
| SQLite | case-insensitive — a table created as `Orders` is found as `orders` |
| PostgreSQL | unquoted identifiers are **folded to lower case**; quoted ones keep their case exactly |
| Oracle | unquoted identifiers are folded to **upper case** |
| MySQL | depends on the platform: table names are case-sensitive on Linux by default and insensitive on Windows |
| SQL Server | depends on the database's collation |

Measured on PostgreSQL, which shows the rule most clearly:

```
CREATE TABLE pgcase …          then   SELECT count(*) FROM PGCASE   -> 0 rows, no error
CREATE TABLE "pGMixed" …       then   SELECT count(*) FROM "pGMixed" -> 0 rows, no error
                                      SELECT count(*) FROM pgmixed   -> ERROR: relation "pgmixed" does not exist
```

**An unquoted identifier is not a name, it is a name that will be folded.** `pgcase` and `PGCASE` both become `pgcase`, so they refer to the same table. A **quoted** identifier is kept exactly as written, which means `"pGMixed"` is a different table from `pgmixed` — and the second one does not exist.

**So a migration that carries quoted identifiers across engines carries the case with them**, and a query written against `"Users"` will not find a table that the same DDL created unquoted somewhere else. Writing identifiers without quotes and in one case throughout is a small discipline that removes a whole class of confusion.

### Part 7: transactions, DDL, and implicit commits

Measured, on both live engines, the same three steps inside one session:

| Engine | `BEGIN; CREATE TABLE …; ROLLBACK;` | table afterwards |
|---|---|---|
| SQLite | rolled back | **does not exist** |
| PostgreSQL | rolled back | **does not exist** |

**Both of these engines have transactional DDL**, and the measured result is the same question asked two ways: after the rollback, the count of matching tables was `0`; after a `COMMIT` of the same statement, it was `1`.

**That is not universal, and the difference is invisible in the code.**

| Engine | DDL and transactions |
|---|---|
| PostgreSQL | DDL is transactional — a schema change can be rolled back |
| SQLite | DDL is transactional |
| MySQL | **DDL causes an implicit commit** — the open transaction is committed first |
| Oracle | **DDL commits automatically** — no rollback is possible |
| SQL Server | mostly transactional, with exceptions that commit implicitly |

**"I wrapped it in a transaction" therefore means different things on different engines**, and nothing about the statement changes to say which. On MySQL an application that creates a table and then hits an error has already committed the creation; on PostgreSQL the rollback removes it.

**The autocommit default is a second layer of the same problem.** Some drivers open a transaction for you and require an explicit commit; others are in autocommit mode and every statement stands alone. The same function that is atomic against one driver is a sequence of independent statements against another — and this is not a property of the SQL, it is a property of the client library, which is exactly why it gets missed.

### Part 8: what follows for security

**Whether an injection works at all depends on the dialect**, and on the mode within it:

| Technique | Works where |
|---|---|
| `' OR '1'='1` as a tautology | depends on the comparison rules in Part 3 |
| `--` to comment out the rest | universal; **MySQL also accepts `#`** |
| `/*! … */` as an executable comment | **MySQL** — it looks like a comment and runs as code |
| Stacked statements (`; DROP …`) | only where the driver permits more than one statement per call |
| `UNION SELECT` | needs the column count and types to line up — which differs per engine and per type strictness |
| Reading or writing files | MySQL has `LOAD_FILE` / `INTO OUTFILE`; PostgreSQL needs a superuser and a server-side function; SQLite has neither |
| Error-based extraction | needs the engine's error text, which is itself a dialect |
| Time-based extraction | needs that engine's sleep or heavy-query primitive |

**So the first question after "there is an injection" is always "which engine, which version, which mode"** — because the same string that is a syntax error on one is a file write on another.

**Escaping differences are the reason escaping routines are engine-specific.** Part 4 measured two engines that do not treat a backslash as an escape; a third that does will interpret the same bytes differently, and a routine written for one is wrong for the other. **Parameterisation sidesteps the entire question**, because a value that never becomes part of the statement text has no quoting or escaping semantics to get wrong.

**Collation decides equality, and equality decides authorisation.** Part 3 measured a case where the same comparison gave opposite answers; collation does the same thing to strings. In a collation where `'a' = 'A'` holds, a login check that compares a username for equality accepts a different username — which is a classic authentication bypass, and it is a property of the column's collation rather than of the query.

| Decision | Why it matters |
|---|---|
| The column's collation | decides whether `=` is case- and accent-sensitive |
| The connection's charset | decides how bytes become characters, including in escaping |
| The server's default collation | decides what a column gets when the DDL does not say |

**A default is invisible until it differs from one environment to the next**, which is why relying on one in a comparison that grants access is a risk rather than a convenience.

**And permissions are a dialect too.** `GRANT` exists everywhere; what can be granted, whether roles exist, whether a schema is a namespace or a user, and whether the application's account can read other databases all differ. The ceiling on an injection is set by those, which is the previous entry's point arriving in a form that varies by engine.

### Detection and mitigation

- **Read the error text in the logs, because it names the engine.** A raw database error handed to a user is both a disclosure and a hint, and the same text in a log is a useful signal: errors from an engine other than the one the deployment is supposed to use mean configuration drift.
- **Alert when the same query returns different row counts in different environments.** A filter that depends on a collation, a type affinity or a case-folding rule will do exactly that, and it is often the first visible sign of a silent dialect assumption.
- **Alert on DDL appearing in production audits unexpectedly.** On engines with implicit commit, a schema change that an application thought it rolled back is live — and it will appear in the audit as having happened.
- **And watch for enumeration of the schema.** `information_schema`, `pg_catalog`, `sqlite_master` and `SHOW TABLES` are among the first things run after an injection works; the query itself is a signal even when it succeeds.
- **For mitigation, name the engine and version explicitly and use it everywhere.** Development, CI, staging and production on the same engine and the same major version removes most of the class, because the remaining differences are then documented rather than discovered.
- **Parameterise everything.** This is the one control that makes the quoting and escaping sections irrelevant: a bound value has no dialect, no escape character, and no collation applied at parse time.
- **Set charset and collation explicitly, and do not rely on defaults.** On the connection, on the columns, and on the database. A default chosen at creation time has a way of differing between a laptop and a production host.
- **Do not let a collation decide an authentication comparison.** Compare what the application meant to compare — an exact value, a hash — rather than relying on the column's default notion of equality.
- **Keep the database account's privileges minimal, and remember the ceiling varies by engine.** A read-only account on one engine may still be able to call functions that read the filesystem on another.
- **And treat "which database is this" as a first-class fact.** It belongs in the deployment documentation, in the health endpoint, and in the incident runbook — because every question about an injection's impact, an escaping routine's correctness and a migration's risk is answered differently depending on it.

<!-- lang:zh -->
### 一个每家都实现得不一样的标准

SQL 有一份 ISO 标准，而没有一个数据库只实现它。每个引擎都是一种**方言**：标准加上自己的类型、自己的函数名、自己的默认值，以及自己的历史。

> **标准 SQL 是一份参考，不是一种能直接跑的方言。** 所谓"写可移植的 SQL"，意思是知道哪些部分是共有的、哪些地方各家会分岔。

分岔不是随机的。它来自三个地方：标准追上之前就加进去的特性、为性能加的特性、以及几十年前选定的默认值 —— 后者现在不能改，一改就会弄坏所有现存应用。而意外都住在最后一类里，因为**默认值在依赖它的代码里是看不见的**。

下面是实测，用的是两个真的在跑的引擎：进程内的 **SQLite 3.53**，和通过本地套接字的 **PostgreSQL 18.4**。当第三家在某处表现出这两家都没有的行为时，会点名写出 —— 因为这一篇的重点是差异的**形状**，而不是某一对的对照表。

### 第一部分：方言为什么会存在，以及忽略它们的代价

| 差异来源 | 例子 |
|---|---|
| 标准之前就加了 | SQLite 的类型亲和性；MySQL 的反斜杠转义 |
| 为某个用途加的 | PostgreSQL 的数组与 JSON 运算符；Oracle 的层次查询 |
| 很久以前选定的默认值 | 标识符的大小写敏感性；`||` 是不是拼接 |
| 存储或引擎设计 | `SEQUENCE` 放在哪；DDL 是否事务性 |

**而忽略它们的代价不是崩溃。** 大多数方言差异产生的是**另一个答案**，不是报错：

| 哪里分岔 | 代替报错发生的事 |
|---|---|
| 类型转换 | 一个值以文本形式存进了数值列 |
| 比较规则 | 过滤条件匹配到另一批行 |
| 排序规则 | `'a' = 'A'` 在一处为真、在另一处为假 |
| 标识符折叠 | 查询点了一个以另一个名字存在的表 |
| 事务处理 | 回滚成功了，或者那个改动其实已经提交了 |

**这就是值得带进这一篇其余部分的那条性质**：方言差异通常是**安静的**。一句响亮的失败容易找到也容易修；一句成功、但意思是别的东西的语句不。

### 第二部分：列类型在一家是约束，在另一家是提示

实测里最清楚、后果也最广的一处。往一个 `INTEGER` 列里插入：

| 语句 | SQLite 3.53 | PostgreSQL 18.4 |
|---|---|---|
| `INSERT ... VALUES ('123')` | **接受** —— 存成 `integer` | 接受，转成数字 |
| `INSERT ... VALUES ('abc')` | **接受** —— 存成 `text` | **拒绝** —— `invalid input syntax for type integer: "abc"` |
| 之后 `SELECT typeof(n)` | `integer\|123`，然后是 `text\|abc` | 没有这个函数；类型就是列的类型 |

**SQLite 的列类型是一种亲和性，不是约束。** 声明的类型告诉引擎它**倾向于**怎么存一个值；看起来像数字的字符串存成数字，不像的存成文本 —— **在同一列里**。所以一个声明为 `INTEGER` 的列最后可能装着文本，而之后每一次比较与计算都得应付这件事。

**PostgreSQL 是强类型的。** 值在进入时被转换 —— 如果那个转换有定义；没有定义就拒绝。列类型是一句关于"这一列装什么"的陈述。

其他引擎落在这两个位置之间，而决定它落在哪里的那个设置值得记住名字：

| 引擎 | 行为 |
|---|---|
| SQLite | 亲和性 —— 什么都能进任何地方 |
| PostgreSQL | 强类型，只在有定义处转换 |
| MySQL / MariaDB | **严格模式**拒绝、非严格模式带警告地转换 —— 而当前版本默认是严格 |
| SQL Server | 强类型，表达式里有隐式转换规则 |
| Oracle | 强类型；`''` 就是 `NULL`，这一条让所有人意外 |

**"这台服务器在哪个模式"这个问题会改变应用存进去的东西**，而它是一个部署设置，不是查询里看得见的东西。

### 第三部分：比较不同类型时

实测，同样四条表达式：

| 表达式 | SQLite | PostgreSQL |
|---|---|---|
| `'1' = 1` | **`0`** —— 假 | **`t`** —— 真 |
| `'1' + 1` | `2` | `2` |
| `'abc' + 1` | **`1`** —— 文本被当成 0 | **报错** —— `invalid input syntax for type integer` |
| `1 = 1.0` | `1` —— 真 | `t` —— 真 |

**两个引擎在"相等"上不一致，在"算术"上一致**，而原因是一个设计选择，不是 bug：

**SQLite 比较不同类型的值时不转换它们。** 在它的排序里数字排在文本之前，所以字符串 `'1'` 和数字 `1` 就是两个不同的值、永不相等。但算术必须产出一个数字，所以 `'1' + 1` 会转换并得到 2 —— 而 `'abc' + 1` 把文本转换成 0，安静地给出 1。

**PostgreSQL 把字面量转成一个让比较有意义的类型**，所以 `'1' = 1` 为真。算术也一样，但 `'abc'` 变不成整数，于是它拒绝而不是猜。

**两个读法随之成立，而两个都要紧。** 一个像 `WHERE token = '1'` 的条件，可能在一家匹配到一行、在另一家不匹配。而一个对文本做的算术表达式，要么产出一个数字、要么让查询停下 —— 取决于它跑在哪里，而这正是"安静的错误答案"与"可见的失败"之间的差别。

### 第四部分：引号、转义，以及那个有三重含义的字符

**双引号。** 标准里它界定的是**标识符**，单引号界定字符串。实测：

| 语句 | SQLite | PostgreSQL |
|---|---|---|
| `SELECT "abc"` | **`abc`** —— 回落成把它当字符串 | **报错** —— `column "abc" does not exist` |
| `SELECT "n" FROM d1` | `123` —— `n` 真的是个列 | `123` |

**SQLite 在找不到同名列时接受双引号当字符串**，那是它遵循标准之前留下的兼容措施。**PostgreSQL 是严格的**：双引号永远是标识符。**MySQL 默认把双引号当字符串**，除非开了 `ANSI_QUOTES`。所以同一个字符在三个引擎里有三种含义，而一条在某一处能跑的查询，在下一处可能是一个字符串、一个列引用、或者一个错误。

**反斜杠。** 这是注入史上影响最久的一处差异，而实测的这两个引擎站在同一边：

| 语句 | SQLite | PostgreSQL |
|---|---|---|
| `SELECT length('\n')` | **2** | **2** |
| `SELECT '\n' = '\' \|\| 'n'` | 真 | 真 |

**两家都不把反斜杠当转义符**：`'\n'` 是一个反斜杠加字母 n，两个字符，不是一个换行。**MySQL 默认把它当转义符**，所以同一个字面量在那里是一个字符 —— 而 `\'` 是一个单引号，不是字符串的结束。

**这就是同一段拼接代码在不同引擎上产出不同字符串的原因**，也是转义函数必须按引擎来写的原因。一个为 MySQL 写的转义函数对 PostgreSQL 不正确，反之亦然；标准自己的答案是"把引号写两次"来表示一个引号 —— 这就是 PostgreSQL 里 `standard_conforming_strings` 默认为开这件事要紧的原因。

**而每个东西都有一个模式。** MySQL 可以被设置成不把反斜杠当转义符（`NO_BACKSLASH_ESCAPES`），而那会改变应用拼出的每一个字符串的含义。在"哪个引擎"与"哪个模式"之间，一段代码关于转义的假设，是那个部署的性质。

### 第五部分：同一件事，几个名字

人们说的"方言差异"里大部分其实是命名，不是能力。每个引擎都能把多行拼成一个字符串，只是对它的叫法不一致：

| 操作 | SQLite | PostgreSQL | MySQL | SQL Server | Oracle |
|---|---|---|---|---|---|
| 拼接两个字符串 | `\|\|` | `\|\|` | `CONCAT()`（开了模式后也可 `\|\|`） | `+` | `\|\|` |
| 拼接一组行 | `group_concat` | **`string_agg`** | `group_concat` | `string_agg` | `listagg` |
| 格式化日期 | `strftime` | **`to_char`** | `date_format` | `format` / `convert` | `to_char` |
| 限制行数 | `LIMIT` | `LIMIT` / `FETCH FIRST` | `LIMIT` | `TOP` / `OFFSET…FETCH` | `FETCH FIRST` / `ROWNUM` |
| 自增 | `INTEGER PRIMARY KEY` | `GENERATED … AS IDENTITY` / `serial` | `AUTO_INCREMENT` | `IDENTITY` | `SEQUENCE` + 触发器 |
| 当前时间 | `datetime('now')` | `now()` | `now()` | `getdate()` | `sysdate` |
| 子串 | `substr` | `substr` / `substring` | `substring` | `substring` | `substr` |

在两个活的引擎上实测，模式就是"每个名字只存在于一处、不存在于另一处"：

| 语句 | SQLite | PostgreSQL |
|---|---|---|
| `SELECT 'a' \|\| 'b'` | `ab` | `ab` |
| `... group_concat(s) ...` | `x,y` | **报错** —— `function group_concat(text) does not exist` |
| `... string_agg(s, ',') ...` | **报错** | `x,y` |
| `SELECT strftime('%Y-%m', '2026-10-08')` | `2026-10` | **报错** —— 没有这个函数 |
| `SELECT to_char(date '2026-10-08', 'YYYY-MM')` | **报错** —— 不接受 `date '…'` 这种字面量 | `2026-10` |
| `SELECT 1 LIMIT 1 OFFSET 0` | `1` | `1` |
| `SELECT 1 FETCH FIRST 1 ROWS ONLY` | **报错** —— 语法 | `1` |

**实用的读法是：迁移 SQL 主要是翻译，不是重新设计。** 能力到处都有，变化的是名字、参数顺序、以及格式串。这就是为什么一次迁移里贵的部分很少是"这个数据库做不到"，而通常是"有人得把三百条查询正确地翻译一遍，包括 ORM 生成的那些"。

**而格式串本身也是一种方言。** `%Y-%m`、`YYYY-MM` 和 `%Y-%m` 在三个不同引擎里表示同一个输出，所以写进报表的一个日期可以在一个部署里正确、在另一个部署里错误，而全程没有任何报错。

### 第六部分：标识符、大小写，以及把它们冻住的那个引号

实测：

| 引擎 | 行为 |
|---|---|
| SQLite | 大小写不敏感 —— 建为 `Orders` 的表用 `orders` 也找得到 |
| PostgreSQL | 未加引号的标识符**折叠成小写**；加了引号的保留原样 |
| Oracle | 未加引号的标识符折叠成**大写** |
| MySQL | 取决于平台：Linux 上表名默认区分大小写，Windows 上不区分 |
| SQL Server | 取决于那个数据库的排序规则 |

在 PostgreSQL 上实测，它把这条规则显示得最清楚：

```
CREATE TABLE pgcase …          然后  SELECT count(*) FROM PGCASE   -> 0 行，无报错
CREATE TABLE "pGMixed" …       然后  SELECT count(*) FROM "pGMixed" -> 0 行，无报错
                                    SELECT count(*) FROM pgmixed   -> ERROR: relation "pgmixed" does not exist
```

**未加引号的标识符不是一个名字，它是一个"将会被折叠"的名字。** `pgcase` 和 `PGCASE` 都会变成 `pgcase`，所以它们指同一张表。而**加了引号**的标识符原样保留，这意味着 `"pGMixed"` 与 `pgmixed` 是两张不同的表 —— 而第二张不存在。

**所以一次带着引号标识符的迁移会把大小写一起带过去**，而一条针对 `"Users"` 写的查询，找不到别处用同一份 DDL 但不加引号建出来的表。全程不写引号、并保持同一种大小写，是一个能消掉一整类困惑的小纪律。

### 第七部分：事务、DDL，与隐式提交

实测，在两个活的引擎上，同一个会话里的同样三步：

| 引擎 | `BEGIN; CREATE TABLE …; ROLLBACK;` | 之后表在吗 |
|---|---|---|
| SQLite | 被回滚 | **不存在** |
| PostgreSQL | 被回滚 | **不存在** |

**这两家都支持事务性 DDL**，而实测结果是同一个问题用两种方式问出来的：回滚之后，匹配的表计数是 `0`；同样语句 `COMMIT` 之后，是 `1`。

**这不是普遍的，而那个差异在代码里看不见。**

| 引擎 | DDL 与事务 |
|---|---|
| PostgreSQL | DDL 是事务性的 —— 一次结构变更可以回滚 |
| SQLite | DDL 是事务性的 |
| MySQL | **DDL 引起隐式提交** —— 当前开着的事务会先被提交 |
| Oracle | **DDL 自动提交** —— 回滚不可能 |
| SQL Server | 大部分是事务性的，但有会隐式提交的例外 |

**所以"我把它包在事务里了"在不同引擎上意思不同**，而语句里没有任何东西会说明是哪一种。在 MySQL 上，一个建了表然后遇到错误的应用，那次创建已经提交了；在 PostgreSQL 上，回滚会把它去掉。

**自动提交的默认值是同一个问题的第二层。** 有些驱动会替你开一个事务、并要求显式提交；另一些处于自动提交模式、每条语句各自独立。同一个函数，对着一个驱动是原子的，对着另一个是一串互相独立的语句 —— 而这不是 SQL 的性质，是客户端库的性质，这也正是它容易被漏掉的原因。

### 第八部分：从这些差异推出的安全观念

**一次注入到底成不成立，取决于方言 —— 以及方言里的模式：**

| 手法 | 在哪里成立 |
|---|---|
| `' OR '1'='1` 这种恒真 | 取决于第三部分里的比较规则 |
| 用 `--` 注释掉后面 | 通用；**MySQL 还认 `#`** |
| `/*! … */` 这种可执行注释 | **MySQL** —— 它看起来是注释，跑起来是代码 |
| 堆叠语句（`; DROP …`） | 只在驱动允许"一次调用多条语句"的地方 |
| `UNION SELECT` | 需要列数与类型对得上 —— 而各引擎、各类型严格程度都不同 |
| 读写文件 | MySQL 有 `LOAD_FILE` / `INTO OUTFILE`；PostgreSQL 需要超级用户与服务端函数；SQLite 两者都没有 |
| 报错注入 | 需要那个引擎的报错文本，而它本身就是一种方言 |
| 时间盲注 | 需要那个引擎的 sleep 或重查询原语 |

**所以"存在注入"之后的第一个问题永远是"哪个引擎、哪个版本、哪个模式"** —— 因为同一段字符串在一家是语法错误、在另一家是文件写入。

**转义差异是"转义函数必须按引擎写"的原因。** 第四部分实测了两个不把反斜杠当转义符的引擎；第三个把它当转义符的引擎会把同样的字节解释成别的东西，而为前者写的函数对后者是错的。**参数化直接绕开整个问题**，因为一个从不成为语句文本一部分的值，没有任何引号或转义语义可以被弄错。

**排序规则决定相等，而相等决定授权。** 第三部分实测了一个"同一个比较给出相反答案"的情形；排序规则对字符串做的是同一件事。在一个 `'a' = 'A'` 成立的排序规则里，一个按相等比较用户名的登录检查会接受另一个用户名 —— 那是经典的认证旁路，而它是列的排序规则的性质，不是查询的性质。

| 决定 | 为什么要紧 |
|---|---|
| 列的排序规则 | 决定 `=` 是否区分大小写与重音 |
| 连接的字符集 | 决定字节怎么变成字符，包括在转义里 |
| 服务器的默认排序规则 | 决定 DDL 没写时列会拿到什么 |

**一个默认值在它与下一个环境不同之前是看不见的**，这就是"在一个授予访问权的比较里依赖默认值"是一份风险而不是一个方便的原因。

**而权限也是一种方言。** `GRANT` 到处都有；能授予什么、有没有角色、schema 是一个命名空间还是一个用户、以及应用那个账号能不能读别的库，全都不一样。一次注入的上限由这些设定，而这是上一篇的论点以一种按引擎变化的形式到来。

### 检测与缓解

- **读日志里的报错文本，因为它会说出引擎的名字。** 一条原样交给用户的数据库报错既是一次泄漏也是一条提示，而同样一段文本留在日志里是一个有用的信号：来自"这个部署本不该用的那个引擎"的报错，意味着配置漂移。
- **当同一条查询在不同环境返回不同行数时告警。** 一个依赖排序规则、类型亲和性或大小写折叠的过滤条件就会这样，而它常常是"一个安静的方言假设"最早的可见迹象。
- **对生产审计里意外出现的 DDL 告警。** 在使用隐式提交的引擎上，一次应用以为被回滚掉的结构变更已经生效了 —— 而它会在审计里表现为"发生过"。
- **并且盯表结构的枚举。** `information_schema`、`pg_catalog`、`sqlite_master` 和 `SHOW TABLES` 是注入成功之后最早跑的东西之一；那条查询本身就是信号，即使它成功返回。
- **缓解上，明确写下引擎与版本，并在所有地方都用它。** 开发、CI、预发布、生产用同一个引擎、同一个大版本，能消掉这一类里的绝大部分，因为剩下的差异就变成"有文档可查"而不是"靠发现"。
- **把一切都参数化。** 这是唯一一项让引号与转义那两节变得无关的控制：一个被绑定的值没有方言、没有转义符，也没有在解析期被施加的排序规则。
- **显式设置字符集与排序规则，不要依赖默认值。** 在连接上、在列上、在数据库上。一个在建库时选定的默认值，很擅长在笔记本与生产主机之间变得不一样。
- **不要让排序规则去决定一次认证比较。** 比较应用本想比较的东西 —— 一个精确值、一个哈希 —— 而不是依赖列默认的"相等"观念。
- **把数据库账号的权限压到最小，并记住这个上限按引擎而变。** 在一家只读的账号，在另一家可能仍能调用读文件系统的函数。
- **并且把"这是哪个数据库"当成一个一等事实。** 它属于部署文档、健康检查端点、以及事故手册 —— 因为关于"一次注入的影响、一个转义函数的正确性、一次迁移的风险"的每一个问题，答案都随它而变。
