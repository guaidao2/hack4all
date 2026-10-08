---
id: sqli-fundamentals
title_en: "SQL Injection, Part 1 — Why It Happens"
title_zh: "SQL 注入（一）：为什么会发生"
summary_en: SQL injection is not a filtering failure, it is an architectural one — the query and the data travel down the same channel, so data can be read as structure. This entry covers the root cause, why escaping works at the wrong layer while parameterising works at the right one, where injection points actually exist, and how to tell which kind you are looking at without a tool.
summary_zh: SQL 注入不是过滤没做好，而是架构问题 —— 查询和数据走在同一条通道上，于是数据可以被当成结构来读。这一篇讲根因、讲为什么转义工作在错误的层次而参数化工作在正确的层次、讲注入点究竟在哪些位置，以及不用工具怎么判断自己面对的是哪一种。
tags: [web, sql-injection, database, fundamentals, parameterization]
tools: [sqlmap, Burp Suite, curl, mysql, sqlite3]
attck: [T1190]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The root cause, stated properly

Most explanations of SQL injection describe a symptom: the application does not escape quotes. That is true and it is misleading, because it suggests the fix is to escape harder.

Here is the actual structure. A query is built by **string concatenation**:

```python
query = "SELECT * FROM users WHERE name = '" + user_input + "'"
```

Two things are being merged that are not the same kind of thing:

- **Code** — `SELECT * FROM users WHERE name = '...'`, whose meaning is determined by SQL's grammar.
- **Data** — whatever the user typed, whose meaning should be "a value" and nothing else.

By concatenating them into one string, the application hands the database **one object with no provenance**. The database has no way to know which characters came from the developer and which came from the attacker. It does what it is designed to do: it parses the whole string as SQL. If the data contains a quote, a comment marker or a `UNION`, the parser cannot tell that it was supposed to be a value — so it becomes structure.

**Neither layer is broken.** The application faithfully assembled a string. The database faithfully parsed a string. The bug is that **they agreed on a representation in which the boundary between code and data does not exist.** That is why this is an architectural problem, and why the entry that follows is about channel separation rather than about character filtering.

The `'` in a payload is not the vulnerability. It is the tool used to demonstrate that the boundary was already missing.

### Why escaping is the wrong layer

The traditional defence is escaping: replace `'` with `\'` or `''`, strip comments, remove `UNION`. It is worth understanding precisely why this is fragile, because the reason is structural rather than a matter of completeness.

**SQL is processed in stages.** A database engine does not interpret a query character by character in one pass. It roughly does:

1. **Tokenise** — split the string into tokens: identifiers, keywords, literals, operators.
2. **Parse** — build a syntax tree. This is where "this is a string literal" versus "this is a keyword" is decided.
3. **Plan and execute** — run the tree.

Escaping operates at **stage 1, from outside**, by trying to ensure that a quote ends up inside a literal token rather than terminating it. Every escape-based defence therefore has three properties that make it fail eventually:

| Property | Consequence |
|---|---|
| It is **lexical** — it reasons about characters | Any encoding or charset difference that changes what a character is breaks it, which is exactly the wide-byte and overlong-encoding material from the encoding series |
| It is a **single point of failure** — one missed path and the query is injectable | An application has hundreds of queries; the one that forgot to escape is the vulnerability |
| It must be **complete** to work | Every dangerous construct in every dialect has to be known and handled, and SQL has many ways to express structure: comments, `UNION`, stacked statements, subqueries, `CASE` |

The cleanest demonstration is the wide-byte case, and it connects straight to the encoding entries: an escaping function that inserts `0x5C` byte-wise, and a database that reads `DF 5C` as one GBK character, **disagree about where the quote is**. The escaping layer was reasoning about bytes; the database was reasoning about characters. Same class of bug as everything in the encoding series — two layers with different models.

### Why parameterising is the right layer

A parameterised query does not filter anything. It **changes the protocol**:

```
without parameters:   one string, parsed as SQL
                      "SELECT * FROM users WHERE name = 'alice' OR 1=1--'"

with parameters:      the statement is sent first, with placeholders
                      "SELECT * FROM users WHERE name = ?"
                      ...and the data is sent separately
                      ["alice' OR 1=1--"]
```

