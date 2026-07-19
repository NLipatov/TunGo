package configuration

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
)

type clientControl struct {
	observer clientObserver
	selector clientSelector
	creator  clientCreator
	deleter  clientDeleter
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

type clientDeleter interface {
	Delete(path string) error
}

type clientConfigurationManager interface {
	Configuration() (*clientConfiguration.Configuration, error)
}

func (c *clientControl) ClientRuntimeConfiguration() (ClientRuntimeConfiguration, error) {
	conf, err := c.manager.Configuration()
	if err != nil {
		return ClientRuntimeConfiguration{}, err
	}
	if err := conf.ResolveActive(); err != nil {
		return ClientRuntimeConfiguration{}, err
	}
	return ClientRuntimeConfiguration{
		ClientID:         conf.ClientID,
		TCPSettings:      conf.TCPSettings,
		UDPSettings:      conf.UDPSettings,
		WSSettings:       conf.WSSettings,
		X25519PublicKey:  append([]byte(nil), conf.X25519PublicKey...),
		Protocol:         conf.Protocol,
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
	if err := conf.ResolveActive(); err != nil {
		return RuntimeInfo{}, err
	}
	activeSettings, err := conf.ActiveSettings()
	if err != nil {
		return RuntimeInfo{}, err
	}

	info := RuntimeInfo{Protocol: conf.Protocol}
	if endpoint, ok := endpointInfoFromSettings(conf.Protocol, activeSettings); ok {
		info.Endpoints = []EndpointInfo{endpoint}
	}
	return info, nil
}

func (c *clientControl) CreateFromJSON(name, rawJSON string) error {
	configuration, err := parseClientConfigurationJSON(rawJSON)
	if err != nil {
		return err
	}
	return c.creator.Create(configuration, name)
}

func (c *clientControl) Delete(path string) error {
	return c.deleter.Delete(path)
}

func parseClientConfigurationJSON(input string) (clientConfiguration.Configuration, error) {
	sanitized := sanitizeConfigurationJSON(input)
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

func sanitizeConfigurationJSON(s string) string {
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
