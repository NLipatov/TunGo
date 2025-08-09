package network

import (
	"bytes"
	"fmt"
	"testing"
)

// helper: build minimal IPv4 header (20 bytes) with given version and dst
func buildIPv4Header(version uint8, dst [4]byte) []byte {
	h := make([]byte, 20)
	h[0] = (version << 4) | 5 // Version=version, IHL=5 (20 bytes)
	copy(h[16:20], dst[:])    // dst at bytes 16..19
	return h
}

func TestIPV4HeaderParser_Success(t *testing.T) {
	p := NewIPV4HeaderParser()
	dst := [4]byte{10, 20, 30, 40}
	header := buildIPv4Header(4, dst)

	out := make([]byte, 4)
	if err := p.ParseDestinationAddressBytes(header, out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(out, dst[:]) {
		t.Fatalf("dst mismatch: got %v, want %v", out, dst)
	}
}

func TestIPV4HeaderParser_BufferTooSmall(t *testing.T) {
	p := NewIPV4HeaderParser()
	header := buildIPv4Header(4, [4]byte{1, 2, 3, 4})

	for _, size := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			out := make([]byte, size)
			err := p.ParseDestinationAddressBytes(header, out)
			if err == nil {
				t.Fatalf("expected error for buffer size %d, got nil", size)
			}
			want := fmt.Sprintf("invalid buffer size, expected 4 bytes, got %d", size)
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestIPV4HeaderParser_PacketTooShort(t *testing.T) {
	p := NewIPV4HeaderParser()
	for _, n := range []int{0, 1, 10, 19} {
		t.Run(fmt.Sprintf("len=%d", n), func(t *testing.T) {
			header := make([]byte, n) // < ipv4.HeaderLen (20)
			out := make([]byte, 4)
			err := p.ParseDestinationAddressBytes(header, out)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			want := fmt.Sprintf("invalid packet size: too small (%d bytes)", n)
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestIPV4HeaderParser_InvalidVersion(t *testing.T) {
	p := NewIPV4HeaderParser()
	for _, ver := range []uint8{0, 5, 6, 7, 15} {
		t.Run(fmt.Sprintf("ver=%d", ver), func(t *testing.T) {
			header := buildIPv4Header(ver, [4]byte{1, 2, 3, 4})
			out := make([]byte, 4)
			err := p.ParseDestinationAddressBytes(header, out)
			if err == nil {
				t.Fatalf("expected version error, got nil")
			}
			want := fmt.Errorf("invalid packet version: got version %d, expected version 4(ipv4)", ver).Error()
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestIPV4HeaderParser_ImplementsInterface(t *testing.T) {
	var _ IPHeader = &IPV4HeaderParser{}
}
