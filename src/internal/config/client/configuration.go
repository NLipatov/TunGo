package client

import (
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strings"

	"tungo/internal/config/settings"
)

type Configuration struct {
	ClientID        int               `json:"ClientID"`
	TCPSettings     settings.Settings `json:"TCPSettings"`
	UDPSettings     settings.Settings `json:"UDPSettings"`
	WSSettings      settings.Settings `json:"WSSettings"`
	X25519PublicKey []byte            `json:"X25519PublicKey"`
	Protocol        settings.Protocol `json:"Protocol"`

	// Client identity for Noise IK handshake.
	// ClientPublicKey MUST match the PublicKey in server's AllowedPeers entry.
	ClientPublicKey []byte `json:"ClientPublicKey"`

	// ClientPrivateKey is the client's X25519 static private key (32 bytes).
	// MUST derive ClientPublicKey when processed with X25519.
	ClientPrivateKey []byte   `json:"ClientPrivateKey"`
	AllowedIPsv4     []string `json:"AllowedIPsv4"`
	AllowedIPsv6     []string `json:"AllowedIPsv6"`
}

func (c *Configuration) applyDefaults() {
	activeSettings, err := c.selectedSettings()
	if err != nil {
		return
	}
	dnsV4, dnsV6 := effectiveDNS(activeSettings.Network)
	allowedV4, allowedV6 := effectiveAllowedIPs(
		c.AllowedIPsv4, c.AllowedIPsv6,
		dnsV4, dnsV6,
		activeSettings.IPv4Subnet, activeSettings.IPv6Subnet,
	)
	mtu := effectiveMTU(*activeSettings, c.Protocol)

	activeSettings.DNSv4, activeSettings.DNSv6 = dnsV4, dnsV6
	c.AllowedIPsv4, c.AllowedIPsv6 = allowedV4, allowedV6
	activeSettings.MTU = mtu
}

func effectiveDNS(network settings.Network) ([]string, []string) {
	defaultv4 := []string{"1.1.1.1", "8.8.8.8"}
	defaultv6 := []string{"2606:4700:4700::1111", "2001:4860:4860::8888"}
	v4, v6 := network.DNSv4, network.DNSv6
	if network.IPv4Subnet.IsValid() && network.IPv4Subnet.Addr().Is4() && len(v4) == 0 {
		v4 = defaultv4
		slog.Info("client DNSv4 defaults applied", "effective", v4)
	}
	if network.IPv6Subnet.IsValid() && network.IPv6Subnet.Addr().Unmap().Is6() && len(v6) == 0 {
		v6 = defaultv6
		slog.Info("client DNSv6 defaults applied", "effective", v6)
	}
	return v4, v6
}

