package ip

import (
	"fmt"
	"testing"
)

// Tests for Valid method.
func TestVersion_Valid(t *testing.T) {
	cases := []struct {
		name string
		in   Version
		want bool
	}{
		{"V4 is valid", V4, true},
		{"V6 is valid", V6, true},
		{"Unknown is not valid", Unknown, false},
		{"Arbitrary invalid (5) is not valid", Version(5), false},
		{"Arbitrary invalid (255) is not valid", Version(255), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Valid(); got != tc.want {
				t.Fatalf("Valid() = %v, want %v (value=%d)", got, tc.want, tc.in)
			}
		})
	}
}

// Tests for FromByte with valid inputs.
func TestFromByte_Valid(t *testing.T) {
	cases := []struct {
		in   byte
		want Version
	}{
		{4, V4},
		{6, V6},
	}
	for _, tc := range cases {
		got, err := FromByte(tc.in)
		if err != nil {
			t.Fatalf("FromByte(%d) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("FromByte(%d) = %v, want %v", tc.in, got, tc.want)
		}
		// Round-trip check: Byte() returns the original number.
		if b := got.Byte(); b != tc.in {
			t.Fatalf("round-trip Byte() = %d, want %d", b, tc.in)
		}
	}
}

// Tests for FromByte with invalid inputs.
func TestFromByte_Invalid(t *testing.T) {
	invalid := []byte{0, 1, 2, 3, 5, 7, 8, 42, 255}
	for _, in := range invalid {
		got, err := FromByte(in)
		if err == nil {
			t.Fatalf("FromByte(%d) expected error, got nil", in)
		}
		// Error message should be stable and informative.
		wantMsg := fmt.Sprintf("invalid IP version: %d", in)
		if err.Error() != wantMsg {
			t.Fatalf("FromByte(%d) error = %q, want %q", in, err.Error(), wantMsg)
		}
		// On error we expect the zero value (Unknown).
		if got != Unknown {
			t.Fatalf("FromByte(%d) = %v, want Unknown(0) on error", in, got)
		}
	}
}

// Tests for Byte accessor.
func TestVersion_Byte(t *testing.T) {
	cases := []struct {
		in   Version
		want byte
	}{
		{V4, 4},
		{V6, 6},
		{Unknown, 0},
		{Version(5), 5},
	}
	for _, tc := range cases {
		if b := tc.in.Byte(); b != tc.want {
			t.Fatalf("(%d).Byte() = %d, want %d", tc.in, b, tc.want)
		}
	}
}
