package tun_device

import (
	"fmt"
	"tungo/application"
	"tungo/settings/client_configuration"
)

func newPlatformTunConfigurator(conf client_configuration.Configuration) (application.PlatformTunConfigurator, error) {
	return nil, fmt.Errorf("not implemented on Windows")
}
