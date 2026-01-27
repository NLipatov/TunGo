package chacha20

import (
	"tungo/application/network/connection"
	"tungo/application/network/rekey"
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

	return &DefaultTcpSession{
		SessionId:          handshake.Id(),
		sendCipher:         sendCipher,
		recvCipher:         recvCipher,
		RecvNonce:          NewNonce(0),
		SendNonce:          NewNonce(0),
		isServer:           isServer,
		encryptionNonceBuf: [12]byte{},
		decryptionNonceBuf: [12]byte{},
	}, nil, nil
}
