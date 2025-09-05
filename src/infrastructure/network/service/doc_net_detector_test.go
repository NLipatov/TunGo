package service

import (
	"net/netip"
	"testing"
)

func TestFrameDetector_HostIsInServiceNetwork_Table(t *testing.T) {
	fd := NewDocNetDetector()

	tests := []struct {
		name     string
		addr     string
		wantTrue bool
	}{
		// RFC 5737 TEST-NET-1
		{"IPv4_TestNet1_First", "192.0.2.0", true},
		{"IPv4_TestNet1_Middle", "192.0.2.123", true},
		{"IPv4_TestNet1_Last", "192.0.2.255", true},

		// RFC 5737 TEST-NET-2
		{"IPv4_TestNet2_First", "198.51.100.0", true},
		{"IPv4_TestNet2_Middle", "198.51.100.77", true},
		{"IPv4_TestNet2_Last", "198.51.100.255", true},

		// RFC 5737 TEST-NET-3
		{"IPv4_TestNet3_First", "203.0.113.0", true},
		{"IPv4_TestNet3_Middle", "203.0.113.200", true},
		{"IPv4_TestNet3_Last", "203.0.113.255", true},

		// RFC 3849 2001:db8::/32
		{"IPv6_Db8_First", "2001:db8::", true},
		{"IPv6_Db8_Middle", "2001:db8:abcd:1::42", true},
		{"IPv6_Db8_Last", "2001:db8:ffff:ffff:ffff:ffff:ffff:ffff", true},

		// Outside ranges (negative cases)
		{"IPv4_Outside_Private", "10.0.0.1", false},
		{"IPv4_Outside_PublicAdjacent1", "192.0.3.1", false},    // just outside 192.0.2.0/24
		{"IPv4_Outside_PublicAdjacent2", "198.51.101.1", false}, // just outside 198.51.100.0/24
		{"IPv4_Outside_PublicAdjacent3", "203.0.114.1", false},  // just outside 203.0.113.0/24
		{"IPv6_Outside_Public", "2001:db9::1", false},           // outside 2001:db8::/32
		{"IPv6_Outside_LinkLocal", "fe80::1", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addr, addrErr := netip.ParseAddr(tc.addr)
			if addrErr != nil {
				t.Fatalf("failed to parse address %q", tc.addr)
			}
			got := fd.IsInDocNet(addr)
			if got != tc.wantTrue {
				t.Errorf("IsInDocNet(%q) = %v, want %v", tc.addr, got, tc.wantTrue)
			}
		})
	}
}

func TestFrameDetector_HostIsInServiceNetwork_IPv4MappedIPv6(t *testing.T) {
	fd := NewDocNetDetector()

	// IPv4-mapped IPv6 should be recognized after Unmap()
	addr := netip.MustParseAddr("::ffff:198.51.100.123")
	got := fd.IsInDocNet(addr)
	if !got {
		t.Fatalf("expected true for IPv4-mapped TEST-NET address, got false")
	}

	// Non-test IPv4 mapped should be false
	addr = netip.MustParseAddr("::ffff:203.0.114.1")
	got = fd.IsInDocNet(addr)
	if got {
		t.Fatalf("expected false for IPv4-mapped non-TEST-NET address, got true")
	}
}

func TestFrameDetector_PrefixesInitialized(t *testing.T) {
	fd := NewDocNetDetector()
	// Spot-check one prefix
	want := netip.MustParsePrefix(testNetTwo)
	if fd.testNetPrefixes[1] != want {
		t.Errorf("expected prefix[1]=%v, got %v", want, fd.testNetPrefixes[1])
	}
}

func BenchmarkFrameDetector_HostIsInServiceNetwork(b *testing.B) {
	fd := NewDocNetDetector()
	addresses := []netip.Addr{
		netip.MustParseAddr("192.0.2.1"),            // hit
		netip.MustParseAddr("198.51.100.200"),       // hit
		netip.MustParseAddr("203.0.113.7"),          // hit
		netip.MustParseAddr("2001:db8:1::1"),        // hit
		netip.MustParseAddr("8.8.8.8"),              // miss
		netip.MustParseAddr("1.1.1.1"),              // miss
		netip.MustParseAddr("2001:4860:4860::8888"), // miss
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		addr := addresses[i%len(addresses)]
		_ = fd.IsInDocNet(addr)
	}
}
