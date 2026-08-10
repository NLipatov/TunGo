package client

import (
	"fmt"
	"tungo/internal/config/settings"
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

func (c *Configuration) ActiveSettings() (settings.Settings, error) {
	var active settings.Settings
	switch c.Protocol {
	case settings.UDP:
		active = c.UDPSettings
	case settings.TCP:
		active = c.TCPSettings
	case settings.WS, settings.WSS:
		active = c.WSSettings
	default:
		return settings.Settings{}, fmt.Errorf("unsupported protocol: %v", c.Protocol)
	}

	active.Protocol = c.Protocol
	if err := active.DeriveIP(c.ClientID); err != nil {
		return settings.Settings{}, err
	}
	return active, nil
}
