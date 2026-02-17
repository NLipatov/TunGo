package settings

import (
	"fmt"
	"net/netip"

	nip "tungo/infrastructure/network/ip"
)

// Addressing groups the network identity fields of a tunnel interface.
// IPv4 and IPv6 are derived at runtime from subnets and are not serialized.
type Addressing struct {
	TunName    string       `json:"TunName"`
	IPv4Subnet netip.Prefix `json:"IPv4Subnet,omitzero"`
	IPv6Subnet netip.Prefix `json:"IPv6Subnet,omitzero"`
	Server     Host         `json:"Server,omitzero"`
	Port       int          `json:"Port,omitzero"`
	DNSv4      []string     `json:"DNSv4,omitempty"`
	DNSv6      []string     `json:"DNSv6,omitempty"`

	// Derived at runtime — not serialized.
	IPv4 netip.Addr `json:"-"`
	IPv6 netip.Addr `json:"-"`
}

// DeriveIP populates IPv4/IPv6 from subnets.
// clientID == 0 means server (first usable address); clientID > 0 means client.
func (a *Addressing) DeriveIP(clientID int) error {
	if a.IPv4Subnet.IsValid() {
		ip, err := allocateIP(a.IPv4Subnet, clientID)
		if err != nil {
			return fmt.Errorf("derive IPv4: %w", err)
		}
		a.IPv4 = ip
	}
	if a.IPv6Subnet.IsValid() {
		ip, err := allocateIP(a.IPv6Subnet, clientID)
		if err != nil {
			return fmt.Errorf("derive IPv6: %w", err)
		}
		a.IPv6 = ip
	}
	return nil
}

func allocateIP(subnet netip.Prefix, clientID int) (netip.Addr, error) {
	if clientID == 0 {
		s, err := nip.AllocateServerIP(subnet)
		if err != nil {
			return netip.Addr{}, err
		}
		return netip.MustParseAddr(s), nil
	}
	return nip.AllocateClientIP(subnet, clientID)
}

func (a Addressing) HasIPv4() bool { return a.IPv4.IsValid() }
func (a Addressing) HasIPv6() bool { return a.IPv6.IsValid() }

func (a Addressing) IsZero() bool {
	return a.TunName == "" &&
		!a.IPv4Subnet.IsValid() &&
		!a.IPv6Subnet.IsValid() &&
		a.Server.IsZero() &&
		a.Port == 0 &&
		len(a.DNSv4) == 0 &&
		len(a.DNSv6) == 0 &&
		!a.IPv4.IsValid() &&
		!a.IPv6.IsValid()
}

func (a Addressing) DNSv4Resolvers() []string {
	if len(a.DNSv4) == 0 {
		return append([]string(nil), DefaultClientDNSv4Resolvers...)
	}
	return append([]string(nil), a.DNSv4...)
}

func (a Addressing) DNSv6Resolvers() []string {
	if len(a.DNSv6) == 0 {
		return append([]string(nil), DefaultClientDNSv6Resolvers...)
	}
	return append([]string(nil), a.DNSv6...)
}

// IPv4CIDR returns the IPv4 address combined with the subnet prefix length, e.g. "10.0.0.2/24".
func (a Addressing) IPv4CIDR() (string, error) {
	if !a.IPv4.IsValid() {
		return "", fmt.Errorf("no IPv4 address")
	}
	if !a.IPv4Subnet.IsValid() {
		return "", fmt.Errorf("no IPv4 subnet")
	}
	return netip.PrefixFrom(a.IPv4.Unmap(), a.IPv4Subnet.Bits()).String(), nil
}

// IPv6CIDR returns the IPv6 address combined with the subnet prefix length, e.g. "fd00::2/64".
func (a Addressing) IPv6CIDR() (string, error) {
	if !a.IPv6.IsValid() {
		return "", fmt.Errorf("no IPv6 address")
	}
	if !a.IPv6Subnet.IsValid() {
		return "", fmt.Errorf("no IPv6 subnet")
	}
	return netip.PrefixFrom(a.IPv6.Unmap(), a.IPv6Subnet.Bits()).String(), nil
}

// WithIPv6Subnet returns a copy with the IPv6Subnet field set.
func (a Addressing) WithIPv6Subnet(subnet netip.Prefix) Addressing {
	a.IPv6Subnet = subnet
	return a
}
