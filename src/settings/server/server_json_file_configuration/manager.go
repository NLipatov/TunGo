package server_json_file_configuration

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"tungo/settings/server"
)

type Manager struct {
	resolver ConfigurationResolver
}

func NewManager() *Manager {
	return &Manager{
		resolver: newResolver(),
	}
}

func (c *Manager) Configuration() (*server.Configuration, error) {
	path, pathErr := c.resolver.resolve()
	if pathErr != nil {
		return nil, fmt.Errorf("failed to read configuration: %s", path)
	}

	_, statErr := os.Stat(path)
	if statErr != nil {
		configuration := server.NewDefaultConfiguration()
		w := newWriter(c.resolver)
		writeErr := w.Write(*configuration)
		if writeErr != nil {
			return nil, fmt.Errorf("could not write default configuration: %s", writeErr)
		}
	}
	return newReader(path).read()
}

func (c *Manager) IncrementClientCounter() error {
	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.ClientCounter += 1
	w := newWriter(c.resolver)
	return w.Write(*configuration)
}

func (c *Manager) InjectEdKeys(public ed25519.PublicKey, private ed25519.PrivateKey) error {
	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.Ed25519PublicKey = public
	configuration.Ed25519PrivateKey = private

	w := newWriter(c.resolver)
	return w.Write(*configuration)
}
