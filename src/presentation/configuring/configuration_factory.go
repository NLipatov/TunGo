package configuring

import (
	"os"
	client_configuration2 "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/presentation/configuring/cli"
	"tungo/presentation/configuring/tui"
	"tungo/presentation/configuring/tui/components/implementations/bubble_tea"
)

type ConfigurationFactory struct {
	serverConfManager server.ServerConfigurationManager
}

func NewConfigurationFactory(manager server.ServerConfigurationManager) *ConfigurationFactory {
	return &ConfigurationFactory{serverConfManager: manager}
}

func (c *ConfigurationFactory) Configurator() Configurator {
	if len(os.Args) >= 2 {
		return c.buildCLIConfigurator()
	}

	return c.buildTUIConfigurator()
}

func (c *ConfigurationFactory) buildCLIConfigurator() Configurator {
	return cli.NewConfigurator()
}

func (c *ConfigurationFactory) buildTUIConfigurator() Configurator {
	clientConfResolver := client_configuration2.NewDefaultResolver()
	serverConfManager := c.serverConfManager

	tuiConfigurator := tui.NewConfigurator(
		client_configuration2.NewDefaultObserver(clientConfResolver),
		client_configuration2.NewDefaultSelector(clientConfResolver),
		client_configuration2.NewDefaultCreator(clientConfResolver),
		client_configuration2.NewDefaultDeleter(clientConfResolver),
		serverConfManager,
		bubble_tea.NewSelectorAdapter(),
		bubble_tea.NewTextInputAdapter(),
		bubble_tea.NewTextAreaAdapter(),
	)

	return tuiConfigurator
}
