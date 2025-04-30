package handshake

import "tungo/settings/client_configuration"

type ClientConf interface {
	ServerEd25519PublicKey() ([]byte, error)
}

type DefaultClientConf struct {
	configurationManager client_configuration.ClientConfigurationManager
}

func NewDefaultClientConf(configurationManager client_configuration.ClientConfigurationManager) ClientConf {
	return &DefaultClientConf{
		configurationManager: configurationManager,
	}
}

func (c *DefaultClientConf) ServerEd25519PublicKey() ([]byte, error) {
	configuration, configurationErr := c.configurationManager.Configuration()
	if configurationErr != nil {
		return nil, configurationErr
	}

	return configuration.Ed25519PublicKey, nil
}
