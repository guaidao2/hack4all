---
id: network-stack-basics
title_en: The Network Stack for Beginners
title_zh: 网络栈入门
summary_en: Every attack and every defence eventually becomes packets. This entry builds the stack from the wire up — Ethernet frames, IP packets, the TCP handshake, DNS lookups, TLS — and then shows how to actually look at them, because reading a capture is the skill that makes the rest usable.
summary_zh: 一切攻击与防御最终都会变成数据包。这一篇从最底下的线路开始把网络栈搭起来 —— 以太网帧、IP 包、TCP 握手、DNS 查询、TLS —— 然后教你怎么真的把它们看见，因为「看得懂抓包」才是让前面这些知识可用的那项技能。
tags: [beginner, networking, tcpip, tcp, udp, dns, tls, wireshark]
tools: [Wireshark, tcpdump, dig, curl, ping, traceroute, mtr, ss, arp, openssl]
attck: [T1040, T1046, T1557]
platform: [network]
difficulty: beginner
updated: 2026-10-08
---

<!-- lang:en -->
### Why this comes before everything else

A security professional who cannot read a packet capture is working blind. When something goes wrong, the packet capture is the ground truth — logs lie, applications lie, dashboards summarise, but the bytes on the wire are what actually happened.

There is a second reason. Every vulnerability is ultimately a place where an implementation of a protocol does something the specification did not intend. You cannot see that without knowing what the protocol looks like when everything is normal. This entry is about normal.

### Part 1: the layers

Network communication is built in layers, and each layer wraps the one above it. That is the whole idea — it is called **encapsulation**.

```
Application      HTTP request
Transport        TCP header    [ HTTP request ]
Network          IP header     [ TCP header [ HTTP request ] ]
Link             Eth header    [ IP header [ TCP header [ HTTP request ] ] ]
```

The two models you will see in writing:

| OSI (7 layers) | TCP/IP (4 layers) | What lives there | Unit is called |
|---|---|---|---|
| Application, Presentation, Session | Application | HTTP, DNS, TLS, SSH | data / message |
| Transport | Transport | TCP, UDP, QUIC | segment (TCP), datagram (UDP) |
| Network | Internet | IP, ICMP | packet |
| Data link, Physical | Link | Ethernet, Wi-Fi, ARP | frame |

Beginners get lost in the model because they try to memorise seven layers. Do not. Learn what each layer is *for*:

- **Link** moves a frame between two devices on the same physical network.
- **Network** moves a packet between any two hosts, across many networks.
- **Transport** gets data to the right program on that host, reliably or not.
- **Application** is what the program actually says.

### Part 2: the link layer — the same wire

#### Ethernet frames

Every frame on a wired network starts with two MAC addresses:

```
[ dest MAC 6B ][ src MAC 6B ][ type 2B ][ payload 46-1500B ][ FCS 4B ]
```

- **MAC address**: 48 bits, usually written as `aa:bb:cc:dd:ee:ff`. The first three bytes are the vendor (the OUI), which is why `aa:bb:cc` can tell you the network card's manufacturer.
- **Type**: what is inside — `0x0800` is IPv4, `0x86DD` is IPv6, `0x0806` is ARP.
- **FCS**: a checksum. If it fails, the frame is dropped silently.

MAC addresses are **local**. They matter only on the current link; they change at every hop. IP addresses are the ones that survive across the internet.

#### ARP — how a host finds a MAC

You know the IP you want to talk to, but to build a frame you need a MAC. ARP bridges that gap:

1. The host broadcasts: "Who has 192.168.1.1? Tell 192.168.1.50."
2. The owner replies directly: "192.168.1.1 is at aa:bb:cc:11:22:33."
3. Both cache the answer for a while.

```bash
arp -a          # the ARP cache, on Linux/macOS/Windows
ip neigh        # Linux
```

**ARP has no authentication.** Anyone can answer, including someone who is not the real owner. That is the whole basis of ARP spoofing, and it is why an intrusion detection system watches ARP replies. For now the important part is that ARP is a real protocol you will see constantly in captures — usually far more of it than you expect.

#### Switches and VLANs

A **switch** forwards frames only to the port where the destination MAC lives; a **hub** (obsolete) repeated everything to everyone. A **VLAN** partitions one physical switch into several logical networks, so two devices on the same switch with different VLAN tags cannot talk directly.

### Part 3: the network layer — across the world

#### The IPv4 header

This is worth learning field by field, because you will read it in every capture:

```
 0               1               2               3
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |Version|  IHL  |    DSCP/ECN   |          Total Length           |
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |         Identification        |Flags|     Fragment Offset       |
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |      TTL      |   Protocol    |        Header Checksum          |
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |                       Source IP Address                         |
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |                    Destination IP Address                       |
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |                    Options (if IHL > 5)                         |
```

The fields that matter most in practice:

