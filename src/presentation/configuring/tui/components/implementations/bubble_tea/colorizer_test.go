package bubble_tea

import (
	"testing"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
)

func TestNewColorizer_ImplementsInterface(t *testing.T) {
	var cz = NewColorizer()
	if cz == nil {
		t.Fatal("expected non-nil Colorizer")
	}
}

func TestColorizeString_BothEnabled(t *testing.T) {
	cz := NewColorizer()

	text := "hello"
	bg := value_objects.NewColor(255, 0, 0, true) // red
	fg := value_objects.NewColor(0, 255, 0, true) // green

	got := cz.ColorizeString(text, bg, fg)

	want := "\033[38;2;0;255;0m" + // foreground first
		"\033[48;2;255;0;0m" + // background second
		"hello\033[0m"

	if got != want {
		t.Fatalf("unexpected output:\n got: %q\nwant: %q", got, want)
	}
}

func TestColorizeString_ForegroundOnly(t *testing.T) {
	cz := NewColorizer()

	text := "hi"
	bg := value_objects.NewTransparentColor()   // disabled bg
	fg := value_objects.NewColor(1, 2, 3, true) // enabled fg

	got := cz.ColorizeString(text, bg, fg)
	want := "\033[38;2;1;2;3m" + "hi" + "\033[0m"

	if got != want {
		t.Fatalf("unexpected output:\n got: %q\nwant: %q", got, want)
	}
}

func TestColorizeString_BackgroundOnly(t *testing.T) {
	cz := NewColorizer()

	text := "bg"
	bg := value_objects.NewColor(9, 8, 7, true) // enabled bg
	fg := value_objects.NewTransparentColor()   // disabled fg

	got := cz.ColorizeString(text, bg, fg)
	want := "\033[48;2;9;8;7m" + "bg" + "\033[0m"

	if got != want {
		t.Fatalf("unexpected output:\n got: %q\nwant: %q", got, want)
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
		t.Fatalf("unexpected output:\n got: %q\nwant: %q", got, want)
	}
}