// effectiveAllowedIPs replaces each missing or invalid family list with full-tunnel defaults.
// Valid /0 prefixes expand into two /1 routes to preserve the system default route.
// DNS routes are added unless covered by a split or the TUN subnet.
func effectiveAllowedIPs(
	v4, v6 []string,
	dnsV4, dnsV6 []string,
	tunSubnetV4, tunSubnetV6 netip.Prefix,
) ([]string, []string) {
	configuredV4, configuredV6 := v4, v6
	defaultV4 := []string{"0.0.0.0/1", "128.0.0.0/1"}
	defaultV6 := []string{"::/1", "8000::/1"}
	if v4 == nil {
		v4 = defaultV4
	}
	if v6 == nil {
		v6 = defaultV6
	}
	seen := make(map[string]struct{}, len(v4)+len(v6))
	normalizedV4 := make([]string, 0, len(v4))
	for _, cidr := range v4 {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil || !prefix.Addr().Is4() {
			normalizedV4 = defaultV4
			break
		}
		routes := []string{prefix.Masked().String()}
		if prefix.Bits() == 0 {
			routes = defaultV4
		}
		for _, route := range routes {
			if _, ok := seen[route]; ok {
				continue
			}
			seen[route] = struct{}{}
			normalizedV4 = append(normalizedV4, route)
		}
	}

	normalizedV6 := make([]string, 0, len(v6))
	for _, cidr := range v6 {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil || prefix.Addr().Unmap().Is4() {
			normalizedV6 = defaultV6
			break
		}
		routes := []string{prefix.Masked().String()}
		if prefix.Bits() == 0 {
			routes = defaultV6
		}
		for _, route := range routes {
			if _, ok := seen[route]; ok {
				continue
			}
			seen[route] = struct{}{}
			normalizedV6 = append(normalizedV6, route)
		}
	}
	if tunSubnetV4.IsValid() && tunSubnetV4.Addr().Is4() {
		normalizedV4 = withDNSRoutes(normalizedV4, dnsV4, tunSubnetV4)
	}
	if tunSubnetV6.IsValid() && tunSubnetV6.Addr().Unmap().Is6() {
		normalizedV6 = withDNSRoutes(normalizedV6, dnsV6, tunSubnetV6)
	}
	if configuredV4 != nil && !slices.Equal(configuredV4, normalizedV4) {
		slog.Warn(
			"client AllowedIPsv4 were changed",
			"configured", configuredV4,
			"effective", normalizedV4,
		)
	}
	if configuredV6 != nil && !slices.Equal(configuredV6, normalizedV6) {
		slog.Warn(
			"client AllowedIPsv6 were changed",
			"configured", configuredV6,
			"effective", normalizedV6,
		)
	}
	return normalizedV4, normalizedV6
}

func withDNSRoutes(splits, resolvers []string, tunSubnet netip.Prefix) []string {
	for _, resolver := range resolvers {
		addr, err := netip.ParseAddr(strings.TrimSpace(resolver))
		if err != nil {
			continue
		}
		addr = addr.WithZone("")
		if tunSubnet.Contains(addr) {
			continue
		}
		if slices.ContainsFunc(splits, func(split string) bool {
			prefix, _ := netip.ParsePrefix(split)
			return prefix.Contains(addr)
		}) {
			continue
		}
		splits = append(splits, netip.PrefixFrom(addr, addr.BitLen()).String())
	}
	return splits
}

// effectiveMTU determines the usable MTU for the configured address families.
// It returns the default MTU when the value falls outside the applicable IPv4 or IPv6 limits.
func effectiveMTU(configured settings.Settings, protocol settings.Protocol) int {
	mtu := configured.MTU
	// Use IPv6 limits for dual-stack because its minimum MTU is higher.
	switch {
	case configured.IPv6Subnet.IsValid() && configured.IPv6Subnet.Addr().Unmap().Is6():
		if mtu < settings.MinimumIPv6MTU || mtu > settings.MaximumMTU {
			mtu = settings.DefaultMTU
		}
	case configured.IPv4Subnet.IsValid() && configured.IPv4Subnet.Addr().Is4():
		if mtu < settings.MinimumIPv4MTU || mtu > settings.MaximumMTU {
			mtu = settings.DefaultMTU
		}
	}
	if configured.MTU != 0 && configured.MTU != mtu {
		slog.Warn(
			"client MTU was changed to a supported default",
			"protocol", protocol.String(),
			"configured", configured.MTU,
			"effective", mtu,
		)
	}
	return mtu
}

func (c *Configuration) ActiveSettings() (settings.Settings, error) {
	selected, err := c.selectedSettings()
	if err != nil {
		return settings.Settings{}, err
	}
	selectedCopy := *selected
	selectedCopy.Protocol = c.Protocol
	if err := selectedCopy.Network.DeriveIP(c.ClientID); err != nil { //nolint:staticcheck // Keep the mutation owner explicit.
		return settings.Settings{}, err
	}
	return selectedCopy, nil
}

func (c *Configuration) selectedSettings() (*settings.Settings, error) {
	switch c.Protocol {
	case settings.UDP:
		return &c.UDPSettings, nil
	case settings.TCP:
		return &c.TCPSettings, nil
	case settings.WS, settings.WSS:
		return &c.WSSettings, nil
	default:
		return nil, fmt.Errorf("unsupported protocol: %v", c.Protocol)
	}
}
