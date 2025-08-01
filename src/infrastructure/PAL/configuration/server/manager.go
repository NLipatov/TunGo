package server

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"time"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/stat"
	"tungo/infrastructure/settings"
)

type ServerConfigurationManager interface {
	Configuration() (*Configuration, error)
	IncrementClientCounter() error
	InjectEdKeys(public ed25519.PublicKey, private ed25519.PrivateKey) error
	InjectSessionTtlIntervals(ttl, interval settings.HumanReadableDuration) error
}

type Manager struct {
	resolver client.Resolver
	reader   Reader
	writer   Writer
	stat     stat.Stat
}

func NewManager(resolver client.Resolver, stat stat.Stat) (ServerConfigurationManager, error) {
	path, pathErr := resolver.Resolve()
	if pathErr != nil {
		return nil, fmt.Errorf("failed to resolve server configuration path: %w", pathErr)
	}

	return NewManagerWithReader(
		resolver,
		NewTTLReader(newDefaultReader(path, stat), time.Minute*15),
		stat,
	)
}

func NewManagerWithReader(
	resolver client.Resolver,
	reader Reader,
	stat stat.Stat,
) (ServerConfigurationManager, error) {
	path, pathErr := resolver.Resolve()
	if pathErr != nil {
		return nil, fmt.Errorf("failed to resolve server configuration path: %w", pathErr)
	}

	return &Manager{
		resolver: resolver,
		writer:   newDefaultWriter(path),
		reader:   reader,
		stat:     stat,
	}, nil
}

func (c *Manager) Configuration() (*Configuration, error) {
	path, pathErr := c.resolver.Resolve()
	if pathErr != nil {
		return nil, fmt.Errorf("failed to read configuration: %w", pathErr)
	}

	_, statErr := c.stat.Stat(path)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			configuration := NewDefaultConfiguration()
			writeErr := c.writer.Write(*configuration)
			if writeErr != nil {
				return nil, fmt.Errorf("could not write default configuration: %w", writeErr)
			}
		} else {
			return nil, statErr
		}
	}

	return c.reader.read()
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
