---
id: container-image-and-runtime
title_en: Container Images and Runtime
title_zh: 容器镜像与运行时
summary_en: An image is layered, content-addressed and immutable, so removing a file is a property of the merged view rather than of the image, and a tag is a pointer rather than a thing. Measured by building an image by hand — a secret deleted in a later layer and still present in the earlier one, configuration readable without running anything, and one tag resolving to two different manifests.
summary_zh: 一个镜像是分层的、按内容寻址的、不可变的，所以"删掉一个文件"是合并视图的性质而不是镜像的性质，而一个 tag 是一个指针而不是那个东西。这一篇手工构造一个镜像来实测 —— 一个在后一层被删掉、却仍然完整留在前一层里的秘密，不用运行就能读到的配置，以及一个 tag 对应两份不同的 manifest。
tags: [beginner, cloud, container, image, supply-chain]
tools: [docker, podman, skopeo, python3]
attck: [T1552, T1525]
platform: [cloud]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Layers are immutable, and a tag is a pointer

Two properties of how images are built account for most of the container mistakes worth calling security problems.

> **Nothing in a layer is ever removed. A deletion is a record in a newer layer that says "stop showing this". And a tag is a name that can point at a different image tomorrow.**

Both are measured below by building an image by hand from a tar, a gzip stream and some sha256 digests, so that each layer's contents are visible rather than summarised by a tool.

### Part 1: the shape of it

| Element | What it is |
|---|---|
| layer | a tar archive, **content-addressed by the digest of its compressed bytes** |
| image configuration | JSON — environment, user, entry point, and the build history |
| manifest | the list of layers plus a pointer to the configuration |
| a tag | **a mutable name** pointing at a manifest digest |
| a running container | the image's layers plus **one writable layer on top** |

**And two facts follow from the content addressing.** A layer that is byte-identical to one already stored is shared rather than duplicated, and **a deletion cannot remove anything from a layer that has already been published** — because its digest is a function of its bytes, and changing them would produce a different layer.

**Which is why the removal has to be expressed as a new layer**, and why the entry's central measurement is what that new layer does and does not do.

### Part 2: a deletion is a record, not a removal

Measured, an image built in two layers: the first writes a configuration file containing a password, and the second deletes it in the ordinary way.

```
layer 1    etc/os-release              12 bytes
           app/config.env              48 bytes   DB_PASSWORD=prod-db-password
           app/main.py                 12 bytes

layer 2    app/main.py                 12 bytes
           app/readme.txt              13 bytes
           app/.wh.config.env           0 bytes   <- the deletion record
```

**Applying the layers in order, the file disappears from the merged view** — the record named `.wh.` plus the original name removes it:

```
merged view: app/main.py, app/readme.txt, etc/os-release
app/config.env present in the merged view?  no
app/config.env present in layer 1?          yes
```

**And extracting it from layer 1 returns the password in full**, because layer 1 is exactly what it was when it was published.

> **"Deleted from the image" is a statement about the merged view. The layer still holds every byte of it, and any layer can be fetched on its own.**

**And the build history says so out loud.** Measured, the configuration's history contains the step that did it:

```
ADD app/
RUN rm app/config.env
```

**Which is a free audit trail and a free disclosure** — a reader learns both that a file was removed and, if the step had named a variable or a URL, something about what it contained.

**And the consequence is a design rule rather than a cleanup**: a secret that must not be in the image must never be written into any layer, which means **a build stage that needs it produces the artefact and the final image is assembled without it**, or the secret is mounted at build time rather than copied.

### Part 3: one tag, two images

Measured, two builds published under the same name:

| Build | Manifest digest |
|---|---|
| first | `sha256:377765e5...` |
| second | `sha256:6752a432...` |

**The name is identical and the contents are not.** The tag is a pointer stored in a registry, and whoever can push can move it.

> **"We deployed app:latest" is not a reproducible statement. "We deployed sha256:6752a432..." is.**

**Which makes the tag a supply-chain surface rather than a convenience**: an image pulled by name is whatever that name pointed at when the pull happened, and the same pipeline run twice can produce two different deployments.

