package session

import (
	"net/netip"
	"testing"
)

type sessionTestCrypto struct{}

func (d *sessionTestCrypto) Encrypt(b []byte) ([]byte, error) { return b, nil }
func (d *sessionTestCrypto) Decrypt(b []byte) ([]byte, error) { return b, nil }

func TestSessionAccessors(t *testing.T) {
	internal, _ := netip.ParseAddr("10.0.1.3")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	s := NewSession(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
	)

	if got := s.InternalAddr(); got != internal {
		t.Errorf("InternalAddr() = %v, want %v", got, internal)
	}
	if got := s.ExternalAddrPort(); got != external {
		t.Errorf("ExternalAddrPort() = %v, want %v", got, external)
	}
	if s.Crypto() == nil {
		t.Error("Crypto() should not be nil")
	}
}

func TestSession_IsSourceAllowed_ClientIP(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.5")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	s := NewSessionWithAuth(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
		[]byte("client-pub-key-placeholder32"),
		nil, // No additional AllowedIPs
	)

	// Internal IP should always be allowed
	if !s.IsSourceAllowed(internal) {
		t.Error("internal IP should be allowed")
	}

	// Different IP should not be allowed
	other := netip.MustParseAddr("10.0.0.99")
	if s.IsSourceAllowed(other) {
		t.Error("other IP should not be allowed")
	}
}

func TestSession_IsSourceAllowed_AdditionalPrefix(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.5")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	allowedIPs := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.0/24"),
	}

	s := NewSessionWithAuth(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
		[]byte("client-pub-key-placeholder32"),
		allowedIPs,
	)

	// IP in allowed prefix should be allowed
	inPrefix := netip.MustParseAddr("192.168.1.100")
	if !s.IsSourceAllowed(inPrefix) {
		t.Error("IP in allowed prefix should be allowed")
	}

	// IP outside prefix should not be allowed
	outside := netip.MustParseAddr("192.168.2.100")
	if s.IsSourceAllowed(outside) {
		t.Error("IP outside allowed prefix should not be allowed")
	}
}

func TestSession_IsSourceAllowed_MultipleRanges(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.5")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	allowedIPs := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.0/24"),
		netip.MustParsePrefix("172.16.0.0/16"),
		netip.MustParsePrefix("10.10.10.0/24"),
	}

	s := NewSessionWithAuth(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
		[]byte("client-pub-key-placeholder32"),
		allowedIPs,
	)

	tests := []struct {
		ip      string
		allowed bool
	}{
		{"10.0.0.5", true},     // Internal IP
		{"192.168.1.1", true},  // First prefix
		{"192.168.1.254", true},
		{"172.16.0.1", true},   // Second prefix
		{"172.16.255.1", true},
		{"10.10.10.50", true},  // Third prefix
		{"192.168.2.1", false}, // Outside all prefixes
		{"8.8.8.8", false},
	}

	for _, tc := range tests {
		ip := netip.MustParseAddr(tc.ip)
		if s.IsSourceAllowed(ip) != tc.allowed {
			t.Errorf("IsSourceAllowed(%s) = %v, want %v", tc.ip, !tc.allowed, tc.allowed)
		}
	}
}

func TestSession_IsSourceAllowed_IPv6(t *testing.T) {
	internal := netip.MustParseAddr("fd00::5")
	external, _ := netip.ParseAddrPort("[::1]:9000")

	allowedIPs := []netip.Prefix{
		netip.MustParsePrefix("2001:db8::/32"),
	}

	s := NewSessionWithAuth(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
		[]byte("client-pub-key-placeholder32"),
		allowedIPs,
	)

	// Internal IPv6 allowed
	if !s.IsSourceAllowed(internal) {
		t.Error("internal IPv6 should be allowed")
	}

	// IP in prefix allowed
	inPrefix := netip.MustParseAddr("2001:db8::1")
	if !s.IsSourceAllowed(inPrefix) {
		t.Error("IPv6 in allowed prefix should be allowed")
	}

	// IP outside prefix not allowed
	outside := netip.MustParseAddr("2001:db9::1")
	if s.IsSourceAllowed(outside) {
		t.Error("IPv6 outside allowed prefix should not be allowed")
	}
}

func TestSession_IsSourceAllowed_Denied(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.5")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	// Session with specific allowed IPs
	allowedIPs := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.0/24"),
	}

	s := NewSessionWithAuth(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
		[]byte("client-pub-key-placeholder32"),
		allowedIPs,
	)

	// Completely unrelated IP should be denied
	denied := netip.MustParseAddr("8.8.8.8")
	if s.IsSourceAllowed(denied) {
		t.Error("public IP should not be allowed")
	}
}

func TestSessionWithAuth_ClientPubKey(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.5")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")
	pubKey := []byte("test-client-public-key-32bytes!")

	s := NewSessionWithAuth(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
		pubKey,
		nil,
	)

	if string(s.ClientPubKey()) != string(pubKey) {
		t.Error("client pub key mismatch")
	}
}

