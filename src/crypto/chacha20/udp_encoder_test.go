package chacha20

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestUDPEncoder_Encode(t *testing.T) {
	encoder := &DefaultUDPEncoder{}
	payload := []byte("test payload")
	nonce := &Nonce{high: 1234, low: 567890}

	packet, err := encoder.Encode(payload, nonce)
	if err != nil {
		t.Fatalf("unexpected error during encoding: %v", err)
	}

	if packet.Nonce.high != nonce.high || packet.Nonce.low != nonce.low {
		t.Errorf("expected nonce %v, got %v", nonce, packet.Nonce)
	}

	expectedPayload := make([]byte, 0, 12+len(payload))
	high := make([]byte, 4)
	binary.BigEndian.PutUint32(high, nonce.high)
	low := make([]byte, 8)
	binary.BigEndian.PutUint64(low, nonce.low)

	expectedPayload = append(expectedPayload, low...)
	expectedPayload = append(expectedPayload, high...)
	expectedPayload = append(expectedPayload, payload...)

	if !bytes.Equal(packet.Payload, expectedPayload) {
		t.Errorf("payload mismatch: expected %v, got %v", expectedPayload, packet.Payload)
	}
}

func TestUDPEncoder_Decode(t *testing.T) {
	encoder := &DefaultUDPEncoder{}
	payload := []byte("test payload")
	nonce := &Nonce{high: 1234, low: 567890}

	high := make([]byte, 4)
	binary.BigEndian.PutUint32(high, nonce.high)
	low := make([]byte, 8)
	binary.BigEndian.PutUint64(low, nonce.low)
	rawData := append(append(low, high...), payload...)

	// Декодируем данные
	packet, err := encoder.Decode(rawData)
	if err != nil {
		t.Fatalf("unexpected error during decoding: %v", err)
	}

	if packet.Nonce.high != nonce.high || packet.Nonce.low != nonce.low {
		t.Errorf("expected nonce %v, got %v", nonce, packet.Nonce)
	}

	if !bytes.Equal(packet.Payload, payload) {
		t.Errorf("payload mismatch: expected %v, got %v", payload, packet.Payload)
	}
}

func TestUDPEncodeDecode(t *testing.T) {
	encoder := &DefaultUDPEncoder{}
	payload := []byte("test payload")
	nonce := &Nonce{high: 1234, low: 567890}

	encodedPacket, err := encoder.Encode(payload, nonce)
	if err != nil {
		t.Fatalf("unexpected error during encoding: %v", err)
	}

	decodedPacket, err := encoder.Decode(encodedPacket.Payload)
	if err != nil {
		t.Fatalf("unexpected error during decoding: %v", err)
	}

	if decodedPacket.Nonce.high != nonce.high || decodedPacket.Nonce.low != nonce.low {
		t.Errorf("expected nonce %v, got %v", nonce, decodedPacket.Nonce)
	}

	if !bytes.Equal(decodedPacket.Payload, payload) {
		t.Errorf("payload mismatch: expected %v, got %v", payload, decodedPacket.Payload)
	}
}
