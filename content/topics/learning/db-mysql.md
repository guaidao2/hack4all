---
id: db-mysql
title_en: MySQL and MariaDB
title_zh: MySQL 与 MariaDB
summary_en: The most deployed relational database, and the one whose defaults surprise people most. Measured on a throwaway instance — the escape character that differs from other engines, a collation where two different letters compare equal, a storage engine that ignores a rollback, and a comment form that runs.
summary_zh: 部署最广的关系数据库，也是默认值最容易让人意外的那一个。这一篇在一个临时实例上实测 —— 与其他引擎不同的转义字符、一个"两个不同字母相等"的排序规则、一个无视回滚的存储引擎，以及一种会执行的注释形式。
tags: [beginner, database, mysql, mariadb, sql, collation, innodb]
tools: [mysql, mariadb, mysqldump, python3]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The database most people meet first

MySQL is the default relational database of the web: every shared host, most PHP applications, a large share of everything else. MariaDB is its fork, protocol-compatible and used by many distributions as the replacement.

That popularity is why its **defaults** matter more than its features. Almost every surprise in this entry is a default someone chose years ago and cannot now change: an escape character, a case-insensitive collation, a strict mode that is on in current versions and off in old ones, a storage engine that used to be the default and does not support transactions.

**And the reason to know the defaults is the same as in any engine: a default is invisible in the code that depends on it.** A migration that ran on one version and behaves differently on another did not change a line.

Measured below on a throwaway **MariaDB 11.8.6** instance, started on a temporary socket with a temporary data directory and removed afterwards. Where the behaviour is shared with MySQL proper it applies to both; where the two forks differ it is noted.

### Part 1: the shape of it

Four terms, and getting them right avoids most confusion:

| Term | Means |
|---|---|
| **server** | `mariadbd` / `mysqld` — the process that owns the data |
| **client** | `mariadb` / `mysql` — the program you type into |
| **database** (schema) | a namespace of tables; **the two words are synonyms here** |
| **account** | **`user` @ `host`** — not just a name |

**The account being a pair is the first thing that differs from other engines.** Measured, on a fresh install the accounts are:

```
kali@localhost   mariadb.sys@localhost   root@127.0.0.1   root@::1   root@kali
```

**`root@localhost` and `root@127.0.0.1` are two accounts.** They can have different passwords and different privileges, and which one you are is decided by where the connection came from. That is why a grant that appears correct can fail: it was given to the wrong host, and a connection over TCP rather than a socket is a different account.

**And a storage engine is chosen per table.** The table is not just a set of rows; the code that reads and writes it is a plugin:

| Engine | Transactions | Notes |
|---|---|---|
| **InnoDB** | yes | the default, and the one to use |
| MyISAM | **no** | historical default; table-level locking, no rollback |
| MEMORY | no | lives in RAM |
| Aria, Archive, CSV, … | varies | special purposes |

### Part 2: an escape character that no other engine has

The measurement that surprised me most, because it makes the same literal mean different things on different engines. Measured:

| Expression | MariaDB | SQLite | PostgreSQL |
|---|---|---|---|
| `length('\n')` | **1** | 2 | 2 |
| `length('\\')` | **1** | 2 | 2 |

**In MySQL a backslash is an escape character inside a string literal**, so `'\n'` is a newline — one character — rather than a backslash followed by the letter n. **In SQLite and PostgreSQL it is an ordinary character**, and the same literal is two characters.

**Why this matters beyond string lengths:** it decides what an escaping routine has to do. A function written for MySQL escapes `'` as `\'`; against PostgreSQL the same output is a backslash followed by the end of the string, which is a different string and a syntax risk. **The standard's own answer is to double the quote** (`''`), which works everywhere, and MySQL can be told to stop treating the backslash specially:

```sql
SET SESSION sql_mode = CONCAT(@@sql_mode, ',NO_BACKSLASH_ESCAPES');
```

**So "is the backslash an escape" is a per-deployment fact**, and it decides whether `\'` is a quote or an escaped backslash followed by a terminator. This is the mechanism behind the whole family of escaping bypasses: not a bug in MySQL, but two correct implementations disagreeing about the same byte.

### Part 3: comparing values, and the collation that decides it

MySQL converts types freely, and the direction it converts in shows up in the answers:

