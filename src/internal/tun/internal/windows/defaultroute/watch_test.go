//go:build windows

package defaultroute

import (
	"net/netip"
	"testing"

	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

func TestIsDefaultRouteChange(t *testing.T) {
	tests := []struct {
		name   string
		prefix netip.Prefix
		want   bool
	}{
		{name: "default route", prefix: netip.MustParsePrefix("0.0.0.0/0"), want: true},
		{name: "split route", prefix: netip.MustParsePrefix("0.0.0.0/1"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var route winipcfg.MibIPforwardRow2
			if err := route.DestinationPrefix.SetPrefix(tt.prefix); err != nil {
				t.Fatal(err)
			}
			if got := isDefaultRouteChange(&route); got != tt.want {
				t.Fatalf("isDefaultRouteChange() = %v, want %v", got, tt.want)
			}
		})
	}
}
