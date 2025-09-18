package server

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// -------------------- Test doubles --------------------

// ListenerMockServer implements the `server` interface for tests.
type ListenerMockServer struct {
	serveCalled atomic.Int64
	serveErr    error

	shutdownCalled atomic.Int64
	shutdownErr    error

	doneCh chan struct{}

	mu  sync.Mutex
	err error // value for Err()
}

func NewListenerMockServer() *ListenerMockServer {
	return &ListenerMockServer{
		doneCh: make(chan struct{}),
	}
}

func (m *ListenerMockServer) Serve() error {
	m.serveCalled.Add(1)
	<-m.doneCh // block until Shutdown() closes doneCh
	return m.serveErr
}

func (m *ListenerMockServer) Shutdown() error {
	m.shutdownCalled.Add(1)
	select {
	case <-m.doneCh: // already closed
	default:
		close(m.doneCh)
	}
	return m.shutdownErr
}

func (m *ListenerMockServer) Done() <-chan struct{} { return m.doneCh }

func (m *ListenerMockServer) Err() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}

func (m *ListenerMockServer) setErr(err error) {
	m.mu.Lock()
	m.err = err
	m.mu.Unlock()
}

// newPipeConn returns a pair of connected net.Conns.
func newPipeConn(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	c1, c2 := net.Pipe()
	return c1, c2
}

// -------------------- Tests --------------------

// Start must be idempotent and call Serve only once even if Start() called multiple times.
func TestListener_Start_IsIdempotent(t *testing.T) {
	ms := NewListenerMockServer()
	l, err := NewListener(nil, make(chan net.Conn, 1), ms)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}

	// Call Start twice; Serve should be invoked once.
	l.(*Listener).Start()
	l.(*Listener).Start()

	// Let goroutine schedule.
	time.Sleep(30 * time.Millisecond)

	if got := ms.serveCalled.Load(); got != 1 {
		t.Fatalf("Serve called %d times, want 1", got)
	}

	// Close to unblock Serve.
	if err := l.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
}

// Accept must return a queued connection (happy path).
func TestListener_Accept_ReturnsQueuedConn(t *testing.T) {
	ms := NewListenerMockServer()
	queue := make(chan net.Conn, 1)
	l, err := NewListener(nil, queue, ms)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}
	l.(*Listener).Start()

	c1, c2 := newPipeConn(t)
	defer func(c2 net.Conn) {
		_ = c2.Close()
	}(c2)
	queue <- c1

	got, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept error: %v", err)
	}
	if got != c1 {
		t.Fatalf("returned unexpected conn")
	}
	_ = got.Close()

	_ = l.Close()
}

// If server.Done() is closed and server.Err() returns a non-nil error,
// Accept must return that error (not net.ErrClosed).
func TestListener_Accept_AfterServerDone_ReturnsServerErr(t *testing.T) {
	ms := NewListenerMockServer()
	l, err := NewListener(nil, make(chan net.Conn, 1), ms)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}
	l.(*Listener).Start()

	want := errors.New("serve failed")
	ms.setErr(want)
	_ = ms.Shutdown() // signal Done()

	_, accErr := l.Accept()
	if !errors.Is(accErr, want) {
		t.Fatalf("Accept err = %v, want %v", accErr, want)
	}
}

// If server.Done() is closed and server.Err() == nil,
// Accept must return net.ErrClosed.
func TestListener_Accept_AfterServerDone_ReturnsNetErrClosed(t *testing.T) {
	ms := NewListenerMockServer()
	l, err := NewListener(nil, make(chan net.Conn, 1), ms)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}
	l.(*Listener).Start()

	ms.setErr(nil)
	_ = ms.Shutdown()

	_, accErr := l.Accept()
	if !errors.Is(accErr, net.ErrClosed) {
		t.Fatalf("Accept err = %v, want net.ErrClosed", accErr)
	}
}

// Close must be idempotent and call underlying Shutdown once (thanks to Listener.closeOnce).
func TestListener_Close_IsIdempotent(t *testing.T) {
	ms := NewListenerMockServer()
	l, err := NewListener(nil, make(chan net.Conn, 1), ms)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}
	l.(*Listener).Start()

	if err := l.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close second error: %v", err)
	}

	if got := ms.shutdownCalled.Load(); got != 1 {
		t.Fatalf("Shutdown called %d times, want 1", got)
	}
}