| Field | Meaning | Why you care |
|---|---|---|
| **TTL** | Hops remaining before the packet is dropped | `traceroute` works by sending packets with TTL 1, 2, 3…; each router that drops one replies with ICMP |
| **Protocol** | What is inside | 6 is TCP, 17 is UDP, 1 is ICMP |
| **Total Length** | Size of the whole packet | A mismatch here is a sign of malformed or crafted traffic |
| **Identification / Fragment Offset** | Used for fragmentation | Fragmentation is rare today and often a sign of something odd |
| **Header Checksum** | Protects the header only | Not the payload |

#### Addresses and subnets

An IPv4 address is 32 bits, written as four decimal numbers. The part that confuses beginners is the **subnet mask**, so here is the idea without the notation:

An address is split into a **network part** and a **host part**. The mask says where the split is. `255.255.255.0` (`/24`) means the first three numbers identify the network, and the last one identifies the host — so `192.168.1.0/24` holds `192.168.1.1` through `192.168.1.254`.

Ranges worth knowing cold:

| Range | What it is |
|---|---|
| `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` | Private — not routable on the internet |
| `127.0.0.0/8` | Loopback — your own machine |
| `169.254.0.0/16` | Link-local, and the cloud metadata address everyone hunts for |
| `224.0.0.0/4` | Multicast |
| `0.0.0.0` | "Any address" when used as a listen address |

**IPv6** exists because IPv4 ran out of addresses. It is 128 bits, written in hexadecimal groups (`2001:0db8::1`), and it changes a few assumptions:

- There is no NAT in the design, because there are enough addresses for everything.
- Every interface usually has several addresses, including a link-local one starting `fe80::`.
- Neighbour Discovery (NDP) replaces ARP.
- The header is simpler and fixed-size, which makes routing faster.

Do not try to memorise IPv6 addresses. Learn to recognise one, and know that `::` collapses a run of zero groups.

#### NAT, and why it shapes everything

Because private addresses are not routable, the device at the edge of your network rewrites them — that is **NAT**. Your host sends from `192.168.1.50`, and the router sends it out as its own public address, remembering which internal host owns which connection.

Consequences you will meet constantly:

- The internet sees your router, not you.
- Incoming connections do not reach you unless a rule or a port mapping exists.
- A server sees many users as one address, which is why rate limiting by IP is tricky.
- Two hosts behind different NATs cannot connect directly, which is why video calls and peer-to-peer systems need STUN, TURN and hole punching.

#### ICMP

ICMP carries control messages, not data. `ping` sends an Echo Request (type 8) and expects an Echo Reply (type 0). `traceroute` exploits TTL expiry. Other types you will see: Destination Unreachable (3), Time Exceeded (11), and Redirect (5).

ICMP is also a covert channel — data in the payload of an echo request looks like a ping to a firewall that only checks the protocol number.

### Part 4: the transport layer — getting to the right program

#### Ports

An IP address gets you to a host. A **port** gets you to a program on that host. Both sides have one; the pair (source port, destination port, source IP, destination IP, protocol) identifies a connection.

| Port | Service |
|---|---|
| 22 | SSH |
| 25 / 587 | SMTP |
| 53 | DNS |
| 80 / 443 | HTTP / HTTPS |
| 139 / 445 | NetBIOS / SMB |
| 1433 / 3306 / 5432 | MSSQL / MySQL / PostgreSQL |
| 3389 | RDP |
| 6379 / 27017 / 9200 | Redis / MongoDB / Elasticsearch |

Client ports are usually **ephemeral** — chosen from a high range (roughly 32768 to 60999 on Linux) and used for one connection.

#### TCP: the three-way handshake

TCP gives you a reliable, ordered stream on top of an unreliable network. It starts by agreeing to talk:

```
Client                          Server
  |  SYN  seq=1000                 |
  |------------------------------->|     "I want to talk, my numbering starts at 1000"
  |  SYN,ACK  seq=5000 ack=1001    |
  |<-------------------------------|     "Agreed, mine starts at 5000, I got your 1000"
  |  ACK  ack=5001                 |
  |------------------------------->|     "Got it, let's go"
```

**Why three and not two?** Because both sides must confirm that the other can both send *and* receive. Two messages would let one direction be unverified, and the server would not know whether its own sequence number arrived. The third message is the client confirming the server's SYN.

**Ending a connection** uses four messages (FIN, ACK, FIN, ACK), because each direction closes independently. It is called a **half-close**: one side can stop sending while still receiving.

**Flags worth recognising:**

| Flag | Meaning |
|---|---|
| SYN | Opening a connection |
| ACK | "I acknowledge up to this sequence number" — set on almost everything after the handshake |
| FIN | "I am finished sending" |
| RST | "Abort this connection" — sent on refused ports, or to tear down anything unexpected |
| PSH | "Deliver this to the application now" |
| URG | Urgent pointer — almost never used |

