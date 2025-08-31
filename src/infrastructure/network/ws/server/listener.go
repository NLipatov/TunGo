//go:build !js

package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
	"tungo/application/listeners"
	"tungo/infrastructure/logging"
)

type Listener struct {
	ctx                                                     context.Context
	requestHandler                                          Handler
	httpReadHeaderTimeout, httpIdleTimeout, shutdownTimeout time.Duration
	listener                                                net.Listener
	path                                                    string
	httpServer                                              *http.Server
	queue                                                   chan net.Conn
	startOnce, closeOnce                                    sync.Once
	httpServerServeErr                                      chan error
	closed                                                  chan struct{}
}

func NewDefaultListener(
	ctx context.Context,
	listener net.Listener,
) (listeners.TcpListener, error) {
	queue := make(chan net.Conn, 1024)
	instance := &Listener{
		ctx: ctx,
		requestHandler: NewDefaultHandler(
			NewDefaultUpgrader(),
			queue,
			logging.NewLogLogger(),
		),
		shutdownTimeout:       time.Second * 5,
		httpReadHeaderTimeout: 5 * time.Second,
		httpIdleTimeout:       60 * time.Second,
		listener:              listener,
		path:                  "/ws",
		queue:                 queue,
		startOnce:             sync.Once{},
		closeOnce:             sync.Once{},
		httpServerServeErr:    make(chan error, 1),
		closed:                make(chan struct{}),
	}

	return instance, instance.Start()
}

func NewListener(
	ctx context.Context,
	listener net.Listener,
	path string,
	requestHandler Handler,
	queue chan net.Conn,
	httpReadHeaderTimeout, httpIdleTimeout, shutdownTimeout time.Duration,
) (listeners.TcpListener, error) {
	return &Listener{
		ctx:                   ctx,
		requestHandler:        requestHandler,
		shutdownTimeout:       shutdownTimeout,
		httpReadHeaderTimeout: httpReadHeaderTimeout,
		httpIdleTimeout:       httpIdleTimeout,
		listener:              listener,
		path:                  path,
		startOnce:             sync.Once{},
		closeOnce:             sync.Once{},
		queue:                 queue,
		httpServerServeErr:    make(chan error, 1),
		closed:                make(chan struct{}),
	}, nil
}

func (l *Listener) Start() error {
	l.startOnce.Do(func() {
		mux := http.NewServeMux()
		mux.HandleFunc(l.path, l.requestHandler.Handle)

		l.httpServer = &http.Server{
			Handler: mux,
			BaseContext: func(_ net.Listener) context.Context {
				return l.ctx
			},
			ReadHeaderTimeout: l.httpReadHeaderTimeout,
			IdleTimeout:       l.httpIdleTimeout,
		}

		go func() {
			err := l.httpServer.Serve(l.listener)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				select {
				case l.httpServerServeErr <- err:
				default: // drop subsequent errs if buffer is full
				}
			}
			select {
			case <-l.closed: // if closed already, do not close it second time
			default:
				close(l.closed) // if not yet closed, close it
			}
		}()

		go func() {
			<-l.ctx.Done()
			_ = l.shutdown()
		}()
	})
	return nil
}

func (l *Listener) serveError() error {
	select {
	case err := <-l.httpServerServeErr:
		return err
	default:
		return nil
	}
}

func (l *Listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.queue:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *Listener) Close() error {
	var shutdownErr error
	l.closeOnce.Do(func() {
		shutdownErr = l.shutdown()
		select {
		case <-l.closed:
		default:
			close(l.closed)
		}
	})
	return shutdownErr
}

func (l *Listener) shutdown() error {
	if l.httpServer == nil {
		if l.listener != nil {
			_ = l.listener.Close()
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), l.shutdownTimeout)
	defer cancel()
	_ = l.httpServer.Shutdown(ctx)
	return l.listener.Close()
}
