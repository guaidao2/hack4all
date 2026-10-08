---
id: bizlogic-fundamentals
title_en: "Business Logic, Part 1 — Finding the Assumption"
title_zh: "业务逻辑（一）：找到那个假设"
summary_en: Every other entry in this series is about a component misreading a value. Business logic flaws are about a component misreading a sequence — each request is valid on its own, and only the order or the combination is wrong. This entry covers the four assumptions that produce them, with a worked example of each.
summary_zh: 本系列其他每一篇讲的都是"某个组件把一个值读错了"。业务逻辑缺陷讲的是"某个组件把一串请求读错了" —— 每一笔请求单独看都合法，错的是顺序或者组合。这一篇讲产生它们的那四类假设，每一类给一个完整例子。
tags: [web, business-logic, cwe-840, authorization, design]
tools: [curl, Burp Suite, browser devtools]
attck: [T1190, T1565]
platform: [web]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The one difference from every other entry

Everything in this series so far has the same shape: two components read the same bytes differently, and the disagreement is the bug. Business logic flaws do not have that shape, and it is worth being precise about why, because it changes how they are found.

> **The code is not misreading anything. It answers the question it was written to answer. The question was the wrong one.**

A price-tampering bug is a good example. The code takes a number from the request and multiplies it. There is no parsing disagreement, no encoding mismatch, no injection. The code is correct; what it assumed is that the number **came from a trustworthy place**, because in the flow its author had in mind, the number came from a form the application itself rendered.

So the trust boundary is different here. In every other entry the application trusted a component; here it trusts **the flow itself** — that the client will use the endpoints in the order designed, provide the values they are meant to provide, and not combine things that were meant to be mutually exclusive.

That is also why these are the hardest class to find with a tool:

- **Every request is well-formed.** There is nothing to match a signature against.
- **Every value may be legal.** The price is a valid number; the coupon code exists; the order number belongs to the user.
- **The defect is in the relationship between requests**, which no per-request inspection can see.
- **The rule is usually written down somewhere** — a business document, a help page, a tooltip — and never in the code.

Measured on a purpose-built lab, all four of the following are real vulnerabilities with a working exploit, and none of them contains a single malformed request.

### Assumption one: where a value comes from

**The assumption**: the client sends back the values the application gave it.

The lab's checkout page puts the unit price in a hidden form field, and the server multiplies it by a quantity from the same form:

```
POST /order
item=chair&price=1888.00&qty=1        -> 1888.00
item=chair&price=0.01&qty=1           -> 0.01        price from the client
item=chair&price=1888.00&qty=-5       -> -9440.00    quantity from the client
```

Two paths, one assumption. The second line is the obvious one; the third is the same bug used the other way — a negative quantity makes the shop pay the customer, and it is often missed because "quantity" feels like a display field rather than a price field.

**Why the field exists at all** is the diagnostic question. A price in a hidden input is there so the page can show a total without another request. It is presentation state, and presentation state belongs to the client; **money belongs to the server**. The presence of a value in a request is not evidence that the value needs to be there.

### Assumption two: how many times an action may happen

**The assumption**: the user presses the button once.

The lab's refund endpoint takes an order number and an amount, checks the order's status, and adds the amount to the balance. It never records that a refund happened:

```
POST /refund  order=SO-2026-0001&amount=199.00   -> balance 199.00   (paid 199.00)
POST /refund  order=SO-2026-0001&amount=199.00   -> balance 398.00   total refunded exceeds paid
```

The same request twice, and the total refunded exceeds what was paid. The amount is also taken from the request, which is the *other* half of the same bug and a second way to reach the same place:

```
POST /refund  order=SO-2026-0001&amount=9999.00  -> balance 9999.00
```

Three questions cover most of this class, and they are worth memorising because they apply to anything that moves money or state:

1. **How many times may this action happen?** Press it again — what stops it?
2. **Who computes the amount?** If the answer involves the request body, that is the bug.
3. **Where is "already done" recorded?** If it is nowhere, the action is repeatable by construction.

The design that answers all three is small and worth showing, because the shape generalises:

```python
def refund(order_no):
    order = load(order_no)
    refunded = sum_of_refunds(order_no)          # a ledger, not a flag
    if order.status not in ("shipped", "delivered"):
        reject("wrong state")
    remaining = order.paid - refunded
    if remaining <= 0:
        reject("nothing left to refund")
    do_refund(order_no, remaining)               # the server computes it
```

Two properties make this **idempotent without a special case**: it computes **the remaining amount** rather than a requested amount, and it records each refund as a row. A second identical request finds `remaining == 0` and does nothing. **Idempotency here is a consequence of the data model, not a check bolted on top.**

A note on the boundary with the next entry: if two refund requests arrive **at the same time**, the sum computed by both may be zero-refunded and both proceed. That is a race condition — the same missing record, with the gap between the read and the write as the opening.

### Assumption three: that values are used one at a time

**The assumption**: a field has one value, and one coupon is one coupon.

The lab's order form reads the coupon field and subtracts each one it finds. The page's rule says one coupon per order and no coupon twice. The rule is on the page. The code has no check for either:

```
POST /order  coupon=FULL200&coupon=NEW30      -> 300 - 50 - 30  = 220.00
POST /order  coupon=FULL200&coupon=FULL200    -> 300 - 50 - 50  = 200.00   same coupon twice
POST /order  coupon=FULL200&coupon=NEW30&coupon=VIP20 -> 300 - 100 = 200.00
```

The mechanism is a single API choice: the framework offered `getlist()` — returning **every** value for a repeated field name — and the developer read it as if it were `get()`, returning one. **The code did what it was told; the mental model of what a form field is was wrong.**

Two lessons generalise beyond coupons:

**A rule that exists only in documentation does not exist.** "One coupon per order" was enforced by a dropdown that only allows one selection — that is a client-side constraint, and the request is written by the client.

**Validation of each element is not validation of the set.** Every coupon code was valid, every discount was correct, and the combination was not allowed. Checking values one at a time cannot detect a rule about combinations, and this is the part that makes the class hard: the check that is missing is not a check on a field.

The five questions worth asking of any feature that combines things:

1. **Each value is legal — what about the combination?** Two mutually exclusive coupons, a discount plus a credit, a coupon plus loyalty points.
2. **Can the same thing be submitted twice?** A repeated field name, or the same request sent again.
3. **Does the order matter?** A percentage discount before or after a fixed one gives different totals — and if the server applies them in request order, the client chooses the order.
4. **Is there a floor or ceiling?** Can the total exceed the price, or go negative?
5. **Is the exclusivity in the code, or only in the description?** If it is only in the description, it is not there.

### Assumption four: that state is a state

**The assumption**: an order's condition is described by its `status` field.

The lab's order has a status machine — paid, shipped, completed — and every endpoint checks the status it cares about before acting:

| Endpoint | Its check | Looks fine |
|---|---|---|
| ship | status must be `paid` | yes |
| refund | status must be `paid` or `shipped` | yes |
| confirm receipt | status must be `shipped` | yes |

And refunded is recorded as **a boolean field on the order**, not as a state:

```
paid --ship--> shipped --refund--> shipped, refunded=1 --confirm--> completed, refunded=1
```

**Every individual check is correct.** The refund endpoint did not move the status, so the order was still `shipped`; the confirm endpoint checked `shipped` and did not know about the boolean. The path that no designer intended is reachable by composing a sequence where each step passed its own check.

> **This is not a missing `if`. It is a state model that does not describe the system.**

The corrected model has two parts, and both matter:

**Refunded belongs in the state machine**, because it decides what the order may do next. It is a state that happens to be terminal, not a flag beside the state.

**The transitions belong in a table, not in scattered conditions.** One table, consulted by every endpoint:

```python
TRANSITIONS = {
    "paid":      ["shipped", "cancelled"],
    "shipped":   ["completed", "refunded"],
    "completed": [],
    "refunded":  [],
    "cancelled": [],
}

def transition(order, target):
    if target not in TRANSITIONS[order.status]:
        reject(f"{order.status} -> {target} is not a transition")
```

Two consequences follow from putting it in a table. **Implementing the corrected model needs no new `if`** — adding "refunded" to the states removes the path automatically, because `completed` is not reachable from `refunded`. And **adding a state changes one table rather than ten endpoints**, which is the difference between a model that stays correct and one that decays.

