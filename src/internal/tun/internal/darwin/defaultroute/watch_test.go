//go:build darwin

package defaultroute

import (
	"testing"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func TestIsDefaultRouteChange(t *testing.T) {
	tests := []struct {
		name    string
		message route.Message
		want    bool
	}{
		{
			name:    "added IPv4 default route without mask",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{}, nil),
			want:    true,
		},
		{
			name:    "deleted IPv4 default route",
			message: newRouteMessage(unix.RTM_DELETE, &route.Inet4Addr{}, &route.Inet4Addr{}),
			want:    true,
		},
		{
			name:    "changed IPv6 default route without mask",
			message: newRouteMessage(unix.RTM_CHANGE, &route.Inet6Addr{}, nil),
			want:    true,
		},
		{
			name:    "changed IPv6 default route",
			message: newRouteMessage(unix.RTM_CHANGE, &route.Inet6Addr{}, &route.Inet6Addr{}),
			want:    true,
		},
		{
			name:    "IPv4 split route",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{}, &route.Inet4Addr{IP: [4]byte{128}}),
		},
		{
			name:    "IPv6 split route",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet6Addr{}, &route.Inet6Addr{IP: [16]byte{128}}),
		},
		{
			name:    "IPv4 route with IPv6 mask",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{}, &route.Inet6Addr{}),
		},
		{
			name:    "IPv6 route with IPv4 mask",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet6Addr{}, &route.Inet4Addr{}),
		},
		{
			name:    "non-default destination",
			message: newRouteMessage(unix.RTM_ADD, &route.Inet4Addr{IP: [4]byte{192, 0, 2, 0}}, &route.Inet4Addr{}),
		},
		{
			name:    "unrelated route operation",
			message: newRouteMessage(unix.RTM_GET, &route.Inet4Addr{}, &route.Inet4Addr{}),
		},
		{
			name:    "missing addresses",
			message: &route.RouteMessage{Type: unix.RTM_ADD},
		},
		{
			name:    "missing destination",
			message: newRouteMessage(unix.RTM_ADD, nil, &route.Inet4Addr{}),
		},
		{
			name:    "non-route message",
			message: &route.InterfaceMessage{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDefaultRouteChange(tt.message); got != tt.want {
				t.Fatalf("isDefaultRouteChange() = %v, want %v", got, tt.want)
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
