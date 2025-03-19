package server_json_file_configuration

import (
	"crypto/ed25519"
	"fmt"
	"log"
	"os"
	"tungo/settings/server"
)

type ServerConfigurationManager struct {
}

func NewServerConfigurationManager() *ServerConfigurationManager {
	return &ServerConfigurationManager{}
}

func (c *ServerConfigurationManager) Configuration() (*server.Configuration, error) {
	configurationPathResolver := newPathResolver()
	configurationPath, configurationPathErr := configurationPathResolver.resolve()
	_, statErr := os.Stat(configurationPath)
	if statErr != nil {
		configuration := server.NewDefaultConfiguration()
		w := newWriter(configurationPathResolver)
		writeErr := w.Write(*configuration)
		if writeErr != nil {
			log.Fatalf("could not write default configuration: %s", writeErr)
		}
	}

	if configurationPathErr != nil {
		return nil, fmt.Errorf("failed to read configuration: %s", configurationPath)
	}

	return newReader(configurationPath).read()
}

func (c *ServerConfigurationManager) IncrementClientCounter() error {
	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.ClientCounter += 1
	configurationPathResolver := newPathResolver()
	return newWriter(configurationPathResolver).Write(*configuration)
}

func (c *ServerConfigurationManager) InjectEdKeys(public ed25519.PublicKey, private ed25519.PrivateKey) error {
	configuration, configurationErr := c.Configuration()
	if configurationErr != nil {
		return configurationErr
	}

	configuration.Ed25519PublicKey = public
	configuration.Ed25519PrivateKey = private

	configurationPathResolver := newPathResolver()
	return newWriter(configurationPathResolver).Write(*configuration)
}
