package runtime

import (
	"strings"
	"testing"
)

func TestNew_InvalidMode(t *testing.T) {
	_, err := New(0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid runtime mode") {
		t.Fatalf("expected invalid mode error, got %v", err)
	}
}
