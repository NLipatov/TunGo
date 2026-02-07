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
		tcp.InterfaceSubnet.String() != "10.0.0.0/24" ||
		tcp.InterfaceIP.String() != "10.0.0.1" ||
		tcp.Port != 8080 ||
		tcp.MTU != settings.DefaultEthernetMTU ||
		tcp.Protocol != settings.TCP ||
		tcp.Encryption != settings.ChaCha20Poly1305 ||
		tcp.DialTimeoutMs != 5000 {
		t.Fatalf("unexpected default TCP settings: %+v", tcp)
	}

	udp := c.defaultSettings(settings.UDP, "udptun0", "10.0.1.0/24", "10.0.1.1", 9090)
	if udp.InterfaceName != "udptun0" ||
		udp.InterfaceSubnet.String() != "10.0.1.0/24" ||
		udp.InterfaceIP.String() != "10.0.1.1" ||
		udp.Port != 9090 ||
		udp.MTU != settings.DefaultEthernetMTU ||
		udp.Protocol != settings.UDP ||
		udp.Encryption != settings.ChaCha20Poly1305 ||
		udp.DialTimeoutMs != 5000 {
		t.Fatalf("unexpected default UDP settings: %+v", udp)
	}

	ws := c.defaultSettings(settings.WS, "wstun0", "10.0.2.0/24", "10.0.2.1", 1010)
	if ws.InterfaceName != "wstun0" ||
		ws.InterfaceSubnet.String() != "10.0.2.0/24" ||
		ws.InterfaceIP.String() != "10.0.2.1" ||
		ws.Port != 1010 ||
		ws.MTU != settings.DefaultEthernetMTU ||
		ws.Protocol != settings.WS ||
		ws.Encryption != settings.ChaCha20Poly1305 ||
		ws.DialTimeoutMs != 5000 {
		t.Fatalf("unexpected default WS settings: %+v", ws)
	}
}

func TestConfiguration_EnsureDefaults_FillsZeroFieldsOnly(t *testing.T) {
	// Start with empty settings so every field should be filled from defaults.
	c := &Configuration{}
	_ = c.EnsureDefaults()

	// TCP
	if c.TCPSettings.InterfaceName == "" ||
		!c.TCPSettings.InterfaceSubnet.IsValid() ||
		!c.TCPSettings.InterfaceIP.IsValid() ||
		c.TCPSettings.Port == 0 ||
		c.TCPSettings.MTU == 0 ||
		c.TCPSettings.Protocol == settings.UNKNOWN ||
		c.TCPSettings.DialTimeoutMs == 0 {
		t.Fatalf("EnsureDefaults did not fill TCP zero fields: %+v", c.TCPSettings)
	}

	// UDP
	if c.UDPSettings.InterfaceName == "" ||
		!c.UDPSettings.InterfaceSubnet.IsValid() ||
		!c.UDPSettings.InterfaceIP.IsValid() ||
		c.UDPSettings.Port == 0 ||
		c.UDPSettings.MTU == 0 ||
		c.UDPSettings.Protocol == settings.UNKNOWN ||
		c.UDPSettings.DialTimeoutMs == 0 {
		t.Fatalf("EnsureDefaults did not fill UDP zero fields: %+v", c.UDPSettings)
	}

	// WS
	if c.WSSettings.InterfaceName == "" ||
		!c.WSSettings.InterfaceSubnet.IsValid() ||
		!c.WSSettings.InterfaceIP.IsValid() ||
		c.WSSettings.Port == 0 ||
		c.WSSettings.MTU == 0 ||
		c.WSSettings.Protocol == settings.UNKNOWN ||
		c.WSSettings.DialTimeoutMs == 0 {
		t.Fatalf("EnsureDefaults did not fill WS zero fields: %+v", c.WSSettings)
	}
}

func TestConfiguration_EnsureDefaults_DoesNotOverrideExplicitFields(t *testing.T) {
	c := &Configuration{
		TCPSettings: settings.Settings{
			InterfaceName:    "custom0",
			InterfaceSubnet:  netip.MustParsePrefix("10.9.0.0/24"),
			InterfaceIP:      netip.MustParseAddr("10.9.0.1"),
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
		c.TCPSettings.InterfaceSubnet.String() != "10.9.0.0/24" ||
		c.TCPSettings.InterfaceIP.String() != "10.9.0.1" ||
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
	cfg.WSSettings.InterfaceSubnet = netip.Prefix{} // invalid, but skipped
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
	cfg.TCPSettings.InterfaceSubnet = netip.Prefix{}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid CIDR")
	}
}

func TestConfiguration_Validate_InvalidAddress(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.InterfaceIP = netip.Addr{}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid address")
	}
}

