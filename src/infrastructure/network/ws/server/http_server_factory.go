package server

import (
	"context"
	"net"
	"time"
	"tungo/infrastructure/logging"
)

type HTTPServerFactory interface {
	NewHTTPServer(
		ctx context.Context,
		listener net.Listener,
		connectionQueue chan net.Conn,
	) (Server, error)
}

type DefaultHTTPServerFactory struct {
}

func NewDefaultHTTPServerFactory() *DefaultHTTPServerFactory {
	return &DefaultHTTPServerFactory{}
}

func (h *DefaultHTTPServerFactory) NewHTTPServer(
	ctx context.Context,
	listener net.Listener,
	connectionQueue chan net.Conn,
) (Server, error) {
	return newHttpServer(
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
}