**Sequence and acknowledgement numbers** are how reliability works. Each byte of data is numbered. The receiver acknowledges the next byte it expects. If a sender does not get an ACK, it retransmits — which is why a capture full of retransmissions means packet loss.

**The window** is flow control: "I can accept this many more bytes." A window of zero means "stop sending", and the sender waits.

**TCP options** appear in the handshake and are worth knowing by name: MSS (maximum segment size), Window Scale (to allow windows above 64 KB), SACK (selective acknowledgement), and Timestamps. They are where stacks differ, and where questions like "why is this session slower than that one" get answered.

#### UDP, and why anyone would use it

UDP has no handshake, no ordering, no retransmission, and no guarantee of delivery. It is a header and a payload. That is exactly what some things want:

- **DNS** — one question, one answer; setting up a connection costs more than the query.
- **Real-time media** — a late packet is worse than a lost one.
- **QUIC** — the transport under HTTP/3, which builds reliability itself on top of UDP, in userspace, to avoid the delays TCP imposes.
- **DHCP, NTP, SNMP** — request and response.

### Part 5: the application layer — what is actually being said

#### DNS

DNS turns names into addresses, and it is the first thing to understand because everything else depends on it.

The lookup, in order:

1. Your machine checks its cache and `/etc/hosts`.
2. It asks the **recursive resolver** (your ISP's, your company's, or `8.8.8.8`).
3. If the resolver does not have it cached, it walks the hierarchy: root servers, then the TLD (`.com`), then the authoritative server for the domain.
4. The answer comes back and is cached for the record's **TTL**.

Record types you will meet:

| Type | Meaning |
|---|---|
| A | Name to IPv4 address |
| AAAA | Name to IPv6 address |
| CNAME | Alias to another name |
| MX | Mail server |
| NS | Authoritative nameserver |
| TXT | Free text — used for SPF, DKIM, and by attackers for data |
| PTR | Reverse lookup, address to name |

```bash
dig example.com A            # ask and see the whole exchange
dig +trace example.com       # walk the hierarchy yourself
dig @1.1.1.1 example.com     # ask a specific resolver
nslookup example.com         # the older tool, on Windows too
```

DNS is plaintext UDP by default, which means it is visible to anyone on the path. That is why DoH and DoT exist, and also why DNS is such a useful channel for attackers and a useful signal for defenders.

#### TLS, in four steps

TLS is what turns HTTP into HTTPS. The handshake, simplified:

1. **Client hello** — here are the versions and ciphers I support, and a random number.
2. **Server hello + certificate** — here is the choice, and here is my certificate proving who I am.
3. **Key exchange** — both sides derive a shared secret (with Diffie-Hellman, the secret is never sent).
4. **Finished** — both confirm, and everything after is encrypted.

Two things a beginner should take from this:

- **The certificate is an identity document**, signed by a certificate authority. Validation failure means "someone may be impersonating this site" — which is why skipping certificate checks in a client is dangerous.
- **After the handshake, everything is encrypted**, headers and all. This is the single most important fact for anyone who wants to inspect traffic: with HTTPS you see the destination IP, the port, the SNI name (usually), and the size and timing of the exchanges — not the content.

#### Other protocols worth recognising by sight

| Protocol | Port | Purpose | Encrypted? |
|---|---|---|---|
| HTTP | 80 | Web | No |
| HTTPS | 443 | Web over TLS | Yes |
| SSH | 22 | Remote shell | Yes |
| SMTP/IMAP/POP3 | 25/143/110 | Mail | Optional |
| SMB | 445 | Windows file sharing | Optional (signing) |
| RDP | 3389 | Remote desktop | Yes, if configured |
| DHCP | 67/68 | Getting an address | No |
| NTP | 123 | Time | No |

### Part 6: actually seeing it

This is the half that makes the theory useful.

#### Wireshark

Capture on the interface you care about, then use display filters — they are how you find anything in a big capture:

| Filter | Shows |
|---|---|
| `ip.addr == 10.0.0.5` | Traffic to or from that host |
| `ip.src == 10.0.0.5 && tcp.port == 443` | Traffic from that host on 443 |
| `dns` | All DNS |
| `http.request` | HTTP requests |
| `tcp.flags.syn == 1 && tcp.flags.ack == 0` | Connection attempts |
| `tcp.analysis.retransmission` | Retransmissions — packet loss |
| `arp` | ARP |
| `icmp` | Ping and friends |

The moves worth learning on day one: **right-click → Follow → TCP Stream** (reassembles a conversation into readable text), **Statistics → Conversations** (who is talking to whom, and how much), and **File → Export Objects → HTTP** (pull files out of a capture).

#### tcpdump

On a server without a GUI, this is what you have:

```bash
tcpdump -i eth0 -nn                       # everything, no name resolution
tcpdump -i eth0 -nn port 80 -A            # HTTP, with payload as text
tcpdump -i eth0 -nn host 10.0.0.5 -w cap.pcap   # write for Wireshark
tcpdump -i eth0 -nn 'tcp[tcpflags] & tcp-syn != 0'   # SYN packets only
```

#### The commands you will use daily

```bash
ping 1.1.1.1                 # is it reachable, and how fast
traceroute 1.1.1.1           # which path, with mtr for a live view
dig example.com              # DNS, in detail
curl -v https://example.com  # the whole HTTP exchange, verbose
curl --resolve example.com:443:10.0.0.5 https://example.com   # pin a name to an IP
ss -tunap                    # listening and established sockets, with processes
netstat -ano                 # the older equivalent, on Windows too
openssl s_client -connect example.com:443 -servername example.com   # inspect a TLS handshake
```

#### Read a handshake yourself

Set a filter of `tcp.port == 443 && ip.addr == <your target>`, then look at the first three packets:

1. `SYN` from your host to the server. Note the sequence number (relative, usually shown as 0).
2. `SYN, ACK` back, with its own sequence number and an acknowledgement of yours plus one.
3. `ACK` from your host.

Then look for the TLS Client Hello in the next packet — the SNI field in it is the hostname you asked for, which is how a network device can tell which site you visited without decrypting anything.

### Part 7: the confusions every beginner has

**"Why can't I see the content of HTTPS?"** Because it is encrypted end to end. You see the IP, the port, the SNI and the sizes. To see more you need to be an endpoint — a proxy with a trusted certificate, or the client itself.

**"Why is there so much ARP?"** Because ARP has no cache coherency mechanism; hosts just re-ask. A busy network produces a lot of it, and a *sudden change* in ARP patterns is more interesting than the volume.

**"Why does one domain resolve to different IPs?"** CDNs and load balancers. The answer depends on where you ask from, which is normal and also why pinning an IP in a test can produce different behaviour than a browser.

**"Why are there retransmissions?"** Packet loss, congestion, or a broken link. A few are normal; a capture full of them is a network problem worth reporting.

**"What is RST?"** A connection reset. You see it when you connect to a closed port, when a firewall rejects rather than drops, and when something tears a session down. Three RSTs in a row from a scanner look very different from one in the middle of a session.

### Part 8: the security angle, briefly

Each layer has its own attack surface. This is orientation, not depth — the individual techniques have their own entries.

| Layer | Weakness in the design | Typical abuse |
|---|---|---|
| Link | ARP and DHCP have no authentication | ARP spoofing, rogue DHCP, MAC flooding |
| Network | Source addresses are not verified | IP spoofing, ICMP tunnels, crafted fragmentation |
| Transport | TCP trusts sequence numbers | SYN floods, session hijacking, port scanning |
| Application | Many protocols are plaintext | Credential capture, DNS poisoning, downgrade attacks |

Two facts worth carrying forward. First, **anything you can see, an attacker on the path can see too** — which is the argument for encrypting everything, including internal traffic. Second, **the same visibility is what detection depends on**, which is why NetFlow, Zeek and packet capture are the backbone of network monitoring.

### Detection and mitigation

- **Encrypt everything, including inside the network.** TLS for application traffic, SSH instead of telnet, DoH or DoT where policy allows. This removes the value of being on the path.
- **Segment the network.** VLANs and firewalls so that a compromised workstation is not adjacent to a database. Flat networks are how one foothold becomes everything.
- **Turn on the link-layer protections** your switches support: Dynamic ARP Inspection, DHCP snooping, port security. They address exactly the unauthenticated protocols above.
- **Filter at the boundary in both directions.** Outbound filtering matters more than most organisations assume: it breaks a lot of C2 and exfiltration, which need to reach somewhere unusual.
- **Baseline the traffic.** What talks to what, on which ports, at what volume. Anomalies in that baseline are how you notice a covert channel or a new service nobody deployed.
- **Keep enough visibility to investigate.** NetFlow or Zeek records at minimum, and full capture on the segments that matter — a capture you did not retain is evidence you do not have.
- **Watch DNS specifically.** It is plaintext, universally allowed, and used for both C2 and exfiltration. Long labels, high query volume, and unusual record types are all cheap signals.

<!-- lang:zh -->
### 为什么这一篇该排在最前面

一个看不懂抓包的安全从业者是在盲人摸象。出事的时候，抓包才是地面真相 —— 日志会说谎、应用会说谎、看板只是概括，但线路上的字节就是实际发生过的事。

还有第二个理由。每一个漏洞，追到根上都是某个协议的某个实现做了规范没打算让它做的事。而如果你不知道协议**正常**时长什么样，你根本看不出哪里不正常。这一篇讲的就是"正常"。

### 第一部分：分层

网络通信是分层搭起来的，每一层把上一层包起来。这就是全部思想 —— 它叫**封装**。

