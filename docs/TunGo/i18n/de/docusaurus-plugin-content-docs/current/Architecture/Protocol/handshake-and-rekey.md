# Handshake- und Rekeying-Protokoll

**Status:** Lebendes Dokument
**Zuletzt aktualisiert:** 2026-02-07

## Überblick

TunGo verwendet einen **Noise IK**-Handshake für gegenseitige Authentifizierung und Schlüsselvereinbarung, gefolgt von periodischem **X25519 + HKDF-SHA256**-Rekeying. Die Transportverschlüsselung nutzt **ChaCha20-Poly1305** AEAD mit epochenbasierter Nonce-Verwaltung.

**Cipher-Suite:** X25519 / ChaChaPoly / SHA-256
**Protokoll-ID:** `"TunGo"`, Version `0x01`

---

## 1. Handshake (Noise IK)

Noise IK setzt voraus, dass der Initiator (Client) den statischen öffentlichen Schlüssel des Responders (Server) bereits kennt.

### 1.1 Nachrichtenfluss

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

Drahtformat:

```
[1B version] [>=80B noise_payload] [16B MAC1] [16B MAC2]
```

- **version:** `0x01`
- **noise_payload:** Noise IK erste Nachricht — öffentlicher ephemerer Client-Schlüssel (32B, Klartext) + verschlüsselter statischer Client-Schlüssel (48B)
- **MAC1:** Zustandslose Authentifizierung (wird immer verifiziert)
- **MAC2:** Cookie-basierte Authentifizierung (wird nur unter Last verifiziert)

Minimalgröße: 113 Bytes.

### 1.3 MSG2 (Server -> Client)

Noise IK zweite Nachricht. Keine MACs — bidirektionale Authentifizierung ist nach Abschluss von Noise implizit.

Nach MSG2 leiten beide Seiten ab:
- `c2sKey` (32 Bytes) — Client-zu-Server-Transportschlüssel
- `s2cKey` (32 Bytes) — Server-zu-Client-Transportschlüssel
- `sessionId` (32 Bytes) — aus der Noise-Kanalbindung

### 1.4 Server-Verifizierungsreihenfolge

```
1. CheckVersion()         — reject unknown protocol versions
2. VerifyMAC1()           — stateless, before any DH or allocation
3. VerifyMAC2()           — only under load (LoadMonitor)
4. Noise handshake        — DH computations, peer lookup
5. Peer ACL check         — AllowedPeers / PeerDisabled
```

Alle Fehler liefern ein einheitliches `ErrHandshakeFailed` zurück, um Informationslecks zu verhindern.

---

## 2. DoS-Schutz

### 2.1 MAC1 (Zustandslos, immer erforderlich)

```
key  = BLAKE2s-256("mac1" || "TunGo" || 0x01 || server_pubkey)
MAC1 = BLAKE2s-128(key, noise_msg1)
```

Wird vor jeder Zustandsallokation oder DH-Berechnung verifiziert.

### 2.2 MAC2 (Zustandsbehaftet, unter Last)

```
key  = BLAKE2s-256("mac2" || "TunGo" || 0x01 || cookie_value)
MAC2 = BLAKE2s-128(key, noise_msg1 || MAC1)
```

Wird nur geprüft, wenn `LoadMonitor` Lastdruck erkennt.

### 2.3 Cookie-Mechanismus

**Cookie-Wert** (IP-gebunden, zeitbucketiert):
```
bucket = unix_seconds / 120
cookie = BLAKE2s-128(server_secret[32], client_ip[16] || bucket[2])
```

Gültig für den aktuellen und vorherigen Bucket (behandelt Übergänge).

**Cookie-Antwort** (verschlüsselt, 56 Bytes):
```
[24B nonce] [16B encrypted_cookie] [16B poly1305_tag]
```

Verschlüsselung:
```
key = BLAKE2s-256("cookie" || "TunGo" || 0x01 || server_pubkey || client_ephemeral)
ciphertext = XChaCha20-Poly1305.Seal(key, nonce, cookie, aad=client_ephemeral)
```

---

## 3. Transportverschlüsselung

### 3.1 AEAD

ChaCha20-Poly1305 mit 60-Byte AAD:

```
AAD [60 bytes]:
  [ 0..31] sessionId       (32 bytes)
  [32..47] direction        (16 bytes: "client-to-server" or "server-to-client")
  [48..59] nonce            (12 bytes)
```

SessionId und Richtung werden bei der Sitzungserstellung vorbelegt. Nur die Nonce wird pro Paket aktualisiert.

### 3.2 Nonce-Struktur (12 Bytes)

```
[0..7]   counterLow   (uint64, big-endian)
[8..9]   counterHigh  (uint16, big-endian)
[10..11] epoch        (uint16, big-endian)
```

- **Counter:** 80-Bit monoton (2^80 Nachrichten pro Epoche). Überlauf gibt einen Fehler zurück.
- **Epoch:** Unveränderlich pro Sitzung, identifiziert die Rekeying-Generation.

### 3.3 TCP-Transport

```
Wire frame: [2B epoch] [ciphertext + 16B tag]
```

- Duale Epoche: aktuelle + vorherige Sitzung koexistieren während des Rekeying.
- Automatische Bereinigung: vorherige Sitzung wird bei der ersten Entschlüsselung der aktuellen Epoche genullt (TCP-Reihenfolgegarantie).
- Kein Replay-Schutz (TCP stellt die Reihenfolge sicher).

