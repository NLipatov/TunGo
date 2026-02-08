# Protokol Handshake dan Rekeying

**Status:** Dokumen yang terus diperbarui
**Terakhir diperbarui:** 2026-02-07

## Ringkasan

TunGo menggunakan handshake **Noise IK** untuk autentikasi timbal balik dan kesepakatan kunci, diikuti oleh rekeying berkala menggunakan **X25519 + HKDF-SHA256**. Enkripsi transport menggunakan AEAD **ChaCha20-Poly1305** dengan manajemen nonce berbasis epoch.

**Cipher suite:** X25519 / ChaChaPoly / SHA-256
**ID Protokol:** `"TunGo"`, versi `0x01`

---

## 1. Handshake (Noise IK)

Noise IK mengasumsikan bahwa inisiator (klien) sudah mengetahui kunci publik statis dari responder (server).

### 1.1 Alur Pesan

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

### 1.2 MSG1 (Klien -> Server)

Format kawat:

```
[1B version] [>=80B noise_payload] [16B MAC1] [16B MAC2]
```

- **version:** `0x01`
- **noise_payload:** Pesan pertama Noise IK — kunci publik efemeral klien (32B, teks terang) + kunci statis klien terenkripsi (48B)
- **MAC1:** Autentikasi tanpa status (selalu diverifikasi)
- **MAC2:** Autentikasi berbasis Cookie (diverifikasi hanya saat beban tinggi)

Ukuran minimum: 113 byte.

### 1.3 MSG2 (Server -> Klien)

Pesan kedua Noise IK. Tanpa MAC — autentikasi dua arah bersifat implisit setelah Noise selesai.

Setelah MSG2, kedua sisi menurunkan:
- `c2sKey` (32 byte) — kunci transport klien-ke-server
- `s2cKey` (32 byte) — kunci transport server-ke-klien
- `sessionId` (32 byte) — dari pengikatan kanal Noise

### 1.4 Urutan Verifikasi Server

```
1. CheckVersion()         — reject unknown protocol versions
2. VerifyMAC1()           — stateless, before any DH or allocation
3. VerifyMAC2()           — only under load (LoadMonitor)
4. Noise handshake        — DH computations, peer lookup
5. Peer ACL check         — AllowedPeers / PeerDisabled
```

Semua kegagalan mengembalikan `ErrHandshakeFailed` yang seragam untuk mencegah kebocoran informasi.

---

## 2. Perlindungan DoS

### 2.1 MAC1 (Tanpa Status, Selalu Diperlukan)

```
key  = BLAKE2s-256("mac1" || "TunGo" || 0x01 || server_pubkey)
MAC1 = BLAKE2s-128(key, noise_msg1)
```

Diverifikasi sebelum alokasi status atau komputasi DH apa pun.

### 2.2 MAC2 (Berisi Status, Saat Beban Tinggi)

```
key  = BLAKE2s-256("mac2" || "TunGo" || 0x01 || cookie_value)
MAC2 = BLAKE2s-128(key, noise_msg1 || MAC1)
```

Diperiksa hanya ketika `LoadMonitor` mendeteksi tekanan.

### 2.3 Mekanisme Cookie

**Nilai Cookie** (terikat IP, dibagi berdasarkan waktu):
```
bucket = unix_seconds / 120
cookie = BLAKE2s-128(server_secret[32], client_ip[16] || bucket[2])
```

Berlaku untuk bucket saat ini dan sebelumnya (menangani transisi).

**Balasan Cookie** (terenkripsi, 56 byte):
```
[24B nonce] [16B encrypted_cookie] [16B poly1305_tag]
```

Enkripsi:
```
key = BLAKE2s-256("cookie" || "TunGo" || 0x01 || server_pubkey || client_ephemeral)
ciphertext = XChaCha20-Poly1305.Seal(key, nonce, cookie, aad=client_ephemeral)
```

---

## 3. Enkripsi Transport

### 3.1 AEAD

ChaCha20-Poly1305 dengan AAD 60 byte:

```
AAD [60 bytes]:
  [ 0..31] sessionId       (32 bytes)
  [32..47] direction        (16 bytes: "client-to-server" or "server-to-client")
  [48..59] nonce            (12 bytes)
```

SessionId dan direction diisi terlebih dahulu saat pembuatan sesi. Hanya nonce yang diperbarui per paket.

### 3.2 Struktur Nonce (12 byte)

```
[0..7]   counterLow   (uint64, big-endian)
[8..9]   counterHigh  (uint16, big-endian)
[10..11] epoch        (uint16, big-endian)
```

- **Counter:** 80-bit monotonik (2^80 pesan per epoch). Overflow menghasilkan error.
- **Epoch:** Tidak dapat diubah per sesi, mengidentifikasi generasi rekeying.

### 3.3 Transport TCP

```
Wire frame: [2B epoch] [ciphertext + 16B tag]
```

