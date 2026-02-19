package bubble_tea

import (
	"testing"
	"tungo/presentation/ui/tui/internal/ui/value_objects"
)

func TestNewColorizer_ImplementsInterface(t *testing.T) {
	var cz = NewColorizer()
	if cz == nil {
		t.Fatal("expected non-nil Colorizer")
	}
}

func TestColorizeString_ForegroundNormal(t *testing.T) {
	cz := NewColorizer()

	text := "test"
	// Foreground normal range: 0..7 → 30..37
	fg := value_objects.NewColor(value_objects.ColorBlue, true) // 4 → 34
	bg := value_objects.NewTransparentColor()                   // disabled

	got := cz.ColorizeString(text, bg, fg)
	want := "\033[34m" + "test" + "\033[0m"

	if got != want {
		t.Fatalf("unexpected output:\n got: %q\nwant: %q", got, want)
	}
}

func TestColorizeString_ForegroundBright(t *testing.T) {
	cz := NewColorizer()

	text := "test"
	// Bright FG: 8..15 → 90..97
	fg := value_objects.NewColor(value_objects.ColorBrightYellow, true) // 11 → 90 + (11-8)=93
	bg := value_objects.NewTransparentColor()

	got := cz.ColorizeString(text, bg, fg)
	want := "\033[93m" + "test" + "\033[0m"

	if got != want {
		t.Fatalf("unexpected bright FG output:\n got: %q\nwant: %q", got, want)
	}
}

func TestColorizeString_BackgroundNormal(t *testing.T) {
	cz := NewColorizer()

	text := "bg"
	// BG normal: 0..7 → 40..47
	bg := value_objects.NewColor(value_objects.ColorMagenta, true) // 5 → 45
	fg := value_objects.NewTransparentColor()

	got := cz.ColorizeString(text, bg, fg)
	want := "\033[45m" + "bg" + "\033[0m"

	if got != want {
		t.Fatalf("unexpected normal BG output:\n got: %q\nwant: %q", got, want)
	}
}

func TestColorizeString_BackgroundBright(t *testing.T) {
	cz := NewColorizer()

	text := "bg"
	// Bright BG: 8..15 → 100..107
	bg := value_objects.NewColor(value_objects.ColorBrightCyan, true) // 14 → 100 + (14-8)=106
	fg := value_objects.NewTransparentColor()

	got := cz.ColorizeString(text, bg, fg)
	want := "\033[106m" + "bg" + "\033[0m"

	if got != want {
		t.Fatalf("unexpected bright BG output:\n got: %q\nwant: %q", got, want)
	}
}

func TestColorizeString_BothEnabled(t *testing.T) {
	cz := NewColorizer()

	text := "hello"
	// fg normal: 2 → 32
	fg := value_objects.NewColor(value_objects.ColorGreen, true)
	// bg bright: 8 → 100
	bg := value_objects.NewColor(value_objects.ColorBrightBlack, true)

	got := cz.ColorizeString(text, bg, fg)
	want := "\033[32m" + "\033[100m" + "hello" + "\033[0m"

	if got != want {
		t.Fatalf("unexpected combined output:\n got: %q\nwant: %q", got, want)
	}
}

func TestColorizeString_NoneEnabled(t *testing.T) {
	cz := NewColorizer()

	text := "plain"
	bg := value_objects.NewTransparentColor()
	fg := value_objects.NewTransparentColor()

	got := cz.ColorizeString(text, bg, fg)
	want := "plain\033[0m"

	if got != want {
		t.Fatalf("unexpected output with no colors:\n got: %q\nwant: %q", got, want)
	}
}
