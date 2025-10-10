package network

import (
	"fmt"
	"tungo/application/network/connection"
	"tungo/infrastructure/settings"
)

type Secret interface {
	Exchange(transport connection.Transport) (connection.Crypto, error)
}

type DefaultSecret struct {
	settings                   settings.Settings
	handshake                  connection.Handshake
	cryptographyServiceFactory connection.CryptoFactory
}

func NewDefaultSecret(settings settings.Settings,
	handshake connection.Handshake,
	cryptographyServiceFactory connection.CryptoFactory,
) Secret {
	return &DefaultSecret{
		settings:                   settings,
		handshake:                  handshake,
		cryptographyServiceFactory: cryptographyServiceFactory,
	}
}

func (s *DefaultSecret) Exchange(
	transport connection.Transport,
) (connection.Crypto, error) {
	handshakeErr := s.handshake.ClientSideHandshake(transport, s.settings)
	if handshakeErr != nil {
		return nil, handshakeErr
	}

	crypto, cryptoErr := s.cryptographyServiceFactory.
		FromHandshake(s.handshake, false)
	if cryptoErr != nil {
		return nil, fmt.Errorf(
			"failed to create client crypto: %w",
			cryptoErr,
		)

	}

	return crypto, nil
}