| Expression | Result | Reading |
|---|---|---|
| `'1' = 1` | **1** (true) | the string is converted to a number |
| `'1' + 1` | **2** | arithmetic converts |
| `'abc' + 1` | **1** | **non-numeric text becomes 0, silently** |
| `'abc' = 0` | **1** (true) | the same conversion, in a comparison |

**Non-numeric text converting to zero is the interesting row.** It is the same behaviour SQLite has and the opposite of PostgreSQL, which refuses. A filter like `WHERE code = 0` therefore matches rows whose `code` is `'abc'` — which is either exactly what you meant or a category error, and nothing tells you which.

**Then the part almost nobody expects.** Measured, with the server's default collation:

| Expression | Result |
|---|---|
| `'a' = 'A'` | **1** — true |
| `'a' = 'A' COLLATE utf8mb4_bin` | 0 — false |
| `'a' = 'A' COLLATE utf8mb4_general_ci` | 1 — true |
| **`'ß' = 'ss'`** | **1 — true** |

**Two different strings compare equal, because a collation is a definition of equality**, not a note about sorting. `utf8mb4_general_ci` is case-insensitive (`ci`) and accent-insensitive (`ai`); the measured server default was `utf8mb4_uca1400_ai_ci`, and under UCA the German sharp s equals `ss`.

**This is why a login check that compares a username with `=` is not necessarily doing what it looks like.** In a case-insensitive collation, `admin` and `ADMIN` are the same string as far as the comparison is concerned — and so are `Admin` and, in the measured case, a pair of letters that render differently. The control is to compare something whose equality you defined: a `BINARY` collation, a hash, or a value normalised by the application.

**And the character set is a separate decision with its own trap.** Measured:

| Expression | `LENGTH` | `CHAR_LENGTH` |
|---|---|---|
| `'a'` | 1 | 1 |
| `'中'` | **3** | 1 |
| `'𝄞'` (U+1D11E) | **4** | 1 |

`LENGTH` counts **bytes**, `CHAR_LENGTH` counts characters — which is why they disagree, and why a `VARCHAR(10)` sized in characters can hold ten wide characters and forty bytes.

**And MySQL's `utf8` is not UTF-8.** Its maximum bytes per character, read from the server's own table:

| Character set | Max bytes per character |
|---|---|
| `latin1` | **1** |
| **`utf8`** (reported as `utf8mb3`) | **3** |
| **`utf8mb4`** | **4** |

**So `utf8` is a three-byte subset that cannot store a four-byte character at all**, and `utf8mb4` is the real thing. Measured, inserting the four-byte character from the table above into a `utf8mb3` column:

```
ERROR 1366 (22007): Incorrect string value: '\xF0\x9D\x84\x9E' for column `probe`.`cs`.`a` at row 1
```

**The error names the four bytes it refused**, which is a useful reminder that the column limit is enforced in bytes — a `VARCHAR(10)` holds ten of those, forty bytes, and the character count is a separate question. A column declared `utf8` and expected to hold text from outside the basic multilingual plane — an emoji, an extension ideograph — will refuse the write when strict mode is on, and behave differently when it is off.

### Part 4: strict mode, and what happens without it

`sql_mode` is a list of switches, and the meaningful one is `STRICT_TRANS_TABLES`. Measured, the packaged default:

```
STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION,...
```

Inserting a five-character value into a three-character column, with and without it, **in the same session**:

| Mode | Result |
|---|---|
| default (strict) | **refused** — `ERROR 1406 (22001): Data too long for column 's'` |
| after `SET SESSION sql_mode=''` | **accepted**, and stored as **`ghi`**, length 3 |

**The same statement, two outcomes: a refusal or a silent truncation.** In non-strict mode the value is cut to fit and a warning is recorded, and an application that does not read warnings stores mangled data while believing it succeeded. That is the shape of the whole setting:

> **Strict mode decides whether the database tells you that it changed your data.**

**And the reason it is a per-deployment fact rather than a language feature** is history: strict mode is the default in current versions and was not in older ones, so the same application on the same schema behaves differently depending on the server it lands on.

### Part 5: transactions, and the one that silently is not one

Two measured results, both about the atomicity of a rollback.

**Storage engines disagree about whether rollback exists.** Creating one table with each engine, inserting a row, and rolling back **within one session**:

