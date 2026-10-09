---
id: db-redis
title_en: Redis
title_zh: Redis
summary_en: The one store here whose value is itself a data structure, executed one command at a time with the configuration writable while it runs. Measured on a live instance — a global stall from a single command, the file-write chain and the switch that now blocks it, two kinds of transaction error that behave differently, and a Lua sandbox with holes in known places.
summary_zh: 这是这份指南里唯一一个"值本身就是数据结构"的存储，命令一条一条执行，而配置在运行中可以改写。在真在跑的实例上实测 —— 一条命令造成的全局停顿、那条写文件的链以及现在挡住它的开关、两种行为完全不同的事务错误，以及一个在已知位置留了口的 Lua 沙箱。
tags: [beginner, database, redis, keyvalue, cache, acl, lua]
tools: [redis-server, redis-cli, python3]
attck: [T1213]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A value is a data structure

Every store so far keeps rows or documents: the shape is decided when the data is written, and a value is a value. Redis does something else.

> **The value at a key is itself a data structure** — a string, a list, a hash, a set, a sorted set, a stream, a bitmap. The key is a name, and the type of what it holds is decided by the first command that used it.

Two consequences follow immediately, and they are the spine of this entry:

**A command has to know what it is operating on**, so the type is enforced at run time rather than declared up front — and a mismatch is an error rather than a conversion.

**And commands are executed one at a time.** Redis runs them on a single thread, which is why each one is atomic without any locking, and why a single slow one stalls every other client.

Measured below on a live **Redis 8.0.6** started on a temporary port with a temporary directory, removed afterwards.

### Part 1: the shape of it

| | Here |
|---|---|
| unit | a **key** and the structure at it |
| types | string, list, hash, set, sorted set, stream, bitmap, HyperLogLog, geo |
| schema | **none declared — the type comes from how the key was first used** |
| keyspace | one flat namespace; databases are numbered, not named |
| storage | **memory first**; persistence is a copy written out |
| execution | **one command at a time, on one thread** |

Measured, five structures created in the same keyspace with no declaration of any kind:

```
SET   a-string hello      -> OK
LPUSH a-list x y          -> 2
ZADD  a-zset 1 one 2 two  -> 2
HSET  a-hash f v          -> 1
SADD  a-set m1 m2         -> 2
```

**And the type is enforced by the command, not by the key.** Measured, using a list command against a string:

```
LPUSH a-string x  -> ERR WRONGTYPE Operation against a key holding the wrong kind of value
```

**But the reverse is allowed:**

```
SET a-list v      -> OK
```

**`SET` overwrites whatever is there, whatever type it was.** So the rule is narrower than "types are checked":

> **A type is enforced by the commands that operate on a structure. Commands that treat a key as an opaque value — `SET`, `DEL`, `EXPIRE` — do not know or care what it holds.**

**Which is why a wrong assumption about a key's type shows up as a runtime error at the moment of use**, and why that error can arrive after the key has been in production for months. Nothing declares the shape; the first command that used it did.

### Part 2: one command at a time, and why latency is global

This is the property that shapes everything about running Redis. Measured, one client executing a script that does real work, while another client sends `PING`:

| | |
|---|---|
| the script ran for | **266 ms** |
| the other client's `PING` during that time | **66 ms** |
| `PING` on an idle instance | **0.03 ms** |

**A two-thousand-fold difference, caused by one command from another connection.** The `PING` was not slow; it was queued behind work that could not be interrupted.

**That is the direct consequence of the single-threaded model**, and it cuts both ways:

> **Every command is atomic for free — and every command's cost is paid by everyone.**

**Which turns ordinary commands into availability controls.** The measured next example is the one to remember, on a keyspace of 500 000 keys:

| Command | Total | Worst stall seen by another client |
|---|---|---|
| `KEYS bench:*` | **244 ms** | **84 ms** |
| `SCAN` in batches of 1000 | 358 ms | **2 ms** |

**Read the columns together.** `SCAN` took *longer* in total and was *vastly* better behaved: it never held the server for more than a couple of milliseconds. `KEYS` returned faster and froze every other client for 84 ms.

**So the rule about `KEYS` is not about its own duration.** It is that the stall it causes is proportional to the whole keyspace, it happens inside one command, and it happens for everyone. `SCAN` exists precisely to spread that work, at the cost of being a cursor: it may return a key more than once, and it does not guarantee a consistent snapshot. **Both of those are the price of not stopping the world, and both have to be handled by the caller.**

