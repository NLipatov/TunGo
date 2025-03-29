package tun_device

import (
	"tungo/application"
	"tungo/settings/client_configuration"
)

// AbstractTunFactory is used to configure platform specific PlatformTunConfigurator
type AbstractTunFactory struct {
}

func NewAbstractTunFactory(
	conf client_configuration.Configuration,
) (application.PlatformTunConfigurator, error) {
	return newPlatformTunConfigurator(conf)
}
