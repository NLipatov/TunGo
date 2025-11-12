package value_objects

// Color is a DDD Value Object that represents an immutable RGB color.
// Two Color instances are considered equal if all their components match.
// The Enabled flag indicates whether the color is active or visually applied.
type Color struct {
	r, g, b uint8 // RGB color components
	enabled bool  // indicates whether the color is enabled
}

// NewDefaultColor returns a default TunGo-styled green color.
func NewDefaultColor() Color {
	return NewColor(0, 200, 0, true)
}

// NewTransparentColor returns a disabled color instance.
func NewTransparentColor() Color {
	return NewColor(0, 0, 0, false)
}

// NewColor returns a new Color with the given RGB components and enabled flag.
func NewColor(r, g, b uint8, enabled bool) Color {
	return Color{
		enabled: enabled,
		r:       r,
		g:       g,
		b:       b,
	}
}

func (c Color) Red() uint8 {
	return c.r
}

func (c Color) Green() uint8 {
	return c.g
}

func (c Color) Blue() uint8 {
	return c.b
}

func (c Color) Enabled() bool {
	return c.enabled
}