**The same reasoning applies to `FLUSHALL`, `FLUSHDB`, a wide `DEL`, `SORT`, and any large `SMEMBERS` or `HGETALL`.** None of them are wrong; all of them are global pauses whose size the application chooses.

### Part 3: expiry, and the difference between expired and gone

Redis expires keys, and it does it in two ways at once: **lazily**, when a key is touched, and **actively**, by a loop that samples the keyspace. Measured, with the active loop switched off so that only the lazy path runs:

```
DEBUG SET-ACTIVE-EXPIRE 0
SET short v PX 1000 ; SET long v
DBSIZE                     -> 2
   ... wait 1.8 seconds ...
DBSIZE                     -> 2      the expired key is still counted
EXISTS short               -> 0      this call is what removed it
DBSIZE                     -> 1
```

**Three steps, and the order decides what you observe.** The expired key was invisible to `EXISTS` — it was logically gone the moment its time passed — and it was still occupying a slot until something touched it.

**So "expired" and "gone" are two different states**, and only the second frees memory. Which matters for two reasons:

**Memory is freed late.** Under a memory limit, an instance full of logically-expired keys still counts them.

**And `DBSIZE` is not a count of live data.** It is a count of keys the keyspace still holds, including ones that would disappear if you asked about them.

**And with the active loop on — the normal case — the free happens without anybody asking**, which is why this is invisible most of the time and appears exactly when the instance is under pressure. The knobs that matter are `maxmemory` (when Redis starts evicting) and `maxmemory-policy` (which keys it picks), and neither of them is about expiry: **they decide what to sacrifice when memory runs out, and `noeviction` means writes start failing instead.**

### Part 4: transactions, and two kinds of error

`MULTI` queues commands and `EXEC` runs them together. The sentence everyone repeats is "Redis transactions do not roll back", and measured, that sentence is only true for one of the two ways a transaction can fail.

**An error while queueing — an unknown command — discards the whole transaction.** Measured:

```
MULTI                      -> OK
SET q 2                    -> QUEUED
NOTACOMMAND                -> ERR unknown command 'NOTACOMMAND'
SET q 3                    -> QUEUED
EXEC                       -> ERR EXECABORT Transaction discarded because of previous errors.
GET q                      -> 1        (unchanged)
```

**Nothing ran**, and the key kept its original value.

**An error during execution — a wrong type — affects only that command.** Measured:

```
MULTI                      -> OK
SET q 10                   -> QUEUED
LPUSH s1 x                 -> QUEUED      (s1 holds a string; the error is not known yet)
SET q 20                   -> QUEUED
EXEC                       -> ['OK', WRONGTYPE ..., 'OK']
GET q                      -> 20          (both writes happened)
```

**Two of the three commands succeeded and the third failed**, and the queue was not unwound.

**So the precise statement is:** a queueing error is caught before anything runs and aborts everything; an execution error is discovered while running and leaves the rest committed.

**And the reason is that the queue is validated for syntax, not for effect.** Whether `LPUSH` will fail depends on what the key holds at execution time, and that is not knowable when the command is queued.

**Which leaves `WATCH` as the only way to do a conditional transaction** — optimistic locking: watch the keys, and `EXEC` fails if any of them changed. There is no mechanism for undoing a committed write inside the transaction.

### Part 5: access control, down to key names and commands

For most of its history Redis had one password (`requirepass`) and nothing else. Since version 6 it has users, and the permissions are unusually fine-grained. Measured, a user created with a key pattern and a command list:

```
ACL SETUSER app on >pw ~session:* +get +set +ping
```

| As that user | Result |
|---|---|
| `GET session:1` | allowed (returns `nil` for a missing key) |
| `GET other:1` | **`NOPERM No permissions to access a key`** |
| `DEL session:1` | **`NOPERM User app has no permissions to run the 'del' command`** |
| `CONFIG GET dir` | **`NOPERM ... 'config\|get'`** |

**Three separate refusals: the key was out of pattern, the command was not granted, and a whole command family was closed.** And the permissions can be tested without changing anything:

```
ACL DRYRUN app flushall  -> User app has no permissions to run the 'flushall' command
ACL DRYRUN default flushall -> OK
```

