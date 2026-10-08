---
id: mobile-app-testing
title_en: Mobile Application Testing
title_zh: 移动应用测试
summary_en: The app is rarely the target. It is a client that talks to an API, and the API is usually less protected than the web one — no WAF, older version, simpler authorization. Break the pinning, read the traffic, and the interesting bugs are on the server.
summary_zh: 应用本身很少是目标。它只是个跟 API 说话的客户端，而那个 API 通常比 Web 端保护更弱 —— 没有 WAF、版本更旧、鉴权更简单。把证书钉扎破了，读到流量，真正的洞在服务端。
tags: [mobile, android, ios, frida, ssl-pinning, api, bugbounty]
tools: [Frida, objection, jadx, apktool, MobSF, Burp Suite, mitmproxy]
attck: [T1409, T1417]
platform: [android, ios]
difficulty: intermediate
updated: 2026-10-08
---

<!-- lang:en -->
### Where the findings actually are

Mobile work is split into two halves, and the split decides where you spend your time:

- **The client** — local storage, exported components, WebView bridges, deep links, hardcoded secrets. Real bugs, but usually medium severity.
- **The API behind it** — the same endpoints the app calls, frequently an older build of the web API with different authorization. This is where the account takeover and the IDORs are.

Most mobile engagements go: defeat pinning → capture the API traffic → find the client is a red herring and the server is the target.

### Step 1 — Get the artifact and read it without running it

```bash
# Android: decode resources, then decompile to Java
apktool d app.apk -o app-decoded
jadx -d app-java app.apk

# the whole first pass at once
docker run -it --rm -p 8000:8000 opensecurity/mobile-security-framework-mobsf
```

iOS needs a decrypted binary (App Store binaries are encrypted); from a jailbroken device use `frida-ios-dump`, then `class-dump` and a disassembler.

Then grep — the first pass finds something embarrassingly often:

```bash
grep -rInE 'api[_-]?key|secret|token|password|AKIA[0-9A-Z]{16}' app-decoded/
# Firebase, Google Maps, AWS, Stripe, and every in-app purchase key live in here
```

`AndroidManifest.xml` deserves its own read:

| Attribute | What it tells you |
|---|---|
| `android:exported="true"` | Reachable by any other app on the device |
| `android:debuggable="true"` | Not shipped that way, but common in internal builds |
| `android:allowBackup="true"` | `adb backup` can extract app data |
| `android:usesCleartextTraffic="true"` | Plaintext HTTP is permitted somewhere |
| `<intent-filter>` with a custom scheme | Deep-link entry point |
| `network_security_config` | Where pinning is declared, if it is |

### Step 2 — Instrument the runtime

Frida is the workhorse on both platforms. On Android, either root the device (or emulator) and run `frida-server`, or patch the APK to embed the gadget — the second option gets past environments where root is not allowed.

```bash
# Android
adb push frida-server /data/local/tmp/ && adb shell "chmod 755 /data/local/tmp/frida-server"
adb shell "/data/local/tmp/frida-server &"
frida-ps -U                       # list processes on the USB device

# objection wraps the common tasks
objection -g com.example.app explore
```

Emulators are convenient but detectable: a rooted emulator without Play Integrity is refused by many apps. Plan for a physical device when the app checks.

### Step 3 — Certificate pinning is the first wall

Pinning comes in several shapes, and the bypass depends on which one you face:

| Pinning style | What it looks like | Where to hook |
|---|---|---|
| Network security config | `pin-set` in XML | Patch the config, or hook the platform trust check |
| OkHttp `CertificatePinner` | Java/Kotlin, app code | `okhttp3.CertificatePinner.check` |
| Custom `X509TrustManager` | Java, app code | `checkServerTrusted` |
| Native (OpenSSL/BoringSSL) | `.so` library | `SSL_CTX_set_verify`, `SSL_get_verify_result` |
| Flutter / React Native | framework layer | Framework-specific scripts |

```bash
# objection does most of the common cases
objection -g com.example.app explore -s "android sslpinning disable"

# Android 7+: user CAs are ignored by apps by default, so either
#   - install a Magisk module that moves your CA into the system store, or
#   - rebuild the APK with a network-security-config that trusts user CAs
```

For native pinning, find the verification function in the `.so` and hook it so it returns success — or patch the binary. On iOS, SSL Kill Switch or an objection script handles the common cases.

### Step 4 — Detection you will run into

Assume the app checks for instrumentation, root, and tampering, and that a failure is silent or a hard exit:

```javascript
// a Frida bypass for the usual Java checks, conceptually
Java.perform(function () {
  var RootBeer = Java.use('com.scottyab.rootbeer.RootBeer');
  RootBeer.isRooted.implementation = function () { return false; };
});
```

