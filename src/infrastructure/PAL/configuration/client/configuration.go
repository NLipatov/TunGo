package client

import (
	"crypto/ed25519"
	"tungo/infrastructure/settings"
)

type Configuration struct {
	TCPSettings      settings.Settings `json:"TCPSettings"`
	UDPSettings      settings.Settings `json:"UDPSettings"`
	WSSettings       settings.Settings `json:"WSSettings"`
	Ed25519PublicKey ed25519.PublicKey `json:"Ed25519PublicKey"`
	Protocol         settings.Protocol `json:"Protocol"`
}
