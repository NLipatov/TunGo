package configuration

import (
	"fmt"
	"tungo/infrastructure/settings"
)

type ClientRuntimeConfiguration struct {
	ClientID         int
	TCPSettings      settings.Settings
	UDPSettings      settings.Settings
	WSSettings       settings.Settings
	X25519PublicKey  []byte
	Protocol         settings.Protocol
	ClientPublicKey  []byte
	ClientPrivateKey []byte
}

func (c ClientRuntimeConfiguration) ActiveSettings() (settings.Settings, error) {
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

type ServerRuntimeConfiguration struct {
	TCPSettings           settings.Settings
	UDPSettings           settings.Settings
	WSSettings            settings.Settings
	FallbackServerAddress string
	X25519PublicKey       []byte
	X25519PrivateKey      []byte
	ClientCounter         int
	EnableTCP             bool
	EnableUDP             bool
	EnableWS              bool
	AllowedPeers          []ServerPeer
}

func (c ServerRuntimeConfiguration) AllSettings() []settings.Settings {
	return []settings.Settings{c.TCPSettings, c.UDPSettings, c.WSSettings}
}

func (c ServerRuntimeConfiguration) EnabledSettings() []settings.Settings {
	result := make([]settings.Settings, 0, 3)
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
