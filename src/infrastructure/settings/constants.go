package settings

import "golang.org/x/crypto/chacha20poly1305"

const (
	DefaultEthernetMTU = 1500
	SafeMTU            = 1200
	MinimumIPv4MTU     = 576
	MinimumIPv6MTU     = 1280
	// TCPChacha20Overhead does not include nonce, as TCP-connection nonce pair is incremented based on strict TCP-delivery order
	TCPChacha20Overhead = chacha20poly1305.Overhead
	// +1 for key ID byte
	UDPChacha20Overhead = chacha20poly1305.Overhead + chacha20poly1305.NonceSize + 1
)
