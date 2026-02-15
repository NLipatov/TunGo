//go:build windows

package manager

import (
	"fmt"
	"tungo/infrastructure/PAL/windows/ipcfg"

	"tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"
)

// Factory builds a family-specific TUN manager (IPv4, IPv6, or dual-stack) based on configured addresses.
type Factory struct {
	connectionSettings settings.Settings
	netConfigFactory   ipcfg.Factory
}

func NewFactory(
	connectionSettings settings.Settings,
) *Factory {
	return &Factory{
		connectionSettings: connectionSettings,
		netConfigFactory:   ipcfg.NewFactory(),
	}
}

// Create returns a tun.ClientManager for the configured address families.
func (f *Factory) Create() (tun.ClientManager, error) {
	has4 := f.connectionSettings.IPv4.IsValid() && !f.connectionSettings.IPv4.IsUnspecified() && f.connectionSettings.IPv4.Unmap().Is4()
	has6 := f.connectionSettings.IPv6.IsValid() && !f.connectionSettings.IPv6.IsUnspecified() && !f.connectionSettings.IPv6.Unmap().Is4()

	if has4 {
		return newV4Manager(
			f.connectionSettings,
			f.netConfigFactory.NewV4(),
		), nil
	}
	if has6 {
		return newV6Manager(
			f.connectionSettings,
			f.netConfigFactory.NewV6(),
		), nil
	}
	return nil, fmt.Errorf("no valid IPv4 or IPv6 configured")
}
