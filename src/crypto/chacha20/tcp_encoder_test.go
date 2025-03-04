package chacha20

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// TestDefaultTCPEncoderEncode checks that Encode correctly writes the 4-byte length prefix.
func TestDefaultTCPEncoderEncode(t *testing.T) {
	encoder := NewDefaultTCPEncoder()
	// Prepare a buffer: first 4 bytes reserved for length, followed by payload.
	payload := []byte("Hello, World!")
	buffer := make([]byte, 4+len(payload))
	copy(buffer[4:], payload)

	err := encoder.Encode(buffer)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	// The length prefix should equal the length of the payload.
	expectedLength := uint32(len(payload))
	gotLength := binary.BigEndian.Uint32(buffer[:4])
	if gotLength != expectedLength {
		t.Errorf("Expected length prefix %d, got %d", expectedLength, gotLength)
	}

	// Verify that the payload remains unchanged.
	if !bytes.Equal(buffer[4:], payload) {
		t.Errorf("Payload mismatch, expected %q, got %q", payload, buffer[4:])
	}
}

// TestDefaultTCPEncoderDecode checks that Decode correctly populates a TCPPacket.
func TestDefaultTCPEncoderDecode(t *testing.T) {
	encoder := NewDefaultTCPEncoder()
	data := []byte("Test packet data")
	packet := &TCPPacket{}

	err := encoder.Decode(data, packet)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	expectedLength := uint32(len(data))
	if packet.Length != expectedLength {
		t.Errorf("Expected packet length %d, got %d", expectedLength, packet.Length)
	}
	if !bytes.Equal(packet.Payload, data) {
		t.Errorf("Expected payload %q, got %q", data, packet.Payload)
	}
}
