package tui

import (
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	uifactory "tungo/presentation/ui/tui/internal/ui/factory"
)

type Configurator struct {
	clientConfigurator *clientConfigurator
	serverConfigurator *serverConfigurator
	serverSupported    bool
	sh                 *sessionHolder
	runtimeUI          *RuntimeUI
}

func NewConfigurator(
	serverConfigurationManager server.ConfigurationManager,
	serverSupported bool,
	runtimeUI *RuntimeUI,
) *Configurator {
	clientConfResolver := clientConfiguration.NewDefaultResolver()
	uiBundle := uifactory.NewDefaultBundle()
	if runtimeUI == nil {
		runtimeUI = NewRuntimeUI()
	}

	return &Configurator{
		clientConfigurator: newClientConfigurator(
			clientConfiguration.NewDefaultObserver(clientConfResolver),
			clientConfiguration.NewDefaultSelector(clientConfResolver),
			clientConfiguration.NewDefaultDeleter(clientConfResolver),
			clientConfiguration.NewDefaultCreator(clientConfResolver),
			uiBundle.SelectorFactory,
			uiBundle.TextInputFactory,
			uiBundle.TextAreaFactory,
			clientConfiguration.NewManager(),
		),
		serverConfigurator: newServerConfigurator(serverConfigurationManager, uiBundle.SelectorFactory),
		serverSupported:    serverSupported,
		runtimeUI:          runtimeUI,
	}
}
