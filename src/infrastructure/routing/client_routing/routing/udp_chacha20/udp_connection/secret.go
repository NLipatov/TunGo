package udp_connection

import (
	"fmt"
	"tungo/application"
	"tungo/infrastructure/PAL/client_configuration"
	"tungo/infrastructure/settings"
)

type Secret interface {
	Exchange(conn application.ConnectionAdapter) (application.CryptographyService, error)
}

type DefaultSecret struct {
	settings                   settings.Settings
	handshake                  application.Handshake
	cryptographyServiceFactory application.CryptographyServiceFactory
}

func NewDefaultSecret(settings settings.Settings, handshake application.Handshake, cryptographyServiceFactory application.CryptographyServiceFactory) Secret {
	return &DefaultSecret{
		settings:                   settings,
		handshake:                  handshake,
		cryptographyServiceFactory: cryptographyServiceFactory,
	}
}

func (s *DefaultSecret) Exchange(conn application.ConnectionAdapter) (application.CryptographyService, error) {
	handshakeErr := s.handshake.ClientSideHandshake(conn, s.settings)
	if handshakeErr != nil {
		return nil, handshakeErr
	}

	configurationManager := client_configuration.NewManager()
	_, clientConfErr := configurationManager.Configuration()
	if clientConfErr != nil {
		return nil, fmt.Errorf("failed to read client configuration: %s", clientConfErr)
	}

	session, sessionErr := s.cryptographyServiceFactory.FromHandshake(s.handshake, false)
	if sessionErr != nil {
		return nil, fmt.Errorf("failed to create client session: %s\n", sessionErr)
	}

	return session, nil
}