**Which makes the review question concrete.** Where the application shares a Redis instance with anything else — a cache, a queue, another service's session store — **an ACL user per consumer with a key prefix pattern and the commands it actually needs is the difference between one compromised service and all of them.** Without it, `requirepass` is one secret shared by everything, and any holder of it can read every key.

### Part 6: the configuration is writable while it runs

An operational convenience with a sharp edge: `CONFIG SET` changes most settings without a restart. Measured, the chain that matters, and the switch that now blocks it:

| Configuration | `CONFIG SET dir /tmp/target` | Then `dbfilename` + `SAVE` |
|---|---|---|
| `enable-protected-configs no` (default) | **refused — `can't set protected config`** | — |
| `enable-protected-configs yes` | **`OK`** | **a file written at the chosen path** |

With the setting enabled, the measured result was a 122-byte file starting `REDIS0012` — the RDB magic — written wherever the operator said, containing the value that had just been written to a key:

```
CONFIG SET dir /tmp/redis-write3        -> OK
CONFIG SET dbfilename planted.rdb       -> OK
SET planted payload-in-the-file
SAVE                                    -> OK
  -> /tmp/redis-write3/planted.rdb, 122 bytes, the value is inside it
```

**So the primitive is: a writable configuration plus a command that writes a file.** Redis 7 added the protected-config list and turns this off by default — **which is why the finding is now version- and configuration-dependent rather than universal**, and why both halves belong in the same sentence: an instance with protected configs enabled, or one whose startup parameters an attacker can influence, has a file-write primitive reachable from any connection that can run `CONFIG`.

**And `dir` is not the only protected one for a reason.** The related history is `dbfilename` pointed at a path that gets loaded — a cron file, an SSH key, a web-served file — and the replication path: point an instance at a rogue replica with `REPLICAOF`, and the replica's data becomes the master's. Both are configuration changes rather than exploits, which is the point of this part.

**The counterweight is that the same interface is how a legitimate operator manages the instance.** The control is who can reach `CONFIG` at all — which is Part 5, and which is what `rename-command` did before ACLs existed.

### Part 7: scripting

`EVAL` runs Lua on the server. Measured, what the sandbox provides and what it removes:

| Probe | Result |
|---|---|
| `redis.call("DBSIZE")` | works |
| **`redis.call("CONFIG", "GET", "dir")`** | **`ERR This Redis command is not allowed from script`** |
| `os ~= nil` | `true` |
| **`os.execute ~= nil`** | **`false`** |
| `io`, `package`, `debug` | **`Script attempted to access nonexistent global variable`** |

**Two layers of restriction, and they are different in kind.** The dangerous Lua globals are gone — `os.execute` is nil and `io`, `package` and `debug` are not merely nil but named as nonexistent, so a script that reaches for them is refused rather than silently getting nil.

**And the command surface is filtered too**: a list of administrative commands is refused from inside a script, `CONFIG` among them.

**But `redis.call` remains, and it runs with the connection's privileges.** A script can call anything the calling connection is allowed to call — which is why a script built from user input is not "a query with a scripting language"; it is **server-side code execution with the permissions of that connection**. On an instance with a single unrestricted user, that is everything.

**And modules go further.** Measured, `MODULE LIST` reports a built-in module; `MODULE LOAD` takes a path to a shared object and loads it into the server process. That is a legitimate extension mechanism and the end of the escalation path that starts with a file write.

### Part 8: what follows for security

**The measured defaults are two protections, and both are about reachability rather than authorisation.**

```
requirepass     -> (empty)
protected-mode  -> yes
```

**No password is set, and protected mode refuses connections from outside loopback when no password is set.** So a fresh instance is not open by accident — it is open only if somebody binds it to a wider address while leaving the password empty, which is exactly what "make it reachable from the app server" instructions do.

**Which makes the two review questions the same as they were for the document store:** what address is it bound to, and does it require anything to connect. On the address question Redis is more forgiving than most, because the default is loopback and protected mode is on.

**Configurability is the second theme.** The measured chain — `CONFIG SET dir`, `CONFIG SET dbfilename`, `SAVE` — is a file write, and a file write plus a loadable module is execution. **So `CONFIG` belongs in the same category as `EVAL` and `MODULE LOAD`: administrative interfaces whose reachability is the security boundary.** An application connection has no reason to be able to call any of them.

