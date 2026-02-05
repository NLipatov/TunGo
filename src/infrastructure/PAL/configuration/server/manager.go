package server

import (
	"errors"
	"fmt"
	"os"
	"time"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/stat"
)

type ConfigurationManager interface {
	Configuration() (*Configuration, error)
	IncrementClientCounter() error
	InjectX25519Keys(public, private []byte) error
	AddAllowedPeer(peer AllowedPeer) error
	InvalidateCache()
}

type Manager struct {
	resolver client.Resolver
	reader   Reader
	writer   Writer
	stat     stat.Stat
}

func NewManager(resolver client.Resolver, stat stat.Stat) (ConfigurationManager, error) {
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
) (ConfigurationManager, error) {
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

func (c *Manager) InjectX25519Keys(public, private []byte) error {
	if len(public) != 32 {
		return fmt.Errorf("invalid public key length: got %d, want 32", len(public))
	}
	if len(private) != 32 {
		return fmt.Errorf("invalid private key length: got %d, want 32", len(private))
	}

	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.X25519PublicKey = append([]byte(nil), public...)
	configuration.X25519PrivateKey = append([]byte(nil), private...)

	return c.writer.Write(*configuration)
}

func (c *Manager) AddAllowedPeer(peer AllowedPeer) error {
	if len(peer.PublicKey) != 32 {
		return fmt.Errorf("invalid public key length: got %d, want 32", len(peer.PublicKey))
	}

	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.AllowedPeers = append(configuration.AllowedPeers, peer)

	return c.writer.Write(*configuration)
}

// InvalidateCache clears the cached configuration if the reader supports it.
// Implements CacheInvalidator interface.
func (c *Manager) InvalidateCache() {
	if ttlReader, ok := c.reader.(*TTLReader); ok {
		ttlReader.InvalidateCache()
	}
}
