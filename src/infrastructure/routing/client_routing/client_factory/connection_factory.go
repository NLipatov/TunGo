package client_factory

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/netip"
	"strconv"
	"time"
	"tungo/application/network/connection"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/handshake"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/network"
	"tungo/infrastructure/network/tcp/adapters"
	wsAdapters "tungo/infrastructure/network/ws/adapter"
	"tungo/infrastructure/settings"

	"github.com/coder/websocket"
)

type ConnectionFactory struct {
	conf client.Configuration
}

func NewConnectionFactory(conf client.Configuration) connection.Factory {
	return &ConnectionFactory{
		conf: conf,
	}
}

func (f *ConnectionFactory) EstablishConnection(
	ctx context.Context,
) (connection.Transport, connection.Crypto, *rekey.StateMachine, error) {
	connSettings, connSettingsErr := f.connectionSettings()
	if connSettingsErr != nil {
		return nil, nil, nil, connSettingsErr
	}

	deadline := time.Now().Add(time.Duration(math.Max(float64(connSettings.DialTimeoutMs), 5000)) * time.Millisecond)
	establishCtx, establishCancel := context.WithDeadline(ctx, deadline)
	defer establishCancel()

	switch connSettings.Protocol {
	case settings.UDP:
		ap, apErr := netip.ParseAddrPort(net.JoinHostPort(connSettings.ConnectionIP, connSettings.Port))
		if apErr != nil {
			return nil, nil, nil, apErr
		}

		adapter, err := f.dialUDP(establishCtx, ap)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to establish UDP connection: %w", err)
		}

		return f.establishSecuredConnection(establishCtx, connSettings, adapter, chacha20.NewUdpSessionBuilder(
			chacha20.NewDefaultAEADBuilder()),
		)
	case settings.TCP:
		ap, apErr := netip.ParseAddrPort(net.JoinHostPort(connSettings.ConnectionIP, connSettings.Port))
		if apErr != nil {
			return nil, nil, nil, apErr
		}

		adapter, err := f.dialTCP(establishCtx, ap)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to establish TCP connection: %w", err)
		}

		return f.establishSecuredConnection(establishCtx, connSettings, adapter, chacha20.NewTcpSessionBuilder(
			chacha20.NewDefaultAEADBuilder()),
		)
	case settings.WS:
		scheme := "ws"
		host := connSettings.Host
		if host == "" {
			host = connSettings.ConnectionIP
		}
		if host == "" {
			return nil, nil, nil, fmt.Errorf("ws dial: empty host (neither Host nor ConnectionIP provided)")
		}
		port := connSettings.Port
		if port == "" {
			return nil, nil, nil, fmt.Errorf("ws dial: empty port")
		}
		if pn, err := strconv.Atoi(port); err != nil || pn < 1 || pn > 65535 {
			return nil, nil, nil, fmt.Errorf("ws dial: invalid port: %q", port)
		}
		adapter, err := f.dialWS(establishCtx, ctx, scheme, host, port)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to establish WebSocket connection: %w", err)
		}

		return f.establishSecuredConnection(establishCtx, connSettings, adapter, chacha20.NewTcpSessionBuilder(
			chacha20.NewDefaultAEADBuilder()),
		)
	case settings.WSS:
		scheme := "wss"
		host := connSettings.Host
		if host == "" {
			return nil, nil, nil, fmt.Errorf("wss dial: empty host")
		}
		port := connSettings.Port
		if port == "" {
			port = "443"
		}
		if portNumber, err := strconv.Atoi(port); err != nil {
			return nil, nil, nil, err
		} else if portNumber < 1 || portNumber > 65535 {
			return nil, nil, nil, fmt.Errorf("wss dial: invalid port: %d", portNumber)
		}
		adapter, err := f.dialWS(establishCtx, ctx, scheme, host, port)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to establish WebSocket connection: %w", err)
		}

		return f.establishSecuredConnection(establishCtx, connSettings, adapter, chacha20.NewTcpSessionBuilder(
			chacha20.NewDefaultAEADBuilder()),
		)
	default:
		return nil, nil, nil, fmt.Errorf("unsupported protocol: %v", connSettings.Protocol)
	}
}

func (f *ConnectionFactory) connectionSettings() (settings.Settings, error) {
	switch f.conf.Protocol {
	case settings.TCP:
		return f.conf.TCPSettings, nil
	case settings.UDP:
		return f.conf.UDPSettings, nil
	case settings.WS:
		return f.conf.WSSettings, nil
	case settings.WSS:
		return f.conf.WSSettings, nil
	default:
		return settings.Settings{}, fmt.Errorf("unsupported protocol: %v", f.conf.Protocol)
	}
}

func (f *ConnectionFactory) establishSecuredConnection(
	ctx context.Context,
	s settings.Settings,
	adapter connection.Transport,
	cryptoFactory connection.CryptoFactory,
) (connection.Transport, connection.Crypto, *rekey.StateMachine, error) {
	//connect to server and exchange secret
	secret := network.NewDefaultSecret(
		s,
		handshake.NewHandshake(f.conf.Ed25519PublicKey, nil),
		cryptoFactory,
	)
	cancellableSecret := network.NewSecretWithDeadline(ctx, secret)

	session := network.NewDefaultSecureSession(adapter, cancellableSecret)
	cancellableSession := network.NewSecureSessionWithDeadline(ctx, session)
	ad, cr, ctrl, err := cancellableSession.Establish()
	if err != nil {
		_ = adapter.Close()
		return nil, nil, nil, err
	}
	return ad, cr, ctrl, nil
}

func (f *ConnectionFactory) dialTCP(
	ctx context.Context,
	ap netip.AddrPort,
) (connection.Transport, error) {
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

	return adapters.NewLengthPrefixFramingAdapter(conn, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
}

func (f *ConnectionFactory) dialUDP(
	ctx context.Context,
	ap netip.AddrPort,
) (connection.Transport, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", ap.String())
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func (f *ConnectionFactory) dialWS(
	establishCtx, connCtx context.Context,
	scheme, host, port string,
) (connection.Transport, error) {
	url := fmt.Sprintf("%s://%s/ws", scheme, net.JoinHostPort(host, port))
	conn, _, err := websocket.Dial(establishCtx, url, nil)
	if err != nil {
		return nil, err
	}

	return adapters.NewLengthPrefixFramingAdapter(
		wsAdapters.NewDefaultAdapter(connCtx, conn, nil, nil),
		settings.DefaultEthernetMTU+settings.TCPChacha20Overhead,
	)
}
