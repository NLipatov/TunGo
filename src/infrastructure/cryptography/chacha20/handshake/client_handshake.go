package handshake

import (
	"crypto/ed25519"
	"fmt"
	"tungo/application"
	"tungo/settings"
)

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
		return ServerHello{}, fmt.Errorf("could not receive hello from server: %w", err)
	}

	return hello, nil
}

func (c *ClientHandshake) SendSignature(
	ed25519PublicKey ed25519.PublicKey,
	ed25519PrivateKey ed25519.PrivateKey,
	sessionPublicKey []byte,
	hello ServerHello,
	sessionSalt []byte) error {
	if !c.crypto.Verify(ed25519PublicKey,
		append(append(hello.CurvePublicKey, hello.Nonce...), sessionSalt...), hello.Signature) {
		return fmt.Errorf("server failed signature check")
	}

	dataToSign := append(append(sessionPublicKey, sessionSalt...), hello.Nonce...)
	signature := NewSignature(c.crypto.Sign(ed25519PrivateKey, dataToSign))
	err := c.clientIO.WriteClientSignature(signature)
	if err != nil {
		return fmt.Errorf("could not send signature to server: %w", err)
	}

	return nil
}
