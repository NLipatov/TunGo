package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	serverconfig "tungo/internal/config/server"
	"tungo/internal/config/settings"
	"tungo/internal/platform"
	"tungo/internal/protocol/noise"
)

var errServerLifecycleTest = errors.New("test error")

// ------------------- test doubles -------------------

// Nop TUN handle.
type nopReadWriteCloser struct{}

func (nopReadWriteCloser) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (nopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopReadWriteCloser) Close() error                { return nil }

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

// ------------------- helpers -------------------

func newTestRuntime(t *testing.T) (*Server, error) {
	t.Helper()
	conf := &serverconfig.Configuration{}
	cookieManager, err := noise.NewCookieManager()
	if err != nil {
		return nil, err
	}
	return &Server{
		configuration: conf,
		allowedPeers:  newAllowedPeers(nil),
		cookieManager: cookieManager,
		loadMonitor:   noise.NewLoadMonitor(noise.DefaultLoadThreshold),
	}, nil
}

// ------------------- tests -------------------

func TestNewServer(t *testing.T) {
	if !platform.ServerModeSupported() {
		if server, err := New(nil); err == nil || server != nil {
			t.Fatalf("New(nil) = %v, %v; want unsupported-platform error", server, err)
		}
		return
	}

	if server, err := New(nil); err == nil || server != nil {
		t.Fatalf("New(nil) = %v, %v; want nil-configuration error", server, err)
	}
	file := serverconfig.NewFile(filepath.Join(t.TempDir(), "server_configuration.json"))
	server, err := New(file)
	if err != nil {
		t.Fatal(err)
	}
	if server.configFile != file || server.configuration == nil || server.tunManager == nil {
		t.Fatalf("incomplete server: %+v", server)
	}
}

func Test_addrPortToListen_ErrorsAndDualStackDefault(t *testing.T) {
	f := &Server{}

	// invalid port string
	if _, err := f.addrPortToListen(mustHost("127.0.0.1"), 0); err == nil {
		t.Fatal("expected error for invalid port")
	}

	// default listen address when host is empty: "::" on dual-stack, "0.0.0.0" on IPv4-only
	addr, err := f.addrPortToListen(mustHost(""), 1234)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := addr.Addr().String(); got != "::" && got != "0.0.0.0" {
		t.Errorf("expected :: or 0.0.0.0, got %q", got)
	}
}

func Test_addrPortToListen_DomainHostNotAllowed(t *testing.T) {
	f := &Server{}
	if _, err := f.addrPortToListen(mustHost("example.org"), 1234); err == nil {
		t.Error("expected error for non-IP host")
	}
}

func Test_addrPortToListen_InvalidPortNumber(t *testing.T) {
	f := &Server{}
	if _, err := f.addrPortToListen(mustHost("127.0.0.1"), 99999); err == nil { // >65535
		t.Error("expected error for invalid port number")
	}
}

func TestNewTunnel_UnsupportedProtocol(t *testing.T) {
	factory, err := newTestRuntime(t)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	ws := settings.Settings{Protocol: settings.UNKNOWN} // unknown enum value
	_, err = factory.newTunnel(context.Background(), nopReadWriteCloser{}, ws)
	if err == nil || err.Error() != "protocol UNKNOWN not supported" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewTunnel_TCP_ListenError(t *testing.T) {
	// Occupy a TCP port to force EADDRINUSE.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(l net.Listener) {
		_ = l.Close()
	}(l)
	_, port, _ := net.SplitHostPort(l.Addr().String())
	portNum, convErr := strconv.Atoi(port)
	if convErr != nil {
		t.Fatalf("failed to parse port %q: %v", port, convErr)
	}

	factory, err := newTestRuntime(t)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ws := settings.Settings{
		Protocol: settings.TCP,
		Network: settings.Network{
			Server: mustHost("127.0.0.1"),
			Port:   portNum,
		},
	}

	if _, err := factory.newTunnel(context.Background(), nopReadWriteCloser{}, ws); err == nil {
		t.Fatal("expected listen error due to port in use")
	}
}

func TestNewTunnel_UDP_ListenError(t *testing.T) {
	// Occupy a UDP port to force EADDRINUSE.
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func(l *net.UDPConn) {
		_ = l.Close()
	}(l)
	_, port, _ := net.SplitHostPort(l.LocalAddr().String())
	portNum, convErr := strconv.Atoi(port)
	if convErr != nil {
		t.Fatalf("failed to parse port %q: %v", port, convErr)
	}

	factory, err := newTestRuntime(t)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ws := settings.Settings{
		Protocol: settings.UDP,
		Network: settings.Network{
			Server: mustHost("127.0.0.1"),
			Port:   portNum,
		},
	}

	if _, err := factory.newTunnel(context.Background(), nopReadWriteCloser{}, ws); err == nil {
		t.Fatal("expected listen error due to port in use")
	}
}

func TestNewTunnel_WS_ListenError(t *testing.T) {
	// WS listener uses a TCP port; occupy it to force an error.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(l net.Listener) {
		_ = l.Close()
	}(l)
	_, port, _ := net.SplitHostPort(l.Addr().String())
	portNum, convErr := strconv.Atoi(port)
	if convErr != nil {
		t.Fatalf("failed to parse port %q: %v", port, convErr)
	}

	factory, err := newTestRuntime(t)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ws := settings.Settings{
		Protocol: settings.WS,
		Network: settings.Network{
			Server: mustHost("127.0.0.1"),
			Port:   portNum,
		},
	}

	if _, err := factory.newTunnel(context.Background(), nopReadWriteCloser{}, ws); err == nil {
		t.Fatal("expected listen error due to port in use")
	}
}

