package client

import (
	"net/netip"
	"strings"
	"testing"
	"tungo/internal/config/settings"
)

func TestValidate_AllowsIPv6OnlyActiveSettings(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.TCP
	cfg.TCPSettings = settings.Settings{
		Network: settings.Network{
			TunName:    "tcp0",
			Server:     mustHostForValidate(t, "2001:db8::1"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			Port:       8080,
		},
		MTU: settings.MinimumIPv6MTU,
	}

	if err := validate(cfg); err != nil {
		t.Fatalf("expected IPv6-only active settings to be valid, got %v", err)
	}
}

func TestValidate_FailsWhenNoIPv4AndNoIPv6Subnet(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.UDP
	cfg.UDPSettings = settings.Settings{
		Network: settings.Network{
			TunName: "udp0",
			Server:  mustHostForValidate(t, "198.51.100.10"),
			Port:    9090,
		},
		MTU: settings.DefaultMTU,
	}

	err := validate(cfg)
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
			Network: settings.Network{
				TunName:    "udp0",
				Server:     mustHostForValidate(t, "198.51.100.10"),
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				Port:       9090,
			},
			MTU: settings.DefaultMTU,
		},
	}
}

func TestValidate_FailsWhenActivePortIsInvalid(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.TCP
	cfg.TCPSettings = settings.Settings{
		Network: settings.Network{
			TunName:    "tcp0",
			Server:     mustHostForValidate(t, "198.51.100.10"),
			IPv4Subnet: netip.MustParsePrefix("10.1.0.0/24"),
			Port:       0,
		},
		MTU: settings.DefaultMTU,
	}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "active settings: invalid Port 0") {
		t.Fatalf("expected active port validation error, got %v", err)
	}
}

func TestValidate_AllowsWSSZeroPort(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.WSS
	cfg.WSSettings = settings.Settings{
		Network: settings.Network{
			TunName:    "ws0",
			Server:     mustHostForValidate(t, "vpn.example.com"),
			IPv4Subnet: netip.MustParsePrefix("10.2.0.0/24"),
			Port:       0,
		},
		MTU: settings.DefaultMTU,
	}

	if err := validate(cfg); err != nil {
		t.Fatalf("expected WSS zero-port config to be valid, got %v", err)
	}
}

func TestValidate_IgnoresNestedProtocol(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.WSS
	cfg.WSSettings = settings.Settings{
		Network: settings.Network{
			TunName:    "ws0",
			Server:     mustHostForValidate(t, "vpn.example.com"),
			IPv4Subnet: netip.MustParsePrefix("10.2.0.0/24"),
			Port:       443,
		},
		MTU:      settings.DefaultMTU,
		Protocol: settings.UDP,
	}

	if err := validate(cfg); err != nil {
		t.Fatalf("expected nested protocol to be ignored, got %v", err)
	}
}

func TestValidate_FailsWhenTunNameIsEmpty(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.TunName = "   "

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "active settings: TunName is not configured") {
		t.Fatalf("expected tun name validation error, got %v", err)
	}
}

func TestValidate_FailsWhenTunNameContainsUnsupportedCharacters(t *testing.T) {
	for _, name := range []string{"tun 0", "tun\x000", "tun\u200b0"} {
		cfg := validClientConfiguration(t)
		cfg.UDPSettings.TunName = name

		err := validate(cfg)
		if err == nil || !strings.Contains(err.Error(), "TunName contains unsupported characters") {
			t.Fatalf("TunName %q: expected unsupported character error, got %v", name, err)
		}
	}
}

func TestValidate_FailsWhenDNSv4ContainsIPv6(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv4 = []string{"2001:4860:4860::8888"}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "is IPv6, expected IPv4") {
		t.Fatalf("expected DNSv4 family validation error, got %v", err)
	}
}

func TestValidate_FailsWhenDNSv6ContainsIPv4(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv6 = []string{"1.1.1.1"}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "is IPv4, expected IPv6") {
		t.Fatalf("expected DNSv6 family validation error, got %v", err)
	}
}

func TestValidate_AllowsCustomDNS(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv4 = []string{"9.9.9.9", "1.0.0.1"}
	cfg.UDPSettings.DNSv6 = []string{"2620:fe::9"}

	if err := validate(cfg); err != nil {
		t.Fatalf("expected custom DNS config to be valid, got %v", err)
	}
}

func TestValidate_RejectsInvalidServerHost(t *testing.T) {
	t.Parallel()

	tests := []settings.Host{
		{IPv4: "2001:db8::1"},
		{IPv6: "192.0.2.1"},
	}
	for _, host := range tests {
		cfg := validClientConfiguration(t)
		cfg.UDPSettings.Server = host
		if err := validate(cfg); err == nil {
			t.Errorf("expected validation error for server host %+v", host)
		}
	}
}

func mustHostForValidate(t *testing.T, raw string) settings.Host {
	t.Helper()
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return settings.Host{Domain: raw}
	}
	if ip.Unmap().Is4() {
		return settings.Host{IPv4: ip.Unmap().String()}
	}
	return settings.Host{IPv6: ip.String()}
}

