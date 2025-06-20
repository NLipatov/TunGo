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
		secret := network.NewDefaultSecret(connSettings, handshake.NewHandshake(), chacha20.NewUdpSessionBuilder())
		cancellableSecret := network.NewSecretWithDeadline(handshakeCtx, secret)

		session := network.NewDefaultSecureSession(network.NewUdpConnection(socket), cancellableSecret)
		cancellableSession := network.NewSecureSessionWithDeadline(handshakeCtx, session)
		return cancellableSession.Establish()
	case settings.TCP:
		//connect to server and exchange secret
		secret := network.NewDefaultSecret(connSettings, handshake.NewHandshake(), chacha20.NewTcpSessionBuilder())
		cancellableSecret := network.NewSecretWithDeadline(handshakeCtx, secret)

		session := network.NewDefaultSecureSession(network.NewTcpConnection(socket), cancellableSecret)
		cancellableSession := network.NewSecureSessionWithDeadline(handshakeCtx, session)
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
