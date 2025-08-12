package udp

import (
	"errors"
	"net"
	"net/netip"
	"testing"
)

type mockDialer struct {
	conn *net.UDPConn
	err  error
}

func (m *mockDialer) Dial(_ *net.UDPAddr) (*net.UDPConn, error) {
	return m.conn, m.err
}

func TestUDPConnection_Establish_Success(t *testing.T) {
	addr := netip.MustParseAddrPort("127.0.0.1:4321")
	c := &UDPConnection{
		addrPort: addr,
		dialer:   &mockDialer{conn: &net.UDPConn{}},
	}
	conn, err := c.Establish()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if conn == nil {
		t.Fatal("expected connection, got nil")
	}
}

func TestUDPConnection_Establish_Error(t *testing.T) {
	addr := netip.MustParseAddrPort("127.0.0.1:4321")
	c := &UDPConnection{
		addrPort: addr,
		dialer:   &mockDialer{err: errors.New("fail")},
	}
	conn, err := c.Establish()
	if err == nil {
		t.Fatal("expected error")
	}
	if conn != nil {
		t.Fatal("expected nil connection")
	}
}

func TestNewUDPConnection_DefaultDialer(t *testing.T) {
	addr := netip.MustParseAddrPort("127.0.0.1:4321")
	conn := NewUDPConnection(addr)
	if conn == nil {
		t.Fatal("expected not nil")
	}
}

func TestDefaultUDPDialer_Dial(t *testing.T) {
	l, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func(l *net.UDPConn) {
		_ = l.Close()
	}(l)
	addr := l.LocalAddr().(*net.UDPAddr)

	dialer := &DefaultUDPDialer{}
	conn, err := dialer.Dial(addr)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if conn == nil {
		t.Fatal("expected not nil connection")
	}
	_ = conn.Close()
}
