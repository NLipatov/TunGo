package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"time"
	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/protocol/chacha20/tcp"
	"tungo/internal/protocol/chacha20/udp"
	"tungo/internal/protocol/keys"
	"tungo/internal/protocol/noise"
	"tungo/internal/protocol/rekey"
	"tungo/internal/transport/host"
	tcptransport "tungo/internal/transport/tcp"
	"tungo/internal/transport/ws"
	"unicode/utf8"

	"github.com/coder/websocket"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/idna"
	"golang.org/x/net/proxy"
)

type epochController interface {
	ReadyForRekey() bool
	SendEpoch() uint16
	StartRekey(c2s, s2c []byte) (uint16, error)
	ActivateSendEpoch(uint16)
	ObservePeerEpoch(uint16)
	CurrentKeys() (clientToServer, serverToClient []byte)
}

type rekeyV2Handshake interface {
	StartRekeyV2(prologue []byte) ([]byte, error)
	FinishRekeyV2(msg2 []byte) (c2s, s2c []byte, err error)
}

func (c *Client) connect(
	ctx context.Context,
	configuration client.Configuration,
) (io.ReadWriteCloser, crypto, clientRekey, error) {
	settings, err := configuration.ActiveSettings()
	if err != nil {
		return nil, nil, nil, err
	}
	deadline := time.Now().Add(time.Duration(math.Max(float64(settings.DialTimeoutMs), 5000)) * time.Millisecond)
	connectCtx, establishCancel := context.WithDeadline(ctx, deadline)
	defer establishCancel()

	adapter, err := dial(connectCtx, ctx, settings)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to establish %s connection: %w", settings.Protocol, err)
	}

	return c.handshake(connectCtx, adapter, settings.Protocol, configuration)
}

