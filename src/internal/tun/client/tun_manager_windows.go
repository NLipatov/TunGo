package client

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/windows/manager"
)

type clientManager interface {
	OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error)
	CloseTunnel() error
}

type Manager struct {
	manager       clientManager
	configuration *client.Configuration
}

func New(conf *client.Configuration) (*Manager, error) {
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
		configuration: conf,
	}, nil
}

func (m *Manager) OpenTunnel(serverAddr netip.Addr) (io.ReadWriteCloser, error) {
	return m.manager.OpenTunnel(serverAddr)
}

func (m *Manager) CloseTunnel() error {
	activeErr := m.manager.CloseTunnel()
	active, err := m.configuration.ActiveSettings()
	if err != nil {
		return errors.Join(activeErr, fmt.Errorf("determine active TUN settings: %w", err))
	}
	m.disposeStale(m.configuration.TCPSettings, active.TunName)
	m.disposeStale(m.configuration.UDPSettings, active.TunName)
	m.disposeStale(m.configuration.WSSettings, active.TunName)
	return activeErr
}

func (m *Manager) disposeStale(s settings.Settings, activeTunName string) {
	if s.TunName == "" || s.TunName == activeTunName {
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
