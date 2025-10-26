package chacha20

import (
	"bytes"
	"crypto/cipher"
	"fmt"
	"net"
	"testing"
	"tungo/application/network/connection"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

// mock aead builder
type fakeAEADBuilder struct{}

func (fakeAEADBuilder) FromHandshake(h connection.Handshake, isServer bool) (cipher.AEAD, cipher.AEAD, error) {
	// Choose correct key directions based on isServer flag
	var sendKey, recvKey []byte
	if isServer {
		sendKey = h.KeyServerToClient()
		recvKey = h.KeyClientToServer()
	} else {
		sendKey = h.KeyClientToServer()
		recvKey = h.KeyServerToClient()
	}

	// Simulate real key length validation
	if len(sendKey) != chacha20poly1305.KeySize || len(recvKey) != chacha20poly1305.KeySize {
		return nil, nil, fmt.Errorf("invalid key length: send=%d recv=%d", len(sendKey), len(recvKey))
	}

	// Return our fake AEAD instances
	return fakeAEAD{}, fakeAEAD{}, nil
}

type fakeAEAD struct{}

func (f fakeAEAD) FromHandshake(_ connection.Handshake, _ bool) (send cipher.AEAD, recv cipher.AEAD, err error) {
	return fakeAEAD{}, fakeAEAD{}, nil
}

func (f fakeAEAD) NonceSize() int { return 12 }
func (f fakeAEAD) Overhead() int  { return 0 }
func (f fakeAEAD) Seal(dst, nonce, plaintext, ad []byte) []byte {
	_ = nonce
	_ = ad
	out := make([]byte, len(dst)+len(plaintext))
	copy(out, dst)
	copy(out[len(dst):], plaintext)
	return out
}
func (f fakeAEAD) Open(dst, nonce, ciphertext, ad []byte) ([]byte, error) {
	_ = nonce
	_ = ad
	out := make([]byte, len(dst)+len(ciphertext))
	copy(out, dst)
	copy(out[len(dst):], ciphertext)
	return out, nil
}

// --- mock handshake ---
type mockHandshake struct {
	id     [32]byte
	server []byte
	client []byte
}

func (m *mockHandshake) Id() [32]byte              { return m.id }
func (m *mockHandshake) KeyServerToClient() []byte { return m.server }
func (m *mockHandshake) KeyClientToServer() []byte { return m.client }
func (m *mockHandshake) ServerSideHandshake(_ connection.Transport) (net.IP, error) {
	return m.server, nil
}
func (m *mockHandshake) ClientSideHandshake(_ connection.Transport, _ settings.Settings) error {
	return nil
}
func (m *mockHandshake) PeerMTU() (int, bool) { return 0, false }

type tcpSessionTestKeyGenerator struct {
}

func (k *tcpSessionTestKeyGenerator) validKey() []byte {
	return bytes.Repeat([]byte{1}, chacha20poly1305.KeySize)
}

func (k *tcpSessionTestKeyGenerator) invalidKey() []byte {
	return []byte("short")
}

func TestTcpSessionBuilder_FromHandshake_Server_Success(t *testing.T) {
	b := NewTcpSessionBuilder(&fakeAEADBuilder{})
	keyGen := tcpSessionTestKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.validKey(),
		client: keyGen.validKey(),
	}
	svc, err := b.FromHandshake(hs, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatalf("expected non-nil service")
	}
}

func TestTcpSessionBuilder_FromHandshake_Client_Success(t *testing.T) {
	b := NewTcpSessionBuilder(&fakeAEADBuilder{})
	keyGen := tcpSessionTestKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.validKey(),
		client: keyGen.validKey(),
	}
	svc, err := b.FromHandshake(hs, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatalf("expected non-nil service")
	}
}

func TestTcpSessionBuilder_FromHandshake_Server_InvalidServerKey(t *testing.T) {
	b := NewTcpSessionBuilder(&fakeAEADBuilder{})
	keyGen := tcpSessionTestKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.invalidKey(),
		client: keyGen.validKey(),
	}
	svc, err := b.FromHandshake(hs, true)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service")
	}
}

func TestTcpSessionBuilder_FromHandshake_Server_InvalidClientKey(t *testing.T) {
	b := NewTcpSessionBuilder(&fakeAEADBuilder{})
	keyGen := tcpSessionTestKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.validKey(),
		client: keyGen.invalidKey(),
	}
	svc, err := b.FromHandshake(hs, true)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service")
	}
}

func TestTcpSessionBuilder_FromHandshake_Client_InvalidClientKey(t *testing.T) {
	b := NewTcpSessionBuilder(&fakeAEADBuilder{})
	keyGen := tcpSessionTestKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.validKey(),
		client: keyGen.invalidKey(),
	}
	svc, err := b.FromHandshake(hs, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service")
	}
}

func TestTcpSessionBuilder_FromHandshake_Client_InvalidServerKey(t *testing.T) {
	b := NewTcpSessionBuilder(&fakeAEADBuilder{})
	keyGen := tcpSessionTestKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.invalidKey(),
		client: keyGen.validKey(),
	}
	svc, err := b.FromHandshake(hs, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service")
	}
}
