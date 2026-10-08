---
id: clickjacking-fundamentals
title_en: "Clickjacking — Faking the One Thing CSRF Cannot"
title_zh: "点击劫持：伪造 CSRF 伪造不了的那件事"
summary_en: CSRF forges a request the user never intended; clickjacking forges the intention itself, by putting an invisible copy of the target under a button the user does want to press. Measured in a browser across five framing policies, including one that looks like protection and is ignored.
summary_zh: CSRF 伪造的是一个用户从未打算发出的请求；点击劫持伪造的是意图本身 —— 把目标的一份看不见的副本，放在一个用户确实想按的按钮下面。这一篇在浏览器里跨五种框住策略实测，其中包括一种看起来是防护、实际被忽略的写法。
tags: [web, clickjacking, cwe-1021, ui-redressing, browser]
tools: [curl, chrome, python3]
attck: [T1185, T1204]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The other half of the intent problem

The CSRF entry described a request that arrives with valid credentials the user never meant to send. This entry describes the mirror image, and it is worth stating as a pair:

| | CSRF | Clickjacking |
|---|---|---|
| What the user did | nothing | **clicked, deliberately** |
| What is forged | **the request** | **what they thought they were clicking** |
| Who acts | the browser, automatically | **the user, by hand** |
| What stops it | a token proving intent | **refusing to be framed** |

> **CSRF exploits the absence of an intention. Clickjacking manufactures one.**

That is why the two have different fixes and why a CSRF token does nothing here: the request carries a token, the session is real, and the user pressed the button. The only thing wrong is **which document they were looking at when they pressed it**.

### The construction

The technique is a page of the attacker's with the target embedded in an iframe, made invisible, and positioned so that a control in the frame sits exactly under a control the user wants to press:

```css
iframe {
  position: absolute; inset: 0;
  width: 100%; height: 100%;
  opacity: 0;          /* invisible, not absent */
  border: 0;
  z-index: 2;          /* above the decoy */
}
```

Measured in a browser, against a page whose only content is a confirmation button labelled "confirm transfer", with a decoy reading "congratulations, you won an iPhone" and a green "claim it here" button underneath. The rendered page is, visually, **an ordinary prize page** — nothing about it suggests a second document is present. The alignment is the only technical requirement: the target's button has to land where the decoy's does, which is a matter of offsets and, where necessary, scrolling the frame.

### Four layers

1. **The trust boundary.** The application trusts that a person pressing its button knows what they are pressing. The page's **appearance** belongs to whoever frames it, and everything the user knows about the situation comes from that appearance.
2. **Data and instruction share a plane.** One screen region is simultaneously **the decoy the user sees** and **the control that receives the click**. The visual layer and the behavioural layer are separated, and only the first one is the attacker's to control — which is exactly why the deception is undetectable by looking.
3. **Why the usual fix fails.** JavaScript-based frame busting is unreliable for two independent reasons, and the first one was measured: **a blocked frame still fires `onload`**, so a script cannot reliably tell whether it was framed. The second is that the `sandbox` attribute lets the attacker disable the scripts that would do the busting while still rendering the page. A check that runs in the framed document cannot override a decision the browser makes about framing.
4. **The variants.** A transparent overlay as above; drag-and-drop variants where the drop target is inside the frame; a paste or keyboard-driven variant; and — the practical one — framing a **subdomain or any origin the attacker can publish content on**, which defeats a policy that only excludes the attacker's own origin.

### Measured: five framing policies in a real browser

The same victim page, served with five different response headers, framed by the same attacker page:

