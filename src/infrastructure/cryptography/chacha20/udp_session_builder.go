package chacha20

import (
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

type UdpSessionBuilder struct {
	aeadBuilder connection.AEADBuilder
}

func NewUdpSessionBuilder(aeadBuilder connection.AEADBuilder) connection.CryptoFactory {
	return &UdpSessionBuilder{
		aeadBuilder: aeadBuilder,
	}
}

func (u UdpSessionBuilder) FromHandshake(
	handshake connection.Handshake,
	isServer bool,
) (connection.Crypto, *rekey.StateMachine, error) {
	sendCipher, recvCipher, err := u.aeadBuilder.FromHandshake(handshake, isServer)
	if err != nil {
		return nil, nil, err
	}

	// Directional keys (raw) stay in the controller, not the core crypto.
	c2s := handshake.KeyClientToServer()
	s2c := handshake.KeyServerToClient()

	core := NewEpochUdpCrypto(handshake.Id(), sendCipher, recvCipher, isServer)
	return core, rekey.NewController(core, c2s, s2c, isServer), nil
}
