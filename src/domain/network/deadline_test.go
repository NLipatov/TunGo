package network

import (
	"testing"
	"time"
)

func TestInfiniteDeadline(t *testing.T) {
	d := InfiniteDeadline()
	if !d.ExpiresAt().IsZero() {
		t.Fatalf("InfiniteDeadline must have zero time; got %v", d.ExpiresAt())
	}

	d2, err := DeadlineFromTime(time.Time{})
	if err != nil {
		t.Fatalf("DeadlineFromTime(zero) unexpected error: %v", err)
	}
	if !d.ExpiresAt().Equal(d2.ExpiresAt()) {
		t.Fatalf("zero deadlines must be equal: %v vs %v", d.ExpiresAt(), d2.ExpiresAt())
	}
}

func TestDeadlineFromTime(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
	}{
		{
			name: "zero_is_infinite",
			in:   time.Time{},
		},
		{
			name: "non_zero_utc",
			in:   time.Date(2030, 1, 2, 3, 4, 5, 6, time.UTC),
		},
		{
			name: "past_time_allowed_immediate_timeout_semantics",
			in:   time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeadlineFromTime(tt.in)
			if err != nil {
				t.Fatalf("DeadlineFromTime(%v) unexpected error: %v", tt.in, err)
			}
			if !got.ExpiresAt().Equal(tt.in) {
				t.Fatalf("ExpiresAt mismatch: want %v, got %v", tt.in, got.ExpiresAt())
			}
		})
	}
}

func TestExpiresAtReturnsExactValue(t *testing.T) {
	ref := time.Date(2029, 12, 31, 23, 59, 59, 123, time.FixedZone("Z", 0))
	d, err := DeadlineFromTime(ref)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.ExpiresAt().Equal(ref) {
		t.Fatalf("ExpiresAt should return the exact value set; want %v, got %v", ref, d.ExpiresAt())
	}
}
