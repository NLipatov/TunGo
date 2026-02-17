# Handshake and Rekeying Protocol

**Status:** Living document
**Last updated:** 2026-02-17

## Overview

TunGo uses a **Noise IK** handshake for mutual authentication and key agreement, followed by periodic **X25519 + HKDF-SHA256** rekeying. Transport encryption uses **ChaCha20-Poly1305** AEAD with epoch-based nonce management.

**Cipher suite:** X25519 / ChaChaPoly / SHA-256
**Protocol ID:** `"TunGo"`, version `0x01`

---

## 1. Handshake (Noise IK)

Noise IK assumes the initiator (client) already knows the responder's (server) static public key.

### 1.1 Message Flow

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

### 1.2 MSG1 (Client -> Server)

Wire format:

```
[1B version] [>=80B noise_payload] [16B MAC1] [16B MAC2]
```

- **version:** `0x01`
- **noise_payload:** Noise IK first message — client ephemeral public (32B, plaintext) + encrypted client static (48B)
- **MAC1:** Stateless authentication (always verified)
- **MAC2:** Cookie-based authentication (verified only under load)

Minimum size: 113 bytes.

### 1.3 MSG2 (Server -> Client)

Noise IK second message. No MACs — bidirectional authentication is implicit after Noise completes.

After MSG2, both sides derive:
- `c2sKey` (32 bytes) — client-to-server transport key
- `s2cKey` (32 bytes) — server-to-client transport key
- `sessionId` (32 bytes) — from Noise channel binding

### 1.4 Server Verification Order

```
1. CheckVersion()         — reject unknown protocol versions
2. VerifyMAC1()           — stateless, before any DH or allocation
3. VerifyMAC2()           — only under load (LoadMonitor)
4. Noise handshake        — DH computations, peer lookup
5. Peer ACL check         — AllowedPeers / PeerDisabled
```

All failures return a uniform `ErrHandshakeFailed` to prevent information leakage.

---

## 2. DoS Protection

### 2.1 MAC1 (Stateless, Always Required)

```
key  = BLAKE2s-256("mac1" || "TunGo" || 0x01 || server_pubkey)
MAC1 = BLAKE2s-128(key, noise_msg1)
```

Verified before any state allocation or DH computation.

### 2.2 MAC2 (Stateful, Under Load)

```
key  = BLAKE2s-256("mac2" || "TunGo" || 0x01 || cookie_value)
MAC2 = BLAKE2s-128(key, noise_msg1 || MAC1)
```

Checked only when `LoadMonitor` detects pressure.

### 2.3 Cookie Mechanism

**Cookie value** (IP-bound, time-bucketed):
```
bucket = unix_seconds / 120
cookie = BLAKE2s-128(server_secret[32], client_ip[16] || bucket[2])
```

Valid for current and previous bucket (handles transitions).

**Cookie reply** (encrypted, 56 bytes):
```
[24B nonce] [16B encrypted_cookie] [16B poly1305_tag]
```

Encryption:
```
key = BLAKE2s-256("cookie" || "TunGo" || 0x01 || server_pubkey || client_ephemeral)
ciphertext = XChaCha20-Poly1305.Seal(key, nonce, cookie, aad=client_ephemeral)
```

---

## 3. Transport Encryption

### 3.1 AEAD

ChaCha20-Poly1305 with 60-byte AAD:

```
AAD [60 bytes]:
  [ 0..31] sessionId       (32 bytes)
  [32..47] direction        (16 bytes: "client-to-server" or "server-to-client")
  [48..59] nonce            (12 bytes)
```

SessionId and direction are pre-filled at session creation. Only the nonce is updated per packet.

### 3.2 Nonce Structure (12 bytes)

```
[0..7]   counterLow   (uint64, big-endian)
[8..9]   counterHigh  (uint16, big-endian)
[10..11] epoch        (uint16, big-endian)
```

- **Counter:** 80-bit monotonic (2^80 messages per epoch). Overflow returns error.
- **Epoch:** Immutable per session, identifies rekeying generation.

### 3.3 TCP Transport

```
Wire frame: [2B epoch] [ciphertext + 16B tag]
```

