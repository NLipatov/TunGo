//go:build windows

package manager

import (
	"fmt"
	"io"
	"net/netip"
	"tungo/infrastructure/PAL/network/windows/ipcfg"

	"tungo/application/configuration/settings"
)

type clientManager interface {
	CreateDevice() (io.ReadWriteCloser, error)
	DisposeDevices() error
	SetRouteEndpoint(netip.AddrPort)
}

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

// Create returns the manager for the configured address families.
func (f *Factory) Create() (clientManager, error) {
	has4 := f.connectionSettings.IPv4.IsValid() && !f.connectionSettings.IPv4.IsUnspecified() && f.connectionSettings.IPv4.Unmap().Is4() ||
		f.connectionSettings.IPv4Subnet.IsValid() && f.connectionSettings.IPv4Subnet.Addr().Unmap().Is4()
	has6 := f.connectionSettings.IPv6.IsValid() && !f.connectionSettings.IPv6.IsUnspecified() && !f.connectionSettings.IPv6.Unmap().Is4() ||
		f.connectionSettings.IPv6Subnet.IsValid() && !f.connectionSettings.IPv6Subnet.Addr().Unmap().Is4()

	if has4 && has6 {
		return newDualStackManager(
			f.connectionSettings,
			f.netConfigFactory.NewV4(),
			f.netConfigFactory.NewV6(),
		), nil
	}
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
