//go:build darwin

package route

import (
	"net/netip"
	"testing"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func TestSplitRouteMessage(t *testing.T) {
	tests := []struct {
		cidr        string
		destination string
		mask        string
		host        bool
	}{
		{"192.0.2.255/25", "192.0.2.128", "255.255.255.128", false},
		{"192.0.2.1/32", "192.0.2.1", "255.255.255.255", false},
		{"0.0.0.0/0", "0.0.0.0", "0.0.0.0", false},
		{"2001:db8:1:2:ffff::1/65", "2001:db8:1:2:8000::", "ffff:ffff:ffff:ffff:8000::", false},
		{"2001:db8::1/128", "2001:db8::1", "", true},
		{"::/0", "::", "::", false},
	}
	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			message, err := splitRouteMessage(tt.cidr, 7)
			if err != nil {
				t.Fatal(err)
			}
			data, err := message.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			messages, err := route.ParseRIB(route.RIBTypeRoute, data)
			if err != nil || len(messages) != 1 {
				t.Fatalf("decode route: %v, messages: %v", err, messages)
			}
			got := messages[0].(*route.RouteMessage)
			if got.Type != unix.RTM_ADD || got.Flags&(unix.RTF_UP|unix.RTF_STATIC) != unix.RTF_UP|unix.RTF_STATIC || got.Flags&unix.RTF_GATEWAY != 0 {
				t.Fatalf("unexpected route type/flags: %+v", got)
			}
			if (got.Flags&unix.RTF_HOST != 0) != tt.host {
				t.Fatalf("host route = %v, want %v", got.Flags&unix.RTF_HOST != 0, tt.host)
			}
			if destination := routeAddress(got.Addrs[unix.RTAX_DST]); destination.String() != tt.destination {
				t.Fatalf("destination = %s, want %s", destination, tt.destination)
			}
			if tt.mask == "" {
				if got.Addrs[unix.RTAX_NETMASK] != nil {
					t.Fatal("host route must omit the netmask")
				}
			} else if mask := routeAddress(got.Addrs[unix.RTAX_NETMASK]); mask.String() != tt.mask {
				t.Fatalf("mask = %s, want %s", mask, tt.mask)
			}
			gateway, ok := got.Addrs[unix.RTAX_GATEWAY].(*route.LinkAddr)
			if !ok || gateway.Index != 7 {
				t.Fatalf("gateway = %+v, want interface 7", got.Addrs[unix.RTAX_GATEWAY])
			}
		})
	}
}

func TestSplitRouteMessageRejectsInvalidPrefix(t *testing.T) {
	for _, cidr := range []string{
		"invalid",
		"192.0.2.1/33",
		"2001:db8::/129",
		"::ffff:192.0.2.1/128",
		"::ffff:192.0.2.1/80",
	} {
		t.Run(cidr, func(t *testing.T) {
			if _, err := splitRouteMessage(cidr, 7); err == nil {
				t.Fatal("expected invalid prefix error")
			}
		})
	}
}

func TestAddSplitRoutesMissingInterface(t *testing.T) {
	cmd := newMockRunner()
	if err := NewV4(cmd).AddSplit("tungo-missing-interface", []string{"192.0.2.0/24"}); err == nil {
		t.Fatal("expected interface lookup error")
	}
	if err := NewV6(cmd).AddSplit("tungo-missing-interface", []string{"2001:db8::/64"}); err == nil {
		t.Fatal("expected interface lookup error")
	}
	if len(cmd.allCalls()) != 0 {
		t.Fatalf("split routes invoked external commands: %v", cmd.allCalls())
	}
}

func routeAddress(addr route.Addr) netip.Addr {
	switch addr := addr.(type) {
	case *route.Inet4Addr:
		return netip.AddrFrom4(addr.IP)
	case *route.Inet6Addr:
		return netip.AddrFrom16(addr.IP)
	default:
		return netip.Addr{}
	}
}
