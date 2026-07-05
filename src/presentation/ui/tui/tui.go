package tui

import (
	"fmt"
	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/PAL/platform"
	"tungo/infrastructure/PAL/stat"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

type TUI struct {
	sessionOptions          bubbleTea.ConfiguratorSessionOptions
	sessionFactory          unifiedSessionFactory
	systemdInstallerFactory systemdInstallerFactory
	session                 unifiedSessionHandle
}

func New() (*TUI, error) {
	serverResolver := serverConfiguration.NewServerResolver()
	serverConfigurationManager, err := serverConfiguration.NewManager(serverResolver, stat.NewDefaultStat())
	if err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}
	return newTUI(
		serverConfigurationManager,
		platform.Capabilities().ServerModeSupported(),
	), nil
}

func newTUI(
	serverConfigurationManager serverConfiguration.ConfigurationManager,
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
