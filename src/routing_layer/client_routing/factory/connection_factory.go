package factory

import (
	"context"
	"fmt"
	"math"
	"net"
	"time"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	tcp_connection2 "tungo/routing_layer/client_routing/routing/tcp_chacha20/tcp_connection"
	udp_connection2 "tungo/routing_layer/client_routing/routing/udp_chacha20/udp_connection"
	"tungo/settings"
	"tungo/settings/client_configuration"
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
) (net.Conn, application.CryptographyService, error) {
	connSettings, connSettingsErr := f.connectionSettings()
	if connSettingsErr != nil {
		return nil, nil, connSettingsErr
	}

	deadline := time.Now().Add(time.Duration(math.Max(float64(connSettings.DialTimeoutMs), 5000)) * time.Millisecond)
	handshakeCtx, handshakeCtxCancel := context.WithDeadline(ctx, deadline)
	defer handshakeCtxCancel()

	switch connSettings.Protocol {
	case settings.UDP:
		//connect to server and exchange secret
		secret := udp_connection2.NewDefaultSecret(connSettings, chacha20.NewHandshake())
		cancellableSecret := udp_connection2.NewSecretWithDeadline(handshakeCtx, secret)

		session := udp_connection2.NewDefaultSecureSession(udp_connection2.NewConnection(connSettings), cancellableSecret)
		cancellableSession := udp_connection2.NewSecureSessionWithDeadline(handshakeCtx, session)
		return cancellableSession.Establish()
	case settings.TCP:
		//connect to server and exchange secret
		secret := tcp_connection2.NewDefaultSecret(connSettings, chacha20.NewHandshake())
		cancellableSecret := tcp_connection2.NewSecretWithDeadline(handshakeCtx, secret)

		session := tcp_connection2.NewDefaultSecureSession(tcp_connection2.NewDefaultConnection(connSettings), cancellableSecret)
		cancellableSession := tcp_connection2.NewSecureSessionWithDeadline(handshakeCtx, session)
		return cancellableSession.Establish()
	default:
		return nil, nil, fmt.Errorf("unsupported protocol: %v", connSettings.Protocol)
	}
}

func (f *ConnectionFactory) connectionSettings() (settings.ConnectionSettings, error) {
	switch f.conf.Protocol {
	case settings.TCP:
		return f.conf.TCPSettings, nil
	case settings.UDP:
		return f.conf.UDPSettings, nil
	default:
		return settings.ConnectionSettings{}, fmt.Errorf("unsupported protocol: %v", f.conf.Protocol)
	}
}
