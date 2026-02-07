package client_factory

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"tungo/application/network/connection"
	framelimit "tungo/domain/network/ip/frame_limit"
	"tungo/infrastructure/PAL/configuration/client"
	serverCfg "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/infrastructure/cryptography/chacha20/rekey"
	"tungo/infrastructure/cryptography/noise"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/network/tcp/adapters"
	"tungo/infrastructure/settings"
)

// mkTCPSettings returns minimal TCP settings for a given port.
func mkTCPSettings(port string) settings.Settings {
	return settings.Settings{
		ConnectionIP:  "127.0.0.1",
		Port:          port,
		Protocol:      settings.TCP,
		DialTimeoutMs: 100,
	}
}

// mkUDPSettings returns minimal UDP settings for a given port.
func mkUDPSettings(port string) settings.Settings {
	return settings.Settings{
		ConnectionIP:  "127.0.0.1",
		Port:          port,
		Protocol:      settings.UDP,
		DialTimeoutMs: 100,
	}
}

// mkWSSettings returns minimal WS/WSS settings.
func mkWSSettings(host, ip, port string, proto settings.Protocol) settings.Settings {
	return settings.Settings{
		Host:          host,
		ConnectionIP:  ip,
		Port:          port,
		Protocol:      proto,
		DialTimeoutMs: 200,
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

func Test_connectionSettings_TCP(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:    settings.TCP,
		TCPSettings: mkTCPSettings("443"),
	}
	f := &ConnectionFactory{conf: conf}
	got, err := f.connectionSettings()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Protocol != settings.TCP || got.Port != "443" {
		t.Fatalf("wrong settings returned: %+v", got)
	}
}

func Test_connectionSettings_UDP(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:    settings.UDP,
		UDPSettings: mkUDPSettings("53"),
	}
	f := &ConnectionFactory{conf: conf}
	got, err := f.connectionSettings()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Protocol != settings.UDP || got.Port != "53" {
		t.Fatalf("wrong settings returned: %+v", got)
	}
}

func Test_connectionSettings_WS(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("example.org", "", "80", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	got, err := f.connectionSettings()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Protocol != settings.WS || got.Port != "80" || got.Host != "example.org" {
		t.Fatalf("wrong settings returned: %+v", got)
	}
}

func Test_connectionSettings_WSS_UsesWSSettingsBucket(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WSS,
		WSSettings: mkWSSettings("secure.example", "", "443", settings.WSS),
	}
	f := &ConnectionFactory{conf: conf}
	got, err := f.connectionSettings()
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.Protocol != settings.WSS || got.Port != "443" || got.Host != "secure.example" {
		t.Fatalf("wrong settings returned: %+v", got)
	}
}

func Test_connectionSettings_Unsupported(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{Protocol: 999}
	f := &ConnectionFactory{conf: conf}
	_, err := f.connectionSettings()
	if err == nil {
		t.Fatalf("expected error for unsupported protocol")
	}
}

func TestEstablishConnection_InvalidPort_TCP_ParseError(t *testing.T) {
	t.Parallel()
	// Using "abc" as port will cause ParseAddrPort to fail.
	conf := client.Configuration{
		Protocol:    settings.TCP,
		TCPSettings: mkTCPSettings("abc"),
	}
	f := &ConnectionFactory{conf: conf}

	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil {
		t.Fatalf("expected parse error for bad port")
	}
}

func TestEstablishConnection_InvalidPort_UDP_ParseError(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:    settings.UDP,
		UDPSettings: mkUDPSettings("bad"),
	}
	f := &ConnectionFactory{conf: conf}

	_, _, _, err := f.EstablishConnection(context.Background())
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
	f := &ConnectionFactory{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	adapter, err := f.dialTCP(ctx, ap)
	if err != nil {
		t.Fatalf("dialTCP failed: %v", err)
	}
	if adapter == nil {
		t.Fatalf("adapter must not be nil on success")
	}
	_ = adapter.Close()
	<-done
}

