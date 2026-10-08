---
id: iot-embedded-security
title_en: IoT and Embedded Device Security
title_zh: IoT 与嵌入式设备安全
summary_en: An IoT device is a small computer with a large attack surface and nobody to patch it. The findings are usually in the firmware you can download, the debug header nobody removed, and the cloud API the device blindly trusts.
summary_zh: IoT 设备就是一台小电脑：攻击面很大，却没人给它打补丁。真正的发现通常在你能下载到的固件里、在没人拆掉的调试接口上，以及在设备盲目信任的那套云端 API 上。
tags: [iot, embedded, firmware, uart, jtag, mqtt, hardware]
tools: [binwalk, unblob, flashrom, picocom, QEMU, FirmAE, Ghidra, Bettercap]
attck: [T1190, T1200]
platform: [embedded]
difficulty: advanced
updated: 2026-10-08
---

<!-- lang:en -->
### Five surfaces, and the order to approach them

| Surface | Typical findings | Effort |
|---|---|---|
| Firmware | Hardcoded credentials, private keys, backdoors, old components | Low, and needs no hardware |
| Network services | Default passwords, command injection in the web console | Low |
| Physical interfaces | UART console, JTAG, unreadable flash | Medium, needs hardware |
| Wireless | BLE pairing weaknesses, replayable RF commands | Medium to high |
| Cloud and mobile | The device API is usually the weakest link in the chain | Low |

Start with the firmware. It is a file, and it usually contains everything the other surfaces would reveal.

### Firmware

```bash
binwalk firmware.bin                  # what is inside
binwalk -e firmware.bin               # extract
unblob firmware.bin                   # a modern alternative with better format coverage
```

The extracted filesystem is the target. Look for:

- **Credentials**: `/etc/passwd` and `/etc/shadow`, `/etc/config/*`, `.conf` files, and private keys in `/etc/ssl` or a vendor directory.
- **Keys in binaries**: `strings` on the web server or the main daemon, and `grep -r` for `BEGIN PRIVATE KEY`, `api_key`, `token`.
- **Startup scripts** (`/etc/init.d/`, `rcS`, systemd units) which reveal what runs, as what user, and with which arguments.
- **The web application**, often a CGI binary or a Lua/PHP application with the same injection bugs as any web application.
- **Debug artifacts**: an enabled telnetd, a `gdb` server, a vendor test binary.
- **The update mechanism**, which is frequently unsigned (see the integrity entry).

Architecture matters for what comes next:

```bash
file ./bin/busybox
readelf -h ./bin/busybox | grep Machine    # MIPS, ARM, x86, and endianness
```

### Physical interfaces

**UART** is the most common and the most productive. A four-pin header near the flash chip or the SoC is usually `VCC`, `GND`, `TX`, `RX`, and the device often prints a boot log and drops you into a shell or a bootloader prompt:

```bash
# identify the pins with a multimeter (GND is continuous with the ground plane, VCC is 3.3V)
# then connect a USB-TTL adapter, remembering that TX goes to RX
picocom -b 115200 /dev/ttyUSB0
# if the output is garbled, try 9600, 38400, 57600, 230400
```

A bootloader prompt (`U-Boot`) is often worth more than the running system: it can boot a modified kernel, read or write flash, and sometimes bypass the login entirely (`init=/bin/sh` in the kernel arguments).

**SPI flash** holds the firmware. Reading it with a programmer (`flashrom` plus a CH341A or similar) gives you the image even when there is no download available, and it is the standard route for a device whose firmware is not published:

```bash
flashrom -p ch341a_spi -r dump.bin
```

**JTAG and SWD** give debugger access to the CPU, which means reading memory, halting execution and bypassing authentication checks. The pinout is usually in the SoC datasheet, and OpenOCD supports most targets.

**eMMC** can often be read by desoldering or by using an in-circuit adapter, which recovers both firmware and data.

### Emulating the firmware

Running the firmware is often easier than reversing it, and it lets you use normal tooling against the device's web interface:

```bash
# user-mode: run a single binary when you know the architecture
qemu-mipsel -L ./rootfs ./rootfs/usr/sbin/httpd

# system-mode: boot the whole image
# FirmAE automates the setup, which is otherwise the fiddly part
./run.sh firmware.bin
```

