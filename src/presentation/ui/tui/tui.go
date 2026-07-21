package tui

import (
	"fmt"
	appConfiguration "tungo/application/configuration"
	appRuntime "tungo/application/runtime"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

type TUI struct {
	sessionOptions          bubbleTea.ConfiguratorSessionOptions
	sessionFactory          unifiedSessionFactory
	systemdInstallerFactory systemdInstallerFactory
	newRuntime              func(appRuntime.Mode) (appRuntime.Runtime, error)
	session                 unifiedSessionHandle
}

func New(configurationControls appConfiguration.Controls) (*TUI, error) {
	if configurationControls.Client == nil {
		return nil, fmt.Errorf("client configuration control is nil")
	}
	return newTUI(configurationControls), nil
}

func newTUI(controls appConfiguration.Controls) *TUI {
	return &TUI{
		sessionOptions: bubbleTea.ConfiguratorSessionOptions{
			ClientConfigurationControl: controls.Client,
			ServerConfigurationControl: controls.Server,
			ServerSupported:            controls.ServerSupported(),
		},
		sessionFactory:          newBubbleTeaUnifiedSession,
		systemdInstallerFactory: newDefaultSystemdInstaller,
		newRuntime:              appRuntime.New,
	}
}
