package contracts

import (
	"context"
	"net"
)

type ServerFactory interface {
	NewServer(
		ctx context.Context,
		listener net.Listener,
		connectionQueue chan net.Conn,
	) (Server, error)
}
