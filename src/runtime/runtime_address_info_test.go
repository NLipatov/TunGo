package runtime

import (
	"net/netip"
	"testing"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

func TestEndpointInfoFromClientConfiguration(t *testing.T) {
	cfg := clientConfiguration.Configuration{
		Protocol: settings.TCP,
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				Server: settings.Host{}.
					WithIPv4(netip.MustParseAddr("198.51.100.10")).
					WithIPv6(netip.MustParseAddr("2001:db8::10")),
				Port: 443,
				IPv4: netip.MustParseAddr("10.0.0.2"),
				IPv6: netip.MustParseAddr("fd00::2"),
			},
		},
	}

	got := EndpointInfoFromClientConfiguration(cfg)
	if len(got) != 1 {
		t.Fatalf("expected one endpoint entry, got %d", len(got))
	}
	if got[0].Protocol != settings.TCP {
		t.Fatalf("got[0].Protocol: got %v", got[0].Protocol)
	}
	if ipv4, ok := got[0].Server.IPv4(); !ok || ipv4 != netip.MustParseAddr("198.51.100.10") {
		t.Fatalf("got[0].Server.IPv4: got %v ok=%v", ipv4, ok)
	}
	if ipv6, ok := got[0].Server.IPv6(); !ok || ipv6 != netip.MustParseAddr("2001:db8::10") {
		t.Fatalf("got[0].Server.IPv6: got %v ok=%v", ipv6, ok)
	}
	if got[0].Port != 443 {
		t.Fatalf("got[0].Port: got %v", got[0].Port)
	}
	if got[0].TunnelIPv4 != netip.MustParseAddr("10.0.0.2") {
		t.Fatalf("got[0].TunnelIPv4: got %v", got[0].TunnelIPv4)
	}
	if got[0].TunnelIPv6 != netip.MustParseAddr("fd00::2") {
		t.Fatalf("got[0].TunnelIPv6: got %v", got[0].TunnelIPv6)
	}
}

func TestEndpointInfoFromServerConfiguration(t *testing.T) {
	cfg := serverConfiguration.Configuration{
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

	got := EndpointInfoFromServerConfiguration(cfg)
	if len(got) != 1 {
		t.Fatalf("expected one endpoint entry, got %d", len(got))
	}
	if got[0].Protocol != settings.UDP {
		t.Fatalf("got[0].Protocol: got %v", got[0].Protocol)
	}
	if ipv4, ok := got[0].Server.IPv4(); !ok || ipv4 != netip.MustParseAddr("203.0.113.10") {
		t.Fatalf("got[0].Server.IPv4: got %v ok=%v", ipv4, ok)
	}
	if ipv6, ok := got[0].Server.IPv6(); !ok || ipv6 != netip.MustParseAddr("2001:db8::20") {
		t.Fatalf("got[0].Server.IPv6: got %v ok=%v", ipv6, ok)
	}
	if got[0].Port != 51820 {
		t.Fatalf("got[0].Port: got %v", got[0].Port)
	}
	if got[0].TunnelIPv4 != netip.MustParseAddr("10.1.0.1") {
		t.Fatalf("got[0].TunnelIPv4: got %v", got[0].TunnelIPv4)
	}
	if got[0].TunnelIPv6 != netip.MustParseAddr("fd01::1") {
		t.Fatalf("got[0].TunnelIPv6: got %v", got[0].TunnelIPv6)
	}
}

func TestEndpointInfoFromServerConfiguration_CollectsAllEnabledEndpoints(t *testing.T) {
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

	got := EndpointInfoFromServerConfiguration(cfg)
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

func TestEndpointInfoFromClientConfiguration_ActiveSettingsErrorReturnsNoEndpoints(t *testing.T) {
	cfg := clientConfiguration.Configuration{
		Protocol: settings.UNKNOWN,
	}

	got := EndpointInfoFromClientConfiguration(cfg)
	if len(got) != 0 {
		t.Fatalf("expected no endpoint entries when active settings resolution fails, got %d", len(got))
	}
}

func TestEndpointInfoFromServerConfiguration_PreservesPerProtocolServerAddresses(t *testing.T) {
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

	got := EndpointInfoFromServerConfiguration(cfg)
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

func TestNewEndpointInfo_InvalidAddressesReturnFalse(t *testing.T) {
	got, ok := newEndpointInfo(settings.TCP, settings.Host{}, 0, netip.Addr{}, netip.Addr{})
	if ok {
		t.Fatal("expected invalid endpoint to be rejected")
	}
	if got != (EndpointInfo{}) {
		t.Fatalf("expected zero endpoint on invalid input, got %+v", got)
	}
}
