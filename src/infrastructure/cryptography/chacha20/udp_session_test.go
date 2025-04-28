package chacha20

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
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

	// use same key for send and recv
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	// client session encrypts client→server
	clientSess, err := NewUdpSession(id, key, key, false)
	if err != nil {
		t.Fatalf("NewUdpSession(client): %v", err)
	}

	// craft packet: 12-byte header + payload, with extra cap for tag
	payload := []byte("hello world")
	aead, _ := chacha20poly1305.New(key)
	overhead := aead.Overhead() // 16
	packet := make([]byte, 12+len(payload), 12+len(payload)+overhead)
	binary.BigEndian.PutUint32(packet[:4], uint32(len(payload)))
	copy(packet[12:], payload)

	ct, err := clientSess.Encrypt(packet)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// server session decrypts server→client
	serverSess, err := NewUdpSession(id, key, key, true)
	if err != nil {
		t.Fatalf("NewUdpSession(server): %v", err)
	}
	pt, err := serverSess.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(pt, payload) {
		t.Errorf("plaintext mismatch: got %q, want %q", pt, payload)
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