**And it decides what signing can even mean.** A signature over a name attests to nothing, because the name can be reused. **A signature is over a digest** — and verification then consists of checking that the thing you are about to run has the digest that was signed.

### Part 4: layers are shared between images

Measured, a second image referencing the same base:

```
this image's layer 1        sha256:e4e54502...
the other image's layer 1   sha256:e4e54502...
```

**Identical, because a layer is stored once and referenced by digest.** Which has three consequences worth separating.

**Distribution is cheap** — the base is transferred once for every image built on it.
**A change to the base changes every image built on it**, so "our image did not change" is a claim that has to be checked rather than assumed.
**And a secret in a base layer is in every image built on that base**, which is Part 2 applying at a larger scale than the one build where it was noticed.

### Part 5: the configuration is readable without running anything

Measured, straight out of the image configuration:

```
Env     = ["PATH=/usr/bin", "API_KEY=sk_live_in-the-image-config"]
User    = ""            (empty means the image's default, which is root)
history = ["ADD app/", "RUN rm app/config.env"]
```

**None of that requires starting a container.** Pulling the image is enough, and that is the access question: **a secret in the image configuration is exposed to everyone who can pull it**, which is a larger set than the set who can run it.

**And `User` being empty is not a neutral default.** An image that does not name a user runs as the image's default, which is root in every common base, **so the container's process has root inside its own namespace and whatever capabilities the runtime granted** — and the entry's mitigation list is mostly about how much that is worth.

### Part 6: the runtime, and what the isolation is

**A container is a process with a restricted view, not a virtual machine.** The isolation comes from namespaces, which decide what it can see, and from control groups, which decide how much it can use — **and the kernel it talks to is the host's**.

> **A container isolates what a process can see and how much it can use. It does not isolate what the kernel is.**

**Which is why a kernel-level escape affects the host rather than the container**, and why the runtime's configuration matters as much as the image's. Three of those settings account for most of the practical difference:

**The user**, since root in a container with a mounted host path is root on those files.
**The capabilities**, since a default grant covers more than most workloads need and removing them is the difference between a compromise confined to the container and one that changes the host's network or clock.
**And mounted sockets**, since a workload with access to the container runtime's own socket can ask it to start another container — with the host's privileges, mounted wherever the requester chooses. **That is not an escape from the isolation; it is a request made through a channel that was deliberately opened.**

### Part 7: where the image comes from

**The registry's access control protects the layers.** An image is a set of layers plus metadata that references them, and the layers can be fetched on their own if a client is entitled to them. **So the unit that is protected is the layer, and every layer carries its own full history.**

**Which is why a private registry does not make an image's contents safe to write things into.** The protection is over who can fetch rather than over what is inside, and the measured deletion shows that what is inside is everything ever written.

**And push access is deployment access.** Anyone who can move a tag can change what a pipeline deploys, which is why the push permission is worth the same review as the deploy permission — and why verifying a digest at deploy time is what separates "we deployed an image" from "we deployed the image we reviewed".

### Part 8: what follows for security

**A secret that entered any layer is in the image**, measured as a password extracted from the layer that a later record hid. **So the control has to be at build time**: never write the secret into a layer, assemble the final image from an artefact rather than from the environment that made it, and treat a found secret as one to rotate rather than one to remove.

**And scanning has to look at the layers rather than the merged view.** A scanner that inspects the final filesystem sees exactly what the merged view shows — **and the measured file is not in it**. **The layers are where the history is**, including the ones the merged view no longer mentions.

**The tag is a pointer, so deployments should name a digest.** Measured, one name pointed at two different manifests. **A pipeline pinned to a digest is reproducible, and a signature is meaningful only over a digest** — because a name can be moved after the signature was made.

**And the runtime's defaults are the other half.** A container that runs as root, keeps the default capabilities and can reach the runtime's socket **is a container whose isolation is nominal**, and none of that is visible in the image's contents. **The image says what will run; the runtime configuration says what it can do while running.**

