//go:build !js

package server

import (
	"context"
	"errors"
	"net"
	"sync"
	"tungo/application/listeners"
	"tungo/infrastructure/network/ws/server/contracts"
)

// Listener is a wrapper around Server to bring net.TCPListener semantics
type Listener struct {
	ctx                  context.Context
	server               contracts.Server
	connectionQueue      chan net.Conn
	startOnce, closeOnce sync.Once
}

// NewListener wires Server and connection queue and starts Server.
func NewListener(
	ctx context.Context,
	server contracts.Server,
	queue chan net.Conn,
) (listeners.TcpListener, error) {
	if ctx == nil {
		return nil, errors.New("ctx must not be nil")
	}
	if server == nil {
		return nil, errors.New("server must not be nil")
	}
	if queue == nil {
		return nil, errors.New("queue must not be nil")
	}
	ln := &Listener{
		ctx:             ctx,
		server:          server,
		connectionQueue: queue,
	}
	ln.Start()
	return ln, nil
}

func (l *Listener) Start() {
	l.startOnce.Do(func() {
		go func() {
			<-l.ctx.Done()
			_ = l.Close()
		}()
		go func() {
			_ = l.server.Serve()
		}()
	})
}

func (l *Listener) Accept() (net.Conn, error) {
	select {
	case <-l.ctx.Done():
		return nil, net.ErrClosed
	case conn, ok := <-l.connectionQueue:
		if !ok || conn == nil {
			return nil, net.ErrClosed
		}
		return conn, nil
	case <-l.server.Done():
		if err := l.server.Err(); err != nil {
			return nil, err
		}
		return nil, net.ErrClosed
	}
}

func (l *Listener) Close() error {
	var err error
	l.closeOnce.Do(func() { err = l.server.Shutdown() })
	return err
}