func TestDialTCP_Refused(t *testing.T) {
	t.Parallel()
	// Port 1 on localhost is almost always closed -> should return an error quickly.
	ap := netip.MustParseAddrPort("127.0.0.1:1")
	f := &ConnectionFactory{}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	adapter, err := f.dialTCP(ctx, ap)
	if err == nil {
		_ = adapter.Close()
		t.Fatalf("expected error dialing to closed port")
	}
}

func TestDialUDP_Success_NoServerNeeded(t *testing.T) {
	t.Parallel()
	// UDP "dial" does not require a server to succeed in most cases.
	ap := netip.MustParseAddrPort("127.0.0.1:9") // discard port
	f := &ConnectionFactory{}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	conn, err := f.dialUDP(ctx, ap)
	if err != nil {
		t.Fatalf("dialUDP failed: %v", err)
	}
	if conn == nil {
		t.Fatalf("conn must not be nil")
	}
	_ = conn.Close()
}

func TestDialWS_Success(t *testing.T) {
	t.Parallel()
	host, port, shutdown := ConnectionFactoryMockWSServer(t)
	defer shutdown()

	f := &ConnectionFactory{}
	adapter, err := f.dialWS(context.Background(), context.Background(), "ws", host, port)
	if err != nil {
		t.Fatalf("dialWS failed: %v", err)
	}
	if adapter == nil {
		t.Fatalf("adapter must not be nil")
	}
	_ = adapter.Close()
}

func TestDialWS_Error_NoServer(t *testing.T) {
	t.Parallel()
	f := &ConnectionFactory{}
	// Use a port with no WS server
	adapter, err := f.dialWS(context.Background(), context.Background(), "ws", "127.0.0.1", "1")
	if err == nil {
		_ = adapter.Close()
		t.Fatalf("expected error when no WS server is listening")
	}
}

func TestEstablishConnection_WS_EmptyHost_And_EmptyConnectionIP(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("", "", "8080", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ws dial: empty host") {
		t.Fatalf("expected empty host error, got: %v", err)
	}
}

func TestEstablishConnection_WS_EmptyPort(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("127.0.0.1", "", "", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ws dial: empty port") {
		t.Fatalf("expected empty port error, got: %v", err)
	}
}

func TestEstablishConnection_WS_InvalidPort_NonNumeric(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("127.0.0.1", "", "abc", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ws dial: invalid port") {
		t.Fatalf("expected invalid port error, got: %v", err)
	}
}

func TestEstablishConnection_WS_InvalidPort_OutOfRange(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("127.0.0.1", "", "70000", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ws dial: invalid port") {
		t.Fatalf("expected invalid port error, got: %v", err)
	}
}

func TestEstablishConnection_WSS_EmptyHost(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WSS,
		WSSettings: mkWSSettings("", "", "443", settings.WSS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "wss dial: empty host") {
		t.Fatalf("expected empty host error, got: %v", err)
	}
}

