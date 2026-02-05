# ADR-001: Migration to Noise IK Handshake (No PSK)

## Status
**Accepted / Spec-Locked**

## Date
2026-02-05

## Context

TunGo historically used a custom handshake and later evaluated Noise-based patterns.
As the protocol matured, several requirements became explicit:

- Cryptographically sound mutual authentication
- Clear separation of **identity**, **authorization**, and **routing**
- Strong replay protection for both handshake and data plane
- Robust DoS resistance under unauthenticated load
- Explicit and reviewable security invariants
- Avoidance of undocumented or implicit guarantees

WireGuard’s design was used as a reference point, but not blindly adopted.
In particular, we aimed to:
- Preserve WireGuard-grade security
- Remove unnecessary complexity
- Make all trust assumptions explicit

This ADR documents the decision to migrate to **Noise IK without PSK**, with an explicit
`AllowedPeers` authorization model and mandatory DoS protection.

---

## Decision

TunGo SHALL use the following handshake and trust model:

- **Handshake pattern**: `Noise_IK_25519_ChaChaPoly_SHA256`
- **No PSK** is used
- **Exactly two handshake messages**
- **Mandatory MAC1/MAC2 + cookie-based DoS protection**
- **Explicit server-side AllowedPeers model**
- **Strict UDP session binding**
- **Explicit protocol versioning**

This decision is fully specified in  
`PROTOCOL SPECIFICATION: Noise IK Migration (Spec-Locked), Revision 1.0`.

---

## Rationale

### 1. Noise IK (without PSK)

Noise IK provides:
- Mutual authentication via static key possession
- Forward secrecy via ephemeral DH
- A minimal, well-analyzed handshake

PSK was explicitly rejected because:
- PSK is **not an authentication mechanism**
- Dynamic PSK lookup after `ReadMessage` violates the Noise specification
- All required revocation and authorization semantics are already covered by `AllowedPeers`

Authentication is performed **only** via static X25519 keys.

---

### 2. Identity vs Authorization Separation

The protocol enforces a strict separation:

- **Identity**: proven cryptographically via static public key
- **Authorization**: defined exclusively by server-side `AllowedPeers`
- **IP addresses**: treated as *claims*, never identity

This prevents:
- IP-based identity confusion
- Session hijacking via spoofed inner packets
- Ambiguous routing decisions

---

### 3. AllowedPeers Model

Authorization is defined by an explicit server configuration:

```go
type AllowedPeer struct {
    PublicKey  []byte
    Enabled    bool
    ClientIP   string
    AllowedIPs []string
}
```

Key properties:

* Server configuration is the **sole source of truth**
* Client configuration is non-authoritative
* Overlapping `AllowedIPs` are rejected at configuration load time
* Revocation is immediate by toggling `Enabled = false`

This model mirrors WireGuard semantics but makes them explicit and verifiable.

---

### 4. DoS Protection (Mandatory)

Unauthenticated handshake load is mitigated via:

* **MAC1 verification before any allocation or DH**
* **Cookie-based challenge-response under load**
* Stateless cookie validation
* Cookie bound to:

  * Client source IP
  * Timestamp bucket
  * Client ephemeral (from msg1)

A critical invariant is enforced:

> Client ephemeral keys MUST be extracted **only after MAC1 verification**
> and strictly according to the Noise IK message format.

This prevents cookie-oracle and parsing attacks.

---

### 5. Replay Protection

#### Handshake replay

* Replaying msg1 produces a fresh server ephemeral
* Derived session keys differ
* Replay is harmless and unusable to the attacker

#### Data-plane replay

* Monotonic counters
* Sliding replay window
* Epoch-based rekeying
* Duplicate or old packets are silently dropped

---

### 6. UDP Session Binding

For UDP transport:

* Each session is bound to the exact `(IP, port)` observed during handshake
* Silent rebinding is forbidden
* NAT rebinding requires a new handshake

This prevents:

* Source-IP spoofing
* Session hijacking via packet injection

---

### 7. Protocol Versioning

* A 1-byte version prefix is included
* Legacy patterns (Noise XX) are explicitly rejected
* No silent downgrade is allowed

---

## Alternatives Considered

### A. Noise IK with PSK

**Rejected**

* PSK does not provide authentication
* Dynamic PSK selection violates Noise semantics
* Adds complexity without meaningful security gain

---

### B. Noise XX

**Rejected**

* Does not authenticate the server to the client early
* Allows unnecessary identity ambiguity
* Not aligned with TunGo’s trust model

---

### C. IP-Based Identity

**Rejected**

* Inner IPs are attacker-controlled claims
* Leads to spoofing and session confusion
* Violates cryptographic identity principles

---

### D. Optional DoS Protection

**Rejected**

* Exposes server to trivial handshake floods
* Cookie mechanism is mandatory for production security

---

## Consequences

### Positive

* WireGuard-grade security with clearer invariants
* Explicit, auditable trust model
* Strong resistance to replay, spoofing, and DoS attacks
* Clear separation of concerns in implementation

### Negative / Limitations

* Go garbage collector prevents perfect key erasure
* Timing uniformity is best-effort only
* NAT rebinding requires re-authentication

These limitations are documented and accepted.

---

## Security Guarantees

The protocol **guarantees**:

* Mutual authentication
* Forward secrecy
* Replay protection (handshake and data)
* DoS resistance
* Authorization correctness
* UDP session binding

The protocol **does NOT guarantee**:

* Client anonymity
* Timing-based enumeration resistance
* Perfect in-memory key erasure (Go limitation)

---

## References

* Noise Protocol Framework Specification
* WireGuard Protocol and Implementation
* TunGo Protocol Specification (Revision 1.0)

---

## Decision

This ADR is **final**.

Any deviation from this specification MUST:

* Introduce a new ADR
* Explicitly document new trust assumptions
* Undergo security review

```