**Third: latency is a shared resource, and a user-facing feature can spend it.** Measured, `KEYS` froze the instance for 84 ms while `SCAN` never exceeded 2 ms. A search box, an admin page, or a debugging endpoint that walks the keyspace is a denial of service with a friendly name — and the fix is `SCAN`, or an index, or not doing it at all.

**Fourth: expired is not the same as gone.** With the active loop off, `DBSIZE` counted a key that `EXISTS` reported as missing. Memory limits, eviction and `INFO memory` all refer to the first state, while the application sees the second.

**Fifth: the access control is there and is usually unused.** The measured ACL refused a key outside the pattern, a command that was not granted, and a whole command family — per user. **Most deployments have one password or none, which means every consumer can do everything.** Where an instance is shared, ACL users with key patterns are the cheapest available isolation.

**And sixth, about data at rest.** Persistence writes a copy of the keyspace to disk — `dump.rdb` by default — and it is not encrypted. It is written next to whatever `dir` says, included in whatever backups include that directory, and readable by anything that can read the file. **A cache holding session tokens or personal data has a plaintext copy on disk, and the entry on embedded databases applies here too: what the store deletes and what the disk still has are two different questions.**

### Detection and mitigation

- **Check the bind address and whether authentication is required, together.** Empty `requirepass` plus a non-loopback bind plus protected mode is a combination that only holds while protected mode is trusted to keep the instance unreachable.
- **Alert on `CONFIG SET` from application connections, and on `CONFIG SET dir`, `dbfilename`, `save`, `appendonly` especially.** They are configuration changes at runtime, and the measured file write needs exactly two of them.
- **Alert on `REPLICAOF`/`SLAVEOF`, `MODULE LOAD`, and `EVAL` from anything but an administrative path.** They are the primitives that turn a data-store compromise into execution.
- **Watch for `KEYS`, `FLUSHALL`, `FLUSHDB` and wide `DEL` in application code.** They are measurable stalls, and their cost scales with the keyspace rather than with the request.
- **Track `evicted_keys`, `rejected_connections` and the memory high-water mark**, because an instance at its limit starts failing writes under `noeviction` and starts silently dropping data under the others.
- **For mitigation, keep the default loopback bind and protected mode, and require a password or ACL user for anything that must cross a network.** The defaults are already the safe configuration; the work is not undoing them casually.
- **Give each consumer its own ACL user with a key pattern and the commands it needs.** Measured, that refused a key outside the pattern and a command outside the list — which is isolation a shared password cannot provide.
- **Deny administrative commands to application users** — `CONFIG`, `EVAL`, `MODULE`, `REPLICAOF`, `FLUSHALL` — rather than relying on nobody calling them.
- **Keep `protected-mode` on and consider leaving the protected configuration list intact**, since it is the measured block on the file-write chain and rarely needs disabling.
- **Set `maxmemory` and a policy deliberately**, and remember that eviction is data loss: choosing `allkeys-lru` for a session store means sessions disappear under pressure, by design.
- **Treat the persistence file as data.** Its permissions, its location, its inclusion in backups and whether the disk is encrypted are all part of the answer to "where does this data live".
- **Use `SCAN` rather than `KEYS`, and expect its two limitations** — possible duplicates and no consistent snapshot — rather than assuming it behaves like a complete listing.
- **And do not build tenant isolation out of key names alone.** Key prefixes are a convention, and every command in Part 5 exists because a convention is not a boundary.

<!-- lang:zh -->
### 值本身就是一种数据结构

到目前为止的每一个存储都在存行或者文档：形状在写入时就被决定了，而值是值。Redis 做的是另一件事。

> **键上的那个值本身就是一种数据结构** —— 字符串、列表、哈希、集合、有序集合、流、位图。键是一个名字，而它里面装着什么类型，由第一个用过它的命令决定。

两个后果立刻成立，而它们就是这一篇的主干：

**一条命令必须知道它在操作什么**，所以类型是在运行时被强制的、而不是事先声明的 —— 而类型不匹配是一个错误，不是一次转换。

**而命令是一条一条执行的。** Redis 在单线程上跑它们，这就是为什么每条命令无需任何加锁就是原子的，也是一条慢命令会挡住其他所有客户端的原因。

下面在一个真在跑的 **Redis 8.0.6** 上实测：临时端口、临时目录，跑完删掉。

### 第一部分：它的形状

