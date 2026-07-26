package tui

import (
	"fmt"

	appConfiguration "tungo/application/configuration"
	"tungo/infrastructure/PAL/service_management/linux/systemd"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

type TUI struct {
	sessionOptions bubbleTea.ConfiguratorSessionOptions
	session        *bubbleTea.UnifiedSession
}

func New(
	configurationControls appConfiguration.Controls,
	daemonControl systemd.Control,
) (*TUI, error) {
	if configurationControls.Client == nil {
		return nil, fmt.Errorf("client configuration control is nil")
	}
	return &TUI{
		sessionOptions: bubbleTea.ConfiguratorSessionOptions{
			ClientConfigurationControl: configurationControls.Client,
			ServerConfigurationControl: configurationControls.Server,
			Daemon:                     daemonControl,
		},
	}, nil
}
