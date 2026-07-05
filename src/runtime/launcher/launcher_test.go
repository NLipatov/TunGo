package launcher

import (
	"context"
	"errors"
	"strings"
	"testing"
	appRuntime "tungo/runtime"
)

var _ appRuntime.Lifecycle = Launcher{}

func TestRun_InvalidMode(t *testing.T) {
	err := Run(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid runtime mode") {
		t.Fatalf("expected invalid mode error, got %v", err)
	}
}

func TestStart_InvalidMode(t *testing.T) {
	_, err := New().Start(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid runtime mode") {
		t.Fatalf("expected invalid mode error, got %v", err)
	}
}

func TestRuntimeErrOrNil_ReturnsRuntimeError(t *testing.T) {
	err := errors.New("boom")
	if got := runtimeErrOrNil(context.Background(), err); !errors.Is(got, err) {
		t.Fatalf("expected runtime error, got %v", got)
	}
}

func TestRuntimeErrOrNil_SuppressesCancellation(t *testing.T) {
	if got := runtimeErrOrNil(context.Background(), context.Canceled); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}

	if got := runtimeErrOrNil(context.Background(), context.DeadlineExceeded); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestRuntimeErrOrNil_SuppressesErrorsAfterParentContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if got := runtimeErrOrNil(ctx, errors.New("late error")); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}
