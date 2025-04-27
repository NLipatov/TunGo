package configuring

import (
	"os"
	"tungo/presentation/configuring/cli"
	"tungo/presentation/configuring/tui"
	"tungo/settings/client_configuration"
	"tungo/settings/server_configuration"
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
		server_configuration.NewManager())

	return tuiConfigurator
}
