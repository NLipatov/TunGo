package app

import (
	"os"
	"testing"
)

func TestName(t *testing.T) {
	if Name == "" {
		t.Fatal("expected non-empty app name")
	}
	if Name != "tungo" {
		t.Fatalf("expected app name %q, got %q", "tungo", Name)
	}
}

func TestCurrentUIMode(t *testing.T) {
	originalArgs := os.Args
	t.Cleanup(func() { os.Args = originalArgs })

	os.Args = []string{"tungo"}
	if got := CurrentUIMode(); got != TUI {
		t.Fatalf("CurrentUIMode() without arguments = %v, want %v", got, TUI)
	}

	os.Args = []string{"tungo", "status"}
	if got := CurrentUIMode(); got != CLI {
		t.Fatalf("CurrentUIMode() with arguments = %v, want %v", got, CLI)
	}
}