| | 这里 |
|---|---|
| 单位 | 一个**键**，以及挂在它上面的那个结构 |
| 类型 | 字符串、列表、哈希、集合、有序集合、流、位图、HyperLogLog、地理 |
| schema | **不声明 —— 类型来自这个键第一次是怎么被用的** |
| 键空间 | 一个扁平的名字空间；数据库是按编号分的，不是按名字 |
| 存储 | **内存优先**；持久化是另写出去的一份拷贝 |
| 执行 | **一条一条、在一个线程上** |

实测，在一个键空间里建出五种结构，没有任何声明：

```
SET   a-string hello      -> OK
LPUSH a-list x y          -> 2
ZADD  a-zset 1 one 2 two  -> 2
HSET  a-hash f v          -> 1
SADD  a-set m1 m2         -> 2
```

**而类型是由命令强制的，不是由键强制的。** 实测，用一个列表命令去操作一个字符串：

```
LPUSH a-string x  -> ERR WRONGTYPE Operation against a key holding the wrong kind of value
```

**但反过来是允许的：**

```
SET a-list v      -> OK
```

**`SET` 会覆盖那里原有的任何东西，不管它原来是什么类型。** 所以规则比"类型会被检查"更窄：

> **类型由那些"按结构操作"的命令来强制。而把一个键当作不透明值来对待的命令 —— `SET`、`DEL`、`EXPIRE` —— 既不知道也不关心它装的是什么。**

**这就是为什么对一个键类型的错误假设，会在用到它的那一刻以运行时错误的形式出现**，也是为什么那个错误可能在这个键上线几个月之后才到来。没有任何东西声明形状；是第一个用过它的命令决定的。

### 第二部分：一次一条命令，以及为什么延迟是全局的

这条性质塑造了运行 Redis 的一切。实测，一个客户端在执行一段真正干活的脚本，同时另一个客户端在发 `PING`：

| | |
|---|---|
| 脚本跑了 | **266 ms** |
| 那段时间里另一个客户端的 `PING` | **66 ms** |
| 空闲实例上的 `PING` | **0.03 ms** |

**两千倍的差距，只因为另一个连接发了一条命令。** `PING` 本身不慢；它是排在一件不可打断的工作后面等。

**这就是单线程模型的直接后果**，而它是双向的：

> **每条命令都免费地原子 —— 而每条命令的代价由所有人一起付。**

**这就把普通命令变成了可用性上的控制点。** 实测的下一组例子是最该记住的，在一个五十万键的键空间上：

| 命令 | 总耗时 | 别的客户端遇到的最长停顿 |
|---|---|---|
| `KEYS bench:*` | **244 ms** | **84 ms** |
| `SCAN` 每次 1000 个 | 358 ms | **2 ms** |

**两列要一起读。** `SCAN` 总耗时**更长**、而表现**好得多**：它从来没有把服务端按住超过两三毫秒。`KEYS` 返回更快，却把其他每个客户端冻了 84 毫秒。

**所以关于 `KEYS` 的规则不是关于它自己的耗时。** 而是它造成的停顿与整个键空间成正比、发生在一个命令内部、而且发生在所有人身上。`SCAN` 的存在就是为了把这份工作摊开，代价是它变成一个游标：它可能把同一个键返回多次，也不保证一致的快照。**两者都是"不停止世界"的价格，而两者都得由调用方处理。**

**同样的推理适用于 `FLUSHALL`、`FLUSHDB`、范围很大的 `DEL`、`SORT`，以及任何大的 `SMEMBERS` 或 `HGETALL`。** 它们没有一个是错的；它们全都是全局暂停，而大小由应用选择。

### 第三部分：过期，以及"过期了"与"没有了"的差别

Redis 会让键过期，而它同时用两种方式做：**惰性**的，在被碰到的时候；以及**主动**的，由一个采样键空间的循环来做。实测，把主动循环关掉、只留惰性那条路：

```
DEBUG SET-ACTIVE-EXPIRE 0
SET short v PX 1000 ; SET long v
DBSIZE                     -> 2
   ……等 1.8 秒……
DBSIZE                     -> 2      过期的键仍然被算着
EXISTS short               -> 0      是这一问把它删掉的
DBSIZE                     -> 1
```

**三步，而顺序决定了你看到什么。** 那个过期的键对 `EXISTS` 已经不可见了 —— 在它的时间过去那一刻，它在逻辑上就没了 —— 而它到有东西碰到它之前，仍然占着一个位置。