func TestValidate_FailsOnBadClientPublicKeyLength(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.ClientPublicKey = make([]byte, 16) // too short

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid ClientPublicKey length") {
		t.Fatalf("expected ClientPublicKey length error, got %v", err)
	}
}

func TestValidate_FailsOnBadClientPrivateKeyLength(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.ClientPrivateKey = make([]byte, 64) // too long

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid ClientPrivateKey length") {
		t.Fatalf("expected ClientPrivateKey length error, got %v", err)
	}
}

func TestValidate_FailsOnBadX25519PublicKeyLength(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.X25519PublicKey = make([]byte, 0) // empty

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid X25519PublicKey") {
		t.Fatalf("expected X25519PublicKey length error, got %v", err)
	}
}

func TestValidate_FailsOnZeroServer(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.Server = settings.Host{} // zero value

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "Server is not configured") {
		t.Fatalf("expected server validation error, got %v", err)
	}
}

func TestValidate_FailsOnHighPort(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.Port = 70000

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid Port") {
		t.Fatalf("expected port validation error, got %v", err)
	}
}

func TestValidate_FailsOnZeroClientID(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.ClientID = 0

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid ClientID") {
		t.Fatalf("expected ClientID validation error, got %v", err)
	}
}

func TestValidate_FailsOnNegativeClientID(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.ClientID = -5

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid ClientID") {
		t.Fatalf("expected ClientID validation error, got %v", err)
	}
}

func TestValidate_FailsOnEmptyDNSString(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv4 = []string{""}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "DNS[0] is empty") {
		t.Fatalf("expected empty DNS error, got %v", err)
	}
}

func TestValidate_FailsOnWhitespaceDNSString(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv4 = []string{"   "}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "DNS[0] is empty") {
		t.Fatalf("expected empty DNS error, got %v", err)
	}
}

func TestValidate_FailsOnNonIPDNS(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv4 = []string{"not-an-ip"}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "is not an IP address") {
		t.Fatalf("expected non-IP DNS error, got %v", err)
	}
}

func TestValidate_FailsOnEmptyDNSv6String(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv6 = []string{""}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "DNS[0] is empty") {
		t.Fatalf("expected empty DNSv6 error, got %v", err)
	}
}

func TestValidate_FailsOnNonIPDNSv6(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.DNSv6 = []string{"example.com"}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "is not an IP address") {
		t.Fatalf("expected non-IP DNSv6 error, got %v", err)
	}
}

func TestValidate_FailsOnNegativePort(t *testing.T) {
	cfg := validClientConfiguration(t)
	cfg.UDPSettings.Port = -1

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid Port") {
		t.Fatalf("expected port validation error, got %v", err)
	}
}

func TestValidate_WSSZeroPortRejectsNonWSS(t *testing.T) {
	// Port 0 is only allowed for WSS, not for WS.
	cfg := validClientConfiguration(t)
	cfg.Protocol = settings.WS
	cfg.WSSettings = settings.Settings{
		Network: settings.Network{
			TunName:    "ws0",
			Server:     mustHostForValidate(t, "198.51.100.10"),
			IPv4Subnet: netip.MustParsePrefix("10.2.0.0/24"),
			Port:       0,
		},
		MTU: settings.DefaultMTU,
	}

	err := validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "invalid Port 0") {
		t.Fatalf("expected port 0 to be rejected for WS protocol, got %v", err)
	}
}

func TestValidate_RejectsMTUOutsideActiveFamilyRange(t *testing.T) {
	tests := []struct {
		name       string
		ipv6Subnet netip.Prefix
		mtu        int
	}{
		{name: "negative", mtu: -1},
		{name: "IPv4 below minimum", mtu: settings.MinimumIPv4MTU - 1},
		{name: "IPv6 below minimum", ipv6Subnet: netip.MustParsePrefix("fd00::/64"), mtu: settings.MinimumIPv6MTU - 1},
		{name: "above maximum", mtu: settings.MaximumMTU + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validClientConfiguration(t)
			cfg.UDPSettings.IPv6Subnet = tt.ipv6Subnet
			cfg.UDPSettings.MTU = tt.mtu
			if err := validate(cfg); err == nil || !strings.Contains(err.Error(), "invalid MTU") {
				t.Fatalf("expected MTU validation error, got %v", err)
			}
		})
	}
}

func TestValidate_RejectsMismatchedSubnetFamilies(t *testing.T) {
	tests := []struct {
		name string
		set  func(*settings.Settings)
	}{
		{
			name: "IPv6 prefix in IPv4Subnet",
			set: func(s *settings.Settings) {
				s.IPv4Subnet = netip.MustParsePrefix("fd00::/64")
			},
		},
		{
			name: "IPv4 prefix in IPv6Subnet",
			set: func(s *settings.Settings) {
				s.IPv6Subnet = netip.MustParsePrefix("10.1.0.0/24")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validClientConfiguration(t)
			tt.set(&cfg.UDPSettings)
			if err := validate(cfg); err == nil || !strings.Contains(err.Error(), "Subnet is not an IPv") {
				t.Fatalf("expected subnet family validation error, got %v", err)
			}
		})
	}
}
