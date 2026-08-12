package ws

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"github.com/coder/websocket"
)

type socket interface {
	Reader(context.Context) (websocket.MessageType, io.Reader, error)
	Writer(context.Context, websocket.MessageType) (io.WriteCloser, error)
	Close(websocket.StatusCode, string) error
}

var _ net.Conn = (*Conn)(nil)

// Conn adapts a WebSocket connection to net.Conn.
type Conn struct {
	conn                        socket
	ctx                         context.Context
	reader                      io.Reader
	lAddr                       net.Addr
	rAddr                       net.Addr
	readDeadline, writeDeadline time.Time
}

func NewConn(
	ctx context.Context,
	conn *websocket.Conn,
	lAddr, rAddr net.Addr,
) *Conn {
	return &Conn{
		ctx:   ctx,
		conn:  conn,
		lAddr: lAddr,
		rAddr: rAddr,
	}
}

func (a *Conn) Write(data []byte) (written int, err error) {
	if len(data) == 0 {
		return 0, nil
	}

	var ctx context.Context
	var cancel context.CancelFunc
	if !a.writeDeadline.IsZero() {
		ctx, cancel = context.WithDeadline(a.ctx, a.writeDeadline)
	} else {
		ctx, cancel = a.ctx, func() {}
	}
	defer cancel()

	writer, err := a.conn.Writer(ctx, websocket.MessageBinary)
	if err != nil {
		return 0, a.mapWriteErr(err)
	}

	defer func() {
		closeErr := writer.Close()
		if err == nil && closeErr != nil {
			err = a.mapWriteErr(closeErr)
		}
	}()

	for written < len(data) {
		n, writeErr := writer.Write(data[written:])
		written += n
		if writeErr != nil {
			return written, a.mapWriteErr(writeErr)
		}
		if n == 0 {
			return written, io.ErrNoProgress
		}
	}

	return written, nil
}

// Read reads from the current binary WebSocket frame (or fetches the next one).
// Non-binary frames are drained. EOF at frame boundary does not bubble up.
func (a *Conn) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}

	for {
		if a.reader != nil {
			n, err := a.reader.Read(buf)
			switch err {
			case nil:
				if n == 0 {
					return 0, io.ErrNoProgress
				}
				return n, nil
			case io.EOF:
				a.reader = nil
				if n > 0 {
					return n, nil
				}
				continue // next frame
			default:
				a.reader = nil
				return n, a.mapReadErr(err)
			}
		}

		// per-frame context; DO NOT defer cancel here
		var ctx context.Context
		var cancel context.CancelFunc
		if !a.readDeadline.IsZero() {
			ctx, cancel = context.WithDeadline(a.ctx, a.readDeadline)
		} else {
			ctx, cancel = a.ctx, func() {}
		}

		mt, r, err := a.conn.Reader(ctx)
		if err != nil {
			cancel()
			return 0, a.mapReadErr(err)
		}

		if mt != websocket.MessageBinary {
			// drain non-binary under the same ctx
			_, _ = io.Copy(io.Discard, r)
			cancel()
			continue
		}

		// keep cancel with the reader; it will be called exactly once on EOF/error
		a.reader = &cancelOnEOF{r: r, cancel: cancel}
	}
}

// cancelOnEOF wraps a frame reader and calls cancel() once when any non-nil error
// (including io.EOF) is returned. This ties the frameCtx lifetime to the frame itself.
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

func (a *Conn) Close() error {
	return a.conn.Close(websocket.StatusNormalClosure, "")
}

func (a *Conn) LocalAddr() net.Addr {
	if a.lAddr != nil {
		return a.lAddr
	}
	return &net.TCPAddr{}
}

func (a *Conn) RemoteAddr() net.Addr {
	if a.rAddr != nil {
		return a.rAddr
	}
	return &net.TCPAddr{}
}

func (a *Conn) SetDeadline(deadline time.Time) error {
	a.readDeadline = deadline
	a.writeDeadline = deadline
	return nil
}

func (a *Conn) SetReadDeadline(deadline time.Time) error {
	a.readDeadline = deadline
	return nil
}

func (a *Conn) SetWriteDeadline(deadline time.Time) error {
	a.writeDeadline = deadline
	return nil
}

// mapReadErr normalizes read-side errors to net.Conn semantics.
func (a *Conn) mapReadErr(err error) error {
	if err == nil {
		return nil
	}
	// Map graceful WS close to io.EOF (as net.Conn Read would do).
	if ce, ok := errors.AsType[*websocket.CloseError](err); ok {
		switch ce.Code {
		case websocket.StatusNormalClosure, websocket.StatusGoingAway:
			return io.EOF
		case websocket.StatusAbnormalClosure, websocket.StatusNoStatusRcvd:
			return io.ErrUnexpectedEOF
		}
		// other close codes: return as-is for caller to diagnose
		return err
	}
	return err
}

// mapWriteErr normalizes write-side errors to net.Conn semantics.
func (a *Conn) mapWriteErr(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*websocket.CloseError](err); ok {
		// For writes after close, most net.Conn impls return net.ErrClosed.
		return net.ErrClosed
	}
	return err
}
