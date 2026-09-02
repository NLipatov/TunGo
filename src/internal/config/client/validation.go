package client

import (
	"fmt"
	"net/netip"
	"strings"
	"unicode"

	"tungo/internal/config/settings"
)

// validate checks that the client configuration contains valid identifiers, keys, active settings, network addresses, subnets, MTU, and DNS servers. It returns the first validation error encountered.
func validate(configuration Configuration) error {
	if configuration.ClientID <= 0 {
		return fmt.Errorf("invalid ClientID %d: must be > 0", configuration.ClientID)
	}
	if len(configuration.ClientPublicKey) != 32 {
		return fmt.Errorf("invalid ClientPublicKey length %d, expected 32", len(configuration.ClientPublicKey))
	}
	if len(configuration.ClientPrivateKey) != 32 {
		return fmt.Errorf("invalid ClientPrivateKey length %d, expected 32", len(configuration.ClientPrivateKey))
	}
	if len(configuration.X25519PublicKey) != 32 {
		return fmt.Errorf("invalid X25519PublicKey (server) length %d, expected 32", len(configuration.X25519PublicKey))
	}
	active, err := configuration.ActiveSettings()
	if err != nil {
		return err
	}
	if strings.TrimSpace(active.TunName) == "" {
		return fmt.Errorf("active settings: TunName is not configured")
	}
	if strings.IndexFunc(active.TunName, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.In(r, unicode.Cf)
	}) >= 0 {
		return fmt.Errorf("active settings: TunName contains unsupported characters")
	}
	if active.Server == (settings.Host{}) {
		return fmt.Errorf("active settings: Server is not configured")
	}
	if err := validateServerHost(active.Server); err != nil {
		return fmt.Errorf("active settings: %w", err)
	}
	if active.Port < 1 || active.Port > 65535 {
		// WSS: zero means "use default 443" in connection factory.
		if configuration.Protocol != settings.WSS || active.Port != 0 {
			return fmt.Errorf("active settings: invalid Port %d", active.Port)
		}
	}
	hasIPv4 := active.IPv4Subnet.IsValid() && active.IPv4Subnet.Addr().Is4()
	hasIPv6 := active.IPv6Subnet.IsValid() && active.IPv6Subnet.Addr().Unmap().Is6()
	if active.IPv4Subnet.IsValid() && !hasIPv4 {
		return fmt.Errorf("active settings: IPv4Subnet is not an IPv4 prefix")
	}
	if active.IPv6Subnet.IsValid() && !hasIPv6 {
		return fmt.Errorf("active settings: IPv6Subnet is not an IPv6 prefix")
	}
	if !hasIPv4 && !hasIPv6 {
		return fmt.Errorf("active settings: both IPv4Subnet and IPv6Subnet are invalid")
	}
	minimumMTU := settings.MinimumIPv4MTU
	if hasIPv6 {
		minimumMTU = settings.MinimumIPv6MTU
	}
	if active.MTU < minimumMTU || active.MTU > settings.MaximumMTU {
		return fmt.Errorf("active settings: invalid MTU %d: expected %d..%d", active.MTU, minimumMTU, settings.MaximumMTU)
	}
	if err := validateDNSv4Servers(active.DNSv4); err != nil {
		return fmt.Errorf("active settings: %w", err)
	}
	if err := validateDNSv6Servers(active.DNSv6); err != nil {
		return fmt.Errorf("active settings: %w", err)
	}
	return nil
}

func validateServerHost(host settings.Host) error {
	if host.IPv4 != "" {
		ip, err := netip.ParseAddr(host.IPv4)
		if err != nil || !ip.Unmap().Is4() {
			return fmt.Errorf("invalid server IPv4 %q", host.IPv4)
		}
	}
	if host.IPv6 != "" {
		ip, err := netip.ParseAddr(host.IPv6)
		if err != nil || ip.Unmap().Is4() {
			return fmt.Errorf("invalid server IPv6 %q", host.IPv6)
		}
	}
	return nil
}

func validateDNSv4Servers(servers []string) error {
	for i, raw := range servers {
		resolver := strings.TrimSpace(raw)
		if resolver == "" {
			return fmt.Errorf("DNSv4[%d] is empty", i)
		}
		addr, err := netip.ParseAddr(resolver)
		if err != nil {
			return fmt.Errorf("DNSv4[%d] %q is not an IP address", i, raw)
		}
		if !addr.Is4() {
			return fmt.Errorf("DNSv4[%d] %q is IPv6, expected IPv4", i, raw)
		}
	}
	return nil
}

func validateDNSv6Servers(servers []string) error {
	for i, raw := range servers {
		resolver := strings.TrimSpace(raw)
		if resolver == "" {
			return fmt.Errorf("DNSv6[%d] is empty", i)
		}
		addr, err := netip.ParseAddr(resolver)
		if err != nil {
			return fmt.Errorf("DNSv6[%d] %q is not an IP address", i, raw)
		}
		if !addr.Is6() || addr.Is4In6() {
			return fmt.Errorf("DNSv6[%d] %q is IPv4, expected IPv6", i, raw)
		}
	}
	return nil
}
