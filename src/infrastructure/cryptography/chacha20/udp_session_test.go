package chacha20

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestNewUdpSession_KeyLengthError(t *testing.T) {
	var id [32]byte
	short := []byte("short")
	ok := randKey()

	if _, err := NewUdpSession(id, short, ok, false, 0); err == nil {
		t.Fatal("expected error for invalid sendKey length")
	}
	if _, err := NewUdpSession(id, ok, short, false, 0); err == nil {
		t.Fatal("expected error for invalid recvKey length")
	}
}

func TestUdpEncrypt_InPlaceCapacityError(t *testing.T) {
	id := randID()
	key := randKey()
	sess, err := NewUdpSession(id, key, key, false, 0)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	// Need cap >= len + Overhead; here cap == len, should error.
	plain := make([]byte, 12+8) // 12 bytes header space + 8 bytes payload
	if _, err := sess.Encrypt(plain); err == nil {
		t.Fatal("expected insufficient capacity error")
	}
}

func TestUdpEncrypt_BufferTooShort(t *testing.T) {
	id := randID()
	key := randKey()
	sess, err := NewUdpSession(id, key, key, false, 0)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	// Less than 12 bytes total → should error "buffer too short"
	tooShort := make([]byte, 11, 11+chacha20poly1305.Overhead)
	if _, err := sess.Encrypt(tooShort); err == nil {
		t.Fatal("expected buffer too short error")
	}
}

func TestUdpEncrypt_Success_LengthAndLayout(t *testing.T) {
	id := randID()
	key := randKey()
	sess, err := NewUdpSession(id, key, key, false, 0)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	payload := []byte("hello-udp")
	buf := make([]byte, 12+len(payload), 12+len(payload)+chacha20poly1305.Overhead)
	copy(buf[12:], payload)

	out, err := sess.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	wantLen := 12 + len(payload) + chacha20poly1305.Overhead
	if len(out) != wantLen {
		t.Fatalf("cipher len=%d; want %d", len(out), wantLen)
	}
	// Nonce (first 12 bytes) should be non-zero with very high probability
	if bytes.Equal(out[:12], make([]byte, 12)) {
		t.Fatal("nonce appears to be all zeros (unexpected)")
	}
}

func TestUdpDecrypt_TooShort(t *testing.T) {
	id := randID()
	key := randKey()
	sess, err := NewUdpSession(id, key, key, true, 0)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	if _, err := sess.Decrypt([]byte("short")); err == nil {
		t.Fatal("expected 'too short' error")
	}
}

func TestUdp_Decrypt_OpenFail_AADMismatch(t *testing.T) {
	idClient := randID()
	idServer := randID() // different → AAD mismatch
	key := randKey()

	cli, err := NewUdpSession(idClient, key, key, false, 0)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	srv, err := NewUdpSession(idServer, key, key, true, 0)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	payload := []byte("payload-udp")
	buf := make([]byte, 12+len(payload), 12+len(payload)+chacha20poly1305.Overhead)
	copy(buf[12:], payload)

	ct, err := cli.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := srv.Decrypt(ct); err == nil {
		t.Fatal("expected decrypt error due to AAD mismatch (different SessionId)")
	}
}

func TestUdpDecrypt_ReplayRejected_ByNonceValidator(t *testing.T) {
	id := randID()
	key := randKey()

	// client (encrypt) → server (decrypt)
	cli, err := NewUdpSession(id, key, key, false, 0)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	srv, err := NewUdpSession(id, key, key, true, 0)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	payload := []byte("once-only")
	buf := make([]byte, 12+len(payload), 12+len(payload)+chacha20poly1305.Overhead)
	copy(buf[12:], payload)

	ct, err := cli.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// First decrypt succeeds
	if _, err := srv.Decrypt(ct); err != nil {
		t.Fatalf("first decrypt failed: %v", err)
	}
	// Replay must be rejected by Sliding64 validator (nonce reuse)
	if _, err := srv.Decrypt(ct); err == nil {
		t.Fatal("expected nonce validator error on replay, got nil")
	}
}

func TestUdp_RoundTrip_OK(t *testing.T) {
	id := randID()
	key := randKey()

	cli, err := NewUdpSession(id, key, key, false, 0)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	srv, err := NewUdpSession(id, key, key, true, 0)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	payload := []byte("round-trip-ok")
	buf := make([]byte, 12+len(payload), 12+len(payload)+chacha20poly1305.Overhead)
	copy(buf[12:], payload)

	ct, err := cli.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	pt, err := srv.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(pt, payload) {
		t.Fatalf("plaintext mismatch: got %q want %q", pt, payload)
	}
}

