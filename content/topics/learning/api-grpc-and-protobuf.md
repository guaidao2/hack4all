---
id: api-grpc-and-protobuf
title_en: gRPC and Protocol Buffers
title_zh: gRPC 与 Protocol Buffers
summary_en: A binary protocol where the field names are not on the wire, so what a message means is decided by whoever holds the schema. Measured by hand-encoding and hand-decoding the format — one byte string read two ways under two schemas, a field that is one value or three depending on its declared cardinality, and a number with more than one legal encoding.
summary_zh: 一种字段名不在线上的二进制协议，所以一条消息是什么意思，取决于谁拿着那份 schema。这一篇用手写的编解码器实测这个格式 —— 一串字节在两个 schema 下读出两种含义、一个字段是"一个值"还是"三个值"取决于声明的基数、以及一个数字有多种合法编码。
tags: [beginner, api, grpc, protobuf, http2, idl]
tools: [grpcurl, protoc, python3]
attck: [T1190]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### The names are not on the wire

JSON carries its structure with it: the field names are in the request, so anyone holding the bytes can read them. A binary format does the opposite, and the trade is worth stating exactly.

> **Protocol Buffers replaces names with numbers and types with a small integer. What travels is the structure, not its meaning — and the meaning lives in a `.proto` file that the sender and receiver each have to hold.**

Which produces a property that has no analogue in the text formats measured earlier in this series: **the same bytes are a different message under a different schema**, and both readings are correct.

Measured below with a hand-written encoder and decoder, so that every byte in the tables is visible and no library is doing anything unexplained.

### Part 1: the shape of it

| | Here |
|---|---|
| transport | **HTTP/2**, multiplexed streams over one connection |
| payload | **length-prefixed frames**, each carrying one message |
| message format | **field number, wire type, value** — no names |
| the contract | a **`.proto`** file, compiled into code on both sides |
| the method | in the HTTP/2 `:path`, as `/package.Service/Method` |
| errors | a status code **in the trailers**, not in the body |
| shapes | unary, server streaming, client streaming, bidirectional |

**Four of those rows are security-relevant by themselves**, and the rest of this entry is about them: the schema decides meaning, the frames have a length the sender writes, the method is a path a gateway may or may not recognise, and the status code arrives in a place some intermediaries discard.

### Part 2: the wire carries field numbers

Measured, encoding a message with two fields and reading it back byte by byte:

```
encoded: 08 2a 12 05 61 6c 69 63 65

08        tag      field 1, wire type 0 (varint)
2a        value    42
12        tag      field 2, wire type 2 (length-delimited)
05        length   5
61 6c 69 63 65     "alice"
```

**The field name is nowhere in those nine bytes.** What is there is a number, an encoding for the value's shape, and the value.

**Four wire types do the work**, and knowing them is enough to read any message:

| Type | Means |
|---|---|
| 0 | varint — integers, booleans, enums |
| 1 | 64-bit fixed |
| 2 | length-delimited — strings, bytes, embedded messages, packed arrays |
| 5 | 32-bit fixed |

**And the consequence is the first principle of this entry:**

> **Nothing on the wire says what a field is for. A field number is an address into a schema, and the schema is held separately by each side.**

### Part 3: one byte string, two schemas, two meanings

Measured, four bytes offered to two different schemas:

```
wire:     08 07 10 01

schema A: field 1 is user_id, field 2 is is_admin   -> {user_id: 7, is_admin: 1}
schema B: field 1 is order_id, field 2 is quantity  -> {order_id: 7, quantity: 1}
```

**Both parse successfully, and neither is wrong.** The numbers and types line up; the nouns come from the schema.

**Which makes the `.proto` a security boundary rather than a build artefact.** Whoever can change it decides what a message means, and the checks worth making follow from that:

**Sender and receiver versions can differ, and the protocol is designed to allow it.** That is the point of the format — an old client and a new server have to interoperate — so a mismatch is normal, and the question is what each side does with a field the other one means something else by.

**And reusing a field number is the one change that breaks the guarantee.** A number that has been published belongs to its meaning permanently:

> **Reusing a retired field number does not recycle an identifier. It tells every client still running the old schema that the new field is the old one.**

**Which is why the format has a `reserved` declaration**, and why a change that removes a field and adds a differently-named one with the same number is the shape of a bug that no test in either version will catch.

