package client

import (
	"fmt"
	"log/slog"
	"net/netip"
	"slices"
	"strings"

	"tungo/internal/config/internal/splits"
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
	TunnelRoutesV4   []string `json:"TunnelRoutesV4"`
	TunnelRoutesV6   []string `json:"TunnelRoutesV6"`
}

func (c *Configuration) applyDefaults() {
	activeSettings, err := c.selectedSettings()
	if err != nil {
		return
	}
	dnsV4, dnsV6 := effectiveDNS(activeSettings.Network)
	activeSettings.DNSv4, activeSettings.DNSv6 = dnsV4, dnsV6
	c.TunnelRoutesV4 = effectiveTunnelRoutes(
		"TunnelRoutesV4",
		c.TunnelRoutesV4,
		dnsV4,
		activeSettings.IPv4Subnet,
		splits.FullTunnelV4(),
		isIPv4Prefix,
	)
	c.TunnelRoutesV6 = effectiveTunnelRoutes(
		"TunnelRoutesV6",
		c.TunnelRoutesV6,
		dnsV6,
		activeSettings.IPv6Subnet,
		splits.FullTunnelV6(),
		isIPv6Prefix,
	)
	activeSettings.MTU = effectiveMTU(*activeSettings, c.Protocol)
	activeSettings.DialTimeout = effectiveDialTimeout(*activeSettings)
}

func isIPv4Prefix(p netip.Prefix) bool {
	return p.Addr().Is4()
}

func isIPv6Prefix(p netip.Prefix) bool {
	return p.Addr().Unmap().Is6()
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

// effectiveTunnelRoutes normalizes one address family's tunnel routes.
// A nil list means full tunnel; an empty list tunnels nothing but DNS.
// Invalid entries are skipped, any /0 entry replaces the list with full tunnel,
// and host bits are cleared. Each resolver outside the TUN subnet gets a host
// route unless a configured prefix already covers it.
func effectiveTunnelRoutes(
	field string,
	routes, resolvers []string,
	tunSubnet netip.Prefix,
	defaultRoutes []string,
	inFamily func(netip.Prefix) bool,
) []string {
	if routes == nil {
		routes = defaultRoutes
	}
	uncoveredDNS := make([]netip.Addr, 0, len(resolvers))
	for _, strAddr := range resolvers {
		addr, err := netip.ParseAddr(strings.TrimSpace(strAddr))
		if err != nil {
			continue
		}
		addr = addr.WithZone("")
		if tunSubnet.IsValid() && !tunSubnet.Contains(addr) && !slices.Contains(uncoveredDNS, addr) {
			uncoveredDNS = append(uncoveredDNS, addr)
		}
	}
	seen := make(map[netip.Prefix]struct{}, len(routes))
	normalized := make([]string, 0, len(routes))
	for _, cidr := range routes {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil || !inFamily(prefix) {
			slog.Warn(
				"client tunnel route skipped: invalid prefix",
				"field", field,
				"configured", cidr,
			)
			continue
		}
		if prefix.Bits() == 0 {
			slog.Warn(
				"client tunnel routes replaced with full tunnel: /0 prefix",
				"field", field,
				"configured", cidr,
			)
			normalized = defaultRoutes
			uncoveredDNS = uncoveredDNS[:0]
			break
		}
		prefixMasked := prefix.Masked()
		if prefix != prefixMasked {
			slog.Warn(
				"client tunnel route host bits cleared",
				"field", field,
				"configured", cidr,
				"effective", prefixMasked,
			)
		}
		uncoveredDNS = slices.DeleteFunc(uncoveredDNS, prefix.Contains)
		if _, ok := seen[prefixMasked]; ok {
			continue
		}
		seen[prefixMasked] = struct{}{}
		normalized = append(normalized, prefixMasked.String())
	}
	for _, addr := range uncoveredDNS {
		route := netip.PrefixFrom(addr, addr.BitLen()).String()
		slog.Info(
			"client DNS resolver route added",
			"field", field,
			"route", route,
		)
		normalized = append(normalized, route)
	}
	return normalized
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

func effectiveDialTimeout(s settings.Settings) settings.HumanReadableDuration {
	timeout := s.DialTimeout
	if timeout <= 0 {
		timeout = settings.DefaultDialTimeout
		slog.Warn(
			"client dial timeout was changed to a positive default",
			"protocol", s.Protocol.String(),
			"configured", s.DialTimeout,
			"effective", timeout,
		)
	}
	return timeout
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
