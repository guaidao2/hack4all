# Hack4all

**面向渗透测试 / 红队 / bug bounty 的双语技术点指南。**

[English](README.md) | 中文

一个 Go 二进制、一份内容库、三种入口：终端界面、本地网页、以及给脚本和 AI 用的非交互命令行。内容英文优先、中文并存，新人和老手看的是同一页。

---

## 安装

从 [releases 页面](https://github.com/guaidao2/hack4all/releases) 下载一个单文件二进制即可 —— Linux、macOS、Windows，amd64 与 arm64 都有。内容库已经嵌进二进制、且是静态编译（`CGO_ENABLED=0`），不需要再装任何东西。

```bash
# Linux / macOS
chmod +x hack4all_v1.5.0_linux_amd64
./hack4all_v1.5.0_linux_amd64 -x "kerberos"

# Windows（PowerShell）
.\hack4all_v1.5.0_windows_amd64.exe -x "kerberos"
```

校验下载到的文件与发布时一致：

```bash
sha256sum -c hack4all_1.0.0_checksums.txt         # Linux
shasum -a 256 -c hack4all_1.0.0_checksums.txt     # macOS
```

想自己编译，见下面的 [编译](#编译) 一节。

## 三种入口，一份内容库

| 入口 | 命令 | 用途 |
|---|---|---|
| 终端界面 | `hack4all` | 在终端里读：左边搜索，右边正文，`Tab` 切语言 |
| 本地网页 | `hack4all web` | 在浏览器里读，地址 `http://127.0.0.1:8080` |
| 命令行 | `hack4all -x "查询"` | 在 shell 里取一条，或给工具 / AI 调用 |

```bash
# 交互式
hack4all
hack4all --lang zh                 # 直接用中文启动终端界面

# 本地网页（默认只绑 127.0.0.1，无遥测、无后端）
hack4all web
hack4all web --addr 127.0.0.1:9000 --no-open
hack4all web --port 0              # 端口被占用时自动挑一个空闲的
hack4all web --host 0.0.0.0        # 共享给局域网（会把可用地址打出来）

# 非交互：取一条技术点，直接在终端里渲染
hack4all -x "kerberos"
hack4all -x "NTLM 中继" --lang zh

# 非交互：结构化输出，给脚本和 AI 用
hack4all -x "cloud metadata" --json
hack4all -x "ssrf" --json --limit 3 | jq '.matches[].id'

# 浏览
hack4all list
hack4all list --category offensive/web

# 按字段收窄搜索
hack4all -x "category:offensive/credential-access"
hack4all -x "attck:T1558"
hack4all -x "tool:hashcat platform:linux"

# 先看结构，再决定读不读全文
hack4all -x "kerberos" --outline
hack4all -x "category:offensive" --outline --limit 0   # 整本书的骨架

# 校验内容库（CI 跑的就是这个）
hack4all check
hack4all check --strict
```

退出码：`0` 成功，`1` 真实错误（参数错、内容读不出来），`2` 查询执行了但没有匹配 —— 与 `grep` 同一套约定，脚本因此能区分"没找到"和"出错了"。

### 查询语法

空格分隔的词之间是「与」关系。字段前缀会在排序之前先收窄范围，这也是 `-x` 对脚本和 agent 好用的原因：它能问一个精确的问题，而不必指望自由文本的排序恰好合意。

| 前缀 | 匹配 |
|---|---|
| `category:offensive/web` | 该分类及其子分类 |
| `tag:kerberos` | 含该标签 |
| `tool:hashcat` | 含该工具 |
| `attck:T1558` | 含该 ATT&CK 编号 |
| `platform:windows` | 含该平台 |
| `difficulty:intermediate` | 该难度 |
| `id:kerberoasting` | 精确的技术点 id |

字段值是**大小写不敏感的子串匹配**，所以 `attck:T1558` 能找到 `T1558.003`。无法识别的前缀（比如 `foo:bar`）会退化成普通文本词：查询永远不会被一个工具不认识的字段悄悄吞掉。中英两个语料是**一起**建索引的，所以用中文查询能命中英文正文里含该词的条目，反之亦然。

### 网页界面

`hack4all web` 把同一份内容库做成一个页面：分类树、搜索、带目录（由正文小节标题生成）的详情区，每个代码块还有个复制按钮。默认只绑 `127.0.0.1`，没有后端、没有遥测、没有 CDN、没有构建步骤 —— 完全断网的机器上也能用。

红队战术地图有一个单独的页面 `/map.html`：`/api/tactical-map` 把同一份 Markdown 解析成结构，页面把 15 个阶段画成时间线、把横穿各阶段的面画成卡片，检查项可以勾选，进度只存在你自己的浏览器里。

### 终端界面

交互视图是把 Markdown **渲染**出来，而不是原样打印：标题去掉井号、表格变成对齐的列、代码块变成缩进区块、列表变成项目符号、强调变成颜色。`Tab` 切换显示语言，且不会丢掉你在列表里的位置。

渲染是这个项目自己写的，没有用通用 Markdown 库，原因有两条实测依据：常见终端渲染器**不会给中文折行**（中文没有空格可断，整段就会溢出被裁），而且它们会绘制文档背景色，会盖掉你设的透明终端。`hack4all -x` 仍然输出原始 Markdown —— 转义序列对管道没有意义。

---

## 内容格式

一个技术点 = 一个 Markdown 文件。两种语言放在同一个文件里，用两个注释标记切分。放在一起（而不是 `en/` 和 `zh/` 两棵树）意味着译文不会悄悄偏离它所属的原文。

```markdown
---
id: kerberoasting
title_en: Kerberoasting
title_zh: Kerberoasting：Kerberos 服务票据离线破解
summary_en: One-line English summary.
summary_zh: 一句话中文摘要。
tags: [active-directory, kerberos, windows]
tools: [impacket-GetUserSPNs, Rubeus, hashcat]
attck: [T1558.003]
platform: [windows, linux]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Why it works
...English body, with fenced code blocks and tables...

<!-- lang:zh -->
### 原理
...中文正文，与英文信息量对等...
```

| frontmatter 字段 | 必填 | 说明 |
|---|---|---|
| `id` | 否 | 默认取文件名。必须唯一；`-x` 精确匹配的就是它 |
| `title_en` / `title_zh` | 至少一个 | 列表和详情页显示 |
| `summary_en` / `summary_zh` | 否 | 一到两行，显示在正文上方 |
| `tags` | 否 | 自由填写；参与搜索 |
| `tools` | 否 | 该技术点涉及的工具名 |
| `attck` | 否 | ATT&CK 编号，如 `T1558.003` |
| `platform` | 否 | `windows`、`linux`、`cloud` 等 |
| `difficulty` | 否 | `beginner` / `intermediate` / `advanced` |
| `updated` | 否 | `YYYY-MM-DD` |

> **关于 YAML 的一个提醒。** frontmatter 的值里出现裸 `": "` 会被当成嵌套键，整个文件就加载不出来了。改写措辞，或者给值加引号：`summary_en: "Careful here: this is fine."`。真发生时 `hack4all check` 会指出来 —— 这是手工写最容易犯的一个错。

**分类只来自目录，永远不来自 frontmatter。** 单一事实来源，目录树和元数据因此不可能对不上：

```
content/topics/offensive/credential-access/kerberoasting.md
         └─────┘ └────────┘ └───────────────┘
           根       分类            技术点
```

加一个技术点就是加一个文件。加一个分类就是加一个目录。不用改 Go 代码，没有索引要重新生成。

以 `_` 或 `.` 开头的草稿文件与目录会被加载器跳过。

---

## 项目结构

```
cmd/hack4all/        CLI 入口：子命令分发、参数、-x 模式
internal/core/       共用的库：解析、索引、检索、JSON 结构
internal/tui/        交互式终端界面（bubbletea）
  markdown.go        终端 Markdown 渲染器：样式与折行在同一次遍历里完成
internal/web/        HTTP 服务 + JSON API + 内嵌的单页前端
content/             内容库，会被嵌进二进制
  embed.go           go:embed 把 topics/ 打进去
  topics/            一个技术点一个 Markdown 文件
```

`internal/core` 是唯一知道"一个技术点是什么"的地方。每个前端都从它读，没有任何前端自己存一份数据 —— 这条规则就是三种入口不会互相跑偏的原因。

`hack4all -x --json` 和网页 API 返回的是**同一个** `core.SearchResponse` 结构。

---

## 编译

需要 Go 1.24 以上。

```bash
go build -o bin/hack4all ./cmd/hack4all
./bin/hack4all -x "kerberos"
```

交叉编译静态二进制（内容库已内嵌，所以这一个文件就是全部）：

```bash
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o dist/hack4all-linux-amd64   ./cmd/hack4all
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/hack4all-windows-amd64.exe ./cmd/hack4all
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o dist/hack4all-darwin-arm64  ./cmd/hack4all
```

### 不重新编译就能预览内容

`--content DIR` 从指定目录读内容，而不是用内嵌的那份，所以贡献者改完 Markdown 可以立刻生效：

```bash
hack4all --content ./content/topics -x "kerberos"
hack4all web --content ./content/topics
```

`DIR` 下的目录结构与 `content/topics/` 完全一致。

---

## 关于分发的一个提醒

攻击技术类的 Markdown 容易触发杀毒软件与 EDR 的内容启发式规则，文件在传输途中可能被隔离。等这个项目开始发布归档包时，值得考虑用加密 zip（口令公开写明）而不是裸 tarball —— 加密在这里不是访问控制，只是让扫描器不要悄悄把内容吃掉。这是发布时再定的事。

---

## 状态

自 1.0 起稳定。三种入口端到端都能跑，终端界面能渲染 Markdown，内容格式已冻结且由 `hack4all check` 守着。内容库现在有 **140 个技术点、26 个分类**（含新手入门轨与详解篇），还在继续加。如果你正准备开始或刚结束一次红队项目，先看战术地图 —— `hack4all -x id:red-team-tactical-map` —— 它是一份覆盖查漏清单，而不是单个技术点。网页界面把同一份文件画成 `/map.html`：阶段是时间线、检查项可勾选，还有一个可以一直开着的工作进度条。

标签可以带中文词，也可以带英文词（`越权`、`未授权访问`）—— 因为读者搜索时用的是他思考的那门语言，而由文件名派生的 id 未必是他伸手去够的那个词。

## 范围与内容政策

这份指南是给**授权**的安全工作用的：渗透测试、红队项目、范围内的 bug bounty、CTF，以及检测同样这些手法的防守工程。它记录的内容，与 MITRE ATT&CK、厂商研究、会议演讲等公开资料记录的素材一致。

每条贡献都遵守三条编辑规则：

- **讲技术，不给武器。** 说明一类攻击是怎么工作的、在防守方眼里长什么样、以及如何检测与缓解。不要加入可直接运行的攻击链。证明"能访问"在范围内；任何效果只是破坏数据或打掉服务的，不在范围内。
- **检测不是可选项。** 每个技术点都要说清它会产生什么遥测。没有检测小节的条目就是未完成。
- **英文优先，中文并存。** 两种语言，按这个顺序，放在同一个文件里。

作者与贡献者不对误用负责。如果你在测试一个你没有书面许可测试的系统，这里的东西不是给你用的。

## 许可证

MIT —— 见 [LICENSE](LICENSE)。

Copyright (c) 2026 guaidao2 & coolmoon.
