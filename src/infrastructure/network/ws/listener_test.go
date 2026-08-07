//go:build !js

package ws

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"tungo/infrastructure/settings"

	"github.com/coder/websocket"
)

type testAddr string

func (a testAddr) Network() string { return "tcp" }
func (a testAddr) String() string  { return string(a) }

type blockingListener struct {
	closed    chan struct{}
	closeOnce sync.Once
	err       error
}

func newBlockingListener() *blockingListener {
	return &blockingListener{closed: make(chan struct{}), err: net.ErrClosed}
}

func (l *blockingListener) Accept() (net.Conn, error) {
	<-l.closed
	return nil, l.err
}

func (l *blockingListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (*blockingListener) Addr() net.Addr { return testAddr("127.0.0.1:8080") }

type errorListener struct{ err error }

func (l errorListener) Accept() (net.Conn, error) { return nil, l.err }
func (errorListener) Close() error                { return nil }
func (errorListener) Addr() net.Addr              { return testAddr("127.0.0.1:8080") }

func TestNewListenerValidation(t *testing.T) {
	listener := newBlockingListener()

	//nolint:staticcheck // This test verifies that a nil context is rejected.
	if _, err := NewListener(nil, listener); err == nil {
		t.Fatal("expected nil context error")
	}
	if _, err := NewListener(context.Background(), nil); err == nil {
		t.Fatal("expected nil listener error")
	}
}

func TestListenerAcceptReturnsServeError(t *testing.T) {
	want := errors.New("accept failed")
	listener, err := NewListener(context.Background(), errorListener{err: want})
	if err != nil {
		t.Fatalf("NewListener() error = %v", err)
	}

	if _, err := listener.Accept(); !errors.Is(err, want) {
		t.Fatalf("Accept() error = %v, want %v", err, want)
	}
}

func TestListenerContextCancellationClosesAccept(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	listener, err := NewListener(ctx, newBlockingListener())
	if err != nil {
		t.Fatalf("NewListener() error = %v", err)
	}

	cancel()
	if _, err := listener.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("Accept() error = %v, want net.ErrClosed", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestListenerRejectsBadRemoteAddress(t *testing.T) {
	l := newHandlerListener(make(chan net.Conn, 1))
	request := httptest.NewRequest(http.MethodGet, "http://example/ws", nil)
	request.RemoteAddr = "bad-address"
	response := httptest.NewRecorder()

	l.handle(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestListenerEnqueuesConnection(t *testing.T) {
	queue := make(chan net.Conn, 1)
	l := newHandlerListener(queue)
	server := httptest.NewServer(http.HandlerFunc(l.handle))
	defer server.Close()

	client := dialTestWebSocket(t, server.URL)
	conn := <-queue
	defer func() {
		_ = client.CloseNow()
		_ = conn.Close()
	}()

	if conn.LocalAddr() == nil || conn.RemoteAddr() == nil {
		t.Fatalf("connection addresses must be populated: local=%v remote=%v", conn.LocalAddr(), conn.RemoteAddr())
	}
}

func TestListenerClosesConnectionWhenQueueIsFull(t *testing.T) {
	l := newHandlerListener(make(chan net.Conn))
	server := httptest.NewServer(http.HandlerFunc(l.handle))
	defer server.Close()

	client := dialTestWebSocket(t, server.URL)
	defer func() { _ = client.CloseNow() }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, err := client.Read(ctx)
	if websocket.CloseStatus(err) != queueFullCloseCode {
		t.Fatalf("Read() error = %v, want close code %d", err, queueFullCloseCode)
	}
}

func TestUpgradeEnforcesReadLimit(t *testing.T) {
	result := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&Listener{}).upgrade(w, r)
		if err != nil {
			result <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		_, reader, err := conn.Reader(r.Context())
		if err == nil {
			_, err = io.Copy(io.Discard, reader)
		}
		if err == nil {
			err = errors.New("expected read limit error")
		}
		result <- err
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, response, err := websocket.Dial(ctx, "ws"+server.URL[len("http"):], nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer func() { _ = conn.CloseNow() }()

	payload := make([]byte, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead+1)
	if err := conn.Write(ctx, websocket.MessageBinary, payload); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("expected server read error")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for read limit error")
	}
}

func newHandlerListener(queue chan net.Conn) *Listener {
	return &Listener{
		ctx:      context.Background(),
		listener: newBlockingListener(),
		queue:    queue,
	}
}

func dialTestWebSocket(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	conn, response, err := websocket.Dial(t.Context(), "ws"+serverURL[len("http"):], nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	return conn
}
