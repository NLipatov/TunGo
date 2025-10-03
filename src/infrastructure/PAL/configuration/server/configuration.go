package server

import (
	"crypto/ed25519"
	"fmt"
	"net/netip"
	"strconv"
	"tungo/infrastructure/settings"
)

type Configuration struct {
	TCPSettings           settings.Settings  `json:"TCPSettings"`
	UDPSettings           settings.Settings  `json:"UDPSettings"`
	WSSettings            settings.Settings  `json:"WSSettings"`
	FallbackServerAddress string             `json:"FallbackServerAddress"`
	Ed25519PublicKey      ed25519.PublicKey  `json:"Ed25519PublicKey"`
	Ed25519PrivateKey     ed25519.PrivateKey `json:"Ed25519PrivateKey"`
	ClientCounter         int                `json:"ClientCounter"`
	EnableTCP             bool               `json:"EnableTCP"`
	EnableUDP             bool               `json:"EnableUDP"`
	EnableWS              bool               `json:"EnableWS"`
}

func NewDefaultConfiguration() *Configuration {
	configuration := &Configuration{
		FallbackServerAddress: "",
		Ed25519PublicKey:      nil,
		Ed25519PrivateKey:     nil,
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
		if !c.EnableTCP && !c.EnableUDP && !c.EnableWS {
			return fmt.Errorf("at least one protocol (TCP/UDP/WS) must be enabled")
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
