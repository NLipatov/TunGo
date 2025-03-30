package application

import (
	"context"
	"net"
)

type ConnectionFactory interface {
	EstablishConnection(ctx context.Context) (net.Conn, CryptographyService, error)
}
