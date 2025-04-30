package handshake

import (
	"crypto/ed25519"
	"testing"
)

func TestClientHello_WriteAndRead(t *testing.T) {
	ipVersion := uint8(4)
	ipAddress := "192.168.1.1"
	edPublicKey := ed25519.PublicKey(make([]byte, curvePublicKeyLength)) // Example public key
	curvePublicKey := make([]byte, curvePublicKeyLength)                 // Example curve public key
	clientNonce := make([]byte, nonceLength)                             // Example nonce

	// Fill keys and nonce with some example data
	for i := 0; i < 32; i++ {
		edPublicKey[i] = byte(i)
		curvePublicKey[i] = byte(i + 32)
	}
	for i := 0; i < 24; i++ {
		clientNonce[i] = byte(i + 64)
	}

	clientHello := &ClientHello{}

	// Test Write
	data, err := clientHello.Write(ipVersion, ipAddress, edPublicKey, &curvePublicKey, &clientNonce)
	if err != nil {
		t.Fatalf("unexpected error during Write: %v", err)
	}

	// Validate the output length
	expectedLength := lengthHeaderLength + len(ipAddress) + curvePublicKeyLength*2 + nonceLength
	if len(*data) != expectedLength {
		t.Errorf("expected length %d, got %d", expectedLength, len(*data))
	}

	// Test Read
	decodedHello, err := clientHello.Read(*data)
	if err != nil {
		t.Fatalf("unexpected error during Read: %v", err)
	}

	// Validate IP version
	if decodedHello.IpVersion != ipVersion {
		t.Errorf("expected IP version %d, got %d", ipVersion, decodedHello.IpVersion)
	}

	// Validate IP address
	if decodedHello.IpAddress != ipAddress {
		t.Errorf("expected IP address %s, got %s", ipAddress, decodedHello.IpAddress)
	}

	// Validate Ed25519 public key
	if string(decodedHello.EdPublicKey) != string(edPublicKey) {
		t.Errorf("EdPublicKey mismatch: expected %v, got %v", edPublicKey, decodedHello.EdPublicKey)
	}

	// Validate Curve public key
	if string(decodedHello.CurvePublicKey) != string(curvePublicKey) {
		t.Errorf("CurvePublicKey mismatch: expected %v, got %v", curvePublicKey, decodedHello.CurvePublicKey)
	}

	// Validate nonce
	if string(decodedHello.ClientNonce) != string(clientNonce) {
		t.Errorf("ClientNonce mismatch: expected %v, got %v", clientNonce, decodedHello.ClientNonce)
	}
}

func TestClientHello_InvalidInput(t *testing.T) {
	clientHello := &ClientHello{}

	// Test Write with invalid IP version
	_, err := clientHello.Write(10, "192.168.1.1", nil, nil, nil)
	if err == nil {
		t.Error("expected error for invalid IP version, got nil")
	}

	// Test Write with invalid IP address
	_, err = clientHello.Write(4, "1.1", nil, nil, nil)
	if err == nil {
		t.Error("expected error for invalid IPv4 address, got nil")
	}

	// Test Read with invalid data length
	_, err = clientHello.Read([]byte{1, 2, 3})
	if err == nil {
		t.Error("expected error for invalid data length, got nil")
	}

	data := make([]byte, minClientHelloSizeBytes)
	data[0] = 4
	data[1] = uint8(len(data))
	_, err = clientHello.Read(data)
	if err == nil || err.Error() != "invalid IP address length" {
		t.Errorf("expected \"invalid IP address length\" error, got %v", err)
	}
}
