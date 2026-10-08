---
id: infoleak-fundamentals
title_en: "Information Disclosure — Judged by What It Changes"
title_zh: "信息泄漏：按它能改变什么来判断"
summary_en: Not a mechanism but a result, so the question is what seeing this lets an attacker do next — credentials and source code change the answer, version banners do not. Measured — an exposed .git directory yields a secret deleted in a later commit, along with the whole history.
summary_zh: 它不是一种机制，而是一个结果，所以要问的是"看到它之后攻击者接下来能做什么" —— 凭据与源码会改变答案，版本横幅不会。实测：一个可读的 .git 目录交出了在后续提交里被删掉的密钥，以及整段历史。
tags: [web, information-disclosure, cwe-538, cwe-540, git, deployment]
tools: [curl, git, git-dumper, browser devtools]
attck: [T1592, T1213, T1552.001]
platform: [web]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### A result, not a mechanism

Every other entry in this series names a mechanism: a parser that disagrees, a check that runs in the wrong layer, a credential treated as an intent. This one names an **outcome**, and that difference is worth starting with because it decides how the class is judged.

The question is never "was something visible". It is:

> **What does seeing this let the attacker do next?**

Answer that, and the finding sorts itself. Measured on a lab whose file endpoint is correctly restricted to the site root — no traversal, no bypass — and which nonetheless served `.env`, `.git/`, an editor's swap file and two script backups, all returning `200`:

| What leaked | What it changes |
|---|---|
| **Credentials** (`.env`, config files, backups of them) | An attacker who was outside the application is now inside it |
| **Source code** (`.git/`, `.bak` of an application file) | Every check becomes readable, every hard-coded secret becomes available, and the search for the next bug stops being blind |
| **Internal structure** (stack traces, directory listings, version banners) | Reconnaissance that would have taken hours collapses into one request |
| **Personal data** | A compliance matter that exists whether or not anything is exploited |

**The first two rows are what make this class worth its own entry**, and the last row is why it cannot be judged by exploitability alone. A version banner and a `.env` file are both "information disclosure", and almost nothing about them is comparable.

### It is not the same as path traversal

The lab states this explicitly, and the distinction is the reason this entry exists separately:

| | Path traversal | This |
|---|---|---|
| What the code does | Fails to restrict where a path may point | Restricts it correctly |
| What the attacker does | **Bypasses** the restriction | **Requests the file directly** |
| Where the fault is | The code | **The deployment** |

**There is no validation to defeat.** The endpoint resolved paths inside the root, exactly as written. The problem is that the root contained files that should never have been there — and the reason that is easy to miss is a reasonable belief held by whoever put them there: *"this file is not linked from anywhere, so nobody will find it."*

**The web server serves a directory, not a list of intended pages.** Every file in a served directory is reachable by anyone who guesses its name, and the set of names worth guessing is small and well known.

### The `.git` directory is the highest-value single item

An exposed `.git/` is not "one file leaked". It is the repository: the object store contains every version of every committed file, and the history contains things that were later removed.

Measured. A repository whose first commit contains a `.env` with a database password, an admin password and a live-looking API key, and whose second commit **removes that file and adds a `.gitignore`** — the cleanup a developer would reasonably think closes the matter. From the `.git/` directory alone:

```
history, in full:
  555d22e remove secrets from repo, add .env.example
  315ecf5 initial commit with config

git show HEAD~1:.env
  DB_PASSWORD=prod-db-pass-9f2a
  ADMIN_PASSWORD=ADMIN-SECRET-4471
  STRIPE_KEY=sk_live_51H8xQ2demo

a plain search across all objects, without naming a file:
  object c642c0de... contains ADMIN_PASSWORD=ADMIN-SECRET-4471

commit metadata, also exposed:
  555d22e dev <dev@example> remove secrets from repo, add .env.example
  315ecf5 dev <dev@example> initial commit with config
```

Three conclusions, and they matter more than the payloads:

