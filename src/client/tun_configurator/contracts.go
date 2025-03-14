package tun_configurator

import (
	"tungo/application"
	"tungo/settings"
)

type TunConfigurator interface {
	// Configure creates a TunAdapter that can be used as TUN-like device
	Configure(s settings.ConnectionSettings) (application.TunDevice, error)
	// Deconfigure performs de-configuration logic to suspend a TUN-like device
	Deconfigure(s settings.ConnectionSettings)
}
