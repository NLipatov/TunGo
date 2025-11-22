package tui

import (
	"fmt"
	"tungo/domain/mode"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_area"
	"tungo/presentation/configuring/tui/components/domain/contracts/text_input"
)

type Configurator struct {
	appMode            AppMode
	clientConfigurator *clientConfigurator
	serverConfigurator *serverConfigurator
}

func NewConfigurator(
	observer clientConfiguration.Observer,
	selector clientConfiguration.Selector,
	creator clientConfiguration.Creator,
	deleter clientConfiguration.Deleter,
	serverConfigurationManager server.ConfigurationManager,
	selectorFactory selector.Factory,
	textInputFactory text_input.TextInputFactory,
	textAreaFactory text_area.TextAreaFactory,
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
