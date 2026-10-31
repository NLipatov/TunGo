package tui

import (
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

// New creates a TUI with the supplied server file, and daemon control.
// It loads the user's UI preferences during construction.
func New(
	serverFile *serverconfig.File,
	daemonControl systemd.Control,
) *TUI {
	return &TUI{
		clientConfigurations: clientconfig.Files(),
		serverFile:           serverFile,
		daemonControl:        daemonControl,
		preferences:          bubbleTea.LoadPreferences(),
	}
}
