package tui

import (
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/configuration/server"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

type TUI struct {
	sessionOptions          bubbleTea.ConfiguratorSessionOptions
	sessionFactory          unifiedSessionFactory
	systemdInstallerFactory systemdInstallerFactory
	session                 unifiedSessionHandle
}

func NewTUI(
	serverConfigurationManager server.ConfigurationManager,
	serverSupported bool,
) *TUI {
	clientConfResolver := clientConfiguration.NewDefaultResolver()

	return &TUI{
		sessionOptions: bubbleTea.ConfiguratorSessionOptions{
			Observer:            clientConfiguration.NewDefaultObserver(clientConfResolver),
			Selector:            clientConfiguration.NewDefaultSelector(clientConfResolver),
			Creator:             clientConfiguration.NewDefaultCreator(clientConfResolver),
			Deleter:             clientConfiguration.NewDefaultDeleter(clientConfResolver),
			ClientConfigManager: clientConfiguration.NewManager(),
			ServerConfigManager: serverConfigurationManager,
			ServerSupported:     serverSupported,
		},
		sessionFactory:          newBubbleTeaUnifiedSession,
		systemdInstallerFactory: newDefaultSystemdInstaller,
	}
}