| Victim page headers | What the browser did |
|---|---|
| *(none)* | **framed successfully** — the page rendered inside the overlay |
| `X-Frame-Options: DENY` | **refused** — `Refused to display ... because it set 'X-Frame-Options' to 'deny'` |
| `Content-Security-Policy: frame-ancestors 'none'` | **refused** — `Framing ... violates the following Content Security Policy directive` |
| **`X-Frame-Options: ALLOW-FROM https://trusted.example`** | **ignored** — `'ALLOW-FROM ...' is not a recognized directive. The header will be ignored.` |
| `X-Frame-Options: DENY, SAMEORIGIN` (two values in one header) | **refused** — `multiple 'X-Frame-Options' headers with conflicting values ... Falling back to 'deny'` |

Three things follow, and the third is the one worth remembering.

**`X-Frame-Options` and `frame-ancestors` both work**, and they are the two controls that act where the decision is made — in the browser's framing logic rather than in page script.

**`frame-ancestors` expresses more.** `X-Frame-Options` offers `DENY` and `SAMEORIGIN` and nothing else, while `frame-ancestors` takes a list of origins, which is what a site that genuinely needs to be framed by a partner requires. Sending both is the usual arrangement: newer browsers honour the CSP directive and older ones fall back to the header.

**`ALLOW-FROM` looks like a policy and is not one.** The browser reported it as an unrecognised directive and ignored the header entirely, which means **that page was as unprotected as one with no header at all** — while its configuration contained something that reads like a restriction. This is the single most valuable row to check in a real deployment, because it is invisible without looking at what the browser actually did.

And the third row of that last column is worth noting in the other direction: a malformed multi-value header caused the browser to **fall back to `deny`**, so that particular mistake fails closed. Not every malformed security header does.

### Why framing a subdomain matters

A policy that denies framing by *other* origins still permits framing by the site itself, and "the site itself" includes every subdomain under a shared registrable domain unless the directive says otherwise. So the question is not only "can an attacker frame us" but:

> **Is there any origin the attacker can put content on that our policy allows to frame us?**

A subdomain used for user content, a staging host, a marketing site on a shared domain, or any origin with an XSS — each is a place the attacker can publish the framing page, which is why `frame-ancestors 'none'` is the safer default and a list of specific origins is the safer form of the exception.

### Detection and mitigation

- **Alert on requests whose `Sec-Fetch-Dest` is `iframe` for pages that should never be framed.** Modern browsers send this fetch metadata header, so the check is a value comparison rather than a heuristic — and it holds for a framing attempt that the CSP then blocks, which means it detects the attempt rather than the outcome.
- **Alert when a sensitive request arrives with a `Referer` from another origin.** A state change submitted from a page whose referrer is somewhere else is either a legitimate integration or a framed click.
- **And treat a sensitive request with no preceding page view in the session as worth a look.** The user's path to a confirmation button normally includes loading the page it is on; a click with no such load is the shape of a framed interaction.
- **For mitigation, send `frame-ancestors` and `X-Frame-Options` together, from one place.** A single middleware or server-level default that applies to every response, rather than per-page headers — because the class of failure here is a page somebody forgot, and a forgotten page is indistinguishable from an unprotected one.
- **Never use `ALLOW-FROM`.** It is not recognised, so it silently grants what it appears to restrict. Where a site genuinely needs to be framed, list the origins in `frame-ancestors`.
- **Default to `'none'` and treat every exception as a decision.** The list of origins allowed to frame the site should be short, deliberate, and reviewed — and each entry should be an origin the site would be comfortable seeing publish arbitrary content about it.
- **Re-authenticate for the actions that matter.** The same control the CSRF entry ends on applies here for the same reason: **a confirmation the attacker's page cannot supply defeats the deception regardless of whether framing is possible** — a current password, a second factor, or a code sent out of band. It is the only measure in this entry that does not depend on the browser enforcing anything.
- **And do not rely on frame busting scripts.** Measured, a blocked frame still fires `onload`; and `sandbox` lets an attacker suppress the script while keeping the render. Both are properties of the attacker's page, not of yours.

<!-- lang:zh -->
### 意图问题的另一半

CSRF 那篇讲的是一个带着有效凭据、而用户从未打算发出的请求。这一篇讲的是它的镜像，值得成对地写出来：

