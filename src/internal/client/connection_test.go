package client

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/net/dns/dnsmessage"

	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/protocol/keys"
	"tungo/internal/protocol/noise"
	"tungo/internal/protocol/servicepacket"
	transport "tungo/internal/transport/tcp"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

type testPeer struct {
	PublicKey []byte
	Enabled   bool
	ClientID  int
}

type testPeers map[string]testPeer

func newTestAllowedPeers(peers []testPeer) testPeers {
	lookup := make(testPeers, len(peers))
	for _, peer := range peers {
		lookup[string(peer.PublicKey)] = peer
	}
	return lookup
}

func (p testPeers) Lookup(publicKey []byte) (int, bool, bool) {
	peer, found := p[string(publicKey)]
	return peer.ClientID, peer.Enabled, found
}

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func mustHost(raw string) settings.Host {
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return settings.Host{Domain: raw}
	}
	if ip.Unmap().Is4() {
		return settings.Host{IPv4: ip.Unmap().String()}
	}
	return settings.Host{IPv6: ip.String()}
}

// mkTCPSettings returns minimal TCP settings for a given port.
func mkTCPSettings(port int) settings.Settings {
	return settings.Settings{
		Network: settings.Network{
			Server: mustHost("127.0.0.1"),
			Port:   port,
		},
		Protocol:      settings.TCP,
		DialTimeoutMs: 100,
	}
}

// mkUDPSettings returns minimal UDP settings for a given port.
func mkUDPSettings(port int) settings.Settings {
	return settings.Settings{
		Network: settings.Network{
			Server: mustHost("127.0.0.1"),
			Port:   port,
		},
		Protocol:      settings.UDP,
		DialTimeoutMs: 100,
	}
}

// mkWSSettings returns minimal WS/WSS settings.
func mkWSSettings(host string, port int, proto settings.Protocol) settings.Settings {
	return settings.Settings{
		Network: settings.Network{
			Server: mustHost(host),
			Port:   port,
		},
		Protocol:      proto,
		DialTimeoutMs: 200,
	}
}

func testConfigurationWithSettings(selected settings.Settings) clientconfig.Configuration {
	conf := clientconfig.Configuration{ClientID: 1, Protocol: selected.Protocol}
	switch selected.Protocol {
	case settings.UDP:
		conf.UDPSettings = selected
	case settings.TCP:
		conf.TCPSettings = selected
	case settings.WS, settings.WSS:
		conf.WSSettings = selected
	}
	return conf
}

// ConnectionFactoryMockWSServer spins up a barebones WS echo server at /ws.
func ConnectionFactoryMockWSServer(t *testing.T) (host string, port string, shutdown func()) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	h, p, _ := strings.Cut(addr, ":")

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func(c *websocket.Conn, code websocket.StatusCode, reason string) {
			_ = c.Close(code, reason)
		}(c, websocket.StatusNormalClosure, "")
		// simple echo loop
		for {
			typ, data, err := c.Read(r.Context())
			if err != nil {
				return
			}
			_ = c.Write(r.Context(), typ, data)
		}
	})
	srv := &http.Server{Handler: mux}

	go func() {
		_ = srv.Serve(ln)
	}()

	return h, p, func() {
		_ = srv.Shutdown(context.Background())
		_ = ln.Close()
	}
}

// ---- tests ----

func TestEstablishConnection_InvalidPort_TCP_ParseError(t *testing.T) {
	t.Parallel()
	// Out-of-range port should fail during addr:port parsing.
	client := &Client{}
	configuration := testConfigurationWithSettings(mkTCPSettings(70000))

	_, _, _, err := client.connect(context.Background(), configuration)
	if err == nil {
		t.Fatalf("expected parse error for bad port")
	}
}

func TestEstablishConnection_InvalidPort_UDP_ParseError(t *testing.T) {
	t.Parallel()
	client := &Client{}
	configuration := testConfigurationWithSettings(mkUDPSettings(70000))

	_, _, _, err := client.connect(context.Background(), configuration)
	if err == nil {
		t.Fatalf("expected parse error for bad UDP port")
	}
}

func TestDialTCP_Success(t *testing.T) {
	t.Parallel()
	// Start a temporary TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen tcp: %v", err)
	}
	defer func(ln net.Listener) { _ = ln.Close() }(ln)

	// Accept one connection in the background
	done := make(chan struct{})
	go func() {
		defer close(done)
		if conn, err := ln.Accept(); err == nil {
			_ = conn.Close()
		}
	}()

	ap := netip.MustParseAddrPort(ln.Addr().String())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	adapter, err := dialTCP(ctx, ap.String())
	if err != nil {
		t.Fatalf("dialTCP failed: %v", err)
	}
	if adapter == nil {
		t.Fatalf("adapter must not be nil on success")
	}
	if remote, ok := adapter.(interface{ RemoteAddrPort() netip.AddrPort }); !ok || !remote.RemoteAddrPort().IsValid() {
		t.Fatalf("expected transport with valid remote address, got %T", adapter)
	}
	_ = adapter.Close()
	<-done
}

func TestDialTCP_Refused(t *testing.T) {
	t.Parallel()
	// Port 1 on localhost is almost always closed -> should return an error quickly.
	ap := netip.MustParseAddrPort("127.0.0.1:1")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	adapter, err := dialTCP(ctx, ap.String())
	if err == nil {
		_ = adapter.Close()
		t.Fatalf("expected error dialing to closed port")
	}
}

