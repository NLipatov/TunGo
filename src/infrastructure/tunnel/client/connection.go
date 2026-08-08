package client

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"
	"tungo/application/configuration"
	"tungo/application/configuration/settings"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/tcp"
	"tungo/infrastructure/cryptography/chacha20/udp"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/network/host_resolver"
	"tungo/infrastructure/network/tcp/adapters"
	"tungo/infrastructure/network/ws"

	"github.com/coder/websocket"
)

type connectionFactory struct {
	conf configuration.ClientRuntimeConfiguration
}

func newConnection(conf configuration.ClientRuntimeConfiguration) *connectionFactory {
	return &connectionFactory{
		conf: conf,
	}
}

func (f *connectionFactory) EstablishConnection(
	ctx context.Context,
) (io.ReadWriteCloser, crypto, rekeyController, error) {
	connSettings := f.conf.Settings

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

func (f *connectionFactory) dial(
	establishCtx, connCtx context.Context,
	s settings.Settings,
) (io.ReadWriteCloser, error) {
	switch s.Protocol {
	case settings.UDP:
		return f.dialWithFallback(establishCtx, s, f.dialUDP)
	case settings.TCP:
		return f.dialWithFallback(establishCtx, s, f.dialTCP)
	case settings.WS, settings.WSS:
		return f.dialWSWithFallback(establishCtx, connCtx, s)
	default:
		return nil, fmt.Errorf("unsupported protocol: %v", s.Protocol)
	}
}

type buildCrypto func(chacha20.KeyMaterial, bool) (crypto, rekeyController, error)

func (f *connectionFactory) sessionBuilder(proto settings.Protocol) buildCrypto {
	if proto == settings.UDP {
		return func(handshake chacha20.KeyMaterial, isServer bool) (crypto, rekeyController, error) {
			return udp.NewFromHandshake(handshake, isServer)
		}
	}
	return func(handshake chacha20.KeyMaterial, isServer bool) (crypto, rekeyController, error) {
		return tcp.NewFromHandshake(handshake, isServer)
	}
}

func (f *connectionFactory) establishSecuredConnection(
	ctx context.Context,
	adapter io.ReadWriteCloser,
	buildCrypto buildCrypto,
) (io.ReadWriteCloser, crypto, rekeyController, error) {
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

	var closeOnce sync.Once
	closeAdapter := func() {
		closeOnce.Do(func() { _ = adapter.Close() })
	}
	cancelCloseOnContextDone := context.AfterFunc(ctx, closeAdapter)

	if err := handshake.ClientSideHandshake(adapter); err != nil {
		cancelCloseOnContextDone()
		closeAdapter()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, nil, ctxErr
		}
		return nil, nil, nil, err
	}

	cr, ctrl, err := buildCrypto(handshake, false)
	cancelCloseOnContextDone()
	if ctxErr := ctx.Err(); ctxErr != nil {
		closeAdapter()
		return nil, nil, nil, ctxErr
	}
	if err != nil {
		closeAdapter()
		return nil, nil, nil, fmt.Errorf("failed to create client crypto: %w", err)
	}
	return adapter, cr, ctrl, nil
}

