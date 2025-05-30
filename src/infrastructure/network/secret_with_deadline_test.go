package network

import (
	"errors"
	"testing"
	"tungo/infrastructure/settings"
)

// TestExchange_HandshakeError verifies that an error from ClientSideHandshake is returned as-is.
func TestSecretWithDeadlineExchange_HandshakeError(t *testing.T) {
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
func TestSecretWithDeadlineExchange_BuilderError(t *testing.T) {
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
func TestSecretWithDeadlineExchange_Success(t *testing.T) {
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
