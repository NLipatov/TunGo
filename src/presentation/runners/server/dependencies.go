package server

import (
	"tungo/application"
	"tungo/infrastructure/PAL/server_configuration"
)

type AppDependencies interface {
	Configuration() server_configuration.Configuration
	TunManager() application.ServerTunManager
	KeyManager() server_configuration.KeyManager
	SessionLifetimeManager() server_configuration.SessionLifetimeManager
}

type Dependencies struct {
	configuration          server_configuration.Configuration
	tunManager             application.ServerTunManager
	keyManager             server_configuration.KeyManager
	sessionLifetimeManager server_configuration.SessionLifetimeManager
}

func NewDependencies(
	tunManager application.ServerTunManager,
	configuration server_configuration.Configuration,
	keyManager server_configuration.KeyManager,
	sessionLifetimeManager server_configuration.SessionLifetimeManager,
) AppDependencies {
	return &Dependencies{
		configuration:          configuration,
		tunManager:             tunManager,
		keyManager:             keyManager,
		sessionLifetimeManager: sessionLifetimeManager,
	}
}

func (s Dependencies) Configuration() server_configuration.Configuration {
	return s.configuration
}

func (s Dependencies) TunManager() application.ServerTunManager {
	return s.tunManager
}

func (s Dependencies) KeyManager() server_configuration.KeyManager {
	return s.keyManager
}

func (s Dependencies) SessionLifetimeManager() server_configuration.SessionLifetimeManager {
	return s.sessionLifetimeManager
}
