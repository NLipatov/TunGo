package ws

import (
	"context"
	"io"
	"net/http"

	"github.com/coder/websocket"
)

// Conn abstracts github.com/coder/websocket.Conn used by Adapter.
type Conn interface {
	Reader(ctx context.Context) (websocket.MessageType, io.Reader, error)
	Writer(ctx context.Context, typ websocket.MessageType) (io.WriteCloser, error)
	Close(status websocket.StatusCode, reason string) error
}

// Upgrader — upgrades HTTP to WebSocket and returns Conn.
type Upgrader interface {
	Upgrade(w http.ResponseWriter, r *http.Request) (Conn, error)
}
