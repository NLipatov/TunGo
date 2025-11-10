package bubble_tea

import (
	"fmt"
	"tungo/presentation/configuring/tui/components"
)

type Colorizer struct {
}

func NewColorizer() components.Colorizer {
	return &Colorizer{}
}

func (c *Colorizer) ColorizeString(
	s string,
	background, foreground components.Color,
) string {
	out := ""

	if foreground.Enabled() {
		out += fmt.Sprintf("\033[38;2;%d;%d;%dm", foreground.Red(), foreground.Green(), foreground.Blue())
	}

	if background.Enabled() {
		out += fmt.Sprintf("\033[48;2;%d;%d;%dm", background.Red(), background.Green(), background.Blue())
	}

	out += s + "\033[0m"
	return out
}
