package chacha20

import (
	"bytes"
	"crypto/rand"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestNewTcpCryptographyService_KeyLen(t *testing.T) {
	var id [32]byte
	short := make([]byte, 5)
	long := make([]byte, chacha20poly1305.KeySize)
	if _, err := NewTcpCryptographyService(id, short, long, false); err == nil {
		t.Error("expected error for short sendKey")
	}
	if _, err := NewTcpCryptographyService(id, long, short, false); err == nil {
		t.Error("expected error for short recvKey")
	}
}

func TestTcpSessionEncryptDecrypt_RoundTrip(t *testing.T) {
	var id [32]byte
	rand.Read(id[:])
	key := make([]byte, chacha20poly1305.KeySize)
	rand.Read(key)

	// client→server
	client, _ := NewTcpCryptographyService(id, key, key, false)
	server, _ := NewTcpCryptographyService(id, key, key, true)

	msg := []byte("secret payload")
	ct, err := client.Encrypt(msg)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	pt, err := server.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(pt, msg) {
		t.Errorf("got %q; want %q", pt, msg)
	}

	// replay same ct → ErrNonUniqueNonce
	if _, err := server.Decrypt(ct); err == nil {
		t.Errorf("expected error on replay, got nil")
	}
}

func TestCreateAAD(t *testing.T) {
	var id [32]byte
	for i := range id {
		id[i] = byte(i)
	}
	svc := &TcpCryptographyService{SessionId: id}
	nonce := make([]byte, 12)
	for i := range nonce {
		nonce[i] = byte(i + 1)
	}
	aad := svc.CreateAAD(false, nonce, make([]byte, 80))
	dir := []byte("client-to-server")
	if exp := 32 + len(dir) + len(nonce); len(aad) != exp {
		t.Fatalf("len(aad) = %d; want %d", len(aad), exp)
	}
	if !bytes.Equal(aad[:32], id[:]) {
		t.Error("session ID part mismatch")
	}
	if !bytes.Equal(aad[32:32+len(dir)], dir) {
		t.Error("direction part mismatch")
	}
	if !bytes.Equal(aad[32+len(dir):], nonce) {
		t.Error("nonce part mismatch")
	}
}
