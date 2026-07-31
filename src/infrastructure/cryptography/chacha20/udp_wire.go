package chacha20

import (
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	UDPRouteIDLength = 8
	UDPNonceOffset   = UDPRouteIDLength
	UDPEpochOffset   = UDPNonceOffset + NonceEpochOffset
	UDPMinPacketSize = UDPRouteIDLength + chacha20poly1305.NonceSize + chacha20poly1305.Overhead
)