func TestEstablishConnection_WSS_DefaultPort443_And_WrappedError(t *testing.T) {
	t.Parallel()
	// No port -> defaults to 443; since nothing listens, expect wrapped WS dial error.
	conf := client.Configuration{
		Protocol:   settings.WSS,
		WSSettings: mkWSSettings("127.0.0.1", "", "", settings.WSS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unable to establish WebSocket connection") {
		t.Fatalf("expected wrapped WS connect error, got: %v", err)
	}
}

func TestEstablishConnection_WSS_InvalidPort_OutOfRange(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:   settings.WSS,
		WSSettings: mkWSSettings("127.0.0.1", "", "70000", settings.WSS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "wss dial: invalid port") {
		t.Fatalf("expected invalid port error, got: %v", err)
	}
}

func TestEstablishConnection_UnsupportedProtocol(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{Protocol: 999}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil {
		t.Fatalf("expected error for unsupported protocol")
	}
}

// Verifies the error wrapping path for TCP.
func TestEstablishConnection_TCP_DialError_IsWrapped(t *testing.T) {
	t.Parallel()
	conf := client.Configuration{
		Protocol:    settings.TCP,
		TCPSettings: mkTCPSettings("1"), // likely closed → Dial error
	}
	f := &ConnectionFactory{conf: conf}

	_, _, _, err := f.EstablishConnection(context.Background())
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
	conf := client.Configuration{
		Protocol:   settings.WS,
		WSSettings: mkWSSettings("127.0.0.1", "", "9", settings.WS),
	}
	f := &ConnectionFactory{conf: conf}
	_, _, _, err := f.EstablishConnection(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unable to establish WebSocket connection") {
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

type cfUnitCryptoFactory struct {
	called bool
}

func (f *cfUnitCryptoFactory) FromHandshake(connection.Handshake, bool) (connection.Crypto, *rekey.StateMachine, error) {
	f.called = true
	return nil, nil, errors.New("unexpected factory call")
}

func TestConnectionFactoryUnit_NewConnectionFactory_ReturnsImpl(t *testing.T) {
	conf := client.Configuration{Protocol: settings.TCP}
	got := NewConnectionFactory(conf)
	if got == nil {
		t.Fatal("expected non-nil factory")
	}
	if _, ok := got.(*ConnectionFactory); !ok {
		t.Fatalf("expected *ConnectionFactory, got %T", got)
	}
}

func TestConnectionFactoryUnit_establishSecuredConnection_MissingClientKeys_ClosesAdapter(t *testing.T) {
	f := &ConnectionFactory{
		conf: client.Configuration{
			ClientPublicKey:  []byte{1, 2, 3}, // invalid length
			ClientPrivateKey: []byte{4, 5, 6}, // invalid length
		},
	}
	tr := &cfUnitTransport{}
	cryptoFactory := &cfUnitCryptoFactory{}

	_, _, _, err := f.establishSecuredConnection(
		context.Background(),
		settings.Settings{Protocol: settings.TCP},
		tr,
		cryptoFactory,
	)
	if err == nil || !strings.Contains(err.Error(), "client keys not configured") {
		t.Fatalf("expected client keys error, got %v", err)
	}
	if !tr.closed {
		t.Fatal("expected adapter to be closed on missing keys")
	}
	if cryptoFactory.called {
		t.Fatal("crypto factory should not be called when keys are missing")
	}
}

func TestConnectionFactoryUnit_establishSecuredConnection_HandshakeError_ClosesAdapter(t *testing.T) {
	clientPub := make([]byte, 32)
	clientPriv := make([]byte, 32)
	serverPub := make([]byte, 32)
	clientPub[0], clientPriv[0], serverPub[0] = 1, 2, 3

	f := &ConnectionFactory{
		conf: client.Configuration{
			ClientPublicKey:  clientPub,
			ClientPrivateKey: clientPriv,
			X25519PublicKey:  serverPub,
		},
	}
	tr := &cfUnitTransport{readErr: io.ErrUnexpectedEOF}
	cryptoFactory := &cfUnitCryptoFactory{}

	_, _, _, err := f.establishSecuredConnection(
		context.Background(),
		settings.Settings{Protocol: settings.TCP},
		tr,
		cryptoFactory,
	)
	if err == nil {
		t.Fatal("expected handshake error")
	}
	if !tr.closed {
		t.Fatal("expected adapter to be closed on handshake error")
	}
	if cryptoFactory.called {
		t.Fatal("crypto factory should not be called when handshake fails")
	}
}

func TestConnectionFactoryUnit_newReadDeadlineTransport_NoDeadlineSupport_ReturnsSame(t *testing.T) {
	tr := &cfUnitNoDeadlineTransport{}
	wrapped := newReadDeadlineTransport(tr, time.Second)
	if wrapped != tr {
		t.Fatal("expected same transport when SetReadDeadline is not supported")
	}
}

func TestConnectionFactoryUnit_newReadDeadlineTransport_WithDeadlineSupport_WrapsAndSetsDeadline(t *testing.T) {
	tr := &cfUnitTransport{readBuf: []byte("abc")}
	wrapped := newReadDeadlineTransport(tr, time.Second)

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

func TestConnectionFactoryUnit_connectionSettings_AllBranches(t *testing.T) {
	t.Run("tcp", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{
			Protocol:    settings.TCP,
			TCPSettings: settings.Settings{Protocol: settings.TCP, Port: "1"},
		}}
		s, err := f.connectionSettings()
		if err != nil || s.Protocol != settings.TCP {
			t.Fatalf("unexpected result: s=%+v err=%v", s, err)
		}
	})

	t.Run("udp", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{
			Protocol:    settings.UDP,
			UDPSettings: settings.Settings{Protocol: settings.UDP, Port: "2"},
		}}
		s, err := f.connectionSettings()
		if err != nil || s.Protocol != settings.UDP {
			t.Fatalf("unexpected result: s=%+v err=%v", s, err)
		}
	})

	t.Run("ws", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{
			Protocol:   settings.WS,
			WSSettings: settings.Settings{Protocol: settings.WS, Port: "80"},
		}}
		s, err := f.connectionSettings()
		if err != nil || s.Protocol != settings.WS {
			t.Fatalf("unexpected result: s=%+v err=%v", s, err)
		}
	})

	t.Run("wss", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{
			Protocol:   settings.WSS,
			WSSettings: settings.Settings{Protocol: settings.WSS, Port: "443"},
		}}
		s, err := f.connectionSettings()
		if err != nil || s.Protocol != settings.WSS {
			t.Fatalf("unexpected result: s=%+v err=%v", s, err)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{Protocol: settings.UNKNOWN}}
		_, err := f.connectionSettings()
		if err == nil {
			t.Fatal("expected unsupported protocol error")
		}
	})
}

