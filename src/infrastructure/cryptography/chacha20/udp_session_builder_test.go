package chacha20

import (
	"bytes"
	"net"
	"testing"
	"tungo/application/network/connection"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
)

// --- mock handshake ---
type mockUdpHandshake struct {
	id     [32]byte
	server []byte
	client []byte
}

func (m *mockUdpHandshake) Id() [32]byte              { return m.id }
func (m *mockUdpHandshake) KeyServerToClient() []byte { return m.server }
func (m *mockUdpHandshake) KeyClientToServer() []byte { return m.client }
func (m *mockUdpHandshake) ServerSideHandshake(_ connection.Transport) (net.IP, error) {
	return m.server, nil
}
func (m *mockUdpHandshake) ClientSideHandshake(_ connection.Transport, _ settings.Settings) error {
	return nil
}

type udpSessionTestKeyGenerator struct {
}

func (k *udpSessionTestKeyGenerator) validKey() []byte {
	return bytes.Repeat([]byte{1}, chacha20poly1305.KeySize)
}

func (k *udpSessionTestKeyGenerator) invalidKey() []byte {
	return []byte("short")
}

func TestUdpSessionBuilder_FromHandshake_Server_Success(t *testing.T) {
	b := NewUdpSessionBuilder(&fakeAEADBuilder{})
	keyGen := udpSessionTestKeyGenerator{}
	hs := &mockUdpHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.validKey(),
		client: keyGen.validKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if ctrl == nil {
		t.Fatal("expected non-nil controller")
	}
}

func TestUdpSessionBuilder_FromHandshake_Client_Success(t *testing.T) {
	b := NewUdpSessionBuilder(&fakeAEADBuilder{})
	keyGen := udpSessionTestKeyGenerator{}
	hs := &mockUdpHandshake{
		id:     [32]byte{4, 5, 6},
		server: keyGen.validKey(),
		client: keyGen.validKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if ctrl == nil {
		t.Fatal("expected non-nil controller")
	}
}

func TestUdpSessionBuilder_FromHandshake_Server_InvalidServerKey(t *testing.T) {
	b := NewUdpSessionBuilder(&fakeAEADBuilder{})
	keyGen := udpSessionTestKeyGenerator{}
	hs := &mockUdpHandshake{
		id:     [32]byte{7, 8, 9},
		server: keyGen.invalidKey(),
		client: keyGen.validKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if svc != nil {
		t.Fatal("expected nil service")
	}
	if ctrl != nil {
		t.Fatal("expected nil controller")
	}
}

func TestUdpSessionBuilder_FromHandshake_Server_InvalidClientKey(t *testing.T) {
	b := NewUdpSessionBuilder(&fakeAEADBuilder{})
	keyGen := udpSessionTestKeyGenerator{}
	hs := &mockUdpHandshake{
		id:     [32]byte{10, 11, 12},
		server: keyGen.validKey(),
		client: keyGen.invalidKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if svc != nil {
		t.Fatal("expected nil service")
	}
	if ctrl != nil {
		t.Fatal("expected nil controller")
	}
}

func TestUdpSessionBuilder_FromHandshake_Client_InvalidClientKey(t *testing.T) {
	b := NewUdpSessionBuilder(&fakeAEADBuilder{})
	keyGen := udpSessionTestKeyGenerator{}
	hs := &mockUdpHandshake{
		id:     [32]byte{13, 14, 15},
		server: keyGen.validKey(),
		client: keyGen.invalidKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if svc != nil {
		t.Fatal("expected nil service")
	}
	if ctrl != nil {
		t.Fatal("expected nil controller")
	}
}

func TestUdpSessionBuilder_FromHandshake_Client_InvalidServerKey(t *testing.T) {
	b := NewUdpSessionBuilder(&fakeAEADBuilder{})
	keyGen := udpSessionTestKeyGenerator{}
	hs := &mockUdpHandshake{
		id:     [32]byte{16, 17, 18},
		server: keyGen.invalidKey(),
		client: keyGen.validKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if svc != nil {
		t.Fatal("expected nil service")
	}
	if ctrl != nil {
		t.Fatal("expected nil controller")
	}
}