func dial(
	establishCtx, connCtx context.Context,
	s settings.Settings,
) (io.ReadWriteCloser, error) {
	sources := addressSources(s.Server)
	lastErr := fmt.Errorf("server host is empty")
	for i, source := range sources {
		selected := s
		selected.Server = source
		endpoint := net.JoinHostPort(source.Domain, strconv.Itoa(s.Port))
		if source.Domain == "" {
			ipVersion := 4
			if source.IPv6 != "" {
				ipVersion = 6
			}
			ap, err := resolveAddrPort(establishCtx, selected, ipVersion)
			if err != nil {
				lastErr = err
				continue
			}
			endpoint = ap.String()
		}

		attemptCtx := establishCtx
		cancel := func() {}
		if i < len(sources)-1 {
			timeout := dialAttemptTimeout(s)
			if deadline, ok := establishCtx.Deadline(); ok {
				timeout = min(timeout, time.Until(deadline)/time.Duration(len(sources)-i))
			}
			attemptCtx, cancel = context.WithTimeout(establishCtx, timeout)
		}
		var (
			transport io.ReadWriteCloser
			err       error
		)
		switch s.Protocol {
		case settings.TCP:
			// DialContext handles both DNS families with Happy Eyeballs.
			transport, err = dialTCP(attemptCtx, endpoint)
		case settings.UDP:
			if source.Domain != "" {
				transport, err = dialUDPDomain(attemptCtx, selected)
			} else {
				transport, err = dialUDP(attemptCtx, endpoint)
			}
		case settings.WS, settings.WSS:
			scheme := "ws"
			if s.Protocol == settings.WSS {
				scheme = "wss"
			}
			transport, err = dialWebSocketEndpoint(attemptCtx, connCtx, scheme, endpoint, s.Server.Domain)
		default:
			err = fmt.Errorf("unsupported protocol: %v", s.Protocol)
		}
		cancel()
		if err == nil {
			return transport, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// addressSources orders configured sources without performing DNS queries.
// The domain is tried with both families before the explicit fallback addresses.
func addressSources(server settings.Host) []settings.Host {
	var sources []settings.Host
	if server.Domain != "" {
		sources = append(sources, settings.Host{Domain: server.Domain})
	}
	if server.IPv6 != "" {
		sources = append(sources, settings.Host{IPv6: server.IPv6})
	}
	if server.IPv4 != "" {
		sources = append(sources, settings.Host{IPv4: server.IPv4})
	}
	return sources
}

func (c *Client) handshake(
	ctx context.Context,
	adapter io.ReadWriteCloser,
	protocol settings.Protocol,
	configuration client.Configuration,
) (io.ReadWriteCloser, crypto, clientRekey, error) {
	// IK handshake requires client keys
	if len(configuration.ClientPublicKey) != 32 || len(configuration.ClientPrivateKey) != 32 {
		_ = adapter.Close()
		return nil, nil, nil, fmt.Errorf("client keys not configured (required for IK handshake)")
	}
	if len(configuration.X25519PublicKey) != 32 {
		_ = adapter.Close()
		return nil, nil, nil, fmt.Errorf("server public key not configured (required for IK handshake)")
	}

	handshake := noise.NewIKHandshakeClient(
		configuration.ClientPublicKey,
		configuration.ClientPrivateKey,
		configuration.X25519PublicKey,
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

	var (
		cr              crypto
		epochController epochController
		err             error
	)
	switch protocol {
	case settings.UDP:
		cr, epochController, err = udp.NewFromHandshake(handshake, false)
	case settings.TCP, settings.WS, settings.WSS:
		cr, epochController, err = tcp.NewFromHandshake(handshake, false)
	default:
		err = fmt.Errorf("unsupported protocol: %v", protocol)
	}
	cancelCloseOnContextDone()
	if ctxErr := ctx.Err(); ctxErr != nil {
		closeAdapter()
		return nil, nil, nil, ctxErr
	}
	if err != nil {
		closeAdapter()
		return nil, nil, nil, fmt.Errorf("failed to create client crypto: %w", err)
	}
	var rehandshake rekeyV2Handshake
	if handshake.Supports(noise.CapabilityRekeyV2) {
		rehandshake = handshake
	}
	coordinator := rekey.NewClientRekeyCoordinator(
		&keys.DefaultKeyDeriver{},
		epochController,
		rehandshake,
		settings.DefaultRekeyInterval,
		time.Now().UTC(),
	)
	return adapter, cr, coordinator, nil
}

func dialTCP(
	ctx context.Context,
	endpoint string,
) (io.ReadWriteCloser, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", endpoint)
	if err != nil {
		return nil, err
	}

	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetNoDelay(true)
		_ = tcp.SetKeepAlive(true)
		_ = tcp.SetKeepAlivePeriod(30 * time.Second)
	}

	transport := tcptransport.WithReadDeadline(conn, settings.PingRestartTimeout)
	if remote := parseNetAddrPort(conn.RemoteAddr()); remote.IsValid() {
		transport = tcptransport.WithRemoteAddr(transport, remote)
	}

	return tcptransport.NewFramedConn(
		transport,
		settings.DefaultEthernetMTU+settings.TCPChacha20Overhead,
	)
}

func dialUDP(
	ctx context.Context,
	endpoint string,
) (io.ReadWriteCloser, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", endpoint)
	if err != nil {
		return nil, err
	}
	if remote := parseNetAddrPort(conn.RemoteAddr()); remote.IsValid() {
		return tcptransport.WithRemoteAddr(conn, remote), nil
	}
	return conn, nil
}

const minimumDialAttemptTimeout = 2 * time.Second

func dialUDPDomain(ctx context.Context, s settings.Settings) (io.ReadWriteCloser, error) {
	var lastErr error
	for _, ipVersion := range []int{6, 4} {
		ap, err := resolveAddrPort(ctx, s, ipVersion)
		if err != nil {
			lastErr = err
			continue
		}
		// UDP dial does not check server reachability. Only a local error
		// allows another address to be tried before the protocol handshake.
		transport, err := dialUDP(ctx, ap.String())
		if err == nil {
			return transport, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func dialWebSocketEndpoint(
	establishCtx, connCtx context.Context,
	scheme, endpoint, serverName string,
) (io.ReadWriteCloser, error) {
	webSocketURL := fmt.Sprintf("%s://%s/ws", scheme, endpoint)
	identityEndpoint := endpoint
	if serverName != "" {
		if _, err := netip.ParseAddr(serverName); err != nil {
			asciiName, err := idna.Lookup.ToASCII(serverName)
			if err != nil {
				return nil, fmt.Errorf("invalid server hostname: %w", err)
			}
			serverName = asciiName
		}
		_, port, err := net.SplitHostPort(endpoint)
		if err != nil {
			return nil, err
		}
		identityEndpoint = net.JoinHostPort(serverName, port)
	}
	var (
		remoteMu   sync.Mutex
		remoteAddr net.Addr
	)
	dialer := &net.Dialer{}
	transport := cloneDefaultTransport()
	defer transport.CloseIdleConnections()
	if proxyForRequest := transport.Proxy; proxyForRequest != nil && identityEndpoint != endpoint {
		// Proxy selection (including NO_PROXY) follows the configured hostname.
		transport.Proxy = func(request *http.Request) (*url.URL, error) {
			if request.URL.Host != endpoint {
				return proxyForRequest(request)
			}
			logicalRequest := request.Clone(request.Context())
			logicalRequest.URL.Host = identityEndpoint
			logicalRequest.Host = identityEndpoint
			return proxyForRequest(logicalRequest)
		}
	}
	transport.DialContext = func(ctx context.Context, network, target string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, target)
		if err != nil {
			return nil, err
		}
		remoteMu.Lock()
		remoteAddr = conn.RemoteAddr()
		remoteMu.Unlock()
		return conn, nil
	}
	if scheme == "ws" && identityEndpoint != endpoint && transport.Proxy != nil {
		// A forward proxy would replace the fallback IP with the domain in Host.
		// CONNECT keeps the selected address separate from the HTTP identity.
		proxyForRequest := transport.Proxy
		transport.Proxy = nil
		var roots *x509.CertPool
		if transport.TLSClientConfig != nil {
			roots = transport.TLSClientConfig.RootCAs
		}
		transport.DialContext = func(ctx context.Context, network, target string) (net.Conn, error) {
			request := (&http.Request{URL: &url.URL{Scheme: "http", Host: target, Path: "/ws"}}).WithContext(ctx)
			proxyURL, err := proxyForRequest(request)
			if err != nil {
				return nil, err
			}
			conn, err := dialWebSocketTunnel(ctx, network, target, proxyURL, roots)
			if err != nil {
				return nil, err
			}
			remoteMu.Lock()
			remoteAddr = conn.RemoteAddr()
			remoteMu.Unlock()
			return conn, nil
		}
	}
	if scheme == "wss" {
		// Own proxy setup so net/http cannot replace destination uTLS after CONNECT.
		proxyForRequest := transport.Proxy
		var roots *x509.CertPool
		if transport.TLSClientConfig != nil {
			roots = transport.TLSClientConfig.RootCAs
		}
		transport.ForceAttemptHTTP2 = false
		transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		// HTTP redirects still need net/http's forward-proxy handling. Its TLS
		// dialer connects only to an HTTPS proxy, without another proxy lookup.
		httpTransport := transport.Clone()
		defer httpTransport.CloseIdleConnections()
		httpTransport.DialTLSContext = func(ctx context.Context, network, target string) (net.Conn, error) {
			conn, err := dialTLSForWebSocket(ctx, network, target, nil, roots, "")
			if err != nil {
				return nil, err
			}
			remoteMu.Lock()
			remoteAddr = conn.RemoteAddr()
			remoteMu.Unlock()
			return conn, nil
		}
		transport.RegisterProtocol("http", httpTransport)
		transport.Proxy = nil
		transport.DialTLSContext = func(ctx context.Context, network, target string) (net.Conn, error) {
			var proxyURL *url.URL
			if proxyForRequest != nil {
				request := (&http.Request{URL: &url.URL{Scheme: "https", Host: target, Path: "/ws"}}).WithContext(ctx)
				var err error
				proxyURL, err = proxyForRequest(request)
				if err != nil {
					return nil, err
				}
			}
			tlsServerName := ""
			if target == endpoint {
				tlsServerName = serverName
			}
			conn, err := dialTLSForWebSocket(ctx, network, target, proxyURL, roots, tlsServerName)
			if err != nil {
				return nil, err
			}
			remoteMu.Lock()
			remoteAddr = conn.RemoteAddr()
			remoteMu.Unlock()
			return conn, nil
		}
	}
	opts := &websocket.DialOptions{HTTPClient: &http.Client{Transport: transport}}
	if identityEndpoint != endpoint {
		opts.Host = identityEndpoint
	}
	conn, resp, err := websocket.Dial(establishCtx, webSocketURL, opts)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	wrapped, wrapErr := tcptransport.NewFramedConn(
		tcptransport.WithReadDeadline(ws.NewConn(connCtx, conn, nil, nil), settings.PingRestartTimeout),
		settings.DefaultEthernetMTU+settings.TCPChacha20Overhead,
	)
	if wrapErr != nil {
		_ = conn.Close(websocket.StatusInternalError, "adapter wrap failed")
		return nil, wrapErr
	}
	remoteMu.Lock()
	remote := parseNetAddrPort(remoteAddr)
	remoteMu.Unlock()
	if remote.IsValid() {
		return tcptransport.WithRemoteAddr(wrapped, remote), nil
	}
	if remote := parseEndpointAddrPort(endpoint); remote.IsValid() {
		return tcptransport.WithRemoteAddr(wrapped, remote), nil
	}
	return wrapped, nil
}

func dialTLSForWebSocket(ctx context.Context, network, target string, proxyURL *url.URL, roots *x509.CertPool, serverName string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	if serverName != "" {
		host = serverName
	}
	raw, err := dialWebSocketTunnel(ctx, network, target, proxyURL, roots)
	if err != nil {
		return nil, err
	}
	return handshakeWebSocketTLS(ctx, raw, host, roots)
}

// dialWebSocketTunnel returns a stream to the destination, before its TLS handshake.
func dialWebSocketTunnel(ctx context.Context, network, target string, proxyURL *url.URL, roots *x509.CertPool) (net.Conn, error) {
	dialer := &net.Dialer{}
	if proxyURL == nil {
		return dialer.DialContext(ctx, network, target)
	}
	host := proxyURL.Hostname()
	// Match net/http's IDNA lookup policy, retaining ASCII names and IP literals.
	for _, r := range host {
		if r >= utf8.RuneSelf {
			var err error
			host, err = idna.Lookup.ToASCII(host)
			if err != nil {
				return nil, fmt.Errorf("invalid proxy hostname: %w", err)
			}
			break
		}
	}
	var defaultPort string
	switch proxyURL.Scheme {
	case "socks5", "socks5h":
		socksURL := *proxyURL
		socksURL.Host = net.JoinHostPort(host, proxyURL.Port())
		socks, err := proxy.FromURL(&socksURL, dialer)
		if err != nil {
			return nil, err
		}
		return socks.(proxy.ContextDialer).DialContext(ctx, network, target)
	case "", "http":
		defaultPort = "80"
	case "https":
		defaultPort = "443"
	default:
		return nil, fmt.Errorf("unsupported WebSocket proxy scheme: %q", proxyURL.Scheme)
	}
	port := proxyURL.Port()
	if port == "" {
		port = defaultPort
	}
	conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	if proxyURL.Scheme == "https" {
		conn, err = handshakeWebSocketTLS(ctx, conn, host, roots)
		if err != nil {
			return nil, fmt.Errorf("TLS to HTTPS proxy: %w", err)
		}
	}
	if err := connectWebSocketProxy(ctx, conn, target, proxyURL); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

func connectWebSocketProxy(ctx context.Context, conn net.Conn, target string, proxyURL *url.URL) (err error) {
	// Request.Write and ReadResponse do not observe a request's context. Interrupt
	// their I/O on cancellation, and join the callback before handing off the conn.
	cancelled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
		close(cancelled)
	})
	defer func() {
		if !stop() {
			<-cancelled
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}()
	request := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: target},
		Host:   target,
		Header: make(http.Header),
	}
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		auth := proxyURL.User.Username() + ":" + password
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth)))
	}
	if err := request.Write(conn); err != nil {
		return fmt.Errorf("write proxy CONNECT: %w", err)
	}
	// As in net/http, the reader can be discarded: the TLS server cannot send
	// tunneled data until we send ClientHello. Bound the proxy response headers.
	response, err := http.ReadResponse(bufio.NewReader(io.LimitReader(conn, 10<<20)), request)
	if err != nil {
		return fmt.Errorf("read proxy CONNECT: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("proxy CONNECT: %s", response.Status)
	}
	return nil
}

