package network

import (
	"testing"
	"time"
)

// Test zero time -> "no deadline" (nil/disabled), no error, ExpiresAt is zero.
func TestDeadlineFromTime_ZeroMeansNoDeadline(t *testing.T) {
	d, err := DeadlineFromTime(time.Time{})
	if err != nil {
		t.Fatalf("expected nil error for zero deadline, got %v", err)
	}
	if !d.ExpiresAt().IsZero() {
		t.Fatalf("expected zero ExpiresAt for disabled deadline, got %v", d.ExpiresAt())
	}
}

// Test past time -> ErrDeadlineInPast.
func TestDeadlineFromTime_Past(t *testing.T) {
	past := time.Now().Add(-1 * time.Millisecond)
	_, err := DeadlineFromTime(past)
	if err == nil {
		t.Fatalf("expected error for past deadline, got nil")
	}
	if err != ErrDeadlineInPast {
		t.Fatalf("expected ErrDeadlineInPast, got %v", err)
	}
}

// Test "now" (or effectively <= now) -> ErrDeadlineInPast.
// We pass time.Now(), and inside function 'now' is captured after our call,
// so the deadline is not strictly in the future.
func TestDeadlineFromTime_NowOrEqual(t *testing.T) {
	atCall := time.Now()
	_, err := DeadlineFromTime(atCall)
	if err == nil {
		t.Fatalf("expected error for deadline at now, got nil")
	}
	if err != ErrDeadlineInPast {
		t.Fatalf("expected ErrDeadlineInPast, got %v", err)
	}
}

// Test future time -> OK, ExpiresAt equals the provided moment.
func TestDeadlineFromTime_Future(t *testing.T) {
	fut := time.Now().Add(50 * time.Millisecond)
	d, err := DeadlineFromTime(fut)
	if err != nil {
		t.Fatalf("expected nil error for future deadline, got %v", err)
	}
	if !d.ExpiresAt().Equal(fut) {
		t.Fatalf("ExpiresAt mismatch: got %v, want %v", d.ExpiresAt(), fut)
	}
	// Sanity: ExpiresAt is non-zero for enabled deadlines.
	if d.ExpiresAt().IsZero() {
		t.Fatalf("expected non-zero ExpiresAt for enabled deadline")
	}
}

// Explicit test for the getter; mostly redundant, but ensures method is covered.
func TestDeadline_ExpiresAtGetter(t *testing.T) {
	fut := time.Now().Add(10 * time.Millisecond)
	d, err := DeadlineFromTime(fut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := d.ExpiresAt()
	if !got.Equal(fut) {
		t.Fatalf("ExpiresAt() = %v, want %v", got, fut)
	}
}
