package adapter

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"time"
	"tungo/application"
	"tungo/infrastructure/network/ws"

	"github.com/coder/websocket"
)

var (
	// Compile-time checks
	_ net.Conn                      = &Adapter{}
	_ application.ConnectionAdapter = &Adapter{}
	_ ws.Conn                       = &websocket.Conn{}
)

type Adapter struct {
	conn                        ws.Conn
	ctx                         context.Context
	errorMapper                 errorMapper
	currentReader               io.Reader // current in-progress binary frame currentReader (wrapped)
	readDeadline, writeDeadline atomic.Int64
	lAddr                       net.Addr
	rAddr                       net.Addr
}

func NewAdapter(ctx context.Context, conn ws.Conn, lAddr, rAddr net.Addr) *Adapter {
	adapter := &Adapter{
		ctx:         ctx,
		conn:        conn,
		errorMapper: defaultErrorMapper{},
		lAddr:       lAddr,
		rAddr:       rAddr,
	}

	adapter.readDeadline.Store(0)
	adapter.writeDeadline.Store(0)
	return adapter
}

func (a *Adapter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	ctx, cancel := a.writeCtx()
	defer cancel()

	w, err := a.conn.Writer(ctx, websocket.MessageBinary)
	if err != nil {
		return 0, a.errorMapper.mapErr(err)
	}

	closed := false
	defer func() {
		if !closed {
			_ = w.Close()
		}
	}()

	var n int
	for n < len(p) {
		m, wErr := w.Write(p[n:])
		n += m
		if wErr != nil {
			return n, a.errorMapper.mapErr(wErr)
		}
	}

	if cErr := w.Close(); cErr != nil {
		return n, a.errorMapper.mapErr(cErr)
	}
	closed = true
	return n, nil
}

func (a *Adapter) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if a.currentReader != nil {
			n, err := a.currentReader.Read(p)
			switch err {
			case nil:
				return n, nil
			case io.EOF:
				// frame fully consumed
				a.currentReader = nil
				if n > 0 {
					return n, nil
				}
				continue // fetch next frame
			default:
				a.currentReader = nil
				return n, a.errorMapper.mapErr(err)
			}
		}

		ctx, cancel := a.readCtx()
		mt, r, err := a.conn.Reader(ctx)
		if err != nil {
			cancel() // failed to get frame; release ctx
			return 0, a.errorMapper.mapErr(err)
		}

		if mt != websocket.MessageBinary {
			// Drain non-binary frames to keep the protocol healthy.
			_, _ = io.Copy(io.Discard, r)
			cancel()
			continue
		}

		// Keep the frame context alive until EOF/error via wrapper.
		a.currentReader = &cancelOnEOF{r: r, cancel: cancel}
	}
}

func (a *Adapter) Close() error {
	return a.conn.Close(websocket.StatusNormalClosure, "")
}

func (a *Adapter) LocalAddr() net.Addr {
	if a.lAddr != nil {
		return a.lAddr
	}
	return &net.TCPAddr{}
}

func (a *Adapter) RemoteAddr() net.Addr {
	if a.rAddr != nil {
		return a.rAddr
	}
	return &net.TCPAddr{}
}

func (a *Adapter) SetDeadline(t time.Time) error {
	a.storeDeadline(&a.readDeadline, t)
	a.storeDeadline(&a.writeDeadline, t)
	return nil
}

func (a *Adapter) SetReadDeadline(t time.Time) error {
	a.storeDeadline(&a.readDeadline, t)
	return nil
}

func (a *Adapter) SetWriteDeadline(t time.Time) error {
	a.storeDeadline(&a.writeDeadline, t)
	return nil
}

func (a *Adapter) storeDeadline(dst *atomic.Int64, t time.Time) {
	if t.IsZero() {
		dst.Store(0)
		return
	}
	dst.Store(t.UnixNano())
}

// cancelOnEOF wraps a currentReader and calls cancel() exactly once
// when any non-nil error (including io.EOF) is returned.
type cancelOnEOF struct {
	r      io.Reader
	cancel context.CancelFunc
	done   bool
}

func (c *cancelOnEOF) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if err != nil && !c.done {
		c.cancel()
		c.done = true
	}
	return n, err
}

func (a *Adapter) readCtx() (context.Context, context.CancelFunc) {
	if t, ok := a.loadDeadline(&a.readDeadline); ok {
		return context.WithDeadline(a.ctx, t)
	}
	return a.ctx, func() {}
}

func (a *Adapter) writeCtx() (context.Context, context.CancelFunc) {
	if t, ok := a.loadDeadline(&a.writeDeadline); ok {
		return context.WithDeadline(a.ctx, t)
	}
	return a.ctx, func() {}
}

func (a *Adapter) loadDeadline(src *atomic.Int64) (time.Time, bool) {
	ns := src.Load()
	if ns == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, ns), true
}