func TestSessionWithAuth_AllowedAddrs_SingleHost(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.5")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")
	allowedIPs := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.1/32"),
	}

	s := NewSessionWithAuth(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
		nil,
		allowedIPs,
	)

	if len(s.AllowedAddrs()) != 1 {
		t.Errorf("expected 1 allowed addr, got %d", len(s.AllowedAddrs()))
	}
	if _, ok := s.AllowedAddrs()[netip.MustParseAddr("192.168.1.1")]; !ok {
		t.Error("expected 192.168.1.1 in allowed addrs")
	}
}

func TestSessionWithAuth_AllowedSubnet(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.5")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")
	allowedIPs := []netip.Prefix{
		netip.MustParsePrefix("192.168.0.0/16"),
	}

	s := NewSessionWithAuth(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
		nil,
		allowedIPs,
	)

	// Subnet goes to fallback, not to the addr map.
	if len(s.AllowedAddrs()) != 0 {
		t.Errorf("expected 0 allowed addrs for subnet, got %d", len(s.AllowedAddrs()))
	}
	if !s.IsSourceAllowed(netip.MustParseAddr("192.168.1.100")) {
		t.Error("address in subnet should be allowed")
	}
}

// TestSession_IsSourceAllowed_AllowedAddrsMapHit verifies that single-host /32
// AllowedIPs entries hit the O(1) allowedAddrs map path (not the internalIP check
// and not the subnet fallback).
func TestSession_IsSourceAllowed_AllowedAddrsMapHit(t *testing.T) {
	internal := netip.MustParseAddr("10.0.0.5")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	// /32 entries go into the allowedAddrs map, not allowedSubnets
	allowedIPs := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.1/32"),
		netip.MustParsePrefix("172.16.0.99/32"),
	}

	s := NewSessionWithAuth(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
		[]byte("client-pub-key-placeholder32"),
		allowedIPs,
	)

	// These IPs should be found via the allowedAddrs map (not internalIP, not subnet scan)
	if !s.IsSourceAllowed(netip.MustParseAddr("192.168.1.1")) {
		t.Error("192.168.1.1 should be allowed via allowedAddrs map")
	}
	if !s.IsSourceAllowed(netip.MustParseAddr("172.16.0.99")) {
		t.Error("172.16.0.99 should be allowed via allowedAddrs map")
	}

	// An IP NOT in the map and not internalIP should be denied
	if s.IsSourceAllowed(netip.MustParseAddr("192.168.1.2")) {
		t.Error("192.168.1.2 should NOT be allowed")
	}
}

// TestSession_IsSourceAllowed_IPv4MappedIPv6 verifies that IPv4-mapped-IPv6 addresses
// (e.g., ::ffff:10.0.0.5) are correctly normalized to IPv4 before comparison.
// This prevents false rejections when dual-stack clients send IPv4-mapped addresses.
func TestSession_IsSourceAllowed_IPv4MappedIPv6(t *testing.T) {
	// Session with pure IPv4 internal IP
	internal := netip.MustParseAddr("10.0.0.5")
	external, _ := netip.ParseAddrPort("93.184.216.34:9000")

	allowedIPs := []netip.Prefix{
		netip.MustParsePrefix("192.168.1.0/24"),
	}

	s := NewSessionWithAuth(
		&sessionTestCrypto{},
		nil,
		internal,
		external,
		[]byte("client-pub-key-placeholder32"),
		allowedIPs,
	)

	// IPv4-mapped-IPv6 version of internal IP should be allowed
	// ::ffff:10.0.0.5 should match 10.0.0.5 after normalization
	ipv4Mapped := netip.MustParseAddr("::ffff:10.0.0.5")
	if !s.IsSourceAllowed(ipv4Mapped) {
		t.Error("IPv4-mapped-IPv6 address (::ffff:10.0.0.5) should be allowed when internal IP is 10.0.0.5")
	}

	// IPv4-mapped-IPv6 in allowed prefix should also be allowed
	// ::ffff:192.168.1.100 should match 192.168.1.0/24 after normalization
	ipv4MappedInPrefix := netip.MustParseAddr("::ffff:192.168.1.100")
	if !s.IsSourceAllowed(ipv4MappedInPrefix) {
		t.Error("IPv4-mapped-IPv6 address (::ffff:192.168.1.100) should be allowed when 192.168.1.0/24 is in AllowedIPs")
	}

	// IPv4-mapped-IPv6 outside allowed range should be denied
	ipv4MappedOutside := netip.MustParseAddr("::ffff:8.8.8.8")
	if s.IsSourceAllowed(ipv4MappedOutside) {
		t.Error("IPv4-mapped-IPv6 address (::ffff:8.8.8.8) should be denied")
	}
}
