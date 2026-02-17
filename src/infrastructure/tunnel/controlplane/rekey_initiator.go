package controlplane

import (
	"time"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/service_packet"

	"golang.org/x/crypto/curve25519"
)

// RekeyInitScheduler decides when to emit client-side RekeyInit packets and builds their plaintext payload.
//
// It is control-plane only: it does not encrypt and does not perform any transport/TUN IO.
// The caller must provide a destination buffer to avoid allocations.
type RekeyInitScheduler struct {
	crypto   primitives.KeyDeriver
	interval time.Duration
	rotateAt time.Time
}

func NewRekeyInitScheduler(crypto primitives.KeyDeriver, interval time.Duration, now time.Time) *RekeyInitScheduler {
	return &RekeyInitScheduler{
		crypto:   crypto,
		interval: interval,
		rotateAt: now.Add(interval),
	}
}

func (s *RekeyInitScheduler) Interval() time.Duration { return s.interval }
func (s *RekeyInitScheduler) RotateAt() time.Time     { return s.rotateAt }
func (s *RekeyInitScheduler) SetInterval(d time.Duration) {
	s.interval = d
}
func (s *RekeyInitScheduler) SetRotateAt(t time.Time) {
	s.rotateAt = t
}

// MaybeBuildRekeyInit returns a v1 service_packet.RekeyInit plaintext payload in dst.
// ok=false means "do nothing".
func (s *RekeyInitScheduler) MaybeBuildRekeyInit(
	now time.Time,
	fsm *rekey.StateMachine,
	dst []byte,
) (payload []byte, ok bool, err error) {
	if s.crypto == nil || fsm == nil {
		return nil, false, nil
	}
	if now.Before(s.rotateAt) {
		return nil, false, nil
	}
	// We are due; schedule next attempt regardless of outcome.
	s.rotateAt = now.Add(s.interval)
	if fsm.State() != rekey.StateStable {
		// Avoid overwriting pending priv or spamming in-flight rekeys.
		return nil, false, nil
	}
	if len(dst) < service_packet.RekeyPacketLen {
		return nil, false, nil
	}

	var (
		publicKey []byte
		keyErr    error
	)
	if pendingPriv, ok := fsm.PendingRekeyPrivateKey(); ok {
		// Reuse the in-flight key to avoid mismatched ACKs.
		publicKey, keyErr = curve25519.X25519(pendingPriv[:], curve25519.Basepoint)
	} else {
		var pendingPriv [32]byte
		publicKey, pendingPriv, keyErr = s.crypto.GenerateX25519KeyPair()
		if keyErr == nil {
			fsm.SetPendingRekeyPrivateKey(pendingPriv)
		}
	}
	if keyErr != nil {
		return nil, false, keyErr
	}
	if len(publicKey) != service_packet.RekeyPublicKeyLen {
		return nil, false, nil
	}

	copy(dst[3:], publicKey)
	servicePayload, encErr := service_packet.EncodeV1Header(service_packet.RekeyInit, dst)
	if encErr != nil {
		return nil, false, encErr
	}
	return servicePayload, true, nil
}
