package tui

import (
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

type Configurator struct {
	sessionOptions bubbleTea.ConfiguratorSessionOptions
	sh             *sessionHolder
	runtimeUI      *RuntimeUI
}

func NewConfigurator(
	serverConfigurationManager server.ConfigurationManager,
	serverSupported bool,
	runtimeUI *RuntimeUI,
) *Configurator {
	clientConfResolver := clientConfiguration.NewDefaultResolver()
	if runtimeUI == nil {
		runtimeUI = NewRuntimeUI()
	}

	return &Configurator{
		sessionOptions: bubbleTea.ConfiguratorSessionOptions{
			Observer:            clientConfiguration.NewDefaultObserver(clientConfResolver),
			Selector:            clientConfiguration.NewDefaultSelector(clientConfResolver),
			Creator:             clientConfiguration.NewDefaultCreator(clientConfResolver),
			Deleter:             clientConfiguration.NewDefaultDeleter(clientConfResolver),
			ClientConfigManager: clientConfiguration.NewManager(),
			ServerConfigManager: serverConfigurationManager,
			ServerSupported:     serverSupported,
		},
		runtimeUI: runtimeUI,
	}
}
