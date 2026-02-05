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
		newReadDeadlineTransport(conn, settings.PingRestartTimeout),
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

func (f *ConnectionFactory) dialWS(
	establishCtx, connCtx context.Context,
	scheme, host, port string,
) (connection.Transport, error) {
	url := fmt.Sprintf("%s://%s/ws", scheme, net.JoinHostPort(host, port))
	conn, resp, err := websocket.Dial(establishCtx, url, nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	return adapters.NewLengthPrefixFramingAdapter(
		newReadDeadlineTransport(wsAdapters.NewDefaultAdapter(connCtx, conn, nil, nil), settings.PingRestartTimeout),
		settings.DefaultEthernetMTU+settings.TCPChacha20Overhead,
	)
}

// readDeadlineTransport wraps a Transport and refreshes a read deadline before
// each Read call. If the underlying transport does not support SetReadDeadline,
// the wrapper is a no-op pass-through.
type readDeadlineTransport struct {
	connection.Transport
	ds      interface{ SetReadDeadline(time.Time) error }
	timeout time.Duration
}

func newReadDeadlineTransport(t connection.Transport, timeout time.Duration) connection.Transport {
	ds, ok := t.(interface{ SetReadDeadline(time.Time) error })
	if !ok {
		return t
	}
	return &readDeadlineTransport{Transport: t, timeout: timeout, ds: ds}
}

func (d *readDeadlineTransport) Read(p []byte) (int, error) {
	_ = d.ds.SetReadDeadline(time.Now().Add(d.timeout))
	return d.Transport.Read(p)
}
