package client_factory

import (
	"context"
	"fmt"
	"math"
	"time"
	"tungo/application"
	"tungo/infrastructure/PAL/client_configuration"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/network"
	"tungo/infrastructure/routing/client_routing/routing/tcp_chacha20/tcp_connection"
	"tungo/infrastructure/routing/client_routing/routing/udp_chacha20/udp_connection"
	"tungo/infrastructure/settings"
)

type ConnectionFactory struct {
	conf client_configuration.Configuration
}

func NewConnectionFactory(conf client_configuration.Configuration) application.ConnectionFactory {
	return &ConnectionFactory{
		conf: conf,
	}
}

func (f *ConnectionFactory) EstablishConnection(
	ctx context.Context,
) (application.ConnectionAdapter, application.CryptographyService, error) {
	connSettings, connSettingsErr := f.connectionSettings()
	if connSettingsErr != nil {
		return nil, nil, connSettingsErr
	}

	socket, socketErr := network.NewSocket(connSettings.ConnectionIP, connSettings.Port)
	if socketErr != nil {
		return nil, nil, socketErr
	}

	deadline := time.Now().Add(time.Duration(math.Max(float64(connSettings.DialTimeoutMs), 5000)) * time.Millisecond)
	handshakeCtx, handshakeCtxCancel := context.WithDeadline(ctx, deadline)
	defer handshakeCtxCancel()

	switch connSettings.Protocol {
	case settings.UDP:
		//connect to server and exchange secret
		secret := udp_connection.NewDefaultSecret(connSettings, handshake.NewHandshake(), &chacha20.DefaultUdpSession{})
		cancellableSecret := udp_connection.NewSecretWithDeadline(handshakeCtx, secret)

		session := udp_connection.NewDefaultSecureSession(network.NewUdpConnection(socket), cancellableSecret)
		cancellableSession := udp_connection.NewSecureSessionWithDeadline(handshakeCtx, session)
		return cancellableSession.Establish()
	case settings.TCP:
		//connect to server and exchange secret
		secret := tcp_connection.NewDefaultSecret(connSettings, handshake.NewHandshake(), &chacha20.DefaultTcpSession{})
		cancellableSecret := tcp_connection.NewSecretWithDeadline(handshakeCtx, secret)

		session := tcp_connection.NewDefaultSecureSession(network.NewDefaultConnection(socket), cancellableSecret)
		cancellableSession := tcp_connection.NewSecureSessionWithDeadline(handshakeCtx, session)
		return cancellableSession.Establish()
	default:
		return nil, nil, fmt.Errorf("unsupported protocol: %v", connSettings.Protocol)
	}
}

func (f *ConnectionFactory) connectionSettings() (settings.Settings, error) {
	switch f.conf.Protocol {
	case settings.TCP:
		return f.conf.TCPSettings, nil
	case settings.UDP:
		return f.conf.UDPSettings, nil
	default:
		return settings.Settings{}, fmt.Errorf("unsupported protocol: %v", f.conf.Protocol)
	}
}
