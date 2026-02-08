# 握手与密钥更新协议

**状态：** 持续更新文档
**最后更新：** 2026-02-07

## 概述

TunGo 使用 **Noise IK** 握手实现双向认证和密钥协商，随后通过 **X25519 + HKDF-SHA256** 进行周期性密钥更新。传输加密使用基于纪元（epoch）随机数管理的 **ChaCha20-Poly1305** AEAD。

**密码套件：** X25519 / ChaChaPoly / SHA-256
**协议标识：** `"TunGo"`，版本 `0x01`

---

## 1. 握手（Noise IK）

Noise IK 假定发起方（客户端）已知响应方（服务端）的静态公钥。

### 1.1 消息流程

```
Client                                          Server
  │                                               │
  │─── MSG1: (e, es, s, ss) + MAC1 + MAC2 ───────>│
  │                                               │
  │<── COOKIE REPLY (optional, under load) ───────│
  │                                               │
  │─── MSG1 (retry with cookie) ─────────────────>│
  │                                               │
  │<── MSG2: (e, ee, se) ─────────────────────────│
  │                                               │
  ├═══ Transport keys established ════════════════╡
```

### 1.2 MSG1（客户端 -> 服务端）

线路格式：

```
[1B version] [>=80B noise_payload] [16B MAC1] [16B MAC2]
```

- **version:** `0x01`
- **noise_payload:** Noise IK 第一条消息 — 客户端临时公钥（32B，明文）+ 加密的客户端静态公钥（48B）
- **MAC1:** 无状态认证（始终验证）
- **MAC2:** 基于 Cookie 的认证（仅在高负载时验证）

最小长度：113 字节。

### 1.3 MSG2（服务端 -> 客户端）

Noise IK 第二条消息。无 MAC — Noise 完成后双向认证是隐式的。

MSG2 之后，双方派生：
- `c2sKey`（32 字节）— 客户端到服务端的传输密钥
- `s2cKey`（32 字节）— 服务端到客户端的传输密钥
- `sessionId`（32 字节）— 来自 Noise 通道绑定

### 1.4 服务端验证顺序

```
1. CheckVersion()         — reject unknown protocol versions
2. VerifyMAC1()           — stateless, before any DH or allocation
3. VerifyMAC2()           — only under load (LoadMonitor)
4. Noise handshake        — DH computations, peer lookup
5. Peer ACL check         — AllowedPeers / PeerDisabled
```

所有失败均返回统一的 `ErrHandshakeFailed`，以防止信息泄露。

---

## 2. DoS 防护

### 2.1 MAC1（无状态，始终必需）

```
key  = BLAKE2s-256("mac1" || "TunGo" || 0x01 || server_pubkey)
MAC1 = BLAKE2s-128(key, noise_msg1)
```

在任何状态分配或 DH 计算之前进行验证。

### 2.2 MAC2（有状态，高负载时使用）

```
key  = BLAKE2s-256("mac2" || "TunGo" || 0x01 || cookie_value)
MAC2 = BLAKE2s-128(key, noise_msg1 || MAC1)
```

仅当 `LoadMonitor` 检测到压力时进行检查。

### 2.3 Cookie 机制

**Cookie 值**（绑定 IP，按时间分桶）：
```
bucket = unix_seconds / 120
cookie = BLAKE2s-128(server_secret[32], client_ip[16] || bucket[2])
```

当前桶和前一个桶均有效（处理过渡情况）。

**Cookie 回复**（加密，56 字节）：
```
[24B nonce] [16B encrypted_cookie] [16B poly1305_tag]
```

加密方式：
```
key = BLAKE2s-256("cookie" || "TunGo" || 0x01 || server_pubkey || client_ephemeral)
ciphertext = XChaCha20-Poly1305.Seal(key, nonce, cookie, aad=client_ephemeral)
```

---

## 3. 传输加密

### 3.1 AEAD

ChaCha20-Poly1305，带 60 字节 AAD：

```
AAD [60 bytes]:
  [ 0..31] sessionId       (32 bytes)
  [32..47] direction        (16 bytes: "client-to-server" or "server-to-client")
  [48..59] nonce            (12 bytes)
```

SessionId 和 direction 在会话创建时预填充。每个数据包仅更新 nonce。

### 3.2 Nonce 结构（12 字节）

```
[0..7]   counterLow   (uint64, big-endian)
[8..9]   counterHigh  (uint16, big-endian)
[10..11] epoch        (uint16, big-endian)
```

- **Counter:** 80 位单调递增（每个纪元 2^80 条消息）。溢出时返回错误。
- **Epoch:** 会话内不可变，标识密钥更新的代次。

### 3.3 TCP 传输

```
Wire frame: [2B epoch] [ciphertext + 16B tag]
```

- 双纪元：密钥更新期间当前会话和前一会话共存。
- 自动清理：首次成功使用当前纪元解密后，前一会话密钥被清零（依赖 TCP 有序性保证）。
- 无重放保护（TCP 提供有序性）。

