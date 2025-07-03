package server_configuration

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"time"
	"tungo/infrastructure/PAL/client_configuration"
	"tungo/infrastructure/settings"
)

type ServerConfigurationManager interface {
	Configuration() (*Configuration, error)
	IncrementClientCounter() error
	InjectEdKeys(public ed25519.PublicKey, private ed25519.PrivateKey) error
	InjectSessionTtlIntervals(ttl, interval settings.HumanReadableDuration) error
}

type Manager struct {
	resolver client_configuration.Resolver
	reader   Reader
	writer   *writer
}

func NewManager(resolver client_configuration.Resolver) (ServerConfigurationManager, error) {
	path, pathErr := resolver.Resolve()
	if pathErr != nil {
		return nil, fmt.Errorf("failed to resolve server configuration path: %w", pathErr)
	}

	return &Manager{
		resolver: resolver,
		writer:   newWriter(path),
		reader: NewTTLReader(
			newDefaultReader(path), time.Minute*15,
		),
	}, nil
}

func (c *Manager) Configuration() (*Configuration, error) {
	path, pathErr := c.resolver.Resolve()
	if pathErr != nil {
		return nil, fmt.Errorf("failed to read configuration: %s", path)
	}

	_, statErr := os.Stat(path)
	if statErr != nil {
		configuration := NewDefaultConfiguration()
		writeErr := c.writer.Write(*configuration)
		if writeErr != nil {
			return nil, fmt.Errorf("could not write default configuration: %s", writeErr)
		}
	}

	return newDefaultReader(path).read()
}

func (c *Manager) IncrementClientCounter() error {
	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.ClientCounter += 1
	return c.writer.Write(*configuration)
}

func (c *Manager) InjectEdKeys(public ed25519.PublicKey, private ed25519.PrivateKey) error {
	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.Ed25519PublicKey = public
	configuration.Ed25519PrivateKey = private

	return c.writer.Write(*configuration)
}

func (c *Manager) InjectSessionTtlIntervals(ttl, interval settings.HumanReadableDuration) error {
	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.UDPSettings.SessionLifetime.Ttl = ttl
	configuration.UDPSettings.SessionLifetime.CleanupInterval = interval

	configuration.TCPSettings.SessionLifetime.Ttl = ttl
	configuration.TCPSettings.SessionLifetime.CleanupInterval = interval

	return c.writer.Write(*configuration)
}