**And this is where the entry on discovery applies to a binary protocol.** Without the schema there is no reading the traffic; with the schema, everything is readable. **A service with reflection enabled hands the schema to anyone who asks**, which is a legitimate development feature and a decision worth making explicitly for anything internet-facing.

### Part 4: order, duplicates, and fields nobody declared

Three properties of the format that matter when anything is hashed, signed or checked.

**Field order carries no meaning.** Measured, the same two fields in two orders:

```
08 2a 12 05 61 6c 69 63 65   -> {id: 42, name: "alice"}
12 05 61 6c 69 63 65 08 2a   -> {id: 42, name: "alice"}
```

**Different bytes, the same message.** So:

> **A hash over a serialised protobuf message is a hash over one encoder's choices, not over the message.** Signing or deduplicating on those bytes compares a representation rather than a value.

**A field number may appear more than once.** Measured, with field 2 sent twice as `1` then `99999`:

| Decoder | Value |
|---|---|
| taking the last occurrence (what the specification says) | **`99999`** |
| taking the first occurrence (some implementations) | **`1`** |

**This is the same shape as the duplicate JSON key measured in the webhook entry** — one byte string, two values, both defensible, and no agreement about which one the application acted on. **The difference is that here it is the specification that says last, so a receiver taking the first is out of step with it rather than exercising a permitted ambiguity.**

**And unknown fields are skipped rather than rejected.** Measured, a message carrying an undeclared field number still parses under a schema that does not know it:

```
wire: 08 2a 48 b9 60 12 05 61 6c 69 63 65    (field 9 is not in the schema)
parsed: {id: 42, name: "alice"}
```

**That is forward compatibility, and it is also the property to be careful about.** A check that looks only at the parsed object sees less than what arrived — and the fields it did not see are still there for anything that re-reads the bytes.

### Part 5: one byte string, one value or three

This is the measurement that best shows what "the schema decides" means in practice. Three occurrences of field 3:

```
wire: 18 0a 18 14 18 1e

under a scalar schema:    {amount: 30}
under a repeated schema:  {amounts: [10, 20, 30]}
```

**The same bytes, and the difference between reading one value and reading three is a declaration in a file that is not travelling with them.**

**So cardinality is wire-compatible.** Changing a field from a single value to a list is a backwards-compatible change at the binary level — which is exactly why it is worth noticing:

> **"Wire-compatible" is not "semantically compatible". A field that becomes a list still parses for every old client, which now sees only the last element.**

**And in the direction that matters for a review**: if a scalar and a list of that scalar are the same bytes, then anything that decides on the scalar and anything that decides on the list are deciding about the same request, with different answers — and the difference is which `.proto` each side compiled against.

### Part 6: more than one encoding of the same number

A varint is a little-endian base-128 number with a continuation bit, which means **small numbers have a short form and a longer form that is also valid.** Measured, the value 1:

```
canonical:  01                  -> 1
overlong:   81 80 80 80 00      -> 1
```

**The same value, five bytes instead of one, and both decode correctly.** So a value has more than one representation on the wire:

```
08 01 12 03 70 61 79              -> {id: 1, name: "pay"}
08 81 80 80 80 00 12 03 70 61 79  -> {id: 1, name: "pay"}
```

**Identical parsed, different bytes.** Which is the second reason not to build an equality check, a cache key or a replay guard out of serialised bytes — and the reason a signature that covers *the bytes* is stronger than one that covers *the message*:

> **A signature over the message is a signature over one encoding of it. Any other valid encoding of the same message is, to that signature, a different request.**

**And the canonical form exists for this**, defined so that two implementations producing the same message produce the same bytes. **It is not the default encoding**, and that distinction is the whole point.

### Part 7: the frame layer, where the length is the sender's

gRPC puts messages inside a five-byte prefix. Measured, one message:

```
frame: 00 00 00 00 02 08 2a

byte 1      0 = not compressed (1 would mean compressed)
bytes 2-5   2 = message length, big-endian
then        2 bytes of message
```

**Three things about that header matter.**

**The length is written by the sender.** Measured, the field is four bytes and admits `0xffffffff`, so a receiver that sizes a buffer from it is sizing a buffer from an untrusted number. **The fix is a maximum message size enforced before allocation**, which every implementation has as a setting and few deployments review.

