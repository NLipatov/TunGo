package application

import (
	"context"
	"net"
)

type Connector interface {
	// UseConnectorDelegate sets a function that is used to create net.Conn instance for given transport
	UseConnectorDelegate(f func() (net.Conn, CryptographyService, error)) Connector
	// Connect invokes a connection delegate
	Connect(ctx context.Context) (net.Conn, CryptographyService, error)
}