**Deleting a file in a later commit does not remove it from the history.** The secret is in the object database under the commit that added it, and it stays there. This is the single most important fact about `.git/` exposure and the reason a leaked repository requires **credential rotation**, not just a deployment fix.

**An attacker does not need to know which file to look for.** Searching the objects for a string — a password pattern, a key prefix, a hostname — finds secrets without any knowledge of the project's structure. Measured: the admin password came out of an object whose filename was never mentioned.

**And the history is also a map.** Commit messages describe what was fixed and when, which tells an attacker where to look for a fix that might be incomplete.

`/.git/HEAD` alone is enough to identify that a repository is exposed; recovering the content is a matter of walking the object store, which is what tools in this space automate.

### Where to look

The list is short, well known, and worth having as a checklist rather than as intuition. Measured against the lab, **every one of the following returned `200`**:

| Category | Paths |
|---|---|
| **Version control** | `/.git/HEAD`, `/.git/config`, `/.svn/entries`, `/.hg/` |
| **Environment files** | `/.env`, `/.env.local`, `/.env.production`, `/.env.bak` |
| **Config backups** | `/config.php.bak`, `/app.py.bak`, `/web.config.old` |
| **Editor leftovers** | `/index.html.swp`, `/page.php~`, `/.index.html.un~` |
| **Deployment** | `/deploy.sh`, `/Makefile`, `/Dockerfile`, `/.dockerignore` |
| **Archives** | `/backup.zip`, `/site.tar.gz`, `/db.sql`, `/dump.sql` |
| **Dependency manifests** | `/package.json`, `/requirements.txt`, `/composer.lock` |
| **IDE project dirs** | `/.vscode/`, `/.idea/`, `/.project` |
| **Logs** | `/debug.log`, `/error.log`, `/access.log` |
| **Test leftovers** | `/test.php`, `/phpinfo.php`, `/.DS_Store` |

Two entries deserve a note. **Dependency manifests** disclose exact versions, which is the vulnerable-components entry's reconnaissance step. And **`robots.txt`** is worth reading for the opposite reason to the one intended: it exists to tell crawlers which paths not to index, so it frequently names exactly the paths an attacker wants — the lab's own `robots.txt` lists `/admin/` and `/files/`.

### Detection and mitigation

- **Alert on a `200` for any path containing a directory beginning with a dot.** `.git`, `.svn`, `.env`, `.vscode` — no legitimate user requests these, and the expected answer is `404`.
- **Alert on requests for backup and editor suffixes.** `.bak`, `.old`, `.orig`, `.swp`, `~`, `.zip`, `.sql`, `.tar.gz` — a request for any of these is either a scanner or a person, and it costs nothing to see.
- **And alert on credentials appearing in responses.** A pattern for key-value pairs whose names suggest secrets, a known key prefix, a private-key header — this is the outcome check, and it holds whatever the request looked like.
- **For mitigation, keep the served directory to what is served.** An application's document root should contain the files the application intends to publish. Source, configuration, tests, deployment scripts and version control belong outside it, which is a deployment decision rather than a code change.
- **Ship the repository rather than copying it.** A `git archive` or an equivalent export produces the committed files without the object store and without the history, and it is the difference between deploying a version and deploying the repository. This is the single most effective control against the measured case.
- **Keep secrets out of the repository in the first place.** Environment-injected configuration, a secret manager, or a file that is generated at deploy time. This is what makes an accidental `.git/` exposure a source-disclosure problem instead of a credential-disclosure problem — a lower grade of bad.
- **And when a repository or a credential file has been exposed, rotate the credentials.** Cleaning the deployment does not reach the object database, and the secret stays readable in every clone that already exists; a review that fixes the path and leaves the password in place has fixed half of it.
- **Treat a leaked source tree as a change in what you must assume.** Once the code is readable, the attacker's model of the application is as good as yours — which means the review should look for what is exploitable **given knowledge of the source**, not only for what is exploitable blind.

<!-- lang:zh -->
### 一个结果，不是一种机制