- Dual-epoch: current + previous session coexist during rekey.
- Auto-cleanup: previous session zeroed on first current-epoch decryption (TCP ordering guarantee).
- No replay protection (TCP provides ordering).

### 3.4 UDP Transport

```
Wire frame: [8B route-id] [12B nonce] [ciphertext + 16B tag]
```

- Route-id is derived from `sessionId` (first 8 bytes, big-endian) and enables O(1) session lookup.
- Epoch embedded in nonce bytes 10..11.
- **Replay protection:** 1024-bit sliding window bitmap per epoch.
  - Tentative check before decryption (Check).
  - Committed only after AEAD authentication succeeds (Accept).
  - Prevents window poisoning by invalid packets.
- **Epoch ring:** Fixed-capacity FIFO of sessions. Evicted sessions are zeroed.

---

## 4. Rekeying

### 4.1 Key Derivation

Both sides perform X25519 ECDH, then derive new transport keys via HKDF-SHA256:

```
shared  = X25519(local_private, remote_public)
newC2S  = HKDF-SHA256(ikm=shared, salt=currentC2S, info="tungo-rekey-c2s")
newS2C  = HKDF-SHA256(ikm=shared, salt=currentS2C, info="tungo-rekey-s2c")
```

Current keys serve as HKDF salt, providing forward secrecy chaining.

### 4.2 Control Plane Packets

```
RekeyInit:  [0xFF] [0x01] [0x02] [32B X25519 public key]   (35 bytes)
RekeyAck:   [0xFF] [0x01] [0x03] [32B X25519 public key]   (35 bytes)
```

### 4.3 Rekey FSM

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

| State | Description |
|-------|-------------|
| **Stable** | Normal operation. One active send epoch. |
| **Rekeying** | StartRekey called, new keys computed, new epoch installed for receive. |
| **Pending** | Awaiting peer confirmation (first successful decryption with new epoch). |

### 4.4 Rekey Flow

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

### 4.5 Safety Invariants

- Only one in-flight rekey at a time.
- Epochs monotonically increase. Max safe epoch: 65000 (of 65535). Beyond this, `ErrEpochExhausted` forces re-handshake.
- Send epoch never decreases.
- Pending keys never overwrite active keys until peer proves possession (via successful decryption).
- Pending rekey auto-aborts after 5 seconds if no peer confirmation.
- Default rekey interval: 120 seconds.

---

## 5. Key Zeroization

| Material | When Zeroed |
|----------|-------------|
| Ephemeral DH private keys | Immediately after DH computation (`defer mem.ZeroBytes`) |
| Shared secrets (rekey) | Immediately after key derivation (`defer mem.ZeroBytes`) |
| Pending rekey keys (FSM) | On abort or promotion to active |
| Previous session keys | On first current-epoch decryption (TCP) or epoch eviction (UDP) |
| Nonce replay window | On session teardown (`SlidingWindow.Zeroize`) |
| AAD buffers | On session teardown (`DefaultUdpSession.Zeroize`) |

**Limitation:** Go GC may copy heap objects before zeroing. `mem.ZeroBytes` is best-effort defense against memory forensics, verified by compiler output analysis to not be optimized away (Go 1.26.x, all target platforms).

---

## 6. Constants

| Constant | Value | Purpose |
|----------|-------|---------|
| Protocol version | `0x01` | Wire format versioning |
| MAC1 / MAC2 size | 16 bytes | BLAKE2s-128 output |
| Cookie bucket | 120 seconds | IP-bound cookie validity window |
| Cookie reply size | 56 bytes | nonce (24) + encrypted cookie (16) + tag (16) |
| AAD length | 60 bytes | sessionId (32) + direction (16) + nonce (12) |
| UDP route-id | 8 bytes | session identifier prefix for O(1) peer lookup |
| Nonce counter | 80 bits | Messages per epoch before overflow |
| Replay window | 1024 bits | UDP out-of-order tolerance |
| Epoch capacity | uint16 | 65535 values, safe threshold 65000 |
| Rekey interval | 120 seconds | Default periodic rekey trigger |
| Pending timeout | 5 seconds | Auto-abort unconfirmed rekey |
