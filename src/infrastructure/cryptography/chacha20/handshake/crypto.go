package handshake

import "crypto/ed25519"

type crypto interface {
	sign(privateKey ed25519.PrivateKey, data []byte) []byte
	verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool
}

type defaultCrypto struct {
}

func newDefaultCrypto() crypto {
	return &defaultCrypto{}
}

func (c *defaultCrypto) verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool {
	return ed25519.Verify(publicKey, data, signature)
}

func (c *defaultCrypto) sign(privateKey ed25519.PrivateKey, data []byte) []byte {
	return ed25519.Sign(privateKey, data)
}
