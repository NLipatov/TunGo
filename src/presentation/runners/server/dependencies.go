package server

import (
	"tungo/application"
	server_configuration2 "tungo/infrastructure/PAL/server_configuration"
)

type AppDependencies interface {
	Configuration() server_configuration2.Configuration
	TunManager() application.ServerTunManager
	KeyManager() server_configuration2.KeyManager
}

type Dependencies struct {
	configuration server_configuration2.Configuration
	tunManager    application.ServerTunManager
	keyManager    server_configuration2.KeyManager
}

func NewDependencies(
	tunManager application.ServerTunManager, configuration server_configuration2.Configuration, keyManager server_configuration2.KeyManager,
) AppDependencies {
	return &Dependencies{
		configuration: configuration,
		tunManager:    tunManager,
		keyManager:    keyManager,
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
