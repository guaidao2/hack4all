---
id: db-sqlite
title_en: SQLite
title_zh: SQLite
summary_en: Not a small database but a library — no server, no network, no users, no grants, one file. That shape explains almost everything about it, including why its security model is file permissions and why deleting a row is not the same as removing it from the file.
summary_zh: 它不是"小数据库"，而是一个库 —— 没有服务端、没有网络、没有用户、没有授权，只有一个文件。这个形态解释了它几乎所有的性质，包括为什么它的安全模型就是文件权限，以及为什么"删掉一行"不等于"文件里没有它了"。
tags: [beginner, database, sqlite, embedded, forensics, file-permissions]
tools: [sqlite3, python3, strings]
attck: [T1213]
platform: [web, mobile]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A library, not a server

Most of what people assume about databases comes from the client/server shape: a service listening somewhere, a set of accounts, a grant for each table. SQLite has none of that.

> **SQLite is a library that reads and writes a file.** There is no process to connect to, no socket, no port, no user, and no permission system inside it.

That is not a reduced version of a database. It is a different design that fits different problems, and it is present in more places than any server ever will be: inside phones, desktop applications, browsers, embedded devices, and — the case that matters most for this guide — **inside web applications**, where each deployment, tenant or test run gets its own file.

**And the shape decides the security model.** With no accounts, access control is the operating system's; with no network, the exposure is the file rather than a port; and with the data in a file the application owns, questions about deletion, backup and residue are questions about that file.

Measured below on **SQLite 3.53.4** as shipped on Kali, using a throwaway file that is removed at the end.

### Part 1: the shape of it

| | SQLite | A client/server engine |
|---|---|---|
| What it is | **a library linked into the program** | a service, usually on another host |
| How you reach it | **open a file** (or `:memory:`) | connect over a socket or TCP |
| Authentication | **none** | accounts and passwords |
| Authorisation | **none** | `GRANT`, roles, row policies |
| Concurrency | **file locking; one writer at a time** | MVCC, many writers |
| Users at once | one process, typically | thousands |
| Deployment | **nothing to install or run** | a server to run and keep running |

**Two consequences follow immediately, and both are security consequences.**

**There is no network attack surface.** The entire class of "the database was reachable from the internet" does not exist here — but the class of "the database file was reachable over HTTP" does, and it is the same finding with a different transport.

**There is no privilege model to configure.** "Least privilege" here means the permissions on a file and the identity of the process holding it, not a set of grants.

**And one design consequence worth naming:** because the database is a file, it can be **copied, backed up, committed to version control, or served by a web server** — every one of those is an ordinary file operation that the database has no opinion about.

### Part 2: the file is the database, literally

Measured, a freshly created database with one small table:

| | |
|---|---|
| file header | `b'SQLite format 3\x00'` |
| `PRAGMA page_size` | **4096** |
| `PRAGMA page_count` | **2** |
| `PRAGMA encoding` | `UTF-8` |
| `PRAGMA journal_mode` | `delete` |

**The file is an array of fixed-size pages**, and everything — tables, indexes, the schema itself — is a B-tree inside those pages. That is why the header starts with a format string: the layout is a published, stable specification, which is what lets other programs read the file without SQLite.

**And the schema is a table.** Measured:

```
SELECT type, name, sql FROM sqlite_master
  table  t  CREATE TABLE t (a INTEGER, s TEXT)
```

**`sqlite_master` is the catalogue**, queryable like anything else. There is no `information_schema`, no `pg_catalog`; there is one table that describes the database, and any code that can run a query can read it.

**Around the database file there are siblings, and they matter.** A rollback journal (`-journal`) or a write-ahead log (`-wal`) plus a shared-memory file (`-shm`) appear while a transaction is in progress. They are not temporary files to ignore: **they contain database content**, they sit next to the database, and a backup or a copy that misses them can be inconsistent.

### Part 3: types are a property of the value, not of the column

