package client

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
	transportaddr "tungo/internal/transport"
	transport "tungo/internal/transport/tcp"
	transportws "tungo/internal/transport/ws"
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

// connectionTestSettings returns endpoint settings; configuration helpers supply the TUN defaults.
func connectionTestSettings(host string, port int, proto settings.Protocol) settings.Settings {
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
	conf := clientTestConfiguration()
	selected.TunName = conf.TCPSettings.TunName
	selected.IPv4Subnet = conf.TCPSettings.IPv4Subnet
	selected.MTU = conf.TCPSettings.MTU
	conf.Protocol = selected.Protocol
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

func connectionTestClient(t *testing.T, configuration clientconfig.Configuration) *Client {
	t.Helper()
	client := newTestClient(t)
	importClientConfiguration(t, client.configurations, "connection", configuration)
	if err := client.configurations.Activate("connection"); err != nil {
		t.Fatal(err)
	}
	return client
}

func connectionTestHandshake(t *testing.T, s settings.Settings) (clientconfig.Configuration, *noise.IKHandshake) {
	t.Helper()
	deriver := &keys.DefaultKeyDeriver{}
	serverPublic, serverPrivate, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientPublic, clientPrivate, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatal(err)
	}
	configuration := testConfigurationWithSettings(s)
	configuration.ClientPublicKey = clientPublic
	configuration.ClientPrivateKey = clientPrivate[:]
	configuration.X25519PublicKey = serverPublic
	server := noise.NewIKHandshakeServer(serverPublic, serverPrivate[:],
		newTestAllowedPeers([]testPeer{{PublicKey: clientPublic, Enabled: true, ClientID: 1}}), nil, nil)
	return configuration, server
}

// The transport stays open for one echo after the handshake. This verifies that
// connect's deferred cancellation does not close an established connection.
func connectionTestEcho(t *testing.T, handshake *noise.IKHandshake, conn net.Conn) {
	t.Helper()
	if _, err := handshake.ServerSideHandshake(conn); err != nil {
		t.Errorf("server handshake: %v", err)
		return
	}
	buf := make([]byte, settings.DefaultEthernetMTU)
	if n, err := conn.Read(buf); err == nil {
		_, _ = conn.Write(buf[:n])
	}
}

func connectionTestWSHandler(t *testing.T, ctx context.Context, handle func(net.Conn)) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.CloseNow() }()
		stop := context.AfterFunc(ctx, func() { _ = conn.CloseNow() })
		defer stop()
		framed, err := transport.NewFramedConn(transportws.NewConn(ctx, conn, nil, nil),
			settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
		if err != nil {
			t.Error(err)
			return
		}
		handle(framed)
	}
}

// A single peer is sufficient for these local UDP handshake fixtures.
type connectionTestUDPConn struct {
	*net.UDPConn
	peer  *net.UDPAddr
	first []byte
}

func (c *connectionTestUDPConn) Read(p []byte) (int, error) {
	if c.first != nil {
		n := copy(p, c.first)
		c.first = nil
		return n, nil
	}
	n, peer, err := c.ReadFromUDP(p)
	if err == nil {
		c.peer = peer
	}
	return n, err
}

func (c *connectionTestUDPConn) RemoteAddr() net.Addr { return c.peer }

func (c *connectionTestUDPConn) Write(p []byte) (int, error) {
	return c.WriteToUDP(p, c.peer)
}

