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
	type named struct {
		name string
		s    settings.Settings
	}
	configs := []named{
		{"TCP", c.TCPSettings},
		{"UDP", c.UDPSettings},
		{"WS", c.WSSettings},
	}

	names := map[string]struct{}{}
	for _, n := range []string{c.TCPSettings.InterfaceName, c.UDPSettings.InterfaceName, c.WSSettings.InterfaceName} {
		if n == "" {
			return fmt.Errorf("interface name is empty")
		}
		if _, ok := names[n]; ok {
			return fmt.Errorf("duplicate interface name: %s", n)
		}
		names[n] = struct{}{}
	}

	ports := make(map[int]struct{}, len(configs))
	subnets := make([]netip.Prefix, 0, len(configs))

	for _, cfg := range configs {
		switch cfg.name {
		case "TCP":
			if !c.EnableTCP {
				continue
			}
		case "UDP":
			if !c.EnableUDP {
				continue
			}
		case "WS":
			if !c.EnableWS {
				continue
			}
		}

		if cfg.s.Protocol == settings.UNKNOWN {
			return fmt.Errorf("[%s/%s] protocol is UNKNOWN", cfg.name, cfg.s.InterfaceName)
		}

		portNumber, err := strconv.Atoi(cfg.s.Port)
		if err != nil {
			return fmt.Errorf("[%s/%s] invalid port %q: not a number", cfg.name, cfg.s.InterfaceName, cfg.s.Port)
		}
		if portNumber < 1 || portNumber > 65535 {
			return fmt.Errorf("[%s/%s] invalid port %d: must be in 1..65535", cfg.name, cfg.s.InterfaceName, portNumber)
		}
		if _, dup := ports[portNumber]; dup {
			return fmt.Errorf("[%s/%s] duplicate port %d", cfg.name, cfg.s.InterfaceName, portNumber)
		}
		ports[portNumber] = struct{}{}

		if cfg.s.MTU < 576 || cfg.s.MTU > 9000 {
			return fmt.Errorf("[%s/%s] invalid MTU %d: expected 576..9000", cfg.name, cfg.s.InterfaceName, cfg.s.MTU)
		}

		pfx, err := netip.ParsePrefix(cfg.s.InterfaceIPCIDR)
		if err != nil {
			return fmt.Errorf("[%s/%s] invalid CIDR %q: %v", cfg.name, cfg.s.InterfaceName, cfg.s.InterfaceIPCIDR, err)
		}
		addr, err := netip.ParseAddr(cfg.s.InterfaceAddress)
		if err != nil {
			return fmt.Errorf("[%s/%s] invalid address %q: %v", cfg.name, cfg.s.InterfaceName, cfg.s.InterfaceAddress, err)
		}
		if !pfx.Contains(addr) {
			return fmt.Errorf("[%s/%s] address %s not in CIDR %s", cfg.name, cfg.s.InterfaceName, cfg.s.InterfaceAddress, cfg.s.InterfaceIPCIDR)
		}
		subnets = append(subnets, pfx)
	}

	// overlap check
	for i := 0; i < len(subnets); i++ {
		for j := i + 1; j < len(subnets); j++ {
			a, b := subnets[i], subnets[j]
			if a.Contains(b.Addr()) || b.Contains(a.Addr()) {
				return fmt.Errorf("subnet overlap: %s and %s", a, b)
			}
		}
	}
	return nil
}
