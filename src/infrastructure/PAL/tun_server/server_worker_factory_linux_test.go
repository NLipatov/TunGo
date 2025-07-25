package tun_server

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net"
	"testing"
	"tungo/infrastructure/PAL/configuration/server"

	"tungo/application"
	"tungo/infrastructure/settings"
)

// --- stub for ReadWriteCloser ---
type nopReadWriteCloser struct{}

func (nopReadWriteCloser) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (nopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopReadWriteCloser) Close() error                { return nil }

// --- fake application.Socket ---
type fakeSocket struct{ addr *net.UDPAddr }

func (f fakeSocket) StringAddr() string             { return f.addr.String() }
func (f fakeSocket) UdpAddr() (*net.UDPAddr, error) { return f.addr, nil } // Not used

// --- socketFactory mock ---
type ServerWorkerFactoryMockSocketFactory struct {
	Socket application.Socket
	Err    error
}

func (m *ServerWorkerFactoryMockSocketFactory) newSocket(_, _ string) (application.Socket, error) {
	return m.Socket, m.Err
}

// --- loggerFactory mock ---
type ServerWorkerFactoryMockLogger struct {
	Logs []string
}

func (l *ServerWorkerFactoryMockLogger) Printf(format string, _ ...any) {
	l.Logs = append(l.Logs, format)
}

type ServerWorkerFactoryMockLoggerFactory struct {
	Count    int
	LastInst *ServerWorkerFactoryMockLogger
}

func (f *ServerWorkerFactoryMockLoggerFactory) newLogger() application.Logger {
	f.Count++
	inst := &ServerWorkerFactoryMockLogger{}
	f.LastInst = inst
	return inst
}

// --- mock ServerConfigurationManager ---
type swflServerConfigurationManager struct{}

func (m *swflServerConfigurationManager) InjectSessionTtlIntervals(_, _ settings.HumanReadableDuration) error {
	return nil
}
func (m *swflServerConfigurationManager) Configuration() (*server.Configuration, error) {
	return &server.Configuration{}, nil
}
func (m *swflServerConfigurationManager) IncrementClientCounter() error {
	return nil
}
func (m *swflServerConfigurationManager) InjectEdKeys(_ ed25519.PublicKey, _ ed25519.PrivateKey) error {
	return nil
}

type swflServerConfigurationManagerWithErr struct{}

func (m *swflServerConfigurationManagerWithErr) InjectSessionTtlIntervals(_, _ settings.HumanReadableDuration) error {
	return nil
}
func (m *swflServerConfigurationManagerWithErr) Configuration() (*server.Configuration, error) {
	return nil, errors.New("config fail")
}
func (m *swflServerConfigurationManagerWithErr) IncrementClientCounter() error { return nil }
func (m *swflServerConfigurationManagerWithErr) InjectEdKeys(_ ed25519.PublicKey, _ ed25519.PrivateKey) error {
	return nil
}

// --- TESTS ---

func TestCreateWorker_UnsupportedProtocol(t *testing.T) {
	s := settings.Settings{Protocol: 42}
	cfgMgr := &swflServerConfigurationManager{}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	sockF := &ServerWorkerFactoryMockSocketFactory{}
	factory := NewTestServerWorkerFactory(s, sockF, logF, cfgMgr)
	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil {
		t.Fatal("expected unsupported-protocol error")
	}
}

func TestCreateWorker_TCP_SocketError(t *testing.T) {
	s := settings.Settings{
		Protocol:     settings.TCP,
		ConnectionIP: "1.2.3.4",
		Port:         "9999",
	}
	sockF := &ServerWorkerFactoryMockSocketFactory{Err: errors.New("bad socket")}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	cfgMgr := &swflServerConfigurationManager{}
	factory := NewTestServerWorkerFactory(s, sockF, logF, cfgMgr)

	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil || err.Error() != "bad socket" {
		t.Fatalf("expected socket error, got %v", err)
	}
}

func TestCreateWorker_TCP_ConfigError(t *testing.T) {
	s := settings.Settings{
		Protocol:     settings.TCP,
		ConnectionIP: "9.9.9.9",
		Port:         "8080",
	}
	udpAddr, udpAddrErr := net.ResolveUDPAddr("udp", "9.9.9.9:8080")
	if udpAddrErr != nil {
		t.Fatal("failed to parse address")
	}
	sockF := &ServerWorkerFactoryMockSocketFactory{Socket: fakeSocket{udpAddr}}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	cfgMgr := &swflServerConfigurationManagerWithErr{}
	factory := NewTestServerWorkerFactory(s, sockF, logF, cfgMgr)

	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil || err.Error() != "config fail" {
		t.Fatalf("expected config error, got %v", err)
	}
}

func TestCreateWorker_TCP_Success(t *testing.T) {
	s := settings.Settings{
		Protocol:     settings.TCP,
		ConnectionIP: "127.0.0.1",
		Port:         "0",
	}
	udpAddr, udpAddrErr := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if udpAddrErr != nil {
		t.Fatal("failed to parse address")
	}
	sockF := &ServerWorkerFactoryMockSocketFactory{Socket: fakeSocket{udpAddr}}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	cfgMgr := &swflServerConfigurationManager{}
	factory := NewTestServerWorkerFactory(s, sockF, logF, cfgMgr)

	w, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logF.Count != 1 {
		t.Errorf("expected 1 logger, got %d", logF.Count)
	}
	// worker.HandleTun обычно вернёт io.EOF из nopReadWriteCloser
	if err := w.HandleTun(); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestCreateWorker_UDP_SocketError(t *testing.T) {
	s := settings.Settings{
		Protocol:     settings.UDP,
		ConnectionIP: "1.2.3.4",
		Port:         "5678",
	}
	sockF := &ServerWorkerFactoryMockSocketFactory{Err: errors.New("udp socket fail")}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	cfgMgr := &swflServerConfigurationManager{}
	factory := NewTestServerWorkerFactory(s, sockF, logF, cfgMgr)

	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil || err.Error() != "udp socket fail" {
		t.Fatalf("expected udp socket error, got %v", err)
	}
}

func TestCreateWorker_UDP_ConfigError(t *testing.T) {
	s := settings.Settings{
		Protocol:     settings.UDP,
		ConnectionIP: "10.20.30.40",
		Port:         "10000",
	}
	udpAddr, udpAddrErr := net.ResolveUDPAddr("udp", "10.20.30.40:10000")
	if udpAddrErr != nil {
		t.Fatal("failed to parse address")
	}
	sockF := &ServerWorkerFactoryMockSocketFactory{Socket: fakeSocket{udpAddr}}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	cfgMgr := &swflServerConfigurationManagerWithErr{}
	factory := NewTestServerWorkerFactory(s, sockF, logF, cfgMgr)

	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil || err.Error() != "config fail" {
		t.Fatalf("expected config error, got %v", err)
	}
}

func TestCreateWorker_UDP_Success(t *testing.T) {
	s := settings.Settings{
		Protocol:     settings.UDP,
		ConnectionIP: "127.0.0.1",
		Port:         "0",
	}
	udpAddr, udpAddrErr := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if udpAddrErr != nil {
		t.Fatal("failed to parse address")
	}
	sockF := &ServerWorkerFactoryMockSocketFactory{Socket: fakeSocket{udpAddr}}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	cfgMgr := &swflServerConfigurationManager{}
	factory := NewTestServerWorkerFactory(s, sockF, logF, cfgMgr)

	w, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logF.Count != 1 {
		t.Errorf("expected 1 logger, got %d", logF.Count)
	}
	if err := w.HandleTun(); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}
