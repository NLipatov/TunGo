package tui

import appConfiguration "tungo/application/configuration"

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

func (m configurationControlMock) CreateFromJSON(string, string) error {
	return nil
}

func (m configurationControlMock) Delete(string) error {
	return nil
}

func (m configurationControlMock) GenerateClientConfiguration() (string, error) {
	return "", nil
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