**The compression flag is written by the sender too**, which makes decompression a step driven by the request — the same shape as the archive and compression entries elsewhere in this series: a small input that expands into a large one.

**And a message is not a frame.** Streaming means one logical message can span frames and one frame can carry part of one, so **"one request is one frame" is not a property to build a check on** — a length-prefixed stream has to be reassembled, and the reassembly is where the limits belong.

**And the status code arrives in the trailers.** In gRPC the outcome is an HTTP/2 trailer, not the response body, which means an intermediary that strips trailers turns a failure into something that looks like a success with an empty body. **That is not a remote bug**; it is the protocol meeting equipment that was written for a different one.

### Part 8: what follows for security

**The text-format injection families do not apply, and something else does.** There are no quotes to break out of, no delimiters to confuse, and no way for a value to change the structure of the message — the structure is a number. **So a request cannot be made to mean something it did not mean by putting characters in a string.**

**But the strings still travel.** A protobuf string field is a byte string that reaches application code, and from there a SQL statement, a shell command or a template. **The injection is downstream, in the same place as always**, and the entries on those families apply unchanged — what is different is only that this entry of the pipeline is not where it happens.

**And what replaces it is a question about schemas.** Measured, one byte string was two different messages under two schemas, one field was one value or three depending on its declaration, and one number had more than one encoding. **Every one of those is decided by a file rather than by the traffic**, which puts the review in the repository rather than in the request log:

**Field numbers are permanent.** A reused number is a silent lie to every client that has not been rebuilt.

**Cardinality changes are wire-compatible and not semantic.** A field that becomes a list still parses everywhere.

**Canonical encoding is not the default**, so bytes are a representation and the message is the value.

**And the same service is often reachable more than one way.** gRPC over HTTP/2, gRPC-Web from a browser, and a JSON transcoding of the same methods — **three front doors onto one implementation, and the access rules are usually written for one of them.** That is the gateway entry's finding with a different protocol: the rules are attached to what the rule-writer recognised, and a binary protocol with methods in the path is easy not to recognise.

**And credentials travel in metadata**, which is the HTTP/2 header block — so the authentication entry applies, with one addition: **metadata is per-call rather than per-connection**, and a long-lived stream established once may carry one authorisation decision for its whole lifetime.

### Detection and mitigation

- **Review `.proto` changes as interface changes, not as code changes.** A removed field number that gets reused, or a scalar that becomes repeated, is a behavioural change that compiles cleanly on both sides.
- **Require `reserved` for every retired field number**, and check it in the same place the schema is versioned.
- **Alert on reflection being enabled in production**, since it hands the schema to anyone who asks and removes the only thing protecting the traffic from being read.
- **Set and review a maximum message size**, because the measured frame header lets the sender declare one up to four gigabytes.
- **Alert on the compression flag being set where nothing compresses**, since it makes decompression a step the request drives.
- **Watch for trailers being dropped or rewritten by intermediaries**, because a stripped trailer turns a failed call into an apparently successful empty one.
- **Check that every front door has the same rules** — the HTTP/2 service, the browser transport, and any JSON transcoding of the same methods.
- **For mitigation, do not build hashes, cache keys or replay guards on serialised bytes.** Use a canonical encoding if one is needed, or sign the fields that matter.
- **Where a signature or a digest is required, be explicit about whether it covers bytes or values**, since the measured overlong encoding and the measured field-order independence are both ways for the two to differ.
- **Validate strings at the boundary where they stop being protobuf**, which is where the injection families apply, rather than trying to validate a format that has no syntax to abuse.
- **Authorise per method and per call, not per connection**, so a long-lived stream does not carry one decision past the point where it should have been re-made.
- **And treat the `.proto` files as part of the attack surface**: measured, they are the only thing that decides what any of the traffic means.

<!-- lang:zh -->
### 名字不在线上

JSON 把它的结构一起带着：字段名就在请求里，所以任何拿到那些字节的人都能读。二进制格式做的是相反的事，而这笔取舍值得说准。

> **Protocol Buffers 把名字换成数字、把类型换成一个小整数。传输的是结构，而不是它的含义 —— 而含义住在一个 `.proto` 文件里，发送方与接收方各自都得有一份。**

