package connection

import (
	"context"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

type Factory interface {
	EstablishConnection(ctx context.Context) (Transport, Crypto, *rekey.StateMachine, error)
}
