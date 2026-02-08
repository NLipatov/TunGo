# بروتوكول المصافحة وتجديد المفاتيح

**الحالة:** مستند قابل للتحديث المستمر
**آخر تحديث:** 2026-02-07

## نظرة عامة

يستخدم TunGo مصافحة **Noise IK** للمصادقة المتبادلة والاتفاق على المفاتيح، يليه تجديد دوري للمفاتيح باستخدام **X25519 + HKDF-SHA256**. يستخدم تشفير النقل **ChaCha20-Poly1305** AEAD مع إدارة أرقام تسلسلية (nonce) قائمة على الحقب (epoch).

**مجموعة التشفير:** X25519 / ChaChaPoly / SHA-256
**معرّف البروتوكول:** `"TunGo"`، الإصدار `0x01`

---

## 1. المصافحة (Noise IK)

يفترض Noise IK أن المُبادر (العميل) يعرف مسبقًا المفتاح العام الثابت للمستجيب (الخادم).

### 1.1 تدفق الرسائل

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

### 1.2 MSG1 (العميل -> الخادم)

صيغة السلك:

```
[1B version] [>=80B noise_payload] [16B MAC1] [16B MAC2]
```

- **version:** `0x01`
- **noise_payload:** رسالة Noise IK الأولى — المفتاح العام المؤقت للعميل (32B، نص صريح) + المفتاح الثابت المشفر للعميل (48B)
- **MAC1:** مصادقة عديمة الحالة (يتم التحقق منها دائمًا)
- **MAC2:** مصادقة قائمة على Cookie (يتم التحقق منها فقط تحت الحمل)

الحجم الأدنى: 113 بايت.

### 1.3 MSG2 (الخادم -> العميل)

رسالة Noise IK الثانية. بدون MAC — المصادقة ثنائية الاتجاه ضمنية بعد اكتمال Noise.

بعد MSG2، يستخلص الطرفان:
- `c2sKey` (32 بايت) — مفتاح النقل من العميل إلى الخادم
- `s2cKey` (32 بايت) — مفتاح النقل من الخادم إلى العميل
- `sessionId` (32 بايت) — من ربط قناة Noise

### 1.4 ترتيب التحقق في الخادم

```
1. CheckVersion()         — reject unknown protocol versions
2. VerifyMAC1()           — stateless, before any DH or allocation
3. VerifyMAC2()           — only under load (LoadMonitor)
4. Noise handshake        — DH computations, peer lookup
5. Peer ACL check         — AllowedPeers / PeerDisabled
```

تُرجع جميع حالات الفشل خطأ موحدًا `ErrHandshakeFailed` لمنع تسريب المعلومات.

---

## 2. الحماية من هجمات حجب الخدمة (DoS)

### 2.1 MAC1 (عديم الحالة، مطلوب دائمًا)

```
key  = BLAKE2s-256("mac1" || "TunGo" || 0x01 || server_pubkey)
MAC1 = BLAKE2s-128(key, noise_msg1)
```

يتم التحقق قبل أي تخصيص حالة أو حساب DH.

### 2.2 MAC2 (ذو حالة، تحت الحمل)

```
key  = BLAKE2s-256("mac2" || "TunGo" || 0x01 || cookie_value)
MAC2 = BLAKE2s-128(key, noise_msg1 || MAC1)
```

يتم الفحص فقط عندما يكتشف `LoadMonitor` ضغطًا.

### 2.3 آلية Cookie

**قيمة Cookie** (مرتبطة بعنوان IP، مقسمة زمنيًا):
```
bucket = unix_seconds / 120
cookie = BLAKE2s-128(server_secret[32], client_ip[16] || bucket[2])
```

صالحة للفترة الحالية والسابقة (للتعامل مع الانتقالات).

**رد Cookie** (مشفر، 56 بايت):
```
[24B nonce] [16B encrypted_cookie] [16B poly1305_tag]
```

التشفير:
```
key = BLAKE2s-256("cookie" || "TunGo" || 0x01 || server_pubkey || client_ephemeral)
ciphertext = XChaCha20-Poly1305.Seal(key, nonce, cookie, aad=client_ephemeral)
```

---

## 3. تشفير النقل

### 3.1 AEAD

ChaCha20-Poly1305 مع AAD بحجم 60 بايت:

```
AAD [60 bytes]:
  [ 0..31] sessionId       (32 bytes)
  [32..47] direction        (16 bytes: "client-to-server" or "server-to-client")
  [48..59] nonce            (12 bytes)
```

يتم ملء SessionId و direction مسبقًا عند إنشاء الجلسة. يتم تحديث nonce فقط لكل حزمة.

### 3.2 بنية Nonce (12 بايت)

```
[0..7]   counterLow   (uint64, big-endian)
[8..9]   counterHigh  (uint16, big-endian)
[10..11] epoch        (uint16, big-endian)
```

- **Counter:** 80 بت أحادي الاتجاه (2^80 رسالة لكل حقبة). يُرجع خطأ عند الطفحان.
- **Epoch:** غير قابل للتغيير خلال الجلسة، يحدد جيل تجديد المفاتيح.

### 3.3 نقل TCP

```
Wire frame: [2B epoch] [ciphertext + 16B tag]
```

- حقبة مزدوجة: تتعايش الجلسة الحالية والسابقة أثناء تجديد المفاتيح.
- تنظيف تلقائي: يتم تصفير الجلسة السابقة عند أول فك تشفير بالحقبة الحالية (بفضل ضمان ترتيب TCP).
- لا حماية من إعادة التشغيل (يوفر TCP الترتيب).

