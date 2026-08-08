package udp

import (
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/mem"
)

func NewFromHandshake(
	handshake chacha20.KeyMaterial,
	isServer bool,
) (*Crypto, *rekey.StateMachine, error) {
	sendCipher, recvCipher, err := chacha20.NewAEADsFromHandshake(handshake, isServer)
	if err != nil {
		return nil, nil, err
	}

	// Directional keys (raw) stay in the controller, not the core crypto.
	c2s := handshake.KeyClientToServer()
	s2c := handshake.KeyServerToClient()

	core := NewCrypto(handshake.ID(), sendCipher, recvCipher, isServer)
	sm := rekey.NewStateMachine(core, c2s, s2c)
	mem.ZeroBytes(c2s)
	mem.ZeroBytes(s2c)
	return core, sm, nil
}
