---
id: software-data-integrity-failures
title_en: Software and Data Integrity Failures
title_zh: 软件与数据完整性失效
summary_en: This category is about trust that was never verified. An update channel with no signature, a build pipeline that runs whatever a pull request contains, a third-party script with no integrity hash. The code is legitimate right up until someone changes it.
summary_zh: 这一类讲的是"从未被验证过的信任"：没有签名的更新通道、会执行 PR 内容的构建流水线、没有完整性哈希的第三方脚本。代码一直是合法的，直到有人把它换掉。
tags: [web, owasp-a08, supply-chain, cicd, sri, code-signing]
tools: [cosign, sigstore, syft, grype, gitleaks, zizmor]
attck: [T1195.002, T1553.002]
platform: [web, cloud]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### Two halves of one idea

OWASP's A08 covers two things that share a root cause — trusting something without verifying it:

1. **Insecure deserialization** (its own entry in this guide): data becomes objects, and objects run code.
2. **Software and data integrity failures**: updates, builds, dependencies and third-party assets that arrive through a channel nobody authenticates.

The second is the one this entry is about, and it is where a single compromised account can become every customer's problem.

### 1. Update channels without signatures

An auto-update mechanism is a remote code execution feature by design. The only question is who can use it:

- **No signature at all.** The client downloads and runs what the server sends. Whoever controls the server — or the DNS, or the TLS connection if it is not verified — controls every installation.
- **Signature checked but not enforced.** A "verify if present" path that falls back to running unsigned packages is the same as no signature, with extra steps.
- **Update over plain HTTP**, or HTTPS with certificate validation disabled in the client.
- **Downgrade allowed.** If old versions are accepted, an attacker can roll a client back to a version with a known bug and exploit that.

```bash
# does the update endpoint actually sign? capture the manifest and look
curl -s https://updates.vendor.example/manifest.json | jq .
# a hash without a signature proves nothing: whoever can change the file can change the hash
```

### 2. Build pipelines

The build system is the highest-value target in a modern organisation: it has credentials for everything, and its output is trusted by everyone. Two failure shapes dominate.

**Running code from an untrusted contribution.** The classic is GitHub Actions' `pull_request_target`, which runs with the repository's secrets and a write token, combined with checking out the contributor's code:

```yaml
# dangerous: pull_request_target has secrets, and this checks out attacker code
on: pull_request_target
jobs:
  build:
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm install          # runs the contributor's postinstall scripts
```

The contributor does not need write access to the repository. They need a pull request, and the pipeline hands them the secrets.

**Unpinned and unverified inputs.**

- Actions referenced by tag (`@v4`) can be retagged; pinning to a commit SHA cannot.
- Dependencies installed without a lockfile resolve to whatever is latest at build time.
- Secrets echoed into build logs, which are often readable by more people than intended.
- `GITHUB_TOKEN` with default write permissions rather than the minimum the job needs.

**Unverifiable output.** If the artifact is not signed and no provenance is published, nobody downstream can tell whether the binary they downloaded came from that pipeline at all.

### 3. Third-party scripts in the browser

A `<script src="https://cdn.example/lib.js">` without an integrity attribute is a trust delegation to a third party:

```html
<!-- anyone who can change that file, at the CDN or in transit, runs code in your users' browsers -->
<script src="https://cdn.example/analytics.js"></script>

<!-- with SRI, the browser refuses a file whose hash does not match -->
<script src="https://cdn.example/analytics.js"
        integrity="sha384-<hash>" crossorigin="anonymous"></script>
```

Subresource Integrity is the control; a CSP that allows the CDN is a weaker one, because the CDN is exactly the thing that might change. When a popular CDN or a widely used analytics library is compromised, every site that loaded it without SRI is compromised at once — which is how one incident becomes a headline.

### 4. Dependency confusion and typo-squatting

Installing from a public registry a package name that was meant to be internal. If the internal package `@company/utils` does not exist publicly, anyone can publish it, and the build will prefer the higher version from the public source. The supply chain entry in this guide covers the mechanics; what matters for A08 is that the *build* has no way to distinguish the legitimate package from the substituted one unless names are pinned, scoped and verified.

### 5. Data integrity

Not everything is code:

