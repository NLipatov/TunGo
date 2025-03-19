package server

import (
	"crypto/ed25519"
	"tungo/settings"
)

type Configuration struct {
	TCPSettings               settings.ConnectionSettings `json:"TCPSettings"`
	UDPSettings               settings.ConnectionSettings `json:"UDPSettings"`
	FallbackServerAddress     string                      `json:"FallbackServerAddress"`
	Ed25519PublicKey          ed25519.PublicKey           `json:"Ed25519PublicKey"`
	Ed25519PrivateKey         ed25519.PrivateKey          `json:"Ed25519PrivateKey"`
	ClientCounter             int                         `json:"ClientCounter"`
	TCPWriteChannelBufferSize int32                       `json:"TCPWriteChannelBufferSize"`
	UDPNonceRingBufferSize    int                         `json:"UDPNonceRingBufferSize"`
	EnableTCP                 bool                        `json:"EnableTCP"`
	EnableUDP                 bool                        `json:"EnableUDP"`
}

func NewDefaultConfiguration() *Configuration {
	return &Configuration{
		TCPSettings: settings.ConnectionSettings{
			InterfaceName:    "tcptun0",
			InterfaceIPCIDR:  "10.0.0.0/24",
			InterfaceAddress: "10.0.0.1",
			ConnectionIP:     "",
			Port:             "8080",
			MTU:              1472,
			Protocol:         0,
			Encryption:       0,
			DialTimeoutMs:    5000,
		},
		UDPSettings: settings.ConnectionSettings{
			InterfaceName:    "udptun0",
			InterfaceIPCIDR:  "10.0.1.0/24",
			InterfaceAddress: "10.0.1.1",
			ConnectionIP:     "",
			Port:             "9090",
			MTU:              1416,
			Protocol:         1,
			Encryption:       0,
			DialTimeoutMs:    5000,
		},
		FallbackServerAddress:     "",
		Ed25519PublicKey:          nil,
		Ed25519PrivateKey:         nil,
		ClientCounter:             0,
		TCPWriteChannelBufferSize: 1000,
		UDPNonceRingBufferSize:    100_000,
		EnableTCP:                 false,
		EnableUDP:                 true,
	}
}
