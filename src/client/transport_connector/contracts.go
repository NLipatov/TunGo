package transport_connector

import (
	"context"
	"net"
	"tungo/crypto/chacha20"
)

type Connector interface {
	// UseConnectorDelegate sets a function that is used to create net.Conn instance for given transport
	UseConnectorDelegate(f func() (net.Conn, *chacha20.Session, error)) Connector
	// Connect invokes a connection delegate
	Connect(ctx context.Context) (net.Conn, *chacha20.Session, error)
}
