package client

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"time"
	"tungo/internal/config/settings"
	"tungo/internal/protocol/chacha20/tcp"
	"tungo/internal/protocol/chacha20/udp"
	"tungo/internal/protocol/keys"
	"tungo/internal/protocol/noise"
	"tungo/internal/protocol/rekey"
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
) (net.Conn, crypto, clientRekey, error) {
	active, err := c.configurations.Active()
	if err != nil {
		return nil, nil, nil, err
	}
	var errs []error
	switch active.Protocol {
	case settings.TCP:
		hosts, err := c.resolveServerAddresses(ctx, active.TCPSettings)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, host := range hosts {
			deadline := time.Now().Add(time.Duration(active.TCPSettings.DialTimeout))
			connectCtx, cancel := context.WithDeadline(ctx, deadline)
			defer cancel()
			endpoint := net.JoinHostPort(host, strconv.Itoa(active.TCPSettings.Port))
			conn, err := dialTCP(connectCtx, endpoint)
			if err != nil {
				errs = append(errs, fmt.Errorf("unable to establish a TCP connection: %w", err))
				continue
			}
			rwc, crypto, rekey, err := c.handshake(connectCtx, conn, active.Protocol)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			return rwc, crypto, rekey, nil
		}
		return nil, nil, nil, errors.Join(errs...)
	case settings.UDP:
		hosts, err := c.resolveServerAddresses(ctx, active.UDPSettings)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, host := range hosts {
			deadline := time.Now().Add(time.Duration(active.UDPSettings.DialTimeout))
			connectCtx, cancel := context.WithDeadline(ctx, deadline)
			defer cancel()
			endpoint := net.JoinHostPort(host, strconv.Itoa(active.UDPSettings.Port))
			conn, err := dialUDP(connectCtx, endpoint)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			rwc, crypto, rekey, err := c.handshake(connectCtx, conn, active.Protocol)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			return rwc, crypto, rekey, nil

		}
	case settings.WS, settings.WSS:
		scheme := "ws"
		if active.Protocol == settings.WSS {
			scheme = "wss"
		}
		var hosts []string
		for _, host := range []string{
			active.WSSettings.Server.Domain,
			active.WSSettings.Server.IPv4,
			active.WSSettings.Server.IPv6,
		} {
			if len(host) > 0 {
				hosts = append(hosts, host)
			}
		}
		for _, host := range hosts {
			deadline := time.Now().Add(time.Duration(active.WSSettings.DialTimeout))
			connectCtx, cancel := context.WithDeadline(ctx, deadline)
			defer cancel()
			endpoint := net.JoinHostPort(host, strconv.Itoa(active.WSSettings.Port))
			conn, err := dialWS(connectCtx, ctx, scheme, endpoint, active.WSSettings.Server.Domain)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			rwc, crypto, rekey, err := c.handshake(connectCtx, conn, active.Protocol)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			return rwc, crypto, rekey, nil
		}

	default:
		return nil, nil, nil, fmt.Errorf("unsupported protocol: %v", active.Protocol)
	}
	return nil, nil, nil, errors.Join(errs...)
}

func (c *Client) resolveServerAddresses(ctx context.Context, s settings.Settings) ([]string, error) {
	var hosts []string
	if len(s.Server.Domain) > 0 {
		v4s, err4 := c.lookupDomainIP(ctx, s, 4)
		if err4 != nil {
			slog.Warn("domain lookup", "err", err4)
		}
		v6s, err6 := c.lookupDomainIP(ctx, s, 6)
		if err6 != nil {
			slog.Warn("domain lookup", "err", err6)
		}
		for _, addr := range append(v4s, v6s...) {
			hosts = append(hosts, addr.String())
		}
	}
	if len(s.Server.IPv4) > 0 {
		hosts = append(hosts, s.Server.IPv4)
	}
	if len(s.Server.IPv6) > 0 {
		hosts = append(hosts, s.Server.IPv6)
	}
	if err := ctx.Err(); err != nil {
		return hosts, err
	}
	if len(hosts) == 0 {
		return hosts, fmt.Errorf("could not resolve hosts list from configuration's Server subsection: %v", s.Server)
	}
	return hosts, nil
}

func (c *Client) lookupDomainIP(ctx context.Context, s settings.Settings, ipFamily int) ([]netip.Addr, error) {
	deadline := time.Now().Add(time.Duration(s.DialTimeout))
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, fmt.Sprintf("ip%d", ipFamily), s.Server.Domain)
	if err != nil && ctx.Err() == nil {
		return nil, fmt.Errorf("failed to resolve domain to ip%d %q: %w", ipFamily, s.Server.Domain, err)
	}
	return ips, ctx.Err()
}

func (c *Client) handshake(
	ctx context.Context,
	conn net.Conn,
	protocol settings.Protocol,
) (net.Conn, crypto, clientRekey, error) {
	configuration, err := c.configurations.Active()
	if err != nil {
		_ = conn.Close()
		return nil, nil, nil, err
	}
	handshake := noise.NewIKHandshakeClient(
		configuration.ClientPublicKey,
		configuration.ClientPrivateKey,
		configuration.X25519PublicKey,
	)

	var closeOnce sync.Once
	closeAdapter := func() {
		closeOnce.Do(func() { _ = conn.Close() })
	}
	cancelCloseOnContextDone := context.AfterFunc(ctx, closeAdapter)

	if err = handshake.ClientSideHandshake(conn); err != nil {
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
	return conn, cr, coordinator, nil
}

func dialTCP(
	ctx context.Context,
	endpoint string,
) (net.Conn, error) {
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

	conn = tcptransport.WithReadDeadline(conn, settings.PingRestartTimeout)
	return tcptransport.NewFramedConn(
		conn,
		settings.DefaultEthernetMTU+settings.TCPChacha20Overhead,
	)
}

func dialUDP(
	ctx context.Context,
	endpoint string,
) (net.Conn, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", endpoint)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func dialWS(
	establishCtx, connCtx context.Context,
	scheme, endpoint, serverName string,
) (net.Conn, error) {
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
		tcptransport.WithReadDeadline(ws.NewConn(connCtx, conn, nil, remoteAddr), settings.PingRestartTimeout),
		settings.DefaultEthernetMTU+settings.TCPChacha20Overhead,
	)
	if wrapErr != nil {
		_ = conn.Close(websocket.StatusInternalError, "adapter wrap failed")
		return nil, wrapErr
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
