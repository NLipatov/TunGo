package colorization

import "tungo/presentation/ui/tui/internal/ui/value_objects"

// Colorizer defines behavior for applying colors to text output in a TUI context.
// Implementations are responsible for rendering colored strings based on the
// provided foreground and background Color value objects.
type Colorizer interface {
	ColorizeString(
		s string,
		foreground, background value_objects.Color,
	) string
}