由此产生了一条在这个系列前面那些文本格式里没有对应物的性质：**同一串字节在不同 schema 下是另一条消息**，而两种读法都正确。

下面用一个手写的编解码器实测，这样每一张表里的每个字节都看得见，也没有任何库在背后做没解释的事。

### 第一部分：它的形状

| | 这里 |
|---|---|
| 传输 | **HTTP/2**，一条连接上多路复用的流 |
| 载荷 | **带长度前缀的帧**，每帧装一条消息 |
| 消息格式 | **字段号、wire type、值** —— 没有名字 |
| 契约 | 一份 **`.proto`** 文件，两边各自编译成代码 |
| 方法 | 在 HTTP/2 的 `:path` 里，形如 `/package.Service/Method` |
| 错误 | 一个状态码，在 **trailer** 里、不在 body 里 |
| 形态 | 一元、服务端流、客户端流、双向流 |

**其中四行本身就与安全相关**，而这一篇剩下的部分就是讲它们：schema 决定含义、帧的长度由发送方写、方法是网关可能认不出的一条路径、以及状态码到达的位置是某些中间设备会丢掉的地方。

### 第二部分：线上带的是字段号

实测，编码一条有两个字段的消息、再逐字节读回来：

```
编码结果: 08 2a 12 05 61 6c 69 63 65

08        tag      字段 1，wire type 0（varint）
2a        值       42
12        tag      字段 2，wire type 2（length-delimited）
05        长度     5
61 6c 69 63 65     "alice"
```

**那九个字节里没有任何地方有字段名。** 那里有的是一个数字、一个描述值形状的编码方式、以及那个值。

**四种 wire type 干完了全部的活**，认识它们就够读任何一条消息：

| 类型 | 含义 |
|---|---|
| 0 | varint —— 整数、布尔、枚举 |
| 1 | 64 位定长 |
| 2 | 长度限定 —— 字符串、字节、嵌套消息、打包数组 |
| 5 | 32 位定长 |

**而后果就是这一篇的第一性原理：**

> **线上没有任何东西说明一个字段是干什么的。一个字段号是一个指向 schema 的地址，而那份 schema 由两边各自持有。**

### 第三部分：同一串字节，两个 schema，两种含义

实测，四个字节交给两份不同的 schema：

```
线上:   08 07 10 01

schema A：字段 1 是 user_id，字段 2 是 is_admin   -> {user_id: 7, is_admin: 1}
schema B：字段 1 是 order_id，字段 2 是 quantity  -> {order_id: 7, quantity: 1}
```

**两者都解析成功，而且两者都不算错。** 数字与类型对得上；名词来自 schema。

**这就让 `.proto` 成为一条安全边界，而不是一个构建产物。** 能改它的人决定了消息是什么意思，而由此推出来的检查是：

**发送方与接收方的版本可以不同，而且协议就是为此设计的。** 这正是这个格式的意义 —— 旧客户端与新服务端必须能互操作 —— 所以版本不一致是正常的，问题在于每一方拿到一个对方另有含义的字段时会怎么做。

**而复用一个字段号是唯一会打破这份保证的改动。** 一个已经发布出去的号码永久属于它的含义：

> **复用一个退役的字段号不是"回收一个标识符"。它是在告诉每一个还跑着旧 schema 的客户端：这个新字段就是那个旧字段。**

**这就是这个格式有 `reserved` 声明的原因**，也是为什么"删掉一个字段、再加一个改了名字但同号的字段"是两个版本的测试都抓不住的那种 bug。

**而这也正是发现那一篇对二进制协议的应用。** 没有 schema 就读不了流量；有了 schema，一切都可读。**一个开着 reflection 的服务把 schema 交给任何开口要的人**，那是一个正当的开发特性，而对任何面向互联网的东西来说，是一个值得明确做出的决定。

### 第四部分：顺序、重复，以及没人声明过的字段

关于这个格式的三条性质，在任何东西被哈希、被签名或者被检查时都要紧。

**字段顺序不携带含义。** 实测，同样两个字段的两种顺序：

```
08 2a 12 05 61 6c 69 63 65   -> {id: 42, name: "alice"}
12 05 61 6c 69 63 65 08 2a   -> {id: 42, name: "alice"}
```

**字节不同，消息相同。** 所以：