func (f *connectionFactory) dialTCP(
	ctx context.Context,
	ap netip.AddrPort,
) (io.ReadWriteCloser, error) {
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

func (f *connectionFactory) dialUDP(
	ctx context.Context,
	ap netip.AddrPort,
) (io.ReadWriteCloser, error) {
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

func (f *connectionFactory) dialWithFallback(
	ctx context.Context,
	s settings.Settings,
	dialFn func(context.Context, netip.AddrPort) (io.ReadWriteCloser, error),
) (io.ReadWriteCloser, error) {
	preferredAP, preferredErr := resolvePreferredAddrPort(ctx, s)
	if preferredErr != nil {
		if ipv6AP, ipv6Err := resolveIPv6AddrPort(ctx, s); ipv6Err == nil {
			return dialFn(ctx, ipv6AP)
		}
		return nil, preferredErr
	}
	// UDP dial only creates a connected socket; it does not prove endpoint reachability.
	// Prefer the default address, but still fall back on immediate local dial errors.
	if s.Protocol == settings.UDP {
		transport, dialErr := dialFn(ctx, preferredAP)
		if dialErr == nil {
			return transport, nil
		}
		if ipv6AP, ipv6Err := resolveIPv6AddrPort(ctx, s); ipv6Err == nil && ipv6AP != preferredAP {
			return dialFn(ctx, ipv6AP)
		}
		return nil, dialErr
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

func (f *connectionFactory) dialWSWithFallback(
	establishCtx, connCtx context.Context,
	s settings.Settings,
) (io.ReadWriteCloser, error) {
	scheme := "ws"
	if s.Protocol == settings.WSS {
		scheme = "wss"
	}

	port := s.Port
	if scheme == "wss" && port == 0 {
		port = 443
	}

	endpoint := net.JoinHostPort(preferredHost(s.Server), strconv.Itoa(port))

	if s.Server.IPv6 != "" {
		ipv6Endpoint := net.JoinHostPort(s.Server.IPv6, strconv.Itoa(port))
		// IPv6-only path: no reason to probe then retry the same endpoint.
		if ipv6Endpoint == endpoint {
			return f.dialWS(establishCtx, connCtx, scheme, endpoint)
		}
		ipv6Ctx, cancel := context.WithTimeout(establishCtx, ipv6ProbeTimeout(s))
		adapter, dialErr := f.dialWS(ipv6Ctx, connCtx, scheme, ipv6Endpoint)
		cancel()
		if dialErr == nil {
			return adapter, nil
		}
	}
	return f.dialWS(establishCtx, connCtx, scheme, endpoint)
}

func preferredHost(host settings.Host) string {
	if host.Domain != "" {
		return host.Domain
	}
	if host.IPv4 != "" {
		return host.IPv4
	}
	return host.IPv6
}

func (f *connectionFactory) dialWS(
	establishCtx, connCtx context.Context,
	scheme, endpoint string,
) (io.ReadWriteCloser, error) {
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
		adapters.NewReadDeadlineTransport(ws.NewConn(connCtx, conn, nil, nil), settings.PingRestartTimeout),
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
	if s.Server.IPv6 != "" {
		ip, err := netip.ParseAddr(s.Server.IPv6)
		if err != nil {
			return netip.AddrPort{}, err
		}
		return addrPort(ip, s.Port)
	}
	if s.Server.Domain == "" {
		return netip.AddrPort{}, fmt.Errorf("host has no IPv6 address")
	}
	return resolveDomainAddrPort(ctx, s, true)
}

func resolvePreferredAddrPort(ctx context.Context, s settings.Settings) (netip.AddrPort, error) {
	rawIP := s.Server.IPv4
	if rawIP == "" {
		rawIP = s.Server.IPv6
	}
	if rawIP != "" {
		ip, err := netip.ParseAddr(rawIP)
		if err != nil {
			return netip.AddrPort{}, err
		}
		return addrPort(ip, s.Port)
	}
	if s.Server.Domain == "" {
		return netip.AddrPort{}, fmt.Errorf("server host is empty")
	}
	if ap4, err4 := resolveDomainAddrPort(ctx, s, false); err4 == nil {
		return ap4, nil
	}
	return resolveDomainAddrPort(ctx, s, true)
}

func resolveDomainAddrPort(ctx context.Context, s settings.Settings, wantIPv6 bool) (netip.AddrPort, error) {
	if err := validatePort(s.Port); err != nil {
		return netip.AddrPort{}, err
	}

	var (
		raw string
		err error
	)
	if wantIPv6 {
		raw, err = host_resolver.ResolveIPv6(ctx, s.Server)
	} else {
		raw, err = host_resolver.ResolveIPv4(ctx, s.Server)
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

func addrPort(ip netip.Addr, port int) (netip.AddrPort, error) {
	if !ip.IsValid() {
		return netip.AddrPort{}, fmt.Errorf("invalid IP address")
	}
	if err := validatePort(port); err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(ip.Unmap(), uint16(port)), nil
}

func validatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	return nil
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
