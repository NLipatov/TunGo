package host_resolver

import (
	"errors"
	"net"
	"testing"
	"time"
)

type stubConn struct {
	localAddr  net.Addr
	closeCalls int
}

func (s *stubConn) Read(_ []byte) (int, error)         { return 0, nil }
func (s *stubConn) Write(_ []byte) (int, error)        { return 0, nil }
func (s *stubConn) Close() error                       { s.closeCalls++; return nil }
func (s *stubConn) LocalAddr() net.Addr                { return s.localAddr }
func (s *stubConn) RemoteAddr() net.Addr               { return &net.UDPAddr{} }
func (s *stubConn) SetDeadline(_ time.Time) error      { return nil }
func (s *stubConn) SetReadDeadline(_ time.Time) error  { return nil }
func (s *stubConn) SetWriteDeadline(_ time.Time) error { return nil }

func TestDialResolver_ResolveIPv4(t *testing.T) {
	conn := &stubConn{localAddr: &net.UDPAddr{IP: net.IPv4(192, 0, 2, 10), Port: 12345}}
	resolver := &DialResolver{
		dial: func(network, address string) (net.Conn, error) {
			if network != "udp4" {
				t.Fatalf("network: want udp4, got %s", network)
			}
			if address != "8.8.8.8:80" {
				t.Fatalf("address: want 8.8.8.8:80, got %s", address)
			}
			return conn, nil
		},
	}

	got, err := resolver.ResolveIPv4()
	if err != nil {
		t.Fatalf("ResolveIPv4 returned error: %v", err)
	}
	if got != "192.0.2.10" {
		t.Fatalf("ResolveIPv4: want 192.0.2.10, got %s", got)
	}
	if conn.closeCalls != 1 {
		t.Fatalf("Close calls: want 1, got %d", conn.closeCalls)
	}
}

func TestDialResolver_ResolveIPv6(t *testing.T) {
	conn := &stubConn{localAddr: &net.UDPAddr{IP: net.ParseIP("2001:db8::1"), Port: 12345}}
	resolver := &DialResolver{
		dial: func(network, address string) (net.Conn, error) {
			if network != "udp6" {
				t.Fatalf("network: want udp6, got %s", network)
			}
			if address != "[2001:4860:4860::8888]:80" {
				t.Fatalf("address: want [2001:4860:4860::8888]:80, got %s", address)
			}
			return conn, nil
		},
	}

	got, err := resolver.ResolveIPv6()
	if err != nil {
		t.Fatalf("ResolveIPv6 returned error: %v", err)
	}
	if got != "2001:db8::1" {
		t.Fatalf("ResolveIPv6: want 2001:db8::1, got %s", got)
	}
	if conn.closeCalls != 1 {
		t.Fatalf("Close calls: want 1, got %d", conn.closeCalls)
	}
}

func TestDialResolver_ResolveIPv4_DialError(t *testing.T) {
	wantErr := errors.New("dial failed")
	resolver := &DialResolver{
		dial: func(string, string) (net.Conn, error) {
			return nil, wantErr
		},
	}

	_, err := resolver.ResolveIPv4()
	if !errors.Is(err, wantErr) {
		t.Fatalf("ResolveIPv4: want %v, got %v", wantErr, err)
	}
}

func TestDialResolver_ResolveIPv4_UnexpectedLocalAddrType(t *testing.T) {
	conn := &stubConn{localAddr: &net.IPAddr{IP: net.IPv4(192, 0, 2, 10)}}
	resolver := &DialResolver{
		dial: func(string, string) (net.Conn, error) {
			return conn, nil
		},
	}

	_, err := resolver.ResolveIPv4()
	if err == nil {
		t.Fatal("ResolveIPv4: expected error for unexpected local address type")
	}
	if conn.closeCalls != 1 {
		t.Fatalf("Close calls: want 1, got %d", conn.closeCalls)
	}
}
