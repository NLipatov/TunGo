package tcp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/settings"
)

// --- Mocks ---

type mockListener struct {
	acceptC chan net.Conn
	errC    chan error
	closed  bool
	mu      sync.Mutex
}

func (m *mockListener) Accept() (net.Conn, error) {
	select {
	case c := <-m.acceptC:
		return c, nil
	case err := <-m.errC:
		return nil, err
	}
}

func (m *mockListener) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

type thMockConn struct {
	bytes.Buffer
}

func (m *thMockConn) LocalAddr() net.Addr {
	panic("not implemented")
}

func (m *thMockConn) SetDeadline(_ time.Time) error {
	panic("not implemented")
}

func (m *thMockConn) SetReadDeadline(_ time.Time) error {
	panic("not implemented")
}

func (m *thMockConn) SetWriteDeadline(_ time.Time) error {
	panic("not implemented")
}

func (m *thMockConn) Read(b []byte) (int, error)  { return m.Buffer.Read(b) }
func (m *thMockConn) Write(b []byte) (int, error) { return m.Buffer.Write(b) }
func (m *thMockConn) Close() error                { return nil }
func (m *thMockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 12345,
	}
}

type mockSessionManager struct {
	sessions map[netip.Addr]Session
	mu       sync.Mutex
}

func (m *mockSessionManager) Add(session Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := session.InternalIP().Unmap()
	m.sessions[key] = session
}
func (m *mockSessionManager) Delete(session Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, session.InternalIP().Unmap())
}
func (m *mockSessionManager) GetByInternalIP(addr netip.Addr) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[addr.Unmap()]; ok {
		return s, nil
	}
	return Session{}, session_management.ErrSessionNotFound
}

func (m *mockSessionManager) GetByExternalIP(_ netip.AddrPort) (Session, error) {
	panic("not implemented")
}

type mockLogger struct {
	logs []string
	mu   sync.Mutex
}

func (m *mockLogger) Printf(format string, _ ...any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs = append(m.logs, format)
}

type mockHandshakeFactory struct{}

func (m mockHandshakeFactory) NewHandshake() application.Handshake {
	return &mockHandshake{}
}

type mockCryptoBuilder struct{}

func (mockCryptoBuilder) FromHandshake(application.Handshake, bool) (application.CryptographyService, error) {
	return &mockCryptoService{}, nil
}

type mockCryptoService struct{}

func (mockCryptoService) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (mockCryptoService) Decrypt(b []byte) ([]byte, error) { return b, nil }

type testHandshake struct {
	ip  []byte
	err error
}

func (h *testHandshake) Id() [32]byte      { return [32]byte{} }
func (h *testHandshake) ClientKey() []byte { return make([]byte, 32) }
func (h *testHandshake) ServerKey() []byte { return make([]byte, 32) }
func (h *testHandshake) ServerSideHandshake(application.ConnectionAdapter) (net.IP, error) {
	return h.ip, h.err
}
func (h *testHandshake) ClientSideHandshake(application.ConnectionAdapter, settings.Settings) error {
	return nil
}

type testHandshakeFactory struct{ h application.Handshake }

func (f testHandshakeFactory) NewHandshake() application.Handshake { return f.h }

type mockHandshake struct{}

func (m *mockHandshake) Id() [32]byte {
	return [32]byte{}
}

func (m *mockHandshake) ClientKey() []byte {
	return make([]byte, 32)
}

func (m *mockHandshake) ServerKey() []byte {
	return make([]byte, 32)
}

func (m *mockHandshake) ServerSideHandshake(application.ConnectionAdapter) (net.IP, error) {
	return net.IPv4(10, 0, 0, 1), nil
}

func (m *mockHandshake) ClientSideHandshake(application.ConnectionAdapter, settings.Settings) error {
	return nil
}

// --- Tests ---

