//go:build windows

package resume

import (
	"context"
	"testing"
)

func TestWatchIsNoOp(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	resumed := Watch(ctx)
	cancel()

	select {
	case <-resumed:
		t.Fatal("Watch signaled resume")
	default:
	}
}
