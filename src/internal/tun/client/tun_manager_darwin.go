//go:build darwin

package client

import (
	"io"
	"log/slog"
	"net/netip"
	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/darwin/manager"
)

type clientManager interface {
	OpenTunnel() (io.ReadWriteCloser, error)
	CloseTunnel() error
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
	concrete, err := manager.New(connectionSettings)
	if err != nil {
		return nil, err
	}
	return &Manager{
		manager:       concrete,
		activeTunName: connectionSettings.TunName,
		configuration: conf,
	}, nil
}

func (m *Manager) OpenTunnel() (io.ReadWriteCloser, error) {
	return m.manager.OpenTunnel()
}

func (m *Manager) CloseTunnel() error {
	activeErr := m.manager.CloseTunnel()
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
	if err := cleanupManager.CloseTunnel(); err != nil {
		slog.Warn("failed to clean stale TUN configuration", "name", s.TunName, "err", err)
	}
}

func (m *Manager) SetRouteEndpoint(addr netip.AddrPort) {
	m.manager.SetRouteEndpoint(addr)
}
