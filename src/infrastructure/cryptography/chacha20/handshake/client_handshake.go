package handshake

import (
	"crypto/ed25519"
	"fmt"
	"tungo/application"
	"tungo/settings"
)

type ClientHandshake struct {
	conn   application.ConnectionAdapter
	crypto crypto
	io     ClientIO
}

func NewClientHandshake(conn application.ConnectionAdapter, io ClientIO, crypto crypto) ClientHandshake {
	return ClientHandshake{
		conn:   conn,
		io:     io,
		crypto: crypto,
	}
}

func (c *ClientHandshake) SendClientHello(
	settings settings.ConnectionSettings,
	edPublicKey ed25519.PublicKey,
	sessionPublicKey, sessionSalt []byte) error {
	hello := NewClientHello(4, settings.InterfaceAddress, edPublicKey, sessionPublicKey, sessionSalt)
	writeErr := c.io.WriteClientHello(hello)
	if writeErr != nil {
		return writeErr
	}

	return writeErr
}

func (c *ClientHandshake) ReceiveServerHello() (ServerHello, error) {
	serverHello, readServerHelloErr := c.io.ReadServerHello()
	if readServerHelloErr != nil {
		return ServerHello{}, readServerHelloErr
	}

	return serverHello, nil
}

func (c *ClientHandshake) SendSignature(
	ed25519PublicKey ed25519.PublicKey,
	ed25519PrivateKey ed25519.PrivateKey,
	sessionPublicKey []byte,
	hello ServerHello,
	sessionSalt []byte) error {
	if !c.crypto.Verify(ed25519PublicKey, append(append(hello.CurvePublicKey, hello.Nonce...), sessionSalt...), hello.Signature) {
		return fmt.Errorf("server failed signature check")
	}

	dataToSign := append(append(sessionPublicKey, sessionSalt...), hello.Nonce...)
	signature := NewSignature(c.crypto.Sign(ed25519PrivateKey, dataToSign))
	writeSignatureErr := c.io.WriteClientSignature(signature)
	if writeSignatureErr != nil {
		return writeSignatureErr
	}

	return nil
}