// handshakeWebSocketTLS takes ownership of raw and closes it on failure.
func handshakeWebSocketTLS(ctx context.Context, raw net.Conn, host string, roots *x509.CertPool) (net.Conn, error) {
	conn := utls.UClient(raw, &utls.Config{
		ServerName: host,
		RootCAs:    roots,
	}, utls.HelloChrome_Auto)

	if err := conn.BuildHandshakeState(); err != nil {
		_ = raw.Close()
		return nil, err
	}

	// WebSocket Upgrade and proxy CONNECT use HTTP/1.1, with no ALPS settings.
	extensions := conn.Extensions[:0]
	for _, ext := range conn.Extensions {
		switch ext := ext.(type) {
		case *utls.ALPNExtension:
			ext.AlpnProtocols = []string{"http/1.1"}
		case *utls.ApplicationSettingsExtension, *utls.ApplicationSettingsExtensionNew:
			continue
		}
		extensions = append(extensions, ext)
	}
	conn.Extensions = extensions

	if err := conn.HandshakeContext(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}

	if protocol := conn.ConnectionState().NegotiatedProtocol; protocol != "" && protocol != "http/1.1" {
		_ = conn.Close()
		return nil, fmt.Errorf("unexpected negotiated ALPN: %q", protocol)
	}

	return conn, nil
}

