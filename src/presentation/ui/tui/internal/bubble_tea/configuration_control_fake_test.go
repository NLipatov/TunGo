package bubble_tea

import appConfiguration "tungo/application/configuration"

type testConfigurationControl struct {
	clientConfigs        []string
	listErr              error
	selected             string
	selectErr            error
	validateActiveErr    error
	createCalled         bool
	createErr            error
	deleted              []string
	deleteErr            error
	generatePath         string
	generateErr          error
	peers                []appConfiguration.ServerPeer
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
		peers: []appConfiguration.ServerPeer{
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
		ClientConfigurationControl: client,
		ServerConfigurationControl: serverControl,
	}
}

func (o *ConfiguratorOptions) testControl() *testConfigurationControl {
	control, ok := o.ClientConfigurationControl.(*testConfigurationControl)
	if !ok || control == nil {
		control = newTestConfigurationControl()
		o.ClientConfigurationControl = control
		o.ServerConfigurationControl = control
	}
	return control
}

func (c *testConfigurationControl) List() ([]string, error) {
	if c.listErr != nil {
		return nil, c.listErr
	}
	return append([]string(nil), c.clientConfigs...), nil
}

func (c *testConfigurationControl) Select(path string) error {
	c.selected = path
	return c.selectErr
}

func (c *testConfigurationControl) ValidateActive() error {
	return c.validateActiveErr
}

func (c *testConfigurationControl) RuntimeInfo() (appConfiguration.RuntimeInfo, error) {
	return appConfiguration.RuntimeInfo{}, nil
}

func (c *testConfigurationControl) CreateFromJSON(string, string) error {
	c.createCalled = true
	return c.createErr
}

func (c *testConfigurationControl) Delete(path string) error {
	c.deleted = append(c.deleted, path)
	return c.deleteErr
}

func (c *testConfigurationControl) GenerateClientConfiguration() (appConfiguration.GeneratedClientConfiguration, error) {
	if c.generateErr != nil {
		return appConfiguration.GeneratedClientConfiguration{}, c.generateErr
	}
	return appConfiguration.GeneratedClientConfiguration{Path: c.generatePath}, nil
}

func (c *testConfigurationControl) ListPeers() ([]appConfiguration.ServerPeer, error) {
	if c.listPeersErr != nil {
		return nil, c.listPeersErr
	}
	peers := make([]appConfiguration.ServerPeer, len(c.peers))
	for i := range c.peers {
		peers[i] = c.peers[i]
		peers[i].PublicKey = append([]byte(nil), c.peers[i].PublicKey...)
	}
	return peers, nil
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
