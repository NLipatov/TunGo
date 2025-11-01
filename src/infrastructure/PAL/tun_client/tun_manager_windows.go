package tun_client

import (
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/windows/manager"
	"tungo/infrastructure/PAL/windows/network_tools/netsh"
	"tungo/infrastructure/PAL/windows/network_tools/route"
	"tungo/infrastructure/settings"
)

type PlatformTunManager struct {
	configuration      client.Configuration
	connectionSettings settings.Settings
	// manager is a backing tun.ClientManager implementation, which handles v4/v6 specific
	manager tun.ClientManager
}

func NewPlatformTunManager(
	configuration client.Configuration,
) (tun.ClientManager, error) {
	connectionSettings, connectionSettingsErr := configuration.ActiveSettings()
	if connectionSettingsErr != nil {
		return nil, connectionSettingsErr
	}
	commander := PAL.NewExecCommander()
	factory := manager.NewFactory(
		connectionSettings,
		commander,
		netsh.NewFactory(connectionSettings, commander),
		route.NewFactory(commander, connectionSettings),
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
