package chacha20

import (
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	sessionIdentifierLength = 32
	directionLength         = 16
	aadLength               = sessionIdentifierLength + chacha20poly1305.NonceSize + directionLength
)
