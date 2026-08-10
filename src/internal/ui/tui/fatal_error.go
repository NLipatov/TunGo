package tui

import bubbleTea "tungo/internal/ui/tui/internal/bubble_tea"

// ShowFatalError displays a themed fatal error screen and blocks until the
// user dismisses it (Enter / Esc / q).
func ShowFatalError(message string) {
	_, _ = bubbleTea.NewFatalErrorProgram(message).Run()
}
