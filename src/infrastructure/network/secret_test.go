package network

import (
	"errors"
	"testing"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

// secretTestMockHandshake implements application.Handshake for testing DefaultSecret.Exchange.
type secretTestMockHandshake struct {
	err error
}

func (m *secretTestMockHandshake) Id() [32]byte              { return [32]byte{} }
func (m *secretTestMockHandshake) KeyClientToServer() []byte { return nil }
func (m *secretTestMockHandshake) KeyServerToClient() []byte { return nil }
func (m *secretTestMockHandshake) ServerSideHandshake(_ connection.Transport) (int, error) {
	return 0, nil
}
func (m *secretTestMockHandshake) ClientSideHandshake(_ connection.Transport) error {
	return m.err
}

// secretTestMockBuilder implements application.CryptoFactory for testing DefaultSecret.Exchange.
type secretTestMockBuilder struct {
	svc connection.Crypto
	err error
}

func (m *secretTestMockBuilder) FromHandshake(_ connection.Handshake, _ bool) (connection.Crypto, *rekey.StateMachine, error) {
	return m.svc, nil, m.err
}

// mockCryptoService implements application.crypto as a dummy.
type mockCryptoService struct{}

func (m *mockCryptoService) Encrypt(_ []byte) ([]byte, error) { return nil, nil }
func (m *mockCryptoService) Decrypt(_ []byte) ([]byte, error) { return nil, nil }

// mockConn is a no-op Transport stub.
type mockConn struct{}

func (m *mockConn) Write([]byte) (int, error) { return 0, nil }
func (m *mockConn) Read([]byte) (int, error)  { return 0, nil }
func (m *mockConn) Close() error              { return nil }

// TestExchange_HandshakeError verifies that an error from ClientSideHandshake is returned as-is.
func TestExchange_HandshakeError(t *testing.T) {
	hsErr := errors.New("handshake failed")
	secret := NewDefaultSecret(&secretTestMockHandshake{err: hsErr}, &secretTestMockBuilder{})
	svc, ctrl, err := secret.Exchange(&mockConn{})
	if svc != nil {
		t.Errorf("expected nil service_packet on handshake error, got %v", svc)
	}
	if ctrl != nil {
		t.Errorf("expected nil controller on handshake error, got %v", ctrl)
	}
	if !errors.Is(err, hsErr) {
		t.Errorf("expected handshake error %v, got %v", hsErr, err)
	}
}

// TestExchange_BuilderError verifies that an error from FromHandshake is wrapped properly.
func TestExchange_BuilderError(t *testing.T) {
	builderErr := errors.New("builder failed")
	secret := NewDefaultSecret(
		&secretTestMockHandshake{err: nil},
		&secretTestMockBuilder{svc: nil, err: builderErr},
	)
	svc, ctrl, err := secret.Exchange(&mockConn{})
	if svc != nil {
		t.Errorf("expected nil service_packet on builder error, got %v", svc)
	}
	if ctrl != nil {
		t.Errorf("expected nil controller on builder error, got %v", ctrl)
	}
	wantPrefix := "failed to create client crypto: "
	if err == nil || err.Error()[:len(wantPrefix)] != wantPrefix {
		t.Errorf("expected error prefix %q, got %v", wantPrefix, err)
	}
	if !errors.Is(err, builderErr) {
		t.Errorf("expected wrapped builder error %v, got %v", builderErr, err)
	}
}

// TestExchange_Success verifies that a successful handshake and builder produce the returned service_packet.
func TestExchange_Success(t *testing.T) {
	fakeSvc := &mockCryptoService{}
	secret := NewDefaultSecret(
		&secretTestMockHandshake{err: nil},
		&secretTestMockBuilder{svc: fakeSvc, err: nil},
	)
	svc, ctrl, err := secret.Exchange(&mockConn{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if svc != fakeSvc {
		t.Errorf("expected service_packet %v, got %v", fakeSvc, svc)
	}
	if ctrl != nil {
		t.Errorf("expected controller nil, got %v", ctrl)
	}
}