func TestDialUDP_Success_NoServerNeeded(t *testing.T) {
	t.Parallel()
	// UDP "dial" does not require a server to succeed in most cases.
	ap := netip.MustParseAddrPort("127.0.0.1:9") // discard port
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	conn, err := dialUDP(ctx, ap.String())
	if err != nil {
		t.Fatalf("dialUDP failed: %v", err)
	}
	if conn == nil {
		t.Fatalf("conn must not be nil")
	}
	if remote, ok := conn.(interface{ RemoteAddrPort() netip.AddrPort }); !ok || !remote.RemoteAddrPort().IsValid() {
		t.Fatalf("expected UDP transport with valid remote address, got %T", conn)
	}
	_ = conn.Close()
}

func TestDialWebSocketEndpoint_Success(t *testing.T) {
	t.Parallel()
	host, port, shutdown := ConnectionFactoryMockWSServer(t)
	defer shutdown()

	adapter, err := dialWebSocketEndpoint(context.Background(), context.Background(), "ws", net.JoinHostPort(host, port), "")
	if err != nil {
		t.Fatalf("dialWebSocketEndpoint failed: %v", err)
	}
	if adapter == nil {
		t.Fatalf("adapter must not be nil")
	}
	if remote, ok := adapter.(interface{ RemoteAddrPort() netip.AddrPort }); !ok || !remote.RemoteAddrPort().IsValid() {
		t.Fatalf("expected WS transport with valid remote address, got %T", adapter)
	}
	_ = adapter.Close()
}

func TestDialWebSocketEndpoint_Success_DomainEndpointHasRemoteAddr(t *testing.T) {
	t.Parallel()
	_, port, shutdown := ConnectionFactoryMockWSServer(t)
	defer shutdown()

	adapter, err := dialWebSocketEndpoint(context.Background(), context.Background(), "ws", net.JoinHostPort("localhost", port), "")
	if err != nil {
		t.Fatalf("dialWebSocketEndpoint failed: %v", err)
	}
	if adapter == nil {
		t.Fatalf("adapter must not be nil")
	}
	remote, ok := adapter.(interface{ RemoteAddrPort() netip.AddrPort })
	if !ok || !remote.RemoteAddrPort().IsValid() {
		t.Fatalf("expected WS transport with valid remote address for domain endpoint, got %T", adapter)
	}
	_ = adapter.Close()
}

func TestDialWebSocketEndpoint_Error_NoServer(t *testing.T) {
	t.Parallel()
	// Use a port with no WS server
	adapter, err := dialWebSocketEndpoint(context.Background(), context.Background(), "ws", net.JoinHostPort("127.0.0.1", "1"), "")
	if err == nil {
		_ = adapter.Close()
		t.Fatalf("expected error when no WS server is listening")
	}
}

func TestDialTLSForWebSocket_ClientHelloHTTP11(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	hellos := make(chan *tls.ClientHelloInfo, 1)
	serverDone := make(chan error, 1)
	go func() {
		raw, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverDone <- acceptErr
			return
		}
		defer func() { _ = raw.Close() }()
		server := tls.Server(raw, &tls.Config{
			GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
				hellos <- hello
				return nil, errors.New("stop after inspecting ClientHello")
			},
		})
		serverDone <- server.HandshakeContext(ctx)
	}()
	conn, err := dialTLSForWebSocket(ctx, "tcp", listener.Addr().String(), nil, nil, "")
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("expected the test server to reject the handshake")
	}
	select {
	case serverErr := <-serverDone:
		if serverErr == nil {
			t.Fatal("expected the server handshake to fail")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for the server handshake")
	}
	select {
	case hello := <-hellos:
		if !slices.Equal(hello.SupportedProtos, []string{"http/1.1"}) {
			t.Errorf("ALPN = %v, want only http/1.1", hello.SupportedProtos)
		}
		for _, extension := range hello.Extensions {
			if extension == 17513 || extension == 17613 {
				t.Errorf("HTTP/1.1 ClientHello advertises ALPS extension %d", extension)
			}
		}
		isGREASE := func(value uint16) bool {
			return value&0x0f0f == 0x0a0a && byte(value>>8) == byte(value)
		}
		if !slices.ContainsFunc(hello.CipherSuites, isGREASE) || !slices.ContainsFunc(hello.Extensions, isGREASE) {
			t.Error("ClientHello lost the browser preset's GREASE")
		}
	default:
		t.Fatal("server did not receive ClientHello")
	}
}

func TestEstablishConnection_WSS_DialError_IsWrapped(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	client := &Client{}
	configuration := testConfigurationWithSettings(mkWSSettings("127.0.0.1", port, settings.WSS))
	_, _, _, err = client.connect(t.Context(), configuration)
	if err == nil || !strings.Contains(err.Error(), "unable to establish WSS") {
		t.Fatalf("expected wrapped WS connect error, got: %v", err)
	}
}

func TestEstablishConnection_UnsupportedProtocol(t *testing.T) {
	t.Parallel()
	client := &Client{}
	configuration := testConfigurationWithSettings(settings.Settings{Protocol: 999})
	_, _, _, err := client.connect(context.Background(), configuration)
	if err == nil {
		t.Fatalf("expected error for unsupported protocol")
	}
}

