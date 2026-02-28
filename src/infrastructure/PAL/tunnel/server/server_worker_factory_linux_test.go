package server

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"

	serverCfg "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

// ------------------- test doubles -------------------

// Dummy ServerConfigurationManager that returns a valid config.
type dummyConfigManager struct{}

func (d *dummyConfigManager) Configuration() (*serverCfg.Configuration, error) {
	return &serverCfg.Configuration{}, nil
}
func (d *dummyConfigManager) AddAllowedPeer(_ serverCfg.AllowedPeer) error {
	return nil
}
func (d *dummyConfigManager) ListAllowedPeers() ([]serverCfg.AllowedPeer, error) { return nil, nil }
func (d *dummyConfigManager) SetAllowedPeerEnabled(_ int, _ bool) error          { return nil }
func (d *dummyConfigManager) RemoveAllowedPeer(_ int) error                      { return nil }
func (d *dummyConfigManager) IncrementClientCounter() error                      { return nil }
func (d *dummyConfigManager) InjectX25519Keys(_, _ []byte) error {
	return nil
}
func (d *dummyConfigManager) EnsureIPv6Subnets() error { return nil }
func (d *dummyConfigManager) InvalidateCache()         {}

// Erroring ServerConfigurationManager to trigger config error paths.
type errorConfigManager struct{}

func (e *errorConfigManager) Configuration() (*serverCfg.Configuration, error) {
	return nil, errors.New("config error")
}
func (e *errorConfigManager) AddAllowedPeer(_ serverCfg.AllowedPeer) error {
	return nil
}
func (e *errorConfigManager) ListAllowedPeers() ([]serverCfg.AllowedPeer, error) { return nil, nil }
func (e *errorConfigManager) SetAllowedPeerEnabled(_ int, _ bool) error          { return nil }
func (e *errorConfigManager) RemoveAllowedPeer(_ int) error                      { return nil }
func (e *errorConfigManager) IncrementClientCounter() error                      { return nil }
func (e *errorConfigManager) InjectX25519Keys(_, _ []byte) error {
	return nil
}
func (e *errorConfigManager) EnsureIPv6Subnets() error { return nil }
func (e *errorConfigManager) InvalidateCache()         {}

// Nop TUN handle.
type nopReadWriteCloser struct{}

func (nopReadWriteCloser) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (nopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopReadWriteCloser) Close() error                { return nil }

func mustHost(raw string) settings.Host {
	h, err := settings.NewHost(raw)
	if err != nil {
		panic(err)
	}
	return h
}

// ------------------- helpers -------------------

func newTestWorkerFactory(t *testing.T) (*WorkerFactory, error) {
	t.Helper()
	runtime, err := NewRuntime(&dummyConfigManager{})
	if err != nil {
		return nil, err
	}
	return NewTestWorkerFactory(newDefaultLoggerFactory(), runtime, &dummyConfigManager{})
}

func newTestWorkerFactoryWithErrorConfig() (*WorkerFactory, error) {
	runtime, err := NewRuntime(&errorConfigManager{})
	if err != nil {
		return nil, err
	}
	return NewTestWorkerFactory(newDefaultLoggerFactory(), runtime, &errorConfigManager{})
}

// ------------------- tests -------------------

func Test_addrPortToListen_ErrorsAndDualStackDefault(t *testing.T) {
	f := &WorkerFactory{}

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
	f := &WorkerFactory{}
	if _, err := f.addrPortToListen(mustHost("example.org"), 1234); err == nil {
		t.Error("expected error for non-IP host")
	}
}

func Test_addrPortToListen_InvalidPortNumber(t *testing.T) {
	f := &WorkerFactory{}
	if _, err := f.addrPortToListen(mustHost("127.0.0.1"), 99999); err == nil { // >65535
		t.Error("expected error for invalid port number")
	}
}

