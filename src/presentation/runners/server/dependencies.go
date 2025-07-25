package server

import (
	"tungo/application"
	server_configuration2 "tungo/infrastructure/PAL/configuration/server"
)

type AppDependencies interface {
	Configuration() server_configuration2.Configuration
	TunManager() application.ServerTunManager
	KeyManager() server_configuration2.KeyManager
	ConfigurationManager() server_configuration2.ServerConfigurationManager
}

type Dependencies struct {
	configuration        server_configuration2.Configuration
	tunManager           application.ServerTunManager
	keyManager           server_configuration2.KeyManager
	configurationManager server_configuration2.ServerConfigurationManager
}

func NewDependencies(
	tunManager application.ServerTunManager,
	configuration server_configuration2.Configuration,
	keyManager server_configuration2.KeyManager,
	configurationManager server_configuration2.ServerConfigurationManager,
) AppDependencies {
	return &Dependencies{
		configuration:        configuration,
		tunManager:           tunManager,
		keyManager:           keyManager,
		configurationManager: configurationManager,
	}
}

func (s Dependencies) Configuration() server_configuration2.Configuration {
	return s.configuration
}

func (s Dependencies) TunManager() application.ServerTunManager {
	return s.tunManager
}

func (s Dependencies) KeyManager() server_configuration2.KeyManager {
	return s.keyManager
}

func (s Dependencies) ConfigurationManager() server_configuration2.ServerConfigurationManager {
	return s.configurationManager
}