func TestConnectionFactoryUnit_EstablishConnection_ErrorBranches(t *testing.T) {
	t.Run("unsupported protocol", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{Protocol: settings.UNKNOWN}}
		_, _, _, err := f.EstablishConnection(context.Background())
		if err == nil {
			t.Fatal("expected unsupported protocol error")
		}
	})

	t.Run("tcp parse addr error", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{
			Protocol:    settings.TCP,
			TCPSettings: mkTCPSettings("bad-port"),
		}}
		_, _, _, err := f.EstablishConnection(context.Background())
		if err == nil {
			t.Fatal("expected parse error")
		}
	})

	t.Run("udp parse addr error", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{
			Protocol:    settings.UDP,
			UDPSettings: mkUDPSettings("bad-port"),
		}}
		_, _, _, err := f.EstablishConnection(context.Background())
		if err == nil {
			t.Fatal("expected parse error")
		}
	})

	t.Run("ws empty host", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{
			Protocol:   settings.WS,
			WSSettings: mkWSSettings("", "", "8080", settings.WS),
		}}
		_, _, _, err := f.EstablishConnection(context.Background())
		if err == nil || !strings.Contains(err.Error(), "ws dial: empty host") {
			t.Fatalf("expected empty host error, got %v", err)
		}
	})

	t.Run("ws invalid port", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{
			Protocol:   settings.WS,
			WSSettings: mkWSSettings("127.0.0.1", "", "70000", settings.WS),
		}}
		_, _, _, err := f.EstablishConnection(context.Background())
		if err == nil || !strings.Contains(err.Error(), "ws dial: invalid port") {
			t.Fatalf("expected invalid ws port error, got %v", err)
		}
	})

	t.Run("wss empty host", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{
			Protocol:   settings.WSS,
			WSSettings: mkWSSettings("", "", "443", settings.WSS),
		}}
		_, _, _, err := f.EstablishConnection(context.Background())
		if err == nil || !strings.Contains(err.Error(), "wss dial: empty host") {
			t.Fatalf("expected empty wss host error, got %v", err)
		}
	})

	t.Run("wss invalid non-numeric port", func(t *testing.T) {
		f := &ConnectionFactory{conf: client.Configuration{
			Protocol:   settings.WSS,
			WSSettings: mkWSSettings("127.0.0.1", "", "bad", settings.WSS),
		}}
		_, _, _, err := f.EstablishConnection(context.Background())
		if err == nil {
			t.Fatal("expected wss port parse error")
		}
	})
}

