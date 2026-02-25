package tui

import bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"

// ShowFatalError displays a themed fatal error screen and blocks until the
// user dismisses it (Enter / Esc / q). If a UnifiedSession is active, the
// error is shown inside it; otherwise a standalone tea.Program is created.
func ShowFatalError(message string) {
	if activeUnifiedSession != nil {
		activeUnifiedSession.ShowFatalError(message)
		activeUnifiedSession.Close()
		activeUnifiedSession = nil
		return
	}
	p := bubbleTea.NewFatalErrorProgram(message)
	_, _ = p.Run()
}
