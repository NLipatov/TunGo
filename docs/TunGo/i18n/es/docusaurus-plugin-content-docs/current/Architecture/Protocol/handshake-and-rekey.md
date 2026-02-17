# Protocolo de Handshake y Renovacion de Claves

**Estado:** Documento activo
**Ultima actualizacion:** 2026-02-17

## Descripcion general

TunGo utiliza un handshake **Noise IK** para autenticacion mutua y acuerdo de claves, seguido de una renovacion periodica de claves mediante **X25519 + HKDF-SHA256**. El cifrado de transporte usa **ChaCha20-Poly1305** AEAD con gestion de nonce basada en epocas.

**Suite de cifrado:** X25519 / ChaChaPoly / SHA-256
**Identificador de protocolo:** `"TunGo"`, version `0x01`

---

## 1. Handshake (Noise IK)

Noise IK asume que el iniciador (cliente) ya conoce la clave publica estatica del respondedor (servidor).

### 1.1 Flujo de mensajes

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

### 1.2 MSG1 (Cliente -> Servidor)

Formato en el cable:

```
[1B version] [>=80B noise_payload] [16B MAC1] [16B MAC2]
```

- **version:** `0x01`
- **noise_payload:** Primer mensaje Noise IK — clave publica efimera del cliente (32B, texto plano) + clave estatica cifrada del cliente (48B)
- **MAC1:** Autenticacion sin estado (siempre verificada)
- **MAC2:** Autenticacion basada en cookie (verificada solo bajo carga)

Tamano minimo: 113 bytes.

### 1.3 MSG2 (Servidor -> Cliente)

Segundo mensaje Noise IK. Sin MACs — la autenticacion bidireccional es implicita tras completar Noise.

Despues de MSG2, ambas partes derivan:
- `c2sKey` (32 bytes) — clave de transporte cliente-a-servidor
- `s2cKey` (32 bytes) — clave de transporte servidor-a-cliente
- `sessionId` (32 bytes) — de la vinculacion de canal Noise

### 1.4 Orden de verificacion del servidor

```
1. CheckVersion()         — reject unknown protocol versions
2. VerifyMAC1()           — stateless, before any DH or allocation
3. VerifyMAC2()           — only under load (LoadMonitor)
4. Noise handshake        — DH computations, peer lookup
5. Peer ACL check         — AllowedPeers / PeerDisabled
```

Todos los fallos devuelven un `ErrHandshakeFailed` uniforme para prevenir la fuga de informacion.

---

## 2. Proteccion contra DoS

### 2.1 MAC1 (Sin estado, siempre requerido)

```
key  = BLAKE2s-256("mac1" || "TunGo" || 0x01 || server_pubkey)
MAC1 = BLAKE2s-128(key, noise_msg1)
```

Se verifica antes de cualquier asignacion de estado o computacion DH.

### 2.2 MAC2 (Con estado, bajo carga)

```
key  = BLAKE2s-256("mac2" || "TunGo" || 0x01 || cookie_value)
MAC2 = BLAKE2s-128(key, noise_msg1 || MAC1)
```

Se verifica solo cuando `LoadMonitor` detecta presion.

### 2.3 Mecanismo de Cookie

**Valor del cookie** (vinculado a IP, agrupado por intervalos de tiempo):
```
bucket = unix_seconds / 120
cookie = BLAKE2s-128(server_secret[32], client_ip[16] || bucket[2])
```

Valido para el intervalo actual y el anterior (manejo de transiciones).

**Respuesta cookie** (cifrada, 56 bytes):
```
[24B nonce] [16B encrypted_cookie] [16B poly1305_tag]
```

Cifrado:
```
key = BLAKE2s-256("cookie" || "TunGo" || 0x01 || server_pubkey || client_ephemeral)
ciphertext = XChaCha20-Poly1305.Seal(key, nonce, cookie, aad=client_ephemeral)
```

---

## 3. Cifrado de transporte

### 3.1 AEAD

ChaCha20-Poly1305 con AAD de 60 bytes:

```
AAD [60 bytes]:
  [ 0..31] sessionId       (32 bytes)
  [32..47] direction        (16 bytes: "client-to-server" or "server-to-client")
  [48..59] nonce            (12 bytes)
```

SessionId y direction se rellenan previamente al crear la sesion. Solo el nonce se actualiza por paquete.

### 3.2 Estructura del Nonce (12 bytes)

```
[0..7]   counterLow   (uint64, big-endian)
[8..9]   counterHigh  (uint16, big-endian)
[10..11] epoch        (uint16, big-endian)
```

- **Contador:** 80 bits monotonico (2^80 mensajes por epoca). El desbordamiento devuelve error.
- **Epoca:** Inmutable por sesion, identifica la generacion de renovacion de claves.

### 3.3 Transporte TCP

```
Wire frame: [2B epoch] [ciphertext + 16B tag]
```

- Doble epoca: la sesion actual y la anterior coexisten durante la renovacion de claves.
- Limpieza automatica: la sesion anterior se pone a cero en el primer descifrado con la epoca actual (garantia de orden TCP).
- Sin proteccion contra repeticion (TCP proporciona ordenamiento).