This is the part that surprises people coming from a server engine, and the reason is a design choice rather than a bug. Measured:

| Statement | Result |
|---|---|
| `INSERT INTO loose VALUES ('abc')` where `loose.n` is `INTEGER` | **accepted** — and `typeof(n)` is **`text`** |
| `CREATE TABLE strict_t (n INTEGER) STRICT` then the same insert | **refused** — `cannot store TEXT value in INTEGER column strict_t.n` |
| `INSERT INTO strict_t VALUES (123)` | accepted, `typeof` is `integer` |

**A value in SQLite has one of five storage classes** — `NULL`, `INTEGER`, `REAL`, `TEXT`, `BLOB` — and it keeps whichever it was given. The declared column type is an **affinity**: a preference about how to store a value on the way in, not a rule about what the column may contain.

**So `INTEGER` on a column means "prefer to store these as integers", not "this column holds integers".** The measured column holds `text` after that insert, and every later comparison and calculation deals with whatever is actually there.

**And `STRICT` is the word that changes it.** A `STRICT` table enforces the declared type, which makes the table behave like the other engines in this guide — and it is worth being precise about why that matters:

> **Without `STRICT`, a type error is not rejected; it is stored.** With it, the same insert fails, and the failure is visible at the moment it happens rather than at the moment somebody queries the column and gets a surprise.

### Part 4: concurrency — readers do not block, writers exclude each other

Measured, two writers and two readers against the same file, in the default `delete` journal mode:

| Step | Result |
|---|---|
| reader 1 reads | `1` row |
| writer 1 opens a transaction and inserts | |
| **reader 2 reads while that transaction is open** | **still reads** — readers are not blocked by a writer |
| **writer 2 inserts** | waits, then **`OperationalError: database is locked`** after ~302 ms |
| writer 1 commits, writer 2 retries | **succeeds** |

**The model is one writer at a time.** Readers can proceed while a write transaction is open, but a second writer cannot, and it is told so by an error rather than by waiting forever. `SQLITE_BUSY` is therefore **part of the model, not an exception** — an application that treats it as a crash has a bug, and one that sets a busy timeout and retries has handled it.

**WAL changes what readers see, not who may write.** Measured, after `PRAGMA journal_mode=WAL`:

| Step | Result |
|---|---|
| writer 1 begins and inserts, **not committed** | another connection reads **2** rows — the pre-commit snapshot |
| writer 1 commits | the same connection now reads **3** |

**A reader sees the last committed state**, and the writer's uncommitted work is invisible to it. The single-writer rule still holds; what WAL adds is that readers never block and writers never block readers.

**Two practical settings follow**, and both are about correctness rather than security:

`PRAGMA busy_timeout` makes a connection wait rather than fail immediately. `BEGIN IMMEDIATE` takes the write lock at the start of a transaction instead of at the first write, which turns "I might fail halfway through" into "I either have the lock or I do not".

### Part 5: no users at all

Measured, three statements that work on every server engine in this guide:

| Statement | Result |
|---|---|
| `CREATE USER bob` | **`OperationalError: near "USER": syntax error`** |
| `GRANT SELECT ON t TO bob` | **`OperationalError: near "GRANT": syntax error`** |
| `CREATE ROLE r` | **`OperationalError: near "ROLE": syntax error`** |

**The concepts do not exist.** Where the previous entries in this series discussed roles, hosts, `search_path` and row-level policies, here the answer is that the only thing standing between a program and the data is whether it can open the file:

| Control | Where it lives |
|---|---|
| who may read the file | the file's permission bits and owner |
| who may write it | the same |
| per-table or per-row rules | **nowhere — the database has no such concept** |

**Which redefines what "least privilege" means here.** On a server engine, the application's account is a permission boundary that can be narrowed independently of everything else. Here, the boundary is the file, and the process that holds it — so the questions become: which user does the application run as, what are the file's permissions, and who else can read that directory.

