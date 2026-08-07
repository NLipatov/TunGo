//go:build !js

package ws

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"tungo/infrastructure/settings"

	"github.com/coder/websocket"
)

const (
	defaultPath              = "/ws"
	defaultQueueSize         = 1024
	defaultReadHeaderTimeout = 5 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultShutdownTimeout   = 5 * time.Second
	queueFullCloseCode       = websocket.StatusCode(3000)
)

var _ net.Listener = (*Listener)(nil)

// Listener accepts WebSocket connections through the net.Listener API.
type Listener struct {
	ctx             context.Context
	listener        net.Listener
	server          *http.Server
	queue           chan net.Conn
	done            chan struct{}
	shutdownTimeout time.Duration
	closeOnce       sync.Once
	errMu           sync.RWMutex
	serveErr        error
}

func NewListener(ctx context.Context, listener net.Listener) (*Listener, error) {
	if ctx == nil {
		return nil, errors.New("context must not be nil")
	}
	if listener == nil {
		return nil, errors.New("listener must not be nil")
	}

	l := &Listener{
		ctx:             ctx,
		listener:        listener,
		queue:           make(chan net.Conn, defaultQueueSize),
		done:            make(chan struct{}),
		shutdownTimeout: defaultShutdownTimeout,
	}
	mux := http.NewServeMux()
	mux.HandleFunc(defaultPath, l.handle)
	l.server = &http.Server{
		Handler:           mux,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: defaultReadHeaderTimeout,
		IdleTimeout:       defaultIdleTimeout,
	}

	go l.serve()
	go func() {
		select {
		case <-ctx.Done():
			_ = l.Close()
		case <-l.done:
		}
	}()

	return l, nil
}

func (l *Listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.queue:
		if conn == nil {
			return nil, net.ErrClosed
		}
		return conn, nil
	case <-l.ctx.Done():
		return nil, net.ErrClosed
	case <-l.done:
		if err := l.err(); err != nil {
			return nil, err
		}
		return nil, net.ErrClosed
	}
}

func (l *Listener) Close() error {
	var closeErr error
	l.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), l.shutdownTimeout)
		defer cancel()

		closeErr = l.server.Shutdown(ctx)
		if err := l.listener.Close(); closeErr == nil && err != nil && !errors.Is(err, net.ErrClosed) {
			closeErr = err
		}
	})
	return closeErr
}

func (l *Listener) Addr() net.Addr {
	return l.listener.Addr()
}

func (l *Listener) handle(w http.ResponseWriter, r *http.Request) {
	remote, err := remoteAddr(r.RemoteAddr)
	if err != nil {
		slog.Warn("bad remote addr", "err", err)
		http.Error(w, "bad remote addr", http.StatusBadRequest)
		return
	}

	conn, err := l.upgrade(w, r)
	if err != nil {
		slog.Warn("WebSocket upgrade failed", "err", err)
		return
	}

	local := l.listener.Addr()
	if addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok && addr != nil {
		local = addr
	}

	adapted := &Conn{ctx: l.ctx, conn: conn, lAddr: local, rAddr: remote}
	select {
	case l.queue <- adapted:
	default:
		_ = conn.Close(queueFullCloseCode, "could not accept new connection")
	}
}

func (l *Listener) serve() {
	err := l.server.Serve(l.listener)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		err = nil
	}
	l.errMu.Lock()
	l.serveErr = err
	l.errMu.Unlock()
	close(l.done)
}

func (l *Listener) err() error {
	l.errMu.RLock()
	defer l.errMu.RUnlock()
	return l.serveErr
}

func (*Listener) upgrade(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(int64(settings.DefaultEthernetMTU + settings.TCPChacha20Overhead))
	return conn, nil
}

func remoteAddr(raw string) (*net.TCPAddr, error) {
	addr, err := netip.ParseAddrPort(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid remote address: %w", err)
	}
	return net.TCPAddrFromAddrPort(addr), nil
}
