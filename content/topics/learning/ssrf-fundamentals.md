---
id: ssrf-fundamentals
title_en: "SSRF, Part 1 — Addresses Are Not Strings"
title_zh: "SSRF（一）：地址不是字符串"
summary_en: A server-side request forgery borrows the server's network position, and the exploitable gap is always between the component that validated the URL and the client that actually connects. This entry measures that gap with real connections across a dozen spellings of one address.
summary_zh: 服务端请求伪造借用的是服务器的网络位置，而可利用的那条缝，永远在"校验 URL 的组件"与"真正去连的客户端"之间。这一篇用一个地址的十几种拼法、配合真实连接，把那条缝量出来。
tags: [web, ssrf, cwe-918, ip-parsing, url-parsing, cloud-metadata]
tools: [python3, curl, Burp Suite, dig]
attck: [T1190, T1090]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### What is actually being borrowed

In every other entry in this series, an attacker gets code or data to be interpreted the wrong way **in place**. SSRF is different in one respect that shapes everything about it: **the request is made from the server's network position rather than the attacker's.**

That position is usually better than the attacker's. From inside an application server you can reach:

- **Loopback services** bound to `127.0.0.1`, which by design are not exposed.
- **Internal networks** — admin panels, databases, service meshes, other tenants' services.
- **Cloud metadata endpoints** at `169.254.169.254`, which often hand out credentials.
- **The server's own filesystem**, through wrappers some HTTP clients support.

So the vulnerability is not "the application fetched a URL". It is: **the application lent its network position to someone who does not have it.**

And the exploitable part is always the same shape, which is this series' recurring theme:

> **One component decides whether the URL is allowed. A different component decides where the request goes. The bug lives where their readings of the same URL differ.**

That is worth stating before any payload, because it tells you what to test: not "what does the filter block", but **"what does the filter see, and what does the client connect to"**.

### Addresses are not strings

This is the first and most productive gap, and it has nothing to do with URL syntax. An IP address is a 32-bit number for IPv4, and **the resolution function accepts many spellings of it**.

Measured on a current Linux, using `getaddrinfo` — the function that real clients use — and then **actually connecting** to a service bound to `127.0.0.1`:

| Spelling | Meaning | `getaddrinfo` resolves to | Connection result |
|---|---|---|---|
| `127.0.0.1` | Dotted decimal | `127.0.0.1` | **reached the service** |
| `127.1` | Last three parts omitted | `127.0.0.1` | **reached** |
| `127.0.1` | Last two parts omitted | `127.0.0.1` | **reached** |
| `0177.0.0.1` | Octal parts | `127.0.0.1` | **reached** |
| `017700000001` | Octal integer | `127.0.0.1` | **reached** |
| `0x7f.0.0.1` | Hex parts | `127.0.0.1` | **reached** |
| `0x7f000001` | Hex integer | `127.0.0.1` | **reached** |
| `2130706433` | Decimal integer | `127.0.0.1` | **reached** |
| `0` | Just zero | **`0.0.0.0`** | **reached** (on Linux, `0.0.0.0` means this host) |
| `127.0.0.1.` | Trailing dot | `127.0.0.1` | **reached** |
| `[::1]` | IPv6 loopback | `::1` | not reached (the service listened on IPv4 only) |
| `[::ffff:7f00:1]` | IPv4-mapped IPv6, hex | `::ffff:127.0.0.1` | **reached** |
| `[::ffff:127.0.0.1]` | IPv4-mapped IPv6, dotted | `::ffff:127.0.0.1` | **reached** |

Now compare what a **string blacklist** does with each of those, against a list containing `127.0.0.1`, `localhost`, `::1`, `10.`, `192.168.`, `169.254.`:

| Spelling | String blacklist says | Reality, from the table above |
|---|---|---|
| `127.0.0.1` | **rejected** | loopback |
| `127.1` | **allowed** | loopback |
| `127.0.1` | **allowed** | loopback |
| `0177.0.0.1` | **allowed** | loopback |
| `017700000001` | **allowed** | loopback |
| `0x7f.0.0.1` | **allowed** | loopback |
| `0x7f000001` | **allowed** | loopback |
| `2130706433` | **allowed** | loopback |
| `0` | **allowed** | this host |
| `127.0.0.1.` | rejected (substring matches) | loopback |
| `[::1]` | rejected (substring matches) | loopback |

