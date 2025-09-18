package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	ErrAlreadyRunning = errors.New("http server is already running")
)

type httpServer struct {
	ctx                                             context.Context
	listener                                        net.Listener
	server                                          *http.Server
	readHeaderTimeout, idleTimeout, shutdownTimeout time.Duration
	handler                                         Handler
	path                                            string
	startOnce, closeOnce                            sync.Once
	closed                                          chan struct{}
	serveErrChan                                    chan error
}

func newHttpServer(
	ctx context.Context,
	listener net.Listener,
	readHeaderTimeout, idleTimeout, shutdownTimeout time.Duration,
	handler Handler,
	path string,
) (*httpServer, error) {
	if ctx == nil {
		return nil, fmt.Errorf("newHttpServer: nil context")
	}
	if listener == nil {
		return nil, fmt.Errorf("newHttpServer: nil net.Listener")
	}
	if handler == nil {
		return nil, fmt.Errorf("newHttpServer: nil Handler")
	}
	if path == "" || path[0] != '/' {
		return nil, fmt.Errorf("newHttpServer: invalid path")
	}
	if shutdownTimeout <= 0 {
		return nil, fmt.Errorf("newHttpServer: shutdownTimeout must be > 0")
	}
	return &httpServer{
		ctx:               ctx,
		listener:          listener,
		readHeaderTimeout: readHeaderTimeout,
		idleTimeout:       idleTimeout,
		shutdownTimeout:   shutdownTimeout,
		handler:           handler,
		path:              path,
		closed:            make(chan struct{}),
		serveErrChan:      make(chan error, 1),
	}, nil
}

func (s *httpServer) Serve() error {
	var (
		err     error
		started bool
	)
	s.startOnce.Do(func() {
		started = true
		mux := http.NewServeMux()
		mux.HandleFunc(s.path, s.handler.Handle)
		s.server = &http.Server{
			Handler: mux,
			BaseContext: func(_ net.Listener) context.Context {
				return s.ctx
			},
			ReadHeaderTimeout: s.readHeaderTimeout,
			IdleTimeout:       s.idleTimeout,
		}
		go func() {
			<-s.ctx.Done()
			_ = s.Shutdown()
		}()
		if sErr := s.server.Serve(s.listener); sErr != nil && !errors.Is(sErr, http.ErrServerClosed) {
			err = sErr
		}
		select {
		case s.serveErrChan <- err:
		default:
		}
		close(s.closed)
	})
	if !started { // second Serve call
		return ErrAlreadyRunning
	}
	return err
}

// Shutdown performs graceful shutdown with a bounded timeout.
func (s *httpServer) Shutdown() error {
	var err error
	s.closeOnce.Do(func() {
		if s.server == nil {
			if s.listener != nil {
				err = s.listener.Close()
			}
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		err = s.server.Shutdown(ctx)
	})
	return err
}

func (s *httpServer) Done() <-chan struct{} { return s.closed }

func (s *httpServer) Err() error {
	select {
	case err := <-s.serveErrChan:
		return err
	default:
		return nil
	}
}