| | CSRF | 点击劫持 |
|---|---|---|
| 用户做了什么 | 什么都没做 | **点了，而且是故意的** |
| 被伪造的是什么 | **请求** | **他以为自己点的是什么** |
| 谁在动作 | 浏览器，自动的 | **用户，亲手** |
| 什么能挡住 | 一个证明意图的 token | **拒绝被框住** |

> **CSRF 利用的是意图的缺席。点击劫持制造了一个意图。**

这就是为什么两者的修法不同，也是为什么一个 CSRF token 在这里毫无作用：那个请求带着 token、会话是真的、用户按了按钮。唯一不对的是**他按下它时看的是哪一个文档**。

### 构造

手法是攻击者的一个页面，把目标嵌进一个 iframe，让它不可见，并定位成"帧里的某个控件正好落在用户想按的那个控件下面"：

```css
iframe {
  position: absolute; inset: 0;
  width: 100%; height: 100%;
  opacity: 0;          /* 不可见，但不是不存在 */
  border: 0;
  z-index: 2;          /* 在诱饵之上 */
}
```

在浏览器里实测，目标页的全部内容就是一个写着"确认转账"的按钮，而诱饵写着"恭喜！你抽中了 iPhone"，下面是一个绿色的"点这里领取"按钮。渲染出来的页面在视觉上**就是一个普通的中奖页** —— 没有任何东西暗示那里存在第二个文档。对齐是唯一的技术要求：目标的按钮必须落在诱饵按钮的位置上，那是偏移量的事，必要时还要滚动帧内的内容。

### 四层

1. **信任边界。** 应用信任"按下它按钮的那个人知道自己在按什么"。而页面的**外观**属于把它框住的那个人，用户对处境的一切了解都来自那个外观。
2. **数据与指令共用同一平面。** 屏幕上同一块区域同时是**用户看见的那个诱饵**和**接收那次点击的那个控件**。视觉层与行为层被分开了，而只有第一层是攻击者能控制的 —— 这正是这种欺骗"看一眼发现不了"的原因。
3. **为什么常见修法失败。** 基于 JavaScript 的 frame busting 因为两个彼此独立的理由而不可靠，而第一个是实测的：**被拦下的帧仍然会触发 `onload`**，所以脚本无法可靠地判断自己有没有被框住。第二个是 `sandbox` 属性让攻击者可以禁掉那些本该执行 busting 的脚本，同时页面照样渲染。一个跑在被框文档里的检查，覆盖不了浏览器关于"能不能框"的决定。
4. **变体。** 上面那种透明覆盖层；拖放变体，其中落点在帧内；粘贴或键盘驱动的变体；以及最实际的那一种 —— **框住一个子域、或者任何攻击者能在上面发布内容的源**，它能击败一份只排除了攻击者自己那个源的策略。

### 实测：真实浏览器里的五种框住策略

同一个受害页，用五种不同的响应头发出去，被同一个攻击者页面框：

| 受害页的响应头 | 浏览器的实际行为 |
|---|---|
| *（无）* | **成功框住** —— 页面在覆盖层里渲染了 |
| `X-Frame-Options: DENY` | **拒绝** —— `Refused to display ... because it set 'X-Frame-Options' to 'deny'` |
| `Content-Security-Policy: frame-ancestors 'none'` | **拒绝** —— `Framing ... violates the following Content Security Policy directive` |
| **`X-Frame-Options: ALLOW-FROM https://trusted.example`** | **被忽略** —— `'ALLOW-FROM ...' is not a recognized directive. The header will be ignored.` |
| `X-Frame-Options: DENY, SAMEORIGIN`（一个头里两个值） | **拒绝** —— `multiple 'X-Frame-Options' headers with conflicting values ... Falling back to 'deny'` |

由此有三件事，而第三件值得记住。

