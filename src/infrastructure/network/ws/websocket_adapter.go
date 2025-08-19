//go:build !js

package ws

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/coder/websocket"
	"tungo/application"
)

type WebsocketAdapter struct {
	conn *websocket.Conn
	ctx  context.Context

	cur io.Reader
	wmu sync.Mutex
}

func NewWebsocketAdapter(ctx context.Context, conn *websocket.Conn) application.ConnectionAdapter {
	if ctx == nil {
		ctx = context.Background()
	}
	return &WebsocketAdapter{conn: conn, ctx: ctx}
}

func (a *WebsocketAdapter) Write(p []byte) (int, error) {
	a.wmu.Lock()
	defer a.wmu.Unlock()

	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()

	wr, err := a.conn.Writer(ctx, websocket.MessageBinary)
	if err != nil {
		return 0, a.mapErr(err)
	}
	n, writeErr := wr.Write(p)
	closeErr := wr.Close()

	if writeErr != nil {
		return n, a.mapErr(writeErr)
	}
	if closeErr != nil {
		return n, a.mapErr(closeErr)
	}
	return n, nil
}

func (a *WebsocketAdapter) Read(p []byte) (int, error) {
	for {
		if a.cur != nil {
			n, err := a.cur.Read(p)
			if err == io.EOF {
				a.cur = nil
				if n > 0 {
					return n, nil
				}
				continue
			}
			return n, a.mapErr(err)
		}

		mt, r, err := a.conn.Reader(a.ctx)
		if err != nil {
			return 0, a.mapErr(err)
		}
		if mt != websocket.MessageBinary {
			_, _ = io.Copy(io.Discard, r)
			continue
		}
		a.cur = r
	}
}

func (a *WebsocketAdapter) Close() error {
	return a.conn.Close(websocket.StatusNormalClosure, "")
}

func (a *WebsocketAdapter) mapErr(err error) error {
	if err == nil {
		return nil
	}
	switch websocket.CloseStatus(err) {
	case websocket.StatusNormalClosure, websocket.StatusGoingAway:
		return io.EOF
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	return err
}