### Part 6: doing it yourself

```bash
sqlite3 app.db
```

```sql
.tables                       -- the tables
.schema t                     -- the DDL as stored
PRAGMA table_info(t);
PRAGMA compile_options;       -- how this build was configured
SELECT type, name, sql FROM sqlite_master;
EXPLAIN QUERY PLAN SELECT ... ;
.backup backup.db             -- a consistent copy while the database is open
VACUUM;                       -- rebuild the file, removing free pages
```

**Three habits worth having:**

**Read `PRAGMA compile_options` when behaviour matters.** A build may or may not have full-text search, may or may not allow loading extensions, and — as the next part shows — may or may not wipe deleted content. Those are compile-time decisions, and the pragma is where they are visible.

**Prefer `.backup` to copying the file.** A file copy taken while a write is in progress can be torn; `.backup` uses the database's own consistency mechanism.

**And remember that an in-memory database is a database.** `:memory:` costs nothing to create, which makes it the right choice for a test, and the wrong choice for anything that has to survive a restart.

**One measured contrast with a server engine:** `ATTACH DATABASE '/tmp/other.db' AS other` works, and then `SELECT ... FROM other.notes` reads across files. **A connection here can reach any database file the process can open**, where a PostgreSQL connection is confined to one database. That is a convenience and it is also the mechanism behind the amplification in Part 8.

### Part 7: where a deleted row goes

This is the part worth reading twice, because the intuitive answer is wrong in both directions. Measured, on the same file, with the same insert and then either a `DELETE` or a `DROP TABLE`, searching the raw bytes of the file afterwards for the deleted value:

| `PRAGMA secure_delete` | Operation | Value still found in the file? |
|---|---|---|
| **OFF** | `DELETE` | **found** |
| **OFF** | `DROP TABLE` | **found** — and still found until `VACUUM` |
| **ON** | `DELETE` | not found |
| **ON** | `DROP TABLE` | not found |

**And this build's default is ON**, which is itself a decision made at compile time:

```
PRAGMA compile_options   -> SECURE_DELETE present
PRAGMA secure_delete     -> 1
```

The SQLite source defaults `secure_delete` to off; this distribution's build enables it. **So the answer to "did deleting that row remove it from the file" is: it depends on a pragma, on how the library was compiled, and on whether a `VACUUM` has run since.**

Three sentences worth keeping:

**A `DELETE` removes the row from the B-tree; it does not necessarily remove the bytes.** With `secure_delete` off, the cell is unlinked and its content stays in the page.

**A `DROP TABLE` returns the pages to a free list**, and the content stays in them until they are reused or the file is rebuilt.

**`VACUUM` rewrites the file**, dropping the free pages — measured, the value stopped being findable after it. It is the operation that makes "gone" mean gone, and it costs a full rewrite.

**Why this is a security topic rather than a curiosity:** an application that stores a token, a card number or a password reset secret, then deletes it, may leave the plaintext in a file that gets **backed up nightly, copied to a laptop, attached to a bug report, or committed to a repository**. The deletion happened in the database; the bytes did not go anywhere.

### Part 8: what follows for security

**The absence of a network is not the absence of exposure.** No port means the classic database finding cannot happen, and the file can still be:
- inside a web root, fetchable as `/app.db`
- returned by a path traversal in the application that owns it
- included in a backup, a snapshot, a container image layer or a repository
- read by any other process on the same host that has the permissions

**Every one of those is an ordinary file exposure**, which is why the mitigation list is about files rather than about database configuration.

**No users means no grants, so the file's mode and owner are the whole policy.** There is no account to narrow, no role to revoke, no row policy to add. That makes the review question short and unusually answerable: **which user runs this process, and who else can read that path.**

**And `ATTACH` is the amplification to know about.** Because a connection can attach any SQLite file it can open, a SQL injection that can reach `ATTACH` — or a feature that lets users supply a path — can read and write **other databases on the same host**, not just the one the application intended. That is a capability with no equivalent in the server engines, and the control is to keep user input away from anything that names a file.

