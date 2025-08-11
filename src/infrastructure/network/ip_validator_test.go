package network

import (
	"net"
	"testing"
	"tungo/domain/network/ip"
)

// --- Helpers ---

func mustIP(s string) net.IP {
	p := net.ParseIP(s)
	if p == nil {
		panic("invalid test IP: " + s)
	}
	return p
}

// --- Coverage boosters ---

func TestNewDefaultPolicyNewIPValidator_Behavior(t *testing.T) {
	// Ensures constructor is executed and its policy is enforced.
	raw := NewDefaultPolicyNewIPValidator()
	v, ok := raw.(*IPValidator)
	if !ok || v == nil {
		t.Fatalf("expected *IPValidator, got %T", raw)
	}

	type tc struct {
		name string
		ver  ip.Version
		ip   net.IP
		ok   bool
	}
	cases := []tc{
		// Allowed by default (RequirePrivate=true, various forbids=true)
		{"allow private v4", ip.V4, net.IPv4(10, 0, 0, 1).To4(), true},
		{"allow ULA v6", ip.V6, mustIP("fd00::1"), true},

		// Must be rejected by default policy
		{"reject public v4", ip.V4, net.IPv4(8, 8, 8, 8).To4(), false},
		{"reject loopback v4", ip.V4, net.IPv4(127, 0, 0, 1).To4(), false},
		{"reject unspecified v4", ip.V4, net.IPv4(0, 0, 0, 0).To4(), false},
		{"reject linklocal v4", ip.V4, net.IPv4(169, 254, 1, 1).To4(), false},
		{"reject broadcast v4", ip.V4, net.IPv4(255, 255, 255, 255).To4(), false},
		{"reject multicast v4", ip.V4, net.IPv4(224, 0, 0, 1).To4(), false},

		{"reject public v6", ip.V6, mustIP("2001:db8::1"), false},
		{"reject loopback v6", ip.V6, mustIP("::1"), false},
		{"reject unspecified v6", ip.V6, mustIP("::"), false},
		{"reject linklocal v6", ip.V6, mustIP("fe80::1"), false},
		{"reject linklocal multicast v6", ip.V6, mustIP("ff02::fb"), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := v.ValidateIP(c.ver, c.ip)
			if c.ok && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !c.ok && err == nil {
				t.Fatalf("expected error, got nil")
			}
		})
	}
}

func TestValidateIP_BroadcastV4Allowed_WhenNotForbidden(t *testing.T) {
	// When ForbidBroadcastV4=false AND RequirePrivate=false,
	// 255.255.255.255 should be allowed.
	val := NewIPValidator(ip.ValidationPolicy{
		AllowV4:           true,
		RequirePrivate:    false,
		ForbidLoopback:    false,
		ForbidMulticast:   false,
		ForbidUnspecified: false,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: false, // key difference
	}).(*IPValidator)

	bcast := net.IPv4(255, 255, 255, 255).To4()
	if err := val.ValidateIP(ip.V4, bcast); err != nil {
		t.Fatalf("broadcast should be allowed when not forbidden: %v", err)
	}
}

func TestValidateIP_MulticastAllowed_WhenNotForbidden(t *testing.T) {
	// When ForbidMulticast=false AND RequirePrivate=false,
	// multicast should pass.
	val := NewIPValidator(ip.ValidationPolicy{
		AllowV4:           true,
		RequirePrivate:    false,
		ForbidLoopback:    false,
		ForbidMulticast:   false, // key difference
		ForbidUnspecified: false,
		ForbidLinkLocal:   false,
		ForbidBroadcastV4: false,
	}).(*IPValidator)

	mc := net.IPv4(239, 1, 2, 3).To4()
	if err := val.ValidateIP(ip.V4, mc); err != nil {
		t.Fatalf("multicast should be allowed when not forbidden: %v", err)
	}
}

func TestValidateIP_LinkLocalAllowed_WhenNotForbidden(t *testing.T) {
	// When ForbidLinkLocal=false AND RequirePrivate=false,
	// IPv4 link-local 169.254.0.0/16 should pass.
	val := NewIPValidator(ip.ValidationPolicy{
		AllowV4:           true,
		RequirePrivate:    false, // link-local is not "private" per net.IP.IsPrivate()
		ForbidLoopback:    false,
		ForbidMulticast:   false,
		ForbidUnspecified: false,
		ForbidLinkLocal:   false, // key difference
		ForbidBroadcastV4: false,
	}).(*IPValidator)

	ll := net.IPv4(169, 254, 10, 10).To4()
	if err := val.ValidateIP(ip.V4, ll); err != nil {
		t.Fatalf("link-local should be allowed when not forbidden: %v", err)
	}
}
