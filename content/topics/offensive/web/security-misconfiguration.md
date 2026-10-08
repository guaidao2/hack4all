---
id: security-misconfiguration
title_en: Security Misconfiguration
title_zh: 安全配置错误
summary_en: Nothing is exploited here and nothing is malformed. A default password was never changed, a debug endpoint was left on, a backup file is sitting in the web root, and a bucket is public. It is the least glamorous category and one of the most productive.
summary_zh: 这里没有被利用的漏洞，也没有畸形请求。一个默认口令没改、一个调试接口没关、一个备份文件躺在 Web 根目录、一个存储桶是公开的。这是最不花哨的一类，也是产出最高的一类。
tags: [web, owasp-a05, misconfiguration, default-credentials, cors, hardening]
tools: [nuclei, ffuf, gobuster, nikto, testssl.sh]
attck: [T1190, T1595]
platform: [web, cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Why a checklist works here

Other categories need reasoning; this one rewards systematic coverage. The bugs are things that were *supposed* to be turned off, changed, or removed, and they are almost always findable with a wordlist, a scanner and a careful eye over response headers.

The reason it stays common is structural: configuration drifts. A staging flag ships to production, a new service is deployed with defaults, a backup job writes into a served directory, and each individual decision is defensible at the time.

### The checklist

**1. Default credentials and default pages.** Every appliance, CMS, monitoring tool and framework has a default account, and a surprising number are reachable. Try the vendor default before anything clever, and check the management interface of anything you find: Jenkins, Grafana, Kibana, Tomcat manager, phpMyAdmin, RabbitMQ, Redis.

**2. Source, backups and metadata in the web root.**

```bash
ffuf -u https://target/FUZZ -w seclists/Discovery/Web-Content/raft-medium-files.txt -mc 200,301,403

# the ones worth trying by hand
/.git/config      /.svn/entries     /.hg/
/.env             /config.php.bak   /backup.zip
/.DS_Store        /Thumbs.db        /.idea/workspace.xml
```

A readable `/.git/` is a full source disclosure: fetch the objects and reconstruct the repository with `git-dumper`. `.env` is often a database password and an API key in two lines.

**3. Debug features left on.** Framework debug pages (`debug=True`) expose settings and allow code execution from the error page. Spring Boot Actuator exposes `/actuator/env`, `/actuator/heapdump` and sometimes `/actuator/shutdown`. `phpinfo()` leaks paths and versions. GraphQL introspection is a documented feature that most production APIs should turn off.

**4. Management interfaces exposed.** Not just unauthenticated, but reachable from the internet at all. An internal admin panel with a VPN requirement is a control; the same panel on a public IP is a finding.

**5. CORS misconfiguration.** The dangerous combination is a permissive origin policy plus credentials:

```http
Access-Control-Allow-Origin: https://evil.example
Access-Control-Allow-Credentials: true
```

A reflected `Origin` header, a `null` origin accepted, or a wildcard with credentials lets any site read authenticated responses. Test by sending an `Origin` you control and reading what comes back — the browser enforces it, so a bad configuration is exploitable by a victim's browser, not by curl.

**6. Missing security headers.** Not a vulnerability by themselves, but the absence of `Content-Security-Policy`, `Strict-Transport-Security`, `X-Content-Type-Options` and a sane `X-Frame-Options`/`frame-ancestors` is how a small bug becomes a serious one.

**7. Open HTTP methods and WebDAV.** `OPTIONS` telling you that `PUT` is allowed, then `PUT` actually writing a file. WebDAV enabled where nobody uses it.

**8. Verbose error handling.** Stack traces name frameworks and versions, expose file paths, and sometimes SQL. The same information in a log is useful; on the screen it is reconnaissance.

**9. Cloud and storage configuration.** Public buckets, permissive IAM, security groups open to the world, snapshots shared publicly, and metadata services reachable from instances that do not need them. This is its own body of work and intersects the cloud entries in this guide.

**10. Directory listing and cache configuration.** Autoindex on is a small information leak; a cache configured without regard for `Cache-Control: private` is a much bigger one, because it can serve one user's page to another.

**11. TLS and certificate configuration.** Expired certificates, TLS 1.0 still enabled, weak cipher suites, missing HSTS. Test it once and move on: `testssl.sh` or an equivalent gives the whole picture.

### Automating the boring half

```bash
# templates for the common misconfigurations: exposed panels, default creds, debug endpoints
nuclei -u https://target -t http/misconfiguration/ -t http/exposures/

# content discovery for backups and metadata
gobuster dir -u https://target -w seclists/Discovery/Web-Content/common.txt -x bak,zip,old,txt
```

Automation covers maybe half. The other half is attention: reading response headers on every request in the proxy history, noticing that an error page changed, checking whether the new subdomain found during reconnaissance has an admin panel.

### Detection

Misconfiguration is visible by inspecting your own systems the way an attacker would:

- **External attack surface monitoring**: what do you actually expose? Compare against what you think you expose. The gap is the finding.
- **Cloud configuration drift**: alert on a bucket becoming public, a security group gaining `0.0.0.0/0`, or a new public IP appearing.
- **Debug endpoints in production**: a route that exists only in the debug profile should not resolve. Alert if it does.
- **Backup files created inside a served directory** — that is a deployment mistake worth failing a build over.
- **Certificate and header monitoring**: expiry warnings, and a check that security headers are present on the responses that matter.

### Mitigation

- **Harden from a baseline.** CIS Benchmarks or the vendor's own hardening guide, applied as code rather than as a one-off checklist.
- **Scan infrastructure as code** (tfsec, checkov, kube-bench) in CI, so a public bucket or an open security group fails the pipeline rather than reaching production.
- **Delete what you do not use**: default accounts, sample applications, unused modules, old API versions, debug profiles.
- **Separate environments properly.** The staging config must not be one environment variable away from production.
- **Set security headers centrally**, at the edge or in the framework's middleware, and verify them in a test rather than in a manual review.
- **Make the secure configuration the default one** in your framework templates, so a new service starts closed and someone has to open it deliberately.
- **Monitor drift continuously.** Configuration is not a state you reach; it is a state you maintain.

<!-- lang:zh -->
### 为什么这里靠清单就管用

其他类别需要推理，这一类奖励的是系统性覆盖。这些 bug 都是"本该被关掉、改掉或删掉"的东西，而且几乎总能用一份字典、一个扫描器加一双盯着响应头的眼睛找出来。

它长期普遍的原因是结构性的：配置会漂移。一个 staging 的开关被带上生产、一个新服务用默认值部署、一个备份任务往被提供的目录里写文件 —— 每个单独的决定在当时都说得通。

### 清单

**一、默认凭据与默认页面。** 每台设备、每套 CMS、每个监控工具和框架都有默认账号，其中可访问的数量多得惊人。在做任何聪明事之前先试厂商默认口令，并检查你找到的任何管理界面：Jenkins、Grafana、Kibana、Tomcat manager、phpMyAdmin、RabbitMQ、Redis。

**二、Web 根目录里的源码、备份与元数据。**

```bash
ffuf -u https://target/FUZZ -w seclists/Discovery/Web-Content/raft-medium-files.txt -mc 200,301,403

# 值得手工试的几个
/.git/config      /.svn/entries     /.hg/
/.env             /config.php.bak   /backup.zip
/.DS_Store        /Thumbs.db        /.idea/workspace.xml
```

可读的 `/.git/` 等于完整源码泄漏：把对象抓下来，用 `git-dumper` 就能重建仓库。`.env` 往往两行里就是数据库口令和一个 API key。

**三、遗留的调试功能。** 框架的调试页（`debug=True`）会暴露配置，还能从报错页直接执行代码。Spring Boot Actuator 暴露 `/actuator/env`、`/actuator/heapdump`，有时还有 `/actuator/shutdown`。`phpinfo()` 泄漏路径与版本。GraphQL introspection 是文档化的功能，但大多数生产 API 应该关掉它。

**四、暴露的管理接口。** 不只是"未认证"，而是"能直接从公网到达"。需要 VPN 才能访问的内部管理面板是一种控制；同一个面板放在公网 IP 上就是一个发现。

**五、CORS 配置错误。** 危险的组合是宽松的来源策略加凭据：

```http
Access-Control-Allow-Origin: https://evil.example
Access-Control-Allow-Credentials: true
```

反射 `Origin` 头、接受 `null` 来源，或带凭据的通配符，都会让任意站点读取已认证的响应。测试方式是发一个你控制的 `Origin` 并读回响应 —— 这个限制由浏览器执行，所以坏配置是受害者的浏览器去利用，而不是 curl。

**六、安全响应头缺失。** 它们本身不是漏洞，但缺 `Content-Security-Policy`、`Strict-Transport-Security`、`X-Content-Type-Options` 和合理的 `X-Frame-Options`/`frame-ancestors`，正是小 bug 变成严重问题的原因。

**七、开放的 HTTP 方法与 WebDAV。** `OPTIONS` 告诉你允许 `PUT`，然后 `PUT` 真的写进去了。没人在用的地方开着 WebDAV。

**八、过于详细的错误处理。** 堆栈会暴露框架与版本、文件路径，有时还有 SQL。同样的信息写在日志里有用，显示在屏幕上就是给攻击者的情报。

**九、云与存储配置。** 公开的存储桶、过宽的 IAM、对全网开放的安全组、公开共享的快照，以及从并不需要它们的实例上可达的元数据服务。这本身是一大块工作，与本指南里的云相关篇目交叉。

**十、目录列表与缓存配置。** 自动索引开着是个小信息泄漏；而忽视 `Cache-Control: private` 的缓存配置则大得多，因为它可能把某个用户的页面发给另一个用户。

**十一、TLS 与证书配置。** 过期证书、仍开着 TLS 1.0、弱套件、缺 HSTS。测一次就好：`testssl.sh` 之类的工具能给出全貌。

### 把枯燥的那一半自动化

```bash
# 常见错误配置的模板：暴露的面板、默认凭据、调试端点
nuclei -u https://target -t http/misconfiguration/ -t http/exposures/

# 找备份与元数据的内容发现
gobuster dir -u https://target -w seclists/Discovery/Web-Content/common.txt -x bak,zip,old,txt
```

自动化大概覆盖一半。另一半靠注意力：读代理历史里每个请求的响应头、注意到某个报错页变了、检查侦察阶段发现的新子域上是不是挂着管理面板。

### 检测

错误配置可以通过"像攻击者那样检查自己的系统"来发现：

- **外部攻击面监控**：你到底暴露了什么？与你以为暴露的做对比。差集就是发现。
- **云配置漂移**：对"某个桶变公开""某个安全组新增 `0.0.0.0/0`""出现新的公网 IP"告警。
- **生产环境里的调试端点**：只在 debug profile 里存在的路由不该能解析。能解析就告警。
- **在会被提供的目录里生成备份文件** —— 这是值得让构建失败一次的部署错误。
- **证书与响应头监控**：过期预警，以及对关键响应是否带齐安全头的检查。

### 缓解

- **按基线加固。** CIS Benchmark 或厂商自己的加固指南，并且**以代码形式**落地，而不是当成一次性清单。
- **在 CI 里扫描基础设施即代码**（tfsec、checkov、kube-bench），让公开的桶或开放的安全组在流水线里失败，而不是进到生产。
- **删掉用不到的东西**：默认账号、示例应用、未使用模块、旧版 API、调试 profile。
- **把环境真正隔开。** staging 的配置不该离生产只差一个环境变量。
- **集中设置安全响应头**，放在边缘或框架中间件里，并用测试而不是人工评审来验证。
- **把安全配置做成框架模板里的默认值**，这样新服务一出生就是关着的，要开必须有人显式去做。
- **持续监控漂移。** 配置不是一个"到达"的状态，而是一个要一直维持的状态。
