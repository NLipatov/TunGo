package server

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- test doubles ---

type mockServer struct {
	doneCh         chan struct{}
	serveCalled    atomic.Int64
	shutdownCalled atomic.Int64

	errMu sync.Mutex
	err   error         // returned by Err()
	shErr error         // returned by Shutdown()
	sigCh chan struct{} // optional: signal that Serve() started
}

func newMockServer() *mockServer {
	return &mockServer{
		doneCh: make(chan struct{}),
		sigCh:  make(chan struct{}, 1),
	}
}

func (m *mockServer) Serve() error {
	m.serveCalled.Add(1)
	// signal that Serve started
	select {
	case m.sigCh <- struct{}{}:
	default:
	}
	// block until Shutdown() closes doneCh
	<-m.doneCh
	return nil
}

func (m *mockServer) Shutdown() error {
	m.shutdownCalled.Add(1)
	select {
	case <-m.doneCh:
		// already closed
	default:
		close(m.doneCh)
	}
	return m.shErr
}

func (m *mockServer) Done() <-chan struct{} { return m.doneCh }

func (m *mockServer) Err() error {
	m.errMu.Lock()
	defer m.errMu.Unlock()
	return m.err
}

func (m *mockServer) setErr(e error) {
	m.errMu.Lock()
	m.err = e
	m.errMu.Unlock()
}

// --- helpers ---

func mustPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	c1, c2 := net.Pipe()
	return c1, c2
}

// --- tests ---

func TestNewListener_Guards(t *testing.T) {
	t.Parallel()

	s := newMockServer()
	q := make(chan net.Conn, 1)

	if _, err := NewListener(nil, s, q); err == nil || err.Error() != "ctx must not be nil" {
		t.Fatalf("expected ctx guard error, got %v", err)
	}
	if _, err := NewListener(context.Background(), nil, q); err == nil || err.Error() != "server must not be nil" {
		t.Fatalf("expected server guard error, got %v", err)
	}
	if _, err := NewListener(context.Background(), s, nil); err == nil || err.Error() != "queue must not be nil" {
		t.Fatalf("expected queue guard error, got %v", err)
	}
}

func TestNewListener_Happy_AutoStart(t *testing.T) {
	s := newMockServer()
	q := make(chan net.Conn, 1)

	lst, err := NewListener(context.Background(), s, q)
	if err != nil {
		t.Fatalf("NewListener err=%v", err)
	}

	// Wait for Serve() to have started
	select {
	case <-s.sigCh:
	case <-time.After(time.Second):
		t.Fatal("Serve() did not start")
	}

	// Close to unblock Serve()
	if err := lst.Close(); err != nil {
		t.Fatalf("Close err=%v", err)
	}

	if got := s.serveCalled.Load(); got != 1 {
		t.Fatalf("Serve called %d times, want 1", got)
	}
}

func TestListener_Start_IsIdempotent(t *testing.T) {
	s := newMockServer()
	q := make(chan net.Conn, 1)
	lst, err := NewListener(context.Background(), s, q)
	if err != nil {
		t.Fatalf("NewListener err=%v", err)
	}

	// Start already called by constructor; call again
	lst.(*Listener).Start()
	lst.(*Listener).Start()

	// Let goroutines schedule
	select {
	case <-s.sigCh:
	case <-time.After(time.Second):
		t.Fatal("Serve() did not start")
	}

	_ = lst.Close()

	if got := s.serveCalled.Load(); got != 1 {
		t.Fatalf("Serve called %d times, want 1", got)
	}
}