func dialAttemptTimeout(s settings.Settings) time.Duration {
	timeout := time.Duration(s.DialTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	attempt := timeout / 2
	if attempt < minimumDialAttemptTimeout {
		return minimumDialAttemptTimeout
	}
	return attempt
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

func resolveAddrPort(ctx context.Context, s settings.Settings, ipVersion int) (netip.AddrPort, error) {
	var (
		raw string
		err error
	)
	switch ipVersion {
	case 4:
		raw, err = host.ResolveIPv4(ctx, s.Server)
	case 6:
		raw, err = host.ResolveIPv6(ctx, s.Server)
	default:
		return netip.AddrPort{}, fmt.Errorf("unsupported IP version: %d", ipVersion)
	}
	if err != nil {
		return netip.AddrPort{}, err
	}

	ip, parseErr := netip.ParseAddr(raw)
	if parseErr != nil {
		return netip.AddrPort{}, parseErr
	}
	isIPv4 := ip.Unmap().Is4()
	if ipVersion == 6 && isIPv4 {
		return netip.AddrPort{}, fmt.Errorf("resolved IPv4 %q, expected IPv6", raw)
	}
	if ipVersion == 4 && !isIPv4 {
		return netip.AddrPort{}, fmt.Errorf("resolved IPv6 %q, expected IPv4", raw)
	}
	if isIPv4 {
		ip = ip.Unmap()
	}
	return netip.ParseAddrPort(net.JoinHostPort(ip.String(), strconv.Itoa(s.Port)))
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
