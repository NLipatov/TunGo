package common

import (
	"net/netip"
	"testing"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

func TestRuntimeAddressInfoFromClientConfiguration(t *testing.T) {
	cfg := clientConfiguration.Configuration{
		Protocol: settings.TCP,
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				Server: settings.Host{}.
					WithIPv4(netip.MustParseAddr("198.51.100.10")).
					WithIPv6(netip.MustParseAddr("2001:db8::10")),
				IPv4: netip.MustParseAddr("10.0.0.2"),
				IPv6: netip.MustParseAddr("fd00::2"),
			},
		},
	}

	got := RuntimeAddressInfoFromClientConfiguration(cfg)
	if got.ServerIPv4 != netip.MustParseAddr("198.51.100.10") {
		t.Fatalf("ServerIPv4: got %v", got.ServerIPv4)
	}
	if got.ServerIPv6 != netip.MustParseAddr("2001:db8::10") {
		t.Fatalf("ServerIPv6: got %v", got.ServerIPv6)
	}
	if got.TunnelIPv4 != netip.MustParseAddr("10.0.0.2") {
		t.Fatalf("TunnelIPv4: got %v", got.TunnelIPv4)
	}
	if got.TunnelIPv6 != netip.MustParseAddr("fd00::2") {
		t.Fatalf("TunnelIPv6: got %v", got.TunnelIPv6)
	}
}

func TestRuntimeAddressInfoFromServerConfiguration(t *testing.T) {
	cfg := serverConfiguration.Configuration{
		EnableUDP: true,
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				Server: settings.Host{}.
					WithIPv4(netip.MustParseAddr("203.0.113.10")).
					WithIPv6(netip.MustParseAddr("2001:db8::20")),
				IPv4: netip.MustParseAddr("10.1.0.1"),
				IPv6: netip.MustParseAddr("fd01::1"),
			},
		},
	}

	got := RuntimeAddressInfoFromServerConfiguration(cfg)
	if got.ServerIPv4 != netip.MustParseAddr("203.0.113.10") {
		t.Fatalf("ServerIPv4: got %v", got.ServerIPv4)
	}
	if got.ServerIPv6 != netip.MustParseAddr("2001:db8::20") {
		t.Fatalf("ServerIPv6: got %v", got.ServerIPv6)
	}
	if got.TunnelIPv4 != netip.MustParseAddr("10.1.0.1") {
		t.Fatalf("TunnelIPv4: got %v", got.TunnelIPv4)
	}
	if got.TunnelIPv6 != netip.MustParseAddr("fd01::1") {
		t.Fatalf("TunnelIPv6: got %v", got.TunnelIPv6)
	}
}

func TestRuntimeAddressInfoFromClientConfiguration_ActiveSettingsErrorReturnsZeroInfo(t *testing.T) {
	cfg := clientConfiguration.Configuration{
		Protocol: settings.UNKNOWN,
	}

	got := RuntimeAddressInfoFromClientConfiguration(cfg)
	if got.ServerIPv4.IsValid() || got.ServerIPv6.IsValid() || got.TunnelIPv4.IsValid() || got.TunnelIPv6.IsValid() {
		t.Fatalf("expected zero info when active settings resolution fails, got %+v", got)
	}
}
