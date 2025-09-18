package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"tungo/infrastructure/network/ws"

	"github.com/coder/websocket"
)

// --- fakes -------------------------------------------------------------------

type fakeUpgrader struct {
	conn ws.Conn
	err  error
}

func (f *fakeUpgrader) Upgrade(_ http.ResponseWriter, _ *http.Request) (ws.Conn, error) {
	return f.conn, f.err
}

type fakeConn struct {
	closed bool
	code   websocket.StatusCode
	reason string
}

func (c *fakeConn) Reader(_ context.Context) (websocket.MessageType, io.Reader, error) {
	return websocket.MessageBinary, nil, io.EOF
}
func (c *fakeConn) Writer(_ context.Context, _ websocket.MessageType) (io.WriteCloser, error) {
	return nopWriteCloser{}, nil
}
func (c *fakeConn) Close(code websocket.StatusCode, reason string) error {
	c.closed = true
	c.code = code
	c.reason = reason
	return nil
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

type fakeLogger struct{ msgs []string }

func (l *fakeLogger) Printf(format string, v ...any) {
	l.msgs = append(l.msgs, fmt.Sprintf(format, v...))
}

// make sure fakeConn satisfies the contract
var _ ws.Conn = (*fakeConn)(nil)

// --- tests -------------------------------------------------------------------

func TestHandler_BadRemoteAddr_400_NilLogger_NoPanic(t *testing.T) {
	h := NewDefaultHandler(&fakeUpgrader{}, make(chan net.Conn, 1), nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example/ws", nil)
	req.RemoteAddr = "not-a-socket-addr"

	h.Handle(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
	if got := rr.Body.String(); got == "" || !contains(got, "bad remote addr") {
		t.Fatalf("expected body to mention bad remote addr, got %q", got)
	}
}

func TestHandler_UpgradeError_Logged_NoEnqueue(t *testing.T) {
	q := make(chan net.Conn, 1)
	log := &fakeLogger{}
	h := NewDefaultHandler(&fakeUpgrader{err: fmt.Errorf("boom")}, q, log)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example/ws", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	h.Handle(rr, req)

	select {
	case c := <-q:
		t.Fatalf("unexpected enqueue: %#v", c)
	default:
		// ok, nothing enqueued
	}
	if len(log.msgs) == 0 || !contains(log.msgs[0], "upgrade failed") {
		t.Fatalf("expected upgrade error to be logged, got %v", log.msgs)
	}
	// handler doesn't write a response on upgrade error; default 200 is fine
	if rr.Code != 200 {
		t.Fatalf("unexpected code %d", rr.Code)
	}
}

func TestHandler_Success_Enqueue_WithLocalAddr(t *testing.T) {
	q := make(chan net.Conn, 1)
	h := NewDefaultHandler(&fakeUpgrader{conn: &fakeConn{}}, q, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example/ws", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	// inject server's local address into context
	local := &net.TCPAddr{IP: net.IPv4(9, 9, 9, 9), Port: 8443}
	req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, net.Addr(local)))

	h.Handle(rr, req)

	select {
	case c := <-q:
		// Adapter is a net.Conn; verify metadata we set via WithAddrs.
		if got := c.LocalAddr().String(); got != local.String() {
			t.Fatalf("LocalAddr mismatch: got %q want %q", got, local.String())
		}
		if got := c.RemoteAddr().String(); got != "1.2.3.4:5678" {
			t.Fatalf("RemoteAddr mismatch: got %q", got)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected a connection enqueued")
	}
}

func TestHandler_Overflow_ClosesWsConn(t *testing.T) {
	// unbuffered channel => send would block => default branch taken
	q := make(chan net.Conn)
	fc := &fakeConn{}
	h := NewDefaultHandler(&fakeUpgrader{conn: fc}, q, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example/ws", nil)
	req.RemoteAddr = "127.0.0.1:2222"

	h.Handle(rr, req)

	if !fc.closed {
		t.Fatal("expected ws conn to be closed on overflow")
	}
	if fc.code != CloseCodeQueueFull {
		t.Fatalf("close code mismatch: got %d want %d", fc.code, CloseCodeQueueFull)
	}
	if fc.reason == "" || !contains(fc.reason, "could not accept") {
		t.Fatalf("close reason mismatch: %q", fc.reason)
	}
}

func TestHandler_BadRemoteAddr_InvalidIP_400_Body(t *testing.T) {
	h := NewDefaultHandler(&fakeUpgrader{}, make(chan net.Conn, 1), nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example/ws", nil)
	req.RemoteAddr = "bad.ip.addr:1234" // invalid IP, can not be parsed

	h.Handle(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
	if got := rr.Body.String(); !contains(got, "bad remote addr") {
		t.Fatalf("expected body to mention bad remote addr, got %q", got)
	}
}

func TestHandler_BadRemoteAddr_InvalidPort_400_Body(t *testing.T) {
	h := NewDefaultHandler(&fakeUpgrader{}, make(chan net.Conn, 1), nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example/ws", nil)
	req.RemoteAddr = "127.0.0.1:notaport" // invalid port, can not be parsed

	h.Handle(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
	if got := rr.Body.String(); !contains(got, "bad remote addr") {
		t.Fatalf("expected body to mention bad remote addr, got %q", got)
	}
}

// --- helpers -----------------------------------------------------------------

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && indexOf(s, sub) >= 0))
}
func indexOf(s, sub string) int { return strings.Index(s, sub) }
