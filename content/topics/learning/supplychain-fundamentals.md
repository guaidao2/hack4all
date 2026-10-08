---
id: supplychain-fundamentals
title_en: "Supply Chain and CI/CD — Where Your Code Actually Came From"
title_zh: "供应链与 CI/CD：你的代码实际从哪里来"
summary_en: A repository you reviewed is not the same as a build you can account for. Measured on a real project — seven direct dependencies become thirty-six modules, checksums cover the second download but not the first, and every action reference is a floating tag.
summary_zh: 你审查过的那个仓库，与你能够交代的那个构建不是同一样东西。在一个真实项目上实测 —— 七个直接依赖变成三十六个模块；校验和管住了第二次下载、管不住第一次；而每一个 action 引用都是浮动的 tag。
tags: [web, supply-chain, cicd, dependencies, cwe-1104, cwe-1357]
tools: [go, git, gh, syft]
attck: [T1195, T1195.001, T1195.002]
platform: [web, cicd]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The repository is not the build

Everything else in this guide is about a request reaching an application. This entry is about a different question: **what code is actually in the application**.

> **A supply chain attack does not break your code. It breaks where your code came from.**

Which means the reassuring statement — "I did not write that, and it is not in my repository" — is not reassuring at all. The attack surface is **outside** the repository: in the dependency graph, in the build pipeline, and in the channel that delivers the artifact. A review of the source can be thorough and complete and still not cover it, because most of what runs was never in the source.

### Four layers

1. **The trust boundary.** The application trusts that the source a person reviewed is what will run. What actually runs is that source plus several hundred modules plus whatever the pipeline did to it.
2. **Data and instruction share a plane.** A dependency name is simultaneously **an identifier for what to download** and **an authorisation to execute somebody's code**. One string decides both, and nothing about a valid-looking name says anything about whose code is behind it.
3. **Why the usual fix fails.** Reviewing your own code covers the part that was never the problem. Popularity is a proxy for attention, not for integrity — and the packages worth attacking are exactly the popular ones. Neither measure touches the pipeline.
4. **The variants.** A name that is one character off; a private package name that also exists publicly; a maintainer account taken over while the package stays identical; a transitive dependency nobody chose; an install-time script; a privileged pipeline; a shared cache; and an artifact channel that can be altered after the build.

### Measured: the dependency graph is mostly other people's decisions

On a real project:

| | Count |
|---|---|
| Dependencies written in `go.mod` | **22 entries** |
| Of those, **direct** — the ones the project actually imports | **7** |
| Of those, **indirect** — pulled in by those seven | **15** |
| Modules in the build graph (`go list -m all`) | **36** |

**Seven direct dependencies produce a build graph of thirty-six modules**, and 42% of the declared entries are indirect — chosen by a dependency rather than by the project. The seven are visible at a glance and could be read; the other twenty-nine are the part that gets reviewed by trust.

**This ratio is not unusual.** A project that thinks of itself as "we have seven dependencies" is running code from thirty-six places, and the gap widens with every framework. The practical conclusion is not "review all thirty-six" — it is that **the question to ask about a dependency is not only what it does, but who would have to be compromised for it to change**.

### Measured: what integrity tooling does and does not cover

Same project:

```
go.sum lines       : 51
go mod verify      : all modules verified
```

**A lock file's checksum protects the second download, not the first.** It ensures the code fetched today is byte-identical to the code fetched when the entry was created — which defends against an upstream rewriting a published version, a real and recurring attack. What it cannot do is tell you whether the version you pinned **the first time** was the right one, because at that moment there was nothing to compare against.

So integrity has two moments, and tooling covers one:

| Moment | Protected by |
|---|---|
| The first time a dependency is introduced | **review, and only review** |
| Every subsequent fetch | the lock file's checksum |

That is why "the lock file is committed" answers a different question than "this dependency is safe to use", and why both belong in a review.

### Measured: the pipeline's own settings

The same review, on CI configuration:

