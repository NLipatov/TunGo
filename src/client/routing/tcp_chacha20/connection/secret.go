package connection

import (
	"fmt"
	"net"
	"tungo/crypto/chacha20"
	"tungo/settings"
)

type Secret interface {
	Exchange(conn *net.Conn) (*chacha20.TcpCryptographyService, error)
}

type DefaultSecret struct {
	settings  settings.ConnectionSettings
	handshake chacha20.Handshake
}

func NewDefaultSecret(settings settings.ConnectionSettings, handshake chacha20.Handshake) Secret {
	return &DefaultSecret{
		settings:  settings,
		handshake: handshake,
	}
}

func (s *DefaultSecret) Exchange(conn *net.Conn) (*chacha20.TcpCryptographyService, error) {
	handshakeErr := s.handshake.ClientSideHandshake(*conn, s.settings)
	if handshakeErr != nil {
		return nil, handshakeErr
	}

	cryptographyService, cryptographyServiceErr := chacha20.NewTcpCryptographyService(s.handshake.Id(), s.handshake.ClientKey(), s.handshake.ServerKey(), false)
	if cryptographyServiceErr != nil {
		return nil, fmt.Errorf("failed to create client cryptographyService: %s\n", cryptographyServiceErr)
	}

	cryptographyService.UseNonceRingBuffer()

	return cryptographyService, nil
}
