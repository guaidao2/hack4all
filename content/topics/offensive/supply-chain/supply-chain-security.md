---
id: supply-chain-security
title_en: Software Supply Chain Security
title_zh: 软件供应链安全
summary_en: The code you did not write is now most of the code you run, and it arrives through registries, build pipelines and update channels designed for convenience rather than trust. The xz backdoor showed how far somebody will go to be inside that trust.
summary_zh: 你没写的代码已经占了你运行的代码的大部分，而它是通过为便利而非为信任设计的仓库、流水线和更新通道进来的。xz 后门事件说明，为了钻进这份信任里，有人愿意走多远。
tags: [supply-chain, dependencies, cicd, sbom, xz, third-party]
tools: [syft, grype, osv-scanner, dependabot, cosign]
attck: [T1195.001, T1195.002]
platform: [any]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### The chain, end to end

An attack can enter at any of these points, and defenders usually watch only the first:

1. **Dependencies** — what you install, directly and transitively.
2. **Build** — the pipeline and the environment that turns source into an artifact.
3. **Distribution** — registries, mirrors, CDNs, the vendor's own download page.
4. **Update** — how the artifact reaches the running system afterwards.

The integrity entry in this guide covers the mechanics of unsigned updates and build pipelines. This entry is about the dependency and maintainer side, which is where the most consequential attacks have happened.

### Where dependencies give way

**Typosquatting.** A package named `reqeusts`, `lodahs` or `python-dateutil` when the real one is `python-dateutil` — published with a payload in an install script. It works because developers type fast and lockfiles are not always used.

**Dependency confusion.** A company publishes internal packages under a scope that does not exist publicly. An attacker registers that name on the public registry with a higher version number, and the build tool prefers the higher version. The build then installs attacker code with the company's credentials available to it.

**Account takeover of an existing package.** Phishing a maintainer, buying a dormant package, or taking over one whose maintainer has moved on. Existing packages are more valuable than new ones because they are already trusted and already in lockfiles.

**Malicious install scripts.** npm's `postinstall`, Python's `setup.py`, a Gradle plugin that runs at configuration time. The code does not need to be in the library to run on every machine that installs it.

**Transitive dependencies.** You reviewed the ten packages you chose. There are eight hundred in the tree, and one of them is what the attacker compromised.

### The xz backdoor, as a case study

Worth studying in detail, because it is the clearest picture of what a determined supply chain attack looks like.

- **The long game.** An account began contributing to the xz project in 2021, made legitimate improvements, built trust, and eventually became a maintainer with commit rights. The social engineering took years and included pressure on the original maintainer during a period of burnout.
- **The payload.** In 2024, versions 5.6.0 and 5.6.1 shipped with a backdoor in liblzma. It was not in the source in any readable form: it was hidden in a binary test file, extracted by a modified build script (`build-to-host.m4`) only when the package was built by a distribution's build system.
- **The target.** The backdoor hooked into the SSH daemon's authentication path (via libsystemd's use of liblzma), allowing an attacker holding a specific private key to authenticate to any affected machine.
- **The discovery.** An engineer at Microsoft, investigating a 500 ms delay in SSH logins on a Debian test machine, noticed that sshd was using more CPU than expected, traced it, and found the backdoor. Not a scanner, not a static analysis tool. A performance anomaly and curiosity.

The lessons that transfer:

1. **Maintainer trust is a control, and it can be earned slowly by an attacker.** Review the person, not only the diff.
2. **Build scripts are part of the code.** A `.m4` file that assembles and injects content is as dangerous as a source file, and it is reviewed far less.
3. **Non-source artifacts bypass review.** Binary test files, generated files and vendored blobs are where code hides from a diff.
4. **Single-maintainer critical projects are systemic risk.** Many of them are dependencies of everything, maintained by volunteers.
5. **Detection came from operations, not from security tooling.** Baseline behaviour — CPU, latency, unusual file access — catches what pattern matching misses.
6. **Distributions acted fast** because the compromise was found before it reached stable releases. That was luck as much as process.

