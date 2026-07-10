package readiness

import (
	"context"
	"errors"
	"testing"
)

func TestSignalWait_ReturnsAfterMark(t *testing.T) {
	signal := NewSignal()
	signal.Mark()
	signal.Mark()

	if err := signal.Wait(context.Background()); err != nil {
		t.Fatalf("expected ready signal, got %v", err)
	}
}

func TestSignalWait_ReturnsContextError(t *testing.T) {
	signal := NewSignal()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := signal.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestSignalWait_PrefersReadyOverCanceledContext(t *testing.T) {
	signal := NewSignal()
	signal.Mark()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := signal.Wait(ctx); err != nil {
		t.Fatalf("expected ready signal to win, got %v", err)
	}
}