本系列其他每一篇都点出一种机制：一个意见不一致的解析器、一道跑错层的检查、一个被当成意图的凭据。这一篇点的是一**个结果**，而这个区别值得放在开头，因为它决定了这一类该怎么判断。

问题从来不是"有没有东西被看见"，而是：

> **看到它之后，攻击者接下来能做什么？**

回答了它，这条发现自己就会归类。在一个文件接口**被正确地限制在站点根目录里**（没有穿越、没有绕过）的靶场上实测 —— 而它照样发出了 `.env`、`.git/`、一个编辑器的交换文件、以及两份脚本备份，全部返回 `200`：

| 泄漏了什么 | 它改变了什么 |
|---|---|
| **凭据**（`.env`、配置文件、它们的备份） | 一个本来在应用之外的攻击者，现在在应用之内 |
| **源码**（`.git/`、某个应用文件的 `.bak`） | 每一道检查都变得可读、每一个硬编码密钥都变得可用，而寻找下一个 bug 不再是盲找 |
| **内部结构**（调用栈、目录列表、版本横幅） | 本来要花几小时的侦察，塌缩成一个请求 |
| **个人信息** | 一件与"有没有被利用"无关的合规事项 |

**前两行才是这一类值得单独成篇的原因**，而最后一行是为什么它不能只按可利用性来判断。一个版本横幅和一个 `.env` 文件都叫"信息泄漏"，而它们之间几乎没有可比之处。

### 它和路径穿越不是一回事

靶场把这一点明说了，而这个区分正是这一篇单独存在的原因：

| | 路径穿越 | 这一篇 |
|---|---|---|
| 代码做了什么 | 没能限制路径可以指向哪里 | **正确限制了** |
| 攻击者做了什么 | **绕过**那道限制 | **直接请求那个文件** |
| 错在哪 | 代码 | **部署** |

**没有校验可以击败。** 那个接口在根目录之内解析路径，完全按写的那样。问题在于根目录里含有本不该在那里的文件 —— 而这一点容易被忽略，是因为把它们放进去的人抱着一个很合理的想法：*"这个文件没有任何地方链接它，所以没人会找到。"*

**web 服务器服务的是一个目录，不是一份"打算公开的页面"清单。** 一个被服务的目录里，每一个文件都能被任何猜中它名字的人拿到，而值得猜的名字集合很小、而且众所周知。

### `.git` 目录是单项价值最高的那个

一个可读的 `.git/` 不是"泄漏了一个文件"，它就是那个仓库：对象库里含有每一个已提交文件的每一个版本，而历史里含有后来被移除的东西。

实测。一个仓库，第一次提交里含有一份 `.env`（数据库口令、管理员口令、一个看着像真的 API key），第二次提交**删掉了那个文件并加了一个 `.gitignore`** —— 一个开发者会合理认为"事情已经了结"的清理动作。只凭那个 `.git/` 目录：

```
历史，完整地：
  555d22e remove secrets from repo, add .env.example
  315ecf5 initial commit with config

git show HEAD~1:.env
  DB_PASSWORD=prod-db-pass-9f2a
  ADMIN_PASSWORD=ADMIN-SECRET-4471
  STRIPE_KEY=sk_live_51H8xQ2demo

在所有对象里做一次普通搜索，不指名任何文件：
  对象 c642c0de... 含 ADMIN_PASSWORD=ADMIN-SECRET-4471

提交元数据，同样暴露：
  555d22e dev <dev@example> remove secrets from repo, add .env.example
  315ecf5 dev <dev@example> initial commit with config
```

三个结论，而它们比 payload 本身更要紧：

**在后续提交里删除一个文件，不会把它从历史里移除。** 那个密钥在对象库里、在添加它的那次提交之下，而它会一直待在那里。这是关于 `.git/` 暴露最重要的一个事实，也是"仓库泄漏需要**轮换凭据**、而不只是修部署"的原因。

**攻击者不需要知道该找哪个文件。** 在对象里搜一个字符串 —— 一个口令模式、一个 key 前缀、一个主机名 —— 就能找到密钥，不需要对这个项目的结构有任何了解。实测里那个管理员口令是从一个文件名从未被提及的对象里出来的。