**And the base image is a shared dependency with a wide edge.** Measured, two images referencing the same layer. **So a base update is a fleet update, and a secret in a base is in every image built on it** — which is why the base is worth the same review as the code, and why its digest belongs in the pipeline rather than its name.

### Detection and mitigation

- **Scan the layers, not only the merged filesystem.** Measured, a file absent from the merged view was present in full in an earlier layer.
- **Read the image history in review**, since the measured history names the step that removed the file and would name a stage that handled a secret.
- **Deploy by digest and record it**, because the measured tag pointed at two different manifests and a signature over a name attests to nothing.
- **Sign digests and verify before running**, so that the artefact reviewed and the artefact run are provably the same.
- **Protect push access as deployment access**, since moving a tag changes what a pipeline deploys.
- **Report containers running as the default user**, and set a user in the image so the default is not root.
- **Remove capabilities the workload does not use**, which is what makes a compromise inside the container stay inside it.
- **Never mount the container runtime's socket into a workload**, since a request through it is not an escape but a granted request.
- **Use a multi-stage build so the stage that handles secrets is not the stage that is published**, and mount secrets at build time instead of copying them.
- **Pin the base image by digest and rebuild when it changes**, because the measured layer is shared and a base update is a fleet update.
- **Keep the image minimal**, since every layer is another place a removed file still exists.
- **And treat a secret found in any layer as disclosed**, because the layer is immutable and the digest that was signed covers the bytes that contain it.

<!-- lang:zh -->
### 层不可变，而 tag 是一个指针

镜像构建方式里的两条性质，解释了大多数值得当成安全问题来讲的容器失误。

> **层里的东西从不被移除。一次删除是新的一层里的一条记录，说的是"别再显示它"。而一个 tag 是一个名字，它明天可以指向另一个镜像。**

两条都在下面实测：用一个 tar、一段 gzip 流和几个 sha256 摘要手工构造一个镜像，这样每一层里有什么是看得见的，而不是由工具总结出来的。

### 第一部分：它的形状

| 元素 | 它是什么 |
|---|---|
| 层 | 一个 tar 归档，**按它压缩后字节的摘要寻址** |
| 镜像配置 | JSON —— 环境变量、用户、入口点、以及构建历史 |
| manifest | 层的清单，加上一个指向配置的指针 |
| 一个 tag | **一个可变的名字**，指向一个 manifest 摘要 |
| 运行中的容器 | 镜像的那些层，**顶上再加一个可写层** |

**而按内容寻址推出两个事实。** 一个与已存的层字节完全相同的层会被共用而不是重复存一份；而**一次删除无法从已经发布出去的层里移走任何东西** —— 因为它的摘要是它那些字节的函数，改了字节就成了另一个层。

**这就是为什么移除必须表达成一个新的层**，也是为什么这一篇的核心实测是"那个新层做了什么、没做什么"。

### 第二部分：一次删除是一条记录，不是一次移除

实测，一个分两层构建的镜像：第一层写下一个含口令的配置文件，第二层用最普通的方式把它删掉。

```
第 1 层    etc/os-release              12 字节
           app/config.env              48 字节   DB_PASSWORD=prod-db-password
           app/main.py                 12 字节

第 2 层    app/main.py                 12 字节
           app/readme.txt              13 字节
           app/.wh.config.env           0 字节   <- 那条删除记录
```

**按顺序叠加这些层，那个文件从合并视图里消失了** —— 那条以 `.wh.` 加上原文件名命名的记录把它移除了：

```
合并视图: app/main.py、app/readme.txt、etc/os-release
app/config.env 在合并视图里吗？   不在
app/config.env 在第 1 层里吗？    在
```

**而只从第 1 层里把它取出来，返回的是完整的口令**，因为第 1 层就是它被发布时的那份东西。

> **"从镜像里删掉了"是一句关于合并视图的话。那个层仍然持有它的每一个字节，而任何一层都可以被单独取下来。**

**而构建历史把这件事明说了。** 实测，配置里的历史含着做这件事的那一步：

