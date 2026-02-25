package elevation

import (
	"os"
	"testing"
)

func TestIsElevated(t *testing.T) {
	want := os.Geteuid() == 0
	if got := IsElevated(); got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestHint(t *testing.T) {
	if h := Hint(); h == "" {
		t.Fatal("expected non-empty hint")
	}
}
