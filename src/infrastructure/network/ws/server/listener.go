//go:build !js

package server

import (
	"context"
	"errors"
	"net"
	"sync"
	"tungo/application/listeners"
)

type Server interface {
	Serve() error
	Shutdown() error
	Done() <-chan struct{}
	Err() error
}

var (
	ErrNilFactory  = errors.New("http server factory is nil")
	ErrNilListener = errors.New("net.Listener is nil")
	ErrNilQueue    = errors.New("connection queue is nil")
	ErrNilServer   = errors.New("server is nil")
)

type Listener struct {
	ctx                  context.Context
	server               Server
	connectionQueue      chan net.Conn
	startOnce, closeOnce sync.Once
}

func NewDefaultListener(
	ctx context.Context,
	ln net.Listener,
) (listeners.TcpListener, error) {
	return NewDefaultListenerWithFactory(ctx, ln, NewDefaultHTTPServerFactory())
}

func NewDefaultListenerWithFactory(
	ctx context.Context,
	listener net.Listener,
	factory HTTPServerFactory,
) (listeners.TcpListener, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if factory == nil {
		return nil, ErrNilFactory
	}
	if listener == nil {
		return nil, ErrNilListener
	}
	connectionQueue := make(chan net.Conn, 1024)
	server, serverErr := factory.NewHTTPServer(ctx, listener, connectionQueue)
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
	server Server,
) (listeners.TcpListener, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if queue == nil {
		return nil, ErrNilQueue
	}
	if server == nil {
		return nil, ErrNilServer
	}
	return &Listener{
		ctx:             ctx,
		connectionQueue: queue,
		server:          server,
	}, nil
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
