package client

import (
	"fmt"
	"tungo/infrastructure/settings"
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

// Validate checks that the configuration has the minimum required fields set.
func (c *Configuration) Validate() error {
	if c.ClientID <= 0 {
		return fmt.Errorf("invalid ClientID %d: must be > 0", c.ClientID)
	}
	if c.Protocol == settings.UNKNOWN {
		return fmt.Errorf("protocol is UNKNOWN")
	}
	if len(c.ClientPublicKey) != 32 {
		return fmt.Errorf("invalid ClientPublicKey length %d, expected 32", len(c.ClientPublicKey))
	}
	if len(c.ClientPrivateKey) != 32 {
		return fmt.Errorf("invalid ClientPrivateKey length %d, expected 32", len(c.ClientPrivateKey))
	}
	if len(c.X25519PublicKey) != 32 {
		return fmt.Errorf("invalid X25519PublicKey (server) length %d, expected 32", len(c.X25519PublicKey))
	}
	active, err := c.ActiveSettings()
	if err != nil {
		return err
	}
	if active.Server.IsZero() {
		return fmt.Errorf("active settings: Server is not configured")
	}
	if !active.IPv4Subnet.IsValid() {
		return fmt.Errorf("active settings: IPv4Subnet is invalid")
	}
	return nil
}

// Resolve derives IPv4/IPv6 addresses from subnets + ClientID.
// Called once after loading config from JSON.
func (c *Configuration) Resolve() error {
	for _, s := range []*settings.Settings{&c.TCPSettings, &c.UDPSettings, &c.WSSettings} {
		if err := s.Addressing.DeriveIP(c.ClientID); err != nil {
			return err
		}
	}
	return nil
}

func (c *Configuration) ActiveSettings() (settings.Settings, error) {
	switch c.Protocol {
	case settings.UDP:
		return c.UDPSettings, nil
	case settings.TCP:
		return c.TCPSettings, nil
	case settings.WS, settings.WSS:
		return c.WSSettings, nil
	default:
		return settings.Settings{}, fmt.Errorf("unsupported protocol: %v", c.Protocol)
	}
}
