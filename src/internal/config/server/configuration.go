package server

import (
	"net/netip"

	"tungo/internal/config/settings"
)

const (
	udpTunName = "s_udptun0"
	tcpTunName = "s_tcptun0"
	wsTunName  = "s_wstun0"
)

type Configuration struct {
	TCPSettings settings.Settings `json:"TCPSettings"`
	UDPSettings settings.Settings `json:"UDPSettings"`
	WSSettings  settings.Settings `json:"WSSettings"`
	// Host is written to generated client configurations.
	// A zero value enables automatic address detection.
	Host             string `json:"Host"`
	X25519PublicKey  []byte `json:"X25519PublicKey"`
	X25519PrivateKey []byte `json:"X25519PrivateKey"`
	ClientCounter    int    `json:"ClientCounter"`
	EnableTCP        bool   `json:"EnableTCP"`
	EnableUDP        bool   `json:"EnableUDP"`
	EnableWS         bool   `json:"EnableWS"`

	// AllowedPeers is the list of authorized clients.
	// Each peer is identified by their X25519 static public key.
	AllowedPeers []AllowedPeer `json:"AllowedPeers"`
}

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

func newConfiguration() *Configuration {
	configuration := &Configuration{
		X25519PublicKey:  nil,
		X25519PrivateKey: nil,
		ClientCounter:    0,
		EnableTCP:        false,
		EnableUDP:        true,
		EnableWS:         false,
	}
	configuration.applyDefaults()
	return configuration
}

func (c *Configuration) applyDefaults() {
	type proto struct {
		protocol settings.Protocol
		tunName  string
		cidr     string
		port     int
	}
	defaults := []proto{
		{settings.TCP, tcpTunName, "10.0.0.0/24", 8080},
		{settings.UDP, udpTunName, "10.0.1.0/24", 9090},
		{settings.WS, wsTunName, "10.0.2.0/24", 1010},
	}
	for i, s := range c.settings() {
		d := defaults[i]
		c.fillDefaults(s, c.defaultSettings(d.protocol, d.tunName, d.cidr, d.port))
	}
}

func (c *Configuration) fillDefaults(
	to *settings.Settings,
	from settings.Settings,
) {
	if to.TunName == "" {
		to.TunName = from.TunName
	}
	if !to.IPv4Subnet.IsValid() {
		to.IPv4Subnet = from.IPv4Subnet
	}
	network := &to.Network
	// Derive server IPv4 from subnet if not already set.
	if network.IPv4Subnet.IsValid() && !network.IPv4.IsValid() {
		_ = network.DeriveIP(0)
	}
	// IPv6 is opt-in: admin sets IPv6Subnet, server IP is derived automatically.
	if network.IPv6Subnet.IsValid() && !network.IPv6.IsValid() {
		_ = network.DeriveIP(0)
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
	network := settings.Network{
		TunName:    tunName,
		IPv4Subnet: netip.MustParsePrefix(ipv4CIDR),
		Port:       port,
	}
	// Derive server IP from subnet.
	_ = network.DeriveIP(0)
	return settings.Settings{
		Network:       network,
		MTU:           settings.DefaultEthernetMTU,
		Protocol:      protocol,
		Encryption:    settings.ChaCha20Poly1305,
		DialTimeoutMs: 5000,
	}
}

func (c Configuration) Profiles() [3]settings.Profile {
	return [3]settings.Profile{
		{Settings: c.TCPSettings, Enabled: c.EnableTCP},
		{Settings: c.UDPSettings, Enabled: c.EnableUDP},
		{Settings: c.WSSettings, Enabled: c.EnableWS},
	}
}

func (c *Configuration) settings() []*settings.Settings {
	return []*settings.Settings{&c.TCPSettings, &c.UDPSettings, &c.WSSettings}
}
