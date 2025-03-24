package tun_device

import (
	"tungo/application"
	"tungo/settings/client_configuration"
)

type PlatformAgnosticTunDeviceFactory struct {
	goos string
}

func NewTunDeviceConfigurator(conf client_configuration.Configuration) (application.PlatformTunConfigurator, error) {
	return newPlatformTunConfigurator(conf)
}
