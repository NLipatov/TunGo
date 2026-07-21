package apple

import (
	"net/netip"
	"reflect"
	"testing"

	"tungo/application/configuration"
	"tungo/infrastructure/settings"
)

func TestNew_DualStackFullTunnel(t *testing.T) {
	host, err := settings.NewHost("vpn.example.com")
	if err != nil {
		t.Fatalf("NewHost() error = %v", err)
	}
	conf := configuration.ClientRuntimeConfiguration{
		Protocol: settings.UDP,
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				Server:     host,
				IPv4:       netip.MustParseAddr("10.0.0.2"),
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				IPv6:       netip.MustParseAddr("fd00::2"),
				IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
				DNSv4:      []string{"1.1.1.1"},
				DNSv6:      []string{"2606:4700:4700::1111"},
			},
			MTU:      1400,
			Protocol: settings.UDP,
		},
	}

	got, err := NewTunnelPlan(conf)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got.RemoteAddress != "vpn.example.com" || got.MTU != 1400 {
		t.Fatalf("plan endpoint/MTU = %q/%d", got.RemoteAddress, got.MTU)
	}
	if got.StartupTimeoutMilliseconds != 6000 {
		t.Fatalf("startup timeout = %d, want 6000", got.StartupTimeoutMilliseconds)
	}
	if got.IPv4 == nil || *got.IPv4 != (IPSettings{Address: "10.0.0.2", PrefixLength: 32}) {
		t.Fatalf("IPv4 settings = %+v", got.IPv4)
	}
	if got.IPv6 == nil || *got.IPv6 != (IPSettings{Address: "fd00::2", PrefixLength: 64}) {
		t.Fatalf("IPv6 settings = %+v", got.IPv6)
	}
	wantRoutes := []Route{{Destination: "0.0.0.0", PrefixLength: 0}, {Destination: "::", PrefixLength: 0}}
	if !reflect.DeepEqual(got.IncludedRoutes, wantRoutes) {
		t.Fatalf("included routes = %+v, want %+v", got.IncludedRoutes, wantRoutes)
	}
	wantDNS := []string{"1.1.1.1", "2606:4700:4700::1111"}
	if !reflect.DeepEqual(got.DNSServers, wantDNS) {
		t.Fatalf("DNS servers = %v, want %v", got.DNSServers, wantDNS)
	}
	if got.ExcludedRoutes == nil || len(got.ExcludedRoutes) != 0 {
		t.Fatalf("excluded routes = %v, want an explicit empty list", got.ExcludedRoutes)
	}
}

func TestNew_IPv6ClampsMTU(t *testing.T) {
	host, _ := settings.NewHost("192.0.2.1")
	conf := configuration.ClientRuntimeConfiguration{
		Protocol: settings.TCP,
		TCPSettings: settings.Settings{Addressing: settings.Addressing{
			Server: host,
			IPv6:   netip.MustParseAddr("fd00::2"),
		}},
	}
	got, err := NewTunnelPlan(conf)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got.MTU != settings.MinimumIPv6MTU {
		t.Fatalf("MTU = %d, want %d", got.MTU, settings.MinimumIPv6MTU)
	}
}

func TestNew_RejectsMissingResolvedAddress(t *testing.T) {
	host, _ := settings.NewHost("192.0.2.1")
	conf := configuration.ClientRuntimeConfiguration{
		Protocol:    settings.TCP,
		TCPSettings: settings.Settings{Addressing: settings.Addressing{Server: host}},
	}
	if _, err := NewTunnelPlan(conf); err == nil {
		t.Fatal("expected missing tunnel address error")
	}
}
