package tun_server

import (
	"tungo/application/network/connection"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/noise"
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
	return noise.NewNoiseHandshake(
		h.configuration.X25519PublicKey,
		h.configuration.X25519PrivateKey,
	)
}