The database **parses and plans the statement before it has ever seen the data.** By the time the value arrives, the syntax tree is already fixed: the parser has decided that position is a value slot. A value cannot change the tree, no matter what it contains — a quote in the data is a quote in a string, because there is no longer a stage at which it could be read as anything else.

This is why parameterisation is immune to the entire encoding family. **It does not matter what bytes the value decodes to**, because the value never participates in parsing. That is the difference between a defence that filters a representation and a defence that removes the ambiguity — the same argument as "separate the channel" in the encoding entry, and the same argument as modular arithmetic in the classical cipher entries: solve the class, not the instance.

```python
# the shape of it, in three languages
cursor.execute("SELECT * FROM users WHERE name = %s", (user_input,))          # Python DB-API
# PreparedStatement ps = conn.prepareStatement("SELECT * FROM users WHERE name = ?");
# ps.setString(1, userInput);                                                 // Java
# db.Query("SELECT * FROM users WHERE name = $1", userInput)                   // Go
```

**One honest caveat.** Parameterisation only gives you real separation if the driver implements it as server-side preparation. Some drivers **emulate** it on the client by escaping for you, which puts you back at the fragile layer — the difference is invisible in the application code and depends on the driver, its version and its configuration. When it matters, check whether the query is actually prepared on the server. It is a good question to ask in a code review, and it is why "we use parameters" is a claim worth verifying rather than accepting.

### Where injection points actually are

The position of the input in the query determines what is possible, and — importantly — **whether parameterisation is even an option.**

| Location | Example | Parameterisable? |
|---|---|---|
| `WHERE` value | `WHERE id = ?` | Yes, the normal case |
| `WHERE` with `LIKE` | `WHERE name LIKE ?` | Yes, but you must handle the wildcards yourself |
| `INSERT` / `UPDATE` values | `INSERT INTO t VALUES (?, ?)` | Yes |
| `ORDER BY` column | `ORDER BY <column> <direction>` | **No** — see below |
| `LIMIT` / `OFFSET` | `LIMIT ?` | Sometimes; dialect-dependent |
| Table or column name | `SELECT * FROM <table>` | **No** |
| `GROUP BY`, `HAVING` | Same problem as `ORDER BY` | No |
| Stacked statements | `...; DROP TABLE ...` | Not a parameter problem at all |

**Why `ORDER BY` and identifiers cannot be parameterised.** A parameter is a *value*. `ORDER BY ?` does not sort by the column named in the parameter — it sorts by the literal value, which is meaningless. Identifiers are part of the **structure** of the query, and structure cannot be data. So for these positions there is only one correct approach: **an allowlist of permitted names**, checked in application code, with anything not on the list rejected. Not escaped, not quoted — rejected.

```python
ALLOWED_SORT = {'name', 'created_at', 'score'}
ALLOWED_DIR = {'asc', 'desc'}

def safe_order_by(column: str, direction: str) -> str:
    if column not in ALLOWED_SORT or direction.lower() not in ALLOWED_DIR:
        raise ValueError('invalid sort')
    return f'ORDER BY {column} {direction.upper()}'      # safe: values came from a fixed set
```

That is not a workaround; it is the only sound construction for that position. It also demonstrates the general principle: **where data cannot be separated from structure, the only defence is to constrain the data to a set you wrote.**

**Stacked queries** are a separate matter. Whether `;` allows a second statement depends on the API and dialect: `mysqli_query` in PHP executes one statement and refuses more, `mysqli_multi_query` does not, MSSQL and PostgreSQL generally allow it through most interfaces, Oracle's default interfaces do not. So "can I stack" is a property of the client library as much as of the database — a detail worth establishing during testing rather than assuming.

### The three families of injection, by how data comes back

This is the taxonomy to know, because it determines the entire technique set.

| Family | How you learn the answer | Works when |
|---|---|---|
| **In-band** | The result appears in the response — either as extra rows (`UNION`) or inside an error message | The application echoes query output or database errors |
| **Inferential (blind)** | The response does not contain the data, but **differs** based on its truth: a different page, or a different delay | Nothing is echoed, but behaviour is observable |
| **Out-of-band** | The database is made to send the data out by another channel — a DNS lookup or an HTTP request | Outbound network access from the database exists, and in-band is blocked |

Within each:

- **In-band** splits into **`UNION`-based** (append a second query whose results are merged into the page) and **error-based** (extract data by making the database put it inside an error message).
- **Blind** splits into **boolean-based** (ask a yes/no question and observe which page comes back) and **time-based** (ask a yes/no question and observe whether the response is delayed).

The progression from in-band to blind to out-of-band is a progression of **less and less feedback**, and modern targets increasingly sit at the blind end: error messages are suppressed, output is templated, and WAFs block obvious payloads. That is why the later entries in this series spend most of their time on inference rather than on `UNION`.

### The input shapes, and how to recognise them

Independently of where the data comes back, the *shape* of the parameter decides how you have to close the injection:

| Shape | The query looks like | How you test |
|---|---|---|
| **Numeric** | `WHERE id = 1` | `1 AND 1=1` (true) versus `1 AND 1=2` (false) — no quote needed |
| **String** | `WHERE name = 'alice'` | `alice'` — a quote to break out |
| **String, quoted with `"`** | `WHERE name = "alice"` | `alice"` |
| **`LIKE` search** | `WHERE name LIKE '%alice%'` | `alice%'` — you must close the `%` and the quote |
| **Numeric in a string context** | `WHERE id = '1'` | `1'` first, then treat as string |

### Working it out by hand

This is the procedure worth having in your fingers, because it is what you do when a tool is unavailable, blocked or unhelpful.

**Step 1 — find a parameter that reaches the database.** Query-string values, form fields, JSON bodies, headers that end up in a query (`X-Forwarded-For` in a logging query), cookies. Change the value and see whether the response changes at all; a parameter that does not affect the output may not reach a query.

**Step 2 — provoke an error or a behaviour change.** Add a single quote:

```
?id=1'      ->  SQL syntax error, or a 500, or a subtle difference
?id=1''     ->  works again (the extra quote rebalanced the string)
```

The `'` then `''` pair is the classic confirmation: if one quote breaks and two fix it, the input is being placed inside a string literal.

**Step 3 — decide the shape.** For numeric parameters, compare `1 AND 1=1` with `1 AND 1=2`. If the first returns normally and the second returns nothing or an error, you have a numeric injection with boolean feedback — which is already enough for blind extraction.

**Step 4 — count the columns.** For `UNION` you need the same column count as the original query:

```
?id=1 ORDER BY 1-- -      increment until it errors: the last successful number is the column count
?id=1 UNION SELECT 1,2,3-- -   or add columns to the UNION until it stops erroring
```

**Step 5 — find which columns are displayed.** Replace the numbers with distinguishable markers and see which appear in the page:

```
?id=-1 UNION SELECT 111,222,333-- -
```

The `-1` matters: making the original query return nothing ensures what you see is your own row.

**Step 6 — extract.** From here the techniques diverge: use the visible column for `UNION` extraction, the error message for error-based, or the true/false signal for blind. Those are the next entries.

**Two habits make this faster.** First, comment syntax differs by dialect — `-- ` (with a trailing space), `#`, or `/* */` — so know all three and try them. Second, **the injection does not have to produce visible output to be exploitable**; the moment you can distinguish true from false, you can extract a database one bit at a time, which is slow but complete.

### Why this is still everywhere

SQL injection has been in the OWASP Top 10 since the beginning and it is still there, for reasons that are worth naming rather than dismissing:

- **Legacy code.** Decades of applications, still running, still maintained.
- **Frameworks do not save you by default.** Most ORMs provide a safe API *and* a raw-query escape hatch, and the raw path is exactly where this appears — often in the one place performance mattered.
- **Structure-required positions.** `ORDER BY` and identifiers cannot be parameterised, so they need the allowlist approach and often do not get it.
- **Second-order injection.** Data that was safely stored is later concatenated into a query, usually by a different developer months later.
- **Defence in depth is often the only defence.** A WAF may be the only thing between a broken query and an incident, and the encoding entry shows how easily signatures are bypassed.

### Detection and mitigation