// Verifies the error wrapping path for TCP.
func TestEstablishConnection_TCP_DialError_IsWrapped(t *testing.T) {
	t.Parallel()
	client := &Client{}
	configuration := testConfigurationWithSettings(mkTCPSettings(1)) // likely closed → Dial error

	_, _, _, err := client.connect(context.Background(), configuration)
	if err == nil {
		t.Fatalf("expected dial error")
	}
	if !strings.Contains(err.Error(), "unable to establish TCP connection") {
		t.Fatalf("expected wrapped TCP dial error, got: %v", err)
	}
}

// Verifies the error wrapping path for WS with provided Host fallback.
func TestEstablishConnection_WS_DialError_IsWrapped(t *testing.T) {
	t.Parallel()
	client := &Client{}
	configuration := testConfigurationWithSettings(mkWSSettings("127.0.0.1", 9, settings.WS))
	_, _, _, err := client.connect(context.Background(), configuration)
	if err == nil || !strings.Contains(err.Error(), "unable to establish WS") {
		t.Fatalf("expected wrapped WS dial error, got: %v", err)
	}
}

type cfUnitTransport struct {
	readErr      error
	readBuf      []byte
	writeErr     error
	closed       bool
	deadlineHits int
}

func (t *cfUnitTransport) Read(p []byte) (int, error) {
	if len(t.readBuf) > 0 {
		n := copy(p, t.readBuf)
		t.readBuf = t.readBuf[n:]
		return n, nil
	}
	if t.readErr != nil {
		return 0, t.readErr
	}
	return 0, io.EOF
}

func (t *cfUnitTransport) Write(p []byte) (int, error) {
	if t.writeErr != nil {
		return 0, t.writeErr
	}
	return len(p), nil
}

func (t *cfUnitTransport) Close() error {
	t.closed = true
	return nil
}

func (t *cfUnitTransport) SetReadDeadline(time.Time) error {
	t.deadlineHits++
	return nil
}

type cfUnitNoDeadlineTransport struct {
	readErr  error
	readBuf  []byte
	writeErr error
	closed   bool
}

type cfBlockingTransport struct {
	readStarted chan struct{}
	closed      chan struct{}
	readOnce    sync.Once
	closeOnce   sync.Once
}

func newCFBlockingTransport() *cfBlockingTransport {
	return &cfBlockingTransport{
		readStarted: make(chan struct{}),
		closed:      make(chan struct{}),
	}
}

func (t *cfBlockingTransport) Read([]byte) (int, error) {
	t.readOnce.Do(func() { close(t.readStarted) })
	<-t.closed
	return 0, net.ErrClosed
}

func (*cfBlockingTransport) Write(p []byte) (int, error) {
	return len(p), nil
}

func (t *cfBlockingTransport) Close() error {
	t.closeOnce.Do(func() { close(t.closed) })
	return nil
}

func (t *cfUnitNoDeadlineTransport) Read(p []byte) (int, error) {
	if len(t.readBuf) > 0 {
		n := copy(p, t.readBuf)
		t.readBuf = t.readBuf[n:]
		return n, nil
	}
	if t.readErr != nil {
		return 0, t.readErr
	}
	return 0, io.EOF
}

func (t *cfUnitNoDeadlineTransport) Write(p []byte) (int, error) {
	if t.writeErr != nil {
		return 0, t.writeErr
	}
	return len(p), nil
}

func (t *cfUnitNoDeadlineTransport) Close() error {
	t.closed = true
	return nil
}

func TestEstablishSecuredConnection_MissingClientKeys_ClosesAdapter(t *testing.T) {
	client := &Client{}
	configuration := clientconfig.Configuration{
		ClientPublicKey:  []byte{1, 2, 3}, // invalid length
		ClientPrivateKey: []byte{4, 5, 6}, // invalid length
	}
	tr := &cfUnitTransport{}

	_, _, _, err := client.handshake(
		context.Background(),
		tr,
		settings.TCP,
		configuration,
	)
	if err == nil || !strings.Contains(err.Error(), "client keys not configured") {
		t.Fatalf("expected client keys error, got %v", err)
	}
	if !tr.closed {
		t.Fatal("expected adapter to be closed on missing keys")
	}
}

func TestEstablishSecuredConnection_MissingServerPublicKey_ClosesAdapter(t *testing.T) {
	clientPub := make([]byte, 32)
	clientPriv := make([]byte, 32)

	client := &Client{}
	configuration := clientconfig.Configuration{
		ClientPublicKey:  clientPub,
		ClientPrivateKey: clientPriv,
		X25519PublicKey:  []byte{7, 8, 9}, // invalid length
	}
	tr := &cfUnitTransport{}

	_, _, _, err := client.handshake(
		context.Background(),
		tr,
		settings.TCP,
		configuration,
	)
	if err == nil || !strings.Contains(err.Error(), "server public key not configured") {
		t.Fatalf("expected server public key error, got %v", err)
	}
	if !tr.closed {
		t.Fatal("expected adapter to be closed on missing server public key")
	}
}

func TestEstablishSecuredConnection_HandshakeError_ClosesAdapter(t *testing.T) {
	clientPub := make([]byte, 32)
	clientPriv := make([]byte, 32)
	serverPub := make([]byte, 32)
	clientPub[0], clientPriv[0], serverPub[0] = 1, 2, 3

	client := &Client{}
	configuration := clientconfig.Configuration{
		ClientPublicKey:  clientPub,
		ClientPrivateKey: clientPriv,
		X25519PublicKey:  serverPub,
	}
	tr := &cfUnitTransport{readErr: io.ErrUnexpectedEOF}

	_, _, _, err := client.handshake(
		context.Background(),
		tr,
		settings.TCP,
		configuration,
	)
	if err == nil {
		t.Fatal("expected handshake error")
	}
	if !tr.closed {
		t.Fatal("expected adapter to be closed on handshake error")
	}
}