- **Imports that are not validated.** A CSV or XML import that feeds directly into business logic, with no schema validation and no size limit.
- **Backup and restore paths.** A restore that takes a backup file from a location anyone can write to is a code execution primitive once the data lands in the right table.
- **Configuration imports** that deserialize YAML, XML or serialized objects from an upload — which is deserialization wearing a different hat.
- **Client-side integrity checks** in desktop and mobile clients, which are always bypassable and should never be the only control.

### Testing it

- Pull the update manifest and check whether a signature exists, what it covers, and whether the client enforces it.
- For an open-source target, read the workflow files: triggers, permissions, whether actions are pinned, whether contributor code is executed.
- Grep page source for `<script src=` without `integrity`.
- For a released binary, check whether it is signed and whether provenance is published.
- Check what the artifact contains: a build log, a `.env`, and a source map in production are all integrity problems in practice.

### Detection

- **Unexpected changes to build configuration** are worth alerting on more than most code changes, because they affect everything downstream.
- **New publishing tokens or registry credentials** used from a new location.
- **Artifacts published without a signature** where the pipeline is supposed to produce one — a failing control is a signal.
- **Page changes that add a third-party script** without SRI.
- **Dependency changes that are not accompanied by a lockfile change** in a repository that commits one.

### Mitigation

- **Sign updates and enforce verification**, with the key held offline and the client refusing unsigned or downgraded packages.
- **Pin CI actions to commit SHAs**, set `permissions: read-all` by default and grant write only where required, and never run contributor code in a job that holds secrets.
- **Lock dependencies and verify hashes**, with a private mirror or a proxy so the build does not silently reach a public registry.
- **Produce provenance and sign artifacts** (Sigstore/cosign, SLSA-style attestations), so a consumer can verify where a binary came from.
- **Use SRI for every third-party script**, and prefer self-hosting the ones you depend on.
- **Validate imports against a schema** and bound their size, and treat any restore path as an untrusted input.
- **Keep secrets out of build logs**, and scan logs for accidental disclosure.
- **Treat the build system as Tier 0.** It is the one system whose compromise affects everything you ship, and it should be protected accordingly.

<!-- lang:zh -->
### 一个想法，两半

OWASP 的 A08 覆盖两件同源的事 —— 信任了却没有验证：

1. **不安全的反序列化**（本指南有专篇）：数据变成对象，对象执行代码。
2. **软件与数据完整性失效**：更新、构建、依赖与第三方资源，来自一条没人验证身份的通道。

这一篇讲的是第二个。在这里，一个被拿下的账号可以变成所有客户的问题。

### 一 —— 没有签名的更新通道

自动更新机制按设计就是"远程代码执行功能"。唯一的问题是**谁能用它**：

- **完全没有签名。** 客户端下载什么就跑什么。谁控制了服务器 —— 或者控制了 DNS，或者在未校验证书时控制了 TLS 连接 —— 谁就控制了每一个安装。
- **验签但不强制。** "有签名就验、没有就照跑"的路径，等于没有签名，只是多绕几步。
- **走明文 HTTP 更新**，或者客户端禁用了证书校验的 HTTPS。
- **允许降级。** 如果旧版本被接受，攻击者可以把客户端回滚到有已知漏洞的版本再打它。

```bash
# 更新接口到底签不签名？把清单抓下来看
curl -s https://updates.vendor.example/manifest.json | jq .
# 只有哈希、没有签名说明不了任何事：能改文件的人也能改哈希
```

### 二 —— 构建流水线

构建系统是现代组织里价值最高的目标：它握有通往一切的凭据，而它的产物被所有人信任。两种失败形态占主导。

**执行来自不可信贡献的代码。** 经典案例是 GitHub Actions 的 `pull_request_target` —— 它以仓库的 secrets 和写权限 token 运行，再配合 checkout 贡献者的代码：

```yaml
# 危险：pull_request_target 带着 secrets，而这里 checkout 了攻击者的代码
on: pull_request_target
jobs:
  build:
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
      - run: npm install          # 会执行贡献者的 postinstall 脚本
```

贡献者不需要仓库写权限，只需要一个 pull request，流水线就把 secrets 交出去了。

**未固定、未验证的输入。**

