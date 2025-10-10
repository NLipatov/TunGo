package chacha20

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"net"
	"testing"
	"tungo/application/network/connection"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

// ---- test helpers ----

type fakeHandshake struct {
	s2c []byte
	c2s []byte
}

func (f fakeHandshake) Id() [32]byte {
	panic("not implemented")
}

func (f fakeHandshake) ServerSideHandshake(_ connection.Transport) (net.IP, error) {
	panic("not implemented")
}

func (f fakeHandshake) ClientSideHandshake(_ connection.Transport, _ settings.Settings) error {
	panic("not implemented")
}

func (f fakeHandshake) KeyServerToClient() []byte { return f.s2c }
func (f fakeHandshake) KeyClientToServer() []byte { return f.c2s }

// If your application.Handshake has more methods, add no-op stubs here to satisfy the interface.
// For these tests, only key methods are required by the AEADBuilder.

// sealOpen is a small helper to exercise AEAD interop (send.Seal vs recv.Open).
func sealOpen(t *testing.T, send, recv interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}) {
	t.Helper()
	nonce := make([]byte, chacha20poly1305.NonceSize) // 12 bytes
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand nonce: %v", err)
	}
	plain := []byte("hello/Γειά σου/こんにちは")

	ct := send.Seal(nil, nonce, plain, nil)
	pt, err := recv.Open(nil, nonce, ct, nil)
	if err != nil {
		t.Fatalf("Open failed: %v (ct=%s)", err, hex.EncodeToString(ct))
	}
	if !bytes.Equal(pt, plain) {
		t.Fatalf("roundtrip mismatch: got %q want %q", pt, plain)
	}
}

// ---- tests ----

func TestChaCha20AEADBuilder_RoleMapping_ServerClientInterop(t *testing.T) {
	// two distinct keys so we detect any direction swap
	kS2C := make([]byte, chacha20poly1305.KeySize)
	kC2S := make([]byte, chacha20poly1305.KeySize)
	for i := range kS2C {
		kS2C[i] = byte(0xA0 + i) // deterministic pattern
		kC2S[i] = byte(0x10 + i)
	}
	h := fakeHandshake{s2c: kS2C, c2s: kC2S}
	b := NewDefaultAEADBuilder()

	// build for server
	sSend, sRecv, err := b.FromHandshake(h, true)
	if err != nil {
		t.Fatalf("server builder error: %v", err)
	}
	// build for client
	cSend, cRecv, err := b.FromHandshake(h, false)
	if err != nil {
		t.Fatalf("client builder error: %v", err)
	}

	// Server → Client must interoperate (S uses S→C to seal; C uses S→C to open)
	sealOpen(t, sSend, cRecv)

	// Client → Server must interoperate (C uses C→S to seal; S uses C→S to open)
	sealOpen(t, cSend, sRecv)
}

func TestChaCha20AEADBuilder_InvalidKeySizes(t *testing.T) {
	type tc struct {
		name string
		s2c  int
		c2s  int
	}
	cases := []tc{
		{"s2c short", 31, 32},
		{"c2s short", 32, 31},
		{"both short", 31, 31},
		{"s2c long", 33, 32},
		{"c2s long", 32, 33},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kS2C := make([]byte, c.s2c)
			kC2S := make([]byte, c.c2s)
			h := fakeHandshake{s2c: kS2C, c2s: kC2S}

			_, _, err := NewDefaultAEADBuilder().FromHandshake(h, true)
			if err == nil {
				t.Fatalf("expected error for sizes s2c=%d c2s=%d", c.s2c, c.c2s)
			}
		})
	}
}
