package network

import (
	"fmt"
	"tungo/application"
	"tungo/infrastructure/settings"
)

type Secret interface {
	Exchange(conn application.ConnectionAdapter) (application.CryptographyService, error)
}

type DefaultSecret struct {
	settings                   settings.Settings
	handshake                  application.Handshake
	cryptographyServiceFactory application.CryptographyServiceBuilder
}

func NewDefaultSecret(settings settings.Settings,
	handshake application.Handshake,
	cryptographyServiceFactory application.CryptographyServiceBuilder,
) Secret {
	return &DefaultSecret{
		settings:                   settings,
		handshake:                  handshake,
		cryptographyServiceFactory: cryptographyServiceFactory,
	}
}

func (s *DefaultSecret) Exchange(
	conn application.ConnectionAdapter,
) (application.CryptographyService, error) {
	handshakeErr := s.handshake.ClientSideHandshake(conn, s.settings)
	if handshakeErr != nil {
		return nil, handshakeErr
	}

	cryptographyService, cryptographyServiceErr := s.cryptographyServiceFactory.
		FromHandshake(s.handshake, false)
	if cryptographyServiceErr != nil {
		return nil, fmt.Errorf(
			"failed to create client cryptographyService: %w",
			cryptographyServiceErr,
		)

	}

	return cryptographyService, nil
}
