package configuration

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	clientConfiguration "tungo/application/configuration/client"
	"tungo/application/configuration/settings"
)

type clientControl struct {
	observer clientObserver
	selector clientSelector
	creator  clientCreator
	manager  clientConfigurationManager
}

type clientObserver interface {
	Observe() ([]string, error)
}

type clientSelector interface {
	Select(path string) error
}

type clientCreator interface {
	Create(configuration clientConfiguration.Configuration, name string) error
}

type clientConfigurationManager interface {
	Configuration() (*clientConfiguration.Configuration, error)
}

func (c *clientControl) ClientRuntimeConfiguration() (ClientRuntimeConfiguration, error) {
	conf, err := c.manager.Configuration()
	if err != nil {
		return ClientRuntimeConfiguration{}, err
	}
	activeSettings, err := runtimeClientSettings(conf)
	if err != nil {
		return ClientRuntimeConfiguration{}, err
	}
	cleanupSettings := make([]settings.Settings, 0, 3)
	for _, profile := range []settings.Settings{conf.TCPSettings, conf.UDPSettings, conf.WSSettings} {
		if profile.TunName == "" {
			continue
		}
		cleanupSettings = append(cleanupSettings, profile)
	}
	return ClientRuntimeConfiguration{
		Settings:         activeSettings,
		CleanupSettings:  cleanupSettings,
		X25519PublicKey:  append([]byte(nil), conf.X25519PublicKey...),
		ClientPublicKey:  append([]byte(nil), conf.ClientPublicKey...),
		ClientPrivateKey: append([]byte(nil), conf.ClientPrivateKey...),
	}, nil
}

func (c *clientControl) List() ([]string, error) {
	return c.observer.Observe()
}

func (c *clientControl) Select(path string) error {
	return c.selector.Select(path)
}

func (c *clientControl) ValidateActive() error {
	_, err := c.manager.Configuration()
	return err
}

func (c *clientControl) RuntimeInfo() (RuntimeInfo, error) {
	conf, err := c.manager.Configuration()
	if err != nil {
		return RuntimeInfo{}, err
	}
	activeSettings, err := runtimeClientSettings(conf)
	if err != nil {
		return RuntimeInfo{}, err
	}

	info := RuntimeInfo{Protocol: conf.Protocol}
	if endpoint, ok := endpointInfoFromSettings(conf.Protocol, activeSettings); ok {
		info.Endpoints = []EndpointInfo{endpoint}
	}
	return info, nil
}

func runtimeClientSettings(conf *clientConfiguration.Configuration) (settings.Settings, error) {
	active, err := conf.ActiveSettings()
	if err != nil {
		return settings.Settings{}, err
	}
	active.Protocol = conf.Protocol
	if err := active.DeriveIP(conf.ClientID); err != nil {
		return settings.Settings{}, err
	}
	return active, nil
}

func (c *clientControl) CreateFromJSON(name, rawJSON string) error {
	configuration, err := parseClientConfigurationJSON(rawJSON)
	if err != nil {
		return err
	}
	return c.creator.Create(configuration, name)
}

func (c *clientControl) Delete(path string) error {
	return os.Remove(path)
}

func parseClientConfigurationJSON(input string) (clientConfiguration.Configuration, error) {
	clean := strings.TrimFunc(input, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.In(r, unicode.Cf)
	})
	var cfg clientConfiguration.Configuration
	if err := json.Unmarshal([]byte(clean), &cfg); err != nil {
		return clientConfiguration.Configuration{}, fmt.Errorf("invalid client configuration: %w", err)
	}
	if err := clientConfiguration.Validate(cfg); err != nil {
		return clientConfiguration.Configuration{}, fmt.Errorf("invalid client configuration: %w", err)
	}
	return cfg, nil
}