> **对一条序列化后的 protobuf 消息做哈希，哈希的是一个编码器所做的选择，而不是那条消息。** 拿这些字节去签名或去重，比较的是一个表示，而不是一个值。

**一个字段号可以出现多次。** 实测，把字段 2 先后发成 `1` 和 `99999`：

| 解码器 | 值 |
|---|---|
| 取最后出现的（规范如此规定） | **`99999`** |
| 取最先出现的（某些实现） | **`1`** |

**这与 webhook 那一篇实测到的重复 JSON 键是同一个形状** —— 一串字节、两个值、两者都说得通，而对于应用究竟据哪一个行事没有任何共识。**差别在于这里规范说了"取最后"，所以取最先的接收方是与规范脱节，而不是在行使一种被允许的歧义。**

**而未知字段是被跳过、而不是被拒绝。** 实测，一条带着未声明字段号的消息，在不认识它的 schema 下照样解析：

```
线上: 08 2a 48 b9 60 12 05 61 6c 69 63 65    （字段 9 不在 schema 里）
解析: {id: 42, name: "alice"}
```

**那是前向兼容，而它也是要小心对待的那条性质。** 一个只看解析后对象的检查，看到的东西比到达的东西少 —— 而它没看见的那些字段，对任何重读字节的东西来说仍然在那里。

### 第五部分：一串字节，一个值或三个值

这一组最能说明"schema 说了算"在实践中是什么意思。字段 3 出现了三次：

```
线上: 18 0a 18 14 18 1e

标量 schema 下:    {amount: 30}
repeated schema 下: {amounts: [10, 20, 30]}
```

**同一串字节，而"读到一个值"与"读到三个值"之间的差别，是写在一个文件里的一句声明，而那个文件并不跟着它们一起走。**

**所以基数是线上兼容的。** 把一个字段从单值改成列表，在二进制层面是一次向后兼容的改动 —— 而这恰恰是它值得被注意的原因：

> **"线上兼容"不等于"语义兼容"。一个变成列表的字段对每个旧客户端来说照样解析，而它现在只看到最后一个元素。**

**而在评审真正要紧的那个方向上**：如果一个标量与它的列表是同一串字节，那么"根据标量做决定的"与"根据列表做决定的"就是在对同一个请求做决定、而给出不同答案 —— 差别在于每一方编译的是哪份 `.proto`。

### 第六部分：同一个数字不止一种编码

varint 是一个带延续位的小端 base-128 数字，这意味着**小数字有一个短形式，还有一个同样合法的长形式。** 实测，值 1：

```
规范编码:  01                  -> 1
超长编码:  81 80 80 80 00      -> 1
```

**同一个值，五个字节而不是一个，而两者都正确解码。** 所以一个值在线上有多种表示：

```
08 01 12 03 70 61 79              -> {id: 1, name: "pay"}
08 81 80 80 80 00 12 03 70 61 79  -> {id: 1, name: "pay"}
```

**解析结果相同，字节不同。** 这是不要拿序列化后的字节去做相等判断、缓存键或重放防护的第二个理由 —— 也是"覆盖字节的签名"强于"覆盖消息的签名"的理由：

> **一个覆盖消息的签名，是覆盖它的某一种编码。同一条消息的任何另一种合法编码，对那个签名来说都是一个不同的请求。**

**规范形式正是为此存在的**，它的定义就是让两个实现产生同一条消息时产生同样的字节。**它不是默认编码**，而这个区别就是全部要点。

### 第七部分：帧层，长度是发送方写的

gRPC 把消息放进一个五字节的前缀里。实测，一条消息：

```
帧: 00 00 00 00 02 08 2a

第 1 字节     0 = 未压缩（1 表示压缩）
第 2-5 字节   2 = 消息长度，大端
之后          2 字节消息体
```

**关于那个头，三件事要紧。**

**长度是发送方写的。** 实测，那个字段是四个字节、允许 `0xffffffff`，所以一个照它分配缓冲区的接收方，是在按一个不可信的数字分配缓冲区。**修法是在分配之前强制一个最大消息尺寸** —— 每个实现都有这个设置，而很少有部署去审它。

**压缩标志也是发送方写的**，这就让解压成为一个由请求驱动的步骤 —— 与这个系列里归档与压缩那几篇是同一个形状：一个小小的输入，展开成一个大大的输出。

