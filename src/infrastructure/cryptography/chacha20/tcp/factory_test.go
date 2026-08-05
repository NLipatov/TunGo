package tcp

import (
	"bytes"
	"testing"
	"tungo/application/network/connection"

	"golang.org/x/crypto/chacha20poly1305"
)

// --- mock handshake ---
type mockHandshake struct {
	id     [32]byte
	server []byte
	client []byte
}

func (m *mockHandshake) Id() [32]byte              { return m.id }
func (m *mockHandshake) KeyServerToClient() []byte { return m.server }
func (m *mockHandshake) KeyClientToServer() []byte { return m.client }
func (m *mockHandshake) ServerSideHandshake(_ connection.Transport) (int, error) {
	return 0, nil
}
func (m *mockHandshake) ClientSideHandshake(_ connection.Transport) error {
	return nil
}

type testKeyGenerator struct {
}

func (k *testKeyGenerator) validKey() []byte {
	return bytes.Repeat([]byte{1}, chacha20poly1305.KeySize)
}

func (k *testKeyGenerator) invalidKey() []byte {
	return []byte("short")
}

func TestFactory_FromHandshake_Server_Success(t *testing.T) {
	b := NewFactory()
	keyGen := testKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.validKey(),
		client: keyGen.validKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatalf("expected non-nil service_packet")
	}
	if ctrl == nil {
		t.Fatalf("expected controller for TCP")
	}
	if !ctrl.ReadyForRekey() {
		t.Fatal("expected controller to be ready for rekey")
	}
}

func TestFactory_FromHandshake_Client_Success(t *testing.T) {
	b := NewFactory()
	keyGen := testKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.validKey(),
		client: keyGen.validKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc == nil {
		t.Fatalf("expected non-nil service_packet")
	}
	if ctrl == nil {
		t.Fatalf("expected controller for TCP")
	}
}

func TestFactory_FromHandshake_Server_InvalidServerKey(t *testing.T) {
	b := NewFactory()
	keyGen := testKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.invalidKey(),
		client: keyGen.validKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, true)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service_packet")
	}
	if ctrl != nil {
		t.Fatalf("expected nil controller")
	}
}

func TestFactory_FromHandshake_Server_InvalidClientKey(t *testing.T) {
	b := NewFactory()
	keyGen := testKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.validKey(),
		client: keyGen.invalidKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, true)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service_packet")
	}
	if ctrl != nil {
		t.Fatalf("expected nil controller")
	}
}

func TestFactory_FromHandshake_Client_InvalidClientKey(t *testing.T) {
	b := NewFactory()
	keyGen := testKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.validKey(),
		client: keyGen.invalidKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service_packet")
	}
	if ctrl != nil {
		t.Fatalf("expected nil controller")
	}
}

func TestFactory_FromHandshake_Client_InvalidServerKey(t *testing.T) {
	b := NewFactory()
	keyGen := testKeyGenerator{}
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: keyGen.invalidKey(),
		client: keyGen.validKey(),
	}
	svc, ctrl, err := b.FromHandshake(hs, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service_packet")
	}
	if ctrl != nil {
		t.Fatalf("expected nil controller")
	}
}
