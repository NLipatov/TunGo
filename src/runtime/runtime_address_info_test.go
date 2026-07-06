package runtime

import (
	"net/netip"
	"testing"

	"tungo/infrastructure/settings"
)

func TestEndpointInfoFromSettings(t *testing.T) {
	settingsValue := settings.Settings{
		Protocol: settings.TCP,
		Addressing: settings.Addressing{
			Server: settings.Host{}.
				WithIPv4(netip.MustParseAddr("198.51.100.10")).
				WithIPv6(netip.MustParseAddr("2001:db8::10")),
			Port: 443,
			IPv4: netip.MustParseAddr("10.0.0.2"),
			IPv6: netip.MustParseAddr("fd00::2"),
		},
	}

	got, ok := EndpointInfoFromSettings(settings.UDP, settingsValue)
	if !ok {
		t.Fatal("expected endpoint entry")
	}
	if got.Protocol != settings.TCP {
		t.Fatalf("Protocol: got %v", got.Protocol)
	}
	if ipv4, ok := got.Server.IPv4(); !ok || ipv4 != netip.MustParseAddr("198.51.100.10") {
		t.Fatalf("Server.IPv4: got %v ok=%v", ipv4, ok)
	}
	if ipv6, ok := got.Server.IPv6(); !ok || ipv6 != netip.MustParseAddr("2001:db8::10") {
		t.Fatalf("Server.IPv6: got %v ok=%v", ipv6, ok)
	}
	if got.Port != 443 {
		t.Fatalf("Port: got %v", got.Port)
	}
	if got.TunnelIPv4 != netip.MustParseAddr("10.0.0.2") {
		t.Fatalf("TunnelIPv4: got %v", got.TunnelIPv4)
	}
	if got.TunnelIPv6 != netip.MustParseAddr("fd00::2") {
		t.Fatalf("TunnelIPv6: got %v", got.TunnelIPv6)
	}
}

func TestEndpointInfoFromSettings_UsesFallbackProtocol(t *testing.T) {
	settingsValue := settings.Settings{
		Protocol: settings.UNKNOWN,
		Addressing: settings.Addressing{
			IPv4: netip.MustParseAddr("10.0.0.1"),
		},
	}

	got, ok := EndpointInfoFromSettings(settings.WS, settingsValue)
	if !ok {
		t.Fatal("expected endpoint entry")
	}
	if got.Protocol != settings.WS {
		t.Fatalf("Protocol: got %v", got.Protocol)
	}
}

func TestEndpointInfoFromSettings_InvalidAddressesReturnFalse(t *testing.T) {
	got, ok := EndpointInfoFromSettings(settings.TCP, settings.Settings{})
	if ok {
		t.Fatal("expected invalid endpoint to be rejected")
	}
	if got != (EndpointInfo{}) {
		t.Fatalf("expected zero endpoint on invalid input, got %+v", got)
	}
}
