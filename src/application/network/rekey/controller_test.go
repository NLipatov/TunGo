package rekey_test

import (
	"testing"
	"time"

	"tungo/application/network/rekey"
	"tungo/infrastructure/cryptography/chacha20"

	"golang.org/x/crypto/chacha20poly1305"
)

// newTestCrypto builds a minimal EpochUdpCrypto for tests.
func newTestCrypto(t *testing.T) *chacha20.EpochUdpCrypto {
	t.Helper()
	var sid [32]byte
	key := make([]byte, chacha20poly1305.KeySize)
	for i := range key {
		key[i] = byte(i)
	}
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	return chacha20.NewEpochUdpCrypto(sid, aead, aead, false)
}

// TestEncryptNeverFailsWhileRingNonEmpty ensures encrypt never returns error
// while at least one session exists.
func TestEncryptNeverFailsWhileRingNonEmpty(t *testing.T) {
	crypto := newTestCrypto(t)
	payload := func(s string) []byte {
		data := []byte(s)
		buf := make([]byte, chacha20poly1305.NonceSize+len(data), chacha20poly1305.NonceSize+len(data)+chacha20poly1305.Overhead)
		copy(buf[chacha20poly1305.NonceSize:], data)
		return buf
	}
	for i := 0; i < 5; i++ {
		if _, err := crypto.Encrypt(payload("hello")); err != nil {
			t.Fatalf("encrypt failed at %d: %v", i, err)
		}
	}
	if _, err := crypto.Rekey(make([]byte, chacha20poly1305.KeySize), make([]byte, chacha20poly1305.KeySize)); err != nil {
		t.Fatalf("rekey: %v", err)
	}
	for i := 0; i < 5; i++ {
		if _, err := crypto.Encrypt(payload("world")); err != nil {
			t.Fatalf("encrypt after rekey failed at %d: %v", i, err)
		}
	}
	if crypto.RemoveEpoch(0) {
		t.Fatal("should not remove active send epoch")
	}
	if _, err := crypto.Encrypt(payload("still")); err != nil {
		t.Fatalf("encrypt after failed remove: %v", err)
	}
}

// TestSinglePendingRekey enforces at most one in-flight rekey and idempotent duplicate.
func TestSinglePendingRekey(t *testing.T) {
	crypto := newTestCrypto(t)
	ctrl := rekey.NewController(crypto, []byte("c2s"), []byte("s2c"), false)
	ctrl.SetPendingTimeout(50 * time.Millisecond)

	epoch1, err := ctrl.RekeyAndApply(make([]byte, chacha20poly1305.KeySize), make([]byte, chacha20poly1305.KeySize))
	if err != nil {
		t.Fatalf("first rekey: %v", err)
	}
	if ctrl.State() != rekey.StatePending {
		t.Fatalf("expected pending, got %v", ctrl.State())
	}
	if _, err := ctrl.RekeyAndApply(make([]byte, chacha20poly1305.KeySize), make([]byte, chacha20poly1305.KeySize)); err == nil {
		t.Fatal("second rekey should fail when pending")
	}
	if p, ok := ctrl.PendingEpoch(); !ok || p != epoch1 {
		t.Fatalf("pending epoch changed: %v != %d", p, epoch1)
	}
	// Confirm resolves to stable.
	ctrl.ConfirmSendEpoch(epoch1)
	if ctrl.State() != rekey.StateStable {
		t.Fatalf("expected stable after confirm, got %v", ctrl.State())
	}
}

// TestDuplicateAckNoAdvance verifies duplicate Ack (rekey) does not advance epoch+2.
func TestDuplicateAckNoAdvance(t *testing.T) {
	crypto := newTestCrypto(t)
	ctrl := rekey.NewController(crypto, []byte("c2s"), []byte("s2c"), false)

	epoch1, err := ctrl.RekeyAndApply(make([]byte, chacha20poly1305.KeySize), make([]byte, chacha20poly1305.KeySize))
	if err != nil {
		t.Fatalf("rekey: %v", err)
	}
	// Duplicate attempt should fail and not change epoch counter.
	if _, err := ctrl.RekeyAndApply(make([]byte, chacha20poly1305.KeySize), make([]byte, chacha20poly1305.KeySize)); err == nil {
		t.Fatal("duplicate rekey should fail")
	}
	// Confirm then check epoch advanced once.
	ctrl.ConfirmSendEpoch(epoch1)
	if ctrl.LastRekeyEpoch != epoch1 {
		t.Fatalf("LastRekeyEpoch should be %d after confirm, got %d", epoch1, ctrl.LastRekeyEpoch)
	}
}

// TestPendingResolvesByAbort checks that timeout aborts pending and returns to Stable.
func TestPendingResolvesByAbort(t *testing.T) {
	crypto := newTestCrypto(t)
	ctrl := rekey.NewController(crypto, []byte("c2s"), []byte("s2c"), false)
	ctrl.SetPendingTimeout(5 * time.Millisecond)

	if _, err := ctrl.RekeyAndApply(make([]byte, chacha20poly1305.KeySize), make([]byte, chacha20poly1305.KeySize)); err != nil {
		t.Fatalf("rekey: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	ctrl.MaybeAbortPending(time.Now())
	if ctrl.State() != rekey.StateStable {
		t.Fatalf("expected stable after abort, got %v", ctrl.State())
	}
	if _, ok := ctrl.PendingEpoch(); ok {
		t.Fatal("pendingSend not cleared after abort")
	}
}

// TestSendEpochNeverEvicted ensures send epoch remains resolvable.
func TestSendEpochNeverEvicted(t *testing.T) {
	crypto := newTestCrypto(t)
	payload := func(s string) []byte {
		data := []byte(s)
		buf := make([]byte, chacha20poly1305.NonceSize+len(data), chacha20poly1305.NonceSize+len(data)+chacha20poly1305.Overhead)
		copy(buf[chacha20poly1305.NonceSize:], data)
		return buf
	}
	if _, err := crypto.Encrypt(payload("ping")); err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if crypto.RemoveEpoch(0) {
		t.Fatal("should not remove active send epoch")
	}
	if _, err := crypto.Encrypt(payload("pong")); err != nil {
		t.Fatalf("encrypt after remove attempt failed: %v", err)
	}
}
