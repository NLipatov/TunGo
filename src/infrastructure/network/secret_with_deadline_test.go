package network

import (
	"context"
	"errors"
	"testing"
	"time"
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

// secretWithDeadlineTestMockSecret implements Secret for testing.
// If block==true, Exchange will hang forever; otherwise returns svc, err.
type secretWithDeadlineTestMockSecret struct {
	svc   connection.Crypto
	ctrl  *rekey.StateMachine
	err   error
	block bool
}

func (m *secretWithDeadlineTestMockSecret) Exchange(_ connection.Transport) (connection.Crypto, *rekey.StateMachine, error) {
	if m.block {
		select {} // hang
	}
	return m.svc, m.ctrl, m.err
}

// secretWithDeadlineTestMockConn is a no-op Transport.
type secretWithDeadlineTestMockConn struct{}

func (m *secretWithDeadlineTestMockConn) Write([]byte) (int, error) { return 0, nil }
func (m *secretWithDeadlineTestMockConn) Read([]byte) (int, error)  { return 0, nil }
func (m *secretWithDeadlineTestMockConn) Close() error              { return nil }

// secretWithDeadlineTestMockCrypto is a dummy cryptographyService.
type secretWithDeadlineTestMockCrypto struct{}

func (m *secretWithDeadlineTestMockCrypto) Encrypt(p []byte) ([]byte, error) { return p, nil }
func (m *secretWithDeadlineTestMockCrypto) Decrypt(p []byte) ([]byte, error) { return p, nil }

// TestSecretWithDeadline_Success ensures that when the underlying Secret returns
// immediately, Exchange returns that service_packet with no error.
func TestSecretWithDeadline_Success(t *testing.T) {
	ctx := context.Background()
	fakeSvc := &secretWithDeadlineTestMockCrypto{}
	underlying := &secretWithDeadlineTestMockSecret{svc: fakeSvc, err: nil, block: false}
	wrapper := NewSecretWithDeadline(ctx, underlying)
	svc, ctrl, err := wrapper.Exchange(&secretWithDeadlineTestMockConn{})

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

// TestSecretWithDeadline_ErrorPropagation ensures that if the underlying Secret
// returns an error immediately, Exchange propagates that error and a nil service_packet.
func TestSecretWithDeadline_ErrorPropagation(t *testing.T) {
	wantErr := errors.New("underlying failure")
	underlying := &secretWithDeadlineTestMockSecret{svc: nil, err: wantErr, block: false}
	wrapper := NewSecretWithDeadline(context.Background(), underlying)
	svc, _, err := wrapper.Exchange(&secretWithDeadlineTestMockConn{})

	if svc != nil {
		t.Errorf("expected nil service_packet on error, got %v", svc)
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected error %v, got %v", wantErr, err)
	}
}

// TestSecretWithDeadline_Cancel ensures that if the context is canceled before
// the underlying Secret returns, Exchange returns context.Canceled and a nil service_packet.
func TestSecretWithDeadline_Cancel(t *testing.T) {
	underlying := &secretWithDeadlineTestMockSecret{block: true}
	ctx, cancel := context.WithCancel(context.Background())
	wrapper := NewSecretWithDeadline(ctx, underlying)

	var svcRes connection.Crypto
	var ctrlRes *rekey.StateMachine
	var errRes error
	done := make(chan struct{})

	go func() {
		svcRes, ctrlRes, errRes = wrapper.Exchange(&secretWithDeadlineTestMockConn{})
		close(done)
	}()

	// Cancel immediately
	cancel()

	select {
	case <-done:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Exchange did not return after context cancellation")
	}

	if svcRes != nil {
		t.Errorf("expected nil service_packet on cancel, got %v", svcRes)
	}
	if ctrlRes != nil {
		t.Errorf("expected nil controller on cancel, got %v", ctrlRes)
	}
	if !errors.Is(errRes, context.Canceled) {
		t.Errorf("expected context.Canceled error, got %v", errRes)
	}
}
