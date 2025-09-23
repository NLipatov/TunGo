package factory

import (
	"context"
	"net"
	"time"
	"tungo/infrastructure/logging"
	"tungo/infrastructure/network/ws/server"
	"tungo/infrastructure/network/ws/server/contracts"
)

type DefaultServerFactory struct {
}

func newDefaultServerFactory() *DefaultServerFactory {
	return &DefaultServerFactory{}
}

func (h *DefaultServerFactory) NewServer(
	ctx context.Context,
	listener net.Listener,
	connectionQueue chan net.Conn,
) (contracts.Server, error) {
	return server.NewDefaultServer(
		ctx,
		listener,
		5*time.Second,
		60*time.Second,
		5*time.Second,
		server.NewDefaultHandler(
			server.NewDefaultUpgrader(),
			connectionQueue,
			logging.NewLogLogger(),
		),
		"/ws",
	)
}