- **Parameterise every value position, and verify it is really prepared.** The check in a review is: does the driver send `PREPARE` and `EXECUTE` (or the protocol equivalent), or does it build the string client-side? "We use parameters" is a claim, not a control, until you have looked.
- **Allowlist the positions that cannot be parameterised.** Column names, sort directions, table names, and anything else that is structure rather than value. Reject anything not on the list — do not escape it, do not quote it.
- **Give the application account the least privilege it needs.** A web application rarely needs `DROP`, `FILE`, `xp_cmdshell`, or read access to other schemas. Least privilege does not prevent injection; it bounds the damage from a successful one, and it removes whole technique families (file read/write, command execution, cross-database extraction) from reach.
- **Do not rely on suppressing errors, but do suppress them.** Turning off detailed error messages removes the error-based family and reduces information leakage, which is a real improvement — but it is **obscurity, not a fix**, because blind and out-of-band techniques need no error output at all. Fix the query first.
- **Log the queries and the failures.** A spike in SQL syntax errors, queries touching `information_schema` or `sysobjects`, unexpected `UNION` in a logged statement, and statements from the application account that the application never issues are all high-value signals. The most reliable one is a **full query log with parameters**, correlated with the request that caused it.
- **Look for the shape of probing, not just the payload.** The same parameter with a quote, then two quotes, then `AND 1=1`, then `AND 1=2`, then delays — that sequence from one source is unmistakable regardless of how the individual payloads are encoded, and it catches encoding-based bypasses that a signature would miss. This is the detection counterpart of the encoding entry's advice to compare the raw and decoded views of a request.
- **Watch for latency anomalies.** Time-based blind injection is quiet in the response body but loud in the timing: a sustained pattern of queries taking a suspiciously round number of seconds, correlated with one client, is the signal. Baseline the normal query latency and alert on the deviation rather than on an absolute threshold.
- **Monitor outbound from the database server, and stop it at the network.** A database host has no reason to make DNS queries or HTTP requests to the internet, so allowing none of it removes the entire out-of-band family — the same reasoning as outbound filtering for command and control.

<!-- lang:zh -->
### 根因，正经说一遍

多数关于 SQL 注入的解释描述的是症状：应用没有转义引号。这个说法是对的，也是误导的 —— 因为它暗示修法是"转义得更狠一点"。

真实的结构是这样。查询是用**字符串拼接**搭出来的：

```python
query = "SELECT * FROM users WHERE name = '" + user_input + "'"
```

这里把两样**不是同一类东西**的东西合并了：

- **代码** —— `SELECT * FROM users WHERE name = '...'`，它的含义由 SQL 的语法决定。
- **数据** —— 用户敲进去的任何东西，它的含义本该是"一个值"，仅此而已。

把它们拼成一个字符串，应用交给数据库的就是**一个没有出处的对象**。数据库没有任何办法知道哪些字符来自开发者、哪些来自攻击者。它做它被设计来做的事：把整个字符串当成 SQL 解析。如果数据里含引号、注释符或者 `UNION`，解析器没法知道"它本该是个值" —— 于是它变成了结构。

**两层都没有坏。** 应用忠实地拼出了一个字符串；数据库忠实地解析了一个字符串。**bug 在于它们认同了一种表示，而在那种表示里，代码与数据的边界根本不存在。** 这就是为什么这是一个架构问题，也是为什么随后那一篇讲的是通道分离、而不是字符过滤。

载荷里那个 `'` 不是漏洞，它只是用来演示"边界本来就没了"的工具。

### 为什么转义工作在错误的层次

传统的防御是转义：把 `'` 换成 `\'` 或 `''`、剥掉注释、删掉 `UNION`。值得精确地理解它为什么脆弱，因为这个理由是结构性的，而不是"考虑得不够全"。

**SQL 是分阶段处理的。** 数据库引擎不是一遍扫描就逐字符解释完一条查询的。它大致会：

1. **词法分析** —— 把字符串切成记号：标识符、关键字、字面量、运算符。
2. **语法分析** —— 构建语法树。**"这是一个字符串字面量"还是"这是一个关键字"，就是在这一步被决定的。**
3. **计划与执行** —— 跑那棵树。

转义是在**第 1 步、从外面**做事的，试图保证引号最终落在一个字面量记号内部、而不是终结它。所以每一种基于转义的防御都有三个性质，决定了它迟早失效：

