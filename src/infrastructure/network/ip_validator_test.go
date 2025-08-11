package network

import (
	"bytes"
	"net"
	"testing"

	"tungo/domain/network/ip"
)

// mustIP is a helper that panics on invalid literal IP.
// It keeps tests concise and focused.
func mustIP(s string) net.IP {
	p := net.ParseIP(s)
	if p == nil {
		panic("invalid test IP: " + s)
	}
	return p
}

func TestNewIPValidator_ReturnsConcrete(t *testing.T) {
	// Smoke test that constructor works and returns a non-nil validator.
	v := NewIPValidator(ip.ValidationPolicy{})
	if v == nil {
		t.Fatalf("NewIPValidator returned nil")
	}
}

func TestNormalizeIP_Nil(t *testing.T) {
	val := &IPValidator{}
	_, _, err := val.NormalizeIP(nil)
	if err == nil {
		t.Fatalf("expected error for nil IP")
	}
}

func TestNormalizeIP_IPv4(t *testing.T) {
	val := &IPValidator{}
	ver, raw, err := val.NormalizeIP(mustIP("192.0.2.1")) // TEST-NET-1 address
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != ip.V4 {
		t.Fatalf("expected V4, got %v", ver)
	}
	if len(raw) != net.IPv4len {
		t.Fatalf("expected 4 raw bytes, got %d", len(raw))
	}
	if !bytes.Equal(raw, net.IPv4(192, 0, 2, 1).To4()) {
		t.Fatalf("unexpected raw bytes: %v", raw)
	}
}

func TestNormalizeIP_IPv4Mapped_ReturnsV4Canonical(t *testing.T) {
	val := &IPValidator{}
	// ::ffff:192.0.2.1 — should normalize to pure v4 (no mapped forms allowed)
	mapped := net.ParseIP("::ffff:192.0.2.1")
	if mapped == nil {
		t.Fatalf("failed to parse mapped v4")
	}
	ver, raw, err := val.NormalizeIP(mapped)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != ip.V4 {
		t.Fatalf("expected V4 for mapped address, got %v", ver)
	}
	if len(raw) != net.IPv4len {
		t.Fatalf("expected 4 bytes, got %d", len(raw))
	}
	if !bytes.Equal(raw, net.IPv4(192, 0, 2, 1).To4()) {
		t.Fatalf("unexpected raw bytes for mapped: %v", raw)
	}
}

func TestNormalizeIP_IPv6(t *testing.T) {
	val := &IPValidator{}
	ver, raw, err := val.NormalizeIP(mustIP("2001:db8::1")) // documentation prefix
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != ip.V6 {
		t.Fatalf("expected V6, got %v", ver)
	}
	if len(raw) != net.IPv6len {
		t.Fatalf("expected 16 raw bytes, got %d", len(raw))
	}
	if net.IP(raw).String() != "2001:db8::1" {
		t.Fatalf("unexpected canonical: %s", net.IP(raw))
	}
}

func TestNormalizeIP_InvalidBytes(t *testing.T) {
	val := &IPValidator{}
	// Construct an invalid net.IP of wrong length to force error.
	bad := net.IP([]byte{1, 2, 3}) // neither To4 nor To16 will match canonical lengths
	_, _, err := val.NormalizeIP(bad)
	if err == nil {
		t.Fatalf("expected error for invalid IP backing bytes")
	}
}

func TestValidateIP_VersionDisallowed(t *testing.T) {
	// Disallow IPv4
	val := NewIPValidator(ip.ValidationPolicy{AllowV4: false, AllowV6: true}).(*IPValidator)
	if err := val.ValidateIP(ip.V4, net.IPv4(1, 1, 1, 1)); err == nil {
		t.Fatalf("expected error when IPv4 not allowed")
	}

	// Disallow IPv6
	val = NewIPValidator(ip.ValidationPolicy{AllowV4: true, AllowV6: false}).(*IPValidator)
	if err := val.ValidateIP(ip.V6, mustIP("2001:db8::1")); err == nil {
		t.Fatalf("expected error when IPv6 not allowed")
	}
}