```
应用层        HTTP 请求
传输层        TCP 头    [ HTTP 请求 ]
网络层        IP 头     [ TCP 头 [ HTTP 请求 ] ]
链路层        以太网头   [ IP 头 [ TCP 头 [ HTTP 请求 ] ] ]
```

你会看到的两种模型：

| OSI（7 层） | TCP/IP（4 层） | 这里住着什么 | 单位叫什么 |
|---|---|---|---|
| 应用、表示、会话 | 应用层 | HTTP、DNS、TLS、SSH | 数据 / 报文 |
| 传输层 | 传输层 | TCP、UDP、QUIC | 段（TCP）、数据报（UDP） |
| 网络层 | 网际层 | IP、ICMP | 包 |
| 数据链路、物理 | 链路层 | 以太网、Wi-Fi、ARP | 帧 |

新手常在模型上迷路，因为他们试图背下七层。**别背。** 去理解每一层**是干什么的**：

- **链路层**：在同一张物理网络里，把一帧从一台设备送到另一台。
- **网络层**：把包从任意一台主机送到任意另一台，跨越很多张网络。
- **传输层**：把数据送到那台主机上**正确的程序**，可靠或不可靠都行。
- **应用层**：程序真正在说的话。

### 第二部分：链路层 —— 同一根线上

#### 以太网帧

有线网络上的每一帧都以两个 MAC 地址开头：

```
[ 目的 MAC 6B ][ 源 MAC 6B ][ 类型 2B ][ 载荷 46-1500B ][ FCS 4B ]
```

- **MAC 地址**：48 位，通常写成 `aa:bb:cc:dd:ee:ff`。前三个字节是厂商（叫 OUI），所以看 `aa:bb:cc` 就能知道网卡是哪家产的。
- **类型**：里面装的是什么 —— `0x0800` 是 IPv4，`0x86DD` 是 IPv6，`0x0806` 是 ARP。
- **FCS**：校验和。不通过就静默丢弃。

MAC 地址是**局部**的：只在当前这一段链路上有意义，每一跳都会换。真正能跨越整个互联网保持不变的是 IP 地址。

#### ARP —— 主机怎么找到 MAC

你知道想通信的 IP，但要拼出一帧还需要 MAC。ARP 就是填这道缝的：

1. 主机广播："谁有 192.168.1.1？告诉 192.168.1.50。"
2. 持有者直接回："192.168.1.1 在 aa:bb:cc:11:22:33。"
3. 双方把这个答案缓存一阵子。

```bash
arp -a          # 看 ARP 缓存，Linux/macOS/Windows 都有
ip neigh        # Linux 上的写法
```

**ARP 没有任何认证。** 谁都能回答，包括不是真正持有者的那个人。ARP 欺骗的全部基础就在这儿，也是为什么入侵检测系统会盯着 ARP 回应。此刻对你重要的部分是：ARP 是一个你会不断看到的真实协议 —— 而且数量远比你预期的多。

#### 交换机与 VLAN

**交换机**只把帧转发到目的 MAC 所在的那个口；**集线器**（早淘汰了）则把一切复制给所有人。**VLAN** 把一台物理交换机切成几个逻辑网络，所以同一台交换机上、带不同 VLAN 标签的两台设备无法直接通信。

### 第三部分：网络层 —— 跨越世界

#### IPv4 包头

这个值得逐字段学，因为你每次抓包都会读它：

```
 0               1               2               3
 +-------+-------+-------+-------+-------+-------+-------+-------+
 | 版本  |  IHL  |    DSCP/ECN   |            总长度                |
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |            标识               |标志 |        分片偏移             |
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |     TTL       |    协议       |           头部校验和             |
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |                        源 IP 地址                              |
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |                       目的 IP 地址                             |
 +-------+-------+-------+-------+-------+-------+-------+-------+
 |                    选项（当 IHL > 5 时）                        |
```

实际中最要紧的几个字段：

| 字段 | 含义 | 你为什么关心 |
|---|---|---|
| **TTL** | 还能经过几跳，超了就丢 | `traceroute` 就是发 TTL 为 1、2、3… 的包；丢掉它的路由器会回一个 ICMP |
| **协议** | 里面装什么 | 6 是 TCP，17 是 UDP，1 是 ICMP |
| **总长度** | 整个包多大 | 这里对不上，就是畸形或被构造过的流量的迹象 |
| **标识 / 分片偏移** | 用于分片 | 今天分片很少见，出现往往意味着有古怪 |
| **头部校验和** | 只保护头部 | 不保护载荷 |

#### 地址与子网

IPv4 地址是 32 位，写成四个十进制数。新手最容易被**子网掩码**绕晕，所以先把思想说清楚：

一个地址被切成**网络部分**和**主机部分**，掩码说明切在哪。`255.255.255.0`（即 `/24`）表示前三个数确定网络、最后一个数确定主机 —— 所以 `192.168.1.0/24` 容纳 `192.168.1.1` 到 `192.168.1.254`。

