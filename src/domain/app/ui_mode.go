package app

import "os"

// UIMode describes how the application interacts with the user.
type UIMode int

const (
	UnknownUIMode UIMode = iota
	TUI
	CLI
)

// CurrentUIMode returns the active UI mode based on command-line arguments.
func CurrentUIMode() UIMode {
	if len(os.Args) < 2 {
		return TUI
	}
	return CLI
}
