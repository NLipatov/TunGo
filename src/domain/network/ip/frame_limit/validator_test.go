package framelimit

import (
	"errors"
	"testing"
)

func TestNewCap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		in     int
		wantOk bool
	}{
		{"zero", 0, false},
		{"negative", -1, false},
		{"one", 1, true},
		{"large", 65535, true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, err := NewCap(tt.in)
			if tt.wantOk {
				if err != nil {
					t.Fatalf("NewCap(%d) unexpected err: %v", tt.in, err)
				}
				if int(c) != tt.in {
					t.Fatalf("cap value mismatch: got=%d want=%d", int(c), tt.in)
				}
			} else {
				if err == nil {
					t.Fatalf("NewCap(%d) expected error, got nil", tt.in)
				}
				if tt.in <= 0 && !errors.Is(err, ErrZeroCap) {
					t.Fatalf("expected ErrZeroCap, got %v", err)
				}
			}
		})
	}
}

func TestValidateLen_Bounds(t *testing.T) {
	t.Parallel()

	c, err := NewCap(10)
	if err != nil {
		t.Fatalf("setup NewCap: %v", err)
	}

	cases := []struct {
		n      int
		hasErr bool
	}{
		{0, false},
		{1, false},
		{10, false}, // == cap
		{11, true},  // > cap
	}
	for _, cs := range cases {
		err := c.ValidateLen(cs.n)
		if cs.hasErr && err == nil {
			t.Fatalf("ValidateLen(%d): want error, got nil", cs.n)
		}
		if !cs.hasErr && err != nil {
			t.Fatalf("ValidateLen(%d): want nil, got %v", cs.n, err)
		}
	}
}

func TestValidateLen_Negative(t *testing.T) {
	t.Parallel()

	c, _ := NewCap(5)
	err := c.ValidateLen(-1)
	if !errors.Is(err, ErrNegativeLength) {
		t.Fatalf("expected ErrNegativeLength, got %v", err)
	}
}

func TestValidateLen_ErrorWrapping(t *testing.T) {
	t.Parallel()

	c, _ := NewCap(3)
	err := c.ValidateLen(4)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Must unwrap to ErrCapExceeded
	if !errors.Is(err, ErrCapExceeded) {
		t.Fatalf("expected errors.Is(err, ErrCapExceeded), got %v", err)
	}
}

// Fuzz test basic properties:
// 1) n < 0  -> ErrNegativeLength
// 2) 0 <= n <= cap -> nil
// 3) n > cap -> ErrCapExceeded
func FuzzCapValidateLen(f *testing.F) {
	// seeds
	f.Add(int(0))
	f.Add(int(1))
	f.Add(int(-1))
	f.Add(int(10000))

	const capValue = 1024
	c, _ := NewCap(capValue)

	f.Fuzz(func(t *testing.T, n int) {
		err := c.ValidateLen(n)
		switch {
		case n < 0:
			if !errors.Is(err, ErrNegativeLength) {
				t.Fatalf("n=%d: expected ErrNegativeLength, got %v", n, err)
			}
		case n <= capValue:
			if err != nil {
				t.Fatalf("n=%d: expected nil, got %v", n, err)
			}
		default: // n > cap
			if !errors.Is(err, ErrCapExceeded) {
				t.Fatalf("n=%d: expected ErrCapExceeded, got %v", n, err)
			}
		}
	})
}

// Optional micro-benchmarks to ensure zero-alloc fast path.
func BenchmarkValidateLen_UnderCap(b *testing.B) {
	c, _ := NewCap(4096)
	for i := 0; i < b.N; i++ {
		if err := c.ValidateLen(1024); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateLen_OverCap(b *testing.B) {
	c, _ := NewCap(4096)
	for i := 0; i < b.N; i++ {
		_ = c.ValidateLen(1 << 20) // expect error, ignore
	}
}
