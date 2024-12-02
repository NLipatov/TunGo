package transportconf_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
	"tungo/client/transportconf"
	"tungo/handshake/ChaCha20"
)

type MockConn struct{}

func (m *MockConn) Read(_ []byte) (n int, err error) {
	return 0, nil
}
func (m *MockConn) Write(_ []byte) (n int, err error) {
	return 0, nil
}
func (m *MockConn) Close() error {
	return nil
}
func (m *MockConn) LocalAddr() net.Addr {
	return nil
}
func (m *MockConn) RemoteAddr() net.Addr {
	return nil
}
func (m *MockConn) SetDeadline(_ time.Time) error {
	return nil
}
func (m *MockConn) SetReadDeadline(_ time.Time) error {
	return nil
}
func (m *MockConn) SetWriteDeadline(_ time.Time) error {
	return nil
}

// Checks that ConnectionManager is returning a connection if connection was created by connection delegate
func TestEstablishConnectionWithRetry_Success(t *testing.T) {
	mockSession := &ChaCha20.Session{}
	mockConn := &MockConn{}

	manager := &transportconf.ConnectionManager{
		ConnectionDelegate: func() (net.Conn, *ChaCha20.Session, error) {
			return mockConn, mockSession, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, session, err := manager.EstablishConnectionWithRetry(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conn != mockConn {
		t.Fatalf("expected mockConn, got %v", conn)
	}

	if session != mockSession {
		t.Fatalf("expected mockSession, got %v", session)
	}
}

// Checks that ConnectionManager is aborting connection creation if deadline exceeded
func TestEstablishConnectionWithRetry_RetryAndFail(t *testing.T) {
	manager := &transportconf.ConnectionManager{
		ConnectionDelegate: func() (net.Conn, *ChaCha20.Session, error) {
			return nil, nil, errors.New("mock connection error")
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, session, err := manager.EstablishConnectionWithRetry(ctx)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if conn != nil {
		t.Fatalf("expected nil connection, got %v", conn)
	}

	if session != nil {
		t.Fatalf("expected nil session, got %v", session)
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

// Checks if ConnectionManager can cancel connection creation if ctx is cancelled
func TestEstablishConnectionWithRetry_CancelContext(t *testing.T) {
	manager := &transportconf.ConnectionManager{
		ConnectionDelegate: func() (net.Conn, *ChaCha20.Session, error) {
			time.Sleep(5 * time.Second)
			return nil, nil, errors.New("mock connection error")
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(time.Second)
		cancel()
	}()

	conn, session, err := manager.EstablishConnectionWithRetry(ctx)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if conn != nil {
		t.Fatalf("expected nil connection, got %v", conn)
	}

	if session != nil {
		t.Fatalf("expected nil session, got %v", session)
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// Checks if manager ConnectionManager is trying to reconnect on connection failures
// Mocked Connection will return an errors first 2 times
func TestEstablishConnectionWithRetry_SuccessAfterRetries(t *testing.T) {
	attempts := 0
	mockSession := &ChaCha20.Session{}
	mockConn := &MockConn{}

	manager := &transportconf.ConnectionManager{
		ConnectionDelegate: func() (net.Conn, *ChaCha20.Session, error) {
			attempts++
			if attempts < 3 {
				return nil, nil, errors.New("mock connection error")
			}
			return mockConn, mockSession, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, session, err := manager.EstablishConnectionWithRetry(ctx)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conn != mockConn {
		t.Fatalf("expected mockConn, got %v", conn)
	}

	if session != mockSession {
		t.Fatalf("expected mockSession, got %v", session)
	}

	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}
