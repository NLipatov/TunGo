package service

import (
	"errors"
	"io"
	"testing"
)

// Ensures the constructor returns a non-nil handler.
func TestNewDefaultPacketHandler(t *testing.T) {
	t.Parallel()
	h := NewDefaultPacketHandler()
	if h == nil {
		t.Fatal("NewDefaultPacketHandler returned nil")
	}
}

// Table-driven tests for TryParseType covering all branches.
func TestDefaultPacketHandler_TryParseType(t *testing.T) {
	t.Parallel()
	h := NewDefaultPacketHandler()

	tests := []struct {
		name   string
		in     []byte
		wantT  PacketType
		wantOK bool
	}{
		{
			name:   "legacy: session reset",
			in:     []byte{byte(SessionReset)},
			wantT:  SessionReset,
			wantOK: true,
		},
		{
			name:   "legacy: unknown type",
			in:     []byte{0xEE},
			wantT:  Unknown,
			wantOK: false,
		},
		{
			name:   "v1: valid framed reset",
			in:     []byte{Prefix, VersionV1, byte(SessionReset)},
			wantT:  SessionReset,
			wantOK: true,
		},
		{
			name:   "v1: header rekey ack no payload (invalid)",
			in:     []byte{Prefix, VersionV1, byte(RekeyAck)},
			wantT:  Unknown,
			wantOK: false,
		},
		{
			name:   "v1: rekey init with payload",
			in:     append([]byte{Prefix, VersionV1, byte(RekeyInit)}, make([]byte, RekeyPublicKeyLen)...),
			wantT:  RekeyInit,
			wantOK: true,
		},
		{
			name:   "v1: rekey ack with payload",
			in:     append([]byte{Prefix, VersionV1, byte(RekeyAck)}, make([]byte, RekeyPublicKeyLen)...),
			wantT:  RekeyAck,
			wantOK: true,
		},
		{
			name:   "v1: wrong prefix",
			in:     []byte{0xFE, VersionV1, byte(SessionReset)},
			wantT:  Unknown,
			wantOK: false,
		},
		{
			name:   "v1: wrong version",
			in:     []byte{Prefix, 0x02, byte(SessionReset)},
			wantT:  Unknown,
			wantOK: false,
		},
		{
			name:   "v1: unknown type",
			in:     []byte{Prefix, VersionV1, 0xFF},
			wantT:  Unknown,
			wantOK: false,
		},
		{
			name:   "len=0",
			in:     nil,
			wantT:  Unknown,
			wantOK: false,
		},
		{
			name:   "len=2",
			in:     []byte{Prefix, VersionV1},
			wantT:  Unknown,
			wantOK: false,
		},
		{
			name:   "len=4",
			in:     []byte{0, 0, 0, 0},
			wantT:  Unknown,
			wantOK: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotT, ok := h.TryParseType(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tc.wantOK)
			}
			if gotT != tc.wantT {
				t.Fatalf("type=%v, want %v", gotT, tc.wantT)
			}
		})
	}
}

