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

// --- Tests for default*Settings and EnsureDefaults/applyDefaults ---

func TestConfiguration_DefaultSettingsValues(t *testing.T) {
	c := &Configuration{}

	tcp := c.defaultTCPSettings()
	if tcp.InterfaceName != "tcptun0" ||
		tcp.InterfaceIPCIDR != "10.0.0.0/24" ||
		tcp.InterfaceAddress != "10.0.0.1" ||
		tcp.Port != "8080" ||
		tcp.MTU != settings.DefaultEthernetMTU ||
		tcp.Protocol != settings.TCP ||
		tcp.Encryption != settings.ChaCha20Poly1305 ||
		tcp.DialTimeoutMs != 5000 {
		t.Fatalf("unexpected default TCP settings: %+v", tcp)
	}

	udp := c.defaultUDPSettings()
	if udp.InterfaceName != "udptun0" ||
		udp.InterfaceIPCIDR != "10.0.1.0/24" ||
		udp.InterfaceAddress != "10.0.1.1" ||
		udp.Port != "9090" ||
		udp.MTU != settings.DefaultEthernetMTU ||
		udp.Protocol != settings.UDP ||
		udp.Encryption != settings.ChaCha20Poly1305 ||
		udp.DialTimeoutMs != 5000 {
		t.Fatalf("unexpected default UDP settings: %+v", udp)
	}

	ws := c.defaultWSSettings()
	if ws.InterfaceName != "wstun0" ||
		ws.InterfaceIPCIDR != "10.0.2.0/24" ||
		ws.InterfaceAddress != "10.0.2.1" ||
		ws.Port != "1010" ||
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
		c.TCPSettings.InterfaceIPCIDR == "" ||
		c.TCPSettings.InterfaceAddress == "" ||
		c.TCPSettings.Port == "" ||
		c.TCPSettings.MTU == 0 ||
		c.TCPSettings.Protocol == settings.UNKNOWN ||
		c.TCPSettings.DialTimeoutMs == 0 {
		t.Fatalf("EnsureDefaults did not fill TCP zero fields: %+v", c.TCPSettings)
	}

	// UDP
	if c.UDPSettings.InterfaceName == "" ||
		c.UDPSettings.InterfaceIPCIDR == "" ||
		c.UDPSettings.InterfaceAddress == "" ||
		c.UDPSettings.Port == "" ||
		c.UDPSettings.MTU == 0 ||
		c.UDPSettings.Protocol == settings.UNKNOWN ||
		c.UDPSettings.DialTimeoutMs == 0 {
		t.Fatalf("EnsureDefaults did not fill UDP zero fields: %+v", c.UDPSettings)
	}

	// WS
	if c.WSSettings.InterfaceName == "" ||
		c.WSSettings.InterfaceIPCIDR == "" ||
		c.WSSettings.InterfaceAddress == "" ||
		c.WSSettings.Port == "" ||
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
			InterfaceIPCIDR:  "10.9.0.0/24",
			InterfaceAddress: "10.9.0.1",
			Port:             "1234",
			MTU:              1400,
			Protocol:         settings.TCP,
			DialTimeoutMs:    2500,
			// Encryption is constant (ChaCha20Poly1305) in defaults; we keep it implicit here.
		},
	}
	_ = c.EnsureDefaults()

	// Ensure values were not overridden.
	if c.TCPSettings.InterfaceName != "custom0" ||
		c.TCPSettings.InterfaceIPCIDR != "10.9.0.0/24" ||
		c.TCPSettings.InterfaceAddress != "10.9.0.1" ||
		c.TCPSettings.Port != "1234" ||
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
	cfg.WSSettings.InterfaceIPCIDR = "10.0.0.0/33" // invalid, but skipped
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

func TestConfiguration_Validate_PortNotNumber(t *testing.T) {
	cfg := mkValid()
	cfg.UDPSettings.Port = "abc"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for non-numeric port")
	}
}

func TestConfiguration_Validate_PortOutOfRangeLow(t *testing.T) {
	cfg := mkValid()
	cfg.UDPSettings.Port = "0"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for port below range")
	}
}

func TestConfiguration_Validate_PortOutOfRangeHigh(t *testing.T) {
	cfg := mkValid()
	cfg.UDPSettings.Port = "70000"
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
	cfg.TCPSettings.InterfaceIPCIDR = "10.0.0.0/33"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid CIDR")
	}
}

