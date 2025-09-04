package server

import (
	"crypto/ed25519"
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
	return &Configuration{
		TCPSettings: settings.Settings{
			InterfaceName:    "tcptun0",
			InterfaceIPCIDR:  "10.0.0.0/24",
			InterfaceAddress: "10.0.0.1",
			ConnectionIP:     "",
			Port:             "8080",
			MTU:              settings.MTU,
			Protocol:         settings.TCP,
			Encryption:       settings.ChaCha20Poly1305,
			DialTimeoutMs:    5000,
		},
		UDPSettings: settings.Settings{
			InterfaceName:    "udptun0",
			InterfaceIPCIDR:  "10.0.1.0/24",
			InterfaceAddress: "10.0.1.1",
			ConnectionIP:     "",
			Port:             "9090",
			MTU:              settings.MTU,
			Protocol:         settings.UDP,
			Encryption:       settings.ChaCha20Poly1305,
			DialTimeoutMs:    5000,
		},
		WSSettings: settings.Settings{
			InterfaceName:    "wstun0",
			InterfaceIPCIDR:  "10.0.2.0/24",
			InterfaceAddress: "10.0.2.1",
			ConnectionIP:     "",
			Port:             "1010",
			MTU:              settings.MTU,
			Protocol:         settings.WS,
			Encryption:       settings.ChaCha20Poly1305,
			DialTimeoutMs:    5000,
		},
		FallbackServerAddress: "",
		Ed25519PublicKey:      nil,
		Ed25519PrivateKey:     nil,
		ClientCounter:         0,
		EnableTCP:             false,
		EnableUDP:             true,
		EnableWS:              false,
	}
}
