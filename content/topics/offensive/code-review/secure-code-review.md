---
id: secure-code-review
title_en: Source Code Review
title_zh: 代码审计
summary_en: White-box review finds in an hour what black-box testing finds in a week, because you can follow the data instead of guessing at it. The skill is not reading every line, it is knowing which lines can possibly matter.
summary_zh: 白盒审计一小时能找到的东西，黑盒测试可能要一周 —— 因为你能跟着数据走，而不是靠猜。这门技能不在于读完每一行，而在于知道哪些行**可能**有关系。
tags: [code-review, sast, whitebox, semgrep, codeql, appsec]
tools: [Semgrep, CodeQL, ripgrep, SonarQube, Snyk Code]
attck: [T1190]
platform: [any]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Read with a question, not from the top

Reading a codebase front to back is a waste of a week. The productive approach is to hold one question and walk the code looking for its answer:

- "Where does user input reach a query?"
- "Which endpoints skip the authorization check the others have?"
- "What happens if this field is null, negative, or a list?"

That is why a map matters more than a reading order:

1. **Entry points** — routes, controllers, message consumers, scheduled jobs, CLI handlers. Anything callable.
2. **Data flows** — how a request's values move through the code, and what transforms them.
3. **Sinks** — the functions where data becomes dangerous: a query, a command, a file path, a template, a deserialiser.

Then a finding is a path from 1 to 3 with nothing in between that validates the data.

### The sinks worth knowing by language

| Category | Dangerous shapes |
|---|---|
| SQL | String concatenation into a query, `query("... " + x)`, ORM raw fragments, `ORDER BY` built from input |
| Command | `os.system`, `subprocess(..., shell=True)`, `Runtime.exec`, backticks, `child_process.exec` |
| Files | `open()` with a user-controlled path, `sendFile`, `include`/`require`, `path.join` with user input |
| Deserialisation | `unserialize`, `pickle.loads`, `yaml.load`, `readObject`, `BinaryFormatter` |
| Templates | `render(string)`, `eval`, `new Function`, `Template()` with input |
| SSRF | `requests.get(url)`, `file_get_contents($url)`, any HTTP client taking a full URL from input |
| Redirects | `redirect(userInput)`, `Location` built from a parameter |
| Crypto | `md5`/`sha1` for passwords, `rand()`/`Math.random()` for tokens, ECB mode |

The same list applies everywhere; the names change. Learning one language's set well makes the next language cheap.

### Finding the paths that matter

**Grep for sources, then read outward.** Searching for the framework's request object (`req.`, `$request->`, `params[`) and reading each hit is faster than reading files.

**Grep for sinks, then read inward.** `rg 'execute\(|query\(|innerHTML'` gives you a list of places worth understanding. Dozens, not thousands.

**Diff the endpoints.** The most reliable manual technique for authorization bugs: find five endpoints that do the same kind of thing, and check whether all five perform the same check. The one that does not is the finding. This is how missing authorization is found in practice, because absence is invisible to a scanner.

**Follow the data through transforms.** A value that passes through `intval()` is not a string injection candidate; a value that goes through `htmlspecialchars()` and then into a SQL query is still a SQL injection. Track what each step guarantees.

**Read the commits.** `git log -p --grep='fix\|security\|CVE'` shows how this team has failed before, and the same mistake is usually still present elsewhere. A patch that fixes one call site and leaves three is the norm.

**Look at the tests.** Code with no test is code nobody thought carefully about, and coverage gaps in an authorization module are worth a second look.

**Search for the smells.** `TODO`, `FIXME`, `HACK`, `XXX`, commented-out permission checks, catch blocks that swallow exceptions, and `if debug` branches are all cheap leads.

### What tools do well, and what they cannot

Semgrep and CodeQL are excellent at the mechanical half: taint tracking across known patterns, dependency misuse, hardcoded secrets, and the sort of sink/source pairing that a human would take hours to spot. Run them first, on every commit.

What they are poor at, and where you should spend your time:

- **Missing authorization**, because there is nothing to match — the check simply is not there.
- **Business logic flaws**, which are correct code implementing a wrong rule.
- **Cross-service and cross-language flows**, where the source is in one repository and the sink in another.
- **Race conditions and state machines**, which are properties of sequences rather than of lines.

```bash
# the two that are worth wiring into CI
semgrep --config auto --severity ERROR .
codeql database create db --language=javascript && codeql database analyze db
```

### A review checklist that survives contact

- Every route: is authentication required, and is it **enforced** rather than assumed from a middleware list?
- Every object fetch: is ownership checked, or only the role?
- Every state change: is it idempotent, and does it verify the object is in the expected state?
- Every input: is its type constrained, and are its bounds checked?
- Every secret: is it from configuration, and is it logged anywhere?
- Every error path: does it fail closed, and does it leak detail?

### Detection, from the building side

Code review is a control, not just a test:

- **SAST in CI** on every pull request, with the rule set reviewed rather than enabled and ignored.
- **A review checklist** that names authorization explicitly, because reviewers default to style and correctness.
- **Secret scanning** on commits and on the repository history.
- **A dependency review step** for new packages, which is where the supply chain entry picks up.
- **Track which findings came from which technique** — if everything comes from the scanner, the manual review is not happening, and the logic bugs are going unread.

### Mitigation

- **Make the secure pattern the easy one.** A framework helper that parameterises by default beats a memo telling people to be careful.
- **Centralise authorization** (policy middleware that fails closed) so a new route cannot forget it.
- **Turn findings into tests.** Every bug found in review becomes a regression test, and the test is what actually prevents the repeat.
- **Review the security-relevant paths first**: authentication, authorization, payment, file handling, deserialisation, template rendering. Most code does not need this level of attention.
- **Budget the time.** A review that is squeezed into the end of a sprint produces style comments, not findings.

