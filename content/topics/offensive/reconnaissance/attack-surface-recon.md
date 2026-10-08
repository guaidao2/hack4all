---
id: attack-surface-recon
title_en: Reconnaissance and Attack Surface Mapping
title_zh: 信息收集与攻击面测绘
summary_en: Reconnaissance is not a tool phase, it is a decision phase. The output that matters is not a list of hosts, it is a ranked map of what is exposed, what is owned by whom, and which one thing is most likely to be reachable from the internet with a bug in it.
summary_zh: 信息收集不是"跑一遍工具"的阶段，而是做决定的阶段。真正有用的产出不是一堆主机，而是一张排过序的地图：暴露了什么、归谁管、以及哪一处最可能从公网摸到一个洞。
tags: [reconnaissance, osint, subdomain, enumeration, bugbounty, pentest]
tools: [subfinder, amass, httpx, naabu, nmap, katana, gitleaks, trufflehog]
attck: [T1595, T1596, T1590]
platform: [web, network]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### What reconnaissance is actually for

You are building three things, in this order of importance:

1. **Scope clarity** — what is in scope, who owns it, what will be considered out of bounds on contact.
2. **A complete asset list** — the honest one, including the forgotten subdomain and the old CDN origin.
3. **Prioritisation** — which few assets deserve the next hour.

The most common failure is skipping step 3 and testing alphabetically. A forgotten staging subdomain with default credentials beats a hardened main site every single time.

### Step 1 — Passive first

Passive collection touches nothing of the target's, so it cannot be logged as hostile and it cannot be rate-limited:

```bash
# certificate transparency is the single richest free source of subdomains
curl -s "https://crt.sh/?q=%25.example.com&output=json" | jq -r '.[].name_value' | sort -u

# aggregators and passive DNS
subfinder -d example.com -all -silent | sort -u
amass enum -passive -d example.com
```

What else is public and useful:

| Source | What it gives |
|---|---|
| Certificate transparency | Every hostname that ever had a certificate, including internal ones that leaked |
| Passive DNS datasets | Historical resolutions, origin IPs behind a CDN |
| Search engine dorks | Exposed directories, indexed configuration files, error pages |
| Job adverts | The exact technology stack, versions and internal tool names |
| GitHub / GitLab | Committed secrets, internal hostnames, CI configuration |
| Shodan / Censys / FOFA | Services already fingerprinted, with banners, without touching them |
| SPF/DMARC/DKIM records | Mail infrastructure, third-party senders, sometimes internal IPs |

For leaked secrets in code, run the scanners rather than grepping by hand:

```bash
gitleaks detect --source ./repo --report-format json --report-path leaks.json
trufflehog github --org=examplecorp
```

### Step 2 — Then active, in the cheapest order

```bash
# resolve what survived, and see what is actually serving HTTP
httpx -l hosts.txt -title -tech-detect -status-code -o live.txt

# ports: fast sweep first, then depth on the interesting ones
naabu -host example.com -top-ports 1000 -silent
nmap -sV -sC -p- --min-rate 2000 -oA nmap-full 10.10.10.0/24
```

Do not start with `-p-` on a /16. Sweep, filter, then go deep on the handful that answer on something other than 80/443.

### Step 3 — Content on the web targets

```bash
# crawl for endpoints, including those referenced only from JavaScript
katana -u https://app.example.com -jc -d 3 -o urls.txt

# then look for the ones the crawler will not find
ffuf -u https://app.example.com/FUZZ -w /usr/share/seclists/Discovery/Web-Content/raft-medium-directories.txt \
     -mc 200,301,302,401,403 -fs 0
```

Always fetch and read `/robots.txt`, `/sitemap.xml`, `/.well-known/`, and the JavaScript bundles. Modern single-page applications put their entire API surface in the bundle, including admin routes that the UI never links to:

```bash
# pull API paths out of the front-end bundle
grep -oE '"/[a-zA-Z0-9_/{}.-]{3,}"' bundle.js | sort -u | head -50
```

### Step 4 — Cloud and third-party assets

A company's data usually leaks through the storage it forgot:

