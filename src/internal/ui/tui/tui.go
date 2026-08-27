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

// New creates a TUI with the supplied client configurations, server file, and daemon control.
// It loads the user's UI preferences during construction.
// New returns an error if clientConfigurations is nil.
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
