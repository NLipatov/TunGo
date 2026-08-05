package tcp

import (
	"tungo/application/network/connection"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/mem"
)

type factory struct{}

func NewFactory() connection.CryptoFactory {
	return factory{}
}

func (f factory) FromHandshake(handshake connection.Handshake,
	isServer bool,
) (connection.Crypto, connection.RekeyController, error) {
	sendCipher, recvCipher, err := chacha20.NewAEADsFromHandshake(handshake, isServer)
	if err != nil {
		return nil, nil, err
	}

	core := NewCrypto(handshake.Id(), sendCipher, recvCipher, isServer)
	// Directional raw keys live in controller for rekey derivation.
	c2s := handshake.KeyClientToServer()
	s2c := handshake.KeyServerToClient()
	sm := rekey.NewStateMachine(core, c2s, s2c)
	mem.ZeroBytes(c2s)
	mem.ZeroBytes(s2c)
	return core, sm, nil
}
