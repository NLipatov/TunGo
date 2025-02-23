package transport_connector

import (
	"context"
	"net"
	"tungo/crypto"
)

type Connector interface {
	// UseConnectorDelegate sets a function that is used to create net.Conn instance for given transport
	UseConnectorDelegate(f func() (net.Conn, crypto.Session, error)) Connector
	// Connect invokes a connection delegate
	Connect(ctx context.Context) (net.Conn, crypto.Session, error)
}
