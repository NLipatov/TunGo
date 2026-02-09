package settings

import (
	"testing"
	"time"
)

func TestDialTimeoutMs_Int(t *testing.T) {
	d := DialTimeoutMs(5000)
	if d.Int() != 5000 {
		t.Fatalf("expected 5000, got %d", d.Int())
	}
}

func TestDialTimeoutMs_Int_Zero(t *testing.T) {
	d := DialTimeoutMs(0)
	if d.Int() != 0 {
		t.Fatalf("expected 0, got %d", d.Int())
	}
}

func TestDialTimeoutMs_Duration(t *testing.T) {
	d := DialTimeoutMs(3000)
	if d.Duration() != 3*time.Second {
		t.Fatalf("expected 3s, got %v", d.Duration())
	}
}

func TestDialTimeoutMs_Duration_Zero(t *testing.T) {
	d := DialTimeoutMs(0)
	if d.Duration() != 0 {
		t.Fatalf("expected 0, got %v", d.Duration())
	}
}
