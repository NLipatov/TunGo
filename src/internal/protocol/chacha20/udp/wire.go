package udp

import (
	"tungo/internal/protocol/chacha20/internal/core"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	RouteIDLength = 8
	NonceOffset   = RouteIDLength
	EpochOffset   = NonceOffset + core.NonceEpochOffset
	PayloadOffset = NonceOffset + chacha20poly1305.NonceSize
	MinPacketSize = PayloadOffset + chacha20poly1305.Overhead
)
