package tun_device

import (
	"fmt"
	"runtime"
	"tungo/application"
	"tungo/settings/client"
)

type PlatformAgnosticTunDeviceFactory struct {
	goos string
}

func NewTunDeviceConfigurator(conf client.Conf) (application.PlatformTunConfigurator, error) {
	switch runtime.GOOS {
	case "linux":
		return newLinuxTunDeviceManager(conf), nil
	default:
		return nil, fmt.Errorf("unsupported platfrom: %s", runtime.GOOS)
	}
}