### 3.4 نقل UDP

```
Wire frame: [12B nonce] [ciphertext + 16B tag]
```

- Epoch مضمّن في البايتات 10..11 من nonce.
- **الحماية من إعادة التشغيل:** خريطة بتات نافذة منزلقة بحجم 1024 بت لكل حقبة.
  - فحص مبدئي قبل فك التشفير (Check).
  - يتم التثبيت فقط بعد نجاح مصادقة AEAD (Accept).
  - يمنع تلويث النافذة بحزم غير صالحة.
- **حلقة Epoch:** قائمة FIFO ذات سعة ثابتة من الجلسات. يتم تصفير الجلسات المُخرجة.

---

## 4. تجديد المفاتيح

### 4.1 اشتقاق المفاتيح

يقوم الطرفان بتنفيذ X25519 ECDH، ثم اشتقاق مفاتيح نقل جديدة عبر HKDF-SHA256:

```
shared  = X25519(local_private, remote_public)
newC2S  = HKDF-SHA256(ikm=shared, salt=currentC2S, info="tungo-rekey-c2s")
newS2C  = HKDF-SHA256(ikm=shared, salt=currentS2C, info="tungo-rekey-s2c")
```

تعمل المفاتيح الحالية كملح HKDF، مما يوفر تسلسل السرية الأمامية.

### 4.2 حزم مستوى التحكم

```
RekeyInit:  [0xFF] [0x01] [0x02] [32B X25519 public key]   (35 bytes)
RekeyAck:   [0xFF] [0x01] [0x03] [32B X25519 public key]   (35 bytes)
```

### 4.3 آلة الحالة المحدودة لتجديد المفاتيح

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

| الحالة | الوصف |
|-------|-------------|
| **Stable** | التشغيل العادي. حقبة إرسال نشطة واحدة. |
| **Rekeying** | تم استدعاء StartRekey، حُسبت المفاتيح الجديدة، ثُبّتت الحقبة الجديدة للاستقبال. |
| **Pending** | في انتظار تأكيد النظير (أول فك تشفير ناجح بالحقبة الجديدة). |

### 4.4 تدفق تجديد المفاتيح

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

### 4.5 ثوابت الأمان

- تجديد مفاتيح واحد فقط قيد التنفيذ في أي وقت.
- تزداد قيم Epoch بشكل أحادي الاتجاه. الحد الأقصى الآمن لـ epoch: 65000 (من أصل 65535). بعد ذلك، يفرض `ErrEpochExhausted` إعادة المصافحة.
- لا تتناقص حقبة الإرسال أبدًا.
- لا تحل المفاتيح المعلقة محل المفاتيح النشطة حتى يُثبت النظير امتلاكها (عبر فك التشفير الناجح).
- يتم إلغاء تجديد المفاتيح المعلق تلقائيًا بعد 5 ثوانٍ إذا لم يتم تأكيد النظير.
- الفاصل الزمني الافتراضي لتجديد المفاتيح: 120 ثانية.

---

## 5. تصفير المفاتيح

| المادة | وقت التصفير |
|----------|-------------|
| المفاتيح الخاصة المؤقتة لـ DH | فورًا بعد حساب DH (`defer mem.ZeroBytes`) |
| الأسرار المشتركة (تجديد المفاتيح) | فورًا بعد اشتقاق المفاتيح (`defer mem.ZeroBytes`) |
| مفاتيح تجديد المفاتيح المعلقة (FSM) | عند الإلغاء أو الترقية إلى مفاتيح نشطة |
| مفاتيح الجلسة السابقة | عند أول فك تشفير بالحقبة الحالية (TCP) أو إخراج الحقبة (UDP) |
| نافذة إعادة تشغيل Nonce | عند تفكيك الجلسة (`SlidingWindow.Zeroize`) |
| مخازن AAD المؤقتة | عند تفكيك الجلسة (`DefaultUdpSession.Zeroize`) |

**قيد:** قد يقوم جامع القمامة في Go بنسخ كائنات الكومة قبل التصفير. يُعدّ `mem.ZeroBytes` دفاعًا بأقصى جهد ضد التحليل الجنائي للذاكرة، وقد تم التحقق من خلال تحليل مخرجات المُصرّف بأنه لا يتم حذفه بالتحسين (Go 1.25.7، جميع المنصات المستهدفة).

---

## 6. الثوابت

| الثابت | القيمة | الغرض |
|----------|-------|---------
| Protocol version | `0x01` | إصدار صيغة السلك |
| MAC1 / MAC2 size | 16 بايت | مخرجات BLAKE2s-128 |
| Cookie bucket | 120 ثانية | نافذة صلاحية Cookie المرتبطة بعنوان IP |
| Cookie reply size | 56 بايت | nonce (24) + encrypted cookie (16) + tag (16) |
| AAD length | 60 بايت | sessionId (32) + direction (16) + nonce (12) |
| Nonce counter | 80 بت | الرسائل لكل حقبة قبل الطفحان |
| Replay window | 1024 بت | تحمل إعادة الترتيب في UDP |
| Epoch capacity | uint16 | 65535 قيمة، عتبة الأمان 65000 |
| Rekey interval | 120 ثانية | مشغل تجديد المفاتيح الدوري الافتراضي |
| Pending timeout | 5 ثوانٍ | إلغاء تلقائي لتجديد المفاتيح غير المؤكد |
