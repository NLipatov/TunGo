package connection

import "tungo/application/network/rekey"

type CryptoFactory interface {
	FromHandshake(
		handshake Handshake,
		isServer bool,
	) (Crypto, *rekey.Controller, error)
}