| 性质 | 后果 |
|---|---|
| 它是**词法的** —— 它推理的是字符 | 任何改变"一个字符是什么"的编码或字符集差异都会打破它，而这恰恰就是编码系列里宽字节与过长编码那部分材料 |
| 它是**单点故障** —— 漏掉一处，那条查询就可注入 | 一个应用有几百条查询；忘了转义的那一条就是漏洞 |
| 它必须**完备**才有效 | 得知道并处理每一种方言里所有危险构造，而 SQL 表达结构的方式有很多：注释、`UNION`、堆叠语句、子查询、`CASE` |

最干净的演示是宽字节那个，而它直接接上编码那几篇：一个按字节插入 `0x5C` 的转义函数，和一个把 `DF 5C` 读成一个 GBK 字符的数据库，**对"引号在哪里"的意见不一致**。转义层在推理字节，数据库在推理字符。和编码系列里的一切是同一类 bug —— 两层持有不同的模型。

### 为什么参数化在正确的层次

参数化查询**不过滤任何东西**，它**改变协议**：

```
不用参数：  一个字符串，被当成 SQL 解析
            "SELECT * FROM users WHERE name = 'alice' OR 1=1--'"

用参数：    先把语句发过去，带占位符
            "SELECT * FROM users WHERE name = ?"
            ……数据单独发送
            ["alice' OR 1=1--"]
```

数据库**在见到数据之前就已经解析并计划好了语句**。等那个值到达时，语法树已经固定：解析器早就判定那个位置是一个值槽。值无法改变树，无论它包含什么 —— 数据里的引号就是字符串里的引号，因为**已经不存在一个阶段能让它被读成别的**。

这就是为什么参数化对整个编码家族免疫。**那个值解码成什么字节根本不重要**，因为这个值从不参与解析。这就是"过滤一种表示"和"消除歧义"之间的区别 —— 和编码那篇里"分离通道"是同一个论证，也和古典密码那几篇里的模运算推理一样：**解决这一类，而不是这一个实例。**

```python
# 三种语言里的形状
cursor.execute("SELECT * FROM users WHERE name = %s", (user_input,))          # Python DB-API
# PreparedStatement ps = conn.prepareStatement("SELECT * FROM users WHERE name = ?");
# ps.setString(1, userInput);                                                 // Java
# db.Query("SELECT * FROM users WHERE name = $1", userInput)                   // Go
```

**一句诚实的提醒。** 参数化只有在驱动把它实现为**服务端预编译**时才真正给你分离。有些驱动是在客户端**模拟**它、替你转义 —— 那就又把你放回了那个脆弱的层次。这个区别在应用代码里看不出来，取决于驱动、版本和配置。要紧的时候，去确认这条查询在服务端是否真的被 prepare 了。这是代码评审里值得问的问题，也是为什么"我们用了参数"是一句**需要验证**而不是接受的话。

### 注入点实际在哪些位置

输入在查询里的位置，决定了什么可行 —— 而且更重要的是，决定了**参数化到底是不是一个选项**。

| 位置 | 例子 | 能参数化吗 |
|---|---|---|
| `WHERE` 的值 | `WHERE id = ?` | 能，正常情况 |
| `WHERE` 配 `LIKE` | `WHERE name LIKE ?` | 能，但通配符得自己处理 |
| `INSERT` / `UPDATE` 的值 | `INSERT INTO t VALUES (?, ?)` | 能 |
| `ORDER BY` 的列 | `ORDER BY <列> <方向>` | **不能** —— 见下 |
| `LIMIT` / `OFFSET` | `LIMIT ?` | 有时可以；看方言 |
| 表名或列名 | `SELECT * FROM <表>` | **不能** |
| `GROUP BY`、`HAVING` | 和 `ORDER BY` 同样的问题 | 不能 |
| 堆叠语句 | `...; DROP TABLE ...` | 这根本不是参数能解决的问题 |

**为什么 `ORDER BY` 和标识符不能参数化。** 参数是**值**。`ORDER BY ?` 不会按参数里那个列名排序 —— 它会按那个字面值排序，而那毫无意义。标识符是查询的**结构**的一部分，而结构不能是数据。所以这些位置只有一种正确做法：**一份允许名称的白名单**，在应用代码里检查，不在名单上的一律拒绝。不转义、不加引号 —— 直接拒绝。

