package value_objects

// Color is a DDD Value Object that represents an immutable RGB color.
// Two Color instances are considered equal if all their components match.
// The Enabled flag indicates whether the color is active or visually applied.
type Color struct {
	code    ColorCode
	enabled bool // indicates whether the color is enabled
}

// NewDefaultColor returns a default TunGo-styled green color.
func NewDefaultColor() Color {
	return NewColor(ColorGreen, true)
}

// NewTransparentColor returns a disabled color instance.
func NewTransparentColor() Color {
	return NewColor(ColorGreen, false)
}

// NewColor creates a new color with the given ANSI code.
func NewColor(code ColorCode, enabled bool) Color {
	return Color{
		code:    code,
		enabled: enabled,
	}
}

func (c Color) Code() ColorCode {
	return c.code
}

func (c Color) Enabled() bool {
	return c.enabled
}
