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

func TestAllocateServerIP_IPv6Rejected(t *testing.T) {
	_, err := AllocateServerIP(netip.MustParsePrefix("2001:db8::/32"))
	if err == nil || !contains(err.Error(), "only IPv4 supported") {
		t.Errorf("expected IPv4 error, got %v", err)
	}
}

func TestAllocateClientIP_SuccessAndBounds(t *testing.T) {
	// /30 network has 2 hosts: .0 network, .1 first host, .2 second host, .3 broadcast
	// so available hosts = 2 (counter 0 -> .1, counter 1 -> .2)
	ip0, err := AllocateClientIP(netip.MustParsePrefix("10.0.0.0/30"), 0)
	if err != nil || ip0 != netip.MustParseAddr("10.0.0.1") {
		t.Errorf("counter 0: got %s, %v; want 10.0.0.1, nil", ip0, err)
	}
	ip1, err := AllocateClientIP(netip.MustParsePrefix("10.0.0.0/30"), 1)
	if err != nil || ip1 != netip.MustParseAddr("10.0.0.2") {
		t.Errorf("counter 1: got %s, %v; want 10.0.0.2, nil", ip1, err)
	}
}

func TestAllocateClientIP_OutOfRange(t *testing.T) {
	// /30 network has only 2 clients
	_, err := AllocateClientIP(netip.MustParsePrefix("10.0.0.0/30"), 2)
	if err == nil || !contains(err.Error(), "client counter exceeds") {
		t.Errorf("expected counter exceeds error, got %v", err)
	}
	// negative counter
	_, err = AllocateClientIP(netip.MustParsePrefix("10.0.0.0/24"), -1)
	if err == nil || !contains(err.Error(), "client counter exceeds") {
		t.Errorf("expected counter exceeds error for -1, got %v", err)
	}
}

func TestAllocateClientIP_IPv6Rejected(t *testing.T) {
	_, err := AllocateClientIP(netip.MustParsePrefix("2001:db8::/64"), 0)
	if err == nil || !contains(err.Error(), "only IPv4 supported") {
		t.Errorf("expected IPv4 error, got %v", err)
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