### Assessing a target from the outside

- Fetch the lockfile or dependency manifest if the project is open source, and look at the transitive tree, not the top level.
- Check whether internal-looking package names exist on the public registry. If they do not, they are claimable.
- Look at the CI configuration: are actions pinned to commit SHAs, is the dependency install reproducible, does the pipeline run code from pull requests?
- Check whether artifacts are signed and whether provenance is published.
- For a product rather than a repository, look at the update mechanism (covered in the integrity entry) and at the vendor's own download path.

### Detection, from the defender's side

- **New network destinations during builds.** A dependency that suddenly talks to an IP is the clearest signal that something is wrong, and almost nobody watches for it.
- **Maintainer and ownership changes** on packages you depend on, which registries expose through their APIs.
- **Build time and resource anomalies**, which is exactly how xz was found.
- **Lockfile diffs that change more than the dependency you intended**, including source URLs and integrity hashes.
- **Postinstall and preinstall scripts** in newly added packages, reviewed before merge rather than after.
- **Unexpected outbound connections from the build agent**, including DNS.

### Mitigation

- **Pin dependencies and commit lockfiles**, and verify integrity hashes so a substituted package fails the build.
- **Use a private registry or proxy** (Artifactory, Nexus, a Go module proxy) and block direct access to public registries from build agents, which removes most dependency confusion at once.
- **Scope internal packages** so their names cannot be claimed publicly.
- **Review new dependencies as code.** A weekly report of added packages with their maintainers, age and install scripts is a cheap control.
- **Generate an SBOM per release** (Syft, cyclonedx) so "are we affected" is a query rather than an investigation.
- **Sign artifacts and publish provenance** so a consumer can verify what they downloaded.
- **Give build agents the least privilege they need** and no production credentials they do not use.
- **Support the critical projects you depend on**, financially or with engineering time. An abandoned dependency is an open door, and the cheapest mitigation is often to pay for maintainership.
- **Watch the build, not just the code.** Network access, install scripts and resource anomalies are where the compromises show up first.

<!-- lang:zh -->
### 这条链，从头到尾

攻击可以从下面任何一点进入，而防守方通常只盯着第一点：

1. **依赖** —— 你直接和间接装进来的东西。
2. **构建** —— 把源码变成产物的流水线与环境。
3. **分发** —— 仓库、镜像站、CDN、厂商自己的下载页。
4. **更新** —— 产物此后怎么到达运行中的系统。

本指南的"完整性"那篇讲的是无签名更新与构建流水线的机制。这一篇讲依赖与维护者这一侧，也是影响最大的那些攻击发生过的地方。

### 依赖在哪些地方失守

**拼写抢注。** 一个叫 `reqeusts`、`lodahs` 的包，或者把 `python-dateutil` 打成别的近似拼写 —— 里面装着安装脚本里的 payload。它能成立，是因为开发者打字快，而 lockfile 不一定总用。

**依赖混淆。** 某公司以某个作用域发布内部包，而该名字在公共仓库并不存在。攻击者在公共仓库注册这个名字并给一个更高的版本号，构建工具偏爱更高版本，于是构建过程带着公司的凭据安装了攻击者的代码。

**接管已有包。** 钓维护者的鱼、买下一个休眠的包，或者接管一个维护者早已离开的包。已有包比新包值钱得多，因为它已经被信任，而且已经在 lockfile 里。

**恶意的安装脚本。** npm 的 `postinstall`、Python 的 `setup.py`、在配置阶段就执行的 Gradle 插件。代码不必在被引用的库里，也能在每台安装它的机器上运行。

**传递依赖。** 你审了自己挑的那十个包。树里有八百个，而被攻陷的是其中之一。

### xz 后门，作为案例

值得细看，因为它是"一个铁了心的供应链攻击长什么样"最清晰的样本。

