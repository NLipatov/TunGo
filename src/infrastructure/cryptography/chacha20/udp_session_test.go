package chacha20

import (
	"bytes"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestNewUdpSession_KeyLengthError(t *testing.T) {
	var id [32]byte
	// too-short sendKey
	_, err := NewUdpSession(id, []byte("short"), make([]byte, chacha20poly1305.KeySize), false)
	if err == nil {
		t.Fatal("expected error for invalid sendKey length")
	}
	// too-short recvKey
	_, err = NewUdpSession(id, make([]byte, chacha20poly1305.KeySize), []byte("short"), false)
	if err == nil {
		t.Fatal("expected error for invalid recvKey length")
	}
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	var id [32]byte
	copy(id[:], bytes.Repeat([]byte{0x7F}, 32))

	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	// client and server use the same key in test
	clientSess, err := NewUdpSession(id, key, key, false)
	if err != nil {
		t.Fatalf("NewUdpSession(client): %v", err)
	}
	serverSess, err := NewUdpSession(id, key, key, true)
	if err != nil {
		t.Fatalf("NewUdpSession(server): %v", err)
	}

	payload := []byte("hello world. this is a UDP packet test 123")
	ciphertext, err := clientSess.Encrypt(payload)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	plaintext, err := serverSess.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, payload) {
		t.Errorf("plaintext mismatch: got %q, want %q", plaintext, payload)
	}
}

func TestEncrypt_UniqueNonce(t *testing.T) {
	var id [32]byte
	key := make([]byte, chacha20poly1305.KeySize)
	clientSess, err := NewUdpSession(id, key, key, false)
	if err != nil {
		t.Fatalf("NewUdpSession(client): %v", err)
	}
	payload := []byte("x")
	ct1, err := clientSess.Encrypt(payload)
	ct1copy := make([]byte, len(ct1))
	copy(ct1copy, ct1)

	ct2, err := clientSess.Encrypt(payload)
	ct2copy := make([]byte, len(ct2))
	copy(ct2copy, ct2)

	if bytes.Equal(ct1copy, ct2copy) {
		t.Logf("ct1: %x", ct1copy)
		t.Logf("ct2: %x", ct2copy)
		t.Error("ciphertexts with different nonces should not be equal")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	var id [32]byte
	key := make([]byte, chacha20poly1305.KeySize)
	sess, _ := NewUdpSession(id, key, key, false)
	_, err := sess.Decrypt([]byte("short"))
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("too short")) {
		t.Errorf("expected 'too short' error, got %v", err)
	}
}

func TestDecrypt_InvalidNonce(t *testing.T) {
	var id [32]byte
	key := make([]byte, chacha20poly1305.KeySize)
	// Encrypt
	sender, _ := NewUdpSession(id, key, key, false)
	payload := []byte("test packet")
	ct, err := sender.Encrypt(payload)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// first Decrypt call on new sliding window
	receiver, _ := NewUdpSession(id, key, key, true)
	_, err = receiver.Decrypt(ct)
	if err != nil {
		t.Fatalf("first Decrypt: %v", err)
	}
	// second Decrypt call (replay attack)
	_, err = receiver.Decrypt(ct)
	if err == nil {
		t.Error("expected error on nonce reuse, got nil")
	}
}

func TestCreateAAD_ContentAndLength(t *testing.T) {
	var id [32]byte
	for i := range id {
		id[i] = byte(i)
	}
	sess := &DefaultUdpSession{SessionId: id}
	nonce := make([]byte, 12)
	for i := range nonce {
		nonce[i] = byte(100 + i)
	}

	for _, isSrv := range []bool{false, true} {
		aad := sess.CreateAAD(isSrv, nonce, make([]byte, 60))
		var dir []byte
		if isSrv {
			dir = []byte("server-to-client")
		} else {
			dir = []byte("client-to-server")
		}
		expLen := 32 + len(dir) + len(nonce)
		if len(aad) != expLen {
			t.Errorf("AAD length = %d; want %d", len(aad), expLen)
		}
		if !bytes.Equal(aad[:32], id[:]) {
			t.Error("AAD session ID mismatch")
		}
		if !bytes.Equal(aad[32:32+len(dir)], dir) {
			t.Error("AAD direction mismatch")
		}
		if !bytes.Equal(aad[32+len(dir):], nonce) {
			t.Error("AAD nonce mismatch")
		}
	}
}
