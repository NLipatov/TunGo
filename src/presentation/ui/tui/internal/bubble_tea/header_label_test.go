package bubble_tea

import (
	"testing"
	"tungo/presentation/runners/version"
)

func TestProductLabel_DefaultWhenVersionUnset(t *testing.T) {
	prevTag := version.Tag
	t.Cleanup(func() { version.Tag = prevTag })

	version.Tag = "version not set"
	if got := productLabel(); got != "TunGo [dev-build]" {
		t.Fatalf("expected dev-build product label, got %q", got)
	}
}

func TestProductLabel_DefaultWhenVersionEmpty(t *testing.T) {
	prevTag := version.Tag
	t.Cleanup(func() { version.Tag = prevTag })

	version.Tag = "   "
	if got := productLabel(); got != "TunGo [dev-build]" {
		t.Fatalf("expected dev-build product label for empty tag, got %q", got)
	}
}

func TestProductLabel_IncludesVersionTag(t *testing.T) {
	prevTag := version.Tag
	t.Cleanup(func() { version.Tag = prevTag })

	version.Tag = "v0.2.99"
	if got := productLabel(); got != "TunGo [v0.2.99]" {
		t.Fatalf("expected versioned product label, got %q", got)
	}
}
