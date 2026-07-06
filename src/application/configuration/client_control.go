package configuration

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
)

type clientControl struct {
	observer clientConfiguration.Observer
	selector clientConfiguration.Selector
	creator  clientConfiguration.Creator
	deleter  clientConfiguration.Deleter
	manager  clientConfiguration.ConfigurationManager
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