| Engine | After `ROLLBACK` |
|---|---|
| **InnoDB** | **0 rows** — the insert was undone |
| **MyISAM** | **1 row** — the insert is still there |

**The statement is identical and the outcome is not.** MyISAM does not implement transactions; a `ROLLBACK` against it is accepted and does nothing, which is worse than an error because the application believes it undid something. And the account matters here too: the same session's `START TRANSACTION` on a MyISAM table is a no-op, so a service that mixes both engines has atomicity in some code paths and not others.

**And DDL commits your transaction for you.** Measured, in one session:

```
START TRANSACTION;
INSERT INTO counter VALUES (1);
  COUNT(*) -> 1
CREATE TABLE ddl_here (id INT);     -- the DDL
ROLLBACK;
  COUNT(*) -> 1                     -- still there
```

**The `ROLLBACK` did not undo the insert**, because the `CREATE TABLE` committed the open transaction before running. So this pattern is broken:

```
BEGIN;  -- migrate: create a table, copy rows, change a column
        -- if anything fails: rollback
```

**On MySQL, a migration that fails halfway cannot be rolled back**, because every `ALTER` and `CREATE` along the way committed everything before it. On PostgreSQL the same sequence is one transaction and does roll back — the previous entry measured that. **The code looks the same and the recovery story is not.**

### Part 6: doing it yourself

A throwaway instance needs three commands and no configuration changes:

```bash
mariadb-install-db --datadir=/tmp/mdb --user=root \
    --auth-root-authentication-method=normal
mariadbd --datadir=/tmp/mdb --user=root --socket=/tmp/mdb.sock \
    --port=3308 --bind-address=127.0.0.1 --log-error=/tmp/mdb.err &
mariadb --socket=/tmp/mdb.sock -u root -e "SELECT VERSION()"
```

**`--user=root` is required** when running as root: the server refuses to start as root otherwise, and the refusal appears in the error log rather than on stdout.

Three habits worth having from the start:

```sql
SELECT @@sql_mode;                          -- which behaviours are switched on
SHOW VARIABLES LIKE 'collation%';            -- what equality means here
SELECT user, host FROM mysql.user;           -- who can connect, and from where
EXPLAIN SELECT ... ;                         -- how it intends to find the rows
```

**And read the error log, not just the client's output.** Every failure in the preparation of this entry was reported as a generic client error; the specific reason — including the successful start that was mistaken for a failure — was only in `--log-error`.

### Part 7: the traps everyone walks into

| Trap | What happens |
|---|---|
| `SELECT 'a' \|\| 'b'` | **logical OR, not concatenation** — returns `0`; use `CONCAT()` |
| `utf8` column and a four-byte character | **refused** — `ERROR 1366 Incorrect string value`; `utf8` is a three-byte subset, use `utf8mb4` |
| `LENGTH` vs `CHAR_LENGTH` | bytes vs characters — they differ for anything non-ASCII |
| `WHERE s = 'a'` matching `'A'` | the collation is case-insensitive by default |
| `ROLLBACK` on a MyISAM table | accepted, does nothing |
| `BEGIN; ALTER TABLE …; ROLLBACK;` | the DDL already committed |
| Non-strict mode and an overlong value | silently truncated, with a warning nobody reads |
| Comparing a value to `NULL` | never true, as in every engine |
| `#` in a query | a comment in MySQL and a syntax error in most others |
| An account granted to the wrong host | `'app'@'localhost'` does not authorise `'app'@'10.0.0.5'` |

### Part 8: what follows for security

**A collation that ignores case is an authentication decision nobody made.** The measured `'a' = 'A'` and `'ß' = 'ss'` are the mechanism behind the classic bypass: a uniqueness check or a login comparison on a `ci` column treats two different strings as one. **Wherever equality is used to grant something, the collation is part of the control** — and the fix is to compare a value with a defined equality, not to add a check somewhere else.

**Escaping is engine-specific because the escape character is.** Part 2 measured a backslash that is special here and ordinary elsewhere. A routine that escapes for one engine produces a different string on another, and the difference is invisible until it is a bypass. **Parameterised queries make the question disappear**, because a bound value never becomes part of the statement text.

**A comment can be code.** Measured:

