package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// -------------------- Test doubles (mocks/stubs) --------------------

// ListenerMockHandler is a minimal Handler impl for tests.
type ListenerMockHandler struct{}

func (h *ListenerMockHandler) Handle(_ http.ResponseWriter, _ *http.Request) {
	// no-op: we don't exercise HTTP path in unit tests
}

// ListenerMockAddr is a trivial net.Addr.
type ListenerMockAddr string

func (a ListenerMockAddr) Network() string { return "tcp" }
func (a ListenerMockAddr) String() string  { return string(a) }

// ListenerMockErrorListener: Accept() immediately returns a non-temporary error.
// Useful to make http.Server.Serve exit right away with that error.
type ListenerMockErrorListener struct {
	closed bool
	addr   net.Addr
	err    error
}

func NewListenerMockErrorListener(err error) *ListenerMockErrorListener {
	return &ListenerMockErrorListener{
		addr: ListenerMockAddr("127.0.0.1:0"),
		err:  err,
	}
}

func (m *ListenerMockErrorListener) Accept() (net.Conn, error) { return nil, m.err }
func (m *ListenerMockErrorListener) Close() error              { m.closed = true; return nil }
func (m *ListenerMockErrorListener) Addr() net.Addr            { return m.addr }

// ListenerMockBlockingListener: Accept() blocks until Close() is called,
// then returns a non-temporary error to make Serve exit.
type ListenerMockBlockingListener struct {
	closeOnce sync.Once
	closedCh  chan struct{}
	addr      net.Addr
	err       error
}

func NewListenerMockBlockingListener(returnErr error) *ListenerMockBlockingListener {
	if returnErr == nil {
		returnErr = errors.New("mock: listener closed")
	}
	return &ListenerMockBlockingListener{
		closedCh: make(chan struct{}),
		addr:     ListenerMockAddr("127.0.0.1:0"),
		err:      returnErr,
	}
}

func (m *ListenerMockBlockingListener) Accept() (net.Conn, error) {
	<-m.closedCh
	return nil, m.err
}

func (m *ListenerMockBlockingListener) Close() error {
	m.closeOnce.Do(func() { close(m.closedCh) })
	return nil
}

func (m *ListenerMockBlockingListener) Addr() net.Addr { return m.addr }

// ListenerMockTrackCloseListener tracks whether Close() was called.
// Accept() should never be hit in tests that don't Start().
type ListenerMockTrackCloseListener struct {
	mu     sync.Mutex
	closed bool
	addr   net.Addr
}

func NewListenerMockTrackCloseListener() *ListenerMockTrackCloseListener {
	return &ListenerMockTrackCloseListener{addr: ListenerMockAddr("127.0.0.1:0")}
}

func (m *ListenerMockTrackCloseListener) Accept() (net.Conn, error) {
	return nil, errors.New("unexpected Accept()")
}
func (m *ListenerMockTrackCloseListener) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}
func (m *ListenerMockTrackCloseListener) Addr() net.Addr { return m.addr }

func (m *ListenerMockTrackCloseListener) WasClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// -------------------- Helpers --------------------

func mustCtx(t *testing.T, d time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), d)
}

func waitClosed(t *testing.T, l *Listener, d time.Duration) {
	t.Helper()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-l.closed:
		return
	case <-timer.C:
		t.Fatalf("timeout waiting for closed")
	}
}

// -------------------- Tests --------------------

// NewDefaultListener auto-starts and Serve() returns an error which is captured.
// Accept() then returns net.ErrClosed; Close() is idempotent.
func TestListener_NewDefault_AutoStart_ServeError_ThenAcceptErrClosed(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	ln := NewListenerMockErrorListener(boom)
	ctx, cancel := mustCtx(t, 2*time.Second)
	defer cancel()

	tcpL, err := NewDefaultListener(ctx, ln)
	if err != nil {
		t.Fatalf("NewDefaultListener error: %v", err)
	}
	l := tcpL.(*Listener)

	// Wait until Serve loop signals closed.
	waitClosed(t, l, time.Second)

	// serveError is non-blocking; should return our boom error.
	if err := l.serveError(); err == nil || !errors.Is(err, boom) {
		t.Fatalf("serveError() = %v, want %v", err, boom)
	}

	// Accept after closed -> net.ErrClosed.
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept() err = %v, want net.ErrClosed", err)
	}

	// Close idempotent.
	if err := l.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close() idempotent error: %v", err)
	}
}

