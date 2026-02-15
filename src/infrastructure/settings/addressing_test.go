package settings

import (
	"net/netip"
	"testing"
)

func TestDeriveIP_Server(t *testing.T) {
	a := Addressing{
		IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
		IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
	}
	if err := a.DeriveIP(0); err != nil {
		t.Fatalf("DeriveIP(0): %v", err)
	}
	if a.IPv4 != netip.MustParseAddr("10.0.0.1") {
		t.Fatalf("IPv4: want 10.0.0.1, got %s", a.IPv4)
	}
	if a.IPv6 != netip.MustParseAddr("fd00::1") {
		t.Fatalf("IPv6: want fd00::1, got %s", a.IPv6)
	}
}

func TestDeriveIP_Client(t *testing.T) {
	a := Addressing{
		IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
		IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
	}
	if err := a.DeriveIP(3); err != nil {
		t.Fatalf("DeriveIP(3): %v", err)
	}
	if a.IPv4 != netip.MustParseAddr("10.0.0.4") {
		t.Fatalf("IPv4: want 10.0.0.4, got %s", a.IPv4)
	}
	if a.IPv6 != netip.MustParseAddr("fd00::4") {
		t.Fatalf("IPv6: want fd00::4, got %s", a.IPv6)
	}
}

func TestDeriveIP_IPv4Only(t *testing.T) {
	a := Addressing{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")}
	if err := a.DeriveIP(1); err != nil {
		t.Fatalf("DeriveIP(1): %v", err)
	}
	if !a.HasIPv4() {
		t.Fatal("expected HasIPv4")
	}
	if a.HasIPv6() {
		t.Fatal("expected no IPv6")
	}
}

func TestDeriveIP_NoSubnets(t *testing.T) {
	a := Addressing{}
	if err := a.DeriveIP(0); err != nil {
		t.Fatalf("DeriveIP on empty: %v", err)
	}
	if a.HasIPv4() || a.HasIPv6() {
		t.Fatal("expected no IPs on empty Addressing")
	}
}

func TestDeriveIP_InvalidSubnet(t *testing.T) {
	a := Addressing{IPv4Subnet: netip.Prefix{}}
	if err := a.DeriveIP(0); err != nil {
		t.Fatal("invalid prefix should be skipped, not error")
	}
}

func TestIPv4CIDR(t *testing.T) {
	a := Addressing{
		IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
		IPv4:       netip.MustParseAddr("10.0.0.2"),
	}
	cidr, err := a.IPv4CIDR()
	if err != nil {
		t.Fatalf("IPv4CIDR: %v", err)
	}
	if cidr != "10.0.0.2/24" {
		t.Fatalf("want 10.0.0.2/24, got %s", cidr)
	}
}

func TestIPv6CIDR(t *testing.T) {
	a := Addressing{
		IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
		IPv6:       netip.MustParseAddr("fd00::2"),
	}
	cidr, err := a.IPv6CIDR()
	if err != nil {
		t.Fatalf("IPv6CIDR: %v", err)
	}
	if cidr != "fd00::2/64" {
		t.Fatalf("want fd00::2/64, got %s", cidr)
	}
}

func TestIPv4CIDR_NoAddr(t *testing.T) {
	a := Addressing{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")}
	if _, err := a.IPv4CIDR(); err == nil {
		t.Fatal("expected error for missing IPv4")
	}
}

func TestIPv6CIDR_NoAddr(t *testing.T) {
	a := Addressing{IPv6Subnet: netip.MustParsePrefix("fd00::/64")}
	if _, err := a.IPv6CIDR(); err == nil {
		t.Fatal("expected error for missing IPv6")
	}
}

func TestIsZero(t *testing.T) {
	if !(Addressing{}).IsZero() {
		t.Fatal("expected zero Addressing to be IsZero")
	}
	a := Addressing{TunName: "tun0"}
	if a.IsZero() {
		t.Fatal("non-zero Addressing should not be IsZero")
	}
}

func TestWithIPv6Subnet(t *testing.T) {
	a := Addressing{TunName: "tun0"}
	subnet := netip.MustParsePrefix("fd00::/64")
	b := a.WithIPv6Subnet(subnet)
	if b.IPv6Subnet != subnet {
		t.Fatalf("WithIPv6Subnet: want %s, got %s", subnet, b.IPv6Subnet)
	}
	if a.IPv6Subnet.IsValid() {
		t.Fatal("original should not be mutated")
	}
}
