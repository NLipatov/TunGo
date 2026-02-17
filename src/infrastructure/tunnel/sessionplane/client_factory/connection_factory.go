package client_factory

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/netip"
	"sync"
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
	connSettings, connSettingsErr := f.conf.ActiveSettings()
	if connSettingsErr != nil {
		return nil, nil, nil, connSettingsErr
	}
	// Use explicitly selected protocol from configuration to avoid implicit fallback
	// when the settings bucket protocol is stale/mismatched.
	connSettings.Protocol = f.conf.Protocol

	deadline := time.Now().Add(time.Duration(math.Max(float64(connSettings.DialTimeoutMs), 5000)) * time.Millisecond)
	establishCtx, establishCancel := context.WithDeadline(ctx, deadline)
	defer establishCancel()

	adapter, err := f.dial(establishCtx, ctx, connSettings)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to establish %s connection: %w", connSettings.Protocol, err)
	}

	builder := f.sessionBuilder(connSettings.Protocol)
	return f.establishSecuredConnection(establishCtx, adapter, builder)
}

func (f *ConnectionFactory) dial(
	establishCtx, connCtx context.Context,
	s settings.Settings,
) (connection.Transport, error) {
	switch s.Protocol {
	case settings.UDP:
		return f.dialWithFallback(establishCtx, s, f.dialUDP)
	case settings.TCP:
		return f.dialWithFallback(establishCtx, s, f.dialTCP)
	case settings.WS:
		return f.dialWSWithFallback(establishCtx, connCtx, s, "ws")
	case settings.WSS:
		return f.dialWSWithFallback(establishCtx, connCtx, s, "wss")
	default:
		return nil, fmt.Errorf("unsupported protocol: %v", s.Protocol)
	}
}

func (f *ConnectionFactory) sessionBuilder(proto settings.Protocol) connection.CryptoFactory {
	if proto == settings.UDP {
		return chacha20.NewUdpSessionBuilder(chacha20.NewDefaultAEADBuilder())
	}
	return chacha20.NewTcpSessionBuilder(chacha20.NewDefaultAEADBuilder())
}

