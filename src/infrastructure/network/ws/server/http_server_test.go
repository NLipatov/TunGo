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

// -------------------- Test doubles --------------------

// HttpServerMockAddr implements net.Addr.
type HttpServerMockAddr string

func (a HttpServerMockAddr) Network() string { return "tcp" }
func (a HttpServerMockAddr) String() string  { return string(a) }

// HttpServerMockErrorListener: Accept returns an error immediately; Close marks closed.
type HttpServerMockErrorListener struct {
	closed bool
	addr   net.Addr
	err    error
}

func NewHttpServerMockErrorListener(err error) *HttpServerMockErrorListener {
	return &HttpServerMockErrorListener{addr: HttpServerMockAddr("127.0.0.1:0"), err: err}
}
func (m *HttpServerMockErrorListener) Accept() (net.Conn, error) { return nil, m.err }
func (m *HttpServerMockErrorListener) Close() error              { m.closed = true; return nil }
func (m *HttpServerMockErrorListener) Addr() net.Addr            { return m.addr }

// HttpServerMockBlockingListener: Accept blocks until Close is called, then returns err.
type HttpServerMockBlockingListener struct {
	closeOnce sync.Once
	closedCh  chan struct{}
	addr      net.Addr
	err       error
}

func NewHttpServerMockBlockingListener(returnErr error) *HttpServerMockBlockingListener {
	if returnErr == nil {
		returnErr = errors.New("mock: listener closed")
	}
	return &HttpServerMockBlockingListener{
		closedCh: make(chan struct{}),
		addr:     HttpServerMockAddr("127.0.0.1:0"),
		err:      returnErr,
	}
}
func (m *HttpServerMockBlockingListener) Accept() (net.Conn, error) {
	<-m.closedCh
	return nil, m.err
}
func (m *HttpServerMockBlockingListener) Close() error {
	m.closeOnce.Do(func() { close(m.closedCh) })
	return nil
}
func (m *HttpServerMockBlockingListener) Addr() net.Addr { return m.addr }

// HttpServerMockHandler is a no-op Handler.
type HttpServerMockHandler struct{}

func (h *HttpServerMockHandler) Handle(http.ResponseWriter, *http.Request) {}

// -------------------- Helpers --------------------

func mustCtx(t *testing.T, d time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), d)
}

// -------------------- Tests --------------------

func TestNewHttpServer_ValidationErrors(t *testing.T) {
	ln := NewHttpServerMockErrorListener(errors.New("x"))
	h := &HttpServerMockHandler{}

	if _, err := newHttpServer(nil, ln, time.Second, time.Second, time.Second, h, "/ws"); err == nil {
		t.Fatal("want error on nil context")
	}
	if _, err := newHttpServer(context.Background(), nil, time.Second, time.Second, time.Second, h, "/ws"); err == nil {
		t.Fatal("want error on nil listener")
	}
	if _, err := newHttpServer(context.Background(), ln, time.Second, time.Second, time.Second, nil, "/ws"); err == nil {
		t.Fatal("want error on nil handler")
	}
	if _, err := newHttpServer(context.Background(), ln, time.Second, time.Second, time.Second, h, "ws"); err == nil {
		t.Fatal("want error on invalid path (no leading slash)")
	}
	if _, err := newHttpServer(context.Background(), ln, time.Second, time.Second, 0, h, "/ws"); err == nil {
		t.Fatal("want error on non-positive shutdownTimeout")
	}
}

func TestHttpServer_Serve_FirstThenSecondCall(t *testing.T) {
	boom := errors.New("accept failed")
	ln := NewHttpServerMockErrorListener(boom)
	h := &HttpServerMockHandler{}
	ctx, cancel := mustCtx(t, 2*time.Second)
	defer cancel()

	s, err := newHttpServer(ctx, ln, 10*time.Millisecond, 10*time.Millisecond, 100*time.Millisecond, h, "/ws")
	if err != nil {
		t.Fatalf("newHttpServer err: %v", err)
	}

	// Pre-fill error buffer so Serve's send takes the default branch.
	sentinel := errors.New("sentinel")
	s.serveErrChan <- sentinel

	// First Serve should finish with the Accept error and close s.closed.
	if err := s.Serve(); !errors.Is(err, boom) {
		t.Fatalf("Serve() err=%v, want %v", err, boom)
	}
	select {
	case <-s.Done():
		// ok
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for Done()")
	}

	// Err() should return the sentinel we injected earlier.
	if got := s.Err(); !errors.Is(got, sentinel) {
		t.Fatalf("Err()=%v, want sentinel", got)
	}

	// Second Serve must return ErrAlreadyRunning.
	if err := s.Serve(); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("Serve() second err=%v, want ErrAlreadyRunning", err)
	}
}

func TestHttpServer_Shutdown_BeforeServe_ClosesListener(t *testing.T) {
	ln := NewHttpServerMockErrorListener(errors.New("accept failed"))
	h := &HttpServerMockHandler{}
	ctx := context.Background()

	s, err := newHttpServer(ctx, ln, time.Second, time.Second, 100*time.Millisecond, h, "/ws")
	if err != nil {
		t.Fatalf("newHttpServer err: %v", err)
	}
	// server == nil branch in Shutdown: must close underlying listener.
	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown err: %v", err)
	}
	if !ln.closed {
		t.Fatal("listener.Close() was not called in server==nil Shutdown path")
	}
}

func TestHttpServer_ContextCancel_TriggersShutdown(t *testing.T) {
	ln := NewHttpServerMockBlockingListener(errors.New("listener closed"))
	h := &HttpServerMockHandler{}
	ctx, cancel := context.WithCancel(context.Background())

	s, err := newHttpServer(ctx, ln, 5*time.Millisecond, 5*time.Millisecond, 100*time.Millisecond, h, "/ws")
	if err != nil {
		t.Fatalf("newHttpServer err: %v", err)
	}

	// Run Serve in a goroutine (it will block on Accept()).
	doneServe := make(chan error, 1)
	go func() {
		doneServe <- s.Serve()
	}()

	// Cancel the context to trigger internal Shutdown().
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-s.Done():
		// ok
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Done() not closed after context cancel")
	}

	// Second Shutdown should be idempotent.
	if err := s.Shutdown(); err != nil {
		t.Fatalf("second Shutdown err: %v", err)
	}

	// Err() should return nil when nothing was sent to serveErrChan.
	if err := s.Err(); err != nil {
		t.Fatalf("Err()=%v, want nil", err)
	}

	// Drain Serve result (may be nil; just ensure it returned).
	select {
	case <-doneServe:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Serve did not exit after shutdown")
	}
}
