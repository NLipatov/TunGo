package ip

import (
	"testing"
)

func TestAllocateServerIp_Success(t *testing.T) {
	// typical /24 network
	ip, err := AllocateServerIp("192.168.1.0/24")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if ip != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %s", ip)
	}
}

func TestAllocateServerIp_Errors(t *testing.T) {
	cases := []struct{ in, wantErr string }{
		{"not a cidr", "invalid subnet"},
		{"2001:db8::/32", "only IPv4 supported"},
	}
	for _, c := range cases {
		_, err := AllocateServerIp(c.in)
		if err == nil || !contains(err.Error(), c.wantErr) {
			t.Errorf("AllocateServerIp(%q) error = %v, want contains %q", c.in, err, c.wantErr)
		}
	}
}

func TestAllocateClientIp_SuccessAndBounds(t *testing.T) {
	// /30 network has 2 hosts: .0 network, .1 first host, .2 second host, .3 broadcast
	// so available hosts = 2 (counter 0 -> .1, counter 1 -> .2)
	ip0, err := AllocateClientIp("10.0.0.0/30", 0)
	if err != nil || ip0 != "10.0.0.1" {
		t.Errorf("counter 0: got %s, %v; want 10.0.0.1, nil", ip0, err)
	}
	ip1, err := AllocateClientIp("10.0.0.0/30", 1)
	if err != nil || ip1 != "10.0.0.2" {
		t.Errorf("counter 1: got %s, %v; want 10.0.0.2, nil", ip1, err)
	}
}

func TestAllocateClientIp_OutOfRange(t *testing.T) {
	// /30 network has only 2 clients
	_, err := AllocateClientIp("10.0.0.0/30", 2)
	if err == nil || !contains(err.Error(), "client counter exceeds") {
		t.Errorf("expected counter exceeds error, got %v", err)
	}
	// negative counter
	_, err = AllocateClientIp("10.0.0.0/24", -1)
	if err == nil || !contains(err.Error(), "client counter exceeds") {
		t.Errorf("expected counter exceeds error for -1, got %v", err)
	}
}

func TestAllocateClientIp_InvalidInputs(t *testing.T) {
	// invalid CIDR
	_, err := AllocateClientIp("foo", 0)
	if err == nil || !contains(err.Error(), "invalid subnet") {
		t.Errorf("expected invalid subnet error, got %v", err)
	}
	// IPv6 not supported
	_, err = AllocateClientIp("2001:db8::/64", 0)
	if err == nil || !contains(err.Error(), "only IPv4 supported") {
		t.Errorf("expected IPv4 supported error, got %v", err)
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
