package framing

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestDefaultTCPEncoderEncode(t *testing.T) {
	encoder := NewDefaultTCPEncoder()
	payload := []byte("Hello, World!")
	buffer := make([]byte, 4+len(payload))
	copy(buffer[4:], payload)

	err := encoder.Encode(buffer)
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}

	expectedLength := uint32(len(payload))
	gotLength := binary.BigEndian.Uint32(buffer[:4])
	if gotLength != expectedLength {
		t.Errorf("Expected length prefix %d, got %d", expectedLength, gotLength)
	}

	if !bytes.Equal(buffer[4:], payload) {
		t.Errorf("Payload mismatch, expected %q, got %q", payload, buffer[4:])
	}
}

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
