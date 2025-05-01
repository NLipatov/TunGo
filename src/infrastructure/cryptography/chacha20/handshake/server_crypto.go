package handshake

import (
	"crypto/rand"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
)

type ServerCrypto interface {
	Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool
	NewX25519SessionKeyPair() ([]byte, [32]byte, error)
	GenerateNonce() []byte
	Sign(privateKey ed25519.PrivateKey, data []byte) []byte
	GenerateSharedSecret(sessionPrivateKey, clientHelloCurvePubKey []byte) ([]byte, error)
}

type DefaultServerCrypto struct {
}

func NewDefaultServerCrypto() ServerCrypto {
	return &DefaultServerCrypto{}
}

func (c *DefaultServerCrypto) Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool {
	return !ed25519.Verify(publicKey, data, signature)
}
func (c *DefaultServerCrypto) NewX25519SessionKeyPair() ([]byte, [32]byte, error) {
	var curvePrivate [32]byte
	_, readErr := io.ReadFull(rand.Reader, curvePrivate[:])
	if readErr != nil {
		return nil, [32]byte{}, readErr
	}

	curvePublic, _ := curve25519.X25519(curvePrivate[:], curve25519.Basepoint)

	return curvePublic, curvePrivate, nil
}

func (c *DefaultServerCrypto) GenerateNonce() []byte {
	serverNonce := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, serverNonce)
	return serverNonce
}

func (c *DefaultServerCrypto) Sign(privateKey ed25519.PrivateKey, data []byte) []byte {
	serverSignature := ed25519.Sign(privateKey, data)

	return serverSignature
}

func (c *DefaultServerCrypto) GenerateSharedSecret(sessionPrivateKey, clientHelloCurvePubKey []byte) ([]byte, error) {
	return curve25519.X25519(sessionPrivateKey[:], clientHelloCurvePubKey)
}