Six spellings of one address pass a blacklist and reach loopback. The reason is not that the list is incomplete — **it is that the list is the wrong kind of thing.** The list enumerates representations; the address has one meaning and unbounded spellings.

### The correct check, and why one step is not enough

The instinct is to resolve the host and judge the resulting address. That is right, and it has a step that is easy to skip:

**`ipaddress.ip_address("127.1")` raises a ValueError.** The module understands *canonical* IP literals, not the operating system's lenient spellings. So a defence written as "parse the host with `ipaddress`, reject if private" fails on `127.1` in the same way the string blacklist does — the check errors out instead of rejecting, or is written to allow on error.

The order that works has two steps:

```python
import ipaddress, socket

def resolved_addresses(host: str):
    """Ask the same resolver the connecting client will use."""
    infos = socket.getaddrinfo(host, None, socket.AF_UNSPEC, socket.SOCK_STREAM)
    return {info[4][0] for info in infos}

def is_forbidden(host: str) -> bool:
    try:
        addrs = resolved_addresses(host)
    except socket.gaierror:
        return True                      # cannot resolve: refuse rather than allow
    for a in addrs:
        ip = ipaddress.ip_address(a)     # here the value IS canonical
        if ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_reserved or ip.is_multicast:
            return True
    return False
```

Three properties of that snippet are the point:

**Resolution happens first, with the same resolver the client uses.** `getaddrinfo` is what turns `127.1` and `2130706433` into `127.0.0.1`, and it is what a defence has to call to see the address the connection will actually use.

**The judgement happens on the resolved value, which is canonical.** `ipaddress` is the right tool — applied to the **output** of resolution, not to the user's string.

**Failure to resolve is a refusal, not an allowance.** A defence that allows when it cannot decide is a defence whose bypass is a domain that does not resolve yet.

And even with all that, one more thing is missing, which is the subject of the next section.

### The gap between checking and connecting

DNS answers can change between the moment a defence resolves a name and the moment a client connects to it. That is **DNS rebinding**, and it is the same failure the upload entry described as check-then-use — with the resource being a name-to-address mapping rather than a file.

```
t0  defence resolves attacker.example -> 203.0.113.9   (public: allowed)
t1  client resolves attacker.example -> 127.0.0.1      (loopback: the request lands inside)
```

The defence's decision was correct when it was made. The request happened later.

The fix is not a better check; it is **not resolving twice**:

- **Resolve once, and connect to the resolved address.** Perform the request against the numeric address, keeping the original hostname in the `Host` header for virtual hosting. The name is never resolved again, so there is no second answer to disagree with the first.
- **Or make the policy independent of DNS**: allow only destinations the application genuinely needs, so a name that resolves anywhere unexpected is out of scope by construction.

There is a same-shape problem with redirects: a defence that validates the **initial** URL and then follows `Location` headers has only checked the first hop. Measured behaviour on a typical client is up to several redirects, and after each one the destination may be somewhere the check never saw. Either validate every hop or do not follow redirects at all.

### The second gap: where the URL says the host is

Address spellings are one disagreement. The other is about **which part of the URL is the host**, and it is best shown by measuring what a correct parser says.

| URL | `urlparse` reports as hostname |
|---|---|
| `http://127.0.0.1/` | `127.0.0.1` |
| `http://attacker.example@127.0.0.1/` | **`127.0.0.1`** |
| `http://127.0.0.1@attacker.example/` | **`attacker.example`** |
| `http://127.0.0.1%2f@attacker.example/` | `attacker.example` |
| `http://127.0.0.1#@attacker.example/` | **`127.0.0.1`** |
| `http://[::ffff:127.0.0.1]/` | `::ffff:127.0.0.1` |

Read the second row carefully: a **correct** URL parser takes `attacker.example` as the *userinfo* and `127.0.0.1` as the *host*. So a defence that uses a real URL parser is not confused by that input at all.

The vulnerability appears when the check is **not** a URL parse but a **substring search** — which is exactly what a challenge described as "the whitelist only does substring matching" looks like:

```
whitelist check:  the URL must contain the string "img.vuln4all.local"
payload:          http://img.vuln4all.local@127.0.0.1:8800/internal-admin/
                      ^^^^^^^^^^^^^^^^^^^^^ the string is present   ^^^^^^^^^ the host that is used
```