func TestEstablishSecuredConnection_CancelClosesAdapter(t *testing.T) {
	clientPub := make([]byte, 32)
	clientPriv := make([]byte, 32)
	serverPub := make([]byte, 32)
	clientPub[0], clientPriv[0], serverPub[0] = 1, 2, 3

	client := &Client{}
	configuration := clientconfig.Configuration{
		ClientPublicKey:  clientPub,
		ClientPrivateKey: clientPriv,
		X25519PublicKey:  serverPub,
	}
	transport := newCFBlockingTransport()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, _, _, err := client.handshake(ctx, transport, settings.TCP, configuration)
		errCh <- err
	}()

	<-transport.readStarted
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handshake did not stop after context cancellation")
	}

	select {
	case <-transport.closed:
	default:
		t.Fatal("expected adapter to be closed on context cancellation")
	}
}

func TestConnectionFactoryUnit_WithReadDeadline_NoDeadlineSupport_ReturnsSame(t *testing.T) {
	tr := &cfUnitNoDeadlineTransport{}
	wrapped := transport.WithReadDeadline(tr, time.Second)
	if wrapped != tr {
		t.Fatal("expected same transport when SetReadDeadline is not supported")
	}
}

func TestConnectionFactoryUnit_WithReadDeadline_WithDeadlineSupport_WrapsAndSetsDeadline(t *testing.T) {
	tr := &cfUnitTransport{readBuf: []byte("abc")}
	wrapped := transport.WithReadDeadline(tr, time.Second)

	if wrapped == tr {
		t.Fatal("expected wrapped transport")
	}
	buf := make([]byte, 3)
	n, err := wrapped.Read(buf)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if n != 3 || string(buf) != "abc" {
		t.Fatalf("unexpected read result n=%d buf=%q", n, string(buf))
	}
	if tr.deadlineHits == 0 {
		t.Fatal("expected SetReadDeadline to be called")
	}
}

func TestEstablishConnection_ErrorBranches(t *testing.T) {
	t.Run("unsupported protocol", func(t *testing.T) {
		client := &Client{}
		configuration := testConfigurationWithSettings(settings.Settings{Protocol: settings.UNKNOWN})
		_, _, _, err := client.connect(context.Background(), configuration)
		if err == nil {
			t.Fatal("expected unsupported protocol error")
		}
	})

	t.Run("tcp parse addr error", func(t *testing.T) {
		client := &Client{}
		configuration := testConfigurationWithSettings(mkTCPSettings(70000))
		_, _, _, err := client.connect(context.Background(), configuration)
		if err == nil {
			t.Fatal("expected parse error")
		}
	})

	t.Run("udp parse addr error", func(t *testing.T) {
		client := &Client{}
		configuration := testConfigurationWithSettings(mkUDPSettings(70000))
		_, _, _, err := client.connect(context.Background(), configuration)
		if err == nil {
			t.Fatal("expected parse error")
		}
	})

}

func TestDialErrorBranches(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	t.Run("dialTCP error", func(t *testing.T) {
		_, err := dialTCP(ctx, "127.0.0.1:1")
		if err == nil {
			t.Fatal("expected dialTCP error")
		}
	})

	t.Run("dialUDP error", func(t *testing.T) {
		conn, err := dialUDP(ctx, "127.0.0.1:9")
		// Environment-dependent: some sandboxes deny UDP connect, others allow it.
		if err == nil {
			if conn == nil {
				t.Fatal("expected non-nil conn when dialUDP succeeds")
			}
			_ = conn.Close()
		}
	})

	t.Run("dialWebSocketEndpoint error", func(t *testing.T) {
		_, err := dialWebSocketEndpoint(ctx, context.Background(), "ws", net.JoinHostPort("127.0.0.1", "1"), "")
		if err == nil {
			t.Fatal("expected dialWebSocketEndpoint error")
		}
	})
}

