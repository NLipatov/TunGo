package handshake

import (
	"crypto/ed25519"
	"tungo/application"
	"tungo/settings"
)

type ClientHandshake struct {
	conn application.ConnectionAdapter
	io   ClientIO
}

func NewClientHandshake(conn application.ConnectionAdapter, io ClientIO) ClientHandshake {
	return ClientHandshake{
		conn: conn,
		io:   io,
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