The generalisation of this one: **every field that describes the condition of an object, other than the one named "status", is a candidate for this bug.** `refunded`, `paid`, `shipped`, `cancelled`, `locked`, `verified` — each can coexist with any status, and each combination that should not exist is a path someone can walk.

### Finding them: from the flow, not from the technology

The method that works is not a scanner and not a payload list. It is reading the flow and writing down what it assumes.

**Draw the states, then look for the fields beside them.** List every state and every transition, and separately list every boolean or numeric field on the same object. For each (state, field) pair, ask whether that combination should be reachable. In the measured example, "completed + refunded=1" is obviously wrong on inspection — and obviously invisible from any single endpoint.

**Write the flow's invariants as sentences, then try to break each one.** "Total equals the sum of unit price times quantity, computed by the server." "A coupon is used at most once." "Refunds never exceed what was paid." Each sentence is a test case, and each is a place where the code may have assumed instead of enforced.

**Ask of every value in a request: does the server need it, or did the server give it to me?** A price, a discount, a user identifier, a role, a total, a status — if the client sent it and the server used it, that is assumption one.

**Ask of every action: what happens if I do it twice?** Press it again, replay the request, send it from two tabs. If nothing in the data model records that it happened, it can happen again — and if two copies arrive together, it becomes the race condition in the next entry.

**And ask of every rule: where is it written?** A rule in a policy document, a tooltip, a dropdown, a JavaScript check or a comment is not enforced. The place to look is the handler for the request that would violate it.

### Detection and mitigation

- **Treat novelty of request sequences as the signal, not novelty of payloads.** Business logic abuse looks like ordinary traffic, so the detection is behavioural: the same account issuing the same action twice in a short window, a sequence that skips a step, a value outside the range that account has ever produced. This is the class where anomaly detection has real value and signature matching has none.
- **Alert on values that arrive from the client in fields the server should own.** A request carrying `price`, `amount`, `discount`, `total`, `role`, `user_id` or `status` is worth an alert the first time it is seen, because it is either a design that will be abused or a payload that already is. The same rule applied to **repeated field names** catches the coupon case: a parameter appearing twice is an ordinary HTTP request and an unusual client.
- **Alert on totals and balances moving the wrong way.** A negative order total, a refund exceeding the paid amount, a balance with a sign that should not be reachable — these are cheap arithmetic checks on data the application already has, and they catch the outcome whichever request produced it.
- **Watch for the missing record rather than the missing check.** If an action has no ledger, the query "how many times has this happened" cannot be asked — by the attacker or by the defender. Building the ledger is usually the fix and the detection at once.
- **For mitigation, put the decision where the data is.** The server computes every amount it charges or refunds, reads no price from a request, and derives identity and role from the session rather than from a field. This is the same principle as everywhere else in the series — the value is used where it was produced — applied to numbers instead of to strings.
- **Model state explicitly, and make the model the only path.** One transition table, consulted by every handler; conditions that affect what happens next are states rather than parallel flags. The measure of whether this was done is whether adding a state requires touching one place or ten.
- **Make the actions idempotent by construction.** Compute the remaining amount rather than a requested amount, and record every occurrence as a row. An idempotent action is safe against the retry, the double click and — with appropriate locking or a unique constraint — the concurrent pair.
- **Write the invariants down as tests.** Every sentence from the method above becomes an assertion the test suite enforces: the refund total never exceeds the paid total, a coupon is consumed once, the state sequence is one of the allowed paths. The rules that were only in a document become rules the build checks.
- **And accept that this class has no central control.** There is no parser setting, no decoder and no network rule that fixes a wrong assumption about a flow. That is why the mitigation here is a design habit rather than a list of settings — and why this is the class most likely to be found by a person reading the feature rather than by a tool reading the traffic.

<!-- lang:zh -->
### 与其他每一篇的那一处区别

到目前为止，本系列里的一切都是同一个形状：两个组件对同一串字节的读法不同，而那个分歧就是 bug。业务逻辑缺陷没有这个形状，而说清为什么值得花点笔墨，因为它改变了"怎么找它们"。

> **代码没有把任何东西读错。它回答了它被写出来要回答的那个问题。是那个问题问错了。**

