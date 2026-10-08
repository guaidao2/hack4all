---
id: authentication-flaws
title_en: Authentication Flaws
title_zh: 身份认证缺陷
summary_en: Authentication answers who you are; authorization answers what you may do. This entry is about the first one — the reset flows, MFA steps and session rules that decide whether an attacker can become somebody else without ever guessing their password.
summary_zh: 认证回答"你是谁"，授权回答"你能做什么"。这一篇讲前者 —— 那些决定攻击者能不能不必猜到口令就变成另一个人的重置流程、MFA 步骤与会话规则。
tags: [web, authentication, mfa, session, password-reset, jwt, bugbounty]
tools: [Burp Suite, ffuf, jwt_tool, hashcat]
attck: [T1110, T1078]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Authentication is several features pretending to be one

"Login" is really a set of flows that each have to hold:

1. Credential submission and the rules around failure.
2. Password reset and account recovery.
3. The second factor, and what happens when it is added, removed or skipped.
4. Session issuance, rotation, expiry and destruction.
5. "Remember me", API tokens and any other long-lived credential.

Bugs live in the seams. A login form with a perfect rate limit means nothing if the password reset hands out tokens that are guessable, or if a "remember me" cookie never expires, or if MFA can be skipped by navigating directly to the post-login page.

### Weakness 1 — credential attacks and what makes them work

```bash
# does the endpoint rate-limit at all?
ffuf -u https://target/login -X POST -d 'user=admin&pass=FUZZ' \
     -w passwords.txt -mc 200 -fr 'Invalid credentials'

# does the error message distinguish a real user from a fake one?
curl -s -d 'user=nobody&pass=x' https://target/login
curl -s -d 'user=admin&pass=x' https://target/login
```

What turns a slow guessing attack into a fast one:

- **No rate limit**, or a limit that resets when you change a header (`X-Forwarded-For`), a cookie, or the case of the username.
- **A different error for an unknown user.** "No such user" versus "wrong password" is a username enumeration primitive, and enumeration is the first half of the attack.
- **A timing difference** between the two cases, when the application checks the password hash only for existing users.
- **No lockout or alerting**, which means the attack can be as slow as it needs to be and still succeed eventually.
- **Password spraying** rather than brute force: one common password against many accounts avoids lockouts entirely and is what real intrusions use.

Default credentials deserve their own line: admin/admin, admin/password, and the vendor default for every appliance and CMS in the environment. Check them before anything clever.

### Weakness 2 — password reset, where the real bugs are

Reset flows are written quickly and tested rarely. Look for:

- **Guessable tokens.** Short, numeric, sequential, based on a timestamp, or derived from the user's data. Request two resets for two accounts and compare.
- **Tokens that do not expire**, or that stay valid after use. A link from an old email should not work twice.
- **A token that is not bound to the user.** If the request carries both a token and a username, try the combination of your token with somebody else's username.
- **Host header injection.** If the reset email's link is built from the request's `Host`, send `Host: attacker.example` and the victim's link points at you:

```http
POST /forgot-password HTTP/1.1
Host: attacker.example
Content-Type: application/x-www-form-urlencoded

email=victim@example.com
```

- **User enumeration through the response**, same as login: "we sent an email" versus "no such account".
- **The reset does not invalidate sessions.** After changing a password, every other session should die; if it does not, an attacker who had access keeps it.
- **The reset token appears in a response body** rather than only in the email.

### Weakness 3 — MFA that can be walked around

MFA is usually implemented as a second step in the same session, which creates a specific family of bugs:

- **Response manipulation.** The second step returns a JSON body with a field like `{"mfa": false}` or `{"authenticated": false}` and the client decides what to do. Change it to `true` and see whether the server believes you. If it does, the check is client-side.
- **Forced browsing.** After the first factor, the session is often already authenticated enough to load the post-login page, and the MFA step is only a redirect. Navigate directly.
- **Backup codes** that are short, not rate-limited, or accepted repeatedly.
- **OTP reuse.** A code that stays valid after being used, or that is accepted for a different user.
- **OTP with no expiry and no rate limit**, which reduces six digits to a guessing game.
- **Enrolling MFA does not invalidate other sessions** — an attacker who is already in stays in, and can now enrol their own factor.
- **Disabling MFA requires only the password**, so a compromised password removes the second factor.

```bash
# replace all values except the one under test, and look for successful logins
ffuf -u https://target/mfa -X POST -d 'code=FUZZ&session=...' -w <(seq -w 0 999999) -mc 200 -fr 'Invalid'
```

### Weakness 4 — session management

