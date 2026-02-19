package bubble_tea

import (
	"fmt"
	"os"
)

var (
	isInteractiveTerminal = IsInteractiveTerminal
	printToStdout         = func(s string) {
		_, _ = fmt.Fprint(os.Stdout, s)
	}
)

func clearTerminalAfterTUI() {
	if !isInteractiveTerminal() {
		return
	}
	// Clear full screen and move cursor home after leaving Bubble Tea.
	printToStdout("\x1b[2J\x1b[H")
}
