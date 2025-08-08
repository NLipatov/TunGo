package chacha20

import (
	"crypto/cipher"
	"golang.org/x/crypto/chacha20poly1305"
	"tungo/application"
)

type TcpSessionBuilder struct {
}

func NewTcpSessionBuilder() application.CryptographyServiceFactory {
	return &TcpSessionBuilder{}
}

func (T TcpSessionBuilder) FromHandshake(handshake application.Handshake,
	isServer bool,
) (application.CryptographyService, error) {
	var sendCipher cipher.AEAD
	var recvCipher cipher.AEAD
	var err error
	if isServer {
		sendCipher, err = chacha20poly1305.New(handshake.ServerKey())
		if err != nil {
			return nil, err
		}

		recvCipher, err = chacha20poly1305.New(handshake.ClientKey())
		if err != nil {
			return nil, err
		}
	} else {
		sendCipher, err = chacha20poly1305.New(handshake.ClientKey())
		if err != nil {
			return nil, err
		}

		recvCipher, err = chacha20poly1305.New(handshake.ServerKey())
		if err != nil {
			return nil, err
		}
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
