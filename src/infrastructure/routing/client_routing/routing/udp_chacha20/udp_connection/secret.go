package udp_connection

import (
	"fmt"
	"net"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/settings"
	"tungo/settings/client_configuration"
)

type Secret interface {
	Exchange(conn *net.UDPConn) (application.CryptographyService, error)
}

type DefaultSecret struct {
	settings  settings.ConnectionSettings
	handshake handshake.Handshake
}

func NewDefaultSecret(settings settings.ConnectionSettings, handshake handshake.Handshake) Secret {
	return &DefaultSecret{
		settings:  settings,
		handshake: handshake,
	}
}

func (s *DefaultSecret) Exchange(conn *net.UDPConn) (application.CryptographyService, error) {
	handshakeErr := s.handshake.ClientSideHandshake(conn, s.settings)
	if handshakeErr != nil {
		return nil, handshakeErr
	}

	configurationManager := client_configuration.NewManager()
	_, clientConfErr := configurationManager.Configuration()
	if clientConfErr != nil {
		return nil, fmt.Errorf("failed to read client configuration: %s", clientConfErr)
	}

	session, sessionErr := chacha20.NewUdpSession(s.handshake.Id(), s.handshake.ClientKey(), s.handshake.ServerKey(), false)
	if sessionErr != nil {
		return nil, fmt.Errorf("failed to create client session: %s\n", sessionErr)
	}

	return session, nil
}
