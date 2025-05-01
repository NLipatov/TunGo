package handshake

import (
	"crypto/rand"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
)

type ClientCrypto interface {
	GenerateSharedSecret(sessionPrivateKey, serverHelloCurvePublicKey []byte) ([]byte, error)
	GenerateEd25519Keys() (ed25519.PublicKey, ed25519.PrivateKey, error)
	NewX25519SessionKeyPair() ([]byte, [32]byte, error)
	GenerateSalt() []byte
	Sign(privateKey ed25519.PrivateKey, data []byte) []byte
	Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool
}

type DefaultClientCrypto struct {
}

func NewDefaultClientCrypto() ClientCrypto {
	return &DefaultClientCrypto{}
}

func (c *DefaultClientCrypto) GenerateEd25519Keys() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func (c *DefaultClientCrypto) NewX25519SessionKeyPair() ([]byte, [32]byte, error) {
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

func (c *DefaultClientCrypto) GenerateSalt() []byte {
	randomSalt := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, randomSalt)
	return randomSalt
}

func (c *DefaultClientCrypto) Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool {
	return ed25519.Verify(publicKey, data, signature)
}

func (c *DefaultClientCrypto) Sign(privateKey ed25519.PrivateKey, data []byte) []byte {
	return ed25519.Sign(privateKey, data)
}

func (c *DefaultClientCrypto) GenerateSharedSecret(sessionPrivateKey, serverHelloCurvePublicKey []byte) ([]byte, error) {
	return curve25519.X25519(sessionPrivateKey[:], serverHelloCurvePublicKey)
}
