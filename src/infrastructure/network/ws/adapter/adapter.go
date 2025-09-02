package adapter

import (
	"context"
	"io"
	"net"
	"time"
	"tungo/domain/network"
	"tungo/infrastructure/network/ws"

	"github.com/coder/websocket"
)

// Adapter is a ws.Conn adaptation to net.Conn
type Adapter struct {
	conn                        ws.Conn
	ctx                         context.Context
	errorMapper                 defaultErrorMapper
	reader                      io.Reader
	lAddr                       net.Addr
	rAddr                       net.Addr
	readDeadline, writeDeadline network.Deadline
}

func NewDefaultAdapter(ctx context.Context, conn ws.Conn, lAddr, rAddr net.Addr) *Adapter {
	deadline, _ := network.DeadlineFromTime(time.Time{})
	return &Adapter{
		ctx:           ctx,
		conn:          conn,
		errorMapper:   defaultErrorMapper{},
		lAddr:         lAddr,
		rAddr:         rAddr,
		readDeadline:  deadline,
		writeDeadline: deadline,
	}
}

func NewAdapter(
	ctx context.Context,
	conn ws.Conn,
	errorMapper defaultErrorMapper,
	reader io.Reader,
	lAddr, rAddr net.Addr,
	readDeadline, writeDeadline network.Deadline,
) *Adapter {
	return &Adapter{
		ctx:           ctx,
		conn:          conn,
		errorMapper:   errorMapper,
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
	if !a.writeDeadline.ExpiresAt().IsZero() {
		ctx, cancel = context.WithDeadline(a.ctx, a.writeDeadline.ExpiresAt())
	} else {
		ctx, cancel = a.ctx, func() {}
	}
	defer cancel()

	writer, writerErr := a.conn.Writer(ctx, websocket.MessageBinary)
	if writerErr != nil {
		return 0, a.errorMapper.mapErr(writerErr)
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
		written += n
		if wErr != nil {
			return written, a.errorMapper.mapErr(wErr)
		}
	}

	if cErr := writer.Close(); cErr != nil {
		return written, a.errorMapper.mapErr(cErr)
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
				return n, nil
			case io.EOF:
				a.reader = nil
				if n > 0 {
					return n, nil
				}
				continue // next frame
			default:
				a.reader = nil
				return n, a.errorMapper.mapErr(err)
			}
		}

		// per-frame context; DO NOT defer cancel here
		var ctx context.Context
		var cancel context.CancelFunc
		if !a.readDeadline.ExpiresAt().IsZero() {
			ctx, cancel = context.WithDeadline(a.ctx, a.readDeadline.ExpiresAt())
		} else {
			ctx, cancel = a.ctx, func() {}
		}

		mt, r, err := a.conn.Reader(ctx)
		if err != nil {
			cancel()
			return 0, a.errorMapper.mapErr(err)
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
	d, err := network.DeadlineFromTime(deadline)
	if err != nil {
		return err
	}
	a.readDeadline = d
	a.writeDeadline = d
	return nil
}

func (a *Adapter) SetReadDeadline(deadline time.Time) error {
	d, err := network.DeadlineFromTime(deadline)
	if err != nil {
		return err
	}
	a.readDeadline = d
	return nil
}

func (a *Adapter) SetWriteDeadline(deadline time.Time) error {
	d, err := network.DeadlineFromTime(deadline)
	if err != nil {
		return err
	}
	a.writeDeadline = d
	return nil
}