Common checks: `su` binaries and `test-keys`, Frida's default port and thread names, debugger attachment, emulator properties, Play Integrity / App Attest responses, and file hashes. Bypass each by hooking the check rather than the environment where possible, and expect the app to make a server-side call with the verdict.

### Step 5 — Client-side bugs still worth reporting

| Class | Android | iOS |
|---|---|---|
| Insecure local storage | Cleartext `SharedPreferences`, data on external storage | `NSUserDefaults` for tokens, world-readable files |
| Exported components | Activity/Service/Provider reachable by any app | URL schemes, app extensions |
| WebView issues | `addJavascriptInterface` with a privileged object, unvalidated `loadUrl`, `file://` access | `WKWebView` message handlers, `loadFileURL` |
| Deep links | Custom scheme hijacking, parameter injection into a WebView | Universal links misconfiguration, scheme collision |
| Log leakage | `Log.d` printing tokens, crash reports | `NSLog` in release |
| Backup | `allowBackup=true` | iTunes/iCloud backup contains app data |
| Clipboard | Tokens copied and left there | same |

A genuinely dangerous combination is common: an exported activity that takes a URL parameter and passes it to a WebView with a JavaScript bridge attached. That is remote code execution inside the app's trust context, reachable by any other app on the device.

### Step 6 — Then attack the API

Once you can read the traffic (or you simply extract the API base URL and auth flow from the decompiled code), the mobile API is just an API:

- It often runs an older version than the web API — try `/v1/`, `/mobile/`, `/legacy/`.
- It frequently has weaker authorization because "only our app calls it".
- Certificate pinning protects the channel, not the endpoint: replay the requests from Burp with a valid token.
- Endpoints the UI never shows are still there; pull them from the code and from the app's own request logs.

Reuse the web techniques here — IDOR, mass assignment, broken function-level authorization — and check whether the token in the app is scoped the same way the web session is.

### Detection and mitigation

From the defender's side:

- **Root and instrumentation detection are speed bumps**, not controls. Rely on server-side authorization and integrity attestation for anything that matters.
- **Never store secrets in the app.** Anything shipped in the binary or in local storage is readable; treat every client-held value as public.
- **Do not treat the mobile client as trusted**: server-side authorization for every object, and rate limits per user rather than per device.
- **Set `android:exported="false"`** unless a component must be reachable, and protect exported components with signature-level permissions.
- **Disable backup and cleartext traffic** unless there is a documented reason.
- **Monitor the API** for the same patterns as the web: enumeration, version downgrade, and requests that no released client build would make.

<!-- lang:zh -->
### 真正的发现在哪里

移动端工作分成两半，这个划分决定你把时间花在哪：

- **客户端** —— 本地存储、导出组件、WebView 桥、深层链接、硬编码密钥。是真漏洞，但通常是中危。
- **它背后的 API** —— 应用调用的那些接口，往往是 Web API 的旧版本、鉴权还不一样。账号接管和 IDOR 都在这里。

多数移动端项目的路径是：破掉钉扎 → 抓到 API 流量 → 发现客户端是障眼法，服务端才是目标。

### 第一步 —— 拿到包，先静态读一遍

```bash
# Android：先解资源，再反编译成 Java
apktool d app.apk -o app-decoded
jadx -d app-java app.apk

# 一把梭做完第一轮
docker run -it --rm -p 8000:8000 opensecurity/mobile-security-framework-mobsf
```

iOS 需要已解密的二进制（App Store 的包是加密的）；在越狱设备上用 `frida-ios-dump` 脱壳，然后 `class-dump` 加反汇编器。

然后 grep —— 第一轮就能翻出东西的概率高得让人尴尬：

```bash
grep -rInE 'api[_-]?key|secret|token|password|AKIA[0-9A-Z]{16}' app-decoded/
# Firebase、Google Maps、AWS、Stripe，以及所有内购密钥都在里面
```

`AndroidManifest.xml` 值得单独看：

| 属性 | 它告诉你什么 |
|---|---|
| `android:exported="true"` | 设备上任何其他应用都能访问 |
| `android:debuggable="true"` | 正式包不该这样，但内部构建里很常见 |
| `android:allowBackup="true"` | `adb backup` 能把应用数据导出来 |
| `android:usesCleartextTraffic="true"` | 某处允许明文 HTTP |
| 带自定义 scheme 的 `<intent-filter>` | 深层链接入口 |
| `network_security_config` | 钉扎（如果有）声明在哪 |

### 第二步 —— 把运行时插上桩

两个平台上 Frida 都是主力。Android 上要么 root 设备（或模拟器）跑 `frida-server`，要么给 APK 打补丁内置 gadget —— 后者能应对不允许 root 的环境。

```bash
# Android
adb push frida-server /data/local/tmp/ && adb shell "chmod 755 /data/local/tmp/frida-server"
adb shell "/data/local/tmp/frida-server &"
frida-ps -U                       # 列出 USB 设备上的进程

# objection 把常用操作包好了
objection -g com.example.app explore
```

