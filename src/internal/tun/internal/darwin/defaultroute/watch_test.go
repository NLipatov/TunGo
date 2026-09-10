//go:build darwin

package defaultroute

import (
	"testing"

	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

func TestIsDefaultRouteChange(t *testing.T) {
	tests := []struct {
		name string
		mask [4]byte
		want bool
	}{
		{name: "default route", want: true},
		{name: "split route", mask: [4]byte{128}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addrs := make([]route.Addr, unix.RTAX_NETMASK+1)
			addrs[unix.RTAX_DST] = &route.Inet4Addr{}
			addrs[unix.RTAX_NETMASK] = &route.Inet4Addr{IP: tt.mask}

			message := &route.RouteMessage{
				Type:  unix.RTM_CHANGE,
				Addrs: addrs,
			}
			if got := isDefaultRouteChange(message); got != tt.want {
				t.Fatalf("isDefaultRouteChange() = %v, want %v", got, tt.want)
			}
		})
	}
}