Network services usually need their configuration adjusted (different interface names, missing hardware) before they will start; persistence pays off here because it turns firmware analysis into ordinary web testing.

### Network, wireless and cloud

- **Network services**: a web console with default credentials, a telnet daemon that was meant for manufacturing, UPnP that exposes more than intended, and MQTT brokers without authentication accepting commands for every device.
- **Wireless**: BLE often has an unauthenticated GATT characteristic that performs an action, or a pairing process that can be replayed. Zigbee and 433/868 MHz devices frequently accept a replayed command with no rolling code.
- **Cloud and mobile**: the device API is usually the easiest target in the whole chain, because it is a normal web application — and the credentials to reach it are extractable from the device or the app. Request signing that uses a key stored in the firmware is not a control, since you have the firmware.

### Detection

- **You cannot secure devices you do not know exist.** Passive network monitoring for the device vendor's traffic patterns, and DHCP or DNS logs, find IoT devices that the asset inventory missed.
- **Segment them.** A separate VLAN with no route to management networks is the single most effective control, because it removes the "compromised camera becomes a foothold" path.
- **Watch the cloud side**, since the device API is where the data is: anomalous request patterns, credentials used from unexpected locations, and a device suddenly sending commands to many others.
- **Egress filtering** stops the common case of a device beaconing to its controller.
- **Firmware version tracking** for the devices that matter, acknowledging that many will never be updated — which is the argument for segmentation rather than patch management.

### Mitigation

- **Change the defaults before deployment**, and disable the services nobody uses (telnet, UPnP, the debug web interface).
- **Do not ship secrets in firmware.** That includes API keys, private keys and per-device passwords; the firmware is public whether or not you publish it.
- **Sign firmware updates and verify signatures on the device**, and refuse downgrades.
- **Encrypt and authenticate device-to-cloud traffic**, and validate the certificate rather than skipping it.
- **Remove or lock the debug interfaces** on production units, and disable the bootloader console in the shipped configuration.
- **Assume the device will not be patched.** Design the network so that a compromised device is contained, and plan for replacement cycles rather than relying on updates.
- **Support long device lifetimes** in the design: a device with a ten-year deployment needs a plan for a ten-year-old component inventory.

<!-- lang:zh -->
### 五个攻击面，以及接触顺序

| 攻击面 | 典型发现 | 成本 |
|---|---|---|
| 固件 | 硬编码凭据、私钥、后门、老旧组件 | 低，且不需要硬件 |
| 网络服务 | 默认口令、Web 管理台的命令注入 | 低 |
| 物理接口 | UART 控制台、JTAG、可读的 flash | 中，需要硬件 |
| 无线 | BLE 配对缺陷、可重放的射频指令 | 中到高 |
| 云与移动端 | 设备 API 通常是整条链上最弱的一环 | 低 |

从固件开始。它就是一个文件，而且它通常包含其他所有面会暴露的东西。

### 固件

```bash
binwalk firmware.bin                  # 里面有什么
binwalk -e firmware.bin               # 提取
unblob firmware.bin                   # 覆盖格式更全的现代替代品
```

解出来的文件系统就是目标。找这些东西：

- **凭据**：`/etc/passwd` 与 `/etc/shadow`、`/etc/config/*`、`.conf` 文件，以及 `/etc/ssl` 或厂商目录里的私钥。
- **二进制里的密钥**：对 Web 服务或主守护进程跑 `strings`，再 `grep -r` 找 `BEGIN PRIVATE KEY`、`api_key`、`token`。
- **启动脚本**（`/etc/init.d/`、`rcS`、systemd unit），它们会告诉你什么在跑、以什么用户跑、带什么参数。
- **Web 应用**，往往是 CGI 二进制或 Lua/PHP 应用，带着和任何 Web 应用一样的注入 bug。
- **调试残留**：开着的 telnetd、`gdb` server、厂商的测试二进制。
- **更新机制**，它常常没有签名（见完整性那篇）。

架构决定了下一步怎么做：

