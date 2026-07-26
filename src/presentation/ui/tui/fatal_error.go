package tui

import bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"

// ShowFatalError displays a themed fatal error screen and blocks until the
// user dismisses it (Enter / Esc / q). Creates a standalone tea.Program.
//
// If a unified session was active, the caller should close it first via
// TUI.Close() so the alternate screen is released before this standalone
// program takes over.
func ShowFatalError(message string) {
	program := bubbleTea.NewFatalErrorProgram(message)
	_, _ = program.Run()
}
