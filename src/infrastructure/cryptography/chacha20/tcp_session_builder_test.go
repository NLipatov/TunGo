package chacha20

import (
	"bytes"
	"net"
	"testing"
	"tungo/infrastructure/settings"

	"golang.org/x/crypto/chacha20poly1305"
	"tungo/application"
)

// --- mock handshake ---
type mockHandshake struct {
	id     [32]byte
	server []byte
	client []byte
}

func (m *mockHandshake) Id() [32]byte      { return m.id }
func (m *mockHandshake) ServerKey() []byte { return m.server }
func (m *mockHandshake) ClientKey() []byte { return m.client }
func (m *mockHandshake) ServerSideHandshake(_ application.ConnectionAdapter) (net.IP, error) {
	return m.server, nil
}
func (m *mockHandshake) ClientSideHandshake(_ application.ConnectionAdapter, _ settings.Settings) error {
	return nil
}

func validKey() []byte {
	return bytes.Repeat([]byte{1}, chacha20poly1305.KeySize)
}

func invalidKey() []byte {
	return []byte("short")
}

func TestTcpSessionBuilder_FromHandshake_Server_Success(t *testing.T) {
	b := NewTcpSessionBuilder()
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: validKey(),
		client: validKey(),
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
	b := NewTcpSessionBuilder()
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: validKey(),
		client: validKey(),
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
	b := NewTcpSessionBuilder()
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: invalidKey(),
		client: validKey(),
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
	b := NewTcpSessionBuilder()
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: validKey(),
		client: invalidKey(),
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
	b := NewTcpSessionBuilder()
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: validKey(),
		client: invalidKey(),
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
	b := NewTcpSessionBuilder()
	hs := &mockHandshake{
		id:     [32]byte{1, 2, 3},
		server: invalidKey(),
		client: validKey(),
	}
	svc, err := b.FromHandshake(hs, false)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if svc != nil {
		t.Fatalf("expected nil service")
	}
}
