//go:build darwin

package client

import (
	"net/netip"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/network/darwin/manager"
	"tungo/infrastructure/settings"
)

type PlatformTunManager struct {
	configuration      client.Configuration
	connectionSettings settings.Settings
	manager            tun.ClientManager
}

func NewPlatformTunManager(configuration client.Configuration) (tun.ClientManager, error) {
	connSettings, err := configuration.ActiveSettings()
	if err != nil {
		return nil, err
	}
	factory := manager.NewFactory(connSettings)
	concrete, err := factory.Create()
	if err != nil {
		return nil, err
	}
	return &PlatformTunManager{
		configuration:      configuration,
		connectionSettings: connSettings,
		manager:            concrete,
	}, nil
}

func (m *PlatformTunManager) CreateDevice() (tun.Device, error) {
	return m.manager.CreateDevice()
}

func (m *PlatformTunManager) DisposeDevices() error {
	return m.manager.DisposeDevices()
}

func (m *PlatformTunManager) SetRouteEndpoint(addr netip.AddrPort) {
	m.manager.SetRouteEndpoint(addr)
}
