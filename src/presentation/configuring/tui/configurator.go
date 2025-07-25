package tui

import (
	"fmt"
	"tungo/domain/mode"
	client_configuration2 "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/presentation/configuring/tui/components"
)

type Configurator struct {
	appMode            AppMode
	clientConfigurator *clientConfigurator
	serverConfigurator *serverConfigurator
}

func NewConfigurator(
	observer client_configuration2.Observer,
	selector client_configuration2.Selector,
	creator client_configuration2.Creator,
	deleter client_configuration2.Deleter,
	serverConfigurationManager server.ServerConfigurationManager,
	selectorFactory components.SelectorFactory,
	textInputFactory components.TextInputFactory,
	textAreaFactory components.TextAreaFactory,
) *Configurator {
	return &Configurator{
		clientConfigurator: newClientConfigurator(observer, selector, deleter, creator, selectorFactory, textInputFactory, textAreaFactory),
		serverConfigurator: newServerConfigurator(serverConfigurationManager, selectorFactory),
		appMode:            NewAppMode(selectorFactory),
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