### 3.4 UDP 传输

```
Wire frame: [12B nonce] [ciphertext + 16B tag]
```

- Epoch 嵌入在 nonce 的第 10..11 字节中。
- **重放保护：** 每个纪元使用 1024 位滑动窗口位图。
  - 解密前进行初步检查（Check）。
  - 仅在 AEAD 认证成功后提交（Accept）。
  - 防止无效数据包导致窗口污染。
- **Epoch 环：** 固定容量的 FIFO 会话队列。被驱逐的会话将被清零。

---

## 4. 密钥更新

### 4.1 密钥派生

双方执行 X25519 ECDH，然后通过 HKDF-SHA256 派生新的传输密钥：

```
shared  = X25519(local_private, remote_public)
newC2S  = HKDF-SHA256(ikm=shared, salt=currentC2S, info="tungo-rekey-c2s")
newS2C  = HKDF-SHA256(ikm=shared, salt=currentS2C, info="tungo-rekey-s2c")
```

当前密钥用作 HKDF 的 salt，提供前向保密链。

### 4.2 控制平面数据包

```
RekeyInit:  [0xFF] [0x01] [0x02] [32B X25519 public key]   (35 bytes)
RekeyAck:   [0xFF] [0x01] [0x03] [32B X25519 public key]   (35 bytes)
```

### 4.3 密钥更新有限状态机

```
         StartRekey            installPending
Stable ──────────> Rekeying ──────────────> Pending
  ^                                           │
  │          ActivateSendEpoch                │
  └───────────────────────────────────────────┘
  ^                                           │
  │          AbortPendingIfExpired (5s)       │
  └───────────────────────────────────────────┘
```

| 状态 | 描述 |
|-------|-------------|
| **Stable** | 正常运行。一个活跃的发送纪元。 |
| **Rekeying** | 已调用 StartRekey，新密钥已计算，新纪元已安装用于接收。 |
| **Pending** | 等待对端确认（使用新纪元首次成功解密）。 |

### 4.4 密钥更新流程

```
Client                                     Server
  │                                          │
  │── RekeyInit (client X25519 pub) ────────>│
  │                                          │ derive newC2S, newS2C
  │                                          │ install new epoch (recv)
  │<── RekeyAck (server X25519 pub) ─────────│
  │                                          │
  │ derive newC2S, newS2C                    │
  │ install new epoch (recv + send)          │
  │                                          │
  │── first packet with new epoch ──────────>│
  │                                          │ peer confirmed → activate send
  │<── first packet with new epoch ──────────│
  │                                          │
  ├═══ Both sides on new epoch ══════════════╡
```

### 4.5 安全不变量

- 同一时间只允许一个进行中的密钥更新。
- Epoch 单调递增。最大安全 epoch：65000（共 65535）。超过此值，`ErrEpochExhausted` 将强制重新握手。
- 发送 epoch 永不递减。
- 待确认密钥在对端证明持有（通过成功解密）之前，永不覆盖活跃密钥。
- 待确认的密钥更新在 5 秒内未收到对端确认则自动中止。
- 默认密钥更新间隔：120 秒。

---

## 5. 密钥清零

| 材料 | 清零时机 |
|----------|-------------|
| 临时 DH 私钥 | DH 计算完成后立即清零（`defer mem.ZeroBytes`） |
| 共享密钥（密钥更新） | 密钥派生完成后立即清零（`defer mem.ZeroBytes`） |
| 待确认密钥更新密钥（FSM） | 中止或提升为活跃密钥时 |
| 前一会话密钥 | 首次使用当前纪元解密时（TCP）或纪元驱逐时（UDP） |
| Nonce 重放窗口 | 会话拆除时（`SlidingWindow.Zeroize`） |
| AAD 缓冲区 | 会话拆除时（`DefaultUdpSession.Zeroize`） |

**限制：** Go GC 可能在清零前复制堆对象。`mem.ZeroBytes` 是对抗内存取证的尽力而为的防御措施，已通过编译器输出分析验证不会被优化掉（Go 1.25.7，所有目标平台）。

---

## 6. 常量

| 常量 | 值 | 用途 |
|----------|-------|---------
| Protocol version | `0x01` | 线路格式版本控制 |
| MAC1 / MAC2 size | 16 字节 | BLAKE2s-128 输出 |
| Cookie bucket | 120 秒 | 绑定 IP 的 Cookie 有效窗口 |
| Cookie reply size | 56 字节 | nonce (24) + encrypted cookie (16) + tag (16) |
| AAD length | 60 字节 | sessionId (32) + direction (16) + nonce (12) |
| Nonce counter | 80 位 | 溢出前每个纪元的消息数 |
| Replay window | 1024 位 | UDP 乱序容错 |
| Epoch capacity | uint16 | 65535 个值，安全阈值 65000 |
| Rekey interval | 120 秒 | 默认周期性密钥更新触发 |
| Pending timeout | 5 秒 | 自动中止未确认的密钥更新 |
