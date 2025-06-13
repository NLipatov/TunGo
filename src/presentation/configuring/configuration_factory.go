package configuring

import (
	"os"
	"tungo/infrastructure/PAL/client_configuration"
	"tungo/infrastructure/PAL/server_configuration"
	"tungo/presentation/configuring/cli"
	"tungo/presentation/configuring/tui"
	"tungo/presentation/configuring/tui/components/implementations/bubble_tea"
)

type ConfigurationFactory struct{}

func NewConfigurationFactory() *ConfigurationFactory {
	return &ConfigurationFactory{}
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
	confResolver := client_configuration.NewDefaultResolver()
	tuiConfigurator := tui.NewConfigurator(
		client_configuration.NewDefaultObserver(confResolver),
		client_configuration.NewDefaultSelector(confResolver),
		client_configuration.NewDefaultCreator(confResolver),
		client_configuration.NewDefaultDeleter(confResolver),
		server_configuration.NewManager(server_configuration.NewServerResolver()),
		bubble_tea.NewSelectorAdapter(),
		bubble_tea.NewTextInputAdapter(),
		bubble_tea.NewTextAreaAdapter(),
	)

	return tuiConfigurator
}