| Setting | What was found | Why it matters |
|---|---|---|
| Action references | **every one a floating tag** (`@v4`, `@v5`, `@v6`), none pinned to a commit | A tag can be moved; "what we tested" and "what runs tomorrow" are then not the same artifact |
| Job permissions | one workflow declared `contents: write`, one declared nothing | An undeclared permission set means the repository default, historically read-write |
| Trigger events | `push`, `pull_request`, `workflow_dispatch`, and tag pushes | Which events run with which credentials is the thing to read first |

**The floating-tag row is the one to understand rather than memorise.** Pinning an action to a full commit hash makes "we reviewed this version" a **verifiable** claim; a major-version tag makes it a claim about whoever controls that tag at the time the job runs. That is the same shape as the subdomain takeover entry — a name that no longer necessarily refers to what you checked.

**And `pull_request_target` deserves its own warning**, because it is a trigger designed to run in the privileged context of the base repository while being initiated by a pull request. A workflow that uses it and then checks out the **pull request's** code has handed the base repository's credentials to code nobody has merged — and the fix is either not to check out untrusted code under that trigger, or to run the checkout with no credentials available.

### The same problem, per ecosystem

The mechanism differs enough between languages that the mitigations do not transfer:

| Ecosystem | Install-time code execution | Lock file |
|---|---|---|
| **npm** | **yes** — `postinstall` and friends | `package-lock.json` |
| **pip** | **yes** — `setup.py` runs at install | `requirements.txt`, and it needs hashes to be a lock file |
| **Go** | **no** — modules are not executed at install | `go.sum` |
| Maven / Gradle | yes, via plugins | present |
| RubyGems | yes, for native extensions | `Gemfile.lock` |

**"Installing a package" means "executing code" in npm and pip, and means "downloading code" in Go.** That is a genuine and underrated advantage of the Go module system: a build that fetches dependencies does not run their install logic, so the step that turns a supply chain compromise into immediate execution is absent. In an ecosystem where install scripts run, that step is present by design and has to be treated as such.

### Detection and mitigation

- **Treat every change to a lock file as a change worth reading.** It is the most direct detector available: an unexpected entry, a version bump nobody requested, or a new transitive dependency in a diff that was supposed to be a one-line fix.
- **And treat every change to CI configuration as a high-privilege change.** A workflow file can grant itself credentials, so a diff there is not a routine edit — it is a change to who may do what.
- **Alert when a dependency's publisher or ownership changes, and on a new version after a long silence.** A package that is identical in code and different in who publishes it is the shape of an account takeover, and it is not visible in a dependency diff.
- **Verify published artifacts against a signature, where the ecosystem provides one.** For a project that ships binaries, this is the only control that covers the delivery channel rather than the source.
- **For mitigation, commit lock files and verify checksums in the build.** This is the control with the best ratio of effort to coverage: it makes an upstream rewrite detectable, and it costs one file per ecosystem.
- **Pin CI actions to commit hashes, and declare minimal permissions.** Two lines per workflow, and they convert two implicit trusts into explicit ones — checkable by reading the file.
- **Keep the credentials that publish separate from the credentials that build.** A release key that is present during every build is a release key that any code path in the build can use; making publication a distinct, narrower step reduces what a compromised build can do.
- **Do not run untrusted code in a privileged pipeline context.** The `pull_request_target` case is the concrete instance; the general rule is that the job which has secrets should not be the job which executes a contribution.
- **Generate a bill of materials, and keep it with the artifact.** An SBOM does not prevent anything; it makes the question "were we affected" answerable in minutes instead of days, which is the difference between a disclosure being an incident and a fire drill.
- **And keep the closing rule in mind, because it explains why this class is under-attended.** Most of its mitigations live in **configuration and process** rather than in application code — the same property that makes the subdomain takeover entry invisible to a code review. **Nothing in the application is wrong, which is exactly why the review of the application does not find it.**

<!-- lang:zh -->
### 仓库不是构建产物

这份指南的其他部分讲的是一个请求如何到达应用。这一篇讲的是另一个问题：**应用里实际装的是哪些代码**。

> **供应链攻击不攻破你的代码。它攻破的是"你的代码从哪里来"。**