func TestConfiguration_Validate_AddressNotInCIDR(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.InterfaceIP = netip.MustParseAddr("10.0.9.9") // not in 10.0.0.0/24
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for address not in CIDR")
	}
}

func TestConfiguration_Validate_SubnetOverlap(t *testing.T) {
	cfg := mkValid()
	// Force overlap: make UDP use same 10.0.0.0/24 as TCP.
	cfg.UDPSettings.InterfaceSubnet = netip.MustParsePrefix("10.0.0.0/24")
	cfg.UDPSettings.InterfaceIP = netip.MustParseAddr("10.0.0.2")
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for overlapping subnets")
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
			PublicKey: make([]byte, 32),
			Enabled:   true,
			Address:   netip.MustParseAddr("10.0.0.5"),
		},
		{
			PublicKey: func() []byte {
				k := make([]byte, 32)
				k[0] = 1
				return k
			}(),
			Enabled: true,
			Address: netip.MustParseAddr("10.0.0.6"),
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
			PublicKey: make([]byte, 16), // Invalid: should be 32
			Enabled:   true,
			Address:   netip.MustParseAddr("10.0.0.5"),
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

func TestConfiguration_ValidateAllowedPeers_InvalidClientIP(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey: make([]byte, 32),
			Enabled:   true,
			Address:   netip.Addr{},
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for invalid Address")
	}
	if !strings.Contains(err.Error(), "invalid Address") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_MissingAddress(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey: make([]byte, 32),
			Enabled:   true,
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for missing Address")
	}
	if !strings.Contains(err.Error(), "invalid Address") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_DuplicatePublicKey(t *testing.T) {
	cfg := mkValid()
	pubKey := make([]byte, 32)
	pubKey[0] = 42
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey: pubKey,
			Enabled:   true,
			Address:   netip.MustParseAddr("10.0.0.5"),
		},
		{
			PublicKey: pubKey, // Duplicate
			Enabled:   true,
			Address:   netip.MustParseAddr("10.0.0.6"),
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

func TestConfiguration_ValidateAllowedPeers_ClientIPOverlap(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey: make([]byte, 32),
			Enabled:   true,
			Address:   netip.MustParseAddr("10.0.0.5"),
		},
		{
			PublicKey: func() []byte {
				k := make([]byte, 32)
				k[0] = 1
				return k
			}(),
			Enabled: true,
			Address: netip.MustParseAddr("10.0.0.5"), // Same address as peer 0
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for address conflict")
	}
	if !strings.Contains(err.Error(), "Address conflict") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_Validate_PropagatesValidateAllowedPeersError(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey: make([]byte, 31), // invalid
			Enabled:   true,
			Address:   netip.MustParseAddr("10.0.0.5"),
		},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Validate to propagate ValidateAllowedPeers error")
	}
}

func TestConfiguration_ValidateAllowedPeers_ClientIPNotInEnabledSubnets(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey: make([]byte, 32),
			Enabled:   true,
			Address:   netip.MustParseAddr("172.16.0.1"), // outside 10.0.x enabled subnets
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil || !strings.Contains(err.Error(), "not within any enabled interface subnet") {
		t.Fatalf("expected out-of-subnet error, got %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_IPv6ClientIP(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.InterfaceSubnet = netip.MustParsePrefix("2001:db8::/64")
	cfg.TCPSettings.InterfaceIP = netip.MustParseAddr("2001:db8::1")
	cfg.EnableUDP = false
	cfg.EnableWS = false
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey: make([]byte, 32),
			Enabled:   true,
			Address:   netip.MustParseAddr("2001:db8::42"),
		},
	}
	if err := cfg.ValidateAllowedPeers(); err != nil {
		t.Fatalf("expected valid IPv6 clientIP, got %v", err)
	}
}

func TestConfiguration_isClientIPInSubnet_False(t *testing.T) {
	cfg := mkValid()
	subnets := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/24"),
	}
	if cfg.isClientIPInSubnet(netip.MustParseAddr("10.0.1.5"), subnets) {
		t.Fatal("expected false for IP outside subnet")
	}
}
