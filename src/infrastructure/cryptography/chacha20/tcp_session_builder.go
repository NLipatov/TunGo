package chacha20

import (
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
)

type TcpSessionBuilder struct {
	aeadBuilder connection.AEADBuilder
}

func NewTcpSessionBuilder(aeadBuilder connection.AEADBuilder) connection.CryptoFactory {
	return &TcpSessionBuilder{
		aeadBuilder: aeadBuilder,
	}
}

func (t TcpSessionBuilder) FromHandshake(handshake connection.Handshake,
	isServer bool,
) (connection.Crypto, *rekey.Controller, error) {
	sendCipher, recvCipher, err := t.aeadBuilder.FromHandshake(handshake, isServer)
	if err != nil {
		return nil, nil, err
	}

	core := NewTcpCrypto(handshake.Id(), sendCipher, recvCipher, isServer)
	// Directional raw keys live in controller for rekey derivation.
	c2s := handshake.KeyClientToServer()
	s2c := handshake.KeyServerToClient()
	return core, rekey.NewController(core, c2s, s2c, isServer), nil
}
