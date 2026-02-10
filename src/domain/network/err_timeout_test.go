package network

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// Compile-time check that ErrTimeout implements net.Error (via duck-typing).
type netError interface {
	error
	Timeout() bool
	Temporary() bool
}

var _ netError = ErrTimeout{cause: context.DeadlineExceeded}

func TestErrTimeout_ErrorReturnsCauseMessage(t *testing.T) {
	e := ErrTimeout{cause: context.DeadlineExceeded}
	if e.Error() == "" {
		t.Fatalf("Error() must return non-empty message")
	}
	// It should include the cause's message
	if got := e.Error(); got != context.DeadlineExceeded.Error() {
		t.Fatalf("Error() mismatch: got %q want %q", got, context.DeadlineExceeded.Error())
	}
}

func TestErrTimeout_UnwrapAndErrorsIs_As(t *testing.T) {
	e := ErrTimeout{cause: context.DeadlineExceeded}

	// Unwrap via errors.Is
	if !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("errors.Is must see the cause")
	}
	// Unwrap when additionally wrapped
	wrapped := fmt.Errorf("wrap: %w", e)
	if !errors.Is(wrapped, context.DeadlineExceeded) {
		t.Fatalf("errors.Is must see cause through additional wrap")
	}

	// errors.As to our type
	var et ErrTimeout
	if !errors.As(e, &et) {
		t.Fatalf("errors.As must match ErrTimeout")
	}
	if !errors.Is(et.cause, context.DeadlineExceeded) {
		t.Fatalf("As must preserve cause")
	}

	// Negative case
	if errors.Is(e, context.Canceled) {
		t.Fatalf("errors.Is must not match unrelated cause")
	}
}

func TestNewErrTimeout(t *testing.T) {
	cause := context.DeadlineExceeded
	et := NewErrTimeout(cause)
	if et == nil {
		t.Fatal("expected non-nil ErrTimeout")
	}
	if !errors.Is(et, cause) {
		t.Fatal("expected Unwrap to return cause")
	}
	if !et.Timeout() {
		t.Fatal("expected Timeout()=true")
	}
}

func TestErrTimeout_TimeoutAndTemporaryFlags(t *testing.T) {
	e := ErrTimeout{cause: context.DeadlineExceeded}

	// Satisfies net.Error semantics
	var ne netError = e
	if !ne.Timeout() {
		t.Fatalf("Timeout() must be true")
	}
	if ne.Temporary() {
		t.Fatalf("Temporary() must be false")
	}
}
