package tcp

import (
	"bytes"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestNewSession_KeyLen(t *testing.T) {
	id := randID()
	short := make([]byte, 5)
	key := randKey()

	if _, err := NewSession(id, short, key, false); err == nil {
		t.Fatal("expected error for short send key")
	}
	if _, err := NewSession(id, key, short, false); err == nil {
		t.Fatal("expected error for short receive key")
	}
}

func TestSession_Encrypt_InPlaceCapacityError(t *testing.T) {
	session, err := NewSession(randID(), randKey(), randKey(), false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	if _, err := session.Encrypt(make([]byte, 32)); err == nil {
		t.Fatal("expected insufficient capacity error")
	}
}

func TestSession_RoundTrip_AndReplay(t *testing.T) {
	id := randID()
	key := randKey()
	client, err := NewSession(id, key, key, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	server, err := NewSession(id, key, key, true)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	message := []byte("secret payload")
	buf := make([]byte, len(message), len(message)+chacha20poly1305.Overhead)
	copy(buf, message)

	ciphertext, err := client.Encrypt(buf)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	plaintext, err := server.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(plaintext, message) {
		t.Fatalf("round-trip mismatch: got %q want %q", plaintext, message)
	}
	if _, err := server.Decrypt(ciphertext); err == nil {
		t.Fatal("expected replay to fail with moved counter")
	}
}

func TestSession_Encrypt_ChangesWithNonce(t *testing.T) {
	session, err := NewSession(randID(), randKey(), randKey(), false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	first := make([]byte, 16, 16+chacha20poly1305.Overhead)
	second := make([]byte, 16, 16+chacha20poly1305.Overhead)
	copy(first, "same-plaintext---")
	copy(second, "same-plaintext---")

	firstCiphertext, err := session.Encrypt(first)
	if err != nil {
		t.Fatalf("Encrypt #1: %v", err)
	}
	secondCiphertext, err := session.Encrypt(second)
	if err != nil {
		t.Fatalf("Encrypt #2: %v", err)
	}
	if bytes.Equal(firstCiphertext, secondCiphertext) {
		t.Fatal("ciphertexts should differ when nonce increments")
	}
}

func TestSession_DifferentSessionIDFails(t *testing.T) {
	key := randKey()
	client, err := NewSession(randID(), key, key, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	server, err := NewSession(randID(), key, key, true)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	message := make([]byte, 8, 8+chacha20poly1305.Overhead)
	copy(message, "payload!")
	ciphertext, err := client.Encrypt(message)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := server.Decrypt(ciphertext); err == nil {
		t.Fatal("expected decryption error with different session ID")
	}
}

func TestSession_CreateAAD_BothDirections(t *testing.T) {
	id := randID()
	key := randKey()
	client, err := NewSession(id, key, key, false)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	server, err := NewSession(id, key, key, true)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	nonce := make([]byte, chacha20poly1305.NonceSize)
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}

	clientAAD := client.CreateAAD(false, nonce, client.encryptionAadBuf[:])
	serverAAD := server.CreateAAD(true, nonce, server.encryptionAadBuf[:])
	assertAAD(t, clientAAD, id, dirC2S, nonce)
	assertAAD(t, serverAAD, id, dirS2C, nonce)
	if bytes.Equal(
		clientAAD[sessionIdentifierLength:sessionIdentifierLength+directionLength],
		serverAAD[sessionIdentifierLength:sessionIdentifierLength+directionLength],
	) {
		t.Fatal("client-to-server and server-to-client directions must differ")
	}
}

func TestSession_Encrypt_NonceOverflow(t *testing.T) {
	session, err := NewSession(randID(), randKey(), randKey(), false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	session.SendNonce.CounterHigh = ^uint16(0)
	session.SendNonce.CounterLow = ^uint64(0)

	message := make([]byte, 1, 1+chacha20poly1305.Overhead)
	if _, err := session.Encrypt(message); err == nil {
		t.Fatal("expected nonce overflow error")
	}
}

func TestSession_Decrypt_PeekNonceOverflow(t *testing.T) {
	session, err := NewSession(randID(), randKey(), randKey(), false)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	session.RecvNonce.CounterHigh = ^uint16(0)
	session.RecvNonce.CounterLow = ^uint64(0)

	if _, err := session.Decrypt([]byte{1}); err == nil {
		t.Fatal("expected nonce overflow error")
	}
}

func assertAAD(t *testing.T, aad []byte, id [32]byte, direction [16]byte, nonce []byte) {
	t.Helper()
	if len(aad) != aadLength {
		t.Fatalf("AAD length=%d, want %d", len(aad), aadLength)
	}
	if !bytes.Equal(aad[:sessionIdentifierLength], id[:]) {
		t.Fatal("session ID mismatch")
	}
	if !bytes.Equal(aad[sessionIdentifierLength:sessionIdentifierLength+directionLength], direction[:]) {
		t.Fatal("direction mismatch")
	}
	if !bytes.Equal(aad[sessionIdentifierLength+directionLength:aadLength], nonce) {
		t.Fatal("nonce mismatch")
	}
}