模拟器方便但可被检测：没有 Play Integrity 的 root 模拟器会被很多应用直接拒绝。应用有校验时，就准备真机。

### 第三步 —— 证书钉扎是第一道墙

钉扎有好几种形态，绕过方式取决于你面对哪一种：

| 钉扎方式 | 长什么样 | hook 哪里 |
|---|---|---|
| network security config | XML 里的 `pin-set` | 改配置，或 hook 平台的信任校验 |
| OkHttp `CertificatePinner` | Java/Kotlin 应用代码 | `okhttp3.CertificatePinner.check` |
| 自定义 `X509TrustManager` | Java 应用代码 | `checkServerTrusted` |
| 原生（OpenSSL/BoringSSL） | `.so` 库 | `SSL_CTX_set_verify`、`SSL_get_verify_result` |
| Flutter / React Native | 框架层 | 框架专用脚本 |

```bash
# objection 覆盖了大多数常见情况
objection -g com.example.app explore -s "android sslpinning disable"

# Android 7+：应用默认不再信任用户 CA，所以要么
#   - 用 Magisk 模块把你的 CA 放进系统存储，要么
#   - 重打包 APK，改用信任用户 CA 的 network-security-config
```

遇到原生钉扎，就在 `.so` 里定位校验函数并 hook 成返回成功，或者直接改二进制。iOS 上 SSL Kill Switch 或 objection 脚本能处理常见情况。

### 第四步 —— 你会撞上的检测

假设应用会检查调试注入、root 和篡改，而且失败时要么静默要么直接退出：

```javascript
// 绕过常见 Java 检测的 Frida 脚本，示意
Java.perform(function () {
  var RootBeer = Java.use('com.scottyab.rootbeer.RootBeer');
  RootBeer.isRooted.implementation = function () { return false; };
});
```

常见的检测点：`su` 二进制与 `test-keys`、Frida 的默认端口与线程名、调试器附加、模拟器属性、Play Integrity / App Attest 的返回、以及文件哈希。尽量 hook 检测函数本身，而不是改环境，并且预期应用会把判定结果发给服务端。

### 第五步 —— 客户端那些仍然值得报的洞

| 类别 | Android | iOS |
|---|---|---|
| 不安全本地存储 | 明文 `SharedPreferences`、写到外部存储 | 用 `NSUserDefaults` 存令牌、全局可读文件 |
| 导出组件 | 任何应用都能访问的 Activity/Service/Provider | URL scheme、应用扩展 |
| WebView 问题 | `addJavascriptInterface` 暴露特权对象、未校验的 `loadUrl`、`file://` 访问 | `WKWebView` message handler、`loadFileURL` |
| 深层链接 | 自定义 scheme 劫持、参数注入进 WebView | Universal Link 配置错误、scheme 冲突 |
| 日志泄漏 | `Log.d` 打印令牌、崩溃上报 | 正式包里的 `NSLog` |
| 备份 | `allowBackup=true` | iTunes/iCloud 备份包含应用数据 |
| 剪贴板 | 令牌复制后留在剪贴板 | 同上 |

有一种真正危险的组合很常见：一个导出的 Activity 接收 URL 参数，把它交给挂了 JavaScript 桥的 WebView。那就是在应用信任上下文里的远程代码执行，而且设备上任何其他应用都能触发。

### 第六步 —— 然后去打 API

一旦你能读到流量（或者干脆从反编译代码里把 API 基址与认证流程抠出来），移动端 API 就只是个 API：

- 它常常跑着比 Web 端更旧的版本 —— 试试 `/v1/`、`/mobile/`、`/legacy/`。
- 鉴权往往更弱，因为"只有我们的 App 会调它"。
- 证书钉扎保护的是通道，不是接口：用有效令牌从 Burp 重放这些请求。
- 界面上从不展示的接口依然存在；从代码里和应用自己的请求日志里把它们挖出来。

这里直接复用 Web 那套：IDOR、批量赋值、功能级授权缺陷 —— 并检查应用里的令牌与 Web 会话的权限范围是否一致。

### 检测与缓解

从防守方角度：

- **root 与注入检测是减速带，不是控制措施。** 关键逻辑要靠服务端鉴权与完整性证明。
- **绝不要把密钥放在应用里。** 任何随二进制或本地存储发出去的东西都可被读取；把客户端持有的每个值都当作公开的。
- **不要把移动端当成可信客户端**：每个对象都在服务端做鉴权，限流按用户而不是按设备。
- **除非组件必须可达，否则设 `android:exported="false"`**，并用签名级权限保护确实需要导出的组件。
- **没有书面理由就关掉备份与明文流量。**
- **像监控 Web 一样监控 API**：枚举、版本回退，以及任何已发布客户端版本都不会发出的请求。
