package ws

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
	"tungo/application"

	"github.com/coder/websocket"
)

// compile-time check (adapter should implement net.Conn, application.ConnectionAdapter)
var _ net.Conn = (*Adapter)(nil)
var _ application.ConnectionAdapter = (*Adapter)(nil)

type Adapter struct {
	conn  *websocket.Conn
	ctx   context.Context
	cur   io.Reader
	wmu   sync.Mutex
	rdl   atomic.Value // time.Time
	wdl   atomic.Value // time.Time
	laddr net.Addr
	raddr net.Addr
}

func NewAdapter(ctx context.Context, conn *websocket.Conn) *Adapter {
	if ctx == nil {
		ctx = context.Background()
	}
	a := &Adapter{conn: conn, ctx: ctx}
	a.rdl.Store(time.Time{})
	a.wdl.Store(time.Time{})
	return a
}

func (a *Adapter) WithAddrs(local, remote net.Addr) *Adapter {
	a.laddr, a.raddr = local, remote
	return a
}

func (a *Adapter) Write(p []byte) (int, error) {
	a.wmu.Lock()
	defer a.wmu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	ctx, cancel := a.writeCtx()
	defer cancel()

	wr, err := a.conn.Writer(ctx, websocket.MessageBinary)
	if err != nil {
		return 0, a.mapErr(err)
	}
	off := 0
	for off < len(p) {
		n, werr := wr.Write(p[off:])
		off += n
		if werr != nil {
			_ = wr.Close()
			return off, a.mapErr(werr)
		}
	}
	if cerr := wr.Close(); cerr != nil {
		return off, a.mapErr(cerr)
	}
	return off, nil
}

func (a *Adapter) Read(p []byte) (int, error) {
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
		ctx, cancel := a.readCtx()
		mt, r, err := a.conn.Reader(ctx)
		cancel()
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

func (a *Adapter) Close() error { return a.conn.Close(websocket.StatusNormalClosure, "") }

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

func (a *Adapter) SetDeadline(t time.Time) error      { a.rdl.Store(t); a.wdl.Store(t); return nil }
func (a *Adapter) SetReadDeadline(t time.Time) error  { a.rdl.Store(t); return nil }
func (a *Adapter) SetWriteDeadline(t time.Time) error { a.wdl.Store(t); return nil }

func (a *Adapter) readCtx() (context.Context, context.CancelFunc) {
	if t, _ := a.rdl.Load().(time.Time); !t.IsZero() {
		return context.WithDeadline(a.ctx, t)
	}
	return a.ctx, func() {}
}
func (a *Adapter) writeCtx() (context.Context, context.CancelFunc) {
	if t, _ := a.wdl.Load().(time.Time); !t.IsZero() {
		return context.WithDeadline(a.ctx, t)
	}
	return a.ctx, func() {}
}

func (a *Adapter) mapErr(err error) error {
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
