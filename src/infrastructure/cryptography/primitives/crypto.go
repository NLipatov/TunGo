package primitives

import (
	"crypto/rand"
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// KeyDeriver provides cryptographic key generation and derivation primitives
// shared across handshake and control-plane (rekey) code paths.
type KeyDeriver interface {
	GenerateX25519KeyPair() (publicKey []byte, privateKey [32]byte, err error)
	DeriveKey(sharedSecret, salt, info []byte) ([]byte, error)
}

// DefaultKeyDeriver implements KeyDeriver using standard crypto primitives.
type DefaultKeyDeriver struct{}

func (d *DefaultKeyDeriver) GenerateX25519KeyPair() ([]byte, [32]byte, error) {
	var private [32]byte
	if _, err := io.ReadFull(rand.Reader, private[:]); err != nil {
		return nil, private, err
	}
	public, err := curve25519.X25519(private[:], curve25519.Basepoint)
	return public, private, err
}

func (d *DefaultKeyDeriver) DeriveKey(sharedSecret, salt, info []byte) ([]byte, error) {
	r := hkdf.New(sha256.New, sharedSecret, salt, info)
	key := make([]byte, chacha20poly1305.KeySize)
	_, err := io.ReadFull(r, key)
	return key, err
}
