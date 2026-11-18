//go:build windows

package defaultroute

import (
	"net/netip"
	"testing"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

func TestIsDefaultRouteChange(t *testing.T) {
	if isDefaultRouteChange(nil, windows.AF_INET) {
		t.Fatal("isDefaultRouteChange(nil) = true")
	}

	if isDefaultRouteChange(&winipcfg.MibIPforwardRow2{}, windows.AF_INET) {
		t.Fatal("isDefaultRouteChange() = true for invalid prefix")
	}

	tests := []struct {
		name   string
		prefix netip.Prefix
		family winipcfg.AddressFamily
		want   bool
	}{
		{name: "IPv4 default route", prefix: netip.MustParsePrefix("0.0.0.0/0"), family: windows.AF_INET, want: true},
		{name: "IPv6 default route", prefix: netip.MustParsePrefix("::/0"), family: windows.AF_INET6, want: true},
		{name: "IPv6 default route while watching IPv4", prefix: netip.MustParsePrefix("::/0"), family: windows.AF_INET},
		{name: "IPv4 default route while watching IPv6", prefix: netip.MustParsePrefix("0.0.0.0/0"), family: windows.AF_INET6},
		{name: "IPv4 split route", prefix: netip.MustParsePrefix("0.0.0.0/1"), family: windows.AF_INET},
		{name: "IPv6 split route", prefix: netip.MustParsePrefix("::/1"), family: windows.AF_INET6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var route winipcfg.MibIPforwardRow2
			if err := route.DestinationPrefix.SetPrefix(tt.prefix); err != nil {
				t.Fatal(err)
			}
			if got := isDefaultRouteChange(&route, tt.family); got != tt.want {
				t.Fatalf("isDefaultRouteChange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddrFamily(t *testing.T) {
	tests := []struct {
		addr netip.Addr
		want winipcfg.AddressFamily
	}{
		{addr: netip.MustParseAddr("192.0.2.1"), want: windows.AF_INET},
		{addr: netip.MustParseAddr("::ffff:192.0.2.1"), want: windows.AF_INET},
		{addr: netip.MustParseAddr("2001:db8::1"), want: windows.AF_INET6},
	}

	for _, tt := range tests {
		t.Run(tt.addr.String(), func(t *testing.T) {
			if got := addrFamily(tt.addr); got != tt.want {
				t.Fatalf("addrFamily() = %v, want %v", got, tt.want)
			}
		})
	}
}
