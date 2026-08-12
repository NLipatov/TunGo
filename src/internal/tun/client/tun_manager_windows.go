package client

import (
	"io"
	"log/slog"
	"net/netip"
	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/windows/manager"
)

type clientManager interface {
	CreateDevice() (io.ReadWriteCloser, error)
	DisposeDevices() error
	SetRouteEndpoint(netip.AddrPort)
}

type Manager struct {
	manager       clientManager
	activeTunName string
	configuration *clientconfig.Configuration
}

func New(conf *clientconfig.Configuration) (*Manager, error) {
	connectionSettings, err := conf.ActiveSettings()
	if err != nil {
		return nil, err
	}
	concreteManager, concreteManagerErr := manager.New(connectionSettings)
	if concreteManagerErr != nil {
		return nil, concreteManagerErr
	}
	return &Manager{
		manager:       concreteManager,
		activeTunName: connectionSettings.TunName,
		configuration: conf,
	}, nil
}

func (m *Manager) CreateDevice() (io.ReadWriteCloser, error) {
	return m.manager.CreateDevice()
}

func (m *Manager) DisposeDevices() error {
	activeErr := m.manager.DisposeDevices()
	m.disposeStale(m.configuration.TCPSettings)
	m.disposeStale(m.configuration.UDPSettings)
	m.disposeStale(m.configuration.WSSettings)
	return activeErr
}

func (m *Manager) disposeStale(s settings.Settings) {
	if s.TunName == "" || s.TunName == m.activeTunName {
		return
	}
	cleanupManager, err := manager.New(s)
	if err != nil {
		slog.Warn("failed to prepare stale TUN cleanup", "name", s.TunName, "err", err)
		return
	}
	if err := cleanupManager.DisposeDevices(); err != nil {
		slog.Warn("failed to clean stale TUN configuration", "name", s.TunName, "err", err)
	}
}

func (m *Manager) SetRouteEndpoint(addr netip.AddrPort) {
	m.manager.SetRouteEndpoint(addr)
}