**所以"过期"和"没了"是两个不同的状态**，而只有后者会释放内存。这有两层意义：

**内存释放得晚。** 在内存限额之下，一个装满逻辑上已过期键的实例仍然把它们算进去。

**而 `DBSIZE` 不是活数据的计数。** 它是键空间仍然持有的键的计数，包括那些你一问就会消失的。

**而在主动循环开着的时候 —— 也就是正常情况 —— 释放不需要任何人来问**，这就是为什么这件事大多数时候看不见，而恰好在实例有压力的时候出现。要紧的旋钮是 `maxmemory`（Redis 何时开始淘汰）与 `maxmemory-policy`（它挑哪些键），而两者都不是关于过期的：**它们决定内存用尽时牺牲什么，而 `noeviction` 意味着写入开始失败。**

### 第四部分：事务，以及两种错误

`MULTI` 把命令排队，`EXEC` 一起执行。人人都重复的那句话是"Redis 的事务不回滚"，而实测下来，那句话只在事务失败的两条路中的一条上成立。

**排队期间出错 —— 一条不认识的命令 —— 会让整个事务作废。** 实测：

```
MULTI                      -> OK
SET q 2                    -> QUEUED
NOTACOMMAND                -> ERR unknown command 'NOTACOMMAND'
SET q 3                    -> QUEUED
EXEC                       -> ERR EXECABORT Transaction discarded because of previous errors.
GET q                      -> 1        （没变）
```

**什么都没跑**，而那个键保持着原来的值。

**执行期间出错 —— 类型不对 —— 只影响那一条命令。** 实测：

```
MULTI                      -> OK
SET q 10                   -> QUEUED
LPUSH s1 x                 -> QUEUED      （s1 里是字符串；此时还不知道会出错）
SET q 20                   -> QUEUED
EXEC                       -> ['OK', WRONGTYPE ..., 'OK']
GET q                      -> 20          （两次写都发生了）
```

**三条里两条成功、第三条失败**，而队列没有被解开。

**所以准确的说法是：** 排队期的错误在任何东西跑之前就被抓到，并让全部作废；执行期的错误是在跑的时候才发现的，它把其余部分留在已提交状态。

**而原因在于队列只校验语法、不校验效果。** `LPUSH` 会不会失败取决于执行那一刻键里装的是什么，而那是排队时无从知道的。

**于是 `WATCH` 成了唯一做条件事务的手段** —— 乐观锁：盯着那些键，若有任何一个变了，`EXEC` 就失败。这里没有在事务内部撤销一次已提交写入的机制。

### 第五部分：访问控制，细到键名与命令

在它历史的大部分时间里，Redis 只有一个口令（`requirepass`），别无其他。从第 6 版起它有了用户，而权限细得不太寻常。实测，一个带键模式与命令清单的用户：

```
ACL SETUSER app on >pw ~session:* +get +set +ping
```

| 以那个用户身份 | 结果 |
|---|---|
| `GET session:1` | 允许（键不存在时返回 `nil`） |
| `GET other:1` | **`NOPERM No permissions to access a key`** |
| `DEL session:1` | **`NOPERM User app has no permissions to run the 'del' command`** |
| `CONFIG GET dir` | **`NOPERM ... 'config\|get'`** |

**三种不同的拒绝：键不在模式里、命令没被授予、整个命令族被关掉。** 而这些权限可以在什么都不改的前提下试出来：

```
ACL DRYRUN app flushall    -> User app has no permissions to run the 'flushall' command
ACL DRYRUN default flushall -> OK
```

**这就让评审的问题变得具体。** 当应用与别的东西共用一个 Redis 实例 —— 一个缓存、一个队列、另一个服务的会话存储 —— **每个消费者一个带键前缀模式、只授予它真正需要的命令的 ACL 用户，就是"一个服务被拿下"与"全部被拿下"之间的差别。** 没有它，`requirepass` 就是一个所有东西共享的秘密，而任何持有它的人都能读每一个键。

### 第六部分：配置在运行中是可写的

一个带刃的运维便利：`CONFIG SET` 不用重启就能改大部分设置。实测，那条要紧的链，以及现在挡住它的那个开关：