func TestUdp_CreateAAD_BothDirections(t *testing.T) {
	// Use constructor to ensure AAD buffers are pre-filled correctly.
	id := randID()
	key := randKey()

	// Test client session (encrypts C2S, decrypts S2C)
	clientSession, err := NewUdpSession(id, key, key, false, 0)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	// Test server session (encrypts S2C, decrypts C2S)
	serverSession, err := NewUdpSession(id, key, key, true, 0)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	nonce := make([]byte, chacha20poly1305.NonceSize)
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}

	// Client encrypts C2S - uses encryptionAadBuf
	aadC2S := clientSession.CreateAAD(false, nonce, clientSession.encryptionAadBuf[:])
	if len(aadC2S) != aadLength {
		t.Fatalf("aad len=%d, want %d", len(aadC2S), aadLength)
	}
	if !bytes.Equal(aadC2S[:sessionIdentifierLength], id[:]) {
		t.Fatal("session id part mismatch (C2S)")
	}
	if !bytes.Equal(aadC2S[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:]) {
		t.Fatal("direction bytes mismatch (C2S)")
	}
	if !bytes.Equal(aadC2S[sessionIdentifierLength+directionLength:aadLength], nonce) {
		t.Fatal("nonce bytes mismatch (C2S)")
	}

	// Server encrypts S2C - uses encryptionAadBuf
	aadS2C := serverSession.CreateAAD(true, nonce, serverSession.encryptionAadBuf[:])
	if len(aadS2C) != aadLength {
		t.Fatalf("aad len=%d, want %d", len(aadS2C), aadLength)
	}
	if !bytes.Equal(aadS2C[:sessionIdentifierLength], id[:]) {
		t.Fatal("session id part mismatch (S2C)")
	}
	if !bytes.Equal(aadS2C[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:]) {
		t.Fatal("direction bytes mismatch (S2C)")
	}
	if !bytes.Equal(aadS2C[sessionIdentifierLength+directionLength:aadLength], nonce) {
		t.Fatal("nonce bytes mismatch (S2C)")
	}

	// Direction segments must differ
	if bytes.Equal(
		aadC2S[sessionIdentifierLength:sessionIdentifierLength+directionLength],
		aadS2C[sessionIdentifierLength:sessionIdentifierLength+directionLength],
	) {
		t.Fatal("C2S and S2C direction segments must differ")
	}
}

func TestUdpEncrypt_ErrOnNonceOverflow(t *testing.T) {
	id := randID()
	key := randKey()
	sess, err := NewUdpSession(id, key, key, false, 0)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	// Force overflow: high=max, low=max ⇒ incrementNonce() must return an error.
	sess.nonce.counterHigh = ^uint16(0)
	sess.nonce.counterLow = ^uint64(0)

	// Must satisfy pre-checks: len >= 12 and cap >= len + Overhead.
	buf := make([]byte, chacha20poly1305.NonceSize, chacha20poly1305.NonceSize+chacha20poly1305.Overhead)

	_, err = sess.Encrypt(buf)
	if err == nil {
		t.Fatal("expected nonce overflow error, got nil")
	}
	if !strings.Contains(err.Error(), "nonce overflow") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUdpEncrypt_NonceRollover_WritesCorrectNonce(t *testing.T) {
	id := randID()
	key := randKey()
	sess, err := NewUdpSession(id, key, key, false, 0)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	// Set state to "just before rollover".
	const startHigh = uint16(123)
	sess.nonce.counterHigh = startHigh
	sess.nonce.counterLow = ^uint64(0)

	// Empty payload (only 12-byte nonce header). Cap allows in-place tag append.
	buf := make([]byte, chacha20poly1305.NonceSize, chacha20poly1305.NonceSize+chacha20poly1305.Overhead)

	out, err := sess.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt (rollover): %v", err)
	}

	// Decode nonce: [0..7]=low, [8..9]=high, [10..11]=epoch.
	encLow := binary.BigEndian.Uint64(out[0:8])
	encHigh := binary.BigEndian.Uint16(out[8:10])

	if encLow != 0 {
		t.Fatalf("nonce.low after rollover = %d; want 0", encLow)
	}
	if encHigh != startHigh+1 {
		t.Fatalf("nonce.high after rollover = %d; want %d", encHigh, startHigh+1)
	}
}