```python
ALLOWED_SORT = {'name', 'created_at', 'score'}
ALLOWED_DIR = {'asc', 'desc'}

def safe_order_by(column: str, direction: str) -> str:
    if column not in ALLOWED_SORT or direction.lower() not in ALLOWED_DIR:
        raise ValueError('invalid sort')
    return f'ORDER BY {column} {direction.upper()}'      # 安全：值来自一个固定集合
```

那不是权宜之计，那是**该位置唯一可靠的构造**。它也演示了那条通用原则：**当数据无法与结构分离时，唯一的防御就是把数据限制在你写下的那个集合里。**

**堆叠语句**是另一回事。`;` 能不能接第二条语句，取决于 API 和方言：PHP 的 `mysqli_query` 只执行一条、拒绝更多，`mysqli_multi_query` 则可以；MSSQL 和 PostgreSQL 在多数接口下一般允许；Oracle 的默认接口不允许。所以"我能不能堆叠"既是数据库的性质，也是客户端库的性质 —— 这是测试时应当**确认**而不是假定的细节。

### 三大类注入，按数据怎么回来分

这是该掌握的归类方式，因为它决定了整个技术集合。

| 类别 | 你怎么得知答案 | 何时可用 |
|---|---|---|
| **带内** | 结果出现在响应里 —— 要么是多出来的行（`UNION`），要么是在报错信息里 | 应用会回显查询结果或数据库报错 |
| **推断（盲注）** | 响应里没有数据，但**会因真假而不同**：页面不同，或者延迟不同 | 什么都不回显，但行为可观察 |
| **带外** | 让数据库通过另一条通道把数据发出去 —— 一次 DNS 查询或一次 HTTP 请求 | 数据库有出网能力，而带内被挡死了 |

每一类内部：

- **带内**分为 **`UNION` 型**（追加一条查询，结果被合并进页面）和**报错型**（通过让数据库把数据放进错误信息里来提取）。
- **盲注**分为**布尔型**（问一个是/否问题，看回来的是哪个页面）和**时间型**（问一个是/否问题，看响应有没有被延迟）。

从带内到盲注到带外，是一条**反馈越来越少**的路径，而现代目标越来越多地坐在盲注那一端：报错被屏蔽、输出被模板化、WAF 挡掉明显的载荷。这就是为什么本系列后面的篇目把大部分篇幅花在推断上，而不是花在 `UNION` 上。

### 输入的形态，以及怎么认出来

与"数据从哪回来"无关，参数的**形态**决定了你得怎么闭合注入：

| 形态 | 查询长这样 | 怎么测 |
|---|---|---|
| **数字型** | `WHERE id = 1` | `1 AND 1=1`（真）对 `1 AND 1=2`（假）—— 不需要引号 |
| **字符型** | `WHERE name = 'alice'` | `alice'` —— 一个引号破出去 |
| **双引号包裹** | `WHERE name = "alice"` | `alice"` |
| **`LIKE` 搜索型** | `WHERE name LIKE '%alice%'` | `alice%'` —— 你得把 `%` 和引号都闭合掉 |
| **字符串语境里的数字** | `WHERE id = '1'` | 先试 `1'`，再按字符串处理 |

### 手工把它做出来

这套流程值得练到手上，因为工具不可用、被挡或者帮不上忙的时候，靠的就是它。

**第一步 —— 找一个真能到达数据库的参数。** 查询串的值、表单字段、JSON 请求体、会被拼进查询的头（比如日志查询里的 `X-Forwarded-For`）、cookie。改它的值，看响应有没有变化；对输出毫无影响的参数，可能根本没到查询里。

**第二步 —— 制造报错或行为变化。** 加一个单引号：

```
?id=1'      ->  SQL 语法错误，或 500，或细微的差别
?id=1''     ->  又正常了（多出来的引号把字符串配平了）
```

`'` 然后 `''` 这一对是经典确认法：**一个引号坏、两个引号好，说明输入被放进了字符串字面量里。**

**第三步 —— 判断形态。** 数字型参数上，比较 `1 AND 1=1` 和 `1 AND 1=2`。如果第一个正常返回、第二个什么都没返回或报错，你就有一个**带布尔反馈的数字型注入** —— 这已经足够做盲注提取了。

**第四步 —— 数出列数。** 用 `UNION` 需要和原查询列数相同：

