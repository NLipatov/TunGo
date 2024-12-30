package tun_configurator

import (
	"tungo/Application/boundary"
	"tungo/Domain/settings"
)

type TunConfigurator interface {
	// Configure creates a TunAdapter that can be used as TUN-like device
	Configure(s settings.ConnectionSettings) boundary.TunAdapter
	// Deconfigure performs de-configuration logic to suspend a TUN-like device
	Deconfigure(s settings.ConnectionSettings)
}