价格篡改是个好例子。代码从请求里取一个数字然后相乘。没有解析分歧、没有编码错配、没有注入。代码是对的；它假设的是那个数字**来自一个可信的地方** —— 因为在它作者心里的那个流程里，那个数字来自应用自己渲染出来的表单。

所以这里的信任边界不一样。其他每一篇里，应用信任的是某个组件；这里它信任的是**流程本身** —— 客户端会按设计的顺序使用这些接口、会提供那些它本该提供的值、不会把本该互斥的东西组合起来。

这也是为什么这类漏洞最难用工具发现：

- **每一笔请求都是良构的。** 没有东西可以拿签名去匹配。
- **每一个值都可能是合法的。** 价格是一个合法数字；券号存在；订单号属于这个用户。
- **缺陷在请求之间的**关系**里**，而任何按单笔请求做的检查都看不到它。
- **规则通常写在某个地方** —— 业务文档、帮助页、tooltip —— 而从来不在代码里。

在一个专门做出来的靶场上实测，下面四类都是真实漏洞、都有可用的利用，而其中**没有一笔畸形请求**。

### 假设一：一个值从哪里来

**假设**：客户端会把应用给它的值原样送回来。

靶场的下单页把单价放在一个隐藏表单字段里，而服务端拿它乘以同一个表单里的数量：

```
POST /order
item=chair&price=1888.00&qty=1        -> 1888.00
item=chair&price=0.01&qty=1           -> 0.01        价格来自客户端
item=chair&price=1888.00&qty=-5       -> -9440.00    数量来自客户端
```

两条路径，一个假设。第二行是显然的那种；第三行是同一个 bug 换了个方向用 —— 一个负数量让商店倒找顾客钱，而它经常被漏掉，因为"数量"感觉像个展示字段而不是价格字段。

**那个字段为什么会存在**，才是诊断性的问题。隐藏输入里的价格在那里，是为了让页面不再发一次请求就能显示总价。它是**展示状态**，而展示状态属于客户端；**钱属于服务端**。一个值出现在请求里，并不证明它需要出现在那里。

### 假设二：一个动作可以做几次

**假设**：用户会按一次按钮。

靶场的退款接口收一个订单号和一个金额，检查订单状态，然后把金额加到余额上。它从不记录发生过退款：

```
POST /refund  order=SO-2026-0001&amount=199.00   -> 余额 199.00   （实付 199.00）
POST /refund  order=SO-2026-0001&amount=199.00   -> 余额 398.00   退款总额超过实付
```

同一个请求发两次，退款总额就超过了实付。金额同样取自请求 —— 那是同一个 bug 的**另一半**，也是通向同一个结果的第二条路：

```
POST /refund  order=SO-2026-0001&amount=9999.00  -> 余额 9999.00
```

三个问题覆盖了这一类的大部分，值得背下来，因为它们适用于任何"会动钱或动状态"的东西：

1. **这个动作允许发生几次？** 再按一次 —— 有什么拦着？
2. **金额是谁算的？** 如果答案里涉及请求体，那就是 bug。
3. **"已经做过"记在哪？** 如果哪儿都没记，这个动作按构造就是可重复的。

一个把三个问题都回答了的实现很小，值得写出来，因为它的形状可推广：

```python
def refund(order_no):
    order = load(order_no)
    refunded = sum_of_refunds(order_no)          # 一张流水表，不是一个标志位
    if order.status not in ("shipped", "delivered"):
        reject("wrong state")
    remaining = order.paid - refunded
    if remaining <= 0:
        reject("nothing left to refund")
    do_refund(order_no, remaining)               # 金额由服务端算
```

有两个性质让它**不需要特判就是幂等的**：它算的是**剩余可退金额**而不是"请求的金额"，并且把每一次退款记成一行。第二个一模一样的请求会发现 `remaining == 0`，于是什么都不做。**这里的幂等是数据模型的推论，不是事后加的一道检查。**

关于和下一篇的边界，说一句：如果两个退款请求**同时**到达，两边各自算出的剩余可能都还没变，于是都继续执行。那是竞态条件 —— 同一个"没有记录"的问题，只是把"读与写之间的间隙"当作了开口。

### 假设三：值是一次用一个的

**假设**：一个字段只有一个值，而一张券就是一张券。

