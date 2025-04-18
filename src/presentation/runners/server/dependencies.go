package server

import (
	"tungo/application"
	"tungo/settings/server_configuration"
)

type AppDependencies interface {
	Configuration() server_configuration.Configuration
	TunManager() application.ServerTunManager
}

type Dependencies struct {
	configuration server_configuration.Configuration
	tunManager    application.ServerTunManager
}

func NewDependencies(
	tunManager application.ServerTunManager, configuration server_configuration.Configuration,
) AppDependencies {
	return &Dependencies{
		configuration: configuration,
		tunManager:    tunManager,
	}
}

func (s Dependencies) Configuration() server_configuration.Configuration {
	return s.configuration
}

func (s Dependencies) TunManager() application.ServerTunManager {
	return s.tunManager
}
