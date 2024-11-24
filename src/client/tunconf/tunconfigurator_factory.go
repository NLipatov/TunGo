package tunconf

import (
	"fmt"
	"runtime"
	"tungo/network"
	"tungo/settings"
)

type (
	TunConfigurator interface {
		Configure(s settings.ConnectionSettings) network.TunAdapter
		Deconfigure(s settings.ConnectionSettings)
	}
)

type TunConfiguratorFactory struct{}

func NewTunConfiguratorFactory() *TunConfiguratorFactory {
	return &TunConfiguratorFactory{}
}

func (f *TunConfiguratorFactory) CreateTunConfigurator() (TunConfigurator, error) {
	platform := runtime.GOOS

	switch platform {
	case "linux":
		return &LinuxTunConfigurator{}, nil
	default:

		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
}