// Tests for EncodeLegacy covering success, short buffer, and invalid type.
func TestDefaultPacketHandler_EncodeLegacy(t *testing.T) {
	t.Parallel()
	h := NewDefaultPacketHandler()

	t.Run("short buffer", func(t *testing.T) {
		t.Parallel()
		_, err := h.EncodeLegacy(SessionReset, make([]byte, 0))
		if !errors.Is(err, io.ErrShortBuffer) {
			t.Fatalf("err=%v, want io.ErrShortBuffer", err)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		t.Parallel()
		_, err := h.EncodeLegacy(Unknown, make([]byte, 1))
		if !errors.Is(err, ErrInvalidPacketType) {
			t.Fatalf("err=%v, want ErrInvalidPacketType", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		buf := make([]byte, 3) // larger than needed on purpose
		out, err := h.EncodeLegacy(SessionReset, buf)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(out) != 1 {
			t.Fatalf("len(out)=%d, want 1", len(out))
		}
		if out[0] != byte(SessionReset) {
			t.Fatalf("out[0]=%x, want %x", out[0], byte(SessionReset))
		}
	})
}

// Tests for EncodeV1 covering success, short buffer, and invalid type.
func TestDefaultPacketHandler_EncodeV1(t *testing.T) {
	t.Parallel()
	h := NewDefaultPacketHandler()

	t.Run("short buffer", func(t *testing.T) {
		t.Parallel()
		_, err := h.EncodeV1(SessionReset, make([]byte, 2))
		if !errors.Is(err, io.ErrShortBuffer) {
			t.Fatalf("err=%v, want io.ErrShortBuffer", err)
		}
	})

	t.Run("short buffer rekey init", func(t *testing.T) {
		t.Parallel()
		_, err := h.EncodeV1(RekeyInit, make([]byte, 3))
		if !errors.Is(err, io.ErrShortBuffer) {
			t.Fatalf("err=%v, want io.ErrShortBuffer", err)
		}
	})

	t.Run("short buffer rekey ack", func(t *testing.T) {
		t.Parallel()
		_, err := h.EncodeV1(RekeyAck, make([]byte, 3))
		if !errors.Is(err, io.ErrShortBuffer) {
			t.Fatalf("err=%v, want io.ErrShortBuffer", err)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		t.Parallel()
		_, err := h.EncodeV1(Unknown, make([]byte, 3))
		if !errors.Is(err, ErrInvalidPacketType) {
			t.Fatalf("err=%v, want ErrInvalidPacketType", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		buf := make([]byte, 3)
		out, err := h.EncodeV1(SessionReset, buf)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(out) != 3 {
			t.Fatalf("len(out)=%d, want 3", len(out))
		}
		if out[0] != Prefix || out[1] != VersionV1 || out[2] != byte(SessionReset) {
			t.Fatalf("out=%v, want [%#x %#x %#x]", out, Prefix, VersionV1, byte(SessionReset))
		}
	})

	t.Run("success rekey init", func(t *testing.T) {
		t.Parallel()
		buf := make([]byte, RekeyPacketLen)
		out, err := h.EncodeV1(RekeyInit, buf)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(out) != RekeyPacketLen {
			t.Fatalf("len(out)=%d, want %d", len(out), RekeyPacketLen)
		}
		if out[0] != Prefix || out[1] != VersionV1 || out[2] != byte(RekeyInit) {
			t.Fatalf("out=%v, want [%#x %#x %#x ...]", out[:3], Prefix, VersionV1, byte(RekeyInit))
		}
	})

	t.Run("success rekey ack", func(t *testing.T) {
		t.Parallel()
		buf := make([]byte, RekeyPacketLen)
		out, err := h.EncodeV1(RekeyAck, buf)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(out) != RekeyPacketLen {
			t.Fatalf("len(out)=%d, want %d", len(out), RekeyPacketLen)
		}
		if out[0] != Prefix || out[1] != VersionV1 || out[2] != byte(RekeyAck) {
			t.Fatalf("out=%v, want [%#x %#x %#x ...]", out[:3], Prefix, VersionV1, byte(RekeyAck))
		}
	})
}

// Optional fuzz test (cheap). Run locally with: go test -fuzz=Fuzz -fuzztime=5s
func FuzzDefaultPacketHandler_TryParseType(f *testing.F) {
	h := NewDefaultPacketHandler()
	f.Add([]byte{byte(SessionReset)})                    // legacy valid
	f.Add([]byte{Prefix, VersionV1, byte(SessionReset)}) // v1 valid
	f.Add([]byte{Prefix, 0x00, 0x00})                    // wrong version
	f.Add([]byte{0xFE, VersionV1, byte(SessionReset)})   // wrong prefix
	f.Add([]byte{})                                      // empty
	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic and must return a boolean deterministically.
		_, _ = h.TryParseType(data)
	})
}