<!-- lang:zh -->
### 带着问题读，而不是从头读到尾

把一个代码库从前到后读完，是浪费一周。有效的做法是手里握着一个问题，沿着代码去要答案：

- "用户输入在哪里到达了查询？"
- "哪些接口缺了其他接口都有的授权检查？"
- "如果这个字段是空、负数、或者一个列表，会发生什么？"

所以一张地图比一个阅读顺序更重要：

1. **入口点** —— 路由、控制器、消息消费者、定时任务、CLI 处理器。一切可被调用的东西。
2. **数据流** —— 请求里的值怎么在代码中移动，中间经过了什么变换。
3. **危险汇聚点（sink）** —— 数据在那里变危险：查询、命令、文件路径、模板、反序列化。

于是漏洞就是"从 1 到 3 的一条路径，中间没有任何东西对数据做过校验"。

### 各语言都该认识的 sink

| 类别 | 危险形态 |
|---|---|
| SQL | 字符串拼进查询、`query("... " + x)`、ORM 原始片段、用输入拼 `ORDER BY` |
| 命令 | `os.system`、`subprocess(..., shell=True)`、`Runtime.exec`、反引号、`child_process.exec` |
| 文件 | 用户可控路径的 `open()`、`sendFile`、`include`/`require`、用用户输入 `path.join` |
| 反序列化 | `unserialize`、`pickle.loads`、`yaml.load`、`readObject`、`BinaryFormatter` |
| 模板 | `render(字符串)`、`eval`、`new Function`、用输入调 `Template()` |
| SSRF | `requests.get(url)`、`file_get_contents($url)`、任何从输入拿完整 URL 的 HTTP 客户端 |
| 跳转 | `redirect(用户输入)`、用参数拼 `Location` |
| 加密 | 口令用 `md5`/`sha1`、token 用 `rand()`/`Math.random()`、ECB 模式 |

这张表到处适用，只是名字不同。把一种语言的这套东西学透，下一种语言就很便宜。

### 找到真正要紧的路径

**先 grep source，再往外读。** 搜框架的请求对象（`req.`、`$request->`、`params[`）并逐个读命中点，比读文件快得多。

**再 grep sink，往里读。** `rg 'execute\(|query\(|innerHTML'` 给你一份值得理解的清单 —— 是几十个，不是几千个。

**横向对比接口。** 找授权类漏洞最可靠的手工技巧：找出五个做同类事的接口，检查是不是五个都做了同样的校验。缺的那个就是发现。现实中的"授权缺失"就是这么找出来的，因为**缺失**对扫描器是不可见的。

**跟着数据穿过变换。** 经过 `intval()` 的值不再是字符串注入候选；经过 `htmlspecialchars()` 再进 SQL 查询的，仍然是 SQL 注入。要跟踪每一步保证了什么。

**读提交记录。** `git log -p --grep='fix\|security\|CVE'` 会告诉你这个团队以前在哪儿摔过，而同样的错误通常还留在别处。修了一处、漏了三处，是常态。

**看测试。** 没有测试的代码是没人仔细想过的代码；授权模块上的覆盖空白值得多看一眼。

**搜味道。** `TODO`、`FIXME`、`HACK`、`XXX`、被注释掉的权限检查、吞掉异常的 catch、`if debug` 分支，都是便宜的线索。

### 工具擅长什么，做不了什么

Semgrep 和 CodeQL 在机械的那一半非常强：已知模式上的污点追踪、依赖误用、硬编码密钥，以及人类要花几小时才能看出的 source/sink 配对。先跑它们，每次提交都跑。

它们很差的地方，也就是你该花时间的地方：

- **授权缺失** —— 没有东西可匹配，因为那个检查根本不存在。
- **业务逻辑缺陷** —— 代码是对的，规则是错的。
- **跨服务、跨语言的流** —— source 在一个仓库，sink 在另一个。
- **竞态与状态机** —— 它们是序列的性质，不是某一行的性质。

```bash
# 值得接进 CI 的两个
semgrep --config auto --severity ERROR .
codeql database create db --language=javascript && codeql database analyze db
```

### 一份经得起用的复查清单

- 每条路由：是否要求认证，而且是**真正强制**，而不是从中间件清单里推定？
- 每次取对象：检查了归属，还是只检查了角色？
- 每次状态变更：是否幂等，是否校验了对象处于预期状态？
- 每个输入：类型是否受限，边界是否检查？
- 每个密钥：是否来自配置，是否被记进了任何日志？
- 每条错误路径：是否安全失败，是否泄漏细节？

### 检测（从建设方角度）

代码审计是一种控制手段，不只是测试：

- **CI 里对每个 PR 跑 SAST**，并且规则集要有人审，而不是开了就不管。
- **复查清单里明确写出"授权"**，因为复查者默认关注风格和正确性。
- **提交和仓库历史上做密钥扫描**。
- **新引入依赖走一道审查**，这正是供应链那篇接上的地方。
- **统计发现来自哪种手段** —— 如果全都来自扫描器，说明人工复查没有真正发生，逻辑漏洞没人读。

### 缓解

- **让安全的写法成为最省事的写法。** 一个默认参数化的框架辅助函数，胜过一份提醒大家要小心的备忘录。
- **集中授权**（默认拒绝的策略中间件），这样新路由不会漏。
- **把发现变成测试。** 复查中发现的每个 bug 都变成一条回归测试，真正阻止重复发生的是测试。
- **先审安全相关的路径**：认证、授权、支付、文件处理、反序列化、模板渲染。大多数代码不需要这种关注度。
- **给足时间。** 被压缩到冲刺末尾的复查，产出的是风格意见，不是发现。
