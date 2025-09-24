package factory

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"tungo/infrastructure/network/ws/server/contracts"
)

type mockServer struct {
	done    chan struct{}
	serveN  atomic.Int64
	shutN   atomic.Int64
	started chan struct{}
}

func newMockServer() *mockServer {
	return &mockServer{
		done:    make(chan struct{}),
		started: make(chan struct{}, 1),
	}
}

func (m *mockServer) Serve() error {
	m.serveN.Add(1)
	select {
	case m.started <- struct{}{}:
	default:
	}
	<-m.done
	return nil
}
func (m *mockServer) Shutdown() error {
	m.shutN.Add(1)
	select {
	case <-m.done:
	default:
		close(m.done)
	}
	return nil
}
func (m *mockServer) Done() <-chan struct{} { return m.done }
func (m *mockServer) Err() error            { return nil }

type mockServerFactory struct {
	mu     sync.Mutex
	srv    *mockServer
	retErr error
}

func (f *mockServerFactory) NewServer(_ context.Context, _ net.Listener, _ chan net.Conn) (contracts.Server, error) {
	if f.retErr != nil {
		return nil, f.retErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.srv == nil {
		f.srv = newMockServer()
	}
	return f.srv, nil
}

func TestNewDefaultListenerFactory_NotNil(t *testing.T) {
	lf := NewDefaultListenerFactory()
	if lf == nil || lf.serverFactory == nil {
		t.Fatal("expected non-nil default listener factory and serverFactory")
	}
}

func TestListenerFactory_NewListener_Success_StartsServerAndCloses(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen error: %v", err)
	}
	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)

	sf := &mockServerFactory{}
	lf := NewListenerFactory(sf)

	l, err := lf.NewListener(context.Background(), ln)
	if err != nil {
		t.Fatalf("NewListener error: %v", err)
	}

	select {
	case <-sf.srv.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Serve() did not start")
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	if got := sf.srv.serveN.Load(); got != 1 {
		t.Fatalf("Serve called %d, want 1", got)
	}
	if got := sf.srv.shutN.Load(); got != 1 {
		t.Fatalf("Shutdown called %d, want 1", got)
	}
}

func TestListenerFactory_NewListener_PropagatesServerFactoryError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen error: %v", err)
	}
	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)

	sf := &mockServerFactory{retErr: errors.New("boom")}
	lf := NewListenerFactory(sf)

	l, e := lf.NewListener(context.Background(), ln)
	if e == nil || e.Error() != "boom" {
		t.Fatalf("expected factory error 'boom', got: %v", e)
	}
	if l != nil {
		t.Fatalf("expected nil listener on error, got: %#v", l)
	}
}
