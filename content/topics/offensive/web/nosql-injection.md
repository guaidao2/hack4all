---
id: nosql-injection
title_en: NoSQL Injection
title_zh: NoSQL 注入
summary_en: NoSQL databases are not injected with quotes and UNION, they are injected with structure. Sending an object where the application expected a string is enough to turn a login check into a query that matches everything.
summary_zh: NoSQL 数据库不是靠引号和 UNION 注入的，而是靠结构。在应用期待字符串的地方送进去一个对象，就足以把一句登录校验变成匹配所有记录的查询。
tags: [web, nosql, mongodb, injection, authentication-bypass, bugbounty]
tools: [nosqlmap, Burp Suite, mongosh, redis-cli]
attck: [T1190]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why the payloads look nothing like SQL injection

In a relational database you inject into a query string, so the metacharacters are quotes, comments and `UNION`. In a document database you inject into a **structure**, because the query is itself a document:

```javascript
db.users.find({ username: req.body.username, password: req.body.password })
```

If `req.body.password` can be an object instead of a string, you are not injecting text — you are supplying a query operator:

```json
{"username": "admin", "password": {"$ne": "x"}}
```

`{"$ne": "x"}` means "not equal to x", which every real password satisfies. That is a complete authentication bypass, and no quote was involved.

### The operator injection toolkit

MongoDB's query operators are the whole vocabulary:

| Operator | Meaning | Typical use |
|---|---|---|
| `$ne` | not equal | `{"$ne": null}` matches any existing value |
| `$gt` / `$gte` / `$lt` | comparison | `{"$gt": ""}` matches any string |
| `$in` | in a list | `{"$in": ["admin","root"]}` |
| `$regex` | regular expression | Blind extraction, character by character |
| `$exists` | field exists | Enumerate which fields are present |
| `$where` | JavaScript predicate | Server-side JavaScript execution |

The delivery mechanism matters as much as the operator. Many stacks turn query parameters into nested objects for you:

```http
# Express with extended query parsing, or PHP's []
POST /login
username[$ne]=x&password[$ne]=x

# JSON APIs, where the body is the attack surface
POST /api/login
Content-Type: application/json

{"username": {"$ne": null}, "password": {"$ne": null}}
```

Both of those are the same bug reached through different parsers. The PHP case is worth remembering: `?username[$ne]=x` becomes the array `["$ne" => "x"]`, which the driver happily interprets as an operator.

### From authentication bypass to data extraction

A `$ne` bypass proves the bug; a regex extracts what you want. Because `$regex` returns a match or not, it is a boolean oracle and can be walked one character at a time:

```json
{"username": "admin", "password": {"$regex": "^a"}}
{"username": "admin", "password": {"$regex": "^ab"}}
{"username": "admin", "password": {"$regex": "^abc"}}
```

The response difference (a valid session versus a failed login, or a different error) tells you whether the guess was right. `$where` goes further and evaluates JavaScript inside the database:

```javascript
{"$where": "this.password.length > 5"}
{"$where": "function(){ return this.username == 'admin' }"}

// time-based, when nothing else returns a signal
{"$where": "function(){ if(this.password[0]=='a'){ sleep(5000); return true } return false }"}
```

Operators also chain: `$exists` to find field names, `$regex` to read them, `$where` to be creative. The extraction is slower than SQL injection but it is scriptable, and `nosqlmap` automates the common forms.

### Other databases, other shapes

- **CouchDB**: unauthenticated access to `/_all_docs` or `/_utils` exposes everything; temporary views accept JavaScript, which is the `$where` equivalent.
- **Redis**: usually not injected but exposed. An unauthenticated instance is a write primitive (cron jobs, SSH keys, webshell via a web root), and Lua scripts add a sandbox to escape.
- **Elasticsearch**: unauthenticated reads, and older versions allowed dynamic scripting in queries — the same idea as `$where`.
- **Cassandra, DynamoDB**: injection is rarer because queries are not assembled from strings, but an application that builds filter expressions from user input still has an equivalent problem.

