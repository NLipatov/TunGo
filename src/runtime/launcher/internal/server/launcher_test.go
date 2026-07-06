package server

import (
	"net/netip"
	"testing"

	serverConf "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

func TestEndpointsFromConfiguration(t *testing.T) {
	cfg := serverConf.Configuration{
		EnableUDP: true,
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				Server: settings.Host{}.
					WithIPv4(netip.MustParseAddr("203.0.113.10")).
					WithIPv6(netip.MustParseAddr("2001:db8::20")),
				Port: 51820,
				IPv4: netip.MustParseAddr("10.1.0.1"),
				IPv6: netip.MustParseAddr("fd01::1"),
			},
		},
	}

	got := endpointsFromConfiguration(cfg)
	if len(got) != 1 {
		t.Fatalf("expected one endpoint entry, got %d", len(got))
	}
	if got[0].Protocol != settings.UDP {
		t.Fatalf("Protocol: got %v", got[0].Protocol)
	}
	if ipv4, ok := got[0].Server.IPv4(); !ok || ipv4 != netip.MustParseAddr("203.0.113.10") {
		t.Fatalf("Server.IPv4: got %v ok=%v", ipv4, ok)
	}
	if ipv6, ok := got[0].Server.IPv6(); !ok || ipv6 != netip.MustParseAddr("2001:db8::20") {
		t.Fatalf("Server.IPv6: got %v ok=%v", ipv6, ok)
	}
	if got[0].Port != 51820 {
		t.Fatalf("Port: got %v", got[0].Port)
	}
	if got[0].TunnelIPv4 != netip.MustParseAddr("10.1.0.1") {
		t.Fatalf("TunnelIPv4: got %v", got[0].TunnelIPv4)
	}
	if got[0].TunnelIPv6 != netip.MustParseAddr("fd01::1") {
		t.Fatalf("TunnelIPv6: got %v", got[0].TunnelIPv6)
	}
}

func TestEndpointsFromConfiguration_CollectsAllEnabledEndpoints(t *testing.T) {
	cfg := serverConf.Configuration{
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

	got := endpointsFromConfiguration(cfg)
	if len(got) != 3 {
		t.Fatalf("expected three endpoint entries, got %d", len(got))
	}
	if got[0].Protocol != settings.TCP || got[0].TunnelIPv4 != netip.MustParseAddr("10.0.0.1") || got[0].TunnelIPv6 != netip.MustParseAddr("fd00::1") {
		t.Fatalf("unexpected TCP endpoint entry: %+v", got[0])
	}
	if got[1].Protocol != settings.UDP || got[1].TunnelIPv4 != netip.MustParseAddr("10.0.1.1") || got[1].TunnelIPv6 != netip.MustParseAddr("fd00::2") {
		t.Fatalf("unexpected UDP endpoint entry: %+v", got[1])
	}
	if got[2].Protocol != settings.WS || got[2].TunnelIPv4 != netip.MustParseAddr("10.0.2.1") || got[2].TunnelIPv6 != netip.MustParseAddr("fd00::3") {
		t.Fatalf("unexpected WS endpoint entry: %+v", got[2])
	}
}

func TestEndpointsFromConfiguration_PreservesPerProtocolServerAddresses(t *testing.T) {
	cfg := serverConf.Configuration{
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

	got := endpointsFromConfiguration(cfg)
	if len(got) != 2 {
		t.Fatalf("expected two endpoint entries, got %d", len(got))
	}
	if ipv4, ok := got[0].Server.IPv4(); !ok || ipv4 != netip.MustParseAddr("198.51.100.10") || got[0].Server.HasIPv6() {
		t.Fatalf("unexpected TCP server address: %+v", got[0].Server)
	}
	if ipv6, ok := got[1].Server.IPv6(); !ok || ipv6 != netip.MustParseAddr("2001:db8::20") || got[1].Server.HasIPv4() {
		t.Fatalf("unexpected UDP server address: %+v", got[1].Server)
	}
}
