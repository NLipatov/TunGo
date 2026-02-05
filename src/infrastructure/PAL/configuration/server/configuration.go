package server

import (
	"fmt"
	"net/netip"
	"strconv"
	"tungo/infrastructure/settings"
)

// AllowedPeer represents a single authorized client.
// This is the sole source of truth for client authorization.
type AllowedPeer struct {
	// PublicKey is the client's X25519 static public key (32 bytes).
	// This is the cryptographic identity.
	PublicKey []byte `json:"PublicKey"`

	// Enabled controls whether this client can connect.
	// Setting to false revokes access immediately.
	Enabled bool `json:"Enabled"`

	// ClientIP is the server-assigned internal IP for this client.
	// Exactly one address: /32 (IPv4) or /128 (IPv6).
	// The client does not choose this.
	ClientIP string `json:"ClientIP"`

	// AllowedIPs are additional prefixes this client may use as source IP.
	// Optional. ClientIP is always implicitly allowed.
	AllowedIPs []string `json:"AllowedIPs"`
}

// AllowedIPPrefixes parses AllowedIPs and returns them as netip.Prefix slice.
// ClientIP is NOT included; caller should handle it separately.
func (p *AllowedPeer) AllowedIPPrefixes() []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(p.AllowedIPs))
	for _, cidr := range p.AllowedIPs {
		prefix, err := netip.ParsePrefix(cidr)
		if err == nil {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes
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
	c.applyDefaults(&c.TCPSettings, c.defaultTCPSettings())
	c.applyDefaults(&c.UDPSettings, c.defaultUDPSettings())
	c.applyDefaults(&c.WSSettings, c.defaultWSSettings())
	return c
}

func (c *Configuration) applyDefaults(
	to *settings.Settings,
	from settings.Settings,
) {
	if to.InterfaceName == "" {
		to.InterfaceName = from.InterfaceName
	}
	if to.InterfaceIPCIDR == "" {
		to.InterfaceIPCIDR = from.InterfaceIPCIDR
	}
	if to.InterfaceAddress == "" {
		to.InterfaceAddress = from.InterfaceAddress
	}
	if to.Port == "" {
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

func (c *Configuration) defaultTCPSettings() settings.Settings {
	return c.defaultSettings(
		settings.TCP,
		"tcptun0",
		"10.0.0.0/24",
		"10.0.0.1",
		"8080",
	)
}

func (c *Configuration) defaultUDPSettings() settings.Settings {
	return c.defaultSettings(
		settings.UDP,
		"udptun0",
		"10.0.1.0/24",
		"10.0.1.1",
		"9090",
	)
}

func (c *Configuration) defaultWSSettings() settings.Settings {
	return c.defaultSettings(
		settings.WS,
		"wstun0",
		"10.0.2.0/24",
		"10.0.2.1",
		"1010",
	)
}

func (c *Configuration) defaultSettings(
	protocol settings.Protocol,
	interfaceName, InterfaceCIDR, InterfaceAddr, port string,
) settings.Settings {
	return settings.Settings{
		InterfaceName:    interfaceName,
		InterfaceIPCIDR:  InterfaceCIDR,
		InterfaceAddress: InterfaceAddr,
		ConnectionIP:     "",
		Port:             port,
		MTU:              settings.DefaultEthernetMTU,
		Protocol:         protocol,
		Encryption:       settings.ChaCha20Poly1305,
		DialTimeoutMs:    5000,
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
		portNumber, err := strconv.Atoi(config.Port)
		if err != nil {
			return fmt.Errorf(
				"invalid 'Port': [%s/%s] invalid port %q: not a number",
				config.Protocol,
				config.InterfaceName,
				config.Port,
			)
		}
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
		// validate interface subnet (InterfaceIPCIDR)
		pfx, err := netip.ParsePrefix(config.InterfaceIPCIDR)
		if err != nil {
			return fmt.Errorf(
				"invalid 'InterfaceIPCIDR': [%s/%s] invalid CIDR %q: %v",
				config.Protocol,
				config.InterfaceName,
				config.InterfaceIPCIDR,
				err,
			)
		}
		addr, err := netip.ParseAddr(config.InterfaceAddress)
		if err != nil {
			return fmt.Errorf(
				"invalid 'InterfaceAddress': [%s/%s] invalid address %q: %v",
				config.Protocol,
				config.InterfaceName,
				config.InterfaceAddress,
				err,
			)
		}
		if !pfx.Contains(addr) {
			return fmt.Errorf(
				"invalid 'InterfaceAddress': [%s/%s] address %s not in 'InterfaceIPCIDR' subnet %s",
				config.Protocol,
				config.InterfaceName,
				config.InterfaceAddress,
				config.InterfaceIPCIDR,
			)
		}
		subnets = append(subnets, pfx)
	}

	// interface subnets must not overlap
	if c.overlappingSubnets(subnets) {
		return fmt.Errorf("invalid 'InterfaceIPCIDR':  two or more interface subnets are overlapping.")
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
// Ensures no AllowedIPs overlap between different peers and no duplicate public keys.
// Also validates that each ClientIP is within at least one enabled interface subnet.
func (c *Configuration) ValidateAllowedPeers() error {
	// Collect interface subnets for ClientIP validation
	var interfaceSubnets []netip.Prefix
	if c.EnableTCP {
		if pfx, err := netip.ParsePrefix(c.TCPSettings.InterfaceIPCIDR); err == nil {
			interfaceSubnets = append(interfaceSubnets, pfx)
		}
	}
	if c.EnableUDP {
		if pfx, err := netip.ParsePrefix(c.UDPSettings.InterfaceIPCIDR); err == nil {
			interfaceSubnets = append(interfaceSubnets, pfx)
		}
	}
	if c.EnableWS {
		if pfx, err := netip.ParsePrefix(c.WSSettings.InterfaceIPCIDR); err == nil {
			interfaceSubnets = append(interfaceSubnets, pfx)
		}
	}

	// Collect all prefixes with their peer index
	type prefixOwner struct {
		prefix netip.Prefix
		peer   int
	}
	var allPrefixes []prefixOwner

	for i, peer := range c.AllowedPeers {
		// Validate public key length
		if len(peer.PublicKey) != 32 {
			return fmt.Errorf("peer %d: invalid public key length %d, expected 32", i, len(peer.PublicKey))
		}

		// Parse and collect ClientIP as /32 or /128
		clientIP, err := netip.ParseAddr(peer.ClientIP)
		if err != nil {
			return fmt.Errorf("peer %d: invalid ClientIP %q: %w", i, peer.ClientIP, err)
		}

		// Validate ClientIP is within at least one interface subnet
		if !c.isClientIPInSubnet(clientIP, interfaceSubnets) {
			return fmt.Errorf(
				"peer %d: ClientIP %s is not within any enabled interface subnet; "+
					"must be in one of: %v",
				i, peer.ClientIP, interfaceSubnets,
			)
		}

		bits := 32
		if clientIP.Is6() {
			bits = 128
		}
		clientPrefix := netip.PrefixFrom(clientIP, bits)
		allPrefixes = append(allPrefixes, prefixOwner{clientPrefix, i})

		// Parse and collect AllowedIPs
		for _, cidr := range peer.AllowedIPs {
			prefix, err := netip.ParsePrefix(cidr)
			if err != nil {
				return fmt.Errorf("peer %d: invalid AllowedIP %q: %w", i, cidr, err)
			}
			allPrefixes = append(allPrefixes, prefixOwner{prefix, i})
		}
	}

	// Check for overlaps between different peers
	for i := 0; i < len(allPrefixes); i++ {
		for j := i + 1; j < len(allPrefixes); j++ {
			a, b := allPrefixes[i], allPrefixes[j]
			if a.peer == b.peer {
				continue // Same peer, overlap is fine
			}
			if a.prefix.Overlaps(b.prefix) {
				return fmt.Errorf(
					"AllowedIPs overlap: peer %d prefix %s overlaps with peer %d prefix %s",
					a.peer, a.prefix, b.peer, b.prefix,
				)
			}
		}
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

// isClientIPInSubnet checks if clientIP is contained in any of the interface subnets.
func (c *Configuration) isClientIPInSubnet(clientIP netip.Addr, subnets []netip.Prefix) bool {
	for _, subnet := range subnets {
		if subnet.Contains(clientIP) {
			return true
		}
	}
	return false
}
