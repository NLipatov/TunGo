package server_configuration

import (
	"crypto/ed25519"
	"time"
	"tungo/infrastructure/settings"
)

const (
	DefaultSessionTtl             = settings.HumanReadableDuration(time.Hour * 12)
	DefaultSessionCleanupInterval = settings.HumanReadableDuration(time.Hour * 6)
)

type Configuration struct {
	TCPSettings           settings.Settings  `json:"TCPSettings"`
	UDPSettings           settings.Settings  `json:"UDPSettings"`
	FallbackServerAddress string             `json:"FallbackServerAddress"`
	Ed25519PublicKey      ed25519.PublicKey  `json:"Ed25519PublicKey"`
	Ed25519PrivateKey     ed25519.PrivateKey `json:"Ed25519PrivateKey"`
	ClientCounter         int                `json:"ClientCounter"`
	EnableTCP             bool               `json:"EnableTCP"`
	EnableUDP             bool               `json:"EnableUDP"`
}

func NewDefaultConfiguration() *Configuration {
	return &Configuration{
		TCPSettings: settings.Settings{
			InterfaceName:    "tcptun0",
			InterfaceIPCIDR:  "10.0.0.0/24",
			InterfaceAddress: "10.0.0.1",
			ConnectionIP:     "",
			Port:             "8080",
			MTU:              1472,
			Protocol:         settings.TCP,
			Encryption:       settings.ChaCha20Poly1305,
			DialTimeoutMs:    5000,
			SessionLifetime: settings.SessionLifetime{
				Ttl:             DefaultSessionTtl,
				CleanupInterval: DefaultSessionCleanupInterval,
			},
		},
		UDPSettings: settings.Settings{
			InterfaceName:    "udptun0",
			InterfaceIPCIDR:  "10.0.1.0/24",
			InterfaceAddress: "10.0.1.1",
			ConnectionIP:     "",
			Port:             "9090",
			MTU:              1472,
			Protocol:         settings.UDP,
			Encryption:       settings.ChaCha20Poly1305,
			DialTimeoutMs:    5000,
			SessionLifetime: settings.SessionLifetime{
				Ttl:             DefaultSessionTtl,
				CleanupInterval: DefaultSessionCleanupInterval,
			},
		},
		FallbackServerAddress: "",
		Ed25519PublicKey:      nil,
		Ed25519PrivateKey:     nil,
		ClientCounter:         0,
		EnableTCP:             false,
		EnableUDP:             true,
	}
}