**`load_extension` is the other one.** Measured, this build does **not** disable it: loading a shared library into the process is one pragma away. An application that never uses extensions should turn it off explicitly, because the default is per-build rather than per-application.

**And `STRICT` tables are cheap depth.** Since a non-strict column will happily hold text in an integer column, a value that "cannot happen" can be stored and then behave oddly in every later comparison. Declaring the tables strict makes the database reject it, which is the same guarantee the other engines give by default.

**Finally, the mobile and desktop case deserves its own sentence.** An application's local database is often treated as internal state — trusted because it is local. But local means **on a device the user controls**, where the file can be read, modified and replaced. Data the application put there is not secret from the user, and data the application reads back is not necessarily what it wrote.

### Detection and mitigation

- **Alert on database files appearing inside anything that can be served or shared.** A `.db`, `.sqlite`, `.sqlite3` path under a web root, in a build output, or in a repository is the whole finding, and it is found by looking at artifacts rather than at runtime.
- **Include the sibling files in that check** — `-wal`, `-shm`, `-journal`. They hold committed and uncommitted data, and a backup that omits them can be inconsistent.
- **Inventory backups against the data's retention policy.** A deleted secret still present in last night's backup is a live copy of the secret; the deletion is only as real as the oldest copy of the file.
- **Watch for user input reaching anything that names a file** — `ATTACH`, a backup path, a restore path, an "export to" feature. On this engine a path is a database.
- **And record which build is in use, because the defaults are per-build.** `PRAGMA compile_options`, `PRAGMA secure_delete` and `PRAGMA journal_mode` are three values that decide behaviour people assume is fixed.
- **For mitigation, set the file's permissions deliberately and run the application as a dedicated user.** `600` and one owner is the entire access-control story; nothing else in the database can narrow it.
- **Keep the database out of anything the web server serves, and deny it explicitly at the server.** A path rule is cheaper than hoping nobody guesses the name, and the name is often guessable because it is derived from the application.
- **Add the patterns to version control ignores and to a pre-commit check.** A committed database file is the most common way a local database becomes a public one.
- **Turn off `load_extension` if nothing uses it**, and treat any use of it as code deployment.
- **Use `STRICT` tables for anything holding values that "should" be one type**, and prefer the bound-parameter path where the type is then decided by the code rather than by the input.
- **Decide what `secure_delete` should be, and understand that `VACUUM` is a separate step.** If deleted content must not remain — a token, a key, personal data — the honest controls are to enable secure deletion, to run `VACUUM` where the file is rebuilt anyway, and to remember that backups age out on their own schedule.
- **And for a database on a user's device, assume the user can read and change it.** Encrypt it if the content deserves it, do not store secrets the user is not entitled to, and never treat a value read back from local storage as trusted input.

<!-- lang:zh -->
### 一个库，不是一个服务端

人们对数据库的大部分预设，来自客户端/服务端那个形状：某个地方监听着的服务、一组账号、每张表一条授权。SQLite 这些一个都没有。

> **SQLite 是一个读写文件的库。** 没有进程可以连、没有套接字、没有端口、没有用户，它里面也没有权限系统。

那不是"数据库的简化版"，而是一种为不同问题设计的另一种形态，它出现的地方比任何服务端都多：手机里、桌面软件里、浏览器里、嵌入式设备里，以及 —— 对这份指南最要紧的那一种 —— **Web 应用里**，每次部署、每个租户、每次测试跑，各拿一个自己的文件。

**而形态决定了安全模型。** 没有账号，访问控制就是操作系统的；没有网络，暴露面是文件而不是端口；而数据在一个归应用所有的文件里，于是关于删除、备份与残留的问题，全都是关于那个文件的问题。

下面在 Kali 自带的 **SQLite 3.53.4** 上实测，用一个临时文件，最后删掉。

