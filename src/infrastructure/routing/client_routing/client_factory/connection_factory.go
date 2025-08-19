package client_factory

import (
	"context"
	"fmt"
	"github.com/coder/websocket"
	"math"
	"net"
	"net/netip"
	"time"
	"tungo/application"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/network"
	"tungo/infrastructure/network/tcp/adapters"
	"tungo/infrastructure/network/ws"
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
	establishCtx, establishCancel := context.WithDeadline(ctx, deadline)
	defer establishCancel()

	switch connSettings.Protocol {
	case settings.UDP:
		adapter, err := f.dialUDP(establishCtx, addrPort)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to establish UDP connection: %w", err)
		}

		return f.establishSecuredConnection(establishCtx, connSettings, adapter, chacha20.NewUdpSessionBuilder(
			chacha20.NewDefaultAEADBuilder()),
		)
	case settings.TCP:
		adapter, err := f.dialTCP(establishCtx, addrPort)
		if err != nil {
			return nil, nil, fmt.Errorf("unable to establish TCP connection: %w", err)
		}

		return f.establishSecuredConnection(establishCtx, connSettings, adapter, chacha20.NewTcpSessionBuilder(
			chacha20.NewDefaultAEADBuilder()),
		)
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

func (f *ConnectionFactory) establishSecuredConnection(
	ctx context.Context,
	s settings.Settings,
	adapter application.ConnectionAdapter,
	cryptoFactory application.CryptographyServiceFactory,
) (application.ConnectionAdapter, application.CryptographyService, error) {
	//connect to server and exchange secret
	secret := network.NewDefaultSecret(
		s,
		handshake.NewHandshake(f.conf.Ed25519PublicKey, nil),
		cryptoFactory,
	)
	cancellableSecret := network.NewSecretWithDeadline(ctx, secret)

	session := network.NewDefaultSecureSession(adapter, cancellableSecret)
	cancellableSession := network.NewSecureSessionWithDeadline(ctx, session)
	ad, cr, err := cancellableSession.Establish()
	if err != nil {
		_ = adapter.Close()
		return nil, nil, err
	}
	return ad, cr, nil
}

func (f *ConnectionFactory) dialTCP(ctx context.Context, ap netip.AddrPort) (application.ConnectionAdapter, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", ap.String())
	if err != nil {
		return nil, err
	}

	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}

	return adapters.NewLengthPrefixFramingAdapter(conn), nil
}

func (f *ConnectionFactory) dialUDP(ctx context.Context, ap netip.AddrPort) (application.ConnectionAdapter, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", ap.String())
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (f *ConnectionFactory) dialWS(ctx context.Context, ap netip.AddrPort) (application.ConnectionAdapter, error) {
	wsAP := fmt.Sprintf("ws://%s", ap.String())
	conn, _, err := websocket.Dial(ctx, wsAP, nil)
	if err != nil {
		return nil, err
	}

	wsAdapter := ws.NewWebsocketAdapter(ctx, conn)
	return adapters.NewLengthPrefixFramingAdapter(wsAdapter), nil
}
