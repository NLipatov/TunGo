# IK + Per-Client PSK + AllowedIPs Migration Plan (TunGo)

Status: Draft plan for implementation (no code changes yet)

Goal:
- Replace Noise XX with IK.
- Use per-client PSK.
- Enforce AllowedIPs (CIDR list) per client.
- Enable revocation by removing a client entry from server config.
- Preserve high security, privacy, and operational safety.

Non-Goals:
- No PKI or cert-based identity.
- No client auto-provisioning beyond existing config flow.

---

## 1) Target Security Model and Invariants

### Identities and Trust
- Server identity: X25519 static key pair already in server config.
- Client identity: new X25519 static key pair stored in client config.
- Trust anchor: server config is authoritative for the allowed client public keys and their AllowedIPs and PSK.

### Cryptographic Invariants
- Noise pattern: IK with CipherSuite(DH25519, ChaChaPoly, SHA256).
- Initiator (client) knows responder (server) static public key in advance.
- Responder (server) validates initiator static key at the earliest possible point (msg1).
- Per-client PSK is mixed into the handshake (Noise IKpsk or IK with SetPresharedKey).
- Session keys are derived only after successful authentication and PSK verification.

### Authorization Invariants
- A client is authorized if and only if:
  - Its static public key is present in server AllowedClients, AND
  - Its per-client PSK matches, AND
  - The allowed IPs for this client are present in the server configuration.
- Any handshake from an unknown client key is rejected.
- Any data-plane packet whose source IP is not within AllowedIPs for that client is dropped.

### Revocation Invariants
- Revocation is immediate when the server config removes or disables a client entry.
- Revoked client cannot successfully complete a handshake.
- Existing session for revoked client is terminated or rekeyed into failure on next control-plane activity.

### Privacy Constraints
- Client identity is revealed in msg1 (IK property). This is accepted for the operational model.
- No sensitive secrets (private keys, PSKs) are logged or emitted via metrics.

---

## 2) Configuration Schema Changes

### Client Configuration (existing file)
Add fields (names can be adjusted to match project conventions):
- ClientX25519PublicKey: []byte (base64 in JSON)
- ClientX25519PrivateKey: []byte (base64 in JSON)
- ClientPSK: []byte (32 bytes, base64 in JSON)

### Server Configuration (existing file)
Add field:
- AllowedClients: array of objects:
  - ClientPublicKey: []byte (base64 in JSON)
  - AllowedIPs: []string (CIDR list, e.g., ["10.0.1.4/32", "10.0.1.0/24"])
  - ClientPSK: []byte (32 bytes, base64 in JSON)
  - Enabled: bool (optional, for soft-revoke)

Validation rules:
- All keys must be 32 bytes for X25519 and PSK.
- AllowedIPs must be valid CIDR; no overlap between different clients unless explicitly allowed.
- If Enabled is false, client must be treated as revoked.

Migration plan:
- If new fields are missing, fail fast with explicit error message.
- Provide a one-time config migration helper (optional).

---

## 3) Protocol and Handshake Changes

### Noise Pattern
- Change pattern from XX to IK.
- Use per-client PSK:
  - Use IKpsk or SetPresharedKey per handshake.
  - Ensure PSK is unique per client.

### Handshake Payload
- Remove client-sent IP from handshake payload (preferred).
- If payload is retained, server must verify it against AllowedIPs, and must not trust it as authoritative.

### Authentication Flow (Server)
1. Read msg1.
2. Extract client static public key from handshake state.
3. Lookup client in AllowedClients.
4. If not found or disabled, abort handshake immediately.
5. Set PSK for handshake from client record.
6. Continue msg2 and finish handshake.
7. Bind session to client identity and AllowedIPs.

### Authentication Flow (Client)
1. Use client static keypair from config.
2. Set PSK for handshake.
3. Use server static public key from config.
4. Execute IK initiator handshake.

---

## 4) Data-Plane Authorization (AllowedIPs)

- Associate each session with AllowedIPs from its client record.
- Enforce AllowedIPs on server ingress:
  - Drop any packet where inner source IP is not in AllowedIPs.
- Enforce AllowedIPs on server egress (optional, defense-in-depth).
- Consider the same check on client side to prevent local misconfiguration.

