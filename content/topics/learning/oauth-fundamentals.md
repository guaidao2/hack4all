---
id: oauth-fundamentals
title_en: "OAuth 2.0 and OIDC, Part 1 — Authorization Is Not Authentication"
title_zh: "OAuth 2.0 与 OIDC（一）：授权不是认证"
summary_en: OAuth answers what a client may do, not who the user is, and most of this class follows from that mix-up. Measured across four redirect_uri validation styles against twelve bypass URLs, plus PKCE shown doing the single thing it does.
summary_zh: OAuth 回答的是"客户端可以做什么"，而不是"用户是谁"，而这一类里的大部分都从那个混淆里来。这一篇量了四种 redirect_uri 校验对着十二个绕过 URL 的表现，也把 PKCE 唯一做的那件事演示了一遍。
tags: [web, oauth, oidc, cwe-601, authentication, pkce]
tools: [curl, python3, Burp Suite]
attck: [T1550, T1528, T1078]
platform: [web]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The mix-up that produces most of this class

OAuth 2.0 is an **authorization** framework. It answers a delegated-access question: may this client do these things on behalf of this user? The token it hands out — the `access_token` — is addressed to a **resource server** and says what the bearer may do.

It is not an authentication protocol. Nothing in the base specification says how to determine **who** the user is, and an `access_token` is not an identity document: it is a bearer token, so whoever holds it can use it, and it is issued to a client rather than to a person. Applications that treat "I have an access token" as "I know who this is" have built their login on a value that was never meant to answer that question — and OIDC exists precisely because that gap needed filling.

The practical consequence, and the reason this entry starts here:

> **Almost every OAuth finding is a place where a mechanism that decides *what may happen* was used to decide *who someone is*.**

### The flow, and who verifies what at each step

Written out because the attacks are all located at one of these steps:

| Step | Who is verifying | What they check |
|---|---|---|
| 1. Client sends the user to `/authorize` | The authorization server | `client_id`, **`redirect_uri`**, `scope`, and with PKCE the `code_challenge` |
| 2. The user consents | The user | What is being granted, in language they can read |
| 3. The browser is redirected back with a `code` | **Nothing** | **There is no verification at this step** |
| 4. The client exchanges the code at `/token` | The authorization server | The code, the client's own identity, and with PKCE the `verifier` |
| 5. The client uses the token | The resource server | That the token is valid and covers the requested scope |

Three observations follow from that table.

**The code is a bearer ticket, not a credential.** It is short-lived and single-use, but it is what buys the credential, and it travels through the **browser** — the one component in the flow that an attacker can influence.

**Step 3 has no verification, so step 1 is doing all the work.** The only thing standing between an authorization code and an attacker is the decision about **where** to send it, made before anything was issued.

**Step 4 is where the client has to prove itself**, and that is what PKCE adds for clients that cannot keep a secret.

### redirect_uri is the whole boundary

The registration that a client sets up names a callback. That value is the client's statement of **where its own code should be delivered**, and the authorization server's job is to send the code there and nowhere else.

Note what this check is **not**: it is not about identity, permissions or consent. It is a check about a **destination**. And a destination cannot be validated by similarity — the same lesson as the CORS, host header and SSRF entries, in a place where the consequence is immediate account takeover.

Measured, with a client registered for `https://app.example/callback`, against twelve URLs an attacker might submit:

| URL | Exact match | Prefix match | Substring match | Host-only match |
|---|---|---|---|---|
| The registered value | allow | allow | allow | allow |
| `/callback/evil` appended | **deny** | allow | allow | allow |
| `/callback/../../evil` | **deny** | allow | allow | allow |
| `/callback?next=https://evil.example` | **deny** | allow | allow | allow |
| `/callback/../../../evil` | **deny** | allow | allow | allow |
| `evil.app.example` subdomain | **deny** | deny | deny | deny |
| `app.example.evil.example` | **deny** | deny | deny | deny |
| port `:8443` | **deny** | deny | deny | deny |
| scheme changed to `http` | **deny** | deny | deny | allow |
| `app.example@evil.example` | **deny** | deny | deny | deny |
| registered value appears elsewhere in the URL | **deny** | deny | allow | deny |

**Only exact matching refused all twelve.** Each of the other three implementations admitted at least four, and every admitted URL controls where the code goes.

Two entries in that table deserve a second look because they show how little the loose checks see:

**`https://evil.example/?x=https://app.example/callback`** — the registered value is *in* the URL, as a query parameter, while the destination is `evil.example`. A substring check reads this as correct. **This is the whole failure of "contains" as a comparison**, and it is the same shape as the host header entry's substring whitelist and the CORS entry's suffix check.

**A scheme change admitted by "same host"** — the code is delivered over plain HTTP, where everything after it is readable on the path. A check that compares only the hostname has decided that transport does not matter, which the code it is delivering disagrees with.

