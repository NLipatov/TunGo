package network

import (
	"context"
	"errors"
	"testing"
	"time"
	"tungo/application/network/connection"
)

// secureSessionWithDeadlineTestMockSession implements SecureSession for testing.
// If block is true, Establish will hang; otherwise returns preset values.
type secureSessionWithDeadlineTestMockSession struct {
	transport connection.Transport
	crypto    connection.Crypto
	err       error
	block     bool
}

func (m *secureSessionWithDeadlineTestMockSession) Establish() (connection.Transport, connection.Crypto, error) {
	if m.block {
		select {} // hang forever
	}
	return m.transport, m.crypto, m.err
}

// secureSessionWithDeadlineTestMockAdapter is a no-op Transport stub.
type secureSessionWithDeadlineTestMockAdapter struct{}

func (m *secureSessionWithDeadlineTestMockAdapter) Write([]byte) (int, error) { return 0, nil }
func (m *secureSessionWithDeadlineTestMockAdapter) Read([]byte) (int, error)  { return 0, nil }
func (m *secureSessionWithDeadlineTestMockAdapter) Close() error              { return nil }

// secureSessionWithDeadlineTestMockCrypto is a dummy cryptographyService.
type secureSessionWithDeadlineTestMockCrypto struct{}

func (m *secureSessionWithDeadlineTestMockCrypto) Encrypt(p []byte) ([]byte, error) { return p, nil }
func (m *secureSessionWithDeadlineTestMockCrypto) Decrypt(p []byte) ([]byte, error) { return p, nil }

// TestSecureSessionWithDeadline_Success covers the case where the underlying
// SecureSession returns immediately with no error.
func TestSecureSessionWithDeadline_Success(t *testing.T) {
	adapter := &secureSessionWithDeadlineTestMockAdapter{}
	crypto := &secureSessionWithDeadlineTestMockCrypto{}
	mockSess := &secureSessionWithDeadlineTestMockSession{
		transport: adapter,
		crypto:    crypto,
		err:       nil,
		block:     false,
	}
	wrapper := NewSecureSessionWithDeadline(context.Background(), mockSess)

	conn, svc, err := wrapper.Establish()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if conn != adapter {
		t.Errorf("expected transport %v, got %v", adapter, conn)
	}
	if svc != crypto {
		t.Errorf("expected crypto %v, got %v", crypto, svc)
	}
}

// TestSecureSessionWithDeadline_UnderlyingError covers the case where the
// underlying SecureSession returns an error immediately.
func TestSecureSessionWithDeadline_UnderlyingError(t *testing.T) {
	wantErr := errors.New("underlying failure")
	mockSess := &secureSessionWithDeadlineTestMockSession{
		transport: nil,
		crypto:    nil,
		err:       wantErr,
		block:     false,
	}
	wrapper := NewSecureSessionWithDeadline(context.Background(), mockSess)

	conn, svc, err := wrapper.Establish()
	if conn != nil {
		t.Errorf("expected nil transport on error, got %v", conn)
	}
	if svc != nil {
		t.Errorf("expected nil crypto on error, got %v", svc)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
}

// TestSecureSessionWithDeadline_Cancel covers the case where the context is
// canceled before the underlying SecureSession returns.
func TestSecureSessionWithDeadline_Cancel(t *testing.T) {
	// Underlying will block forever
	mockSess := &secureSessionWithDeadlineTestMockSession{block: true}
	ctx, cancel := context.WithCancel(context.Background())
	wrapper := NewSecureSessionWithDeadline(ctx, mockSess)

	resultCh := make(chan struct{})
	var transportRes connection.Transport
	var svcRes connection.Crypto
	var errRes error

	go func() {
		transportRes, svcRes, errRes = wrapper.Establish()
		close(resultCh)
	}()

	// Cancel immediately
	cancel()

	select {
	case <-resultCh:
		// returned as expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Establish did not return after context cancellation")
	}

	if transportRes != nil {
		t.Errorf("expected nil transportRes after cancel, got %v", transportRes)
	}
	if svcRes != nil {
		t.Errorf("expected nil crypto after cancel, got %v", svcRes)
	}
	if !errors.Is(errRes, context.Canceled) {
		t.Errorf("expected error context.Canceled, got %v", errRes)
	}
}
