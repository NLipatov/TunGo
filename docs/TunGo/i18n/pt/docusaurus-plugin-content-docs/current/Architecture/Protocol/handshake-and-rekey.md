# Protocolo de Handshake e Rekeying

**Status:** Documento vivo
**Última atualização:** 2026-02-07

## Visão Geral

O TunGo utiliza um handshake **Noise IK** para autenticação mútua e acordo de chaves, seguido de rekeying periódico com **X25519 + HKDF-SHA256**. A criptografia de transporte utiliza AEAD **ChaCha20-Poly1305** com gerenciamento de nonce baseado em épocas.

**Conjunto de cifras:** X25519 / ChaChaPoly / SHA-256
**ID do protocolo:** `"TunGo"`, versão `0x01`

---

## 1. Handshake (Noise IK)

O Noise IK pressupõe que o iniciador (cliente) já conhece a chave pública estática do respondedor (servidor).

### 1.1 Fluxo de Mensagens

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

Formato de transmissão:

```
[1B version] [>=80B noise_payload] [16B MAC1] [16B MAC2]
```

- **version:** `0x01`
- **noise_payload:** Primeira mensagem Noise IK — chave pública efêmera do cliente (32B, texto claro) + chave estática do cliente criptografada (48B)
- **MAC1:** Autenticação sem estado (sempre verificada)
- **MAC2:** Autenticação baseada em cookie (verificada apenas sob carga)

Tamanho mínimo: 113 bytes.

### 1.3 MSG2 (Servidor -> Cliente)

Segunda mensagem Noise IK. Sem MACs — a autenticação bidirecional é implícita após a conclusão do Noise.

Após MSG2, ambos os lados derivam:
- `c2sKey` (32 bytes) — chave de transporte cliente-para-servidor
- `s2cKey` (32 bytes) — chave de transporte servidor-para-cliente
- `sessionId` (32 bytes) — da vinculação de canal Noise

### 1.4 Ordem de Verificação do Servidor

```
1. CheckVersion()         — reject unknown protocol versions
2. VerifyMAC1()           — stateless, before any DH or allocation
3. VerifyMAC2()           — only under load (LoadMonitor)
4. Noise handshake        — DH computations, peer lookup
5. Peer ACL check         — AllowedPeers / PeerDisabled
```

Todas as falhas retornam um `ErrHandshakeFailed` uniforme para prevenir vazamento de informações.

---

## 2. Proteção contra DoS

### 2.1 MAC1 (Sem estado, sempre obrigatório)

```
key  = BLAKE2s-256("mac1" || "TunGo" || 0x01 || server_pubkey)
MAC1 = BLAKE2s-128(key, noise_msg1)
```

Verificado antes de qualquer alocação de estado ou computação DH.

### 2.2 MAC2 (Com estado, sob carga)

```
key  = BLAKE2s-256("mac2" || "TunGo" || 0x01 || cookie_value)
MAC2 = BLAKE2s-128(key, noise_msg1 || MAC1)
```

Verificado apenas quando o `LoadMonitor` detecta pressão.

### 2.3 Mecanismo de Cookie

**Valor do cookie** (vinculado a IP, agrupado por tempo):
```
bucket = unix_seconds / 120
cookie = BLAKE2s-128(server_secret[32], client_ip[16] || bucket[2])
```

Válido para o bucket atual e anterior (trata transições).

**Resposta de cookie** (criptografada, 56 bytes):
```
[24B nonce] [16B encrypted_cookie] [16B poly1305_tag]
```

Criptografia:
```
key = BLAKE2s-256("cookie" || "TunGo" || 0x01 || server_pubkey || client_ephemeral)
ciphertext = XChaCha20-Poly1305.Seal(key, nonce, cookie, aad=client_ephemeral)
```

---

## 3. Criptografia de Transporte

### 3.1 AEAD

ChaCha20-Poly1305 com AAD de 60 bytes:

```
AAD [60 bytes]:
  [ 0..31] sessionId       (32 bytes)
  [32..47] direction        (16 bytes: "client-to-server" or "server-to-client")
  [48..59] nonce            (12 bytes)
```

SessionId e direção são preenchidos na criação da sessão. Apenas o nonce é atualizado por pacote.

### 3.2 Estrutura do Nonce (12 bytes)

```
[0..7]   counterLow   (uint64, big-endian)
[8..9]   counterHigh  (uint16, big-endian)
[10..11] epoch        (uint16, big-endian)
```

- **Counter:** 80 bits monotônico (2^80 mensagens por época). Estouro retorna erro.
- **Epoch:** Imutável por sessão, identifica a geração de rekeying.

### 3.3 Transporte TCP

```
Wire frame: [2B epoch] [ciphertext + 16B tag]
```

