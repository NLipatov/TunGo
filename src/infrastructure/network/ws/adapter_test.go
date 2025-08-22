package ws

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// helper to spin up a websocket server with a custom handler fn.
// The handler gets a *websocket.Conn and a context bound to the request.
func withWSServer(t *testing.T, fn func(ctx context.Context, c *websocket.Conn)) (url string, closeFn func()) {
	t.Helper()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			// Compression or subprotocols can be configured here if needed.
		})
		if err != nil {
			// If the upgrade fails, just return; the test will fail on the client side.
			return
		}
		defer func(c *websocket.Conn, code websocket.StatusCode, reason string) {
			_ = c.Close(code, reason)
		}(c, websocket.StatusNormalClosure, "")

		ctx := r.Context()
		fn(ctx, c)
	}))
	t.Cleanup(s.Close)

	// Convert http://127.0.0.1:port to ws://127.0.0.1:port
	return "ws" + strings.TrimPrefix(s.URL, "http"), s.Close
}

func dialClient(t *testing.T, url string) (*websocket.Conn, context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		cancel()
		t.Fatalf("websocket.Dial: %v", err)
	}
	return c, ctx, cancel
}

func TestAdapter_WriteReadBinary_Echo(t *testing.T) {
	const payload = "hello over ws"

	// Server echos back any binary message it receives.
	url, _ := withWSServer(t, func(ctx context.Context, c *websocket.Conn) {
		for {
			mt, r, err := c.Reader(ctx)
			if err != nil {
				return
			}
			if mt != websocket.MessageBinary {
				_, _ = io.Copy(io.Discard, r)
				continue
			}
			b, _ := io.ReadAll(r)
			if err := c.Write(ctx, websocket.MessageBinary, b); err != nil {
				return
			}
		}
	})

	c, dialCtx, cancel := dialClient(t, url)
	defer cancel()
	a := NewAdapter(dialCtx, c)

	// Write via adapter
	if n, err := a.Write([]byte(payload)); err != nil || n != len(payload) {
		t.Fatalf("adapter.Write: n=%d err=%v", n, err)
	}

	// Read echo via adapter
	buf := make([]byte, 64)
	n, err := a.Read(buf)
	if err != nil {
		t.Fatalf("adapter.Read: %v", err)
	}
	got := string(buf[:n])
	if got != payload {
		t.Fatalf("echo mismatch: got %q want %q", got, payload)
	}
}

func TestAdapter_Read_MultipleBinaryFrames(t *testing.T) {
	msg1 := []byte("first")
	msg2 := []byte("second-longer")

	// Server sends two binary frames, then returns.
	url, _ := withWSServer(t, func(ctx context.Context, c *websocket.Conn) {
		_ = c.Write(ctx, websocket.MessageBinary, msg1)
		_ = c.Write(ctx, websocket.MessageBinary, msg2)
	})

	c, dialCtx, cancel := dialClient(t, url)
	defer cancel()
	a := NewAdapter(dialCtx, c)

	buf := make([]byte, 64)

	// First frame
	n, err := a.Read(buf)
	if err != nil {
		t.Fatalf("read #1: %v", err)
	}
	if string(buf[:n]) != string(msg1) {
		t.Fatalf("frame #1 mismatch: got %q want %q", string(buf[:n]), string(msg1))
	}

	// Second frame
	n, err = a.Read(buf)
	if err != nil {
		t.Fatalf("read #2: %v", err)
	}
	if string(buf[:n]) != string(msg2) {
		t.Fatalf("frame #2 mismatch: got %q want %q", string(buf[:n]), string(msg2))
	}
}

func TestAdapter_Read_SkipsTextFrames(t *testing.T) {
	bin := []byte("only-binary-please")

	// Server sends a text frame, then a binary frame.
	url, _ := withWSServer(t, func(ctx context.Context, c *websocket.Conn) {
		_ = c.Write(ctx, websocket.MessageText, []byte("ignore me"))
		_ = c.Write(ctx, websocket.MessageBinary, bin)
	})

	c, dialCtx, cancel := dialClient(t, url)
	defer cancel()
	a := NewAdapter(dialCtx, c)

	buf := make([]byte, 64)
	n, err := a.Read(buf)
	if err != nil {
		t.Fatalf("adapter.Read: %v", err)
	}
	if string(buf[:n]) != string(bin) {
		t.Fatalf("got %q want %q", string(buf[:n]), string(bin))
	}
}

func TestAdapter_ReadDeadline_Expired(t *testing.T) {
	// Server does nothing; client read should time out.
	url, _ := withWSServer(t, func(ctx context.Context, c *websocket.Conn) {
		<-ctx.Done()
	})

	c, dialCtx, cancel := dialClient(t, url)
	defer cancel()
	a := NewAdapter(dialCtx, c)

	// Set a past deadline → should cause context deadline exceeded on Reader().
	_ = a.SetReadDeadline(time.Now().Add(-100 * time.Millisecond))

	buf := make([]byte, 1)
	_, err := a.Read(buf)
	if err == nil {
		t.Fatal("expected read deadline error, got nil")
	}
	if !errorsIsDeadline(err) {
		t.Fatalf("expected deadline error, got %v", err)
	}
}

func TestAdapter_WriteDeadline_Expired(t *testing.T) {
	// Server accepts but doesn't read; write should respect deadline.
	url, _ := withWSServer(t, func(ctx context.Context, c *websocket.Conn) {
		<-ctx.Done()
	})

	c, dialCtx, cancel := dialClient(t, url)
	defer cancel()
	a := NewAdapter(dialCtx, c)

	_ = a.SetWriteDeadline(time.Now().Add(-100 * time.Millisecond))

	n, err := a.Write([]byte("payload"))
	if err == nil {
		t.Fatal("expected write deadline error, got nil")
	}
	// n may be 0 or partial depending on underlying behavior; we only assert the error kind.
	if !errorsIsDeadline(err) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	_ = n
}

func TestAdapter_Read_ReturnsEOF_OnNormalClose(t *testing.T) {
	// Server immediately closes with StatusNormalClosure.
	url, _ := withWSServer(t, func(ctx context.Context, c *websocket.Conn) {
		_ = c.Close(websocket.StatusNormalClosure, "")
	})

	c, dialCtx, cancel := dialClient(t, url)
	defer cancel()
	a := NewAdapter(dialCtx, c)

	buf := make([]byte, 1)
	_, err := a.Read(buf)
	if err == nil {
		t.Fatal("expected EOF after normal close, got nil")
	}
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

// helper to check deadline-ish errors without importing net.Error.
func errorsIsDeadline(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}