func connectionTestListen(t *testing.T, protocol settings.Protocol, host string, port int, handle func(net.Conn)) int {
	t.Helper()
	endpoint := net.JoinHostPort(host, strconv.Itoa(port))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	if protocol == settings.UDP {
		addr, err := net.ResolveUDPAddr("udp", endpoint)
		if err != nil {
			t.Fatal(err)
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			cancel()
			if host == "::1" {
				t.Skipf("IPv6 loopback unavailable: %v", err)
			}
			t.Fatal(err)
		}
		t.Cleanup(func() { cancel(); _ = conn.Close(); <-done })
		go func() {
			defer close(done)
			buf := make([]byte, settings.DefaultEthernetMTU)
			n, peer, err := conn.ReadFromUDP(buf)
			if err == nil {
				handle(&connectionTestUDPConn{UDPConn: conn, peer: peer, first: buf[:n]})
			}
		}()
		return conn.LocalAddr().(*net.UDPAddr).Port
	}
	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		cancel()
		if host == "::1" {
			t.Skipf("IPv6 loopback unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if protocol == settings.WS {
		server := httptest.NewUnstartedServer(connectionTestWSHandler(t, ctx, handle))
		_ = server.Listener.Close()
		server.Listener = listener
		server.Start()
		t.Cleanup(func() { cancel(); server.Close() })
		return listener.Addr().(*net.TCPAddr).Port
	}
	t.Cleanup(func() { cancel(); _ = listener.Close(); <-done })
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		defer stop()
		framed, err := transport.NewFramedConn(conn, settings.DefaultEthernetMTU+settings.TCPChacha20Overhead)
		if err != nil {
			t.Error(err)
			return
		}
		handle(framed)
	}()
	return listener.Addr().(*net.TCPAddr).Port
}

func connectionTestRoundTrip(t *testing.T, conn net.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q, %v; want ping", buf, err)
	}
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
	if remote, err := transportaddr.NetipAddrFrom(adapter.RemoteAddr()); err != nil || remote != ap.Addr() {
		t.Fatalf("remote address = %v, %v; want %v", remote, err, ap.Addr())
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
	if remote, err := transportaddr.NetipAddrFrom(conn.RemoteAddr()); err != nil || remote != ap.Addr() {
		t.Fatalf("UDP remote address = %v, %v; want %v", remote, err, ap.Addr())
	}
	_ = conn.Close()
}

func TestDialWS_Success(t *testing.T) {
	t.Parallel()
	host, port, shutdown := ConnectionFactoryMockWSServer(t)
	defer shutdown()

	adapter, err := dialWS(context.Background(), context.Background(), "ws", net.JoinHostPort(host, port), "")
	if err != nil {
		t.Fatalf("dialWS failed: %v", err)
	}
	if adapter == nil {
		t.Fatalf("adapter must not be nil")
	}
	if remote, err := transportaddr.NetipAddrFrom(adapter.RemoteAddr()); err != nil || remote.String() != host {
		t.Fatalf("WS remote address = %v, %v; want %s", remote, err, host)
	}
	_ = adapter.Close()
}

func TestDialWS_Success_DomainEndpointHasRemoteAddr(t *testing.T) {
	t.Parallel()
	_, port, shutdown := ConnectionFactoryMockWSServer(t)
	defer shutdown()

	adapter, err := dialWS(context.Background(), context.Background(), "ws", net.JoinHostPort("localhost", port), "")
	if err != nil {
		t.Fatalf("dialWS failed: %v", err)
	}
	if adapter == nil {
		t.Fatalf("adapter must not be nil")
	}
	if remote, err := transportaddr.NetipAddrFrom(adapter.RemoteAddr()); err != nil || !remote.IsLoopback() {
		t.Fatalf("WS domain remote address = %v, %v; want loopback", remote, err)
	}
	_ = adapter.Close()
}

