package client

import (
	"net/netip"
	appConfiguration "tungo/application/configuration"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/network/windows/manager"
	"tungo/infrastructure/settings"
)

type PlatformTunManager struct {
	configuration      appConfiguration.ClientRuntimeConfiguration
	connectionSettings settings.Settings
	// manager is a backing tun.ClientManager implementation, which handles v4/v6 specific
	manager tun.ClientManager
}

func NewPlatformTunManager(
	configuration appConfiguration.ClientRuntimeConfiguration,
) (tun.ClientManager, error) {
	connectionSettings, connectionSettingsErr := configuration.ActiveSettings()
	if connectionSettingsErr != nil {
		return nil, connectionSettingsErr
	}
	factory := manager.NewFactory(
		connectionSettings,
	)
	concreteManager, concreteManagerErr := factory.Create()
	if concreteManagerErr != nil {
		return nil, concreteManagerErr
	}
	return &PlatformTunManager{
		configuration:      configuration,
		connectionSettings: connectionSettings,
		manager:            concreteManager,
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
