package app

import "testing"

func TestName(t *testing.T) {
	if Name == "" {
		t.Fatal("expected non-empty app name")
	}
	if Name != "tungo" {
		t.Fatalf("expected app name %q, got %q", "tungo", Name)
	}
}
