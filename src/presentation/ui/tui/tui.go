package tui

import (
	"context"
	"fmt"
	appConfiguration "tungo/application/configuration"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
	appRuntime "tungo/runtime"
)

type TUI struct {
	sessionOptions          bubbleTea.ConfiguratorSessionOptions
	sessionFactory          unifiedSessionFactory
	systemdInstallerFactory systemdInstallerFactory
	startRuntime            func(context.Context, appRuntime.Mode) (appRuntime.Session, error)
	session                 unifiedSessionHandle
}

func New(configurationControls appConfiguration.Controls) (*TUI, error) {
	if configurationControls.Client == nil {
		return nil, fmt.Errorf("client configuration control is nil")
	}
	return newTUI(configurationControls), nil
}

func newTUI(configurationControls appConfiguration.Controls) *TUI {
	return &TUI{
		sessionOptions: bubbleTea.ConfiguratorSessionOptions{
			ClientConfigurationControl: configurationControls.Client,
			ServerConfigurationControl: configurationControls.Server,
			ServerSupported:            configurationControls.ServerSupported(),
		},
		sessionFactory:          newBubbleTeaUnifiedSession,
		systemdInstallerFactory: newDefaultSystemdInstaller,
		startRuntime:            appRuntime.Start,
	}
}
