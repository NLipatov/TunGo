# Protocole de Handshake et de Renouvellement des Cles

**Statut :** Document evolutif
**Derniere mise a jour :** 2026-02-07

## Apercu

TunGo utilise un handshake **Noise IK** pour l'authentification mutuelle et l'accord de cles, suivi d'un renouvellement periodique des cles via **X25519 + HKDF-SHA256**. Le chiffrement de transport utilise **ChaCha20-Poly1305** AEAD avec une gestion des nonces basee sur les epoques.

**Suite cryptographique :** X25519 / ChaChaPoly / SHA-256
**Identifiant du protocole :** `"TunGo"`, version `0x01`

---

## 1. Handshake (Noise IK)

Noise IK suppose que l'initiateur (client) connait deja la cle publique statique du repondeur (serveur).

### 1.1 Flux de messages

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

### 1.2 MSG1 (Client -> Serveur)

Format sur le reseau :

```
[1B version] [>=80B noise_payload] [16B MAC1] [16B MAC2]
```

- **version :** `0x01`
- **noise_payload :** Premier message Noise IK — cle publique ephemere du client (32B, en clair) + cle statique chiffree du client (48B)
- **MAC1 :** Authentification sans etat (toujours verifiee)
- **MAC2 :** Authentification basee sur cookie (verifiee uniquement sous charge)

Taille minimale : 113 octets.

### 1.3 MSG2 (Serveur -> Client)

Deuxieme message Noise IK. Sans MACs — l'authentification bidirectionnelle est implicite apres l'achevement de Noise.

Apres MSG2, les deux parties derivent :
- `c2sKey` (32 octets) — cle de transport client-vers-serveur
- `s2cKey` (32 octets) — cle de transport serveur-vers-client
- `sessionId` (32 octets) — a partir de la liaison de canal Noise

### 1.4 Ordre de verification du serveur

```
1. CheckVersion()         — reject unknown protocol versions
2. VerifyMAC1()           — stateless, before any DH or allocation
3. VerifyMAC2()           — only under load (LoadMonitor)
4. Noise handshake        — DH computations, peer lookup
5. Peer ACL check         — AllowedPeers / PeerDisabled
```

Tous les echecs renvoient un `ErrHandshakeFailed` uniforme pour empecher la fuite d'informations.

---

## 2. Protection contre les DoS

### 2.1 MAC1 (Sans etat, toujours requis)

```
key  = BLAKE2s-256("mac1" || "TunGo" || 0x01 || server_pubkey)
MAC1 = BLAKE2s-128(key, noise_msg1)
```

Verifie avant toute allocation d'etat ou calcul DH.

### 2.2 MAC2 (Avec etat, sous charge)

```
key  = BLAKE2s-256("mac2" || "TunGo" || 0x01 || cookie_value)
MAC2 = BLAKE2s-128(key, noise_msg1 || MAC1)
```

Verifie uniquement lorsque `LoadMonitor` detecte une pression.

### 2.3 Mecanisme de Cookie

