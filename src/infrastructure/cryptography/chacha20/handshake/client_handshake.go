package handshake

import (
	"crypto/ed25519"
	"fmt"
	"golang.org/x/crypto/curve25519"
	"tungo/application"
	"tungo/settings"
)

// ClientHandshake performs the three‑step handshake with the server.
// 1 - Send Client Hello;
// 2 - Receive Server Hello;
// 3 - Send signed Server Hello.
type ClientHandshake struct {
	conn     application.ConnectionAdapter
	crypto   crypto
	clientIO ClientIO
}

func NewClientHandshake(conn application.ConnectionAdapter, io ClientIO, crypto crypto) ClientHandshake {
	return ClientHandshake{
		conn:     conn,
		clientIO: io,
		crypto:   crypto,
	}
}

func (c *ClientHandshake) SendClientHello(
	settings settings.ConnectionSettings,
	edPublicKey ed25519.PublicKey,
	sessionPublicKey, sessionSalt []byte) error {
	hello := NewClientHello(4, settings.InterfaceAddress, edPublicKey, sessionPublicKey, sessionSalt)
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

	if !c.crypto.Verify(ed25519PublicKey,
		append(append(hello.CurvePublicKey, hello.Nonce...), sessionSalt...), hello.Signature) {
		return fmt.Errorf("client handshake: server failed signature check")
	}

	dataToSign := make([]byte, len(sessionPublicKey)+len(sessionSalt)+len(hello.Nonce))
	offset := 0
	copy(dataToSign[offset:], sessionPublicKey)
	offset += len(sessionPublicKey)
	copy(dataToSign[offset:], sessionSalt)
	offset += len(sessionSalt)
	copy(dataToSign[offset:], hello.Nonce)

	signature := NewSignature(c.crypto.Sign(ed25519PrivateKey, dataToSign))
	err := c.clientIO.WriteClientSignature(signature)
	if err != nil {
		return fmt.Errorf("client handshake: could not send signature to server: %w", err)
	}

	return nil
}