- Epoch ganda: sesi saat ini dan sebelumnya hidup berdampingan selama rekey.
- Pembersihan otomatis: sesi sebelumnya di-nol-kan pada dekripsi pertama epoch saat ini (jaminan pengurutan TCP).
- Tanpa perlindungan replay (TCP menyediakan pengurutan).

### 3.4 Transport UDP

```
Wire frame: [12B nonce] [ciphertext + 16B tag]
```

- Epoch tertanam dalam byte nonce 10..11.
- **Perlindungan replay:** Bitmap jendela geser 1024-bit per epoch.
  - Pemeriksaan tentatif sebelum dekripsi (Check).
  - Dikonfirmasi hanya setelah autentikasi AEAD berhasil (Accept).
  - Mencegah pencemaran jendela oleh paket tidak valid.
- **Epoch ring:** FIFO berkapasitas tetap untuk sesi. Sesi yang dikeluarkan akan di-nol-kan.

---

## 4. Rekeying

### 4.1 Penurunan Kunci

Kedua sisi melakukan X25519 ECDH, kemudian menurunkan kunci transport baru melalui HKDF-SHA256:

```
shared  = X25519(local_private, remote_public)
newC2S  = HKDF-SHA256(ikm=shared, salt=currentC2S, info="tungo-rekey-c2s")
newS2C  = HKDF-SHA256(ikm=shared, salt=currentS2C, info="tungo-rekey-s2c")
```

Kunci saat ini berfungsi sebagai salt HKDF, memberikan rantai kerahasiaan maju (forward secrecy).

### 4.2 Paket Control Plane

```
RekeyInit:  [0xFF] [0x01] [0x02] [32B X25519 public key]   (35 bytes)
RekeyAck:   [0xFF] [0x01] [0x03] [32B X25519 public key]   (35 bytes)
```

### 4.3 FSM Rekey

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

| Status | Deskripsi |
|-------|-------------|
| **Stable** | Operasi normal. Satu epoch pengiriman aktif. |
| **Rekeying** | StartRekey dipanggil, kunci baru dihitung, epoch baru dipasang untuk penerimaan. |
| **Pending** | Menunggu konfirmasi peer (dekripsi pertama yang berhasil dengan epoch baru). |

### 4.4 Alur Rekey

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

### 4.5 Invarian Keamanan

- Hanya satu rekey yang sedang berlangsung pada satu waktu.
- Epoch meningkat secara monotonik. Epoch aman maksimum: 65000 (dari 65535). Melampaui batas ini, `ErrEpochExhausted` memaksa handshake ulang.
- Epoch pengiriman tidak pernah menurun.
- Kunci yang menunggu konfirmasi tidak pernah menimpa kunci aktif sampai peer membuktikan kepemilikan (melalui dekripsi yang berhasil).
- Rekey yang menunggu konfirmasi dibatalkan secara otomatis setelah 5 detik jika tidak ada konfirmasi peer.
- Interval rekey default: 120 detik.

---

## 5. Penghapusan Kunci (Zeroization)

| Material | Kapan Di-nol-kan |
|----------|-----------------|
| Kunci privat DH efemeral | Segera setelah komputasi DH (`defer mem.ZeroBytes`) |
| Rahasia bersama (rekey) | Segera setelah penurunan kunci (`defer mem.ZeroBytes`) |
| Kunci rekey yang menunggu konfirmasi (FSM) | Saat dibatalkan atau dipromosikan menjadi aktif |
| Kunci sesi sebelumnya | Pada dekripsi pertama epoch saat ini (TCP) atau pengusiran epoch (UDP) |
| Jendela replay Nonce | Saat pembongkaran sesi (`SlidingWindow.Zeroize`) |
| Buffer AAD | Saat pembongkaran sesi (`DefaultUdpSession.Zeroize`) |

**Keterbatasan:** GC Go mungkin menyalin objek heap sebelum penghapusan. `mem.ZeroBytes` adalah pertahanan terbaik yang mungkin terhadap forensik memori, diverifikasi melalui analisis output compiler untuk memastikan tidak dioptimalkan (Go 1.25.7, semua platform target).

---

## 6. Konstanta

| Konstanta | Nilai | Tujuan |
|----------|-------|---------
| Protocol version | `0x01` | Pemversian format kawat |
| MAC1 / MAC2 size | 16 byte | Output BLAKE2s-128 |
| Cookie bucket | 120 detik | Jendela validitas cookie terikat IP |
| Cookie reply size | 56 byte | nonce (24) + encrypted cookie (16) + tag (16) |
| AAD length | 60 byte | sessionId (32) + direction (16) + nonce (12) |
| Nonce counter | 80 bit | Pesan per epoch sebelum overflow |
| Replay window | 1024 bit | Toleransi ketidakterurutan UDP |
| Epoch capacity | uint16 | 65535 nilai, ambang aman 65000 |
| Rekey interval | 120 detik | Pemicu rekey berkala default |
| Pending timeout | 5 detik | Pembatalan otomatis rekey yang belum dikonfirmasi |
