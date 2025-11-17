package value_objects

import "testing"

func TestNewColor(t *testing.T) {
	c := NewColor(ColorBlue, true)

	if c.Code() != ColorBlue {
		t.Errorf("expected Code = ColorBlue, got %d", c.Code())
	}

	if !c.Enabled() {
		t.Errorf("expected Enabled = true, got false")
	}
}

func TestNewDefaultColor(t *testing.T) {
	c := NewDefaultColor()

	if c.Code() != ColorGreen {
		t.Errorf("expected default color code = ColorGreen, got %d", c.Code())
	}

	if !c.Enabled() {
		t.Errorf("expected default color to be enabled")
	}
}

func TestNewTransparentColor(t *testing.T) {
	c := NewTransparentColor()
	if c.Code() != ColorGreen {
		t.Errorf("expected transparent color code = ColorGreen, got %d", c.Code())
	}
	if c.Enabled() {
		t.Errorf("expected transparent color to be disabled")
	}
}

func TestColor_Immutability(t *testing.T) {
	c1 := NewColor(ColorRed, true)
	c2 := c1
	if c2.Code() != c1.Code() || c2.Enabled() != c1.Enabled() {
		t.Errorf("expected c2 to be an identical copy of c1")
	}
	c2 = NewColor(ColorBlue, false)
	if c1.Code() == c2.Code() && c1.Enabled() == c2.Enabled() {
		t.Errorf("expected c1 and c2 to be different after creating new value")
	}
}
