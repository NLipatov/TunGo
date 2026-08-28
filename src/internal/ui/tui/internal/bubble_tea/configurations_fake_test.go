package bubble_tea

import (
	clientconfig "tungo/internal/config/client"
	serverconfig "tungo/internal/config/server"
)

type testConfigurationControl struct {
	clientConfigs        []string
	listErr              error
	activated            string
	activateErr          error
	activeErr            error
	importCalled         bool
	importErr            error
	deleted              []string
	deleteErr            error
	generatePath         string
	generateErr          error
	peers                []serverconfig.AllowedPeer
	listPeersErr         error
	listPeersErrOnRemove error
	setEnabledErr        error
	removeErr            error
	removeCalls          int
	lastRemoved          int
}

func newTestConfigurationControl() *testConfigurationControl {
	return &testConfigurationControl{
		generatePath: "/tmp/client_configuration.json.1",
		peers: []serverconfig.AllowedPeer{
			{Name: "test", ClientID: 1, Enabled: true},
		},
	}
}

func testConfiguratorOptions(
	client *testConfigurationControl,
	server ...*testConfigurationControl,
) ConfiguratorOptions {
	serverControl := client
	if len(server) > 0 {
		serverControl = server[0]
	}
	return ConfiguratorOptions{
		ClientConfigurations: testClientConfigurations{client},
		ServerConfigurations: testServerConfigurations{serverControl},
	}
}

func (o *ConfiguratorOptions) testControl() *testConfigurationControl {
	adapter, ok := o.ClientConfigurations.(testClientConfigurations)
	control := adapter.testConfigurationControl
	if !ok || control == nil {
		control = newTestConfigurationControl()
		o.ClientConfigurations = testClientConfigurations{control}
		o.ServerConfigurations = testServerConfigurations{control}
	}
	return control
}

type testClientConfigurations struct{ *testConfigurationControl }

func (c testClientConfigurations) Active() (*clientconfig.Configuration, error) {
	return &clientconfig.Configuration{}, c.activeErr
}

func (c testClientConfigurations) Import(string, string) error {
	c.importCalled = true
	return c.importErr
}

type testServerConfigurations struct{ *testConfigurationControl }

func (c testServerConfigurations) GenerateClient() (serverconfig.GeneratedClient, error) {
	if c.generateErr != nil {
		return serverconfig.GeneratedClient{}, c.generateErr
	}
	return serverconfig.GeneratedClient{Path: c.generatePath}, nil
}

func (c testServerConfigurations) Peers() ([]serverconfig.AllowedPeer, error) {
	if c.listPeersErr != nil {
		return nil, c.listPeersErr
	}
	peers := make([]serverconfig.AllowedPeer, len(c.peers))
	for i := range c.peers {
		peers[i] = c.peers[i]
		peers[i].PublicKey = append([]byte(nil), c.peers[i].PublicKey...)
	}
	return peers, nil
}

func (c *testConfigurationControl) List() ([]string, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return append([]string(nil), c.clientConfigs...), nil
}

func (c *testConfigurationControl) Activate(name string) error {
	c.activated = name
	return c.activateErr
}

func (c *testConfigurationControl) Delete(name string) error {
	c.deleted = append(c.deleted, name)
	return c.deleteErr
}

func (c *testConfigurationControl) SetPeerEnabled(clientID int, enabled bool) error {
	if c.setEnabledErr != nil {
		return c.setEnabledErr
	}
	for i := range c.peers {
		if c.peers[i].ClientID == clientID {
			c.peers[i].Enabled = enabled
			return nil
		}
	}
	return nil
}

func (c *testConfigurationControl) RemovePeer(clientID int) error {
	c.removeCalls++
	c.lastRemoved = clientID
	if c.removeErr != nil {
		return c.removeErr
	}
	for i := range c.peers {
		if c.peers[i].ClientID == clientID {
			c.peers = append(c.peers[:i], c.peers[i+1:]...)
			if c.listPeersErrOnRemove != nil {
				c.listPeersErr = c.listPeersErrOnRemove
			}
			return nil
		}
	}
	return nil
}
