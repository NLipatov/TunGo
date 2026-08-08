//go:build darwin

package client

import (
	"io"
	"log/slog"
	"net/netip"
	"tungo/application/configuration/settings"
	"tungo/infrastructure/PAL/network/darwin/manager"
)

type clientManager interface {
	CreateDevice() (io.ReadWriteCloser, error)
	DisposeDevices() error
	SetRouteEndpoint(netip.AddrPort)
}

type PlatformTunManager struct {
	manager         clientManager
	activeTunName   string
	cleanupSettings []settings.Settings
}

func NewPlatformTunManager(
	connectionSettings settings.Settings,
	cleanupSettings []settings.Settings,
) (*PlatformTunManager, error) {
	factory := manager.NewFactory(connectionSettings)
	concrete, err := factory.Create()
	if err != nil {
		return nil, err
	}
	return &PlatformTunManager{
		manager:         concrete,
		activeTunName:   connectionSettings.TunName,
		cleanupSettings: append([]settings.Settings(nil), cleanupSettings...),
	}, nil
}

func (m *PlatformTunManager) CreateDevice() (io.ReadWriteCloser, error) {
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
