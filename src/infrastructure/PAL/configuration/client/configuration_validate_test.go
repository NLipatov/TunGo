package client

import (
	"net/netip"
	"strings"
	"testing"
	"tungo/infrastructure/settings"
)

func TestConfigurationValidate_AllowsIPv6OnlyActiveSettings(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.TCP
	cfg.TCPSettings = settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tcp0",
			Server:     mustHostForValidate(t, "2001:db8::1"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			Port:       8080,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected IPv6-only active settings to be valid, got %v", err)
	}
}

func TestConfigurationValidate_FailsWhenNoIPv4AndNoIPv6Subnet(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.UDP
	cfg.UDPSettings = settings.Settings{
		Addressing: settings.Addressing{
			TunName: "udp0",
			Server:  mustHostForValidate(t, "198.51.100.10"),
			Port:    9090,
		},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "both IPv4Subnet and IPv6Subnet are invalid") {
		t.Fatalf("expected subnet validation error, got %v", err)
	}
}

func validClientConfiguration(t *testing.T) Configuration {
	t.Helper()
	return Configuration{
		ClientID:         1,
		ClientPublicKey:  make([]byte, 32),
		ClientPrivateKey: make([]byte, 32),
		X25519PublicKey:  make([]byte, 32),
		Protocol:         settings.UDP,
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				TunName:    "udp0",
				Server:     mustHostForValidate(t, "198.51.100.10"),
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				Port:       9090,
			},
		},
	}
}

func TestConfigurationValidate_FailsWhenActivePortIsInvalid(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.TCP
	cfg.TCPSettings = settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tcp0",
			Server:     mustHostForValidate(t, "198.51.100.10"),
			IPv4Subnet: netip.MustParsePrefix("10.1.0.0/24"),
			Port:       0,
		},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "active settings: invalid Port 0") {
		t.Fatalf("expected active port validation error, got %v", err)
	}
}

func TestConfigurationValidate_AllowsWSSZeroPort(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.WSS
	cfg.WSSettings = settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "ws0",
			Server:     mustHostForValidate(t, "vpn.example.com"),
			IPv4Subnet: netip.MustParsePrefix("10.2.0.0/24"),
			Port:       0,
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected WSS zero-port config to be valid, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnSelectedProtocolMismatch(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.WSS
	cfg.WSSettings = settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "ws0",
			Server:     mustHostForValidate(t, "vpn.example.com"),
			IPv4Subnet: netip.MustParsePrefix("10.2.0.0/24"),
			Port:       443,
		},
		Protocol: settings.UDP,
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "active settings protocol mismatch") {
		t.Fatalf("expected protocol mismatch error, got %v", err)
	}
}

func TestConfigurationValidate_FailsWhenTunNameIsEmpty(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.TunName = "   "

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "active settings: TunName is not configured") {
		t.Fatalf("expected tun name validation error, got %v", err)
	}
}

func TestConfigurationValidate_FailsWhenDNSv4ContainsIPv6(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv4 = []string{"2001:4860:4860::8888"}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "is IPv6, expected IPv4") {
		t.Fatalf("expected DNSv4 family validation error, got %v", err)
	}
}

func TestConfigurationValidate_FailsWhenDNSv6ContainsIPv4(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv6 = []string{"1.1.1.1"}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "is IPv4, expected IPv6") {
		t.Fatalf("expected DNSv6 family validation error, got %v", err)
	}
}

func TestConfigurationValidate_AllowsCustomDNS(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv4 = []string{"9.9.9.9", "1.0.0.1"}
	cfg.UDPSettings.DNSv6 = []string{"2620:fe::9"}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected custom DNS config to be valid, got %v", err)
	}
}

func mustHostForValidate(t *testing.T, raw string) settings.Host {
	t.Helper()
	h, err := settings.NewHost(raw)
	if err != nil {
		t.Fatalf("settings.NewHost(%q): %v", raw, err)
	}
	return h
}
