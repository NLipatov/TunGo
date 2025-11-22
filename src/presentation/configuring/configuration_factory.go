package configuring

import (
	"os"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/presentation/configuring/cli"
	"tungo/presentation/configuring/tui"
	"tungo/presentation/configuring/tui/components/implementations/bubble_tea"
)

type ConfigurationFactory struct {
	serverConfManager server.ConfigurationManager
}

func NewConfigurationFactory(manager server.ConfigurationManager) *ConfigurationFactory {
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
	clientConfResolver := clientConfiguration.NewDefaultResolver()
	serverConfManager := c.serverConfManager

	tuiConfigurator := tui.NewConfigurator(
		clientConfiguration.NewDefaultObserver(clientConfResolver),
		clientConfiguration.NewDefaultSelector(clientConfResolver),
		clientConfiguration.NewDefaultCreator(clientConfResolver),
		clientConfiguration.NewDefaultDeleter(clientConfResolver),
		serverConfManager,
		bubble_tea.NewSelectorAdapter(),
		bubble_tea.NewTextInputAdapter(),
		bubble_tea.NewTextAreaAdapter(),
	)

	return tuiConfigurator
}
