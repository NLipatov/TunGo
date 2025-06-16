package network

import (
	"bytes"
	"fmt"
	"testing"
)

func TestReadDestinationAddressBytes_Success(t *testing.T) {
	// Build a minimal valid IPv4 header with version=4 and 20 bytes length
	data := make([]byte, 20)
	data[0] = 4<<4 | 5 // version=4, IHL=5 (not used but realistic)
	// fill destination address bytes (16-19)
	want := []byte{10, 20, 30, 40}
	copy(data[16:20], want)

	h := NewIPV4HeaderParser()
	buf := make([]byte, 4)
	err := h.ParseDestinationAddressBytes(data, buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(buf, want) {
		t.Errorf("buffer = %v, want %v", buf, want)
	}
}

func TestReadDestinationAddressBytes_BufferTooSmall(t *testing.T) {
	data := make([]byte, 20)
	h := NewIPV4HeaderParser()

	for _, size := range []int{0, 1, 2, 3} {
		buf := make([]byte, size)
		err := h.ParseDestinationAddressBytes(data, buf)
		if err == nil {
			t.Errorf("expected error for buffer size %d, got nil", size)
			continue
		}
		expected := fmt.Sprintf("invalid buffer size, expected 4 bytes, got %d", size)
		if err.Error() != expected {
			t.Errorf("error = %q, want %q", err.Error(), expected)
		}
	}
}

func TestReadDestinationAddressBytes_PacketTooShort(t *testing.T) {
	// data shorter than 20 bytes
	data := make([]byte, 10)
	h := NewIPV4HeaderParser()
	buf := make([]byte, 4)
	err := h.ParseDestinationAddressBytes(data, buf)
	if err == nil {
		t.Fatal("expected packet size error, got nil")
	}
	expected := fmt.Sprintf("invalid packet size: %d", len(data))
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

func TestReadDestinationAddressBytes_InvalidVersion(t *testing.T) {
	// data length ok but version !=4
	data := make([]byte, 20)
	data[0] = 6<<4 | 5 // version=6
	h := NewIPV4HeaderParser()
	buf := make([]byte, 4)
	err := h.ParseDestinationAddressBytes(data, buf)
	if err == nil {
		t.Fatal("expected version error, got nil")
	}
	expected := fmt.Sprintf("invalid packet version: %d", data[0])
	if err.Error() != expected {
		t.Errorf("error = %q, want %q", err.Error(), expected)
	}
}

func TestIPHeaderV4_ImplementsInterface(t *testing.T) {
	var _ interface {
		ParseDestinationAddressBytes(header, resultBuffer []byte) error
	} = &IPV4HeaderParser{}
}
