# ハンドシェイクおよびリキー・プロトコル

**ステータス:** 継続的に更新される文書
**最終更新日:** 2026-02-07

## 概要

TunGoは相互認証と鍵合意のために**Noise IK**ハンドシェイクを使用し、その後定期的な**X25519 + HKDF-SHA256**リキーを行います。トランスポート暗号化にはエポックベースのナンス管理を伴う**ChaCha20-Poly1305** AEADを使用します。

**暗号スイート:** X25519 / ChaChaPoly / SHA-256
**プロトコルID:** `"TunGo"`、バージョン `0x01`

---

## 1. ハンドシェイク (Noise IK)

Noise IKは、イニシエータ（クライアント）がレスポンダ（サーバー）の静的公開鍵を既に知っていることを前提とします。

### 1.1 メッセージフロー

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

### 1.2 MSG1 (クライアント -> サーバー)

ワイヤーフォーマット:

```
[1B version] [>=80B noise_payload] [16B MAC1] [16B MAC2]
```

- **version:** `0x01`
- **noise_payload:** Noise IK第1メッセージ — クライアントの一時公開鍵（32B、平文）+ 暗号化されたクライアント静的鍵（48B）
- **MAC1:** ステートレス認証（常に検証される）
- **MAC2:** Cookieベースの認証（高負荷時のみ検証される）

最小サイズ: 113バイト。

### 1.3 MSG2 (サーバー -> クライアント)

Noise IK第2メッセージ。MACなし — Noise完了後、双方向認証は暗黙的です。

MSG2の後、双方が以下を導出します:
- `c2sKey`（32バイト）— クライアントからサーバーへのトランスポート鍵
- `s2cKey`（32バイト）— サーバーからクライアントへのトランスポート鍵
- `sessionId`（32バイト）— Noiseチャネルバインディングから

### 1.4 サーバー検証順序

```
1. CheckVersion()         — reject unknown protocol versions
2. VerifyMAC1()           — stateless, before any DH or allocation
3. VerifyMAC2()           — only under load (LoadMonitor)
4. Noise handshake        — DH computations, peer lookup
5. Peer ACL check         — AllowedPeers / PeerDisabled
```

すべての失敗は情報漏洩を防ぐために統一された`ErrHandshakeFailed`を返します。

---

## 2. DoS防御

### 2.1 MAC1（ステートレス、常に必須）

```
key  = BLAKE2s-256("mac1" || "TunGo" || 0x01 || server_pubkey)
MAC1 = BLAKE2s-128(key, noise_msg1)
```

状態の割り当てやDH計算の前に検証されます。

### 2.2 MAC2（ステートフル、高負荷時）

```
key  = BLAKE2s-256("mac2" || "TunGo" || 0x01 || cookie_value)
MAC2 = BLAKE2s-128(key, noise_msg1 || MAC1)
```

`LoadMonitor`が負荷圧力を検出した場合のみチェックされます。

### 2.3 Cookieメカニズム

**Cookie値**（IPバインド、時間バケット方式）:
```
bucket = unix_seconds / 120
cookie = BLAKE2s-128(server_secret[32], client_ip[16] || bucket[2])
```

現在および前のバケットに対して有効（遷移を処理）。

**Cookie応答**（暗号化済み、56バイト）:
```
[24B nonce] [16B encrypted_cookie] [16B poly1305_tag]
```

暗号化:
```
key = BLAKE2s-256("cookie" || "TunGo" || 0x01 || server_pubkey || client_ephemeral)
ciphertext = XChaCha20-Poly1305.Seal(key, nonce, cookie, aad=client_ephemeral)
```

---

## 3. トランスポート暗号化

### 3.1 AEAD

60バイトAADを伴うChaCha20-Poly1305:

```
AAD [60 bytes]:
  [ 0..31] sessionId       (32 bytes)
  [32..47] direction        (16 bytes: "client-to-server" or "server-to-client")
  [48..59] nonce            (12 bytes)
```

SessionIdと方向はセッション作成時に事前設定されます。パケットごとに更新されるのはナンスのみです。

### 3.2 ナンス構造（12バイト）

```
[0..7]   counterLow   (uint64, big-endian)
[8..9]   counterHigh  (uint16, big-endian)
[10..11] epoch        (uint16, big-endian)
```

- **Counter:** 80ビット単調増加（エポックあたり2^80メッセージ）。オーバーフロー時はエラーを返します。
- **Epoch:** セッションごとに不変、リキーの世代を識別します。

### 3.3 TCPトランスポート

```
Wire frame: [2B epoch] [ciphertext + 16B tag]
```

