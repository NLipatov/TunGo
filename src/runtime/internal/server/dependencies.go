package server

import (
	"tungo/application/network/routing/tun"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
)

type AppDependencies interface {
	Configuration() serverConfiguration.Configuration
	TunManager() tun.ServerManager
	KeyManager() serverConfiguration.KeyManager
	ConfigurationManager() serverConfiguration.ConfigurationManager
}

type Dependencies struct {
	configuration        serverConfiguration.Configuration
	tunManager           tun.ServerManager
	keyManager           serverConfiguration.KeyManager
	configurationManager serverConfiguration.ConfigurationManager
}

func NewDependencies(
	tunManager tun.ServerManager,
	configuration serverConfiguration.Configuration,
	keyManager serverConfiguration.KeyManager,
	configurationManager serverConfiguration.ConfigurationManager,
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

func (s Dependencies) TunManager() tun.ServerManager {
	return s.tunManager
}

func (s Dependencies) KeyManager() serverConfiguration.KeyManager {
	return s.keyManager
}

func (s Dependencies) ConfigurationManager() serverConfiguration.ConfigurationManager {
	return s.configurationManager
}