func TestConfiguration_Validate_InvalidAddress(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.InterfaceAddress = "bad.ip.addr"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid address")
	}
}

func TestConfiguration_Validate_AddressNotInCIDR(t *testing.T) {
	cfg := mkValid()
	cfg.TCPSettings.InterfaceAddress = "10.0.9.9" // not in 10.0.0.0/24
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for address not in CIDR")
	}
}

func TestConfiguration_Validate_SubnetOverlap(t *testing.T) {
	cfg := mkValid()
	// Force overlap: make UDP use same 10.0.0.0/24 as TCP.
	cfg.UDPSettings.InterfaceIPCIDR = "10.0.0.0/24"
	cfg.UDPSettings.InterfaceAddress = "10.0.0.2"
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
			ClientIP:  "10.0.0.5",
		},
		{
			PublicKey: func() []byte {
				k := make([]byte, 32)
				k[0] = 1
				return k
			}(),
			Enabled:  true,
			ClientIP: "10.0.0.6",
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
			ClientIP:  "10.0.0.5",
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
			ClientIP:  "invalid.ip",
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for invalid ClientIP")
	}
	if !strings.Contains(err.Error(), "invalid ClientIP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_InvalidAllowedIP(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey:  make([]byte, 32),
			Enabled:    true,
			ClientIP:   "10.0.0.5",
			AllowedIPs: []string{"192.168.1.0/33"}, // Invalid CIDR
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for invalid AllowedIP")
	}
	if !strings.Contains(err.Error(), "invalid AllowedIP") {
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
			ClientIP:  "10.0.0.5",
		},
		{
			PublicKey: pubKey, // Duplicate
			Enabled:   true,
			ClientIP:  "10.0.0.6",
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
			ClientIP:  "10.0.0.5",
		},
		{
			PublicKey: func() []byte {
				k := make([]byte, 32)
				k[0] = 1
				return k
			}(),
			Enabled:  true,
			ClientIP: "10.0.0.5", // Same ClientIP as peer 0
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for ClientIP overlap")
	}
	if !strings.Contains(err.Error(), "AllowedIPs overlap") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_AllowedIPsOverlap(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey:  make([]byte, 32),
			Enabled:    true,
			ClientIP:   "10.0.0.5",
			AllowedIPs: []string{"192.168.0.0/16"},
		},
		{
			PublicKey: func() []byte {
				k := make([]byte, 32)
				k[0] = 1
				return k
			}(),
			Enabled:    true,
			ClientIP:   "10.0.0.6",
			AllowedIPs: []string{"192.168.1.0/24"}, // Overlaps with peer 0's 192.168.0.0/16
		},
	}
	err := cfg.ValidateAllowedPeers()
	if err == nil {
		t.Fatal("expected error for AllowedIPs overlap")
	}
	if !strings.Contains(err.Error(), "AllowedIPs overlap") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfiguration_ValidateAllowedPeers_SamePeerAllowedIPsOverlapOk(t *testing.T) {
	cfg := mkValid()
	cfg.AllowedPeers = []AllowedPeer{
		{
			PublicKey: make([]byte, 32),
			Enabled:   true,
			ClientIP:  "10.0.0.5",
			AllowedIPs: []string{
				"192.168.0.0/16",
				"192.168.1.0/24", // Overlaps within same peer - OK
			},
		},
	}
	if err := cfg.ValidateAllowedPeers(); err != nil {
		t.Fatalf("expected no error for same-peer overlap, got: %v", err)
	}
}

func TestAllowedPeer_AllowedIPPrefixes(t *testing.T) {
	peer := AllowedPeer{
		PublicKey: make([]byte, 32),
		Enabled:   true,
		ClientIP:  "10.0.0.5",
		AllowedIPs: []string{
			"192.168.1.0/24",
			"10.10.0.0/16",
			"invalid/cidr", // Should be skipped
		},
	}

	prefixes := peer.AllowedIPPrefixes()
	if len(prefixes) != 2 {
		t.Fatalf("expected 2 valid prefixes, got %d", len(prefixes))
	}
	if prefixes[0].String() != "192.168.1.0/24" {
		t.Fatalf("unexpected first prefix: %s", prefixes[0])
	}
	if prefixes[1].String() != "10.10.0.0/16" {
		t.Fatalf("unexpected second prefix: %s", prefixes[1])
	}
}
