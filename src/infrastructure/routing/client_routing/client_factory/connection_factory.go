package client_factory

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/netip"
	"time"
	"tungo/application"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/network"
	"tungo/infrastructure/settings"
)

type ConnectionFactory struct {
	conf client.Configuration
}

func NewConnectionFactory(conf client.Configuration) application.ConnectionFactory {
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

	addrPort, addrPortErr := netip.ParseAddrPort(net.JoinHostPort(connSettings.ConnectionIP, connSettings.Port))
	if addrPortErr != nil {
		return nil, nil, addrPortErr
	}

	deadline := time.Now().Add(time.Duration(math.Max(float64(connSettings.DialTimeoutMs), 5000)) * time.Millisecond)
	handshakeCtx, handshakeCtxCancel := context.WithDeadline(ctx, deadline)
	defer handshakeCtxCancel()

	switch connSettings.Protocol {
	case settings.UDP:
		//connect to server and exchange secret
		secret := network.NewDefaultSecret(
			connSettings,
			handshake.NewHandshake(f.conf.Ed25519PublicKey, make([]byte, 0)),
			chacha20.NewUdpSessionBuilder(),
		)
		cancellableSecret := network.NewSecretWithDeadline(handshakeCtx, secret)

		session := network.NewDefaultSecureSession(network.NewUDPConnection(addrPort), cancellableSecret)
		cancellableSession := network.NewSecureSessionWithDeadline(handshakeCtx, session)
		return cancellableSession.Establish()
	case settings.TCP:
		//connect to server and exchange secret
		secret := network.NewDefaultSecret(
			connSettings,
			handshake.NewHandshake(f.conf.Ed25519PublicKey, make([]byte, 0)),
			chacha20.NewTcpSessionBuilder(),
		)
		cancellableSecret := network.NewSecretWithDeadline(handshakeCtx, secret)

		session := network.NewDefaultSecureSession(network.NewTCPConnection(addrPort), cancellableSecret)
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
