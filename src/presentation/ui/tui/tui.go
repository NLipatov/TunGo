package tui

import (
	"fmt"

	appConfiguration "tungo/application/configuration"
	"tungo/infrastructure/PAL/service_management/linux/systemd"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

type TUI struct {
	configuratorOptions bubbleTea.ConfiguratorOptions
	preferences         *bubbleTea.Preferences
}

func New(
	configurationControls appConfiguration.Controls,
	daemonControl systemd.Control,
) (*TUI, error) {
	if configurationControls.Client == nil {
		return nil, fmt.Errorf("client configuration control is nil")
	}
	return &TUI{
		configuratorOptions: bubbleTea.ConfiguratorOptions{
			ClientConfigurationControl: configurationControls.Client,
			ServerConfigurationControl: configurationControls.Server,
			Daemon:                     daemonControl,
		},
		preferences: bubbleTea.LoadPreferences(),
	}, nil
}
