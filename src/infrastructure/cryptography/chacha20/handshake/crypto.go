package handshake

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"golang.org/x/crypto/curve25519"
	"io"
)

type crypto interface {
	sign(privateKey ed25519.PrivateKey, data []byte) []byte
	verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool
	generateEd25519Keys() (ed25519.PublicKey, ed25519.PrivateKey, error)
	newX25519SessionKeyPair() ([]byte, [32]byte, error)
	randomBytesArray(size int) []byte
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
func (c *defaultCrypto) generateEd25519Keys() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func (c *defaultCrypto) newX25519SessionKeyPair() ([]byte, [32]byte, error) {
	var private [32]byte

	_, privateErr := io.ReadFull(rand.Reader, private[:])
	if privateErr != nil {
		return make([]byte, 0), private, fmt.Errorf("private key generation err: %s", privateErr)
	}

	curvePublic, _ := curve25519.X25519(private[:], curve25519.Basepoint)
	if len(curvePublic) != 32 {
		return make([]byte, 0), private, fmt.Errorf("public key generation err: invalid public key length")
	}

	return curvePublic, private, nil
}

func (c *defaultCrypto) randomBytesArray(size int) []byte {
	randomSalt := make([]byte, size)
	_, _ = io.ReadFull(rand.Reader, randomSalt)
	return randomSalt
}