The check found its substring. The client connected to loopback. **Neither component was broken** — one was searching text, the other was parsing a URL, and they answered different questions.

The same table shows the useful corollary: **a substring is not confined to the host position.** It can be placed in the path, the query, the fragment or the userinfo:

| Placement | Example |
|---|---|
| Userinfo | `http://allowed.example@127.0.0.1/` |
| Path | `http://127.0.0.1/allowed.example` |
| Query | `http://127.0.0.1/?x=allowed.example` |
| Fragment | `http://127.0.0.1#allowed.example` |

So a check of the form "the URL contains this string" can be satisfied without the request going anywhere near that host, and a check of the form "the hostname equals this string" is only as good as its parser.

### What the borrowed position can reach

| Target | Why it matters |
|---|---|
| **`169.254.169.254`** | Cloud metadata. On many deployments this returns instance credentials, instance identity and user data — the classic SSRF objective |
| **`127.0.0.1` admin interfaces** | Bound to loopback precisely because they are not meant to be reachable |
| **Internal HTTP services** | Service meshes, sidecars, CI, dashboards, Kubernetes APIs |
| **Databases and caches** | Reachable via protocols that carry commands in a line-based format |
| **The server's own files** | Where the client supports `file://` |
| **Ports** | Reachability and timing differences enumerate what is listening |

Two of those deserve emphasis.

**Cloud metadata is the highest-value target** and it is also a link-local address (`169.254.0.0/16`), which means it is catchable by a resolved-address check if that check tests `is_link_local`. Providers have added protections — requiring a token header, for instance — which is a control at the **target** rather than at the caller, and worth deploying precisely because the caller-side check is hard to get right.

**`gopher://` changes the class of the attack.** An HTTP-only client can only speak HTTP; a client that supports `gopher://` can be made to send **arbitrary bytes to an arbitrary port**, which turns SSRF into a way to speak Redis, Memcached, SMTP or any line-based protocol from the server's position. That is why "which schemes does our client support" is a first-order question: it decides whether this is a request-forgery bug or a generic "send bytes anywhere" primitive.

### Detection and mitigation

- **Monitor outbound connections from application servers, and alert on the ones that should not exist.** A web process connecting to loopback, to a private range, to link-local, or to a new destination is the single most reliable signal in the class, because it holds regardless of how the URL was written. The check belongs in the network, not only in the application.
- **Alert on the shapes of the bypass.** A URL containing `@`; a host that is a bare integer, or hex, or octal, or has fewer than four parts; a trailing dot; an IPv4-mapped IPv6 form; `169.254.169.254` in any spelling. Each is individually unusual, and together they are the fingerprint of someone reasoning about a validator.
- **Alert on DNS that answers differently at different times.** The rebinding attack has a signature: a name resolving to one address during validation and another moments later. Sampled DNS logging on the application server makes that visible.
- **Treat redirects as new requests.** If the application follows them, every hop needs validation, and a hop to a private address is a finding. Log the final resolved destination, not just the requested URL.
- **For mitigation, first ask whether the user needs to choose the target at all.** If the feature is "fetch a preview of a link", the honest design is a **fixed allow-list of hosts**, or fetching through a dedicated egress proxy whose own policy is the only one that matters. Where the user must influence the destination, resolve with the client's resolver, judge the canonical address, and **connect to that resolved address** rather than resolving again.
- **Constrain egress at the network layer.** A web process that can only reach the destinations it needs turns a bypassed validator into a failed connection. This is the same "bound the consequence" layer as the upload and deserialisation entries, and here it is unusually cheap: outbound policy for a web tier is a small list.
- **Disable the schemes that are not needed.** `file://`, `gopher://`, `dict://`, `ftp://` and friends are rarely required by a feature that fetches links, and each one widens "forge a request" into "speak another protocol" or "read a file".
- **Protect the metadata endpoint independently.** Requiring a token for metadata access is a target-side control that works even when the caller's validation is wrong — which is exactly the situation to plan for, given how many ways there are to spell an address.

<!-- lang:zh -->
### 真正被借走的是什么

本系列其他每一篇里，攻击者都是让代码或数据**在原地**被错误解释。SSRF 有一点不同，而这一点塑造了它的全部：**请求是从服务器的网络位置发出的，不是从攻击者的位置。**

那个位置通常比攻击者的好。从一台应用服务器内部，你能到：

