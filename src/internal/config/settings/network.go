package settings

import (
	"fmt"
	"net/netip"

	nip "tungo/internal/config/addressing"
)

// Network groups the network-related fields of a tunnel configuration.
// IPv4 and IPv6 are derived at runtime from subnets and are not serialized.
type Network struct {
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
func (n *Network) DeriveIP(clientID int) error {
	if n.IPv4Subnet.IsValid() {
		ip, err := allocateIP(n.IPv4Subnet, clientID)
		if err != nil {
			return fmt.Errorf("derive IPv4: %w", err)
		}
		n.IPv4 = ip
	}
	if n.IPv6Subnet.IsValid() {
		ip, err := allocateIP(n.IPv6Subnet, clientID)
		if err != nil {
			return fmt.Errorf("derive IPv6: %w", err)
		}
		n.IPv6 = ip
	}
	return nil
}

// allocateIP allocates an IP address from subnet for the server when clientID is zero, or for the specified client otherwise.
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

func (n Network) HasIPv4() bool { return n.IPv4.IsValid() }
func (n Network) HasIPv6() bool { return n.IPv6.IsValid() }

func (n Network) IsZero() bool {
	return n.TunName == "" &&
		!n.IPv4Subnet.IsValid() &&
		!n.IPv6Subnet.IsValid() &&
		n.Server == (Host{}) &&
		n.Port == 0 &&
		len(n.DNSv4) == 0 &&
		len(n.DNSv6) == 0 &&
		!n.IPv4.IsValid() &&
		!n.IPv6.IsValid()
}

// IPv4CIDR returns the IPv4 address combined with the subnet prefix length, e.g. "10.0.0.2/24".
func (n Network) IPv4CIDR() (string, error) {
	if !n.IPv4.IsValid() {
		return "", fmt.Errorf("no IPv4 address")
	}
	if !n.IPv4Subnet.IsValid() {
		return "", fmt.Errorf("no IPv4 subnet")
	}
	return netip.PrefixFrom(n.IPv4.Unmap(), n.IPv4Subnet.Bits()).String(), nil
}

// IPv6CIDR returns the IPv6 address combined with the subnet prefix length, e.g. "fd00::2/64".
func (n Network) IPv6CIDR() (string, error) {
	if !n.IPv6.IsValid() {
		return "", fmt.Errorf("no IPv6 address")
	}
	if !n.IPv6Subnet.IsValid() {
		return "", fmt.Errorf("no IPv6 subnet")
	}
	return netip.PrefixFrom(n.IPv6.Unmap(), n.IPv6Subnet.Bits()).String(), nil
}

// WithIPv6Subnet returns a copy with the IPv6Subnet field set.
func (n Network) WithIPv6Subnet(subnet netip.Prefix) Network {
	n.IPv6Subnet = subnet
	return n
}
