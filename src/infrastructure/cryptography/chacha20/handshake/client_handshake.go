package handshake

import (
	"crypto/ed25519"
	"fmt"
	"golang.org/x/crypto/curve25519"
	"net"
	"tungo/application"
	"tungo/infrastructure/settings"
)

// ClientHandshake performs the three‑step authenticated key‑exchange:
// 1. send ClientHello
// 2. receive and verify ServerHello
// 3. send signed Signature
// It drives its I/O through a ClientIO and Crypto through the Crypto interface.
type ClientHandshake struct {
	conn     application.ConnectionAdapter
	crypto   Crypto
	clientIO ClientIO
}

func NewClientHandshake(conn application.ConnectionAdapter, io ClientIO, crypto Crypto) ClientHandshake {
	return ClientHandshake{
		conn:     conn,
		clientIO: io,
		crypto:   crypto,
	}
}

func (c *ClientHandshake) SendClientHello(
	settings settings.Settings,
	edPublicKey ed25519.PublicKey,
	sessionPublicKey, sessionSalt []byte) error {
	hello := NewClientHello(4, net.ParseIP(settings.InterfaceAddress), edPublicKey, sessionPublicKey, sessionSalt)
	return c.clientIO.WriteClientHello(hello)
}

func (c *ClientHandshake) ReceiveServerHello() (ServerHello, error) {
	hello, err := c.clientIO.ReadServerHello()
	if err != nil {
		return ServerHello{}, fmt.Errorf("client handshake: could not receive hello from server: %w", err)
	}

	return hello, nil
}

func (c *ClientHandshake) SendSignature(
	ed25519PublicKey ed25519.PublicKey,
	ed25519PrivateKey ed25519.PrivateKey,
	sessionPublicKey []byte,
	hello ServerHello,
	sessionSalt []byte) error {
	if len(ed25519PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("client handshake: invalid Ed25519 public key length: %d", len(ed25519PublicKey))
	}

	if len(sessionPublicKey) != curve25519.ScalarSize {
		return fmt.Errorf("client handshake: invalid X25519 session public key length: %d", len(sessionPublicKey))
	}

	offset := 0
	dataToVerify := make([]byte, len(hello.curvePublicKey)+len(hello.nonce)+len(sessionSalt))
	copy(dataToVerify[offset:], hello.curvePublicKey)
	offset += len(hello.curvePublicKey)
	copy(dataToVerify[offset:], hello.nonce)
	offset += len(hello.nonce)
	copy(dataToVerify[offset:], sessionSalt)

	if !c.crypto.Verify(ed25519PublicKey, dataToVerify, hello.signature) {
		return fmt.Errorf("client handshake: server failed signature check")
	}

	offset = 0
	dataToSign := make([]byte, len(sessionPublicKey)+len(sessionSalt)+len(hello.nonce))
	copy(dataToSign[offset:], sessionPublicKey)
	offset += len(sessionPublicKey)
	copy(dataToSign[offset:], sessionSalt)
	offset += len(sessionSalt)
	copy(dataToSign[offset:], hello.nonce)

	signature := NewSignature(c.crypto.Sign(ed25519PrivateKey, dataToSign))
	err := c.clientIO.WriteClientSignature(signature)
	if err != nil {
		return fmt.Errorf("client handshake: could not send signature to server: %w", err)
	}

	return nil
}