```
?id=1 ORDER BY 1-- -     一直加，直到报错：最后一个成功的数字就是列数
?id=1 UNION SELECT 1,2,3-- -   或者往 UNION 里加列，直到不再报错
```

**第五步 —— 找出哪几列会被显示。** 把数字换成互相好认的标记，看哪些出现在页面里：

```
?id=-1 UNION SELECT 111,222,333-- -
```

那个 `-1` 是有意的：让原查询返回空，才能保证你看到的是你自己那一行。

**第六步 —— 提取。** 从这里开始技术就分岔了：用可见列做 `UNION` 提取、用报错信息做报错型、或者用真假信号做盲注。那些是后面几篇的内容。

**两个习惯能让你更快。** 第一，注释语法各方言不同 —— `-- `（注意尾随空格）、`#`、`/* */` —— 三种都要知道，都要试。第二，**注入不必产生可见输出才是可利用的**；一旦你能区分真和假，你就可以一次一比特地把整个数据库提出来，慢，但完整。

### 为什么它至今到处都是

SQL 注入从第一天起就在 OWASP Top 10 里，现在还在，原因值得点名而不是一句"老问题"带过：

- **遗留代码。** 几十年的应用还在跑、还在维护。
- **框架默认救不了你。** 多数 ORM 既提供安全的 API，也留了裸查询的口子，而这类问题恰恰出现在那个口子上 —— 常常是在当初为了性能的那一处。
- **结构性的位置。** `ORDER BY` 和标识符不能参数化，所以需要白名单做法，而它们往往得不到。
- **二阶注入。** 安全存下来的数据，日后被拼进了查询 —— 通常还是另一个开发者几个月后干的。
- **纵深防御常常是唯一的防御。** 一条坏查询和一起事故之间，可能只隔着一个 WAF —— 而编码那篇已经展示了签名有多容易被绕过。

### 检测与缓解

- **每一个值位置都参数化，并且确认它真的是预编译。** 评审时的检查是：驱动发的是 `PREPARE` 和 `EXECUTE`（或协议上的等价物），还是在客户端拼字符串？在你亲眼看过之前，"我们用了参数"是一个说法，不是一个控制。
- **不能参数化的位置，用白名单。** 列名、排序方向、表名，以及任何属于结构而非值的东西。不在名单上的一律拒绝 —— 不要转义它，不要给它加引号。
- **给应用账号它真正需要的最小权限。** Web 应用很少需要 `DROP`、`FILE`、`xp_cmdshell`，也很少需要读别的 schema。最小权限不能防止注入；它能**限制一次成功注入造成的破坏**，并且把整整几个技术家族（文件读写、命令执行、跨库提取）从可达范围里移走。
- **不要指望"屏蔽报错"，但要屏蔽报错。** 关掉详细报错能移除报错型这一整族、并减少信息泄漏，这是实打实的改进 —— 但它是**模糊化，不是修复**，因为盲注和带外技术根本不需要报错输出。**先把查询修好。**
- **把查询和失败都记下来。** SQL 语法错误突增、触碰 `information_schema` 或 `sysobjects` 的查询、日志语句里意外的 `UNION`、以及应用账号发出应用从不发出的语句，都是高价值信号。最可靠的那一个是**带参数的完整查询日志**，并与导致它的请求关联起来。
- **找"探测的形状"，而不只是找载荷。** 同一个参数先来一个引号、再来两个引号、再 `AND 1=1`、再 `AND 1=2`、然后延迟 —— 来自同一来源的这一串是骗不了人的，不管每个载荷怎么被编码，而它能抓到签名会漏掉的编码绕过。这就是编码那篇里"对比请求的原始视图与解码视图"在检测侧的对应物。
- **盯延迟异常。** 时间盲注在响应体里很安静，在时序里很吵：与某个客户端相关的一串持续、取整得可疑的秒级查询，就是那个信号。给正常查询延迟建基线，对**偏离**告警，而不是对一个绝对阈值告警。
- **监控数据库服务器的出站，并在网络上掐断它。** 一台数据库主机没有理由对互联网做 DNS 查询或 HTTP 请求，所以一条都不放行就移除了整个带外家族 —— 和针对命令控制的出站过滤是同一个思路。
