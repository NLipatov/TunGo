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

	// ClientIndex is the 1-based ordinal passed to AllocateClientIP at registration time.
	// Each peer must have a unique, positive ClientIndex.
	ClientIndex int `json:"ClientIndex"`
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
	c.applyDefaults(&c.TCPSettings, c.defaultSettings(
		settings.TCP,
		"tcptun0",
		"10.0.0.0/24",
		"10.0.0.1",
		8080,
	))
	c.applyDefaults(&c.UDPSettings, c.defaultSettings(
		settings.UDP,
		"udptun0",
		"10.0.1.0/24",
		"10.0.1.1",
		9090,
	))
	c.applyDefaults(&c.WSSettings, c.defaultSettings(
		settings.WS,
		"wstun0",
		"10.0.2.0/24",
		"10.0.2.1",
		1010,
	))
	return c
}

func (c *Configuration) applyDefaults(
	to *settings.Settings,
	from settings.Settings,
) {
	if to.InterfaceName == "" {
		to.InterfaceName = from.InterfaceName
	}
	if !to.InterfaceSubnet.IsValid() {
		to.InterfaceSubnet = from.InterfaceSubnet
	}
	if !to.InterfaceIP.IsValid() {
		to.InterfaceIP = from.InterfaceIP
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
	interfaceName, InterfaceCIDR, InterfaceAddr string,
	port int,
) settings.Settings {
	return settings.Settings{
		InterfaceName:   interfaceName,
		InterfaceSubnet: netip.MustParsePrefix(InterfaceCIDR),
		InterfaceIP:     netip.MustParseAddr(InterfaceAddr),
		Host:            "",
		Port:            port,
		MTU:             settings.DefaultEthernetMTU,
		Protocol:        protocol,
		Encryption:      settings.ChaCha20Poly1305,
		DialTimeoutMs:   5000,
	}
}

func (c *Configuration) Validate() error {
	configs := []settings.Settings{c.TCPSettings, c.UDPSettings, c.WSSettings}
	// interface names (ifNames) should be unique
	ifNames := map[string]struct{}{}
	for _, ifName := range []string{c.TCPSettings.InterfaceName, c.UDPSettings.InterfaceName, c.WSSettings.InterfaceName} {
		if ifName == "" {
			return fmt.Errorf("interface name is empty")
		}
		if _, ok := ifNames[ifName]; ok {
			return fmt.Errorf("duplicate interface name: %s", ifName)
		}
		ifNames[ifName] = struct{}{}
	}
	// ports should be unique
	ports := make(map[int]struct{}, len(configs))
	// subnets must not overlap
	subnets := make([]netip.Prefix, 0, len(configs))

	for _, config := range configs {
		switch config.Protocol {
		// if protocol is turned off, its validation may be skipped
		case settings.TCP:
			if !c.EnableTCP {
				continue
			}
		case settings.UDP:
			if !c.EnableUDP {
				continue
			}
		case settings.WS:
			if !c.EnableWS {
				continue
			}
		case settings.UNKNOWN:
			return fmt.Errorf("[%s] protocol is UNKNOWN", config.InterfaceName)
		default:
			return fmt.Errorf(
				"[%s/%s] unsupported protocol %v",
				config.Protocol,
				config.InterfaceName,
				config.Protocol,
			)
		}
		// validate port number
		portNumber := config.Port
		if portNumber < 1 || portNumber > 65535 {
			return fmt.Errorf(
				"invalid 'Port': [%s/%s] invalid port %d: must be in 1..65535",
				config.Protocol,
				config.InterfaceName,
				portNumber,
			)
		}
		if _, dup := ports[portNumber]; dup {
			return fmt.Errorf(
				"invalid 'Port': [%s/%s] duplicate port %d",
				config.Protocol,
				config.InterfaceName,
				portNumber,
			)
		}
		ports[portNumber] = struct{}{}
		// validate MTU
		if config.MTU < 576 || config.MTU > 9000 {
			return fmt.Errorf(
				"invalid 'MTU': [%s/%s] invalid MTU %d: expected 576..9000",
				config.Protocol,
				config.InterfaceName,
				config.MTU,
			)
		}
		pfx := config.InterfaceSubnet
		if !pfx.IsValid() {
			return fmt.Errorf(
				"invalid 'InterfaceSubnet': [%s/%s] invalid CIDR %q",
				config.Protocol,
				config.InterfaceName,
				config.InterfaceSubnet,
			)
		}
		addr := config.InterfaceIP.Unmap()
		if !addr.IsValid() {
			return fmt.Errorf(
				"invalid 'InterfaceIP': [%s/%s] invalid address %q",
				config.Protocol,
				config.InterfaceName,
				config.InterfaceIP,
			)
		}
		if !pfx.Contains(addr) {
			return fmt.Errorf(
				"invalid 'InterfaceIP': [%s/%s] address %s not in 'InterfaceSubnet' subnet %s",
				config.Protocol,
				config.InterfaceName,
				config.InterfaceIP,
				config.InterfaceSubnet,
			)
		}
		subnets = append(subnets, pfx)
	}

	// interface subnets must not overlap
	if c.overlappingSubnets(subnets) {
		return fmt.Errorf("invalid 'InterfaceSubnet':  two or more interface subnets are overlapping.")
	}

	// validate AllowedPeers
	if err := c.ValidateAllowedPeers(); err != nil {
		return err
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
// Ensures no ClientIndex overlap between different peers and no duplicate public keys.
func (c *Configuration) ValidateAllowedPeers() error {
	seenIndex := make(map[int]int) // ClientIndex → peer index

	for i, peer := range c.AllowedPeers {
		// Validate public key length
		if len(peer.PublicKey) != 32 {
			return fmt.Errorf("peer %d: invalid public key length %d, expected 32", i, len(peer.PublicKey))
		}

		if peer.ClientIndex <= 0 {
			return fmt.Errorf("peer %d: invalid ClientIndex %d: must be > 0", i, peer.ClientIndex)
		}

		// Check for duplicate ClientIndex
		if prev, exists := seenIndex[peer.ClientIndex]; exists {
			return fmt.Errorf(
				"ClientIndex conflict: peer %d and peer %d both have ClientIndex %d",
				prev, i, peer.ClientIndex,
			)
		}
		seenIndex[peer.ClientIndex] = i
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