| Statement | Result |
|---|---|
| `SELECT /*!50600 2 */` | `2` — **the content ran** |
| `SELECT /*!99999 1 */` | syntax error — the content did not run, leaving `SELECT` alone |
| `SELECT /*! 50600 2 */` | syntax error — **a space after `/*!` makes it an ordinary comment** |
| `SELECT 'a' /*!50600 , 'b' */` | `a`, `b` — **a column was added** |

**The version number must follow `/*!` immediately**, and the content is executed when the server's version is at least that number. Which means:

**A filter that strips comments can remove the version test and leave the payload**, or remove the payload and leave a broken statement — both wrong, and both different from what the same filter produces on another engine.

**And an attacker can use it to make a statement version-dependent**: the same bytes are a two-column query on one version and a one-column query on another, which changes what a `UNION` has to match.

**The privilege model is a pair, and that is the thing to check.** Grants are to `user@host`, so "the application account" is really a set of accounts, and a permission given to the socket account does not follow the application when it connects over TCP. **Combined with the storage engine, the blast radius has two dimensions nobody looks at**: which account, and which tables are on an engine that cannot undo.

**And the migration problem is a security problem.** A failed migration on MySQL can leave the schema halfway, because the DDL committed as it went; on PostgreSQL the same failure rolls back. **An incident plan that assumes "we rolled it back" is only correct on one of them.**

### Detection and mitigation

- **Read the error log, and keep it.** Measured, every diagnostic that mattered was in `--log-error` and not in the client's output — including a successful start that looked like a failure. The log is also where a refused start as root is reported.
- **Alert on `sql_mode` changes at runtime.** A session or global change to strict mode changes whether the application's writes are refused or silently truncated; it is a configuration change with a data-correctness consequence.
- **Alert on non-InnoDB tables appearing in a schema.** A table on an engine without transactions silently breaks every rollback in the code that touches it, and the table definition is where that is visible.
- **Watch failed migrations, and never assume a rollback happened.** On this engine a DDL statement commits the open transaction, so "we rolled back" is not a property of the script — it is a property of the engine, and only on some of them.
- **For mitigation, use InnoDB unless there is a specific reason not to**, and check `SHOW TABLE STATUS` for engines chosen years ago.
- **Leave `STRICT_TRANS_TABLES` on, and read warnings in the code that writes.** Strict mode is what turns a silent truncation into an error; the alternative is data that no longer matches what the application believes it stored.
- **Use `utf8mb4` for anything that stores user text, and `CHAR_LENGTH` when the limit is about characters.** A `utf8` column cannot hold a four-byte character at all — measured, the server refuses the write and names the bytes — while `LENGTH` measures bytes, which is what a `VARCHAR` limit is enforced in anyway. The two need to be considered together.
- **Set the collation explicitly on columns where equality decides access**, or compare a hash. Do not let the server's default decide whether `ADMIN` is an administrator.
- **Parameterise, and stop relying on escaping.** The measured backslash difference is exactly the class of bug that parameterisation removes: a bound value has no escape semantics.
- **Grant to the right host, and remember it is a pair.** Audit `mysql.user` and the grants per host, because a permission on one of the application's accounts is not a permission on all of them.
- **And when reading SQL you did not write, treat a comment as potentially live.** `/*!…*/` runs, `#` is a comment here and an error elsewhere, and a version number in a comment is a condition rather than decoration.

<!-- lang:zh -->
### 大多数人最先遇到的数据库

MySQL 是 Web 的默认关系数据库：几乎每一家共享主机、大多数 PHP 应用、以及其他很大一部分东西。MariaDB 是它的分支，协议兼容，很多发行版拿它当替代品。

**正因为普及，它的默认值比它的特性更要紧。** 这一篇里几乎每一个意外，都是某人在多年前选定、而现在已经改不动的默认值：一个转义字符、一个区分大小写与否的排序规则、一个在新版本开着而在旧版本关着的严格模式、以及一个曾经是默认、却不支持事务的存储引擎。

**而要弄清默认值的理由，和在任何引擎里一样：默认值在依赖它的代码里是看不见的。** 一个在某个版本上跑通、在另一个版本上表现不同的迁移，没有改过一行。

下面是在一个一次性的 **MariaDB 11.8.6** 实例上实测的：临时 socket、临时数据目录，跑完删掉。与 MySQL 本体共有的行为对两者都成立；两个分支不同的地方会指出。