**而一条消息不是一个帧。** 流式意味着一条逻辑消息可以跨多个帧、一个帧也可以只装其中一部分，所以**"一个请求就是一个帧"不是一条可以用来做检查的性质** —— 一个带长度前缀的流必须被重新拼装，而限制就属于拼装那一步。

**而状态码是在 trailer 里到达的。** 在 gRPC 里，结果是 HTTP/2 的 trailer、不是响应体，这意味着一个剥掉 trailer 的中间设备会把一次失败变成一次看起来成功、而 body 为空的东西。**那不是对方的 bug**；那是这个协议遇到了为另一个协议写的设备。

### 第八部分：从这些机制推出的安全观念

**文本格式那几族注入不适用，而别的东西适用。** 这里没有引号可以闭合、没有分隔符可以被弄混、也没有任何办法让一个值改变消息的结构 —— 结构是一个数字。**所以一个请求无法靠往字符串里塞字符，被弄得表达它本来没有表达的意思。**

**但字符串照样在走。** 一个 protobuf 字符串字段是一串字节，它到达应用代码，从那里到达一条 SQL 语句、一条 shell 命令或者一个模板。**注入在下游、在一直以来的那个位置**，而讲那几族的篇目原样适用 —— 不同的只是这条流水线上的这一环不是它发生的地方。

**取而代之的是一个关于 schema 的问题。** 实测，一串字节在两个 schema 下是两条不同的消息，一个字段是一个值还是三个值取决于它的声明，一个数字有多种编码。**这些每一样都由一个文件决定、而不是由流量决定**，这就把评审放进了代码仓库、而不是请求日志：

**字段号是永久的。** 一个被复用的号码，是对每一个还没重新编译过的客户端的无声谎言。

**基数的改动是线上兼容、而非语义兼容的。** 一个变成列表的字段在哪里都照样解析。

**规范编码不是默认的**，所以字节是一个表示，而消息才是那个值。

**而同一个服务常常不止一种方式可达。** HTTP/2 上的 gRPC、浏览器发起的 gRPC-Web、以及同一批方法的 JSON 转码 —— **同一个实现的三扇前门，而访问规则通常只是照其中一扇写的。** 那是网关那一篇的发现换了一个协议：规则挂在规则编写者认得的东西上，而一个方法写在路径里的二进制协议很容易不被认出来。

**而凭据走在 metadata 里**，那是 HTTP/2 的头块 —— 所以认证那一篇适用，只多一条：**metadata 是按调用、而不是按连接的**，而一条建立一次的长连接可能把一次授权决定带过它的整个生命期。

### 检测与缓解

- **把 `.proto` 的改动当成接口改动来评审，而不是当成代码改动。** 一个被复用的已删除字段号、或者一个变成 repeated 的标量，是一次在两个版本上都能干净编译的行为改动。
- **每一个退役的字段号都要求 `reserved`**，并在 schema 做版本管理的同一个地方检查它。
- **对生产环境里开启 reflection 告警**，因为它把 schema 交给任何开口要的人，也移除了唯一让流量不被读懂的东西。
- **设置并评审一个最大消息尺寸**，因为实测那个帧头允许发送方声明到四个 GB。
- **对"没有东西需要压缩却设了压缩标志"告警**，因为它让解压成一个由请求驱动的步骤。
- **盯被中间设备丢掉或被改写的 trailer**，因为被剥掉的 trailer 会把一次失败的调用变成一次看起来成功的空调用。
- **检查每一扇前门是否有同样的规则** —— HTTP/2 服务、浏览器传输方式、以及同一批方法的任何 JSON 转码。
- **缓解上，不要拿序列化后的字节做哈希、缓存键或重放防护。** 需要的话用一个规范编码，或者对要紧的那些字段签名。
- **在需要签名或摘要的地方，明确它是覆盖字节还是覆盖值**，因为实测到的超长编码与字段顺序无关，都是两者可以不同的方式。
- **在字符串"不再是 protobuf"的那条边界上校验它们**，也就是注入那几族适用的地方，而不是试图去校验一个没有语法可滥用的格式。
- **按方法、按调用授权，而不是按连接**，这样一条长连接不会把一次决定带过它本该被重新做出的那个时点。
- **并且把 `.proto` 文件当成攻击面的一部分**：实测，它们是唯一决定这些流量是什么意思的东西。