// Start/Close idempotency; branch where error buffer is already full (default case),
// and Serve goroutine finds l.closed already closed (guarded close path).
func TestListener_StartClose_Idempotent_BufferAlreadyFull_GuardedClose(t *testing.T) {
	t.Parallel()

	ctx, cancel := mustCtx(t, 2*time.Second)
	defer cancel()

	sentinel := errors.New("sentinel in buffer")
	ln := NewListenerMockBlockingListener(errors.New("serve exit")) // any non-temporary error
	h := &ListenerMockHandler{}
	queue := make(chan net.Conn, 1)

	// Build without auto-start.
	tcpL, err := NewListener(ctx, ln, "/ws", h, queue, 50*time.Millisecond, 50*time.Millisecond, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}
	l := tcpL.(*Listener)

	// Prefill error buffer so Serve's send takes the default branch.
	l.httpServerServeErr <- sentinel

	// Start twice -> idempotent
	if err := l.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	if err := l.Start(); err != nil {
		t.Fatalf("Start() idempotent error: %v", err)
	}

	// Close BEFORE Serve returns to hit "guarded close" inside Serve goroutine.
	if err := l.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	// Second Close -> idempotent.
	if err := l.Close(); err != nil {
		t.Fatalf("Close() idempotent error: %v", err)
	}

	// closed must be signaled.
	select {
	case <-l.closed:
	default:
		t.Fatalf("expected closed to be signaled")
	}

	// serveError should still return the sentinel (buffer already full).
	if err := l.serveError(); err == nil || !errors.Is(err, sentinel) {
		t.Fatalf("serveError() = %v, want sentinel", err)
	}
}

// Close() before Start(): exercises shutdown() branch with httpServer == nil.
// Ensures underlying net.Listener.Close() is called and Accept() returns net.ErrClosed.
func TestListener_CloseBeforeStart_ShutdownWithoutHttpServer(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ln := NewListenerMockTrackCloseListener()
	h := &ListenerMockHandler{}
	queue := make(chan net.Conn, 1)

	tcpL, err := NewListener(ctx, ln, "/ws", h, queue, time.Second, time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}
	l := tcpL.(*Listener)

	// Close before Start.
	if err := l.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if !ln.WasClosed() {
		t.Fatalf("underlying listener was not closed in shutdown() path")
	}

	// Accept now returns net.ErrClosed.
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept() after early Close err = %v, want net.ErrClosed", err)
	}
}

// Accept() happy path: enqueued conn is returned; then after Close() Accept() returns net.ErrClosed.
func TestListener_Accept_ReturnsQueuedConn_ThenErrClosed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ln := NewListenerMockTrackCloseListener()
	h := &ListenerMockHandler{}
	queue := make(chan net.Conn, 2)

	tcpL, err := NewListener(ctx, ln, "/ws", h, queue, time.Second, time.Second, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}
	l := tcpL.(*Listener)

	// Enqueue a net.Pipe conn (no need to Start()).
	c1, c2 := net.Pipe()
	defer func(c2 net.Conn) {
		_ = c2.Close()
	}(c2)
	queue <- c1

	got, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept() error: %v", err)
	}
	if got != c1 {
		t.Fatalf("Accept() returned unexpected conn")
	}
	_ = got.Close()

	// After Close(), Accept returns net.ErrClosed.
	if err := l.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept() after Close err = %v, want net.ErrClosed", err)
	}
}

// serveError() default branch: channel is empty -> returns nil.
func TestListener_ServeError_DefaultNil(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ln := NewListenerMockTrackCloseListener()
	h := &ListenerMockHandler{}
	queue := make(chan net.Conn, 1)

	tcpL, err := NewListener(ctx, ln, "/ws", h, queue, time.Second, time.Second, time.Second)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}
	l := tcpL.(*Listener)

	if err := l.serveError(); err != nil {
		t.Fatalf("serveError() = %v, want nil", err)
	}
}