```
ADD app/
RUN rm app/config.env
```

**这既是一份免费的审计线索，也是一处免费的信息泄露** —— 读的人既知道了有个文件被移除，如果那一步写明了某个变量或 URL，还知道了它大概装着什么。

**而后果是一条设计规则、而不是一次清理**：一个不能留在镜像里的秘密，就不能被写进任何一层。这意味着**需要它的那个构建阶段生产出产物，而最终镜像是在没有它的情况下组装出来的**，或者那个秘密在构建时被挂载进来、而不是被复制进来。

### 第三部分：一个 tag，两个镜像

实测，两次构建以同一个名字发布：

| 构建 | manifest 摘要 |
|---|---|
| 第一次 | `sha256:377765e5...` |
| 第二次 | `sha256:6752a432...` |

**名字一模一样，内容不是。** tag 是存在仓库里的一个指针，而谁能推谁就能移动它。

> **"我们部署的是 app:latest"不是一句能复现的话。"我们部署的是 sha256:6752a432..."才是。**

**这就让 tag 成为一个供应链面、而不是一个便利**：一个按名字拉下来的镜像，就是那个名字在拉取那一刻指向的东西；同一条流水线跑两次可以产出两次不同的部署。

**而它也决定了"签名"这件事能意味着什么。** 对一个名字的签名什么都证明不了，因为那个名字可以被复用。**签名是对一个摘要做的** —— 而验证就是检查"你将要运行的那个东西"是不是那个被签过的摘要。

### 第四部分：层在镜像之间是共用的

实测，另一个镜像引用了同一个基础层：

```
本镜像的第 1 层        sha256:e4e54502...
另一个镜像的第 1 层     sha256:e4e54502...
```

**完全相同，因为一个层只存一份、按摘要被引用。** 这带来三个值得分开说的后果。

**分发很便宜** —— 基础层为每一个建立在它之上的镜像只传一次。
**基础层一变，建立在它之上的每一个镜像都变**，所以"我们的镜像没变"是一句要核实、而不能默认的话。
**而基础层里的一个秘密，就在每一个以它为基的镜像里** —— 那是第二部分在一场比"当初注意到它的那次构建"更大的规模上重演。

### 第五部分：不用运行任何东西就能读到配置

实测，直接从镜像配置里读出来：

```
Env     = ["PATH=/usr/bin", "API_KEY=sk_live_in-the-image-config"]
User    = ""            （空 = 用镜像的默认，也就是 root）
history = ["ADD app/", "RUN rm app/config.env"]
```

**这些全都不需要启动一个容器。** 拉得到镜像就够了，而这就是那个访问问题：**镜像配置里的一个秘密，对每一个能拉它的人都是暴露的**，而那个集合比"能运行它的人"更大。

**而 `User` 为空不是一个中性的默认值。** 一个没有指定用户的镜像会以镜像的默认用户运行，而在每一个常见基础镜像里那都是 root，**所以容器里的进程在它自己的命名空间里有 root、并且拥有运行时授予的那些能力** —— 而这一篇的缓解清单大半是关于"那值多少"的。

### 第六部分：运行时，以及这里的隔离到底是什么

**容器是一个视野受限的进程，不是一台虚拟机。** 隔离来自命名空间（决定它能看见什么）与控制组（决定它能用多少）—— **而它对话的内核是宿主的内核**。

> **容器隔离的是"一个进程能看见什么、能用多少"。它不隔离"内核是什么"。**

**这就是为什么一次内核层的逃逸影响的是宿主、而不是容器**，也是为什么运行时的配置与镜像本身一样要紧。那些设置里有三样解释了实践中的大部分差别：

**用户** —— 在一个挂了宿主路径的容器里，root 对那些文件就是 root。
**能力** —— 默认授予的那些比大多数工作负载需要的多，去掉它们就是"一次入侵被关在容器里"与"它能改宿主的网络或时钟"之间的差别。
**以及被挂进来的套接字** —— 一个能访问容器运行时自己那个套接字的工作负载，可以要求它再起一个容器，带着宿主的权限、把请求者指定的任何东西挂进去。**那不是一次对隔离的逃逸；那是一次通过一条被有意打开的通道发出的请求。**

