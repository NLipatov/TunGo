package client_factory

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/netip"
	"time"
	"tungo/application/network/connection"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/noise"
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
		adapter, err := f.dialWithFallback(establishCtx, connSettings, f.dialUDP)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to establish UDP connection: %w", err)
		}

		return f.establishSecuredConnection(establishCtx, connSettings, adapter, chacha20.NewUdpSessionBuilder(
			chacha20.NewDefaultAEADBuilder()),
		)
	case settings.TCP:
		adapter, err := f.dialWithFallback(establishCtx, connSettings, f.dialTCP)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to establish TCP connection: %w", err)
		}

		return f.establishSecuredConnection(establishCtx, connSettings, adapter, chacha20.NewTcpSessionBuilder(
			chacha20.NewDefaultAEADBuilder()),
		)
	case settings.WS:
		adapter, err := f.dialWSWithFallback(establishCtx, ctx, connSettings, "ws")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to establish WebSocket connection: %w", err)
		}

		return f.establishSecuredConnection(establishCtx, connSettings, adapter, chacha20.NewTcpSessionBuilder(
			chacha20.NewDefaultAEADBuilder()),
		)
	case settings.WSS:
		adapter, err := f.dialWSWithFallback(establishCtx, ctx, connSettings, "wss")
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
	// IK handshake requires client keys
	if len(f.conf.ClientPublicKey) != 32 || len(f.conf.ClientPrivateKey) != 32 {
		_ = adapter.Close()
		return nil, nil, nil, fmt.Errorf("client keys not configured (required for IK handshake)")
	}

	handshake := noise.NewIKHandshakeClient(
		f.conf.ClientPublicKey,
		f.conf.ClientPrivateKey,
		f.conf.X25519PublicKey,
	)

	secret := network.NewDefaultSecret(
		s,
		handshake,
		cryptoFactory,
	)
	cancellableSecret := network.NewSecretWithDeadline(ctx, secret)
	cr, ctrl, err := cancellableSecret.Exchange(adapter)
	if err != nil {
		_ = adapter.Close()
		return nil, nil, nil, err
	}
	return adapter, cr, ctrl, nil
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

	return adapters.NewLengthPrefixFramingAdapter(
		adapters.NewReadDeadlineTransport(conn, settings.PingRestartTimeout),
		settings.DefaultEthernetMTU+settings.TCPChacha20Overhead,
	)
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

const ipv6FallbackTimeout = 2 * time.Second

func (f *ConnectionFactory) dialWithFallback(
	ctx context.Context,
	s settings.Settings,
	dialFn func(context.Context, netip.AddrPort) (connection.Transport, error),
) (connection.Transport, error) {
	if !s.IPv6Host.IsZero() {
		ipv6AP, err := s.IPv6Host.AddrPort(s.Port)
		if err == nil {
			ipv6Ctx, cancel := context.WithTimeout(ctx, ipv6FallbackTimeout)
			transport, dialErr := dialFn(ipv6Ctx, ipv6AP)
			cancel()
			if dialErr == nil {
				return transport, nil
			}
		}
	}
	ap, err := s.Host.AddrPort(s.Port)
	if err != nil {
		return nil, err
	}
	return dialFn(ctx, ap)
}

func (f *ConnectionFactory) dialWSWithFallback(
	establishCtx, connCtx context.Context,
	s settings.Settings,
	scheme string,
) (connection.Transport, error) {
	port := s.Port
	if scheme == "wss" && port == 0 {
		port = 443
	}

	if !s.IPv6Host.IsZero() {
		endpoint, err := s.IPv6Host.Endpoint(port)
		if err == nil {
			ipv6Ctx, cancel := context.WithTimeout(establishCtx, ipv6FallbackTimeout)
			adapter, dialErr := f.dialWS(ipv6Ctx, connCtx, scheme, endpoint)
			cancel()
			if dialErr == nil {
				return adapter, nil
			}
		}
	}
	endpoint, err := s.Host.Endpoint(port)
	if err != nil {
		return nil, err
	}
	return f.dialWS(establishCtx, connCtx, scheme, endpoint)
}

func (f *ConnectionFactory) dialWS(
	establishCtx, connCtx context.Context,
	scheme, endpoint string,
) (connection.Transport, error) {
	url := fmt.Sprintf("%s://%s/ws", scheme, endpoint)
	conn, resp, err := websocket.Dial(establishCtx, url, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return adapters.NewLengthPrefixFramingAdapter(
		adapters.NewReadDeadlineTransport(wsAdapters.NewDefaultAdapter(connCtx, conn, nil, nil), settings.PingRestartTimeout),
		settings.DefaultEthernetMTU+settings.TCPChacha20Overhead,
	)
}