- **长期经营。** 一个账号从 2021 年起向 xz 项目贡献，做的是真实的改进，逐步建立起信任，最终成为有提交权限的维护者。这场社工持续数年，期间还包括在原维护者倦怠期施加压力。
- **载荷。** 2024 年发布的 5.6.0 与 5.6.1 版本里，liblzma 中被植入了后门。它在源码里没有任何可读形态：藏在二进制测试文件里，由被改过的构建脚本（`build-to-host.m4`）在发行版构建系统构建时才解出并注入。
- **目标。** 后门挂进了 SSH 守护进程的认证路径（经由 libsystemd 对 liblzma 的使用），持有特定私钥的攻击者可以对任何受影响的机器完成认证。
- **发现过程。** 微软的一位工程师在排查 Debian 测试机上 SSH 登录多出的 500 毫秒延迟时，注意到 sshd 的 CPU 占用异常，顺藤摸瓜找到了后门。不是扫描器，也不是静态分析工具 —— 是一个性能异常，加上好奇心。

可迁移的教训：

1. **维护者信任本身就是一种控制，而攻击者可以慢慢把它挣到。** 要审人，不只审 diff。
2. **构建脚本也是代码。** 一个负责拼装和注入内容的 `.m4` 文件，危险程度不亚于源码文件，而它被审阅的次数少得多。
3. **非源码产物绕过审查。** 二进制测试文件、生成文件、vendor 进来的 blob，正是代码躲开 diff 的地方。
4. **单人维护的关键项目是系统性风险。** 其中很多被所有东西依赖，却由志愿者维护。
5. **发现来自运维，而不是安全工具。** 基线行为 —— CPU、延迟、异常文件访问 —— 能抓住模式匹配漏掉的东西。
6. **各发行版反应很快**，因为它进入稳定版之前就被发现了。这里面运气与流程各占一半。

### 从外部评估一个目标

- 项目开源的话，取它的 lockfile 或依赖清单，看**传递树**而不是顶层。
- 检查那些看起来像内部包的名字在公共仓库是否存在。不存在，就意味着可被注册。
- 看 CI 配置：action 是否固定到 commit SHA、依赖安装是否可复现、流水线是否会执行 PR 里的代码。
- 产物是否签名、是否发布 provenance。
- 如果目标是产品而不是仓库，看它的更新机制（完整性那篇有讲）和厂商自己的下载路径。

### 检测（防守方视角）

- **构建期间出现的新网络目标。** 一个依赖突然向外连某个 IP，是最清楚的信号，而几乎没人在看这个。
- **你所依赖的包发生维护者或所有权变更** —— 仓库 API 会暴露这些。
- **构建耗时与资源异常**，xz 就是这么被发现的。
- **lockfile 的差异超出了你本意要改的那个依赖**，包括来源 URL 与完整性哈希。
- **新加入包里的 postinstall / preinstall 脚本**，要在合并前审而不是合并后。
- **构建节点上意外的出站连接**，包括 DNS。

### 缓解

- **固定依赖并提交 lockfile**，校验完整性哈希，让被替换的包直接构建失败。
- **使用私有仓库或代理**（Artifactory、Nexus、Go module proxy），并在构建节点上阻断直连公共仓库 —— 这一条就能一次性消掉大部分依赖混淆。
- **给内部包加作用域**，让它们的名字无法在公网被注册。
- **把新依赖当代码来审。** 一份每周新增包的报告（含维护者、包龄、安装脚本）是便宜的控制。
- **每个发布版本生成 SBOM**（Syft、cyclonedx），让"我们是否受影响"变成一个查询而不是一次调查。
- **给产物签名并发布 provenance**，让消费方能验证自己下载的东西。
- **构建节点给最小权限**，不要给它们用不到的生产凭据。
- **支持你所依赖的关键项目** —— 出钱或出工程时间。被弃养的依赖就是一扇开着的门，而最便宜的缓解往往就是为维护付费。
- **盯构建，不只盯代码。** 网络访问、安装脚本与资源异常，是这些沦陷最先露头的地方。