### 第一部分：它的形状

| | SQLite | 客户端/服务端引擎 |
|---|---|---|
| 它是什么 | **链进程序里的一个库** | 一个服务，通常在另一台主机上 |
| 怎么够到它 | **打开一个文件**（或 `:memory:`） | 通过套接字或 TCP 连接 |
| 认证 | **没有** | 账号与口令 |
| 授权 | **没有** | `GRANT`、角色、行策略 |
| 并发 | **文件锁；同一时刻一个写者** | MVCC，多个写者 |
| 同时的用户 | 一个进程，通常如此 | 成千上万 |
| 部署 | **没有东西要装、要跑** | 一个要一直跑着的服务端 |

**两个后果立刻成立，而两个都是安全后果。**

**没有网络攻击面。** "数据库能从公网够到"这一整类在这里不存在 —— 但"数据库文件能通过 HTTP 拉到"这一类存在，而它是同一条发现换了一种传输方式。

**没有可配置的权限模型。** 这里的"最小权限"指的是一个文件的权限位、以及持有它的那个进程的身份，不是一组授权。

**还有一个值得点名的设计后果：** 因为数据库是一个文件，它可以被**复制、备份、提交进版本控制、或者被一个 web 服务器提供出去** —— 每一件都是普通的文件操作，而数据库对此没有任何意见。

### 第二部分：文件就是数据库，字面意义上

实测，一个刚建好、只有一张小表的数据库：

| | |
|---|---|
| 文件头 | `b'SQLite format 3\x00'` |
| `PRAGMA page_size` | **4096** |
| `PRAGMA page_count` | **2** |
| `PRAGMA encoding` | `UTF-8` |
| `PRAGMA journal_mode` | `delete` |

**文件是一个固定大小页组成的数组**，而一切 —— 表、索引、schema 本身 —— 都是那些页里的 B 树。这就是文件头以一段格式字符串开头的原因：那套布局是一份公开、稳定的规范，也正是别的程序能不用 SQLite 就读这个文件的原因。

**而 schema 本身就是一张表。** 实测：

```
SELECT type, name, sql FROM sqlite_master
  table  t  CREATE TABLE t (a INTEGER, s TEXT)
```

**`sqlite_master` 就是那份目录**，和别的东西一样可以查。这里没有 `information_schema`、没有 `pg_catalog`；只有一张描述这个数据库的表，而任何能执行查询的代码都能读它。

**数据库文件旁边还有兄弟文件，而它们要紧。** 事务进行中会出现回滚日志（`-journal`）或者预写日志（`-wal`）外加一个共享内存文件（`-shm`）。它们不是可以忽略的临时文件：**它们包含数据库内容**，它们就躺在数据库旁边，而一次漏掉它们的备份或拷贝可能是不一致的。

### 第三部分：类型是值的性质，不是列的性质

这是从服务端引擎过来的人最容易意外的一处，而原因是设计选择而不是 bug。实测：

| 语句 | 结果 |
|---|---|
| 往 `INTEGER` 列 `loose.n` 里 `INSERT INTO loose VALUES ('abc')` | **接受** —— 而 `typeof(n)` 是 **`text`** |
| `CREATE TABLE strict_t (n INTEGER) STRICT` 之后同样插入 | **拒绝** —— `cannot store TEXT value in INTEGER column strict_t.n` |
| `INSERT INTO strict_t VALUES (123)` | 接受，`typeof` 是 `integer` |

**SQLite 里的一个值属于五种存储类之一** —— `NULL`、`INTEGER`、`REAL`、`TEXT`、`BLOB` —— 而它保持被给到的那个。声明的列类型是一种**亲和性**：一个关于"进来时倾向怎么存"的偏好，而不是一条关于"这一列可以装什么"的规则。

**所以列上的 `INTEGER` 意思是"倾向把这些按整数存"，不是"这一列装整数"。** 实测那一列在那次插入之后装的是 `text`，而之后每一次比较与计算都得应付"实际在那儿的东西"。

