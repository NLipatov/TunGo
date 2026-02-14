//go:build windows

package manager

import (
	"fmt"
	"tungo/infrastructure/PAL/windows/ipcfg"

	"tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"
)

// Factory builds a family-specific TUN manager (IPv4 or IPv6) based on InterfaceIP.
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

// Create returns a tun.ClientManager specialized for IPv4 or IPv6.
func (f *Factory) Create() (tun.ClientManager, error) {
	ifAddr := f.connectionSettings.InterfaceIP
	if !ifAddr.IsValid() {
		return nil, fmt.Errorf("invalid InterfaceIP: %q", ifAddr)
	}
	if ifAddr.IsUnspecified() {
		return nil, fmt.Errorf("unspecified InterfaceIP is not allowed: %q", ifAddr)
	}

	if ifAddr.Unmap().Is4() {
		return newV4Manager(
			f.connectionSettings,
			f.netConfigFactory.NewV4(),
		), nil
	}
	return newV6Manager(
		f.connectionSettings,
		f.netConfigFactory.NewV6(),
	), nil
}
