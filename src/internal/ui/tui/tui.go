package tui

import (
	"fmt"

	clientconfig "tungo/internal/config/client"
	serverconfig "tungo/internal/config/server"
	"tungo/internal/daemon/systemd"
	bubbleTea "tungo/internal/ui/tui/internal/bubble_tea"
)

type TUI struct {
	clientConfigurations *clientconfig.Configurations
	serverFile           *serverconfig.File
	daemonControl        systemd.Control
	preferences          *bubbleTea.Preferences
}

func New(
	clientConfigurations *clientconfig.Configurations,
	serverFile *serverconfig.File,
	daemonControl systemd.Control,
) (*TUI, error) {
	if clientConfigurations == nil {
		return nil, fmt.Errorf("client configuration files are nil")
	}
	return &TUI{
		clientConfigurations: clientConfigurations,
		serverFile:           serverFile,
		daemonControl:        daemonControl,
		preferences:          bubbleTea.LoadPreferences(),
	}, nil
}