**而历史同时也是一张地图。** 提交信息描述了"什么被修了、什么时候修的"，这告诉攻击者去哪里找一个可能修得不彻底的修复。

单是 `/.git/HEAD` 就足以判断有一个仓库暴露了；把内容还原出来则是遍历对象库的事，而这件事在这个领域里有现成的工具。

### 该去哪里找

这份清单很短、众所周知，值得当成一张检查表而不是凭直觉。对着靶场实测，**下面每一个都返回了 `200`**：

| 类别 | 路径 |
|---|---|
| **版本控制** | `/.git/HEAD`、`/.git/config`、`/.svn/entries`、`/.hg/` |
| **环境文件** | `/.env`、`/.env.local`、`/.env.production`、`/.env.bak` |
| **配置备份** | `/config.php.bak`、`/app.py.bak`、`/web.config.old` |
| **编辑器残留** | `/index.html.swp`、`/page.php~`、`/.index.html.un~` |
| **部署相关** | `/deploy.sh`、`/Makefile`、`/Dockerfile`、`/.dockerignore` |
| **打包产物** | `/backup.zip`、`/site.tar.gz`、`/db.sql`、`/dump.sql` |
| **依赖清单** | `/package.json`、`/requirements.txt`、`/composer.lock` |
| **IDE 工程目录** | `/.vscode/`、`/.idea/`、`/.project` |
| **日志** | `/debug.log`、`/error.log`、`/access.log` |
| **测试残留** | `/test.php`、`/phpinfo.php`、`/.DS_Store` |

有两个条目值得注一句。**依赖清单**泄漏精确版本号，那正是过时组件那篇的侦察步骤。而 **`robots.txt`** 值得为了与它本意相反的原因去读：它存在的意义是告诉爬虫哪些路径不该被索引，所以它经常恰好点出攻击者想要的路径 —— 靶场自己的 `robots.txt` 里就写着 `/admin/` 和 `/files/`。

### 检测与缓解

- **对任何含"以点开头的目录"的路径返回 `200` 告警。** `.git`、`.svn`、`.env`、`.vscode` —— 没有正当用户会请求这些，而期望的回答是 `404`。
- **对请求备份与编辑器后缀告警。** `.bak`、`.old`、`.orig`、`.swp`、`~`、`.zip`、`.sql`、`.tar.gz` —— 对其中任何一个的请求，要么是扫描器要么是个人，而看见它不花任何代价。
- **并且对"响应里出现凭据"告警。** 一个匹配"名字暗示秘密的键值对"的模式、一个已知的 key 前缀、一个私钥头 —— 这是结果层的检查，而无论那个请求长什么样它都成立。
- **缓解上，让被服务的目录只含被服务的东西。** 一个应用的文档根应当只含应用打算公开的文件。源码、配置、测试、部署脚本与版本控制属于它之外，而这是一个部署决定，不是一次代码改动。
- **发布仓库，而不是拷贝仓库。** `git archive` 或等价的导出会产生已提交的文件，但不含对象库、也不含历史 —— 这是"部署一个版本"与"部署那个仓库"之间的区别。它是针对实测那种情形最有效的一项控制。
- **首先，让密钥不进仓库。** 由环境注入的配置、一个密钥管理服务、或者一个部署时生成的文件。这正是让一次意外的 `.git/` 暴露成为"源码泄漏"而不是"凭据泄漏"的东西 —— 同样是坏，但低一级。
- **而当仓库或凭据文件已经暴露时，轮换凭据。** 清理部署到不了对象库，而那个密钥在每一个已经存在的克隆里都仍然可读；一次修好了路径、却把口令留在原地的处理，只修了一半。
- **把"源码已泄漏"当成一次"你必须假设的东西变了"。** 一旦代码可读，攻击者对应用的理解就和你一样好 —— 这意味着评审该去找**在知道源码的前提下**可利用的东西，而不只是盲测时能找到的东西。
