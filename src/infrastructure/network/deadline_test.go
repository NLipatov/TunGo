package network

import (
	"errors"
	"testing"
	"time"
)

func TestNewDeadline(t *testing.T) {
	d, err := NewDeadline(150 * time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Duration(d) != 150*time.Millisecond {
		t.Fatalf("got %v, want %v", d, 150*time.Millisecond)
	}

	if _, err := NewDeadline(-1); !errors.Is(err, ErrInvalidDuration) {
		t.Fatalf("expected ErrInvalidDuration, got %v", err)
	}
}

func TestDeadline_Time(t *testing.T) {
	// zero → no deadline
	if got := Deadline(0).Time(); !got.IsZero() {
		t.Fatalf("zero duration should return zero time, got %v", got)
	}

	// positive duration
	const dur = 100 * time.Millisecond
	dl := Deadline(dur)

	before := time.Now().Add(dur - 5*time.Millisecond)
	after := time.Now().Add(dur + 5*time.Millisecond)
	got := dl.Time()

	if got.Before(before) || got.After(after) {
		t.Fatalf("time out of expected range: %v not in [%v, %v]", got, before, after)
	}
}
