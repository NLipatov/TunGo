package client

import (
	"log/slog"
	"net/netip"
	"tungo/application/configuration/settings"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/network/windows/manager"
)

type PlatformTunManager struct {
	// manager is a backing tun.ClientManager implementation, which handles v4/v6 specific
	manager         tun.ClientManager
	activeTunName   string
	cleanupSettings []settings.Settings
}

func NewPlatformTunManager(
	connectionSettings settings.Settings,
	cleanupSettings []settings.Settings,
) (tun.ClientManager, error) {
	factory := manager.NewFactory(
		connectionSettings,
	)
	concreteManager, concreteManagerErr := factory.Create()
	if concreteManagerErr != nil {
		return nil, concreteManagerErr
	}
	return &PlatformTunManager{
		manager:         concreteManager,
		activeTunName:   connectionSettings.TunName,
		cleanupSettings: append([]settings.Settings(nil), cleanupSettings...),
	}, nil
}

func (m *PlatformTunManager) CreateDevice() (tun.Device, error) {
	return m.manager.CreateDevice()
}

func (m *PlatformTunManager) DisposeDevices() error {
	activeErr := m.manager.DisposeDevices()
	for _, s := range m.cleanupSettings {
		if s.TunName == "" || s.TunName == m.activeTunName {
			continue
		}
		cleanupManager, err := manager.NewFactory(s).Create()
		if err != nil {
			slog.Warn("failed to prepare stale TUN cleanup", "name", s.TunName, "err", err)
			continue
		}
		if err := cleanupManager.DisposeDevices(); err != nil {
			slog.Warn("failed to clean stale TUN configuration", "name", s.TunName, "err", err)
		}
	}
	return activeErr
}

func (m *PlatformTunManager) SetRouteEndpoint(addr netip.AddrPort) {
	m.manager.SetRouteEndpoint(addr)
}