几个该背下来的网段：

| 网段 | 是什么 |
|---|---|
| `10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16` | 私有地址 —— 不能在公网路由 |
| `127.0.0.0/8` | 回环 —— 你自己这台机器 |
| `169.254.0.0/16` | 链路本地，也是所有人都在找的那个云元数据地址 |
| `224.0.0.0/4` | 组播 |
| `0.0.0.0` | 作为监听地址时表示"任意地址" |

**IPv6** 出现是因为 IPv4 地址用完了。它有 128 位，写成十六进制的组（`2001:0db8::1`），并且改变了几个假设：

- 设计里**没有 NAT**，因为地址足够所有东西用。
- 每个接口通常有多个地址，其中包括以 `fe80::` 开头的链路本地地址。
- 邻居发现（NDP）取代了 ARP。
- 包头更简单、长度固定，所以路由更快。

不用去背 IPv6 地址。学会**认出**它是 IPv6，并知道 `::` 是省略了一串零组就够了。

#### NAT，以及它如何塑造了一切

因为私有地址不可路由，网络边缘那台设备会改写它们 —— 这就是 **NAT**。你的主机从 `192.168.1.50` 发出，路由器用自己的公网地址发出去，同时记住哪个内部主机拥有哪条连接。

由此产生一堆你会不断遇到的后果：

- 互联网看到的是你的路由器，不是你。
- 除非有规则或端口映射，外部连接进不来。
- 服务端会把很多用户看成同一个地址，所以按 IP 限流很棘手。
- 处在两个不同 NAT 后面的主机无法直连，所以视频通话和 P2P 需要 STUN、TURN 和打洞。

#### ICMP

ICMP 承载的是控制消息，不是数据。`ping` 发一个 Echo Request（类型 8），期待 Echo Reply（类型 0）。`traceroute` 利用 TTL 过期。你还会看到的类型：目标不可达（3）、超时（11）、重定向（5）。

ICMP 同时也是一条隐蔽通道 —— 把数据放进 echo 请求的载荷里，对一个只检查协议号的防火墙来说，它就长得像 ping。

### 第四部分：传输层 —— 到达正确的程序

#### 端口

IP 地址把你带到一台主机，**端口**把你带到那台主机上的某个程序。两端各有一个端口；五元组（源端口、目的端口、源 IP、目的 IP、协议）唯一标识一条连接。

| 端口 | 服务 |
|---|---|
| 22 | SSH |
| 25 / 587 | SMTP |
| 53 | DNS |
| 80 / 443 | HTTP / HTTPS |
| 139 / 445 | NetBIOS / SMB |
| 1433 / 3306 / 5432 | MSSQL / MySQL / PostgreSQL |
| 3389 | RDP |
| 6379 / 27017 / 9200 | Redis / MongoDB / Elasticsearch |

客户端端口通常是**临时端口** —— 从一个高位区间挑（Linux 上大致 32768 到 60999），用完就释放。

#### TCP：三次握手

TCP 在不可靠的网络之上给你一条可靠、有序的字节流。它从"先谈妥"开始：

```
客户端                          服务端
  |  SYN  seq=1000                 |
  |------------------------------->|     "我想通信，我的编号从 1000 开始"
  |  SYN,ACK  seq=5000 ack=1001    |
  |<-------------------------------|     "同意，我的从 5000 开始，我收到你的 1000 了"
  |  ACK  ack=5001                 |
  |------------------------------->|     "收到，开始吧"
```

**为什么是三次而不是两次？** 因为双方都必须确认对方**既能发也能收**。两次的话，总有一个方向没被验证，服务端也不知道自己的序号有没有到达。第三个消息，就是客户端在确认服务端的 SYN。

**断开连接**用四条消息（FIN、ACK、FIN、ACK），因为两个方向各自独立关闭。这被称为**半关闭**：一方可以停止发送，同时继续接收。

**值得认得的标志位：**

| 标志 | 含义 |
|---|---|
| SYN | 发起连接 |
| ACK | "我已确认到这个序号" —— 握手之后几乎每个包都带 |
| FIN | "我不再发送了" |
| RST | "中止这条连接" —— 端口被拒时、或要拆掉任何意外会话时发出 |
| PSH | "立刻交给应用层" |
| URG | 紧急指针 —— 几乎没人用 |

**序列号与确认号**是可靠性的实现方式。数据的每一个字节都被编号，接收方确认它期待的下一个字节。发送方收不到 ACK 就重传 —— 所以抓包里满是重传就意味着丢包。

**窗口**是流量控制："我还能再收这么多字节。" 窗口为 0 就是"别发了"，发送方会等着。

**TCP 选项**出现在握手里，值得知道名字：MSS（最大段大小）、窗口缩放（让窗口能超过 64 KB）、SACK（选择性确认）、时间戳。各家的协议栈差异就在这里，像"为什么这条会话比那条慢"这类问题的答案也在这里。

