package connection

import (
	"fmt"
	"net"
	"tungo/application"
	chacha21 "tungo/infrastructure/cryptography/chacha20"
	"tungo/settings"
	"tungo/settings/client"
)

type Secret interface {
	Exchange(conn *net.UDPConn) (application.CryptographyService, error)
}

type DefaultSecret struct {
	settings  settings.ConnectionSettings
	handshake chacha21.Handshake
}

func NewDefaultSecret(settings settings.ConnectionSettings, handshake chacha21.Handshake) Secret {
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

	conf, confErr := (&client.Conf{}).Read()
	if confErr != nil {
		return nil, confErr
	}

	session, sessionErr := chacha21.NewUdpSession(s.handshake.Id(), s.handshake.ClientKey(), s.handshake.ServerKey(), false, conf.UDPNonceRingBufferSize)
	if sessionErr != nil {
		return nil, fmt.Errorf("failed to create client session: %s\n", sessionErr)
	}

	return session, nil
}
