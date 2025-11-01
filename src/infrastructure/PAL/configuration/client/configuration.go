package client

import (
	"crypto/ed25519"
	"fmt"
	"tungo/infrastructure/settings"
)

type Configuration struct {
	TCPSettings      settings.Settings `json:"TCPSettings"`
	UDPSettings      settings.Settings `json:"UDPSettings"`
	WSSettings       settings.Settings `json:"WSSettings"`
	Ed25519PublicKey ed25519.PublicKey `json:"Ed25519PublicKey"`
	Protocol         settings.Protocol `json:"Protocol"`
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
