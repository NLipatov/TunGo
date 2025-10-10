package connection

import (
	"context"
)

type Factory interface {
	EstablishConnection(ctx context.Context) (Transport, Crypto, error)
}