### Finding it without source

The behavioural probes are the same as anywhere: send a string where a string is expected, then send an object and see whether the response changes. Useful variants:

- `{"$ne": 1}` versus a normal value — a login that suddenly succeeds is a bypass.
- A duplicate parameter (`username=admin&username=x`) where the framework picks the last one and the validator picks the first.
- A parameter that accepts an array (`id[]=1&id[]=2`) and reaches a query builder unchanged.
- Numbers where strings are expected, and `{"$gt": ""}` where a username is expected.

### Detection

- Request bodies and query strings containing `$`-prefixed keys (`$ne`, `$gt`, `$regex`, `$where`) — no legitimate client sends those to a login endpoint.
- Array syntax in parameters (`param[]=`, `param[0]=`) on endpoints that never expected arrays.
- Queries with a `$where` clause at all: it is rare in application code and expensive, and it is the execution primitive.
- Database logs showing full collection scans from a single client, which is what regex-based extraction looks like.
- Anomalous latency for one parameter — the time-based variant.

### Mitigation

- **Enforce types before the query.** If a field must be a string, reject anything that is not a string; the whole class depends on an object reaching a driver that understands operators. In Node, `typeof x === 'string'`; in PHP, `is_string()`; in a typed language, the deserialiser already does it.
- **Do not pass request bodies straight into queries.** Map the request into an explicit object with known fields — this is the same discipline as parameterised queries in SQL, applied to structure.
- **Reject unknown operators** with an allow-list if you genuinely need to accept query fragments.
- **Disable server-side JavaScript** (`$where`, `mapReduce` with JS) in MongoDB where the application does not need it.
- **Authenticate Redis, CouchDB and Elasticsearch**, bind them to a private interface, and never expose them to the internet — most "NoSQL injection" incidents are really unauthenticated databases.
- **Alert on `$`-prefixed input** at the WAF and in application logs; it is a cheap, high-signal rule.

<!-- lang:zh -->
### 为什么 payload 和 SQL 注入完全不像

关系型数据库里你注入的是一段查询字符串，所以元字符是引号、注释和 `UNION`。文档型数据库里你注入的是**结构**，因为查询本身就是一份文档：

```javascript
db.users.find({ username: req.body.username, password: req.body.password })
```

如果 `req.body.password` 可以是对象而不是字符串，那你注入的就不是文本 —— 你提供的是一个查询操作符：

```json
{"username": "admin", "password": {"$ne": "x"}}
```

`{"$ne": "x"}` 的意思是"不等于 x"，而任何真实口令都满足这个条件。这是一次完整的认证绕过，全程没有一个引号。

### 操作符注入的词汇表

MongoDB 的查询操作符就是全部词汇：

| 操作符 | 含义 | 典型用法 |
|---|---|---|
| `$ne` | 不等于 | `{"$ne": null}` 匹配任何已存在的值 |
| `$gt` / `$gte` / `$lt` | 比较 | `{"$gt": ""}` 匹配任意字符串 |
| `$in` | 属于列表 | `{"$in": ["admin","root"]}` |
| `$regex` | 正则 | 逐字符盲注提取 |
| `$exists` | 字段存在 | 枚举存在哪些字段 |
| `$where` | JavaScript 谓词 | 服务端 JavaScript 执行 |

投递方式与操作符同等重要。很多技术栈会替你把查询参数变成嵌套对象：

```http
# Express 开了 extended 查询解析，或 PHP 的 []
POST /login
username[$ne]=x&password[$ne]=x

# JSON 接口，请求体就是攻击面
POST /api/login
Content-Type: application/json

{"username": {"$ne": null}, "password": {"$ne": null}}
```

这两种是同一个漏洞经由不同解析器抵达。PHP 那种尤其值得记住：`?username[$ne]=x` 会变成数组 `["$ne" => "x"]`，而驱动会老老实实把它当操作符解释。

