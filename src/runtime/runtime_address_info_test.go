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
	if len(got.ProtocolAddresses) != 1 {
		t.Fatalf("expected one protocol address entry, got %d", len(got.ProtocolAddresses))
	}
	if got.ProtocolAddresses[0].Protocol != settings.TCP {
		t.Fatalf("ProtocolAddresses[0].Protocol: got %v", got.ProtocolAddresses[0].Protocol)
	}
	if got.ProtocolAddresses[0].ServerAddress.IPv4 != netip.MustParseAddr("198.51.100.10") {
		t.Fatalf("ProtocolAddresses[0].ServerAddress.IPv4: got %v", got.ProtocolAddresses[0].ServerAddress.IPv4)
	}
	if got.ProtocolAddresses[0].ServerAddress.IPv6 != netip.MustParseAddr("2001:db8::10") {
		t.Fatalf("ProtocolAddresses[0].ServerAddress.IPv6: got %v", got.ProtocolAddresses[0].ServerAddress.IPv6)
	}
	if got.ProtocolAddresses[0].TunnelAddress.IPv4 != netip.MustParseAddr("10.0.0.2") {
		t.Fatalf("ProtocolAddresses[0].TunnelAddress.IPv4: got %v", got.ProtocolAddresses[0].TunnelAddress.IPv4)
	}
	if got.ProtocolAddresses[0].TunnelAddress.IPv6 != netip.MustParseAddr("fd00::2") {
		t.Fatalf("ProtocolAddresses[0].TunnelAddress.IPv6: got %v", got.ProtocolAddresses[0].TunnelAddress.IPv6)
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
	if len(got.ProtocolAddresses) != 1 {
		t.Fatalf("expected one protocol address entry, got %d", len(got.ProtocolAddresses))
	}
	if got.ProtocolAddresses[0].Protocol != settings.UDP {
		t.Fatalf("ProtocolAddresses[0].Protocol: got %v", got.ProtocolAddresses[0].Protocol)
	}
	if got.ProtocolAddresses[0].ServerAddress.IPv4 != netip.MustParseAddr("203.0.113.10") {
		t.Fatalf("ProtocolAddresses[0].ServerAddress.IPv4: got %v", got.ProtocolAddresses[0].ServerAddress.IPv4)
	}
	if got.ProtocolAddresses[0].ServerAddress.IPv6 != netip.MustParseAddr("2001:db8::20") {
		t.Fatalf("ProtocolAddresses[0].ServerAddress.IPv6: got %v", got.ProtocolAddresses[0].ServerAddress.IPv6)
	}
	if got.ProtocolAddresses[0].TunnelAddress.IPv4 != netip.MustParseAddr("10.1.0.1") {
		t.Fatalf("ProtocolAddresses[0].TunnelAddress.IPv4: got %v", got.ProtocolAddresses[0].TunnelAddress.IPv4)
	}
	if got.ProtocolAddresses[0].TunnelAddress.IPv6 != netip.MustParseAddr("fd01::1") {
		t.Fatalf("ProtocolAddresses[0].TunnelAddress.IPv6: got %v", got.ProtocolAddresses[0].TunnelAddress.IPv6)
	}
}

func TestRuntimeAddressInfoFromServerConfiguration_CollectsAllEnabledProtocolAddresses(t *testing.T) {
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
	if len(got.ProtocolAddresses) != 3 {
		t.Fatalf("expected three protocol address entries, got %d", len(got.ProtocolAddresses))
	}
	if got.ProtocolAddresses[0].Protocol != settings.TCP || got.ProtocolAddresses[0].TunnelAddress.IPv4 != netip.MustParseAddr("10.0.0.1") || got.ProtocolAddresses[0].TunnelAddress.IPv6 != netip.MustParseAddr("fd00::1") {
		t.Fatalf("unexpected TCP protocol address entry: %+v", got.ProtocolAddresses[0])
	}
	if got.ProtocolAddresses[1].Protocol != settings.UDP || got.ProtocolAddresses[1].TunnelAddress.IPv4 != netip.MustParseAddr("10.0.1.1") || got.ProtocolAddresses[1].TunnelAddress.IPv6 != netip.MustParseAddr("fd00::2") {
		t.Fatalf("unexpected UDP protocol address entry: %+v", got.ProtocolAddresses[1])
	}
	if got.ProtocolAddresses[2].Protocol != settings.WS || got.ProtocolAddresses[2].TunnelAddress.IPv4 != netip.MustParseAddr("10.0.2.1") || got.ProtocolAddresses[2].TunnelAddress.IPv6 != netip.MustParseAddr("fd00::3") {
		t.Fatalf("unexpected WS protocol address entry: %+v", got.ProtocolAddresses[2])
	}
}

func TestRuntimeAddressInfoFromClientConfiguration_ActiveSettingsErrorReturnsZeroInfo(t *testing.T) {
	cfg := clientConfiguration.Configuration{
		Protocol: settings.UNKNOWN,
	}

	got := RuntimeAddressInfoFromClientConfiguration(cfg)
	if len(got.ProtocolAddresses) != 0 {
		t.Fatalf("expected no protocol address entries when active settings resolution fails, got %d", len(got.ProtocolAddresses))
	}
}

func TestRuntimeAddressInfoFromServerConfiguration_PreservesPerProtocolServerAddresses(t *testing.T) {
	cfg := serverConfiguration.Configuration{
		EnableTCP: true,
		EnableUDP: true,
		TCPSettings: settings.Settings{
			Protocol: settings.TCP,
			Addressing: settings.Addressing{
				Server: settings.Host{}.WithIPv4(netip.MustParseAddr("198.51.100.10")),
				IPv4:   netip.MustParseAddr("10.0.0.1"),
			},
		},
		UDPSettings: settings.Settings{
			Protocol: settings.UDP,
			Addressing: settings.Addressing{
				Server: settings.Host{}.WithIPv6(netip.MustParseAddr("2001:db8::20")),
				IPv4:   netip.MustParseAddr("10.0.1.1"),
			},
		},
	}

	got := RuntimeAddressInfoFromServerConfiguration(cfg)
	if len(got.ProtocolAddresses) != 2 {
		t.Fatalf("expected two protocol address entries, got %d", len(got.ProtocolAddresses))
	}
	if got.ProtocolAddresses[0].ServerAddress.IPv4 != netip.MustParseAddr("198.51.100.10") || got.ProtocolAddresses[0].ServerAddress.IPv6.IsValid() {
		t.Fatalf("unexpected TCP server address: %+v", got.ProtocolAddresses[0].ServerAddress)
	}
	if got.ProtocolAddresses[1].ServerAddress.IPv6 != netip.MustParseAddr("2001:db8::20") || got.ProtocolAddresses[1].ServerAddress.IPv4.IsValid() {
		t.Fatalf("unexpected UDP server address: %+v", got.ProtocolAddresses[1].ServerAddress)
	}
}

func TestNewRuntimeProtocolAddress_InvalidAddressesReturnFalse(t *testing.T) {
	got, ok := newRuntimeProtocolAddress(settings.TCP, RuntimeAddressPair{}, RuntimeAddressPair{})
	if ok {
		t.Fatal("expected invalid address pairs to be rejected")
	}
	if got != (RuntimeProtocolAddress{}) {
		t.Fatalf("expected zero protocol address on invalid input, got %+v", got)
	}
}
