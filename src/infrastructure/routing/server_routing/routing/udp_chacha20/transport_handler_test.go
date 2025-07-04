package udp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"tungo/application"
	"tungo/infrastructure/listeners/udp_listener"
	"tungo/infrastructure/settings"
	"unsafe"
)

/* ---------- mocks ---------- */

type mockConn struct {
	udp_listener.Listener

	readFn   func(b, oob []byte) (int, int, int, netip.AddrPort, error)
	closedMu sync.Mutex
	closed   bool
}

func (c *mockConn) ListenUDP() (*net.UDPConn, error) {
	return (*net.UDPConn)(unsafe.Pointer(c)), nil
}
func (c *mockConn) ReadMsgUDPAddrPort(b, oob []byte) (int, int, int, netip.AddrPort, error) {
	return c.readFn(b, oob)
}
func (c *mockConn) Close() error {
	c.closedMu.Lock()
	c.closed = true
	c.closedMu.Unlock()
	return nil
}

type mockLogger struct{ logs []string }

func (l *mockLogger) Printf(f string, v ...any) { l.logs = append(l.logs, fmt.Sprintf(f, v...)) }

type mockMgr struct {
}

func (m *mockMgr) Add(Session)                                   {}
func (m *mockMgr) Delete(Session)                                {}
func (m *mockMgr) GetByInternalIP(_ netip.Addr) (Session, error) { return Session{}, nil }
func (m *mockMgr) GetByExternalIP(_ netip.AddrPort) (Session, error) {
	return Session{}, errors.New("not found")
}

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

type mockBuilder struct{}

func (mockBuilder) FromHandshake(application.Handshake, bool) (application.CryptographyService, error) {
	return &mockCryptoService{}, nil
}

type mockCryptoService struct{}

func (mockCryptoService) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (mockCryptoService) Decrypt(b []byte) ([]byte, error) { return b, nil }

type errBuilder struct{ err error }

func (e errBuilder) FromHandshake(application.Handshake, bool) (application.CryptographyService, error) {
	return nil, e.err
}

/* ---------- tests ---------- */

func TestTransportHandler_HandleTransport_ReadError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	c := &mockConn{
		readFn: func(_, _ []byte) (int, int, int, netip.AddrPort, error) {
			calls++
			if calls == 1 {
				return 0, 0, 0, netip.AddrPort{}, errors.New("read fail")
			}
			cancel()
			return 0, 0, 0, netip.AddrPort{}, io.EOF
		},
	}
	lg := &mockLogger{}
	h := &TransportHandler{
		ctx:            ctx,
		settings:       settings.Settings{Port: "4242"},
		writer:         io.Discard,
		sessionManager: &mockMgr{},
		logger:         lg,
		listener:       c,
		cryptoBuilder:  mockBuilder{},
	}

	done := make(chan struct{})
	go func() { _ = h.HandleTransport(); close(done) }()
	<-done

	if len(lg.logs) == 0 || lg.logs[0] != "server listening on port 4242 (UDP)" {
		t.Fatalf("unexpected logs: %v", lg.logs)
	}
}

func TestRegisterClient_ErrHandshake(t *testing.T) {
	conn := &mockConn{}
	factory := testHandshakeFactory{h: &testHandshake{err: errors.New("boom")}}
	h := &TransportHandler{
		ctx:              context.Background(),
		settings:         settings.Settings{},
		writer:           io.Discard,
		sessionManager:   &mockMgr{},
		logger:           &mockLogger{},
		listener:         conn,
		handshakeFactory: factory,
		cryptoBuilder:    mockBuilder{},
	}
	udpConn, _ := conn.ListenUDP()
	err := h.registerClient(udpConn, netip.AddrPortFrom(netip.AddrFrom4([4]byte{1, 2, 3, 4}), 5555), nil)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected handshake error, got %v", err)
	}
}

func TestRegisterClient_ErrCrypto(t *testing.T) {
	conn := &mockConn{}
	factory := testHandshakeFactory{h: &testHandshake{ip: net.IPv4(10, 0, 0, 2)}}
	cryptoErr := errors.New("crypt")
	h := &TransportHandler{
		ctx:              context.Background(),
		settings:         settings.Settings{},
		writer:           io.Discard,
		sessionManager:   &mockMgr{},
		logger:           &mockLogger{},
		listener:         conn,
		handshakeFactory: factory,
		cryptoBuilder:    errBuilder{err: cryptoErr},
	}
	udpConn, _ := conn.ListenUDP()
	err := h.registerClient(udpConn, netip.AddrPortFrom(netip.AddrFrom4([4]byte{1, 1, 1, 1}), 4444), nil)
	if err == nil || err.Error() != "crypt" {
		t.Fatalf("expected crypto error, got %v", err)
	}
}

func TestRegisterClient_BadIP(t *testing.T) {
	conn := &mockConn{}
	factory := testHandshakeFactory{h: &testHandshake{ip: []byte{1, 2}}}
	h := &TransportHandler{
		ctx:              context.Background(),
		settings:         settings.Settings{},
		writer:           io.Discard,
		sessionManager:   &mockMgr{},
		logger:           &mockLogger{},
		listener:         conn,
		handshakeFactory: factory,
		cryptoBuilder:    mockBuilder{},
	}
	udpConn, _ := conn.ListenUDP()
	err := h.registerClient(udpConn, netip.AddrPortFrom(netip.AddrFrom4([4]byte{1, 1, 1, 1}), 2222), nil)
	if err == nil || !strings.Contains(err.Error(), "failed to parse internal IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}
