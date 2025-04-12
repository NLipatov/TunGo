package configuration_provider

import (
	"os"
	"tungo/presentation/configuration_provider/cli"
	"tungo/presentation/configuration_provider/tui"
	"tungo/settings/client_configuration"
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
		client_configuration.NewDefaultDeleter(confResolver))

	return tuiConfigurator
}