这意味着那句让人安心的话 —— "这个不是我写的，也不在我的仓库里" —— 一点都不让人安心。攻击面在仓库**之外**：在依赖图里、在构建流水线里、在把制品送达的那个通道里。对源码的审查可以彻底而完整，却仍然覆盖不到它，因为实际运行的东西大部分从来不在源码里。

### 四层

1. **信任边界。** 应用信任"有人审查过的那份源码，就是将运行的东西"。而实际运行的是那份源码，加上几百个模块，加上流水线对它做过的一切。
2. **数据与指令共用同一平面。** 一个依赖名同时是**"要下载什么"的标识符**和**"执行谁的代码"的授权**。一个字符串同时决定这两件事，而一个看起来正常的名字，对背后是谁的代码什么都没说。
3. **为什么常见修法失败。** 审查你自己的代码，覆盖的是从来不是问题的那部分。流行度是"关注度"的代理，不是"完整性"的代理 —— 而值得被攻击的包恰好就是那些流行的。两个指标都碰不到流水线。
4. **变体。** 差一个字符的名字；一个同时也存在于公共仓库的私有包名；一个被接管的维护者账号（而包本身一字未改）；一个没人主动选择的传递依赖；一个安装期脚本；一条有权限的流水线；一个共享的缓存；以及一个在构建之后仍可被改动的制品通道。

### 实测：依赖图大部分是别人的决定

在一个真实项目上：

| | 数量 |
|---|---|
| `go.mod` 里写下的依赖条目 | **22 条** |
| 其中**直接**依赖 —— 项目真正 import 的那些 | **7** |
| 其中**间接**依赖 —— 被那七个拉进来的 | **15** |
| 构建图里的模块（`go list -m all`） | **36** |

**七个直接依赖产生了一张三十六个模块的构建图**，而声明的条目里有 42% 是间接的 —— 由一个依赖选择，而不是由这个项目选择。那七个一眼可见、也确实读得完；另外二十九个，是靠信任被审查过去的那部分。

**这个比例并不特殊。** 一个自认为"我们有七个依赖"的项目，正在运行来自三十六个地方的代码，而每引入一个框架，这个差距就变宽一次。实用的结论不是"把那三十六个都读一遍" —— 而是**关于一个依赖该问的问题，不只是它做什么，还有"要让它的行为改变，谁必须被攻破"**。

### 实测：完整性工具管住了什么、没管住什么

同一个项目：

```
go.sum 行数   : 51
go mod verify : all modules verified
```

**锁文件的校验和保护的是第二次下载，不是第一次。** 它保证今天取到的代码与那条记录被创建时取到的代码逐字节一致 —— 这防的是上游重写一个已发布版本，那是一种真实且反复出现的攻击。它做不到的是告诉你，你**第一次**锁定的那个版本是不是对的那一个，因为那一刻没有东西可以比对。

所以完整性有两个时刻，而工具只覆盖其中一个：

| 时刻 | 由什么保护 |
|---|---|
| 一个依赖被首次引入时 | **审查，而且只有审查** |
| 之后每一次取用时 | 锁文件的校验和 |

这就是为什么"锁文件提交了"回答的问题与"这个依赖可以安全使用"不是同一个，也为什么两者都属于一次评审。

### 实测：流水线自己的设置

同一次盘点，看 CI 配置：

| 设置 | 看到的情况 | 为什么要紧 |
|---|---|---|
| action 引用 | **全部是浮动 tag**（`@v4`、`@v5`、`@v6`），没有一个 pin 到 commit | tag 可以被移动；于是"我们测过的东西"与"明天跑的东西"不是同一个制品 |
| 任务权限 | 一个工作流声明了 `contents: write`，一个什么都没声明 | 未声明意味着仓库默认值，而历史上是读加写 |
| 触发事件 | `push`、`pull_request`、`workflow_dispatch`，以及 tag 推送 | 哪些事件带着哪些凭据运行，是第一个要读的东西 |

