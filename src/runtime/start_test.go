package runtime

import (
	"context"
	"strings"
	"testing"
)

func TestStart_InvalidMode(t *testing.T) {
	_, err := Start(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid runtime mode") {
		t.Fatalf("expected invalid mode error, got %v", err)
	}
}
