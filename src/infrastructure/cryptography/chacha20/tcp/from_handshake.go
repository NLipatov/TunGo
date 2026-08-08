package tcp

import (
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/mem"
)

func NewFromHandshake(handshake chacha20.KeyMaterial,
	isServer bool,
) (*Crypto, *rekey.StateMachine, error) {
	sendCipher, recvCipher, err := chacha20.NewAEADsFromHandshake(handshake, isServer)
	if err != nil {
		return nil, nil, err
	}

	core := NewCrypto(handshake.ID(), sendCipher, recvCipher, isServer)
	// Directional raw keys live in controller for rekey derivation.
	c2s := handshake.KeyClientToServer()
	s2c := handshake.KeyServerToClient()
	sm := rekey.NewStateMachine(core, c2s, s2c)
	mem.ZeroBytes(c2s)
	mem.ZeroBytes(s2c)
	return core, sm, nil
}