func TestEstablishSecuredConnection_Success(t *testing.T) {
	deriver := &keys.DefaultKeyDeriver{}

	serverPub, serverPrivArr, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("server keygen failed: %v", err)
	}
	clientPub, clientPrivArr, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("client keygen failed: %v", err)
	}

	client := &Client{}
	configuration := clientconfig.Configuration{
		ClientPublicKey:  clientPub,
		ClientPrivateKey: clientPrivArr[:],
		X25519PublicKey:  serverPub,
	}

	clientConn, serverConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()
	defer func() { _ = serverConn.Close() }()

	clientAdapter, err := transport.NewFramedConn(clientConn, 2048)
	if err != nil {
		t.Fatalf("client framing adapter failed: %v", err)
	}
	serverAdapter, err := transport.NewFramedConn(serverConn, 2048)
	if err != nil {
		t.Fatalf("server framing adapter failed: %v", err)
	}

	cookieManager, err := noise.NewCookieManager()
	if err != nil {
		t.Fatalf("cookie manager failed: %v", err)
	}
	allowedPeers := []testPeer{
		{
			PublicKey: clientPub,
			Enabled:   true,
			ClientID:  1,
		},
	}
	serverHS := noise.NewIKHandshakeServer(
		serverPub,
		serverPrivArr[:],
		newTestAllowedPeers(allowedPeers),
		cookieManager,
		noise.NewLoadMonitor(10000),
	)

	serverErrCh := make(chan error, 1)
	go func() {
		_, serr := serverHS.ServerSideHandshake(serverAdapter)
		serverErrCh <- serr
	}()

	adapter, crypto, coordinator, err := client.handshake(
		context.Background(),
		clientAdapter,
		settings.TCP,
		configuration,
	)
	if err != nil {
		t.Fatalf("establishSecuredConnection failed: %v", err)
	}
	if adapter == nil || crypto == nil || coordinator == nil {
		t.Fatal("expected non-nil adapter, crypto, and rekey coordinator")
	}
	if serr := <-serverErrCh; serr != nil {
		t.Fatalf("server handshake failed: %v", serr)
	}
	init, ok, err := coordinator.MaybeBuildRekeyInit(
		time.Now().Add(settings.DefaultRekeyInterval+time.Second),
		make([]byte, settings.DefaultEthernetMTU),
	)
	if err != nil || !ok {
		t.Fatalf("build negotiated rekey init: ok=%v err=%v", ok, err)
	}
	if kind, parsed := servicepacket.Parse(init); !parsed || kind != servicepacket.RekeyInitV2 {
		t.Fatalf("unexpected negotiated rekey type: kind=%v parsed=%v", kind, parsed)
	}
}

func TestResolveAddrPort(t *testing.T) {
	t.Parallel()

	s := mkTCPSettings(443)
	s.Server = settings.Host{IPv6: "2001:db8::1"}
	ap, err := resolveAddrPort(t.Context(), s, 6)
	if err != nil {
		t.Fatalf("resolveAddrPort: %v", err)
	}
	if got, want := ap.String(), "[2001:db8::1]:443"; got != want {
		t.Fatalf("resolveAddrPort = %q, want %q", got, want)
	}
	if _, err := resolveAddrPort(t.Context(), s, 5); err == nil {
		t.Fatal("expected unsupported IP version error")
	}
	s.Server.IPv6 = "invalid-ipv6"
	if _, err := resolveAddrPort(t.Context(), s, 6); err == nil {
		t.Fatal("expected invalid IP error")
	}
	s.Server.IPv6 = "2001:db8::1"
	for _, port := range []int{-1, 65536, 70000} {
		s.Port = port
		if ap, err := resolveAddrPort(t.Context(), s, 6); err == nil {
			t.Fatalf("out-of-range port %d became %v without an error", port, ap)
		}
	}
}

func TestDialAttemptTimeout_FromDialTimeout(t *testing.T) {
	t.Parallel()

	if got := dialAttemptTimeout(settings.Settings{}); got != 2500*time.Millisecond {
		t.Fatalf("unexpected default attempt timeout: got %v want %v", got, 2500*time.Millisecond)
	}
	if got := dialAttemptTimeout(settings.Settings{DialTimeoutMs: 1000}); got != 2*time.Second {
		t.Fatalf("unexpected clamped attempt timeout for short dial timeout: got %v", got)
	}
	if got := dialAttemptTimeout(settings.Settings{DialTimeoutMs: 12000}); got != 6*time.Second {
		t.Fatalf("unexpected attempt timeout: got %v want 6s", got)
	}
}