- **Session fixation.** If the session identifier does not change when the user logs in, an attacker who can plant a session (through a link, a subdomain, or XSS) can ride the victim's login.
- **Sessions that never expire** and cookies without `Secure`, `HttpOnly` or `SameSite`.
- **Logout that clears the cookie client-side only**, leaving the token valid server-side.
- **Predictable session identifiers**: sequential, derived from the username, or generated with a weak PRNG.
- **JWT issues** are their own list: `alg: none` accepted, weak HMAC secret (crack it offline with `hashcat -m 16500`), `kid` or `jku` pointing at attacker-controlled keys, claims trusted without signature verification, and `exp`/`nbf` not checked. `jwt_tool` covers the common probes.
- **Tokens in URLs**, which end up in logs, referrers and browser history.

### Detecting it from the outside

Not every test needs a payload. Useful habits:

- Register two accounts and diff every flow: reset, MFA enrolment, profile change, logout. The difference between them is where the bug is.
- Send the same request twice and compare responses byte for byte; a token that increments or repeats is broken.
- Log out, and replay the old cookie. Then log in again and try the pre-login cookie.
- Change the `Host` header on any request that triggers an email.
- Watch the response codes: a login that returns 200 and a body containing `"success": false` is a different code path from a 401, and often a weaker one.

### Detection, from the defender's side

- **Rate limits per account, per IP and per device**, and alerts on distributed spraying (many accounts, one password) rather than only on volume against one account.
- **Reset token requests** spiking for a single account, or for addresses that do not exist.
- **MFA bypass attempts** show as requests to post-login endpoints from a session that has only completed the first factor — a sequence, not an event, and very learnable.
- **Session identifiers used from two geographies** at once, or reused after logout.
- **Host header values that do not match the site's domains** on any request that generates a link.

### Mitigation

- **MFA everywhere it is possible**, with TOTP or WebAuthn rather than SMS, and the second step enforced **server-side on every request** for sensitive operations rather than as a redirect.
- **Rate limit and alert**, on both login and every step of reset and MFA; prefer per-account limits with exponential backoff over hard lockouts, which are themselves a denial-of-service vector.
- **Identical responses and timings** for unknown users and wrong passwords; do the hashing work either way.
- **Reset tokens**: at least 128 bits of CSPRNG, single use, short expiry, bound to the account, sent only by email, and never built from the `Host` header — use a configured base URL.
- **Rotate the session identifier on login, on privilege change and on password change**, and destroy all other sessions when the password or MFA state changes.
- **Cookies**: `Secure`, `HttpOnly`, `SameSite=Lax` at minimum, with a short idle timeout and an absolute lifetime.
- **Do not roll your own session or token scheme.** Use the framework's, and keep it patched.
- **Log and alert on authentication anomalies**, because credential attacks are noisy long before they succeed.

<!-- lang:zh -->
### 认证是"假装成一个功能"的一堆功能

"登录"其实是一组流程，每一条都得站得住：

1. 凭据提交，以及失败时的规则。
2. 口令重置与账号找回。
3. 第二因素，以及它在被添加、移除、跳过时会发生什么。
4. 会话的签发、轮换、过期与销毁。
5. "记住我"、API token 以及其他长期凭据。

漏洞长在接缝里。一个限流完美的登录表单毫无意义 —— 如果口令重置发出去的 token 可以猜、如果"记住我"的 cookie 永不过期、如果直接访问登录后的页面就能跳过 MFA。

### 弱点一 —— 凭据攻击，以及它为什么能成

```bash
# 这个接口到底有没有限流？
ffuf -u https://target/login -X POST -d 'user=admin&pass=FUZZ' \
     -w passwords.txt -mc 200 -fr 'Invalid credentials'

# 报错信息能不能区分真实用户和不存在用户？
curl -s -d 'user=nobody&pass=x' https://target/login
curl -s -d 'user=admin&pass=x' https://target/login
```

把"慢慢猜"变成"快速猜"的因素：

- **没有限流**，或者换一个 header（`X-Forwarded-For`）、换 cookie、改一下用户名大小写就能重置的限流。
- **不存在的用户报错不同。** "用户不存在"和"口令错误"是用户名枚举原语，而枚举是攻击的前半程。
- **两种情况耗时不同** —— 应用只对已存在的用户做口令哈希校验时就会出现。
- **没有锁定与告警**，攻击可以慢到不被发现，但最终照样成功。
- **口令喷洒（spraying）而非爆破**：一个常见口令打很多账号，完全避开锁定，真实的入侵就是这么干的。

默认口令值得单列：admin/admin、admin/password，以及环境里每台设备和 CMS 的厂商默认口令。在做任何聪明事之前先试它们。

### 弱点二 —— 口令重置，真漏洞都在这里

重置流程写得快、测得少。重点看：

- **可猜的 token。** 太短、纯数字、自增、基于时间戳，或由用户数据派生。给两个账号各请求一次重置，然后对比。
- **token 不过期**，或者用过之后仍然有效。旧邮件里的链接不该能用第二次。
- **token 没有绑定用户。** 如果请求里同时带 token 和用户名，试试你的 token 配别人的用户名。
- **Host 头注入。** 如果重置邮件里的链接是用请求的 `Host` 拼出来的，发一个 `Host: attacker.example`，受害者的链接就指向你：

