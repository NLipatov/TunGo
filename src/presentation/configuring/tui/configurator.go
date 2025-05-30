package tui

import (
	"fmt"
	"tungo/domain/mode"
	"tungo/infrastructure/PAL/client_configuration"
	"tungo/infrastructure/PAL/server_configuration"
)

type Configurator struct {
	appMode            AppMode
	clientConfigurator *clientConfigurator
	serverConfigurator *serverConfigurator
}

func NewConfigurator(
	observer client_configuration.Observer,
	selector client_configuration.Selector,
	creator client_configuration.Creator,
	deleter client_configuration.Deleter,
	serverConfigurationManager server_configuration.ServerConfigurationManager,
) *Configurator {
	return &Configurator{
		clientConfigurator: newClientConfigurator(observer, selector, deleter, creator),
		serverConfigurator: newServerConfigurator(serverConfigurationManager),
		appMode:            NewAppMode(),
	}
}

func (p *Configurator) Configure() (mode.Mode, error) {
	appMode, appModeErr := p.appMode.Mode()
	if appModeErr != nil {
		return mode.Unknown, appModeErr
	}

	switch appMode {
	case mode.Server:
		return appMode, p.serverConfigurator.Configure()
	case mode.Client:
		return appMode, p.clientConfigurator.Configure()
	default:
		return mode.Unknown, fmt.Errorf("invalid mode")
	}
}