| 配置 | `CONFIG SET dir /tmp/target` | 然后 `dbfilename` + `SAVE` |
|---|---|---|
| `enable-protected-configs no`（默认） | **被拒 —— `can't set protected config`** | —— |
| `enable-protected-configs yes` | **`OK`** | **在指定路径写出一个文件** |

在把那个开关打开的情况下，实测结果是一个 122 字节、以 `REDIS0012` 开头的文件 —— RDB 的魔数 —— 写在操作者指定的任何地方，内容里有刚刚写进某个键的值：

```
CONFIG SET dir /tmp/redis-write3        -> OK
CONFIG SET dbfilename planted.rdb       -> OK
SET planted payload-in-the-file
SAVE                                    -> OK
  -> /tmp/redis-write3/planted.rdb，122 字节，那个值就在里面
```

**所以这个原语是：一个可写的配置，加上一条会写文件的命令。** Redis 7 加上了受保护配置这份清单、并在默认情况下把它关掉 —— **这就是为什么这条发现如今是随版本与配置而变的，而不是普遍成立**，也是为什么两半必须放在同一句话里：一个打开了受保护配置的实例，或者一个启动参数能被攻击者影响的实例，就有一个可以从任何能跑 `CONFIG` 的连接够到的写文件原语。

**而 `dir` 不是唯一被保护的那一个，这是有道理的。** 相关的历史是把 `dbfilename` 指向一个会被加载的路径 —— 一个 cron 文件、一个 SSH 公钥、一个会被 web 提供的文件 —— 以及复制那条路：用 `REPLICAOF` 把一个实例指向一个冒充的副本，那个副本的数据就成了主的。两者都是配置变更而不是漏洞利用，而这就是这一部分的要点。

**而制衡在于，同一个接口正是合法运维管理实例的方式。** 控制点是谁能到达 `CONFIG` —— 那是第五部分，也是在 ACL 出现之前 `rename-command` 所做的事。

### 第七部分：脚本

`EVAL` 在服务端跑 Lua。实测，沙箱提供了什么、移除了什么：

| 探针 | 结果 |
|---|---|
| `redis.call("DBSIZE")` | 可用 |
| **`redis.call("CONFIG", "GET", "dir")`** | **`ERR This Redis command is not allowed from script`** |
| `os ~= nil` | `true` |
| **`os.execute ~= nil`** | **`false`** |
| `io`、`package`、`debug` | **`Script attempted to access nonexistent global variable`** |

**两层限制，而它们的性质不同。** 危险的那些 Lua 全局被拿掉了 —— `os.execute` 是 nil，而 `io`、`package`、`debug` 不只是 nil，还被点名为"不存在的全局变量"，所以一段伸手去够它们的脚本会被拒绝，而不是悄悄拿到一个 nil。

**而命令面也被过滤了**：有一批管理类命令在脚本里被拒，`CONFIG` 是其中之一。

**但 `redis.call` 还在，而它以那个连接的权限执行。** 一段脚本能调用调用它的那个连接被允许调用的任何东西 —— 这就是为什么一段由用户输入搭出来的脚本不是"一条带脚本语言的查询"，而是**以那个连接的权限进行的服务端代码执行**。在一个只有一个不受限用户的实例上，那就是一切。

**而模块走得更远。** 实测，`MODULE LIST` 报出一个内建模块；`MODULE LOAD` 接受一个共享对象的路径，并把它加载进服务端进程。那是一个正当的扩展机制，也是从一次写文件开始的那条提权链条的终点。

### 第八部分：从这些机制推出的安全观念

**实测到的默认值是两个保护，而两者都是关于可达性的，不是关于授权的。**

```
requirepass     -> （空）
protected-mode  -> yes
```

**没有设口令，而保护模式在没设口令时拒绝来自回环之外的连接。** 所以一个全新实例不是碰巧开放的 —— 它只有在有人为了让应用服务器够到它而把它绑到更宽的地址、同时留着空口令的时候才开放，而这恰恰就是那些"让它从应用服务器可达"的说明所做的事。

**这就让那两个评审问题与文档型存储那边一模一样：** 它绑在哪个地址上，以及连上去需不需要什么。在地址这个问题上 Redis 比大多数都宽容，因为默认是回环、而且保护模式是开的。

