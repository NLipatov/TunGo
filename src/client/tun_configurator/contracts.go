package tun_configurator

import (
	"tungo/network"
	"tungo/settings"
)

type TunConfigurator interface {
	// Configure creates a TunAdapter that can be used as TUN-like device
	Configure(s settings.ConnectionSettings) (network.TunAdapter, error)
	// Deconfigure performs de-configuration logic to suspend a TUN-like device
	Deconfigure(s settings.ConnectionSettings)
}
