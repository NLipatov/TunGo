package server

import (
	"tungo/application"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
)

type AppDependencies interface {
	Configuration() serverConfiguration.Configuration
	TunManager() application.ServerTunManager
	KeyManager() serverConfiguration.KeyManager
	ConfigurationManager() serverConfiguration.ServerConfigurationManager
}

type Dependencies struct {
	configuration        serverConfiguration.Configuration
	tunManager           application.ServerTunManager
	keyManager           serverConfiguration.KeyManager
	configurationManager serverConfiguration.ServerConfigurationManager
}

func NewDependencies(
	tunManager application.ServerTunManager,
	configuration serverConfiguration.Configuration,
	keyManager serverConfiguration.KeyManager,
	configurationManager serverConfiguration.ServerConfigurationManager,
) AppDependencies {
	return &Dependencies{
		configuration:        configuration,
		tunManager:           tunManager,
		keyManager:           keyManager,
		configurationManager: configurationManager,
	}
}

func (s Dependencies) Configuration() serverConfiguration.Configuration {
	return s.configuration
}

func (s Dependencies) TunManager() application.ServerTunManager {
	return s.tunManager
}

func (s Dependencies) KeyManager() serverConfiguration.KeyManager {
	return s.keyManager
}

func (s Dependencies) ConfigurationManager() serverConfiguration.ServerConfigurationManager {
	return s.configurationManager
}
