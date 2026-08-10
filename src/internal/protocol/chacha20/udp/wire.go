package udp

import (
	"golang.org/x/crypto/chacha20poly1305"
	"tungo/internal/protocol/chacha20/internal/core"
)

const (
	NonceEpochOffset = core.NonceEpochOffset
	RouteIDLength    = 8
	NonceOffset      = RouteIDLength
	EpochOffset      = NonceOffset + NonceEpochOffset
	PayloadOffset    = NonceOffset + chacha20poly1305.NonceSize
	MinPacketSize    = PayloadOffset + chacha20poly1305.Overhead
)
