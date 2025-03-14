package chacha20

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDefaultUDPEncoderDecode(t *testing.T) {
	encoder := &DefaultUDPEncoder{}

	// Prepare test values.
	low := uint64(0x0102030405060708)
	high := uint32(0x0A0B0C0D)
	payload := []byte("test payload")

	// Build data: first 8 bytes - low, next 4 bytes - high, then payload.
	data := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint64(data[:8], low)
	binary.BigEndian.PutUint32(data[8:12], high)
	copy(data[12:], payload)

	// Decode the data.
	packet, err := encoder.Decode(data)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	// Check that the nonce is properly decoded.
	if packet.Nonce == nil {
		t.Fatal("Decoded packet has nil nonce")
	}
	if packet.Nonce.low != low {
		t.Errorf("Expected nonce.low %x, got %x", low, packet.Nonce.low)
	}
	if packet.Nonce.high != high {
		t.Errorf("Expected nonce.high %x, got %x", high, packet.Nonce.high)
	}

	// Check that payload is correctly extracted.
	if !bytes.Equal(packet.Payload, payload) {
		t.Errorf("Expected payload %q, got %q", payload, packet.Payload)
	}
}
