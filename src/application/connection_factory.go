package application

import (
	"context"
)

type ConnectionFactory interface {
	EstablishConnection(ctx context.Context) (ConnectionAdapter, CryptographyService, error)
}
