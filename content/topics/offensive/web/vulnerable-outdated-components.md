---
id: vulnerable-outdated-components
title_en: Vulnerable and Outdated Components
title_zh: 易受攻击与过时的组件
summary_en: Most of what an attacker exploits on a mature target is not code the organisation wrote. It is a component somebody else shipped, with a CVE published since and a patch nobody applied — and the highest-value ones sit at the network edge.
summary_zh: 在一个成熟目标上，攻击者利用的东西大多不是组织自己写的代码，而是别人发布的组件 —— 之后公布了 CVE，而补丁没人打。价值最高的那些，通常就在网络边界上。
tags: [web, owasp-a06, cve, fingerprinting, patch-management, edge-devices]
tools: [nuclei, whatweb, retire.js, searchsploit, trivy]
attck: [T1190, T1595]
platform: [web, network]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### What this category covers

Three different things that get filed together:

- **Known vulnerabilities in components you use** — a library, framework or service with a published CVE.
- **End-of-life software** — no longer supported, therefore no longer patched.
- **Components you cannot see** — the appliance, the plugin, the transitive dependency of a dependency.

The distinction from supply chain attacks matters: here the code is *legitimate but known-broken*. Nobody inserted anything malicious; somebody just did not patch.

### Step 1 — Fingerprint what is running

Version identification is the whole game, because a version number maps directly to a CVE list.

```bash
whatweb -a 3 https://target.example
nuclei -u https://target.example -t http/technologies/ -t http/exposures/
```

Where versions hide:

| Signal | What it reveals |
|---|---|
| `Server`, `X-Powered-By`, `X-AspNet-Version` | Server and framework versions |
| Asset paths | `/wp-content/`, `/static/js/app.4f3a.js`, `/jquery-3.4.1.min.js` |
| Cookie names | `JSESSIONID` (Java), `PHPSESSID` (PHP), `ASP.NET_SessionId` |
| Error pages and stack traces | Framework, version, sometimes the whole dependency tree |
| Favicon hash | A reliable fingerprint for appliances and panels |
| `robots.txt`, `sitemap.xml`, `/CHANGELOG`, `/README` | Product and version |
| Behaviour | A response header order or 404 page unique to a version |

For front-end libraries, the bundle itself is the evidence:

```bash
retire --js --path ./downloaded-js      # flags known-vulnerable JS libraries with versions
```

### Step 2 — Map versions to vulnerabilities

```bash
searchsploit apache 2.4.49
nuclei -u https://target.example -t http/cves/ -tags cve
```

The components worth memorising, because they keep appearing:

- **Java ecosystem**: Log4Shell (Log4j 2), Spring4Shell, Struts2 OGNL, WebLogic T3/IIOP, Jenkins plugins.
- **Collaboration and wiki platforms**: Confluence, Jira, GitLab — they hold credentials and are internet-facing by design.
- **Edge appliances**: VPN concentrators, firewalls, load balancers, mail gateways. These are the most valuable target on any perimeter, they are frequently unpatched, and they are frequently not covered by the asset inventory at all.
- **File transfer and backup software**: the class of product that exists to move data between networks, which is why compromising it is so productive.
- **CMS and plugins**: WordPress and its plugin ecosystem, where the plugin is often the vulnerable part and the update nobody performed.

```bash
# does the target actually respond to the specific CVE probe?
nuclei -u https://target.example -t http/cves/2021/CVE-2021-44228.yaml
```

Always confirm a version match with a behaviour test. A version string in a header can be spoofed, backported patches do not change the number, and a scanner hit that nobody verified is a false positive in the report.

### Step 3 — When you cannot get a version

- **Probe the behaviour instead.** Many CVEs have a distinguishable response: a specific path, a specific error, a specific timing.
- **Compare builds.** A patched and an unpatched instance of the same product often differ in one file hash or one header.
- **Look for the exploit's prerequisites** rather than the version: a vulnerable endpoint that exists is enough, regardless of what the version says.

### Detection

- **You cannot patch what you do not know you run.** An inventory that covers servers but not appliances and not transitive dependencies is the root cause of most of this.
- **SBOM generation** (Syft, cyclonedx-gomod, and equivalents) makes the dependency tree visible; the same data feeds vulnerability scanning.
- **Scanning in CI** (Trivy, Grype, OWASP Dependency-Check, Dependabot) catches the library before it ships, which is much cheaper than after.
- **Alert on new CVEs affecting components you run** — asset-linked vulnerability feeds, not raw CVE firehoses.
- **Watch the edge specifically.** Appliances are the least patched and most exposed; a monthly "what firmware are these running" review is worth more than most vulnerability programmes.
- **Detect exploitation attempts**: exploit probes for well-known CVEs are noisy and recognisable, and they arrive at every internet-facing service eventually.

### Mitigation

- **Patch, and prioritise by exposure.** An internet-facing appliance with a known RCE outranks a hundred internal findings. Set a timeline by severity and hold to it.
- **Remove the component if you cannot patch it.** An unsupported plugin that nobody uses is pure risk; deleting it is a fix.
- **Track end-of-life dates** as a first-class asset attribute, so unsupported software is visible before it becomes an incident.
- **Lock and verify dependencies** (lockfiles, hashes, and a private mirror) so the version you tested is the version you deploy.
- **Keep an SBOM per release**, and use it to answer "are we affected by this CVE" in minutes rather than days.
- **Virtual patching through a WAF** buys time, not safety; it is a temporary control while the real patch is scheduled.
- **Segment and restrict the edge.** The appliance will be unpatched at some point; making it reachable from everywhere makes that worse.