**`X-Frame-Options` 与 `frame-ancestors` 都有效**，而它们是仅有的两项"作用在决定发生的地方"的控制 —— 在浏览器的框住逻辑里，而不是在页面脚本里。

**`frame-ancestors` 表达力更强。** `X-Frame-Options` 只提供 `DENY` 与 `SAMEORIGIN`，别无其他；而 `frame-ancestors` 接受一组来源，那才是一个"确实需要被合作方框住"的站点所需要的东西。两个都发是通常的安排：新浏览器看 CSP 指令，老浏览器退回看那个头。

**`ALLOW-FROM` 看起来像一条策略，而它不是。** 浏览器把它报成"未被识别的指令"并**完全忽略那个头** —— 这意味着**那个页面和一个完全没有头的页面一样不受保护**，而它的配置里却写着某种读起来像限制的东西。这是真实部署里最值得查的一行，因为不去看浏览器实际做了什么就看不见它。

而最后那一列的第三行则值得从反方向记一笔：一个畸形的多值头让浏览器**回退到 `deny`**，所以那个特定的错误是"失败关闭"的。不是每一个畸形的安全头都这样。

### 为什么框住一个子域是要紧的

一份"拒绝其他源框住"的策略仍然允许站点自己框住自己，而"站点自己"包括共享注册域下的每一个子域，除非那条指令另有说明。所以问题不只是"攻击者能不能框住我们"，而是：

> **有没有一个"攻击者能在上面放内容"的源，被我们的策略允许框住我们？**

一个用于用户内容的子域、一个预发布主机、一个挂在共享域上的市场站点，或者任何有 XSS 的源 —— 每一个都是攻击者可以发布那个框住页面的地方，这就是为什么 `frame-ancestors 'none'` 是更安全的默认值，而列出一组具体来源是更安全的例外形式。

### 检测与缓解

- **对那些"本不该被框住"的页面，当请求的 `Sec-Fetch-Dest` 是 `iframe` 时告警。** 现代浏览器会发这个 fetch metadata 头，所以检查是一次取值比较而不是启发式 —— 而且它连"被 CSP 随后拦下的框住尝试"也能看到，这意味着它检测的是尝试而不是结果。
- **当一个敏感请求带着来自另一个源的 `Referer` 到达时告警。** 一个来自别处页面的状态变更，要么是一次正当的集成，要么是一次被框住的点击。
- **并且把"会话里没有对应的页面浏览"的敏感请求当作值得看一眼。** 用户走到一个确认按钮的正常路径包含加载它所在的那个页面；一次没有那次加载的点击，就是被框住交互的形状。
- **缓解上，从一个地方同时发 `frame-ancestors` 与 `X-Frame-Options`。** 一个施加到每个响应上的中间件或服务端默认值，而不是逐页设置的头 —— 因为这里的失效类别是"有人忘了某一页"，而一页被忘掉和一页没有保护是无法区分的。
- **绝不要用 `ALLOW-FROM`。** 它不被识别，所以它静默地授予了它看起来要限制的东西。确实需要被框住的站点，把来源列进 `frame-ancestors`。
- **默认用 `'none'`，并把每一个例外都当成一个决定。** 允许框住本站的来源清单应当很短、经过刻意选择、并且被评审 —— 而每一条都应当是一个"站点能接受它发布关于自己的任意内容"的源。
- **对要紧的操作要求重新认证。** CSRF 那篇结尾的那项控制在这里因为同样的理由适用：**一个攻击者的页面提供不出来的确认，无论能不能被框住都能击败这种欺骗** —— 当前口令、第二因素，或者一条带外发送的验证码。这是这一篇里唯一一项不依赖浏览器执行任何东西的措施。
- **并且不要依赖 frame busting 脚本。** 实测：被拦下的帧仍会触发 `onload`；而 `sandbox` 让攻击者可以压住脚本、同时保留渲染。两者都是攻击者那个页面的性质，不是你的。
