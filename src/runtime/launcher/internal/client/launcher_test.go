package client

import (
	"net/netip"
	"testing"

	palClient "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"
)

func TestEndpointsFromConfiguration(t *testing.T) {
	cfg := palClient.Configuration{
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

	got := endpointsFromConfiguration(cfg)
	if len(got) != 1 {
		t.Fatalf("expected one endpoint entry, got %d", len(got))
	}
	if got[0].Protocol != settings.TCP {
		t.Fatalf("Protocol: got %v", got[0].Protocol)
	}
	if ipv4, ok := got[0].Server.IPv4(); !ok || ipv4 != netip.MustParseAddr("198.51.100.10") {
		t.Fatalf("Server.IPv4: got %v ok=%v", ipv4, ok)
	}
	if ipv6, ok := got[0].Server.IPv6(); !ok || ipv6 != netip.MustParseAddr("2001:db8::10") {
		t.Fatalf("Server.IPv6: got %v ok=%v", ipv6, ok)
	}
	if got[0].Port != 443 {
		t.Fatalf("Port: got %v", got[0].Port)
	}
	if got[0].TunnelIPv4 != netip.MustParseAddr("10.0.0.2") {
		t.Fatalf("TunnelIPv4: got %v", got[0].TunnelIPv4)
	}
	if got[0].TunnelIPv6 != netip.MustParseAddr("fd00::2") {
		t.Fatalf("TunnelIPv6: got %v", got[0].TunnelIPv6)
	}
}

func TestEndpointsFromConfiguration_ActiveSettingsErrorReturnsNoEndpoints(t *testing.T) {
	cfg := palClient.Configuration{Protocol: settings.UNKNOWN}

	got := endpointsFromConfiguration(cfg)
	if len(got) != 0 {
		t.Fatalf("expected no endpoint entries when active settings resolution fails, got %d", len(got))
	}
}