func TestValidateIP_GeneralRestrictions(t *testing.T) {
	type tc struct {
		name   string
		ipStr  string
		ver    ip.Version
		pol    ip.ValidationPolicy
		should bool // should error
	}

	tests := []tc{
		{
			name:   "loopback v4",
			ipStr:  "127.0.0.1",
			ver:    ip.V4,
			pol:    ip.ValidationPolicy{AllowV4: true, ForbidLoopback: true},
			should: true,
		},
		{
			name:   "multicast v4",
			ipStr:  "224.0.0.1",
			ver:    ip.V4,
			pol:    ip.ValidationPolicy{AllowV4: true, ForbidMulticast: true},
			should: true,
		},
		{
			name:   "unspecified v4",
			ipStr:  "0.0.0.0",
			ver:    ip.V4,
			pol:    ip.ValidationPolicy{AllowV4: true, ForbidUnspecified: true},
			should: true,
		},
		{
			name:   "link-local unicast v4",
			ipStr:  "169.254.1.1",
			ver:    ip.V4,
			pol:    ip.ValidationPolicy{AllowV4: true, ForbidLinkLocal: true},
			should: true,
		},
		{
			name:   "broadcast v4",
			ipStr:  "255.255.255.255",
			ver:    ip.V4,
			pol:    ip.ValidationPolicy{AllowV4: true, ForbidBroadcastV4: true},
			should: true,
		},
		{
			name:   "loopback v6",
			ipStr:  "::1",
			ver:    ip.V6,
			pol:    ip.ValidationPolicy{AllowV6: true, ForbidLoopback: true},
			should: true,
		},
		{
			name:   "multicast v6",
			ipStr:  "ff02::1",
			ver:    ip.V6,
			pol:    ip.ValidationPolicy{AllowV6: true, ForbidMulticast: true},
			should: true,
		},
		{
			name:   "unspecified v6",
			ipStr:  "::",
			ver:    ip.V6,
			pol:    ip.ValidationPolicy{AllowV6: true, ForbidUnspecified: true},
			should: true,
		},
		{
			name:   "link-local unicast v6",
			ipStr:  "fe80::1",
			ver:    ip.V6,
			pol:    ip.ValidationPolicy{AllowV6: true, ForbidLinkLocal: true},
			should: true,
		},
		{
			name:   "link-local multicast v6",
			ipStr:  "ff02::fb",
			ver:    ip.V6,
			pol:    ip.ValidationPolicy{AllowV6: true, ForbidLinkLocal: true},
			should: true,
		},
		{
			name:   "allowed non-restricted v4",
			ipStr:  "8.8.8.8",
			ver:    ip.V4,
			pol:    ip.ValidationPolicy{AllowV4: true},
			should: false,
		},
		{
			name:   "allowed non-restricted v6",
			ipStr:  "2001:4860:4860::8888",
			ver:    ip.V6,
			pol:    ip.ValidationPolicy{AllowV6: true},
			should: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val := NewIPValidator(tt.pol).(*IPValidator)

			netIP := mustIP(tt.ipStr)
			// Feed canonical byte form consistent with the version hint.
			if tt.ver == ip.V4 {
				netIP = netIP.To4()
			} else {
				netIP = netIP.To16()
			}

			err := val.ValidateIP(tt.ver, netIP)
			if tt.should && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.should && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateIP_Privacy(t *testing.T) {
	// Public IPv4 should fail when RequirePrivate is set.
	val := NewIPValidator(ip.ValidationPolicy{AllowV4: true, RequirePrivate: true}).(*IPValidator)
	if err := val.ValidateIP(ip.V4, net.IPv4(8, 8, 8, 8).To4()); err == nil {
		t.Fatalf("expected error for public IPv4 when RequirePrivate")
	}

	// Private IPv4 should pass when RequirePrivate is set.
	val = NewIPValidator(ip.ValidationPolicy{AllowV4: true, RequirePrivate: true}).(*IPValidator)
	if err := val.ValidateIP(ip.V4, net.IPv4(10, 0, 0, 1).To4()); err != nil {
		t.Fatalf("unexpected error for private IPv4 when RequirePrivate: %v", err)
	}

	// Public IPv6 should fail when RequirePrivate is set (IPv6 ULA is fc00::/7).
	val = NewIPValidator(ip.ValidationPolicy{AllowV6: true, RequirePrivate: true}).(*IPValidator)
	if err := val.ValidateIP(ip.V6, mustIP("2001:db8::1")); err == nil {
		t.Fatalf("expected error for public IPv6 when RequirePrivate")
	}

	// IPv6 ULA (Unique Local Address) should pass when RequirePrivate is set.
	val = NewIPValidator(ip.ValidationPolicy{AllowV6: true, RequirePrivate: true}).(*IPValidator)
	if err := val.ValidateIP(ip.V6, mustIP("fd00::1")); err != nil {
		t.Fatalf("unexpected error for ULA IPv6 when RequirePrivate: %v", err)
	}
}
