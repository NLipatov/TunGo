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

func (c *Configuration) ApplyClientDefaults() *Configuration {
	active, err := c.activeSettings()
	if err != nil {
		return c
	}
	effectiveMTU := effectiveMTU(active.MTU, active.IPv4Subnet, active.IPv6Subnet)
	if active.MTU != 0 && active.MTU != effectiveMTU {
		slog.Warn(
			"client MTU was changed to a supported default",
			"configured", active.MTU,
			"effective", effectiveMTU,
		)
	}
	active.MTU = effectiveMTU
	return c
}

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
	configured, err := c.activeSettings()
	if err != nil {
		return settings.Settings{}, err
	}

	active := *configured
	active.Protocol = c.Protocol
	if err := active.DeriveIP(c.ClientID); err != nil {
		return settings.Settings{}, err
	}
	return active, nil
}

func (c *Configuration) activeSettings() (*settings.Settings, error) {
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