### The code interception chain

Putting it together, this is what a loose match buys:

1. The attacker crafts an authorization URL for a real client, with the callback pointing somewhere they control.
2. A user — already logged in to the identity provider — follows it, possibly from a link in a page the attacker controls, and **consents**, because the consent screen shows a real client asking for plausible access.
3. The authorization server sends the code to the attacker's destination.
4. The attacker exchanges the code at `/token` using the **client's own public identity**, and receives tokens for that user.

**The user's password was never involved, and the attacker never needed to intercept anything** — the code was delivered to them by the authorization server, because it was told to.

### state and PKCE solve different problems

They are frequently mentioned together and they are not substitutes.

**`state` binds the response to the request.** The client sends a random value, and on the callback it checks that the value came back. What it prevents is a **code substitution**: an attacker who can make a victim's browser complete a flow the attacker started — for the attacker's account — would otherwise land the victim logged in as the attacker, with whatever the victim then does happening inside the attacker's account. `state` makes the callback refer to a flow the client actually began.

**PKCE binds the code to the client that requested it.** The client generates a random `verifier`, sends only its hash as the `code_challenge` with the authorization request, and presents the `verifier` when exchanging the code. Measured:

| Who tries to exchange the code | Result |
|---|---|
| An attacker who intercepted the code, without the verifier | **refused** — no verifier |
| An attacker guessing a verifier | **refused** — does not match the challenge |
| The real client, with its verifier | **accepted** |

The mechanism is `code_challenge = base64url(sha256(verifier))`, and the property that makes it work is that **the verifier never travels through the browser** — it goes from the client to the token endpoint directly. So a leaked or intercepted code is not enough to obtain tokens, which is what closes the interception chain above even when `redirect_uri` matching is imperfect.

Both are needed, and for different reasons: **one stops a code being leaked, the other stops a code being substituted.**

### OIDC adds what OAuth was missing

OIDC layers an identity layer on top: an `id_token`, a signed JWT containing claims about the authentication event, alongside the access token. Introducing a signed token adds the checks that the JWT entries already described, in a new setting:

| Claim | The question it answers |
|---|---|
| `iss` | Was this issued by the provider I trust? |
| `aud` | Was this token issued **to this client**? |
| `exp` / `iat` | Is it valid now? |
| `nonce` | Does it correspond to the request this client made? |
| The signature algorithm | **Fixed by configuration**, not taken from the token |

That last row is the algorithm confusion entry, unchanged — and the `nonce` row is the OIDC equivalent of `state` for the identity layer, binding the token to the request rather than to the browser session.

### Detection and mitigation

- **Alert when `redirect_uri` does not exactly equal a registered value.** The expected values are a fixed list per client known at deploy time, so this is a comparison rather than a heuristic — and any near-miss is a probe.
- **Alert on a second exchange of the same authorization code.** Codes are single-use; a second attempt means someone other than the legitimate client has it. This is the strongest available signal of interception, and it is invisible if the server does not log both attempts with the same code identifier.
- **Alert when a code is exchanged from a different address or client than the one that requested it.** The flow has an origin; a change in it is the trace of a forwarded code.
- **And alert on flows that complete without `state` or without a PKCE verifier.** Both indicate a client that will not survive the attacks above, and both are visible server-side.
- **For mitigation, compare the callback exactly, and refuse anything else.** The measured table is the argument: three looser implementations each admitted four or more URL shapes that control the destination. Where multiple callbacks are supported, they are a list of exact strings, each registered deliberately.
- **Require PKCE for every client, not only public ones.** It costs one hash and one extra parameter, and it means an intercepted code is not immediately a token.
- **Check `state` on every callback, and treat its absence as a failure.** The check has to be a comparison against a value the client generated and stored, not a check that the parameter exists.
- **Bind the code to the client that requested it**, so a code obtained through a second client cannot be redeemed.
- **And stop using `access_token` as proof of identity.** Where the application needs to know who the user is, that is the `id_token` and its claims, validated as the JWT entries describe — the fix belongs at the level of what the value is for, not at the level of the check around it.
- **Treat an open redirect on the client as an OAuth vulnerability.** The redirect entry described how a redirect parameter is usually treated as low severity; in this flow it is a second hop that can deliver a code, so the two findings multiply rather than add.

<!-- lang:zh -->
### 那个混淆产出了这一类的大部分

OAuth 2.0 是一个**授权**框架。它回答的是一个"委托访问"的问题：这个客户端可以在代表这个用户的前提下做这些事吗？它发出的 token —— `access_token` —— 是**给资源服务器**的，说的是持有者可以做什么。

