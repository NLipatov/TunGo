package network

import (
	"errors"
	"testing"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

type stubAdapter struct{}

func (s *stubAdapter) Write(p []byte) (int, error) { return len(p), nil }
func (s *stubAdapter) Read(_ []byte) (int, error)  { return 0, nil }
func (s *stubAdapter) Close() error                { return nil }

type stubCrypto struct{}

func (s *stubCrypto) Encrypt(p []byte) ([]byte, error) { return p, nil }
func (s *stubCrypto) Decrypt(p []byte) ([]byte, error) { return p, nil }

type secretErr struct{}

func (s *secretErr) Exchange(_ connection.Transport) (connection.Crypto, *rekey.StateMachine, error) {
	return nil, nil, errors.New("exchange failed")
}

type secretOK struct {
	svc connection.Crypto
}

func (s *secretOK) Exchange(_ connection.Transport) (connection.Crypto, *rekey.StateMachine, error) {
	return s.svc, nil, nil
}

func TestDefaultSecureSession_Establish_SecretError(t *testing.T) {
	adapter := &stubAdapter{}
	s := NewDefaultSecureSession(adapter, &secretErr{})

	gotAdapter, gotSvc, _, err := s.Establish()
	if err == nil || err.Error() != "exchange failed" {
		t.Fatalf("want error 'exchange failed', got %v", err)
	}
	if gotAdapter != nil {
		t.Errorf("want transport=nil on error, got %v", gotAdapter)
	}
	if gotSvc != nil {
		t.Errorf("want service_packet=nil on error, got %v", gotSvc)
	}
}

func TestDefaultSecureSession_Establish_Success(t *testing.T) {
	adapter := &stubAdapter{}
	svc := &stubCrypto{}
	s := NewDefaultSecureSession(adapter, &secretOK{svc: svc})

	gotAdapter, gotSvc, _, err := s.Establish()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAdapter != adapter {
		t.Errorf("want transport=%v, got %v", adapter, gotAdapter)
	}
	if gotSvc != svc {
		t.Errorf("want service_packet=%v, got %v", svc, gotSvc)
	}
}