func TestDialWS_Error_NoServer(t *testing.T) {
	t.Parallel()
	// Use a port with no WS server
	adapter, err := dialWS(context.Background(), context.Background(), "ws", net.JoinHostPort("127.0.0.1", "1"), "")
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

type cfUnitTransport struct {
	net.Conn
	readErr error
	closed  bool
}

func (t *cfUnitTransport) Read([]byte) (int, error) {
	if t.readErr != nil {
		return 0, t.readErr
	}
	return 0, io.EOF
}

func (t *cfUnitTransport) Write(p []byte) (int, error) {
	return len(p), nil
}

func (t *cfUnitTransport) Close() error {
	t.closed = true
	return nil
}

type cfBlockingTransport struct {
	net.Conn
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

func TestHandshake_ConfigurationErrorClosesAdapter(t *testing.T) {
	client := newTestClient(t)
	tr := &cfUnitTransport{}
	conn, crypto, rekey, err := client.handshake(t.Context(), tr, settings.TCP)
	if err == nil || conn != nil || crypto != nil || rekey != nil {
		t.Fatalf("handshake with no configuration: conn=%v crypto=%v rekey=%v err=%v", conn, crypto, rekey, err)
	}
	if !tr.closed {
		t.Fatal("configuration error did not close the adapter")
	}
}

func TestHandshake_HandshakeError_ClosesAdapter(t *testing.T) {
	configuration, _ := connectionTestHandshake(t, connectionTestSettings("127.0.0.1", 8080, settings.TCP))
	client := connectionTestClient(t, configuration)
	tr := &cfUnitTransport{readErr: io.ErrUnexpectedEOF}

	_, _, _, err := client.handshake(
		context.Background(),
		tr,
		settings.TCP,
	)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("handshake error = %v, want underlying read error", err)
	}
	if !tr.closed {
		t.Fatal("expected adapter to be closed on handshake error")
	}
}

func TestHandshake_CancelClosesAdapter(t *testing.T) {
	configuration, _ := connectionTestHandshake(t, connectionTestSettings("127.0.0.1", 8080, settings.TCP))
	client := connectionTestClient(t, configuration)
	transport := newCFBlockingTransport()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, _, _, err := client.handshake(ctx, transport, settings.TCP)
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

	t.Run("dialWS error", func(t *testing.T) {
		_, err := dialWS(ctx, context.Background(), "ws", net.JoinHostPort("127.0.0.1", "1"), "")
		if err == nil {
			t.Fatal("expected dialWS error")
		}
	})
}

func TestHandshake_Success(t *testing.T) {
	configuration, serverHS := connectionTestHandshake(t, connectionTestSettings("127.0.0.1", 8080, settings.TCP))
	client := connectionTestClient(t, configuration)

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

	serverErrCh := make(chan error, 1)
	go func() {
		_, serr := serverHS.ServerSideHandshake(serverAdapter)
		serverErrCh <- serr
	}()

	adapter, crypto, coordinator, err := client.handshake(
		context.Background(),
		clientAdapter,
		settings.TCP,
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

func connectionTestDNS(t *testing.T, addresses ...string) func() []dnsmessage.Type {
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
			for _, raw := range addresses {
				addr := netip.MustParseAddr(raw)
				var body dnsmessage.ResourceBody
				switch {
				case question.Type == dnsmessage.TypeA && addr.Is4():
					body = &dnsmessage.AResource{A: addr.As4()}
				case question.Type == dnsmessage.TypeAAAA && addr.Is6():
					body = &dnsmessage.AAAAResource{AAAA: addr.As16()}
				}
				if body != nil {
					response.Answers = append(response.Answers, dnsmessage.Resource{
						Header: dnsmessage.ResourceHeader{Name: question.Name, Type: question.Type, Class: dnsmessage.ClassINET},
						Body:   body,
					})
				}
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

func TestConnect_ConfigurationError(t *testing.T) {
	client := newTestClient(t)
	conn, crypto, rekey, err := client.connect(t.Context())
	if err == nil || conn != nil || crypto != nil || rekey != nil {
		t.Fatalf("connect with no configuration: conn=%v crypto=%v rekey=%v err=%v", conn, crypto, rekey, err)
	}
}

func TestConnect_DialErrorsPreserveCause(t *testing.T) {
	proxyTestTransport(t, nil, nil)
	for _, protocol := range []settings.Protocol{settings.TCP, settings.WS, settings.WSS} {
		t.Run(protocol.String(), func(t *testing.T) {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			port := listener.Addr().(*net.TCPAddr).Port
			_ = listener.Close()
			s := connectionTestSettings("127.0.0.1", port, protocol)
			client := connectionTestClient(t, testConfigurationWithSettings(s))
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			conn, crypto, rekey, err := client.connect(ctx)
			var dialErr *net.OpError
			if !errors.As(err, &dialErr) || dialErr.Op != "dial" {
				t.Fatalf("connect error = %v, want underlying dial error", err)
			}
			if conn != nil || crypto != nil || rekey != nil {
				t.Fatal("failed connection returned a transport or session state")
			}
		})
	}
}

func TestConnect_AddressPriority(t *testing.T) {
	proxyTestTransport(t, nil, nil)
	for _, protocol := range []settings.Protocol{settings.TCP, settings.UDP, settings.WS} {
		for _, tc := range []struct {
			name string
			host settings.Host
			dns  []string
			want string
		}{
			{"explicit IPv4 before IPv6", settings.Host{IPv4: "127.0.0.1", IPv6: "::1"}, nil, "127.0.0.1"},
			{"IPv6 only", settings.Host{IPv6: "::1"}, nil, "::1"},
			{"domain IPv4 before IPv6", settings.Host{Domain: "connect.test", IPv6: "::1"}, []string{"127.0.0.1", "::1"}, "127.0.0.1"},
			{"domain IPv6 before explicit IPv4", settings.Host{Domain: "connect.test", IPv4: "127.0.0.1"}, []string{"::1"}, "::1"},
			{"domain IPv4 without AAAA", settings.Host{Domain: "connect.test", IPv6: "::1"}, []string{"127.0.0.1"}, "127.0.0.1"},
			{"explicit IPv4 after DNS failure", settings.Host{Domain: "connect.test", IPv4: "127.0.0.1", IPv6: "::1"}, nil, "127.0.0.1"},
		} {
			// WS leaves family selection to the HTTP dialer, including proxy DNS.
			if protocol == settings.WS && tc.name == "domain IPv4 before IPv6" {
				continue
			}
			t.Run(protocol.String()+"/"+tc.name, func(t *testing.T) {
				queries := connectionTestDNS(t, tc.dns...)
				s := settings.Settings{Network: settings.Network{Server: tc.host}, Protocol: protocol}
				configuration, handshake := connectionTestHandshake(t, s)
				attempts := make(chan string, 2)
				handle := func(host string) func(net.Conn) {
					return func(conn net.Conn) {
						attempts <- host
						if tc.host.Domain != "" && protocol != settings.WS {
							got := queries()
							if !slices.Contains(got, dnsmessage.TypeA) || !slices.Contains(got, dnsmessage.TypeAAAA) {
								t.Errorf("connection started before both DNS families were resolved: %v", got)
							}
						}
						connectionTestEcho(t, handshake, conn)
					}
				}
				port := connectionTestListen(t, protocol, "::1", 0, handle("::1"))
				connectionTestListen(t, protocol, "127.0.0.1", port, handle("127.0.0.1"))
				configuration.TCPSettings.Port = port
				configuration.UDPSettings.Port = port
				configuration.WSSettings.Port = port
				client := connectionTestClient(t, configuration)
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				conn, crypto, rekey, err := client.connect(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = conn.Close() }()
				if crypto == nil || rekey == nil {
					t.Fatal("successful connection has no crypto or rekey state")
				}
				if got := <-attempts; got != tc.want {
					t.Fatalf("first peer = %s, want %s", got, tc.want)
				}
				if got, err := transportaddr.NetipAddrFrom(conn.RemoteAddr()); err != nil || got.String() != tc.want {
					t.Fatalf("remote address = %v, %v; want %s", got, err, tc.want)
				}
				connectionTestRoundTrip(t, conn)
				if tc.host.Domain == "" && len(queries()) != 0 {
					t.Fatal("explicit addresses triggered a DNS lookup")
				}
			})
		}
	}
}

func TestConnect_TriesAllDomainAddresses(t *testing.T) {
	for _, protocol := range []settings.Protocol{settings.TCP, settings.UDP} {
		t.Run(protocol.String(), func(t *testing.T) {
			// The first address has no listener. The second must still be tried.
			connectionTestDNS(t, "127.0.0.2", "127.0.0.1")
			s := settings.Settings{Network: settings.Network{Server: settings.Host{Domain: "connect.test"}}, Protocol: protocol}
			configuration, handshake := connectionTestHandshake(t, s)
			port := connectionTestListen(t, protocol, "127.0.0.1", 0, func(conn net.Conn) {
				connectionTestEcho(t, handshake, conn)
			})
			configuration.TCPSettings.Port = port
			configuration.UDPSettings.Port = port
			client := connectionTestClient(t, configuration)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			resolved, err := client.resolveServerAddresses(ctx, s)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"127.0.0.2", "127.0.0.1"}
			if !slices.Equal(resolved, want) {
				t.Fatalf("DNS candidates = %v, want unreachable address before listener: %v", resolved, want)
			}
			conn, _, _, err := client.connect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			connectionTestRoundTrip(t, conn)
		})
	}
}

func TestConnect_HandshakeFailureFallsBack(t *testing.T) {
	proxyTestTransport(t, nil, nil)
	for _, protocol := range []settings.Protocol{settings.TCP, settings.UDP, settings.WS} {
		for _, failure := range []string{"invalid response", "timeout"} {
			t.Run(protocol.String()+"/"+failure, func(t *testing.T) {
				s := settings.Settings{Network: settings.Network{Server: settings.Host{IPv4: "127.0.0.1", IPv6: "::1"}}, Protocol: protocol}
				if failure == "invalid response" && protocol != settings.WS {
					connectionTestDNS(t, "127.0.0.1", "::1")
					s.Server = settings.Host{Domain: "connect.test"}
				}
				configuration, handshake := connectionTestHandshake(t, s)
				port := connectionTestListen(t, protocol, "::1", 0, func(conn net.Conn) {
					connectionTestEcho(t, handshake, conn)
				})
				started := make(chan struct{})
				closed := make(chan struct{})
				connectionTestListen(t, protocol, "127.0.0.1", port, func(conn net.Conn) {
					buf := make([]byte, settings.DefaultEthernetMTU)
					if _, err := conn.Read(buf); err != nil {
						t.Errorf("first handshake request: %v", err)
						return
					}
					close(started)
					if failure == "invalid response" {
						if _, err := conn.Write([]byte{0}); err != nil {
							t.Errorf("reject handshake: %v", err)
							return
						}
					}
					if _, err := conn.Read(buf); err == nil {
						t.Error("unexpected packet after failed handshake")
					}
					close(closed)
				})
				configuration.TCPSettings.Port = port
				configuration.UDPSettings.Port = port
				configuration.WSSettings.Port = port
				client := connectionTestClient(t, configuration)
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				conn, _, _, err := client.connect(ctx)
				if err != nil {
					t.Fatalf("fallback after %s: %v", failure, err)
				}
				defer func() { _ = conn.Close() }()
				select {
				case <-started:
				default:
					t.Fatal("skipped the first handshake attempt")
				}
				if got, err := transportaddr.NetipAddrFrom(conn.RemoteAddr()); err != nil || got.String() != "::1" {
					t.Fatalf("fallback address = %v, %v; want ::1", got, err)
				}
				connectionTestRoundTrip(t, conn)
				// UDP has no remote close notification; stream transports do.
				if protocol != settings.UDP {
					select {
					case <-closed:
					case <-ctx.Done():
						t.Fatal("failed transport was not closed")
					}
				}
			})
		}
	}
}

func TestConnect_CancellationDuringHandshake(t *testing.T) {
	proxyTestTransport(t, nil, nil)
	for _, protocol := range []settings.Protocol{settings.TCP, settings.UDP, settings.WS} {
		t.Run(protocol.String(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			canceled := make(chan struct{})
			port := connectionTestListen(t, protocol, "127.0.0.1", 0, func(conn net.Conn) {
				if _, err := conn.Read(make([]byte, settings.DefaultEthernetMTU)); err != nil {
					t.Errorf("handshake request: %v", err)
					return
				}
				cancel()
				close(canceled)
				// Keep the peer open until the client closes its side or test cleanup runs.
				_, _ = conn.Read(make([]byte, settings.DefaultEthernetMTU))
			})
			s := settings.Settings{Network: settings.Network{Server: settings.Host{IPv4: "127.0.0.1"}, Port: port}, Protocol: protocol}
			configuration, _ := connectionTestHandshake(t, s)
			client := connectionTestClient(t, configuration)
			done := make(chan struct{})
			var connectErr error
			go func() {
				defer close(done)
				conn, crypto, rekey, err := client.connect(ctx)
				connectErr = err
				if conn != nil || crypto != nil || rekey != nil {
					t.Error("canceled connect returned session state")
				}
				if conn != nil {
					_ = conn.Close()
				}
			}()
			t.Cleanup(func() { cancel(); <-done })
			select {
			case <-canceled:
			case <-time.After(3 * time.Second):
				t.Fatal("handshake did not start")
			}
			select {
			case <-done:
				if !errors.Is(connectErr, context.Canceled) {
					t.Fatalf("connect error = %v, want cancellation", connectErr)
				}
			case <-time.After(time.Second):
				t.Fatal("connect did not stop after cancellation")
			}
		})
	}
}

func TestConnect_DNSFailureStillTriesExplicitAddress(t *testing.T) {
	for _, protocol := range []settings.Protocol{settings.TCP, settings.UDP} {
		t.Run(protocol.String(), func(t *testing.T) {
			connectionTestDNS(t)
			started := make(chan struct{})
			port := connectionTestListen(t, protocol, "127.0.0.1", 0, func(conn net.Conn) {
				if _, err := conn.Read(make([]byte, settings.DefaultEthernetMTU)); err != nil {
					t.Errorf("handshake request: %v", err)
					return
				}
				close(started)
				_, _ = conn.Read(make([]byte, settings.DefaultEthernetMTU))
			})
			s := settings.Settings{Network: settings.Network{Server: settings.Host{Domain: "connect.test", IPv4: "127.0.0.1"}, Port: port}, Protocol: protocol}
			configuration, _ := connectionTestHandshake(t, s)
			client := connectionTestClient(t, configuration)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			conn, crypto, rekey, err := client.connect(ctx)
			select {
			case <-started:
			default:
				t.Fatal("explicit address handshake did not start after DNS failure")
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("connect error = %v, want handshake timeout", err)
			}
			if conn != nil || crypto != nil || rekey != nil {
				t.Fatal("failed attempts returned session state")
			}
		})
	}
}

func TestConnect_DNSFailureWithoutFallback(t *testing.T) {
	for _, protocol := range []settings.Protocol{settings.TCP, settings.UDP} {
		t.Run(protocol.String(), func(t *testing.T) {
			connectionTestDNS(t)
			s := connectionTestSettings("missing.test", 443, protocol)
			client := connectionTestClient(t, testConfigurationWithSettings(s))
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			addresses, err := client.resolveServerAddresses(ctx, s)
			if err == nil || len(addresses) != 0 {
				t.Fatalf("resolveServerAddresses = %v, %v; want no addresses and an error", addresses, err)
			}
			conn, crypto, rekey, err := client.connect(ctx)
			if conn != nil {
				defer func() { _ = conn.Close() }()
			}
			if err == nil || conn != nil || crypto != nil || rekey != nil {
				t.Fatalf("DNS failure without fallback: conn=%v crypto=%v rekey=%v err=%v", conn, crypto, rekey, err)
			}
			if ctx.Err() != nil {
				t.Fatalf("connect waited for the context instead of rejecting an empty address list: %v", err)
			}
		})
	}
}

func TestConnect_ContextEndsDuringDNS(t *testing.T) {
	// These tests replace the default resolver; keep them sequential.
	for _, protocol := range []settings.Protocol{settings.TCP, settings.UDP} {
		for _, tc := range []struct {
			name string
			want error
		}{
			{"cancel", context.Canceled},
			{"deadline", context.DeadlineExceeded},
		} {
			t.Run(protocol.String()+"/"+tc.name, func(t *testing.T) {
				previous := net.DefaultResolver
				started := make(chan struct{}, 1)
				net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
					select {
					case started <- struct{}{}:
					default:
					}
					<-ctx.Done()
					return nil, ctx.Err()
				}}
				t.Cleanup(func() { net.DefaultResolver = previous })
				s := connectionTestSettings("connect.test", 443, protocol)
				client := connectionTestClient(t, testConfigurationWithSettings(s))
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				done := make(chan struct{})
				var connectErr error
				go func() {
					defer close(done)
					conn, crypto, rekey, err := client.connect(ctx)
					connectErr = err
					if conn != nil || crypto != nil || rekey != nil {
						t.Error("interrupted DNS lookup returned session state")
					}
					if conn != nil {
						_ = conn.Close()
					}
				}()
				t.Cleanup(func() { cancel(); <-done })
				select {
				case <-started:
				case <-time.After(3 * time.Second):
					t.Fatal("DNS lookup did not start")
				}
				if tc.want == context.Canceled {
					cancel()
				}
				select {
				case <-done:
					if !errors.Is(connectErr, tc.want) {
						t.Fatalf("connect error = %v, want %v", connectErr, tc.want)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("DNS lookup ignored the parent context")
				}
			})
		}
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