func Test_CreateWorker_UnsupportedProtocol(t *testing.T) {
	factory, err := newTestWorkerFactory(t)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	ws := settings.Settings{Protocol: settings.UNKNOWN} // unknown enum value
	_, err = factory.CreateWorker(context.Background(), nopReadWriteCloser{}, ws)
	if err == nil || err.Error() != "protocol UNKNOWN not supported" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_CreateWorker_TCP_ConfigError(t *testing.T) {
	factory, err := newTestWorkerFactoryWithErrorConfig()
	if err == nil {
		t.Fatal("expected constructor error")
	}

	if factory != nil {
		t.Fatal("expected nil factory on constructor error")
	}
}

func Test_CreateWorker_UDP_ConfigError(t *testing.T) {
	factory, err := newTestWorkerFactoryWithErrorConfig()
	if err == nil {
		t.Fatal("expected constructor error")
	}

	if factory != nil {
		t.Fatal("expected nil factory on constructor error")
	}
}

func Test_CreateWorker_WS_ConfigError(t *testing.T) {
	factory, err := newTestWorkerFactoryWithErrorConfig()
	if err == nil {
		t.Fatal("expected constructor error")
	}

	if factory != nil {
		t.Fatal("expected nil factory on constructor error")
	}
}

func Test_CreateWorker_TCP_ListenError(t *testing.T) {
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

	factory, err := newTestWorkerFactory(t)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ws := settings.Settings{
		Protocol: settings.TCP,
		Addressing: settings.Addressing{
			Server: mustHost("127.0.0.1"),
			Port:   portNum,
		},
	}

	if _, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{}, ws); err == nil {
		t.Fatal("expected listen error due to port in use")
	}
}

func Test_CreateWorker_UDP_ListenError(t *testing.T) {
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

	factory, err := newTestWorkerFactory(t)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ws := settings.Settings{
		Protocol: settings.UDP,
		Addressing: settings.Addressing{
			Server: mustHost("127.0.0.1"),
			Port:   portNum,
		},
	}

	if _, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{}, ws); err == nil {
		t.Fatal("expected listen error due to port in use")
	}
}

func Test_CreateWorker_WS_ListenError(t *testing.T) {
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

	factory, err := newTestWorkerFactory(t)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ws := settings.Settings{
		Protocol: settings.WS,
		Addressing: settings.Addressing{
			Server: mustHost("127.0.0.1"),
			Port:   portNum,
		},
	}

	if _, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{}, ws); err == nil {
		t.Fatal("expected listen error due to port in use")
	}
}

func Test_CreateWorker_WS_ListenerInitError_ClosesTCPListener(t *testing.T) {
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

	factory, err := newTestWorkerFactory(t)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	ws := settings.Settings{
		Protocol: settings.WS,
		Addressing: settings.Addressing{
			Server: mustHost("127.0.0.1"),
			Port:   portNum,
		},
	}

	// nil ctx makes ws listener creation fail; underlying TCP listener must be closed.
	if _, err := factory.CreateWorker(nil, nopReadWriteCloser{}, ws); err == nil {
		t.Fatal("expected ws listener init error for nil context")
	}

	reopen, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		t.Fatalf("expected port to be free after ws listener init failure, got: %v", err)
	}
	_ = reopen.Close()
}

func Test_CreateWorker_TCP_UDP_WS_Success(t *testing.T) {
	for _, proto := range []settings.Protocol{settings.TCP, settings.UDP, settings.WS} {
		ctx, cancel := context.WithCancel(context.Background())
		factory, err := newTestWorkerFactory(t)
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
			Addressing: settings.Addressing{
				Server: mustHost("127.0.0.1"),
				Port:   portNum,
			},
		}
		w, err := factory.CreateWorker(ctx, nopReadWriteCloser{}, ws)
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", proto, err)
		}
		if w == nil {
			t.Fatalf("expected worker for %s, got nil", proto)
		}
		cancel()
	}
}

func Test_NewWorkerFactory_Coverage(t *testing.T) {
	dcm := &dummyConfigManager{}
	runtime, err := NewRuntime(dcm)
	if err != nil {
		t.Fatalf("unexpected runtime error: %v", err)
	}
	// Production constructor
	if f, err := NewWorkerFactory(runtime, dcm); f == nil || err != nil {
		t.Errorf("nil/error factory (prod): factory=%v err=%v", f, err)
	}
	// Test constructor
	if f, err := NewTestWorkerFactory(newDefaultLoggerFactory(), runtime, dcm); f == nil || err != nil {
		t.Errorf("nil/error factory (test): factory=%v err=%v", f, err)
	}
}

func Test_Runtime_SessionRevokerAndAllowedPeersUpdater(t *testing.T) {
	runtime, err := NewRuntime(&dummyConfigManager{})
	if err != nil {
		t.Fatalf("unexpected runtime error: %v", err)
	}

	revoker := runtime.SessionRevoker()
	if revoker == nil {
		t.Fatal("expected non-nil session revoker")
	}

	updater := runtime.AllowedPeersUpdater()
	if updater == nil {
		t.Fatal("expected non-nil allowed peers updater")
	}

	updater.Update(nil)
}