**Valeur du cookie** (liee a l'IP, decoupee en intervalles de temps) :
```
bucket = unix_seconds / 120
cookie = BLAKE2s-128(server_secret[32], client_ip[16] || bucket[2])
```

Valide pour l'intervalle actuel et le precedent (gestion des transitions).

**Reponse cookie** (chiffree, 56 octets) :
```
[24B nonce] [16B encrypted_cookie] [16B poly1305_tag]
```

Chiffrement :
```
key = BLAKE2s-256("cookie" || "TunGo" || 0x01 || server_pubkey || client_ephemeral)
ciphertext = XChaCha20-Poly1305.Seal(key, nonce, cookie, aad=client_ephemeral)
```

---

## 3. Chiffrement de transport

### 3.1 AEAD

ChaCha20-Poly1305 avec AAD de 60 octets :

```
AAD [60 bytes]:
  [ 0..31] sessionId       (32 bytes)
  [32..47] direction        (16 bytes: "client-to-server" or "server-to-client")
  [48..59] nonce            (12 bytes)
```

SessionId et direction sont preremplis a la creation de la session. Seul le nonce est mis a jour par paquet.

### 3.2 Structure du Nonce (12 octets)

```
[0..7]   counterLow   (uint64, big-endian)
[8..9]   counterHigh  (uint16, big-endian)
[10..11] epoch        (uint16, big-endian)
```

- **Compteur :** 80 bits monotone (2^80 messages par epoque). Le depassement renvoie une erreur.
- **Epoque :** Immuable par session, identifie la generation de renouvellement des cles.

### 3.3 Transport TCP

```
Wire frame: [2B epoch] [ciphertext + 16B tag]
```

- Double epoque : la session actuelle et la precedente coexistent pendant le renouvellement des cles.
- Nettoyage automatique : la session precedente est mise a zero lors du premier dechiffrement avec l'epoque actuelle (garantie d'ordonnancement TCP).
- Pas de protection contre la repetition (TCP assure l'ordonnancement).

### 3.4 Transport UDP

```
Wire frame: [12B nonce] [ciphertext + 16B tag]
```

- Epoque integree dans les octets 10..11 du nonce.
- **Protection contre la repetition :** Fenetre glissante de 1024 bits (bitmap) par epoque.
  - Verification provisoire avant le dechiffrement (Check).
  - Confirmation uniquement apres la reussite de l'authentification AEAD (Accept).
  - Empeche l'empoisonnement de la fenetre par des paquets invalides.
- **Anneau d'epoques :** FIFO a capacite fixe pour les sessions. Les sessions evincees sont mises a zero.

---

## 4. Renouvellement des cles

### 4.1 Derivation des cles

Les deux parties effectuent un X25519 ECDH, puis derivent de nouvelles cles de transport via HKDF-SHA256 :

```
shared  = X25519(local_private, remote_public)
newC2S  = HKDF-SHA256(ikm=shared, salt=currentC2S, info="tungo-rekey-c2s")
newS2C  = HKDF-SHA256(ikm=shared, salt=currentS2C, info="tungo-rekey-s2c")
```

Les cles actuelles servent de sel HKDF, assurant le chainage de la confidentialite persistante.

### 4.2 Paquets du plan de controle

```
RekeyInit:  [0xFF] [0x01] [0x02] [32B X25519 public key]   (35 bytes)
RekeyAck:   [0xFF] [0x01] [0x03] [32B X25519 public key]   (35 bytes)
```

### 4.3 Machine a etats du renouvellement des cles

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

| Etat | Description |
|------|-------------|
| **Stable** | Fonctionnement normal. Une seule epoque d'envoi active. |
| **Rekeying** | StartRekey appele, nouvelles cles calculees, nouvelle epoque installee pour la reception. |
| **Pending** | En attente de la confirmation du pair (premier dechiffrement reussi avec la nouvelle epoque). |

### 4.4 Flux de renouvellement des cles

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

### 4.5 Invariants de securite

- Un seul renouvellement de cles en cours a la fois.
- Les epoques augmentent de maniere monotone. Epoque sure maximale : 65000 (sur 65535). Au-dela, `ErrEpochExhausted` force un nouveau handshake.
- L'epoque d'envoi ne diminue jamais.
- Les cles en attente ne remplacent jamais les cles actives tant que le pair n'a pas prouve la possession (via un dechiffrement reussi).
- Le renouvellement en attente s'annule automatiquement apres 5 secondes sans confirmation du pair.
- Intervalle de renouvellement des cles par defaut : 120 secondes.

---

## 5. Mise a zero des cles

| Materiau | Moment de la mise a zero |
|----------|--------------------------|
| Cles privees DH ephemeres | Immediatement apres le calcul DH (`defer mem.ZeroBytes`) |
| Secrets partages (renouvellement des cles) | Immediatement apres la derivation des cles (`defer mem.ZeroBytes`) |
| Cles en attente de renouvellement (machine a etats) | A l'abandon ou a la promotion en cles actives |
| Cles de la session precedente | Au premier dechiffrement avec l'epoque actuelle (TCP) ou a l'eviction de l'epoque (UDP) |
| Fenetre de repetition des nonces | A la fermeture de la session (`SlidingWindow.Zeroize`) |
| Tampons AAD | A la fermeture de la session (`DefaultUdpSession.Zeroize`) |

**Limitation :** Le ramasse-miettes de Go peut copier des objets du tas avant la mise a zero. `mem.ZeroBytes` est une defense au mieux contre l'analyse forensique de la memoire, verifiee par l'analyse de la sortie du compilateur pour confirmer l'absence d'optimisation (Go 1.25.7, toutes les plateformes cibles).

---

## 6. Constantes

| Constante | Valeur | Objectif |
|-----------|--------|----------|
| Version du protocole | `0x01` | Gestion des versions du format reseau |
| Taille de MAC1 / MAC2 | 16 octets | Sortie BLAKE2s-128 |
| Intervalle de cookie | 120 secondes | Fenetre de validite du cookie lie a l'IP |
| Taille de la reponse cookie | 56 octets | nonce (24) + cookie chiffre (16) + tag (16) |
| Longueur de AAD | 60 octets | sessionId (32) + direction (16) + nonce (12) |
| Compteur de nonce | 80 bits | Messages par epoque avant depassement |
| Fenetre de repetition | 1024 bits | Tolerance au desordre UDP |
| Capacite des epoques | uint16 | 65535 valeurs, seuil de securite 65000 |
| Intervalle de renouvellement des cles | 120 secondes | Declencheur periodique de renouvellement par defaut |
| Delai d'attente | 5 secondes | Abandon automatique du renouvellement non confirme |
