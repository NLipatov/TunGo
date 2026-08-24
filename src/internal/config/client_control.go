package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
)

type clientControl struct {
	observer clientObserver
	selector clientSelector
	creator  clientCreator
	manager  clientconfigManager
}

type clientObserver interface {
	Observe() ([]string, error)
}

type clientSelector interface {
	Select(path string) error
}

type clientCreator interface {
	Create(configuration clientconfig.Configuration, name string) error
}

type clientconfigManager interface {
	Configuration() (*clientconfig.Configuration, error)
}

func (c *clientControl) Configuration() (*clientconfig.Configuration, error) {
	return c.manager.Configuration()
}

func (c *clientControl) List() ([]string, error) {
	return c.observer.Observe()
}

func (c *clientControl) Select(path string) error {
	return c.selector.Select(path)
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

func runtimeClientSettings(conf *clientconfig.Configuration) (settings.Settings, error) {
	return conf.ActiveSettings()
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

func parseClientConfigurationJSON(input string) (clientconfig.Configuration, error) {
	clean := strings.TrimFunc(input, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || unicode.In(r, unicode.Cf)
	})
	var cfg clientconfig.Configuration
	if err := json.Unmarshal([]byte(clean), &cfg); err != nil {
		return clientconfig.Configuration{}, fmt.Errorf("invalid client configuration: %w", err)
	}
	cfg.ApplyClientDefaults()
	if err := clientconfig.Validate(cfg); err != nil {
		return clientconfig.Configuration{}, fmt.Errorf("invalid client configuration: %w", err)
	}
	return cfg, nil
}