### 从认证绕过到数据提取

`$ne` 绕过证明了漏洞存在；正则才是提取数据的手段。因为 `$regex` 只返回匹配与否，它就是一个布尔预言机，可以逐字符走：

```json
{"username": "admin", "password": {"$regex": "^a"}}
{"username": "admin", "password": {"$regex": "^ab"}}
{"username": "admin", "password": {"$regex": "^abc"}}
```

响应差异（拿到会话还是登录失败，或者错误不同）就告诉你猜得对不对。`$where` 更进一步，它在数据库内部执行 JavaScript：

```javascript
{"$where": "this.password.length > 5"}
{"$where": "function(){ return this.username == 'admin' }"}

// 时间盲：其他方式都没有信号时
{"$where": "function(){ if(this.password[0]=='a'){ sleep(5000); return true } return false }"}
```

操作符还能串联：`$exists` 找字段名，`$regex` 读内容，`$where` 负责发挥。提取速度比 SQL 注入慢，但可以脚本化，`nosqlmap` 把常见形态都自动化了。

### 其他数据库，其他形态

- **CouchDB**：未授权访问 `/_all_docs` 或 `/_utils` 就等于全部暴露；临时视图接受 JavaScript，这就是它的 `$where`。
- **Redis**：通常不是被注入，而是被暴露。未授权的实例就是一个写入原语（写 cron、写 SSH key、向 Web 根目录写 shell），而 Lua 脚本又给了沙箱一层要逃的东西。
- **Elasticsearch**：未授权读取；老版本还允许在查询里动态执行脚本 —— 与 `$where` 同一个思路。
- **Cassandra、DynamoDB**：注入较少见，因为查询不是从字符串拼出来的；但一个用用户输入拼过滤表达式的应用，仍然有等价的问题。

### 没有源码时怎么找

行为探针到哪都一样：先送一个字符串，再送一个对象，看响应是否变化。几个有用的变体：

- `{"$ne": 1}` 对比正常值 —— 登录突然成功就是绕过。
- 重复参数（`username=admin&username=x`），框架取最后一个而校验器取第一个。
- 接受数组的参数（`id[]=1&id[]=2`），原封不动到了查询构造器。
- 该是字符串的位置送数字，该是用户名的地方送 `{"$gt": ""}`。

### 检测

- 请求体或查询串里出现 `$` 开头的键（`$ne`、`$gt`、`$regex`、`$where`）—— 正常客户端不会往登录接口发这些。
- 参数里的数组语法（`param[]=`、`param[0]=`），出现在从不期待数组的接口上。
- 查询里出现 `$where`：应用代码里很少用，代价还高，而且它就是执行原语。
- 数据库日志里单个客户端触发全表扫描 —— 基于正则的提取就长这样。
- 某一个参数的耗时异常 —— 时间盲变体。

### 缓解

- **在查询之前强制类型。** 一个字段必须是字符串，就拒绝任何不是字符串的东西；整个漏洞类别都依赖"对象抵达了懂操作符的驱动"。Node 里用 `typeof x === 'string'`，PHP 里用 `is_string()`，强类型语言里反序列化那一步已经做了。
- **不要把请求体直接交给查询。** 把请求映射成一个字段明确的显式对象 —— 这与 SQL 里的参数化查询是同一条纪律，只是作用在结构上。
- 如果确实需要接受查询片段，**用白名单拒绝未知操作符**。
- 在应用不需要时，**关掉 MongoDB 的服务端 JavaScript**（`$where`、带 JS 的 `mapReduce`）。
- **给 Redis、CouchDB、Elasticsearch 加上认证**，绑定到内网接口，绝不暴露到公网 —— 大多数"NoSQL 注入"事件其实是数据库未授权。
- **对 `$` 开头的输入告警**（WAF 与应用日志），这是一条便宜且高命中的规则。
