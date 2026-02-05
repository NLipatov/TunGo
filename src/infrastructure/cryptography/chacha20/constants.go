package chacha20

import (
	"tungo/infrastructure/cryptography/mem"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	sessionIdentifierLength = 32
	directionLength         = 16
	aadLength               = sessionIdentifierLength + chacha20poly1305.NonceSize + directionLength
)

// zeroBytes is a package-local alias for mem.ZeroBytes.
func zeroBytes(b []byte) {
	mem.ZeroBytes(b)
}