**浮动 tag 那一行值得弄懂，而不是背下来。** 把一个 action pin 到完整的 commit 哈希，让"我们审查过这个版本"成为一个**可验证**的说法；而一个主版本 tag 让它变成一个关于"运行那一刻谁控制着那个 tag"的说法。这和子域接管那篇是同一个形状 —— 一个不再必然指向你检查过的那个东西的名字。

**而 `pull_request_target` 值得单独警告**，因为它是一个被设计成"在基仓库的特权上下文里运行、却由一次 pull request 发起"的触发器。一个用了它、然后去 checkout **那个 pull request 的代码**的工作流，等于把基仓库的凭据交给了一段没人合并过的代码 —— 修法要么是在那个触发器下不 checkout 不受信任的代码，要么让那次 checkout 在没有任何凭据可用的前提下运行。

### 同一个问题，按生态各不相同

机制在语言之间差别足够大，以至于缓解措施不能互相搬运：

| 生态 | 安装期执行代码 | 锁文件 |
|---|---|---|
| **npm** | **有** —— `postinstall` 之类 | `package-lock.json` |
| **pip** | **有** —— `setup.py` 在安装时运行 | `requirements.txt`，而且要有哈希才算锁文件 |
| **Go** | **没有** —— 模块在安装时不被执行 | `go.sum` |
| Maven / Gradle | 有，经由插件 | 有 |
| RubyGems | 有，原生扩展 | `Gemfile.lock` |

**"装一个包"在 npm 与 pip 里意味着"执行一段代码"，在 Go 里意味着"下载一段代码"。** 那是 Go 模块系统一项真实而且被低估的优势：一次取依赖的构建不会运行它们的安装逻辑，于是"把一次供应链失陷变成立即执行"的那一步不存在。在一个安装脚本会运行的生态里，那一步是设计的一部分，必须被当成设计的一部分来对待。

### 检测与缓解

- **把每一次锁文件的改动都当成值得读的改动。** 这是最直接的检测器：一个预期之外的条目、一次没人要求过的版本跳升、或者一个出现在"本该只有一行修复"的 diff 里的新传递依赖。
- **并且把每一次 CI 配置的改动都当成高权限改动。** 一个工作流文件可以给自己授予凭据，所以那里的 diff 不是例行编辑 —— 它是对"谁可以做什么"的改动。
- **当一个依赖的发布者或归属发生变化、以及在长时间沉寂之后出现新版本时告警。** 一个代码完全相同、而发布它的人变了的包，就是账号被接管的形状，而它在依赖 diff 里看不见。
- **在生态提供签名的地方校验已发布的制品。** 对一个分发二进制的项目来说，这是唯一一项覆盖"送达通道"而不是"源码"的控制。
- **缓解上，提交锁文件并在构建中校验校验和。** 这是投入产出比最好的一项控制：它让上游重写变得可检测，而代价是每个生态一个文件。
- **把 CI 的 action pin 到 commit 哈希，并声明最小权限。** 每个工作流两行，而它们把两项隐式信任变成显式 —— 读那个文件就能核对。
- **把"发布用的凭据"与"构建用的凭据"分开。** 一把在每次构建时都在场的发布密钥，就是一把构建里任何代码路径都能用的发布密钥；把发布做成一个独立、更窄的步骤，缩小了"构建被攻破"能造成的事。
- **不要在带权限的流水线上下文里运行不受信任的代码。** `pull_request_target` 是那个具体的实例；通则是有密钥的那个任务，不该是执行一份贡献的那个任务。
- **生成一份物料清单，并让它跟着制品走。** SBOM 什么也不阻止；它让"我们受影响了吗"这个问题能在几分钟而不是几天内被回答，而那就是一次披露是一场事故还是一场虚惊之间的差别。
- **并且记住那句收束的话，因为它解释了为什么这一类被关注得不够。** 它的缓解措施大多住在**配置与流程**里，而不是应用代码里 —— 与子域接管那篇一样，正是这个性质让代码评审看不见它。**应用里没有任何东西是错的，而这恰好就是"对应用的评审"找不到它的原因。**
