package configuring

import (
	"os"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/presentation/configuring/cli"
	"tungo/presentation/configuring/tui"
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
	return tui.NewDefaultConfigurator(c.serverConfManager)
}
