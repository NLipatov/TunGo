package tui

import bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"

// ShowFatalError displays a themed fatal error screen and blocks until the
// user dismisses it (Enter / Esc / q). Creates a standalone tea.Program.
//
// If a unified session was active, the caller should close it first via
// Configurator.Close() so the alternate screen is released before this
// standalone program takes over.
func ShowFatalError(message string) {
	p := bubbleTea.NewFatalErrorProgram(message)
	_, _ = p.Run()
}