func TestConnectionFactoryUnit_dial_ErrorBranches(t *testing.T) {
	f := &ConnectionFactory{}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	t.Run("dialTCP error", func(t *testing.T) {
		_, err := f.dialTCP(ctx, netip.MustParseAddrPort("127.0.0.1:1"))
		if err == nil {
			t.Fatal("expected dialTCP error")
		}
	})

	t.Run("dialUDP error", func(t *testing.T) {
		conn, err := f.dialUDP(ctx, netip.MustParseAddrPort("127.0.0.1:9"))
		// Environment-dependent: some sandboxes deny UDP connect, others allow it.
		if err == nil {
			if conn == nil {
				t.Fatal("expected non-nil conn when dialUDP succeeds")
			}
			_ = conn.Close()
		}
	})

	t.Run("dialWS error", func(t *testing.T) {
		_, err := f.dialWS(ctx, context.Background(), "ws", "127.0.0.1", "1")
		if err == nil {
			t.Fatal("expected dialWS error")
		}
	})
}

func TestConnectionFactoryUnit_establishSecuredConnection_Success(t *testing.T) {
	deriver := &primitives.DefaultKeyDeriver{}

	serverPub, serverPrivArr, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("server keygen failed: %v", err)
	}
	clientPub, clientPrivArr, err := deriver.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("client keygen failed: %v", err)
	}

	f := &ConnectionFactory{
		conf: client.Configuration{
			ClientPublicKey:  clientPub,
			ClientPrivateKey: clientPrivArr[:],
			X25519PublicKey:  serverPub,
		},
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	clientAdapter, err := adapters.NewLengthPrefixFramingAdapter(clientConn, framelimit.Cap(2048))
	if err != nil {
		t.Fatalf("client framing adapter failed: %v", err)
	}
	serverAdapter, err := adapters.NewLengthPrefixFramingAdapter(serverConn, framelimit.Cap(2048))
	if err != nil {
		t.Fatalf("server framing adapter failed: %v", err)
	}

	cookieManager, err := noise.NewCookieManager()
	if err != nil {
		t.Fatalf("cookie manager failed: %v", err)
	}
	allowedPeers := []serverCfg.AllowedPeer{
		{
			PublicKey: clientPub,
			Enabled:   true,
			ClientIP:  "10.44.0.2",
		},
	}
	serverHS := noise.NewIKHandshakeServer(
		serverPub,
		serverPrivArr[:],
		noise.NewAllowedPeersLookup(allowedPeers),
		cookieManager,
		noise.NewLoadMonitor(10000),
	)

	serverErrCh := make(chan error, 1)
	go func() {
		_, serr := serverHS.ServerSideHandshake(serverAdapter)
		serverErrCh <- serr
	}()

	adapter, crypto, _, err := f.establishSecuredConnection(
		context.Background(),
		settings.Settings{Protocol: settings.TCP},
		clientAdapter,
		chacha20.NewTcpSessionBuilder(chacha20.NewDefaultAEADBuilder()),
	)
	if err != nil {
		t.Fatalf("establishSecuredConnection failed: %v", err)
	}
	if adapter == nil || crypto == nil {
		t.Fatal("expected non-nil adapter and crypto")
	}
	if serr := <-serverErrCh; serr != nil {
		t.Fatalf("server handshake failed: %v", serr)
	}
}
