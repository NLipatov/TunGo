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
	conn          ws.Conn
	ctx           context.Context
	errorMapper   errorMapper
	currentReader io.Reader // current in-progress binary frame currentReader (wrapped)
	lAddr         net.Addr
	rAddr         net.Addr

	// 0 - no deadline
	deadlineNS      atomic.Int64
	readDeadlineNS  atomic.Int64
	writeDeadlineNS atomic.Int64
}

func NewAdapter(ctx context.Context, conn ws.Conn, lAddr, rAddr net.Addr) *Adapter {
	adapter := &Adapter{
		ctx:             ctx,
		conn:            conn,
		errorMapper:     defaultErrorMapper{},
		lAddr:           lAddr,
		rAddr:           rAddr,
		deadlineNS:      atomic.Int64{},
		readDeadlineNS:  atomic.Int64{},
		writeDeadlineNS: atomic.Int64{},
	}

	adapter.deadlineNS.Store(0)
	adapter.readDeadlineNS.Store(0)
	adapter.writeDeadlineNS.Store(0)
	return adapter
}

func (a *Adapter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	var ctx context.Context
	var cancel context.CancelFunc
	deadline, deadlineOk := a.nearestDeadline(&a.deadlineNS, &a.writeDeadlineNS)
	if deadlineOk {
		ctx, cancel = context.WithDeadline(a.ctx, deadline)
	} else {
		ctx, cancel = context.WithCancel(a.ctx)
	}
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

		var ctx context.Context
		var cancel context.CancelFunc
		deadline, deadlineOk := a.nearestDeadline(&a.deadlineNS, &a.readDeadlineNS)
		if deadlineOk {
			ctx, cancel = context.WithDeadline(a.ctx, deadline)
		} else {
			ctx, cancel = context.WithCancel(a.ctx)
		}
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
	a.storeNS(&a.deadlineNS, t)     // 0 => снять
	a.storeNS(&a.readDeadlineNS, t) // как у net.Conn: общий дедлайн задаёт оба
	a.storeNS(&a.writeDeadlineNS, t)
	return nil
}

func (a *Adapter) SetReadDeadline(t time.Time) error {
	a.storeNS(&a.readDeadlineNS, t) // 0 => снять
	return nil
}

func (a *Adapter) SetWriteDeadline(t time.Time) error {
	a.storeNS(&a.writeDeadlineNS, t) // 0 => снять
	return nil
}

func (a *Adapter) storeNS(dst *atomic.Int64, t time.Time) {
	if t.IsZero() {
		dst.Store(0) // снять дедлайн
		return
	}
	dst.Store(t.UnixNano())
}

func (a *Adapter) nearestDeadline(firstNS, secondNS *atomic.Int64) (time.Time, bool) {
	first := a.nsToTime(firstNS)
	second := a.nsToTime(secondNS)
	firstIsZero := first.IsZero()
	secondIsZero := second.IsZero()
	if firstIsZero && secondIsZero {
		return time.Time{}, false
	}
	if firstIsZero {
		return second, true
	}
	if secondIsZero {
		return first, true
	}
	if second.Before(first) {
		return second, true
	}
	return first, true
}

func (a *Adapter) nsToTime(src *atomic.Int64) time.Time {
	ns := src.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}
