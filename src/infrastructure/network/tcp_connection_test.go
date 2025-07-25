package network

import (
	"errors"
	"net"
	"net/netip"
	"testing"
)

type fakeConn struct{ net.Conn }
type fakeDialer struct {
	conn net.Conn
	err  error
}

func (d *fakeDialer) Dial(_, _ string) (net.Conn, error) {
	return d.conn, d.err
}

func TestTCPConnection_Establish_Success(t *testing.T) {
	addr := netip.MustParseAddrPort("127.0.0.1:12345")
	tc := NewTCPConnectionWithDialer(addr, &fakeDialer{conn: &fakeConn{}})
	c, err := tc.Establish()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if c == nil {
		t.Fatal("expected conn, got nil")
	}
}

func TestTCPConnection_Establish_Error(t *testing.T) {
	addr := netip.MustParseAddrPort("127.0.0.1:12345")
	tc := NewTCPConnectionWithDialer(addr, &fakeDialer{err: errors.New("fail")})
	c, err := tc.Establish()
	if err == nil {
		t.Fatal("expected error")
	}
	if c != nil {
		t.Fatal("expected nil connection")
	}
}

func TestNewTCPConnection_DefaultDialer(t *testing.T) {
	addr := netip.MustParseAddrPort("127.0.0.1:12345")
	c := NewTCPConnection(addr)
	if c == nil {
		t.Fatal("expected not nil")
	}
}
