package handshake

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
	"io"
)

type crypto interface {
	Sign(privateKey ed25519.PrivateKey, data []byte) []byte
	Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool
	GenerateEd25519KeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error)
	GenerateX25519KeyPair() ([]byte, [32]byte, error)
	GenerateRandomBytesArray(size int) []byte
	GenerateChaCha20KeysServerside(
		curvePrivate,
		serverNonce []byte,
		hello ClientHello) (sessionId [32]byte, clientToServerKey, serverToClientKey []byte, err error)
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
		return nil, private, privateErr
	}

	public, publicErr := curve25519.X25519(private[:], curve25519.Basepoint)

	return public, private, publicErr
}

func (c *defaultCrypto) GenerateRandomBytesArray(size int) []byte {
	randomSalt := make([]byte, size)
	_, _ = io.ReadFull(rand.Reader, randomSalt)
	return randomSalt
}
func (h *defaultCrypto) GenerateChaCha20KeysServerside(
	curvePrivate,
	serverNonce []byte,
	hello ClientHello) (sessionId [32]byte, clientToServerKey, serverToClientKey []byte, err error) {
	// Generate shared secret and salt
	sharedSecret, _ := curve25519.X25519(curvePrivate[:], hello.curvePublicKey)
	salt := sha256.Sum256(append(serverNonce, hello.clientNonce...))

	infoSC := []byte("server-to-client") // server-key info
	infoCS := []byte("client-to-server") // client-key info

	// Generate HKDF for both encryption directions
	serverToClientHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoSC)
	clientToServerHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoCS)
	keySize := chacha20poly1305.KeySize

	// Generate keys for both encryption directions
	serverToClientKey = make([]byte, keySize)
	_, _ = io.ReadFull(serverToClientHKDF, serverToClientKey)
	clientToServerKey = make([]byte, keySize)
	_, _ = io.ReadFull(clientToServerHKDF, clientToServerKey)

	sessionId, deriveSessionIdErr := deriveSessionId(sharedSecret, salt[:])
	if deriveSessionIdErr != nil {
		return [32]byte{},
			nil,
			nil,
			fmt.Errorf("failed to derive session id: %s", deriveSessionIdErr)
	}

	return sessionId, clientToServerKey, serverToClientKey, nil
}