func TestTransportHandler_HandleTransport_acceptFail(t *testing.T) {
	listener := &mockListener{
		errC: make(chan error, 1),
	}
	listener.errC <- errors.New("accept error")
	logger := &mockLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handler := &TransportHandler{
		ctx:      ctx,
		settings: settings.Settings{Port: "1234"},
		listener: listener,
		sessionManager: &mockSessionManager{
			sessions: make(map[netip.Addr]Session),
		},
		Logger:           logger,
		handshakeFactory: mockHandshakeFactory{},
		cryptoBuilder:    mockCryptoBuilder{},
	}

	_ = handler.HandleTransport()

	if len(logger.logs) == 0 {
		t.Error("expected log for failed accept")
	}
}

func TestTransportHandler_HandleTransport_contextCancel(t *testing.T) {
	listener := &mockListener{}
	logger := &mockLogger{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handler := &TransportHandler{
		ctx:      ctx,
		settings: settings.Settings{Port: "1234"},
		listener: listener,
		sessionManager: &mockSessionManager{
			sessions: make(map[netip.Addr]Session),
		},
		Logger:           logger,
		handshakeFactory: mockHandshakeFactory{},
		cryptoBuilder:    mockCryptoBuilder{},
	}

	_ = handler.HandleTransport()

	if !listener.closed {
		t.Error("expected listener to be closed")
	}
}

func TestTransportHandler_registerClient_success(t *testing.T) {
	conn := &thMockConn{}
	writer := &mockConn{}
	logger := &mockLogger{}
	mgr := &mockSessionManager{sessions: map[netip.Addr]Session{}}

	session := Session{conn: conn}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handler := &TransportHandler{
		ctx:              ctx,
		settings:         settings.Settings{},
		writer:           writer,
		sessionManager:   mgr,
		Logger:           logger,
		handshakeFactory: mockHandshakeFactory{},
		cryptoBuilder:    mockCryptoBuilder{},
	}

	handler.handleClient(ctx, session, writer)

	if len(mgr.sessions) != 0 {
		t.Error("expected session to be removed after context cancel")
	}
}

func TestRegisterClient_HandshakeError(t *testing.T) {
	conn := &thMockConn{}
	h := &TransportHandler{
		ctx:              context.Background(),
		settings:         settings.Settings{},
		writer:           &mockConn{},
		sessionManager:   &mockSessionManager{sessions: map[netip.Addr]Session{}},
		Logger:           &mockLogger{},
		handshakeFactory: testHandshakeFactory{h: &testHandshake{err: errors.New("hs")}},
		cryptoBuilder:    mockCryptoBuilder{},
	}
	err := h.registerClient(conn, &mockConn{}, context.Background())
	if err == nil || err.Error() != "client 127.0.0.1:12345 failed registration: hs" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type badAddrConn struct{ thMockConn }

func (badAddrConn) RemoteAddr() net.Addr { return mockAddr{} }

type mockAddr struct{}

func (mockAddr) Network() string { return "mock" }
func (mockAddr) String() string  { return "mock" }

func TestRegisterClient_InvalidAddr(t *testing.T) {
	conn := &badAddrConn{}
	h := &TransportHandler{
		ctx:              context.Background(),
		settings:         settings.Settings{},
		writer:           &mockConn{},
		sessionManager:   &mockSessionManager{sessions: map[netip.Addr]Session{}},
		Logger:           &mockLogger{},
		handshakeFactory: testHandshakeFactory{h: &testHandshake{ip: net.IPv4(10, 0, 0, 2)}},
		cryptoBuilder:    mockCryptoBuilder{},
	}
	err := h.registerClient(conn, &mockConn{}, context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid remote address type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRegisterClient_InternalIPInUse(t *testing.T) {
	ip := netip.AddrFrom4([4]byte{10, 0, 0, 3})
	mgr := &mockSessionManager{sessions: map[netip.Addr]Session{
		ip: {internalIP: ip},
	}}
	conn := &thMockConn{}
	h := &TransportHandler{
		ctx:              context.Background(),
		settings:         settings.Settings{},
		writer:           &mockConn{},
		sessionManager:   mgr,
		Logger:           &mockLogger{},
		handshakeFactory: testHandshakeFactory{h: &testHandshake{ip: net.IPv4(10, 0, 0, 3)}},
		cryptoBuilder:    mockCryptoBuilder{},
	}
	err := h.registerClient(conn, &mockConn{}, context.Background())
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("expected in-use error, got %v", err)
	}
}
