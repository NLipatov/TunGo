package client

import (
	"fmt"
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
	if err != nil || active.MTU != 0 {
		return c
	}

	switch {
	case isIPv6Prefix(active.IPv6Subnet):
		active.MTU = settings.DefaultIPv6MTU
	case isIPv4Prefix(active.IPv4Subnet):
		active.MTU = settings.DefaultIPv4MTU
	}
	return c
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

func isIPv4Prefix(prefix netip.Prefix) bool {
	return prefix.IsValid() && prefix.Addr().Is4()
}

func isIPv6Prefix(prefix netip.Prefix) bool {
	return prefix.IsValid() && !prefix.Addr().Is4()
}
