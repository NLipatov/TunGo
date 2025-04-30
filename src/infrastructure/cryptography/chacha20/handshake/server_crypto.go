package handshake

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/hkdf"
)

type ServerCrypto interface {
	Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool
	NewX25519SessionKeyPair() ([]byte, [32]byte, error)
	GenerateNonce() []byte
	Sign(privateKey ed25519.PrivateKey, data []byte) []byte
	CalculateKeys(
		sessionPrivateKey,
		sessionSalt,
		serverNonce,
		clientNonce,
		clientHelloCurvePubKey,
		sharedSecret []byte) ([]byte, []byte, error)
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

func (c *DefaultServerCrypto) CalculateKeys(
	sessionPrivateKey,
	sessionSalt,
	clientHelloNonce,
	clientNonce,
	clientHelloCurvePubKey,
	sharedSecret []byte) ([]byte, []byte, error) {
	salt := sha256.Sum256(append(clientHelloNonce, clientNonce...))

	infoSC := []byte("server-to-client") // server-key info
	infoCS := []byte("client-to-server") // client-key info

	// Generate HKDF for both encryption directions
	serverToClientHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoSC)
	clientToServerHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoCS)
	keySize := chacha20poly1305.KeySize

	// Generate keys for both encryption directions
	serverToClientKey := make([]byte, keySize)
	_, readErr := io.ReadFull(serverToClientHKDF, serverToClientKey)
	if readErr != nil {
		return nil, nil, fmt.Errorf("failed to read server to client HKDF: %s", readErr)
	}

	clientToServerKey := make([]byte, keySize)
	_, readErr = io.ReadFull(clientToServerHKDF, clientToServerKey)
	if readErr != nil {
		return nil, nil, fmt.Errorf("failed to read client to server HKDF: %s", readErr)
	}

	return serverToClientKey, clientToServerKey, nil
}
