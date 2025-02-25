package udp_chacha20

import (
	"context"
	"fmt"
	"net"
	"time"
	"tungo/crypto/chacha20"
	"tungo/settings"
	"tungo/settings/client"
)

type Secret interface {
	Exchange(ctx context.Context, conn *net.UDPConn) (*chacha20.UdpSession, error)
}

type SecretExchangerImpl struct {
	settings  settings.ConnectionSettings
	handshake chacha20.Handshake
}

func NewSecret(settings settings.ConnectionSettings, handshake chacha20.Handshake) Secret {
	return &SecretExchangerImpl{
		settings:  settings,
		handshake: handshake,
	}
}

func (s *SecretExchangerImpl) Exchange(ctx context.Context, conn *net.UDPConn) (*chacha20.UdpSession, error) {
	if s.settings.DialTimeoutMs <= 0 || s.settings.DialTimeoutMs >= 300_000 {
		s.settings.DialTimeoutMs = 5_000 //5 seconds is default timeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.settings.DialTimeoutMs)*time.Millisecond)
	defer cancel()

	handshakeErr := s.handshake.ClientSideHandshake(ctx, conn, s.settings)
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
