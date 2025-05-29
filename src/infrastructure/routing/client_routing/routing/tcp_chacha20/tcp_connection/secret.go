package tcp_connection

import (
	"fmt"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/settings"
)

type Secret interface {
	Exchange(conn application.ConnectionAdapter) (application.CryptographyService, error)
}

type DefaultSecret struct {
	settings  settings.Settings
	handshake handshake.Handshake
}

func NewDefaultSecret(settings settings.Settings, handshake handshake.Handshake) Secret {
	return &DefaultSecret{
		settings:  settings,
		handshake: handshake,
	}
}

func (s *DefaultSecret) Exchange(conn application.ConnectionAdapter) (application.CryptographyService, error) {
	handshakeErr := s.handshake.ClientSideHandshake(conn, s.settings)
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