func TestNewTunnel_WS_ListenerInitError_ClosesTCPListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	portNum, convErr := strconv.Atoi(port)
	if convErr != nil {
		_ = ln.Close()
		t.Fatalf("failed to parse port %q: %v", port, convErr)
	}
	_ = ln.Close()

	factory, err := newTestRuntime(t)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ws := settings.Settings{
		Protocol: settings.WS,
		Network: settings.Network{
			Server: mustHost("127.0.0.1"),
			Port:   portNum,
		},
	}

	// nil ctx makes ws listener creation fail; underlying TCP listener must be closed.
	//nolint:staticcheck // This test verifies nil-context cleanup.
	if _, err := factory.newTunnel(nil, nopReadWriteCloser{}, ws); err == nil {
		t.Fatal("expected ws listener init error for nil context")
	}

	reopen, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		t.Fatalf("expected port to be free after ws listener init failure, got: %v", err)
	}
	_ = reopen.Close()
}

func TestNewTunnel_TCP_UDP_WS_Success(t *testing.T) {
	for _, proto := range []settings.Protocol{settings.TCP, settings.UDP, settings.WS} {
		ctx, cancel := context.WithCancel(context.Background())
		factory, err := newTestRuntime(t)
		if err != nil {
			t.Fatalf("unexpected constructor error for %s: %v", proto, err)
		}

		var portNum int
		switch proto {
		case settings.UDP:
			addr, resolveErr := net.ResolveUDPAddr("udp", "127.0.0.1:0")
			if resolveErr != nil {
				t.Fatalf("resolve udp addr failed for %s: %v", proto, resolveErr)
			}
			conn, listenErr := net.ListenUDP("udp", addr)
			if listenErr != nil {
				t.Fatalf("listen udp failed for %s: %v", proto, listenErr)
			}
			_, port, _ := net.SplitHostPort(conn.LocalAddr().String())
			_ = conn.Close()
			portNum, err = strconv.Atoi(port)
			if err != nil {
				t.Fatalf("failed to parse port %q for %s: %v", port, proto, err)
			}
		default:
			ln, listenErr := net.Listen("tcp", "127.0.0.1:0")
			if listenErr != nil {
				t.Fatalf("listen tcp failed for %s: %v", proto, listenErr)
			}
			_, port, _ := net.SplitHostPort(ln.Addr().String())
			_ = ln.Close()
			portNum, err = strconv.Atoi(port)
			if err != nil {
				t.Fatalf("failed to parse port %q for %s: %v", port, proto, err)
			}
		}

		ws := settings.Settings{
			Protocol: proto,
			Network: settings.Network{
				Server: mustHost("127.0.0.1"),
				Port:   portNum,
			},
		}
		w, err := factory.newTunnel(ctx, nopReadWriteCloser{}, ws)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", proto, err)
		}
		if w == nil {
			t.Fatalf("expected tunnel runner for %s, got nil", proto)
		}
		cancel()
	}
}

func TestRuntimeRevocationAndAllowedPeersUpdate(t *testing.T) {
	runtime, err := newTestRuntime(t)
	if err != nil {
		t.Fatalf("unexpected runtime error: %v", err)
	}

	if revoked := runtime.RevokeByPubKey(nil); revoked != 0 {
		t.Fatalf("RevokeByPubKey() = %d, want 0", revoked)
	}

	runtime.Update(nil)
}

type serverLifecycleTun struct {
	closed int32
}

func (*serverLifecycleTun) Read([]byte) (int, error)         { return 0, io.EOF }
func (*serverLifecycleTun) Write(packet []byte) (int, error) { return len(packet), nil }
func (t *serverLifecycleTun) Close() error {
	atomic.AddInt32(&t.closed, 1)
	return nil
}

type serverLifecycleTunManager struct {
	createErr    error
	disposeErr   error
	disposeCalls int32
	device       *serverLifecycleTun
}

func (m *serverLifecycleTunManager) OpenTunnel(settings.Settings) (io.ReadWriteCloser, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.device = &serverLifecycleTun{}
	return m.device, nil
}

func (m *serverLifecycleTunManager) CloseTunnel(settings.Settings) error {
	atomic.AddInt32(&m.disposeCalls, 1)
	return m.disposeErr
}

func TestServerRunOwnsCleanupAndReadiness(t *testing.T) {
	manager := &serverLifecycleTunManager{}
	server := &Server{configuration: &serverconfig.Configuration{}, tunManager: manager}

	if err := server.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !server.Ready() {
		t.Fatal("server did not become ready")
	}
	if calls := atomic.LoadInt32(&manager.disposeCalls); calls != 6 {
		t.Fatalf("CloseTunnel() calls = %d, want 6", calls)
	}
}

func TestServerCreateTunnelReturnsTunError(t *testing.T) {
	manager := &serverLifecycleTunManager{createErr: errServerLifecycleTest}
	server := &Server{tunManager: manager}

	_, _, err := server.createTunnel(context.Background(), settings.Settings{Protocol: settings.TCP})
	if !errors.Is(err, errServerLifecycleTest) {
		t.Fatalf("createTunnel() error = %v, want %v", err, errServerLifecycleTest)
	}
}

func TestServerCreateTunnelClosesTunAfterProtocolError(t *testing.T) {
	manager := &serverLifecycleTunManager{}
	server := &Server{tunManager: manager}

	_, _, err := server.createTunnel(context.Background(), settings.Settings{Protocol: settings.UNKNOWN})
	if err == nil {
		t.Fatal("createTunnel() error = nil, want unsupported protocol error")
	}
	if calls := atomic.LoadInt32(&manager.device.closed); calls != 1 {
		t.Fatalf("TUN Close() calls = %d, want 1", calls)
	}
}
