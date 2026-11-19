//go:build darwin

package defaultroute

import (
	"net/netip"
	"testing"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func TestChangesDefaultRoute(t *testing.T) {
	tests := []struct {
		name    string
		message route.Message
		family  int
		want    bool
	}{
		{
			name:    "added IPv4 default route without mask",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{}, nil),
			family:  unix.AF_INET,
			want:    true,
		},
		{
			name:    "deleted IPv4 default route",
			message: newRouteMessage(unix.RTM_DELETE, &route.Inet4Addr{}, &route.Inet4Addr{}),
			family:  unix.AF_INET,
			want:    true,
		},
		{
			name:    "changed IPv6 default route without mask",
			message: newRouteMessage(unix.RTM_CHANGE, &route.Inet6Addr{}, nil),
			family:  unix.AF_INET6,
			want:    true,
		},
		{
			name:    "changed IPv6 default route",
			message: newRouteMessage(unix.RTM_CHANGE, &route.Inet6Addr{}, &route.Inet6Addr{}),
			family:  unix.AF_INET6,
			want:    true,
		},
		{
			name:    "IPv6 default route while watching IPv4",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet6Addr{}, &route.Inet6Addr{}),
			family:  unix.AF_INET,
		},
		{
			name:    "IPv4 default route while watching IPv6",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{}, &route.Inet4Addr{}),
			family:  unix.AF_INET6,
		},
		{
			name: "scoped IPv4 default route",
			message: &route.RouteMessage{
				Type:  unix.RTM_ADD,
				Flags: unix.RTF_IFSCOPE,
				Addrs: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{}, &route.Inet4Addr{}).Addrs,
			},
			family: unix.AF_INET,
		},
		{
			name: "failed IPv4 default route operation",
			message: &route.RouteMessage{
				Type:  unix.RTM_ADD,
				Err:   unix.EEXIST,
				Addrs: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{}, &route.Inet4Addr{}).Addrs,
			},
			family: unix.AF_INET,
		},
		{
			name:    "IPv4 split route",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{}, &route.Inet4Addr{IP: [4]byte{128}}),
			family:  unix.AF_INET,
		},
		{
			name:    "IPv6 split route",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet6Addr{}, &route.Inet6Addr{IP: [16]byte{128}}),
			family:  unix.AF_INET6,
		},
		{
			name:    "IPv4 route with IPv6 mask",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{}, &route.Inet6Addr{}),
			family:  unix.AF_INET,
		},
		{
			name:    "IPv6 route with IPv4 mask",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet6Addr{}, &route.Inet4Addr{}),
			family:  unix.AF_INET6,
		},
		{
			name:    "non-default destination",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{IP: [4]byte{192, 0, 2, 0}}, &route.Inet4Addr{}),
			family:  unix.AF_INET,
		},
		{
			name:    "unrelated route operation",
			message: newRouteMessage(unix.RTM_GET, &route.Inet4Addr{}, &route.Inet4Addr{}),
			family:  unix.AF_INET,
		},
		{
			name:    "missing addresses",
			message: &route.RouteMessage{Type: unix.RTM_ADD},
			family:  unix.AF_INET,
		},
		{
			name:    "missing destination",
			message: newRouteMessage(unix.RTM_ADD, nil, &route.Inet4Addr{}),
			family:  unix.AF_INET,
		},
		{
			name:    "non-route message",
			message: &route.InterfaceMessage{},
			family:  unix.AF_INET,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := changesDefaultRoute(tt.message, tt.family); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddrFamily(t *testing.T) {
	tests := []struct {
		addr netip.Addr
		want int
	}{
		{addr: netip.MustParseAddr("192.0.2.1"), want: unix.AF_INET},
		{addr: netip.MustParseAddr("::ffff:192.0.2.1"), want: unix.AF_INET},
		{addr: netip.MustParseAddr("2001:db8::1"), want: unix.AF_INET6},
	}

	for _, tt := range tests {
		t.Run(tt.addr.String(), func(t *testing.T) {
			if got := addrFamily(tt.addr); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func newRouteMessage(kind int, destination, mask route.Addr) *route.RouteMessage {
	addrs := make([]route.Addr, unix.RTAX_NETMASK+1)
	addrs[unix.RTAX_DST] = destination
	addrs[unix.RTAX_NETMASK] = mask
	return &route.RouteMessage{Type: kind, Addrs: addrs}
}

func TestWatchStarts(t *testing.T) {
	errCh, err := Watch(t.Context(), netip.MustParseAddr("192.0.2.1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errCh == nil {
		t.Fatal("got nil channel")
	}
}
