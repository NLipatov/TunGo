package transport

import (
	"net"
	"net/netip"
	"testing"
)

func TestNetipAddrFrom(t *testing.T) {
	for _, endpoint := range []string{
		"192.0.2.10:443", "[2001:db8::10]:443",
		"[::ffff:192.0.2.10]:443", "[fe80::1%test-zone]:443",
	} {
		ap := netip.MustParseAddrPort(endpoint)
		for _, addr := range []net.Addr{net.TCPAddrFromAddrPort(ap), net.UDPAddrFromAddrPort(ap)} {
			t.Run(addr.Network()+"/"+endpoint, func(t *testing.T) {
				got, err := NetipAddrFrom(addr)
				if want := ap.Addr().Unmap(); err != nil || got != want {
					t.Fatalf("NetipAddrFrom(%v) = %v, %v; want %v", addr, got, err, want)
				}
			})
		}
	}
}

func TestNetipAddrFromRejectsInvalidAddresses(t *testing.T) {
	for _, tc := range []struct {
		name string
		addr net.Addr
	}{
		{"nil", nil},
		{"empty TCP", &net.TCPAddr{}},
		{"empty UDP", &net.UDPAddr{}},
		{"invalid IP", &net.TCPAddr{IP: net.IP{1, 2, 3}}},
		{"unsupported network", &net.UnixAddr{Name: "test.sock", Net: "unix"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NetipAddrFrom(tc.addr)
			if err == nil || got.IsValid() {
				t.Fatalf("NetipAddrFrom(%v) = %v, %v; want invalid address and error", tc.addr, got, err)
			}
		})
	}
}