**可配置性是第二个主题。** 实测那条链 —— `CONFIG SET dir`、`CONFIG SET dbfilename`、`SAVE` —— 就是一次写文件，而一次写文件加上一个可加载模块就是执行。**所以 `CONFIG` 与 `EVAL`、`MODULE LOAD` 属于同一类：那些"可达性就是安全边界"的管理接口。** 一个应用连接没有任何理由能调用其中任何一个。

**第三：延迟是一种共享资源，而一个面向用户的功能可以把它花掉。** 实测，`KEYS` 把实例冻了 84 毫秒，而 `SCAN` 从未超过 2 毫秒。一个搜索框、一个管理页、或者一个调试端点，只要它遍历键空间，就是一次名字好听的拒绝服务 —— 而修法是 `SCAN`、或者一个索引、或者干脆不做。

**第四：过期了不等于没有了。** 在主动循环关掉的情况下，`DBSIZE` 把一个 `EXISTS` 报为不存在的键算了进去。内存限额、淘汰策略与 `INFO memory` 指的都是第一个状态，而应用看到的是第二个。

**第五：访问控制就在那里，而且通常没被用上。** 实测的 ACL 拒绝了一个不在模式里的键、一条没被授予的命令、以及整个命令族 —— 而且是按用户。**大多数部署只有一个口令或者根本没有，这意味着每个消费者都能做任何事。** 在实例被共享的地方，带键模式的 ACL 用户是可用的最便宜的隔离。

**第六，关于落盘的数据。** 持久化会把键空间的一份拷贝写到磁盘 —— 默认是 `dump.rdb` —— 而它不加密。它被写在 `dir` 说的那个地方、被任何包含那个目录的备份包含进去、被任何能读那个文件的东西读到。**一个装着会话令牌或个人数据的缓存，在磁盘上有一份明文拷贝，而嵌入式数据库那一篇的道理在这里同样适用：存储删掉了什么、与磁盘上还剩什么，是两个不同的问题。**

### 检测与缓解

- **把绑定地址与"是否需要认证"放在一起查。** 空的 `requirepass` 加非回环的绑定加保护模式，这个组合成立的前提是"保护模式能挡住"。要做到这一点，得信任保护模式确实让实例够不到。
- **对来自应用连接的 `CONFIG SET` 告警**，尤其是 `CONFIG SET dir`、`dbfilename`、`save`、`appendonly`。它们是在运行时改配置，而实测那条写文件的链正好需要其中两条。
- **对 `REPLICAOF`/`SLAVEOF`、`MODULE LOAD`、`EVAL` 告警**（除管理通道外的任何来源）。它们是把"数据存储被拿下"变成"执行"的原语。
- **盯应用代码里的 `KEYS`、`FLUSHALL`、`FLUSHDB` 与大范围 `DEL`。** 它们是可测量的停顿，而代价随键空间而不是随请求增长。
- **追踪 `evicted_keys`、`rejected_connections` 与内存高水位**，因为一个到限额的实例在 `noeviction` 下开始写入失败、在其他策略下开始悄悄丢数据。
- **缓解上，保持默认的回环绑定与保护模式，并对任何必须跨网络的东西要求口令或 ACL 用户。** 默认值本来就是安全配置；要做的是别随手把它撤掉。
- **给每个消费者一个自己的 ACL 用户，带键模式与它需要的命令。** 实测，那拒绝了一个不在模式里的键和一条不在清单里的命令 —— 那是共享口令给不了的隔离。
- **对应用用户禁掉管理类命令** —— `CONFIG`、`EVAL`、`MODULE`、`REPLICAOF`、`FLUSHALL` —— 而不是指望没人去调。
- **保持 `protected-mode` 开着，并考虑别动那份受保护配置清单**，因为实测它就是挡住那条写文件链的东西，而它很少需要被关掉。
- **有意识地设 `maxmemory` 与淘汰策略**，并记住淘汰就是数据丢失：给会话存储选 `allkeys-lru` 意味着压力一大、会话就按设计消失了。
- **把持久化文件当作数据。** 它的权限、它的位置、它是否被备份包含、以及磁盘是否加密，都属于"这份数据住在哪里"的答案。
- **用 `SCAN` 而不是 `KEYS`，并且预期它那两个限制** —— 可能重复、没有一致快照 —— 而不是假定它表现得像一次完整列举。
- **而且不要用键名来搭租户隔离。** 键前缀是一个约定，而第五部分里每一条命令的存在，都是因为约定不是边界。
