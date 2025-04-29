package handshake

import (
	"bytes"
	"testing"
)

func TestClientHelloReadValidIPv4(t *testing.T) {
	ip := "127.0.0.1"
	edPubKey := make([]byte, 32)
	curvePubKey := make([]byte, 32)
	clientNonce := make([]byte, 32)

	data := append([]byte{4, byte(len(ip))}, ip...)
	data = append(data, edPubKey...)
	data = append(data, curvePubKey...)
	data = append(data, clientNonce...)

	clientHello := &ClientHello{}
	parsed, err := clientHello.Read(data)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if parsed.IpAddress != ip {
		t.Errorf("Expected IP %s, got %s", ip, parsed.IpAddress)
	}
	if !bytes.Equal(parsed.EdPublicKey, edPubKey) {
		t.Errorf("EdPublicKey mismatch")
	}
	if !bytes.Equal(parsed.CurvePublicKey, curvePubKey) {
		t.Errorf("CurvePublicKey mismatch")
	}
	if !bytes.Equal(parsed.ClientNonce, clientNonce) {
		t.Errorf("ClientNonce mismatch")
	}
}

func TestClientHelloReadInvalidLength(t *testing.T) {
	data := make([]byte, minClientHelloSizeBytes-1)
	clientHello := &ClientHello{}

	_, err := clientHello.Read(data)
	if err == nil {
		t.Fatal("Expected error for invalid message length, got nil")
	}
}

func TestClientHelloReadInvalidIPVersion(t *testing.T) {
	data := make([]byte, minClientHelloSizeBytes)
	data[0] = 7 // Invalid IP version

	clientHello := &ClientHello{}
	_, err := clientHello.Read(data)
	if err == nil {
		t.Fatal("Expected error for invalid IP version, got nil")
	}
}

func TestClientHelloWriteValid(t *testing.T) {
	ip := "192.168.0.1"
	ipVersion := uint8(4)
	edPubKey := make([]byte, 32)
	curvePubKey := make([]byte, 32)
	clientNonce := make([]byte, 32)

	clientHello := &ClientHello{}
	data, err := clientHello.Write(ipVersion, ip, edPubKey, &curvePubKey, &clientNonce)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	parsed := &ClientHello{}
	_, err = parsed.Read(*data)
	if err != nil {
		t.Fatalf("Expected no error during Read, got: %v", err)
	}

	if parsed.IpAddress != ip {
		t.Errorf("Expected IP %s, got %s", ip, parsed.IpAddress)
	}
	if !bytes.Equal(parsed.EdPublicKey, edPubKey) {
		t.Errorf("EdPublicKey mismatch")
	}
	if !bytes.Equal(parsed.CurvePublicKey, curvePubKey) {
		t.Errorf("CurvePublicKey mismatch")
	}
	if !bytes.Equal(parsed.ClientNonce, clientNonce) {
		t.Errorf("ClientNonce mismatch")
	}
}

func TestClientHelloWriteInvalidIPVersion(t *testing.T) {
	clientHello := &ClientHello{}
	_, err := clientHello.Write(7, "127.0.0.1", nil, nil, nil) // Invalid IP version
	if err == nil {
		t.Fatal("Expected error for invalid IP version, got nil")
	}
}

func TestClientHelloWriteInvalidIPv4Address(t *testing.T) {
	clientHello := &ClientHello{}
	_, err := clientHello.Write(4, "12345", nil, nil, nil) // Invalid IPv4 address
	if err == nil {
		t.Fatal("Expected error for invalid IPv4 address, got nil")
	}
}

func TestClientHelloWriteInvalidIPv6Address(t *testing.T) {
	clientHello := &ClientHello{}
	_, err := clientHello.Write(6, "1", nil, nil, nil) // Invalid IPv6 address
	if err == nil {
		t.Fatal("Expected error for invalid IPv6 address, got nil")
	}
}
