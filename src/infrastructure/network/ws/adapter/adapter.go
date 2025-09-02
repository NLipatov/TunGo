package adapter

import (
	"context"
	"errors"
	"io"
	"net"
	"time"
	"tungo/infrastructure/network/ws"

	"github.com/coder/websocket"
)

// Adapter is a ws.Conn adaptation to net.Conn
type Adapter struct {
	conn                        ws.Conn
	ctx                         context.Context
	reader                      io.Reader
	lAddr                       net.Addr
	rAddr                       net.Addr
	readDeadline, writeDeadline time.Time
}

func NewDefaultAdapter(
	ctx context.Context,
	conn ws.Conn,
	lAddr, rAddr net.Addr,
) *Adapter {
	return &Adapter{
		ctx:           ctx,
		conn:          conn,
		lAddr:         lAddr,
		rAddr:         rAddr,
		readDeadline:  time.Time{},
		writeDeadline: time.Time{},
	}
}

func NewAdapter(
	ctx context.Context,
	conn ws.Conn,
	reader io.Reader,
	lAddr, rAddr net.Addr,
	readDeadline, writeDeadline time.Time,
) *Adapter {
	return &Adapter{
		ctx:           ctx,
		conn:          conn,
		reader:        reader,
		lAddr:         lAddr,
		rAddr:         rAddr,
		readDeadline:  readDeadline,
		writeDeadline: writeDeadline,
	}
}

func (a *Adapter) Write(data []byte) (int, error) {
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

	writer, writerErr := a.conn.Writer(ctx, websocket.MessageBinary)
	if writerErr != nil {
		return 0, a.mapWriteErr(writerErr)
	}

	closed := false
	defer func() {
		if !closed {
			_ = writer.Close()
		}
	}()

	var written int
	for written < len(data) {
		n, wErr := writer.Write(data[written:])
		if n == 0 && wErr == nil {
			return written, io.ErrNoProgress
		}
		written += n
		if wErr != nil {
			return written, a.mapWriteErr(wErr)
		}
	}

	if cErr := writer.Close(); cErr != nil {
		return written, a.mapWriteErr(cErr)
	}
	closed = true
	return written, nil
}

// Read reads from the current binary WebSocket frame (or fetches the next one).
// Non-binary frames are drained. EOF at frame boundary does not bubble up.
func (a *Adapter) Read(buf []byte) (int, error) {
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

func (a *Adapter) SetDeadline(deadline time.Time) error {
	a.readDeadline = deadline
	a.writeDeadline = deadline
	return nil
}

func (a *Adapter) SetReadDeadline(deadline time.Time) error {
	a.readDeadline = deadline
	return nil
}

func (a *Adapter) SetWriteDeadline(deadline time.Time) error {
	a.writeDeadline = deadline
	return nil
}

// mapReadErr normalizes read-side errors to net.Conn semantics.
func (a *Adapter) mapReadErr(err error) error {
	if err == nil {
		return nil
	}
	// Map graceful WS close to io.EOF (as net.Conn Read would do).
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		switch ce.Code {
		case websocket.StatusNormalClosure, websocket.StatusGoingAway:
			return io.EOF
		case websocket.StatusAbnormalClosure, websocket.StatusNoStatusRcvd:
			return io.ErrUnexpectedEOF
		}
		// other close codes: return as-is for caller to diagnose
		return err
	}
	// Translate context deadline into net.Error timeout.
	if errors.Is(err, context.DeadlineExceeded) {
		return errTimeout{cause: err}
	}
	return err
}

// mapWriteErr normalizes write-side errors to net.Conn semantics.
func (a *Adapter) mapWriteErr(err error) error {
	if err == nil {
		return nil
	}
	var ce *websocket.CloseError
	if errors.As(err, &ce) {
		// For writes after close, most net.Conn impls return net.ErrClosed.
		return net.ErrClosed
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errTimeout{cause: err}
	}
	return err
}
