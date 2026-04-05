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
	if got.ServerAddress.IPv4 != netip.MustParseAddr("198.51.100.10") {
		t.Fatalf("ServerAddress.IPv4: got %v", got.ServerAddress.IPv4)
	}
	if got.ServerAddress.IPv6 != netip.MustParseAddr("2001:db8::10") {
		t.Fatalf("ServerAddress.IPv6: got %v", got.ServerAddress.IPv6)
	}
	if len(got.TunnelAddresses) != 1 {
		t.Fatalf("expected one tunnel address entry, got %d", len(got.TunnelAddresses))
	}
	if got.TunnelAddresses[0].Protocol != settings.TCP {
		t.Fatalf("TunnelAddresses[0].Protocol: got %v", got.TunnelAddresses[0].Protocol)
	}
	if got.TunnelAddresses[0].Address.IPv4 != netip.MustParseAddr("10.0.0.2") {
		t.Fatalf("TunnelAddresses[0].Address.IPv4: got %v", got.TunnelAddresses[0].Address.IPv4)
	}
	if got.TunnelAddresses[0].Address.IPv6 != netip.MustParseAddr("fd00::2") {
		t.Fatalf("TunnelAddresses[0].Address.IPv6: got %v", got.TunnelAddresses[0].Address.IPv6)
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
	if got.ServerAddress.IPv4 != netip.MustParseAddr("203.0.113.10") {
		t.Fatalf("ServerAddress.IPv4: got %v", got.ServerAddress.IPv4)
	}
	if got.ServerAddress.IPv6 != netip.MustParseAddr("2001:db8::20") {
		t.Fatalf("ServerAddress.IPv6: got %v", got.ServerAddress.IPv6)
	}
	if len(got.TunnelAddresses) != 1 {
		t.Fatalf("expected one tunnel address entry, got %d", len(got.TunnelAddresses))
	}
	if got.TunnelAddresses[0].Protocol != settings.UDP {
		t.Fatalf("TunnelAddresses[0].Protocol: got %v", got.TunnelAddresses[0].Protocol)
	}
	if got.TunnelAddresses[0].Address.IPv4 != netip.MustParseAddr("10.1.0.1") {
		t.Fatalf("TunnelAddresses[0].Address.IPv4: got %v", got.TunnelAddresses[0].Address.IPv4)
	}
	if got.TunnelAddresses[0].Address.IPv6 != netip.MustParseAddr("fd01::1") {
		t.Fatalf("TunnelAddresses[0].Address.IPv6: got %v", got.TunnelAddresses[0].Address.IPv6)
	}
}

func TestRuntimeAddressInfoFromServerConfiguration_CollectsAllEnabledTunnelAddresses(t *testing.T) {
	cfg := serverConfiguration.Configuration{
		EnableTCP: true,
		EnableUDP: true,
		EnableWS:  true,
		TCPSettings: settings.Settings{
			Protocol: settings.TCP,
			Addressing: settings.Addressing{
				IPv4: netip.MustParseAddr("10.0.0.1"),
				IPv6: netip.MustParseAddr("fd00::1"),
			},
		},
		UDPSettings: settings.Settings{
			Protocol: settings.UDP,
			Addressing: settings.Addressing{
				IPv4: netip.MustParseAddr("10.0.1.1"),
				IPv6: netip.MustParseAddr("fd00::2"),
			},
		},
		WSSettings: settings.Settings{
			Protocol: settings.WS,
			Addressing: settings.Addressing{
				IPv4: netip.MustParseAddr("10.0.2.1"),
				IPv6: netip.MustParseAddr("fd00::3"),
			},
		},
	}

	got := RuntimeAddressInfoFromServerConfiguration(cfg)
	if len(got.TunnelAddresses) != 3 {
		t.Fatalf("expected three tunnel address entries, got %d", len(got.TunnelAddresses))
	}
	if got.TunnelAddresses[0].Protocol != settings.TCP || got.TunnelAddresses[0].Address.IPv4 != netip.MustParseAddr("10.0.0.1") || got.TunnelAddresses[0].Address.IPv6 != netip.MustParseAddr("fd00::1") {
		t.Fatalf("unexpected TCP tunnel address entry: %+v", got.TunnelAddresses[0])
	}
	if got.TunnelAddresses[1].Protocol != settings.UDP || got.TunnelAddresses[1].Address.IPv4 != netip.MustParseAddr("10.0.1.1") || got.TunnelAddresses[1].Address.IPv6 != netip.MustParseAddr("fd00::2") {
		t.Fatalf("unexpected UDP tunnel address entry: %+v", got.TunnelAddresses[1])
	}
	if got.TunnelAddresses[2].Protocol != settings.WS || got.TunnelAddresses[2].Address.IPv4 != netip.MustParseAddr("10.0.2.1") || got.TunnelAddresses[2].Address.IPv6 != netip.MustParseAddr("fd00::3") {
		t.Fatalf("unexpected WS tunnel address entry: %+v", got.TunnelAddresses[2])
	}
}

func TestRuntimeAddressInfoFromClientConfiguration_ActiveSettingsErrorReturnsZeroInfo(t *testing.T) {
	cfg := clientConfiguration.Configuration{
		Protocol: settings.UNKNOWN,
	}

	got := RuntimeAddressInfoFromClientConfiguration(cfg)
	if got.ServerAddress.IsValid() {
		t.Fatalf("expected zero info when active settings resolution fails, got %+v", got)
	}
	if len(got.TunnelAddresses) != 0 {
		t.Fatalf("expected no tunnel address entries when active settings resolution fails, got %d", len(got.TunnelAddresses))
	}
}
