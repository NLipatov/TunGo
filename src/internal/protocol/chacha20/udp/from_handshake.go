package udp

import (
	"tungo/internal/protocol/chacha20"
	"tungo/internal/protocol/chacha20/rekey"
	"tungo/internal/protocol/securemem"
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
	securemem.ZeroBytes(c2s)
	securemem.ZeroBytes(s2c)
	return core, sm, nil
}
