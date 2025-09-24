package factory

import (
	"context"
	"net"
	"time"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/network/ws/server"
	"tungo/infrastructure/network/ws/server/contracts"
)

const (
	defaultReadHeaderTimeout = 5 * time.Second
	defaultIdleTimeout       = 60 * time.Second
	defaultShutdownTimeout   = 5 * time.Second
	defaultPath              = "/ws"
)

type ServerFactory struct {
	readHeaderTimeout, idleTimeout, shutdownTimeout time.Duration
	path                                            string
}

func newDefaultServerFactory() *ServerFactory {
	return newServerFactory(defaultReadHeaderTimeout, defaultIdleTimeout, defaultShutdownTimeout, defaultPath)
}

func newServerFactory(
	readHeaderTimeout, idleTimeout, shutdownTimeout time.Duration,
	path string,
) *ServerFactory {
	return &ServerFactory{
		readHeaderTimeout: readHeaderTimeout,
		idleTimeout:       idleTimeout,
		shutdownTimeout:   shutdownTimeout,
		path:              path,
	}
}

func (h *ServerFactory) NewServer(
	ctx context.Context,
	listener net.Listener,
	connectionQueue chan net.Conn,
) (contracts.Server, error) {
	return server.NewDefaultServer(
		ctx,
		listener,
		h.readHeaderTimeout,
		h.idleTimeout,
		h.shutdownTimeout,
		server.NewDefaultHandler(
			server.NewDefaultUpgrader(),
			connectionQueue,
			logging.NewLogLogger(),
		),
		h.path,
	)
}