func TestDialUsesIPv6BeforeIPv4(t *testing.T) {
	t.Parallel()

	ipv6Listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer func() { _ = ipv6Listener.Close() }()
	port := ipv6Listener.Addr().(*net.TCPAddr).Port
	ipv4Listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("listen on IPv4: %v", err)
	}
	defer func() { _ = ipv4Listener.Close() }()

	for _, tc := range []struct {
		name     string
		protocol settings.Protocol
		server   settings.Host
		wantIP   string
		wantErr  bool
	}{
		{
			name:     "TCP probes IPv6 first",
			protocol: settings.TCP,
			server:   settings.Host{IPv4: "127.0.0.1", IPv6: "::1"},
			wantIP:   "::1",
		},
		{
			name:     "UDP starts with IPv6",
			protocol: settings.UDP,
			server:   settings.Host{IPv4: "127.0.0.1", IPv6: "::1"},
			wantIP:   "::1",
		},
		{
			name:     "UDP uses IPv6 when IPv4 is invalid",
			protocol: settings.UDP,
			server:   settings.Host{IPv4: "invalid-ipv4", IPv6: "::1"},
			wantIP:   "::1",
		},
		{
			name:     "UDP uses IPv4 when IPv6 is invalid",
			protocol: settings.UDP,
			server:   settings.Host{IPv4: "127.0.0.1", IPv6: "invalid-ipv6"},
			wantIP:   "127.0.0.1",
		},
		{
			name:     "UDP uses the only IPv4 address",
			protocol: settings.UDP,
			server:   settings.Host{IPv4: "127.0.0.1"},
			wantIP:   "127.0.0.1",
		},
		{
			name:     "UDP uses the only IPv6 address",
			protocol: settings.UDP,
			server:   settings.Host{IPv6: "::1"},
			wantIP:   "::1",
		},
		{
			name:     "UDP rejects IPv4 in the IPv6 field",
			protocol: settings.UDP,
			server:   settings.Host{IPv4: "invalid-ipv4", IPv6: "127.0.0.1"},
			wantErr:  true,
		},
		{
			name:     "UDP rejects IPv6 in the IPv4 field",
			protocol: settings.UDP,
			server:   settings.Host{IPv4: "::1", IPv6: "invalid-ipv6"},
			wantErr:  true,
		},
		{
			name:     "TCP uses IPv6 when IPv4 is invalid",
			protocol: settings.TCP,
			server:   settings.Host{IPv4: "invalid-ipv4", IPv6: "::1"},
			wantIP:   "::1",
		},
		{
			name:     "TCP uses IPv4 when IPv6 is invalid",
			protocol: settings.TCP,
			server:   settings.Host{IPv4: "127.0.0.1", IPv6: "invalid-ipv6"},
			wantIP:   "127.0.0.1",
		},
		{
			name:     "TCP uses the only IPv6 address",
			protocol: settings.TCP,
			server:   settings.Host{IPv6: "::1"},
			wantIP:   "::1",
		},
		{
			name:     "TCP rejects IPv4 in the IPv6 field",
			protocol: settings.TCP,
			server:   settings.Host{IPv4: "invalid-ipv4", IPv6: "127.0.0.1"},
			wantErr:  true,
		},
		{
			name:     "TCP rejects IPv6 in the IPv4 field",
			protocol: settings.TCP,
			server:   settings.Host{IPv4: "::1", IPv6: "invalid-ipv6"},
			wantErr:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := settings.Settings{
				Network: settings.Network{
					Server: tc.server,
					Port:   port,
				},
				Protocol: tc.protocol,
			}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			conn, err := dial(ctx, ctx, s)
			if tc.wantErr {
				if conn != nil {
					_ = conn.Close()
					t.Fatal("dial unexpectedly returned a connection")
				}
				if err == nil {
					t.Fatal("expected address family error")
				}
				return
			}
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer func() { _ = conn.Close() }()
			if got, err := transportServerAddr(conn); err != nil || got != netip.MustParseAddr(tc.wantIP) {
				t.Fatalf("remote address = %v, %v; want %s", got, err, tc.wantIP)
			}
		})
	}
}

func TestDialTCPUsesIPv4AfterIPv6Refusal(t *testing.T) {
	t.Parallel()

	ipv6Listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer func() { _ = ipv6Listener.Close() }()
	port := ipv6Listener.Addr().(*net.TCPAddr).Port
	ipv4Listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("listen on IPv4: %v", err)
	}
	defer func() { _ = ipv4Listener.Close() }()
	if err := ipv6Listener.Close(); err != nil {
		t.Fatalf("close IPv6 listener: %v", err)
	}

	s := mkTCPSettings(port)
	s.Server.IPv6 = "::1"
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	conn, err := dial(ctx, ctx, s)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if got, err := transportServerAddr(conn); err != nil || got != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("remote address = %v, %v; want 127.0.0.1", got, err)
	}
}

func TestDialTCPReturnsIPv4DialError(t *testing.T) {
	t.Parallel()

	ipv6Listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	defer func() { _ = ipv6Listener.Close() }()
	port := ipv6Listener.Addr().(*net.TCPAddr).Port
	ipv4Listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("reserve IPv4 endpoint: %v", err)
	}
	defer func() { _ = ipv4Listener.Close() }()
	if err := ipv6Listener.Close(); err != nil {
		t.Fatalf("close IPv6 listener: %v", err)
	}
	if err := ipv4Listener.Close(); err != nil {
		t.Fatalf("close IPv4 listener: %v", err)
	}

	s := mkTCPSettings(port)
	s.Server.IPv6 = "::1"
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	conn, err := dial(ctx, ctx, s)
	if conn != nil {
		_ = conn.Close()
		t.Fatal("dial unexpectedly succeeded")
	}
	var opErr *net.OpError
	if !errors.As(err, &opErr) || opErr.Addr == nil {
		t.Fatalf("dial error = %v, want an endpoint-specific dial error", err)
	}
	want := netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), uint16(port))
	if got := parseNetAddrPort(opErr.Addr); got != want {
		t.Fatalf("dial error endpoint = %v, want %v", got, want)
	}
}

func TestDialTCPReturnsIPv4ResolutionError(t *testing.T) {
	t.Parallel()

	s := mkTCPSettings(443)
	s.Server = settings.Host{IPv4: "invalid-ipv4", IPv6: "invalid-ipv6"}
	_, wantErr := netip.ParseAddr(s.Server.IPv4)
	_, err := dial(t.Context(), t.Context(), s)
	if err == nil || !strings.Contains(err.Error(), wantErr.Error()) {
		t.Fatalf("dial error = %v, want IPv4 address error %v", err, wantErr)
	}
}

func TestDialUDPReturnsIPv4DialError(t *testing.T) {
	t.Parallel()

	s := mkUDPSettings(9)
	s.Server.IPv6 = "::1"
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Make both dial attempts fail without relying on network routing.
	conn, err := dial(ctx, ctx, s)
	if conn != nil {
		_ = conn.Close()
		t.Fatal("dial unexpectedly succeeded")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dial error = %v, want context.Canceled", err)
	}
	var opErr *net.OpError
	if !errors.As(err, &opErr) || opErr.Addr == nil {
		t.Fatalf("dial error = %v, want an endpoint-specific dial error", err)
	}
	want := netip.MustParseAddrPort("127.0.0.1:9")
	if got := parseNetAddrPort(opErr.Addr); got != want {
		t.Fatalf("dial error endpoint = %v, want %v", got, want)
	}
}

