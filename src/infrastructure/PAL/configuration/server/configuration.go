package server

import (
	"fmt"
	"net/netip"
	"tungo/infrastructure/settings"
)

// AllowedPeer represents a single authorized client.
// This is the sole source of truth for client authorization.
type AllowedPeer struct {
	// Name is a human-friendly client identifier (e.g., "client-42").
	// Optional; does not participate in cryptographic authentication.
	Name string `json:"Name,omitempty"`

	// PublicKey is the client's X25519 static public key (32 bytes).
	// This is the cryptographic identity.
	PublicKey []byte `json:"PublicKey"`

	// Enabled controls whether this client can connect.
	// Setting to false revokes access immediately.
	Enabled bool `json:"Enabled"`

	// ClientID is the 1-based ordinal passed to AllocateClientIP at registration time.
	// Each peer must have a unique, positive ClientID.
	ClientID int `json:"ClientID"`
}

type Configuration struct {
	TCPSettings           settings.Settings `json:"TCPSettings"`
	UDPSettings           settings.Settings `json:"UDPSettings"`
	WSSettings            settings.Settings `json:"WSSettings"`
	FallbackServerAddress string            `json:"FallbackServerAddress"`
	X25519PublicKey       []byte            `json:"X25519PublicKey"`
	X25519PrivateKey      []byte            `json:"X25519PrivateKey"`
	ClientCounter         int               `json:"ClientCounter"`
	EnableTCP             bool              `json:"EnableTCP"`
	EnableUDP             bool              `json:"EnableUDP"`
	EnableWS              bool              `json:"EnableWS"`

	// AllowedPeers is the list of authorized clients.
	// Each peer is identified by their X25519 static public key.
	AllowedPeers []AllowedPeer `json:"AllowedPeers"`
}

func NewDefaultConfiguration() *Configuration {
	configuration := &Configuration{
		FallbackServerAddress: "",
		X25519PublicKey:       nil,
		X25519PrivateKey:      nil,
		ClientCounter:         0,
		EnableTCP:             false,
		EnableUDP:             true,
		EnableWS:              false,
	}
	return configuration.EnsureDefaults()
}

func (c *Configuration) EnsureDefaults() *Configuration {
	type proto struct {
		protocol settings.Protocol
		tunName  string
		cidr     string
		port     int
	}
	defaults := []proto{
		{settings.TCP, "tcptun0", "10.0.0.0/24", 8080},
		{settings.UDP, "udptun0", "10.0.1.0/24", 9090},
		{settings.WS, "wstun0", "10.0.2.0/24", 1010},
	}
	for i, s := range c.AllSettingsPtrs() {
		d := defaults[i]
		c.applyDefaults(s, c.defaultSettings(d.protocol, d.tunName, d.cidr, d.port))
	}
	return c
}

func (c *Configuration) applyDefaults(
	to *settings.Settings,
	from settings.Settings,
) {
	if to.TunName == "" {
		to.TunName = from.TunName
	}
	if !to.IPv4Subnet.IsValid() {
		to.IPv4Subnet = from.IPv4Subnet
	}
	// Derive server IPv4 from subnet if not already set.
	if to.IPv4Subnet.IsValid() && !to.IPv4.IsValid() {
		_ = to.Addressing.DeriveIP(0)
	}
	// IPv6 is opt-in: admin sets IPv6Subnet, server IP is derived automatically.
	if to.IPv6Subnet.IsValid() && !to.IPv6.IsValid() {
		_ = to.Addressing.DeriveIP(0)
	}
	if to.Port == 0 {
		to.Port = from.Port
	}
	if to.MTU == 0 {
		to.MTU = from.MTU
	}
	if to.Protocol == settings.UNKNOWN {
		to.Protocol = from.Protocol
	}
	if to.DialTimeoutMs == 0 {
		to.DialTimeoutMs = from.DialTimeoutMs
	}
}

func (c *Configuration) defaultSettings(
	protocol settings.Protocol,
	tunName, ipv4CIDR string,
	port int,
) settings.Settings {
	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    tunName,
			IPv4Subnet: netip.MustParsePrefix(ipv4CIDR),
			Port:       port,
		},
		MTU:           settings.DefaultEthernetMTU,
		Protocol:      protocol,
		Encryption:    settings.ChaCha20Poly1305,
		DialTimeoutMs: 5000,
	}
	// Derive server IP from subnet.
	_ = s.Addressing.DeriveIP(0)
	return s
}

// AllSettings returns all protocol settings regardless of enabled state.
func (c Configuration) AllSettings() []settings.Settings {
	return []settings.Settings{c.TCPSettings, c.UDPSettings, c.WSSettings}
}

// EnabledSettings returns only the settings for enabled protocols.
func (c Configuration) EnabledSettings() []settings.Settings {
	var result []settings.Settings
	if c.EnableTCP {
		result = append(result, c.TCPSettings)
	}
	if c.EnableUDP {
		result = append(result, c.UDPSettings)
	}
	if c.EnableWS {
		result = append(result, c.WSSettings)
	}
	return result
}

// AllSettingsPtrs returns pointers to all protocol settings for in-place mutation.
func (c *Configuration) AllSettingsPtrs() []*settings.Settings {
	return []*settings.Settings{&c.TCPSettings, &c.UDPSettings, &c.WSSettings}
}

