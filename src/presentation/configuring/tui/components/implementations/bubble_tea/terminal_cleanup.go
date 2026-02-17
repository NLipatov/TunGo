package bubble_tea

import (
	"fmt"
	"os"
)

func clearTerminalAfterTUI() {
	if !IsInteractiveTerminal() {
		return
	}
	// Clear full screen and move cursor home after leaving Bubble Tea.
	_, _ = fmt.Fprint(os.Stdout, "\x1b[2J\x1b[H")
}
