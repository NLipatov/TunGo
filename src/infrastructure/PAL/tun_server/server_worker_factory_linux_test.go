package tun_server

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net"
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
func (d *dummyConfigManager) InjectSessionTtlIntervals(_, _ settings.HumanReadableDuration) error {
	return nil
}
func (d *dummyConfigManager) IncrementClientCounter() error { return nil }
func (d *dummyConfigManager) InjectEdKeys(_ ed25519.PublicKey, _ ed25519.PrivateKey) error {
	return nil
}

// Erroring ServerConfigurationManager to trigger config error paths.
type errorConfigManager struct{}

func (e *errorConfigManager) Configuration() (*serverCfg.Configuration, error) {
	return nil, errors.New("config error")
}
func (e *errorConfigManager) InjectSessionTtlIntervals(_, _ settings.HumanReadableDuration) error {
	return nil
}
func (e *errorConfigManager) IncrementClientCounter() error { return nil }
func (e *errorConfigManager) InjectEdKeys(_ ed25519.PublicKey, _ ed25519.PrivateKey) error {
	return nil
}

// Nop TUN handle.
type nopReadWriteCloser struct{}

func (nopReadWriteCloser) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (nopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopReadWriteCloser) Close() error                { return nil }

// ------------------- tests -------------------

func Test_addrPortToListen_ErrorsAndDualStackDefault(t *testing.T) {
	f := &ServerWorkerFactory{}

	// invalid port string
	if _, err := f.addrPortToListen("127.0.0.1", "notaport"); err == nil {
		t.Fatal("expected error for invalid port string")
	}

	// default dual-stack when ip is empty
	addr, err := f.addrPortToListen("", "1234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := addr.Addr().String(); got != "::" {
		t.Errorf("expected ::, got %q", got)
	}
}

func Test_addrPortToListen_InvalidIP(t *testing.T) {
	f := &ServerWorkerFactory{}
	if _, err := f.addrPortToListen("invalid_ip", "1234"); err == nil {
		t.Error("expected error for invalid IP")
	}
}

func Test_addrPortToListen_InvalidPortNumber(t *testing.T) {
	f := &ServerWorkerFactory{}
	if _, err := f.addrPortToListen("127.0.0.1", "99999"); err == nil { // >65535
		t.Error("expected error for invalid port number")
	}
}

func Test_CreateWorker_UnsupportedProtocol(t *testing.T) {
	factory := NewTestServerWorkerFactory(newDefaultLoggerFactory(), &dummyConfigManager{})

	ws := settings.Settings{Protocol: settings.UNKNOWN} // unknown enum value
	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{}, ws)
	if err == nil || err.Error() != "protocol UNKNOWN not supported" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_CreateWorker_TCP_ConfigError(t *testing.T) {
	factory := NewTestServerWorkerFactory(newDefaultLoggerFactory(), &errorConfigManager{})
	ws := settings.Settings{Protocol: settings.TCP, ConnectionIP: "127.0.0.1", Port: "0"}

	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{}, ws)
	if err == nil || err.Error() != "config error" {
		t.Fatalf("expected config error, got %v", err)
	}
}

func Test_CreateWorker_UDP_ConfigError(t *testing.T) {
	factory := NewTestServerWorkerFactory(newDefaultLoggerFactory(), &errorConfigManager{})
	ws := settings.Settings{Protocol: settings.UDP, ConnectionIP: "127.0.0.1", Port: "0"}

	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{}, ws)
	if err == nil || err.Error() != "config error" {
		t.Fatalf("expected config error, got %v", err)
	}
}

func Test_CreateWorker_WS_ConfigError(t *testing.T) {
	factory := NewTestServerWorkerFactory(newDefaultLoggerFactory(), &errorConfigManager{})
	ws := settings.Settings{Protocol: settings.WS, ConnectionIP: "127.0.0.1", Port: "0"}

	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{}, ws)
	if err == nil || err.Error() != "config error" {
		t.Fatalf("expected config error, got %v", err)
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

	factory := NewTestServerWorkerFactory(newDefaultLoggerFactory(), &dummyConfigManager{})
	ws := settings.Settings{Protocol: settings.TCP, ConnectionIP: "127.0.0.1", Port: port}

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

	factory := NewTestServerWorkerFactory(newDefaultLoggerFactory(), &dummyConfigManager{})
	ws := settings.Settings{Protocol: settings.UDP, ConnectionIP: "127.0.0.1", Port: port}

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

	factory := NewTestServerWorkerFactory(newDefaultLoggerFactory(), &dummyConfigManager{})
	ws := settings.Settings{Protocol: settings.WS, ConnectionIP: "127.0.0.1", Port: port}

	if _, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{}, ws); err == nil {
		t.Fatal("expected listen error due to port in use")
	}
}

func Test_CreateWorker_TCP_UDP_WS_Success(t *testing.T) {
	for _, proto := range []settings.Protocol{settings.TCP, settings.UDP, settings.WS} {
		ctx, cancel := context.WithCancel(context.Background())
		factory := NewTestServerWorkerFactory(newDefaultLoggerFactory(), &dummyConfigManager{})
		ws := settings.Settings{Protocol: proto, ConnectionIP: "127.0.0.1", Port: "0"}
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

func Test_NewServerWorkerFactory_Coverage(t *testing.T) {
	dcm := &dummyConfigManager{}
	// Production constructor
	if f := NewServerWorkerFactory(dcm); f == nil {
		t.Error("nil factory (prod)")
	}
	// Test constructor
	if f := NewTestServerWorkerFactory(newDefaultLoggerFactory(), dcm); f == nil {
		t.Error("nil factory (test)")
	}
}
