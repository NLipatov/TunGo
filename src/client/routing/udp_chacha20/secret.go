package udp_chacha20

import (
	"fmt"
	"net"
	"tungo/crypto/chacha20"
	"tungo/settings"
	"tungo/settings/client"
)

type Secret interface {
	Exchange(conn *net.UDPConn) (*chacha20.UdpSession, error)
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

func (s *DefaultSecret) Exchange(conn *net.UDPConn) (*chacha20.UdpSession, error) {
	handshakeErr := s.handshake.ClientSideHandshake(conn, s.settings)
	if handshakeErr != nil {
		return nil, handshakeErr
	}

	session, sessionErr := chacha20.NewUdpSession(s.handshake.Id(), s.handshake.ClientKey(), s.handshake.ServerKey(), false)
	if sessionErr != nil {
		return nil, fmt.Errorf("failed to create client session: %s\n", sessionErr)
	}

	conf, confErr := (&client.Conf{}).Read()
	if confErr != nil {
		return nil, confErr
	}
	session.UseNonceRingBuffer(conf.UDPNonceRingBufferSize)

	return session, nil
}