- Época dupla: sessão atual + anterior coexistem durante o rekeying.
- Limpeza automática: sessão anterior é zerada na primeira descriptografia da época atual (garantia de ordenação TCP).
- Sem proteção contra replay (TCP fornece ordenação).

### 3.4 Transporte UDP

```
Wire frame: [12B nonce] [ciphertext + 16B tag]
```

- Época embutida nos bytes 10..11 do nonce.
- **Proteção contra replay:** bitmap de janela deslizante de 1024 bits por época.
  - Verificação provisória antes da descriptografia (Check).
  - Confirmação apenas após autenticação AEAD bem-sucedida (Accept).
  - Previne envenenamento da janela por pacotes inválidos.
- **Anel de épocas:** FIFO de sessões com capacidade fixa. Sessões removidas são zeradas.

---

## 4. Rekeying

### 4.1 Derivação de Chaves

Ambos os lados realizam ECDH X25519 e então derivam novas chaves de transporte via HKDF-SHA256:

```
shared  = X25519(local_private, remote_public)
newC2S  = HKDF-SHA256(ikm=shared, salt=currentC2S, info="tungo-rekey-c2s")
newS2C  = HKDF-SHA256(ikm=shared, salt=currentS2C, info="tungo-rekey-s2c")
```

As chaves atuais servem como salt do HKDF, proporcionando encadeamento de forward secrecy.

### 4.2 Pacotes do Plano de Controle

```
RekeyInit:  [0xFF] [0x01] [0x02] [32B X25519 public key]   (35 bytes)
RekeyAck:   [0xFF] [0x01] [0x03] [32B X25519 public key]   (35 bytes)
```

### 4.3 FSM de Rekey

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

| Estado | Descrição |
|--------|-----------|
| **Stable** | Operação normal. Uma época de envio ativa. |
| **Rekeying** | StartRekey chamado, novas chaves computadas, nova época instalada para recepção. |
| **Pending** | Aguardando confirmação do peer (primeira descriptografia bem-sucedida com a nova época). |

### 4.4 Fluxo de Rekeying

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

### 4.5 Invariantes de Segurança

- Apenas um rekeying em andamento por vez.
- Épocas aumentam monotonicamente. Época segura máxima: 65000 (de 65535). Além disso, `ErrEpochExhausted` força um novo handshake.
- A época de envio nunca diminui.
- Chaves pendentes nunca sobrescrevem chaves ativas até que o peer comprove posse (via descriptografia bem-sucedida).
- Rekeying pendente é abortado automaticamente após 5 segundos se não houver confirmação do peer.
- Intervalo padrão de rekeying: 120 segundos.

---

## 5. Zeragem de Chaves

| Material | Quando é zerado |
|----------|-----------------|
| Chaves privadas DH efêmeras | Imediatamente após a computação DH (`defer mem.ZeroBytes`) |
| Segredos compartilhados (rekeying) | Imediatamente após a derivação de chaves (`defer mem.ZeroBytes`) |
| Chaves de rekeying pendentes (FSM) | No aborto ou promoção para ativa |
| Chaves de sessão anteriores | Na primeira descriptografia da época atual (TCP) ou remoção de época (UDP) |
| Janela de replay de nonce | No encerramento da sessão (`SlidingWindow.Zeroize`) |
| Buffers AAD | No encerramento da sessão (`DefaultUdpSession.Zeroize`) |

**Limitação:** O GC do Go pode copiar objetos do heap antes da zeragem. `mem.ZeroBytes` é uma defesa de melhor esforço contra análise forense de memória, verificada por análise da saída do compilador para não ser otimizada e removida (Go 1.25.7, todas as plataformas alvo).

---

## 6. Constantes

| Constante | Valor | Finalidade |
|-----------|-------|------------|
| Versão do protocolo | `0x01` | Versionamento do formato de transmissão |
| Tamanho MAC1 / MAC2 | 16 bytes | Saída BLAKE2s-128 |
| Bucket de cookie | 120 segundos | Janela de validade de cookie vinculado a IP |
| Tamanho da resposta de cookie | 56 bytes | nonce (24) + cookie criptografado (16) + tag (16) |
| Comprimento AAD | 60 bytes | sessionId (32) + direction (16) + nonce (12) |
| Contador de nonce | 80 bits | Mensagens por época antes do estouro |
| Janela de replay | 1024 bits | Tolerância a pacotes fora de ordem UDP |
| Capacidade de época | uint16 | 65535 valores, limiar seguro 65000 |
| Intervalo de rekeying | 120 segundos | Gatilho padrão de rekeying periódico |
| Timeout de pendência | 5 segundos | Aborto automático de rekeying não confirmado |