### 第一部分：它的形状

四个词，把它们弄清能避免大部分困惑：

| 词 | 意思 |
|---|---|
| **服务端** | `mariadbd` / `mysqld` —— 拥有数据的那个进程 |
| **客户端** | `mariadb` / `mysql` —— 你敲命令的那个程序 |
| **数据库**（schema） | 表的名字空间；**这两个词在这里是同义的** |
| **账号** | **`user` @ `host`** —— 不只是一个名字 |

**账号是一对，这是它与其他引擎的第一处不同。** 实测，全新安装之后的账号是：

```
kali@localhost   mariadb.sys@localhost   root@127.0.0.1   root@::1   root@kali
```

**`root@localhost` 与 `root@127.0.0.1` 是两个账号。** 它们可以有不同的口令和不同的权限，而你是哪一个，由连接从哪来决定的。这就是为什么一条看起来正确的授权会失败：它授给了错误的主机，而一个走 TCP 而不是 socket 的连接，是另一个账号。

**而存储引擎是按表选的。** 一张表不只是"一堆行"；读写它的那段代码是一个插件：

| 引擎 | 事务 | 备注 |
|---|---|---|
| **InnoDB** | 有 | 默认值，也是该用的那个 |
| MyISAM | **没有** | 历史上的默认值；表级锁，没有回滚 |
| MEMORY | 没有 | 活在内存里 |
| Aria、Archive、CSV…… | 各不相同 | 特殊用途 |

### 第二部分：别的引擎都没有的转义字符

这是最让我意外的一组实测，因为它让同一个字面量在不同引擎上是不同的东西：

| 表达式 | MariaDB | SQLite | PostgreSQL |
|---|---|---|---|
| `length('\n')` | **1** | 2 | 2 |
| `length('\\')` | **1** | 2 | 2 |

**在 MySQL 里，反斜杠在字符串字面量中是转义符**，所以 `'\n'` 是一个换行 —— 一个字符 —— 而不是一个反斜杠加字母 n。**在 SQLite 与 PostgreSQL 里它是普通字符**，同一个字面量是两个字符。

**为什么这件事超出"字符串长度"的范围：** 它决定了转义函数该怎么做。一个为 MySQL 写的函数把 `'` 转义成 `\'`；把这个结果发给 PostgreSQL，它就是一个反斜杠加字符串结束符 —— 不同的字符串，以及一个语法风险。**标准自己的答案是把引号写两次**（`''`），而那在哪里都成立；MySQL 也可以被要求不再特殊对待反斜杠：

```sql
SET SESSION sql_mode = CONCAT(@@sql_mode, ',NO_BACKSLASH_ESCAPES');
```

**所以"反斜杠是不是转义符"是一个按部署而变的事实**，而它决定了 `\'` 是一个引号、还是一个被转义的反斜杠加一个结束符。这正是那一整族转义绕过背后的机制：不是 MySQL 的 bug，而是两个正确的实现，对同一个字节有不同理解。

### 第三部分：比较值，以及决定比较的那个排序规则

MySQL 自由地转换类型，而它转换的方向体现在答案里：

| 表达式 | 结果 | 读法 |
|---|---|---|
| `'1' = 1` | **1**（真） | 字符串被转成数字 |
| `'1' + 1` | **2** | 算术会转换 |
| `'abc' + 1` | **1** | **非数字文本静默地变成 0** |
| `'abc' = 0` | **1**（真） | 同一个转换，发生在比较里 |

**"非数字文本变成 0"是最有意思的一行。** 这个行为和 SQLite 一样、和 PostgreSQL 相反（后者拒绝）。所以一个 `WHERE code = 0` 会匹配到 `code` 是 `'abc'` 的那些行 —— 那要么正是你要的，要么是一个范畴错误，而没有任何东西告诉你哪一个。

**接下来是几乎没人会预期的那部分。** 实测，用服务器的默认排序规则：

| 表达式 | 结果 |
|---|---|
| `'a' = 'A'` | **1** —— 真 |
| `'a' = 'A' COLLATE utf8mb4_bin` | 0 —— 假 |
| `'a' = 'A' COLLATE utf8mb4_general_ci` | 1 —— 真 |
| **`'ß' = 'ss'`** | **1 —— 真** |

