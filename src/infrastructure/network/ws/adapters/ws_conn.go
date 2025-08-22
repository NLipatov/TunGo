package adapters

import (
	"context"
	"io"

	"github.com/coder/websocket"
)

// compile-time assertion that *websocket.Conn implements WSConn
var _ WSConn = (*websocket.Conn)(nil)

// WSConn abstracts github.com/coder/websocket.Conn used by Adapter.
type WSConn interface {
	Reader(ctx context.Context) (websocket.MessageType, io.Reader, error)
	Writer(ctx context.Context, typ websocket.MessageType) (io.WriteCloser, error)
	Close(status websocket.StatusCode, reason string) error
}
