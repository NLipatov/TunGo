package handshake

import (
	"bytes"
	"testing"
)

func makeBytes(n byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

func TestMarshalBinary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{"too short", makeBytes(0), true},
		{"just under", makeBytes(63), true},
		{"exact", makeBytes(64), false},
		{"too long", makeBytes(65), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sig := NewSignature(tc.data)
			out, err := sig.MarshalBinary()
			if tc.wantErr {
				if err == nil {
					t.Errorf("MarshalBinary() expected error, got nil")
				}
				return
			}
			// success case: no error, output length must be 64 and match input
			if err != nil {
				t.Fatalf("MarshalBinary() unexpected error: %v", err)
			}
			if len(out) != 64 {
				t.Errorf("MarshalBinary() length = %d; want 64", len(out))
			}
			if !bytes.Equal(out, tc.data) {
				t.Errorf("MarshalBinary() data mismatch")
			}
			// verify it's a copy, not the same backing array
			out[0] ^= 0xFF
			if tc.data[0] == out[0] {
				t.Errorf("MarshalBinary() did not return a copy")
			}
		})
	}
}

func TestUnmarshalBinary(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   []byte
		wantErr bool
	}{
		{"too short", makeBytes(0), true},
		{"just under", makeBytes(63), true},
		{"exact", makeBytes(64), false},
		{"too long", makeBytes(65), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sig Signature
			err := sig.UnmarshalBinary(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("UnmarshalBinary() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalBinary() unexpected error: %v", err)
			}
			// on success, MarshalBinary should round‑trip
			out, err := sig.MarshalBinary()
			if err != nil {
				t.Fatalf("after Unmarshal, MarshalBinary() error: %v", err)
			}
			if !bytes.Equal(out, tc.input[:64]) {
				t.Errorf("round‑trip mismatch")
			}
		})
	}
}
