package chacha20

import (
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/mem"
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
) (connection.Crypto, *rekey.StateMachine, error) {
	sendCipher, recvCipher, err := t.aeadBuilder.FromHandshake(handshake, isServer)
	if err != nil {
		return nil, nil, err
	}

	core := NewTcpCrypto(handshake.Id(), sendCipher, recvCipher, isServer)
	// Directional raw keys live in controller for rekey derivation.
	c2s := handshake.KeyClientToServer()
	s2c := handshake.KeyServerToClient()
	sm := rekey.NewStateMachine(core, c2s, s2c, isServer)
	mem.ZeroBytes(c2s)
	mem.ZeroBytes(s2c)
	return core, sm, nil
}
