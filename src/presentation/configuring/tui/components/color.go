package components

type Color struct {
	r, g, b uint8
	enabled bool
}

func NewDefaultColor() Color {
	return NewColor(0, 255, 0, true)
}

func NewTransparentColor() Color {
	return NewColor(0, 0, 0, false)
}

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
