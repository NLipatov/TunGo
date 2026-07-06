package bubble_tea

import (
	"encoding/json"
	"fmt"
	"strings"
	appConfiguration "tungo/application/configuration"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"unicode"
)

type sessionClientObserver interface {
	Observe() ([]string, error)
}

type sessionClientSelector interface {
	Select(string) error
}

type sessionClientCreator interface {
	Create(clientConfiguration.Configuration, string) error
}

type sessionClientDeleter interface {
	Delete(string) error
}

type sessionClientManager interface {
	Configuration() (*clientConfiguration.Configuration, error)
}

type sessionServerPeerManager interface {
	ListAllowedPeers() ([]serverConfiguration.AllowedPeer, error)
	SetAllowedPeerEnabled(int, bool) error
	RemoveAllowedPeer(int) error
}

type sessionConfigurationControl struct {
	Observer            sessionClientObserver
	Selector            sessionClientSelector
	Creator             sessionClientCreator
	Deleter             sessionClientDeleter
	ClientConfigManager sessionClientManager
	ServerConfigManager sessionServerPeerManager
	generatePath        string
	generateErr         error
}

func defaultSessionConfigurationControl() *sessionConfigurationControl {
	return &sessionConfigurationControl{
		Observer:            sessionObserverStub{},
		Selector:            sessionSelectorStub{},
		Creator:             sessionCreatorStub{},
		Deleter:             sessionDeleterStub{},
		ClientConfigManager: sessionClientConfigManagerStub{},
		ServerConfigManager: &sessionServerConfigManagerStub{
			peers: []serverConfiguration.AllowedPeer{
				{Name: "test", ClientID: 1, Enabled: true},
			},
		},
		generatePath: "/tmp/client_configuration.json.1",
	}
}

func sessionOptionsWithControl(control *sessionConfigurationControl) ConfiguratorSessionOptions {
	return ConfiguratorSessionOptions{
		ClientConfigurationControl: control,
		ServerConfigurationControl: control,
		ServerSupported:            true,
	}
}

func (o *ConfiguratorSessionOptions) testControl() *sessionConfigurationControl {
	control, ok := o.ClientConfigurationControl.(*sessionConfigurationControl)
	if !ok || control == nil {
		control = defaultSessionConfigurationControl()
		o.ClientConfigurationControl = control
		o.ServerConfigurationControl = control
		o.ServerSupported = true
	}
	return control
}

func (c *sessionConfigurationControl) List() ([]string, error) {
	if c.Observer == nil {
		return nil, nil
	}
	return c.Observer.Observe()
}

func (c *sessionConfigurationControl) Select(path string) error {
	if c.Selector == nil {
		return nil
	}
	return c.Selector.Select(path)
}

func (c *sessionConfigurationControl) ValidateActive() error {
	if c.ClientConfigManager == nil {
		return nil
	}
	_, err := c.ClientConfigManager.Configuration()
	return err
}

func (c *sessionConfigurationControl) CreateFromJSON(name, rawJSON string) error {
	if c.Creator == nil {
		return nil
	}
	configuration, err := parseTestClientConfigurationJSON(rawJSON)
	if err != nil {
		return err
	}
	return c.Creator.Create(configuration, name)
}

func (c *sessionConfigurationControl) Delete(path string) error {
	if c.Deleter == nil {
		return nil
	}
	return c.Deleter.Delete(path)
}

func (c *sessionConfigurationControl) GenerateClientConfiguration() (string, error) {
	if c.generateErr != nil {
		return "", c.generateErr
	}
	if c.generatePath != "" {
		return c.generatePath, nil
	}
	return "/tmp/client_configuration.json.1", nil
}

func (c *sessionConfigurationControl) ListPeers() ([]appConfiguration.ServerPeer, error) {
	if c.ServerConfigManager == nil {
		return nil, nil
	}
	peers, err := c.ServerConfigManager.ListAllowedPeers()
	if err != nil {
		return nil, err
	}
	result := make([]appConfiguration.ServerPeer, len(peers))
	for i := range peers {
		result[i] = appConfiguration.ServerPeer{
			Name:      peers[i].Name,
			PublicKey: append([]byte(nil), peers[i].PublicKey...),
			Enabled:   peers[i].Enabled,
			ClientID:  peers[i].ClientID,
		}
	}
	return result, nil
}

func (c *sessionConfigurationControl) SetPeerEnabled(clientID int, enabled bool) error {
	if c.ServerConfigManager == nil {
		return nil
	}
	return c.ServerConfigManager.SetAllowedPeerEnabled(clientID, enabled)
}

func (c *sessionConfigurationControl) RemovePeer(clientID int) error {
	if c.ServerConfigManager == nil {
		return nil
	}
	return c.ServerConfigManager.RemoveAllowedPeer(clientID)
}

func parseTestClientConfigurationJSON(input string) (clientConfiguration.Configuration, error) {
	sanitized := sanitizeTestConfigurationJSON(input)
	clean := strings.TrimSpace(sanitized)
	var cfg clientConfiguration.Configuration
	if err := json.Unmarshal([]byte(clean), &cfg); err != nil {
		return clientConfiguration.Configuration{}, fmt.Errorf("invalid client configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return clientConfiguration.Configuration{}, fmt.Errorf("invalid client configuration: %w", err)
	}
	return cfg, nil
}

func sanitizeTestConfigurationJSON(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(r)
		case unicode.IsControl(r) || unicode.In(r, unicode.Cf):
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sessionServerPeer(peer serverConfiguration.AllowedPeer) appConfiguration.ServerPeer {
	return appConfiguration.ServerPeer{
		Name:      peer.Name,
		PublicKey: append([]byte(nil), peer.PublicKey...),
		Enabled:   peer.Enabled,
		ClientID:  peer.ClientID,
	}
}

func sessionServerPeers(peers []serverConfiguration.AllowedPeer) []appConfiguration.ServerPeer {
	result := make([]appConfiguration.ServerPeer, len(peers))
	for i := range peers {
		result[i] = sessionServerPeer(peers[i])
	}
	return result
}