#### UDP，以及为什么有人要用它

UDP 没有握手、不保证顺序、不重传、不保证送达。它就是"一个头 + 一段载荷"。而这恰恰是某些场景想要的：

- **DNS** —— 一问一答；建立连接的开销比查询本身还大。
- **实时音视频** —— 迟到的包比丢掉的包更糟。
- **QUIC** —— HTTP/3 底下的传输层，它在 UDP 之上、在用户态自己实现可靠性，为的是绕开 TCP 带来的延迟。
- **DHCP、NTP、SNMP** —— 请求与响应。

### 第五部分：应用层 —— 实际在说什么

#### DNS

DNS 把名字变成地址，而且它是第一个该懂的东西，因为其他一切都依赖它。

一次查询的顺序：

1. 本机先查缓存和 `/etc/hosts`。
2. 去问**递归解析器**（运营商的、公司的、或者 `8.8.8.8`）。
3. 解析器若没有缓存，就沿层级往上走：根服务器、顶级域（`.com`）、再到该域的权威服务器。
4. 答案返回，并按记录的 **TTL** 缓存一段时间。

你会遇到的记录类型：

| 类型 | 含义 |
|---|---|
| A | 名字 → IPv4 地址 |
| AAAA | 名字 → IPv6 地址 |
| CNAME | 指向另一个名字的别名 |
| MX | 邮件服务器 |
| NS | 权威域名服务器 |
| TXT | 自由文本 —— 用于 SPF、DKIM，也被攻击者用来传数据 |
| PTR | 反向查询，地址 → 名字 |

```bash
dig example.com A            # 发一次查询，看完整过程
dig +trace example.com       # 自己走一遍整个层级
dig @1.1.1.1 example.com     # 指定解析器
nslookup example.com         # 更老的命令，Windows 上也有
```

DNS 默认是明文 UDP，也就是说路径上任何人都看得见。这就是 DoH 和 DoT 存在的原因，也是 DNS 对攻击者如此好用、对防守方又如此有信号价值的原因。

#### TLS，四步

TLS 就是把 HTTP 变成 HTTPS 的东西。握手简化后是这样：

1. **Client Hello** —— 我支持这些版本和密码套件，这是我的一个随机数。
2. **Server Hello + 证书** —— 这是我的选择，这是我的证书，用来证明我是谁。
3. **密钥交换** —— 双方各自算出共享密钥（用 Diffie-Hellman 时，密钥本身从不传输）。
4. **Finished** —— 双方确认，之后的一切都加密。

新手该从这里拿走两点：

- **证书是一份身份证**，由证书颁发机构签名。校验失败意味着"可能有人在冒充这个站点" —— 所以客户端跳过证书校验是危险的。
- **握手之后一切都是加密的**，包括请求头。对任何想检查流量的人来说，这是最重要的事实：面对 HTTPS，你能看到目的 IP、端口、SNI 名称（通常）以及交换的大小与时序 —— 但看不到内容。

#### 其他该认得出来的协议

| 协议 | 端口 | 用途 | 加密吗 |
|---|---|---|---|
| HTTP | 80 | 网页 | 否 |
| HTTPS | 443 | 跑在 TLS 上的网页 | 是 |
| SSH | 22 | 远程 shell | 是 |
| SMTP/IMAP/POP3 | 25/143/110 | 邮件 | 可选 |
| SMB | 445 | Windows 文件共享 | 可选（签名） |
| RDP | 3389 | 远程桌面 | 配好了才加密 |
| DHCP | 67/68 | 获取地址 | 否 |
| NTP | 123 | 时间 | 否 |

### 第六部分：真的把它看见

这一半才让前面的理论变得有用。

#### Wireshark

在你要看的网卡上抓包，然后用显示过滤器 —— 这是你在一大堆抓包里找到东西的方式：

| 过滤器 | 显示什么 |
|---|---|
| `ip.addr == 10.0.0.5` | 与该主机相关的流量 |
| `ip.src == 10.0.0.5 && tcp.port == 443` | 该主机在 443 上的出向流量 |
| `dns` | 所有 DNS |
| `http.request` | HTTP 请求 |
| `tcp.flags.syn == 1 && tcp.flags.ack == 0` | 连接尝试 |
| `tcp.analysis.retransmission` | 重传 —— 说明有丢包 |
| `arp` | ARP |
| `icmp` | ping 之类 |

第一天就该学会的三个操作：**右键 → Follow → TCP Stream**（把一段会话重组成可读文本）、**Statistics → Conversations**（谁在和谁说话、量有多大）、**File → Export Objects → HTTP**（把抓包里的文件导出来）。

#### tcpdump

在没有图形界面的服务器上，这就是你手边的东西：