func connectionTestDNS(t *testing.T, ipv4, ipv6 string) func() []dnsmessage.Type {
	t.Helper()
	dns, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var queries []dnsmessage.Type
	done := make(chan struct{})
	t.Cleanup(func() {
		_ = dns.Close()
		<-done
	})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, peer, err := dns.ReadFrom(buf)
			if err != nil {
				return
			}
			var request dnsmessage.Message
			if err := request.Unpack(buf[:n]); err != nil || len(request.Questions) != 1 {
				t.Errorf("invalid DNS request: %v", err)
				return
			}
			question := request.Questions[0]
			mu.Lock()
			queries = append(queries, question.Type)
			mu.Unlock()
			response := dnsmessage.Message{
				Header: dnsmessage.Header{
					ID: request.ID, Response: true, RecursionDesired: true, RecursionAvailable: true,
				},
				Questions: request.Questions,
			}
			var body dnsmessage.ResourceBody
			switch question.Type {
			case dnsmessage.TypeAAAA:
				if ipv6 != "" {
					body = &dnsmessage.AAAAResource{AAAA: netip.MustParseAddr(ipv6).As16()}
				}
			case dnsmessage.TypeA:
				if ipv4 != "" {
					body = &dnsmessage.AResource{A: netip.MustParseAddr(ipv4).As4()}
				}
			default:
				t.Errorf("unexpected DNS question: %v", question.Type)
				return
			}
			if body != nil {
				response.Answers = []dnsmessage.Resource{{
					Header: dnsmessage.ResourceHeader{Name: question.Name, Type: question.Type, Class: dnsmessage.ClassINET},
					Body:   body,
				}}
			}
			packet, err := response.Pack()
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := dns.WriteTo(packet, peer); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	previousResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp4", dns.LocalAddr().String())
	}}
	t.Cleanup(func() { net.DefaultResolver = previousResolver })
	return func() []dnsmessage.Type {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(queries)
	}
}

func TestDialAddressSourcePriority(t *testing.T) {
	// These subtests replace DNS and HTTP defaults; keep them sequential.
	proxyTestTransport(t, nil, nil)
	for _, protocol := range []settings.Protocol{settings.TCP, settings.UDP, settings.WS} {
		t.Run(protocol.String(), func(t *testing.T) {
			for _, tc := range []struct {
				name    string
				dnsIPv4 string
				dnsIPv6 string
				wantIP  string
			}{
				{"domain before explicit addresses", "127.0.0.1", "", "127.0.0.1"},
				{"IPv6 after DNS failure", "", "", "::1"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					ipv6Listener, err := net.Listen("tcp6", "[::1]:0")
					if err != nil {
						t.Skipf("IPv6 loopback unavailable: %v", err)
					}
					t.Cleanup(func() { _ = ipv6Listener.Close() })
					port := ipv6Listener.Addr().(*net.TCPAddr).Port
					ipv4Listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = ipv4Listener.Close() })
					for _, listener := range []net.Listener{ipv6Listener, ipv4Listener} {
						server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							conn, err := websocket.Accept(w, r, nil)
							if err == nil {
								_ = conn.CloseNow()
							}
						})}
						go func() { _ = server.Serve(listener) }()
						t.Cleanup(func() { _ = server.Close() })
					}
					queries := connectionTestDNS(t, tc.dnsIPv4, tc.dnsIPv6)
					s := settings.Settings{
						Network: settings.Network{
							Server: settings.Host{Domain: "dial-source.test.", IPv4: "127.0.0.2", IPv6: "::1"},
							Port:   port,
						},
						Protocol: protocol,
					}
					ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
					defer cancel()
					conn, err := dial(ctx, ctx, s)
					if err != nil {
						t.Fatalf("dial: %v", err)
					}
					defer func() { _ = conn.Close() }()
					if got, err := transportServerAddr(conn); err != nil || got != netip.MustParseAddr(tc.wantIP) {
						t.Fatalf("remote address = %v, %v; want %s", got, err, tc.wantIP)
					}
					gotQueries := queries()
					slices.Sort(gotQueries)
					if want := []dnsmessage.Type{dnsmessage.TypeA, dnsmessage.TypeAAAA}; !slices.Equal(gotQueries, want) {
						t.Fatalf("DNS queries = %v, want %v", gotQueries, want)
					}
				})
			}
		})
	}
}

func TestDialReservesTimeForIPFallback(t *testing.T) {
	proxyTestTransport(t, nil, nil)
	connectionTestDNS(t, "127.0.0.1", "")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	ipv6Listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("IPv6 loopback unavailable: %v", err)
	}
	t.Cleanup(func() { _ = ipv6Listener.Close() })
	port := ipv6Listener.Addr().(*net.TCPAddr).Port
	ipv4Listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ipv4Listener.Close() })
	domainCancelled := make(chan struct{})
	domainServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(domainCancelled)
	})}
	go func() { _ = domainServer.Serve(ipv4Listener) }()
	t.Cleanup(func() { _ = domainServer.Close() })
	fallbackServer := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err == nil {
			_ = conn.CloseNow()
		}
	})}
	go func() { _ = fallbackServer.Serve(ipv6Listener) }()
	t.Cleanup(func() { _ = fallbackServer.Close() })
	s := mkWSSettings("dial-budget.test.", port, settings.WS)
	s.Server.IPv6 = "::1"
	conn, err := dial(ctx, ctx, s)
	if err != nil {
		t.Fatalf("fallback: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if got, err := transportServerAddr(conn); err != nil || got != netip.MustParseAddr("::1") {
		t.Fatalf("fallback address = %v, %v, want ::1", got, err)
	}
	select {
	case <-domainCancelled:
	case <-ctx.Done():
		t.Fatal("the stalled domain attempt was not closed before fallback")
	}
}

