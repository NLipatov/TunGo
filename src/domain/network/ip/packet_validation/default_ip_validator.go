package packet_validation

import (
	"fmt"
	"net"
	"tungo/infrastructure/network/ip"
)

type DefaultIPValidator struct {
	policy Policy
}

func NewDefaultIPValidator(policy Policy) IPValidator {
	return &DefaultIPValidator{
		policy: policy,
	}
}

func NewDefaultPolicyNewIPValidator() IPValidator {
	return &DefaultIPValidator{
		policy: Policy{
			AllowV4:           true,
			AllowV6:           true,
			RequirePrivate:    true,
			ForbidLoopback:    true,
			ForbidMulticast:   true,
			ForbidUnspecified: true,
			ForbidLinkLocal:   true,
			ForbidBroadcastV4: true,
		},
	}
}

// NormalizeIP returns the IP version and raw bytes in canonical form.
// IPv4 addresses are 4 bytes, IPv6 addresses are 16 bytes. No mapped forms are allowed.
func (i *DefaultIPValidator) NormalizeIP(netIp net.IP) (ver ip.Version, raw []byte, err error) {
	if netIp == nil {
		return 0, nil, fmt.Errorf("ip is nil")
	}
	if ip4 := netIp.To4(); ip4 != nil {
		return ip.V4, append([]byte(nil), ip4...), nil
	}
	if ip16 := netIp.To16(); ip16 != nil {
		// ip.To4() != nil has already been processed, so this is a pure IPv6 address
		return ip.V6, append([]byte(nil), ip16...), nil
	}
	return 0, nil, fmt.Errorf("invalid ip: %v", netIp)
}

// ValidateIP checks if the normalized IP matches the provided validation policy.
func (i *DefaultIPValidator) ValidateIP(ver ip.Version, netIP net.IP) error {
	// Version check
	if ver == ip.V4 && !i.policy.AllowV4 {
		return fmt.Errorf("ipv4 not allowed")
	}
	if ver == ip.V6 && !i.policy.AllowV6 {
		return fmt.Errorf("ipv6 not allowed")
	}

	// General restrictions
	if i.policy.ForbidLoopback && netIP.IsLoopback() {
		return fmt.Errorf("loopback not allowed: %s", netIP)
	}
	if i.policy.ForbidMulticast && netIP.IsMulticast() {
		return fmt.Errorf("multicast not allowed: %s", netIP)
	}
	if i.policy.ForbidUnspecified && netIP.IsUnspecified() {
		return fmt.Errorf("unspecified not allowed: %s", netIP)
	}
	if i.policy.ForbidLinkLocal && (netIP.IsLinkLocalUnicast() || netIP.IsLinkLocalMulticast()) {
		return fmt.Errorf("link-local not allowed: %s", netIP)
	}

	// IPv4-specific rules
	if ver == ip.V4 {
		if i.policy.ForbidBroadcastV4 && netIP.Equal(net.IPv4bcast) {
			return fmt.Errorf("broadcast not allowed: %s", netIP)
		}
	}

	// Privacy check (RFC1918 and ULA)
	if i.policy.RequirePrivate && !netIP.IsPrivate() {
		return fmt.Errorf("non-private not allowed: %s", netIP)
	}
	return nil
}