```http
POST /forgot-password HTTP/1.1
Host: attacker.example
Content-Type: application/x-www-form-urlencoded

email=victim@example.com
```

- **通过响应枚举用户**，和登录一样：`"已发送邮件"` 对比 `"账号不存在"`。
- **重置不会使会话失效。** 改完口令，其他所有会话都该失效；如果不失效，已经进来的攻击者会一直待着。
- **重置 token 出现在响应体里**，而不是只出现在邮件里。

### 弱点三 —— 可以绕过去的 MFA

MFA 通常实现成同一会话里的第二步，于是产生了一类很具体的 bug：

- **响应篡改。** 第二步返回的 JSON 里有 `{"mfa": false}` 或 `{"authenticated": false}` 之类的字段，由客户端决定怎么做。把它改成 `true`，看服务端信不信。信了，说明检查在客户端。
- **强制浏览。** 完成第一因素后，会话往往已经"足够认证"到能加载登录后的页面，MFA 那一步只是个跳转。直接访问目标页面。
- **备份码**太短、无限流、或可以重复使用。
- **OTP 复用。** 用过的码仍然有效，或者能用在别的用户身上。
- **OTP 没有过期时间也没有限流**，六位数字就退化成猜谜。
- **启用 MFA 不会使其他会话失效** —— 已经进来的攻击者继续待着，还能顺手绑上自己的因素。
- **关闭 MFA 只要口令**，那么一个泄漏的口令就把第二因素一起拿走了。

```bash
# 固定其他字段，只变被测字段，找成功登录
ffuf -u https://target/mfa -X POST -d 'code=FUZZ&session=...' -w <(seq -w 0 999999) -mc 200 -fr 'Invalid'
```

### 弱点四 —— 会话管理

- **会话固定。** 如果用户登录时会话标识不变，能植入会话的攻击者（通过链接、子域或 XSS）就能搭上受害者的登录。
- **会话永不过期**，cookie 缺 `Secure`、`HttpOnly` 或 `SameSite`。
- **登出只在客户端清 cookie**，服务端的 token 依然有效。
- **可预测的会话标识**：自增、由用户名派生，或用弱 PRNG 生成。
- **JWT 的问题自成一类**：接受 `alg: none`、HMAC 密钥太弱（可以离线用 `hashcat -m 16500` 破）、`kid` 或 `jku` 指向攻击者控制的密钥、不验签就信任声明、不检查 `exp`/`nbf`。`jwt_tool` 覆盖了常见探针。
- **把 token 放在 URL 里**，于是它进了日志、Referer 和浏览器历史。

### 从外部检测

不是每个测试都需要 payload。几个有用的习惯：

- 注册两个账号，把每条流程做差分：重置、绑定 MFA、改资料、登出。两者之间的差异就是漏洞所在。
- 同一个请求发两次，逐字节比对响应；会自增或重复的 token 就是坏的。
- 登出后用旧 cookie 重放；再登录，试登录前的那个 cookie。
- 对任何会触发邮件的请求，改一下 `Host` 头。
- 盯响应码：返回 200 而响应体里写着 `"success": false` 的登录，走的是另一条代码路径，而且往往更弱。

### 检测（防守方视角）

- **按账号、按 IP、按设备分别限流**，并对分布式喷洒（很多账号、同一个口令）告警，而不只是盯单个账号的请求量。
- **单个账号的重置 token 请求激增**，或者对不存在的地址请求重置。
- **MFA 绕过尝试**表现为"仅完成第一因素的会话去请求登录后的接口" —— 这是一条序列而不是一个事件，而且非常好识别。
- **同一个会话标识同时出现在两个地理位置**，或在登出后被复用。
- 任何会生成链接的请求里，**Host 头与站点域名不符**。

### 缓解

- **能上 MFA 的地方都上**，用 TOTP 或 WebAuthn 而不是短信，并且第二步要**在服务端对每个敏感请求强制校验**，而不是当成一次跳转。
- **限流并告警**，登录、重置、MFA 的每一步都要；优先用按账号的限流加指数退避，而不是硬锁定 —— 硬锁定本身就是一种拒绝服务手段。
- **对"用户不存在"和"口令错误"给出完全一致的响应与耗时**；两种情况下都执行哈希运算。
- **重置 token**：至少 128 位 CSPRNG、一次性、短有效期、绑定账号、只通过邮件发送，且绝不使用 `Host` 头拼接 —— 用配置里的固定基址。
- **在登录、权限变更和改口令时轮换会话标识**，并在口令或 MFA 状态变化时销毁其他所有会话。
- **Cookie**：至少 `Secure`、`HttpOnly`、`SameSite=Lax`，配短的空闲超时和绝对有效期。
- **不要自己发明会话或 token 方案。** 用框架自带的，并保持更新。
- **对认证异常记录并告警**，因为凭据攻击在成功之前很早就已经很吵了。