**两个不同的字符串比较相等，因为排序规则是一份"相等"的定义**，不是一条关于排序的说明。`utf8mb4_general_ci` 不区分大小写（`ci`）、也不区分重音（`ai`）；实测那台服务器的默认值是 `utf8mb4_uca1400_ai_ci`，而在 UCA 之下德语的那个 sharp s 等于 `ss`。

**这就是为什么一个用 `=` 比较用户名的登录检查，未必在做它看起来在做的事。** 在一个不区分大小写的排序规则里，`admin` 与 `ADMIN` 就比较而言是同一个字符串 —— 而 `Admin` 也是，在那个实测到的例子里，还有一对**渲染出来完全不同**的字母。控制手段是比较一个你定义了相等的东西：一个 `BINARY` 排序规则、一个哈希、或者由应用归一化过的值。

**而字符集是一个独立的决定，有它自己的坑。** 实测：

| 表达式 | `LENGTH` | `CHAR_LENGTH` |
|---|---|---|
| `'a'` | 1 | 1 |
| `'中'` | **3** | 1 |
| `'𝄞'`（U+1D11E） | **4** | 1 |

`LENGTH` 数的是**字节**，`CHAR_LENGTH` 数的是字符 —— 这就是它们不一致的原因，也是为什么一个按字符算的 `VARCHAR(10)` 能装十个宽字符、四十个字节。

**而 MySQL 的 `utf8` 不是 UTF-8。** 它每个字符的字节上限，从服务端自己的表里读出来：

| 字符集 | 每字符最大字节数 |
|---|---|
| `latin1` | **1** |
| **`utf8`**（实际报作 `utf8mb3`） | **3** |
| **`utf8mb4`** | **4** |

**所以 `utf8` 是一个最多三个字节的子集，根本存不下四字节字符**，`utf8mb4` 才是真的那个。实测，把上表里那个四字节字符插进一个 `utf8mb3` 列：

```
ERROR 1366 (22007): Incorrect string value: '\xF0\x9D\x84\x9E' for column `probe`.`cs`.`a` at row 1
```

**报错把它拒绝的那四个字节念了出来**，这也是一个有用的提醒：列的限额是按字节执行的 —— 一个 `VARCHAR(10)` 装十个那样的字符、四十个字节，而字符数是另一个问题。一个声明为 `utf8`、却指望它装基本平面之外文本的列 —— 一个 emoji、一个扩展汉字 —— 在严格模式开着时会拒绝写入，关着时表现又不同。

### 第四部分：严格模式，以及没有它会怎样

`sql_mode` 是一串开关，其中有意义的是 `STRICT_TRANS_TABLES`。实测，发行版打包的默认值：

```
STRICT_TRANS_TABLES,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION,...
```

往一个三字符的列里插一个五字符的值，**在同一个会话里**开关它：

| 模式 | 结果 |
|---|---|
| 默认（严格） | **拒绝** —— `ERROR 1406 (22001): Data too long for column 's'` |
| `SET SESSION sql_mode=''` 之后 | **接受**，并存成 **`ghi`**，长度 3 |

**同一句语句，两种结果：一次拒绝，或者一次静默的截断。** 非严格模式下值被裁到合适大小、并记一条警告，而不读警告的应用就在自以为成功的情况下存下了残缺的数据。这就是那个设置的整体形状：

> **严格模式决定的是：数据库要不要告诉你，它改了你的数据。**

**而它之所以是一个按部署而变的事实、而不是一个语言特性**，原因在历史：严格模式在当前版本是默认、在旧版本不是 —— 所以同一个应用、同一份 schema，落在不同的服务器上行为不同。

### 第五部分：事务，以及那个静默地不算事务的引擎

两组实测，都关于回滚的原子性。

**存储引擎对"回滚是否存在"的意见不一致。** 各建一张表、插一行、**在同一个会话里**回滚：

| 引擎 | `ROLLBACK` 之后 |
|---|---|
| **InnoDB** | **0 行** —— 插入被撤销了 |
| **MyISAM** | **1 行** —— 那行还在 |