它不是认证协议。基础规范里没有任何东西说如何判断用户**是谁**，而 `access_token` 也不是身份文件：它是一个持有者令牌，谁拿着谁能用，而且它是发给一个客户端的，不是发给一个人的。那些把"我拿到了 access token"当成"我知道这是谁"的应用，是把登录建立在一个从未被设计来回答那个问题的值上 —— 而 OIDC 之所以存在，正是因为那个缺口需要被填。

实际的后果，也是这一篇从这里开始的原因：

> **几乎每一条 OAuth 发现，都是"某个决定*可以发生什么*的机制，被用来决定*某个人是谁*"的地方。**

### 流程，以及每一跳谁在验证什么

写出来，因为所有攻击都落在其中某一步上：

| 步骤 | 谁在验证 | 验证什么 |
|---|---|---|
| 1. 客户端把用户送到 `/authorize` | 授权服务器 | `client_id`、**`redirect_uri`**、`scope`，以及用 PKCE 时的 `code_challenge` |
| 2. 用户同意 | 用户 | 被授予的是什么，用他能读懂的话说 |
| 3. 浏览器带着 `code` 被重定向回来 | **没有谁** | **这一步没有任何验证** |
| 4. 客户端在 `/token` 兑换授权码 | 授权服务器 | 授权码、客户端自己的身份，以及用 PKCE 时的 `verifier` |
| 5. 客户端使用 token | 资源服务器 | token 有效，且覆盖了所请求的 scope |

那张表带来三个观察。

**授权码是一张持有者票据，不是凭据。** 它短命、一次性，但它是**买凭据的东西**，而它经过的是**浏览器** —— 流程里唯一一个攻击者能影响的部分。

**第 3 步没有验证，所以第 1 步承担了全部的活。** 挡在一张授权码与攻击者之间的唯一东西，是"发往**哪里**"这个在任何东西被签发之前就做出的判断。

**第 4 步是客户端必须自证的地方**，而那正是 PKCE 为"无法保守秘密的客户端"补上的。

### `redirect_uri` 就是全部的边界

客户端注册时会给一个回调地址。那个值是客户端在陈述**它自己的授权码应当被送到哪里**，而授权服务器的职责是把授权码送到那里、不送到别处。

注意这个校验**不是**什么：它不是关于身份、权限或同意的。它是一次关于**目的地**的校验。而目的地无法通过相似性来验证 —— 这和 CORS、Host 头、SSRF 那几篇是同一条教训，只是在这里后果立刻就是账号接管。

实测：一个注册了 `https://app.example/callback` 的客户端，对着十二个攻击者可能提交的 URL：

| URL | 精确匹配 | 前缀匹配 | 子串匹配 | 只比主机 |
|---|---|---|---|---|
| 注册的那个值 | 放行 | 放行 | 放行 | 放行 |
| 追加 `/callback/evil` | **拒绝** | 放行 | 放行 | 放行 |
| `/callback/../../evil` | **拒绝** | 放行 | 放行 | 放行 |
| `/callback?next=https://evil.example` | **拒绝** | 放行 | 放行 | 放行 |
| `/callback/../../../evil` | **拒绝** | 放行 | 放行 | 放行 |
| `evil.app.example` 子域 | **拒绝** | 拒绝 | 拒绝 | 拒绝 |
| `app.example.evil.example` | **拒绝** | 拒绝 | 拒绝 | 拒绝 |
| 端口 `:8443` | **拒绝** | 拒绝 | 拒绝 | 拒绝 |
| scheme 换成 `http` | **拒绝** | 拒绝 | 拒绝 | 放行 |
| `app.example@evil.example` | **拒绝** | 拒绝 | 拒绝 | 拒绝 |
| 注册值出现在 URL 里别的位置 | **拒绝** | 拒绝 | 放行 | 拒绝 |

**只有精确匹配拒绝了全部十二个。** 另外三种实现各放行了至少四个，而每一个被放行的 URL 都控制着授权码去哪里。

那张表里有两行值得再看一眼，因为它们显示那些宽松的检查看到了多少：

**`https://evil.example/?x=https://app.example/callback`** —— 注册值**在** URL 里，作为一个查询参数，而目的地是 `evil.example`。子串检查读出来是"正确"。**这就是"包含"作为一种比较方式的全部失效**，而它和 Host 头那篇的子串白名单、CORS 那篇的后缀检查是同一个形状。

**一个被"同主机"放行的 scheme 变更** —— 授权码通过明文 HTTP 投递，而它之后的一切在路上都是可读的。一个只比主机名的检查，等于决定了传输方式无关紧要，而被投递的那个授权码不同意这一点。

### 授权码拦截那条链

把这些放在一起，一次宽松匹配换来的是：

1. 攻击者为某个真实客户端构造一条授权 URL，回调指向自己控制的地方。
2. 一个用户 —— 已经登录着身份提供方 —— 跟着它走，可能是从攻击者控制页面里的一个链接，然后**同意**，因为同意页面显示的确实是一个真实客户端在请求一些看起来合理的权限。
3. 授权服务器把授权码送到攻击者的目的地。
4. 攻击者用**客户端自己的公开身份**在 `/token` 兑换这个码，拿到那个用户的 token。

