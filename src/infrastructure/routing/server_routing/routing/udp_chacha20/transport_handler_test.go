package udp_chacha20

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
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

func (m *mockMgr) Add(Session)                                         {}
func (m *mockMgr) Delete(Session)                                      {}
func (m *mockMgr) GetByInternalAddrPort(_ netip.Addr) (Session, error) { return Session{}, nil }
func (m *mockMgr) GetByExternalAddrPort(_ netip.AddrPort) (Session, error) {
	return Session{}, errors.New("not found")
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
	}

	done := make(chan struct{})
	go func() { _ = h.HandleTransport(); close(done) }()
	<-done

	if len(lg.logs) == 0 || lg.logs[0] != "server listening on port 4242 (UDP)" {
		t.Fatalf("unexpected logs: %v", lg.logs)
	}
}