- **绑定在 `127.0.0.1` 上的回环服务** —— 它们按设计就不对外暴露。
- **内网** —— 管理面板、数据库、服务网格、别的租户的服务。
- **`169.254.169.254` 上的云元数据端点**，它们经常发放凭据。
- **服务器自己的文件系统**，通过某些 HTTP 客户端支持的包装器。

所以这个漏洞不是"应用取了一个 URL"。它是：**应用把它的网络位置借给了一个本来没有它的人。**

而可利用的部分永远是同一个形状，也就是本系列反复出现的那条主线：

> **一个组件决定这个 URL 是否被允许。另一个组件决定请求去哪里。bug 就住在它们对同一个 URL 的读法不一致的地方。**

这值得写在任何 payload 之前，因为它告诉你该测什么：不是"过滤器封了什么"，而是**"过滤器看到什么，而客户端连到哪里"**。

### 地址不是字符串

这是第一个也是最出产的一条缝，而它与 URL 语法毫无关系。IPv4 地址是一个 32 位整数，而**解析函数接受它的很多种拼法**。

在当前 Linux 上实测，用的是真实客户端会用的 `getaddrinfo`，然后**真的去连**一个绑定在 `127.0.0.1` 的服务：

| 拼法 | 含义 | `getaddrinfo` 解析为 | 连接结果 |
|---|---|---|---|
| `127.0.0.1` | 点分十进制 | `127.0.0.1` | **连上了那个服务** |
| `127.1` | 省略后三段 | `127.0.0.1` | **连上了** |
| `127.0.1` | 省略后两段 | `127.0.0.1` | **连上了** |
| `0177.0.0.1` | 八进制各段 | `127.0.0.1` | **连上了** |
| `017700000001` | 八进制整数 | `127.0.0.1` | **连上了** |
| `0x7f.0.0.1` | 十六进制各段 | `127.0.0.1` | **连上了** |
| `0x7f000001` | 十六进制整数 | `127.0.0.1` | **连上了** |
| `2130706433` | 十进制整数 | `127.0.0.1` | **连上了** |
| `0` | 就是个零 | **`0.0.0.0`** | **连上了**（Linux 上 `0.0.0.0` 意为本机） |
| `127.0.0.1.` | 末尾带点 | `127.0.0.1` | **连上了** |
| `[::1]` | IPv6 回环 | `::1` | 没连上（那个服务只听 IPv4） |
| `[::ffff:7f00:1]` | IPv4 映射的 IPv6，十六进制 | `::ffff:127.0.0.1` | **连上了** |
| `[::ffff:127.0.0.1]` | IPv4 映射的 IPv6，点分 | `::ffff:127.0.0.1` | **连上了** |

现在看一份**字符串黑名单**（含 `127.0.0.1`、`localhost`、`::1`、`10.`、`192.168.`、`169.254.`）对上面每一种怎么说：

| 拼法 | 字符串黑名单的判断 | 上面那张表里的现实 |
|---|---|---|
| `127.0.0.1` | **拒绝** | 回环 |
| `127.1` | **放行** | 回环 |
| `127.0.1` | **放行** | 回环 |
| `0177.0.0.1` | **放行** | 回环 |
| `017700000001` | **放行** | 回环 |
| `0x7f.0.0.1` | **放行** | 回环 |
| `0x7f000001` | **放行** | 回环 |
| `2130706433` | **放行** | 回环 |
| `0` | **放行** | 本机 |
| `127.0.0.1.` | 拒绝（子串匹配上了） | 回环 |
| `[::1]` | 拒绝（子串匹配上了） | 回环 |

**同一个地址的六种拼法穿过一份黑名单、到达回环。** 原因不是这份清单不全 —— **而是这份清单是错的东西。** 清单在枚举"表示"，而地址只有一个含义、却有无限种拼法。

### 正确的检查，以及为什么一步不够

直觉是"解析 host、判断解析出来的地址"。这是对的，而它有一个容易被跳过的步骤：

**`ipaddress.ip_address("127.1")` 会抛 ValueError。** 这个模块懂的是**规范**的 IP 字面量，不是操作系统的宽松拼法。所以一个写成"用 `ipaddress` 解析 host、是私有就拒绝"的防御，会在 `127.1` 上以**和字符串黑名单一样的方式**失效 —— 检查出错了，而不是拒绝；或者被写成"出错就放行"。

能用的顺序是两步：

