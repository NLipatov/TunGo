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
	clientConfResolver := client_configuration.NewDefaultResolver()
	serverConfManager, serverConfManagerErr := server_configuration.NewManager(server_configuration.NewServerResolver())
	if serverConfManagerErr != nil {
		panic(serverConfManagerErr)
	}

	tuiConfigurator := tui.NewConfigurator(
		client_configuration.NewDefaultObserver(clientConfResolver),
		client_configuration.NewDefaultSelector(clientConfResolver),
		client_configuration.NewDefaultCreator(clientConfResolver),
		client_configuration.NewDefaultDeleter(clientConfResolver),
		serverConfManager,
		bubble_tea.NewSelectorAdapter(),
		bubble_tea.NewTextInputAdapter(),
		bubble_tea.NewTextAreaAdapter(),
	)

	return tuiConfigurator
}
