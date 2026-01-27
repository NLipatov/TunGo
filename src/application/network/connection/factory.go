package connection

import (
	"context"
	"tungo/application/network/rekey"
)

type Factory interface {
	EstablishConnection(ctx context.Context) (Transport, Crypto, *rekey.Controller, error)
}