```bash
tcpdump -i eth0 -nn                       # 全部，不做名称解析
tcpdump -i eth0 -nn port 80 -A            # HTTP，载荷按文本显示
tcpdump -i eth0 -nn host 10.0.0.5 -w cap.pcap   # 写文件给 Wireshark 看
tcpdump -i eth0 -nn 'tcp[tcpflags] & tcp-syn != 0'   # 只看 SYN 包
```

#### 每天都会用到的命令

```bash
ping 1.1.1.1                 # 通不通、多快
traceroute 1.1.1.1           # 走哪条路；mtr 能看实时视图
dig example.com              # DNS 细节
curl -v https://example.com  # 整个 HTTP 交换过程，带细节
curl --resolve example.com:443:10.0.0.5 https://example.com   # 把域名钉到指定 IP
ss -tunap                    # 监听与已建立的套接字，含进程
netstat -ano                 # 更老的等价物，Windows 上也有
openssl s_client -connect example.com:443 -servername example.com   # 检查一次 TLS 握手
```

#### 自己读一次握手

设过滤器 `tcp.port == 443 && ip.addr == <目标>`，然后看前三个包：

1. 你主机发出的 `SYN`。注意序列号（显示的是相对值，通常是 0）。
2. 服务端回的 `SYN, ACK`，带着它自己的序列号，以及"你的序号 +1"的确认。
3. 你主机发出的 `ACK`。

接着看下一个包里的 TLS Client Hello —— 其中 SNI 字段就是你要访问的主机名。这就是网络设备在不做解密的情况下也能知道你访问了哪个站点的原因。

### 第七部分：每个新手都会有的困惑

**"为什么看不到 HTTPS 的内容？"** 因为它端到端加密了。你能看到 IP、端口、SNI 和大小。想看得更多，你必须成为其中一端 —— 一个带受信任证书的代理，或者客户端自己。

**"为什么 ARP 这么多？"** 因为 ARP 没有缓存一致性机制，主机只能反复问。繁忙网络里它本来就多，而 ARP 模式的**突然变化**比它的数量更有意思。

**"为什么同一个域名解析出不同的 IP？"** CDN 和负载均衡。答案取决于你从哪里问 —— 这很正常，也正是"测试时把 IP 钉死"会和浏览器行为不一致的原因。

**"为什么会有重传？"** 丢包、拥塞，或者链路有问题。少量是正常的；满屏重传是一个值得上报的网络问题。

**"RST 是什么？"** 连接重置。你会在连到关闭的端口时看到它，在防火墙"拒绝"而不是"丢弃"时看到它，也会在某个东西拆掉会话时看到它。扫描器连着发三个 RST，和会话中间出现一个 RST，看起来完全不是一回事。

### 第八部分：安全视角（点到为止）

每一层都有自己的攻击面。这一节是给你一个方位感，不是深入 —— 各种具体手法另有专篇。

| 层 | 设计上的弱点 | 典型滥用 |
|---|---|---|
| 链路层 | ARP 与 DHCP 没有认证 | ARP 欺骗、恶意 DHCP、MAC 泛洪 |
| 网络层 | 源地址不被验证 | IP 欺骗、ICMP 隧道、构造分片 |
| 传输层 | TCP 信任序列号 | SYN 洪水、会话劫持、端口扫描 |
| 应用层 | 大量协议是明文 | 凭据窃取、DNS 投毒、降级攻击 |

有两件事值得带走。第一，**凡是你能看见的，路径上的攻击者也能看见** —— 这就是"连内网流量都要加密"的论据。第二，**同一份可见性正是检测赖以存在的基础** —— 这也是 NetFlow、Zeek 和抓包成为网络监控骨干的原因。

### 检测与缓解

- **一切加密，包括网络内部。** 应用流量走 TLS，用 SSH 而不是 telnet，策略允许时上 DoH 或 DoT。这直接让"待在路径上"失去价值。
- **给网络分段。** 用 VLAN 和防火墙，让一台被拿下的工作站不和数据库相邻。扁平网络就是"一个落脚点变成全部"的原因。
- **打开交换机支持的链路层防护**：动态 ARP 检测、DHCP snooping、端口安全。它们针对的正是上面那些没有认证的协议。
- **在边界做双向过滤。** 出站过滤比多数组织以为的更重要：它会掐断大量 C2 和外泄，因为那些东西需要连到不寻常的地方去。
- **给流量做基线。** 谁和谁说话、用哪些端口、量多大。基线里的异常，就是你发现隐蔽通道或"没人部署过的服务"的方式。
- **保留足够的可见性用于调查。** 至少留下 NetFlow 或 Zeek 记录，在要紧的网段留完整抓包 —— 没有留下来的抓包，就是你没有的证据。
- **单独盯住 DNS。** 它是明文、到处都被允许，而且同时被用于 C2 和外泄。超长的标签、异常高的查询量、不常见的记录类型，都是便宜的信号。
