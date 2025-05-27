package configuring

import (
	"os"
	client_configuration2 "tungo/infrastructure/PAL/client_configuration"
	server_configuration2 "tungo/infrastructure/PAL/server_configuration"
	"tungo/presentation/configuring/cli"
	"tungo/presentation/configuring/tui"
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
	confResolver := client_configuration2.NewDefaultResolver()
	tuiConfigurator := tui.NewConfigurator(
		client_configuration2.NewDefaultObserver(confResolver),
		client_configuration2.NewDefaultSelector(confResolver),
		client_configuration2.NewDefaultCreator(confResolver),
		client_configuration2.NewDefaultDeleter(confResolver),
		server_configuration2.NewManager(server_configuration2.NewServerResolver()))

	return tuiConfigurator
}
