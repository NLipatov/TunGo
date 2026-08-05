package network

import (
	"fmt"
	"tungo/application/network/connection"
)

type Secret interface {
	Exchange(transport connection.Transport) (connection.Crypto, connection.RekeyController, error)
}

type DefaultSecret struct {
	handshake                  connection.Handshake
	cryptographyServiceFactory connection.CryptoFactory
}

func NewDefaultSecret(handshake connection.Handshake,
	cryptographyServiceFactory connection.CryptoFactory,
) Secret {
	return &DefaultSecret{
		handshake:                  handshake,
		cryptographyServiceFactory: cryptographyServiceFactory,
	}
}

func (s *DefaultSecret) Exchange(
	transport connection.Transport,
) (connection.Crypto, connection.RekeyController, error) {
	if handshakeErr := s.handshake.ClientSideHandshake(transport); handshakeErr != nil {
		return nil, nil, handshakeErr
	}

	crypto, controller, cryptoErr := s.cryptographyServiceFactory.
		FromHandshake(s.handshake, false)
	if cryptoErr != nil {
		return nil, nil, fmt.Errorf(
			"failed to create client crypto: %w",
			cryptoErr,
		)

	}

	return crypto, controller, nil
}