- 用 tag 引用的 action（`@v4`）可以被重新指向；固定到 commit SHA 则不能。
- 没有 lockfile 的依赖安装，会在构建时解析到"当时最新"的版本。
- secrets 被回显进构建日志，而日志的可见范围往往比预期大。
- `GITHUB_TOKEN` 用默认的写权限，而不是该任务所需的最小权限。

**无法验证的产物。** 如果产物没有签名、也没有发布 provenance，下游没人能判断自己下到的二进制是否真的来自那条流水线。

### 三 —— 浏览器里的第三方脚本

不带 integrity 属性的 `<script src="https://cdn.example/lib.js">` 就是把信任委托给第三方：

```html
<!-- 谁能改那个文件 —— 无论是 CDN 上还是在传输中 —— 谁就能在你的用户浏览器里执行代码 -->
<script src="https://cdn.example/analytics.js"></script>

<!-- 有 SRI 时，哈希不匹配的文件会被浏览器拒绝 -->
<script src="https://cdn.example/analytics.js"
        integrity="sha384-<hash>" crossorigin="anonymous"></script>
```

SRI 才是控制手段；允许该 CDN 的 CSP 是更弱的控制，因为那个 CDN 恰恰是可能变的东西。当某个流行的 CDN 或广泛使用的分析库被拿下时，所有没加 SRI 就加载它的站点会同时沦陷 —— 一个事件变成头条就是这么来的。

### 四 —— 依赖混淆与拼写抢注

从公共仓库安装一个本应来自内部的包名。如果内部包 `@company/utils` 在公网上并不存在，任何人都能发布它，而构建会优先选公共源里版本号更高的那个。本指南的供应链篇讲了机制；对 A08 而言要紧的是：除非名字被固定、限定作用域并验证过，**构建本身没有任何办法区分合法包和被替换的包**。

### 五 —— 数据完整性

不是所有东西都是代码：

- **不校验的导入。** CSV 或 XML 导入直接喂进业务逻辑，既没有 schema 校验也没有大小限制。
- **备份与恢复路径。** 如果有人能写的位置取备份文件来恢复，那么当数据落进正确的表之后，它就是一个代码执行原语。
- **配置导入**会从上传内容里反序列化 YAML、XML 或序列化对象 —— 这就是换了顶帽子的反序列化。
- **客户端完整性校验**（桌面与移动客户端），它总是可以被绕过，绝不该是唯一的控制。

### 怎么测

- 拉下更新清单，看有没有签名、签名覆盖什么、客户端是否强制。
- 目标是开源项目时直接读 workflow 文件：触发条件、权限、action 是否固定、是否执行了贡献者代码。
- 在页面源码里 grep 不带 `integrity` 的 `<script src=`。
- 对已发布的二进制，看是否签名、是否发布 provenance。
- 看产物里包含什么：构建日志、`.env`、生产环境的 source map，实际都是完整性问题。

### 检测

- **构建配置的意外变更**比大多数代码变更更值得告警，因为它影响所有下游。
- **新的发布 token 或仓库凭据**从新位置被使用。
- **本该签名却出现未签名的产物** —— 控制失效本身就是信号。
- **页面变更中新增了不带 SRI 的第三方脚本**。
- **依赖变更没有伴随 lockfile 变更**，而该仓库本来是提交 lockfile 的。

### 缓解

- **给更新签名并强制校验**，密钥离线保存，客户端拒绝未签名或降级的包。
- **CI 的 action 固定到 commit SHA**，默认 `permissions: read-all`，只在必需处授予写权限，绝不在持有 secrets 的任务里执行贡献者代码。
- **锁定依赖并校验哈希**，配私有镜像或代理，避免构建悄悄去拉公共仓库。
- **生成 provenance 并给产物签名**（Sigstore/cosign、SLSA 风格的证明），让消费方能验证二进制的来源。
- **给每个第三方脚本加 SRI**，能自托管的就自托管。
- **对导入做 schema 校验**并限制体积，把任何恢复路径都当作不可信输入。
- **别让 secrets 进构建日志**，并扫描日志排查意外泄漏。
- **把构建系统当作 Tier 0。** 它是唯一一个被拿下就影响全部产物的系统，防护级别应当与之相称。
