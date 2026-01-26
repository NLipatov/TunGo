package chacha20

import (
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	sessionIdentifierLength = 32
	directionLength         = 16
	keyIDLength             = 1
	aadLength               = sessionIdentifierLength + directionLength + keyIDLength + chacha20poly1305.NonceSize
)
