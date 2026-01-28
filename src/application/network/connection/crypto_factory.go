package connection

import (
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

type CryptoFactory interface {
	FromHandshake(
		handshake Handshake,
		isServer bool,
	) (Crypto, *rekey.StateMachine, error)
}
