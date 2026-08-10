//go:build darwin

package client

import (
	"io"
	"log/slog"
	"net/netip"
	clientConfiguration "tungo/application/configuration/client"
	"tungo/application/configuration/settings"
	"tungo/infrastructure/PAL/network/darwin/manager"
)

type clientManager interface {
	CreateDevice() (io.ReadWriteCloser, error)
	DisposeDevices() error
	SetRouteEndpoint(netip.AddrPort)
}

type PlatformTunManager struct {
	manager       clientManager
	activeTunName string
	configuration *clientConfiguration.Configuration
}

func NewPlatformTunManager(conf *clientConfiguration.Configuration) (*PlatformTunManager, error) {
	connectionSettings, err := selectedSettings(conf)
	if err != nil {
		return nil, err
	}
	factory := manager.NewFactory(connectionSettings)
	concrete, err := factory.Create()
	if err != nil {
		return nil, err
	}
	return &PlatformTunManager{
		manager:       concrete,
		activeTunName: connectionSettings.TunName,
		configuration: conf,
	}, nil
}

func (m *PlatformTunManager) CreateDevice() (io.ReadWriteCloser, error) {
	return m.manager.CreateDevice()
}

func (m *PlatformTunManager) DisposeDevices() error {
	activeErr := m.manager.DisposeDevices()
	m.disposeStale(m.configuration.TCPSettings)
	m.disposeStale(m.configuration.UDPSettings)
	m.disposeStale(m.configuration.WSSettings)
	return activeErr
}

func (m *PlatformTunManager) disposeStale(s settings.Settings) {
	if s.TunName == "" || s.TunName == m.activeTunName {
		return
	}
	cleanupManager, err := manager.NewFactory(s).Create()
	if err != nil {
		slog.Warn("failed to prepare stale TUN cleanup", "name", s.TunName, "err", err)
		return
	}
	if err := cleanupManager.DisposeDevices(); err != nil {
		slog.Warn("failed to clean stale TUN configuration", "name", s.TunName, "err", err)
	}
}

func (m *PlatformTunManager) SetRouteEndpoint(addr netip.AddrPort) {
	m.manager.SetRouteEndpoint(addr)
}