**而 `STRICT` 就是改变这件事的那个词。** 一张 `STRICT` 表会强制声明的类型，这让它的行为与这份指南里其他引擎一致 —— 而值得把原因说准：

> **没有 `STRICT`，类型错误不会被拒绝，而是被存下来。** 有了它，同样的插入会失败，而那次失败在发生的那一刻就可见，而不是等到有人查那一列、看到一个意外时才可见。

### 第四部分：并发 —— 读不阻塞，写者互相排斥

实测，在默认的 `delete` 日志模式下，两个写者和两个读者对着同一个文件：

| 步骤 | 结果 |
|---|---|
| 读者 1 读 | `1` 行 |
| 写者 1 开事务并插入 | |
| **读者 2 在那个事务开着时读** | **照样读得到** —— 读不被写阻塞 |
| **写者 2 插入** | 等待，约 302 ms 后 **`OperationalError: database is locked`** |
| 写者 1 提交后写者 2 重试 | **成功** |

**模型是同一时刻只有一个写者。** 写事务开着时读者可以继续，但第二个写者不行，而它是被一个错误告知的、不是永远等下去。所以 **`SQLITE_BUSY` 是这个模型的一部分，不是一个异常** —— 把它当崩溃处理的应用有 bug，而设了 busy timeout 并重试的应用处理了它。

**WAL 改变的是读者看到什么，不是谁可以写。** 实测，`PRAGMA journal_mode=WAL` 之后：

| 步骤 | 结果 |
|---|---|
| 写者 1 开始并插入，**未提交** | 另一个连接读到 **2** 行 —— 提交前的快照 |
| 写者 1 提交 | 同一个连接现在读到 **3** |

**读者看到的是最后提交的状态**，写者未提交的工作对它不可见。单写者规则仍然成立；WAL 增加的是"读从不阻塞写、写从不阻塞读"。

**由此两个实用设置**，而两者都关于正确性而不是安全：

`PRAGMA busy_timeout` 让连接等一会儿而不是立刻失败。`BEGIN IMMEDIATE` 在事务开始时就拿写锁，而不是等到第一次写 —— 这把"我可能做到一半失败"变成"我要么拿到锁、要么没有"。

### 第五部分：根本没有用户

实测，三条在这份指南里每个服务端引擎上都能用的语句：

| 语句 | 结果 |
|---|---|
| `CREATE USER bob` | **`OperationalError: near "USER": syntax error`** |
| `GRANT SELECT ON t TO bob` | **`OperationalError: near "GRANT": syntax error`** |
| `CREATE ROLE r` | **`OperationalError: near "ROLE": syntax error`** |

**这些概念不存在。** 在这个系列的前几篇里讨论的角色、来源主机、`search_path`、行级策略，在这里的答案都是：横在程序和数据之间的唯一东西，是它能不能打开那个文件。

| 控制 | 住在哪里 |
|---|---|
| 谁能读这个文件 | 文件的权限位与属主 |
| 谁能写它 | 同上 |
| 按表或按行的规则 | **哪里都没有 —— 数据库里没有这个概念** |

**这重新定义了"最小权限"在这里的含义。** 在服务端引擎上，应用那个账号是一条可以独立收窄的权限边界。这里，边界就是文件、以及持有它的进程 —— 于是问题变成了：应用以哪个用户身份跑、那个文件的权限是什么、以及还有谁能读那个目录。

### 第六部分：自己动手做一次

```bash
sqlite3 app.db
```

```sql
.tables                       -- 有哪些表
.schema t                     -- 存下来的 DDL
PRAGMA table_info(t);
PRAGMA compile_options;       -- 这个构建是怎么配置的
SELECT type, name, sql FROM sqlite_master;
EXPLAIN QUERY PLAN SELECT ... ;
.backup backup.db             -- 数据库开着时做一份一致的拷贝
VACUUM;                       -- 重建文件，去掉空闲页
```

