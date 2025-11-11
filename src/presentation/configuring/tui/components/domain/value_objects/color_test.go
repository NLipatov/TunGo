package value_objects

import "testing"

func TestNewColor(t *testing.T) {
	c := NewColor(10, 20, 30, true)

	if c.Red() != 10 {
		t.Errorf("expected Red = 10, got %d", c.Red())
	}
	if c.Green() != 20 {
		t.Errorf("expected Green = 20, got %d", c.Green())
	}
	if c.Blue() != 30 {
		t.Errorf("expected Blue = 30, got %d", c.Blue())
	}
	if !c.Enabled() {
		t.Errorf("expected Enabled = true, got false")
	}
}

func TestNewDefaultColor(t *testing.T) {
	c := NewDefaultColor()

	if c.Red() != 0 || c.Green() != 255 || c.Blue() != 0 {
		t.Errorf("unexpected default color RGB: (%d,%d,%d)", c.Red(), c.Green(), c.Blue())
	}
	if !c.Enabled() {
		t.Errorf("expected default color to be enabled")
	}
}

func TestNewTransparentColor(t *testing.T) {
	c := NewTransparentColor()

	if c.Red() != 0 || c.Green() != 0 || c.Blue() != 0 {
		t.Errorf("unexpected transparent color RGB: (%d,%d,%d)", c.Red(), c.Green(), c.Blue())
	}
	if c.Enabled() {
		t.Errorf("expected transparent color to be disabled")
	}
}

func TestColor_Immutability(t *testing.T) {
	c1 := NewColor(1, 2, 3, true)
	c2 := c1 // copy

	// Ensure values are equal but stored separately (immutability by copy)
	if c2.Red() != 1 || c2.Green() != 2 || c2.Blue() != 3 || !c2.Enabled() {
		t.Errorf("expected identical copy, got different values")
	}

	// Changing the copy (by creating a new one) doesn't affect original
	c2 = NewColor(255, 255, 255, false)
	if c1.Red() == c2.Red() && c1.Green() == c2.Green() && c1.Blue() == c2.Blue() && c1.Enabled() == c2.Enabled() {
		t.Errorf("expected c1 and c2 to be different after creating new color")
	}
}
