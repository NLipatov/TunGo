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

	return &DefaultUdpSession{
		SessionId:      handshake.Id(),
		sendCipher:     sendCipher,
		recvCipher:     recvCipher,
		nonce:          NewNonce(),
		isServer:       isServer,
		nonceValidator: NewSliding64(),
		encoder:        DefaultUDPEncoder{},
	}, nil
}
