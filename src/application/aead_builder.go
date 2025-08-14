package application

import "crypto/cipher"

type AEADBuilder interface {
	FromHandshake(
		h Handshake,
		isServer bool,
	) (send cipher.AEAD, recv cipher.AEAD, err error)
}
