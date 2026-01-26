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

	if _, err := NewUdpSession(id, short, ok, false); err == nil {
		t.Fatal("expected error for invalid sendKey length")
	}
	if _, err := NewUdpSession(id, ok, short, false); err == nil {
		t.Fatal("expected error for invalid recvKey length")
	}
}

func TestUdpEncrypt_InPlaceCapacityError(t *testing.T) {
	id := randID()
	key := randKey()
	sess, err := NewUdpSession(id, key, key, false)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	// Need cap >= len + Overhead; here cap == len, should error.
	plain := make([]byte, 13+8) // 1 key byte + 12 nonce + 8 payload
	if _, err := sess.Encrypt(plain); err == nil {
		t.Fatal("expected insufficient capacity error")
	}
}

func TestUdpEncrypt_BufferTooShort(t *testing.T) {
	id := randID()
	key := randKey()
	sess, err := NewUdpSession(id, key, key, false)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	// Less than 12 bytes total → should error "buffer too short"
	tooShort := make([]byte, 12, 12+chacha20poly1305.Overhead)
	if _, err := sess.Encrypt(tooShort); err == nil {
		t.Fatal("expected buffer too short error")
	}
}

func TestUdpEncrypt_Success_LengthAndLayout(t *testing.T) {
	id := randID()
	key := randKey()
	sess, err := NewUdpSession(id, key, key, false)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	payload := []byte("hello-udp")
	buf := make([]byte, 13+len(payload), 13+len(payload)+chacha20poly1305.Overhead)
	copy(buf[13:], payload)

	out, err := sess.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	wantLen := 13 + len(payload) + chacha20poly1305.Overhead
	if len(out) != wantLen {
		t.Fatalf("cipher len=%d; want %d", len(out), wantLen)
	}
	// Header (key + nonce) should be non-zero with very high probability
	if bytes.Equal(out[:13], make([]byte, 13)) {
		t.Fatal("nonce appears to be all zeros (unexpected)")
	}
}

func TestUdpDecrypt_TooShort(t *testing.T) {
	id := randID()
	key := randKey()
	sess, err := NewUdpSession(id, key, key, true)
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

	cli, err := NewUdpSession(idClient, key, key, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	srv, err := NewUdpSession(idServer, key, key, true)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	payload := []byte("payload-udp")
	buf := make([]byte, 13+len(payload), 13+len(payload)+chacha20poly1305.Overhead)
	copy(buf[13:], payload)

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
	cli, err := NewUdpSession(id, key, key, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	srv, err := NewUdpSession(id, key, key, true)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	payload := []byte("once-only")
	buf := make([]byte, 13+len(payload), 13+len(payload)+chacha20poly1305.Overhead)
	copy(buf[13:], payload)

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

	cli, err := NewUdpSession(id, key, key, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	srv, err := NewUdpSession(id, key, key, true)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	payload := []byte("round-trip-ok")
	buf := make([]byte, 13+len(payload), 13+len(payload)+chacha20poly1305.Overhead)
	copy(buf[13:], payload)

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
	id := randID()
	s := &DefaultUdpSession{SessionId: id}

	header := make([]byte, 1+chacha20poly1305.NonceSize)
	header[0] = 0xAB
	for i := 1; i < len(header); i++ {
		header[i] = byte(i)
	}

	// Client->Server
	aadC2S := s.CreateAAD(false, header, make([]byte, aadLength))
	if len(aadC2S) != aadLength {
		t.Fatalf("aad len=%d, want %d", len(aadC2S), aadLength)
	}
	if !bytes.Equal(aadC2S[:sessionIdentifierLength], id[:]) {
		t.Fatal("session id part mismatch (C2S)")
	}
	if !bytes.Equal(aadC2S[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirC2S[:]) {
		t.Fatal("direction bytes mismatch (C2S)")
	}
	if aadC2S[sessionIdentifierLength+directionLength] != header[0] {
		t.Fatal("key id mismatch (C2S)")
	}
	if !bytes.Equal(aadC2S[sessionIdentifierLength+directionLength+1:aadLength], header[1:]) {
		t.Fatal("nonce bytes mismatch (C2S)")
	}

	// Server->Client
	aadS2C := s.CreateAAD(true, header, make([]byte, aadLength))
	if len(aadS2C) != aadLength {
		t.Fatalf("aad len=%d, want %d", len(aadS2C), aadLength)
	}
	if !bytes.Equal(aadS2C[:sessionIdentifierLength], id[:]) {
		t.Fatal("session id part mismatch (S2C)")
	}
	if !bytes.Equal(aadS2C[sessionIdentifierLength:sessionIdentifierLength+directionLength], dirS2C[:]) {
		t.Fatal("direction bytes mismatch (S2C)")
	}
	if aadS2C[sessionIdentifierLength+directionLength] != header[0] {
		t.Fatal("key id mismatch (S2C)")
	}
	if !bytes.Equal(aadS2C[sessionIdentifierLength+directionLength+1:aadLength], header[1:]) {
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
	sess, err := NewUdpSession(id, key, key, false)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	// Force overflow: high=max, low=max ⇒ incrementNonce() must return an error.
	sess.current.nonce.high = ^uint32(0)
	sess.current.nonce.low = ^uint64(0)

	// Must satisfy pre-checks: len >= 12 and cap >= len + Overhead.
	buf := make([]byte, 1+chacha20poly1305.NonceSize, 1+chacha20poly1305.NonceSize+chacha20poly1305.Overhead)

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
	sess, err := NewUdpSession(id, key, key, false)
	if err != nil {
		t.Fatalf("NewUdpSession: %v", err)
	}

	// Set state to "just before rollover".
	const startHigh = uint32(123)
	sess.current.nonce.high = startHigh
	sess.current.nonce.low = ^uint64(0)

	// Empty payload (only 12-byte nonce header). Cap allows in-place tag append.
	buf := make([]byte, 1+chacha20poly1305.NonceSize, 1+chacha20poly1305.NonceSize+chacha20poly1305.Overhead)

	out, err := sess.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt (rollover): %v", err)
	}

	// Decode nonce: [1..8]=low (uint64 BE), [9..12]=high (uint32 BE).
	encLow := binary.BigEndian.Uint64(out[1:9])
	encHigh := binary.BigEndian.Uint32(out[9:13])

	if encLow != 0 {
		t.Fatalf("nonce.low after rollover = %d; want 0", encLow)
	}
	if encHigh != startHigh+1 {
		t.Fatalf("nonce.high after rollover = %d; want %d", encHigh, startHigh+1)
	}
}
