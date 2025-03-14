package tun_configurator

import (
	"fmt"
	"runtime"
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
