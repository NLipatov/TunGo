package tui

import (
	"context"
	appConfiguration "tungo/application/configuration"
	"tungo/infrastructure/settings"
)

type configurationControlMock struct{}

func configurationControlsMock(serverSupported bool) appConfiguration.Controls {
	controls := appConfiguration.Controls{Client: configurationControlMock{}}
	if serverSupported {
		controls.Server = configurationControlMock{}
	}
	return controls
}

func (m configurationControlMock) List() ([]string, error) {
	return nil, nil
}

func (m configurationControlMock) Select(string) error {
	return nil
}

func (m configurationControlMock) ValidateActive() error {
	return nil
}

func (m configurationControlMock) RuntimeInfo() (appConfiguration.RuntimeInfo, error) {
	return appConfiguration.RuntimeInfo{Protocol: settings.TCP}, nil
}

func (m configurationControlMock) CreateFromJSON(string, string) error {
	return nil
}

func (m configurationControlMock) Delete(string) error {
	return nil
}

func (m configurationControlMock) ClientRuntimeConfiguration() (appConfiguration.ClientRuntimeConfiguration, error) {
	return appConfiguration.ClientRuntimeConfiguration{}, nil
}

func (m configurationControlMock) GenerateClientConfiguration() (appConfiguration.GeneratedClientConfiguration, error) {
	return appConfiguration.GeneratedClientConfiguration{}, nil
}

func (m configurationControlMock) ListPeers() ([]appConfiguration.ServerPeer, error) {
	return nil, nil
}

func (m configurationControlMock) SetPeerEnabled(int, bool) error {
	return nil
}

func (m configurationControlMock) RemovePeer(int) error {
	return nil
}

func (m configurationControlMock) ServerRuntimeConfiguration() (appConfiguration.ServerRuntimeConfiguration, error) {
	return appConfiguration.ServerRuntimeConfiguration{}, nil
}

func (m configurationControlMock) WatchServerRuntimeConfiguration(
	context.Context,
	appConfiguration.ServerSessionRevoker,
	appConfiguration.ServerAllowedPeersUpdater,
) {
}
