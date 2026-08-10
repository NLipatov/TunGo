package version

import "testing"

func TestCurrent(t *testing.T) {
	prevTag := Tag
	t.Cleanup(func() { Tag = prevTag })

	Tag = " v0.3.0 "
	if got := Current(); got != "v0.3.0" {
		t.Fatalf("expected trimmed tag, got %q", got)
	}

	Tag = ""
	if got := Current(); got != "" {
		t.Fatalf("expected empty value for empty tag, got %q", got)
	}
}
