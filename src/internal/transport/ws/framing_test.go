package ws

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"tungo/internal/config/settings"
	"tungo/internal/transport/tcp"
)

// Capture incoming ciphertext below the test server's TLS connection.
type recordListener struct {
	net.Listener
	mu   sync.Mutex
	data bytes.Buffer
}

func (l *recordListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &recordConn{Conn: conn, listener: l}, nil
}

type recordConn struct {
	net.Conn
	listener *recordListener
}

func (c *recordConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.listener.mu.Lock()
	_, _ = c.listener.data.Write(p[:n])
	c.listener.mu.Unlock()
	return n, err
}

func TestFramedConn_WebSocketMessagesAndLegacyPeer(t *testing.T) {
	// Existing servers limit a message to the payload cap, without the u16 prefix.
	const legacyLimit = settings.DefaultEthernetMTU + settings.TCPChacha20Overhead
	for _, size := range []int{1, 64, legacyLimit - 2, legacyLimit - 1, legacyLimit} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			payload := bytes.Repeat([]byte{0xa5}, size)
			wire := make([]byte, 2+size)
			binary.BigEndian.PutUint16(wire, uint16(size))
			copy(wire[2:], payload)
			type received struct {
				sizes []int
				err   error
			}
			result := make(chan received, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				peer, err := websocket.Accept(w, r, nil)
				if err != nil {
					result <- received{err: err}
					return
				}
				defer func() { _ = peer.CloseNow() }()
				peer.SetReadLimit(legacyLimit)
				var got []byte
				var sizes []int
				for len(got) < len(wire) {
					kind, message, err := peer.Read(ctx)
					if err != nil {
						result <- received{err: err}
						return
					}
					if kind != websocket.MessageBinary {
						result <- received{err: fmt.Errorf("message type = %v", kind)}
						return
					}
					sizes = append(sizes, len(message))
					got = append(got, message...)
				}
				if !bytes.Equal(got, wire) {
					result <- received{err: fmt.Errorf("framing bytes changed: got %x, want %x", got, wire)}
					return
				}
				// Reply in the old format: prefix and payload in separate messages.
				err = peer.Write(ctx, websocket.MessageBinary, wire[:2])
				if err == nil {
					err = peer.Write(ctx, websocket.MessageBinary, wire[2:])
				}
				result <- received{sizes: sizes, err: err}
			}))
			recorder := &recordListener{Listener: server.Listener}
			server.Listener = recorder
			server.StartTLS()
			defer server.Close()
			conn, _, err := websocket.Dial(ctx, "wss"+strings.TrimPrefix(server.URL, "https"), &websocket.DialOptions{HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.CloseNow() }()
			framed, err := tcp.NewFramedConn(NewConn(ctx, conn, nil, nil), legacyLimit)
			if err != nil {
				t.Fatal(err)
			}
			// The handshake and HTTP upgrade have completed; record only frame writes.
			recorder.mu.Lock()
			recorder.data.Reset()
			recorder.mu.Unlock()
			if n, err := framed.Write(payload); err != nil || n != size {
				t.Fatalf("Write = %d, %v", n, err)
			}
			got := make([]byte, legacyLimit)
			if n, err := framed.Read(got); err != nil || !bytes.Equal(got[:n], payload) {
				t.Fatalf("reading legacy reply: n=%d err=%v", n, err)
			}
			response := <-result
			if response.err != nil {
				t.Fatal(response.err)
			}
			wantMessages := (len(wire) + legacyLimit - 1) / legacyLimit
			if len(response.sizes) != wantMessages {
				t.Errorf("WebSocket message sizes = %v, want %d messages", response.sizes, wantMessages)
			}
			for _, n := range response.sizes {
				if n < len(wire)/wantMessages || n > legacyLimit {
					t.Errorf("unbalanced or oversized WebSocket message: sizes = %v", response.sizes)
				}
			}
			recorder.mu.Lock()
			ciphertext := bytes.Clone(recorder.data.Bytes())
			recorder.mu.Unlock()
			var recordSizes []int
			for len(ciphertext) > 0 {
				if len(ciphertext) < 5 {
					t.Fatal("truncated TLS record header")
				}
				n := 5 + int(binary.BigEndian.Uint16(ciphertext[3:5]))
				if n > len(ciphertext) {
					t.Fatal("truncated TLS record")
				}
				if ciphertext[0] != 23 {
					t.Fatalf("unexpected TLS record type %d", ciphertext[0])
				}
				recordSizes = append(recordSizes, n)
				ciphertext = ciphertext[n:]
			}
			if len(recordSizes) != wantMessages {
				t.Errorf("TLS record sizes = %v, want %d records", recordSizes, wantMessages)
			}
		})
	}
}