### 3.4 UDP-Transport

```
Wire frame: [12B nonce] [ciphertext + 16B tag]
```

- Epoche eingebettet in Nonce-Bytes 10..11.
- **Replay-Schutz:** 1024-Bit-Schiebefenster-Bitmap pro Epoche.
  - Vorläufige Prüfung vor der Entschlüsselung (Check).
  - Bestätigung erst nach erfolgreicher AEAD-Authentifizierung (Accept).
  - Verhindert Fenstervergiftung durch ungültige Pakete.
- **Epoch-Ring:** FIFO mit fester Kapazität für Sitzungen. Entfernte Sitzungen werden genullt.

---

## 4. Rekeying

### 4.1 Schlüsselableitung

Beide Seiten führen X25519 ECDH durch und leiten dann neue Transportschlüssel über HKDF-SHA256 ab:

```
shared  = X25519(local_private, remote_public)
newC2S  = HKDF-SHA256(ikm=shared, salt=currentC2S, info="tungo-rekey-c2s")
newS2C  = HKDF-SHA256(ikm=shared, salt=currentS2C, info="tungo-rekey-s2c")
```

Aktuelle Schlüssel dienen als HKDF-Salt und ermöglichen verkettete Forward Secrecy.

### 4.2 Steuerungsebene-Pakete

```
RekeyInit:  [0xFF] [0x01] [0x02] [32B X25519 public key]   (35 bytes)
RekeyAck:   [0xFF] [0x01] [0x03] [32B X25519 public key]   (35 bytes)
```

### 4.3 Rekey-FSM

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

| Zustand | Beschreibung |
|---------|--------------|
| **Stable** | Normalbetrieb. Eine aktive Sende-Epoche. |
| **Rekeying** | StartRekey aufgerufen, neue Schlüssel berechnet, neue Epoche für den Empfang installiert. |
| **Pending** | Wartet auf Bestätigung des Peers (erste erfolgreiche Entschlüsselung mit neuer Epoche). |

### 4.4 Rekeying-Ablauf

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

### 4.5 Sicherheitsinvarianten

- Nur ein laufendes Rekeying gleichzeitig.
- Epochen steigen monoton. Maximale sichere Epoche: 65000 (von 65535). Darüber hinaus erzwingt `ErrEpochExhausted` einen erneuten Handshake.
- Die Sende-Epoche nimmt nie ab.
- Ausstehende Schlüssel überschreiben niemals aktive Schlüssel, bis der Peer den Besitz nachweist (durch erfolgreiche Entschlüsselung).
- Ausstehendes Rekeying wird nach 5 Sekunden automatisch abgebrochen, wenn keine Bestätigung vom Peer erfolgt.
- Standard-Rekeying-Intervall: 120 Sekunden.

---

## 5. Schlüssellöschung

| Material | Wann gelöscht |
|----------|---------------|
| Ephemere DH-Privatschlüssel | Sofort nach der DH-Berechnung (`defer mem.ZeroBytes`) |
| Gemeinsame Geheimnisse (Rekeying) | Sofort nach der Schlüsselableitung (`defer mem.ZeroBytes`) |
| Ausstehende Rekeying-Schlüssel (FSM) | Bei Abbruch oder Beförderung zum aktiven Schlüssel |
| Vorherige Sitzungsschlüssel | Bei der ersten Entschlüsselung der aktuellen Epoche (TCP) oder Epochen-Verdrängung (UDP) |
| Nonce-Replay-Fenster | Bei Sitzungsabbau (`SlidingWindow.Zeroize`) |
| AAD-Puffer | Bei Sitzungsabbau (`DefaultUdpSession.Zeroize`) |

**Einschränkung:** Der Go-GC kann Heap-Objekte vor dem Nullen kopieren. `mem.ZeroBytes` ist eine Best-Effort-Verteidigung gegen Speicherforensik, durch Compiler-Ausgabeanalyse verifiziert, dass sie nicht wegoptimiert wird (Go 1.25.7, alle Zielplattformen).

---

## 6. Konstanten

| Konstante | Wert | Zweck |
|-----------|------|-------|
| Protokollversion | `0x01` | Drahtformat-Versionierung |
| MAC1- / MAC2-Größe | 16 Bytes | BLAKE2s-128-Ausgabe |
| Cookie-Bucket | 120 Sekunden | IP-gebundenes Cookie-Gültigkeitsfenster |
| Cookie-Antwortgröße | 56 Bytes | Nonce (24) + verschlüsseltes Cookie (16) + Tag (16) |
| AAD-Länge | 60 Bytes | sessionId (32) + direction (16) + nonce (12) |
| Nonce-Counter | 80 Bits | Nachrichten pro Epoche vor Überlauf |
| Replay-Fenster | 1024 Bits | UDP-Toleranz für ungeordnete Pakete |
| Epochen-Kapazität | uint16 | 65535 Werte, sicherer Schwellenwert 65000 |
| Rekeying-Intervall | 120 Sekunden | Standard-Auslöser für periodisches Rekeying |
| Ausstehend-Timeout | 5 Sekunden | Automatischer Abbruch eines unbestätigten Rekeying |
