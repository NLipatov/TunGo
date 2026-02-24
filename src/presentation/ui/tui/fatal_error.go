package tui

import bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"

// ShowFatalError displays a themed fatal error screen in a standalone
// bubbletea program. It blocks until the user dismisses the screen
// (Enter / Esc / q). Use this when the UnifiedSession is not available.
func ShowFatalError(title, message string) {
	p := bubbleTea.NewFatalErrorProgram(title, message)
	_, _ = p.Run()
}
