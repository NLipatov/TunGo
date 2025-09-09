package settings

import "golang.org/x/crypto/chacha20poly1305"

const (
	MTU = 1500
	// TCPChacha20Overhead does not include nonce, as TCP-connection nonce pair is incremented based on strict TCP-delivery order
	TCPChacha20Overhead = chacha20poly1305.Overhead
	UDPChacha20Overhead = chacha20poly1305.Overhead + chacha20poly1305.NonceSize
)