func (c *Configuration) Validate() error {
	// interface names (ifNames) should be unique
	ifNames := map[string]struct{}{}
	for _, s := range c.AllSettings() {
		if s.TunName == "" {
			return fmt.Errorf("interface name is empty")
		}
		if _, ok := ifNames[s.TunName]; ok {
			return fmt.Errorf("duplicate interface name: %s", s.TunName)
		}
		ifNames[s.TunName] = struct{}{}
	}

	enabled := c.EnabledSettings()
	ports := make(map[int]struct{}, len(enabled))
	subnets := make([]netip.Prefix, 0, len(enabled))

	for _, config := range enabled {
		switch config.Protocol {
		case settings.TCP, settings.UDP, settings.WS, settings.WSS:
			// known protocol
		case settings.UNKNOWN:
			return fmt.Errorf("[%s] protocol is UNKNOWN", config.TunName)
		default:
			return fmt.Errorf(
				"[%s/%s] unsupported protocol %v",
				config.Protocol, config.TunName, config.Protocol,
			)
		}
		// validate port number
		portNumber := config.Port
		if portNumber < 1 || portNumber > 65535 {
			return fmt.Errorf(
				"invalid 'Port': [%s/%s] invalid port %d: must be in 1..65535",
				config.Protocol,
				config.TunName,
				portNumber,
			)
		}
		if _, dup := ports[portNumber]; dup {
			return fmt.Errorf(
				"invalid 'Port': [%s/%s] duplicate port %d",
				config.Protocol,
				config.TunName,
				portNumber,
			)
		}
		ports[portNumber] = struct{}{}
		// validate MTU
		if config.MTU < 576 || config.MTU > 9000 {
			return fmt.Errorf(
				"invalid 'MTU': [%s/%s] invalid MTU %d: expected 576..9000",
				config.Protocol,
				config.TunName,
				config.MTU,
			)
		}
		if err := c.validateSubnetContainsAddr("IPv4", config.IPv4Subnet, config.IPv4, config.Protocol, config.TunName); err != nil {
			return err
		}
		subnets = append(subnets, config.IPv4Subnet)

		// Validate optional IPv6 settings
		if config.IPv6Subnet.IsValid() {
			if err := c.validateSubnetContainsAddr("IPv6", config.IPv6Subnet, config.IPv6, config.Protocol, config.TunName); err != nil {
				return err
			}
			subnets = append(subnets, config.IPv6Subnet)
		}
	}

	// interface subnets must not overlap
	if c.overlappingSubnets(subnets) {
		return fmt.Errorf("two or more subnets are overlapping")
	}

	// validate AllowedPeers
	return c.ValidateAllowedPeers()
}

func (c *Configuration) validateSubnetContainsAddr(
	family string,
	subnet netip.Prefix,
	addr netip.Addr,
	proto settings.Protocol,
	tunName string,
) error {
	if !subnet.IsValid() {
		return fmt.Errorf(
			"invalid '%sSubnet': [%s/%s] invalid CIDR %q",
			family, proto, tunName, subnet,
		)
	}
	unmapped := addr.Unmap()
	if !unmapped.IsValid() {
		return fmt.Errorf(
			"invalid '%s': [%s/%s] invalid address %q",
			family, proto, tunName, addr,
		)
	}
	if !subnet.Contains(unmapped) {
		return fmt.Errorf(
			"invalid '%s': [%s/%s] address %s not in '%sSubnet' %s",
			family, proto, tunName, addr, family, subnet,
		)
	}
	return nil
}

func (c *Configuration) overlappingSubnets(subnets []netip.Prefix) bool {
	for i := 0; i < len(subnets); i++ {
		for j := i + 1; j < len(subnets); j++ {
			a, b := subnets[i], subnets[j]
			if a.Overlaps(b) || b.Overlaps(a) {
				return true
			}
		}
	}
	return false
}

// ValidateAllowedPeers validates the AllowedPeers configuration.
// Ensures no ClientID overlap between different peers and no duplicate public keys.
func (c *Configuration) ValidateAllowedPeers() error {
	seenIndex := make(map[int]int) // ClientID -> peer index

	for i, peer := range c.AllowedPeers {
		// Validate public key length
		if len(peer.PublicKey) != 32 {
			return fmt.Errorf("peer %d: invalid public key length %d, expected 32", i, len(peer.PublicKey))
		}

		if peer.ClientID <= 0 {
			return fmt.Errorf("peer %d: invalid ClientID %d: must be > 0", i, peer.ClientID)
		}

		// Check for duplicate ClientID
		if prev, exists := seenIndex[peer.ClientID]; exists {
			return fmt.Errorf(
				"ClientID conflict: peer %d and peer %d both have ClientID %d",
				prev, i, peer.ClientID,
			)
		}
		seenIndex[peer.ClientID] = i
	}

	// Check for duplicate public keys
	seen := make(map[string]int)
	for i, peer := range c.AllowedPeers {
		key := string(peer.PublicKey)
		if prev, exists := seen[key]; exists {
			return fmt.Errorf("duplicate public key: peer %d and peer %d", prev, i)
		}
		seen[key] = i
	}

	return nil
}
