package factory

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestServerFactory_NewDefaultServerFactory(t *testing.T) {
	f := newDefaultServerFactory()
	if f.readHeaderTimeout != defaultReadHeaderTimeout {
		t.Fatalf("readHeaderTimeout = %v, want %v", f.readHeaderTimeout, defaultReadHeaderTimeout)
	}
	if f.idleTimeout != defaultIdleTimeout {
		t.Fatalf("idleTimeout = %v, want %v", f.idleTimeout, defaultIdleTimeout)
	}
	if f.shutdownTimeout != defaultShutdownTimeout {
		t.Fatalf("shutdownTimeout = %v, want %v", f.shutdownTimeout, defaultShutdownTimeout)
	}
	if f.path != defaultPath {
		t.Fatalf("path = %q, want %q", f.path, defaultPath)
	}
}

func TestServerFactory_NewServerFactory_WithCustomValues(t *testing.T) {
	rh, it, st := 10*time.Millisecond, 20*time.Millisecond, 30*time.Millisecond
	p := "/custom"
	f := newServerFactory(rh, it, st, p)
	if f.readHeaderTimeout != rh {
		t.Fatalf("readHeaderTimeout = %v, want %v", f.readHeaderTimeout, rh)
	}
	if f.idleTimeout != it {
		t.Fatalf("idleTimeout = %v, want %v", f.idleTimeout, it)
	}
	if f.shutdownTimeout != st {
		t.Fatalf("shutdownTimeout = %v, want %v", f.shutdownTimeout, st)
	}
	if f.path != p {
		t.Fatalf("path = %q, want %q", f.path, p)
	}
}

func TestServerFactory_NewServer_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen error: %v", err)
	}
	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)

	q := make(chan net.Conn, 1)
	ctx := context.Background()

	f := newServerFactory(5*time.Millisecond, 5*time.Millisecond, 5*time.Millisecond, "/ws")
	srv, err := f.NewServer(ctx, ln, q)
	if err != nil {
		t.Fatalf("NewServer error: %v", err)
	}
	// Сервер ещё не запущен (Serve не вызывали), Shutdown должен пройти по ветке server==nil.
	if err := srv.Shutdown(); err != nil {
		t.Fatalf("Shutdown error: %v", err)
	}
}

func TestServerFactory_NewServer_InvalidPath_Error(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen error: %v", err)
	}
	defer func(ln net.Listener) {
		_ = ln.Close()
	}(ln)

	q := make(chan net.Conn, 1)
	ctx := context.Background()

	f := newServerFactory(1*time.Millisecond, 1*time.Millisecond, 1*time.Millisecond, "")
	if _, err := f.NewServer(ctx, ln, q); err == nil {
		t.Fatalf("expected error for invalid path, got nil")
	}
}
