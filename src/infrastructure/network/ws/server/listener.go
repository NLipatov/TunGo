//go:build !js

package server

import (
	"context"
	"net"
	"sync"
	"time"
	"tungo/application/listeners"
	"tungo/infrastructure/logging"
)

type server interface {
	Serve() error
	Shutdown() error
	Done() <-chan struct{}
	Err() error
}

type Listener struct {
	ctx                  context.Context
	server               server
	connectionQueue      chan net.Conn
	startOnce, closeOnce sync.Once
}

func NewDefaultListener(
	ctx context.Context,
	listener net.Listener,
) (listeners.TcpListener, error) {
	connectionQueue := make(chan net.Conn, 1024)
	server, serverErr := newHttpServer(
		ctx,
		listener,
		5*time.Second,
		60*time.Second,
		5*time.Second,
		NewDefaultHandler(
			NewDefaultUpgrader(),
			connectionQueue,
			logging.NewLogLogger(),
		),
		"/ws",
	)
	if serverErr != nil {
		return nil, serverErr
	}
	instance := &Listener{
		ctx:             ctx,
		server:          server,
		connectionQueue: connectionQueue,
	}
	instance.Start()
	return instance, nil
}

func NewListener(
	ctx context.Context,
	queue chan net.Conn,
	server server,
) (listeners.TcpListener, error) {
	return &Listener{
		ctx:             ctx,
		connectionQueue: queue,
		server:          server,
	}, nil
}

func (l *Listener) Start() {
	l.startOnce.Do(func() {
		go func() {
			_ = l.server.Serve()
		}()
	})
}

func (l *Listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connectionQueue:
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
