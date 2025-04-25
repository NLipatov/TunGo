package client_configuration

import (
	"crypto/ed25519"
	"tungo/settings"
)

type Configuration struct {
	TCPSettings      settings.ConnectionSettings `json:"TCPSettings"`
	UDPSettings      settings.ConnectionSettings `json:"UDPSettings"`
	Ed25519PublicKey ed25519.PublicKey           `json:"Ed25519PublicKey"`
	Protocol         settings.Protocol           `json:"Protocol"`
}
