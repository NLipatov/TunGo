package value_objects

// ColorCode represents an ANSI base or bright color code (0–15).
// These colors are universally supported across all terminals,
// including macOS Terminal.app, Linux TTY, tmux, SSH, Windows Terminal.
type ColorCode uint8

const (
	ColorBlack ColorCode = iota
	ColorRed
	ColorGreen
	ColorYellow
	ColorBlue
	ColorMagenta
	ColorCyan
	ColorWhite

	ColorBrightBlack
	ColorBrightRed
	ColorBrightGreen
	ColorBrightYellow
	ColorBrightBlue
	ColorBrightMagenta
	ColorBrightCyan
	ColorBrightWhite
)
