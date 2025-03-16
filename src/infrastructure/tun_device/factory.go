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

func NewTunDevice(conf client.Conf) (application.TunDevice, error) {
	switch runtime.GOOS {
	case "linux":
		return newLinuxTunDeviceManager(conf)
	default:
		return nil, fmt.Errorf("unsupported platfrom: %s", runtime.GOOS)
	}
}
