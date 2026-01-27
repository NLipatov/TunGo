package chacha20

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDefaultUDPEncoderDecode(t *testing.T) {
	encoder := &DefaultUDPEncoder{}

	// Prepare test values.
	epoch := Epoch(0x0A0B)
	low := uint64(0x0102030405060708)
	high := uint16(0x0C0D)
	payload := []byte("test payload")

	// Build data: 2 bytes epoch, 2 bytes high, 8 bytes low, then payload.
	data := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint16(data[0:2], uint16(epoch))
	binary.BigEndian.PutUint16(data[2:4], high)
	binary.BigEndian.PutUint64(data[4:12], low)
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
	if packet.Nonce.epoch != epoch {
		t.Errorf("Expected nonce.epoch %x, got %x", epoch, packet.Nonce.epoch)
	}
	if packet.Nonce.counterLow != low {
		t.Errorf("Expected nonce.low %x, got %x", low, packet.Nonce.counterLow)
	}
	if packet.Nonce.counterHigh != high {
		t.Errorf("Expected nonce.high %x, got %x", high, packet.Nonce.counterHigh)
	}

	// Check that payload is correctly extracted.
	if !bytes.Equal(packet.Payload, payload) {
		t.Errorf("Expected payload %q, got %q", payload, packet.Payload)
	}
}