func TestListener_Close_IsIdempotent_PropagatesFirstError(t *testing.T) {
	s := newMockServer()
	s.shErr = errors.New("boom")
	q := make(chan net.Conn, 1)
	lst, err := NewListener(context.Background(), s, q)
	if err != nil {
		t.Fatalf("NewListener err=%v", err)
	}

	err1 := lst.Close()
	if !errors.Is(err1, s.shErr) {
		t.Fatalf("first Close err=%v, want %v", err1, s.shErr)
	}
	err2 := lst.Close()
	if err2 != nil {
		t.Fatalf("second Close err=%v, want nil", err2)
	}
	if got := s.shutdownCalled.Load(); got != 1 {
		t.Fatalf("Shutdown called %d times, want 1", got)
	}
}

func TestAccept_ReturnsQueuedConn(t *testing.T) {
	s := newMockServer()
	q := make(chan net.Conn, 1)
	lst, err := NewListener(context.Background(), s, q)
	if err != nil {
		t.Fatalf("NewListener err=%v", err)
	}

	c1, c2 := mustPipe(t)
	defer func(c2 net.Conn) {
		_ = c2.Close()
	}(c2)

	q <- c1

	conn, accErr := lst.Accept()
	if accErr != nil {
		t.Fatalf("Accept err=%v", accErr)
	}
	if conn != c1 {
		t.Fatalf("Accept returned unexpected conn")
	}
	_ = conn.Close()
	_ = lst.Close()
}

func TestAccept_ContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := newMockServer()
	q := make(chan net.Conn, 1)
	lst, err := NewListener(ctx, s, q)
	if err != nil {
		t.Fatalf("NewListener err=%v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, e := lst.Accept()
		done <- e
	}()

	// Ensure Accept is blocking, then cancel context
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case e := <-done:
		if !errors.Is(e, net.ErrClosed) {
			t.Fatalf("Accept err=%v, want net.ErrClosed", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Accept after ctx cancel")
	}
	_ = lst.Close()
}

func TestAccept_ServerDone_WithErr(t *testing.T) {
	s := newMockServer()
	s.setErr(errors.New("serve failed"))
	q := make(chan net.Conn, 1)
	lst, err := NewListener(context.Background(), s, q)
	if err != nil {
		t.Fatalf("NewListener err=%v", err)
	}

	// Close server -> Done() readable, Err() non-nil
	_ = s.Shutdown()

	_, accErr := lst.Accept()
	if !errors.Is(accErr, s.Err()) {
		t.Fatalf("Accept err=%v, want %v", accErr, s.Err())
	}
	_ = lst.Close()
}

func TestAccept_ServerDone_NoErr(t *testing.T) {
	s := newMockServer()
	// Err() returns nil by default
	q := make(chan net.Conn, 1)
	lst, err := NewListener(context.Background(), s, q)
	if err != nil {
		t.Fatalf("NewListener err=%v", err)
	}

	_ = s.Shutdown() // Done() readable, Err()==nil

	_, accErr := lst.Accept()
	if !errors.Is(accErr, net.ErrClosed) {
		t.Fatalf("Accept err=%v, want net.ErrClosed", accErr)
	}
	_ = lst.Close()
}

func TestAccept_ClosedQueue(t *testing.T) {
	s := newMockServer()
	q := make(chan net.Conn, 1)
	lst, err := NewListener(context.Background(), s, q)
	if err != nil {
		t.Fatalf("NewListener err=%v", err)
	}

	close(q) // reading from closed channel must return net.ErrClosed

	_, accErr := lst.Accept()
	if !errors.Is(accErr, net.ErrClosed) {
		t.Fatalf("Accept err=%v, want net.ErrClosed", accErr)
	}
	_ = lst.Close()
}

func TestAccept_NilConnValue(t *testing.T) {
	s := newMockServer()
	q := make(chan net.Conn, 1)
	lst, err := NewListener(context.Background(), s, q)
	if err != nil {
		t.Fatalf("NewListener err=%v", err)
	}

	q <- nil

	_, accErr := lst.Accept()
	if !errors.Is(accErr, net.ErrClosed) {
		t.Fatalf("Accept err=%v, want net.ErrClosed", accErr)
	}
	_ = lst.Close()
}