- **S3 buckets**: try `company`, `company-backup`, `company-dev`, `company-assets`, and permutations with the environment suffix. Public listing is the finding; public write is the incident.
- **Azure Blob** and **GCP Storage**: same idea, different naming conventions.
- **Third-party SaaS in DNS**: the CNAMEs point at services that may be on a free tier the company stopped paying for — that is subdomain takeover, and it is checkable in bulk.
- **Old origins**: if a CDN fronts the site, the direct origin IP may still serve it, bypassing the WAF entirely. Historical DNS is how you find it.

```bash
# subdomain takeover candidates
subzy run --targets subdomains.txt
```

### Step 5 — Turn the pile into a plan

A useful asset list has, per asset: hostname, IP, owner, technology, exposure, and a first hypothesis. Sort by:

1. **Exposure** — internet-facing beats internal, unless you already have a foothold.
2. **Ownership** — a shadow asset nobody maintains is a better bet than the main product.
3. **Age of the stack** — the older the framework, the more likely something is unpatched.
4. **Authentication surface** — anything with a login, an upload or an admin panel.

Then write down the hypothesis before testing. "This staging app has a login and the same codebase as production, so an auth bug here is likely to be reproducible there" is worth more than twenty more subdomains.

### Detection, from the defender's side

- **Certificate transparency is public.** If you do not monitor it for your own domain, an attacker will find your new internal hostname before your inventory does.
- **DNS and flow logs** show the passive-DNS aggregators and the mass resolution attempts; a sudden burst of NXDOMAIN for your domain is reconnaissance.
- **Honeypots and canary tokens** placed in the asset list catch the attacker who is enumerating too eagerly.
- **Keep the inventory honest.** Most of what reconnaissance finds is not sophisticated — it is an asset that was never decommissioned, or a bucket nobody remembered creating.
- **Monitor job adverts and code repositories for your own leaked internals**, and treat a committed secret as already compromised.

### Mitigation

- **Reduce the surface deliberately**: decommission old hosts, close unused ports, remove stale DNS records, and take back abandoned storage buckets.
- **Monitor certificate transparency** for your domains and reconcile it against your inventory.
- **Put a WAF and authentication in front of admin panels**, and never leave a staging environment reachable from the internet with production data.
- **Rotate anything that was ever committed**, and scan your own repositories continuously.
- **Publish a security.txt** with a contact, so a researcher who finds something contacts you instead of a forum.

<!-- lang:zh -->
### 信息收集到底是为了什么

你在按重要性顺序构建三样东西：

1. **范围清晰** —— 什么在范围内、归谁管、接触时会碰到哪条红线。
2. **一份完整的资产清单** —— 诚实的清单，包含那个被遗忘的子域和老 CDN 后面的源站。
3. **优先级** —— 哪少数几个资产值得花下一个小时。

最常见的失败是跳过第 3 步、按字母顺序挨着测。一个被遗忘的、用着默认口令的 staging 子域，每次都比加固良好的主站更容易出结果。

### 第一步 —— 先被动

被动收集不碰目标任何资产，所以不会被记为攻击行为，也不会被限流：

```bash
# 证书透明日志是免费的子域来源里最富的一座矿
curl -s "https://crt.sh/?q=%25.example.com&output=json" | jq -r '.[].name_value' | sort -u

# 聚合器与被动 DNS
subfinder -d example.com -all -silent | sort -u
amass enum -passive -d example.com
```

其他公开且有用的来源：

| 来源 | 它给你什么 |
|---|---|
| 证书透明日志 | 所有曾签发过证书的主机名，包括泄漏出去的内网名 |
| 被动 DNS 数据集 | 历史解析记录、CDN 背后的源站 IP |
| 搜索引擎语法 | 暴露的目录、被索引的配置文件、报错页 |
| 招聘广告 | 精确的技术栈、版本和内部工具名 |
| GitHub / GitLab | 提交过的密钥、内部主机名、CI 配置 |
| Shodan / Censys / FOFA | 已经被指纹化的服务与 banner，且不需要你去碰它 |
| SPF/DMARC/DKIM 记录 | 邮件基础设施、三方发信服务，有时还有内网 IP |

代码里的密钥泄漏要用工具，不要手工 grep：

```bash
gitleaks detect --source ./repo --report-format json --report-path leaks.json
trufflehog github --org=examplecorp
```