```python
import ipaddress, socket

def resolved_addresses(host: str):
    """用连接客户端会用的同一个解析器去问"""
    infos = socket.getaddrinfo(host, None, socket.AF_UNSPEC, socket.SOCK_STREAM)
    return {info[4][0] for info in infos}

def is_forbidden(host: str) -> bool:
    try:
        addrs = resolved_addresses(host)
    except socket.gaierror:
        return True                      # 解析不了就拒绝，而不是放行
    for a in addrs:
        ip = ipaddress.ip_address(a)     # 到这里它才是规范的
        if ip.is_private or ip.is_loopback or ip.is_link_local or ip.is_reserved or ip.is_multicast:
            return True
    return False
```

那段代码里有三点就是全部要点：

**解析先发生，而且用的是客户端会用的同一个解析器。** 正是 `getaddrinfo` 把 `127.1` 和 `2130706433` 变成 `127.0.0.1`，而一项防御必须调用它，才能看到连接实际会用的那个地址。

**判断发生在解析后的值上，而那是规范的。** `ipaddress` 是对的工具 —— 用在**解析的输出**上，不是用在用户的字符串上。

**解析失败是拒绝，不是放行。** 一项"判断不了就放行"的防御，它的绕过就是一个"现在还解析不出来"的域名。

而即使做到这些，还差一件事，那是下一节的主题。

### 检查与连接之间的那道缝

DNS 答案可能在"防御解析域名的那一刻"与"客户端连接它的那一刻"之间改变。这就是 **DNS 重绑定**，而它和上传那篇描述的 check-then-use 是同一种失效 —— 只不过被争夺的资源是**名字到地址的映射**，不是一个文件。

```
t0  防御解析 attacker.example -> 203.0.113.9   （公网：放行）
t1  客户端解析 attacker.example -> 127.0.0.1   （回环：请求落到了内部）
```

那个决定在做出的那一刻是正确的。请求发生在之后。

修法不是"更好的检查"，而是**不要解析两次**：

- **只解析一次，并连到那个解析出来的地址。** 对数字地址发起请求，把原主机名放在 `Host` 头里以支持虚拟主机。这个名字不会再被解析，所以没有第二个答案能跟第一个不一致。
- **或者让策略与 DNS 无关**：只允许应用确实需要的目标，于是一个"解析到任何意外地址"的名字从结构上就在范围之外。

重定向有同一个形状的问题：一项只校验**初始** URL、随后跟随 `Location` 头的防御，只检查了第一跳。典型客户端的实测行为是最多跟随若干跳，而每一跳之后目的地都可能落在检查从未见过的地方。要么每一跳都校验，要么根本不跟随重定向。

### 第二条缝：URL 说"主机"在哪里

地址的拼法是其中一种不一致。另一种是关于**URL 的哪一段是主机**，而最好的展示方式是量出一个正确解析器说了什么。

| URL | `urlparse` 报告的主机名 |
|---|---|
| `http://127.0.0.1/` | `127.0.0.1` |
| `http://attacker.example@127.0.0.1/` | **`127.0.0.1`** |
| `http://127.0.0.1@attacker.example/` | **`attacker.example`** |
| `http://127.0.0.1%2f@attacker.example/` | `attacker.example` |
| `http://127.0.0.1#@attacker.example/` | **`127.0.0.1`** |
| `http://[::ffff:127.0.0.1]/` | `::ffff:127.0.0.1` |

仔细读第二行：一个**正确的** URL 解析器把 `attacker.example` 当作 *userinfo*、把 `127.0.0.1` 当作 *host*。所以一个用真正 URL 解析器的防御，根本不会被那个输入搞糊涂。

漏洞出现在"检查**不是**一次 URL 解析、而是一次**子串搜索**"的时候 —— 而一道被描述为"白名单只做子串匹配"的题，长的正是这样：

```
白名单检查:  URL 里必须出现字符串 "img.vuln4all.local"
payload:     http://img.vuln4all.local@127.0.0.1:8800/internal-admin/
                 ^^^^^^^^^^^^^^^^^^^^^ 字符串确实在    ^^^^^^^^^ 而用的是这个主机
```

检查找到了它的子串。而客户端连到了回环。**两个组件都没有坏** —— 一个在搜文本，另一个在解析 URL，它们回答的是不同的问题。

同一张表还给出一个有用的推论：**子串并不局限在主机那个位置。** 它可以被放在路径、查询、片段或 userinfo 里：

