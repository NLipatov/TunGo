package tui

import (
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
)

type cfgObserverMock struct{}

func (m *cfgObserverMock) Observe() ([]string, error) {
	return nil, nil
}

type cfgSelectorMock struct{}

func (m *cfgSelectorMock) Select(string) error {
	return nil
}

type cfgCreatorMock struct{}

func (m *cfgCreatorMock) Create(clientConfiguration.Configuration, string) error {
	return nil
}

type cfgDeleterMock struct{}

func (m *cfgDeleterMock) Delete(string) error {
	return nil
}

type mockManager struct{}

func (m *mockManager) Configuration() (*serverConfiguration.Configuration, error) {
	return nil, nil
}

func (m *mockManager) IncrementClientCounter() error {
	return nil
}

func (m *mockManager) InjectX25519Keys(_, _ []byte) error {
	return nil
}

func (m *mockManager) AddAllowedPeer(serverConfiguration.AllowedPeer) error {
	return nil
}

func (m *mockManager) ListAllowedPeers() ([]serverConfiguration.AllowedPeer, error) {
	return nil, nil
}

func (m *mockManager) SetAllowedPeerEnabled(int, bool) error {
	return nil
}

func (m *mockManager) RemoveAllowedPeer(int) error {
	return nil
}

func (m *mockManager) EnsureIPv6Subnets() error {
	return nil
}

func (m *mockManager) InvalidateCache() {}