**全程没有用到用户的密码，攻击者也不需要拦截任何东西** —— 授权码是被授权服务器送来的，因为它被告知要送到那里。

### `state` 与 PKCE 解决的是不同问题

它们经常被一起提到，而它们不是互相替代的。

**`state` 把响应绑定到请求上。** 客户端发一个随机值，并在回调时检查这个值回来了。它防的是**授权码替换**：一个能让受害者浏览器完成"攻击者发起的流程"的攻击者，本来会让受害者以攻击者的身份登录，而受害者之后做的一切都发生在攻击者的账号里。`state` 让回调指向一个客户端确实发起过的流程。

**PKCE 把授权码绑定到请求它的那个客户端上。** 客户端生成一个随机 `verifier`，在授权请求里只发送它的哈希作为 `code_challenge`，而在兑换授权码时出示 `verifier`。实测：

| 谁试图兑换这个码 | 结果 |
|---|---|
| 拦截到码、没有 verifier 的攻击者 | **拒绝** —— 没有 verifier |
| 猜一个 verifier 的攻击者 | **拒绝** —— 与 challenge 不匹配 |
| 真正的客户端，带着它的 verifier | **通过** |

机制是 `code_challenge = base64url(sha256(verifier))`，而让它成立的性质是：**verifier 从不经过浏览器** —— 它从客户端直接到 token 端点。所以一个泄露或被拦截的授权码不足以拿到 token，这就在 `redirect_uri` 匹配不完美时也把上面那条拦截链关上了。

两者都需要，原因不同：**一个阻止授权码被泄漏，另一个阻止授权码被替换。**

### OIDC 补上了 OAuth 缺的东西

OIDC 在上面加了一层身份层：一个 `id_token`，一个关于这次认证事件的、带签名的 JWT，与 access token 并列。引入一个签名 token 就引入了 JWT 那两篇已经讲过的那些检查，只是换了个场景：

| 声明 | 它回答的问题 |
|---|---|
| `iss` | 这是不是我信任的那个提供方签发的？ |
| `aud` | 这个 token 是不是签发给**这个客户端**的？ |
| `exp` / `iat` | 现在有效吗？ |
| `nonce` | 它对应这个客户端发出的那次请求吗？ |
| 签名算法 | **由配置固定**，不从 token 里取 |

最后一行就是算法混淆那篇，原样搬过来 —— 而 `nonce` 那一行是 OIDC 里与 `state` 对等的东西，针对身份层，把 token 绑定到请求上而不是绑定到浏览器会话上。

### 检测与缓解

- **当 `redirect_uri` 与某个注册值不完全相等时告警。** 期望值是每个客户端一份部署时就已知的固定清单，所以这是一次比较而不是启发式 —— 而任何近似就是一次探测。
- **对同一个授权码的第二次兑换告警。** 授权码是一次性的；第二次尝试意味着合法客户端之外的人拿到了它。这是现有的最强拦截信号，而如果服务端不用同一个授权码标识把两次尝试都记下来，它就是不可见的。
- **当授权码从与请求它时不同的地址或客户端被兑换时告警。** 这个流程有一个来源；来源变了，就是授权码被转发的痕迹。
- **并且对"没有 `state`、或者没有 PKCE verifier 就完成的流程"告警。** 两者都表明这个客户端撑不过上面那些攻击，而两者在服务端都是可见的。
- **缓解上，精确比较回调地址，其余一律拒绝。** 上面那张实测表就是论据：三种更宽松的实现各放行了四种以上的、能控制目的地的 URL 形状。需要支持多个回调时，它们是一份精确字符串的清单，每一个都被刻意注册过。
- **对每个客户端都要求 PKCE，而不只是公共客户端。** 它只花一次哈希和一个额外参数，而它让一个被拦截的授权码不会立刻变成 token。
- **在每次回调上检查 `state`，并把它的缺失当作失败。** 这个检查必须是与客户端生成并保存的那个值做比较，而不是检查那个参数存在。
- **把授权码绑定到请求它的那个客户端上**，这样通过另一个客户端拿到的码就无法被兑换。
- **并且停止用 `access_token` 当身份证明。** 应用需要知道用户是谁时，那是 `id_token` 及其声明，按 JWT 那两篇描述的方式校验 —— 修法属于"这个值是干什么用的"这个层面，而不是它外面那圈检查。
- **把客户端上的开放重定向当作 OAuth 漏洞。** 重定向那篇讲过，一个重定向参数通常被当作低危；在这个流程里，它是一跳能把授权码送出去的二次跳转，所以两条发现是相乘而不是相加。
