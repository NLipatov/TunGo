package tun_server

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net"
	"testing"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

// --- Дамми конфиг-менеджеры для разных веток ---
type dummyConfigManager struct{}

func (d *dummyConfigManager) Configuration() (*server.Configuration, error) {
	return &server.Configuration{}, nil
}
func (d *dummyConfigManager) InjectSessionTtlIntervals(_, _ settings.HumanReadableDuration) error {
	return nil
}
func (d *dummyConfigManager) IncrementClientCounter() error { return nil }
func (d *dummyConfigManager) InjectEdKeys(_ ed25519.PublicKey, _ ed25519.PrivateKey) error {
	return nil
}

type errorConfigManager struct{}

func (e *errorConfigManager) Configuration() (*server.Configuration, error) {
	return nil, errors.New("config error")
}
func (e *errorConfigManager) InjectSessionTtlIntervals(_, _ settings.HumanReadableDuration) error {
	return nil
}
func (e *errorConfigManager) IncrementClientCounter() error { return nil }
func (e *errorConfigManager) InjectEdKeys(_ ed25519.PublicKey, _ ed25519.PrivateKey) error {
	return nil
}

// --- Ноп-ридер для TUN ---
type nopReadWriteCloser struct{}

func (nopReadWriteCloser) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (nopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopReadWriteCloser) Close() error                { return nil }

// --- Юнит-тесты фабрики ---
func Test_ServerWorkerFactory_addrPortToListen_Errors(t *testing.T) {
	f := &ServerWorkerFactory{}

	// Некорректный порт
	_, err := f.addrPortToListen("127.0.0.1", "notaport")
	if err == nil {
		t.Fatal("expected error for invalid port")
	}

	// Пустой IP -> dualstack (::)
	addr, err := f.addrPortToListen("", "1234")
	if err != nil {
		t.Fatal(err)
	}
	if addr.Addr().String() != "::" {
		t.Errorf("expected ::, got %v", addr.Addr())
	}
}

func Test_ServerWorkerFactory_CreateWorker_UnsupportedProtocol(t *testing.T) {
	factory := &ServerWorkerFactory{
		settings:             settings.Settings{Protocol: 42},
		configurationManager: &dummyConfigManager{},
		loggerFactory:        newDefaultLoggerFactory(),
	}
	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil || err.Error() != "protocol 42 not supported" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_ServerWorkerFactory_CreateWorker_TCP_ConfigError(t *testing.T) {
	factory := &ServerWorkerFactory{
		settings:             settings.Settings{Protocol: settings.TCP, ConnectionIP: "127.0.0.1", Port: "0"},
		configurationManager: &errorConfigManager{},
		loggerFactory:        newDefaultLoggerFactory(),
	}
	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil || err.Error() != "config error" {
		t.Fatalf("expected config error, got %v", err)
	}
}

func Test_ServerWorkerFactory_CreateWorker_UDP_ConfigError(t *testing.T) {
	factory := &ServerWorkerFactory{
		settings:             settings.Settings{Protocol: settings.UDP, ConnectionIP: "127.0.0.1", Port: "0"},
		configurationManager: &errorConfigManager{},
		loggerFactory:        newDefaultLoggerFactory(),
	}
	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil || err.Error() != "config error" {
		t.Fatalf("expected config error, got %v", err)
	}
}

func Test_ServerWorkerFactory_CreateWorker_TCP_ListenError(t *testing.T) {
	// Открываем TCP-порт, чтобы занять его и вызвать ошибку в фабрике
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func(l net.Listener) {
		_ = l.Close()
	}(l)
	_, port, _ := net.SplitHostPort(l.Addr().String())

	factory := &ServerWorkerFactory{
		settings:             settings.Settings{Protocol: settings.TCP, ConnectionIP: "127.0.0.1", Port: port},
		configurationManager: &dummyConfigManager{},
		loggerFactory:        newDefaultLoggerFactory(),
	}
	_, err = factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil {
		t.Fatal("expected listen error due to port in use")
	}
}

func Test_ServerWorkerFactory_CreateWorker_UDP_ListenError(t *testing.T) {
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

	factory := &ServerWorkerFactory{
		settings:             settings.Settings{Protocol: settings.UDP, ConnectionIP: "127.0.0.1", Port: port},
		configurationManager: &dummyConfigManager{},
		loggerFactory:        newDefaultLoggerFactory(),
	}
	_, err = factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil {
		t.Fatal("expected listen error due to port in use")
	}
}

func Test_ServerWorkerFactory_CreateWorker_TCP_UDP_Success(t *testing.T) {
	for _, proto := range []int{settings.TCP, settings.UDP} {
		factory := &ServerWorkerFactory{
			settings:             settings.Settings{Protocol: settings.Protocol(proto), ConnectionIP: "127.0.0.1", Port: "0"},
			configurationManager: &dummyConfigManager{},
			loggerFactory:        newDefaultLoggerFactory(),
		}
		w, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
		if err != nil {
			t.Fatalf("unexpected error (%d): %v", proto, err)
		}
		if w == nil {
			t.Fatalf("expected worker, got nil")
		}
		if err := w.HandleTun(); err != io.EOF {
			t.Errorf("expected EOF, got %v", err)
		}
	}
}

func Test_NewServerWorkerFactory_Coverage(t *testing.T) {
	dcm := &dummyConfigManager{}
	f1 := NewServerWorkerFactory(settings.Settings{}, dcm)
	if f1 == nil {
		t.Error("nil factory")
	}
	f2 := NewTestServerWorkerFactory(settings.Settings{}, newDefaultLoggerFactory(), dcm)
	if f2 == nil {
		t.Error("nil factory (test)")
	}
}

func Test_ServerWorkerFactory_addrPortToListen_InvalidIP(t *testing.T) {
	f := &ServerWorkerFactory{}
	_, err := f.addrPortToListen("invalid_ip", "1234")
	if err == nil {
		t.Error("expected error for invalid IP")
	}
}

func Test_ServerWorkerFactory_addrPortToListen_InvalidPort(t *testing.T) {
	f := &ServerWorkerFactory{}
	_, err := f.addrPortToListen("127.0.0.1", "99999") // > 65535
	if err == nil {
		t.Error("expected error for invalid port number")
	}
}
