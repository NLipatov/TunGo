package ip

import (
	"net/netip"
	"testing"
)

func TestAllocateServerIP_Success(t *testing.T) {
	// typical /24 network
	ip, err := AllocateServerIP(netip.MustParsePrefix("192.168.1.0/24"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

func TestAllocateServerIP_IPv6(t *testing.T) {
	ip, err := AllocateServerIP(netip.MustParsePrefix("fd00::/64"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ip != "fd00::1" {
		t.Errorf("expected fd00::1, got %s", ip)
	}
}

func TestAllocateClientIP_SuccessAndBounds(t *testing.T) {
	// /29 network: .0 network, .1 server, .2-.6 clients, .7 broadcast
	// clientID starts at 1 (confgen: ClientCounter+1)
	ip1, err := AllocateClientIP(netip.MustParsePrefix("10.0.0.0/29"), 1)
	if err != nil || ip1 != netip.MustParseAddr("10.0.0.2") {
		t.Errorf("counter 1: got %s, %v; want 10.0.0.2, nil", ip1, err)
	}
	ip5, err := AllocateClientIP(netip.MustParsePrefix("10.0.0.0/29"), 5)
	if err != nil || ip5 != netip.MustParseAddr("10.0.0.6") {
		t.Errorf("counter 5: got %s, %v; want 10.0.0.6, nil", ip5, err)
	}
}

func TestAllocateClientIP_RejectsZero(t *testing.T) {
	// clientID=0 would produce the server's address (base+1) — must be rejected
	_, err := AllocateClientIP(netip.MustParsePrefix("10.0.0.0/24"), 0)
	if err == nil {
		t.Fatal("expected error for clientCounter=0 (collides with server address)")
	}
}

func TestAllocateClientIP_OutOfRange(t *testing.T) {
	// /29 has 8 addrs: .0 network, .1 server, .2-.6 clients (counter 1-5), .7 broadcast
	_, err := AllocateClientIP(netip.MustParsePrefix("10.0.0.0/29"), 6)
	if err == nil || !contains(err.Error(), "out of range") {
		t.Errorf("expected out of range error, got %v", err)
	}
	// negative counter
	_, err = AllocateClientIP(netip.MustParsePrefix("10.0.0.0/24"), -1)
	if err == nil || !contains(err.Error(), "out of range") {
		t.Errorf("expected out of range error for -1, got %v", err)
	}
}

func TestAllocateClientIP_IPv6(t *testing.T) {
	// clientID=0 collides with server address — must be rejected
	_, err := AllocateClientIP(netip.MustParsePrefix("fd00::/64"), 0)
	if err == nil {
		t.Fatal("expected error for clientCounter=0 (collides with server address)")
	}

	ip1, err := AllocateClientIP(netip.MustParsePrefix("fd00::/64"), 1)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// offset = 1 + 1 = 2 → fd00::2
	if ip1 != netip.MustParseAddr("fd00::2") {
		t.Errorf("counter 1: got %s, want fd00::2", ip1)
	}
}

func TestAllocateClientIP_IPv6_NegativeCounter(t *testing.T) {
	_, err := AllocateClientIP(netip.MustParsePrefix("fd00::/64"), -1)
	if err == nil || !contains(err.Error(), "out of range") {
		t.Errorf("expected out of range error for -1, got %v", err)
	}
}

func TestToCIDR_Success(t *testing.T) {
	o, err := ToCIDR("192.0.2.0/24", "192.0.2.5")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if o != "192.0.2.5/24" {
		t.Errorf("expected 192.0.2.5/24, got %s", o)
	}
}

func TestToCIDR_Errors(t *testing.T) {
	// invalid subnet
	_, err := ToCIDR("notcidr", "192.0.2.5")
	if err == nil || !contains(err.Error(), "invalid subnet") {
		t.Errorf("expected invalid subnet error, got %v", err)
	}
	// invalid IP
	_, err = ToCIDR("192.0.2.0/24", "notip")
	if err == nil || !contains(err.Error(), "invalid IP address") {
		t.Errorf("expected invalid IP error, got %v", err)
	}
}

// contains reports whether s contains substr.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && ( /* simple: */ len(substr) == 0 || index(s, substr) >= 0)
}

// index returns the first index of substr in s, or -1.
func index(s, substr string) int {
	for i := range s {
		if len(s)-i < len(substr) {
			break
		}
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
