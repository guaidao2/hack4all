---
id: idor-broken-object-authorization
title_en: IDOR and Broken Object-Level Authorization
title_zh: IDOR 与对象级授权缺陷
summary_en: The most common serious bug in web applications is not exotic. The endpoint knows exactly who you are, and then never checks whether the object you asked for belongs to you. Systematic enumeration finds these faster than any scanner.
summary_zh: Web 应用里最常见的高危漏洞一点都不复杂：接口清楚地知道你是谁，却从不检查你请求的对象是否属于你。系统化枚举比任何扫描器都更快找到它们。
tags: [web, idor, bola, authorization, access-control, bugbounty, api, 越权, 未授权访问, 水平越权]
tools: [Burp Suite, Autorize, ffuf, curl]
attck: [T1190]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Three names for related bugs

- **IDOR** (insecure direct object reference) is the classic form: a parameter names an object — `?invoice_id=1001` — and changing it hands you somebody else's record.
- **BOLA** (broken object-level authorization) is the API-era name for the same thing, and it is the number one finding in API bug bounty programmes.
- **BFLA** (broken function-level authorization) is the sibling bug one level up: not "another user's object", but "a function your role should not be able to call at all" — `POST /admin/users`.

They share a cause. Authentication answers *who are you*; authorization answers *may you touch this*. Frameworks handle the first automatically and leave the second to application code, which is exactly where it gets forgotten — especially on the endpoint added last sprint, or the one the mobile app uses.

### Step 1 — Two accounts, always

Every serious test of this class starts with two accounts at different privilege levels:

- **A** — the attacker: a normal user, ideally the lowest role that can still log in.
- **B** — the victim: a second normal user, so you can create objects as B and try to read them as A.

A third account in an admin or support role is useful for BFLA. Keep both sessions in Burp or in two browser profiles, and make sure you know which cookie belongs to which.

The pattern for every finding is the same sentence: *as A, I could read/modify object X that belongs to B*. If you cannot say that sentence with two accounts, you have not proven anything.

### Step 2 — Map every object reference

Look everywhere an object is named, not just the obvious path segment:

| Location | Example |
|---|---|
| Path segment | `/api/v1/users/1042/invoices` |
| Query parameter | `?account_id=88&file=report.pdf` |
| JSON body | `{"user_id": 1042, "role": "user"}` |
| Header | `X-Account-Id: 88` |
| Cookie | `tenant=acme` |
| GraphQL argument | `node(id: "VXNlcjoxMDQy")` |
| Indirect: filename | `/exports/2026-10-08-user-1042.csv` |

Then classify the identifier, because predictability decides how far you can go: sequential integers are the easiest (increment by one), UUIDv1 embeds a timestamp and MAC (predictable), and UUIDv4 is not. Base64 is not obfuscation — decode it, change the number, re-encode. Hashed IDs are only interesting if the input space is small.

### Step 3 — The eight shapes worth trying

| Shape | Test |
|---|---|
| Sequential swap | Change the id to a neighbouring value, and to one from your other account. |
| Nested resources | `/users/me/invoices/1001` — the parent says "me", the child is not checked. |
| Batch endpoints | `POST /api/users` with `{"ids":[1,2,3]}` often lacks the per-object check the single-item route has. |
| Export and report endpoints | CSV/PDF exports are written for speed and frequently read across tenants. |
| Legacy or mobile APIs | `/api/v1/` is patched; `/api/mobile/` or the older route is not. |
| GraphQL | `node(id:)` and `edges { node }` bypass per-resolver authorization. |
| Chained references | The object id is fine, but a nested field (`owner_id`) inside a write request is trusted. |
| Indirect references | Attachment ids, filename parameters, `download?file=` — same bug, no numeric id. |

### Step 4 — When the obvious swap fails

An endpoint that rejects `id=1002` may still be broken. Things to try, in order:

- **Change the method.** `GET` is checked, `PUT`/`PATCH`/`DELETE` on the same path is not.
- **Duplicate the parameter.** `?id=1001&id=1002` — the validator reads one and the handler reads the other. This is parameter pollution, and it also applies in JSON bodies with repeated keys.
- **Wrap it in an array.** `{"id": 1002}` rejected, `{"id": [1002]}` accepted by a handler that iterates.
- **Move it somewhere else.** If the path id is validated, send the same value in the body, or add `user_id` alongside.
- **Reach the object through another route.** The web UI refuses; the mobile endpoint, the export job, or the older API version does not.
- **Change the case or encoding.** `Id` vs `id`, `%31%30%30%32`, HTTP/2 pseudo-header tricks — usually not the bug itself, but occasionally the difference between a filter and a handler.
- **Try the "me" alias.** `users/{your-id}` works; so does `users/me` sometimes pointing at a different lookup path with a different (missing) check.

### Step 5 — Prove impact, not status codes

A 200 does not mean you won. The proof is the response body containing data that belongs to B, or a state change visible in B's session.

Rank what you find, roughly:

1. **Read other users' PII or documents** — serious, reportable.
2. **Modify another user's data** (email, MFA settings, API keys) — worse, because it is an account-takeover primitive.
3. **Access another tenant's data** — critical; it is the bug that ends a company's day.
4. **Perform a privileged action** (refund, role change, coupon) — critical when money or privilege is involved.

Include in the report exactly how you created the object as B and retrieved it as A, with the two request/response pairs. That is the difference between "there is an IDOR here" and a payout.

### Automating the boring part

Autorize (Burp extension) replays every request you make with a second, lower-privileged session and flags the ones that still succeed. Set it up with A's session, browse the whole application as B, and read the table it produces. It will not find blind cases or chained ones, but it removes the tedium of doing it by hand.

For enumeration of unknown ids, `ffuf` with a numeric wordlist against the endpoint, with the response length filter, is often enough:

```bash
ffuf -u 'https://target/api/v1/invoices/FUZZ' -H "Cookie: session=A_SESSION" \
     -w ids.txt -mc 200 -fr 'not found'
```

### When there is no visible output

Blind IDOR exists: the endpoint acts on an object you cannot read. Prove it indirectly with timing, response length, or a side effect you can observe in B's account:

1. Trigger the action with B's object id as A.
2. Ask B (your second session) to look at what changed — a setting, a counter, an email, an order state.
3. That observable change is the proof.

### Detection

- **An IDOR is silent by design.** A request that should have been denied returns an ordinary `200`, so nothing in the response is anomalous and nothing in a default access log says that an object was fetched by the wrong person.
- **Log the identity and the object together**: `subject`, `object_id`, `owner`, `decision`. An access log without the object cannot answer the only question that matters, which is whether A read B's row.
- **Alert on the pattern rather than on a single request**: one session touching many object ids in sequence, a walk through `1001, 1002, 1003`, or an account reading objects it never created.
- **Cross-tenant access is a hard violation** with no legitimate case, and it deserves an immediate alert rather than a line in a weekly report.
- **Watch the bulk endpoints** — exports, reports, search — because they leak the most data and their authorization is the most often forgotten.
- **Instrument the authorization decision itself**, so that a handler touching an object without going through the check is an event. That turns "we think every endpoint checks" into something observable.
- **Test with two accounts in CI** and alert when that test fails, which is the only way to notice a regression in a check nobody looks at.

### Mitigation

- **Check authorization on the object, in the handler, every time.** Not in the router, not in a middleware that only knows the URL shape, and not only in the UI.
- **Never treat an unpredictable ID as authorization.** UUIDs make guessing harder and fix nothing.
- **Centralise the check but still apply it per object**, so a new endpoint cannot forget it by omission.
- **Scope every query by the current tenant and user** in the data layer — a query that cannot return another user's row is a query that cannot leak one.
- **Test with two accounts in CI**, at least for the endpoints the mobile app and the exports use.

<!-- lang:zh -->
### 同一类问题的三个名字