靶场的下单表单读优惠券字段，把它找到的每一张都减掉。页面上的规则写着：一单一张券、同一张券不能用两次。规则在页面上。代码里两条都没检查：

```
POST /order  coupon=FULL200&coupon=NEW30      -> 300 - 50 - 30  = 220.00
POST /order  coupon=FULL200&coupon=FULL200    -> 300 - 50 - 50  = 200.00   同一张券两次
POST /order  coupon=FULL200&coupon=NEW30&coupon=VIP20 -> 300 - 100 = 200.00
```

机制只是一个 API 选择：框架提供了 `getlist()` —— 返回重复字段名的**每一个**值 —— 而开发者把它当成了返回一个值的 `get()` 来读。**代码照它被要求的方式执行了；错的是"一个表单字段是什么"这个心智模型。**

两个教训超出了优惠券这个场景：

**只写在文档里的规则不存在。** "一单一张券"是由一个只允许选一项的下拉框"实现"的 —— 那是客户端约束，而请求是客户端写的。

**对每个元素做校验，不等于对这个集合做校验。** 每个券号都合法、每一笔折扣都算对，而不允许的是那个组合。逐值检查不可能发现一条关于组合的规则，而这一点正是这类漏洞难的地方：**缺的那道检查，不是对一个字段的检查。**

对任何"会把东西组合起来"的功能，五个值得问的问题：

1. **每个值都合法 —— 那组合呢？** 两张互斥的券、一笔折扣加一次积分抵扣、一张券加一次余额抵扣。
2. **同一个东西能提交两次吗？** 重复的字段名，或者同一个请求再发一次。
3. **顺序要紧吗？** 百分比折扣在固定减免之前还是之后，总额不一样 —— 而如果服务端按请求里的顺序处理，那就是客户端在选顺序。
4. **有下限和上限吗？** 总额能不能超过售价，或者变成负数？
5. **互斥是写在代码里，还是只写在说明里？** 如果只在说明里，那就是没有。

### 假设四：状态是一个状态

**假设**：一个订单的处境由它的 `status` 字段描述。

靶场的订单有一个状态机 —— 已付款、已发货、已完成 —— 而每个接口在动作之前都检查它在意的那个状态：

| 接口 | 它的检查 | 看着没问题 |
|---|---|---|
| 发货 | status 必须是 `paid` | 是 |
| 退款 | status 必须是 `paid` 或 `shipped` | 是 |
| 确认收货 | status 必须是 `shipped` | 是 |

而"已退款"被记成**订单上的一个布尔字段**，不是状态：

```
已付款 --发货--> 已发货 --退款--> 已发货, refunded=1 --确认收货--> 已完成, refunded=1
```

**每一个单独的检查都是对的。** 退款接口没有动 status，所以订单仍是 `shipped`；确认收货接口检查的是 `shipped`，它不知道那个布尔字段。那条没有设计师想要过的路径，可以通过组合一串"每一步都过了自己那关"的请求走到。

> **这不是漏了一句 `if`。这是一个描述不了系统的状态模型。**

修正后的模型有两部分，两部分都要紧：

**"已退款"属于状态机**，因为它决定这个订单接下来还能做什么。它是一个恰好是终态的状态，而不是状态旁边的一个标志位。

**转换属于一张表，而不是散落各处的条件。** 一张表，被每个接口查阅：

```python
TRANSITIONS = {
    "paid":      ["shipped", "cancelled"],
    "shipped":   ["completed", "refunded"],
    "completed": [],
    "refunded":  [],
    "cancelled": [],
}

def transition(order, target):
    if target not in TRANSITIONS[order.status]:
        reject(f"{order.status} -> {target} is not a transition")
```

把转换放进表里带来两个后果。**实现修正后的模型不需要新增任何 `if`** —— 把 "refunded" 加进状态集合，那条路径自动消失，因为从 `refunded` 到不了 `completed`。而**新增一个状态改的是一张表，不是十个接口**，这就是"保持正确的模型"与"逐渐腐烂的模型"之间的差别。

这一条的推广是：**每一个描述对象处境、而名字不叫 "status" 的字段，都是这个 bug 的候选。** `refunded`、`paid`、`shipped`、`cancelled`、`locked`、`verified` —— 每一个都能与任何状态共存，而每一个"本不该存在"的组合，都是一条有人能走的路。

