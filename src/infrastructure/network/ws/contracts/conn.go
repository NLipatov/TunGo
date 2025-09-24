package contracts

import (
	"context"
	"io"

	"github.com/coder/websocket"
)

// Conn abstracts github.com/coder/websocket.Conn used by Adapter.
type Conn interface {
	Reader(ctx context.Context) (websocket.MessageType, io.Reader, error)
	Writer(ctx context.Context, typ websocket.MessageType) (io.WriteCloser, error)
	Close(status websocket.StatusCode, reason string) error
}
