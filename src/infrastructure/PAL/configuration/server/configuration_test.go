package server

import (
	"net/netip"
	"strings"
	"testing"

	"tungo/infrastructure/settings"
)

// mkValid returns a fully valid configuration ready for Validate().
// It uses NewDefaultConfiguration and ensures all protocols are enabled.
func mkValid() *Configuration {
	cfg := NewDefaultConfiguration()
	cfg.EnableTCP = true
	cfg.EnableUDP = true
	cfg.EnableWS = true

	// Keep default non-overlapping subnets and distinct ports:
	// TCP: 10.0.0.0/24 :8080
	// UDP: 10.0.1.0/24 :9090
	//  WS: 10.0.2.0/24 :1010
	return cfg
}

// --- Tests for defaultSettings and EnsureDefaults/applyDefaults ---

func TestConfiguration_DefaultSettingsValues(t *testing.T) {
	c := &Configuration{}

	tcp := c.defaultSettings(settings.TCP, "tcptun0", "10.0.0.0/24", "10.0.0.1", 8080)
	if tcp.InterfaceName != "tcptun0" ||
		tcp.IPv4Subnet.String() != "10.0.0.0/24" ||
		tcp.IPv4IP.String() != "10.0.0.1" ||
		tcp.Port != 8080 ||
		tcp.MTU != settings.DefaultEthernetMTU ||
		tcp.Protocol != settings.TCP ||
		tcp.Encryption != settings.ChaCha20Poly1305 ||
		tcp.DialTimeoutMs != 5000 {
		t.Fatalf("unexpected default TCP settings: %+v", tcp)
	}
	// IPv6 is opt-in — no defaults
	if tcp.IPv6Subnet.IsValid() || tcp.IPv6IP.IsValid() {
		t.Fatalf("IPv6 should not have defaults: %+v", tcp)
	}
}

func TestConfiguration_EnsureDefaults_FillsZeroFieldsOnly(t *testing.T) {
	// Start with empty settings so every field should be filled from defaults.
	c := &Configuration{}
	_ = c.EnsureDefaults()

	for _, tc := range []struct {
		name string
		s    settings.Settings
	}{
		{"TCP", c.TCPSettings},
		{"UDP", c.UDPSettings},
		{"WS", c.WSSettings},
	} {
		if tc.s.InterfaceName == "" ||
			!tc.s.IPv4Subnet.IsValid() ||
			!tc.s.IPv4IP.IsValid() ||
			tc.s.Port == 0 ||
			tc.s.MTU == 0 ||
			tc.s.Protocol == settings.UNKNOWN ||
			tc.s.DialTimeoutMs == 0 {
			t.Fatalf("EnsureDefaults did not fill %s zero fields: %+v", tc.name, tc.s)
		}
		// IPv6 is opt-in — EnsureDefaults must NOT populate it
		if tc.s.IPv6Subnet.IsValid() || tc.s.IPv6IP.IsValid() {
			t.Fatalf("EnsureDefaults should not set IPv6 defaults for %s: %+v", tc.name, tc.s)
		}
	}
}

func TestConfiguration_EnsureDefaults_DerivesIPv6IPFromSubnet(t *testing.T) {
	c := &Configuration{
		TCPSettings: settings.Settings{
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			// IPv6IP not set — should be derived as fd00::1
		},
	}
	_ = c.EnsureDefaults()

	if !c.TCPSettings.IPv6IP.IsValid() {
		t.Fatal("expected IPv6IP to be derived from IPv6Subnet")
	}
	if c.TCPSettings.IPv6IP != netip.MustParseAddr("fd00::1") {
		t.Fatalf("expected fd00::1, got %s", c.TCPSettings.IPv6IP)
	}
}

func TestConfiguration_EnsureDefaults_DoesNotOverrideExplicitFields(t *testing.T) {
	c := &Configuration{
		TCPSettings: settings.Settings{
			InterfaceName:    "custom0",
			IPv4Subnet:  netip.MustParsePrefix("10.9.0.0/24"),
			IPv4IP:      netip.MustParseAddr("10.9.0.1"),
			Port:             1234,
			MTU:              1400,
			Protocol:         settings.TCP,
			DialTimeoutMs:    2500,
			// Encryption is constant (ChaCha20Poly1305) in defaults; we keep it implicit here.
		},
	}
	_ = c.EnsureDefaults()

	// Ensure values were not overridden.
	if c.TCPSettings.InterfaceName != "custom0" ||
		c.TCPSettings.IPv4Subnet.String() != "10.9.0.0/24" ||
		c.TCPSettings.IPv4IP.String() != "10.9.0.1" ||
		c.TCPSettings.Port != 1234 ||
		c.TCPSettings.MTU != 1400 ||
		c.TCPSettings.Protocol != settings.TCP ||
		c.TCPSettings.DialTimeoutMs != 2500 {
		t.Fatalf("EnsureDefaults should not override explicit fields: %+v", c.TCPSettings)
	}
}

// --- Tests for Validate() happy-path and all error branches ---

func TestConfiguration_Validate_HappyPath_AllEnabled(t *testing.T) {
	cfg := mkValid()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got error: %v", err)
	}
}

func TestConfiguration_Validate_SkipsDisabledProtocol(t *testing.T) {
	cfg := mkValid()
	// Make WS invalid but disabled; Validate should ignore WS and still pass.
	cfg.EnableWS = false
	cfg.WSSettings.IPv4Subnet = netip.Prefix{} // invalid, but skipped
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config with WS disabled, got: %v", err)
	}
}

func TestConfiguration_Validate_InterfaceNameEmpty(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.InterfaceName = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for empty interface name")
	}
}

