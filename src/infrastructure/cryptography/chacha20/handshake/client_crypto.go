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

type ClientCrypto interface {
	GenerateEd25519Keys() (ed25519.PublicKey, ed25519.PrivateKey, error)
	NewX25519SessionKeyPair() ([]byte, [32]byte, error)
	GenerateSalt() []byte
	Sign(privateKey ed25519.PrivateKey, data []byte) []byte
	Verify(publicKey ed25519.PublicKey, data []byte, signature []byte) bool
	CalculateKeys(sessionPrivateKey, sessionSalt, serverHelloNonce, serverHelloCurvePublicKey []byte) ([]byte, []byte, [32]byte, error)
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

func (c *DefaultClientCrypto) CalculateKeys(sessionPrivateKey, sessionSalt, serverHelloNonce, serverHelloCurvePublicKey []byte) ([]byte, []byte, [32]byte, error) {
	sharedSecret, _ := curve25519.X25519(sessionPrivateKey[:], serverHelloCurvePublicKey)
	salt := sha256.Sum256(append(serverHelloNonce, sessionSalt...))
	infoSC := []byte("server-to-client") // server-key info
	infoCS := []byte("client-to-server") // client-key info
	serverToClientHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoSC)
	clientToServerHKDF := hkdf.New(sha256.New, sharedSecret, salt[:], infoCS)
	keySize := chacha20poly1305.KeySize
	serverToClientKey := make([]byte, keySize)
	_, _ = io.ReadFull(serverToClientHKDF, serverToClientKey)
	clientToServerKey := make([]byte, keySize)
	_, _ = io.ReadFull(clientToServerHKDF, clientToServerKey)

	derivedSessionId, deriveSessionIdErr := deriveSessionId(sharedSecret, salt[:])
	if deriveSessionIdErr != nil {
		return nil, nil, [32]byte{}, fmt.Errorf("failed to derive session id: %s", derivedSessionId)
	}

	return serverToClientKey, clientToServerKey, derivedSessionId, nil
}

func deriveSessionId(sharedSecret []byte, salt []byte) ([32]byte, error) {
	var sessionID [32]byte

	hkdfReader := hkdf.New(sha256.New, sharedSecret, salt, []byte("session-id-derivation"))
	if _, err := io.ReadFull(hkdfReader, sessionID[:]); err != nil {
		return [32]byte{}, fmt.Errorf("failed to derive session ID: %w", err)
	}

	return sessionID, nil
}
