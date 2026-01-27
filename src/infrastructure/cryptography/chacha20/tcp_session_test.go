package chacha20

import (
	"bytes"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func randKey() []byte {
	k := make([]byte, chacha20poly1305.KeySize)
	_, _ = rand.Read(k)
	return k
}

func randID() [32]byte {
	var id [32]byte
	_, _ = rand.Read(id[:])
	return id
}

func TestNewTcpCryptographyService_KeyLen(t *testing.T) {
	id := randID()
	short := make([]byte, 5)
	long := randKey()

	if _, err := NewTcpCryptographyService(id, short, long, false); err == nil {
		t.Fatal("expected error for short sendKey")
	}
	if _, err := NewTcpCryptographyService(id, long, short, false); err == nil {
		t.Fatal("expected error for short recvKey")
	}
}

func TestTcpSession_Encrypt_InPlaceCapacityError(t *testing.T) {
	id := randID()
	key := randKey()

	s, err := NewTcpCryptographyService(id, key, key, false)
	if err != nil {
		t.Fatalf("NewTcpCryptographyService: %v", err)
	}

	// Not enough cap for in-place encryption (need +Overhead)
	msg := make([]byte, 32) // len=32, cap=32
	if _, err := s.Encrypt(msg); err == nil {
		t.Fatal("expected insufficient capacity error")
	}
}

func TestTcpSession_RoundTrip_And_Replay(t *testing.T) {
	id := randID()
	key := randKey()

	// client -> server
	client, err := NewTcpCryptographyService(id, key, key, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	server, err := NewTcpCryptographyService(id, key, key, true)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	// Grow backing array to allow in-place encryption
	msg := []byte("secret payload")
	buf := make([]byte, len(msg), len(msg)+chacha20poly1305.Overhead)
	copy(buf, msg)

	ct1, err := client.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	pt, err := server.Decrypt(ct1)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(pt, msg) {
		t.Fatalf("round-trip mismatch: got %q want %q", pt, msg)
	}

	// Replay should fail because deterministic counter has advanced.
	if _, err := server.Decrypt(ct1); err == nil {
		t.Fatalf("expected replay to fail with moved counter")
	}
}

func TestTcpSession_Encrypt_ChangesWithNonce(t *testing.T) {
	id := randID()
	key := randKey()

	cli, err := NewTcpCryptographyService(id, key, key, false)
	if err != nil {
		t.Fatalf("NewTcpCryptographyService: %v", err)
	}

	// prepare same message content with enough cap
	msg1 := make([]byte, 16, 16+chacha20poly1305.Overhead)
	msg2 := make([]byte, 16, 16+chacha20poly1305.Overhead)
	copy(msg1, "same-plaintext---")
	copy(msg2, "same-plaintext---")

	ct1, err := cli.Encrypt(msg1)
	if err != nil {
		t.Fatalf("Encrypt #1: %v", err)
	}
	ct2, err := cli.Encrypt(msg2)
	if err != nil {
		t.Fatalf("Encrypt #2: %v", err)
	}

	if bytes.Equal(ct1, ct2) {
		t.Fatal("ciphertexts should differ when nonce increments")
	}
}

func TestTcpSession_DifferentSessionID_Fails(t *testing.T) {
	// Different SessionId -> AAD mismatch -> decryption must fail
	idClient := randID()
	idServer := randID()
	key := randKey()

	client, err := NewTcpCryptographyService(idClient, key, key, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	server, err := NewTcpCryptographyService(idServer, key, key, true)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	msg := make([]byte, 8, 8+chacha20poly1305.Overhead)
	copy(msg, "payload!")
	ct, err := client.Encrypt(msg)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := server.Decrypt(ct); err == nil {
		t.Fatal("expected decryption error with different SessionId, got nil")
	}
}

func TestCreateAAD_BothDirections(t *testing.T) {
	// Use visible unexported constants/vars since tests are in the same package.
	id := randID()
	s := &DefaultTcpSession{SessionId: id}

	nonce := make([]byte, chacha20poly1305.NonceSize)
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}

	// Client->Server
	aadC2S := s.CreateAAD(false, nonce, make([]byte, aadLength))
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

	// Server->Client
	aadS2C := s.CreateAAD(true, nonce, make([]byte, aadLength))
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

	// Direction slices must differ
	if bytes.Equal(aadC2S[sessionIdentifierLength:sessionIdentifierLength+directionLength],
		aadS2C[sessionIdentifierLength:sessionIdentifierLength+directionLength]) {
		t.Fatal("C2S and S2C direction segments must differ")
	}
}