**三个值得养成的习惯：**

**当行为要紧时，读一遍 `PRAGMA compile_options`。** 一个构建可能有也可能没有全文检索、可能允许也可能不允许加载扩展，以及 —— 下一部分会看到 —— 可能擦除也可能不擦除被删掉的内容。那些都是编译期的决定，而这个 pragma 是它们可见的地方。

**备份用 `.backup`，不要直接拷文件。** 写入进行中做文件拷贝可能被撕裂；`.backup` 用的是数据库自己的一致性机制。

**并且记住内存数据库也是一个数据库。** `:memory:` 建起来不花代价，这让它适合测试，而不适合任何需要活过重启的东西。

**一个与前面那些引擎实测到的对照：** `ATTACH DATABASE '/tmp/other.db' AS other` 是可用的，然后 `SELECT ... FROM other.notes` 能跨文件读。**这里一个连接能碰到的，是这个进程能打开的任何数据库文件**，而一个 PostgreSQL 连接被限制在一个数据库里。这既是便利，也正是第八部分那条放大的机制。

### 第七部分：被删掉的那一行去了哪里

这一部分值得读两遍，因为直觉的答案在两个方向上都是错的。实测，同一个文件、同样的插入，然后要么 `DELETE` 要么 `DROP TABLE`，之后在文件的原始字节里搜那个被删掉的值：

| `PRAGMA secure_delete` | 操作 | 文件里还找得到那个值吗 |
|---|---|---|
| **OFF** | `DELETE` | **找得到** |
| **OFF** | `DROP TABLE` | **找得到** —— 而且在 `VACUUM` 之前一直找得到 |
| **ON** | `DELETE` | 找不到 |
| **ON** | `DROP TABLE` | 找不到 |

**而这个构建的默认值是 ON**，它本身是一个在编译期做出的决定：

```
PRAGMA compile_options   -> 有 SECURE_DELETE
PRAGMA secure_delete     -> 1
```

SQLite 源码里 `secure_delete` 的默认是关；这个发行版的构建把它编成了开。**所以"删掉那一行有没有把它从文件里去掉"的答案是：取决于一个 pragma、取决于那个库怎么编译的、以及自那之后有没有跑过 `VACUUM`。**

三句话值得记住：

**`DELETE` 把那一行从 B 树里摘掉，不一定把那些字节去掉。** 在 `secure_delete` 关闭时，那个单元被解链，而它的内容留在页里。

**`DROP TABLE` 把页还给空闲列表**，而内容留在那些页里，直到它们被复用或者文件被重建。

**`VACUUM` 会重写这个文件**，丢掉空闲页 —— 实测，那之后这个值就找不到了。它就是让"没了"真的等于没了的那个操作，代价是一次完整的重写。

**为什么这是安全话题而不是冷知识：** 一个应用存了一个令牌、一个卡号或者一个口令重置密钥，然后把它删掉，可能把那串明文留在一个文件里 —— 而那个文件会被**每晚备份、被拷到笔记本上、被附在一次 bug 报告里、或者被提交进仓库**。删除发生在数据库里；那些字节哪儿也没去。

### 第八部分：从这些机制推出的安全观念

**没有网络不等于没有暴露。** 没有端口意味着经典的那种数据库发现不可能发生，而那个文件仍然可能：
- 在 web 根目录里，能被当作 `/app.db` 拉下来
- 被拥有它的那个应用里的路径穿越吐出来
- 出现在一次备份、一个快照、一个容器镜像层或者一个仓库里
- 被同一台主机上任何一个有权限的进程读走

**上面每一条都是普通的文件暴露**，这就是为什么缓解清单是关于文件的，而不是关于数据库配置的。

**没有用户就没有授权，所以文件的模式和属主就是全部的策略。** 没有账号可以收窄、没有角色可以撤销、没有行策略可以加。这让评审的问题变得很短、而且罕见地可回答：**这个进程以哪个用户跑，还有谁能读那个路径。**

