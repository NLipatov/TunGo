package bubble_tea

import (
	"fmt"
	"tungo/presentation/ui/tui/internal/ui/contracts/colorization"
	"tungo/presentation/ui/tui/internal/ui/value_objects"
)

type Colorizer struct {
}

func NewColorizer() colorization.Colorizer {
	return &Colorizer{}
}

func (c *Colorizer) ColorizeString(
	s string,
	background, foreground value_objects.Color,
) string {
	out := ""

	if foreground.Enabled() {
		code := foreground.Code()
		if code <= 7 {
			out += fmt.Sprintf("\x1b[%dm", 30+code)
		} else {
			out += fmt.Sprintf("\x1b[%dm", 90+(code-8))
		}
	}

	if background.Enabled() {
		code := background.Code()
		if code <= 7 {
			out += fmt.Sprintf("\x1b[%dm", 40+code)
		} else {
			out += fmt.Sprintf("\x1b[%dm", 100+(code-8))
		}
	}

	out += s + "\x1b[0m"
	return out
}
