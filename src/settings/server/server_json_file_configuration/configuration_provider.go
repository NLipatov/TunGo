package server_json_file_configuration

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"tungo/settings/server"
)

type ServerConfigurationManager struct {
	resolver resolver
}

func NewServerConfigurationManager() *ServerConfigurationManager {
	return &ServerConfigurationManager{
		resolver: newPathResolver(),
	}
}

func (c *ServerConfigurationManager) Configuration() (*server.Configuration, error) {
	path, pathErr := c.resolver.resolve()
	if pathErr != nil {
		return nil, fmt.Errorf("failed to read configuration: %s", path)
	}

	_, statErr := os.Stat(path)
	if statErr != nil {
		configuration := server.NewDefaultConfiguration()
		w := newWriter(path)
		writeErr := w.Write(*configuration)
		if writeErr != nil {
			return nil, fmt.Errorf("could not write default configuration: %s", writeErr)
		}
	}
	return newReader(path).read()
}

func (c *ServerConfigurationManager) IncrementClientCounter() error {
	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.ClientCounter += 1
	path, pathErr := c.resolver.resolve()
	if pathErr != nil {
		return pathErr
	}

	w := newWriter(path)
	return w.Write(*configuration)
}

func (c *ServerConfigurationManager) InjectEdKeys(public ed25519.PublicKey, private ed25519.PrivateKey) error {
	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.Ed25519PublicKey = public
	configuration.Ed25519PrivateKey = private

	path, pathErr := c.resolver.resolve()
	if pathErr != nil {
		return pathErr
	}

	w := newWriter(path)
	return w.Write(*configuration)
}
