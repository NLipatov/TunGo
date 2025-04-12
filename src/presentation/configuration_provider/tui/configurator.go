package tui

import (
	"fmt"
	"tungo/domain/mode"
	"tungo/settings/client_configuration"
)

const (
	AddOption    string = "+ add configuration"
	RemoveOption string = "- remove configuration"
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
) *Configurator {
	return &Configurator{
		clientConfigurator: newClientConfigurator(observer, selector, deleter, creator),
		serverConfigurator: newServerConfigurator(),
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
