package chacha20

import (
	"tungo/application"
)

type UdpSessionBuilder struct {
	aeadBuilder application.AEADBuilder
}

func NewUdpSessionBuilder(aeadBuilder application.AEADBuilder) application.CryptographyServiceFactory {
	return &UdpSessionBuilder{
		aeadBuilder: aeadBuilder,
	}
}

func (u UdpSessionBuilder) FromHandshake(
	handshake application.Handshake,
	isServer bool,
) (application.CryptographyService, error) {
	sendCipher, recvCipher, err := u.aeadBuilder.FromHandshake(handshake, isServer)
	if err != nil {
		return nil, err
	}

	return &DefaultUdpSession{
		SessionId:      handshake.Id(),
		sendCipher:     sendCipher,
		recvCipher:     recvCipher,
		RecvNonce:      NewNonce(),
		SendNonce:      NewNonce(),
		isServer:       isServer,
		nonceValidator: NewSliding64(),
		encoder:        DefaultUDPEncoder{},
	}, nil
}
