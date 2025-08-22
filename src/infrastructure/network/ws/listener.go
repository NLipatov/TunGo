//go:build !js

package ws

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"
	"tungo/infrastructure/network/ws/adapters"
	"tungo/infrastructure/settings"

	"tungo/application/listeners"

	"github.com/coder/websocket"
)

// compile-time check (Listener must implement listeners.TcpListener)
var _ listeners.TcpListener = (*Listener)(nil)

type Listener struct {
	ctx    context.Context
	ln     net.Listener
	srv    *http.Server
	queue  chan net.Conn
	once   sync.Once
	closed chan struct{}
}

func NewListener(ctx context.Context, ap netip.AddrPort) (listeners.TcpListener, error) {
	ln, err := net.Listen("tcp", ap.String())
	if err != nil {
		return nil, err
	}
	q := make(chan net.Conn, 1024)
	closed := make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			CompressionMode: websocket.CompressionDisabled,
		})
		if err != nil {
			return
		}
		c.SetReadLimit(int64(settings.MTU + settings.TCPChacha20Overhead))

		local := ln.Addr()
		remote := parseTCPAddr(r.RemoteAddr)
		conn := adapters.NewAdapter(context.Background(), c).WithAddrs(local, remote)

		select {
		case q <- conn:
		default:
			_ = c.Close(websocket.StatusPolicyViolation, "queue full")
		}
	})

	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(ln)
		close(closed)
	}()
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()

	return &Listener{ctx: ctx, ln: ln, srv: srv, queue: q, closed: closed}, nil
}

func (l *Listener) Accept() (net.Conn, error) {
	select {
	case c := <-l.queue:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *Listener) Close() error {
	l.once.Do(func() {
		shCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = l.srv.Shutdown(shCtx)
		_ = l.ln.Close()
	})
	return nil
}

func parseTCPAddr(s string) net.Addr {
	host, port, _ := net.SplitHostPort(s)
	p, _ := strconv.Atoi(port)
	return &net.TCPAddr{IP: net.ParseIP(host), Port: p}
}
