package handshake

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"golang.org/x/crypto/curve25519"
	"io"
)

type crypto interface {
	Sign(privateKey ed25519.PrivateKey, data []byte) []byte
	Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool
	GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error)
	GenerateX25519KeyPair() ([]byte, [32]byte, error)
	GenerateRandomBytesArray(size int) []byte
}

type defaultCrypto struct {
}

func newDefaultCrypto() crypto {
	return &defaultCrypto{}
}

func (c *defaultCrypto) Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool {
	return ed25519.Verify(publicKey, data, signature)
}

func (c *defaultCrypto) Sign(privateKey ed25519.PrivateKey, data []byte) []byte {
	return ed25519.Sign(privateKey, data)
}
func (c *defaultCrypto) GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func (c *defaultCrypto) GenerateX25519KeyPair() ([]byte, [32]byte, error) {
	var private [32]byte

	_, privateErr := io.ReadFull(rand.Reader, private[:])
	if privateErr != nil {
		return make([]byte, 0), private, fmt.Errorf("private key generation err: %s", privateErr)
	}

	public, publicErr := curve25519.X25519(private[:], curve25519.Basepoint)
	if publicErr != nil {
		return make([]byte, 0), private, fmt.Errorf("public key generation err: %s", publicErr)
	}

	if len(public) != 32 {
		return make([]byte, 0), private, fmt.Errorf("public key generation err: invalid public key length")
	}

	return public, private, nil
}

func (c *defaultCrypto) GenerateRandomBytesArray(size int) []byte {
	randomSalt := make([]byte, size)
	_, _ = io.ReadFull(rand.Reader, randomSalt)
	return randomSalt
}
