package client

import (
	"fmt"
	"tungo/infrastructure/settings"
)

type Configuration struct {
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
