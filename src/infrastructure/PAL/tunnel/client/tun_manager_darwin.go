//go:build darwin

package client

import (
	"net/netip"
	appConfiguration "tungo/application/configuration"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/network/darwin/manager"
	"tungo/infrastructure/settings"
)

type PlatformTunManager struct {
	configuration      appConfiguration.ClientRuntimeConfiguration
	connectionSettings settings.Settings
	manager            tun.ClientManager
}

func NewPlatformTunManager(configuration appConfiguration.ClientRuntimeConfiguration) (tun.ClientManager, error) {
	if networkExtensionManager, ok := newNetworkExtensionTunManager(); ok {
		return networkExtensionManager, nil
	}

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