### 第七部分：镜像从哪里来

**仓库的访问控制保护的是层。** 一个镜像是一组层、加上引用它们的元数据，而层可以被单独取下来（如果客户端有那个权限）。**所以被保护的单位是层，而每一个层都带着它自己的完整历史。**

**这就是为什么一个私有仓库并不让"往镜像里写东西"变得安全。** 那层保护是关于"谁能取"，而不是关于"里面是什么"；而实测那次删除表明，里面是曾经写进去的一切。

**而推的权限就是部署的权限。** 任何能移动一个 tag 的人都能改变一条流水线部署什么，这就是为什么推的权限值得与部署的权限同样的评审 —— 也是为什么在部署时校验摘要，是"我们部署了一个镜像"与"我们部署了我们评审过的那个镜像"之间的分界。

### 第八部分：从这些机制推出的安全观念

**任何进过某一层的秘密都在这个镜像里**，实测为从一个被后来的记录藏起来的口令、从那一层里被完整取出。**所以控制点必须在构建时**：绝不把秘密写进任何一层，最终镜像从一个产物组装、而不是从生产它的那个环境组装，并且发现秘密时把它当成要轮换的东西、而不是要移除的东西。

**而扫描必须看层，而不是看合并视图。** 一个只看最终文件系统的扫描器，看到的正是合并视图显示的那些 —— **而实测那个文件不在其中**。**层才是历史所在的地方**，包括合并视图不再提到的那些。

**tag 是指针，所以部署应该写摘要。** 实测，一个名字指向了两份不同的 manifest。**一条固定在摘要上的流水线是可复现的，而签名只有在针对摘要时才有意义** —— 因为一个名字可以在签名之后被移动。

**而运行时的默认值是另一半。** 一个以 root 运行、保留着默认能力、又能碰到运行时套接字的容器，**是一个隔离只是名义上的容器** —— 而这些在镜像的内容里一样都看不见。**镜像说的是"什么会运行"，运行时配置说的是"运行的时候它能做什么"。**

**而基础镜像是一个边缘很宽的共享依赖。** 实测，两个镜像引用了同一层。**所以一次基础层的更新是一次机群更新，而基础层里的一个秘密在每一个以它为基的镜像里** —— 这就是为什么基础层值得与代码一样的评审，也是为什么进流水线的应该是它的摘要而不是名字。

### 检测与缓解

- **扫层，不只扫合并后的文件系统。** 实测，一个在合并视图里不存在的文件，在更早的一层里完整存在。
- **在评审里读镜像历史**，因为实测那段历史写明了移除那个文件的那一步，也会写明某个处理过秘密的阶段。
- **按摘要部署并记录下来**，因为实测那个 tag 指向过两份不同的 manifest，而对一个名字的签名什么都证明不了。
- **对摘要签名并在运行前验证**，这样被评审的那个产物与被运行的那个产物可证明是同一个。
- **把推的权限当成部署权限来保护**，因为移动一个 tag 就会改变流水线部署什么。
- **报出"以默认用户运行"的容器**，并在镜像里指定一个用户，让默认不是 root。
- **去掉工作负载用不到的能力**，那才是让"容器里出的事留在容器里"的东西。
- **绝不把容器运行时的套接字挂进工作负载**，因为透过它发出的请求不是一次逃逸，而是一次被批准的请求。
- **用多阶段构建，让处理秘密的那个阶段不是被发布的那个阶段**，并在构建时把秘密挂载进来、而不是复制进来。
- **按摘要固定基础镜像，并在它变化时重建**，因为实测层是共用的，而一次基础层更新是一次机群更新。
- **让镜像尽量小**，因为每一层都是"一个被删掉的文件仍然存在"的又一处地方。
- **并且把在任何一层里发现的秘密当成已泄露**，因为层不可变，而被签的那个摘要覆盖的正是含着它的那些字节。