- **IDOR**（不安全的直接对象引用）是经典形态：某个参数直接指向一个对象 —— `?invoice_id=1001` —— 改一下就拿别人的记录。
- **BOLA**（对象级授权缺陷）是 API 时代的叫法，也是 API bug bounty 项目里的头号发现类型。
- **BFLA**（功能级授权缺陷）是它上一层楼的兄弟：不是「别人的对象」，而是「你这个角色根本不该能调用的功能」—— `POST /admin/users`。

它们的成因一致。认证回答的是**你是谁**，授权回答的是**你能不能碰这个**。框架自动处理前者，把后者留给业务代码 —— 而业务代码恰恰最容易漏，尤其是上个迭代刚加的那个接口，或者移动端在用的那个。

### 第一步 —— 永远先准备两个账号

这一类漏洞的严肃测试都从两个不同权限的账号开始：

- **A** —— 攻击者：一个普通用户，最好是最低权限但还能登录的角色。
- **B** —— 受害者：另一个普通用户，这样你能用 B 创建对象，再试着用 A 去读。

再准备一个管理员或客服角色用于测 BFLA。把两个会话分别放在 Burp 和两个浏览器 profile 里，并且明确知道哪个 cookie 是谁的。

每个发现的表述都是同一句话：**以 A 的身份，我能读到/修改属于 B 的对象 X**。如果你不凑齐两个账号就说不出这句话，那你还没证明任何东西。

### 第二步 —— 把所有对象引用都找出来

不要只看路径段，对象可以出现在任何地方：

| 位置 | 例子 |
|---|---|
| 路径段 | `/api/v1/users/1042/invoices` |
| 查询参数 | `?account_id=88&file=report.pdf` |
| JSON 正文 | `{"user_id": 1042, "role": "user"}` |
| 请求头 | `X-Account-Id: 88` |
| Cookie | `tenant=acme` |
| GraphQL 参数 | `node(id: "VXNlcjoxMDQy")` |
| 间接：文件名 | `/exports/2026-10-08-user-1042.csv` |

然后判断标识符的类型，因为可预测性决定你能走多远：自增整数最容易（加一就行），UUIDv1 里嵌了时间戳和 MAC（可预测），UUIDv4 不行。base64 不是混淆 —— 解开、改数字、再编码回去。哈希 ID 只有在输入空间很小时才值得一看。

### 第三步 —— 八种值得一试的形态

| 形态 | 测法 |
|---|---|
| 顺序替换 | 把 id 换成相邻值，以及另一个账号的 id。 |
| 嵌套资源 | `/users/me/invoices/1001` —— 父级写了「me」，子级却没检查。 |
| 批量接口 | `POST /api/users` 带 `{"ids":[1,2,3]}`，往往缺少单条接口有的逐个检查。 |
| 导出与报表 | CSV/PDF 导出为了性能写得很随意，经常跨租户读取。 |
| 旧版或移动端 API | `/api/v1/` 修过了，`/api/mobile/` 或更老的路由没有。 |
| GraphQL | `node(id:)` 和 `edges { node }` 会绕过各个 resolver 里的授权。 |
| 链式引用 | 对象 id 没问题，但写请求里嵌套的字段（`owner_id`）被直接信任。 |
| 间接引用 | 附件 id、文件名参数、`download?file=` —— 同一个 bug，只是没有数字 id。 |

### 第四步 —— 明显替换失败之后

一个接口拒绝了 `id=1002`，不代表它是安全的。按顺序试：

- **换方法。** `GET` 检查了，同一路径上的 `PUT`/`PATCH`/`DELETE` 没有。
- **重复参数。** `?id=1001&id=1002` —— 校验器读一个，业务代码读另一个。这就是参数污染，JSON 里重复键同理。
- **用数组包起来。** `{"id": 1002}` 被拒，`{"id": [1002]}` 被遍历型的处理器接受。
- **把它挪个地方。** 路径里的 id 被校验，就把同一个值放进 body，或者额外加一个 `user_id`。
- **换条路访问同一个对象。** Web 界面拒绝，移动端接口、导出任务、老版本 API 不拒绝。
- **改大小写或编码。** `Id` 与 `id`、`%31%30%30%32`、HTTP/2 伪头 —— 通常本身不是漏洞，但偶尔正好是过滤器与处理器之间的差异。
- **试「me」别名。** `users/{你的id}` 能用，`users/me` 有时走的是另一条查询路径，而那条路径少了一个检查。