**语句完全一样，结果不一样。** MyISAM 不实现事务；对它执行 `ROLLBACK` 会被接受、然后什么也不做 —— 这比报错更糟，因为应用相信自己撤销了什么。而账号在这里也要紧：同一个会话对一个 MyISAM 表的 `START TRANSACTION` 是个空操作，所以一个混用两种引擎的服务，在有些代码路径上有原子性、在另一些上没有。

**而 DDL 会替你提交事务。** 实测，在一个会话里：

```
START TRANSACTION;
INSERT INTO counter VALUES (1);
  COUNT(*) -> 1
CREATE TABLE ddl_here (id INT);     -- 这句 DDL
ROLLBACK;
  COUNT(*) -> 1                     -- 还在
```

**`ROLLBACK` 没有撤销那次插入**，因为 `CREATE TABLE` 在执行之前就把开着的事务提交了。所以这个模式是坏的：

```
BEGIN;  -- 迁移：建一张表、拷数据、改一个列
        -- 中途失败就 rollback
```

**在 MySQL 上，一个中途失败的迁移回滚不了**，因为沿途每一条 `ALTER` 和 `CREATE` 都把它之前的一切提交了。在 PostgreSQL 上同样的序列是一个事务、而且真的会回滚 —— 那是前面实测过的。**代码看起来一样，而"怎么恢复"这件事不一样。**

### 第六部分：自己动手做一次

一个一次性实例需要三条命令，不改任何配置：

```bash
mariadb-install-db --datadir=/tmp/mdb --user=root \
    --auth-root-authentication-method=normal
mariadbd --datadir=/tmp/mdb --user=root --socket=/tmp/mdb.sock \
    --port=3308 --bind-address=127.0.0.1 --log-error=/tmp/mdb.err &
mariadb --socket=/tmp/mdb.sock -u root -e "SELECT VERSION()"
```

**以 root 运行时 `--user=root` 是必需的**：否则服务端拒绝启动，而那个拒绝出现在错误日志里、不在标准输出上。

三个值得一开始就养成的习惯：

```sql
SELECT @@sql_mode;                          -- 哪些行为是打开的
SHOW VARIABLES LIKE 'collation%';            -- 这里的"相等"是什么意思
SELECT user, host FROM mysql.user;           -- 谁能连，从哪里连
EXPLAIN SELECT ... ;                         -- 它打算怎么找这些行
```

**而且要看错误日志，不只看客户端的输出。** 准备这一篇时遇到的每一次失败，客户端报的都是一个笼统的错误；具体原因 —— 包括那次被误判为失败的**成功启动** —— 只在 `--log-error` 里。

### 第七部分：每个人都会踩的坑

| 坑 | 会发生什么 |
|---|---|
| `SELECT 'a' \|\| 'b'` | **是逻辑或，不是拼接** —— 返回 `0`；要拼接用 `CONCAT()` |
| `utf8` 列装四字节字符 | **被拒** —— `ERROR 1366 Incorrect string value`；`utf8` 是三字节子集，要用 `utf8mb4` |
| `LENGTH` 与 `CHAR_LENGTH` | 字节与字符 —— 任何非 ASCII 上两者都不同 |
| `WHERE s = 'a'` 匹配到 `'A'` | 排序规则默认不区分大小写 |
| 对 MyISAM 表 `ROLLBACK` | 被接受，什么也不做 |
| `BEGIN; ALTER TABLE …; ROLLBACK;` | 那句 DDL 已经提交了 |
| 非严格模式下的超长值 | 静默截断，配一条没人看的警告 |
| 拿值和 `NULL` 比较 | 永远不为真，各引擎一样 |
| 查询里的 `#` | 在 MySQL 里是注释，在多数别的引擎里是语法错 |
| 授权授给了错误的主机 | `'app'@'localhost'` 不授权 `'app'@'10.0.0.5'` |

### 第八部分：从这些机制推出的安全观念

**一个忽略大小写的排序规则，是一次没人做过的认证决定。** 实测到的 `'a' = 'A'` 与 `'ß' = 'ss'` 就是经典旁路背后的机制：在一个 `ci` 列上做的唯一性检查或登录比较，会把两个不同的字符串当成一个。**凡是"相等"被用来授予什么的地方，排序规则就是那项控制的一部分** —— 而修法是比较一个相等被定义过的值，不是到别处再加一道检查。