- デュアルエポック: リキー中は現在のセッションと前のセッションが共存します。
- 自動クリーンアップ: 現在のエポックでの最初の復号時に前のセッションがゼロ化されます（TCPの順序保証）。
- リプレイ防御なし（TCPが順序を保証するため）。

### 3.4 UDPトランスポート

```
Wire frame: [12B nonce] [ciphertext + 16B tag]
```

- エポックはナンスのバイト10..11に埋め込まれます。
- **リプレイ防御:** エポックごとの1024ビットスライディングウィンドウビットマップ。
  - 復号前の仮チェック（Check）。
  - AEAD認証成功後にのみ確定（Accept）。
  - 無効なパケットによるウィンドウポイズニングを防止します。
- **エポックリング:** 固定容量のセッションFIFO。排出されたセッションはゼロ化されます。

---

## 4. リキー

### 4.1 鍵導出

双方がX25519 ECDHを実行し、HKDF-SHA256を介して新しいトランスポート鍵を導出します:

```
shared  = X25519(local_private, remote_public)
newC2S  = HKDF-SHA256(ikm=shared, salt=currentC2S, info="tungo-rekey-c2s")
newS2C  = HKDF-SHA256(ikm=shared, salt=currentS2C, info="tungo-rekey-s2c")
```

現在の鍵がHKDFソルトとして機能し、前方秘匿性の連鎖を提供します。

### 4.2 制御プレーンパケット

```
RekeyInit:  [0xFF] [0x01] [0x02] [32B X25519 public key]   (35 bytes)
RekeyAck:   [0xFF] [0x01] [0x03] [32B X25519 public key]   (35 bytes)
```

### 4.3 リキーFSM

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

| 状態 | 説明 |
|------|------|
| **Stable** | 通常動作。1つのアクティブな送信エポック。 |
| **Rekeying** | StartRekeyが呼び出され、新しい鍵が計算され、受信用の新しいエポックがインストールされた状態。 |
| **Pending** | ピアの確認待ち（新しいエポックでの最初の復号成功）。 |

### 4.4 リキーフロー

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

### 4.5 安全性の不変条件

- 同時に実行されるリキーは1つのみ。
- エポックは単調増加します。最大安全エポック: 65000（65535中）。これを超えると`ErrEpochExhausted`が再ハンドシェイクを強制します。
- 送信エポックは決して減少しません。
- 保留中の鍵は、ピアが所有を証明するまで（復号の成功により）アクティブな鍵を上書きしません。
- ピアの確認がない場合、保留中のリキーは5秒後に自動的に中止されます。
- デフォルトのリキー間隔: 120秒。

---

## 5. 鍵のゼロ化

| 対象 | ゼロ化のタイミング |
|------|-------------------|
| 一時DH秘密鍵 | DH計算直後（`defer mem.ZeroBytes`） |
| 共有シークレット（リキー） | 鍵導出直後（`defer mem.ZeroBytes`） |
| 保留中のリキー鍵（FSM） | 中止時またはアクティブへの昇格時 |
| 前のセッション鍵 | 現在のエポックでの最初の復号時（TCP）またはエポック排出時（UDP） |
| ナンスリプレイウィンドウ | セッション終了時（`SlidingWindow.Zeroize`） |
| AADバッファ | セッション終了時（`DefaultUdpSession.Zeroize`） |

**制限事項:** GoのGCはゼロ化前にヒープオブジェクトをコピーする可能性があります。`mem.ZeroBytes`はメモリフォレンジックに対するベストエフォートの防御であり、コンパイラ出力分析によって最適化で除去されないことが検証されています（Go 1.25.7、全対象プラットフォーム）。

---

## 6. 定数

| 定数 | 値 | 用途 |
|------|-----|------|
| プロトコルバージョン | `0x01` | ワイヤーフォーマットのバージョン管理 |
| MAC1 / MAC2 サイズ | 16バイト | BLAKE2s-128出力 |
| Cookieバケット | 120秒 | IPバインドCookieの有効期間 |
| Cookie応答サイズ | 56バイト | nonce (24) + 暗号化Cookie (16) + tag (16) |
| AAD長 | 60バイト | sessionId (32) + direction (16) + nonce (12) |
| ナンスカウンター | 80ビット | オーバーフロー前のエポックあたりのメッセージ数 |
| リプレイウィンドウ | 1024ビット | UDP順序外パケット許容範囲 |
| エポック容量 | uint16 | 65535値、安全閾値65000 |
| リキー間隔 | 120秒 | デフォルトの定期リキートリガー |
| 保留タイムアウト | 5秒 | 未確認リキーの自動中止 |
