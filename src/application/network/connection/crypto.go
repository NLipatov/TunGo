package connection

type Crypto interface {
	Encrypt(plaintext []byte) ([]byte, error)
	Decrypt(ciphertext []byte) ([]byte, error)
}

// CryptoRouteIDProvider is an optional interface for UDP crypto implementations
// that expose a stable per-session route identifier.
type CryptoRouteIDProvider interface {
	RouteID() uint64
}

// CryptoZeroizer is an optional interface for crypto implementations that support
// explicit zeroing of key material. MUST be called when sessions are destroyed.
type CryptoZeroizer interface {
	// Zeroize overwrites all key material with zeros.
	// After this call, the crypto instance is unusable.
	Zeroize()
}