**而 `ATTACH` 是那个要知道的放大点。** 因为一个连接可以挂上它能打开的任何 SQLite 文件，一次能触到 `ATTACH` 的 SQL 注入 —— 或者一个允许用户提供路径的功能 —— 就能读写**同一台主机上的其他数据库**，而不只是应用打算用的那一个。这是一个在服务端引擎里没有对应物的能力，而控制手段是不让用户输入碰到任何"命名一个文件"的东西。

**`load_extension` 是另一个。** 实测，这个构建**没有**禁用它：把一份共享库加载进进程只差一个 pragma。从不使用扩展的应用应当显式关掉它，因为那个默认是按构建而定的、不是按应用而定的。

**而 `STRICT` 表是便宜的纵深。** 因为非严格的一列会乐意在整数列里装着文本，"不可能发生"的值可以被存下来，然后在之后每一次比较里表现古怪。把表声明成严格，会让数据库拒绝它 —— 这和其他引擎默认给出的那份保证是一样的。

**最后，手机和桌面那一种情形值得单独一句。** 应用的本地数据库常被当成内部状态 —— 因为它是本地的，所以被信任。但本地意味着**在一个用户控制的设备上**，那里文件能被读、被改、被替换。应用放进去的数据对用户不是秘密，而应用读回来的数据不一定是它写进去的。

### 检测与缓解

- **对"数据库文件出现在任何能被提供出去或共享的东西里"告警。** 一个 web 根目录下、一个构建产物里、或者一个仓库里的 `.db`、`.sqlite`、`.sqlite3` 路径就是整条发现，而它是靠看产物而不是靠看运行时发现的。
- **把兄弟文件也纳入那个检查** —— `-wal`、`-shm`、`-journal`。它们装着已提交与未提交的数据，而一次漏掉它们的备份可能不一致。
- **拿备份的数据保留期与数据库对账。** 一个被删掉的密钥仍然留在昨晚的备份里，就是那个密钥的一份活副本；删除的真实程度，只等于那个文件最老的一份拷贝。
- **盯"用户输入碰到任何命名文件的东西"** —— `ATTACH`、一个备份路径、一个恢复路径、一个"导出到"功能。在这个引擎上，一个路径就是一个数据库。
- **并且记下在用哪个构建，因为默认值是按构建而定的。** `PRAGMA compile_options`、`PRAGMA secure_delete`、`PRAGMA journal_mode` 是三个决定"人们以为是固定的行为"的值。
- **缓解上，有意识地设好文件权限，并让应用以一个专用用户跑。** `600` 加一个属主就是访问控制的全部；数据库里没有别的东西能收窄它。
- **把数据库放在 web 服务器提供的东西之外，并在服务器上显式拒绝它。** 一条路径规则比"指望没人猜到那个名字"便宜，而那个名字常常是可以猜到的，因为它由应用派生而来。
- **把那些模式加进版本控制的忽略清单和一次提交前检查。** 一个被提交的数据库文件，是本地数据库变成公开数据库最常见的方式。
- **如果没有东西用它，就关掉 `load_extension`**，并把任何对它的使用当成一次代码部署。
- **给"本应是某种类型"的值用 `STRICT` 表**，并优先走绑定参数的路径 —— 那样类型由代码决定，而不是由输入决定。
- **决定 `secure_delete` 该是哪个值，并明白 `VACUUM` 是另一个独立步骤。** 如果被删的内容不能留下 —— 一个令牌、一个密钥、个人数据 —— 诚实的控制是打开安全删除、在本来就要重建文件的场合跑 `VACUUM`，并记住备份按它自己的节奏过期。
- **而对一个在用户设备上的数据库，假定用户能读也能改它。** 内容值得的话就加密，不要存用户无权得到的秘密，并且永远不要把从本地存储读回来的值当成可信输入。