func TestDialTCPDomainSucceeds(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen tcp: %v", err)
	}
	defer func() { _ = ln.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		if conn, acceptErr := ln.Accept(); acceptErr == nil {
			_ = conn.Close()
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	s := settings.Settings{
		Network: settings.Network{
			Server: mustHost("localhost"),
			Port:   port,
		},
		Protocol: settings.TCP,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, dialErr := dial(ctx, ctx, s)
	if dialErr != nil {
		t.Fatalf("expected domain dial success, got %v", dialErr)
	}
	_ = conn.Close()
	<-done
}

func TestDialWebSocketAddressFallback(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name         string
		server       settings.Host
		rejectIPv6   bool
		wantIP       string
		wantRequests []string
	}{
		{
			name: "IPv6 first", server: settings.Host{IPv4: "127.0.0.1", IPv6: "::1"},
			wantIP: "::1", wantRequests: []string{"IPv6"},
		},
		{
			name: "IPv4 after IPv6 upgrade rejection", server: settings.Host{IPv4: "127.0.0.1", IPv6: "::1"},
			rejectIPv6: true, wantIP: "127.0.0.1", wantRequests: []string{"IPv6", "IPv4"},
		},
		{
			name: "IPv6 only", server: settings.Host{IPv6: "::1"},
			wantIP: "::1", wantRequests: []string{"IPv6"},
		},
		{
			name: "IPv6 only rejection is not retried", server: settings.Host{IPv6: "::1"},
			rejectIPv6: true, wantRequests: []string{"IPv6"},
		},
		{
			name: "IPv4 only", server: settings.Host{IPv4: "127.0.0.1"},
			wantIP: "127.0.0.1", wantRequests: []string{"IPv4"},
		},
		{
			name: "IPv6 field rejects IPv4", server: settings.Host{IPv6: "127.0.0.1"},
		},
		{
			name: "IPv4 field rejects IPv6", server: settings.Host{IPv4: "::1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			ipv6Listener, err := net.Listen("tcp6", "[::1]:0")
			if err != nil {
				t.Skipf("IPv6 loopback unavailable: %v", err)
			}
			t.Cleanup(func() { _ = ipv6Listener.Close() })
			port := ipv6Listener.Addr().(*net.TCPAddr).Port
			ipv4Listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ipv4Listener.Close() })
			requests := make(chan string, 4)
			serve := func(listener net.Listener, family string) {
				server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests <- family
					if family == "IPv6" && tc.rejectIPv6 {
						http.Error(w, "upgrade rejected", http.StatusServiceUnavailable)
						return
					}
					conn, err := websocket.Accept(w, r, nil)
					if err != nil {
						t.Error(err)
						return
					}
					defer func() { _ = conn.CloseNow() }()
					kind, payload, err := conn.Read(ctx)
					if err == nil {
						_ = conn.Write(ctx, kind, payload)
					}
				})}
				go func() { _ = server.Serve(listener) }()
				t.Cleanup(func() { _ = server.Close() })
			}
			serve(ipv6Listener, "IPv6")
			serve(ipv4Listener, "IPv4")
			s := mkWSSettings("", port, settings.WS)
			s.Server = tc.server
			establishCtx, cancelEstablish := context.WithCancel(ctx)
			defer cancelEstablish()
			conn, err := dial(establishCtx, ctx, s)
			cancelEstablish()
			if tc.wantIP == "" {
				if conn != nil {
					_ = conn.Close()
					t.Fatal("dial unexpectedly returned a connection")
				}
				if err == nil {
					t.Fatal("expected WebSocket dial error")
				}
			} else {
				if err != nil {
					t.Fatalf("dial: %v", err)
				}
				defer func() { _ = conn.Close() }()
				if got, err := transportServerAddr(conn); err != nil || got != netip.MustParseAddr(tc.wantIP) {
					t.Fatalf("remote address = %v, %v; want %s", got, err, tc.wantIP)
				}
				payload := []byte("connection outlives dial context")
				if _, err := conn.Write(payload); err != nil {
					t.Fatal(err)
				}
				got := make([]byte, len(payload))
				if _, err := io.ReadFull(conn, got); err != nil {
					t.Fatal(err)
				}
				if string(got) != string(payload) {
					t.Fatalf("echo = %q, want %q", got, payload)
				}
			}
			var gotRequests []string
			for len(requests) > 0 {
				gotRequests = append(gotRequests, <-requests)
			}
			if !slices.Equal(gotRequests, tc.wantRequests) {
				t.Fatalf("requests = %v, want %v", gotRequests, tc.wantRequests)
			}
		})
	}
}

func TestCloneDefaultTransport_WhenGlobalDefaultIsCustomRoundTripper(t *testing.T) {
	old := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("not used")
	})
	t.Cleanup(func() {
		http.DefaultTransport = old
	})

	tr := cloneDefaultTransport()
	if tr == nil {
		t.Fatal("expected non-nil transport clone")
	}
}
