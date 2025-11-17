package bubble_tea

import (
	"fmt"
	"tungo/presentation/configuring/tui/components/domain/contracts/colorization"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
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
			out += fmt.Sprintf("\033[%dm", 30+code)
		} else {
			out += fmt.Sprintf("\033[%dm", 90+(code-8))
		}
	}

	if background.Enabled() {
		code := background.Code()
		if code <= 7 {
			out += fmt.Sprintf("\033[%dm", 40+code)
		} else {
			out += fmt.Sprintf("\033[%dm", 100+(code-8))
		}
	}

	out += s + "\033[0m"
	return out
}
