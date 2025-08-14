package packet_validation

import (
	"net"
	"testing"

	"tungo/infrastructure/network/ip"
)

// --- NormalizeIP ---

func TestNormalizeIP_Nil(t *testing.T) {
	v := NewDefaultPolicyNewIPValidator().(*DefaultIPValidator)
	_, _, err := v.NormalizeIP(nil)
	if err == nil {
		t.Fatalf("expected error for nil IP")
	}
}

func TestNormalizeIP_IPv4(t *testing.T) {
	v := NewDefaultPolicyNewIPValidator().(*DefaultIPValidator)
	in := net.ParseIP("192.168.1.10")
	ver, raw, err := v.NormalizeIP(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != ip.V4 {
		t.Fatalf("version = %v, want V4", ver)
	}
	if len(raw) != 4 {
		t.Fatalf("raw length = %d, want 4", len(raw))
	}
	if raw[0] != 192 || raw[1] != 168 || raw[2] != 1 || raw[3] != 10 {
		t.Fatalf("raw bytes = %v, want 192.168.1.10", raw)
	}
}

func TestNormalizeIP_IPv6(t *testing.T) {
	v := NewDefaultPolicyNewIPValidator().(*DefaultIPValidator)
	in := net.ParseIP("fd00::1")
	ver, raw, err := v.NormalizeIP(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ver != ip.V6 {
		t.Fatalf("version = %v, want V6", ver)
	}
	if len(raw) != 16 {
		t.Fatalf("raw length = %d, want 16", len(raw))
	}
	// quick sanity: first byte of fd00::/8 is 0xfd
	if raw[0] != 0xfd {
		t.Fatalf("raw[0] = 0x%x, want 0xfd", raw[0])
	}
}

func TestNormalizeIP_Invalid(t *testing.T) {
	v := NewDefaultPolicyNewIPValidator().(*DefaultIPValidator)
	// an invalid-length net.IP (neither 4 nor 16 bytes)
	bad := net.IP{1, 2, 3}
	_, _, err := v.NormalizeIP(bad)
	if err == nil {
		t.Fatalf("expected error for invalid IP")
	}
}

// --- Constructors coverage ---

func TestConstructors(t *testing.T) {
	// Custom policy via NewDefaultIPValidator
	p := Policy{
		AllowV4:           true,
		AllowV6:           false,
		RequirePrivate:    false,
		ForbidLoopback:    false,
		ForbidMulticast:   false,
		ForbidUnspecified: false,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: false,
	}
	v1 := NewDefaultIPValidator(p)
	if v1 == nil {
		t.Fatalf("NewDefaultIPValidator returned nil")
	}

	// Default policy via NewDefaultPolicyNewIPValidator must allow private v4/v6
	v2 := NewDefaultPolicyNewIPValidator()
	if v2 == nil {
		t.Fatalf("NewDefaultPolicyNewIPValidator returned nil")
	}
	if err := v2.ValidateIP(ip.V4, net.ParseIP("10.0.0.1")); err != nil {
		t.Fatalf("default policy should allow private v4: %v", err)
	}
	if err := v2.ValidateIP(ip.V6, net.ParseIP("fd00::1")); err != nil {
		t.Fatalf("default policy should allow ULA v6: %v", err)
	}
}

// --- ValidateIP: policy branches ---

func TestValidateIP_VersionNotAllowed(t *testing.T) {
	v4Only := NewDefaultIPValidator(Policy{
		AllowV4:           true,
		AllowV6:           false,
		RequirePrivate:    false,
		ForbidLoopback:    false,
		ForbidMulticast:   false,
		ForbidUnspecified: false,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: false,
	})
	if err := v4Only.ValidateIP(ip.V4, net.ParseIP("8.8.8.8")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := v4Only.ValidateIP(ip.V6, net.ParseIP("::1")); err == nil {
		t.Fatalf("expected error: ipv6 not allowed")
	}

	v6Only := NewDefaultIPValidator(Policy{
		AllowV4:           false,
		AllowV6:           true,
		RequirePrivate:    false,
		ForbidLoopback:    false,
		ForbidMulticast:   false,
		ForbidUnspecified: false,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: false,
	})
	if err := v6Only.ValidateIP(ip.V6, net.ParseIP("2001:4860:4860::8888")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := v6Only.ValidateIP(ip.V4, net.ParseIP("1.1.1.1")); err == nil {
		t.Fatalf("expected error: ipv4 not allowed")
	}
}

func TestValidateIP_LoopbackRejected(t *testing.T) {
	v := NewDefaultIPValidator(Policy{
		AllowV4:           true,
		AllowV6:           true,
		RequirePrivate:    false,
		ForbidLoopback:    true,
		ForbidMulticast:   false,
		ForbidUnspecified: false,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: false,
	})
	if err := v.ValidateIP(ip.V4, net.ParseIP("127.0.0.1")); err == nil {
		t.Fatalf("expected loopback rejection for 127.0.0.1")
	}
	if err := v.ValidateIP(ip.V6, net.ParseIP("::1")); err == nil {
		t.Fatalf("expected loopback rejection for ::1")
	}
}

func TestValidateIP_MulticastRejected(t *testing.T) {
	v := NewDefaultIPValidator(Policy{
		AllowV4:           true,
		AllowV6:           true,
		RequirePrivate:    false,
		ForbidLoopback:    false,
		ForbidMulticast:   true,
		ForbidUnspecified: false,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: false,
	})
	if err := v.ValidateIP(ip.V4, net.ParseIP("224.0.0.1")); err == nil {
		t.Fatalf("expected multicast rejection (v4)")
	}
	if err := v.ValidateIP(ip.V6, net.ParseIP("ff02::1")); err == nil {
		t.Fatalf("expected multicast rejection (v6)")
	}
}

func TestValidateIP_UnspecifiedRejected(t *testing.T) {
	v := NewDefaultIPValidator(Policy{
		AllowV4:           true,
		AllowV6:           true,
		RequirePrivate:    false,
		ForbidLoopback:    false,
		ForbidMulticast:   false,
		ForbidUnspecified: true,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: false,
	})
	if err := v.ValidateIP(ip.V4, net.ParseIP("0.0.0.0")); err == nil {
		t.Fatalf("expected unspecified rejection (v4)")
	}
	if err := v.ValidateIP(ip.V6, net.ParseIP("::")); err == nil {
		t.Fatalf("expected unspecified rejection (v6)")
	}
}

func TestValidateIP_LinkLocalRejected(t *testing.T) {
	v := NewDefaultIPValidator(Policy{
		AllowV4:           true,
		AllowV6:           true,
		RequirePrivate:    false,
		ForbidLoopback:    false,
		ForbidMulticast:   false,
		ForbidUnspecified: false,
		ForbidLinkLocal:   true,
		ForbidBroadcastV4: false,
	})
	if err := v.ValidateIP(ip.V4, net.ParseIP("169.254.1.1")); err == nil {
		t.Fatalf("expected link-local rejection (v4)")
	}
	if err := v.ValidateIP(ip.V6, net.ParseIP("fe80::1")); err == nil {
		t.Fatalf("expected link-local rejection (v6)")
	}
}

func TestValidateIP_BroadcastV4Rejected(t *testing.T) {
	v := NewDefaultIPValidator(Policy{
		AllowV4:           true,
		AllowV6:           true,
		RequirePrivate:    false,
		ForbidLoopback:    false,
		ForbidMulticast:   false,
		ForbidUnspecified: false,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: true,
	})
	if err := v.ValidateIP(ip.V4, net.IPv4bcast); err == nil {
		t.Fatalf("expected broadcast v4 rejection")
	}
}

func TestValidateIP_RequirePrivateRejectsPublic(t *testing.T) {
	v := NewDefaultIPValidator(Policy{
		AllowV4:           true,
		AllowV6:           true,
		RequirePrivate:    true,
		ForbidLoopback:    false,
		ForbidMulticast:   false,
		ForbidUnspecified: false,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: false,
	})
	if err := v.ValidateIP(ip.V4, net.ParseIP("8.8.8.8")); err == nil {
		t.Fatalf("expected rejection for non-private v4")
	}
	if err := v.ValidateIP(ip.V6, net.ParseIP("2001:4860:4860::8888")); err == nil {
		t.Fatalf("expected rejection for non-private v6")
	}
}

func TestValidateIP_AllAllowed_PublicOK(t *testing.T) {
	// Everything allowed, privacy not required: public IPs should pass.
	v := NewDefaultIPValidator(Policy{
		AllowV4:           true,
		AllowV6:           true,
		RequirePrivate:    false,
		ForbidLoopback:    false,
		ForbidMulticast:   false,
		ForbidUnspecified: false,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: false,
	})
	if err := v.ValidateIP(ip.V4, net.ParseIP("8.8.8.8")); err != nil {
		t.Fatalf("unexpected error for public v4: %v", err)
	}
	if err := v.ValidateIP(ip.V6, net.ParseIP("2001:4860:4860::8888")); err != nil {
		t.Fatalf("unexpected error for public v6: %v", err)
	}
}

func TestValidateIP_DefaultPolicy_SaneFailures(t *testing.T) {
	v := NewDefaultPolicyNewIPValidator()
	// Loopback must be rejected first (before private check).
	if err := v.ValidateIP(ip.V4, net.ParseIP("127.0.0.1")); err == nil {
		t.Fatalf("expected loopback rejection for default policy")
	}
	// Public should be rejected because RequirePrivate = true.
	if err := v.ValidateIP(ip.V4, net.ParseIP("1.1.1.1")); err == nil {
		t.Fatalf("expected public v4 rejection for default policy")
	}
}