### 怎么找它们：从流程入手，不是从技术入手

管用的方法不是扫描器，也不是 payload 清单，而是读懂流程、把它假设的东西写下来。

**画出状态，然后找状态旁边的那些字段。** 列出每一个状态与每一条转换，再单独列出同一个对象上每一个布尔或数值字段。对每一个（状态, 字段）组合，问这个组合是否应该可达。在实测的那个例子里，"已完成 + refunded=1" 一眼就是错的 —— 而从任何一个单独的接口看，它一眼也看不见。

**把流程的不变式写成句子，然后逐个试着破坏。** "总额等于服务端算出的单价乘数量之和。" "一张券最多用一次。" "退款永远不会超过实付。" 每一句都是一个测试用例，也是代码可能"假设了"而没有"强制了"的地方。

**对请求里的每一个值都问：服务端需要它，还是服务端把它给了我？** 价格、折扣、用户标识、角色、总额、状态 —— 如果是客户端发的而服务端用了，那就是假设一。

**对每一个动作都问：我做两次会怎样？** 再按一次、把请求重放、从两个标签页各发一次。如果数据模型里没有任何东西记录它发生过，它就能再发生一次 —— 而如果两份同时到达，它就变成下一篇里的竞态条件。

**并且对每一条规则都问：它写在哪？** 写在策略文档、tooltip、下拉框、JavaScript 检查或注释里的规则，都没有被强制。该去看的地方，是**那个会违反它的请求的处理器**。

### 检测与缓解

- **把"请求序列的新奇程度"当作信号，而不是"载荷的新奇程度"。** 业务逻辑滥用看起来就是普通流量，所以检测是行为式的：同一个账号在短窗口内发出同一个动作两次、一条跳过某一步的序列、一个该账号从未产出过的越界数值。这是异常检测真正有价值、而签名匹配完全没有价值的一类。
- **对"从客户端到来、却本应由服务端拥有的字段"告警。** 一个携带 `price`、`amount`、`discount`、`total`、`role`、`user_id` 或 `status` 的请求，第一次看到就值得告警，因为它要么是一个迟早会被滥用的设计，要么就已经是一次攻击。同一条规则用在**重复的字段名**上就能抓到优惠券那个场景：一个参数出现两次是普通的 HTTP 请求，却是不寻常的客户端。
- **对"总额与余额朝错的方向走"告警。** 一个负的订单总额、一笔超过实付的退款、一个本不该可达的符号 —— 这些都是对应用本来就有的数据做的廉价算术检查，而无论哪一笔请求造成了它，它们都能抓到**结果**。
- **盯"缺失的记录"，而不是"缺失的检查"。** 如果一个动作没有流水账，"这件事发生过几次"这个问题就无法被问出口 —— 无论对攻击者还是对防守方。把流水账建起来，通常同时是修法和检测。
- **缓解上，把决定放在数据产生的地方。** 服务端算它收取或退还的每一笔金额，不从请求里读价格，身份与角色来自会话而不是来自一个字段。这和其他每一篇是同一条原则 —— 值在它被产生的地方使用 —— 只是这里用在数字上而不是字符串上。
- **显式建模状态，并让这个模型成为唯一的路径。** 一张转换表，被每个处理器查阅；影响"接下来能做什么"的条件是状态，不是并行的标志位。判断这件事做没做到的标尺是：新增一个状态需要改一个地方，还是十个。
- **让动作按构造就是幂等的。** 算"剩余可退"而不是"请求要退"，并把每一次发生都记成一行。一个幂等的动作对重试、对双击、以及在配合锁或唯一约束之后对并发那一对，都是安全的。
- **把不变式写成测试。** 上面方法里的每一句都变成测试套件强制执行的一条断言：退款总额永远不超过实付、一张券只被消费一次、状态序列是允许路径之一。那些原本只写在文档里的规则，变成了构建会检查的规则。
- **并且接受这一类没有集中控制。** 没有任何解析器设置、解码器或网络规则能修好一个关于流程的错误假设。这就是为什么这里的缓解是一种设计习惯而不是一份设置清单 —— 也是为什么这一类最可能被**读功能的人**发现，而不是被**读流量的工具**发现。
