package handshake

import (
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

type ClientCrypto interface {
	CalculateKeys(sessionPrivateKey, sessionSalt, serverHelloNonce, serverHelloCurvePublicKey []byte) ([]byte, []byte, [32]byte, error)
}

type DefaultClientCrypto struct {
}

func NewDefaultClientCrypto() ClientCrypto {
	return &DefaultClientCrypto{}
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

	identifier := NewSessionIdentifier(sharedSecret, salt[:])
	derivedSessionId, deriveSessionIdErr := identifier.Identify()
	if deriveSessionIdErr != nil {
		return nil, nil, [32]byte{}, fmt.Errorf("failed to derive session id: %s", derivedSessionId)
	}

	return serverToClientKey, clientToServerKey, derivedSessionId, nil
}
