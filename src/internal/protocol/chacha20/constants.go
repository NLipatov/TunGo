package chacha20

import "golang.org/x/crypto/chacha20poly1305"

const (
	// TCPEpochPrefixSize is the number of bytes prepended to every encrypted frame.
	TCPEpochPrefixSize = 2
	// TCPOverhead is the 2-byte epoch prefix plus the poly1305 tag added
	// to every TCP frame. Nonce is not on the wire — it is derived from the
	// deterministic counter incremented based on strict TCP-delivery order.
	TCPOverhead = chacha20poly1305.Overhead + TCPEpochPrefixSize
	// UDPOverhead includes route-id (8 bytes), nonce (12 bytes), and AEAD tag (16 bytes).
	UDPOverhead = chacha20poly1305.Overhead + chacha20poly1305.NonceSize + 8
)
