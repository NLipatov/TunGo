package udp

import (
	"golang.org/x/crypto/chacha20poly1305"
	"tungo/infrastructure/cryptography/chacha20/internal/core"
)

const (
	NonceEpochOffset = core.NonceEpochOffset
	RouteIDLength    = 8
	NonceOffset      = RouteIDLength
	EpochOffset      = NonceOffset + NonceEpochOffset
	MinPacketSize    = RouteIDLength + chacha20poly1305.NonceSize + chacha20poly1305.Overhead
)
