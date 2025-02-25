package chacha20

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestTCPEncoder_Encode(t *testing.T) {
	encoder := &DefaultTCPEncoder{}
	payload := []byte("test payload")

	packet, err := encoder.Encode(payload)
	if err != nil {
		t.Fatalf("unexpected error during encoding: %v", err)
	}

	// checks if length header has correct length
	if packet.Length != uint32(len(payload)) {
		t.Errorf("expected packet length %d, got %d", len(payload), packet.Length)
	}

	// packet payload = length header(4 bytes) + payload
	expectedPayload := append(make([]byte, 4), payload...)
	// puts data in length header
	binary.BigEndian.PutUint32(expectedPayload[:4], uint32(len(payload)))

	// checks if packet formed correctly
	if !bytes.Equal(packet.Payload, expectedPayload) {
		t.Errorf("payload mismatch: expected %v, got %v", expectedPayload, packet.Payload)
	}
}

func TestTCPEncoder_Decode(t *testing.T) {
	encoder := &DefaultTCPEncoder{}
	payload := []byte("test payload")

	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(payload)))
	rawData := append(lengthBuf, payload...)

	packet, err := encoder.Decode(rawData)
	if err != nil {
		t.Fatalf("unexpected error during decoding: %v", err)
	}

	if packet.Length != uint32(len(rawData)) {
		t.Errorf("expected decoded length %d, got %d", len(rawData), packet.Length)
	}

	if !bytes.Equal(packet.Payload, rawData) {
		t.Errorf("payload mismatch: expected %v, got %v", rawData, packet.Payload)
	}
}

func TestTCPEncodeDecode(t *testing.T) {
	encoder := &DefaultTCPEncoder{}
	payload := []byte("test payload")

	encodedPacket, err := encoder.Encode(payload)
	if err != nil {
		t.Fatalf("unexpected error during encoding: %v", err)
	}

	decodedPacket, err := encoder.Decode(encodedPacket.Payload)
	if err != nil {
		t.Fatalf("unexpected error during decoding: %v", err)
	}

	if decodedPacket.Length != uint32(len(payload)+4) {
		t.Errorf("expected decoded length %d, got %d", len(payload)+4, decodedPacket.Length)
	}

	if !bytes.Equal(decodedPacket.Payload, encodedPacket.Payload) {
		t.Errorf("payload mismatch: expected %v, got %v", encodedPacket.Payload, decodedPacket.Payload)
	}
}