```bash
file ./bin/busybox
readelf -h ./bin/busybox | grep Machine    # MIPS、ARM、x86，以及大小端
```

### 物理接口

**UART** 最常见也最有产出。flash 芯片或 SoC 附近那排四针通常是 `VCC`、`GND`、`TX`、`RX`，而设备往往会打印启动日志，然后直接给你一个 shell 或 bootloader 提示符：

```bash
# 用万用表找针脚（GND 与地平面导通，VCC 是 3.3V）
# 然后接 USB-TTL 适配器，注意 TX 要接 RX
picocom -b 115200 /dev/ttyUSB0
# 输出是乱码就试 9600、38400、57600、230400
```

bootloader 提示符（`U-Boot`）往往比运行中的系统更值钱：它能启动改过的内核、读写 flash，有时还能通过内核参数里的 `init=/bin/sh` 完全绕开登录。

**SPI flash** 里就是固件。用编程器（`flashrom` 加 CH341A 之类）读出来，即使厂商不提供下载你也能拿到镜像 —— 这是"固件不公开"的设备的标准走法：

```bash
flashrom -p ch341a_spi -r dump.bin
```

**JTAG 与 SWD** 给的是对 CPU 的调试访问，也就是读内存、暂停执行、绕过认证检查。针脚定义通常能在 SoC 数据手册里找到，OpenOCD 支持大多数目标。

**eMMC** 通常可以拆焊或用在线转接座读取，固件和数据一起拿到。

### 模拟固件

**跑起来往往比逆它更容易**，而且能让你用常规工具去打设备的 Web 界面：

```bash
# user mode：知道架构之后跑单个二进制
qemu-mipsel -L ./rootfs ./rootfs/usr/sbin/httpd

# system mode：启动整个镜像
# FirmAE 把最麻烦的配置自动化了
./run.sh firmware.bin
```

网络服务通常需要先调整配置（网卡名不同、缺硬件）才能起来；这件事值得花时间，因为它把固件分析变成了普通的 Web 测试。

### 网络、无线与云

- **网络服务**：带默认凭据的 Web 控制台、本来只为产线准备的 telnet、暴露过多的 UPnP，以及没有认证、接受任意设备指令的 MQTT broker。
- **无线**：BLE 上常有一个未认证的 GATT 特征就能触发动作，或者配对流程可以重放。Zigbee 与 433/868 MHz 设备常常接受重放的指令，没有滚动码。
- **云与移动端**：设备 API 通常是整条链上最好打的目标，因为它就是个普通 Web 应用 —— 而访问它所需的凭据可以从设备或 App 里提取出来。**用固件里的密钥做请求签名不是控制**，因为你已经拿到固件了。

### 检测

- **你不知道存在的设备，你保护不了。** 被动监控设备厂商的流量特征，加上 DHCP 或 DNS 日志，能找出资产台账漏掉的 IoT 设备。
- **给它们分段。** 一个与管理网不通的独立 VLAN 是最有效的单项控制，因为它切断了"摄像头被拿下 → 变成落脚点"这条路。
- **盯住云侧**，因为数据在那里：异常的请求模式、从意外位置使用的凭据、以及某台设备突然向很多其他设备发指令。
- **出网过滤**能挡住最常见的那种"设备向控制器心跳"的情况。
- **给要紧的设备跟踪固件版本**，同时承认很多设备永远不会更新 —— 这正是"分段优先于补丁管理"的论据。

### 缓解

- **部署前改掉默认值**，并关掉没人用的服务（telnet、UPnP、调试用 Web 界面）。
- **不要把机密放进固件。** 包括 API key、私钥和每台设备的口令；无论你是否公开发布，固件都是公开的。
- **给固件更新签名，并在设备上校验签名**，且拒绝降级。
- **加密并认证设备到云的流量**，并且真的校验证书，而不是跳过。
- **量产机型上移除或锁住调试接口**，出厂配置里关掉 bootloader 控制台。
- **假设设备不会被打补丁。** 把网络设计成"设备被拿下也能被限制住"，并为更换周期做计划，而不是依赖更新。
- **在设计阶段就考虑设备的长生命周期**：一台要部署十年的设备，需要一份"十年后的组件清单"方案。