### 第二步 —— 再主动，按成本从低到高

```bash
# 解析出还活着的，并看哪些真的在提供 HTTP
httpx -l hosts.txt -title -tech-detect -status-code -o live.txt

# 端口：先快速扫，再对有意思的深挖
naabu -host example.com -top-ports 1000 -silent
nmap -sV -sC -p- --min-rate 2000 -oA nmap-full 10.10.10.0/24
```

不要一上来就对 /16 网段跑 `-p-`。先扫、再筛，然后对那几个在 80/443 之外应答的目标深挖。

### 第三步 —— Web 目标上的内容

```bash
# 爬取端点，包括只在 JavaScript 里引用到的
katana -u https://app.example.com -jc -d 3 -o urls.txt

# 再找爬虫找不到的那些
ffuf -u https://app.example.com/FUZZ -w /usr/share/seclists/Discovery/Web-Content/raft-medium-directories.txt \
     -mc 200,301,302,401,403 -fs 0
```

一定要抓取并阅读 `/robots.txt`、`/sitemap.xml`、`/.well-known/`，以及 JavaScript 打包文件。现代单页应用会把整套 API 面都打进 bundle，包括界面上从不链接的管理路由：

```bash
# 从前端 bundle 里挖 API 路径
grep -oE '"/[a-zA-Z0-9_/{}.-]{3,}"' bundle.js | sort -u | head -50
```

### 第四步 —— 云与三方资产

公司的数据通常从它忘了的存储里漏出来：

- **S3 桶**：试 `company`、`company-backup`、`company-dev`、`company-assets`，以及各种带环境后缀的变形。公开可列就是发现，公开可写就是事故。
- **Azure Blob** 与 **GCP Storage**：同样的思路，不同的命名习惯。
- **DNS 里的三方 SaaS**：那些 CNAME 指向的服务，可能用的是公司已经不再付费的免费档 —— 那就是子域接管，而且可以批量检查。
- **老源站**：如果站点前面挂了 CDN，源站 IP 可能仍在直接提供服务，从而完全绕过 WAF。历史 DNS 就是找到它的办法。

```bash
# 子域接管候选项
subzy run --targets subdomains.txt
```

### 第五步 —— 把一堆数据变成计划

一份有用的资产清单，每条至少含：主机名、IP、归属、技术栈、暴露面，以及一条初步假设。按下面排序：

1. **暴露面** —— 除非你已经有了立足点，否则公网资产优先于内网。
2. **归属** —— 没人维护的影子资产比主产品更值得试。
3. **技术栈年龄** —— 框架越老，越可能有没打补丁的地方。
4. **认证面** —— 任何有登录、有上传、有管理后台的地方。

然后在开测**之前**把假设写下来。"这个 staging 应用有登录页、代码和生产同一套，所以这里如果有认证缺陷，生产上很可能也能复现"——这一句话比再多二十个子域都值钱。

### 检测（防守方视角）

- **证书透明日志是公开的。** 如果你不监控自己域名的日志，攻击者会比你的资产台账更早发现你新上的内网主机名。
- **DNS 与流量日志**能看出被动 DNS 聚合器和批量解析尝试；某个域名突然出现大量 NXDOMAIN 查询，就是侦察。
- **蜜罐与金丝雀令牌**放在资产清单里，能抓住那些枚举得太急的攻击者。
- **让台账保持诚实。** 侦察找到的东西大多并不高级 —— 只是一台从未下线的主机，或者一个没人记得自己创建过的桶。
- **监控招聘广告和代码仓库里属于你自己的内部信息泄漏**，并把任何被提交过的密钥视为已泄漏。

### 缓解

- **主动缩小攻击面**：下线老主机、关掉不用的端口、清理陈旧的 DNS 记录、把废弃的存储桶收回来。
- **监控自己域名的证书透明日志**，并与资产台账对账。
- **给管理后台加 WAF 和认证**，绝不要让带着生产数据的 staging 环境暴露在公网。
- **轮换一切曾经被提交过的凭据**，并持续扫描自己的仓库。
- **发布 security.txt** 留下联系方式，让发现问题的研究者联系你而不是去论坛发帖。
