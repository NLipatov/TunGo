package tun_server

import (
	"tungo/application"
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

func (h *HandshakeFactory) NewHandshake() application.Handshake {
	return handshake.NewHandshake(
		h.configuration.Ed25519PublicKey,
		h.configuration.Ed25519PrivateKey,
	)
}
