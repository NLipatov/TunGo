package configuring

import (
	"context"
	"time"
	"tungo/domain/app"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/telemetry/trafficstats"
	"tungo/presentation/ui/cli"
	"tungo/presentation/ui/tui"
)

type ConfigurationFactory struct {
	uiMode            app.UIMode
	serverConfManager server.ConfigurationManager
	serverSupported   bool
}

func NewConfigurationFactory(uiMode app.UIMode, manager server.ConfigurationManager, serverSupported bool) *ConfigurationFactory {
	return &ConfigurationFactory{uiMode: uiMode, serverConfManager: manager, serverSupported: serverSupported}
}

func (c *ConfigurationFactory) Configurator(ctx context.Context) (Configurator, func()) {
	if c.uiMode == app.CLI {
		return c.buildCLIConfigurator()
	}

	return c.buildTUIConfigurator(ctx)
}

func (c *ConfigurationFactory) buildCLIConfigurator() (Configurator, func()) {
	return cli.NewConfigurator(), func() {}
}

func (c *ConfigurationFactory) buildTUIConfigurator(ctx context.Context) (Configurator, func()) {
	trafficCollector := trafficstats.NewCollector(time.Second, 0.35)
	trafficstats.SetGlobal(trafficCollector)
	go trafficCollector.Start(ctx)

	tui.EnableRuntimeLogCapture(1200)

	cleanup := func() {
		tui.DisableRuntimeLogCapture()
		trafficstats.SetGlobal(nil)
	}

	return tui.NewDefaultConfigurator(c.serverConfManager, c.serverSupported), cleanup
}
