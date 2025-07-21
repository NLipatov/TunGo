package chacha20

import (
	"crypto/cipher"
	"golang.org/x/crypto/chacha20poly1305"
	"tungo/application"
)

type UdpSessionBuilder struct {
}

func NewUdpSessionBuilder() application.CryptographyServiceFactory {
	return &UdpSessionBuilder{}
}

func (u UdpSessionBuilder) FromHandshake(
	handshake application.Handshake,
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

	return &DefaultUdpSession{
		SessionId:      handshake.Id(),
		sendCipher:     sendCipher,
		recvCipher:     recvCipher,
		RecvNonce:      NewNonce(),
		SendNonce:      NewNonce(),
		isServer:       isServer,
		nonceValidator: NewSliding64(),
	}, nil
}
