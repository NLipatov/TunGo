package adapter

import (
	"context"
	"io"
	"net"
	"sync"
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

// Adapter bridges coder/websocket.Conn to net.Conn semantics for binary messages.
//
// Concurrency:
//   - Write is safe for concurrent use.
//   - Read is serialized internally (do NOT call Read concurrently).
//
// Deadlines:
//   - Set(Read|Write)Deadline with zero time clears the deadline.
//   - Deadlines apply to the next frame acquisition and the in-frame reads/writes
//     because coder/websocket binds the frame reader/writer to the provided context.
//
// Framing:
//   - Only binary frames are exposed to callers. Non-binary frames are drained and ignored.
type Adapter struct {
	conn ws.Conn
	ctx  context.Context

	// Dependencies
	ctxf  CtxFactory
	copyr Copier
	em    ErrorMapper

	// read state
	rmu sync.Mutex // serialize Read() callers
	cur io.Reader  // current in-progress binary frame reader (wrapped)

	// write state
	wmu sync.Mutex // serialize Writer() creation/Close to avoid overlapping writers

	// deadlines (unix nano; 0 means "no deadline")
	rdl atomic.Int64
	wdl atomic.Int64

	laddr net.Addr
	raddr net.Addr
}

// NewAdapter creates a new Adapter with injected dependencies.
// Pass nil ctx to use context.Background(). Options may be nil (defaults used).
func NewAdapter(ctx context.Context, conn ws.Conn, opts *Options) *Adapter {
	if ctx == nil {
		ctx = context.Background()
	}
	o := opts.WithDefaults()
	a := &Adapter{
		conn:  conn,
		ctx:   ctx,
		ctxf:  o.CtxFactory,
		copyr: o.Copier,
		em:    o.ErrorMapper,
	}
	a.rdl.Store(0)
	a.wdl.Store(0)
	return a
}

// WithAddrs attaches LocalAddr and RemoteAddr metadata.
func (a *Adapter) WithAddrs(local, remote net.Addr) *Adapter {
	a.laddr, a.raddr = local, remote
	return a
}

func (a *Adapter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	a.wmu.Lock()
	defer a.wmu.Unlock()

	ctx, cancel := a.writeCtx()
	defer cancel() // always release writer context

	wr, err := a.conn.Writer(ctx, websocket.MessageBinary)
	if err != nil {
		return 0, a.em.Map(err)
	}

	// Ensure writer is closed even on early returns.
	closed := false
	defer func() {
		if !closed {
			_ = wr.Close()
		}
	}()

	off := 0
	for off < len(p) {
		n, werr := wr.Write(p[off:])
		off += n
		if werr != nil {
			return off, a.em.Map(werr)
		}
	}
	if cerr := wr.Close(); cerr != nil {
		return off, a.em.Map(cerr)
	}
	closed = true
	return off, nil
}

func (a *Adapter) Read(p []byte) (int, error) {
	a.rmu.Lock()
	defer a.rmu.Unlock()

	for {
		if a.cur != nil {
			n, err := a.cur.Read(p)
			switch err {
			case nil:
				return n, nil
			case io.EOF:
				// frame fully consumed
				a.cur = nil
				if n > 0 {
					return n, nil
				}
				continue // fetch next frame
			default:
				a.cur = nil
				return n, a.em.Map(err)
			}
		}

		ctx, cancel := a.readCtx()
		mt, r, err := a.conn.Reader(ctx)
		if err != nil {
			cancel() // failed to get frame; release ctx
			return 0, a.em.Map(err)
		}

		if mt != websocket.MessageBinary {
			// Drain non-binary frames to keep the protocol healthy.
			_, _ = a.copyr.Copy(io.Discard, r)
			cancel()
			continue
		}

		// Keep the frame context alive until EOF/error via wrapper.
		a.cur = &cancelOnEOF{r: r, cancel: cancel}
	}
}

func (a *Adapter) Close() error {
	return a.conn.Close(websocket.StatusNormalClosure, "")
}

func (a *Adapter) LocalAddr() net.Addr {
	if a.laddr != nil {
		return a.laddr
	}
	return &net.TCPAddr{}
}

func (a *Adapter) RemoteAddr() net.Addr {
	if a.raddr != nil {
		return a.raddr
	}
	return &net.TCPAddr{}
}

func (a *Adapter) SetDeadline(t time.Time) error {
	a.storeDeadline(&a.rdl, t)
	a.storeDeadline(&a.wdl, t)
	return nil
}

func (a *Adapter) SetReadDeadline(t time.Time) error {
	a.storeDeadline(&a.rdl, t)
	return nil
}

func (a *Adapter) SetWriteDeadline(t time.Time) error {
	a.storeDeadline(&a.wdl, t)
	return nil
}

// cancelOnEOF wraps a reader and calls cancel() exactly once
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

func (a *Adapter) storeDeadline(dst *atomic.Int64, t time.Time) {
	if t.IsZero() {
		dst.Store(0)
		return
	}
	dst.Store(t.UnixNano())
}

func (a *Adapter) loadDeadline(src *atomic.Int64) (time.Time, bool) {
	ns := src.Load()
	if ns == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, ns), true
}

func (a *Adapter) readCtx() (context.Context, context.CancelFunc) {
	if t, ok := a.loadDeadline(&a.rdl); ok {
		return a.ctxf.WithDeadline(a.ctx, t)
	}
	// return no-op cancel to keep calling code simple
	return a.ctx, func() {}
}

func (a *Adapter) writeCtx() (context.Context, context.CancelFunc) {
	if t, ok := a.loadDeadline(&a.wdl); ok {
		return a.ctxf.WithDeadline(a.ctx, t)
	}
	return a.ctx, func() {}
}
