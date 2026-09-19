package settings

import "golang.org/x/crypto/chacha20poly1305"

const (
	DefaultEthernetMTU = 1500
	DefaultMTU         = 1280
	MinimumIPv4MTU     = 576
	MinimumIPv6MTU     = 1280
	MaximumMTU         = 9000
	// TCPChacha20Overhead is the poly1305 tag + 2-byte epoch prefix prepended
	// to every TCP frame. Nonce is not on the wire — it is derived from the
	// deterministic counter incremented based on strict TCP-delivery order.
	TCPChacha20Overhead = chacha20poly1305.Overhead + 2
	// UDPChacha20Overhead includes route-id (8 bytes), nonce (12 bytes), and AEAD tag (16 bytes).
	UDPChacha20Overhead = chacha20poly1305.Overhead + chacha20poly1305.NonceSize + 8
)
