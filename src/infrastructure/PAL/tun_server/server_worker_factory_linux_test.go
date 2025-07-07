package tun_server

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"tungo/infrastructure/PAL/server_configuration"

	"tungo/application"
	"tungo/infrastructure/settings"
)

// --- stub for ReadWriteCloser ---
type nopReadWriteCloser struct{}

func (nopReadWriteCloser) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (nopReadWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopReadWriteCloser) Close() error                { return nil }

// --- fake application.Socket ---
type fakeSocket struct{ addr string }

func (f fakeSocket) StringAddr() string { return f.addr }
func (f fakeSocket) UdpAddr() (*net.UDPAddr, error) {
	return net.ResolveUDPAddr("udp", f.addr)
}

// --- socketFactory mock ---
type ServerWorkerFactoryMockSocketFactory struct {
	Socket application.Socket
	Err    error
}

func (m *ServerWorkerFactoryMockSocketFactory) newSocket(_, _ string) (application.Socket, error) {
	return m.Socket, m.Err
}

// --- tcpListenerFactory mock ---
type fakeTCPListener struct{}

func (*fakeTCPListener) Accept() (net.Conn, error) { return nil, io.EOF }
func (*fakeTCPListener) Close() error              { return nil }
func (*fakeTCPListener) Addr() net.Addr            { return &net.TCPAddr{} }

type ServerWorkerFactoryMockTcpListenerFactory struct {
	AddrCalled string
	Err        error
}

func (m *ServerWorkerFactoryMockTcpListenerFactory) listenTCP(addr string) (net.Listener, error) {
	m.AddrCalled = addr
	if m.Err != nil {
		return nil, m.Err
	}
	return &fakeTCPListener{}, nil
}

// --- udpListenerFactory mock ---
type fakeUDPListener struct{}

func (*fakeUDPListener) Listen() (application.UdpListenerConn, error) { return nil, io.EOF }
func (*fakeUDPListener) Read(_ []byte, _ []byte) (int, int, int, netip.AddrPort, error) {
	return 0, 0, 0, netip.AddrPort{}, io.EOF
}

type ServerWorkerFactoryMockUdpListenerFactory struct {
	Called bool
}

func (m *ServerWorkerFactoryMockUdpListenerFactory) listenUDP(_ application.Socket) application.Listener {
	m.Called = true
	return &fakeUDPListener{}
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
func (m *swflServerConfigurationManager) Configuration() (*server_configuration.Configuration, error) {
	return &server_configuration.Configuration{}, nil
}
func (m *swflServerConfigurationManager) IncrementClientCounter() error {
	return nil
}
func (m *swflServerConfigurationManager) InjectEdKeys(_ ed25519.PublicKey, _ ed25519.PrivateKey) error {
	return nil
}

// --- tests ---
func TestCreateWorker_UnsupportedProtocol(t *testing.T) {
	s := settings.Settings{Protocol: 42}
	cfgMgr := &swflServerConfigurationManager{}
	factory := NewTestServerWorkerFactory(s, nil, nil, nil, nil, cfgMgr)
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
	tcpF := &ServerWorkerFactoryMockTcpListenerFactory{}
	udpF := &ServerWorkerFactoryMockUdpListenerFactory{}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	cfgMgr := &swflServerConfigurationManager{}
	factory := NewTestServerWorkerFactory(s, sockF, tcpF, udpF, logF, cfgMgr)

	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil || err.Error() != "bad socket" {
		t.Fatalf("expected socket error, got %v", err)
	}
}

func TestCreateWorker_TCP_ListenerError(t *testing.T) {
	s := settings.Settings{
		Protocol:     settings.TCP,
		ConnectionIP: "1.2.3.4",
		Port:         "9999",
	}
	sockF := &ServerWorkerFactoryMockSocketFactory{Socket: fakeSocket{"1.2.3.4:9999"}}
	tcpF := &ServerWorkerFactoryMockTcpListenerFactory{Err: errors.New("listen fail")}
	udpF := &ServerWorkerFactoryMockUdpListenerFactory{}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	cfgMgr := &swflServerConfigurationManager{}
	factory := NewTestServerWorkerFactory(s, sockF, tcpF, udpF, logF, cfgMgr)

	_, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err == nil || err.Error() != "failed to listen TCP: listen fail" {
		t.Fatalf("expected listener error, got %v", err)
	}
}

func TestCreateWorker_TCP_Success(t *testing.T) {
	s := settings.Settings{
		Protocol:     settings.TCP,
		ConnectionIP: "127.0.0.1",
		Port:         "0",
	}
	sockF := &ServerWorkerFactoryMockSocketFactory{Socket: fakeSocket{"127.0.0.1:0"}}
	tcpF := &ServerWorkerFactoryMockTcpListenerFactory{}
	udpF := &ServerWorkerFactoryMockUdpListenerFactory{}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	cfgMgr := &swflServerConfigurationManager{}
	factory := NewTestServerWorkerFactory(s, sockF, tcpF, udpF, logF, cfgMgr)

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

func TestCreateWorker_UDP_Success(t *testing.T) {
	s := settings.Settings{
		Protocol:     settings.UDP,
		ConnectionIP: "5.6.7.8",
		Port:         "4242",
	}
	sockF := &ServerWorkerFactoryMockSocketFactory{Socket: fakeSocket{"5.6.7.8:4242"}}
	tcpF := &ServerWorkerFactoryMockTcpListenerFactory{}
	udpF := &ServerWorkerFactoryMockUdpListenerFactory{}
	logF := &ServerWorkerFactoryMockLoggerFactory{}
	cfgMgr := &swflServerConfigurationManager{}
	factory := NewTestServerWorkerFactory(s, sockF, tcpF, udpF, logF, cfgMgr)

	w, err := factory.CreateWorker(context.Background(), nopReadWriteCloser{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !udpF.Called {
		t.Error("expected UDP factory to be called")
	}
	if logF.Count != 1 {
		t.Errorf("expected 1 logger, got %d", logF.Count)
	}
	if err := w.HandleTun(); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}
