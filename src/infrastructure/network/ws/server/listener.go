//go:build !js

package server

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
	"tungo/application/listeners"
	"tungo/infrastructure/logging"
)

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
	queue := make(chan net.Conn, 1024)
	closed := make(chan struct{})

	handler := NewDefaultHandler(NewDefaultUpgrader(), queue, logging.NewLogLogger())

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handler.Handle)

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

	return &Listener{ctx: ctx, ln: ln, srv: srv, queue: queue, closed: closed}, nil
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
