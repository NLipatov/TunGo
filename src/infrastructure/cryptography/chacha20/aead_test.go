package chacha20

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"testing"
	"tungo/application/network/connection"

	"golang.org/x/crypto/chacha20poly1305"
)

type fakeHandshake struct {
	s2c []byte
	c2s []byte
}

func (f fakeHandshake) Id() [32]byte {
	panic("not implemented")
}

func (f fakeHandshake) ServerSideHandshake(_ connection.Transport) (int, error) {
	panic("not implemented")
}

func (f fakeHandshake) ClientSideHandshake(_ connection.Transport) error {
	panic("not implemented")
}

func (f fakeHandshake) KeyServerToClient() []byte { return f.s2c }
func (f fakeHandshake) KeyClientToServer() []byte { return f.c2s }

func sealOpen(t *testing.T, send, recv interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}) {
	t.Helper()
	nonce := make([]byte, chacha20poly1305.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand nonce: %v", err)
	}
	plain := []byte("hello/Γειά σου/こんにちは")

	ciphertext := send.Seal(nil, nonce, plain, nil)
	decrypted, err := recv.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("Open failed: %v (ciphertext=%s)", err, hex.EncodeToString(ciphertext))
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("roundtrip mismatch: got %q want %q", decrypted, plain)
	}
}

func TestNewAEADsFromHandshake_RoleMapping(t *testing.T) {
	s2c := make([]byte, chacha20poly1305.KeySize)
	c2s := make([]byte, chacha20poly1305.KeySize)
	for i := range s2c {
		s2c[i] = byte(0xA0 + i)
		c2s[i] = byte(0x10 + i)
	}
	h := fakeHandshake{s2c: s2c, c2s: c2s}

	serverSend, serverRecv, err := NewAEADsFromHandshake(h, true)
	if err != nil {
		t.Fatalf("create server AEADs: %v", err)
	}
	clientSend, clientRecv, err := NewAEADsFromHandshake(h, false)
	if err != nil {
		t.Fatalf("create client AEADs: %v", err)
	}

	sealOpen(t, serverSend, clientRecv)
	sealOpen(t, clientSend, serverRecv)
}

func TestNewAEADsFromHandshake_InvalidKeySizes(t *testing.T) {
	tests := []struct {
		name string
		s2c  int
		c2s  int
	}{
		{"s2c short", 31, 32},
		{"c2s short", 32, 31},
		{"both short", 31, 31},
		{"s2c long", 33, 32},
		{"c2s long", 32, 33},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			h := fakeHandshake{
				s2c: make([]byte, test.s2c),
				c2s: make([]byte, test.c2s),
			}

			_, _, err := NewAEADsFromHandshake(h, true)
			if err == nil {
				t.Fatalf("expected error for sizes s2c=%d c2s=%d", test.s2c, test.c2s)
			}
		})
	}
}
