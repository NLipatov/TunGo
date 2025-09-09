package server

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"tungo/infrastructure/network/ws"

	"tungo/infrastructure/settings"

	"github.com/coder/websocket"
)

// helper timeout to avoid hanging tests
const testTimeout = 2 * time.Second

// TestUpgrader_Upgrade_Success_ReadLimitEnforced verifies that:
// 1) Upgrade succeeds on a proper WS handshake;
// 2) the connection enforces the read limit set by Upgrader (SafeMTU + overhead).
func TestUpgrader_Upgrade_Success_ReadLimitEnforced(t *testing.T) {
	u := NewDefaultUpgrader()

	errCh := make(chan error, 1)

	// HTTP handler that uses Upgrader and then tries to read a frame.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := u.Upgrade(w, r)
		if err != nil {
			errCh <- err
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

		// Try to read a single frame; since client will send > limit, this should error.
		ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
		defer cancel()

		_, rdr, err := conn.Reader(ctx)
		if err != nil {
			// If the library rejects at Reader() boundary due to headers/length, we are done.
			errCh <- nil
			return
		}
		// Drain the frame; exceeding the configured limit must yield an error while reading.
		_, err = io.Copy(io.Discard, rdr)
		if err == nil {
			errCh <- errors.New("expected read error due to read-limit, got nil")
			return
		}
		errCh <- nil
	}))
	defer srv.Close()

	// Dial as a WebSocket client.
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	wsURL := "ws" + srv.URL[len("http"):] // http:// -> ws://
	c, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("client Dial failed: %v", err)
	}
	defer func() { _ = c.Close(websocket.StatusNormalClosure, "") }()

	// Send a frame strictly larger than the server read limit.
	limit := settings.DefaultEthernetMTU + settings.TCPChacha20Overhead
	payload := make([]byte, limit+8)

	wr, err := c.Writer(ctx, websocket.MessageBinary)
	if err != nil {
		t.Fatalf("client Writer failed: %v", err)
	}
	if _, err = wr.Write(payload); err != nil {
		t.Fatalf("client write failed: %v", err)
	}
	if err = wr.Close(); err != nil {
		t.Fatalf("client writer close failed: %v", err)
	}

	select {
	case e := <-errCh:
		if e != nil {
			t.Fatalf("server side reported error: %v", e)
		}
	case <-time.After(testTimeout):
		t.Fatal("server did not report read-limit outcome in time")
	}
}

// TestUpgrader_Upgrade_BadRequest_Error verifies that Upgrade returns an error
// when the request is not a valid websocket handshake.
func TestUpgrader_Upgrade_BadRequest_Error(t *testing.T) {
	u := NewDefaultUpgrader()

	req := httptest.NewRequest(http.MethodGet, "http://example.org/ws", nil)
	// Intentionally omit Upgrade headers to force Accept() error.
	rr := httptest.NewRecorder()

	conn, err := u.Upgrade(rr, req)
	if err == nil {
		_ = conn.Close(websocket.StatusNormalClosure, "")
		t.Fatalf("expected error on non-WS request, got nil")
	}
}

// compile-time sanity: returned type implements the expected interface in tests too.
var _ ws.Conn = (*websocket.Conn)(nil)
