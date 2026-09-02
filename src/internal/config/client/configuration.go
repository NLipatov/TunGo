package client

import (
	"fmt"
	"log/slog"
	"net/netip"

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
	ClientPrivateKey []byte `json:"ClientPrivateKey"`
}

func (c *Configuration) applyDefaults() {
	active, err := c.selectedSettings()
	if err != nil {
		return
	}
	if active.IPv4Subnet.IsValid() && active.IPv4Subnet.Addr().Is4() && len(active.DNSv4) == 0 {
		active.DNSv4 = append([]string(nil), settings.DefaultClientDNSv4Resolvers...)
	}
	if active.IPv6Subnet.IsValid() && active.IPv6Subnet.Addr().Unmap().Is6() && len(active.DNSv6) == 0 {
		active.DNSv6 = append([]string(nil), settings.DefaultClientDNSv6Resolvers...)
	}
	effectiveMTU := effectiveMTU(active.MTU, active.IPv4Subnet, active.IPv6Subnet)
	if active.MTU != 0 && active.MTU != effectiveMTU {
		slog.Warn(
			"client MTU was changed to a supported default",
			"protocol", c.Protocol.String(),
			"configured", active.MTU,
			"effective", effectiveMTU,
		)
	}
	active.MTU = effectiveMTU
}

// effectiveMTU determines the usable MTU for the configured address families.
// It returns the default MTU when the value falls outside the applicable IPv4 or IPv6 limits.
func effectiveMTU(mtu int, v4Subnet, v6Subnet netip.Prefix) int {
	// Use IPv6 limits for dual-stack because its minimum MTU is higher.
	switch {
	case v6Subnet.IsValid() && v6Subnet.Addr().Unmap().Is6():
		if mtu < settings.MinimumIPv6MTU || mtu > settings.MaximumMTU {
			return settings.DefaultMTU
		}
	case v4Subnet.IsValid() && v4Subnet.Addr().Is4():
		if mtu < settings.MinimumIPv4MTU || mtu > settings.MaximumMTU {
			return settings.DefaultMTU
		}
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
