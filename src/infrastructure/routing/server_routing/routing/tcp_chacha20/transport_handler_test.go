package tcp_chacha20

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/session_management/repository"
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
	key := session.InternalAddr().Unmap()
	m.sessions[key] = session
}
func (m *mockSessionManager) Delete(session Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, session.InternalAddr().Unmap())
}
func (m *mockSessionManager) GetByInternalAddrPort(addr netip.Addr) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[addr.Unmap()]; ok {
		return s, nil
	}
	return Session{}, repository.ErrSessionNotFound
}

func (m *mockSessionManager) GetByExternalAddrPort(_ netip.AddrPort) (Session, error) {
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
	}

	handler.handleClient(ctx, session, writer)

	if len(mgr.sessions) != 0 {
		t.Error("expected session to be removed after context cancel")
	}
}