func (f *ConnectionFactory) establishSecuredConnection(
	ctx context.Context,
	adapter connection.Transport,
	cryptoFactory connection.CryptoFactory,
) (connection.Transport, connection.Crypto, *rekey.StateMachine, error) {
	// IK handshake requires client keys
	if len(f.conf.ClientPublicKey) != 32 || len(f.conf.ClientPrivateKey) != 32 {
		_ = adapter.Close()
		return nil, nil, nil, fmt.Errorf("client keys not configured (required for IK handshake)")
	}
	if len(f.conf.X25519PublicKey) != 32 {
		_ = adapter.Close()
		return nil, nil, nil, fmt.Errorf("server public key not configured (required for IK handshake)")
	}

	handshake := noise.NewIKHandshakeClient(
		f.conf.ClientPublicKey,
		f.conf.ClientPrivateKey,
		f.conf.X25519PublicKey,
	)

	secret := network.NewDefaultSecret(
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

	transport := adapters.NewReadDeadlineTransport(conn, settings.PingRestartTimeout)
	if remote := parseNetAddrPort(conn.RemoteAddr()); remote.IsValid() {
		transport = adapters.NewRemoteAddrTransport(transport, remote)
	}

	return adapters.NewLengthPrefixFramingAdapter(
		transport,
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
	if remote := parseNetAddrPort(conn.RemoteAddr()); remote.IsValid() {
		return adapters.NewRemoteAddrTransport(conn, remote), nil
	}
	return conn, nil
}

const minimumIPv6ProbeTimeout = 2 * time.Second

func (f *ConnectionFactory) dialWithFallback(
	ctx context.Context,
	s settings.Settings,
	dialFn func(context.Context, netip.AddrPort) (connection.Transport, error),
) (connection.Transport, error) {
	preferredAP, preferredErr := resolvePreferredAddrPort(ctx, s)
	if preferredErr != nil {
		if ipv6AP, ipv6Err := resolveIPv6AddrPort(ctx, s); ipv6Err == nil {
			return dialFn(ctx, ipv6AP)
		}
		return nil, preferredErr
	}

	ipv6AP, ipv6Err := resolveIPv6AddrPort(ctx, s)
	if ipv6Err != nil {
		return dialFn(ctx, preferredAP)
	}

	// IPv6-only path: avoid probing then retrying the exact same endpoint.
	if ipv6AP == preferredAP {
		return dialFn(ctx, preferredAP)
	}

	ipv6Ctx, cancel := context.WithTimeout(ctx, ipv6ProbeTimeout(s))
	transport, dialErr := dialFn(ipv6Ctx, ipv6AP)
	cancel()
	if dialErr == nil {
		return transport, nil
	}
	return dialFn(ctx, preferredAP)
}

func (f *ConnectionFactory) dialWSWithFallback(
	establishCtx, connCtx context.Context,
	s settings.Settings,
	scheme string,
) (connection.Transport, error) {
	return f.dialWSWithFallbackUsing(establishCtx, connCtx, s, scheme, f.dialWS)
}

func (f *ConnectionFactory) dialWSWithFallbackUsing(
	establishCtx, connCtx context.Context,
	s settings.Settings,
	scheme string,
	dialFn func(context.Context, context.Context, string, string) (connection.Transport, error),
) (connection.Transport, error) {
	port := s.Port
	if scheme == "wss" && port == 0 {
		port = 443
	}

	endpoint, err := s.Server.Endpoint(port)
	if err != nil {
		return nil, err
	}

	if s.Server.HasIPv6() {
		ipv6Endpoint, err := s.Server.IPv6Endpoint(port)
		if err == nil {
			// IPv6-only path: no reason to probe then retry the same endpoint.
			if ipv6Endpoint == endpoint {
				return dialFn(establishCtx, connCtx, scheme, endpoint)
			}
			ipv6Ctx, cancel := context.WithTimeout(establishCtx, ipv6ProbeTimeout(s))
			adapter, dialErr := dialFn(ipv6Ctx, connCtx, scheme, ipv6Endpoint)
			cancel()
			if dialErr == nil {
				return adapter, nil
			}
		}
	}
	return dialFn(establishCtx, connCtx, scheme, endpoint)
}

func (f *ConnectionFactory) dialWS(
	establishCtx, connCtx context.Context,
	scheme, endpoint string,
) (connection.Transport, error) {
	url := fmt.Sprintf("%s://%s/ws", scheme, endpoint)
	opts, remoteAddr := newWSDialOptionsWithRemoteCapture()
	conn, resp, err := websocket.Dial(establishCtx, url, opts)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	wrapped, wrapErr := adapters.NewLengthPrefixFramingAdapter(
		adapters.NewReadDeadlineTransport(wsAdapters.NewDefaultAdapter(connCtx, conn, nil, nil), settings.PingRestartTimeout),
		settings.DefaultEthernetMTU+settings.TCPChacha20Overhead,
	)
	if wrapErr != nil {
		_ = conn.Close(websocket.StatusInternalError, "adapter wrap failed")
		return nil, wrapErr
	}
	if remote := parseNetAddrPort(remoteAddr()); remote.IsValid() {
		return adapters.NewRemoteAddrTransport(wrapped, remote), nil
	}
	if remote := parseEndpointAddrPort(endpoint); remote.IsValid() {
		return adapters.NewRemoteAddrTransport(wrapped, remote), nil
	}
	return wrapped, nil
}

func ipv6ProbeTimeout(s settings.Settings) time.Duration {
	timeout := time.Duration(s.DialTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	probe := timeout / 2
	if probe < minimumIPv6ProbeTimeout {
		return minimumIPv6ProbeTimeout
	}
	return probe
}

func newWSDialOptionsWithRemoteCapture() (*websocket.DialOptions, func() net.Addr) {
	var (
		mu   sync.Mutex
		addr net.Addr
	)

	dialer := &net.Dialer{}
	transport := cloneDefaultTransport()
	transport.DialContext = func(ctx context.Context, network, target string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, target)
		if err != nil {
			return nil, err
		}
		mu.Lock()
		addr = conn.RemoteAddr()
		mu.Unlock()
		return conn, nil
	}

	return &websocket.DialOptions{
			HTTPClient: &http.Client{Transport: transport},
		}, func() net.Addr {
			mu.Lock()
			defer mu.Unlock()
			return addr
		}
}

func cloneDefaultTransport() *http.Transport {
	if base, ok := http.DefaultTransport.(*http.Transport); ok && base != nil {
		return base.Clone()
	}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

func resolveIPv6AddrPort(ctx context.Context, s settings.Settings) (netip.AddrPort, error) {
	if ap, err := s.Server.IPv6AddrPort(s.Port); err == nil {
		return ap, nil
	} else if _, isDomain := s.Server.Domain(); !isDomain {
		return netip.AddrPort{}, err
	}
	return resolveDomainAddrPort(ctx, s, true)
}

func resolvePreferredAddrPort(ctx context.Context, s settings.Settings) (netip.AddrPort, error) {
	if ap, err := s.Server.AddrPort(s.Port); err == nil {
		return ap, nil
	} else if _, isDomain := s.Server.Domain(); !isDomain {
		return netip.AddrPort{}, err
	}
	if ap4, err4 := resolveDomainAddrPort(ctx, s, false); err4 == nil {
		return ap4, nil
	}
	return resolveDomainAddrPort(ctx, s, true)
}

func resolveDomainAddrPort(ctx context.Context, s settings.Settings, wantIPv6 bool) (netip.AddrPort, error) {
	if s.Port < 1 || s.Port > 65535 {
		return netip.AddrPort{}, fmt.Errorf("invalid port: %d", s.Port)
	}

	var (
		raw string
		err error
	)
	if wantIPv6 {
		raw, err = s.Server.RouteIPv6Context(ctx)
	} else {
		raw, err = s.Server.RouteIPv4Context(ctx)
	}
	if err != nil {
		return netip.AddrPort{}, err
	}

	ip, parseErr := netip.ParseAddr(raw)
	if parseErr != nil {
		return netip.AddrPort{}, parseErr
	}
	isIPv4 := ip.Unmap().Is4()
	if wantIPv6 && isIPv4 {
		return netip.AddrPort{}, fmt.Errorf("resolved IPv4 %q, expected IPv6", raw)
	}
	if !wantIPv6 && !isIPv4 {
		return netip.AddrPort{}, fmt.Errorf("resolved IPv6 %q, expected IPv4", raw)
	}
	if isIPv4 {
		ip = ip.Unmap()
	}
	return netip.AddrPortFrom(ip, uint16(s.Port)), nil
}

func parseEndpointAddrPort(endpoint string) netip.AddrPort {
	ap, err := netip.ParseAddrPort(endpoint)
	if err == nil {
		return ap
	}

	host, portStr, splitErr := net.SplitHostPort(endpoint)
	if splitErr != nil {
		return netip.AddrPort{}
	}
	ip, ipErr := netip.ParseAddr(host)
	if ipErr != nil {
		return netip.AddrPort{}
	}
	port, portErr := net.LookupPort("tcp", portStr)
	if portErr != nil || port < 1 || port > 65535 {
		return netip.AddrPort{}
	}
	if ip.Unmap().Is4() {
		ip = ip.Unmap()
	}
	return netip.AddrPortFrom(ip, uint16(port))
}

func parseNetAddrPort(addr net.Addr) netip.AddrPort {
	if addr == nil {
		return netip.AddrPort{}
	}
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		ip, ok := netip.AddrFromSlice(tcpAddr.IP)
		if !ok {
			return netip.AddrPort{}
		}
		if ip.Unmap().Is4() {
			ip = ip.Unmap()
		}
		return netip.AddrPortFrom(ip, uint16(tcpAddr.Port))
	}
	if udpAddr, ok := addr.(*net.UDPAddr); ok {
		ip, ok := netip.AddrFromSlice(udpAddr.IP)
		if !ok {
			return netip.AddrPort{}
		}
		if ip.Unmap().Is4() {
			ip = ip.Unmap()
		}
		return netip.AddrPortFrom(ip, uint16(udpAddr.Port))
	}
	if ap, err := netip.ParseAddrPort(addr.String()); err == nil {
		return ap
	}
	return netip.AddrPort{}
}
