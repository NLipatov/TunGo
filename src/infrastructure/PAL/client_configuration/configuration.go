package client_configuration

import (
	"crypto/ed25519"
	settings2 "tungo/infrastructure/settings"
)

type Configuration struct {
	TCPSettings      settings2.Settings `json:"TCPSettings"`
	UDPSettings      settings2.Settings `json:"UDPSettings"`
	Ed25519PublicKey ed25519.PublicKey  `json:"Ed25519PublicKey"`
	Protocol         settings2.Protocol `json:"Protocol"`
}
