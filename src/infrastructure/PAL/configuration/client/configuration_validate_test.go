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

func TestConfigurationValidate_FailsOnUnknownProtocol(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.UNKNOWN

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "protocol is UNKNOWN") {
		t.Fatalf("expected UNKNOWN protocol error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnBadClientPublicKeyLength(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.ClientPublicKey = make([]byte, 16) // too short

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid ClientPublicKey length") {
		t.Fatalf("expected ClientPublicKey length error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnBadClientPrivateKeyLength(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.ClientPrivateKey = make([]byte, 64) // too long

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid ClientPrivateKey length") {
		t.Fatalf("expected ClientPrivateKey length error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnBadX25519PublicKeyLength(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.X25519PublicKey = make([]byte, 0) // empty

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid X25519PublicKey") {
		t.Fatalf("expected X25519PublicKey length error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnZeroServer(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.Server = settings.Host{} // zero value

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "Server is not configured") {
		t.Fatalf("expected server validation error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnHighPort(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.Port = 70000

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid Port") {
		t.Fatalf("expected port validation error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnZeroClientID(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.ClientID = 0

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid ClientID") {
		t.Fatalf("expected ClientID validation error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnNegativeClientID(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.ClientID = -5

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid ClientID") {
		t.Fatalf("expected ClientID validation error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnEmptyDNSString(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv4 = []string{""}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DNS[0] is empty") {
		t.Fatalf("expected empty DNS error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnWhitespaceDNSString(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv4 = []string{"   "}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DNS[0] is empty") {
		t.Fatalf("expected empty DNS error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnNonIPDNS(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv4 = []string{"not-an-ip"}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "is not an IP address") {
		t.Fatalf("expected non-IP DNS error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnEmptyDNSv6String(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv6 = []string{""}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DNS[0] is empty") {
		t.Fatalf("expected empty DNSv6 error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnNonIPDNSv6(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv6 = []string{"example.com"}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "is not an IP address") {
		t.Fatalf("expected non-IP DNSv6 error, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnNegativePort(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.Port = -1

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid Port") {
		t.Fatalf("expected port validation error, got %v", err)
	}
}

func TestConfigurationValidate_WSSZeroPortRejectsNonWSS(t *testing.T) {
	// Port 0 is only allowed for WSS, not for WS.
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.WS
	cfg.WSSettings = settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "ws0",
			Server:     mustHostForValidate(t, "198.51.100.10"),
			IPv4Subnet: netip.MustParsePrefix("10.2.0.0/24"),
			Port:       0,
		},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid Port 0") {
		t.Fatalf("expected port 0 to be rejected for WS protocol, got %v", err)
	}
}

func TestConfigurationValidate_FailsOnUnsupportedProtocolFromActiveSettings(t *testing.T) {
	// Protocol(255) passes the != UNKNOWN check but fails in activeSettingsPtr.
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.Protocol(255)

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "unsupported protocol") {
		t.Fatalf("expected unsupported protocol error from ActiveSettings, got %v", err)
	}
}