### 第五步 —— 证明影响，而不是状态码

返回 200 不等于你赢了。证据是响应体里出现了属于 B 的数据，或者 B 的会话里能看到的状态变更。

发现大致按这个顺序定级：

1. **读到其他用户的 PII 或文档** —— 严重，可报。
2. **改掉别人的数据**（邮箱、MFA 设置、API key）—— 更严重，因为它是账号接管的原语。
3. **跨租户访问数据** —— 致命，这是能让一家公司当天加班的那种 bug。
4. **执行特权操作**（退款、改角色、领券）—— 涉及钱或权限时同样是致命级。

报告里要写清楚你是怎么用 B 创建对象、又是怎么用 A 取回的，附上两组请求/响应。这就是「这里有个 IDOR」和实际拿到赏金的差别。

### 把枯燥的部分自动化

Autorize（Burp 插件）会用第二个低权会话重放你发出的每个请求，并把依然成功的那些标出来。用 A 的会话配置它，然后用 B 的身份把整个应用点一遍，读它生成的表格。它找不到盲打和链式的场景，但能把手工重复劳动省掉。

枚举未知 id 时，用 `ffuf` 加数字字典打接口，配合响应长度过滤通常就够：

```bash
ffuf -u 'https://target/api/v1/invoices/FUZZ' -H "Cookie: session=A_SESSION" \
     -w ids.txt -mc 200 -fr 'not found'
```

### 没有可见输出的时候

盲 IDOR 是存在的：接口对你读不到的对象执行了操作。用时间、响应长度，或者你在 B 账号里能观察到的副作用来间接证明：

1. 以 A 的身份，用 B 的对象 id 触发那个操作。
2. 让 B（你的第二个会话）去看什么变了 —— 某个设置、计数器、邮件、订单状态。
3. 那个可观察的变化就是证据。

### 检测

- **IDOR 天生是安静的。** 本该被拒绝的请求返回一个普通的 `200`，所以响应里没有异常，默认访问日志里也没有任何东西说明"这个对象是被不该看的人取走的"。
- **把身份和对象一起记下来**：`subject`、`object_id`、`owner`、`decision`。不带对象的访问日志回答不了唯一要紧的那个问题 —— A 到底有没有读到 B 的那一行。
- **对模式告警，而不是对单个请求告警**：同一个会话连续触碰大量对象 id、按 `1001, 1002, 1003` 的顺序走一遍、或者一个账号去读它从未创建过的对象。
- **跨租户访问是硬性违规**，没有任何正当场景，值得立即告警，而不是写进周报里的一行。
- **盯住批量接口** —— 导出、报表、搜索 —— 它们泄漏的数据最多，授权也最常被忘掉。
- **对授权决定本身埋点**，让"处理函数碰了对象却没走检查"本身成为一个事件。这把"我们认为每个接口都检查了"变成可观测的东西。
- **在 CI 里用两个账号测**，并对这个测试失败告警 —— 这是发现一个没人看的检查发生回归的唯一办法。

### 缓解

- **在业务处理里对对象做授权检查，每一次都做。** 不是放在路由层，不是放在只知道 URL 形状的中间件里，也不是只靠前端。
- **永远不要把「ID 猜不到」当成授权。** UUID 让猜测变难，但什么问题都没修。
- **把检查集中化，但仍然按对象逐个施加**，这样新接口不会因为遗漏而忘记。
- **在数据层就按当前租户和用户限定每条查询** —— 一条查不出别人数据的查询，就没法泄露别人的数据。
- **CI 里用两个账号跑测试**，至少覆盖移动端和导出任务用的那些接口。
