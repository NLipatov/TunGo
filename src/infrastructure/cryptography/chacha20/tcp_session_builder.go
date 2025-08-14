package chacha20

import (
	"tungo/application"
)

type TcpSessionBuilder struct {
	aeadBuilder application.AEADBuilder
}

func NewTcpSessionBuilder(aeadBuilder application.AEADBuilder) application.CryptographyServiceFactory {
	return &TcpSessionBuilder{
		aeadBuilder: aeadBuilder,
	}
}

func (t TcpSessionBuilder) FromHandshake(handshake application.Handshake,
	isServer bool,
) (application.CryptographyService, error) {
	sendCipher, recvCipher, err := t.aeadBuilder.FromHandshake(handshake, isServer)
	if err != nil {
		return nil, err
	}

	return &DefaultTcpSession{
		SessionId:          handshake.Id(),
		sendCipher:         sendCipher,
		recvCipher:         recvCipher,
		RecvNonce:          NewNonce(),
		SendNonce:          NewNonce(),
		isServer:           isServer,
		nonceValidator:     NewStrictCounter(),
		encryptionNonceBuf: [12]byte{},
		decryptionNonceBuf: [12]byte{},
	}, nil
}
