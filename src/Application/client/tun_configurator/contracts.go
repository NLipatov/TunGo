package tun_configurator

import (
	"tungo/Domain/settings"
	"tungo/Infrastructure/network"
)

type TunConfigurator interface {
	// Configure creates a TunAdapter that can be used as TUN-like device
	Configure(s settings.ConnectionSettings) network.TunAdapter
	// Deconfigure performs de-configuration logic to suspend a TUN-like device
	Deconfigure(s settings.ConnectionSettings)
}
