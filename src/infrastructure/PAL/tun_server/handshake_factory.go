package tun_server

import (
	"tungo/application"
	"tungo/infrastructure/PAL/server_configuration"
	"tungo/infrastructure/cryptography/chacha20/handshake"
)

type HandshakeFactory struct {
	configuration server_configuration.Configuration
}

func NewHandshakeFactory(configuration server_configuration.Configuration) *HandshakeFactory {
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
