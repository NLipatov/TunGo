package network

import (
	"errors"
	"net"
	"testing"
	"tungo/application"
	"tungo/infrastructure/settings"
)

// secretTestMockHandshake implements application.Handshake for testing DefaultSecret.Exchange.
type secretTestMockHandshake struct {
	err error
}

func (m *secretTestMockHandshake) Id() [32]byte      { return [32]byte{} }
func (m *secretTestMockHandshake) ClientKey() []byte { return nil }
func (m *secretTestMockHandshake) ServerKey() []byte { return nil }
func (m *secretTestMockHandshake) ServerSideHandshake(conn application.ConnectionAdapter) (net.IP, error) {
	return nil, nil
}
func (m *secretTestMockHandshake) ClientSideHandshake(conn application.ConnectionAdapter, settings settings.Settings) error {
	return m.err
}

// secretTestMockBuilder implements application.CryptographyServiceFactory for testing DefaultSecret.Exchange.
type secretTestMockBuilder struct {
	svc application.CryptographyService
	err error
}

func (m *secretTestMockBuilder) FromHandshake(h application.Handshake, isServer bool) (application.CryptographyService, error) {
	return m.svc, m.err
}

// mockCryptoService implements application.CryptographyService as a dummy.
type mockCryptoService struct{}

func (m *mockCryptoService) Encrypt(p []byte) ([]byte, error) { return nil, nil }
func (m *mockCryptoService) Decrypt(p []byte) ([]byte, error) { return nil, nil }

// mockConn is a no-op ConnectionAdapter stub.
type mockConn struct{}

func (m *mockConn) Write([]byte) (int, error) { return 0, nil }
func (m *mockConn) Read([]byte) (int, error)  { return 0, nil }
func (m *mockConn) Close() error              { return nil }

// TestExchange_HandshakeError verifies that an error from ClientSideHandshake is returned as-is.
func TestExchange_HandshakeError(t *testing.T) {
	hsErr := errors.New("handshake failed")
	secret := NewDefaultSecret(settings.Settings{}, &secretTestMockHandshake{err: hsErr}, &secretTestMockBuilder{})
	svc, err := secret.Exchange(&mockConn{})
	if svc != nil {
		t.Errorf("expected nil service on handshake error, got %v", svc)
	}
	if !errors.Is(err, hsErr) {
		t.Errorf("expected handshake error %v, got %v", hsErr, err)
	}
}

// TestExchange_BuilderError verifies that an error from FromHandshake is wrapped properly.
func TestExchange_BuilderError(t *testing.T) {
	builderErr := errors.New("builder failed")
	secret := NewDefaultSecret(
		settings.Settings{},
		&secretTestMockHandshake{err: nil},
		&secretTestMockBuilder{svc: nil, err: builderErr},
	)
	svc, err := secret.Exchange(&mockConn{})
	if svc != nil {
		t.Errorf("expected nil service on builder error, got %v", svc)
	}
	wantPrefix := "failed to create client cryptographyService: "
	if err == nil || err.Error()[:len(wantPrefix)] != wantPrefix {
		t.Errorf("expected error prefix %q, got %v", wantPrefix, err)
	}
	if !errors.Is(err, builderErr) {
		t.Errorf("expected wrapped builder error %v, got %v", builderErr, err)
	}
}

// TestExchange_Success verifies that a successful handshake and builder produce the returned service.
func TestExchange_Success(t *testing.T) {
	fakeSvc := &mockCryptoService{}
	secret := NewDefaultSecret(
		settings.Settings{},
		&secretTestMockHandshake{err: nil},
		&secretTestMockBuilder{svc: fakeSvc, err: nil},
	)
	svc, err := secret.Exchange(&mockConn{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if svc != fakeSvc {
		t.Errorf("expected service %v, got %v", fakeSvc, svc)
	}
}