### 3.4 Transporte UDP

```
Wire frame: [8B route-id] [12B nonce] [ciphertext + 16B tag]
```

- Route-id is derived from `sessionId` (first 8 bytes, big-endian) and enables O(1) session lookup.

- Epoca incrustada en los bytes 10..11 del nonce.
- **Proteccion contra repeticion:** Ventana deslizante de 1024 bits (bitmap) por epoca.
  - Verificacion tentativa antes del descifrado (Check).
  - Se confirma solo despues de que la autenticacion AEAD tiene exito (Accept).
  - Previene el envenenamiento de la ventana por paquetes invalidos.
- **Anillo de epocas:** FIFO de capacidad fija para sesiones. Las sesiones desalojadas se ponen a cero.

---

## 4. Renovacion de claves

### 4.1 Derivacion de claves

Ambas partes realizan X25519 ECDH, luego derivan nuevas claves de transporte mediante HKDF-SHA256:

```
shared  = X25519(local_private, remote_public)
newC2S  = HKDF-SHA256(ikm=shared, salt=currentC2S, info="tungo-rekey-c2s")
newS2C  = HKDF-SHA256(ikm=shared, salt=currentS2C, info="tungo-rekey-s2c")
```

Las claves actuales sirven como sal de HKDF, proporcionando encadenamiento de secreto perfecto hacia adelante.

### 4.2 Paquetes del plano de control

```
RekeyInit:  [0xFF] [0x01] [0x02] [32B X25519 public key]   (35 bytes)
RekeyAck:   [0xFF] [0x01] [0x03] [32B X25519 public key]   (35 bytes)
```

### 4.3 Maquina de estados de renovacion de claves

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

| Estado | Descripcion |
|--------|-------------|
| **Stable** | Operacion normal. Una epoca de envio activa. |
| **Rekeying** | StartRekey invocado, nuevas claves calculadas, nueva epoca instalada para recepcion. |
| **Pending** | Esperando confirmacion del par (primer descifrado exitoso con la nueva epoca). |

### 4.4 Flujo de renovacion de claves

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

### 4.5 Invariantes de seguridad

- Solo una renovacion de claves en curso a la vez.
- Las epocas aumentan monotonicamente. Epoca segura maxima: 65000 (de 65535). Mas alla, `ErrEpochExhausted` fuerza un nuevo handshake.
- La epoca de envio nunca disminuye.
- Las claves pendientes nunca sobrescriben las activas hasta que el par demuestra posesion (mediante descifrado exitoso).
- La renovacion pendiente se aborta automaticamente despues de 5 segundos si no hay confirmacion del par.
- Intervalo de renovacion de claves por defecto: 120 segundos.

---

## 5. Puesta a cero de claves

| Material | Momento de puesta a cero |
|----------|--------------------------|
| Claves privadas DH efimeras | Inmediatamente despues de la computacion DH (`defer mem.ZeroBytes`) |
| Secretos compartidos (renovacion de claves) | Inmediatamente despues de la derivacion de claves (`defer mem.ZeroBytes`) |
| Claves pendientes de renovacion (maquina de estados) | Al abortar o al promover a activas |
| Claves de sesion anterior | En el primer descifrado con la epoca actual (TCP) o al desalojar la epoca (UDP) |
| Ventana de repeticion de nonce | Al finalizar la sesion (`SlidingWindow.Zeroize`) |
| Buffers AAD | Al finalizar la sesion (`DefaultUdpSession.Zeroize`) |

**Limitacion:** El recolector de basura de Go puede copiar objetos del heap antes de la puesta a cero. `mem.ZeroBytes` es una defensa de mejor esfuerzo contra el analisis forense de memoria, verificada mediante analisis de la salida del compilador para confirmar que no se optimiza (Go 1.26.x, todas las plataformas objetivo).

---

## 6. Constantes

| Constante | Valor | Proposito |
|-----------|-------|-----------|
| Version del protocolo | `0x01` | Versionado del formato en el cable |
| Tamano de MAC1 / MAC2 | 16 bytes | Salida de BLAKE2s-128 |
| Intervalo de cookie | 120 segundos | Ventana de validez del cookie vinculado a IP |
| Tamano de respuesta cookie | 56 bytes | nonce (24) + cookie cifrado (16) + tag (16) |
| Longitud de AAD | 60 bytes | sessionId (32) + direction (16) + nonce (12) |
| UDP route-id | 8 bytes | session identifier prefix for O(1) peer lookup |
| Contador de nonce | 80 bits | Mensajes por epoca antes del desbordamiento |
| Ventana de repeticion | 1024 bits | Tolerancia de desorden en UDP |
| Capacidad de epocas | uint16 | 65535 valores, umbral seguro 65000 |
| Intervalo de renovacion de claves | 120 segundos | Disparador periodico de renovacion por defecto |
| Tiempo de espera pendiente | 5 segundos | Aborto automatico de renovacion no confirmada |
