package network

import (
	"errors"
	"testing"
	"tungo/application"
)

// defaultSecureSessionTestMockConnectionErr returns an error on Establish.
type defaultSecureSessionTestMockConnectionErr struct{}

func (m *defaultSecureSessionTestMockConnectionErr) Establish() (application.ConnectionAdapter, error) {
	return nil, errors.New("connection failed")
}

// defaultSecureSessionTestMockAdapter is a stub ConnectionAdapter.
type defaultSecureSessionTestMockAdapter struct{}

func (m *defaultSecureSessionTestMockAdapter) Write(b []byte) (int, error) { return len(b), nil }
func (m *defaultSecureSessionTestMockAdapter) Read(_ []byte) (int, error)  { return 0, nil }
func (m *defaultSecureSessionTestMockAdapter) Close() error                { return nil }

// defaultSecureSessionTestMockConnectionOK returns a stub adapter.
type defaultSecureSessionTestMockConnectionOK struct {
	adapter application.ConnectionAdapter
}

func (m *defaultSecureSessionTestMockConnectionOK) Establish() (application.ConnectionAdapter, error) {
	return m.adapter, nil
}

// defaultSecureSessionTestMockSecretErr returns an error on Exchange.
type defaultSecureSessionTestMockSecretErr struct{}

func (m *defaultSecureSessionTestMockSecretErr) Exchange(_ application.ConnectionAdapter) (application.CryptographyService, error) {
	return nil, errors.New("exchange failed")
}

// defaultSecureSessionTestMockCryptoService is a dummy CryptographyService.
type defaultSecureSessionTestMockCryptoService struct{}

func (m *defaultSecureSessionTestMockCryptoService) Encrypt(p []byte) ([]byte, error) { return p, nil }
func (m *defaultSecureSessionTestMockCryptoService) Decrypt(p []byte) ([]byte, error) { return p, nil }

// defaultSecureSessionTestMockSecretOK returns a dummy service.
type defaultSecureSessionTestMockSecretOK struct {
	svc application.CryptographyService
}

func (m *defaultSecureSessionTestMockSecretOK) Exchange(_ application.ConnectionAdapter) (application.CryptographyService, error) {
	return m.svc, nil
}

func TestDefaultSecureSession_Establish_ConnectionError(t *testing.T) {
	secret := NewDefaultSecureSession(
		&defaultSecureSessionTestMockConnectionErr{},
		&defaultSecureSessionTestMockSecretOK{},
	)
	conn, svc, err := secret.Establish()
	if conn != nil {
		t.Errorf("expected nil connection, got %v", conn)
	}
	if svc != nil {
		t.Errorf("expected nil service, got %v", svc)
	}
	if err == nil || err.Error() != "connection failed" {
		t.Errorf("expected connection failed error, got %v", err)
	}
}

func TestDefaultSecureSession_Establish_SecretError(t *testing.T) {
	adapter := &defaultSecureSessionTestMockAdapter{}
	secret := NewDefaultSecureSession(
		&defaultSecureSessionTestMockConnectionOK{adapter: adapter},
		&defaultSecureSessionTestMockSecretErr{},
	)
	conn, svc, err := secret.Establish()
	if conn != nil {
		t.Errorf("expected nil connection on secret error, got %v", conn)
	}
	if svc != nil {
		t.Errorf("expected nil service on secret error, got %v", svc)
	}
	if err == nil || err.Error() != "exchange failed" {
		t.Errorf("expected exchange failed error, got %v", err)
	}
}

func TestDefaultSecureSession_Establish_Success(t *testing.T) {
	adapter := &defaultSecureSessionTestMockAdapter{}
	fakeSvc := &defaultSecureSessionTestMockCryptoService{}
	secret := NewDefaultSecureSession(
		&defaultSecureSessionTestMockConnectionOK{adapter: adapter},
		&defaultSecureSessionTestMockSecretOK{svc: fakeSvc},
	)
	conn, svc, err := secret.Establish()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if conn != adapter {
		t.Errorf("expected adapter %v, got %v", adapter, conn)
	}
	if svc != fakeSvc {
		t.Errorf("expected service %v, got %v", fakeSvc, svc)
	}
}
