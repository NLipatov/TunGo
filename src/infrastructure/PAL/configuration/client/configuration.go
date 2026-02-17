package client

import (
	"fmt"
	"net/netip"
	"strings"
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
	if !selectedProtocolMatchesSettings(c.Protocol, active.Protocol) {
		return fmt.Errorf(
			"active settings protocol mismatch: selected %s, active settings has %s",
			c.Protocol,
			active.Protocol,
		)
	}
	if strings.TrimSpace(active.TunName) == "" {
		return fmt.Errorf("active settings: TunName is not configured")
	}
	if active.Server.IsZero() {
		return fmt.Errorf("active settings: Server is not configured")
	}
	if active.Port < 1 || active.Port > 65535 {
		// WSS: zero means "use default 443" in connection factory.
		if !(c.Protocol == settings.WSS && active.Port == 0) {
			return fmt.Errorf("active settings: invalid Port %d", active.Port)
		}
	}
	if !active.IPv4Subnet.IsValid() && !active.IPv6Subnet.IsValid() {
		return fmt.Errorf("active settings: both IPv4Subnet and IPv6Subnet are invalid")
	}
	if err := validateDNSServers(active.DNSv4, false); err != nil {
		return fmt.Errorf("active settings: %w", err)
	}
	if err := validateDNSServers(active.DNSv6, true); err != nil {
		return fmt.Errorf("active settings: %w", err)
	}
	return nil
}

// Resolve derives IPv4/IPv6 addresses for all protocol settings from subnets + ClientID.
func (c *Configuration) Resolve() error {
	for _, s := range []*settings.Settings{&c.TCPSettings, &c.UDPSettings, &c.WSSettings} {
		if err := s.Addressing.DeriveIP(c.ClientID); err != nil {
			return err
		}
	}
	return nil
}

// ResolveActive derives IPv4/IPv6 addresses only for active protocol settings.
// Use this for runtime client startup to avoid failing on inactive profile garbage.
func (c *Configuration) ResolveActive() error {
	active, err := c.activeSettingsPtr()
	if err != nil {
		return err
	}
	return active.Addressing.DeriveIP(c.ClientID)
}

func (c *Configuration) ActiveSettings() (settings.Settings, error) {
	active, err := c.activeSettingsPtr()
	if err != nil {
		return settings.Settings{}, err
	}
	return *active, nil
}

func (c *Configuration) activeSettingsPtr() (*settings.Settings, error) {
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

func selectedProtocolMatchesSettings(selected, bucket settings.Protocol) bool {
	if bucket == settings.UNKNOWN {
		return true
	}
	switch selected {
	case settings.WS, settings.WSS:
		return bucket == settings.WS || bucket == settings.WSS
	default:
		return bucket == selected
	}
}

func validateDNSServers(servers []string, wantIPv6 bool) error {
	for i, raw := range servers {
		resolver := strings.TrimSpace(raw)
		if resolver == "" {
			return fmt.Errorf("DNS[%d] is empty", i)
		}
		addr, err := netip.ParseAddr(resolver)
		if err != nil {
			return fmt.Errorf("DNS[%d] %q is not an IP address", i, raw)
		}
		isV4 := addr.Unmap().Is4()
		if wantIPv6 && isV4 {
			return fmt.Errorf("DNS[%d] %q is IPv4, expected IPv6", i, raw)
		}
		if !wantIPv6 && !isV4 {
			return fmt.Errorf("DNS[%d] %q is IPv6, expected IPv4", i, raw)
		}
	}
	return nil
}
