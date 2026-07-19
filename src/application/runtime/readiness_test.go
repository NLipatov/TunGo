package runtime

import (
	"context"
	"errors"
	"testing"
)

func TestSignalWait_ReturnsAfterMark(t *testing.T) {
	signal := newReadySignal()
	signal.mark()
	signal.mark()

	if err := signal.wait(context.Background()); err != nil {
		t.Fatalf("expected ready signal, got %v", err)
	}
}

func TestSignalWait_ReturnsContextError(t *testing.T) {
	signal := newReadySignal()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := signal.wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestSignalWait_PrefersReadyOverCanceledContext(t *testing.T) {
	signal := newReadySignal()
	signal.mark()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := signal.wait(ctx); err != nil {
		t.Fatalf("expected ready signal to win, got %v", err)
	}
}