---

## 5) DoS and Abuse Controls

- Rate limit handshake attempts per source IP and per client key.
- Short-circuit unknown keys at msg1 (IK allows early reject).
- Limit registration queue size and concurrent handshakes.
- Optional: add cookie-like stateless challenge for UDP to mitigate spoofing.

---

## 6) Observability and Logging

- Log: client public key fingerprint (short hash), not raw key.
- Log: rejection reasons without leaking secrets.
- Metrics: handshake success/failure counts, auth failures, AllowedIPs violations.
- Ensure logs never include PSK or private keys.

---

## 7) Testing Strategy

### Unit Tests
- IK handshake success (client/server) with PSK.
- Reject unknown client key.
- Reject incorrect PSK.
- AllowedIPs mismatch causes rejection or drop.
- Ensure key material is zeroed where intended.

### Integration Tests
- End-to-end tunnel up with IK+PSK.
- Revocation: remove client from server config, client fails to connect.
- AllowedIPs enforcement: client spoofed IP is dropped.

### Security Tests
- Replay attempts across sessions should fail.
- Handshake under load and rate limits behave as expected.

---

## 8) Rollout Plan

1. Add config fields and validation to client and server.
2. Implement IK+PSK handshake while keeping XX in a feature flag if necessary.
3. Add AllowedIPs enforcement in server dataplane.
4. Add migration tooling or scripts (if required).
5. Deploy to staging, run integration tests.
6. Roll out to production with monitoring.

---

# STRICT IMPLEMENTATION CHECKLIST (Production-Grade)

Use this checklist as a hard gate. Do not ship unless every item is explicitly verified.

## A) Cryptographic Correctness
- [ ] Noise pattern is IK (or IKpsk) and correctly configured.
- [ ] Client uses static keypair from config (not ephemeral).
- [ ] Server static keypair is loaded and validated (32 bytes).
- [ ] Per-client PSK is set in handshake (32 bytes) and verified.
- [ ] Handshake aborts on any mismatch or missing client entry.
- [ ] Keys derived are used only after successful handshake.
- [ ] Key direction mapping is correct (c2s, s2c).
- [ ] All secret material is zeroized when no longer needed.

## B) Identity and Authorization
- [ ] Server rejects unknown client public keys at msg1.
- [ ] Server rejects disabled/revoked clients.
- [ ] AllowedIPs are enforced on server ingress.
- [ ] AllowedIPs are validated for format and policy conflicts.
- [ ] Client IP is never trusted from handshake payload.

## C) Revocation Semantics
- [ ] Removing client entry prevents new handshakes.
- [ ] Existing sessions for revoked clients are terminated or forced to re-handshake and fail.
- [ ] No fallback path allows revoked clients to remain connected.

## D) Configuration Hygiene
- [ ] All key and PSK fields are base64 encoded in JSON and validated.
- [ ] Invalid length keys/PSKs cause hard errors at startup.
- [ ] Missing required fields cause explicit, actionable errors.
- [ ] Config migration notes are documented.

## E) DoS and Abuse Controls
- [ ] Handshake attempts are rate-limited per IP.
- [ ] Handshake concurrency is bounded.
- [ ] Unknown clients are rejected with minimal CPU effort.
- [ ] UDP spoofing is mitigated (cookie or equivalent) if exposed.

## F) Logging and Privacy
- [ ] Logs never include private keys or PSK.
- [ ] Only public key fingerprints are logged.
- [ ] Auth failures log reason without leaking sensitive info.

## G) Testing and Verification
- [ ] Unit tests cover IK+PSK success and failure paths.
- [ ] AllowedIPs enforcement is tested with spoofed IPs.
- [ ] Revocation is tested.
- [ ] Regression tests for existing protocols (TCP/UDP/WS/WSS) pass.

## H) Operational Readiness
- [ ] Staging rollout validated with monitoring on handshake failures.
- [ ] Metrics dashboards include auth failures and AllowedIPs drops.
- [ ] Runbook updated for key/PSK rotation and revocation.

---

## Next Steps
- Confirm config schema names and storage format.
- Decide if you want feature-flagged rollout or hard switch.
- Implement and validate per this checklist.
