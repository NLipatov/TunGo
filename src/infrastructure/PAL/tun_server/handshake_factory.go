package tun_server

import (
	"tungo/application/network/connection"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/chacha20/handshake"
)

type HandshakeFactory struct {
	configuration server.Configuration
}

func NewHandshakeFactory(configuration server.Configuration) *HandshakeFactory {
	return &HandshakeFactory{
		configuration: configuration,
	}
}

func (h *HandshakeFactory) NewHandshake() connection.Handshake {
	return handshake.NewHandshake(
		h.configuration.Ed25519PublicKey,
		h.configuration.Ed25519PrivateKey,
	)
}
