package chacha20

import (
	"tungo/application/network/connection"
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
) (connection.Crypto, error) {
	sendCipher, recvCipher, err := u.aeadBuilder.FromHandshake(handshake, isServer)
	if err != nil {
		return nil, err
	}

	n := NewNonce()
	session := &DefaultUdpSession{
		SessionId: handshake.Id(),
		isServer:  isServer,
		encoder:   DefaultUDPEncoder{},
	}
	if isServer {
		session.current = keySlot{
			send:    sendCipher,
			recv:    recvCipher,
			sendKey: handshake.KeyServerToClient(), // server sends S->C
			recvKey: handshake.KeyClientToServer(), // server receives C->S
			keyID:   0,
			nonce:   n,
			window:  NewSliding64(),
			set:     true,
		}
	} else {
		session.current = keySlot{
			send:    sendCipher,
			recv:    recvCipher,
			sendKey: handshake.KeyClientToServer(), // client sends C->S
			recvKey: handshake.KeyServerToClient(), // client receives S->C
			keyID:   0,
			nonce:   n,
			window:  NewSliding64(),
			set:     true,
		}
	}
	return session, nil
}
