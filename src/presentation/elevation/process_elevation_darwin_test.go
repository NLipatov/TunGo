package elevation

import (
	"os"
	"testing"
)

func TestNewProcessElevation(t *testing.T) {
	p := NewProcessElevation()
	if p == nil {
		t.Fatal("expected non-nil process elevation")
	}
	if _, ok := p.(*ProcessElevationImpl); !ok {
		t.Fatalf("expected *ProcessElevationImpl, got %T", p)
	}
}

func TestProcessElevationImpl_IsElevated(t *testing.T) {
	p := &ProcessElevationImpl{}
	want := os.Geteuid() == 0
	if got := p.IsElevated(); got != want {
		t.Fatalf("unexpected IsElevated result: got %v, want %v", got, want)
	}
}