**转义必须按引擎写，因为转义字符按引擎而变。** 第二部分实测了一个在这里特殊、在别处普通的反斜杠。为一个引擎写的转义函数在另一个引擎上产出不同的字符串，而那个差别在它变成一次绕过之前是看不见的。**参数化让这个问题消失**，因为被绑定的值从不成为语句文本的一部分。

**注释可以是代码。** 实测：

| 语句 | 结果 |
|---|---|
| `SELECT /*!50600 2 */` | `2` —— **里面的内容执行了** |
| `SELECT /*!99999 1 */` | 语法错 —— 内容没执行，只剩下 `SELECT` |
| `SELECT /*! 50600 2 */` | 语法错 —— **`/*!` 后面一个空格，它就成了普通注释** |
| `SELECT 'a' /*!50600 , 'b' */` | `a`、`b` —— **多出来一列** |

**版本号必须紧跟在 `/*!` 之后**，而当服务器版本不低于那个号时，里面的内容会被执行。这意味着：

**一个"去掉注释"的过滤器可能把版本判断删掉、把载荷留下**，也可能把载荷删掉、留下一个残缺语句 —— 两种都错，而且都和同一个过滤器在别的引擎上产出不同。

**而攻击者可以用它让一条语句依赖版本**：同一段字节在一个版本上是两列的查询、在另一个版本上是一列的，而那会改变一个 `UNION` 需要对齐的东西。

**权限模型是一对，而那才是要查的东西。** 授权是授给 `user@host` 的，所以"应用那个账号"实际上是一组账号，而授给 socket 账号的权限不会跟着应用走到 TCP 连接上。**再叠上存储引擎，波及面就有两个没人看的维度**：哪个账号，以及哪些表在一个无法撤销的引擎上。

**而迁移问题就是一个安全问题。** 在 MySQL 上一次失败的迁移可能把 schema 留在半路，因为 DDL 一路提交；在 PostgreSQL 上同样的失败会回滚。**一份假定"我们回滚了"的事故预案，只对其中的一个成立。**

### 检测与缓解

- **读错误日志，并留着它。** 实测，所有要紧的诊断都在 `--log-error` 里、而不在客户端输出里 —— 包括一次看起来像失败的**成功启动**。以 root 启动被拒绝也记在那里。
- **对运行期的 `sql_mode` 变更告警。** 会话级或全局级改动严格模式，会改变应用的写入是被拒绝还是被静默截断；那是一次带数据正确性后果的配置变更。
- **对 schema 里出现的非 InnoDB 表告警。** 一张没有事务的引擎上的表，会静默地破坏碰它的每一处回滚，而表定义正是这件事可见的地方。
- **盯失败的迁移，并且永远不要假定回滚发生了。** 在这个引擎上，一句 DDL 会提交开着的事务，所以"我们回滚了"不是脚本的性质 —— 它是引擎的性质，而且只在其中一些上成立。
- **缓解上，没有特别理由就用 InnoDB**，并用 `SHOW TABLE STATUS` 查一下多年前选定的引擎。
- **让 `STRICT_TRANS_TABLES` 开着，并在写数据的代码里读警告。** 严格模式是把"静默截断"变成报错的那件事；另一条路的代价，是数据不再匹配应用自以为存下的东西。
- **任何存用户文本的地方都用 `utf8mb4`，而当限制是关于字符时用 `CHAR_LENGTH`。** 一个 `utf8` 列根本装不下四字节字符，而 `LENGTH` 数的是字节 —— 对 `VARCHAR` 来说列的限额本来就是按字节执行的，所以这两件事要一起考虑。
- **在"相等决定访问权"的列上显式指定排序规则**，或者比较一个哈希。不要让服务器的默认值来决定 `ADMIN` 是不是管理员。
- **参数化，不要依赖转义。** 实测那个反斜杠差异，正是参数化能消掉的那一类 bug：一个被绑定的值没有转义语义。
- **把权限授给正确的主机，并记住它是一对。** 审计 `mysql.user` 以及按主机拆开的授权，因为应用其中一个账号上的权限，不等于它所有账号上的权限。
- **而且在读别人写的 SQL 时，把注释当成可能活的。** `/*!…*/` 会执行，`#` 在这里是注释、在别处是错误，而注释里的版本号是一个条件，不是装饰。
