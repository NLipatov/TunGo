package presentation

import (
	"tungo/application"
	"tungo/settings/server_configuration"
)

type ServerAppDependencies interface {
	Configuration() server_configuration.Configuration
	TunManager() application.ServerTunManager
}

type ServerDependencies struct {
	configuration server_configuration.Configuration
	tunManager    application.ServerTunManager
}

func NewServerDependencies(
	tunManager application.ServerTunManager, configuration server_configuration.Configuration,
) ServerAppDependencies {
	return &ServerDependencies{
		configuration: configuration,
		tunManager:    tunManager,
	}
}

func (s ServerDependencies) Configuration() server_configuration.Configuration {
	return s.configuration
}

func (s ServerDependencies) TunManager() application.ServerTunManager {
	return s.tunManager
}