func TestConfiguration_Validate_InterfaceNameDuplicate(t *testing.T) {
	cfg := mkValid()
	cfg.UDPSettings.InterfaceName = cfg.TCPSettings.InterfaceName // duplicate
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for duplicate interface name")
	}
}

func TestConfiguration_Validate_ProtocolUnknown(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.Protocol = settings.UNKNOWN
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for UNKNOWN protocol")
	}
}

func TestConfiguration_Validate_PortOutOfRangeLow(t *testing.T) {
	cfg := mkValid()
	cfg.UDPSettings.Port = 0
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for port below range")
	}
}

func TestConfiguration_Validate_PortOutOfRangeHigh(t *testing.T) {
	cfg := mkValid()
	cfg.UDPSettings.Port = 70000
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for port above range")
	}
}

func TestConfiguration_Validate_PortDuplicateAcrossAll(t *testing.T) {
	cfg := mkValid()
	// Make UDP use same port as TCP; ports must be globally unique per your rule.
	cfg.UDPSettings.Port = cfg.TCPSettings.Port
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for duplicate port across protocols")
	}
}

func TestConfiguration_Validate_MTUTooSmall(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.MTU = 500 // below 576
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for MTU too small")
	}
}

func TestConfiguration_Validate_MTUTooLarge(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.MTU = 9500 // above 9000
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for MTU too large")
	}
}

func TestConfiguration_Validate_InvalidCIDR(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.IPv4Subnet = netip.Prefix{}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid CIDR")
	}
}

func TestConfiguration_Validate_InvalidAddress(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.IPv4IP = netip.Addr{}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid address")
	}
}

func TestConfiguration_Validate_AddressNotInCIDR(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.IPv4IP = netip.MustParseAddr("10.0.9.9") // not in 10.0.0.0/24
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for address not in CIDR")
	}
}

func TestConfiguration_Validate_SubnetOverlap(t *testing.T) {
	cfg := mkValid()
	// Force overlap: make UDP use same 10.0.0.0/24 as TCP.
	cfg.UDPSettings.IPv4Subnet = netip.MustParsePrefix("10.0.0.0/24")
	cfg.UDPSettings.IPv4IP = netip.MustParseAddr("10.0.0.2")
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for overlapping subnets")
	}
}

func TestConfiguration_Validate_IPv6IP_Invalid(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
	// IPv6IP left as zero value → invalid after Unmap
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "invalid 'IPv6IP'") {
		t.Fatalf("expected IPv6IP validation error, got: %v", err)
	}
}

func TestConfiguration_Validate_IPv6IP_NotInSubnet(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
	cfg.TCPSettings.IPv6IP = netip.MustParseAddr("fd01::99") // outside fd00::/64
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "not in 'IPv6Subnet'") {
		t.Fatalf("expected IPv6IP not-in-subnet error, got: %v", err)
	}
}

func TestConfiguration_Validate_IPv6_HappyPath(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
	cfg.TCPSettings.IPv6IP = netip.MustParseAddr("fd00::1")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config with IPv6, got: %v", err)
	}
}

func TestConfiguration_Validate_UnsupportedProtocol(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.Protocol = settings.Protocol(99)
	if err := cfg.Validate(); err == nil ||
		!strings.Contains(err.Error(), "unsupported protocol") {
		t.Fatalf("expected unsupported protocol error, got: %v", err)
	}
}

func TestConfiguration_OverlappingSubnets_NoOverlap(t *testing.T) {
	cfg := mkValid()
	subs := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/24"),
		netip.MustParsePrefix("10.0.1.0/24"),
	}
	if cfg.overlappingSubnets(subs) {
		t.Fatalf("expected no overlap, got true")
	}
}

// --- Tests for AllowedPeers validation ---

func TestConfiguration_ValidateAllowedPeers_ValidConfig(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey:    make([]byte, 32),
			Enabled:      true,
			ClientID:  5,
		},
		{
			PublicKey: func() []byte {
				k := make([]byte, 32)
				k[0] = 1
				return k
			}(),
			Enabled:     true,
			ClientID: 6,
		},
	}
	if err := cfg.ValidateAllowedPeers(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_InvalidKeyLength(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey:    make([]byte, 16), // Invalid: should be 32
			Enabled:      true,
			ClientID:  5,
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for invalid key length")
	}
	if !strings.Contains(err.Error(), "invalid public key length") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_InvalidClientID(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey:    make([]byte, 32),
			Enabled:      true,
			ClientID:  0, // invalid: must be > 0
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for invalid ClientID")
	}
	if !strings.Contains(err.Error(), "invalid ClientID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_MissingClientID(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey: make([]byte, 32),
			Enabled:   true,
			// ClientID defaults to 0 (zero value)
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for missing ClientID")
	}
	if !strings.Contains(err.Error(), "invalid ClientID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_DuplicatePublicKey(t *testing.T) {
	cfg := mkValid()
	pubKey := make([]byte, 32)
	pubKey[0] = 42
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey:    pubKey,
			Enabled:      true,
			ClientID:  5,
		},
		{
			PublicKey:    pubKey, // Duplicate
			Enabled:      true,
			ClientID:  6,
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for duplicate public key")
	}
	if !strings.Contains(err.Error(), "duplicate public key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_ClientIDConflict(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey:    make([]byte, 32),
			Enabled:      true,
			ClientID:  5,
		},
		{
			PublicKey: func() []byte {
				k := make([]byte, 32)
				k[0] = 1
				return k
			}(),
			Enabled:     true,
			ClientID: 5, // Same index as peer 0
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for ClientID conflict")
	}
	if !strings.Contains(err.Error(), "ClientID conflict") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_Validate_PropagatesValidateAllowedPeersError(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey:    make([]byte, 31), // invalid
			Enabled:      true,
			ClientID:  5,
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate to propagate ValidateAllowedPeers error")
	}
}