| 位置 | 例子 |
|---|---|
| userinfo | `http://allowed.example@127.0.0.1/` |
| 路径 | `http://127.0.0.1/allowed.example` |
| 查询 | `http://127.0.0.1/?x=allowed.example` |
| 片段 | `http://127.0.0.1#allowed.example` |

所以"URL 里含这个字符串"形式的检查，可以在请求根本不去那个主机的情况下被满足；而"主机名等于这个字符串"形式的检查，只和它的解析器一样好。

### 借来的位置能够到什么

| 目标 | 为什么重要 |
|---|---|
| **`169.254.169.254`** | 云元数据。在很多部署里它返回实例凭据、实例身份与用户数据 —— SSRF 的经典目标 |
| **`127.0.0.1` 上的管理界面** | 绑在回环上，恰恰是因为它们不该被够到 |
| **内部 HTTP 服务** | 服务网格、sidecar、CI、看板、Kubernetes API |
| **数据库与缓存** | 通过那些"用行式格式携带命令"的协议可达 |
| **服务器自己的文件** | 在客户端支持 `file://` 的地方 |
| **端口** | 可达性与时间差异能枚举出在听什么 |

其中两个值得强调。

**云元数据是价值最高的目标**，而它同时也是一个链路本地地址（`169.254.0.0/16`），这意味着如果那项解析后检查会测 `is_link_local`，它是能被拦住的。各家云厂商加了防护 —— 例如要求一个令牌头 —— 而那是**目标侧**而不是调用侧的控制，它值得部署，恰恰因为调用侧的检查很难写对。

**`gopher://` 会改变这一类的性质。** 一个只会说 HTTP 的客户端只能说 HTTP；而一个支持 `gopher://` 的客户端**可以被指使把任意字节发到任意端口**，这就把 SSRF 从"伪造一个请求"变成了"从服务器的位置说 Redis、Memcached、SMTP 或任何行式协议"。这就是为什么"我们的客户端支持哪些协议"是一个一等问题：它决定这是个请求伪造 bug，还是一个通用的"往任何地方发字节"原语。

### 检测与缓解

- **监控从应用服务器发出的出站连接，并对那些不该存在的告警。** 一个 Web 进程连到回环、连到私有网段、连到链路本地，或者连到一个新目的地 —— 这是这一类里最可靠的单一信号，因为**无论 URL 是怎么写的它都成立**。这项检查属于网络层，而不只属于应用。
- **对绕过的形状告警。** 含 `@` 的 URL；一个纯整数、或十六进制、或八进制、或段数少于四的 host；末尾的点；IPv4 映射的 IPv6 形式；任何拼法的 `169.254.169.254`。每一条单独看都不寻常，合起来就是"有人在推理一个校验器"的指纹。
- **对"不同时间给不同答案"的 DNS 告警。** 重绑定攻击有签名：一个名字在校验时解析到一个地址、片刻之后解析到另一个。在应用服务器上做抽样 DNS 日志能让它现形。
- **把重定向当成新的请求。** 如果应用跟随它们，每一跳都需要校验，而某一跳指向私有地址就是一条发现。记录最终解析到的目的地，而不只是被请求的 URL。
- **缓解上，先问"用户到底需不需要选择目标"。** 如果这个功能是"抓一个链接的预览"，诚实的设计是**一份固定的主机允许清单**，或者通过一个专门的出网代理去抓、而那个代理自己的策略才是唯一作数的策略。在用户必须影响目的地的地方：用客户端的解析器解析、判断规范地址、并且**连到那个解析出来的地址**而不是再解析一次。
- **在网络层约束出网。** 一个只能到达它需要的那些目的地的 Web 进程，会把一次被绕过的校验变成一次失败的连接。这和上传、反序列化那两篇是同一个"限制后果"层次，而这里它格外便宜：一个 Web 层的出站策略就是一份短名单。
- **关掉不需要的协议。** `file://`、`gopher://`、`dict://`、`ftp://` 之类，对一个"抓链接"的功能很少是必需的，而每一个都会把"伪造一个请求"拓宽成"说另一种协议"或"读一个文件"。
- **独立地保护元数据端点。** 要求元数据访问带一个令牌，是一项**目标侧**控制，即使调用方的校验写错了也管用 —— 而考虑到一个地址有多少种拼法，那正是该为之做打算的情形。