<!-- lang:zh -->
### 这一类覆盖什么

三种东西常被归到一起：

- **你在用的组件里已知的漏洞** —— 带有已公布 CVE 的库、框架或服务。
- **生命周期结束的软件** —— 不再支持，因此不再打补丁。
- **你看不见的组件** —— 那台设备、那个插件、依赖的依赖。

与供应链攻击的区别很要紧：这里代码是**合法的，只是已知是坏的**。没有人塞进恶意代码，只是没人打补丁。

### 第一步 —— 指纹识别跑着什么

版本识别就是全部，因为版本号直接对应一张 CVE 清单。

```bash
whatweb -a 3 https://target.example
nuclei -u https://target.example -t http/technologies/ -t http/exposures/
```

版本信息藏在哪里：

| 信号 | 它暴露什么 |
|---|---|
| `Server`、`X-Powered-By`、`X-AspNet-Version` | 服务器与框架版本 |
| 静态资源路径 | `/wp-content/`、`/static/js/app.4f3a.js`、`/jquery-3.4.1.min.js` |
| Cookie 名 | `JSESSIONID`（Java）、`PHPSESSID`（PHP）、`ASP.NET_SessionId` |
| 报错页与堆栈 | 框架、版本，有时是整棵依赖树 |
| favicon 哈希 | 对设备和面板来说非常可靠的指纹 |
| `robots.txt`、`sitemap.xml`、`/CHANGELOG`、`/README` | 产品与版本 |
| 行为特征 | 某个版本特有的响应头顺序或 404 页 |

对前端库来说，bundle 本身就是证据：

```bash
retire --js --path ./downloaded-js      # 标出已知有漏洞的 JS 库及版本
```

### 第二步 —— 把版本映射到漏洞

```bash
searchsploit apache 2.4.49
nuclei -u https://target.example -t http/cves/ -tags cve
```

反复出现、值得记住的组件：

- **Java 生态**：Log4Shell（Log4j 2）、Spring4Shell、Struts2 的 OGNL、WebLogic 的 T3/IIOP、Jenkins 插件。
- **协作与知识库平台**：Confluence、Jira、GitLab —— 它们存着凭据，而且按设计就暴露在公网。
- **边界设备**：VPN 网关、防火墙、负载均衡、邮件网关。它们通常是整个边界上最有价值的目标，经常没打补丁，而且经常根本不在资产台账里。
- **文件传输与备份软件**：这类产品存在就是为了在网之间搬数据，所以拿下它收益极高。
- **CMS 与插件**：WordPress 及其插件生态，有漏洞的常常是插件，而更新没人做。

```bash
# 目标到底会不会对某个具体 CVE 的探针有反应？
nuclei -u https://target.example -t http/cves/2021/CVE-2021-44228.yaml
```

版本匹配一定要用行为测试确认。响应头里的版本号可以伪造，向后移植的补丁不会改版本号，而没人验证过的扫描结果写进报告就是误报。

### 第三步 —— 拿不到版本的时候

- **改测行为。** 很多 CVE 有可区分的响应：特定路径、特定报错、特定耗时。
- **对比构建。** 同一产品的已修补实例和未修补实例，常常只差一个文件哈希或一个响应头。
- **找利用的前置条件，而不是版本**：一个真实存在的有漏洞端点就够了，版本号说什么并不重要。

### 检测

- **你不知道自己在跑什么，就修不了它。** 台账只覆盖服务器、不覆盖设备和传递依赖，就是这一类问题的根因。
- **生成 SBOM**（Syft、cyclonedx-gomod 等）让依赖树可见，同一份数据还能喂给漏洞扫描。
- **在 CI 里扫描**（Trivy、Grype、OWASP Dependency-Check、Dependabot），在组件出厂前就抓住它，比出厂后便宜得多。
- **对影响你所跑组件的新 CVE 告警** —— 要关联资产的漏洞源，而不是 CVE 原始洪流。
- **单独盯住边界。** 设备是补丁最少、暴露最多的；每月做一次"这些固件跑的是哪个版本"的复查，比大多数漏洞管理项目都有价值。
- **检测利用尝试**：知名 CVE 的探针很吵也很容易认，而且它们终究会到达每一个暴露在公网的服务。

### 缓解

- **打补丁，并按暴露面排序。** 一个暴露在公网、存在已知 RCE 的设备，优先级高于一百个内网发现。按严重级别定时间线并守住它。
- **修不了就删掉。** 一个没人用、又不被支持的插件就是纯风险；删掉就是修复。
- **把 EOL 日期当作一等资产属性来跟踪**，让不支持的软件在变成事故之前就可见。
- **锁定并校验依赖**（lockfile、哈希、私有镜像源），让你测过的版本就是你部署的版本。
- **每个发布版本留一份 SBOM**，用它把"我们是否受这个 CVE 影响"从几天缩短到几分钟。
- **用 WAF 做虚拟补丁**买的是时间，不是安全；它只是真正补丁排期期间的临时控制。
- **给边界做分段与访问限制。** 那台设备迟早会有没打补丁的时候，让它从任何地方都能访问只会让情况更糟。
