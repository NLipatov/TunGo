package bubble_tea

import "testing"

func Test_computeCardWidth_ClampedToTerminal(t *testing.T) {
	if got := computeCardWidth(40); got > 40 {
		t.Fatalf("card width must not exceed terminal width, got=%d", got)
	}
	if got := computeCardWidth(200); got != maxCardWidth {
		t.Fatalf("expected max width %d, got %d", maxCardWidth, got)
	}
}

func Test_computeCardHeight_ClampedToTerminal(t *testing.T) {
	if got := computeCardHeight(12); got > 12 {
		t.Fatalf("card height must not exceed terminal height, got=%d", got)
	}
	if got := computeCardHeight(200); got != maxCardHeight {
		t.Fatalf("expected max height %d, got %d", maxCardHeight, got)
	}
}

func Test_wrapLine_WrapsLongSentence(t *testing.T) {
	lines := wrapLine("one two three four five", 8)
	if len(lines) < 3 {
		t.Fatalf("expected wrapped output, got %v", lines)
	}
	for _, line := range lines {
		if len([]rune(line)) > 8 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func Test_wrapLine_BreaksLongWord(t *testing.T) {
	lines := wrapLine("supercalifragilistic", 6)
	if len(lines) < 2 {
		t.Fatalf("expected hard-wrap chunks, got %v", lines)
	}
	for _, line := range lines {
		if len([]rune(line)) > 6 {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func Test_wrapText_ANSIIsNotWrapped(t *testing.T) {
	raw := "\x1b[31mthis should stay as is even if very long\x1b[0m"
	lines := wrapText(raw, 5)
	if len(lines) != 1 {
		t.Fatalf("expected ANSI line to stay intact, got %v", lines)
	}
	if lines[0] != raw {
		t.Fatalf("expected unchanged ANSI line")
	}
}
